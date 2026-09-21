package goods

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	providerauthusecases "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/commercequote"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/logisticscapability"
	"cosmicforge/logistics/shared/go/serviceauth"
	"cosmicforge/logistics/shared/go/supplypreview"
)

const maxBodyBytes = 64 << 10
const operationsServiceCaller = "operations-service"

type Handler struct {
	service *Service
	// corporateProviderSupported reports whether organization-managed
	// providers may execute Taxi Goods work in this deployment. It is
	// reported truthfully rather than assumed -- see capabilities.go.
	corporateProviderSupported bool
}

func RegisterRoutes(group *gin.RouterGroup, service *Service, verifier *serviceauth.Verifier, providerSigner *sharedauth.TokenSigner, corporateProviderSupported bool) {
	if group == nil || service == nil {
		return
	}
	handler := &Handler{service: service, corporateProviderSupported: corporateProviderSupported}
	if verifier != nil {
		internal := group.Group("/internal/commerce")
		internal.Use(serviceauth.Middleware(verifier), handler.vendorOnlyAndNonce())
		internal.GET("/capabilities", handler.capabilities)

		// The shared cross-authority capability contract Vendor reads to
		// decide whether Taxi Goods may be selected at all (VND-007). It is
		// a different document from the Taxi-shaped one above -- one
		// vocabulary Vendor can compare across three services -- so it gets
		// its own path, behind the identical vendor-only boundary.
		capability := group.Group("")
		capability.Use(serviceauth.Middleware(verifier), handler.vendorOnlyAndNonce())
		logisticscapability.RegisterRoutes(capability, LogisticsCapabilityProvider{
			Enabled:                    true,
			CorporateProviderSupported: corporateProviderSupported,
		})
		internal.POST("/quotes", handler.quote)
		internal.POST("/missions", handler.createMission)
		internal.GET("/missions/operations/:operationRef", handler.readOperation)
		internal.GET("/missions/:missionRef", handler.readMission)
		internal.GET("/missions/:missionRef/handover/:stage", handler.readHandover)
		internal.POST("/missions/:missionRef/cancel", handler.cancelMission)
		internal.POST("/missions/:missionRef/pickup/verify", handler.verifyPickup)
		internal.POST("/missions/:missionRef/receiver", handler.changeReceiver)
		internal.GET("/execution-capability", handler.commerceCapabilities)
		internal.POST("/serviceability", handler.serviceability)

		// The read-only planning surface Vendor's just-in-time orchestration
		// reads before it commits. It creates nothing -- no mission, no
		// booking, no offer, no reservation -- and it names no provider or
		// vehicle, because a preview that returned an identity would be an
		// assignment by another name.
		supplypreview.RegisterRoutes(internal, NewPreviewer(service, true))
		internal.POST("/missions/:missionRef/pickups/delay", handler.pickupDelay)
		internal.POST("/missions/:missionRef/route/replan", handler.replanRoute)

		operations := group.Group("/internal/operations/goods")
		operations.Use(serviceauth.Middleware(verifier), handler.operationsOnlyAndNonce())
		operations.PUT("/providers/:providerId/capability", handler.setProviderCapability)
		operations.PUT("/providers/:providerId/vehicles/:vehicleId/verification", handler.verifyCargoProfile)
		operations.POST("/missions/:missionRef/reassign", handler.reassignProvider)
		operations.POST("/missions/:missionRef/recovery", handler.startRecovery)
		operations.POST("/missions/:missionRef/recovery/replacement", handler.selectReplacement)
		operations.POST("/missions/:missionRef/recovery/transfer", handler.completeRecoveryTransfer)
	}
	if providerSigner != nil {
		provider := group.Group("/provider/goods")
		provider.Use(sharedauth.BearerMiddleware(providerSigner, providerauthusecases.ProviderRole, providerauthusecases.ProviderService))
		provider.GET("/settings", handler.providerSettings)
		provider.PUT("/settings", handler.putProviderSettings)
		provider.GET("/vehicles/:vehicleId/cargo-profile", handler.cargoProfile)
		provider.PUT("/vehicles/:vehicleId/cargo-profile", handler.putCargoProfile)
		provider.GET("/missions", handler.listProviderMissions)
		provider.GET("/missions/:missionRef", handler.getProviderMission)
		provider.POST("/missions/:missionRef/accept", handler.accept)
		provider.POST("/missions/:missionRef/en-route-pickup", handler.enRoutePickup)
		provider.POST("/missions/:missionRef/arrived-pickup", handler.arrivedPickup)
		provider.POST("/missions/:missionRef/pickup/credential", handler.pickupCredential)
		provider.POST("/missions/:missionRef/pickup/resend", handler.resendPickup)
		provider.POST("/missions/:missionRef/in-transit", handler.inTransit)
		provider.POST("/missions/:missionRef/arrived-dropoff", handler.arrivedDropoff)
		provider.POST("/missions/:missionRef/delivery/resend", handler.resendDelivery)
		provider.POST("/missions/:missionRef/delivery/verify", handler.verifyDelivery)
		provider.POST("/missions/:missionRef/evidence", handler.addEvidence)
		provider.POST("/missions/:missionRef/complete", handler.complete)
		provider.POST("/missions/:missionRef/stops/:stopRef/en-route", handler.stopEnRoute)
		provider.POST("/missions/:missionRef/stops/:stopRef/arrived", handler.stopArrived)
		provider.POST("/missions/:missionRef/stops/:stopRef/handover", handler.stopHandover)
	}
}

