package campaignusecases_test

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/notification-service/internal/features/campaigns/campaigntest"
	campaignmodels "cosmicforge/logistics/services/notification-service/internal/features/campaigns/models"
	campaignrepositories "cosmicforge/logistics/services/notification-service/internal/features/campaigns/repositories"
	campaignusecases "cosmicforge/logistics/services/notification-service/internal/features/campaigns/usecases"
	messageclients "cosmicforge/logistics/services/notification-service/internal/features/messages/clients"
	messagerepositories "cosmicforge/logistics/services/notification-service/internal/features/messages/repositories"
	messageusecases "cosmicforge/logistics/services/notification-service/internal/features/messages/usecases"
	"cosmicforge/logistics/shared/go/admincampaign"
	"cosmicforge/logistics/shared/go/adminevents"
)

// recordingQueue stands in for the Redis stream: the test drives delivery
// processing explicitly so every outcome is deterministic.
type recordingQueue struct {
	mu  sync.Mutex
	ids []string
}

func (q *recordingQueue) EnqueueDelivery(_ context.Context, id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ids = append(q.ids, id)
	return nil
}
func (q *recordingQueue) DeadLetterDelivery(context.Context, string, string) error { return nil }
func (q *recordingQueue) StartDeliveryConsumer(context.Context, func(context.Context, string) error) {
}
func (q *recordingQueue) StartRequestConsumer(context.Context, func(context.Context, []byte) error) {
}
func (q *recordingQueue) drain() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := q.ids
	q.ids = nil
	return out
}

// pushProvider is a scripted push provider.
type pushProvider struct {
	mu      sync.Mutex
	results []func() (messageclients.ProviderResult, error)
	calls   int
}

func (p *pushProvider) SendPush(context.Context, messageclients.PushMessage) (messageclients.ProviderResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	if len(p.results) == 0 {
		id := "fcm-" + uuid.NewString()
		return messageclients.ProviderResult{Provider: "firebase", ProviderMessageID: &id}, nil
	}
	next := p.results[0]
	if len(p.results) > 1 {
		p.results = p.results[1:]
	}
	return next()
}

type emailProvider struct{}

func (emailProvider) SendEmail(context.Context, messageclients.EmailMessage) (messageclients.ProviderResult, error) {
	id := "smtp-accepted"
	return messageclients.ProviderResult{Provider: "smtp", ProviderMessageID: &id}, nil
}

type harness struct {
	t        *testing.T
	db       *pgxpool.Pool
	repo     *campaignrepositories.Repository
	service  *campaignusecases.Service
	engine   *campaignusecases.Engine
	consent  *campaigntest.Consent
	queue    *recordingQueue
	pipeline *messageusecases.NotificationService
	push     *pushProvider
}

func newHarness(t *testing.T, push messageclients.PushSender) *harness {
	t.Helper()
	db := campaigntest.Pool(t)
	repo := campaignrepositories.New(db)
	queue := &recordingQueue{}
	provider := &pushProvider{}
	if push == nil {
		push = provider
	}
	pipeline := messageusecases.NewNotificationService(messageusecases.Options{
		Repository: messagerepositories.NewPostgresNotificationRepository(db), PushSender: push, EmailSender: emailProvider{},
		RealtimeSender: messageclients.NewWebSocketHub(), Queue: queue, RealtimeTokenSecret: []byte("realtime"), MaxAttempts: 3,
	})
	consent := campaigntest.NewConsent()
	return &harness{t: t, db: db, repo: repo, service: &campaignusecases.Service{Repo: repo}, consent: consent, queue: queue, pipeline: pipeline, push: provider,
		engine: &campaignusecases.Engine{Repo: repo, Consent: consent, Sender: pipeline, Owner: "test-engine", Retry: time.Millisecond}}
}

func (h *harness) device(customer string) {
	h.t.Helper()
	if _, err := h.db.Exec(context.Background(), `INSERT INTO notification_devices(id,recipient_type,recipient_id,token,platform,app) VALUES($1,'customer',$2,$3,'android','customer')`, uuid.NewString(), customer, "tok-"+uuid.NewString()); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) accept(i admincampaign.Intent) admincampaign.Status {
	h.t.Helper()
	status, created, err := h.service.Accept(context.Background(), campaigntest.Raw(h.t, i))
	if err != nil || !created || status.State != campaignmodels.StateAccepted {
		h.t.Fatalf("accept: %+v created=%t err=%v", status, created, err)
	}
	return status
}

