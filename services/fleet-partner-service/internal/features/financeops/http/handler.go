// Package http serves the Section 22 finance-operations routes and the
// internal payment-event ingress. Responses carry the standard Money object
// beside each flat money pair (spec 6.1).
package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenantcmd"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Route describes one tenant route.
type Route struct {
	Method, Path, OperationID, Permission, FeatureFlag, AuthClass string
	IdempotencyRequired, ExpectedVersionRequired                  bool
}

func TenantRoutes() []Route {
	return append(coreRoutes(), analysisRoutes()...)
}

func coreRoutes() []Route {
	return []Route{
		{"GET", "/invoices", "listInvoices", "invoices.read", "INVOICES_ENABLED", "corporate", false, false}, {"POST", "/invoices", "createInvoice", "invoices.manage", "INVOICES_ENABLED", "corporate", true, false}, {"GET", "/invoices/:invoiceId", "getInvoice", "invoices.read", "INVOICES_ENABLED", "corporate", false, false}, {"PATCH", "/invoices/:invoiceId", "updateInvoice", "invoices.manage", "INVOICES_ENABLED", "corporate", true, true}, {"POST", "/invoices/:invoiceId/approve", "approveInvoice", "invoices.approve", "INVOICES_ENABLED", "corporate", true, true}, {"POST", "/invoices/:invoiceId/issue", "issueInvoice", "invoices.issue", "INVOICES_ENABLED", "corporate", true, true}, {"POST", "/invoices/:invoiceId/credit-note", "creditInvoice", "invoices.credit", "INVOICES_ENABLED", "corporate", true, true}, {"POST", "/invoices/:invoiceId/cancel", "cancelInvoice", "invoices.manage", "INVOICES_ENABLED", "corporate", true, true},
		{"GET", "/receivables", "listReceivables", "receivables.read", "RECEIVABLES_ENABLED", "corporate", false, false}, {"GET", "/receivables/:receivableId", "getReceivable", "receivables.read", "RECEIVABLES_ENABLED", "corporate", false, false},
		{"GET", "/expenses", "listExpenses", "expenses.read", "EXPENSES_ENABLED", "corporate", false, false}, {"POST", "/expenses", "createExpense", "expenses.manage", "EXPENSES_ENABLED", "corporate", true, false}, {"GET", "/expenses/:expenseId", "getExpense", "expenses.read", "EXPENSES_ENABLED", "corporate", false, false}, {"PATCH", "/expenses/:expenseId", "updateExpense", "expenses.manage", "EXPENSES_ENABLED", "corporate", true, true}, {"POST", "/expenses/:expenseId/submit", "submitExpense", "expenses.manage", "EXPENSES_ENABLED", "corporate", true, true}, {"POST", "/expenses/:expenseId/approve", "approveExpense", "expenses.approve", "EXPENSES_ENABLED", "corporate", true, true}, {"POST", "/expenses/:expenseId/reject", "rejectExpense", "expenses.approve", "EXPENSES_ENABLED", "corporate", true, true}, {"POST", "/expenses/:expenseId/reverse", "reverseExpense", "expenses.reverse", "EXPENSES_ENABLED", "corporate", true, true},
		{"GET", "/budgets", "listBudgets", "budgets.read", "BUDGETS_ENABLED", "corporate", false, false}, {"POST", "/budgets", "createBudget", "budgets.manage", "BUDGETS_ENABLED", "corporate", true, false}, {"GET", "/budgets/:budgetId", "getBudget", "budgets.read", "BUDGETS_ENABLED", "corporate", false, false}, {"PATCH", "/budgets/:budgetId", "updateBudget", "budgets.manage", "BUDGETS_ENABLED", "corporate", true, true}, {"POST", "/budgets/:budgetId/approve", "approveBudget", "budgets.approve", "BUDGETS_ENABLED", "corporate", true, true}, {"POST", "/budgets/:budgetId/close", "closeBudget", "budgets.manage", "BUDGETS_ENABLED", "corporate", true, true},
		{"GET", "/compensation-rules", "listCompensationRules", "compensation.read", "COMPENSATION_ENABLED", "corporate", false, false}, {"POST", "/compensation-rules", "createCompensationRule", "compensation.manage", "COMPENSATION_ENABLED", "corporate", true, false}, {"GET", "/compensation-rules/:ruleId", "getCompensationRule", "compensation.read", "COMPENSATION_ENABLED", "corporate", false, false}, {"PATCH", "/compensation-rules/:ruleId", "updateCompensationRule", "compensation.manage", "COMPENSATION_ENABLED", "corporate", true, true}, {"POST", "/compensation-rules/:ruleId/approve", "approveCompensationRule", "compensation.approve", "COMPENSATION_ENABLED", "corporate", true, true}, {"POST", "/compensation-rules/:ruleId/deactivate", "deactivateCompensationRule", "compensation.manage", "COMPENSATION_ENABLED", "corporate", true, true},
		{"GET", "/payroll-periods", "listPayrollPeriods", "payroll.read", "PAYROLL_ENABLED", "corporate", false, false}, {"POST", "/payroll-periods", "createPayrollPeriod", "payroll.manage", "PAYROLL_ENABLED", "corporate", true, false}, {"GET", "/payroll-periods/:periodId", "getPayrollPeriod", "payroll.read", "PAYROLL_ENABLED", "corporate", false, false}, {"POST", "/payroll-periods/:periodId/calculate", "calculatePayroll", "payroll.calculate", "PAYROLL_ENABLED", "corporate", true, true}, {"POST", "/payroll-periods/:periodId/approve", "approvePayroll", "payroll.approve", "PAYROLL_ENABLED", "corporate", true, true}, {"POST", "/payroll-periods/:periodId/post", "postPayroll", "payroll.post", "PAYROLL_ENABLED", "corporate", true, true}, {"POST", "/payroll-periods/:periodId/reverse", "reversePayroll", "payroll.reverse", "PAYROLL_ENABLED", "corporate", true, true}, {"GET", "/providers/:membershipId/statements", "providerStatements", "payroll.read", "PAYROLL_ENABLED", "corporate", false, false},
		{"GET", "/subcontractor-settlements", "listSettlements", "subcontractor_settlements.read", "SUBCONTRACTOR_SETTLEMENTS_ENABLED", "corporate", false, false}, {"POST", "/subcontractor-settlements", "createSettlement", "subcontractor_settlements.manage", "SUBCONTRACTOR_SETTLEMENTS_ENABLED", "corporate", true, false}, {"GET", "/subcontractor-settlements/:settlementId", "getSettlement", "subcontractor_settlements.read", "SUBCONTRACTOR_SETTLEMENTS_ENABLED", "corporate", false, false}, {"POST", "/subcontractor-settlements/:settlementId/approve", "approveSettlement", "subcontractor_settlements.approve", "SUBCONTRACTOR_SETTLEMENTS_ENABLED", "corporate", true, true}, {"POST", "/subcontractor-settlements/:settlementId/post", "postSettlement", "subcontractor_settlements.post", "SUBCONTRACTOR_SETTLEMENTS_ENABLED", "corporate", true, true},
		{"GET", "/claims", "listClaims", "claims.read", "CLAIMS_ENABLED", "corporate", false, false}, {"POST", "/claims", "createClaim", "claims.manage", "CLAIMS_ENABLED", "corporate", true, false}, {"GET", "/claims/:claimId", "getClaim", "claims.read", "CLAIMS_ENABLED", "corporate", false, false}, {"PATCH", "/claims/:claimId", "updateClaim", "claims.manage", "CLAIMS_ENABLED", "corporate", true, true},
	}
}
func (r Route) FullPath() string { return "/fleet-partner/v1" + r.Path }

