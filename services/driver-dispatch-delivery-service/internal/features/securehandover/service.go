package securehandover

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/observability"
	"cosmicforge/logistics/shared/go/apperrors"
	"github.com/google/uuid"
)

type MediaAuthorizer interface {
	AuthorizeEvidence(ctx context.Context, missionRef string, parcelRefs, mediaRefs []string) error
}

type Service struct {
	repo        *Repository
	cipher      *credentialCipher
	policy      Policy
	notifier    SecretTransport
	media       MediaAuthorizer
	limiter     RateLimiter
	workerOwner string
	now         func() time.Time
}

func NewService(repo *Repository, secret []byte, policy Policy) (*Service, error) {
	cipher, err := newCredentialCipher(secret)
	if err != nil {
		return nil, err
	}
	if repo == nil {
		return nil, errors.New("handover repository is required")
	}
	return &Service{repo: repo, cipher: cipher, policy: policy.normalized(), workerOwner: "handover-" + uuid.NewString(), now: func() time.Time { return time.Now().UTC() }}, nil
}

func (s *Service) WithNotifier(notifier SecretTransport) *Service     { s.notifier = notifier; return s }
func (s *Service) WithMediaAuthorizer(media MediaAuthorizer) *Service { s.media = media; return s }
func (s *Service) WithRateLimiter(limiter RateLimiter) *Service       { s.limiter = limiter; return s }

func (s *Service) Enabled() bool { return s != nil && s.policy.Enabled }

func (s *Service) Required(ctx context.Context, tripID string) (bool, error) {
	if s == nil || s.repo == nil {
		return false, nil
	}
	protection, err := s.repo.ProtectionByTrip(ctx, tripID)
	if err != nil {
		return false, err
	}
	return protection.HandoverRequired || protection.ExistingChallenge, nil
}

func (s *Service) IssuePickup(ctx context.Context, providerID, tripID, stopRef, operationRef string) (Credential, error) {
	if !s.Enabled() {
		return Credential{}, apperrors.Unavailable("Secure handover is not enabled.", nil)
	}
	if err := s.limit(ctx, "issue:provider:"+providerID, 10, time.Minute); err != nil {
		return Credential{}, err
	}
	context, err := s.repo.ResolveByTrip(ctx, tripID, StagePickup, stopRef)
	if err != nil || context.ProviderID != providerID {
		return Credential{}, apperrors.NotFound("Assigned trip pickup was not found.", err)
	}
	challenge, _, err := s.issue(ctx, context, StagePickup, "PROVIDER", "provider:"+providerID, "provider:"+providerID, operationRef, false)
	if err != nil {
		return Credential{}, mapError(err)
	}
	return s.credential(challenge, true), nil
}

func (s *Service) IssueDelivery(ctx context.Context, tripID, operationRef string) (Challenge, error) {
	if !s.Enabled() {
		return Challenge{}, nil
	}
	context, err := s.repo.ResolveByTrip(ctx, tripID, StageDelivery, "")
	if err != nil {
		returnContext, returnErr := s.repo.ResolveByTrip(ctx, tripID, StageReturn, "")
		if returnErr != nil {
			return Challenge{}, mapError(err)
		}
		// Customer-origin returns can use the authenticated customer notification
		// path. Commerce/partner senders obtain a separate source-scoped
		// credential through the internal HMAC route.
		if returnContext.SourceType != "CUSTOMER" {
			return Challenge{}, nil
		}
		return s.issueReturnForCustomer(ctx, returnContext, operationRef)
	}
	recipient := s.cipher.recipientRef(strings.TrimSpace(context.ReceiverPhone))
	challenge, replayed, err := s.issue(ctx, context, StageDelivery, "RECEIVER", recipient, "system:"+tripID, operationRef, true)
	if err != nil {
		return Challenge{}, mapError(err)
	}
	if !replayed || challenge.NotificationState == "NOT_REQUESTED" || challenge.NotificationState == "FAILED" {
		s.deliver(ctx, context, challenge, false)
		challenge, _ = s.repo.FindChallenge(ctx, challenge.ChallengeRef)
	}
	return sanitize(challenge), nil
}

