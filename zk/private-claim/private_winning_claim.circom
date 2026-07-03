pragma circom 2.1.6;

include "circomlib/circuits/poseidon.circom";

template MerkleRoot(levels) {
    signal input leaf;
    signal input pathElements[levels];
    signal input pathIndices[levels];
    signal output root;

    signal current[levels + 1];
    signal left[levels];
    signal right[levels];
    component hashers[levels];

    current[0] <== leaf;

    for (var i = 0; i < levels; i++) {
        pathIndices[i] * (pathIndices[i] - 1) === 0;

        left[i] <== current[i] + pathIndices[i] * (pathElements[i] - current[i]);
        right[i] <== pathElements[i] + pathIndices[i] * (current[i] - pathElements[i]);

        hashers[i] = Poseidon(2);
        hashers[i].inputs[0] <== left[i];
        hashers[i].inputs[1] <== right[i];
        current[i + 1] <== hashers[i].out;
    }

    root <== current[levels];
}

template PrivateWinningClaim(levels) {
    // Public inputs. These are safe to reveal to ZKVerify and later to a verifier.
    signal input root;
    signal input resolvedOutcome;
    signal input nullifierHash;

    // Private witness. These should be created and stored client-side.
    signal input marketId;
    signal input outcome;
    signal input amount;
    signal input amountInverse;
    signal input userSalt;
    signal input secret;
    signal input pathElements[levels];
    signal input pathIndices[levels];

    // BudolPH markets currently use two outcomes: 1 = A/Yes, 2 = B/No.
    (outcome - 1) * (outcome - 2) === 0;
    (resolvedOutcome - 1) * (resolvedOutcome - 2) === 0;
    outcome === resolvedOutcome;

    // Prove amount is nonzero without making amount public.
    amount * amountInverse === 1;

    component noteHash = Poseidon(5);
    noteHash.inputs[0] <== marketId;
    noteHash.inputs[1] <== outcome;
    noteHash.inputs[2] <== amount;
    noteHash.inputs[3] <== userSalt;
    noteHash.inputs[4] <== secret;

    component nullifier = Poseidon(2);
    nullifier.inputs[0] <== secret;
    nullifier.inputs[1] <== marketId;
    nullifier.out === nullifierHash;

    component tree = MerkleRoot(levels);
    tree.leaf <== noteHash.out;

    for (var i = 0; i < levels; i++) {
        tree.pathElements[i] <== pathElements[i];
        tree.pathIndices[i] <== pathIndices[i];
    }

    tree.root === root;
}

component main { public [root, resolvedOutcome, nullifierHash] } = PrivateWinningClaim(20);
