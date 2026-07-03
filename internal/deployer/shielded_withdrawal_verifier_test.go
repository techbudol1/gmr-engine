package deployer

import (
	"context"
	"testing"

	"github.com/techbudol1/gmr-engine/internal/contracts"
)

func TestShieldedWithdrawalVerifierTemplateCompiles(t *testing.T) {
	_, err := compileSolidity(context.Background(), "ShieldedWithdrawalVerifier", contracts.ShieldedWithdrawalVerifierSource("ShieldedWithdrawalVerifier"))
	if err != nil {
		t.Fatal(err)
	}
}
