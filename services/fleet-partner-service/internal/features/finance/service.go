package finance

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/enforcement"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Service struct {
	pool   *pgxpool.Pool
	repo   *Repository
	wallet platformclients.FinanceWalletContract
	audit  *audit.Recorder
	events *events.DomainPublisher
	// eligibility gates every money-moving request. It is set by the
	// composition root; a service without it refuses those requests rather
	// than running them ungated.
	eligibility enforcement.Gate
}

func NewService(pool *pgxpool.Pool, repo *Repository, wallet platformclients.FinanceWalletContract, publisher *events.DomainPublisher) *Service {
	return &Service{pool: pool, repo: repo, wallet: wallet, audit: audit.NewRecorder(), events: publisher}
}

// WithEligibility wires the durable eligibility gate.
//
// Nil-safe on the receiver: BuildFinanceService returns nil when the Wallet
// client is unconfigured, and a composition root should not have to know that
// to wire a gate.
func (s *Service) WithEligibility(gate enforcement.Gate) *Service {
	if s == nil {
		return nil
	}
	s.eligibility = gate
	return s
}

// requireEligible blocks a money-moving request unless a current, unexpired
// ELIGIBLE decision authorizes it.
//
// The decision is durable and its identifier travels no further than the audit
// trail here, because the operation's own record already carries the actor,
// the fingerprint and the expected version. What matters at this point is only
// that the gate was passed and that refusing is the default.
func (s *Service) requireEligible(ctx context.Context, organizationID string,
	branchID string, operationType string, actor Actor) error {
	subject := enforcement.Company(organizationID, operationType)
	subject.BranchID = branchID
	subject.ActorType = actor.Type
	subject.ActorID = actor.ID
	subject.CorrelationID = actor.CorrelationID
	subject.CausationID = actor.CausationID
	_, err := enforcement.Require(ctx, s.eligibility, subject)
	return err
}

type Request struct {
	OrganizationID, BranchID, SubjectType, SubjectID, OperationType, IdempotencyKey, CorrelationID, CausationID, ActorType, ActorID string
	Payload                                                                                                                         json.RawMessage
	PayloadVersion                                                                                                                  int
	ExpectedVersion                                                                                                                 int64
}

func (s *Service) Request(ctx context.Context, in Request) (*Operation, error) {
	if in.OrganizationID == "" || in.SubjectID == "" || in.IdempotencyKey == "" || in.ActorID == "" {
		return nil, errors.New("finance operation: required field missing")
	}
	if in.PayloadVersion == 0 {
		in.PayloadVersion = 1
	}
	sum := sha256.Sum256(append([]byte(in.OperationType+"\x00"+in.OrganizationID+"\x00"+in.SubjectID+"\x00"), in.Payload...))
	o := &Operation{OrganizationID: in.OrganizationID, BranchID: in.BranchID, RequestedByActorType: in.ActorType, RequestedByActorID: in.ActorID, SubjectType: in.SubjectType, SubjectID: in.SubjectID, OperationType: in.OperationType, Payload: in.Payload, PayloadVersion: in.PayloadVersion, ExpectedVersion: in.ExpectedVersion, IdempotencyKey: in.IdempotencyKey, RequestFingerprint: hex.EncodeToString(sum[:]), CorrelationID: in.CorrelationID, CausationID: in.CausationID}
	err := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		created, err := s.repo.Create(ctx, tx, o)
		if err != nil {
			return err
		}
		if !created {
			return nil
		}
		if err = s.audit.Record(ctx, tx, audit.Entry{OrganizationID: in.OrganizationID, ActorType: in.ActorType, ActorID: in.ActorID, Action: "finance.operation.requested", ResourceType: "finance_operation", ResourceID: o.ID, NewValues: map[string]any{"operationType": in.OperationType, "status": o.Status}, CorrelationID: in.CorrelationID}); err != nil {
			return err
		}
		return s.events.Publish(ctx, tx, "fleet.finance.operation.requested.v1", "FINANCE_OPERATION", o.ID, o.Version, in.OrganizationID, in.CorrelationID, map[string]any{"operationId": o.ID, "operationType": o.OperationType, "status": o.Status})
	})
	return o, err
}

func (s *Service) ReplayOperation(ctx context.Context, organizationID, operationID, actorID, correlationID string) error {
	if organizationID == "" || operationID == "" || actorID == "" {
		return errors.New("finance replay: required field missing")
	}
	return database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		var version int64
		if err := tx.QueryRow(ctx, `SELECT version FROM fleet_partner_finance_operations WHERE id=$1 AND organization_id=$2 FOR UPDATE`, operationID, organizationID).Scan(&version); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if err := s.repo.ReplayIn(ctx, tx, operationID); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: organizationID, ActorType: "SERVICE", ActorID: actorID, Action: "finance.operation.replayed", ResourceType: "finance_operation", ResourceID: operationID, CorrelationID: correlationID}); err != nil {
			return err
		}
		return s.events.Publish(ctx, tx, "fleet.finance.operation.replayed.v1", "FINANCE_OPERATION", operationID, version+1, organizationID, correlationID, map[string]any{"operationId": operationID})
	})
}

