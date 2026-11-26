package usecases

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops/models"
)

// Store is the persistence the finance-operations workflows need.
// repositories.Repository satisfies it; a nil Querier means the pool.
type Store interface {
	WithinTx(ctx context.Context, f func(pgx.Tx) error) error
	ActiveOrganizations(ctx context.Context) ([]string, error)

	CustomerExists(ctx context.Context, q database.Querier, organizationID, id string) (bool, error)
	ContractExists(ctx context.Context, q database.Querier, organizationID, id string) (bool, error)
	QuotationExists(ctx context.Context, q database.Querier, organizationID, id string) (bool, error)
	InsertInvoice(ctx context.Context, q database.Querier, id, organizationID, actorID string, in models.InvoiceInput, totals models.InvoiceTotals) error
	InsertInvoiceLines(ctx context.Context, q database.Querier, organizationID, invoiceID string, lines []models.CalculatedLine) error
	DeleteInvoiceLines(ctx context.Context, q database.Querier, organizationID, invoiceID string) error
	GetInvoice(ctx context.Context, organizationID, id string) (map[string]any, error)
	InvoiceSnapshot(ctx context.Context, q database.Querier, organizationID, id string) ([]byte, error)
	ListInvoices(ctx context.Context, organizationID string, branchIDs []string) ([]map[string]any, error)
	LockInvoice(ctx context.Context, q database.Querier, organizationID, id string) (models.InvoiceState, error)
	UpdateInvoiceTotals(ctx context.Context, q database.Querier, organizationID, id string, totals models.InvoiceTotals, due time.Time) (int64, error)
	SetInvoiceStatus(ctx context.Context, q database.Querier, organizationID, id, status string) (int64, error)
	InsertInvoiceSnapshot(ctx context.Context, q database.Querier, organizationID, invoiceID string, version int64, snapshot []byte, actorID string) error
	CreditInvoiceTotals(ctx context.Context, q database.Querier, organizationID, id string, amount int64, status string) (int64, error)
	SetInvoicePaymentStatus(ctx context.Context, q database.Querier, organizationID, invoiceID, status string) error
	InsertCreditNote(ctx context.Context, q database.Querier, id, organizationID, invoiceID, currency string, amount int64, affectedLines json.RawMessage, reason, actorID string) error
	GetCreditNote(ctx context.Context, organizationID, id string) (map[string]any, error)

	InsertReceivable(ctx context.Context, q database.Querier, organizationID, invoiceID string, amount int64, currency string, due time.Time) error
	LockReceivable(ctx context.Context, q database.Querier, organizationID, invoiceID string) (models.ReceivableState, error)
	AdjustReceivable(ctx context.Context, q database.Querier, organizationID, id string, original, outstanding int64, status string) error
	InsertReceivablePayment(ctx context.Context, q database.Querier, receivableID string, in models.PaymentEvent, amount int64, currency string) error
	ApplyReceivablePayment(ctx context.Context, q database.Querier, receivableID string, in models.PaymentEvent, paid, outstanding int64, status string) error
	ListReceivables(ctx context.Context, organizationID string) ([]map[string]any, error)
	GetReceivable(ctx context.Context, organizationID, id string) (map[string]any, error)
	SweepOverdue(ctx context.Context, limit int) (int64, error)

	InsertExpense(ctx context.Context, q database.Querier, id, organizationID, actorID string, in models.ExpenseInput, currency string) error
	GetExpense(ctx context.Context, organizationID, id string) (map[string]any, error)
	ListExpenses(ctx context.Context, organizationID string, branchIDs []string) ([]map[string]any, error)
	ExpenseState(ctx context.Context, q database.Querier, organizationID, id string, lock bool) (models.ExpenseState, error)
	UpdateExpense(ctx context.Context, q database.Querier, organizationID, id string, in models.ExpenseInput, currency string) (int64, error)
	SetExpenseStatus(ctx context.Context, q database.Querier, organizationID, id, status, actorID string) (int64, error)
	MarkExpenseReversed(ctx context.Context, q database.Querier, organizationID, id string) (int64, error)
	MarkExpensePosted(ctx context.Context, organizationID, id, operationID string) error
	InsertExpenseHistory(ctx context.Context, q database.Querier, organizationID, id, previous, next string, snapshot []byte, actorID, reason string, version int64) error
	InsertExpenseReversal(ctx context.Context, q database.Querier, organizationID, id string, amount int64, currency, reason, postingOperationID, actorID string) error

	InsertBudget(ctx context.Context, q database.Querier, id, organizationID string, in models.BudgetInput, currency string) error
	GetBudget(ctx context.Context, organizationID, id string) (map[string]any, error)
	ListBudgets(ctx context.Context, organizationID string) ([]map[string]any, error)
	UpdateBudget(ctx context.Context, organizationID, id string, in models.BudgetInput, currency string) (bool, error)
	LockBudget(ctx context.Context, q database.Querier, organizationID, id string) (string, int64, error)
	SetBudgetStatus(ctx context.Context, q database.Querier, organizationID, id, status, actorID string) (int64, error)
	InsertBudgetHistory(ctx context.Context, q database.Querier, organizationID, id, previous, next string, snapshot []byte, actorID string, version int64) error
	ProjectBudgets(ctx context.Context, organizationID string) error

	InsertClaim(ctx context.Context, q database.Querier, id, organizationID string, in models.ClaimInput, currency, status string, evidence []byte) error
	GetClaim(ctx context.Context, organizationID, id string) (map[string]any, error)
	ListClaims(ctx context.Context, organizationID string) ([]map[string]any, error)
	LockClaim(ctx context.Context, q database.Querier, organizationID, id string) (string, int64, error)
	UpdateClaim(ctx context.Context, q database.Querier, organizationID, id, status string, in models.ClaimInput, evidence []byte) (int64, error)
	InsertClaimHistory(ctx context.Context, q database.Querier, organizationID, id, previous, next string, snapshot []byte, actorID string) error
	TouchOpenClaims(ctx context.Context, organizationID string) error

	InsertCompensationRule(ctx context.Context, q database.Querier, id, organizationID, actorID string, in models.CompensationRuleInput, roles, services, cfg json.RawMessage, currency string) error
	InsertRuleVersion(ctx context.Context, q database.Querier, organizationID, ruleID string, version int64, snapshot []byte, actorID string) error
	GetCompensationRule(ctx context.Context, organizationID, id string) (map[string]any, error)
	ListCompensationRules(ctx context.Context, organizationID string) ([]map[string]any, error)
	LockCompensationRule(ctx context.Context, q database.Querier, organizationID, id string) (string, int64, error)
	UpdateCompensationRule(ctx context.Context, q database.Querier, organizationID, id string, in models.CompensationRuleInput, cfg json.RawMessage) (int64, error)
	TransitionCompensationRule(ctx context.Context, organizationID, id, next, actorID string, expected int64) (bool, error)

	InsertPayrollPeriod(ctx context.Context, q database.Querier, id, organizationID, actorID string, in models.PayrollPeriodInput, currency string) error
	GetPayrollPeriod(ctx context.Context, organizationID, id string) (map[string]any, error)
	ListPayrollPeriods(ctx context.Context, organizationID string, branchIDs []string) ([]map[string]any, error)
	PayrollState(ctx context.Context, q database.Querier, organizationID, id string, lock bool) (models.PayrollState, error)
	ActiveCompensationRules(ctx context.Context, q database.Querier, organizationID, currency string, start, end time.Time) ([]models.ActiveRule, error)
	CompletedAssignments(ctx context.Context, q database.Querier, organizationID, branch string, start, end time.Time) ([]models.SourceAssignment, error)
	WorkSettledComponents(ctx context.Context, q database.Querier, organizationID string, assignmentIDs []string) (map[string]bool, error)
	ActiveMemberships(ctx context.Context, q database.Querier, organizationID string) ([]string, error)
	InsertCompensationItem(ctx context.Context, q database.Querier, item models.CompensationItem) (int64, error)
	MarkPayrollCalculated(ctx context.Context, q database.Querier, organizationID, id string, calculationVersion int64, calculationHash string) (int64, error)
	PayrollCalculationHash(ctx context.Context, q database.Querier, organizationID, id string) (string, error)
	InsertPayrollHistory(ctx context.Context, q database.Querier, organizationID, id, previous, next string, calculationVersion int64, snapshot []byte, actorID, reason string) error
	ApprovePayrollPeriod(ctx context.Context, q database.Querier, organizationID, id, actorID string) (int64, error)
	ApproveCompensationItems(ctx context.Context, q database.Querier, organizationID, periodID string, calculationVersion int64) error
	PayrollItems(ctx context.Context, organizationID, periodID string, statuses []string) ([]models.PayrollItem, error)
	CompensationItemStatus(ctx context.Context, organizationID, id string) (string, error)
	MarkCompensationItemPosted(ctx context.Context, organizationID, id string) error
	MarkPayrollPartiallyPosted(ctx context.Context, organizationID, id string) error
	MarkPayrollPosted(ctx context.Context, q database.Querier, organizationID, id, lastOperationID string) (int64, bool, error)
	MarkPayrollReversed(ctx context.Context, q database.Querier, organizationID, id string, expected int64) (int64, error)
	ReverseCompensationItems(ctx context.Context, q database.Querier, organizationID, periodID string) error
	MembershipExists(ctx context.Context, organizationID, id string) (bool, error)
	ProviderStatementPeriods(ctx context.Context, organizationID, membershipID string) ([]json.RawMessage, error)

	InsertSettlement(ctx context.Context, q database.Querier, id, organizationID string, in models.SettlementInput, currency, actorID string) error
	CompletedAssignmentExists(ctx context.Context, q database.Querier, organizationID, id string) (bool, error)
	InsertSettlementJob(ctx context.Context, q database.Querier, settlementID, organizationID, job string) error
	GetSettlement(ctx context.Context, organizationID, id string) (map[string]any, error)
	ListSettlements(ctx context.Context, organizationID string) ([]map[string]any, error)
	ApproveSettlement(ctx context.Context, organizationID, id, actorID string, expected int64) (bool, error)
	MarkSettlementPosted(ctx context.Context, organizationID, id, operationID string) error

	InsertPostingOperation(ctx context.Context, id string, req models.PostingRequest, fingerprint string) (bool, error)
	PostingOperationByKey(ctx context.Context, organizationID, operationType, key string) (string, string, string, error)
	RecordPostingAttempt(ctx context.Context, q database.Querier, a models.PostingAttemptRecord) error
	PostingState(ctx context.Context, organizationID, operationID string) (models.PostingOperationState, error)
	MarkPostingFailed(ctx context.Context, organizationID, operationID, code string) error
	DuePostingOperations(ctx context.Context, organizationID, excludeType string) ([]models.DuePosting, error)
	ConfirmPostingOperation(ctx context.Context, organizationID, id, reference string, sourceVersion int64) error
}
