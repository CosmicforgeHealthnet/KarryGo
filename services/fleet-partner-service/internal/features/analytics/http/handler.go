// Package http serves the Section 24/54 analytics routes: dashboards,
// report definitions and runs, explainable recommendations and automation
// rules. Every route is tenant scoped and deny by default.
package http

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/analytics/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenantcmd"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Route describes one tenant route.
type Route struct {
	Method, Path, OperationID, Permission, FeatureFlag, AuthClass string
	IdempotencyRequired, ExpectedVersionRequired                  bool
}

// FullPath is the mounted path.
func (r Route) FullPath() string { return "/fleet-partner/v1" + r.Path }

// TenantRoutes lists the analytics tenant routes.
func TenantRoutes() []Route {
	const rep, rec, aut = "ANALYTICS_REPORTS_ENABLED", "RECOMMENDATIONS_ENABLED", "AUTOMATION_ENABLED"
	return []Route{
		{"GET", "/dashboards/:dashboard", "getDashboard", "analytics.read", rep, "corporate", false, false},
		{"GET", "/reports", "listReports", "reports.read", rep, "corporate", false, false},
		{"POST", "/reports", "createReport", "reports.create", rep, "corporate", true, false},
		{"GET", "/reports/:reportId", "getReport", "reports.read", rep, "corporate", false, false},
		{"POST", "/reports/:reportId/run", "runReport", "reports.create", rep, "corporate", true, false},
		{"GET", "/report-runs", "listReportRuns", "reports.read", rep, "corporate", false, false},
		{"GET", "/report-runs/:runId", "getReportRun", "reports.read", rep, "corporate", false, false},
		{"GET", "/recommendations", "listRecommendations", "recommendations.read", rec, "corporate", false, false},
		{"POST", "/recommendations/generate", "generateRecommendations", "recommendations.generate", rec, "corporate", true, false},
		{"GET", "/recommendations/:recommendationId", "getRecommendation", "recommendations.read", rec, "corporate", false, false},
		{"POST", "/recommendations/:recommendationId/decisions", "decideRecommendation", "recommendations.decide", rec, "corporate", true, true},
		{"GET", "/model-versions", "listModelVersions", "recommendations.read", rec, "corporate", false, false},
		{"GET", "/automation-rules", "listAutomationRules", "automation.read", aut, "corporate", false, false},
		{"POST", "/automation-rules", "createAutomationRule", "automation.manage", aut, "corporate", true, false},
		{"GET", "/automation-rules/:ruleId", "getAutomationRule", "automation.read", aut, "corporate", false, false},
		{"PATCH", "/automation-rules/:ruleId", "updateAutomationRule", "automation.manage", aut, "corporate", true, true},
		{"POST", "/automation-rules/:ruleId/submit", "submitAutomationRule", "automation.manage", aut, "corporate", true, true},
		{"POST", "/automation-rules/:ruleId/approve", "approveAutomationRule", "automation.approve", aut, "corporate", true, true},
		{"POST", "/automation-rules/:ruleId/disable", "disableAutomationRule", "automation.approve", aut, "corporate", true, true},
		{"GET", "/automation-executions", "listAutomationExecutions", "automation.read", aut, "corporate", false, false},
	}
}

// Service is the slice of the analytics use cases the handler calls.
type Service interface {
	Dashboard(ctx context.Context, t tenant.Context, name string, f models.ReportFilters) (models.ReportResult, error)
	CreateReportDefinition(ctx context.Context, t tenant.Context, in models.ReportDefinitionInput) (map[string]any, error)
	GetReportDefinition(ctx context.Context, t tenant.Context, id string) (map[string]any, error)
	ListReportDefinitions(ctx context.Context, t tenant.Context) ([]map[string]any, error)
	RunReport(ctx context.Context, t tenant.Context, definitionID, key string) (map[string]any, error)
	GetReportRun(ctx context.Context, t tenant.Context, id string) (map[string]any, error)
	ListReportRuns(ctx context.Context, t tenant.Context) ([]map[string]any, error)
	GenerateRecommendations(ctx context.Context, t tenant.Context, in models.GenerateInput) (map[string]any, error)
	ListRecommendations(ctx context.Context, t tenant.Context) ([]map[string]any, error)
	GetRecommendation(ctx context.Context, t tenant.Context, id string) (map[string]any, error)
	DecideRecommendation(ctx context.Context, t tenant.Context, id string, in models.RecommendationDecision) (map[string]any, error)
	ListModelVersions(ctx context.Context, t tenant.Context) ([]map[string]any, error)
	CreateAutomationRule(ctx context.Context, t tenant.Context, in models.AutomationRuleInput) (map[string]any, error)
	GetAutomationRule(ctx context.Context, t tenant.Context, id string) (map[string]any, error)
	ListAutomationRules(ctx context.Context, t tenant.Context) ([]map[string]any, error)
	AutomationRuleCommand(ctx context.Context, t tenant.Context, id, command string, in models.AutomationRuleInput) (map[string]any, error)
	ListAutomationExecutions(ctx context.Context, t tenant.Context) ([]map[string]any, error)
}