func (s *Service) issueReturnForCustomer(ctx context.Context, context Context, operationRef string) (Challenge, error) {
	recipient := s.cipher.recipientRef("customer:" + context.CustomerID)
	challenge, replayed, err := s.issue(ctx, context, StageReturn, "SENDER", recipient, "system:"+context.TripID, operationRef, true)
	if err != nil {
		return Challenge{}, mapError(err)
	}
	if !replayed || challenge.NotificationState == "NOT_REQUESTED" || challenge.NotificationState == "FAILED" {
		s.deliver(ctx, context, challenge, false)
		challenge, _ = s.repo.FindChallenge(ctx, challenge.ChallengeRef)
	}
	return sanitize(challenge), nil
}

func (s *Service) IssueReturnByCustomer(ctx context.Context, customerID, bookingID, operationRef string) (Credential, error) {
	context, err := s.repo.ResolveCustomerBooking(ctx, bookingID, customerID, StageReturn)
	if err != nil {
		return Credential{}, mapError(err)
	}
	challenge, _, err := s.issue(ctx, context, StageReturn, "SENDER", s.cipher.recipientRef("customer:"+customerID), "customer:"+customerID, operationRef, false)
	if err != nil {
		return Credential{}, mapError(err)
	}
	return s.credential(challenge, true), nil
}

func (s *Service) IssueReturnBySource(ctx context.Context, caller, missionRef, stopRef, operationRef string) (Credential, error) {
	context, err := s.repo.ResolveMission(ctx, missionRef, StageReturn, stopRef)
	if err != nil {
		return Credential{}, mapError(err)
	}
	if (caller == "vendor-service" && context.SourceType != "VENDOR_COMMERCE") || (caller == "api-gateway" && context.SourceType != "PARTNER_API") {
		return Credential{}, apperrors.NotFound("Return handover was not found.", nil)
	}
	recipient := s.cipher.recipientRef("source:" + caller + ":" + context.SourceRef)
	challenge, _, err := s.issue(ctx, context, StageReturn, "SENDER", recipient, "source:"+caller, operationRef, false)
	if err != nil {
		return Credential{}, mapError(err)
	}
	return s.credential(challenge, true), nil
}

func (s *Service) EnsureDelivery(ctx context.Context, tripID, operationRef string) error {
	_, err := s.IssueDelivery(ctx, tripID, operationRef)
	return err
}

func (s *Service) issue(ctx context.Context, context Context, stage Stage, recipientType, recipientRef, caller, operation string, notify bool) (Challenge, bool, error) {
	if !s.Enabled() {
		return Challenge{}, false, apperrors.Unavailable("Secure handover is not enabled.", nil)
	}
	for _, key := range []string{"issue:booking:" + context.BookingID, "issue:recipient:" + recipientRef, "issue:source:" + context.SourceType + ":" + context.SourceRef} {
		if err := s.limit(ctx, key, 20, time.Minute); err != nil {
			return Challenge{}, false, err
		}
	}
	if strings.TrimSpace(operation) == "" {
		return Challenge{}, false, ErrConflict
	}
	reference, err := randomReference("handover")
	if err != nil {
		return Challenge{}, false, err
	}
	now := s.now()
	input := issueInput{Context: context, Stage: stage, ChallengeRef: reference, RecipientRef: recipientRef,
		RecipientType: recipientType, MaskedDestination: maskPhone(context.ReceiverPhone), ExpiresAt: now.Add(s.policy.TTL),
		CooldownUntil: now.Add(s.policy.ResendCooldown), MaxAttempts: s.policy.MaxAttempts, PolicyVersion: s.policy.PolicyVersion,
		OperationRef: operation, CallerScope: caller, Notify: notify, ParcelScans: context.MissionID != "" && (stage == StagePickup || stage == StageReturn),
		PhotoRequired: (stage == StagePickup && s.policy.PickupEvidenceEnabled) || ((stage == StageDelivery || stage == StageReturn) && s.policy.DeliveryEvidenceEnabled), Now: now}
	input.Fingerprint = fingerprint(struct {
		Context Context
		Caller  string
		Stage   Stage
	}{context, caller, stage})
	// Notification intent commits with the challenge, before any remote send.
	return s.repo.Issue(ctx, input)
}