// run ticks the engine and processes every queued delivery until the
// revision settles or the step budget runs out.
func (h *harness) run(i admincampaign.Intent, steps int) admincampaign.Status {
	h.t.Helper()
	ctx := context.Background()
	for n := 0; n < steps; n++ {
		if err := h.engine.Tick(ctx); err != nil {
			h.t.Fatal(err)
		}
		for _, id := range h.queue.drain() {
			if err := h.pipeline.ProcessDelivery(ctx, id); err != nil {
				h.t.Fatal(err)
			}
		}
		time.Sleep(3 * time.Millisecond)
		status := h.status(i)
		if campaignmodels.Terminal(status.State) {
			return status
		}
	}
	return h.status(i)
}

func (h *harness) status(i admincampaign.Intent) admincampaign.Status {
	h.t.Helper()
	s, err := h.service.Status(context.Background(), i.CampaignID, i.Revision)
	if err != nil {
		h.t.Fatal(err)
	}
	return s
}

func (h *harness) recipients(i admincampaign.Intent) map[string]string {
	h.t.Helper()
	rows, err := h.db.Query(context.Background(), `SELECT recipient_id||'/'||channel, state||COALESCE(':'||suppression_reason,'') FROM campaign_recipients WHERE campaign_id=$1 AND revision=$2`, i.CampaignID, i.Revision)
	if err != nil {
		h.t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			h.t.Fatal(err)
		}
		out[k] = v
	}
	return out
}

func (h *harness) messages(i admincampaign.Intent) int {
	var n int
	if err := h.db.QueryRow(context.Background(), `SELECT count(*) FROM notification_messages WHERE idempotency_key LIKE $1`, "campaign:"+i.CampaignID+":%").Scan(&n); err != nil {
		h.t.Fatal(err)
	}
	return n
}

func (h *harness) template() (string, int64, string) {
	return campaigntest.Template(h.t, h.db, "Weekend offer", "Save on your next ride this weekend.")
}

func assertTruthful(t *testing.T, s admincampaign.Status) {
	t.Helper()
	if s.DeliveredCount != nil || s.DeliveryConfirmation != "UNAVAILABLE" || s.ConversionReporting != "UNAVAILABLE" || s.State == "DELIVERED" {
		t.Fatalf("status claims delivery facts without receipts: %+v", s)
	}
}

func TestTargetedMarketOnlyUnknownMarketExcludedAndMessageStatusUpdated(t *testing.T) {
	h := newHarness(t, nil)
	a, b := h.consent.Add("NG-LAG", "push"), h.consent.Add("NG-LAG", "push")
	other := h.consent.Add("NG-ABJ", "push")
	unknown := h.consent.Add("", "push")
	for _, id := range []string{a, b, other, unknown} {
		h.device(id)
	}
	key, rev, hash := h.template()
	i := campaigntest.Intent(key, rev, hash, []string{"NG-LAG"}, []string{"push"})
	h.accept(i)
	s := h.run(i, 20)
	if s.State != campaignmodels.StateCompleted || s.AudienceCount != 2 || s.SentCount != 2 || s.EligibleCount != 2 || s.QueuedCount != 0 {
		t.Fatalf("status: %+v", s)
	}
	assertTruthful(t, s)
	got := h.recipients(i)
	if len(got) != 2 || got[a+"/push"] != "SENT" || got[b+"/push"] != "SENT" {
		t.Fatalf("recipients: %v", got)
	}
	// The pipeline message status now reflects its final delivery.
	var statuses []string
	rows, _ := h.db.Query(context.Background(), `SELECT status FROM notification_messages WHERE idempotency_key LIKE $1`, "campaign:"+i.CampaignID+":%")
	for rows.Next() {
		var st string
		_ = rows.Scan(&st)
		statuses = append(statuses, st)
	}
	rows.Close()
	if len(statuses) != 2 || statuses[0] != "sent" || statuses[1] != "sent" {
		t.Fatalf("notification_messages.status not updated: %v", statuses)
	}
	var source, event string
	_ = h.db.QueryRow(context.Background(), `SELECT source_service,event_type FROM notification_messages WHERE idempotency_key=$1`, campaignmodels.MessageIdempotencyKey(i.CampaignID, 1, a, "push")).Scan(&source, &event)
	if source != "notification-service" || event != "campaign.marketing" {
		t.Fatalf("message provenance: %s %s", source, event)
	}
}

