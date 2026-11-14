package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"cosmicforge/logistics/shared/go/apperrors"
)

// CreateCompensationRule records a draft compensation rule and its first
// version snapshot.
func (s *Service) CreateCompensationRule(ctx context.Context, t tenant.Context, in models.CompensationRuleInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Compensation, true); err != nil {
		return nil, err
	}
	currency, err := models.NormalizeCurrency(in.Currency)
	if err != nil {
		return nil, err
	}
	in.RuleType = strings.ToUpper(in.RuleType)
	if !models.ValidRuleType(in.RuleType) || strings.TrimSpace(in.Name) == "" || in.EffectiveFrom.IsZero() {
		return nil, apperrors.Validation("Compensation rule is invalid.", nil)
	}
	cfg := models.RawOr(in.CalculationConfiguration, "{}")
	calc, err := models.ParseRuleCalculation(cfg)
	if err != nil {
		return nil, err
	}
	if in.RuleType == "DEDUCTION" && strings.TrimSpace(calc.ApprovedBasis) == "" {
		return nil, apperrors.Validation("Deduction rules require an approved basis.", nil)
	}
	roles := models.RawOr(in.ApplicableRoles, "[]")
	services := models.RawOr(in.ApplicableServices, "[]")
	id := ids.New("cpr")
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := s.o.Store.InsertCompensationRule(ctx, tx, id, t.OrganizationID, t.ActorID, in, roles, services, cfg, currency); err != nil {
			return err
		}
		snapshot, _ := json.Marshal(map[string]any{"id": id, "name": in.Name, "ruleType": in.RuleType, "configuration": json.RawMessage(cfg), "currency": currency, "version": 1})
		if err := s.o.Store.InsertRuleVersion(ctx, tx, t.OrganizationID, id, 1, snapshot, t.ActorID); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "compensation_rule.created.v1", "compensation_rule", id, 1, map[string]any{"ruleId": id, "ruleType": in.RuleType, "currency": currency})
	})
	if err != nil {
		return nil, err
	}
	return s.GetCompensationRule(ctx, t, id)
}

// GetCompensationRule reads one rule.
func (s *Service) GetCompensationRule(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Compensation, false); err != nil {
		return nil, err
	}
	return s.o.Store.GetCompensationRule(ctx, t.OrganizationID, id)
}

// ListCompensationRules lists the organization's rules.
func (s *Service) ListCompensationRules(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Compensation, false); err != nil {
		return nil, err
	}
	return s.o.Store.ListCompensationRules(ctx, t.OrganizationID)
}

// UpdateCompensationRule rewrites a draft rule at the expected version.
func (s *Service) UpdateCompensationRule(ctx context.Context, t tenant.Context, id string, in models.CompensationRuleInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Compensation, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(in.ExpectedVersion); err != nil {
		return nil, err
	}
	cfg := models.RawOr(in.CalculationConfiguration, "{}")
	if _, err := models.ParseRuleCalculation(cfg); err != nil {
		return nil, err
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		status, version, err := s.o.Store.LockCompensationRule(ctx, tx, t.OrganizationID, id)
		if err != nil {
			return err
		}
		if version != in.ExpectedVersion {
			return apperrors.Conflict("Compensation rule version changed.", nil)
		}
		if status == "APPROVED" || status == "ACTIVE" || status == "ARCHIVED" {
			return apperrors.Conflict("Approved compensation rules are immutable; create a replacement version.", nil)
		}
		nv, err := s.o.Store.UpdateCompensationRule(ctx, tx, t.OrganizationID, id, in, cfg)
		if err != nil {
			return err
		}
		snapshot, _ := json.Marshal(map[string]any{"id": id, "name": in.Name, "configuration": json.RawMessage(cfg), "version": nv})
		if err := s.o.Store.InsertRuleVersion(ctx, tx, t.OrganizationID, id, nv, snapshot, t.ActorID); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "compensation_rule.updated.v1", "compensation_rule", id, nv, map[string]any{"ruleId": id, "version": nv})
	})
	if err != nil {
		return nil, err
	}
	return s.GetCompensationRule(ctx, t, id)
}

