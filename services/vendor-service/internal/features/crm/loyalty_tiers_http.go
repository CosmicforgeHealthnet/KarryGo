package crm

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/httpx"
)

// Section 8 completion permission codes.
const (
	PermLoyaltyTiersManage   = "vendor.crm.loyalty.tiers.manage"
	PermCustomerValueRead    = "vendor.crm.customer_value.read"
	PermFollowUpTriggersRead = "vendor.crm.followup_triggers.read"
)

// The tier surface is exactly two routes: read the ladder, replace the ladder.
//
// There is no create, no per-rung update and no delete, because a rung is not
// independently meaningful — thresholds must stay distinct and ordered, so an
// API that moved one rung would have to either reject most edits or accept a
// transiently invalid ladder. Replacing the set makes every write a transition
// between two valid states, which is also why the write is idempotent and
// version-checked rather than merge-based.
//
// There is deliberately no route to set a CUSTOMER's tier. Standing is derived
// from the ledger, and a grant that let a staff member type one in would make
// every other customer's tier unfalsifiable.

type replaceLoyaltyTiersRequest struct {
	Tiers []struct {
		TierKey               string `json:"tier_key"`
		DisplayName           string `json:"display_name"`
		MinimumLifetimePoints int64  `json:"minimum_lifetime_points"`
	} `json:"tiers"`
	// Absent on a first write, when there is no ladder to have changed under
	// the caller. Present thereafter, so two managers editing the ladder
	// concurrently cannot silently overwrite one another.
	ExpectedVersion *int64 `json:"expected_version,omitempty"`
}

// GetLoyaltyTiers returns the active ladder. An unconfigured business gets an
// empty set at version 0 rather than a 404: "this vendor has no tiers" is a
// perfectly good answer to "what are this vendor's tiers".
func (h *LoyaltyHandler) GetLoyaltyTiers(c *gin.Context) {
	actor, _, ok := h.authorized(c, PermLoyaltyRead)
	if !ok {
		return
	}
	set, err := h.service.GetLoyaltyTiers(c, actor.VendorID)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": set})
}

// ReplaceLoyaltyTiers atomically replaces the ladder.
func (h *LoyaltyHandler) ReplaceLoyaltyTiers(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		if _, err := h.service.Authorize(c, actor, PermLoyaltyTiersManage); err != nil {
			return nil, err
		}
		if err := h.rateLimit(c, actor, "crm_loyalty_tiers"); err != nil {
			return nil, err
		}
		var req replaceLoyaltyTiersRequest
		if err := decodeBody(raw, &req); err != nil {
			return nil, err
		}
		inputs := make([]LoyaltyTierInput, 0, len(req.Tiers))
		for _, t := range req.Tiers {
			inputs = append(inputs, LoyaltyTierInput{
				TierKey:               t.TierKey,
				DisplayName:           t.DisplayName,
				MinimumLifetimePoints: t.MinimumLifetimePoints,
			})
		}
		return h.service.ReplaceLoyaltyTiers(c, actor, inputs, req.ExpectedVersion)
	})
}

// RegisterLoyaltyTierRoutes mounts the completion's only new mutation surface.
// It shares the Section 8D feature flag: tiers are a view of the loyalty
// ledger, so a business with loyalty disabled has nothing to rank.
func RegisterLoyaltyTierRoutes(group *gin.RouterGroup, h *LoyaltyHandler, flags Flags) {
	if !flags.LoyaltyEnabled {
		return
	}
	loyalty := group.Group("/crm/loyalty")
	loyalty.GET("/tiers", h.GetLoyaltyTiers)
	loyalty.PUT("/tiers", h.ReplaceLoyaltyTiers)
}
