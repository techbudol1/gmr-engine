package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/techbudol1/gmr-engine/internal/contracts"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type MemgraphStore struct {
	driver neo4j.DriverWithContext
}

func NewMemgraphStore(ctx context.Context, uri string, username string, password string) (*MemgraphStore, error) {
	auth := neo4j.NoAuth()
	if username != "" {
		auth = neo4j.BasicAuth(username, password, "")
	}
	driver, err := neo4j.NewDriverWithContext(uri, auth)
	if err != nil {
		return nil, err
	}
	if err := driver.VerifyConnectivity(ctx); err != nil {
		_ = driver.Close(ctx)
		return nil, err
	}
	return &MemgraphStore{driver: driver}, nil
}

func (s *MemgraphStore) Close(ctx context.Context) error {
	return s.driver.Close(ctx)
}

var validScopes = map[string]bool{
	"transactions:write": true,
	"transactions:read":  true,
	"wallets:read":       true,
	"wallets:write":      true,
	"contracts:read":     true,
	"contracts:write":    true,
	"admin":              true,
}

func (s *MemgraphStore) SeedOwnerAccount(ctx context.Context, username string, email string, passwordHash string, name string) (Account, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	email = strings.ToLower(strings.TrimSpace(email))
	name = strings.TrimSpace(name)
	if username == "" || strings.TrimSpace(passwordHash) == "" {
		return Account{}, errors.New("owner username and password hash are required")
	}
	if name == "" {
		name = "Owner"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MERGE (a:GMRAccount {username: $username})
ON CREATE SET
  a.id = $id,
  a.role = "owner",
  a.status = "active",
  a.createdAt = $now
SET
  a.name = $name,
  a.email = $email,
  a.passwordHash = $passwordHash,
  a.updatedAt = $now
WITH a
OPTIONAL MATCH (legacy:GMRAccount {email: "owner@gmr.local"})
WHERE coalesce(legacy.username, "") <> $username
SET legacy.status = "disabled", legacy.updatedAt = $now
RETURN a.id AS id, coalesce(a.email, "") AS email, coalesce(a.username, "") AS username, coalesce(a.name, "") AS name, coalesce(a.role, "owner") AS role,
  coalesce(a.status, "active") AS status, a.createdAt AS createdAt, a.updatedAt AS updatedAt
`, map[string]any{
			"id":           uuid.NewString(),
			"username":     username,
			"email":        email,
			"name":         name,
			"passwordHash": strings.TrimSpace(passwordHash),
			"now":          now,
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return accountFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return Account{}, err
	}
	return result.(Account), nil
}

func (s *MemgraphStore) VerifyAccountPassword(ctx context.Context, login string, passwordHash string) (Account, bool, error) {
	login = strings.ToLower(strings.TrimSpace(login))
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:GMRAccount {passwordHash: $passwordHash})
WHERE (a.username = $login OR a.email = $login)
  AND coalesce(a.status, "active") = "active"
RETURN a.id AS id, coalesce(a.email, "") AS email, coalesce(a.username, "") AS username, coalesce(a.name, "") AS name, coalesce(a.role, "owner") AS role,
  coalesce(a.status, "active") AS status, a.createdAt AS createdAt, a.updatedAt AS updatedAt
`, map[string]any{
			"login":        login,
			"passwordHash": strings.TrimSpace(passwordHash),
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return accountFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return Account{}, false, err
	}
	if result == nil {
		return Account{}, false, nil
	}
	return result.(Account), true, nil
}

func (s *MemgraphStore) CreateAccountSession(ctx context.Context, accountID string, tokenHash string, expiresAt string) (AccountSession, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:GMRAccount {id: $accountID})
CREATE (s:GMRAccountSession {
  id: $id,
  accountId: $accountID,
  tokenHash: $tokenHash,
  expiresAt: $expiresAt,
  createdAt: $now
})
MERGE (a)-[:HAS_GMR_SESSION]->(s)
RETURN s.id AS id, s.accountId AS accountId, s.tokenHash AS tokenHash, s.expiresAt AS expiresAt, s.createdAt AS createdAt
`, map[string]any{
			"id":        uuid.NewString(),
			"accountID": strings.TrimSpace(accountID),
			"tokenHash": strings.TrimSpace(tokenHash),
			"expiresAt": strings.TrimSpace(expiresAt),
			"now":       now,
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return accountSessionFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("account not found")
	})
	if err != nil {
		return AccountSession{}, err
	}
	return result.(AccountSession), nil
}

func (s *MemgraphStore) GetAccountBySession(ctx context.Context, tokenHash string) (Account, bool, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:GMRAccount)-[:HAS_GMR_SESSION]->(s:GMRAccountSession {tokenHash: $tokenHash})
WHERE coalesce(a.status, "active") = "active" AND s.expiresAt > $now
RETURN a.id AS id, coalesce(a.email, "") AS email, coalesce(a.username, "") AS username, coalesce(a.name, "") AS name, coalesce(a.role, "owner") AS role,
  coalesce(a.status, "active") AS status, a.createdAt AS createdAt, a.updatedAt AS updatedAt
`, map[string]any{"tokenHash": strings.TrimSpace(tokenHash), "now": now})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return accountFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return Account{}, false, err
	}
	if result == nil {
		return Account{}, false, nil
	}
	return result.(Account), true, nil
}

func (s *MemgraphStore) DeleteAccountSession(ctx context.Context, tokenHash string) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
MATCH (s:GMRAccountSession {tokenHash: $tokenHash})
DETACH DELETE s
`, map[string]any{"tokenHash": strings.TrimSpace(tokenHash)})
		return nil, err
	})
	return err
}

func (s *MemgraphStore) CreateApp(ctx context.Context, input AppInput) (App, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return App{}, errors.New("app name is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"id":                    uuid.NewString(),
		"name":                  name,
		"environment":           normalizeEnvironment(input.Environment),
		"allowedChains":         normalizeInt64Slice(input.AllowedChains),
		"allowedSolanaNetworks": normalizeStringSlice(input.AllowedSolanaNetworks),
		"allowedContracts":      normalizeStringSlice(input.AllowedContracts),
		"gasFreeEnabled":        input.GasFreeEnabled,
		"tradingFeeBps":         defaultTradingFeeBps(input.TradingFeeBps),
		"rateLimit":             normalizeRateLimit(input.RateLimitPerMin),
		"now":                   now,
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
CREATE (a:EngineApp {
  id: $id,
  name: $name,
  environment: $environment,
  status: "active",
  allowedChains: $allowedChains,
  allowedSolanaNetworks: $allowedSolanaNetworks,
  allowedContracts: $allowedContracts,
  gasFreeEnabled: $gasFreeEnabled,
  tradingFeeBps: $tradingFeeBps,
  rateLimitPerMinute: $rateLimit,
  createdAt: $now,
  updatedAt: $now
})
RETURN a.id AS id, a.name AS name, a.environment AS environment, a.status AS status,
  a.allowedChains AS allowedChains, coalesce(a.allowedSolanaNetworks, []) AS allowedSolanaNetworks, a.allowedContracts AS allowedContracts,
  coalesce(a.gasFreeEnabled, false) AS gasFreeEnabled, coalesce(a.tradingFeeBps, 50) AS tradingFeeBps,
  a.rateLimitPerMinute AS rateLimitPerMinute, coalesce(a.webhookUrl, "") AS webhookUrl, a.createdAt AS createdAt, a.updatedAt AS updatedAt
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return appFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return App{}, err
	}
	return result.(App), nil
}

func (s *MemgraphStore) CreateAccountApp(ctx context.Context, accountID string, input AppInput) (App, error) {
	app, err := s.createAppWithAccount(ctx, strings.TrimSpace(accountID), input)
	if err != nil {
		return App{}, err
	}
	return app, nil
}

func (s *MemgraphStore) createAppWithAccount(ctx context.Context, accountID string, input AppInput) (App, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return App{}, errors.New("app name is required")
	}
	environment := normalizeEnvironment(input.Environment)
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"id":                    uuid.NewString(),
		"accountID":             accountID,
		"name":                  name,
		"environment":           environment,
		"allowedChains":         normalizeInt64Slice(input.AllowedChains),
		"allowedSolanaNetworks": normalizeStringSlice(input.AllowedSolanaNetworks),
		"allowedContracts":      normalizeStringSlice(input.AllowedContracts),
		"gasFreeEnabled":        input.GasFreeEnabled,
		"tradingFeeBps":         defaultTradingFeeBps(input.TradingFeeBps),
		"rateLimit":             normalizeRateLimit(input.RateLimitPerMin),
		"now":                   now,
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (owner:GMRAccount {id: $accountID})
CREATE (a:EngineApp {
  id: $id,
  name: $name,
  environment: $environment,
  status: "active",
  allowedChains: $allowedChains,
  allowedSolanaNetworks: $allowedSolanaNetworks,
  allowedContracts: $allowedContracts,
  gasFreeEnabled: $gasFreeEnabled,
  tradingFeeBps: $tradingFeeBps,
  rateLimitPerMinute: $rateLimit,
  createdAt: $now,
  updatedAt: $now
})
MERGE (owner)-[:OWNS_ENGINE_APP]->(a)
RETURN a.id AS id, a.name AS name, a.environment AS environment, a.status AS status,
  a.allowedChains AS allowedChains, coalesce(a.allowedSolanaNetworks, []) AS allowedSolanaNetworks, a.allowedContracts AS allowedContracts,
  coalesce(a.gasFreeEnabled, false) AS gasFreeEnabled, coalesce(a.tradingFeeBps, 50) AS tradingFeeBps,
  a.rateLimitPerMinute AS rateLimitPerMinute, coalesce(a.webhookUrl, "") AS webhookUrl, a.createdAt AS createdAt, a.updatedAt AS updatedAt
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return appFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("account not found")
	})
	if err != nil {
		return App{}, err
	}
	return result.(App), nil
}

func (s *MemgraphStore) ListApps(ctx context.Context) ([]App, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:EngineApp)
RETURN a.id AS id, a.name AS name, a.environment AS environment, coalesce(a.status, "active") AS status,
  coalesce(a.allowedChains, []) AS allowedChains, coalesce(a.allowedSolanaNetworks, []) AS allowedSolanaNetworks, coalesce(a.allowedContracts, []) AS allowedContracts,
  coalesce(a.gasFreeEnabled, false) AS gasFreeEnabled, coalesce(a.tradingFeeBps, 50) AS tradingFeeBps,
  coalesce(a.rateLimitPerMinute, 60) AS rateLimitPerMinute, coalesce(a.webhookUrl, "") AS webhookUrl, a.createdAt AS createdAt, a.updatedAt AS updatedAt
ORDER BY a.createdAt DESC
`, nil)
		if err != nil {
			return nil, err
		}
		apps := []App{}
		for rows.Next(ctx) {
			apps = append(apps, appFromRecord(rows.Record()))
		}
		return apps, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]App), nil
}

func (s *MemgraphStore) ListAccountApps(ctx context.Context, accountID string) ([]App, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:GMRAccount {id: $accountID})-[:OWNS_ENGINE_APP]->(a:EngineApp)
WHERE coalesce(a.status, "active") <> "archived"
RETURN a.id AS id, a.name AS name, a.environment AS environment, coalesce(a.status, "active") AS status,
  coalesce(a.allowedChains, []) AS allowedChains, coalesce(a.allowedSolanaNetworks, []) AS allowedSolanaNetworks, coalesce(a.allowedContracts, []) AS allowedContracts,
  coalesce(a.gasFreeEnabled, false) AS gasFreeEnabled, coalesce(a.tradingFeeBps, 50) AS tradingFeeBps,
  coalesce(a.rateLimitPerMinute, 60) AS rateLimitPerMinute, coalesce(a.webhookUrl, "") AS webhookUrl, a.createdAt AS createdAt, a.updatedAt AS updatedAt
ORDER BY a.createdAt DESC
`, map[string]any{"accountID": strings.TrimSpace(accountID)})
		if err != nil {
			return nil, err
		}
		apps := []App{}
		for rows.Next(ctx) {
			apps = append(apps, appFromRecord(rows.Record()))
		}
		return apps, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]App), nil
}

func (s *MemgraphStore) GetApp(ctx context.Context, id string) (App, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:EngineApp {id: $id})
RETURN a.id AS id, a.name AS name, a.environment AS environment, coalesce(a.status, "active") AS status,
  coalesce(a.allowedChains, []) AS allowedChains, coalesce(a.allowedSolanaNetworks, []) AS allowedSolanaNetworks, coalesce(a.allowedContracts, []) AS allowedContracts,
  coalesce(a.gasFreeEnabled, false) AS gasFreeEnabled, coalesce(a.tradingFeeBps, 50) AS tradingFeeBps,
  coalesce(a.rateLimitPerMinute, 60) AS rateLimitPerMinute, coalesce(a.webhookUrl, "") AS webhookUrl, a.createdAt AS createdAt, a.updatedAt AS updatedAt
`, map[string]any{"id": strings.TrimSpace(id)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return appFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return App{}, false, err
	}
	if result == nil {
		return App{}, false, nil
	}
	return result.(App), true, nil
}

func (s *MemgraphStore) GetAccountApp(ctx context.Context, accountID string, id string) (App, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:GMRAccount {id: $accountID})-[:OWNS_ENGINE_APP]->(a:EngineApp {id: $id})
WHERE coalesce(a.status, "active") <> "archived"
RETURN a.id AS id, a.name AS name, a.environment AS environment, coalesce(a.status, "active") AS status,
  coalesce(a.allowedChains, []) AS allowedChains, coalesce(a.allowedSolanaNetworks, []) AS allowedSolanaNetworks, coalesce(a.allowedContracts, []) AS allowedContracts,
  coalesce(a.gasFreeEnabled, false) AS gasFreeEnabled, coalesce(a.tradingFeeBps, 50) AS tradingFeeBps,
  coalesce(a.rateLimitPerMinute, 60) AS rateLimitPerMinute, coalesce(a.webhookUrl, "") AS webhookUrl, a.createdAt AS createdAt, a.updatedAt AS updatedAt
`, map[string]any{"accountID": strings.TrimSpace(accountID), "id": strings.TrimSpace(id)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return appFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return App{}, false, err
	}
	if result == nil {
		return App{}, false, nil
	}
	return result.(App), true, nil
}

func (s *MemgraphStore) UpdateAccountApp(ctx context.Context, accountID string, id string, input AppInput) (App, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return App{}, errors.New("project name is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:GMRAccount {id: $accountID})-[:OWNS_ENGINE_APP]->(a:EngineApp {id: $id})
WHERE coalesce(a.status, "active") <> "archived"
SET a.name = $name,
  a.environment = $environment,
  a.allowedChains = $allowedChains,
  a.allowedSolanaNetworks = $allowedSolanaNetworks,
  a.allowedContracts = $allowedContracts,
  a.gasFreeEnabled = $gasFreeEnabled,
  a.rateLimitPerMinute = $rateLimit,
  a.webhookUrl = $webhookUrl,
  a.updatedAt = $now
RETURN a.id AS id, a.name AS name, a.environment AS environment, coalesce(a.status, "active") AS status,
  coalesce(a.allowedChains, []) AS allowedChains, coalesce(a.allowedSolanaNetworks, []) AS allowedSolanaNetworks, coalesce(a.allowedContracts, []) AS allowedContracts,
  coalesce(a.gasFreeEnabled, false) AS gasFreeEnabled, coalesce(a.tradingFeeBps, 50) AS tradingFeeBps,
  coalesce(a.rateLimitPerMinute, 60) AS rateLimitPerMinute, coalesce(a.webhookUrl, "") AS webhookUrl, a.createdAt AS createdAt, a.updatedAt AS updatedAt
`, map[string]any{
			"accountID":             strings.TrimSpace(accountID),
			"id":                    strings.TrimSpace(id),
			"name":                  name,
			"environment":           normalizeEnvironment(input.Environment),
			"allowedChains":         normalizeInt64Slice(input.AllowedChains),
			"allowedSolanaNetworks": normalizeStringSlice(input.AllowedSolanaNetworks),
			"allowedContracts":      normalizeStringSlice(input.AllowedContracts),
			"gasFreeEnabled":        input.GasFreeEnabled,
			"rateLimit":             normalizeRateLimit(input.RateLimitPerMin),
			"webhookUrl":            strings.TrimSpace(input.WebhookURL),
			"now":                   now,
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return appFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("project not found")
	})
	if err != nil {
		return App{}, err
	}
	return result.(App), nil
}

func (s *MemgraphStore) UpdateAppGasFree(ctx context.Context, id string, gasFreeEnabled bool) (App, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:EngineApp {id: $id})
WHERE coalesce(a.status, "active") <> "archived"
SET a.gasFreeEnabled = $gasFreeEnabled,
  a.updatedAt = $now
RETURN a.id AS id, a.name AS name, a.environment AS environment, coalesce(a.status, "active") AS status,
  coalesce(a.allowedChains, []) AS allowedChains, coalesce(a.allowedSolanaNetworks, []) AS allowedSolanaNetworks, coalesce(a.allowedContracts, []) AS allowedContracts,
  coalesce(a.gasFreeEnabled, false) AS gasFreeEnabled, coalesce(a.tradingFeeBps, 50) AS tradingFeeBps,
  coalesce(a.rateLimitPerMinute, 60) AS rateLimitPerMinute, coalesce(a.webhookUrl, "") AS webhookUrl, a.createdAt AS createdAt, a.updatedAt AS updatedAt
`, map[string]any{
			"id":             strings.TrimSpace(id),
			"gasFreeEnabled": gasFreeEnabled,
			"now":            now,
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return appFromRecord(rows.Record()), rows.Err()
		}
		return nil, errors.New("project not found")
	})
	if err != nil {
		return App{}, err
	}
	return result.(App), nil
}

func (s *MemgraphStore) UpdateAppTradingFee(ctx context.Context, id string, tradingFeeBps int64) (App, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:EngineApp {id: $id})
WHERE coalesce(a.status, "active") <> "archived"
SET a.tradingFeeBps = $tradingFeeBps,
  a.updatedAt = $now
RETURN a.id AS id, a.name AS name, a.environment AS environment, coalesce(a.status, "active") AS status,
  coalesce(a.allowedChains, []) AS allowedChains, coalesce(a.allowedSolanaNetworks, []) AS allowedSolanaNetworks, coalesce(a.allowedContracts, []) AS allowedContracts,
  coalesce(a.gasFreeEnabled, false) AS gasFreeEnabled, coalesce(a.tradingFeeBps, 50) AS tradingFeeBps,
  coalesce(a.rateLimitPerMinute, 60) AS rateLimitPerMinute, coalesce(a.webhookUrl, "") AS webhookUrl, a.createdAt AS createdAt, a.updatedAt AS updatedAt
`, map[string]any{
			"id":            strings.TrimSpace(id),
			"tradingFeeBps": normalizeTradingFeeBps(tradingFeeBps),
			"now":           now,
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return appFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("project not found")
	})
	if err != nil {
		return App{}, err
	}
	return result.(App), nil
}

func (s *MemgraphStore) ArchiveAccountApp(ctx context.Context, accountID string, id string) (App, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:GMRAccount {id: $accountID})-[:OWNS_ENGINE_APP]->(a:EngineApp {id: $id})
SET a.status = "archived", a.updatedAt = $now
RETURN a.id AS id, a.name AS name, a.environment AS environment, coalesce(a.status, "active") AS status,
  coalesce(a.allowedChains, []) AS allowedChains, coalesce(a.allowedSolanaNetworks, []) AS allowedSolanaNetworks, coalesce(a.allowedContracts, []) AS allowedContracts,
  coalesce(a.gasFreeEnabled, false) AS gasFreeEnabled, coalesce(a.tradingFeeBps, 50) AS tradingFeeBps,
  coalesce(a.rateLimitPerMinute, 60) AS rateLimitPerMinute, coalesce(a.webhookUrl, "") AS webhookUrl, a.createdAt AS createdAt, a.updatedAt AS updatedAt
