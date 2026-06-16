package contracts

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed shielded_withdrawal_verifier.sol
var shieldedWithdrawalVerifierSolidity string

func ERC20Source(contractName string, name string, symbol string, decimals int64) string {
	return fmt.Sprintf(`// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

contract %s {
    string public name;
    string public symbol;
    uint8 public decimals;
    uint256 public totalSupply;
    address public owner;

    mapping(address => uint256) public balanceOf;
    mapping(address => mapping(address => uint256)) public allowance;

    event Transfer(address indexed from, address indexed to, uint256 value);
    event Approval(address indexed owner, address indexed spender, uint256 value);
    event OwnershipTransferred(address indexed previousOwner, address indexed newOwner);

    modifier onlyOwner() {
        require(msg.sender == owner, "not owner");
        _;
    }

    constructor(address initialOwner, uint256 initialSupply) {
        require(initialOwner != address(0), "owner required");
        owner = initialOwner;
        name = %q;
        symbol = %q;
        decimals = %d;
        _mint(initialOwner, initialSupply);
        emit OwnershipTransferred(address(0), initialOwner);
    }

    function transfer(address to, uint256 value) external returns (bool) {
        _transfer(msg.sender, to, value);
        return true;
    }

    function approve(address spender, uint256 value) external returns (bool) {
        allowance[msg.sender][spender] = value;
        emit Approval(msg.sender, spender, value);
        return true;
    }

    function transferFrom(address from, address to, uint256 value) external returns (bool) {
        uint256 allowed = allowance[from][msg.sender];
        require(allowed >= value, "allowance");
        if (allowed != type(uint256).max) {
            allowance[from][msg.sender] = allowed - value;
        }
        _transfer(from, to, value);
        return true;
    }

    function mint(address to, uint256 value) external onlyOwner {
        _mint(to, value);
    }

    function burn(uint256 value) external {
        _burn(msg.sender, value);
    }

    function burnFrom(address from, uint256 value) external {
        uint256 allowed = allowance[from][msg.sender];
        require(allowed >= value, "allowance");
        if (allowed != type(uint256).max) {
            allowance[from][msg.sender] = allowed - value;
        }
        _burn(from, value);
    }

    function transferOwnership(address newOwner) external onlyOwner {
        require(newOwner != address(0), "owner required");
        emit OwnershipTransferred(owner, newOwner);
        owner = newOwner;
    }

    function _transfer(address from, address to, uint256 value) internal {
        require(to != address(0), "to required");
        uint256 balance = balanceOf[from];
        require(balance >= value, "balance");
        unchecked {
            balanceOf[from] = balance - value;
        }
        balanceOf[to] += value;
        emit Transfer(from, to, value);
    }

    function _mint(address to, uint256 value) internal {
        require(to != address(0), "to required");
        totalSupply += value;
        balanceOf[to] += value;
        emit Transfer(address(0), to, value);
    }

    function _burn(address from, uint256 value) internal {
        uint256 balance = balanceOf[from];
        require(balance >= value, "balance");
        unchecked {
            balanceOf[from] = balance - value;
        }
        totalSupply -= value;
        emit Transfer(from, address(0), value);
    }
}
`, contractName, name, symbol, decimals)
}

