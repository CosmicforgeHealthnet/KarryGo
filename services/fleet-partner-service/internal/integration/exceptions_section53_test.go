package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	communicationsmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/communications/models"
	communicationsrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/communications/repositories"
	communicationsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/communications/usecases"
	engagementmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/models"
	engagementrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/repositories"
	engagementusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// failingNotifications is notification-service answering every submission
// with a definite failure.
type failingNotifications struct{}

func (failingNotifications) Configured() bool { return true }
func (failingNotifications) Submit(context.Context, string, string, json.RawMessage) (communicationsmodels.DeliveryOutcome, error) {
	return communicationsmodels.DeliveryOutcome{}, &platformclients.Error{Service: "notification-test", Outcome: platformclients.OutcomeTransient, Status: http.StatusServiceUnavailable}
}
func (failingNotifications) Read(context.Context, string, string) (communicationsmodels.DeliveryOutcome, error) {
	return communicationsmodels.DeliveryOutcome{}, errors.New("not found")
}

// TestSection53ExceptionEscalationJourney is Journey E: an operational
// exception is raised (duplicates collapse), acknowledged, then breaches its
// SLA; the durable worker escalates it once per level and the escalation
// worker queues a durable notification intent. Notification-service failing
// leaves the escalated business state intact (53.2). Resolution needs a root
// cause and resolution, and a resolved exception is final.
func TestSection53ExceptionEscalationJourney(t *testing.T) {
	pool := testPool(t)
	scope, branch, _ := section2223Scope(t, pool)
	ctx := context.Background()
	queue := jobs.NewQueue(pool)
	publisher := events.NewDomainPublisher(events.NewOutboxRepository(pool))
	comms := communicationsusecases.NewService(communicationsusecases.Options{Store: communicationsrepos.NewRepository(pool), Events: publisher, Queue: queue, Notification: failingNotifications{},
		Flags: communicationsmodels.Flags{Communications: true, NotificationRules: true, Preferences: true, Workers: true}})
	svc := engagementusecases.NewService(engagementusecases.Options{Store: engagementrepos.NewRepository(pool), Events: publisher, Queue: queue,
		Notifications: app.EscalationNotices(comms),
		Flags: engagementmodels.Flags{Support: true, Quality: true, SLA: true, Escalations: true, Exceptions: true, Workers: true}})

	in := engagementmodels.ExceptionInput{BranchID: branch, ExceptionType: "delay", Severity: "high", SourceType: "SHIPMENT", SourceReference: "shp-" + ids.New("x"), Summary: "Truck stuck at checkpoint"}
	first, err := svc.RaiseException(ctx, scope, in)
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.RaiseException(ctx, scope, in)
	if err != nil || again["id"] != first["id"] {
		t.Fatalf("duplicate raise created a second exception: %v %v", again["id"], err)
	}
	id := first["id"].(string)
	if _, err = svc.ExceptionCommand(ctx, scope, id, "RESOLVE", engagementmodels.ExceptionInput{ExpectedVersion: v64(first, "version"), RootCause: "x"}); appCode(err) != "validation_failed" {
		t.Fatalf("resolve without resolution = %v", err)
	}
	ack, err := svc.ExceptionCommand(ctx, scope, id, "ACKNOWLEDGE", engagementmodels.ExceptionInput{ExpectedVersion: v64(first, "version")})
	if err != nil || ack["status"] != "ACKNOWLEDGED" || ack["owner_user_id"] != scope.ActorID {
		t.Fatalf("acknowledge = %v %v", ack, err)
	}
	if _, err = svc.ExceptionCommand(ctx, scope, id, "ESCALATE", engagementmodels.ExceptionInput{ExpectedVersion: v64(first, "version")}); appCode(err) != "version_conflict" {
		t.Fatalf("stale escalate = %v", err)
	}

	// The SLA (HIGH = 1h) passes: the worker escalates once.
	if _, err = pool.Exec(ctx, `UPDATE fleet_partner_operational_exceptions SET sla_due_at=now()-interval '1 minute' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	worker := &jobs.Worker{Queue: queue, Metrics: observability.NewMetrics(), Owner: "s53-worker", PollInterval: 20 * time.Millisecond, LeaseDuration: 5 * time.Second, HeartbeatInterval: time.Second}
	svc.RegisterWorkers(worker)
	comms.RegisterWorkers(worker)
	run := func(jobType, key string) {
		t.Helper()
		jobID, err := queue.Enqueue(ctx, pool, jobs.EnqueueRequest{OrganizationID: scope.OrganizationID, JobType: jobType, DedupeKey: key, Payload: map[string]string{"organizationId": scope.OrganizationID}, MaxAttempts: 3})
		if err != nil {
			t.Fatal(err)
		}
		runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		done := make(chan struct{})
		go func() { worker.Run(runCtx); close(done) }()
		for {
			var status string
			_ = pool.QueryRow(ctx, `SELECT status FROM fleet_partner_jobs WHERE id=$1`, jobID).Scan(&status)
			if status == "SUCCEEDED" {
				break
			}
			if runCtx.Err() != nil {
				t.Fatalf("job %s (%s) ended %s", jobType, jobID, status)
			}
			time.Sleep(25 * time.Millisecond)
		}
		cancel()
		<-done
	}
	run("SECTION23_EXCEPTION_SLA", "s53-sla-1")
	got, err := svc.GetException(ctx, scope, id)
	if err != nil || got["status"] != "ESCALATED" || v64(got, "escalation_level") != 1 {
		t.Fatalf("after SLA breach = %v %v", got, err)
	}
	// Re-running before the new deadline escalates nothing more.
	run("SECTION23_EXCEPTION_SLA", "s53-sla-2")
	var escalations int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_escalations WHERE organization_id=$1 AND source_type='OPERATIONAL_EXCEPTION' AND source_reference=$2`, scope.OrganizationID, id).Scan(&escalations)
	if escalations != 1 {
		t.Fatalf("escalations = %d, want 1 (no duplicate escalation)", escalations)
	}
	// The due escalation is turned into a durable notification intent.
	run("SECTION23_ESCALATION_SCHEDULE", "s53-esc-1")
	var intents int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_notification_intents WHERE organization_id=$1 AND source_type='ESCALATION'`, scope.OrganizationID).Scan(&intents)
	if intents != 1 {
		t.Fatalf("escalation notification intents = %d, want 1", intents)
	}
	// Give the failing delivery attempt a moment, then prove the business
	// state survived it.
	runCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	worker.Run(runCtx)
	cancel()
	if got, _ = svc.GetException(ctx, scope, id); got["status"] != "ESCALATED" {
		t.Fatalf("notification failure changed the exception: %v", got["status"])
	}

	// Manual escalation to the next level, then resolution.
	got, err = svc.ExceptionCommand(ctx, scope, id, "ESCALATE", engagementmodels.ExceptionInput{ExpectedVersion: v64(got, "version"), Note: "customer chasing"})
	if err != nil || v64(got, "escalation_level") != 2 {
		t.Fatalf("manual escalate = %v %v", got, err)
	}
	got, err = svc.ExceptionCommand(ctx, scope, id, "RESOLVE", engagementmodels.ExceptionInput{ExpectedVersion: v64(got, "version"), RootCause: "Missing permit", Resolution: "Permit issued; truck released"})
	if err != nil || got["status"] != "RESOLVED" {
		t.Fatalf("resolve = %v %v", got, err)
	}
	if _, err = svc.ExceptionCommand(ctx, scope, id, "ESCALATE", engagementmodels.ExceptionInput{ExpectedVersion: v64(got, "version")}); appCode(err) != "state_transition_invalid" {
		t.Fatalf("resolved exception changed: %v", err)
	}
	if history, _ := got["history"].([]map[string]any); len(history) < 5 {
		t.Fatalf("history = %d entries, want >= 5", len(history))
	}
	if _, err = pool.Exec(ctx, `DELETE FROM fleet_partner_operational_exception_history WHERE exception_id=$1`, id); err == nil {
		t.Fatal("exception history was deletable")
	}
	// A resolved exception frees the source: a new one may be raised.
	if again, err = svc.RaiseException(ctx, scope, in); err != nil || again["id"] == id {
		t.Fatalf("re-raise after resolve = %v %v", again["id"], err)
	}

	// Notification preferences: the caller's own preference narrows a user
	// recipient's channels; an organization user cannot see another's.
	if _, err = comms.SavePreference(ctx, scope, communicationsmodels.PreferenceInput{EventType: "fleet.escalation.due", Channels: []string{"push"}}, false); err != nil {
		t.Fatal(err)
	}
	if _, err = comms.SavePreference(ctx, scope, communicationsmodels.PreferenceInput{EventType: "fleet.escalation.due", Channels: []string{"email"}}, false); appCode(err) != "version_conflict" {
		t.Fatalf("stale preference update = %v", err)
	}
	other := scope
	other.OrganizationUserID, other.ActorID = ids.New("usr"), ids.New("usr")
	if prefs, _ := comms.ListPreferences(ctx, other); len(prefs) != 0 {
		t.Fatalf("another user's preferences visible: %v", prefs)
	}
	if err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		intentID, err := comms.QueueIntentTx(ctx, tx, communicationsmodels.IntentInput{OrganizationID: scope.OrganizationID, SourceType: "TEST", SourceReference: "pref", RecipientType: "ORGANIZATION_USER", RecipientReference: scope.OrganizationUserID, Channels: json.RawMessage(`["EMAIL","PUSH"]`), TemplateCode: "fleet.escalation.due", SafePayload: json.RawMessage(`{}`), IdempotencyKey: "pref-" + ids.New("k")})
		if err != nil {
			return err
		}
		var channels string
		if err = tx.QueryRow(ctx, `SELECT channels::text FROM fleet_partner_notification_intents WHERE id=$1`, intentID).Scan(&channels); err != nil {
			return err
		}
		if channels != `["PUSH"]` {
			t.Errorf("channels after preference = %s, want [\"PUSH\"]", channels)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
