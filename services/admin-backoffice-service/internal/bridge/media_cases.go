package bridge

import (
	"cosmicforge/logistics/services/admin-backoffice-service/internal/config"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/persistence"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/security"
	"cosmicforge/logistics/shared/go/admincase"
	"cosmicforge/logistics/shared/go/adminmedia"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"errors"
	"github.com/google/uuid"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const mediaRevokePermission = "media.case.grant.revoke"

type mediaCaseInput struct {
	CaseOwner        string `json:"case_owner"`
	CaseKind         string `json:"case_kind"`
	CaseReference    string `json:"case_reference"`
	SubjectType      string `json:"subject_type"`
	SubjectReference string `json:"subject_reference"`
	MediaObjectID    string `json:"media_object_id"`
}
type mediaContentInput struct {
	mediaCaseInput
	Token string `json:"token"`
}

func (b *Bridge) ConfigureMediaCases(client *adminmedia.Client, policy *adminpolicy.Engine, reads, revokes bool) error {
	if client == nil || b.mediaCases != nil || !reads && !revokes {
		return errors.New("invalid private Media configuration")
	}
	b.mediaCases, b.mediaPolicy, b.mediaReads, b.mediaRevokes = client, policy, reads, revokes
	return nil
}
func (b *Bridge) mediaCaseAvailability(values map[string]bool, roles, zones, tenants []string) {
	rule, err := b.mediaPolicy.Rule(adminmedia.Permission, b.now())
	_, disclosureErr := b.mediaPolicy.MediaDisclosure(b.now())
	values[adminmedia.Permission] = b.mediaCases != nil && b.mediaReads && err == nil && disclosureErr == nil && rule.Owner == "media-file-service" && rule.SourceContract == adminmedia.Contract && hasRole(roles, rule.Roles) && len(zones) > 0 && len(tenants) > 0
	values[mediaRevokePermission] = b.mediaCases != nil && b.mediaRevokes && len(zones) > 0 && len(tenants) > 0
}
func mediaScopeQuery(r *http.Request) (adminpolicy.Scope, string, error) {
	empty := adminpolicy.Scope{}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(values) != 5 {
		return empty, "", security.ErrDenied
	}
	for _, name := range []string{"market", "zone", "organization", "tenant", "purpose"} {
		if len(values[name]) != 1 || !security.Opaque(values.Get(name)) {
			return empty, "", security.ErrDenied
		}
	}
	return adminpolicy.Scope{Product: config.Product, Market: values.Get("market"), Zone: values.Get("zone"), Organization: values.Get("organization"), Tenant: values.Get("tenant")}, values.Get("purpose"), nil
}

// Authentication, principal/revocation checks and durable HQ replay protection
// have already run. Actor, purpose, scope and source capability are derived here;
// no request can supply a downstream actor, credential, origin or raw URL.
func (b *Bridge) serveMediaCase(w http.ResponseWriter, r *http.Request, body []byte, claims security.Claims, a persistence.Audit, capability, correlation string) {
	fail := func(status int, code string) {
		a.Outcome = code
		if !b.audit(r, a) {
			status, code = 503, "audit_unavailable"
		}
		b.reply(w, correlation, status, nil, &apiError{Code: code, Message: "Private evidence request could not be completed.", Retryable: false})
	}
	revoke := capability == "media.case.grant.revoke"
	if b.mediaCases == nil || revoke && !b.mediaRevokes || !revoke && !b.mediaReads {
		fail(503, "capability_blocked")
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(r.Header.Values("Content-Type")) != 1 {
		fail(415, "json_required")
		return
	}
	scope, purpose, err := mediaScopeQuery(r)
	if err != nil {
		fail(400, "invalid_query")
		return
	}
	a.Market, a.Zone, a.Organization, a.ReasonCode = scope.Market, scope.Zone, scope.Organization, purpose
	permission := adminmedia.Permission
	risk := security.R2
	if revoke {
		permission, risk = mediaRevokePermission, security.R1
	}
	if security.Authorize(claims, security.Policy{Permission: permission, Risk: risk, Version: b.trust.PolicyVersion}, security.Scope{Market: scope.Market, Organization: scope.Organization}, b.trust) != nil || !config.Contains(claims.Zones, scope.Zone) || !config.Contains(claims.Tenants, scope.Tenant) {
		fail(403, "forbidden")
		return
	}
	allowed, err := b.repo.Rate(r.Context(), strings.Join([]string{claims.Subject, permission, scope.Market, scope.Organization}, ":"), b.now(), b.trust.RequestsPerMinute)
	if err != nil {
		fail(503, "authorization_unavailable")
		return
	}
	if !allowed {
		w.Header().Set("Retry-After", "60")
		fail(429, "rate_limited")
		return
	}
	actor := adminmedia.Actor{Product: claims.Product, Staff: claims.Subject, Roles: claims.Roles, Permissions: claims.Permissions, Markets: claims.Markets, Zones: claims.Zones, Organizations: claims.Organizations, Tenants: claims.Tenants, ExpiresAt: time.Unix(claims.Expires, 0).UTC()}
	var result any
	var content []byte
	var request adminmedia.Request
	id := ""
	path := strings.TrimPrefix(r.URL.Path, Base+"/media/case-grants")
	if path != "" {
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
		if len(parts) != 2 {
			fail(400, "invalid_reference")
			return
		}
		id = parts[0]
		parsed, e := uuid.Parse(id)
		if e != nil || parsed == uuid.Nil || parsed.String() != id {
			fail(400, "invalid_reference")
			return
		}
	}
	var token, reason string
	if revoke {
		var input struct {
			ReasonCode string `json:"reason_code"`
		}
		if adminmedia.Decode(body, &input) != nil || !security.Opaque(input.ReasonCode) {
			fail(400, "invalid_request")
			return
		}
		reason = input.ReasonCode
		a.TargetRef = id
		a.ReasonCode = reason
	} else {
		var input mediaCaseInput
		if capability == "media.case.content.read" {
			var read mediaContentInput
			if adminmedia.Decode(body, &read) != nil {
				fail(400, "invalid_request")
				return
			}
			input, token = read.mediaCaseInput, read.Token
			if len(token) != 43 {
				fail(400, "invalid_request")
				return
			}
		} else if adminmedia.Decode(body, &input) != nil {
			fail(400, "invalid_request")
			return
		}
		spec, ok := admincase.Find(input.CaseOwner, input.CaseKind)
		if !ok || len(spec.Subjects[input.SubjectType]) == 0 {
			fail(400, "unsupported_case")
			return
		}
		request = adminmedia.Request{Scope: scope, Purpose: purpose, SourceCapability: spec.Permission, CaseOwner: spec.Owner, CaseKind: spec.Kind, CaseReference: input.CaseReference, SubjectType: input.SubjectType, SubjectReference: input.SubjectReference, MediaObjectID: input.MediaObjectID}
		if !request.Valid() {
			fail(400, "invalid_request")
			return
		}
		delegated := actor.Delegation()
		disclosure, e := b.mediaPolicy.MediaDisclosure(b.now())
		if e != nil || !config.Contains(disclosure.SourceCapabilities, spec.Permission) {
			fail(503, "blocked_policy")
			return
		}
		for _, cap := range []struct{ permission, owner, contract string }{{adminmedia.Permission, "media-file-service", adminmedia.Contract}, {spec.Permission, spec.Owner, spec.SourceContract}} {
			rule, e := b.mediaPolicy.Rule(cap.permission, b.now())
			if e != nil || rule.Owner != cap.owner || rule.SourceContract != cap.contract {
				fail(503, "blocked_policy")
				return
			}
			if e := b.mediaPolicy.AuthorizeDelegation(delegated, cap.permission, purpose, scope, b.now()); e != nil {
				if errors.Is(e, adminpolicy.ErrBlocked) {
					fail(503, "blocked_policy")
				} else {
					fail(403, "forbidden")
				}
				return
			}
		}
		a.TargetRef = request.MediaObjectID
		a.EvidenceRef = b.mediaPolicy.ID() + "@" + b.mediaPolicy.Checksum()
		if b.mediaPolicy.ExpiresAt().Before(actor.ExpiresAt) {
			actor.ExpiresAt = b.mediaPolicy.ExpiresAt()
		}
	}
	// No credential or bytes reach HQ if either audit commit fails. Native Media
	// also audits its own grant/disclosure/revocation transaction independently.
	a.Outcome = "ATTEMPT_AUTHORIZED"
	if !b.audit(r, a) {
		fail(503, "audit_unavailable")
		return
	}
	switch capability {
	case "media.case.grant.issue":
		result, err = b.mediaCases.Issue(r.Context(), actor, request)
	case "media.case.content.read":
		content, err = b.mediaCases.Read(r.Context(), actor, request, id, token)
	case "media.case.grant.revoke":
		err = b.mediaCases.Revoke(r.Context(), actor, id, reason)
		result = map[string]string{"status": "REVOKED"}
	default:
		fail(404, "not_found")
		return
	}
	if err != nil {
		status, code := 503, "owner_unavailable"
		if errors.Is(err, adminmedia.ErrDenied) {
			status, code = 403, "forbidden"
		}
		if errors.Is(err, adminmedia.ErrNotFound) {
			status, code = 404, "not_found"
		}
		if errors.Is(err, adminmedia.ErrInvalid) {
			code = "owner_contract_invalid"
		}
		fail(status, code)
		return
	}
	a.Outcome = "CONFIRMED"
	if !b.audit(r, a) {
		b.reply(w, correlation, 503, nil, &apiError{Code: "audit_unavailable", Message: "Private evidence request could not be completed.", Retryable: false})
		return
	}
	if capability == "media.case.content.read" {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="evidence"`)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Correlation-ID", correlation)
		w.Header().Set("X-Media-Case-Contract", adminmedia.Contract)
		w.Header().Set("X-Media-Object-ID", request.MediaObjectID)
		w.WriteHeader(200)
		_, _ = w.Write(content)
		return
	}
	b.reply(w, correlation, 200, result, nil)
}
