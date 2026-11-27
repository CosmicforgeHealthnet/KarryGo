package integration_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	engagementmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/models"
	engagementrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/repositories"
	engagementusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// fakeSupportMessageAuthority stands in for support-dispute-service's organization
// message contract: one stored message per (case, client message id). fail
// makes the next CreateMessage fail with the given outcome; storeThenFail
// stores the message but still reports an unknown outcome (a lost response).
type fakeSupportMessageAuthority struct {
	mu            sync.Mutex
	messages      map[string]engagementmodels.SupportMessage
	creates       int
	fail          platformclients.Outcome
	storeThenFail bool
	readFails     bool
	// replyDelay holds the reply after storing, so concurrent readers find
	// the message and race the creator to confirm it.
	replyDelay time.Duration
}

func (f *fakeSupportMessageAuthority) Configured() bool { return true }
func (f *fakeSupportMessageAuthority) key(caseID, clientID string) string {
	return caseID + "|" + clientID
}
func (f *fakeSupportMessageAuthority) CreateMessage(_ context.Context, r engagementmodels.SupportMessageRequest) (engagementmodels.SupportMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creates++
	if f.fail != "" && !f.storeThenFail {
		out := f.fail
		f.fail = ""
		return engagementmodels.SupportMessage{}, &platformclients.Error{Service: "support-dispute-service", Outcome: out}
	}
	k := f.key(r.SupportCaseID, r.ClientMessageID)
	m, ok := f.messages[k]
	if !ok {
		m = engagementmodels.SupportMessage{ID: ids.New("sdm"), ClientMessageID: r.ClientMessageID, Text: r.Text, Visibility: r.Visibility, FileIDs: r.FileIDs, CreatedAt: time.Now().UTC()}
		f.messages[k] = m
	}
	if f.storeThenFail {
		f.storeThenFail, f.fail = false, ""
		return engagementmodels.SupportMessage{}, &platformclients.Error{Service: "support-dispute-service", Outcome: platformclients.OutcomeUnknown}
	}
	if d := f.replyDelay; d > 0 {
		f.mu.Unlock()
		time.Sleep(d)
		f.mu.Lock()
	}
	return m, nil
}
func (f *fakeSupportMessageAuthority) ReadMessage(_ context.Context, _, caseID, clientID string) (engagementmodels.SupportMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.readFails {
		return engagementmodels.SupportMessage{}, &platformclients.Error{Service: "support-dispute-service", Outcome: platformclients.OutcomeTransient}
	}
	m, ok := f.messages[f.key(caseID, clientID)]
	if !ok {
		return engagementmodels.SupportMessage{}, &platformclients.Error{Service: "support-dispute-service", Outcome: platformclients.OutcomeNotFound, Status: 404}
	}
	return m, nil
}

// fakeSupportCaseReads satisfies the case SupportPort the reconcile job checks.
type fakeSupportCaseReads struct{}

func (fakeSupportCaseReads) Configured() bool { return true }
func (fakeSupportCaseReads) Create(context.Context, string, string, engagementmodels.SupportCreateInput) (engagementmodels.SupportProjection, error) {
	return engagementmodels.SupportProjection{}, nil
}
func (fakeSupportCaseReads) Read(context.Context, string, string) (engagementmodels.SupportProjection, error) {
	return engagementmodels.SupportProjection{}, nil
}

type supportMessageMedia struct{ org string }

func (supportMessageMedia) Configured() bool { return true }
func (m supportMessageMedia) GetFile(_ context.Context, _ platformclients.CallMeta, id string) (platformclients.FileMetadata, error) {
	switch id {
	case "fil_ready":
		return platformclients.FileMetadata{FileID: id, OwnerID: m.org, UploadState: "COMPLETED", ScanState: "CLEAN"}, nil
	case "fil_unscanned":
		return platformclients.FileMetadata{FileID: id, OwnerID: m.org, UploadState: "COMPLETED", ScanState: "PENDING"}, nil
	case "fil_foreign":
		return platformclients.FileMetadata{FileID: id, OwnerID: "org_someone_else", UploadState: "COMPLETED", ScanState: "CLEAN"}, nil
	}
	return platformclients.FileMetadata{}, &platformclients.Error{Service: "media-file-service", Outcome: platformclients.OutcomeNotFound}
}

