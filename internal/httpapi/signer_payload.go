package httpapi

import (
	"context"
	"errors"
	"strings"

	"budol/gmr-engine/internal/store"
	"budol/gmr-engine/internal/vaultclient"
)

func (s Server) projectSignerPayload(ctx context.Context, appID string, wallet store.ProjectWalletSecret, walletAddress string) (map[string]any, error) {
	if strings.TrimSpace(walletAddress) != "" && !strings.EqualFold(wallet.Address, walletAddress) {
		return nil, errors.New("transaction wallet is not the project default admin wallet")
	}
	if !vaultclient.IsReference(wallet.EncryptedPrivateKey) {
		return nil, errors.New("legacy local project wallet is no longer supported; create a GMR Vault wallet")
	}
	return map[string]any{
		"vaultAddress":   wallet.Address,
		"vaultApiKey":    s.cfg.VaultInternalKey,
		"vaultProjectId": appID,
		"vaultUrl":       s.cfg.VaultURL,
		"vaultWalletRef": wallet.EncryptedPrivateKey,
	}, nil
}
