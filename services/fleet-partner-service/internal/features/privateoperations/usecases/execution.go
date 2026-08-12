package usecases

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type ExecutionOutcome struct {
	Definite, Confirmed, Cancelled                                   bool
	CanonicalRequestReference, CanonicalTripReference, SafeErrorCode string
	NormalizedStatus                                                 string
	SourceVersion                                                    int64
}
type ExecutionPort interface {
	Submit(context.Context, tenant.Context, models.ExecutionOperation) (ExecutionOutcome, error)
	ReadBack(context.Context, tenant.Context, models.ExecutionOperation) (ExecutionOutcome, error)
	Cancel(context.Context, tenant.Context, models.ExecutionOperation, string) (ExecutionOutcome, error)
	ReadCancellation(context.Context, tenant.Context, models.ExecutionOperation) (ExecutionOutcome, error)
}
type ExecutionWorkerConfig struct {
	Owner                  string
	Lease                  time.Duration
	BatchSize, MaxAttempts int
	BaseBackoff            time.Duration
}
type ExecutionWorkerResult struct{ Claimed, Confirmed, Unknown, Failed, Cancelled int }

func (s *Service) RunExecutionPass(ctx context.Context, port ExecutionPort, c ExecutionWorkerConfig) (ExecutionWorkerResult, error) {
	var result ExecutionWorkerResult
	if !s.opts.Flags.WorkerEnabled || s.opts.Flags.KillSwitch || port == nil {
		return result, nil
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 20
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 8
	}
	if c.Lease <= 0 {
		c.Lease = time.Minute
	}
	if c.BaseBackoff <= 0 {
		c.BaseBackoff = 5 * time.Second
	}
	if _, e := s.opts.Repository.RecoverExpiredExecutions(ctx); e != nil {
		return result, e
	}
	ops, e := s.opts.Repository.ClaimExecutions(ctx, c.Owner, c.Lease, c.BatchSize)
	if e != nil {
		return result, e
	}
	result.Claimed = len(ops)
	for _, op := range ops {
		tc := tenant.Context{OrganizationID: op.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "private-execution-worker", CorrelationID: op.ID}
		if e := s.opts.Repository.Attempt(ctx, nil, op.OrganizationID, op.ID, "SUBMITTING", "", op.Attempts); e != nil && !errors.Is(e, repositories.ErrConflict) {
			return result, e
		}
		var out ExecutionOutcome
		var callErr error
		stopHeartbeat := make(chan struct{})
		heartbeatDone := make(chan struct{})
		go func(operationID string) {
			defer close(heartbeatDone)
			interval := c.Lease / 3
			if interval < time.Second {
				interval = time.Second
			}
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-stopHeartbeat:
					return
				case <-ctx.Done():
					return
				case <-ticker.C:
					_ = s.opts.Repository.HeartbeatExecution(ctx, c.Owner, operationID, c.Lease)
				}
			}
		}(op.ID)
		switch op.Status {
		case models.ExecutionSubmitting:
			out, callErr = port.Submit(ctx, tc, op)
		case models.ExecutionUnknown, models.ExecutionReconciliation:
			out, callErr = port.ReadBack(ctx, tc, op)
		case models.ExecutionCancellationPending:
			out, callErr = port.Cancel(ctx, tc, op, "ORGANIZATION_CANCELLED")
		case models.ExecutionCancellationUnknown:
			out, callErr = port.ReadCancellation(ctx, tc, op)
		}
		close(stopHeartbeat)
		<-heartbeatDone
		if callErr != nil || !out.Definite {
			code := "private_execution_outcome_unknown"
			var clientErr *platformclients.Error
			if errors.As(callErr, &clientErr) && clientErr.Outcome != platformclients.OutcomeUnknown && clientErr.Outcome != platformclients.OutcomeTransient {
				if e := s.opts.Repository.Attempt(ctx, nil, op.OrganizationID, op.ID,
					"DEAD_LETTER", "private_execution_permanent_failure", op.Attempts); e != nil {
					return result, e
				}
				if e := s.opts.Repository.DeadLetterExecution(ctx, c.Owner, op.ID, "private_execution_permanent_failure"); e != nil {
					return result, e
				}
				result.Failed++
				continue
			}
			if e := s.opts.Repository.Attempt(ctx, nil, op.OrganizationID, op.ID, "OUTCOME_UNKNOWN", code, op.Attempts); e != nil && !errors.Is(e, repositories.ErrConflict) {
				return result, e
			}
			next := s.opts.Now().Add(c.BaseBackoff * time.Duration(1<<min(op.Attempts, 6)))
			status := models.ExecutionUnknown
			if op.Status == models.ExecutionCancellationPending || op.Status == models.ExecutionCancellationUnknown {
				status = models.ExecutionCancellationUnknown
			}
			if op.Attempts >= c.MaxAttempts {
				if e := s.opts.Repository.Attempt(ctx, nil, op.OrganizationID, op.ID,
					"DEAD_LETTER", code, op.Attempts); e != nil {
					return result, e
				}
				if e := s.opts.Repository.DeadLetterExecution(ctx, c.Owner, op.ID, code); e != nil {
					return result, e
				}
				result.Failed++
				continue
			}
			if e := s.opts.Repository.RetryExecution(ctx, c.Owner, op.ID, status, code, next); e != nil {
				return result, e
			}
			result.Unknown++
			continue
		}
		if out.Confirmed {
			e := s.tx(ctx, func(tx pgx.Tx) error {
				if err := s.opts.Repository.ResolveExecution(ctx, tx, c.Owner, op.ID, models.ExecutionConfirmed, out.CanonicalRequestReference, out.CanonicalTripReference, "", out.SourceVersion); err != nil {
					return err
				}
				assignment, err := s.opts.Repository.LockAssignment(ctx, tx, op.OrganizationID, op.AssignmentID)
				if err != nil {
					return err
				}
				if assignment.Status == models.StatusAccepted {
					updated, err := s.opts.Repository.SetAssignmentStatus(ctx, tx, op.OrganizationID,
						assignment.ID, assignment.Version, models.StatusReady, "EXECUTION_CONFIRMED", "", "")
					if err != nil {
						return err
					}
					if err = s.opts.Repository.History(ctx, tx, op.OrganizationID, assignment.ID,
						assignment.Status, updated.Status, "EXECUTION_CONFIRMED", "system",
						"private-execution-worker", op.CorrelationID); err != nil {
						return err
					}
				}
				if err := s.opts.Repository.Attempt(ctx, tx, op.OrganizationID, op.ID, "CONFIRMED", "", op.Attempts); err != nil {
					return err
				}
				if err := s.opts.Audit.Record(ctx, tx, audit.EntryFromTenant(tc,
					"private_executions.confirmed", "private_execution", op.ID)); err != nil {
					return err
				}
				if s.opts.Events != nil {
					return s.opts.Events.Publish(ctx, tx, "private_execution.confirmed.v1",
						"private_execution", op.ID, op.Version+1, op.OrganizationID,
						op.CorrelationID, map[string]any{"executionId": op.ID,
							"assignmentId": op.AssignmentID, "executionService": op.ExecutionService})
				}
				return nil
			})
			if e != nil {
				return result, e
			}
			result.Confirmed++
			continue
		}
		if out.Cancelled {
			if e := s.opts.Repository.ResolveExecution(ctx, nil, c.Owner, op.ID, models.ExecutionCancelled, out.CanonicalRequestReference, out.CanonicalTripReference, "", out.SourceVersion); e != nil {
				return result, e
			}
			result.Cancelled++
			continue
		}
		if e := s.opts.Repository.ResolveExecution(ctx, nil, c.Owner, op.ID, models.ExecutionFailed, "", "", out.SafeErrorCode, out.SourceVersion); e != nil {
			return result, e
		}
		result.Failed++
	}
	return result, nil
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
