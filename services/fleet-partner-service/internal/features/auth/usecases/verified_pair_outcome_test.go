package usecases

import (
    "errors"
    "testing"
    "time"

    "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/models"
    authrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/repositories"
)

func TestVerifiedPairOutcomePreservesPrivacyAndBlocksUnverifiedAccountAccess(t *testing.T) {
    now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
    active := &models.Identity{Status: models.IdentityStatusActive}
    suspended := &models.Identity{Status: models.IdentityStatusSuspended}
    locked := &models.Identity{Status: models.IdentityStatusLocked}
    expires := now.Add(time.Hour)
    temporarilyLocked := &models.Identity{Status: models.IdentityStatusActive, LockedUntil: &expires}
    databaseFailure := errors.New("database unavailable")
    tests := []struct {
        name string
        identity *models.Identity
        resolveErr error
        wantNext string
        wantErr bool
    }{
        {"existing pair", active, nil, "", false},
        {"new pair", nil, authrepos.ErrNotFound, "", false},
        {"email or phone mismatch", nil, authrepos.ErrIdentityPairConflict, NextStepContactMismatch, false},
        {"suspended identity", suspended, nil, NextStepAccountUnavailable, false},
        {"locked identity", locked, nil, NextStepAccountUnavailable, false},
        {"temporarily locked", temporarilyLocked, nil, NextStepAccountUnavailable, false},
        {"database failure", nil, databaseFailure, "", true},
        {"nil successful lookup", nil, nil, "", true},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            got, err := verifiedPairOutcome(tc.identity, tc.resolveErr, now)
            if got != tc.wantNext || (err != nil) != tc.wantErr {
                t.Fatalf("outcome = %q, error = %v; expected %q, error=%v", got, err, tc.wantNext, tc.wantErr)
            }
        })
    }
}
