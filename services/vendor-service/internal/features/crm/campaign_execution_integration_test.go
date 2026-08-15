package crm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/shared/go/notifications"
	"cosmicforge/logistics/shared/go/serviceauth"
)

type recordingNotifier struct {
	mu       sync.Mutex
	requests []notifications.Request
	err      error
}

func (n *recordingNotifier) Send(_ context.Context, r notifications.Request) (notifications.SendResponse, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.requests = append(n.requests, r)
	if n.err != nil {
		return notifications.SendResponse{}, n.err
	}
	return notifications.SendResponse{MessageID: "msg_" + r.IDempotencyKey, Status: "accepted"}, nil
}
func (n *recordingNotifier) count() int { n.mu.Lock(); defer n.mu.Unlock(); return len(n.requests) }

func executionFixture(t *testing.T, in CampaignDefinitionInput) (*testEnv, *recordingNotifier, Profile, Campaign, CampaignSnapshot) {
	t.Helper()
	env := newTestEnv(t, allFlagsOn())
	notifier := &recordingNotifier{}
	env.service.notifier = notifier
	p := env.seedProfile(t, businessA, "vs_a", customerSubject("cus_execution"))
	env.allowMarketing(t, businessA, "vs_a", p.ID, ChannelEmail, 1)
	seg := env.staticAudience(t, businessA, "vs_a", "Execution audience", p.ID)
	if in.Name == "" {
		in = campaignInput("Execution campaign", seg.ID, string(ChannelEmail))
	} else {
		in.SourceSegmentRef = seg.ID
	}
	campaign := env.createCampaign(t, businessA, "vs_a", in)
	snapshot := env.prepareAndRun(t, businessA, "vs_a", campaign.ID)
	return env, notifier, p, campaign, snapshot
}

func createExecutionRun(t *testing.T, e *testEnv, c Campaign, s CampaignSnapshot) CampaignRun {
	t.Helper()
	c = e.reloadCampaign(t, c.BusinessID, c.ID)
	r, err := e.service.CreateCampaignRun(context.Background(), actorFor(businessA, "vs_a"), globalScope(), c.ID, CreateCampaignRunInput{SnapshotRef: s.ID, Mode: "SEND_NOW", ExpectedVersion: c.Version, TemplateKey: "crm.test"})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	return r
}
func dispatchRun(t *testing.T, e *testEnv) {
	t.Helper()
	if n, err := e.service.RunCampaignSchedulerPass(context.Background(), 10); err != nil || n != 1 {
		t.Fatalf("schedule pass=%d err=%v", n, err)
	}
	if n, err := e.service.RunCampaignDeliveryPass(context.Background(), 10); err != nil || n < 1 {
		t.Fatalf("delivery pass=%d err=%v", n, err)
	}
}

func TestCampaignExecution_HandoffUsesOpaqueRecipientAndStableIdempotency(t *testing.T) {
	e, n, _, c, s := executionFixture(t, CampaignDefinitionInput{})
	r := createExecutionRun(t, e, c, s)
	dispatchRun(t, e)
	if n.count() != 1 {
		t.Fatalf("handoffs=%d want=1", n.count())
	}
	req := n.requests[0]
	if req.Recipient.ID != "cus_execution" || req.Recipient.Email != "" || req.Recipient.Phone != "" {
		t.Fatalf("recipient leaked or wrong: %+v", req.Recipient)
	}
	ds, _, err := e.service.ListCampaignDeliveries(context.Background(), actorFor(businessA, "vs_a"), globalScope(), c.ID, r.ID, 10, 0)
	if err != nil || len(ds) != 1 || ds[0].Status != DeliveryHandedOff {
		t.Fatalf("deliveries=%+v err=%v", ds, err)
	}
	if req.IDempotencyKey != ds[0].NotificationRequestRef {
		t.Fatalf("idempotency changed: %s != %s", req.IDempotencyKey, ds[0].NotificationRequestRef)
	}
	if derefOr(ds[0].ConsentDecisionAtHandoff) != "ALLOW" || ds[0].ConsentEligibleAtHandoff == nil || !*ds[0].ConsentEligibleAtHandoff || ds[0].ConsentCheckedAt == nil || ds[0].ConsentSourceVersion == nil || *ds[0].ConsentSourceVersion != 1 {
		t.Fatalf("send-time consent evidence=%+v", ds[0])
	}
}