func (s *Service) VerifyPickupByCustomer(ctx context.Context, customerID, bookingID string, command VerifyCommand) (Challenge, error) {
	if err := s.limit(ctx, "verify:customer:"+customerID, 20, time.Minute); err != nil {
		return Challenge{}, err
	}
	context, err := s.repo.ResolveCustomerBooking(ctx, bookingID, customerID, StagePickup)
	if err != nil {
		return Challenge{}, mapError(err)
	}
	return s.verify(ctx, context, Actor{Type: "CUSTOMER", Ref: customerID}, StagePickup, command)
}

func (s *Service) VerifyPickupBySource(ctx context.Context, caller, missionRef, stopRef string, command VerifyCommand) (Challenge, error) {
	if err := s.limit(ctx, "verify:source:"+caller+":"+missionRef, 30, time.Minute); err != nil {
		return Challenge{}, err
	}
	context, err := s.repo.ResolveMission(ctx, missionRef, StagePickup, stopRef)
	if err != nil {
		return Challenge{}, mapError(err)
	}
	if (caller == "vendor-service" && context.SourceType != "VENDOR_COMMERCE") || (caller == "api-gateway" && context.SourceType != "PARTNER_API") {
		return Challenge{}, apperrors.NotFound("Pickup handover was not found.", nil)
	}
	return s.verify(ctx, context, Actor{Type: sourceActor(caller), Ref: caller, SourceRef: context.SourceRef}, StagePickup, command)
}

// ReadBySource returns only an opaque challenge observation for an existing
// Commerce mission. It is the canonical reconciliation path after a caller
// loses the response to a verification request.
func (s *Service) ReadBySource(ctx context.Context, caller, missionRef, stopRef string, stage Stage) (Observation, error) {
	if !s.Enabled() || (stage != StagePickup && stage != StageDelivery) {
		return Observation{}, apperrors.Unavailable("Secure handover is not enabled.", nil)
	}
	handoverContext, err := s.repo.ResolveMission(ctx, missionRef, stage, stopRef)
	if err != nil {
		return Observation{}, mapError(err)
	}
	if (caller == "vendor-service" && handoverContext.SourceType != "VENDOR_COMMERCE") ||
		(caller == "api-gateway" && handoverContext.SourceType != "PARTNER_API") {
		return Observation{}, apperrors.NotFound("Handover was not found.", nil)
	}
	challenge, err := s.repo.LatestChallenge(ctx, handoverContext.TripID, stage)
	if err != nil {
		return Observation{}, mapError(err)
	}
	if challenge.MissionRef != handoverContext.MissionRef || challenge.StopRef != handoverContext.StopRef {
		return Observation{}, apperrors.NotFound("Handover was not found.", nil)
	}
	version := challenge.OptimisticVersion
	if version < 1 {
		version = 1
	}
	return Observation{MissionRef: missionRef, HandoverRef: challenge.ChallengeRef, Stage: challenge.Stage, Status: challenge.Status, SourceVersion: version}, nil
}

func (s *Service) VerifyDelivery(ctx context.Context, providerID, tripID string, command VerifyCommand) (Challenge, error) {
	if err := s.limit(ctx, "verify:provider:"+providerID, 20, time.Minute); err != nil {
		return Challenge{}, err
	}
	context, err := s.repo.ResolveByTrip(ctx, tripID, StageDelivery, "")
	if err != nil || context.ProviderID != providerID {
		return Challenge{}, apperrors.NotFound("Assigned trip delivery was not found.", err)
	}
	return s.verify(ctx, context, Actor{Type: "PROVIDER", Ref: providerID}, StageDelivery, command)
}

