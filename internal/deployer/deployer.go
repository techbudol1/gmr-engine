package deployer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/techbudol1/gmr-engine/internal/config"
	"github.com/techbudol1/gmr-engine/internal/contracts"
	"github.com/techbudol1/gmr-engine/internal/store"
	"github.com/techbudol1/gmr-engine/internal/vaultclient"
)

type Worker struct {
	cfg   config.Config
	store store.Store
}

func New(ctx context.Context, cfg config.Config, engineStore store.Store) (*Worker, error) {
	if strings.TrimSpace(cfg.AlchemyRPCURL) == "" {
		return nil, errors.New("alchemy rpc url is required")
	}
	return &Worker{cfg: cfg, store: engineStore}, nil
}

func (w *Worker) Close() {
}

func (w *Worker) Run(ctx context.Context) {
	interval := w.cfg.DeployerInterval
	if interval <= 0 {
		interval = 8 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		w.processOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) processOnce(ctx context.Context) {
	if err := w.processTransaction(ctx); err != nil {
		log.Printf("transaction worker error: %v", err)
	}
	if err := w.processERC20(ctx); err != nil {
		log.Printf("erc20 deployer error: %v", err)
	}
	if err := w.processERC1155Edition(ctx); err != nil {
		log.Printf("erc1155 edition deployer error: %v", err)
	}
	if err := w.processEscrow(ctx); err != nil {
		log.Printf("escrow deployer error: %v", err)
	}
	if err := w.processPrivateClaimRegistry(ctx); err != nil {
		log.Printf("private claim registry deployer error: %v", err)
	}
	if err := w.processShieldedPayoutPool(ctx); err != nil {
		log.Printf("shielded payout pool deployer error: %v", err)
	}
	if err := w.processShieldedWithdrawalVerifier(ctx); err != nil {
		log.Printf("shielded withdrawal verifier deployer error: %v", err)
	}
}

func (w *Worker) processTransaction(ctx context.Context) error {
	transaction, ok, err := w.store.ClaimNextTransaction(ctx)
	if err != nil || !ok {
		return err
	}
	lock, locked, err := w.store.AcquireWalletLock(ctx, transaction.AppID, transaction.WalletAddress, transaction.ID, 300)
	if err != nil {
		w.markTransactionAttempt(ctx, transaction, err.Error())
		return err
	}
	if !locked {
		w.markTransactionAttempt(ctx, transaction, "wallet is busy")
		return nil
	}
	defer func() {
		_ = w.store.ReleaseWalletLock(context.Background(), lock.AppID, lock.WalletAddress, lock.TransactionID)
	}()

	result, err := w.broadcastERC20Transaction(ctx, transaction)
	if err != nil {
		w.markTransactionAttempt(ctx, transaction, err.Error())
		return err
	}
	_ = w.store.MarkTransactionSubmitted(ctx, transaction.ID, result.TransactionHash)
	w.sendTransactionWebhook(ctx, transaction.ID, "submitted")
	if result.Status != "success" {
		_ = w.store.MarkTransactionFailed(ctx, transaction.ID, "transaction reverted")
		w.sendTransactionWebhook(ctx, transaction.ID, "failed")
		return nil
	}
	err = w.store.MarkTransactionConfirmed(ctx, transaction.ID, result.TransactionHash)
	w.sendTransactionWebhook(ctx, transaction.ID, "confirmed")
	return err
}

func (w *Worker) markTransactionAttempt(ctx context.Context, transaction store.Transaction, message string) {
	if transaction.AttemptCount < 3 {
		_ = w.store.MarkTransactionRetry(ctx, transaction.ID, message)
		return
	}
	_ = w.store.MarkTransactionFailed(ctx, transaction.ID, message)
	w.sendTransactionWebhook(ctx, transaction.ID, "failed")
}

func (w *Worker) sendTransactionWebhook(ctx context.Context, transactionID string, event string) {
	transaction, ok, err := w.store.GetTransaction(ctx, transactionID)
	if err != nil || !ok {
		return
	}
	app, ok, err := w.store.GetApp(ctx, transaction.AppID)
	if err != nil || !ok || strings.TrimSpace(app.WebhookURL) == "" {
		return
	}
	payload, err := json.Marshal(map[string]any{
		"event":       "transaction." + event,
		"projectId":   app.ID,
		"projectName": app.Name,
		"transaction": transaction,
	})
	if err != nil {
		return
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, app.WebhookURL, bytes.NewReader(payload))
	if err != nil {
		return
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-GMR-Engine-Event", "transaction."+event)
	client := http.Client{Timeout: 8 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		log.Printf("webhook dispatch failed for transaction %s: %v", transaction.ID, err)
		return
	}
	defer response.Body.Close()
	if response.StatusCode >= 300 {
		log.Printf("webhook dispatch returned %d for transaction %s", response.StatusCode, transaction.ID)
	}
}

func (w *Worker) processERC20(ctx context.Context) error {
	deployment, ok, err := w.store.ClaimNextERC20Deployment(ctx)
	if err != nil || !ok {
		return err
	}

	result, err := w.deployERC20(ctx, deployment)
	if err != nil {
		_ = w.store.MarkERC20DeploymentFailed(ctx, deployment.ID, err.Error())
		return err
	}
	_ = w.store.MarkERC20DeploymentSubmitted(ctx, deployment.ID, result.TransactionHash)
	if result.Status != "success" {
		_ = w.store.MarkERC20DeploymentFailed(ctx, deployment.ID, "deployment transaction reverted")
		return nil
	}
	return w.store.MarkERC20DeploymentConfirmed(ctx, deployment.ID, result.ContractAddress)
}

func (w *Worker) processERC1155Edition(ctx context.Context) error {
	deployment, ok, err := w.store.ClaimNextERC1155EditionDeployment(ctx)
	if err != nil || !ok {
		return err
	}

	result, err := w.deployERC1155Edition(ctx, deployment)
	if err != nil {
		_ = w.store.MarkERC1155EditionDeploymentFailed(ctx, deployment.ID, err.Error())
		return err
	}
	_ = w.store.MarkERC1155EditionDeploymentSubmitted(ctx, deployment.ID, result.TransactionHash)
	if result.Status != "success" {
		_ = w.store.MarkERC1155EditionDeploymentFailed(ctx, deployment.ID, "deployment transaction reverted")
		return nil
	}
	return w.store.MarkERC1155EditionDeploymentConfirmed(ctx, deployment.ID, result.ContractAddress)
}

func (w *Worker) processEscrow(ctx context.Context) error {
	deployment, ok, err := w.store.ClaimNextEscrowDeployment(ctx)
	if err != nil || !ok {
		return err
	}

	result, artifact, err := w.deployEscrow(ctx, deployment)
	if err != nil {
		_ = w.store.MarkEscrowDeploymentFailed(ctx, deployment.ID, err.Error())
		return err
	}
	_ = w.store.MarkEscrowDeploymentSubmitted(ctx, deployment.ID, result.TransactionHash)
	if result.Status != "success" {
		_ = w.store.MarkEscrowDeploymentFailed(ctx, deployment.ID, "deployment transaction reverted")
		return nil
	}
	return w.store.MarkEscrowDeploymentConfirmed(ctx, deployment.ID, result.ContractAddress, artifact.ABI)
}

func (w *Worker) processPrivateClaimRegistry(ctx context.Context) error {
	deployment, ok, err := w.store.ClaimNextPrivateClaimRegistryDeployment(ctx)
	if err != nil || !ok {
		return err
	}

	result, artifact, err := w.deployPrivateClaimRegistry(ctx, deployment)
	if err != nil {
		_ = w.store.MarkPrivateClaimRegistryDeploymentFailed(ctx, deployment.ID, err.Error())
		return err
	}
	_ = w.store.MarkPrivateClaimRegistryDeploymentSubmitted(ctx, deployment.ID, result.TransactionHash)
	if result.Status != "success" {
		_ = w.store.MarkPrivateClaimRegistryDeploymentFailed(ctx, deployment.ID, "deployment transaction reverted")
		return nil
	}
	return w.store.MarkPrivateClaimRegistryDeploymentConfirmed(ctx, deployment.ID, result.ContractAddress, artifact.ABI)
}

func (w *Worker) processShieldedPayoutPool(ctx context.Context) error {
	deployment, ok, err := w.store.ClaimNextShieldedPayoutPoolDeployment(ctx)
	if err != nil || !ok {
		return err
	}

	result, artifact, err := w.deployShieldedPayoutPool(ctx, deployment)
	if err != nil {
		_ = w.store.MarkShieldedPayoutPoolDeploymentFailed(ctx, deployment.ID, err.Error())
		return err
	}
	_ = w.store.MarkShieldedPayoutPoolDeploymentSubmitted(ctx, deployment.ID, result.TransactionHash)
	if result.Status != "success" {
		_ = w.store.MarkShieldedPayoutPoolDeploymentFailed(ctx, deployment.ID, "deployment transaction reverted")
		return nil
	}
	return w.store.MarkShieldedPayoutPoolDeploymentConfirmed(ctx, deployment.ID, result.ContractAddress, artifact.ABI)
}

func (w *Worker) processShieldedWithdrawalVerifier(ctx context.Context) error {
	deployment, ok, err := w.store.ClaimNextShieldedWithdrawalVerifierDeployment(ctx)
	if err != nil || !ok {
		return err
	}

	result, artifact, err := w.deployShieldedWithdrawalVerifier(ctx, deployment)
	if err != nil {
		_ = w.store.MarkShieldedWithdrawalVerifierDeploymentFailed(ctx, deployment.ID, err.Error())
		return err
	}
	_ = w.store.MarkShieldedWithdrawalVerifierDeploymentSubmitted(ctx, deployment.ID, result.TransactionHash)
	if result.Status != "success" {
		_ = w.store.MarkShieldedWithdrawalVerifierDeploymentFailed(ctx, deployment.ID, "deployment transaction reverted")
		return nil
	}
	return w.store.MarkShieldedWithdrawalVerifierDeploymentConfirmed(ctx, deployment.ID, result.ContractAddress, artifact.ABI)
}

func (w *Worker) deployERC20(ctx context.Context, deployment store.ERC20Deployment) (deployResult, error) {
	artifact, err := compileSolidity(ctx, deployment.SourceName, contracts.ERC20Source(deployment.SourceName, deployment.Name, deployment.Symbol, deployment.Decimals))
	if err != nil {
		return deployResult{}, err
	}
	initialSupply, err := decimalAmountToBaseUnits(deployment.InitialSupply, deployment.Decimals)
	if err != nil {
		return deployResult{}, err
	}
	return w.broadcastDeployment(ctx, deployment.AppID, deployment.ChainID, artifact, []constructorArg{
		{Kind: "address", Value: deployment.OwnerAddress},
		{Kind: "uint", Value: initialSupply},
	})
}

func (w *Worker) deployERC1155Edition(ctx context.Context, deployment store.ERC1155EditionDeployment) (deployResult, error) {
	artifact, err := compileSolidity(ctx, deployment.SourceName, contracts.ERC1155EditionSource(deployment.SourceName, deployment.Name, deployment.Symbol))
	if err != nil {
		return deployResult{}, err
	}
	return w.broadcastDeployment(ctx, deployment.AppID, deployment.ChainID, artifact, []constructorArg{
		{Kind: "address", Value: deployment.OwnerAddress},
		{Kind: "string", Value: deployment.BaseURI},
		{Kind: "string", Value: deployment.ContractURI},
		{Kind: "address", Value: deployment.RecipientAddress},
		{Kind: "uint", Value: deployment.InitialTokenID},
		{Kind: "uint", Value: deployment.InitialSupply},
	})
}

func (w *Worker) deployEscrow(ctx context.Context, deployment store.EscrowDeployment) (deployResult, compiledArtifact, error) {
	artifact, err := compileSolidity(ctx, deployment.SourceName, contracts.BudolEscrowSource(deployment.SourceName))
	if err != nil {
		return deployResult{}, compiledArtifact{}, err
	}
	result, err := w.broadcastDeployment(ctx, deployment.AppID, deployment.ChainID, artifact, []constructorArg{
		{Kind: "address", Value: deployment.TokenAddress},
		{Kind: "address", Value: deployment.OwnerAddress},
		{Kind: "address", Value: deployment.TreasuryAddress},
		{Kind: "uint", Value: fmt.Sprintf("%d", deployment.FeeBps)},
	})
	if err != nil {
		return deployResult{}, compiledArtifact{}, err
	}
	return result, artifact, nil
}

func (w *Worker) deployPrivateClaimRegistry(ctx context.Context, deployment store.PrivateClaimRegistryDeployment) (deployResult, compiledArtifact, error) {
	artifact, err := compileSolidity(ctx, deployment.SourceName, contracts.PrivateClaimRegistrySource(deployment.SourceName))
	if err != nil {
		return deployResult{}, compiledArtifact{}, err
	}
	result, err := w.broadcastDeployment(ctx, deployment.AppID, deployment.ChainID, artifact, []constructorArg{
		{Kind: "address", Value: deployment.OwnerAddress},
		{Kind: "address", Value: deployment.VerifierAddress},
	})
	if err != nil {
		return deployResult{}, compiledArtifact{}, err
	}
	return result, artifact, nil
}

func (w *Worker) deployShieldedPayoutPool(ctx context.Context, deployment store.ShieldedPayoutPoolDeployment) (deployResult, compiledArtifact, error) {
	artifact, err := compileSolidity(ctx, deployment.SourceName, contracts.ShieldedPayoutPoolSource(deployment.SourceName))
	if err != nil {
		return deployResult{}, compiledArtifact{}, err
	}
	result, err := w.broadcastDeployment(ctx, deployment.AppID, deployment.ChainID, artifact, []constructorArg{
		{Kind: "address", Value: deployment.TokenAddress},
		{Kind: "address", Value: deployment.OwnerAddress},
		{Kind: "address", Value: deployment.VerifierAddress},
		{Kind: "uint", Value: deployment.Denomination},
	})
	if err != nil {
		return deployResult{}, compiledArtifact{}, err
	}
	return result, artifact, nil
}

func (w *Worker) deployShieldedWithdrawalVerifier(ctx context.Context, deployment store.ShieldedWithdrawalVerifierDeployment) (deployResult, compiledArtifact, error) {
	artifact, err := compileSolidity(ctx, deployment.SourceName, contracts.ShieldedWithdrawalVerifierSource(deployment.SourceName))
	if err != nil {
		return deployResult{}, compiledArtifact{}, err
	}
	result, err := w.broadcastDeployment(ctx, deployment.AppID, deployment.ChainID, artifact, []constructorArg{})
	if err != nil {
		return deployResult{}, compiledArtifact{}, err
	}
	return result, artifact, nil
}

type constructorArg struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type deployResult struct {
	ContractAddress string `json:"contractAddress"`
	Status          string `json:"status"`
	TransactionHash string `json:"transactionHash"`
}

type transactionResult struct {
	Status          string `json:"status"`
	TransactionHash string `json:"transactionHash"`
}

func (w *Worker) broadcastERC20Transaction(ctx context.Context, transaction store.Transaction) (transactionResult, error) {
	if transaction.Kind != "contract_write" {
		return transactionResult{}, errors.New("unsupported transaction kind")
	}
	if result, ok, err := w.broadcastGenericTransaction(ctx, transaction); ok || err != nil {
		return result, err
	}
	method := strings.ToLower(strings.TrimSpace(transaction.Method))
	if method != "mint" && method != "transfer" && method != "burn" {
		return transactionResult{}, errors.New("unsupported contract write method")
	}
	if strings.TrimSpace(transaction.ContractAddress) == "" {
		return transactionResult{}, errors.New("contract address is required")
	}
	signerPayload, err := w.projectSignerPayloadForWallet(ctx, transaction.AppID, transaction.WalletAddress)
	if err != nil {
		return transactionResult{}, err
	}
	payload := map[string]any{
		"action":          method,
		"chainId":         transaction.ChainID,
		"contractAddress": transaction.ContractAddress,
		"mode":            "write",
		"rpcUrl":          w.cfg.AlchemyRPCURL,
		"walletAddress":   transaction.WalletAddress,
	}
	for key, value := range signerPayload {
		payload[key] = value
	}
	switch method {
	case "mint", "transfer":
		if len(transaction.Args) < 2 {
			return transactionResult{}, errors.New("recipient and amount are required")
		}
		payload["recipient"] = transaction.Args[0]
		payload["amount"] = transaction.Args[1]
	case "burn":
		if len(transaction.Args) < 1 {
			return transactionResult{}, errors.New("amount is required")
		}
		payload["amount"] = transaction.Args[0]
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return transactionResult{}, err
	}
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "bun", "scripts/erc20-console.ts")
	cmd.Stdin = bytes.NewReader(body)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return transactionResult{}, fmt.Errorf("transaction broadcast failed: %s", strings.TrimSpace(string(output)))
	}
	var result struct {
		Transactions []transactionResult `json:"transactions"`
	}
	if err := unmarshalCommandJSON(output, &result); err != nil {
		return transactionResult{}, err
	}
	if len(result.Transactions) == 0 || result.Transactions[0].TransactionHash == "" {
		return transactionResult{}, errors.New("transaction did not return a hash")
	}
	return result.Transactions[0], nil
}