`, map[string]any{"accountID": strings.TrimSpace(accountID), "id": strings.TrimSpace(id), "now": now})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return appFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("project not found")
	})
	if err != nil {
		return App{}, err
	}
	return result.(App), nil
}

func (s *MemgraphStore) CreateImportedContract(ctx context.Context, appID string, input ImportedContractInput) (ImportedContract, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return ImportedContract{}, errors.New("contract name is required")
	}
	address := strings.ToLower(strings.TrimSpace(input.ContractAddress))
	if address == "" {
		return ImportedContract{}, errors.New("contract address is required")
	}
	if input.ChainID <= 0 {
		return ImportedContract{}, errors.New("chainId is required")
	}
	if strings.TrimSpace(input.ABI) == "" {
		return ImportedContract{}, errors.New("abi is required")
	}
	contractType := strings.TrimSpace(input.ContractType)
	if contractType == "" {
		contractType = "custom"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:EngineApp {id: $appID})
CREATE (c:ImportedContract {
  id: $id, appId: $appID, name: $name, contractType: $contractType,
  contractAddress: $contractAddress, chainId: $chainID, abi: $abi,
  description: $description, status: "active", createdAt: $now, updatedAt: $now
})
MERGE (a)-[:HAS_IMPORTED_CONTRACT]->(c)
RETURN c.id AS id, c.appId AS appId, c.name AS name, c.contractType AS contractType,
  c.contractAddress AS contractAddress, c.chainId AS chainId, c.abi AS abi,
  coalesce(c.description, "") AS description, coalesce(c.status, "active") AS status,
  c.createdAt AS createdAt, c.updatedAt AS updatedAt
`, map[string]any{
			"id":              uuid.NewString(),
			"appID":           strings.TrimSpace(appID),
			"name":            name,
			"contractType":    contractType,
			"contractAddress": address,
			"chainID":         input.ChainID,
			"abi":             strings.TrimSpace(input.ABI),
			"description":     strings.TrimSpace(input.Description),
			"now":             now,
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return importedContractFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("engine app not found")
	})
	if err != nil {
		return ImportedContract{}, err
	}
	return result.(ImportedContract), nil
}

func (s *MemgraphStore) ListImportedContracts(ctx context.Context, appID string, limit int64) ([]ImportedContract, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp {id: $appID})-[:HAS_IMPORTED_CONTRACT]->(c:ImportedContract)
WHERE coalesce(c.status, "active") <> "deleted"
RETURN c.id AS id, c.appId AS appId, c.name AS name, c.contractType AS contractType,
  c.contractAddress AS contractAddress, c.chainId AS chainId, c.abi AS abi,
  coalesce(c.description, "") AS description, coalesce(c.status, "active") AS status,
  c.createdAt AS createdAt, c.updatedAt AS updatedAt
ORDER BY c.createdAt DESC
LIMIT $limit
`, map[string]any{"appID": strings.TrimSpace(appID), "limit": limit})
		if err != nil {
			return nil, err
		}
		contracts := []ImportedContract{}
		for rows.Next(ctx) {
			contracts = append(contracts, importedContractFromRecord(rows.Record()))
		}
		return contracts, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]ImportedContract), nil
}

func (s *MemgraphStore) DeleteImportedContract(ctx context.Context, accountID string, contractID string) (ImportedContract, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:GMRAccount {id: $accountID})-[:OWNS_ENGINE_APP]->(:EngineApp)-[:HAS_IMPORTED_CONTRACT]->(c:ImportedContract {id: $id})
SET c.status = "deleted", c.updatedAt = $now
RETURN c.id AS id, c.appId AS appId, c.name AS name, c.contractType AS contractType,
  c.contractAddress AS contractAddress, c.chainId AS chainId, c.abi AS abi,
  coalesce(c.description, "") AS description, coalesce(c.status, "active") AS status,
  c.createdAt AS createdAt, c.updatedAt AS updatedAt
`, map[string]any{"accountID": strings.TrimSpace(accountID), "id": strings.TrimSpace(contractID), "now": now})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return importedContractFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("imported contract not found")
	})
	if err != nil {
		return ImportedContract{}, err
	}
	return result.(ImportedContract), nil
}

func (s *MemgraphStore) CreateProjectWallet(ctx context.Context, appID string, input ProjectWalletInput) (ProjectWallet, error) {
	address := strings.ToLower(strings.TrimSpace(input.Address))
	if address == "" {
		return ProjectWallet{}, errors.New("wallet address is required")
	}
	if strings.TrimSpace(input.EncryptedPrivateKey) == "" {
		return ProjectWallet{}, errors.New("encrypted private key is required")
	}
	if !strings.HasPrefix(strings.TrimSpace(input.EncryptedPrivateKey), "gmr-vault:v1:") {
		return ProjectWallet{}, errors.New("project wallets must be backed by GMR Vault")
	}
	walletType := strings.TrimSpace(input.WalletType)
	if walletType == "" {
		walletType = "server_admin"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:EngineApp {id: $appID})
CREATE (w:ProjectWallet {
  id: $id,
  appId: $appID,
  address: $address,
  encryptedPrivateKey: $encryptedPrivateKey,
  walletType: $walletType,
  status: "active",
  isDefaultAdmin: $isDefaultAdmin,
  createdAt: $now,
  updatedAt: $now
})
MERGE (a)-[:HAS_PROJECT_WALLET]->(w)
RETURN w.id AS id, w.appId AS appId, w.address AS address, w.walletType AS walletType,
  coalesce(w.status, "active") AS status, coalesce(w.isDefaultAdmin, false) AS isDefaultAdmin,
  w.createdAt AS createdAt, w.updatedAt AS updatedAt
`, map[string]any{
			"id":                  uuid.NewString(),
			"appID":               strings.TrimSpace(appID),
			"address":             address,
			"encryptedPrivateKey": strings.TrimSpace(input.EncryptedPrivateKey),
			"isDefaultAdmin":      input.IsDefaultAdmin,
			"walletType":          walletType,
			"now":                 now,
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return projectWalletFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("project not found")
	})
	if err != nil {
		return ProjectWallet{}, err
	}
	return result.(ProjectWallet), nil
}

func (s *MemgraphStore) ListProjectWallets(ctx context.Context, appID string, limit int64) ([]ProjectWallet, error) {
	if limit <= 0 || limit > 250 {
		limit = 100
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp {id: $appID})-[:HAS_PROJECT_WALLET]->(w:ProjectWallet)
WHERE coalesce(w.status, "active") <> "deleted"
RETURN w.id AS id, w.appId AS appId, w.address AS address, w.walletType AS walletType,
  coalesce(w.status, "active") AS status, coalesce(w.isDefaultAdmin, false) AS isDefaultAdmin,
  w.createdAt AS createdAt, w.updatedAt AS updatedAt
ORDER BY coalesce(w.isDefaultAdmin, false) DESC, w.createdAt ASC
LIMIT $limit
`, map[string]any{
			"appID": strings.TrimSpace(appID),
			"limit": limit,
		})
		if err != nil {
			return nil, err
		}
		wallets := []ProjectWallet{}
		for rows.Next(ctx) {
			wallets = append(wallets, projectWalletFromRecord(rows.Record()))
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return wallets, nil
	})
	if err != nil {
		return nil, err
	}
	return result.([]ProjectWallet), nil
}

func (s *MemgraphStore) UpsertUserWallet(ctx context.Context, appID string, input UserWalletInput) (UserWallet, error) {
	address := strings.ToLower(strings.TrimSpace(input.Address))
	if address == "" {
		return UserWallet{}, errors.New("wallet address is required")
	}
	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		userID = address
	}
	walletCustody := strings.ToLower(strings.TrimSpace(input.WalletCustody))
	if walletCustody == "" {
		walletCustody = "external"
	}
	if walletCustody != "managed" && walletCustody != "external" {
		return UserWallet{}, errors.New("walletCustody must be managed or external")
	}
	walletType := strings.TrimSpace(input.WalletType)
	if walletType == "" {
		walletType = walletCustody
	}
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:EngineApp {id: $appID})
MERGE (w:EngineUserWallet {appId: $appID, address: $address})
ON CREATE SET
  w.id = $id,
  w.createdAt = $now,
  w.status = "active"
SET
  w.userId = $userID,
  w.authProvider = $authProvider,
  w.email = $email,
  w.metadata = $metadata,
  w.vaultWalletRef = CASE WHEN $vaultWalletRef = "" THEN coalesce(w.vaultWalletRef, "") ELSE $vaultWalletRef END,
  w.walletCustody = $walletCustody,
  w.walletType = $walletType,
  w.updatedAt = $now,
  w.lastSeenAt = $now
MERGE (a)-[:HAS_USER_WALLET]->(w)
RETURN w.id AS id, w.appId AS appId, w.userId AS userId, w.address AS address,
  coalesce(w.authProvider, "") AS authProvider, coalesce(w.email, "") AS email,
  coalesce(w.status, "active") AS status, coalesce(w.metadata, "") AS metadata,
  coalesce(w.walletCustody, "external") AS walletCustody, coalesce(w.walletType, "external") AS walletType,
  w.createdAt AS createdAt, w.updatedAt AS updatedAt, w.lastSeenAt AS lastSeenAt
`, map[string]any{
			"address":        address,
			"appID":          strings.TrimSpace(appID),
			"authProvider":   strings.TrimSpace(input.AuthProvider),
			"email":          strings.TrimSpace(input.Email),
			"id":             uuid.NewString(),
			"metadata":       strings.TrimSpace(input.Metadata),
			"now":            now,
			"userID":         userID,
			"vaultWalletRef": strings.TrimSpace(input.VaultWalletRef),
			"walletCustody":  walletCustody,
			"walletType":     walletType,
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return userWalletFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("project not found")
	})
	if err != nil {
		return UserWallet{}, err
	}
	return result.(UserWallet), nil
}

func (s *MemgraphStore) GetActiveManagedUserWallet(ctx context.Context, appID string, authProvider string, userID string) (UserWallet, bool, error) {
	appID = strings.TrimSpace(appID)
	authProvider = strings.TrimSpace(authProvider)
	userID = strings.TrimSpace(userID)
	if appID == "" || userID == "" {
		return UserWallet{}, false, nil
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp {id: $appID})-[:HAS_USER_WALLET]->(w:EngineUserWallet)
WHERE coalesce(w.status, "active") = "active"
  AND coalesce(w.walletCustody, "") = "managed"
  AND w.userId = $userID
  AND ($authProvider = "" OR coalesce(w.authProvider, "") = $authProvider)
RETURN w.id AS id, w.appId AS appId, w.userId AS userId, w.address AS address,
  coalesce(w.authProvider, "") AS authProvider, coalesce(w.email, "") AS email,
  coalesce(w.status, "active") AS status, coalesce(w.metadata, "") AS metadata,
  coalesce(w.walletCustody, "external") AS walletCustody, coalesce(w.walletType, "external") AS walletType,
  w.createdAt AS createdAt, w.updatedAt AS updatedAt, coalesce(w.lastSeenAt, "") AS lastSeenAt
ORDER BY coalesce(w.lastSeenAt, w.updatedAt, w.createdAt) DESC
LIMIT 1
`, map[string]any{
			"appID":        appID,
			"authProvider": authProvider,
			"userID":       userID,
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return userWalletFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	})
	if err != nil {
		return UserWallet{}, false, err
	}
	if result == nil {
		return UserWallet{}, false, nil
	}
	return result.(UserWallet), true, nil
}

func (s *MemgraphStore) GetActiveManagedUserWalletSecretByAddress(ctx context.Context, appID string, address string) (UserWalletSecret, bool, error) {
	appID = strings.TrimSpace(appID)
	address = strings.ToLower(strings.TrimSpace(address))
	if appID == "" || address == "" {
		return UserWalletSecret{}, false, nil
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp {id: $appID})-[:HAS_USER_WALLET]->(w:EngineUserWallet {address: $address})
WHERE coalesce(w.status, "active") = "active"
  AND coalesce(w.walletCustody, "") = "managed"
  AND coalesce(w.vaultWalletRef, "") <> ""
RETURN w.id AS id, w.appId AS appId, w.userId AS userId, w.address AS address,
  coalesce(w.authProvider, "") AS authProvider, coalesce(w.email, "") AS email,
  coalesce(w.status, "active") AS status, coalesce(w.metadata, "") AS metadata,
  coalesce(w.walletCustody, "managed") AS walletCustody, coalesce(w.walletType, "managed_user") AS walletType,
  coalesce(w.vaultWalletRef, "") AS vaultWalletRef,
  w.createdAt AS createdAt, w.updatedAt AS updatedAt, coalesce(w.lastSeenAt, "") AS lastSeenAt
LIMIT 1
`, map[string]any{"appID": appID, "address": address})
		if err != nil {
			return nil, err
		}
		if !rows.Next(ctx) {
			return nil, rows.Err()
		}
		record := rows.Record()
		wallet := userWalletFromRecord(record)
		return UserWalletSecret{
			UserWallet:     wallet,
			VaultWalletRef: stringValue(record, "vaultWalletRef"),
		}, rows.Err()
	})
	if err != nil {
		return UserWalletSecret{}, false, err
	}
	if result == nil {
		return UserWalletSecret{}, false, nil
	}
	return result.(UserWalletSecret), true, nil
}

func (s *MemgraphStore) ListUserWallets(ctx context.Context, appID string, limit int64) ([]UserWallet, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp {id: $appID})-[:HAS_USER_WALLET]->(w:EngineUserWallet)
WHERE coalesce(w.status, "active") <> "deleted"
RETURN w.id AS id, w.appId AS appId, w.userId AS userId, w.address AS address,
  coalesce(w.authProvider, "") AS authProvider, coalesce(w.email, "") AS email,
  coalesce(w.status, "active") AS status, coalesce(w.metadata, "") AS metadata,
  coalesce(w.walletCustody, "external") AS walletCustody, coalesce(w.walletType, "external") AS walletType,
  w.createdAt AS createdAt, w.updatedAt AS updatedAt, coalesce(w.lastSeenAt, "") AS lastSeenAt
ORDER BY coalesce(w.lastSeenAt, w.updatedAt, w.createdAt) DESC
LIMIT $limit
`, map[string]any{
			"appID": strings.TrimSpace(appID),
			"limit": limit,
		})
		if err != nil {
			return nil, err
		}
		wallets := []UserWallet{}
		for rows.Next(ctx) {
			wallets = append(wallets, userWalletFromRecord(rows.Record()))
		}
		return wallets, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]UserWallet), nil
}

func (s *MemgraphStore) DeleteUserWallet(ctx context.Context, appID string, id string) (UserWallet, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp {id: $appID})-[:HAS_USER_WALLET]->(w:EngineUserWallet {id: $id})
SET w.status = "deleted", w.updatedAt = $now
RETURN w.id AS id, w.appId AS appId, w.userId AS userId, w.address AS address,
  coalesce(w.authProvider, "") AS authProvider, coalesce(w.email, "") AS email,
  coalesce(w.status, "active") AS status, coalesce(w.metadata, "") AS metadata,
  coalesce(w.walletCustody, "external") AS walletCustody, coalesce(w.walletType, "external") AS walletType,
  w.createdAt AS createdAt, w.updatedAt AS updatedAt, coalesce(w.lastSeenAt, "") AS lastSeenAt
`, map[string]any{
			"appID": strings.TrimSpace(appID),
			"id":    strings.TrimSpace(id),
			"now":   now,
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return userWalletFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("user wallet not found")
	})
	if err != nil {
		return UserWallet{}, err
	}
	return result.(UserWallet), nil
}

func (s *MemgraphStore) GetProjectDefaultAdminWallet(ctx context.Context, appID string) (ProjectWallet, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp {id: $appID})-[:HAS_PROJECT_WALLET]->(w:ProjectWallet)
WHERE coalesce(w.isDefaultAdmin, false) = true AND coalesce(w.status, "active") = "active"
RETURN w.id AS id, w.appId AS appId, w.address AS address, w.walletType AS walletType,
  coalesce(w.status, "active") AS status, coalesce(w.isDefaultAdmin, false) AS isDefaultAdmin,
  w.createdAt AS createdAt, w.updatedAt AS updatedAt
ORDER BY w.createdAt ASC
LIMIT 1
`, map[string]any{"appID": strings.TrimSpace(appID)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return projectWalletFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return ProjectWallet{}, false, err
	}
	if result == nil {
		return ProjectWallet{}, false, nil
	}
	return result.(ProjectWallet), true, nil
}

func (s *MemgraphStore) GetProjectDefaultAdminWalletSecret(ctx context.Context, appID string) (ProjectWalletSecret, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp {id: $appID})-[:HAS_PROJECT_WALLET]->(w:ProjectWallet)
WHERE coalesce(w.isDefaultAdmin, false) = true AND coalesce(w.status, "active") = "active"
RETURN w.id AS id, w.appId AS appId, w.address AS address, w.walletType AS walletType,
  coalesce(w.status, "active") AS status, coalesce(w.isDefaultAdmin, false) AS isDefaultAdmin,
  coalesce(w.encryptedPrivateKey, "") AS encryptedPrivateKey, w.createdAt AS createdAt, w.updatedAt AS updatedAt
ORDER BY w.createdAt ASC
LIMIT 1
`, map[string]any{"appID": strings.TrimSpace(appID)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return projectWalletSecretFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return ProjectWalletSecret{}, false, err
	}
	if result == nil {
		return ProjectWalletSecret{}, false, nil
	}
	return result.(ProjectWalletSecret), true, nil
}

func (s *MemgraphStore) CreateAPIKey(ctx context.Context, appID string, input APIKeyInput, prefix string, secretHash string) (APIKey, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return APIKey{}, errors.New("key name is required")
	}
	scopes, err := normalizeScopes(input.Scopes)
	if err != nil {
		return APIKey{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"id":             uuid.NewString(),
		"appID":          strings.TrimSpace(appID),
		"name":           name,
		"prefix":         strings.TrimSpace(prefix),
		"secretHash":     strings.TrimSpace(secretHash),
		"scopes":         scopes,
		"allowedOrigins": normalizeStringSlice(input.AllowedOrigins),
		"allowedIPs":     normalizeStringSlice(input.AllowedIPs),
		"expiresAt":      strings.TrimSpace(input.ExpiresAt),
		"now":            now,
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:EngineApp {id: $appID})
CREATE (k:EngineAPIKey {
  id: $id, appId: $appID, name: $name, prefix: $prefix, secretHash: $secretHash,
  scopes: $scopes, allowedOrigins: $allowedOrigins, allowedIPs: $allowedIPs,
  status: "active", expiresAt: $expiresAt, createdAt: $now
})
MERGE (a)-[:HAS_ENGINE_KEY]->(k)
RETURN k.id AS id, k.appId AS appId, a.name AS appName, k.name AS name, k.prefix AS prefix,
  k.scopes AS scopes, coalesce(k.allowedOrigins, []) AS allowedOrigins, coalesce(k.allowedIPs, []) AS allowedIPs,
  coalesce(k.status, "active") AS status, coalesce(k.expiresAt, "") AS expiresAt,
  coalesce(k.lastUsedAt, "") AS lastUsedAt, k.createdAt AS createdAt, coalesce(k.revokedAt, "") AS revokedAt
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return apiKeyFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("engine app not found")
	})
	if err != nil {
		return APIKey{}, err
	}
	return result.(APIKey), nil
}

func (s *MemgraphStore) ListAPIKeys(ctx context.Context, appID string) ([]APIKey, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:EngineApp {id: $appID})-[:HAS_ENGINE_KEY]->(k:EngineAPIKey)
RETURN k.id AS id, k.appId AS appId, a.name AS appName, k.name AS name, k.prefix AS prefix,
  k.scopes AS scopes, coalesce(k.allowedOrigins, []) AS allowedOrigins, coalesce(k.allowedIPs, []) AS allowedIPs,
  coalesce(k.status, "active") AS status, coalesce(k.expiresAt, "") AS expiresAt,
  coalesce(k.lastUsedAt, "") AS lastUsedAt, k.createdAt AS createdAt, coalesce(k.revokedAt, "") AS revokedAt
ORDER BY k.createdAt DESC
`, map[string]any{"appID": strings.TrimSpace(appID)})
		if err != nil {
			return nil, err
		}
		keys := []APIKey{}
		for rows.Next(ctx) {
			keys = append(keys, apiKeyFromRecord(rows.Record()))
		}
		return keys, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]APIKey), nil
}

func (s *MemgraphStore) GetAccountAPIKey(ctx context.Context, accountID string, keyID string) (APIKey, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:GMRAccount {id: $accountID})-[:OWNS_ENGINE_APP]->(a:EngineApp)-[:HAS_ENGINE_KEY]->(k:EngineAPIKey {id: $keyID})
RETURN k.id AS id, k.appId AS appId, a.name AS appName, k.name AS name, k.prefix AS prefix,
  k.scopes AS scopes, coalesce(k.allowedOrigins, []) AS allowedOrigins, coalesce(k.allowedIPs, []) AS allowedIPs,
  coalesce(k.status, "active") AS status, coalesce(k.expiresAt, "") AS expiresAt,
  coalesce(k.lastUsedAt, "") AS lastUsedAt, k.createdAt AS createdAt, coalesce(k.revokedAt, "") AS revokedAt
