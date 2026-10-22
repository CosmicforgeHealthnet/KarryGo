package legacymediaadoption

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/adminmedia"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type recordingAdopter struct {
	mu    sync.Mutex
	seen  []adminmedia.LegacyAdoption
	fails map[string]error
}

func (r *recordingAdopter) Adopt(_ context.Context, a adminmedia.LegacyAdoption) (adminmedia.LegacyAdoptionReceipt, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, a)
	if err := r.fails[a.LegacyAssetID]; err != nil {
		return adminmedia.LegacyAdoptionReceipt{}, err
	}
	return adminmedia.LegacyAdoptionReceipt{ContractVersion: adminmedia.LegacyAdoptionContract, LegacyAssetID: a.LegacyAssetID, MediaObjectID: "mobj_x"}, nil
}

// seedHubCheckpoint creates a real trip, delivery attempt, HUB_HOLD recovery
// action, hub hold operation and checkpoint carrying a legacy media ID.
func seedHubCheckpoint(t *testing.T, db *pgxpool.Pool, organization *string) (trip, recovery, legacy string) {
	t.Helper()
	ctx := context.Background()
	provider, customer := uuid.NewString(), uuid.NewString()
	if _, err := db.Exec(ctx, `INSERT INTO providers(id,phone,full_name) VALUES($1,$2,'PRIVATE_NAME')`, provider, "+234"+provider[:8]); err != nil {
		t.Fatal(err)
	}
	var booking string
	if err := db.QueryRow(ctx, `INSERT INTO dispatch_bookings(customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone) VALUES($1,'A',0,0,'B',0,0,100,'R','P') RETURNING id::text`, customer).Scan(&booking); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO trips(booking_id,provider_id,customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone,organization_id) VALUES($1,$2,$3,'A',0,0,'B',0,0,100,'R','P',$4) RETURNING id::text`, booking, provider, customer, organization).Scan(&trip); err != nil {
		t.Fatal(err)
	}
	attempt, hold := uuid.NewString(), uuid.NewString()
	recovery, legacy = uuid.NewString(), uuid.NewString()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO dispatch_delivery_attempts(id,trip_id,provider_id,attempt_number,arrived_at,grace_expires_at,policy_version,source_version,idempotency_key,request_fingerprint) VALUES($1,$2,$3,1,now(),now()+interval '5 minutes','test-policy',1,$4,'FP')`, []any{attempt, trip, provider, "attempt-" + attempt}},
		{`INSERT INTO dispatch_delivery_recovery_actions(id,delivery_attempt_id,trip_id,booking_id,customer_id,action,state,policy_version,expected_trip_version,idempotency_key,request_fingerprint) VALUES($1,$2,$3,$4,$5,'HUB_HOLD','SCHEDULED','test-policy',1,$6,'FP')`, []any{recovery, attempt, trip, booking, customer, "recovery-" + recovery}},
		{`INSERT INTO dispatch_hub_hold_operations(id,recovery_action_id,trip_id,hub_ref,hub_authority_ref,state,expires_at) VALUES($1,$2,$3,'hub-1','hub-authority-1','HELD',now()+interval '1 day')`, []any{hold, recovery, trip}},
		{`INSERT INTO dispatch_hub_hold_checkpoints(hub_hold_operation_id,trip_id,provider_id,stage,manifest_ref,parcel_codes,media_ref,idempotency_key,request_fingerprint,recorded_at) VALUES($1,$2,$3,'ARRIVAL','manifest-1','{P1}',$4,$5,'FP',now())`, []any{hold, trip, provider, legacy, "checkpoint-" + hold}},
	} {
		if _, err := db.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	return trip, recovery, legacy
}

