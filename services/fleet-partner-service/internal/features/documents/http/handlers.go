// Package http exposes the corporate document API.
//
// Every route is tenant-scoped and permission-gated, and the four permissions
// are deliberately separate: reading a document record, uploading one,
// obtaining the file itself and deciding whether it is genuine are four
// different capabilities. Someone who may attach an insurance certificate is
// not automatically someone who may declare it verified, and neither is
// automatically someone who may read the file.
//
// Default deny is structural: every handler resolves the tenant first and
// refuses when the authorizer is absent, so a missing gate can never read as
// an open one.
package http

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Permissions guarding the routes.
const (
	PermissionRead     = "documents.read"
	PermissionManage   = "documents.manage"
	PermissionDownload = "documents.download"
	PermissionVerify   = "documents.verify"
	PermissionDelete   = "documents.delete"
	// PermissionDownloadRestricted gates RESTRICTED and COMPANY_CONFIDENTIAL
	// documents on top of PermissionDownload. Holding the ordinary download
	// permission does not extend to the most sensitive visibility tier.
	PermissionDownloadRestricted = "documents.download_restricted"
)

// Handler serves the corporate document routes.
type Handler struct {
	service    *usecases.Service
	authorizer *accesscontrol.Authorizer
}

// NewHandler builds the handler.
func NewHandler(service *usecases.Service, authorizer *accesscontrol.Authorizer) *Handler {
	return &Handler{service: service, authorizer: authorizer}
}

// RegisterTenantRoutes mounts routes that require resolved organization scope.
func (h *Handler) RegisterTenantRoutes(group *gin.RouterGroup) {
	group.POST("/documents/upload-requests", h.requestUpload)
	group.GET("/documents", h.listDocuments)
	group.GET("/documents/:documentId", h.getDocument)
	group.GET("/documents/:documentId/history", h.documentHistory)
	group.GET("/documents/:documentId/verifications", h.documentVerifications)
	group.POST("/documents/:documentId/download-requests", h.requestDownload)
	group.POST("/documents/:documentId/verification-requests", h.requestVerification)
	group.POST("/documents/:documentId/verify", h.verify)
	group.POST("/documents/:documentId/reject", h.reject)
	group.POST("/documents/:documentId/replace", h.replace)
	group.POST("/documents/:documentId/revoke", h.revoke)
	group.POST("/documents/:documentId/archive", h.archive)
	group.POST("/documents/:documentId/retention", h.setRetention)
	group.POST("/documents/:documentId/deletion-requests", h.scheduleDeletion)
	group.GET("/documents/:documentId/deletion", h.readDeletion)
	group.GET("/documents/reconciliation-cases", h.listCases)
	group.POST("/documents/reconciliation-cases/:caseId/replay", h.replayCase)
	group.POST("/documents/deletion-requests/:requestId/replay", h.replayDeletion)
}

// requireTenant resolves the tenant context or fails closed.
func requireTenant(c *gin.Context) (tenant.Context, bool) {
	tenantCtx, ok := middleware.TenantFromGin(c)
	if !ok {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return tenant.Context{}, false
	}
	return tenantCtx, true
}

