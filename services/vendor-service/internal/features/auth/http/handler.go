// Package vendorauthhttp is the thin HTTP layer for Vendor AUTHENTICATION:
// DTO parsing, rate limiting, and delegation to vendorauthusecases.AuthService
// — no business rules live here (CLAUDE.md, "Keep handlers thin").
package vendorauthhttp

import (
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"

	vendorauthusecases "cosmicforge/logistics/services/vendor-service/internal/features/auth/usecases"
	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/ratelimit"
)

// RateLimits configures the per-bucket max-per-window enforced before each
// flow does any expensive work (Section 5 rule: "apply rate limits before
// expensive work"). Each flow has its own bucket/window so one abused
// endpoint never exhausts another's budget.
type RateLimits struct {
	StartMax, StartWindowSeconds               int
	ResendMax, ResendWindowSeconds             int
	VerifyMax, VerifyWindowSeconds             int
	RefreshMax, RefreshWindowSeconds           int
	StepUpStartMax, StepUpStartWindowSeconds   int
	StepUpVerifyMax, StepUpVerifyWindowSeconds int
}

type Handler struct {
	auth    *vendorauthusecases.AuthService
	limiter *ratelimit.Limiter
	limits  RateLimits
}

func NewHandler(auth *vendorauthusecases.AuthService, limiter *ratelimit.Limiter, limits RateLimits) *Handler {
	return &Handler{auth: auth, limiter: limiter, limits: limits}
}

// rateLimitKey uses the normalized identifier when present (Section 5 rule:
// dimensions should include normalized login identity, not only IP) — an IP
// alone is never the sole OTP abuse protection here.
func rateLimitKey(c *gin.Context, identifierFallback string) string {
	if identifierFallback != "" {
		return identifierFallback
	}
	return c.ClientIP()
}

func (h *Handler) Start(c *gin.Context) {
	var req startRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", err))
		return
	}
	key := rateLimitKey(c, req.Phone+req.Email)
	if err := h.limiter.Allow(c.Request.Context(), "auth_start", key, h.limits.StartMax, time.Duration(h.limits.StartWindowSeconds)*time.Second); err != nil {
		httpx.RespondError(c, err)
		return
	}

	result, err := h.auth.Start(c.Request.Context(), vendorauthusecases.StartInput{Phone: req.Phone, Email: req.Email})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respondOK(c, startResponse{ChallengeID: result.ChallengeID, ExpiresIn: result.ExpiresIn, DebugOTP: result.DebugOTP})
}

func (h *Handler) Resend(c *gin.Context) {
	var req resendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", err))
		return
	}
	key := rateLimitKey(c, req.Phone+req.Email)
	if err := h.limiter.Allow(c.Request.Context(), "auth_resend", key, h.limits.ResendMax, time.Duration(h.limits.ResendWindowSeconds)*time.Second); err != nil {
		httpx.RespondError(c, err)
		return
	}

	result, err := h.auth.Resend(c.Request.Context(), vendorauthusecases.ResendInput{Phone: req.Phone, Email: req.Email, ChallengeID: req.ChallengeID})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respondOK(c, startResponse{ChallengeID: result.ChallengeID, ExpiresIn: result.ExpiresIn, DebugOTP: result.DebugOTP})
}

func (h *Handler) Verify(c *gin.Context) {
	var req verifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", err))
		return
	}
	key := rateLimitKey(c, req.Phone+req.Email)
	if err := h.limiter.Allow(c.Request.Context(), "auth_verify", key, h.limits.VerifyMax, time.Duration(h.limits.VerifyWindowSeconds)*time.Second); err != nil {
		httpx.RespondError(c, err)
		return
	}

	var deviceID *string
	if req.DeviceID != "" {
		deviceID = &req.DeviceID
	}
	result, err := h.auth.Verify(c.Request.Context(), vendorauthusecases.VerifyInput{
		Phone: req.Phone, Email: req.Email, OTP: req.OTP, ChallengeID: req.ChallengeID, VendorID: req.VendorID,
		DeviceID: deviceID, UserAgent: c.Request.UserAgent(), IPAddress: c.ClientIP(),
	})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	if result.RequiresVendorChoice {
		respondOK(c, verifyResponse{RequiresVendorChoice: true, AvailableVendorIDs: result.AvailableVendorIDs})
		return
	}
	respondOK(c, verifyResponse{
		AccessToken: result.Tokens.AccessToken, RefreshToken: result.Tokens.RefreshToken, ExpiresIn: result.Tokens.ExpiresIn,
		StaffID: result.Tokens.StaffID, VendorID: result.Tokens.VendorID,
	})
}