func ERC1155EditionSource(contractName string, name string, symbol string) string {
	return fmt.Sprintf(`// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

contract %s {
    string public name;
    string public symbol;
    string private baseTokenURI;
    string public contractURI;
    address public owner;

    mapping(uint256 => mapping(address => uint256)) private balances;
    mapping(address => mapping(address => bool)) private operatorApprovals;
    mapping(uint256 => uint256) public totalSupply;

    event TransferSingle(address indexed operator, address indexed from, address indexed to, uint256 id, uint256 value);
    event TransferBatch(address indexed operator, address indexed from, address indexed to, uint256[] ids, uint256[] values);
    event ApprovalForAll(address indexed account, address indexed operator, bool approved);
    event URI(string value, uint256 indexed id);
    event OwnershipTransferred(address indexed previousOwner, address indexed newOwner);

    modifier onlyOwner() {
        require(msg.sender == owner, "not owner");
        _;
    }

    constructor(
        address initialOwner,
        string memory initialBaseURI,
        string memory initialContractURI,
        address initialRecipient,
        uint256 initialTokenId,
        uint256 initialSupply
    ) {
        require(initialOwner != address(0), "owner required");
        owner = initialOwner;
        name = %q;
        symbol = %q;
        baseTokenURI = initialBaseURI;
        contractURI = initialContractURI;
        emit OwnershipTransferred(address(0), initialOwner);

        if (initialSupply > 0) {
            address receiver = initialRecipient == address(0) ? initialOwner : initialRecipient;
            _mint(receiver, initialTokenId, initialSupply);
        }
    }

    function supportsInterface(bytes4 interfaceId) external pure returns (bool) {
        return interfaceId == 0x01ffc9a7 || interfaceId == 0xd9b67a26 || interfaceId == 0x0e89341c;
    }

    function uri(uint256) external view returns (string memory) {
        return baseTokenURI;
    }

    function balanceOf(address account, uint256 id) public view returns (uint256) {
        require(account != address(0), "account required");
        return balances[id][account];
    }

    function balanceOfBatch(address[] calldata accounts, uint256[] calldata ids) external view returns (uint256[] memory batchBalances) {
        require(accounts.length == ids.length, "length mismatch");
        batchBalances = new uint256[](accounts.length);
        for (uint256 i = 0; i < accounts.length; i++) {
            batchBalances[i] = balanceOf(accounts[i], ids[i]);
        }
    }

    function isApprovedForAll(address account, address operator) external view returns (bool) {
        return operatorApprovals[account][operator];
    }

    function setApprovalForAll(address operator, bool approved) external {
        require(operator != msg.sender, "self approval");
        operatorApprovals[msg.sender][operator] = approved;
        emit ApprovalForAll(msg.sender, operator, approved);
    }

    function safeTransferFrom(address from, address to, uint256 id, uint256 amount, bytes calldata data) external {
        require(from == msg.sender || operatorApprovals[from][msg.sender], "not approved");
        _safeTransferFrom(from, to, id, amount, data);
    }

    function safeBatchTransferFrom(address from, address to, uint256[] calldata ids, uint256[] calldata amounts, bytes calldata data) external {
        require(from == msg.sender || operatorApprovals[from][msg.sender], "not approved");
        require(ids.length == amounts.length, "length mismatch");
        require(to != address(0), "to required");
        for (uint256 i = 0; i < ids.length; i++) {
            uint256 id = ids[i];
            uint256 amount = amounts[i];
            uint256 balance = balances[id][from];
            require(balance >= amount, "balance");
            unchecked {
                balances[id][from] = balance - amount;
            }
            balances[id][to] += amount;
        }
        emit TransferBatch(msg.sender, from, to, ids, amounts);
        data;
    }

    function setURI(string calldata newURI) external onlyOwner {
        baseTokenURI = newURI;
        emit URI(newURI, 0);
    }

    function setContractURI(string calldata newContractURI) external onlyOwner {
        contractURI = newContractURI;
    }

    function mint(address to, uint256 id, uint256 amount, bytes calldata data) external onlyOwner {
        _mint(to, id, amount);
        data;
    }

    function mintBatch(address to, uint256[] calldata ids, uint256[] calldata amounts, bytes calldata data) external onlyOwner {
        require(ids.length == amounts.length, "length mismatch");
        require(to != address(0), "to required");
        for (uint256 i = 0; i < ids.length; i++) {
            balances[ids[i]][to] += amounts[i];
            totalSupply[ids[i]] += amounts[i];
        }
        emit TransferBatch(msg.sender, address(0), to, ids, amounts);
        data;
    }

    function burn(address account, uint256 id, uint256 amount) external {
        require(account == msg.sender || operatorApprovals[account][msg.sender], "not approved");
        uint256 balance = balances[id][account];
        require(balance >= amount, "balance");
        unchecked {
            balances[id][account] = balance - amount;
        }
        totalSupply[id] -= amount;
        emit TransferSingle(msg.sender, account, address(0), id, amount);
    }

    function exists(uint256 id) external view returns (bool) {
        return totalSupply[id] > 0;
    }

    function transferOwnership(address newOwner) external onlyOwner {
        require(newOwner != address(0), "owner required");
        emit OwnershipTransferred(owner, newOwner);
        owner = newOwner;
    }

    function _safeTransferFrom(address from, address to, uint256 id, uint256 amount, bytes calldata data) internal {
        require(to != address(0), "to required");
        uint256 balance = balances[id][from];
        require(balance >= amount, "balance");
        unchecked {
            balances[id][from] = balance - amount;
        }
        balances[id][to] += amount;
        emit TransferSingle(msg.sender, from, to, id, amount);
        data;
    }

    function _mint(address to, uint256 id, uint256 amount) internal {
        require(to != address(0), "to required");
        balances[id][to] += amount;
        totalSupply[id] += amount;
        emit TransferSingle(msg.sender, address(0), to, id, amount);
    }
}
`, contractName, name, symbol)
}

