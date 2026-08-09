package financeops

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

type Route struct {
	Method, Path, OperationID, Permission, FeatureFlag, AuthClass string
	IdempotencyRequired, ExpectedVersionRequired                  bool
}

func TenantRoutes() []Route {
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

type Handler struct {
	s    *Service
	a    *accesscontrol.Authorizer
	idem *idempotency.Store
}

func NewHandler(s *Service, a *accesscontrol.Authorizer, i *idempotency.Store) *Handler {
	return &Handler{s: s, a: a, idem: i}
}
func (h *Handler) TenantRoutes() []Route { return TenantRoutes() }
func (h *Handler) RegisterTenantRoutes(g *gin.RouterGroup) {
	for _, r := range TenantRoutes() {
		r := r
		g.Handle(r.Method, r.Path, func(c *gin.Context) { h.handle(c, r) })
	}
}
func (h *Handler) gate(c *gin.Context, p string) (tenant.Context, bool) {
	t, ok := middleware.TenantFromGin(c)
	if !ok || h.a == nil {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return t, false
	}
	rv := int64(1)
	if x, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		rv, _ = x.(int64)
	}
	if err := h.a.Require(c, t.OrganizationID, t.OrganizationUserID, rv, p, accesscontrol.Target{BranchID: c.Query("branchId")}); err != nil {
		respond.Error(c, err)
		return t, false
	}
	return t, true
}
func bind(c *gin.Context, v any) bool {
	if err := c.ShouldBindJSON(v); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", err))
		return false
	}
	return true
}
func expectedVersion(c *gin.Context, v int64) int64 {
	if v > 0 {
		return v
	}
	n, _ := strconv.ParseInt(strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`), 10, 64)
	return n
}

type commandBody struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason"`
}

func (h *Handler) command(c *gin.Context, t tenant.Context, r Route, run func(string) (any, error)) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		respond.Error(c, apperrors.BadRequest("Idempotency-Key header is required.", nil))
		return
	}
	if h.idem == nil {
		respond.Error(c, apperrors.Internal("Idempotency store is unavailable.", nil))
		return
	}
	raw, err := c.GetRawData()
	if err != nil || (!json.Valid(raw) && len(bytes.TrimSpace(raw)) > 0) {
		respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", err))
		return
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = []byte(`{}`)
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	endpoint := r.Method + " " + r.Path
	rec, err := h.idem.Begin(c, t.OrganizationID, endpoint, key, idempotency.Fingerprint(r.Method, endpoint, raw))
	if err != nil {
		respond.Error(c, idempotency.AsAppError(err))
		return
	}
	if rec != nil {
		c.Data(rec.ResponseStatus, "application/json; charset=utf-8", rec.ResponseBody)
		return
	}
	value, runErr := run(key)
	if errors.Is(runErr, errBound) {
		return
	}
	status := http.StatusOK
	if strings.HasPrefix(r.OperationID, "create") {
		status = http.StatusCreated
	}
	var payload []byte
	if runErr != nil {
		ae := apperrors.From(runErr)
		status = ae.Status
		payload, _ = json.Marshal(gin.H{"error": respond.ErrorBody{Code: ae.Code, Message: ae.Message, Details: []apperrors.FieldViolation{}, RequestID: httpx.GetRequestID(c)}})
	} else {
		payload, _ = json.Marshal(gin.H{"data": value, "meta": gin.H{"requestId": httpx.GetRequestID(c)}})
	}
	if err = h.idem.Complete(c, t.OrganizationID, endpoint, key, status, payload); err != nil {
		respond.Error(c, err)
		return
	}
	c.Data(status, "application/json; charset=utf-8", payload)
}

