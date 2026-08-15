package crm

import (
	"context"
	"testing"
)

// Corporate activity is the accepted Section 8C engine with a corporate
// subject. These tests check both halves of that claim: the corporate path
// behaves correctly, and the individual path is exactly what it was before.

func TestCorporateInteraction_ReusesTheSection8CEngine(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")
	person := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_buyer"))
	contact := env.addCorporateContact(t, businessA, "vs_a", profile.ID, person.ID, ContactRoleProcurement, true)
	site := env.addCorporateLocation(t, businessA, "vs_a", profile.ID, "HQ", "Head office")
	center := env.addCorporateCostCenter(t, businessA, "vs_a", profile.ID, "OPS", "Operations")

	interaction := env.addCorporateInteraction(t, businessA, "vs_a", profile.ID, CreateCorporateInteractionInput{
		Type: InteractionAccountReview, Direction: DirectionOutbound, Outcome: OutcomeFollowUpRequired,
		Subject: "Quarterly review", Note: "Discussed renewal terms.",
		ContactRef: contact.ID, LocationRef: site.ID, CostCenterRef: center.ID, OccurredAt: env.now})

	if !interaction.IsCorporate() || *interaction.CorporateCustomerID != profile.ID {
		t.Fatalf("the interaction is not bound to the corporate account: %+v", interaction.CorporateCustomerID)
	}
	if interaction.CustomerProfileID != "" {
		t.Fatalf("a corporate interaction should carry no individual subject, got %q", interaction.CustomerProfileID)
	}
	if interaction.CorporateContactID == nil || interaction.CorporateLocationID == nil || interaction.CorporateCostCenterID == nil {
		t.Fatalf("the corporate references were not stored: %+v", interaction)
	}

	// The Section 8C immutable revision history applies unchanged: the CREATE
	// snapshot is revision 1, and a correction appends the next.
	revisions, err := env.service.ListInteractionRevisions(ctx, actor, globalScope(), interaction.ID)
	if err != nil || len(revisions) != 1 {
		t.Fatalf("expected one revision after create, got %d err=%v", len(revisions), err)
	}
	newSubject := "Quarterly review (corrected)"
	reason := "Wrong quarter recorded"
	corrected, err := env.service.CorrectInteraction(ctx, actor, globalScope(), interaction.ID,
		CorrectInteractionInput{Subject: &newSubject, CorrectionReason: reason, ExpectedVersion: interaction.Version})
	if err != nil {
		t.Fatalf("correct corporate interaction: %v", err)
	}
	if corrected.Subject != newSubject {
		t.Fatalf("the correction was not applied: %s", corrected.Subject)
	}
	revisions, err = env.service.ListInteractionRevisions(ctx, actor, globalScope(), interaction.ID)
	if err != nil || len(revisions) != 2 {
		t.Fatalf("expected two revisions after a correction, got %d err=%v", len(revisions), err)
	}
	if _, err = env.service.CorrectInteraction(ctx, actor, globalScope(), interaction.ID,
		CorrectInteractionInput{Subject: &newSubject, CorrectionReason: reason,
			ExpectedVersion: interaction.Version}); err == nil {
		t.Fatal("a stale interaction version should conflict")
	}

	// Safe-note rules are Section 8C's, unchanged.
	for name, input := range map[string]CreateCorporateInteractionInput{
		"unsupported type":  {Type: "TELEPATHY", Direction: DirectionInbound, Subject: "S", OccurredAt: env.now},
		"markup in subject": {Type: InteractionNote, Direction: DirectionInternal, Subject: "<b>S</b>", OccurredAt: env.now},
		"missing subject":   {Type: InteractionNote, Direction: DirectionInternal, OccurredAt: env.now},
		"future occurrence": {Type: InteractionNote, Direction: DirectionInternal, Subject: "S", OccurredAt: env.now.AddDate(0, 0, 2)},
	} {
		if _, err := env.service.CreateCorporateInteraction(ctx, actor, globalScope(), profile.ID, input); err == nil {
			t.Errorf("expected %s to be rejected", name)
		}
	}

	// Cross-account references are refused.
	other := env.activeCorporate(t, businessA, "vs_a", "Beta Schools")
	otherSite := env.addCorporateLocation(t, businessA, "vs_a", other.ID, "HQ", "Campus")
	if _, err = env.service.CreateCorporateInteraction(ctx, actor, globalScope(), profile.ID,
		CreateCorporateInteractionInput{Type: InteractionNote, Direction: DirectionInternal, Subject: "S",
			OccurredAt: env.now, LocationRef: otherSite.ID}); err == nil {
		t.Fatal("an interaction referenced another account's location")
	}
}

