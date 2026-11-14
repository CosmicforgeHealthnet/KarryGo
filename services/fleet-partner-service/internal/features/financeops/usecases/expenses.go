package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// CreateExpense records a draft expense.
func (s *Service) CreateExpense(ctx context.Context, t tenant.Context, in models.ExpenseInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Expenses, true); err != nil {
		return nil, err
	}
	if !branchAllowed(t, in.BranchID) {
		return nil, apperrors.Forbidden("Branch access is denied.", nil)
	}
	currency, err := models.NormalizeCurrency(in.Currency)
	if err != nil {
		return nil, err
	}
	if in.AmountMinor <= 0 || strings.TrimSpace(in.Category) == "" || strings.TrimSpace(in.CostCentre) == "" || strings.TrimSpace(in.Description) == "" {
		return nil, apperrors.Validation("Category, cost centre, description and a positive minor-unit amount are required.", nil)
	}
	if err = s.validateMedia(ctx, t, in.ReceiptMediaID); err != nil {
		return nil, err
	}
	id := ids.New("exp")
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := s.o.Store.InsertExpense(ctx, tx, id, t.OrganizationID, t.ActorID, in, currency); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "expense.created.v1", "expense", id, 1, map[string]any{"expenseId": id, "currency": currency, "amountMinor": in.AmountMinor, "category": strings.ToUpper(in.Category)})
	})
	if err != nil {
		return nil, err
	}
	return s.GetExpense(ctx, t, id)
}

// GetExpense reads an expense in the caller's branch scope.
func (s *Service) GetExpense(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Expenses, false); err != nil {
		return nil, err
	}
	v, err := s.o.Store.GetExpense(ctx, t.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	branch, _ := v["branch_id"].(string)
	if !branchAllowed(t, branch) {
		return nil, apperrors.NotFound("Resource not found.", nil)
	}
	return v, nil
}

// ListExpenses lists expenses in the caller's branch scope.
func (s *Service) ListExpenses(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Expenses, false); err != nil {
		return nil, err
	}
	return s.o.Store.ListExpenses(ctx, t.OrganizationID, t.PermittedBranchIDs)
}

// UpdateExpense rewrites a draft expense at the expected version.
func (s *Service) UpdateExpense(ctx context.Context, t tenant.Context, id string, in models.ExpenseInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Expenses, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(in.ExpectedVersion); err != nil {
		return nil, err
	}
	currency, err := models.NormalizeCurrency(in.Currency)
	if err != nil {
		return nil, err
	}
	if in.AmountMinor <= 0 {
		return nil, apperrors.Validation("Expense amount must be positive.", nil)
	}
	if err = s.validateMedia(ctx, t, in.ReceiptMediaID); err != nil {
		return nil, err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		state, err := s.o.Store.ExpenseState(ctx, tx, t.OrganizationID, id, true)
		if err != nil {
			return err
		}
		if !branchAllowed(t, state.Branch) {
			return apperrors.NotFound("Resource not found.", nil)
		}
		if state.Version != in.ExpectedVersion {
			return apperrors.Conflict("Expense version changed.", nil)
		}
		if state.Status != "DRAFT" {
			return apperrors.Conflict("Approved or submitted expenses are immutable.", nil)
		}
		nv, err := s.o.Store.UpdateExpense(ctx, tx, t.OrganizationID, id, in, currency)
		if err != nil {
			return err
		}
		return s.record(ctx, tx, t, "expense.updated.v1", "expense", id, nv, map[string]any{"expenseId": id, "currency": currency, "amountMinor": in.AmountMinor})
	})
	if err != nil {
		return nil, err
	}
	return s.GetExpense(ctx, t, id)
}

// ExpenseCommand submits, approves, rejects or reverses an expense.
func (s *Service) ExpenseCommand(ctx context.Context, t tenant.Context, id, command, reason string, expected int64) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Expenses, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(expected); err != nil {
		return nil, err
	}
	if command == "reverse" {
		return s.reverseExpense(ctx, t, id, reason, expected)
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		state, err := s.o.Store.ExpenseState(ctx, tx, t.OrganizationID, id, true)
		if err != nil {
			return err
		}
		if state.Version != expected {
			return apperrors.Conflict("Expense version changed.", nil)
		}
		next, err := models.NextExpenseStatus(state.Status, command, state.Submitter, t.ActorID, reason)
		if err != nil {
			return err
		}
		nv, err := s.o.Store.SetExpenseStatus(ctx, tx, t.OrganizationID, id, next, t.ActorID)
		if err != nil {
			return err
		}
		snapshot, _ := json.Marshal(map[string]any{"expenseId": id, "status": next, "currency": state.Currency, "amountMinor": state.Amount, "version": nv})
		if err := s.o.Store.InsertExpenseHistory(ctx, tx, t.OrganizationID, id, state.Status, next, snapshot, t.ActorID, reason, nv); err != nil {
			return err
		}
		if command == "approve" && s.o.Flags.Posting && s.o.Flags.Workers {
			if _, err := s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: t.OrganizationID, JobType: jobExpensePosting, DedupeKey: id, Payload: map[string]string{"organizationId": t.OrganizationID, "expenseId": id}, MaxAttempts: 8}); err != nil {
				return err
			}
		}
		return s.record(ctx, tx, t, "expense."+command+"d.v1", "expense", id, nv, map[string]any{"expenseId": id, "status": next, "currency": state.Currency, "amountMinor": state.Amount})
	})
	if err != nil {
		return nil, err
	}
	return s.GetExpense(ctx, t, id)
}

