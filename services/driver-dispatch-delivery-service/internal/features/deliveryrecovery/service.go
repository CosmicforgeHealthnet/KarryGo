package deliveryrecovery

import (
	"context"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/observability"
	"cosmicforge/logistics/shared/go/mediaclient"
	"log"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
	"github.com/google/uuid"
)

type HandoverInvalidator interface {
	InvalidateForReceiverChange(context.Context, string) error
}
type ReturnRouteAuthority interface {
	ReturnRoute(context.Context, TripContext) (RouteDecision, error)
}
type ReplacementAuthority interface {
	RequestReplacement(context.Context, string, string, string) error
	DispatchMission(context.Context, string, string) error
}
type HubAuthority interface {
	AuthorizeHub(context.Context, string) (string, error)
}
type FinancialAuthority interface {
	ValidateRecoveryAuthorization(context.Context, string, string, int64) error
}
type HubEvidenceAuthority interface {
	AuthorizeEvidence(context.Context, string, []string, []string) error
}

type Service struct {
	uploads           *mediaclient.Client
	repo              *Repository
	policy            Policy
	handover          HandoverInvalidator
	routes            ReturnRouteAuthority
	replacement       ReplacementAuthority
	hubs              HubAuthority
	financial         FinancialAuthority
	hubEvidence       HubEvidenceAuthority
	now               func() time.Time
	workerOwner       string
	workerLease       time.Duration
	workerMaxAttempts int
	workerBackoffBase time.Duration
	workerBackoffMax  time.Duration
}

func New(repo *Repository, policy Policy) *Service {
	return &Service{repo: repo, policy: policy.normalized(), now: func() time.Time { return time.Now().UTC() }, workerOwner: "delivery-recovery", workerLease: time.Minute, workerMaxAttempts: 10, workerBackoffBase: 5 * time.Second, workerBackoffMax: 15 * time.Minute}
}

func (s *Service) WithReliabilityPolicy(owner string, lease time.Duration, maxAttempts int, baseBackoff, maxBackoff time.Duration) *Service {
	if strings.TrimSpace(owner) != "" {
		s.workerOwner = strings.TrimSpace(owner)
	}
	if lease > 0 {
		s.workerLease = lease
	}
	if maxAttempts > 0 {
		s.workerMaxAttempts = maxAttempts
	}
	if baseBackoff > 0 {
		s.workerBackoffBase = baseBackoff
	}
	if maxBackoff >= s.workerBackoffBase {
		s.workerBackoffMax = maxBackoff
	}
	return s
}
func (s *Service) WithHandover(v HandoverInvalidator) *Service          { s.handover = v; return s }
func (s *Service) WithRoutes(v ReturnRouteAuthority) *Service           { s.routes = v; return s }
func (s *Service) WithReplacement(v ReplacementAuthority) *Service      { s.replacement = v; return s }
func (s *Service) WithHubAuthority(v HubAuthority) *Service             { s.hubs = v; return s }
func (s *Service) WithFinancialAuthority(v FinancialAuthority) *Service { s.financial = v; return s }
func (s *Service) WithHubEvidenceAuthority(v HubEvidenceAuthority) *Service {
	s.hubEvidence = v
	return s
}

func (s *Service) StartAttempt(ctx context.Context, command StartAttemptCommand) (Attempt, error) {
	if s == nil || !s.policy.Enabled {
		return Attempt{}, apperrors.Unavailable("Delivery recovery is not enabled.", nil)
	}
	command.TripID, command.ProviderID, command.IdempotencyKey = strings.TrimSpace(command.TripID), strings.TrimSpace(command.ProviderID), strings.TrimSpace(command.IdempotencyKey)
	if command.TripID == "" || command.ProviderID == "" || command.IdempotencyKey == "" {
		return Attempt{}, apperrors.BadRequest("Trip and Idempotency-Key are required.", nil)
	}
	return s.repo.StartAttempt(ctx, command, s.policy.GracePeriod, s.now(), fingerprint(command))
}

func (s *Service) RecordContact(ctx context.Context, command ContactCommand) (ContactAttempt, error) {
	command.AttemptID, command.ProviderID, command.IdempotencyKey = strings.TrimSpace(command.AttemptID), strings.TrimSpace(command.ProviderID), strings.TrimSpace(command.IdempotencyKey)
	command.Method, command.Outcome = strings.ToUpper(strings.TrimSpace(command.Method)), strings.ToUpper(strings.TrimSpace(command.Outcome))
	if command.AttemptID == "" || command.ProviderID == "" || command.IdempotencyKey == "" || !contactMethods[command.Method] || !contactOutcomes[command.Outcome] {
		return ContactAttempt{}, apperrors.BadRequest("Contact attempt is invalid.", nil)
	}
	return s.repo.AppendContact(ctx, command, s.now(), fingerprint(command))
}