func PrivateClaimRegistrySource(contractName string) string {
	return fmt.Sprintf(`// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

interface IPrivateClaimVerifier {
    function verifyProof(bytes calldata proof, uint256[] calldata publicSignals) external view returns (bool);
}

contract %s {
    address public owner;
    IPrivateClaimVerifier public verifier;

    mapping(bytes32 => mapping(bytes32 => bool)) public acceptedRoots;
    mapping(bytes32 => uint256) public resolvedOutcome;
    mapping(bytes32 => bool) public usedNullifiers;

    event OwnershipTransferred(address indexed previousOwner, address indexed newOwner);
    event VerifierUpdated(address indexed previousVerifier, address indexed newVerifier);
    event RootAccepted(bytes32 indexed marketId, bytes32 indexed root, uint256 leafCount);
    event RootRevoked(bytes32 indexed marketId, bytes32 indexed root);
    event MarketResolved(bytes32 indexed marketId, uint256 resolvedOutcome);
    event ClaimRegistered(bytes32 indexed marketId, bytes32 indexed root, bytes32 indexed nullifierHash, address claimant);

    modifier onlyOwner() {
        require(msg.sender == owner, "not owner");
        _;
    }

    constructor(address initialOwner, address initialVerifier) {
        require(initialOwner != address(0), "owner required");
        owner = initialOwner;
        verifier = IPrivateClaimVerifier(initialVerifier);
        emit OwnershipTransferred(address(0), initialOwner);
        emit VerifierUpdated(address(0), initialVerifier);
    }

    function transferOwnership(address newOwner) external onlyOwner {
        require(newOwner != address(0), "owner required");
        emit OwnershipTransferred(owner, newOwner);
        owner = newOwner;
    }

    function setVerifier(address newVerifier) external onlyOwner {
        emit VerifierUpdated(address(verifier), newVerifier);
        verifier = IPrivateClaimVerifier(newVerifier);
    }

    function acceptRoot(bytes32 marketId, bytes32 root, uint256 leafCount) external onlyOwner {
        require(marketId != bytes32(0), "market required");
        require(root != bytes32(0), "root required");
        acceptedRoots[marketId][root] = true;
        emit RootAccepted(marketId, root, leafCount);
    }

    function revokeRoot(bytes32 marketId, bytes32 root) external onlyOwner {
        acceptedRoots[marketId][root] = false;
        emit RootRevoked(marketId, root);
    }

    function setMarketResolution(bytes32 marketId, uint256 outcome) external onlyOwner {
        require(outcome == 1 || outcome == 2, "bad outcome");
        resolvedOutcome[marketId] = outcome;
        emit MarketResolved(marketId, outcome);
    }

    function registerVerifiedClaim(
        bytes32 marketId,
        bytes32 root,
        bytes32 nullifierHash,
        bytes calldata proof,
        uint256[] calldata publicSignals
    ) external {
        require(acceptedRoots[marketId][root], "root not accepted");
        require(!usedNullifiers[nullifierHash], "nullifier used");
        require(publicSignals.length == 3, "bad public signals");
        require(publicSignals[0] == uint256(root), "root mismatch");
        require(publicSignals[1] == resolvedOutcome[marketId], "outcome mismatch");
        require(publicSignals[2] == uint256(nullifierHash), "nullifier mismatch");
        require(address(verifier) != address(0), "verifier missing");
        require(verifier.verifyProof(proof, publicSignals), "invalid proof");

        usedNullifiers[nullifierHash] = true;
        emit ClaimRegistered(marketId, root, nullifierHash, msg.sender);
    }

    function registerZKVerifyClaim(
        bytes32 marketId,
        bytes32 root,
        bytes32 nullifierHash,
        bytes32 zkProofSubmissionId
    ) external onlyOwner {
        require(acceptedRoots[marketId][root], "root not accepted");
        require(!usedNullifiers[nullifierHash], "nullifier used");
        require(zkProofSubmissionId != bytes32(0), "proof required");

        usedNullifiers[nullifierHash] = true;
        emit ClaimRegistered(marketId, root, nullifierHash, msg.sender);
    }
}
`, contractName)
}

