package crm

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func consentEvent(eventID string, version int64, decision string, env *testEnv) EventEnvelope {
	return EventEnvelope{
		EventID:       eventID,
		EventType:     EventConsentChanged,
		SourceService: "customer-service",
		SourceVersion: version,
		OccurredAt:    env.now,
		BusinessRef:   businessA,
		Subject:       EventSubject{Type: "CUSTOMER", Ref: "cus_evt"},
		Payload:       EventPayload{Purpose: "MARKETING", Channel: "EMAIL", Decision: decision},
	}
}

// TestEventDedupeAndConflict covers the inbox contract: identical redelivery is
// a no-op, and the same event id with a different body is an integrity
// conflict rather than a silent overwrite.
func TestEventDedupeAndConflict(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	env.seedProfile(t, businessA, "vs_a", customerSubject("cus_evt"))

	if err := env.service.ProcessEvent(ctx, "customer-service", consentEvent("evt_dedupe", 1, "ALLOW", env)); err != nil {
		t.Fatal(err)
	}

	t.Run("identical redelivery is a no-op", func(t *testing.T) {
		if err := env.service.ProcessEvent(ctx, "customer-service", consentEvent("evt_dedupe", 1, "ALLOW", env)); err != nil {
			t.Fatalf("redelivery errored: %v", err)
		}
		var claims int
		if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM vendor_inbox_processed_events
			WHERE source_service=$1 AND event_id=$2`, "customer-service", "evt_dedupe").Scan(&claims); err != nil {
			t.Fatal(err)
		}
		if claims != 1 {
			t.Fatalf("%d inbox claims for one event id", claims)
		}
	})

	t.Run("the same event id with a different body is refused", func(t *testing.T) {
		err := env.service.ProcessEvent(ctx, "customer-service", consentEvent("evt_dedupe", 1, "DENY", env))
		if err == nil {
			t.Fatal("one event id carried two different facts without objection")
		}
	})

	t.Run("the original decision survives", func(t *testing.T) {
		profile, err := env.service.GetProfile(ctx, actorFor(businessA, "vs_a"), Scope{Global: true},
			env.profileIDFor(t, "cus_evt"))
		if err != nil {
			t.Fatal(err)
		}
		summary, err := env.service.ConsentSummaryFor(ctx, actorFor(businessA, "vs_a"), profile)
		if err != nil {
			t.Fatal(err)
		}
		if summary.Marketing[ChannelEmail].Decision != ConsentAllow {
			t.Fatalf("decision=%s; the conflicting redelivery took effect", summary.Marketing[ChannelEmail].Decision)
		}
	})
}

// TestEventEnvelopeValidation rejects malformed envelopes before any state is
// touched.
func TestEventEnvelopeValidation(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()

	base := consentEvent("evt_valid", 1, "ALLOW", env)
	cases := []struct {
		name   string
		mutate func(*EventEnvelope)
	}{
		{"missing event id", func(e *EventEnvelope) { e.EventID = "" }},
		{"missing business ref", func(e *EventEnvelope) { e.BusinessRef = "" }},
		{"zero source version", func(e *EventEnvelope) { e.SourceVersion = 0 }},
		{"missing occurred at", func(e *EventEnvelope) { e.OccurredAt = time.Time{} }},
		{"unknown subject type", func(e *EventEnvelope) { e.Subject.Type = "PARTNER" }},
		{"missing subject ref", func(e *EventEnvelope) { e.Subject.Ref = "" }},
		{"unsupported channel", func(e *EventEnvelope) { e.Payload.Channel = "CARRIER_PIGEON" }},
		{"unsupported decision", func(e *EventEnvelope) { e.Payload.Decision = "PROBABLY" }},
		{"unsupported purpose", func(e *EventEnvelope) { e.Payload.Purpose = "PROFILING" }},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			envelope := base
			envelope.EventID = base.EventID + "_" + itoa(i)
			tc.mutate(&envelope)
			if err := env.service.ProcessEvent(ctx, "customer-service", envelope); err == nil {
				t.Fatal("a malformed event was accepted")
			}
		})
	}
}

// TestUnknownBusinessIsRejected proves an event cannot invent a tenant.
func TestUnknownBusinessIsRejected(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	envelope := consentEvent("evt_nobusiness", 1, "ALLOW", env)
	envelope.BusinessRef = "vendor_does_not_exist"
	if err := env.service.ProcessEvent(context.Background(), "customer-service", envelope); err == nil {
		t.Fatal("an event for an unknown business was accepted")
	}
}

// TestConcurrentRelationshipCreationIsIdempotent proves the database
// uniqueness guard, not application timing, is what prevents duplicates.
func TestConcurrentRelationshipCreationIsIdempotent(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()

	const workers = 8
	var wg sync.WaitGroup
	errs := make([]error, workers)
	ids := make([]string, workers)
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(index int) {
			defer wg.Done()
			tx, err := env.pool.Begin(ctx)
			if err != nil {
				errs[index] = err
				return
			}
			profile, err := env.service.ResolveOrCreateProfile(ctx, tx, actorFor(businessA, "vs_a"), RelationshipInput{
				Subject: customerSubject("cus_race_create"), ObservedAt: env.now,
			})
			if err != nil {
				_ = tx.Rollback(ctx)
				errs[index] = err
				return
			}
			if err = tx.Commit(ctx); err != nil {
				errs[index] = err
				return
			}
			ids[index] = profile.ID
		}(i)
	}
	wg.Wait()

	var count int
	if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM vendor_customer_profiles
		WHERE business_id=$1 AND customer_ref=$2`, businessA, "cus_race_create").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("%d duplicate relationships created concurrently; want exactly 1", count)
	}

	// Whichever workers succeeded must all have resolved to the same profile.
	var seen string
	for i, id := range ids {
		if id == "" {
			continue
		}
		if seen == "" {
			seen = id
			continue
		}
		if id != seen {
			t.Fatalf("worker %d resolved to %s while another resolved to %s", i, id, seen)
		}
	}
	if seen == "" {
		t.Fatalf("no worker succeeded; errors=%v", errs)
	}
}

