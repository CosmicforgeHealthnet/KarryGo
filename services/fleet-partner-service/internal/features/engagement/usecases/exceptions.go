package usecases

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// ExceptionStore is the persistence of operational exceptions and the SLA
// breach view. repositories.Repository satisfies it.
type ExceptionStore interface {
	LiveExceptionFor(ctx context.Context, q database.Querier, organizationID, sourceType, sourceReference, exceptionType string) (string, error)
	InsertException(ctx context.Context, q database.Querier, id, organizationID string, in models.ExceptionInput, due time.Time, actor string) (bool, error)
	GetException(ctx context.Context, organizationID, id string) (map[string]any, error)
	ListExceptions(ctx context.Context, organizationID string, branchIDs []string) ([]map[string]any, error)
	LockException(ctx context.Context, q database.Querier, organizationID, id string) (models.ExceptionState, error)
	UpdateException(ctx context.Context, q database.Querier, organizationID, id, status, severity, owner string, level int, due time.Time, rootCause, resolution string) (int64, error)
	InsertExceptionHistory(ctx context.Context, q database.Querier, organizationID, id, previous, next, actor, note string, version int64) error
	ExceptionHistory(ctx context.Context, organizationID, id string) ([]map[string]any, error)
	OverdueExceptions(ctx context.Context, organizationID string, now time.Time) ([]models.OverdueException, error)
	ListSLABreaches(ctx context.Context, organizationID string) ([]map[string]any, error)
}

const jobExceptionSLA = "SECTION23_EXCEPTION_SLA"

func (s *Service) exceptions() (ExceptionStore, error) {
	x, ok := s.o.Store.(ExceptionStore)
	if !ok {
		return nil, apperrors.Unavailable("Operational exceptions are unavailable.", nil)
	}
	return x, nil
}

func errTransition(msg string) error {
	return apperrors.New(http.StatusConflict, "state_transition_invalid", msg, nil)
}

// RaiseException opens an operational exception. A live exception for the
// same source and type is returned instead of a duplicate.
func (s *Service) RaiseException(ctx context.Context, t tenant.Context, in models.ExceptionInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Exceptions, true); err != nil {
		return nil, err
	}
	x, err := s.exceptions()
	if err != nil {
		return nil, err
	}
	in.ExceptionType = strings.ToUpper(strings.TrimSpace(in.ExceptionType))
	in.Severity = strings.ToUpper(strings.TrimSpace(in.Severity))
	in.Summary = strings.TrimSpace(in.Summary)
	if !models.ExceptionTypes[in.ExceptionType] || !models.ExceptionSeverities[in.Severity] {
		return nil, apperrors.Validation("exceptionType and severity must be supported values.", nil)
	}
	if in.SourceType == "" || in.SourceReference == "" || in.Summary == "" || len(in.Summary) > 2000 {
		return nil, apperrors.Validation("Source and a summary of at most 2000 characters are required.", nil)
	}
	if !branchAllowed(t, in.BranchID) {
		return nil, apperrors.Forbidden("The branch is outside your scope.", nil)
	}
	if len(in.EvidenceFileIDs) > 20 {
		return nil, apperrors.Validation("At most 20 evidence file references are allowed.", nil)
	}
	for _, f := range in.EvidenceFileIDs {
		if strings.TrimSpace(f) == "" {
			return nil, apperrors.Validation("Evidence file references must not be empty.", nil)
		}
	}
	id := ids.New("oex")
	due := s.o.Now().UTC().Add(models.ExceptionSLA(in.Severity))
	err = s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		created, err := x.InsertException(ctx, tx, id, t.OrganizationID, in, due, t.ActorID)
		if err != nil {
			return err
		}
		if !created {
			id, err = x.LiveExceptionFor(ctx, tx, t.OrganizationID, in.SourceType, in.SourceReference, in.ExceptionType)
			return err
		}
		if err = x.InsertExceptionHistory(ctx, tx, t.OrganizationID, id, "", "OPEN", t.ActorID, in.Summary, 1); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "exception.opened.v1", "operational_exception", id, 1, map[string]any{"exceptionId": id, "exceptionType": in.ExceptionType, "severity": in.Severity, "sourceType": in.SourceType, "slaDueAt": due})
	})
	if err != nil {
		return nil, err
	}
	return s.GetException(ctx, t, id)
}

