package http

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ratelimit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
)

// PublicHandler serves the customer-facing quotation routes.
//
// These are the only unauthenticated CRM routes. They are reached with a secure
// link rather than a session, so the token is the whole authorization decision
// and everything else about the request is untrusted input.
type PublicHandler struct {
	service *usecases.Service
	limiter ratelimit.Limiter
}

// NewPublicHandler builds the customer-facing handler.
func NewPublicHandler(service *usecases.Service, limiter ratelimit.Limiter) *PublicHandler {
	return &PublicHandler{service: service, limiter: limiter}
}

// tokenValidationRule bounds how fast a caller may present tokens.
//
// Without it a secure link is a 256-bit secret guarded only by arithmetic; with
// it, an attacker gets a handful of attempts per window from an address, which
// makes brute force useless rather than merely expensive.
var tokenValidationRule = ratelimit.Rule{Limit: 20, Window: time.Minute}

// RegisterPublicRoutes mounts the customer quotation routes.
//
// They are only mounted when a rate limiter exists. An unlimited token
// validation endpoint is worse than an absent one, so the absence is the safe
// failure.
func (h *PublicHandler) RegisterPublicRoutes(group *gin.RouterGroup) {
	if h.service == nil || h.limiter == nil {
		return
	}
	mount(group, h.CustomerTokenRoutes(), map[string]gin.HandlerFunc{
		"viewQuotationAsCustomer":   h.viewQuotation,
		"acceptQuotationAsCustomer": h.acceptQuotation,
		"rejectQuotationAsCustomer": h.rejectQuotation,
	})
}

// customerToken reads the presented token.
//
// A bearer header is preferred over the query string because a URL ends up in
// browser history, proxy logs and referrer headers. The query parameter is
// accepted because an emailed link cannot set a header, and the token's short
// life and single-quotation scope are what make that acceptable.
func customerToken(c *gin.Context) string {
	if header := strings.TrimSpace(c.GetHeader("Authorization")); header != "" {
		if strings.HasPrefix(strings.ToLower(header), "bearer ") {
			return strings.TrimSpace(header[7:])
		}
	}
	return strings.TrimSpace(c.Query("token"))
}

// allow applies the rate limit, keyed by the client address.
//
// The key is the remote address and nothing else: keying by token would let an
// attacker sidestep the limit simply by guessing a different token each time,
// which is exactly the attack the limit exists to stop.
func (h *PublicHandler) allow(c *gin.Context) bool {
	permitted, err := h.limiter.Allow(c.Request.Context(), "crm_quotation_token",
		c.ClientIP(), tokenValidationRule)
	if err != nil {
		// The limiter is unavailable. Refusing is the only safe answer: an
		// unlimited validation endpoint is a brute-force oracle.
		respond.Error(c, apperrors.Unavailable("Please try again shortly.", nil))
		return false
	}
	if !permitted {
		respond.Error(c, apperrors.RateLimited("Too many attempts. Try again shortly.", nil))
		return false
	}
	return true
}

func (h *PublicHandler) viewQuotation(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	view, err := h.service.ViewQuotationAsCustomer(c.Request.Context(), customerToken(c),
		c.Param("quotationId"), middleware.GetCorrelationID(c))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, view)
}

type customerAcceptRequest struct {
	AcceptedVersionID string `json:"acceptedVersionId"`
}

type customerAcceptResponse struct {
	QuotationNumber  string               `json:"quotationNumber"`
	Status           string               `json:"status"`
	AcceptedAt       string               `json:"acceptedAt"`
	ConversionIntent ConversionIntentView `json:"conversionIntent"`
}

