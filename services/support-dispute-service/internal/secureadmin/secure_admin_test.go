// Package secureadmin_test certifies Support's Section 16 Secure Admin owner
// boundary end to end against real PostgreSQL: signed owner contexts, native
// staff grants, the command state machine, HQ approvals, event emission,
// resource projections and the backfill snapshot.
package secureadmin_test

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/support-dispute-service/internal/features/admincommands"
	"cosmicforge/logistics/services/support-dispute-service/internal/features/adminresources"
	supportclients "cosmicforge/logistics/services/support-dispute-service/internal/features/support/clients"
	supportmodels "cosmicforge/logistics/services/support-dispute-service/internal/features/support/models"
	supportrepositories "cosmicforge/logistics/services/support-dispute-service/internal/features/support/repositories"
	supportusecases "cosmicforge/logistics/services/support-dispute-service/internal/features/support/usecases"
	"cosmicforge/logistics/services/support-dispute-service/internal/testdb"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminevents"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const purpose = "case-1"

var coordinate = regexp.MustCompile(`(6\.524412|3\.379205)[^0-9Z]`)

type fakeMedia struct{ usable map[string]bool }

func (m fakeMedia) Validate(_ context.Context, objectID, bindingType, _ string) (supportclients.EvidenceVerdict, error) {
	if bindingType != "COMPLAINT" {
		return supportclients.EvidenceVerdict{}, errors.New("unexpected binding")
	}
	return supportclients.EvidenceVerdict{Usable: m.usable[objectID]}, nil
}

type harness struct {
	t       *testing.T
	ctx     context.Context
	pool    *pgxpool.Pool
	support *supportusecases.SupportService
	spec    admincommand.Spec
	scope   adminpolicy.Scope
	issuer  *contextfixture.Fixture
	store   *admincontext.Store
	service *admincommand.Service
	staff   string
}

func newHarness(t *testing.T, media admincommands.EvidenceValidator) *harness {
	pool := testdb.Fresh(t)
	h := &harness{t: t, ctx: context.Background(), pool: pool, spec: admincommands.Spec(), staff: "support-staff-" + uuid.NewString()}
	h.support = supportusecases.NewSupportService(supportrepositories.NewPostgresSupportRepository(pool), supportusecases.Options{})
	h.scope = adminpolicy.Scope{Product: "karryam", Market: "TEST-MARKET", Zone: "TEST-ZONE", Organization: "TEST-ORGANIZATION", Tenant: "TEST-TENANT"}
	policy := policyfixture.New(t, h.scope, h.spec.Owner, h.spec.Capability, h.spec.Contract)
	policy.Document.ID = "section16-" + uuid.NewString()
	profile := adminowner.SupportProfile()
	extra := []string{h.spec.ReadCapability}
	for _, d := range profile.Definitions {
		extra = append(extra, d.Capability)
	}
	for _, capability := range extra {
		rule := policy.Document.Rules[0]
		rule.Capability, rule.Permission = capability, capability
		if capability != h.spec.ReadCapability {
			rule.SourceContract = "support.admin-resources.v1"
		}
		policy.Document.Rules = append(policy.Document.Rules, rule)
	}
	policy.Sign(t)
	h.issuer = contextfixture.New(t, h.spec.Owner)
	h.store = &admincontext.Store{DB: pool, Authority: h.issuer.Authority, Reader: adminresources.Source()}
	h.service = &admincommand.Service{Spec: h.spec, DB: pool, Policy: policy.Engine, Contexts: h.store, Executor: admincommands.Executor{Media: media}, WriteEnabled: true, ReadEnabled: true}
	grants := []string{adminresources.PermissionCaseManage, adminresources.PermissionSOSManage, adminresources.PermissionDisputeDecide, adminresources.PermissionEvidenceAttach}
	for _, d := range profile.Definitions {
		grants = append(grants, d.Capability)
	}
	h.grant(h.staff, grants...)
	return h
}

