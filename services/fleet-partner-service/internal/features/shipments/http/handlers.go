// Package http implements the Section 15 shipment and cargo item HTTP
// surface.
package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Permissions guarding the Section 15 shipment routes. These are the exact
// codes seeded by migration 000027.
const (
	PermissionShipmentsRead   = "shipments.read"
	PermissionShipmentsManage = "shipments.manage"
)

// Handler serves the Section 15 shipment and cargo item routes.
type Handler struct {
	service     *usecases.Service
	authorizer  *accesscontrol.Authorizer
	idempotency *idempotency.Store
}

// NewHandler builds the handler. idempotencyStore may be nil only in tests
// that never exercise a write route.
func NewHandler(service *usecases.Service, authorizer *accesscontrol.Authorizer,
	idempotencyStore *idempotency.Store) *Handler {
	return &Handler{service: service, authorizer: authorizer, idempotency: idempotencyStore}
}

// RegisterTenantRoutes mounts the corporate shipment and cargo item routes.
func (h *Handler) RegisterTenantRoutes(group *gin.RouterGroup) {
	if h.service == nil {
		return
	}
	mount(group, h.TenantRoutes(), map[string]gin.HandlerFunc{
		"listShipments":  h.list,
		"createShipment": h.create,
		"getShipment":    h.get,
		"updateShipment": h.update,
		"listCargoItems": h.listCargoItems,
		"addCargoItem":   h.addCargoItem,
	})
}

func mount(group *gin.RouterGroup, routes []Route, handlers map[string]gin.HandlerFunc) {
	for _, route := range routes {
		handler, ok := handlers[route.OperationID]
		if !ok || handler == nil {
			panic("shipments: no handler registered for operation " + route.OperationID)
		}
		group.Handle(route.Method, route.Path, handler)
	}
}

// ---------------------------------------------------------------------------
// Request plumbing
// ---------------------------------------------------------------------------

func requireTenant(c *gin.Context) (tenant.Context, bool) {
	tenantCtx, ok := middleware.TenantFromGin(c)
	if !ok {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return tenant.Context{}, false
	}
	return tenantCtx, true
}

func (h *Handler) authorize(c *gin.Context, t tenant.Context, permission string) bool {
	if h.authorizer == nil {
		respond.Error(c, apperrors.Forbidden("You do not have permission to perform this action.", nil))
		return false
	}
	if err := h.authorizer.Require(c.Request.Context(), t.OrganizationID, t.OrganizationUserID,
		roleVersionFrom(c), permission, accesscontrol.Target{BranchID: c.Query("branchId")}); err != nil {
		respond.Error(c, err)
		return false
	}
	return true
}

func roleVersionFrom(c *gin.Context) int64 {
	if value, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		if version, ok := value.(int64); ok {
			return version
		}
	}
	return 1
}

