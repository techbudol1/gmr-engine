package store

import "context"

type Account struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	Username  string `json:"username"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type AccountSession struct {
	ID        string `json:"id"`
	AccountID string `json:"accountId"`
	TokenHash string `json:"-"`
	ExpiresAt string `json:"expiresAt"`
	CreatedAt string `json:"createdAt"`
}

type App struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Environment      string   `json:"environment"`
	Status           string   `json:"status"`
	AllowedChains    []int64  `json:"allowedChains"`
	AllowedContracts []string `json:"allowedContracts"`
	GasFreeEnabled   bool     `json:"gasFreeEnabled"`
	RateLimitPerMin  int64    `json:"rateLimitPerMinute"`
	WebhookURL       string   `json:"webhookUrl"`
	CreatedAt        string   `json:"createdAt"`
	UpdatedAt        string   `json:"updatedAt"`
}

type AppInput struct {
	Name             string   `json:"name"`
	Environment      string   `json:"environment"`
	AllowedChains    []int64  `json:"allowedChains"`
	AllowedContracts []string `json:"allowedContracts"`
	GasFreeEnabled   bool     `json:"gasFreeEnabled"`
	RateLimitPerMin  int64    `json:"rateLimitPerMinute"`
	WebhookURL       string   `json:"webhookUrl"`
}

type ImportedContractInput struct {
	Name            string `json:"name"`
	ContractType    string `json:"contractType"`
	ContractAddress string `json:"contractAddress"`
	ChainID         int64  `json:"chainId"`
	ABI             string `json:"abi"`
	Description     string `json:"description"`
}

type ImportedContract struct {
	ID              string `json:"id"`
	AppID           string `json:"appId"`
	Name            string `json:"name"`
	ContractType    string `json:"contractType"`
	ContractAddress string `json:"contractAddress"`
	ChainID         int64  `json:"chainId"`
	ABI             string `json:"abi"`
	Description     string `json:"description"`
	Status          string `json:"status"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
}

type ProjectWalletInput struct {
	Address             string
	EncryptedPrivateKey string
	IsDefaultAdmin      bool
	WalletType          string
}

type ProjectWallet struct {
	ID             string `json:"id"`
	AppID          string `json:"appId"`
	Address        string `json:"address"`
	WalletType     string `json:"walletType"`
	Status         string `json:"status"`
	IsDefaultAdmin bool   `json:"isDefaultAdmin"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

type UserWalletInput struct {
	UserID         string
	Address        string
	AuthProvider   string
	Email          string
	Metadata       string
	VaultWalletRef string
	WalletCustody  string
	WalletType     string
}

type UserWallet struct {
	ID            string `json:"id"`
	AppID         string `json:"appId"`
	UserID        string `json:"userId"`
	Address       string `json:"address"`
	AuthProvider  string `json:"authProvider"`
	Email         string `json:"email"`
	Status        string `json:"status"`
	Metadata      string `json:"metadata"`
	WalletCustody string `json:"walletCustody"`
	WalletType    string `json:"walletType"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
	LastSeenAt    string `json:"lastSeenAt"`
}

type UserWalletSecret struct {
	UserWallet
	VaultWalletRef string
}

type ProjectWalletSecret struct {
	ProjectWallet
	EncryptedPrivateKey string `json:"-"`
}

type APIKey struct {
	ID             string   `json:"id"`
	AppID          string   `json:"appId"`
	AppName        string   `json:"appName,omitempty"`
	Name           string   `json:"name"`
	Prefix         string   `json:"prefix"`
	Scopes         []string `json:"scopes"`
	AllowedOrigins []string `json:"allowedOrigins"`
	AllowedIPs     []string `json:"allowedIps"`
	Status         string   `json:"status"`
	ExpiresAt      string   `json:"expiresAt,omitempty"`
	LastUsedAt     string   `json:"lastUsedAt,omitempty"`
	CreatedAt      string   `json:"createdAt"`
	RevokedAt      string   `json:"revokedAt,omitempty"`
}

type APIKeyInput struct {
	Name           string   `json:"name"`
	Scopes         []string `json:"scopes"`
	AllowedOrigins []string `json:"allowedOrigins"`
	AllowedIPs     []string `json:"allowedIps"`
	ExpiresAt      string   `json:"expiresAt"`
}

