package providerauthusecases

import (
	"context"
	"errors"
	"testing"
	"time"

	providerauthmodels "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

type memChallenges struct{ saved []providerauthmodels.OTPChallenge }

func (m *memChallenges) Save(_ context.Context, c providerauthmodels.OTPChallenge, _, _ time.Duration, _ int) error {
	m.saved = append(m.saved, c)
	return nil
}
func (m *memChallenges) Get(context.Context, string) (providerauthmodels.OTPChallenge, bool, error) {
	return providerauthmodels.OTPChallenge{}, false, nil
}
func (m *memChallenges) RecordFailedAttempt(context.Context, providerauthmodels.OTPChallenge, time.Duration) error {
	return nil
}
func (m *memChallenges) Delete(context.Context, string) error { return nil }

type recordingSender struct {
	kind, to, code string
	err            error
}

func (r *recordingSender) SendLoginCode(_ context.Context, kind, to, code string) error {
	r.kind, r.to, r.code = kind, to, code
	return r.err
}

func startService(sender OTPSender) *AuthService {
	return NewAuthService(Options{
		Challenges:     &memChallenges{},
		OTPSecret:      []byte("test-otp-secret"),
		OTPTTL:         5 * time.Minute,
		OTPRateWindow:  time.Minute,
		OTPMaxRequests: 5,
		OTPSender:      sender,
	})
}

func TestStartAuth_EmailCodeIsSentAndNotReturned(t *testing.T) {
	sender := &recordingSender{}
	res, err := startService(sender).StartAuth(context.Background(), StartAuthInput{Email: " Ada@Example.com "})
	if err != nil {
		t.Fatal(err)
	}
	if sender.kind != "email" || sender.to != "ada@example.com" || len(sender.code) != 6 {
		t.Fatalf("code not sent to the normalised email: %+v", sender)
	}
	if res.DebugOTP != "" {
		t.Fatal("the code must not be returned outside debug mode")
	}
}

func TestStartAuth_DeliveryFailureIsReported(t *testing.T) {
	sender := &recordingSender{err: errors.New("notification-service down")}
	_, err := startService(sender).StartAuth(context.Background(), StartAuthInput{Email: "ada@example.com"})
	if appErr := apperrors.From(err); appErr == nil || appErr.Status != 503 {
		t.Fatalf("want a 503 so the app says try again, got %v", err)
	}
}
