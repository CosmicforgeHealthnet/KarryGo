package providerauthusecases

import (
	"context"
	"errors"
	"testing"

	"cosmicforge/logistics/shared/go/apperrors"
)

type recordingSender struct {
	kind, to, code string
	err            error
}

func (r *recordingSender) SendLoginCode(_ context.Context, kind, to, code string) error {
	r.kind, r.to, r.code = kind, to, code
	return r.err
}

func TestEmailLoginSendsCodeAndCreatesEmailDriver(t *testing.T) {
	svc, providers, _, _ := newTestService(t, false)
	sender := &recordingSender{}
	svc.otpSender = sender
	ctx := context.Background()

	start, err := svc.StartAuth(ctx, StartAuthInput{Email: " Ada@Example.com "})
	if err != nil {
		t.Fatalf("StartAuth: %v", err)
	}
	if sender.kind != "email" || sender.to != "ada@example.com" || len(sender.code) != 6 {
		t.Fatalf("code not emailed to the normalised address: %+v", sender)
	}
	if start.DebugOTP != "" {
		t.Fatal("the code must not be returned outside debug mode")
	}

	tokens, err := svc.VerifyAuth(ctx, VerifyAuthInput{
		Email: "ada@example.com", OTP: sender.code, ChallengeID: start.ChallengeID,
	})
	if err != nil {
		t.Fatalf("VerifyAuth: %v", err)
	}
	if tokens.Provider.Email != "ada@example.com" || tokens.Provider.Phone != "" {
		t.Fatalf("expected an email-only driver, got %+v", tokens.Provider)
	}
	if len(providers.byID) != 1 {
		t.Fatalf("expected one driver, got %d", len(providers.byID))
	}
}

func TestEmailCodeCannotVerifyAPhone(t *testing.T) {
	svc, _, _, _ := newTestService(t, true)
	ctx := context.Background()
	start, err := svc.StartAuth(ctx, StartAuthInput{Email: "ada@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.VerifyAuth(ctx, VerifyAuthInput{
		Phone: "08011112222", OTP: start.DebugOTP, ChallengeID: start.ChallengeID,
	}); err == nil {
		t.Fatal("a code issued for an email must not sign in a phone number")
	}
}

func TestStartAuthRejectsBadEmailAndReportsDeliveryFailure(t *testing.T) {
	svc, _, _, _ := newTestService(t, false)
	if _, err := svc.StartAuth(context.Background(), StartAuthInput{Email: "not-an-email"}); err == nil {
		t.Fatal("expected a validation error")
	}
	svc.otpSender = &recordingSender{err: errors.New("down")}
	_, err := svc.StartAuth(context.Background(), StartAuthInput{Email: "ada@example.com"})
	if appErr := apperrors.From(err); appErr == nil || appErr.Status != 503 {
		t.Fatalf("want 503 when the email cannot be sent, got %v", err)
	}
}
