package vendors

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/verification-compliance-service/internal/testdb"
	"cosmicforge/logistics/shared/go/adminevents"
	"cosmicforge/logistics/shared/go/mediaclient"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestCaseStateMachineTable(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	future, past := now.Add(24*time.Hour), now.Add(-time.Hour)
	all := []string{StatusNotStarted, StatusInProgress, StatusPendingReview, StatusVerified, StatusRejected, StatusExpired, StatusSuspended}
	allowed := map[[2]string]bool{
		{StatusNotStarted, StatusInProgress}: true, {StatusNotStarted, StatusPendingReview}: true,
		{StatusPendingReview, StatusInProgress}: true, {StatusPendingReview, StatusVerified}: true, {StatusPendingReview, StatusRejected}: true,
		{StatusInProgress, StatusPendingReview}: true, {StatusInProgress, StatusVerified}: true, {StatusInProgress, StatusRejected}: true,
		{StatusVerified, StatusSuspended}: true, {StatusVerified, StatusExpired}: true,
		{StatusRejected, StatusPendingReview}: true, {StatusExpired, StatusPendingReview}: true, {StatusSuspended, StatusPendingReview}: true,
	}
	for _, from := range all {
		for _, to := range all {
			tr := Transition{From: from, To: to, ReasonCode: "OTHER_REVIEWED", ExpiresAt: &future, Now: now}
			if IsReopen(from, to) {
				tr.AppealRef = "appeal-1"
			}
			err := tr.Validate()
			if allowed[[2]string{from, to}] != (err == nil) {
				t.Errorf("%s -> %s: allowed=%v err=%v", from, to, allowed[[2]string{from, to}], err)
			}
		}
	}
	cases := []struct {
		name string
		tr   Transition
		want error
	}{
		{"verify without expiry", Transition{From: StatusInProgress, To: StatusVerified, Now: now}, ErrFutureExpiryRequired},
		{"verify with past expiry", Transition{From: StatusInProgress, To: StatusVerified, ExpiresAt: &past, Now: now}, ErrFutureExpiryRequired},
		{"reject without reason", Transition{From: StatusPendingReview, To: StatusRejected, Now: now}, ErrReasonRequired},
		{"suspend without reason", Transition{From: StatusVerified, To: StatusSuspended, Now: now}, ErrReasonRequired},
		{"free text reason", Transition{From: StatusPendingReview, To: StatusRejected, ReasonCode: "blurry photo", Now: now}, ErrInvalidReason},
		{"reopen without appeal", Transition{From: StatusRejected, To: StatusPendingReview, Now: now}, ErrAppealRequired},
		{"appeal on ordinary move", Transition{From: StatusPendingReview, To: StatusInProgress, AppealRef: "appeal-1", Now: now}, ErrAppealNotApplicable},
		{"plain PENDING", Transition{From: StatusPendingReview, To: "PENDING", Now: now}, ErrIllegalTransition},
		{"unknown source", Transition{From: "PENDING", To: StatusInProgress, Now: now}, ErrIllegalTransition},
		{"self transition", Transition{From: StatusPendingReview, To: StatusPendingReview, Now: now}, ErrIllegalTransition},
		{"verified to verified", Transition{From: StatusVerified, To: StatusVerified, ExpiresAt: &future, Now: now}, ErrIllegalTransition},
	}
	for _, c := range cases {
		if err := c.tr.Validate(); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v want %v", c.name, err, c.want)
		}
	}
}

func TestRecommendationNeverAutoEligibleWithoutExternalVerification(t *testing.T) {
	base := []Finding{{CheckRefsPresent, OutcomePass, SeverityInfo}, {CheckMediaState, OutcomePass, SeverityInfo}}
	if got := Recommend(append(base, Finding{CheckExternal, OutcomeSourceUnverified, SeverityWarn})); got != RecommendationSourceUnverified {
		t.Fatalf("unavailable external authority -> %s", got)
	}
	if got := Recommend(append(base, Finding{CheckDuplicateDoc, OutcomeFail, SeverityBlocking}, Finding{CheckExternal, OutcomePass, SeverityInfo})); got != RecommendationReviewRequired {
		t.Fatalf("any FAIL must require review, got %s", got)
	}
	if got := Recommend(append(base, Finding{CheckExternal, OutcomePass, SeverityInfo})); got != RecommendationAutoEligible {
		t.Fatalf("external verified -> %s", got)
	}
	if got := Recommend(append(base, Finding{CheckExternal, OutcomeNotEvaluated, SeverityInfo})); got != RecommendationFormatValid {
		t.Fatalf("external not applicable -> %s", got)
	}
	result, err := DisabledAuthority{}.Check(context.Background(), CaseFacts{})
	if err != nil || result != ExternalUnavailable {
		t.Fatalf("disabled authority must be unavailable: %s %v", result, err)
	}
}

