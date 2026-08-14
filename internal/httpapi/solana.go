package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

const (
	solanaTokenProgramID     = "TokenkegQfeZyiNwAJbNbGKPFXCWuBvf9Ss623VQ5DA"
	solanaToken2022ProgramID = "TokenzQdBNbLqP5VEhdkAS6EPFLC1PHnBqCXEpPxuEb"
	maxSolanaTransactionSize = 1232
)

var solanaAddressPattern = regexp.MustCompile(`^[1-9A-HJ-NP-Za-km-z]{32,44}$`)
var solanaSignaturePattern = regexp.MustCompile(`^[1-9A-HJ-NP-Za-km-z]{80,90}$`)

type solanaTokenAmount struct {
	Amount         string `json:"amount"`
	Decimals       int64  `json:"decimals"`
	UIAmountString string `json:"uiAmountString"`
}

type solanaTokenAccount struct {
	Pubkey  string `json:"pubkey"`
	Account struct {
		Data struct {
			Parsed struct {
				Info struct {
					Mint        string            `json:"mint"`
					TokenAmount solanaTokenAmount `json:"tokenAmount"`
				} `json:"info"`
			} `json:"parsed"`
		} `json:"data"`
	} `json:"account"`
}

func (s Server) solanaNativeBalance(c *fiber.Ctx) error {
	principal, network, walletAddress, err := s.solanaWalletQuery(c)
	if err != nil {
		return err
	}
	var result struct {
		Context struct {
			Slot uint64 `json:"slot"`
		} `json:"context"`
		Value uint64 `json:"value"`
	}
	if err := s.solanaRPC(c.Context(), network, "getBalance", []any{walletAddress, map[string]any{"commitment": "confirmed"}}, &result); err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	raw := strconv.FormatUint(result.Value, 10)
	return c.JSON(fiber.Map{"balance": fiber.Map{
		"network": network, "walletAddress": walletAddress, "symbol": "SOL", "decimals": 9,
		"raw": raw, "value": formatBaseUnitsExact(raw, 9), "slot": result.Context.Slot,
	}, "projectId": principal.App.ID})
}

func (s Server) solanaSPLBalance(c *fiber.Ctx) error {
	_, network, walletAddress, err := s.solanaWalletQuery(c)
	if err != nil {
		return err
	}
	mintAddress := strings.TrimSpace(c.Query("mintAddress"))
	if !validSolanaAddress(mintAddress) {
		return fiber.NewError(fiber.StatusBadRequest, "valid mintAddress is required")
	}
	accounts, err := s.solanaTokenAccounts(c.Context(), network, walletAddress, map[string]any{"mint": mintAddress})
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	balance := aggregateSolanaTokenAccounts(accounts)[mintAddress]
	if balance.Mint == "" {
		balance = solanaTokenBalance{Mint: mintAddress, Raw: "0", Value: "0"}
	}
	return c.JSON(fiber.Map{"balance": balance, "network": network, "walletAddress": walletAddress})
}

func (s Server) solanaSPLBalances(c *fiber.Ctx) error {
	_, network, walletAddress, err := s.solanaWalletQuery(c)
	if err != nil {
		return err
	}
	allAccounts := make([]solanaTokenAccount, 0)
	for _, programID := range []string{solanaTokenProgramID, solanaToken2022ProgramID} {
		accounts, rpcErr := s.solanaTokenAccounts(c.Context(), network, walletAddress, map[string]any{"programId": programID})
		if rpcErr != nil {
			return fiber.NewError(fiber.StatusBadGateway, rpcErr.Error())
		}
		allAccounts = append(allAccounts, accounts...)
	}
	aggregated := aggregateSolanaTokenAccounts(allAccounts)
	balances := make([]solanaTokenBalance, 0, len(aggregated))
	for _, balance := range aggregated {
		balances = append(balances, balance)
	}
	sort.Slice(balances, func(i, j int) bool { return balances[i].Mint < balances[j].Mint })
	return c.JSON(fiber.Map{"balances": balances, "network": network, "walletAddress": walletAddress})
}

