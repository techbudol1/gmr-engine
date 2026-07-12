package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/techbudol1/gmr-engine/internal/config"
	"github.com/techbudol1/gmr-engine/internal/store"
	"github.com/techbudol1/gmr-engine/internal/vaultclient"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

const apiKeyHeader = "X-GMR-Engine-Key"
const legacyAPIKeyHeader = "X-Budol-Engine-Key"
const adminKeyHeader = "X-GMR-Engine-Admin-Key"
const legacyAdminKeyHeader = "X-Budol-Engine-Admin-Key"
const principalLocalKey = "gmr_engine_principal"
const accountLocalKey = "gmr_engine_account"

var defaultScopes = []string{
	"transactions:write",
	"transactions:read",
	"wallets:read",
	"wallets:write",
	"contracts:read",
	"contracts:write",
}

type Server struct {
	cfg     config.Config
	limiter *rateLimiter
	store   store.Store
}

type principal struct {
	App store.App
	Key store.APIKey
}

type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]rateBucket
}

type rateBucket struct {
	Window int64
	Count  int64
}

type apiKeyRequest struct {
	Name           string   `json:"name"`
	Scopes         []string `json:"scopes"`
	AllowedOrigins []string `json:"allowedOrigins"`
	AllowedIPs     []string `json:"allowedIps"`
	ExpiresAt      string   `json:"expiresAt"`
}

type generatedAPIKey struct {
	Prefix    string
	Secret    string
	Plaintext string
}