func TestCampaignExecution_SendTimeConsentWithdrawalFailsClosed(t *testing.T) {
	e, n, p, c, s := executionFixture(t, CampaignDefinitionInput{})
	r := createExecutionRun(t, e, c, s)
	e.projectConsent(t, businessA, "vs_a", p.ID, ChannelEmail, ConsentDeny, 2, nil)
	dispatchRun(t, e)
	if n.count() != 0 {
		t.Fatal("withdrawn consent reached Notification Service")
	}
	ds, _, _ := e.service.ListCampaignDeliveries(context.Background(), actorFor(businessA, "vs_a"), globalScope(), c.ID, r.ID, 10, 0)
	if len(ds) != 1 || ds[0].Status != DeliverySuppressed || derefOr(ds[0].SuppressionReason) != "CONSENT_NOT_ALLOWED" {
		t.Fatalf("delivery=%+v", ds)
	}
	if derefOr(ds[0].ConsentDecisionAtHandoff) != "DENY" || ds[0].ConsentEligibleAtHandoff == nil || *ds[0].ConsentEligibleAtHandoff {
		t.Fatalf("withdrawal evidence=%+v", ds[0])
	}
}

func TestCampaignExecution_SMSIsUnavailableAndNeverDispatched(t *testing.T) {
	e, n, p, _, _ := executionFixture(t, CampaignDefinitionInput{})
	seg := e.staticAudience(t, businessA, "vs_a", "SMS audience", p.ID)
	e.allowMarketing(t, businessA, "vs_a", p.ID, ChannelSMS, 1)
	c := e.createCampaign(t, businessA, "vs_a", campaignInput("SMS campaign", seg.ID, string(ChannelSMS)))
	s := e.prepareAndRun(t, businessA, "vs_a", c.ID)
	r := createExecutionRun(t, e, c, s)
	dispatchRun(t, e)
	if n.count() != 0 {
		t.Fatal("SMS was advertised to Notification Service")
	}
	ds, _, _ := e.service.ListCampaignDeliveries(context.Background(), actorFor(businessA, "vs_a"), globalScope(), c.ID, r.ID, 10, 0)
	if len(ds) != 1 || derefOr(ds[0].SuppressionReason) != "CHANNEL_UNAVAILABLE" {
		t.Fatalf("delivery=%+v", ds)
	}
}

func TestCampaignExecution_QuietHoursDefersToWindowEnd(t *testing.T) {
	yes := true
	in := CampaignDefinitionInput{Name: "Quiet campaign", Objective: ObjectiveGeneralMarketing, ScopeType: ScopeBusiness, Channels: []string{string(ChannelEmail)}, QuietHoursEnabled: &yes, QuietHoursStart: "13:00", QuietHoursEnd: "15:00", QuietHoursTimezone: "Africa/Lagos"}
	e, n, _, c, s := executionFixture(t, in)
	r := createExecutionRun(t, e, c, s)
	dispatchRun(t, e)
	if n.count() != 0 {
		t.Fatal("quiet-hours delivery handed off")
	}
	ds, _, _ := e.service.ListCampaignDeliveries(context.Background(), actorFor(businessA, "vs_a"), globalScope(), c.ID, r.ID, 10, 0)
	want := time.Date(2026, 8, 11, 14, 0, 0, 0, time.UTC)
	if len(ds) != 1 || ds[0].Status != DeliveryDeferred || !ds[0].NextAttemptAt.Equal(want) {
		t.Fatalf("delivery=%+v want next=%s", ds, want)
	}
}

func TestCampaignExecution_QuietHoursDSTTransitions(t *testing.T) {
	policy := QuietHoursPolicy{Enabled: true, StartMinute: 60, EndMinute: 240, Timezone: "America/New_York"}
	spring := time.Date(2026, 3, 8, 6, 30, 0, 0, time.UTC)
	got, err := nextQuietEnd(policy, spring)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 3, 8, 8, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("spring next=%s want=%s", got, want)
	}
	policy.StartMinute, policy.EndMinute = 0, 180
	fall := time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)
	got, err = nextQuietEnd(policy, fall)
	if err != nil {
		t.Fatal(err)
	}
	want = time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("fall next=%s want=%s", got, want)
	}
}

