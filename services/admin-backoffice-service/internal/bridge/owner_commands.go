package bridge

import (
	"errors"
	"mime"
	"net/http"
	"strings"

	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/ownercommandops"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/persistence"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
)

// ConfigureOwnerCommands is startup-only. A domain with no configured service
// keeps every command, approval and readback route capability-blocked.
func (b *Bridge) ConfigureOwnerCommands(s *ownercommandops.Service) error {
	if s == nil || s.DB == nil || s.Owner == nil {
		return errors.New("invalid owner command configuration")
	}
	if b.ownerCommands == nil {
		b.ownerCommands = map[string]*ownercommandops.Service{}
	}
	domain := s.Owner.Spec().Domain
	if _, ok := admincommand.ForDomain(domain); !ok || b.ownerCommands[domain] != nil {
		return errors.New("invalid owner command configuration")
	}
	b.ownerCommands[domain] = s
	return nil
}

// ownerCommandRoute maps a matched capability to its configured domain service.
func (b *Bridge) ownerCommandRoute(capability string) (*ownercommandops.Service, string, bool) {
	parts := strings.SplitN(capability, ".command.", 2)
	if len(parts) != 2 {
		return nil, "", false
	}
	if _, ok := admincommand.ForDomain(parts[0]); !ok {
		return nil, "", false
	}
	return b.ownerCommands[parts[0]], parts[1], true
}

func (b *Bridge) ownerCommandAvailability(values map[string]bool, roles, zones, tenants []string) {
	for _, spec := range admincommand.Specs() {
		s := b.ownerCommands[spec.Domain]
		for _, permission := range []string{spec.Capability, spec.ReadCapability, spec.ApproveCapability} {
			values[permission] = false
			if s == nil || !s.ReadEnabled || s.Policy == nil || len(zones) == 0 || len(tenants) == 0 {
				continue
			}
			if permission != spec.ReadCapability && (!s.WriteEnabled || !b.features.WriteFamilyRequested(permission)) {
				continue
			}
			rule, err := s.Policy.Rule(permission, b.now())
			values[permission] = err == nil && rule.Owner == spec.Owner && rule.SourceContract == spec.Contract && hasRole(roles, rule.Roles)
		}
	}
}

// serveOwnerCommand runs after authentication, five-scope delegation, policy
// pinning and rate limiting. Failures are never advertised as retryable: the
// only recovery for a submitted command is original-identity readback.
func (b *Bridge) serveOwnerCommand(w http.ResponseWriter, r *http.Request, body []byte, s *ownercommandops.Service, route, path string, actor adminpolicy.Actor, q adminread.Query, a persistence.Audit, correlation string) {
	fail := func(status int, code string) {
		a.Outcome = code
		if !b.audit(r, a) {
			status, code = 503, "audit_unavailable"
		}
		b.reply(w, correlation, status, nil, &apiError{Code: code, Message: "Use original operation readback to resolve the outcome.", Retryable: false})
	}
	spec := s.Owner.Spec()
	segments := strings.Split(strings.TrimPrefix(path, "/"+spec.Domain+"/"), "/")
	json := func() bool {
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		return err == nil && media == "application/json" && len(r.Header.Values("Content-Type")) == 1
	}
	write := r.Method == http.MethodPost
	if write && !b.features.WriteFamilyRequested(spec.Capability) {
		fail(503, "capability_blocked")
		return
	}
	if write && !json() {
		fail(415, "json_required")
		return
	}
	var result any
	var err error
	switch route {
	case "submit":
		in, e := admincommand.DecodeSubmission(body)
		if e != nil {
			fail(400, "invalid_command")
			return
		}
		a.TargetRef, a.ReasonCode = in.SubjectID, in.ReasonCode
		var out admincommand.Operation
		out, err = s.Submit(r.Context(), actor, q, in)
		if err == nil {
			a.Status = out.Status
			if out.Receipt != nil {
				a.SourceVersion = out.Receipt.RecordedVersion
			}
		}
		result = out
	case "readback":
		a.TargetRef = segments[len(segments)-1]
		var out admincommand.Operation
		out, err = s.Reconcile(r.Context(), actor, q, segments[len(segments)-1])
		if err == nil {
			a.Status = out.Status
		}
		result = out
	case "history":
		if len(segments) != 3 || !admincommand.Segment(segments[1]) || !admincommand.Segment(segments[2]) {
			fail(400, "invalid_reference")
			return
		}
		a.TargetRef = segments[2]
		result, err = s.History(r.Context(), actor, q, segments[1], segments[2])
	case "approval.propose":
		in, e := admincommand.DecodeProposal(body)
		if e != nil {
			fail(400, "invalid_command")
			return
		}
		a.TargetRef, a.ReasonCode = in.SubjectID, in.ReasonCode
		result, err = s.Propose(r.Context(), actor, q, in)
	case "approval.read":
		a.TargetRef = segments[len(segments)-1]
		result, err = s.Approval(r.Context(), actor, q, segments[len(segments)-1])
	case "approval.decide":
		in, e := admincommand.DecodeDecision(body)
		if e != nil || len(segments) != 3 {
			fail(400, "invalid_command")
			return
		}
		a.TargetRef, a.ReasonCode = segments[1], in.ReasonCode
		result, err = s.Decide(r.Context(), actor, q, segments[1], in)
	default:
		fail(404, "not_found")
		return
	}
	if err != nil {
		status, code := 503, "operation_unavailable"
		switch {
		case errors.Is(err, ownercommandops.ErrDisabled):
			code = "capability_blocked"
		case errors.Is(err, adminpolicy.ErrBlocked):
			code = "blocked_policy"
		case errors.Is(err, adminpolicy.ErrDenied), errors.Is(err, admincommand.ErrRefused):
			status, code = 403, "forbidden"
		case errors.Is(err, ownercommandops.ErrConflict):
			status, code = 409, "idempotency_or_slot_conflict"
		case errors.Is(err, ownercommandops.ErrRemedyUnconfirmed):
			status, code = 409, "wallet_confirmation_required"
		case errors.Is(err, ownercommandops.ErrApproval):
			status, code = 409, "approval_unusable"
		case errors.Is(err, ownercommandops.ErrNotFound), errors.Is(err, admincommand.ErrAbsent):
			status, code = 404, "not_found"
		case errors.Is(err, admincommand.ErrInvalid):
			status, code = 400, "invalid_command"
		case errors.Is(err, ownercommandops.ErrProof):
			code = "owner_contract_invalid"
		}
		fail(status, code)
		return
	}
	a.Outcome = "AUTHORIZED"
	if !b.audit(r, a) {
		b.reply(w, correlation, 503, nil, &apiError{Code: "audit_unavailable", Message: "Use original operation readback to resolve the outcome.", Retryable: false})
		return
	}
	b.reply(w, correlation, 200, result, nil)
}
