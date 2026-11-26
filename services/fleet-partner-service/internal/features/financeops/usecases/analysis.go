package usecases

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// AnalysisStore is the persistence of the Section 22/52 analytical finance
// views. repositories.Repository satisfies it.
type AnalysisStore interface {
	AgeingRows(ctx context.Context, organizationID string, branches []string, asOf time.Time) ([]models.AgeingRow, error)
	ProfitabilityFacts(ctx context.Context, organizationID, dimension string, branches []string, from, to time.Time) ([]models.ProfitabilityFact, error)
	InsertProfitabilitySnapshot(ctx context.Context, q database.Querier, organizationID, setID, dimension string, from, to time.Time, l models.ProfitabilityLine, actor string) error
	ListProfitabilitySnapshots(ctx context.Context, organizationID string) ([]map[string]any, error)
	ExpenseForAllocation(ctx context.Context, q database.Querier, organizationID, id string) (int64, string, string, string, error)
	AllocationSetFor(ctx context.Context, q database.Querier, organizationID, sourceType, sourceID string) (string, error)
	InsertCostAllocation(ctx context.Context, q database.Querier, organizationID, setID string, in models.CostAllocationInput, target models.CostAllocationTarget, currency, actor string) error
	ListCostAllocations(ctx context.Context, organizationID string) ([]map[string]any, error)
	AllocationTargetExists(ctx context.Context, q database.Querier, organizationID, targetType, targetID string) (bool, error)
}

func (s *Service) analysis() (AnalysisStore, error) {
	a, ok := s.o.Store.(AnalysisStore)
	if !ok {
		return nil, apperrors.Unavailable("Finance analysis is unavailable.", nil)
	}
	return a, nil
}

// ReceivablesAgeing reports open receivables by currency, age band and
// customer as of now. Receivables are company accounting records; nothing
// here reads or changes wallet balances.
func (s *Service) ReceivablesAgeing(ctx context.Context, t tenant.Context) (models.ReceivablesAgeing, error) {
	if err := s.gate(s.o.Flags.Receivables, false); err != nil {
		return models.ReceivablesAgeing{}, err
	}
	a, err := s.analysis()
	if err != nil {
		return models.ReceivablesAgeing{}, err
	}
	asOf := s.o.Now()
	rows, err := a.AgeingRows(ctx, t.OrganizationID, t.PermittedBranchIDs, asOf)
	if err != nil {
		return models.ReceivablesAgeing{}, err
	}
	return models.BuildAgeing(asOf, rows), nil
}

func parsePeriod(from, to string, now time.Time) (time.Time, time.Time, error) {
	end := now.UTC().Truncate(24 * time.Hour)
	start := end.AddDate(0, 0, -30)
	var err error
	if strings.TrimSpace(from) != "" {
		if start, err = time.Parse("2006-01-02", from); err != nil {
			return start, end, apperrors.Validation("from must be a YYYY-MM-DD date.", []apperrors.FieldViolation{{Field: "from", Message: "invalid date"}})
		}
	}
	if strings.TrimSpace(to) != "" {
		if end, err = time.Parse("2006-01-02", to); err != nil {
			return start, end, apperrors.Validation("to must be a YYYY-MM-DD date.", []apperrors.FieldViolation{{Field: "to", Message: "invalid date"}})
		}
	}
	if end.Before(start) {
		return start, end, apperrors.Validation("to must not be before from.", []apperrors.FieldViolation{{Field: "to", Message: "before from"}})
	}
	if end.Sub(start) > 366*24*time.Hour {
		return start, end, apperrors.Validation("A profitability period is at most one year.", []apperrors.FieldViolation{{Field: "to", Message: "period too long"}})
	}
	return start, end, nil
}