// CompensationRuleCommand approves or deactivates a rule.
func (s *Service) CompensationRuleCommand(ctx context.Context, t tenant.Context, id, command string, expected int64) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Compensation, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(expected); err != nil {
		return nil, err
	}
	next := "APPROVED"
	if command == "deactivate" {
		next = "INACTIVE"
	}
	changed, err := s.o.Store.TransitionCompensationRule(ctx, t.OrganizationID, id, next, t.ActorID, expected)
	if err != nil {
		return nil, err
	}
	if !changed {
		return nil, apperrors.Conflict("Compensation rule changed or command is invalid.", nil)
	}
	return s.GetCompensationRule(ctx, t, id)
}

// CreatePayrollPeriod records a draft payroll period.
func (s *Service) CreatePayrollPeriod(ctx context.Context, t tenant.Context, in models.PayrollPeriodInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Payroll, true); err != nil {
		return nil, err
	}
	if !branchAllowed(t, in.BranchID) {
		return nil, apperrors.Forbidden("Branch access is denied.", nil)
	}
	currency, err := models.NormalizeCurrency(in.Currency)
	if err != nil {
		return nil, err
	}
	if in.PeriodStart.IsZero() || in.PeriodEnd.Before(in.PeriodStart) {
		return nil, apperrors.Validation("Payroll period is invalid.", nil)
	}
	id := ids.New("pay")
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := s.o.Store.InsertPayrollPeriod(ctx, tx, id, t.OrganizationID, t.ActorID, in, currency); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "payroll_period.created.v1", "payroll_period", id, 1, map[string]any{"payrollPeriodId": id, "currency": currency, "periodStart": in.PeriodStart, "periodEnd": in.PeriodEnd})
	})
	if err != nil {
		return nil, err
	}
	return s.GetPayrollPeriod(ctx, t, id)
}

// GetPayrollPeriod reads a payroll period with its items.
func (s *Service) GetPayrollPeriod(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Payroll, false); err != nil {
		return nil, err
	}
	return s.o.Store.GetPayrollPeriod(ctx, t.OrganizationID, id)
}

// ListPayrollPeriods lists payroll periods in the caller's branch scope.
func (s *Service) ListPayrollPeriods(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Payroll, false); err != nil {
		return nil, err
	}
	return s.o.Store.ListPayrollPeriods(ctx, t.OrganizationID, t.PermittedBranchIDs)
}

