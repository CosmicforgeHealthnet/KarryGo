// Package http exposes Section 28's executable-retention administration
// (internal/platform/retentionexec) as tenant-scoped routes. Deliberately
// thin: every real rule (legal hold, minimum retention, which classes are
// even executable) lives in retentionexec itself, not here -- this package
// is DTO translation and permission/step-up enforcement only, the same
// shape every other Section 28 HTTP package in this pass already takes.
package http

import (
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/auth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/retentionexec"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// PermissionRetentionExecute gates both routes -- initiating a DRY_RUN
// preview and an EXECUTE run both require it: a preview is still a real
// operation over potentially sensitive record identifiers, not a
// no-permission-needed read.
const PermissionRetentionExecute = "privacy.retention.execute"

// Handler serves the retention-execution routes.
type Handler struct {
	admin      *retentionexec.Administrator
	authorizer *accesscontrol.Authorizer
}

// NewHandler builds the handler. admin may be nil in a deployment that has
// not wired retention execution (FLEET_PARTNER_RETENTION_EXECUTION_ENABLED
// unset) -- routes then answer unavailable rather than panicking, matching
// every other optional-dependency convention in this service.
func NewHandler(admin *retentionexec.Administrator, authorizer *accesscontrol.Authorizer) *Handler {
	return &Handler{admin: admin, authorizer: authorizer}
}

// RegisterTenantRoutes mounts the retention-execution surface.
func (h *Handler) RegisterTenantRoutes(group *gin.RouterGroup) {
	group.POST("/retention-runs", h.initiate)
	group.GET("/retention-runs/:runId", h.get)
	group.GET("/retention-runs/:runId/records", h.listRecords)
}

func requireTenant(c *gin.Context) (tenant.Context, bool) {
	t, ok := middleware.TenantFromGin(c)
	if !ok {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return tenant.Context{}, false
	}
	return t, true
}

func (h *Handler) authorize(c *gin.Context, t tenant.Context) bool {
	if h.authorizer == nil {
		respond.Error(c, apperrors.Forbidden("You do not have permission to perform this action.", nil))
		return false
	}
	if err := h.authorizer.Require(c.Request.Context(), t.OrganizationID, t.OrganizationUserID,
		roleVersionFrom(c), PermissionRetentionExecute, accesscontrol.Target{}); err != nil {
		respond.Error(c, err)
		return false
	}
	return true
}

func roleVersionFrom(c *gin.Context) int64 {
	if value, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		if version, ok := value.(int64); ok {
			return version
		}
	}
	return 1
}

// sessionIDFrom resolves the caller's session ID from the verified access
// token claims -- tenant.Context itself does not carry SessionID (see
// legalhold/http's identical helper, which this mirrors exactly). Needed so
// a step-up grant obtained for one session cannot be presented by a request
// authenticated with a different one.
func sessionIDFrom(c *gin.Context) string {
	claims, ok := auth.ClaimsFromContext(c)
	if !ok {
		return ""
	}
	return claims.SessionID
}

type initiateRequest struct {
	RetentionClass  string `json:"retentionClass"`
	Mode            string `json:"mode"`
	IdempotencyKey  string `json:"idempotencyKey"`
	StepUpReference string `json:"stepUpReference"`
}

// RunView mirrors retentionexec.Run for the wire response -- counts and
// status only, never a record identifier or field value. Section 28
// requires "preview/count endpoints expose no sensitive values"; this
// struct is how that is enforced structurally rather than by convention --
// there is no field here a future change could accidentally populate with
// PII, because the underlying data (organization contact fields, in this
// pass's one executor) is never read into a Run in the first place.
type RunView struct {
	ID                string `json:"id"`
	RetentionClass    string `json:"retentionClass"`
	Mode              string `json:"mode"`
	Status            string `json:"status"`
	RecordsConsidered int    `json:"recordsConsidered"`
	RecordsArchived   int    `json:"recordsArchived"`
	RecordsAnonymized int    `json:"recordsAnonymized"`
	RecordsSkipped    int    `json:"recordsSkipped"`
	RecordsFailed     int    `json:"recordsFailed"`
	ErrorMessage      string `json:"errorMessage,omitempty"`
}

func newRunView(run retentionexec.Run) RunView {
	return RunView{
		ID: run.ID, RetentionClass: run.RetentionClass, Mode: run.Mode, Status: run.Status,
		RecordsConsidered: run.RecordsConsidered, RecordsArchived: run.RecordsArchived,
		RecordsAnonymized: run.RecordsAnonymized, RecordsSkipped: run.RecordsSkipped,
		RecordsFailed: run.RecordsFailed, ErrorMessage: run.ErrorMessage,
	}
}

func (h *Handler) initiate(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t) {
		return
	}
	if h.admin == nil {
		respond.Error(c, apperrors.Unavailable("Retention execution is not available.", nil))
		return
	}
	var req initiateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}
	run, err := h.admin.Initiate(c.Request.Context(), retentionexec.InitiateInput{
		Actor: retentionexec.Actor{
			OrganizationID: t.OrganizationID, ActorType: string(t.ActorType), ActorID: t.OrganizationUserID,
		},
		IdentityID: t.ActorID, SessionID: sessionIDFrom(c),
		StepUpReference: strings.TrimSpace(req.StepUpReference),
		RetentionClass:  req.RetentionClass, Mode: req.Mode, IdempotencyKey: req.IdempotencyKey,
		RequestID: t.RequestID, CorrelationID: t.CorrelationID,
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, newRunView(run))
}

func (h *Handler) get(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t) {
		return
	}
	if h.admin == nil {
		respond.Error(c, apperrors.Unavailable("Retention execution is not available.", nil))
		return
	}
	run, err := h.admin.Get(c.Request.Context(), t.OrganizationID, c.Param("runId"))
	if err != nil {
		respond.Error(c, apperrors.NotFound("Retention run not found.", err))
		return
	}
	respond.OK(c, newRunView(run))
}

// RecordView is one immutable per-candidate outcome for the operator detail
// view -- record TYPE and ID and a short human-reason detail, never a field
// value from the underlying table.
type RecordView struct {
	RecordType string `json:"recordType"`
	RecordID   string `json:"recordId"`
	Outcome    string `json:"outcome"`
	Detail     string `json:"detail"`
}

func (h *Handler) listRecords(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t) {
		return
	}
	if h.admin == nil {
		respond.Error(c, apperrors.Unavailable("Retention execution is not available.", nil))
		return
	}
	run, err := h.admin.Get(c.Request.Context(), t.OrganizationID, c.Param("runId"))
	if err != nil {
		respond.Error(c, apperrors.NotFound("Retention run not found.", err))
		return
	}
	records, err := h.admin.ListRecords(c.Request.Context(), run.ID)
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]RecordView, 0, len(records))
	for _, r := range records {
		views = append(views, RecordView{RecordType: r.RecordType, RecordID: r.RecordID, Outcome: r.Outcome, Detail: r.Detail})
	}
	respond.List(c, views, "")
}
