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
	"cosmicforge/logistics/services/admin-backoffice-service/internal/clients"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/config"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/customerreviewops"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/customerworkitemops"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/operations"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/ownercommandops"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/persistence"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/security"
	"cosmicforge/logistics/shared/go/adminmedia"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/analyticsadmin"
	"cosmicforge/logistics/shared/go/customerreview"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/google/uuid"
	"strconv"
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
	repo                     Repository
	trust                    config.Trust
	enabled                  bool
	taxi                     *clients.Taxi
	resources                map[string]*clients.Resources
	resourcePolicies         map[string]*adminpolicy.Engine
	taxiRoles                []string
	accessPolicy             *adminpolicy.Engine
	features                 config.DomainFlags
	customerReview           *customerreviewops.Service
	customerWorkItems        *customerworkitemops.Service
	ownerCommands            map[string]*ownercommandops.Service
	mediaCases               *adminmedia.Client
	mediaPolicy              *adminpolicy.Engine
	mediaReads, mediaRevokes bool
	scopedAnalytics          *analyticsadmin.Client
	scopedAnalyticsPolicy    *adminpolicy.Engine
	scopedAnalyticsRate      bool
	verifiers                map[string]*serviceauth.Verifier
	now                      func() time.Time
	Denied, AuditFailures    atomic.Uint64
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
	asOf := b.now().UTC()
	if snapshot, ok := result.(adminread.TaxiSnapshot); ok && problem == nil {
		asOf, freshness = snapshot.AsOf, snapshot.Freshness
	}
	if snapshot, ok := result.(adminowner.Snapshot); ok && problem == nil {
		asOf, freshness = snapshot.AsOf, "PARTIAL"
	}
	if snapshot, ok := result.(analyticsadmin.Release); ok && problem == nil {
		asOf, freshness = snapshot.AsOf, snapshot.Freshness
	}
	if rate, ok := result.(analyticsadmin.RateRelease); ok && problem == nil {
		asOf, freshness = rate.AsOf, rate.Freshness
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Correlation-ID", correlation)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response{ContractVersion: "1.0", Product: config.Product, CorrelationID: correlation, AsOf: asOf, Freshness: freshness, Result: result, Error: problem})
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
	if strings.HasPrefix(capability.ID, "media.case.") {
		b.serveMediaCase(w, r, body, actor, a, capability.ID, correlation)
		return
	}
	resourceProfile, resourceDefinition, resourceRef, resourceRead := adminowner.MatchBridge(path)
	workItemRequest := capability.ID == "customer.review.work_item.submit" || capability.ID == "customer.review.work_item.readback"
	reviewRequest := capability.ID == "customer.review.submit" || capability.ID == "customer.review.readback" || workItemRequest
	analyticsRate := capability.ID == analyticsadmin.MissionRatePermission
	analyticsReport, scopedReport := scopedReportCapability(capability.ID)
	analyticsRead := scopedReport || analyticsRate
	commandService, commandRoute, ownerCommand := b.ownerCommandRoute(capability.ID)
	readPolicy := b.accessPolicy
	if ownerCommand {
		if commandService == nil {
			fail(503, "capability_blocked")
			return
		}
		readPolicy = commandService.Policy
		if readPolicy == nil {
			fail(503, "blocked_policy")
			return
		}
	}
	if analyticsRead {
		if b.scopedAnalytics == nil || !b.features.AnalyticsReads {
			fail(503, "capability_blocked")
			return
		}
		if analyticsRate && !b.scopedRateApproved() {
			fail(503, "blocked_policy")
			return
		}
		readPolicy = b.scopedAnalyticsPolicy
	}
	if reviewRequest {
		if workItemRequest {
			if b.customerWorkItems == nil {
				fail(503, "capability_blocked")
				return
			}
			readPolicy = b.customerWorkItems.Policy
		} else {
			if b.customerReview == nil {
				fail(503, "capability_blocked")
				return
			}
			readPolicy = b.customerReview.Policy
		}
	}
	if resourceRead {
		readPolicy = b.resourcePolicies[resourceProfile.Owner]
		if readPolicy == nil {
			fail(503, "blocked_policy")
			return
		}
	}
	if capability.Permission == adminread.TaxiPermission && b.accessPolicy == nil {
		fail(503, "blocked_policy")
		return
	}
	var taxiQuery adminread.Query
	var analyticsFilters analyticsadmin.Filters
	query, err := strictQuery(r)
	if capability.Permission == adminread.TaxiPermission || resourceRead || reviewRequest || analyticsRead || ownerCommand {
		role := actor.Roles[0]
		approvedRoles := b.taxiRoles
		if analyticsRead {
			rule, e := readPolicy.Rule(analyticsReport.Permission, b.now())
			if e != nil || rule.Owner != "analytics-service" || !analyticsReport.ValidSourceContract(rule.SourceContract) {
				fail(503, "blocked_policy")
				return
			}
			approvedRoles = rule.Roles
		}
		if reviewRequest {
			rule, e := readPolicy.Rule(capability.Permission, b.now())
			expected := customerreview.Contract
			if r.Method == "GET" {
				expected = customerreview.StateContract
			}
			if workItemRequest {
				expected = customerreview.WorkItemContract
			}
			if e != nil || rule.Owner != customerreview.Owner || rule.SourceContract != expected {
				fail(503, "blocked_policy")
				return
			}
			approvedRoles = rule.Roles
		}
		if ownerCommand {
			spec := commandService.Owner.Spec()
			rule, e := readPolicy.Rule(capability.Permission, b.now())
			if e != nil || rule.Owner != spec.Owner || rule.SourceContract != spec.Contract {
				fail(503, "blocked_policy")
				return
			}
			approvedRoles = rule.Roles
		}
		if resourceRead {
			rule, e := readPolicy.Rule(resourceDefinition.Capability, b.now())
			if e != nil || rule.Owner != resourceProfile.Owner || rule.SourceContract != resourceDefinition.SourceContract {
				fail(503, "blocked_policy")
				return
			}
			approvedRoles = rule.Roles
		}
		for _, candidate := range actor.Roles {
			if config.Contains(approvedRoles, candidate) {
				role = candidate
				break
			}
		}
		if analyticsRead {
			taxiQuery, analyticsFilters, query, err = scopedAnalyticsQuery(r, actor.Subject, role, readPolicy, b.now())
		} else {
			taxiQuery, query, err = taxiReadQuery(r, actor.Subject, role, readPolicy, b.now())
		}
	}
	if err != nil {
		fail(400, "invalid_query")
		return
	}
	scope := security.Scope{Market: query.Get("market"), Organization: query.Get("organization")}
	a.Market = scope.Market
	a.Organization = scope.Organization
	a.Zone = taxiQuery.Scope.Zone
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
	if capability.Permission == adminread.TaxiPermission && (!config.Contains(actor.Zones, taxiQuery.Scope.Zone) || !hasRole(actor.Roles, b.taxiRoles)) {
		fail(403, "forbidden")
		return
	}
	if capability.Permission == adminread.TaxiPermission || resourceRead || reviewRequest || analyticsRead || ownerCommand {
		delegated := adminpolicy.Actor{Product: actor.Product, Staff: actor.Subject, Roles: actor.Roles, Permissions: actor.Permissions, Markets: actor.Markets, Zones: actor.Zones, Organizations: actor.Organizations, Tenants: actor.Tenants}
		requested := adminpolicy.Scope{Product: config.Product, Market: taxiQuery.Scope.Market, Zone: taxiQuery.Scope.Zone, Organization: taxiQuery.Scope.Organization, Tenant: taxiQuery.Scope.Tenant}
		if e := readPolicy.AuthorizeDelegation(delegated, capability.Permission, taxiQuery.Purpose, requested, b.now()); e != nil {
			if errors.Is(e, adminpolicy.ErrBlocked) {
				fail(503, "blocked_policy")
			} else {
				fail(403, "forbidden")
			}
			return
		}
		if analyticsRate {
			if e := readPolicy.AuthorizeDelegation(delegated, analyticsadmin.MissionSuccessPermission, taxiQuery.Purpose, requested, b.now()); e != nil {
				fail(403, "forbidden")
				return
			}
		}
		a.EvidenceRef = readPolicy.ID() + "@" + strconv.FormatInt(readPolicy.Version(), 10) + ":" + readPolicy.Checksum()
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
	if ownerCommand {
		delegated := adminpolicy.Actor{Product: actor.Product, Staff: actor.Subject, Roles: actor.Roles, Permissions: actor.Permissions, Markets: actor.Markets, Zones: actor.Zones, Organizations: actor.Organizations, Tenants: actor.Tenants}
		b.serveOwnerCommand(w, r, body, commandService, commandRoute, path, delegated, taxiQuery, a, correlation)
		return
	}

	var result any
	switch capability.ID {
	case "bridge.capabilities":
		_, policyErr := b.accessPolicy.Rule(adminread.TaxiPermission, b.now())
		availability := b.resourceAvailability(actor.Roles, actor.Zones, actor.Tenants)
		b.customerReviewAvailability(availability, actor.Roles, actor.Zones, actor.Tenants)
		b.customerWorkItemAvailability(availability, actor.Roles, actor.Zones, actor.Tenants)
		b.mediaCaseAvailability(availability, actor.Roles, actor.Zones, actor.Tenants)
		b.ownerCommandAvailability(availability, actor.Roles, actor.Zones, actor.Tenants)
		for _, report := range analyticsadmin.Reports() {
			availability[report.Permission] = b.scopedReportAvailable(report, actor.Roles, actor.Zones, actor.Tenants)
		}
		availability[analyticsadmin.MissionRatePermission] = b.scopedRateAvailable(actor.Roles, actor.Zones, actor.Tenants, actor.Permissions)
		availability[adminread.TaxiPermission] = b.taxi != nil && policyErr == nil && hasRole(actor.Roles, b.taxiRoles) && len(actor.Zones) > 0 && len(actor.Tenants) > 0
		result = contract.DiscoverWithAvailability(actor.Permissions, availability)
	case "taxi.rides.list", "taxi.rides.detail":
		if b.taxi == nil {
			fail(503, "capability_blocked")
			return
		}
		id := ""
		if capability.ID == "taxi.rides.detail" {
			id = strings.TrimPrefix(path, "/taxi/rides/")
			ref, e := uuid.Parse(id)
			if e != nil || ref.String() != id {
				fail(400, "invalid_reference")
				return
			}
		}
		a.TargetRef = id
		snapshot, e := b.taxi.Rides(r.Context(), taxiQuery, id)
		if errors.Is(e, clients.ErrScopeDenied) {
			fail(403, "forbidden")
			return
		}
		if errors.Is(e, clients.ErrNotFound) {
			fail(404, "not_found")
			return
		}
		if e != nil {
			fail(503, "owner_unavailable")
			return
		}
		result = snapshot
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
	case "analytics.scoped.mission-success.read", "analytics.scoped.mission-success-rate.read", "analytics.scoped.mission-outcomes.read", "analytics.scoped.recovery.read", "analytics.scoped.capability-mismatches.read", "analytics.scoped.declines.read", "analytics.scoped.demand.read", "analytics.scoped.demand-forecast.read", "analytics.scoped.supply.read", "analytics.scoped.provider-quality.read", "analytics.scoped.recovery-resolution.read", "analytics.scoped.provider-quality-score.read", "analytics.scoped.readiness-components.read":
		out, e := b.scopedReportRead(r.Context(), analyticsReport.Name, taxiQuery, analyticsFilters)
		if errors.Is(e, analyticsadmin.ErrDenied) {
			fail(403, "forbidden")
			return
		}
		if errors.Is(e, analyticsadmin.ErrRate) {
			fail(429, "rate_limited")
			return
		}
		if errors.Is(e, analyticsadmin.ErrInvalid) {
			fail(503, "owner_contract_invalid")
			return
		}
		if e != nil {
			fail(503, "owner_unavailable")
			return
		}
		if analyticsRate {
			rate, e := analyticsadmin.MissionRate(out)
			if e != nil {
				fail(503, "owner_contract_invalid")
				return
			}
			result = rate
		} else {
			result = out
		}
	case "customer.review.work_item.submit", "customer.review.work_item.readback":
		delegated := adminpolicy.Actor{Product: actor.Product, Staff: actor.Subject, Roles: actor.Roles, Permissions: actor.Permissions, Markets: actor.Markets, Zones: actor.Zones, Organizations: actor.Organizations, Tenants: actor.Tenants}
		var out customerworkitemops.Operation
		var e error
		if r.Method == "POST" {
			if !b.features.WriteFamilyRequested(customerreview.WorkItemCapability) {
				fail(503, "capability_blocked")
				return
			}
			media, _, parseErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if parseErr != nil || media != "application/json" {
				fail(415, "json_required")
				return
			}
			input, decodeErr := customerreview.DecodeWorkItemCommand(body)
			if decodeErr != nil {
				fail(400, "invalid_command")
				return
			}
			a.TargetRef = input.CustomerID
			a.ReasonCode = input.ReasonCode
			out, e = b.customerWorkItems.Submit(r.Context(), delegated, taxiQuery, input)
		} else {
			id := strings.TrimPrefix(path, "/customer/review-work-items/")
			a.TargetRef = id
			out, e = b.customerWorkItems.Reconcile(r.Context(), delegated, taxiQuery, id)
		}
		if e != nil {
			status, code := 503, "operation_unavailable"
			switch {
			case errors.Is(e, customerworkitemops.ErrDisabled):
				code = "capability_blocked"
			case errors.Is(e, adminpolicy.ErrBlocked):
				code = "blocked_policy"
			case errors.Is(e, adminpolicy.ErrDenied), errors.Is(e, customerreview.ErrRefused):
				status, code = 403, "forbidden"
			case errors.Is(e, customerworkitemops.ErrConflict):
				status, code = 409, "idempotency_conflict"
			case errors.Is(e, customerworkitemops.ErrNotFound):
				status, code = 404, "not_found"
			case errors.Is(e, customerreview.ErrInvalid):
				status, code = 400, "invalid_command"
			case errors.Is(e, customerworkitemops.ErrProof):
				code = "owner_contract_invalid"
			}
			// Generic transport retry advice must never cause a command resubmit.
			a.Outcome = code
			if !b.audit(r, a) {
				status, code = 503, "audit_unavailable"
			}
			b.reply(w, correlation, status, nil, &apiError{Code: code, Message: "Use original operation readback to resolve the outcome.", Retryable: false})
			return
		}
		a.Status = out.Status
		if out.Receipt != nil {
			a.SourceVersion = out.Receipt.RecordedVersion
		}
		result = out
	case "customer.review.submit", "customer.review.readback":
		delegated := adminpolicy.Actor{Product: actor.Product, Staff: actor.Subject, Roles: actor.Roles, Permissions: actor.Permissions, Markets: actor.Markets, Zones: actor.Zones, Organizations: actor.Organizations, Tenants: actor.Tenants}
		var out customerreviewops.Operation
		var e error
		if r.Method == "POST" {
			if !b.features.WriteFamilyRequested(customerreview.RequestCapability) {
				fail(503, "capability_blocked")
				return
			}
			media, _, parseErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if parseErr != nil || media != "application/json" {
				fail(415, "json_required")
				return
			}
			input, decodeErr := customerreview.DecodeRequest(body)
			if decodeErr != nil {
				fail(400, "invalid_command")
				return
			}
			a.TargetRef = input.CustomerID
			a.ReasonCode = input.ReasonCode
			out, e = b.customerReview.Submit(r.Context(), delegated, taxiQuery, input)
		} else {
			id := strings.TrimPrefix(path, "/customer/review-requests/")
			a.TargetRef = id
			out, e = b.customerReview.Reconcile(r.Context(), delegated, taxiQuery, id)
		}
		if e != nil {
			status, code := 503, "operation_unavailable"
			switch {
			case errors.Is(e, customerreviewops.ErrDisabled):
				code = "capability_blocked"
			case errors.Is(e, adminpolicy.ErrBlocked):
				code = "blocked_policy"
			case errors.Is(e, adminpolicy.ErrDenied), errors.Is(e, customerreview.ErrRefused):
				status, code = 403, "forbidden"
			case errors.Is(e, customerreviewops.ErrConflict):
				status, code = 409, "idempotency_conflict"
			case errors.Is(e, customerreviewops.ErrNotFound):
				status, code = 404, "not_found"
			case errors.Is(e, customerreview.ErrInvalid):
				status, code = 400, "invalid_command"
			case errors.Is(e, customerreviewops.ErrProof):
				code = "owner_contract_invalid"
			}
			// Generic transport retry advice must never cause a command resubmit.
			a.Outcome = code
			if !b.audit(r, a) {
				status, code = 503, "audit_unavailable"
			}
			b.reply(w, correlation, status, nil, &apiError{Code: code, Message: "Use original operation readback to resolve the outcome.", Retryable: false})
			return
		}
		a.Status = out.Status
		if out.Receipt != nil {
			a.SourceVersion = out.Receipt.RecordedReviewRevision
		}
		result = out
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
		message := "Remote privileged writes are disabled."
		if b.features.WriteFamilyRequested(command.Action) {
			message = "No approved owner command adapter is enabled."
		}
		b.reply(w, correlation, 503, nil, &apiError{Code: "capability_blocked", Message: message, Retryable: false, OperationID: op.ID})
		return
	default:
		if !resourceRead || r.Method != "GET" || b.resources[resourceProfile.Owner] == nil {
			fail(503, "capability_blocked")
			return
		}
		a.TargetRef = resourceRef
		snapshot, e := b.resources[resourceProfile.Owner].Read(r.Context(), taxiQuery, resourceDefinition.Kind, resourceRef)
		if errors.Is(e, clients.ErrScopeDenied) {
			fail(403, "forbidden")
			return
		}
		if errors.Is(e, clients.ErrNotFound) {
			fail(404, "not_found")
			return
		}
		if errors.Is(e, clients.ErrInvalid) {
			fail(503, "owner_contract_invalid")
			return
		}
		if e != nil {
			fail(503, "owner_unavailable")
			return
		}
		result = snapshot
	}
	a.Outcome = "AUTHORIZED"
	if !b.audit(r, a) {
		b.reply(w, correlation, 503, nil, &apiError{Code: "audit_unavailable", Message: "Request could not be completed.", Retryable: !reviewRequest})
		return
	}
	b.reply(w, correlation, 200, result, nil)
}
