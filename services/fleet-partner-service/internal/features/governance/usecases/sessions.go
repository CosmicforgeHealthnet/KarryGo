package usecases

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/governance/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// ErrSessionNotFound is what a SessionAuthority returns for a session that
// does not exist or belongs to another organization; both read the same.
var ErrSessionNotFound = errors.New("governance: session not found")

// SessionAuthority is the slice of the authentication feature's session
// store that organization security management needs (spec 59.10). The
// composition root adapts it; governance never stores sessions itself.
type SessionAuthority interface {
	ListOrganizationSessions(ctx context.Context, organizationID string) ([]models.OrganizationSession, error)
	// RevokeOrganizationSession revokes the organization-scoped active
	// credentials of the session's family in the caller's transaction. It
	// reports whether anything was revoked, so a repeat is a no-op.
	RevokeOrganizationSession(ctx context.Context, tx pgx.Tx, organizationID, sessionID, reason, requestID, correlationID string) (models.OrganizationSession, bool, error)
}

// requireOrganizationWide refuses branch-scoped callers: staff sessions are
// organization-level, so a branch manager must not see every colleague's.
func requireOrganizationWide(t tenant.Context) error {
	if len(t.PermittedBranchIDs) > 0 {
		return apperrors.Forbidden("Organization-wide access is required to manage staff sessions.", nil)
	}
	return nil
}

// ListOrganizationSessions lists the staff sessions scoped to the caller's
// organization. currentSessionID marks the caller's own session.
func (s *Service) ListOrganizationSessions(ctx context.Context, t tenant.Context, currentSessionID string) ([]models.OrganizationSession, error) {
	if err := s.gate(s.o.Flags.SecuritySessions, false); err != nil {
		return nil, err
	}
	if err := requireOrganizationWide(t); err != nil {
		return nil, err
	}
	if s.o.Sessions == nil {
		return nil, apperrors.Unavailable("Session management is unavailable.", nil)
	}
	sessions, err := s.o.Sessions.ListOrganizationSessions(ctx, t.OrganizationID)
	if err != nil {
		return nil, err
	}
	out := make([]models.OrganizationSession, 0, len(sessions))
	for _, v := range sessions {
		v.Current = currentSessionID != "" && v.ID == currentSessionID
		out = append(out, v)
	}
	return out, nil
}

// RevokeOrganizationSession ends a staff member's access through one
// session. The revocation, its audit entry and security.session_revoked.v1
// commit together; revoking an already ended session is a no-op success.
func (s *Service) RevokeOrganizationSession(ctx context.Context, t tenant.Context, sessionID string, in models.SessionRevokeInput) (models.OrganizationSession, error) {
	if err := s.gate(s.o.Flags.SecuritySessions, true); err != nil {
		return models.OrganizationSession{}, err
	}
	if err := requireOrganizationWide(t); err != nil {
		return models.OrganizationSession{}, err
	}
	if s.o.Sessions == nil {
		return models.OrganizationSession{}, apperrors.Unavailable("Session management is unavailable.", nil)
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" || len(reason) > 500 {
		return models.OrganizationSession{}, apperrors.Validation("A reason of up to 500 characters is required.", []apperrors.FieldViolation{{Field: "reason", Message: "required"}})
	}
	var out models.OrganizationSession
	err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		session, revoked, err := s.o.Sessions.RevokeOrganizationSession(ctx, tx, t.OrganizationID, sessionID, reason, t.RequestID, t.CorrelationID)
		if errors.Is(err, ErrSessionNotFound) {
			return apperrors.NotFound("Session not found.", nil)
		}
		if err != nil {
			return err
		}
		out = session
		if !revoked {
			return nil
		}
		return s.recordReason(ctx, tx, t, "security.session_revoked.v1", "session", sessionID, reason, 1, map[string]any{
			"sessionId": sessionID, "organizationUserId": session.OrganizationUserID, "reasonCode": "ORGANIZATION_REVOKED",
		})
	})
	return out, err
}
