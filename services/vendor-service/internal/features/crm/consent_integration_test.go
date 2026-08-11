package crm

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func (e *testEnv) applyConsent(t *testing.T, profileID string, in ConsentInput) error {
	t.Helper()
	return e.tx(t, func(tx pgx.Tx) error {
		return e.service.ApplyConsent(context.Background(), tx, actorFor(businessA, "vs_a"), profileID, in)
	})
}

func consentInput(channel ConsentChannel, decision ConsentDecision, version int64, observedAt time.Time) ConsentInput {
	return ConsentInput{
		SourceService: "customer-service",
		SourceVersion: version,
		Purpose:       ConsentPurposeMarketing,
		Channel:       channel,
		Decision:      decision,
		ObservedAt:    observedAt,
	}
}

// TestConsentDecisionMatrix walks every decision against real storage.
func TestConsentDecisionMatrix(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_matrix"))

	// EMAIL allows, PUSH denies, IN_APP is explicitly unknown, SMS has no row
	// at all — the absence case that must behave exactly like UNKNOWN.
	for _, seed := range []struct {
		channel  ConsentChannel
		decision ConsentDecision
	}{
		{ChannelEmail, ConsentAllow},
		{ChannelPush, ConsentDeny},
		{ChannelInApp, ConsentUnknown},
	} {
		if err := env.applyConsent(t, profile.ID, consentInput(seed.channel, seed.decision, 1, env.now)); err != nil {
			t.Fatal(err)
		}
	}

	summary, err := env.service.ConsentSummaryFor(ctx, actor, env.reload(t, businessA, profile.ID))
	if err != nil {
		t.Fatal(err)
	}

	expect := map[ConsentChannel]struct {
		decision ConsentDecision
		eligible bool
		reason   string
	}{
		ChannelEmail: {ConsentAllow, true, ReasonConsentAllowed},
		ChannelPush:  {ConsentDeny, false, ReasonConsentDenied},
		ChannelInApp: {ConsentUnknown, false, ReasonConsentUnknown},
		ChannelSMS:   {ConsentUnknown, false, ReasonConsentUnknown},
	}
	for channel, want := range expect {
		got, present := summary.Marketing[channel]
		if !present {
			t.Fatalf("channel %s missing from the summary; a missing key must never be mistaken for permission", channel)
		}
		if got.Decision != want.decision || got.Eligible != want.eligible || got.Reason != want.reason {
			t.Fatalf("channel %s decision=%s eligible=%t reason=%s; want %s/%t/%s",
				channel, got.Decision, got.Eligible, got.Reason, want.decision, want.eligible, want.reason)
		}
	}
}

// TestConsentSourceVersionRules covers stale, duplicate, conflicting and newer
// deliveries.
func TestConsentSourceVersionRules(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_versions"))

	if err := env.applyConsent(t, profile.ID, consentInput(ChannelEmail, ConsentAllow, 5, env.now)); err != nil {
		t.Fatal(err)
	}

	t.Run("a stale version is rejected", func(t *testing.T) {
		if err := env.applyConsent(t, profile.ID, consentInput(ChannelEmail, ConsentDeny, 2, env.now)); err == nil {
			t.Fatal("a stale consent version overwrote a newer decision")
		}
		if decision := env.currentDecision(t, profile.ID, ChannelEmail); decision != ConsentAllow {
			t.Fatalf("decision=%s; the stale version took effect", decision)
		}
	})

	t.Run("an identical redelivery is a no-op", func(t *testing.T) {
		if err := env.applyConsent(t, profile.ID, consentInput(ChannelEmail, ConsentAllow, 5, env.now)); err != nil {
			t.Fatalf("identical redelivery errored: %v", err)
		}
	})

	t.Run("the same version with a different decision is a conflict", func(t *testing.T) {
		if err := env.applyConsent(t, profile.ID, consentInput(ChannelEmail, ConsentDeny, 5, env.now)); err == nil {
			t.Fatal("conflicting decisions at the same version were silently resolved")
		}
		if decision := env.currentDecision(t, profile.ID, ChannelEmail); decision != ConsentAllow {
			t.Fatalf("decision=%s after a conflict; neither side should win silently", decision)
		}
	})

	t.Run("a newer version applies", func(t *testing.T) {
		if err := env.applyConsent(t, profile.ID, consentInput(ChannelEmail, ConsentDeny, 6, env.now)); err != nil {
			t.Fatal(err)
		}
		if decision := env.currentDecision(t, profile.ID, ChannelEmail); decision != ConsentDeny {
			t.Fatalf("decision=%s want DENY", decision)
		}
	})
}