func (h *harness) grant(staff string, permissions ...string) {
	for _, permission := range permissions {
		if _, err := h.pool.Exec(h.ctx, `INSERT INTO support_admin_staff_grants(id,staff_ref,permission,granted_by,approval_ref) VALUES($1,$2,$3,'test-owner','test-only-approval')`, uuid.NewString(), staff, permission); err != nil {
			h.t.Fatal(err)
		}
	}
}

// importContext imports a TEST_ONLY signed owner context for one subject.
func (h *harness) importContext(kind, ref string) {
	tx, err := h.pool.Begin(h.ctx)
	if err != nil {
		h.t.Fatal(err)
	}
	binding, err := adminresources.Source().Binding(h.ctx, tx, kind, ref)
	_ = tx.Rollback(h.ctx)
	if err != nil {
		h.t.Fatalf("binding %s/%s: %v", kind, ref, err)
	}
	approval := h.issuer.Approval("section16-"+uuid.NewString(), kind, ref, h.spec.Capability, binding, h.scope)
	approval.Capabilities = append(approval.Capabilities, h.spec.ReadCapability, adminowner.SupportProfile().Definitions[kind].Capability)
	if err := h.store.Import(h.ctx, h.issuer.Sign(h.t, approval), time.Now()); err != nil {
		h.t.Fatalf("import context %s: %v", kind, err)
	}
}

func (h *harness) actor(staff string) adminpolicy.Actor {
	return adminpolicy.Actor{Product: "karryam", Staff: staff, Roles: []string{"staff"}, Permissions: []string{h.spec.Capability, h.spec.ReadCapability}, Markets: []string{h.scope.Market}, Zones: []string{h.scope.Zone}, Organizations: []string{h.scope.Organization}, Tenants: []string{h.scope.Tenant}}
}

func (h *harness) version(table, id string) int64 {
	var v int64
	if err := h.pool.QueryRow(h.ctx, `SELECT admin_version FROM `+table+` WHERE id=$1`, id).Scan(&v); err != nil {
		h.t.Fatal(err)
	}
	return v
}

func (h *harness) command(kind, id, action string, version int64, params map[string]string) admincommand.Command {
	if params == nil {
		params = map[string]string{}
	}
	return admincommand.Command{ContractVersion: h.spec.Contract, OperationID: uuid.NewString(), SubjectKind: kind, SubjectID: id, Action: action, ExpectedVersion: version, IdempotencyKey: uuid.NewString(), ReasonCode: "CASE_REVIEW", Params: params, RequestedAt: time.Now().UTC().Add(-time.Second)}
}

func (h *harness) approve(c *admincommand.Command, approver string) {
	c.Approval = &admincommand.Approval{ApprovalID: uuid.NewString(), ApproverRef: approver, IntentDigest: c.IntentDigest(h.staff, purpose, h.scope), ApprovedAt: time.Now().UTC().Add(-time.Second)}
}

func (h *harness) exec(staff string, c admincommand.Command) (admincommand.Receipt, error) {
	return h.service.Execute(h.ctx, h.actor(staff), purpose, h.scope, c)
}

func (h *harness) mustExec(c admincommand.Command, wantState string) admincommand.Receipt {
	h.t.Helper()
	receipt, err := h.exec(h.staff, c)
	if err != nil {
		h.t.Fatalf("%s: %v", c.Action, err)
	}
	if receipt.ResultingState != wantState || receipt.RecordedVersion != c.ExpectedVersion+1 {
		h.t.Fatalf("%s receipt state=%s version=%d", c.Action, receipt.ResultingState, receipt.RecordedVersion)
	}
	return receipt
}

func (h *harness) count(query string, args ...any) int {
	var n int
	if err := h.pool.QueryRow(h.ctx, query, args...).Scan(&n); err != nil {
		h.t.Fatal(err)
	}
	return n
}

