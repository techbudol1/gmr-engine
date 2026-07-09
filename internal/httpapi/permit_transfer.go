package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/techbudol1/gmr-engine/internal/vaultclient"

	"github.com/gofiber/fiber/v2"
)

type erc20PermitTransferRequest struct {
	Amount          string `json:"amount"`
	ChainID         int64  `json:"chainId"`
	ContractAddress string `json:"contractAddress"`
	Deadline        string `json:"deadline"`
	Decimals        int64  `json:"decimals"`
	Owner           string `json:"owner"`
	Recipient       string `json:"recipient"`
	R               string `json:"r"`
	S               string `json:"s"`
	ValidateOnly    bool   `json:"validateOnly"`
	V               int    `json:"v"`
}

type erc20PermitTransferResult struct {
	PermitTransactionHash   string `json:"permitTransactionHash"`
	SignatureValid          bool   `json:"signatureValid,omitempty"`
	TransferTransactionHash string `json:"transferTransactionHash"`
	Transactions            []struct {
		Kind            string `json:"kind"`
		Status          string `json:"status"`
		TransactionHash string `json:"transactionHash"`
	} `json:"transactions"`
}

func (s Server) erc20TransferWithPermit(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	if !principal.App.GasFreeEnabled {
		return fiber.NewError(fiber.StatusForbidden, "gas-free transfers are disabled for this project")
	}

	var request erc20PermitTransferRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if request.ChainID <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "valid chainId is required")
	}
	if request.Decimals == 0 {
		request.Decimals = 18
	}
	if !evmAddressPattern.MatchString(strings.TrimSpace(request.ContractAddress)) {
		return fiber.NewError(fiber.StatusBadRequest, "valid contractAddress is required")
	}
	if !evmAddressPattern.MatchString(strings.TrimSpace(request.Owner)) {
		return fiber.NewError(fiber.StatusBadRequest, "valid owner is required")
	}
	if !evmAddressPattern.MatchString(strings.TrimSpace(request.Recipient)) {
		return fiber.NewError(fiber.StatusBadRequest, "valid recipient is required")
	}
	if !isUintString(request.Deadline) {
		return fiber.NewError(fiber.StatusBadRequest, "valid deadline is required")
	}
	if !isBytes32Hex(request.R) || !isBytes32Hex(request.S) {
		return fiber.NewError(fiber.StatusBadRequest, "valid permit signature is required")
	}
	if request.V == 0 || request.V == 1 {
		request.V += 27
	}
	if request.V != 27 && request.V != 28 {
		return fiber.NewError(fiber.StatusBadRequest, "valid permit recovery id is required")
	}
	if err := enforceProjectPolicy(principal.App, request.ChainID, request.ContractAddress); err != nil {
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	}

	result, spender, err := s.runERC20PermitTransfer(c.Context(), principal.App.ID, request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	transactionIDs := make([]string, 0, len(result.Transactions))
	for _, transaction := range result.Transactions {
		if strings.TrimSpace(transaction.TransactionHash) != "" {
			transactionIDs = append(transactionIDs, transaction.TransactionHash)
		}
	}
	return c.JSON(fiber.Map{
		"amount":                  strings.TrimSpace(request.Amount),
		"gasFree":                 true,
		"ok":                      true,
		"owner":                   strings.ToLower(strings.TrimSpace(request.Owner)),
		"permitTransactionHash":   result.PermitTransactionHash,
		"recipient":               strings.ToLower(strings.TrimSpace(request.Recipient)),
		"signatureValid":          result.SignatureValid,
		"spender":                 strings.ToLower(spender),
		"transactionIds":          transactionIDs,
		"transactions":            result.Transactions,
		"transferTransactionHash": result.TransferTransactionHash,
	})
}

func (s Server) erc20ManagedTransferWithPermit(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	if !principal.App.GasFreeEnabled {
		return fiber.NewError(fiber.StatusForbidden, "gas-free transfers are disabled for this project")
	}

	var request erc20PermitTransferRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if request.ChainID <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "valid chainId is required")
	}
	if request.Decimals == 0 {
		request.Decimals = 18
	}
	if !evmAddressPattern.MatchString(strings.TrimSpace(request.ContractAddress)) {
		return fiber.NewError(fiber.StatusBadRequest, "valid contractAddress is required")
	}
	if !evmAddressPattern.MatchString(strings.TrimSpace(request.Owner)) {
		return fiber.NewError(fiber.StatusBadRequest, "valid managed owner is required")
	}
	if !evmAddressPattern.MatchString(strings.TrimSpace(request.Recipient)) {
		return fiber.NewError(fiber.StatusBadRequest, "valid recipient is required")
	}
	if !isUintString(request.Deadline) {
		return fiber.NewError(fiber.StatusBadRequest, "valid deadline is required")
	}
	if err := enforceProjectPolicy(principal.App, request.ChainID, request.ContractAddress); err != nil {
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	}

	result, spender, err := s.runERC20ManagedPermitTransfer(c.Context(), principal.App.ID, request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	transactionIDs := make([]string, 0, len(result.Transactions))
	for _, transaction := range result.Transactions {
		if strings.TrimSpace(transaction.TransactionHash) != "" {
			transactionIDs = append(transactionIDs, transaction.TransactionHash)
		}
	}
	return c.JSON(fiber.Map{
		"amount":                  strings.TrimSpace(request.Amount),
		"gasFree":                 true,
		"managed":                 true,
		"ok":                      true,
		"owner":                   strings.ToLower(strings.TrimSpace(request.Owner)),
		"permitTransactionHash":   result.PermitTransactionHash,
		"recipient":               strings.ToLower(strings.TrimSpace(request.Recipient)),
		"signatureValid":          result.SignatureValid,
		"spender":                 strings.ToLower(spender),
		"transactionIds":          transactionIDs,
		"transactions":            result.Transactions,
		"transferTransactionHash": result.TransferTransactionHash,
	})
}

