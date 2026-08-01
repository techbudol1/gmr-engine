# Shielded trade circuit

This Circom circuit authorizes one fixed-denomination BUDOL deposit note for a
private batched order without exposing the deposit leaf, market, side, or trade
principal.

Public signals:

1. deposit Merkle root
2. spend nullifier
3. order commitment
4. chain ID
5. BUDOL token address
6. shielded trade vault address
7. deposited denomination (principal plus fee)
8. trading fee in basis points
9. batch ID

The contract rejects unknown roots, reused nullifiers, mismatched configuration,
and closed batches. A relayer can submit the proof so the transaction sender is
not the trader.

This directory contains development proving artifacts only after running the
build script. Production use requires a new multi-party phase-2 ceremony and an
independent circuit and contract audit.
