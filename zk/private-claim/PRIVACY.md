# What This Private Claim Flow Hides

This is the privacy model for the current Budol private winning-claim circuit.

## Private From Public Observers

The proof does not reveal these values to ZKVerify, Arbitrum, block explorers, or normal users:

- which private bet note was used
- the bettor's original note secret
- the bettor's note salt
- the committed bet amount inside the proof
- the selected side inside the note beyond proving it equals the public winning outcome
- the Merkle path showing where the note sits in the market tree

The public only sees that some valid note in the accepted Merkle root won and produced a nullifier.

## Public By Design

These values are public:

- `root`: the market's accepted bet-note Merkle root
- `resolvedOutcome`: the winning side
- `nullifierHash`: the one-time claim ID used to block double claims
- the ZKVerify submission transaction and submitting ZKVerify account
- any later ERC20 payout transfer on Arbitrum Sepolia, including recipient and amount, if the payout is sent directly on-chain

The nullifier is not the wallet address and does not reveal the note secret, but it is linkable if reused. Reuse must be rejected.

## Private From Users And Backend

The frontend creates the private claim note and keeps the secret locally. Budol API receives only the public commitment leaf at trade time and the public nullifier at claim time.

GMR Engine still includes a helper script for development, testing, and root/circuit-input tooling, but the user-facing Budol flow no longer sends full note values to the backend.

## What Is Not Finished Yet

This implementation adds the circuit source, development proving artifacts, browser proof generation, Budol proof submission, ZKVerify submission through GMR Engine, and claim-time payout checks. Budol also has a separate shielded payout pool path for fixed-denomination withdrawals.

Still needed:

- replace the development `.zkey` files with production ceremony outputs
- publish and retain ceremony transcripts, final key hashes, and verification logs
- a dedicated Merkle tree service that appends committed bet notes per market
- mandatory private-claim registry configuration if claims must have an on-chain nullifier record
- optional relayer withdrawal flow if we want to hide the wallet that submits the withdrawal transaction

## Practical Meaning For Budol

With this circuit, Budol can evolve from public bet records into private claim records:

1. When a user places a bet, the app creates a private note.
2. The public market tree stores only the note commitment.
3. When the market resolves, the user proves their note is in the tree and matches the winning outcome.
4. The claim reveals only a nullifier, not the private note.
5. Budol pays the winner after confirming the proof and that the nullifier was not used before.

If payouts are still normal ERC20 transfers, the final payment remains visible on-chain. The shielded payout pool improves this by crediting a fixed-denomination note and letting the user withdraw later, but the final withdrawal transfer is still public. See `docs/budol-zkp-architecture.md` and `docs/zk-production-ceremony.md` for the current production checklist.
