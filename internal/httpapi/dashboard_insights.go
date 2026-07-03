package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/techbudol1/gmr-engine/internal/store"

	"github.com/gofiber/fiber/v2"
)

type dashboardNotification struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Detail    string `json:"detail"`
	Tone      string `json:"tone"`
	CreatedAt string `json:"createdAt"`
	Link      string `json:"link,omitempty"`
}

type dashboardWalletDetail struct {
	Wallet        any                     `json:"wallet"`
	NativeBalance dashboardBalance        `json:"nativeBalance"`
	TokenBalances []dashboardTokenBalance `json:"tokenBalances"`
	Transfers     []dashboardTransfer     `json:"transfers"`
	Pagination    dashboardPagination     `json:"pagination"`
}

type dashboardBalance struct {
	ChainID  int64  `json:"chainId"`
	Decimals int64  `json:"decimals"`
	Raw      string `json:"raw"`
	Symbol   string `json:"symbol"`
	Value    string `json:"value"`
}

type dashboardTokenBalance struct {
	ContractAddress string `json:"contractAddress"`
	Decimals        int64  `json:"decimals"`
	Name            string `json:"name"`
	Raw             string `json:"raw"`
	Symbol          string `json:"symbol"`
	Value           string `json:"value"`
}

type dashboardTransfer struct {
	BlockNum        string `json:"blockNum"`
	Category        string `json:"category"`
	ContractAddress string `json:"contractAddress"`
	Direction       string `json:"direction"`
	From            string `json:"from"`
	Hash            string `json:"hash"`
	To              string `json:"to"`
	TokenSymbol     string `json:"tokenSymbol"`
	Value           string `json:"value"`
}

type dashboardPagination struct {
	Limit   int  `json:"limit"`
	Page    int  `json:"page"`
	HasMore bool `json:"hasMore"`
}

func (s Server) dashboardTransaction(c *fiber.Ctx) error {
	account, ok := c.Locals(accountLocalKey).(store.Account)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	if _, _, err := s.dashboardAccountApp(c); err != nil {
		return err
	}
	transaction, found, err := s.store.GetAccountTransaction(c.Context(), account.ID, c.Params("transactionId"))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load transaction")
	}
	if !found {
		return fiber.NewError(fiber.StatusNotFound, "transaction not found")
	}
	return c.JSON(fiber.Map{"transaction": transaction})
}

func (s Server) dashboardNotifications(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	transactions, err := s.store.ListTransactions(c.Context(), app.ID, 50)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load transactions")
	}
	wallets, err := s.store.ListProjectWallets(c.Context(), app.ID, 25)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load wallets")
	}
	userWallets, err := s.store.ListUserWallets(c.Context(), app.ID, 25)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load user wallets")
	}
	erc20Deployments, err := s.store.ListERC20Deployments(c.Context(), app.ID, 25)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load token deployments")
	}
	notifications := make([]dashboardNotification, 0, len(transactions)+len(wallets)+len(userWallets)+len(erc20Deployments))
	for _, transaction := range transactions {
		tone := "info"
		if transaction.Status == "confirmed" {
			tone = "success"
		}
		if transaction.Status == "failed" {
			tone = "danger"
		}
		detail := fmt.Sprintf("%s on %s", transaction.Method, shortForNotification(transaction.ContractAddress))
		if transaction.Error != "" {
			detail = transaction.Error
		}
		notifications = append(notifications, dashboardNotification{
			ID:        "tx-" + transaction.ID,
			Kind:      "transaction",
			Title:     "Transaction " + transaction.Status,
			Detail:    detail,
			Tone:      tone,
			CreatedAt: firstNonEmpty(transaction.UpdatedAt, transaction.CreatedAt),
			Link:      fmt.Sprintf("/projects/%s/transactions/%s", app.ID, transaction.ID),
		})
	}
	for _, deployment := range erc20Deployments {
		tone := "info"
		if deployment.Status == "confirmed" {
			tone = "success"
		}
		if deployment.Status == "failed" {
			tone = "danger"
		}
		detail := deployment.Symbol + " / " + deploymentStatusPhrase(deployment.Status)
		if deployment.Error != "" {
			detail = deployment.Error
		}
		notifications = append(notifications, dashboardNotification{
			ID:        "erc20-" + deployment.ID,
			Kind:      "contract",
			Title:     deployment.Name,
			Detail:    detail,
			Tone:      tone,
			CreatedAt: firstNonEmpty(deployment.UpdatedAt, deployment.CreatedAt),
			Link:      fmt.Sprintf("/projects/%s/contracts", app.ID),
		})
	}
	for _, wallet := range wallets {
		notifications = append(notifications, dashboardNotification{
			ID:        "wallet-" + wallet.ID,
			Kind:      "wallet",
			Title:     "Project wallet available",
			Detail:    fmt.Sprintf("%s / %s", wallet.WalletType, shortForNotification(wallet.Address)),
			Tone:      "success",
			CreatedAt: firstNonEmpty(wallet.UpdatedAt, wallet.CreatedAt),
			Link:      fmt.Sprintf("/projects/%s/wallets", app.ID),
		})
	}
	for _, wallet := range userWallets {
		notifications = append(notifications, dashboardNotification{
			ID:        "user-wallet-" + wallet.ID,
			Kind:      "user_wallet",
			Title:     "User wallet registered",
			Detail:    fmt.Sprintf("%s / %s", firstNonEmpty(wallet.Email, wallet.AuthProvider, "user"), shortForNotification(wallet.Address)),
			Tone:      "info",
			CreatedAt: firstNonEmpty(wallet.LastSeenAt, wallet.UpdatedAt, wallet.CreatedAt),
			Link:      fmt.Sprintf("/projects/%s/wallets", app.ID),
		})
	}
	sort.SliceStable(notifications, func(i, j int) bool {
		return notifications[i].CreatedAt > notifications[j].CreatedAt
	})
	limit := queryLimit(c)
	if int64(len(notifications)) > limit {
		notifications = notifications[:limit]
	}
	return c.JSON(fiber.Map{"notifications": notifications})
}