func New(cfg config.Config, engineStore store.Store) *fiber.App {
	server := Server{cfg: cfg, limiter: &rateLimiter{buckets: map[string]rateBucket{}}, store: engineStore}

	app := fiber.New(fiber.Config{
		AppName:      "GMR Engine",
		ErrorHandler: errorHandler,
	})
	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(helmet.New())
	app.Use(cors.New(cors.Config{
		AllowCredentials: true,
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization, X-GMR-Engine-Key, X-GMR-Engine-Admin-Key, X-Budol-Engine-Key, X-Budol-Engine-Admin-Key, Idempotency-Key",
		AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowOrigins:     strings.Join(cfg.AllowedOrigins, ","),
	}))

	app.Get("/healthz", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true, "service": "gmr-engine"})
	})

	admin := app.Group("/admin", server.requireAdmin)
	admin.Get("/apps", server.adminApps)
	admin.Post("/apps", server.adminCreateApp)
	admin.Get("/apps/:id", server.adminApp)
	admin.Post("/apps/:id/api-keys", server.adminCreateAPIKey)
	admin.Get("/apps/:id/api-keys", server.adminAPIKeys)
	admin.Get("/apps/:id/usage", server.adminUsage)
	admin.Get("/apps/:id/transactions", server.adminTransactions)
	admin.Post("/api-keys/:id/revoke", server.adminRevokeAPIKey)
	admin.Post("/api-keys/:id/rotate", server.adminRotateAPIKey)

	dashboard := app.Group("/dashboard")
	dashboard.Post("/auth/login", server.dashboardLogin)
	dashboard.Get("/auth/me", server.requireAccount, server.dashboardMe)
	dashboard.Post("/auth/logout", server.requireAccount, server.dashboardLogout)
	dashboard.Get("/projects", server.requireAccount, server.dashboardProjects)
	dashboard.Post("/projects", server.requireAccount, server.dashboardCreateProject)
	dashboard.Get("/projects/:id", server.requireAccount, server.dashboardProject)
	dashboard.Put("/projects/:id/settings", server.requireAccount, server.dashboardUpdateProject)
	dashboard.Post("/projects/:id/archive", server.requireAccount, server.dashboardArchiveProject)
	dashboard.Post("/projects/:id/api-keys", server.requireAccount, server.dashboardCreateAPIKey)
	dashboard.Get("/projects/:id/api-keys", server.requireAccount, server.dashboardAPIKeys)
	dashboard.Get("/projects/:id/usage", server.requireAccount, server.dashboardUsage)
	dashboard.Get("/projects/:id/transactions", server.requireAccount, server.dashboardTransactions)
	dashboard.Get("/projects/:id/transactions/:transactionId", server.requireAccount, server.dashboardTransaction)
	dashboard.Get("/projects/:id/notifications", server.requireAccount, server.dashboardNotifications)
	dashboard.Get("/projects/:id/zkverify/account", server.requireAccount, server.dashboardZKVerifyAccount)
	dashboard.Get("/projects/:id/zkverify/proofs", server.requireAccount, server.dashboardZKProofSubmissions)
	dashboard.Post("/projects/:id/zkverify/proofs", server.requireAccount, server.dashboardSubmitZKProof)
	dashboard.Get("/projects/:id/wallets", server.requireAccount, server.dashboardProjectWallets)
	dashboard.Post("/projects/:id/wallets", server.requireAccount, server.dashboardCreateProjectWallet)
	dashboard.Get("/projects/:id/wallets/:walletId/detail", server.requireAccount, server.dashboardProjectWalletDetail)
	dashboard.Get("/projects/:id/user-wallets", server.requireAccount, server.dashboardUserWallets)
	dashboard.Get("/projects/:id/user-wallets/:walletId/detail", server.requireAccount, server.dashboardUserWalletDetail)
	dashboard.Delete("/projects/:id/user-wallets/:walletId", server.requireAccount, server.dashboardDeleteUserWallet)
	dashboard.Get("/projects/:id/contracts/erc20", server.requireAccount, server.dashboardERC20Deployments)
	dashboard.Post("/projects/:id/contracts/erc20", server.requireAccount, server.dashboardCreateERC20Deployment)
	dashboard.Get("/projects/:id/contracts/escrow", server.requireAccount, server.dashboardEscrowDeployments)
	dashboard.Post("/projects/:id/contracts/escrow", server.requireAccount, server.dashboardCreateEscrowDeployment)
	dashboard.Delete("/contracts/escrow/:id", server.requireAccount, server.dashboardDeleteEscrowDeployment)
	dashboard.Get("/projects/:id/contracts/private-claim-registries", server.requireAccount, server.dashboardPrivateClaimRegistryDeployments)
	dashboard.Post("/projects/:id/contracts/private-claim-registries", server.requireAccount, server.dashboardCreatePrivateClaimRegistryDeployment)
	dashboard.Delete("/contracts/private-claim-registries/:id", server.requireAccount, server.dashboardDeletePrivateClaimRegistryDeployment)
	dashboard.Get("/projects/:id/contracts/shielded-payout-pools", server.requireAccount, server.dashboardShieldedPayoutPoolDeployments)
	dashboard.Post("/projects/:id/contracts/shielded-payout-pools", server.requireAccount, server.dashboardCreateShieldedPayoutPoolDeployment)
	dashboard.Delete("/contracts/shielded-payout-pools/:id", server.requireAccount, server.dashboardDeleteShieldedPayoutPoolDeployment)
	dashboard.Get("/projects/:id/contracts/shielded-withdrawal-verifiers", server.requireAccount, server.dashboardShieldedWithdrawalVerifierDeployments)
	dashboard.Post("/projects/:id/contracts/shielded-withdrawal-verifiers", server.requireAccount, server.dashboardCreateShieldedWithdrawalVerifierDeployment)
	dashboard.Delete("/contracts/shielded-withdrawal-verifiers/:id", server.requireAccount, server.dashboardDeleteShieldedWithdrawalVerifierDeployment)
	dashboard.Get("/projects/:id/contracts/account-abstraction", server.requireAccount, server.dashboardAccountAbstractionDeployments)
	dashboard.Post("/projects/:id/contracts/account-abstraction", server.requireAccount, server.dashboardCreateAccountAbstractionDeployment)
	dashboard.Delete("/contracts/account-abstraction/:id", server.requireAccount, server.dashboardDeleteAccountAbstractionDeployment)
	dashboard.Get("/projects/:id/contracts/imported", server.requireAccount, server.dashboardImportedContracts)
	dashboard.Post("/projects/:id/contracts/imported", server.requireAccount, server.dashboardImportContract)
	dashboard.Delete("/contracts/imported/:id", server.requireAccount, server.dashboardDeleteImportedContract)
	dashboard.Post("/projects/:id/contracts/read", server.requireAccount, server.dashboardContractRead)
	dashboard.Post("/projects/:id/contracts/write", server.requireAccount, server.dashboardContractWrite)
	dashboard.Get("/contracts/erc20/:id/console", server.requireAccount, server.dashboardERC20Console)
	dashboard.Post("/contracts/erc20/:id/actions", server.requireAccount, server.dashboardERC20Action)
	dashboard.Post("/contracts/erc20/:id/retry", server.requireAccount, server.dashboardRetryERC20Deployment)
	dashboard.Delete("/contracts/erc20/:id", server.requireAccount, server.dashboardDeleteERC20Deployment)
	dashboard.Get("/projects/:id/contracts/erc1155-editions", server.requireAccount, server.dashboardERC1155EditionDeployments)
	dashboard.Post("/projects/:id/contracts/erc1155-editions", server.requireAccount, server.dashboardCreateERC1155EditionDeployment)
	dashboard.Delete("/contracts/erc1155-editions/:id", server.requireAccount, server.dashboardDeleteERC1155EditionDeployment)
	dashboard.Post("/api-keys/:id/revoke", server.requireAccount, server.dashboardRevokeAPIKey)
	dashboard.Post("/api-keys/:id/rotate", server.requireAccount, server.dashboardRotateAPIKey)

	v1 := app.Group("/v1")
	v1.Get("/auth/me", server.requireScope("transactions:read"), server.authMe)
	v1.Patch("/app/gas-free", server.requireScope("transactions:write"), server.updateAppGasFree)
	v1.Get("/wallets", server.requireScope("wallets:read"), server.wallets)
	v1.Post("/wallets", server.requireScope("wallets:write"), server.createWallet)
	v1.Get("/user-wallets", server.requireScope("wallets:read"), server.userWallets)
	v1.Post("/user-wallets", server.requireScope("wallets:write"), server.upsertUserWallet)
	v1.Post("/user-wallets/managed", server.requireScope("wallets:write"), server.createManagedUserWallet)
	v1.Delete("/user-wallets/:id", server.requireScope("wallets:write"), server.deleteUserWallet)
	v1.Get("/erc20/balance", server.requireScope("wallets:read"), server.erc20Balance)
	v1.Post("/erc20/transfer", server.requireScope("transactions:write"), server.erc20Transfer)
	v1.Post("/erc20/transfer-with-permit", server.requireScope("transactions:write"), server.erc20TransferWithPermit)
	v1.Post("/erc20/managed-transfer-with-permit", server.requireScope("transactions:write"), server.erc20ManagedTransferWithPermit)
	v1.Post("/transactions", server.requireScope("transactions:write"), server.enqueueTransaction)
	v1.Get("/transactions/:id", server.requireScope("transactions:read"), server.transaction)
	v1.Get("/zkverify/account", server.requireScope("contracts:read"), server.zkVerifyAccount)
	v1.Get("/zkverify/proofs/:id", server.requireScope("contracts:read"), server.zkProofSubmission)
	v1.Post("/zkverify/proofs", server.requireScope("contracts:write"), server.submitZKProof)
	v1.Post("/contracts/read", server.requireScope("contracts:read"), server.contractRead)
	v1.Post("/contracts/write", server.requireScope("contracts:write"), server.contractWrite)
	v1.Post("/contracts/erc20/deployments", server.requireScope("contracts:write"), server.createERC20Deployment)
	v1.Post("/contracts/escrow/deployments", server.requireScope("contracts:write"), server.createEscrowDeployment)
	v1.Post("/contracts/erc1155-editions/deployments", server.requireScope("contracts:write"), server.createERC1155EditionDeployment)
	v1.Post("/contracts/private-claim-registries/deployments", server.requireScope("contracts:write"), server.createPrivateClaimRegistryDeployment)
	v1.Post("/contracts/shielded-payout-pools/deployments", server.requireScope("contracts:write"), server.createShieldedPayoutPoolDeployment)
	v1.Post("/contracts/shielded-withdrawal-verifiers/deployments", server.requireScope("contracts:write"), server.createShieldedWithdrawalVerifierDeployment)
	v1.Post("/contracts/account-abstraction/deployments", server.requireScope("contracts:write"), server.createAccountAbstractionDeployment)
	v1.Post("/wallet-locks", server.requireScope("wallets:write"), server.acquireWalletLock)
	v1.Delete("/wallet-locks", server.requireScope("wallets:write"), server.releaseWalletLock)

	return app
}

