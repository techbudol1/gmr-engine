# Shielded trading on Horizen Testnet

BudolPH shielded trading uses fixed-denomination deposits, Poseidon commitments,
an incremental Merkle tree, nullifiers, Groth16 proofs, a relayer, and delayed
batch settlement. No new cryptographic primitive is introduced.

## Public and private data

The vault publicly exposes deposits, commitments, roots, nullifiers, accepted
order commitments, and aggregate batch collateral. The proof conceals which
deposit was spent and binds a hidden market, side, amount, and salt to the order
commitment. The API intentionally withholds individual private orders from the
public activity and holder endpoints. Odds are published only after the batch is
settled.

The testnet coordinator still receives the market, side, and amount so that it
can execute BudolPH's market-making logic. Protecting orders from the operator
requires confidential execution (TEE, FHE, or MPC) and is outside this version.

## Components

- `zk/shielded-trade/shielded_trade.circom`: deposit membership, nullifier,
  order commitment, chain/vault/token binding, and exact fee arithmetic.
- `contracts/privacy/BudolShieldedTradeVault.sol`: fixed denomination vault,
  Merkle roots, proof verification, nullifier enforcement, and permissionless
  post-delay settlement to an immutable escrow.
- `scripts/privacy/deploy-shielded-trade.ts`: Horizen Testnet deployment of a
  Poseidon hasher, generated Groth16 verifier, and vault.
- `scripts/test-shielded-trade.ts`: generates and verifies a complete proof.

One vault is required for every supported trade amount and fee schedule. With a
0.5% fee, a 10 BUDOL order uses a 10.05 BUDOL vault denomination.
Deploy Poseidon and the Groth16 verifier with the first vault, then pass the
returned `poseidonAddress` and `verifierAddress` when deploying additional
denomination vaults. This keeps one audited verifier/hasher pair and avoids two
unnecessary deployments per amount.

## Build and verify

```bash
PTAU_FILE=/secure/path/powersOfTau28_hez_final_16.ptau bun run privacy:shielded-trade:build
bun run privacy:shielded-trade:test
```

The repository artifacts are for testnet development. Before real-value use,
run a documented multi-party ceremony (or migrate to a universal setup), pin
artifact checksums, deploy a new verifier and vaults, and obtain an independent
audit of the circuit, contracts, relayer, API, and note-recovery flow.
