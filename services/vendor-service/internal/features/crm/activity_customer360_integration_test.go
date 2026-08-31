package crm

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestCustomer360_ActivityBusinessAndBranchScope(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	profile := env.activityProfile(t, "cus_360_activity", branchA1)
	env.seedOrder(t, businessA, "vs_a", profile, "ord_360_activity_b2", branchA2, 1000, 1000, env.now.Add(-2*time.Hour), 1)
	profile = env.reload(t, businessA, profile.ID)
	createInteraction := func(visibility, branch, subject string) {
		in := basicInteraction(profile, env.now)
		in.Visibility = visibility
		in.BranchRef = branch
		in.Subject = subject
		if _, err := env.service.CreateInteraction(ctx, actor, globalScope(), in); err != nil {
			t.Fatal(err)
		}
	}
	createTask := func(visibility, branch, title string) {
		in := basicTask(profile)
		in.Visibility = visibility
		in.BranchRef = branch
		in.Title = title
		if _, err := env.service.CreateTask(ctx, actor, globalScope(), in); err != nil {
			t.Fatal(err)
		}
	}
	createInteraction(VisibilityBusiness, "", "Business interaction")
	createInteraction(VisibilityBranch, branchA1, "Branch A interaction")
	createInteraction(VisibilityBranch, branchA2, "Branch B interaction")
	createTask(VisibilityBusiness, "", "Business task")
	createTask(VisibilityBranch, branchA1, "Branch A task")
	createTask(VisibilityBranch, branchA2, "Branch B task")
	businessView, err := env.service.Customer360For(ctx, actor, globalScope(), profile)
	if err != nil {
		t.Fatal(err)
	}
	if businessView.RecentInteractions == nil || len(businessView.RecentInteractions.Items) != 3 || businessView.OpenTasks == nil || businessView.OpenTasks.Count != 3 {
		t.Fatalf("business activity interactions=%+v tasks=%+v", businessView.RecentInteractions, businessView.OpenTasks)
	}
	branchView, err := env.service.Customer360For(ctx, actor, branchScope(branchA1), profile)
	if err != nil {
		t.Fatal(err)
	}
	if branchView.RecentInteractions == nil || len(branchView.RecentInteractions.Items) != 1 || branchView.RecentInteractions.Items[0].Subject != "Branch A interaction" {
		t.Fatalf("branch interactions=%+v", branchView.RecentInteractions)
	}
	if branchView.OpenTasks == nil || branchView.OpenTasks.Count != 1 || len(branchView.OpenTasks.Items) != 1 || branchView.OpenTasks.Items[0].Title != "Branch A task" {
		t.Fatalf("branch tasks=%+v", branchView.OpenTasks)
	}
}

func TestCustomer360_ActivityRedactedAndMergedProfiles(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	redacted := env.activityProfile(t, "cus_360_activity_redacted", branchA1)
	if _, err := env.service.CreateInteraction(ctx, actor, globalScope(), basicInteraction(redacted, env.now)); err != nil {
		t.Fatal(err)
	}
	if _, err := env.service.CreateTask(ctx, actor, globalScope(), basicTask(redacted)); err != nil {
		t.Fatal(err)
	}
	if err := env.tx(t, func(tx pgx.Tx) error {
		return env.service.ApplyPrivacy(ctx, tx, actor, &redacted, PrivacyInput{SourceService: "customer-service", SourceVersion: 1, PrivacySubjectRef: "cus_360_activity_redacted", Action: PrivacyActionRedact, ObservedAt: env.now})
	}); err != nil {
		t.Fatal(err)
	}
	redacted = env.reload(t, businessA, redacted.ID)
	view, err := env.service.Customer360For(ctx, actor, globalScope(), redacted)
	if err != nil {
		t.Fatal(err)
	}
	if view.RecentInteractions == nil || len(view.RecentInteractions.Items) != 0 || view.OpenTasks == nil || view.OpenTasks.Count != 0 {
		t.Fatalf("redacted activity leaked: interactions=%+v tasks=%+v", view.RecentInteractions, view.OpenTasks)
	}
	sourceRef, targetRef := "guest_360_activity_merge", "cus_360_activity_target"
	source := env.seedProfile(t, businessA, "vs_a", guestSubject(sourceRef))
	env.seedOrder(t, businessA, "vs_a", source, "ord_"+sourceRef, branchA1, 1000, 1000, env.now.Add(-time.Hour), 1)
	target := env.activityProfile(t, targetRef, branchA1)
	if _, err = env.service.CreateInteraction(ctx, actor, globalScope(), basicInteraction(source, env.now)); err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.CreateTask(ctx, actor, globalScope(), basicTask(source)); err != nil {
		t.Fatal(err)
	}
	if err = env.tx(t, func(tx pgx.Tx) error {
		return env.service.ApplyIdentityLink(ctx, tx, actor, IdentityLinkInput{AuthorityService: "customer-service", AuthorityReference: "merge", AuthorityVersion: 1, EvidenceEventID: "evt_360_activity_merge", Source: SubjectRef{Type: SubjectGuest, Ref: sourceRef}, Target: SubjectRef{Type: SubjectCustomer, Ref: targetRef}})
	}); err != nil {
		t.Fatal(err)
	}
	source = env.reload(t, businessA, source.ID)
	sourceView, err := env.service.Customer360For(ctx, actor, globalScope(), source)
	if err != nil {
		t.Fatal(err)
	}
	if len(sourceView.RecentInteractions.Items) != 0 || sourceView.OpenTasks.Count != 0 {
		t.Fatal("merged source remained independently actionable")
	}
	target = env.reload(t, businessA, target.ID)
	targetView, err := env.service.Customer360For(ctx, actor, globalScope(), target)
	if err != nil || len(targetView.RecentInteractions.Items) != 1 || targetView.OpenTasks.Count != 1 {
		t.Fatalf("target activity interactions=%+v tasks=%+v err=%v", targetView.RecentInteractions, targetView.OpenTasks, err)
	}
}
