package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"os/exec"
	"strings"
	"time"

	"github.com/techbudol1/gmr-engine/internal/store"

	"github.com/gofiber/fiber/v2"
)

type dashboardLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Username string `json:"username"`
}

func (s Server) requireDeploymentGas(ctx context.Context, appID string, chainID int64) error {
	wallet, ok, err := s.store.GetProjectDefaultAdminWalletSecret(ctx, appID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load project admin wallet")
	}
	if !ok || strings.TrimSpace(wallet.Address) == "" {
		return fiber.NewError(fiber.StatusConflict, "project admin wallet is not configured")
	}
	balance, err := s.nativeBalance(ctx, chainID, wallet.Address)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "failed to check project admin wallet gas balance: "+err.Error())
	}
	raw, ok := new(big.Int).SetString(strings.TrimSpace(balance.Raw), 10)
	if !ok || raw.Sign() <= 0 {
		return fiber.NewError(fiber.StatusConflict, "project admin wallet has no native gas on the selected chain")
	}
	return nil
}

func (s Server) dashboardLogin(c *fiber.Ctx) error {
	var request dashboardLoginRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	login := request.Username
	if strings.TrimSpace(login) == "" {
		login = request.Email
	}
	account, ok, err := s.store.VerifyAccountPassword(c.Context(), login, HashPassword(request.Password))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to verify account")
	}
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid username or password")
	}
	token, err := randomURLToken(32)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to issue session")
	}
	expiresAt := time.Now().UTC().Add(s.cfg.SessionTTL)
	if _, err := s.store.CreateAccountSession(c.Context(), account.ID, hashToken(token), expiresAt.Format(time.RFC3339)); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to save session")
	}
	sameSite, secure := dashboardCookieSettings(c, s.cfg.IsProduction())
	c.Cookie(&fiber.Cookie{
		Name:     s.cfg.DashboardCookie,
		Value:    token,
		Expires:  expiresAt,
		HTTPOnly: true,
		SameSite: sameSite,
		Secure:   secure,
		Path:     "/",
	})
	return c.JSON(fiber.Map{"account": account})
}

func (s Server) dashboardMe(c *fiber.Ctx) error {
	account, ok := c.Locals(accountLocalKey).(store.Account)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	return c.JSON(fiber.Map{"account": account})
}

func (s Server) dashboardLogout(c *fiber.Ctx) error {
	token := strings.TrimSpace(c.Cookies(s.cfg.DashboardCookie))
	if token != "" {
		_ = s.store.DeleteAccountSession(c.Context(), hashToken(token))
	}
	sameSite, secure := dashboardCookieSettings(c, s.cfg.IsProduction())
	c.Cookie(&fiber.Cookie{
		Name:     s.cfg.DashboardCookie,
		Value:    "",
		Expires:  time.Now().UTC().Add(-time.Hour),
		HTTPOnly: true,
		SameSite: sameSite,
		Secure:   secure,
		Path:     "/",
	})
	return c.JSON(fiber.Map{"ok": true})
}

func dashboardCookieSettings(c *fiber.Ctx, production bool) (string, bool) {
	secure := production ||
		strings.EqualFold(c.Protocol(), "https") ||
		strings.EqualFold(c.Get("X-Forwarded-Proto"), "https") ||
		strings.Contains(strings.ToLower(c.Get("Cf-Visitor")), `"scheme":"https"`)
	if secure {
		return fiber.CookieSameSiteNoneMode, true
	}
	return fiber.CookieSameSiteLaxMode, false
}

func (s Server) dashboardProjects(c *fiber.Ctx) error {
	account, ok := c.Locals(accountLocalKey).(store.Account)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	apps, err := s.store.ListAccountApps(c.Context(), account.ID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load projects")
	}
	return c.JSON(fiber.Map{"projects": apps})
}

func (s Server) dashboardCreateProject(c *fiber.Ctx) error {
	account, ok := c.Locals(accountLocalKey).(store.Account)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	var request store.AppInput
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	app, err := s.store.CreateAccountApp(c.Context(), account.ID, request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	serverWallet, err := s.ensureProjectDefaultAdminWallet(c, app.ID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"project": app, "serverWallet": serverWallet})
}