// events returns every outbox event for one case, validated against the
// compiled adapter, ordered by entity version.
func (h *harness) events(caseID string) ([]adminevents.SourceEvent, []string) {
	rows, err := h.pool.Query(h.ctx, `SELECT payload::text FROM admin_event_outbox WHERE aggregate_id=$1 ORDER BY (payload->>'entity_version')::bigint`, caseID)
	if err != nil {
		h.t.Fatal(err)
	}
	defer rows.Close()
	adapter, ok := adminevents.Lookup("support-dispute-service", adminevents.SupportCaseChanged)
	if !ok {
		h.t.Fatal("support adapter not registered")
	}
	var out []adminevents.SourceEvent
	var raws []string
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			h.t.Fatal(err)
		}
		ev, err := adminevents.Decode([]byte(raw))
		if err != nil {
			h.t.Fatalf("event does not decode: %v %s", err, raw)
		}
		minimized, err := adapter.Minimize(ev)
		if err != nil {
			h.t.Fatalf("event fails adapter: %v %s", err, raw)
		}
		if len(minimized) != len(adapter.Fields) || len(ev.Payload) != len(adapter.Fields) {
			h.t.Fatalf("payload fields %d want exactly %d", len(ev.Payload), len(adapter.Fields))
		}
		out = append(out, ev)
		raws = append(raws, raw)
	}
	return out, raws
}

func payloadString(t *testing.T, ev adminevents.SourceEvent, key string) string {
	var value string
	if err := json.Unmarshal(ev.Payload[key], &value); err != nil {
		t.Fatalf("payload %s: %v", key, err)
	}
	return value
}

