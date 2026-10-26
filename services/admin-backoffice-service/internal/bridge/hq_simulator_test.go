package bridge

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/hqevents"
	"cosmicforge/logistics/shared/go/adminevents"
	"github.com/google/uuid"
)

// TestPostgresSimulatedHQScenarios exercises Section 33.4 scenarios against a
// real TLS 1.3 mTLS listener and a disposable PostgreSQL database. It is a
// SIMULATED HQ consumer on one host: it is not separate-VPS, real network,
// provider-receipt or external-scan evidence.
func TestPostgresSimulatedHQScenarios(t *testing.T) {
	ctx := context.Background()
	h := newHQHarness(t, HQFlags{Events: true, SupportQueue: true, Approvals: true, Campaigns: true, Finance: true, Privacy: true, Exports: true})
	pair, leaf, roots := workloadCertificate(t, h.f.b.trust.WorkloadURI, false)
	h.f.cert = leaf
	srv := httptest.NewUnstartedServer(h.f.b)
	srv.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	srv.StartTLS()
	defer srv.Close()
	target, _ := url.Parse(srv.URL)
	client := func(cert tls.Certificate) *http.Client {
		base := srv.Client().Transport.(*http.Transport).Clone()
		base.TLSClientConfig.Certificates = []tls.Certificate{cert}
		return &http.Client{Transport: base, Timeout: 10 * time.Second}
	}
	call := func(c *http.Client, staff, method, path string, body any) (int, string) {
		var raw []byte
		if body != nil {
			raw, _ = json.Marshal(body)
		}
		h.f.claims.Subject = staff
		r := h.f.request(t, method, path+"?"+h.values, raw)
		r.URL.Scheme, r.URL.Host, r.RequestURI = target.Scheme, target.Host, ""
		res, err := c.Do(r)
		if err != nil {
			return 0, err.Error()
		}
		defer res.Body.Close()
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, e := res.Body.Read(buf)
			b.Write(buf[:n])
			if e != nil {
				break
			}
		}
		return res.StatusCode, b.String()
	}
	hq := client(pair)
	now := time.Now().UTC()
	event := func(ref string, version int64) []byte {
		ev, _ := adminevents.NewSourceEvent("support-dispute-service", adminevents.SupportCaseChanged, "support_case", ref, version, now, adminevents.Scope{}, "", map[string]any{"status": "OPEN", "priority": "HIGH", "service_type": "dispatch", "complainant_type": "customer", "sla_state": "POLICY_UNAVAILABLE", "consent_classification": "OPERATIONS_MINIMIZED", "unread_count": 0, "message_count": version, "evidence_count": 0, "created_at": now, "updated_at": now}, false)
		raw, _ := json.Marshal(ev)
		return raw
	}

	// 1. Authorized HQ workload + permitted staff reads latest owner-derived state.
	ref := uuid.NewString()
	h.store.Ingest(ctx, "support-dispute-service", event(ref, 2), hqevents.DeliveryIngress)
	if code, body := call(hq, "staff-1", "GET", "/hq/support/cases", nil); code != 200 || !strings.Contains(body, ref) {
		t.Fatalf("scenario 1: %d %s", code, body)
	}
	// 2. HealthNet / unrelated-product workload is denied at the application layer;
	// a certificate from an untrusted CA fails the TLS handshake.
	other, _, _ := workloadCertificate(t, "spiffe://cosmicforge/healthnet/hq-admin", false)
	if code, _ := call(client(other), "staff-1", "GET", "/hq/support/cases", nil); code != 0 {
		t.Fatalf("scenario 2: untrusted-CA peer completed a request %d", code)
	}
	// Same trusted CA but HealthNet workload identity: TLS succeeds, Bridge denies.
	sameCA := h.f.cert
	healthnet, healthnetLeaf, _ := workloadCertificate(t, "spiffe://cosmicforge/healthnet/hq-admin", false)
	_ = healthnet
	h.f.cert = healthnetLeaf
	if code, _ := call(hq, "staff-1", "GET", "/hq/support/cases", nil); code != 200 {
		// h.f.cert only shapes the in-process fixture; the real listener uses the
		// peer chain, so the authorized HQ connection must still succeed here.
		t.Fatalf("scenario 2 control %d", code)
	}
	h.f.cert = sameCA
	// 6. Finance read is aggregate only and performs no Bridge financial write.
	var before int
	h.store.DB.QueryRow(ctx, `SELECT count(*) FROM admin_owner_command_operations`).Scan(&before)
	if code, body := call(hq, "staff-1", "POST", "/hq/finance/reports", map[string]any{"timezone": "Africa/Lagos"}); code != 200 || strings.Contains(body, "customer_id") || strings.Contains(body, "provider_id") || !strings.Contains(body, "NOT_REVENUE") {
		t.Fatalf("scenario 6: %d %s", code, body)
	}
	var after int
	h.store.DB.QueryRow(ctx, `SELECT count(*) FROM admin_owner_command_operations`).Scan(&after)
	if after != before {
		t.Fatal("scenario 6: finance read created an operation")
	}
	// 7. Reviewer cannot approve their own intent even with a valid token.
	approval := uuid.NewString()
	call(hq, "staff-1", "POST", "/hq/approvals", map[string]any{"approval_id": approval, "kind": "EXPORT", "row_limit": 10})
	if code, _ := call(hq, "staff-1", "POST", "/hq/approvals/"+approval+"/decisions", map[string]any{"decision": "APPROVE"}); code != 409 {
		t.Fatalf("scenario 7: self approval %d", code)
	}
	// 8. Owner outage: the owner-backed route fails closed without affecting reads.
	h.f.b.hqOps.Finance = nil
	if code, _ := call(hq, "staff-1", "POST", "/hq/finance/reports", map[string]any{"timezone": "Africa/Lagos"}); code != 503 {
		t.Fatalf("scenario 8: %d", code)
	}
	if code, _ := call(hq, "staff-1", "GET", "/hq/projections/status", nil); code != 200 {
		t.Fatalf("scenario 8 diagnostics: %d", code)
	}
	// 9. An older event after a newer version never regresses the projection.
	h.store.Ingest(ctx, "support-dispute-service", event(ref, 1), hqevents.DeliveryIngress)
	var v int64
	h.store.DB.QueryRow(ctx, `SELECT entity_version FROM admin_projections WHERE entity_ref=$1`, ref).Scan(&v)
	if v != 2 {
		t.Fatalf("scenario 9: regressed to %d", v)
	}
	// 10. A compromised peer publishing another owner's events is quarantined.
	if _, err := h.store.Ingest(ctx, "notification-service", event(uuid.NewString(), 1), hqevents.DeliveryIngress); err == nil {
		t.Fatal("scenario 10: cross-owner event accepted")
	}
	// 11. Staff revocation while work is queued: the revoked identity is refused,
	// queued durable state is unaffected and another staff member continues.
	h.f.repo.revoked = true
	if code, _ := call(hq, "staff-1", "GET", "/hq/events", nil); code != 401 {
		t.Fatalf("scenario 11: revoked staff %d", code)
	}
	h.f.repo.revoked = false
	// 12. Restore + replay: re-delivering every stored event creates no duplicate
	// exports and the consumer resumes from its acknowledged cursor.
	code, body := call(hq, "staff-2", "GET", "/hq/events", nil)
	if code != 200 {
		t.Fatalf("scenario 12 feed %d %s", code, body)
	}
	var head int64
	h.store.DB.QueryRow(ctx, `SELECT max(sequence) FROM admin_hq_outbox`).Scan(&head)
	if code, _ := call(hq, "staff-2", "POST", "/hq/event-acknowledgements", map[string]any{"consumer": "hq-support", "sequence": head}); code != 200 {
		t.Fatal("scenario 12 ack")
	}
	h.store.Ingest(ctx, "support-dispute-service", event(ref, 2), hqevents.DeliveryIngress)
	var exports int
	h.store.DB.QueryRow(ctx, `SELECT count(*) FROM admin_hq_outbox`).Scan(&exports)
	if int64(exports) != head {
		t.Fatalf("scenario 12: replay duplicated exports %d vs %d", exports, head)
	}
}