`, map[string]any{"accountID": strings.TrimSpace(accountID), "keyID": strings.TrimSpace(keyID)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return apiKeyFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return APIKey{}, false, err
	}
	if result == nil {
		return APIKey{}, false, nil
	}
	return result.(APIKey), true, nil
}

func (s *MemgraphStore) VerifyAPIKey(ctx context.Context, prefix string, secretHash string) (APIKeyVerification, bool, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:EngineApp)-[:HAS_ENGINE_KEY]->(k:EngineAPIKey {prefix: $prefix, secretHash: $secretHash})
WHERE coalesce(a.status, "active") = "active"
  AND coalesce(k.status, "active") = "active"
  AND (coalesce(k.expiresAt, "") = "" OR k.expiresAt > $now)
SET k.lastUsedAt = $now
RETURN a.id AS appId, a.name AS appName, a.environment AS appEnvironment, coalesce(a.status, "active") AS appStatus,
  coalesce(a.allowedChains, []) AS appAllowedChains,
  coalesce(a.allowedSolanaNetworks, []) AS appAllowedSolanaNetworks,
  coalesce(a.allowedContracts, []) AS appAllowedContracts,
  coalesce(a.gasFreeEnabled, false) AS appGasFreeEnabled, coalesce(a.tradingFeeBps, 50) AS appTradingFeeBps,
  coalesce(a.rateLimitPerMinute, 60) AS appRateLimitPerMinute, coalesce(a.webhookUrl, "") AS appWebhookUrl, a.createdAt AS appCreatedAt, a.updatedAt AS appUpdatedAt,
  k.id AS id, k.appId AS keyAppId, k.name AS name, k.prefix AS prefix, k.scopes AS scopes,
  coalesce(k.allowedOrigins, []) AS allowedOrigins, coalesce(k.allowedIPs, []) AS allowedIPs,
  coalesce(k.status, "active") AS status, coalesce(k.expiresAt, "") AS expiresAt,
  coalesce(k.lastUsedAt, "") AS lastUsedAt, k.createdAt AS createdAt, coalesce(k.revokedAt, "") AS revokedAt
`, map[string]any{"prefix": strings.TrimSpace(prefix), "secretHash": strings.TrimSpace(secretHash), "now": now})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return APIKeyVerification{App: appFromPrefixedRecord(rows.Record(), "app"), Key: apiKeyFromRecordWithAppID(rows.Record(), "keyAppId")}, nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return APIKeyVerification{}, false, err
	}
	if result == nil {
		return APIKeyVerification{}, false, nil
	}
	return result.(APIKeyVerification), true, nil
}

func (s *MemgraphStore) RevokeAPIKey(ctx context.Context, id string) (APIKey, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:EngineApp)-[:HAS_ENGINE_KEY]->(k:EngineAPIKey {id: $id})
SET k.status = "revoked", k.revokedAt = $now
RETURN k.id AS id, k.appId AS appId, a.name AS appName, k.name AS name, k.prefix AS prefix,
  k.scopes AS scopes, coalesce(k.allowedOrigins, []) AS allowedOrigins, coalesce(k.allowedIPs, []) AS allowedIPs,
  coalesce(k.status, "active") AS status, coalesce(k.expiresAt, "") AS expiresAt,
  coalesce(k.lastUsedAt, "") AS lastUsedAt, k.createdAt AS createdAt, coalesce(k.revokedAt, "") AS revokedAt
`, map[string]any{"id": strings.TrimSpace(id), "now": now})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return apiKeyFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("engine api key not found")
	})
	if err != nil {
		return APIKey{}, err
	}
	return result.(APIKey), nil
}

func (s *MemgraphStore) RecordAPIUsage(ctx context.Context, input APIUsageInput) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
MATCH (a:EngineApp {id: $appID})
OPTIONAL MATCH (k:EngineAPIKey {id: $keyID})
CREATE (u:EngineAPIUsage {
  id: $id, appId: $appID, keyId: $keyID, method: $method, path: $path,
  statusCode: $statusCode, ipAddress: $ipAddress, userAgent: $userAgent, createdAt: $now
})
MERGE (a)-[:HAS_ENGINE_USAGE]->(u)
FOREACH (_ IN CASE WHEN k IS NULL THEN [] ELSE [1] END | MERGE (k)-[:MADE_ENGINE_REQUEST]->(u))
`, map[string]any{
			"id":         uuid.NewString(),
			"appID":      strings.TrimSpace(input.AppID),
			"keyID":      strings.TrimSpace(input.KeyID),
			"method":     strings.TrimSpace(input.Method),
			"path":       strings.TrimSpace(input.Path),
			"statusCode": int64(input.StatusCode),
			"ipAddress":  strings.TrimSpace(input.IPAddress),
			"userAgent":  strings.TrimSpace(input.UserAgent),
			"now":        time.Now().UTC().Format(time.RFC3339),
		})
		return nil, err
	})
	return err
}

func (s *MemgraphStore) ListAPIUsage(ctx context.Context, appID string, limit int64) ([]APIUsage, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp {id: $appID})-[:HAS_ENGINE_USAGE]->(u:EngineAPIUsage)
RETURN u.id AS id, u.appId AS appId, u.keyId AS keyId, u.method AS method, u.path AS path,
  coalesce(u.statusCode, 0) AS statusCode, coalesce(u.ipAddress, "") AS ipAddress,
  coalesce(u.userAgent, "") AS userAgent, u.createdAt AS createdAt
ORDER BY u.createdAt DESC
LIMIT $limit
`, map[string]any{"appID": strings.TrimSpace(appID), "limit": limit})
		if err != nil {
			return nil, err
		}
		usage := []APIUsage{}
		for rows.Next(ctx) {
			usage = append(usage, usageFromRecord(rows.Record()))
		}
		return usage, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]APIUsage), nil
}

func (s *MemgraphStore) EnqueueTransaction(ctx context.Context, input TransactionInput) (Transaction, error) {
	if input.ChainID <= 0 {
		return Transaction{}, errors.New("chainId is required")
	}
	wallet := strings.ToLower(strings.TrimSpace(input.WalletAddress))
	if wallet == "" {
		return Transaction{}, errors.New("walletAddress is required")
	}
	kind := normalizeTransactionKind(input.Kind)
	args := normalizeStringSlice(input.Args)
	metadata, _ := json.Marshal(input.Metadata)
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"id":              uuid.NewString(),
		"appID":           strings.TrimSpace(input.AppID),
		"keyID":           strings.TrimSpace(input.KeyID),
		"idempotencyKey":  strings.TrimSpace(input.IdempotencyKey),
		"chainID":         input.ChainID,
		"walletAddress":   wallet,
		"contractAddress": strings.ToLower(strings.TrimSpace(input.ContractAddress)),
		"kind":            kind,
		"method":          strings.TrimSpace(input.Method),
		"args":            args,
		"value":           strings.TrimSpace(input.Value),
		"metadata":        string(metadata),
		"now":             now,
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:EngineApp {id: $appID})
CREATE (t:EngineTransaction {
  id: $id, appId: $appID, keyId: $keyID, idempotencyKey: $idempotencyKey,
  chainId: $chainID, walletAddress: $walletAddress, contractAddress: $contractAddress,
  kind: $kind, method: $method, args: $args, value: $value, metadata: $metadata,
  status: "queued", attemptCount: 0, attemptLog: [], error: "", transactionHash: "",
  createdAt: $now, updatedAt: $now, queuedAt: $now
})
MERGE (a)-[:HAS_ENGINE_TRANSACTION]->(t)
RETURN t.id AS id, t.appId AS appId, t.keyId AS keyId, t.idempotencyKey AS idempotencyKey,
  t.chainId AS chainId, t.walletAddress AS walletAddress, t.contractAddress AS contractAddress,
  t.kind AS kind, t.method AS method, t.args AS args, coalesce(t.metadata, "") AS metadata,
  t.value AS value, t.status AS status,
  coalesce(t.attemptCount, 0) AS attemptCount, coalesce(t.attemptLog, []) AS attemptLog, coalesce(t.error, "") AS error,
  coalesce(t.transactionHash, "") AS transactionHash, t.createdAt AS createdAt, t.updatedAt AS updatedAt,
  t.queuedAt AS queuedAt, coalesce(t.startedAt, "") AS startedAt, coalesce(t.submittedAt, "") AS submittedAt,
  coalesce(t.confirmedAt, "") AS confirmedAt, coalesce(t.failedAt, "") AS failedAt
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return transactionFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("engine app not found")
	})
	if err != nil {
		return Transaction{}, err
	}
	return result.(Transaction), nil
}

func (s *MemgraphStore) GetTransaction(ctx context.Context, id string) (Transaction, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, transactionReturnQuery("MATCH (t:EngineTransaction {id: $id})"), map[string]any{"id": strings.TrimSpace(id)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return transactionFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return Transaction{}, false, err
	}
	if result == nil {
		return Transaction{}, false, nil
	}
	return result.(Transaction), true, nil
}

func (s *MemgraphStore) GetAccountTransaction(ctx context.Context, accountID string, transactionID string) (Transaction, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, transactionReturnQuery(`
MATCH (:GMRAccount {id: $accountID})-[:OWNS_ENGINE_APP]->(:EngineApp)-[:HAS_ENGINE_TRANSACTION]->(t:EngineTransaction {id: $transactionID})
`), map[string]any{"accountID": strings.TrimSpace(accountID), "transactionID": strings.TrimSpace(transactionID)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return transactionFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return Transaction{}, false, err
	}
	if result == nil {
		return Transaction{}, false, nil
	}
	return result.(Transaction), true, nil
}

func (s *MemgraphStore) ListTransactions(ctx context.Context, appID string, limit int64) ([]Transaction, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, transactionReturnQuery("MATCH (:EngineApp {id: $appID})-[:HAS_ENGINE_TRANSACTION]->(t:EngineTransaction)")+" ORDER BY t.createdAt DESC LIMIT $limit", map[string]any{"appID": strings.TrimSpace(appID), "limit": limit})
		if err != nil {
			return nil, err
		}
		transactions := []Transaction{}
		for rows.Next(ctx) {
			transactions = append(transactions, transactionFromRecord(rows.Record()))
		}
		return transactions, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]Transaction), nil
}

func (s *MemgraphStore) ClaimNextTransaction(ctx context.Context) (Transaction, bool, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, transactionReturnQuery(`
MATCH (t:EngineTransaction)
WHERE t.status = "queued" AND coalesce(t.attemptCount, 0) < 3
WITH t
ORDER BY t.queuedAt ASC
LIMIT 1
SET t.status = "signing",
  t.startedAt = $now,
  t.updatedAt = $now,
  t.attemptCount = coalesce(t.attemptCount, 0) + 1
`), map[string]any{"now": now})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return transactionFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return Transaction{}, false, err
	}
	if result == nil {
		return Transaction{}, false, nil
	}
	return result.(Transaction), true, nil
}

func (s *MemgraphStore) MarkTransactionSubmitted(ctx context.Context, transactionID string, transactionHash string) error {
	return s.updateTransactionStatus(ctx, transactionID, "submitted", transactionHash, "")
}

func (s *MemgraphStore) MarkTransactionConfirmed(ctx context.Context, transactionID string, transactionHash string) error {
	return s.updateTransactionStatus(ctx, transactionID, "confirmed", transactionHash, "")
}

func (s *MemgraphStore) MarkTransactionFailed(ctx context.Context, transactionID string, message string) error {
	return s.updateTransactionStatus(ctx, transactionID, "failed", "", message)
}

func (s *MemgraphStore) MarkTransactionRetry(ctx context.Context, transactionID string, message string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	entry := now + " " + strings.TrimSpace(message)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
MATCH (t:EngineTransaction {id: $id})
SET t.status = "queued",
  t.error = $message,
  t.attemptLog = coalesce(t.attemptLog, []) + [$entry],
  t.queuedAt = $now,
  t.updatedAt = $now
`, map[string]any{
			"id":      strings.TrimSpace(transactionID),
			"message": strings.TrimSpace(message),
			"entry":   entry,
			"now":     now,
		})
		return nil, err
	})
	return err
}

func (s *MemgraphStore) updateTransactionStatus(ctx context.Context, transactionID string, status string, transactionHash string, message string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	entry := ""
	if strings.TrimSpace(message) != "" {
		entry = now + " " + strings.TrimSpace(message)
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
MATCH (t:EngineTransaction {id: $id})
SET t.status = $status,
  t.transactionHash = CASE WHEN $transactionHash <> "" THEN $transactionHash ELSE coalesce(t.transactionHash, "") END,
  t.error = CASE WHEN $message <> "" THEN $message ELSE "" END,
  t.attemptLog = CASE WHEN $entry <> "" THEN coalesce(t.attemptLog, []) + [$entry] ELSE coalesce(t.attemptLog, []) END,
  t.updatedAt = $now,
  t.submittedAt = CASE WHEN $status = "submitted" THEN $now ELSE coalesce(t.submittedAt, "") END,
  t.confirmedAt = CASE WHEN $status = "confirmed" THEN $now ELSE coalesce(t.confirmedAt, "") END,
  t.failedAt = CASE WHEN $status = "failed" THEN $now ELSE coalesce(t.failedAt, "") END
`, map[string]any{
			"id":              strings.TrimSpace(transactionID),
			"status":          strings.TrimSpace(status),
			"transactionHash": strings.TrimSpace(transactionHash),
			"message":         strings.TrimSpace(message),
			"entry":           entry,
			"now":             now,
		})
		return nil, err
	})
	return err
}

type contractDeploymentSpec struct {
	label        string
	relationship string
}

var contractDeploymentSpecs = map[string]contractDeploymentSpec{
	"erc20":                         {label: "ERC20Deployment", relationship: "HAS_ERC20_DEPLOYMENT"},
	"erc1155-editions":              {label: "ERC1155EditionDeployment", relationship: "HAS_ERC1155_EDITION_DEPLOYMENT"},
	"escrow":                        {label: "EscrowDeployment", relationship: "HAS_ESCROW_DEPLOYMENT"},
	"marketplaces":                  {label: "MarketplaceDeployment", relationship: "HAS_MARKETPLACE_DEPLOYMENT"},
	"private-claim-registries":      {label: "PrivateClaimRegistryDeployment", relationship: "HAS_PRIVATE_CLAIM_REGISTRY_DEPLOYMENT"},
	"shielded-payout-pools":         {label: "ShieldedPayoutPoolDeployment", relationship: "HAS_SHIELDED_PAYOUT_POOL_DEPLOYMENT"},
	"privacy-access-passes":         {label: "PrivacyAccessPassDeployment", relationship: "HAS_PRIVACY_ACCESS_PASS_DEPLOYMENT"},
	"shielded-withdrawal-verifiers": {label: "ShieldedWithdrawalVerifierDeployment", relationship: "HAS_SHIELDED_WITHDRAWAL_VERIFIER_DEPLOYMENT"},
	"account-abstraction":           {label: "AccountAbstractionDeployment", relationship: "HAS_ACCOUNT_ABSTRACTION_DEPLOYMENT"},
}

func lookupContractDeploymentSpec(deploymentType string) (contractDeploymentSpec, bool) {
	spec, ok := contractDeploymentSpecs[strings.ToLower(strings.TrimSpace(deploymentType))]
	return spec, ok
}

func (s *MemgraphStore) RetryContractDeployment(ctx context.Context, accountID string, deploymentType string, deploymentID string) error {
	spec, ok := lookupContractDeploymentSpec(deploymentType)
	if !ok {
		return errors.New("unsupported contract deployment type")
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		query := fmt.Sprintf(`
MATCH (:GMRAccount {id: $accountID})-[:OWNS_ENGINE_APP]->(:EngineApp)-[:%s]->(d:%s {id: $deploymentID})
WHERE coalesce(d.hiddenFromDashboard, false) = false AND coalesce(d.status, "queued") = "failed"
SET d.status = "queued",
    d.error = "",
    d.transactionHash = "",
    d.contractAddress = "",
    d.queuedAt = $now,
    d.updatedAt = $now
RETURN d.id AS id
`, spec.relationship, spec.label)
		rows, err := tx.Run(ctx, query, map[string]any{
			"accountID":    strings.TrimSpace(accountID),
			"deploymentID": strings.TrimSpace(deploymentID),
			"now":          now,
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return true, nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("failed deployment not found")
	})
	if err != nil {
		return err
	}
	if result == nil {
		return errors.New("failed deployment not found")
	}
	return nil
}

func (s *MemgraphStore) FailStaleContractDeployments(ctx context.Context, staleBefore string) (int64, error) {
	staleBefore = strings.TrimSpace(staleBefore)
	if staleBefore == "" {
		return 0, errors.New("stale deployment cutoff is required")
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		var recovered int64
		for _, spec := range contractDeploymentSpecs {
			query := fmt.Sprintf(`
MATCH (d:%s)
WHERE coalesce(d.status, "queued") = "deploying" AND coalesce(d.updatedAt, "") < $staleBefore
SET d.status = "failed",
    d.error = "The deployment worker stopped before this attempt completed. Check the project wallet and explorer before retrying.",
    d.updatedAt = $now
RETURN count(d) AS recovered
`, spec.label)
			rows, err := tx.Run(ctx, query, map[string]any{
				"staleBefore": staleBefore,
				"now":         time.Now().UTC().Format(time.RFC3339),
			})
			if err != nil {
				return nil, err
			}
			if rows.Next(ctx) {
				recovered += intValue(rows.Record(), "recovered")
			}
			if err := rows.Err(); err != nil {
				return nil, err
			}
		}
		return recovered, nil
	})
	if err != nil {
		return 0, err
	}
	return result.(int64), nil
}

func (s *MemgraphStore) CreateERC20Deployment(ctx context.Context, input ERC20DeploymentInput) (ERC20Deployment, error) {
	name := strings.TrimSpace(input.Name)
	symbol := strings.ToUpper(strings.TrimSpace(input.Symbol))
	if name == "" {
		return ERC20Deployment{}, errors.New("token name is required")
	}
	if symbol == "" {
		return ERC20Deployment{}, errors.New("token symbol is required")
	}
	if input.Decimals < 0 || input.Decimals > 36 {
		return ERC20Deployment{}, errors.New("decimals must be between 0 and 36")
	}
	if input.Decimals == 0 {
		input.Decimals = 18
	}
	if strings.TrimSpace(input.InitialSupply) == "" {
		return ERC20Deployment{}, errors.New("initial supply is required")
	}
	if input.ChainID <= 0 {
		return ERC20Deployment{}, errors.New("chainId is required")
	}
	owner := strings.ToLower(strings.TrimSpace(input.OwnerAddress))
	if owner == "" {
		return ERC20Deployment{}, errors.New("owner address is required")
	}
	sourceName := safeContractName(name, symbol)
	sourceCode := erc20Source(sourceName, name, symbol, input.Decimals)
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"id":            uuid.NewString(),
		"appID":         strings.TrimSpace(input.AppID),
		"keyID":         strings.TrimSpace(input.KeyID),
		"name":          name,
		"symbol":        symbol,
		"decimals":      input.Decimals,
		"initialSupply": strings.TrimSpace(input.InitialSupply),
		"ownerAddress":  owner,
		"chainID":       input.ChainID,
		"description":   strings.TrimSpace(input.Description),
		"imageURL":      strings.TrimSpace(input.ImageURL),
		"socialURLs":    normalizeStringSlice(input.SocialURLs),
		"sourceName":    sourceName,
		"sourceCode":    sourceCode,
		"now":           now,
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:EngineApp {id: $appID})
CREATE (d:ERC20Deployment {
  id: $id, appId: $appID, keyId: $keyID, name: $name, symbol: $symbol,
  decimals: $decimals, initialSupply: $initialSupply, ownerAddress: $ownerAddress,
  chainId: $chainID, description: $description, imageUrl: $imageURL, socialUrls: $socialURLs,
  status: "queued", contractAddress: "", transactionHash: "", error: "",
  sourceName: $sourceName, sourceCode: $sourceCode,
  createdAt: $now, updatedAt: $now, queuedAt: $now
})
MERGE (a)-[:HAS_ERC20_DEPLOYMENT]->(d)
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name, d.symbol AS symbol,
  d.decimals AS decimals, d.initialSupply AS initialSupply, d.ownerAddress AS ownerAddress,
  d.chainId AS chainId, coalesce(d.description, "") AS description, coalesce(d.imageUrl, "") AS imageUrl,
  coalesce(d.socialUrls, []) AS socialUrls, coalesce(d.status, "queued") AS status,
  coalesce(d.contractAddress, "") AS contractAddress, coalesce(d.transactionHash, "") AS transactionHash,
  coalesce(d.error, "") AS error, coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  d.createdAt AS createdAt, d.updatedAt AS updatedAt, coalesce(d.queuedAt, "") AS queuedAt
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return erc20DeploymentFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("engine app not found")
	})
	if err != nil {
		return ERC20Deployment{}, err
	}
	return result.(ERC20Deployment), nil
}

func (s *MemgraphStore) ListERC20Deployments(ctx context.Context, appID string, limit int64) ([]ERC20Deployment, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp {id: $appID})-[:HAS_ERC20_DEPLOYMENT]->(d:ERC20Deployment)
WHERE coalesce(d.hiddenFromDashboard, false) = false
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name, d.symbol AS symbol,
  d.decimals AS decimals, d.initialSupply AS initialSupply, d.ownerAddress AS ownerAddress,
  d.chainId AS chainId, coalesce(d.description, "") AS description, coalesce(d.imageUrl, "") AS imageUrl,
  coalesce(d.socialUrls, []) AS socialUrls, coalesce(d.status, "queued") AS status,
  coalesce(d.contractAddress, "") AS contractAddress, coalesce(d.transactionHash, "") AS transactionHash,
  coalesce(d.error, "") AS error, coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  d.createdAt AS createdAt, d.updatedAt AS updatedAt, coalesce(d.queuedAt, "") AS queuedAt
ORDER BY d.createdAt DESC
LIMIT $limit
`, map[string]any{"appID": strings.TrimSpace(appID), "limit": limit})
		if err != nil {
			return nil, err
		}
		deployments := []ERC20Deployment{}
		for rows.Next(ctx) {
			deployments = append(deployments, erc20DeploymentFromRecord(rows.Record()))
		}
		return deployments, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]ERC20Deployment), nil
}