// acceptQuotation records a customer acceptance made through a secure link.
//
// The response is deliberately thin: a customer learns that their acceptance
// landed and what it produced. It carries no internal identifiers beyond the
// conversion intent, no staff identity and no approval history.
func (h *PublicHandler) acceptQuotation(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	var body customerAcceptRequest
	if err := c.ShouldBindJSON(&body); err != nil && c.Request.ContentLength != 0 {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}
	result, err := h.service.AcceptQuotationByToken(c.Request.Context(), customerToken(c),
		c.Param("quotationId"), usecases.AcceptQuotationInput{
			AcceptedVersionID: body.AcceptedVersionID,
			// The address and user agent are hashed with the contact secret
			// before storage and are never returned or logged. They are
			// provenance for an acceptance, not a tracking record.
			RemoteAddress:  c.ClientIP(),
			UserAgent:      c.Request.UserAgent(),
			IdempotencyKey: idempotencyKey(c),
			CorrelationID:  middleware.GetCorrelationID(c),
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, customerAcceptResponse{
		QuotationNumber:  result.Quotation.QuotationNumber,
		Status:           result.Quotation.Status,
		AcceptedAt:       formatTime(result.Acceptance.AcceptedAt),
		ConversionIntent: NewConversionIntentView(result.ConversionIntent),
	})
}

type customerRejectRequest struct {
	ReasonCode string `json:"reasonCode"`
}

func (h *PublicHandler) rejectQuotation(c *gin.Context) {
	if !h.allow(c) {
		return
	}
	var body customerRejectRequest
	if err := c.ShouldBindJSON(&body); err != nil && c.Request.ContentLength != 0 {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}
	quotation, err := h.service.RejectQuotationByToken(c.Request.Context(), customerToken(c),
		c.Param("quotationId"), usecases.RejectQuotationInput{
			ReasonCode:     body.ReasonCode,
			IdempotencyKey: idempotencyKey(c),
			CorrelationID:  middleware.GetCorrelationID(c),
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, gin.H{
		"quotationNumber": quotation.QuotationNumber,
		"status":          quotation.Status,
	})
}

// ---------------------------------------------------------------------------
// Internal service routes
// ---------------------------------------------------------------------------

// InternalHandler serves the HMAC-authenticated CRM routes.
type InternalHandler struct {
	service *usecases.Service
}

// NewInternalHandler builds the internal handler.
func NewInternalHandler(service *usecases.Service) *InternalHandler {
	return &InternalHandler{service: service}
}

// RegisterInternalRoutes mounts the private service-to-service routes.
//
// The group these are mounted on already carries HMAC v2 verification,
// timestamp freshness and nonce replay protection from the shared middleware.
// What this handler adds is the authorization question that signature
// verification does not answer: whether this particular caller is entitled to
// assert an SLA outcome.
func (h *InternalHandler) RegisterInternalRoutes(group *gin.RouterGroup) {
	if h.service == nil {
		return
	}
	mount(group, h.InternalRoutes(), map[string]gin.HandlerFunc{
		"ingestServiceLevelObservation": h.ingestObservation,
	})
}

type observationRequest struct {
	OrganizationID  string  `json:"organizationId"`
	SLAID           string  `json:"slaId"`
	SourceType      string  `json:"sourceType"`
	SourceReference string  `json:"sourceReference"`
	MeasuredValue   *string `json:"measuredValue"`
	MeasurementUnit string  `json:"measurementUnit"`
	MeasuredAt      string  `json:"measuredAt"`
	WindowStart     string  `json:"windowStart"`
	WindowEnd       string  `json:"windowEnd"`
	SourceVersion   int64   `json:"sourceVersion"`
	EventID         string  `json:"eventId"`
	PayloadHash     string  `json:"payloadHash"`
	CausationID     string  `json:"causationId"`
}

// ingestObservation records one authoritative SLA measurement.
//
// The caller identity comes from the verified signature context, never from the
// body. The organization ID in the body is checked against the SLA's actual
// owner rather than trusted: a valid signature proves who is calling, and
// proves nothing at all about whose data they may write.
func (h *InternalHandler) ingestObservation(c *gin.Context) {
	caller := strings.TrimSpace(c.GetString(middleware.ServiceCallerKey))
	if caller == "" {
		respond.Error(c, apperrors.Unauthorized("Caller identity is required.", nil))
		return
	}
	var body observationRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return
	}
	measuredAt, err := parseRequired(body.MeasuredAt, "measuredAt")
	if err != nil {
		respond.Error(c, err)
		return
	}
	windowStart, err := parseRequired(body.WindowStart, "windowStart")
	if err != nil {
		respond.Error(c, err)
		return
	}
	windowEnd, err := parseRequired(body.WindowEnd, "windowEnd")
	if err != nil {
		respond.Error(c, err)
		return
	}

	result, err := h.service.IngestObservation(c.Request.Context(), caller,
		usecases.IngestObservationInput{
			OrganizationID: body.OrganizationID, SLAID: body.SLAID,
			SourceType: body.SourceType, SourceReference: body.SourceReference,
			MeasuredValue: body.MeasuredValue, MeasurementUnit: body.MeasurementUnit,
			MeasuredAt: measuredAt, WindowStart: windowStart, WindowEnd: windowEnd,
			SourceVersion: body.SourceVersion, EventID: body.EventID,
			PayloadHash:   body.PayloadHash,
			CorrelationID: middleware.GetCorrelationID(c), CausationID: body.CausationID,
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	status := http.StatusCreated
	if result.Duplicate {
		// A replay is answered with the stored row and 200, so a retrying caller
		// can tell "already recorded" from "recorded just now".
		status = http.StatusOK
	}
	respond.JSON(c, status, gin.H{
		"observation": NewObservationView(result.Observation),
		"duplicate":   result.Duplicate,
	})
}

func parseRequired(value, field string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, apperrors.BadRequest(field+" must be an RFC 3339 timestamp.", nil)
	}
	return parsed, nil
}