// Handler serves the analytics tenant routes.
type Handler struct {
	s    Service
	a    tenantcmd.ScopedAuthorizer
	idem *idempotency.Store
}

// NewHandler builds the handler.
func NewHandler(s Service, a tenantcmd.ScopedAuthorizer, i *idempotency.Store) *Handler {
	return &Handler{s: s, a: a, idem: i}
}

// TenantRoutes lists the routes this handler serves.
func (h *Handler) TenantRoutes() []Route { return TenantRoutes() }

// RegisterTenantRoutes mounts the routes.
func (h *Handler) RegisterTenantRoutes(g *gin.RouterGroup) {
	for _, r := range TenantRoutes() {
		r := r
		g.Handle(r.Method, r.Path, func(c *gin.Context) { h.handle(c, r) })
	}
}

var ruleCommands = map[string]string{"updateAutomationRule": "UPDATE", "submitAutomationRule": "SUBMIT", "approveAutomationRule": "APPROVE", "disableAutomationRule": "DISABLE"}

// dashboardFilters reads the allowlisted dashboard query filters and refuses
// any other parameter (spec 6).
func dashboardFilters(c *gin.Context) (models.ReportFilters, error) {
	for k := range c.Request.URL.Query() {
		if k != "branchId" && k != "from" && k != "to" {
			return models.ReportFilters{}, apperrors.Validation("Unknown dashboard filter: "+k, []apperrors.FieldViolation{{Field: k, Message: "unknown filter"}})
		}
	}
	f := models.ReportFilters{BranchID: c.Query("branchId"), From: c.Query("from"), To: c.Query("to")}
	for field, d := range map[string]string{"from": f.From, "to": f.To} {
		if d == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", d); err != nil {
			return f, apperrors.Validation("Dashboard dates must be YYYY-MM-DD.", []apperrors.FieldViolation{{Field: field, Message: "invalid date"}})
		}
	}
	return f, nil
}

func (h *Handler) handle(c *gin.Context, r Route) {
	t, ok := tenantcmd.Gate(c, h.a, r.Permission)
	if !ok {
		return
	}
	run := func(key string) (any, error) {
		switch r.OperationID {
		case "getDashboard":
			f, err := dashboardFilters(c)
			if err != nil {
				return nil, err
			}
			return h.s.Dashboard(c, t, c.Param("dashboard"), f)
		case "listReports":
			return h.s.ListReportDefinitions(c, t)
		case "createReport":
			var in models.ReportDefinitionInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			return h.s.CreateReportDefinition(c, t, in)
		case "getReport":
			return h.s.GetReportDefinition(c, t, c.Param("reportId"))
		case "runReport":
			return h.s.RunReport(c, t, c.Param("reportId"), key)
		case "listReportRuns":
			return h.s.ListReportRuns(c, t)
		case "getReportRun":
			return h.s.GetReportRun(c, t, c.Param("runId"))
		case "listRecommendations":
			return h.s.ListRecommendations(c, t)
		case "generateRecommendations":
			var in models.GenerateInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			return h.s.GenerateRecommendations(c, t, in)
		case "getRecommendation":
			return h.s.GetRecommendation(c, t, c.Param("recommendationId"))
		case "decideRecommendation":
			var in models.RecommendationDecision
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			in.ExpectedVersion = tenantcmd.ExpectedVersion(c, in.ExpectedVersion)
			return h.s.DecideRecommendation(c, t, c.Param("recommendationId"), in)
		case "listModelVersions":
			return h.s.ListModelVersions(c, t)
		case "listAutomationRules":
			return h.s.ListAutomationRules(c, t)
		case "createAutomationRule":
			var in models.AutomationRuleInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			return h.s.CreateAutomationRule(c, t, in)
		case "getAutomationRule":
			return h.s.GetAutomationRule(c, t, c.Param("ruleId"))
		case "updateAutomationRule", "submitAutomationRule", "approveAutomationRule", "disableAutomationRule":
			var in models.AutomationRuleInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			in.ExpectedVersion = tenantcmd.ExpectedVersion(c, in.ExpectedVersion)
			return h.s.AutomationRuleCommand(c, t, c.Param("ruleId"), ruleCommands[r.OperationID], in)
		case "listAutomationExecutions":
			return h.s.ListAutomationExecutions(c, t)
		}
		return nil, apperrors.NotFound("Resource not found.", nil)
	}
	if r.Method == http.MethodGet {
		v, err := run("")
		if err != nil {
			respond.Error(c, err)
			return
		}
		respond.JSON(c, http.StatusOK, v)
		return
	}
	tenantcmd.Command(c, h.idem, t.OrganizationID, r.Method, r.Path, strings.HasPrefix(r.OperationID, "create"), run)
}