func (s Server) dashboardArchiveProject(c *fiber.Ctx) error {
	account, ok := c.Locals(accountLocalKey).(store.Account)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	app, err := s.store.ArchiveAccountApp(c.Context(), account.ID, c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	return c.JSON(fiber.Map{"project": app})
}

func (s Server) dashboardUpdateProject(c *fiber.Ctx) error {
	account, ok := c.Locals(accountLocalKey).(store.Account)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	var request store.AppInput
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	app, err := s.store.UpdateAccountApp(c.Context(), account.ID, c.Params("id"), request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.JSON(fiber.Map{"project": app})
}

func (s Server) dashboardRetryContractDeployment(c *fiber.Ctx) error {
	account, ok := c.Locals(accountLocalKey).(store.Account)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	if err := s.store.RetryContractDeployment(c.Context(), account.ID, c.Params("deploymentType"), c.Params("id")); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.JSON(fiber.Map{"ok": true})
}

func (s Server) dashboardProject(c *fiber.Ctx) error {
	account, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
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
	wallets, err := s.store.ListProjectWallets(c.Context(), app.ID, 100)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load project wallets")
	}
	userWallets, err := s.store.ListUserWallets(c.Context(), app.ID, 100)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load user wallets")
	}
	deployments, err := s.store.ListERC20Deployments(c.Context(), app.ID, 50)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load token deployments")
	}
	editionDeployments, err := s.store.ListERC1155EditionDeployments(c.Context(), app.ID, 50)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load edition deployments")
	}
	escrowDeployments, err := s.store.ListEscrowDeployments(c.Context(), app.ID, 50)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load escrow deployments")
	}
	marketplaceDeployments, err := s.store.ListMarketplaceDeployments(c.Context(), app.ID, 50)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load marketplace deployments")
	}
	privateClaimRegistryDeployments, err := s.store.ListPrivateClaimRegistryDeployments(c.Context(), app.ID, 50)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load private claim registry deployments")
	}
	shieldedPayoutPoolDeployments, err := s.store.ListShieldedPayoutPoolDeployments(c.Context(), app.ID, 50)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load shielded payout pool deployments")
	}
	privacyAccessPassDeployments, err := s.store.ListPrivacyAccessPassDeployments(c.Context(), app.ID, 50)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load privacy access pass deployments")
	}
	shieldedWithdrawalVerifierDeployments, err := s.store.ListShieldedWithdrawalVerifierDeployments(c.Context(), app.ID, 50)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load shielded withdrawal verifier deployments")
	}
	accountAbstractionDeployments, err := s.store.ListAccountAbstractionDeployments(c.Context(), app.ID, 50)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load account abstraction deployments")
	}
	importedContracts, err := s.store.ListImportedContracts(c.Context(), app.ID, 50)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load imported contracts")
	}
	serverWallet, err := s.ensureProjectDefaultAdminWallet(c, app.ID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"account": account, "project": app, "keys": keys, "usage": usage, "transactions": transactions, "wallets": wallets, "userWallets": userWallets, "erc20Deployments": deployments, "erc1155EditionDeployments": editionDeployments, "escrowDeployments": escrowDeployments, "marketplaceDeployments": marketplaceDeployments, "privateClaimRegistryDeployments": privateClaimRegistryDeployments, "shieldedPayoutPoolDeployments": shieldedPayoutPoolDeployments, "privacyAccessPassDeployments": privacyAccessPassDeployments, "shieldedWithdrawalVerifierDeployments": shieldedWithdrawalVerifierDeployments, "accountAbstractionDeployments": accountAbstractionDeployments, "importedContracts": importedContracts, "serverWallet": serverWallet})
}

func (s Server) dashboardCreateAPIKey(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	var request apiKeyRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
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

func (s Server) dashboardAPIKeys(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	keys, err := s.store.ListAPIKeys(c.Context(), app.ID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load api keys")
	}
	return c.JSON(fiber.Map{"keys": keys})
}

func (s Server) dashboardUsage(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	usage, err := s.store.ListAPIUsage(c.Context(), app.ID, queryLimit(c))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load usage")
	}
	return c.JSON(fiber.Map{"usage": usage})
}

func (s Server) dashboardTransactions(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	transactions, err := s.store.ListTransactions(c.Context(), app.ID, queryLimit(c))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load transactions")
	}
	return c.JSON(fiber.Map{"transactions": transactions})
}

func (s Server) dashboardProjectWallets(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	wallets, err := s.store.ListProjectWallets(c.Context(), app.ID, queryLimit(c))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load project wallets")
	}
	return c.JSON(fiber.Map{"wallets": wallets})
}

func (s Server) dashboardCreateProjectWallet(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	var request struct {
		WalletType string `json:"walletType"`
	}
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	projectWallet, err := s.generateProjectWallet(c, app.ID, request.WalletType, false)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"wallet": projectWallet})
}

func (s Server) dashboardUserWallets(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	wallets, err := s.store.ListUserWallets(c.Context(), app.ID, queryLimit(c))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load user wallets")
	}
	return c.JSON(fiber.Map{"wallets": wallets})
}

func (s Server) dashboardDeleteUserWallet(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	wallet, err := s.store.DeleteUserWallet(c.Context(), app.ID, c.Params("walletId"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	return c.JSON(fiber.Map{"wallet": wallet})
}

func (s Server) dashboardERC20Deployments(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	deployments, err := s.store.ListERC20Deployments(c.Context(), app.ID, queryLimit(c))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load token deployments")
	}
	return c.JSON(fiber.Map{"erc20Deployments": deployments})
}

func (s Server) dashboardCreateERC20Deployment(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	var request store.ERC20DeploymentInput
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	request.AppID = app.ID
	if err := enforceProjectPolicy(app, request.ChainID, ""); err != nil {
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	}
	if err := s.requireDeploymentGas(c.Context(), app.ID, request.ChainID); err != nil {
		return err
	}
	deployment, err := s.store.CreateERC20Deployment(c.Context(), request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"deployment": deployment})
}

