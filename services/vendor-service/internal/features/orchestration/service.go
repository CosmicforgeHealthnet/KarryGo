package orchestration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	readiness "cosmicforge/logistics/services/vendor-service/internal/features/fulfilment_readiness"
	"cosmicforge/logistics/services/vendor-service/internal/integrations/logistics"
	"cosmicforge/logistics/shared/go/supplypreview"
)

// AuthorityGate is the read side of the logistics registry.
//
// It is a narrow interface rather than the concrete registry so the selection
// rules can be tested without a deployment, and so this package depends on the
// two questions it actually asks rather than on everything the registry can do.
type AuthorityGate interface {
	// Usable reports whether this deployment may select an authority at all.
	Usable(authority logistics.Authority) bool

	// Serviceability asks one authority whether it can execute a requirement.
	Serviceability(ctx context.Context, authority logistics.Authority,
		requirement logistics.Requirement) (logistics.Serviceability, error)

	// Capability returns the authority's declared limits, which is where the
	// grouping policy's pickup and capacity bounds come from.
	Capability(ctx context.Context, authority logistics.Authority) (logistics.Capability, error)
}

// SupplyPreviewer asks an authority about supply without creating anything.
type SupplyPreviewer interface {
	Preview(ctx context.Context, authority logistics.Authority,
		request supplypreview.Request) (supplypreview.Response, error)
}

// MissionOutcome is what a commit attempt produced.
type MissionOutcome struct {
	// Definite means the authority gave a real answer. An indefinite outcome is
	// the one that matters: the create may or may not have landed, and the only
	// safe next step is to ask.
	Definite bool

	Accepted    bool
	MissionRef  string
	ExternalRef string
	RefusalCode string
}

// MissionCommitter creates and reads back a commerce mission.
//
// ReadMission is not optional convenience. After an ambiguous create, retrying
// is how a second vehicle gets dispatched for one order, so the contract
// requires a way to ask the authority what actually happened.
type MissionCommitter interface {
	CreateMission(ctx context.Context, orchestration Orchestration,
		pickups []Pickup) (MissionOutcome, error)

	// ReadMission answers whether a mission with this deterministic reference
	// exists at the authority. found=false must mean "definitely not there",
	// never "could not tell" -- an error is how the caller learns the
	// difference.
	ReadMission(ctx context.Context, authority logistics.Authority,
		missionRef string) (outcome MissionOutcome, found bool, err error)
}

// ReadinessReader supplies the readiness the timing decision rests on.
type ReadinessReader interface {
	Read(ctx context.Context, vendorOrderID string) (readiness.Readiness, error)
}

// Service holds Section 11's orchestration rules.
type Service struct {
	repo      *Repository
	gate      AuthorityGate
	preview   SupplyPreviewer
	committer MissionCommitter
	readiness ReadinessReader

	jitEnabled           bool
	autoSelectionEnabled bool
	now                  func() time.Time
}

// Options configures the service.
type Options struct {
	Repository *Repository
	Gate       AuthorityGate
	Preview    SupplyPreviewer
	Committer  MissionCommitter
	Readiness  ReadinessReader

	// JITEnabled is VENDOR_JIT_PICKUP_ENABLED.
	JITEnabled bool
	// AutoSelectionEnabled is VENDOR_AUTO_AUTHORITY_SELECTION_ENABLED. When
	// false the orchestration still plans and times, but keeps whatever
	// authority the checkout plan chose.
	AutoSelectionEnabled bool

	Now func() time.Time
}

// NewService builds the service.
func NewService(options Options) *Service {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Service{
		repo: options.Repository, gate: options.Gate, preview: options.Preview,
		committer: options.Committer, readiness: options.Readiness,
		jitEnabled: options.JITEnabled, autoSelectionEnabled: options.AutoSelectionEnabled,
		now: now,
	}
}

// OpenRequest starts an orchestration for a fulfilment group.
type OpenRequest struct {
	FulfilmentGroupRef string
	ParentOrderID      string
	Requirement        Requirement
	Pickups            []Pickup
	// PlannedAuthority is the checkout plan's own choice. It is used as-is
	// while automatic selection is off, and as a starting point when it is on.
	PlannedAuthority   logistics.Authority
	PlannedProductCode string
}

