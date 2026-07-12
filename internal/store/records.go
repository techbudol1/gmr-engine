package store

import "github.com/neo4j/neo4j-go-driver/v5/neo4j"

func accountFromRecord(record *neo4j.Record) Account {
	return Account{
		ID:        stringValue(record, "id"),
		Email:     stringValue(record, "email"),
		Name:      stringValue(record, "name"),
		Role:      stringValue(record, "role"),
		Status:    stringValue(record, "status"),
		Username:  stringValue(record, "username"),
		CreatedAt: stringValue(record, "createdAt"),
		UpdatedAt: stringValue(record, "updatedAt"),
	}
}

func accountSessionFromRecord(record *neo4j.Record) AccountSession {
	return AccountSession{
		ID:        stringValue(record, "id"),
		AccountID: stringValue(record, "accountId"),
		TokenHash: stringValue(record, "tokenHash"),
		ExpiresAt: stringValue(record, "expiresAt"),
		CreatedAt: stringValue(record, "createdAt"),
	}
}

func appFromRecord(record *neo4j.Record) App {
	return App{
		ID:               stringValue(record, "id"),
		Name:             stringValue(record, "name"),
		Environment:      stringValue(record, "environment"),
		Status:           stringValue(record, "status"),
		AllowedChains:    int64SliceValue(record, "allowedChains"),
		AllowedContracts: stringSliceValue(record, "allowedContracts"),
		GasFreeEnabled:   boolValue(record, "gasFreeEnabled"),
		TradingFeeBps:    intValueOr(record, "tradingFeeBps", 50),
		RateLimitPerMin:  intValue(record, "rateLimitPerMinute"),
		WebhookURL:       stringValue(record, "webhookUrl"),
		CreatedAt:        stringValue(record, "createdAt"),
		UpdatedAt:        stringValue(record, "updatedAt"),
	}
}

func appFromPrefixedRecord(record *neo4j.Record, prefix string) App {
	return App{
		ID:               stringValue(record, prefix+"Id"),
		Name:             stringValue(record, prefix+"Name"),
		Environment:      stringValue(record, prefix+"Environment"),
		Status:           stringValue(record, prefix+"Status"),
		AllowedChains:    int64SliceValue(record, prefix+"AllowedChains"),
		AllowedContracts: stringSliceValue(record, prefix+"AllowedContracts"),
		GasFreeEnabled:   boolValue(record, prefix+"GasFreeEnabled"),
		TradingFeeBps:    intValueOr(record, prefix+"TradingFeeBps", 50),
		RateLimitPerMin:  intValue(record, prefix+"RateLimitPerMinute"),
		WebhookURL:       stringValue(record, prefix+"WebhookUrl"),
		CreatedAt:        stringValue(record, prefix+"CreatedAt"),
		UpdatedAt:        stringValue(record, prefix+"UpdatedAt"),
	}
}

func importedContractFromRecord(record *neo4j.Record) ImportedContract {
	return ImportedContract{
		ID:              stringValue(record, "id"),
		AppID:           stringValue(record, "appId"),
		Name:            stringValue(record, "name"),
		ContractType:    stringValue(record, "contractType"),
		ContractAddress: stringValue(record, "contractAddress"),
		ChainID:         intValue(record, "chainId"),
		ABI:             stringValue(record, "abi"),
		Description:     stringValue(record, "description"),
		Status:          stringValue(record, "status"),
		CreatedAt:       stringValue(record, "createdAt"),
		UpdatedAt:       stringValue(record, "updatedAt"),
	}
}

func apiKeyFromRecord(record *neo4j.Record) APIKey {
	return apiKeyFromRecordWithAppID(record, "appId")
}

func projectWalletFromRecord(record *neo4j.Record) ProjectWallet {
	return ProjectWallet{
		ID:             stringValue(record, "id"),
		AppID:          stringValue(record, "appId"),
		Address:        stringValue(record, "address"),
		WalletType:     stringValue(record, "walletType"),
		Status:         stringValue(record, "status"),
		IsDefaultAdmin: boolValue(record, "isDefaultAdmin"),
		CreatedAt:      stringValue(record, "createdAt"),
		UpdatedAt:      stringValue(record, "updatedAt"),
	}
}

