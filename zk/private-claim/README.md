# Private Winning Claim Circuit

This circuit is the first concrete Budol/GMR privacy primitive for prediction-market payouts.

It proves:

- the user owns a committed bet note inside a market Merkle root
- the committed bet outcome matches the resolved winning outcome
- the committed amount is nonzero
- the claim publishes a deterministic nullifier so the same bet note cannot be claimed twice

It does not yet execute payouts by itself. The payout integration should accept a verified ZKVerify submission, check that the nullifier has not been used, mark it claimed, then trigger the escrow payout flow.

## Public Inputs

- `root`: Merkle root for the accepted bet-note set.
- `resolvedOutcome`: winning outcome. Budol currently uses `1` for outcome A/Yes and `2` for outcome B/No.
- `nullifierHash`: one-time claim ID derived from the private note secret and market ID.

## Private Witness

- `marketId`
- `outcome`
- `amount`
- `amountInverse`
- `userSalt`
- `secret`
- `pathElements`
- `pathIndices`

The note leaf is:

```text
Poseidon(marketId, outcome, amount, userSalt, secret)
```

The claim nullifier is:

```text
Poseidon(secret, marketId)
```

## Helper Script

Create a private bet note:

```bash
printf '{"action":"createNote","marketId":"20280601","outcome":"1","amount":"100000000000000000000"}' \
  | bun gmr-engine/scripts/private-claim-note.ts
```

Build a circuit input from a note:

```bash
printf '{"action":"buildInput","note":{"marketId":"20280601","outcome":"1","amount":"100000000000000000000","userSalt":"665460784797217855476928382749091762640283159974376854772936189086346745967","secret":"195970156666852433864105879969003050947487557243378444829830534749349387199"},"resolvedOutcome":"1"}' \
  | bun gmr-engine/scripts/private-claim-note.ts
```

## Compile And Prove

For repeatable artifact generation, use the repo script:

```bash
bun run zk:private-claim:artifacts /path/to/pot20_final.ptau "Contributor name"
```

The script compiles the circuit, runs Groth16 setup, adds a contribution, verifies the final zkey, exports `verification_key.json`, publishes browser artifacts under `public/zk/private-claim/`, and writes `checksums.sha256`.

For production, run a real multi-party ceremony before publishing the final zkey. The script is useful for local development and publishing files after a ceremony, but it is not a substitute for ceremony governance. Use `docs/zk-production-ceremony.md` as the production checklist.

Install Circom and use the local `snarkjs` dependency:

```bash
circom gmr-engine/zk/private-claim/private_winning_claim.circom \
  --r1cs --wasm --sym \
  -o gmr-engine/zk/private-claim/build

bunx snarkjs groth16 setup \
  gmr-engine/zk/private-claim/build/private_winning_claim.r1cs \
  pot20_final.ptau \
  gmr-engine/zk/private-claim/build/private_winning_claim_0000.zkey

bunx snarkjs zkey contribute \
  gmr-engine/zk/private-claim/build/private_winning_claim_0000.zkey \
  gmr-engine/zk/private-claim/build/private_winning_claim_final.zkey \
  --name="GMR Engine dev contribution" -v

node gmr-engine/zk/private-claim/build/private_winning_claim_js/generate_witness.js \
  gmr-engine/zk/private-claim/build/private_winning_claim_js/private_winning_claim.wasm \
  gmr-engine/zk/private-claim/input.example.json \
  gmr-engine/zk/private-claim/build/witness.wtns

bunx snarkjs groth16 prove \
  gmr-engine/zk/private-claim/build/private_winning_claim_final.zkey \
  gmr-engine/zk/private-claim/build/witness.wtns \
  gmr-engine/zk/private-claim/build/proof.json \
  gmr-engine/zk/private-claim/build/public.json

bunx snarkjs zkey export verificationkey \
  gmr-engine/zk/private-claim/build/private_winning_claim_final.zkey \
  gmr-engine/zk/private-claim/build/verification_key.json
```

Then submit `proof.json`, `public.json`, and `verification_key.json` through GMR Engine:

```bash
curl -X POST http://localhost:8090/v1/zkverify/proofs \
  -H "Content-Type: application/json" \
  -H "X-GMR-Engine-Key: $GMR_ENGINE_API_KEY" \
  -d '{
    "proofSystem": "groth16",
    "domainId": 1,
    "proof": {},
    "publicSignals": [],
    "vk": {}
  }'
```

Replace the empty proof values with the generated files.