// CalculatePayroll computes compensation items from the active rules.
func (s *Service) CalculatePayroll(ctx context.Context, t tenant.Context, id string, expected int64) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Payroll, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(expected); err != nil {
		return nil, err
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		period, err := s.o.Store.PayrollState(ctx, tx, t.OrganizationID, id, true)
		if err != nil {
			return err
		}
		if period.Version != expected {
			return apperrors.Conflict("Payroll period version changed.", nil)
		}
		if models.PayrollFrozen(period.Status) {
			return apperrors.Conflict("Approved payroll periods are immutable.", nil)
		}
		calcVersion := period.CalculationVersion + 1
		active, err := s.o.Store.ActiveCompensationRules(ctx, tx, t.OrganizationID, period.Currency, period.Start, period.End)
		if err != nil {
			return err
		}
		// CMP-027. Components already paid with a completed job's own money
		// must not be paid a second time here. The two sides of that check are
		// computed by different code from different tables, so payroll reads
		// the durable record of what work settlement already paid and skips
		// exactly those components.
		//
		// The completed assignments and the exclusions are read once rather
		// than per rule: the query does not depend on the rule, and re-reading
		// it per rule only multiplies the work.
		var jobSources []models.SourceAssignment
		var workSettled map[string]bool
		jobSourcesLoaded := false
		loadJobSources := func() error {
			if jobSourcesLoaded {
				return nil
			}
			jobSources, err = s.o.Store.CompletedAssignments(ctx, tx, t.OrganizationID, period.Branch, period.Start, period.End)
			if err != nil {
				return err
			}
			assignmentIDs := make([]string, 0, len(jobSources))
			for _, item := range jobSources {
				assignmentIDs = append(assignmentIDs, item.Source)
			}
			// Matched on the assignment rather than a time window: a job
			// completed inside the period can be settled moments after the
			// period ends, and a windowed read would miss it and pay it again.
			workSettled, err = s.o.Store.WorkSettledComponents(ctx, tx, t.OrganizationID, assignmentIDs)
			if err != nil {
				return err
			}
			jobSourcesLoaded = true
			return nil
		}
		count := 0
		for _, rule := range active {
			if rule.Skipped() {
				continue
			}
			explanation := "Deterministic " + rule.Type + " rule version " + fmt.Sprint(rule.Version)
			if rule.JobLevel() {
				if err := loadJobSources(); err != nil {
					return err
				}
				for _, item := range jobSources {
					// Already paid with this job's own money. Paying it here
					// as well is the double payment CMP-027 exists to stop.
					if workSettled[item.Source+":"+rule.ID] {
						continue
					}
					inputs, _ := json.Marshal(map[string]any{"assignmentId": item.Source, "assignmentVersion": item.Version, "ruleVersion": rule.Version})
					n, err := s.o.Store.InsertCompensationItem(ctx, tx, models.CompensationItem{OrganizationID: t.OrganizationID, PeriodID: id, MembershipID: item.Member, SourceType: "COMPLETED_ASSIGNMENT", SourceReference: item.Source, RuleID: rule.ID, RuleVersion: rule.Version, CalculationVersion: calcVersion, AmountMinor: rule.Config.AmountMinor, Currency: period.Currency, ReasonCode: rule.Type, Explanation: explanation, SourceInputs: inputs})
					if err != nil {
						return err
					}
					count += int(n)
				}
				continue
			}
			memberIDs, err := s.o.Store.ActiveMemberships(ctx, tx, t.OrganizationID)
			if err != nil {
				return err
			}
			for _, member := range memberIDs {
				inputs, _ := json.Marshal(map[string]any{"periodStart": period.Start, "periodEnd": period.End, "ruleVersion": rule.Version})
				n, err := s.o.Store.InsertCompensationItem(ctx, tx, models.CompensationItem{OrganizationID: t.OrganizationID, PeriodID: id, MembershipID: member, SourceType: "PAYROLL_PERIOD", SourceReference: id, RuleID: rule.ID, RuleVersion: rule.Version, CalculationVersion: calcVersion, AmountMinor: rule.Config.AmountMinor, Currency: period.Currency, ReasonCode: rule.Type, Explanation: explanation, SourceInputs: inputs})
				if err != nil {
					return err
				}
				count += int(n)
			}
		}
		nv, err := s.o.Store.MarkPayrollCalculated(ctx, tx, t.OrganizationID, id, calcVersion)
		if err != nil {
			return err
		}
		snapshot, _ := json.Marshal(map[string]any{"payrollPeriodId": id, "status": "CALCULATED", "calculationVersion": calcVersion, "itemCount": count})
		if err := s.o.Store.InsertPayrollHistory(ctx, tx, t.OrganizationID, id, period.Status, "CALCULATED", calcVersion, snapshot, t.ActorID, ""); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "payroll_period.calculated.v1", "payroll_period", id, nv, map[string]any{"payrollPeriodId": id, "calculationVersion": calcVersion, "itemCount": count, "currency": period.Currency})
	})
	if err != nil {
		return nil, err
	}
	return s.GetPayrollPeriod(ctx, t, id)
}

