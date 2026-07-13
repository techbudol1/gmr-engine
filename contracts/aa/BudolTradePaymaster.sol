// SPDX-License-Identifier: MIT
pragma solidity ^0.8.28;

struct PackedUserOperation {
    address sender;
    uint256 nonce;
    bytes initCode;
    bytes callData;
    bytes32 accountGasLimits;
    uint256 preVerificationGas;
    bytes32 gasFees;
    bytes paymasterAndData;
    bytes signature;
}

interface IEntryPoint {
    function depositTo(address account) external payable;
    function withdrawTo(address payable withdrawAddress, uint256 withdrawAmount) external;
    function balanceOf(address account) external view returns (uint256);
}

interface IPaymaster {
    enum PostOpMode {
        opSucceeded,
        opReverted,
        postOpReverted
    }

    function validatePaymasterUserOp(
        PackedUserOperation calldata userOp,
        bytes32 userOpHash,
        uint256 maxCost
    ) external returns (bytes memory context, uint256 validationData);

    function postOp(
        PostOpMode mode,
        bytes calldata context,
        uint256 actualGasCost,
        uint256 actualUserOpFeePerGas
    ) external;
}

contract BudolTradePaymaster is IPaymaster {
    error Disabled();
    error InvalidCall();
    error InvalidEntryPoint();
    error InvalidPaymasterData();
    error InvalidSignature();
    error NotOwner();

    bytes4 private constant SIMPLE_ACCOUNT_EXECUTE_SELECTOR = bytes4(keccak256("execute(address,uint256,bytes)"));
    bytes4 private constant ERC20_TRANSFER_SELECTOR = bytes4(keccak256("transfer(address,uint256)"));
    uint256 private constant PAYMASTER_DATA_OFFSET = 52;

    IEntryPoint public immutable entryPoint;
    address public owner;
    address public signer;
    address public sponsoredToken;
    address public sponsoredRecipient;
    bool public enabled = true;

    event OwnerUpdated(address indexed owner);
    event SignerUpdated(address indexed signer);
    event SponsorshipUpdated(address indexed sponsoredToken, address indexed sponsoredRecipient);
    event EnabledUpdated(bool enabled);
    event DepositAdded(address indexed sender, uint256 amount);
    event Withdrawn(address indexed to, uint256 amount);

    modifier onlyOwner() {
        if (msg.sender != owner) revert NotOwner();
        _;
    }

    constructor(address entryPoint_, address signer_, address sponsoredToken_, address sponsoredRecipient_) {
        if (entryPoint_ == address(0) || signer_ == address(0) || sponsoredToken_ == address(0) || sponsoredRecipient_ == address(0)) {
            revert InvalidCall();
        }
        entryPoint = IEntryPoint(entryPoint_);
        owner = msg.sender;
        signer = signer_;
        sponsoredToken = sponsoredToken_;
        sponsoredRecipient = sponsoredRecipient_;
    }

    receive() external payable {
        emit DepositAdded(msg.sender, msg.value);
    }

    function addDeposit() external payable onlyOwner {
        entryPoint.depositTo{value: msg.value}(address(this));
        emit DepositAdded(msg.sender, msg.value);
    }

    function withdrawDepositTo(address payable to, uint256 amount) external onlyOwner {
        entryPoint.withdrawTo(to, amount);
        emit Withdrawn(to, amount);
    }

    function setEnabled(bool enabled_) external onlyOwner {
        enabled = enabled_;
        emit EnabledUpdated(enabled_);
    }

    function setOwner(address owner_) external onlyOwner {
        if (owner_ == address(0)) revert InvalidCall();
        owner = owner_;
        emit OwnerUpdated(owner_);
    }

    function setSigner(address signer_) external onlyOwner {
        if (signer_ == address(0)) revert InvalidCall();
        signer = signer_;
        emit SignerUpdated(signer_);
    }

    function setSponsorship(address token_, address recipient_) external onlyOwner {
        if (token_ == address(0) || recipient_ == address(0)) revert InvalidCall();
        sponsoredToken = token_;
        sponsoredRecipient = recipient_;
        emit SponsorshipUpdated(token_, recipient_);
    }

    function getDeposit() external view returns (uint256) {
        return entryPoint.balanceOf(address(this));
    }

    function validatePaymasterUserOp(
        PackedUserOperation calldata userOp,
        bytes32,
        uint256
    ) external view returns (bytes memory context, uint256 validationData) {
        if (msg.sender != address(entryPoint)) revert InvalidEntryPoint();
        if (!enabled) revert Disabled();
        _validateSponsoredCall(userOp.callData);
        (uint256 validUntil, bytes calldata signature) = _decodePaymasterData(userOp.paymasterAndData);
        if (validUntil != 0 && validUntil < block.timestamp) revert InvalidSignature();
        bytes32 sponsorHash = getSponsorHash(userOp, validUntil);
        if (_recover(_toEthSignedMessageHash(sponsorHash), signature) != signer) revert InvalidSignature();
        return ("", 0);
    }

    function postOp(
        PostOpMode,
        bytes calldata,
        uint256,
        uint256
    ) external view {
        if (msg.sender != address(entryPoint)) revert InvalidEntryPoint();
    }

    function getSponsorHash(PackedUserOperation calldata userOp, uint256 validUntil) public view returns (bytes32) {
        return keccak256(
            abi.encode(
                address(this),
                block.chainid,
                userOp.sender,
                userOp.nonce,
                keccak256(userOp.initCode),
                keccak256(userOp.callData),
                userOp.accountGasLimits,
                userOp.preVerificationGas,
                userOp.gasFees,
                validUntil
            )
        );
    }

    function _validateSponsoredCall(bytes calldata callData) internal view {
        if (callData.length < 4 || bytes4(callData[:4]) != SIMPLE_ACCOUNT_EXECUTE_SELECTOR) revert InvalidCall();
        (address target, uint256 value, bytes memory innerCall) = abi.decode(callData[4:], (address, uint256, bytes));
        if (target != sponsoredToken || value != 0 || innerCall.length < 4) revert InvalidCall();
        bytes4 innerSelector;
        assembly {
            innerSelector := mload(add(innerCall, 32))
        }
        if (innerSelector != ERC20_TRANSFER_SELECTOR) revert InvalidCall();
        (address recipient, uint256 amount) = abi.decode(_sliceAfterSelector(innerCall), (address, uint256));
        if (recipient != sponsoredRecipient || amount == 0) revert InvalidCall();
    }

    function _sliceAfterSelector(bytes memory data) internal pure returns (bytes memory out) {
        if (data.length < 4) revert InvalidCall();
        out = new bytes(data.length - 4);
        for (uint256 i = 4; i < data.length; i++) {
            out[i - 4] = data[i];
        }
    }

    function _decodePaymasterData(bytes calldata paymasterAndData) internal pure returns (uint256 validUntil, bytes calldata signature) {
        if (paymasterAndData.length < PAYMASTER_DATA_OFFSET + 32 + 65) revert InvalidPaymasterData();
        validUntil = abi.decode(paymasterAndData[PAYMASTER_DATA_OFFSET:PAYMASTER_DATA_OFFSET + 32], (uint256));
        signature = paymasterAndData[PAYMASTER_DATA_OFFSET + 32:];
        if (signature.length != 65) revert InvalidPaymasterData();
    }

    function _toEthSignedMessageHash(bytes32 hash) internal pure returns (bytes32) {
        return keccak256(abi.encodePacked("\x19Ethereum Signed Message:\n32", hash));
    }

    function _recover(bytes32 digest, bytes calldata signature) internal pure returns (address) {
        bytes32 r;
        bytes32 s;
        uint8 v;
        assembly {
            r := calldataload(signature.offset)
            s := calldataload(add(signature.offset, 32))
            v := byte(0, calldataload(add(signature.offset, 64)))
        }
        if (v < 27) v += 27;
        if (v != 27 && v != 28) revert InvalidSignature();
        address recovered = ecrecover(digest, v, r, s);
        if (recovered == address(0)) revert InvalidSignature();
        return recovered;
    }
}
