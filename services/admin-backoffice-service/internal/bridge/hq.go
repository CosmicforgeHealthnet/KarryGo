package bridge

import (
	"errors"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/services/admin-backoffice-service/internal/clients"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/hqevents"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/hqops"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/persistence"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/security"
	"cosmicforge/logistics/shared/go/admincampaign"
	"cosmicforge/logistics/shared/go/adminmedia"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"github.com/google/uuid"
)

// hqRoute binds each HQ department capability to the owner and source
// contract the signed policy rule must name (Sections 22-30).
type hqRoute struct {
	Owner, Contract string
	Write           bool
}

var hqRoutes = map[string]hqRoute{
	"hq.events.feed":        {"admin-backoffice-service", "karryam.admin-events.v1", false},
	"hq.events.ack":         {"admin-backoffice-service", "karryam.admin-events.v1", false},
	"hq.projections.status": {"admin-backoffice-service", "karryam.admin-events.v1", false},
	"hq.support.queue":      {"support-dispute-service", "karryam.support-queue.v1", false},
	"hq.approvals.propose":  {"admin-backoffice-service", "karryam.hq-approval.v1", true},
	"hq.approvals.read":     {"admin-backoffice-service", "karryam.hq-approval.v1", false},
	"hq.approvals.decide":   {"admin-backoffice-service", "karryam.hq-approval.v1", true},
	"hq.campaigns.submit":   {admincampaign.Owner, admincampaign.Contract, true},
	"hq.campaigns.readback": {admincampaign.Owner, admincampaign.Contract, false},
	"hq.campaigns.cancel":   {admincampaign.Owner, admincampaign.Contract, true},
	"hq.finance.report":     {"payment-wallet-service", "karryam.finance-report.v1", false},
	"hq.privacy.register":   {"admin-backoffice-service", "karryam.privacy-register.v1", true},
	"hq.privacy.read":       {"admin-backoffice-service", "karryam.privacy-register.v1", false},
	"hq.privacy.fulfilment": {"admin-backoffice-service", "karryam.privacy-register.v1", true},
	"hq.exports.create":     {"admin-backoffice-service", "karryam.controlled-export.v1", true},
	"hq.exports.download":   {"admin-backoffice-service", "karryam.controlled-export.v1", false},
}

// HQFlags are KarryGo-owned rollout switches; HQ cannot supply them.
type HQFlags struct {
	Events, SupportQueue, Approvals, Campaigns, Finance, Privacy, Exports, MediaMapping bool
}

func (b *Bridge) ConfigureHQ(events *hqevents.Store, ops *hqops.Service, policy *adminpolicy.Engine, flags HQFlags) error {
	if b.hqEvents != nil || b.hqOps != nil {
		return errors.New("duplicate HQ configuration")
	}
	if events == nil || ops == nil {
		return errors.New("HQ event store and orchestration required")
	}
	b.hqEvents, b.hqOps, b.hqPolicy, b.hqFlags = events, ops, policy, flags
	return nil
}

func (b *Bridge) hqEnabled(id string) bool {
	if b.hqEvents == nil || b.hqOps == nil {
		return false
	}
	f := b.hqFlags
	switch {
	case strings.HasPrefix(id, "hq.events.") || id == "hq.projections.status":
		return f.Events
	case id == "hq.support.queue":
		return f.SupportQueue
	case strings.HasPrefix(id, "hq.approvals."):
		return f.Approvals
	case strings.HasPrefix(id, "hq.campaigns."):
		return f.Campaigns
	case id == "hq.finance.report":
		return f.Finance
	case strings.HasPrefix(id, "hq.privacy."):
		return f.Privacy
	case strings.HasPrefix(id, "hq.exports."):
		return f.Exports
	case id == "hq.media.legacy-mapping":
		return f.MediaMapping && b.mediaCases != nil
	}
	return false
}

// hqAvailable feeds truthful capability discovery.
func (b *Bridge) hqAvailable(permission string, actor security.Claims) bool {
	for id, route := range hqRoutes {
		if hqPermission(id) != permission {
			continue
		}
		if !b.hqEnabled(id) || b.hqPolicy == nil || (route.Write && !b.features.RemoteWrites) {
			return false
		}
		rule, err := b.hqPolicy.Rule(permission, b.now())
		return err == nil && rule.Owner == route.Owner && rule.SourceContract == route.Contract && hasRole(actor.Roles, rule.Roles) && len(actor.Zones) > 0 && len(actor.Tenants) > 0
	}
	return false
}

func hqPermission(id string) string { return id }