func (h *Handler) operationsOnlyAndNonce() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString("service_name") != operationsServiceCaller {
			httpx.RespondError(c, apperrors.Forbidden("This caller may not administer Taxi Goods operations.", nil))
			c.Abort()
			return
		}
		h.vendorNonce(c)
	}
}

func (h *Handler) vendorOnlyAndNonce() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, _ := c.Get("service_name")
		caller, _ := raw.(string)
		if caller != commercemission.SignatureCaller {
			httpx.RespondError(c, apperrors.Forbidden("This caller may not use Taxi Commerce.", nil))
			c.Abort()
			return
		}
		h.vendorNonce(c)
	}
}

func (h *Handler) vendorNonce(c *gin.Context) {
	caller := c.GetString("service_name")
	nonce := strings.TrimSpace(c.GetHeader(serviceauth.HeaderNonce))
	if err := h.service.repo.ClaimNonce(c.Request.Context(), caller, nonce, c.Request.URL.Path, time.Now().Add(10*time.Minute)); err != nil {
		httpx.RespondError(c, apperrors.Unauthorized("Service request was already used.", err))
		c.Abort()
		return
	}
	c.Next()
}

func caller(c *gin.Context) string {
	raw, _ := c.Get("service_name")
	value, _ := raw.(string)
	return value
}
func provider(c *gin.Context) (string, bool) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		hError(c, apperrors.Unauthorized("Authentication is required.", nil))
		return "", false
	}
	return claims.Subject, true
}

func bind(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
	decoder := jsonDecoder(c.Request.Body)
	if err := decoder.Decode(target); err != nil {
		hError(c, apperrors.BadRequest("Request body is invalid.", err))
		return false
	}
	return true
}