func TestSameIntentTwiceIsOneCampaignAndRevisionConflicts(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	c := h.consent.Add("NG-LAG", "push")
	h.device(c)
	key, rev, hash := h.template()
	i := campaigntest.Intent(key, rev, hash, []string{"NG-LAG"}, []string{"push"})
	first := h.accept(i)
	again, created, err := h.service.Accept(ctx, campaigntest.Raw(t, i))
	if err != nil || created || again.IntentDigest != first.IntentDigest {
		t.Fatalf("replay: %+v %t %v", again, created, err)
	}
	h.run(i, 20)
	h.run(i, 5) // a second engine pass must not resend
	if n := h.messages(i); n != 1 {
		t.Fatalf("messages sent: %d", n)
	}
	var rows int
	_ = h.db.QueryRow(ctx, `SELECT count(*) FROM campaigns WHERE campaign_id=$1`, i.CampaignID).Scan(&rows)
	if rows != 1 {
		t.Fatalf("campaign rows: %d", rows)
	}
	// Same idempotency key, different content: conflict.
	changed := i
	changed.MaxMessages = 5
	if _, _, err := h.service.Accept(ctx, campaigntest.Raw(t, changed)); !errors.Is(err, campaignusecases.ErrConflict) {
		t.Fatalf("key reuse with new digest: %v", err)
	}
	// Same campaign and revision under a new key with different content.
	changed.IdempotencyKey = "idem-" + uuid.NewString()
	if _, _, err := h.service.Accept(ctx, campaigntest.Raw(t, changed)); !errors.Is(err, campaignusecases.ErrConflict) {
		t.Fatalf("revision reuse with new digest: %v", err)
	}
	// Same campaign and revision under a new key with the same content.
	same := i
	same.IdempotencyKey = "idem-" + uuid.NewString()
	if s, created, err := h.service.Accept(ctx, campaigntest.Raw(t, same)); err != nil || created || s.Revision != 1 {
		t.Fatalf("same revision new key: %+v %t %v", s, created, err)
	}
}

func TestHigherRevisionCancelsUnsentWorkOfOlderAndLowerRevisionRefused(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	for k := 0; k < 3; k++ {
		h.device(h.consent.Add("NG-LAG", "push"))
	}
	key, rev, hash := h.template()
	r1 := campaigntest.Intent(key, rev, hash, []string{"NG-LAG"}, []string{"push"})
	r1.StartAt = time.Now().UTC().Add(time.Hour) // planned, not yet sendable
	h.accept(r1)
	h.run(r1, 3)
	if s := h.status(r1); s.State != campaignmodels.StatePlanned || s.AudienceCount != 3 {
		t.Fatalf("r1 planned: %+v", s)
	}
	r2 := r1
	r2.Revision, r2.IdempotencyKey, r2.StartAt = 2, "idem-"+uuid.NewString(), time.Now().UTC().Add(-time.Minute)
	h.accept(r2)
	s1 := h.status(r1)
	if s1.State != campaignmodels.StateCancelled {
		t.Fatalf("r1 not cancelled: %+v", s1)
	}
	for k, v := range h.recipients(r1) {
		if v != "CANCELLED" {
			t.Fatalf("r1 recipient %s: %s", k, v)
		}
	}
	if s := h.run(r2, 20); s.SentCount != 3 || s.State != campaignmodels.StateCompleted {
		t.Fatalf("r2: %+v", s)
	}
	if h.messages(r1) != 3 { // all three are revision 2's messages (prefix covers both)
		t.Fatalf("unexpected message count %d", h.messages(r1))
	}
	stale := r1
	stale.IdempotencyKey, stale.MaxMessages = "idem-"+uuid.NewString(), 7
	if _, _, err := h.service.Accept(ctx, campaigntest.Raw(t, stale)); !errors.Is(err, campaignusecases.ErrConflict) {
		t.Fatalf("lower revision accepted: %v", err)
	}
}

func TestConsentWithdrawnAfterPlanningIsSuppressedAtSend(t *testing.T) {
	h := newHarness(t, nil)
	keep, leave := h.consent.Add("NG-LAG", "push"), h.consent.Add("NG-LAG", "push")
	h.device(keep)
	h.device(leave)
	key, rev, hash := h.template()
	i := campaigntest.Intent(key, rev, hash, []string{"NG-LAG"}, []string{"push"})
	i.StartAt = time.Now().UTC().Add(300 * time.Millisecond)
	h.accept(i)
	h.run(i, 2)
	if s := h.status(i); s.State != campaignmodels.StatePlanned || s.AudienceCount != 2 {
		t.Fatalf("planned: %+v", s)
	}
	h.consent.Withdraw(leave, "push")
	time.Sleep(350 * time.Millisecond)
	s := h.run(i, 20)
	got := h.recipients(i)
	if got[leave+"/push"] != "SUPPRESSED:CONSENT_WITHDRAWN" || got[keep+"/push"] != "SENT" {
		t.Fatalf("recipients: %v", got)
	}
	if s.SuppressedCount != 1 || s.SuppressionReasons["CONSENT_WITHDRAWN"] != 1 || s.SentCount != 1 || s.EligibleCount != 1 {
		t.Fatalf("status: %+v", s)
	}
	if h.messages(i) != 1 {
		t.Fatal("withdrawn customer was messaged")
	}
}

