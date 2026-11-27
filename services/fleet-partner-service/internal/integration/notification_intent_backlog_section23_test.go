package integration_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	communicationsmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/communications/models"
	communicationsrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/communications/repositories"
	communicationsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/communications/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// backlogNotifications refuses every submission (as notification-service
// does for a template it does not hold) and answers read-backs from held.
type backlogNotifications struct {
	mu       sync.Mutex
	submits  int
	reads    int
	held     map[string]bool
	readFail map[string]bool
}

func (n *backlogNotifications) Configured() bool { return true }
func (n *backlogNotifications) Submit(context.Context, string, string, json.RawMessage) (communicationsmodels.DeliveryOutcome, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.submits++
	return communicationsmodels.DeliveryOutcome{}, &platformclients.Error{Service: "notification-service", Outcome: platformclients.OutcomeNotFound, Status: 404, Message: "template not found"}
}
func (n *backlogNotifications) Read(_ context.Context, _, key string) (communicationsmodels.DeliveryOutcome, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.reads++
	if n.readFail[key] {
		return communicationsmodels.DeliveryOutcome{}, &platformclients.Error{Service: "notification-service", Outcome: platformclients.OutcomeTransient, Status: 503}
	}
	if n.held[key] {
		return communicationsmodels.DeliveryOutcome{Reference: "ntf-" + key, Status: "sent", SourceVersion: 1}, nil
	}
	return communicationsmodels.DeliveryOutcome{}, &platformclients.Error{Service: "notification-service", Outcome: platformclients.OutcomeNotFound, Status: 404}
}

// TestNotificationIntentBacklogIsClosedNotReplayed proves the delivery
// worker cannot flood or loop on a historical backlog: an intent older than
// the maximum age is never sent late; one that may already have been sent is
// read back and recorded by what notification-service says; retries stop at
// the attempt cap; and a closed intent is never picked up again. Nothing is
// deleted: every decision is an append-only attempt row.
func TestNotificationIntentBacklogIsClosedNotReplayed(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	scope, _, _ := section2223Scope(t, pool)
	org := scope.OrganizationID
	notify := &backlogNotifications{held: map[string]bool{}, readFail: map[string]bool{}}
	queue := jobs.NewQueue(pool)
	comms := communicationsusecases.NewService(communicationsusecases.Options{Store: communicationsrepos.NewRepository(pool),
		Events: events.NewDomainPublisher(events.NewOutboxRepository(pool)), Queue: queue, Notification: notify,
		Flags: communicationsmodels.Flags{Communications: true, Workers: true}, IntentMaxAttempts: 3, IntentMaxAge: 24 * time.Hour})
	worker := &jobs.Worker{Queue: queue, Metrics: observability.NewMetrics(), Owner: "intent-backlog-worker", PollInterval: 20 * time.Millisecond, LeaseDuration: 5 * time.Second, HeartbeatInterval: time.Second}
	comms.RegisterWorkers(worker)
	h := &s24{t: t, pool: pool, queue: queue, worker: worker, org: org}

	intent := func(status string, attempts int, age time.Duration) (id, key string) {
		id, key = ids.New("nti"), ids.New("key")
		if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_notification_intents(id,organization_id,source_type,source_reference,recipient_type,recipient_reference,channels,template_code,idempotency_key,status,attempts,created_at)
VALUES($1,$2,'ESCALATION',$3,'USER',$4,'["IN_APP"]','fleet.escalation.due',$5,$6,$7,now()-$8::interval)`, id, org, ids.New("esc"), ids.New("usr"), key, status, attempts, age.String()); err != nil {
			t.Fatal(err)
		}
		return id, key
	}
	state := func(id string) (status, code string, attempts, rows int) {
		if err := pool.QueryRow(ctx, `SELECT status,last_error_code,attempts,(SELECT count(*) FROM fleet_partner_notification_delivery_attempts a WHERE a.intent_id=i.id) FROM fleet_partner_notification_intents i WHERE id=$1`, id).Scan(&status, &code, &attempts, &rows); err != nil {
			t.Fatal(err)
		}
		return
	}
	submit := func(id string) { h.runJob("SECTION23_NOTIFICATION_SUBMIT", map[string]string{"intentId": id}) }

	// Never sent and three days old: closed as expired, no network call.
	stale, _ := intent("PENDING", 0, 72*time.Hour)
	submit(stale)
	if s, c, a, r := state(stale); s != "DEAD_LETTERED" || c != "expired" || a != 1 || r != 1 || notify.submits != 0 || notify.reads != 0 {
		t.Fatalf("stale pending = %s/%s attempts %d rows %d, submits %d reads %d", s, c, a, r, notify.submits, notify.reads)
	}

	// Old, attempted before, and notification-service holds it: recorded as
	// accepted from the read-back, never resubmitted.
	held, heldKey := intent("FAILED", 2, 48*time.Hour)
	notify.held[heldKey] = true
	submit(held)
	if s, _, _, _ := state(held); s != "ACCEPTED" || notify.submits != 0 {
		t.Fatalf("stale but delivered = %s, submits %d; want ACCEPTED without a submission", s, notify.submits)
	}

	// Old with an unknown outcome that notification-service never received.
	lost, _ := intent("OUTCOME_UNKNOWN", 1, 48*time.Hour)
	submit(lost)
	if s, c, _, _ := state(lost); s != "DEAD_LETTERED" || c != "expired" || notify.submits != 0 {
		t.Fatalf("stale unknown = %s/%s submits %d", s, c, notify.submits)
	}

	// Old, outcome unknown, and the read-back keeps failing: it stays
	// unknown until the attempt cap closes it as unverified.
	unverified, unverifiedKey := intent("OUTCOME_UNKNOWN", 1, 48*time.Hour)
	notify.readFail[unverifiedKey] = true
	submit(unverified)
	if s, c, _, _ := state(unverified); s != "OUTCOME_UNKNOWN" || c != "expired_readback_failed" {
		t.Fatalf("first unverified read = %s/%s", s, c)
	}
	submit(unverified)
	if s, c, a, _ := state(unverified); s != "DEAD_LETTERED" || c != "expired_unverified" || a != 3 || notify.submits != 0 {
		t.Fatalf("unverified at cap = %s/%s attempts %d submits %d", s, c, a, notify.submits)
	}

	// A recent intent already refused twice: the third refusal reaches the
	// cap and dead-letters it; it is then untouched by further submissions
	// and by the reconcile sweep.
	fresh, _ := intent("FAILED", 2, time.Minute)
	submit(fresh)
	if s, c, a, _ := state(fresh); s != "DEAD_LETTERED" || c != "attempts_exhausted" || a != 3 || notify.submits != 1 {
		t.Fatalf("refused at the cap = %s/%s attempts %d submits %d; want DEAD_LETTERED after the third", s, c, a, notify.submits)
	}
	submit(fresh)
	if _, err := pool.Exec(ctx, `UPDATE fleet_partner_notification_intents SET next_attempt_at=now()-interval '1 minute' WHERE organization_id=$1`, org); err != nil {
		t.Fatal(err)
	}
	h.runJob("SECTION23_NOTIFICATION_RECONCILE", map[string]string{})
	if _, _, a, _ := state(fresh); a != 3 || notify.submits != 1 {
		t.Fatalf("closed intent was retried: attempts %d submits %d", a, notify.submits)
	}
	var open int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_notification_intents WHERE organization_id=$1 AND status IN('PENDING','FAILED','OUTCOME_UNKNOWN')`, org).Scan(&open); err != nil || open != 0 {
		t.Fatalf("open intents after the sweep = %d, %v; want 0", open, err)
	}
}
