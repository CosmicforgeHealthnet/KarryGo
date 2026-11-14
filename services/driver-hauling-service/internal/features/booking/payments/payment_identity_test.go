package payments

import (
	"testing"

	"cosmicforge/logistics/shared/go/managedsettlement"
)

// FLEET-13.11. Fleet settles Hauling work as HAULING; Wallet finds the
// booking's payment only through the spellings shared/go/managedsettlement
// lists for HAULING. If the name this package creates payments under ever
// leaves that list, every managed Hauling settlement stops resolving.
func TestHaulingPaymentSpellingResolvesToTheCanonicalSettlementService(t *testing.T) {
	canonical, ok := managedsettlement.CanonicalSource(sourceService)
	if !ok || canonical != managedsettlement.SourceHauling {
		t.Fatalf("payment source %q resolves to %q,%t; want %s", sourceService, canonical, ok, managedsettlement.SourceHauling)
	}
}
