package deployer

import (
	"context"
	"strings"
	"testing"

	"github.com/techbudol1/gmr-engine/internal/contracts"
)

func TestCompileDualCurrencyMarketplace(t *testing.T) {
	artifact, err := compileSolidity(
		context.Background(),
		"GameAssetMarketplace",
		contracts.DualCurrencyMarketplaceSource("GameAssetMarketplace"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.ABI == "" || artifact.Bytecode == "" {
		t.Fatal("marketplace compilation returned an empty artifact")
	}
	for _, functionName := range []string{
		`"name":"createListing"`,
		`"name":"approveCurrencyForListing"`,
		`"name":"buyFromListing"`,
	} {
		if !strings.Contains(artifact.ABI, functionName) {
			t.Fatalf("marketplace ABI is missing %s", functionName)
		}
	}
}
