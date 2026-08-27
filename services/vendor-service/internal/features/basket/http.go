package basket

// The customer HTTP surface.
//
// Every route authenticates the customer from the verified Customer Service
// bearer token and takes the subject from there and nowhere else. No request
// body carries a customer id, a price, a delivery fee, a vendor selection, a
// stock level or a winning variant. That is the security model in one
// sentence: the client states what it WANTS and the server states what things
// COST, and no field crosses that line.
//
// The routes divide cleanly by side effect, which is the distinction the
// section asks for:
//
//	READ / QUOTE-ONLY   optimize, budget, read a proposal, read a template
//	MUTATION            accept a proposal, create or edit a template
//
// The optimize routes persist proposal evidence, which is a write, but they
// change nothing the customer owns. The cart is untouched until an explicit
// acceptance, which is its own route with its own confirmation.

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

// Handler serves the basket optimization API.
type Handler struct {
	service *Service
}

// NewHandler builds the handler.
func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// customerSubject resolves the customer from the verified bearer token.
//
// The same check the fulfilment surface uses, for the same reason: a customer
// id taken from a request body would let anyone read or rewrite anyone's
// shopping by typing a different string.
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
	case errors.Is(err, ErrProposalNotFound),
		errors.Is(err, ErrTemplateNotFound):
		httpx.RespondError(c, apperrors.NotFound(
			"That was not found.", nil,
		))

	case errors.Is(err, ErrProposalExpired):
		httpx.RespondError(c, apperrors.Conflict(
			"This suggestion has expired. Refresh it and try again.", nil,
		))

	case errors.Is(err, ErrProposalStale),
		errors.Is(err, ErrTemplateConflict):
		httpx.RespondError(c, apperrors.Conflict(
			"This changed while you were deciding. Reload and try again.", nil,
		))

	case errors.Is(err, ErrTemplateDisabled):
		httpx.RespondError(c, apperrors.Conflict(
			"This recurring basket is paused.", nil,
		))

	case errors.Is(err, ErrNoRequirements),
		errors.Is(err, ErrTooManyRequirements),
		errors.Is(err, ErrInvalidRequirement),
		errors.Is(err, ErrInvalidSchedule),
		errors.Is(err, ErrNoDestination),
		errors.Is(err, ErrCurrencyRequired):
		httpx.RespondError(c, apperrors.Validation(err.Error(), nil))

	case errors.Is(err, ErrDependencyMissing):
		httpx.RespondError(c, apperrors.Unavailable(
			"Basket optimization is temporarily unavailable.", nil,
		))

	default:
		httpx.RespondError(c, apperrors.Internal(
			"Basket optimization failed.", err,
		))
	}
}

func respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

// ---------------------------------------------------------------------------
// Budget shopping
// ---------------------------------------------------------------------------

// optimizeRequest is the budget-shopping contract.
//
// Note what is NOT here: no price, no delivery fee, no vendor, no listing the
// client claims is cheapest, no total. The client says what it needs and what
// it will spend; every number in the answer is computed server-side from
// authoritative catalogue, inventory and logistics data.
type optimizeRequest struct {
	Currency             string        `json:"currency"`
	BudgetMinor          int64         `json:"budget_minor"`
	AddressRef           string        `json:"address_ref"`
	FulfilmentPreference string        `json:"fulfilment_preference"`
	Objectives           []string      `json:"objectives"`
	Requirements         []Requirement `json:"requirements"`
}

func (h *Handler) optimizeBudget(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}

	var body optimizeRequest

	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.RespondError(c, apperrors.Validation(
			"Check your shopping list.", nil,
		))

		return
	}

	objectives, err := parseObjectives(body.Objectives)
	if err != nil {
		respondError(c, err)

		return
	}

	// The destination is resolved from the customer's OWN saved location, by
	// reference. Accepting coordinates from the body would let anyone price a
	// delivery to an address they have no connection to.
	destination, err := h.service.resolveCustomerAddress(
		c.Request.Context(), customerID, body.AddressRef,
	)
	if err != nil {
		respondError(c, err)

		return
	}

	result, err := h.service.Optimize(c.Request.Context(), Request{
		CustomerID:           customerID,
		Currency:             body.Currency,
		BudgetMinor:          body.BudgetMinor,
		Destination:          destination,
		FulfilmentPreference: body.FulfilmentPreference,
		Requirements:         body.Requirements,
		Objectives:           objectives,
	})
	if err != nil {
		respondError(c, err)

		return
	}

	// A result that fits no budget is a successful answer, not an error. The
	// customer is being told precisely how far short they are and which
	// constraint is holding it there, which is more useful than a 4xx that
	// carries none of it.
	respondOK(c, result)
}