type hqBody struct {
	ApprovalID      string                `json:"approval_id"`
	Kind            string                `json:"kind"`
	CampaignID      string                `json:"campaign_id"`
	Revision        int64                 `json:"revision"`
	IntentDigest    string                `json:"intent_digest"`
	Decision        string                `json:"decision"`
	Intent          *admincampaign.Intent `json:"intent"`
	CancellationKey string                `json:"cancellation_key"`
	ReasonCode      string                `json:"reason_code"`
	Consumer        string                `json:"consumer"`
	Sequence        int64                 `json:"sequence"`
	Timezone        string                `json:"timezone"`
	RequestID       string                `json:"request_id"`
	SubjectKind     string                `json:"subject_kind"`
	SubjectRef      string                `json:"subject_ref"`
	RequestType     string                `json:"request_type"`
	ReceivedOn      string                `json:"received_on"`
	LegalHold       bool                  `json:"legal_hold"`
	Owner           string                `json:"owner"`
	OperationID     string                `json:"operation_id"`
	RowLimit        int                   `json:"row_limit"`
	Token           string                `json:"token"`
	LegacyAssetID   string                `json:"legacy_asset_id"`
}

func pathID(path, prefix, suffix string) (string, bool) {
	id := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	parsed, err := uuid.Parse(id)
	return id, err == nil && parsed.String() == id
}