func TestCampaignExecution_FrequencyCapAcrossRuns(t *testing.T) {
	cap, window := 1, 1440
	in := CampaignDefinitionInput{Name: "Capped campaign", Objective: ObjectiveGeneralMarketing, ScopeType: ScopeBusiness, Channels: []string{string(ChannelEmail)}, FrequencyCapCount: &cap, FrequencyCapWindowMinutes: &window}
	e, n, _, c, s := executionFixture(t, in)
	r1 := createExecutionRun(t, e, c, s)
	dispatchRun(t, e)
	r2 := createExecutionRun(t, e, c, s)
	dispatchRun(t, e)
	if n.count() != 1 {
		t.Fatalf("handoffs=%d want=1", n.count())
	}
	ds, _, _ := e.service.ListCampaignDeliveries(context.Background(), actorFor(businessA, "vs_a"), globalScope(), c.ID, r2.ID, 10, 0)
	if len(ds) != 1 || derefOr(ds[0].SuppressionReason) != "FREQUENCY_CAP" {
		t.Fatalf("second delivery=%+v first=%s", ds, r1.ID)
	}
}

func TestCampaignExecution_TimeoutRetriesOneLogicalNotification(t *testing.T) {
	e, n, _, c, s := executionFixture(t, CampaignDefinitionInput{})
	n.err = errors.New("timeout")
	r := createExecutionRun(t, e, c, s)
	dispatchRun(t, e)
	ds, _, _ := e.service.ListCampaignDeliveries(context.Background(), actorFor(businessA, "vs_a"), globalScope(), c.ID, r.ID, 10, 0)
	if len(ds) != 1 || ds[0].Status != DeliveryFailedRetry {
		t.Fatalf("delivery=%+v", ds)
	}
	first := n.requests[0].IDempotencyKey
	e.now = e.now.Add(3 * time.Minute)
	n.err = nil
	if got, err := e.service.RunCampaignDeliveryPass(context.Background(), 10); err != nil || got != 1 {
		t.Fatalf("retry got=%d err=%v", got, err)
	}
	if n.count() != 2 || n.requests[1].IDempotencyKey != first {
		t.Fatalf("retry identities=%+v", n.requests)
	}
}

func TestCampaignDeliveryEvents_DedupeAndDoNotRegress(t *testing.T) {
	e, _, _, c, s := executionFixture(t, CampaignDefinitionInput{})
	r := createExecutionRun(t, e, c, s)
	dispatchRun(t, e)
	ds, _, _ := e.service.ListCampaignDeliveries(context.Background(), actorFor(businessA, "vs_a"), globalScope(), c.ID, r.ID, 10, 0)
	d := ds[0]
	delivered := DeliveryEvent{EventID: "evt_delivered", DeliveryRef: d.ID, Status: DeliveryDelivered, SourceVersion: 2, OccurredAt: e.now}
	applied, err := e.service.IngestCampaignDeliveryEvent(context.Background(), "notification-service", delivered)
	if err != nil || !applied {
		t.Fatalf("delivered applied=%t err=%v", applied, err)
	}
	applied, err = e.service.IngestCampaignDeliveryEvent(context.Background(), "notification-service", delivered)
	if err != nil || applied {
		t.Fatalf("duplicate applied=%t err=%v", applied, err)
	}
	applied, err = e.service.IngestCampaignDeliveryEvent(context.Background(), "notification-service", DeliveryEvent{EventID: "evt_late", DeliveryRef: d.ID, Status: DeliveryAccepted, SourceVersion: 3, OccurredAt: e.now})
	if err != nil || applied {
		t.Fatalf("regression applied=%t err=%v", applied, err)
	}
	current, err := e.service.GetCampaignDelivery(context.Background(), actorFor(businessA, "vs_a"), globalScope(), c.ID, r.ID, d.ID)
	if err != nil || current.Status != DeliveryDelivered {
		t.Fatalf("current=%+v err=%v", current, err)
	}
}

