package crm

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

func (e *testEnv) applyPrivacy(t *testing.T, business, staff string, profile *Profile, in PrivacyInput) error {
	t.Helper()
	return e.tx(t, func(tx pgx.Tx) error {
		return e.service.ApplyPrivacy(context.Background(), tx, actorFor(business, staff), profile, in)
	})
}

// TestRedactionRemovesTargetabilityAndVisibility proves a redaction actually
// takes effect across every surface, rather than only setting a flag.
func TestRedactionRemovesTargetabilityAndVisibility(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	global := Scope{Global: true}

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_redact"))
	env.seedOrder(t, businessA, "vs_a", profile, "ord_r1", branchA1, 20_000, 20_000, env.now.AddDate(0, 0, -4), 1)

	// Give it an explicit marketing ALLOW so the test proves privacy beats it.
	if err := env.tx(t, func(tx pgx.Tx) error {
		return env.service.ApplyConsent(ctx, tx, actor, profile.ID, ConsentInput{
			SourceService: "customer-service", SourceVersion: 1, Purpose: ConsentPurposeMarketing,
			Channel: ChannelEmail, Decision: ConsentAllow, ObservedAt: env.now,
		})
	}); err != nil {
		t.Fatal(err)
	}

	current := env.reload(t, businessA, profile.ID)
	if err := env.applyPrivacy(t, businessA, "vs_a", &current, PrivacyInput{
		SourceService: "customer-service", SourceVersion: 9, PrivacySubjectRef: "cus_redact",
		Action: PrivacyActionRedact, ObservedAt: env.now,
	}); err != nil {
		t.Fatal(err)
	}

	redacted := env.reload(t, businessA, profile.ID)
	if redacted.PrivacyState != PrivacyRedacted || redacted.RedactedAt == nil {
		t.Fatalf("privacy_state=%s redacted_at=%v", redacted.PrivacyState, redacted.RedactedAt)
	}
	if redacted.SafeDisplayLabel != nil {
		t.Fatal("a redacted profile retained a display label")
	}

	t.Run("excluded from ordinary search", func(t *testing.T) {
		profiles, _, err := env.service.ListProfiles(ctx, actor, global, ListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for _, candidate := range profiles {
			if candidate.ID == profile.ID {
				t.Fatal("a redacted relationship appeared in ordinary search")
			}
		}
	})

	t.Run("not readable through the ordinary route", func(t *testing.T) {
		if _, err := env.service.GetProfile(ctx, actor, global, profile.ID); err == nil {
			t.Fatal("a redacted relationship was readable")
		}
	})

	t.Run("cannot be archived or restored to launder the redaction", func(t *testing.T) {
		if _, err := env.service.SetArchived(ctx, actor, global, profile.ID, true, redacted.Version); err == nil {
			t.Fatal("a redacted relationship was archived")
		}
		if _, err := env.service.SetArchived(ctx, actor, global, profile.ID, false, redacted.Version); err == nil {
			t.Fatal("a redacted relationship was restored")
		}
	})

	t.Run("branch targeting associations are removed", func(t *testing.T) {
		var relations int
		if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_customer_branch_relations
			WHERE business_id=$1 AND profile_id=$2`, businessA, profile.ID).Scan(&relations); err != nil {
			t.Fatal(err)
		}
		if relations != 0 {
			t.Fatalf("%d branch relations survived redaction", relations)
		}
	})

	t.Run("consent copy is dropped and marketing is ineligible", func(t *testing.T) {
		var consents int
		if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_consent_projections
			WHERE business_id=$1 AND profile_id=$2`, businessA, profile.ID).Scan(&consents); err != nil {
			t.Fatal(err)
		}
		if consents != 0 {
			t.Fatalf("%d consent rows survived redaction", consents)
		}
		summary, err := env.service.ConsentSummaryFor(ctx, actor, redacted)
		if err != nil {
			t.Fatal(err)
		}
		for channel, entry := range summary.Marketing {
			if entry.Eligible {
				t.Fatalf("channel %s still marketing-eligible after redaction", channel)
			}
			if entry.Reason != ReasonPrivacyRedacted {
				t.Fatalf("channel %s reason=%s want %s", channel, entry.Reason, ReasonPrivacyRedacted)
			}
		}
	})

	t.Run("commercial evidence is retained, targeting data is not", func(t *testing.T) {
		var facts int
		if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_customer_order_facts
			WHERE business_id=$1 AND profile_id=$2`, businessA, profile.ID).Scan(&facts); err != nil {
			t.Fatal(err)
		}
		if facts != 1 {
			t.Fatalf("order fact evidence count=%d; CRM must not delete records owned by the order authority", facts)
		}
	})

	t.Run("processing history is written", func(t *testing.T) {
		var result, previous, next string
		if err := env.pool.QueryRow(ctx, `SELECT processing_result,previous_privacy_state,new_privacy_state
			FROM crm_privacy_processing_history WHERE business_id=$1 AND profile_id=$2 ORDER BY processed_at DESC LIMIT 1`,
			businessA, profile.ID).Scan(&result, &previous, &next); err != nil {
			t.Fatal(err)
		}
		if result != "APPLIED" || previous != "ACTIVE" || next != "REDACTED" {
			t.Fatalf("history result=%s %s->%s", result, previous, next)
		}
	})
}

// TestLateEventCannotResurrectRedactedRelationship is the anti-resurrection
// guarantee: an order or consent event that was in flight when the erasure
// landed must not revive the relationship.
func TestLateEventCannotResurrectRedactedRelationship(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_late"))
	current := env.reload(t, businessA, profile.ID)
	if err := env.applyPrivacy(t, businessA, "vs_a", &current, PrivacyInput{
		SourceService: "customer-service", SourceVersion: 9, PrivacySubjectRef: "cus_late",
		Action: PrivacyActionRedact, ObservedAt: env.now,
	}); err != nil {
		t.Fatal(err)
	}

	t.Run("a late order event is refused", func(t *testing.T) {
		err := env.tx(t, func(tx pgx.Tx) error {
			_, resolveErr := env.service.ResolveOrCreateProfile(ctx, tx, actor, RelationshipInput{
				Subject: customerSubject("cus_late"), ObservedAt: env.now.AddDate(0, 0, -1),
			})
			return resolveErr
		})
		if err == nil {
			t.Fatal("a late event resurrected a redacted relationship")
		}
	})

	t.Run("the relationship stays redacted", func(t *testing.T) {
		after := env.reload(t, businessA, profile.ID)
		if after.PrivacyState != PrivacyRedacted {
			t.Fatalf("privacy_state=%s after a late event", after.PrivacyState)
		}
	})

	t.Run("no duplicate profile was created", func(t *testing.T) {
		var count int
		if err := env.pool.QueryRow(ctx, `SELECT count(*) FROM vendor_customer_profiles
			WHERE business_id=$1 AND customer_ref=$2`, businessA, "cus_late").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%d profiles exist for a redacted subject; want 1", count)
		}
	})
}

// TestRedactionBeforeAnyRelationshipBlocksLaterCreation covers the harder
// ordering: the erasure arrives first, and the order event arrives afterwards.
func TestRedactionBeforeAnyRelationshipBlocksLaterCreation(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()

	// No profile exists yet.
	if err := env.applyPrivacy(t, businessA, "vs_a", nil, PrivacyInput{
		SourceService: "customer-service", SourceVersion: 3, PrivacySubjectRef: "cus_ghost",
		Action: PrivacyActionRedact, ObservedAt: env.now,
	}); err != nil {
		t.Fatal(err)
	}

	err := env.tx(t, func(tx pgx.Tx) error {
		_, resolveErr := env.service.ResolveOrCreateProfile(ctx, tx, actorFor(businessA, "vs_a"), RelationshipInput{
			Subject: customerSubject("cus_ghost"), ObservedAt: env.now,
		})
		return resolveErr
	})
	if err == nil {
		t.Fatal("a relationship was created for a subject already subject to an erasure instruction")
	}

	var count int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM vendor_customer_profiles
		WHERE business_id=$1 AND customer_ref=$2`, businessA, "cus_ghost").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("%d profiles created despite a standing erasure instruction", count)
	}
}

