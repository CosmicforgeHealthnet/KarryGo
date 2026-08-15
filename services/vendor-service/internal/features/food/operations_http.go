package food

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

// Section 9B management surface.
//
// Vendor-management only. There is no discovery route here, no unauthenticated
// browse and no internal service contract: Section 9C owns consumption, and
// building any of it now would fix a shape that section has not yet chosen.

// pricingGate returns a closure the service calls only when a request actually
// moves a price, so one endpoint that edits both preparation and price does not
// hand pricing authority to whoever may edit preparation.
func (h *Handler) pricingGate(c *gin.Context, actor Actor, branchID string) func() error {
	return func() error { return h.service.requirePricingAuthority(c, actor, branchID) }
}

func atFrom(c *gin.Context) (time.Time, error) {
	raw := strings.TrimSpace(c.Query("at"))
	if raw == "" {
		return time.Time{}, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "at", Message: "Time must be an RFC3339 timestamp."}})
	}
	return value, nil
}

// ---------------------------------------------------------------------------
// Menu schedules
// ---------------------------------------------------------------------------

type scheduleRequest struct {
	Label              string  `json:"label"`
	DaysOfWeek         []int   `json:"days_of_week"`
	StartLocalTime     string  `json:"start_local_time"`
	EndLocalTime       string  `json:"end_local_time"`
	EffectiveStartDate *string `json:"effective_start_date,omitempty"`
	EffectiveEndDate   *string `json:"effective_end_date,omitempty"`
}

func (h *Handler) CreateSchedule(c *gin.Context) {
	h.command(c, http.StatusCreated, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermOperationsManage)
		if err != nil {
			return nil, err
		}
		var req scheduleRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		schedule, err := h.service.CreateSchedule(c, actor, scope, c.Param("menuId"), CreateScheduleInput{
			Label: req.Label, DaysOfWeek: req.DaysOfWeek,
			StartLocalTime: req.StartLocalTime, EndLocalTime: req.EndLocalTime,
			EffectiveStartDate: req.EffectiveStartDate, EffectiveEndDate: req.EffectiveEndDate})
		if err != nil {
			return nil, err
		}
		return ScheduleViewOf(schedule), nil
	})
}

func (h *Handler) ListSchedules(c *gin.Context) {
	actor, scope, allowed := h.authorized(c, PermCatalogueRead)
	if !allowed {
		return
	}
	schedules, timezone, err := h.service.ListSchedules(c, actor, scope, c.Param("menuId"), includeArchivedFrom(c))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	views := make([]ScheduleView, 0, len(schedules))
	for _, schedule := range schedules {
		views = append(views, ScheduleViewOf(schedule))
	}
	// The timezone travels with the list because a window of 09:00-11:00 is
	// meaningless without knowing whose morning it is.
	ok(c, gin.H{"schedules": views, "timezone": timezone,
		"unrestricted_when_empty": len(views) == 0})
}

func (h *Handler) GetSchedule(c *gin.Context) {
	actor, scope, allowed := h.authorized(c, PermCatalogueRead)
	if !allowed {
		return
	}
	schedule, err := h.service.GetSchedule(c, actor, scope, c.Param("menuId"), c.Param("scheduleId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	ok(c, ScheduleViewOf(schedule))
}

type updateScheduleRequest struct {
	Label              *string `json:"label,omitempty"`
	DaysOfWeek         *[]int  `json:"days_of_week,omitempty"`
	StartLocalTime     *string `json:"start_local_time,omitempty"`
	EndLocalTime       *string `json:"end_local_time,omitempty"`
	EffectiveStartDate *string `json:"effective_start_date,omitempty"`
	EffectiveEndDate   *string `json:"effective_end_date,omitempty"`
	ClearStartDate     bool    `json:"clear_effective_start_date,omitempty"`
	ClearEndDate       bool    `json:"clear_effective_end_date,omitempty"`
	ExpectedVersion    int64   `json:"expected_version"`
}

func (h *Handler) UpdateSchedule(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermOperationsManage)
		if err != nil {
			return nil, err
		}
		var req updateScheduleRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		schedule, err := h.service.UpdateSchedule(c, actor, scope, c.Param("menuId"), c.Param("scheduleId"),
			UpdateScheduleInput{Label: req.Label, DaysOfWeek: req.DaysOfWeek,
				StartLocalTime: req.StartLocalTime, EndLocalTime: req.EndLocalTime,
				EffectiveStartDate: req.EffectiveStartDate, EffectiveEndDate: req.EffectiveEndDate,
				ClearStartDate: req.ClearStartDate, ClearEndDate: req.ClearEndDate,
				ExpectedVersion: req.ExpectedVersion})
		if err != nil {
			return nil, err
		}
		return ScheduleViewOf(schedule), nil
	})
}

