package store

import "testing"

func TestLookupContractDeploymentSpec(t *testing.T) {
	for _, deploymentType := range []string{
		"erc20",
		"erc1155-editions",
		"escrow",
		"marketplaces",
		"private-claim-registries",
		"shielded-payout-pools",
		"shielded-withdrawal-verifiers",
		"account-abstraction",
	} {
		if spec, ok := lookupContractDeploymentSpec(deploymentType); !ok || spec.label == "" || spec.relationship == "" {
			t.Fatalf("missing deployment spec for %q", deploymentType)
		}
	}
	if _, ok := lookupContractDeploymentSpec("unknown"); ok {
		t.Fatal("unsupported deployment type was accepted")
	}
}
