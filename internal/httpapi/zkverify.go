package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/techbudol1/gmr-engine/internal/store"

	"github.com/gofiber/fiber/v2"
)

type zkProofRequest struct {
	Context       json.RawMessage `json:"context"`
	DomainID      int64           `json:"domainId"`
	Proof         json.RawMessage `json:"proof"`
	ProofSystem   string          `json:"proofSystem"`
	PublicSignals json.RawMessage `json:"publicSignals"`
	VK            json.RawMessage `json:"vk"`
}

type zkVerifyScriptResult struct {
	Accounts []struct {
		Address         string `json:"address"`
		FreeBalance     string `json:"freeBalance"`
		Nonce           any    `json:"nonce"`
		ReservedBalance string `json:"reservedBalance"`
	} `json:"accounts"`
	TransactionResult json.RawMessage `json:"transactionResult"`
}

func (s Server) dashboardZKVerifyAccount(c *fiber.Ctx) error {
	account, err := s.zkVerifyAccountInfo(c.Context())
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return c.JSON(fiber.Map{"account": account, "network": s.cfg.ZKVerifyNetwork})
}

func (s Server) dashboardZKProofSubmissions(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	submissions, err := s.store.ListZKProofSubmissions(c.Context(), app.ID, queryLimit(c))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load zk proof submissions")
	}
	return c.JSON(fiber.Map{"submissions": submissions})
}

func (s Server) dashboardSubmitZKProof(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	var request zkProofRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	submission, err := s.submitZKProofForApp(c.Context(), app.ID, "", request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"submission": submission})
}

func (s Server) zkVerifyAccount(c *fiber.Ctx) error {
	account, err := s.zkVerifyAccountInfo(c.Context())
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return c.JSON(fiber.Map{"account": account, "network": s.cfg.ZKVerifyNetwork})
}

func (s Server) submitZKProof(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	var request zkProofRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	submission, err := s.submitZKProofForApp(c.Context(), principal.App.ID, principal.Key.ID, request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"submission": submission})
}

func (s Server) zkProofSubmission(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	submission, found, err := s.store.GetZKProofSubmission(c.Context(), principal.App.ID, c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load zk proof submission")
	}
	if !found {
		return fiber.NewError(fiber.StatusNotFound, "zk proof submission not found")
	}
	return c.JSON(fiber.Map{"submission": submission})
}

func (s Server) submitZKProofForApp(ctx context.Context, appID string, keyID string, request zkProofRequest) (store.ZKProofSubmission, error) {
	input := store.ZKProofSubmissionInput{
		AppID:         appID,
		Context:       rawJSONString(request.Context),
		DomainID:      request.DomainID,
		KeyID:         keyID,
		Proof:         rawJSONString(request.Proof),
		ProofSystem:   request.ProofSystem,
		PublicSignals: rawJSONString(request.PublicSignals),
		VK:            rawJSONString(request.VK),
	}
	submission, err := s.store.CreateZKProofSubmission(ctx, input)
	if err != nil {
		return store.ZKProofSubmission{}, err
	}
	result, err := s.runZKVerify(ctx, map[string]any{
		"action":        "submitProof",
		"context":       optionalRawJSON(input.Context),
		"domainId":      input.DomainID,
		"network":       s.cfg.ZKVerifyNetwork,
		"proof":         json.RawMessage(input.Proof),
		"proofSystem":   submission.ProofSystem,
		"publicSignals": optionalRawJSON(input.PublicSignals),
		"rpcUrl":        s.cfg.ZKVerifyRPC,
		"seedPhrase":    s.cfg.ZKVerifySeed,
		"vk":            json.RawMessage(input.VK),
		"websocketUrl":  s.cfg.ZKVerifyWS,
	})
	if err != nil {
		_ = s.store.MarkZKProofSubmissionFailed(ctx, submission.ID, err.Error())
		submission.Error = err.Error()
		submission.Status = "failed"
		return submission, err
	}
	accountAddress := ""
	if account, err := s.zkVerifyAccountInfo(ctx); err == nil {
		accountAddress = account.Address
	}
	resultBytes, _ := json.Marshal(result.TransactionResult)
	if err := s.store.MarkZKProofSubmissionSubmitted(ctx, submission.ID, s.cfg.ZKVerifyNetwork, accountAddress, string(resultBytes)); err != nil {
		return submission, err
	}
	submissions, err := s.store.ListZKProofSubmissions(ctx, appID, 1)
	if err == nil && len(submissions) > 0 {
		return submissions[0], nil
	}
	submission.Status = "submitted"
	submission.ZKVerifyNetwork = s.cfg.ZKVerifyNetwork
	submission.AccountAddress = accountAddress
	submission.TransactionResult = string(resultBytes)
	return submission, nil
}

func (s Server) zkVerifyAccountInfo(ctx context.Context) (struct {
	Address         string `json:"address"`
	FreeBalance     string `json:"freeBalance"`
	Nonce           any    `json:"nonce"`
	ReservedBalance string `json:"reservedBalance"`
}, error) {
	result, err := s.runZKVerify(ctx, map[string]any{
		"action":       "accountInfo",
		"network":      s.cfg.ZKVerifyNetwork,
		"rpcUrl":       s.cfg.ZKVerifyRPC,
		"seedPhrase":   s.cfg.ZKVerifySeed,
		"websocketUrl": s.cfg.ZKVerifyWS,
	})
	if err != nil {
		return struct {
			Address         string `json:"address"`
			FreeBalance     string `json:"freeBalance"`
			Nonce           any    `json:"nonce"`
			ReservedBalance string `json:"reservedBalance"`
		}{}, err
	}
	if len(result.Accounts) == 0 {
		return struct {
			Address         string `json:"address"`
			FreeBalance     string `json:"freeBalance"`
			Nonce           any    `json:"nonce"`
			ReservedBalance string `json:"reservedBalance"`
		}{}, errors.New("zkVerify account not found")
	}
	return result.Accounts[0], nil
}

func (s Server) runZKVerify(ctx context.Context, payload map[string]any) (zkVerifyScriptResult, error) {
	if strings.TrimSpace(s.cfg.ZKVerifySeed) == "" {
		return zkVerifyScriptResult{}, errors.New("GMR_ENGINE_ZKVERIFY_SEED_PHRASE is required")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return zkVerifyScriptResult{}, err
	}
	commandCtx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "bun", "scripts/zkverify.ts")
	cmd.Stdin = bytes.NewReader(body)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return zkVerifyScriptResult{}, fmt.Errorf("zkVerify call failed: %s", strings.TrimSpace(string(output)))
	}
	trimmed := bytes.TrimSpace(output)
	lines := bytes.Split(trimmed, []byte("\n"))
	jsonLine := bytes.TrimSpace(lines[len(lines)-1])
	var result zkVerifyScriptResult
	if err := json.Unmarshal(jsonLine, &result); err != nil {
		return zkVerifyScriptResult{}, err
	}
	return result, nil
}

func rawJSONString(value json.RawMessage) string {
	if len(value) == 0 || string(value) == "null" {
		return ""
	}
	return strings.TrimSpace(string(value))
}

func optionalRawJSON(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return json.RawMessage(value)
}