func TestSupportSecureAdminCaseStateMachine(t *testing.T) {
	media := fakeMedia{usable: map[string]bool{}}
	h := newHarness(t, media)
	ctx := h.ctx
	owner := "cust-" + uuid.NewString()
	name, phone := "Ada Okafor Secret", "+2348011112222"
	complaint, err := h.support.CreateComplaint(ctx, supportusecases.CreateComplaintInput{ComplainantType: supportmodels.ComplainantCustomer, ComplainantID: owner, ServiceType: supportmodels.ServiceTypeTaxi, Subject: "Driver used code 4821 wrongly", Description: "Private narrative text"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.pool.Exec(ctx, `UPDATE complaints SET complainant_name=$2, complainant_phone=$3 WHERE id=$1`, complaint.ID, name, phone); err != nil {
		t.Fatal(err)
	}
	id := complaint.ID
	h.importContext("cases", id)

	// REPLY: one message whether the identical command is retried or the
	// same client message id is reused under a fresh command.
	clientMessage := uuid.NewString()
	reply := h.command("cases", id, "REPLY_TO_CASE", h.version("complaints", id), map[string]string{"message": "We are looking into it.", "client_message_id": clientMessage})
	first := h.mustExec(reply, "OPEN")
	again, err := h.exec(h.staff, reply)
	if err != nil || again.DecisionID != first.DecisionID {
		t.Fatalf("idempotent replay: %+v %v", again, err)
	}
	reuse := h.command("cases", id, "REPLY_TO_CASE", h.version("complaints", id), map[string]string{"message": "Duplicate", "client_message_id": clientMessage})
	if _, err := h.exec(h.staff, reuse); !errors.Is(err, admincommand.ErrConflict) {
		t.Fatalf("reused client message id: %v", err)
	}
	if n := h.count(`SELECT count(*) FROM support_chat_messages WHERE complaint_id=$1`, id); n != 1 {
		t.Fatalf("messages=%d want 1", n)
	}
	if n := h.count(`SELECT count(*) FROM support_chat_messages WHERE complaint_id=$1 AND sender_type='admin' AND sender_id=$2 AND client_message_id=$3`, id, h.staff, clientMessage); n != 1 {
		t.Fatal("reply not stored as the staff member's admin message")
	}
	if n := h.count(`SELECT count(*) FROM support_admin_reply_deliveries WHERE complaint_id=$1 AND state='PENDING'`, id); n != 1 {
		t.Fatalf("pending deliveries=%d", n)
	}

	// Stale expected version.
	stale := h.command("cases", id, "ASSIGN_CASE", 1, map[string]string{"assignee_ref": "agent-7"})
	if _, err := h.exec(h.staff, stale); !errors.Is(err, admincommand.ErrConflict) {
		t.Fatalf("stale version: %v", err)
	}
	// Missing native mutation grant (read grant alone is not enough).
	reader := "support-reader-" + uuid.NewString()
	h.grant(reader, "support.cases.read")
	denied := h.command("cases", id, "ASSIGN_CASE", h.version("complaints", id), map[string]string{"assignee_ref": "agent-7"})
	if _, err := h.exec(reader, denied); !errors.Is(err, adminpolicy.ErrDenied) {
		t.Fatalf("missing grant: %v", err)
	}
	// Illegal transition: OPEN cannot jump to AWAITING_EVIDENCE.
	illegal := h.command("cases", id, "TRANSITION_CASE", h.version("complaints", id), map[string]string{"status": "AWAITING_EVIDENCE"})
	if _, err := h.exec(h.staff, illegal); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatalf("illegal transition: %v", err)
	}

	h.mustExec(h.command("cases", id, "TRANSITION_CASE", h.version("complaints", id), map[string]string{"status": "UNDER_REVIEW"}), "UNDER_REVIEW")
	h.mustExec(h.command("cases", id, "TRANSITION_CASE", h.version("complaints", id), map[string]string{"status": "AWAITING_EVIDENCE"}), "AWAITING_EVIDENCE")
	h.mustExec(h.command("cases", id, "ASSIGN_CASE", h.version("complaints", id), map[string]string{"assignee_ref": "agent-7"}), "AWAITING_EVIDENCE")
	noop := h.command("cases", id, "ASSIGN_CASE", h.version("complaints", id), map[string]string{"assignee_ref": "agent-7"})
	if _, err := h.exec(h.staff, noop); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatalf("no-op assignment: %v", err)
	}
	h.mustExec(h.command("cases", id, "ESCALATE_CASE", h.version("complaints", id), map[string]string{"escalation": "SAFETY_REVIEW"}), "ESCALATED")
	h.mustExec(h.command("cases", id, "TRANSITION_CASE", h.version("complaints", id), map[string]string{"status": "UNDER_REVIEW"}), "UNDER_REVIEW")

	// Evidence: Media decides usability.
	good, bad := uuid.NewString(), uuid.NewString()
	media.usable[good] = true
	unusable := h.command("cases", id, "ATTACH_EVIDENCE", h.version("complaints", id), map[string]string{"media_object_id": bad})
	if _, err := h.exec(h.staff, unusable); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatalf("unusable evidence: %v", err)
	}
	h.mustExec(h.command("cases", id, "ATTACH_EVIDENCE", h.version("complaints", id), map[string]string{"media_object_id": good}), "UNDER_REVIEW")

	h.mustExec(h.command("cases", id, "CLOSE_CASE", h.version("complaints", id), map[string]string{"closure": "RESOLVED"}), "RESOLVED")
	closed := h.command("cases", id, "REPLY_TO_CASE", h.version("complaints", id), map[string]string{"message": "late", "client_message_id": uuid.NewString()})
	if _, err := h.exec(h.staff, closed); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatalf("reply on closed case: %v", err)
	}
	reopen := h.command("cases", id, "TRANSITION_CASE", h.version("complaints", id), map[string]string{"status": "OPEN"})
	if _, err := h.exec(h.staff, reopen); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatalf("terminal exit: %v", err)
	}

	// Every applied action left a Support audit row by the staff member.
	if n := h.count(`SELECT count(*) FROM complaint_events WHERE complaint_id=$1 AND actor_type='admin' AND actor_id=$2`, id, h.staff); n != 8 {
		t.Fatalf("admin audit rows=%d want 8", n)
	}
	if n := h.count(`SELECT count(*) FROM admin_owner_command_decisions WHERE subject_id=$1`, id); n != 8 {
		t.Fatalf("decisions=%d want 8", n)
	}

	// Events: strictly increasing versions, each adapter-valid, no PII.
	events, raws := h.events(id)
	if len(events) < 9 {
		t.Fatalf("events=%d", len(events))
	}
	for i, ev := range events {
		if i > 0 && ev.EntityVersion <= events[i-1].EntityVersion {
			t.Fatalf("event versions not strictly increasing: %d then %d", events[i-1].EntityVersion, ev.EntityVersion)
		}
		for _, secret := range []string{name, phone, "Driver used", "code 4821", "Private narrative", "We are looking"} {
			if strings.Contains(raws[i], secret) {
				t.Fatalf("event leaks %q", secret)
			}
		}
	}
	last := events[len(events)-1]
	if last.EntityVersion != h.version("complaints", id) || payloadString(t, last, "status") != "RESOLVED" || payloadString(t, last, "consent_classification") != "OPERATIONS_MINIMIZED" || payloadString(t, last, "assignee") != "agent-7" || payloadString(t, last, "sla_state") != "POLICY_UNAVAILABLE" || string(last.Payload["locale"]) != "null" {
		t.Fatalf("final event: %s", raws[len(raws)-1])
	}
	if string(last.Payload["message_count"]) != "1" || string(last.Payload["evidence_count"]) != "1" {
		t.Fatalf("counts: %s", raws[len(raws)-1])
	}

	// Resource projections for the case, its message and its evidence.
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	resource, err := adminresources.Source().Read(ctx, tx, "cases", id, true)
	if err != nil || adminowner.ValidateResource(resource, adminowner.SupportProfile().Definitions["cases"], true) != nil {
		t.Fatalf("case resource: %+v %v", resource, err)
	}
	if resource.Label == nil || *resource.Label != "Driver used code [REDACTED] wrongly" || *resource.Status != "resolved" || *resource.SourceVersion != h.version("complaints", id) || len(resource.Timeline) == 0 {
		t.Fatalf("case projection: %+v", resource)
	}
	var messageID, evidenceID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM support_chat_messages WHERE complaint_id=$1`, id).Scan(&messageID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT id::text FROM complaint_evidence WHERE complaint_id=$1`, id).Scan(&evidenceID); err != nil {
		t.Fatal(err)
	}
	message, err := adminresources.Source().Read(ctx, tx, "case-messages", messageID, true)
	if err != nil || adminowner.ValidateResource(message, adminowner.SupportProfile().Definitions["case-messages"], true) != nil || message.Label == nil || *message.Label != "We are looking into it." {
		t.Fatalf("message projection: %+v %v", message, err)
	}
	if *message.Counters[0].Value != 1 || *message.Dimensions[0].Value != "admin" {
		t.Fatalf("message counters/dimensions: %+v", message)
	}
	evidence, err := adminresources.Source().Read(ctx, tx, "case-evidence", evidenceID, true)
	if err != nil || adminowner.ValidateResource(evidence, adminowner.SupportProfile().Definitions["case-evidence"], true) != nil {
		t.Fatalf("evidence projection: %+v %v", evidence, err)
	}
	encoded, _ := json.Marshal(evidence)
	if !strings.Contains(string(encoded), good) || strings.Contains(string(encoded), "http") {
		t.Fatalf("evidence must expose only the media object ref: %s", encoded)
	}
	all, _ := json.Marshal([]adminowner.Resource{resource, message, evidence})
	for _, secret := range []string{name, phone, "Private narrative"} {
		if strings.Contains(string(all), secret) {
			t.Fatalf("projection leaks %q", secret)
		}
	}
}