// ApprovePayroll approves a calculated period; its creator may not.
func (s *Service) ApprovePayroll(ctx context.Context, t tenant.Context, id string, expected int64) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Payroll, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(expected); err != nil {
		return nil, err
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		period, err := s.o.Store.PayrollState(ctx, tx, t.OrganizationID, id, true)
		if err != nil {
			return err
		}
		if period.Version != expected || period.Status != "CALCULATED" {
			return apperrors.Conflict("Payroll period changed or is not calculated.", nil)
		}
		if period.Creator == t.ActorID {
			return apperrors.Forbidden("Payroll creator cannot approve the same period.", nil)
		}
		nv, err := s.o.Store.ApprovePayrollPeriod(ctx, tx, t.OrganizationID, id, t.ActorID)
		if err != nil {
			return err
		}
		if err := s.o.Store.ApproveCompensationItems(ctx, tx, t.OrganizationID, id, period.CalculationVersion); err != nil {
			return err
		}
		snapshot, _ := json.Marshal(map[string]any{"payrollPeriodId": id, "status": "APPROVED", "calculationVersion": period.CalculationVersion})
		if err := s.o.Store.InsertPayrollHistory(ctx, tx, t.OrganizationID, id, period.Status, "APPROVED", period.CalculationVersion, snapshot, t.ActorID, ""); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "payroll_period.approved.v1", "payroll_period", id, nv, map[string]any{"payrollPeriodId": id, "calculationVersion": period.CalculationVersion})
	})
	if err != nil {
		return nil, err
	}
	return s.GetPayrollPeriod(ctx, t, id)
}

// createPosting records one durable posting operation per idempotency key.
// It returns the operation ID and whether it is already confirmed; a key
// reused for a different request is a conflict.
func (s *Service) createPosting(ctx context.Context, t tenant.Context, req models.PostingRequest) (string, bool, error) {
	operationID := ids.New("fpo")
	fp := models.Fingerprint(req)
	req.OrganizationID = t.OrganizationID
	inserted, err := s.o.Store.InsertPostingOperation(ctx, operationID, req, fp)
	if err != nil {
		return "", false, err
	}
	if !inserted {
		storedID, status, storedFingerprint, err := s.o.Store.PostingOperationByKey(ctx, t.OrganizationID, req.OperationType, req.IdempotencyKey)
		if err != nil {
			return "", false, err
		}
		if storedFingerprint != fp {
			return "", false, apperrors.Conflict("Posting idempotency key was already used for a different request.", nil)
		}
		return storedID, status == "CONFIRMED", nil
	}
	return operationID, false, nil
}

func (s *Service) executePosting(ctx context.Context, t tenant.Context, operationID string, req models.PostingRequest) (models.PostingOutcome, error) {
	if !s.o.Flags.Posting || s.o.Wallet == nil || !s.o.Wallet.Configured(req.OperationType) {
		return models.PostingOutcome{}, apperrors.Unavailable("The authoritative financial posting adapter is unavailable.", nil)
	}
	start := s.o.Now().UTC()
	out, err := s.o.Wallet.Submit(ctx, operationMeta(t, req.IdempotencyKey), req)
	outcome := "SUCCESS"
	status := "CONFIRMED"
	code := ""
	if err != nil {
		outcome = "FAILED"
		status = "RETRY_SCHEDULED"
		code = "wallet_unavailable"
		var ce *platformclients.Error
		if errors.As(err, &ce) && ce.Outcome == platformclients.OutcomeUnknown {
			outcome = "OUTCOME_UNKNOWN"
			status = "OUTCOME_UNKNOWN"
			read, readErr := s.o.Wallet.ReadByKey(ctx, operationMeta(t, req.IdempotencyKey), req)
			if readErr == nil {
				out = read
				err = nil
				outcome = "READ_BACK_CONFIRMED"
				status = "CONFIRMED"
				code = ""
			}
		}
	}
	if err == nil && !models.PostingConfirmed(out.Status) {
		err = fmt.Errorf("authoritative posting status %q is not confirmed", out.Status)
		outcome = "OUTCOME_UNKNOWN"
		status = "OUTCOME_UNKNOWN"
		code = "authoritative_confirmation_pending"
	}
	if txErr := s.recordPostingAttempt(ctx, t, operationID, start, outcome, status, out.Reference, out.SourceVersion, code); txErr != nil {
		return out, txErr
	}
	return out, err
}

// recordPostingAttempt appends one attempt row and moves the operation to the
// status that attempt established, in one transaction. An operation left
// OUTCOME_UNKNOWN or RETRY_SCHEDULED is rescheduled thirty seconds out.
func (s *Service) recordPostingAttempt(ctx context.Context, t tenant.Context, operationID string, start time.Time, outcome, status, reference string, sourceVersion int64, code string) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		return s.o.Store.RecordPostingAttempt(ctx, tx, models.PostingAttemptRecord{OrganizationID: t.OrganizationID, OperationID: operationID, Outcome: outcome, Status: status, Reference: reference, SourceVersion: sourceVersion, ErrorCode: code, Started: start})
	})
}