func ShieldedWithdrawalVerifierSource(contractName string) string {
	source := strings.ReplaceAll(shieldedWithdrawalVerifierSolidity, "contract Groth16Verifier", "contract ShieldedWithdrawalGroth16Verifier")
	source = strings.Replace(source, "function verifyProof(uint[2] calldata _pA, uint[2][2] calldata _pB, uint[2] calldata _pC, uint[7] calldata _pubSignals) public view returns (bool)", "function verifyGroth16Proof(uint[2] calldata _pA, uint[2][2] calldata _pB, uint[2] calldata _pC, uint[7] calldata _pubSignals) public view returns (bool)", 1)
	return source + fmt.Sprintf(`

contract %s is ShieldedWithdrawalGroth16Verifier {
    function verifyProof(bytes calldata proof, uint256[] calldata publicSignals) external view returns (bool) {
        require(publicSignals.length == 7, "bad public signals");
        (uint[2] memory a, uint[2][2] memory b, uint[2] memory c) = abi.decode(proof, (uint[2], uint[2][2], uint[2]));
        uint[7] memory signals;
        for (uint256 i = 0; i < 7; i++) {
            signals[i] = publicSignals[i];
        }
        return this.verifyGroth16Proof(a, b, c, signals);
    }
}
`, contractName)
}