func (s Server) solanaWalletTransactions(c *fiber.Ctx) error {
	_, network, walletAddress, err := s.solanaWalletQuery(c)
	if err != nil {
		return err
	}
	limit := limitValue(c, 25, 100)
	params := []any{walletAddress, map[string]any{"commitment": "confirmed", "limit": limit}}
	if before := strings.TrimSpace(c.Query("before")); before != "" {
		if !solanaSignaturePattern.MatchString(before) {
			return fiber.NewError(fiber.StatusBadRequest, "valid before signature is required")
		}
		params[1].(map[string]any)["before"] = before
	}
	var signatures []struct {
		BlockTime          *int64 `json:"blockTime"`
		ConfirmationStatus string `json:"confirmationStatus"`
		Err                any    `json:"err"`
		Memo               any    `json:"memo"`
		Signature          string `json:"signature"`
		Slot               uint64 `json:"slot"`
	}
	if err := s.solanaRPC(c.Context(), network, "getSignaturesForAddress", params, &signatures); err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return c.JSON(fiber.Map{"network": network, "transactions": signatures, "walletAddress": walletAddress})
}

func (s Server) solanaAccount(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	network, err := normalizeSolanaRequestNetwork(c.Query("network"))
	if err != nil {
		return err
	}
	if err := enforceSolanaProjectPolicy(principal.App, network); err != nil {
		return err
	}
	address := strings.TrimSpace(c.Params("address"))
	if !validSolanaAddress(address) {
		return fiber.NewError(fiber.StatusBadRequest, "valid Solana account address is required")
	}
	var result json.RawMessage
	if err := s.solanaRPC(c.Context(), network, "getAccountInfo", []any{address, map[string]any{"commitment": "confirmed", "encoding": "base64"}}, &result); err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return c.JSON(fiber.Map{"account": result, "address": address, "network": network})
}

func (s Server) solanaLatestBlockhash(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	network, err := normalizeSolanaRequestNetwork(c.Query("network"))
	if err != nil {
		return err
	}
	if err := enforceSolanaProjectPolicy(principal.App, network); err != nil {
		return err
	}
	var result json.RawMessage
	if err := s.solanaRPC(c.Context(), network, "getLatestBlockhash", []any{map[string]any{"commitment": "confirmed"}}, &result); err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return c.JSON(fiber.Map{"blockhash": result, "network": network})
}

func (s Server) solanaSimulateTransaction(c *fiber.Ctx) error {
	network, transaction, options, err := s.solanaTransactionRequest(c)
	if err != nil {
		return err
	}
	options["sigVerify"] = true
	var result json.RawMessage
	if err := s.solanaRPC(c.Context(), network, "simulateTransaction", []any{transaction, options}, &result); err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return c.JSON(fiber.Map{"network": network, "simulation": result})
}

func (s Server) solanaSendTransaction(c *fiber.Ctx) error {
	network, transaction, options, err := s.solanaTransactionRequest(c)
	if err != nil {
		return err
	}
	options["skipPreflight"] = false
	var signature string
	if err := s.solanaRPC(c.Context(), network, "sendTransaction", []any{transaction, options}, &signature); err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"network": network, "signature": signature, "status": "submitted"})
}

func (s Server) solanaWalletQuery(c *fiber.Ctx) (principal, string, string, error) {
	project, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return principal{}, "", "", fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	network, err := normalizeSolanaRequestNetwork(c.Query("network"))
	if err != nil {
		return principal{}, "", "", err
	}
	if err := enforceSolanaProjectPolicy(project.App, network); err != nil {
		return principal{}, "", "", err
	}
	walletAddress := strings.TrimSpace(c.Query("walletAddress"))
	if !validSolanaAddress(walletAddress) {
		return principal{}, "", "", fiber.NewError(fiber.StatusBadRequest, "valid Solana walletAddress is required")
	}
	return project, network, walletAddress, nil
}