func (s Server) dashboardDeleteERC20Deployment(c *fiber.Ctx) error {
	account, ok := c.Locals(accountLocalKey).(store.Account)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	deployment, err := s.store.RemoveERC20DeploymentFromDashboard(c.Context(), account.ID, c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	return c.JSON(fiber.Map{"deployment": deployment})
}

func (s Server) dashboardEscrowDeployments(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	deployments, err := s.store.ListEscrowDeployments(c.Context(), app.ID, queryLimit(c))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load escrow deployments")
	}
	return c.JSON(fiber.Map{"escrowDeployments": deployments})
}

func (s Server) dashboardCreateEscrowDeployment(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	var request store.EscrowDeploymentInput
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	request.AppID = app.ID
	if err := enforceProjectPolicy(app, request.ChainID, ""); err != nil {
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	}
	if err := s.requireDeploymentGas(c.Context(), app.ID, request.ChainID); err != nil {
		return err
	}
	deployment, err := s.store.CreateEscrowDeployment(c.Context(), request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"deployment": deployment})
}

func (s Server) dashboardDeleteEscrowDeployment(c *fiber.Ctx) error {
	account, ok := c.Locals(accountLocalKey).(store.Account)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	deployment, err := s.store.RemoveEscrowDeploymentFromDashboard(c.Context(), account.ID, c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	return c.JSON(fiber.Map{"deployment": deployment})
}

func (s Server) dashboardMarketplaceDeployments(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	deployments, err := s.store.ListMarketplaceDeployments(c.Context(), app.ID, queryLimit(c))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load marketplace deployments")
	}
	return c.JSON(fiber.Map{"marketplaceDeployments": deployments})
}

func (s Server) dashboardCreateMarketplaceDeployment(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	var request store.MarketplaceDeploymentInput
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	request.AppID = app.ID
	if err := enforceProjectPolicy(app, request.ChainID, ""); err != nil {
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	}
	if err := s.requireDeploymentGas(c.Context(), app.ID, request.ChainID); err != nil {
		return err
	}
	deployment, err := s.store.CreateMarketplaceDeployment(c.Context(), request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"deployment": deployment})
}

func (s Server) dashboardDeleteMarketplaceDeployment(c *fiber.Ctx) error {
	account, ok := c.Locals(accountLocalKey).(store.Account)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	deployment, err := s.store.RemoveMarketplaceDeploymentFromDashboard(c.Context(), account.ID, c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	return c.JSON(fiber.Map{"deployment": deployment})
}

func (s Server) dashboardPrivateClaimRegistryDeployments(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	deployments, err := s.store.ListPrivateClaimRegistryDeployments(c.Context(), app.ID, queryLimit(c))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load private claim registry deployments")
	}
	return c.JSON(fiber.Map{"privateClaimRegistryDeployments": deployments})
}

func (s Server) dashboardCreatePrivateClaimRegistryDeployment(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	var request store.PrivateClaimRegistryDeploymentInput
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	request.AppID = app.ID
	if err := enforceProjectPolicy(app, request.ChainID, ""); err != nil {
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	}
	if err := s.requireDeploymentGas(c.Context(), app.ID, request.ChainID); err != nil {
		return err
	}
	deployment, err := s.store.CreatePrivateClaimRegistryDeployment(c.Context(), request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"deployment": deployment})
}

func (s Server) dashboardDeletePrivateClaimRegistryDeployment(c *fiber.Ctx) error {
	account, ok := c.Locals(accountLocalKey).(store.Account)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	deployment, err := s.store.RemovePrivateClaimRegistryDeploymentFromDashboard(c.Context(), account.ID, c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	return c.JSON(fiber.Map{"deployment": deployment})
}

func (s Server) dashboardShieldedPayoutPoolDeployments(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	deployments, err := s.store.ListShieldedPayoutPoolDeployments(c.Context(), app.ID, queryLimit(c))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load shielded payout pool deployments")
	}
	return c.JSON(fiber.Map{"shieldedPayoutPoolDeployments": deployments})
}

func (s Server) dashboardCreateShieldedPayoutPoolDeployment(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	var request store.ShieldedPayoutPoolDeploymentInput
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	request.AppID = app.ID
	if err := enforceProjectPolicy(app, request.ChainID, request.TokenAddress); err != nil {
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	}
	if err := s.requireDeploymentGas(c.Context(), app.ID, request.ChainID); err != nil {
		return err
	}
	deployment, err := s.store.CreateShieldedPayoutPoolDeployment(c.Context(), request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"deployment": deployment})
}

