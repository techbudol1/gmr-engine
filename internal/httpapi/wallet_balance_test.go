package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/techbudol1/gmr-engine/internal/config"
	"github.com/techbudol1/gmr-engine/internal/store"
)

func TestNativeTokenBalance(t *testing.T) {
	rpc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Method != "eth_getBalance" {
			t.Fatalf("unexpected RPC method %q", request.Method)
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x16345785d8a0000"}`))
	}))
	defer rpc.Close()

	server := Server{cfg: config.Config{ChainRPCURLs: map[int64]string{421614: rpc.URL}}}
	app := fiber.New()
	app.Get("/v1/native/balance", func(c *fiber.Ctx) error {
		c.Locals(principalLocalKey, principal{App: store.App{AllowedChains: []int64{421614}}})
		return server.nativeTokenBalance(c)
	})

	request := httptest.NewRequest(http.MethodGet, "/v1/native/balance?chainId=421614&walletAddress=0x5a7660a14fe029ffa00554bd8047dd4ca09dd9a3", nil)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	var body struct {
		Balance struct {
			ChainID       int64  `json:"chainId"`
			Raw           string `json:"raw"`
			Symbol        string `json:"symbol"`
			Value         string `json:"value"`
			WalletAddress string `json:"walletAddress"`
		} `json:"balance"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Balance.ChainID != 421614 || body.Balance.Raw != "100000000000000000" || body.Balance.Value != "0.1" || body.Balance.Symbol != "ETH" {
		t.Fatalf("unexpected balance: %#v", body.Balance)
	}
}

func TestNativeTokenBalanceRejectsDisallowedChain(t *testing.T) {
	server := Server{cfg: config.Config{ChainRPCURLs: map[int64]string{421614: "https://example.invalid"}}}
	app := fiber.New()
	app.Get("/v1/native/balance", func(c *fiber.Ctx) error {
		c.Locals(principalLocalKey, principal{App: store.App{AllowedChains: []int64{2651420}}})
		return server.nativeTokenBalance(c)
	})

	request := httptest.NewRequest(http.MethodGet, "/v1/native/balance?chainId=421614&walletAddress=0x5a7660a14fe029ffa00554bd8047dd4ca09dd9a3", nil)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.StatusCode)
	}
}

func TestFormatBaseUnitsExactPreservesPrecision(t *testing.T) {
	got := formatBaseUnitsExact("14960506682244920", 18)
	if got != "0.01496050668224492" {
		t.Fatalf("formatBaseUnitsExact() = %q", got)
	}
}
