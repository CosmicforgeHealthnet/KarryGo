package usecases

import (
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

func TestOrganizationSourceRequiresActivePersistedSession(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	valid := models.Session{ID: "ses_source", IdentityID: "identity", SessionFamilyID: "family", Status: models.SessionStatusActive, RefreshExpiresAt: now.Add(time.Hour)}
	if err := validateOrganizationSource(&valid, "identity", now); err != nil {
		t.Fatal("active source rejected")
	}
	for _, state := range []string{models.SessionStatusRevoked, models.SessionStatusExpired, models.SessionStatusCompromised, "", "unknown"} {
		t.Run(state, func(t *testing.T) {
			s := valid
			s.Status = state
			if err := validateOrganizationSource(&s, "identity", now); err == nil || apperrors.From(err).Status != 401 {
				t.Fatal("inactive source must be unauthorized")
			}
		})
	}
	for name, mutate := range map[string]func(*models.Session){
		"expired":          func(s *models.Session) { s.RefreshExpiresAt = now.Add(-time.Second) },
		"expiry_boundary":  func(s *models.Session) { s.RefreshExpiresAt = now },
		"foreign_identity": func(s *models.Session) { s.IdentityID = "other" },
		"missing_session":  func(s *models.Session) { s.ID = "" },
		"missing_family":   func(s *models.Session) { s.SessionFamilyID = "" },
		"revoked_at":       func(s *models.Session) { s.RevokedAt = &now },
	} {
		t.Run(name, func(t *testing.T) {
			s := valid
			mutate(&s)
			if err := validateOrganizationSource(&s, "identity", now); err == nil || apperrors.From(err).Status != 401 {
				t.Fatal("invalid source must be unauthorized")
			}
		})
	}
	if err := validateOrganizationSource(nil, "identity", now); err == nil || apperrors.From(err).Status != 401 {
		t.Fatal("missing source accepted")
	}
}