func (s Server) requireAccount(c *fiber.Ctx) error {
	token := strings.TrimSpace(c.Cookies(s.cfg.DashboardCookie))
	if token == "" {
		return fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	account, ok, err := s.store.GetAccountBySession(c.Context(), hashToken(token))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to verify session")
	}
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "login required")
	}
	c.Locals(accountLocalKey, account)
	return c.Next()
}

func (l *rateLimiter) allow(keyID string, limit int64) bool {
	if limit <= 0 {
		limit = 60
	}
	window := time.Now().Unix() / 60
	l.mu.Lock()
	defer l.mu.Unlock()
	bucket := l.buckets[keyID]
	if bucket.Window != window {
		bucket = rateBucket{Window: window}
	}
	if bucket.Count >= limit {
		l.buckets[keyID] = bucket
		return false
	}
	bucket.Count++
	l.buckets[keyID] = bucket
	return true
}

func (s Server) requireAdmin(c *fiber.Ctx) error {
	value := strings.TrimSpace(c.Get(adminKeyHeader))
	if value == "" {
		value = strings.TrimSpace(c.Get(legacyAdminKeyHeader))
	}
	if value == "" {
		authHeader := strings.TrimSpace(c.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			value = strings.TrimSpace(authHeader[7:])
		}
	}
	if subtle.ConstantTimeCompare([]byte(value), []byte(s.cfg.AdminAPIKey)) != 1 {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid engine admin key")
	}
	return c.Next()
}

