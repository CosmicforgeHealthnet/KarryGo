package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

// Handler serves the corporate (Fleet manager) marketplace endpoints.
type Handler struct {
	service     *usecases.Service
	authorizer  *accesscontrol.Authorizer
	idempotency *idempotency.Store
}

// NewHandler constructs the corporate marketplace handler.
func NewHandler(service *usecases.Service, authorizer *accesscontrol.Authorizer, idem *idempotency.Store) *Handler {
	return &Handler{
		service:     service,
		authorizer:  authorizer,
		idempotency: idem,
	}
}

// ---------------------------------------------------------------------------
// Gate and bind helpers
// ---------------------------------------------------------------------------

func (h *Handler) gate(c *gin.Context, permission string) (tenant.Context, bool) {
	t, ok := middleware.TenantFromGin(c)
	if !ok {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return t, false
	}
	if h.authorizer == nil {
		respond.Error(c, apperrors.Forbidden("Permission denied.", nil))
		return t, false
	}
	roleVersion := int64(1)
	if raw, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		roleVersion, _ = raw.(int64)
	}
	target := accesscontrol.Target{BranchID: c.Query("branchId")}
	if err := h.authorizer.Require(c, t.OrganizationID, t.OrganizationUserID, roleVersion, permission, target); err != nil {
		respond.Error(c, err)
		return t, false
	}
	return t, true
}

func bindJSON(c *gin.Context, v any) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", err))
		return false
	}
	return true
}

