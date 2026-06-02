package authhttp

type startAuthRequest struct {
	Phone string `json:"phone"`
}

type verifyAuthRequest struct {
	Phone       string  `json:"phone"`
	OTP         string  `json:"otp"`
	ChallengeID string  `json:"challenge_id"`
	DeviceID    *string `json:"device_id"`
}

type refreshRequest struct {
	RefreshToken string  `json:"refresh_token"`
	DeviceID     *string `json:"device_id"`
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}