func (b *Bridge) serveHQ(w http.ResponseWriter, r *http.Request, body []byte, actor security.Claims, a persistence.Audit, q adminread.Query, id, path, correlation string) {
	fail := func(status int, code string) {
		a.Outcome = code
		if !b.audit(r, a) {
			status, code = 503, "audit_unavailable"
		}
		b.reply(w, correlation, status, nil, &apiError{Code: code, Message: "Request could not be completed.", Retryable: status == 503 && code != "capability_blocked"})
	}
	route := hqRoutes[id]
	if !b.hqEnabled(id) {
		fail(503, "capability_blocked")
		return
	}
	if route.Write && !b.features.RemoteWrites {
		fail(503, "remote_writes_disabled")
		return
	}
	var in hqBody
	if r.Method == http.MethodPost {
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/json" {
			fail(415, "json_required")
			return
		}
		if security.Strict(body, &in) != nil {
			fail(400, "invalid_command")
			return
		}
	}
	scope := hqops.Scope{Market: q.Scope.Market, Zone: q.Scope.Zone, Organization: q.Scope.Organization, Tenant: q.Scope.Tenant}
	evScope := hqevents.Scope(scope)
	ctx := r.Context()
	var result any
	var err error
	switch id {
	case "hq.events.feed":
		after := int64(0)
		if q.Cursor != "" {
			after, err = strconv.ParseInt(q.Cursor, 10, 64)
			if err != nil {
				fail(400, "invalid_cursor")
				return
			}
		}
		result, err = b.hqEvents.Feed(ctx, after, q.Limit, evScope)
	case "hq.events.ack":
		err = b.hqEvents.Ack(ctx, in.Consumer, in.Sequence, actor.Subject)
		result = map[string]any{"consumer": in.Consumer, "acknowledged_sequence": in.Sequence}
	case "hq.projections.status":
		result, err = b.hqEvents.Status(ctx, 15*time.Minute)
	case "hq.support.queue":
		result, err = b.hqEvents.SupportQueue(ctx, evScope, "", "", q.Cursor, q.Limit)
	case "hq.approvals.propose":
		subject, digest := "", in.IntentDigest
		switch in.Kind {
		case "CAMPAIGN":
			if in.Intent == nil {
				fail(400, "invalid_command")
				return
			}
			subject, digest = hqops.CampaignSubject(in.Intent.CampaignID, in.Intent.Revision), campaignDigest(*in.Intent, actor.Subject)
		case "EXPORT":
			subject, digest = hqops.ExportSubject("SUPPORT_CASE_QUEUE", q.Start, q.End, in.RowLimit, scope)
		default:
			fail(400, "invalid_command")
			return
		}
		a.TargetRef = in.ApprovalID
		result, err = b.hqOps.ProposeApproval(ctx, actor.Subject, q.Purpose, scope, in.ApprovalID, in.Kind, subject, digest)
	case "hq.approvals.read":
		approvalID, ok := pathID(path, "/hq/approvals/", "")
		if !ok {
			fail(400, "invalid_reference")
			return
		}
		result, err = b.hqOps.ReadApproval(ctx, approvalID, scope)
	case "hq.approvals.decide":
		approvalID, ok := pathID(path, "/hq/approvals/", "/decisions")
		if !ok || (in.Decision != "APPROVE" && in.Decision != "REJECT") {
			fail(400, "invalid_command")
			return
		}
		a.TargetRef = approvalID
		result, err = b.hqOps.Decide(ctx, approvalID, actor.Subject, scope, in.Decision == "APPROVE")
	case "hq.campaigns.submit":
		if in.Intent == nil {
			fail(400, "invalid_command")
			return
		}
		a.TargetRef = in.Intent.CampaignID
		result, err = b.hqOps.SubmitCampaign(ctx, actor.Subject, scope, actor.Markets, *in.Intent, in.ApprovalID)
	case "hq.campaigns.readback":
		opID, ok := pathID(path, "/hq/campaign-operations/", "")
		if !ok {
			fail(400, "invalid_reference")
			return
		}
		a.TargetRef = opID
		result, err = b.hqOps.CampaignReadback(ctx, opID)
	case "hq.campaigns.cancel":
		opID, ok := pathID(path, "/hq/campaign-operations/", "/cancellations")
		if !ok || !security.Opaque(in.CancellationKey) || !security.Opaque(in.ReasonCode) {
			fail(400, "invalid_command")
			return
		}
		a.TargetRef = opID
		result, err = b.hqOps.CancelCampaign(ctx, actor.Subject, opID, in.CancellationKey, in.ReasonCode)
	case "hq.finance.report":
		result, err = b.hqOps.FinanceReport(ctx, q.Start, q.End, in.Timezone)
	case "hq.privacy.register":
		a.TargetRef = in.RequestID
		result, err = b.hqOps.RegisterPrivacyRequest(ctx, actor.Subject, in.RequestID, in.SubjectKind, in.SubjectRef, in.RequestType, in.ReceivedOn, in.LegalHold)
	case "hq.privacy.read":
		requestID, ok := pathID(path, "/hq/privacy-requests/", "")
		if !ok {
			fail(400, "invalid_reference")
			return
		}
		result, err = b.hqOps.PrivacyRegister(ctx, requestID)
	case "hq.privacy.fulfilment":
		requestID, ok := pathID(path, "/hq/privacy-requests/", "/fulfilments")
		if !ok {
			fail(400, "invalid_reference")
			return
		}
		a.TargetRef = requestID
		result, err = b.hqOps.RecordOwnerFulfilment(ctx, actor.Subject, requestID, in.Owner, in.OperationID)
	case "hq.exports.create":
		a.TargetRef = in.ApprovalID
		result, err = b.hqOps.CreateExport(ctx, actor.Subject, q.Purpose, scope, q.Start, q.End, in.RowLimit, in.ApprovalID)
	case "hq.exports.download":
		exportID, ok := pathID(path, "/hq/exports/", "/download")
		if !ok {
			fail(400, "invalid_reference")
			return
		}
		a.TargetRef = exportID
		plain, e := b.hqOps.DownloadExport(ctx, actor.Subject, exportID, in.Token)
		if e == nil {
			a.Outcome = "AUTHORIZED"
			if !b.audit(r, a) {
				b.reply(w, correlation, 503, nil, &apiError{Code: "audit_unavailable", Message: "Request could not be completed.", Retryable: true})
				return
			}
			w.Header().Set("Content-Type", "text/csv")
			w.Header().Set("Content-Disposition", `attachment; filename="karryam-support-export.csv"`)
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Correlation-ID", correlation)
			w.WriteHeader(200)
			_, _ = w.Write(plain)
			return
		}
		err = e
	case "hq.media.legacy-mapping":
		a.TargetRef = in.LegacyAssetID
		mapping, e := b.mediaCases.LegacyMapping(ctx, in.LegacyAssetID)
		switch {
		case errors.Is(e, adminmedia.ErrInvalid):
			fail(400, "invalid_command")
			return
		case e != nil && strings.Contains(e.Error(), "not found"):
			fail(404, "not_found")
			return
		case e != nil:
			fail(503, "owner_unavailable")
			return
		}
		result = mapping
	default:
		fail(404, "not_found")
		return
	}
	if err != nil {
		status, code := 503, "operation_unavailable"
		switch {
		case errors.Is(err, hqops.ErrDisabled):
			code = "capability_blocked"
		case errors.Is(err, hqops.ErrInvalid), errors.Is(err, hqevents.ErrInvalid), errors.Is(err, clients.ErrInvalid):
			status, code = 400, "invalid_command"
		case errors.Is(err, hqops.ErrApproval):
			status, code = 409, "approval_unusable"
		case errors.Is(err, hqops.ErrConflict), errors.Is(err, hqevents.ErrCursor):
			status, code = 409, "conflict"
		case errors.Is(err, hqops.ErrNotFound):
			status, code = 404, "not_found"
		case errors.Is(err, hqops.ErrDenied):
			status, code = 403, "forbidden"
		case errors.Is(err, hqops.ErrExpired):
			status, code = 410, "expired"
		case errors.Is(err, clients.ErrOwnerRefused):
			status, code = 422, "owner_refused"
		case errors.Is(err, clients.ErrUnavailable):
			code = "owner_unavailable"
		}
		fail(status, code)
		return
	}
	a.Outcome = "AUTHORIZED"
	if !b.audit(r, a) {
		b.reply(w, correlation, 503, nil, &apiError{Code: "audit_unavailable", Message: "Request could not be completed.", Retryable: true})
		return
	}
	b.reply(w, correlation, 200, result, nil)
}

// campaignDigest is the digest the approver approves: the Bridge fixes the
// requester so the later submission's digest matches exactly.
func campaignDigest(in admincampaign.Intent, requester string) string {
	in.RequestedBy = requester
	return in.Digest()
}