type APIKeyVerification struct {
	App App    `json:"app"`
	Key APIKey `json:"key"`
}

type APIUsageInput struct {
	AppID      string
	KeyID      string
	Method     string
	Path       string
	StatusCode int
	IPAddress  string
	UserAgent  string
}

type APIUsage struct {
	ID         string `json:"id"`
	AppID      string `json:"appId"`
	KeyID      string `json:"keyId"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	StatusCode int64  `json:"statusCode"`
	IPAddress  string `json:"ipAddress"`
	UserAgent  string `json:"userAgent"`
	CreatedAt  string `json:"createdAt"`
}

type TransactionInput struct {
	AppID           string         `json:"-"`
	KeyID           string         `json:"-"`
	IdempotencyKey  string         `json:"idempotencyKey"`
	ChainID         int64          `json:"chainId"`
	WalletAddress   string         `json:"walletAddress"`
	ContractAddress string         `json:"contractAddress"`
	Kind            string         `json:"kind"`
	Method          string         `json:"method"`
	Args            []string       `json:"args"`
	Value           string         `json:"value"`
	Metadata        map[string]any `json:"metadata"`
}

type Transaction struct {
	ID              string   `json:"id"`
	AppID           string   `json:"appId"`
	KeyID           string   `json:"keyId"`
	IdempotencyKey  string   `json:"idempotencyKey,omitempty"`
	ChainID         int64    `json:"chainId"`
	WalletAddress   string   `json:"walletAddress"`
	ContractAddress string   `json:"contractAddress"`
	Kind            string   `json:"kind"`
	Method          string   `json:"method"`
	Args            []string `json:"args"`
	Metadata        string   `json:"metadata,omitempty"`
	Value           string   `json:"value"`
	Status          string   `json:"status"`
	AttemptCount    int64    `json:"attemptCount"`
	AttemptLog      []string `json:"attemptLog"`
	Error           string   `json:"error,omitempty"`
	TransactionHash string   `json:"transactionHash,omitempty"`
	CreatedAt       string   `json:"createdAt"`
	UpdatedAt       string   `json:"updatedAt"`
	QueuedAt        string   `json:"queuedAt"`
	StartedAt       string   `json:"startedAt,omitempty"`
	SubmittedAt     string   `json:"submittedAt,omitempty"`
	ConfirmedAt     string   `json:"confirmedAt,omitempty"`
	FailedAt        string   `json:"failedAt,omitempty"`
}

type ERC20DeploymentInput struct {
	AppID         string   `json:"-"`
	KeyID         string   `json:"-"`
	Name          string   `json:"name"`
	Symbol        string   `json:"symbol"`
	Decimals      int64    `json:"decimals"`
	InitialSupply string   `json:"initialSupply"`
	OwnerAddress  string   `json:"ownerAddress"`
	ChainID       int64    `json:"chainId"`
	Description   string   `json:"description"`
	ImageURL      string   `json:"imageUrl"`
	SocialURLs    []string `json:"socialUrls"`
}

type ERC20Deployment struct {
	ID              string   `json:"id"`
	AppID           string   `json:"appId"`
	KeyID           string   `json:"keyId,omitempty"`
	Name            string   `json:"name"`
	Symbol          string   `json:"symbol"`
	Decimals        int64    `json:"decimals"`
	InitialSupply   string   `json:"initialSupply"`
	OwnerAddress    string   `json:"ownerAddress"`
	ChainID         int64    `json:"chainId"`
	Description     string   `json:"description"`
	ImageURL        string   `json:"imageUrl"`
	SocialURLs      []string `json:"socialUrls"`
	Status          string   `json:"status"`
	ContractAddress string   `json:"contractAddress,omitempty"`
	TransactionHash string   `json:"transactionHash,omitempty"`
	Error           string   `json:"error,omitempty"`
	SourceName      string   `json:"sourceName"`
	SourceCode      string   `json:"sourceCode"`
	CreatedAt       string   `json:"createdAt"`
	UpdatedAt       string   `json:"updatedAt"`
	QueuedAt        string   `json:"queuedAt"`
}

type ERC1155EditionDeploymentInput struct {
	AppID            string   `json:"-"`
	KeyID            string   `json:"-"`
	Name             string   `json:"name"`
	Symbol           string   `json:"symbol"`
	BaseURI          string   `json:"baseUri"`
	ContractURI      string   `json:"contractUri"`
	OwnerAddress     string   `json:"ownerAddress"`
	RecipientAddress string   `json:"recipientAddress"`
	InitialTokenID   string   `json:"initialTokenId"`
	InitialSupply    string   `json:"initialSupply"`
	ChainID          int64    `json:"chainId"`
	Description      string   `json:"description"`
	ImageURL         string   `json:"imageUrl"`
	SocialURLs       []string `json:"socialUrls"`
}

type ERC1155EditionDeployment struct {
	ID               string   `json:"id"`
	AppID            string   `json:"appId"`
	KeyID            string   `json:"keyId,omitempty"`
	Name             string   `json:"name"`
	Symbol           string   `json:"symbol"`
	BaseURI          string   `json:"baseUri"`
	ContractURI      string   `json:"contractUri"`
	OwnerAddress     string   `json:"ownerAddress"`
	RecipientAddress string   `json:"recipientAddress"`
	InitialTokenID   string   `json:"initialTokenId"`
	InitialSupply    string   `json:"initialSupply"`
	ChainID          int64    `json:"chainId"`
	Description      string   `json:"description"`
	ImageURL         string   `json:"imageUrl"`
	SocialURLs       []string `json:"socialUrls"`
	Status           string   `json:"status"`
	ContractAddress  string   `json:"contractAddress,omitempty"`
	TransactionHash  string   `json:"transactionHash,omitempty"`
	Error            string   `json:"error,omitempty"`
	SourceName       string   `json:"sourceName"`
	SourceCode       string   `json:"sourceCode"`
	ABI              string   `json:"abi"`
	CreatedAt        string   `json:"createdAt"`
	UpdatedAt        string   `json:"updatedAt"`
	QueuedAt         string   `json:"queuedAt"`
}

type EscrowDeploymentInput struct {
	AppID           string `json:"-"`
	KeyID           string `json:"-"`
	Name            string `json:"name"`
	TokenAddress    string `json:"tokenAddress"`
	OwnerAddress    string `json:"ownerAddress"`
	TreasuryAddress string `json:"treasuryAddress"`
	FeeBps          int64  `json:"feeBps"`
	ChainID         int64  `json:"chainId"`
	Description     string `json:"description"`
}

type EscrowDeployment struct {
	ID              string `json:"id"`
	AppID           string `json:"appId"`
	KeyID           string `json:"keyId,omitempty"`
	Name            string `json:"name"`
	TokenAddress    string `json:"tokenAddress"`
	OwnerAddress    string `json:"ownerAddress"`
	TreasuryAddress string `json:"treasuryAddress"`
	FeeBps          int64  `json:"feeBps"`
	ChainID         int64  `json:"chainId"`
	Description     string `json:"description"`
	Status          string `json:"status"`
	ContractAddress string `json:"contractAddress,omitempty"`
	TransactionHash string `json:"transactionHash,omitempty"`
	Error           string `json:"error,omitempty"`
	SourceName      string `json:"sourceName"`
	SourceCode      string `json:"sourceCode"`
	ABI             string `json:"abi"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
	QueuedAt        string `json:"queuedAt"`
}

