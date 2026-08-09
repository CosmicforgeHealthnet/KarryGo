package usecases

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

type ProjectionRepairPort interface {
	RepairPrivateExecutionProjection(context.Context, pgx.Tx, models.ExecutionOperation, ExecutionOutcome) error
}

type ReconciliationWorkerConfig struct {
	Owner                  string
	Lease                  time.Duration
	BatchSize, MaxAttempts int
	BaseBackoff            time.Duration
}

type ReconciliationWorkerResult struct{ Claimed, Resolved, Retried, DeadLettered int }

func (s *Service) RunProjectionReconciliationPass(ctx context.Context, execution ExecutionPort,
	repair ProjectionRepairPort, c ReconciliationWorkerConfig) (ReconciliationWorkerResult, error) {
	var result ReconciliationWorkerResult
	if !s.opts.Flags.WorkerEnabled || s.opts.Flags.KillSwitch || execution == nil || repair == nil {
		return result, nil
	}
	if c.Owner == "" {
		c.Owner = "private-projection-reconciliation"
	}
	if c.Lease <= 0 {
		c.Lease = time.Minute
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 20
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 8
	}
	if c.BaseBackoff <= 0 {
		c.BaseBackoff = 5 * time.Second
	}
	if _, err := s.opts.Repository.RecoverExpiredReconciliations(ctx); err != nil {
		return result, err
	}
	cases, err := s.opts.Repository.ClaimReconciliations(ctx, c.Owner, c.Lease, c.BatchSize)
	if err != nil {
		return result, err
	}
	result.Claimed = len(cases)
	for _, item := range cases {
		op, err := s.opts.Repository.GetExecutionByID(ctx, nil, item.ExecutionOperationID)
		if err != nil {
			return result, err
		}
		t := tenant.Context{OrganizationID: op.OrganizationID, ActorType: tenant.ActorTypeSystem,
			ActorID: "private-projection-reconciliation", CorrelationID: item.CorrelationID}
		stop := make(chan struct{})
		done := make(chan struct{})
		go func(caseID string) {
			defer close(done)
			interval := c.Lease / 3
			if interval < time.Second {
				interval = time.Second
			}
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					return
				case <-ctx.Done():
					return
				case <-ticker.C:
					_ = s.opts.Repository.HeartbeatReconciliation(ctx, c.Owner, caseID, c.Lease)
				}
			}
		}(item.ID)
		out, callErr := execution.ReadBack(ctx, t, op)
		close(stop)
		<-done
		if callErr == nil && out.Definite && out.SourceVersion > 0 {
			err = s.tx(ctx, func(tx pgx.Tx) error {
				if err := repair.RepairPrivateExecutionProjection(ctx, tx, op, out); err != nil {
					return err
				}
				if err := s.opts.Repository.ReconciliationAttempt(ctx, tx, item, "RESOLVED", ""); err != nil {
					return err
				}
				return s.opts.Repository.ResolveReconciliation(ctx, tx, c.Owner, item.ID)
			})
			if err != nil {
				return result, err
			}
			result.Resolved++
			continue
		}
		code := "private_projection_readback_unknown"
		permanent := false
		var clientErr *platformclients.Error
		if errors.As(callErr, &clientErr) {
			code = string(clientErr.Outcome)
			permanent = clientErr.Outcome != platformclients.OutcomeUnknown &&
				clientErr.Outcome != platformclients.OutcomeTransient
		}
		if err := s.opts.Repository.ReconciliationAttempt(ctx, nil, item, "FAILED", code); err != nil {
			return result, err
		}
		if permanent || item.Attempts >= c.MaxAttempts {
			if err := s.opts.Repository.DeadLetterReconciliation(ctx, c.Owner, item.ID, code); err != nil {
				return result, err
			}
			result.DeadLettered++
			continue
		}
		next := s.opts.Now().Add(c.BaseBackoff * time.Duration(1<<min(item.Attempts, 6)))
		if err := s.opts.Repository.RetryReconciliation(ctx, c.Owner, item.ID, code, next); err != nil {
			return result, err
		}
		result.Retried++
	}
	return result, nil
}