func (s *MemgraphStore) DeleteQueuedERC20Deployment(ctx context.Context, accountID string, deploymentID string) (ERC20Deployment, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:GMRAccount {id: $accountID})-[:OWNS_ENGINE_APP]->(:EngineApp)-[:HAS_ERC20_DEPLOYMENT]->(d:ERC20Deployment {id: $deploymentID})
WHERE coalesce(d.status, "queued") = "queued"
WITH d,
  d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name, d.symbol AS symbol,
  d.decimals AS decimals, d.initialSupply AS initialSupply, d.ownerAddress AS ownerAddress,
  d.chainId AS chainId, coalesce(d.description, "") AS description, coalesce(d.imageUrl, "") AS imageUrl,
  coalesce(d.socialUrls, []) AS socialUrls, coalesce(d.status, "queued") AS status,
  coalesce(d.contractAddress, "") AS contractAddress, coalesce(d.transactionHash, "") AS transactionHash,
  coalesce(d.error, "") AS error, coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  d.createdAt AS createdAt, d.updatedAt AS updatedAt, coalesce(d.queuedAt, "") AS queuedAt
DETACH DELETE d
RETURN id, appId, keyId, name, symbol, decimals, initialSupply, ownerAddress, chainId,
  description, imageUrl, socialUrls, status, contractAddress, transactionHash, error, sourceName,
  sourceCode, createdAt, updatedAt, queuedAt
`, map[string]any{
			"accountID":    strings.TrimSpace(accountID),
			"deploymentID": strings.TrimSpace(deploymentID),
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return erc20DeploymentFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("queued deployment not found")
	})
	if err != nil {
		return ERC20Deployment{}, err
	}
	return result.(ERC20Deployment), nil
}

func (s *MemgraphStore) RemoveERC20DeploymentFromDashboard(ctx context.Context, accountID string, deploymentID string) (ERC20Deployment, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:GMRAccount {id: $accountID})-[:OWNS_ENGINE_APP]->(:EngineApp)-[:HAS_ERC20_DEPLOYMENT]->(d:ERC20Deployment {id: $deploymentID})
WHERE coalesce(d.hiddenFromDashboard, false) = false AND NOT coalesce(d.status, "queued") IN ["deploying", "submitted"]
SET d.hiddenFromDashboard = true, d.removedAt = $now, d.updatedAt = $now
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name, d.symbol AS symbol,
  d.decimals AS decimals, d.initialSupply AS initialSupply, d.ownerAddress AS ownerAddress,
  d.chainId AS chainId, coalesce(d.description, "") AS description, coalesce(d.imageUrl, "") AS imageUrl,
  coalesce(d.socialUrls, []) AS socialUrls, coalesce(d.status, "queued") AS status,
  coalesce(d.contractAddress, "") AS contractAddress, coalesce(d.transactionHash, "") AS transactionHash,
  coalesce(d.error, "") AS error, coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  d.createdAt AS createdAt, d.updatedAt AS updatedAt, coalesce(d.queuedAt, "") AS queuedAt
`, map[string]any{
			"accountID":    strings.TrimSpace(accountID),
			"deploymentID": strings.TrimSpace(deploymentID),
			"now":          time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return erc20DeploymentFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("removable deployment not found")
	})
	if err != nil {
		return ERC20Deployment{}, err
	}
	return result.(ERC20Deployment), nil
}

func (s *MemgraphStore) ClaimNextERC20Deployment(ctx context.Context) (ERC20Deployment, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp)-[:HAS_ERC20_DEPLOYMENT]->(d:ERC20Deployment)
WHERE coalesce(d.status, "queued") = "queued" AND coalesce(d.hiddenFromDashboard, false) = false
WITH d ORDER BY d.createdAt ASC LIMIT 1
SET d.status = "deploying", d.updatedAt = $now, d.error = ""
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name, d.symbol AS symbol,
  d.decimals AS decimals, d.initialSupply AS initialSupply, d.ownerAddress AS ownerAddress,
  d.chainId AS chainId, coalesce(d.description, "") AS description, coalesce(d.imageUrl, "") AS imageUrl,
  coalesce(d.socialUrls, []) AS socialUrls, coalesce(d.status, "queued") AS status,
  coalesce(d.contractAddress, "") AS contractAddress, coalesce(d.transactionHash, "") AS transactionHash,
  coalesce(d.error, "") AS error, coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  d.createdAt AS createdAt, d.updatedAt AS updatedAt, coalesce(d.queuedAt, "") AS queuedAt
`, map[string]any{"now": time.Now().UTC().Format(time.RFC3339)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return erc20DeploymentFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return ERC20Deployment{}, false, err
	}
	if result == nil {
		return ERC20Deployment{}, false, nil
	}
	return result.(ERC20Deployment), true, nil
}

func (s *MemgraphStore) MarkERC20DeploymentSubmitted(ctx context.Context, deploymentID string, transactionHash string) error {
	return s.updateERC20DeploymentStatus(ctx, deploymentID, "submitted", transactionHash, "", "")
}

func (s *MemgraphStore) MarkERC20DeploymentConfirmed(ctx context.Context, deploymentID string, contractAddress string) error {
	return s.updateERC20DeploymentStatus(ctx, deploymentID, "confirmed", "", strings.ToLower(strings.TrimSpace(contractAddress)), "")
}

func (s *MemgraphStore) MarkERC20DeploymentFailed(ctx context.Context, deploymentID string, message string) error {
	return s.updateERC20DeploymentStatus(ctx, deploymentID, "failed", "", "", message)
}

func (s *MemgraphStore) updateERC20DeploymentStatus(ctx context.Context, deploymentID string, status string, transactionHash string, contractAddress string, message string) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
MATCH (d:ERC20Deployment {id: $deploymentID})
SET d.status = $status, d.updatedAt = $now
SET d.transactionHash = CASE WHEN $transactionHash <> "" THEN $transactionHash ELSE coalesce(d.transactionHash, "") END
SET d.contractAddress = CASE WHEN $contractAddress <> "" THEN $contractAddress ELSE coalesce(d.contractAddress, "") END
SET d.error = CASE WHEN $error <> "" THEN $error ELSE "" END
RETURN d.id AS id
`, map[string]any{
			"deploymentID":    strings.TrimSpace(deploymentID),
			"status":          strings.TrimSpace(status),
			"transactionHash": strings.TrimSpace(transactionHash),
			"contractAddress": strings.TrimSpace(contractAddress),
			"error":           strings.TrimSpace(message),
			"now":             time.Now().UTC().Format(time.RFC3339),
		})
		return nil, err
	})
	return err
}

func (s *MemgraphStore) GetAccountERC20Deployment(ctx context.Context, accountID string, deploymentID string) (ERC20Deployment, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:GMRAccount {id: $accountID})-[:OWNS_ENGINE_APP]->(:EngineApp)-[:HAS_ERC20_DEPLOYMENT]->(d:ERC20Deployment {id: $deploymentID})
WHERE coalesce(d.hiddenFromDashboard, false) = false
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name, d.symbol AS symbol,
  d.decimals AS decimals, d.initialSupply AS initialSupply, d.ownerAddress AS ownerAddress,
  d.chainId AS chainId, coalesce(d.description, "") AS description, coalesce(d.imageUrl, "") AS imageUrl,
  coalesce(d.socialUrls, []) AS socialUrls, coalesce(d.status, "queued") AS status,
  coalesce(d.contractAddress, "") AS contractAddress, coalesce(d.transactionHash, "") AS transactionHash,
  coalesce(d.error, "") AS error, coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  d.createdAt AS createdAt, d.updatedAt AS updatedAt, coalesce(d.queuedAt, "") AS queuedAt
LIMIT 1
`, map[string]any{
			"accountID":    strings.TrimSpace(accountID),
			"deploymentID": strings.TrimSpace(deploymentID),
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return erc20DeploymentFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return ERC20Deployment{}, false, err
	}
	if result == nil {
		return ERC20Deployment{}, false, nil
	}
	return result.(ERC20Deployment), true, nil
}

func (s *MemgraphStore) CreateERC1155EditionDeployment(ctx context.Context, input ERC1155EditionDeploymentInput) (ERC1155EditionDeployment, error) {
	name := strings.TrimSpace(input.Name)
	symbol := strings.ToUpper(strings.TrimSpace(input.Symbol))
	if name == "" {
		return ERC1155EditionDeployment{}, errors.New("collection name is required")
	}
	if symbol == "" {
		return ERC1155EditionDeployment{}, errors.New("collection symbol is required")
	}
	if input.ChainID <= 0 {
		return ERC1155EditionDeployment{}, errors.New("chainId is required")
	}
	owner := strings.ToLower(strings.TrimSpace(input.OwnerAddress))
	if owner == "" {
		return ERC1155EditionDeployment{}, errors.New("owner address is required")
	}
	recipient := strings.ToLower(strings.TrimSpace(input.RecipientAddress))
	if recipient == "" {
		recipient = owner
	}
	initialTokenID := strings.TrimSpace(input.InitialTokenID)
	if initialTokenID == "" {
		initialTokenID = "0"
	}
	initialSupply := strings.TrimSpace(input.InitialSupply)
	if initialSupply == "" {
		initialSupply = "0"
	}
	sourceName := safeContractName(name, symbol)
	if !strings.HasSuffix(strings.ToLower(sourceName), "edition") {
		sourceName += "Edition"
	}
	sourceCode := erc1155EditionSource(sourceName, name, symbol)
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"id":               uuid.NewString(),
		"appID":            strings.TrimSpace(input.AppID),
		"keyID":            strings.TrimSpace(input.KeyID),
		"name":             name,
		"symbol":           symbol,
		"baseURI":          strings.TrimSpace(input.BaseURI),
		"contractURI":      strings.TrimSpace(input.ContractURI),
		"ownerAddress":     owner,
		"recipientAddress": recipient,
		"initialTokenID":   initialTokenID,
		"initialSupply":    initialSupply,
		"chainID":          input.ChainID,
		"description":      strings.TrimSpace(input.Description),
		"imageURL":         strings.TrimSpace(input.ImageURL),
		"socialURLs":       normalizeStringSlice(input.SocialURLs),
		"sourceName":       sourceName,
		"sourceCode":       sourceCode,
		"abi":              erc1155EditionABI,
		"now":              now,
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:EngineApp {id: $appID})
CREATE (d:ERC1155EditionDeployment {
  id: $id, appId: $appID, keyId: $keyID, name: $name, symbol: $symbol,
  baseUri: $baseURI, contractUri: $contractURI, ownerAddress: $ownerAddress,
  recipientAddress: $recipientAddress, initialTokenId: $initialTokenID, initialSupply: $initialSupply,
  chainId: $chainID, description: $description, imageUrl: $imageURL, socialUrls: $socialURLs,
  status: "queued", contractAddress: "", transactionHash: "", error: "",
  sourceName: $sourceName, sourceCode: $sourceCode, abi: $abi,
  createdAt: $now, updatedAt: $now, queuedAt: $now
})
MERGE (a)-[:HAS_ERC1155_EDITION_DEPLOYMENT]->(d)
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name, d.symbol AS symbol,
  coalesce(d.baseUri, "") AS baseUri, coalesce(d.contractUri, "") AS contractUri,
  d.ownerAddress AS ownerAddress, coalesce(d.recipientAddress, "") AS recipientAddress,
  coalesce(d.initialTokenId, "0") AS initialTokenId, coalesce(d.initialSupply, "0") AS initialSupply,
  d.chainId AS chainId, coalesce(d.description, "") AS description, coalesce(d.imageUrl, "") AS imageUrl,
  coalesce(d.socialUrls, []) AS socialUrls, coalesce(d.status, "queued") AS status,
  coalesce(d.contractAddress, "") AS contractAddress, coalesce(d.transactionHash, "") AS transactionHash,
  coalesce(d.error, "") AS error, coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt, coalesce(d.queuedAt, "") AS queuedAt
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return erc1155EditionDeploymentFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("engine app not found")
	})
	if err != nil {
		return ERC1155EditionDeployment{}, err
	}
	return result.(ERC1155EditionDeployment), nil
}

func (s *MemgraphStore) ListERC1155EditionDeployments(ctx context.Context, appID string, limit int64) ([]ERC1155EditionDeployment, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp {id: $appID})-[:HAS_ERC1155_EDITION_DEPLOYMENT]->(d:ERC1155EditionDeployment)
WHERE coalesce(d.hiddenFromDashboard, false) = false
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name, d.symbol AS symbol,
  coalesce(d.baseUri, "") AS baseUri, coalesce(d.contractUri, "") AS contractUri,
  d.ownerAddress AS ownerAddress, coalesce(d.recipientAddress, "") AS recipientAddress,
  coalesce(d.initialTokenId, "0") AS initialTokenId, coalesce(d.initialSupply, "0") AS initialSupply,
  d.chainId AS chainId, coalesce(d.description, "") AS description, coalesce(d.imageUrl, "") AS imageUrl,
  coalesce(d.socialUrls, []) AS socialUrls, coalesce(d.status, "queued") AS status,
  coalesce(d.contractAddress, "") AS contractAddress, coalesce(d.transactionHash, "") AS transactionHash,
  coalesce(d.error, "") AS error, coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt, coalesce(d.queuedAt, "") AS queuedAt
ORDER BY d.createdAt DESC
LIMIT $limit
`, map[string]any{"appID": strings.TrimSpace(appID), "limit": limit})
		if err != nil {
			return nil, err
		}
		deployments := []ERC1155EditionDeployment{}
		for rows.Next(ctx) {
			deployments = append(deployments, erc1155EditionDeploymentFromRecord(rows.Record()))
		}
		return deployments, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]ERC1155EditionDeployment), nil
}

func (s *MemgraphStore) DeleteQueuedERC1155EditionDeployment(ctx context.Context, accountID string, deploymentID string) (ERC1155EditionDeployment, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:GMRAccount {id: $accountID})-[:OWNS_ENGINE_APP]->(:EngineApp)-[:HAS_ERC1155_EDITION_DEPLOYMENT]->(d:ERC1155EditionDeployment {id: $deploymentID})
WHERE coalesce(d.status, "queued") = "queued"
WITH d,
  d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name, d.symbol AS symbol,
  coalesce(d.baseUri, "") AS baseUri, coalesce(d.contractUri, "") AS contractUri,
  d.ownerAddress AS ownerAddress, coalesce(d.recipientAddress, "") AS recipientAddress,
  coalesce(d.initialTokenId, "0") AS initialTokenId, coalesce(d.initialSupply, "0") AS initialSupply,
  d.chainId AS chainId, coalesce(d.description, "") AS description, coalesce(d.imageUrl, "") AS imageUrl,
  coalesce(d.socialUrls, []) AS socialUrls, coalesce(d.status, "queued") AS status,
  coalesce(d.contractAddress, "") AS contractAddress, coalesce(d.transactionHash, "") AS transactionHash,
  coalesce(d.error, "") AS error, coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt, coalesce(d.queuedAt, "") AS queuedAt
DETACH DELETE d
RETURN id, appId, keyId, name, symbol, baseUri, contractUri, ownerAddress, recipientAddress,
  initialTokenId, initialSupply, chainId, description, imageUrl, socialUrls, status, contractAddress,
  transactionHash, error, sourceName, sourceCode, abi, createdAt, updatedAt, queuedAt
`, map[string]any{
			"accountID":    strings.TrimSpace(accountID),
			"deploymentID": strings.TrimSpace(deploymentID),
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return erc1155EditionDeploymentFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("queued deployment not found")
	})
	if err != nil {
		return ERC1155EditionDeployment{}, err
	}
	return result.(ERC1155EditionDeployment), nil
}

func (s *MemgraphStore) RemoveERC1155EditionDeploymentFromDashboard(ctx context.Context, accountID string, deploymentID string) (ERC1155EditionDeployment, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:GMRAccount {id: $accountID})-[:OWNS_ENGINE_APP]->(:EngineApp)-[:HAS_ERC1155_EDITION_DEPLOYMENT]->(d:ERC1155EditionDeployment {id: $deploymentID})
WHERE coalesce(d.hiddenFromDashboard, false) = false AND NOT coalesce(d.status, "queued") IN ["deploying", "submitted"]
SET d.hiddenFromDashboard = true, d.removedAt = $now, d.updatedAt = $now
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name, d.symbol AS symbol,
  coalesce(d.baseUri, "") AS baseUri, coalesce(d.contractUri, "") AS contractUri,
  d.ownerAddress AS ownerAddress, coalesce(d.recipientAddress, "") AS recipientAddress,
  coalesce(d.initialTokenId, "0") AS initialTokenId, coalesce(d.initialSupply, "0") AS initialSupply,
  d.chainId AS chainId, coalesce(d.description, "") AS description, coalesce(d.imageUrl, "") AS imageUrl,
  coalesce(d.socialUrls, []) AS socialUrls, coalesce(d.status, "queued") AS status,
  coalesce(d.contractAddress, "") AS contractAddress, coalesce(d.transactionHash, "") AS transactionHash,
  coalesce(d.error, "") AS error, coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt, coalesce(d.queuedAt, "") AS queuedAt
`, map[string]any{
			"accountID":    strings.TrimSpace(accountID),
			"deploymentID": strings.TrimSpace(deploymentID),
			"now":          time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return erc1155EditionDeploymentFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("removable deployment not found")
	})
	if err != nil {
		return ERC1155EditionDeployment{}, err
	}
	return result.(ERC1155EditionDeployment), nil
}

func (s *MemgraphStore) ClaimNextERC1155EditionDeployment(ctx context.Context) (ERC1155EditionDeployment, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp)-[:HAS_ERC1155_EDITION_DEPLOYMENT]->(d:ERC1155EditionDeployment)
WHERE coalesce(d.status, "queued") = "queued" AND coalesce(d.hiddenFromDashboard, false) = false
WITH d ORDER BY d.createdAt ASC LIMIT 1
SET d.status = "deploying", d.updatedAt = $now, d.error = ""
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name, d.symbol AS symbol,
  coalesce(d.baseUri, "") AS baseUri, coalesce(d.contractUri, "") AS contractUri,
  d.ownerAddress AS ownerAddress, coalesce(d.recipientAddress, "") AS recipientAddress,
  coalesce(d.initialTokenId, "0") AS initialTokenId, coalesce(d.initialSupply, "0") AS initialSupply,
  d.chainId AS chainId, coalesce(d.description, "") AS description, coalesce(d.imageUrl, "") AS imageUrl,
  coalesce(d.socialUrls, []) AS socialUrls, coalesce(d.status, "queued") AS status,
  coalesce(d.contractAddress, "") AS contractAddress, coalesce(d.transactionHash, "") AS transactionHash,
  coalesce(d.error, "") AS error, coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt, coalesce(d.queuedAt, "") AS queuedAt
`, map[string]any{"now": time.Now().UTC().Format(time.RFC3339)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return erc1155EditionDeploymentFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return ERC1155EditionDeployment{}, false, err
	}
	if result == nil {
		return ERC1155EditionDeployment{}, false, nil
	}
	return result.(ERC1155EditionDeployment), true, nil
}

func (s *MemgraphStore) MarkERC1155EditionDeploymentSubmitted(ctx context.Context, deploymentID string, transactionHash string) error {
	return s.updateERC1155EditionDeploymentStatus(ctx, deploymentID, "submitted", transactionHash, "", "")
}

func (s *MemgraphStore) MarkERC1155EditionDeploymentConfirmed(ctx context.Context, deploymentID string, contractAddress string) error {
	return s.updateERC1155EditionDeploymentStatus(ctx, deploymentID, "confirmed", "", strings.ToLower(strings.TrimSpace(contractAddress)), "")
}

func (s *MemgraphStore) MarkERC1155EditionDeploymentFailed(ctx context.Context, deploymentID string, message string) error {
	return s.updateERC1155EditionDeploymentStatus(ctx, deploymentID, "failed", "", "", message)
}

func (s *MemgraphStore) updateERC1155EditionDeploymentStatus(ctx context.Context, deploymentID string, status string, transactionHash string, contractAddress string, message string) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
MATCH (d:ERC1155EditionDeployment {id: $deploymentID})
SET d.status = $status, d.updatedAt = $now
SET d.transactionHash = CASE WHEN $transactionHash <> "" THEN $transactionHash ELSE coalesce(d.transactionHash, "") END
SET d.contractAddress = CASE WHEN $contractAddress <> "" THEN $contractAddress ELSE coalesce(d.contractAddress, "") END
SET d.error = CASE WHEN $error <> "" THEN $error ELSE "" END
RETURN d.id AS id
`, map[string]any{
			"deploymentID":    strings.TrimSpace(deploymentID),
			"status":          strings.TrimSpace(status),
			"transactionHash": strings.TrimSpace(transactionHash),
			"contractAddress": strings.TrimSpace(contractAddress),
			"error":           strings.TrimSpace(message),
			"now":             time.Now().UTC().Format(time.RFC3339),
		})
		return nil, err
	})
	return err
}

