package supporthttp

import (
	nethttp "net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	supportmodels "cosmicforge/logistics/services/support-dispute-service/internal/features/support/models"
	supportrepositories "cosmicforge/logistics/services/support-dispute-service/internal/features/support/repositories"
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

// ─── Customer / provider-facing handlers ───────────────────────────────────────

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

	result, err := h.service.CreateComplaint(c.Request.Context(), supportusecases.CreateComplaintInput{
		ComplainantType:  complainantTypeFromRole(claims.Role),
		ComplainantID:    claims.Subject,
		ServiceType:      supportmodels.ServiceType(req.ServiceType),
		BookingReference: req.BookingReference,
		Category:         req.Category,
		Subject:          req.Subject,
		Description:      req.Description,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusCreated, result.Public())
}

// POST /sos
func (h *Handler) CreateSOS(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	var req sosRequest
	_ = c.ShouldBindJSON(&req) // body optional; location/description are best-effort

	result, err := h.service.CreateSOS(c.Request.Context(), supportusecases.SOSInput{
		ComplainantType: complainantTypeFromRole(claims.Role),
		ComplainantID:   claims.Subject,
		ServiceType:     supportmodels.ServiceType(req.ServiceType),
		Description:     req.Description,
		Lat:             req.Lat,
		Lng:             req.Lng,
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

	limit, offset := pageParams(c)
	results, err := h.service.MyComplaints(c.Request.Context(), complainantTypeFromRole(claims.Role), claims.Subject, limit, offset)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, gin.H{"complaints": publicComplaints(results)})
}

// GET /complaints/:id
func (h *Handler) GetComplaint(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	result, err := h.service.GetComplaint(c.Request.Context(), c.Param("id"), complainantTypeFromRole(claims.Role), claims.Subject)
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
	respond(c, nethttp.StatusCreated, evidenceJSON(result))
}

// GET /complaints/:id/evidence
func (h *Handler) ListEvidence(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	results, err := h.service.ListEvidence(c.Request.Context(), c.Param("id"), complainantTypeFromRole(claims.Role), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, gin.H{"evidence": results})
}

// POST /complaints/:id/dispute
func (h *Handler) EscalateToDispute(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
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
		RequesterType:  complainantTypeFromRole(claims.Role),
		RequesterID:    claims.Subject,
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
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	result, err := h.service.GetDispute(c.Request.Context(), c.Param("id"), complainantTypeFromRole(claims.Role), claims.Subject)
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

	complaint, err := h.service.StartSupportChat(c.Request.Context(), complainantTypeFromRole(claims.Role), claims.Subject)
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

	limit, offset := pageParams(c)
	senderType := supportmodels.SenderType(complainantTypeFromRole(claims.Role))
	messages, err := h.service.ListChatMessages(c.Request.Context(), c.Param("id"), senderType, claims.Subject, limit, offset)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, gin.H{"messages": publicMessages(messages)})
}

// POST /complaints/:id/messages/read
func (h *Handler) MarkMessagesRead(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	senderType := supportmodels.SenderType(complainantTypeFromRole(claims.Role))
	if err := h.service.MarkMessagesRead(c.Request.Context(), c.Param("id"), senderType, claims.Subject); err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, gin.H{"ok": true})
}

// ─── Public help/categories ─────────────────────────────────────────────────────

// GET /support/categories
func (h *Handler) Categories(c *gin.Context) {
	respondOK(c, gin.H{"categories": h.service.Categories()})
}

// GET /support/faqs
func (h *Handler) FAQs(c *gin.Context) {
	articles, err := h.service.ListHelpArticles(c.Request.Context(), c.Query("audience"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	if articles == nil {
		articles = []supportmodels.HelpArticle{}
	}
	respondOK(c, gin.H{"faqs": articles})
}

// ─── Admin handlers ────────────────────────────────────────────────────────────

// GET /admin/complaints
func (h *Handler) AdminListComplaints(c *gin.Context) {
	limit, offset := pageParams(c)
	results, err := h.service.ListComplaintsAdmin(c.Request.Context(), supportrepositories.ComplaintFilter{
		ServiceType:     supportmodels.ServiceType(c.Query("service_type")),
		Status:          supportmodels.ComplaintStatus(c.Query("status")),
		ComplainantType: supportmodels.ComplainantType(c.Query("complainant_type")),
		Priority:        c.Query("priority"),
	}, limit, offset)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, gin.H{"complaints": publicComplaints(results)})
}

// GET /admin/complaints/:id
func (h *Handler) AdminGetComplaint(c *gin.Context) {
	result, err := h.service.GetComplaint(c.Request.Context(), c.Param("id"), "", "")
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result.Public())
}

// GET /admin/complaints/:id/evidence
func (h *Handler) AdminListEvidence(c *gin.Context) {
	results, err := h.service.ListEvidence(c.Request.Context(), c.Param("id"), "", "")
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, gin.H{"evidence": results})
}