func (h *Handler) ArchiveSchedule(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermOperationsManage)
		if err != nil {
			return nil, err
		}
		var req versionedRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		schedule, err := h.service.ArchiveSchedule(c, actor, scope, c.Param("menuId"),
			c.Param("scheduleId"), req.ExpectedVersion)
		if err != nil {
			return nil, err
		}
		return ScheduleViewOf(schedule), nil
	})
}

// ---------------------------------------------------------------------------
// Operational availability
// ---------------------------------------------------------------------------

type setAvailabilityRequest struct {
	Status          string     `json:"status"`
	Reason          string     `json:"reason,omitempty"`
	EffectiveUntil  *time.Time `json:"effective_until,omitempty"`
	ExpectedVersion int64      `json:"expected_version"`
}

func (h *Handler) GetItemAvailability(c *gin.Context) {
	actor, scope, allowed := h.authorized(c, PermCatalogueRead)
	if !allowed {
		return
	}
	availability, err := h.service.GetItemAvailability(c, actor, scope, c.Param("itemId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	ok(c, ItemAvailabilityViewOf(availability, h.service.now()))
}

// SetItemAvailability is the one way to change operational state. There is
// deliberately no separate /sold-out and /restore pair: two routes writing one
// field is two ways for them to disagree.
func (h *Handler) SetItemAvailability(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermOperationsManage)
		if err != nil {
			return nil, err
		}
		var req setAvailabilityRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		availability, err := h.service.SetItemAvailability(c, actor, scope, c.Param("itemId"),
			SetAvailabilityInput{Status: req.Status, Reason: req.Reason,
				EffectiveUntil: req.EffectiveUntil, ExpectedVersion: req.ExpectedVersion})
		if err != nil {
			return nil, err
		}
		return ItemAvailabilityViewOf(availability, h.service.now()), nil
	})
}

// GetEffectiveAvailability runs the authoritative resolver so a client never
// has to reproduce the precedence chain itself.
func (h *Handler) GetEffectiveAvailability(c *gin.Context) {
	actor, scope, allowed := h.authorized(c, PermCatalogueRead)
	if !allowed {
		return
	}
	at, err := atFrom(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	profile, err := h.service.ResolveOperationalProfile(c, actor, scope, c.Param("itemId"), at)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	ok(c, profile)
}

// ---------------------------------------------------------------------------
// Branch item override
// ---------------------------------------------------------------------------

type branchOverrideRequest struct {
	BasePriceOverrideMinor *int64 `json:"base_price_override_minor,omitempty"`
	ClearBasePrice         bool   `json:"clear_base_price_override,omitempty"`
	PrepMinMinutes         *int   `json:"prep_min_minutes,omitempty"`
	PrepMaxMinutes         *int   `json:"prep_max_minutes,omitempty"`
	ClearPrepProfile       bool   `json:"clear_prep_override,omitempty"`
	ExpectedVersion        int64  `json:"expected_version"`
}

func (h *Handler) GetBranchOverride(c *gin.Context) {
	actor, scope, allowed := h.authorized(c, PermCatalogueRead)
	if !allowed {
		return
	}
	override, err := h.service.GetBranchOverride(c, actor, scope, c.Param("itemId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	ok(c, BranchItemOverrideViewOf(override))
}

func (h *Handler) SetBranchOverride(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermOperationsManage)
		if err != nil {
			return nil, err
		}
		var req branchOverrideRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		item, err := h.service.scopedItem(c, actor, scope, c.Param("itemId"))
		if err != nil {
			return nil, err
		}
		override, err := h.service.SetBranchOverride(c, actor, scope, c.Param("itemId"),
			SetBranchOverrideInput{
				BasePriceOverrideMinor: req.BasePriceOverrideMinor, ClearBasePrice: req.ClearBasePrice,
				PrepMinMinutes: req.PrepMinMinutes, PrepMaxMinutes: req.PrepMaxMinutes,
				ClearPrepProfile: req.ClearPrepProfile, ExpectedVersion: req.ExpectedVersion},
			h.pricingGate(c, actor, item.BranchID))
		if err != nil {
			return nil, err
		}
		return BranchItemOverrideViewOf(override), nil
	})
}

