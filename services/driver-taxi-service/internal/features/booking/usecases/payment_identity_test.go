package bookingusecases

import (
	"testing"

	"cosmicforge/logistics/shared/go/managedsettlement"
)

// FLEET-13.11. Fleet settles Taxi work as TAXI; Wallet finds the booking's
// payment only through the spellings shared/go/managedsettlement lists for
// TAXI. If the name this package creates payments under ever leaves that
// list, every managed Taxi settlement stops resolving.
func TestTaxiPaymentSpellingResolvesToTheCanonicalSettlementService(t *testing.T) {
	canonical, ok := managedsettlement.CanonicalSource(PaymentServiceName)
	if !ok || canonical != managedsettlement.SourceTaxi {
		t.Fatalf("payment source %q resolves to %q,%t; want %s", PaymentServiceName, canonical, ok, managedsettlement.SourceTaxi)
	}
}
