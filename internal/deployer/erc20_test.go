package deployer

import (
	"context"
	"testing"

	"github.com/techbudol1/gmr-engine/internal/contracts"
)

func TestCompileERC20(t *testing.T) {
	artifact, err := compileSolidity(context.Background(), "Apecoin", contracts.ERC20Source("Apecoin", "Apecoin", "APE", 18))
	if err != nil {
		t.Fatal(err)
	}
	if artifact.ABI == "" || artifact.Bytecode == "" {
		t.Fatal("ERC20 compilation returned an empty artifact")
	}
}
