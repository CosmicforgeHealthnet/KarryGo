package usecases

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// CreateSupportCase creates a case in support-dispute-service and projects it.
func (s *Service) CreateSupportCase(ctx context.Context, t tenant.Context, key string, in models.SupportCreateInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Support, true); err != nil {
		return nil, err
	}
	if strings.TrimSpace(key) == "" || strings.TrimSpace(in.Subject) == "" || strings.TrimSpace(in.Description) == "" {
		return nil, apperrors.Validation("Support idempotency key, subject and description are required.", nil)
	}
	if s.o.Support == nil || !s.o.Support.Configured() {
		return nil, apperrors.Unavailable("Support service is unavailable.", nil)
	}
	if existing, err := s.o.Store.SupportCaseByCreateKey(ctx, t.OrganizationID, key); err == nil {
		return existing, nil
	}
	projection, err := s.o.Support.Create(ctx, t.OrganizationID, key, in)
	if err != nil {
		return nil, apperrors.Unavailable("Support case outcome is unknown and requires reconciliation.", err)
	}
	if projection.CaseID == "" || projection.SourceVersion <= 0 {
		return nil, apperrors.Unavailable("Support service returned an invalid projection.", nil)
	}
	localID := ids.New("scp")
	err = s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		if err := s.o.Store.InsertCreatedSupportCase(ctx, tx, localID, t.OrganizationID, in, projection, key); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "support_case_projection.created.v1", "support_case_projection", localID, 1, map[string]any{"projectionId": localID, "supportCaseId": projection.CaseID, "status": projection.Status, "sourceVersion": projection.SourceVersion})
	})
	if err != nil {
		return nil, err
	}
	return s.GetSupportCase(ctx, t, projection.CaseID)
}

// GetSupportCase reads a projection by local or upstream ID.
func (s *Service) GetSupportCase(ctx context.Context, t tenant.Context, caseID string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Support, false); err != nil {
		return nil, err
	}
	return s.o.Store.GetSupportCase(ctx, t.OrganizationID, caseID)
}

// ListSupportCases lists the organization's projections.
func (s *Service) ListSupportCases(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Support, false); err != nil {
		return nil, err
	}
	return s.o.Store.ListSupportCases(ctx, t.OrganizationID)
}

// ApplySupportEvent projects a support event in its own transaction.
func (s *Service) ApplySupportEvent(ctx context.Context, in models.SupportEvent) error {
	if err := s.gate(s.o.Flags.Support, true); err != nil {
		return err
	}
	if in.EventID == "" || in.OrganizationID == "" || in.CaseID == "" || in.SourceVersion <= 0 {
		return apperrors.Validation("Support event is incomplete.", nil)
	}
	in.SafePayload = models.RawOr(in.SafePayload, "{}")
	return s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error { return s.ApplySupportEventTx(ctx, tx, in) })
}

// ApplySupportEventTx projects a support event in a caller-owned transaction.
// It is used by the common Section 26 stream consumer so the shared inbox and
// the authoritative support projection commit atomically.
func (s *Service) ApplySupportEventTx(ctx context.Context, tx pgx.Tx, in models.SupportEvent) error {
	if err := s.gate(s.o.Flags.Support, true); err != nil {
		return err
	}
	if in.EventID == "" || in.OrganizationID == "" || in.CaseID == "" || in.SourceVersion <= 0 {
		return apperrors.Validation("Support event is incomplete.", nil)
	}
	in.SafePayload = models.RawOr(in.SafePayload, "{}")
	t := tenant.Context{OrganizationID: in.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "support-dispute-service"}
	id, current, err := s.o.Store.LockSupportCase(ctx, tx, in.OrganizationID, in.CaseID)
	if errors.Is(err, pgx.ErrNoRows) {
		id = ids.New("scp")
		if err := s.o.Store.InsertEventSupportCase(ctx, tx, id, in); err != nil {
			return err
		}
		current = 0
	} else if err != nil {
		return err
	}
	result, gap := models.SupportEventOutcome(current, in.SourceVersion)
	if err := s.o.Store.InsertSupportInbox(ctx, tx, in, result); err != nil {
		return err
	}
	if result == "DUPLICATE" {
		return nil
	}
	if err := s.o.Store.ApplySupportEvent(ctx, tx, in, gap); err != nil {
		return err
	}
	return s.record(ctx, tx, t, "support_case_projection.updated.v1", "support_case_projection", id, in.SourceVersion, map[string]any{"projectionId": id, "supportCaseId": in.CaseID, "status": in.Status, "sourceVersion": in.SourceVersion, "gapStatus": gap})
}

