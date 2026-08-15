package crm

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func waitForActivityFenceLock(t *testing.T, env *testEnv) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var waiting bool
		if err := env.pool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE query LIKE '%crm_activity_profile_fence%' AND wait_event_type='Lock')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("8C write never reached the shared profile lock")
}

func TestActivityConcurrency_InteractionCorrectionConflict(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	profile := env.activityProfile(t, "cus_correct_race", branchA1)
	interaction, err := env.service.CreateInteraction(ctx, actor, globalScope(), basicInteraction(profile, env.now))
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, subject := range []string{"Correction A", "Correction B"} {
		wg.Add(1)
		go func(value string) {
			defer wg.Done()
			<-start
			_, e := env.service.CorrectInteraction(ctx, actor, globalScope(), interaction.ID, CorrectInteractionInput{Subject: &value, CorrectionReason: "Concurrent correction", ExpectedVersion: interaction.Version})
			errs <- e
		}(subject)
	}
	close(start)
	wg.Wait()
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("successful corrections=%d want 1", success)
	}
	revisions, err := env.service.ListInteractionRevisions(ctx, actor, globalScope(), interaction.ID)
	if err != nil || len(revisions) != 2 {
		t.Fatalf("revisions=%d err=%v", len(revisions), err)
	}
}

func TestActivityConcurrency_TaskTerminalAndAssignmentRaces(t *testing.T) {
	t.Run("complete versus cancel", func(t *testing.T) {
		env := newTestEnv(t, allFlagsOn())
		ctx := context.Background()
		actor := actorFor(businessA, "vs_a")
		profile := env.activityProfile(t, "cus_terminal_race", branchA1)
		task, err := env.service.CreateTask(ctx, actor, globalScope(), basicTask(profile))
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		for _, target := range []string{TaskCompleted, TaskCancelled} {
			go func(target string) {
				<-start
				_, e := env.service.TransitionTask(ctx, actor, globalScope(), task.ID, target, task.Version)
				results <- e
			}(target)
		}
		close(start)
		success := 0
		for i := 0; i < 2; i++ {
			if <-results == nil {
				success++
			}
		}
		if success != 1 {
			t.Fatalf("successful terminal transitions=%d", success)
		}
		settled, err := env.service.GetTask(ctx, actor, globalScope(), task.ID)
		if err != nil || settled.Status != TaskCompleted && settled.Status != TaskCancelled {
			t.Fatalf("settled=%+v err=%v", settled, err)
		}
	})
	t.Run("reassign versus complete", func(t *testing.T) {
		env := newTestEnv(t, allFlagsOn())
		ctx := context.Background()
		actor := actorFor(businessA, "vs_a")
		profile := env.activityProfile(t, "cus_assign_race", branchA1)
		env.addStaff(t, "vs_race_assignee", businessA, nil, true)
		task, err := env.service.CreateTask(ctx, actor, globalScope(), basicTask(profile))
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		go func() {
			<-start
			_, e := env.service.ReassignTask(ctx, actor, globalScope(), task.ID, ReassignTaskInput{AssignedStaffRef: "vs_race_assignee", ExpectedVersion: task.Version})
			results <- e
		}()
		go func() {
			<-start
			_, e := env.service.TransitionTask(ctx, actor, globalScope(), task.ID, TaskCompleted, task.Version)
			results <- e
		}()
		close(start)
		success := 0
		for i := 0; i < 2; i++ {
			if <-results == nil {
				success++
			}
		}
		if success != 1 {
			t.Fatalf("successful conflicting changes=%d", success)
		}
		settled, err := env.service.GetTask(ctx, actor, globalScope(), task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if settled.Status == TaskCompleted && settled.AssignedStaffID != nil {
			t.Fatal("completion and stale reassignment both appeared to win")
		}
	})
}

func TestActivityConcurrency_RedactionFencesCreatesAndCompletion(t *testing.T) {
	t.Run("interaction create", func(t *testing.T) {
		env := newTestEnv(t, allFlagsOn())
		ctx := context.Background()
		actor := actorFor(businessA, "vs_a")
		profile := env.activityProfile(t, "cus_redact_interaction_race", branchA1)
		tx, err := env.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `SELECT id FROM vendor_customer_profiles WHERE business_id=$1 AND id=$2 FOR UPDATE`, businessA, profile.ID); err != nil {
			t.Fatal(err)
		}
		result := make(chan error, 1)
		go func() {
			_, e := env.service.CreateInteraction(ctx, actor, globalScope(), basicInteraction(profile, env.now))
			result <- e
		}()
		waitForActivityFenceLock(t, env)
		if err = env.service.ApplyPrivacy(ctx, tx, actor, &profile, PrivacyInput{SourceService: "customer-service", SourceVersion: 1, PrivacySubjectRef: "cus_redact_interaction_race", Action: PrivacyActionRedact, ObservedAt: env.now}); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if e := <-result; codeOf(t, e) != CodeProfileRedacted {
			t.Fatalf("create after redaction code=%s err=%v", codeOf(t, e), e)
		}
		var count int
		if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_interactions WHERE business_id=$1 AND customer_profile_id=$2 AND display_suppressed=false`, businessA, profile.ID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("visible interactions=%d err=%v", count, err)
		}
	})
	t.Run("task create", func(t *testing.T) {
		env := newTestEnv(t, allFlagsOn())
		ctx := context.Background()
		actor := actorFor(businessA, "vs_a")
		profile := env.activityProfile(t, "cus_redact_task_race", branchA1)
		tx, err := env.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `SELECT id FROM vendor_customer_profiles WHERE business_id=$1 AND id=$2 FOR UPDATE`, businessA, profile.ID); err != nil {
			t.Fatal(err)
		}
		result := make(chan error, 1)
		go func() { _, e := env.service.CreateTask(ctx, actor, globalScope(), basicTask(profile)); result <- e }()
		waitForActivityFenceLock(t, env)
		if err = env.service.ApplyPrivacy(ctx, tx, actor, &profile, PrivacyInput{SourceService: "customer-service", SourceVersion: 1, PrivacySubjectRef: "cus_redact_task_race", Action: PrivacyActionRedact, ObservedAt: env.now}); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if e := <-result; codeOf(t, e) != CodeProfileRedacted {
			t.Fatalf("create after redaction code=%s err=%v", codeOf(t, e), e)
		}
		var count int
		if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_tasks WHERE business_id=$1 AND customer_profile_id=$2 AND status IN ('OPEN','IN_PROGRESS') AND display_suppressed=false`, businessA, profile.ID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("actionable tasks=%d err=%v", count, err)
		}
	})
	t.Run("task completion", func(t *testing.T) {
		env := newTestEnv(t, allFlagsOn())
		ctx := context.Background()
		actor := actorFor(businessA, "vs_a")
		profile := env.activityProfile(t, "cus_redact_complete_race", branchA1)
		task, err := env.service.CreateTask(ctx, actor, globalScope(), basicTask(profile))
		if err != nil {
			t.Fatal(err)
		}
		tx, err := env.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `SELECT id FROM vendor_customer_profiles WHERE business_id=$1 AND id=$2 FOR UPDATE`, businessA, profile.ID); err != nil {
			t.Fatal(err)
		}
		result := make(chan error, 1)
		go func() {
			_, e := env.service.TransitionTask(ctx, actor, globalScope(), task.ID, TaskCompleted, task.Version)
			result <- e
		}()
		waitForActivityFenceLock(t, env)
		if err = env.service.ApplyPrivacy(ctx, tx, actor, &profile, PrivacyInput{SourceService: "customer-service", SourceVersion: 1, PrivacySubjectRef: "cus_redact_complete_race", Action: PrivacyActionRedact, ObservedAt: env.now}); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if e := <-result; e == nil {
			t.Fatal("completion succeeded after redaction won the fence")
		}
		var status string
		var suppressed bool
		if err = env.pool.QueryRow(ctx, `SELECT status,display_suppressed FROM crm_tasks WHERE id=$1`, task.ID).Scan(&status, &suppressed); err != nil || status != TaskCancelled || !suppressed {
			t.Fatalf("task status=%s suppressed=%t err=%v", status, suppressed, err)
		}
	})
}