// GET /admin/complaints/:id/dispute
func (h *Handler) AdminGetDispute(c *gin.Context) {
	result, err := h.service.GetDispute(c.Request.Context(), c.Param("id"), "", "")
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result.Public())
}

// GET /admin/complaints/:id/events
func (h *Handler) AdminListEvents(c *gin.Context) {
	events, err := h.service.ListEvents(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	if events == nil {
		events = []supportmodels.ComplaintEvent{}
	}
	respondOK(c, gin.H{"events": events})
}

// GET /admin/disputes
func (h *Handler) AdminListDisputes(c *gin.Context) {
	limit, offset := pageParams(c)
	results, err := h.service.ListDisputesAdmin(c.Request.Context(), supportrepositories.DisputeFilter{
		ServiceType: supportmodels.ServiceType(c.Query("service_type")),
		Outcome:     supportmodels.DisputeOutcome(c.Query("outcome")),
	}, limit, offset)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	public := make([]supportmodels.PublicDispute, len(results))
	for i, d := range results {
		public[i] = d.Public()
	}
	respondOK(c, gin.H{"disputes": public})
}

// POST /admin/complaints/:id/refresh-identity
func (h *Handler) AdminRefreshIdentity(c *gin.Context) {
	result, err := h.service.RefreshIdentity(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result.Public())
}

// POST /admin/complaints/:id/messages
func (h *Handler) AdminSendMessage(c *gin.Context) {
	var req sendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Invalid request body.", err))
		return
	}
	msg, err := h.service.SendChatMessage(c.Request.Context(), c.Param("id"), supportmodels.SenderTypeAdmin, adminID(c), req.Content)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusCreated, msg.Public())
}

// GET /admin/complaints/:id/messages
func (h *Handler) AdminListMessages(c *gin.Context) {
	limit, offset := pageParams(c)
	messages, err := h.service.ListChatMessages(c.Request.Context(), c.Param("id"), supportmodels.SenderTypeAdmin, "admin", limit, offset)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, gin.H{"messages": publicMessages(messages)})
}

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
		ActorID:        adminID(c),
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

	result, err := h.service.ResolveDispute(c.Request.Context(), supportusecases.ResolveDisputeInput{
		DisputeID:             c.Param("id"),
		Outcome:               supportmodels.DisputeOutcome(req.Outcome),
		Note:                  req.Note,
		AdjudicatorID:         adminID(c),
		RefundAmountKobo:      req.RefundAmountKobo,
		RefundSourceReference: req.RefundSourceReference,
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
	Category         string `json:"category"`
	Subject          string `json:"subject"`
	Description      string `json:"description"`
}

type sosRequest struct {
	ServiceType string   `json:"service_type"`
	Description string   `json:"description"`
	Lat         *float64 `json:"lat"`
	Lng         *float64 `json:"lng"`
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
	Outcome               string `json:"outcome"`
	Note                  string `json:"note"`
	RefundAmountKobo      int64  `json:"refund_amount_kobo"`
	RefundSourceReference string `json:"refund_source_reference"`
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func complainantTypeFromRole(role string) supportmodels.ComplainantType {
	switch role {
	case "taxi", "taxi_provider":
		return supportmodels.ComplainantTaxiProvider
	case "dispatch", "dispatch_provider":
		return supportmodels.ComplainantDispatchProvider
	case "hauling", "truck_provider":
		return supportmodels.ComplainantHaulingProvider
	default:
		return supportmodels.ComplainantCustomer
	}
}

func adminID(c *gin.Context) string {
	id := c.GetHeader("X-Admin-ID")
	if id == "" {
		return "admin"
	}
	return id
}

func pageParams(c *gin.Context) (int, int) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	return limit, offset
}

func publicComplaints(in []supportmodels.Complaint) []supportmodels.PublicComplaint {
	out := make([]supportmodels.PublicComplaint, len(in))
	for i, r := range in {
		out[i] = r.Public()
	}
	return out
}

func publicMessages(in []supportmodels.ChatMessage) []supportmodels.PublicChatMessage {
	out := make([]supportmodels.PublicChatMessage, len(in))
	for i, m := range in {
		out[i] = m.Public()
	}
	return out
}

func evidenceJSON(e supportmodels.Evidence) gin.H {
	return gin.H{
		"id":             e.ID,
		"complaint_id":   e.ComplaintID,
		"media_url":      e.MediaURL,
		"media_asset_id": e.MediaAssetID,
		"note":           e.Note,
		"created_at":     e.CreatedAt,
	}
}

func respondOK(c *gin.Context, data interface{}) {
	c.JSON(nethttp.StatusOK, gin.H{"success": true, "data": data})
}

func respond(c *gin.Context, status int, data interface{}) {
	c.JSON(status, gin.H{"success": true, "data": data})
}
