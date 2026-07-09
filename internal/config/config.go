package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr             string
	AdminAPIKey      string
	AllowedOrigins   []string
	ChainRPCURL      string
	AppEnv           string
	DashboardCookie  string
	DeployerEnabled  bool
	DeployerInterval time.Duration
	MemgraphURI      string
	MemgraphUser     string
	MemgraphPassword string
	OwnerEmail       string
	OwnerName        string
	OwnerPassword    string
	OwnerUsername    string
	SessionTTL       time.Duration
	ShutdownTimeout  time.Duration
	VaultEnabled     bool
	VaultInternalKey string
	VaultURL         string
	ZKVerifyNetwork  string
	ZKVerifyRPC      string
	ZKVerifySeed     string
	ZKVerifyWS       string
}

func Load() (Config, error) {
	loadDotEnv(".env.gmr-engine", "../.env.gmr-engine")

	cfg := Config{
		Addr:             envAny([]string{"GMR_ENGINE_ADDR", "ENGINE_ADDR"}, ":8090"),
		AdminAPIKey:      envAny([]string{"GMR_ENGINE_ADMIN_API_KEY", "ENGINE_ADMIN_API_KEY"}, ""),
		AllowedOrigins:   splitCSV(envAny([]string{"GMR_ENGINE_ALLOWED_ORIGINS", "ENGINE_ALLOWED_ORIGINS"}, "http://localhost:3000,http://127.0.0.1:3000,http://localhost:3001,http://127.0.0.1:3001,http://localhost:3002,http://127.0.0.1:3002")),
		ChainRPCURL:      env("GMR_ENGINE_RPC_URL", "https://horizen-testnet.rpc.caldera.xyz/http"),
		AppEnv:           env("APP_ENV", "development"),
		DashboardCookie:  env("GMR_ENGINE_DASHBOARD_COOKIE", "gmr_engine_session"),
		DeployerEnabled:  envBool("GMR_ENGINE_DEPLOYER_ENABLED", true),
		DeployerInterval: time.Duration(envInt("GMR_ENGINE_DEPLOYER_INTERVAL_SECONDS", 8)) * time.Second,
		MemgraphURI:      envAny([]string{"GMR_ENGINE_MEMGRAPH_URI", "ENGINE_MEMGRAPH_URI"}, "bolt://localhost:7689"),
		MemgraphUser:     envAny([]string{"GMR_ENGINE_MEMGRAPH_USER", "ENGINE_MEMGRAPH_USER"}, ""),
		MemgraphPassword: envAny([]string{"GMR_ENGINE_MEMGRAPH_PASSWORD", "ENGINE_MEMGRAPH_PASSWORD"}, ""),
		OwnerEmail:       env("GMR_ENGINE_OWNER_EMAIL", ""),
		OwnerName:        env("GMR_ENGINE_OWNER_NAME", "destrega"),
		OwnerPassword:    os.Getenv("GMR_ENGINE_OWNER_PASSWORD"),
		OwnerUsername:    env("GMR_ENGINE_OWNER_USERNAME", "destrega"),
		SessionTTL:       time.Duration(envInt("GMR_ENGINE_SESSION_TTL_HOURS", 24)) * time.Hour,
		ShutdownTimeout:  time.Duration(envIntAny([]string{"GMR_ENGINE_SHUTDOWN_TIMEOUT_SECONDS", "ENGINE_SHUTDOWN_TIMEOUT_SECONDS"}, 5)) * time.Second,
		VaultEnabled:     envBool("GMR_ENGINE_VAULT_ENABLED", true),
		VaultInternalKey: os.Getenv("GMR_ENGINE_VAULT_INTERNAL_API_KEY"),
		VaultURL:         env("GMR_ENGINE_VAULT_URL", "http://localhost:8091"),
		ZKVerifyNetwork:  env("GMR_ENGINE_ZKVERIFY_NETWORK", "Volta"),
		ZKVerifyRPC:      env("GMR_ENGINE_ZKVERIFY_RPC_URL", "https://testnet-rpc.zkverify.io"),
		ZKVerifySeed:     os.Getenv("GMR_ENGINE_ZKVERIFY_SEED_PHRASE"),
		ZKVerifyWS:       env("GMR_ENGINE_ZKVERIFY_WS_URL", "wss://testnet-rpc.zkverify.io"),
	}

	if len(cfg.AdminAPIKey) < 24 {
		return Config{}, errors.New("GMR_ENGINE_ADMIN_API_KEY must be at least 24 characters")
	}
	if len(cfg.OwnerPassword) < 12 {
		return Config{}, errors.New("GMR_ENGINE_OWNER_PASSWORD must be at least 12 characters")
	}
	if !cfg.VaultEnabled {
		return Config{}, errors.New("legacy local project wallets have been removed; set GMR_ENGINE_VAULT_ENABLED=true")
	}
	if strings.TrimSpace(cfg.VaultURL) == "" {
		return Config{}, errors.New("GMR_ENGINE_VAULT_URL is required")
	}
	if len(cfg.VaultInternalKey) < 32 {
		return Config{}, errors.New("GMR_ENGINE_VAULT_INTERNAL_API_KEY must be at least 32 characters")
	}
	if cfg.DeployerEnabled && strings.TrimSpace(cfg.ChainRPCURL) == "" {
		return Config{}, errors.New("GMR_ENGINE_RPC_URL is required when deployer is enabled")
	}
	if cfg.IsProduction() {
		if len(cfg.AllowedOrigins) == 0 {
			return Config{}, errors.New("GMR_ENGINE_ALLOWED_ORIGINS is required in production")
		}
		for _, origin := range cfg.AllowedOrigins {
			if origin == "*" {
				return Config{}, errors.New("GMR_ENGINE_ALLOWED_ORIGINS cannot contain * in production")
			}
		}
	}

	return cfg, nil
}

func (c Config) IsProduction() bool {
	return c.AppEnv == "production"
}

func env(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envAny(keys []string, fallback string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return fallback
}

func envInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func envIntAny(keys []string, fallback int) int {
	for _, key := range keys {
		raw := os.Getenv(key)
		if raw == "" {
			continue
		}
		value, err := strconv.Atoi(raw)
		if err == nil && value > 0 {
			return value
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if raw == "" {
		return fallback
	}
	return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