func (s *Service) VerifyReturn(ctx context.Context, providerID, tripID string, command VerifyCommand) (Challenge, error) {
	if err := s.limit(ctx, "verify:return:provider:"+providerID, 20, time.Minute); err != nil {
		return Challenge{}, err
	}
	context, err := s.repo.ResolveByTrip(ctx, tripID, StageReturn, "")
	if err != nil || context.ProviderID != providerID {
		return Challenge{}, apperrors.NotFound("Assigned return handover was not found.", err)
	}
	return s.verify(ctx, context, Actor{Type: "PROVIDER", Ref: providerID}, StageReturn, command)
}

func (s *Service) verify(ctx context.Context, context Context, actor Actor, stage Stage, command VerifyCommand) (resultChallenge Challenge, resultErr error) {
	defer func() { observability.Handover("verification", string(stage), string(command.Method), resultErr) }()
	if !s.Enabled() {
		return Challenge{}, apperrors.Unavailable("Secure handover is not enabled.", nil)
	}
	if err := command.Validate(); err != nil {
		return Challenge{}, apperrors.BadRequest("Handover verification is invalid.", err)
	}
	if command.Method == MethodQR && !s.policy.QREnabled {
		return Challenge{}, apperrors.Unavailable("QR handover is not enabled.", nil)
	}
	if err := s.limit(ctx, "verify:challenge:"+command.ChallengeRef, 30, time.Minute); err != nil {
		return Challenge{}, err
	}
	challenge, err := s.repo.FindChallenge(ctx, command.ChallengeRef)
	if err != nil {
		return Challenge{}, mapError(err)
	}
	if challenge.TripID != context.TripID || challenge.AssignedProviderID != context.ProviderID || challenge.IntendedRecipientRef == "" {
		return Challenge{}, apperrors.NotFound("Handover challenge was not found.", nil)
	}
	expected := challenge.OTPHash
	if command.Method == MethodQR {
		expected = challenge.QRTokenHash
	}
	valid := s.cipher.verify(command.Method, challenge.ChallengeRef, challenge.Version, strings.TrimSpace(command.Credential), expected)
	input := verifyInput{Command: command, Actor: actor, Stage: stage, Fingerprint: s.cipher.requestFingerprint(struct {
		Command VerifyCommand
		Actor   Actor
	}{command, actor}), CallerScope: strings.ToLower(actor.Type) + ":" + actor.Ref, SecretValid: valid, Now: s.now()}
	result, _, err := s.repo.Verify(ctx, input)
	if err != nil {
		return Challenge{}, mapError(err)
	}
	if _, completeErr := s.repo.CompleteStageIfReady(ctx, context, stage, s.now()); completeErr != nil && !errors.Is(completeErr, ErrEvidenceNeeded) {
		return Challenge{}, mapError(completeErr)
	}
	return sanitize(result), nil
}

func (s *Service) Resend(ctx context.Context, actor Actor, command ResendCommand) (resultChallenge Challenge, resultErr error) {
	if !s.Enabled() {
		return Challenge{}, apperrors.Unavailable("Secure handover is not enabled.", nil)
	}
	stage := "other"
	defer func() { observability.Handover("resend", stage, "", resultErr) }()
	if strings.TrimSpace(command.ChallengeRef) == "" || strings.TrimSpace(command.OperationRef) == "" {
		return Challenge{}, apperrors.BadRequest("Handover resend request is invalid.", nil)
	}
	if err := s.limit(ctx, "resend:"+strings.ToLower(actor.Type)+":"+actor.Ref, 5, 10*time.Minute); err != nil {
		return Challenge{}, err
	}
	old, err := s.repo.FindChallenge(ctx, command.ChallengeRef)
	if err != nil {
		return Challenge{}, mapError(err)
	}
	stage = string(old.Stage)
	context, err := s.repo.ResolveByTrip(ctx, old.TripID, old.Stage, old.StopRef)
	if err != nil || !actorAuthorized(actor, context, old.Stage) {
		return Challenge{}, apperrors.NotFound("Handover challenge was not found.", err)
	}
	for _, key := range []string{"resend:booking:" + old.BookingID, "resend:recipient:" + old.IntendedRecipientRef, "resend:source:" + context.SourceType + ":" + context.SourceRef} {
		if err := s.limit(ctx, key, 5, 10*time.Minute); err != nil {
			return Challenge{}, err
		}
	}
	reference, err := randomReference("handover")
	if err != nil {
		return Challenge{}, err
	}
	now := s.now()
	input := resendInput{OldChallenge: old, NewReference: reference, OperationRef: command.OperationRef,
		CallerScope: strings.ToLower(actor.Type) + ":" + actor.Ref, ExpiresAt: now.Add(s.policy.TTL),
		CooldownUntil: now.Add(s.policy.ResendCooldown), MaxResends: s.policy.MaxResends, Now: now}
	input.Fingerprint = fingerprint(struct {
		Ref   string
		Actor Actor
	}{command.ChallengeRef, actor})
	challenge, replayed, err := s.repo.Resend(ctx, input)
	if err != nil {
		return Challenge{}, mapError(err)
	}
	if challenge.Stage == StageDelivery && (!replayed || challenge.NotificationState != "ACCEPTED") {
		s.deliver(ctx, context, challenge, true)
		challenge, _ = s.repo.FindChallenge(ctx, challenge.ChallengeRef)
	}
	return sanitize(challenge), nil
}

