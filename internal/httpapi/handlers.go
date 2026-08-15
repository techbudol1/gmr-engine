package httpapi

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/techbudol1/gmr-engine/internal/store"
	"github.com/techbudol1/gmr-engine/internal/vaultclient"

	"github.com/gofiber/fiber/v2"
)

var evmAddressPattern = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)

func (s Server) adminApps(c *fiber.Ctx) error {
	apps, err := s.store.ListApps(c.Context())
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load apps")
	}
	return c.JSON(fiber.Map{"apps": apps})
}

func (s Server) adminCreateApp(c *fiber.Ctx) error {
	var request store.AppInput
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	app, err := s.store.CreateApp(c.Context(), request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	serverWallet, err := s.ensureProjectDefaultAdminWallet(c, app.ID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"app": app, "serverWallet": serverWallet})
}

func (s Server) adminApp(c *fiber.Ctx) error {
	app, ok, err := s.store.GetApp(c.Context(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load app")
	}
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "app not found")
	}
	keys, err := s.store.ListAPIKeys(c.Context(), app.ID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load api keys")
	}
	usage, err := s.store.ListAPIUsage(c.Context(), app.ID, 50)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load usage")
	}
	transactions, err := s.store.ListTransactions(c.Context(), app.ID, 50)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load transactions")
	}
	serverWallet, err := s.ensureProjectDefaultAdminWallet(c, app.ID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"app": app, "keys": keys, "usage": usage, "transactions": transactions, "serverWallet": serverWallet})
}

func (s Server) adminCreateAPIKey(c *fiber.Ctx) error {
	var request apiKeyRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	app, ok, err := s.store.GetApp(c.Context(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load app")
	}
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "app not found")
	}
	if len(request.Scopes) == 0 {
		request.Scopes = defaultScopes
	}
	keySecret, err := generateAPIKey(app.Environment)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to generate api key")
	}
	key, err := s.store.CreateAPIKey(c.Context(), app.ID, store.APIKeyInput{
		Name:           request.Name,
		Scopes:         request.Scopes,
		AllowedOrigins: request.AllowedOrigins,
		AllowedIPs:     request.AllowedIPs,
		ExpiresAt:      request.ExpiresAt,
	}, keySecret.Prefix, hashAPIKey(keySecret.Plaintext))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.JSON(fiber.Map{"key": key, "secret": keySecret.Secret, "plaintext": keySecret.Plaintext})
}

func (s Server) adminAPIKeys(c *fiber.Ctx) error {
	keys, err := s.store.ListAPIKeys(c.Context(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load api keys")
	}
	return c.JSON(fiber.Map{"keys": keys})
}

func (s Server) adminUsage(c *fiber.Ctx) error {
	usage, err := s.store.ListAPIUsage(c.Context(), c.Params("id"), queryLimit(c))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load usage")
	}
	return c.JSON(fiber.Map{"usage": usage})
}

func (s Server) adminTransactions(c *fiber.Ctx) error {
	transactions, err := s.store.ListTransactions(c.Context(), c.Params("id"), queryLimit(c))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load transactions")
	}
	return c.JSON(fiber.Map{"transactions": transactions})
}

