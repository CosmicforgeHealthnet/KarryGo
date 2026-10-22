package adminresources

import (
	"context"
	"cosmicforge/logistics/services/hauling-service/internal/cargofit"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminowner"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHaulingCargoInputsUseActualBookingWithoutInventingMeasurements(t *testing.T) {
	db := fixture.Database(t, "../../../migrations")
	ctx := context.Background()
	booking, org, otherOrg := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO haulage_bookings(id,customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,cargo_type,cargo_weight_kg,preferred_truck_type,is_fragile,corporate_organization_id,settlement_organization_id)VALUES($1,gen_random_uuid(),'PRIVATE_PICKUP',51,2,'PRIVATE_DROP',52,3,'PRIVATE_CARGO',0,'flatbed',false,$2,$3)`, booking, org, otherOrg); err != nil {
		t.Fatal(err)
	}
	source := Source()
	definition := source.Specs["booking-cargo-inputs"].Definition
	if definition.SourceContract != "hauling.admin-booking-cargo-inputs.v1" {
		t.Fatal(definition)
	}
	read := func(tx pgx.Tx) adminowner.Resource {
		t.Helper()
		item, err := source.Read(ctx, tx, definition.Kind, booking, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := adminowner.ValidateResource(item, definition, true); err != nil {
			t.Fatal(err)
		}
		return item
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	binding, err := source.Binding(ctx, tx, definition.Kind, booking)
	if err != nil {
		t.Fatal(err)
	}
	initial := read(tx)
	tx.Rollback(ctx)
	if binding.Canonical.Organization != org || binding.Canonical.Tenant != org || binding.Canonical.Market != "" || binding.Canonical.Zone != "" {
		t.Fatal("settlement/geography became authorization", binding.Canonical)
	}
	if initial.SourceVersion != nil || len(initial.Dimensions) != 2 || len(initial.Counters) != 2 || len(initial.References) != 0 {
		t.Fatal("invented cargo facts/version", initial)
	}
	for _, counter := range initial.Counters {
		if counter.Value != nil {
			t.Fatal("unstated weight became zero measured mass", counter)
		}
	}
	for _, d := range initial.Dimensions {
		if d.Name == "fragile_handling_required" && (d.Flag == nil || *d.Flag) {
			t.Fatal("actual explicit false handling input lost")
		}
	}
	issuer := contextfixture.New(t, Owner)
	store := &admincontext.Store{DB: db, Reader: source, Authority: issuer.Authority}
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: org, Tenant: org}
	allocation := issuer.Approval("booking-cargo-allocation", definition.Kind, booking, definition.Capability, binding, scope)
	bad := allocation
	bad.Scope.Organization = otherOrg
	bad.Scope.Tenant = otherOrg
	if err := store.Import(ctx, issuer.Sign(t, bad), time.Now()); err == nil {
		t.Fatal("settlement organization overrode planning owner")
	}
	if err := store.Import(ctx, issuer.Sign(t, allocation), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE haulage_bookings SET cargo_weight_kg=125,is_fragile=true,updated_at=now() WHERE id=$1`, booking); err != nil {
		t.Fatal(err)
	}
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	current := read(tx)
	if _, err := store.Resolve(ctx, tx, definition.Kind, booking, definition.Capability, scope, time.Now()); err != nil {
		t.Fatal("ordinary current input update invented new ownership", err)
	}
	tx.Rollback(ctx)
	counts := map[string]int64{}
	for _, c := range current.Counters {
		if c.Value == nil {
			t.Fatal("recorded weight disappeared")
		}
		counts[c.Name] = *c.Value
	}
	if counts["recorded_weight_kg"] != 125 || counts["normalized_weight_grams"] != cargofit.FromKilograms(125) || current.SnapshotETag == initial.SnapshotETag {
		t.Fatal("current native cargo input/unit normalization lost", counts)
	}
	raw, _ := json.Marshal(current)
	if strings.Contains(string(raw), "PRIVATE_") || strings.Contains(string(raw), "latitude") || strings.Contains(string(raw), "temperature") {
		t.Fatal("private/invented cargo payload disclosed", string(raw))
	}
	profile, _ := adminowner.ForOwner(Owner)
	policy := policyfixture.New(t, scope, Owner, definition.Capability, definition.SourceContract)
	key := []byte(strings.Repeat("c", 32))
	h := &adminowner.Handler{Owner: Owner, Path: profile.Path, DB: db, Source: source, Contexts: store, Policy: policy.Engine, Secret: key, Enabled: true, Definitions: profile.Definitions}
	router := gin.New()
	h.Register(router)
	q := adminread.Query{Scope: adminread.Scope{Market: scope.Market, Zone: scope.Zone, Organization: scope.Organization, Tenant: scope.Tenant}, Staff: "staff-1", Role: "staff", Purpose: "case-1", PolicyID: policy.Engine.ID(), PolicyVersion: policy.Engine.Version(), PolicyChecksum: policy.Engine.Checksum(), Start: time.Now().Add(-time.Hour), End: time.Now().Add(time.Second), Limit: 10}
	request := httptest.NewRequest("GET", profile.Path+"/resources/"+definition.Kind+"/"+booking+"?"+q.Values().Encode(), nil)
	if err := serviceauth.SignRequest(request, adminread.Caller, key, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 200 || strings.Contains(response.Body.String(), "PRIVATE_") {
		t.Fatal("native cargo input wire", response.Code, response.Body.String())
	}
	if dir := os.Getenv("ADMIN_OWNER_CUSTODY_CAPTURE_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "hauling-booking-cargo-inputs.json"), response.Body.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, `UPDATE haulage_bookings SET winner_organization_id=$2 WHERE id=$1`, booking, otherOrg); err != nil {
		t.Fatal(err)
	}
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := source.Read(ctx, tx, definition.Kind, booking, true); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("conflicting native planning/winner organization exposed cargo", err)
	}
	if _, err := store.Resolve(ctx, tx, definition.Kind, booking, definition.Capability, scope, time.Now()); !errors.Is(err, admincontext.ErrUnmapped) {
		t.Fatal("old allocation survived conflicting owner", err)
	}
}