func (s *MemgraphStore) CreateEscrowDeployment(ctx context.Context, input EscrowDeploymentInput) (EscrowDeployment, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = "BudolPH Prediction Escrow"
	}
	if input.ChainID <= 0 {
		return EscrowDeployment{}, errors.New("chainId is required")
	}
	tokenAddress := strings.ToLower(strings.TrimSpace(input.TokenAddress))
	if tokenAddress == "" {
		return EscrowDeployment{}, errors.New("token address is required")
	}
	ownerAddress := strings.ToLower(strings.TrimSpace(input.OwnerAddress))
	if ownerAddress == "" {
		return EscrowDeployment{}, errors.New("owner address is required")
	}
	treasuryAddress := strings.ToLower(strings.TrimSpace(input.TreasuryAddress))
	if treasuryAddress == "" {
		treasuryAddress = ownerAddress
	}
	if input.FeeBps < 0 || input.FeeBps > 1000 {
		return EscrowDeployment{}, errors.New("feeBps must be between 0 and 1000")
	}
	sourceName := safeContractName(name, "Escrow")
	if !strings.HasSuffix(strings.ToLower(sourceName), "escrow") {
		sourceName += "Escrow"
	}
	sourceCode := budolEscrowSource(sourceName)
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"id":              uuid.NewString(),
		"appID":           strings.TrimSpace(input.AppID),
		"keyID":           strings.TrimSpace(input.KeyID),
		"name":            name,
		"tokenAddress":    tokenAddress,
		"ownerAddress":    ownerAddress,
		"treasuryAddress": treasuryAddress,
		"feeBps":          input.FeeBps,
		"chainID":         input.ChainID,
		"description":     strings.TrimSpace(input.Description),
		"sourceName":      sourceName,
		"sourceCode":      sourceCode,
		"now":             now,
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:EngineApp {id: $appID})
CREATE (d:EscrowDeployment {
  id: $id, appId: $appID, keyId: $keyID, name: $name, tokenAddress: $tokenAddress,
  ownerAddress: $ownerAddress, treasuryAddress: $treasuryAddress, feeBps: $feeBps,
  chainId: $chainID, description: $description, status: "queued",
  contractAddress: "", transactionHash: "", error: "", sourceName: $sourceName,
  sourceCode: $sourceCode, abi: "", createdAt: $now, updatedAt: $now, queuedAt: $now
})
MERGE (a)-[:HAS_ESCROW_DEPLOYMENT]->(d)
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name,
  d.tokenAddress AS tokenAddress, d.ownerAddress AS ownerAddress, d.treasuryAddress AS treasuryAddress,
  d.feeBps AS feeBps, d.chainId AS chainId, coalesce(d.description, "") AS description,
  coalesce(d.status, "queued") AS status, coalesce(d.contractAddress, "") AS contractAddress,
  coalesce(d.transactionHash, "") AS transactionHash, coalesce(d.error, "") AS error,
  coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt,
  coalesce(d.queuedAt, "") AS queuedAt
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return escrowDeploymentFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("engine app not found")
	})
	if err != nil {
		return EscrowDeployment{}, err
	}
	return result.(EscrowDeployment), nil
}

func (s *MemgraphStore) ListEscrowDeployments(ctx context.Context, appID string, limit int64) ([]EscrowDeployment, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp {id: $appID})-[:HAS_ESCROW_DEPLOYMENT]->(d:EscrowDeployment)
WHERE coalesce(d.hiddenFromDashboard, false) = false
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name,
  d.tokenAddress AS tokenAddress, d.ownerAddress AS ownerAddress, d.treasuryAddress AS treasuryAddress,
  d.feeBps AS feeBps, d.chainId AS chainId, coalesce(d.description, "") AS description,
  coalesce(d.status, "queued") AS status, coalesce(d.contractAddress, "") AS contractAddress,
  coalesce(d.transactionHash, "") AS transactionHash, coalesce(d.error, "") AS error,
  coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt,
  coalesce(d.queuedAt, "") AS queuedAt
ORDER BY d.createdAt DESC
LIMIT $limit
`, map[string]any{"appID": strings.TrimSpace(appID), "limit": limit})
		if err != nil {
			return nil, err
		}
		deployments := []EscrowDeployment{}
		for rows.Next(ctx) {
			deployments = append(deployments, escrowDeploymentFromRecord(rows.Record()))
		}
		return deployments, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]EscrowDeployment), nil
}

func (s *MemgraphStore) DeleteQueuedEscrowDeployment(ctx context.Context, accountID string, deploymentID string) (EscrowDeployment, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:GMRAccount {id: $accountID})-[:OWNS_ENGINE_APP]->(:EngineApp)-[:HAS_ESCROW_DEPLOYMENT]->(d:EscrowDeployment {id: $deploymentID})
WHERE coalesce(d.status, "queued") = "queued"
WITH d,
  d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name,
  d.tokenAddress AS tokenAddress, d.ownerAddress AS ownerAddress, d.treasuryAddress AS treasuryAddress,
  d.feeBps AS feeBps, d.chainId AS chainId, coalesce(d.description, "") AS description,
  coalesce(d.status, "queued") AS status, coalesce(d.contractAddress, "") AS contractAddress,
  coalesce(d.transactionHash, "") AS transactionHash, coalesce(d.error, "") AS error,
  coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt,
  coalesce(d.queuedAt, "") AS queuedAt
DETACH DELETE d
RETURN id, appId, keyId, name, tokenAddress, ownerAddress, treasuryAddress, feeBps,
  chainId, description, status, contractAddress, transactionHash, error, sourceName,
  sourceCode, abi, createdAt, updatedAt, queuedAt
`, map[string]any{"accountID": strings.TrimSpace(accountID), "deploymentID": strings.TrimSpace(deploymentID)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return escrowDeploymentFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("queued deployment not found")
	})
	if err != nil {
		return EscrowDeployment{}, err
	}
	return result.(EscrowDeployment), nil
}

func (s *MemgraphStore) RemoveEscrowDeploymentFromDashboard(ctx context.Context, accountID string, deploymentID string) (EscrowDeployment, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:GMRAccount {id: $accountID})-[:OWNS_ENGINE_APP]->(:EngineApp)-[:HAS_ESCROW_DEPLOYMENT]->(d:EscrowDeployment {id: $deploymentID})
WHERE coalesce(d.hiddenFromDashboard, false) = false AND NOT coalesce(d.status, "queued") IN ["deploying", "submitted"]
SET d.hiddenFromDashboard = true, d.removedAt = $now, d.updatedAt = $now
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name,
  d.tokenAddress AS tokenAddress, d.ownerAddress AS ownerAddress, d.treasuryAddress AS treasuryAddress,
  d.feeBps AS feeBps, d.chainId AS chainId, coalesce(d.description, "") AS description,
  coalesce(d.status, "queued") AS status, coalesce(d.contractAddress, "") AS contractAddress,
  coalesce(d.transactionHash, "") AS transactionHash, coalesce(d.error, "") AS error,
  coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt,
  coalesce(d.queuedAt, "") AS queuedAt
`, map[string]any{
			"accountID":    strings.TrimSpace(accountID),
			"deploymentID": strings.TrimSpace(deploymentID),
			"now":          time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return escrowDeploymentFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("removable deployment not found")
	})
	if err != nil {
		return EscrowDeployment{}, err
	}
	return result.(EscrowDeployment), nil
}

func (s *MemgraphStore) ClaimNextEscrowDeployment(ctx context.Context) (EscrowDeployment, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp)-[:HAS_ESCROW_DEPLOYMENT]->(d:EscrowDeployment)
WHERE coalesce(d.status, "queued") = "queued" AND coalesce(d.hiddenFromDashboard, false) = false
WITH d ORDER BY d.createdAt ASC LIMIT 1
SET d.status = "deploying", d.updatedAt = $now, d.error = ""
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name,
  d.tokenAddress AS tokenAddress, d.ownerAddress AS ownerAddress, d.treasuryAddress AS treasuryAddress,
  d.feeBps AS feeBps, d.chainId AS chainId, coalesce(d.description, "") AS description,
  coalesce(d.status, "queued") AS status, coalesce(d.contractAddress, "") AS contractAddress,
  coalesce(d.transactionHash, "") AS transactionHash, coalesce(d.error, "") AS error,
  coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt,
  coalesce(d.queuedAt, "") AS queuedAt
`, map[string]any{"now": time.Now().UTC().Format(time.RFC3339)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return escrowDeploymentFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return EscrowDeployment{}, false, err
	}
	if result == nil {
		return EscrowDeployment{}, false, nil
	}
	return result.(EscrowDeployment), true, nil
}

func (s *MemgraphStore) MarkEscrowDeploymentSubmitted(ctx context.Context, deploymentID string, transactionHash string) error {
	return s.updateEscrowDeploymentStatus(ctx, deploymentID, "submitted", transactionHash, "", "", "")
}

func (s *MemgraphStore) MarkEscrowDeploymentConfirmed(ctx context.Context, deploymentID string, contractAddress string, abi string) error {
	return s.updateEscrowDeploymentStatus(ctx, deploymentID, "confirmed", "", strings.ToLower(strings.TrimSpace(contractAddress)), "", abi)
}

func (s *MemgraphStore) MarkEscrowDeploymentFailed(ctx context.Context, deploymentID string, message string) error {
	return s.updateEscrowDeploymentStatus(ctx, deploymentID, "failed", "", "", message, "")
}

func (s *MemgraphStore) updateEscrowDeploymentStatus(ctx context.Context, deploymentID string, status string, transactionHash string, contractAddress string, message string, abi string) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
MATCH (d:EscrowDeployment {id: $deploymentID})
SET d.status = $status, d.updatedAt = $now
SET d.transactionHash = CASE WHEN $transactionHash <> "" THEN $transactionHash ELSE coalesce(d.transactionHash, "") END
SET d.contractAddress = CASE WHEN $contractAddress <> "" THEN $contractAddress ELSE coalesce(d.contractAddress, "") END
SET d.error = CASE WHEN $error <> "" THEN $error ELSE "" END
SET d.abi = CASE WHEN $abi <> "" THEN $abi ELSE coalesce(d.abi, "") END
RETURN d.id AS id
`, map[string]any{
			"deploymentID":    strings.TrimSpace(deploymentID),
			"status":          strings.TrimSpace(status),
			"transactionHash": strings.TrimSpace(transactionHash),
			"contractAddress": strings.TrimSpace(contractAddress),
			"error":           strings.TrimSpace(message),
			"abi":             strings.TrimSpace(abi),
			"now":             time.Now().UTC().Format(time.RFC3339),
		})
		return nil, err
	})
	return err
}

func (s *MemgraphStore) CreatePrivateClaimRegistryDeployment(ctx context.Context, input PrivateClaimRegistryDeploymentInput) (PrivateClaimRegistryDeployment, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = "Private Claim Registry"
	}
	if input.ChainID <= 0 {
		return PrivateClaimRegistryDeployment{}, errors.New("chainId is required")
	}
	ownerAddress := strings.ToLower(strings.TrimSpace(input.OwnerAddress))
	if ownerAddress == "" {
		return PrivateClaimRegistryDeployment{}, errors.New("owner address is required")
	}
	verifierAddress := strings.ToLower(strings.TrimSpace(input.VerifierAddress))
	if verifierAddress == "" {
		verifierAddress = "0x0000000000000000000000000000000000000000"
	}
	sourceName := safeContractName(name, "PrivateClaimRegistry")
	if !strings.Contains(strings.ToLower(sourceName), "registry") {
		sourceName += "Registry"
	}
	sourceCode := privateClaimRegistrySource(sourceName)
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"id":              uuid.NewString(),
		"appID":           strings.TrimSpace(input.AppID),
		"keyID":           strings.TrimSpace(input.KeyID),
		"name":            name,
		"ownerAddress":    ownerAddress,
		"verifierAddress": verifierAddress,
		"chainID":         input.ChainID,
		"description":     strings.TrimSpace(input.Description),
		"sourceName":      sourceName,
		"sourceCode":      sourceCode,
		"now":             now,
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:EngineApp {id: $appID})
CREATE (d:PrivateClaimRegistryDeployment {
  id: $id, appId: $appID, keyId: $keyID, name: $name,
  ownerAddress: $ownerAddress, verifierAddress: $verifierAddress,
  chainId: $chainID, description: $description, status: "queued",
  contractAddress: "", transactionHash: "", error: "", sourceName: $sourceName,
  sourceCode: $sourceCode, abi: "", createdAt: $now, updatedAt: $now, queuedAt: $now
})
MERGE (a)-[:HAS_PRIVATE_CLAIM_REGISTRY_DEPLOYMENT]->(d)
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name,
  d.ownerAddress AS ownerAddress, d.verifierAddress AS verifierAddress,
  d.chainId AS chainId, coalesce(d.description, "") AS description,
  coalesce(d.status, "queued") AS status, coalesce(d.contractAddress, "") AS contractAddress,
  coalesce(d.transactionHash, "") AS transactionHash, coalesce(d.error, "") AS error,
  coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt,
  coalesce(d.queuedAt, "") AS queuedAt
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return privateClaimRegistryDeploymentFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("engine app not found")
	})
	if err != nil {
		return PrivateClaimRegistryDeployment{}, err
	}
	return result.(PrivateClaimRegistryDeployment), nil
}

func (s *MemgraphStore) ListPrivateClaimRegistryDeployments(ctx context.Context, appID string, limit int64) ([]PrivateClaimRegistryDeployment, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp {id: $appID})-[:HAS_PRIVATE_CLAIM_REGISTRY_DEPLOYMENT]->(d:PrivateClaimRegistryDeployment)
WHERE coalesce(d.hiddenFromDashboard, false) = false
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name,
  d.ownerAddress AS ownerAddress, d.verifierAddress AS verifierAddress,
  d.chainId AS chainId, coalesce(d.description, "") AS description,
  coalesce(d.status, "queued") AS status, coalesce(d.contractAddress, "") AS contractAddress,
  coalesce(d.transactionHash, "") AS transactionHash, coalesce(d.error, "") AS error,
  coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt,
  coalesce(d.queuedAt, "") AS queuedAt
ORDER BY d.createdAt DESC
LIMIT $limit
`, map[string]any{"appID": strings.TrimSpace(appID), "limit": limit})
		if err != nil {
			return nil, err
		}
		deployments := []PrivateClaimRegistryDeployment{}
		for rows.Next(ctx) {
			deployments = append(deployments, privateClaimRegistryDeploymentFromRecord(rows.Record()))
		}
		return deployments, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]PrivateClaimRegistryDeployment), nil
}

func (s *MemgraphStore) RemovePrivateClaimRegistryDeploymentFromDashboard(ctx context.Context, accountID string, deploymentID string) (PrivateClaimRegistryDeployment, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:GMRAccount {id: $accountID})-[:OWNS_ENGINE_APP]->(:EngineApp)-[:HAS_PRIVATE_CLAIM_REGISTRY_DEPLOYMENT]->(d:PrivateClaimRegistryDeployment {id: $deploymentID})
WHERE coalesce(d.hiddenFromDashboard, false) = false AND NOT coalesce(d.status, "queued") IN ["deploying", "submitted"]
SET d.hiddenFromDashboard = true, d.removedAt = $now, d.updatedAt = $now
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name,
  d.ownerAddress AS ownerAddress, d.verifierAddress AS verifierAddress,
  d.chainId AS chainId, coalesce(d.description, "") AS description,
  coalesce(d.status, "queued") AS status, coalesce(d.contractAddress, "") AS contractAddress,
  coalesce(d.transactionHash, "") AS transactionHash, coalesce(d.error, "") AS error,
  coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt,
  coalesce(d.queuedAt, "") AS queuedAt
`, map[string]any{
			"accountID":    strings.TrimSpace(accountID),
			"deploymentID": strings.TrimSpace(deploymentID),
			"now":          time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return privateClaimRegistryDeploymentFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("removable deployment not found")
	})
	if err != nil {
		return PrivateClaimRegistryDeployment{}, err
	}
	return result.(PrivateClaimRegistryDeployment), nil
}

func (s *MemgraphStore) ClaimNextPrivateClaimRegistryDeployment(ctx context.Context) (PrivateClaimRegistryDeployment, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp)-[:HAS_PRIVATE_CLAIM_REGISTRY_DEPLOYMENT]->(d:PrivateClaimRegistryDeployment)
WHERE coalesce(d.status, "queued") = "queued" AND coalesce(d.hiddenFromDashboard, false) = false
WITH d ORDER BY d.createdAt ASC LIMIT 1
SET d.status = "deploying", d.updatedAt = $now, d.error = ""
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name,
  d.ownerAddress AS ownerAddress, d.verifierAddress AS verifierAddress,
  d.chainId AS chainId, coalesce(d.description, "") AS description,
  coalesce(d.status, "queued") AS status, coalesce(d.contractAddress, "") AS contractAddress,
  coalesce(d.transactionHash, "") AS transactionHash, coalesce(d.error, "") AS error,
  coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt,
  coalesce(d.queuedAt, "") AS queuedAt
`, map[string]any{"now": time.Now().UTC().Format(time.RFC3339)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return privateClaimRegistryDeploymentFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return PrivateClaimRegistryDeployment{}, false, err
	}
	if result == nil {
		return PrivateClaimRegistryDeployment{}, false, nil
	}
	return result.(PrivateClaimRegistryDeployment), true, nil
}

func (s *MemgraphStore) MarkPrivateClaimRegistryDeploymentSubmitted(ctx context.Context, deploymentID string, transactionHash string) error {
	return s.updatePrivateClaimRegistryDeploymentStatus(ctx, deploymentID, "submitted", transactionHash, "", "", "")
}

func (s *MemgraphStore) MarkPrivateClaimRegistryDeploymentConfirmed(ctx context.Context, deploymentID string, contractAddress string, abi string) error {
	return s.updatePrivateClaimRegistryDeploymentStatus(ctx, deploymentID, "confirmed", "", strings.ToLower(strings.TrimSpace(contractAddress)), "", abi)
}

func (s *MemgraphStore) MarkPrivateClaimRegistryDeploymentFailed(ctx context.Context, deploymentID string, message string) error {
	return s.updatePrivateClaimRegistryDeploymentStatus(ctx, deploymentID, "failed", "", "", message, "")
}