func (h *Handler) handle(c *gin.Context, r Route) {
	t, ok := h.gate(c, r.Permission)
	if !ok {
		return
	}
	run := func(key string) (any, error) {
		switch r.OperationID {
		case "listInvoices":
			return h.s.ListInvoices(c, t)
		case "createInvoice":
			var in InvoiceInput
			if !bind(c, &in) {
				return nil, errorsBound
			}
			return h.s.CreateInvoice(c, t, in)
		case "getInvoice":
			return h.s.GetInvoice(c, t, c.Param("invoiceId"))
		case "updateInvoice":
			var in InvoiceInput
			if !bind(c, &in) {
				return nil, errorsBound
			}
			in.ExpectedVersion = expectedVersion(c, in.ExpectedVersion)
			return h.s.UpdateInvoice(c, t, c.Param("invoiceId"), in)
		case "approveInvoice", "issueInvoice", "cancelInvoice":
			var b commandBody
			if !bind(c, &b) {
				return nil, errorsBound
			}
			v := expectedVersion(c, b.ExpectedVersion)
			if r.OperationID == "approveInvoice" {
				return h.s.ApproveInvoice(c, t, c.Param("invoiceId"), v)
			}
			if r.OperationID == "issueInvoice" {
				return h.s.IssueInvoice(c, t, c.Param("invoiceId"), v)
			}
			return h.s.CancelInvoice(c, t, c.Param("invoiceId"), v)
		case "creditInvoice":
			var in CreditNoteInput
			if !bind(c, &in) {
				return nil, errorsBound
			}
			in.ExpectedVersion = expectedVersion(c, in.ExpectedVersion)
			return h.s.CreditInvoice(c, t, c.Param("invoiceId"), in)
		case "listReceivables":
			return h.s.ListReceivables(c, t)
		case "getReceivable":
			return h.s.GetReceivable(c, t, c.Param("receivableId"))
		case "listExpenses":
			return h.s.ListExpenses(c, t)
		case "createExpense":
			var in ExpenseInput
			if !bind(c, &in) {
				return nil, errorsBound
			}
			return h.s.CreateExpense(c, t, in)
		case "getExpense":
			return h.s.GetExpense(c, t, c.Param("expenseId"))
		case "updateExpense":
			var in ExpenseInput
			if !bind(c, &in) {
				return nil, errorsBound
			}
			in.ExpectedVersion = expectedVersion(c, in.ExpectedVersion)
			return h.s.UpdateExpense(c, t, c.Param("expenseId"), in)
		case "submitExpense", "approveExpense", "rejectExpense", "reverseExpense":
			var b commandBody
			if !bind(c, &b) {
				return nil, errorsBound
			}
			cmd := map[string]string{"submitExpense": "submit", "approveExpense": "approve", "rejectExpense": "reject", "reverseExpense": "reverse"}[r.OperationID]
			return h.s.ExpenseCommand(c, t, c.Param("expenseId"), cmd, b.Reason, expectedVersion(c, b.ExpectedVersion))
		case "listBudgets":
			return h.s.ListBudgets(c, t)
		case "createBudget":
			var in BudgetInput
			if !bind(c, &in) {
				return nil, errorsBound
			}
			return h.s.CreateBudget(c, t, in)
		case "getBudget":
			return h.s.GetBudget(c, t, c.Param("budgetId"))
		case "updateBudget":
			var in BudgetInput
			if !bind(c, &in) {
				return nil, errorsBound
			}
			in.ExpectedVersion = expectedVersion(c, in.ExpectedVersion)
			return h.s.UpdateBudget(c, t, c.Param("budgetId"), in)
		case "approveBudget", "closeBudget":
			var b commandBody
			if !bind(c, &b) {
				return nil, errorsBound
			}
			cmd := strings.TrimSuffix(r.OperationID, "Budget")
			return h.s.BudgetCommand(c, t, c.Param("budgetId"), cmd, expectedVersion(c, b.ExpectedVersion))
		case "listCompensationRules":
			return h.s.ListCompensationRules(c, t)
		case "createCompensationRule":
			var in CompensationRuleInput
			if !bind(c, &in) {
				return nil, errorsBound
			}
			return h.s.CreateCompensationRule(c, t, in)
		case "getCompensationRule":
			return h.s.GetCompensationRule(c, t, c.Param("ruleId"))
		case "updateCompensationRule":
			var in CompensationRuleInput
			if !bind(c, &in) {
				return nil, errorsBound
			}
			in.ExpectedVersion = expectedVersion(c, in.ExpectedVersion)
			return h.s.UpdateCompensationRule(c, t, c.Param("ruleId"), in)
		case "approveCompensationRule", "deactivateCompensationRule":
			var b commandBody
			if !bind(c, &b) {
				return nil, errorsBound
			}
			cmd := strings.TrimSuffix(r.OperationID, "CompensationRule")
			return h.s.CompensationRuleCommand(c, t, c.Param("ruleId"), cmd, expectedVersion(c, b.ExpectedVersion))
		case "listPayrollPeriods":
			return h.s.ListPayrollPeriods(c, t)
		case "createPayrollPeriod":
			var in PayrollPeriodInput
			if !bind(c, &in) {
				return nil, errorsBound
			}
			return h.s.CreatePayrollPeriod(c, t, in)
		case "getPayrollPeriod":
			return h.s.GetPayrollPeriod(c, t, c.Param("periodId"))
		case "calculatePayroll", "approvePayroll", "postPayroll", "reversePayroll":
			var b commandBody
			if !bind(c, &b) {
				return nil, errorsBound
			}
			v := expectedVersion(c, b.ExpectedVersion)
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
			var in SettlementInput
			if !bind(c, &in) {
				return nil, errorsBound
			}
			return h.s.CreateSettlement(c, t, in)
		case "getSettlement":
			return h.s.GetSettlement(c, t, c.Param("settlementId"))
		case "approveSettlement", "postSettlement":
			var b commandBody
			if !bind(c, &b) {
				return nil, errorsBound
			}
			cmd := strings.TrimSuffix(r.OperationID, "Settlement")
			return h.s.SettlementCommand(c, t, c.Param("settlementId"), cmd, key, expectedVersion(c, b.ExpectedVersion))
		case "listClaims":
			return h.s.ListClaims(c, t)
		case "createClaim":
			var in ClaimInput
			if !bind(c, &in) {
				return nil, errorsBound
			}
			return h.s.CreateClaim(c, t, in)
		case "getClaim":
			return h.s.GetClaim(c, t, c.Param("claimId"))
		case "updateClaim":
			var in ClaimInput
			if !bind(c, &in) {
				return nil, errorsBound
			}
			in.ExpectedVersion = expectedVersion(c, in.ExpectedVersion)
			return h.s.UpdateClaim(c, t, c.Param("claimId"), in)
		}
		return nil, apperrors.NotFound("Resource not found.", nil)
	}
	if r.Method == http.MethodGet {
		value, err := run("")
		if err != nil {
			respond.Error(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": value, "meta": gin.H{"requestId": httpx.GetRequestID(c)}})
		return
	}
	h.command(c, t, r, run)
}

var errBound = errors.New("request body already rendered")
var errorsBound = errBound

type InternalHandler struct{ s *Service }

func NewInternalHandler(s *Service) *InternalHandler { return &InternalHandler{s: s} }
func (h *InternalHandler) RegisterInternalRoutes(g *gin.RouterGroup) {
	g.POST("/finance/payment-events", h.payment)
}
func (h *InternalHandler) payment(c *gin.Context) {
	var in PaymentEvent
	if !bind(c, &in) {
		return
	}
	in.OrganizationID = c.GetHeader("X-Organization-ID")
	if err := h.s.ApplyPayment(c, in); err != nil {
		respond.Error(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": gin.H{"accepted": true}})
}
