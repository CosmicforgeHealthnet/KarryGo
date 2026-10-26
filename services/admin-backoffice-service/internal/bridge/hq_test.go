package bridge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cosmicforge/logistics/services/admin-backoffice-service/internal/clients"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/hqevents"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/hqops"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/testutil"
	"cosmicforge/logistics/shared/go/admincampaign"
	"cosmicforge/logistics/shared/go/adminevents"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/financereport"
	"github.com/google/uuid"
)

type hqHarness struct {
	f      *fixture
	values string
	store  *hqevents.Store
	ops    *hqops.Service
	calls  atomic.Int32
	drop   atomic.Bool
	last   atomic.Value
}

func newHQHarness(t *testing.T, flags HQFlags) *hqHarness {
	db := testutil.Database(t)
	h := &hqHarness{}
	status := func(in admincampaign.Intent, state string) admincampaign.Status {
		var none *int64
		return admincampaign.Status{Contract: admincampaign.Contract, CampaignID: in.CampaignID, Revision: in.Revision, IntentDigest: in.Digest(), State: state, SuppressionReasons: map[string]int64{}, DeliveredCount: none, DeliveryConfirmation: "UNAVAILABLE", ConversionReporting: "UNAVAILABLE", AsOf: time.Now().UTC()}
	}
	owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.calls.Add(1)
		raw, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == "POST" && r.URL.Path == admincampaign.IntentPath:
			in, err := admincampaign.Decode(raw)
			if err != nil {
				w.WriteHeader(400)
				return
			}
			h.last.Store(in)
			if h.drop.Load() { // commit happened, response lost
				hj, _ := w.(http.Hijacker)
				conn, _, _ := hj.Hijack()
				conn.Close()
				return
			}
			json.NewEncoder(w).Encode(status(in, "ACCEPTED_BY_KARRYGO"))
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, admincampaign.IntentPath+"/"):
			in, ok := h.last.Load().(admincampaign.Intent)
			if !ok {
				w.WriteHeader(404)
				return
			}
			json.NewEncoder(w).Encode(status(in, "QUEUED"))
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/cancellations"):
			in := h.last.Load().(admincampaign.Intent)
			json.NewEncoder(w).Encode(status(in, "CANCELLED"))
		case r.URL.Path == financereport.Path:
			now := time.Now().UTC()
			amount := int64(150000)
			rep := financereport.Report{Contract: financereport.Contract, Owner: financereport.Owner, PeriodStart: now.Add(-time.Hour), PeriodEnd: now, Timezone: "Africa/Lagos", DataAsOf: now, SourceWatermark: now, AggregationVersion: "test", Unit: "MINOR_UNITS", Definitions: financereport.Measures(), Figures: []financereport.Figure{{Measure: "gross_transaction_value", Currency: "NGN", AmountMinor: &amount, Completeness: "COMPLETE"}}, Completeness: "COMPLETE"}
			json.NewEncoder(w).Encode(rep)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(owner.Close)
	campaigns, _ := clients.NewCampaigns(owner.URL, bytes.Repeat([]byte{3}, 32))
	finance, _ := clients.NewFinance(owner.URL, bytes.Repeat([]byte{4}, 32))
	h.store = &hqevents.Store{DB: db}
	h.ops = &hqops.Service{DB: db, Campaigns: campaigns, Finance: finance, CampaignReads: true, CampaignWrites: true, FinanceReads: true, PrivacyWrites: true, ExportKey: bytes.Repeat([]byte{5}, 32), ExportKeyID: "test-export-key"}
	scope := adminpolicy.Scope{Product: "karryam", Market: "NG", Zone: "Z1", Organization: "org-1", Tenant: "org-1"}
	policy := testfixture.New(t, scope, "admin-backoffice-service", "hq.events.feed", "karryam.admin-events.v1")
	owners := map[string]bool{}
	policy.Document.Rules = nil
	for id, route := range hqRoutes {
		rule := testfixture.New(t, scope, route.Owner, id, route.Contract).Document.Rules[0]
		policy.Document.Rules = append(policy.Document.Rules, rule)
		owners[route.Owner] = true
	}
	policy.Document.ApprovedOwners = nil
	for o := range owners {
		policy.Document.ApprovedOwners = append(policy.Document.ApprovedOwners, o)
	}
	policy.Sign(t)
	h.f = newFixture(t)
	h.f.claims.Zones, h.f.claims.Tenants = []string{"Z1"}, []string{"org-1"}
	h.f.claims.Permissions = nil
	for id := range hqRoutes {
		h.f.claims.Permissions = append(h.f.claims.Permissions, id)
	}
	h.f.b.features.RemoteWrites = true
	if err := h.f.b.ConfigureHQ(h.store, h.ops, policy.Engine, flags); err != nil {
		t.Fatal(err)
	}
	q := adminread.Query{Scope: adminread.Scope{Market: "NG", Zone: "Z1", Organization: "org-1", Tenant: "org-1"}, Purpose: "case-1", Start: time.Now().UTC().Add(-time.Hour).Truncate(time.Second), End: time.Now().UTC().Truncate(time.Second), Limit: 10}
	v := q.Values()
	for _, k := range []string{"staff", "role", "policy_id", "policy_version", "policy_checksum"} {
		v.Del(k)
	}
	h.values = v.Encode()
	return h
}