func (s *Service) reverseExpense(ctx context.Context, t tenant.Context, id, reason string, expected int64) (map[string]any, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, apperrors.Validation("Reversal reason is required.", nil)
	}
	state, err := s.o.Store.ExpenseState(ctx, nil, t.OrganizationID, id, false)
	if err != nil {
		return nil, err
	}
	if state.Version != expected || (state.Status != "APPROVED" && state.Status != "POSTED") {
		return nil, apperrors.Conflict("Expense changed or is not approved or posted.", nil)
	}
	postingOperationID := ""
	if state.Status == "POSTED" {
		payload, _ := json.Marshal(map[string]any{"reason": reason, "originalPostingReference": id})
		req := models.PostingRequest{OrganizationID: t.OrganizationID, RecordType: "EXPENSE", RecordID: id, OperationType: "EXPENSE_REVERSAL", IdempotencyKey: fmt.Sprintf("expense-reversal:%s:%d", id, expected), AmountMinor: state.Amount, Currency: state.Currency, ExpectedVersion: expected, Payload: payload}
		opID, confirmed, err := s.createPosting(ctx, t, req)
		if err != nil {
			return nil, err
		}
		postingOperationID = opID
		if !confirmed {
			if _, err = s.executePosting(ctx, t, opID, req); err != nil {
				return nil, apperrors.Unavailable("Expense reversal requires authoritative reconciliation.", err)
			}
		}
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		current, err := s.o.Store.ExpenseState(ctx, tx, t.OrganizationID, id, true)
		if err != nil {
			return err
		}
		if current.Version != expected || current.Status != state.Status {
			return apperrors.Conflict("Expense changed while its reversal was being confirmed.", nil)
		}
		nv, err := s.o.Store.MarkExpenseReversed(ctx, tx, t.OrganizationID, id)
		if err != nil {
			return err
		}
		if err := s.o.Store.InsertExpenseReversal(ctx, tx, t.OrganizationID, id, state.Amount, state.Currency, reason, postingOperationID, t.ActorID); err != nil {
			return err
		}
		snapshot, _ := json.Marshal(map[string]any{"expenseId": id, "status": "REVERSED", "currency": state.Currency, "amountMinor": state.Amount, "version": nv, "postingOperationId": postingOperationID})
		if err := s.o.Store.InsertExpenseHistory(ctx, tx, t.OrganizationID, id, state.Status, "REVERSED", snapshot, t.ActorID, reason, nv); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "expense.reversed.v1", "expense", id, nv, map[string]any{"expenseId": id, "currency": state.Currency, "amountMinor": state.Amount, "postingOperationId": postingOperationID})
	})
	if err != nil {
		return nil, err
	}
	return s.GetExpense(ctx, t, id)
}

// CreateBudget records a draft budget.
func (s *Service) CreateBudget(ctx context.Context, t tenant.Context, in models.BudgetInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Budgets, true); err != nil {
		return nil, err
	}
	currency, err := models.NormalizeCurrency(in.Currency)
	if err != nil {
		return nil, err
	}
	if in.PeriodStart.IsZero() || in.PeriodEnd.Before(in.PeriodStart) || in.ApprovedAmountMinor < 0 || in.CostCentre == "" || in.Category == "" {
		return nil, apperrors.Validation("Budget period, scope and amount are invalid.", nil)
	}
	id := ids.New("bdg")
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := s.o.Store.InsertBudget(ctx, tx, id, t.OrganizationID, in, currency); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "budget.created.v1", "budget", id, 1, map[string]any{"budgetId": id, "currency": currency, "approvedAmountMinor": in.ApprovedAmountMinor})
	})
	if err != nil {
		return nil, err
	}
	return s.GetBudget(ctx, t, id)
}

// GetBudget reads one budget.
func (s *Service) GetBudget(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Budgets, false); err != nil {
		return nil, err
	}
	return s.o.Store.GetBudget(ctx, t.OrganizationID, id)
}

// ListBudgets lists the organization's budgets.
func (s *Service) ListBudgets(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Budgets, false); err != nil {
		return nil, err
	}
	return s.o.Store.ListBudgets(ctx, t.OrganizationID)
}