func (s *Service) CreateCaptureSession(ctx context.Context, providerID, tripID, stopRef string, command CaptureSessionCommand) (CaptureSession, error) {
	evidenceType := strings.ToUpper(strings.TrimSpace(command.EvidenceType))
	if strings.TrimSpace(command.OperationRef) == "" || strings.TrimSpace(command.ManifestRef) == "" || command.ManifestVersion < 1 ||
		(command.Stage != StagePickup && command.Stage != StageDelivery && command.Stage != StageReturn) || (evidenceType != "PHOTO" && evidenceType != "SIGNATURE") {
		return CaptureSession{}, apperrors.BadRequest("Evidence capture session request is invalid.", nil)
	}
	command.EvidenceType = evidenceType
	context, err := s.repo.ResolveByTrip(ctx, tripID, command.Stage, stopRef)
	if err != nil || context.ProviderID != providerID {
		return CaptureSession{}, apperrors.NotFound("Assigned handover context was not found.", err)
	}
	challenge, err := s.repo.LatestChallenge(ctx, tripID, command.Stage)
	if err != nil {
		return CaptureSession{}, mapError(err)
	}
	ref, err := randomReference("capture")
	if err != nil {
		return CaptureSession{}, err
	}
	result, err := s.repo.CreateCaptureSession(ctx, context, challenge, providerID, command, ref, fingerprint(command), s.now(), s.policy.CaptureTTL)
	return result, mapError(err)
}

func (s *Service) ScanParcel(ctx context.Context, providerID, tripID, stopRef string, command ScanCommand) error {
	if strings.TrimSpace(command.OperationRef) == "" || strings.TrimSpace(command.ParcelCode) == "" || strings.TrimSpace(command.ManifestRef) == "" || command.ManifestVersion < 1 {
		return apperrors.BadRequest("Parcel scan request is invalid.", nil)
	}
	stage := command.Stage
	if stage == "" {
		stage = StagePickup
	}
	if stage != StagePickup && stage != StageReturn {
		return apperrors.BadRequest("Parcel scan stage is invalid.", nil)
	}
	context, err := s.repo.ResolveByTrip(ctx, tripID, stage, stopRef)
	if err != nil || context.ProviderID != providerID || context.ManifestRef != command.ManifestRef || context.ManifestVersion != command.ManifestVersion {
		return apperrors.NotFound("Expected parcel was not found.", err)
	}
	return mapError(s.repo.ScanParcel(ctx, context, providerID, command, fingerprint(command), s.now()))
}