// Profitability computes the analytical profitability of one dimension over
// a period, per dimension value and currency, from issued invoices, credit
// notes, approved expenses, approved compensation and cost allocations. It
// separates revenue, direct cost and allocated cost (spec 52.2); it is an
// analysis, not a ledger, and never aggregates across currencies.
func (s *Service) Profitability(ctx context.Context, t tenant.Context, in models.ProfitabilityInput) (models.ProfitabilityReport, error) {
	if err := s.gate(s.o.Flags.Invoices || s.o.Flags.Expenses || s.o.Flags.Payroll, false); err != nil {
		return models.ProfitabilityReport{}, err
	}
	a, err := s.analysis()
	if err != nil {
		return models.ProfitabilityReport{}, err
	}
	dimension := strings.ToUpper(strings.TrimSpace(in.Dimension))
	if dimension == "" {
		dimension = "BRANCH"
	}
	if !models.ProfitabilityDimensions[dimension] {
		return models.ProfitabilityReport{}, apperrors.Validation("dimension must be CUSTOMER, BRANCH, PROVIDER, ASSET or COST_CENTRE.", []apperrors.FieldViolation{{Field: "dimension", Message: "unsupported"}})
	}
	from, to, err := parsePeriod(in.From, in.To, s.o.Now())
	if err != nil {
		return models.ProfitabilityReport{}, err
	}
	facts, err := a.ProfitabilityFacts(ctx, t.OrganizationID, dimension, t.PermittedBranchIDs, from, to)
	if err != nil {
		return models.ProfitabilityReport{}, err
	}
	return models.ProfitabilityReport{
		Dimension: dimension, PeriodStart: from.Format("2006-01-02"), PeriodEnd: to.Format("2006-01-02"),
		ModelVersion: models.ProfitabilityModelVersion,
		Method:       "revenue = issued invoices - credit notes; direct cost = approved expenses without an allocation set + approved compensation; allocated cost = cost allocations; margin = revenue - direct - allocated",
		Lines:        models.BuildProfitability(facts),
	}, nil
}

// SnapshotProfitability persists the current analysis as an append-only,
// explainable snapshot (model version and input counts retained).
func (s *Service) SnapshotProfitability(ctx context.Context, t tenant.Context, in models.ProfitabilityInput) (models.ProfitabilityReport, error) {
	if s.o.Flags.KillSwitch {
		return models.ProfitabilityReport{}, apperrors.Unavailable("Fleet integrations are disabled.", nil)
	}
	report, err := s.Profitability(ctx, t, in)
	if err != nil {
		return report, err
	}
	a, _ := s.analysis()
	from, _ := time.Parse("2006-01-02", report.PeriodStart)
	to, _ := time.Parse("2006-01-02", report.PeriodEnd)
	report.SnapshotSetID = ids.New("pfset")
	err = s.tx(ctx, func(tx pgx.Tx) error {
		for _, l := range report.Lines {
			if err := a.InsertProfitabilitySnapshot(ctx, tx, t.OrganizationID, report.SnapshotSetID, report.Dimension, from, to, l, t.ActorID); err != nil {
				return err
			}
		}
		return s.record(ctx, tx, t, "profitability.generated.v1", "profitability_snapshot", report.SnapshotSetID, 1, map[string]any{"snapshotSetId": report.SnapshotSetID, "dimension": report.Dimension, "periodStart": report.PeriodStart, "periodEnd": report.PeriodEnd, "lineCount": len(report.Lines), "modelVersion": report.ModelVersion})
	})
	return report, err
}

// ListProfitabilitySnapshots lists generated snapshot lines.
func (s *Service) ListProfitabilitySnapshots(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Invoices || s.o.Flags.Expenses || s.o.Flags.Payroll, false); err != nil {
		return nil, err
	}
	a, err := s.analysis()
	if err != nil {
		return nil, err
	}
	return a.ListProfitabilitySnapshots(ctx, t.OrganizationID)
}

