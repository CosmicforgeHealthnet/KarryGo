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