func TestCorporateTask_ReusesTheSection8CStateMachine(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	profile := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")
	person := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_buyer"))
	contact := env.addCorporateContact(t, businessA, "vs_a", profile.ID, person.ID, ContactRoleProcurement, true)
	source := env.addCorporateInteraction(t, businessA, "vs_a", profile.ID, CreateCorporateInteractionInput{})

	due := env.now.AddDate(0, 0, 7)
	task := env.addCorporateTask(t, businessA, "vs_a", profile.ID, CreateCorporateTaskInput{
		Title: "Call procurement", Description: "Confirm renewal date.", Priority: PriorityHigh,
		AssignedStaffRef: "vs_a", DueAt: &due, ContactRef: contact.ID, SourceInteractionRef: source.ID})

	if !task.IsCorporate() || task.CustomerProfileID != "" {
		t.Fatalf("the task is not a corporate one: %+v", task)
	}
	if task.Status != TaskOpen || task.Priority != PriorityHigh {
		t.Fatalf("unexpected initial task state: %+v", task)
	}

	// The whole state machine is Section 8C's, reached through the same
	// helpers individual tasks use.
	started, err := env.service.TransitionTask(ctx, actor, globalScope(), task.ID, TaskInProgress, task.Version)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	reassigned, err := env.service.ReassignTask(ctx, actor, globalScope(), task.ID,
		ReassignTaskInput{AssignedStaffRef: "", ExpectedVersion: started.Version})
	if err != nil {
		t.Fatalf("unassign: %v", err)
	}
	completed, err := env.service.TransitionTask(ctx, actor, globalScope(), task.ID, TaskCompleted, reassigned.Version)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if completed.CompletedAt == nil {
		t.Fatal("a completed task should carry a completion timestamp")
	}
	// A terminal task cannot be cancelled afterwards.
	if _, err = env.service.TransitionTask(ctx, actor, globalScope(), task.ID, TaskCancelled, completed.Version); err == nil {
		t.Fatal("a completed task was cancelled")
	}
	// Nor started from a terminal state.
	if _, err = env.service.TransitionTask(ctx, actor, globalScope(), task.ID, TaskInProgress, completed.Version); err == nil {
		t.Fatal("a completed task was restarted")
	}

	history, err := env.service.ListTaskHistory(ctx, actor, globalScope(), task.ID)
	if err != nil {
		t.Fatalf("task history: %v", err)
	}
	if len(history) < 4 {
		t.Fatalf("expected the full immutable history, got %d entries", len(history))
	}

	// The overdue derivation is Section 8C's too.
	overdue := env.now.AddDate(0, 0, -1)
	late := env.addCorporateTask(t, businessA, "vs_a", profile.ID, CreateCorporateTaskInput{
		Title: "Overdue call", DueAt: &overdue})
	if !late.IsOverdue(env.now) {
		t.Fatal("an overdue corporate task was not derived as overdue")
	}
}

func TestCorporateTask_AssigneeMustBeAbleToSeeTheAccount(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	branchAccount := env.branchCorporate(t, businessA, "vs_a", "Branch account", branchA1)
	insider := env.seedBranchStaff(t, businessA, "vs_branch_one", branchA1)
	outsider := env.seedBranchStaff(t, businessA, "vs_branch_two", branchA2)

	if _, err := env.service.CreateCorporateTask(ctx, actor, globalScope(), branchAccount.ID,
		CreateCorporateTaskInput{Title: "Visit site", AssignedStaffRef: insider}); err != nil {
		t.Fatalf("a staff member covering the account's branch should be assignable: %v", err)
	}
	if _, err := env.service.CreateCorporateTask(ctx, actor, globalScope(), branchAccount.ID,
		CreateCorporateTaskInput{Title: "Visit site", AssignedStaffRef: outsider}); err == nil {
		t.Fatal("a staff member who cannot see the account was assigned work on it")
	}
	// Another tenant's staff member is never assignable.
	if _, err := env.service.CreateCorporateTask(ctx, actor, globalScope(), branchAccount.ID,
		CreateCorporateTaskInput{Title: "Visit site", AssignedStaffRef: "vs_b"}); err == nil {
		t.Fatal("another tenant's staff member was assigned a corporate task")
	}
	// A business-scope account needs a business-wide grant.
	businessAccount := env.activeCorporate(t, businessA, "vs_a", "Business account")
	if _, err := env.service.CreateCorporateTask(ctx, actor, globalScope(), businessAccount.ID,
		CreateCorporateTaskInput{Title: "Review", AssignedStaffRef: insider}); err == nil {
		t.Fatal("a branch-only staff member was assigned a business-wide corporate task")
	}
}

