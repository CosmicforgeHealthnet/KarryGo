package authhttp

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	authusecases "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/usecases"
	"karrygo/shared/go/apperrors"
	"karrygo/shared/go/httpx"
)

type Handler struct {
	auth *authusecases.AuthUsecase
}

func NewHandler(auth *authusecases.AuthUsecase) *Handler {
	return &Handler{auth: auth}
}

// Start handles POST /api/v1/auth/start (legacy — phone-only, no login/signup split).
// Kept for backward compatibility with older clients.
func (h *Handler) Start(c *gin.Context) {
	var req StartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "phone_number", Message: "Invalid request body."},
		}))
		return
	}

	result, err := h.auth.Start(c.Request.Context(), authusecases.StartInput{
		PhoneNumber:   req.PhoneNumber,
		CorrelationID: httpx.GetRequestID(c),
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": StartResponse{
			Message:          "OTP sent successfully.",
			ExpiresInSeconds: result.ExpiresInSeconds,
		},
	})
}

// SignupStart handles POST /api/v1/auth/signup/start.
// Validates phone + email, checks neither is already registered, sends OTP.
func (h *Handler) SignupStart(c *gin.Context) {
	var req SignupStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "phone_number", Message: "Invalid request body."},
		}))
		return
	}

	result, err := h.auth.SignupStart(c.Request.Context(), authusecases.SignupStartInput{
		PhoneNumber:   req.PhoneNumber,
		Email:         req.Email,
		CorrelationID: httpx.GetRequestID(c),
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": StartResponse{
			Message:          "OTP sent to your phone number and email.",
			ExpiresInSeconds: result.ExpiresInSeconds,
		},
	})
}

// LoginStart handles POST /api/v1/auth/login/start.
// Accepts a phone number or email as identifier, looks up the existing account,
// returns 404 if not found.
func (h *Handler) LoginStart(c *gin.Context) {
	var req LoginStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "identifier", Message: "Invalid request body."},
		}))
		return
	}

	result, err := h.auth.LoginStart(c.Request.Context(), authusecases.LoginStartInput{
		Identifier:    req.Identifier,
		CorrelationID: httpx.GetRequestID(c),
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": StartResponse{
			Message:          "OTP sent to the contact method associated with your account.",
			ExpiresInSeconds: result.ExpiresInSeconds,
		},
	})
}

// Verify handles POST /api/v1/auth/verify.
// Validates OTP, resolves/creates the dispatch rider identity (based on purpose),
// creates a session, and issues tokens.
//
// Backward compatible: clients that send only phone_number + otp_code (no purpose /
// identifier) continue to work via the legacy upsert path.
func (h *Handler) Verify(c *gin.Context) {
	var req VerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "phone_number", Message: "Invalid request body."},
		}))
		return
	}

	result, err := h.auth.Verify(c.Request.Context(), authusecases.VerifyInput{
		PhoneNumber:   req.PhoneNumber,
		Identifier:    req.Identifier,
		OTPCode:       req.OTPCode,
		Purpose:       req.Purpose,
		CorrelationID: httpx.GetRequestID(c),
		Metadata: authusecases.RequestMetadata{
			DeviceID:   req.DeviceID,
			DeviceType: req.DeviceType,
			IPAddress:  c.ClientIP(),
			UserAgent:  c.Request.UserAgent(),
		},
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func (h *Handler) Refresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Abort(c, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "refresh_token", Message: "Invalid request body."},
		}))
		return
	}

	result, err := h.auth.Refresh(c.Request.Context(), authusecases.RefreshInput{
		RefreshToken: req.RefreshToken,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func (h *Handler) Logout(c *gin.Context) {
	req, err := decodeOptionalLogoutRequest(c)
	if err != nil {
		httpx.Abort(c, apperrors.Validation("Check your details.", []apperrors.FieldViolation{
			{Field: "refresh_token", Message: "Invalid request body."},
		}))
		return
	}

	result, err := h.auth.Logout(c.Request.Context(), authusecases.LogoutInput{
		SessionID:       SessionID(c),
		DispatchRiderID: DispatchRiderID(c),
		PhoneNumber:     PhoneNumber(c),
		Role:            Role(c),
		RefreshToken:    req.RefreshToken,
		CorrelationID:   httpx.GetRequestID(c),
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": LogoutResponse{
			Message: result.Message,
		},
	})
}

func decodeOptionalLogoutRequest(c *gin.Context) (LogoutRequest, error) {
	if c.Request.Body == nil {
		return LogoutRequest{}, nil
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return LogoutRequest{}, err
	}
	if strings.TrimSpace(string(body)) == "" {
		return LogoutRequest{}, nil
	}

	var req LogoutRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return LogoutRequest{}, err
	}

	return req, nil
}
