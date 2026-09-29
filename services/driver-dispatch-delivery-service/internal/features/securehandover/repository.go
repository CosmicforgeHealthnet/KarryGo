package securehandover

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound       = errors.New("secure handover not found")
	ErrConflict       = errors.New("secure handover conflict")
	ErrExpired        = errors.New("secure handover expired")
	ErrLocked         = errors.New("secure handover locked")
	ErrInvalidated    = errors.New("secure handover invalidated")
	ErrInvalidCode    = errors.New("secure handover credential invalid")
	ErrScopeMismatch  = errors.New("secure handover scope mismatch")
	ErrEvidenceNeeded = errors.New("secure handover evidence incomplete")
)

type Repository struct {
	db     *pgxpool.Pool
	cipher *credentialCipher
}

func NewRepository(db *pgxpool.Pool, secret []byte) (*Repository, error) {
	cipher, err := newCredentialCipher(secret)
	if err != nil {
		return nil, err
	}
	return &Repository{db: db, cipher: cipher}, nil
}

func (r *Repository) ResolveByTrip(ctx context.Context, tripID string, stage Stage, requestedStop string) (Context, error) {
	var result Context
	var receiverPhone string
	err := r.db.QueryRow(ctx, `SELECT t.id::text,t.booking_id::text,t.provider_id::text,t.customer_id::text,
		b.receiver_phone,b.receiver_email,COALESCE(b.receiver_customer_id::text,''),COALESCE(dm.id::text,''),COALESCE(dm.delivery_group_id::text,''),
		COALESCE(dm.source_type,'CUSTOMER'),COALESCE(dm.source_ref,t.id::text),COALESCE(dm.source_version,1)
		FROM trips t JOIN dispatch_bookings b ON b.id=t.booking_id
		LEFT JOIN dispatch_delivery_missions dm ON dm.trip_id=t.id WHERE t.id=$1::uuid`, tripID).
		Scan(&result.TripID, &result.BookingID, &result.ProviderID, &result.CustomerID,
			&receiverPhone, &result.ReceiverEmail, &result.ReceiverRef, &result.MissionID, &result.DeliveryGroupID, &result.SourceType, &result.SourceRef, &result.SourceVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return Context{}, ErrNotFound
	}
	if err != nil {
		return Context{}, err
	}
	result.ReceiverPhone = receiverPhone
	if result.MissionID == "" {
		result.MissionRef = "trip:" + result.TripID
		result.StopRef = strings.ToLower(string(stage))
		result.ManifestRef = "legacy-trip:" + result.TripID
		result.ManifestVersion = 1
		return result, nil
	}
	result.MissionRef = result.MissionID
	stopType := string(stage)
	if stage == StageDelivery {
		stopType = "DELIVERY"
	} else if stage == StageReturn {
		stopType = "RETURN"
	}
	query := `SELECT id::text,stop_ref FROM dispatch_mission_stops WHERE mission_id=$1::uuid AND stop_type=$2`
	args := []any{result.MissionID, stopType}
	if strings.TrimSpace(requestedStop) != "" {
		query += ` AND (id::text=$3 OR stop_ref=$3)`
		args = append(args, requestedStop)
	}
	query += ` ORDER BY sequence LIMIT 1`
	if err = r.db.QueryRow(ctx, query, args...).Scan(&result.StopID, &result.StopRef); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Context{}, ErrNotFound
		}
		return Context{}, err
	}
	if result.StopRef == "" {
		result.StopRef = result.StopID
	}
	manifestStates := []string{"ACTIVE", "CUSTODY_ACTIVE"}
	if stage == StageDelivery || stage == StageReturn {
		manifestStates = append(manifestStates, "COMPLETED")
	}
	err = r.db.QueryRow(ctx, `SELECT id::text,manifest_ref,version FROM dispatch_parcel_manifests
		WHERE mission_id=$1::uuid AND status=ANY($2) ORDER BY version DESC LIMIT 1`, result.MissionID, manifestStates).
		Scan(&result.ManifestID, &result.ManifestRef, &result.ManifestVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return Context{}, ErrNotFound
	}
	return result, err
}

// ProtectionByTrip reads the immutable Booking rollout policy. Existing
// challenge rows also keep the Trip protected so work admitted before the
// policy-pinning migration cannot be downgraded by a flag change.
func (r *Repository) ProtectionByTrip(ctx context.Context, tripID string) (ProtectionSnapshot, error) {
	var result ProtectionSnapshot
	err := r.db.QueryRow(ctx, `SELECT b.rollout_protection_mode,b.rollout_policy_version,
		b.handover_required,b.pickup_evidence_required,b.delivery_evidence_required,b.provider_transfer_required,
		EXISTS(SELECT 1 FROM dispatch_handover_challenges h WHERE h.trip_id=t.id)
		FROM trips t JOIN dispatch_bookings b ON b.id=t.booking_id WHERE t.id=$1::uuid`, tripID).Scan(
		&result.Mode, &result.PolicyVersion, &result.HandoverRequired,
		&result.PickupEvidenceRequired, &result.DeliveryEvidenceRequired,
		&result.ProviderTransferRequired, &result.ExistingChallenge,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProtectionSnapshot{}, ErrNotFound
	}
	return result, err
}

func (r *Repository) StageEvidenceRequired(ctx context.Context, tripID string, stage Stage) (bool, error) {
	context, err := r.ResolveByTrip(ctx, tripID, stage, "")
	if err != nil {
		return false, err
	}
	var required bool
	err = r.db.QueryRow(ctx, `SELECT COALESCE(bool_or(photo_required OR signature_required OR gps_required OR min_evidence_count>0),false)
		FROM dispatch_evidence_requirements WHERE context_key=$1 AND stage=$2`, context.ContextKey(stage), stage).Scan(&required)
	return required, err
}

func (r *Repository) ResolveCustomerBooking(ctx context.Context, bookingID, customerID string, stage Stage) (Context, error) {
	var tripID string
	err := r.db.QueryRow(ctx, `SELECT id::text FROM trips WHERE booking_id=$1::uuid AND customer_id=$2::uuid`, bookingID, customerID).Scan(&tripID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Context{}, ErrNotFound
	}
	if err != nil {
		return Context{}, err
	}
	return r.ResolveByTrip(ctx, tripID, stage, "")
}