func (s *MemgraphStore) updatePrivateClaimRegistryDeploymentStatus(ctx context.Context, deploymentID string, status string, transactionHash string, contractAddress string, message string, abi string) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
MATCH (d:PrivateClaimRegistryDeployment {id: $deploymentID})
SET d.status = $status, d.updatedAt = $now
SET d.transactionHash = CASE WHEN $transactionHash <> "" THEN $transactionHash ELSE coalesce(d.transactionHash, "") END
SET d.contractAddress = CASE WHEN $contractAddress <> "" THEN $contractAddress ELSE coalesce(d.contractAddress, "") END
SET d.error = CASE WHEN $error <> "" THEN $error ELSE "" END
SET d.abi = CASE WHEN $abi <> "" THEN $abi ELSE coalesce(d.abi, "") END
RETURN d.id AS id
`, map[string]any{
			"deploymentID":    strings.TrimSpace(deploymentID),
			"status":          strings.TrimSpace(status),
			"transactionHash": strings.TrimSpace(transactionHash),
			"contractAddress": strings.TrimSpace(contractAddress),
			"error":           strings.TrimSpace(message),
			"abi":             strings.TrimSpace(abi),
			"now":             time.Now().UTC().Format(time.RFC3339),
		})
		return nil, err
	})
	return err
}

func (s *MemgraphStore) CreateShieldedPayoutPoolDeployment(ctx context.Context, input ShieldedPayoutPoolDeploymentInput) (ShieldedPayoutPoolDeployment, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = "Shielded Payout Pool"
	}
	if input.ChainID <= 0 {
		return ShieldedPayoutPoolDeployment{}, errors.New("chainId is required")
	}
	tokenAddress := strings.ToLower(strings.TrimSpace(input.TokenAddress))
	if tokenAddress == "" {
		return ShieldedPayoutPoolDeployment{}, errors.New("token address is required")
	}
	ownerAddress := strings.ToLower(strings.TrimSpace(input.OwnerAddress))
	if ownerAddress == "" {
		return ShieldedPayoutPoolDeployment{}, errors.New("owner address is required")
	}
	verifierAddress := strings.ToLower(strings.TrimSpace(input.VerifierAddress))
	if verifierAddress == "" {
		verifierAddress = "0x0000000000000000000000000000000000000000"
	}
	denomination := strings.TrimSpace(input.Denomination)
	if denomination == "" {
		return ShieldedPayoutPoolDeployment{}, errors.New("denomination is required")
	}
	sourceName := safeContractName(name, "ShieldedPayoutPool")
	if !strings.Contains(strings.ToLower(sourceName), "pool") {
		sourceName += "Pool"
	}
	sourceCode := shieldedPayoutPoolSource(sourceName)
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"id":              uuid.NewString(),
		"appID":           strings.TrimSpace(input.AppID),
		"keyID":           strings.TrimSpace(input.KeyID),
		"name":            name,
		"tokenAddress":    tokenAddress,
		"ownerAddress":    ownerAddress,
		"verifierAddress": verifierAddress,
		"denomination":    denomination,
		"chainID":         input.ChainID,
		"description":     strings.TrimSpace(input.Description),
		"sourceName":      sourceName,
		"sourceCode":      sourceCode,
		"now":             now,
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:EngineApp {id: $appID})
CREATE (d:ShieldedPayoutPoolDeployment {
  id: $id, appId: $appID, keyId: $keyID, name: $name,
  tokenAddress: $tokenAddress, ownerAddress: $ownerAddress, verifierAddress: $verifierAddress,
  denomination: $denomination, chainId: $chainID, description: $description, status: "queued",
  contractAddress: "", transactionHash: "", error: "", sourceName: $sourceName,
  sourceCode: $sourceCode, abi: "", createdAt: $now, updatedAt: $now, queuedAt: $now
})
MERGE (a)-[:HAS_SHIELDED_PAYOUT_POOL_DEPLOYMENT]->(d)
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name,
  d.tokenAddress AS tokenAddress, d.ownerAddress AS ownerAddress, d.verifierAddress AS verifierAddress,
  d.denomination AS denomination, d.chainId AS chainId, coalesce(d.description, "") AS description,
  coalesce(d.status, "queued") AS status, coalesce(d.contractAddress, "") AS contractAddress,
  coalesce(d.transactionHash, "") AS transactionHash, coalesce(d.error, "") AS error,
  coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt,
  coalesce(d.queuedAt, "") AS queuedAt
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return shieldedPayoutPoolDeploymentFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("engine app not found")
	})
	if err != nil {
		return ShieldedPayoutPoolDeployment{}, err
	}
	return result.(ShieldedPayoutPoolDeployment), nil
}

func (s *MemgraphStore) ListShieldedPayoutPoolDeployments(ctx context.Context, appID string, limit int64) ([]ShieldedPayoutPoolDeployment, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp {id: $appID})-[:HAS_SHIELDED_PAYOUT_POOL_DEPLOYMENT]->(d:ShieldedPayoutPoolDeployment)
WHERE coalesce(d.hiddenFromDashboard, false) = false
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name,
  d.tokenAddress AS tokenAddress, d.ownerAddress AS ownerAddress, d.verifierAddress AS verifierAddress,
  d.denomination AS denomination, d.chainId AS chainId, coalesce(d.description, "") AS description,
  coalesce(d.status, "queued") AS status, coalesce(d.contractAddress, "") AS contractAddress,
  coalesce(d.transactionHash, "") AS transactionHash, coalesce(d.error, "") AS error,
  coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt,
  coalesce(d.queuedAt, "") AS queuedAt
ORDER BY d.createdAt DESC
LIMIT $limit
`, map[string]any{"appID": strings.TrimSpace(appID), "limit": limit})
		if err != nil {
			return nil, err
		}
		deployments := []ShieldedPayoutPoolDeployment{}
		for rows.Next(ctx) {
			deployments = append(deployments, shieldedPayoutPoolDeploymentFromRecord(rows.Record()))
		}
		return deployments, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]ShieldedPayoutPoolDeployment), nil
}

func (s *MemgraphStore) RemoveShieldedPayoutPoolDeploymentFromDashboard(ctx context.Context, accountID string, deploymentID string) (ShieldedPayoutPoolDeployment, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:GMRAccount {id: $accountID})-[:OWNS_ENGINE_APP]->(:EngineApp)-[:HAS_SHIELDED_PAYOUT_POOL_DEPLOYMENT]->(d:ShieldedPayoutPoolDeployment {id: $deploymentID})
WHERE coalesce(d.hiddenFromDashboard, false) = false AND NOT coalesce(d.status, "queued") IN ["deploying", "submitted"]
SET d.hiddenFromDashboard = true, d.removedAt = $now, d.updatedAt = $now
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name,
  d.tokenAddress AS tokenAddress, d.ownerAddress AS ownerAddress, d.verifierAddress AS verifierAddress,
  d.denomination AS denomination, d.chainId AS chainId, coalesce(d.description, "") AS description,
  coalesce(d.status, "queued") AS status, coalesce(d.contractAddress, "") AS contractAddress,
  coalesce(d.transactionHash, "") AS transactionHash, coalesce(d.error, "") AS error,
  coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt,
  coalesce(d.queuedAt, "") AS queuedAt
`, map[string]any{
			"accountID":    strings.TrimSpace(accountID),
			"deploymentID": strings.TrimSpace(deploymentID),
			"now":          time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return shieldedPayoutPoolDeploymentFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("removable deployment not found")
	})
	if err != nil {
		return ShieldedPayoutPoolDeployment{}, err
	}
	return result.(ShieldedPayoutPoolDeployment), nil
}

func (s *MemgraphStore) ClaimNextShieldedPayoutPoolDeployment(ctx context.Context) (ShieldedPayoutPoolDeployment, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp)-[:HAS_SHIELDED_PAYOUT_POOL_DEPLOYMENT]->(d:ShieldedPayoutPoolDeployment)
WHERE coalesce(d.status, "queued") = "queued" AND coalesce(d.hiddenFromDashboard, false) = false
WITH d ORDER BY d.createdAt ASC LIMIT 1
SET d.status = "deploying", d.updatedAt = $now, d.error = ""
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name,
  d.tokenAddress AS tokenAddress, d.ownerAddress AS ownerAddress, d.verifierAddress AS verifierAddress,
  d.denomination AS denomination, d.chainId AS chainId, coalesce(d.description, "") AS description,
  coalesce(d.status, "queued") AS status, coalesce(d.contractAddress, "") AS contractAddress,
  coalesce(d.transactionHash, "") AS transactionHash, coalesce(d.error, "") AS error,
  coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt,
  coalesce(d.queuedAt, "") AS queuedAt
`, map[string]any{"now": time.Now().UTC().Format(time.RFC3339)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return shieldedPayoutPoolDeploymentFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return ShieldedPayoutPoolDeployment{}, false, err
	}
	if result == nil {
		return ShieldedPayoutPoolDeployment{}, false, nil
	}
	return result.(ShieldedPayoutPoolDeployment), true, nil
}

func (s *MemgraphStore) MarkShieldedPayoutPoolDeploymentSubmitted(ctx context.Context, deploymentID string, transactionHash string) error {
	return s.updateShieldedPayoutPoolDeploymentStatus(ctx, deploymentID, "submitted", transactionHash, "", "", "")
}

func (s *MemgraphStore) MarkShieldedPayoutPoolDeploymentConfirmed(ctx context.Context, deploymentID string, contractAddress string, abi string) error {
	return s.updateShieldedPayoutPoolDeploymentStatus(ctx, deploymentID, "confirmed", "", strings.ToLower(strings.TrimSpace(contractAddress)), "", abi)
}

func (s *MemgraphStore) MarkShieldedPayoutPoolDeploymentFailed(ctx context.Context, deploymentID string, message string) error {
	return s.updateShieldedPayoutPoolDeploymentStatus(ctx, deploymentID, "failed", "", "", message, "")
}

func (s *MemgraphStore) updateShieldedPayoutPoolDeploymentStatus(ctx context.Context, deploymentID string, status string, transactionHash string, contractAddress string, message string, abi string) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
MATCH (d:ShieldedPayoutPoolDeployment {id: $deploymentID})
SET d.status = $status, d.updatedAt = $now
SET d.transactionHash = CASE WHEN $transactionHash <> "" THEN $transactionHash ELSE coalesce(d.transactionHash, "") END
SET d.contractAddress = CASE WHEN $contractAddress <> "" THEN $contractAddress ELSE coalesce(d.contractAddress, "") END
SET d.error = CASE WHEN $error <> "" THEN $error ELSE "" END
SET d.abi = CASE WHEN $abi <> "" THEN $abi ELSE coalesce(d.abi, "") END
RETURN d.id AS id
`, map[string]any{
			"deploymentID":    strings.TrimSpace(deploymentID),
			"status":          strings.TrimSpace(status),
			"transactionHash": strings.TrimSpace(transactionHash),
			"contractAddress": strings.TrimSpace(contractAddress),
			"error":           strings.TrimSpace(message),
			"abi":             strings.TrimSpace(abi),
			"now":             time.Now().UTC().Format(time.RFC3339),
		})
		return nil, err
	})
	return err
}

const privacyAccessPassDeploymentReturn = `
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name,
  d.tokenAddress AS tokenAddress, d.ownerAddress AS ownerAddress, d.treasuryAddress AS treasuryAddress,
  d.initialAllowedFee AS initialAllowedFee, d.chainId AS chainId, coalesce(d.description, "") AS description,
  coalesce(d.status, "queued") AS status, coalesce(d.contractAddress, "") AS contractAddress,
  coalesce(d.transactionHash, "") AS transactionHash, coalesce(d.error, "") AS error,
  coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt,
  coalesce(d.queuedAt, "") AS queuedAt`

func normalizePrivacyAccessPassFee(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "0", nil
	}
	fee, ok := new(big.Int).SetString(value, 10)
	if !ok || fee.Sign() < 0 {
		return "", errors.New("initialAllowedFee must be a non-negative base-unit integer")
	}
	return fee.String(), nil
}

func (s *MemgraphStore) CreatePrivacyAccessPassDeployment(ctx context.Context, input PrivacyAccessPassDeploymentInput) (PrivacyAccessPassDeployment, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = "Privacy Access Pass"
	}
	if input.ChainID <= 0 {
		return PrivacyAccessPassDeployment{}, errors.New("chainId is required")
	}
	tokenAddress := strings.ToLower(strings.TrimSpace(input.TokenAddress))
	if tokenAddress == "" {
		return PrivacyAccessPassDeployment{}, errors.New("token address is required")
	}
	ownerAddress := strings.ToLower(strings.TrimSpace(input.OwnerAddress))
	if ownerAddress == "" {
		return PrivacyAccessPassDeployment{}, errors.New("owner address is required")
	}
	treasuryAddress := strings.ToLower(strings.TrimSpace(input.TreasuryAddress))
	if treasuryAddress == "" {
		return PrivacyAccessPassDeployment{}, errors.New("treasury address is required")
	}
	initialAllowedFee, err := normalizePrivacyAccessPassFee(input.InitialAllowedFee)
	if err != nil {
		return PrivacyAccessPassDeployment{}, err
	}
	sourceName := safeContractName(name, "PrivacyAccessPass")
	if !strings.Contains(strings.ToLower(sourceName), "pass") {
		sourceName += "Pass"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"id": uuid.NewString(), "appID": strings.TrimSpace(input.AppID), "keyID": strings.TrimSpace(input.KeyID),
		"name": name, "tokenAddress": tokenAddress, "ownerAddress": ownerAddress,
		"treasuryAddress": treasuryAddress, "initialAllowedFee": initialAllowedFee,
		"chainID": input.ChainID, "description": strings.TrimSpace(input.Description),
		"sourceName": sourceName, "sourceCode": contracts.PrivacyAccessPassSource(sourceName), "now": now,
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:EngineApp {id: $appID})
CREATE (d:PrivacyAccessPassDeployment {
  id: $id, appId: $appID, keyId: $keyID, name: $name, tokenAddress: $tokenAddress,
  ownerAddress: $ownerAddress, treasuryAddress: $treasuryAddress, initialAllowedFee: $initialAllowedFee,
  chainId: $chainID, description: $description, status: "queued", contractAddress: "",
  transactionHash: "", error: "", sourceName: $sourceName, sourceCode: $sourceCode,
  abi: "", createdAt: $now, updatedAt: $now, queuedAt: $now
})
MERGE (a)-[:HAS_PRIVACY_ACCESS_PASS_DEPLOYMENT]->(d)`+privacyAccessPassDeploymentReturn, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return privacyAccessPassDeploymentFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("engine app not found")
	})
	if err != nil {
		return PrivacyAccessPassDeployment{}, err
	}
	return result.(PrivacyAccessPassDeployment), nil
}

func (s *MemgraphStore) ListPrivacyAccessPassDeployments(ctx context.Context, appID string, limit int64) ([]PrivacyAccessPassDeployment, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp {id: $appID})-[:HAS_PRIVACY_ACCESS_PASS_DEPLOYMENT]->(d:PrivacyAccessPassDeployment)
WHERE coalesce(d.hiddenFromDashboard, false) = false`+privacyAccessPassDeploymentReturn+`
ORDER BY d.createdAt DESC
LIMIT $limit`, map[string]any{"appID": strings.TrimSpace(appID), "limit": limit})
		if err != nil {
			return nil, err
		}
		deployments := []PrivacyAccessPassDeployment{}
		for rows.Next(ctx) {
			deployments = append(deployments, privacyAccessPassDeploymentFromRecord(rows.Record()))
		}
		return deployments, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]PrivacyAccessPassDeployment), nil
}

func (s *MemgraphStore) RemovePrivacyAccessPassDeploymentFromDashboard(ctx context.Context, accountID string, deploymentID string) (PrivacyAccessPassDeployment, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:GMRAccount {id: $accountID})-[:OWNS_ENGINE_APP]->(:EngineApp)-[:HAS_PRIVACY_ACCESS_PASS_DEPLOYMENT]->(d:PrivacyAccessPassDeployment {id: $deploymentID})
WHERE coalesce(d.hiddenFromDashboard, false) = false AND NOT coalesce(d.status, "queued") IN ["deploying", "submitted"]
SET d.hiddenFromDashboard = true, d.removedAt = $now, d.updatedAt = $now`+privacyAccessPassDeploymentReturn, map[string]any{
			"accountID": strings.TrimSpace(accountID), "deploymentID": strings.TrimSpace(deploymentID),
			"now": time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return privacyAccessPassDeploymentFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("removable deployment not found")
	})
	if err != nil {
		return PrivacyAccessPassDeployment{}, err
	}
	return result.(PrivacyAccessPassDeployment), nil
}

func (s *MemgraphStore) ClaimNextPrivacyAccessPassDeployment(ctx context.Context) (PrivacyAccessPassDeployment, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp)-[:HAS_PRIVACY_ACCESS_PASS_DEPLOYMENT]->(d:PrivacyAccessPassDeployment)
WHERE coalesce(d.status, "queued") = "queued" AND coalesce(d.hiddenFromDashboard, false) = false
WITH d ORDER BY d.createdAt ASC LIMIT 1
SET d.status = "deploying", d.updatedAt = $now, d.error = ""`+privacyAccessPassDeploymentReturn,
			map[string]any{"now": time.Now().UTC().Format(time.RFC3339)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return privacyAccessPassDeploymentFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return PrivacyAccessPassDeployment{}, false, err
	}
	if result == nil {
		return PrivacyAccessPassDeployment{}, false, nil
	}
	return result.(PrivacyAccessPassDeployment), true, nil
}

func (s *MemgraphStore) MarkPrivacyAccessPassDeploymentSubmitted(ctx context.Context, deploymentID string, transactionHash string) error {
	return s.updatePrivacyAccessPassDeploymentStatus(ctx, deploymentID, "submitted", transactionHash, "", "", "")
}

func (s *MemgraphStore) MarkPrivacyAccessPassDeploymentConfirmed(ctx context.Context, deploymentID string, contractAddress string, abi string) error {
	return s.updatePrivacyAccessPassDeploymentStatus(ctx, deploymentID, "confirmed", "", strings.ToLower(strings.TrimSpace(contractAddress)), "", abi)
}

func (s *MemgraphStore) MarkPrivacyAccessPassDeploymentFailed(ctx context.Context, deploymentID string, message string) error {
	return s.updatePrivacyAccessPassDeploymentStatus(ctx, deploymentID, "failed", "", "", message, "")
}

func (s *MemgraphStore) updatePrivacyAccessPassDeploymentStatus(ctx context.Context, deploymentID string, status string, transactionHash string, contractAddress string, message string, abi string) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
MATCH (d:PrivacyAccessPassDeployment {id: $deploymentID})
SET d.status = $status, d.updatedAt = $now
SET d.transactionHash = CASE WHEN $transactionHash <> "" THEN $transactionHash ELSE coalesce(d.transactionHash, "") END
SET d.contractAddress = CASE WHEN $contractAddress <> "" THEN $contractAddress ELSE coalesce(d.contractAddress, "") END
SET d.error = CASE WHEN $error <> "" THEN $error ELSE "" END
SET d.abi = CASE WHEN $abi <> "" THEN $abi ELSE coalesce(d.abi, "") END
RETURN d.id AS id`, map[string]any{
			"deploymentID": strings.TrimSpace(deploymentID), "status": strings.TrimSpace(status),
			"transactionHash": strings.TrimSpace(transactionHash), "contractAddress": strings.TrimSpace(contractAddress),
			"error": strings.TrimSpace(message), "abi": strings.TrimSpace(abi),
			"now": time.Now().UTC().Format(time.RFC3339),
		})
		return nil, err
	})
	return err
}

func (s *MemgraphStore) CreateShieldedWithdrawalVerifierDeployment(ctx context.Context, input ShieldedWithdrawalVerifierDeploymentInput) (ShieldedWithdrawalVerifierDeployment, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = "Shielded Withdrawal Verifier"
	}
	if input.ChainID <= 0 {
		return ShieldedWithdrawalVerifierDeployment{}, errors.New("chainId is required")
	}
	sourceName := safeContractName(name, "ShieldedWithdrawalVerifier")
	if !strings.Contains(strings.ToLower(sourceName), "verifier") {
		sourceName += "Verifier"
	}
	sourceCode := shieldedWithdrawalVerifierSource(sourceName)
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"id":          uuid.NewString(),
		"appID":       strings.TrimSpace(input.AppID),
		"keyID":       strings.TrimSpace(input.KeyID),
		"name":        name,
		"chainID":     input.ChainID,
		"description": strings.TrimSpace(input.Description),
		"sourceName":  sourceName,
		"sourceCode":  sourceCode,
		"now":         now,
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:EngineApp {id: $appID})
CREATE (d:ShieldedWithdrawalVerifierDeployment {
  id: $id, appId: $appID, keyId: $keyID, name: $name,
  chainId: $chainID, description: $description, status: "queued",
  contractAddress: "", transactionHash: "", error: "", sourceName: $sourceName,
  sourceCode: $sourceCode, abi: "", createdAt: $now, updatedAt: $now, queuedAt: $now
})
MERGE (a)-[:HAS_SHIELDED_WITHDRAWAL_VERIFIER_DEPLOYMENT]->(d)
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name,
  d.chainId AS chainId, coalesce(d.description, "") AS description,
  coalesce(d.status, "queued") AS status, coalesce(d.contractAddress, "") AS contractAddress,
  coalesce(d.transactionHash, "") AS transactionHash, coalesce(d.error, "") AS error,
  coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt,
  coalesce(d.queuedAt, "") AS queuedAt
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return shieldedWithdrawalVerifierDeploymentFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("engine app not found")
	})
	if err != nil {
		return ShieldedWithdrawalVerifierDeployment{}, err
	}
	return result.(ShieldedWithdrawalVerifierDeployment), nil
}

func (s *MemgraphStore) ListShieldedWithdrawalVerifierDeployments(ctx context.Context, appID string, limit int64) ([]ShieldedWithdrawalVerifierDeployment, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp {id: $appID})-[:HAS_SHIELDED_WITHDRAWAL_VERIFIER_DEPLOYMENT]->(d:ShieldedWithdrawalVerifierDeployment)
WHERE coalesce(d.hiddenFromDashboard, false) = false
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name,
  d.chainId AS chainId, coalesce(d.description, "") AS description,
  coalesce(d.status, "queued") AS status, coalesce(d.contractAddress, "") AS contractAddress,
  coalesce(d.transactionHash, "") AS transactionHash, coalesce(d.error, "") AS error,
  coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt,
  coalesce(d.queuedAt, "") AS queuedAt
