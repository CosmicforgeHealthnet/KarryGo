package authhttp

import (
	nethttp "net/http"

	"github.com/gin-gonic/gin"

	authusecases "cosmicforge/logistics/services/customer-service/internal/features/auth/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

type AuthHandler struct {
	auth *authusecases.AuthService
}

func NewAuthHandler(auth *authusecases.AuthService) *AuthHandler {
	return &AuthHandler{auth: auth}
}

func (h *AuthHandler) StartAuth(c *gin.Context) {
	var request startAuthRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	result, err := h.auth.StartAuth(c.Request.Context(), authusecases.StartAuthInput{
		Phone: request.Phone,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	respond(c, nethttp.StatusCreated, result)
}

func (h *AuthHandler) VerifyAuth(c *gin.Context) {
	var request verifyAuthRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	result, err := h.auth.VerifyAuth(c.Request.Context(), authusecases.VerifyAuthInput{
		Phone:       request.Phone,
		OTP:         request.OTP,
		ChallengeID: request.ChallengeID,
		DeviceID:    request.DeviceID,
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
	var request refreshRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	result, err := h.auth.Refresh(c.Request.Context(), authusecases.RefreshInput{
		RefreshToken: request.RefreshToken,
		DeviceID:     request.DeviceID,
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
	var request logoutRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	if err := h.auth.Logout(c.Request.Context(), request.RefreshToken); err != nil {
		httpx.Abort(c, err)
		return
	}

	respondOK(c, gin.H{"logged_out": true})
}

func (h *AuthHandler) Me(c *gin.Context) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return
	}

	customer, err := h.auth.Me(c.Request.Context(), claims.Subject)
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	respondOK(c, customer)
}
