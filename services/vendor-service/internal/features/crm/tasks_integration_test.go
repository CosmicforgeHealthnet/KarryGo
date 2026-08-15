package crm

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestTasks_CreateAssignmentLinkFilteringAndHistory(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	profile := env.activityProfile(t, "cus_tasks", branchA1)
	other := env.activityProfile(t, "cus_tasks_other", branchA1)
	env.addStaff(t, "vs_branch_a", businessA, ptr(branchA1), true)
	env.addStaff(t, "vs_inactive", businessA, nil, false)
	interactionInput := basicInteraction(profile, env.now.Add(-time.Hour))
	interactionInput.Visibility = VisibilityBranch
	interactionInput.BranchRef = branchA1
	interaction, err := env.service.CreateInteraction(ctx, actor, globalScope(), interactionInput)
	if err != nil {
		t.Fatal(err)
	}
	in := CreateTaskInput{CustomerProfileRef: profile.ID, BranchRef: branchA1, Visibility: VisibilityBranch, Title: "Branch follow-up", Description: "Review the branch conversation.", Priority: PriorityHigh, AssignedStaffRef: "vs_branch_a", DueAt: ptr(env.now.Add(-time.Hour)), SourceInteractionRef: interaction.ID}
	task, err := env.service.CreateTask(ctx, actor, globalScope(), in)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != TaskOpen || task.Priority != PriorityHigh || !task.IsOverdue(env.now) {
		t.Fatalf("task=%+v", task)
	}
	items, more, err := env.service.ListTasks(ctx, actor, branchScope(branchA1), TaskListOptions{Overdue: ptr(true), Limit: 1})
	if err != nil || more || len(items) != 1 || items[0].ID != task.ID || items[0].AssigneeActive == nil || !*items[0].AssigneeActive {
		t.Fatalf("tasks=%+v more=%t err=%v", items, more, err)
	}
	if _, err = env.service.GetTask(ctx, actor, branchScope(branchA2), task.ID); codeOf(t, err) != CodeTaskNotFound {
		t.Fatalf("cross branch code=%s", codeOf(t, err))
	}
	bad := in
	bad.AssignedStaffRef = "vs_b"
	if _, err = env.service.CreateTask(ctx, actor, globalScope(), bad); codeOf(t, err) != CodeTaskAssigneeInvalid {
		t.Fatalf("cross tenant assignee code=%s err=%v", codeOf(t, err), err)
	}
	bad.AssignedStaffRef = "vs_inactive"
	if _, err = env.service.CreateTask(ctx, actor, globalScope(), bad); codeOf(t, err) != CodeTaskAssigneeInvalid {
		t.Fatalf("inactive assignee code=%s", codeOf(t, err))
	}
	otherInteraction, err := env.service.CreateInteraction(ctx, actor, globalScope(), basicInteraction(other, env.now))
	if err != nil {
		t.Fatal(err)
	}
	bad = in
	bad.SourceInteractionRef = otherInteraction.ID
	if _, err = env.service.CreateTask(ctx, actor, globalScope(), bad); codeOf(t, err) != CodeInteractionNotFound {
		t.Fatalf("cross customer link code=%s err=%v", codeOf(t, err), err)
	}
	history, err := env.service.ListTaskHistory(ctx, actor, branchScope(branchA1), task.ID)
	if err != nil || len(history) != 1 || history[0].Action != "CREATED" {
		t.Fatalf("history=%+v err=%v", history, err)
	}
}

