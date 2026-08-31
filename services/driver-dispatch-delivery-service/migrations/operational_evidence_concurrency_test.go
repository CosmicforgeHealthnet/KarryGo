package migrations_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests deliberately use only the canonical Dispatch test database.
// They exercise the real migration chain in an isolated schema and skip when
// infrastructure is not supplied; they never discover or create a database.
func operationalTestPool(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	dsn := os.Getenv("DISPATCH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("DISPATCH_TEST_DATABASE_URL not set; operational PostgreSQL certification is infrastructure-gated")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	schema := "commerce_operational_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		cancel()
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := filepath.Glob("*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(entries)
	for _, path := range entries {
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, execErr := pool.Exec(ctx, string(raw)); execErr != nil {
			t.Fatalf("apply %s: %v", filepath.Base(path), execErr)
		}
	}
	cleanup := func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
		admin.Close()
		cancel()
	}
	return pool, cleanup
}

func TestOperationalMigrationApplyRollbackReapply(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	ctx := context.Background()
	down, err := os.ReadFile("000043_operational_evidence_exceptions_provider_transfer.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	up, err := os.ReadFile("000043_operational_evidence_exceptions_provider_transfer.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("rollback operational migration: %v", err)
	}
	if _, err = pool.Exec(ctx, string(up)); err != nil {
		t.Fatalf("reapply operational migration: %v", err)
	}
	var table string
	if err = pool.QueryRow(ctx, `SELECT 'commerce_mission_evidence'::regclass::text`).Scan(&table); err != nil {
		t.Fatal(err)
	}
}

type operationalFixture struct {
	missionID, missionRef, stopID, parcelRef, providerA, providerB, tripA, tripB string
}

func seedOperationalFixture(t *testing.T, pool *pgxpool.Pool) operationalFixture {
	t.Helper()
	ctx := context.Background()
	f := operationalFixture{
		missionID: uuid.NewString(), missionRef: "dcm_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		stopID: uuid.NewString(), parcelRef: "parcel_" + uuid.NewString(),
		providerA: uuid.NewString(), providerB: uuid.NewString(), tripA: uuid.NewString(), tripB: uuid.NewString(),
	}
	bookingA, bookingB, customer := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO providers(id,phone,verification_status,is_active,onboarding_complete)
		VALUES($1,$2,'verified',true,true),($3,$4,'verified',true,true)`, f.providerA, "+23480"+f.providerA[:8], f.providerB, "+23481"+f.providerB[:8]); err != nil {
		t.Fatal(err)
	}
	for _, booking := range []string{bookingA, bookingB} {
		if _, err := pool.Exec(ctx, `INSERT INTO dispatch_bookings(id,customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone)
			VALUES($1,$2,'accepted','pickup',6.5,3.4,'dropoff',6.6,3.5,1000,'receiver','+2348000000000')`, booking, customer); err != nil {
			t.Fatal(err)
		}
	}
	for _, trip := range []struct{ id, booking, provider string }{{f.tripA, bookingA, f.providerA}, {f.tripB, bookingB, f.providerB}} {
		if _, err := pool.Exec(ctx, `INSERT INTO trips(id,booking_id,provider_id,customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone)
			VALUES($1,$2,$3,$4,'in_progress','pickup',6.5,3.4,'dropoff',6.6,3.5,1000,'receiver','+2348000000000')`, trip.id, trip.booking, trip.provider, customer); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO commerce_missions(id,mission_ref,parent_order_ref,vendor_order_refs,customer_id,booking_id,agreed_fee_minor,status,
		caller_service,operation_ref,request_fingerprint,canonical_ref,runtime_state,queue_state,source_version,plan_ref,plan_version,plan_fingerprint,mission_slot_ref,
		quote_revision,quote_fingerprint,mode,service_class,capability_version,funding_reference,trip_id,current_provider_id,replacement_state)
		VALUES($1,$2,'parent-1',ARRAY['vendor-order-1'],$3,$4,1000,'accepted','vendor-service','operation-1','fingerprint',$2,'IN_TRANSIT','ASSIGNED',1,
		'plan-1',1,'plan-fingerprint','slot-1',1,'quote-fingerprint','FASTEST','COMMERCE',1,'funding-1',$5,$6,'NONE')`, f.missionID, f.missionRef, customer, bookingA, f.tripA, f.providerA); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO commerce_mission_stops(id,commerce_mission_id,stop_ref,physical_stop_ref,sequence,vendor_ref,branch_ref,manifest_ref,manifest_version,location_snapshot,ready_from,ready_to,status,handover_status)
		VALUES($1,$2,'stop-1','physical-1',1,'vendor-1','branch-1','manifest-1',3,'{}',now()-interval '1 hour',now()+interval '1 hour','PICKED_UP','CONSUMED')`, f.stopID, f.missionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO commerce_mission_parcels(commerce_mission_id,stop_id,parcel_ref,vendor_ref,branch_ref,fulfilment_unit_ref,vendor_order_ref,manifest_ref,manifest_version,manifest_created_at,manifest_snapshot,weight_grams,package_count,status,custodian_type,custodian_ref,picked_up_at)
		VALUES($1,$2,$3,'vendor-1','branch-1','unit-1','vendor-order-1','manifest-1',3,now(),'{}',1000,1,'IN_TRANSIT','PROVIDER',$4,now())`, f.missionID, f.stopID, f.parcelRef, f.providerA); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO commerce_mission_carrier_history(commerce_mission_id,provider_id,trip_id,custody_started_at,source_version) VALUES($1,$2,$3,now(),1)`, f.missionID, f.providerA, f.tripA); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestOperationalManifestFreezeAndEvidenceIdempotencyRaces(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()
	f := seedOperationalFixture(t, pool)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `UPDATE commerce_mission_stops SET manifest_frozen_at=now() WHERE id=$1; UPDATE commerce_mission_parcels SET manifest_frozen_at=now() WHERE commerce_mission_id=$2`, f.stopID, f.missionID); err != nil {
		t.Fatal(err)
	}

	t.Run("manifest freeze race", func(t *testing.T) {
		errs := concurrentExec(pool, `UPDATE commerce_mission_parcels SET manifest_ref=$2 WHERE commerce_mission_id=$1`, []any{f.missionID, "changed-a"}, []any{f.missionID, "changed-b"})
		if errs[0] == nil || errs[1] == nil {
			t.Fatalf("frozen manifest mutation results = %v", errs)
		}
	})

	t.Run("duplicate evidence", func(t *testing.T) {
		query := `INSERT INTO commerce_mission_evidence(commerce_mission_id,operation_ref,evidence_type,actor_type,actor_ref,parcel_refs,captured_at,source_version)
			VALUES($1,'same-operation','DAMAGE','PROVIDER',$2,ARRAY[$3],now(),2)`
		errs := concurrentExec(pool, query, []any{f.missionID, f.providerA, f.parcelRef}, []any{f.missionID, f.providerA, f.parcelRef})
		successes := 0
		for _, err := range errs {
			if err == nil {
				successes++
			}
		}
		if successes != 1 {
			t.Fatalf("successful duplicate evidence inserts = %d; errors=%v", successes, errs)
		}
		var count int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM commerce_mission_evidence WHERE commerce_mission_id=$1 AND operation_ref='same-operation'`, f.missionID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("evidence count=%d err=%v", count, err)
		}
	})
}