func (s *Service) SubmitEvidence(ctx context.Context, providerID, tripID, stopRef string, command EvidenceCommand) (Evidence, error) {
	if strings.TrimSpace(command.OperationRef) == "" || strings.TrimSpace(command.CaptureSessionRef) == "" || strings.TrimSpace(command.MediaRef) == "" {
		return Evidence{}, apperrors.BadRequest("Evidence request is invalid.", nil)
	}
	if command.CapturedAt.IsZero() {
		command.CapturedAt = s.now()
	}
	stage, err := s.repo.CaptureSessionStage(ctx, command.CaptureSessionRef)
	if err != nil {
		return Evidence{}, mapError(err)
	}
	context, err := s.repo.ResolveByTrip(ctx, tripID, stage, stopRef)
	if err != nil || context.ProviderID != providerID {
		return Evidence{}, apperrors.NotFound("Evidence context was not found.", err)
	}
	if s.media == nil {
		return Evidence{}, apperrors.Unavailable("Media authorization is unavailable.", nil)
	}
	parcels, err := s.repo.ParcelRefs(ctx, context)
	if err != nil {
		return Evidence{}, err
	}
	ownerRef := context.MissionRef
	if context.MissionID == "" {
		ownerRef = context.TripID
	}
	if err = s.media.AuthorizeEvidence(ctx, ownerRef, parcels, []string{command.MediaRef}); err != nil {
		return Evidence{}, apperrors.New(409, "MEDIA_ATTACHMENT_SCOPE_MISMATCH", "Media is not authorized for this handover context.", err)
	}
	result, err := s.repo.RecordEvidence(ctx, evidenceInput{Context: context, ProviderID: providerID, Command: command, Fingerprint: fingerprint(command), Now: s.now()})
	return result, mapError(err)
}

func (s *Service) PickupReady(ctx context.Context, tripID string) (bool, error) {
	protection, err := s.repo.ProtectionByTrip(ctx, tripID)
	if err != nil {
		return false, err
	}
	required, err := s.repo.StageEvidenceRequired(ctx, tripID, StagePickup)
	if err != nil {
		return false, err
	}
	return s.repo.StageReady(ctx, tripID, StagePickup, protection.PickupEvidenceRequired || required)
}

func (s *Service) DeliveryReady(ctx context.Context, tripID string) (bool, error) {
	protection, err := s.repo.ProtectionByTrip(ctx, tripID)
	if err != nil {
		return false, err
	}
	required, err := s.repo.StageEvidenceRequired(ctx, tripID, StageDelivery)
	if err != nil {
		return false, err
	}
	return s.repo.StageReady(ctx, tripID, StageDelivery, protection.DeliveryEvidenceRequired || required)
}

func (s *Service) ReturnReady(ctx context.Context, tripID string) (bool, error) {
	required, err := s.repo.StageEvidenceRequired(ctx, tripID, StageReturn)
	if err != nil {
		return false, err
	}
	return s.repo.StageReady(ctx, tripID, StageReturn, required)
}

func (s *Service) MarkInTransit(ctx context.Context, tripID string) error {
	return s.repo.MarkMissionInTransit(ctx, tripID, s.now())
}

func (s *Service) InvalidateForProviderChange(ctx context.Context, tripID, oldProviderID string) error {
	return s.repo.InvalidateTripChallenges(ctx, tripID, oldProviderID, "PROVIDER_REASSIGNED", s.now())
}

func (s *Service) InvalidateForReceiverChange(ctx context.Context, tripID string) error {
	return s.repo.InvalidateTripChallenges(ctx, tripID, "", "RECEIVER_CHANGED", s.now())
}

func (s *Service) Expire(ctx context.Context) (int64, error) { return s.repo.Expire(ctx, s.now(), 100) }

func (s *Service) RunExpiryWorker(ctx context.Context, interval time.Duration) {
	defer observability.WorkerRunning("handover")()
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			started := time.Now()
			_, expiryErr := s.Expire(ctx)
			if expiryErr != nil && ctx.Err() == nil {
				log.Printf("handover expiry maintenance failed")
			}
			notificationErr := s.reconcileNotifications(ctx, 100)
			if notificationErr != nil && ctx.Err() == nil {
				log.Printf("handover notification reconciliation failed")
			}
			observability.Worker("handover", started, errors.Join(expiryErr, notificationErr))
		}
	}
}