func applyMergeForRace(ctx context.Context, env *testEnv, tx pgx.Tx, actor Actor, sourceRef, targetRef, eventID string) error {
	return env.service.ApplyIdentityLink(ctx, tx, actor, IdentityLinkInput{AuthorityService: "customer-service", AuthorityReference: "merge", AuthorityVersion: 1, EvidenceEventID: eventID, Source: SubjectRef{Type: SubjectGuest, Ref: sourceRef}, Target: SubjectRef{Type: SubjectCustomer, Ref: targetRef}})
}

func TestActivityConcurrency_MergeFencesCreatesAndCompletion(t *testing.T) {
	t.Run("interaction create", func(t *testing.T) {
		env := newTestEnv(t, allFlagsOn())
		ctx := context.Background()
		actor := actorFor(businessA, "vs_a")
		sourceRef, targetRef := "guest_merge_interaction_race", "cus_merge_interaction_target"
		source := env.seedProfile(t, businessA, "vs_a", guestSubject(sourceRef))
		env.seedOrder(t, businessA, "vs_a", source, "ord_"+sourceRef, branchA1, 1000, 1000, env.now.Add(-time.Hour), 1)
		target := env.activityProfile(t, targetRef, branchA1)
		tx, err := env.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `SELECT id FROM vendor_customer_profiles WHERE business_id=$1 AND id IN ($2,$3) ORDER BY id FOR UPDATE`, businessA, source.ID, target.ID); err != nil {
			t.Fatal(err)
		}
		result := make(chan error, 1)
		go func() {
			_, e := env.service.CreateInteraction(ctx, actor, globalScope(), basicInteraction(source, env.now))
			result <- e
		}()
		waitForActivityFenceLock(t, env)
		if err = applyMergeForRace(ctx, env, tx, actor, sourceRef, targetRef, "evt_merge_interaction_race"); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if e := <-result; codeOf(t, e) != CodeProfileMerged {
			t.Fatalf("create after merge code=%s err=%v", codeOf(t, e), e)
		}
	})
	t.Run("task create", func(t *testing.T) {
		env := newTestEnv(t, allFlagsOn())
		ctx := context.Background()
		actor := actorFor(businessA, "vs_a")
		sourceRef, targetRef := "guest_merge_task_race", "cus_merge_task_target"
		source := env.seedProfile(t, businessA, "vs_a", guestSubject(sourceRef))
		env.seedOrder(t, businessA, "vs_a", source, "ord_"+sourceRef, branchA1, 1000, 1000, env.now.Add(-time.Hour), 1)
		target := env.activityProfile(t, targetRef, branchA1)
		tx, err := env.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `SELECT id FROM vendor_customer_profiles WHERE business_id=$1 AND id IN ($2,$3) ORDER BY id FOR UPDATE`, businessA, source.ID, target.ID); err != nil {
			t.Fatal(err)
		}
		result := make(chan error, 1)
		go func() { _, e := env.service.CreateTask(ctx, actor, globalScope(), basicTask(source)); result <- e }()
		waitForActivityFenceLock(t, env)
		if err = applyMergeForRace(ctx, env, tx, actor, sourceRef, targetRef, "evt_merge_task_race"); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if e := <-result; codeOf(t, e) != CodeProfileMerged {
			t.Fatalf("create after merge code=%s err=%v", codeOf(t, e), e)
		}
	})
	t.Run("task completion", func(t *testing.T) {
		env := newTestEnv(t, allFlagsOn())
		ctx := context.Background()
		actor := actorFor(businessA, "vs_a")
		sourceRef, targetRef := "guest_merge_complete_race", "cus_merge_complete_target"
		source := env.seedProfile(t, businessA, "vs_a", guestSubject(sourceRef))
		env.seedOrder(t, businessA, "vs_a", source, "ord_"+sourceRef, branchA1, 1000, 1000, env.now.Add(-time.Hour), 1)
		target := env.activityProfile(t, targetRef, branchA1)
		task, err := env.service.CreateTask(ctx, actor, globalScope(), basicTask(source))
		if err != nil {
			t.Fatal(err)
		}
		tx, err := env.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `SELECT id FROM vendor_customer_profiles WHERE business_id=$1 AND id IN ($2,$3) ORDER BY id FOR UPDATE`, businessA, source.ID, target.ID); err != nil {
			t.Fatal(err)
		}
		result := make(chan error, 1)
		go func() {
			_, e := env.service.TransitionTask(ctx, actor, globalScope(), task.ID, TaskCompleted, task.Version)
			result <- e
		}()
		waitForActivityFenceLock(t, env)
		if err = applyMergeForRace(ctx, env, tx, actor, sourceRef, targetRef, "evt_merge_complete_race"); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if e := <-result; e == nil {
			t.Fatal("source completion succeeded after merge won")
		}
		moved, err := env.service.GetTask(ctx, actor, globalScope(), task.ID)
		if err != nil || moved.CustomerProfileID != target.ID || moved.Status != TaskOpen {
			t.Fatalf("moved=%+v err=%v", moved, err)
		}
	})
}