func (h *Handler) ClearBranchOverride(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermOperationsManage)
		if err != nil {
			return nil, err
		}
		var req versionedRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		item, err := h.service.scopedItem(c, actor, scope, c.Param("itemId"))
		if err != nil {
			return nil, err
		}
		if err = h.service.ClearBranchOverride(c, actor, scope, c.Param("itemId"), req.ExpectedVersion,
			h.pricingGate(c, actor, item.BranchID)); err != nil {
			return nil, err
		}
		return gin.H{"item_id": item.ID, "cleared": true}, nil
	})
}

// ---------------------------------------------------------------------------
// Pricing
// ---------------------------------------------------------------------------

func (h *Handler) GetPriceHistory(c *gin.Context) {
	actor, scope, allowed := h.authorized(c, PermCatalogueRead)
	if !allowed {
		return
	}
	limit, offset, err := pageFrom(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	history, hasMore, err := h.service.ListItemPriceHistory(c, actor, scope, c.Param("itemId"),
		PriceHistoryOptions{SubjectType: c.Query("subject_type"), Limit: limit, Offset: offset})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	views := make([]PriceVersionView, 0, len(history))
	for _, version := range history {
		views = append(views, PriceVersionViewOf(version))
	}
	if limit <= 0 {
		limit = 50
	}
	ok(c, gin.H{"price_versions": views, "pagination": gin.H{
		"limit": limit, "offset": offset, "has_more": hasMore}})
}

type resolvePriceRequest struct {
	VariantRef   string   `json:"variant_ref,omitempty"`
	ModifierRefs []string `json:"modifier_refs,omitempty"`
}

// ResolvePrice is a management preview of the authoritative resolver. It is not
// checkout, not a quote and not a payment: the client selects references and
// the service supplies every number from the database.
//
// It is a POST because the selection is a structured body, not because it
// changes anything — it writes nothing and takes no idempotency key.
func (h *Handler) ResolvePrice(c *gin.Context) {
	actor, scope, allowed := h.authorized(c, PermCatalogueRead)
	if !allowed {
		return
	}
	raw, err := readLimitedBody(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	var req resolvePriceRequest
	if err = decodeBody(raw, &req); err != nil {
		httpx.RespondError(c, err)
		return
	}
	resolved, err := h.service.ResolvePrice(c, actor, scope, c.Param("itemId"),
		ResolvePriceInput{VariantRef: req.VariantRef, ModifierRefs: req.ModifierRefs})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	ok(c, resolved)
}

// ---------------------------------------------------------------------------
// Preparation, logistics and availability source
// ---------------------------------------------------------------------------

type prepProfileRequest struct {
	PrepMinMinutes  int   `json:"prep_min_minutes"`
	PrepMaxMinutes  int   `json:"prep_max_minutes"`
	ExpectedVersion int64 `json:"expected_version"`
}

func (h *Handler) GetPrepProfile(c *gin.Context) {
	actor, scope, allowed := h.authorized(c, PermCatalogueRead)
	if !allowed {
		return
	}
	profile, found, err := h.service.GetPrepProfile(c, actor, scope, c.Param("itemId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	item, err := h.service.scopedItem(c, actor, scope, c.Param("itemId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	resolved, err := h.service.ResolvePrepProfile(c, h.service.db, item)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	// An unconfigured profile answers "not configured" rather than an invented
	// default range: a fabricated 10-20 minutes would be indistinguishable from
	// one a chef actually chose.
	response := gin.H{"configured": found, "effective": resolved}
	if found {
		response["profile"] = PrepProfileViewOf(profile)
	}
	ok(c, response)
}

func (h *Handler) SetPrepProfile(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermOperationsManage)
		if err != nil {
			return nil, err
		}
		var req prepProfileRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		profile, err := h.service.SetPrepProfile(c, actor, scope, c.Param("itemId"),
			SetPrepProfileInput{PrepMinMinutes: req.PrepMinMinutes, PrepMaxMinutes: req.PrepMaxMinutes,
				ExpectedVersion: req.ExpectedVersion})
		if err != nil {
			return nil, err
		}
		return PrepProfileViewOf(profile), nil
	})
}

func (h *Handler) ClearPrepProfile(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermOperationsManage)
		if err != nil {
			return nil, err
		}
		var req versionedRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		if err = h.service.ClearPrepProfile(c, actor, scope, c.Param("itemId"), req.ExpectedVersion); err != nil {
			return nil, err
		}
		return gin.H{"item_id": strings.TrimSpace(c.Param("itemId")), "cleared": true}, nil
	})
}