func (s Server) adminRevokeAPIKey(c *fiber.Ctx) error {
	key, err := s.store.RevokeAPIKey(c.Context(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	return c.JSON(fiber.Map{"key": key})
}

func (s Server) adminRotateAPIKey(c *fiber.Ctx) error {
	oldKey, err := s.store.RevokeAPIKey(c.Context(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	app, ok, err := s.store.GetApp(c.Context(), oldKey.AppID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load app")
	}
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "app not found")
	}
	keySecret, err := generateAPIKey(app.Environment)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to generate api key")
	}
	key, err := s.store.CreateAPIKey(c.Context(), app.ID, store.APIKeyInput{
		Name:           oldKey.Name + " rotated",
		Scopes:         oldKey.Scopes,
		AllowedOrigins: oldKey.AllowedOrigins,
		AllowedIPs:     oldKey.AllowedIPs,
		ExpiresAt:      oldKey.ExpiresAt,
	}, keySecret.Prefix, hashAPIKey(keySecret.Plaintext))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.JSON(fiber.Map{"key": key, "secret": keySecret.Secret, "plaintext": keySecret.Plaintext})
}

func (s Server) authMe(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	return c.JSON(fiber.Map{"app": principal.App, "key": principal.Key})
}

func (s Server) updateAppGasFree(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	var request struct {
		GasFreeEnabled bool `json:"gasFreeEnabled"`
	}
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	app, err := s.store.UpdateAppGasFree(c.Context(), principal.App.ID, request.GasFreeEnabled)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.JSON(fiber.Map{"app": app})
}

func (s Server) updateAppTradingFee(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	var request struct {
		TradingFeeBps int64 `json:"tradingFeeBps"`
	}
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if request.TradingFeeBps < 0 || request.TradingFeeBps > 1000 {
		return fiber.NewError(fiber.StatusBadRequest, "tradingFeeBps must be between 0 and 1000")
	}
	app, err := s.store.UpdateAppTradingFee(c.Context(), principal.App.ID, request.TradingFeeBps)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.JSON(fiber.Map{"app": app})
}

func (s Server) wallets(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	wallets, err := s.store.ListProjectWallets(c.Context(), principal.App.ID, queryLimit(c))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load wallets")
	}
	return c.JSON(fiber.Map{"wallets": wallets})
}

func (s Server) createWallet(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	var request struct {
		WalletType string `json:"walletType"`
	}
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	projectWallet, err := s.generateProjectWallet(c, principal.App.ID, request.WalletType, false)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"wallet": projectWallet})
}

func (s Server) userWallets(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	wallets, err := s.store.ListUserWallets(c.Context(), principal.App.ID, queryLimit(c))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load user wallets")
	}
	return c.JSON(fiber.Map{"wallets": wallets})
}

func (s Server) upsertUserWallet(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	var request store.UserWalletInput
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	wallet, err := s.store.UpsertUserWallet(c.Context(), principal.App.ID, request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.JSON(fiber.Map{"wallet": wallet})
}

func (s Server) createManagedUserWallet(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	var request struct {
		AuthProvider string `json:"authProvider"`
		Email        string `json:"email"`
		Metadata     string `json:"metadata"`
		UserID       string `json:"userID"`
	}
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	userID := strings.TrimSpace(request.UserID)
	if userID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "userID is required")
	}
	authProvider := firstNonEmpty(request.AuthProvider, "google_oauth")
	if wallet, ok, err := s.store.GetActiveManagedUserWallet(c.Context(), principal.App.ID, authProvider, userID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load managed user wallet")
	} else if ok {
		return c.JSON(fiber.Map{"wallet": wallet})
	}
	client, err := vaultclient.New(s.cfg.VaultURL, s.cfg.VaultInternalKey, 20*time.Second)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to configure vault client")
	}
	metadata := strings.TrimSpace(request.Metadata)
	generated, err := client.CreateWallet(c.Context(), principal.App.ID, "managed_user", metadata)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "failed to create managed user wallet in vault")
	}
	wallet, err := s.store.UpsertUserWallet(c.Context(), principal.App.ID, store.UserWalletInput{
		Address:        generated.Address,
		AuthProvider:   authProvider,
		Email:          request.Email,
		Metadata:       metadata,
		UserID:         userID,
		VaultWalletRef: vaultclient.Reference(generated.ID),
		WalletCustody:  "managed",
		WalletType:     "managed_user",
	})
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"wallet": wallet})
}