// authorize enforces a permission for the caller's membership.
func (h *Handler) authorize(c *gin.Context, tenantCtx tenant.Context, permission string) bool {
	if h.authorizer == nil {
		// No authorizer means no way to prove authority. Denying is the only
		// safe answer: a missing gate must never read as an open one.
		respond.Error(c, apperrors.Forbidden("You do not have permission to perform this action.", nil))
		return false
	}
	if err := h.authorizer.Require(c.Request.Context(), tenantCtx.OrganizationID,
		tenantCtx.OrganizationUserID, roleVersionFrom(c), permission,
		accesscontrol.Target{}); err != nil {
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

func actorFrom(tenantCtx tenant.Context) usecases.Actor {
	return usecases.Actor{
		OrganizationID: tenantCtx.OrganizationID,
		ActorType:      models.ActorOrganizationUser,
		ActorID:        tenantCtx.OrganizationUserID,
	}
}

// DocumentView is the allowlisted response shape.
//
// Built field by field. There is deliberately no field here for a URL, a
// storage path, a bucket or a token: a private document that carried a durable
// link in its own representation would not be private.
type DocumentView struct {
	ID                  string  `json:"id"`
	OwnerType           string  `json:"ownerType"`
	OwnerID             string  `json:"ownerId"`
	BranchID            string  `json:"branchId,omitempty"`
	Category            string  `json:"category"`
	DocumentType        string  `json:"documentType"`
	IssueDate           string  `json:"issueDate,omitempty"`
	ExpiryDate          string  `json:"expiryDate,omitempty"`
	LifecycleState      string  `json:"lifecycleState"`
	VerificationState   string  `json:"verificationState"`
	SafetyProjection    string  `json:"safetyProjection"`
	ProjectionFreshness string  `json:"projectionFreshness"`
	MediaSourceVersion  int64   `json:"mediaSourceVersion"`
	ReplacementID       string  `json:"replacementDocumentId,omitempty"`
	ReplacesID          string  `json:"replacesDocumentId,omitempty"`
	RevokedReason       string  `json:"revokedReason,omitempty"`
	RejectedReason      string  `json:"rejectedReason,omitempty"`
	RetentionClass      string  `json:"retentionClass"`
	RetentionUntil      string  `json:"retentionUntil,omitempty"`
	LegalHold           bool    `json:"legalHold"`
	Visibility          string  `json:"visibility"`
	Archived            bool    `json:"archived"`
	ArchivedAt          string  `json:"archivedAt,omitempty"`
	Verified            bool    `json:"verified"`
	Downloadable        bool    `json:"downloadable"`
	Version             int64   `json:"version"`
	CreatedAt           string  `json:"createdAt"`
	UpdatedAt           string  `json:"updatedAt"`
	Warning             *string `json:"warning,omitempty"`
}

// staleWarning is returned with every document whose media projection is not
// current.
//
// Present in the payload rather than only in documentation: a client that has
// to read a manual to learn a record is unconfirmed will eventually not read
// it.
const staleWarning = "The document service could not confirm this file's current state, so it cannot be used to satisfy a requirement or be downloaded until it is reconciled."

func newDocumentView(document *models.Document) DocumentView {
	view := DocumentView{
		ID:                  document.ID,
		OwnerType:           document.OwnerType,
		OwnerID:             document.OwnerID,
		Category:            document.Category,
		DocumentType:        document.DocumentType,
		LifecycleState:      document.LifecycleState,
		VerificationState:   document.VerificationState,
		SafetyProjection:    document.SafetyProjection,
		ProjectionFreshness: document.ProjectionFreshness,
		MediaSourceVersion:  document.MediaSourceVersion,
		RevokedReason:       document.RevokedReason,
		RejectedReason:      document.RejectedReason,
		RetentionClass:      document.RetentionClass,
		LegalHold:           document.LegalHold,
		Visibility:          document.Visibility,
		Archived:            document.Archived(),
		Verified:            document.Verified(),
		Downloadable:        document.Downloadable(),
		Version:             document.Version,
		CreatedAt:           document.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:           document.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if document.BranchID != nil {
		view.BranchID = *document.BranchID
	}
	if document.ReplacementDocumentID != nil {
		view.ReplacementID = *document.ReplacementDocumentID
	}
	if document.ReplacesDocumentID != nil {
		view.ReplacesID = *document.ReplacesDocumentID
	}
	if document.IssueDate != nil {
		view.IssueDate = document.IssueDate.UTC().Format("2006-01-02")
	}
	if document.ExpiryDate != nil {
		view.ExpiryDate = document.ExpiryDate.UTC().Format("2006-01-02")
	}
	if document.RetentionUntil != nil {
		view.RetentionUntil = document.RetentionUntil.UTC().Format(time.RFC3339)
	}
	if document.ArchivedAt != nil {
		view.ArchivedAt = document.ArchivedAt.UTC().Format(time.RFC3339)
	}
	if !document.ProjectionCurrent() {
		warning := staleWarning
		view.Warning = &warning
	}
	return view
}

type uploadRequestBody struct {
	OwnerType          string   `json:"ownerType"`
	OwnerID            string   `json:"ownerId"`
	BranchID           string   `json:"branchId"`
	Category           string   `json:"category"`
	DocumentType       string   `json:"documentType"`
	IssueDate          string   `json:"issueDate"`
	ExpiryDate         string   `json:"expiryDate"`
	AllowedMIMETypes   []string `json:"allowedMimeTypes"`
	MaxSizeBytes       int64    `json:"maxSizeBytes"`
	RetentionClass     string   `json:"retentionClass"`
	ReplacesDocumentID string   `json:"replacesDocumentId"`
	// Visibility defaults to INTERNAL when omitted.
	Visibility string `json:"visibility"`
}

func parseDate(value string) (*time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func (h *Handler) requestUpload(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionManage) {
		return
	}

	var body uploadRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	idempotencyKey := c.GetHeader("Idempotency-Key")
	if idempotencyKey == "" {
		respond.Error(c, apperrors.BadRequest("An Idempotency-Key header is required.", nil))
		return
	}
	issueDate, err := parseDate(body.IssueDate)
	if err != nil {
		respond.Error(c, apperrors.BadRequest("issueDate must be YYYY-MM-DD.", err))
		return
	}
	expiryDate, err := parseDate(body.ExpiryDate)
	if err != nil {
		respond.Error(c, apperrors.BadRequest("expiryDate must be YYYY-MM-DD.", err))
		return
	}

	ticket, err := h.service.RequestUpload(c.Request.Context(), usecases.RequestUploadInput{
		Actor:              actorFrom(tenantCtx),
		OwnerType:          body.OwnerType,
		OwnerID:            body.OwnerID,
		BranchID:           body.BranchID,
		Category:           body.Category,
		DocumentType:       body.DocumentType,
		IssueDate:          issueDate,
		ExpiryDate:         expiryDate,
		AllowedMIMETypes:   body.AllowedMIMETypes,
		MaxSizeBytes:       body.MaxSizeBytes,
		RetentionClass:     body.RetentionClass,
		ReplacesDocumentID: body.ReplacesDocumentID,
		Visibility:         body.Visibility,
		IdempotencyKey:     idempotencyKey,
		CorrelationID:      middleware.GetCorrelationID(c),
		CausationID:        c.GetHeader("X-Causation-ID"),
		RequestID:          c.GetHeader("X-Request-ID"),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}

	status := http.StatusCreated
	if ticket.Replayed {
		status = http.StatusOK
	}
	// The upload token is returned once, here. It is not stored on the fleet
	// side and never appears in a log line.
	c.JSON(status, gin.H{"data": gin.H{
		"document":         newDocumentView(ticket.Document),
		"uploadToken":      ticket.UploadToken,
		"uploadPath":       ticket.UploadPath,
		"expiresAt":        ticket.ExpiresAt.UTC().Format(time.RFC3339),
		"maxSizeBytes":     ticket.MaxSizeBytes,
		"allowedMimeTypes": ticket.AllowedMIME,
		"replayed":         ticket.Replayed,
	}, "meta": gin.H{"requestId": c.GetHeader("X-Request-ID")}})
}

func (h *Handler) listDocuments(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionRead) {
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))

	documents, err := h.service.List(c.Request.Context(), tenant.Query{
		OrganizationID:     tenantCtx.OrganizationID,
		PermittedBranchIDs: tenantCtx.PermittedBranchIDs,
		Limit:              limit,
	}, repositories.ListFilter{
		OwnerType:       strings.ToUpper(c.Query("ownerType")),
		OwnerID:         c.Query("ownerId"),
		BranchID:        c.Query("branchId"),
		Category:        strings.ToUpper(c.Query("category")),
		DocumentType:    strings.ToUpper(c.Query("documentType")),
		LifecycleState:  strings.ToUpper(c.Query("lifecycleState")),
		Visibility:      strings.ToUpper(c.Query("visibility")),
		IncludeRetired:  c.Query("includeRetired") == "true",
		IncludeArchived: c.Query("includeArchived") == "true",
	})
	if err != nil {
		respond.Error(c, err)
		return
	}

	views := make([]DocumentView, 0, len(documents))
	for index := range documents {
		views = append(views, newDocumentView(&documents[index]))
	}
	respond.OK(c, gin.H{"documents": views})
}

func (h *Handler) getDocument(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionRead) {
		return
	}
	document, err := h.service.Get(c.Request.Context(), tenantCtx.OrganizationID, c.Param("documentId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	if !h.withinBranchScope(tenantCtx, document) {
		respond.Error(c, apperrors.NotFound("Document not found.", nil))
		return
	}
	respond.OK(c, newDocumentView(document))
}

// withinBranchScope refuses a document outside the caller's permitted branches.
//
// The listing filters in SQL; a direct read by identifier has to be checked
// here, or a caller restricted to one branch could reach another branch's
// document simply by knowing its id.
func (h *Handler) withinBranchScope(tenantCtx tenant.Context, document *models.Document) bool {
	if len(tenantCtx.PermittedBranchIDs) == 0 || document.BranchID == nil {
		return true
	}
	for _, permitted := range tenantCtx.PermittedBranchIDs {
		if permitted == *document.BranchID {
			return true
		}
	}
	return false
}

func (h *Handler) documentHistory(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionRead) {
		return
	}
	entries, err := h.service.History(c.Request.Context(), tenantCtx.OrganizationID, c.Param("documentId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]gin.H, 0, len(entries))
	for _, entry := range entries {
		views = append(views, gin.H{
			"action":     entry.Action,
			"fromState":  entry.FromState,
			"toState":    entry.ToState,
			"actorType":  entry.ActorType,
			"occurredAt": entry.OccurredAt.UTC().Format(time.RFC3339),
		})
	}
	respond.OK(c, gin.H{"history": views})
}

func (h *Handler) documentVerifications(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionRead) {
		return
	}
	decisions, err := h.service.Verifications(c.Request.Context(), tenantCtx.OrganizationID,
		c.Param("documentId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]gin.H, 0, len(decisions))
	for _, decision := range decisions {
		views = append(views, gin.H{
			"decision":           decision.Decision,
			"reasonCode":         decision.ReasonCode,
			"reasonNote":         decision.ReasonNote,
			"mediaSourceVersion": decision.MediaSourceVersion,
			"decidedByActorType": decision.DecidedByActorType,
			"decidedAt":          decision.DecidedAt.UTC().Format(time.RFC3339),
		})
	}
	respond.OK(c, gin.H{"verifications": views})
}

func (h *Handler) requestDownload(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionDownload) {
		return
	}
	document, err := h.service.Get(c.Request.Context(), tenantCtx.OrganizationID, c.Param("documentId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	if !h.withinBranchScope(tenantCtx, document) {
		respond.Error(c, apperrors.NotFound("Document not found.", nil))
		return
	}
	if models.RequiresElevatedAccess(document.Visibility) &&
		!h.authorize(c, tenantCtx, PermissionDownloadRestricted) {
		return
	}

	grant, err := h.service.AuthorizeDownload(c.Request.Context(), actorFrom(tenantCtx),
		c.Param("documentId"), middleware.GetCorrelationID(c), c.GetHeader("X-Causation-ID"),
		c.GetHeader("X-Request-ID"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	// The grant is returned once and never stored. Clients present it to the
	// content route within its lifetime or ask for a new one.
	respond.OK(c, gin.H{
		"documentId":    grant.DocumentID,
		"downloadToken": grant.Token,
		"expiresAt":     grant.ExpiresAt.UTC().Format(time.RFC3339),
		"contentType":   grant.ContentType,
		"sizeBytes":     grant.SizeBytes,
	})
}

type verificationBody struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	ReasonCode      string `json:"reasonCode"`
	ReasonNote      string `json:"reasonNote"`
}

func (h *Handler) verificationInput(c *gin.Context, tenantCtx tenant.Context,
	body verificationBody) usecases.VerificationInput {
	return usecases.VerificationInput{
		Actor:           actorFrom(tenantCtx),
		DocumentID:      c.Param("documentId"),
		ExpectedVersion: body.ExpectedVersion,
		ReasonCode:      strings.ToUpper(strings.TrimSpace(body.ReasonCode)),
		ReasonNote:      body.ReasonNote,
		CorrelationID:   middleware.GetCorrelationID(c),
		CausationID:     c.GetHeader("X-Causation-ID"),
		RequestID:       c.GetHeader("X-Request-ID"),
	}
}

func (h *Handler) requestVerification(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	// Asking for a review is part of managing a document, not of deciding it.
	if !h.authorize(c, tenantCtx, PermissionManage) {
		return
	}
	var body verificationBody
	_ = c.ShouldBindJSON(&body)

	document, err := h.service.RequestVerification(c.Request.Context(),
		h.verificationInput(c, tenantCtx, body))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, newDocumentView(document))
}

func (h *Handler) verify(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionVerify) {
		return
	}
	var body verificationBody
	_ = c.ShouldBindJSON(&body)

	document, err := h.service.Verify(c.Request.Context(), h.verificationInput(c, tenantCtx, body))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, newDocumentView(document))
}

func (h *Handler) reject(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionVerify) {
		return
	}
	var body verificationBody
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	document, err := h.service.Reject(c.Request.Context(), h.verificationInput(c, tenantCtx, body))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, newDocumentView(document))
}

