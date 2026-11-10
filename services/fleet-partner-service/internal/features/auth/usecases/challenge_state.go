package usecases

import (
    "time"

    "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/models"
    "cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
)

// challengeVerificationError preserves the difference between an expired OTP
// and a newer OTP having replaced it. Neither an old nor a consumed code may
// authenticate the user, even when the browser's countdown still has time.
func challengeVerificationError(challenge *models.Challenge, now time.Time) error {
    switch {
    case challenge.Status == models.ChallengeStatusConsumed || challenge.Status == models.ChallengeStatusVerified:
        return errorcodes.Unauthorized(errorcodes.OTPChallengeConsumed, "This verification request has already been used. Start again.")
    case challenge.Status == models.ChallengeStatusSuperseded:
        return errorcodes.Unauthorized(errorcodes.OTPChallengeSuperseded, "A newer verification request replaced this one. Start again.")
    case challenge.Status == models.ChallengeStatusLocked || challenge.FailedAttempts >= challenge.MaximumAttempts:
        return errorcodes.Unauthorized(errorcodes.OTPAttemptLimitExceeded, "Too many incorrect attempts. Start again.")
    case challenge.Status != models.ChallengeStatusPending || !now.Before(challenge.ExpiresAt):
        return errorcodes.Unauthorized(errorcodes.OTPChallengeExpired, "This verification code has expired. Request a new one.")
    default:
        return nil
    }
}