func (s Server) deleteUserWallet(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	wallet, err := s.store.DeleteUserWallet(c.Context(), principal.App.ID, c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	return c.JSON(fiber.Map{"wallet": wallet})
}

func (s Server) nativeTokenBalance(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	chainID, walletAddress, err := balanceQuery(c)
	if err != nil {
		return err
	}
	if err := enforceProjectPolicy(principal.App, chainID, ""); err != nil {
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	}
	balance, err := s.nativeBalance(c.Context(), chainID, walletAddress)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return c.JSON(fiber.Map{
		"balance": fiber.Map{
			"chainId":       balance.ChainID,
			"decimals":      balance.Decimals,
			"raw":           balance.Raw,
			"symbol":        balance.Symbol,
			"value":         formatBaseUnitsExact(balance.Raw, balance.Decimals),
			"walletAddress": strings.ToLower(walletAddress),
		},
	})
}

func balanceQuery(c *fiber.Ctx) (int64, string, error) {
	chainID, err := strconv.ParseInt(strings.TrimSpace(c.Query("chainId")), 10, 64)
	if err != nil || chainID <= 0 {
		return 0, "", fiber.NewError(fiber.StatusBadRequest, "valid chainId is required")
	}
	walletAddress := strings.TrimSpace(c.Query("walletAddress"))
	if !evmAddressPattern.MatchString(walletAddress) {
		return 0, "", fiber.NewError(fiber.StatusBadRequest, "valid walletAddress is required")
	}
	return chainID, walletAddress, nil
}

func (s Server) erc20Balance(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	chainID, walletAddress, err := balanceQuery(c)
	if err != nil {
		return err
	}
	contractAddress := strings.TrimSpace(c.Query("contractAddress"))
	if !evmAddressPattern.MatchString(contractAddress) {
		return fiber.NewError(fiber.StatusBadRequest, "valid contractAddress is required")
	}
	if err := enforceProjectPolicy(principal.App, chainID, contractAddress); err != nil {
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	}
	token, err := s.erc20ConsoleRead(c.Context(), store.ERC20Deployment{
		AppID:           principal.App.ID,
		ChainID:         chainID,
		ContractAddress: contractAddress,
	}, walletAddress)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return c.JSON(fiber.Map{"balance": token})
}

func (s Server) erc20Transfer(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	var request struct {
		Amount          string `json:"amount"`
		ChainID         int64  `json:"chainId"`
		ContractAddress string `json:"contractAddress"`
		Decimals        int64  `json:"decimals"`
		Recipient       string `json:"recipient"`
	}
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
	if !evmAddressPattern.MatchString(strings.TrimSpace(request.Recipient)) {
		return fiber.NewError(fiber.StatusBadRequest, "valid recipient is required")
	}
	if err := enforceProjectPolicy(principal.App, request.ChainID, request.ContractAddress); err != nil {
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	}
	result, err := s.erc20ConsoleWrite(c.Context(), store.ERC20Deployment{
		AppID:           principal.App.ID,
		ChainID:         request.ChainID,
		ContractAddress: strings.TrimSpace(request.ContractAddress),
		Decimals:        request.Decimals,
	}, erc20ActionRequest{
		Action:    "transfer",
		Amount:    request.Amount,
		Recipient: strings.TrimSpace(request.Recipient),
	})
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
		"ok":             true,
		"recipient":      strings.TrimSpace(request.Recipient),
		"amount":         strings.TrimSpace(request.Amount),
		"transactionIds": transactionIDs,
		"transactions":   result.Transactions,
	})
}

func (s Server) enqueueTransaction(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	var request store.TransactionInput
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if request.IdempotencyKey == "" {
		request.IdempotencyKey = c.Get("Idempotency-Key")
	}
	request.AppID = principal.App.ID
	request.KeyID = principal.Key.ID
	if err := enforceProjectPolicy(principal.App, request.ChainID, request.ContractAddress); err != nil {
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	}
	transaction, err := s.store.EnqueueTransaction(c.Context(), request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"transaction": transaction})
}

func (s Server) transaction(c *fiber.Ctx) error {
	transaction, ok, err := s.store.GetTransaction(c.Context(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load transaction")
	}
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "transaction not found")
	}
	principal, _ := c.Locals(principalLocalKey).(principal)
	if transaction.AppID != principal.App.ID && !hasScope(principal.Key.Scopes, "admin") {
		return fiber.NewError(fiber.StatusNotFound, "transaction not found")
	}
	return c.JSON(fiber.Map{"transaction": transaction})
}