type flakyNotifier struct {
	mu    sync.Mutex
	fails int
	sent  []string
}

func (n *flakyNotifier) DeliverNewMessage(_ context.Context, _ supportmodels.Complaint, msg supportmodels.ChatMessage) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.fails > 0 {
		n.fails--
		return errors.New("notification outage")
	}
	n.sent = append(n.sent, msg.ID)
	return nil
}

func TestSupportAdminReplyDeliveryIsSeparateFromTheMessage(t *testing.T) {
	h := newHarness(t, nil)
	complaint, err := h.support.CreateComplaint(h.ctx, supportusecases.CreateComplaintInput{ComplainantType: supportmodels.ComplainantCustomer, ComplainantID: "cust-" + uuid.NewString(), ServiceType: supportmodels.ServiceTypeTaxi, Subject: "s", Description: "d"})
	if err != nil {
		t.Fatal(err)
	}
	h.importContext("cases", complaint.ID)
	h.mustExec(h.command("cases", complaint.ID, "REPLY_TO_CASE", 1, map[string]string{"message": "hello", "client_message_id": uuid.NewString()}), "OPEN")
	notifier := &flakyNotifier{fails: 1}
	delivery := &admincommands.ReplyDelivery{DB: h.pool, Notifier: notifier}
	if sent, err := delivery.RunOnce(h.ctx, 10); err != nil || sent != 0 {
		t.Fatalf("first pass sent=%d err=%v", sent, err)
	}
	// The outage changed nothing about the persisted reply.
	if n := h.count(`SELECT count(*) FROM support_chat_messages WHERE complaint_id=$1`, complaint.ID); n != 1 {
		t.Fatal("delivery failure touched the message")
	}
	if n := h.count(`SELECT count(*) FROM support_admin_reply_deliveries WHERE complaint_id=$1 AND state='PENDING' AND attempts=1 AND next_attempt_at>now()`, complaint.ID); n != 1 {
		t.Fatal("failed attempt not scheduled for retry")
	}
	if _, err := h.pool.Exec(h.ctx, `UPDATE support_admin_reply_deliveries SET next_attempt_at=now()`); err != nil {
		t.Fatal(err)
	}
	if sent, err := delivery.RunOnce(h.ctx, 10); err != nil || sent != 1 || len(notifier.sent) != 1 {
		t.Fatalf("retry sent=%d err=%v", sent, err)
	}
	if n := h.count(`SELECT count(*) FROM support_admin_reply_deliveries WHERE complaint_id=$1 AND state='SENT' AND delivered_at IS NOT NULL`, complaint.ID); n != 1 {
		t.Fatal("delivery state not recorded")
	}
	if sent, _ := delivery.RunOnce(h.ctx, 10); sent != 0 {
		t.Fatal("a delivered reply was sent twice")
	}
}

