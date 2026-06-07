package profile

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	authhttp "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/http"
	authmodels "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/models"
	authusecases "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/usecases"
	"karrygo/shared/go/apperrors"
	"karrygo/shared/go/httpx"
)

type Handler struct {
	service ProviderService
}

func NewHandler(service ProviderService) *Handler {
	return &Handler{service: service}
}

func RegisterRoutes(engine *gin.Engine, tokens *authusecases.TokenUsecase, service ProviderService) {
	handler := NewHandler(service)

	engine.GET("/api/v1/provider/:id/public", newPublicProfileRateLimiter(60, time.Minute), handler.GetPublicProfile)

	protected := engine.Group("/api/v1/provider")
	protected.Use(authhttp.DispatchRiderAuthRequired(tokens), requireDispatchProviderRole())
	protected.POST("/onboarding", handler.Onboarding)
	protected.GET("/me", handler.GetMe)
	protected.PATCH("/me", handler.UpdateMe)
	protected.POST("/emergency-contact", handler.SetEmergencyContact)
	protected.GET("/emergency-contact", handler.GetEmergencyContact)
	protected.POST("/guarantor", handler.SetGuarantor)
	protected.GET("/guarantor", handler.GetGuarantor)
	protected.GET("/stats", handler.GetStats)
}

func (h *Handler) GetPublicProfile(c *gin.Context) {
	result, err := h.service.GetPublicProfile(c.Request.Context(), c.Param("id"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func (h *Handler) Onboarding(c *gin.Context) {
	var req OnboardingInput
	if err := decodeProfileJSON(c, &req, onboardingReadOnlyFields()); err != nil {
		httpx.Abort(c, err)
		return
	}
	result, err := h.service.Onboarding(c.Request.Context(), authFromContext(c), req)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func (h *Handler) GetMe(c *gin.Context) {
	result, err := h.service.GetMe(c.Request.Context(), authFromContext(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func (h *Handler) UpdateMe(c *gin.Context) {
	var req UpdateProviderInput
	if err := decodeProfileJSON(c, &req, updateReadOnlyFields()); err != nil {
		httpx.Abort(c, err)
		return
	}
	result, err := h.service.UpdateMe(c.Request.Context(), authFromContext(c), req)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func (h *Handler) SetEmergencyContact(c *gin.Context) {
	var req EmergencyContactInput
	if err := decodeProfileJSON(c, &req, nil); err != nil {
		httpx.Abort(c, err)
		return
	}
	result, err := h.service.SetEmergencyContact(c.Request.Context(), authFromContext(c), req)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func (h *Handler) GetEmergencyContact(c *gin.Context) {
	result, err := h.service.GetEmergencyContact(c.Request.Context(), authFromContext(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func (h *Handler) SetGuarantor(c *gin.Context) {
	var req GuarantorInput
	if err := decodeProfileJSON(c, &req, nil); err != nil {
		httpx.Abort(c, err)
		return
	}
	result, err := h.service.SetGuarantor(c.Request.Context(), authFromContext(c), req)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func (h *Handler) GetGuarantor(c *gin.Context) {
	result, err := h.service.GetGuarantor(c.Request.Context(), authFromContext(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func (h *Handler) GetStats(c *gin.Context) {
	result, err := h.service.GetStats(c.Request.Context(), authFromContext(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, result)
}

func requireDispatchProviderRole() gin.HandlerFunc {
	return func(c *gin.Context) {
		if authhttp.Role(c) != authmodels.RoleDispatchProvider {
			httpx.Abort(c, apperrors.Forbidden("This route is only available to dispatch providers.", nil))
			return
		}
		c.Next()
	}
}

func authFromContext(c *gin.Context) AuthContext {
	return AuthContext{
		ProviderID:    authhttp.DispatchRiderID(c),
		PhoneNumber:   authhttp.PhoneNumber(c),
		CorrelationID: httpx.GetRequestID(c),
	}
}

func respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}

func decodeProfileJSON(c *gin.Context, out any, readOnly map[string]struct{}) error {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return validationBadRequest("Check your details.", []apperrors.FieldViolation{
			{Field: "body", Message: "Invalid request body."},
		})
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return validationBadRequest("Check your details.", []apperrors.FieldViolation{
			{Field: "body", Message: "Invalid request body."},
		})
	}

	for field := range raw {
		if _, ok := readOnly[field]; ok {
			return validationBadRequest("Check your details.", []apperrors.FieldViolation{
				{Field: field, Message: "This field cannot be updated."},
			})
		}
	}

	if err := json.Unmarshal(body, out); err != nil {
		return validationBadRequest("Check your details.", []apperrors.FieldViolation{
			{Field: "body", Message: "Invalid request body."},
		})
	}
	return nil
}

func onboardingReadOnlyFields() map[string]struct{} {
	return map[string]struct{}{
		"phone":               {},
		"verification_status": {},
		"avg_rating":          {},
		"total_trips":         {},
		"is_active":           {},
		"onboarding_complete": {},
	}
}

func updateReadOnlyFields() map[string]struct{} {
	fields := onboardingReadOnlyFields()
	fields["id"] = struct{}{}
	fields["provider_id"] = struct{}{}
	fields["operation_type"] = struct{}{}
	return fields
}

type ipRateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	clients map[string]rateLimitEntry
}

type rateLimitEntry struct {
	count   int
	resetAt time.Time
}

func newPublicProfileRateLimiter(limit int, window time.Duration) gin.HandlerFunc {
	limiter := &ipRateLimiter{
		limit:   limit,
		window:  window,
		clients: make(map[string]rateLimitEntry),
	}
	return limiter.handle
}

func (l *ipRateLimiter) handle(c *gin.Context) {
	now := time.Now()
	ip := c.ClientIP()
	if ip == "" {
		ip = "unknown"
	}

	l.mu.Lock()
	entry := l.clients[ip]
	if entry.resetAt.IsZero() || now.After(entry.resetAt) {
		entry = rateLimitEntry{resetAt: now.Add(l.window)}
	}
	entry.count++
	l.clients[ip] = entry
	limited := entry.count > l.limit
	l.mu.Unlock()

	if limited {
		httpx.Abort(c, apperrors.RateLimited("Too many public profile requests. Try again later.", nil))
		return
	}
	c.Next()
}