func (s Server) runERC20PermitTransfer(ctx context.Context, appID string, request erc20PermitTransferRequest) (erc20PermitTransferResult, string, error) {
	wallet, ok, err := s.store.GetProjectDefaultAdminWalletSecret(ctx, appID)
	if err != nil {
		return erc20PermitTransferResult{}, "", err
	}
	if !ok {
		return erc20PermitTransferResult{}, "", errors.New("project wallet not found")
	}
	signerPayload, err := s.projectSignerPayload(ctx, appID, wallet, "")
	if err != nil {
		return erc20PermitTransferResult{}, "", err
	}
	amount, err := decimalStringToBaseUnits(request.Amount, request.Decimals)
	if err != nil {
		return erc20PermitTransferResult{}, "", err
	}
	payloadMap := map[string]any{
		"amount":          amount,
		"chainId":         request.ChainID,
		"contractAddress": strings.TrimSpace(request.ContractAddress),
		"deadline":        strings.TrimSpace(request.Deadline),
		"owner":           strings.TrimSpace(request.Owner),
		"recipient":       strings.TrimSpace(request.Recipient),
		"rpcUrl":          s.cfg.ChainRPCURL,
		"r":               strings.TrimSpace(request.R),
		"s":               strings.TrimSpace(request.S),
		"spender":         wallet.Address,
		"v":               request.V,
	}
	for key, value := range signerPayload {
		payloadMap[key] = value
	}
	payload, err := json.Marshal(payloadMap)
	if err != nil {
		return erc20PermitTransferResult{}, "", err
	}
	output, err := runERC20PermitTransferScript(ctx, payload)
	if err != nil {
		return erc20PermitTransferResult{}, "", err
	}
	var result erc20PermitTransferResult
	if err := json.Unmarshal(output, &result); err != nil {
		return erc20PermitTransferResult{}, "", err
	}
	return result, wallet.Address, nil
}

func (s Server) runERC20ManagedPermitTransfer(ctx context.Context, appID string, request erc20PermitTransferRequest) (erc20PermitTransferResult, string, error) {
	relayer, ok, err := s.store.GetProjectDefaultAdminWalletSecret(ctx, appID)
	if err != nil {
		return erc20PermitTransferResult{}, "", err
	}
	if !ok {
		return erc20PermitTransferResult{}, "", errors.New("project wallet not found")
	}
	managed, ok, err := s.store.GetActiveManagedUserWalletSecretByAddress(ctx, appID, request.Owner)
	if err != nil {
		return erc20PermitTransferResult{}, "", err
	}
	if !ok {
		return erc20PermitTransferResult{}, "", errors.New("managed user wallet not found")
	}
	if !vaultclient.IsReference(managed.VaultWalletRef) {
		return erc20PermitTransferResult{}, "", errors.New("managed user wallet has no vault signer")
	}
	relayerSigner, err := s.projectSignerPayload(ctx, appID, relayer, "")
	if err != nil {
		return erc20PermitTransferResult{}, "", err
	}
	amount, err := decimalStringToBaseUnits(request.Amount, request.Decimals)
	if err != nil {
		return erc20PermitTransferResult{}, "", err
	}
	payloadMap := map[string]any{
		"amount":              amount,
		"chainId":             request.ChainID,
		"contractAddress":     strings.TrimSpace(request.ContractAddress),
		"deadline":            strings.TrimSpace(request.Deadline),
		"owner":               managed.Address,
		"ownerVaultAddress":   managed.Address,
		"ownerVaultApiKey":    s.cfg.VaultInternalKey,
		"ownerVaultProjectId": appID,
		"ownerVaultUrl":       s.cfg.VaultURL,
		"ownerVaultWalletRef": managed.VaultWalletRef,
		"recipient":           strings.TrimSpace(request.Recipient),
		"rpcUrl":              s.cfg.ChainRPCURL,
		"spender":             relayer.Address,
		"validateOnly":        request.ValidateOnly,
	}
	for key, value := range relayerSigner {
		payloadMap[key] = value
	}
	payload, err := json.Marshal(payloadMap)
	if err != nil {
		return erc20PermitTransferResult{}, "", err
	}
	output, err := runERC20PermitTransferScript(ctx, payload)
	if err != nil {
		return erc20PermitTransferResult{}, "", err
	}
	var result erc20PermitTransferResult
	if err := json.Unmarshal(output, &result); err != nil {
		return erc20PermitTransferResult{}, "", err
	}
	return result, relayer.Address, nil
}

func runERC20PermitTransferScript(ctx context.Context, payload []byte) ([]byte, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "bun", "scripts/erc20-permit-transfer.ts")
	cmd.Stdin = bytes.NewReader(payload)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, errors.New(strings.TrimSpace(string(output)))
	}
	return bytes.TrimSpace(output), nil
}

func isBytes32Hex(value string) bool {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) != 66 || !strings.HasPrefix(trimmed, "0x") {
		return false
	}
	for _, char := range trimmed[2:] {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') && (char < 'A' || char > 'F') {
			return false
		}
	}
	return true
}

func isUintString(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	for _, char := range trimmed {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
