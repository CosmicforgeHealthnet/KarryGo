package assignment

import (
	"context"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/observability"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrConflict   = errors.New("assignment conflict")
	ErrNotFound   = errors.New("assignment not found")
	ErrExpired    = errors.New("assignment offer expired")
	ErrStaleQuote = errors.New("quote is stale; explicit requote is required")
)

type Store interface {
	Start(context.Context, StartRequest, Strategy, []Candidate, time.Duration, string, bool) (Assignment, error)
	ListProviderOffers(context.Context, string) ([]Offer, error)
	Accept(context.Context, string, string, Decision, time.Time) (Assignment, error)
	OfferAuthorityContext(context.Context, string, string) (OfferAuthorityContext, error)
	Decline(context.Context, string, string, Decision, time.Time, time.Duration) (Assignment, error)
	ExpireOffers(context.Context, time.Time, time.Duration, int) (int, error)
	ApplyQueueOption(context.Context, string, string, QueueOption, time.Time) (QueueEntry, error)
	ApplyPartnerQueueOption(context.Context, string, string, int64, QueueOption, time.Time) (QueueEntry, error)
	ProcessQueue(context.Context, time.Time, time.Duration, time.Duration, int) (int, error)
	ListDueQueueMissions(context.Context, time.Time, int) ([]string, error)
	ListPendingQueueFinancial(context.Context, int) ([]PendingQueueFinancial, error)
	MarkQueueFinancial(context.Context, string, string, string) error
	AddCandidates(context.Context, string, []Candidate) error
	ListAcceptedWithoutTrip(context.Context, int) ([]PendingWinner, error)
}

type TripEnsurer interface {
	EnsureTrip(context.Context, string, string) (string, error)
}
type CandidateFinder interface {
	Discover(context.Context, string, bool) ([]Candidate, error)
	Validate(context.Context, string, string) error
}
type OrganizationAuthority interface {
	ValidateMembership(context.Context, string, string) error
}
type QueueFinancialAuthority interface {
	ReleaseForNoProvider(context.Context, string, string) (string, error)
}

type queueFinancialReconciler interface {
	ClaimPendingQueueFinancial(context.Context, time.Time, string, time.Duration, int, int) ([]PendingQueueFinancial, error)
	CompleteQueueFinancial(context.Context, string, string, string, time.Time, time.Duration, int, string) error
}

type Service struct {
	store         Store
	trips         TripEnsurer
	policy        Policy
	now           func() time.Time
	finder        CandidateFinder
	organizations OrganizationAuthority
	financial     QueueFinancialAuthority
	workerOwner   string
}

func (s *Service) WithCandidateFinder(f CandidateFinder) *Service { s.finder = f; return s }
func (s *Service) WithOrganizationAuthority(a OrganizationAuthority) *Service {
	s.organizations = a
	return s
}
func (s *Service) WithQueueFinancialAuthority(a QueueFinancialAuthority) *Service {
	s.financial = a
	return s
}