func (h *hqHarness) do(t *testing.T, staff, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	if body != nil {
		raw, _ = json.Marshal(body)
	}
	h.f.claims.Subject = staff
	rec := httptest.NewRecorder()
	h.f.b.ServeHTTP(rec, h.f.request(t, method, path+"?"+h.values, raw))
	return rec
}

func result(t *testing.T, rec *httptest.ResponseRecorder, out any) {
	t.Helper()
	var env struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil || json.Unmarshal(env.Result, out) != nil {
		t.Fatalf("bad envelope %d %s", rec.Code, rec.Body)
	}
}

func intentFor(t *testing.T) admincampaign.Intent {
	now := time.Now().UTC().Truncate(time.Second)
	sum := sha256.Sum256([]byte("x"))
	return admincampaign.Intent{ContractVersion: admincampaign.Contract, CampaignID: uuid.NewString(), Revision: 1, IdempotencyKey: "camp-" + uuid.NewString(), Product: "karryam", RequestingDepartment: "hq-marketing", Category: "MARKETING", Markets: []string{"NG"}, Zones: []string{}, Audience: "CUSTOMERS_OPTED_IN", AudienceDefinitionSHA256: hex.EncodeToString(sum[:]), LegalBasis: "CONSENT", Channels: []string{"in_app"}, TemplateKey: "promo.weekend", TemplateLocale: "en-NG", TemplateRevision: 1, ContentSHA256: hex.EncodeToString(sum[:]), StartAt: now, EndAt: now.Add(48 * time.Hour), MaxPerRecipientPerDay: 1, MaxMessages: 1000, Priority: "normal"}
}

