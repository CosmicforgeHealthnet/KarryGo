package hqevents

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/admin-backoffice-service/internal/testutil"
	"cosmicforge/logistics/shared/go/adminevents"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/google/uuid"
)

func caseEvent(t *testing.T, ref string, version int64, priority string, extra map[string]any) []byte {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Millisecond)
	p := map[string]any{"status": "OPEN", "priority": priority, "service_type": "taxi", "category": nil, "complainant_type": "customer", "context_type": nil, "locale": nil, "sla_state": "POLICY_UNAVAILABLE", "consent_classification": "OPERATIONS_MINIMIZED", "assignee": nil, "escalation_level": nil, "mission_ref": nil, "order_ref": nil, "dispute_ref": nil, "unread_count": version, "message_count": version, "evidence_count": 0, "created_at": now, "updated_at": now}
	for k, v := range extra {
		p[k] = v
	}
	ev, err := adminevents.NewSourceEvent("support-dispute-service", adminevents.SupportCaseChanged, "support_case", ref, version, now, adminevents.Scope{}, "", p, false)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(ev)
	return raw
}

func TestPostgresEventInboxDedupeVersionFenceGapAndQuarantine(t *testing.T) {
	ctx := context.Background()
	s := &Store{DB: testutil.Database(t)}
	ref := uuid.NewString()
	v1 := caseEvent(t, ref, 1, "NORMAL", nil)
	if o, err := s.Ingest(ctx, "support-dispute-service", v1, DeliveryIngress); err != nil || o != Applied {
		t.Fatalf("v1 %v %v", o, err)
	}
	// Replay of the same event: one inbox row, no second export.
	if o, _ := s.Ingest(ctx, "support-dispute-service", v1, DeliveryIngress); o != Duplicate {
		t.Fatalf("replay %v", o)
	}
	v3 := caseEvent(t, ref, 3, "EMERGENCY", nil)
	if o, _ := s.Ingest(ctx, "support-dispute-service", v3, DeliveryIngress); o != GapApplied {
		t.Fatalf("gap %v", o)
	}
	// Older version arriving late must never regress the projection.
	v2 := caseEvent(t, ref, 2, "NORMAL", nil)
	if o, _ := s.Ingest(ctx, "support-dispute-service", v2, DeliveryIngress); o != Stale {
		t.Fatalf("stale %v", o)
	}
	var version int64
	var priority string
	if err := s.DB.QueryRow(ctx, `SELECT entity_version,payload->>'priority' FROM admin_projections WHERE entity_ref=$1`, ref).Scan(&version, &priority); err != nil || version != 3 || priority != "EMERGENCY" {
		t.Fatalf("projection regressed %d %s %v", version, priority, err)
	}
	var exports, gaps int
	s.DB.QueryRow(ctx, `SELECT count(*) FROM admin_hq_outbox`).Scan(&exports)
	s.DB.QueryRow(ctx, `SELECT count(*) FROM admin_projection_gaps WHERE repaired_at IS NULL`).Scan(&gaps)
	if exports != 2 || gaps != 1 {
		t.Fatalf("exports=%d gaps=%d", exports, gaps)
	}
	// Free text / extra PII fields / foreign product / wrong caller are quarantined, never projected.
	for _, raw := range [][]byte{
		caseEvent(t, uuid.NewString(), 1, "NORMAL", map[string]any{"complainant_phone": "+2348030000000"}),
		caseEvent(t, uuid.NewString(), 1, "NORMAL", map[string]any{"assignee": "free text name"}),
		bytes.Replace(caseEvent(t, uuid.NewString(), 1, "NORMAL", nil), []byte(`"product":"karryam"`), []byte(`"product":"healthnet"`), 1),
	} {
		if o, err := s.Ingest(ctx, "support-dispute-service", raw, DeliveryIngress); o != Quarantined || !errors.Is(err, ErrQuarantined) {
			t.Fatalf("not quarantined %v %v", o, err)
		}
	}
	if _, err := s.Ingest(ctx, "notification-service", caseEvent(t, uuid.NewString(), 1, "NORMAL", nil), DeliveryIngress); !errors.Is(err, ErrMismatch) {
		t.Fatalf("source mismatch accepted: %v", err)
	}
	var quarantined, projections int
	s.DB.QueryRow(ctx, `SELECT count(*) FROM admin_event_quarantine`).Scan(&quarantined)
	s.DB.QueryRow(ctx, `SELECT count(*) FROM admin_projections`).Scan(&projections)
	if quarantined != 4 || projections != 1 {
		t.Fatalf("quarantine=%d projections=%d", quarantined, projections)
	}
	var reasons string
	s.DB.QueryRow(ctx, `SELECT string_agg(reason,',' ORDER BY reason) FROM admin_event_quarantine`).Scan(&reasons)
	if reasons != "INVALID_PAYLOAD,PRODUCT_MISMATCH,SOURCE_MISMATCH,UNKNOWN_SCHEMA" {
		t.Fatalf("reasons %s", reasons)
	}
	// Evidence tables are append-only.
	if _, err := s.DB.Exec(ctx, `DELETE FROM admin_event_inbox`); err == nil {
		t.Fatal("inbox mutable")
	}
	if _, err := s.DB.Exec(ctx, `UPDATE admin_hq_outbox SET export='{}'`); err == nil {
		t.Fatal("outbox mutable")
	}
}

