package integration

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/services/pricing-service/internal/platform/audit"
	"cosmicforge/logistics/services/pricing-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/pricing-service/internal/platform/operators"
	"cosmicforge/logistics/services/pricing-service/internal/platform/servicetrust"
)

// TestAuditTrailIsAbsolutelyAppendOnly is the most important test in this file.
//
// The audit trail is the evidence that a price change was authorised. If it can
// be edited or deleted, then nothing else in this service can be trusted either.
// It must resist UPDATE, DELETE, and DELETE while the retention-purge flag is
// set -- the last case is the one that was broken when this migration was first
// written, and it is pinned here so it cannot come back.
func TestAuditTrailIsAbsolutelyAppendOnly(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	subjectID := unique("pv")
	recorder := audit.NewPostgresRecorder()
	if err := recorder.Record(ctx, pool, audit.Entry{
		ActorCaller: "admin-backoffice-service",
		PerformedBy: "ada@karrygo",
		Action:      "POLICY_VERSION_APPROVED",
		SubjectType: "policy_version",
		SubjectID:   subjectID,
		Reason:      "quarterly fuel cost review",
		Before:      map[string]any{"status": "SUBMITTED"},
		After:       map[string]any{"status": "APPROVED"},
		RequestID:   "req-1",
	}); err != nil {
		t.Fatalf("record: %v", err)
	}

	mutations := []struct {
		name string
		sql  string
	}{
		{"update the reason", `UPDATE pricing_audit_log SET reason = 'tampered' WHERE subject_id = $1`},
		{"delete the row", `DELETE FROM pricing_audit_log WHERE subject_id = $1`},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, mutation.sql, subjectID); err == nil {
				t.Fatal("the audit trail accepted a mutation; it must be append-only")
			}
		})
	}

	// The retention purge exists for expired unbound quotes. It must not reach
	// the audit trail, even though it is the same service setting the flag.
	t.Run("delete under the retention purge flag", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()

		if _, err := tx.Exec(ctx, `SET LOCAL pricing.allow_purge = 'on'`); err != nil {
			t.Fatalf("set flag: %v", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM pricing_audit_log WHERE subject_id = $1`, subjectID); err == nil {
			t.Fatal("the purge flag deleted an audit row; the audit guard must have no escape hatch")
		}
	})

	// The row is still there and still says what it said.
	var reason string
	if err := pool.QueryRow(ctx,
		`SELECT reason FROM pricing_audit_log WHERE subject_id = $1`, subjectID,
	).Scan(&reason); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if reason != "quarterly fuel cost review" {
		t.Fatalf("reason = %q, want the original text", reason)
	}
}

// TestAuditEntryRequiresItsEvidence checks both layers: the Go validation that
// gives a clear message, and the database CHECK that catches anything bypassing
// it.
func TestAuditEntryRequiresItsEvidence(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	recorder := audit.NewPostgresRecorder()

	complete := audit.Entry{
		ActorCaller: "admin-backoffice-service",
		PerformedBy: "ada@karrygo",
		Action:      "POLICY_VERSION_APPROVED",
		SubjectType: "policy_version",
		SubjectID:   unique("pv"),
		Reason:      "because",
	}

	missing := map[string]func(audit.Entry) audit.Entry{
		"actor_caller": func(e audit.Entry) audit.Entry { e.ActorCaller = ""; return e },
		"performed_by": func(e audit.Entry) audit.Entry { e.PerformedBy = ""; return e },
		"action":       func(e audit.Entry) audit.Entry { e.Action = ""; return e },
		"subject_type": func(e audit.Entry) audit.Entry { e.SubjectType = ""; return e },
		"subject_id":   func(e audit.Entry) audit.Entry { e.SubjectID = ""; return e },
		"reason":       func(e audit.Entry) audit.Entry { e.Reason = ""; return e },
	}
	for field, strip := range missing {
		t.Run("without "+field, func(t *testing.T) {
			err := recorder.Record(ctx, pool, strip(complete))
			if !errors.Is(err, audit.ErrIncomplete) {
				t.Fatalf("error = %v, want ErrIncomplete", err)
			}
			if !strings.Contains(err.Error(), field) {
				t.Fatalf("error should name %s, got: %v", field, err)
			}
		})
	}

	// The database refuses an empty reason even when Go validation is bypassed.
	t.Run("the database also refuses it", func(t *testing.T) {
		_, err := pool.Exec(ctx, `
			INSERT INTO pricing_audit_log (actor_caller, performed_by, action, subject_type, subject_id, reason)
			VALUES ('a', 'b', 'c', 'd', $1, '')`, unique("pv"))
		if err == nil {
			t.Fatal("the database accepted an empty reason")
		}
	})
}

// TestOperatorRolesAreConstrained protects the separation of duties: an unknown
// role must be refused rather than stored and silently ignored by later checks.
func TestOperatorRolesAreConstrained(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	store := operators.NewStore(pool)

	editorID := unique("editor")
	if _, err := pool.Exec(ctx,
		`INSERT INTO pricing_operators (operator_id, roles) VALUES ($1, '{EDITOR}')`, editorID,
	); err != nil {
		t.Fatalf("insert editor: %v", err)
	}

	t.Run("an invented role is refused", func(t *testing.T) {
		_, err := pool.Exec(ctx,
			`INSERT INTO pricing_operators (operator_id, roles) VALUES ($1, '{SUPER_ADMIN}')`, unique("bad"))
		if err == nil {
			t.Fatal("the database accepted an unknown role")
		}
	})

	t.Run("an editor may edit but not approve", func(t *testing.T) {
		if _, err := store.Require(ctx, editorID, operators.RoleEditor); err != nil {
			t.Fatalf("editor should hold EDITOR: %v", err)
		}
		_, err := store.Require(ctx, editorID, operators.RoleApprover)
		if !errors.Is(err, operators.ErrMissingRole) {
			t.Fatalf("error = %v, want ErrMissingRole: proposing a change must never imply approving it", err)
		}
	})

	t.Run("an unknown operator is refused, not treated as unprivileged", func(t *testing.T) {
		_, err := store.Get(ctx, "nobody@example.com")
		if !errors.Is(err, operators.ErrUnknownOperator) {
			t.Fatalf("error = %v, want ErrUnknownOperator", err)
		}
	})

	t.Run("an empty performed_by is refused", func(t *testing.T) {
		if _, err := store.Get(ctx, "  "); !errors.Is(err, operators.ErrUnknownOperator) {
			t.Fatalf("error = %v, want ErrUnknownOperator", err)
		}
	})

	t.Run("a deactivated operator loses authority", func(t *testing.T) {
		if _, err := pool.Exec(ctx,
			`UPDATE pricing_operators SET active = false WHERE operator_id = $1`, editorID); err != nil {
			t.Fatalf("deactivate: %v", err)
		}
		if _, err := store.Get(ctx, editorID); !errors.Is(err, operators.ErrUnknownOperator) {
			t.Fatalf("error = %v, want ErrUnknownOperator once inactive", err)
		}
	})
}

// TestNoncesMakeSignedRequestsSingleUse pins the replay guard. Replaying a quote
// request would mint a second sealed quote for one intent.
func TestNoncesMakeSignedRequestsSingleUse(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	nonce := unique("nonce")
	insert := func() int64 {
		tag, err := pool.Exec(ctx, `
			INSERT INTO pricing_service_nonces (caller, nonce, expires_at)
			VALUES ($1, $2, now() + interval '15 minutes')
			ON CONFLICT DO NOTHING`, "taxi-service", nonce)
		if err != nil {
			t.Fatalf("insert nonce: %v", err)
		}
		return tag.RowsAffected()
	}

	if got := insert(); got != 1 {
		t.Fatalf("first use affected %d rows, want 1", got)
	}
	if got := insert(); got != 0 {
		t.Fatalf("replay affected %d rows, want 0 (the request must be refused)", got)
	}

	// The same nonce from a different caller is a different request.
	tag, err := pool.Exec(ctx, `
		INSERT INTO pricing_service_nonces (caller, nonce, expires_at)
		VALUES ($1, $2, now() + interval '15 minutes')
		ON CONFLICT DO NOTHING`, "api-gateway", nonce)
	if err != nil {
		t.Fatalf("insert for second caller: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatal("a nonce is scoped per caller; a different caller must not collide")
	}

	// Pruning removes only expired nonces.
	expired := unique("expired")
	if _, err := pool.Exec(ctx,
		`INSERT INTO pricing_service_nonces (caller, nonce, expires_at) VALUES ('taxi-service', $1, now() - interval '1 hour')`,
		expired); err != nil {
		t.Fatalf("insert expired: %v", err)
	}
	if err := servicetrust.PruneNonces(ctx, pool); err != nil {
		t.Fatalf("prune: %v", err)
	}

	var liveCount, expiredCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM pricing_service_nonces WHERE nonce = $1`, nonce).Scan(&liveCount); err != nil {
		t.Fatalf("count live: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM pricing_service_nonces WHERE nonce = $1`, expired).Scan(&expiredCount); err != nil {
		t.Fatalf("count expired: %v", err)
	}
	if liveCount != 2 {
		t.Fatalf("live nonces = %d, want 2 (pruning must not remove unexpired nonces)", liveCount)
	}
	if expiredCount != 0 {
		t.Fatalf("expired nonces = %d, want 0", expiredCount)
	}
}

// TestIdempotentAdminCommands covers the three outcomes a retry can have.
func TestIdempotentAdminCommands(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	store := idempotency.NewStore(pool, time.Hour)

	const (
		caller   = "admin-backoffice-service"
		endpoint = "POST /admin/policy-versions"
	)
	key := unique("key")
	body := []byte(`{"policy_id":"taxi.tariff.NG","reason":"fuel review"}`)
	fingerprint := idempotency.Fingerprint("POST", "/admin/policy-versions", body)

	t.Run("the first call proceeds", func(t *testing.T) {
		outcome, err := store.Begin(ctx, caller, endpoint, key, fingerprint)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		if !outcome.Proceed {
			t.Fatal("the first call must own the claim")
		}
	})

	t.Run("an identical call while in progress is told to wait", func(t *testing.T) {
		_, err := store.Begin(ctx, caller, endpoint, key, fingerprint)
		if !errors.Is(err, idempotency.ErrInProgress) {
			t.Fatalf("error = %v, want ErrInProgress", err)
		}
	})

	t.Run("a different command under the same key is refused", func(t *testing.T) {
		other := idempotency.Fingerprint("POST", "/admin/policy-versions", []byte(`{"policy_id":"dispatch.tariff.NG"}`))
		_, err := store.Begin(ctx, caller, endpoint, key, other)
		if !errors.Is(err, idempotency.ErrFingerprintMismatch) {
			t.Fatalf("error = %v, want ErrFingerprintMismatch: a reused key must never execute a different command", err)
		}
	})

	t.Run("after completion a retry replays the stored response", func(t *testing.T) {
		response := []byte(`{"policy_version_id":"pv_42","status":"DRAFT"}`)
		if err := store.Complete(ctx, pool, caller, endpoint, key, response); err != nil {
			t.Fatalf("complete: %v", err)
		}
		outcome, err := store.Begin(ctx, caller, endpoint, key, fingerprint)
		if err != nil {
			t.Fatalf("begin after complete: %v", err)
		}
		if outcome.Proceed {
			t.Fatal("a completed command must not run twice")
		}
		if string(outcome.Response) != string(response) {
			t.Fatalf("replayed response = %s, want %s", outcome.Response, response)
		}
	})

	t.Run("releasing an in-flight claim allows a genuine retry", func(t *testing.T) {
		releaseKey := unique("release")
		if _, err := store.Begin(ctx, caller, endpoint, releaseKey, fingerprint); err != nil {
			t.Fatalf("begin: %v", err)
		}
		if err := store.Release(ctx, caller, endpoint, releaseKey); err != nil {
			t.Fatalf("release: %v", err)
		}
		outcome, err := store.Begin(ctx, caller, endpoint, releaseKey, fingerprint)
		if err != nil {
			t.Fatalf("begin after release: %v", err)
		}
		if !outcome.Proceed {
			t.Fatal("a released claim must be retryable")
		}
	})

	t.Run("a completed claim cannot be released", func(t *testing.T) {
		if err := store.Release(ctx, caller, endpoint, key); err != nil {
			t.Fatalf("release: %v", err)
		}
		outcome, err := store.Begin(ctx, caller, endpoint, key, fingerprint)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		if outcome.Proceed {
			t.Fatal("releasing must not resurrect a completed command")
		}
	})
}