func (r *Repository) ResolveMission(ctx context.Context, missionRef string, stage Stage, stopRef string) (Context, error) {
	var tripID string
	err := r.db.QueryRow(ctx, `SELECT trip_id::text FROM dispatch_delivery_missions
		WHERE (id::text=$1 OR source_ref=$1) AND trip_id IS NOT NULL`, missionRef).Scan(&tripID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Context{}, ErrNotFound
	}
	if err != nil {
		return Context{}, err
	}
	return r.ResolveByTrip(ctx, tripID, stage, stopRef)
}

type issueInput struct {
	Context           Context
	Stage             Stage
	ChallengeRef      string
	RecipientRef      string
	RecipientType     string
	MaskedDestination string
	ExpiresAt         time.Time
	CooldownUntil     time.Time
	MaxAttempts       int
	PolicyVersion     string
	OperationRef      string
	Fingerprint       string
	CallerScope       string
	ParcelScans       bool
	PhotoRequired     bool
	Notify            bool
	Now               time.Time
}

func (r *Repository) Issue(ctx context.Context, in issueInput) (Challenge, bool, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Challenge{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	contextKey := in.Context.ContextKey(in.Stage)
	stageContextKey := strings.Join([]string{in.Context.MissionRef, in.Context.StopRef, string(in.Stage)}, ":")
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, stageContextKey); err != nil {
		return Challenge{}, false, err
	}
	if existing, found, operationErr := findOperationTx(ctx, tx, in.CallerScope, in.OperationRef, in.Fingerprint); operationErr != nil {
		return Challenge{}, false, operationErr
	} else if found {
		challenge, loadErr := loadChallengeTx(ctx, tx, existing)
		return challenge, true, loadErr
	}
	challenge, found, err := activeChallengeForStageTx(ctx, tx, in.Context.MissionRef, in.Context.StopRef, in.Stage)
	if err != nil {
		return Challenge{}, false, err
	}
	if found && (challenge.AssignedProviderID != in.Context.ProviderID ||
		challenge.IntendedRecipientRef != in.RecipientRef ||
		challenge.ManifestRef != in.Context.ManifestRef || challenge.ManifestVersion != in.Context.ManifestVersion ||
		challenge.SourceVersion != in.Context.SourceVersion) {
		_, err = tx.Exec(ctx, `UPDATE dispatch_handover_challenges SET status='INVALIDATED',invalidated_at=$2,
			invalidated_reason='CONTEXT_CHANGED',locked_at=NULL,updated_at=$2,optimistic_version=optimistic_version+1
			WHERE id=$1::uuid`, challenge.ID, in.Now)
		if err != nil {
			return Challenge{}, false, err
		}
		found = false
	}
	if found {
		if err = insertOperationTx(ctx, tx, in.CallerScope, in.OperationRef, "ISSUE", in.Fingerprint, challenge); err != nil {
			return Challenge{}, false, err
		}
		return challenge, true, tx.Commit(ctx)
	}
	var version int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(max(version),0)+1 FROM dispatch_handover_challenges WHERE context_key=$1`, contextKey).Scan(&version); err != nil {
		return Challenge{}, false, err
	}
	id := uuid.NewString()
	otp := r.cipher.deriveOTP(in.ChallengeRef, version)
	qr := r.cipher.deriveQR(in.ChallengeRef, version)
	otpHash := r.cipher.hash(MethodOTP, in.ChallengeRef, version, otp)
	qrHash := r.cipher.hash(MethodQR, in.ChallengeRef, version, qr)
	challenge = Challenge{ID: id, ChallengeRef: in.ChallengeRef, ContextKey: contextKey, Version: version, Stage: in.Stage,
		MissionRef: in.Context.MissionRef, StopRef: in.Context.StopRef, TripID: in.Context.TripID, BookingID: in.Context.BookingID,
		ManifestRef: in.Context.ManifestRef, ManifestVersion: in.Context.ManifestVersion, AssignedProviderID: in.Context.ProviderID,
		IntendedRecipientRef: in.RecipientRef, RecipientType: in.RecipientType, MaskedDestination: in.MaskedDestination,
		OTPHash: otpHash, QRTokenHash: qrHash, ExpiresAt: in.ExpiresAt, MaxAttempts: in.MaxAttempts,
		CooldownUntil: &in.CooldownUntil, Status: StatusActive, NotificationState: "NOT_REQUESTED",
		SourceVersion: in.Context.SourceVersion, OptimisticVersion: 1, CreatedAt: in.Now, UpdatedAt: in.Now}
	if in.Notify {
		challenge.NotificationState = "REQUESTED"
	}
	_, err = tx.Exec(ctx, `INSERT INTO dispatch_handover_challenges(
		id,challenge_ref,context_key,version,stage,delivery_group_id,mission_id,mission_ref,stop_id,stop_ref,
		trip_id,booking_id,manifest_ref,manifest_version,assigned_provider_id,intended_recipient_ref,recipient_type,
		masked_destination,otp_hash,qr_token_hash,expires_at,max_attempts,resend_cooldown_until,created_by_source,
		source_version,created_at,updated_at,notification_state)
		VALUES($1::uuid,$2,$3,$4,$5,NULLIF($6,'')::uuid,NULLIF($7,'')::uuid,$8,NULLIF($9,'')::uuid,$10,
		$11::uuid,$12::uuid,$13,$14,NULLIF($15,'')::uuid,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$26,$27)`,
		id, in.ChallengeRef, contextKey, version, in.Stage, in.Context.DeliveryGroupID, in.Context.MissionID, in.Context.MissionRef,
		in.Context.StopID, in.Context.StopRef, in.Context.TripID, in.Context.BookingID, in.Context.ManifestRef,
		in.Context.ManifestVersion, in.Context.ProviderID, in.RecipientRef, in.RecipientType, in.MaskedDestination,
		otpHash, qrHash, in.ExpiresAt, in.MaxAttempts, in.CooldownUntil, in.CallerScope, in.Context.SourceVersion, in.Now, challenge.NotificationState)
	if err != nil {
		return Challenge{}, false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO dispatch_evidence_requirements(context_key,stage,policy_version,parcel_scans_required,photo_required,signature_required,gps_required,min_evidence_count)
		VALUES($1,$2,$3,$4,$5,false,false,$6) ON CONFLICT DO NOTHING`, contextKey, in.Stage, in.PolicyVersion, in.ParcelScans, in.PhotoRequired, boolInt(in.PhotoRequired))
	if err != nil {
		return Challenge{}, false, err
	}
	if err = insertOperationTx(ctx, tx, in.CallerScope, in.OperationRef, "ISSUE", in.Fingerprint, challenge); err != nil {
		return Challenge{}, false, err
	}
	if err = emitEventTx(ctx, tx, "dispatch.handover.challenge_issued.v1", challenge, map[string]any{"stage": in.Stage}); err != nil {
		return Challenge{}, false, err
	}
	return challenge, false, tx.Commit(ctx)
}

