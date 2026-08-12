package vendors

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

type representativeRequest struct {
	Capacity         string   `json:"capacity"`
	FirstName        string   `json:"first_name"`
	LastName         string   `json:"last_name"`
	Email            string   `json:"email"`
	Phone            string   `json:"phone"`
	OwnershipPercent *float64 `json:"ownership_percent"`
	Authorized       bool     `json:"authorized"`
	ExpectedVersion  int64    `json:"expected_version"`
	StepUpGrantID    string   `json:"step_up_grant_id"`
}
type documentRequest struct {
	RepresentativeID string                `json:"representative_id"`
	BranchID         string                `json:"branch_id"`
	BrandID          string                `json:"brand_id"`
	Dimension        VerificationDimension `json:"dimension"`
	MediaRef         string                `json:"media_ref"`
	DocumentType     string                `json:"document_type"`
	ExpectedVersion  int64                 `json:"expected_version"`
}
type verificationRequest struct {
	SubjectType  string   `json:"subject_type"`
	SubjectRef   string   `json:"subject_ref"`
	DocumentRefs []string `json:"document_refs"`
}
type bankProfileRequest struct {
	AccountNumber   string `json:"account_number"`
	BankCode        string `json:"bank_code"`
	StepUpGrantID   string `json:"step_up_grant_id"`
	ExpectedVersion int64  `json:"expected_version"`
}

func (h *Handler) OnboardingOverview(c *gin.Context) {
	a, y := h.readActor(c)
	if !y {
		return
	}
	v, e := h.service.Overview(c, a)
	if e != nil {
		h.fail(c, e)
		return
	}
	ok(c, v)
}
func (h *Handler) OnboardingReadiness(c *gin.Context) {
	a, y := h.readActor(c)
	if !y {
		return
	}
	v, e := h.service.Readiness(c, a)
	if e != nil {
		h.fail(c, e)
		return
	}
	ok(c, v)
}
func (h *Handler) SubmitOnboarding(c *gin.Context) {
	h.mutate(c, http.StatusOK, func(a Actor, _ []byte) (any, error) { return h.service.SubmitOnboarding(c, a) })
}
func (h *Handler) RequestActivation(c *gin.Context) {
	h.mutate(c, http.StatusOK, func(a Actor, _ []byte) (any, error) { return h.service.RequestActivation(c, a) })
}

func (h *Handler) GetRepresentative(c *gin.Context) {
	a, y := h.readActor(c)
	if !y {
		return
	}
	v, e := h.service.GetRepresentative(c, a)
	if e != nil {
		h.fail(c, e)
		return
	}
	ok(c, v)
}
func (h *Handler) PutRepresentative(c *gin.Context) {
	h.mutate(c, http.StatusOK, func(a Actor, raw []byte) (any, error) {
		var r representativeRequest
		if e := strict(raw, &r); e != nil {
			return nil, e
		}
		return h.service.PutRepresentative(c, a, RepresentativeInput{Capacity: r.Capacity, FirstName: r.FirstName, LastName: r.LastName, Email: r.Email, Phone: r.Phone, OwnershipPercent: r.OwnershipPercent, Authorized: r.Authorized, ExpectedVersion: r.ExpectedVersion, StepUpGrantID: r.StepUpGrantID})
	})
}

func (h *Handler) ListDocuments(c *gin.Context) {
	a, y := h.readActor(c)
	if !y {
		return
	}
	v, e := h.service.ListDocuments(c, a)
	if e != nil {
		h.fail(c, e)
		return
	}
	ok(c, v)
}
func docInput(r documentRequest) DocumentInput {
	return DocumentInput{RepresentativeID: r.RepresentativeID, BranchID: r.BranchID, BrandID: r.BrandID, Dimension: r.Dimension, MediaRef: r.MediaRef, DocumentType: r.DocumentType, ExpectedVersion: r.ExpectedVersion}
}
func (h *Handler) CreateDocument(c *gin.Context) {
	h.mutate(c, http.StatusCreated, func(a Actor, raw []byte) (any, error) {
		var r documentRequest
		if e := strict(raw, &r); e != nil {
			return nil, e
		}
		return h.service.CreateDocument(c, a, docInput(r))
	})
}
func (h *Handler) UpdateDocument(c *gin.Context) {
	h.mutate(c, http.StatusOK, func(a Actor, raw []byte) (any, error) {
		var r documentRequest
		if e := strict(raw, &r); e != nil {
			return nil, e
		}
		return h.service.UpdateDocument(c, a, c.Param("documentId"), docInput(r))
	})
}
func (h *Handler) DeleteDocument(c *gin.Context) {
	h.mutate(c, http.StatusOK, func(a Actor, raw []byte) (any, error) {
		var r commandRequest
		if e := strict(raw, &r); e != nil {
			return nil, e
		}
		if e := h.service.DeleteDocument(c, a, c.Param("documentId"), r.ExpectedVersion); e != nil {
			return nil, e
		}
		return gin.H{"deleted": true}, nil
	})
}

func (h *Handler) ListVerifications(c *gin.Context) {
	a, y := h.readActor(c)
	if !y {
		return
	}
	v, e := h.service.ListVerifications(c, a)
	if e != nil {
		h.fail(c, e)
		return
	}
	ok(c, v)
}
func (h *Handler) GetVerification(c *gin.Context) {
	a, y := h.readActor(c)
	if !y {
		return
	}
	v, e := h.service.GetVerification(c, a, VerificationDimension(strings.ToUpper(c.Param("dimension"))))
	if e != nil {
		h.fail(c, e)
		return
	}
	ok(c, v)
}
func (h *Handler) SubmitVerification(c *gin.Context) {
	h.mutate(c, http.StatusAccepted, func(a Actor, raw []byte) (any, error) {
		var r verificationRequest
		if e := strict(raw, &r); e != nil {
			return nil, e
		}
		return h.service.SubmitVerification(c, a, SubmitVerificationInput{Dimension: VerificationDimension(strings.ToUpper(c.Param("dimension"))), SubjectType: strings.ToUpper(r.SubjectType), SubjectRef: r.SubjectRef, DocumentRefs: r.DocumentRefs}, c.GetHeader("Idempotency-Key"))
	})
}