// CreateQualityCase opens an internal quality case.
func (s *Service) CreateQualityCase(ctx context.Context, t tenant.Context, in models.QualityInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Quality, true); err != nil {
		return nil, err
	}
	in.Severity = strings.ToUpper(in.Severity)
	if in.Category == "" || in.SourceType == "" || in.SourceReference == "" || !models.ValidQualitySeverity(in.Severity) {
		return nil, apperrors.Validation("Quality category, severity and source are required.", nil)
	}
	if !branchAllowed(t, in.BranchID) {
		return nil, apperrors.Forbidden("Branch access is denied.", nil)
	}
	id := ids.New("qlt")
	err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		if err := s.o.Store.InsertQualityCase(ctx, tx, id, t.OrganizationID, t.ActorID, in); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "quality_case.created.v1", "quality_case", id, 1, map[string]any{"qualityCaseId": id, "category": in.Category, "severity": in.Severity, "sourceType": in.SourceType, "sourceReference": in.SourceReference})
	})
	if err != nil {
		return nil, err
	}
	return s.GetQualityCase(ctx, t, id)
}

// GetQualityCase reads a quality case in the caller's branch scope.
func (s *Service) GetQualityCase(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Quality, false); err != nil {
		return nil, err
	}
	v, err := s.o.Store.GetQualityCase(ctx, t.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	branch, _ := v["branch_id"].(string)
	if !branchAllowed(t, branch) {
		return nil, apperrors.NotFound("Resource not found.", nil)
	}
	return v, nil
}

// ListQualityCases lists quality cases in the caller's branch scope.
func (s *Service) ListQualityCases(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Quality, false); err != nil {
		return nil, err
	}
	return s.o.Store.ListQualityCases(ctx, t.OrganizationID, t.PermittedBranchIDs)
}

// UpdateQualityCase edits or resolves a quality case at the expected version.
func (s *Service) UpdateQualityCase(ctx context.Context, t tenant.Context, id string, in models.QualityInput, resolve bool) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Quality, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(in.ExpectedVersion); err != nil {
		return nil, err
	}
	if resolve && (strings.TrimSpace(in.RootCause) == "" || strings.TrimSpace(in.CorrectiveActionSummary) == "") {
		return nil, apperrors.Validation("Quality resolution requires root cause and corrective action.", nil)
	}
	err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		status, version, err := s.o.Store.LockQualityCase(ctx, tx, t.OrganizationID, id)
		if err != nil {
			return err
		}
		if version != in.ExpectedVersion {
			return apperrors.Conflict("Quality case version changed.", nil)
		}
		next, err := models.NextQualityStatus(status, in.Status, resolve)
		if err != nil {
			return err
		}
		nv, err := s.o.Store.UpdateQualityCase(ctx, tx, t.OrganizationID, id, in, strings.ToUpper(in.Severity), next, t.ActorID)
		if err != nil {
			return err
		}
		if err := s.o.Store.InsertQualityHistory(ctx, tx, t.OrganizationID, id, status, next, in, t.ActorID, nv); err != nil {
			return err
		}
		event := "quality_case.updated.v1"
		if resolve {
			event = "quality_case.resolved.v1"
		}
		return s.record(ctx, tx, t, event, "quality_case", id, nv, map[string]any{"qualityCaseId": id, "status": next})
	})
	if err != nil {
		return nil, err
	}
	return s.GetQualityCase(ctx, t, id)
}

