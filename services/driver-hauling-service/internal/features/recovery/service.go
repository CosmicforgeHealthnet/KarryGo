package recovery

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/hauling-service/internal/cargofit"
	"cosmicforge/logistics/services/hauling-service/internal/observability"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/capabilityfit"
	"cosmicforge/logistics/shared/go/custodyops"
	"cosmicforge/logistics/shared/go/geo"
	"cosmicforge/logistics/shared/go/logging"
)

// CandidateSource returns providers who could take over, nearest first.
//
// It is the availability index, narrowed. Recovery deliberately does NOT get
// the matcher's whole candidate pipeline: reusing that would drag in the
// booking usecase and its rejection bookkeeping, which is about an offer that
// was declined rather than a provider who broke down.
type CandidateSource interface {
	NearbyProviders(ctx context.Context, center geo.Point, radiusKm float64, limit int) ([]NearbyProvider, error)
}

// NearbyProvider is one anonymous candidate.
type NearbyProvider struct {
	ProviderID string
	DistanceKm float64
}

// CapabilitySource is Hauling's capability store, seen from recovery.
//
// It is the SAME interface shape the matcher's authority satisfies, and the
// implementation behind it is the same truckcapability.Service. That is what
// makes "a replacement is as eligible as the original" true by construction
// rather than by review.
type CapabilitySource interface {
	LoadProviderTruckFits(ctx context.Context, providerIDs []string, requirement cargofit.Requirement) (map[string][]cargofit.TruckCapability, error)
	ProductForRequirement(requirement cargofit.Requirement) (productCode string, certificationRequired bool)
}

// CustodyAuthority moves manifests between providers inside a caller-owned
// transaction.
type CustodyAuthority interface {
	AssignManifestReplacementTx(ctx context.Context, tx pgx.Tx, command custodyops.AssignReplacementCommand) (custodyops.Result, error)
	TransferManifestsTx(ctx context.Context, tx pgx.Tx, command custodyops.TransferManifestsCommand) (custodyops.Result, error)
}

// EventSink publishes recovery facts through the service's transactional
// outbox.
type EventSink interface {
	PublishTx(ctx context.Context, tx pgx.Tx, event Event) error
}

// Options configure the service.
type Options struct {
	Repository *Repository
	Candidates CandidateSource
	Capability CapabilitySource
	Custody    CustodyAuthority
	Events     EventSink
	// MaxRadiusKm bounds the replacement search. It defaults to the matcher's
	// own radius so a truck that was never a candidate for this booking does
	// not become one merely because the first provider failed.
	MaxRadiusKm float64
	Enabled     bool
	Now         func() time.Time
}

// Service owns recovery.
type Service struct {
	repo       *Repository
	candidates CandidateSource
	capability CapabilitySource
	custody    CustodyAuthority
	events     EventSink
	radiusKm   float64
	enabled    bool
	now        func() time.Time
}

// New builds the service.
func New(options Options) *Service {
	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	radius := options.MaxRadiusKm
	if radius <= 0 {
		radius = 25
	}
	return &Service{
		repo: options.Repository, candidates: options.Candidates,
		capability: options.Capability, custody: options.Custody, events: options.Events,
		radiusKm: radius, enabled: options.Enabled, now: now,
	}
}

// Enabled reports whether recovery is switched on for this deployment.
func (s *Service) Enabled() bool { return s != nil && s.enabled && s.repo != nil }

const candidateLimit = 50

