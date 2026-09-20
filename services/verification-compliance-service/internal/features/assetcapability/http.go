package assetcapability

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// Handler is the internal HTTP surface for Section 12.
//
// Every route here is service-to-service: HMAC-signed, allowlisted, replay
// protected. There is no user-facing surface and deliberately no public one --
// a provider must not be able to open a case about their own vehicle, because
// self-certification is not certification.
type Handler struct {
	service *Service
	db      *pgxpool.Pool
	allowed map[string]bool
}

// NewHandler builds the handler.
//
// allowed is the deployment's caller allowlist. It is intersected with the
// per-route authority rules rather than replacing them: HMAC proves who is
// calling, the allowlist says whether that caller may reach this service at
// all, and the service layer decides whether they may do this particular thing
// to this particular authority's assets.
func NewHandler(service *Service, db *pgxpool.Pool, allowed []string) *Handler {
	set := map[string]bool{}
	for _, name := range allowed {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			set[trimmed] = true
		}
	}
	// The three owning authorities and Admin are always permitted to reach
	// this feature: without them the surface has no legitimate caller at all,
	// and an operator who forgets to list one would silently disable
	// certification rather than see a configuration error.
	for _, authority := range Authorities() {
		set[authority.OwningService()] = true
	}
	set[AdminService] = true
	return &Handler{service: service, db: db, allowed: set}
}

// Register mounts the feature on the service's API group.
func Register(group *gin.RouterGroup, handler *Handler, verifier *serviceauth.Verifier) {
	if group == nil || handler == nil || handler.service == nil || verifier == nil {
		return
	}
	routes := group.Group("/internal/asset-capability")
	routes.Use(serviceauth.Middleware(verifier), handler.requireAllowedCaller, handler.claimNonce)

	routes.POST("/cases", handler.openCase)
	routes.GET("/cases/:caseRef", handler.readCase)
	routes.POST("/cases/:caseRef/decisions", handler.decide)

	routes.GET("/assets/:authority/:assetId/certifications", handler.readCertifications)
	routes.GET("/assets/:authority/:assetId/documents", handler.readDocuments)

	routes.POST("/inspections", handler.recordInspection)
	routes.GET("/inspections/:inspectionRef", handler.readInspection)

	routes.POST("/vehicle-documents", handler.recordDocument)
	routes.PUT("/vehicle-documents/:documentRef/status", handler.setDocumentStatus)

	routes.PUT("/providers/:authority/:providerId", handler.reportProvider)
	routes.GET("/providers/:authority/:providerId", handler.readProvider)
}

func caller(c *gin.Context) string {
	value, _ := c.Get("service_name")
	name, _ := value.(string)
	return name
}

func (h *Handler) requireAllowedCaller(c *gin.Context) {
	if !h.allowed[caller(c)] {
		fail(c, http.StatusForbidden, "CALLER_NOT_ALLOWED", "This service may not use asset certification.")
		c.Abort()
		return
	}
	c.Next()
}

// claimNonce consumes the request nonce exactly once.
//
// The signature already proves the caller and bounds the clock skew; the nonce
// is what stops a captured request being replayed inside that window. A decision
// replayed an hour later would re-suspend an asset somebody had since restored.
func (h *Handler) claimNonce(c *gin.Context) {
	nonce := strings.TrimSpace(c.GetHeader(serviceauth.HeaderNonce))
	if nonce == "" {
		fail(c, http.StatusUnauthorized, "NONCE_REQUIRED", "A request nonce is required.")
		c.Abort()
		return
	}
	tag, err := h.db.Exec(c,
		`INSERT INTO verification_service_nonces(caller, nonce, expires_at)
		 VALUES($1,$2,now()+interval '10 minutes') ON CONFLICT DO NOTHING`, caller(c), nonce)
	if err != nil || tag.RowsAffected() != 1 {
		fail(c, http.StatusUnauthorized, "NONCE_REPLAYED", "This request has already been seen.")
		c.Abort()
		return
	}
	c.Next()
}

func ok(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"success": true, "data": data})
}

func fail(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"success": false, "error": gin.H{"code": code, "message": message}})
}