func (s *Service) credential(challenge Challenge, includeSecret bool) Credential {
	result := Credential{ChallengeRef: challenge.ChallengeRef, Version: challenge.Version, Stage: challenge.Stage,
		ExpiresAt: challenge.ExpiresAt, ManifestRef: challenge.ManifestRef, ManifestVersion: challenge.ManifestVersion}
	if includeSecret {
		result.OTP = s.cipher.deriveOTP(challenge.ChallengeRef, challenge.Version)
		if s.policy.QREnabled {
			result.QRToken = s.cipher.deriveQR(challenge.ChallengeRef, challenge.Version)
		}
	}
	return result
}

func (s *Service) deliver(ctx context.Context, context Context, challenge Challenge, resend bool, leaseOwner ...string) {
	channels := make([]string, 0, 3)
	if s.policy.InAppAllowed && context.CustomerID != "" {
		channels = append(channels, "in_app")
	}
	if s.policy.PushAllowed && context.CustomerID != "" {
		channels = append(channels, "push")
	}
	if s.policy.EmailAllowed && strings.TrimSpace(context.ReceiverEmail) != "" {
		channels = append(channels, "email")
	}
	// SMS is deliberately not appended: notification-service has no SMS
	// channel. The policy bit reserves domain intent for later activation.
	if s.notifier == nil || len(channels) == 0 {
		now := s.now()
		_ = s.repo.RecordNotificationOutcome(ctx, challenge.ChallengeRef, "UNAVAILABLE", "", channels, "CHANNEL_UNAVAILABLE", now, now, s.policy.ReconciliationMaxAttempts, leaseOwner...)
		return
	}
	reference, state, accepted, err := s.notifier.Deliver(ctx, SecretDeliveryIntent{Challenge: challenge, Context: context,
		OTP: s.cipher.deriveOTP(challenge.ChallengeRef, challenge.Version), QRToken: s.cipher.deriveQR(challenge.ChallengeRef, challenge.Version), Resend: resend, Channels: channels})
	if err != nil && state == "" {
		state = "FAILED"
	}
	errorCode := ""
	if err != nil {
		errorCode = "NOTIFICATION_OUTCOME_UNKNOWN"
	}
	now := s.now()
	_ = s.repo.RecordNotificationOutcome(ctx, challenge.ChallengeRef, state, reference, accepted, errorCode, now, now.Add(s.notificationBackoff(challenge.NotificationAttemptCount+1)), s.policy.ReconciliationMaxAttempts, leaseOwner...)
}

func (s *Service) reconcileNotifications(ctx context.Context, limit int) error {
	if s.notifier == nil {
		return nil
	}
	owner := s.workerOwner + ":" + uuid.NewString()
	pending, err := s.repo.ClaimPendingNotifications(ctx, s.now(), owner, s.policy.ReconciliationLease, s.policy.ReconciliationMaxAttempts, limit)
	if err != nil {
		return err
	}
	for _, item := range pending {
		challenge, findErr := s.repo.FindChallenge(ctx, item.ChallengeRef)
		if findErr != nil {
			now := s.now()
			_ = s.repo.RecordNotificationOutcome(ctx, item.ChallengeRef, "FAILED", "", nil, "CHALLENGE_READ_FAILED", now, now.Add(s.notificationBackoff(item.AttemptCount+1)), s.policy.ReconciliationMaxAttempts, owner)
			continue
		}
		context, resolveErr := s.repo.ResolveByTrip(ctx, challenge.TripID, challenge.Stage, challenge.StopRef)
		if resolveErr != nil {
			now := s.now()
			_ = s.repo.RecordNotificationOutcome(ctx, item.ChallengeRef, "FAILED", "", nil, "CONTEXT_READ_FAILED", now, now.Add(s.notificationBackoff(item.AttemptCount+1)), s.policy.ReconciliationMaxAttempts, owner)
			continue
		}
		if !s.notificationContextMatches(challenge, context) {
			now := s.now()
			_ = s.repo.RecordNotificationOutcome(ctx, item.ChallengeRef, "UNAVAILABLE", "", nil, "CONTEXT_CHANGED", now, now, s.policy.ReconciliationMaxAttempts, owner)
			continue
		}
		s.deliver(ctx, context, challenge, challenge.ResendCount > 0, owner)
	}
	return nil
}