type verifyInput struct {
	Command     VerifyCommand
	Actor       Actor
	Stage       Stage
	Fingerprint string
	CallerScope string
	SecretValid bool
	Now         time.Time
}

func (r *Repository) Verify(ctx context.Context, in verifyInput) (Challenge, bool, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Challenge{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if challengeID, found, operationErr := findOperationTx(ctx, tx, in.CallerScope, in.Command.OperationRef, in.Fingerprint); operationErr != nil {
		return Challenge{}, false, operationErr
	} else if found {
		challenge, loadErr := loadChallengeTx(ctx, tx, challengeID)
		return challenge, true, loadErr
	}
	challenge, err := lockChallengeTx(ctx, tx, in.Command.ChallengeRef)
	if err != nil {
		return Challenge{}, false, err
	}
	result, reason := "FAILED", "OTP_INVALID"
	terminalErr := error(nil)
	if challenge.Status == StatusExpired || (challenge.Status == StatusActive && !in.Now.Before(challenge.ExpiresAt)) {
		result, reason, terminalErr = "EXPIRED", "OTP_EXPIRED", ErrExpired
		if challenge.Status == StatusActive {
			_, err = tx.Exec(ctx, `UPDATE dispatch_handover_challenges SET status='EXPIRED',updated_at=$2,optimistic_version=optimistic_version+1 WHERE id=$1::uuid`, challenge.ID, in.Now)
		}
	} else if challenge.Status == StatusLocked {
		result, reason, terminalErr = "LOCKED", "OTP_LOCKED", ErrLocked
	} else if challenge.Status == StatusInvalidated {
		result, reason, terminalErr = "INVALIDATED", "CHALLENGE_INVALIDATED", ErrInvalidated
	} else if challenge.Status != StatusActive {
		result, reason, terminalErr = "INVALIDATED", "CHALLENGE_CONSUMED", ErrConflict
	} else if challenge.Stage != in.Stage || challenge.ManifestRef != in.Command.ManifestRef || challenge.ManifestVersion != in.Command.ManifestVersion ||
		(in.Command.MissionRef != "" && challenge.MissionRef != in.Command.MissionRef) || (in.Command.StopRef != "" && challenge.StopRef != in.Command.StopRef) ||
		(in.Command.ExpectedSourceVersion > 0 && challenge.SourceVersion != in.Command.ExpectedSourceVersion) {
		result, reason, terminalErr = "SCOPE_MISMATCH", "HANDOVER_SCOPE_MISMATCH", ErrScopeMismatch
	} else if !in.SecretValid {
		attempts := challenge.AttemptCount + 1
		status := StatusActive
		if attempts >= challenge.MaxAttempts {
			status = StatusLocked
			result, reason, terminalErr = "LOCKED", "OTP_LOCKED", ErrLocked
		} else {
			terminalErr = ErrInvalidCode
		}
		_, err = tx.Exec(ctx, `UPDATE dispatch_handover_challenges SET attempt_count=$2,status=$3,
		locked_at=CASE WHEN $3='LOCKED' THEN $4::timestamptz ELSE NULL END,updated_at=$4::timestamptz,optimistic_version=optimistic_version+1 WHERE id=$1::uuid`, challenge.ID, attempts, status, in.Now)
		challenge.AttemptCount, challenge.Status = attempts, status
	} else {
		result, reason = "SUCCEEDED", ""
		_, err = tx.Exec(ctx, `UPDATE dispatch_handover_challenges SET status='CONSUMED',consumed_at=$2,
			updated_at=$2,optimistic_version=optimistic_version+1 WHERE id=$1::uuid AND status='ACTIVE'`, challenge.ID, in.Now)
		if err == nil {
			challenge.Status = StatusConsumed
		}
	}
	if err != nil {
		return Challenge{}, false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO dispatch_handover_attempts(challenge_id,operation_ref,request_fingerprint,method,actor_type,actor_ref,result,reason_code,attempted_at)
		VALUES($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9)`, challenge.ID, in.Command.OperationRef, in.Fingerprint, in.Command.Method, in.Actor.Type, in.Actor.Ref, result, reason, in.Now)
	if err != nil {
		return Challenge{}, false, err
	}
	if terminalErr == nil {
		if err = insertOperationTx(ctx, tx, in.CallerScope, in.Command.OperationRef, "VERIFY", in.Fingerprint, challenge); err != nil {
			return Challenge{}, false, err
		}
		if err = emitEventTx(ctx, tx, eventForVerification(in.Stage), challenge, map[string]any{"method": in.Command.Method}); err != nil {
			return Challenge{}, false, err
		}
	}
	if commitErr := tx.Commit(ctx); commitErr != nil {
		return Challenge{}, false, commitErr
	}
	return challenge, false, terminalErr
}

type resendInput struct {
	OldChallenge  Challenge
	NewReference  string
	OperationRef  string
	Fingerprint   string
	CallerScope   string
	ExpiresAt     time.Time
	CooldownUntil time.Time
	MaxResends    int
	Now           time.Time
}

func (r *Repository) Resend(ctx context.Context, in resendInput) (Challenge, bool, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Challenge{}, false, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if challengeID, found, operationErr := findOperationTx(ctx, tx, in.CallerScope, in.OperationRef, in.Fingerprint); operationErr != nil {
		return Challenge{}, false, operationErr
	} else if found {
		challenge, loadErr := loadChallengeTx(ctx, tx, challengeID)
		return challenge, true, loadErr
	}
	current, err := lockChallengeTx(ctx, tx, in.OldChallenge.ChallengeRef)
	if err != nil {
		return Challenge{}, false, err
	}
	if current.Status != StatusActive && current.Status != StatusLocked && current.Status != StatusExpired {
		return Challenge{}, false, ErrConflict
	}
	if current.ResendCount >= in.MaxResends {
		return Challenge{}, false, ErrLocked
	}
	if current.CooldownUntil != nil && in.Now.Before(*current.CooldownUntil) {
		return Challenge{}, false, ErrConflict
	}
	_, err = tx.Exec(ctx, `UPDATE dispatch_handover_challenges SET status='INVALIDATED',invalidated_at=$2,
		invalidated_reason='RESENT',locked_at=NULL,updated_at=$2,optimistic_version=optimistic_version+1 WHERE id=$1::uuid`, current.ID, in.Now)
	if err != nil {
		return Challenge{}, false, err
	}
	newID := uuid.NewString()
	next := current
	next.ID, next.ChallengeRef, next.Version = newID, in.NewReference, current.Version+1
	otp := r.cipher.deriveOTP(in.NewReference, next.Version)
	qr := r.cipher.deriveQR(in.NewReference, next.Version)
	next.OTPHash = r.cipher.hash(MethodOTP, in.NewReference, next.Version, otp)
	next.QRTokenHash = r.cipher.hash(MethodQR, in.NewReference, next.Version, qr)
	next.ExpiresAt, next.AttemptCount, next.ResendCount = in.ExpiresAt, 0, current.ResendCount+1
	next.CooldownUntil, next.Status = &in.CooldownUntil, StatusActive
	next.NotificationState, next.NotificationChannels = "NOT_REQUESTED", nil
	if next.Stage == StageDelivery {
		next.NotificationState = "REQUESTED"
	}
	next.OptimisticVersion, next.CreatedAt, next.UpdatedAt = 1, in.Now, in.Now
	_, err = tx.Exec(ctx, `INSERT INTO dispatch_handover_challenges(
		id,challenge_ref,context_key,version,stage,delivery_group_id,mission_id,mission_ref,stop_id,stop_ref,trip_id,booking_id,
		manifest_ref,manifest_version,assigned_provider_id,intended_recipient_ref,recipient_type,masked_destination,otp_hash,qr_token_hash,
		expires_at,max_attempts,resend_count,resend_cooldown_until,created_by_source,source_version,created_at,updated_at,notification_state)
		SELECT $1::uuid,$2,context_key,$3,stage,delivery_group_id,mission_id,mission_ref,stop_id,stop_ref,trip_id,booking_id,
		manifest_ref,manifest_version,assigned_provider_id,intended_recipient_ref,recipient_type,masked_destination,$4,$5,$6,max_attempts,$7,$8,
		$9,source_version,$10,$10,$12 FROM dispatch_handover_challenges WHERE id=$11::uuid`,
		newID, in.NewReference, next.Version, next.OTPHash, next.QRTokenHash, in.ExpiresAt, next.ResendCount, in.CooldownUntil, in.CallerScope, in.Now, current.ID, next.NotificationState)
	if err != nil {
		return Challenge{}, false, err
	}
	if err = insertOperationTx(ctx, tx, in.CallerScope, in.OperationRef, "RESEND", in.Fingerprint, next); err != nil {
		return Challenge{}, false, err
	}
	if err = emitEventTx(ctx, tx, "dispatch.handover.resent.v1", next, map[string]any{"previousVersion": current.Version}); err != nil {
		return Challenge{}, false, err
	}
	return next, false, tx.Commit(ctx)
}

func (r *Repository) FindChallenge(ctx context.Context, challengeRef string) (Challenge, error) {
	return scanChallenge(r.db.QueryRow(ctx, challengeSelect+` WHERE challenge_ref=$1`, challengeRef))
}

func (r *Repository) LatestChallenge(ctx context.Context, tripID string, stage Stage) (Challenge, error) {
	return scanChallenge(r.db.QueryRow(ctx, challengeSelect+` WHERE trip_id=$1::uuid AND stage=$2 ORDER BY version DESC,created_at DESC LIMIT 1`, tripID, stage))
}

func (r *Repository) ParcelRefs(ctx context.Context, context Context) ([]string, error) {
	if context.MissionID == "" || context.StopID == "" || context.ManifestID == "" {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `SELECT p.karrygo_parcel_ref FROM dispatch_stop_expected_parcels e
		JOIN dispatch_parcels p ON p.id=e.parcel_id WHERE e.mission_stop_id=$1::uuid AND e.manifest_id=$2::uuid ORDER BY p.karrygo_parcel_ref`, context.StopID, context.ManifestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var ref string
		if err = rows.Scan(&ref); err != nil {
			return nil, err
		}
		result = append(result, ref)
	}
	return result, rows.Err()
}

func (r *Repository) MarkNotification(ctx context.Context, challengeRef, state, notificationRef string, channels []string) error {
	now := time.Now().UTC()
	return r.RecordNotificationOutcome(ctx, challengeRef, state, notificationRef, channels, "", now, now, 10)
}

func (r *Repository) RecordNotificationOutcome(ctx context.Context, challengeRef, state, notificationRef string, channels []string, errorCode string, now, next time.Time, maxAttempts int, leaseOwner ...string) error {
	owner := ""
	if len(leaseOwner) > 0 {
		owner = leaseOwner[0]
	}
	if channels == nil {
		channels = []string{}
	}
	if maxAttempts <= 0 {
		maxAttempts = 10
	}
	_, err := r.db.Exec(ctx, `UPDATE dispatch_handover_challenges SET
		notification_attempt_count=notification_attempt_count+1,
		notification_state=CASE WHEN $2='FAILED' AND notification_attempt_count+1 >= $8 THEN 'DEAD_LETTER' ELSE $2 END,
		notification_ref=$3,notification_channels=$4,notification_last_error_code=$5,
		notification_next_attempt_at=$7::timestamptz,notification_terminal_at=CASE WHEN $2 IN ('ACCEPTED','UNAVAILABLE') OR notification_attempt_count+1 >= $8 THEN $6::timestamptz ELSE NULL END,
		notification_lease_owner=NULL,notification_lease_expires_at=NULL,updated_at=$6::timestamptz,optimistic_version=optimistic_version+1
		WHERE challenge_ref=$1 AND notification_state NOT IN ('ACCEPTED','DEAD_LETTER')
		AND COALESCE(notification_lease_owner,'')=$9`, challengeRef, state, notificationRef, channels, errorCode, now, next, maxAttempts, owner)
	return err
}

func (r *Repository) ClaimPendingNotifications(ctx context.Context, now time.Time, owner string, lease time.Duration, maxAttempts, limit int) ([]PendingNotification, error) {
	if limit <= 0 {
		limit = 100
	}
	if maxAttempts <= 0 {
		maxAttempts = 10
	}
	rows, err := r.db.Query(ctx, `WITH due AS (
		SELECT id FROM dispatch_handover_challenges
		WHERE notification_state IN ('REQUESTED','FAILED') AND status='ACTIVE' AND expires_at>$1
		  AND notification_attempt_count<$4 AND notification_next_attempt_at<=$1
		  AND (notification_lease_expires_at IS NULL OR notification_lease_expires_at<=$1)
		ORDER BY notification_next_attempt_at,updated_at FOR UPDATE SKIP LOCKED LIMIT $5
	)
	UPDATE dispatch_handover_challenges c SET notification_state='REQUESTED',notification_lease_owner=$2,
		notification_lease_expires_at=$1::timestamptz+$3::interval,updated_at=$1::timestamptz FROM due WHERE c.id=due.id
	RETURNING c.challenge_ref,c.notification_attempt_count`, now, owner, lease.String(), maxAttempts, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []PendingNotification
	for rows.Next() {
		var item PendingNotification
		if err = rows.Scan(&item.ChallengeRef, &item.AttemptCount); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) CreateCaptureSession(ctx context.Context, context Context, challenge Challenge, providerID string, command CaptureSessionCommand, ref, fingerprint string, now time.Time, ttl time.Duration) (CaptureSession, error) {
	if challenge.Status != StatusConsumed || challenge.AssignedProviderID != providerID || challenge.MissionRef != context.MissionRef ||
		challenge.StopRef != context.StopRef || challenge.Stage != command.Stage || challenge.ManifestRef != command.ManifestRef || challenge.ManifestVersion != command.ManifestVersion {
		return CaptureSession{}, ErrScopeMismatch
	}
	var existing CaptureSession
	err := r.db.QueryRow(ctx, `SELECT capture_session_ref,c.challenge_ref,s.stage,s.evidence_type,s.manifest_ref,s.manifest_version,s.expires_at
		FROM dispatch_evidence_capture_sessions s JOIN dispatch_handover_challenges c ON c.id=s.challenge_id WHERE s.operation_ref=$1 AND s.request_fingerprint=$2`, command.OperationRef, fingerprint).
		Scan(&existing.Ref, &existing.ChallengeRef, &existing.Stage, &existing.EvidenceType, &existing.ManifestRef, &existing.ManifestVersion, &existing.ExpiresAt)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return CaptureSession{}, err
	}
	_, err = r.db.Exec(ctx, `INSERT INTO dispatch_evidence_capture_sessions(capture_session_ref,challenge_id,mission_id,stop_id,trip_id,provider_id,
		manifest_ref,manifest_version,stage,evidence_type,expires_at,operation_ref,request_fingerprint,created_at,updated_at)
		VALUES($1,$2::uuid,NULLIF($3,'')::uuid,NULLIF($4,'')::uuid,$5::uuid,$6::uuid,$7,$8,$9,$10,$11,$12,$13,$14,$14)`,
		ref, challenge.ID, context.MissionID, context.StopID, context.TripID, providerID, command.ManifestRef, command.ManifestVersion,
		command.Stage, strings.ToUpper(strings.TrimSpace(command.EvidenceType)), now.Add(ttl), command.OperationRef, fingerprint, now)
	if err != nil {
		return CaptureSession{}, err
	}
	return CaptureSession{Ref: ref, ChallengeRef: challenge.ChallengeRef, Stage: command.Stage, EvidenceType: strings.ToUpper(command.EvidenceType), ManifestRef: command.ManifestRef, ManifestVersion: command.ManifestVersion, ExpiresAt: now.Add(ttl)}, nil
}

func (r *Repository) CaptureSessionStage(ctx context.Context, reference string) (Stage, error) {
	var stage Stage
	err := r.db.QueryRow(ctx, `SELECT stage FROM dispatch_evidence_capture_sessions WHERE capture_session_ref=$1`, reference).Scan(&stage)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return stage, err
}

func (r *Repository) ScanParcel(ctx context.Context, context Context, providerID string, command ScanCommand, fingerprint string, now time.Time) error {
	if context.MissionID == "" || context.StopID == "" || context.ManifestID == "" {
		return ErrScopeMismatch
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var verified bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM dispatch_handover_challenges WHERE context_key=$1 AND stage='PICKUP' AND status='CONSUMED')`, context.ContextKey(StagePickup)).Scan(&verified); err != nil {
		return err
	}
	if !verified {
		return ErrEvidenceNeeded
	}
	var parcelID string
	err = tx.QueryRow(ctx, `SELECT p.id::text FROM dispatch_parcels p
		JOIN dispatch_stop_expected_parcels e ON e.parcel_id=p.id
		JOIN dispatch_manifest_parcels mp ON mp.parcel_id=p.id AND mp.manifest_id=e.manifest_id
		WHERE e.mission_stop_id=$1::uuid AND e.manifest_id=$2::uuid
		AND (p.karrygo_parcel_ref=$3 OR p.scannable_code=$3)`, context.StopID, context.ManifestID, strings.TrimSpace(command.ParcelCode)).Scan(&parcelID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrScopeMismatch
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO dispatch_parcel_scan_events(mission_id,stop_id,manifest_id,parcel_id,trip_id,provider_id,operation_ref,request_fingerprint,scanned_at)
		VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,$5::uuid,$6::uuid,$7,$8,$9)
		ON CONFLICT(mission_id,stop_id,manifest_id,parcel_id) DO NOTHING`, context.MissionID, context.StopID, context.ManifestID, parcelID, context.TripID, providerID, command.OperationRef, fingerprint, now)
	if err != nil {
		return err
	}
	_, err = r.completeStageIfReadyTx(ctx, tx, context, StagePickup, now)
	if err != nil && !errors.Is(err, ErrEvidenceNeeded) {
		return err
	}
	return tx.Commit(ctx)
}

type evidenceInput struct {
	Context     Context
	ProviderID  string
	Command     EvidenceCommand
	Fingerprint string
	Now         time.Time
}

func (r *Repository) RecordEvidence(ctx context.Context, in evidenceInput) (Evidence, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Evidence{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var sessionID, challengeID, challengeRef, missionID, stopID, tripID, providerID, manifestRef, stage, evidenceType, status string
	var manifestVersion int64
	var expires time.Time
	err = tx.QueryRow(ctx, `SELECT s.id::text,s.challenge_id::text,c.challenge_ref,COALESCE(s.mission_id::text,''),COALESCE(s.stop_id::text,''),
		s.trip_id::text,s.provider_id::text,s.manifest_ref,s.manifest_version,s.stage,s.evidence_type,s.status,s.expires_at
		FROM dispatch_evidence_capture_sessions s JOIN dispatch_handover_challenges c ON c.id=s.challenge_id
		WHERE s.capture_session_ref=$1 FOR UPDATE OF s`, in.Command.CaptureSessionRef).
		Scan(&sessionID, &challengeID, &challengeRef, &missionID, &stopID, &tripID, &providerID, &manifestRef, &manifestVersion, &stage, &evidenceType, &status, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return Evidence{}, ErrNotFound
	}
	if err != nil {
		return Evidence{}, err
	}
	if providerID != in.ProviderID || tripID != in.Context.TripID || missionID != in.Context.MissionID || stopID != in.Context.StopID ||
		manifestRef != in.Context.ManifestRef || manifestVersion != in.Context.ManifestVersion || status != "ACTIVE" || !in.Now.Before(expires) {
		return Evidence{}, ErrScopeMismatch
	}
	var existing Evidence
	err = tx.QueryRow(ctx, `SELECT evidence_ref,media_ref,stage,evidence_type,manifest_ref,manifest_version,captured_at
		FROM dispatch_evidence_refs WHERE operation_ref=$1`, in.Command.OperationRef).
		Scan(&existing.EvidenceRef, &existing.MediaRef, &existing.Stage, &existing.EvidenceType, &existing.ManifestRef, &existing.ManifestVersion, &existing.CapturedAt)
	if err == nil {
		return existing, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Evidence{}, err
	}
	evidenceRef, err := randomReference("evidence")
	if err != nil {
		return Evidence{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO dispatch_evidence_refs(evidence_ref,capture_session_id,challenge_id,mission_id,stop_id,trip_id,provider_id,
		manifest_ref,manifest_version,stage,evidence_type,media_ref,captured_at,latitude,longitude,reason_code,supersedes_evidence_ref,
		operation_ref,request_fingerprint,created_at)
		VALUES($1,$2::uuid,$3::uuid,NULLIF($4,'')::uuid,NULLIF($5,'')::uuid,$6::uuid,$7::uuid,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`,
		evidenceRef, sessionID, challengeID, missionID, stopID, tripID, providerID, manifestRef, manifestVersion, stage, evidenceType,
		in.Command.MediaRef, in.Command.CapturedAt, in.Command.Latitude, in.Command.Longitude, strings.TrimSpace(in.Command.ReasonCode),
		strings.TrimSpace(in.Command.SupersedesRef), in.Command.OperationRef, in.Fingerprint, in.Now)
	if err != nil {
		return Evidence{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE dispatch_evidence_capture_sessions SET upload_count=upload_count+1,
		status=CASE WHEN upload_count+1>=max_uploads THEN 'CONSUMED' ELSE status END,
		consumed_at=CASE WHEN upload_count+1>=max_uploads THEN $2 ELSE consumed_at END,updated_at=$2 WHERE id=$1::uuid`, sessionID, in.Now)
	if err != nil {
		return Evidence{}, err
	}
	transition, err := r.completeStageIfReadyTx(ctx, tx, in.Context, Stage(stage), in.Now)
	if err != nil && !errors.Is(err, ErrEvidenceNeeded) {
		return Evidence{}, err
	}
	challenge, loadErr := loadChallengeTx(ctx, tx, challengeID)
	if loadErr != nil {
		return Evidence{}, loadErr
	}
	if emitErr := emitEventTx(ctx, tx, "dispatch.evidence.accepted.v1", challenge, map[string]any{"evidenceRef": evidenceRef, "evidenceType": evidenceType}); emitErr != nil {
		return Evidence{}, emitErr
	}
	result := Evidence{EvidenceRef: evidenceRef, MediaRef: in.Command.MediaRef, Stage: Stage(stage), EvidenceType: evidenceType,
		ManifestRef: manifestRef, ManifestVersion: manifestVersion, CapturedAt: in.Command.CapturedAt, CustodyTransition: transition}
	return result, tx.Commit(ctx)
}

func (r *Repository) completeStageIfReadyTx(ctx context.Context, tx pgx.Tx, context Context, stage Stage, now time.Time) (string, error) {
	var scansRequired, photoRequired bool
	var minEvidence int
	err := tx.QueryRow(ctx, `SELECT parcel_scans_required,photo_required,min_evidence_count FROM dispatch_evidence_requirements
		WHERE context_key=$1 AND stage=$2 ORDER BY created_at DESC LIMIT 1`, context.ContextKey(stage), stage).
		Scan(&scansRequired, &photoRequired, &minEvidence)
	if err != nil {
		return "", err
	}
	if scansRequired {
		var expected, scanned int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM dispatch_stop_expected_parcels WHERE mission_stop_id=$1::uuid AND manifest_id=$2::uuid`, context.StopID, context.ManifestID).Scan(&expected); err != nil {
			return "", err
		}
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM dispatch_parcel_scan_events WHERE mission_id=$1::uuid AND stop_id=$2::uuid AND manifest_id=$3::uuid`, context.MissionID, context.StopID, context.ManifestID).Scan(&scanned); err != nil {
			return "", err
		}
		if expected == 0 || scanned != expected {
			return "", ErrEvidenceNeeded
		}
	}
	if photoRequired || minEvidence > 0 {
		var count int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM dispatch_evidence_refs WHERE trip_id=$1::uuid AND stage=$2 AND manifest_ref=$3 AND manifest_version=$4`, context.TripID, stage, context.ManifestRef, context.ManifestVersion).Scan(&count); err != nil {
			return "", err
		}
		if count < minEvidence {
			return "", ErrEvidenceNeeded
		}
	}
	if context.MissionID == "" {
		if stage == StageDelivery || stage == StageReturn {
			_, err = tx.Exec(ctx, `UPDATE trips SET status='proof_submitted',updated_at=$2 WHERE id=$1::uuid AND status='in_progress'`, context.TripID, now)
			return "DELIVERY_VERIFIED", err
		}
		return "PICKUP_VERIFIED", nil
	}
	var missionState string
	if err = tx.QueryRow(ctx, `SELECT state FROM dispatch_delivery_missions WHERE id=$1::uuid FOR UPDATE`, context.MissionID).Scan(&missionState); err != nil {
		return "", err
	}
	if stage == StagePickup {
		if missionState == "PICKED_UP" {
			return "CUSTODY_ACTIVATED", nil
		}
		_, err = tx.Exec(ctx, `INSERT INTO dispatch_active_custody_manifest_parcels(parcel_id,manifest_id,mission_id,activated_at)
			SELECT parcel_id,$1::uuid,$2::uuid,$3 FROM dispatch_manifest_parcels WHERE manifest_id=$1::uuid
			ON CONFLICT(parcel_id) DO NOTHING`, context.ManifestID, context.MissionID, now)
		if err != nil {
			return "", err
		}
		var expected, activated int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM dispatch_manifest_parcels WHERE manifest_id=$1::uuid`, context.ManifestID).Scan(&expected); err != nil {
			return "", err
		}
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM dispatch_active_custody_manifest_parcels WHERE manifest_id=$1::uuid AND mission_id=$2::uuid`, context.ManifestID, context.MissionID).Scan(&activated); err != nil {
			return "", err
		}
		if expected == 0 || activated != expected {
			return "", ErrConflict
		}
		_, err = tx.Exec(ctx, `UPDATE dispatch_parcel_manifests SET status='CUSTODY_ACTIVE' WHERE id=$1::uuid`, context.ManifestID)
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE dispatch_delivery_missions SET state='PICKED_UP',version=version+1,updated_at=$2 WHERE id=$1::uuid`, context.MissionID, now)
		}
		if err == nil {
			_, err = tx.Exec(ctx, `UPDATE dispatch_parcels p SET state='IN_CUSTODY',version=version+1,updated_at=$2 FROM dispatch_manifest_parcels mp WHERE mp.manifest_id=$1::uuid AND p.id=mp.parcel_id`, context.ManifestID, now)
		}
		return "CUSTODY_ACTIVATED", err
	}
	terminalMissionState := "DELIVERED"
	terminalParcelState := "DELIVERED"
	transition := "CUSTODY_RELEASED"
	if stage == StageReturn {
		terminalMissionState = "RETURNED"
		terminalParcelState = "RETURNED"
		transition = "RETURN_CUSTODY_RELEASED"
	}
	if missionState == terminalMissionState {
		return "CUSTODY_RELEASED", nil
	}
	var expected, active int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM dispatch_manifest_parcels WHERE manifest_id=$1::uuid`, context.ManifestID).Scan(&expected); err != nil {
		return "", err
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM dispatch_active_custody_manifest_parcels WHERE manifest_id=$1::uuid AND mission_id=$2::uuid`, context.ManifestID, context.MissionID).Scan(&active); err != nil {
		return "", err
	}
	if expected == 0 || active != expected {
		return "", ErrConflict
	}
	_, err = tx.Exec(ctx, `DELETE FROM dispatch_active_custody_manifest_parcels WHERE manifest_id=$1::uuid AND mission_id=$2::uuid`, context.ManifestID, context.MissionID)
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE dispatch_parcel_manifests SET status='COMPLETED' WHERE id=$1::uuid`, context.ManifestID)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE dispatch_delivery_missions SET state=$3,version=version+1,updated_at=$2 WHERE id=$1::uuid`, context.MissionID, now, terminalMissionState)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE dispatch_parcels p SET state=$3,version=version+1,updated_at=$2 FROM dispatch_manifest_parcels mp WHERE mp.manifest_id=$1::uuid AND p.id=mp.parcel_id`, context.ManifestID, now, terminalParcelState)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE trips SET status='proof_submitted',updated_at=$2 WHERE id=$1::uuid AND status='in_progress'`, context.TripID, now)
	}
	return transition, err
}

func (r *Repository) MarkMissionInTransit(ctx context.Context, tripID string, now time.Time) error {
	_, err := r.db.Exec(ctx, `UPDATE dispatch_delivery_missions SET state='IN_TRANSIT',version=version+1,updated_at=$2
		WHERE trip_id=$1::uuid AND state='PICKED_UP'`, tripID, now)
	return err
}

func (r *Repository) CompleteStageIfReady(ctx context.Context, context Context, stage Stage, now time.Time) (string, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	transition, err := r.completeStageIfReadyTx(ctx, tx, context, stage, now)
	if err != nil {
		return "", err
	}
	return transition, tx.Commit(ctx)
}

func (r *Repository) StageReady(ctx context.Context, tripID string, stage Stage, evidenceRequired bool) (bool, error) {
	context, err := r.ResolveByTrip(ctx, tripID, stage, "")
	if err != nil {
		return false, err
	}
	var consumed int
	err = r.db.QueryRow(ctx, `SELECT count(*) FROM dispatch_handover_challenges WHERE context_key=$1 AND status='CONSUMED'`, context.ContextKey(stage)).Scan(&consumed)
	if err != nil || consumed == 0 {
		return false, err
	}
	if evidenceRequired {
		var evidence int
		err = r.db.QueryRow(ctx, `SELECT count(*) FROM dispatch_evidence_refs WHERE trip_id=$1::uuid AND stage=$2 AND manifest_ref=$3 AND manifest_version=$4`, tripID, stage, context.ManifestRef, context.ManifestVersion).Scan(&evidence)
		if err != nil || evidence == 0 {
			return false, err
		}
	}
	if context.MissionID != "" {
		var active, expected int
		err = r.db.QueryRow(ctx, `SELECT count(*) FROM dispatch_active_custody_manifest_parcels WHERE mission_id=$1::uuid AND manifest_id=$2::uuid`, context.MissionID, context.ManifestID).Scan(&active)
		if err != nil {
			return false, err
		}
		if stage == StagePickup {
			if err = r.db.QueryRow(ctx, `SELECT count(*) FROM dispatch_manifest_parcels WHERE manifest_id=$1::uuid`, context.ManifestID).Scan(&expected); err != nil {
				return false, err
			}
			return expected > 0 && active == expected, nil
		}
		return active == 0, nil
	}
	return true, nil
}

func (r *Repository) InvalidateTripChallenges(ctx context.Context, tripID, providerID, reason string, now time.Time) error {
	_, err := r.db.Exec(ctx, `UPDATE dispatch_handover_challenges SET status='INVALIDATED',invalidated_at=$3,invalidated_reason=$4,
		updated_at=$3,optimistic_version=optimistic_version+1 WHERE trip_id=$1::uuid AND status='ACTIVE'
		AND (NULLIF($2,'')::uuid IS NULL OR assigned_provider_id=NULLIF($2,'')::uuid)`, tripID, providerID, now, reason)
	return err
}

func (r *Repository) Expire(ctx context.Context, now time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = 100
	}
	tag, err := r.db.Exec(ctx, `WITH due AS (SELECT id FROM dispatch_handover_challenges WHERE status='ACTIVE' AND expires_at<=$1 ORDER BY expires_at FOR UPDATE SKIP LOCKED LIMIT $2)
		UPDATE dispatch_handover_challenges c SET status='EXPIRED',updated_at=$1,optimistic_version=optimistic_version+1 FROM due WHERE c.id=due.id`, now, limit)
	return tag.RowsAffected(), err
}

const challengeSelect = `SELECT id::text,challenge_ref,context_key,version,stage,mission_ref,stop_ref,trip_id::text,booking_id::text,
	manifest_ref,manifest_version,COALESCE(assigned_provider_id::text,''),intended_recipient_ref,recipient_type,masked_destination,
	otp_hash,qr_token_hash,expires_at,attempt_count,max_attempts,resend_count,resend_cooldown_until,status,
	notification_state,notification_channels,notification_attempt_count,source_version,optimistic_version,created_at,updated_at FROM dispatch_handover_challenges`

type rowScanner interface{ Scan(...any) error }

func scanChallenge(row rowScanner) (Challenge, error) {
	var challenge Challenge
	err := row.Scan(&challenge.ID, &challenge.ChallengeRef, &challenge.ContextKey, &challenge.Version, &challenge.Stage,
		&challenge.MissionRef, &challenge.StopRef, &challenge.TripID, &challenge.BookingID, &challenge.ManifestRef,
		&challenge.ManifestVersion, &challenge.AssignedProviderID, &challenge.IntendedRecipientRef, &challenge.RecipientType,
		&challenge.MaskedDestination, &challenge.OTPHash, &challenge.QRTokenHash, &challenge.ExpiresAt, &challenge.AttemptCount,
		&challenge.MaxAttempts, &challenge.ResendCount, &challenge.CooldownUntil, &challenge.Status, &challenge.NotificationState,
		&challenge.NotificationChannels, &challenge.NotificationAttemptCount, &challenge.SourceVersion, &challenge.OptimisticVersion, &challenge.CreatedAt, &challenge.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Challenge{}, ErrNotFound
	}
	return challenge, err
}

func lockChallengeTx(ctx context.Context, tx pgx.Tx, challengeRef string) (Challenge, error) {
	return scanChallenge(tx.QueryRow(ctx, challengeSelect+` WHERE challenge_ref=$1 FOR UPDATE`, challengeRef))
}

func loadChallengeTx(ctx context.Context, tx pgx.Tx, id string) (Challenge, error) {
	return scanChallenge(tx.QueryRow(ctx, challengeSelect+` WHERE id=$1::uuid`, id))
}

func activeChallengeTx(ctx context.Context, tx pgx.Tx, contextKey string) (Challenge, bool, error) {
	challenge, err := scanChallenge(tx.QueryRow(ctx, challengeSelect+` WHERE context_key=$1 AND status='ACTIVE' FOR UPDATE`, contextKey))
	if errors.Is(err, ErrNotFound) {
		return Challenge{}, false, nil
	}
	return challenge, err == nil, err
}

func activeChallengeForStageTx(ctx context.Context, tx pgx.Tx, missionRef, stopRef string, stage Stage) (Challenge, bool, error) {
	challenge, err := scanChallenge(tx.QueryRow(ctx, challengeSelect+` WHERE mission_ref=$1 AND stop_ref=$2 AND stage=$3 AND status='ACTIVE' FOR UPDATE`, missionRef, stopRef, stage))
	if errors.Is(err, ErrNotFound) {
		return Challenge{}, false, nil
	}
	return challenge, err == nil, err
}

func findOperationTx(ctx context.Context, tx pgx.Tx, caller, operation, fingerprint string) (string, bool, error) {
	var challengeID, stored string
	err := tx.QueryRow(ctx, `SELECT COALESCE(challenge_id::text,''),request_fingerprint FROM dispatch_handover_operations WHERE caller_scope=$1 AND operation_ref=$2`, caller, operation).Scan(&challengeID, &stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if stored != fingerprint {
		return "", false, ErrConflict
	}
	return challengeID, true, nil
}

func insertOperationTx(ctx context.Context, tx pgx.Tx, caller, operation, operationType, fingerprint string, challenge Challenge) error {
	snapshot, _ := json.Marshal(challenge)
	_, err := tx.Exec(ctx, `INSERT INTO dispatch_handover_operations(caller_scope,operation_ref,operation_type,request_fingerprint,challenge_id,result_snapshot)
		VALUES($1,$2,$3,$4,$5::uuid,$6)`, caller, operation, operationType, fingerprint, challenge.ID, snapshot)
	return err
}

func emitEventTx(ctx context.Context, tx pgx.Tx, eventType string, challenge Challenge, extra map[string]any) error {
	payload := map[string]any{"challengeRef": challenge.ChallengeRef, "challengeVersion": challenge.Version,
		"stage": challenge.Stage, "missionRef": challenge.MissionRef, "stopRef": challenge.StopRef,
		"manifestRef": challenge.ManifestRef, "manifestVersion": challenge.ManifestVersion,
		"status": challenge.Status, "occurredAt": time.Now().UTC()}
	for key, value := range extra {
		payload[key] = value
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	_, err := tx.Exec(ctx, `INSERT INTO dispatch_marketplace_outbox(event_type,aggregate_type,aggregate_id,aggregate_version,payload,payload_hash,target_service)
		VALUES($1,'handover_challenge',$2,$3,$4,$5,'vendor-service') ON CONFLICT DO NOTHING`, eventType, challenge.ChallengeRef, challenge.Version, raw, hex.EncodeToString(sum[:]))
	return err
}

func eventForVerification(stage Stage) string {
	switch stage {
	case StagePickup:
		return "dispatch.pickup.verified.v1"
	case StageDelivery:
		return "dispatch.receiver.verified.v1"
	case StageReturn:
		return "dispatch.return.completed.v1"
	default:
		return "dispatch.provider_transfer.verified.v1"
	}
}

func fingerprint(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
