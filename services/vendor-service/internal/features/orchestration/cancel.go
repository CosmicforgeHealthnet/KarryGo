package orchestration

// Withdrawing a planned handover.
//
// Cancellation before commitment is cheap and must stay cheap: nobody has been
// asked, no vehicle is moving, and the only thing that exists is Vendor's own
// intention. Cancellation AFTER commitment is a different operation entirely --
// an authority owns the work, and only that authority can stop it.
//
// So this file does exactly one thing, and refuses the other. It withdraws an
// uncommitted plan, and it reports plainly when the plan is already committed
// so the caller can use the authority's own cancellation contract instead.
// Faking a local cancellation for committed work would leave Vendor believing
// nothing is happening while a rider is on their way.

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// ErrAlreadyCommitted means an execution authority owns this work now.
//
// It is distinct from ErrCommitted (which the state machine raises for a
// refused transition) because callers act on it differently: this one tells the
// order flow to use the authority's cancellation path rather than to give up.
var ErrAlreadyCommitted = errors.New("orchestration: an authority already owns this work")

// Cancel withdraws an orchestration that has not been committed.
//
// The read and the write happen under the same row lock, because the race that
// matters here is precisely cancellation against commitment: a customer
// cancelling at the same moment a worker decides the commit window is open. One
// of the two has to lose, and the lock is what makes it exactly one.
func (s *Service) Cancel(ctx context.Context, id, reasonCode string) (Orchestration, error) {
	if s == nil || s.repo == nil {
		return Orchestration{}, ErrDisabled
	}
	now := s.now().UTC()

	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return Orchestration{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	current, err := s.repo.Lock(ctx, tx, id)
	if err != nil {
		return Orchestration{}, err
	}
	if current.State == StateCancelled {
		// Already withdrawn. Saying so is not an error: a retried cancellation
		// must be as harmless as the first one.
		return current, nil
	}
	if current.State.Committed() {
		return current, ErrAlreadyCommitted
	}
	if current.State == StateCommitting {
		// A create is in flight and its outcome is unknown. Withdrawing now
		// would abandon a mission that may already exist, so the reconciler
		// finishes establishing what happened first.
		return current, fmt.Errorf(
			"%w: a commit is in flight and must be reconciled first", ErrTransition)
	}
	if !CanTransition(current.State, StateCancelled) {
		return current, fmt.Errorf("%w: %s cannot be cancelled", ErrTransition, current.State)
	}

	reason := strings.TrimSpace(reasonCode)
	if reason == "" {
		reason = "ORDER_CANCELLED"
	}
	updated, err := s.repo.Apply(ctx, tx, current.ID, current.Version, Update{
		State: StateCancelled, RecoveryState: current.RecoveryState, ReasonCode: reason,
	}, now)
	if err != nil {
		return current, err
	}
	if err = s.repo.RecordDecision(ctx, tx, DecisionEvent{
		OrchestrationID: current.ID, PlanVersion: current.PlanVersion,
		EventType: EventCancelled, ReasonCode: reason,
	}); err != nil {
		return current, err
	}
	if err = tx.Commit(ctx); err != nil {
		return current, err
	}
	return updated, nil
}

// CancelByGroup withdraws the orchestration owning one fulfilment group.
//
// It reports found=false when nothing was ever planned for the group, which is
// an ordinary outcome rather than an error: an order cancelled before its
// fulfilment was ever activated has no plan to withdraw.
func (s *Service) CancelByGroup(ctx context.Context, groupRef, reasonCode string) (
	Orchestration, bool, error) {
	if s == nil || s.repo == nil {
		return Orchestration{}, false, ErrDisabled
	}
	current, err := s.repo.GetByGroup(ctx, strings.TrimSpace(groupRef))
	if errors.Is(err, ErrNotFound) {
		return Orchestration{}, false, nil
	}
	if err != nil {
		return Orchestration{}, false, err
	}
	cancelled, err := s.Cancel(ctx, current.ID, reasonCode)
	if err != nil {
		return cancelled, false, err
	}
	return cancelled, true, nil
}

// sameRequirement compares two requirement snapshots by value.
func sameRequirement(left, right Requirement) bool {
	return reflect.DeepEqual(left, right)
}

// samePickups compares the stored pickups with a freshly built set.
//
// Only the facts grouping is decided FROM are compared. The planner's own
// output -- group index and split reason -- is deliberately excluded: it is
// derived from these inputs, so including it would make an orchestration that
// has already been planned look different from the identical one that has not,
// and every retry would replan.
func samePickups(stored, incoming []Pickup) bool {
	if len(stored) != len(incoming) {
		return false
	}
	for index := range stored {
		left, right := stored[index], incoming[index]
		left.GroupIndex, right.GroupIndex = nil, nil
		left.SplitReason, right.SplitReason = "", ""
		if len(left.Handling) == 0 {
			left.Handling = nil
		}
		if len(right.Handling) == 0 {
			right.Handling = nil
		}
		if !reflect.DeepEqual(left, right) {
			return false
		}
	}
	return true
}

// RecordOrderBinding notes that a real paid order owns this orchestration.
//
// It is recorded once and is a no-op on a repeat, because activation and
// recovery both re-run: a second ORDER_BOUND row would suggest the order was
// bound twice, which is exactly the thing this evidence exists to rule out.
func (s *Service) RecordOrderBinding(ctx context.Context, id, parentOrderID, missionRef string) error {
	if s == nil || s.repo == nil {
		return ErrDisabled
	}
	existing, err := s.repo.Decisions(ctx, id, 0)
	if err != nil {
		return err
	}
	for _, event := range existing {
		if event.EventType == EventOrderBound {
			return nil
		}
	}
	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	current, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if err = s.repo.RecordDecision(ctx, tx, DecisionEvent{
		OrchestrationID: id, PlanVersion: current.PlanVersion,
		EventType: EventOrderBound,
		// The reason code carries the slot this orchestration was opened for,
		// so the binding can be read back without joining anything.
		ReasonCode:    "MISSION_SLOT",
		CorrelationID: strings.TrimSpace(parentOrderID) + ":" + strings.TrimSpace(missionRef),
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