func (s *Service) FailAttempt(ctx context.Context, command FailAttemptCommand) (Attempt, error) {
	command.AttemptID, command.ProviderID, command.IdempotencyKey = strings.TrimSpace(command.AttemptID), strings.TrimSpace(command.ProviderID), strings.TrimSpace(command.IdempotencyKey)
	command.ReasonCode = strings.ToUpper(strings.TrimSpace(command.ReasonCode))
	if command.AttemptID == "" || command.ProviderID == "" || command.IdempotencyKey == "" || len(receiverReasons[command.ReasonCode]) == 0 {
		return Attempt{}, apperrors.BadRequest("Receiver-unavailable reason is invalid.", nil)
	}
	actions := allowedActions(command.ReasonCode, s.policy)
	return s.repo.FailAttempt(ctx, command, actions, s.now(), fingerprint(command))
}

func (s *Service) SelectRecovery(ctx context.Context, command SelectRecoveryCommand) (RecoveryResult, error) {
	if s == nil || !s.policy.Enabled {
		return RecoveryResult{}, apperrors.Unavailable("Delivery recovery is not enabled.", nil)
	}
	command.BookingID, command.CustomerID, command.IdempotencyKey = strings.TrimSpace(command.BookingID), strings.TrimSpace(command.CustomerID), strings.TrimSpace(command.IdempotencyKey)
	if command.BookingID == "" || command.CustomerID == "" || command.IdempotencyKey == "" {
		return RecoveryResult{}, apperrors.BadRequest("Booking and Idempotency-Key are required.", nil)
	}
	if command.AdditionalFeeKobo < 0 {
		return RecoveryResult{}, apperrors.BadRequest("Additional recovery fee is invalid.", nil)
	}
	fp := fingerprint(command)
	if existing, stored, found, err := s.repo.FindRecovery(ctx, command.IdempotencyKey); err != nil {
		return RecoveryResult{}, err
	} else if found {
		if stored != fp {
			return RecoveryResult{}, apperrors.Conflict("Idempotency key was reused with different recovery data.", nil)
		}
		if existing.Action == ReturnToSender && existing.ReturnMissionID != "" {
			if s.replacement == nil {
				return existing, apperrors.Unavailable("Return assignment authority is unavailable.", nil)
			}
			if err = s.replacement.DispatchMission(ctx, existing.ReturnMissionID, command.IdempotencyKey+":return-assignment"); err != nil {
				return existing, err
			}
			_ = s.repo.MarkReturnAssignmentAcknowledged(ctx, existing.ReturnMissionID, s.now())
		}
		existing.Replayed = true
		return existing, nil
	}
	attempt, err := s.repo.LatestUnavailableForBooking(ctx, command.BookingID, command.CustomerID)
	if err != nil {
		return RecoveryResult{}, err
	}
	if !containsAction(attempt.AllowedActions, command.Action) {
		return RecoveryResult{}, apperrors.New(409, "RECOVERY_ACTION_NOT_ALLOWED", "That recovery action is not allowed by current policy.", nil)
	}
	now := s.now()
	if (command.Action == RetrySameDay || command.Action == Reschedule) && (command.ScheduledFor == nil || !command.ScheduledFor.After(now)) {
		return RecoveryResult{}, apperrors.BadRequest("A future schedule is required.", nil)
	}
	if command.Action == RetrySameDay && command.ScheduledFor.Sub(now) > s.policy.MaximumSameDayWindow {
		return RecoveryResult{}, apperrors.New(409, "SAME_DAY_WINDOW_EXCEEDED", "Retry is outside the same-day policy window.", nil)
	}
	receiverRef := ""
	if command.Action == AlternativeReceiver {
		if strings.TrimSpace(command.AlternativeReceiverContact) == "" || s.handover == nil {
			return RecoveryResult{}, apperrors.New(409, "ALTERNATIVE_RECEIVER_AUTHORITY_UNAVAILABLE", "Receiver change authority is unavailable.", nil)
		}
		if err = s.handover.InvalidateForReceiverChange(ctx, attempt.TripID); err != nil {
			return RecoveryResult{}, apperrors.Unavailable("The existing delivery challenge could not be invalidated.", err)
		}
		receiverRef = opaqueReceiverRef(command.AlternativeReceiverContact)
	}
	var route *RouteDecision
	if command.Action == ReturnToSender {
		if !s.policy.ReturnEnabled || s.routes == nil {
			return RecoveryResult{}, apperrors.Unavailable("Return routing is unavailable.", nil)
		}
		trip, tripErr := s.repo.Trip(ctx, attempt.TripID, "", false)
		if tripErr != nil {
			return RecoveryResult{}, tripErr
		}
		decision, routeErr := s.routes.ReturnRoute(ctx, trip)
		if routeErr != nil {
			return RecoveryResult{}, apperrors.Unavailable("Return route could not be established.", routeErr)
		}
		route = &decision
	}
	if command.AdditionalFeeKobo > 0 {
		if s.financial == nil || strings.TrimSpace(command.FinancialAuthorizationRef) == "" {
			return RecoveryResult{}, apperrors.New(409, "FINANCIAL_AUTHORIZATION_REQUIRED", "Wallet authorization is required for this recovery charge.", nil)
		}
		if err = s.financial.ValidateRecoveryAuthorization(ctx, command.FinancialAuthorizationRef, command.CustomerID, command.AdditionalFeeKobo); err != nil {
			return RecoveryResult{}, err
		}
	}
	if command.Action == HubHold {
		if !s.policy.HubHoldEnabled || s.hubs == nil {
			return RecoveryResult{}, apperrors.New(409, "HUB_AUTHORITY_UNAVAILABLE", "Approved hub hold is unavailable.", nil)
		}
		authorityRef, hubErr := s.hubs.AuthorizeHub(ctx, command.HubRef)
		if hubErr != nil {
			return RecoveryResult{}, hubErr
		}
		command.HubAuthorityRef = authorityRef
	}
	result, err := s.repo.SelectRecovery(ctx, attempt, command, receiverRef, route, now, fp)
	if err != nil {
		return result, err
	}
	if result.Action == ReturnToSender && result.ReturnMissionID != "" {
		if s.replacement == nil {
			return result, apperrors.Unavailable("Return assignment authority is unavailable.", nil)
		}
		if err = s.replacement.DispatchMission(ctx, result.ReturnMissionID, command.IdempotencyKey+":return-assignment"); err != nil {
			return result, err
		}
		_ = s.repo.MarkReturnAssignmentAcknowledged(ctx, result.ReturnMissionID, s.now())
	}
	return result, nil
}

