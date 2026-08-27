package fulfilmentgroup

// The vendor-facing HTTP surface.
//
// What is deliberately NOT here is as important as what is. There is no route
// that creates a platform mission, sets a platform mission's status, chooses a
// canonical mission reference, or deletes a parcel, plan or mission reference.
// Those are all operations that would let a request either impersonate a
// logistics authority or erase operational history, so they have no surface at
// all rather than a surface that answers 403.
//
// Every mutating route takes If-Match. A write without it is refused rather
// than applied blind, which is what makes concurrent packing and concurrent
// mission execution safe from a shared vendor console.

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/features/production"
	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/shared/go/httpx"
)

const maxFulfilmentBody = 64 << 10

// Handler serves the fulfilment API.
type Handler struct {
	service *Service
}

// NewHandler builds the fulfilment handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) actor(c *gin.Context) (production.Actor, bool) {
	staff, ok := authctx.FromContext(c)
	if !ok {
		return production.Actor{}, false
	}
	return production.Actor{
		StaffID:       staff.StaffID,
		VendorID:      staff.VendorID,
		SessionID:     staff.SessionID,
		CorrelationID: httpx.GetRequestID(c),
		CausationID:   c.GetHeader("X-Causation-ID"),
		CommandRef:    c.GetHeader("Idempotency-Key"),
	}, true
}

func strictJSON(c *gin.Context, destination any) error {
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, maxFulfilmentBody+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return ErrInvalid
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ErrInvalid
	}
	return nil
}