type replaceBody struct {
	ExpectedVersion  int64    `json:"expectedVersion"`
	Category         string   `json:"category"`
	DocumentType     string   `json:"documentType"`
	IssueDate        string   `json:"issueDate"`
	ExpiryDate       string   `json:"expiryDate"`
	AllowedMIMETypes []string `json:"allowedMimeTypes"`
	MaxSizeBytes     int64    `json:"maxSizeBytes"`
	RetentionClass   string   `json:"retentionClass"`
}

func (h *Handler) replace(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionManage) {
		return
	}
	var body replaceBody
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	idempotencyKey := c.GetHeader("Idempotency-Key")
	if idempotencyKey == "" {
		respond.Error(c, apperrors.BadRequest("An Idempotency-Key header is required.", nil))
		return
	}
	issueDate, err := parseDate(body.IssueDate)
	if err != nil {
		respond.Error(c, apperrors.BadRequest("issueDate must be YYYY-MM-DD.", err))
		return
	}
	expiryDate, err := parseDate(body.ExpiryDate)
	if err != nil {
		respond.Error(c, apperrors.BadRequest("expiryDate must be YYYY-MM-DD.", err))
		return
	}

	ticket, err := h.service.Replace(c.Request.Context(), usecases.ReplaceInput{
		Actor:            actorFrom(tenantCtx),
		DocumentID:       c.Param("documentId"),
		ExpectedVersion:  body.ExpectedVersion,
		Category:         strings.ToUpper(strings.TrimSpace(body.Category)),
		DocumentType:     strings.ToUpper(strings.TrimSpace(body.DocumentType)),
		IssueDate:        issueDate,
		ExpiryDate:       expiryDate,
		AllowedMIMETypes: body.AllowedMIMETypes,
		MaxSizeBytes:     body.MaxSizeBytes,
		RetentionClass:   body.RetentionClass,
		IdempotencyKey:   idempotencyKey,
		CorrelationID:    middleware.GetCorrelationID(c),
		CausationID:      c.GetHeader("X-Causation-ID"),
		RequestID:        c.GetHeader("X-Request-ID"),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": gin.H{
		"document":         newDocumentView(ticket.Document),
		"uploadToken":      ticket.UploadToken,
		"uploadPath":       ticket.UploadPath,
		"expiresAt":        ticket.ExpiresAt.UTC().Format(time.RFC3339),
		"maxSizeBytes":     ticket.MaxSizeBytes,
		"allowedMimeTypes": ticket.AllowedMIME,
	}, "meta": gin.H{"requestId": c.GetHeader("X-Request-ID")}})
}