func projectWalletSecretFromRecord(record *neo4j.Record) ProjectWalletSecret {
	return ProjectWalletSecret{
		ProjectWallet:       projectWalletFromRecord(record),
		EncryptedPrivateKey: stringValue(record, "encryptedPrivateKey"),
	}
}

func userWalletFromRecord(record *neo4j.Record) UserWallet {
	return UserWallet{
		ID:            stringValue(record, "id"),
		AppID:         stringValue(record, "appId"),
		UserID:        stringValue(record, "userId"),
		Address:       stringValue(record, "address"),
		AuthProvider:  stringValue(record, "authProvider"),
		Email:         stringValue(record, "email"),
		Status:        stringValue(record, "status"),
		Metadata:      stringValue(record, "metadata"),
		WalletCustody: stringValue(record, "walletCustody"),
		WalletType:    stringValue(record, "walletType"),
		CreatedAt:     stringValue(record, "createdAt"),
		UpdatedAt:     stringValue(record, "updatedAt"),
		LastSeenAt:    stringValue(record, "lastSeenAt"),
	}
}

func apiKeyFromRecordWithAppID(record *neo4j.Record, appIDKey string) APIKey {
	return APIKey{
		ID:             stringValue(record, "id"),
		AppID:          stringValue(record, appIDKey),
		AppName:        stringValue(record, "appName"),
		Name:           stringValue(record, "name"),
		Prefix:         stringValue(record, "prefix"),
		Scopes:         stringSliceValue(record, "scopes"),
		AllowedOrigins: stringSliceValue(record, "allowedOrigins"),
		AllowedIPs:     stringSliceValue(record, "allowedIPs"),
		Status:         stringValue(record, "status"),
		ExpiresAt:      stringValue(record, "expiresAt"),
		LastUsedAt:     stringValue(record, "lastUsedAt"),
		CreatedAt:      stringValue(record, "createdAt"),
		RevokedAt:      stringValue(record, "revokedAt"),
	}
}

func usageFromRecord(record *neo4j.Record) APIUsage {
	return APIUsage{
		ID:         stringValue(record, "id"),
		AppID:      stringValue(record, "appId"),
		KeyID:      stringValue(record, "keyId"),
		Method:     stringValue(record, "method"),
		Path:       stringValue(record, "path"),
		StatusCode: intValue(record, "statusCode"),
		IPAddress:  stringValue(record, "ipAddress"),
		UserAgent:  stringValue(record, "userAgent"),
		CreatedAt:  stringValue(record, "createdAt"),
	}
}

func transactionFromRecord(record *neo4j.Record) Transaction {
	return Transaction{
		ID:              stringValue(record, "id"),
		AppID:           stringValue(record, "appId"),
		KeyID:           stringValue(record, "keyId"),
		IdempotencyKey:  stringValue(record, "idempotencyKey"),
		ChainID:         intValue(record, "chainId"),
		WalletAddress:   stringValue(record, "walletAddress"),
		ContractAddress: stringValue(record, "contractAddress"),
		Kind:            stringValue(record, "kind"),
		Method:          stringValue(record, "method"),
		Args:            stringSliceValue(record, "args"),
		Metadata:        stringValue(record, "metadata"),
		Value:           stringValue(record, "value"),
		Status:          stringValue(record, "status"),
		AttemptCount:    intValue(record, "attemptCount"),
		AttemptLog:      stringSliceValue(record, "attemptLog"),
		Error:           stringValue(record, "error"),
		TransactionHash: stringValue(record, "transactionHash"),
		CreatedAt:       stringValue(record, "createdAt"),
		UpdatedAt:       stringValue(record, "updatedAt"),
		QueuedAt:        stringValue(record, "queuedAt"),
		StartedAt:       stringValue(record, "startedAt"),
		SubmittedAt:     stringValue(record, "submittedAt"),
		ConfirmedAt:     stringValue(record, "confirmedAt"),
		FailedAt:        stringValue(record, "failedAt"),
	}
}