func New(store Store, trips TripEnsurer, policy Policy) *Service {
	return &Service{store: store, trips: trips, policy: normalizePolicy(policy), workerOwner: "assignment-" + uuid.NewString(), now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) Start(ctx context.Context, in StartRequest, adminAuthorized bool) (Assignment, error) {
	if strings.TrimSpace(in.MissionID) == "" || strings.TrimSpace(in.IdempotencyKey) == "" {
		return Assignment{}, errors.New("mission and idempotency key are required")
	}
	strategy, err := s.policy.Select(in.Strategy, in.OrganizationID, adminAuthorized)
	if err != nil {
		return Assignment{}, err
	}
	if strategy == SequentialNearest && !s.policy.SequentialSelected(in.ServiceTier) {
		strategy = BroadcastFirstAccept
	}
	if strategy == BroadcastFirstAccept {
		return s.store.Start(ctx, in, strategy, nil, s.policy.OfferWindow, PolicyVersion, s.policy.QueueEnabled)
	}
	priority, reason, version := s.policy.EffectivePriority(in)
	in.Priority, in.PriorityReason, in.PriorityPolicyVersion = priority, reason, version
	in.InitialRadiusKM, in.MaxRadiusKM = s.policy.InitialRadiusKM, s.policy.MaxRadiusKM
	if reader, ok := s.store.(interface {
		ReadStart(context.Context, StartRequest, Strategy) (Assignment, bool, error)
	}); ok {
		if existing, found, readErr := reader.ReadStart(ctx, in, strategy); readErr != nil || found {
			return existing, readErr
		}
	}
	if strategy == SequentialNearest {
		if in.QuoteExpiresAt != nil && !in.QuoteExpiresAt.After(s.now()) {
			return Assignment{}, ErrStaleQuote
		}
		if s.finder == nil {
			return Assignment{}, errors.New("authoritative candidate discovery is unavailable")
		}
		discovered, discoverErr := s.finder.Discover(ctx, in.MissionID, false)
		if discoverErr != nil {
			return Assignment{}, discoverErr
		}
		candidates, rankErr := RankCandidates(discovered)
		if rankErr != nil {
			return Assignment{}, errors.New("sequential assignment candidates are invalid")
		}
		return s.store.Start(ctx, in, strategy, candidates, s.policy.OfferWindow, PolicyVersion, s.policy.QueueEnabled)
	}
	if strings.TrimSpace(in.ProviderID) == "" || s.finder == nil {
		return Assignment{}, errors.New("target provider and authoritative eligibility are required")
	}
	if strategy == OrganizationDispatch {
		if s.organizations == nil {
			return Assignment{}, errors.New("organization authority unavailable")
		}
		if err = s.organizations.ValidateMembership(ctx, in.OrganizationID, in.ProviderID); err != nil {
			return Assignment{}, err
		}
	}
	if strategy == AdminAssisted && (strings.TrimSpace(in.ActorRef) == "" || strings.TrimSpace(in.ReasonCode) == "" || in.ExpectedMissionVersion < 1) {
		return Assignment{}, errors.New("trusted actor and reason code are required")
	}
	if strategy == AdminAssisted {
		allowedReason := map[string]bool{"RECOVERY": true, "SAFETY_REASSIGNMENT": true, "OPERATIONS_ESCALATION": true, "PROVIDER_SUPPORT": true}
		in.ReasonCode = strings.ToUpper(strings.TrimSpace(in.ReasonCode))
		if !allowedReason[in.ReasonCode] || len(in.Note) > 500 {
			return Assignment{}, errors.New("controlled admin reason or note is invalid")
		}
	}
	if err = s.finder.Validate(ctx, in.MissionID, in.ProviderID); err != nil {
		return Assignment{}, err
	}
	target := []Candidate{{ProviderID: in.ProviderID, Rank: 1, DistanceM: 0, Snapshot: map[string]any{"authority": string(strategy)}}}
	return s.store.Start(ctx, in, strategy, target, s.policy.OfferWindow, PolicyVersion, s.policy.QueueEnabled)
}

func (s *Service) ListProviderOffers(ctx context.Context, providerID string) ([]Offer, error) {
	return s.store.ListProviderOffers(ctx, providerID)
}

func (s *Service) Accept(ctx context.Context, offerID, providerID string, decision Decision) (Assignment, error) {
	if strings.TrimSpace(decision.IdempotencyKey) == "" || decision.ExpectedVersion < 1 {
		return Assignment{}, errors.New("idempotency key and expected version are required")
	}
	// The durable candidate snapshot is not current availability authority.
	// Store.Accept resolves the mission and repeats current database/live
	// eligibility while holding the offer lock.
	scope, scopeErr := s.store.OfferAuthorityContext(ctx, offerID, providerID)
	if scopeErr != nil {
		return Assignment{}, scopeErr
	}
	if s.finder == nil {
		return Assignment{}, errors.New("authoritative candidate validation is unavailable")
	}
	if err := s.finder.Validate(ctx, scope.MissionID, providerID); err != nil {
		return Assignment{}, err
	}
	if scope.Strategy == OrganizationDispatch {
		if s.organizations == nil {
			return Assignment{}, errors.New("organization authority unavailable")
		}
		if err := s.organizations.ValidateMembership(ctx, scope.OrganizationID, providerID); err != nil {
			return Assignment{}, err
		}
	}
	result, err := s.store.Accept(ctx, offerID, providerID, decision, s.now())
	if err != nil {
		return Assignment{}, err
	}
	if s.trips != nil && result.TripID == "" {
		tripID, ensureErr := s.trips.EnsureTrip(ctx, result.MissionID, providerID)
		if ensureErr != nil {
			return result, ensureErr
		}
		result.TripID = tripID
	}
	return result, nil
}

func (s *Service) Decline(ctx context.Context, offerID, providerID string, decision Decision) (Assignment, error) {
	if strings.TrimSpace(decision.IdempotencyKey) == "" || decision.ExpectedVersion < 1 {
		return Assignment{}, errors.New("idempotency key and expected version are required")
	}
	return s.store.Decline(ctx, offerID, providerID, decision, s.now(), s.policy.OfferWindow)
}

func (s *Service) ApplyQueueOption(ctx context.Context, missionID, customerID string, option QueueOption) (QueueEntry, error) {
	if strings.TrimSpace(option.IdempotencyKey) == "" {
		return QueueEntry{}, errors.New("idempotency key is required")
	}
	switch option.Option {
	case "KEEP_WAITING", "CANCEL":
	case "SCHEDULE":
		if option.ScheduledFor == nil || !option.ScheduledFor.After(s.now()) {
			return QueueEntry{}, errors.New("future schedule is required")
		}
	case "EXPANDED_SEARCH", "REQUOTE":
		if option.QuoteExpiresAt == nil || !option.QuoteExpiresAt.After(s.now()) || option.QuoteRef == "" {
			return QueueEntry{}, ErrStaleQuote
		}
		if s.finder == nil {
			return QueueEntry{}, errors.New("authoritative candidate discovery is unavailable")
		}
		discovered, err := s.finder.Discover(ctx, missionID, true)
		if err != nil {
			return QueueEntry{}, err
		}
		ranked, err := RankCandidates(discovered)
		if err != nil {
			return QueueEntry{}, err
		}
		if err = s.store.AddCandidates(ctx, missionID, ranked); err != nil {
			return QueueEntry{}, err
		}
	default:
		return QueueEntry{}, errors.New("invalid queue option")
	}
	return s.store.ApplyQueueOption(ctx, missionID, customerID, option, s.now())
}

func (s *Service) ApplyPartnerQueueOption(ctx context.Context, missionID string, option QueueOption) (QueueEntry, error) {
	if strings.TrimSpace(option.IdempotencyKey) == "" || strings.TrimSpace(option.SourceRef) == "" || option.SourceVersion < 1 {
		return QueueEntry{}, errors.New("idempotency and source scope are required")
	}
	switch option.Option {
	case "KEEP_WAITING", "CANCEL":
	case "SCHEDULE":
		if option.ScheduledFor == nil || !option.ScheduledFor.After(s.now()) {
			return QueueEntry{}, errors.New("future schedule is required")
		}
	case "EXPANDED_SEARCH", "REQUOTE":
		if option.QuoteExpiresAt == nil || !option.QuoteExpiresAt.After(s.now()) || option.QuoteRef == "" {
			return QueueEntry{}, ErrStaleQuote
		}
		if s.finder == nil {
			return QueueEntry{}, errors.New("authoritative candidate discovery is unavailable")
		}
		discovered, err := s.finder.Discover(ctx, missionID, true)
		if err != nil {
			return QueueEntry{}, err
		}
		ranked, err := RankCandidates(discovered)
		if err != nil {
			return QueueEntry{}, err
		}
		if err = s.store.AddCandidates(ctx, missionID, ranked); err != nil {
			return QueueEntry{}, err
		}
	default:
		return QueueEntry{}, errors.New("invalid queue option")
	}
	return s.store.ApplyPartnerQueueOption(ctx, missionID, option.SourceRef, option.SourceVersion, option, s.now())
}

func (s *Service) RunMaintenance(ctx context.Context) (runErr error) {
	started := time.Now()
	defer func() { observability.Worker("assignment", started, runErr) }()
	if _, err := s.store.ExpireOffers(ctx, s.now(), s.policy.OfferWindow, 100); err != nil {
		return err
	}
	if s.policy.QueueEnabled {
		due, err := s.store.ListDueQueueMissions(ctx, s.now(), 100)
		if err != nil {
			return err
		}
		if s.finder == nil && len(due) > 0 {
			return errors.New("authoritative candidate discovery is unavailable")
		}
		for _, missionID := range due {
			fresh, discoverErr := s.finder.Discover(ctx, missionID, false)
			if discoverErr != nil {
				continue
			}
			ranked, rankErr := RankCandidates(fresh)
			if rankErr == nil {
				_ = s.store.AddCandidates(ctx, missionID, ranked)
			}
		}
		if _, err := s.store.ProcessQueue(ctx, s.now(), s.policy.OfferWindow, s.policy.QueueRetryInterval, 100); err != nil {
			return err
		}
		reconciler, durable := s.store.(queueFinancialReconciler)
		leaseOwner := s.workerOwner + ":" + uuid.NewString()
		var pendingFinancial []PendingQueueFinancial
		if durable {
			pendingFinancial, err = reconciler.ClaimPendingQueueFinancial(ctx, s.now(), leaseOwner, s.policy.ReconciliationLease, s.policy.ReconciliationMaxAttempts, 100)
		} else {
			pendingFinancial, err = s.store.ListPendingQueueFinancial(ctx, 100)
		}
		if err != nil {
			return err
		}
		for _, item := range pendingFinancial {
			complete := func(state, code string) {
				if durable {
					if err := reconciler.CompleteQueueFinancial(ctx, item.QueueID, state, code, s.now(), s.reconciliationBackoff(item.AttemptCount+1), s.policy.ReconciliationMaxAttempts, leaseOwner); err != nil {
						log.Printf("queue financial outcome persistence failed")
					}
					return
				}
				_ = s.store.MarkQueueFinancial(ctx, item.QueueID, state, code)
			}
			if s.financial == nil {
				complete("UNKNOWN", "WALLET_AUTHORITY_UNAVAILABLE")
				continue
			}
			state, releaseErr := s.financial.ReleaseForNoProvider(ctx, item.BookingID, item.OperationRef)
			if releaseErr != nil {
				complete("UNKNOWN", "WALLET_OUTCOME_UNKNOWN")
				continue
			}
			complete(state, "")
		}
	}
	if s.trips != nil {
		pending, err := s.store.ListAcceptedWithoutTrip(ctx, 100)
		if err != nil {
			return err
		}
		for _, winner := range pending {
			if _, err = s.trips.EnsureTrip(ctx, winner.MissionID, winner.ProviderID); err != nil {
				return err
			}
		}
	}
	return nil
}

// RunDrainMaintenance performs only safety/reconciliation work. It creates no
// new offers and advances no Queue entry, so it is safe while assignment is
// disabled or the kill switch is active. Accepted winners still receive their
// canonical Trip and previously requested Wallet outcomes are reconciled.
func (s *Service) RunDrainMaintenance(ctx context.Context) (runErr error) {
	started := time.Now()
	defer func() { observability.Worker("assignment", started, runErr) }()
	reconciler, durable := s.store.(queueFinancialReconciler)
	leaseOwner := s.workerOwner + ":" + uuid.NewString()
	var pending []PendingQueueFinancial
	var err error
	if durable {
		pending, err = reconciler.ClaimPendingQueueFinancial(ctx, s.now(), leaseOwner, s.policy.ReconciliationLease, s.policy.ReconciliationMaxAttempts, 100)
	} else {
		pending, err = s.store.ListPendingQueueFinancial(ctx, 100)
	}
	if err != nil {
		return err
	}
	for _, item := range pending {
		state, code := "UNKNOWN", "WALLET_AUTHORITY_UNAVAILABLE"
		if s.financial != nil {
			var releaseErr error
			state, releaseErr = s.financial.ReleaseForNoProvider(ctx, item.BookingID, item.OperationRef)
			code = ""
			if releaseErr != nil {
				state, code = "UNKNOWN", "WALLET_OUTCOME_UNKNOWN"
			}
		}
		if durable {
			if err = reconciler.CompleteQueueFinancial(ctx, item.QueueID, state, code, s.now(), s.reconciliationBackoff(item.AttemptCount+1), s.policy.ReconciliationMaxAttempts, leaseOwner); err != nil {
				return err
			}
		} else if err = s.store.MarkQueueFinancial(ctx, item.QueueID, state, code); err != nil {
			return err
		}
	}
	if s.trips != nil {
		winners, listErr := s.store.ListAcceptedWithoutTrip(ctx, 100)
		if listErr != nil {
			return listErr
		}
		for _, winner := range winners {
			if _, err = s.trips.EnsureTrip(ctx, winner.MissionID, winner.ProviderID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) RunWorker(ctx context.Context, interval time.Duration) {
	defer observability.WorkerRunning("assignment")()
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.RunMaintenance(ctx); err != nil && ctx.Err() == nil {
				log.Printf("assignment reliability worker failed")
			}
		}
	}
}

func (s *Service) RunDrainWorker(ctx context.Context, interval time.Duration) {
	defer observability.WorkerRunning("assignment")()
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.RunDrainMaintenance(ctx); err != nil && ctx.Err() == nil {
				log.Printf("assignment drain worker failed")
			}
		}
	}
}

func (s *Service) reconciliationBackoff(attempt int) time.Duration {
	delay := s.policy.ReconciliationBackoffBase
	for i := 1; i < attempt && delay < s.policy.ReconciliationBackoffMax; i++ {
		delay *= 2
		if delay > s.policy.ReconciliationBackoffMax {
			delay = s.policy.ReconciliationBackoffMax
		}
	}
	return delay
}
