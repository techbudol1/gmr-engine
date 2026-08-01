# BudolPH ZK Production Ceremony Guide

This document describes what must happen before BudolPH ZK proofs are safe for real-value or mainnet production usage.

## Current Status

BudolPH has two Groth16 circuits:

- `private_winning_claim`: proves a user owns a winning committed market note.
- `shielded_withdrawal`: proves a user owns a credited shielded payout note and can withdraw it.

The end-to-end testnet path is implemented on Horizen Testnet. Current deployment addresses and runtime configuration are intentionally supplied by the API and Engine rather than duplicated in this procedure.

The checked-in `.zkey` files are still local development artifacts. They are intentionally marked:

```json
{
  "productionReady": false
}
```

BudolPH refuses production proving-artifact usage unless ceremony metadata is present and marked production-ready.

## Why The Ceremony Matters

Groth16 proving keys include toxic waste from setup. If one party controls the full setup randomness, that party may be able to forge proofs. A production ceremony reduces that risk by letting multiple contributors add entropy. If at least one contributor is honest and destroys their randomness, the final key is safe under the ceremony assumptions.

You cannot fix this by editing code or changing `productionReady` to `true`. The ceremony must actually happen, and the final artifacts must be verified.

## Production Readiness Checklist

Before real funds or mainnet:

- Freeze the circuit source code for both circuits.
- Record the git commit hash used for the ceremony.
- Use a public Powers of Tau file large enough for each circuit constraint count.
- Run a multi-party phase-2 ceremony for each circuit.
- Apply a final random beacon.
- Verify every final `.zkey` against the circuit `.r1cs` and `.ptau`.
- Export new `verification_key.json` files.
- Export and deploy the matching Solidity verifier for `shielded_withdrawal`.
- Replace browser artifacts under `public/zk/...`.
- Publish `checksums.sha256`.
- Update `ceremony.json` with production metadata.
- Store ceremony transcripts and contributor hashes under a release folder.
- Run the full BudolPH and GMR smoke tests after deployment.

## Folder Layout

Use a dedicated release folder outside the public app artifacts:

```text
ceremonies/
  2026-xx-budol-zk-mainnet/
    README.md
    git-commit.txt
    ptau-source.txt
    private-winning-claim/
      contributors.txt
      transcript.log
      final-zkey.sha256
      verification.log
    shielded-withdrawal/
      contributors.txt
      transcript.log
      final-zkey.sha256
      verification.log
```

Only the proving artifacts needed by the browser should go under:

```text
public/zk/private-claim/
public/zk/shielded-withdrawal/
```

## Ceremony Steps

Run these steps separately for each circuit.

### 1. Freeze The Circuit

Record the source commit:

```bash
git rev-parse HEAD > ceremonies/2026-xx-budol-zk-mainnet/git-commit.txt
```

Do not change the circuit after this point unless you restart the ceremony.

### 2. Compile The Circuit

Private winning claim:

```bash
circom gmr-engine/zk/private-claim/private_winning_claim.circom \
  --r1cs --wasm --sym \
  -o gmr-engine/zk/private-claim/build
```

Shielded withdrawal:

```bash
circom gmr-engine/zk/shielded-withdrawal/shielded_withdrawal.circom \
  --r1cs --wasm --sym \
  -o gmr-engine/zk/shielded-withdrawal/build
```

Check constraints:

```bash
bunx snarkjs r1cs info gmr-engine/zk/private-claim/build/private_winning_claim.r1cs
bunx snarkjs r1cs info gmr-engine/zk/shielded-withdrawal/build/shielded_withdrawal.r1cs
```

Choose a `.ptau` whose power is large enough for the larger circuit.

### 3. Start Phase 2

Example for private winning claim:

```bash
bunx snarkjs groth16 setup \
  gmr-engine/zk/private-claim/build/private_winning_claim.r1cs \
  /path/to/pot_final.ptau \
  ceremonies/2026-xx-budol-zk-mainnet/private-winning-claim/private_winning_claim_0000.zkey
```

Example for shielded withdrawal:

