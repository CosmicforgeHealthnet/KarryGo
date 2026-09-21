package orchestration

import (
	"context"
	"time"
)

// Section 11's background work.
//
// Every one of these is a bounded sweep over durable rows, claimed with
// FOR UPDATE SKIP LOCKED, rather than a goroutine holding state in memory. The
// reason is the failure this section exists to survive: a process that dies
// between deciding to commit and hearing back. In-memory work would simply be
// lost; a claimed row is picked up by the next pass.
//
// None of them retries a mission create. Retrying an ambiguous create is how
// one order gets two vehicles, so the only pass that touches an unknown outcome
// is the reconciler, and it asks rather than repeats.

// SweepResult counts what one pass did, for metrics and for tests that need to
// assert a pass actually did something.
type SweepResult struct {
	Claimed   int `json:"claimed"`
	Advanced  int `json:"advanced"`
	Committed int `json:"committed"`
	Failed    int `json:"failed"`
}

// Worker intervals. They differ because the work differs: evaluation is the hot
// loop that catches an early-ready kitchen, reconciliation is rarer and more
// expensive, and both are far more frequent than a stale-signal sweep.
const (
	// EvaluationInterval is how often the planner looks at due orchestrations.
	// Thirty seconds is short enough that a kitchen finishing early is noticed
	// while the difference still matters.
	EvaluationInterval = 30 * time.Second

	// ReconciliationInterval is how often unknown commit outcomes are chased.
	ReconciliationInterval = 20 * time.Second

	// BatchSize bounds one pass so a backlog is worked through over several
	// ticks rather than in one long transaction.
	BatchSize = 50
)

// RunEvaluationPass advances every orchestration whose evaluation is due.
//
// A failure on one orchestration does not abandon the pass. One order that
// cannot be planned must not stop every other order being planned, which is
// exactly what returning on the first error would do.
func (s *Service) RunEvaluationPass(ctx context.Context) (SweepResult, error) {
	var result SweepResult
	if s == nil || !s.jitEnabled {
		return result, nil
	}
	now := s.now().UTC()
	due, err := s.repo.ClaimDue(ctx, now, BatchSize)
	if err != nil {
		return result, err
	}
	result.Claimed = len(due)

	for _, orchestration := range due {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		updated, evalErr := s.Evaluate(ctx, orchestration.ID)
		if evalErr != nil {
			// The orchestration keeps its claimed next-evaluation time, so it
			// is retried on a later pass rather than hammered now.
			result.Failed++
			continue
		}
		result.Advanced++
		if updated.State.Committed() {
			result.Committed++
		}
	}
	return result, nil
}

// RunReconciliationPass resolves orchestrations whose commit outcome is unknown.
//
// This is the pass that makes an ambiguous create survivable. It asks the
// authority whether the deterministic mission reference exists; only a definite
// absence returns the orchestration to the commit queue.
func (s *Service) RunReconciliationPass(ctx context.Context) (SweepResult, error) {
	var result SweepResult
	if s == nil || s.committer == nil {
		return result, nil
	}
	now := s.now().UTC()
	pending, err := s.repo.ClaimReconciling(ctx, now, BatchSize)
	if err != nil {
		return result, err
	}
	result.Claimed = len(pending)

	for _, orchestration := range pending {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		updated, reconcileErr := s.Reconcile(ctx, orchestration.ID)
		if reconcileErr != nil {
			result.Failed++
			continue
		}
		result.Advanced++
		if updated.State.Committed() {
			result.Committed++
		}
	}
	return result, nil
}

// StaleSignalSweeper finds readiness that has gone quiet.
//
// It is a separate concern from the planner because its output is not a
// decision: a quiet order is probably fine and nobody has said so. What the
// sweep does is make the silence visible -- to the predictor as lower
// confidence, and to an operator as a list -- rather than acting on it.
type StaleSignalSweeper interface {
	// SweepStale re-times every orchestration whose readiness has gone quiet,
	// returning how many it touched.
	SweepStale(ctx context.Context, cutoff time.Time) (int, error)
}

// SweepStale re-evaluates orchestrations whose readiness has gone quiet.
//
// Re-evaluating rather than escalating is deliberate. The prediction already
// widens its own window for a stale signal, so simply looking again applies
// that widening to the commit timing; escalating every quiet order would bury
// an operator in orders that are merely unattended.
func (s *Service) SweepStale(ctx context.Context, cutoff time.Time) (int, error) {
	if s == nil || !s.jitEnabled {
		return 0, nil
	}
	// The due sweep already re-times everything it claims, so a stale signal is
	// handled by bringing the next evaluation forward rather than by a separate
	// mutation path. Two paths writing the same timing is how they disagree.
	due, err := s.repo.ClaimDue(ctx, cutoff, BatchSize)
	if err != nil {
		return 0, err
	}
	touched := 0
	for _, orchestration := range due {
		if ctx.Err() != nil {
			return touched, ctx.Err()
		}
		if _, evalErr := s.Evaluate(ctx, orchestration.ID); evalErr == nil {
			touched++
		}
	}
	return touched, nil
}
