package bookingrepo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	bookingmodels "cosmicforge/logistics/services/hauling-service/internal/features/booking/models"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/handover"
)

const secureHandoverService = "hauling"

var ErrSecureHandoverChallengeNotFound = errors.New("secure handover challenge not found")

// SecureHandoverRepository upgrades the existing booking handover authority.
// It is deliberately part of the booking repository, not a second service.
type SecureHandoverRepository interface {
	GetHandoverChallenge(ctx context.Context, bookingID string, stage handover.Step) (handover.CodeRecord, string, error)
	ConsumeHandoverChallenge(ctx context.Context, bookingID string, stage handover.Step, actorID, credential string, secret []byte, proofPhotoURL, signatureURL string) (bookingmodels.Booking, bool, error)
}

// CommerceHandoverBinding is resolved exclusively from the accepted Commerce
// mission row. Vendor never supplies either the booking or customer identity.
type CommerceHandoverBinding struct {
	MissionRef string
	BookingID  string
	CustomerID string
}

func (r *PostgresBookingRepository) CommerceHandoverBinding(ctx context.Context, missionRef string) (CommerceHandoverBinding, error) {
	var binding CommerceHandoverBinding
	err := r.db.QueryRow(ctx, `SELECT mission_ref,booking_id::text,customer_id::text
		FROM commerce_missions WHERE mission_ref=$1 AND status='accepted'`, strings.TrimSpace(missionRef)).Scan(
		&binding.MissionRef, &binding.BookingID, &binding.CustomerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return CommerceHandoverBinding{}, ErrSecureHandoverChallengeNotFound
	}
	return binding, err
}

func (r *PostgresBookingRepository) GetHandoverChallenge(ctx context.Context, bookingID string, stage handover.Step) (handover.CodeRecord, string, error) {
	var rec handover.CodeRecord
	var status string
	err := r.db.QueryRow(ctx, `
		SELECT challenge_ref::text,version,status,otp_hash,qr_token_hash,
		       assigned_provider_id::text,owner_id::text,expires_at,
		       attempt_count,resend_count,last_sent_at,consumed_at
		  FROM hauling_handover_challenges
		 WHERE booking_id=$1 AND stage=$2
		 ORDER BY version DESC LIMIT 1`, bookingID, stage).Scan(
		&rec.ChallengeRef, &rec.Version, &status, &rec.Hash, &rec.QRHash,
		&rec.ProviderID, &rec.OwnerID, &rec.ExpiresAt, &rec.Attempts,
		&rec.ResendCount, &rec.LastSentAt, &rec.VerifiedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return handover.CodeRecord{}, "", ErrSecureHandoverChallengeNotFound
	}
	rec.Service, rec.Stage, rec.BookingID = secureHandoverService, stage, bookingID
	return rec, status, err
}

