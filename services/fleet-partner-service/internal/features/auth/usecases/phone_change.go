package usecases

import (
	"context"
	"errors"
	"strings"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/models"
	authrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/contact"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/shared/go/apperrors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ChangePhone associates a new unique number after email step-up in the same
// active session. The grant's action is bound to the exact normalized number.
// No ownership or SMS verification of the new phone is implied.
func (s *Service) ChangePhone(ctx context.Context, identityID, sessionID, organizationID, phone, reference string) error {
	normalized, _, err := contact.NormalizePhone(phone, s.opts.DefaultCountryCode)
	if err != nil {
		return apperrors.Validation("Enter a valid phone number.", []apperrors.FieldViolation{{Field: "phone", Message: "Enter a valid phone number with its country code."}})
	}
	action := "phone_change:" + normalized
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		grant, err := s.opts.Challenges.LockForVerification(ctx, tx, reference)
		if err != nil {
			return errorcodes.Forbidden(errorcodes.StepUpRequired, "Confirm the change using your registered email.")
		}
		if grant.Purpose != models.PurposeStepUp || grant.Status != models.ChallengeStatusConsumed || grant.VerifiedAt == nil ||
			!s.opts.Now().Before(grant.VerifiedAt.Add(s.opts.StepUpTTL)) || derefString(grant.IdentityID) != identityID ||
			derefString(grant.SessionID) != sessionID || derefString(grant.OrganizationID) != organizationID || derefString(grant.ActionClass) != action {
			return errorcodes.Forbidden(errorcodes.StepUpRequired, "Confirm this phone change again using your registered email.")
		}
		session, err := s.opts.Sessions.LockByID(ctx, tx, sessionID)
		if err != nil || validateOrganizationSource(session, identityID, s.opts.Now()) != nil {
			return signInUnavailable()
		}
		identity, err := s.opts.Identities.GetByIDUsing(ctx, tx, identityID)
		if err != nil {
			return err
		}
		if !identity.Usable(s.opts.Now()) || identity.EmailVerifiedAt == nil || !strings.EqualFold(identity.EmailNormalized, grant.EmailNormalized) {
			return signInUnavailable()
		}
		existing, err := s.opts.Identities.FindByPhone(ctx, tx, normalized)
		if err != nil && !errors.Is(err, authrepos.ErrNotFound) {
			return err
		}
		if existing != nil && existing.ID != identityID {
			return errorcodes.Conflict(errorcodes.PhoneAlreadyAssociated, "This phone number is already associated with a Karryam Fleet account. Use another number or contact support.")
		}
		tag, err := tx.Exec(ctx, `UPDATE fleet_partner_corporate_identities SET phone_e164=$2,phone_display=$2,phone_bound_at=now(),version=version+1,updated_at=now() WHERE id=$1 AND version=$3`, identityID, normalized, identity.Version)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return errorcodes.Conflict(errorcodes.IdentityPairConflict, "Your account changed. Start again.")
		}
		// A successful change spends the grant permanently, even for the same number.
		_, err = tx.Exec(ctx, `UPDATE fleet_partner_auth_otp_challenges SET action_class='used:' || action_class,updated_at=now() WHERE id=$1`, reference)
		if err != nil {
			return err
		}
		return s.opts.SecurityEvents.Record(ctx, tx, models.SecurityEvent{EventType: "IDENTITY_PHONE_CHANGED", Severity: models.SeverityInfo, IdentityID: &identityID})
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return errorcodes.Conflict(errorcodes.PhoneAlreadyAssociated, "This phone number is already associated with a Karryam Fleet account. Use another number or contact support.")
	}
	return err
}