// Start opens a recovery for a booking whose provider can no longer finish.
//
// It is idempotent on operation_ref, and the schema's open-case uniqueness
// covers the harder case: two DIFFERENT reports of the same failure. Without
// that, both would search and each bind a different replacement to one load.
func (s *Service) Start(ctx context.Context, command StartCommand) (Public, error) {
	if !s.Enabled() {
		return Public{}, apperrors.Unavailable("Recovery is not enabled.", nil)
	}
	if !ValidTrigger(command.TriggerCode) {
		return Public{}, apperrors.Validation("Unknown recovery trigger.", []apperrors.FieldViolation{
			{Field: "trigger_code", Message: "Trigger is not one this service recognises."},
		})
	}
	if command.OperationRef == "" {
		return Public{}, apperrors.Validation("Operation reference is required.", []apperrors.FieldViolation{
			{Field: "operation_ref", Message: "An idempotency key is required."},
		})
	}

	booking, err := s.repo.LoadBooking(ctx, command.BookingID)
	if errors.Is(err, ErrNotFound) {
		return Public{}, apperrors.NotFound("Booking not found.", err)
	}
	if err != nil {
		return Public{}, apperrors.Internal("Booking could not be read.", err)
	}
	if !recoverable(booking.Status) {
		// A booking nobody is executing has nothing to recover, and one that
		// already completed must not be reopened: a replacement bound to a
		// finished job would be paid for work already settled.
		return Public{}, apperrors.Conflict(
			fmt.Sprintf("A booking in %s cannot enter recovery.", booking.Status), nil)
	}

	opened, created, err := s.repo.Open(ctx, command, booking, s.now())
	if errors.Is(err, ErrNotFound) {
		return Public{}, apperrors.Conflict("This booking already has an open recovery.", err)
	}
	if err != nil {
		// An open-case uniqueness violation lands here: a second, different
		// report for a booking already under recovery.
		return Public{}, apperrors.Conflict("This booking already has an open recovery.", err)
	}
	if !created {
		// A retry of the same command. Returning the original case is what
		// makes the endpoint safe to call from an at-least-once pipeline.
		return opened.Public(), nil
	}

	observability.RecoveryOutcome("STARTED")
	s.publish(ctx, nil, EventRecoveryStarted, opened, map[string]any{
		"trigger_code": opened.TriggerCode, "custody_transfer_required": opened.CustodyTransferRequired,
	})
	// A provider failing mid-job is a broken service guarantee whether or not
	// a replacement is found, so the breach is reported at detection rather
	// than only on failure. Reporting it later would understate every recovery
	// that happened to succeed.
	s.publish(ctx, nil, EventGuaranteeBreached, opened, map[string]any{
		"reason_code": opened.TriggerCode,
	})
	return opened.Public(), nil
}

// recoverable reports whether a booking is in a state recovery can act on.
func recoverable(status string) bool {
	switch status {
	case "accepted", "en_route_pickup", "arrived_at_pickup", "picked_up", "en_route_delivery":
		return true
	default:
		return false
	}
}

