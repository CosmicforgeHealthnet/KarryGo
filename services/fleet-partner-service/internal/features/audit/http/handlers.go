package http

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/audit/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Permissions guarding the routes. Deliberately two tiers: audit.read yields
// actor/action/resource/timing only; audit.read_sensitive additionally
// unlocks the before/after summaries and network metadata a record carries.
// Reading who-did-what is a broader capability than reading the redacted
// details of what changed. audit_chain.verify is separate again: it does not
// expose any record content beyond what audit.read already would (a break
// report cites record IDs and a reason, never before/after values), but it
// is still gated distinctly because "prove the log has not been tampered
// with" is a compliance action of its own, not implied by ordinary read
// access -- see migration 000069 for the permission grant (rol_system_owner,
// rol_system_compliance).
const (
	PermissionAuditRead          = "audit.read"
	PermissionAuditReadSensitive = "audit.read_sensitive"
	PermissionAuditVerifyChain   = "security.audit_chain.verify"
)

// Handler serves the corporate audit-log read routes.
type Handler struct {
	repo       *repositories.Repository
	authorizer *accesscontrol.Authorizer
}

// NewHandler builds the handler. repo may be nil in deployments where the
// pool is unavailable; routes then answer unavailable rather than panicking.
func NewHandler(repo *repositories.Repository, authorizer *accesscontrol.Authorizer) *Handler {
	return &Handler{repo: repo, authorizer: authorizer}
}

// RegisterTenantRoutes mounts the audit-log read surface.
func (h *Handler) RegisterTenantRoutes(group *gin.RouterGroup) {
	group.GET("/audit-logs", h.list)
	group.GET("/audit-logs/:auditId", h.get)
	group.POST("/audit/verify-chain", h.verifyChain)
}

func requireTenant(c *gin.Context) (tenant.Context, bool) {
	tenantCtx, ok := middleware.TenantFromGin(c)
	if !ok {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return tenant.Context{}, false
	}
	return tenantCtx, true
}

// authorize enforces a permission for the caller's membership. It returns
// whether the caller additionally holds audit.read_sensitive, so a single
// authorization pass decides both admission and field visibility.
func (h *Handler) authorize(c *gin.Context, t tenant.Context, permission string) (sensitive bool, ok bool) {
	if h.authorizer == nil {
		respond.Error(c, apperrors.Forbidden("You do not have permission to perform this action.", nil))
		return false, false
	}
	if err := h.authorizer.Require(c.Request.Context(), t.OrganizationID, t.OrganizationUserID,
		roleVersionFrom(c), permission, accesscontrol.Target{}); err != nil {
		respond.Error(c, err)
		return false, false
	}
	sensitiveErr := h.authorizer.Require(c.Request.Context(), t.OrganizationID, t.OrganizationUserID,
		roleVersionFrom(c), PermissionAuditReadSensitive, accesscontrol.Target{})
	return sensitiveErr == nil, true
}

func roleVersionFrom(c *gin.Context) int64 {
	if value, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		if version, ok := value.(int64); ok {
			return version
		}
	}
	return 1
}

// SummaryView is the audit-log list row. It never carries before/after
// summaries or network metadata — those are sensitive-tier-only and only
// ever served from the detail route, never bulk-listed.
type SummaryView struct {
	ID           string    `json:"id"`
	BranchID     string    `json:"branchId,omitempty"`
	ActorType    string    `json:"actorType"`
	ActorID      string    `json:"actorId"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resourceType"`
	ResourceID   string    `json:"resourceId"`
	Reason       string    `json:"reason,omitempty"`
	RequestID    string    `json:"requestId"`
	HighRisk     bool      `json:"highRisk"`
	OccurredAt   time.Time `json:"occurredAt"`
}

func newSummaryView(r repositories.Record) SummaryView {
	return SummaryView{
		ID: r.ID, BranchID: r.BranchID, ActorType: r.ActorType, ActorID: r.ActorID,
		Action: r.Action, ResourceType: r.ResourceType, ResourceID: r.ResourceID,
		Reason: r.Reason, RequestID: r.RequestID, HighRisk: repositories.IsHighRisk(r.Action),
		OccurredAt: r.OccurredAt,
	}
}

