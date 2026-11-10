package usecases

import (
    "testing"
    "time"

    "cosmicforge/logistics/shared/go/apperrors"
    "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/models"
    "cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
)

func TestChallengeVerificationErrorDistinguishesReplacedFromExpired(t *testing.T) {
    now := time.Date(2026, 11, 10, 18, 0, 0, 0, time.UTC)
    tests := []struct {
        name, status string
        expiry time.Time
        attempts int
        want apperrors.Code
    }{
        {"active", models.ChallengeStatusPending, now.Add(time.Minute), 0, ""},
        {"expired by time", models.ChallengeStatusPending, now.Add(-time.Second), 0, errorcodes.OTPChallengeExpired},
        {"expired by worker", models.ChallengeStatusExpired, now.Add(time.Minute), 0, errorcodes.OTPChallengeExpired},
        {"replaced with time remaining", models.ChallengeStatusSuperseded, now.Add(9*time.Minute), 0, errorcodes.OTPChallengeSuperseded},
        {"consumed", models.ChallengeStatusConsumed, now.Add(time.Minute), 0, errorcodes.OTPChallengeConsumed},
        {"verified", models.ChallengeStatusVerified, now.Add(time.Minute), 0, errorcodes.OTPChallengeConsumed},
        {"locked", models.ChallengeStatusLocked, now.Add(time.Minute), 0, errorcodes.OTPAttemptLimitExceeded},
        {"max attempts", models.ChallengeStatusPending, now.Add(time.Minute), 5, errorcodes.OTPAttemptLimitExceeded},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            ch := &models.Challenge{Status: tc.status, ExpiresAt: tc.expiry, FailedAttempts: tc.attempts, MaximumAttempts: 5}
            got := apperrors.From(challengeVerificationError(ch, now))
            if tc.want == "" {
                if err := challengeVerificationError(ch, now); err != nil {
                    t.Fatalf("expected usable challenge, got %v", err)
                }
                return
            }
            if got.Code != tc.want || got.Status != 401 {
                t.Fatalf("got %s HTTP %d, want %s HTTP 401", got.Code, got.Status, tc.want)
            }
        })
    }
}
