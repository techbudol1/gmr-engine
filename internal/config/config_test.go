package config

import "testing"

func TestRPCURLRoutesHorizenAndAlchemyChains(t *testing.T) {
	cfg := Config{
		ChainRPCURL: "https://horizen.example/rpc",
		ChainRPCURLs: map[int64]string{
			421614: "https://arb-sepolia.g.alchemy.com/v2/key",
		},
	}

	tests := []struct {
		chainID int64
		want    string
	}{
		{HorizenTestnetChainID, "https://horizen.example/rpc"},
		{421614, "https://arb-sepolia.g.alchemy.com/v2/key"},
	}
	for _, test := range tests {
		got, err := cfg.RPCURL(test.chainID)
		if err != nil {
			t.Fatalf("RPCURL(%d) returned error: %v", test.chainID, err)
		}
		if got != test.want {
			t.Fatalf("RPCURL(%d) = %q, want %q", test.chainID, got, test.want)
		}
	}
}

func TestRPCURLRejectsUnconfiguredChain(t *testing.T) {
	_, err := (Config{ChainRPCURL: "https://horizen.example/rpc"}).RPCURL(1)
	if err == nil {
		t.Fatal("RPCURL accepted an unconfigured chain")
	}
}

func TestParseChainRPCURLs(t *testing.T) {
	urls, err := parseChainRPCURLs(`{"1":"https://eth-mainnet.g.alchemy.com/v2/key","421614":"https://arb-sepolia.g.alchemy.com/v2/key"}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(urls) != 2 || urls[421614] != "https://arb-sepolia.g.alchemy.com/v2/key" {
		t.Fatalf("unexpected URLs: %#v", urls)
	}
	if _, err := parseChainRPCURLs(`{"invalid":"https://example.com"}`); err == nil {
		t.Fatal("invalid chain ID was accepted")
	}
}

func TestSolanaRPCURLRoutesConfiguredNetworks(t *testing.T) {
	cfg := Config{SolanaRPCURLs: map[string]string{
		"mainnet": "https://solana-mainnet.g.alchemy.com/v2/key",
		"devnet":  "https://solana-devnet.g.alchemy.com/v2/key",
	}}

	for network, want := range map[string]string{
		"mainnet-beta": "https://solana-mainnet.g.alchemy.com/v2/key",
		"devnet":       "https://solana-devnet.g.alchemy.com/v2/key",
	} {
		got, err := cfg.SolanaRPCURL(network)
		if err != nil {
			t.Fatalf("SolanaRPCURL(%q) returned error: %v", network, err)
		}
		if got != want {
			t.Fatalf("SolanaRPCURL(%q) = %q, want %q", network, got, want)
		}
	}
}

func TestParseSolanaRPCURLsRejectsInvalidNetwork(t *testing.T) {
	if _, err := parseSolanaRPCURLs(`{"localnet":"https://example.com"}`); err == nil {
		t.Fatal("parseSolanaRPCURLs accepted an unsupported network")
	}
}

func TestAlchemyAPIKeyFromRPCURL(t *testing.T) {
	got := alchemyAPIKeyFromRPCURL("https://arb-sepolia.g.alchemy.com/v2/example-key")
	if got != "example-key" {
		t.Fatalf("alchemyAPIKeyFromRPCURL() = %q", got)
	}
	if got := alchemyAPIKeyFromRPCURL("https://example.com/rpc"); got != "" {
		t.Fatalf("non-Alchemy URL returned key %q", got)
	}
}
