package commercequote

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/hauling-service/internal/features/fleetauthority"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/commercequote"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
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
// hauling surface uses: HMAC v2, a caller allowlist, and single-use nonce
// replay protection (shared with fleetauthority so there is exactly one
// implementation of replay rejection in this service).
//
// The allowlist matters as much as the signature. A service holding a valid
// signature for some unrelated hauling integration must not be able to mint
// delivery quotes.
func Register(
	group *gin.RouterGroup,
	handler *Handler,
	db *pgxpool.Pool,
	verifier *serviceauth.Verifier,
	allowedCallers []string,
) {
	if handler == nil || handler.service == nil ||
		verifier == nil || db == nil {
		return
	}

	quotes := group.Group("/internal/commerce")
	quotes.Use(fleetauthority.BoundaryWithDB(db, verifier, allowedCallers)...)
	quotes.POST("/quotes", handler.quote)
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

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxQuoteBody+1))
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
		httpx.Abort(c, apperrors.BadRequest("Quote request is invalid.", nil))
		return

	case err != nil:
		// Deliberately a 503 rather than a serviceable-false answer: the
		// caller must be able to tell "we could not check" from "there is
		// nothing", and only one of those is a fact about trucks.
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
