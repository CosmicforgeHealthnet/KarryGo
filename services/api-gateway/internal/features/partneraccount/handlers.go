package partneraccount

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/delivery"
)

// Handlers holds the dashboard/keys/internal HTTP handlers.
type Handlers struct {
	svc *Service
}

// NewHandlers builds the account handlers.
func NewHandlers(svc *Service) *Handlers { return &Handlers{svc: svc} }

const rfc3339 = time.RFC3339

// --- dashboard auth ----------------------------------------------------------

// Register handles POST /auth/register (public).
func (h *Handlers) Register(c *gin.Context) {
	var in RegisterInput
	if !bindJSON(c, &in) {
		return
	}
	res, err := h.svc.Register(c.Request.Context(), in, metaFrom(c))
	if err != nil {
		respondAPIError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data": gin.H{
			"account":    res.Account,
			"token":      res.Token,
			"expires_at": res.ExpiresAt.UTC().Format(rfc3339),
			"api_key": gin.H{
				"key":     res.APIKey.Key,
				"prefix":  res.APIKey.Prefix,
				"warning": res.APIKey.Warning,
			},
		},
	})
}

// Login handles POST /auth/login (public).
func (h *Handlers) Login(c *gin.Context) {
	var in LoginInput
	if !bindJSON(c, &in) {
		return
	}
	res, err := h.svc.Login(c.Request.Context(), in, metaFrom(c))
	if err != nil {
		respondAPIError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"token":      res.Token,
			"expires_at": res.ExpiresAt.UTC().Format(rfc3339),
			"account":    res.Account,
		},
	})
}

// Logout handles POST /auth/logout (session required).
func (h *Handlers) Logout(c *gin.Context) {
	if err := h.svc.Logout(c.Request.Context(), SessionTokenHash(c)); err != nil {
		respondAPIError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Logged out."})
}

// Me handles GET /auth/me (session required).
func (h *Handlers) Me(c *gin.Context) {
	account, err := h.svc.Me(c.Request.Context(), SessionPartnerID(c))
	if err != nil {
		respondAPIError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"account": account}})
}

// UpdateProfile handles PATCH /profile (session required). This is the
// Settings > Profile edit form: business name, phone, state, city, logo URL.
// Only fields present in the request body are changed - omitted fields are
// left untouched (see UpdateProfileInput / UpdateProfileParams).
func (h *Handlers) UpdateProfile(c *gin.Context) {
	var in UpdateProfileInput
	if !bindJSON(c, &in) {
		return
	}
	account, err := h.svc.UpdateProfile(c.Request.Context(), SessionPartnerID(c), in)
	if err != nil {
		respondAPIError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"account": account}})
}

// --- API key management ------------------------------------------------------

// GenerateKey handles POST /keys (session required).
func (h *Handlers) GenerateKey(c *gin.Context) {
	var in CreateKeyInput
	// Body is optional (label/environment); ignore bind errors on empty body.
	_ = c.ShouldBindJSON(&in)
	issued, err := h.svc.CreateKey(c.Request.Context(), SessionPartnerID(c), PartnerPlan(c), in)
	if err != nil {
		respondAPIError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data": gin.H{
			"id":          issued.ID,
			"key":         issued.Key,
			"prefix":      issued.Prefix,
			"environment": issued.Environment,
			"warning":     issued.Warning,
		},
	})
}

// ListKeys handles GET /keys (session required).
func (h *Handlers) ListKeys(c *gin.Context) {
	views, err := h.svc.ListKeys(c.Request.Context(), SessionPartnerID(c))
	if err != nil {
		respondAPIError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": views})
}

// RevokeKey handles DELETE /keys/:id (session required).
func (h *Handlers) RevokeKey(c *gin.Context) {
	if err := h.svc.RevokeKey(c.Request.Context(), SessionPartnerID(c), c.Param("id")); err != nil {
		respondAPIError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Key revoked."})
}

// --- internal ----------------------------------------------------------------

// ActivateLive handles POST /internal/partner/accounts/:id/activate-live.
func (h *Handlers) ActivateLive(c *gin.Context) {
	if err := h.svc.ActivateLive(c.Request.Context(), c.Param("id")); err != nil {
		respondAPIError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// UpdateStatus handles PATCH /internal/partner/accounts/:id/status.
func (h *Handlers) UpdateStatus(c *gin.Context) {
	var body struct {
		Status string `json:"status"`
	}
	if !bindJSON(c, &body) {
		return
	}
	if err := h.svc.UpdateStatus(c.Request.Context(), c.Param("id"), body.Status); err != nil {
		respondAPIError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// --- helpers -----------------------------------------------------------------

func bindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		respondAPIError(c, delivery.NewAPIError(http.StatusBadRequest, "validation_error",
			"Request body is not valid JSON.", nil))
		return false
	}
	return true
}

func metaFrom(c *gin.Context) RequestMeta {
	return RequestMeta{IPAddress: c.ClientIP(), UserAgent: c.Request.UserAgent()}
}

func respondAPIError(c *gin.Context, err error) {
	var apiErr *delivery.APIError
	if !errors.As(err, &apiErr) {
		apiErr = delivery.ErrInternal()
	}
	fields := apiErr.Fields
	if fields == nil {
		fields = []delivery.FieldError{}
	}
	c.AbortWithStatusJSON(apiErr.Status, gin.H{
		"success": false,
		"error": gin.H{
			"code":    apiErr.Code,
			"message": apiErr.Message,
			"fields":  fields,
		},
	})
}