func (s Server) dashboardDeleteShieldedPayoutPoolDeployment(c *fiber.Ctx) error {
	account, ok := c.Locals(accountLocalKey).(store.Account)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	deployment, err := s.store.RemoveShieldedPayoutPoolDeploymentFromDashboard(c.Context(), account.ID, c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.JSON(fiber.Map{"deployment": deployment})
}

func (s Server) dashboardPrivacyAccessPassDeployments(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	deployments, err := s.store.ListPrivacyAccessPassDeployments(c.Context(), app.ID, queryLimit(c))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load privacy access pass deployments")
	}
	return c.JSON(fiber.Map{"privacyAccessPassDeployments": deployments})
}

func (s Server) dashboardCreatePrivacyAccessPassDeployment(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	var request store.PrivacyAccessPassDeploymentInput
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	request.AppID = app.ID
	if err := enforceProjectPolicy(app, request.ChainID, request.TokenAddress); err != nil {
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	}
	if err := s.requireDeploymentGas(c.Context(), app.ID, request.ChainID); err != nil {
		return err
	}
	deployment, err := s.store.CreatePrivacyAccessPassDeployment(c.Context(), request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"deployment": deployment})
}

func (s Server) dashboardDeletePrivacyAccessPassDeployment(c *fiber.Ctx) error {
	account, ok := c.Locals(accountLocalKey).(store.Account)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	deployment, err := s.store.RemovePrivacyAccessPassDeploymentFromDashboard(c.Context(), account.ID, c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.JSON(fiber.Map{"deployment": deployment})
}

func (s Server) dashboardShieldedWithdrawalVerifierDeployments(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	deployments, err := s.store.ListShieldedWithdrawalVerifierDeployments(c.Context(), app.ID, queryLimit(c))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load shielded withdrawal verifier deployments")
	}
	return c.JSON(fiber.Map{"shieldedWithdrawalVerifierDeployments": deployments})
}

func (s Server) dashboardCreateShieldedWithdrawalVerifierDeployment(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	var request store.ShieldedWithdrawalVerifierDeploymentInput
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	request.AppID = app.ID
	if err := enforceProjectPolicy(app, request.ChainID, ""); err != nil {
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	}
	if err := s.requireDeploymentGas(c.Context(), app.ID, request.ChainID); err != nil {
		return err
	}
	deployment, err := s.store.CreateShieldedWithdrawalVerifierDeployment(c.Context(), request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"deployment": deployment})
}

func (s Server) dashboardDeleteShieldedWithdrawalVerifierDeployment(c *fiber.Ctx) error {
	account, ok := c.Locals(accountLocalKey).(store.Account)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	deployment, err := s.store.RemoveShieldedWithdrawalVerifierDeploymentFromDashboard(c.Context(), account.ID, c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.JSON(fiber.Map{"deployment": deployment})
}

func (s Server) dashboardAccountAbstractionDeployments(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	deployments, err := s.store.ListAccountAbstractionDeployments(c.Context(), app.ID, queryLimit(c))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load account abstraction deployments")
	}
	return c.JSON(fiber.Map{"accountAbstractionDeployments": deployments})
}

func (s Server) dashboardCreateAccountAbstractionDeployment(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	var request store.AccountAbstractionDeploymentInput
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	request.AppID = app.ID
	if err := enforceProjectPolicy(app, request.ChainID, ""); err != nil {
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	}
	if err := s.requireDeploymentGas(c.Context(), app.ID, request.ChainID); err != nil {
		return err
	}
	deployment, err := s.store.CreateAccountAbstractionDeployment(c.Context(), request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"deployment": deployment})
}

func (s Server) dashboardDeleteAccountAbstractionDeployment(c *fiber.Ctx) error {
	account, ok := c.Locals(accountLocalKey).(store.Account)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	deployment, err := s.store.RemoveAccountAbstractionDeploymentFromDashboard(c.Context(), account.ID, c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.JSON(fiber.Map{"deployment": deployment})
}

func (s Server) dashboardERC20Console(c *fiber.Ctx) error {
	account, ok := c.Locals(accountLocalKey).(store.Account)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	deployment, ok, err := s.store.GetAccountERC20Deployment(c.Context(), account.ID, c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load token")
	}
	if !ok || deployment.Status != "confirmed" || strings.TrimSpace(deployment.ContractAddress) == "" {
		return fiber.NewError(fiber.StatusNotFound, "deployed token not found")
	}
	serverWallet, ok, err := s.store.GetProjectDefaultAdminWallet(c.Context(), deployment.AppID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load project wallet")
	}
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "project wallet not found")
	}
	token, err := s.erc20ConsoleRead(c.Context(), deployment, serverWallet.Address)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	token.SupportsBurn = erc20DeploymentSupportsBurn(deployment)
	writeFunctions := []string{"mint", "transfer", "airdrop"}
	if token.SupportsBurn {
		writeFunctions = append(writeFunctions, "burn")
	}
	return c.JSON(fiber.Map{
		"token":          token,
		"readFunctions":  []string{"name", "symbol", "decimals", "totalSupply", "balanceOf"},
		"writeFunctions": writeFunctions,
		"events":         []string{"Transfer", "Approval", "OwnershipTransferred"},
	})
}