func (h *Handler) GetBankProfile(c *gin.Context) {
	a, y := h.readActor(c)
	if !y {
		return
	}
	v, e := h.service.GetBankProfile(c, a)
	if e != nil {
		h.fail(c, e)
		return
	}
	ok(c, v)
}
func (h *Handler) PutBankProfile(c *gin.Context) {
	h.mutate(c, http.StatusOK, func(a Actor, raw []byte) (any, error) {
		var r bankProfileRequest
		if e := strict(raw, &r); e != nil {
			return nil, e
		}
		return h.service.PutBankProfile(c, a, r.AccountNumber, r.BankCode, c.GetHeader("Idempotency-Key"), r.StepUpGrantID, r.ExpectedVersion)
	})
}

func RegisterOnboardingRoutes(group *gin.RouterGroup, h *Handler) {
	o := group.Group("/onboarding")
	o.GET("", h.OnboardingOverview)
	o.GET("/progress", h.OnboardingOverview)
	o.GET("/checklist", h.OnboardingReadiness)
	o.GET("/blockers", h.OnboardingReadiness)
	o.GET("/activation-readiness", h.OnboardingReadiness)
	o.POST("/submit", h.SubmitOnboarding)
	o.POST("/request-activation", h.RequestActivation)
	o.GET("/representative", h.GetRepresentative)
	o.PUT("/representative", h.PutRepresentative)
	o.GET("/documents", h.ListDocuments)
	o.POST("/documents", h.CreateDocument)
	o.PATCH("/documents/:documentId", h.UpdateDocument)
	o.DELETE("/documents/:documentId", h.DeleteDocument)
	o.GET("/verifications", h.ListVerifications)
	o.GET("/verifications/:dimension", h.GetVerification)
	o.POST("/verifications/:dimension/submit", h.SubmitVerification)
	o.POST("/verifications/:dimension/resubmit", h.SubmitVerification)
	o.GET("/bank-profile", h.GetBankProfile)
	o.PUT("/bank-profile", h.PutBankProfile)
}

type verificationEventRequest struct {
	EventID       string                `json:"event_id"`
	BusinessID    string                `json:"business_id"`
	Dimension     VerificationDimension `json:"dimension"`
	SubjectType   string                `json:"subject_type"`
	SubjectRef    string                `json:"subject_ref"`
	SourceRef     string                `json:"source_ref"`
	SourceVersion int64                 `json:"source_version"`
	Status        VerificationState     `json:"status"`
	ObservedAt    time.Time             `json:"observed_at"`
	ExpiresAt     *time.Time            `json:"expires_at"`
	ReasonCode    string                `json:"reason_code"`
	Remediation   string                `json:"remediation"`
}

func (h *Handler) internalAllow(c *gin.Context) {
	raw, _ := c.Get("service_name")
	if raw != "verification-compliance-service" {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	c.Next()
}
func (h *Handler) internalNonce(c *gin.Context) {
	raw, _ := c.Get("service_name")
	nonce := c.GetHeader(serviceauth.HeaderNonce)
	if nonce == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	tag, e := h.service.db.Exec(c, `INSERT INTO vendor_service_nonces(caller,nonce,expires_at) VALUES($1,$2,now()+interval '10 minutes') ON CONFLICT DO NOTHING`, raw, nonce)
	if e != nil || tag.RowsAffected() != 1 {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Next()
}
func (h *Handler) VerificationEvent(c *gin.Context) {
	var r verificationEventRequest
	if e := c.ShouldBindJSON(&r); e != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", e))
		return
	}
	if strings.TrimSpace(r.EventID) == "" || strings.TrimSpace(r.BusinessID) == "" || !validDimension(r.Dimension) || !validVerificationState(r.Status) || r.SourceVersion < 1 || r.ObservedAt.IsZero() {
		httpx.RespondError(c, apperrors.BadRequest("Authoritative verification event is missing required source metadata.", nil))
		return
	}
	p, e := h.service.ApplyVerificationEvent(c, "verification-compliance-service", r.EventID, r.BusinessID, VerificationCaseResult{SourceRef: r.SourceRef, SourceVersion: r.SourceVersion, Status: r.Status, ObservedAt: r.ObservedAt, ExpiresAt: r.ExpiresAt, ReasonCode: r.ReasonCode, Remediation: r.Remediation}, r.Dimension, r.SubjectType, r.SubjectRef)
	if e != nil {
		httpx.RespondError(c, e)
		return
	}
	ok(c, p)
}

func validVerificationState(status VerificationState) bool {
	switch status {
	case VerificationNotStarted, VerificationInProgress, VerificationPendingReview, VerificationVerified, VerificationRejected, VerificationExpired, VerificationSuspended:
		return true
	}
	return false
}

func RegisterInternalOnboardingRoutes(router *gin.Engine, h *Handler, verifier *serviceauth.Verifier) {
	r := router.Group("/internal/vendor/v1")
	r.Use(serviceauth.Middleware(verifier), h.internalAllow, h.internalNonce)
	r.POST("/verification-events", h.VerificationEvent)
}