// TestStalePrivacyVersionCannotDowngradeRedaction is the out-of-order privacy
// race against real storage: version 8 RESTRICT arriving after version 9
// REDACT must not weaken the outcome.
func TestStalePrivacyVersionCannotDowngradeRedaction(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_race"))

	current := env.reload(t, businessA, profile.ID)
	if err := env.applyPrivacy(t, businessA, "vs_a", &current, PrivacyInput{
		SourceService: "customer-service", SourceVersion: 9, PrivacySubjectRef: "cus_race",
		Action: PrivacyActionRedact, ObservedAt: env.now,
	}); err != nil {
		t.Fatal(err)
	}

	// A late, lower-versioned RESTRICT for the same subject.
	current = env.reload(t, businessA, profile.ID)
	if err := env.applyPrivacy(t, businessA, "vs_a", &current, PrivacyInput{
		SourceService: "customer-service", SourceVersion: 8, PrivacySubjectRef: "cus_race",
		Action: PrivacyActionRestrict, ObservedAt: env.now,
	}); err != nil {
		t.Fatalf("late restrict should be absorbed, not error: %v", err)
	}

	after := env.reload(t, businessA, profile.ID)
	if after.PrivacyState != PrivacyRedacted {
		t.Fatalf("privacy_state=%s; a lower-versioned RESTRICT downgraded a REDACT", after.PrivacyState)
	}
}

