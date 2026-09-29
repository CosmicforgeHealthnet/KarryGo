package movementusecases

import (
	"context"
	"net/http"
	"testing"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/events"
)

func purchaseRow(state string) events.MovementSummary {
	return events.MovementSummary{CustomerID: customerA, Authority: events.MovementAuthorityVendorCommerce, SourceRef: "po-1",
		ProductCode: "FOOD", MovementType: events.MovementTypeCommerceOrder, State: state, ParentContextType: "COMMERCE_ORDER", ParentContextRef: "po-1"}
}

// Vendor publishes only its own purchase rows; nobody else may, and Vendor
// may not publish logistics movements.
func TestVendorCommerceIngestIsCallerBound(t *testing.T) {
	repo := newMemoryRepo()
	withVendor := CallerAuthorities{"vendor-service": events.MovementAuthorityVendorCommerce}
	for caller, authority := range callers {
		withVendor[caller] = authority
	}
	svc := NewService(Options{Repository: repo, Callers: withVendor, ConsumerEnabled: true, GroupingCallers: []string{"vendor-service"}})
	ctx := context.Background()
	if _, err := svc.Ingest(ctx, "vendor-service", envelope(t, "p1", "vendor-service", "commerce.customer_order.created", 1, purchaseRow(events.MovementStateRequested))); err != nil {
		t.Fatalf("vendor purchase row refused: %v", err)
	}
	if _, err := svc.Ingest(ctx, "driver-taxi-service", envelope(t, "p2", "driver-taxi-service", "x", 2, purchaseRow(events.MovementStateCancelled))); apperrors.From(err).Status != http.StatusForbidden {
		t.Fatalf("taxi publishing a purchase row status = %d", apperrors.From(err).Status)
	}
	ride := taxi(events.MovementStateSearching, 0)
	if _, err := svc.Ingest(ctx, "vendor-service", envelope(t, "p3", "vendor-service", "x", 1, ride)); apperrors.From(err).Status != http.StatusForbidden {
		t.Fatalf("vendor publishing a Ride status = %d", apperrors.From(err).Status)
	}

	// Reconciliation reads: grouping by default, the purchase row on request,
	// and never another authority's rows.
	if _, err := svc.ProjectedVersionsFor(ctx, "vendor-service", "", []string{"po-1"}); err != nil {
		t.Fatalf("grouping reconciliation: %v", err)
	}
	versions, err := svc.ProjectedVersionsFor(ctx, "vendor-service", events.MovementAuthorityVendorCommerce, []string{"po-1"})
	if err != nil || versions["po-1"] != 1 {
		t.Fatalf("purchase reconciliation = %v %v", versions, err)
	}
	if _, err = svc.ProjectedVersionsFor(ctx, "vendor-service", events.MovementAuthorityTaxi, []string{"po-1"}); apperrors.From(err).Status != http.StatusForbidden {
		t.Fatalf("vendor reading Taxi versions status = %d", apperrors.From(err).Status)
	}
	if _, err = svc.ProjectedVersionsFor(ctx, "driver-taxi-service", events.MovementAuthorityVendorCommerce, []string{"po-1"}); apperrors.From(err).Status != http.StatusForbidden {
		t.Fatalf("taxi reading purchase versions status = %d", apperrors.From(err).Status)
	}
}

func TestParseCallerAuthoritiesAcceptsVendorCommerce(t *testing.T) {
	parsed, err := ParseCallerAuthorities("vendor-service=VENDOR_COMMERCE;driver-taxi-service=TAXI")
	if err != nil || parsed["vendor-service"] != events.MovementAuthorityVendorCommerce {
		t.Fatalf("parsed=%v err=%v", parsed, err)
	}
	if _, err = ParseCallerAuthorities("vendor-service=COMMERCE"); err == nil {
		t.Fatal("an unknown authority was accepted")
	}
}