func TestDuplicateRecipientsAreDeduplicatedPerChannel(t *testing.T) {
	h := newHarness(t, nil)
	h.consent.Duplicate = true
	both := h.consent.Add("NG-LAG", "push", "in_app")
	h.device(both)
	key, rev, hash := h.template()
	i := campaigntest.Intent(key, rev, hash, []string{"NG-LAG"}, []string{"push", "in_app"})
	h.accept(i)
	s := h.run(i, 20)
	got := h.recipients(i)
	if len(got) != 2 || got[both+"/push"] != "SENT" || got[both+"/in_app"] != "SENT" || s.AudienceCount != 2 || h.messages(i) != 2 {
		t.Fatalf("dedupe: %v %+v", got, s)
	}
}

func TestQuietHoursAcrossTimezoneBoundaryDeferNotDrop(t *testing.T) {
	// Lagos is UTC+1. 22:30 UTC is 23:30 local: inside a 21:00-07:00 window
	// that spans local midnight; sending resumes at 07:00 local = 06:00 UTC.
	q := &admincampaign.QuietHours{Start: "21:00", End: "07:00", Timezone: "Africa/Lagos"}
	at := time.Date(2026, 10, 26, 22, 30, 0, 0, time.UTC)
	quiet, resume := campaignmodels.QuietDeferral(q, at)
	if !quiet || !resume.Equal(time.Date(2026, 10, 27, 6, 0, 0, 0, time.UTC)) {
		t.Fatalf("quiet=%t resume=%s", quiet, resume)
	}
	// 05:30 UTC is 06:30 local the next morning: still quiet until 06:00 UTC.
	if quiet, resume := campaignmodels.QuietDeferral(q, time.Date(2026, 10, 27, 5, 30, 0, 0, time.UTC)); !quiet || !resume.Equal(time.Date(2026, 10, 27, 6, 0, 0, 0, time.UTC)) {
		t.Fatalf("morning: %t %s", quiet, resume)
	}
	// 20:30 UTC is 21:30 local (quiet) although 20:30 UTC would be outside a
	// UTC-evaluated window: the zone, not UTC, decides.
	if quiet, _ := campaignmodels.QuietDeferral(q, time.Date(2026, 10, 26, 20, 30, 0, 0, time.UTC)); !quiet {
		t.Fatal("window evaluated in UTC instead of the intent zone")
	}
	if quiet, _ := campaignmodels.QuietDeferral(q, time.Date(2026, 10, 26, 6, 30, 0, 0, time.UTC)); quiet {
		t.Fatal("07:30 local must be outside the window")
	}
	if quiet, _ := campaignmodels.QuietDeferral(nil, at); quiet {
		t.Fatal("no quiet-hour default may be invented")
	}

	// Engine: inside the quiet window nothing is sent and recipients wait.
	h := newHarness(t, nil)
	c := h.consent.Add("NG-LAG", "push")
	h.device(c)
	key, rev, hash := h.template()
	i := campaigntest.Intent(key, rev, hash, []string{"NG-LAG"}, []string{"push"})
	local := time.Now().In(mustZone(t, "Africa/Lagos"))
	// A one-hour quiet window around the current local minute.
	start, end := local.Add(-30*time.Minute).Format("15:04"), local.Add(30*time.Minute).Format("15:04")
	i.QuietHours = &admincampaign.QuietHours{Start: start, End: end, Timezone: "Africa/Lagos"}
	h.accept(i)
	h.run(i, 4)
	if got := h.recipients(i); got[c+"/push"] != "PLANNED" || h.messages(i) != 0 {
		t.Fatalf("quiet hours: %v messages=%d", got, h.messages(i))
	}
	var next time.Time
	_ = h.db.QueryRow(context.Background(), `SELECT next_run_at FROM campaigns WHERE campaign_id=$1`, i.CampaignID).Scan(&next)
	if next.Before(time.Now().Add(20 * time.Minute)) {
		t.Fatalf("not deferred to the end of the quiet window: %s", next)
	}
}