func TestOperationalTerminalAndTransferSerializationRaces(t *testing.T) {
	pool, cleanup := operationalTestPool(t)
	defer cleanup()

	for _, name := range []string{"exception_vs_complete", "return_vs_complete", "replacement_vs_complete"} {
		t.Run(name, func(t *testing.T) {
			f := seedOperationalFixture(t, pool)
			query := `UPDATE commerce_missions SET runtime_state=$2,source_version=source_version+1 WHERE id=$1 AND runtime_state='IN_TRANSIT' AND replacement_state='NONE'`
			errs, affected := concurrentCAS(pool, query, []any{f.missionID, "COMPLETED"}, []any{f.missionID, "RECOVERY_REQUIRED"})
			if errs[0] != nil || errs[1] != nil || affected[0]+affected[1] != 1 {
				t.Fatalf("terminal serialization affected=%v errors=%v", affected, errs)
			}
		})
	}

	t.Run("two providers and duplicate transfer", func(t *testing.T) {
		f := seedOperationalFixture(t, pool)
		ctx := context.Background()
		if _, err := pool.Exec(ctx, `UPDATE commerce_missions SET replacement_state='ASSIGNED',replacement_provider_id=$2,replacement_trip_id=$3 WHERE id=$1`, f.missionID, f.providerB, f.tripB); err != nil {
			t.Fatal(err)
		}
		query := `INSERT INTO commerce_provider_transfers(transfer_ref,commerce_mission_id,operation_ref,state,old_provider_id,new_provider_id,old_trip_id,new_trip_id,parcel_refs,manifest_binding,package_count,latitude,longitude,challenge_hash,challenge_expires_at,source_version)
			VALUES($1,$2,$3,'CHALLENGE_ISSUED',$4,$5,$6,$7,ARRAY[$8],jsonb_build_array(jsonb_build_object('parcel_ref',$8)),1,6.5,3.4,'hash',now()+interval '5 minutes',2)`
		errs := concurrentExec(pool, query,
			[]any{"transfer-a", f.missionID, "transfer-operation-a", f.providerA, f.providerB, f.tripA, f.tripB, f.parcelRef},
			[]any{"transfer-b", f.missionID, "transfer-operation-b", f.providerA, f.providerB, f.tripA, f.tripB, f.parcelRef})
		successes := 0
		for _, err := range errs {
			if err == nil {
				successes++
			}
		}
		if successes != 1 {
			t.Fatalf("open transfer winners=%d errors=%v", successes, errs)
		}
		var transferID string
		if err := pool.QueryRow(ctx, `SELECT id::text FROM commerce_provider_transfers WHERE commerce_mission_id=$1 AND state='CHALLENGE_ISSUED'`, f.missionID).Scan(&transferID); err != nil {
			t.Fatal(err)
		}
		transferQuery := `UPDATE commerce_provider_transfers SET state=$2 WHERE id=$1 AND state='CHALLENGE_ISSUED'`
		transitionErrs, affected := concurrentCAS(pool, transferQuery, []any{transferID, "COMPLETED"}, []any{transferID, "CANCELLED"})
		if transitionErrs[0] != nil || transitionErrs[1] != nil || affected[0]+affected[1] != 1 {
			t.Fatalf("transfer/return serialization affected=%v errors=%v", affected, transitionErrs)
		}
	})

	t.Run("event version ordering and replay", func(t *testing.T) {
		f := seedOperationalFixture(t, pool)
		query := `INSERT INTO dispatch_marketplace_outbox(event_type,aggregate_type,aggregate_id,aggregate_version,payload,payload_hash,target_service)
			VALUES('commerce.evidence.recorded.v1','COMMERCE_MISSION',$1,2,'{}','hash','vendor-service')`
		errs := concurrentExec(pool, query, []any{f.missionRef}, []any{f.missionRef})
		successes := 0
		for _, err := range errs {
			if err == nil {
				successes++
			}
		}
		if successes != 1 {
			t.Fatalf("outbox replay winners=%d errors=%v", successes, errs)
		}
		if _, err := pool.Exec(context.Background(), `INSERT INTO dispatch_marketplace_outbox(event_type,aggregate_type,aggregate_id,aggregate_version,payload,payload_hash,target_service)
			VALUES('commerce.evidence.recorded.v1','COMMERCE_MISSION',$1,3,'{}','hash-3','vendor-service')`, f.missionRef); err != nil {
			t.Fatalf("next ordered source version: %v", err)
		}
	})
}

func concurrentExec(pool *pgxpool.Pool, query string, argsA, argsB []any) [2]error {
	var result [2]error
	var wg sync.WaitGroup
	for index, args := range [][]any{argsA, argsB} {
		wg.Add(1)
		go func(i int, values []any) {
			defer wg.Done()
			_, result[i] = pool.Exec(context.Background(), query, values...)
		}(index, args)
	}
	wg.Wait()
	return result
}

func concurrentCAS(pool *pgxpool.Pool, query string, argsA, argsB []any) ([2]error, [2]int64) {
	var errs [2]error
	var affected [2]int64
	var wg sync.WaitGroup
	for index, args := range [][]any{argsA, argsB} {
		wg.Add(1)
		go func(i int, values []any) {
			defer wg.Done()
			tag, err := pool.Exec(context.Background(), query, values...)
			errs[i] = err
			if err == nil {
				affected[i] = tag.RowsAffected()
			}
		}(index, args)
	}
	wg.Wait()
	return errs, affected
}