func (h *Handler) quote(c *gin.Context) {
	var request commercequote.Request
	if !bind(c, &request) {
		return
	}
	quote, err := h.service.Quote(c, caller(c), request)
	if err != nil {
		hError(c, err)
		return
	}
	c.JSON(http.StatusOK, commercequote.Envelope{Success: true, Data: quote})
}
func (h *Handler) createMission(c *gin.Context) {
	var request commercemission.Request
	if !bind(c, &request) {
		return
	}
	mission, err := h.service.CreateMission(c, caller(c), request)
	if err != nil {
		hError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": contractMission(mission)})
}
func (h *Handler) readOperation(c *gin.Context) {
	mission, err := h.service.ReadOperation(c, caller(c), c.Param("operationRef"))
	if err != nil {
		hError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": contractMission(mission)})
}
func (h *Handler) readMission(c *gin.Context) {
	mission, err := h.service.ReadMission(c, caller(c), c.Param("missionRef"))
	if err != nil {
		hError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": contractMission(mission)})
}
func (h *Handler) cancelMission(c *gin.Context) {
	var request commercemission.CancelRequest
	if !bind(c, &request) {
		return
	}
	mission, err := h.service.Cancel(c, caller(c), c.Param("missionRef"), request.ReasonCode, request.ExpectedVersion)
	if err != nil {
		hError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": contractMission(mission)})
}
func (h *Handler) verifyPickup(c *gin.Context) {
	var request VerifyCredential
	if !bind(c, &request) {
		return
	}
	mission, err := h.service.VerifyPickup(c, caller(c), c.Param("missionRef"), request)
	if err != nil {
		hError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": mission})
}

func (h *Handler) changeReceiver(c *gin.Context) {
	var request ReceiverChangeCommand
	if !bind(c, &request) {
		return
	}
	mission, err := h.service.ChangeReceiver(c, caller(c), caller(c), c.Param("missionRef"), request)
	if err != nil {
		hError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": contractMission(mission)})
}

func (h *Handler) reassignProvider(c *gin.Context) {
	var request ReassignProviderCommand
	if !bind(c, &request) {
		return
	}
	mission, err := h.service.ReassignProvider(c, caller(c), c.Param("missionRef"), request)
	if err != nil {
		hError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": contractMission(mission)})
}

func (h *Handler) setProviderCapability(c *gin.Context) {
	var request struct {
		Status          string `json:"status"`
		OperatorID      string `json:"operator_id"`
		ExpectedVersion int64  `json:"expected_version"`
	}
	if !bind(c, &request) {
		return
	}
	operator := strings.TrimSpace(request.OperatorID)
	if operator == "" {
		operator = c.GetString("service_name")
	}
	out, err := h.service.SetProviderCapability(c, c.Param("providerId"), request.Status, operator, request.ExpectedVersion)
	respond(c, out, err)
}

func (h *Handler) verifyCargoProfile(c *gin.Context) {
	var request struct {
		Verified        bool   `json:"verified"`
		OperatorID      string `json:"operator_id"`
		ExpectedVersion int64  `json:"expected_version"`
	}
	if !bind(c, &request) {
		return
	}
	operator := strings.TrimSpace(request.OperatorID)
	if operator == "" {
		operator = c.GetString("service_name")
	}
	out, err := h.service.VerifyCargoProfile(c, c.Param("providerId"), c.Param("vehicleId"), operator, request.Verified, request.ExpectedVersion)
	respond(c, out, err)
}

func contractMission(m Mission) commercemission.Mission {
	status := commercemission.StatusAccepted
	if m.Status == StatusCancelled {
		status = commercemission.StatusCancelled
	}
	return commercemission.Mission{MissionRef: m.MissionRef, OperationRef: m.OperationRef, ExternalRef: m.ID, Status: status, Version: m.Version, AcceptedFeeMinor: m.AgreedFeeMinor, AcceptedAt: m.CreatedAt}
}

func (h *Handler) providerSettings(c *gin.Context) {
	id, ok := provider(c)
	if !ok {
		return
	}
	out, err := h.service.ProviderSettings(c, id)
	respond(c, out, err)
}
func (h *Handler) putProviderSettings(c *gin.Context) {
	id, ok := provider(c)
	if !ok {
		return
	}
	var request struct {
		GoodsAvailable  bool  `json:"goods_available"`
		ExpectedVersion int64 `json:"expected_version"`
	}
	if !bind(c, &request) {
		return
	}
	out, err := h.service.SetProviderAvailability(c, id, request.GoodsAvailable, request.ExpectedVersion)
	respond(c, out, err)
}
func (h *Handler) cargoProfile(c *gin.Context) {
	id, ok := provider(c)
	if !ok {
		return
	}
	out, err := h.service.CargoProfile(c, id, c.Param("vehicleId"))
	respond(c, out, err)
}
func (h *Handler) putCargoProfile(c *gin.Context) {
	id, ok := provider(c)
	if !ok {
		return
	}
	var request struct {
		MaxWeightGrams       int64    `json:"max_weight_grams"`
		MaxVolumeML          int64    `json:"max_volume_ml"`
		MaxPackageCount      int      `json:"max_package_count"`
		LongestDimensionMM   int      `json:"longest_dimension_mm"`
		HandlingCapabilities []string `json:"handling_capabilities"`
		ExpectedVersion      int64    `json:"expected_version"`
	}
	if !bind(c, &request) {
		return
	}
	out, err := h.service.PutCargoProfile(c, id, CargoProfile{VehicleID: c.Param("vehicleId"), MaxWeightGrams: request.MaxWeightGrams, MaxVolumeML: request.MaxVolumeML, MaxPackageCount: request.MaxPackageCount, LongestDimensionMM: request.LongestDimensionMM, HandlingCapabilities: request.HandlingCapabilities}, request.ExpectedVersion)
	respond(c, out, err)
}
func (h *Handler) listProviderMissions(c *gin.Context) {
	id, ok := provider(c)
	if !ok {
		return
	}
	out, err := h.service.ListProviderMissions(c, id)
	if err != nil {
		respond(c, nil, err)
		return
	}
	projection := make([]ProviderMission, 0, len(out))
	for _, mission := range out {
		projection = append(projection, providerMission(mission))
	}
	respond(c, projection, nil)
}
func (h *Handler) getProviderMission(c *gin.Context) {
	id, ok := provider(c)
	if !ok {
		return
	}
	out, err := h.service.GetProviderMission(c, id, c.Param("missionRef"))
	respondProviderMission(c, out, err)
}
func (h *Handler) accept(c *gin.Context)         { h.providerTransition(c, h.service.Accept) }
func (h *Handler) enRoutePickup(c *gin.Context)  { h.providerTransition(c, h.service.EnRoutePickup) }
func (h *Handler) arrivedPickup(c *gin.Context)  { h.providerTransition(c, h.service.ArrivePickup) }
func (h *Handler) inTransit(c *gin.Context)      { h.providerTransition(c, h.service.InTransit) }
func (h *Handler) arrivedDropoff(c *gin.Context) { h.providerTransition(c, h.service.ArriveDropoff) }
func (h *Handler) complete(c *gin.Context)       { h.providerTransition(c, h.service.Complete) }
func (h *Handler) providerTransition(c *gin.Context, fn func(context.Context, string, string) (Mission, error)) {
	id, ok := provider(c)
	if !ok {
		return
	}
	out, err := fn(c, id, c.Param("missionRef"))
	respondProviderMission(c, out, err)
}
func (h *Handler) pickupCredential(c *gin.Context) {
	id, ok := provider(c)
	if !ok {
		return
	}
	out, err := h.service.IssuePickupCredential(c, id, c.Param("missionRef"))
	respond(c, out, err)
}
func (h *Handler) resendPickup(c *gin.Context) { h.resend(c, StagePickup) }
func (h *Handler) resendDelivery(c *gin.Context) {
	id, ok := provider(c)
	if !ok {
		return
	}
	out, err := h.service.ResendDelivery(c, id, c.Param("missionRef"))
	respond(c, out, err)
}
func (h *Handler) resend(c *gin.Context, stage string) {
	id, ok := provider(c)
	if !ok {
		return
	}
	out, err := h.service.Resend(c, id, c.Param("missionRef"), stage)
	respond(c, out, err)
}
func (h *Handler) verifyDelivery(c *gin.Context) {
	id, ok := provider(c)
	if !ok {
		return
	}
	var request VerifyCredential
	if !bind(c, &request) {
		return
	}
	out, err := h.service.VerifyDelivery(c, id, c.Param("missionRef"), request)
	respondProviderMission(c, out, err)
}
func (h *Handler) addEvidence(c *gin.Context) {
	id, ok := provider(c)
	if !ok {
		return
	}
	var request EvidenceCommand
	if !bind(c, &request) {
		return
	}
	out, err := h.service.AddEvidence(c, id, c.Param("missionRef"), request)
	respond(c, gin.H{"evidence_ref": out}, err)
}

func respond(c *gin.Context, data any, err error) {
	if err != nil {
		hError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func respondProviderMission(c *gin.Context, mission Mission, err error) {
	if err != nil {
		respond(c, nil, err)
		return
	}
	respond(c, providerMission(mission), nil)
}
func hError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrDisabled), errors.Is(err, ErrUnavailable):
		httpx.RespondError(c, apperrors.Unavailable("Taxi Goods is unavailable.", err))
	case errors.Is(err, ErrStalePlan):
		httpx.RespondError(c, apperrors.Conflict("The fulfilment plan version is stale.", err))
	case errors.Is(err, ErrInvalid):
		httpx.RespondError(c, apperrors.BadRequest("Taxi Goods request is invalid.", err))
	case errors.Is(err, ErrNotFound):
		httpx.RespondError(c, apperrors.NotFound("Taxi Goods resource was not found.", err))
	case errors.Is(err, ErrConflict), errors.Is(err, ErrCredentialInvalid), errors.Is(err, ErrCredentialExpired), errors.Is(err, ErrCredentialLocked), errors.Is(err, ErrResendUnavailable), errors.Is(err, ErrEvidenceRequired), errors.Is(err, ErrMediaScopeMismatch):
		httpx.RespondError(c, apperrors.Conflict("Taxi Goods request conflicts with authoritative state.", err))
	default:
		httpx.RespondError(c, apperrors.Internal("Taxi Goods request failed.", err))
	}
}

type strictDecoder interface {
	Decode(any) error
	DisallowUnknownFields()
}

func jsonDecoder(reader io.Reader) strictDecoder {
	decoder := newJSONDecoder(reader)
	decoder.DisallowUnknownFields()
	return decoder
}

// commerceCapabilities answers the Taxi Goods execution envelope (contract
// v2) that Vendor reads for multi-pickup planning. It is a different document
// from the cross-authority declaration in capabilities.go, and has its own
// route.
func (h *Handler) commerceCapabilities(c *gin.Context) {
	out, err := h.service.Capability(c)
	respond(c, out, err)
}

// serviceability is the zero-side-effect pre-checkout preview. The strict
// decoder rejects provider_id, vehicle_id and any other identity field.
func (h *Handler) serviceability(c *gin.Context) {
	var request ServiceabilityRequest
	if !bind(c, &request) {
		return
	}
	out, err := h.service.PreviewServiceability(c, request)
	if err == nil {
		h.service.metrics.preview(out.Serviceable)
	}
	respond(c, out, err)
}

func (h *Handler) pickupDelay(c *gin.Context) {
	var request PickupDelayCommand
	if !bind(c, &request) {
		return
	}
	mission, err := h.service.ReportPickupDelay(c, caller(c), c.Param("missionRef"), request.StopRef, request.ReadyAt)
	if err != nil {
		hError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": contractMission(mission)})
}

func (h *Handler) replanRoute(c *gin.Context) {
	var request RouteReplanCommand
	if !bind(c, &request) {
		return
	}
	version, err := h.service.ReplanRoute(c, caller(c), c.Param("missionRef"), request)
	respond(c, gin.H{"route_version": version}, err)
}

func (h *Handler) startRecovery(c *gin.Context) {
	var request StartRecoveryCommand
	if !bind(c, &request) {
		return
	}
	out, err := h.service.StartRecovery(c, caller(c), c.Param("missionRef"), request)
	respond(c, out, err)
}

func (h *Handler) selectReplacement(c *gin.Context) {
	var request struct {
		OperationRef string `json:"operation_ref"`
	}
	if !bind(c, &request) {
		return
	}
	out, err := h.service.SelectReplacement(c, caller(c), c.Param("missionRef"), request.OperationRef)
	respond(c, out, err)
}

func (h *Handler) completeRecoveryTransfer(c *gin.Context) {
	var request CompleteTransferCommand
	if !bind(c, &request) {
		return
	}
	mission, err := h.service.CompleteRecoveryTransfer(c, caller(c), c.Param("missionRef"), request)
	if err != nil {
		hError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": contractMission(mission)})
}

func (h *Handler) stopTransition(c *gin.Context, fn func(context.Context, string, string, string) (Mission, error)) {
	id, ok := provider(c)
	if !ok {
		return
	}
	out, err := fn(c, id, c.Param("missionRef"), c.Param("stopRef"))
	respondProviderMission(c, out, err)
}

func (h *Handler) stopEnRoute(c *gin.Context) { h.stopTransition(c, h.service.StopEnRoute) }
func (h *Handler) stopArrived(c *gin.Context) { h.stopTransition(c, h.service.StopArrived) }
func (h *Handler) stopHandover(c *gin.Context) {
	id, ok := provider(c)
	if !ok {
		return
	}
	out, err := h.service.StopHandover(c, id, c.Param("missionRef"), c.Param("stopRef"))
	respond(c, out, err)
}
