package vendorauthhttp

type startRequest struct {
	Phone string `json:"phone"`
	Email string `json:"email"`
}

type startResponse struct {
	ChallengeID string `json:"challenge_id"`
	ExpiresIn   int64  `json:"expires_in"`
	DebugOTP    string `json:"debug_otp,omitempty"`
}

type resendRequest struct {
	Phone       string `json:"phone"`
	Email       string `json:"email"`
	ChallengeID string `json:"challenge_id"`
}

type verifyRequest struct {
	Phone       string `json:"phone"`
	Email       string `json:"email"`
	OTP         string `json:"otp"`
	ChallengeID string `json:"challenge_id"`
	VendorID    string `json:"vendor_id"`
	DeviceID    string `json:"device_id"`
}

type verifyResponse struct {
	AccessToken          string   `json:"access_token,omitempty"`
	RefreshToken         string   `json:"refresh_token,omitempty"`
	ExpiresIn            int64    `json:"expires_in,omitempty"`
	StaffID              string   `json:"staff_id,omitempty"`
	VendorID             string   `json:"vendor_id,omitempty"`
	RequiresVendorChoice bool     `json:"requires_vendor_choice,omitempty"`
	AvailableVendorIDs   []string `json:"available_vendor_ids,omitempty"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
	DeviceID     string `json:"device_id"`
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type stepUpStartRequest struct {
	Purpose string `json:"purpose"`
}

type stepUpVerifyRequest struct {
	Purpose     string `json:"purpose"`
	ChallengeID string `json:"challenge_id"`
	OTP         string `json:"otp"`
}

type stepUpVerifyResponse struct {
	GrantID   string `json:"grant_id"`
	ExpiresIn int64  `json:"expires_in"`
}
