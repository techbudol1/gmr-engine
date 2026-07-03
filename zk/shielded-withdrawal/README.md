# Shielded Withdrawal Circuit

This circuit proves that a user owns a credited shielded payout note and can withdraw it from the BudolPH shielded payout pool.

It is separate from the private winning-claim circuit:

- `private_winning_claim` proves the user has a winning market note.
- `shielded_withdrawal` proves the user can spend a credited payout-pool note.

## Public Inputs

The circuit public signals are:

```text
noteCommitment
nullifierHash
recipient
chainId
tokenAddress
poolAddress
denomination
```

The pool verifier adapter and BudolPH API both require these public values to match the configured pool context.

## Private Witness

The private witness is:

```text
secret
blinding
```

The commitment is:

```text
noteCommitment = Poseidon(secret, blinding, chainId, tokenAddress, poolAddress, denomination)
```

The withdrawal nullifier is:

```text
nullifierHash = Poseidon(secret, blinding, recipient)
```

This lets the pool block double withdrawals without learning the note secret or blinding value.

## Runtime Flow

1. BudolPH credits a fixed-denomination note commitment into the shielded payout pool.
2. The user stores the full note locally in the browser.
3. The user later chooses a recipient wallet.
4. The browser generates a Groth16 proof and Solidity calldata.
5. BudolPH checks the public signals and asks GMR Engine to call the pool.
6. In verified mode, the pool calls the configured verifier adapter before releasing tokens.
7. The pool marks the note and nullifier as used.

Production should use:

```text
SHIELDED_PAYOUT_ENABLED=true
SHIELDED_PAYOUT_REQUIRED=true
SHIELDED_WITHDRAWAL_MODE=verified
```

The development `zkverify` withdrawal mode is retained for audit continuity and test flows, but it should not be used for production shielded payouts.

## Artifacts

Browser artifacts are published to:

```text
public/zk/shielded-withdrawal/
```

The expected files are:

```text
shielded_withdrawal.wasm
shielded_withdrawal_final.zkey
verification_key.json
checksums.sha256
ceremony.json
```

Current checked-in artifacts are local development artifacts and are marked `productionReady: false`.

## Production Ceremony

Before real-value usage, replace the development `.zkey` with a final key from a proper multi-party ceremony, export a matching `verification_key.json`, regenerate the Solidity verifier, deploy the new verifier adapter, and update `ceremony.json` with `productionReady: true`.

Use `docs/zk-production-ceremony.md` as the production checklist.