func TestCampaignExecution_PrivacyChangeAfterSnapshotSuppresses(t *testing.T) {
	e, n, p, c, s := executionFixture(t, CampaignDefinitionInput{})
	r := createExecutionRun(t, e, c, s)
	if err := e.tx(t, func(tx pgx.Tx) error {
		return e.service.ApplyPrivacy(context.Background(), tx, actorFor(businessA, "vs_a"), &p, PrivacyInput{PrivacySubjectRef: "cus_execution", Action: PrivacyActionRestrict, SourceService: "customer-service", SourceRef: ptr("privacy_1"), SourceVersion: 1, ObservedAt: e.now})
	}); err != nil {
		t.Fatalf("privacy: %v", err)
	}
	dispatchRun(t, e)
	if n.count() != 0 {
		t.Fatal("privacy-restricted customer reached Notification Service")
	}
	ds, _, _ := e.service.ListCampaignDeliveries(context.Background(), actorFor(businessA, "vs_a"), globalScope(), c.ID, r.ID, 10, 0)
	if len(ds) != 1 || derefOr(ds[0].SuppressionReason) != "PRIVACY_OR_IDENTITY_INELIGIBLE" {
		t.Fatalf("delivery=%+v", ds)
	}
}

func TestCampaignExecution_MergeLeavesOnlyCanonicalSurvivorActionable(t *testing.T) {
	e := newTestEnv(t, allFlagsOn())
	n := &recordingNotifier{}
	e.service.notifier = n
	source := e.seedProfile(t, businessA, "vs_a", guestSubject("gst_execution_merge"))
	survivor := e.seedProfile(t, businessA, "vs_a", customerSubject("cus_execution_merge"))
	e.allowMarketing(t, businessA, "vs_a", source.ID, ChannelEmail, 1)
	e.allowMarketing(t, businessA, "vs_a", survivor.ID, ChannelEmail, 1)
	segment := e.staticAudience(t, businessA, "vs_a", "Merge execution audience", source.ID, survivor.ID)
	campaign := e.createCampaign(t, businessA, "vs_a", campaignInput("Merge execution", segment.ID, string(ChannelEmail)))
	snapshot := e.prepareAndRun(t, businessA, "vs_a", campaign.ID)
	run := createExecutionRun(t, e, campaign, snapshot)
	if err := e.applyLink(t, businessA, "vs_a", linkInput("evt_execution_merge", guestSubject("gst_execution_merge"), customerSubject("cus_execution_merge"), 2)); err != nil {
		t.Fatal(err)
	}
	dispatchRun(t, e)
	if n.count() != 1 {
		t.Fatalf("handoffs=%d want canonical survivor only", n.count())
	}
	if n.requests[0].Recipient.ID != "cus_execution_merge" {
		t.Fatalf("recipient=%+v", n.requests[0].Recipient)
	}
	ds, _, _ := e.service.ListCampaignDeliveries(context.Background(), actorFor(businessA, "vs_a"), globalScope(), campaign.ID, run.ID, 10, 0)
	suppressed, handed := 0, 0
	for _, d := range ds {
		if d.Status == DeliverySuppressed {
			suppressed++
		}
		if d.Status == DeliveryHandedOff {
			handed++
		}
	}
	if suppressed != 1 || handed != 1 {
		t.Fatalf("deliveries=%+v", ds)
	}
}

