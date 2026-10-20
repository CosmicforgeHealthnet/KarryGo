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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestTaxiRecoveryReadsBindNativeParentsAndNeverInferGoodsScope(t *testing.T) {
	db := fixture.Database(t, "../../../migrations")
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	market, zone, otherMarket, otherZone := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, id := range []string{market, otherMarket} {
		exec(`INSERT INTO markets(id,country,name,code) VALUES($1,'TEST','PRIVATE_NAME',$2)`, id, id)
	}
	exec(`INSERT INTO operating_zones(id,market_id,name,code,center_lat,center_lng) VALUES($1,$2,'PRIVATE_ZONE','Z1',10,20),($3,$4,'PRIVATE_ZONE','Z2',30,40)`, zone, market, otherZone, otherMarket)
	booking, quote, goods, passengerCase, goodsCase := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec(`INSERT INTO taxi_bookings(id,customer_id,market_id,operating_zone_id,organization_id,is_corporate_managed,membership_id,fleet_assignment_id,status,source_context,ride_notes) VALUES($1,$2,$3,$4,'org-1',true,'PRIVATE_MEMBERSHIP','PRIVATE_ASSIGNMENT','completed','FLEET_ASSIGNMENT','PRIVATE_NOTES')`, booking, uuid.NewString(), market, zone)
	exec(`INSERT INTO taxi_goods_quotes(id,source_service,quote_reference,revision,request_fingerprint,request_payload,serviceable,reason_code,market_id,operating_zone_id,currency,expires_at) VALUES($1,'vendor-service','PRIVATE_QUOTE',1,'PRIVATE_HASH','{}',true,'SERVICEABLE',$2,$3,'NGN',now()+interval '15 minutes')`, quote, otherMarket, otherZone)
	exec(`INSERT INTO taxi_goods_missions(id,source_service,mission_ref,operation_ref,request_fingerprint,parent_order_ref,vendor_order_refs,customer_ref,quote_id,pickup,dropoff,load,agreed_fee_minor,currency,requested_at,receiver_name,receiver_phone,pickup_evidence_ref,delivery_evidence_ref) VALUES($1,'vendor-service','PRIVATE_MISSION','PRIVATE_OPERATION','PRIVATE_HASH','PRIVATE_ORDER','{PRIVATE_VENDOR}','PRIVATE_CUSTOMER',$2,'{"latitude":51}','{"longitude":10}','{}',1000,'NGN',now(),'PRIVATE_NAME','PRIVATE_PHONE','PRIVATE_MEDIA','PRIVATE_MEDIA')`, goods, quote)
	detected := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	auditAt := detected.Add(time.Minute)
	resolved := auditAt.Add(time.Minute)
	audit, _ := json.Marshal([]map[string]any{{"at": auditAt, "state": "IN_PROGRESS", "operator": "PRIVATE_STAFF", "reason_code": "PRIVATE_REASON", "evidence_ref": "PRIVATE_MEDIA"}})
	exec(`INSERT INTO taxi_recovery_cases(id,subject_type,booking_id,trigger_code,state,operation_ref,detected_at,resolved_at,version,audit_json) VALUES($1,'BOOKING',$2,'PROVIDER_DELAY','RESOLVED','PRIVATE_PASSENGER_OP',$3,$4,7,$5)`, passengerCase, booking, detected, resolved, audit)
	exec(`INSERT INTO taxi_recovery_cases(id,subject_type,goods_mission_id,trigger_code,state,operation_ref,detected_at,version,audit_json,custody_transfer_required,custody_transfer_ref) VALUES($1,'GOODS_MISSION',$2,'PROVIDER_UNAVAILABLE','TRANSFER_REQUIRED','PRIVATE_GOODS_OP',$3,4,$4,true,'PRIVATE_TRANSFER')`, goodsCase, goods, detected, audit)
	source := Source()
	binding := func(id string) admincontext.RecordBinding {
		t.Helper()
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		out, err := source.Binding(ctx, tx, "recoveries", id)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	pb := binding(passengerCase)
	gb := binding(goodsCase)
	if pb.Canonical.Market != market || pb.Canonical.Zone != "Z1" || pb.Canonical.Organization != "org-1" || pb.Canonical.Tenant != "org-1" {
		t.Fatal("native booking scope lost", pb.Canonical)
	}
	if gb.Canonical != (adminpolicy.Scope{Product: "karryam"}) {
		t.Fatal("goods quote/GPS/vendor/customer inferred as authorization", gb.Canonical)
	}
	for _, id := range []string{passengerCase, goodsCase} {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		item, err := source.Read(ctx, tx, "recoveries", id, true)
		tx.Rollback(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := adminowner.ValidateResource(item, source.Specs["recoveries"].Definition, true); err != nil {
			t.Fatal(err)
		}
		version, at := int64(4), auditAt
		if id == passengerCase {
			version, at = 7, resolved
		}
		if item.SourceVersion == nil || *item.SourceVersion != version || !item.UpdatedAt.Equal(at) {
			t.Fatal("case revision/lifecycle timestamp lost", item)
		}
		raw, _ := json.Marshal(item)
		if strings.Contains(string(raw), "PRIVATE_") || len(item.Timeline) != 0 {
			t.Fatal("private recovery payload disclosed", string(raw))
		}
		if dir := os.Getenv("ADMIN_OWNER_CUSTODY_CAPTURE_DIR"); dir != "" {
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			name := "taxi-goods-recovery-resource.json"
			if id == passengerCase {
				name = "taxi-passenger-recovery-resource.json"
			}
			if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	issuer := contextfixture.New(t, Owner)
	store := &admincontext.Store{DB: db, Reader: source, Authority: issuer.Authority}
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "org-1", Tenant: "tenant-1"}
	approval := issuer.Approval("test-only-goods-recovery-allocation", "recoveries", goodsCase, "taxi.recovery.read", gb, scope)
	if err := store.Import(ctx, issuer.Sign(t, approval), time.Now()); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve(ctx, tx, "recoveries", goodsCase, "taxi.recovery.read", scope, time.Now()); err != nil {
		t.Fatal(err)
	}
	tx.Rollback(ctx)
	exec(`UPDATE taxi_goods_missions SET customer_ref='DIFFERENT_CUSTOMER' WHERE id=$1`, goods)
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve(ctx, tx, "recoveries", goodsCase, "taxi.recovery.read", scope, time.Now()); !errors.Is(err, admincontext.ErrUnmapped) {
		t.Fatal("old allocation survived changed native goods parent", err)
	}
	tx.Rollback(ctx)
	// The schema has separate FKs but no composite zone/market constraint. The
	// owner read contract must refuse a historically inconsistent parent.
	exec(`UPDATE taxi_bookings SET operating_zone_id=$2 WHERE id=$1`, booking, otherZone)
	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := source.Read(ctx, tx, "recoveries", passengerCase, true); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("cross-market booking zone accepted", err)
	}
}