// expectedVersion reads If-Match.
//
// Absent is refused, not defaulted. A mutation with no stated expectation is a
// blind write, and on a shared operational console a blind write silently
// discards whatever a colleague just did.
func expectedVersion(c *gin.Context) (int64, error) {
	raw := strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`)
	if raw == "" {
		return 0, ErrVersionConflict
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 1 {
		return 0, ErrVersionConflict
	}
	return value, nil
}

func unauthorized(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, gin.H{
		"success": false,
		"error": gin.H{
			"code":       "UNAUTHORIZED",
			"message":    "Authentication is required.",
			"request_id": httpx.GetRequestID(c),
		},
	})
}

func respondError(c *gin.Context, err error) {
	status, code, message := http.StatusInternalServerError,
		"FULFILMENT_UNAVAILABLE", "Fulfilment is temporarily unavailable."

	switch {
	case errors.Is(err, ErrNotFound):
		status, code, message = http.StatusNotFound,
			"FULFILMENT_NOT_FOUND", "This fulfilment record was not found."

	case errors.Is(err, ErrForbidden):
		status, code, message = http.StatusForbidden,
			"FULFILMENT_FORBIDDEN", "You do not have access to this fulfilment record."

	case errors.Is(err, ErrInvalid):
		status, code, message = http.StatusUnprocessableEntity,
			"FULFILMENT_VALIDATION_FAILED", "Check the submitted fulfilment values."

	case errors.Is(err, ErrVersionConflict):
		status, code, message = http.StatusConflict,
			"FULFILMENT_VERSION_CONFLICT", "This record changed. Reload and try again."

	case errors.Is(err, ErrStalePlan):
		status, code, message = http.StatusConflict,
			"FULFILMENT_PLAN_STALE", "The delivery plan changed. Reload and try again."

	case errors.Is(err, ErrCustodySealed):
		status, code, message = http.StatusConflict,
			"FULFILMENT_PARCEL_SEALED",
			"This package has already been handed over and can no longer be changed."

	case errors.Is(err, ErrConflict):
		status, code, message = http.StatusConflict,
			"FULFILMENT_CONFLICT", "This action is not allowed in the current state."

	case errors.Is(err, ErrDisabled):
		status, code, message = http.StatusServiceUnavailable,
			"COMMERCE_FULFILMENT_DISABLED", "Commerce fulfilment is not enabled."

	case errors.Is(err, ErrDependencyMissing):
		status, code, message = http.StatusServiceUnavailable,
			"FULFILMENT_UNAVAILABLE", "Fulfilment is temporarily unavailable."

	case errors.Is(err, ErrHandoverUnavailable):
		status, code, message = http.StatusServiceUnavailable,
			"VENDOR_HANDOVER_UNAVAILABLE", "Secure Vendor delivery handover is not available for this mission."
	}

	c.JSON(status, gin.H{
		"success": false,
		"error": gin.H{
			"code":       code,
			"message":    message,
			"request_id": httpx.GetRequestID(c),
		},
	})
}

func ok(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

// -----------------------------------------------------------------------------
// Reads
// -----------------------------------------------------------------------------

func (h *Handler) getGroup(c *gin.Context) {
	actor, authed := h.actor(c)
	if !authed {
		unauthorized(c)
		return
	}
	view, err := h.service.ViewGroup(c.Request.Context(), actor, c.Param("groupId"))
	if err != nil {
		respondError(c, err)
		return
	}
	ok(c, view)
}

// getPlanHistory returns every plan version, newest first.
//
// Superseded versions are included deliberately: "the plan changed and here is
// what it was before" is the answer an operator actually needs, and a history
// that only shows the current plan cannot give it.
func (h *Handler) getPlanHistory(c *gin.Context) {
	actor, authed := h.actor(c)
	if !authed {
		unauthorized(c)
		return
	}
	plans, err := h.service.PlanHistory(c.Request.Context(), actor, c.Param("groupId"))
	if err != nil {
		respondError(c, err)
		return
	}
	if plans == nil {
		plans = []Plan{}
	}
	ok(c, gin.H{"plans": plans})
}

func (h *Handler) listUnitParcels(c *gin.Context) {
	actor, authed := h.actor(c)
	if !authed {
		unauthorized(c)
		return
	}
	parcels, err := h.service.ListUnitParcels(c.Request.Context(), actor, c.Param("unitId"))
	if err != nil {
		respondError(c, err)
		return
	}
	if parcels == nil {
		parcels = []Parcel{}
	}
	ok(c, gin.H{"parcels": parcels})
}

func (h *Handler) getParcel(c *gin.Context) {
	actor, authed := h.actor(c)
	if !authed {
		unauthorized(c)
		return
	}
	parcel, err := h.service.GetParcel(c.Request.Context(), actor, c.Param("parcelId"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.Header("ETag", strconv.FormatInt(parcel.Version, 10))
	ok(c, parcel)
}

func (h *Handler) getParcelCustody(c *gin.Context) {
	actor, authed := h.actor(c)
	if !authed {
		unauthorized(c)
		return
	}
	events, err := h.service.ListParcelCustody(c.Request.Context(), actor, c.Param("parcelId"))
	if err != nil {
		respondError(c, err)
		return
	}
	if events == nil {
		events = []CustodyEvent{}
	}
	ok(c, gin.H{"custody_events": events})
}

// -----------------------------------------------------------------------------
// Parcel writes
// -----------------------------------------------------------------------------

type packParcelRequest struct {
	Manifest Manifest `json:"manifest"`

	WeightGrams int64 `json:"weight_grams"`
	LengthMm    int64 `json:"length_mm"`
	WidthMm     int64 `json:"width_mm"`
	HeightMm    int64 `json:"height_mm"`

	Fragile            bool               `json:"fragile"`
	TemperatureControl TemperatureControl `json:"temperature_control"`
}

// packParcel creates one parcel.
//
// The Idempotency-Key header is required rather than optional. Packing is
// exactly the operation a retrying worker performs twice, and without a stable
// key the second attempt produces a second physical box.
func (h *Handler) packParcel(c *gin.Context) {
	actor, authed := h.actor(c)
	if !authed {
		unauthorized(c)
		return
	}

	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		respondError(c, ErrInvalid)
		return
	}

	var request packParcelRequest
	if err := strictJSON(c, &request); err != nil {
		respondError(c, err)
		return
	}

	parcel, err := h.service.PackParcel(c.Request.Context(), actor, c.Param("unitId"),
		ParcelInput{
			IdempotencyKey:     key,
			Manifest:           request.Manifest,
			WeightGrams:        request.WeightGrams,
			LengthMm:           request.LengthMm,
			WidthMm:            request.WidthMm,
			HeightMm:           request.HeightMm,
			Fragile:            request.Fragile,
			TemperatureControl: request.TemperatureControl,
		})
	if err != nil {
		respondError(c, err)
		return
	}

	c.Header("ETag", strconv.FormatInt(parcel.Version, 10))
	ok(c, parcel)
}

func (h *Handler) amendManifest(c *gin.Context) {
	actor, authed := h.actor(c)
	if !authed {
		unauthorized(c)
		return
	}

	version, err := expectedVersion(c)
	if err != nil {
		respondError(c, err)
		return
	}

	var manifest Manifest
	if err = strictJSON(c, &manifest); err != nil {
		respondError(c, err)
		return
	}

	parcel, err := h.service.AmendManifest(
		c.Request.Context(), actor, c.Param("parcelId"), version, manifest)
	if err != nil {
		respondError(c, err)
		return
	}

	c.Header("ETag", strconv.FormatInt(parcel.Version, 10))
	ok(c, parcel)
}

type readinessRequest struct {
	Readiness Readiness `json:"readiness"`
}

func (h *Handler) setUnitReadiness(c *gin.Context) {
	actor, authed := h.actor(c)
	if !authed {
		unauthorized(c)
		return
	}

	version, err := expectedVersion(c)
	if err != nil {
		respondError(c, err)
		return
	}

	var request readinessRequest
	if err = strictJSON(c, &request); err != nil {
		respondError(c, err)
		return
	}

	unit, err := h.service.AdvanceUnitReadiness(
		c.Request.Context(), actor, c.Param("unitId"), version, request.Readiness)
	if err != nil {
		respondError(c, err)
		return
	}

	c.Header("ETag", strconv.FormatInt(unit.Version, 10))
	ok(c, unit)
}

type returnRequest struct {
	ReasonCode string `json:"reason_code"`
}

// initiateReturn opens a return for a parcel.
//
// It creates no new parcel. The response carries the same parcel id the vendor
// has been looking at all along, which is the point of the whole return model.
func (h *Handler) initiateReturn(c *gin.Context) {
	actor, authed := h.actor(c)
	if !authed {
		unauthorized(c)
		return
	}

	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		respondError(c, ErrInvalid)
		return
	}

	var request returnRequest
	if err := strictJSON(c, &request); err != nil {
		respondError(c, err)
		return
	}

	parcelReturn, err := h.service.InitiateReturn(
		c.Request.Context(), actor, c.Param("parcelId"), request.ReasonCode, key)
	if err != nil {
		respondError(c, err)
		return
	}
	ok(c, parcelReturn)
}

// -----------------------------------------------------------------------------
// Vendor missions
// -----------------------------------------------------------------------------

func (h *Handler) listVendorMissions(c *gin.Context) {
	actor, authed := h.actor(c)
	if !authed {
		unauthorized(c)
		return
	}

	limit, _ := strconv.Atoi(c.Query("limit"))
	missions, err := h.service.ListVendorMissions(c.Request.Context(), actor, limit)
	if err != nil {
		respondError(c, err)
		return
	}
	if missions == nil {
		missions = []VendorMission{}
	}
	ok(c, gin.H{"vendor_missions": missions})
}

func (h *Handler) getVendorMission(c *gin.Context) {
	actor, authed := h.actor(c)
	if !authed {
		unauthorized(c)
		return
	}
	mission, err := h.service.GetVendorMission(c.Request.Context(), actor, c.Param("missionId"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.Header("ETag", strconv.FormatInt(mission.Version, 10))
	ok(c, mission)
}

type assignRiderRequest struct {
	StaffID string `json:"staff_id"`
}

func (h *Handler) assignRider(c *gin.Context) {
	actor, authed := h.actor(c)
	if !authed {
		unauthorized(c)
		return
	}

	version, err := expectedVersion(c)
	if err != nil {
		respondError(c, err)
		return
	}

	var request assignRiderRequest
	if err = strictJSON(c, &request); err != nil {
		respondError(c, err)
		return
	}

	mission, err := h.service.AssignRider(
		c.Request.Context(), actor, c.Param("missionId"), request.StaffID, version)
	if err != nil {
		respondError(c, err)
		return
	}

	c.Header("ETag", strconv.FormatInt(mission.Version, 10))
	ok(c, mission)
}

type missionTransitionRequest struct {
	Status VendorMissionStatus `json:"status"`

	// HandoverRef references a credential issued by the authority that owns
	// it. The credential itself never travels through this API.
	HandoverRef   string `json:"handover_ref"`
	ProofRef      string `json:"proof_ref"`
	FailureReason string `json:"failure_reason"`
}

// advanceVendorMission executes one step of a vendor-owned delivery.
//
// Each step is validated against the legal successor set, so this is not a
// status field a caller can set to whatever it likes. A mission cannot jump
// straight to delivered, and one that has never been assigned cannot report a
// delivery at all: there would be no rider to have made it.
func (h *Handler) advanceVendorMission(c *gin.Context) {
	actor, authed := h.actor(c)
	if !authed {
		unauthorized(c)
		return
	}

	version, err := expectedVersion(c)
	if err != nil {
		respondError(c, err)
		return
	}

	var request missionTransitionRequest
	if err = strictJSON(c, &request); err != nil {
		respondError(c, err)
		return
	}

	mission, err := h.service.AdvanceVendorMission(
		c.Request.Context(), actor, c.Param("missionId"), VendorMissionTransition{
			To:              request.Status,
			ExpectedVersion: version,
			HandoverRef:     request.HandoverRef,
			ProofRef:        request.ProofRef,
			FailureReason:   request.FailureReason,
		})
	if err != nil {
		respondError(c, err)
		return
	}

	c.Header("ETag", strconv.FormatInt(mission.Version, 10))
	ok(c, mission)
}

type riderAvailabilityRequest struct {
	BranchID string                 `json:"branch_id"`
	State    RiderAvailabilityState `json:"state"`
}

func availabilityExpectedVersion(c *gin.Context) (int64, error) {
	raw := strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`)
	if raw == "" {
		return 0, ErrVersionConflict
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, ErrVersionConflict
	}
	return value, nil
}