func expectedVersion(c *gin.Context, bodyValue int64) int64 {
	if bodyValue != 0 {
		return bodyValue
	}
	raw := strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`)
	if raw == "" {
		return 0
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func bindJSON(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// Idempotent command execution
// ---------------------------------------------------------------------------
//
// A creation command's whole raw response — success or error — is persisted
// under the caller's Idempotency-Key, so an identical retry replays the
// original byte-for-byte answer rather than risking a second create.

type successEnvelope struct {
	Data any  `json:"data"`
	Meta meta `json:"meta"`
}

type meta struct {
	RequestID string `json:"requestId"`
}

type errorEnvelope struct {
	Error respond.ErrorBody `json:"error"`
}

// runIdempotent executes fn under the request's Idempotency-Key. body is the
// already-bound request struct, used only to compute the fingerprint that
// detects key reuse with a different request.
func (h *Handler) runIdempotent(c *gin.Context, endpoint string, t tenant.Context, body any,
	fn func() (status int, data any, err error)) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		respond.Error(c, apperrors.BadRequest("Idempotency-Key header is required.", nil))
		return
	}
	if h.idempotency == nil {
		respond.Error(c, apperrors.Internal("Idempotent commands are not available in this deployment.", nil))
		return
	}
	requestBytes, _ := json.Marshal(body)
	fingerprint := idempotency.Fingerprint(c.Request.Method, endpoint, requestBytes)

	record, err := h.idempotency.Begin(c.Request.Context(), t.OrganizationID, endpoint, key, fingerprint)
	if err != nil {
		respond.Error(c, idempotency.AsAppError(err))
		return
	}
	if record != nil {
		c.Data(record.ResponseStatus, "application/json; charset=utf-8", record.ResponseBody)
		return
	}

	requestID := httpx.GetRequestID(c)
	status, data, svcErr := fn()
	var payload []byte
	if svcErr != nil {
		appErr := apperrors.From(svcErr)
		details := appErr.Fields
		if details == nil {
			details = []apperrors.FieldViolation{}
		}
		payload, _ = json.Marshal(errorEnvelope{Error: respond.ErrorBody{
			Code: appErr.Code, Message: appErr.Message, Details: details, RequestID: requestID,
		}})
		status = appErr.Status
	} else {
		payload, _ = json.Marshal(successEnvelope{Data: data, Meta: meta{RequestID: requestID}})
	}

	if completeErr := h.idempotency.Complete(c.Request.Context(), t.OrganizationID, endpoint, key, status, payload); completeErr != nil {
		respond.Error(c, apperrors.Internal("Failed to persist the idempotent response.", completeErr))
		return
	}
	c.Data(status, "application/json; charset=utf-8", payload)
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// ShipmentView is the wire representation of a shipment.
type ShipmentView struct {
	ID                        string         `json:"id"`
	OrganizationID            string         `json:"organizationId"`
	BranchID                  string         `json:"branchId,omitempty"`
	OrderID                   string         `json:"orderId"`
	OriginStopID              string         `json:"originStopId"`
	DestinationStopID         string         `json:"destinationStopId"`
	CargoProfile              map[string]any `json:"cargoProfile"`
	PassengerProfile          map[string]any `json:"passengerProfile"`
	RequiredVehicleClass      string         `json:"requiredVehicleClass,omitempty"`
	RequiredServiceCapability string         `json:"requiredServiceCapability,omitempty"`
	TemperatureRequirements   map[string]any `json:"temperatureRequirements"`
	DeclaredValueAmountMinor  int64          `json:"declaredValueAmountMinor"`
	DeclaredValueCurrency     string         `json:"declaredValueCurrency"`
	PackageCount              int            `json:"packageCount"`
	Status                    string         `json:"status"`
	Version                   int64          `json:"version"`
	CreatedAt                 string         `json:"createdAt"`
	UpdatedAt                 string         `json:"updatedAt"`
}

// NewShipmentView projects the domain model to the wire shape.
func NewShipmentView(s models.Shipment) ShipmentView {
	view := ShipmentView{
		ID: s.ID, OrganizationID: s.OrganizationID, BranchID: s.BranchID, OrderID: s.OrderID,
		OriginStopID: s.OriginStopID, DestinationStopID: s.DestinationStopID,
		CargoProfile: map[string]any(s.CargoProfile), PassengerProfile: map[string]any(s.PassengerProfile),
		RequiredVehicleClass: s.RequiredVehicleClass, RequiredServiceCapability: s.RequiredServiceCapability,
		TemperatureRequirements:  map[string]any(s.TemperatureRequirements),
		DeclaredValueAmountMinor: s.DeclaredValueAmountMinor, DeclaredValueCurrency: s.DeclaredValueCurrency,
		PackageCount: s.PackageCount, Status: s.Status, Version: s.Version,
		CreatedAt: s.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
		UpdatedAt: s.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
	}
	if view.CargoProfile == nil {
		view.CargoProfile = map[string]any{}
	}
	if view.PassengerProfile == nil {
		view.PassengerProfile = map[string]any{}
	}
	if view.TemperatureRequirements == nil {
		view.TemperatureRequirements = map[string]any{}
	}
	return view
}

// CargoItemView is the wire representation of a cargo item.
type CargoItemView struct {
	ID                       string  `json:"id"`
	ShipmentID               string  `json:"shipmentId"`
	Description              string  `json:"description"`
	Quantity                 string  `json:"quantity"`
	Unit                     string  `json:"unit"`
	HandlingCategory         string  `json:"handlingCategory"`
	HazardousClassification  string  `json:"hazardousClassification,omitempty"`
	TemperatureMin           *string `json:"temperatureMin,omitempty"`
	TemperatureMax           *string `json:"temperatureMax,omitempty"`
	TemperatureUnit          string  `json:"temperatureUnit,omitempty"`
	DeclaredValueAmountMinor int64   `json:"declaredValueAmountMinor"`
	DeclaredValueCurrency    string  `json:"declaredValueCurrency"`
	RestrictedItemStatus     string  `json:"restrictedItemStatus"`
	Version                  int64   `json:"version"`
}

// NewCargoItemView projects the domain model to the wire shape.
func NewCargoItemView(c models.CargoItem) CargoItemView {
	return CargoItemView{
		ID: c.ID, ShipmentID: c.ShipmentID, Description: c.Description, Quantity: c.Quantity, Unit: c.Unit,
		HandlingCategory: c.HandlingCategory, HazardousClassification: c.HazardousClassification,
		TemperatureMin: c.TemperatureMin, TemperatureMax: c.TemperatureMax, TemperatureUnit: c.TemperatureUnit,
		DeclaredValueAmountMinor: c.DeclaredValueAmountMinor, DeclaredValueCurrency: c.DeclaredValueCurrency,
		RestrictedItemStatus: c.RestrictedItemStatus, Version: c.Version,
	}
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func (h *Handler) list(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionShipmentsRead) {
		return
	}
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			respond.Error(c, apperrors.BadRequest("limit must be an integer.", nil))
			return
		}
		limit = parsed
	}
	shipments, next, err := h.service.List(c.Request.Context(), t, usecases.ListInput{
		Status: c.Query("status"), OrderID: c.Query("orderId"), BranchID: c.Query("branchId"),
		Cursor: c.Query("cursor"), Limit: cursor.ClampLimit(limit),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]ShipmentView, 0, len(shipments))
	for _, shipment := range shipments {
		views = append(views, NewShipmentView(shipment))
	}
	respond.List(c, views, next)
}

func (h *Handler) get(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionShipmentsRead) {
		return
	}
	shipment, err := h.service.Get(c.Request.Context(), t, c.Param("shipmentId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewShipmentView(shipment))
}

type createShipmentRequest struct {
	BranchID                  string         `json:"branchId"`
	OrderID                   string         `json:"orderId"`
	OriginStopID              string         `json:"originStopId"`
	DestinationStopID         string         `json:"destinationStopId"`
	CargoProfile              map[string]any `json:"cargoProfile"`
	PassengerProfile          map[string]any `json:"passengerProfile"`
	RequiredVehicleClass      string         `json:"requiredVehicleClass"`
	RequiredServiceCapability string         `json:"requiredServiceCapability"`
	TemperatureRequirements   map[string]any `json:"temperatureRequirements"`
	DeclaredValueAmountMinor  int64          `json:"declaredValueAmountMinor"`
	DeclaredValueCurrency     string         `json:"declaredValueCurrency"`
}

func (h *Handler) create(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionShipmentsManage) {
		return
	}
	var body createShipmentRequest
	if !bindJSON(c, &body) {
		return
	}
	h.runIdempotent(c, "POST /shipments", t, body, func() (int, any, error) {
		shipment, err := h.service.Create(c.Request.Context(), t, usecases.CreateInput{
			BranchID: body.BranchID, OrderID: body.OrderID, OriginStopID: body.OriginStopID,
			DestinationStopID: body.DestinationStopID, CargoProfile: models.JSONMap(body.CargoProfile),
			PassengerProfile: models.JSONMap(body.PassengerProfile), RequiredVehicleClass: body.RequiredVehicleClass,
			RequiredServiceCapability: body.RequiredServiceCapability,
			TemperatureRequirements:   models.JSONMap(body.TemperatureRequirements),
			DeclaredValueAmountMinor:  body.DeclaredValueAmountMinor, DeclaredValueCurrency: body.DeclaredValueCurrency,
		})
		if err != nil {
			return 0, nil, err
		}
		return http.StatusCreated, NewShipmentView(shipment), nil
	})
}

type updateShipmentRequest struct {
	CargoProfile              map[string]any `json:"cargoProfile"`
	PassengerProfile          map[string]any `json:"passengerProfile"`
	RequiredVehicleClass      string         `json:"requiredVehicleClass"`
	RequiredServiceCapability string         `json:"requiredServiceCapability"`
	TemperatureRequirements   map[string]any `json:"temperatureRequirements"`
	DeclaredValueAmountMinor  int64          `json:"declaredValueAmountMinor"`
	DeclaredValueCurrency     string         `json:"declaredValueCurrency"`
	ExpectedVersion           int64          `json:"expectedVersion"`
}

func (h *Handler) update(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionShipmentsManage) {
		return
	}
	var body updateShipmentRequest
	if !bindJSON(c, &body) {
		return
	}
	shipment, err := h.service.Update(c.Request.Context(), t, c.Param("shipmentId"),
		expectedVersion(c, body.ExpectedVersion), usecases.UpdateInput{
			CargoProfile: models.JSONMap(body.CargoProfile), PassengerProfile: models.JSONMap(body.PassengerProfile),
			RequiredVehicleClass: body.RequiredVehicleClass, RequiredServiceCapability: body.RequiredServiceCapability,
			TemperatureRequirements:  models.JSONMap(body.TemperatureRequirements),
			DeclaredValueAmountMinor: body.DeclaredValueAmountMinor, DeclaredValueCurrency: body.DeclaredValueCurrency,
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewShipmentView(shipment))
}

func (h *Handler) listCargoItems(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionShipmentsRead) {
		return
	}
	items, err := h.service.ListCargoItems(c.Request.Context(), t, c.Param("shipmentId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]CargoItemView, 0, len(items))
	for _, item := range items {
		views = append(views, NewCargoItemView(item))
	}
	respond.OK(c, views)
}

type addCargoItemRequest struct {
	Description              string  `json:"description"`
	Quantity                 string  `json:"quantity"`
	Unit                     string  `json:"unit"`
	HandlingCategory         string  `json:"handlingCategory"`
	HazardousClassification  string  `json:"hazardousClassification"`
	TemperatureMin           *string `json:"temperatureMin"`
	TemperatureMax           *string `json:"temperatureMax"`
	TemperatureUnit          string  `json:"temperatureUnit"`
	DeclaredValueAmountMinor int64   `json:"declaredValueAmountMinor"`
	DeclaredValueCurrency    string  `json:"declaredValueCurrency"`
}

func (h *Handler) addCargoItem(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionShipmentsManage) {
		return
	}
	var body addCargoItemRequest
	if !bindJSON(c, &body) {
		return
	}
	shipmentID := c.Param("shipmentId")
	h.runIdempotent(c, "POST /shipments/:shipmentId/cargo-items", t, body, func() (int, any, error) {
		item, err := h.service.AddCargoItem(c.Request.Context(), t, shipmentID, usecases.AddCargoItemInput{
			Description: body.Description, Quantity: body.Quantity, Unit: body.Unit,
			HandlingCategory: body.HandlingCategory, HazardousClassification: body.HazardousClassification,
			TemperatureMin: body.TemperatureMin, TemperatureMax: body.TemperatureMax,
			TemperatureUnit: body.TemperatureUnit, DeclaredValueAmountMinor: body.DeclaredValueAmountMinor,
			DeclaredValueCurrency: body.DeclaredValueCurrency,
		})
		if err != nil {
			return 0, nil, err
		}
		return http.StatusCreated, NewCargoItemView(item), nil
	})
}
