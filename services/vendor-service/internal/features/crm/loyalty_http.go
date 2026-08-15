package crm

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/shared/go/httpx"
)

// Section 8D permission codes.
//
// Four, because reading a balance, redefining the programme, moving points by
// hand and undoing a posted entry are genuinely different authorities. There
// is no separate "config.manage": the programme is a single small object and
// splitting its permission would only create roles that can half-configure it.
const (
	PermLoyaltyRead    = "vendor.crm.loyalty.read"
	PermLoyaltyManage  = "vendor.crm.loyalty.manage"
	PermLoyaltyAdjust  = "vendor.crm.loyalty.adjust"
	PermLoyaltyReverse = "vendor.crm.loyalty.reverse"
)

const loyaltyMutationRateMax = 60

// LoyaltyHandler exposes the Section 8D vendor surface. It reuses the accepted
// Section 8B/8C command edge — the same Idempotency-Key store, the same actor
// resolution — rather than introducing a third convention.
type LoyaltyHandler struct {
	*SegmentationHandler
	limiter ActivityRateLimiter
}

// NewLoyaltyHandler wires the Section 8D handler.
func NewLoyaltyHandler(service *Service, store *idempotency.Store, limiters ...ActivityRateLimiter) *LoyaltyHandler {
	var limiter ActivityRateLimiter
	if len(limiters) > 0 {
		limiter = limiters[0]
	}
	return &LoyaltyHandler{SegmentationHandler: NewSegmentationHandler(service, store), limiter: limiter}
}

func (h *LoyaltyHandler) rateLimit(c *gin.Context, actor Actor, bucket string) error {
	if h.limiter == nil {
		return nil
	}
	return h.limiter.Allow(c, bucket, actor.VendorID+":"+actor.StaffID, loyaltyMutationRateMax, activityRateWindow)
}

// scopedProfile resolves the customer through the ordinary Customer 360 read
// path, so tenancy, redaction and branch reach are all decided by the accepted
// Section 8A rules before any loyalty row is touched.
func (h *LoyaltyHandler) scopedProfile(c *gin.Context, permission string) (Actor, Profile, bool) {
	actor, scope, ok := h.authorized(c, permission)
	if !ok {
		return Actor{}, Profile{}, false
	}
	profile, err := h.service.GetProfile(c, actor, scope, strings.TrimSpace(c.Param("profileId")))
	if err != nil {
		httpx.RespondError(c, err)
		return Actor{}, Profile{}, false
	}
	return actor, profile, true
}

// ---------------------------------------------------------------------------
// Programme configuration
// ---------------------------------------------------------------------------

type updateLoyaltyProgramRequest struct {
	Status                    *string `json:"status,omitempty"`
	PointsNameSingular        *string `json:"points_name_singular,omitempty"`
	PointsNamePlural          *string `json:"points_name_plural,omitempty"`
	ExpiryPolicy              *string `json:"expiry_policy,omitempty"`
	ExpiryDays                *int    `json:"expiry_days,omitempty"`
	ClearExpiryDays           bool    `json:"clear_expiry_days,omitempty"`
	MinimumRedemptionPoints   *int64  `json:"minimum_redemption_points,omitempty"`
	RedemptionIncrementPoints *int64  `json:"redemption_increment_points,omitempty"`
	ManualAdjustmentLimit     *int64  `json:"manual_adjustment_limit,omitempty"`
	ExpectedVersion           int64   `json:"expected_version"`
}

// GetLoyaltyProgram returns the programme, or its unwritten defaults at
// version 0 when the business has never configured one.
func (h *LoyaltyHandler) GetLoyaltyProgram(c *gin.Context) {
	actor, _, ok := h.authorized(c, PermLoyaltyRead)
	if !ok {
		return
	}
	program, err := h.service.GetLoyaltyProgram(c, actor)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": LoyaltyProgramViewOf(program)})
}

func (h *LoyaltyHandler) UpdateLoyaltyProgram(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		if _, err := h.service.Authorize(c, actor, PermLoyaltyManage); err != nil {
			return nil, err
		}
		if err := h.rateLimit(c, actor, "crm_loyalty_program"); err != nil {
			return nil, err
		}
		var req updateLoyaltyProgramRequest
		if err := decodeBody(raw, &req); err != nil {
			return nil, err
		}
		program, err := h.service.UpdateLoyaltyProgram(c, actor, UpdateLoyaltyProgramInput{
			Status: req.Status, PointsNameSingular: req.PointsNameSingular, PointsNamePlural: req.PointsNamePlural,
			ExpiryPolicy: req.ExpiryPolicy, ExpiryDays: req.ExpiryDays, ClearExpiryDays: req.ClearExpiryDays,
			MinimumRedemptionPoints: req.MinimumRedemptionPoints, RedemptionIncrementPoints: req.RedemptionIncrementPoints,
			ManualAdjustmentLimit: req.ManualAdjustmentLimit, ExpectedVersion: req.ExpectedVersion,
		})
		if err != nil {
			return nil, err
		}
		return LoyaltyProgramViewOf(program), nil
	})
}