func (w *Worker) broadcastGenericTransaction(ctx context.Context, transaction store.Transaction) (transactionResult, bool, error) {
	var metadata struct {
		ABI json.RawMessage `json:"abi"`
	}
	if strings.TrimSpace(transaction.Metadata) == "" {
		return transactionResult{}, false, nil
	}
	if err := json.Unmarshal([]byte(transaction.Metadata), &metadata); err != nil || len(metadata.ABI) == 0 || string(metadata.ABI) == "null" {
		return transactionResult{}, false, nil
	}
	signerPayload, err := w.projectSignerPayloadForWallet(ctx, transaction.AppID, transaction.WalletAddress)
	if err != nil {
		return transactionResult{}, true, err
	}
	payloadMap := map[string]any{
		"abi":             metadata.ABI,
		"args":            transaction.Args,
		"chainId":         transaction.ChainID,
		"contractAddress": transaction.ContractAddress,
		"functionName":    transaction.Method,
		"mode":            "write",
		"rpcUrl":          w.cfg.AlchemyRPCURL,
		"value":           transaction.Value,
	}
	for key, value := range signerPayload {
		payloadMap[key] = value
	}
	payload, err := json.Marshal(payloadMap)
	if err != nil {
		return transactionResult{}, true, err
	}
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "bun", "scripts/contract-call.ts")
	cmd.Stdin = bytes.NewReader(payload)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return transactionResult{}, true, fmt.Errorf("generic transaction broadcast failed: %s", strings.TrimSpace(string(output)))
	}
	var result struct {
		Transactions []transactionResult `json:"transactions"`
	}
	if err := unmarshalCommandJSON(output, &result); err != nil {
		return transactionResult{}, true, err
	}
	if len(result.Transactions) == 0 || result.Transactions[0].TransactionHash == "" {
		return transactionResult{}, true, errors.New("generic transaction did not return a hash")
	}
	return result.Transactions[0], true, nil
}