func ShieldedPayoutPoolSource(contractName string) string {
	return fmt.Sprintf(`// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

interface IShieldedPayoutERC20 {
    function transfer(address to, uint256 value) external returns (bool);
    function transferFrom(address from, address to, uint256 value) external returns (bool);
}

interface IShieldedWithdrawalVerifier {
    function verifyProof(bytes calldata proof, uint256[] calldata publicSignals) external view returns (bool);
}

contract %s {
    IShieldedPayoutERC20 public immutable token;
    uint256 public immutable denomination;
    address public owner;
    IShieldedWithdrawalVerifier public verifier;

    mapping(bytes32 => bool) public creditedNotes;
    mapping(bytes32 => bool) public spentNotes;
    mapping(bytes32 => bool) public usedNullifiers;

    bool private locked;

    event OwnershipTransferred(address indexed previousOwner, address indexed newOwner);
    event VerifierUpdated(address indexed previousVerifier, address indexed newVerifier);
    event NoteCredited(bytes32 indexed noteCommitment);
    event Withdrawn(bytes32 indexed noteCommitment, bytes32 indexed nullifierHash, address recipient, address relayer);

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

    constructor(address tokenAddress, address initialOwner, address initialVerifier, uint256 fixedDenomination) {
        require(tokenAddress != address(0), "token required");
        require(initialOwner != address(0), "owner required");
        require(fixedDenomination > 0, "denomination required");
        token = IShieldedPayoutERC20(tokenAddress);
        owner = initialOwner;
        verifier = IShieldedWithdrawalVerifier(initialVerifier);
        denomination = fixedDenomination;
        emit OwnershipTransferred(address(0), initialOwner);
        emit VerifierUpdated(address(0), initialVerifier);
    }

    function transferOwnership(address newOwner) external onlyOwner {
        require(newOwner != address(0), "owner required");
        emit OwnershipTransferred(owner, newOwner);
        owner = newOwner;
    }

    function setVerifier(address newVerifier) external onlyOwner {
        emit VerifierUpdated(address(verifier), newVerifier);
        verifier = IShieldedWithdrawalVerifier(newVerifier);
    }

    function depositAndCredit(bytes32 noteCommitment) external onlyOwner {
        require(token.transferFrom(msg.sender, address(this), denomination), "transferFrom failed");
        _credit(noteCommitment);
    }

    function creditFundedNote(bytes32 noteCommitment) external onlyOwner {
        _credit(noteCommitment);
    }

    function withdrawVerified(
        bytes32 noteCommitment,
        bytes32 nullifierHash,
        address recipient,
        address relayer,
        uint256 relayerFee,
        bytes calldata proof,
        uint256[] calldata publicSignals
    ) external nonReentrant {
        require(address(verifier) != address(0), "verifier missing");
        require(publicSignals.length >= 2, "bad public signals");
        require(publicSignals[0] == uint256(noteCommitment), "note mismatch");
        require(publicSignals[1] == uint256(nullifierHash), "nullifier mismatch");
        require(verifier.verifyProof(proof, publicSignals), "invalid proof");
        _withdraw(noteCommitment, nullifierHash, bytes32(0), recipient, relayer, relayerFee);
    }

    function withdrawWithZKVerify(
        bytes32 noteCommitment,
        bytes32 nullifierHash,
        bytes32 zkProofSubmissionId,
        address recipient,
        address relayer,
        uint256 relayerFee
    ) external onlyOwner nonReentrant {
        require(zkProofSubmissionId != bytes32(0), "proof required");
        _withdraw(noteCommitment, nullifierHash, zkProofSubmissionId, recipient, relayer, relayerFee);
    }

    function _credit(bytes32 noteCommitment) internal {
        require(noteCommitment != bytes32(0), "note required");
        require(!creditedNotes[noteCommitment], "note credited");
        creditedNotes[noteCommitment] = true;
        emit NoteCredited(noteCommitment);
    }

    function _withdraw(
        bytes32 noteCommitment,
        bytes32 nullifierHash,
        bytes32,
        address recipient,
        address relayer,
        uint256 relayerFee
    ) internal {
        require(noteCommitment != bytes32(0), "note required");
        require(nullifierHash != bytes32(0), "nullifier required");
        require(recipient != address(0), "recipient required");
        require(creditedNotes[noteCommitment], "note not credited");
        require(!spentNotes[noteCommitment], "note spent");
        require(!usedNullifiers[nullifierHash], "nullifier used");
        require(relayerFee <= denomination / 10, "fee too high");

        spentNotes[noteCommitment] = true;
        usedNullifiers[nullifierHash] = true;

        uint256 payout = denomination - relayerFee;
        require(token.transfer(recipient, payout), "payout failed");
        if (relayerFee > 0) {
            address feeRecipient = relayer == address(0) ? msg.sender : relayer;
            require(token.transfer(feeRecipient, relayerFee), "fee failed");
        }
        emit Withdrawn(noteCommitment, nullifierHash, recipient, relayer);
    }
}
`, contractName)
}