func (s *Service) ReplayProjection(ctx context.Context, organizationID, projectionID, actorID, correlationID string) error {
	if organizationID == "" || projectionID == "" || actorID == "" {
		return errors.New("finance projection replay: required field missing")
	}
	return database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		var version int64
		if err := tx.QueryRow(ctx, `SELECT version FROM fleet_partner_finance_projections WHERE id=$1 AND organization_id=$2 FOR UPDATE`, projectionID, organizationID).Scan(&version); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if err := s.repo.ReplayProjectionIn(ctx, tx, projectionID); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: organizationID, ActorType: "SERVICE", ActorID: actorID, Action: "finance.projection.replayed", ResourceType: "finance_projection", ResourceID: projectionID, CorrelationID: correlationID}); err != nil {
			return err
		}
		return s.events.Publish(ctx, tx, "fleet.finance.projection.replayed.v1", "FINANCE_PROJECTION", projectionID, version+1, organizationID, correlationID, map[string]any{"projectionId": projectionID})
	})
}

type WorkerConfig struct {
	Owner                        string
	BatchSize                    int
	Lease, Interval, BaseBackoff time.Duration
	ProjectionStaleAfter         time.Duration
	MaxAttempts                  int
}

func (c WorkerConfig) norm() WorkerConfig {
	if c.Owner == "" {
		c.Owner = "fleet-finance"
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 25
	}
	if c.Lease <= 0 {
		c.Lease = 30 * time.Second
	}
	if c.Interval <= 0 {
		c.Interval = 5 * time.Second
	}
	if c.BaseBackoff <= 0 {
		c.BaseBackoff = 2 * time.Second
	}
	if c.ProjectionStaleAfter <= 0 {
		c.ProjectionStaleAfter = 15 * time.Minute
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 8
	}
	return c
}
func (s *Service) Run(ctx context.Context, c WorkerConfig) error {
	c = c.norm()
	ticker := time.NewTicker(c.Interval)
	defer ticker.Stop()
	for {
		_ = s.RunPass(ctx, c)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
func (s *Service) RunPass(ctx context.Context, c WorkerConfig) error {
	c = c.norm()
	if s.wallet == nil || !s.wallet.Configured() {
		return platformclients.ErrNotConfigured
	}
	_, _ = s.repo.Recover(ctx)
	ops, err := s.repo.Claim(ctx, c.Owner, c.Lease, c.BatchSize)
	if err != nil {
		return err
	}
	for _, o := range ops {
		if err = s.process(ctx, c, o); err != nil {
			return err
		}
	}
	return s.runProjectionPass(ctx, c)
}
func (s *Service) process(ctx context.Context, c WorkerConfig, o Operation) error {
	meta := platformclients.CallMeta{OrganizationID: o.OrganizationID, CorrelationID: o.CorrelationID, CausationID: o.CausationID, IdempotencyKey: o.IdempotencyKey}
	var result platformclients.WalletOperation
	var err error
	if o.Status == StatusReconciling || o.Status == StatusUnknown {
		result, err = s.wallet.ReadOperationByKey(ctx, meta, o.OrganizationID, o.OperationType, o.IdempotencyKey)
	} else {
		result, err = s.wallet.SubmitOperation(ctx, meta, platformclients.WalletOperationRequest{OperationType: o.OperationType, OrganizationID: o.OrganizationID, SubjectID: o.SubjectID, Payload: o.Payload, PayloadVersion: o.PayloadVersion, ExpectedVersion: o.ExpectedVersion, IdempotencyKey: o.IdempotencyKey})
	}
	status, code := StatusSucceeded, ""
	if err != nil {
		status = StatusRetryable
		code = "wallet_unavailable"
		var e *platformclients.Error
		if errors.As(err, &e) {
			switch e.Outcome {
			case platformclients.OutcomeUnknown:
				status = StatusUnknown
				code = "wallet_outcome_unknown"
			case platformclients.OutcomeValidation, platformclients.OutcomeForbidden, platformclients.OutcomePermanent:
				status = StatusPermanent
				code = "wallet_rejected"
			}
		}
	}
	next := time.Now().Add(c.BaseBackoff * time.Duration(1<<min(o.Attempts, 8)))
	return database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.repo.Outcome(ctx, tx, o, status, result.ID, code, next, c.MaxAttempts); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: o.OrganizationID, ActorType: "SYSTEM", ActorID: c.Owner, Action: "finance.operation.status_changed", ResourceType: "finance_operation", ResourceID: o.ID, PreviousValues: map[string]any{"status": o.Status}, NewValues: map[string]any{"status": status, "safeErrorCode": code}, CorrelationID: o.CorrelationID}); err != nil {
			return err
		}
		return s.events.Publish(ctx, tx, "fleet.finance.operation.status_changed.v1", "FINANCE_OPERATION", o.ID, o.Version+1, o.OrganizationID, o.CorrelationID, map[string]any{"operationId": o.ID, "operationType": o.OperationType, "walletReference": result.ID, "status": status, "safeErrorCode": code})
	})
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *Service) runProjectionPass(ctx context.Context, c WorkerConfig) error {
	if err := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		queued, err := s.repo.QueueStale(ctx, tx, time.Now().Add(-c.ProjectionStaleAfter), c.BatchSize)
		if err != nil {
			return err
		}
		for _, p := range queued {
			if err = s.audit.Record(ctx, tx, audit.Entry{OrganizationID: p.OrganizationID, ActorType: "SYSTEM", ActorID: c.Owner, Action: "finance.projection.stale", ResourceType: "finance_projection", ResourceID: p.ID, PreviousValues: map[string]any{"freshness": "CURRENT"}, NewValues: map[string]any{"freshness": "STALE", "reconciliationStatus": "PENDING"}}); err != nil {
				return err
			}
			if err = s.events.Publish(ctx, tx, "fleet.finance.projection.reconciliation_requested.v1", "FINANCE_PROJECTION", p.ID, p.Version, p.OrganizationID, "", map[string]any{"projectionId": p.ID, "projectionType": p.ProjectionType, "walletReference": p.WalletReference, "reason": "stale"}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	_, _ = s.repo.RecoverProjectionLeases(ctx)
	projections, err := s.repo.ClaimProjections(ctx, c.Owner, c.Lease, c.BatchSize)
	if err != nil {
		return err
	}
	for _, projection := range projections {
		if err = s.processProjection(ctx, c, projection); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) processProjection(ctx context.Context, c WorkerConfig, p Projection) error {
	meta := platformclients.CallMeta{OrganizationID: p.OrganizationID, CorrelationID: "finance-reconcile-" + p.ID, CausationID: p.LastEventID}
	result, readErr := s.wallet.ReadProjection(ctx, meta, p.OrganizationID, p.ProjectionType, p.WalletReference)
	reconciliationStatus, freshness, code := "IDLE", "CURRENT", ""
	if readErr == nil && (result.ID != p.WalletReference || (result.OrganizationID != "" && result.OrganizationID != p.OrganizationID) || result.Version < p.SourceVersion) {
		readErr = errors.New("authoritative read-back is older than projection")
		reconciliationStatus, freshness, code = StatusRetryable, "GAP_DETECTED", "wallet_version_behind"
	}
	if readErr != nil && code == "" {
		reconciliationStatus, freshness, code = StatusRetryable, "UNKNOWN", "wallet_unavailable"
		var clientErr *platformclients.Error
		if errors.As(readErr, &clientErr) {
			switch clientErr.Outcome {
			case platformclients.OutcomeUnknown:
				reconciliationStatus, code = StatusUnknown, "wallet_outcome_unknown"
			case platformclients.OutcomeValidation, platformclients.OutcomeForbidden, platformclients.OutcomePermanent, platformclients.OutcomeNotFound:
				reconciliationStatus, code = StatusDeadLetter, "wallet_projection_rejected"
			}
		}
	}
	if reconciliationStatus == StatusRetryable && p.ReconciliationAttempts+1 >= c.MaxAttempts {
		reconciliationStatus = StatusDeadLetter
	}
	next := time.Now().Add(c.BaseBackoff * time.Duration(1<<min(p.ReconciliationAttempts, 8)))
	return database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.repo.ProjectionOutcome(ctx, tx, p, reconciliationStatus, freshness, result.Status, code, result.Version, next, c.MaxAttempts); err != nil {
			return err
		}
		if err := s.audit.Record(ctx, tx, audit.Entry{OrganizationID: p.OrganizationID, ActorType: "SYSTEM", ActorID: c.Owner, Action: "finance.projection.reconciled", ResourceType: "finance_projection", ResourceID: p.ID, PreviousValues: map[string]any{"freshness": p.FreshnessState, "reconciliationStatus": p.ReconciliationStatus}, NewValues: map[string]any{"freshness": freshness, "reconciliationStatus": reconciliationStatus, "safeErrorCode": code}}); err != nil {
			return err
		}
		return s.events.Publish(ctx, tx, "fleet.finance.projection.reconciled.v1", "FINANCE_PROJECTION", p.ID, p.Version+1, p.OrganizationID, meta.CorrelationID, map[string]any{"projectionId": p.ID, "projectionType": p.ProjectionType, "walletReference": p.WalletReference, "freshness": freshness, "reconciliationStatus": reconciliationStatus, "safeErrorCode": code})
	})
}