func (s Server) requireScope(scope string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		principal, err := s.principal(c)
		if err != nil {
			return err
		}
		if !hasScope(principal.Key.Scopes, scope) {
			s.recordUsage(c, principal, fiber.StatusForbidden)
			return fiber.NewError(fiber.StatusForbidden, "api key missing scope "+scope)
		}
		if !allowedOrigin(principal.Key.AllowedOrigins, c.Get("Origin")) {
			s.recordUsage(c, principal, fiber.StatusForbidden)
			return fiber.NewError(fiber.StatusForbidden, "origin is not allowed")
		}
		if !s.limiter.allow(principal.Key.ID, principal.App.RateLimitPerMin) {
			s.recordUsage(c, principal, fiber.StatusTooManyRequests)
			return fiber.NewError(fiber.StatusTooManyRequests, "api key rate limit exceeded")
		}
		c.Locals(principalLocalKey, principal)
		err = c.Next()
		status := c.Response().StatusCode()
		if err != nil {
			if fiberErr, ok := err.(*fiber.Error); ok {
				status = fiberErr.Code
			} else {
				status = fiber.StatusInternalServerError
			}
		}
		s.recordUsage(c, principal, status)
		return err
	}
}

func (s Server) principal(c *fiber.Ctx) (principal, error) {
	keyValue := strings.TrimSpace(c.Get(apiKeyHeader))
	if keyValue == "" {
		keyValue = strings.TrimSpace(c.Get(legacyAPIKeyHeader))
	}
	authHeader := strings.TrimSpace(c.Get("Authorization"))
	if keyValue == "" && strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		keyValue = strings.TrimSpace(authHeader[7:])
	}
	prefix, ok := apiKeyPrefix(keyValue)
	if !ok {
		return principal{}, fiber.NewError(fiber.StatusUnauthorized, "invalid engine api key")
	}
	verification, ok, err := s.store.VerifyAPIKey(c.Context(), prefix, hashAPIKey(keyValue))
	if err != nil {
		return principal{}, fiber.NewError(fiber.StatusInternalServerError, "failed to verify api key")
	}
	if !ok {
		return principal{}, fiber.NewError(fiber.StatusUnauthorized, "invalid engine api key")
	}
	return principal{App: verification.App, Key: verification.Key}, nil
}