// respond maps a domain error onto the wire.
//
// The mapping lives in one function so a new refusal cannot accidentally be
// reported as a 500. In particular an evidence refusal is 422, not 400: the
// request was well formed and the caller cannot fix it by reformatting.
func respond(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrDisabled):
		fail(c, http.StatusServiceUnavailable, "CERTIFICATION_DISABLED",
			"Asset capability certification is not enabled.")
	case errors.Is(err, ErrForbidden):
		fail(c, http.StatusForbidden, "CALLER_NOT_PERMITTED", err.Error())
	case errors.Is(err, ErrNotFound):
		fail(c, http.StatusNotFound, "NOT_FOUND", "No such record.")
	case errors.Is(err, ErrConflict):
		fail(c, http.StatusConflict, "IDEMPOTENCY_CONFLICT",
			"This idempotency key was used for a different request.")
	case errors.Is(err, ErrVersion):
		fail(c, http.StatusConflict, "VERSION_CONFLICT",
			"The record moved since it was read; re-read and retry.")
	case errors.Is(err, ErrEvidence):
		fail(c, http.StatusUnprocessableEntity, "EVIDENCE_NOT_ACCEPTABLE", err.Error())
	case errors.Is(err, ErrValidation):
		fail(c, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
	default:
		fail(c, http.StatusInternalServerError, "INTERNAL_ERROR", "The request could not be completed.")
	}
}

func idempotencyKey(c *gin.Context) string {
	return strings.TrimSpace(c.GetHeader("Idempotency-Key"))
}

func correlationID(c *gin.Context) string {
	return strings.TrimSpace(c.GetHeader("X-Correlation-ID"))
}

func authorityParam(c *gin.Context) (Authority, bool) {
	authority, err := ParseAuthority(c.Param("authority"))
	if err != nil {
		fail(c, http.StatusBadRequest, "UNKNOWN_AUTHORITY", "That is not an asset authority.")
		return "", false
	}
	return authority, true
}

// --- Cases -------------------------------------------------------------------

type openCaseBody struct {
	Authority      string     `json:"authority"`
	ProviderID     string     `json:"provider_id"`
	AssetID        string     `json:"asset_id"`
	CapabilityCode string     `json:"capability_code"`
	ProductCode    string     `json:"product_code"`
	RequestedValue any        `json:"requested_value"`
	EvidenceRefs   []string   `json:"evidence_refs"`
	DocumentRefs   []string   `json:"document_refs"`
	InspectionRef  string     `json:"inspection_ref"`
	PolicyVersion  string     `json:"policy_version"`
	ExpiresAt      *time.Time `json:"expires_at"`
}

func (h *Handler) openCase(c *gin.Context) {
	var body openCaseBody
	if c.ShouldBindJSON(&body) != nil {
		fail(c, http.StatusBadRequest, "VALIDATION_FAILED", "Request body is invalid.")
		return
	}
	authority, err := ParseAuthority(body.Authority)
	if err != nil {
		respond(c, err)
		return
	}
	view, err := h.service.OpenCase(c, OpenCaseRequest{
		Caller: caller(c), Authority: authority, ProviderID: strings.TrimSpace(body.ProviderID),
		AssetID: strings.TrimSpace(body.AssetID), CapabilityCode: strings.TrimSpace(body.CapabilityCode),
		ProductCode: strings.TrimSpace(body.ProductCode), RequestedValue: body.RequestedValue,
		EvidenceRefs: body.EvidenceRefs, DocumentRefs: body.DocumentRefs,
		InspectionRef: strings.TrimSpace(body.InspectionRef), PolicyVersion: body.PolicyVersion,
		ExpiresAt: body.ExpiresAt, CorrelationID: correlationID(c), IdempotencyKey: idempotencyKey(c),
	})
	if err != nil {
		respond(c, err)
		return
	}
	ok(c, http.StatusCreated, view)
}

func (h *Handler) readCase(c *gin.Context) {
	detail, err := h.service.ReadCase(c, caller(c), c.Param("caseRef"))
	if err != nil {
		respond(c, err)
		return
	}
	ok(c, http.StatusOK, detail)
}

type decideBody struct {
	Decision              string     `json:"decision"`
	VerifiedValue         any        `json:"verified_value"`
	ReasonCode            string     `json:"reason_code"`
	Reviewer              string     `json:"reviewer"`
	EvidenceRefs          []string   `json:"evidence_refs"`
	DocumentRefs          []string   `json:"document_refs"`
	PolicyVersion         string     `json:"policy_version"`
	EffectiveAt           *time.Time `json:"effective_at"`
	ExpiresAt             *time.Time `json:"expires_at"`
	ExpectedSourceVersion int64      `json:"expected_source_version"`
}

