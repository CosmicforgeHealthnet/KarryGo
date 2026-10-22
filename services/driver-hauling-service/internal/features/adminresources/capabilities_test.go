package adminresources

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminowner"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"github.com/jackc/pgx/v5"
)

func TestHaulingTruckCapabilitiesPreserveUnknownAndRequireNewContract(t *testing.T) {
	db := fixture.Database(t, "../../../migrations")
	ctx := context.Background()
	const provider = "11111111-1111-4111-8111-111111111111"
	const truck = "22222222-2222-4222-8222-222222222222"
	if _, err := db.Exec(ctx, `INSERT INTO truck_providers(id,first_name) VALUES($1,'PRIVATE_NAME')`, provider); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO trucks(id,provider_id,truck_type,capacity_kg,plate_number,inspection_ref,certification_ref) VALUES($1,$2,'flatbed',2000,'PRIVATE_PLATE','https://private.example/inspection','PRIVATE_CERTIFICATION')`, truck, provider); err != nil {
		t.Fatal(err)
	}
	source := Source()
	read := func() adminowner.Resource {
		t.Helper()
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		resource, err := source.Read(ctx, tx, "trucks", truck, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := adminowner.ValidateResource(resource, source.Specs["trucks"].Definition, true); err != nil {
			t.Fatal(err)
		}
		return resource
	}
	initial := read()
	for _, dim := range initial.Dimensions {
		if dim.Flag != nil || dim.Value != nil {
			t.Fatalf("unrecorded capability %s became known", dim.Name)
		}
	}
	for _, c := range initial.Counters {
		if c.Name == "capacity_kg" {
			if c.Value == nil || *c.Value != 2000 {
				t.Fatal(c)
			}
		} else if c.Name != "capability_version" && c.Name != "capability_source_version" && c.Value != nil {
			t.Fatalf("unrecorded physical fact became known: %s", c.Name)
		}
	}
	if initial.SourceVersion != nil {
		t.Fatal("partial capability revision became full truck mutation version")
	}
	if _, err := db.Exec(ctx, `UPDATE trucks SET max_payload_grams=1900000,internal_length_mm=5500,enclosed=false,fragile_safe=true,capability_version=2,capability_source_version=7,updated_at=now() WHERE id=$1`, truck); err != nil {
		t.Fatal(err)
	}
	recorded := read()
	for _, d := range recorded.Dimensions {
		switch d.Name {
		case "enclosed":
			if d.Flag == nil || *d.Flag {
				t.Fatal("explicit false lost")
			}
		case "fragile_safe":
			if d.Flag == nil || !*d.Flag {
				t.Fatal("explicit true lost")
			}
		case "eligibility", "inspection", "refrigerated":
			if d.Flag != nil || d.Value != nil {
				t.Fatal("suitability/compliance inferred")
			}
		}
	}
	for _, c := range recorded.Counters {
		if c.Name == "max_payload_grams" && (c.Value == nil || *c.Value != 1900000) {
			t.Fatal("stored payload was replaced by legacy capacity")
		}
	}
	captureHaulingResource(t, "hauling-capability-resource.json", recorded)
	raw, _ := json.Marshal(recorded)
	if strings.Contains(string(raw), "PRIVATE_") || strings.Contains(string(raw), "private.example") {
		t.Fatal("private asset identity/document reference leaked")
	}
	if initial.SnapshotETag == recorded.SnapshotETag {
		t.Fatal("physical changes missing from read fingerprint")
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := source.Binding(ctx, tx, "trucks", truck)
	tx.Rollback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if binding.SourceContract != "hauling.admin-truck-capability.v2" {
		t.Fatal("larger disclosure reused v1 contract")
	}
	issuer := contextfixture.New(t, Owner)
	store := admincontext.Store{DB: db, Reader: source, Authority: issuer.Authority}
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "org-1", Tenant: "tenant-1"}
	approval := issuer.Approval("truck-capability-context", "trucks", truck, "hauling.trucks.read", binding, scope)
	old := approval
	old.SourceContract = "hauling.admin-resources.v1"
	if err := store.Import(ctx, issuer.Sign(t, old), time.Now()); err == nil {
		t.Fatal("old v1 approval authorized physical-capability disclosure")
	}
	if err := store.Import(ctx, issuer.Sign(t, approval), time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestHaulingRecoveryReadsBindActualPlanningTenantAndMission(t *testing.T) {
	db := fixture.Database(t, "../../../migrations")
	ctx := context.Background()
	const booking = "11111111-1111-4111-8111-111111111111"
	const recovery = "22222222-2222-4222-8222-222222222222"
	if _, err := db.Exec(ctx, `INSERT INTO haulage_bookings(id,customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,cargo_type,cargo_weight_kg,corporate_organization_id,settlement_organization_id) VALUES($1,gen_random_uuid(),'PRIVATE_PICKUP',0,0,'PRIVATE_DROP',0,0,'general',100,'planning-org','aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa')`, booking); err != nil {
		t.Fatal(err)
	}
	latest := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	audit, _ := json.Marshal([]map[string]any{{"at": latest, "state": "IN_PROGRESS", "actor": "PRIVATE_ACTOR", "reason": "PRIVATE_NOTE"}})
	if _, err := db.Exec(ctx, `INSERT INTO hauling_recovery_cases(id,booking_id,trigger_code,state,operation_ref,version,detected_at,audit_json,custody_transfer_required,custody_transfer_ref,evidence_ref,reason_code) VALUES($1,$2,'PROVIDER_BREAKDOWN','IN_PROGRESS','PRIVATE_OPERATION',7,now()-interval '1 hour',$3,true,'PRIVATE_TRANSFER','PRIVATE_EVIDENCE','PRIVATE_REASON')`, recovery, booking, audit); err != nil {
		t.Fatal(err)
	}
	source := Source()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := source.Binding(ctx, tx, "recoveries", recovery)
	if err != nil {
		t.Fatal(err)
	}
	item, err := source.Read(ctx, tx, "recoveries", recovery, true)
	tx.Rollback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := adminowner.ValidateResource(item, source.Specs["recoveries"].Definition, true); err != nil {
		t.Fatal(err)
	}
	if item.SourceVersion == nil || *item.SourceVersion != 7 || !item.UpdatedAt.Equal(latest) {
		t.Fatal("native case revision/audit timestamp lost")
	}
	if binding.Canonical.Organization != "planning-org" || binding.Canonical.Tenant != "planning-org" {
		t.Fatal("settlement identity became authorization")
	}
	t.Run("case-evidence", func(t *testing.T) { fixture.CaseBoundary(t, db, source, Owner, "recoveries", recovery, true) })
	captureHaulingResource(t, "hauling-recovery-resource.json", item)
	raw, _ := json.Marshal(item)
	if strings.Contains(string(raw), "PRIVATE_") {
		t.Fatal("recovery private audit/evidence leaked")
	}
	issuer := contextfixture.New(t, Owner)
	store := admincontext.Store{DB: db, Reader: source, Authority: issuer.Authority}
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "planning-org", Tenant: "planning-org"}
	approval := issuer.Approval("hauling-recovery-context", "recoveries", recovery, "hauling.recovery.read", binding, scope)
	if err := store.Import(ctx, issuer.Sign(t, approval), time.Now()); err != nil {
		t.Fatal(err)
	}
	// Settlement bookkeeping cannot change the authority of an approved case.
	if _, err := db.Exec(ctx, `UPDATE haulage_bookings SET settlement_organization_id='bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb' WHERE id=$1`, booking); err != nil {
		t.Fatal(err)
	}
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve(ctx, tx, "recoveries", recovery, "hauling.recovery.read", scope, time.Now()); err != nil {
		t.Fatal(err)
	}
	tx.Rollback(ctx)
	// A declared mission must actually belong to this booking; no arbitrary
	// commerce reference, even in an owner-signed context, establishes a link.
	if _, err := db.Exec(ctx, `UPDATE hauling_recovery_cases SET mission_ref='foreign-mission' WHERE id=$1`, recovery); err != nil {
		t.Fatal(err)
	}
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = source.Read(ctx, tx, "recoveries", recovery, true)
	tx.Rollback(ctx)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("foreign mission accepted: %v", err)
	}
	if _, err := db.Exec(ctx, `UPDATE hauling_recovery_cases SET mission_ref='' WHERE id=$1`, recovery); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE haulage_bookings SET winner_organization_id='bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb' WHERE id=$1`, booking); err != nil {
		t.Fatal(err)
	}
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := store.Resolve(ctx, tx, "recoveries", recovery, "hauling.recovery.read", scope, time.Now()); !errors.Is(err, admincontext.ErrUnmapped) {
		t.Fatalf("conflicting planning owners accepted: %v", err)
	}
}

func captureHaulingResource(t *testing.T, filename string, resource adminowner.Resource) {
	t.Helper()
	if dir := os.Getenv("ADMIN_OWNER_CUSTODY_CAPTURE_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(resource)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, filename), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