func (s Server) dashboardUserWalletDetail(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	wallets, err := s.store.ListUserWallets(c.Context(), app.ID, 500)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load user wallets")
	}
	for _, wallet := range wallets {
		if wallet.ID == c.Params("walletId") {
			detail, err := s.walletDetail(c.Context(), app, wallet, wallet.Address, true, pageValue(c), limitValue(c, 5, 25))
			if err != nil {
				return fiber.NewError(fiber.StatusBadGateway, err.Error())
			}
			return c.JSON(fiber.Map{"detail": detail})
		}
	}
	return fiber.NewError(fiber.StatusNotFound, "user wallet not found")
}

func (s Server) dashboardProjectWalletDetail(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	wallets, err := s.store.ListProjectWallets(c.Context(), app.ID, 500)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load project wallets")
	}
	for _, wallet := range wallets {
		if wallet.ID == c.Params("walletId") {
			detail, err := s.walletDetail(c.Context(), app, wallet, wallet.Address, false, pageValue(c), limitValue(c, 5, 25))
			if err != nil {
				return fiber.NewError(fiber.StatusBadGateway, err.Error())
			}
			return c.JSON(fiber.Map{"detail": detail})
		}
	}
	return fiber.NewError(fiber.StatusNotFound, "project wallet not found")
}

func (s Server) walletDetail(ctx context.Context, app store.App, wallet any, walletAddress string, includeTransfers bool, page int, limit int) (dashboardWalletDetail, error) {
	chainID := int64(421614)
	if len(app.AllowedChains) > 0 {
		chainID = app.AllowedChains[0]
	}
	nativeBalance, err := s.nativeBalance(ctx, chainID, walletAddress)
	if err != nil {
		return dashboardWalletDetail{}, err
	}
	deployments, err := s.store.ListERC20Deployments(ctx, app.ID, 20)
	if err != nil {
		return dashboardWalletDetail{}, err
	}
	tokenBalances := []dashboardTokenBalance{}
	contractAddresses := []string{}
	for _, deployment := range deployments {
		if deployment.Status != "confirmed" || strings.TrimSpace(deployment.ContractAddress) == "" {
			continue
		}
		token, err := s.erc20ConsoleRead(ctx, deployment, walletAddress)
		if err != nil {
			continue
		}
		tokenBalances = append(tokenBalances, dashboardTokenBalance{
			ContractAddress: deployment.ContractAddress,
			Decimals:        token.Decimals,
			Name:            token.Name,
			Raw:             token.OwnedBalanceRaw,
			Symbol:          token.Symbol,
			Value:           token.OwnedBalance,
		})
		contractAddresses = append(contractAddresses, deployment.ContractAddress)
	}
	transfers := []dashboardTransfer{}
	hasMore := false
	if includeTransfers && len(contractAddresses) > 0 {
		transfers, hasMore, _ = s.walletERC20Transfers(ctx, walletAddress, contractAddresses, page, limit)
	}
	return dashboardWalletDetail{
		Wallet:        wallet,
		NativeBalance: nativeBalance,
		TokenBalances: tokenBalances,
		Transfers:     transfers,
		Pagination:    dashboardPagination{Limit: limit, Page: page, HasMore: hasMore},
	}, nil
}

func (s Server) nativeBalance(ctx context.Context, chainID int64, walletAddress string) (dashboardBalance, error) {
	var result string
	if err := s.rpc(ctx, "eth_getBalance", []any{walletAddress, "latest"}, &result); err != nil {
		return dashboardBalance{}, err
	}
	value, ok := new(big.Int).SetString(strings.TrimPrefix(result, "0x"), 16)
	if !ok {
		value = big.NewInt(0)
	}
	return dashboardBalance{
		ChainID:  chainID,
		Decimals: 18,
		Raw:      value.String(),
		Symbol:   nativeSymbol(chainID),
		Value:    formatBaseUnits(value.String(), 18),
	}, nil
}