// TestConsentDenyTakesEffectImmediately proves a withdrawal is honoured as soon
// as it commits.
func TestConsentDenyTakesEffectImmediately(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_withdraw"))

	if err := env.applyConsent(t, profile.ID, consentInput(ChannelEmail, ConsentAllow, 1, env.now)); err != nil {
		t.Fatal(err)
	}
	summary, err := env.service.ConsentSummaryFor(ctx, actor, env.reload(t, businessA, profile.ID))
	if err != nil {
		t.Fatal(err)
	}
	if !summary.Marketing[ChannelEmail].Eligible {
		t.Fatal("an explicit ALLOW was not eligible")
	}

	if err = env.applyConsent(t, profile.ID, consentInput(ChannelEmail, ConsentDeny, 2, env.now)); err != nil {
		t.Fatal(err)
	}
	summary, err = env.service.ConsentSummaryFor(ctx, actor, env.reload(t, businessA, profile.ID))
	if err != nil {
		t.Fatal(err)
	}
	entry := summary.Marketing[ChannelEmail]
	if entry.Eligible || entry.Reason != ReasonConsentDenied {
		t.Fatalf("after DENY: eligible=%t reason=%s", entry.Eligible, entry.Reason)
	}
}

// TestStaleConsentProjectionFailsClosed proves a silently dead consent feed
// degrades to "cannot target" rather than to a cached ALLOW.
func TestStaleConsentProjectionFailsClosed(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_stale_consent"))

	if err := env.applyConsent(t, profile.ID, consentInput(ChannelEmail, ConsentAllow, 1, env.now)); err != nil {
		t.Fatal(err)
	}

	env.now = env.now.Add(ConsentStaleAfter + time.Hour)
	summary, err := env.service.ConsentSummaryFor(ctx, actor, env.reload(t, businessA, profile.ID))
	if err != nil {
		t.Fatal(err)
	}
	entry := summary.Marketing[ChannelEmail]
	if entry.Eligible {
		t.Fatal("a stale ALLOW remained targetable")
	}
	if !entry.Stale || entry.Reason != ReasonConsentStale {
		t.Fatalf("stale=%t reason=%s", entry.Stale, entry.Reason)
	}
}

// TestExpiredConsentIsUnknown proves a time-boxed ALLOW lapses.
func TestExpiredConsentIsUnknown(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_expiry"))

	expires := env.now.Add(-time.Hour)
	input := consentInput(ChannelEmail, ConsentAllow, 1, env.now)
	input.ExpiresAt = &expires
	if err := env.applyConsent(t, profile.ID, input); err != nil {
		t.Fatal(err)
	}

	summary, err := env.service.ConsentSummaryFor(ctx, actor, env.reload(t, businessA, profile.ID))
	if err != nil {
		t.Fatal(err)
	}
	entry := summary.Marketing[ChannelEmail]
	if entry.Decision != ConsentUnknown || entry.Eligible {
		t.Fatalf("expired ALLOW: decision=%s eligible=%t", entry.Decision, entry.Eligible)
	}
}

// TestConsentIsScopedPerChannel proves one channel's decision never leaks into
// another's.
func TestConsentIsScopedPerChannel(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_scope"))

	if err := env.applyConsent(t, profile.ID, consentInput(ChannelEmail, ConsentAllow, 1, env.now)); err != nil {
		t.Fatal(err)
	}
	summary, err := env.service.ConsentSummaryFor(ctx, actor, env.reload(t, businessA, profile.ID))
	if err != nil {
		t.Fatal(err)
	}
	if !summary.Marketing[ChannelEmail].Eligible {
		t.Fatal("email should be eligible")
	}
	for _, channel := range []ConsentChannel{ChannelPush, ChannelInApp, ChannelSMS} {
		if summary.Marketing[channel].Eligible {
			t.Fatalf("an email ALLOW widened into %s", channel)
		}
	}
}

func (e *testEnv) currentDecision(t *testing.T, profileID string, channel ConsentChannel) ConsentDecision {
	t.Helper()
	var decision ConsentDecision
	if err := e.pool.QueryRow(context.Background(), `SELECT decision FROM crm_consent_projections
		WHERE business_id=$1 AND profile_id=$2 AND channel=$3`, businessA, profileID, channel).Scan(&decision); err != nil {
		t.Fatal(err)
	}
	return decision
}
