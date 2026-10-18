// Package bridge wires the private, deny-by-default HQ boundary.
package bridge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	contract "cosmicforge/logistics/services/admin-backoffice-service/docs/openapi"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/config"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/operations"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/persistence"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/security"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/google/uuid"
)

const Base = "/internal/karryam-admin/v1"

type Repository interface {
	Ready(context.Context) error
	Principal(context.Context, string, string, string, time.Time) error
	Revoked(context.Context, security.Claims) (bool, error)
	Claim(context.Context, string, string, time.Time) error
	Rate(context.Context, string, time.Time, int) (bool, error)
	Audit(context.Context, persistence.Audit) error
	RecordRejected(context.Context, operations.Command, string, string) (operations.Operation, error)
	Operation(context.Context, string, string, security.Scope) (operations.Operation, error)
}
type Bridge struct {
	repo                  Repository
	trust                 config.Trust
	enabled               bool
	verifiers             map[string]*serviceauth.Verifier
	now                   func() time.Time
	Denied, AuditFailures atomic.Uint64
}

func New(repo Repository, trust config.Trust, enabled bool) (*Bridge, error) {
	if repo == nil {
		return nil, errors.New("bridge persistence required")
	}
	if err := trust.Validate(); err != nil {
		return nil, err
	}
	b := &Bridge{repo: repo, trust: trust, enabled: enabled, now: time.Now, verifiers: map[string]*serviceauth.Verifier{}}
	for kid, value := range trust.ServiceKeys {
		secret, _ := base64.RawURLEncoding.DecodeString(value)
		b.verifiers[kid] = serviceauth.NewVerifier(serviceauth.Secrets{config.HQPrincipal: secret}, 30*time.Second)
	}
	return b, nil
}

type response struct {
	ContractVersion string    `json:"contract_version"`
	Product         string    `json:"product"`
	CorrelationID   string    `json:"correlation_id"`
	AsOf            time.Time `json:"as_of"`
	Freshness       string    `json:"freshness"`
	Result          any       `json:"result"`
	Error           *apiError `json:"error,omitempty"`
}
type apiError struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Retryable   bool   `json:"retryable"`
	OperationID string `json:"operation_id,omitempty"`
}

func (b *Bridge) reply(w http.ResponseWriter, correlation string, status int, result any, problem *apiError) {
	freshness := "LIVE"
	if problem != nil {
		freshness = "UNAVAILABLE"
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Correlation-ID", correlation)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response{ContractVersion: "1.0", Product: config.Product, CorrelationID: correlation, AsOf: b.now().UTC(), Freshness: freshness, Result: result, Error: problem})
}

// Probe is mounted only on container loopback, separate from the mTLS listener.
func (b *Bridge) Probe(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" || (r.URL.Path != "/ready" && r.URL.Path != "/health") {
		http.NotFound(w, r)
		return
	}
	if r.URL.Path == "/ready" {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if b.repo.Ready(ctx) != nil {
			w.WriteHeader(503)
			_, _ = w.Write([]byte("unavailable"))
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "contract_version": "1.0", "remote_enabled": b.enabled})
}