func (h *Handler) getRiderAvailability(c *gin.Context) {
	actor, okActor := h.actor(c)
	if !okActor {
		unauthorized(c)
		return
	}
	value, err := h.service.RiderAvailability(c, actor)
	if err != nil {
		respondError(c, err)
		return
	}
	c.Header("ETag", strconv.FormatInt(value.Version, 10))
	ok(c, value)
}
func (h *Handler) setRiderAvailability(c *gin.Context) {
	actor, okActor := h.actor(c)
	if !okActor {
		unauthorized(c)
		return
	}
	version, err := availabilityExpectedVersion(c)
	if err != nil {
		respondError(c, err)
		return
	}
	var request riderAvailabilityRequest
	if err = strictJSON(c, &request); err != nil {
		respondError(c, err)
		return
	}
	value, err := h.service.SetRiderAvailability(c, actor, request.BranchID, request.State, version)
	if err != nil {
		respondError(c, err)
		return
	}
	c.Header("ETag", strconv.FormatInt(value.Version, 10))
	ok(c, value)
}
func (h *Handler) currentRiderMission(c *gin.Context) {
	actor, okActor := h.actor(c)
	if !okActor {
		unauthorized(c)
		return
	}
	value, err := h.service.CurrentRiderMission(c, actor)
	if err != nil {
		respondError(c, err)
		return
	}
	c.Header("ETag", strconv.FormatInt(value.Mission.Version, 10))
	ok(c, value)
}
func (h *Handler) listRiderMissions(c *gin.Context) {
	actor, okActor := h.actor(c)
	if !okActor {
		unauthorized(c)
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	values, err := h.service.ListRiderMissions(c, actor, limit)
	if err != nil {
		respondError(c, err)
		return
	}
	ok(c, gin.H{"vendor_missions": values})
}
func (h *Handler) getRiderMission(c *gin.Context) {
	actor, okActor := h.actor(c)
	if !okActor {
		unauthorized(c)
		return
	}
	value, err := h.service.RiderMission(c, actor, c.Param("missionId"))
	if err != nil {
		respondError(c, err)
		return
	}
	c.Header("ETag", strconv.FormatInt(value.Mission.Version, 10))
	ok(c, value)
}

type riderActionBody struct {
	EvidenceRef string   `json:"evidence_ref"`
	ReasonCode  string   `json:"reason_code"`
	ParcelRefs  []string `json:"parcel_refs"`
}

func riderActionFromPath(value string) (RiderAction, bool) {
	actions := map[string]RiderAction{"acknowledge": ActionAcknowledge, "start-pickup": ActionStartPickup, "arrive-pickup": ActionArrivePickup, "complete-pickup": ActionCompletePickup, "start-receiver": ActionStartReceiver, "arrive-receiver": ActionArriveReceiver, "complete-delivery": ActionCompleteDelivery, "receiver-unavailable": ActionReceiverUnavailable, "return": ActionReturn}
	action, found := actions[value]
	return action, found
}
func (h *Handler) executeRiderAction(c *gin.Context) {
	actor, okActor := h.actor(c)
	if !okActor {
		unauthorized(c)
		return
	}
	action, found := riderActionFromPath(c.Param("action"))
	if !found {
		respondError(c, ErrInvalid)
		return
	}
	version, err := expectedVersion(c)
	if err != nil {
		respondError(c, err)
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		respondError(c, ErrInvalid)
		return
	}
	var body riderActionBody
	if err = strictJSON(c, &body); err != nil {
		respondError(c, err)
		return
	}
	value, err := h.service.ExecuteRiderAction(c, actor, c.Param("missionId"), RiderActionRequest{Action: action, OperationRef: key, ExpectedVersion: version, EvidenceRef: body.EvidenceRef, ReasonCode: body.ReasonCode, ParcelRefs: body.ParcelRefs})
	if err != nil {
		respondError(c, err)
		return
	}
	c.Header("ETag", strconv.FormatInt(value.Mission.Version, 10))
	ok(c, value)
}

type hybridFallbackBody struct {
	ReasonCode PlanReason `json:"reason_code"`
}

func (h *Handler) triggerHybridFallback(c *gin.Context) {
	actor, okActor := h.actor(c)
	if !okActor {
		unauthorized(c)
		return
	}
	version, err := expectedVersion(c)
	if err != nil {
		respondError(c, err)
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		respondError(c, ErrInvalid)
		return
	}
	var body hybridFallbackBody
	if err = strictJSON(c, &body); err != nil {
		respondError(c, err)
		return
	}
	value, err := h.service.TriggerHybridFallback(c, actor, c.Param("missionId"), HybridFallbackRequest{OperationRef: key, ExpectedVersion: version, ReasonCode: body.ReasonCode})
	if err != nil {
		respondError(c, err)
		return
	}
	ok(c, value)
}
func (h *Handler) getHybridFallback(c *gin.Context) {
	actor, okActor := h.actor(c)
	if !okActor {
		unauthorized(c)
		return
	}
	value, err := h.service.GetHybridFallback(c, actor, c.Param("operationRef"))
	if err != nil {
		respondError(c, err)
		return
	}
	ok(c, value)
}

// RegisterRoutes mounts the fulfilment API.
//
// With the capability disabled no route is registered at all, so a deployment
// without commerce fulfilment has no surface rather than routes that answer 503
// and confirm the feature exists.
func RegisterRoutes(root *gin.RouterGroup, h *Handler, enabled bool) {
	if root == nil || h == nil || h.service == nil || !enabled {
		return
	}

	groups := root.Group("/fulfilment/groups/:groupId")
	groups.GET("", h.getGroup)
	groups.GET("/plans", h.getPlanHistory)

	units := root.Group("/fulfilment/units/:unitId")
	units.GET("/parcels", h.listUnitParcels)
	units.POST("/parcels", h.packParcel)
	units.PUT("/readiness", h.setUnitReadiness)

	parcels := root.Group("/fulfilment/parcels/:parcelId")
	parcels.GET("", h.getParcel)
	parcels.GET("/custody", h.getParcelCustody)
	parcels.PUT("/manifest", h.amendManifest)
	parcels.POST("/returns", h.initiateReturn)

	missions := root.Group("/fulfilment/vendor-missions")
	missions.GET("", h.listVendorMissions)
	missions.GET("/:missionId", h.getVendorMission)
	missions.PUT("/:missionId/rider", h.assignRider)
	missions.PUT("/:missionId/status", h.advanceVendorMission)
	missions.POST("/:missionId/fallbacks", h.triggerHybridFallback)
	missions.GET("/fallbacks/:operationRef", h.getHybridFallback)

	rider := root.Group("/delivery/rider")
	rider.GET("/availability", h.getRiderAvailability)
	rider.PUT("/availability", h.setRiderAvailability)
	rider.GET("/missions", h.listRiderMissions)
	rider.GET("/missions/current", h.currentRiderMission)
	rider.GET("/missions/:missionId", h.getRiderMission)
	rider.POST("/missions/:missionId/actions/:action", h.executeRiderAction)
}