func (w *Worker) broadcastDeployment(ctx context.Context, appID string, chainID int64, artifact compiledArtifact, args []constructorArg) (deployResult, error) {
	signerPayload, err := w.projectSignerPayload(ctx, appID)
	if err != nil {
		return deployResult{}, err
	}
	payloadMap := map[string]any{
		"abi":      artifact.ABI,
		"args":     args,
		"bytecode": artifact.Bytecode,
		"chainId":  chainID,
		"rpcUrl":   w.cfg.AlchemyRPCURL,
	}
	for key, value := range signerPayload {
		payloadMap[key] = value
	}
	payload, err := json.Marshal(payloadMap)
	if err != nil {
		return deployResult{}, err
	}
	commandCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "bun", "scripts/deploy-contract.ts")
	cmd.Stdin = bytes.NewReader(payload)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return deployResult{}, fmt.Errorf("deployment broadcast failed: %s", strings.TrimSpace(string(output)))
	}
	var result deployResult
	if err := unmarshalCommandJSON(output, &result); err != nil {
		return deployResult{}, err
	}
	if result.TransactionHash == "" {
		return deployResult{}, errors.New("deployment did not return transaction hash")
	}
	return result, nil
}

func (w *Worker) projectSignerPayload(ctx context.Context, appID string) (map[string]any, error) {
	return w.projectSignerPayloadForWallet(ctx, appID, "")
}

