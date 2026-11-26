package http

import (
	"context"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenantcmd"
	"cosmicforge/logistics/shared/go/apperrors"
)

// AnalysisService is the Section 22/52 analytical-finance slice of the
// finance use cases.
type AnalysisService interface {
	SubmitInvoice(ctx context.Context, t tenant.Context, id string, v int64) (map[string]any, error)
	ReceivablesAgeing(ctx context.Context, t tenant.Context) (models.ReceivablesAgeing, error)
	Profitability(ctx context.Context, t tenant.Context, in models.ProfitabilityInput) (models.ProfitabilityReport, error)
	SnapshotProfitability(ctx context.Context, t tenant.Context, in models.ProfitabilityInput) (models.ProfitabilityReport, error)
	ListProfitabilitySnapshots(ctx context.Context, t tenant.Context) ([]map[string]any, error)
	AllocateCost(ctx context.Context, t tenant.Context, in models.CostAllocationInput) (map[string]any, error)
	ListCostAllocations(ctx context.Context, t tenant.Context) ([]map[string]any, error)
}

// analysisRoutes are invoice submission, receivables ageing, profitability
// and cost allocation.
func analysisRoutes() []Route {
	return []Route{
		{"POST", "/invoices/:invoiceId/submit", "submitInvoice", "invoices.manage", "INVOICES_ENABLED", "corporate", true, true},
		{"GET", "/receivables/ageing", "getReceivablesAgeing", "receivables.read", "RECEIVABLES_ENABLED", "corporate", false, false},
		{"GET", "/profitability", "getProfitability", "profitability.read", "EXPENSES_ENABLED", "corporate", false, false},
		{"GET", "/profitability/snapshots", "listProfitabilitySnapshots", "profitability.read", "EXPENSES_ENABLED", "corporate", false, false},
		{"POST", "/profitability/snapshots", "createProfitabilitySnapshot", "profitability.snapshot", "EXPENSES_ENABLED", "corporate", true, false},
		{"GET", "/cost-allocations", "listCostAllocations", "cost_allocations.read", "EXPENSES_ENABLED", "corporate", false, false},
		{"POST", "/cost-allocations", "createCostAllocation", "cost_allocations.manage", "EXPENSES_ENABLED", "corporate", true, false},
	}
}

func (h *Handler) analysis(c *gin.Context, t tenant.Context, r Route) (any, error, bool) {
	s, ok := h.s.(AnalysisService)
	if !ok {
		for _, x := range analysisRoutes() {
			if x.OperationID == r.OperationID {
				return nil, apperrors.Unavailable("Finance analysis is unavailable.", nil), true
			}
		}
		return nil, nil, false
	}
	switch r.OperationID {
	case "submitInvoice":
		var b commandBody
		if !tenantcmd.Bind(c, &b) {
			return nil, tenantcmd.ErrBound, true
		}
		v, err := s.SubmitInvoice(c, t, c.Param("invoiceId"), tenantcmd.ExpectedVersion(c, b.ExpectedVersion))
		return v, err, true
	case "getReceivablesAgeing":
		v, err := s.ReceivablesAgeing(c, t)
		return v, err, true
	case "getProfitability":
		v, err := s.Profitability(c, t, models.ProfitabilityInput{Dimension: c.Query("dimension"), From: c.Query("from"), To: c.Query("to")})
		return v, err, true
	case "listProfitabilitySnapshots":
		v, err := s.ListProfitabilitySnapshots(c, t)
		return v, err, true
	case "createProfitabilitySnapshot":
		var in models.ProfitabilityInput
		if !tenantcmd.Bind(c, &in) {
			return nil, tenantcmd.ErrBound, true
		}
		v, err := s.SnapshotProfitability(c, t, in)
		return v, err, true
	case "listCostAllocations":
		v, err := s.ListCostAllocations(c, t)
		return v, err, true
	case "createCostAllocation":
		var in models.CostAllocationInput
		if !tenantcmd.Bind(c, &in) {
			return nil, tenantcmd.ErrBound, true
		}
		v, err := s.AllocateCost(c, t, in)
		return v, err, true
	}
	return nil, nil, false
}
