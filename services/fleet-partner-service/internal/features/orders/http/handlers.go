// Package http implements the Section 15 private order HTTP surface.
package http

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Permissions guarding the Section 15 order routes. These are the exact
// codes seeded by migration 000026.
const (
	PermissionOrdersRead       = "orders.read"
	PermissionOrdersManage     = "orders.manage"
	PermissionOrdersConfirm    = "orders.confirm"
	PermissionOrdersCancel     = "orders.cancel"
	PermissionOrderStopsManage = "orders.stops.manage"
)

// Handler serves the Section 15 private order routes.
type Handler struct {
	service    *usecases.Service
	authorizer *accesscontrol.Authorizer
}

// NewHandler builds the handler.
func NewHandler(service *usecases.Service, authorizer *accesscontrol.Authorizer) *Handler {
	return &Handler{service: service, authorizer: authorizer}
}

// RegisterTenantRoutes mounts the corporate private order routes.
//
// The routes are mounted whenever the service exists. With the enablement
// flag off they answer not-found rather than disappearing from the surface,
// so a disabled capability is observable instead of indistinguishable from a
// deployment that never had it.
func (h *Handler) RegisterTenantRoutes(group *gin.RouterGroup) {
	if h.service == nil {
		return
	}
	mount(group, h.TenantRoutes(), map[string]gin.HandlerFunc{
		"listOrders":              h.list,
		"createOrder":             h.create,
		"getOrder":                h.get,
		"confirmOrder":            h.confirm,
		"cancelOrder":             h.cancel,
		"listOrderHistory":        h.history,
		"listOrderStops":          h.listStops,
		"addOrderStop":            h.addStop,
		"convertQuotationToOrder": h.convertConversionIntent,
	})
}