type PrivateClaimRegistryDeploymentInput struct {
	AppID           string `json:"-"`
	KeyID           string `json:"-"`
	Name            string `json:"name"`
	OwnerAddress    string `json:"ownerAddress"`
	VerifierAddress string `json:"verifierAddress"`
	ChainID         int64  `json:"chainId"`
	Description     string `json:"description"`
}

type PrivateClaimRegistryDeployment struct {
	ID              string `json:"id"`
	AppID           string `json:"appId"`
	KeyID           string `json:"keyId,omitempty"`
	Name            string `json:"name"`
	OwnerAddress    string `json:"ownerAddress"`
	VerifierAddress string `json:"verifierAddress"`
	ChainID         int64  `json:"chainId"`
	Description     string `json:"description"`
	Status          string `json:"status"`
	ContractAddress string `json:"contractAddress,omitempty"`
	TransactionHash string `json:"transactionHash,omitempty"`
	Error           string `json:"error,omitempty"`
	SourceName      string `json:"sourceName"`
	SourceCode      string `json:"sourceCode"`
	ABI             string `json:"abi"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
	QueuedAt        string `json:"queuedAt"`
}

type ShieldedPayoutPoolDeploymentInput struct {
	AppID           string `json:"-"`
	KeyID           string `json:"-"`
	Name            string `json:"name"`
	TokenAddress    string `json:"tokenAddress"`
	OwnerAddress    string `json:"ownerAddress"`
	VerifierAddress string `json:"verifierAddress"`
	Denomination    string `json:"denomination"`
	ChainID         int64  `json:"chainId"`
	Description     string `json:"description"`
}

type ShieldedPayoutPoolDeployment struct {
	ID              string `json:"id"`
	AppID           string `json:"appId"`
	KeyID           string `json:"keyId,omitempty"`
	Name            string `json:"name"`
	TokenAddress    string `json:"tokenAddress"`
	OwnerAddress    string `json:"ownerAddress"`
	VerifierAddress string `json:"verifierAddress"`
	Denomination    string `json:"denomination"`
	ChainID         int64  `json:"chainId"`
	Description     string `json:"description"`
	Status          string `json:"status"`
	ContractAddress string `json:"contractAddress,omitempty"`
	TransactionHash string `json:"transactionHash,omitempty"`
	Error           string `json:"error,omitempty"`
	SourceName      string `json:"sourceName"`
	SourceCode      string `json:"sourceCode"`
	ABI             string `json:"abi"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
	QueuedAt        string `json:"queuedAt"`
}

