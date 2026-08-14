package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/techbudol1/gmr-engine/internal/config"
	"github.com/techbudol1/gmr-engine/internal/store"
)

const (
	testSolanaWallet = "11111111111111111111111111111111"
	testSolanaMint   = "4zMMC9srt5Ri5X14GAgXhaHii3GnPAEERYPJgZJDncDU"
)

func TestSolanaNativeBalance(t *testing.T) {
	rpc := solanaRPCServer(t, func(method string, _ []json.RawMessage) any {
		if method != "getBalance" {
			t.Fatalf("unexpected RPC method %q", method)
		}
		return map[string]any{"context": map[string]any{"slot": 42}, "value": 1_500_000_000}
	})
	defer rpc.Close()

	app := fiber.New()
	server := Server{cfg: config.Config{SolanaRPCURLs: map[string]string{"devnet": rpc.URL}}}
	app.Get("/balance", solanaPrincipal(store.App{ID: "project", AllowedSolanaNetworks: []string{"devnet"}}), server.solanaNativeBalance)

	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/balance?network=devnet&walletAddress="+testSolanaWallet, nil))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	var body struct {
		Balance struct {
			Raw   string `json:"raw"`
			Value string `json:"value"`
			Slot  uint64 `json:"slot"`
		} `json:"balance"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Balance.Raw != "1500000000" || body.Balance.Value != "1.5" || body.Balance.Slot != 42 {
		t.Fatalf("unexpected balance: %#v", body.Balance)
	}
}

func TestSolanaSPLBalancesAggregatesAccounts(t *testing.T) {
	rpcCalls := 0
	rpc := solanaRPCServer(t, func(method string, _ []json.RawMessage) any {
		if method != "getTokenAccountsByOwner" {
			t.Fatalf("unexpected RPC method %q", method)
		}
		rpcCalls++
		if rpcCalls == 2 {
			return map[string]any{"value": []any{}}
		}
		account := func(amount string) map[string]any {
			return map[string]any{"pubkey": testSolanaWallet, "account": map[string]any{"data": map[string]any{"parsed": map[string]any{"info": map[string]any{
				"mint": testSolanaMint, "tokenAmount": map[string]any{"amount": amount, "decimals": 6, "uiAmountString": "ignored"},
			}}}}}
		}
		return map[string]any{"value": []any{account("1250000"), account("750000")}}
	})
	defer rpc.Close()

	app := fiber.New()
	server := Server{cfg: config.Config{SolanaRPCURLs: map[string]string{"devnet": rpc.URL}}}
	app.Get("/balances", solanaPrincipal(store.App{}), server.solanaSPLBalances)
	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/balances?network=devnet&walletAddress="+testSolanaWallet, nil))
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Balances []solanaTokenBalance `json:"balances"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Balances) != 1 || body.Balances[0].Raw != "2000000" || body.Balances[0].Value != "2" {
		t.Fatalf("unexpected balances: %#v", body.Balances)
	}
}

func TestSolanaSendTransaction(t *testing.T) {
	rpc := solanaRPCServer(t, func(method string, _ []json.RawMessage) any {
		if method != "sendTransaction" {
			t.Fatalf("unexpected RPC method %q", method)
		}
		return "5UfDuW2iN9QexampleSignature"
	})
	defer rpc.Close()

	app := fiber.New()
	server := Server{cfg: config.Config{SolanaRPCURLs: map[string]string{"devnet": rpc.URL}}}
	app.Post("/send", solanaPrincipal(store.App{AllowedSolanaNetworks: []string{"devnet"}}), server.solanaSendTransaction)
	payload := `{"network":"devnet","transaction":"` + base64.StdEncoding.EncodeToString([]byte{1, 2, 3}) + `"}`
	request := httptest.NewRequest(http.MethodPost, "/send", strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

func TestSolanaProjectPolicyRejectsNetwork(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: errorHandler})
	server := Server{cfg: config.Config{SolanaRPCURLs: map[string]string{"devnet": "https://example.invalid"}}}
	app.Get("/balance", solanaPrincipal(store.App{AllowedSolanaNetworks: []string{"mainnet"}}), server.solanaNativeBalance)
	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/balance?network=devnet&walletAddress="+testSolanaWallet, nil))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusForbidden)
	}
}

func solanaPrincipal(app store.App) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Locals(principalLocalKey, principal{App: app})
		return c.Next()
	}
}

func solanaRPCServer(t *testing.T, result func(method string, params []json.RawMessage) any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		var payload struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		response.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(response).Encode(map[string]any{"id": 1, "jsonrpc": "2.0", "result": result(payload.Method, payload.Params)})
	}))
}
