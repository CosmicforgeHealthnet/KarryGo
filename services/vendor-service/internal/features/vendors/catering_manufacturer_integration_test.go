package vendors

// Catering and Manufacturer onboarding, against real PostgreSQL.
//
// The readiness engine has modelled both categories since migration 000117,
// but a business could not hold the capability, so the models described work
// nobody could start. Migration 000121 admits them. These tests prove the
// whole onboarding path -- business, capability, branch, branch capability --
// works for both on exactly the same terms as the seven original categories,
// and that widening the vocabulary did not widen it to anything else.

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/audit"
	"cosmicforge/logistics/services/vendor-service/internal/database"
	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
)

func capabilityVocabularyService(t *testing.T) (*Service, *pgxpool.Pool, context.Context) {
	t.Helper()
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres capability vocabulary test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err = database.ApplyMigrations(ctx, pool, "../../../migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewService(pool, allowAll{}, integrationAudit{recorder: audit.NewPostgresRecorder(pool)},
		events.NewDomainPublisher(events.NewOutboxRepository(pool))), pool, ctx
}

// onboard runs the real path a new vendor takes: staff, business, capability,
// branch, branch capability. It returns the branch capability so a caller can
// assert what the vendor ended up holding.
func onboard(t *testing.T, svc *Service, pool *pgxpool.Pool, ctx context.Context,
	suffix string, capability CapabilityType) (Capability, Branch, BranchCapability) {
	t.Helper()
	// Unique per run: these tests share whatever database they are given, and
	// a fixed id would make a rerun collide with its own previous fixture.
	suffix += "_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:8]
	vendorID, staffID, userID := "vendor_"+suffix, "vs_"+suffix, "vu_"+suffix
	t.Cleanup(func() {
		clean, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = pool.Exec(clean, `DELETE FROM vendor_businesses WHERE id=$1`, vendorID)
		_, _ = pool.Exec(clean, `DELETE FROM vendor_staff_role_assignments WHERE staff_id=$1`, staffID)
		_, _ = pool.Exec(clean, `DELETE FROM vendor_staff WHERE id=$1`, staffID)
		_, _ = pool.Exec(clean, `DELETE FROM vendor_users WHERE id=$1`, userID)
	})
	if _, err := pool.Exec(ctx, `INSERT INTO vendor_users(id,email) VALUES($1,$2)`, userID, suffix+"@example.test"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO vendor_staff(id,vendor_id,user_id,status) VALUES($1,$2,$3,'active')`,
		staffID, vendorID, userID); err != nil {
		t.Fatalf("seed staff: %v", err)
	}
	actor := Actor{StaffID: staffID, VendorID: vendorID}
	business, err := svc.CreateBusiness(ctx, actor, Business{LegalName: "Karry " + suffix + " Limited",
		TradingName: "Karry " + suffix, Currency: "NGN", Timezone: "Africa/Lagos"})
	if err != nil {
		t.Fatalf("create business: %v", err)
	}
	held, err := svc.CreateCapability(ctx, actor, business.ID, capability)
	if err != nil {
		t.Fatalf("create %s capability: %v", capability, err)
	}
	branch, err := svc.CreateBranch(ctx, actor, business.ID, Branch{DisplayName: "Karry " + suffix + " Branch",
		Address:  Address{Line1: "1 Marina Road", City: "Lagos", Region: "Lagos", CountryCode: "NG"},
		Latitude: 6.45, Longitude: 3.39, Timezone: "Africa/Lagos"})
	if err != nil {
		t.Fatalf("create branch: %v", err)
	}
	branchCapability, err := svc.PutBranchCapability(ctx, actor, business.ID, branch.ID, held.ID, 0)
	if err != nil {
		t.Fatalf("attach %s to branch: %v", capability, err)
	}
	return held, branch, branchCapability
}

func TestCateringCapabilityCanBeOnboarded(t *testing.T) {
	svc, pool, ctx := capabilityVocabularyService(t)
	capability, branch, branchCapability := onboard(t, svc, pool, ctx, "catering", CapabilityCatering)

	if capability.CapabilityType != CapabilityCatering {
		t.Fatalf("capability type = %q, want CATERING", capability.CapabilityType)
	}
	if branchCapability.VendorCapabilityID != capability.ID || branchCapability.BranchID != branch.ID {
		t.Fatalf("branch capability not bound: %+v", branchCapability)
	}
	// It reads back from the database as itself, not as something coerced.
	var stored string
	if err := pool.QueryRow(ctx, `SELECT capability_type FROM vendor_capabilities WHERE id=$1`, capability.ID).Scan(&stored); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if stored != "CATERING" {
		t.Fatalf("stored capability = %q, want CATERING", stored)
	}
	if family := CapabilityFamily(CapabilityCatering); family != "FOOD" {
		t.Fatalf("catering family = %q, want FOOD", family)
	}
}

func TestManufacturerCapabilityCanBeOnboarded(t *testing.T) {
	svc, pool, ctx := capabilityVocabularyService(t)
	capability, branch, branchCapability := onboard(t, svc, pool, ctx, "manufacturer", CapabilityManufacturer)

	if capability.CapabilityType != CapabilityManufacturer {
		t.Fatalf("capability type = %q, want MANUFACTURER", capability.CapabilityType)
	}
	if branchCapability.VendorCapabilityID != capability.ID || branchCapability.BranchID != branch.ID {
		t.Fatalf("branch capability not bound: %+v", branchCapability)
	}
	var stored string
	if err := pool.QueryRow(ctx, `SELECT capability_type FROM vendor_capabilities WHERE id=$1`, capability.ID).Scan(&stored); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if stored != "MANUFACTURER" {
		t.Fatalf("stored capability = %q, want MANUFACTURER", stored)
	}
	if family := CapabilityFamily(CapabilityManufacturer); family != "GOODS" {
		t.Fatalf("manufacturer family = %q, want GOODS", family)
	}
}

// Every original category still onboards, including the Warehouse business
// model, which the platform expresses as WHOLESALE and which this change does
// not rename or re-map.
func TestExistingVendorTypesRemainValid(t *testing.T) {
	svc, pool, ctx := capabilityVocabularyService(t)
	for _, capability := range []CapabilityType{
		CapabilityRestaurant, CapabilityFastFood, CapabilityCloudKitchen, CapabilityPastryBakery,
		CapabilityShopGrocery, CapabilityMarket, CapabilityWholesale,
	} {
		held, _, _ := onboard(t, svc, pool, ctx, "legacy_"+string(capability), capability)
		if held.CapabilityType != capability {
			t.Errorf("capability type = %q, want %q", held.CapabilityType, capability)
		}
	}
}

func TestWarehouseWholesaleMappingUnchanged(t *testing.T) {
	// Warehouse is the WHOLESALE capability, and stays so: the two new
	// categories are additions beside it, never a re-mapping of it.
	if CapabilityFamily(CapabilityWholesale) != "SHOP" {
		t.Fatalf("wholesale family changed: %q", CapabilityFamily(CapabilityWholesale))
	}
	if !IsSupportedCapability(CapabilityWholesale) {
		t.Fatal("WHOLESALE is no longer supported")
	}
	for _, added := range []CapabilityType{CapabilityCatering, CapabilityManufacturer} {
		if added == CapabilityWholesale {
			t.Fatalf("%s collides with WHOLESALE", added)
		}
	}
}

func TestUnknownCapabilityTypeStillRejected(t *testing.T) {
	svc, pool, ctx := capabilityVocabularyService(t)
	capability, _, _ := onboard(t, svc, pool, ctx, "reject", CapabilityRestaurant)
	actor := Actor{StaffID: "vs_" + capability.BusinessID[len("vendor_"):], VendorID: capability.BusinessID}
	for _, unsupported := range []CapabilityType{"PHARMACY", "CATERER", "BULK_FOOD", "FACTORY", "MANUFACTURING", "catering", ""} {
		if _, err := svc.CreateCapability(ctx, actor, capability.BusinessID, unsupported); err == nil {
			t.Errorf("capability %q was accepted", unsupported)
		}
	}
}

// The same thing over HTTP, through the real router: the onboarding API
// accepts both categories, still refuses anything else, and lists them.
func TestOnboardingAPIAcceptsCateringAndManufacturer(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set; skipping real-Postgres onboarding API test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = database.ApplyMigrations(ctx, pool, "../../../migrations"); err != nil {
		t.Fatal(err)
	}
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	vendorID, staffID, userID := "vendor_api_"+suffix, "vs_api_"+suffix, "vu_api_"+suffix
	t.Cleanup(func() {
		clean := context.Background()
		_, _ = pool.Exec(clean, `DELETE FROM vendor_businesses WHERE id=$1`, vendorID)
		_, _ = pool.Exec(clean, `DELETE FROM vendor_staff WHERE id=$1`, staffID)
		_, _ = pool.Exec(clean, `DELETE FROM vendor_users WHERE id=$1`, userID)
	})
	if _, err = pool.Exec(ctx, `INSERT INTO vendor_users(id,email) VALUES($1,$2)`, userID, userID+"@example.test"); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO vendor_staff(id,vendor_id,user_id,status) VALUES($1,$2,$3,'active')`,
		staffID, vendorID, userID); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/vendor")
	group.Use(func(c *gin.Context) {
		authctx.Set(c, authctx.StaffContext{StaffID: staffID, VendorID: vendorID, SessionID: "session"})
		c.Next()
	})
	RegisterRoutes(group, NewHandler(NewService(pool, allowAll{}, nil, nil), idempotency.NewStore(pool, time.Hour), true, false))

	created := requestSection6(t, router, http.MethodPost, "/api/v1/vendor/businesses",
		[]byte(`{"legal_name":"API Production Limited","currency":"NGN","timezone":"Africa/Lagos"}`),
		"business-"+suffix)
	if created.Code != http.StatusCreated {
		t.Fatalf("create business: %d %s", created.Code, created.Body.String())
	}
	path := "/api/v1/vendor/businesses/" + vendorID + "/capabilities"
	for _, capability := range []string{"CATERING", "MANUFACTURER"} {
		response := requestSection6(t, router, http.MethodPost, path,
			[]byte(`{"capability_type":"`+capability+`"}`), "cap-"+capability+"-"+suffix)
		if response.Code != http.StatusCreated {
			t.Fatalf("POST %s: %d %s", capability, response.Code, response.Body.String())
		}
		if !strings.Contains(response.Body.String(), capability) {
			t.Fatalf("%s is not in the response: %s", capability, response.Body.String())
		}
	}
	rejected := requestSection6(t, router, http.MethodPost, path,
		[]byte(`{"capability_type":"FACTORY"}`), "cap-bad-"+suffix)
	if rejected.Code < 400 {
		t.Fatalf("FACTORY was accepted: %d %s", rejected.Code, rejected.Body.String())
	}

	listed := requestSection6(t, router, http.MethodGet, "/api/v1/vendor/capability-types", nil, "")
	if listed.Code != http.StatusOK {
		t.Fatalf("capability-types: %d", listed.Code)
	}
	for _, capability := range []string{"CATERING", "MANUFACTURER", "WHOLESALE", "RESTAURANT"} {
		if !strings.Contains(listed.Body.String(), capability) {
			t.Fatalf("%s missing from the capability-types listing: %s", capability, listed.Body.String())
		}
	}
}