func (s *Service) RunMaintenance(ctx context.Context) (runErr error) {
	started := time.Now()
	defer func() { observability.Worker("recovery", started, runErr) }()
	if err := s.reconcileIncidents(ctx); err != nil {
		return err
	}
	if _, err := s.repo.ActivateScheduled(ctx, s.now(), 100); err != nil {
		return err
	}
	// A disabled assignment authority is a deployment gate, not a failed attempt.
	if s.replacement == nil {
		return nil
	}
	owner := s.workerOwner + ":" + uuid.NewString()
	pending, err := s.repo.ClaimReturnAssignments(ctx, s.now(), owner, s.workerLease, s.workerMaxAttempts, 100)
	if err != nil {
		return err
	}
	for _, item := range pending {
		dispatchErr := s.replacement.DispatchMission(ctx, item.MissionID, item.OperationRef)
		if err = s.repo.CompleteReturnAssignment(ctx, item.WorkflowID, dispatchErr == nil, errorCode(dispatchErr), s.now(), s.backoff(item.AttemptCount+1), s.workerMaxAttempts, owner); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) RunWorker(ctx context.Context, interval time.Duration) {
	if !s.policy.Enabled {
		return
	}
	defer observability.WorkerRunning("recovery")()
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.RunMaintenance(ctx); err != nil && ctx.Err() == nil {
				log.Printf("delivery recovery worker failed: %v", err)
			}
		}
	}
}

func (s *Service) backoff(attempt int) time.Duration {
	delay := s.workerBackoffBase
	for i := 1; i < attempt && delay < s.workerBackoffMax; i++ {
		delay *= 2
		if delay > s.workerBackoffMax {
			delay = s.workerBackoffMax
		}
	}
	return delay
}

func errorCode(err error) string {
	if err == nil {
		return ""
	}
	return "ASSIGNMENT_OUTCOME_UNKNOWN"
}