// UpdateBudget rewrites a draft budget at the expected version.
func (s *Service) UpdateBudget(ctx context.Context, t tenant.Context, id string, in models.BudgetInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Budgets, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(in.ExpectedVersion); err != nil {
		return nil, err
	}
	currency, err := models.NormalizeCurrency(in.Currency)
	if err != nil {
		return nil, err
	}
	changed, err := s.o.Store.UpdateBudget(ctx, t.OrganizationID, id, in, currency)
	if err != nil {
		return nil, err
	}
	if !changed {
		return nil, apperrors.Conflict("Budget changed or is immutable.", nil)
	}
	return s.GetBudget(ctx, t, id)
}

// BudgetCommand approves or closes a budget.
func (s *Service) BudgetCommand(ctx context.Context, t tenant.Context, id, command string, expected int64) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Budgets, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(expected); err != nil {
		return nil, err
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		status, version, err := s.o.Store.LockBudget(ctx, tx, t.OrganizationID, id)
		if err != nil {
			return err
		}
		if version != expected {
			return apperrors.Conflict("Budget version changed.", nil)
		}
		next, err := models.NextBudgetStatus(status, command)
		if err != nil {
			return err
		}
		nv, err := s.o.Store.SetBudgetStatus(ctx, tx, t.OrganizationID, id, next, t.ActorID)
		if err != nil {
			return err
		}
		snapshot, _ := json.Marshal(map[string]any{"budgetId": id, "status": next, "version": nv})
		if err := s.o.Store.InsertBudgetHistory(ctx, tx, t.OrganizationID, id, status, next, snapshot, t.ActorID, nv); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "budget."+command+"d.v1", "budget", id, nv, map[string]any{"budgetId": id, "status": next})
	})
	if err != nil {
		return nil, err
	}
	return s.GetBudget(ctx, t, id)
}

// CreateClaim records a claim.
func (s *Service) CreateClaim(ctx context.Context, t tenant.Context, in models.ClaimInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Claims, true); err != nil {
		return nil, err
	}
	currency, err := models.NormalizeCurrency(in.Currency)
	if err != nil {
		return nil, err
	}
	in.ClaimType = strings.ToUpper(in.ClaimType)
	if !models.ValidClaimType(in.ClaimType) {
		return nil, apperrors.Validation("Claim type is invalid.", nil)
	}
	for _, fileID := range in.EvidenceMediaIDs {
		if err = s.validateMedia(ctx, t, fileID); err != nil {
			return nil, err
		}
	}
	if in.Status == "" {
		in.Status = "DRAFT"
	}
	evidence, _ := json.Marshal(in.EvidenceMediaIDs)
	id := ids.New("clm")
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := s.o.Store.InsertClaim(ctx, tx, id, t.OrganizationID, in, currency, strings.ToUpper(in.Status), evidence); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "claim.created.v1", "claim", id, 1, map[string]any{"claimId": id, "claimType": in.ClaimType, "currency": currency, "amountReferenceMinor": in.AmountReferenceMinor})
	})
	if err != nil {
		return nil, err
	}
	return s.GetClaim(ctx, t, id)
}

// GetClaim reads one claim.
func (s *Service) GetClaim(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Claims, false); err != nil {
		return nil, err
	}
	return s.o.Store.GetClaim(ctx, t.OrganizationID, id)
}

// ListClaims lists the organization's claims.
func (s *Service) ListClaims(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Claims, false); err != nil {
		return nil, err
	}
	return s.o.Store.ListClaims(ctx, t.OrganizationID)
}

// UpdateClaim updates a non-final claim at the expected version.
func (s *Service) UpdateClaim(ctx context.Context, t tenant.Context, id string, in models.ClaimInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Claims, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(in.ExpectedVersion); err != nil {
		return nil, err
	}
	for _, fileID := range in.EvidenceMediaIDs {
		if err := s.validateMedia(ctx, t, fileID); err != nil {
			return nil, err
		}
	}
	evidence, _ := json.Marshal(in.EvidenceMediaIDs)
	status := strings.ToUpper(in.Status)
	if status == "" {
		status = "DRAFT"
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		old, version, err := s.o.Store.LockClaim(ctx, tx, t.OrganizationID, id)
		if err != nil {
			return err
		}
		if version != in.ExpectedVersion {
			return apperrors.Conflict("Claim version changed.", nil)
		}
		if models.ClaimFinal(old) {
			return apperrors.Conflict("Final claim records are immutable.", nil)
		}
		nv, err := s.o.Store.UpdateClaim(ctx, tx, t.OrganizationID, id, status, in, evidence)
		if err != nil {
			return err
		}
		snapshot, _ := json.Marshal(map[string]any{"claimId": id, "status": status, "version": nv})
		if err := s.o.Store.InsertClaimHistory(ctx, tx, t.OrganizationID, id, old, status, snapshot, t.ActorID); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "claim.updated.v1", "claim", id, nv, map[string]any{"claimId": id, "status": status, "externalClaimReference": in.ExternalClaimReference})
	})
	if err != nil {
		return nil, err
	}
	return s.GetClaim(ctx, t, id)
}