func TestCampaignExecution_MergeAfterSourceHandoffSuppressesSurvivorDuplicate(t *testing.T) {
	e := newTestEnv(t, allFlagsOn())
	n := &recordingNotifier{}
	e.service.notifier = n
	source := e.seedProfile(t, businessA, "vs_a", customerSubject("cus_execution_handoff_first"))
	survivor := e.seedProfile(t, businessA, "vs_a", customerSubject("cus_execution_handoff_second"))
	e.allowMarketing(t, businessA, "vs_a", source.ID, ChannelEmail, 1)
	e.allowMarketing(t, businessA, "vs_a", survivor.ID, ChannelEmail, 1)
	segment := e.staticAudience(t, businessA, "vs_a", "Merge after handoff", source.ID, survivor.ID)
	campaign := e.createCampaign(t, businessA, "vs_a", campaignInput("Merge after source handoff", segment.ID, string(ChannelEmail)))
	snapshot := e.prepareAndRun(t, businessA, "vs_a", campaign.ID)
	run := createExecutionRun(t, e, campaign, snapshot)
	if _, err := e.service.RunCampaignSchedulerPass(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(context.Background(), `UPDATE crm_campaign_deliveries SET next_attempt_at=$1 WHERE run_id=$2 AND customer_profile_id=$3`, e.now.Add(time.Hour), run.ID, survivor.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := e.service.RunCampaignDeliveryPass(context.Background(), 1); err != nil || got != 1 {
		t.Fatalf("source handoff=%d err=%v", got, err)
	}
	if n.count() != 1 || n.requests[0].Recipient.ID != "cus_execution_handoff_first" {
		t.Fatalf("first handoff=%+v", n.requests)
	}
	if err := e.applyLink(t, businessA, "vs_a", linkInput("evt_merge_after_handoff", customerSubject("cus_execution_handoff_first"), customerSubject("cus_execution_handoff_second"), 2)); err != nil {
		t.Fatal(err)
	}
	e.now = e.now.Add(2 * time.Hour)
	if got, err := e.service.RunCampaignDeliveryPass(context.Background(), 10); err != nil || got != 0 {
		t.Fatalf("post-merge dispatcher=%d err=%v", got, err)
	}
	if n.count() != 1 {
		t.Fatalf("duplicate canonical handoff count=%d", n.count())
	}
	ds, _, _ := e.service.ListCampaignDeliveries(context.Background(), actorFor(businessA, "vs_a"), globalScope(), campaign.ID, run.ID, 10, 0)
	for _, d := range ds {
		if d.ProfileID == survivor.ID && (d.Status != DeliverySuppressed || derefOr(d.SuppressionReason) != "MERGE_DUPLICATE") {
			t.Fatalf("survivor delivery=%+v", d)
		}
	}
}

func TestCampaignExecution_ConcurrentFrequencyFenceAllowsOneHandoff(t *testing.T) {
	cap, window := 1, 1440
	in := CampaignDefinitionInput{Name: "Concurrent cap", Objective: ObjectiveGeneralMarketing, ScopeType: ScopeBusiness, Channels: []string{string(ChannelEmail)}, FrequencyCapCount: &cap, FrequencyCapWindowMinutes: &window}
	e, n, _, c, s := executionFixture(t, in)
	r1 := createExecutionRun(t, e, c, s)
	r2 := createExecutionRun(t, e, c, s)
	if got, err := e.service.RunCampaignSchedulerPass(context.Background(), 10); err != nil || got != 2 {
		t.Fatalf("scheduler=%d err=%v", got, err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.service.RunCampaignDeliveryPass(context.Background(), 1)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if n.count() != 1 {
		t.Fatalf("handoffs=%d want=1", n.count())
	}
	for _, run := range []CampaignRun{r1, r2} {
		ds, _, err := e.service.ListCampaignDeliveries(context.Background(), actorFor(businessA, "vs_a"), globalScope(), c.ID, run.ID, 10, 0)
		if err != nil || len(ds) != 1 {
			t.Fatalf("run %s deliveries=%+v err=%v", run.ID, ds, err)
		}
	}
}

func TestCampaignExecution_CancelPreventsPendingHandoff(t *testing.T) {
	e, n, _, c, s := executionFixture(t, CampaignDefinitionInput{})
	r := createExecutionRun(t, e, c, s)
	r, err := e.service.CancelCampaignRun(context.Background(), actorFor(businessA, "vs_a"), globalScope(), c.ID, r.ID, r.Version)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != CampaignRunCancelled {
		t.Fatalf("status=%s", r.Status)
	}
	if got, err := e.service.RunCampaignSchedulerPass(context.Background(), 10); err != nil || got != 0 {
		t.Fatalf("scheduler=%d err=%v", got, err)
	}
	if got, err := e.service.RunCampaignDeliveryPass(context.Background(), 10); err != nil || got != 0 {
		t.Fatalf("dispatcher=%d err=%v", got, err)
	}
	if n.count() != 0 {
		t.Fatal("cancelled run handed off")
	}
	ds, _, _ := e.service.ListCampaignDeliveries(context.Background(), actorFor(businessA, "vs_a"), globalScope(), c.ID, r.ID, 10, 0)
	if len(ds) != 1 || ds[0].Status != DeliveryCancelled {
		t.Fatalf("delivery=%+v", ds)
	}
	hs, _, _ := e.service.CampaignDeliveryHistory(context.Background(), actorFor(businessA, "vs_a"), globalScope(), c.ID, r.ID, ds[0].ID, 10, 0)
	if len(hs) != 2 || hs[1].ToStatus != DeliveryCancelled {
		t.Fatalf("history=%+v", hs)
	}
}

func TestCampaignExecution_KillSwitchStopsCommandsAndWorkers(t *testing.T) {
	e, n, _, c, s := executionFixture(t, CampaignDefinitionInput{})
	r := createExecutionRun(t, e, c, s)
	e.service.flags.CampaignExecutionEnabled = false
	if _, err := e.service.CreateCampaignRun(context.Background(), actorFor(businessA, "vs_a"), globalScope(), c.ID, CreateCampaignRunInput{SnapshotRef: s.ID, Mode: "SEND_NOW", ExpectedVersion: e.reloadCampaign(t, businessA, c.ID).Version, TemplateKey: "x"}); err == nil {
		t.Fatal("execution command passed disabled flag")
	}
	if got, err := e.service.RunCampaignSchedulerPass(context.Background(), 10); err != nil || got != 0 {
		t.Fatalf("scheduler=%d err=%v", got, err)
	}
	if got, err := e.service.RunCampaignDeliveryPass(context.Background(), 10); err != nil || got != 0 {
		t.Fatalf("dispatcher=%d err=%v", got, err)
	}
	if n.count() != 0 {
		t.Fatal("kill switch still handed off")
	}
	if _, err := e.service.GetCampaignRun(context.Background(), actorFor(businessA, "vs_a"), globalScope(), c.ID, r.ID); err != nil {
		t.Fatalf("historical run hidden: %v", err)
	}
}

func TestCampaignExecutionHTTP_RegisteredSurfaceIsNineRoutes(t *testing.T) {
	e := newTestEnv(t, allFlagsOn())
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/vendor")
	group.Use(func(c *gin.Context) {
		authctx.Set(c, authctx.StaffContext{StaffID: "vs_a", VendorID: businessA, SessionID: "session"})
		c.Next()
	})
	RegisterCampaignExecutionRoutes(group, NewCampaignHandler(e.service, idempotency.NewStore(e.pool, 0)), e.service.FlagSet())
	registered := map[string]bool{}
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = true
	}
	want := []string{"POST /api/v1/vendor/crm/campaigns/:campaignId/runs", "GET /api/v1/vendor/crm/campaigns/:campaignId/runs", "GET /api/v1/vendor/crm/campaigns/:campaignId/runs/:runId", "POST /api/v1/vendor/crm/campaigns/:campaignId/runs/:runId/cancel", "POST /api/v1/vendor/crm/campaigns/:campaignId/runs/:runId/pause", "POST /api/v1/vendor/crm/campaigns/:campaignId/runs/:runId/resume", "GET /api/v1/vendor/crm/campaigns/:campaignId/runs/:runId/deliveries", "GET /api/v1/vendor/crm/campaigns/:campaignId/runs/:runId/deliveries/:deliveryId", "GET /api/v1/vendor/crm/campaigns/:campaignId/runs/:runId/deliveries/:deliveryId/history"}
	if len(registered) != len(want) {
		t.Fatalf("routes=%v", registered)
	}
	for _, route := range want {
		if !registered[route] {
			t.Errorf("missing %s", route)
		}
	}
}

func TestCampaignDeliveryEventHTTP_RequiresNotificationHMACAndSingleUseNonce(t *testing.T) {
	e := newTestEnv(t, allFlagsOn())
	gin.SetMode(gin.TestMode)
	router := gin.New()
	verifier := serviceauth.NewVerifier(serviceauth.ParseSecrets("notification-service=secret,customer-service=secret"), 5*time.Minute)
	RegisterInternalRoutes(router, NewInternalHandler(e.service), verifier, e.service.FlagSet())
	body, _ := json.Marshal(deliveryEventRequest{EventID: "evt_security", DeliveryRef: "missing_delivery", Status: DeliveryDelivered, SourceVersion: 1, OccurredAt: time.Now().UTC()})
	path := "/internal/vendor/v1/crm/campaign-delivery-events"
	unsigned := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, unsigned)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unsigned status=%d", rec.Code)
	}
	wrong := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	if err := serviceauth.SignRequest(wrong, "customer-service", []byte("secret"), body, time.Now()); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, wrong)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("wrong caller status=%d body=%s", rec.Code, rec.Body.String())
	}
	signed := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	signed.Header.Set(serviceauth.HeaderNonce, "fixed-campaign-event-nonce")
	if err := serviceauth.SignRequest(signed, "notification-service", []byte("secret"), body, time.Now()); err != nil {
		t.Fatal(err)
	}
	headers := signed.Header.Clone()
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, signed)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("valid signed status=%d body=%s", rec.Code, rec.Body.String())
	}
	replay := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	replay.Header = headers
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, replay)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("replay status=%d", rec.Code)
	}
}
