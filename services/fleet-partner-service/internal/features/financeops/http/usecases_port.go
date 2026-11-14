package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Service is the slice of the finance-operations use cases the tenant
// handler calls.
type Service interface {
	ListInvoices(ctx context.Context, t tenant.Context) ([]map[string]any, error)
	CreateInvoice(ctx context.Context, t tenant.Context, in models.InvoiceInput) (map[string]any, error)
	GetInvoice(ctx context.Context, t tenant.Context, id string) (map[string]any, error)
	UpdateInvoice(ctx context.Context, t tenant.Context, id string, in models.InvoiceInput) (map[string]any, error)
	ApproveInvoice(ctx context.Context, t tenant.Context, id string, v int64) (map[string]any, error)
	IssueInvoice(ctx context.Context, t tenant.Context, id string, v int64) (map[string]any, error)
	CancelInvoice(ctx context.Context, t tenant.Context, id string, v int64) (map[string]any, error)
	CreditInvoice(ctx context.Context, t tenant.Context, id string, in models.CreditNoteInput) (map[string]any, error)
	ListReceivables(ctx context.Context, t tenant.Context) ([]map[string]any, error)
	GetReceivable(ctx context.Context, t tenant.Context, id string) (map[string]any, error)
	ListExpenses(ctx context.Context, t tenant.Context) ([]map[string]any, error)
	CreateExpense(ctx context.Context, t tenant.Context, in models.ExpenseInput) (map[string]any, error)
	GetExpense(ctx context.Context, t tenant.Context, id string) (map[string]any, error)
	UpdateExpense(ctx context.Context, t tenant.Context, id string, in models.ExpenseInput) (map[string]any, error)
	ExpenseCommand(ctx context.Context, t tenant.Context, id, command, reason string, expected int64) (map[string]any, error)
	ListBudgets(ctx context.Context, t tenant.Context) ([]map[string]any, error)
	CreateBudget(ctx context.Context, t tenant.Context, in models.BudgetInput) (map[string]any, error)
	GetBudget(ctx context.Context, t tenant.Context, id string) (map[string]any, error)
	UpdateBudget(ctx context.Context, t tenant.Context, id string, in models.BudgetInput) (map[string]any, error)
	BudgetCommand(ctx context.Context, t tenant.Context, id, command string, expected int64) (map[string]any, error)
	ListCompensationRules(ctx context.Context, t tenant.Context) ([]map[string]any, error)
	CreateCompensationRule(ctx context.Context, t tenant.Context, in models.CompensationRuleInput) (map[string]any, error)
	GetCompensationRule(ctx context.Context, t tenant.Context, id string) (map[string]any, error)
	UpdateCompensationRule(ctx context.Context, t tenant.Context, id string, in models.CompensationRuleInput) (map[string]any, error)
	CompensationRuleCommand(ctx context.Context, t tenant.Context, id, command string, expected int64) (map[string]any, error)
	ListPayrollPeriods(ctx context.Context, t tenant.Context) ([]map[string]any, error)
	CreatePayrollPeriod(ctx context.Context, t tenant.Context, in models.PayrollPeriodInput) (map[string]any, error)
	GetPayrollPeriod(ctx context.Context, t tenant.Context, id string) (map[string]any, error)
	CalculatePayroll(ctx context.Context, t tenant.Context, id string, expected int64) (map[string]any, error)
	ApprovePayroll(ctx context.Context, t tenant.Context, id string, expected int64) (map[string]any, error)
	PostPayroll(ctx context.Context, t tenant.Context, id, key string, expected int64) (map[string]any, error)
	ReversePayroll(ctx context.Context, t tenant.Context, id, reason string, expected int64) (map[string]any, error)
	ProviderStatements(ctx context.Context, t tenant.Context, membershipID string) (models.Statement, error)
	ListSettlements(ctx context.Context, t tenant.Context) ([]map[string]any, error)
	CreateSettlement(ctx context.Context, t tenant.Context, in models.SettlementInput) (map[string]any, error)
	GetSettlement(ctx context.Context, t tenant.Context, id string) (map[string]any, error)
	SettlementCommand(ctx context.Context, t tenant.Context, id, command, key string, expected int64) (map[string]any, error)
	ListClaims(ctx context.Context, t tenant.Context) ([]map[string]any, error)
	CreateClaim(ctx context.Context, t tenant.Context, in models.ClaimInput) (map[string]any, error)
	GetClaim(ctx context.Context, t tenant.Context, id string) (map[string]any, error)
	UpdateClaim(ctx context.Context, t tenant.Context, id string, in models.ClaimInput) (map[string]any, error)
}

// PaymentIngress is the use case the internal payment-event route calls.
type PaymentIngress interface {
	ApplyPayment(ctx context.Context, in models.PaymentEvent) error
}