func TestSupportChatSequencesAreContiguousUnderConcurrency(t *testing.T) {
	h := newHarness(t, nil)
	owner := "cust-" + uuid.NewString()
	complaint, err := h.support.CreateComplaint(h.ctx, supportusecases.CreateComplaintInput{ComplainantType: supportmodels.ComplainantCustomer, ComplainantID: owner, ServiceType: supportmodels.ServiceTypeTaxi, Subject: "s", Description: "d"})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := h.support.SendChatMessage(h.ctx, complaint.ID, supportmodels.SenderTypeCustomer, owner, "concurrent")
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
	var sequences string
	if err := h.pool.QueryRow(h.ctx, `SELECT string_agg(sequence::text, ',' ORDER BY sequence) FROM support_chat_messages WHERE complaint_id=$1`, complaint.ID).Scan(&sequences); err != nil {
		t.Fatal(err)
	}
	if sequences != "1,2,3,4,5,6,7,8,9,10" {
		t.Fatalf("sequences=%s", sequences)
	}
	messages, err := h.support.ListChatMessages(h.ctx, complaint.ID, supportmodels.SenderTypeCustomer, owner, 50, 0)
	if err != nil || len(messages) != 10 {
		t.Fatalf("list=%d err=%v", len(messages), err)
	}
	// Each committed message bumped the case; versions stay strictly ordered.
	events, _ := h.events(complaint.ID)
	for i := 1; i < len(events); i++ {
		if events[i].EntityVersion <= events[i-1].EntityVersion {
			t.Fatal("event versions regressed")
		}
	}
	if got := events[len(events)-1]; string(got.Payload["message_count"]) != "10" || string(got.Payload["unread_count"]) != "10" {
		t.Fatalf("final counts message=%s unread=%s", got.Payload["message_count"], got.Payload["unread_count"])
	}
}

