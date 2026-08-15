package crm

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/vendor-service/internal/events"
)

func TestInteractions_CreateScopeHistoryAndFiltering(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	profile := env.activityProfile(t, "cus_interactions", branchA1)
	business, err := env.service.CreateInteraction(ctx, actor, globalScope(), basicInteraction(profile, env.now.Add(-2*time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	branchInput := basicInteraction(profile, env.now.Add(-time.Hour))
	branchInput.Visibility = VisibilityBranch
	branchInput.BranchRef = branchA1
	branchInput.Type = InteractionInPerson
	branchInput.Subject = "Branch visit"
	branch, err := env.service.CreateInteraction(ctx, actor, globalScope(), branchInput)
	if err != nil {
		t.Fatal(err)
	}
	if business.Version != 1 || branch.BranchID == nil || *branch.BranchID != branchA1 {
		t.Fatalf("unexpected records: business=%+v branch=%+v", business, branch)
	}
	items, more, err := env.service.ListInteractions(ctx, actor, globalScope(), InteractionListOptions{CustomerProfileRef: profile.ID, Limit: 1})
	if err != nil || !more || len(items) != 1 || items[0].ID != branch.ID {
		t.Fatalf("timeline ordering/page items=%v more=%t err=%v", items, more, err)
	}
	items, _, err = env.service.ListInteractions(ctx, actor, branchScope(branchA1), InteractionListOptions{CustomerProfileRef: profile.ID, Search: "visit", Type: InteractionInPerson})
	if err != nil || len(items) != 1 || items[0].ID != branch.ID {
		t.Fatalf("branch filter items=%v err=%v", items, err)
	}
	if _, err = env.service.GetInteraction(ctx, actor, branchScope(branchA1), business.ID); codeOf(t, err) != CodeInteractionNotFound {
		t.Fatalf("branch read code=%s", codeOf(t, err))
	}
	if _, err = env.service.GetInteraction(ctx, actorFor(businessB, "vs_b"), globalScope(), branch.ID); codeOf(t, err) != CodeInteractionNotFound {
		t.Fatalf("tenant read code=%s", codeOf(t, err))
	}
	bad := basicInteraction(profile, env.now)
	bad.Visibility = VisibilityBranch
	bad.BranchRef = branchA2
	if _, err = env.service.CreateInteraction(ctx, actor, branchScope(branchA1), bad); err == nil {
		t.Fatal("Branch A created a Branch B interaction")
	}
	newSubject := "Corrected follow-up"
	newOutcome := OutcomeResolved
	corrected, err := env.service.CorrectInteraction(ctx, actor, globalScope(), business.ID, CorrectInteractionInput{Subject: &newSubject, Outcome: &newOutcome, CorrectionReason: "Correct staff entry", ExpectedVersion: business.Version})
	if err != nil {
		t.Fatal(err)
	}
	if corrected.Version != 2 || corrected.Subject != newSubject {
		t.Fatalf("corrected=%+v", corrected)
	}
	revisions, err := env.service.ListInteractionRevisions(ctx, actor, globalScope(), business.ID)
	if err != nil || len(revisions) != 2 || revisions[0].Subject == revisions[1].Subject {
		t.Fatalf("revisions=%v err=%v", revisions, err)
	}
	if _, err = env.service.CorrectInteraction(ctx, actor, globalScope(), business.ID, CorrectInteractionInput{Subject: &newSubject, CorrectionReason: "stale", ExpectedVersion: 1}); codeOf(t, err) != CodeVersionConflict {
		t.Fatalf("stale correction code=%s err=%v", codeOf(t, err), err)
	}
}

func TestInteractions_ValidationPrivacyMergeAndSafeEvents(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	profile := env.activityProfile(t, "cus_interaction_safe", branchA1)
	for name, mutate := range map[string]func(*CreateInteractionInput){"type": func(in *CreateInteractionInput) { in.Type = "BOGUS" }, "direction": func(in *CreateInteractionInput) { in.Direction = "SIDEWAYS" }, "outcome": func(in *CreateInteractionInput) { in.Outcome = "MAGIC" }, "control": func(in *CreateInteractionInput) { in.Note = "unsafe\x00note" }, "markup": func(in *CreateInteractionInput) { in.Note = "<script>bad</script>" }, "too_long": func(in *CreateInteractionInput) { in.Note = strings.Repeat("x", 5001) }} {
		t.Run(name, func(t *testing.T) {
			in := basicInteraction(profile, env.now)
			mutate(&in)
			if _, err := env.service.CreateInteraction(ctx, actor, globalScope(), in); err == nil {
				t.Fatal("invalid interaction accepted")
			}
		})
	}
	env.service.events = events.NewDomainPublisher(events.NewOutboxRepository(env.pool))
	in := basicInteraction(profile, env.now)
	in.Note = "private-token-should-not-leak"
	created, err := env.service.CreateInteraction(ctx, actor, globalScope(), in)
	if err != nil {
		t.Fatal(err)
	}
	var evidence string
	if err = env.pool.QueryRow(ctx, `SELECT coalesce(string_agg(coalesce(before_data::text,'')||coalesce(after_data::text,''),''),'') FROM vendor_audit_log WHERE aggregate_id=$1`, created.ID).Scan(&evidence); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(evidence, "private-token") {
		t.Fatalf("free text leaked to audit: %s", evidence)
	}
	if err = env.pool.QueryRow(ctx, `SELECT payload::text FROM vendor_outbox WHERE aggregate_id=$1`, created.ID).Scan(&evidence); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(evidence, "private-token") {
		t.Fatalf("free text leaked to outbox: %s", evidence)
	}
	if err = env.tx(t, func(tx pgx.Tx) error {
		return env.service.ApplyPrivacy(ctx, tx, actor, &profile, PrivacyInput{SourceService: "customer-service", SourceVersion: 1, PrivacySubjectRef: "cus_interaction_safe", Action: PrivacyActionRedact, ObservedAt: env.now})
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.GetInteraction(ctx, actor, globalScope(), created.ID); codeOf(t, err) != CodeInteractionNotFound {
		t.Fatalf("redacted interaction code=%s", codeOf(t, err))
	}
	if _, err = env.service.CreateInteraction(ctx, actor, globalScope(), basicInteraction(profile, env.now)); codeOf(t, err) != CodeProfileRedacted {
		t.Fatalf("redacted create code=%s err=%v", codeOf(t, err), err)
	}

	source := env.seedProfile(t, businessA, "vs_a", guestSubject("guest_activity"))
	env.seedOrder(t, businessA, "vs_a", source, "ord_guest_activity", branchA1, 1000, 1000, env.now.Add(-time.Hour), 1)
	target := env.activityProfile(t, "customer_activity", branchA1)
	moved, err := env.service.CreateInteraction(ctx, actor, globalScope(), basicInteraction(source, env.now))
	if err != nil {
		t.Fatal(err)
	}
	if err = env.tx(t, func(tx pgx.Tx) error {
		return env.service.ApplyIdentityLink(ctx, tx, actor, IdentityLinkInput{AuthorityService: "customer-service", AuthorityReference: "merge", AuthorityVersion: 1, EvidenceEventID: "evt_activity_merge", Source: SubjectRef{Type: SubjectGuest, Ref: "guest_activity"}, Target: SubjectRef{Type: SubjectCustomer, Ref: "customer_activity"}})
	}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := env.service.GetInteraction(ctx, actor, globalScope(), moved.ID)
	if err != nil || reloaded.CustomerProfileID != target.ID {
		t.Fatalf("merged interaction=%+v err=%v", reloaded, err)
	}
}