type revokeBody struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason"`
}

func (h *Handler) revoke(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionManage) {
		return
	}
	var body revokeBody
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	document, err := h.service.Revoke(c.Request.Context(), usecases.RevokeInput{
		Actor:           actorFrom(tenantCtx),
		DocumentID:      c.Param("documentId"),
		ExpectedVersion: body.ExpectedVersion,
		Reason:          body.Reason,
		CorrelationID:   middleware.GetCorrelationID(c),
		CausationID:     c.GetHeader("X-Causation-ID"),
		RequestID:       c.GetHeader("X-Request-ID"),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, newDocumentView(document))
}

type archiveBody struct {
	ExpectedVersion int64 `json:"expectedVersion"`
}

func (h *Handler) archive(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionManage) {
		return
	}
	var body archiveBody
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	document, err := h.service.Archive(c.Request.Context(), usecases.ArchiveInput{
		Actor:           actorFrom(tenantCtx),
		DocumentID:      c.Param("documentId"),
		ExpectedVersion: body.ExpectedVersion,
		CorrelationID:   middleware.GetCorrelationID(c),
		CausationID:     c.GetHeader("X-Causation-ID"),
		RequestID:       c.GetHeader("X-Request-ID"),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, newDocumentView(document))
}

type retentionBody struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	RetentionClass  string `json:"retentionClass"`
	RetentionUntil  string `json:"retentionUntil"`
	LegalHold       *bool  `json:"legalHold"`
	Reason          string `json:"reason"`
}