// PostPayroll posts every approved item of an approved period to the money
// authority, one idempotent posting per item.
func (s *Service) PostPayroll(ctx context.Context, t tenant.Context, id, key string, expected int64) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Payroll, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(expected); err != nil {
		return nil, err
	}
	if strings.TrimSpace(key) == "" {
		return nil, apperrors.Validation("Posting idempotency key is required.", nil)
	}
	period, err := s.o.Store.PayrollState(ctx, nil, t.OrganizationID, id, false)
	if err != nil {
		return nil, err
	}
	if period.Status != "APPROVED" || period.Version != expected {
		return nil, apperrors.Conflict("Payroll period changed or is not approved.", nil)
	}
	currency := period.Currency
	items, err := s.o.Store.PayrollItems(ctx, t.OrganizationID, id, []string{"APPROVED", "POSTED"})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, apperrors.Conflict("Payroll has no approved compensation items.", nil)
	}
	total := int64(0)
	lastOperationID := ""
	for _, x := range items {
		total += x.Amount
		if x.SubwalletID == "" || x.ProviderID == "" {
			return nil, apperrors.Unavailable("Provider sub-wallet provisioning is incomplete.", nil)
		}
		itemStatus, err := s.o.Store.CompensationItemStatus(ctx, t.OrganizationID, x.ID)
		if err != nil {
			return nil, err
		}
		if itemStatus == "POSTED" {
			continue
		}
		allocationType := models.PayrollAllocationType(x.Reason)
		payload, _ := json.Marshal(map[string]any{"earningAccountId": x.SubwalletID, "organizationMembershipId": x.MembershipID, "providerId": x.ProviderID, "allocationType": allocationType, "businessReferenceType": "PAYROLL_REFERENCE", "businessReferenceId": id + ":" + x.ID, "amountMinor": x.Amount, "currency": currency, "reasonCode": x.Reason, "providerExplanation": "Approved payroll compensation item " + x.ID, "approvalReference": id, "sourceService": "fleet-partner-service", "sourceReference": x.ID})
		req := models.PostingRequest{OrganizationID: t.OrganizationID, RecordType: "PAYROLL_ITEM", RecordID: x.ID, OperationType: "PROVIDER_ALLOCATION", IdempotencyKey: key + ":" + x.ID, AmountMinor: x.Amount, Currency: currency, Payload: payload}
		opID, confirmed, createErr := s.createPosting(ctx, t, req)
		if createErr != nil {
			return nil, createErr
		}
		lastOperationID = opID
		if !confirmed {
			out, execErr := s.executePosting(ctx, t, opID, req)
			if execErr != nil {
				_ = s.o.Store.MarkPayrollPartiallyPosted(ctx, t.OrganizationID, id)
				return nil, apperrors.Unavailable("Payroll posting requires reconciliation.", execErr)
			}
			if out.Status != "" && out.Status != "CONFIRMED" && out.Status != "POSTED" && out.Status != "SUCCEEDED" {
				return nil, apperrors.Unavailable("Wallet has not confirmed payroll posting.", nil)
			}
		}
		if err := s.o.Store.MarkCompensationItemPosted(ctx, t.OrganizationID, x.ID); err != nil {
			return nil, err
		}
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		nv, posted, err := s.o.Store.MarkPayrollPosted(ctx, tx, t.OrganizationID, id, lastOperationID)
		if err != nil || !posted {
			return err
		}
		return s.record(ctx, tx, t, "payroll_period.posted.v1", "payroll_period", id, nv, map[string]any{"payrollPeriodId": id, "currency": currency, "amountMinor": total, "postingOperationId": lastOperationID, "itemCount": len(items)})
	})
	if err != nil {
		return nil, err
	}
	return s.GetPayrollPeriod(ctx, t, id)
}

