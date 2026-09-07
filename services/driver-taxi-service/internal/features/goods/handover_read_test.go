package goods

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestSection32OpaqueReadBackTracksCanonicalPickup(t *testing.T) {
	db := goodsTestDB(t)
	f := seedHandoverRaceFixture(t, db, StagePickup)
	ctx := context.Background()
	ref := missionRefFor(t, db, f.mission)
	before, err := f.service.ReadHandover(ctx, "vendor-service", ref, StagePickup)
	if err != nil || before.Status != "ACTIVE" || before.HandoverRef != f.credential.ChallengeRef {
		t.Fatalf("%+v %v", before, err)
	}
	command := VerifyCredential{ChallengeRef: f.credential.ChallengeRef, Method: "OTP", Credential: f.credential.OTP, OperationRef: "section32:" + ref}
	if _, err = f.service.VerifyPickup(ctx, "vendor-service", ref, command); err != nil {
		t.Fatal(err)
	}
	after, err := f.service.ReadHandover(ctx, "vendor-service", ref, StagePickup)
	if err != nil || after.Status != "CONSUMED" || after.SourceVersion <= before.SourceVersion {
		t.Fatalf("%+v %v", after, err)
	}
	raw, _ := json.Marshal(after)
	for _, secret := range []string{f.credential.OTP, f.credential.QRToken, "otp_hash", "qr_hash", "expires_at", "attempt"} {
		if secret != "" && strings.Contains(string(raw), secret) {
			t.Fatal("secret or challenge state escaped read-back")
		}
	}
	if _, err = f.service.ReadHandover(ctx, "unrelated-service", ref, StagePickup); err == nil {
		t.Fatal("wrong caller read handover")
	}
}
