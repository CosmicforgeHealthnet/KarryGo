package providerauthhttp

// startAuthRequest is phone-only: taxi driver auth has no email path.
type startAuthRequest struct {
	Phone string `json:"phone"`
}

type verifyAuthRequest struct {
	Phone       string  `json:"phone"`
	OTP         string  `json:"otp"`
	ChallengeID string  `json:"challenge_id"`
	DeviceID    *string `json:"device_id"`
	// Optional referral code: from a shared link, or typed on the sign-up
	// screen. Used only when this verification creates the account.
	ReferralLinkCode string `json:"referral_link_code"`
	ReferralCode     string `json:"referral_code"`
}

type refreshRequest struct {
	RefreshToken string  `json:"refresh_token"`
	DeviceID     *string `json:"device_id"`
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type changePhoneStartRequest struct {
	Phone string `json:"phone"`
}

type changePhoneVerifyRequest struct {
	Phone       string `json:"phone"`
	OTP         string `json:"otp"`
	ChallengeID string `json:"challenge_id"`
}

// startAuthResponse is the wire shape for an issued OTP challenge.
//
// DebugOTP is the only field the usecase does not serialize itself: the result
// struct tags it json:"-" so it cannot leak by accident, and the handler copies
// it here only when TAXI_OTP_DEBUG is on. omitempty keeps it absent otherwise.
type startAuthResponse struct {
	ChallengeID string `json:"challenge_id"`
	ExpiresIn   int64  `json:"expires_in"`
	DebugOTP    string `json:"debug_otp,omitempty"`
}