func (s Server) contractRead(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	var request contractCallRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if err := validateContractCall(principal.App, request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	result, err := s.runContractRead(c.Context(), request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return c.JSON(fiber.Map{"result": result})
}

func (s Server) contractWrite(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	var request contractCallRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if err := validateContractCall(principal.App, request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	walletAddress := strings.TrimSpace(request.WalletAddress)
	if walletAddress == "" {
		serverWallet, ok, err := s.store.GetProjectDefaultAdminWallet(c.Context(), principal.App.ID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to load project wallet")
		}
		if !ok {
			return fiber.NewError(fiber.StatusNotFound, "project wallet not found")
		}
		walletAddress = serverWallet.Address
	}
	transaction, err := s.store.EnqueueTransaction(c.Context(), store.TransactionInput{
		AppID:           principal.App.ID,
		KeyID:           principal.Key.ID,
		ChainID:         request.ChainID,
		WalletAddress:   walletAddress,
		ContractAddress: strings.TrimSpace(request.ContractAddress),
		Kind:            "contract_write",
		Method:          strings.TrimSpace(request.FunctionName),
		Args:            request.Args,
		Value:           strings.TrimSpace(request.Value),
		Metadata: map[string]any{
			"abi":    json.RawMessage(request.ABI),
			"source": "public_contract_write",
		},
	})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"transaction": transaction})
}

func (s Server) createERC20Deployment(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	var request store.ERC20DeploymentInput
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	request.AppID = principal.App.ID
	request.KeyID = principal.Key.ID
	if err := enforceProjectPolicy(principal.App, request.ChainID, ""); err != nil {
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	}
	if err := s.requireDeploymentGas(c.Context(), principal.App.ID, request.ChainID); err != nil {
		return err
	}
	deployment, err := s.store.CreateERC20Deployment(c.Context(), request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"deployment": deployment})
}

func (s Server) createERC1155EditionDeployment(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	var request store.ERC1155EditionDeploymentInput
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	request.AppID = principal.App.ID
	request.KeyID = principal.Key.ID
	if err := enforceProjectPolicy(principal.App, request.ChainID, ""); err != nil {
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	}
	if err := s.requireDeploymentGas(c.Context(), principal.App.ID, request.ChainID); err != nil {
		return err
	}
	deployment, err := s.store.CreateERC1155EditionDeployment(c.Context(), request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"deployment": deployment})
}

func (s Server) createEscrowDeployment(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	var request store.EscrowDeploymentInput
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	request.AppID = principal.App.ID
	request.KeyID = principal.Key.ID
	if err := enforceProjectPolicy(principal.App, request.ChainID, ""); err != nil {
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	}
	if err := s.requireDeploymentGas(c.Context(), principal.App.ID, request.ChainID); err != nil {
		return err
	}
	deployment, err := s.store.CreateEscrowDeployment(c.Context(), request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"deployment": deployment})
}

func (s Server) createMarketplaceDeployment(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	var request store.MarketplaceDeploymentInput
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	request.AppID = principal.App.ID
	request.KeyID = principal.Key.ID
	if err := enforceProjectPolicy(principal.App, request.ChainID, ""); err != nil {
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	}
	if err := s.requireDeploymentGas(c.Context(), principal.App.ID, request.ChainID); err != nil {
		return err
	}
	deployment, err := s.store.CreateMarketplaceDeployment(c.Context(), request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"deployment": deployment})
}

func (s Server) createPrivateClaimRegistryDeployment(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	var request store.PrivateClaimRegistryDeploymentInput
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	request.AppID = principal.App.ID
	request.KeyID = principal.Key.ID
	if err := enforceProjectPolicy(principal.App, request.ChainID, ""); err != nil {
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	}
	if err := s.requireDeploymentGas(c.Context(), principal.App.ID, request.ChainID); err != nil {
		return err
	}
	deployment, err := s.store.CreatePrivateClaimRegistryDeployment(c.Context(), request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"deployment": deployment})
}

func (s Server) createShieldedPayoutPoolDeployment(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	var request store.ShieldedPayoutPoolDeploymentInput
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	request.AppID = principal.App.ID
	request.KeyID = principal.Key.ID
	if err := enforceProjectPolicy(principal.App, request.ChainID, request.TokenAddress); err != nil {
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	}
	if err := s.requireDeploymentGas(c.Context(), principal.App.ID, request.ChainID); err != nil {
		return err
	}
	deployment, err := s.store.CreateShieldedPayoutPoolDeployment(c.Context(), request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"deployment": deployment})
}