// DetailView is the audit-log detail response. PreviousSummary, NewSummary
// and NetworkMeta are populated only for callers holding
// audit.read_sensitive; a caller with only audit.read gets everything
// SummaryView has, plus CorrelationID, and nothing more.
type DetailView struct {
	SummaryView
	CorrelationID   string            `json:"correlationId"`
	PreviousSummary map[string]any    `json:"previousSummary,omitempty"`
	NewSummary      map[string]any    `json:"newSummary,omitempty"`
	NetworkMeta     map[string]string `json:"networkMeta,omitempty"`
}

func newDetailView(r repositories.Record, sensitive bool) DetailView {
	view := DetailView{SummaryView: newSummaryView(r), CorrelationID: r.CorrelationID}
	if sensitive {
		view.PreviousSummary = r.PreviousSummary
		view.NewSummary = r.NewSummary
		view.NetworkMeta = r.NetworkMeta
	}
	return view
}

func (h *Handler) list(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok {
		return
	}
	_, ok = h.authorize(c, t, PermissionAuditRead)
	if !ok {
		return
	}
	if h.repo == nil {
		respond.Error(c, apperrors.Unavailable("Audit log access is not available.", nil))
		return
	}

	filter := repositories.ListFilter{
		OrganizationID:     t.OrganizationID,
		PermittedBranchIDs: t.PermittedBranchIDs,
		BranchID:           strings.TrimSpace(c.Query("branchId")),
		ActorType:          strings.TrimSpace(c.Query("actorType")),
		ActorID:            strings.TrimSpace(c.Query("actorId")),
		Action:             strings.TrimSpace(c.Query("action")),
		ResourceType:       strings.TrimSpace(c.Query("resourceType")),
		ResourceID:         strings.TrimSpace(c.Query("resourceId")),
		RequestID:          strings.TrimSpace(c.Query("requestId")),
		CorrelationID:      strings.TrimSpace(c.Query("correlationId")),
		HighRiskOnly:       c.Query("highRiskOnly") == "true",
	}

	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			respond.Error(c, apperrors.BadRequest("limit must be a non-negative integer.", nil))
			return
		}
		filter.Limit = parsed
	}

	from, to, ok := dateRangeFrom(c)
	if !ok {
		return
	}
	filter.From, filter.To = from, to

	if raw := strings.TrimSpace(c.Query("cursor")); raw != "" {
		occurredAt, id, err := decodeCursor(raw)
		if err != nil {
			respond.Error(c, apperrors.BadRequest("cursor is invalid.", nil))
			return
		}
		filter.BeforeOccurredAt, filter.BeforeID = occurredAt, id
	}

	records, nextCursor, err := h.repo.List(c.Request.Context(), filter)
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]SummaryView, 0, len(records))
	for _, record := range records {
		views = append(views, newSummaryView(record))
	}
	respond.List(c, views, nextCursor)
}

// verifyChainRequest is the optional request body. month defaults to the
// current UTC calendar month when omitted -- the vast majority of
// verification calls are "is this month's chain intact", and requiring a
// caller to always spell that out would be needless friction.
type verifyChainRequest struct {
	Month string `json:"month"`
}

// ChainBreakView mirrors audit.ChainBreak for the wire response.
type ChainBreakView struct {
	RecordID string `json:"recordId"`
	Reason   string `json:"reason"`
}

// VerifyChainView mirrors audit.VerifyChainResult for the wire response.
// Never carries record content -- only IDs, counts and break reasons, so
// this route's response is safe to return to anyone holding
// audit_chain.verify without ALSO requiring audit.read_sensitive.
type VerifyChainView struct {
	Month          string           `json:"month"`
	EntriesChecked int              `json:"entriesChecked"`
	LegacyEntries  int              `json:"legacyEntries"`
	Valid          bool             `json:"valid"`
	Breaks         []ChainBreakView `json:"breaks"`
}

