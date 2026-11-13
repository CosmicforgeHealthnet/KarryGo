package organizationproviders

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/orgprovider"
)

// Handler serves both halves of the contract: the private Fleet surface and
// the rider's own association routes.
type Handler struct{ service *Service }

// NewHandler builds the handler.
func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// RegisterInternal mounts the private surface. boundary must authenticate the
// caller (HMAC), enforce the caller allowlist and reject replays; nothing is
// mounted without it.
func RegisterInternal(group *gin.RouterGroup, handler *Handler, boundary ...gin.HandlerFunc) {
	if group == nil || handler == nil || len(boundary) == 0 {
		return
	}
	group.Use(boundary...)
	group.POST("/providers/resolve", handler.resolve)
	group.POST("/providers/sponsored", handler.sponsor)
	group.GET("/providers/:providerId", handler.readProvider)
	group.POST("/provider-links", handler.createLink)
	group.GET("/provider-links/by-provider/:providerId", handler.readLinkByProvider)
	group.GET("/provider-links/:linkId", handler.readLink)
	group.POST("/provider-links/:linkId/transitions", handler.transition)
}

// RegisterProvider mounts the rider's routes. auth must authenticate a rider
// session; providerID extracts the authenticated rider's id from it.
func RegisterProvider(group *gin.RouterGroup, handler *Handler, providerID func(*gin.Context) string, auth ...gin.HandlerFunc) {
	if group == nil || handler == nil || providerID == nil || len(auth) == 0 {
		return
	}
	group.Use(auth...)
	group.GET("", func(c *gin.Context) { handler.list(c, providerID(c)) })
	group.POST("/:linkId/accept", func(c *gin.Context) { handler.decide(c, providerID(c), true) })
	group.POST("/:linkId/decline", func(c *gin.Context) { handler.decide(c, providerID(c), false) })
}

func ok(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"success": true, "data": data})
}

func fail(c *gin.Context, err error) {
	status, code := http.StatusInternalServerError, orgprovider.CodeAuthorityUnavailable
	switch {
	case errors.Is(err, ErrInvalid):
		status, code = http.StatusBadRequest, orgprovider.CodeInvalidRequest
	case errors.Is(err, ErrNotFound):
		status, code = http.StatusNotFound, orgprovider.CodeNotFound
	case errors.Is(err, ErrIdempotencyConflict):
		status, code = http.StatusConflict, orgprovider.CodeIdempotencyConflict
	case errors.Is(err, ErrIdentityConflict):
		status, code = http.StatusConflict, orgprovider.CodeIdentityConflict
	case errors.Is(err, ErrLinkConflict):
		status, code = http.StatusConflict, orgprovider.CodeLinkConflict
	case errors.Is(err, ErrInvalidStateVersion), errors.Is(err, ErrDecisionNotAvailable):
		status, code = http.StatusConflict, orgprovider.CodeInvalidStateVersion
	case errors.Is(err, ErrIneligible):
		status, code = http.StatusUnprocessableEntity, orgprovider.CodeProviderIneligible
	case errors.Is(err, ErrDisabled):
		status, code = http.StatusServiceUnavailable, orgprovider.CodeIntegrationDisabled
	default:
		log.Printf("organization providers: %v", err)
	}
	c.AbortWithStatusJSON(status, gin.H{"success": false, "error": gin.H{"code": code}})
}

func caller(c *gin.Context) string {
	name, _ := c.Get("service_name")
	value, _ := name.(string)
	return value
}

func correlation(c *gin.Context) string {
	if value := strings.TrimSpace(c.GetHeader("X-Correlation-ID")); value != "" {
		return value
	}
	return strings.TrimSpace(c.GetHeader("X-Request-ID"))
}

func (h *Handler) resolve(c *gin.Context) {
	var body orgprovider.ResolveRequest
	if c.ShouldBindJSON(&body) != nil {
		fail(c, ErrInvalid)
		return
	}
	out, err := h.service.Resolve(c.Request.Context(), body)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, out)
}

func (h *Handler) sponsor(c *gin.Context) {
	var body orgprovider.SponsoredRequest
	if c.ShouldBindJSON(&body) != nil {
		fail(c, ErrInvalid)
		return
	}
	out, err := h.service.ProvisionSponsored(c.Request.Context(), c.GetHeader("Idempotency-Key"), body)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusCreated, out)
}

func (h *Handler) readProvider(c *gin.Context) {
	out, err := h.service.ReadProvider(c.Request.Context(), c.Param("providerId"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, out)
}

func (h *Handler) createLink(c *gin.Context) {
	var body orgprovider.LinkRequest
	if c.ShouldBindJSON(&body) != nil {
		fail(c, ErrInvalid)
		return
	}
	out, created, err := h.service.CreateLink(c.Request.Context(), c.GetHeader("Idempotency-Key"),
		c.GetHeader("X-Organization-ID"), caller(c), correlation(c), body)
	if err != nil {
		fail(c, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	ok(c, status, out.LinkView)
}

func (h *Handler) readLink(c *gin.Context) {
	out, err := h.service.ReadLink(c.Request.Context(), c.Param("linkId"), c.GetHeader("X-Organization-ID"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, out.LinkView)
}

func (h *Handler) readLinkByProvider(c *gin.Context) {
	out, err := h.service.ReadLinkByProvider(c.Request.Context(), c.Param("providerId"), c.GetHeader("X-Organization-ID"))
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, out.LinkView)
}

func (h *Handler) transition(c *gin.Context) {
	var body orgprovider.TransitionRequest
	if c.ShouldBindJSON(&body) != nil {
		fail(c, ErrInvalid)
		return
	}
	out, err := h.service.TransitionLink(c.Request.Context(), c.Param("linkId"), c.GetHeader("Idempotency-Key"),
		c.GetHeader("X-Organization-ID"), caller(c), correlation(c), body)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, out.LinkView)
}

func (h *Handler) list(c *gin.Context, providerID string) {
	out, err := h.service.ListForProvider(c.Request.Context(), providerID)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, gin.H{"associations": out})
}

func (h *Handler) decide(c *gin.Context, providerID string, accept bool) {
	var body orgprovider.ProviderDecisionRequest
	if c.Request.ContentLength > 0 && c.ShouldBindJSON(&body) != nil {
		fail(c, ErrInvalid)
		return
	}
	out, err := h.service.Decide(c.Request.Context(), providerID, c.Param("linkId"), accept, body.ExpectedVersion)
	if err != nil {
		fail(c, err)
		return
	}
	ok(c, http.StatusOK, out)
}