func TestPostgresHQBridgeRoutesEndToEnd(t *testing.T) {
	ctx := context.Background()
	h := newHQHarness(t, HQFlags{Events: true, SupportQueue: true, Approvals: true, Campaigns: true, Finance: true, Privacy: true, Exports: true})

	// Events: an owner event lands in the feed and the support queue.
	now := time.Now().UTC()
	ev, _ := adminevents.NewSourceEvent("support-dispute-service", adminevents.SupportCaseChanged, "support_case", uuid.NewString(), 1, now, adminevents.Scope{}, "", map[string]any{"status": "OPEN", "priority": "EMERGENCY", "service_type": "taxi", "complainant_type": "customer", "sla_state": "POLICY_UNAVAILABLE", "consent_classification": "SAFETY_EVIDENCE", "unread_count": 1, "message_count": 1, "evidence_count": 0, "created_at": now, "updated_at": now}, false)
	raw, _ := json.Marshal(ev)
	if _, err := h.store.Ingest(ctx, "support-dispute-service", raw, hqevents.DeliveryIngress); err != nil {
		t.Fatal(err)
	}
	if rec := h.do(t, "staff-1", "GET", "/hq/events", nil); rec.Code != 200 || !strings.Contains(rec.Body.String(), "karryam.support.case.updated.v1") {
		t.Fatalf("feed %d %s", rec.Code, rec.Body)
	}
	var queue hqevents.SupportQueue
	rec := h.do(t, "staff-1", "GET", "/hq/support/cases", nil)
	result(t, rec, &queue)
	if len(queue.Items) != 1 || queue.Items[0].Fields["priority"] != "EMERGENCY" {
		t.Fatalf("queue %+v", queue)
	}
	if rec := h.do(t, "staff-1", "POST", "/hq/event-acknowledgements", map[string]any{"consumer": "hq-support", "sequence": 1}); rec.Code != 200 {
		t.Fatalf("ack %d %s", rec.Code, rec.Body)
	}

	// Campaign: requester proposes, cannot self-approve; second staff approves.
	in := intentFor(t)
	approvalID := uuid.NewString()
	if rec := h.do(t, "staff-1", "POST", "/hq/approvals", map[string]any{"approval_id": approvalID, "kind": "CAMPAIGN", "intent": in}); rec.Code != 200 {
		t.Fatalf("propose %d %s", rec.Code, rec.Body)
	}
	if rec := h.do(t, "staff-1", "POST", "/hq/approvals/"+approvalID+"/decisions", map[string]any{"decision": "APPROVE"}); rec.Code != 409 {
		t.Fatalf("self approval %d", rec.Code)
	}
	if rec := h.do(t, "staff-2", "POST", "/hq/approvals/"+approvalID+"/decisions", map[string]any{"decision": "APPROVE"}); rec.Code != 200 {
		t.Fatalf("approve %d %s", rec.Code, rec.Body)
	}
	// Approver cannot submit someone else's approval.
	if rec := h.do(t, "staff-2", "POST", "/hq/campaigns", map[string]any{"approval_id": approvalID, "intent": in}); rec.Code != 409 {
		t.Fatalf("foreign submit %d", rec.Code)
	}
	var op hqops.CampaignOperation
	rec = h.do(t, "staff-1", "POST", "/hq/campaigns", map[string]any{"approval_id": approvalID, "intent": in})
	result(t, rec, &op)
	sent := h.last.Load().(admincampaign.Intent)
	if op.Status != "CONFIRMED" || sent.RequestedBy != "staff-1" || len(sent.Approvals) != 1 || sent.Approvals[0].ApproverRef != "staff-2" {
		t.Fatalf("submit %+v sent %+v", op, sent)
	}
	before := h.calls.Load()
	// Duplicate submission returns the original operation via read-back only.
	rec = h.do(t, "staff-1", "POST", "/hq/campaigns", map[string]any{"approval_id": approvalID, "intent": in})
	var dup hqops.CampaignOperation
	result(t, rec, &dup)
	if dup.OperationID != op.OperationID || h.calls.Load() != before+1 {
		t.Fatalf("duplicate resent: %+v calls %d->%d", dup, before, h.calls.Load())
	}
	// Same idempotency key, altered content -> conflict, no owner call.
	altered := in
	altered.MaxMessages = 5
	if rec := h.do(t, "staff-1", "POST", "/hq/campaigns", map[string]any{"approval_id": approvalID, "intent": altered}); rec.Code != 409 {
		t.Fatalf("altered %d", rec.Code)
	}
	if rec := h.do(t, "staff-1", "POST", "/hq/campaign-operations/"+op.OperationID+"/cancellations", map[string]any{"cancellation_key": "cancel-1", "reason_code": "HQ_WITHDRAWN"}); rec.Code != 200 || !strings.Contains(rec.Body.String(), "CANCELLED") {
		t.Fatalf("cancel %d %s", rec.Code, rec.Body)
	}

	// Lost owner response -> UNKNOWN_OUTCOME; read-back resolves without resubmit.
	in2 := intentFor(t)
	a2 := uuid.NewString()
	h.do(t, "staff-1", "POST", "/hq/approvals", map[string]any{"approval_id": a2, "kind": "CAMPAIGN", "intent": in2})
	h.do(t, "staff-2", "POST", "/hq/approvals/"+a2+"/decisions", map[string]any{"decision": "APPROVE"})
	h.drop.Store(true)
	rec = h.do(t, "staff-1", "POST", "/hq/campaigns", map[string]any{"approval_id": a2, "intent": in2})
	var unknown hqops.CampaignOperation
	result(t, rec, &unknown)
	h.drop.Store(false)
	if unknown.Status != "UNKNOWN_OUTCOME" {
		t.Fatalf("expected UNKNOWN_OUTCOME %+v", unknown)
	}
	posts := h.calls.Load()
	rec = h.do(t, "staff-1", "GET", "/hq/campaign-operations/"+unknown.OperationID, nil)
	var resolved hqops.CampaignOperation
	result(t, rec, &resolved)
	if resolved.Status != "CONFIRMED" || resolved.OwnerStatus == nil || resolved.OwnerStatus.DeliveredCount != nil || h.calls.Load() != posts+1 {
		t.Fatalf("readback %+v", resolved)
	}

	// Finance: validated aggregate passthrough.
	if rec := h.do(t, "staff-1", "POST", "/hq/finance/reports", map[string]any{"timezone": "Africa/Lagos"}); rec.Code != 200 || !strings.Contains(rec.Body.String(), "NOT_REVENUE") {
		t.Fatalf("finance %d %s", rec.Code, rec.Body)
	}

	// Privacy: registration stores no raw subject; fulfilment needs owner proof.
	reqID := uuid.NewString()
	rec = h.do(t, "staff-1", "POST", "/hq/privacy-requests", map[string]any{"request_id": reqID, "subject_kind": "CUSTOMER", "subject_ref": "cust-9", "request_type": "ERASURE", "received_on": time.Now().UTC().Format("2006-01-02"), "legal_hold": false})
	var pr hqops.PrivacyRequest
	result(t, rec, &pr)
	if pr.Complete || len(pr.Dispositions) < 10 {
		t.Fatalf("privacy %+v", pr)
	}
	var leaked int
	h.store.DB.QueryRow(ctx, `SELECT count(*) FROM admin_privacy_requests WHERE subject_digest LIKE '%cust-9%'`).Scan(&leaked)
	if leaked != 0 {
		t.Fatal("raw subject stored")
	}
	if rec := h.do(t, "staff-1", "POST", "/hq/privacy-requests/"+reqID+"/fulfilments", map[string]any{"owner": "customer-service", "operation_id": uuid.NewString()}); rec.Code != 409 {
		t.Fatalf("unproven fulfilment %d", rec.Code)
	}

	// Export: two-person approval, single-use requester-bound download.
	exportApproval := uuid.NewString()
	h.do(t, "staff-1", "POST", "/hq/approvals", map[string]any{"approval_id": exportApproval, "kind": "EXPORT", "row_limit": 50})
	h.do(t, "staff-2", "POST", "/hq/approvals/"+exportApproval+"/decisions", map[string]any{"decision": "APPROVE"})
	var export hqops.Export
	rec = h.do(t, "staff-1", "POST", "/hq/exports", map[string]any{"approval_id": exportApproval, "row_limit": 50})
	result(t, rec, &export)
	if export.Token == "" {
		t.Fatalf("export %d %s", rec.Code, rec.Body)
	}
	if rec := h.do(t, "staff-2", "POST", "/hq/exports/"+export.ExportID+"/download", map[string]any{"token": export.Token}); rec.Code != 403 {
		t.Fatalf("copied link used by other staff %d", rec.Code)
	}
	rec = h.do(t, "staff-1", "POST", "/hq/exports/"+export.ExportID+"/download", map[string]any{"token": export.Token})
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "text/csv" || !strings.Contains(rec.Body.String(), "composite_id") {
		t.Fatalf("download %d %s", rec.Code, rec.Body)
	}
	if rec := h.do(t, "staff-1", "POST", "/hq/exports/"+export.ExportID+"/download", map[string]any{"token": export.Token}); rec.Code != 409 {
		t.Fatalf("second download %d", rec.Code)
	}
	var cipher []byte
	h.store.DB.QueryRow(ctx, `SELECT ciphertext FROM admin_exports WHERE id=$1`, export.ExportID).Scan(&cipher)
	if bytes.Contains(cipher, []byte("composite_id")) {
		t.Fatal("export stored in plaintext")
	}
}

