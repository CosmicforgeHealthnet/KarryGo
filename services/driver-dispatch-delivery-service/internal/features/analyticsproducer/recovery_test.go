package analyticsproducer

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/deliveryrecovery"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"cosmicforge/logistics/shared/go/networkanalytics"
)

func insertIncident(t *testing.T, db *pgxpool.Pool, trip, rider string) string {
	t.Helper()
	var id string
	if err := db.QueryRow(context.Background(), `INSERT INTO dispatch_delivery_incidents(trip_id,provider_id,reason_code,state,source_version,idempotency_key,request_fingerprint) VALUES($1,$2,'BREAKDOWN','REASSIGNING',2,$3,'fp') RETURNING id::text`, trip, rider, uuid.NewString()).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func insertRecoveryAction(t *testing.T, db *pgxpool.Pool, booking, trip, rider, action, state string, scheduled *time.Time) string {
	t.Helper()
	ctx := context.Background()
	var attempt, id string
	now := time.Now().UTC()
	if err := db.QueryRow(ctx, `INSERT INTO dispatch_delivery_attempts(trip_id,provider_id,attempt_number,state,arrived_at,grace_expires_at,policy_version,source_version,idempotency_key,request_fingerprint) VALUES($1,$2,1,'RECOVERY_SELECTED',$3,$4,'v1',1,$5,'fp') RETURNING id::text`, trip, rider, now.Add(-time.Hour), now.Add(-50*time.Minute), uuid.NewString()).Scan(&attempt); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO dispatch_delivery_recovery_actions(delivery_attempt_id,trip_id,booking_id,customer_id,action,state,scheduled_for,policy_version,expected_trip_version,idempotency_key,request_fingerprint) VALUES($1,$2,$3,$4,$5,$6,$7,'v1',1,$8,'fp') RETURNING id::text`, attempt, trip, booking, uuid.NewString(), action, state, scheduled, uuid.NewString()).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// recoveryFacts counts the analytics facts of each recovery kind for one
// recovery, checking the product the mission carries.
func recoveryFacts(t *testing.T, db *pgxpool.Pool, product, aggregate, id string) map[networkanalytics.Kind]int {
	t.Helper()
	out := kindCountsWhere(t, db, product, `aggregate_type=$1 AND aggregate_id=$2`, "analytics:"+aggregate, id)
	return out
}

func kindCountsWhere(t *testing.T, db *pgxpool.Pool, product, where string, args ...any) map[networkanalytics.Kind]int {
	t.Helper()
	rows, err := db.Query(context.Background(), `SELECT event_type,payload->>'product_code' FROM dispatch_marketplace_outbox WHERE target_service='analytics-service' AND `+where, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[networkanalytics.Kind]int{}
	for rows.Next() {
		var event, got string
		if err := rows.Scan(&event, &got); err != nil {
			t.Fatal(err)
		}
		authority, kind, ok := networkanalytics.Classify(event, "")
		if !ok || authority != networkanalytics.AuthorityDispatch {
			t.Fatal("unclassified recovery fact", event)
		}
		if got != product {
			t.Fatalf("%s attributed to %s, want %s", event, got, product)
		}
		out[kind]++
	}
	return out
}

func wantRecovery(t *testing.T, name string, got map[networkanalytics.Kind]int, started, resolved, failed int) {
	t.Helper()
	if got[networkanalytics.KindRecoveryStarted] != started || got[networkanalytics.KindRecoveryResolved] != resolved || got[networkanalytics.KindRecoveryFailed] != failed {
		t.Errorf("%s: started %d resolved %d failed %d, want %d/%d/%d", name, got[networkanalytics.KindRecoveryStarted], got[networkanalytics.KindRecoveryResolved], got[networkanalytics.KindRecoveryFailed], started, resolved, failed)
	}
}

// Migration 000077: exactly one outcome fact per recovery at the engine's own
// terminal transition; none while in progress or when the recovery became
// moot; idempotent under repeats and concurrent writers.
func TestPostgresDispatchRecoveryOutcomesOncePerRecovery(t *testing.T) {
	db := fixture.Database(t, "../../../migrations", true)
	ctx := context.Background()
	repo := deliveryrecovery.NewRepository(db)
	rider, replacement := uuid.NewString(), uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO providers(id,phone,full_name) VALUES($1,'P1','N1'),($2,'P2','N2')`, rider, replacement); err != nil {
		t.Fatal(err)
	}
	trip := func(commerce bool) (string, string) {
		var b string
		if commerce {
			b, _ = commerceBooking(t, db)
		} else {
			b = insertBooking(t, db)
		}
		return b, insertTrip(t, db, b, rider)
	}
	parcel, commerce := networkanalytics.ProductDispatchParcel, networkanalytics.ProductDispatchCommerce

	// Replacement found: started, in-progress UNKNOWN records nothing, READY
	// resolves once (a repeated READY write adds nothing). Commerce product.
	_, t1 := trip(true)
	resolvedIncident := insertIncident(t, db, t1, rider)
	wantRecovery(t, "incident started", recoveryFacts(t, db, commerce, "delivery_incident", resolvedIncident), 1, 0, 0)
	for _, sql := range []string{`UPDATE dispatch_delivery_incidents SET recovery_result='UNKNOWN' WHERE id=$1`, `UPDATE dispatch_delivery_incidents SET recovery_result='PENDING' WHERE id=$1`} {
		if _, err := db.Exec(ctx, sql, resolvedIncident); err != nil {
			t.Fatal(err)
		}
	}
	wantRecovery(t, "incident in progress", recoveryFacts(t, db, commerce, "delivery_incident", resolvedIncident), 1, 0, 0)
	for i := 0; i < 2; i++ {
		if _, err := db.Exec(ctx, `UPDATE dispatch_delivery_incidents SET replacement_provider_id=$2,recovery_result='READY' WHERE id=$1`, resolvedIncident, replacement); err != nil {
			t.Fatal(err)
		}
	}
	wantRecovery(t, "incident resolved", recoveryFacts(t, db, commerce, "delivery_incident", resolvedIncident), 1, 1, 0)

	// Exhaustion/escalation to MANUAL_REVIEW fails once, through the real
	// administrative escalation; a later operations takeover adds nothing.
	_, t2 := trip(false)
	escalated := insertIncident(t, db, t2, rider)
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deliveryrecovery.EscalateIncidentRecoveryTx(ctx, tx, escalated, "OPERATIONS_REVIEW", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE dispatch_delivery_incidents SET state='EMERGENCY_RECOVERY' WHERE id=$1`, escalated); err != nil {
		t.Fatal(err)
	}
	wantRecovery(t, "incident escalated", recoveryFacts(t, db, parcel, "delivery_incident", escalated), 1, 0, 1)

	// Concurrent workers terminalizing the same recovery: one failed fact.
	_, t3 := trip(false)
	raced := insertIncident(t, db, t3, rider)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = db.Exec(ctx, `UPDATE dispatch_delivery_incidents SET recovery_result='MANUAL_REVIEW' WHERE id=$1 AND recovery_result IN ('PENDING','UNKNOWN')`, raced)
		}()
	}
	wg.Wait()
	wantRecovery(t, "incident concurrent exhaustion", recoveryFacts(t, db, parcel, "delivery_incident", raced), 1, 0, 1)

	// Operations takeover of an unresolved recovery fails once.
	_, t4 := trip(false)
	emergency := insertIncident(t, db, t4, rider)
	if _, err := db.Exec(ctx, `UPDATE dispatch_delivery_incidents SET state='EMERGENCY_RECOVERY' WHERE id=$1`, emergency); err != nil {
		t.Fatal(err)
	}
	wantRecovery(t, "incident emergency", recoveryFacts(t, db, parcel, "delivery_incident", emergency), 1, 0, 1)

	// Moot: the trip completed, the engine cancels the recovery. Not counted.
	_, t5 := trip(false)
	moot := insertIncident(t, db, t5, rider)
	if _, err := db.Exec(ctx, `UPDATE trips SET status='completed' WHERE id=$1`, t5); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ClaimIncidentRecovery(ctx, time.Now(), "test-owner", time.Minute, 5); err != nil {
		t.Fatal(err)
	}
	var result string
	if err := db.QueryRow(ctx, `SELECT recovery_result FROM dispatch_delivery_incidents WHERE id=$1`, moot).Scan(&result); err != nil || result != "CANCELLED" {
		t.Fatal("engine did not cancel the moot recovery", result, err)
	}
	wantRecovery(t, "incident moot", recoveryFacts(t, db, parcel, "delivery_incident", moot), 1, 0, 0)

	// Delivery recovery: an immediate action resolves when it takes effect.
	b6, t6 := trip(false)
	immediate := insertRecoveryAction(t, db, b6, t6, rider, "ALTERNATIVE_RECEIVER", "READY", nil)
	wantRecovery(t, "alternative receiver", recoveryFacts(t, db, parcel, "delivery_recovery_action", immediate), 0, 1, 0)

	// Handed to operations: failed.
	b7, t7 := trip(true)
	review := insertRecoveryAction(t, db, b7, t7, rider, "OPERATIONS_REVIEW", "READY", nil)
	wantRecovery(t, "operations review", recoveryFacts(t, db, commerce, "delivery_recovery_action", review), 0, 0, 1)

	// Scheduled: nothing until activation, one fact on activation (re-running
	// the worker adds nothing); a scheduled action whose trip ended is moot.
	due := time.Now().Add(-time.Minute)
	b8, t8 := trip(false)
	scheduled := insertRecoveryAction(t, db, b8, t8, rider, "RETRY_SAME_DAY", "SCHEDULED", &due)
	b9, t9 := trip(false)
	cancelled := insertRecoveryAction(t, db, b9, t9, rider, "RESCHEDULE", "SCHEDULED", &due)
	if _, err := db.Exec(ctx, `UPDATE trips SET status='completed' WHERE id=$1`, t9); err != nil {
		t.Fatal(err)
	}
	wantRecovery(t, "scheduled before activation", recoveryFacts(t, db, parcel, "delivery_recovery_action", scheduled), 0, 0, 0)
	for i := 0; i < 2; i++ {
		if _, err := repo.ActivateScheduled(ctx, time.Now(), 100); err != nil {
			t.Fatal(err)
		}
	}
	wantRecovery(t, "scheduled activated", recoveryFacts(t, db, parcel, "delivery_recovery_action", scheduled), 0, 1, 0)
	wantRecovery(t, "scheduled moot", recoveryFacts(t, db, parcel, "delivery_recovery_action", cancelled), 0, 0, 0)
	var state string
	if err := db.QueryRow(ctx, `SELECT state FROM dispatch_delivery_recovery_actions WHERE id=$1`, cancelled).Scan(&state); err != nil || state != "CANCELLED" {
		t.Fatal("engine did not cancel the moot scheduled action", state, err)
	}
}