func (h *Handler) setRetention(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionDelete) {
		return
	}
	var body retentionBody
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	var until *time.Time
	if strings.TrimSpace(body.RetentionUntil) != "" {
		parsed, err := time.Parse(time.RFC3339, body.RetentionUntil)
		if err != nil {
			respond.Error(c, apperrors.BadRequest("retentionUntil must be an RFC3339 timestamp.", err))
			return
		}
		until = &parsed
	}

	document, err := h.service.SetRetention(c.Request.Context(), usecases.RetentionInput{
		Actor:           actorFrom(tenantCtx),
		DocumentID:      c.Param("documentId"),
		ExpectedVersion: body.ExpectedVersion,
		RetentionClass:  body.RetentionClass,
		RetentionUntil:  until,
		LegalHold:       body.LegalHold,
		Reason:          body.Reason,
		CorrelationID:   middleware.GetCorrelationID(c),
		CausationID:     c.GetHeader("X-Causation-ID"),
		RequestID:       c.GetHeader("X-Request-ID"),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, newDocumentView(document))
}

type deletionBody struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason"`
	ScheduledFor    string `json:"scheduledFor"`
}

func (h *Handler) scheduleDeletion(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionDelete) {
		return
	}
	var body deletionBody
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	idempotencyKey := c.GetHeader("Idempotency-Key")
	if idempotencyKey == "" {
		respond.Error(c, apperrors.BadRequest("An Idempotency-Key header is required.", nil))
		return
	}
	var scheduledFor time.Time
	if strings.TrimSpace(body.ScheduledFor) != "" {
		parsed, err := time.Parse(time.RFC3339, body.ScheduledFor)
		if err != nil {
			respond.Error(c, apperrors.BadRequest("scheduledFor must be an RFC3339 timestamp.", err))
			return
		}
		scheduledFor = parsed
	}

	request, err := h.service.ScheduleDeletion(c.Request.Context(), usecases.ScheduleDeletionInput{
		Actor:           actorFrom(tenantCtx),
		DocumentID:      c.Param("documentId"),
		ExpectedVersion: body.ExpectedVersion,
		Reason:          body.Reason,
		ScheduledFor:    scheduledFor,
		IdempotencyKey:  idempotencyKey,
		CorrelationID:   middleware.GetCorrelationID(c),
		CausationID:     c.GetHeader("X-Causation-ID"),
		RequestID:       c.GetHeader("X-Request-ID"),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": deletionView(request),
		"meta": gin.H{"requestId": c.GetHeader("X-Request-ID")}})
}

