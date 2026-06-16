# GMR Engine

Standalone transaction engine for Budol and other projects.

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
