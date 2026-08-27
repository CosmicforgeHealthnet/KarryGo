package commercequote

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/authority"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/dispatchcommerce"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const maxMultiStopQuoteBody = 512 << 10

type MultiStopHandler struct{ service *MultiStopService }

func NewMultiStopHandler(service *MultiStopService) *MultiStopHandler {
	return &MultiStopHandler{service: service}
}

func RegisterMultiStop(group *gin.RouterGroup, handler *MultiStopHandler, verifier *serviceauth.Verifier,
	nonces authority.NonceClaimer, allowedCallers []string) {
	if handler == nil || handler.service == nil || verifier == nil {
		return
	}
	commerce := group.Group("/dispatch/v1/commerce")
	commerce.Use(authority.ServiceBoundary(verifier, nonces, allowedCallers)...)
	commerce.POST("/quotes", handler.quote)
	commerce.GET("/capabilities/motorcycle", handler.capability)
}

func (h *MultiStopHandler) quote(c *gin.Context) {
	caller := serviceCaller(c)
	if caller != dispatchcommerce.SignatureCaller {
		httpx.Abort(c, apperrors.Unauthorized("This caller may not request Commerce quotes.", nil))
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxMultiStopQuoteBody+1))
	if err != nil || len(body) > maxMultiStopQuoteBody {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	var request dispatchcommerce.QuoteRequest
	if err = decoder.Decode(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	var trailing any
	if err = decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		httpx.Abort(c, apperrors.BadRequest("Request body must contain exactly one object.", err))
		return
	}
	quote, err := h.service.Quote(c.Request.Context(), caller, request)
	if errors.Is(err, ErrRouteServiceUnavailable) {
		httpx.Abort(c, apperrors.New(http.StatusServiceUnavailable, ReasonRouteUnavailable, "Authoritative route service is unavailable.", err))
		return
	}
	if err != nil {
		httpx.Abort(c, apperrors.BadRequest("Quote request is invalid.", err))
		return
	}
	c.JSON(http.StatusOK, dispatchcommerce.Envelope[dispatchcommerce.Quote]{Success: true, Data: quote})
}

func (h *MultiStopHandler) capability(c *gin.Context) {
	if serviceCaller(c) != dispatchcommerce.SignatureCaller {
		httpx.Abort(c, apperrors.Unauthorized("This caller may not read Commerce capability.", nil))
		return
	}
	c.JSON(http.StatusOK, dispatchcommerce.Envelope[dispatchcommerce.MotorcycleCapability]{Success: true, Data: Capability(h.service.now())})
}

func serviceCaller(c *gin.Context) string {
	raw, _ := c.Get("service_name")
	caller, _ := raw.(string)
	return strings.TrimSpace(caller)
}