// AllocateCost splits one approved expense across cost targets. The parts
// must sum exactly to the expense amount, in the expense currency; a source
// has at most one allocation set and allocations are append-only.
func (s *Service) AllocateCost(ctx context.Context, t tenant.Context, in models.CostAllocationInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Expenses, true); err != nil {
		return nil, err
	}
	a, err := s.analysis()
	if err != nil {
		return nil, err
	}
	in.SourceType = strings.ToUpper(strings.TrimSpace(in.SourceType))
	in.Method = strings.ToUpper(strings.TrimSpace(in.Method))
	if in.SourceType == "" {
		in.SourceType = "EXPENSE"
	}
	if in.SourceType != "EXPENSE" || in.SourceID == "" || len(in.Targets) == 0 || strings.TrimSpace(in.Reason) == "" {
		return nil, apperrors.Validation("An EXPENSE source, a reason and at least one target are required.", nil)
	}
	if in.Method != "MANUAL" && in.Method != "EVEN" {
		return nil, apperrors.Validation("method must be MANUAL or EVEN.", []apperrors.FieldViolation{{Field: "method", Message: "unsupported"}})
	}
	if len(in.Targets) > 50 {
		return nil, apperrors.Validation("At most 50 allocation targets are allowed.", nil)
	}
	seen := map[string]bool{}
	for i := range in.Targets {
		in.Targets[i].TargetType = strings.ToUpper(strings.TrimSpace(in.Targets[i].TargetType))
		k := in.Targets[i].TargetType + ":" + in.Targets[i].TargetID
		if !models.CostAllocationTargetTypes[in.Targets[i].TargetType] || in.Targets[i].TargetID == "" || seen[k] {
			return nil, apperrors.Validation("Every target needs a supported type and a distinct id.", nil)
		}
		seen[k] = true
	}
	var setID string
	err = s.tx(ctx, func(tx pgx.Tx) error {
		amount, currency, status, branch, err := a.ExpenseForAllocation(ctx, tx, t.OrganizationID, in.SourceID)
		if err != nil {
			if err == pgx.ErrNoRows {
				return apperrors.NotFound("Expense not found.", nil)
			}
			return err
		}
		if branch != "" && !t.BranchPermitted(branch) {
			return apperrors.NotFound("Expense not found.", nil)
		}
		if status != "APPROVED" && status != "POSTING" && status != "POSTED" {
			return apperrors.New(http.StatusConflict, "state_transition_invalid", "Only an approved expense can be allocated.", nil)
		}
		existing, err := a.AllocationSetFor(ctx, tx, t.OrganizationID, in.SourceType, in.SourceID)
		if err != nil {
			return err
		}
		if existing != "" {
			return apperrors.New(http.StatusConflict, "duplicate_financial_source", "This expense is already allocated; reverse the expense to correct it.", nil)
		}
		if in.Method == "EVEN" {
			for i, part := range models.SplitEven(amount, len(in.Targets)) {
				in.Targets[i].AmountMinor = part
			}
		}
		var total int64
		for _, target := range in.Targets {
			if target.AmountMinor <= 0 {
				return apperrors.Validation("Every allocated amount must be positive.", nil)
			}
			total += target.AmountMinor
			ok, err := a.AllocationTargetExists(ctx, tx, t.OrganizationID, target.TargetType, target.TargetID)
			if err != nil {
				return err
			}
			if !ok {
				return apperrors.NotFound("Allocation target not found.", nil)
			}
		}
		if total != amount {
			return apperrors.Validation("Allocated amounts must sum exactly to the expense amount.", []apperrors.FieldViolation{{Field: "targets", Message: "sum mismatch"}})
		}
		setID = ids.New("calset")
		for _, target := range in.Targets {
			if err := a.InsertCostAllocation(ctx, tx, t.OrganizationID, setID, in, target, currency, t.ActorID); err != nil {
				return err
			}
		}
		return s.record(ctx, tx, t, "cost_allocation.recorded.v1", "expense", in.SourceID, 1, map[string]any{"allocationSetId": setID, "expenseId": in.SourceID, "currency": currency, "amountMinor": amount, "targetCount": len(in.Targets), "method": in.Method})
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"allocationSetId": setID, "sourceType": in.SourceType, "sourceId": in.SourceID, "method": in.Method, "targets": in.Targets}, nil
}

// ListCostAllocations lists allocation rows.
func (s *Service) ListCostAllocations(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Expenses, false); err != nil {
		return nil, err
	}
	a, err := s.analysis()
	if err != nil {
		return nil, err
	}
	return a.ListCostAllocations(ctx, t.OrganizationID)
}
