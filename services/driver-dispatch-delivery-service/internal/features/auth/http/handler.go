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

// Start handles POST /api/v1/auth/start.
// Validates phone number, enforces OTP rate limit, generates and stores an OTP,
// publishes the OTP-requested event, and returns expires_in_seconds.
// OTP is never included in the response.
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

// Verify handles POST /api/v1/auth/verify.
// Validates OTP, upserts dispatch rider identity, creates session, issues tokens.
// Tokens are never logged. OTP is never returned in the response.
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
		OTPCode:       req.OTPCode,
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
