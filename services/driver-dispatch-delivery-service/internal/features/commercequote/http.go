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
	"cosmicforge/logistics/shared/go/commercequote"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/logisticscapability"
	"cosmicforge/logistics/shared/go/serviceauth"
	"cosmicforge/logistics/shared/go/supplypreview"
)

const maxQuoteBody = 16 << 10

// Handler serves the internal commerce quote endpoint.
type Handler struct {
	service *Service
}

// NewHandler builds the handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register mounts the quote endpoint behind the same three gates every private
// dispatch surface uses: HMAC v2, a caller allowlist, and single-use nonce
// replay protection. There is no unauthenticated variant and no flag that
// turns them off.
//
// The allowlist matters as much as the signature here. A service holding a
// valid signature for some unrelated dispatch integration must not be able to
// mint delivery quotes.
func Register(
	group *gin.RouterGroup,
	handler *Handler,
	verifier *serviceauth.Verifier,
	nonces authority.NonceClaimer,
	allowedCallers []string,
) {
	if handler == nil || handler.service == nil || verifier == nil {
		return
	}

	quotes := group.Group("/commerce")
	quotes.Use(authority.ServiceBoundary(verifier, nonces, allowedCallers)...)
	quotes.POST("/quotes", handler.quote)
}

// RegisterLogisticsCapability mounts the shared cross-authority capability
// contract Vendor reads before it may select Dispatch (VND-006).
//
// It reuses the same service boundary the quote route uses -- HMAC v2, caller
// allowlist and durable nonce replay protection -- rather than installing a
// second, weaker one for a read. Capability is an internal contract: what
// Dispatch can physically carry is not public information.
func RegisterLogisticsCapability(
	group *gin.RouterGroup,
	provider logisticscapability.Provider,
	verifier *serviceauth.Verifier,
	nonces authority.NonceClaimer,
	allowedCallers []string,
) {
	if group == nil || provider == nil || verifier == nil {
		return
	}
	capability := group.Group("")
	capability.Use(authority.ServiceBoundary(verifier, nonces, allowedCallers)...)
	logisticscapability.RegisterRoutes(capability, provider)
}

func (h *Handler) quote(c *gin.Context) {
	raw, _ := c.Get("service_name")
	caller, _ := raw.(string)

	if strings.TrimSpace(caller) == "" {
		httpx.Abort(c, apperrors.Unauthorized(
			"Authentication is required.", nil,
		))

		return
	}

	body, err := io.ReadAll(
		io.LimitReader(c.Request.Body, maxQuoteBody+1),
	)
	if err != nil || len(body) > maxQuoteBody {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", nil))
		return
	}

	var request commercequote.Request

	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()

	if err = decoder.Decode(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	quote, err := h.service.Quote(c.Request.Context(), caller, request)

	switch {
	case errors.Is(err, ErrInvalidRequest):
		httpx.Abort(c, apperrors.BadRequest(
			"Quote request is invalid.", nil,
		))

		return

	case errors.Is(err, ErrUnavailable):
		// Deliberately a 503 rather than a serviceable-false answer: the
		// caller must be able to tell "we could not check" from "there is
		// nobody", and only one of those two is a fact about riders.
		httpx.Abort(c, apperrors.Unavailable(
			"Delivery quoting is temporarily unavailable.", nil,
		))

		return

	case err != nil:
		httpx.Abort(c, apperrors.Unavailable(
			"Delivery quoting is temporarily unavailable.", nil,
		))

		return
	}

	c.JSON(http.StatusOK, commercequote.Envelope{
		Success: true,
		Data:    quote,
	})
}

// RegisterSupplyPreview mounts the read-only supply preview on the SAME
// internal boundary the quote and capability routes use.
//
// Sharing the boundary rather than building a second one is deliberate: the
// preview is an internal contract, and a surface that quietly installed its own
// auth would be the one that ends up weaker than its neighbours. `enabled` is
// checked here so a disabled deployment has no route at all, rather than a
// route that always answers UNKNOWN.
func RegisterSupplyPreview(
	group *gin.RouterGroup,
	previewer supplypreview.Previewer,
	verifier *serviceauth.Verifier,
	nonces authority.NonceClaimer,
	allowedCallers []string,
	enabled bool,
) {
	if !enabled || group == nil || previewer == nil || verifier == nil {
		return
	}
	preview := group.Group("")
	preview.Use(authority.ServiceBoundary(verifier, nonces, allowedCallers)...)
	supplypreview.RegisterRoutes(preview, previewer)
}

// RegisterInternalCapability mounts an authority-owned internal capability
// surface behind the same boundary.
//
// It takes a registrar rather than a concrete handler so the capability feature
// stays out of this package's imports; the boundary lives here because this is
// where the verifier, nonce store and caller allowlist already are.
func RegisterInternalCapability(
	group *gin.RouterGroup,
	register func(*gin.RouterGroup),
	verifier *serviceauth.Verifier,
	nonces authority.NonceClaimer,
	allowedCallers []string,
) {
	if group == nil || register == nil || verifier == nil {
		return
	}
	internal := group.Group("")
	internal.Use(authority.ServiceBoundary(verifier, nonces, allowedCallers)...)
	register(internal)
}
