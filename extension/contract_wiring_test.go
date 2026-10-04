package extension

import (
	"testing"

	dashcontract "github.com/xraph/forge/extensions/dashboard/contract"
	"github.com/xraph/forge/extensions/dashboard/contract/dispatcher"
)

func TestContractContributorNeedsAnInitialisedHerald(t *testing.T) {
	// An extension that was never initialised skips registration quietly
	// rather than panicking the dashboard.
	var e Extension
	if err := e.RegisterContractContributor(dispatcher.New(nil), dashcontract.NewRegistry(), dashcontract.NewWardenRegistry()); err != nil {
		t.Fatalf("uninitialised extension: %v", err)
	}
}
