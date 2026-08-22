package checkout

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

const maxCheckoutCommandBody = 8 << 10

type startCheckoutRequest struct {
	CartID string `json:"cart_id"`
}

func strictCheckoutJSON(
	c *gin.Context,
	target any,
) error {
	body, err := io.ReadAll(
		io.LimitReader(
			c.Request.Body,
			maxCheckoutCommandBody+1,
		),
	)
	if err != nil {
		return apperrors.BadRequest(
			"Request body is invalid.",
			err,
		)
	}

	if len(body) > maxCheckoutCommandBody {
		return apperrors.BadRequest(
			"Request body is too large.",
			nil,
		)
	}

	decoder := json.NewDecoder(
		bytes.NewReader(body),
	)
	decoder.DisallowUnknownFields()

	if err = decoder.Decode(target); err != nil {
		return apperrors.BadRequest(
			"Request body is invalid.",
			err,
		)
	}

	var trailing any

	if err = decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return apperrors.BadRequest(
			"Request body must contain exactly one JSON object.",
			err,
		)
	}

	return nil
}

// customerSubject resolves the customer from the verified Customer Service
// bearer token and nowhere else.
//
// There is deliberately no customerId path, query parameter or body field
// anywhere in the Section 17B contract.
func customerSubject(
	c *gin.Context,
) (string, bool) {
	claims, ok := sharedauth.ClaimsFromContext(c)

	if !ok ||
		claims.Type != sharedauth.TokenTypeAccess ||
		claims.Role != "customer" ||
		claims.Service != "customer" ||
		strings.TrimSpace(claims.Subject) == "" {
		httpx.RespondError(
			c,
			apperrors.Unauthorized(
				"Authentication is required.",
				nil,
			),
		)

		return "", false
	}

	return strings.TrimSpace(claims.Subject), true
}

// expectedCartVersion reads the cart version this checkout was requested
// against. It is required: validating a cart the client has not actually seen
// would be exactly the stale-checkout bug this section exists to prevent.
func expectedCartVersion(
	c *gin.Context,
) (int64, error) {
	raw := strings.TrimSpace(
		c.GetHeader("If-Match"),
	)

	if raw == "" {
		return 0, apperrors.Validation(
			"Check your details.",
			[]apperrors.FieldViolation{
				{
					Field:   "If-Match",
					Message: "The current cart version is required.",
				},
			},
		)
	}

	if strings.HasPrefix(
		strings.ToUpper(raw),
		"W/",
	) {
		return 0, apperrors.Validation(
			"Check your details.",
			[]apperrors.FieldViolation{
				{
					Field:   "If-Match",
					Message: "Weak entity tags are not accepted for checkout.",
				},
			},
		)
	}

	if strings.HasPrefix(raw, `"`) &&
		strings.HasSuffix(raw, `"`) &&
		len(raw) >= 2 {
		raw = strings.TrimSuffix(
			strings.TrimPrefix(raw, `"`),
			`"`,
		)
	}

	version, err := strconv.ParseInt(
		strings.TrimSpace(raw),
		10,
		64,
	)

	if err != nil || version < 1 {
		return 0, apperrors.Validation(
			"Check your details.",
			[]apperrors.FieldViolation{
				{
					Field:   "If-Match",
					Message: "The cart version must be a positive integer.",
				},
			},
		)
	}

	return version, nil
}

func checkoutHTTPError(err error) error {
	var precondition CartPreconditionError

	if errors.As(err, &precondition) {
		switch precondition.Code {
		case PreconditionDeliveryAddress:
			return apperrors.Conflict(
				"Choose a delivery address before checking out.",
				err,
			)

		case PreconditionCartEmpty:
			return apperrors.Conflict(
				"This cart is empty.",
				err,
			)

		default:
			return apperrors.Conflict(
				"This cart cannot be checked out right now.",
				err,
			)
		}
	}

	switch {
	case errors.Is(err, ErrSessionNotFound):
		return apperrors.NotFound(
			"Checkout session not found.",
			err,
		)

	case errors.Is(err, ErrSessionNotLive):
		return apperrors.Conflict(
			"This checkout session has already finished.",
			err,
		)

	case errors.Is(err, ErrCheckoutConflict):
		return apperrors.Conflict(
			"This cart already has a checkout in progress.",
			err,
		)

	case errors.Is(err, ErrCartNotCheckoutable):
		return apperrors.Conflict(
			"This cart cannot be checked out right now.",
			err,
		)

	case errors.Is(err, cart.ErrCartNotFound):
		return apperrors.NotFound(
			"Cart not found.",
			err,
		)

	case errors.Is(err, cart.ErrVersionConflict):
		return apperrors.Conflict(
			"The cart changed. Reload it and retry with the current version.",
			err,
		)

	case errors.Is(err, cart.ErrCartEmpty):
		return apperrors.Conflict(
			"This cart is empty.",
			err,
		)

	case errors.Is(err, cart.ErrCartNotMutable),
		errors.Is(err, cart.ErrCheckoutAlreadyInProgress):
		return apperrors.Conflict(
			"This cart already has a checkout in progress.",
			err,
		)

	case errors.Is(err, ErrMoneyOverflow):
		return apperrors.Conflict(
			"This cart total is too large to check out.",
			err,
		)

	case errors.Is(err, ErrDependencyMissing),
		errors.Is(err, ErrInvalidTTL):
		return apperrors.Internal(
			"A required commerce authority is unavailable.",
			err,
		)

	default:
		// Food, Shop and Inventory already return platform application
		// errors. Preserve them rather than flattening their domain code.
		return err
	}
}

