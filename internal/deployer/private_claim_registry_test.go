package deployer

import (
	"context"
	"testing"

	"budol/gmr-engine/internal/contracts"
)

func TestPrivateClaimRegistryTemplateCompiles(t *testing.T) {
	artifact, err := compileSolidity(context.Background(), "PrivateClaimRegistry", contracts.PrivateClaimRegistrySource("PrivateClaimRegistry"))
	if err != nil {
		t.Fatalf("expected private claim registry template to compile: %v", err)
	}
	if artifact.Bytecode == "" {
		t.Fatal("expected private claim registry bytecode")
	}
	if artifact.ABI == "" {
		t.Fatal("expected private claim registry ABI")
	}
}