func parseObjectives(values []string) ([]Objective, error) {
	objectives := make([]Objective, 0, len(values))

	for _, value := range values {
		objective := Objective(strings.ToUpper(strings.TrimSpace(value)))

		if !objective.Valid() {
			return nil, ErrInvalidRequirement
		}

		objectives = append(objectives, objective)
	}

	return objectives, nil
}

// ---------------------------------------------------------------------------
// Optimize My Cart
// ---------------------------------------------------------------------------

type optimizeCartRequest struct {
	SubstitutionAllowance string   `json:"substitution_allowance"`
	FulfilmentPreference  string   `json:"fulfilment_preference"`
	Objectives            []string `json:"objectives"`
}

func (h *Handler) optimizeCart(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}

	var body optimizeCartRequest

	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			httpx.RespondError(c, apperrors.Validation(
				"Check your optimization options.", nil,
			))

			return
		}
	}

	allowance, err := ParseSubstitutionAllowance(body.SubstitutionAllowance)
	if err != nil {
		respondError(c, err)

		return
	}

	objectives, err := parseObjectives(body.Objectives)
	if err != nil {
		respondError(c, err)

		return
	}

	result, err := h.service.OptimizeCart(
		c.Request.Context(), customerID, allowance,
		body.FulfilmentPreference, objectives,
	)
	if err != nil {
		respondError(c, err)

		return
	}

	respondOK(c, result)
}

func (h *Handler) getProposal(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}

	proposal, err := h.service.GetProposal(
		c.Request.Context(), customerID, c.Param("proposalId"),
	)
	if err != nil {
		respondError(c, err)

		return
	}

	respondOK(c, proposal)
}

// acceptProposal is the ONE route in this feature that changes the customer's
// cart.
//
// It takes no body at all. The proposal identifies the basket, and everything
// about that basket was computed server-side; accepting a client-supplied
// total or line list here would let a customer approve one thing and receive
// another.
func (h *Handler) acceptProposal(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}

	result, err := h.service.AcceptProposal(
		c.Request.Context(), customerID, c.Param("proposalId"),
	)
	if err != nil {
		respondError(c, err)

		return
	}

	respondOK(c, result)
}

// ---------------------------------------------------------------------------
// Recurring baskets
// ---------------------------------------------------------------------------

type templateRequest struct {
	Name                 string        `json:"name"`
	Currency             string        `json:"currency"`
	AddressRef           string        `json:"address_ref"`
	Requirements         []Requirement `json:"requirements"`
	FulfilmentPreference string        `json:"fulfilment_preference"`
	Objective            string        `json:"objective"`
	ApprovalPolicy       string        `json:"approval_policy"`
	CeilingMinor         int64         `json:"ceiling_minor"`
	Cadence              string        `json:"cadence"`
	Timezone             string        `json:"timezone"`
	LocalHour            int           `json:"local_hour"`
	LocalMinute          int           `json:"local_minute"`
	Weekday              int           `json:"weekday"`
	DayOfMonth           int           `json:"day_of_month"`
	Enabled              *bool         `json:"enabled"`
}

func (r templateRequest) toTemplate(customerID string) Template {
	enabled := true

	if r.Enabled != nil {
		enabled = *r.Enabled
	}

	return Template{
		CustomerID:           customerID,
		Name:                 r.Name,
		Currency:             r.Currency,
		AddressRef:           r.AddressRef,
		Requirements:         r.Requirements,
		FulfilmentPreference: r.FulfilmentPreference,
		Objective:            Objective(strings.ToUpper(strings.TrimSpace(r.Objective))),
		ApprovalPolicy:       ApprovalPolicy(strings.ToUpper(strings.TrimSpace(r.ApprovalPolicy))),
		CeilingMinor:         r.CeilingMinor,
		Cadence:              Cadence(strings.ToUpper(strings.TrimSpace(r.Cadence))),
		Timezone:             r.Timezone,
		LocalHour:            r.LocalHour,
		LocalMinute:          r.LocalMinute,
		Weekday:              r.Weekday,
		DayOfMonth:           r.DayOfMonth,
		Enabled:              enabled,
	}
}

func (h *Handler) createTemplate(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}

	var body templateRequest

	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.RespondError(c, apperrors.Validation(
			"Check your recurring basket.", nil,
		))

		return
	}

	template, err := h.service.CreateTemplate(
		c.Request.Context(), body.toTemplate(customerID),
	)
	if err != nil {
		respondError(c, err)

		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": template})
}