func (s Server) dashboardERC20Action(c *fiber.Ctx) error {
	account, ok := c.Locals(accountLocalKey).(store.Account)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	deployment, ok, err := s.store.GetAccountERC20Deployment(c.Context(), account.ID, c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load token")
	}
	if !ok || deployment.Status != "confirmed" || strings.TrimSpace(deployment.ContractAddress) == "" {
		return fiber.NewError(fiber.StatusNotFound, "deployed token not found")
	}
	var request erc20ActionRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if err := s.verifyDashboardPassword(c, account, request.ConfirmPassword); err != nil {
		return err
	}
	result, err := s.enqueueERC20ConsoleWrite(c.Context(), deployment, request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"result": result})
}

func (s Server) dashboardContractRead(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	var request contractCallRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if err := validateContractCall(app, request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	result, err := s.runContractRead(c.Context(), request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return c.JSON(fiber.Map{"result": result})
}

func (s Server) dashboardContractWrite(c *fiber.Ctx) error {
	account, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	var request contractCallRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if err := validateContractCall(app, request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if err := s.verifyDashboardPassword(c, account, request.ConfirmPassword); err != nil {
		return err
	}
	serverWallet, ok, err := s.store.GetProjectDefaultAdminWallet(c.Context(), app.ID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load project wallet")
	}
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "project wallet not found")
	}
	transaction, err := s.store.EnqueueTransaction(c.Context(), store.TransactionInput{
		AppID:           app.ID,
		ChainID:         request.ChainID,
		WalletAddress:   serverWallet.Address,
		ContractAddress: request.ContractAddress,
		Kind:            "contract_write",
		Method:          request.FunctionName,
		Args:            request.Args,
		Value:           strings.TrimSpace(request.Value),
		Metadata: map[string]any{
			"abi":    json.RawMessage(request.ABI),
			"source": "dashboard_generic_console",
		},
	})
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"transaction": transaction})
}

func (s Server) verifyDashboardPassword(c *fiber.Ctx, account store.Account, password string) error {
	if strings.TrimSpace(password) == "" {
		return fiber.NewError(fiber.StatusUnauthorized, "confirm password is required")
	}
	login := account.Username
	if strings.TrimSpace(login) == "" {
		login = account.Email
	}
	_, verified, err := s.store.VerifyAccountPassword(c.Context(), login, HashPassword(password))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to verify password")
	}
	if !verified {
		return fiber.NewError(fiber.StatusUnauthorized, "password confirmation failed")
	}
	return nil
}

func validateContractCall(app store.App, request contractCallRequest) error {
	if request.ChainID <= 0 {
		return errors.New("valid chainId is required")
	}
	if len(app.AllowedChains) > 0 {
		allowed := false
		for _, chainID := range app.AllowedChains {
			if chainID == request.ChainID {
				allowed = true
				break
			}
		}
		if !allowed {
			return errors.New("chain is not allowed for this project")
		}
	}
	if strings.TrimSpace(request.ContractAddress) == "" {
		return errors.New("contractAddress is required")
	}
	if len(app.AllowedContracts) > 0 {
		allowed := false
		for _, address := range app.AllowedContracts {
			if strings.EqualFold(address, request.ContractAddress) {
				allowed = true
				break
			}
		}
		if !allowed {
			return errors.New("contract is not allowed for this project")
		}
	}
	if strings.TrimSpace(request.FunctionName) == "" {
		return errors.New("functionName is required")
	}
	if len(request.ABI) == 0 || string(request.ABI) == "null" {
		return errors.New("abi is required")
	}
	return nil
}

func (s Server) dashboardERC1155EditionDeployments(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	deployments, err := s.store.ListERC1155EditionDeployments(c.Context(), app.ID, queryLimit(c))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load edition deployments")
	}
	return c.JSON(fiber.Map{"erc1155EditionDeployments": deployments})
}

func (s Server) dashboardCreateERC1155EditionDeployment(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	var request store.ERC1155EditionDeploymentInput
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	request.AppID = app.ID
	if err := enforceProjectPolicy(app, request.ChainID, ""); err != nil {
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	}
	if err := s.requireDeploymentGas(c.Context(), app.ID, request.ChainID); err != nil {
		return err
	}
	deployment, err := s.store.CreateERC1155EditionDeployment(c.Context(), request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"deployment": deployment})
}

