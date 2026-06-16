pragma circom 2.1.6;

include "circomlib/circuits/poseidon.circom";

template ShieldedWithdrawal() {
    // Public signals. Budol and the pool use these to bind the proof to one
    // credited note, one withdrawal nullifier, and one recipient.
    signal input noteCommitment;
    signal input nullifierHash;
    signal input recipient;
    signal input chainId;
    signal input tokenAddress;
    signal input poolAddress;
    signal input denomination;

    // Private witness from the local browser note.
    signal input secret;
    signal input blinding;

    component commitment = Poseidon(6);
    commitment.inputs[0] <== secret;
    commitment.inputs[1] <== blinding;
    commitment.inputs[2] <== chainId;
    commitment.inputs[3] <== tokenAddress;
    commitment.inputs[4] <== poolAddress;
    commitment.inputs[5] <== denomination;
    commitment.out === noteCommitment;

    component nullifier = Poseidon(3);
    nullifier.inputs[0] <== secret;
    nullifier.inputs[1] <== blinding;
    nullifier.inputs[2] <== recipient;
    nullifier.out === nullifierHash;
}

component main { public [noteCommitment, nullifierHash, recipient, chainId, tokenAddress, poolAddress, denomination] } = ShieldedWithdrawal();