// TestStaleSameActionVersionIsIgnored proves an older instruction for the same
// action is recorded as stale rather than applied.
func TestStaleSameActionVersionIsIgnored(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_stale"))

	current := env.reload(t, businessA, profile.ID)
	if err := env.applyPrivacy(t, businessA, "vs_a", &current, PrivacyInput{
		SourceService: "customer-service", SourceVersion: 5, PrivacySubjectRef: "cus_stale",
		Action: PrivacyActionRestrict, ObservedAt: env.now,
	}); err != nil {
		t.Fatal(err)
	}
	current = env.reload(t, businessA, profile.ID)
	if err := env.applyPrivacy(t, businessA, "vs_a", &current, PrivacyInput{
		SourceService: "customer-service", SourceVersion: 2, PrivacySubjectRef: "cus_stale",
		Action: PrivacyActionRestrict, ObservedAt: env.now,
	}); err != nil {
		t.Fatal(err)
	}

	var version int64
	if err := env.pool.QueryRow(context.Background(), `SELECT source_version FROM crm_privacy_projections
		WHERE business_id=$1 AND privacy_subject_ref=$2 AND action=$3`,
		businessA, "cus_stale", PrivacyActionRestrict).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 5 {
		t.Fatalf("source_version=%d; a stale instruction overwrote a newer one", version)
	}

	var stale int
	if err := env.pool.QueryRow(context.Background(), `SELECT count(*) FROM crm_privacy_processing_history
		WHERE business_id=$1 AND processing_result='STALE_IGNORED'`, businessA).Scan(&stale); err != nil {
		t.Fatal(err)
	}
	if stale == 0 {
		t.Fatal("a stale instruction was not recorded in processing history")
	}
}

// TestRedactionIsIdempotent proves replaying the same erasure changes nothing.
func TestRedactionIsIdempotent(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_replay"))
	input := PrivacyInput{
		SourceService: "customer-service", SourceVersion: 4, PrivacySubjectRef: "cus_replay",
		Action: PrivacyActionRedact, ObservedAt: env.now,
	}

	current := env.reload(t, businessA, profile.ID)
	if err := env.applyPrivacy(t, businessA, "vs_a", &current, input); err != nil {
		t.Fatal(err)
	}
	first := env.reload(t, businessA, profile.ID)

	current = env.reload(t, businessA, profile.ID)
	if err := env.applyPrivacy(t, businessA, "vs_a", &current, input); err != nil {
		t.Fatalf("replay errored: %v", err)
	}
	second := env.reload(t, businessA, profile.ID)

	if first.Version != second.Version {
		t.Fatalf("replay bumped the profile version from %d to %d", first.Version, second.Version)
	}
	if second.PrivacyState != PrivacyRedacted {
		t.Fatalf("privacy_state=%s after replay", second.PrivacyState)
	}
}