// GetException reads an exception with its history.
func (s *Service) GetException(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Exceptions, false); err != nil {
		return nil, err
	}
	x, err := s.exceptions()
	if err != nil {
		return nil, err
	}
	v, err := x.GetException(ctx, t.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if b, _ := v["branch_id"].(string); !branchAllowed(t, b) {
		return nil, apperrors.NotFound("Resource not found.", nil)
	}
	v["history"], err = x.ExceptionHistory(ctx, t.OrganizationID, id)
	return v, err
}

// ListExceptions lists exceptions, most urgent first.
func (s *Service) ListExceptions(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Exceptions, false); err != nil {
		return nil, err
	}
	x, err := s.exceptions()
	if err != nil {
		return nil, err
	}
	return x.ListExceptions(ctx, t.OrganizationID, t.PermittedBranchIDs)
}

// ExceptionCommand runs UPDATE (owner/severity), ACKNOWLEDGE, ESCALATE or
// RESOLVE under the expected version. Resolution requires a root cause and a
// resolution; escalation creates the next escalation level through the
// durable escalation and notification-intent pipeline, and a repeated
// escalation of the same level is a no-op (spec 53.2: repeat-safe).
func (s *Service) ExceptionCommand(ctx context.Context, t tenant.Context, id, command string, in models.ExceptionInput) (map[string]any, error) {
	if _, err := s.GetException(ctx, t, id); err != nil {
		return nil, err
	}
	if err := s.gate(s.o.Flags.Exceptions, true); err != nil {
		return nil, err
	}
	if err := expectedRequired(in.ExpectedVersion); err != nil {
		return nil, err
	}
	x, _ := s.exceptions()
	err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		state, err := x.LockException(ctx, tx, t.OrganizationID, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.NotFound("Resource not found.", nil)
		}
		if err != nil {
			return err
		}
		if state.Version != in.ExpectedVersion {
			return apperrors.New(http.StatusConflict, "version_conflict", "Exception version changed.", nil)
		}
		live := state.Status == "OPEN" || state.Status == "ACKNOWLEDGED" || state.Status == "ESCALATED"
		if !live {
			return errTransition("A resolved exception cannot change.")
		}
		next, severity, owner, level, due := state.Status, state.Severity, state.OwnerUserID, state.EscalationLevel, state.SLADueAt
		rootCause, resolution, note, event := "", "", strings.TrimSpace(in.Note), ""
		switch command {
		case "UPDATE":
			if in.Severity != "" {
				severity = strings.ToUpper(in.Severity)
				if !models.ExceptionSeverities[severity] {
					return apperrors.Validation("Unsupported severity.", nil)
				}
				if severity != state.Severity {
					due = s.o.Now().UTC().Add(models.ExceptionSLA(severity))
				}
			}
			if in.OwnerUserID != "" {
				owner = in.OwnerUserID
			}
			event = "exception.updated.v1"
		case "ACKNOWLEDGE":
			if state.Status != "OPEN" {
				return errTransition("Only an open exception can be acknowledged.")
			}
			next, event = "ACKNOWLEDGED", "exception.acknowledged.v1"
			if owner == "" {
				owner = t.ActorID
			}
		case "ESCALATE":
			level++
			if err := s.escalateTx(ctx, tx, t, id, level); err != nil {
				return err
			}
			next, event = "ESCALATED", "exception.escalated.v1"
		case "RESOLVE":
			rootCause, resolution = strings.TrimSpace(in.RootCause), strings.TrimSpace(in.Resolution)
			if rootCause == "" || resolution == "" {
				return apperrors.Validation("rootCause and resolution are required to resolve an exception.", nil)
			}
			next, event = "RESOLVED", "exception.resolved.v1"
		default:
			return apperrors.BadRequest("Unsupported exception command.", nil)
		}
		v, err := x.UpdateException(ctx, tx, t.OrganizationID, id, next, severity, owner, level, due, rootCause, resolution)
		if err != nil {
			return err
		}
		if err = x.InsertExceptionHistory(ctx, tx, t.OrganizationID, id, state.Status, next, t.ActorID, note, v); err != nil {
			return err
		}
		return s.record(ctx, tx, t, event, "operational_exception", id, v, map[string]any{"exceptionId": id, "status": next, "severity": severity, "escalationLevel": level})
	})
	if err != nil {
		return nil, err
	}
	return s.GetException(ctx, t, id)
}

