package fulfilment_readiness

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Actor is who is asking to change readiness.
//
// It carries the branch scope rather than a role name, because the question
// that decides permission is "may this person act for this branch", and a role
// that grants nothing at this branch is not permission.
type Actor struct {
	Kind              Source
	Ref               string
	BusinessID        string
	PermittedBranches []string
	// AllBranches is set for a business-wide staff scope.
	AllBranches bool
	// OriginEventRef is set for SYSTEM actors and names the Vendor-owned event.
	OriginEventRef string
}

// MayAct reports whether this actor may change readiness for a branch.
//
// The three refusals it encodes are the ones that matter. A customer is not an
// actor here at all. A provider is not an actor here at all. And a member of
// one business's staff has no standing at another business's branch, however
// senior they are at their own.
func (a Actor) MayAct(businessID, branchID string) bool {
	switch a.Kind {
	case SourceAdmin:
		// Platform operators act across tenants by design; the audit trail is
		// what constrains them, not the scope check.
		return true
	case SourceSystem:
		// A system adapter is Vendor's own code applying a Vendor-owned event.
		// It is still scoped to the business that owns the order.
		return a.BusinessID == "" || a.BusinessID == businessID
	case SourceVendorStaff:
		if a.BusinessID != businessID {
			return false
		}
		if a.AllBranches {
			return true
		}
		for _, permitted := range a.PermittedBranches {
			if permitted == branchID {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// QueueReader reports how much other work a branch has in hand.
//
// It is an interface because the count lives in Vendor's production and
// capacity features, which already maintain it. Recomputing it here would give
// the platform two answers to one question.
type QueueReader interface {
	BranchQueueDepth(ctx context.Context, businessID, branchID string) (int, error)
}

// ReadySignal is the Section 13 fact that an order became ready, measured
// against the readiness the branch had promised before it did.
type ReadySignal struct {
	VendorOrderID   string
	BusinessID      string
	BranchID        string
	Version         int64
	OnTime          bool
	LatenessSeconds int64
	PrepSeconds     int64
}

// ReadySignalSink records a ReadySignal inside the readiness transaction, so
// the fact commits with the state it describes or not at all.
type ReadySignalSink interface {
	RecordReady(ctx context.Context, tx pgx.Tx, signal ReadySignal) error
}

// Service holds the readiness rules.
type Service struct {
	repo    *Repository
	queue   QueueReader
	signals ReadySignalSink
	enabled bool
	now     func() time.Time
}

// Options configures the service.
type Options struct {
	Repository *Repository
	Queue      QueueReader
	// Enabled is VENDOR_READINESS_ORCHESTRATION_ENABLED.
	Enabled bool
	Now     func() time.Time
	// Signals receives readiness facts for analytics. Optional: without it
	// readiness behaves exactly as before.
	Signals ReadySignalSink
}

// NewService builds the service.
func NewService(options Options) *Service {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Service{repo: options.Repository, queue: options.Queue, signals: options.Signals,
		enabled: options.Enabled, now: now}
}

// Enabled reports whether readiness writes are active.
func (s *Service) Enabled() bool { return s != nil && s.enabled }

// Read returns one order's readiness with a fresh prediction.
//
// Reads stay available while the feature flag is off. A disabled flag stops new
// readiness being declared; hiding what is already recorded would make an order
// that is genuinely ready look like one nobody has started.
func (s *Service) Read(ctx context.Context, vendorOrderID string) (Readiness, error) {
	return s.repo.Get(ctx, vendorOrderID)
}

// History returns one order's transitions.
func (s *Service) History(ctx context.Context, vendorOrderID string) ([]HistoryEntry, error) {
	return s.repo.History(ctx, vendorOrderID)
}

// StartRequest begins preparation for an order.
type StartRequest struct {
	VendorOrderID  string
	ParentOrderID  string
	BusinessID     string
	BranchID       string
	VendorType     VendorType
	Actor          Actor
	VendorEstimate *time.Time
}

// Start moves an order into preparation and produces its first prediction.
func (s *Service) Start(ctx context.Context, request StartRequest) (Readiness, error) {
	if !s.Enabled() {
		return Readiness{}, ErrDisabled
	}
	if !request.Actor.MayAct(request.BusinessID, request.BranchID) {
		return Readiness{}, fmt.Errorf("%w: %s may not act for branch %s",
			ErrForbidden, request.Actor.Kind, request.BranchID)
	}
	if _, err := ParseVendorType(string(request.VendorType)); err != nil {
		return Readiness{}, err
	}

	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return Readiness{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	current, err := s.repo.Ensure(ctx, tx, Readiness{
		VendorOrderID: request.VendorOrderID, ParentOrderID: request.ParentOrderID,
		BusinessID: request.BusinessID, BranchID: request.BranchID,
		VendorType: request.VendorType, State: StateNotStarted, Source: request.Actor.Kind,
	})
	if err != nil {
		return Readiness{}, err
	}
	// Re-read under the lock so a concurrent start loses on version rather than
	// both writing a first prediction.
	current, err = s.repo.Lock(ctx, tx, request.VendorOrderID)
	if err != nil {
		return Readiness{}, err
	}

	move := Move{To: StatePreparing, Source: request.Actor.Kind,
		ActorRef: request.Actor.Ref, OriginEventRef: request.Actor.OriginEventRef,
		EstimatedAt: request.VendorEstimate}
	if current.State == StateNotStarted {
		// NOT_STARTED cannot reach PREPARING directly: accepting the order is a
		// distinct commitment, and collapsing the two would lose the moment the
		// branch took responsibility for it.
		if err = s.commit(ctx, tx, current, Move{To: StateAccepted, Source: request.Actor.Kind,
			ActorRef: request.Actor.Ref, OriginEventRef: request.Actor.OriginEventRef}); err != nil {
			return Readiness{}, err
		}
		if current, err = s.repo.Lock(ctx, tx, request.VendorOrderID); err != nil {
			return Readiness{}, err
		}
	}
	if err = s.commit(ctx, tx, current, move); err != nil {
		return Readiness{}, err
	}
	updated, err := s.repo.Lock(ctx, tx, request.VendorOrderID)
	if err != nil {
		return Readiness{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Readiness{}, err
	}
	return updated, nil
}

// AdvanceRequest is any other readiness change.
type AdvanceRequest struct {
	VendorOrderID   string
	ExpectedVersion int64
	Move            Move
	Actor           Actor
}

// Advance applies one readiness change.
//
// Version is required and checked. Two staff members with the same order open
// on two screens is the normal case, not an edge one, and the second save must
// be told the ground moved rather than silently winning.
func (s *Service) Advance(ctx context.Context, request AdvanceRequest) (Readiness, error) {
	if !s.Enabled() {
		return Readiness{}, ErrDisabled
	}
	if request.ExpectedVersion < 1 {
		return Readiness{}, fmt.Errorf("%w: expected_version is required", ErrValidation)
	}

	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return Readiness{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	current, err := s.repo.Lock(ctx, tx, request.VendorOrderID)
	if err != nil {
		return Readiness{}, err
	}
	if current.Version != request.ExpectedVersion {
		return Readiness{}, fmt.Errorf("%w: readiness is at version %d", ErrVersion, current.Version)
	}
	if !request.Actor.MayAct(current.BusinessID, current.BranchID) {
		return Readiness{}, fmt.Errorf("%w: %s may not act for branch %s",
			ErrForbidden, request.Actor.Kind, current.BranchID)
	}

	move := request.Move
	move.Source = request.Actor.Kind
	move.ActorRef = request.Actor.Ref
	if move.OriginEventRef == "" {
		move.OriginEventRef = request.Actor.OriginEventRef
	}
	if err = s.commit(ctx, tx, current, move); err != nil {
		return Readiness{}, err
	}
	updated, err := s.repo.Lock(ctx, tx, request.VendorOrderID)
	if err != nil {
		return Readiness{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Readiness{}, err
	}
	return updated, nil
}

// commit validates one move, predicts, and writes the readiness row and its
// history entry inside the caller's transaction.
//
// Both writes or neither. A readiness that moved with no history entry is a
// change nobody can explain afterwards, and explaining it is the whole reason
// the history exists.
func (s *Service) commit(ctx context.Context, tx pgx.Tx, current Readiness, move Move) error {
	if err := ValidateMove(current.State, current.Substate, current.VendorType, move); err != nil {
		return err
	}

	now := s.now().UTC()
	rules, err := s.rulesFor(ctx, current, now)
	if err != nil {
		return err
	}

	estimate := move.EstimatedAt
	if estimate == nil {
		estimate = current.VendorEstimatedReadyAt
	}
	if estimate != nil && move.EstimatedAt != nil {
		// The vendor's own number is bounded before it is stored, so one
		// mistyped estimate cannot drag the branch's planning with it. The
		// original still appears in the history entry below.
		start := now
		if current.PrepStartedAt != nil {
			start = current.PrepStartedAt.UTC()
		}
		clamped, _ := rules.ClampVendorEstimate(start, estimate.UTC())
		estimate = &clamped
	}

	prediction := s.predict(ctx, current, move, rules, estimate, now)

	var prepStarted *time.Time
	if move.To == StatePreparing || move.To == StateAccepted {
		prepStarted = &now
	}
	var actualReady *time.Time
	if move.To == StateReady {
		actualReady = &now
	}

	blockReason, delayReason := current.BlockReason, current.DelayReason
	switch move.To {
	case StateBlocked:
		blockReason = move.ReasonCode
	case StateDelayed:
		delayReason = move.ReasonCode
	default:
		// Leaving a side state clears the reason that put it there, so a stale
		// "OUT_OF_STOCK" does not follow an order that has since been resolved.
		blockReason, delayReason = "", ""
	}

	updated, err := s.repo.Apply(ctx, tx, ApplyInput{
		VendorOrderID: current.VendorOrderID, ExpectedVersion: current.Version,
		State: move.To, Substate: move.Substate, Source: move.Source,
		BlockReason: blockReason, DelayReason: delayReason,
		VendorEstimate: estimate, Prediction: prediction,
		PrepStartedAt: prepStarted, ActualReadyAt: actualReady, Now: now,
	})
	if err != nil {
		return err
	}
	if err = s.recordReady(ctx, tx, current, updated, now, move); err != nil {
		return err
	}

	return s.repo.AppendHistory(ctx, tx, HistoryEntry{
		VendorOrderID: current.VendorOrderID,
		FromState:     current.State, ToState: move.To,
		FromSubstate: current.Substate, ToSubstate: move.Substate,
		Source:         move.Source,
		EstimateBefore: current.VendorEstimatedReadyAt,
		EstimateAfter:  estimate,
		ActorRef:       move.ActorRef, OriginEventRef: move.OriginEventRef,
		ReasonCode: move.ReasonCode, Version: updated.Version,
	})
}

func (s *Service) rulesFor(ctx context.Context, current Readiness, now time.Time) (Rules, error) {
	candidates, err := s.repo.Rules(ctx, current.VendorType, current.BusinessID, current.BranchID, now)
	if err != nil {
		return Rules{}, err
	}
	return SelectRules(candidates, current.VendorType, current.BusinessID, current.BranchID), nil
}

// predict builds the forecast for the state the order is moving INTO.
//
// An Analytics-sourced branch profile that is missing or stale is not an error:
// the predictor falls back to the category default and says so through the
// basis, which is the difference between a degraded prediction and no
// fulfilment planning at all.
func (s *Service) predict(ctx context.Context, current Readiness, move Move,
	rules Rules, estimate *time.Time, now time.Time) Prediction {
	input := PredictionInput{
		Now: now, VendorType: current.VendorType, Rules: rules,
		State: move.To, Substate: move.Substate,
		PrepStartedAt: current.PrepStartedAt, VendorEstimatedReadyAt: estimate,
		LastSignalAt: current.LastSignalAt,
	}
	if profile, err := s.repo.Profile(ctx, current.BusinessID, current.BranchID,
		current.VendorType); err == nil {
		input.Profile = profile
		input.ProfileFresh = profile.Fresh(now, ProfileMaxAge)
	}
	if s.queue != nil {
		if depth, err := s.queue.BranchQueueDepth(ctx, current.BusinessID, current.BranchID); err == nil {
			input.QueueDepth = depth
		}
	}
	return Predict(input)
}

// Ready reports whether an order has been declared ready and not since
// corrected.
func Ready(r Readiness) bool { return r.State == StateReady }

// DescribeCorrection renders why a READY order was moved back, for the history.
func DescribeCorrection(move Move) string {
	reason := strings.TrimSpace(move.ReasonCode)
	if reason == "" {
		return ""
	}
	return string(move.To) + ":" + reason
}

// recordReady emits the readiness fact analytics measures branches by.
//
// Only the FIRST transition to ready counts: an order that bounced back to
// preparing and became ready again was still only late once. And only an
// order that had a promise can be on time or late -- with no prediction and
// no vendor estimate there is nothing to measure against, so nothing is
// emitted rather than a guess.
func (s *Service) recordReady(ctx context.Context, tx pgx.Tx, before, after Readiness, now time.Time, move Move) error {
	if s.signals == nil || move.To != StateReady || before.FirstActualReadyAt != nil {
		return nil
	}
	promise := before.PredictedReadyEnd
	if promise == nil {
		promise = before.VendorEstimatedReadyAt
	}
	if promise == nil {
		return nil
	}
	signal := ReadySignal{
		VendorOrderID: after.VendorOrderID, BusinessID: after.BusinessID, BranchID: after.BranchID,
		Version: after.Version, OnTime: !now.After(promise.UTC()),
	}
	if !signal.OnTime {
		signal.LatenessSeconds = int64(now.Sub(promise.UTC()).Seconds())
	}
	if before.PrepStartedAt != nil && now.After(before.PrepStartedAt.UTC()) {
		signal.PrepSeconds = int64(now.Sub(before.PrepStartedAt.UTC()).Seconds())
	}
	return s.signals.RecordReady(ctx, tx, signal)
}