func TestSupportSOSIsSafetyEvidenceAndEscalatable(t *testing.T) {
	h := newHarness(t, nil)
	lat, lng := 6.524412, 3.379205
	sos, err := h.support.CreateSOS(h.ctx, supportusecases.SOSInput{ComplainantType: supportmodels.ComplainantCustomer, ComplainantID: "cust-" + uuid.NewString(), ServiceType: supportmodels.ServiceTypeTaxi, Description: "Unsafe driver, call 08012345678", Lat: &lat, Lng: &lng})
	if err != nil {
		t.Fatal(err)
	}
	normal, err := h.support.CreateComplaint(h.ctx, supportusecases.CreateComplaintInput{ComplainantType: supportmodels.ComplainantCustomer, ComplainantID: "cust-" + uuid.NewString(), ServiceType: supportmodels.ServiceTypeTaxi, Subject: "s", Description: "d"})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := h.pool.Begin(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	item, err := adminresources.Source().Read(h.ctx, tx, "sos-cases", sos.ID, true)
	if err != nil || adminowner.ValidateResource(item, adminowner.SupportProfile().Definitions["sos-cases"], true) != nil || item.Label != nil {
		t.Fatalf("sos projection: %+v %v", item, err)
	}
	if _, err := adminresources.Source().Read(h.ctx, tx, "sos-cases", normal.ID, false); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("non-emergency case visible as SOS: %v", err)
	}
	_ = tx.Rollback(h.ctx)

	events, raws := h.events(sos.ID)
	if len(events) == 0 {
		t.Fatal("no SOS event")
	}
	for i, ev := range events {
		if payloadString(t, ev, "consent_classification") != "SAFETY_EVIDENCE" || payloadString(t, ev, "priority") != "EMERGENCY" {
			t.Fatalf("SOS classification: %s", raws[i])
		}
		for _, secret := range []string{"08012345678", "Unsafe driver", "incident", "\"lng\"", "\"lat\""} {
			if strings.Contains(raws[i], secret) {
				t.Fatalf("SOS event leaks %q", secret)
			}
		}
		// Coordinates would be bare JSON numbers; timestamps end in Z.
		if coordinate.MatchString(raws[i]) {
			t.Fatal("SOS event leaks incident coordinates")
		}
	}
	h.importContext("sos-cases", sos.ID)
	h.mustExec(h.command("sos-cases", sos.ID, "ESCALATE_SOS", h.version("complaints", sos.ID), map[string]string{"level": "SAFETY_LEAD"}), "ESCALATED")
	repeat := h.command("sos-cases", sos.ID, "ESCALATE_SOS", h.version("complaints", sos.ID), map[string]string{"level": "SAFETY_LEAD"})
	if _, err := h.exec(h.staff, repeat); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatalf("repeated SOS escalation: %v", err)
	}
	// Without the SOS grant a case manager cannot escalate an SOS.
	manager := "support-manager-" + uuid.NewString()
	h.grant(manager, "support.sos.read", adminresources.PermissionCaseManage)
	other := h.command("sos-cases", sos.ID, "ESCALATE_SOS", h.version("complaints", sos.ID), map[string]string{"level": "EMERGENCY_LIAISON"})
	if _, err := h.exec(manager, other); !errors.Is(err, adminpolicy.ErrDenied) {
		t.Fatalf("SOS escalation without grant: %v", err)
	}
	final, _ := h.events(sos.ID)
	if payloadString(t, final[len(final)-1], "escalation_level") != "SAFETY_LEAD" {
		t.Fatal("escalation not in the event")
	}
}