func erc20DeploymentFromRecord(record *neo4j.Record) ERC20Deployment {
	return ERC20Deployment{
		ID:              stringValue(record, "id"),
		AppID:           stringValue(record, "appId"),
		KeyID:           stringValue(record, "keyId"),
		Name:            stringValue(record, "name"),
		Symbol:          stringValue(record, "symbol"),
		Decimals:        intValue(record, "decimals"),
		InitialSupply:   stringValue(record, "initialSupply"),
		OwnerAddress:    stringValue(record, "ownerAddress"),
		ChainID:         intValue(record, "chainId"),
		Description:     stringValue(record, "description"),
		ImageURL:        stringValue(record, "imageUrl"),
		SocialURLs:      stringSliceValue(record, "socialUrls"),
		Status:          stringValue(record, "status"),
		ContractAddress: stringValue(record, "contractAddress"),
		TransactionHash: stringValue(record, "transactionHash"),
		Error:           stringValue(record, "error"),
		SourceName:      stringValue(record, "sourceName"),
		SourceCode:      stringValue(record, "sourceCode"),
		CreatedAt:       stringValue(record, "createdAt"),
		UpdatedAt:       stringValue(record, "updatedAt"),
		QueuedAt:        stringValue(record, "queuedAt"),
	}
}

func erc1155EditionDeploymentFromRecord(record *neo4j.Record) ERC1155EditionDeployment {
	return ERC1155EditionDeployment{
		ID:               stringValue(record, "id"),
		AppID:            stringValue(record, "appId"),
		KeyID:            stringValue(record, "keyId"),
		Name:             stringValue(record, "name"),
		Symbol:           stringValue(record, "symbol"),
		BaseURI:          stringValue(record, "baseUri"),
		ContractURI:      stringValue(record, "contractUri"),
		OwnerAddress:     stringValue(record, "ownerAddress"),
		RecipientAddress: stringValue(record, "recipientAddress"),
		InitialTokenID:   stringValue(record, "initialTokenId"),
		InitialSupply:    stringValue(record, "initialSupply"),
		ChainID:          intValue(record, "chainId"),
		Description:      stringValue(record, "description"),
		ImageURL:         stringValue(record, "imageUrl"),
		SocialURLs:       stringSliceValue(record, "socialUrls"),
		Status:           stringValue(record, "status"),
		ContractAddress:  stringValue(record, "contractAddress"),
		TransactionHash:  stringValue(record, "transactionHash"),
		Error:            stringValue(record, "error"),
		SourceName:       stringValue(record, "sourceName"),
		SourceCode:       stringValue(record, "sourceCode"),
		ABI:              stringValue(record, "abi"),
		CreatedAt:        stringValue(record, "createdAt"),
		UpdatedAt:        stringValue(record, "updatedAt"),
		QueuedAt:         stringValue(record, "queuedAt"),
	}
}

func escrowDeploymentFromRecord(record *neo4j.Record) EscrowDeployment {
	return EscrowDeployment{
		ID:              stringValue(record, "id"),
		AppID:           stringValue(record, "appId"),
		KeyID:           stringValue(record, "keyId"),
		Name:            stringValue(record, "name"),
		TokenAddress:    stringValue(record, "tokenAddress"),
		OwnerAddress:    stringValue(record, "ownerAddress"),
		TreasuryAddress: stringValue(record, "treasuryAddress"),
		FeeBps:          intValue(record, "feeBps"),
		ChainID:         intValue(record, "chainId"),
		Description:     stringValue(record, "description"),
		Status:          stringValue(record, "status"),
		ContractAddress: stringValue(record, "contractAddress"),
		TransactionHash: stringValue(record, "transactionHash"),
		Error:           stringValue(record, "error"),
		SourceName:      stringValue(record, "sourceName"),
		SourceCode:      stringValue(record, "sourceCode"),
		ABI:             stringValue(record, "abi"),
		CreatedAt:       stringValue(record, "createdAt"),
		UpdatedAt:       stringValue(record, "updatedAt"),
		QueuedAt:        stringValue(record, "queuedAt"),
	}
}