func (s Server) recordUsage(c *fiber.Ctx, p principal, status int) {
	_ = s.store.RecordAPIUsage(c.Context(), store.APIUsageInput{
		AppID:      p.App.ID,
		KeyID:      p.Key.ID,
		Method:     c.Method(),
		Path:       c.Path(),
		StatusCode: status,
		IPAddress:  c.IP(),
		UserAgent:  c.Get("User-Agent"),
	})
}

func generateAPIKey(environment string) (generatedAPIKey, error) {
	randomPrefix, err := randomURLToken(8)
	if err != nil {
		return generatedAPIKey{}, err
	}
	secret, err := randomURLToken(32)
	if err != nil {
		return generatedAPIKey{}, err
	}
	envPart := "dev"
	if strings.EqualFold(environment, "production") {
		envPart = "live"
	}
	prefix := "bdl_" + envPart + "_" + strings.ToLower(randomPrefix)
	return generatedAPIKey{Prefix: prefix, Secret: secret, Plaintext: prefix + "." + secret}, nil
}

func randomURLToken(byteCount int) (string, error) {
	buffer := make([]byte, byteCount)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func hashAPIKey(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

func HashPassword(value string) string {
	return hashToken("gmr-password:" + value)
}

func hashToken(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

func apiKeyPrefix(value string) (string, bool) {
	prefix, _, ok := strings.Cut(strings.TrimSpace(value), ".")
	if !ok || !strings.HasPrefix(prefix, "bdl_") {
		return "", false
	}
	return prefix, true
}

func hasScope(scopes []string, required string) bool {
	for _, scope := range scopes {
		if subtle.ConstantTimeCompare([]byte(scope), []byte("admin")) == 1 || subtle.ConstantTimeCompare([]byte(scope), []byte(required)) == 1 {
			return true
		}
	}
	return false
}

func allowedOrigin(allowed []string, origin string) bool {
	if len(allowed) == 0 || strings.TrimSpace(origin) == "" {
		return true
	}
	for _, value := range allowed {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(origin)) {
			return true
		}
	}
	return false
}

func errorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := "internal server error"
	if fiberErr, ok := err.(*fiber.Error); ok {
		code = fiberErr.Code
		message = fiberErr.Message
	}
	return c.Status(code).JSON(fiber.Map{"error": message})
}

func (s Server) ensureProjectDefaultAdminWallet(c *fiber.Ctx, appID string) (store.ProjectWallet, error) {
	existing, ok, err := s.store.GetProjectDefaultAdminWallet(c.Context(), appID)
	if err != nil {
		return store.ProjectWallet{}, fiber.NewError(fiber.StatusInternalServerError, "failed to load project wallet")
	}
	if ok {
		return existing, nil
	}
	return s.generateProjectWallet(c, appID, "server_admin", true)
}

func (s Server) generateProjectWallet(c *fiber.Ctx, appID string, walletType string, isDefaultAdmin bool) (store.ProjectWallet, error) {
	walletType = strings.TrimSpace(walletType)
	if walletType == "" {
		walletType = "server"
	}
	client, err := vaultclient.New(s.cfg.VaultURL, s.cfg.VaultInternalKey, 20*time.Second)
	if err != nil {
		return store.ProjectWallet{}, fiber.NewError(fiber.StatusInternalServerError, "failed to configure vault client")
	}
	generated, err := client.CreateWallet(c.Context(), appID, walletType, "")
	if err != nil {
		return store.ProjectWallet{}, fiber.NewError(fiber.StatusBadGateway, "failed to create project wallet in vault")
	}
	projectWallet, err := s.store.CreateProjectWallet(c.Context(), appID, store.ProjectWalletInput{
		Address:             generated.Address,
		EncryptedPrivateKey: vaultclient.Reference(generated.ID),
		IsDefaultAdmin:      isDefaultAdmin,
		WalletType:          walletType,
	})
	if err != nil {
		return store.ProjectWallet{}, fiber.NewError(fiber.StatusInternalServerError, "failed to save project server wallet")
	}
	return projectWallet, nil
}