ORDER BY d.createdAt DESC
LIMIT $limit
`, map[string]any{"appID": strings.TrimSpace(appID), "limit": limit})
		if err != nil {
			return nil, err
		}
		deployments := []ShieldedWithdrawalVerifierDeployment{}
		for rows.Next(ctx) {
			deployments = append(deployments, shieldedWithdrawalVerifierDeploymentFromRecord(rows.Record()))
		}
		return deployments, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]ShieldedWithdrawalVerifierDeployment), nil
}

func (s *MemgraphStore) RemoveShieldedWithdrawalVerifierDeploymentFromDashboard(ctx context.Context, accountID string, deploymentID string) (ShieldedWithdrawalVerifierDeployment, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:GMRAccount {id: $accountID})-[:OWNS_ENGINE_APP]->(:EngineApp)-[:HAS_SHIELDED_WITHDRAWAL_VERIFIER_DEPLOYMENT]->(d:ShieldedWithdrawalVerifierDeployment {id: $deploymentID})
WHERE coalesce(d.hiddenFromDashboard, false) = false AND NOT coalesce(d.status, "queued") IN ["deploying", "submitted"]
SET d.hiddenFromDashboard = true, d.removedAt = $now, d.updatedAt = $now
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name,
  d.chainId AS chainId, coalesce(d.description, "") AS description,
  coalesce(d.status, "queued") AS status, coalesce(d.contractAddress, "") AS contractAddress,
  coalesce(d.transactionHash, "") AS transactionHash, coalesce(d.error, "") AS error,
  coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt,
  coalesce(d.queuedAt, "") AS queuedAt
`, map[string]any{
			"accountID":    strings.TrimSpace(accountID),
			"deploymentID": strings.TrimSpace(deploymentID),
			"now":          time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return shieldedWithdrawalVerifierDeploymentFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("removable deployment not found")
	})
	if err != nil {
		return ShieldedWithdrawalVerifierDeployment{}, err
	}
	return result.(ShieldedWithdrawalVerifierDeployment), nil
}

func (s *MemgraphStore) ClaimNextShieldedWithdrawalVerifierDeployment(ctx context.Context) (ShieldedWithdrawalVerifierDeployment, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp)-[:HAS_SHIELDED_WITHDRAWAL_VERIFIER_DEPLOYMENT]->(d:ShieldedWithdrawalVerifierDeployment)
WHERE coalesce(d.status, "queued") = "queued" AND coalesce(d.hiddenFromDashboard, false) = false
WITH d ORDER BY d.createdAt ASC LIMIT 1
SET d.status = "deploying", d.updatedAt = $now, d.error = ""
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name,
  d.chainId AS chainId, coalesce(d.description, "") AS description,
  coalesce(d.status, "queued") AS status, coalesce(d.contractAddress, "") AS contractAddress,
  coalesce(d.transactionHash, "") AS transactionHash, coalesce(d.error, "") AS error,
  coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt,
  coalesce(d.queuedAt, "") AS queuedAt
`, map[string]any{"now": time.Now().UTC().Format(time.RFC3339)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return shieldedWithdrawalVerifierDeploymentFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return ShieldedWithdrawalVerifierDeployment{}, false, err
	}
	if result == nil {
		return ShieldedWithdrawalVerifierDeployment{}, false, nil
	}
	return result.(ShieldedWithdrawalVerifierDeployment), true, nil
}

func (s *MemgraphStore) MarkShieldedWithdrawalVerifierDeploymentSubmitted(ctx context.Context, deploymentID string, transactionHash string) error {
	return s.updateShieldedWithdrawalVerifierDeploymentStatus(ctx, deploymentID, "submitted", transactionHash, "", "", "")
}

func (s *MemgraphStore) MarkShieldedWithdrawalVerifierDeploymentConfirmed(ctx context.Context, deploymentID string, contractAddress string, abi string) error {
	return s.updateShieldedWithdrawalVerifierDeploymentStatus(ctx, deploymentID, "confirmed", "", strings.ToLower(strings.TrimSpace(contractAddress)), "", abi)
}

func (s *MemgraphStore) MarkShieldedWithdrawalVerifierDeploymentFailed(ctx context.Context, deploymentID string, message string) error {
	return s.updateShieldedWithdrawalVerifierDeploymentStatus(ctx, deploymentID, "failed", "", "", message, "")
}

func (s *MemgraphStore) updateShieldedWithdrawalVerifierDeploymentStatus(ctx context.Context, deploymentID string, status string, transactionHash string, contractAddress string, message string, abi string) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
MATCH (d:ShieldedWithdrawalVerifierDeployment {id: $deploymentID})
SET d.status = $status, d.updatedAt = $now
SET d.transactionHash = CASE WHEN $transactionHash <> "" THEN $transactionHash ELSE coalesce(d.transactionHash, "") END
SET d.contractAddress = CASE WHEN $contractAddress <> "" THEN $contractAddress ELSE coalesce(d.contractAddress, "") END
SET d.error = CASE WHEN $error <> "" THEN $error ELSE "" END
SET d.abi = CASE WHEN $abi <> "" THEN $abi ELSE coalesce(d.abi, "") END
RETURN d.id AS id
`, map[string]any{
			"deploymentID":    strings.TrimSpace(deploymentID),
			"status":          strings.TrimSpace(status),
			"transactionHash": strings.TrimSpace(transactionHash),
			"contractAddress": strings.TrimSpace(contractAddress),
			"error":           strings.TrimSpace(message),
			"abi":             strings.TrimSpace(abi),
			"now":             time.Now().UTC().Format(time.RFC3339),
		})
		return nil, err
	})
	return err
}

func (s *MemgraphStore) CreateAccountAbstractionDeployment(ctx context.Context, input AccountAbstractionDeploymentInput) (AccountAbstractionDeployment, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = "Account Abstraction"
	}
	if input.ChainID != 2651420 {
		return AccountAbstractionDeployment{}, errors.New("AA deployment currently supports Horizen Testnet chainId 2651420")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"id":                uuid.NewString(),
		"appID":             strings.TrimSpace(input.AppID),
		"keyID":             strings.TrimSpace(input.KeyID),
		"name":              name,
		"chainID":           input.ChainID,
		"description":       strings.TrimSpace(input.Description),
		"entryPointAddress": strings.ToLower(strings.TrimSpace(input.EntryPointAddress)),
		"bundlerURL":        strings.TrimSpace(input.BundlerURL),
		"version":           "0.8",
		"now":               now,
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:EngineApp {id: $appID})
CREATE (d:AccountAbstractionDeployment {
  id: $id, appId: $appID, keyId: $keyID, name: $name, chainId: $chainID,
  description: $description, status: "queued", contractAddress: "",
  transactionHash: "", entryPointAddress: $entryPointAddress, entryPointTransactionHash: "",
  factoryAddress: "", factoryTransactionHash: "", bundlerUrl: $bundlerURL, version: $version,
  error: "", createdAt: $now, updatedAt: $now, queuedAt: $now
})
MERGE (a)-[:HAS_ACCOUNT_ABSTRACTION_DEPLOYMENT]->(d)
`+accountAbstractionDeploymentReturn()+`
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return accountAbstractionDeploymentFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("engine app not found")
	})
	if err != nil {
		return AccountAbstractionDeployment{}, err
	}
	return result.(AccountAbstractionDeployment), nil
}

func (s *MemgraphStore) ListAccountAbstractionDeployments(ctx context.Context, appID string, limit int64) ([]AccountAbstractionDeployment, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp {id: $appID})-[:HAS_ACCOUNT_ABSTRACTION_DEPLOYMENT]->(d:AccountAbstractionDeployment)
WHERE coalesce(d.hiddenFromDashboard, false) = false
`+accountAbstractionDeploymentReturn()+`
ORDER BY d.createdAt DESC
LIMIT $limit
`, map[string]any{"appID": strings.TrimSpace(appID), "limit": limit})
		if err != nil {
			return nil, err
		}
		deployments := []AccountAbstractionDeployment{}
		for rows.Next(ctx) {
			deployments = append(deployments, accountAbstractionDeploymentFromRecord(rows.Record()))
		}
		return deployments, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]AccountAbstractionDeployment), nil
}

func (s *MemgraphStore) RemoveAccountAbstractionDeploymentFromDashboard(ctx context.Context, accountID string, deploymentID string) (AccountAbstractionDeployment, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:GMRAccount {id: $accountID})-[:OWNS_ENGINE_APP]->(:EngineApp)-[:HAS_ACCOUNT_ABSTRACTION_DEPLOYMENT]->(d:AccountAbstractionDeployment {id: $deploymentID})
WHERE coalesce(d.hiddenFromDashboard, false) = false AND NOT coalesce(d.status, "queued") IN ["deploying", "submitted"]
SET d.hiddenFromDashboard = true, d.removedAt = $now, d.updatedAt = $now
`+accountAbstractionDeploymentReturn()+`
`, map[string]any{
			"accountID":    strings.TrimSpace(accountID),
			"deploymentID": strings.TrimSpace(deploymentID),
			"now":          time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return accountAbstractionDeploymentFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("removable deployment not found")
	})
	if err != nil {
		return AccountAbstractionDeployment{}, err
	}
	return result.(AccountAbstractionDeployment), nil
}

func (s *MemgraphStore) ClaimNextAccountAbstractionDeployment(ctx context.Context) (AccountAbstractionDeployment, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp)-[:HAS_ACCOUNT_ABSTRACTION_DEPLOYMENT]->(d:AccountAbstractionDeployment)
WHERE coalesce(d.status, "queued") = "queued" AND coalesce(d.hiddenFromDashboard, false) = false
WITH d ORDER BY d.createdAt ASC LIMIT 1
SET d.status = "deploying", d.updatedAt = $now, d.error = ""
`+accountAbstractionDeploymentReturn()+`
`, map[string]any{"now": time.Now().UTC().Format(time.RFC3339)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return accountAbstractionDeploymentFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return AccountAbstractionDeployment{}, false, err
	}
	if result == nil {
		return AccountAbstractionDeployment{}, false, nil
	}
	return result.(AccountAbstractionDeployment), true, nil
}

func (s *MemgraphStore) MarkAccountAbstractionDeploymentSubmitted(ctx context.Context, deploymentID string, transactionHash string) error {
	return s.updateAccountAbstractionDeploymentStatus(ctx, deploymentID, "submitted", transactionHash, AccountAbstractionDeployment{}, "")
}

func (s *MemgraphStore) MarkAccountAbstractionDeploymentConfirmed(ctx context.Context, deploymentID string, deployment AccountAbstractionDeployment) error {
	return s.updateAccountAbstractionDeploymentStatus(ctx, deploymentID, "confirmed", "", deployment, "")
}

func (s *MemgraphStore) MarkAccountAbstractionDeploymentFailed(ctx context.Context, deploymentID string, message string) error {
	return s.updateAccountAbstractionDeploymentStatus(ctx, deploymentID, "failed", "", AccountAbstractionDeployment{}, message)
}

func (s *MemgraphStore) updateAccountAbstractionDeploymentStatus(ctx context.Context, deploymentID string, status string, transactionHash string, deployment AccountAbstractionDeployment, message string) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
MATCH (d:AccountAbstractionDeployment {id: $deploymentID})
SET d.status = $status, d.updatedAt = $now
SET d.transactionHash = CASE WHEN $transactionHash <> "" THEN $transactionHash ELSE coalesce(d.transactionHash, "") END
SET d.contractAddress = CASE WHEN $contractAddress <> "" THEN $contractAddress ELSE coalesce(d.contractAddress, "") END
SET d.entryPointAddress = CASE WHEN $entryPointAddress <> "" THEN $entryPointAddress ELSE coalesce(d.entryPointAddress, "") END
SET d.entryPointTransactionHash = CASE WHEN $entryPointTransactionHash <> "" THEN $entryPointTransactionHash ELSE coalesce(d.entryPointTransactionHash, "") END
SET d.factoryAddress = CASE WHEN $factoryAddress <> "" THEN $factoryAddress ELSE coalesce(d.factoryAddress, "") END
SET d.factoryTransactionHash = CASE WHEN $factoryTransactionHash <> "" THEN $factoryTransactionHash ELSE coalesce(d.factoryTransactionHash, "") END
SET d.bundlerUrl = CASE WHEN $bundlerURL <> "" THEN $bundlerURL ELSE coalesce(d.bundlerUrl, "") END
SET d.version = CASE WHEN $version <> "" THEN $version ELSE coalesce(d.version, "0.8") END
SET d.error = CASE WHEN $error <> "" THEN $error ELSE "" END
RETURN d.id AS id
`, map[string]any{
			"deploymentID":              strings.TrimSpace(deploymentID),
			"status":                    strings.TrimSpace(status),
			"transactionHash":           strings.TrimSpace(transactionHash),
			"contractAddress":           strings.ToLower(strings.TrimSpace(deployment.ContractAddress)),
			"entryPointAddress":         strings.ToLower(strings.TrimSpace(deployment.EntryPointAddress)),
			"entryPointTransactionHash": strings.TrimSpace(deployment.EntryPointTransactionHash),
			"factoryAddress":            strings.ToLower(strings.TrimSpace(deployment.FactoryAddress)),
			"factoryTransactionHash":    strings.TrimSpace(deployment.FactoryTransactionHash),
			"bundlerURL":                strings.TrimSpace(deployment.BundlerURL),
			"version":                   strings.TrimSpace(deployment.Version),
			"error":                     strings.TrimSpace(message),
			"now":                       time.Now().UTC().Format(time.RFC3339),
		})
		return nil, err
	})
	return err
}

func (s *MemgraphStore) CreateZKProofSubmission(ctx context.Context, input ZKProofSubmissionInput) (ZKProofSubmission, error) {
	proofSystem := strings.ToLower(strings.TrimSpace(input.ProofSystem))
	if proofSystem == "" {
		proofSystem = "groth16"
	}
	if strings.TrimSpace(input.VK) == "" {
		return ZKProofSubmission{}, errors.New("verification key is required")
	}
	if strings.TrimSpace(input.Proof) == "" {
		return ZKProofSubmission{}, errors.New("proof is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"id":            uuid.NewString(),
		"appID":         strings.TrimSpace(input.AppID),
		"keyID":         strings.TrimSpace(input.KeyID),
		"proofSystem":   proofSystem,
		"domainID":      input.DomainID,
		"vk":            strings.TrimSpace(input.VK),
		"proof":         strings.TrimSpace(input.Proof),
		"publicSignals": strings.TrimSpace(input.PublicSignals),
		"context":       strings.TrimSpace(input.Context),
		"now":           now,
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:EngineApp {id: $appID})
CREATE (p:ZKProofSubmission {
  id: $id, appId: $appID, keyId: $keyID, proofSystem: $proofSystem, domainId: $domainID,
  vk: $vk, proof: $proof, publicSignals: $publicSignals, context: $context, status: "queued",
  zkVerifyNetwork: "", accountAddress: "", transactionResult: "", error: "",
  createdAt: $now, updatedAt: $now, submittedAt: "", finalizedAt: "", failedAt: ""
})
MERGE (a)-[:HAS_ZK_PROOF_SUBMISSION]->(p)
RETURN p.id AS id, p.appId AS appId, p.keyId AS keyId, p.proofSystem AS proofSystem,
  p.domainId AS domainId, p.vk AS vk, p.proof AS proof, coalesce(p.publicSignals, "") AS publicSignals,
  coalesce(p.context, "") AS context, coalesce(p.status, "queued") AS status,
  coalesce(p.zkVerifyNetwork, "") AS zkVerifyNetwork, coalesce(p.accountAddress, "") AS accountAddress,
  coalesce(p.transactionResult, "") AS transactionResult, coalesce(p.error, "") AS error,
  p.createdAt AS createdAt, p.updatedAt AS updatedAt, coalesce(p.submittedAt, "") AS submittedAt,
  coalesce(p.finalizedAt, "") AS finalizedAt, coalesce(p.failedAt, "") AS failedAt
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return zkProofSubmissionFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("engine app not found")
	})
	if err != nil {
		return ZKProofSubmission{}, err
	}
	return result.(ZKProofSubmission), nil
}

func (s *MemgraphStore) ListZKProofSubmissions(ctx context.Context, appID string, limit int64) ([]ZKProofSubmission, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp {id: $appID})-[:HAS_ZK_PROOF_SUBMISSION]->(p:ZKProofSubmission)
RETURN p.id AS id, p.appId AS appId, p.keyId AS keyId, p.proofSystem AS proofSystem,
  p.domainId AS domainId, p.vk AS vk, p.proof AS proof, coalesce(p.publicSignals, "") AS publicSignals,
  coalesce(p.context, "") AS context, coalesce(p.status, "queued") AS status,
  coalesce(p.zkVerifyNetwork, "") AS zkVerifyNetwork, coalesce(p.accountAddress, "") AS accountAddress,
  coalesce(p.transactionResult, "") AS transactionResult, coalesce(p.error, "") AS error,
  p.createdAt AS createdAt, p.updatedAt AS updatedAt, coalesce(p.submittedAt, "") AS submittedAt,
  coalesce(p.finalizedAt, "") AS finalizedAt, coalesce(p.failedAt, "") AS failedAt
ORDER BY p.createdAt DESC
LIMIT $limit
`, map[string]any{"appID": strings.TrimSpace(appID), "limit": limit})
		if err != nil {
			return nil, err
		}
		submissions := []ZKProofSubmission{}
		for rows.Next(ctx) {
			submissions = append(submissions, zkProofSubmissionFromRecord(rows.Record()))
		}
		return submissions, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]ZKProofSubmission), nil
}

func (s *MemgraphStore) GetZKProofSubmission(ctx context.Context, appID string, id string) (ZKProofSubmission, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp {id: $appID})-[:HAS_ZK_PROOF_SUBMISSION]->(p:ZKProofSubmission {id: $id})
RETURN p.id AS id, p.appId AS appId, p.keyId AS keyId, p.proofSystem AS proofSystem,
  p.domainId AS domainId, p.vk AS vk, p.proof AS proof, coalesce(p.publicSignals, "") AS publicSignals,
  coalesce(p.context, "") AS context, coalesce(p.status, "queued") AS status,
  coalesce(p.zkVerifyNetwork, "") AS zkVerifyNetwork, coalesce(p.accountAddress, "") AS accountAddress,
  coalesce(p.transactionResult, "") AS transactionResult, coalesce(p.error, "") AS error,
  p.createdAt AS createdAt, p.updatedAt AS updatedAt, coalesce(p.submittedAt, "") AS submittedAt,
  coalesce(p.finalizedAt, "") AS finalizedAt, coalesce(p.failedAt, "") AS failedAt
`, map[string]any{"appID": strings.TrimSpace(appID), "id": strings.TrimSpace(id)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return zkProofSubmissionFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	})
	if err != nil {
		return ZKProofSubmission{}, false, err
	}
	if result == nil {
		return ZKProofSubmission{}, false, nil
	}
	return result.(ZKProofSubmission), true, nil
}

func (s *MemgraphStore) MarkZKProofSubmissionSubmitted(ctx context.Context, id string, network string, accountAddress string, transactionResult string) error {
	return s.updateZKProofSubmissionStatus(ctx, id, "submitted", network, accountAddress, transactionResult, "")
}

func (s *MemgraphStore) MarkZKProofSubmissionFailed(ctx context.Context, id string, message string) error {
	return s.updateZKProofSubmissionStatus(ctx, id, "failed", "", "", "", message)
}

func (s *MemgraphStore) updateZKProofSubmissionStatus(ctx context.Context, id string, status string, network string, accountAddress string, transactionResult string, message string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
MATCH (p:ZKProofSubmission {id: $id})
SET p.status = $status,
  p.updatedAt = $now,
  p.zkVerifyNetwork = CASE WHEN $network <> "" THEN $network ELSE coalesce(p.zkVerifyNetwork, "") END,
  p.accountAddress = CASE WHEN $accountAddress <> "" THEN $accountAddress ELSE coalesce(p.accountAddress, "") END,
  p.transactionResult = CASE WHEN $transactionResult <> "" THEN $transactionResult ELSE coalesce(p.transactionResult, "") END,
  p.error = CASE WHEN $message <> "" THEN $message ELSE "" END,
  p.submittedAt = CASE WHEN $status = "submitted" THEN $now ELSE coalesce(p.submittedAt, "") END,
  p.finalizedAt = CASE WHEN $status = "submitted" THEN $now ELSE coalesce(p.finalizedAt, "") END,
  p.failedAt = CASE WHEN $status = "failed" THEN $now ELSE coalesce(p.failedAt, "") END
`, map[string]any{
			"id":                strings.TrimSpace(id),
			"status":            strings.TrimSpace(status),
			"network":           strings.TrimSpace(network),
			"accountAddress":    strings.TrimSpace(accountAddress),
			"transactionResult": strings.TrimSpace(transactionResult),
			"message":           strings.TrimSpace(message),
			"now":               now,
		})
		return nil, err
	})
	return err
}

func (s *MemgraphStore) AcquireWalletLock(ctx context.Context, appID string, walletAddress string, transactionID string, ttlSeconds int64) (WalletLock, bool, error) {
	if ttlSeconds <= 0 {
		ttlSeconds = 120
	}
	now := time.Now().UTC()
	until := now.Add(time.Duration(ttlSeconds) * time.Second).Format(time.RFC3339)
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MERGE (l:EngineWalletLock {appId: $appID, walletAddress: $walletAddress})
ON CREATE SET l.transactionId = $transactionID, l.lockedUntil = $lockedUntil, l.updatedAt = $now
WITH l
WHERE l.transactionId = $transactionID OR coalesce(l.lockedUntil, "") < $now
SET l.transactionId = $transactionID, l.lockedUntil = $lockedUntil, l.updatedAt = $now
RETURN l.appId AS appId, l.walletAddress AS walletAddress, l.transactionId AS transactionId,
  l.lockedUntil AS lockedUntil, l.updatedAt AS updatedAt
`, map[string]any{
			"appID":         strings.TrimSpace(appID),
			"walletAddress": strings.ToLower(strings.TrimSpace(walletAddress)),
			"transactionID": strings.TrimSpace(transactionID),
			"lockedUntil":   until,
			"now":           now.Format(time.RFC3339),
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return lockFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return WalletLock{}, false, err
	}
	if result == nil {
		return WalletLock{}, false, nil
	}
	return result.(WalletLock), true, nil
}