func privateClaimRegistryDeploymentFromRecord(record *neo4j.Record) PrivateClaimRegistryDeployment {
	return PrivateClaimRegistryDeployment{
		ID:              stringValue(record, "id"),
		AppID:           stringValue(record, "appId"),
		KeyID:           stringValue(record, "keyId"),
		Name:            stringValue(record, "name"),
		OwnerAddress:    stringValue(record, "ownerAddress"),
		VerifierAddress: stringValue(record, "verifierAddress"),
		ChainID:         intValue(record, "chainId"),
		Description:     stringValue(record, "description"),
		Status:          stringValue(record, "status"),
		ContractAddress: stringValue(record, "contractAddress"),
		TransactionHash: stringValue(record, "transactionHash"),
		Error:           stringValue(record, "error"),
		SourceName:      stringValue(record, "sourceName"),
		SourceCode:      stringValue(record, "sourceCode"),
		ABI:             stringValue(record, "abi"),
		CreatedAt:       stringValue(record, "createdAt"),
		UpdatedAt:       stringValue(record, "updatedAt"),
		QueuedAt:        stringValue(record, "queuedAt"),
	}
}

func shieldedPayoutPoolDeploymentFromRecord(record *neo4j.Record) ShieldedPayoutPoolDeployment {
	return ShieldedPayoutPoolDeployment{
		ID:              stringValue(record, "id"),
		AppID:           stringValue(record, "appId"),
		KeyID:           stringValue(record, "keyId"),
		Name:            stringValue(record, "name"),
		TokenAddress:    stringValue(record, "tokenAddress"),
		OwnerAddress:    stringValue(record, "ownerAddress"),
		VerifierAddress: stringValue(record, "verifierAddress"),
		Denomination:    stringValue(record, "denomination"),
		ChainID:         intValue(record, "chainId"),
		Description:     stringValue(record, "description"),
		Status:          stringValue(record, "status"),
		ContractAddress: stringValue(record, "contractAddress"),
		TransactionHash: stringValue(record, "transactionHash"),
		Error:           stringValue(record, "error"),
		SourceName:      stringValue(record, "sourceName"),
		SourceCode:      stringValue(record, "sourceCode"),
		ABI:             stringValue(record, "abi"),
		CreatedAt:       stringValue(record, "createdAt"),
		UpdatedAt:       stringValue(record, "updatedAt"),
		QueuedAt:        stringValue(record, "queuedAt"),
	}
}

func shieldedWithdrawalVerifierDeploymentFromRecord(record *neo4j.Record) ShieldedWithdrawalVerifierDeployment {
	return ShieldedWithdrawalVerifierDeployment{
		ID:              stringValue(record, "id"),
		AppID:           stringValue(record, "appId"),
		KeyID:           stringValue(record, "keyId"),
		Name:            stringValue(record, "name"),
		ChainID:         intValue(record, "chainId"),
		Description:     stringValue(record, "description"),
		Status:          stringValue(record, "status"),
		ContractAddress: stringValue(record, "contractAddress"),
		TransactionHash: stringValue(record, "transactionHash"),
		Error:           stringValue(record, "error"),
		SourceName:      stringValue(record, "sourceName"),
		SourceCode:      stringValue(record, "sourceCode"),
		ABI:             stringValue(record, "abi"),
		CreatedAt:       stringValue(record, "createdAt"),
		UpdatedAt:       stringValue(record, "updatedAt"),
		QueuedAt:        stringValue(record, "queuedAt"),
	}
}