func (b *Bridge) authenticate(r *http.Request, body []byte, a *persistence.Audit) (security.Claims, error) {
	var empty security.Claims
	now := b.now()
	if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.VerifiedChains[0]) == 0 || len(r.TLS.PeerCertificates) == 0 {
		return empty, security.ErrDenied
	}
	cert := r.TLS.PeerCertificates[0]
	// VerifiedChains is produced by RequireAndVerifyClientCert, never proxy headers.
	if now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) || len(cert.URIs) != 1 || cert.URIs[0].String() != b.trust.WorkloadURI || !bytes.Equal(r.TLS.VerifiedChains[0][0].Raw, cert.Raw) {
		return empty, security.ErrDenied
	}
	fingerprint := sha256.Sum256(cert.Raw)
	if err := b.repo.Principal(r.Context(), hex.EncodeToString(fingerprint[:]), b.trust.WorkloadURI, b.trust.Audience, now); err != nil {
		return empty, err
	}
	for _, h := range []string{"Authorization", "X-HQ-Key-ID", serviceauth.HeaderServiceName, serviceauth.HeaderSignature, serviceauth.HeaderTimestamp, serviceauth.HeaderVersion, serviceauth.HeaderNonce} {
		if len(r.Header.Values(h)) != 1 {
			return empty, security.ErrDenied
		}
	}
	verifier := b.verifiers[r.Header.Get("X-HQ-Key-ID")]
	nonce := r.Header.Get(serviceauth.HeaderNonce)
	if verifier == nil || b.trust.ServiceKeyExpires[r.Header.Get("X-HQ-Key-ID")] <= now.Unix() || !security.Opaque(nonce) || nonce != strings.TrimSpace(nonce) {
		return empty, security.ErrDenied
	}
	if _, err := verifier.VerifyRequest(r); err != nil {
		return empty, security.ErrDenied
	}
	a.Principal = config.HQPrincipal
	bearer := r.Header.Get("Authorization")
	if !strings.HasPrefix(bearer, "Bearer ") {
		return empty, security.ErrDenied
	}
	c, err := security.Verify(strings.TrimPrefix(bearer, "Bearer "), b.trust, now)
	if err != nil {
		return empty, security.ErrDenied
	}
	if c.RequestDigest != security.Digest(r, body) {
		return empty, security.ErrDenied
	}
	a.Staff = c.Subject
	revoked, err := b.repo.Revoked(r.Context(), c)
	if err != nil {
		return empty, err
	}
	if revoked {
		return empty, security.ErrDenied
	}
	// Retain nonce longer than the entire HMAC acceptance window, not just JWT exp.
	if err = b.repo.Claim(r.Context(), r.Header.Get(serviceauth.HeaderNonce), c.Issuer+":"+c.JTI, now.Add(11*time.Minute)); err != nil {
		return empty, err
	}
	return c, nil
}

func (b *Bridge) audit(r *http.Request, a persistence.Audit) bool {
	// A cancelled client must not cancel its audit write.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Second)
	defer cancel()
	if err := b.repo.Audit(ctx, a); err != nil {
		b.AuditFailures.Add(1)
		slog.Error("admin_audit_unavailable", "correlation_id", a.Correlation, "outcome", a.Outcome)
		return false
	}
	return true
}

