package authhttp

type StartRequest struct {
	PhoneNumber string `json:"phone_number"`
}

// StartResponse is the success payload for POST /api/v1/auth/start.
// OTP is intentionally absent — it must never appear in HTTP responses.
type StartResponse struct {
	Message          string `json:"message"`
	ExpiresInSeconds int64  `json:"expires_in_seconds"`
}

type VerifyRequest struct {
	PhoneNumber string  `json:"phone_number"`
	OTPCode     string  `json:"otp_code"`
	DeviceID    *string `json:"device_id,omitempty"`
	DeviceType  *string `json:"device_type,omitempty"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type LogoutRequest struct {
	RefreshToken *string `json:"refresh_token,omitempty"`
}

type LogoutResponse struct {
	Message string `json:"message"`
}