func (h *Handler) decide(c *gin.Context) {
	var body decideBody
	if c.ShouldBindJSON(&body) != nil {
		fail(c, http.StatusBadRequest, "VALIDATION_FAILED", "Request body is invalid.")
		return
	}
	_, cached, err := h.service.Decide(c, DecideRequest{
		Caller: caller(c), CaseRef: c.Param("caseRef"),
		Decision:      Decision(strings.ToUpper(strings.TrimSpace(body.Decision))),
		VerifiedValue: body.VerifiedValue, ReasonCode: strings.TrimSpace(body.ReasonCode),
		Reviewer: strings.TrimSpace(body.Reviewer), EvidenceRefs: body.EvidenceRefs,
		DocumentRefs: body.DocumentRefs, PolicyVersion: body.PolicyVersion,
		EffectiveAt: body.EffectiveAt, ExpiresAt: body.ExpiresAt,
		CorrelationID: correlationID(c), ExpectedVersion: body.ExpectedSourceVersion,
		IdempotencyKey: idempotencyKey(c),
	})
	if err != nil {
		respond(c, err)
		return
	}
	// Both a fresh decision and a replayed one answer with the response that
	// was recorded when the decision was made, rather than one re-rendered from
	// the current row. A re-render would drift the moment these structs change,
	// and a caller comparing its retry against its original would see a
	// decision that appeared to have moved on its own.
	//
	// The body is stored in a jsonb column, so a replay is semantically
	// identical rather than byte-identical: PostgreSQL normalises key order and
	// whitespace. That is the guarantee callers actually need, and claiming a
	// byte-level one would be claiming something the storage does not provide.
	c.Data(http.StatusOK, "application/json", cached)
}

// --- Certifications ----------------------------------------------------------

func (h *Handler) readCertifications(c *gin.Context) {
	authority, valid := authorityParam(c)
	if !valid {
		return
	}
	certifications, err := h.service.ReadCertifications(c, caller(c), authority, c.Param("assetId"))
	if err != nil {
		respond(c, err)
		return
	}
	ok(c, http.StatusOK, gin.H{"authority": authority, "asset_id": c.Param("assetId"),
		"certifications": certifications})
}

// --- Inspections -------------------------------------------------------------

type inspectionBody struct {
	Authority      string     `json:"authority"`
	ProviderID     string     `json:"provider_id"`
	AssetID        string     `json:"asset_id"`
	InspectionType string     `json:"inspection_type"`
	Outcome        string     `json:"outcome"`
	EvidenceRefs   []string   `json:"evidence_refs"`
	InspectedAt    *time.Time `json:"inspected_at"`
	ExpiresAt      *time.Time `json:"expires_at"`
	InspectorRef   string     `json:"inspector_ref"`
	ReasonCode     string     `json:"reason_code"`
}

func (h *Handler) recordInspection(c *gin.Context) {
	var body inspectionBody
	if c.ShouldBindJSON(&body) != nil {
		fail(c, http.StatusBadRequest, "VALIDATION_FAILED", "Request body is invalid.")
		return
	}
	authority, err := ParseAuthority(body.Authority)
	if err != nil {
		respond(c, err)
		return
	}
	inspection, err := h.service.RecordInspection(c, RecordInspectionRequest{
		Caller: caller(c), Authority: authority, ProviderID: strings.TrimSpace(body.ProviderID),
		AssetID:        strings.TrimSpace(body.AssetID),
		InspectionType: InspectionType(strings.ToUpper(strings.TrimSpace(body.InspectionType))),
		Outcome:        InspectionOutcome(strings.ToUpper(strings.TrimSpace(body.Outcome))),
		EvidenceRefs:   body.EvidenceRefs, InspectedAt: body.InspectedAt, ExpiresAt: body.ExpiresAt,
		InspectorRef: strings.TrimSpace(body.InspectorRef), ReasonCode: strings.TrimSpace(body.ReasonCode),
		CorrelationID: correlationID(c), IdempotencyKey: idempotencyKey(c),
	})
	if err != nil {
		respond(c, err)
		return
	}
	ok(c, http.StatusCreated, inspection)
}

func (h *Handler) readInspection(c *gin.Context) {
	inspection, err := h.service.ReadInspection(c, caller(c), c.Param("inspectionRef"))
	if err != nil {
		respond(c, err)
		return
	}
	ok(c, http.StatusOK, inspection)
}

// --- Vehicle documents -------------------------------------------------------

type documentBody struct {
	Authority      string     `json:"authority"`
	ProviderID     string     `json:"provider_id"`
	AssetID        string     `json:"asset_id"`
	DocumentType   string     `json:"document_type"`
	MediaAssetID   string     `json:"media_asset_id"`
	DocumentNumber string     `json:"document_number"`
	Status         string     `json:"status"`
	IssuedAt       *time.Time `json:"issued_at"`
	ExpiresAt      *time.Time `json:"expires_at"`
	ReasonCode     string     `json:"reason_code"`
}