func (w *Worker) projectSignerPayloadForWallet(ctx context.Context, appID string, walletAddress string) (map[string]any, error) {
	wallet, ok, err := w.store.GetProjectDefaultAdminWalletSecret(ctx, appID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("project server wallet not found")
	}
	if strings.TrimSpace(walletAddress) != "" && !strings.EqualFold(wallet.Address, walletAddress) {
		return nil, errors.New("transaction wallet is not the project default admin wallet")
	}
	if !vaultclient.IsReference(wallet.EncryptedPrivateKey) {
		return nil, errors.New("legacy local project wallet is no longer supported; create a GMR Vault wallet")
	}
	return map[string]any{
		"vaultAddress":   wallet.Address,
		"vaultApiKey":    w.cfg.VaultInternalKey,
		"vaultProjectId": appID,
		"vaultUrl":       w.cfg.VaultURL,
		"vaultWalletRef": wallet.EncryptedPrivateKey,
	}, nil
}

func decimalAmountToBaseUnits(amount string, decimals int64) (string, error) {
	trimmed := strings.TrimSpace(amount)
	if trimmed == "" {
		return "", errors.New("amount is required")
	}
	if decimals < 0 || decimals > 77 {
		return "", errors.New("invalid decimals")
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) > 2 {
		return "", errors.New("invalid decimal amount")
	}
	whole := parts[0]
	if whole == "" {
		whole = "0"
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if len(fraction) > int(decimals) {
		return "", errors.New("amount has too many decimal places")
	}
	for _, char := range whole + fraction {
		if char < '0' || char > '9' {
			return "", errors.New("amount must be numeric")
		}
	}
	base := new(big.Int).Exp(big.NewInt(10), big.NewInt(decimals), nil)
	wholeValue, ok := new(big.Int).SetString(whole, 10)
	if !ok {
		return "", errors.New("invalid amount")
	}
	result := new(big.Int).Mul(wholeValue, base)
	if fraction != "" {
		fraction += strings.Repeat("0", int(decimals)-len(fraction))
		fractionValue, ok := new(big.Int).SetString(fraction, 10)
		if !ok {
			return "", errors.New("invalid amount")
		}
		result.Add(result, fractionValue)
	}
	return result.String(), nil
}

type compiledArtifact struct {
	ABI      string
	Bytecode string
}

func unmarshalCommandJSON(output []byte, target any) error {
	trimmed := bytes.TrimSpace(output)
	jsonStart := bytes.IndexAny(trimmed, "{[")
	if jsonStart < 0 {
		return errors.New("command did not return JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed[jsonStart:]))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func compileSolidity(ctx context.Context, contractName string, source string) (compiledArtifact, error) {
	contractName = strings.TrimSpace(contractName)
	if contractName == "" {
		return compiledArtifact{}, errors.New("contract source name is required")
	}
	fileName := contractName + ".sol"
	input := map[string]any{
		"language": "Solidity",
		"sources": map[string]any{
			fileName: map[string]any{"content": source},
		},
		"settings": map[string]any{
			"optimizer": map[string]any{"enabled": true, "runs": 200},
			"outputSelection": map[string]any{
				"*": map[string]any{"*": []string{"abi", "evm.bytecode.object"}},
			},
		},
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return compiledArtifact{}, err
	}
	commandCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "npx", "-y", "solc", "--standard-json")
	cmd.Stdin = bytes.NewReader(payload)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return compiledArtifact{}, fmt.Errorf("solc compile failed: %s", strings.TrimSpace(string(output)))
	}
	jsonStart := bytes.IndexByte(output, '{')
	if jsonStart < 0 {
		return compiledArtifact{}, errors.New("solc compile returned invalid output")
	}
	var parsed struct {
		Errors []struct {
			Severity         string `json:"severity"`
			FormattedMessage string `json:"formattedMessage"`
		} `json:"errors"`
		Contracts map[string]map[string]struct {
			ABI []any `json:"abi"`
			EVM struct {
				Bytecode struct {
					Object string `json:"object"`
				} `json:"bytecode"`
			} `json:"evm"`
		} `json:"contracts"`
	}
	if err := json.Unmarshal(output[jsonStart:], &parsed); err != nil {
		return compiledArtifact{}, err
	}
	for _, compilerError := range parsed.Errors {
		if compilerError.Severity == "error" {
			return compiledArtifact{}, errors.New(strings.TrimSpace(compilerError.FormattedMessage))
		}
	}
	contract, ok := parsed.Contracts[fileName][contractName]
	if !ok {
		return compiledArtifact{}, errors.New("compiled contract artifact not found")
	}
	abiBytes, err := json.Marshal(contract.ABI)
	if err != nil {
		return compiledArtifact{}, err
	}
	if strings.TrimSpace(contract.EVM.Bytecode.Object) == "" {
		return compiledArtifact{}, errors.New("compiled contract bytecode is empty")
	}
	return compiledArtifact{ABI: string(abiBytes), Bytecode: contract.EVM.Bytecode.Object}, nil
}
