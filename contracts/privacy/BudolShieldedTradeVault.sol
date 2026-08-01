// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

interface IERC20ShieldedTrade {
    function balanceOf(address account) external view returns (uint256);
    function transfer(address to, uint256 value) external returns (bool);
    function transferFrom(address from, address to, uint256 value) external returns (bool);
}

interface IPoseidonT3 {
    function poseidon(uint256[2] calldata input) external view returns (uint256);
}

interface IShieldedTradeVerifier {
    function verifyProof(
        uint256[2] calldata pA,
        uint256[2][2] calldata pB,
        uint256[2] calldata pC,
        uint256[9] calldata publicSignals
    ) external view returns (bool);
}

/// @notice Fixed-denomination deposit pool for relayed, batched private orders.
/// @dev Deposits are public, but a spend proves membership without revealing
///      which deposit leaf is consumed. Order details are committed in the
///      proof and delivered privately to the BudolPH batch coordinator.
contract BudolShieldedTradeVault {
    uint256 private constant SNARK_SCALAR_FIELD =
        21888242871839275222246405745257275088548364400416034343698204186575808495617;
    uint32 public constant TREE_LEVELS = 20;
    uint32 public constant ROOT_HISTORY_SIZE = 64;

    struct Batch {
        uint64 executeAfter;
        uint64 expiresAt;
        uint64 orderCount;
        bool open;
        bool settled;
        uint256 collateral;
    }

    IERC20ShieldedTrade public immutable token;
    IPoseidonT3 public immutable hasher;
    address public immutable escrow;
    uint256 public immutable denomination;
    uint256 public immutable feeBps;
    uint256 public immutable chainId;

    address public owner;
    IShieldedTradeVerifier public verifier;
    uint32 public nextLeafIndex;
    uint32 public currentRootIndex;

    mapping(uint32 => bytes32) public zeros;
    mapping(uint32 => bytes32) public filledSubtrees;
    mapping(uint32 => bytes32) public roots;
    mapping(bytes32 => bool) public knownRoots;
    mapping(bytes32 => bool) public commitments;
    mapping(uint32 => bytes32) public commitmentAt;
    mapping(bytes32 => bool) public usedNullifiers;
    mapping(bytes32 => bool) public acceptedOrders;
    mapping(bytes32 => Batch) public batches;

    bool private locked;

    event OwnershipTransferred(address indexed previousOwner, address indexed newOwner);
    event VerifierUpdated(address indexed previousVerifier, address indexed newVerifier);
    event Deposit(bytes32 indexed commitment, uint32 leafIndex, bytes32 root, uint256 timestamp);
    event BatchOpened(bytes32 indexed batchId, uint64 executeAfter, uint64 expiresAt);
    event PrivateOrderAccepted(
        bytes32 indexed batchId,
        bytes32 indexed orderCommitment,
        bytes32 indexed nullifierHash,
        uint256 denomination,
        address relayer
    );
    event BatchSettled(bytes32 indexed batchId, uint256 orderCount, uint256 collateral, address escrow);

    modifier onlyOwner() {
        require(msg.sender == owner, "not owner");
        _;
    }

    modifier nonReentrant() {
        require(!locked, "reentrant");
        locked = true;
        _;
        locked = false;
    }

    constructor(
        address tokenAddress,
        address hasherAddress,
        address verifierAddress,
        address escrowAddress,
        address initialOwner,
        uint256 fixedDenomination,
        uint256 tradingFeeBps
    ) {
        require(tokenAddress != address(0), "token required");
        require(hasherAddress != address(0), "hasher required");
        require(verifierAddress != address(0), "verifier required");
        require(escrowAddress != address(0), "escrow required");
        require(initialOwner != address(0), "owner required");
        require(fixedDenomination > 0, "denomination required");
        require(tradingFeeBps <= 1_000, "fee too high");

        token = IERC20ShieldedTrade(tokenAddress);
        hasher = IPoseidonT3(hasherAddress);
        verifier = IShieldedTradeVerifier(verifierAddress);
        escrow = escrowAddress;
        owner = initialOwner;
        denomination = fixedDenomination;
        feeBps = tradingFeeBps;
        chainId = block.chainid;

        bytes32 currentZero;
        for (uint32 level = 0; level < TREE_LEVELS; level++) {
            zeros[level] = currentZero;
            filledSubtrees[level] = currentZero;
            currentZero = _hashLeftRight(currentZero, currentZero);
        }
        roots[0] = currentZero;
        knownRoots[currentZero] = true;
        emit OwnershipTransferred(address(0), initialOwner);
        emit VerifierUpdated(address(0), verifierAddress);
    }

    function transferOwnership(address newOwner) external onlyOwner {
        require(newOwner != address(0), "owner required");
        emit OwnershipTransferred(owner, newOwner);
        owner = newOwner;
    }

    function setVerifier(address newVerifier) external onlyOwner {
        require(newVerifier != address(0), "verifier required");
        emit VerifierUpdated(address(verifier), newVerifier);
        verifier = IShieldedTradeVerifier(newVerifier);
    }

    function latestRoot() external view returns (bytes32) {
        return roots[currentRootIndex];
    }

    function deposit(bytes32 commitment) external nonReentrant returns (uint32 leafIndex, bytes32 root) {
        require(commitment != bytes32(0), "commitment required");
        require(uint256(commitment) < SNARK_SCALAR_FIELD, "commitment outside field");
        require(!commitments[commitment], "commitment used");
        require(nextLeafIndex < uint32(1) << TREE_LEVELS, "tree full");

        uint256 beforeBalance = token.balanceOf(address(this));
        require(token.transferFrom(msg.sender, address(this), denomination), "transferFrom failed");
        require(token.balanceOf(address(this)) - beforeBalance == denomination, "unsupported token transfer");

        commitments[commitment] = true;
        (leafIndex, root) = _insert(commitment);
        commitmentAt[leafIndex] = commitment;
        emit Deposit(commitment, leafIndex, root, block.timestamp);
    }

    function getCommitments(uint32 offset, uint32 limit) external view returns (bytes32[] memory values) {
        uint32 end = offset + limit;
        if (end > nextLeafIndex) end = nextLeafIndex;
        if (offset >= end) return new bytes32[](0);
        values = new bytes32[](end - offset);
        for (uint32 index = offset; index < end; index++) {
            values[index - offset] = commitmentAt[index];
        }
    }

    function openBatch(bytes32 batchId, uint64 executeAfter, uint64 expiresAt) external onlyOwner {
        require(batchId != bytes32(0), "batch required");
        require(uint256(batchId) < SNARK_SCALAR_FIELD, "batch outside field");
        require(!batches[batchId].open && !batches[batchId].settled, "batch exists");
        require(executeAfter >= block.timestamp, "executeAfter in past");
        require(expiresAt > executeAfter, "bad expiry");
        batches[batchId] = Batch({
            executeAfter: executeAfter,
            expiresAt: expiresAt,
            orderCount: 0,
            open: true,
            settled: false,
            collateral: 0
        });
        emit BatchOpened(batchId, executeAfter, expiresAt);
    }

    function submitPrivateOrder(
        bytes32 root,
        bytes32 nullifierHash,
        bytes32 orderCommitment,
        bytes32 batchId,
        uint256[2] calldata pA,
        uint256[2][2] calldata pB,
        uint256[2] calldata pC,
        uint256[9] calldata publicSignals
    ) external nonReentrant {
        require(publicSignals[0] == uint256(root), "root mismatch");
        require(publicSignals[1] == uint256(nullifierHash), "nullifier mismatch");
        require(publicSignals[2] == uint256(orderCommitment), "order mismatch");
        require(publicSignals[3] == chainId, "chain mismatch");
        require(publicSignals[4] == uint256(uint160(address(token))), "token mismatch");
        require(publicSignals[5] == uint256(uint160(address(this))), "vault mismatch");
        require(publicSignals[6] == denomination, "denomination mismatch");
        require(publicSignals[7] == feeBps, "fee mismatch");
        require(publicSignals[8] == uint256(batchId), "batch mismatch");
        require(verifier.verifyProof(pA, pB, pC, publicSignals), "invalid proof");

        _acceptOrder(root, nullifierHash, orderCommitment, batchId);
    }

    /// @notice Permissionless after the delay. The destination is immutable,
    /// so this removes operator liveness risk without exposing collateral.
    function settleBatch(bytes32 batchId) external nonReentrant {
        Batch storage batch = batches[batchId];
        require(batch.open, "batch not open");
        require(!batch.settled, "batch settled");
        require(block.timestamp >= batch.executeAfter, "batch not ready");
        require(batch.orderCount > 0, "empty batch");

        batch.open = false;
        batch.settled = true;
        uint256 collateral = batch.collateral;
        require(token.transfer(escrow, collateral), "escrow transfer failed");
        emit BatchSettled(batchId, batch.orderCount, collateral, escrow);
    }

    function _acceptOrder(
        bytes32 root,
        bytes32 nullifierHash,
        bytes32 orderCommitment,
        bytes32 batchId
    ) internal {
        require(root != bytes32(0) && knownRoots[root], "unknown root");
        require(nullifierHash != bytes32(0), "nullifier required");
        require(orderCommitment != bytes32(0), "order required");
        require(!usedNullifiers[nullifierHash], "nullifier used");
        require(!acceptedOrders[orderCommitment], "order used");

        Batch storage batch = batches[batchId];
        require(batch.open && !batch.settled, "batch closed");
        require(block.timestamp < batch.executeAfter, "batch sealed");
        require(block.timestamp < batch.expiresAt, "batch expired");

        usedNullifiers[nullifierHash] = true;
        acceptedOrders[orderCommitment] = true;
        batch.orderCount += 1;
        batch.collateral += denomination;
        emit PrivateOrderAccepted(batchId, orderCommitment, nullifierHash, denomination, msg.sender);
    }

    function _insert(bytes32 leaf) internal returns (uint32 leafIndex, bytes32 root) {
        leafIndex = nextLeafIndex;
        uint32 index = leafIndex;
        bytes32 current = leaf;

        for (uint32 level = 0; level < TREE_LEVELS; level++) {
            if ((index & 1) == 0) {
                filledSubtrees[level] = current;
                current = _hashLeftRight(current, zeros[level]);
            } else {
                current = _hashLeftRight(filledSubtrees[level], current);
            }
            index >>= 1;
        }

        nextLeafIndex = leafIndex + 1;
        uint32 newRootIndex = (currentRootIndex + 1) % ROOT_HISTORY_SIZE;
        bytes32 evictedRoot = roots[newRootIndex];
        if (evictedRoot != bytes32(0)) {
            knownRoots[evictedRoot] = false;
        }
        roots[newRootIndex] = current;
        knownRoots[current] = true;
        currentRootIndex = newRootIndex;
        root = current;
    }

    function _hashLeftRight(bytes32 left, bytes32 right) internal view returns (bytes32) {
        uint256[2] memory input = [uint256(left), uint256(right)];
        uint256 result = hasher.poseidon(input);
        require(result < SNARK_SCALAR_FIELD, "invalid hash");
        return bytes32(result);
    }
}