func (s *Service) notificationContextMatches(challenge Challenge, current Context) bool {
	recipient := s.cipher.recipientRef(strings.TrimSpace(current.ReceiverPhone))
	if challenge.Stage == StageReturn {
		recipient = s.cipher.recipientRef("customer:" + current.CustomerID)
	}
	return challenge.Status == StatusActive && challenge.ExpiresAt.After(s.now()) &&
		challenge.TripID == current.TripID && challenge.MissionRef == current.MissionRef &&
		challenge.AssignedProviderID == current.ProviderID && challenge.ManifestRef == current.ManifestRef &&
		challenge.ManifestVersion == current.ManifestVersion && challenge.SourceVersion == current.SourceVersion &&
		challenge.IntendedRecipientRef == recipient && (challenge.Stage == StageDelivery || challenge.Stage == StageReturn)
}

func (s *Service) notificationBackoff(attempt int) time.Duration {
	delay := s.policy.ReconciliationBackoffBase
	for i := 1; i < attempt && delay < s.policy.ReconciliationBackoffMax; i++ {
		delay *= 2
		if delay > s.policy.ReconciliationBackoffMax {
			delay = s.policy.ReconciliationBackoffMax
		}
	}
	return delay
}

func (s *Service) limit(ctx context.Context, key string, maximum int, window time.Duration) error {
	if s.limiter == nil {
		return apperrors.Unavailable("Handover rate limiting is unavailable.", nil)
	}
	if err := s.limiter.Allow(ctx, key, maximum, window); err != nil {
		if errors.Is(err, ErrLocked) {
			return apperrors.RateLimited("Too many handover requests. Try again later.", nil)
		}
		return apperrors.Unavailable("Handover rate limiting is unavailable.", err)
	}
	return nil
}

func sanitize(challenge Challenge) Challenge {
	challenge.ID, challenge.ContextKey, challenge.AssignedProviderID, challenge.IntendedRecipientRef = "", "", "", ""
	challenge.OTPHash, challenge.QRTokenHash = "", ""
	return challenge
}

func maskPhone(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < 4 {
		return ""
	}
	return "***" + value[len(value)-4:]
}

func actorAuthorized(actor Actor, context Context, stage Stage) bool {
	switch strings.ToUpper(actor.Type) {
	case "CUSTOMER":
		return actor.Ref == context.CustomerID
	case "PROVIDER":
		return actor.Ref == context.ProviderID
	case "VENDOR":
		return context.SourceType == "VENDOR_COMMERCE" && (actor.SourceRef == context.SourceRef || actor.SourceRef == context.MissionID || actor.SourceRef == context.MissionRef)
	case "PARTNER":
		return context.SourceType == "PARTNER_API" && (actor.SourceRef == context.SourceRef || actor.SourceRef == context.MissionID || actor.SourceRef == context.MissionRef)
	default:
		return false
	}
}

func sourceActor(caller string) string {
	if caller == "vendor-service" {
		return "VENDOR"
	}
	return "PARTNER"
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, ErrNotFound):
		return apperrors.NotFound("Handover resource was not found.", nil)
	case errors.Is(err, ErrExpired):
		return apperrors.New(409, "HANDOVER_EXPIRED", "Handover challenge has expired.", nil)
	case errors.Is(err, ErrLocked):
		return apperrors.RateLimited("Handover challenge is locked.", nil)
	case errors.Is(err, ErrInvalidated):
		return apperrors.New(409, "HANDOVER_INVALIDATED", "Handover challenge is no longer valid.", nil)
	case errors.Is(err, ErrInvalidCode):
		return apperrors.BadRequest("Handover credential is invalid.", nil)
	case errors.Is(err, ErrScopeMismatch):
		return apperrors.NotFound("Handover resource was not found.", nil)
	case errors.Is(err, ErrEvidenceNeeded):
		return apperrors.New(409, "EVIDENCE_REQUIRED", "Required scans or evidence are incomplete.", nil)
	case errors.Is(err, ErrConflict):
		return apperrors.Conflict("Handover operation conflicts with current state.", nil)
	default:
		return err
	}
}