func (b *Bridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	r = r.WithContext(ctx)
	correlation := uuid.NewString() // caller-supplied headers never become audit identifiers
	a := persistence.Audit{Correlation: correlation, PolicyVersion: b.trust.PolicyVersion, TargetService: config.Caller, RequestType: "unrecognized", Outcome: "DENIED"}
	fail := func(status int, code string) {
		count := b.Denied.Add(1)
		slog.Warn("admin_request_denied", "correlation_id", correlation, "code", code, "denied_total", count)
		a.Outcome = code
		if !b.audit(r, a) {
			status = 503
			code = "audit_unavailable"
		}
		b.reply(w, correlation, status, nil, &apiError{Code: code, Message: "Request could not be completed.", Retryable: (status == 503 && code != "capability_blocked" && code != "remote_disabled") || status == 429})
	}
	if len(r.URL.RequestURI()) > 4096 || r.URL.RawPath != "" || strings.ContainsAny(r.URL.Path, "%\\") || strings.Contains(r.URL.Path, "//") {
		fail(400, "invalid_request")
		return
	}
	for _, segment := range strings.Split(r.URL.Path, "/") {
		if segment == "." || segment == ".." {
			fail(400, "invalid_request")
			return
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		fail(413, "request_too_large")
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	if !b.enabled {
		fail(503, "remote_disabled")
		return
	}
	actor, err := b.authenticate(r, body, &a)
	if err != nil {
		status, code := 401, "unauthorized"
		if errors.Is(err, persistence.ErrReplay) {
			status = 409
			code = "replay_rejected"
		} else if !errors.Is(err, security.ErrDenied) {
			status = 503
			code = "authentication_unavailable"
		}
		fail(status, code)
		return
	}
	a.Principal = config.HQPrincipal
	a.Staff = actor.Subject
	path := strings.TrimPrefix(r.URL.Path, Base)
	capability, ok := contract.Match(r.Method, path)
	if !strings.HasPrefix(r.URL.Path, Base+"/") || !ok {
		fail(404, "not_found")
		return
	}
	a.RequestType = capability.ID
	a.Permission = capability.Permission
	a.TargetService = capability.Owner
	query, err := strictQuery(r)
	if err != nil {
		fail(400, "invalid_query")
		return
	}
	scope := security.Scope{Market: query.Get("market"), Organization: query.Get("organization")}
	a.Market = scope.Market
	a.Organization = scope.Organization
	a.ReasonCode = query.Get("purpose")
	if !security.Opaque(a.ReasonCode) {
		a.ReasonCode = ""
		fail(400, "purpose_required")
		return
	}
	if err = security.Authorize(actor, security.Policy{Permission: capability.Permission, Risk: security.Risk(capability.Risk), Version: b.trust.PolicyVersion}, scope, b.trust); err != nil {
		a.Market = ""
		a.Organization = ""
		fail(403, "forbidden")
		return
	}
	allowed, err := b.repo.Rate(r.Context(), strings.Join([]string{actor.Subject, capability.Permission, scope.Market, scope.Organization}, ":"), b.now(), b.trust.RequestsPerMinute)
	if err != nil {
		fail(503, "authorization_unavailable")
		return
	}
	if !allowed {
		w.Header().Set("Retry-After", "60")
		fail(429, "rate_limited")
		return
	}
	if r.Method == "GET" && len(body) != 0 {
		fail(400, "invalid_request")
		return
	}

	var result any
	switch capability.ID {
	case "bridge.capabilities":
		result = contract.Discover(actor.Permissions)
	case "bridge.health":
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if b.repo.Ready(ctx) != nil {
			fail(503, "bridge_unavailable")
			return
		}
		result = map[string]any{"bridge": "READY", "owners": "UNAVAILABLE", "remote_writes": "DISABLED", "contract_version": "1.0"}
	case "bridge.operations.read":
		id := strings.TrimPrefix(path, "/operations/")
		parsed, e := uuid.Parse(id)
		if e != nil || parsed.String() != id {
			fail(400, "invalid_operation_id")
			return
		}
		op, e := b.repo.Operation(r.Context(), id, actor.Subject, scope)
		if errors.Is(e, operations.ErrNotFound) {
			fail(404, "not_found")
			return
		}
		if e != nil {
			fail(503, "operation_unavailable")
			return
		}
		a.TargetRef = id
		a.Status = op.Status
		if op.SourceVersion != nil {
			a.SourceVersion = *op.SourceVersion
		}
		result = op
	case "bridge.operations.submit":
		media, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if e != nil || media != "application/json" {
			fail(415, "json_required")
			return
		}
		var command operations.Command
		if security.Strict(body, &command) != nil || command.Validate(b.now()) != nil || command.Scope != scope {
			fail(400, "invalid_command")
			return
		}
		a.TargetRef = command.Target.ID
		a.ReasonCode = command.ReasonCode
		a.EvidenceRef = command.CaseReference
		op, e := b.repo.RecordRejected(r.Context(), command, actor.Subject, security.Hash(string(body)))
		if errors.Is(e, operations.ErrConflict) {
			fail(409, "idempotency_conflict")
			return
		}
		if e != nil {
			fail(503, "operation_unavailable")
			return
		}
		a.Outcome = "CAPABILITY_BLOCKED"
		a.Status = op.Status
		if !b.audit(r, a) {
			b.reply(w, correlation, 503, nil, &apiError{Code: "audit_unavailable", Message: "Request could not be completed.", Retryable: true})
			return
		}
		b.reply(w, correlation, 503, nil, &apiError{Code: "capability_blocked", Message: "No owner command is enabled.", Retryable: false, OperationID: op.ID})
		return
	default:
		fail(503, "capability_blocked")
		return
	}
	a.Outcome = "AUTHORIZED"
	if !b.audit(r, a) {
		b.reply(w, correlation, 503, nil, &apiError{Code: "audit_unavailable", Message: "Request could not be completed.", Retryable: true})
		return
	}
	b.reply(w, correlation, 200, result, nil)
}
