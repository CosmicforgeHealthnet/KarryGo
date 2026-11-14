// Package usecases implements the Section 22 finance-operations workflows:
// invoices and receivables, expenses, budgets, claims, compensation rules,
// payroll, subcontractor settlements and the delivery of postings and managed
// settlement instructions to the money authority. Wallet balances, ledger
// entries and payment execution remain external.
package usecases

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"cosmicforge/logistics/shared/go/apperrors"
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
	MarkPayrollCalculated(ctx context.Context, q database.Querier, organizationID, id string, calculationVersion int64) (int64, error)
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

// Events publishes domain events inside the caller's transaction.
type Events interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}

// Enqueuer schedules durable jobs inside the caller's transaction.
type Enqueuer interface {
	Enqueue(ctx context.Context, q database.Querier, request jobs.EnqueueRequest) (string, error)
}

// AuditRecorder writes audit records inside the caller's transaction.
type AuditRecorder interface {
	Record(ctx context.Context, q database.Querier, entry audit.Entry) error
}

// PostingPort is the money authority's posting contract.
type PostingPort interface {
	Submit(context.Context, platformclients.CallMeta, models.PostingRequest) (models.PostingOutcome, error)
	ReadByKey(context.Context, platformclients.CallMeta, models.PostingRequest) (models.PostingOutcome, error)
	Configured(operationType string) bool
}

// Options configures the service.
type Options struct {
	Store  Store
	Events Events
	Audit  AuditRecorder
	Queue  Enqueuer
	Media  platformclients.MediaContract
	Wallet PostingPort
	Flags  models.Flags
	Now    func() time.Time
}

// Service implements the finance-operations workflows.
type Service struct{ o Options }

// NewService builds the service.
func NewService(o Options) *Service {
	if o.Audit == nil {
		o.Audit = audit.NewRecorder()
	}
	if o.Queue == nil {
		o.Queue = jobs.NewQueue(nil)
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Service{o: o}
}

func (s *Service) gate(enabled, write bool) error {
	if s.o.Flags.KillSwitch {
		return apperrors.Unavailable("Fleet integrations are disabled.", nil)
	}
	if !enabled {
		if write {
			return apperrors.Forbidden("This finance capability is disabled.", nil)
		}
		return apperrors.NotFound("Resource not found.", nil)
	}
	return nil
}

func (s *Service) tx(ctx context.Context, f func(pgx.Tx) error) error {
	return s.o.Store.WithinTx(ctx, f)
}

func (s *Service) record(ctx context.Context, tx pgx.Tx, t tenant.Context, event, kind, id string, version int64, payload any) error {
	if err := s.o.Audit.Record(ctx, tx, audit.EntryFromTenant(t, strings.TrimSuffix(event, ".v1"), kind, id)); err != nil {
		return err
	}
	if s.o.Events != nil {
		return s.o.Events.Publish(ctx, tx, event, kind, id, version, t.OrganizationID, t.CorrelationID, payload)
	}
	return nil
}

func branchAllowed(t tenant.Context, branch string) bool {
	return branch == "" || t.BranchPermitted(branch)
}

func (s *Service) validateMedia(ctx context.Context, t tenant.Context, fileID string) error {
	if strings.TrimSpace(fileID) == "" {
		return nil
	}
	if s.o.Media == nil || !s.o.Media.Configured() {
		return apperrors.Unavailable("Media validation is unavailable.", nil)
	}
	meta, err := s.o.Media.GetFile(ctx, platformclients.CallMeta{OrganizationID: t.OrganizationID, CorrelationID: t.CorrelationID}, fileID)
	if err != nil {
		return apperrors.Unavailable("Media validation failed.", err)
	}
	if !meta.Ready() || meta.OwnerID != t.OrganizationID {
		return apperrors.Forbidden("Media is not owned and safe for this organization.", nil)
	}
	return nil
}

func expectedRequired(v int64) error {
	if v <= 0 {
		return apperrors.Conflict("expectedVersion is required.", nil)
	}
	return nil
}

func statusString(v map[string]any) string { x, _ := v["status"].(string); return x }

func int64Value(v map[string]any, key string) int64 {
	switch x := v[key].(type) {
	case int64:
		return x
	case float64:
		return int64(x)
	case json.Number:
		n, _ := x.Int64()
		return n
	}
	return 0
}

func operationMeta(t tenant.Context, key string) platformclients.CallMeta {
	return platformclients.CallMeta{OrganizationID: t.OrganizationID, CorrelationID: t.CorrelationID, IdempotencyKey: key}
}