// ConsumeHandoverChallenge serializes credential verification with resend,
// reassignment and the booking transition. Only one OTP/QR contender can move
// the shared challenge from ACTIVE to CONSUMED.
func (r *PostgresBookingRepository) ConsumeHandoverChallenge(ctx context.Context, bookingID string, stage handover.Step, actorID, credential string, secret []byte, proofPhotoURL, signatureURL string) (bookingmodels.Booking, bool, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return bookingmodels.Booking{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var customerID, providerID, bookingStatus string
	err = tx.QueryRow(ctx, `SELECT customer_id::text,COALESCE(provider_id::text,''),status FROM haulage_bookings WHERE id=$1 FOR UPDATE`, bookingID).Scan(&customerID, &providerID, &bookingStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return bookingmodels.Booking{}, false, apperrors.NotFound("Booking could not be found.", err)
	}
	if err != nil {
		return bookingmodels.Booking{}, false, err
	}
	if (stage == handover.StepPickup && actorID != customerID) || (stage == handover.StepDelivery && actorID != providerID) {
		return bookingmodels.Booking{}, false, apperrors.NotFound("Booking could not be found.", nil)
	}
	if stage == handover.StepPickup && bookingStatus != bookingmodels.StatusArrivedAtPickup {
		return bookingmodels.Booking{}, false, apperrors.Conflict("Pickup verification is not active for this booking.", nil)
	}
	if stage == handover.StepDelivery && bookingStatus != bookingmodels.StatusEnRouteDelivery && bookingStatus != bookingmodels.StatusPickedUp {
		return bookingmodels.Booking{}, false, apperrors.Conflict("Delivery verification is not active for this booking.", nil)
	}

	var rec handover.CodeRecord
	var status string
	err = tx.QueryRow(ctx, `
		SELECT challenge_ref::text,version,status,otp_hash,qr_token_hash,
		       assigned_provider_id::text,owner_id::text,expires_at,
		       attempt_count,resend_count,last_sent_at,consumed_at
		  FROM hauling_handover_challenges
		 WHERE booking_id=$1 AND stage=$2 ORDER BY version DESC LIMIT 1 FOR UPDATE`, bookingID, stage).Scan(
		&rec.ChallengeRef, &rec.Version, &status, &rec.Hash, &rec.QRHash,
		&rec.ProviderID, &rec.OwnerID, &rec.ExpiresAt, &rec.Attempts,
		&rec.ResendCount, &rec.LastSentAt, &rec.VerifiedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return bookingmodels.Booking{}, false, ErrSecureHandoverChallengeNotFound
	}
	if err != nil {
		return bookingmodels.Booking{}, false, err
	}
	if status != "ACTIVE" {
		return bookingmodels.Booking{}, false, apperrors.Conflict("This handover credential is no longer active.", nil)
	}
	if rec.ProviderID != providerID || rec.OwnerID != customerID {
		_, _ = tx.Exec(ctx, `UPDATE hauling_handover_challenges SET status='INVALIDATED',updated_at=now() WHERE challenge_ref=$1 AND status='ACTIVE'`, rec.ChallengeRef)
		if err = tx.Commit(ctx); err != nil {
			return bookingmodels.Booking{}, false, err
		}
		return bookingmodels.Booking{}, false, apperrors.Unauthorized("This credential is no longer valid for the assigned provider.", nil)
	}
	if !time.Now().Before(rec.ExpiresAt) {
		_, _ = tx.Exec(ctx, `UPDATE hauling_handover_challenges SET status='EXPIRED',updated_at=now() WHERE challenge_ref=$1 AND status='ACTIVE'`, rec.ChallengeRef)
		if err = tx.Commit(ctx); err != nil {
			return bookingmodels.Booking{}, false, err
		}
		return bookingmodels.Booking{}, false, apperrors.Unauthorized("This credential has expired.", nil)
	}
	if rec.Attempts >= handover.MaxVerifyAttempts {
		return bookingmodels.Booking{}, false, apperrors.Unauthorized("Too many incorrect attempts. Request a new credential.", nil)
	}

	rec.Service, rec.Stage, rec.BookingID = secureHandoverService, stage, bookingID
	method, expected := "QR", rec.QRHash
	if isNumericOTP(credential) {
		method, expected = "OTP", rec.Hash
	}
	if !handover.VerifySecureCredential(secret, method, rec, credential, expected) {
		_, err = tx.Exec(ctx, `
			UPDATE hauling_handover_challenges
			   SET attempt_count=attempt_count+1,
			       status=CASE WHEN attempt_count+1 >= $2 THEN 'LOCKED' ELSE status END,
			       updated_at=now()
			 WHERE challenge_ref=$1 AND status='ACTIVE'`, rec.ChallengeRef, handover.MaxVerifyAttempts)
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			return bookingmodels.Booking{}, false, err
		}
		return bookingmodels.Booking{}, false, apperrors.Unauthorized("Incorrect credential. Please try again.", nil)
	}

	tag, err := tx.Exec(ctx, `UPDATE hauling_handover_challenges SET status='CONSUMED',consumed_at=now(),consumed_method=$2,updated_at=now() WHERE challenge_ref=$1 AND status='ACTIVE'`, rec.ChallengeRef, method)
	if err != nil || tag.RowsAffected() != 1 {
		return bookingmodels.Booking{}, false, apperrors.Conflict("This handover credential has already been consumed.", err)
	}
	var row pgx.Row
	if stage == handover.StepPickup {
		row = tx.QueryRow(ctx, `UPDATE haulage_bookings SET pickup_code_verified_at=now(),updated_at=now() WHERE id=$1 AND status=$2 RETURNING `+bookingSelectCols, bookingID, bookingmodels.StatusArrivedAtPickup)
	} else {
		row = tx.QueryRow(ctx, `UPDATE haulage_bookings SET status=$2,delivered_at=now(),delivery_code_verified_at=now(),proof_photo_url=$4,signature_url=$5,updated_at=now() WHERE id=$1 AND status IN ($3,$6) RETURNING `+bookingSelectCols,
			bookingID, bookingmodels.StatusDelivered, bookingmodels.StatusPickedUp, strings.TrimSpace(proofPhotoURL), strings.TrimSpace(signatureURL), bookingmodels.StatusEnRouteDelivery)
	}
	updated, err := scanBookingNotFound(row)
	if err != nil {
		return bookingmodels.Booking{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return bookingmodels.Booking{}, false, err
	}
	return updated, true, nil
}

func isNumericOTP(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 6 {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func insertSecureChallenge(ctx context.Context, tx pgx.Tx, rec handover.CodeRecord) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO hauling_handover_challenges(
			challenge_ref,booking_id,stage,version,status,otp_hash,qr_token_hash,
			assigned_provider_id,owner_id,expires_at,attempt_count,resend_count,last_sent_at)
		VALUES($1,$2,$3,$4,'ACTIVE',$5,$6,$7,$8,$9,0,$10,now())`,
		rec.ChallengeRef, rec.BookingID, rec.Stage, rec.Version, rec.Hash, rec.QRHash,
		rec.ProviderID, rec.OwnerID, rec.ExpiresAt, rec.ResendCount)
	return err
}

// insertIssuedSecureChallenge handles a booking that legitimately re-enters a
// handover stage after provider reassignment. The former context is invalidated
// and the newly assigned provider receives the next version.
func insertIssuedSecureChallenge(ctx context.Context, tx pgx.Tx, rec handover.CodeRecord) error {
	var currentVersion int64
	var currentStatus string
	err := tx.QueryRow(ctx, `
		SELECT version,status FROM hauling_handover_challenges
		 WHERE booking_id=$1 AND stage=$2
		 ORDER BY version DESC LIMIT 1 FOR UPDATE`, rec.BookingID, rec.Stage).Scan(&currentVersion, &currentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		if rec.Version != 1 {
			return apperrors.Conflict("The first handover credential must use version 1.", nil)
		}
		return insertSecureChallenge(ctx, tx, rec)
	}
	if err != nil {
		return err
	}
	if rec.Version != currentVersion+1 {
		return apperrors.Conflict("The handover credential version changed; retry the command.", nil)
	}
	if currentStatus == "ACTIVE" {
		if _, err = tx.Exec(ctx, `UPDATE hauling_handover_challenges SET status='INVALIDATED',updated_at=now() WHERE booking_id=$1 AND stage=$2 AND status='ACTIVE'`, rec.BookingID, rec.Stage); err != nil {
			return err
		}
	}
	return insertSecureChallenge(ctx, tx, rec)
}

func supersedeAndInsertSecureChallenge(ctx context.Context, tx pgx.Tx, rec handover.CodeRecord) error {
	var providerID, ownerID, bookingStatus string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(provider_id::text,''),customer_id::text,status FROM haulage_bookings WHERE id=$1 FOR UPDATE`, rec.BookingID).Scan(&providerID, &ownerID, &bookingStatus); err != nil {
		return err
	}
	if providerID != rec.ProviderID || ownerID != rec.OwnerID || bookingStatus == bookingmodels.StatusCancelled || bookingStatus == bookingmodels.StatusCompleted || bookingStatus == bookingmodels.StatusDelivered {
		return apperrors.Conflict("The booking handover context is no longer active.", nil)
	}
	var currentVersion int64
	var currentStatus string
	err := tx.QueryRow(ctx, `
		SELECT version,status FROM hauling_handover_challenges
		 WHERE booking_id=$1 AND stage=$2
		 ORDER BY version DESC LIMIT 1 FOR UPDATE`, rec.BookingID, rec.Stage).Scan(&currentVersion, &currentStatus)
	if errors.Is(err, pgx.ErrNoRows) && rec.Version == 1 {
		return insertSecureChallenge(ctx, tx, rec)
	}
	if err != nil {
		return err
	}
	if rec.Version != currentVersion+1 {
		return apperrors.Conflict("The handover credential changed; retry the resend.", nil)
	}
	if currentStatus == "CONSUMED" || currentStatus == "INVALIDATED" || currentStatus == "SUPERSEDED" {
		return apperrors.Conflict("The handover challenge cannot be resent in its current state.", nil)
	}
	if currentStatus == "ACTIVE" {
		if _, err = tx.Exec(ctx, `UPDATE hauling_handover_challenges SET status='SUPERSEDED',updated_at=now() WHERE booking_id=$1 AND stage=$2 AND status='ACTIVE'`, rec.BookingID, rec.Stage); err != nil {
			return err
		}
	}
	return insertSecureChallenge(ctx, tx, rec)
}