// TestSupportCaseMessagesSubmitOnceAndReconcile proves POST
// /support-cases/{id}/messages (spec 59.10): only the organization's own
// case, owned and clean files, a client message id posted at most once
// (with the same body; a different body is refused), definite refusals
// recorded without a message, an unknown outcome resolved by reading the
// authority back rather than resending, the reconcile job confirming a
// message that did land and closing one that never arrived, and
// support_message.created announced exactly once per message.
func TestSupportCaseMessagesSubmitOnceAndReconcile(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	scope, _, _ := section2223Scope(t, pool)
	scope.PermittedBranchIDs = nil
	other, _, _ := section2223Scope(t, pool)
	upstreamCase := ids.New("sdc")
	projection := ids.New("scp")
	if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_support_case_projections(id,organization_id,support_service_case_id,case_type,status,priority,source_version,last_synced_at) VALUES($1,$2,$3,'OPERATIONS','open','normal',1,now())`, projection, scope.OrganizationID, upstreamCase); err != nil {
		t.Fatal(err)
	}
	authority := &fakeSupportMessageAuthority{messages: map[string]engagementmodels.SupportMessage{}}
	clock := time.Now()
	repo := engagementrepos.NewRepository(pool)
	queue := jobs.NewQueue(pool)
	svc := engagementusecases.NewService(engagementusecases.Options{Store: repo, Events: events.NewDomainPublisher(events.NewOutboxRepository(pool)), Queue: queue,
		Support: fakeSupportCaseReads{}, SupportMessages: authority, MessageStore: repo, Media: supportMessageMedia{org: scope.OrganizationID},
		Flags: engagementmodels.Flags{Support: true, Workers: true}, Now: func() time.Time { return clock }})
	worker := &jobs.Worker{Queue: queue, Metrics: observability.NewMetrics(), Owner: "support-message-worker", PollInterval: 20 * time.Millisecond, LeaseDuration: 5 * time.Second, HeartbeatInterval: time.Second}
	svc.RegisterWorkers(worker)
	h := &s24{t: t, pool: pool, queue: queue, worker: worker, org: scope.OrganizationID}

	msg := func(clientID, text string, files ...string) engagementmodels.SupportMessageInput {
		return engagementmodels.SupportMessageInput{ClientMessageID: clientID, Text: text, FileIDs: files}
	}
	created := func(ledgerID string) int {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_outbox WHERE event_type='support_message.created.v1' AND aggregate_id=$1`, ledgerID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// Refusals before anything is sent.
	for name, tc := range map[string]struct {
		caseID string
		in     engagementmodels.SupportMessageInput
		code   string
	}{
		"bad client id":     {projection, msg("nope", "Hello"), "validation_failed"},
		"empty text":        {projection, msg("1b2c3d4e-0000-4000-8000-000000000001", " "), "validation_failed"},
		"bad visibility":    {projection, engagementmodels.SupportMessageInput{ClientMessageID: "1b2c3d4e-0000-4000-8000-000000000001", Text: "Hi", Visibility: "PUBLIC"}, "validation_failed"},
		"unknown case":      {ids.New("scp"), msg("1b2c3d4e-0000-4000-8000-000000000001", "Hello"), "not_found"},
		"unscanned file":    {projection, msg("1b2c3d4e-0000-4000-8000-000000000001", "Hello", "fil_unscanned"), "forbidden"},
		"foreign file":      {projection, msg("1b2c3d4e-0000-4000-8000-000000000001", "Hello", "fil_foreign"), "forbidden"},
		"missing file":      {projection, msg("1b2c3d4e-0000-4000-8000-000000000001", "Hello", "fil_missing"), "forbidden"},
		"duplicate file id": {projection, msg("1b2c3d4e-0000-4000-8000-000000000001", "Hello", "fil_ready", "fil_ready"), "validation_failed"},
	} {
		if _, err := svc.PostSupportMessage(ctx, scope, tc.caseID, tc.in); appCode(err) != tc.code {
			t.Fatalf("%s: err = %v, want %s", name, err, tc.code)
		}
	}
	if _, err := svc.PostSupportMessage(ctx, other, projection, msg("1b2c3d4e-0000-4000-8000-000000000001", "Hello")); appCode(err) != "not_found" {
		t.Fatalf("another organization's case = %v, want not_found", err)
	}
	if authority.creates != 0 {
		t.Fatalf("refused requests reached the authority %d times", authority.creates)
	}

	// Happy path, by local projection id; then a replay by upstream id.
	first, err := svc.PostSupportMessage(ctx, scope, projection, msg("1b2c3d4e-0000-4000-8000-000000000001", "Please call the Ikeja branch.", "fil_ready"))
	if err != nil || first["status"] != "CONFIRMED" || first["text"] != "Please call the Ikeja branch." || first["visibility"] != "SHARED" {
		t.Fatalf("first = %v, %v", first, err)
	}
	again, err := svc.PostSupportMessage(ctx, scope, upstreamCase, msg("1b2c3d4e-0000-4000-8000-000000000001", "Please call the Ikeja branch.", "fil_ready"))
	if err != nil || again["id"] != first["id"] || again["supportMessageId"] != first["supportMessageId"] {
		t.Fatalf("replay = %v, %v; want the same message", again, err)
	}
	if _, err := svc.PostSupportMessage(ctx, scope, projection, msg("1b2c3d4e-0000-4000-8000-000000000001", "A different body")); appCode(err) != "conflict" {
		t.Fatalf("reused client id with a different body = %v, want conflict", err)
	}
	if authority.creates != 1 || created(first["id"].(string)) != 1 {
		t.Fatalf("creates = %d, events = %d; want 1 and 1", authority.creates, created(first["id"].(string)))
	}

	// Concurrent submissions of one new client message id: one message
	// upstream, one ledger row, one confirmation and one event.
	authority.replyDelay = 300 * time.Millisecond
	var wg sync.WaitGroup
	results := make(chan map[string]any, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v, err := svc.PostSupportMessage(ctx, scope, projection, msg("1b2c3d4e-0000-4000-8000-000000000009", "Concurrent")); err == nil {
				results <- v
			}
		}()
	}
	wg.Wait()
	close(results)
	authority.replyDelay = 0
	var concurrentID string
	for v := range results {
		if concurrentID != "" && v["id"] != concurrentID {
			t.Fatalf("concurrent submissions produced two ledger rows")
		}
		concurrentID, _ = v["id"].(string)
	}
	if concurrentID == "" || created(concurrentID) != 1 || len(authority.messages) != 2 {
		t.Fatalf("concurrent: id %q, events %d, upstream messages %d; want one event and two messages in total", concurrentID, created(concurrentID), len(authority.messages))
	}

	// A definite refusal records no message and allows a later resubmission.
	authority.fail = platformclients.OutcomeValidation
	if _, err := svc.PostSupportMessage(ctx, scope, projection, msg("1b2c3d4e-0000-4000-8000-000000000002", "Second")); appCode(err) != "validation_failed" {
		t.Fatalf("definite refusal = %v, want validation_failed", err)
	}
	retried, err := svc.PostSupportMessage(ctx, scope, projection, msg("1b2c3d4e-0000-4000-8000-000000000002", "Second"))
	if err != nil || retried["status"] != "CONFIRMED" {
		t.Fatalf("resubmission after a refusal = %v, %v", retried, err)
	}

	// Lost response: stored upstream, but the answer never came and the
	// read-back fails too. Fleet records OUTCOME_UNKNOWN and answers 503; the
	// client's retry reads back and confirms without a second create.
	authority.storeThenFail, authority.fail, authority.readFails = true, platformclients.OutcomeUnknown, true
	if _, err := svc.PostSupportMessage(ctx, scope, projection, msg("1b2c3d4e-0000-4000-8000-000000000003", "Third", "fil_ready")); appCode(err) != "service_unavailable" {
		t.Fatalf("unknown outcome = %v, want service_unavailable", err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM fleet_partner_support_case_messages WHERE organization_id=$1 AND client_message_id='1b2c3d4e-0000-4000-8000-000000000003'`, scope.OrganizationID).Scan(&status); err != nil || status != "OUTCOME_UNKNOWN" {
		t.Fatalf("ledger status = %q, %v; want OUTCOME_UNKNOWN", status, err)
	}
	authority.readFails = false
	createsBefore := authority.creates
	third, err := svc.PostSupportMessage(ctx, scope, projection, msg("1b2c3d4e-0000-4000-8000-000000000003", "Third", "fil_ready"))
	if err != nil || third["status"] != "CONFIRMED" || authority.creates != createsBefore {
		t.Fatalf("retry after unknown = %v, %v, creates %d -> %d; want confirmed by read-back alone", third, err, createsBefore, authority.creates)
	}
	if created(third["id"].(string)) != 1 {
		t.Fatalf("support_message.created events = %d, want 1", created(third["id"].(string)))
	}

	// The reconcile job confirms a lost-response message nobody retried.
	authority.storeThenFail, authority.fail, authority.readFails = true, platformclients.OutcomeUnknown, true
	if _, err := svc.PostSupportMessage(ctx, scope, projection, msg("1b2c3d4e-0000-4000-8000-000000000004", "Fourth")); appCode(err) != "service_unavailable" {
		t.Fatalf("unknown outcome = %v", err)
	}
	authority.readFails = false
	clock = clock.Add(2 * time.Minute)
	h.runJob("SECTION23_SUPPORT_RECONCILE", map[string]string{})
	var fourthID string
	if err := pool.QueryRow(ctx, `SELECT id,status FROM fleet_partner_support_case_messages WHERE organization_id=$1 AND client_message_id='1b2c3d4e-0000-4000-8000-000000000004'`, scope.OrganizationID).Scan(&fourthID, &status); err != nil || status != "CONFIRMED" || created(fourthID) != 1 {
		t.Fatalf("reconciled status = %q (%v), events %d; want CONFIRMED and 1", status, err, created(fourthID))
	}

	// A submission that never reached the authority is closed as not
	// received after the give-up window, never resent by Fleet.
	authority.fail, authority.readFails = platformclients.OutcomeUnknown, true
	if _, err := svc.PostSupportMessage(ctx, scope, projection, msg("1b2c3d4e-0000-4000-8000-000000000005", "Fifth")); appCode(err) != "service_unavailable" {
		t.Fatalf("unknown outcome = %v", err)
	}
	authority.readFails = false
	createsBefore = authority.creates
	clock = clock.Add(2 * time.Minute)
	h.runJob("SECTION23_SUPPORT_RECONCILE", map[string]string{})
	if err := pool.QueryRow(ctx, `SELECT status FROM fleet_partner_support_case_messages WHERE organization_id=$1 AND client_message_id='1b2c3d4e-0000-4000-8000-000000000005'`, scope.OrganizationID).Scan(&status); err != nil || status != "OUTCOME_UNKNOWN" {
		t.Fatalf("before the give-up window status = %q, %v; want OUTCOME_UNKNOWN", status, err)
	}
	clock = clock.Add(2 * time.Hour)
	if _, err := pool.Exec(ctx, `UPDATE fleet_partner_support_case_messages SET created_at=now()-interval '2 hours', updated_at=now()-interval '2 hours' WHERE organization_id=$1 AND client_message_id='1b2c3d4e-0000-4000-8000-000000000005'`, scope.OrganizationID); err != nil {
		t.Fatal(err)
	}
	h.runJob("SECTION23_SUPPORT_RECONCILE", map[string]string{})
	var code string
	if err := pool.QueryRow(ctx, `SELECT status,last_error_code FROM fleet_partner_support_case_messages WHERE organization_id=$1 AND client_message_id='1b2c3d4e-0000-4000-8000-000000000005'`, scope.OrganizationID).Scan(&status, &code); err != nil || status != "REJECTED" || code != "not_received" {
		t.Fatalf("after the give-up window = %q/%q, %v; want REJECTED/not_received", status, code, err)
	}
	if authority.creates != createsBefore {
		t.Fatalf("the reconcile job resent a message (%d -> %d creates)", createsBefore, authority.creates)
	}
}