func (s Server) dashboardImportedContracts(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	contracts, err := s.store.ListImportedContracts(c.Context(), app.ID, queryLimit(c))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load imported contracts")
	}
	return c.JSON(fiber.Map{"contracts": contracts})
}

func (s Server) dashboardImportContract(c *fiber.Ctx) error {
	_, app, err := s.dashboardAccountApp(c)
	if err != nil {
		return err
	}
	var request store.ImportedContractInput
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if request.ChainID <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "valid chainId is required")
	}
	if !evmAddressPattern.MatchString(strings.TrimSpace(request.ContractAddress)) {
		return fiber.NewError(fiber.StatusBadRequest, "valid contractAddress is required")
	}
	var abi []any
	if err := json.Unmarshal([]byte(request.ABI), &abi); err != nil || len(abi) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "valid JSON ABI is required")
	}
	contract, err := s.store.CreateImportedContract(c.Context(), app.ID, request)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"contract": contract})
}

func (s Server) dashboardDeleteImportedContract(c *fiber.Ctx) error {
	account, ok := c.Locals(accountLocalKey).(store.Account)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	contract, err := s.store.DeleteImportedContract(c.Context(), account.ID, c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	return c.JSON(fiber.Map{"contract": contract})
}

func (s Server) dashboardDeleteERC1155EditionDeployment(c *fiber.Ctx) error {
	account, ok := c.Locals(accountLocalKey).(store.Account)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	deployment, err := s.store.RemoveERC1155EditionDeploymentFromDashboard(c.Context(), account.ID, c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	return c.JSON(fiber.Map{"deployment": deployment})
}

func (s Server) dashboardRevokeAPIKey(c *fiber.Ctx) error {
	account, ok := c.Locals(accountLocalKey).(store.Account)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	if _, ok, err := s.store.GetAccountAPIKey(c.Context(), account.ID, c.Params("id")); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load api key")
	} else if !ok {
		return fiber.NewError(fiber.StatusNotFound, "api key not found")
	}
	key, err := s.store.RevokeAPIKey(c.Context(), c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	return c.JSON(fiber.Map{"key": key})
}

func (s Server) dashboardRotateAPIKey(c *fiber.Ctx) error {
	account, ok := c.Locals(accountLocalKey).(store.Account)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	oldKey, ok, err := s.store.GetAccountAPIKey(c.Context(), account.ID, c.Params("id"))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load api key")
	}
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "api key not found")
	}
	app, ok, err := s.store.GetAccountApp(c.Context(), account.ID, oldKey.AppID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to load project")
	}
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "api key not found")
	}
	if _, err := s.store.RevokeAPIKey(c.Context(), oldKey.ID); err != nil {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
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

func (s Server) dashboardAccountApp(c *fiber.Ctx) (store.Account, store.App, error) {
	account, ok := c.Locals(accountLocalKey).(store.Account)
	if !ok {
		return store.Account{}, store.App{}, fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	app, found, err := s.store.GetAccountApp(c.Context(), account.ID, c.Params("id"))
	if err != nil {
		return store.Account{}, store.App{}, fiber.NewError(fiber.StatusInternalServerError, "failed to load project")
	}
	if !found {
		return store.Account{}, store.App{}, fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	return account, app, nil
}

type erc20ConsoleToken struct {
	ContractAddress string `json:"contractAddress"`
	Decimals        int64  `json:"decimals"`
	Name            string `json:"name"`
	OwnedBalance    string `json:"ownedBalance"`
	OwnedBalanceRaw string `json:"ownedBalanceRaw"`
	SupportsBurn    bool   `json:"supportsBurn"`
	Symbol          string `json:"symbol"`
	TotalSupply     string `json:"totalSupply"`
	TotalSupplyRaw  string `json:"totalSupplyRaw"`
	WalletAddress   string `json:"walletAddress"`
}

type erc20ActionRequest struct {
	Action          string                  `json:"action"`
	Amount          string                  `json:"amount"`
	ConfirmPassword string                  `json:"confirmPassword"`
	Recipient       string                  `json:"recipient"`
	Recipients      []erc20AirdropRecipient `json:"recipients"`
}

type erc20AirdropRecipient struct {
	Address string `json:"address"`
	Amount  string `json:"amount"`
}

type contractCallRequest struct {
	ABI             json.RawMessage `json:"abi"`
	Args            []string        `json:"args"`
	ChainID         int64           `json:"chainId"`
	ConfirmPassword string          `json:"confirmPassword"`
	ContractAddress string          `json:"contractAddress"`
	FunctionName    string          `json:"functionName"`
	Value           string          `json:"value"`
	WalletAddress   string          `json:"walletAddress"`
}

type erc20ActionResult struct {
	Transactions []struct {
		ID              string `json:"id"`
		Status          string `json:"status"`
		TransactionHash string `json:"transactionHash"`
	} `json:"transactions"`
}

func (s Server) erc20ConsoleRead(ctx context.Context, deployment store.ERC20Deployment, walletAddress string) (erc20ConsoleToken, error) {
	rpcURL, err := s.cfg.RPCURL(deployment.ChainID)
	if err != nil {
		return erc20ConsoleToken{}, err
	}
	payload, err := json.Marshal(map[string]any{
		"chainId":         deployment.ChainID,
		"contractAddress": deployment.ContractAddress,
		"mode":            "read",
		"rpcUrl":          rpcURL,
		"walletAddress":   walletAddress,
	})
	if err != nil {
		return erc20ConsoleToken{}, err
	}
	output, err := runERC20ConsoleScript(ctx, payload)
	if err != nil {
		return erc20ConsoleToken{}, err
	}
	var token erc20ConsoleToken
	if err := json.Unmarshal(output, &token); err != nil {
		return erc20ConsoleToken{}, err
	}
	token.ContractAddress = deployment.ContractAddress
	return token, nil
}

func (s Server) runContractRead(ctx context.Context, request contractCallRequest) (json.RawMessage, error) {
	rpcURL, err := s.cfg.RPCURL(request.ChainID)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{
		"abi":             json.RawMessage(request.ABI),
		"args":            request.Args,
		"chainId":         request.ChainID,
		"contractAddress": request.ContractAddress,
		"functionName":    request.FunctionName,
		"mode":            "read",
		"rpcUrl":          rpcURL,
	})
	if err != nil {
		return nil, err
	}
	output, err := runContractCallScript(ctx, payload)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(output), nil
}

func (s Server) erc20ConsoleWrite(ctx context.Context, deployment store.ERC20Deployment, request erc20ActionRequest) (erc20ActionResult, error) {
	action := strings.ToLower(strings.TrimSpace(request.Action))
	if action != "mint" && action != "transfer" && action != "burn" && action != "airdrop" {
		return erc20ActionResult{}, errors.New("unsupported token action")
	}
	if action == "burn" && !erc20DeploymentSupportsBurn(deployment) {
		return erc20ActionResult{}, errors.New("this token was deployed before GMR burn support and does not expose burn(uint256)")
	}
	wallet, ok, err := s.store.GetProjectDefaultAdminWalletSecret(ctx, deployment.AppID)
	if err != nil {
		return erc20ActionResult{}, err
	}
	if !ok {
		return erc20ActionResult{}, errors.New("project wallet not found")
	}
	signerPayload, err := s.projectSignerPayload(ctx, deployment.AppID, wallet, "")
	if err != nil {
		return erc20ActionResult{}, err
	}
	rpcURL, err := s.cfg.RPCURL(deployment.ChainID)
	if err != nil {
		return erc20ActionResult{}, err
	}
	payload := map[string]any{
		"action":          action,
		"chainId":         deployment.ChainID,
		"contractAddress": deployment.ContractAddress,
		"mode":            "write",
		"rpcUrl":          rpcURL,
		"walletAddress":   wallet.Address,
	}
	for key, value := range signerPayload {
		payload[key] = value
	}
	if action == "airdrop" {
		recipients := []map[string]string{}
		for _, recipient := range request.Recipients {
			amount, err := decimalStringToBaseUnits(recipient.Amount, deployment.Decimals)
			if err != nil {
				return erc20ActionResult{}, err
			}
			recipients = append(recipients, map[string]string{"address": strings.TrimSpace(recipient.Address), "amount": amount})
		}
		if len(recipients) == 0 {
			return erc20ActionResult{}, errors.New("airdrop recipients are required")
		}
		payload["recipients"] = recipients
	} else {
		amount, err := decimalStringToBaseUnits(request.Amount, deployment.Decimals)
		if err != nil {
			return erc20ActionResult{}, err
		}
		payload["amount"] = amount
		if action == "mint" || action == "transfer" {
			if strings.TrimSpace(request.Recipient) == "" {
				return erc20ActionResult{}, errors.New("recipient is required")
			}
			payload["recipient"] = strings.TrimSpace(request.Recipient)
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return erc20ActionResult{}, err
	}
	output, err := runERC20ConsoleScript(ctx, body)
	if err != nil {
		return erc20ActionResult{}, err
	}
	var result erc20ActionResult
	if err := json.Unmarshal(output, &result); err != nil {
		return erc20ActionResult{}, err
	}
	return result, nil
}

func (s Server) enqueueERC20ConsoleWrite(ctx context.Context, deployment store.ERC20Deployment, request erc20ActionRequest) (erc20ActionResult, error) {
	action := strings.ToLower(strings.TrimSpace(request.Action))
	if action != "mint" && action != "transfer" && action != "burn" && action != "airdrop" {
		return erc20ActionResult{}, errors.New("unsupported token action")
	}
	if action == "burn" && !erc20DeploymentSupportsBurn(deployment) {
		return erc20ActionResult{}, errors.New("this token was deployed before GMR burn support and does not expose burn(uint256)")
	}
	wallet, ok, err := s.store.GetProjectDefaultAdminWallet(ctx, deployment.AppID)
	if err != nil {
		return erc20ActionResult{}, err
	}
	if !ok {
		return erc20ActionResult{}, errors.New("project wallet not found")
	}
	enqueue := func(method string, args []string, amount string, recipient string) (store.Transaction, error) {
		return s.store.EnqueueTransaction(ctx, store.TransactionInput{
			AppID:           deployment.AppID,
			ChainID:         deployment.ChainID,
			WalletAddress:   wallet.Address,
			ContractAddress: deployment.ContractAddress,
			Kind:            "contract_write",
			Method:          method,
			Args:            args,
			Value:           "0",
			Metadata: map[string]any{
				"amount":       amount,
				"deploymentId": deployment.ID,
				"recipient":    recipient,
				"source":       "dashboard",
				"symbol":       deployment.Symbol,
			},
		})
	}
	result := erc20ActionResult{}
	if action == "airdrop" {
		for _, recipient := range request.Recipients {
			amount, err := decimalStringToBaseUnits(recipient.Amount, deployment.Decimals)
			if err != nil {
				return erc20ActionResult{}, err
			}
			address := strings.TrimSpace(recipient.Address)
			if address == "" {
				return erc20ActionResult{}, errors.New("airdrop recipient address is required")
			}
			transaction, err := enqueue("transfer", []string{address, amount}, recipient.Amount, address)
			if err != nil {
				return erc20ActionResult{}, err
			}
			result.Transactions = append(result.Transactions, struct {
				ID              string `json:"id"`
				Status          string `json:"status"`
				TransactionHash string `json:"transactionHash"`
			}{ID: transaction.ID, Status: transaction.Status})
		}
		if len(result.Transactions) == 0 {
			return erc20ActionResult{}, errors.New("airdrop recipients are required")
		}
		return result, nil
	}
	amount, err := decimalStringToBaseUnits(request.Amount, deployment.Decimals)
	if err != nil {
		return erc20ActionResult{}, err
	}
	args := []string{amount}
	recipient := ""
	if action == "mint" || action == "transfer" {
		recipient = strings.TrimSpace(request.Recipient)
		if recipient == "" {
			return erc20ActionResult{}, errors.New("recipient is required")
		}
		args = []string{recipient, amount}
	}
	transaction, err := enqueue(action, args, request.Amount, recipient)
	if err != nil {
		return erc20ActionResult{}, err
	}
	result.Transactions = append(result.Transactions, struct {
		ID              string `json:"id"`
		Status          string `json:"status"`
		TransactionHash string `json:"transactionHash"`
	}{ID: transaction.ID, Status: transaction.Status})
	return result, nil
}

func runERC20ConsoleScript(ctx context.Context, payload []byte) ([]byte, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "bun", "scripts/erc20-console.ts")
	cmd.Stdin = bytes.NewReader(payload)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = "ERC20 console command failed without output"
		}
		return nil, errors.New(message)
	}
	return bytes.TrimSpace(output), nil
}

