package adminprivacy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/analyticsadmin"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
)

// Handler serves the suppressed mission-success contract only. Other reports
// need their own native reducers, membership proof and approved source contract.
type Handler struct {
	Service *Service
	Enabled bool
	Secret  []byte
	Reports map[string]*Service
}

func (h *Handler) Register(root *gin.Engine) {
	for _, report := range analyticsadmin.Reports() {
		root.GET(report.Path(), h.signedRead)
	}
}
func (h *Handler) signedRead(c *gin.Context) {
	name := strings.TrimPrefix(c.Request.URL.Path, "/internal/analytics/v1/admin/scoped-network/")
	report, known := analyticsadmin.ReportNamed(name)
	if !known {
		c.Status(404)
		return
	}
	local := *h
	if name != "mission-success" {
		local.Service = h.Reports[name]
	}
	h = &local

	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	fail := func(status int, code string) { c.JSON(status, gin.H{"error": gin.H{"code": code}}) }
	if !h.Enabled || h.Service == nil || h.Service.DB == nil || len(h.Secret) < 32 {
		fail(503, "scoped_analytics_disabled")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	r := c.Request
	if r.Method != "GET" || r.URL.Path != report.Path() || r.URL.RawPath != "" || len(r.URL.RequestURI()) > 8192 || strings.ContainsAny(r.URL.Path, "%\\") || r.ContentLength != 0 || len(r.TransferEncoding) > 0 || r.Body != nil && r.Body != http.NoBody {
		fail(400, "invalid_request")
		return
	}
	if r.Body == nil {
		r.Body = http.NoBody
	}
	for _, header := range []string{serviceauth.HeaderServiceName, serviceauth.HeaderVersion, serviceauth.HeaderNonce, serviceauth.HeaderTimestamp, serviceauth.HeaderSignature} {
		if len(r.Header.Values(header)) != 1 {
			fail(401, "unauthorized")
			return
		}
	}
	if _, err := serviceauth.NewVerifier(serviceauth.Secrets{adminread.Caller: h.Secret}, 30*time.Second).VerifyRequest(r); err != nil {
		fail(401, "unauthorized")
		return
	}
	nonce := r.Header.Get(serviceauth.HeaderNonce)
	if !adminread.Opaque(nonce) || nonce != strings.TrimSpace(nonce) {
		fail(401, "unauthorized")
		return
	}
	sum := sha256.Sum256([]byte("analytics.scoped." + report.Name + ":" + nonce))
	claim, err := h.Service.DB.Exec(ctx, `INSERT INTO analytics_admin_http_nonces(caller,nonce_hash,expires_at) VALUES($1,$2,now()+interval '11 minutes') ON CONFLICT DO NOTHING`, adminread.Caller, hex.EncodeToString(sum[:]))
	if err != nil {
		fail(503, "replay_protection_unavailable")
		return
	}
	if claim.RowsAffected() != 1 {
		fail(409, "replay_rejected")
		return
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		fail(400, "invalid_query")
		return
	}
	for _, key := range []string{"authority", "product_code"} {
		if len(values[key]) != 1 {
			fail(400, "invalid_query")
			return
		}
	}
	filters := analyticsadmin.Filters{Authority: values.Get("authority"), ProductCode: values.Get("product_code")}
	values.Del("authority")
	values.Del("product_code")
	query, err := adminread.Parse(values, time.Now())
	if err != nil {
		fail(400, "invalid_query")
		return
	}
	if _, err := report.Values(query, filters); err != nil {
		fail(400, "invalid_query")
		return
	}
	deny := func(status int, code string) {
		detached, done := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer done()
		fingerprint := sha256.Sum256([]byte(r.URL.RawQuery))
		_, e := h.Service.DB.Exec(detached, `INSERT INTO analytics_admin_query_audit(staff_ref,policy_id,policy_version,query_hash,purpose_code,outcome) VALUES($1,$2,$3,$4,$5,$6)`, query.Staff, query.PolicyID, query.PolicyVersion, hex.EncodeToString(fingerprint[:]), query.Purpose, code)
		if e != nil {
			status, code = 503, "audit_unavailable"
		}
		fail(status, code)
	}
	policy := h.Service.Policy
	if policy == nil {
		deny(503, "blocked_policy")
		return
	}
	if query.PolicyID != policy.ID() || query.PolicyVersion != policy.Version() || query.PolicyChecksum != policy.Checksum() {
		deny(503, "policy_revision_mismatch")
		return
	}
	rule, e := policy.Rule(report.Permission, time.Now())
	if e != nil || rule.Owner != "analytics-service" || !report.ValidSourceContract(rule.SourceContract) {
		deny(503, "blocked_policy")
		return
	}
	actor := adminpolicy.Actor{Product: "karryam", Staff: query.Staff, Roles: []string{query.Role}, Permissions: []string{report.Permission}, Markets: []string{query.Scope.Market}, Zones: []string{query.Scope.Zone}, Organizations: []string{query.Scope.Organization}, Tenants: []string{query.Scope.Tenant}}
	q := Query{Capability: report.Permission, Metric: report.Name, Purpose: query.Purpose, Scope: adminpolicy.Scope{Product: "karryam", Market: query.Scope.Market, Zone: query.Scope.Zone, Organization: query.Scope.Organization, Tenant: query.Scope.Tenant}, Start: query.Start, End: query.End, Filters: map[string]string{"authority": filters.Authority, "product_code": filters.ProductCode}}
	out, err := h.Service.Disclose(ctx, actor, q)
	if err != nil {
		status, code := 503, "source_unavailable"
		switch {
		case errors.Is(err, adminpolicy.ErrDenied):
			status, code = 403, "forbidden"
		case errors.Is(err, adminpolicy.ErrBlocked):
			code = "blocked_policy"
		case errors.Is(err, ErrRestricted):
			status, code = 400, "restricted_query"
		case errors.Is(err, ErrRate):
			status, code = 429, "query_budget_exhausted"
		case errors.Is(err, ErrReconstruction):
			status, code = 409, "reconstruction_blocked"
		case errors.Is(err, ErrUnmapped):
			code = "source_membership_unproven"
		}
		// Disclose persists its own validated-query evidence; delegation failures
		// occur before that transaction, so also record a bounded denial here.
		deny(status, code)
		return
	}
	c.JSON(200, out)
}