func expectedVersion(c *gin.Context, bodyVersion int64) int64 {
	if bodyVersion > 0 {
		return bodyVersion
	}
	raw := strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`)
	v, _ := strconv.ParseInt(raw, 10, 64)
	return v
}

func parseCorporatePage(c *gin.Context) usecases.CorporatePageFilter {
	limit, _ := strconv.Atoi(c.Query("limit"))
	return usecases.CorporatePageFilter{
		Status:          strings.ToUpper(strings.TrimSpace(c.Query("status"))),
		BranchID:        strings.TrimSpace(c.Query("branchId")),
		Limit:           limit,
		CursorSortValue: strings.TrimSpace(c.Query("cursorSortValue")),
		CursorLastID:    strings.TrimSpace(c.Query("cursorLastId")),
	}
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

// ListOffers returns the organization's open marketplace offers.
// GET /marketplace/offers
func (h *Handler) ListOffers(c *gin.Context) {
	t, ok := h.gate(c, PermissionMarketplaceRead)
	if !ok {
		return
	}
	offers, err := h.service.ListCorporateOffers(c.Request.Context(), t, parseCorporatePage(c))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, offers)
}

// GetOffer returns one offer with its projection.
// GET /marketplace/offers/:offerId
func (h *Handler) GetOffer(c *gin.Context) {
	t, ok := h.gate(c, PermissionMarketplaceRead)
	if !ok {
		return
	}
	offer, err := h.service.GetCorporateOffer(c.Request.Context(), t, c.Param("offerId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, offer)
}

// GetOfferMetrics returns the four counters at the top of the offers list.
// GET /marketplace/metrics
func (h *Handler) GetOfferMetrics(c *gin.Context) {
	t, ok := h.gate(c, PermissionMarketplaceRead)
	if !ok {
		return
	}
	metrics, err := h.service.GetCorporateOfferMetrics(c.Request.Context(), t)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, metrics)
}

// ListCandidates returns eligible providers and assets for a request.
// GET /marketplace/requests/:projectionId/candidates
func (h *Handler) ListCandidates(c *gin.Context) {
	t, ok := h.gate(c, PermissionMarketplaceRead)
	if !ok {
		return
	}
	candidates, err := h.service.ListCorporateCandidates(c.Request.Context(), t, c.Param("projectionId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, candidates)
}

// ListAssignments returns the organization's assignments.
// GET /marketplace/assignments
func (h *Handler) ListAssignments(c *gin.Context) {
	t, ok := h.gate(c, PermissionMarketplaceRead)
	if !ok {
		return
	}
	assignments, err := h.service.ListCorporateAssignments(c.Request.Context(), t, parseCorporatePage(c))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, assignments)
}

// GetAssignment returns one assignment.
// GET /marketplace/assignments/:assignmentId
func (h *Handler) GetAssignment(c *gin.Context) {
	t, ok := h.gate(c, PermissionMarketplaceRead)
	if !ok {
		return
	}
	assignment, err := h.service.GetCorporateAssignment(c.Request.Context(), t, c.Param("assignmentId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, assignment)
}

// ListAssignmentHistory returns the append-only state history for an assignment.
// GET /marketplace/assignments/:assignmentId/history
func (h *Handler) ListAssignmentHistory(c *gin.Context) {
	t, ok := h.gate(c, PermissionMarketplaceRead)
	if !ok {
		return
	}
	history, err := h.service.ListCorporateAssignmentHistory(c.Request.Context(), t, c.Param("assignmentId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, history)
}

// ---------------------------------------------------------------------------
// Writes (idempotency required)
// ---------------------------------------------------------------------------

type acceptOfferBody struct {
	ExpectedVersion int64 `json:"expectedVersion"`
}

// AcceptOffer commits the organization to an open offer.
// POST /marketplace/offers/:offerId/accept
func (h *Handler) AcceptOffer(c *gin.Context) {
	t, ok := h.gate(c, PermissionMarketplaceAccept)
	if !ok {
		return
	}
	var body acceptOfferBody
	if !bindJSON(c, &body) {
		return
	}
	body.ExpectedVersion = expectedVersion(c, body.ExpectedVersion)
	h.command(c, t, "POST /marketplace/offers/:offerId/accept", body, func() (int, any, error) {
		result, err := h.service.AcceptCorporateOffer(c.Request.Context(), t, usecases.CorporateAcceptOfferInput{
			OfferID:         c.Param("offerId"),
			ExpectedVersion: body.ExpectedVersion,
			IdempotencyKey:  c.GetHeader("Idempotency-Key"),
		})
		return http.StatusOK, result, err
	})
}

type declineOfferBody struct {
	ReasonCode string `json:"reasonCode"`
	Note       string `json:"note"`
}

// DeclineOffer refuses an offer on behalf of the organization.
// POST /marketplace/offers/:offerId/decline
func (h *Handler) DeclineOffer(c *gin.Context) {
	t, ok := h.gate(c, PermissionMarketplaceDecline)
	if !ok {
		return
	}
	var body declineOfferBody
	if !bindJSON(c, &body) {
		return
	}
	h.command(c, t, "POST /marketplace/offers/:offerId/decline", body, func() (int, any, error) {
		err := h.service.DeclineCorporateOffer(c.Request.Context(), t, usecases.CorporateDeclineOfferInput{
			OfferID:    c.Param("offerId"),
			ReasonCode: body.ReasonCode,
			Note:       body.Note,
		})
		return http.StatusOK, gin.H{"status": "DECLINED"}, err
	})
}

type assignBody struct {
	MembershipID           string `json:"membershipId"`
	AssetReference         string `json:"assetReference"`
	ResponseTimeoutSeconds int    `json:"responseTimeoutSeconds"`
	ExpectedRequestVersion int64  `json:"expectedRequestVersion"`
}

// Assign creates a PENDING_PROVIDER_RESPONSE assignment.
// POST /marketplace/requests/:projectionId/assign
func (h *Handler) Assign(c *gin.Context) {
	t, ok := h.gate(c, PermissionMarketplaceAssign)
	if !ok {
		return
	}
	var body assignBody
	if !bindJSON(c, &body) {
		return
	}
	body.ExpectedRequestVersion = expectedVersion(c, body.ExpectedRequestVersion)
	h.command(c, t, "POST /marketplace/requests/:projectionId/assign", body, func() (int, any, error) {
		assignment, err := h.service.AssignCorporateRequest(c.Request.Context(), t, usecases.CorporateAssignInput{
			ProjectionID:           c.Param("projectionId"),
			MembershipID:           body.MembershipID,
			AssetReference:         body.AssetReference,
			ResponseTimeoutSeconds: body.ResponseTimeoutSeconds,
			ExpectedRequestVersion: body.ExpectedRequestVersion,
			IdempotencyKey:         c.GetHeader("Idempotency-Key"),
		})
		return http.StatusCreated, assignment, err
	})
}

type cancelAssignmentBody struct {
	Reason          string `json:"reason"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

// CancelAssignment cancels an assignment before execution starts.
// POST /marketplace/assignments/:assignmentId/cancel
func (h *Handler) CancelAssignment(c *gin.Context) {
	t, ok := h.gate(c, PermissionMarketplaceCancel)
	if !ok {
		return
	}
	var body cancelAssignmentBody
	if !bindJSON(c, &body) {
		return
	}
	body.ExpectedVersion = expectedVersion(c, body.ExpectedVersion)
	h.command(c, t, "POST /marketplace/assignments/:assignmentId/cancel", body, func() (int, any, error) {
		err := h.service.CancelCorporateAssignment(c.Request.Context(), t, usecases.CorporateCancelAssignmentInput{
			AssignmentID:    c.Param("assignmentId"),
			Reason:          body.Reason,
			ExpectedVersion: body.ExpectedVersion,
		})
		return http.StatusOK, gin.H{"status": "CANCELLED"}, err
	})
}

type reassignBody struct {
	MembershipID           string `json:"membershipId"`
	AssetReference         string `json:"assetReference"`
	ResponseTimeoutSeconds int    `json:"responseTimeoutSeconds"`
	Reason                 string `json:"reason"`
}

// ReassignAssignment replaces an assignment with another provider.
// POST /marketplace/assignments/:assignmentId/reassign
func (h *Handler) ReassignAssignment(c *gin.Context) {
	t, ok := h.gate(c, PermissionMarketplaceReassign)
	if !ok {
		return
	}
	var body reassignBody
	if !bindJSON(c, &body) {
		return
	}
	h.command(c, t, "POST /marketplace/assignments/:assignmentId/reassign", body, func() (int, any, error) {
		assignment, err := h.service.ReassignCorporateAssignment(c.Request.Context(), t, usecases.CorporateReassignAssignmentInput{
			AssignmentID:           c.Param("assignmentId"),
			MembershipID:           body.MembershipID,
			AssetReference:         body.AssetReference,
			ResponseTimeoutSeconds: body.ResponseTimeoutSeconds,
			Reason:                 body.Reason,
			IdempotencyKey:         c.GetHeader("Idempotency-Key"),
		})
		return http.StatusOK, assignment, err
	})
}

// ---------------------------------------------------------------------------
// Idempotent command wrapper
// ---------------------------------------------------------------------------

func (h *Handler) command(c *gin.Context, t tenant.Context, endpoint string, body any, run func() (int, any, error)) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		respond.Error(c, apperrors.BadRequest("Idempotency-Key header is required.", nil))
		return
	}
	if h.idempotency == nil {
		respond.Error(c, apperrors.Internal("Idempotency store is unavailable.", nil))
		return
	}

	raw, err := json.Marshal(body)
	if err != nil {
		respond.Error(c, apperrors.BadRequest("Request body could not be encoded.", err))
		return
	}
	fingerprint := idempotency.Fingerprint(c.Request.Method, endpoint, raw)

	record, err := h.idempotency.Begin(c.Request.Context(), t.OrganizationID, endpoint, key, fingerprint)
	if err != nil {
		respond.Error(c, idempotency.AsAppError(err))
		return
	}
	if record != nil {
		c.Data(record.ResponseStatus, "application/json; charset=utf-8", record.ResponseBody)
		return
	}

	status, data, err := run()
	var payload []byte
	if err != nil {
		if errors.Is(err, repositories.ErrNotFound) {
			err = apperrors.NotFound("Resource not found.", err)
		}
		appErr := apperrors.From(err)
		payload, _ = json.Marshal(gin.H{
			"error": respond.ErrorBody{
				Code:      appErr.Code,
				Message:   appErr.Message,
				Details:   appErr.Fields,
				RequestID: httpx.GetRequestID(c),
			},
		})
		status = appErr.Status
	} else {
		payload, _ = json.Marshal(gin.H{
			"data": data,
			"meta": gin.H{"requestId": httpx.GetRequestID(c)},
		})
	}

	if err := h.idempotency.Complete(c.Request.Context(), t.OrganizationID, endpoint, key, status, payload); err != nil {
		respond.Error(c, err)
		return
	}
	c.Data(status, "application/json; charset=utf-8", payload)
}