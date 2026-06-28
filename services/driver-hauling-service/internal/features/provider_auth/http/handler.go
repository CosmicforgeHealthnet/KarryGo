package providerauthhttp

import (
	"github.com/gin-gonic/gin"

	providerauthusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

type AuthHandler struct {
	auth *providerauthusecases.AuthService
}

func NewAuthHandler(auth *providerauthusecases.AuthService) *AuthHandler {
	return &AuthHandler{auth: auth}
}

func (h *AuthHandler) StartAuth(c *gin.Context) {
	var req startAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	result, err := h.auth.StartAuth(c.Request.Context(), providerauthusecases.StartAuthInput{
		Phone: req.Phone,
		Email: req.Email,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	respondCreated(c, result)
}

func (h *AuthHandler) VerifyAuth(c *gin.Context) {
	var req verifyAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	result, err := h.auth.VerifyAuth(c.Request.Context(), providerauthusecases.VerifyAuthInput{
		Phone:       req.Phone,
		Email:       req.Email,
		OTP:         req.OTP,
		ChallengeID: req.ChallengeID,
		DeviceID:    req.DeviceID,
		UserAgent:   c.Request.UserAgent(),
		IPAddress:   c.ClientIP(),
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	respondOK(c, result)
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	result, err := h.auth.Refresh(c.Request.Context(), providerauthusecases.RefreshInput{
		RefreshToken: req.RefreshToken,
		DeviceID:     req.DeviceID,
		UserAgent:    c.Request.UserAgent(),
		IPAddress:    c.ClientIP(),
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	respondOK(c, result)
}

func (h *AuthHandler) Logout(c *gin.Context) {
	var req logoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	if err := h.auth.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		httpx.Abort(c, err)
		return
	}

	respondOK(c, gin.H{"logged_out": true})
}

func (h *AuthHandler) ChangePhoneStart(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	var req changePhoneStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	result, err := h.auth.ChangePhoneStart(c.Request.Context(), claims.Subject, req.Phone)
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	respondCreated(c, result)
}

func (h *AuthHandler) ChangePhoneVerify(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	var req changePhoneVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	provider, err := h.auth.ChangePhoneVerify(c.Request.Context(), claims.Subject, req.Phone, req.OTP, req.ChallengeID)
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	respondOK(c, provider)
}

func (h *AuthHandler) Me(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	provider, err := h.auth.Me(c.Request.Context(), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	respondOK(c, provider)
}