type logisticsProfileRequest struct {
	TemperatureClass  string   `json:"temperature_class"`
	PackageSizeClass  string   `json:"package_size_class"`
	SpillRisk         bool     `json:"spill_risk,omitempty"`
	Fragile           bool     `json:"fragile,omitempty"`
	ApproxWeightGrams *int     `json:"approx_weight_grams,omitempty"`
	VehicleHints      []string `json:"vehicle_hints,omitempty"`
	ExpectedVersion   int64    `json:"expected_version"`
}

func (h *Handler) GetLogisticsProfile(c *gin.Context) {
	actor, scope, allowed := h.authorized(c, PermCatalogueRead)
	if !allowed {
		return
	}
	profile, found, err := h.service.GetLogisticsProfile(c, actor, scope, c.Param("itemId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	response := gin.H{"configured": found}
	if found {
		response["profile"] = LogisticsProfileViewOf(profile)
	}
	ok(c, response)
}

func (h *Handler) SetLogisticsProfile(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermOperationsManage)
		if err != nil {
			return nil, err
		}
		var req logisticsProfileRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		profile, err := h.service.SetLogisticsProfile(c, actor, scope, c.Param("itemId"),
			SetLogisticsProfileInput{TemperatureClass: req.TemperatureClass,
				PackageSizeClass: req.PackageSizeClass, SpillRisk: req.SpillRisk, Fragile: req.Fragile,
				ApproxWeightGrams: req.ApproxWeightGrams, VehicleHints: req.VehicleHints,
				ExpectedVersion: req.ExpectedVersion})
		if err != nil {
			return nil, err
		}
		return LogisticsProfileViewOf(profile), nil
	})
}

func (h *Handler) ClearLogisticsProfile(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermOperationsManage)
		if err != nil {
			return nil, err
		}
		var req versionedRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		if err = h.service.ClearLogisticsProfile(c, actor, scope, c.Param("itemId"), req.ExpectedVersion); err != nil {
			return nil, err
		}
		return gin.H{"item_id": strings.TrimSpace(c.Param("itemId")), "cleared": true}, nil
	})
}

type availabilitySourceRequest struct {
	Mode               string  `json:"mode"`
	InventoryRecipeRef *string `json:"inventory_recipe_ref,omitempty"`
	ExpectedVersion    int64   `json:"expected_version"`
}

