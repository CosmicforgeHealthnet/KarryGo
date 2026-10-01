package providerauthhttp

type startAuthRequest struct {
	Phone string `json:"phone"`
	Email string `json:"email"`
}

type verifyAuthRequest struct {
	Phone       string  `json:"phone"`
	Email       string  `json:"email"`
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