func newVerifyChainView(month string, result audit.VerifyChainResult) VerifyChainView {
	breaks := make([]ChainBreakView, 0, len(result.Breaks))
	for _, b := range result.Breaks {
		breaks = append(breaks, ChainBreakView{RecordID: b.RecordID, Reason: b.Reason})
	}
	return VerifyChainView{
		Month: month, EntriesChecked: result.EntriesChecked, LegacyEntries: result.LegacyEntries,
		Valid: result.Valid(), Breaks: breaks,
	}
}

// verifyChain recomputes the tamper-evident hash chain (internal/platform/
// audit's chain.go) for the caller's own organization and a single calendar
// month, and reports whether it is intact. Scoped strictly to
// t.OrganizationID from the resolved tenant context -- never a
// caller-supplied organization id -- so this route can never be used to
// probe another organization's chain.
func (h *Handler) verifyChain(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok {
		return
	}
	if _, ok := h.authorize(c, t, PermissionAuditVerifyChain); !ok {
		return
	}
	if h.repo == nil {
		respond.Error(c, apperrors.Unavailable("Audit log access is not available.", nil))
		return
	}

	var body verifyChainRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			respond.Error(c, apperrors.BadRequest("Request body is invalid.", nil))
			return
		}
	}
	month := strings.TrimSpace(body.Month)
	if month == "" {
		month = time.Now().UTC().Format("2006-01")
	}

	partition, err := audit.PartitionKey(t.OrganizationID, month)
	if err != nil {
		respond.Error(c, apperrors.BadRequest("month must be formatted YYYY-MM.", nil))
		return
	}
	result, err := h.repo.VerifyChain(c.Request.Context(), partition)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, newVerifyChainView(month, result))
}

func (h *Handler) get(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok {
		return
	}
	sensitive, ok := h.authorize(c, t, PermissionAuditRead)
	if !ok {
		return
	}
	if h.repo == nil {
		respond.Error(c, apperrors.Unavailable("Audit log access is not available.", nil))
		return
	}
	record, err := h.repo.Get(c.Request.Context(), t.OrganizationID, c.Param("auditId"))
	if err != nil {
		respond.Error(c, apperrors.NotFound("Audit record not found.", err))
		return
	}
	respond.OK(c, newDetailView(record, sensitive))
}

// dateRangeFrom parses and bounds the from/to query parameters. A caller may
// omit both (unbounded newest-first paging), but supplying either requires
// both, and the span may not exceed repositories.MaxDateRange — an audit
// query is not allowed to scan an organization's entire history in one call.
func dateRangeFrom(c *gin.Context) (from, to time.Time, ok bool) {
	rawFrom := strings.TrimSpace(c.Query("from"))
	rawTo := strings.TrimSpace(c.Query("to"))
	if rawFrom == "" && rawTo == "" {
		return time.Time{}, time.Time{}, true
	}
	if rawFrom == "" || rawTo == "" {
		respond.Error(c, apperrors.BadRequest("from and to must be supplied together.", nil))
		return time.Time{}, time.Time{}, false
	}
	from, err := time.Parse(time.RFC3339, rawFrom)
	if err != nil {
		respond.Error(c, apperrors.BadRequest("from must be an RFC3339 timestamp.", nil))
		return time.Time{}, time.Time{}, false
	}
	to, err = time.Parse(time.RFC3339, rawTo)
	if err != nil {
		respond.Error(c, apperrors.BadRequest("to must be an RFC3339 timestamp.", nil))
		return time.Time{}, time.Time{}, false
	}
	if to.Before(from) {
		respond.Error(c, apperrors.BadRequest("to must not be before from.", nil))
		return time.Time{}, time.Time{}, false
	}
	if to.Sub(from) > repositories.MaxDateRange {
		respond.Error(c, apperrors.BadRequest("date range must not exceed 90 days.", nil))
		return time.Time{}, time.Time{}, false
	}
	return from, to, true
}

func decodeCursor(raw string) (time.Time, string, error) {
	parts := strings.SplitN(raw, "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", apperrors.BadRequest("cursor is invalid.", nil)
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", err
	}
	return occurredAt, parts[1], nil
}