// TestRestrictIsNotRedact proves RESTRICT limits processing without erasing.
func TestRestrictIsNotRedact(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_restrict"))
	env.seedOrder(t, businessA, "vs_a", profile, "ord_res", branchA1, 5_000, 5_000, env.now.AddDate(0, 0, -2), 1)

	current := env.reload(t, businessA, profile.ID)
	if err := env.applyPrivacy(t, businessA, "vs_a", &current, PrivacyInput{
		SourceService: "customer-service", SourceVersion: 1, PrivacySubjectRef: "cus_restrict",
		Action: PrivacyActionRestrict, ObservedAt: env.now,
	}); err != nil {
		t.Fatal(err)
	}

	restricted := env.reload(t, businessA, profile.ID)
	if restricted.PrivacyState != PrivacyRestricted {
		t.Fatalf("privacy_state=%s want RESTRICTED", restricted.PrivacyState)
	}
	if restricted.RedactedAt != nil {
		t.Fatal("RESTRICT set a redaction timestamp")
	}
	// Still readable: restriction is not erasure.
	if _, err := env.service.GetProfile(ctx, actor, Scope{Global: true}, profile.ID); err != nil {
		t.Fatalf("a restricted relationship became unreadable: %v", err)
	}
	// But not targetable.
	summary, err := env.service.ConsentSummaryFor(ctx, actor, restricted)
	if err != nil {
		t.Fatal(err)
	}
	for channel, entry := range summary.Marketing {
		if entry.Eligible {
			t.Fatalf("channel %s targetable while restricted", channel)
		}
	}
}

// TestMarketingSuppressKeepsTheRelationship proves suppression removes
// targetability only.
func TestMarketingSuppressKeepsTheRelationship(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_suppress"))
	env.seedOrder(t, businessA, "vs_a", profile, "ord_sup", branchA1, 7_000, 7_000, env.now.AddDate(0, 0, -1), 1)
	if err := env.tx(t, func(tx pgx.Tx) error {
		return env.service.ApplyConsent(ctx, tx, actor, profile.ID, ConsentInput{
			SourceService: "customer-service", SourceVersion: 1, Purpose: ConsentPurposeMarketing,
			Channel: ChannelEmail, Decision: ConsentAllow, ObservedAt: env.now,
		})
	}); err != nil {
		t.Fatal(err)
	}

	current := env.reload(t, businessA, profile.ID)
	if err := env.applyPrivacy(t, businessA, "vs_a", &current, PrivacyInput{
		SourceService: "customer-service", SourceVersion: 2, PrivacySubjectRef: "cus_suppress",
		Action: PrivacyActionMarketingSuppress, ObservedAt: env.now,
	}); err != nil {
		t.Fatal(err)
	}

	after := env.reload(t, businessA, profile.ID)
	if after.PrivacyState != PrivacyActive {
		t.Fatalf("privacy_state=%s; suppression must not change the privacy state", after.PrivacyState)
	}
	if _, err := env.service.GetProfile(ctx, actor, Scope{Global: true}, profile.ID); err != nil {
		t.Fatalf("a suppressed relationship became unreadable: %v", err)
	}
	if after.CompletedOrderCount != 1 {
		t.Fatalf("commercial history lost: completed=%d", after.CompletedOrderCount)
	}

	summary, err := env.service.ConsentSummaryFor(ctx, actor, after)
	if err != nil {
		t.Fatal(err)
	}
	entry := summary.Marketing[ChannelEmail]
	if entry.Eligible || entry.Reason != ReasonMarketingSuppressed {
		t.Fatalf("email eligible=%t reason=%s despite an ALLOW plus suppression", entry.Eligible, entry.Reason)
	}
	if entry.Decision != ConsentAllow {
		t.Fatalf("the stored decision should still read ALLOW, got %s", entry.Decision)
	}
}

// TestPrivacyHandlerRegistryCoversSection8ADomains proves redaction is
// registry-driven, so later CRM packages can extend it.
func TestPrivacyHandlerRegistryCoversSection8ADomains(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	domains := map[string]bool{}
	for _, domain := range env.service.privacy.Domains() {
		domains[domain] = true
	}
	for _, required := range []string{"customer_profile", "consent", "commercial_projection", "identity_link"} {
		if !domains[required] {
			t.Fatalf("privacy registry is missing the %s domain", required)
		}
	}
}