func TestHQRoutesDefaultDisabledAndWritesKillSwitch(t *testing.T) {
	h := newHQHarness(t, HQFlags{Events: true})
	if rec := h.do(t, "staff-1", "GET", "/hq/support/cases", nil); rec.Code != 503 || !strings.Contains(rec.Body.String(), "capability_blocked") {
		t.Fatalf("disabled queue %d", rec.Code)
	}
	h.f.b.hqFlags.Approvals = true
	h.f.b.features.RemoteWrites = false
	if rec := h.do(t, "staff-1", "POST", "/hq/approvals", map[string]any{"approval_id": uuid.NewString(), "kind": "EXPORT", "row_limit": 1}); rec.Code != 503 || !strings.Contains(rec.Body.String(), "remote_writes_disabled") {
		t.Fatalf("kill switch %d %s", rec.Code, rec.Body)
	}
	// Reads stay available under the global write kill switch.
	if rec := h.do(t, "staff-1", "GET", "/hq/projections/status", nil); rec.Code != 200 {
		t.Fatalf("diagnostics during kill switch %d", rec.Code)
	}
	// A permission missing from the delegated assertion is forbidden.
	h.f.claims.Permissions = []string{"hq.projections.status"}
	if rec := h.do(t, "staff-1", "GET", "/hq/events", nil); rec.Code != 403 {
		t.Fatalf("undelegated %d", rec.Code)
	}
}