// SelectReplacement finds a provider who can actually carry this cargo.
//
// Every candidate goes through cargofit.EvaluateProvider -- the same call the
// original match used. The three failure modes stay distinct all the way out:
// a store outage is DEPENDENCY_UNAVAILABLE, an unsurveyed truck is
// CAPABILITY_UNKNOWN, and a truck that provably cannot carry the load is
// REQUIRED_CAPABILITY_UNAVAILABLE. Only the last is a permanent fact about a
// vehicle, and an operator triaging a stuck load needs to know which they have.
func (s *Service) SelectReplacement(ctx context.Context, bookingID, actor string) (Public, error) {
	if !s.Enabled() {
		return Public{}, apperrors.Unavailable("Recovery is not enabled.", nil)
	}
	current, err := s.repo.OpenForBooking(ctx, bookingID)
	if errors.Is(err, ErrNotFound) {
		return Public{}, apperrors.NotFound("No open recovery for this booking.", err)
	}
	if err != nil {
		return Public{}, apperrors.Internal("Recovery could not be read.", err)
	}
	// Already chosen. Returning the case rather than searching again is what
	// stops a retried command binding a second replacement.
	if current.ReplacementProviderID != "" {
		return current.Public(), nil
	}

	booking, err := s.repo.LoadBooking(ctx, bookingID)
	if err != nil {
		return Public{}, apperrors.Internal("Booking could not be read.", err)
	}

	searching, err := s.advance(ctx, AdvanceInput{
		CaseID: current.ID, State: StateReplacementSearch, Actor: actor,
		ExpectedVersion: current.Version,
	})
	if err != nil {
		return Public{}, err
	}

	requirement := requirementFor(booking)
	requirement.ProductCode, requirement.CertificationRequired = s.capability.ProductForRequirement(requirement)

	nearby, err := s.candidates.NearbyProviders(ctx,
		geo.Point{Lat: booking.PickupLat, Lng: booking.PickupLng}, s.radiusKm, candidateLimit)
	if err != nil {
		// The index did not answer. That is not "no trucks exist"; parking the
		// case for an operator is better than recording a permanent failure
		// against a fleet that may be perfectly able to help.
		return s.toOps(ctx, searching, capabilityfit.ReasonDependencyUnavailable, actor)
	}

	eligibleFrom := make([]string, 0, len(nearby))
	distance := make(map[string]float64, len(nearby))
	for _, candidate := range nearby {
		// The failed provider is never their own replacement. The schema
		// refuses it too; excluding here means the search does not waste its
		// one best candidate slot on a truck that is already broken down.
		if candidate.ProviderID == booking.ProviderID || candidate.ProviderID == current.OriginalProviderID {
			continue
		}
		eligibleFrom = append(eligibleFrom, candidate.ProviderID)
		distance[candidate.ProviderID] = candidate.DistanceKm
	}
	if len(eligibleFrom) == 0 {
		return s.toOps(ctx, searching, capabilityfit.ReasonSupplyCurrentlyUnavailable, actor)
	}

	byProvider, err := s.capability.LoadProviderTruckFits(ctx, eligibleFrom, requirement)
	if err != nil {
		// Same reasoning as the index failure: unknown, not incapable.
		return s.toOps(ctx, searching, capabilityfit.ReasonDependencyUnavailable, actor)
	}

	sort.Slice(eligibleFrom, func(i, j int) bool {
		return distance[eligibleFrom[i]] < distance[eligibleFrom[j]]
	})

	firstRefusal := ""
	for _, providerID := range eligibleFrom {
		truck, decision := cargofit.EvaluateProvider(
			byProvider[providerID], cargofit.ResolutionFound, requirement)
		if !decision.Eligible {
			if firstRefusal == "" {
				firstRefusal = decision.PrimaryReason()
			}
			observability.CargoFitOutcome(decision.PrimaryReason(), false)
			continue
		}
		return s.bindReplacement(ctx, searching, booking, providerID, truck.TruckID, actor)
	}

	// Candidates existed and none fitted. Never fall back to the nearest
	// truck: an incompatible truck that keeps the mission moving is how cargo
	// arrives damaged, and a stuck load an operator can see is strictly better
	// than a moving load nobody checked.
	reason := firstRefusal
	if reason == "" {
		reason = capabilityfit.ReasonNoEligibleProvider
	}
	return s.toOps(ctx, searching, reason, actor)
}

