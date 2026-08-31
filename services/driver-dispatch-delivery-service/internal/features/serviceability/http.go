package serviceability

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/apiresponse"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func RegisterRoutes(engine *gin.Engine, customerSigner *sharedauth.TokenSigner, handler *Handler) {
	group := engine.Group("/api/v1/customer/dispatch")
	group.Use(sharedauth.BearerMiddleware(customerSigner, "customer", "customer"))
	group.POST("/serviceability", handler.Evaluate)
}

func (h *Handler) Evaluate(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok || strings.TrimSpace(claims.Subject) == "" {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" || len(key) > 200 {
		httpx.Abort(c, apperrors.BadRequest("A valid Idempotency-Key header is required.", nil))
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128<<10)
	var request Request
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	quote, err := h.service.Evaluate(c.Request.Context(), claims.Subject, key, request)
	if err != nil {
		switch {
		case errors.Is(err, ErrFingerprintConflict):
			httpx.Abort(c, apperrors.Conflict("Idempotency key was already used for a different request.", err))
		case errors.Is(err, ErrUnavailable):
			httpx.Abort(c, apperrors.Unavailable("Serviceability is temporarily unavailable.", err))
		default:
			httpx.Abort(c, apperrors.BadRequest("Serviceability request is invalid.", err))
		}
		return
	}
	apiresponse.RespondSuccess(c, http.StatusCreated, "Serviceability evaluated.", quote)
}