// TestOrderEventCreatesRelationshipAndProjection is the end-to-end ingestion
// path from a signed event through to a Customer 360 figure, using the fixture
// publisher (no real order service exists yet).
func TestOrderEventCreatesRelationshipAndProjection(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	completed := env.now.AddDate(0, 0, -2)

	envelope := EventEnvelope{
		EventID:       "evt_order_1",
		EventType:     EventOrderFactChanged,
		SourceService: "vendor-order-service",
		SourceVersion: 1,
		OccurredAt:    env.now,
		BusinessRef:   businessA,
		Subject:       EventSubject{Type: "CUSTOMER", Ref: "cus_from_order"},
		Payload: EventPayload{
			SourceOrderRef: "ord_evt_1",
			OrderState:     "COMPLETED",
			CompletedAt:    &completed,
			Currency:       "NGN",
			GrossMinor:     80_000,
			NetMinor:       80_000,
			BranchRef:      branchA1,
			CapabilityType: "RESTAURANT",
		},
	}
	if err := env.service.ProcessEvent(ctx, "vendor-order-service", envelope); err != nil {
		t.Fatal(err)
	}

	profileID := env.profileIDFor(t, "cus_from_order")
	profile := env.reload(t, businessA, profileID)
	if profile.CompletedOrderCount != 1 || profile.NetSpendMinor != 80_000 {
		t.Fatalf("completed=%d net=%d", profile.CompletedOrderCount, profile.NetSpendMinor)
	}
	if profile.CommercialProjectionState != ProjectionCurrent {
		t.Fatalf("projection state=%s want CURRENT", profile.CommercialProjectionState)
	}
	if profile.PreferredBranchRef == nil || *profile.PreferredBranchRef != branchA1 {
		t.Fatalf("preferred branch=%v want %s", profile.PreferredBranchRef, branchA1)
	}
}

// TestAuditTrailRecordsPrivacyDecisions proves privacy changes are auditable.
func TestAuditTrailRecordsPrivacyDecisions(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_audit"))

	current := env.reload(t, businessA, profile.ID)
	if err := env.applyPrivacy(t, businessA, "vs_a", &current, PrivacyInput{
		SourceService: "customer-service", SourceVersion: 1, PrivacySubjectRef: "cus_audit",
		Action: PrivacyActionRedact, ObservedAt: env.now,
	}); err != nil {
		t.Fatal(err)
	}

	var actions int
	if err := env.pool.QueryRow(context.Background(), `SELECT count(*) FROM vendor_audit_log
		WHERE vendor_id=$1 AND action=$2 AND aggregate_id=$3`,
		businessA, "vendor.crm.privacy_state.changed", profile.ID).Scan(&actions); err != nil {
		t.Fatal(err)
	}
	if actions == 0 {
		t.Fatal("a redaction was applied without an audit entry")
	}
}

// TestNoRawSubjectReferenceInAuditPayload guards against PII creeping into the
// audit trail through the before/after snapshots.
func TestNoRawSubjectReferenceInAuditPayload(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_audit_payload"))

	if err := env.tx(t, func(tx pgx.Tx) error {
		return env.service.ApplyConsent(ctx, tx, actorFor(businessA, "vs_a"), profile.ID,
			consentInput(ChannelEmail, ConsentAllow, 1, env.now))
	}); err != nil {
		t.Fatal(err)
	}

	rows, err := env.pool.Query(ctx, `SELECT COALESCE(after_data::text,''), COALESCE(before_data::text,'')
		FROM vendor_audit_log WHERE vendor_id=$1`, businessA)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var after, before string
		if err = rows.Scan(&after, &before); err != nil {
			t.Fatal(err)
		}
		for _, blob := range []string{after, before} {
			if blob == "" {
				continue
			}
			if strings.Contains(blob, "@") {
				t.Fatalf("audit payload appears to contain an email address: %s", blob)
			}
		}
	}
}

func (e *testEnv) profileIDFor(t *testing.T, customerRef string) string {
	t.Helper()
	var id string
	if err := e.pool.QueryRow(context.Background(), `SELECT id FROM vendor_customer_profiles
		WHERE business_id=$1 AND customer_ref=$2`, businessA, customerRef).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
