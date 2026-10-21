package providerauthusecases

import (
	"context"
	"testing"

	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/phonenumber"
)

func TestTestLoginNumberSignsInWithFixedCode(t *testing.T) {
	svc, _, _, _ := newTestService(t, false)
	tl, err := sharedauth.ParseTestLogin("staging", "08000003001", "246810", phonenumber.NormalizeNigerianPhoneNumber)
	if err != nil {
		t.Fatal(err)
	}
	svc.testLogin = tl
	ctx := context.Background()

	start, err := svc.StartAuth(ctx, StartAuthInput{Phone: "08000003001"})
	if err != nil {
		t.Fatal(err)
	}
	if start.DebugOTP != "" {
		t.Fatal("the fixed code is never echoed back")
	}
	if _, err := svc.VerifyAuth(ctx, VerifyAuthInput{Phone: "08000003001", OTP: "246810", ChallengeID: start.ChallengeID}); err != nil {
		t.Fatalf("listed test number should sign in with the fixed code: %v", err)
	}

	other, err := svc.StartAuth(ctx, StartAuthInput{Phone: "08011112222"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.VerifyAuth(ctx, VerifyAuthInput{Phone: "08011112222", OTP: "246810", ChallengeID: other.ChallengeID}); err == nil {
		t.Fatal("an unlisted number must not accept the fixed code")
	}
}
