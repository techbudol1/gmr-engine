package deployer

import "testing"

func TestUnconfiguredRPCIsDeterministicFailure(t *testing.T) {
	if !isDeterministicTransactionFailure("no RPC URL configured for chainId 421614") {
		t.Fatal("unconfigured RPC should fail without retrying")
	}
}