func TestTasks_StateMachineUpdatesAndIdempotentCompletion(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	profile := env.activityProfile(t, "cus_task_flow", branchA1)
	env.addStaff(t, "vs_assignee", businessA, nil, true)
	task, err := env.service.CreateTask(ctx, actor, globalScope(), basicTask(profile))
	if err != nil {
		t.Fatal(err)
	}
	started, err := env.service.TransitionTask(ctx, actor, globalScope(), task.ID, TaskInProgress, task.Version)
	if err != nil {
		t.Fatal(err)
	}
	priority := PriorityUrgent
	title := "Urgent follow-up"
	due := env.now.Add(2 * time.Hour)
	updated, err := env.service.UpdateTask(ctx, actor, globalScope(), task.ID, UpdateTaskInput{Title: &title, Priority: &priority, DueAt: &due, ExpectedVersion: started.Version})
	if err != nil {
		t.Fatal(err)
	}
	reassigned, err := env.service.ReassignTask(ctx, actor, globalScope(), task.ID, ReassignTaskInput{AssignedStaffRef: "vs_assignee", ExpectedVersion: updated.Version})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := env.service.TransitionTask(ctx, actor, globalScope(), task.ID, TaskCompleted, reassigned.Version)
	if err != nil {
		t.Fatal(err)
	}
	if completed.CompletedAt == nil || completed.Status != TaskCompleted {
		t.Fatalf("completed=%+v", completed)
	}
	replayed, err := env.service.TransitionTask(ctx, actor, globalScope(), task.ID, TaskCompleted, reassigned.Version)
	if err != nil || replayed.Version != completed.Version {
		t.Fatalf("replay=%+v err=%v", replayed, err)
	}
	if _, err = env.service.TransitionTask(ctx, actor, globalScope(), task.ID, TaskCancelled, completed.Version); codeOf(t, err) != CodeTaskAlreadyComplete {
		t.Fatalf("terminal conflict code=%s err=%v", codeOf(t, err), err)
	}
	if _, err = env.service.UpdateTask(ctx, actor, globalScope(), task.ID, UpdateTaskInput{Title: &title, ExpectedVersion: completed.Version}); codeOf(t, err) != CodeTaskInvalidState {
		t.Fatalf("terminal update code=%s", codeOf(t, err))
	}
	history, err := env.service.ListTaskHistory(ctx, actor, globalScope(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"CREATED", "STARTED", "UPDATED", "REASSIGNED", "COMPLETED"}
	if len(history) != len(want) {
		t.Fatalf("history=%+v", history)
	}
	for i, action := range want {
		if history[i].Action != action || history[i].TaskVersion != int64(i+1) {
			t.Fatalf("history[%d]=%+v", i, history[i])
		}
	}
}

func TestTasks_PrivacyAndMergeSafety(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	profile := env.activityProfile(t, "cus_task_privacy", branchA1)
	task, err := env.service.CreateTask(ctx, actor, globalScope(), basicTask(profile))
	if err != nil {
		t.Fatal(err)
	}
	if err = env.tx(t, func(tx pgx.Tx) error {
		return env.service.ApplyPrivacy(ctx, tx, actor, &profile, PrivacyInput{SourceService: "customer-service", SourceVersion: 1, PrivacySubjectRef: "cus_task_privacy", Action: PrivacyActionRedact, ObservedAt: env.now})
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.GetTask(ctx, actor, globalScope(), task.ID); codeOf(t, err) != CodeTaskNotFound {
		t.Fatalf("redacted task code=%s", codeOf(t, err))
	}
	var status string
	var suppressed bool
	if err = env.pool.QueryRow(ctx, `SELECT status,display_suppressed FROM crm_tasks WHERE id=$1`, task.ID).Scan(&status, &suppressed); err != nil || status != TaskCancelled || !suppressed {
		t.Fatalf("retained redacted task status=%s suppressed=%t err=%v", status, suppressed, err)
	}
	if _, err = env.service.CreateTask(ctx, actor, globalScope(), basicTask(profile)); codeOf(t, err) != CodeProfileRedacted {
		t.Fatalf("redacted create code=%s", codeOf(t, err))
	}

	source := env.seedProfile(t, businessA, "vs_a", guestSubject("guest_task_merge"))
	env.seedOrder(t, businessA, "vs_a", source, "ord_guest_task_merge", branchA1, 1000, 1000, env.now.Add(-time.Hour), 1)
	target := env.activityProfile(t, "cus_task_merge_target", branchA1)
	moved, err := env.service.CreateTask(ctx, actor, globalScope(), basicTask(source))
	if err != nil {
		t.Fatal(err)
	}
	if err = env.tx(t, func(tx pgx.Tx) error {
		return env.service.ApplyIdentityLink(ctx, tx, actor, IdentityLinkInput{AuthorityService: "customer-service", AuthorityReference: "merge", AuthorityVersion: 1, EvidenceEventID: "evt_task_merge", Source: SubjectRef{Type: SubjectGuest, Ref: "guest_task_merge"}, Target: SubjectRef{Type: SubjectCustomer, Ref: "cus_task_merge_target"}})
	}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := env.service.GetTask(ctx, actor, globalScope(), moved.ID)
	if err != nil || reloaded.CustomerProfileID != target.ID {
		t.Fatalf("merged task=%+v err=%v", reloaded, err)
	}
}
