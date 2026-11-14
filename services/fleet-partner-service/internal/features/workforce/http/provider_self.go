package http

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
)

// Provider self-view (spec 46.2 "providers can view their own schedules,
// qualifications, training and explainable performance inputs",
// FLEET-17.15).
//
// Providers never call Fleet directly. Their owning provider service
// (driver-dispatch-delivery-service, driver-taxi-service,
// driver-hauling-service) authenticates the provider session and calls these
// INTERNAL, HMAC-authenticated routes on the provider's behalf. The
// membership is resolved from the verified caller service plus the provider
// id it vouches for -- never from request input naming a membership -- so a
// provider can only ever read their own records.
//
// Every response is an explicit, provider-facing DTO built field by field.
// No fare, customer price, commission, margin, settlement amount or other
// organization financial data is read, and staff identifiers (who verified,
// who assigned) are not returned.

// ProviderSelfUseCases is the use-case surface the provider self-view needs.
// *usecases.Service satisfies it.
type ProviderSelfUseCases interface {
	Now() time.Time
	ProviderOwnSchedule(ctx context.Context, org, membership string, from, to time.Time) (models.OwnSchedule, error)
	ProviderOwnQualifications(ctx context.Context, org, membership string) ([]models.OwnQualification, error)
	ProviderOwnTraining(ctx context.Context, org, membership string) (models.OwnTraining, error)
	ProviderOwnPerformance(ctx context.Context, org, membership string) ([]models.PerformanceSnapshot, error)
}

// ProviderSelfResolver resolves the live membership a provider-service caller
// is speaking for. On failure it writes the error response itself and
// returns ok=false.
type ProviderSelfResolver interface {
	ResolveMembership(c *gin.Context) (organizationID string, membershipID string, ok bool)
}

// InternalRoute is one internal provider self-view operation.
type InternalRoute struct {
	Method, Path, OperationID string
}

// InternalProviderRoutes lists the provider self-view routes, mounted under
// /internal/fleet-partner/v1.
func InternalProviderRoutes() []InternalRoute {
	return []InternalRoute{
		{"GET", "/providers/:providerId/schedule", "getProviderOwnSchedule"},
		{"GET", "/providers/:providerId/qualifications", "getProviderOwnQualifications"},
		{"GET", "/providers/:providerId/training", "getProviderOwnTraining"},
		{"GET", "/providers/:providerId/performance", "getProviderOwnPerformance"},
	}
}

// InternalProviderHandler serves the provider self-view.
type InternalProviderHandler struct {
	s        ProviderSelfUseCases
	resolver ProviderSelfResolver
}

// NewInternalProviderHandler builds the handler.
func NewInternalProviderHandler(s ProviderSelfUseCases, resolver ProviderSelfResolver) *InternalProviderHandler {
	return &InternalProviderHandler{s: s, resolver: resolver}
}

// RegisterInternalProviderRoutes mounts the self-view routes. Without a
// resolver there is no trustworthy way to know whose records are being read,
// so nothing is mounted.
func RegisterInternalProviderRoutes(group *gin.RouterGroup, h *InternalProviderHandler) {
	if h == nil || h.resolver == nil {
		return
	}
	for _, route := range InternalProviderRoutes() {
		route := route
		group.Handle(route.Method, route.Path, func(c *gin.Context) { h.handle(c, route.OperationID) })
	}
}

func (h *InternalProviderHandler) handle(c *gin.Context, operation string) {
	org, membership, ok := h.resolver.ResolveMembership(c)
	if !ok {
		if !c.Writer.Written() {
			respond.Error(c, apperrors.Forbidden("Provider authentication is required.", nil))
		}
		return
	}
	ctx := c.Request.Context()
	var (
		data any
		err  error
	)
	switch operation {
	case "getProviderOwnSchedule":
		var from, to time.Time
		if from, to, err = scheduleWindow(c.Query("from"), c.Query("to"), h.s.Now().UTC()); err == nil {
			data, err = h.s.ProviderOwnSchedule(ctx, org, membership, from, to)
		}
	case "getProviderOwnQualifications":
		data, err = h.s.ProviderOwnQualifications(ctx, org, membership)
	case "getProviderOwnTraining":
		data, err = h.s.ProviderOwnTraining(ctx, org, membership)
	case "getProviderOwnPerformance":
		data, err = h.s.ProviderOwnPerformance(ctx, org, membership)
	default:
		err = apperrors.NotFound("Resource not found.", nil)
	}
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, data)
}

// maxScheduleWindow bounds one schedule read.
const maxScheduleWindow = 92 * 24 * time.Hour

func scheduleWindow(rawFrom, rawTo string, now time.Time) (time.Time, time.Time, error) {
	from, to := now.Add(-7*24*time.Hour), now.Add(30*24*time.Hour)
	invalid := apperrors.Validation("from and to must be RFC 3339 instants, from before to, at most 92 days apart.",
		[]apperrors.FieldViolation{{Field: "from", Message: "Invalid window."}})
	if v := strings.TrimSpace(rawFrom); v != "" {
		parsed, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return from, to, invalid
		}
		from = parsed.UTC()
	}
	if v := strings.TrimSpace(rawTo); v != "" {
		parsed, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return from, to, invalid
		}
		to = parsed.UTC()
	}
	if !to.After(from) || to.Sub(from) > maxScheduleWindow {
		return from, to, invalid
	}
	return from, to, nil
}