func respondSession(
	c *gin.Context,
	status int,
	session Session,
) {
	c.Header(
		"ETag",
		`"`+strconv.FormatInt(session.Version, 10)+`"`,
	)

	c.JSON(
		status,
		gin.H{
			"success": true,
			"data":    session,
		},
	)
}

// CustomerHandler is the Section 17B customer HTTP surface.
type CustomerHandler struct {
	service *Service
}

func NewCustomerHandler(service *Service) *CustomerHandler {
	return &CustomerHandler{service: service}
}

// start validates a cart and, for Shop, holds the stock it needs.
//
// A refused candidate is still a created session: it carries the per-line
// reasons the customer needs in order to fix the cart, so it is returned as a
// resource rather than as an error envelope that could not hold them.
func (h *CustomerHandler) start(c *gin.Context) {
	customerID, ok := customerSubject(c)

	if !ok {
		return
	}

	version, err := expectedCartVersion(c)

	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	var request startCheckoutRequest

	if err = strictCheckoutJSON(c, &request); err != nil {
		httpx.RespondError(c, err)
		return
	}

	if strings.TrimSpace(request.CartID) == "" {
		httpx.RespondError(
			c,
			apperrors.Validation(
				"Check your details.",
				[]apperrors.FieldViolation{
					{
						Field:   "cart_id",
						Message: "A cart is required.",
					},
				},
			),
		)

		return
	}

	session, err := h.service.StartCheckout(
		c,
		customerID,
		request.CartID,
		version,
	)
	if err != nil {
		httpx.RespondError(c, checkoutHTTPError(err))
		return
	}

	respondSession(c, http.StatusCreated, session)
}

func (h *CustomerHandler) get(c *gin.Context) {
	customerID, ok := customerSubject(c)

	if !ok {
		return
	}

	session, err := h.service.GetSession(
		c,
		customerID,
		c.Param("sessionId"),
	)
	if err != nil {
		httpx.RespondError(c, checkoutHTTPError(err))
		return
	}

	respondSession(c, http.StatusOK, session)
}

func (h *CustomerHandler) cancel(c *gin.Context) {
	customerID, ok := customerSubject(c)

	if !ok {
		return
	}

	session, err := h.service.CancelSession(
		c,
		customerID,
		c.Param("sessionId"),
	)
	if err != nil {
		httpx.RespondError(c, checkoutHTTPError(err))
		return
	}

	respondSession(c, http.StatusOK, session)
}

// RegisterCustomerRoutes mounts the Section 17B customer surface.
//
// Three routes, and deliberately no more: there is no quote route, no payment
// route, no order route and no vendor-facing checkout route anywhere in this
// section.
func RegisterCustomerRoutes(
	root *gin.RouterGroup,
	service *Service,
	customerSigner *sharedauth.TokenSigner,
	idempotencyStore *idempotency.Store,
	enabled bool,
) {
	if root == nil ||
		service == nil ||
		customerSigner == nil ||
		!enabled {
		return
	}

	handler := NewCustomerHandler(service)

	sessions := root.Group("/checkout-sessions")

	sessions.Use(
		sharedauth.BearerMiddleware(
			customerSigner,
			"customer",
			"customer",
		),
	)

	sessions.Use(
		idempotency.CustomerCommandMiddleware(
			idempotencyStore,
		),
	)

	sessions.POST("", handler.start)
	sessions.GET("/:sessionId", handler.get)
	sessions.POST("/:sessionId/cancel", handler.cancel)
}