func runContractCallScript(ctx context.Context, payload []byte) ([]byte, error) {
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "bun", "scripts/contract-call.ts")
	cmd.Stdin = bytes.NewReader(payload)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = "contract call command failed without output"
		}
		return nil, errors.New(message)
	}
	return bytes.TrimSpace(output), nil
}

func erc20DeploymentSupportsBurn(deployment store.ERC20Deployment) bool {
	source := deployment.SourceCode
	return strings.Contains(source, "function burn(uint256") || strings.Contains(source, "function burn (uint256") || strings.Contains(source, "function burn(address")
}

func decimalStringToBaseUnits(amount string, decimals int64) (string, error) {
	trimmed := strings.TrimSpace(amount)
	if trimmed == "" {
		return "", errors.New("amount is required")
	}
	if decimals < 0 || decimals > 77 {
		return "", errors.New("invalid decimals")
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) > 2 {
		return "", errors.New("invalid decimal amount")
	}
	whole := parts[0]
	if whole == "" {
		whole = "0"
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if len(fraction) > int(decimals) {
		return "", errors.New("amount has too many decimal places")
	}
	for _, char := range whole + fraction {
		if char < '0' || char > '9' {
			return "", errors.New("amount must be numeric")
		}
	}
	base := new(big.Int).Exp(big.NewInt(10), big.NewInt(decimals), nil)
	wholeValue, ok := new(big.Int).SetString(whole, 10)
	if !ok {
		return "", errors.New("invalid amount")
	}
	result := new(big.Int).Mul(wholeValue, base)
	if fraction != "" {
		fraction += strings.Repeat("0", int(decimals)-len(fraction))
		fractionValue, ok := new(big.Int).SetString(fraction, 10)
		if !ok {
			return "", errors.New("invalid amount")
		}
		result.Add(result, fractionValue)
	}
	return result.String(), nil
}