func accountAbstractionDeploymentFromRecord(record *neo4j.Record) AccountAbstractionDeployment {
	return AccountAbstractionDeployment{
		ID:                        stringValue(record, "id"),
		AppID:                     stringValue(record, "appId"),
		KeyID:                     stringValue(record, "keyId"),
		Name:                      stringValue(record, "name"),
		ChainID:                   intValue(record, "chainId"),
		Description:               stringValue(record, "description"),
		Status:                    stringValue(record, "status"),
		ContractAddress:           stringValue(record, "contractAddress"),
		TransactionHash:           stringValue(record, "transactionHash"),
		EntryPointAddress:         stringValue(record, "entryPointAddress"),
		EntryPointTransactionHash: stringValue(record, "entryPointTransactionHash"),
		FactoryAddress:            stringValue(record, "factoryAddress"),
		FactoryTransactionHash:    stringValue(record, "factoryTransactionHash"),
		BundlerURL:                stringValue(record, "bundlerUrl"),
		Version:                   stringValue(record, "version"),
		Error:                     stringValue(record, "error"),
		CreatedAt:                 stringValue(record, "createdAt"),
		UpdatedAt:                 stringValue(record, "updatedAt"),
		QueuedAt:                  stringValue(record, "queuedAt"),
	}
}

func zkProofSubmissionFromRecord(record *neo4j.Record) ZKProofSubmission {
	return ZKProofSubmission{
		ID:                stringValue(record, "id"),
		AppID:             stringValue(record, "appId"),
		KeyID:             stringValue(record, "keyId"),
		ProofSystem:       stringValue(record, "proofSystem"),
		DomainID:          intValue(record, "domainId"),
		VK:                stringValue(record, "vk"),
		Proof:             stringValue(record, "proof"),
		PublicSignals:     stringValue(record, "publicSignals"),
		Context:           stringValue(record, "context"),
		Status:            stringValue(record, "status"),
		ZKVerifyNetwork:   stringValue(record, "zkVerifyNetwork"),
		AccountAddress:    stringValue(record, "accountAddress"),
		TransactionResult: stringValue(record, "transactionResult"),
		Error:             stringValue(record, "error"),
		CreatedAt:         stringValue(record, "createdAt"),
		UpdatedAt:         stringValue(record, "updatedAt"),
		SubmittedAt:       stringValue(record, "submittedAt"),
		FinalizedAt:       stringValue(record, "finalizedAt"),
		FailedAt:          stringValue(record, "failedAt"),
	}
}

func lockFromRecord(record *neo4j.Record) WalletLock {
	return WalletLock{
		AppID:         stringValue(record, "appId"),
		WalletAddress: stringValue(record, "walletAddress"),
		TransactionID: stringValue(record, "transactionId"),
		LockedUntil:   stringValue(record, "lockedUntil"),
		UpdatedAt:     stringValue(record, "updatedAt"),
	}
}

func stringValue(record *neo4j.Record, key string) string {
	value, ok := record.Get(key)
	if !ok || value == nil {
		return ""
	}
	if str, ok := value.(string); ok {
		return str
	}
	return ""
}

func intValue(record *neo4j.Record, key string) int64 {
	value, ok := record.Get(key)
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	default:
		return 0
	}
}

func intValueOr(record *neo4j.Record, key string, fallback int64) int64 {
	value, ok := record.Get(key)
	if !ok || value == nil {
		return fallback
	}
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	default:
		return fallback
	}
}

func boolValue(record *neo4j.Record, key string) bool {
	value, ok := record.Get(key)
	if !ok || value == nil {
		return false
	}
	typed, ok := value.(bool)
	return ok && typed
}

func stringSliceValue(record *neo4j.Record, key string) []string {
	value, ok := record.Get(key)
	if !ok || value == nil {
		return []string{}
	}
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	default:
		return []string{}
	}
}

func int64SliceValue(record *neo4j.Record, key string) []int64 {
	value, ok := record.Get(key)
	if !ok || value == nil {
		return []int64{}
	}
	switch typed := value.(type) {
	case []int64:
		return typed
	case []any:
		result := make([]int64, 0, len(typed))
		for _, item := range typed {
			switch number := item.(type) {
			case int64:
				result = append(result, number)
			case int:
				result = append(result, int64(number))
			}
		}
		return result
	default:
		return []int64{}
	}
}