// RecordMeasurement stores an SLA measurement judged against its target and
// raises an escalation on breach.
func (s *Service) RecordMeasurement(ctx context.Context, in models.MeasurementInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.SLA, true); err != nil {
		return nil, err
	}
	if in.OrganizationID == "" || in.AgreementReference == "" || in.MetricCode == "" || in.FormulaVersion <= 0 || !in.PeriodEnd.After(in.PeriodStart) {
		return nil, apperrors.Validation("SLA measurement scope, formula and period are required.", nil)
	}
	status, measured, err := models.BreachStatus(in)
	if err != nil {
		return nil, err
	}
	id := ids.New("slm")
	t := tenant.Context{OrganizationID: in.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "section23-sla-worker"}
	err = s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		stored, created, err := s.o.Store.InsertMeasurement(ctx, tx, id, in, measured, status)
		if err != nil {
			return err
		}
		id = stored
		if !created {
			return nil
		}
		if err := s.record(ctx, tx, t, "sla_measurement.created.v1", "sla_measurement", id, 1, map[string]any{"measurementId": id, "metricCode": in.MetricCode, "breachStatus": status, "formulaVersion": in.FormulaVersion}); err != nil {
			return err
		}
		if status == "BREACHED" {
			eid := ids.New("esc")
			if err := s.o.Store.InsertBreachEscalation(ctx, tx, eid, in.OrganizationID, id); err != nil {
				return err
			}
			return s.record(ctx, tx, t, "escalation.created.v1", "escalation", eid, 1, map[string]any{"escalationId": eid, "sourceType": "SLA_MEASUREMENT", "sourceReference": id, "level": 1})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.o.Store.GetMeasurement(ctx, in.OrganizationID, id)
}

// GetMeasurement reads one measurement.
func (s *Service) GetMeasurement(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.SLA, false); err != nil {
		return nil, err
	}
	return s.o.Store.GetMeasurement(ctx, t.OrganizationID, id)
}

// ListMeasurements lists the organization's measurements.
func (s *Service) ListMeasurements(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.SLA, false); err != nil {
		return nil, err
	}
	return s.o.Store.ListMeasurements(ctx, t.OrganizationID)
}

// CreateEscalation opens an escalation; the same source and level is opened
// once.
func (s *Service) CreateEscalation(ctx context.Context, t tenant.Context, in models.EscalationInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Escalations, true); err != nil {
		return nil, err
	}
	if in.SourceType == "" || in.SourceReference == "" || in.Level <= 0 || in.RecipientType == "" || in.RecipientReference == "" || in.DueAt.IsZero() {
		return nil, apperrors.Validation("Escalation source, level, recipient and due time are required.", nil)
	}
	id := ids.New("esc")
	err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		stored, created, err := s.o.Store.InsertEscalation(ctx, tx, id, t.OrganizationID, in)
		if err != nil {
			return err
		}
		id = stored
		if !created {
			return nil
		}
		return s.record(ctx, tx, t, "escalation.created.v1", "escalation", id, 1, map[string]any{"escalationId": id, "sourceType": in.SourceType, "sourceReference": in.SourceReference, "level": in.Level})
	})
	if err != nil {
		return nil, err
	}
	return s.GetEscalation(ctx, t, id)
}

// GetEscalation reads one escalation.
func (s *Service) GetEscalation(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Escalations, false); err != nil {
		return nil, err
	}
	return s.o.Store.GetEscalation(ctx, t.OrganizationID, id)
}

// ListEscalations lists the organization's escalations.
func (s *Service) ListEscalations(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Escalations, false); err != nil {
		return nil, err
	}
	return s.o.Store.ListEscalations(ctx, t.OrganizationID)
}

// EscalationCommand acknowledges or resolves an escalation.
func (s *Service) EscalationCommand(ctx context.Context, t tenant.Context, id, command string, expected int64) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Escalations, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(expected); err != nil {
		return nil, err
	}
	err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		status, version, err := s.o.Store.LockEscalation(ctx, tx, t.OrganizationID, id)
		if err != nil {
			return err
		}
		if version != expected {
			return apperrors.Conflict("Escalation version changed.", nil)
		}
		next, err := models.NextEscalationStatus(status, command)
		if err != nil {
			return err
		}
		nv, err := s.o.Store.UpdateEscalationStatus(ctx, tx, t.OrganizationID, id, next)
		if err != nil {
			return err
		}
		if err := s.o.Store.InsertEscalationHistory(ctx, tx, t.OrganizationID, id, status, next, t.ActorID, nv); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "escalation."+command+"d.v1", "escalation", id, nv, map[string]any{"escalationId": id, "status": next})
	})
	if err != nil {
		return nil, err
	}
	return s.GetEscalation(ctx, t, id)
}