func (s *Service) ReportIncident(ctx context.Context, command IncidentCommand) (Incident, error) {
	if s == nil || !s.policy.ReplacementEnabled {
		return Incident{}, apperrors.Unavailable("Replacement recovery is not enabled.", nil)
	}
	command.TripID, command.ProviderID, command.IdempotencyKey = strings.TrimSpace(command.TripID), strings.TrimSpace(command.ProviderID), strings.TrimSpace(command.IdempotencyKey)
	command.ReasonCode = strings.ToUpper(strings.TrimSpace(command.ReasonCode))
	// Provider reports the incident; server policy selects the safe marketplace
	// replacement. Organization/admin strategies remain available only on their
	// existing authenticated authority routes.
	command.PreferredStrategy = "SEQUENTIAL_NEAREST"
	if command.TripID == "" || command.ProviderID == "" || command.IdempotencyKey == "" || !incidentReasons[command.ReasonCode] || len(command.SafeNote) > 500 {
		return Incident{}, apperrors.BadRequest("Incident report is invalid.", nil)
	}
	item, err := s.repo.ReportIncident(ctx, command, s.now(), fingerprint(command))
	if err != nil {
		return Incident{}, err
	}
	// The incident row is the durable replacement intent. The existing recovery
	// worker claims it and reads back the assignment before retrying.
	return item, nil
}

func (s *Service) RecordHubCheckpoint(ctx context.Context, command HubCheckpointCommand) (HubCheckpoint, error) {
	if s == nil || !s.policy.HubHoldEnabled {
		return HubCheckpoint{}, apperrors.Unavailable("Approved hub hold is unavailable.", nil)
	}
	command.TripID, command.ProviderID = strings.TrimSpace(command.TripID), strings.TrimSpace(command.ProviderID)
	command.Stage, command.MediaRef = strings.ToUpper(strings.TrimSpace(command.Stage)), strings.TrimSpace(command.MediaRef)
	command.IdempotencyKey = strings.TrimSpace(command.IdempotencyKey)
	if command.TripID == "" || command.ProviderID == "" || command.IdempotencyKey == "" || command.MediaRef == "" || (command.Stage != "ARRIVAL" && command.Stage != "RELEASE") {
		return HubCheckpoint{}, apperrors.BadRequest("Hub checkpoint is invalid.", nil)
	}
	context, err := s.repo.HubCheckpointContext(ctx, command)
	if err != nil {
		return HubCheckpoint{}, err
	}
	if s.hubEvidence == nil {
		return HubCheckpoint{}, apperrors.Unavailable("Hub evidence authority is unavailable.", nil)
	}
	if err = s.hubEvidence.AuthorizeEvidence(ctx, context.MissionRef, context.ParcelRefs, []string{command.MediaRef}); err != nil {
		return HubCheckpoint{}, apperrors.New(409, "MEDIA_ATTACHMENT_SCOPE_MISMATCH", "Hub evidence is not authorized for this mission.", err)
	}
	if !sameCodes(context.ParcelCodes, command.ParcelCodes) {
		return HubCheckpoint{}, apperrors.New(409, "PARCEL_MANIFEST_MISMATCH", "Scanned parcels do not match the active manifest.", nil)
	}
	return s.repo.RecordHubCheckpoint(ctx, command, context, s.now(), fingerprint(command))
}

func (s *Service) RecordEmergencyRecovery(ctx context.Context, command EmergencyRecoveryCommand) (EmergencyRecovery, error) {
	if s == nil || !s.policy.ReplacementEnabled {
		return EmergencyRecovery{}, apperrors.Unavailable("Emergency recovery is unavailable.", nil)
	}
	command.TripID, command.ActorRef = strings.TrimSpace(command.TripID), strings.TrimSpace(command.ActorRef)
	command.ReasonCode, command.IdempotencyKey = strings.ToUpper(strings.TrimSpace(command.ReasonCode)), strings.TrimSpace(command.IdempotencyKey)
	if command.TripID == "" || command.ActorRef == "" || command.IdempotencyKey == "" || command.ReasonCode == "" || len(command.ReasonCode) > 64 || len(command.SafeNote) > 500 || command.ExpectedTripVersion < 1 {
		return EmergencyRecovery{}, apperrors.BadRequest("Emergency recovery command is invalid.", nil)
	}
	return s.repo.RecordEmergencyRecovery(ctx, command, s.now(), fingerprint(command))
}

func (s *Service) WithMediaUploads(client *mediaclient.Client) *Service {
	if s != nil {
		s.uploads = client
	}
	return s
}