func TestPostgresConcurrentDuplicateDeliveriesApplyOnce(t *testing.T) {
	ctx := context.Background()
	s := &Store{DB: testutil.Database(t)}
	raw := caseEvent(t, uuid.NewString(), 1, "HIGH", nil)
	var wg sync.WaitGroup
	var mu sync.Mutex
	applied := 0
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			o, err := s.Ingest(ctx, "support-dispute-service", raw, DeliveryIngress)
			if err != nil {
				t.Error(err)
			}
			mu.Lock()
			if o == Applied {
				applied++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	var exports int
	s.DB.QueryRow(ctx, `SELECT count(*) FROM admin_hq_outbox`).Scan(&exports)
	if applied != 1 || exports != 1 {
		t.Fatalf("applied=%d exports=%d", applied, exports)
	}
}

func TestPostgresFeedCursorScopeAndMonotonicAck(t *testing.T) {
	ctx := context.Background()
	s := &Store{DB: testutil.Database(t)}
	for i := 0; i < 5; i++ {
		if _, err := s.Ingest(ctx, "support-dispute-service", caseEvent(t, uuid.NewString(), 1, "NORMAL", nil), DeliveryIngress); err != nil {
			t.Fatal(err)
		}
	}
	page, err := s.Feed(ctx, 0, 2, Scope{Market: "NG"})
	if err != nil || len(page.Events) != 2 || page.HeadSequence != 5 {
		t.Fatalf("page %+v %v", page, err)
	}
	var first adminevents.ExportEvent
	if err := json.Unmarshal(page.Events[0], &first); err != nil || first.Schema != adminevents.ExportSchema || first.Product != "karryam" || first.Sequence != 1 {
		t.Fatalf("export %+v %v", first, err)
	}
	for _, forbidden := range []string{"phone", "subject", "description", "incident_lat", "name"} {
		if strings.Contains(string(page.Events[0]), forbidden) {
			t.Fatalf("export leaks %s", forbidden)
		}
	}
	next, _ := s.Feed(ctx, page.NextSequence, 10, Scope{})
	if len(next.Events) != 3 {
		t.Fatalf("resume %d", len(next.Events))
	}
	if err := s.Ack(ctx, "hq-support", 4, "staff-a"); err != nil {
		t.Fatal(err)
	}
	if err := s.Ack(ctx, "hq-support", 3, "staff-a"); !errors.Is(err, ErrCursor) {
		t.Fatalf("regressing ack accepted %v", err)
	}
	if err := s.Ack(ctx, "hq-support", 99, "staff-a"); !errors.Is(err, ErrCursor) {
		t.Fatalf("ack beyond head accepted %v", err)
	}
	st, err := s.Status(ctx, time.Hour)
	if err != nil || st.Consumers["hq-support"] != 1 {
		t.Fatalf("status %+v %v", st, err)
	}
}

type fakeSource struct {
	pages []adminevents.SnapshotPage
	fail  int
}

func (f *fakeSource) Source() string     { return "support-dispute-service" }
func (f *fakeSource) EntityType() string { return "support_case" }
func (f *fakeSource) Page(_ context.Context, cursor string) (adminevents.SnapshotPage, error) {
	i := 0
	if cursor != "" {
		i = int(cursor[1] - '0')
	}
	if f.fail > 0 && i == f.fail {
		return adminevents.SnapshotPage{}, errors.New("owner down")
	}
	return f.pages[i], nil
}

func page(t *testing.T, events [][]byte, next string) adminevents.SnapshotPage {
	p := adminevents.SnapshotPage{Contract: adminevents.SnapshotContract, Source: "support-dispute-service", EntityType: "support_case", Watermark: time.Now().UTC(), NextCursor: next, Complete: next == ""}
	for _, raw := range events {
		ev, err := adminevents.Decode(raw)
		if err != nil {
			t.Fatal(err)
		}
		p.Events = append(p.Events, ev)
	}
	if err := p.Validate("support-dispute-service"); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPostgresBackfillReconciliationAndPartialCoverage(t *testing.T) {
	ctx := context.Background()
	s := &Store{DB: testutil.Database(t)}
	a, b, c := uuid.NewString(), uuid.NewString(), uuid.NewString()
	// Incremental v5 already present for a; snapshot carries older v4 -> stays v5.
	s.Ingest(ctx, "support-dispute-service", caseEvent(t, a, 5, "NORMAL", nil), DeliveryIngress)
	// Gap on b repaired by reconciliation.
	s.Ingest(ctx, "support-dispute-service", caseEvent(t, b, 1, "NORMAL", nil), DeliveryIngress)
	s.Ingest(ctx, "support-dispute-service", caseEvent(t, b, 3, "NORMAL", nil), DeliveryIngress)
	// c exists in the Bridge but the owner no longer reports it.
	s.Ingest(ctx, "support-dispute-service", caseEvent(t, c, 1, "NORMAL", nil), DeliveryIngress)
	d := uuid.NewString()
	src := &fakeSource{pages: []adminevents.SnapshotPage{
		page(t, [][]byte{caseEvent(t, a, 4, "NORMAL", nil), caseEvent(t, b, 4, "NORMAL", nil)}, "p1"),
		page(t, [][]byte{caseEvent(t, d, 2, "HIGH", nil)}, ""),
	}}
	res, err := s.Sweep(ctx, src, DeliveryReconciliation, 10)
	if err != nil || res.Status != "COMPLETE" || res.MissingAtSource != 1 || res.Repaired != 2 {
		t.Fatalf("reconcile %+v %v", res, err)
	}
	var av int64
	s.DB.QueryRow(ctx, `SELECT entity_version FROM admin_projections WHERE entity_ref=$1`, a).Scan(&av)
	var open, cPresent int
	s.DB.QueryRow(ctx, `SELECT count(*) FROM admin_projection_gaps WHERE repaired_at IS NULL`).Scan(&open)
	s.DB.QueryRow(ctx, `SELECT count(*) FROM admin_projections WHERE entity_ref=$1`, c).Scan(&cPresent)
	if av != 5 || open != 0 || cPresent != 1 {
		t.Fatalf("a=%d openGaps=%d cKept=%d", av, open, cPresent)
	}
	// Owner outage mid-snapshot: honest PARTIAL_COVERAGE, never COMPLETE.
	partial, err := s.Sweep(ctx, &fakeSource{pages: src.pages, fail: 1}, DeliveryBackfill, 10)
	if err != nil || partial.Status != "PARTIAL_COVERAGE" {
		t.Fatalf("partial %+v %v", partial, err)
	}
	st, _ := s.Status(ctx, time.Hour)
	for _, x := range st.Sources {
		if x.SourceService == "support-dispute-service" && x.Coverage != "PARTIAL_COVERAGE" {
			t.Fatalf("coverage claimed after partial run: %+v", x)
		}
		if x.SourceService == "notification-service" && (x.Freshness != "UNKNOWN" || x.LagSeconds != nil) {
			t.Fatalf("empty source must be UNKNOWN, not fresh zero: %+v", x)
		}
	}
	q, err := s.SupportQueue(ctx, Scope{}, "", "", "", 10)
	if err != nil || len(q.Items) != 4 || q.Items[0].Fields["priority"] != "HIGH" || !strings.HasPrefix(q.Items[0].CompositeID, "karryam:support-dispute-service:") {
		t.Fatalf("queue %+v %v", q, err)
	}
}

func TestPostgresSignedIngressReplayAndCallerBinding(t *testing.T) {
	_ = context.Background()
	s := &Store{DB: testutil.Database(t)}
	secret := bytes.Repeat([]byte{7}, 32)
	other := bytes.Repeat([]byte{9}, 32)
	h := &IngressHandler{Store: s, Enabled: true, Metrics: &Metrics{}, Verifier: serviceauth.NewVerifier(serviceauth.Secrets{"support-dispute-service": secret, "notification-service": other}, 30*time.Second)}
	send := func(name string, key []byte, body []byte, nonce string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, adminevents.IngressPath, bytes.NewReader(body))
		if err := serviceauth.SignRequest(req, name, key, body, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		if nonce != "" {
			req.Header.Set(serviceauth.HeaderNonce, nonce)
			_ = serviceauth.SignRequest(req, name, key, body, time.Now().UTC())
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	body := caseEvent(t, uuid.NewString(), 1, "EMERGENCY", nil)
	if rec := send("support-dispute-service", secret, body, ""); rec.Code != 202 {
		t.Fatalf("accept %d %s", rec.Code, rec.Body)
	}
	// Exact replay of a signed request (same nonce) is refused.
	fresh := caseEvent(t, uuid.NewString(), 1, "NORMAL", nil)
	req := httptest.NewRequest(http.MethodPost, adminevents.IngressPath, bytes.NewReader(fresh))
	serviceauth.SignRequest(req, "support-dispute-service", secret, fresh, time.Now().UTC())
	replay := httptest.NewRequest(http.MethodPost, adminevents.IngressPath, bytes.NewReader(fresh))
	replay.Header = req.Header.Clone()
	h.ServeHTTP(httptest.NewRecorder(), req)
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, replay)
	if rec2.Code != 409 {
		t.Fatalf("replay %d", rec2.Code)
	}
	// Another owner cannot publish support events (event poisoning).
	if rec := send("notification-service", other, caseEvent(t, uuid.NewString(), 1, "NORMAL", nil), ""); rec.Code != 403 {
		t.Fatalf("cross-owner %d", rec.Code)
	}
	// Tampered body or wrong key.
	if rec := send("support-dispute-service", other, body, ""); rec.Code != 401 {
		t.Fatalf("bad key %d", rec.Code)
	}
	disabled := &IngressHandler{Store: s, Verifier: h.Verifier}
	r := httptest.NewRequest(http.MethodPost, adminevents.IngressPath, bytes.NewReader(body))
	w := httptest.NewRecorder()
	disabled.ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatalf("disabled ingress %d", w.Code)
	}
	if h.Metrics.IngressAccepted.Load() < 1 || h.Metrics.IngressReplay.Load() != 1 {
		t.Fatal("metrics not recorded")
	}
}