func (h *Handler) readDeletion(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionRead) {
		return
	}
	request, err := h.service.LatestDeletion(c.Request.Context(), tenantCtx.OrganizationID,
		c.Param("documentId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, deletionView(request))
}

func deletionView(request *models.DeletionRequest) gin.H {
	view := gin.H{
		"deletionRequestId": request.ID,
		"documentId":        request.DocumentID,
		"status":            request.Status,
		"blockReason":       request.BlockReason,
		"attempts":          request.Attempts,
		"scheduledFor":      request.ScheduledFor.UTC().Format(time.RFC3339),
		"createdAt":         request.CreatedAt.UTC().Format(time.RFC3339),
	}
	if request.CompletedAt != nil {
		view["completedAt"] = request.CompletedAt.UTC().Format(time.RFC3339)
	}
	return view
}

func (h *Handler) listCases(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionRead) {
		return
	}
	cases, err := h.service.Cases(c.Request.Context(), tenantCtx.OrganizationID, c.Query("documentId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]gin.H, 0, len(cases))
	for _, record := range cases {
		views = append(views, gin.H{
			"id":            record.ID,
			"documentId":    record.DocumentID,
			"reason":        record.Reason,
			"status":        record.Status,
			"attempts":      record.Attempts,
			"lastErrorCode": record.LastErrorCode,
			"createdAt":     record.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	respond.OK(c, gin.H{"cases": views})
}

func (h *Handler) replayCase(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	// Replaying a dead-lettered case restarts platform work, so it sits with
	// the deletion/retention capability rather than plain read access.
	if !h.authorize(c, tenantCtx, PermissionDelete) {
		return
	}
	if err := h.service.ReplayReconciliationCase(c.Request.Context(),
		tenantCtx.OrganizationID, c.Param("caseId")); err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{"status": "OPEN"})
}

func (h *Handler) replayDeletion(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionDelete) {
		return
	}
	if err := h.service.ReplayDeletion(c.Request.Context(),
		tenantCtx.OrganizationID, c.Param("requestId")); err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{"status": "SCHEDULED"})
}