// escalateTx creates the escalation for one level, due now, routed to the
// organization's operations role. It is idempotent per level.
func (s *Service) escalateTx(ctx context.Context, tx pgx.Tx, t tenant.Context, exceptionID string, level int) error {
	id, created, err := s.o.Store.InsertEscalation(ctx, tx, ids.New("esc"), t.OrganizationID, models.EscalationInput{SourceType: "OPERATIONAL_EXCEPTION", SourceReference: exceptionID, Level: level, RecipientType: "ORGANIZATION_ROLE", RecipientReference: "OPERATIONS", DueAt: s.o.Now().UTC()})
	if err != nil || !created {
		return err
	}
	return s.record(ctx, tx, t, "escalation.created.v1", "escalation", id, 1, map[string]any{"escalationId": id, "sourceType": "OPERATIONAL_EXCEPTION", "sourceReference": exceptionID, "level": level})
}

// ListSLABreaches lists breached and at-risk SLA measurements.
func (s *Service) ListSLABreaches(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.SLA, false); err != nil {
		return nil, err
	}
	x, err := s.exceptions()
	if err != nil {
		return nil, err
	}
	return x.ListSLABreaches(ctx, t.OrganizationID)
}

// handleExceptionSLA escalates every live exception past its SLA by one
// level and moves its next deadline, so an unanswered exception keeps
// climbing until someone owns it. Re-running the job never duplicates an
// escalation: each level is unique per exception.
func (s *Service) handleExceptionSLA(ctx context.Context, job jobs.Job) error {
	v, err := jobPayload(job)
	if err != nil {
		return err
	}
	x, err := s.exceptions()
	if err != nil {
		return err
	}
	org := v["organizationId"]
	overdue, err := x.OverdueExceptions(ctx, org, s.o.Now().UTC())
	if err != nil {
		return err
	}
	t := tenant.Context{OrganizationID: org, ActorType: tenant.ActorTypeSystem, ActorID: "section23-exception-sla-worker", CorrelationID: job.ID}
	for _, o := range overdue {
		o := o
		err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
			state, err := x.LockException(ctx, tx, org, o.ID)
			if err != nil {
				return err
			}
			if state.SLADueAt.After(s.o.Now().UTC()) || state.Status == "RESOLVED" || state.Status == "CLOSED" {
				return nil
			}
			level := state.EscalationLevel + 1
			if err = s.escalateTx(ctx, tx, t, o.ID, level); err != nil {
				return err
			}
			due := s.o.Now().UTC().Add(models.ExceptionSLA(state.Severity))
			nv, err := x.UpdateException(ctx, tx, org, o.ID, "ESCALATED", state.Severity, state.OwnerUserID, level, due, "", "")
			if err != nil {
				return err
			}
			if err = x.InsertExceptionHistory(ctx, tx, org, o.ID, state.Status, "ESCALATED", t.ActorID, "SLA breached", nv); err != nil {
				return err
			}
			return s.record(ctx, tx, t, "exception.escalated.v1", "operational_exception", o.ID, nv, map[string]any{"exceptionId": o.ID, "status": "ESCALATED", "escalationLevel": level, "reason": "SLA_BREACHED"})
		})
		if err != nil {
			return err
		}
	}
	return nil
}
