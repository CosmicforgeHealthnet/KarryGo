package booking

import (
	"testing"

	"cosmicforge/logistics/shared/go/managedsettlement"
)

// FLEET-13.11. Fleet settles Dispatch work as DISPATCH; Wallet finds the
// booking's payment only through the spellings shared/go/managedsettlement
// lists for DISPATCH. If the name this package creates payments under ever
// leaves that list, every managed Dispatch settlement stops resolving.
func TestDispatchPaymentSpellingResolvesToTheCanonicalSettlementService(t *testing.T) {
	canonical, ok := managedsettlement.CanonicalSource(walletSourceService)
	if !ok || canonical != managedsettlement.SourceDispatch {
		t.Fatalf("payment source %q resolves to %q,%t; want %s", walletSourceService, canonical, ok, managedsettlement.SourceDispatch)
	}
}
