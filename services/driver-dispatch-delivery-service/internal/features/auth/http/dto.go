package authhttp

// ── Legacy start (phone-only) ─────────────────────────────────────────────────

type StartRequest struct {
	PhoneNumber string `json:"phone_number"`
	// Email is optional. When provided, the same OTP code is also sent via email.
	Email string `json:"email,omitempty"`
}

// StartResponse is the success payload for POST /api/v1/auth/start,
// POST /api/v1/auth/signup/start, and POST /api/v1/auth/login/start.
// OTP is intentionally absent — it must never appear in HTTP responses.
type StartResponse struct {
	Message          string `json:"message"`
	ExpiresInSeconds int64  `json:"expires_in_seconds"`
}

// ── Signup start ──────────────────────────────────────────────────────────────

type SignupStartRequest struct {
	PhoneNumber string `json:"phone_number"`
	Email       string `json:"email"`
}

// ── Login start ───────────────────────────────────────────────────────────────

type LoginStartRequest struct {
	// Identifier is either a phone number (E.164) or an email address.
	Identifier string `json:"identifier"`
}

// ── Verify ────────────────────────────────────────────────────────────────────

type VerifyRequest struct {
	// Legacy field: phone number.  Honoured when Identifier is absent.
	PhoneNumber string `json:"phone_number"`
	// New field: phone (E.164) or email.  Takes precedence over PhoneNumber.
	Identifier string `json:"identifier"`
	OTPCode    string `json:"otp_code"`
	// Purpose: "login" | "signup" | "" (empty = legacy upsert, backward compat).
	Purpose    string  `json:"purpose"`
	DeviceID   *string `json:"device_id,omitempty"`
	DeviceType *string `json:"device_type,omitempty"`
}

// ── Refresh / Logout ──────────────────────────────────────────────────────────

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type LogoutRequest struct {
	RefreshToken *string `json:"refresh_token,omitempty"`
}

type LogoutResponse struct {
	Message string `json:"message"`
}

// ── Phone change (authenticated) ─────────────────────────────────────────────

type PhoneChangeStartRequest struct {
	NewPhoneNumber string `json:"new_phone_number"`
}

type PhoneChangeVerifyRequest struct {
	NewPhoneNumber string `json:"new_phone_number"`
	OTPCode        string `json:"otp_code"`
}

type PhoneChangeResponse struct {
	Message     string `json:"message"`
	PhoneNumber string `json:"phone_number,omitempty"`
}
