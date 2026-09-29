package movement

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/events"
)

// Customer V4 §22.2 SEND against real PostgreSQL: the sweep publishes a new
// version when drop-off arrival is recorded by an issued DELIVERY challenge,
// and the published payload never carries the credential material.
func TestSweepPublishesDropoffArrivalWithoutCredential(t *testing.T) {
	dsn := os.Getenv("DISPATCH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NOT EXECUTED: DISPATCH_TEST_DATABASE_URL (isolated, migrated dispatch database) is required")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := db.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	owner, provider, bookingID, tripID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec("INSERT INTO providers(id,phone,full_name)VALUES($1,$2,'Rider')", provider, provider)
	exec(`INSERT INTO dispatch_bookings(id,customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone)
		VALUES($1,$2,'accepted','pickup',6,3,'dropoff',7,4,0,'Receiver','+2348000000000')`, bookingID, owner)
	exec(`INSERT INTO trips(id,booking_id,provider_id,customer_id,status,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone)
		VALUES($1,$2,$3,$4,'in_progress','pickup',6,3,'dropoff',7,4,0,'Receiver','+2348000000000')`, tripID, bookingID, provider, owner)

	latest := func() (int64, events.MovementSummary, string) {
		t.Helper()
		var version int64
		var raw []byte
		if err := db.QueryRow(ctx, `SELECT source_version,payload FROM customer_movement_outbox WHERE source_ref=$1
			ORDER BY source_version DESC LIMIT 1`, bookingID).Scan(&version, &raw); err != nil {
			t.Fatal(err)
		}
		var envelope events.Envelope
		var summary events.MovementSummary
		if json.Unmarshal(raw, &envelope) != nil || json.Unmarshal(envelope.Payload, &summary) != nil {
			t.Fatal("unreadable outbox payload")
		}
		return version, summary, string(raw)
	}
	sweep := func() {
		t.Helper()
		p := NewPublisher(db)
		for i := 0; i < 1000; i++ {
			if _, err := p.Sweep(ctx); err != nil {
				t.Fatal(err)
			}
			if p.after == "" {
				return
			}
		}
	}

	sweep()
	first, summary, _ := latest()
	if summary.Stage != events.MovementStageInTransit || summary.State != events.MovementStateInProgress {
		t.Fatalf("in-custody trip published %s/%s", summary.State, summary.Stage)
	}
	sweep()
	if again, _, _ := latest(); again != first {
		t.Fatalf("an unchanged trip republished: %d -> %d", first, again)
	}

	exec(`INSERT INTO dispatch_handover_challenges(challenge_ref,context_key,version,stage,mission_ref,stop_ref,trip_id,booking_id,
		manifest_ref,manifest_version,intended_recipient_ref,recipient_type,otp_hash,qr_token_hash,expires_at,max_attempts,created_by_source,source_version)
		VALUES($1,$2,1,'DELIVERY',$3,'dropoff',$4,$5,'manifest',1,'receiver','RECEIVER','otp-digest-SECRET','qr-digest-SECRET',now()+interval '10 minutes',5,'test',1)`,
		"ch_"+uuid.NewString(), "ctx_"+uuid.NewString(), "mission_"+bookingID, tripID, bookingID)
	sweep()
	version, summary, raw := latest()
	if version != first+1 || summary.Stage != events.MovementStageArrivedDropoff {
		t.Fatalf("drop-off arrival: version %d->%d stage=%s", first, version, summary.Stage)
	}
	for _, secret := range []string{"otp", "qr", "SECRET", "challenge", "+2348000000000", "Receiver"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("movement payload leaks %q", secret)
		}
	}
}
