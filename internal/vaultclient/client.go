package vaultclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

type Wallet struct {
	ID         string `json:"id"`
	ProjectID  string `json:"projectId"`
	Address    string `json:"address"`
	WalletType string `json:"walletType"`
	Status     string `json:"status"`
	KeyVersion string `json:"keyVersion"`
	Metadata   string `json:"metadata,omitempty"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

func New(baseURL string, apiKey string, timeout time.Duration) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, errors.New("vault base URL is required")
	}
	if len(strings.TrimSpace(apiKey)) < 32 {
		return nil, errors.New("vault API key must be at least 32 characters")
	}
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	return &Client{
		baseURL: baseURL,
		apiKey:  strings.TrimSpace(apiKey),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

func (c *Client) CreateWallet(ctx context.Context, projectID string, walletType string, metadata string) (Wallet, error) {
	var response struct {
		Wallet Wallet `json:"wallet"`
	}
	err := c.request(ctx, http.MethodPost, "/v1/wallets", map[string]string{
		"projectId":  strings.TrimSpace(projectID),
		"walletType": strings.TrimSpace(walletType),
		"metadata":   strings.TrimSpace(metadata),
	}, &response)
	if err != nil {
		return Wallet{}, err
	}
	return response.Wallet, nil
}

func (c *Client) request(ctx context.Context, method string, path string, body any, target any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GMR-Vault-Key", c.apiKey)
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		var errorBody struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(res.Body).Decode(&errorBody)
		if strings.TrimSpace(errorBody.Error) == "" {
			errorBody.Error = res.Status
		}
		return fmt.Errorf("vault request failed: %s", errorBody.Error)
	}
	return json.NewDecoder(res.Body).Decode(target)
}

func Reference(walletID string) string {
	return "gmr-vault:v1:" + strings.TrimSpace(walletID)
}

func IsReference(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), "gmr-vault:v1:")
}
