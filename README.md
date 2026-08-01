# GMR Engine

GMR Engine is a self-hosted transaction and contract-operations service for BudolPH and compatible EVM projects. It keeps operational queues, enforces project and contract controls, and delegates key custody and signing to GMR Vault.

## Core capabilities

- Contract deployment and verified contract registration.
- Project wallet administration, transaction queues, locks, and lifecycle tracking.
- Controlled ERC-20 and generic contract writes through approved project configuration.
- Horizen Testnet support for BudolPH’s BUDOL token, privacy contracts, relayed shielded payouts, and ERC-4337 account infrastructure.
- Self-hosted ERC-4337 bundler and restrictive paymaster support for sponsored smart-account BUDOL escrow transfers.

## Architecture

```text
BudolPH API / GMR Dashboard
            │
            ▼
        GMR Engine ── GMR Vault ── Horizen Testnet
            │
            └── dedicated Memgraph store
```

## Local development

Requirements: Go, Bun, Docker, Memgraph, and a running GMR Vault service for any signing operation.

```bash
docker compose up -d gmr-engine-memgraph
go run ./cmd/api
```

Local services:

- API: `http://localhost:8090`
- Memgraph: `bolt://localhost:7689`
- Memgraph Lab: `http://localhost:7445`

## Verification

```bash
go test ./...
bun run aa:check
```

## Horizen account abstraction

See [docs/horizen-account-abstraction.md](./docs/horizen-account-abstraction.md) for ERC-4337 deployment, bundler, paymaster, and smoke-test guidance. See [docs/zk-production-ceremony.md](./docs/zk-production-ceremony.md) for the mandatory ceremony requirements before real-value privacy use.

## Shielded trading

See [docs/shielded-trading.md](./docs/shielded-trading.md) for the fixed-denomination vault, Poseidon/Merkle circuit, relayed private orders, batch settlement, deployment, and security boundaries.

## Security model

GMR Engine stores only Vault wallet references, never project private keys. Administrative and project API keys must remain server-side. Run Engine and Vault on private network paths, restrict client origins and scopes, and use monitored wallet/contract allowlists for every production deployment.