// Handler serves the finance-operations tenant routes.
type Handler struct {
	s    Service
	a    tenantcmd.ScopedAuthorizer
	idem *idempotency.Store
}

// NewHandler builds the handler.
func NewHandler(s Service, a tenantcmd.ScopedAuthorizer, i *idempotency.Store) *Handler {
	return &Handler{s: s, a: a, idem: i}
}
func (h *Handler) TenantRoutes() []Route { return TenantRoutes() }
func (h *Handler) RegisterTenantRoutes(g *gin.RouterGroup) {
	for _, r := range TenantRoutes() {
		r := r
		g.Handle(r.Method, r.Path, func(c *gin.Context) { h.handle(c, r) })
	}
}

type commandBody struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason"`
}

func (h *Handler) handle(c *gin.Context, r Route) {
	t, ok := tenantcmd.Gate(c, h.a, r.Permission)
	if !ok {
		return
	}
	run := func(key string) (any, error) {
		if v, err, handled := h.analysis(c, t, r); handled {
			return v, err
		}
		switch r.OperationID {
		case "listInvoices":
			return h.s.ListInvoices(c, t)
		case "createInvoice":
			var in models.InvoiceInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			return h.s.CreateInvoice(c, t, in)
		case "getInvoice":
			return h.s.GetInvoice(c, t, c.Param("invoiceId"))
		case "updateInvoice":
			var in models.InvoiceInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			in.ExpectedVersion = tenantcmd.ExpectedVersion(c, in.ExpectedVersion)
			return h.s.UpdateInvoice(c, t, c.Param("invoiceId"), in)
		case "approveInvoice", "issueInvoice", "cancelInvoice":
			var b commandBody
			if !tenantcmd.Bind(c, &b) {
				return nil, tenantcmd.ErrBound
			}
			v := tenantcmd.ExpectedVersion(c, b.ExpectedVersion)
			if r.OperationID == "approveInvoice" {
				return h.s.ApproveInvoice(c, t, c.Param("invoiceId"), v)
			}
			if r.OperationID == "issueInvoice" {
				return h.s.IssueInvoice(c, t, c.Param("invoiceId"), v)
			}
			return h.s.CancelInvoice(c, t, c.Param("invoiceId"), v)
		case "creditInvoice":
			var in models.CreditNoteInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			in.ExpectedVersion = tenantcmd.ExpectedVersion(c, in.ExpectedVersion)
			return h.s.CreditInvoice(c, t, c.Param("invoiceId"), in)
		case "listReceivables":
			return h.s.ListReceivables(c, t)
		case "getReceivable":
			return h.s.GetReceivable(c, t, c.Param("receivableId"))
		case "listExpenses":
			return h.s.ListExpenses(c, t)
		case "createExpense":
			var in models.ExpenseInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			return h.s.CreateExpense(c, t, in)
		case "getExpense":
			return h.s.GetExpense(c, t, c.Param("expenseId"))
		case "updateExpense":
			var in models.ExpenseInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			in.ExpectedVersion = tenantcmd.ExpectedVersion(c, in.ExpectedVersion)
			return h.s.UpdateExpense(c, t, c.Param("expenseId"), in)
		case "submitExpense", "approveExpense", "rejectExpense", "reverseExpense":
			var b commandBody
			if !tenantcmd.Bind(c, &b) {
				return nil, tenantcmd.ErrBound
			}
			cmd := map[string]string{"submitExpense": "submit", "approveExpense": "approve", "rejectExpense": "reject", "reverseExpense": "reverse"}[r.OperationID]
			return h.s.ExpenseCommand(c, t, c.Param("expenseId"), cmd, b.Reason, tenantcmd.ExpectedVersion(c, b.ExpectedVersion))
		case "listBudgets":
			return h.s.ListBudgets(c, t)
		case "createBudget":
			var in models.BudgetInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			return h.s.CreateBudget(c, t, in)
		case "getBudget":
			return h.s.GetBudget(c, t, c.Param("budgetId"))
		case "updateBudget":
			var in models.BudgetInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			in.ExpectedVersion = tenantcmd.ExpectedVersion(c, in.ExpectedVersion)
			return h.s.UpdateBudget(c, t, c.Param("budgetId"), in)
		case "approveBudget", "closeBudget":
			var b commandBody
			if !tenantcmd.Bind(c, &b) {
				return nil, tenantcmd.ErrBound
			}
			cmd := strings.TrimSuffix(r.OperationID, "Budget")
			return h.s.BudgetCommand(c, t, c.Param("budgetId"), cmd, tenantcmd.ExpectedVersion(c, b.ExpectedVersion))
		case "listCompensationRules":
			return h.s.ListCompensationRules(c, t)
		case "createCompensationRule":
			var in models.CompensationRuleInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			return h.s.CreateCompensationRule(c, t, in)
		case "getCompensationRule":
			return h.s.GetCompensationRule(c, t, c.Param("ruleId"))
		case "updateCompensationRule":
			var in models.CompensationRuleInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			in.ExpectedVersion = tenantcmd.ExpectedVersion(c, in.ExpectedVersion)
			return h.s.UpdateCompensationRule(c, t, c.Param("ruleId"), in)
		case "approveCompensationRule", "deactivateCompensationRule":
			var b commandBody
			if !tenantcmd.Bind(c, &b) {
				return nil, tenantcmd.ErrBound
			}
			cmd := strings.TrimSuffix(r.OperationID, "CompensationRule")
			return h.s.CompensationRuleCommand(c, t, c.Param("ruleId"), cmd, tenantcmd.ExpectedVersion(c, b.ExpectedVersion))
		case "listPayrollPeriods":
			return h.s.ListPayrollPeriods(c, t)
		case "createPayrollPeriod":
			var in models.PayrollPeriodInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			return h.s.CreatePayrollPeriod(c, t, in)
		case "getPayrollPeriod":
			return h.s.GetPayrollPeriod(c, t, c.Param("periodId"))
		case "calculatePayroll", "approvePayroll", "postPayroll", "reversePayroll":
			var b commandBody
			if !tenantcmd.Bind(c, &b) {
				return nil, tenantcmd.ErrBound
			}
			v := tenantcmd.ExpectedVersion(c, b.ExpectedVersion)
			if r.OperationID == "calculatePayroll" {
				return h.s.CalculatePayroll(c, t, c.Param("periodId"), v)
			}
			if r.OperationID == "approvePayroll" {
				return h.s.ApprovePayroll(c, t, c.Param("periodId"), v)
			}
			if r.OperationID == "postPayroll" {
				return h.s.PostPayroll(c, t, c.Param("periodId"), key, v)
			}
			return h.s.ReversePayroll(c, t, c.Param("periodId"), b.Reason, v)
		case "providerStatements":
			return h.s.ProviderStatements(c, t, c.Param("membershipId"))
		case "listSettlements":
			return h.s.ListSettlements(c, t)
		case "createSettlement":
			var in models.SettlementInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			return h.s.CreateSettlement(c, t, in)
		case "getSettlement":
			return h.s.GetSettlement(c, t, c.Param("settlementId"))
		case "approveSettlement", "postSettlement":
			var b commandBody
			if !tenantcmd.Bind(c, &b) {
				return nil, tenantcmd.ErrBound
			}
			cmd := strings.TrimSuffix(r.OperationID, "Settlement")
			return h.s.SettlementCommand(c, t, c.Param("settlementId"), cmd, key, tenantcmd.ExpectedVersion(c, b.ExpectedVersion))
		case "listClaims":
			return h.s.ListClaims(c, t)
		case "createClaim":
			var in models.ClaimInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			return h.s.CreateClaim(c, t, in)
		case "getClaim":
			return h.s.GetClaim(c, t, c.Param("claimId"))
		case "updateClaim":
			var in models.ClaimInput
			if !tenantcmd.Bind(c, &in) {
				return nil, tenantcmd.ErrBound
			}
			in.ExpectedVersion = tenantcmd.ExpectedVersion(c, in.ExpectedVersion)
			return h.s.UpdateClaim(c, t, c.Param("claimId"), in)
		}
		return nil, apperrors.NotFound("Resource not found.", nil)
	}
	// Every finance record carries the Money object beside its flat
	// "<x>_minor" amounts (spec 6.1); the flat fields are unchanged.
	withMoney := func(key string) (any, error) {
		v, err := run(key)
		if err != nil {
			return v, err
		}
		return models.WithMoney(v), nil
	}
	if r.Method == http.MethodGet {
		value, err := withMoney("")
		if err != nil {
			respond.Error(c, err)
			return
		}
		respond.JSON(c, http.StatusOK, value)
		return
	}
	tenantcmd.Command(c, h.idem, t.OrganizationID, r.Method, r.Path, strings.HasPrefix(r.OperationID, "create"), withMoney)
}

// InternalHandler serves the internal payment-event ingress.
type InternalHandler struct{ s PaymentIngress }

// NewInternalHandler builds the internal handler.
func NewInternalHandler(s PaymentIngress) *InternalHandler { return &InternalHandler{s: s} }
func (h *InternalHandler) RegisterInternalRoutes(g *gin.RouterGroup) {
	g.POST("/finance/payment-events", h.payment)
}
func (h *InternalHandler) payment(c *gin.Context) {
	var in models.PaymentEvent
	if !tenantcmd.Bind(c, &in) {
		return
	}
	in.OrganizationID = c.GetHeader("X-Organization-ID")
	if err := h.s.ApplyPayment(c, in); err != nil {
		respond.Error(c, err)
		return
	}
	respond.JSON(c, http.StatusAccepted, gin.H{"accepted": true})
}
