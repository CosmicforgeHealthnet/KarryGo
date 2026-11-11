package usecases

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/models"
	"cosmicforge/logistics/shared/go/apperrors"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

type IdentityProfile struct {
	IdentityID          string     `json:"identityId"`
	FullName            string     `json:"fullName"`
	Email               string     `json:"email"`
	Phone               string     `json:"phone"`
	EmailVerifiedAt     *time.Time `json:"emailVerifiedAt"`
	IsOrganizationOwner bool       `json:"isOrganizationOwner"`
}

func (s *Service) CurrentProfile(ctx context.Context, id, sessionID, requestID string) (*IdentityProfile, error) {
	session, err := s.opts.Sessions.GetByID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.Status != models.SessionStatusActive || !s.opts.Now().Before(session.RefreshExpiresAt) {
		return nil, apperrors.Unauthorized("Your session expired. Sign in again.", nil)
	}
	if session.IdentityID != id {
		return nil, apperrors.Forbidden("Account does not match this session.", nil)
	}
	orgID := ""
	if session.OrganizationID != nil {
		orgID = *session.OrganizationID
	}
	identity, err := s.IdentityByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !identity.Usable(s.opts.Now()) {
		return nil, apperrors.Forbidden("This account is not eligible for access.", nil)
	}
	result := &IdentityProfile{IdentityID: id, Email: identity.EmailNormalized, Phone: identity.PhoneE164, EmailVerifiedAt: identity.EmailVerifiedAt}
	if orgID == "" {
		// An unscoped session can resolve exactly one active membership, like the
		// existing tenant resolver. Ambiguous memberships require organization selection.
		var candidate string
		err := s.opts.Pool.QueryRow(ctx, `SELECT min(organization_id) FROM fleet_partner_organization_users WHERE identity_id=$1 AND status='ACTIVE' HAVING count(*)=1`, id).Scan(&candidate)
		if err == nil {
			orgID = candidate
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	if orgID != "" {
		err = s.opts.Pool.QueryRow(ctx, `SELECT full_name,is_owner FROM fleet_partner_organization_users WHERE identity_id=$1 AND organization_id=$2 AND status='ACTIVE'`, id, orgID).Scan(&result.FullName, &result.IsOrganizationOwner)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}
	if result.FullName == "Organization Owner" {
		result.FullName = ""
	}
	if err := s.opts.SecurityEvents.Record(ctx, nil, models.SecurityEvent{EventType: "identity.profile_viewed", IdentityID: &id, RequestID: requestID, Detail: map[string]any{"projection": "own_contact"}}); err != nil {
		return nil, err
	}
	return result, nil
}
