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

template ShieldedTrade(levels) {
    // Public signals bind a proof to one vault, fee schedule, and open batch.
    // Market, side, principal and the spent deposit leaf remain private.
    signal input root;
    signal input nullifierHash;
    signal input orderCommitment;
    signal input chainId;
    signal input tokenAddress;
    signal input vaultAddress;
    signal input denomination;
    signal input feeBps;
    signal input batchId;

    // Private deposit note and order witness.
    signal input secret;
    signal input blinding;
    signal input marketId;
    signal input outcome;
    signal input tradeAmount;
    signal input tradeAmountInverse;
    signal input orderSalt;
    signal input pathElements[levels];
    signal input pathIndices[levels];

    // Binary markets use 1 = Yes and 2 = No.
    (outcome - 1) * (outcome - 2) === 0;
    tradeAmount * tradeAmountInverse === 1;

    // The deposit pays principal plus the configured trading fee exactly.
    // Values are token base units, so this relation has no rounding ambiguity.
    tradeAmount * (10000 + feeBps) === denomination * 10000;

    component deposit = Poseidon(6);
    deposit.inputs[0] <== secret;
    deposit.inputs[1] <== blinding;
    deposit.inputs[2] <== chainId;
    deposit.inputs[3] <== tokenAddress;
    deposit.inputs[4] <== vaultAddress;
    deposit.inputs[5] <== denomination;

    component tree = MerkleRoot(levels);
    tree.leaf <== deposit.out;
    for (var i = 0; i < levels; i++) {
        tree.pathElements[i] <== pathElements[i];
        tree.pathIndices[i] <== pathIndices[i];
    }
    tree.root === root;

    component nullifier = Poseidon(3);
    nullifier.inputs[0] <== secret;
    nullifier.inputs[1] <== blinding;
    nullifier.inputs[2] <== vaultAddress;
    nullifier.out === nullifierHash;

    component order = Poseidon(5);
    order.inputs[0] <== marketId;
    order.inputs[1] <== outcome;
    order.inputs[2] <== tradeAmount;
    order.inputs[3] <== orderSalt;
    order.inputs[4] <== batchId;
    order.out === orderCommitment;
}

component main { public [root, nullifierHash, orderCommitment, chainId, tokenAddress, vaultAddress, denomination, feeBps, batchId] } = ShieldedTrade(20);