func seedCase(t *testing.T, pool *pgxpool.Pool, business, dimension string, docs []string) string {
	t.Helper()
	raw, _ := json.Marshal(docs)
	key := "seed-" + uuid.NewString()
	var id string
	if err := pool.QueryRow(context.Background(), `INSERT INTO vendor_verification_cases(client_reference,business_id,dimension,subject_type,subject_ref,document_refs,request_fingerprint,idempotency_key) VALUES($1,$2,$3,'BUSINESS',$2,$4,'fp',$1) RETURNING id::text`, key, business, dimension, raw).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO vendor_verification_case_history(case_id,source_version,status,observed_at) VALUES($1::uuid,1,'PENDING_REVIEW',now())`, id); err != nil {
		t.Fatal(err)
	}
	return id
}

func decideInTx(t *testing.T, pool *pgxpool.Pool, in DecisionInput) (DecisionResult, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if in.DecidedBy == "" {
		in.DecidedBy, in.DecidedByKind = "staff-test-reviewer", DecidedByStaff
	}
	if in.OperationRef == "" {
		in.OperationRef = uuid.NewString()
	}
	out, err := ApplyDecision(ctx, tx, in)
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}

func TestSection18DecisionRouteContract(t *testing.T) {
	pool := testdb.Fresh(t)
	ctx := context.Background()
	adminSecret := []byte("admin-secret")
	router := gin.New()
	Register(router.Group("/api/v1/verification-compliance"), New(pool, true, []string{"vendor-service", "admin-backoffice-service"}), serviceauth.NewVerifier(serviceauth.Secrets{"admin-backoffice-service": adminSecret}, 5*time.Minute))
	base := "/api/v1/verification-compliance/internal/vendor-cases/cases/"
	post := func(ref string, body map[string]any, key string) *httptest.ResponseRecorder {
		return call(t, router, "admin-backoffice-service", adminSecret, http.MethodPost, base+ref+"/decisions", body, key, uuid.NewString())
	}
	code := func(r *httptest.ResponseRecorder) string {
		var env struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(r.Body.Bytes(), &env)
		return env.Error.Code
	}
	id := seedCase(t, pool, "biz-route", "BUSINESS", []string{"doc-route-1"})

	if r := post(uuid.NewString(), map[string]any{"status": "IN_PROGRESS", "expected_source_version": 1}, "k-unknown"); r.Code != 404 || code(r) != "CASE_NOT_FOUND" {
		t.Fatalf("unknown case=%d %s", r.Code, r.Body.String())
	}
	if r := post("not-a-case", map[string]any{"status": "IN_PROGRESS", "expected_source_version": 1}, "k-garbage"); r.Code != 404 {
		t.Fatalf("garbage case ref=%d", r.Code)
	}
	if r := post(id, map[string]any{"status": "SUSPENDED", "reason_code": "FRAUD_SIGNAL", "expected_source_version": 1}, "k-illegal"); r.Code != 409 || code(r) != "ILLEGAL_TRANSITION" {
		t.Fatalf("illegal=%d %s", r.Code, r.Body.String())
	}
	if r := post(id, map[string]any{"status": "REJECTED", "expected_source_version": 1}, "k-noreason"); r.Code != 400 || code(r) != "REASON_REQUIRED" {
		t.Fatalf("reject without reason=%d %s", r.Code, r.Body.String())
	}
	if r := post(id, map[string]any{"status": "VERIFIED", "expected_source_version": 1}, "k-noexpiry"); r.Code != 400 || code(r) != "FUTURE_EXPIRY_REQUIRED" {
		t.Fatalf("verify without expiry=%d %s", r.Code, r.Body.String())
	}
	if r := post(id, map[string]any{"status": "VERIFIED", "expires_at": time.Now().Add(-time.Hour), "expected_source_version": 1}, "k-pastexpiry"); r.Code != 400 {
		t.Fatalf("verify past expiry=%d", r.Code)
	}

	start := map[string]any{"status": "IN_PROGRESS", "expected_source_version": 1, "decided_by_staff_ref": "staff-ada"}
	if r := post(id, start, "k-start"); r.Code != 200 {
		t.Fatalf("start=%d %s", r.Code, r.Body.String())
	}
	if r := post(id, start, "k-start"); r.Code != 200 || !strings.Contains(strings.ReplaceAll(r.Body.String(), " ", ""), `"source_version":2`) {
		t.Fatalf("idempotent replay=%d %s", r.Code, r.Body.String())
	}
	changed := map[string]any{"status": "PENDING_REVIEW", "expected_source_version": 1, "decided_by_staff_ref": "staff-ada"}
	if r := post(id, changed, "k-start"); r.Code != 409 || code(r) != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("key reuse with different body=%d %s", r.Code, r.Body.String())
	}
	other := seedCase(t, pool, "biz-route-2", "BUSINESS", []string{"doc-route-2"})
	if r := post(other, start, "k-start"); r.Code != 409 || code(r) != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("key reuse on another case=%d %s", r.Code, r.Body.String())
	}
	if r := post(id, map[string]any{"status": "PENDING_REVIEW", "expected_source_version": 1}, "k-stale"); r.Code != 409 || code(r) != "VERSION_CONFLICT" {
		t.Fatalf("stale=%d %s", r.Code, r.Body.String())
	}
	var decidedBy, kind string
	if err := pool.QueryRow(ctx, `SELECT decided_by,decided_by_kind FROM vendor_verification_case_history WHERE case_id=$1::uuid AND source_version=2`, id).Scan(&decidedBy, &kind); err != nil || decidedBy != "staff-ada" || kind != DecidedByStaff {
		t.Fatalf("history decider=%s kind=%s err=%v", decidedBy, kind, err)
	}

	// Duplicate nonce is refused before anything is read.
	nonce := uuid.NewString()
	first := call(t, router, "admin-backoffice-service", adminSecret, http.MethodGet, base+id, nil, "", nonce)
	if first.Code != 200 {
		t.Fatalf("read=%d", first.Code)
	}
	if replay := call(t, router, "admin-backoffice-service", adminSecret, http.MethodPost, base+id+"/decisions", map[string]any{"status": "PENDING_REVIEW", "expected_source_version": 2}, "k-nonce", nonce); replay.Code != 401 {
		t.Fatalf("duplicate decision nonce=%d", replay.Code)
	}

	// Two reviewers race on the same version with different keys: one wins.
	var wins, conflicts atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := map[string]any{"status": "REJECTED", "reason_code": "DOCUMENT_MISMATCH", "expected_source_version": 2}
			if i == 1 {
				body = map[string]any{"status": "VERIFIED", "expires_at": time.Now().Add(365 * 24 * time.Hour), "expected_source_version": 2}
			}
			switch post(id, body, "k-race-"+uuid.NewString()).Code {
			case 200:
				wins.Add(1)
			case 409:
				conflicts.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if wins.Load() != 1 || conflicts.Load() != 1 {
		t.Fatalf("concurrent reviewers wins=%d conflicts=%d", wins.Load(), conflicts.Load())
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM vendor_verification_case_history WHERE case_id=$1::uuid AND source_version=3`, id).Scan(&count); err != nil || count != 1 {
		t.Fatalf("exactly one decision at version 3, got %d err=%v", count, err)
	}

	// History is append-only.
	if _, err := pool.Exec(ctx, `UPDATE vendor_verification_case_history SET decided_by='forged' WHERE case_id=$1::uuid`, id); err == nil {
		t.Fatal("history UPDATE succeeded")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM vendor_verification_case_history WHERE case_id=$1::uuid`, id); err == nil {
		t.Fatal("history DELETE succeeded")
	}

	// Reopen through the route needs an appeal, once.
	reopenCase := seedCase(t, pool, "biz-route-3", "BRAND", []string{"doc-route-3"})
	if r := post(reopenCase, map[string]any{"status": "REJECTED", "reason_code": "INSUFFICIENT_EVIDENCE", "expected_source_version": 1}, "k-r3-reject"); r.Code != 200 {
		t.Fatalf("reject=%d %s", r.Code, r.Body.String())
	}
	if r := post(reopenCase, map[string]any{"status": "PENDING_REVIEW", "expected_source_version": 2}, "k-r3-noappeal"); r.Code != 400 || code(r) != "APPEAL_REQUIRED" {
		t.Fatalf("reopen without appeal=%d %s", r.Code, r.Body.String())
	}
	if r := post(reopenCase, map[string]any{"status": "PENDING_REVIEW", "appeal_ref": "appeal-r3", "expected_source_version": 2}, "k-r3-reopen"); r.Code != 200 {
		t.Fatalf("reopen=%d %s", r.Code, r.Body.String())
	}
	if r := post(reopenCase, map[string]any{"status": "REJECTED", "reason_code": "INSUFFICIENT_EVIDENCE", "expected_source_version": 3}, "k-r3-reject2"); r.Code != 200 {
		t.Fatalf("second reject=%d", r.Code)
	}
	if r := post(reopenCase, map[string]any{"status": "PENDING_REVIEW", "appeal_ref": "appeal-r3", "expected_source_version": 4}, "k-r3-reuse"); r.Code != 409 || code(r) != "APPEAL_ALREADY_DECIDED" {
		t.Fatalf("appeal reuse=%d %s", r.Code, r.Body.String())
	}
}

type stubMedia map[string]mediaclient.MediaAsset

func (s stubMedia) GetByID(_ context.Context, id string) (mediaclient.MediaAsset, error) {
	asset, ok := s[id]
	if !ok {
		return mediaclient.MediaAsset{}, mediaclient.Error{StatusCode: http.StatusNotFound}
	}
	return asset, nil
}

func findingsFor(t *testing.T, pool *pgxpool.Pool, id string) map[string]Finding {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT check_code,outcome,severity FROM vendor_case_automation_findings WHERE case_id=$1::uuid`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]Finding{}
	for rows.Next() {
		var f Finding
		if err := rows.Scan(&f.CheckCode, &f.Outcome, &f.Severity); err != nil {
			t.Fatal(err)
		}
		out[f.CheckCode] = f
	}
	return out
}