// ---------------------------------------------------------------------------
// Customer loyalty
// ---------------------------------------------------------------------------

// GetCustomerLoyalty returns the balance summary. A customer with no account
// gets NOT_ENROLLED rather than a fabricated zero, and no row is created.
func (h *LoyaltyHandler) GetCustomerLoyalty(c *gin.Context) {
	actor, profile, ok := h.scopedProfile(c, PermLoyaltyRead)
	if !ok {
		return
	}
	summary, err := h.service.LoyaltySummaryFor(c, actor, profile)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": summary})
}

func (h *LoyaltyHandler) ListCustomerLoyaltyEntries(c *gin.Context) {
	actor, profile, ok := h.scopedProfile(c, PermLoyaltyRead)
	if !ok {
		return
	}
	limit, offset, err := parsePage(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	from, err := parseQueryTime(c, "created_from")
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	to, err := parseQueryTime(c, "created_to")
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	entries, more, err := h.service.ListLoyaltyEntries(c, actor, profile, LoyaltyLedgerOptions{
		EntryType: c.Query("entry_type"), CreatedFrom: from, CreatedTo: to, Limit: limit, Offset: offset})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	views, err := h.service.LoyaltyEntryViewsWithReversals(c, actor.VendorID, entries)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"entries": views, "has_more": more, "limit": limit, "offset": offset}})
}

type adjustLoyaltyRequest struct {
	Direction string `json:"direction"`
	Points    int64  `json:"points"`
	Reason    string `json:"reason"`
}

type adjustLoyaltyResponse struct {
	Entry   LoyaltyEntryView `json:"entry"`
	Summary LoyaltySummary   `json:"summary"`
}

// AdjustCustomerLoyalty applies a manual credit or debit.
func (h *LoyaltyHandler) AdjustCustomerLoyalty(c *gin.Context) {
	h.command(c, http.StatusCreated, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermLoyaltyAdjust)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_loyalty_adjust"); err != nil {
			return nil, err
		}
		var req adjustLoyaltyRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		entry, _, err := h.service.AdjustPoints(c, actor, scope, AdjustPointsInput{
			CustomerProfileRef: strings.TrimSpace(c.Param("profileId")), Direction: req.Direction,
			Points: req.Points, Reason: req.Reason, CommandRef: c.GetHeader("Idempotency-Key"),
		})
		if err != nil {
			return nil, err
		}
		profile, err := h.service.GetProfile(c, actor, scope, strings.TrimSpace(c.Param("profileId")))
		if err != nil {
			return nil, err
		}
		summary, err := h.service.LoyaltySummaryFor(c, actor, profile)
		if err != nil {
			return nil, err
		}
		return adjustLoyaltyResponse{Entry: LoyaltyEntryViewOf(entry), Summary: summary}, nil
	})
}

type reverseLoyaltyRequest struct {
	Reason string `json:"reason"`
}

// ReverseLoyaltyEntry posts a compensating entry against a previous one.
func (h *LoyaltyHandler) ReverseLoyaltyEntry(c *gin.Context) {
	h.command(c, http.StatusCreated, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermLoyaltyReverse)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_loyalty_reverse"); err != nil {
			return nil, err
		}
		var req reverseLoyaltyRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		entry, _, err := h.service.ReverseLoyaltyEntry(c, actor, scope, c.Param("entryId"),
			ReverseLoyaltyEntryInput{Reason: req.Reason, CommandRef: c.GetHeader("Idempotency-Key")})
		if err != nil {
			return nil, err
		}
		return LoyaltyEntryViewOf(entry), nil
	})
}

// RegisterLoyaltyRoutes wires Section 8D.
//
// There is deliberately no POST /earn and no POST /redeem. Both engines exist
// as service-internal primitives for a future trusted producer; exposing them
// as vendor-callable endpoints would let any authorised staff member name
// their own points total, which is a different and much larger authority than
// the bounded, reason-carrying manual adjustment below. There are likewise no
// internal HTTP routes: no producer contract exists to receive on yet.
func RegisterLoyaltyRoutes(group *gin.RouterGroup, h *LoyaltyHandler, flags Flags) {
	if !flags.CRMEnabled || !flags.LoyaltyEnabled {
		return
	}
	crm := group.Group("/crm")
	loyalty := crm.Group("/loyalty")
	loyalty.GET("/program", h.GetLoyaltyProgram)
	loyalty.PATCH("/program", h.UpdateLoyaltyProgram)
	loyalty.POST("/entries/:entryId/reverse", h.ReverseLoyaltyEntry)

	crm.GET("/customers/:profileId/loyalty", h.GetCustomerLoyalty)
	crm.GET("/customers/:profileId/loyalty/entries", h.ListCustomerLoyaltyEntries)
	crm.POST("/customers/:profileId/loyalty/adjustments", h.AdjustCustomerLoyalty)
}