func (h *Handler) listTemplates(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}

	templates, err := h.service.ListTemplates(
		c.Request.Context(), customerID, parseLimit(c),
	)
	if err != nil {
		respondError(c, err)

		return
	}

	respondOK(c, gin.H{"templates": templates})
}

func (h *Handler) getTemplate(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}

	template, err := h.service.GetTemplate(
		c.Request.Context(), customerID, c.Param("templateId"),
	)
	if err != nil {
		respondError(c, err)

		return
	}

	respondOK(c, template)
}

func (h *Handler) updateTemplate(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}

	var body templateRequest

	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.RespondError(c, apperrors.Validation(
			"Check your recurring basket.", nil,
		))

		return
	}

	// The expected version is REQUIRED, not optional.
	//
	// Two people editing one household's recurring basket must not silently
	// overwrite each other, and an edit that declines to say what it was
	// based on cannot be checked. Refusing it is better than accepting a
	// last-write-wins update to somebody's standing grocery order.
	expected, ok := expectedVersion(c)
	if !ok {
		return
	}

	template := body.toTemplate(customerID)
	template.ID = c.Param("templateId")

	updated, err := h.service.UpdateTemplate(
		c.Request.Context(), template, expected,
	)
	if err != nil {
		respondError(c, err)

		return
	}

	respondOK(c, updated)
}

type templateStateRequest struct {
	Enabled bool `json:"enabled"`
}

func (h *Handler) setTemplateState(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}

	var body templateStateRequest

	if err := c.ShouldBindJSON(&body); err != nil {
		httpx.RespondError(c, apperrors.Validation(
			"Say whether the recurring basket should be active.", nil,
		))

		return
	}

	expected, ok := expectedVersion(c)
	if !ok {
		return
	}

	template, err := h.service.SetTemplateEnabled(
		c.Request.Context(), customerID, c.Param("templateId"),
		body.Enabled, expected,
	)
	if err != nil {
		respondError(c, err)

		return
	}

	respondOK(c, template)
}

func (h *Handler) listOccurrences(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}

	occurrences, err := h.service.ListOccurrences(
		c.Request.Context(), customerID, c.Param("templateId"),
		parseLimit(c),
	)
	if err != nil {
		respondError(c, err)

		return
	}

	respondOK(c, gin.H{"occurrences": occurrences})
}

// generateNow shops a recurring list on demand.
//
// It goes through exactly the same claim-first path as the scheduled sweep,
// so a customer tapping this while the worker is generating the same
// occurrence still produces one generation rather than two.
func (h *Handler) generateNow(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}

	generation, err := h.service.GenerateNow(
		c.Request.Context(), customerID, c.Param("templateId"),
	)
	if err != nil {
		respondError(c, err)

		return
	}

	respondOK(c, generation)
}

func expectedVersion(c *gin.Context) (int64, bool) {
	raw := strings.TrimSpace(c.Query("expected_version"))

	if raw == "" {
		raw = strings.TrimSpace(c.GetHeader("If-Match"))
	}

	version, err := strconv.ParseInt(raw, 10, 64)

	if err != nil || version <= 0 {
		httpx.RespondError(c, apperrors.Validation(
			"Provide the version you are editing.", nil,
		))

		return 0, false
	}

	return version, true
}

func parseLimit(c *gin.Context) int {
	limit, err := strconv.Atoi(strings.TrimSpace(c.Query("limit")))

	if err != nil || limit <= 0 {
		return 0
	}

	return limit
}

// RegisterCustomerRoutes mounts the basket optimization surface.
//
// With the feature disabled no route is registered at all, so a deployment
// that has not enabled optimization has no surface rather than routes that
// answer 503 and confirm one exists.
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

	group := root.Group("/basket")
	group.Use(sharedauth.BearerMiddleware(customerSigner, "customer", "customer"))

	group.POST("/optimize", handler.optimizeBudget)
	group.POST("/cart/optimize", handler.optimizeCart)

	group.GET("/proposals/:proposalId", handler.getProposal)
	group.POST("/proposals/:proposalId/accept", handler.acceptProposal)

	group.POST("/recurring", handler.createTemplate)
	group.GET("/recurring", handler.listTemplates)
	group.GET("/recurring/:templateId", handler.getTemplate)
	group.PUT("/recurring/:templateId", handler.updateTemplate)
	group.PUT("/recurring/:templateId/state", handler.setTemplateState)
	group.GET("/recurring/:templateId/occurrences", handler.listOccurrences)
	group.POST("/recurring/:templateId/generate", handler.generateNow)
}