func (s Server) solanaTransactionRequest(c *fiber.Ctx) (string, string, map[string]any, error) {
	project, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return "", "", nil, fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	var request struct {
		Commitment  string `json:"commitment"`
		Network     string `json:"network"`
		Transaction string `json:"transaction"`
	}
	if err := c.BodyParser(&request); err != nil {
		return "", "", nil, fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	network, err := normalizeSolanaRequestNetwork(request.Network)
	if err != nil {
		return "", "", nil, err
	}
	if err := enforceSolanaProjectPolicy(project.App, network); err != nil {
		return "", "", nil, err
	}
	transaction := strings.TrimSpace(request.Transaction)
	raw, decodeErr := base64.StdEncoding.DecodeString(transaction)
	if decodeErr != nil || len(raw) == 0 || len(raw) > maxSolanaTransactionSize {
		return "", "", nil, fiber.NewError(fiber.StatusBadRequest, "transaction must be a base64-encoded signed Solana transaction")
	}
	commitment := strings.ToLower(strings.TrimSpace(request.Commitment))
	if commitment == "" {
		commitment = "confirmed"
	}
	if commitment != "processed" && commitment != "confirmed" && commitment != "finalized" {
		return "", "", nil, fiber.NewError(fiber.StatusBadRequest, "commitment must be processed, confirmed, or finalized")
	}
	return network, transaction, map[string]any{"commitment": commitment, "encoding": "base64", "preflightCommitment": commitment}, nil
}

func (s Server) solanaTokenAccounts(ctx context.Context, network string, walletAddress string, filter map[string]any) ([]solanaTokenAccount, error) {
	var result struct {
		Value []solanaTokenAccount `json:"value"`
	}
	err := s.solanaRPC(ctx, network, "getTokenAccountsByOwner", []any{
		walletAddress, filter, map[string]any{"commitment": "confirmed", "encoding": "jsonParsed"},
	}, &result)
	return result.Value, err
}

type solanaTokenBalance struct {
	Decimals int64  `json:"decimals"`
	Mint     string `json:"mintAddress"`
	Raw      string `json:"raw"`
	Value    string `json:"value"`
}

func aggregateSolanaTokenAccounts(accounts []solanaTokenAccount) map[string]solanaTokenBalance {
	totals := map[string]*big.Int{}
	decimals := map[string]int64{}
	for _, account := range accounts {
		mint := strings.TrimSpace(account.Account.Data.Parsed.Info.Mint)
		amount := strings.TrimSpace(account.Account.Data.Parsed.Info.TokenAmount.Amount)
		value, ok := new(big.Int).SetString(amount, 10)
		if mint == "" || !ok {
			continue
		}
		if totals[mint] == nil {
			totals[mint] = new(big.Int)
		}
		totals[mint].Add(totals[mint], value)
		decimals[mint] = account.Account.Data.Parsed.Info.TokenAmount.Decimals
	}
	result := make(map[string]solanaTokenBalance, len(totals))
	for mint, total := range totals {
		raw := total.String()
		result[mint] = solanaTokenBalance{Decimals: decimals[mint], Mint: mint, Raw: raw, Value: formatBaseUnitsExact(raw, decimals[mint])}
	}
	return result
}

func (s Server) solanaRPC(ctx context.Context, network string, method string, params []any, result any) error {
	rpcURL, err := s.cfg.SolanaRPCURL(network)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{"id": 1, "jsonrpc": "2.0", "method": method, "params": params})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, rpcURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if err != nil {
		return err
	}
	var envelope struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		if response.StatusCode >= http.StatusBadRequest {
			return fmt.Errorf("Solana RPC returned HTTP %d", response.StatusCode)
		}
		return err
	}
	if envelope.Error != nil {
		return fmt.Errorf("Solana RPC error %d: %s", envelope.Error.Code, envelope.Error.Message)
	}
	if response.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("Solana RPC returned HTTP %d", response.StatusCode)
	}
	if len(envelope.Result) == 0 {
		return errors.New("Solana RPC returned no result")
	}
	return json.Unmarshal(envelope.Result, result)
}

func normalizeSolanaRequestNetwork(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "mainnet", "mainnet-beta":
		return "mainnet", nil
	case "devnet":
		return "devnet", nil
	default:
		return "", fiber.NewError(fiber.StatusBadRequest, "network must be mainnet or devnet")
	}
}

func validSolanaAddress(value string) bool {
	value = strings.TrimSpace(value)
	if !solanaAddressPattern.MatchString(value) {
		return false
	}
	number := new(big.Int)
	for _, character := range value {
		index := strings.IndexRune("123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz", character)
		if index < 0 {
			return false
		}
		number.Mul(number, big.NewInt(58))
		number.Add(number, big.NewInt(int64(index)))
	}
	leadingZeros := 0
	for leadingZeros < len(value) && value[leadingZeros] == '1' {
		leadingZeros++
	}
	return leadingZeros+len(number.Bytes()) == 32
}