func mount(group *gin.RouterGroup, routes []Route, handlers map[string]gin.HandlerFunc) {
	for _, route := range routes {
		handler, ok := handlers[route.OperationID]
		if !ok || handler == nil {
			panic("orders: no handler registered for operation " + route.OperationID)
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

const rfc3339Milli = "2006-01-02T15:04:05.999999999Z07:00"

func formatTime(t time.Time) string { return t.UTC().Format(rfc3339Milli) }

func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	formatted := formatTime(*t)
	return &formatted
}

func parseTimeField(raw string) (*time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, apperrors.BadRequest("timestamp fields must be RFC 3339.", nil)
	}
	return &parsed, nil
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// OrderView is the wire representation of a private order.
type OrderView struct {
	ID                     string         `json:"id"`
	OrganizationID         string         `json:"organizationId"`
	BranchID               string         `json:"branchId,omitempty"`
	OrderNumber            string         `json:"orderNumber"`
	CustomerID             string         `json:"customerId"`
	CustomerReference      string         `json:"customerReference,omitempty"`
	Source                 string         `json:"source"`
	SourceReference        string         `json:"sourceReference,omitempty"`
	ServiceCatalogItemID   string         `json:"serviceCatalogItemId"`
	ServiceFields          map[string]any `json:"serviceFields"`
	ContractID             string         `json:"contractId,omitempty"`
	QuotationID            string         `json:"quotationId,omitempty"`
	RequestedScheduleStart *string        `json:"requestedScheduleStart,omitempty"`
	RequestedScheduleEnd   *string        `json:"requestedScheduleEnd,omitempty"`
	Timezone               string         `json:"timezone"`
	Priority               string         `json:"priority"`
	SpecialInstructions    string         `json:"specialInstructions,omitempty"`
	Status                 string         `json:"status"`
	Version                int64          `json:"version"`
	CreatedAt              string         `json:"createdAt"`
	UpdatedAt              string         `json:"updatedAt"`
	ConfirmedAt            *string        `json:"confirmedAt,omitempty"`
	CancelledAt            *string        `json:"cancelledAt,omitempty"`
	CompletedAt            *string        `json:"completedAt,omitempty"`
}

// NewOrderView projects the domain model to the wire shape.
func NewOrderView(o models.Order) OrderView {
	view := OrderView{
		ID: o.ID, OrganizationID: o.OrganizationID, BranchID: o.BranchID, OrderNumber: o.OrderNumber,
		CustomerID: o.CustomerID, CustomerReference: o.CustomerReference, Source: o.Source,
		SourceReference: o.SourceReference, ServiceCatalogItemID: o.ServiceCatalogItemID,
		ServiceFields:          map[string]any(o.ServiceFields),
		ContractID:             o.ContractID,
		QuotationID:            o.QuotationID,
		RequestedScheduleStart: formatTimePtr(o.RequestedScheduleStart),
		RequestedScheduleEnd:   formatTimePtr(o.RequestedScheduleEnd),
		Timezone:               o.Timezone,
		Priority:               o.Priority,
		SpecialInstructions:    o.SpecialInstructions,
		Status:                 o.Status,
		Version:                o.Version,
		CreatedAt:              formatTime(o.CreatedAt),
		UpdatedAt:              formatTime(o.UpdatedAt),
		ConfirmedAt:            formatTimePtr(o.ConfirmedAt),
		CancelledAt:            formatTimePtr(o.CancelledAt),
		CompletedAt:            formatTimePtr(o.CompletedAt),
	}
	if view.ServiceFields == nil {
		view.ServiceFields = map[string]any{}
	}
	return view
}

// OrderStopView is the wire representation of an order stop.
type OrderStopView struct {
	ID                 string   `json:"id"`
	OrderID            string   `json:"orderId"`
	Sequence           int      `json:"sequence"`
	StopType           string   `json:"stopType"`
	CustomerLocationID string   `json:"customerLocationId,omitempty"`
	Latitude           *float64 `json:"latitude,omitempty"`
	Longitude          *float64 `json:"longitude,omitempty"`
	TimeWindowStart    *string  `json:"timeWindowStart,omitempty"`
	TimeWindowEnd      *string  `json:"timeWindowEnd,omitempty"`
	Timezone           string   `json:"timezone"`
	Instructions       string   `json:"instructions,omitempty"`
	Status             string   `json:"status"`
	Version            int64    `json:"version"`
}

// NewOrderStopView projects the domain model to the wire shape.
func NewOrderStopView(stop models.OrderStop) OrderStopView {
	return OrderStopView{
		ID: stop.ID, OrderID: stop.OrderID, Sequence: stop.Sequence, StopType: stop.StopType,
		CustomerLocationID: stop.CustomerLocationID, Latitude: stop.Latitude, Longitude: stop.Longitude,
		TimeWindowStart: formatTimePtr(stop.TimeWindowStart), TimeWindowEnd: formatTimePtr(stop.TimeWindowEnd),
		Timezone: stop.Timezone, Instructions: stop.Instructions, Status: stop.Status, Version: stop.Version,
	}
}

// OrderHistoryView is the wire representation of one immutable status change.
type OrderHistoryView struct {
	PreviousStatus string `json:"previousStatus"`
	NewStatus      string `json:"newStatus"`
	ReasonCode     string `json:"reasonCode,omitempty"`
	ActorType      string `json:"actorType"`
	OccurredAt     string `json:"occurredAt"`
}

// NewOrderHistoryView projects the domain model to the wire shape.
func NewOrderHistoryView(h models.OrderHistory) OrderHistoryView {
	return OrderHistoryView{
		PreviousStatus: h.PreviousStatus, NewStatus: h.NewStatus, ReasonCode: h.ReasonCode,
		ActorType: h.ActorType, OccurredAt: formatTime(h.OccurredAt),
	}
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func (h *Handler) list(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionOrdersRead) {
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
	orders, next, err := h.service.List(c.Request.Context(), t, usecases.ListInput{
		Status: c.Query("status"), CustomerID: c.Query("customerId"), BranchID: c.Query("branchId"),
		Cursor: c.Query("cursor"), Limit: cursor.ClampLimit(limit),
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]OrderView, 0, len(orders))
	for _, order := range orders {
		views = append(views, NewOrderView(order))
	}
	respond.List(c, views, next)
}

func (h *Handler) get(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionOrdersRead) {
		return
	}
	order, err := h.service.Get(c.Request.Context(), t, c.Param("orderId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewOrderView(order))
}

func (h *Handler) history(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionOrdersRead) {
		return
	}
	history, err := h.service.History(c.Request.Context(), t, c.Param("orderId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]OrderHistoryView, 0, len(history))
	for _, entry := range history {
		views = append(views, NewOrderHistoryView(entry))
	}
	respond.OK(c, views)
}

func (h *Handler) listStops(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionOrdersRead) {
		return
	}
	stops, err := h.service.ListStops(c.Request.Context(), t, c.Param("orderId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]OrderStopView, 0, len(stops))
	for _, stop := range stops {
		views = append(views, NewOrderStopView(stop))
	}
	respond.OK(c, views)
}

type createOrderRequest struct {
	BranchID               string         `json:"branchId"`
	CustomerID             string         `json:"customerId"`
	CustomerReference      string         `json:"customerReference"`
	ServiceCatalogItemID   string         `json:"serviceCatalogItemId"`
	ServiceFields          map[string]any `json:"serviceFields"`
	ContractID             string         `json:"contractId"`
	RequestedScheduleStart string         `json:"requestedScheduleStart"`
	RequestedScheduleEnd   string         `json:"requestedScheduleEnd"`
	Timezone               string         `json:"timezone"`
	Priority               string         `json:"priority"`
	SpecialInstructions    string         `json:"specialInstructions"`
}

func (h *Handler) create(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionOrdersManage) {
		return
	}
	var body createOrderRequest
	if !bindJSON(c, &body) {
		return
	}
	start, err := parseTimeField(body.RequestedScheduleStart)
	if err != nil {
		respond.Error(c, err)
		return
	}
	end, err := parseTimeField(body.RequestedScheduleEnd)
	if err != nil {
		respond.Error(c, err)
		return
	}
	order, err := h.service.Create(c.Request.Context(), t, usecases.CreateInput{
		BranchID: body.BranchID, CustomerID: body.CustomerID, CustomerReference: body.CustomerReference,
		ServiceCatalogItemID: body.ServiceCatalogItemID, ServiceFields: models.JSONMap(body.ServiceFields),
		ContractID: body.ContractID, RequestedScheduleStart: start, RequestedScheduleEnd: end,
		Timezone: body.Timezone, Priority: body.Priority, SpecialInstructions: body.SpecialInstructions,
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, NewOrderView(order))
}

type addStopRequest struct {
	StopType           string         `json:"stopType"`
	CustomerLocationID string         `json:"customerLocationId"`
	AddressSnapshot    map[string]any `json:"addressSnapshot"`
	Latitude           *float64       `json:"latitude"`
	Longitude          *float64       `json:"longitude"`
	TimeWindowStart    string         `json:"timeWindowStart"`
	TimeWindowEnd      string         `json:"timeWindowEnd"`
	Timezone           string         `json:"timezone"`
	ContactSnapshot    map[string]any `json:"contactSnapshot"`
	Instructions       string         `json:"instructions"`
}

func (h *Handler) addStop(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionOrderStopsManage) {
		return
	}
	var body addStopRequest
	if !bindJSON(c, &body) {
		return
	}
	start, err := parseTimeField(body.TimeWindowStart)
	if err != nil {
		respond.Error(c, err)
		return
	}
	end, err := parseTimeField(body.TimeWindowEnd)
	if err != nil {
		respond.Error(c, err)
		return
	}
	stop, err := h.service.AddStop(c.Request.Context(), t, c.Param("orderId"), usecases.AddStopInput{
		StopType: body.StopType, CustomerLocationID: body.CustomerLocationID,
		AddressSnapshot: models.JSONMap(body.AddressSnapshot), Latitude: body.Latitude, Longitude: body.Longitude,
		TimeWindowStart: start, TimeWindowEnd: end, Timezone: body.Timezone,
		ContactSnapshot: models.JSONMap(body.ContactSnapshot), Instructions: body.Instructions,
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, NewOrderStopView(stop))
}

type versionOnlyRequest struct {
	ExpectedVersion int64 `json:"expectedVersion"`
}

func (h *Handler) confirm(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionOrdersConfirm) {
		return
	}
	var body versionOnlyRequest
	if !bindJSON(c, &body) {
		return
	}
	order, err := h.service.Confirm(c.Request.Context(), t, c.Param("orderId"), expectedVersion(c, body.ExpectedVersion))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewOrderView(order))
}

type cancelOrderRequest struct {
	ReasonCode      string `json:"reasonCode"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

func (h *Handler) cancel(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionOrdersCancel) {
		return
	}
	var body cancelOrderRequest
	if !bindJSON(c, &body) {
		return
	}
	order, err := h.service.Cancel(c.Request.Context(), t, c.Param("orderId"),
		expectedVersion(c, body.ExpectedVersion), body.ReasonCode)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, NewOrderView(order))
}

type convertConversionIntentRequest struct {
	BranchID               string         `json:"branchId"`
	ServiceCatalogItemID   string         `json:"serviceCatalogItemId"`
	ServiceFields          map[string]any `json:"serviceFields"`
	RequestedScheduleStart string         `json:"requestedScheduleStart"`
	RequestedScheduleEnd   string         `json:"requestedScheduleEnd"`
	Timezone               string         `json:"timezone"`
	Priority               string         `json:"priority"`
	SpecialInstructions    string         `json:"specialInstructions"`
}

// convertConversionIntent consumes a Section 14 accepted-quotation conversion
// intent and produces (or, on retry, returns) the matching private order.
//
// This endpoint needs no transport-level idempotency store: the conversion
// intent itself is the natural idempotency key (a unique index guarantees at
// most one order per intent), so a repeated call safely returns the same
// order rather than creating a second one.
func (h *Handler) convertConversionIntent(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionOrdersManage) {
		return
	}
	var body convertConversionIntentRequest
	if !bindJSON(c, &body) {
		return
	}
	start, err := parseTimeField(body.RequestedScheduleStart)
	if err != nil {
		respond.Error(c, err)
		return
	}
	end, err := parseTimeField(body.RequestedScheduleEnd)
	if err != nil {
		respond.Error(c, err)
		return
	}
	order, err := h.service.ConvertConversionIntent(c.Request.Context(), t, c.Param("acceptedVersionId"),
		usecases.ConvertConversionIntentInput{
			BranchID: body.BranchID, ServiceCatalogItemID: body.ServiceCatalogItemID,
			ServiceFields: models.JSONMap(body.ServiceFields), RequestedScheduleStart: start,
			RequestedScheduleEnd: end, Timezone: body.Timezone, Priority: body.Priority,
			SpecialInstructions: body.SpecialInstructions,
		})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.Created(c, NewOrderView(order))
}