type ShieldedWithdrawalVerifierDeploymentInput struct {
	AppID       string `json:"-"`
	KeyID       string `json:"-"`
	Name        string `json:"name"`
	ChainID     int64  `json:"chainId"`
	Description string `json:"description"`
}

type ShieldedWithdrawalVerifierDeployment struct {
	ID              string `json:"id"`
	AppID           string `json:"appId"`
	KeyID           string `json:"keyId,omitempty"`
	Name            string `json:"name"`
	ChainID         int64  `json:"chainId"`
	Description     string `json:"description"`
	Status          string `json:"status"`
	ContractAddress string `json:"contractAddress,omitempty"`
	TransactionHash string `json:"transactionHash,omitempty"`
	Error           string `json:"error,omitempty"`
	SourceName      string `json:"sourceName"`
	SourceCode      string `json:"sourceCode"`
	ABI             string `json:"abi"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
	QueuedAt        string `json:"queuedAt"`
}

type AccountAbstractionDeploymentInput struct {
	AppID             string `json:"-"`
	KeyID             string `json:"-"`
	Name              string `json:"name"`
	ChainID           int64  `json:"chainId"`
	EntryPointAddress string `json:"entryPointAddress"`
	BundlerURL        string `json:"bundlerUrl"`
	Description       string `json:"description"`
}

type AccountAbstractionDeployment struct {
	ID                        string `json:"id"`
	AppID                     string `json:"appId"`
	KeyID                     string `json:"keyId,omitempty"`
	Name                      string `json:"name"`
	ChainID                   int64  `json:"chainId"`
	Description               string `json:"description"`
	Status                    string `json:"status"`
	ContractAddress           string `json:"contractAddress,omitempty"`
	TransactionHash           string `json:"transactionHash,omitempty"`
	EntryPointAddress         string `json:"entryPointAddress,omitempty"`
	EntryPointTransactionHash string `json:"entryPointTransactionHash,omitempty"`
	FactoryAddress            string `json:"factoryAddress,omitempty"`
	FactoryTransactionHash    string `json:"factoryTransactionHash,omitempty"`
	BundlerURL                string `json:"bundlerUrl,omitempty"`
	Version                   string `json:"version"`
	Error                     string `json:"error,omitempty"`
	CreatedAt                 string `json:"createdAt"`
	UpdatedAt                 string `json:"updatedAt"`
	QueuedAt                  string `json:"queuedAt"`
}

type ZKProofSubmissionInput struct {
	AppID         string `json:"-"`
	KeyID         string `json:"-"`
	ProofSystem   string `json:"proofSystem"`
	DomainID      int64  `json:"domainId"`
	VK            string `json:"vk"`
	Proof         string `json:"proof"`
	PublicSignals string `json:"publicSignals"`
	Context       string `json:"context"`
}

type ZKProofSubmission struct {
	ID                string `json:"id"`
	AppID             string `json:"appId"`
	KeyID             string `json:"keyId,omitempty"`
	ProofSystem       string `json:"proofSystem"`
	DomainID          int64  `json:"domainId"`
	VK                string `json:"vk"`
	Proof             string `json:"proof"`
	PublicSignals     string `json:"publicSignals"`
	Context           string `json:"context"`
	Status            string `json:"status"`
	ZKVerifyNetwork   string `json:"zkVerifyNetwork"`
	AccountAddress    string `json:"accountAddress"`
	TransactionResult string `json:"transactionResult"`
	Error             string `json:"error,omitempty"`
	CreatedAt         string `json:"createdAt"`
	UpdatedAt         string `json:"updatedAt"`
	SubmittedAt       string `json:"submittedAt,omitempty"`
	FinalizedAt       string `json:"finalizedAt,omitempty"`
	FailedAt          string `json:"failedAt,omitempty"`
}

type WalletLock struct {
	WalletAddress string `json:"walletAddress"`
	AppID         string `json:"appId"`
	TransactionID string `json:"transactionId"`
	LockedUntil   string `json:"lockedUntil"`
	UpdatedAt     string `json:"updatedAt"`
}

type Store interface {
	SeedOwnerAccount(ctx context.Context, username string, email string, passwordHash string, name string) (Account, error)
	VerifyAccountPassword(ctx context.Context, login string, passwordHash string) (Account, bool, error)
	CreateAccountSession(ctx context.Context, accountID string, tokenHash string, expiresAt string) (AccountSession, error)
	GetAccountBySession(ctx context.Context, tokenHash string) (Account, bool, error)
	DeleteAccountSession(ctx context.Context, tokenHash string) error
	CreateApp(ctx context.Context, input AppInput) (App, error)
	CreateAccountApp(ctx context.Context, accountID string, input AppInput) (App, error)
	ListApps(ctx context.Context) ([]App, error)
	ListAccountApps(ctx context.Context, accountID string) ([]App, error)
	GetApp(ctx context.Context, id string) (App, bool, error)
	GetAccountApp(ctx context.Context, accountID string, id string) (App, bool, error)
	UpdateAccountApp(ctx context.Context, accountID string, id string, input AppInput) (App, error)
	ArchiveAccountApp(ctx context.Context, accountID string, id string) (App, error)
	CreateProjectWallet(ctx context.Context, appID string, input ProjectWalletInput) (ProjectWallet, error)
	ListProjectWallets(ctx context.Context, appID string, limit int64) ([]ProjectWallet, error)
	UpsertUserWallet(ctx context.Context, appID string, input UserWalletInput) (UserWallet, error)
	GetActiveManagedUserWallet(ctx context.Context, appID string, authProvider string, userID string) (UserWallet, bool, error)
	GetActiveManagedUserWalletSecretByAddress(ctx context.Context, appID string, address string) (UserWalletSecret, bool, error)
	ListUserWallets(ctx context.Context, appID string, limit int64) ([]UserWallet, error)
	DeleteUserWallet(ctx context.Context, appID string, id string) (UserWallet, error)
	GetProjectDefaultAdminWallet(ctx context.Context, appID string) (ProjectWallet, bool, error)
	GetProjectDefaultAdminWalletSecret(ctx context.Context, appID string) (ProjectWalletSecret, bool, error)
	CreateImportedContract(ctx context.Context, appID string, input ImportedContractInput) (ImportedContract, error)
	ListImportedContracts(ctx context.Context, appID string, limit int64) ([]ImportedContract, error)
	DeleteImportedContract(ctx context.Context, accountID string, contractID string) (ImportedContract, error)
	CreateAPIKey(ctx context.Context, appID string, input APIKeyInput, prefix string, secretHash string) (APIKey, error)
	ListAPIKeys(ctx context.Context, appID string) ([]APIKey, error)
	GetAccountAPIKey(ctx context.Context, accountID string, keyID string) (APIKey, bool, error)
	VerifyAPIKey(ctx context.Context, prefix string, secretHash string) (APIKeyVerification, bool, error)
	RevokeAPIKey(ctx context.Context, id string) (APIKey, error)
	RecordAPIUsage(ctx context.Context, input APIUsageInput) error
	ListAPIUsage(ctx context.Context, appID string, limit int64) ([]APIUsage, error)
	EnqueueTransaction(ctx context.Context, input TransactionInput) (Transaction, error)
	GetTransaction(ctx context.Context, id string) (Transaction, bool, error)
	GetAccountTransaction(ctx context.Context, accountID string, transactionID string) (Transaction, bool, error)
	ListTransactions(ctx context.Context, appID string, limit int64) ([]Transaction, error)
	ClaimNextTransaction(ctx context.Context) (Transaction, bool, error)
	MarkTransactionSubmitted(ctx context.Context, transactionID string, transactionHash string) error
	MarkTransactionConfirmed(ctx context.Context, transactionID string, transactionHash string) error
	MarkTransactionFailed(ctx context.Context, transactionID string, message string) error
	MarkTransactionRetry(ctx context.Context, transactionID string, message string) error
	CreateERC20Deployment(ctx context.Context, input ERC20DeploymentInput) (ERC20Deployment, error)
	ListERC20Deployments(ctx context.Context, appID string, limit int64) ([]ERC20Deployment, error)
	DeleteQueuedERC20Deployment(ctx context.Context, accountID string, deploymentID string) (ERC20Deployment, error)
	RemoveERC20DeploymentFromDashboard(ctx context.Context, accountID string, deploymentID string) (ERC20Deployment, error)
	RetryERC20Deployment(ctx context.Context, accountID string, deploymentID string) (ERC20Deployment, error)
	ClaimNextERC20Deployment(ctx context.Context) (ERC20Deployment, bool, error)
	MarkERC20DeploymentSubmitted(ctx context.Context, deploymentID string, transactionHash string) error
	MarkERC20DeploymentConfirmed(ctx context.Context, deploymentID string, contractAddress string) error
	MarkERC20DeploymentFailed(ctx context.Context, deploymentID string, message string) error
	GetAccountERC20Deployment(ctx context.Context, accountID string, deploymentID string) (ERC20Deployment, bool, error)
	CreateERC1155EditionDeployment(ctx context.Context, input ERC1155EditionDeploymentInput) (ERC1155EditionDeployment, error)
	ListERC1155EditionDeployments(ctx context.Context, appID string, limit int64) ([]ERC1155EditionDeployment, error)
	DeleteQueuedERC1155EditionDeployment(ctx context.Context, accountID string, deploymentID string) (ERC1155EditionDeployment, error)
	RemoveERC1155EditionDeploymentFromDashboard(ctx context.Context, accountID string, deploymentID string) (ERC1155EditionDeployment, error)
	ClaimNextERC1155EditionDeployment(ctx context.Context) (ERC1155EditionDeployment, bool, error)
	MarkERC1155EditionDeploymentSubmitted(ctx context.Context, deploymentID string, transactionHash string) error
	MarkERC1155EditionDeploymentConfirmed(ctx context.Context, deploymentID string, contractAddress string) error
	MarkERC1155EditionDeploymentFailed(ctx context.Context, deploymentID string, message string) error
	CreateEscrowDeployment(ctx context.Context, input EscrowDeploymentInput) (EscrowDeployment, error)
	ListEscrowDeployments(ctx context.Context, appID string, limit int64) ([]EscrowDeployment, error)
	DeleteQueuedEscrowDeployment(ctx context.Context, accountID string, deploymentID string) (EscrowDeployment, error)
	RemoveEscrowDeploymentFromDashboard(ctx context.Context, accountID string, deploymentID string) (EscrowDeployment, error)
	ClaimNextEscrowDeployment(ctx context.Context) (EscrowDeployment, bool, error)
	MarkEscrowDeploymentSubmitted(ctx context.Context, deploymentID string, transactionHash string) error
	MarkEscrowDeploymentConfirmed(ctx context.Context, deploymentID string, contractAddress string, abi string) error
	MarkEscrowDeploymentFailed(ctx context.Context, deploymentID string, message string) error
	CreatePrivateClaimRegistryDeployment(ctx context.Context, input PrivateClaimRegistryDeploymentInput) (PrivateClaimRegistryDeployment, error)
	ListPrivateClaimRegistryDeployments(ctx context.Context, appID string, limit int64) ([]PrivateClaimRegistryDeployment, error)
	RemovePrivateClaimRegistryDeploymentFromDashboard(ctx context.Context, accountID string, deploymentID string) (PrivateClaimRegistryDeployment, error)
	ClaimNextPrivateClaimRegistryDeployment(ctx context.Context) (PrivateClaimRegistryDeployment, bool, error)
	MarkPrivateClaimRegistryDeploymentSubmitted(ctx context.Context, deploymentID string, transactionHash string) error
	MarkPrivateClaimRegistryDeploymentConfirmed(ctx context.Context, deploymentID string, contractAddress string, abi string) error
	MarkPrivateClaimRegistryDeploymentFailed(ctx context.Context, deploymentID string, message string) error
	CreateShieldedPayoutPoolDeployment(ctx context.Context, input ShieldedPayoutPoolDeploymentInput) (ShieldedPayoutPoolDeployment, error)
	ListShieldedPayoutPoolDeployments(ctx context.Context, appID string, limit int64) ([]ShieldedPayoutPoolDeployment, error)
	RemoveShieldedPayoutPoolDeploymentFromDashboard(ctx context.Context, accountID string, deploymentID string) (ShieldedPayoutPoolDeployment, error)
	ClaimNextShieldedPayoutPoolDeployment(ctx context.Context) (ShieldedPayoutPoolDeployment, bool, error)
	MarkShieldedPayoutPoolDeploymentSubmitted(ctx context.Context, deploymentID string, transactionHash string) error
	MarkShieldedPayoutPoolDeploymentConfirmed(ctx context.Context, deploymentID string, contractAddress string, abi string) error
	MarkShieldedPayoutPoolDeploymentFailed(ctx context.Context, deploymentID string, message string) error
	CreateShieldedWithdrawalVerifierDeployment(ctx context.Context, input ShieldedWithdrawalVerifierDeploymentInput) (ShieldedWithdrawalVerifierDeployment, error)
	ListShieldedWithdrawalVerifierDeployments(ctx context.Context, appID string, limit int64) ([]ShieldedWithdrawalVerifierDeployment, error)
	RemoveShieldedWithdrawalVerifierDeploymentFromDashboard(ctx context.Context, accountID string, deploymentID string) (ShieldedWithdrawalVerifierDeployment, error)
	ClaimNextShieldedWithdrawalVerifierDeployment(ctx context.Context) (ShieldedWithdrawalVerifierDeployment, bool, error)
	MarkShieldedWithdrawalVerifierDeploymentSubmitted(ctx context.Context, deploymentID string, transactionHash string) error
	MarkShieldedWithdrawalVerifierDeploymentConfirmed(ctx context.Context, deploymentID string, contractAddress string, abi string) error
	MarkShieldedWithdrawalVerifierDeploymentFailed(ctx context.Context, deploymentID string, message string) error
	CreateAccountAbstractionDeployment(ctx context.Context, input AccountAbstractionDeploymentInput) (AccountAbstractionDeployment, error)
	ListAccountAbstractionDeployments(ctx context.Context, appID string, limit int64) ([]AccountAbstractionDeployment, error)
	RemoveAccountAbstractionDeploymentFromDashboard(ctx context.Context, accountID string, deploymentID string) (AccountAbstractionDeployment, error)
	ClaimNextAccountAbstractionDeployment(ctx context.Context) (AccountAbstractionDeployment, bool, error)
	MarkAccountAbstractionDeploymentSubmitted(ctx context.Context, deploymentID string, transactionHash string) error
	MarkAccountAbstractionDeploymentConfirmed(ctx context.Context, deploymentID string, result AccountAbstractionDeployment) error
	MarkAccountAbstractionDeploymentFailed(ctx context.Context, deploymentID string, message string) error
	CreateZKProofSubmission(ctx context.Context, input ZKProofSubmissionInput) (ZKProofSubmission, error)
	ListZKProofSubmissions(ctx context.Context, appID string, limit int64) ([]ZKProofSubmission, error)
	GetZKProofSubmission(ctx context.Context, appID string, id string) (ZKProofSubmission, bool, error)
	MarkZKProofSubmissionSubmitted(ctx context.Context, id string, network string, accountAddress string, transactionResult string) error
	MarkZKProofSubmissionFailed(ctx context.Context, id string, message string) error
	AcquireWalletLock(ctx context.Context, appID string, walletAddress string, transactionID string, ttlSeconds int64) (WalletLock, bool, error)
	ReleaseWalletLock(ctx context.Context, appID string, walletAddress string, transactionID string) error
	Close(ctx context.Context) error
}
