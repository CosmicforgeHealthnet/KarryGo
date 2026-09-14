package walletusecases

import (
	"strings"
	"testing"
)

// WAL-011B. The read-back route exists so an execution service whose
// completion ended ambiguously can ask what actually happened instead of
// guessing. That only works if the service is recognised under the identity it
// actually signs with -- which on this platform is not the directory name and
// is not uniform: Taxi signs as "taxi-service", Dispatch as
// "dispatch-delivery-service", Hauling as "driver-hauling-service". A caller
// that passes HMAC and is then refused here fails in a way that looks like a
// credential problem and is not one.
func TestEveryExecutionServiceMayReadItsOwnSettlementBack(t *testing.T) {
	for _, caller := range []string{
		FleetServiceCaller,
		"taxi-service", "driver-taxi-service",
		"dispatch-delivery-service", "driver-dispatch-delivery-service",
		"hauling-service", "driver-hauling-service",
	} {
		if err := requireFinanceReader(caller); err != nil {
			t.Errorf("caller %q refused the read-back it needs: %v", caller, err)
		}
	}
}

// Reading is where the allowlist stops. An execution service must never be
// able to instruct a settlement, and an unrelated service must not be able to
// read one at all.
func TestUnrelatedCallersCannotReadManagedSettlements(t *testing.T) {
	for _, caller := range []string{
		"", "   ", "vendor-service", "admin-backoffice-service",
		"support-dispute-service", "customer-service", "notification-service",
	} {
		if err := requireFinanceReader(caller); err == nil {
			t.Errorf("caller %q was allowed to read managed settlements", caller)
		}
	}
}

// Marking a job managed only ever closes the legacy direct-payout path, so it
// shares the reader's allowlist -- but it must not quietly widen it.
func TestManagedMarkerAllowlistMatchesTheReaderExactly(t *testing.T) {
	for _, caller := range []string{
		FleetServiceCaller, "taxi-service", "dispatch-delivery-service",
		"driver-hauling-service", "vendor-service", "customer-service", "",
	} {
		readerErr := requireFinanceReader(caller)
		markerErr := requireManagedMarker(caller)
		if (readerErr == nil) != (markerErr == nil) {
			t.Errorf("caller %q: reader allowed=%t but marker allowed=%t",
				caller, readerErr == nil, markerErr == nil)
		}
	}
}

// Only Fleet may move the money. This is the boundary that keeps an execution
// service from deciding its own provider's payout.
func TestOnlyFleetMayInstructOrReverse(t *testing.T) {
	if strings.TrimSpace(FleetServiceCaller) == "" {
		t.Fatal("fleet caller identity is empty")
	}
	for _, caller := range []string{"taxi-service", "dispatch-delivery-service", "driver-hauling-service"} {
		if caller == FleetServiceCaller {
			t.Fatalf("an execution service shares Fleet's caller identity: %q", caller)
		}
	}
}