func (h *Handler) Register(c *gin.Context) {
	var req struct {
		verifyRequest
		LegalName string `json:"legal_name"`
		Currency  string `json:"currency"`
		Timezone  string `json:"timezone"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", err))
		return
	}
	key := rateLimitKey(c, req.Phone+req.Email)
	if err := h.limiter.Allow(c.Request.Context(), "auth_verify", key, h.limits.VerifyMax, time.Duration(h.limits.VerifyWindowSeconds)*time.Second); err != nil {
		httpx.RespondError(c, err)
		return
	}

	var deviceID *string
	if req.DeviceID != "" {
		deviceID = &req.DeviceID
	}
	result, err := h.auth.Register(c.Request.Context(), vendorauthusecases.VerifyInput{
		Phone: req.Phone, Email: req.Email, OTP: req.OTP, ChallengeID: req.ChallengeID, VendorID: req.VendorID,
		DeviceID: deviceID, UserAgent: c.Request.UserAgent(), IPAddress: c.ClientIP(),
	}, vendorauthusecases.RegistrationInput{LegalName: req.LegalName, Currency: req.Currency, Timezone: req.Timezone})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	if result.RequiresVendorChoice {
		respondOK(c, verifyResponse{RequiresVendorChoice: true, AvailableVendorIDs: result.AvailableVendorIDs})
		return
	}
	respondOK(c, verifyResponse{
		AccessToken: result.Tokens.AccessToken, RefreshToken: result.Tokens.RefreshToken, ExpiresIn: result.Tokens.ExpiresIn,
		StaffID: result.Tokens.StaffID, VendorID: result.Tokens.VendorID,
	})
}

func (h *Handler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", err))
		return
	}
	if req.RefreshToken == "" {
		httpx.RespondError(c, apperrors.Validation("Check your details.", []apperrors.FieldViolation{{Field: "refresh_token", Message: "Refresh token is required."}}))
		return
	}
	if err := h.limiter.Allow(c.Request.Context(), "auth_refresh", rateLimitKey(c, req.RefreshToken), h.limits.RefreshMax, time.Duration(h.limits.RefreshWindowSeconds)*time.Second); err != nil {
		httpx.RespondError(c, err)
		return
	}

	var deviceID *string
	if req.DeviceID != "" {
		deviceID = &req.DeviceID
	}
	result, err := h.auth.Refresh(c.Request.Context(), vendorauthusecases.RefreshInput{
		RefreshToken: req.RefreshToken, DeviceID: deviceID, UserAgent: c.Request.UserAgent(), IPAddress: c.ClientIP(),
	})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respondOK(c, verifyResponse{AccessToken: result.AccessToken, RefreshToken: result.RefreshToken, ExpiresIn: result.ExpiresIn, StaffID: result.StaffID, VendorID: result.VendorID})
}

func (h *Handler) Logout(c *gin.Context) {
	var req logoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", err))
		return
	}
	if req.RefreshToken == "" {
		respondOK(c, gin.H{"logged_out": true})
		return
	}
	if err := h.auth.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		httpx.RespondError(c, err)
		return
	}
	respondOK(c, gin.H{"logged_out": true})
}

func (h *Handler) StepUpStart(c *gin.Context) {
	staffCtx, ok := authctx.FromContext(c)
	if !ok {
		httpx.RespondError(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	var req stepUpStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", err))
		return
	}
	if err := h.limiter.Allow(c.Request.Context(), "step_up_start", staffCtx.StaffID, h.limits.StepUpStartMax, time.Duration(h.limits.StepUpStartWindowSeconds)*time.Second); err != nil {
		httpx.RespondError(c, err)
		return
	}

	result, err := h.auth.StepUpStart(c.Request.Context(), vendorauthusecases.StepUpStartInput{
		SessionID: staffCtx.SessionID, StaffID: staffCtx.StaffID, VendorID: staffCtx.VendorID, Purpose: req.Purpose,
	})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respondOK(c, startResponse{ChallengeID: result.ChallengeID, ExpiresIn: result.ExpiresIn, DebugOTP: result.DebugOTP})
}

func (h *Handler) StepUpVerify(c *gin.Context) {
	staffCtx, ok := authctx.FromContext(c)
	if !ok {
		httpx.RespondError(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}
	var req stepUpVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Check your details.", err))
		return
	}
	if err := h.limiter.Allow(c.Request.Context(), "step_up_verify", staffCtx.StaffID, h.limits.StepUpVerifyMax, time.Duration(h.limits.StepUpVerifyWindowSeconds)*time.Second); err != nil {
		httpx.RespondError(c, err)
		return
	}

	result, err := h.auth.StepUpVerify(c.Request.Context(), vendorauthusecases.StepUpVerifyInput{
		SessionID: staffCtx.SessionID, StaffID: staffCtx.StaffID, VendorID: staffCtx.VendorID,
		Purpose: req.Purpose, ChallengeID: req.ChallengeID, OTP: req.OTP,
	})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respondOK(c, stepUpVerifyResponse{GrantID: result.GrantID, ExpiresIn: result.ExpiresIn})
}

func respondOK(c *gin.Context, data any) {
	c.JSON(200, gin.H{"success": true, "data": data})
}
