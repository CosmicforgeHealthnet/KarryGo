package integration_test

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	mktmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	mktrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	mktusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	providermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// FLEET-6f: GET /marketplace/trips was offset-only. It now also pages by an
// opaque cursor on its existing order (updated_at DESC, id), and offset stays
// for compatibility.

func seedCursorTripMembership(t *testing.T, pool *pgxpool.Pool, organizationID, branchID string) string {
	t.Helper()
	providerID := "prv_tripcur_" + randomHex()
	membership := &providermodels.Membership{
		ProviderService: "driver-dispatch-delivery-service", ServiceCategory: providermodels.CategoryDispatch,
		OnboardingSource: providermodels.SourceSponsored, RelationshipType: providermodels.RelationshipEmployee,
		Status: providermodels.StatusActive, ProviderID: &providerID, HomeBranchID: &branchID,
	}
	if err := providerrepos.NewRepository(pool).CreateMembership(context.Background(), pool, organizationID, membership); err != nil {
		t.Fatal(err)
	}
	return membership.ID
}

func seedCursorTrip(t *testing.T, pool *pgxpool.Pool, organizationID string, membershipID *string) string {
	t.Helper()
	trip := &mktmodels.TripProjection{
		OrganizationID: organizationID, SourceService: "DISPATCH", PlatformTripID: "trip-cursor-" + randomHex(),
		ExecutingProviderID: "drv-cursor", OrganizationMembershipID: membershipID,
		ServiceType: "DISPATCH", TripStatus: "in_progress", SourceVersion: 1,
	}
	if _, err := mktrepos.NewTripRepository(pool).UpsertFromSource(context.Background(), pool, trip); err != nil {
		t.Fatal(err)
	}
	return trip.ID
}

// pageAllTrips follows nextCursor from the first page to the end and fails on
// any trip that appears twice.
func pageAllTrips(t *testing.T, service *mktusecases.Service, scope tenant.Context) map[string]bool {
	t.Helper()
	seen := map[string]bool{}
	next := ""
	for page := 0; page < 20; page++ {
		result, err := service.ListCorporateTrips(context.Background(), scope,
			mktusecases.CorporateTripFilter{Limit: 2, Cursor: next})
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		for _, item := range result.Items {
			if seen[item.TripID] {
				t.Fatalf("trip %s repeated across pages", item.TripID)
			}
			seen[item.TripID] = true
		}
		if result.HasMore != (result.NextCursor != "") {
			t.Fatalf("page %d: hasMore=%v but nextCursor=%q", page, result.HasMore, result.NextCursor)
		}
		if next != "" && result.NextOffset != 0 {
			t.Fatalf("cursor page %d carries an offset continuation %d", page, result.NextOffset)
		}
		next = result.NextCursor
		if next == "" {
			return seen
		}
	}
	t.Fatal("trip pages never ended")
	return nil
}

func TestMarketplaceTripsPageWithCursor(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Trip cursor "+randomHex()[:6], "RC-TRIPCUR-"+randomHex()[:8])
	other := createOrganization(t, pool, "Trip cursor other "+randomHex()[:6], "RC-TRIPCUR-"+randomHex()[:8])
	branchA := seedBranch(t, pool, org.ID, "Trip cursor A "+randomHex()[:6])
	branchB := seedBranch(t, pool, org.ID, "Trip cursor B "+randomHex()[:6])
	memberA := seedCursorTripMembership(t, pool, org.ID, branchA)
	memberB := seedCursorTripMembership(t, pool, org.ID, branchB)

	inA, all := map[string]bool{}, map[string]bool{}
	for i := 0; i < 5; i++ {
		id := seedCursorTrip(t, pool, org.ID, &memberA)
		inA[id], all[id] = true, true
	}
	for i := 0; i < 2; i++ {
		all[seedCursorTrip(t, pool, org.ID, &memberB)] = true
	}
	foreign := seedCursorTrip(t, pool, other.ID, nil)
	// Equal activity timestamps: only the id tie-breaker keeps pages disjoint.
	if _, err := pool.Exec(ctx, `UPDATE fleet_partner_trip_projections SET updated_at = '2026-03-01T00:00:00Z'
		WHERE organization_id = ANY($1::text[])`, []string{org.ID, other.ID}); err != nil {
		t.Fatal(err)
	}

	service := newOpportunityService(pool, true)
	orgWide := tenant.Context{OrganizationID: org.ID, OrganizationUserID: "oru_tripcur"}
	if got := pageAllTrips(t, service, orgWide); !reflect.DeepEqual(got, all) || got[foreign] {
		t.Fatalf("org-wide cursor pages = %v, want %v", got, all)
	}
	branchScoped := orgWide
	branchScoped.PermittedBranchIDs = []string{branchA}
	if got := pageAllTrips(t, service, branchScoped); !reflect.DeepEqual(got, inA) {
		t.Fatalf("branch-scoped cursor pages = %v, want only branch A trips %v", got, inA)
	}

	// Offset paging still works and agrees with the cursor order.
	first, err := service.ListCorporateTrips(ctx, orgWide, mktusecases.CorporateTripFilter{Limit: 3})
	if err != nil || len(first.Items) != 3 || first.NextOffset != 3 || first.NextCursor == "" {
		t.Fatalf("offset page 1 = %+v %v", first, err)
	}
	byOffset, err := service.ListCorporateTrips(ctx, orgWide, mktusecases.CorporateTripFilter{Limit: 3, Offset: 3})
	if err != nil {
		t.Fatal(err)
	}
	byCursor, err := service.ListCorporateTrips(ctx, orgWide, mktusecases.CorporateTripFilter{Limit: 3, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(byOffset.Items) != 3 || len(byCursor.Items) != 3 {
		t.Fatalf("page 2 sizes: offset %d cursor %d", len(byOffset.Items), len(byCursor.Items))
	}
	for i := range byOffset.Items {
		if byOffset.Items[i].TripID != byCursor.Items[i].TripID {
			t.Fatalf("offset and cursor page 2 differ at %d: %s vs %s", i, byOffset.Items[i].TripID, byCursor.Items[i].TripID)
		}
	}

	if _, err := service.ListCorporateTrips(ctx, orgWide, mktusecases.CorporateTripFilter{Cursor: "not-a-cursor"}); err == nil {
		t.Fatal("a malformed cursor must be refused")
	}
}

// TestMarketplaceTripsRefusesCursorWithOffset: page/limit stay only for
// compatibility, so a request naming both styles is ambiguous and refused.
func TestMarketplaceTripsRefusesCursorWithOffset(t *testing.T) {
	h := newConventionsHarness(t)
	code, body, raw := h.get(t, "/fleet-partner/v1/marketplace/trips?limit=2")
	if code != http.StatusOK {
		t.Fatalf("plain trip list = %d %s", code, raw)
	}
	if _, ok := body["data"].(map[string]any)["items"]; !ok {
		t.Fatalf("trip list data shape changed: %s", raw)
	}
	cursor := "eyJzIjoiMjAyNi0wMy0wMVQwMDowMDowMFoiLCJpZCI6InRycF94In0"
	if code, _, raw := h.get(t, "/fleet-partner/v1/marketplace/trips?cursor="+cursor); code != http.StatusOK {
		t.Fatalf("cursor trip list = %d %s", code, raw)
	}
	code, body, raw = h.get(t, "/fleet-partner/v1/marketplace/trips?cursor="+cursor+"&offset=0")
	if code != http.StatusUnprocessableEntity || !reflect.DeepEqual(validationFields(body), []string{"cursor"}) {
		t.Fatalf("cursor+offset = %d %s, want 422 naming cursor", code, raw)
	}
}
