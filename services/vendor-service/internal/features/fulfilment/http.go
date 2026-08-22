package fulfilment

// Section 17C customer HTTP surface.
//
// Two routes: build a plan for a checkout session, and read the plan back.
//
// The request contract carries NO fee, no business id, no branch id and no
// customer id. That is deliberate and it is the security model: a customer
// cannot overwrite a logistics fee from a request payload, and a vendor cannot
// inject another branch as a pickup, because neither value is accepted as
// input anywhere in this surface.

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

// Handler serves the customer fulfilment planning API.
type Handler struct {
	service *PlanningService
}

// NewHandler builds the handler.
func NewHandler(service *PlanningService) *Handler {
	return &Handler{service: service}
}

// customerSubject resolves the customer from the verified Customer Service
// bearer token and nowhere else.
func customerSubject(c *gin.Context) (string, bool) {
	claims, ok := sharedauth.ClaimsFromContext(c)

	if !ok ||
		claims.Type != sharedauth.TokenTypeAccess ||
		claims.Role != "customer" ||
		claims.Service != "customer" ||
		strings.TrimSpace(claims.Subject) == "" {
		httpx.RespondError(c, apperrors.Unauthorized(
			"Authentication is required.", nil,
		))

		return "", false
	}

	return strings.TrimSpace(claims.Subject), true
}

func respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrPlanNotFound):
		httpx.RespondError(c, apperrors.NotFound(
			"Fulfilment plan was not found.", nil,
		))

	case errors.Is(err, ErrCheckoutNotPlannable):
		httpx.RespondError(c, apperrors.Conflict(
			"This checkout cannot be planned. Revalidate it and try again.",
			nil,
		))

	case errors.Is(err, ErrPlanConflict):
		httpx.RespondError(c, apperrors.Conflict(
			"This plan changed. Reload and try again.", nil,
		))

	case errors.Is(err, ErrDependencyMissing):
		httpx.RespondError(c, apperrors.Unavailable(
			"Delivery planning is temporarily unavailable.", nil,
		))

	default:
		httpx.RespondError(c, apperrors.Internal(
			"Delivery planning failed.", err,
		))
	}
}

func (h *Handler) createPlan(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}

	plan, err := h.service.Plan(
		c.Request.Context(), customerID, c.Param("sessionId"),
	)
	if err != nil {
		respondError(c, err)
		return
	}

	// An unserviceable plan is a successful planning outcome, not a server
	// error: the customer is being told, precisely, which vendors cannot be
	// delivered and why. 200 with the reasons is more useful than a 4xx that
	// carries none of them.
	c.JSON(http.StatusOK, gin.H{"success": true, "data": plan})
}

func (h *Handler) getPlanForCheckout(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}

	plan, err := h.service.GetForCheckout(
		c.Request.Context(), customerID, c.Param("sessionId"),
	)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": plan})
}

// RegisterCustomerRoutes mounts the Section 17C customer surface.
//
// With the feature disabled no route is registered at all, so a deployment
// that has not enabled planning has no surface rather than routes that answer
// 503 and confirm it exists.
func RegisterCustomerRoutes(
	root *gin.RouterGroup,
	handler *Handler,
	customerSigner *sharedauth.TokenSigner,
	enabled bool,
) {
	if root == nil || handler == nil || handler.service == nil ||
		customerSigner == nil || !enabled {
		return
	}

	group := root.Group("/checkout/:sessionId/fulfilment-plan")
	group.Use(sharedauth.BearerMiddleware(customerSigner, "customer", "customer"))

	group.POST("", handler.createPlan)
	group.GET("", handler.getPlanForCheckout)
}
