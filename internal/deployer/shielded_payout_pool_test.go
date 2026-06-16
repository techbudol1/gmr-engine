package deployer

import (
	"context"
	"testing"

	"budol/gmr-engine/internal/contracts"
)

func TestShieldedPayoutPoolTemplateCompiles(t *testing.T) {
	artifact, err := compileSolidity(context.Background(), "ShieldedPayoutPool", contracts.ShieldedPayoutPoolSource("ShieldedPayoutPool"))
	if err != nil {
		t.Fatalf("expected shielded payout pool template to compile: %v", err)
	}
	if artifact.Bytecode == "" {
		t.Fatal("expected shielded payout pool bytecode")
	}
	if artifact.ABI == "" {
		t.Fatal("expected shielded payout pool ABI")
	}
}