func (s *MemgraphStore) ReleaseWalletLock(ctx context.Context, appID string, walletAddress string, transactionID string) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
MATCH (l:EngineWalletLock {appId: $appID, walletAddress: $walletAddress, transactionId: $transactionID})
DETACH DELETE l
`, map[string]any{
			"appID":         strings.TrimSpace(appID),
			"walletAddress": strings.ToLower(strings.TrimSpace(walletAddress)),
			"transactionID": strings.TrimSpace(transactionID),
		})
		return nil, err
	})
	return err
}

func transactionReturnQuery(match string) string {
	return match + `
RETURN t.id AS id, t.appId AS appId, t.keyId AS keyId, coalesce(t.idempotencyKey, "") AS idempotencyKey,
  t.chainId AS chainId, t.walletAddress AS walletAddress, coalesce(t.contractAddress, "") AS contractAddress,
  coalesce(t.kind, "") AS kind, coalesce(t.method, "") AS method, coalesce(t.args, []) AS args,
  coalesce(t.metadata, "") AS metadata, coalesce(t.value, "") AS value, coalesce(t.status, "queued") AS status,
  coalesce(t.attemptCount, 0) AS attemptCount, coalesce(t.attemptLog, []) AS attemptLog, coalesce(t.error, "") AS error,
  coalesce(t.transactionHash, "") AS transactionHash, t.createdAt AS createdAt, t.updatedAt AS updatedAt,
  coalesce(t.queuedAt, "") AS queuedAt, coalesce(t.startedAt, "") AS startedAt,
  coalesce(t.submittedAt, "") AS submittedAt, coalesce(t.confirmedAt, "") AS confirmedAt,
  coalesce(t.failedAt, "") AS failedAt`
}

func accountAbstractionDeploymentReturn() string {
	return `
RETURN d.id AS id, d.appId AS appId, coalesce(d.keyId, "") AS keyId, d.name AS name,
  d.chainId AS chainId, coalesce(d.description, "") AS description,
  coalesce(d.status, "queued") AS status, coalesce(d.contractAddress, "") AS contractAddress,
  coalesce(d.transactionHash, "") AS transactionHash, coalesce(d.entryPointAddress, "") AS entryPointAddress,
  coalesce(d.entryPointTransactionHash, "") AS entryPointTransactionHash,
  coalesce(d.factoryAddress, "") AS factoryAddress, coalesce(d.factoryTransactionHash, "") AS factoryTransactionHash,
  coalesce(d.bundlerUrl, "") AS bundlerUrl, coalesce(d.version, "0.8") AS version,
  coalesce(d.error, "") AS error, d.createdAt AS createdAt, d.updatedAt AS updatedAt,
  coalesce(d.queuedAt, "") AS queuedAt`
}

func normalizeEnvironment(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "production", "live":
		return "production"
	default:
		return "development"
	}
}

func normalizeScopes(scopes []string) ([]string, error) {
	normalized := normalizeStringSlice(scopes)
	if len(normalized) == 0 {
		return []string{"transactions:read"}, nil
	}
	for _, scope := range normalized {
		if !validScopes[scope] {
			return nil, errors.New("invalid scope: " + scope)
		}
	}
	return normalized, nil
}

func normalizeRateLimit(value int64) int64 {
	if value <= 0 {
		return 60
	}
	if value > 10000 {
		return 10000
	}
	return value
}

func (s *MemgraphStore) CreateMarketplaceDeployment(ctx context.Context, input MarketplaceDeploymentInput) (MarketplaceDeployment, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = "Dual Currency Marketplace"
	}
	if input.ChainID <= 0 {
		return MarketplaceDeployment{}, errors.New("chainId is required")
	}
	ownerAddress := strings.ToLower(strings.TrimSpace(input.OwnerAddress))
	if ownerAddress == "" {
		return MarketplaceDeployment{}, errors.New("owner address is required")
	}
	feeRecipientAddress := strings.ToLower(strings.TrimSpace(input.FeeRecipientAddress))
	if feeRecipientAddress == "" {
		feeRecipientAddress = ownerAddress
	}
	if input.FeeBps < 0 || input.FeeBps > 2500 {
		return MarketplaceDeployment{}, errors.New("feeBps must be between 0 and 2500")
	}
	sourceName := safeContractName(name, "Marketplace")
	if !strings.HasSuffix(strings.ToLower(sourceName), "marketplace") {
		sourceName += "Marketplace"
	}
	sourceCode := contracts.DualCurrencyMarketplaceSource(sourceName)
	now := time.Now().UTC().Format(time.RFC3339)
	params := map[string]any{
		"id":                  uuid.NewString(),
		"appID":               strings.TrimSpace(input.AppID),
		"keyID":               strings.TrimSpace(input.KeyID),
		"name":                name,
		"ownerAddress":        ownerAddress,
		"feeRecipientAddress": feeRecipientAddress,
		"feeBps":              input.FeeBps,
		"chainID":             input.ChainID,
		"description":         strings.TrimSpace(input.Description),
		"sourceName":          sourceName,
		"sourceCode":          sourceCode,
		"now":                 now,
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (a:EngineApp {id: $appID})
CREATE (d:MarketplaceDeployment {
  id: $id, appId: $appID, keyId: $keyID, name: $name, ownerAddress: $ownerAddress,
  feeRecipientAddress: $feeRecipientAddress, feeBps: $feeBps, chainId: $chainID,
  description: $description, status: "queued", contractAddress: "", transactionHash: "",
  error: "", sourceName: $sourceName, sourceCode: $sourceCode, abi: "",
  createdAt: $now, updatedAt: $now, queuedAt: $now
})
MERGE (a)-[:HAS_MARKETPLACE_DEPLOYMENT]->(d)
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name,
  d.ownerAddress AS ownerAddress, d.feeRecipientAddress AS feeRecipientAddress,
  d.feeBps AS feeBps, d.chainId AS chainId, coalesce(d.description, "") AS description,
  coalesce(d.status, "queued") AS status, coalesce(d.contractAddress, "") AS contractAddress,
  coalesce(d.transactionHash, "") AS transactionHash, coalesce(d.error, "") AS error,
  coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt,
  coalesce(d.queuedAt, "") AS queuedAt
`, params)
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return marketplaceDeploymentFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("engine app not found")
	})
	if err != nil {
		return MarketplaceDeployment{}, err
	}
	return result.(MarketplaceDeployment), nil
}

func (s *MemgraphStore) ListMarketplaceDeployments(ctx context.Context, appID string, limit int64) ([]MarketplaceDeployment, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp {id: $appID})-[:HAS_MARKETPLACE_DEPLOYMENT]->(d:MarketplaceDeployment)
WHERE coalesce(d.hiddenFromDashboard, false) = false
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name,
  d.ownerAddress AS ownerAddress, d.feeRecipientAddress AS feeRecipientAddress,
  d.feeBps AS feeBps, d.chainId AS chainId, coalesce(d.description, "") AS description,
  coalesce(d.status, "queued") AS status, coalesce(d.contractAddress, "") AS contractAddress,
  coalesce(d.transactionHash, "") AS transactionHash, coalesce(d.error, "") AS error,
  coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt,
  coalesce(d.queuedAt, "") AS queuedAt
ORDER BY d.createdAt DESC
LIMIT $limit
`, map[string]any{"appID": strings.TrimSpace(appID), "limit": limit})
		if err != nil {
			return nil, err
		}
		deployments := []MarketplaceDeployment{}
		for rows.Next(ctx) {
			deployments = append(deployments, marketplaceDeploymentFromRecord(rows.Record()))
		}
		return deployments, rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result.([]MarketplaceDeployment), nil
}

func (s *MemgraphStore) RemoveMarketplaceDeploymentFromDashboard(ctx context.Context, accountID string, deploymentID string) (MarketplaceDeployment, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:GMRAccount {id: $accountID})-[:OWNS_ENGINE_APP]->(:EngineApp)-[:HAS_MARKETPLACE_DEPLOYMENT]->(d:MarketplaceDeployment {id: $deploymentID})
WHERE coalesce(d.hiddenFromDashboard, false) = false AND NOT coalesce(d.status, "queued") IN ["deploying", "submitted"]
SET d.hiddenFromDashboard = true, d.removedAt = $now, d.updatedAt = $now
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name,
  d.ownerAddress AS ownerAddress, d.feeRecipientAddress AS feeRecipientAddress,
  d.feeBps AS feeBps, d.chainId AS chainId, coalesce(d.description, "") AS description,
  coalesce(d.status, "queued") AS status, coalesce(d.contractAddress, "") AS contractAddress,
  coalesce(d.transactionHash, "") AS transactionHash, coalesce(d.error, "") AS error,
  coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt,
  coalesce(d.queuedAt, "") AS queuedAt
`, map[string]any{
			"accountID":    strings.TrimSpace(accountID),
			"deploymentID": strings.TrimSpace(deploymentID),
			"now":          time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return marketplaceDeploymentFromRecord(rows.Record()), nil
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("removable deployment not found")
	})
	if err != nil {
		return MarketplaceDeployment{}, err
	}
	return result.(MarketplaceDeployment), nil
}

func (s *MemgraphStore) ClaimNextMarketplaceDeployment(ctx context.Context) (MarketplaceDeployment, bool, error) {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	result, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		rows, err := tx.Run(ctx, `
MATCH (:EngineApp)-[:HAS_MARKETPLACE_DEPLOYMENT]->(d:MarketplaceDeployment)
WHERE coalesce(d.status, "queued") = "queued" AND coalesce(d.hiddenFromDashboard, false) = false
WITH d ORDER BY d.createdAt ASC LIMIT 1
SET d.status = "deploying", d.updatedAt = $now, d.error = ""
RETURN d.id AS id, d.appId AS appId, d.keyId AS keyId, d.name AS name,
  d.ownerAddress AS ownerAddress, d.feeRecipientAddress AS feeRecipientAddress,
  d.feeBps AS feeBps, d.chainId AS chainId, coalesce(d.description, "") AS description,
  coalesce(d.status, "queued") AS status, coalesce(d.contractAddress, "") AS contractAddress,
  coalesce(d.transactionHash, "") AS transactionHash, coalesce(d.error, "") AS error,
  coalesce(d.sourceName, "") AS sourceName, coalesce(d.sourceCode, "") AS sourceCode,
  coalesce(d.abi, "") AS abi, d.createdAt AS createdAt, d.updatedAt AS updatedAt,
  coalesce(d.queuedAt, "") AS queuedAt
`, map[string]any{"now": time.Now().UTC().Format(time.RFC3339)})
		if err != nil {
			return nil, err
		}
		if rows.Next(ctx) {
			return marketplaceDeploymentFromRecord(rows.Record()), nil
		}
		return nil, rows.Err()
	})
	if err != nil {
		return MarketplaceDeployment{}, false, err
	}
	if result == nil {
		return MarketplaceDeployment{}, false, nil
	}
	return result.(MarketplaceDeployment), true, nil
}

func (s *MemgraphStore) MarkMarketplaceDeploymentSubmitted(ctx context.Context, deploymentID string, transactionHash string) error {
	return s.updateMarketplaceDeploymentStatus(ctx, deploymentID, "submitted", transactionHash, "", "", "")
}

func (s *MemgraphStore) MarkMarketplaceDeploymentConfirmed(ctx context.Context, deploymentID string, contractAddress string, abi string) error {
	return s.updateMarketplaceDeploymentStatus(ctx, deploymentID, "confirmed", "", strings.ToLower(strings.TrimSpace(contractAddress)), "", abi)
}

func (s *MemgraphStore) MarkMarketplaceDeploymentFailed(ctx context.Context, deploymentID string, message string) error {
	return s.updateMarketplaceDeploymentStatus(ctx, deploymentID, "failed", "", "", message, "")
}

func (s *MemgraphStore) updateMarketplaceDeploymentStatus(ctx context.Context, deploymentID string, status string, transactionHash string, contractAddress string, message string, abi string) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, `
MATCH (d:MarketplaceDeployment {id: $deploymentID})
SET d.status = $status, d.updatedAt = $now
SET d.transactionHash = CASE WHEN $transactionHash <> "" THEN $transactionHash ELSE coalesce(d.transactionHash, "") END
SET d.contractAddress = CASE WHEN $contractAddress <> "" THEN $contractAddress ELSE coalesce(d.contractAddress, "") END
SET d.error = CASE WHEN $error <> "" THEN $error ELSE "" END
SET d.abi = CASE WHEN $abi <> "" THEN $abi ELSE coalesce(d.abi, "") END
RETURN d.id AS id
`, map[string]any{
			"deploymentID":    strings.TrimSpace(deploymentID),
			"status":          strings.TrimSpace(status),
			"transactionHash": strings.TrimSpace(transactionHash),
			"contractAddress": strings.TrimSpace(contractAddress),
			"error":           strings.TrimSpace(message),
			"abi":             strings.TrimSpace(abi),
			"now":             time.Now().UTC().Format(time.RFC3339),
		})
		return nil, err
	})
	return err
}

func normalizeTradingFeeBps(value int64) int64 {
	if value < 0 {
		return 0
	}
	if value > 1000 {
		return 1000
	}
	return value
}

func defaultTradingFeeBps(value int64) int64 {
	if value <= 0 {
		return 50
	}
	return normalizeTradingFeeBps(value)
}

func normalizeTransactionKind(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "contract_write", "erc20_transfer", "native_transfer":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "contract_write"
	}
}

func safeContractName(name string, symbol string) string {
	raw := name
	if strings.TrimSpace(raw) == "" {
		raw = symbol
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9')
	})
	result := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		result += strings.ToUpper(part[:1]) + part[1:]
	}
	if result == "" {
		result = "GMRToken"
	}
	if result[0] >= '0' && result[0] <= '9' {
		result = "Token" + result
	}
	return result
}

func erc20Source(contractName string, name string, symbol string, decimals int64) string {
	return contracts.ERC20Source(contractName, name, symbol, decimals)
}

func erc1155EditionSource(contractName string, name string, symbol string) string {
	return contracts.ERC1155EditionSource(contractName, name, symbol)
}

func budolEscrowSource(contractName string) string {
	return contracts.BudolEscrowSource(contractName)
}

func privateClaimRegistrySource(contractName string) string {
	return contracts.PrivateClaimRegistrySource(contractName)
}

func shieldedPayoutPoolSource(contractName string) string {
	return contracts.ShieldedPayoutPoolSource(contractName)
}

func shieldedWithdrawalVerifierSource(contractName string) string {
	return contracts.ShieldedWithdrawalVerifierSource(contractName)
}

const erc1155EditionABI = `[
  {"type":"constructor","inputs":[{"name":"initialOwner","type":"address"},{"name":"initialBaseURI","type":"string"},{"name":"initialContractURI","type":"string"},{"name":"initialRecipient","type":"address"},{"name":"initialTokenId","type":"uint256"},{"name":"initialSupply","type":"uint256"}],"stateMutability":"nonpayable"},
  {"type":"event","name":"ApprovalForAll","inputs":[{"name":"account","type":"address","indexed":true},{"name":"operator","type":"address","indexed":true},{"name":"approved","type":"bool","indexed":false}],"anonymous":false},
  {"type":"event","name":"TransferBatch","inputs":[{"name":"operator","type":"address","indexed":true},{"name":"from","type":"address","indexed":true},{"name":"to","type":"address","indexed":true},{"name":"ids","type":"uint256[]","indexed":false},{"name":"values","type":"uint256[]","indexed":false}],"anonymous":false},
  {"type":"event","name":"TransferSingle","inputs":[{"name":"operator","type":"address","indexed":true},{"name":"from","type":"address","indexed":true},{"name":"to","type":"address","indexed":true},{"name":"id","type":"uint256","indexed":false},{"name":"value","type":"uint256","indexed":false}],"anonymous":false},
  {"type":"event","name":"URI","inputs":[{"name":"value","type":"string","indexed":false},{"name":"id","type":"uint256","indexed":true}],"anonymous":false},
  {"type":"function","name":"balanceOf","inputs":[{"name":"account","type":"address"},{"name":"id","type":"uint256"}],"outputs":[{"type":"uint256"}],"stateMutability":"view"},
  {"type":"function","name":"balanceOfBatch","inputs":[{"name":"accounts","type":"address[]"},{"name":"ids","type":"uint256[]"}],"outputs":[{"type":"uint256[]"}],"stateMutability":"view"},
  {"type":"function","name":"burn","inputs":[{"name":"account","type":"address"},{"name":"id","type":"uint256"},{"name":"value","type":"uint256"}],"outputs":[],"stateMutability":"nonpayable"},
  {"type":"function","name":"burnBatch","inputs":[{"name":"account","type":"address"},{"name":"ids","type":"uint256[]"},{"name":"values","type":"uint256[]"}],"outputs":[],"stateMutability":"nonpayable"},
  {"type":"function","name":"contractURI","inputs":[],"outputs":[{"type":"string"}],"stateMutability":"view"},
  {"type":"function","name":"exists","inputs":[{"name":"id","type":"uint256"}],"outputs":[{"type":"bool"}],"stateMutability":"view"},
  {"type":"function","name":"isApprovedForAll","inputs":[{"name":"account","type":"address"},{"name":"operator","type":"address"}],"outputs":[{"type":"bool"}],"stateMutability":"view"},
  {"type":"function","name":"mint","inputs":[{"name":"to","type":"address"},{"name":"id","type":"uint256"},{"name":"amount","type":"uint256"},{"name":"data","type":"bytes"}],"outputs":[],"stateMutability":"nonpayable"},
  {"type":"function","name":"mintBatch","inputs":[{"name":"to","type":"address"},{"name":"ids","type":"uint256[]"},{"name":"amounts","type":"uint256[]"},{"name":"data","type":"bytes"}],"outputs":[],"stateMutability":"nonpayable"},
  {"type":"function","name":"name","inputs":[],"outputs":[{"type":"string"}],"stateMutability":"view"},
  {"type":"function","name":"owner","inputs":[],"outputs":[{"type":"address"}],"stateMutability":"view"},
  {"type":"function","name":"renounceOwnership","inputs":[],"outputs":[],"stateMutability":"nonpayable"},
  {"type":"function","name":"safeBatchTransferFrom","inputs":[{"name":"from","type":"address"},{"name":"to","type":"address"},{"name":"ids","type":"uint256[]"},{"name":"values","type":"uint256[]"},{"name":"data","type":"bytes"}],"outputs":[],"stateMutability":"nonpayable"},
  {"type":"function","name":"safeTransferFrom","inputs":[{"name":"from","type":"address"},{"name":"to","type":"address"},{"name":"id","type":"uint256"},{"name":"value","type":"uint256"},{"name":"data","type":"bytes"}],"outputs":[],"stateMutability":"nonpayable"},
  {"type":"function","name":"setApprovalForAll","inputs":[{"name":"operator","type":"address"},{"name":"approved","type":"bool"}],"outputs":[],"stateMutability":"nonpayable"},
  {"type":"function","name":"setContractURI","inputs":[{"name":"newContractURI","type":"string"}],"outputs":[],"stateMutability":"nonpayable"},
  {"type":"function","name":"setURI","inputs":[{"name":"newURI","type":"string"}],"outputs":[],"stateMutability":"nonpayable"},
  {"type":"function","name":"supportsInterface","inputs":[{"name":"interfaceId","type":"bytes4"}],"outputs":[{"type":"bool"}],"stateMutability":"view"},
  {"type":"function","name":"symbol","inputs":[],"outputs":[{"type":"string"}],"stateMutability":"view"},
  {"type":"function","name":"totalSupply","inputs":[{"name":"id","type":"uint256"}],"outputs":[{"type":"uint256"}],"stateMutability":"view"},
  {"type":"function","name":"transferOwnership","inputs":[{"name":"newOwner","type":"address"}],"outputs":[],"stateMutability":"nonpayable"},
  {"type":"function","name":"uri","inputs":[{"name":"id","type":"uint256"}],"outputs":[{"type":"string"}],"stateMutability":"view"}
]`

func normalizeStringSlice(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		result = append(result, trimmed)
	}
	return result
}

func normalizeInt64Slice(values []int64) []int64 {
	seen := map[int64]bool{}
	result := []int64{}
	for _, value := range values {
		if value <= 0 || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
