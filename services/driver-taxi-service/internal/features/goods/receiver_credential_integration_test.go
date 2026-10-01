package goods

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/handover"
)

type receiverReadPermit struct{}

func (receiverReadPermit) Allow(context.Context, string, int, time.Duration) (bool, error) {
	return true, nil
}

func TestTaxiGoodsReceiverAccountReadAndContactInvalidation(t *testing.T) {
	db := goodsTestDB(t)
	f := seedGoodsRaceFixture(t, db)
	ctx := context.Background()
	receiverID := uuid.NewString()
	if _, err := db.Exec(ctx, `UPDATE taxi_goods_missions SET provider_id=$2,receiver_ref=$3,status='PICKED_UP' WHERE id=$1`, f.mission, f.provider, receiverID); err != nil {
		t.Fatal(err)
	}
	secret := []byte("01234567890123456789012345678901")
	challengeRef := "receiver-" + uuid.NewString()
	secure, err := handover.NewSecureCodeRecord(secret, "taxi-goods", handover.StepDelivery, challengeRef, 1, f.mission, f.provider, receiverID, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db)
	if err = repo.InsertChallenge(ctx, HandoverRecord{ChallengeRef: challengeRef, MissionID: f.mission, Stage: StageDelivery, ProviderID: f.provider, ReceiverRef: receiverID, OTPHash: secure.Hash, QRHash: secure.QRHash, Version: 1, ExpiresAt: secure.ExpiresAt, ResendAvailableAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	svc := NewService(Options{Repository: repo, Enabled: true, HandoverSecret: secret, ReceiverReadLimiter: receiverReadPermit{}})
	missionRef := "goods-mission-" // fixture's unique ref is loaded from the database below.
	if err = db.QueryRow(ctx, `SELECT mission_ref FROM taxi_goods_missions WHERE id=$1`, f.mission).Scan(&missionRef); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ReadReceiverCredential(ctx, uuid.NewString(), missionRef, challengeRef); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other customer read: %v", err)
	}
	got, err := svc.ReadReceiverCredential(ctx, receiverID, missionRef, challengeRef)
	if err != nil || got.OTP != secure.Code || got.QRToken != secure.QRToken {
		t.Fatalf("bound receiver read: %v", err)
	}
	if _, err = db.Exec(ctx, `UPDATE taxi_goods_missions SET receiver_ref=$2 WHERE id=$1`, f.mission, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ReadReceiverCredential(ctx, receiverID, missionRef, challengeRef); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stale receiver read after change: %v", err)
	}
}

func TestTaxiGoodsPhoneOnlyFailsBeforePickupCustody(t *testing.T) {
	db := goodsTestDB(t)
	f := seedGoodsRaceFixture(t, db)
	if _, err := db.Exec(context.Background(), `UPDATE taxi_goods_missions SET provider_id=$2,status='AT_PICKUP',receiver_phone='+390001234567' WHERE id=$1`, f.mission, f.provider); err != nil {
		t.Fatal(err)
	}
	var missionRef string
	if err := db.QueryRow(context.Background(), `SELECT mission_ref FROM taxi_goods_missions WHERE id=$1`, f.mission).Scan(&missionRef); err != nil {
		t.Fatal(err)
	}
	svc := NewService(Options{Repository: NewRepository(db), Enabled: true, HandoverSecret: []byte("01234567890123456789012345678901")})
	if _, err := svc.IssuePickupCredential(context.Background(), f.provider, missionRef); !errors.Is(err, ErrReceiverSecureChannelRequired) {
		t.Fatalf("phone-only pickup credential: %v", err)
	}
	var status string
	if err := db.QueryRow(context.Background(), `SELECT status FROM taxi_goods_missions WHERE id=$1`, f.mission).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "AT_PICKUP" {
		t.Fatalf("phone-only gate changed custody state to %s", status)
	}
}
