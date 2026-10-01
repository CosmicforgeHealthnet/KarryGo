package jobs

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSection14HDurableDefaultsAreBounded(
	t *testing.T,
) {
	settings :=
		(DurableJob{}).settings()

	if settings.interval != 30*time.Second {
		t.Fatalf(
			"interval=%s",
			settings.interval,
		)
	}

	if settings.leaseFor != 2*time.Minute {
		t.Fatalf(
			"lease=%s",
			settings.leaseFor,
		)
	}

	if settings.maxAttempts != 5 {
		t.Fatalf(
			"attempts=%d",
			settings.maxAttempts,
		)
	}

	if settings.pollEvery > 5*time.Second {
		t.Fatalf(
			"poll=%s",
			settings.pollEvery,
		)
	}
}

func TestSection14HRetryBackoffIsBounded(
	t *testing.T,
) {
	got :=
		boundedRetryDelay(
			time.Second,
			20,
		)

	if got > 5*time.Minute {
		t.Fatalf(
			"retry=%s",
			got,
		)
	}
}

func TestSection14HWorkerErrorIsBounded(
	t *testing.T,
) {
	got :=
		redactedWorkerError(
			errors.New(
				strings.Repeat(
					"x",
					1500,
				) +
					"\nnext",
			),
		)

	if len(got) > 1000 {
		t.Fatalf(
			"length=%d",
			len(got),
		)
	}

	if strings.Contains(
		got,
		"\n",
	) {
		t.Fatal(
			"worker error contains newline",
		)
	}
}

func TestDurableWorkerErrorUsesEventCredentialRedaction(t *testing.T) {
	got := redactedWorkerError(errors.New(
		"upstream authorization: Bearer token-secret otp=123456 qr=qr-secret private_key=key-secret",
	))
	for _, secret := range []string{"token-secret", "123456", "qr-secret", "key-secret"} {
		if strings.Contains(got, secret) {
			t.Fatalf("worker error leaked %q: %s", secret, got)
		}
	}
}

func TestWorkerHealthNeverReportsBlockedOrUnavailableWorkReady(t *testing.T) {
	cases := []struct {
		name    string
		enabled bool
		health  WorkerHealth
		want    string
	}{
		{"disabled retains backlog visibility", false, WorkerHealth{PendingCount: 4}, WorkerDisabled},
		{"dead letter blocks", true, WorkerHealth{IntegrationAvailable: true, DeadLetterCount: 1}, WorkerBlocked},
		{"unavailable dependency with backlog blocks", true, WorkerHealth{PendingCount: 1}, WorkerBlocked},
		{"unavailable dependency degrades", true, WorkerHealth{}, WorkerDegraded},
		{"starved lease degrades", true, WorkerHealth{IntegrationAvailable: true, LeaseStarvationCount: 1}, WorkerDegraded},
		{"retry degrades", true, WorkerHealth{IntegrationAvailable: true, RetryCount: 1}, WorkerDegraded},
		{"pending is visible", true, WorkerHealth{IntegrationAvailable: true, PendingCount: 1}, WorkerBacklogged},
		{"empty healthy queue", true, WorkerHealth{IntegrationAvailable: true}, WorkerReady},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyWorkerHealth(tc.enabled, tc.health); got != tc.want {
				t.Fatalf("state=%s want=%s", got, tc.want)
			}
		})
	}
}
