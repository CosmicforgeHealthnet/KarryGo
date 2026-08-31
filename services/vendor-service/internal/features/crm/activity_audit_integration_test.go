package crm

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/vendor-service/internal/events"
)

type failingActivityPublisher struct{}

func (failingActivityPublisher) Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, string, any) error {
	return errors.New("forced outbox failure")
}

func TestActivityAuditOutboxAndTransactionalRollback(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	profile := env.activityProfile(t, "cus_activity_audit", branchA1)
	env.addStaff(t, "vs_activity_audit", businessA, nil, true)
	env.service.events = events.NewDomainPublisher(events.NewOutboxRepository(env.pool))
	in := basicInteraction(profile, env.now)
	in.Note = "interaction-secret-not-for-events"
	interaction, err := env.service.CreateInteraction(ctx, actor, globalScope(), in)
	if err != nil {
		t.Fatal(err)
	}
	subject := "Corrected"
	note := "revised-secret-not-for-events"
	interaction, err = env.service.CorrectInteraction(ctx, actor, globalScope(), interaction.ID, CorrectInteractionInput{Subject: &subject, Note: &note, CorrectionReason: "Fix", ExpectedVersion: interaction.Version})
	if err != nil {
		t.Fatal(err)
	}
	taskIn := basicTask(profile)
	taskIn.Description = "task-secret-not-for-events"
	task, err := env.service.CreateTask(ctx, actor, globalScope(), taskIn)
	if err != nil {
		t.Fatal(err)
	}
	priority := PriorityHigh
	task, err = env.service.UpdateTask(ctx, actor, globalScope(), task.ID, UpdateTaskInput{Priority: &priority, ExpectedVersion: task.Version})
	if err != nil {
		t.Fatal(err)
	}
	task, err = env.service.TransitionTask(ctx, actor, globalScope(), task.ID, TaskInProgress, task.Version)
	if err != nil {
		t.Fatal(err)
	}
	task, err = env.service.ReassignTask(ctx, actor, globalScope(), task.ID, ReassignTaskInput{AssignedStaffRef: "vs_activity_audit", ExpectedVersion: task.Version})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.TransitionTask(ctx, actor, globalScope(), task.ID, TaskCompleted, task.Version); err != nil {
		t.Fatal(err)
	}
	cancelTask, err := env.service.CreateTask(ctx, actor, globalScope(), basicTask(profile))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.TransitionTask(ctx, actor, globalScope(), cancelTask.ID, TaskCancelled, cancelTask.Version); err != nil {
		t.Fatal(err)
	}
	for _, eventType := range []string{"vendor.crm.interaction.created", "vendor.crm.interaction.corrected", "vendor.crm.task.created", "vendor.crm.task.updated", "vendor.crm.task.started", "vendor.crm.task.reassigned", "vendor.crm.task.completed", "vendor.crm.task.cancelled"} {
		var count int
		if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM vendor_outbox WHERE event_type=$1`, eventType).Scan(&count); err != nil || count < 1 {
			t.Fatalf("event %s count=%d err=%v", eventType, count, err)
		}
	}
	var blob string
	if err = env.pool.QueryRow(ctx, `SELECT coalesce(string_agg(payload::text,''),'') FROM vendor_outbox WHERE vendor_id=$1`, businessA).Scan(&blob); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(blob, "secret-not-for-events") {
		t.Fatalf("free text leaked to outbox: %s", blob)
	}
	if err = env.pool.QueryRow(ctx, `SELECT coalesce(string_agg(coalesce(before_data::text,'')||coalesce(after_data::text,''),''),'') FROM vendor_audit_log WHERE vendor_id=$1 AND action LIKE 'vendor.crm.%'`, businessA).Scan(&blob); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(blob, "secret-not-for-events") {
		t.Fatalf("free text leaked to audit: %s", blob)
	}

	env.service.events = failingActivityPublisher{}
	failed := basicInteraction(profile, env.now)
	failed.Subject = "Must roll back"
	if _, err = env.service.CreateInteraction(ctx, actor, globalScope(), failed); err == nil {
		t.Fatal("forced outbox failure did not fail command")
	}
	var count int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM crm_interactions WHERE business_id=$1 AND subject='Must roll back'`, businessA).Scan(&count); err != nil || count != 0 {
		t.Fatalf("interaction survived failed transaction count=%d err=%v", count, err)
	}
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM vendor_audit_log WHERE vendor_id=$1 AND aggregate_type='crm_interaction' AND after_data->>'profile_ref'=$2`, businessA, profile.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
}