func (s Server) createShieldedWithdrawalVerifierDeployment(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	var request store.ShieldedWithdrawalVerifierDeploymentInput
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	request.AppID = principal.App.ID
	request.KeyID = principal.Key.ID
	if err := enforceProjectPolicy(principal.App, request.ChainID, ""); err != nil {
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	}
	if err := s.requireDeploymentGas(c.Context(), principal.App.ID, request.ChainID); err != nil {
		return err
	}
	deployment, err := s.store.CreateShieldedWithdrawalVerifierDeployment(c.Context(), request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"deployment": deployment})
}

func (s Server) createAccountAbstractionDeployment(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	var request store.AccountAbstractionDeploymentInput
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	request.AppID = principal.App.ID
	request.KeyID = principal.Key.ID
	if err := enforceProjectPolicy(principal.App, request.ChainID, ""); err != nil {
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	}
	if err := s.requireDeploymentGas(c.Context(), principal.App.ID, request.ChainID); err != nil {
		return err
	}
	deployment, err := s.store.CreateAccountAbstractionDeployment(c.Context(), request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"deployment": deployment})
}

func (s Server) acquireWalletLock(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	var request struct {
		WalletAddress string `json:"walletAddress"`
		TransactionID string `json:"transactionId"`
		TTLSeconds    int64  `json:"ttlSeconds"`
	}
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	lock, acquired, err := s.store.AcquireWalletLock(c.Context(), principal.App.ID, request.WalletAddress, request.TransactionID, request.TTLSeconds)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to acquire wallet lock")
	}
	if !acquired {
		return fiber.NewError(fiber.StatusConflict, "wallet is locked")
	}
	return c.JSON(fiber.Map{"lock": lock})
}

func (s Server) releaseWalletLock(c *fiber.Ctx) error {
	principal, ok := c.Locals(principalLocalKey).(principal)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "engine auth required")
	}
	var request struct {
		WalletAddress string `json:"walletAddress"`
		TransactionID string `json:"transactionId"`
	}
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if err := s.store.ReleaseWalletLock(c.Context(), principal.App.ID, request.WalletAddress, request.TransactionID); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to release wallet lock")
	}
	return c.JSON(fiber.Map{"ok": true})
}

func queryLimit(c *fiber.Ctx) int64 {
	limit := int64(100)
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if parsed, err := strconv.ParseInt(raw, 10, 64); err == nil {
			limit = parsed
		}
	}
	return limit
}

func enforceProjectPolicy(app store.App, chainID int64, contractAddress string) error {
	if chainID <= 0 {
		return nil
	}
	if len(app.AllowedChains) > 0 {
		allowed := false
		for _, allowedChain := range app.AllowedChains {
			if allowedChain == chainID {
				allowed = true
				break
			}
		}
		if !allowed {
			return fiber.NewError(fiber.StatusForbidden, "chain is not allowed for this project")
		}
	}
	contractAddress = strings.TrimSpace(contractAddress)
	if contractAddress == "" || len(app.AllowedContracts) == 0 {
		return nil
	}
	for _, allowedContract := range app.AllowedContracts {
		if strings.EqualFold(strings.TrimSpace(allowedContract), contractAddress) {
			return nil
		}
	}
	return fiber.NewError(fiber.StatusForbidden, "contract is not allowed for this project")
}

func enforceSolanaProjectPolicy(app store.App, network string) error {
	network = strings.ToLower(strings.TrimSpace(network))
	if len(app.AllowedSolanaNetworks) == 0 {
		return nil
	}
	for _, allowedNetwork := range app.AllowedSolanaNetworks {
		candidate := strings.ToLower(strings.TrimSpace(allowedNetwork))
		if candidate == "mainnet-beta" {
			candidate = "mainnet"
		}
		if candidate == network {
			return nil
		}
	}
	return fiber.NewError(fiber.StatusForbidden, "Solana network is not allowed for this project")
}