// ReversePayroll reverses every posted item of a posted period.
func (s *Service) ReversePayroll(ctx context.Context, t tenant.Context, id, reason string, expected int64) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Payroll, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(expected); err != nil {
		return nil, err
	}
	if strings.TrimSpace(reason) == "" {
		return nil, apperrors.Validation("Reversal reason is required.", nil)
	}
	period, err := s.o.Store.PayrollState(ctx, nil, t.OrganizationID, id, false)
	if err != nil {
		return nil, err
	}
	if period.Status != "POSTED" || period.Version != expected {
		return nil, apperrors.Conflict("Payroll period changed or is not posted.", nil)
	}
	currency := period.Currency
	items, err := s.o.Store.PayrollItems(ctx, t.OrganizationID, id, []string{"POSTED"})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, apperrors.Conflict("Posted payroll has no compensation items to reverse.", nil)
	}
	for _, x := range items {
		if x.ProviderID == "" || x.SubwalletID == "" || x.Amount <= 0 {
			return nil, apperrors.Unavailable("Provider sub-wallet reversal data is incomplete.", nil)
		}
		payload, _ := json.Marshal(map[string]any{"earningAccountId": x.SubwalletID, "organizationMembershipId": x.MembershipID, "providerId": x.ProviderID, "allocationType": "REVERSAL", "businessReferenceType": "PAYROLL_REFERENCE", "businessReferenceId": id + ":" + x.ID + ":reversal", "amountMinor": x.Amount, "currency": currency, "reasonCode": "PAYROLL_REVERSAL", "providerExplanation": reason, "approvalReference": id, "sourceService": "fleet-partner-service", "sourceReference": x.ID})
		req := models.PostingRequest{OrganizationID: t.OrganizationID, RecordType: "PAYROLL_ITEM", RecordID: x.ID, OperationType: "PAYROLL_REVERSAL", IdempotencyKey: fmt.Sprintf("payroll-reversal:%s:%d:%s", id, expected, x.ID), AmountMinor: x.Amount, Currency: currency, Payload: payload}
		opID, confirmed, err := s.createPosting(ctx, t, req)
		if err != nil {
			return nil, err
		}
		if !confirmed {
			if _, err = s.executePosting(ctx, t, opID, req); err != nil {
				return nil, apperrors.Unavailable("Payroll reversal requires authoritative reconciliation.", err)
			}
		}
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		nv, err := s.o.Store.MarkPayrollReversed(ctx, tx, t.OrganizationID, id, expected)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperrors.Conflict("Payroll period changed while its reversals were being confirmed.", nil)
			}
			return err
		}
		if err := s.o.Store.ReverseCompensationItems(ctx, tx, t.OrganizationID, id); err != nil {
			return err
		}
		snapshot, _ := json.Marshal(map[string]any{"payrollPeriodId": id, "status": "REVERSED", "calculationVersion": period.CalculationVersion, "itemCount": len(items), "version": nv})
		if err := s.o.Store.InsertPayrollHistory(ctx, tx, t.OrganizationID, id, "POSTED", "REVERSED", period.CalculationVersion, snapshot, t.ActorID, reason); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "payroll_period.reversed.v1", "payroll_period", id, nv, map[string]any{"payrollPeriodId": id, "currency": currency, "itemCount": len(items)})
	})
	if err != nil {
		return nil, err
	}
	return s.GetPayrollPeriod(ctx, t, id)
}

// ProviderStatements returns a provider membership's payroll statement.
func (s *Service) ProviderStatements(ctx context.Context, t tenant.Context, membershipID string) (models.Statement, error) {
	if err := s.gate(s.o.Flags.Payroll, false); err != nil {
		return models.Statement{}, err
	}
	belongs, err := s.o.Store.MembershipExists(ctx, t.OrganizationID, membershipID)
	if err != nil {
		return models.Statement{}, err
	}
	if !belongs {
		return models.Statement{}, apperrors.NotFound("Resource not found.", nil)
	}
	periods, err := s.o.Store.ProviderStatementPeriods(ctx, t.OrganizationID, membershipID)
	if err != nil {
		return models.Statement{}, err
	}
	return models.Statement{MembershipID: membershipID, Periods: periods}, nil
}

