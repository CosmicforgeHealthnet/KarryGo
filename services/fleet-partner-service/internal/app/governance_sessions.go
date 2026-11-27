package app

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	authmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/models"
	authrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/repositories"
	governancemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/governance/models"
	governanceusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/governance/usecases"
)

// organizationRevocationReason is the machine reason stored on the session
// row; the human reason lives on the audit entry.
const organizationRevocationReason = "organization_revoked"

// governanceSessions adapts the authentication feature's session store to
// the governance SessionAuthority port, so organization session management
// (spec 59.10) reads and revokes the one authoritative session table.
type governanceSessions struct{ sessions *authrepos.SessionRepository }

var _ governanceusecases.SessionAuthority = governanceSessions{}

func (g governanceSessions) ListOrganizationSessions(ctx context.Context, organizationID string) ([]governancemodels.OrganizationSession, error) {
	rows, err := g.sessions.ListForOrganization(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	out := make([]governancemodels.OrganizationSession, 0, len(rows))
	for _, s := range rows {
		out = append(out, organizationSessionView(s))
	}
	return out, nil
}

func (g governanceSessions) RevokeOrganizationSession(ctx context.Context, tx pgx.Tx, organizationID, sessionID, _, requestID, correlationID string) (governancemodels.OrganizationSession, bool, error) {
	// LockByID takes the family advisory lock, so a concurrent refresh
	// cannot rotate a successor past this revocation.
	session, err := g.sessions.LockByID(ctx, tx, sessionID)
	if errors.Is(err, authrepos.ErrNotFound) {
		return governancemodels.OrganizationSession{}, false, governanceusecases.ErrSessionNotFound
	}
	if err != nil {
		return governancemodels.OrganizationSession{}, false, err
	}
	if session.OrganizationID == nil || *session.OrganizationID != organizationID {
		return governancemodels.OrganizationSession{}, false, governanceusecases.ErrSessionNotFound
	}
	revoked, err := g.sessions.RevokeActiveFamilyInOrganization(ctx, tx, session.SessionFamilyID, organizationID, organizationRevocationReason)
	if err != nil {
		return governancemodels.OrganizationSession{}, false, err
	}
	if revoked > 0 {
		if err = g.sessions.AppendHistory(ctx, tx, sessionID, session.IdentityID, session.Status, authmodels.SessionStatusRevoked, organizationRevocationReason, requestID, correlationID); err != nil {
			return governancemodels.OrganizationSession{}, false, err
		}
		if session, err = g.sessions.LockByID(ctx, tx, sessionID); err != nil {
			return governancemodels.OrganizationSession{}, false, err
		}
	}
	return organizationSessionView(*session), revoked > 0, nil
}

func organizationSessionView(s authmodels.Session) governancemodels.OrganizationSession {
	v := governancemodels.OrganizationSession{ID: s.ID, Status: s.Status, LastSeenAt: s.LastSeenAt, CreatedAt: s.CreatedAt, RevokedAt: s.RevokedAt}
	if s.OrganizationID != nil {
		v.OrganizationID = *s.OrganizationID
	}
	if s.OrganizationUserID != nil {
		v.OrganizationUserID = *s.OrganizationUserID
	}
	if s.DeviceLabel != nil {
		v.DeviceLabel = *s.DeviceLabel
	}
	if s.RevocationReason != nil {
		v.RevocationReason = *s.RevocationReason
	}
	return v
}