func (h *Handler) GetAvailabilitySource(c *gin.Context) {
	actor, scope, allowed := h.authorized(c, PermCatalogueRead)
	if !allowed {
		return
	}
	source, err := h.service.GetAvailabilitySource(c, actor, scope, c.Param("itemId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	ok(c, AvailabilitySourceViewOf(source))
}

// SetAvailabilitySource configures the inventory seam. It exposes no stock,
// no quantity and no reservation, because Vendor Service is not the inventory
// authority — and while no such authority exists, an INVENTORY_LINKED item is
// reported unsellable rather than assumed available.
func (h *Handler) SetAvailabilitySource(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermOperationsManage)
		if err != nil {
			return nil, err
		}
		var req availabilitySourceRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		source, err := h.service.SetAvailabilitySource(c, actor, scope, c.Param("itemId"),
			SetAvailabilitySourceInput{Mode: req.Mode, InventoryRecipeRef: req.InventoryRecipeRef,
				ExpectedVersion: req.ExpectedVersion})
		if err != nil {
			return nil, err
		}
		return AvailabilitySourceViewOf(source), nil
	})
}

func (h *Handler) ClearAvailabilitySource(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermOperationsManage)
		if err != nil {
			return nil, err
		}
		var req versionedRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		if err = h.service.ClearAvailabilitySource(c, actor, scope, c.Param("itemId"), req.ExpectedVersion); err != nil {
			return nil, err
		}
		return gin.H{"item_id": strings.TrimSpace(c.Param("itemId")),
			"mode": SourceManual, "cleared": true}, nil
	})
}

// RegisterOperationsRoutes wires the Section 9B management surface.
//
// It shares Section 9A's feature flag rather than introducing a second one:
// schedules, availability and pricing operations are the same product surface
// as the catalogue they operate on, and a separate flag would let a deployment
// serve a catalogue nobody can schedule. Registration depends only on the flag,
// so the route set stays deterministic for the parity guards.
//
// Zero internal routes. Section 9C owns service-facing consumption.
func RegisterOperationsRoutes(group *gin.RouterGroup, handler *Handler, flags Flags) {
	if !flags.CatalogueEnabled {
		return
	}
	food := group.Group("/food")

	menus := food.Group("/menus")
	menus.POST("/:menuId/schedules", handler.CreateSchedule)
	menus.GET("/:menuId/schedules", handler.ListSchedules)
	menus.GET("/:menuId/schedules/:scheduleId", handler.GetSchedule)
	menus.PATCH("/:menuId/schedules/:scheduleId", handler.UpdateSchedule)
	menus.POST("/:menuId/schedules/:scheduleId/archive", handler.ArchiveSchedule)

	items := food.Group("/items")
	items.GET("/:itemId/availability", handler.GetItemAvailability)
	items.PUT("/:itemId/availability", handler.SetItemAvailability)
	items.GET("/:itemId/availability/effective", handler.GetEffectiveAvailability)

	items.GET("/:itemId/availability-source", handler.GetAvailabilitySource)
	items.PUT("/:itemId/availability-source", handler.SetAvailabilitySource)
	items.DELETE("/:itemId/availability-source", handler.ClearAvailabilitySource)

	items.GET("/:itemId/branch-override", handler.GetBranchOverride)
	items.PUT("/:itemId/branch-override", handler.SetBranchOverride)
	items.DELETE("/:itemId/branch-override", handler.ClearBranchOverride)

	items.GET("/:itemId/pricing/history", handler.GetPriceHistory)
	items.POST("/:itemId/pricing/resolve", handler.ResolvePrice)

	items.GET("/:itemId/prep-profile", handler.GetPrepProfile)
	items.PUT("/:itemId/prep-profile", handler.SetPrepProfile)
	items.DELETE("/:itemId/prep-profile", handler.ClearPrepProfile)

	items.GET("/:itemId/logistics-profile", handler.GetLogisticsProfile)
	items.PUT("/:itemId/logistics-profile", handler.SetLogisticsProfile)
	items.DELETE("/:itemId/logistics-profile", handler.ClearLogisticsProfile)
}
