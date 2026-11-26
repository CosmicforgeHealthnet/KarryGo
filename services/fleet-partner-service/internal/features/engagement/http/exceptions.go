package http

import (
	"context"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenantcmd"
	"cosmicforge/logistics/shared/go/apperrors"
)

// ExceptionService is the Section 53 operational-exception slice of the
// engagement use cases.
type ExceptionService interface {
	RaiseException(ctx context.Context, t tenant.Context, in models.ExceptionInput) (map[string]any, error)
	GetException(ctx context.Context, t tenant.Context, id string) (map[string]any, error)
	ListExceptions(ctx context.Context, t tenant.Context) ([]map[string]any, error)
	ExceptionCommand(ctx context.Context, t tenant.Context, id, command string, in models.ExceptionInput) (map[string]any, error)
	ListSLABreaches(ctx context.Context, t tenant.Context) ([]map[string]any, error)
}

func exceptionRoutes() []Route {
	const f = "OPERATIONAL_EXCEPTIONS_ENABLED"
	return []Route{
		{"GET", "/exceptions", "listExceptions", "exceptions.read", f, "corporate", false, false},
		{"POST", "/exceptions", "createException", "exceptions.manage", f, "corporate", true, false},
		{"GET", "/exceptions/:exceptionId", "getException", "exceptions.read", f, "corporate", false, false},
		{"PATCH", "/exceptions/:exceptionId", "updateException", "exceptions.manage", f, "corporate", true, true},
		{"POST", "/exceptions/:exceptionId/acknowledge", "acknowledgeException", "exceptions.manage", f, "corporate", true, true},
		{"POST", "/exceptions/:exceptionId/escalate", "escalateException", "exceptions.manage", f, "corporate", true, true},
		{"POST", "/exceptions/:exceptionId/resolve", "resolveException", "exceptions.resolve", f, "corporate", true, true},
		{"GET", "/sla-breaches", "listSLABreaches", "sla.read", "SLA_MEASUREMENTS_ENABLED", "corporate", false, false},
	}
}

var exceptionCommands = map[string]string{"updateException": "UPDATE", "acknowledgeException": "ACKNOWLEDGE", "escalateException": "ESCALATE", "resolveException": "RESOLVE"}

func (h *Handler) exceptionOps(c *gin.Context, t tenant.Context, r Route) (any, error, bool) {
	s, ok := h.s.(ExceptionService)
	if !ok {
		for _, x := range exceptionRoutes() {
			if x.OperationID == r.OperationID {
				return nil, apperrors.Unavailable("Operational exceptions are unavailable.", nil), true
			}
		}
		return nil, nil, false
	}
	switch r.OperationID {
	case "listExceptions":
		v, err := s.ListExceptions(c, t)
		return v, err, true
	case "createException":
		var in models.ExceptionInput
		if !tenantcmd.Bind(c, &in) {
			return nil, tenantcmd.ErrBound, true
		}
		v, err := s.RaiseException(c, t, in)
		return v, err, true
	case "getException":
		v, err := s.GetException(c, t, c.Param("exceptionId"))
		return v, err, true
	case "updateException", "acknowledgeException", "escalateException", "resolveException":
		var in models.ExceptionInput
		if !tenantcmd.Bind(c, &in) {
			return nil, tenantcmd.ErrBound, true
		}
		in.ExpectedVersion = tenantcmd.ExpectedVersion(c, in.ExpectedVersion)
		v, err := s.ExceptionCommand(c, t, c.Param("exceptionId"), exceptionCommands[r.OperationID], in)
		return v, err, true
	case "listSLABreaches":
		v, err := s.ListSLABreaches(c, t)
		return v, err, true
	}
	return nil, nil, false
}