func TestPostgresDispatchAttestsHubCheckpointLegacyMediaForItsRecoveryCase(t *testing.T) {
	db := fixture.Database(t, "../../../migrations", true)
	ctx := context.Background()
	org := uuid.NewString()
	trip, recovery, legacy := seedHubCheckpoint(t, db, &org)
	independentTrip, _, independentLegacy := seedHubCheckpoint(t, db, nil)
	_, _, refusedLegacy := seedHubCheckpoint(t, db, &org)
	adopter := &recordingAdopter{fails: map[string]error{refusedLegacy: adminmedia.ErrDenied}}
	w := &Worker{DB: db, Media: adopter}
	adopted, err := w.RunOnce(ctx)
	if err != nil || adopted != 1 {
		t.Fatal("adoption pass", adopted, err)
	}
	var got *adminmedia.LegacyAdoption
	for i, a := range adopter.seen {
		if a.LegacyAssetID == independentLegacy || a.SubjectReference == independentTrip {
			t.Fatal("trip without an organization attested")
		}
		if a.LegacyAssetID == legacy {
			got = &adopter.seen[i]
		}
	}
	want := adminmedia.LegacyAdoption{ContractVersion: adminmedia.LegacyAdoptionContract, LegacyAssetID: legacy, LegacyOwnerReference: "trip:" + trip, OrganizationID: org, CaseKind: "delivery-recoveries", CaseReference: recovery, SubjectType: "TRIP", SubjectReference: trip, Purpose: "CARGO_CONDITION"}
	if got == nil || *got != want || !want.Valid(Owner) {
		t.Fatalf("attestation %+v want %+v", got, want)
	}
	// Settled outcomes (adopted or refused) are not resent in this process.
	before := len(adopter.seen)
	if _, err := w.RunOnce(ctx); err != nil || len(adopter.seen) != before {
		t.Fatal("settled attestation resent", err)
	}
	// An unavailable Media is retried on the next pass.
	_, _, flaky := seedHubCheckpoint(t, db, &org)
	adopter.fails[flaky] = adminmedia.ErrUnavailable
	w.RunOnce(ctx)
	delete(adopter.fails, flaky)
	if n, _ := w.RunOnce(ctx); n != 1 {
		t.Fatal("unavailable outcome not retried", n)
	}
}

// The configured worker signs as Dispatch with its dedicated key over the
// fixed Media path; the default is disabled.
func TestDispatchLegacyAdoptionWorkerSignedClientAndDefaultOff(t *testing.T) {
	t.Setenv("DISPATCH_LEGACY_MEDIA_ADOPTION_ENABLED", "")
	if w, err := FromEnvironment(nil); w != nil || err != nil {
		t.Fatal("worker enabled by default", err)
	}
	key := bytes.Repeat([]byte("k"), 32)
	var mu sync.Mutex
	var callers []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(raw))
		caller, err := serviceauth.NewVerifier(serviceauth.Secrets{Owner: key}, 30*time.Second).VerifyRequest(r)
		var a adminmedia.LegacyAdoption
		if err != nil || r.URL.Path != adminmedia.LegacyAdoptionPath || adminmedia.Decode(raw, &a) != nil || !a.Valid(caller) {
			w.WriteHeader(401)
			return
		}
		mu.Lock()
		callers = append(callers, caller)
		mu.Unlock()
		json.NewEncoder(w).Encode(adminmedia.LegacyAdoptionReceipt{ContractVersion: adminmedia.LegacyAdoptionContract, LegacyAssetID: a.LegacyAssetID, MediaObjectID: "mobj_" + a.LegacyAssetID[:8], ChecksumSHA256: string(bytes.Repeat([]byte("a"), 64)), AttestationDigest: a.Digest(caller), RecordedAt: time.Now().UTC()})
	}))
	defer server.Close()
	client, err := adminmedia.NewLegacyAdoptionClient(Owner, server.URL, key)
	if err != nil {
		t.Fatal(err)
	}
	c := Candidate{LegacyAssetID: uuid.NewString(), MissionRef: "trip:x", OrganizationID: "org-1", RecoveryID: uuid.NewString(), TripID: uuid.NewString()}
	receipt, err := client.Adopt(context.Background(), c.Attestation())
	if err != nil || receipt.LegacyAssetID != c.LegacyAssetID || len(callers) != 1 || callers[0] != Owner {
		t.Fatal("signed adoption", err, callers)
	}
	wrong, _ := adminmedia.NewLegacyAdoptionClient(Owner, server.URL, bytes.Repeat([]byte("z"), 32))
	if _, err := wrong.Adopt(context.Background(), c.Attestation()); err == nil {
		t.Fatal("wrong adoption key accepted")
	}
}
