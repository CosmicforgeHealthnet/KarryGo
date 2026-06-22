package supporthttp

import (
	nethttp "net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	supportmodels "cosmicforge/logistics/services/support-dispute-service/internal/features/support/models"
	supportusecases "cosmicforge/logistics/services/support-dispute-service/internal/features/support/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

type Handler struct {
	service *supportusecases.SupportService
}

func NewHandler(service *supportusecases.SupportService) *Handler {
	return &Handler{service: service}
}

// ─── Customer-facing handlers ──────────────────────────────────────────────────

// POST /complaints
func (h *Handler) CreateComplaint(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	var req createComplaintRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Invalid request body.", err))
		return
	}

	complainantType := complainantTypeFromRole(claims.Role)

	result, err := h.service.CreateComplaint(c.Request.Context(), supportusecases.CreateComplaintInput{
		ComplainantType:  complainantType,
		ComplainantID:    claims.Subject,
		ServiceType:      supportmodels.ServiceType(req.ServiceType),
		BookingReference: req.BookingReference,
		Subject:          req.Subject,
		Description:      req.Description,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	respond(c, nethttp.StatusCreated, result.Public())
}

// GET /complaints
func (h *Handler) MyComplaints(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	complainantType := complainantTypeFromRole(claims.Role)
	results, err := h.service.MyComplaints(c.Request.Context(), complainantType, claims.Subject, limit, offset)
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	public := make([]supportmodels.PublicComplaint, len(results))
	for i, r := range results {
		public[i] = r.Public()
	}
	respondOK(c, gin.H{"complaints": public})
}

// GET /complaints/:id
func (h *Handler) GetComplaint(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	complainantType := complainantTypeFromRole(claims.Role)
	result, err := h.service.GetComplaint(c.Request.Context(), c.Param("id"), complainantType, claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result.Public())
}

// POST /complaints/:id/evidence
func (h *Handler) AddEvidence(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	var req addEvidenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Invalid request body.", err))
		return
	}

	result, err := h.service.AddEvidence(c.Request.Context(), supportusecases.AddEvidenceInput{
		ComplaintID:  c.Param("id"),
		UploaderType: complainantTypeFromRole(claims.Role),
		UploaderID:   claims.Subject,
		MediaAssetID: req.MediaAssetID,
		MediaURL:     req.MediaURL,
		Note:         req.Note,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusCreated, gin.H{
		"id":             result.ID,
		"complaint_id":   result.ComplaintID,
		"media_url":      result.MediaURL,
		"media_asset_id": result.MediaAssetID,
		"note":           result.Note,
		"created_at":     result.CreatedAt,
	})
}

// GET /complaints/:id/evidence
func (h *Handler) ListEvidence(c *gin.Context) {
	_, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	results, err := h.service.ListEvidence(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, gin.H{"evidence": results})
}

// POST /complaints/:id/dispute
func (h *Handler) EscalateToDispute(c *gin.Context) {
	_, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	var req escalateDisputeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Invalid request body.", err))
		return
	}

	result, err := h.service.EscalateToDispute(c.Request.Context(), supportusecases.EscalateToDisputeInput{
		ComplaintID:    c.Param("id"),
		RespondentType: supportmodels.ComplainantType(req.RespondentType),
		RespondentID:   req.RespondentID,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusCreated, result.Public())
}

// GET /complaints/:id/dispute
func (h *Handler) GetDispute(c *gin.Context) {
	_, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	result, err := h.service.GetDispute(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result.Public())
}

// ─── Chat handlers ────────────────────────────────────────────────────────────

// POST /support-chat/start
func (h *Handler) StartSupportChat(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	complainantType := complainantTypeFromRole(claims.Role)
	complaint, err := h.service.StartSupportChat(c.Request.Context(), complainantType, claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, complaint.Public())
}

// POST /complaints/:id/messages
func (h *Handler) SendMessage(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	var req sendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Invalid request body.", err))
		return
	}

	senderType := supportmodels.SenderType(complainantTypeFromRole(claims.Role))
	msg, err := h.service.SendChatMessage(c.Request.Context(), c.Param("id"), senderType, claims.Subject, req.Content)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusCreated, msg.Public())
}