func caseState(t *testing.T, pool *pgxpool.Pool, id string) (string, int64, string) {
	t.Helper()
	var status, recommendation string
	var version int64
	if err := pool.QueryRow(context.Background(), `SELECT status,source_version,automation_recommendation FROM vendor_verification_cases WHERE id=$1::uuid`, id).Scan(&status, &version, &recommendation); err != nil {
		t.Fatal(err)
	}
	return status, version, recommendation
}

func TestSection18AutomationRecordsFindingsAndNeverDecides(t *testing.T) {
	pool := testdb.Fresh(t)
	ctx := context.Background()
	now := time.Now().UTC()
	media := stubMedia{
		"doc-shared":  {ID: "doc-shared", Status: "active"},
		"doc-clean":   {ID: "doc-clean", Status: "active"},
		"doc-revoked": {ID: "doc-revoked", Status: "revoked"},
		"doc-expired": {ID: "doc-expired", Status: "active", Metadata: map[string]any{"expires_at": now.Add(-time.Hour).Format(time.RFC3339)}},
	}
	dualA := seedCase(t, pool, "biz-a", "BUSINESS", []string{"doc-shared"})
	dualB := seedCase(t, pool, "biz-b", "IDENTITY", []string{"doc-shared"})
	revoked := seedCase(t, pool, "biz-c", "BUSINESS", []string{"doc-revoked"})
	expired := seedCase(t, pool, "biz-d", "FOOD_KITCHEN", []string{"doc-expired"})
	missing := seedCase(t, pool, "biz-e", "BUSINESS", []string{"doc-missing"})
	clean := seedCase(t, pool, "biz-f", "IDENTITY", []string{"doc-clean"})
	closed := seedCase(t, pool, "biz-g", "BUSINESS", []string{"doc-clean-2"})
	if _, err := decideInTx(t, pool, DecisionInput{CaseRef: closed, ExpectedVersion: 1, Status: StatusRejected, ReasonCode: "OTHER_REVIEWED", Now: now}); err != nil {
		t.Fatal(err)
	}

	worker := &Automation{DB: pool, Media: media, External: DisabledAuthority{}, Owner: "test-automation"}
	recorded, err := worker.RunOnce(ctx, 50)
	if err != nil || recorded != 6 {
		t.Fatalf("recorded=%d err=%v", recorded, err)
	}
	for _, id := range []string{dualA, dualB} {
		f := findingsFor(t, pool, id)
		if d := f[CheckDuplicateDoc]; d.Outcome != OutcomeFail || d.Severity != SeverityBlocking {
			t.Fatalf("dual-source mismatch not flagged: %+v", d)
		}
		if _, _, rec := caseState(t, pool, id); rec != RecommendationReviewRequired {
			t.Fatalf("dual-source recommendation=%s", rec)
		}
	}
	for _, id := range []string{revoked, expired, missing} {
		if f := findingsFor(t, pool, id)[CheckMediaState]; f.Outcome != OutcomeFail {
			t.Fatalf("media failure not flagged for %s: %+v", id, f)
		}
		if _, _, rec := caseState(t, pool, id); rec != RecommendationReviewRequired {
			t.Fatalf("media failure recommendation=%s", rec)
		}
	}
	f := findingsFor(t, pool, clean)
	if f[CheckExternal].Outcome != OutcomeSourceUnverified || f[CheckMediaState].Outcome != OutcomePass || f[CheckAddress].Outcome != OutcomeNotEvaluated {
		t.Fatalf("clean findings=%+v", f)
	}
	for _, id := range []string{dualA, dualB, revoked, expired, missing, clean} {
		status, version, rec := caseState(t, pool, id)
		if status != StatusPendingReview || version != 1 {
			t.Fatalf("automation changed formal status: %s v%d", status, version)
		}
		if rec == RecommendationAutoEligible || status == StatusVerified {
			t.Fatal("automation produced eligibility without an external authority")
		}
	}
	if _, _, rec := caseState(t, pool, clean); rec != RecommendationSourceUnverified {
		t.Fatalf("unavailable KYC recommendation=%s", rec)
	}
	if len(findingsFor(t, pool, closed)) != 0 {
		t.Fatal("a closed case was evaluated")
	}
	if again, err := worker.RunOnce(ctx, 50); err != nil || again != 0 {
		t.Fatalf("re-evaluated an unchanged version: %d %v", again, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE vendor_case_automation_findings SET outcome='PASS'`); err == nil {
		t.Fatal("findings UPDATE succeeded")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM vendor_case_automation_findings`); err == nil {
		t.Fatal("findings DELETE succeeded")
	}

	// A new version (reviewer picked the case up) is evaluated again, and
	// media not configured is NOT_EVALUATED rather than a pass.
	if _, err := decideInTx(t, pool, DecisionInput{CaseRef: clean, ExpectedVersion: 1, Status: StatusInProgress, Now: now}); err != nil {
		t.Fatal(err)
	}
	noMedia := &Automation{DB: pool, Owner: "test-automation"}
	if n, err := noMedia.RunOnce(ctx, 50); err != nil || n != 1 {
		t.Fatalf("new version evaluated=%d err=%v", n, err)
	}
	var mediaOutcome string
	if err := pool.QueryRow(ctx, `SELECT outcome FROM vendor_case_automation_findings WHERE case_id=$1::uuid AND case_version=2 AND check_code='MEDIA_OBJECT_STATE'`, clean).Scan(&mediaOutcome); err != nil || mediaOutcome != OutcomeNotEvaluated {
		t.Fatalf("media not configured outcome=%s err=%v", mediaOutcome, err)
	}

	// Every emitted admin event decodes, minimizes and carries no document,
	// subject or remediation data.
	assertAdminEvents(t, pool, []string{"doc-shared", "doc-clean", "doc-revoked", "doc-expired", "doc-missing"})
}

func assertAdminEvents(t *testing.T, pool *pgxpool.Pool, forbidden []string) {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT payload::text FROM admin_event_outbox ORDER BY created_at`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		n++
		ev, err := adminevents.Decode([]byte(raw))
		if err != nil {
			t.Fatalf("event does not decode: %v %s", err, raw)
		}
		adapter, ok := adminevents.Lookup(ev.SourceService, ev.EventType)
		if !ok {
			t.Fatal("event type not registered")
		}
		if _, err := adapter.Minimize(ev); err != nil {
			t.Fatalf("event does not minimize: %v %s", err, raw)
		}
		for _, value := range append(forbidden, "document_refs", "subject_ref", "remediation", "staff-", "nin", "bank_account") {
			if strings.Contains(strings.ToLower(raw), strings.ToLower(value)) {
				t.Fatalf("event leaks %q: %s", value, raw)
			}
		}
	}
	if n == 0 {
		t.Fatal("no admin events emitted")
	}
}

func TestSection18ExpirySweepNotifiesVendorAndCallbackRepair(t *testing.T) {
	pool := testdb.Fresh(t)
	ctx := context.Background()
	now := time.Now().UTC()
	kitchen := seedCase(t, pool, "biz-kitchen", "FOOD_KITCHEN", []string{"doc-kitchen"})
	expires := now.Add(48 * time.Hour)
	if _, err := decideInTx(t, pool, DecisionInput{CaseRef: kitchen, ExpectedVersion: 1, Status: StatusVerified, ExpiresAt: &expires, Now: now}); err != nil {
		t.Fatal(err)
	}
	still := seedCase(t, pool, "biz-kitchen-2", "FOOD_KITCHEN", []string{"doc-kitchen-2"})
	later := now.Add(30 * 24 * time.Hour)
	if _, err := decideInTx(t, pool, DecisionInput{CaseRef: still, ExpectedVersion: 1, Status: StatusVerified, ExpiresAt: &later, Now: now}); err != nil {
		t.Fatal(err)
	}
	if n, err := RunExpirySweep(ctx, pool, 10, nil); err != nil || n != 0 {
		t.Fatalf("expired a still-valid case: %d %v", n, err)
	}
	n, err := RunExpirySweep(ctx, pool, 10, func() time.Time { return now.Add(72 * time.Hour) })
	if err != nil || n != 1 {
		t.Fatalf("expired=%d err=%v", n, err)
	}
	if status, version, _ := caseState(t, pool, kitchen); status != StatusExpired || version != 3 {
		t.Fatalf("kitchen case=%s v%d", status, version)
	}
	if status, _, _ := caseState(t, pool, still); status != StatusVerified {
		t.Fatalf("unexpired case moved to %s", status)
	}
	var decidedBy, kind string
	if err := pool.QueryRow(ctx, `SELECT decided_by,decided_by_kind FROM vendor_verification_case_history WHERE case_id=$1::uuid AND source_version=3`, kitchen).Scan(&decidedBy, &kind); err != nil || decidedBy != ExpiryDecider || kind != DecidedBySystem {
		t.Fatalf("expiry decider=%s kind=%s err=%v", decidedBy, kind, err)
	}
	var callbackID string
	var payload []byte
	if err := pool.QueryRow(ctx, `SELECT id::text,payload FROM verification_outbox WHERE aggregate_id=$1::uuid AND event_type='vendor.verification.status_changed' AND event_version=3`, kitchen).Scan(&callbackID, &payload); err != nil || !strings.Contains(strings.ReplaceAll(string(payload), " ", ""), `"status":"EXPIRED"`) || !strings.Contains(strings.ReplaceAll(string(payload), " ", ""), `"dimension":"FOOD_KITCHEN"`) {
		t.Fatalf("expiry callback row=%s err=%v", payload, err)
	}

	// Vendor is down: retries end in a dead letter; an operator replay repairs.
	var healthy atomic.Bool
	var delivered atomic.Int32
	secret := []byte("callback-secret")
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{"verification-compliance-service": secret}, 5*time.Minute)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := verifier.VerifyRequest(r); err != nil {
			http.Error(w, "unauthenticated", http.StatusUnauthorized)
			return
		}
		if !healthy.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		delivered.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	worker := NewCallbackWorker(pool, server.URL, string(secret), time.Second)
	if _, err := pool.Exec(ctx, `UPDATE verification_outbox SET delivery_status='DELIVERED' WHERE event_type='vendor.verification.status_changed' AND id<>$1::uuid`, callbackID); err != nil {
		t.Fatal(err)
	}
	var status string
	for i := 0; i < 8; i++ {
		if _, err := pool.Exec(ctx, `UPDATE verification_outbox SET next_attempt_at=now() WHERE id=$1::uuid`, callbackID); err != nil {
			t.Fatal(err)
		}
		if err := worker.Process(ctx, "test-callback", 10); err != nil {
			t.Fatal(err)
		}
	}
	if err := pool.QueryRow(ctx, `SELECT delivery_status FROM verification_outbox WHERE id=$1::uuid`, callbackID).Scan(&status); err != nil || status != "DEAD_LETTER" {
		t.Fatalf("callback status after failures=%s err=%v", status, err)
	}
	healthy.Store(true)
	if err := worker.Replay(ctx, callbackID); err != nil {
		t.Fatal(err)
	}
	if err := worker.Process(ctx, "test-callback", 10); err != nil || delivered.Load() != 1 {
		t.Fatalf("replay delivered=%d err=%v", delivered.Load(), err)
	}
	assertAdminEvents(t, pool, []string{"doc-kitchen"})
}
