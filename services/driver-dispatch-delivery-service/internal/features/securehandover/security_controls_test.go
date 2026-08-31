package securehandover

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type scopeLimiter struct{ keys []string }

func (s *scopeLimiter) Allow(_ context.Context, key string, _ int, _ time.Duration) error {
	s.keys = append(s.keys, key)
	if len(s.keys) == 3 {
		return ErrLocked
	}
	return nil
}

func TestIssueLimitsBookingRecipientAndSourceBeforePersistence(t *testing.T) {
	limiter := &scopeLimiter{}
	s := &Service{policy: Policy{Enabled: true}, limiter: limiter}
	_, _, err := s.issue(context.Background(), Context{BookingID: "booking-a", SourceType: "VENDOR_COMMERCE", SourceRef: "source-a"}, StagePickup, "SENDER", "recipient-a", "caller", "operation", false)
	if apperrors.From(err).Status != 429 {
		t.Fatalf("limit not enforced: %v", err)
	}
	want := []string{"issue:booking:booking-a", "issue:recipient:recipient-a", "issue:source:VENDOR_COMMERCE:source-a"}
	for i := range want {
		if limiter.keys[i] != want[i] {
			t.Fatal("missing scope")
		}
	}
}

func TestMissingLimiterAndDisabledQRFailClosed(t *testing.T) {
	s := &Service{policy: Policy{Enabled: true}}
	if err := s.limit(context.Background(), "provider-a", 1, time.Minute); apperrors.From(err).Status != 503 {
		t.Fatal("missing limiter accepted")
	}
	_, err := s.verify(context.Background(), Context{}, Actor{}, StagePickup, VerifyCommand{ChallengeRef: "challenge", Credential: "opaque", Method: MethodQR, ManifestRef: "manifest", ManifestVersion: 1, OperationRef: "operation"})
	if apperrors.From(err).Status != 503 {
		t.Fatal("disabled QR accepted")
	}
}

func TestRedisConcurrentAttemptsHaveOneSharedAllowance(t *testing.T) {
	server := miniredis.RunT(t)
	a := redis.NewClient(&redis.Options{Addr: server.Addr()})
	b := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer a.Close()
	defer b.Close()
	limiters := []*RedisRateLimiter{NewRedisRateLimiter(a), NewRedisRateLimiter(b)}
	out := make(chan error, 40)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			out <- limiters[n%2].Allow(context.Background(), "verify:shared-challenge", 5, time.Minute)
		}(i)
	}
	wg.Wait()
	close(out)
	allowed := 0
	for err := range out {
		if err == nil {
			allowed++
		} else if !errors.Is(err, ErrLocked) {
			t.Fatal(err)
		}
	}
	if allowed != 5 {
		t.Fatalf("allowed %d want 5", allowed)
	}
	server.Close()
	if err := limiters[0].Allow(context.Background(), "verify:shared-challenge", 5, time.Minute); err == nil {
		t.Fatal("Redis outage failed open")
	}
}

func TestSecretBearingIdempotencyFingerprintIsKeyed(t *testing.T) {
	a, _ := newCredentialCipher([]byte("first-handover-secret-key"))
	b, _ := newCredentialCipher([]byte("second-handover-secret-key"))
	command := VerifyCommand{Credential: "123456", OperationRef: "same"}
	if a.requestFingerprint(command) == fingerprint(command) || a.requestFingerprint(command) == b.requestFingerprint(command) {
		t.Fatal("credential fingerprint permits unkeyed guessing")
	}
	if a.requestFingerprint(command) != a.requestFingerprint(command) {
		t.Fatal("fingerprint not deterministic")
	}
}