// bindReplacement commits the replacement, and -- when custody has already
// moved -- the custody transfer, in ONE transaction.
//
// Splitting them would leave a window in which the ledger says the new
// provider holds the cargo and the booking says the old one does. A crash in
// that window leaves it there permanently, with no way for either side to tell
// which is right.
func (s *Service) bindReplacement(
	ctx context.Context, current Case, booking BookingSnapshot,
	providerID, truckID, actor string,
) (Public, error) {
	if current.CustodyTransferRequired {
		// The cargo is already in the failed provider's truck. The
		// replacement is chosen, but nothing moves until the handover is
		// actually performed and confirmed by both sides.
		advanced, err := s.advance(ctx, AdvanceInput{
			CaseID: current.ID, State: StateTransferRequired,
			ReplacementProviderID: providerID, ReplacementTruckID: truckID,
			ExpectedVersion: current.Version, Actor: actor,
		})
		if err != nil {
			return Public{}, err
		}
		s.publish(ctx, nil, EventRecoveryProviderReplaced, advanced, map[string]any{
			"custody_transfer_required": true,
		})
		return advanced.Public(), nil
	}

	// No custody to move: the failed provider never collected. The booking is
	// rebound and the replacement drives.
	transaction, err := s.repo.DB().BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Public{}, apperrors.Internal("Replacement could not be bound.", err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()

	if err := s.repo.RebindProviderTx(ctx, transaction, booking.ID,
		current.OriginalProviderID, providerID, truckID); err != nil {
		if errors.Is(err, ErrConflict) {
			// The original provider completed, cancelled or was replaced
			// between the search and this write. Refusing is the only safe
			// answer: overwriting would produce two providers who each believe
			// they hold this job.
			return Public{}, apperrors.Conflict(
				"This booking changed while a replacement was being selected.", err)
		}
		return Public{}, apperrors.Internal("Replacement could not be bound.", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return Public{}, apperrors.Internal("Replacement could not be bound.", err)
	}

	advanced, err := s.advance(ctx, AdvanceInput{
		CaseID: current.ID, State: StateInProgress,
		ReplacementProviderID: providerID, ReplacementTruckID: truckID,
		ExpectedVersion: current.Version, Actor: actor,
	})
	if err != nil {
		return Public{}, err
	}
	observability.RecoveryOutcome("REPLACED")
	s.publish(ctx, nil, EventRecoveryProviderReplaced, advanced, map[string]any{
		"custody_transfer_required": false,
	})
	return advanced.Public(), nil
}

// CompleteTransfer performs the controlled custody handover and rebinds the
// booking, atomically.
func (s *Service) CompleteTransfer(ctx context.Context, command TransferCommand) (Public, error) {
	if !s.Enabled() {
		return Public{}, apperrors.Unavailable("Recovery is not enabled.", nil)
	}
	current, err := s.repo.OpenForBooking(ctx, command.BookingID)
	if errors.Is(err, ErrNotFound) {
		return Public{}, apperrors.NotFound("No open recovery for this booking.", err)
	}
	if err != nil {
		return Public{}, apperrors.Internal("Recovery could not be read.", err)
	}
	if current.State != StateTransferRequired {
		return Public{}, apperrors.Conflict(
			"This recovery is not awaiting a custody transfer.", nil)
	}
	if !command.OldProviderConfirmed || !command.NewProviderConfirmed {
		// A handover one side did not confirm is not a handover. Recording it
		// as one would leave the ledger asserting the cargo moved while it may
		// still be in the first truck.
		return Public{}, apperrors.Validation("Both providers must confirm the handover.",
			[]apperrors.FieldViolation{
				{Field: "old_provider_confirmed", Message: "Both sides must confirm."},
				{Field: "new_provider_confirmed", Message: "Both sides must confirm."},
			})
	}
	if command.ExpectedVersion != 0 && command.ExpectedVersion != current.Version {
		return Public{}, apperrors.Conflict("This recovery changed. Reload and try again.", nil)
	}

	booking, err := s.repo.LoadBooking(ctx, command.BookingID)
	if err != nil {
		return Public{}, apperrors.Internal("Booking could not be read.", err)
	}

	transaction, err := s.repo.DB().BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Public{}, apperrors.Internal("Transfer could not be completed.", err)
	}
	defer func() { _ = transaction.Rollback(ctx) }()

	// The custody ledger and the booking move together. Either both say the
	// replacement holds the cargo or neither does.
	if s.custody != nil && booking.MissionRef != "" {
		if _, err := s.custody.TransferManifestsTx(ctx, transaction, custodyops.TransferManifestsCommand{
			OperationRef:            "recovery-transfer:" + command.OperationRef,
			CorrelationID:           current.CorrelationID,
			MissionRef:              booking.MissionRef,
			TransferRef:             command.TransferRef,
			OldProviderRef:          current.OriginalProviderID,
			NewProviderRef:          current.ReplacementProviderID,
			EvidenceRef:             command.EvidenceRef,
			OldProviderConfirm:      command.OldProviderConfirmed,
			NewProviderConfirm:      command.NewProviderConfirmed,
			NewReceiverChallengeRef: command.TransferRef,
		}); err != nil {
			if errors.Is(err, custodyops.ErrConflict) {
				return Public{}, apperrors.Conflict("This transfer was already recorded differently.", err)
			}
			return Public{}, apperrors.Internal("Custody transfer failed.", err)
		}
	}

	if err := s.repo.RebindProviderTx(ctx, transaction, booking.ID,
		current.OriginalProviderID, current.ReplacementProviderID, current.ReplacementTruckID); err != nil {
		if errors.Is(err, ErrConflict) {
			return Public{}, apperrors.Conflict(
				"This booking changed while the transfer was being completed.", err)
		}
		return Public{}, apperrors.Internal("Replacement could not be bound.", err)
	}

	if err := transaction.Commit(ctx); err != nil {
		return Public{}, apperrors.Internal("Transfer could not be completed.", err)
	}

	advanced, err := s.advance(ctx, AdvanceInput{
		CaseID: current.ID, State: StateInProgress,
		CustodyTransferRef: command.TransferRef, EvidenceRef: command.EvidenceRef,
		ExpectedVersion: current.Version, Actor: command.Actor,
	})
	if err != nil {
		return Public{}, err
	}
	observability.RecoveryOutcome("REPLACED")
	s.publish(ctx, nil, EventRecoveryProviderReplaced, advanced, map[string]any{
		"custody_transfer_required": true, "transfer_ref": command.TransferRef,
	})
	return advanced.Public(), nil
}

// Resolve closes a recovery whose replacement is executing.
func (s *Service) Resolve(ctx context.Context, command ResolveCommand) (Public, error) {
	if !s.Enabled() {
		return Public{}, apperrors.Unavailable("Recovery is not enabled.", nil)
	}
	current, err := s.repo.OpenForBooking(ctx, command.BookingID)
	if errors.Is(err, ErrNotFound) {
		return Public{}, apperrors.NotFound("No open recovery for this booking.", err)
	}
	if err != nil {
		return Public{}, apperrors.Internal("Recovery could not be read.", err)
	}
	resolved, err := s.advance(ctx, AdvanceInput{
		CaseID: current.ID, State: StateResolved,
		ExpectedVersion: command.ExpectedVersion, Actor: command.Actor,
	})
	if err != nil {
		return Public{}, err
	}
	observability.RecoveryOutcome("RESOLVED")
	if resolved.StartedAt != nil {
		observability.RecoveryDuration(resolved.ResolvedAt.Sub(resolved.DetectedAt).Seconds())
	}
	s.publish(ctx, nil, EventRecoveryResolved, resolved, nil)
	return resolved.Public(), nil
}

// Get reads a booking's open recovery.
func (s *Service) Get(ctx context.Context, bookingID string) (Public, error) {
	current, err := s.repo.OpenForBooking(ctx, bookingID)
	if errors.Is(err, ErrNotFound) {
		return Public{}, apperrors.NotFound("No open recovery for this booking.", err)
	}
	if err != nil {
		return Public{}, apperrors.Internal("Recovery could not be read.", err)
	}
	return current.Public(), nil
}

// toOps parks a case for a human.
//
// This is the honest outcome when no safe automatic recovery exists. It is
// explicitly NOT a failure: the work is still live and the cargo still
// somewhere, and the alternative -- assigning an incompatible truck to keep
// the mission moving -- is the one thing Section 9 forbids.
func (s *Service) toOps(ctx context.Context, current Case, reason, actor string) (Public, error) {
	parked, err := s.advance(ctx, AdvanceInput{
		CaseID: current.ID, State: StateOpsRequired, ReasonCode: reason,
		ExpectedVersion: current.Version, Actor: actor,
	})
	if err != nil {
		return Public{}, err
	}
	observability.RecoveryOutcome("OPERATOR_REQUIRED")
	s.publish(ctx, nil, EventCapabilityMismatch, parked, map[string]any{"reason_code": reason})
	logging.Notice("hauling-recovery",
		"booking %s needs an operator: no compatible replacement (%s)", parked.BookingID, reason)
	return parked.Public(), nil
}

func (s *Service) advance(ctx context.Context, input AdvanceInput) (Case, error) {
	advanced, err := s.repo.Advance(ctx, input, s.now())
	switch {
	case errors.Is(err, ErrConflict):
		return Case{}, apperrors.Conflict("This recovery changed. Reload and try again.", err)
	case errors.Is(err, ErrNotFound):
		return Case{}, apperrors.NotFound("Recovery not found.", err)
	case err != nil:
		return Case{}, apperrors.Internal("Recovery could not be updated.", err)
	}
	return advanced, nil
}

// requirementFor builds the cargo requirement from the booking.
//
// It mirrors the matcher's own derivation exactly, including the inference
// that a fragile load must stay upright. A replacement evaluated against a
// LOOSER requirement than the original match would be the subtlest possible
// way to put cargo in a truck that should never have had it.
func requirementFor(booking BookingSnapshot) cargofit.Requirement {
	requirement := cargofit.Requirement{
		WeightGrams:        cargofit.FromKilograms(booking.CargoWeightKg),
		PreferredTruckType: booking.PreferredTruckType,
		Fragile:            booking.IsFragile,
	}
	if booking.IsFragile {
		requirement.UprightRequired = true
	}
	return requirement
}