func (s Server) walletERC20Transfers(ctx context.Context, walletAddress string, contractAddresses []string, page int, limit int) ([]dashboardTransfer, bool, error) {
	fetchLimit := page*limit + 1
	if fetchLimit < limit+1 {
		fetchLimit = limit + 1
	}
	inbound, _ := s.assetTransfers(ctx, map[string]any{
		"category":          []string{"erc20"},
		"contractAddresses": contractAddresses,
		"fromBlock":         "0x0",
		"maxCount":          "0x" + strconv.FormatInt(int64(fetchLimit), 16),
		"order":             "desc",
		"toAddress":         walletAddress,
		"withMetadata":      false,
	})
	outbound, _ := s.assetTransfers(ctx, map[string]any{
		"category":          []string{"erc20"},
		"contractAddresses": contractAddresses,
		"fromAddress":       walletAddress,
		"fromBlock":         "0x0",
		"maxCount":          "0x" + strconv.FormatInt(int64(fetchLimit), 16),
		"order":             "desc",
		"withMetadata":      false,
	})
	combined := append(inbound, outbound...)
	sort.SliceStable(combined, func(i, j int) bool {
		return combined[i].BlockNum > combined[j].BlockNum
	})
	start := (page - 1) * limit
	if start >= len(combined) {
		return []dashboardTransfer{}, false, nil
	}
	end := start + limit
	hasMore := len(combined) > end
	if end > len(combined) {
		end = len(combined)
	}
	selected := combined[start:end]
	for index := range selected {
		if strings.EqualFold(selected[index].To, walletAddress) {
			selected[index].Direction = "received"
		} else if strings.EqualFold(selected[index].From, walletAddress) {
			selected[index].Direction = "sent"
		}
	}
	return selected, hasMore, nil
}

func (s Server) assetTransfers(ctx context.Context, params map[string]any) ([]dashboardTransfer, error) {
	var response struct {
		Transfers []struct {
			BlockNum    string `json:"blockNum"`
			Category    string `json:"category"`
			RawContract struct {
				Address string `json:"address"`
			} `json:"rawContract"`
			ContractAddress string `json:"contractAddress"`
			From            string `json:"from"`
			Hash            string `json:"hash"`
			To              string `json:"to"`
			TokenSymbol     string `json:"asset"`
			Value           any    `json:"value"`
		} `json:"transfers"`
	}
	if err := s.rpc(ctx, "alchemy_getAssetTransfers", []any{params}, &response); err != nil {
		return nil, err
	}
	transfers := make([]dashboardTransfer, 0, len(response.Transfers))
	for _, transfer := range response.Transfers {
		contractAddress := transfer.ContractAddress
		if contractAddress == "" {
			contractAddress = transfer.RawContract.Address
		}
		transfers = append(transfers, dashboardTransfer{
			BlockNum:        transfer.BlockNum,
			Category:        transfer.Category,
			ContractAddress: contractAddress,
			From:            transfer.From,
			Hash:            transfer.Hash,
			To:              transfer.To,
			TokenSymbol:     transfer.TokenSymbol,
			Value:           fmt.Sprint(transfer.Value),
		})
	}
	return transfers, nil
}

func (s Server) rpc(ctx context.Context, method string, params []any, result any) error {
	payload, err := json.Marshal(map[string]any{
		"id":      1,
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.AlchemyRPCURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	var envelope struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return err
	}
	if envelope.Error != nil {
		return errors.New(envelope.Error.Message)
	}
	if len(envelope.Result) == 0 {
		return nil
	}
	return json.Unmarshal(envelope.Result, result)
}

func pageValue(c *fiber.Ctx) int {
	value, err := strconv.Atoi(c.Query("page", "1"))
	if err != nil || value <= 0 {
		return 1
	}
	return value
}

func limitValue(c *fiber.Ctx, fallback int, max int) int {
	value, err := strconv.Atoi(c.Query("limit", strconv.Itoa(fallback)))
	if err != nil || value <= 0 {
		return fallback
	}
	if value > max {
		return max
	}
	return value
}

func nativeSymbol(chainID int64) string {
	switch chainID {
	case 42161, 421614, 1, 11155111:
		return "ETH"
	default:
		return "Native"
	}
}

func formatBaseUnits(raw string, decimals int64) string {
	value, ok := new(big.Int).SetString(strings.TrimSpace(raw), 10)
	if !ok {
		return "0"
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(decimals), nil)
	whole := new(big.Int).Div(value, scale).String()
	fraction := new(big.Int).Mod(value, scale).String()
	if fraction == "0" {
		return whole
	}
	fraction = strings.Repeat("0", int(decimals)-len(fraction)) + fraction
	fraction = strings.TrimRight(fraction, "0")
	if len(fraction) > 6 {
		fraction = fraction[:6]
	}
	return whole + "." + fraction
}

func shortForNotification(value string) string {
	if len(value) <= 14 {
		return value
	}
	return value[:6] + "..." + value[len(value)-4:]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func deploymentStatusPhrase(status string) string {
	switch status {
	case "confirmed":
		return "deployed"
	case "submitted":
		return "submitted on-chain"
	case "failed":
		return "failed"
	case "deploying":
		return "deploying"
	default:
		return "queued"
	}
}
