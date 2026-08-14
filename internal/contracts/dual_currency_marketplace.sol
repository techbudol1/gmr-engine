// SPDX-License-Identifier: MIT
pragma solidity ^0.8.24;

interface IDualCurrencyERC20 {
    function transferFrom(address from, address to, uint256 value) external returns (bool);
}

interface IDualCurrencyERC165 {
    function supportsInterface(bytes4 interfaceId) external view returns (bool);
}

interface IDualCurrencyERC721 {
    function ownerOf(uint256 tokenId) external view returns (address);
    function getApproved(uint256 tokenId) external view returns (address);
    function isApprovedForAll(address account, address operator) external view returns (bool);
    function safeTransferFrom(address from, address to, uint256 tokenId) external;
}

interface IDualCurrencyERC1155 {
    function balanceOf(address account, uint256 id) external view returns (uint256);
    function isApprovedForAll(address account, address operator) external view returns (bool);
    function safeTransferFrom(address from, address to, uint256 id, uint256 amount, bytes calldata data) external;
}

interface IDualCurrencyERC2981 {
    function royaltyInfo(uint256 tokenId, uint256 salePrice) external view returns (address receiver, uint256 royaltyAmount);
}

contract DualCurrencyMarketplace {
    address public constant NATIVE_TOKEN = 0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE;
    uint16 public constant MAX_PLATFORM_FEE_BPS = 2_500;
    uint256 private constant BPS = 10_000;

    enum TokenType {
        ERC721,
        ERC1155
    }

    enum ListingStatus {
        UNSET,
        CREATED,
        COMPLETED,
        CANCELLED
    }

    struct ListingParameters {
        address assetContract;
        uint256 tokenId;
        uint256 quantity;
        address primaryCurrency;
        uint256 primaryPricePerToken;
        address secondaryCurrency;
        uint256 secondaryPricePerToken;
        uint128 startTimestamp;
        uint128 endTimestamp;
        bool reserved;
    }

    struct Listing {
        uint256 listingId;
        address listingCreator;
        address assetContract;
        uint256 tokenId;
        uint256 quantity;
        address primaryCurrency;
        uint256 primaryPricePerToken;
        address secondaryCurrency;
        uint256 secondaryPricePerToken;
        uint128 startTimestamp;
        uint128 endTimestamp;
        bool reserved;
        TokenType tokenType;
        ListingStatus status;
    }

    address public owner;
    address public platformFeeRecipient;
    uint16 public platformFeeBps;
    uint256 public totalListings;

    mapping(uint256 => Listing) private listings;
    mapping(uint256 => mapping(address => bool)) public approvedBuyer;
    bool private locked;

    event ListingAdded(uint256 indexed listingId, address indexed assetContract, address indexed listingCreator);
    event ListingUpdated(uint256 indexed listingId);
    event ListingCancelled(uint256 indexed listingId, address indexed listingCreator);
    event NewSale(
        uint256 indexed listingId,
        address indexed assetContract,
        address indexed buyer,
        address listingCreator,
        uint256 quantity,
        address currency,
        uint256 totalPrice
    );
    event BuyerApprovedForListing(uint256 indexed listingId, address indexed buyer, bool approved);
    event OwnershipTransferred(address indexed previousOwner, address indexed newOwner);
    event PlatformFeeUpdated(address indexed recipient, uint16 feeBps);

    modifier onlyOwner() {
        require(msg.sender == owner, "not owner");
        _;
    }

    modifier nonReentrant() {
        require(!locked, "reentrant call");
        locked = true;
        _;
        locked = false;
    }

    constructor(address initialOwner, address initialPlatformFeeRecipient, uint16 initialPlatformFeeBps) {
        require(initialOwner != address(0), "owner required");
        require(initialPlatformFeeRecipient != address(0), "fee recipient required");
        require(initialPlatformFeeBps <= MAX_PLATFORM_FEE_BPS, "fee too high");
        owner = initialOwner;
        platformFeeRecipient = initialPlatformFeeRecipient;
        platformFeeBps = initialPlatformFeeBps;
        emit OwnershipTransferred(address(0), initialOwner);
        emit PlatformFeeUpdated(initialPlatformFeeRecipient, initialPlatformFeeBps);
    }

    function createListing(ListingParameters calldata params) external returns (uint256 listingId) {
        TokenType tokenType = _validateListing(params, msg.sender);
        listingId = totalListings++;
        listings[listingId] = Listing({
            listingId: listingId,
            listingCreator: msg.sender,
            assetContract: params.assetContract,
            tokenId: params.tokenId,
            quantity: params.quantity,
            primaryCurrency: params.primaryCurrency,
            primaryPricePerToken: params.primaryPricePerToken,
            secondaryCurrency: params.secondaryCurrency,
            secondaryPricePerToken: params.secondaryPricePerToken,
            startTimestamp: params.startTimestamp,
            endTimestamp: params.endTimestamp,
            reserved: params.reserved,
            tokenType: tokenType,
            status: ListingStatus.CREATED
        });
        emit ListingAdded(listingId, params.assetContract, msg.sender);
    }

    function updateListing(uint256 listingId, ListingParameters calldata params) external {
        Listing storage current = listings[listingId];
        require(current.status == ListingStatus.CREATED, "listing inactive");
        require(current.listingCreator == msg.sender, "not listing creator");
        TokenType tokenType = _validateListing(params, msg.sender);
        current.assetContract = params.assetContract;
        current.tokenId = params.tokenId;
        current.quantity = params.quantity;
        current.primaryCurrency = params.primaryCurrency;
        current.primaryPricePerToken = params.primaryPricePerToken;
        current.secondaryCurrency = params.secondaryCurrency;
        current.secondaryPricePerToken = params.secondaryPricePerToken;
        current.startTimestamp = params.startTimestamp;
        current.endTimestamp = params.endTimestamp;
        current.reserved = params.reserved;
        current.tokenType = tokenType;
        emit ListingUpdated(listingId);
    }

    function approveCurrencyForListing(
        uint256 listingId,
        address currency,
        uint256 pricePerTokenInCurrency
    ) external {
        Listing storage listing = listings[listingId];
        require(listing.status == ListingStatus.CREATED, "listing inactive");
        require(listing.listingCreator == msg.sender, "not listing creator");
        require(currency != address(0), "currency required");
        require(currency != listing.primaryCurrency, "use secondary currency");
        require(currency == NATIVE_TOKEN || currency.code.length > 0, "invalid currency");
        require(pricePerTokenInCurrency > 0, "price required");
        listing.secondaryCurrency = currency;
        listing.secondaryPricePerToken = pricePerTokenInCurrency;
        emit ListingUpdated(listingId);
    }

    function approveBuyerForListing(uint256 listingId, address buyer, bool toApprove) external {
        Listing storage listing = listings[listingId];
        require(listing.status == ListingStatus.CREATED, "listing inactive");
        require(listing.listingCreator == msg.sender, "not listing creator");
        require(listing.reserved, "listing not reserved");
        approvedBuyer[listingId][buyer] = toApprove;
        emit BuyerApprovedForListing(listingId, buyer, toApprove);
    }

    function cancelListing(uint256 listingId) external {
        Listing storage listing = listings[listingId];
        require(listing.status == ListingStatus.CREATED, "listing inactive");
        require(listing.listingCreator == msg.sender, "not listing creator");
        listing.status = ListingStatus.CANCELLED;
        emit ListingCancelled(listingId, msg.sender);
    }

    function buyFromListing(
        uint256 listingId,
        address buyFor,
        uint256 quantity,
        address currency,
        uint256 expectedTotalPrice
    ) external payable nonReentrant {
        Listing storage listing = listings[listingId];
        require(listing.status == ListingStatus.CREATED, "listing inactive");
        require(block.timestamp >= listing.startTimestamp && block.timestamp < listing.endTimestamp, "listing not active");
        require(buyFor != address(0), "recipient required");
        require(quantity > 0 && quantity <= listing.quantity, "invalid quantity");
        require(!listing.reserved || approvedBuyer[listingId][msg.sender], "buyer not approved");

        uint256 pricePerToken;
        if (currency == listing.primaryCurrency) {
            pricePerToken = listing.primaryPricePerToken;
        } else if (currency == listing.secondaryCurrency) {
            pricePerToken = listing.secondaryPricePerToken;
        } else {
            revert("currency not approved");
        }
        uint256 totalPrice = pricePerToken * quantity;
        require(totalPrice == expectedTotalPrice, "price changed");
        _validateOwnershipAndApproval(listing, quantity);

        listing.quantity -= quantity;
        if (listing.quantity == 0) {
            listing.status = ListingStatus.COMPLETED;
        }

        _paySale(currency, totalPrice, listing.listingCreator, listing.assetContract, listing.tokenId);
        if (listing.tokenType == TokenType.ERC721) {
            IDualCurrencyERC721(listing.assetContract).safeTransferFrom(listing.listingCreator, buyFor, listing.tokenId);
        } else {
            IDualCurrencyERC1155(listing.assetContract).safeTransferFrom(
                listing.listingCreator,
                buyFor,
                listing.tokenId,
                quantity,
                ""
            );
        }
        emit NewSale(
            listingId,
            listing.assetContract,
            msg.sender,
            listing.listingCreator,
            quantity,
            currency,
            totalPrice
        );
    }

    function getListing(uint256 listingId) external view returns (Listing memory) {
        return listings[listingId];
    }

    function getAllListings(uint256 startId, uint256 endId) external view returns (Listing[] memory result) {
        require(startId <= endId && endId < totalListings, "invalid range");
        result = new Listing[](endId - startId + 1);
        for (uint256 i = startId; i <= endId; i++) {
            result[i - startId] = listings[i];
        }
    }

    function setPlatformFee(address newRecipient, uint16 newFeeBps) external onlyOwner {
        require(newRecipient != address(0), "fee recipient required");
        require(newFeeBps <= MAX_PLATFORM_FEE_BPS, "fee too high");
        platformFeeRecipient = newRecipient;
        platformFeeBps = newFeeBps;
        emit PlatformFeeUpdated(newRecipient, newFeeBps);
    }

    function transferOwnership(address newOwner) external onlyOwner {
        require(newOwner != address(0), "owner required");
        emit OwnershipTransferred(owner, newOwner);
        owner = newOwner;
    }

    function _validateListing(ListingParameters calldata params, address creator) private view returns (TokenType tokenType) {
        require(params.assetContract.code.length > 0, "asset contract required");
        require(params.quantity > 0, "quantity required");
        require(params.startTimestamp < params.endTimestamp, "invalid duration");
        require(params.endTimestamp > block.timestamp, "listing expired");
        _validateCurrencies(params);

        bool isERC721 = _supportsInterface(params.assetContract, 0x80ac58cd);
        bool isERC1155 = _supportsInterface(params.assetContract, 0xd9b67a26);
        require(isERC721 != isERC1155, "unsupported asset");
        if (isERC721) {
            require(params.quantity == 1, "ERC721 quantity must be 1");
            require(IDualCurrencyERC721(params.assetContract).ownerOf(params.tokenId) == creator, "not token owner");
            require(
                IDualCurrencyERC721(params.assetContract).getApproved(params.tokenId) == address(this) ||
                    IDualCurrencyERC721(params.assetContract).isApprovedForAll(creator, address(this)),
                "marketplace not approved"
            );
            return TokenType.ERC721;
        }

        require(IDualCurrencyERC1155(params.assetContract).balanceOf(creator, params.tokenId) >= params.quantity, "insufficient tokens");
        require(IDualCurrencyERC1155(params.assetContract).isApprovedForAll(creator, address(this)), "marketplace not approved");
        return TokenType.ERC1155;
    }

    function _validateCurrencies(ListingParameters calldata params) private view {
        require(params.primaryCurrency != address(0) && params.secondaryCurrency != address(0), "currency required");
        require(params.primaryCurrency != params.secondaryCurrency, "currencies must differ");
        require(params.primaryPricePerToken > 0 && params.secondaryPricePerToken > 0, "price required");
        require(params.primaryCurrency == NATIVE_TOKEN || params.primaryCurrency.code.length > 0, "invalid primary currency");
        require(params.secondaryCurrency == NATIVE_TOKEN || params.secondaryCurrency.code.length > 0, "invalid secondary currency");
    }

    function _validateOwnershipAndApproval(Listing storage listing, uint256 quantity) private view {
        if (listing.tokenType == TokenType.ERC721) {
            IDualCurrencyERC721 token = IDualCurrencyERC721(listing.assetContract);
            require(token.ownerOf(listing.tokenId) == listing.listingCreator, "seller no longer owns token");
            require(
                token.getApproved(listing.tokenId) == address(this) ||
                    token.isApprovedForAll(listing.listingCreator, address(this)),
                "marketplace not approved"
            );
        } else {
            IDualCurrencyERC1155 token = IDualCurrencyERC1155(listing.assetContract);
            require(token.balanceOf(listing.listingCreator, listing.tokenId) >= quantity, "seller balance too low");
            require(token.isApprovedForAll(listing.listingCreator, address(this)), "marketplace not approved");
        }
    }

    function _paySale(address currency, uint256 totalPrice, address seller, address assetContract, uint256 tokenId) private {
        uint256 platformFee = (totalPrice * platformFeeBps) / BPS;
        (address royaltyRecipient, uint256 royaltyAmount) = _royaltyInfo(assetContract, tokenId, totalPrice);
        require(platformFee + royaltyAmount <= totalPrice, "fees exceed price");
        uint256 sellerProceeds = totalPrice - platformFee - royaltyAmount;

        if (currency == NATIVE_TOKEN) {
            require(msg.value == totalPrice, "incorrect native value");
            _sendNative(platformFeeRecipient, platformFee);
            _sendNative(royaltyRecipient, royaltyAmount);
            _sendNative(seller, sellerProceeds);
        } else {
            require(msg.value == 0, "native value not accepted");
            _safeTransferFrom(currency, msg.sender, platformFeeRecipient, platformFee);
            _safeTransferFrom(currency, msg.sender, royaltyRecipient, royaltyAmount);
            _safeTransferFrom(currency, msg.sender, seller, sellerProceeds);
        }
    }

    function _royaltyInfo(address assetContract, uint256 tokenId, uint256 totalPrice) private view returns (address, uint256) {
        try IDualCurrencyERC2981(assetContract).royaltyInfo(tokenId, totalPrice) returns (
            address recipient,
            uint256 amount
        ) {
            if (recipient != address(0) && amount > 0) {
                return (recipient, amount);
            }
        } catch {}
        return (address(0), 0);
    }

    function _supportsInterface(address account, bytes4 interfaceId) private view returns (bool) {
        try IDualCurrencyERC165(account).supportsInterface(interfaceId) returns (bool supported) {
            return supported;
        } catch {
            return false;
        }
    }

    function _safeTransferFrom(address token, address from, address to, uint256 amount) private {
        if (amount == 0) return;
        (bool success, bytes memory result) = token.call(
            abi.encodeCall(IDualCurrencyERC20.transferFrom, (from, to, amount))
        );
        require(success && (result.length == 0 || abi.decode(result, (bool))), "ERC20 transfer failed");
    }

    function _sendNative(address recipient, uint256 amount) private {
        if (amount == 0) return;
        (bool success, ) = payable(recipient).call{value: amount}("");
        require(success, "native transfer failed");
    }
}
