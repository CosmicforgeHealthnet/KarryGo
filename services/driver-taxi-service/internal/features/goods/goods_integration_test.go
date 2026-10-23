package goods

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/handover"
)

func goodsTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TAXI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TAXI_TEST_DATABASE_URL is required for Taxi Goods PostgreSQL certification")
	}
	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(db.Close)
	var exists bool
	if err = db.QueryRow(context.Background(), `SELECT to_regclass('public.taxi_goods_missions') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Skip("Taxi Goods migrations 007-008 are not applied to TAXI_TEST_DATABASE_URL")
	}
	return db
}

type goodsFixture struct {
	provider, booking, mission, quote, market, zone string
}

func seedGoodsRaceFixture(t *testing.T, db *pgxpool.Pool) goodsFixture {
	t.Helper()
	f := goodsFixture{provider: uuid.NewString(), booking: uuid.NewString(), mission: uuid.NewString(), quote: uuid.NewString(), market: uuid.NewString(), zone: uuid.NewString()}
	code := "TG-" + uuid.NewString()[:8]
	ctx := context.Background()
	must := func(sql string, args ...any) {
		if _, err := db.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seed Taxi Goods fixture: %v", err)
		}
	}
	must(`INSERT INTO markets(id,country,name,code) VALUES($1,'NG','Taxi Goods Test',$2)`, f.market, code)
	must(`INSERT INTO operating_zones(id,market_id,name,code,center_lat,center_lng,match_radius_km,supported_classes) VALUES($1,$2,'Taxi Goods Zone',$3,50,50,1,'{economy}')`, f.zone, f.market, code)
	must(`INSERT INTO taxi_providers(id,phone) VALUES($1,$2)`, f.provider, "+234"+f.provider[:10])
	must(`INSERT INTO taxi_bookings(id,customer_id,market_id,operating_zone_id,vehicle_class,status,pricing_quote_reference) VALUES($1,$2,$3,$4,'economy','pending_match',$5)`, f.booking, uuid.NewString(), f.market, f.zone, "quote-"+code)
	must(`INSERT INTO taxi_goods_quotes(id,source_service,quote_reference,revision,request_fingerprint,request_payload,serviceable,reason_code,distance_km,estimated_minutes,fee_minor,currency,available_providers,expires_at) VALUES($1,'vendor-service',$2,1,'fp','{}',true,'SERVICEABLE',1,5,1000,'NGN',1,now()+interval '15 minutes')`, f.quote, "goods-quote-"+code)
	must(`INSERT INTO taxi_goods_missions(id,source_service,mission_ref,operation_ref,request_fingerprint,parent_order_ref,vendor_order_refs,customer_ref,quote_id,pickup,dropoff,load,agreed_fee_minor,currency,requested_at) VALUES($1,'vendor-service',$2,$2,'fp',$3,ARRAY[$4],$5,$6,'{}','{}','{}',1000,'NGN',now())`, f.mission, "goods-mission-"+code, "parent-"+code, "vendor-order-"+code, "customer-"+code, f.quote)
	must(`INSERT INTO provider_operational_state(provider_id,state,version) VALUES($1,'AVAILABLE',1)`, f.provider)
	t.Cleanup(func() {
		_, _ = db.Exec(ctx, `DELETE FROM taxi_goods_handover_challenges WHERE mission_id=$1`, f.mission)
		_, _ = db.Exec(ctx, `DELETE FROM taxi_goods_binding_operations WHERE mission_id=$1`, f.mission)
		_, _ = db.Exec(ctx, `DELETE FROM taxi_admin_audit_events WHERE entity_id=$1`, f.mission)
		_, _ = db.Exec(ctx, `DELETE FROM provider_operational_state_history WHERE provider_id=$1`, f.provider)
		_, _ = db.Exec(ctx, `DELETE FROM provider_operational_state WHERE provider_id=$1`, f.provider)
		_, _ = db.Exec(ctx, `DELETE FROM taxi_outbox WHERE aggregate_id IN ($1,$2)`, f.mission, f.booking)
		_, _ = db.Exec(ctx, `DELETE FROM taxi_goods_missions WHERE id=$1`, f.mission)
		_, _ = db.Exec(ctx, `DELETE FROM taxi_goods_quotes WHERE id=$1`, f.quote)
		_, _ = db.Exec(ctx, `DELETE FROM taxi_bookings WHERE id=$1`, f.booking)
		_, _ = db.Exec(ctx, `DELETE FROM taxi_providers WHERE id=$1`, f.provider)
		_, _ = db.Exec(ctx, `DELETE FROM operating_zones WHERE id=$1`, f.zone)
		_, _ = db.Exec(ctx, `DELETE FROM markets WHERE id=$1`, f.market)
	})
	return f
}

type testCredentialNotifier struct{}

func (testCredentialNotifier) NotifyDeliveryCredential(context.Context, string, string, Credential) error {
	return nil
}
func (testCredentialNotifier) NotifyPickupCredential(context.Context, string, string, Credential) error {
	return nil
}

type failingCredentialNotifier struct{}

func (failingCredentialNotifier) NotifyDeliveryCredential(context.Context, string, string, Credential) error {
	return errors.New("notification unavailable")
}
func (failingCredentialNotifier) NotifyPickupCredential(context.Context, string, string, Credential) error {
	return errors.New("notification unavailable")
}

type handoverRaceFixture struct {
	goodsFixture
	newProvider string
	newVehicle  string
	credential  Credential
	service     *Service
}

func seedHandoverRaceFixture(t *testing.T, db *pgxpool.Pool, stage string) handoverRaceFixture {
	t.Helper()
	f := seedGoodsRaceFixture(t, db)
	ctx := context.Background()
	newProvider, newVehicle := uuid.NewString(), uuid.NewString()
	must := func(query string, args ...any) {
		if _, err := db.Exec(ctx, query, args...); err != nil {
			t.Fatalf("seed handover race: %v", err)
		}
	}
	must(`INSERT INTO taxi_providers(id,phone) VALUES($1,$2)`, newProvider, "+235"+newProvider[:10])
	must(`INSERT INTO vehicles(id,provider_id,vehicle_class,plate_number,status) VALUES($1,$2,'economy',$3,'active')`, newVehicle, newProvider, "TG"+newVehicle[:8])
	must(`INSERT INTO taxi_goods_provider_settings(provider_id,capability_status,goods_available) VALUES($1,'APPROVED',true)`, newProvider)
	must(`INSERT INTO taxi_vehicle_cargo_profiles(vehicle_id,max_weight_grams,max_volume_ml,max_package_count,longest_dimension_mm,verified) VALUES($1,100000,1000000,20,2000,true)`, newVehicle)
	must(`INSERT INTO provider_operational_state(provider_id,state,version) VALUES($1,'AVAILABLE',1)`, newProvider)
	status, opState := StatusPickupHandoverRequired, "AT_PICKUP"
	if stage == StageDelivery {
		status, opState = StatusDeliveryHandoverRequired, "ON_TRIP"
	}
	must(`UPDATE taxi_goods_missions SET provider_id=$2,receiver_ref='receiver-old',status=$3 WHERE id=$1`, f.mission, f.provider, status)
	must(`UPDATE provider_operational_state SET state=$2,goods_mission_id=$3 WHERE provider_id=$1`, f.provider, opState, f.mission)
	secret := []byte("01234567890123456789012345678901")
	step := handover.StepPickup
	if stage == StageDelivery {
		step = handover.StepDelivery
	}
	challengeRef := "race-" + uuid.NewString()
	now := time.Now().UTC()
	secure, err := handover.NewSecureCodeRecord(secret, "taxi-goods", step, challengeRef, 1, f.mission, f.provider, "receiver-old", 0, now)
	if err != nil {
		t.Fatal(err)
	}
	record := HandoverRecord{ChallengeRef: challengeRef, MissionID: f.mission, Stage: stage, ProviderID: f.provider, ReceiverRef: "receiver-old", OTPHash: secure.Hash, QRHash: secure.QRHash, Version: 1, ExpiresAt: secure.ExpiresAt, ResendAvailableAt: now.Add(-time.Second)}
	repo := NewRepository(db)
	if err = repo.InsertChallenge(ctx, record); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(ctx, `DELETE FROM taxi_goods_handover_challenges WHERE mission_id=$1`, f.mission)
		_, _ = db.Exec(ctx, `DELETE FROM taxi_goods_binding_operations WHERE mission_id=$1`, f.mission)
		_, _ = db.Exec(ctx, `DELETE FROM provider_operational_state_history WHERE provider_id=$1`, newProvider)
		_, _ = db.Exec(ctx, `DELETE FROM provider_operational_state WHERE provider_id=$1`, newProvider)
		_, _ = db.Exec(ctx, `DELETE FROM taxi_goods_provider_settings WHERE provider_id=$1`, newProvider)
		_, _ = db.Exec(ctx, `DELETE FROM vehicles WHERE id=$1`, newVehicle)
		_, _ = db.Exec(ctx, `DELETE FROM taxi_providers WHERE id=$1`, newProvider)
	})
	return handoverRaceFixture{goodsFixture: f, newProvider: newProvider, newVehicle: newVehicle,
		credential: Credential{ChallengeRef: challengeRef, Stage: stage, OTP: secure.Code, QRToken: secure.QRToken, Version: 1, ExpiresAt: secure.ExpiresAt},
		service:    NewService(Options{Repository: repo, Enabled: true, HandoverSecret: secret, Notifier: testCredentialNotifier{}, Now: time.Now})}
}

func TestPassengerVsGoodsExecutionRaceExactlyOneWins(t *testing.T) {
	db := goodsTestDB(t)
	f := seedGoodsRaceFixture(t, db)
	ctx := context.Background()
	start := make(chan struct{})
	results := make(chan int64, 2)
	var wg sync.WaitGroup
	for _, command := range []struct {
		sql string
		ref string
	}{
		{`UPDATE provider_operational_state SET state='OFFERED',booking_id=$2,goods_mission_id=NULL,version=version+1 WHERE provider_id=$1 AND state='AVAILABLE' AND booking_id IS NULL AND goods_mission_id IS NULL`, f.booking},
		{`UPDATE provider_operational_state SET state='OFFERED',booking_id=NULL,goods_mission_id=$2,version=version+1 WHERE provider_id=$1 AND state='AVAILABLE' AND booking_id IS NULL AND goods_mission_id IS NULL`, f.mission},
	} {
		wg.Add(1)
		go func(sql, ref string) {
			defer wg.Done()
			<-start
			tag, err := db.Exec(ctx, sql, f.provider, ref)
			if err != nil {
				results <- -1
				return
			}
			results <- tag.RowsAffected()
		}(command.sql, command.ref)
	}
	close(start)
	wg.Wait()
	close(results)
	var winners int64
	for result := range results {
		if result < 0 {
			t.Fatal("execution race update failed")
		}
		winners += result
	}
	if winners != 1 {
		t.Fatalf("execution winners = %d, want exactly 1", winners)
	}
}

func TestOTPAndQRConcurrentVerificationOneConsumes(t *testing.T) {
	db := goodsTestDB(t)
	f := seedGoodsRaceFixture(t, db)
	now := time.Now().UTC()
	_, err := db.Exec(context.Background(), `INSERT INTO taxi_goods_handover_challenges(challenge_ref,mission_id,stage,provider_id,receiver_ref,otp_hash,qr_hash,version,status,expires_at,resend_available_at) VALUES('challenge-race',$1,'DELIVERY',$2,'receiver',decode('aa','hex'),decode('bb','hex'),1,'ACTIVE',$3,$4)`, f.mission, f.provider, now.Add(time.Minute), now)
	if err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, attempt := range []struct{ method, hash string }{{"OTP", string([]byte{0xaa})}, {"QR", string([]byte{0xbb})}} {
		go func(method, hash string) {
			<-start
			_, consumeErr := repo.ConsumeChallenge(context.Background(), f.mission, "challenge-race", method, hash, now)
			results <- consumeErr
		}(attempt.method, attempt.hash)
	}
	close(start)
	successes := 0
	for i := 0; i < 2; i++ {
		if <-results == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful alternative credential consumptions = %d, want 1", successes)
	}
}

func TestReassignmentVsVerificationRacesHaveOneDurableWinner(t *testing.T) {
	for _, method := range []string{"OTP", "QR"} {
		t.Run(method, func(t *testing.T) {
			db := goodsTestDB(t)
			f := seedHandoverRaceFixture(t, db, StagePickup)
			missionRef := missionRefFor(t, db, f.mission)
			credential := f.credential.OTP
			if method == "QR" {
				credential = f.credential.QRToken
			}
			verify := VerifyCredential{ChallengeRef: f.credential.ChallengeRef, Method: method, Credential: credential, OperationRef: "verify-" + uuid.NewString()}
			reassign := ReassignProviderCommand{NewProviderID: f.newProvider, NewVehicleID: f.newVehicle, OperationRef: "reassign-" + uuid.NewString(), ExpectedVersion: 1}
			start := make(chan struct{})
			results := make(chan error, 2)
			go func() {
				<-start
				_, err := f.service.VerifyPickup(context.Background(), "vendor-service", missionRef, verify)
				results <- err
			}()
			go func() {
				<-start
				_, err := f.service.ReassignProvider(context.Background(), "operations-service", missionRef, reassign)
				results <- err
			}()
			close(start)
			successes := 0
			for range 2 {
				if <-results == nil {
					successes++
				}
			}
			if successes != 1 {
				t.Fatalf("successful race operations=%d, want 1", successes)
			}
			replay := verify
			replay.OperationRef = "old-credential-replay-" + uuid.NewString()
			if _, err := f.service.VerifyPickup(context.Background(), "vendor-service", missionRef, replay); err == nil {
				t.Fatal("old pickup credential verified after race resolution")
			}
		})
	}
}

func TestReceiverChangeVsVerificationRacesHaveOneDurableWinner(t *testing.T) {
	for _, method := range []string{"OTP", "QR"} {
		t.Run(method, func(t *testing.T) {
			db := goodsTestDB(t)
			f := seedHandoverRaceFixture(t, db, StageDelivery)
			missionRef := missionRefFor(t, db, f.mission)
			credential := f.credential.OTP
			if method == "QR" {
				credential = f.credential.QRToken
			}
			verify := VerifyCredential{ChallengeRef: f.credential.ChallengeRef, Method: method, Credential: credential, OperationRef: "verify-" + uuid.NewString(), ReceiverRef: "receiver-old"}
			change := ReceiverChangeCommand{ReceiverRef: "receiver-new", ReceiverName: "New Receiver", OperationRef: "receiver-" + uuid.NewString(), ExpectedVersion: 1}
			start := make(chan struct{})
			results := make(chan error, 2)
			go func() {
				<-start
				_, err := f.service.VerifyDelivery(context.Background(), f.provider, missionRef, verify)
				results <- err
			}()
			go func() {
				<-start
				_, err := f.service.ChangeReceiver(context.Background(), "vendor-service", "vendor-service", missionRef, change)
				results <- err
			}()
			close(start)
			successes := 0
			for range 2 {
				if <-results == nil {
					successes++
				}
			}
			if successes != 1 {
				t.Fatalf("successful race operations=%d, want 1", successes)
			}
			replay := verify
			replay.OperationRef = "old-credential-replay-" + uuid.NewString()
			if _, err := f.service.VerifyDelivery(context.Background(), f.provider, missionRef, replay); err == nil {
				t.Fatal("old receiver credential verified after race resolution")
			}
		})
	}
}

func TestResendVsVerificationRacesHaveOneDurableWinner(t *testing.T) {
	for _, method := range []string{"OTP", "QR"} {
		t.Run(method, func(t *testing.T) {
			db := goodsTestDB(t)
			f := seedHandoverRaceFixture(t, db, StagePickup)
			missionRef := missionRefFor(t, db, f.mission)
			credential := f.credential.OTP
			if method == "QR" {
				credential = f.credential.QRToken
			}
			verify := VerifyCredential{ChallengeRef: f.credential.ChallengeRef, Method: method, Credential: credential, OperationRef: "verify-" + uuid.NewString()}
			start := make(chan struct{})
			results := make(chan error, 2)
			go func() {
				<-start
				_, err := f.service.VerifyPickup(context.Background(), "vendor-service", missionRef, verify)
				results <- err
			}()
			go func() {
				<-start
				_, err := f.service.Resend(context.Background(), f.provider, missionRef, StagePickup)
				results <- err
			}()
			close(start)
			successes := 0
			for range 2 {
				if <-results == nil {
					successes++
				}
			}
			if successes != 1 {
				t.Fatalf("successful race operations=%d, want 1", successes)
			}
			replay := verify
			replay.OperationRef = "old-credential-replay-" + uuid.NewString()
			if _, err := f.service.VerifyPickup(context.Background(), "vendor-service", missionRef, replay); err == nil {
				t.Fatal("pre-resend credential verified after race resolution")
			}
		})
	}
}

func TestBindingCommandsAreIdempotentAndRejectFingerprintReuse(t *testing.T) {
	t.Run("provider reassignment", func(t *testing.T) {
		db := goodsTestDB(t)
		f := seedHandoverRaceFixture(t, db, StagePickup)
		missionRef := missionRefFor(t, db, f.mission)
		command := ReassignProviderCommand{NewProviderID: f.newProvider, NewVehicleID: f.newVehicle, OperationRef: "reassign-" + uuid.NewString(), ExpectedVersion: 1}
		first, err := f.service.ReassignProvider(context.Background(), "operations-service", missionRef, command)
		if err != nil {
			t.Fatal(err)
		}
		second, err := f.service.ReassignProvider(context.Background(), "operations-service", missionRef, command)
		if err != nil || second.Version != first.Version || second.ProviderID != first.ProviderID {
			t.Fatalf("idempotent reassignment mismatch: first=%+v second=%+v err=%v", first, second, err)
		}
		var operations, active int
		if err = db.QueryRow(context.Background(), `SELECT count(*) FROM taxi_goods_binding_operations WHERE operation_ref=$1`, command.OperationRef).Scan(&operations); err != nil {
			t.Fatal(err)
		}
		if err = db.QueryRow(context.Background(), `SELECT count(*) FROM taxi_goods_handover_challenges WHERE mission_id=$1 AND status='ACTIVE'`, f.mission).Scan(&active); err != nil {
			t.Fatal(err)
		}
		if operations != 1 || active != 1 {
			t.Fatalf("operations=%d active_challenges=%d, want 1/1", operations, active)
		}
		changed := command
		changed.NewVehicleID = uuid.NewString()
		if _, err = f.service.ReassignProvider(context.Background(), "operations-service", missionRef, changed); !errors.Is(err, ErrConflict) {
			t.Fatalf("reused operation with changed payload err=%v, want conflict", err)
		}
	})

	t.Run("receiver change", func(t *testing.T) {
		db := goodsTestDB(t)
		f := seedHandoverRaceFixture(t, db, StageDelivery)
		missionRef := missionRefFor(t, db, f.mission)
		command := ReceiverChangeCommand{ReceiverRef: "receiver-new", ReceiverName: "New Receiver", OperationRef: "receiver-" + uuid.NewString(), ExpectedVersion: 1}
		first, err := f.service.ChangeReceiver(context.Background(), "vendor-service", "vendor-service", missionRef, command)
		if err != nil {
			t.Fatal(err)
		}
		second, err := f.service.ChangeReceiver(context.Background(), "vendor-service", "vendor-service", missionRef, command)
		if err != nil || second.Version != first.Version || second.ReceiverRef != first.ReceiverRef {
			t.Fatalf("idempotent receiver change mismatch: first=%+v second=%+v err=%v", first, second, err)
		}
		changed := command
		changed.ReceiverName = "Different Receiver"
		if _, err = f.service.ChangeReceiver(context.Background(), "vendor-service", "vendor-service", missionRef, changed); !errors.Is(err, ErrConflict) {
			t.Fatalf("reused operation with changed payload err=%v, want conflict", err)
		}
	})
}

func TestVerificationOperationReplayReturnsCommittedMission(t *testing.T) {
	for _, stage := range []string{StagePickup, StageDelivery} {
		t.Run(stage, func(t *testing.T) {
			db := goodsTestDB(t)
			f := seedHandoverRaceFixture(t, db, stage)
			missionRef := missionRefFor(t, db, f.mission)
			command := VerifyCredential{ChallengeRef: f.credential.ChallengeRef, Method: "OTP", Credential: f.credential.OTP, OperationRef: "verify-" + uuid.NewString()}
			var first, second Mission
			var err error
			if stage == StagePickup {
				first, err = f.service.VerifyPickup(context.Background(), "vendor-service", missionRef, command)
				if err == nil {
					second, err = f.service.VerifyPickup(context.Background(), "vendor-service", missionRef, command)
				}
			} else {
				command.ReceiverRef = "receiver-old"
				first, err = f.service.VerifyDelivery(context.Background(), f.provider, missionRef, command)
				if err == nil {
					second, err = f.service.VerifyDelivery(context.Background(), f.provider, missionRef, command)
				}
			}
			if err != nil || first.ID != second.ID || first.Version != second.Version || first.Status != second.Status {
				t.Fatalf("verification replay mismatch: first=%+v second=%+v err=%v", first, second, err)
			}
		})
	}
}

func TestNotificationFailureInvalidatesCredentialAndAllowsRotation(t *testing.T) {
	db := goodsTestDB(t)
	f := seedHandoverRaceFixture(t, db, StageDelivery)
	missionRef := missionRefFor(t, db, f.mission)
	f.service = NewService(Options{Repository: NewRepository(db), Enabled: true, HandoverSecret: []byte("01234567890123456789012345678901"), Notifier: failingCredentialNotifier{}, Now: time.Now})
	command := ReceiverChangeCommand{ReceiverRef: "receiver-new", ReceiverName: "New Receiver", OperationRef: "receiver-" + uuid.NewString(), ExpectedVersion: 1}
	if _, err := f.service.ChangeReceiver(context.Background(), "vendor-service", "vendor-service", missionRef, command); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("notification failure err=%v, want unavailable", err)
	}
	var challengeRef, status string
	if err := db.QueryRow(context.Background(), `SELECT challenge_ref,status FROM taxi_goods_handover_challenges WHERE mission_id=$1 ORDER BY version DESC LIMIT 1`, f.mission).Scan(&challengeRef, &status); err != nil {
		t.Fatal(err)
	}
	if status != "INVALIDATED" {
		t.Fatalf("replacement challenge status=%s, want INVALIDATED", status)
	}
	if _, err := db.Exec(context.Background(), `UPDATE taxi_goods_handover_challenges SET resend_available_at=now()-interval '1 second' WHERE challenge_ref=$1`, challengeRef); err != nil {
		t.Fatal(err)
	}
	f.service = NewService(Options{Repository: NewRepository(db), Enabled: true, HandoverSecret: []byte("01234567890123456789012345678901"), Notifier: testCredentialNotifier{}, Now: time.Now})
	if _, err := f.service.Resend(context.Background(), f.provider, missionRef, StageDelivery); err != nil {
		t.Fatalf("rotate invalidated undelivered credential: %v", err)
	}
}

func missionRefFor(t *testing.T, db *pgxpool.Pool, missionID string) string {
	t.Helper()
	var ref string
	if err := db.QueryRow(context.Background(), `SELECT mission_ref FROM taxi_goods_missions WHERE id=$1`, missionID).Scan(&ref); err != nil {
		t.Fatal(err)
	}
	return ref
}

// Verification winning before LatestChallenge used to let Resend insert a new
// ACTIVE row. This ordering is deterministic rather than scheduler-dependent.
func TestConsumedPickupCredentialCannotBeResent(t *testing.T) {
	for _, method := range []string{"OTP", "QR"} {
		t.Run(method, func(t *testing.T) {
			db := goodsTestDB(t)
			f := seedHandoverRaceFixture(t, db, StagePickup)
			missionRef := missionRefFor(t, db, f.mission)
			credential := f.credential.OTP
			if method == "QR" {
				credential = f.credential.QRToken
			}
			command := VerifyCredential{ChallengeRef: f.credential.ChallengeRef, Method: method, Credential: credential, OperationRef: "consume-before-resend-" + uuid.NewString()}
			if _, err := f.service.VerifyPickup(context.Background(), "vendor-service", missionRef, command); err != nil {
				t.Fatal(err)
			}
			if _, err := f.service.Resend(context.Background(), f.provider, missionRef, StagePickup); !errors.Is(err, ErrResendUnavailable) {
				t.Fatalf("consumed credential resend: %v", err)
			}
			var total, active, consumed int
			if err := db.QueryRow(context.Background(), `SELECT count(*),count(*) FILTER (WHERE status='ACTIVE'),count(*) FILTER (WHERE status='CONSUMED') FROM taxi_goods_handover_challenges WHERE mission_id=$1 AND stage=$2`, f.mission, StagePickup).Scan(&total, &active, &consumed); err != nil {
				t.Fatal(err)
			}
			if total != 1 || active != 0 || consumed != 1 {
				t.Fatalf("resend changed consumed history: total=%d active=%d consumed=%d", total, active, consumed)
			}
		})
	}
}
