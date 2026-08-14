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

Transactions start with `queued` status. Workers later claim queued transactions, acquire the wallet lock, submit through the configured chain RPC path, and update the transaction status through `submitted`, `confirmed`, or `failed`.

## Chain RPC Routing

Horizen testnet keeps its direct RPC connection:

```bash
GMR_ENGINE_RPC_URL=https://horizen-testnet.rpc.caldera.xyz/http
```

Configure additional EVM networks as a JSON object keyed by chain ID. Each value can be an Alchemy endpoint for any EVM chain enabled in your Alchemy app:

```bash
GMR_ENGINE_CHAIN_RPC_URLS='{"421614":"https://arb-sepolia.g.alchemy.com/v2/your-alchemy-api-key"}'
```

Arbitrum Sepolia is chain ID `421614`. The legacy `GMR_ENGINE_ALCHEMY_RPC_URL` (or `ARBITRUM_SEPOLIA_RPC_URL`) variable remains supported and maps to Arbitrum Sepolia by default; set `GMR_ENGINE_ALCHEMY_CHAIN_ID` when using that compatibility variable for a different chain.

RPC routing and project authorization are separate. Add every usable chain ID to the project's `allowedChains` and configure its RPC endpoint. Horizen testnet chain ID is `2651420` and always routes through `GMR_ENGINE_RPC_URL`. Requests for an allowed but unconfigured chain fail explicitly rather than broadcasting to another network.

### Solana through Alchemy

Configure Solana independently by cluster. There is no numeric EVM chain ID fallback:

```bash
GMR_ENGINE_SOLANA_RPC_URLS='{"devnet":"https://solana-devnet.g.alchemy.com/v2/your-alchemy-api-key","mainnet":"https://solana-mainnet.g.alchemy.com/v2/your-alchemy-api-key"}'
```

`GMR_ENGINE_ALCHEMY_API_KEY` may be used instead. If neither Solana setting is present, Engine derives the key from the first configured `*.g.alchemy.com/v2/...` EVM URL and constructs the Mainnet and Devnet endpoints. The Alchemy app must have those Solana networks enabled.

Projects may restrict access with `allowedSolanaNetworks` (`mainnet` or `devnet`, the Solana networks currently supported by Alchemy). An empty list allows every Solana network configured on the engine, matching the existing empty `allowedChains` behavior.

The Solana API supports SOL and SPL balances, Token-2022 balances, signature history, account/program reads, latest blockhash lookup, transaction simulation, and submission of fully signed transactions. It does not reuse Solidity deployment or ABI endpoints: Solana programs are sBPF binaries and calls are encoded instructions. Managed Solana signing requires an Ed25519-capable GMR Vault and is intentionally not emulated with the existing EVM secp256k1 wallet records.

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
- `GET /v1/native/balance?chainId=:chainId&walletAddress=:walletAddress`
- `GET /v1/erc20/balance?chainId=:chainId&walletAddress=:walletAddress&contractAddress=:contractAddress`
- `GET /v1/solana/native/balance?network=:network&walletAddress=:walletAddress`
- `GET /v1/solana/spl/balance?network=:network&walletAddress=:walletAddress&mintAddress=:mintAddress`
- `GET /v1/solana/spl/balances?network=:network&walletAddress=:walletAddress`
- `GET /v1/solana/wallets/transactions?network=:network&walletAddress=:walletAddress`
- `GET /v1/solana/accounts/:address?network=:network`
- `GET /v1/solana/blockhash/latest?network=:network`
- `POST /v1/solana/transactions/simulate`
- `POST /v1/solana/transactions/send`
- `POST /v1/wallet-locks`
- `DELETE /v1/wallet-locks`