// Open records the plan for a fulfilment group.
//
// The orchestration is made durable BEFORE any authority is contacted. A crash
// between deciding to fulfil and asking anybody must leave a record a recovery
// pass can find, not a request nothing wrote down.
func (s *Service) Open(ctx context.Context, request OpenRequest) (Orchestration, error) {
	if !s.jitEnabled {
		return Orchestration{}, ErrDisabled
	}
	if strings.TrimSpace(request.FulfilmentGroupRef) == "" {
		return Orchestration{}, fmt.Errorf("%w: a fulfilment group reference is required", ErrValidation)
	}
	if err := request.Requirement.Validate(); err != nil {
		return Orchestration{}, err
	}
	if len(request.Pickups) == 0 {
		return Orchestration{}, fmt.Errorf("%w: at least one pickup is required", ErrValidation)
	}

	now := s.now().UTC()
	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return Orchestration{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	next := now
	opened, err := s.repo.Open(ctx, tx, Orchestration{
		ID:                 "orc_" + uuid.NewString(),
		FulfilmentGroupRef: request.FulfilmentGroupRef,
		ParentOrderID:      request.ParentOrderID,
		PlanVersion:        1,
		State:              StatePlanned,
		Requirement:        request.Requirement,
		DesiredPickupStart: timePointer(request.Requirement.DesiredPickupStart),
		DesiredPickupEnd:   timePointer(request.Requirement.DesiredPickupEnd),
		NextEvaluationAt:   &next,
		SelectionSource:    SelectionAutomatic,
	})
	if err != nil {
		return Orchestration{}, err
	}
	if opened.State.Committed() {
		// Already committed by an earlier pass. Replanning it would be Vendor
		// re-deciding work an authority now owns.
		return opened, nil
	}

	if err = s.repo.ReplacePickups(ctx, tx, opened.ID, request.Pickups); err != nil {
		return Orchestration{}, err
	}
	if err = s.repo.RecordDecision(ctx, tx, DecisionEvent{
		OrchestrationID: opened.ID, PlanVersion: opened.PlanVersion,
		EventType: EventRequirementBuilt,
	}); err != nil {
		return Orchestration{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Orchestration{}, err
	}
	return opened, nil
}

// Evaluate advances one orchestration by one step.
//
// It is the whole planner loop in one function so the ordering is visible:
// select an authority if needed, time the commitment, warm the supply, and
// commit only when the moment arrives. A worker calls this repeatedly; nothing
// here assumes it runs once.
func (s *Service) Evaluate(ctx context.Context, id string) (Orchestration, error) {
	if !s.jitEnabled {
		return Orchestration{}, ErrDisabled
	}
	current, err := s.repo.Get(ctx, id)
	if err != nil {
		return Orchestration{}, err
	}
	if current.State.Terminal() {
		return current, nil
	}
	if current.State.Committed() {
		// The authority owns it now. Vendor projects and does not re-time.
		return current, nil
	}

	now := s.now().UTC()

	// 1. Choose an authority, if one is not chosen or the choice is stale.
	if s.autoSelectionEnabled && current.SelectedAuthority == "" {
		if current, err = s.selectAuthority(ctx, current); err != nil {
			return current, err
		}
	}
	if current.SelectedAuthority == "" {
		return s.park(ctx, current, StateWaitingReadiness, "NO_AUTHORITY_SELECTED", now)
	}

	// 2. Time the commitment from the current readiness prediction.
	decision, err := s.timeCommit(ctx, current, now)
	if err != nil {
		return current, err
	}

	// 3. Warm the supply as commitment approaches. This creates nothing.
	nextState := StateWaitingReadiness
	if ShouldWarmMatch(now, decision.CommitNotBefore) {
		nextState = StateWarmMatch
		if err = s.warmMatch(ctx, current, now); err != nil {
			return current, err
		}
	}
	if decision.Due {
		nextState = StateCommitDue
	}

	commitAt := decision.CommitNotBefore
	nextAt := decision.NextEvaluationAt
	updated, err := s.transition(ctx, current, Update{
		State: nextState, CommitNotBefore: &commitAt, NextEvaluationAt: &nextAt,
		RecoveryState: current.RecoveryState, ReasonCode: decision.ReasonCode,
	}, now)
	if err != nil {
		return current, err
	}

	// 4. Commit, once, when the window is open.
	if nextState == StateCommitDue {
		return s.Commit(ctx, updated.ID)
	}
	return updated, nil
}

// selectAuthority asks every usable authority and records every answer.
func (s *Service) selectAuthority(ctx context.Context, current Orchestration) (Orchestration, error) {
	now := s.now().UTC()
	candidates := make([]Candidate, 0, 3)
	events := make([]DecisionEvent, 0, 6)

	for _, authority := range logistics.PlatformAuthorities() {
		if s.gate == nil || !s.gate.Usable(authority) {
			candidates = append(candidates, Candidate{Authority: authority,
				Outcome: OutcomeDisabled, ReasonCode: "AUTHORITY_DISABLED"})
			events = append(events, DecisionEvent{
				OrchestrationID: current.ID, PlanVersion: current.PlanVersion,
				EventType: EventServiceabilityChecked, CandidateAuthority: authority,
				OutcomeClass: OutcomeDisabled, ReasonCode: "AUTHORITY_DISABLED",
			})
			continue
		}
		answer, err := s.gate.Serviceability(ctx, authority, current.Requirement.Capability())
		// Serviceability answers whether the authority CAN, not which of its
		// products it would use. The product is resolved by the supply preview,
		// which is the surface that actually knows.
		candidate := Candidate{Authority: authority, ReasonCode: answer.ReasonCode,
			CapabilityVersion: answer.Version}
		switch {
		case err != nil:
			// An authority that could not be asked has said nothing about the
			// goods. It is not eliminated permanently.
			failure := logistics.Classify(authority, err)
			candidate.Outcome = ClassifyFailure(failure.Category, failure.ReasonCode)
			candidate.ReasonCode = failure.ReasonCode
		case answer.Serviceable:
			candidate.Outcome = OutcomeServiceable
		default:
			candidate.Outcome = ClassifyFailure(
				logistics.CategoryForReasonCode(answer.ReasonCode), answer.ReasonCode)
		}
		serviceable := candidate.Outcome == OutcomeServiceable
		candidates = append(candidates, candidate)
		events = append(events, DecisionEvent{
			OrchestrationID: current.ID, PlanVersion: current.PlanVersion,
			EventType: EventServiceabilityChecked, CandidateAuthority: authority,
			CandidateProduct: candidate.ProductCode, Serviceable: &serviceable,
			OutcomeClass: candidate.Outcome, ReasonCode: candidate.ReasonCode,
		})
	}

	selected, selectErr := SelectAuthority(candidates)

	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return current, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, event := range events {
		if selectErr == nil && event.CandidateAuthority == selected.Authority {
			event.Selected = true
		}
		if err = s.repo.RecordDecision(ctx, tx, event); err != nil {
			return current, err
		}
	}

	if selectErr != nil {
		// Nothing usable. The distinction between "cannot be carried" and
		// "could not ask" is preserved in the reason so an operator surface and
		// a retry policy can both act on it.
		reason := "NO_ELIGIBLE_AUTHORITY"
		state := StateOpsReview
		if errors.Is(selectErr, ErrAuthoritiesUnreachable) {
			reason = "AUTHORITIES_UNREACHABLE"
			// An outage is retried, not escalated: the goods are fine and the
			// network is not.
			state = StateWaitingReadiness
		}
		next := now.Add(time.Minute)
		updated, applyErr := s.repo.Apply(ctx, tx, current.ID, current.Version, Update{
			State: state, RecoveryState: current.RecoveryState,
			NextEvaluationAt: &next, ReasonCode: reason,
		}, now)
		if applyErr != nil {
			return current, applyErr
		}
		if err = tx.Commit(ctx); err != nil {
			return current, err
		}
		return updated, selectErr
	}

	if err = s.repo.RecordDecision(ctx, tx, DecisionEvent{
		OrchestrationID: current.ID, PlanVersion: current.PlanVersion,
		EventType: EventAuthoritySelected, CandidateAuthority: selected.Authority,
		CandidateProduct: selected.ProductCode, OutcomeClass: OutcomeServiceable,
		PolicyVersion: selected.PolicyVersion, Selected: true,
	}); err != nil {
		return current, err
	}
	updated, err := s.repo.Apply(ctx, tx, current.ID, current.Version, Update{
		State: StateWaitingReadiness, SelectedAuthority: selected.Authority,
		SelectedProductCode: selected.ProductCode, SelectionSource: SelectionAutomatic,
		CapabilityPolicyVersion: selected.PolicyVersion,
		RecoveryState:           current.RecoveryState, ReasonCode: "AUTHORITY_SELECTED",
	}, now)
	if err != nil {
		return current, err
	}
	if err = tx.Commit(ctx); err != nil {
		return current, err
	}

	// Group the pickups against the authority that was just selected.
	//
	// It runs after the selection commits, not inside it, because the two
	// answer different questions and fail differently: the selection is a
	// decision about the whole order and must be durable, while a grouping that
	// could not be computed leaves the orchestration correctly selected and
	// simply unplanned, to be planned on the next pass. Folding grouping into
	// the selection transaction would let a capability lookup roll back a
	// decision that was already sound.
	if groupErr := s.planGroups(ctx, updated); groupErr != nil {
		return updated, groupErr
	}
	return updated, nil
}

// timeCommit computes the commit window from the current readiness.
func (s *Service) timeCommit(ctx context.Context, current Orchestration, now time.Time) (CommitDecision, error) {
	input := CommitInput{Now: now, Deadline: current.Requirement.DeliveryDeadline}

	pickups, err := s.repo.Pickups(ctx, current.ID)
	if err != nil {
		return CommitDecision{}, err
	}
	// The group leaves when its slowest member is ready. Aiming at the
	// earliest would send a provider to wait for the others.
	for _, pickup := range pickups {
		if s.readiness == nil {
			break
		}
		state, readErr := s.readiness.Read(ctx, pickup.VendorOrderID)
		if readErr != nil {
			continue
		}
		if state.PredictedReadyStart != nil && state.PredictedReadyStart.After(input.PredictedReadyStart) {
			input.PredictedReadyStart = state.PredictedReadyStart.UTC()
			input.ConfidenceBand = state.ConfidenceBand
		}
		if state.PredictedReadyEnd != nil && state.PredictedReadyEnd.After(input.PredictedReadyEnd) {
			input.PredictedReadyEnd = state.PredictedReadyEnd.UTC()
		}
		if state.Stale(now, readiness.DefaultRules(state.VendorType).StaleSignal) {
			input.ReadinessStale = true
		}
	}
	return PlanCommit(input), nil
}

// warmMatch asks the selected authority about supply and records the answer.
//
// It creates nothing: no mission, no booking, no offer, no reservation. That is
// the contract's own guarantee, and it is what makes a pre-commitment delay
// free -- nobody was ever held.
func (s *Service) warmMatch(ctx context.Context, current Orchestration, now time.Time) error {
	if s.preview == nil || current.SelectedAuthority == "" {
		return nil
	}
	answer, err := s.preview.Preview(ctx, current.SelectedAuthority,
		current.Requirement.Preview(current.SelectedProductCode))
	if err != nil {
		// A preview that could not be taken is not a refusal. The commitment
		// timing simply proceeds on what it already knew.
		return nil
	}
	tx, txErr := s.repo.Pool().Begin(ctx)
	if txErr != nil {
		return txErr
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	serviceable := answer.Serviceable
	if err = s.repo.RecordDecision(ctx, tx, DecisionEvent{
		OrchestrationID: current.ID, PlanVersion: current.PlanVersion,
		EventType: EventSupplyPreviewed, CandidateAuthority: current.SelectedAuthority,
		CandidateProduct: answer.ProductCode, Serviceable: &serviceable,
		SupplyStatus: string(answer.SupplyStatus), ReasonCode: answer.ReasonCode,
		EstimatedPickupETASeconds: answer.EstimatedPickupETASeconds,
		PolicyVersion:             answer.PolicyVersion,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Commit creates the mission, exactly once.
//
// The sequence is the careful part. The orchestration is moved to COMMITTING
// and its requirement frozen BEFORE the authority is called, so a crash leaves
// a row that says "a create may be in flight" rather than one that says
// nothing. An ambiguous answer does NOT retry the create: it marks the
// orchestration for reconciliation, and the reconciler asks the authority what
// actually happened. Retrying instead is how one order gets two vehicles.
func (s *Service) Commit(ctx context.Context, id string) (Orchestration, error) {
	if !s.jitEnabled {
		return Orchestration{}, ErrDisabled
	}
	if s.committer == nil {
		return Orchestration{}, fmt.Errorf("%w: no mission committer is configured", ErrDisabled)
	}
	now := s.now().UTC()

	current, err := s.repo.Get(ctx, id)
	if err != nil {
		return Orchestration{}, err
	}
	if current.State.Committed() {
		// Already done. A second commit would be a second vehicle.
		return current, nil
	}
	if current.State != StateCommitDue {
		return current, fmt.Errorf("%w: %s is not ready to commit", ErrTransition, current.State)
	}

	pickups, err := s.repo.Pickups(ctx, current.ID)
	if err != nil {
		return current, err
	}

	committing, err := s.transition(ctx, current, Update{
		State: StateCommitting, RecoveryState: current.RecoveryState,
		ReasonCode: "COMMIT_ATTEMPTED", FreezeRequirements: true,
	}, now)
	if err != nil {
		return current, err
	}

	outcome, commitErr := s.committer.CreateMission(ctx, committing, pickups)

	if commitErr != nil || !outcome.Definite {
		// The create may or may not have landed. Asking is the only safe next
		// step, so the orchestration is parked for reconciliation rather than
		// retried.
		return s.markReconciling(ctx, committing, now)
	}

	if !outcome.Accepted {
		// A definite refusal. The authority answered and said no, so a replan
		// is legitimate: the selection is cleared and the next evaluation
		// chooses again.
		next := now.Add(30 * time.Second)
		return s.transition(ctx, committing, Update{
			State: StateCommitDue, RecoveryState: RecoveryReplanning,
			NextEvaluationAt: &next, ReasonCode: outcome.RefusalCode,
		}, now)
	}

	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return committing, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err = s.repo.RecordDecision(ctx, tx, DecisionEvent{
		OrchestrationID: committing.ID, PlanVersion: committing.PlanVersion,
		EventType: EventMissionCommitted, CandidateAuthority: committing.SelectedAuthority,
		CandidateProduct: committing.SelectedProductCode, OutcomeClass: OutcomeServiceable,
		Selected: true,
	}); err != nil {
		return committing, err
	}
	committed, err := s.repo.Apply(ctx, tx, committing.ID, committing.Version, Update{
		State: StateCommitted, CurrentMissionRef: outcome.MissionRef,
		RecoveryState: RecoveryNone, ReasonCode: "MISSION_COMMITTED",
	}, now)
	if err != nil {
		return committing, err
	}
	if err = tx.Commit(ctx); err != nil {
		return committing, err
	}
	return committed, nil
}

// Reconcile resolves an orchestration whose mission outcome is unknown.
//
// It asks the authority whether the deterministic mission reference exists.
// Only a definite "no" returns the orchestration to the commit queue; anything
// else leaves it reconciling, because the one thing worse than a stuck order is
// a duplicate vehicle.
func (s *Service) Reconcile(ctx context.Context, id string) (Orchestration, error) {
	if s.committer == nil {
		return Orchestration{}, fmt.Errorf("%w: no mission committer is configured", ErrDisabled)
	}
	now := s.now().UTC()
	current, err := s.repo.Get(ctx, id)
	if err != nil {
		return Orchestration{}, err
	}
	if current.RecoveryState != RecoveryReconciling {
		return current, nil
	}

	missionRef := current.CurrentMissionRef
	if missionRef == "" {
		missionRef = MissionRefFor(current)
	}
	outcome, found, readErr := s.committer.ReadMission(ctx, current.SelectedAuthority, missionRef)
	if readErr != nil {
		// Still cannot tell. Leaving it reconciling is correct: an unanswered
		// question is not an answer, and guessing here dispatches twice.
		return current, nil
	}

	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return current, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err = s.repo.RecordDecision(ctx, tx, DecisionEvent{
		OrchestrationID: current.ID, PlanVersion: current.PlanVersion,
		EventType: EventRecoveryObserved, CandidateAuthority: current.SelectedAuthority,
		ReasonCode: "READ_BACK", Selected: found,
	}); err != nil {
		return current, err
	}

	update := Update{RecoveryState: RecoveryNone}
	switch {
	case found && outcome.Accepted:
		update.State = StateCommitted
		update.CurrentMissionRef = outcome.MissionRef
		update.ReasonCode = "RECONCILED_COMMITTED"
	default:
		// Definitely not there. One more attempt is safe precisely because the
		// authority has confirmed nothing exists.
		next := now.Add(15 * time.Second)
		update.State = StateCommitDue
		update.NextEvaluationAt = &next
		update.ReasonCode = "RECONCILED_ABSENT"
	}
	reconciled, err := s.repo.Apply(ctx, tx, current.ID, current.Version, update, now)
	if err != nil {
		return current, err
	}
	if err = tx.Commit(ctx); err != nil {
		return current, err
	}
	return reconciled, nil
}

// MissionRefFor is the deterministic mission identity for an orchestration.
//
// It is derived rather than generated so a read-back after an ambiguous create
// can ask about the same reference the create used. A random reference would
// make the mission unfindable exactly when finding it matters.
func MissionRefFor(o Orchestration) string {
	return fmt.Sprintf("commerce-orc:%s:%d", o.FulfilmentGroupRef, o.PlanVersion)
}

// ObserveAuthorityRecovery records that the executing authority is repairing
// its own provider assignment.
//
// Vendor does not intervene. Replacing a provider is the authority's decision
// and Vendor has no way to make it; what Vendor does is project the fact so a
// multi-mission order can be re-timed around it.
func (s *Service) ObserveAuthorityRecovery(ctx context.Context, id, reasonCode string) (Orchestration, error) {
	now := s.now().UTC()
	current, err := s.repo.Get(ctx, id)
	if err != nil {
		return Orchestration{}, err
	}
	if !current.State.Committed() {
		return current, fmt.Errorf("%w: %s has no authority to recover it", ErrTransition, current.State)
	}
	return s.transition(ctx, current, Update{
		State: StateRecovering, RecoveryState: RecoveryAuthority, ReasonCode: reasonCode,
	}, now)
}

// markReconciling parks an orchestration whose commit outcome is unknown.
func (s *Service) markReconciling(ctx context.Context, current Orchestration, now time.Time) (Orchestration, error) {
	next := now.Add(10 * time.Second)
	return s.transition(ctx, current, Update{
		State: StateCommitting, RecoveryState: RecoveryReconciling,
		NextEvaluationAt: &next, ReasonCode: "COMMIT_OUTCOME_UNKNOWN",
	}, now)
}

// park moves an orchestration to a waiting state with a reason.
func (s *Service) park(ctx context.Context, current Orchestration, state State,
	reason string, now time.Time) (Orchestration, error) {
	next := now.Add(time.Minute)
	return s.transition(ctx, current, Update{
		State: state, RecoveryState: current.RecoveryState,
		NextEvaluationAt: &next, ReasonCode: reason,
	}, now)
}

// transition applies a guarded state change, refusing a move the machine does
// not allow.
func (s *Service) transition(ctx context.Context, current Orchestration,
	update Update, now time.Time) (Orchestration, error) {
	if !CanTransition(current.State, update.State) {
		return current, fmt.Errorf("%w: %s cannot move to %s",
			ErrTransition, current.State, update.State)
	}
	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return current, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	updated, err := s.repo.Apply(ctx, tx, current.ID, current.Version, update, now)
	if err != nil {
		return current, err
	}
	if err = tx.Commit(ctx); err != nil {
		return current, err
	}
	return updated, nil
}

func timePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copied := value.UTC()
	return &copied
}

// OverrideRequest replaces the automatically selected authority.
type OverrideRequest struct {
	GroupRef    string
	Authority   logistics.Authority
	ProductCode string
	Reason      string
	ActorRef    string
}

// Override records an authorized operator's choice of authority.
//
// It is refused once the work is committed. At that point an execution
// authority already holds the mission, and changing Vendor's record would only
// make the two disagree about who is carrying the goods -- the operator's real
// remedy then is to cancel through the authority, not to edit a plan.
//
// It is also refused against a confirmed physical incompatibility. An override
// may overrule a preference; it may not overrule a vehicle that cannot hold the
// load, because the vehicle will still not hold it.
func (s *Service) Override(ctx context.Context, request OverrideRequest) (Orchestration, error) {
	if !s.jitEnabled {
		return Orchestration{}, ErrDisabled
	}
	if strings.TrimSpace(request.Reason) == "" {
		return Orchestration{}, fmt.Errorf("%w: an override reason is required", ErrValidation)
	}
	if !request.Authority.PlatformExecuted() {
		return Orchestration{}, fmt.Errorf("%w: %q is not a platform execution authority",
			ErrValidation, request.Authority)
	}

	now := s.now().UTC()
	current, err := s.repo.GetByGroup(ctx, request.GroupRef)
	if err != nil {
		return Orchestration{}, err
	}
	if current.State.Committed() {
		return current, ErrCommitted
	}
	if s.gate != nil && !s.gate.Usable(request.Authority) {
		return current, fmt.Errorf("%w: %s is not usable in this deployment",
			ErrValidation, request.Authority)
	}

	// The authority's own answer still binds. An operator may prefer a
	// different service; they may not decide that a load which does not fit
	// will fit.
	if s.gate != nil {
		answer, checkErr := s.gate.Serviceability(ctx, request.Authority,
			current.Requirement.Capability())
		if checkErr == nil && !answer.Serviceable {
			outcome := ClassifyFailure(
				logistics.CategoryForReasonCode(answer.ReasonCode), answer.ReasonCode)
			if outcome.PositiveIncompatibility() {
				return current, fmt.Errorf("%w: %s refused this load (%s)",
					ErrValidation, request.Authority, answer.ReasonCode)
			}
		}
	}

	tx, err := s.repo.Pool().Begin(ctx)
	if err != nil {
		return current, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// The evidence records the decision that WAS made automatically alongside
	// the one replacing it, so the two can be compared afterwards.
	if err = s.repo.RecordDecision(ctx, tx, DecisionEvent{
		OrchestrationID: current.ID, PlanVersion: current.PlanVersion,
		EventType: EventOverridden, CandidateAuthority: request.Authority,
		CandidateProduct: request.ProductCode, Selected: true,
		ActorRef: request.ActorRef, OverrideReason: request.Reason,
		ReasonCode: "OVERRIDDEN_FROM_" + string(current.SelectedAuthority),
	}); err != nil {
		return current, err
	}

	next := now
	updated, err := s.repo.Apply(ctx, tx, current.ID, current.Version, Update{
		State: current.State, SelectedAuthority: request.Authority,
		SelectedProductCode: request.ProductCode, SelectionSource: SelectionAdminOverride,
		NextEvaluationAt: &next, RecoveryState: current.RecoveryState,
		ReasonCode: "AUTHORITY_OVERRIDDEN",
	}, now)
	if err != nil {
		return current, err
	}
	if err = tx.Commit(ctx); err != nil {
		return current, err
	}
	return updated, nil
}