func BudolEscrowSource(contractName string) string {
	return fmt.Sprintf(`// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

interface IBudolERC20 {
    function transfer(address to, uint256 value) external returns (bool);
    function transferFrom(address from, address to, uint256 value) external returns (bool);
}

contract %s {
    enum MarketState {
        Unset,
        Open,
        Paused,
        Resolved,
        Cancelled
    }

    struct Market {
        uint64 startTime;
        uint64 endTime;
        uint8 winningOutcome;
        uint16 feeBps;
        uint256 yesPool;
        uint256 noPool;
        uint256 feeAccrued;
        MarketState state;
        string dataURI;
    }

    struct Position {
        uint256 yes;
        uint256 no;
        bool claimed;
    }

    uint8 public constant OUTCOME_YES = 1;
    uint8 public constant OUTCOME_NO = 2;
    uint16 public constant BPS = 10000;
    uint16 public constant MAX_FEE_BPS = 1000;

    IBudolERC20 public immutable stakeToken;
    address public owner;
    address public treasury;
    uint16 public defaultFeeBps;

    mapping(bytes32 => Market) public markets;
    mapping(bytes32 => mapping(address => Position)) public positions;

    bool private locked;

    event OwnershipTransferred(address indexed previousOwner, address indexed newOwner);
    event TreasuryUpdated(address indexed previousTreasury, address indexed newTreasury);
    event DefaultFeeUpdated(uint16 previousFeeBps, uint16 newFeeBps);
    event MarketCreated(bytes32 indexed marketId, uint64 startTime, uint64 endTime, uint16 feeBps, string dataURI);
    event MarketPaused(bytes32 indexed marketId);
    event MarketReopened(bytes32 indexed marketId);
    event MarketResolved(bytes32 indexed marketId, uint8 winningOutcome, uint256 feeAccrued);
    event MarketCancelled(bytes32 indexed marketId);
    event PositionBought(bytes32 indexed marketId, address indexed trader, uint8 indexed outcome, uint256 amount);
    event Claimed(bytes32 indexed marketId, address indexed trader, uint256 amount);
    event FeesWithdrawn(bytes32 indexed marketId, address indexed treasury, uint256 amount);

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

    constructor(address tokenAddress, address initialOwner, address initialTreasury, uint16 initialFeeBps) {
        require(tokenAddress != address(0), "token required");
        require(initialOwner != address(0), "owner required");
        require(initialTreasury != address(0), "treasury required");
        require(initialFeeBps <= MAX_FEE_BPS, "fee too high");
        stakeToken = IBudolERC20(tokenAddress);
        owner = initialOwner;
        treasury = initialTreasury;
        defaultFeeBps = initialFeeBps;
        emit OwnershipTransferred(address(0), initialOwner);
        emit TreasuryUpdated(address(0), initialTreasury);
        emit DefaultFeeUpdated(0, initialFeeBps);
    }

    function createMarket(bytes32 marketId, uint64 startTime, uint64 endTime, uint16 feeBps, string calldata dataURI) external onlyOwner {
        _createMarket(marketId, startTime, endTime, feeBps, dataURI);
    }

    function createMarketWithDefaultFee(bytes32 marketId, uint64 startTime, uint64 endTime, string calldata dataURI) external onlyOwner {
        _createMarket(marketId, startTime, endTime, defaultFeeBps, dataURI);
    }

    function _createMarket(bytes32 marketId, uint64 startTime, uint64 endTime, uint16 feeBps, string calldata dataURI) internal {
        require(marketId != bytes32(0), "market required");
        require(markets[marketId].state == MarketState.Unset, "market exists");
        require(feeBps <= MAX_FEE_BPS, "fee too high");
        if (startTime != 0 && endTime != 0) {
            require(endTime > startTime, "bad dates");
        }
        markets[marketId] = Market({
            startTime: startTime,
            endTime: endTime,
            winningOutcome: 0,
            feeBps: feeBps,
            yesPool: 0,
            noPool: 0,
            feeAccrued: 0,
            state: MarketState.Open,
            dataURI: dataURI
        });
        emit MarketCreated(marketId, startTime, endTime, feeBps, dataURI);
    }

    function buy(bytes32 marketId, uint8 outcome, uint256 amount) external nonReentrant {
        Market storage market = markets[marketId];
        require(market.state == MarketState.Open, "market not open");
        require(outcome == OUTCOME_YES || outcome == OUTCOME_NO, "bad outcome");
        require(amount > 0, "amount required");
        require(market.startTime == 0 || block.timestamp >= market.startTime, "not started");
        require(market.endTime == 0 || block.timestamp < market.endTime, "ended");

        require(stakeToken.transferFrom(msg.sender, address(this), amount), "transfer failed");

        Position storage position = positions[marketId][msg.sender];
        if (outcome == OUTCOME_YES) {
            market.yesPool += amount;
            position.yes += amount;
        } else {
            market.noPool += amount;
            position.no += amount;
        }
        emit PositionBought(marketId, msg.sender, outcome, amount);
    }

    function pauseMarket(bytes32 marketId) external onlyOwner {
        Market storage market = markets[marketId];
        require(market.state == MarketState.Open, "not open");
        market.state = MarketState.Paused;
        emit MarketPaused(marketId);
    }

    function reopenMarket(bytes32 marketId) external onlyOwner {
        Market storage market = markets[marketId];
        require(market.state == MarketState.Paused, "not paused");
        market.state = MarketState.Open;
        emit MarketReopened(marketId);
    }

    function cancelMarket(bytes32 marketId) external onlyOwner {
        Market storage market = markets[marketId];
        require(market.state == MarketState.Open || market.state == MarketState.Paused, "cannot cancel");
        market.state = MarketState.Cancelled;
        emit MarketCancelled(marketId);
    }

    function resolveMarket(bytes32 marketId, uint8 winningOutcome) external onlyOwner {
        Market storage market = markets[marketId];
        require(market.state == MarketState.Open || market.state == MarketState.Paused, "cannot resolve");
        require(winningOutcome == OUTCOME_YES || winningOutcome == OUTCOME_NO, "bad outcome");
        require(market.endTime == 0 || block.timestamp >= market.endTime, "not ended");
        uint256 totalWinning = winningOutcome == OUTCOME_YES ? market.yesPool : market.noPool;
        uint256 totalLosing = winningOutcome == OUTCOME_YES ? market.noPool : market.yesPool;
        require(totalWinning > 0, "no winners");
        uint256 fee = (totalLosing * market.feeBps) / BPS;
        market.winningOutcome = winningOutcome;
        market.feeAccrued = fee;
        market.state = MarketState.Resolved;
        emit MarketResolved(marketId, winningOutcome, fee);
    }

    function claim(bytes32 marketId) external nonReentrant returns (uint256 payout) {
        Position storage position = positions[marketId][msg.sender];
        require(!position.claimed, "claimed");
        Market storage market = markets[marketId];
        require(market.state == MarketState.Resolved || market.state == MarketState.Cancelled, "not claimable");
        position.claimed = true;

        if (market.state == MarketState.Cancelled) {
            payout = position.yes + position.no;
        } else {
            uint256 winningStake = market.winningOutcome == OUTCOME_YES ? position.yes : position.no;
            require(winningStake > 0, "no winning stake");
            uint256 totalWinning = market.winningOutcome == OUTCOME_YES ? market.yesPool : market.noPool;
            uint256 distributable = market.yesPool + market.noPool - market.feeAccrued;
            payout = (winningStake * distributable) / totalWinning;
        }

        require(payout > 0, "nothing to claim");
        require(stakeToken.transfer(msg.sender, payout), "transfer failed");
        emit Claimed(marketId, msg.sender, payout);
    }

    function pendingPayout(bytes32 marketId, address trader) external view returns (uint256) {
        Position storage position = positions[marketId][trader];
        if (position.claimed) {
            return 0;
        }
        Market storage market = markets[marketId];
        if (market.state == MarketState.Cancelled) {
            return position.yes + position.no;
        }
        if (market.state != MarketState.Resolved) {
            return 0;
        }
        uint256 winningStake = market.winningOutcome == OUTCOME_YES ? position.yes : position.no;
        if (winningStake == 0) {
            return 0;
        }
        uint256 totalWinning = market.winningOutcome == OUTCOME_YES ? market.yesPool : market.noPool;
        uint256 distributable = market.yesPool + market.noPool - market.feeAccrued;
        return (winningStake * distributable) / totalWinning;
    }

    function withdrawFees(bytes32 marketId) external onlyOwner nonReentrant returns (uint256 amount) {
        Market storage market = markets[marketId];
        require(market.state == MarketState.Resolved, "not resolved");
        amount = market.feeAccrued;
        require(amount > 0, "no fees");
        market.feeAccrued = 0;
        require(stakeToken.transfer(treasury, amount), "transfer failed");
        emit FeesWithdrawn(marketId, treasury, amount);
    }

    function setTreasury(address newTreasury) external onlyOwner {
        require(newTreasury != address(0), "treasury required");
        emit TreasuryUpdated(treasury, newTreasury);
        treasury = newTreasury;
    }

    function setDefaultFeeBps(uint16 newFeeBps) external onlyOwner {
        require(newFeeBps <= MAX_FEE_BPS, "fee too high");
        emit DefaultFeeUpdated(defaultFeeBps, newFeeBps);
        defaultFeeBps = newFeeBps;
    }

    function transferOwnership(address newOwner) external onlyOwner {
        require(newOwner != address(0), "owner required");
        emit OwnershipTransferred(owner, newOwner);
        owner = newOwner;
    }
}
`, contractName)
}
