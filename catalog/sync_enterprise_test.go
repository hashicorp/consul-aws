package catalog

import (
	"os"
	"testing"
)

func TestSyncEnterprise(t *testing.T) {
	if len(os.Getenv("INTTEST_ENTERPRISE")) == 0 {
		t.Skip("Set INTTEST_ENTERPRISE=1 to enable integration tests targetting consul enterprise")
	}
	awsNamespaceID := os.Getenv("NAMESPACEID")
	if len(awsNamespaceID) == 0 {
		awsNamespaceID = "ns-n5qqli2346hqood4"
	}

	runSyncTest(t, awsNamespaceID, "test-partition", "test-namespace")
}
