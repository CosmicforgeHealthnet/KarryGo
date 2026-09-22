package supporthttp

// Section 15 HTTP surface.
//
// Complainants may attach context to their own complaint -- they know which
// trip they mean. Everything else is agent-only: reading an owner's view of a
// job, seeing snapshots, and asking an owner to act. Those are support
// operations, and exposing them to a complainant would let a customer trigger
// another service's recovery process directly.

import (
	nethttp "net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"

	supportmodels "cosmicforge/logistics/services/support-dispute-service/internal/features/support/models"
	supportusecases "cosmicforge/logistics/services/support-dispute-service/internal/features/support/usecases"
)

// NetworkContextHandler serves Section 15.
type NetworkContextHandler struct {
	service *supportusecases.NetworkContextService
}

// NewNetworkContextHandler builds the handler.
func NewNetworkContextHandler(service *supportusecases.NetworkContextService) *NetworkContextHandler {
	return &NetworkContextHandler{service: service}
}

// RegisterNetworkContextRoutes mounts Section 15 behind the same boundaries the
// rest of Support uses: bearer auth for complainants, and HMAC plus the
// admin-backoffice caller for agents. It builds its own groups so the existing
// route registration -- and the tests that call it -- are unchanged.
func RegisterNetworkContextRoutes(group *gin.RouterGroup, handler *NetworkContextHandler,
	customerAccessSecret []byte, providerGroups []ProviderGroup, serviceSecrets serviceauth.Secrets) {
	if group == nil || handler == nil || handler.service == nil {
		return
	}
	customer := group.Group("")
	customer.Use(sharedauth.BearerMiddleware(
		sharedauth.NewTokenSigner(customerAccessSecret), "customer", "customer"))
	customer.PUT("/complaints/:id/context", handler.attachOwn)
	customer.POST("/complaints/:id/evidence/references", handler.referenceEvidence)

	for _, pg := range providerGroups {
		if len(pg.Secret) == 0 {
			continue
		}
		provider := group.Group(pg.Prefix)
		provider.Use(sharedauth.BearerMiddleware(sharedauth.NewTokenSigner(pg.Secret), pg.Role, pg.Service))
		provider.PUT("/complaints/:id/context", handler.attachOwn)
		provider.POST("/complaints/:id/evidence/references", handler.referenceEvidence)
	}

	admin := group.Group("/admin")
	admin.Use(serviceauth.Middleware(serviceauth.NewVerifier(serviceSecrets, 5*time.Minute)),
		requireService("admin-backoffice-service"))
	admin.PUT("/complaints/:id/context", handler.attachAdmin)
	admin.POST("/complaints/:id/context/enrich", handler.enrich)
	admin.GET("/complaints/:id/context/snapshots", handler.snapshots)
	admin.POST("/complaints/:id/actions", handler.requestAction)
	admin.GET("/complaints/:id/actions", handler.listActions)
}

func (h *NetworkContextHandler) attachOwn(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	var refs supportmodels.ContextRefs
	if err := c.ShouldBindJSON(&refs); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Invalid context.", err))
		return
	}
	result, err := h.service.AttachContext(c.Request.Context(), supportusecases.AttachContextInput{
		ComplaintID: c.Param("id"), RequesterType: complainantTypeFromRole(claims.Role),
		RequesterID: claims.Subject, Refs: refs,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func (h *NetworkContextHandler) attachAdmin(c *gin.Context) {
	var refs supportmodels.ContextRefs
	if err := c.ShouldBindJSON(&refs); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Invalid context.", err))
		return
	}
	result, err := h.service.AttachContext(c.Request.Context(), supportusecases.AttachContextInput{
		ComplaintID: c.Param("id"), Admin: true, Refs: refs,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func (h *NetworkContextHandler) enrich(c *gin.Context) {
	snapshot, err := h.service.Enrich(c.Request.Context(), c.Param("id"), correlationOf(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, snapshot)
}

func (h *NetworkContextHandler) snapshots(c *gin.Context) {
	snapshots, err := h.service.ListSnapshots(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, gin.H{"snapshots": snapshots})
}

type actionRequestBody struct {
	ActionCode  string            `json:"action_code"`
	Parameters  map[string]string `json:"parameters"`
	RequestedBy string            `json:"requested_by"`
}

func (h *NetworkContextHandler) requestAction(c *gin.Context) {
	var body actionRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Invalid action request.", err))
		return
	}
	action, err := h.service.RequestAction(c.Request.Context(), supportusecases.RequestActionInput{
		ComplaintID: c.Param("id"), ActionCode: body.ActionCode, Parameters: body.Parameters,
		IdempotencyKey: strings.TrimSpace(c.GetHeader("Idempotency-Key")),
		RequestedBy:    body.RequestedBy, CorrelationID: correlationOf(c),
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, action)
}

func (h *NetworkContextHandler) listActions(c *gin.Context) {
	actions, err := h.service.ListActions(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, gin.H{"actions": actions})
}

// correlationOf reuses the inbound correlation rather than minting a new one,
// so a support action can be traced back to the request that caused it.
func correlationOf(c *gin.Context) string {
	for _, header := range []string{"X-Correlation-ID", "X-Request-ID"} {
		if value := strings.TrimSpace(c.GetHeader(header)); value != "" {
			return value
		}
	}
	return httpx.GetRequestID(c)
}

// referenceEvidence cites a Media object on the caller's own complaint.
func (h *NetworkContextHandler) referenceEvidence(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	var body struct {
		MediaObjectID string `json:"media_object_id"`
		BindingType   string `json:"binding_type"`
		Note          string `json:"note"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Invalid evidence reference.", err))
		return
	}
	evidence, err := h.service.ReferenceEvidence(c.Request.Context(), supportusecases.ReferenceEvidenceInput{
		ComplaintID: c.Param("id"), MediaObjectID: body.MediaObjectID, BindingType: body.BindingType,
		Note: body.Note, RequesterType: complainantTypeFromRole(claims.Role), RequesterID: claims.Subject,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respond(c, nethttp.StatusCreated, evidence)
}
