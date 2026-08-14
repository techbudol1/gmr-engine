package deployer

import (
	"context"
	"strings"
	"testing"

	"github.com/techbudol1/gmr-engine/internal/contracts"
)

// The privacy-access contract gates all paid privacy features. Keep a compile
// test in the deployment package so a template change cannot silently remove
// its receipt, replay-protection, or treasury controls.
func TestPrivacyAccessPassTemplateCompiles(t *testing.T) {
	artifact, err := compileSolidity(context.Background(), "PrivacyAccessPass", contracts.PrivacyAccessPassSource("PrivacyAccessPass"))
	if err != nil {
		t.Fatalf("expected privacy access pass template to compile: %v", err)
	}
	if artifact.Bytecode == "" {
		t.Fatal("expected privacy access pass bytecode")
	}
	if artifact.ABI == "" {
		t.Fatal("expected privacy access pass ABI")
	}
	for _, requiredFunction := range []string{"payAccess", "consumeReceipt", "setAllowedFee", "setTreasury"} {
		if !strings.Contains(artifact.ABI, requiredFunction) {
			t.Fatalf("expected ABI to include %s", requiredFunction)
		}
	}
}
