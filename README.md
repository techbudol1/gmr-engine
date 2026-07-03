# GMR Engine

Standalone transaction engine for BudolPH and other projects.

## Local Services

- API: `http://localhost:8090`
- Dedicated Memgraph: `bolt://localhost:7689`
- Memgraph Lab: `http://localhost:7445`

Start the engine graph:

```bash
docker compose up -d gmr-engine-memgraph
```

Start the API:

```bash
cd gmr-engine
go run ./cmd/api
```

## Auth Model

Admin routes use `X-GMR-Engine-Admin-Key` or `Authorization: Bearer <admin key>`.

Client routes use generated engine API keys through `X-GMR-Engine-Key` or `Authorization: Bearer <engine key>`.

Legacy `X-Budol-Engine-*` headers are still accepted during the rename transition.

API keys are stored as SHA-256 hashes. The plaintext key is only returned once when created or rotated.

Initial scopes:

- `transactions:write`
- `transactions:read`
- `wallets:read`
- `wallets:write`
- `contracts:read`
- `contracts:write`
- `admin`

## Durable Records

The engine persists these records in its own Memgraph instance:

- `EngineApp`
- `EngineAPIKey`
- `EngineAPIUsage`
- `EngineTransaction`
- `EngineWalletLock`

Transactions start with `queued` status. Workers will later claim queued transactions, acquire the wallet lock, submit through the Alchemy RPC path, and update the transaction status through `submitted`, `confirmed`, or `failed`.

## GMR Vault Integration

GMR Engine creates project wallets through the standalone `gmr-vault` service. Legacy local encrypted project wallets have been removed.

Vault-backed wallet creation:

```bash
GMR_ENGINE_VAULT_ENABLED=true
GMR_ENGINE_VAULT_URL=http://localhost:8091
GMR_ENGINE_VAULT_INTERNAL_API_KEY=replace-with-same-value-as-gmr-vault-internal-api-key
```

When Vault is enabled, Engine stores only a reference like `gmr-vault:v1:<wallet-id>` in its project wallet secret field. The actual encrypted private key lives in GMR Vault's own Memgraph instance.

Vault-backed project wallets are used by:

- contract deployment worker
- queued ERC20 writes
- queued generic contract writes
- dashboard ERC20 console actions
- gas-free permit transfer relays

Engine passes a Vault wallet reference to the broadcaster script; the script asks GMR Vault to sign the transaction and broadcasts only the signed raw transaction. Non-Vault `ProjectWallet` records are rejected by new writes and should be deleted from the Engine graph.

## Core Endpoints

- `GET /healthz`
- `POST /admin/apps`
- `GET /admin/apps`
- `GET /admin/apps/:id`
- `POST /admin/apps/:id/api-keys`
- `POST /admin/api-keys/:id/revoke`
- `POST /admin/api-keys/:id/rotate`
- `GET /v1/auth/me`
- `POST /v1/transactions`
- `GET /v1/transactions/:id`
- `POST /v1/wallet-locks`
- `DELETE /v1/wallet-locks`