func TestSupportDisputeResolutionAndRemedy(t *testing.T) {
	h := newHarness(t, nil)
	dispute := func() supportmodels.Dispute {
		owner := "cust-" + uuid.NewString()
		complaint, err := h.support.CreateComplaint(h.ctx, supportusecases.CreateComplaintInput{ComplainantType: supportmodels.ComplainantCustomer, ComplainantID: owner, ServiceType: supportmodels.ServiceTypeTaxi, Subject: "s", Description: "d"})
		if err != nil {
			t.Fatal(err)
		}
		d, err := h.support.EscalateToDispute(h.ctx, supportusecases.EscalateToDisputeInput{ComplaintID: complaint.ID, RequesterType: supportmodels.ComplainantCustomer, RequesterID: owner, RespondentType: supportmodels.ComplainantTaxiProvider, RespondentID: "driver-" + uuid.NewString()})
		if err != nil {
			t.Fatal(err)
		}
		h.importContext("disputes", d.ID)
		return d
	}
	respondent, complainant := dispute(), dispute()

	resolve := h.command("disputes", respondent.ID, "RESOLVE_DISPUTE", h.version("disputes", respondent.ID), map[string]string{"outcome": "FAVOUR_RESPONDENT"})
	if _, err := h.exec(h.staff, resolve); err == nil {
		t.Fatal("dispute resolved without HQ approval")
	}
	self := resolve
	h.approve(&self, h.staff)
	if _, err := h.exec(h.staff, self); err == nil {
		t.Fatal("self approval accepted")
	}
	h.approve(&resolve, "independent-support-reviewer")
	h.mustExec(resolve, "FAVOUR_RESPONDENT")

	remedy := h.command("disputes", respondent.ID, "RECORD_DISPUTE_REMEDY", h.version("disputes", respondent.ID), map[string]string{"wallet_operation_id": uuid.NewString(), "amount_minor": "250000"})
	h.approve(&remedy, "independent-support-reviewer")
	if _, err := h.exec(h.staff, remedy); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatalf("remedy for FAVOUR_RESPONDENT: %v", err)
	}
	if n := h.count(`SELECT count(*) FROM disputes WHERE id=$1 AND remedy_state='NONE'`, respondent.ID); n != 1 {
		t.Fatal("refused remedy changed the dispute")
	}

	won := h.command("disputes", complainant.ID, "RESOLVE_DISPUTE", h.version("disputes", complainant.ID), map[string]string{"outcome": "FAVOUR_COMPLAINANT"})
	h.approve(&won, "independent-support-reviewer")
	h.mustExec(won, "FAVOUR_COMPLAINANT")
	again := h.command("disputes", complainant.ID, "RESOLVE_DISPUTE", h.version("disputes", complainant.ID), map[string]string{"outcome": "SPLIT"})
	h.approve(&again, "independent-support-reviewer")
	if _, err := h.exec(h.staff, again); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatalf("second resolution: %v", err)
	}
	operation := uuid.NewString()
	record := h.command("disputes", complainant.ID, "RECORD_DISPUTE_REMEDY", h.version("disputes", complainant.ID), map[string]string{"wallet_operation_id": operation, "amount_minor": "250000"})
	h.approve(&record, "independent-support-reviewer")
	h.mustExec(record, "FAVOUR_COMPLAINANT")
	twice := h.command("disputes", complainant.ID, "RECORD_DISPUTE_REMEDY", h.version("disputes", complainant.ID), map[string]string{"wallet_operation_id": uuid.NewString(), "amount_minor": "1"})
	h.approve(&twice, "independent-support-reviewer")
	if _, err := h.exec(h.staff, twice); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatalf("remedy recorded twice: %v", err)
	}
	if n := h.count(`SELECT count(*) FROM disputes WHERE id=$1 AND remedy_state='RECORDED' AND remedy_wallet_operation_id=$2 AND remedy_amount_minor=250000`, complainant.ID, operation); n != 1 {
		t.Fatal("remedy not recorded exactly once")
	}

	tx, err := h.pool.Begin(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(h.ctx)
	item, err := adminresources.Source().Read(h.ctx, tx, "disputes", complainant.ID, true)
	if err != nil || adminowner.ValidateResource(item, adminowner.SupportProfile().Definitions["disputes"], true) != nil {
		t.Fatalf("dispute projection: %+v %v", item, err)
	}
	refs := map[string]string{}
	for _, r := range item.References {
		refs[r.Kind] = r.Reference
	}
	if *item.Status != "favour_complainant" || refs["remedy_operation"] != operation || refs["case"] != complainant.ComplaintID || *item.Dimensions[2].Value != "RECORDED" {
		t.Fatalf("dispute projection content: %+v", item)
	}
	// The case event carries the dispute reference.
	events, _ := h.events(complainant.ComplaintID)
	if payloadString(t, events[len(events)-1], "dispute_ref") != complainant.ID {
		t.Fatal("case event lacks dispute reference")
	}
}