func (h *Handler) recordDocument(c *gin.Context) {
	var body documentBody
	if c.ShouldBindJSON(&body) != nil {
		fail(c, http.StatusBadRequest, "VALIDATION_FAILED", "Request body is invalid.")
		return
	}
	authority, err := ParseAuthority(body.Authority)
	if err != nil {
		respond(c, err)
		return
	}
	document, err := h.service.RecordDocument(c, RecordDocumentRequest{
		Caller: caller(c), Authority: authority, ProviderID: strings.TrimSpace(body.ProviderID),
		AssetID:        strings.TrimSpace(body.AssetID),
		DocumentType:   DocumentType(strings.ToUpper(strings.TrimSpace(body.DocumentType))),
		MediaAssetID:   strings.TrimSpace(body.MediaAssetID),
		DocumentNumber: strings.TrimSpace(body.DocumentNumber),
		Status:         DocumentStatus(strings.ToUpper(strings.TrimSpace(body.Status))),
		IssuedAt:       body.IssuedAt, ExpiresAt: body.ExpiresAt,
		ReasonCode: strings.TrimSpace(body.ReasonCode), CorrelationID: correlationID(c),
		IdempotencyKey: idempotencyKey(c),
	})
	if err != nil {
		respond(c, err)
		return
	}
	ok(c, http.StatusCreated, document)
}

type documentStatusBody struct {
	Status                string `json:"status"`
	ReasonCode            string `json:"reason_code"`
	ExpectedSourceVersion int64  `json:"expected_source_version"`
}

func (h *Handler) setDocumentStatus(c *gin.Context) {
	var body documentStatusBody
	if c.ShouldBindJSON(&body) != nil {
		fail(c, http.StatusBadRequest, "VALIDATION_FAILED", "Request body is invalid.")
		return
	}
	document, err := h.service.SetDocumentStatus(c, SetDocumentStatusRequest{
		Caller: caller(c), DocumentRef: c.Param("documentRef"),
		Status:     DocumentStatus(strings.ToUpper(strings.TrimSpace(body.Status))),
		ReasonCode: strings.TrimSpace(body.ReasonCode), ExpectedVersion: body.ExpectedSourceVersion,
	})
	if err != nil {
		respond(c, err)
		return
	}
	ok(c, http.StatusOK, document)
}

func (h *Handler) readDocuments(c *gin.Context) {
	authority, valid := authorityParam(c)
	if !valid {
		return
	}
	documents, err := h.service.ReadDocuments(c, caller(c), authority, c.Param("assetId"))
	if err != nil {
		respond(c, err)
		return
	}
	ok(c, http.StatusOK, gin.H{"authority": authority, "asset_id": c.Param("assetId"),
		"documents": documents})
}

// --- Provider verification ---------------------------------------------------

type providerBody struct {
	Status        string     `json:"status"`
	ReasonCode    string     `json:"reason_code"`
	SourceVersion int64      `json:"source_version"`
	EffectiveAt   *time.Time `json:"effective_at"`
	ExpiresAt     *time.Time `json:"expires_at"`
}

func (h *Handler) reportProvider(c *gin.Context) {
	authority, valid := authorityParam(c)
	if !valid {
		return
	}
	var body providerBody
	if c.ShouldBindJSON(&body) != nil {
		fail(c, http.StatusBadRequest, "VALIDATION_FAILED", "Request body is invalid.")
		return
	}
	record, err := h.service.ReportProviderVerification(c, ReportProviderRequest{
		Caller: caller(c), Authority: authority, ProviderID: c.Param("providerId"),
		Status:     ProviderStatus(strings.ToUpper(strings.TrimSpace(body.Status))),
		ReasonCode: strings.TrimSpace(body.ReasonCode), SourceVersion: body.SourceVersion,
		EffectiveAt: body.EffectiveAt, ExpiresAt: body.ExpiresAt, CorrelationID: correlationID(c),
	})
	if err != nil {
		respond(c, err)
		return
	}
	ok(c, http.StatusOK, record)
}

func (h *Handler) readProvider(c *gin.Context) {
	authority, valid := authorityParam(c)
	if !valid {
		return
	}
	record, err := h.service.ReadProviderVerification(c, caller(c), authority, c.Param("providerId"))
	if err != nil {
		respond(c, err)
		return
	}
	ok(c, http.StatusOK, record)
}