// GET /complaints/:id/messages
func (h *Handler) ListMessages(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	senderType := supportmodels.SenderType(complainantTypeFromRole(claims.Role))
	messages, err := h.service.ListChatMessages(c.Request.Context(), c.Param("id"), senderType, claims.Subject, limit, offset)
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	public := make([]supportmodels.PublicChatMessage, len(messages))
	for i, m := range messages {
		public[i] = m.Public()
	}
	respondOK(c, gin.H{"messages": public})
}

// POST /admin/complaints/:id/messages
func (h *Handler) AdminSendMessage(c *gin.Context) {
	var req sendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Invalid request body.", err))
		return
	}
	adminID := c.GetHeader("X-Admin-ID")
	if adminID == "" {
		adminID = "admin"
	}

	msg, err := h.service.SendChatMessage(c.Request.Context(), c.Param("id"), supportmodels.SenderTypeAdmin, adminID, req.Content)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusCreated, msg.Public())
}

// GET /admin/complaints/:id/messages
func (h *Handler) AdminListMessages(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	messages, err := h.service.ListChatMessages(c.Request.Context(), c.Param("id"), supportmodels.SenderTypeAdmin, "admin", limit, offset)
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	public := make([]supportmodels.PublicChatMessage, len(messages))
	for i, m := range messages {
		public[i] = m.Public()
	}
	respondOK(c, gin.H{"messages": public})
}

// ─── Admin handlers ────────────────────────────────────────────────────────────

// PUT /admin/complaints/:id/status
func (h *Handler) AdminUpdateStatus(c *gin.Context) {
	var req updateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Invalid request body.", err))
		return
	}

	result, err := h.service.UpdateStatus(c.Request.Context(), supportusecases.UpdateComplaintStatusInput{
		ComplaintID:    c.Param("id"),
		Status:         supportmodels.ComplaintStatus(req.Status),
		ResolutionNote: req.ResolutionNote,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result.Public())
}

// POST /admin/disputes/:id/resolve
func (h *Handler) AdminResolveDispute(c *gin.Context) {
	var req resolveDisputeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Invalid request body.", err))
		return
	}

	// adjudicator_id comes from the service-auth context in a real setup;
	// using header as simple stand-in for now.
	adjudicatorID := c.GetHeader("X-Admin-ID")

	result, err := h.service.ResolveDispute(c.Request.Context(), supportusecases.ResolveDisputeInput{
		DisputeID:     c.Param("id"),
		Outcome:       supportmodels.DisputeOutcome(req.Outcome),
		Note:          req.Note,
		AdjudicatorID: adjudicatorID,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result.Public())
}

// ─── Request/response DTOs ────────────────────────────────────────────────────

type sendMessageRequest struct {
	Content string `json:"content"`
}

type createComplaintRequest struct {
	ServiceType      string `json:"service_type"`
	BookingReference string `json:"booking_reference"`
	Subject          string `json:"subject"`
	Description      string `json:"description"`
}

type addEvidenceRequest struct {
	MediaAssetID string `json:"media_asset_id"`
	MediaURL     string `json:"media_url"`
	Note         string `json:"note"`
}

type escalateDisputeRequest struct {
	RespondentType string `json:"respondent_type"`
	RespondentID   string `json:"respondent_id"`
}

type updateStatusRequest struct {
	Status         string `json:"status"`
	ResolutionNote string `json:"resolution_note"`
}

type resolveDisputeRequest struct {
	Outcome string `json:"outcome"`
	Note    string `json:"note"`
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func complainantTypeFromRole(role string) supportmodels.ComplainantType {
	switch role {
	case "taxi":
		return supportmodels.ComplainantTaxiProvider
	case "dispatch":
		return supportmodels.ComplainantDispatchProvider
	case "hauling":
		return supportmodels.ComplainantHaulingProvider
	default:
		return supportmodels.ComplainantCustomer
	}
}

func respondOK(c *gin.Context, data interface{}) {
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": data})
}

func respond(c *gin.Context, status int, data interface{}) {
	c.JSON(status, gin.H{"success": true, "data": data})
}