func mustZone(t *testing.T, name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestCancellationBeforeSendPreventsAllMessages(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	for k := 0; k < 2; k++ {
		h.device(h.consent.Add("NG-LAG", "push"))
	}
	key, rev, hash := h.template()
	i := campaigntest.Intent(key, rev, hash, []string{"NG-LAG"}, []string{"push"})
	i.StartAt = time.Now().UTC().Add(time.Hour)
	h.accept(i)
	h.run(i, 3)
	cancel := admincampaign.Cancellation{ContractVersion: admincampaign.Contract, CampaignID: i.CampaignID, Revision: 1, CancellationKey: "cancel-" + uuid.NewString(), RequestedBy: "staff-ops", ReasonCode: "OFFER_WITHDRAWN"}
	s, err := h.service.Cancel(ctx, cancel, i.CampaignID)
	if err != nil || s.State != campaignmodels.StateCancelled {
		t.Fatalf("cancel: %+v %v", s, err)
	}
	if again, err := h.service.Cancel(ctx, cancel, i.CampaignID); err != nil || again.State != campaignmodels.StateCancelled {
		t.Fatalf("idempotent cancel: %+v %v", again, err)
	}
	h.run(i, 3)
	if h.messages(i) != 0 {
		t.Fatal("cancelled campaign sent messages")
	}
	for k, v := range h.recipients(i) {
		if v != "CANCELLED" {
			t.Fatalf("%s: %s", k, v)
		}
	}
	other := campaigntest.Intent(key, rev, hash, []string{"NG-LAG"}, []string{"push"})
	h.accept(other)
	reused := cancel
	reused.CampaignID = other.CampaignID
	if _, err := h.service.Cancel(ctx, reused, other.CampaignID); !errors.Is(err, campaignusecases.ErrConflict) {
		t.Fatalf("cancellation key reused for another campaign: %v", err)
	}
}

func TestCancellationWithdrawsQueuedButUnattemptedDeliveries(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	c := h.consent.Add("NG-LAG", "push")
	h.device(c)
	key, rev, hash := h.template()
	i := campaigntest.Intent(key, rev, hash, []string{"NG-LAG"}, []string{"push"})
	h.accept(i)
	for n := 0; n < 3; n++ { // plan and enqueue, but do not process deliveries
		if err := h.engine.Tick(ctx); err != nil {
			t.Fatal(err)
		}
		time.Sleep(3 * time.Millisecond)
	}
	pending := h.queue.drain()
	if len(pending) != 1 {
		t.Fatalf("expected one queued delivery, got %d", len(pending))
	}
	if _, err := h.service.Cancel(ctx, admincampaign.Cancellation{ContractVersion: admincampaign.Contract, CampaignID: i.CampaignID, Revision: 1, CancellationKey: "k-" + uuid.NewString(), RequestedBy: "staff-ops", ReasonCode: "STOP"}, i.CampaignID); err != nil {
		t.Fatal(err)
	}
	if err := h.pipeline.ProcessDelivery(ctx, pending[0]); err != nil {
		t.Fatal(err)
	}
	if h.push.calls != 0 {
		t.Fatal("a cancelled delivery reached the provider")
	}
	if got := h.recipients(i); got[c+"/push"] != "CANCELLED" {
		t.Fatalf("recipient: %v", got)
	}
}

func TestProviderPermanentRejectionFails(t *testing.T) {
	h := newHarness(t, nil)
	c := h.consent.Add("NG-LAG", "push")
	h.device(c)
	h.push.results = []func() (messageclients.ProviderResult, error){func() (messageclients.ProviderResult, error) {
		return messageclients.ProviderResult{Provider: "firebase", Retryable: false}, errors.New("rejected by provider")
	}}
	key, rev, hash := h.template()
	i := campaigntest.Intent(key, rev, hash, []string{"NG-LAG"}, []string{"push"})
	h.accept(i)
	s := h.run(i, 20)
	if s.State != campaignmodels.StateFailed || s.FailedCount != 1 || s.SentCount != 0 {
		t.Fatalf("status: %+v", s)
	}
	assertTruthful(t, s)
}

func TestProviderOutageKeepsQueuedThenRetrySends(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	c := h.consent.Add("NG-LAG", "push")
	h.device(c)
	outage := func() (messageclients.ProviderResult, error) {
		return messageclients.ProviderResult{Provider: "firebase", Retryable: true}, errors.New("provider unavailable")
	}
	h.push.results = []func() (messageclients.ProviderResult, error){outage, func() (messageclients.ProviderResult, error) {
		id := "fcm-recovered"
		return messageclients.ProviderResult{Provider: "firebase", ProviderMessageID: &id}, nil
	}}
	key, rev, hash := h.template()
	i := campaigntest.Intent(key, rev, hash, []string{"NG-LAG"}, []string{"push"})
	h.accept(i)
	s := h.run(i, 6)
	if s.State != campaignmodels.StateQueued || s.QueuedCount != 1 || h.recipients(i)[c+"/push"] != "QUEUED" {
		t.Fatalf("outage must keep the recipient queued: %+v %v", s, h.recipients(i))
	}
	var status string
	_ = h.db.QueryRow(ctx, `SELECT status FROM notification_messages WHERE idempotency_key=$1`, campaignmodels.MessageIdempotencyKey(i.CampaignID, 1, c, "push")).Scan(&status)
	if status != "retrying" {
		t.Fatalf("message status during outage: %s", status)
	}
	// The existing retry mechanism re-enqueues the due delivery.
	if _, err := h.db.Exec(ctx, `UPDATE notification_deliveries SET next_attempt_at=now() WHERE status='retrying'`); err != nil {
		t.Fatal(err)
	}
	due, err := messagerepositories.NewPostgresNotificationRepository(h.db).ListDueRetryDeliveries(ctx, 10)
	if err != nil || len(due) != 1 {
		t.Fatalf("due retries: %d %v", len(due), err)
	}
	if err := h.pipeline.ProcessDelivery(ctx, due[0].ID); err != nil {
		t.Fatal(err)
	}
	s = h.run(i, 6)
	if s.State != campaignmodels.StateCompleted || s.SentCount != 1 {
		t.Fatalf("after recovery: %+v", s)
	}
}

func TestLoggingProviderIsLoggedOnlyNeverSent(t *testing.T) {
	h := newHarness(t, messageclients.NewLoggingPushSender())
	c := h.consent.Add("NG-LAG", "push")
	h.device(c)
	key, rev, hash := h.template()
	i := campaigntest.Intent(key, rev, hash, []string{"NG-LAG"}, []string{"push"})
	h.accept(i)
	s := h.run(i, 20)
	if h.recipients(i)[c+"/push"] != "LOGGED_ONLY" || s.SentCount != 0 || s.LoggedOnlyCount != 1 || s.EligibleCount != 1 {
		t.Fatalf("logging provider: %+v %v", s, h.recipients(i))
	}
	assertTruthful(t, s)
	// The owner event and the snapshot carry the same split.
	var raw []byte
	if err := h.db.QueryRow(context.Background(), `SELECT payload FROM admin_event_outbox WHERE aggregate_id=$1 ORDER BY (payload->>'entity_version')::bigint DESC LIMIT 1`, i.CampaignID+":1").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	ev, err := adminevents.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	adapter, _ := adminevents.Lookup("notification-service", adminevents.NotificationCampaignStat)
	payload, err := adapter.Minimize(ev)
	if err != nil || payload["logged_only_count"] != int64(1) || payload["sent_count"] != int64(0) {
		t.Fatalf("event payload: %v %v", payload, err)
	}
	page, err := h.service.Snapshot(context.Background(), "", 200)
	if err != nil || len(page.Events) != 1 || string(page.Events[0].Payload["logged_only_count"]) != "1" || string(page.Events[0].Payload["sent_count"]) != "0" {
		t.Fatalf("snapshot payload: %+v %v", page, err)
	}
}

func TestCapsPreferencesAndContactSuppressions(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	key, rev, hash := h.template()
	// Frequency cap across campaigns.
	c := h.consent.Add("NG-LAG", "push")
	h.device(c)
	first := campaigntest.Intent(key, rev, hash, []string{"NG-LAG"}, []string{"push"})
	first.MaxPerRecipientPerDay = 1
	h.accept(first)
	h.run(first, 20)
	second := campaigntest.Intent(key, rev, hash, []string{"NG-LAG"}, []string{"push"})
	second.MaxPerRecipientPerDay = 1
	h.accept(second)
	s := h.run(second, 20)
	if h.recipients(second)[c+"/push"] != "SUPPRESSED:FREQUENCY_CAP" || s.State != campaignmodels.StateSuppressed {
		t.Fatalf("frequency cap: %+v %v", s, h.recipients(second))
	}
	// Campaign cap and channel preference.
	h2 := newHarness(t, nil)
	key2, rev2, hash2 := h2.template()
	var ids []string
	for k := 0; k < 3; k++ {
		id := h2.consent.Add("NG-LAG", "push")
		h2.device(id)
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if _, err := h2.db.Exec(ctx, `INSERT INTO notification_preferences(recipient_type,recipient_id,channel,enabled) VALUES('customer',$1,'push',false)`, ids[0]); err != nil {
		t.Fatal(err)
	}
	capped := campaigntest.Intent(key2, rev2, hash2, []string{"NG-LAG"}, []string{"push"})
	capped.MaxMessages = 1
	h2.accept(capped)
	s = h2.run(capped, 20)
	got := h2.recipients(capped)
	if got[ids[0]+"/push"] != "SUPPRESSED:CHANNEL_DISABLED" || got[ids[1]+"/push"] != "SENT" || got[ids[2]+"/push"] != "SUPPRESSED:CAMPAIGN_CAP" || s.SentCount != 1 || s.SuppressedCount != 2 {
		t.Fatalf("caps: %v %+v", got, s)
	}
	// No device: the pipeline skip becomes a recipient suppression.
	h3 := newHarness(t, nil)
	key3, rev3, hash3 := h3.template()
	lonely := h3.consent.Add("NG-LAG", "push")
	i3 := campaigntest.Intent(key3, rev3, hash3, []string{"NG-LAG"}, []string{"push"})
	h3.accept(i3)
	if s := h3.run(i3, 20); h3.recipients(i3)[lonely+"/push"] != "SUPPRESSED:NO_ACTIVE_DEVICE" || s.SentCount != 0 {
		t.Fatalf("no device: %+v %v", s, h3.recipients(i3))
	}
}

func TestEmailChannelUsesAddressFromConsentRecheck(t *testing.T) {
	h := newHarness(t, nil)
	c := h.consent.Add("NG-LAG", "email")
	key, rev, hash := h.template()
	i := campaigntest.Intent(key, rev, hash, []string{"NG-LAG"}, []string{"email"})
	h.accept(i)
	if s := h.run(i, 20); s.SentCount != 1 || h.recipients(i)[c+"/email"] != "SENT" {
		t.Fatalf("email: %+v %v", s, h.recipients(i))
	}
}

func TestConsentAuthorityUnavailableNeverSends(t *testing.T) {
	h := newHarness(t, nil)
	c := h.consent.Add("NG-LAG", "push")
	h.device(c)
	key, rev, hash := h.template()
	i := campaigntest.Intent(key, rev, hash, []string{"NG-LAG"}, []string{"push"})
	h.accept(i)
	h.run(i, 1) // planned
	if s := h.status(i); s.State != campaignmodels.StatePlanned {
		t.Fatalf("not planned: %+v", s)
	}
	h.consent.Unavailable = true
	h.run(i, 4)
	if h.messages(i) != 0 || h.recipients(i)[c+"/push"] != "PLANNED" {
		t.Fatalf("unknown eligibility was treated as permission: %v", h.recipients(i))
	}
}

func TestCategoryTemplateAndZoneRefusals(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	key, rev, hash := h.template()
	for _, category := range []string{"EMERGENCY", "SAFETY", "", "marketing"} {
		i := campaigntest.Intent(key, rev, hash, []string{"NG-LAG"}, []string{"push"})
		i.Category = category
		if _, _, err := h.service.Accept(ctx, campaigntest.Raw(t, i)); !errors.Is(err, campaignusecases.ErrInvalid) {
			t.Fatalf("category %q accepted: %v", category, err)
		}
	}
	unknown := strings.Replace(string(campaigntest.Raw(t, campaigntest.Intent(key, rev, hash, []string{"NG-LAG"}, []string{"push"}))), `"category"`, `"html":"<b>x</b>","category"`, 1)
	if _, _, err := h.service.Accept(ctx, []byte(unknown)); !errors.Is(err, campaignusecases.ErrInvalid) {
		t.Fatalf("unknown field accepted: %v", err)
	}
	mismatch := campaigntest.Intent(key, rev, strings.Repeat("0", 64), []string{"NG-LAG"}, []string{"push"})
	h.accept(mismatch)
	if s := h.run(mismatch, 3); s.State != campaignmodels.StateRejected || s.RejectionCode != "TEMPLATE_MISMATCH" {
		t.Fatalf("hash mismatch: %+v", s)
	}
	stale := campaigntest.Intent(key, rev+1, hash, []string{"NG-LAG"}, []string{"push"})
	h.accept(stale)
	if s := h.run(stale, 3); s.RejectionCode != "TEMPLATE_MISMATCH" {
		t.Fatalf("revision mismatch: %+v", s)
	}
	zoned := campaigntest.Intent(key, rev, hash, []string{"NG-LAG"}, []string{"push"})
	zoned.Zones = []string{"LAG-IKEJA"}
	h.accept(zoned)
	if s := h.run(zoned, 3); s.RejectionCode != "AUDIENCE_DIMENSION_UNAVAILABLE" {
		t.Fatalf("zones: %+v", s)
	}
	// Editing the template advances its revision; a campaign bound to the
	// old revision is refused rather than sent with new copy.
	if _, err := h.db.Exec(ctx, `UPDATE notification_templates SET body='Changed copy' WHERE key=$1`, key); err != nil {
		t.Fatal(err)
	}
	old := campaigntest.Intent(key, rev, hash, []string{"NG-LAG"}, []string{"push"})
	h.accept(old)
	if s := h.run(old, 3); s.RejectionCode != "TEMPLATE_MISMATCH" {
		t.Fatalf("edited template: %+v", s)
	}
}

func TestOwnerEventsMatchTheAdapterAndSnapshot(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()
	c := h.consent.Add("NG-LAG", "push")
	h.device(c)
	key, rev, hash := h.template()
	i := campaigntest.Intent(key, rev, hash, []string{"NG-LAG"}, []string{"push"})
	i.CorrelationID = "corr-1"
	h.accept(i)
	final := h.run(i, 20)
	rows, err := h.db.Query(ctx, `SELECT payload FROM admin_event_outbox WHERE aggregate_id=$1 ORDER BY created_at, (payload->>'entity_version')::bigint`, i.CampaignID+":1")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	adapter, ok := adminevents.Lookup("notification-service", adminevents.NotificationCampaignStat)
	if !ok {
		t.Fatal("adapter not registered")
	}
	var versions []int64
	var states []string
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		ev, err := adminevents.Decode(raw)
		if err != nil {
			t.Fatalf("event does not decode: %v", err)
		}
		payload, err := adapter.Minimize(ev)
		if err != nil {
			t.Fatalf("event rejected by adapter: %v %s", err, raw)
		}
		if ev.EntityType != "campaign_revision" || ev.EntityRef != i.CampaignID+":1" || ev.Scope.Market != "NG-LAG" || ev.CorrelationID != "corr-1" || payload["delivered_count"] != nil || payload["delivery_confirmation"] != "UNAVAILABLE" {
			t.Fatalf("event: %+v %v", ev, payload)
		}
		if strings.Contains(string(raw), "Weekend offer") || strings.Contains(string(raw), "example.invalid") {
			t.Fatal("event carries message content or contact data")
		}
		versions = append(versions, ev.EntityVersion)
		states = append(states, payload["state"].(string))
	}
	if len(versions) < 3 || states[0] != "ACCEPTED_BY_KARRYGO" || states[len(states)-1] != final.State {
		t.Fatalf("event history: %v %v", versions, states)
	}
	for k := 1; k < len(versions); k++ {
		if versions[k] != versions[k-1]+1 {
			t.Fatalf("versions not contiguous: %v", versions)
		}
	}
	page, err := h.service.Snapshot(ctx, "", 200)
	if err != nil || !page.Complete || len(page.Events) != 1 || page.Events[0].EntityVersion != versions[len(versions)-1] {
		t.Fatalf("snapshot: %+v %v", page, err)
	}
	raw, _ := json.Marshal(page)
	if strings.Contains(string(raw), "Weekend offer") {
		t.Fatal("snapshot carries message content")
	}
	// Paging with a cursor.
	second := campaigntest.Intent(key, rev, hash, []string{"NG-LAG"}, []string{"push"})
	h.accept(second)
	first, err := h.service.Snapshot(ctx, "", 1)
	if err != nil || first.Complete || first.NextCursor == "" {
		t.Fatalf("first page: %+v %v", first, err)
	}
	rest, err := h.service.Snapshot(ctx, first.NextCursor, 1)
	if err != nil || !rest.Complete || len(rest.Events) != 1 || rest.Events[0].EntityRef == first.Events[0].EntityRef {
		t.Fatalf("second page: %+v %v", rest, err)
	}
}

func TestCampaignIntentIsImmutable(t *testing.T) {
	h := newHarness(t, nil)
	key, rev, hash := h.template()
	i := campaigntest.Intent(key, rev, hash, []string{"NG-LAG"}, []string{"push"})
	h.accept(i)
	for _, stmt := range []string{`UPDATE campaigns SET intent='{}'::jsonb`, `UPDATE campaigns SET intent_digest=repeat('0',64)`, `DELETE FROM campaigns`} {
		if _, err := h.db.Exec(context.Background(), stmt); err == nil {
			t.Fatalf("%s succeeded", stmt)
		}
	}
}