// CreateSettlement records a subcontractor settlement over completed jobs; a
// job is settled at most once.
func (s *Service) CreateSettlement(ctx context.Context, t tenant.Context, in models.SettlementInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Settlements, true); err != nil {
		return nil, err
	}
	currency, err := models.NormalizeCurrency(in.Currency)
	if err != nil {
		return nil, err
	}
	if in.SupplierReference == "" || in.ProviderOrCarrierReference == "" || len(in.JobReferences) == 0 || in.AmountMinor < 0 || in.PeriodEnd.Before(in.PeriodStart) {
		return nil, apperrors.Validation("Settlement scope, jobs, period and amount are required.", nil)
	}
	id := ids.New("sts")
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := s.o.Store.InsertSettlement(ctx, tx, id, t.OrganizationID, in, currency, t.ActorID); err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, job := range in.JobReferences {
			job = strings.TrimSpace(job)
			if job == "" || seen[job] {
				return apperrors.Validation("Settlement job references must be unique and non-empty.", nil)
			}
			seen[job] = true
			belongs, err := s.o.Store.CompletedAssignmentExists(ctx, tx, t.OrganizationID, job)
			if err != nil {
				return err
			}
			if !belongs {
				return apperrors.Validation("Settlement job does not belong to this organization or is incomplete.", nil)
			}
			if err := s.o.Store.InsertSettlementJob(ctx, tx, id, t.OrganizationID, job); err != nil {
				return apperrors.Conflict("A job cannot be settled twice.", err)
			}
		}
		return s.record(ctx, tx, t, "subcontractor_settlement.created.v1", "subcontractor_settlement", id, 1, map[string]any{"settlementId": id, "currency": currency, "amountMinor": in.AmountMinor, "jobCount": len(in.JobReferences)})
	})
	if err != nil {
		return nil, err
	}
	return s.GetSettlement(ctx, t, id)
}

// GetSettlement reads a settlement with its jobs.
func (s *Service) GetSettlement(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Settlements, false); err != nil {
		return nil, err
	}
	return s.o.Store.GetSettlement(ctx, t.OrganizationID, id)
}

// ListSettlements lists the organization's settlements.
func (s *Service) ListSettlements(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Settlements, false); err != nil {
		return nil, err
	}
	return s.o.Store.ListSettlements(ctx, t.OrganizationID)
}

// SettlementCommand approves or posts a settlement.
func (s *Service) SettlementCommand(ctx context.Context, t tenant.Context, id, command, key string, expected int64) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Settlements, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(expected); err != nil {
		return nil, err
	}
	if command == "approve" {
		changed, err := s.o.Store.ApproveSettlement(ctx, t.OrganizationID, id, t.ActorID, expected)
		if err != nil {
			return nil, err
		}
		if !changed {
			return nil, apperrors.Conflict("Settlement changed or cannot be approved.", nil)
		}
		return s.GetSettlement(ctx, t, id)
	}
	if command != "post" {
		return nil, apperrors.Validation("Unsupported settlement command.", nil)
	}
	v, err := s.GetSettlement(ctx, t, id)
	if err != nil {
		return nil, err
	}
	if statusString(v) != "APPROVED" || int64Value(v, "version") != expected {
		return nil, apperrors.Conflict("Settlement changed or is not approved.", nil)
	}
	req := models.PostingRequest{OrganizationID: t.OrganizationID, RecordType: "SUBCONTRACTOR_SETTLEMENT", RecordID: id, OperationType: "SETTLEMENT_REQUEST", IdempotencyKey: key, AmountMinor: int64Value(v, "amount_minor"), Currency: fmt.Sprint(v["currency"]), ExpectedVersion: expected}
	opID, confirmed, err := s.createPosting(ctx, t, req)
	if err != nil {
		return nil, err
	}
	if !confirmed {
		if _, err = s.executePosting(ctx, t, opID, req); err != nil {
			return nil, apperrors.Unavailable("Settlement posting requires reconciliation.", err)
		}
	}
	if err := s.o.Store.MarkSettlementPosted(ctx, t.OrganizationID, id, opID); err != nil {
		return nil, err
	}
	return s.GetSettlement(ctx, t, id)
}