```bash
bunx snarkjs groth16 setup \
  gmr-engine/zk/shielded-withdrawal/build/shielded_withdrawal.r1cs \
  /path/to/pot_final.ptau \
  ceremonies/2026-xx-budol-zk-mainnet/shielded-withdrawal/shielded_withdrawal_0000.zkey
```

### 4. Collect Multiple Contributions

Each contributor receives the previous `.zkey`, contributes, and returns the next `.zkey`.

```bash
bunx snarkjs zkey contribute \
  previous.zkey \
  next.zkey \
  --name="Contributor name" \
  -v
```

Record contributor names, dates, output hashes, and logs.

At minimum, use contributors controlled by separate people or machines. For stronger assurance, use public external contributors and publish the transcript.

### 5. Apply A Final Beacon

After all contributions:

```bash
bunx snarkjs zkey beacon \
  last_contribution.zkey \
  final.zkey \
  <public-random-hex> \
  10 \
  -n="Public random beacon"
```

Use public randomness that can be independently checked later, for example a future block hash or a public randomness beacon.

### 6. Verify The Final Key

```bash
bunx snarkjs zkey verify \
  circuit.r1cs \
  /path/to/pot_final.ptau \
  final.zkey
```

Save the verification output in the ceremony folder.

### 7. Export Public Artifacts

```bash
bunx snarkjs zkey export verificationkey final.zkey verification_key.json
```

For `shielded_withdrawal`, also export the Solidity verifier and add it to GMR Engine:

```bash
bunx snarkjs zkey export solidityverifier \
  final.zkey \
  gmr-engine/internal/contracts/shielded_withdrawal_verifier.sol
```

Deploy the new verifier adapter and wire the pool with `setVerifier`.

### 8. Publish Browser Artifacts

Copy the final files into the app:

```text
public/zk/private-claim/private_winning_claim.wasm
public/zk/private-claim/private_winning_claim_final.zkey
public/zk/private-claim/verification_key.json

public/zk/shielded-withdrawal/shielded_withdrawal.wasm
public/zk/shielded-withdrawal/shielded_withdrawal_final.zkey
public/zk/shielded-withdrawal/verification_key.json
```

Generate checksums:

```bash
(
  cd public/zk/private-claim
  sha256sum private_winning_claim.wasm private_winning_claim_final.zkey verification_key.json > checksums.sha256
)

(
  cd public/zk/shielded-withdrawal
  sha256sum shielded_withdrawal.wasm shielded_withdrawal_final.zkey verification_key.json > checksums.sha256
)
```

### 9. Mark Ceremony Metadata Production-Ready

Update each `ceremony.json` only after verification passes.

Example:

```json
{
  "circuit": "shielded_withdrawal",
  "ceremony": "budol-shielded-withdrawal-mainnet-2026-xx",
  "finalZKeyHash": "sha256-of-final-zkey",
  "productionReady": true,
  "ptau": "pot_final.ptau source and hash",
  "gitCommit": "commit hash",
  "contributors": 5,
  "beacon": "public random beacon source",
  "verifiedAt": "2026-xx-xxT00:00:00Z"
}
```

Do not set `productionReady: true` for local single-contributor output.

## Post-Ceremony Validation

Run:

```bash
go test ./...
bun run build
bun build ./gmr-dashboard/index.html --outdir=/tmp/gmr-dashboard-build --minify
```

Then smoke-test on the target chain:

1. Place a trade.
2. Resolve the market.
3. Generate a private claim proof in the browser.
4. Submit the proof to ZKVerify through GMR Engine.
5. Claim into the shielded payout pool.
6. Generate a shielded withdrawal proof.
7. Withdraw with `SHIELDED_WITHDRAWAL_MODE=verified`.
8. Confirm the pool marked the note and nullifier as used.

Production environment should use:

```text
SHIELDED_PAYOUT_ENABLED=true
SHIELDED_PAYOUT_REQUIRED=true
SHIELDED_WITHDRAWAL_MODE=verified
```

For production shielded payout mode, do not use `SHIELDED_WITHDRAWAL_MODE=zkverify`.