// TestCorporateActivity_DoesNotDisturbIndividualCRM is the Section 8C
// regression guard. Corporate rows share the tables, so the individual
// timelines must be provably unaffected.
func TestCorporateActivity_DoesNotDisturbIndividualCRM(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")

	person := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_person"))
	individualInteraction, err := env.service.CreateInteraction(ctx, actor, globalScope(), CreateInteractionInput{
		CustomerProfileRef: person.ID, Type: InteractionPhoneCall, Direction: DirectionInbound,
		Subject: "Called about an order", Visibility: VisibilityBusiness, OccurredAt: env.now})
	if err != nil {
		t.Fatalf("create individual interaction: %v", err)
	}
	individualTask, err := env.service.CreateTask(ctx, actor, globalScope(), CreateTaskInput{
		CustomerProfileRef: person.ID, Title: "Call back", Visibility: VisibilityBusiness})
	if err != nil {
		t.Fatalf("create individual task: %v", err)
	}

	profile := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")
	corporateInteraction := env.addCorporateInteraction(t, businessA, "vs_a", profile.ID, CreateCorporateInteractionInput{})
	corporateTask := env.addCorporateTask(t, businessA, "vs_a", profile.ID, CreateCorporateTaskInput{Title: "Corporate work"})

	t.Run("individual lists exclude corporate rows", func(t *testing.T) {
		interactions, _, err := env.service.ListInteractions(ctx, actor, globalScope(), InteractionListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(interactions) != 1 || interactions[0].ID != individualInteraction.ID {
			t.Fatalf("the individual interaction list is contaminated: %d rows", len(interactions))
		}
		tasks, _, err := env.service.ListTasks(ctx, actor, globalScope(), TaskListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(tasks) != 1 || tasks[0].ID != individualTask.ID {
			t.Fatalf("the individual task list is contaminated: %d rows", len(tasks))
		}
	})

	t.Run("corporate lists exclude individual rows", func(t *testing.T) {
		interactions, _, err := env.service.ListCorporateInteractions(ctx, actor, globalScope(), profile.ID,
			InteractionListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(interactions) != 1 || interactions[0].ID != corporateInteraction.ID {
			t.Fatalf("the corporate timeline is contaminated: %d rows", len(interactions))
		}
		tasks, _, err := env.service.ListCorporateTasks(ctx, actor, globalScope(), profile.ID, TaskListOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if len(tasks) != 1 || tasks[0].ID != corporateTask.ID {
			t.Fatalf("the corporate task list is contaminated: %d rows", len(tasks))
		}
	})

	t.Run("a corporate row is not reachable through the individual getters", func(t *testing.T) {
		if _, err := env.service.GetInteraction(ctx, actor, globalScope(), corporateInteraction.ID); err == nil {
			t.Fatal("a corporate interaction was returned by the individual getter")
		}
		if _, err := env.service.GetTask(ctx, actor, globalScope(), corporateTask.ID); err == nil {
			t.Fatal("a corporate task was returned by the individual getter")
		}
		// And the reverse: an individual row is not reachable through a
		// corporate account's path.
		if _, err := env.service.GetCorporateInteraction(ctx, actor, globalScope(), profile.ID,
			individualInteraction.ID); err == nil {
			t.Fatal("an individual interaction was returned through a corporate account")
		}
		if _, err := env.service.GetCorporateTask(ctx, actor, globalScope(), profile.ID, individualTask.ID); err == nil {
			t.Fatal("an individual task was returned through a corporate account")
		}
	})

	t.Run("Customer 360 open tasks stay individual", func(t *testing.T) {
		tasks, count, err := env.service.ListOpenTasksForProfile(ctx, actor, globalScope(), person.ID, 10)
		if err != nil {
			t.Fatal(err)
		}
		if count != 1 || len(tasks) != 1 || tasks[0].ID != individualTask.ID {
			t.Fatalf("Customer 360 open tasks are contaminated: count=%d rows=%d", count, len(tasks))
		}
	})
}
