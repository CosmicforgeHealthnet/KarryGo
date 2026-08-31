package delivery

// Section 17C vendor-own delivery HTTP surface.
//
// Two routes only: read and replace one branch's policy. There is deliberately
// no list-all-branches route — a branch-scoped actor would then need the
// response filtered, and a filtered list is a slower way of leaking which
// branches exist.

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/features/production"
	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/shared/go/httpx"
)

const maxPolicyBody = 8 << 10

// Handler serves the vendor-own delivery policy API.
type Handler struct {
	service *Service
}

// NewHandler builds the delivery handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func strictJSON(c *gin.Context, destination any) error {
	decoder := json.NewDecoder(
		io.LimitReader(c.Request.Body, maxPolicyBody+1),
	)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		return ErrInvalid
	}

	var extra any

	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return ErrInvalid
	}

	return nil
}

// expectedVersion reads If-Match. A mutation without it is refused rather than
// applied blind. "0" is the explicit "I expect no policy yet" create case, so
// a create cannot silently overwrite a policy that appeared in between.
func expectedVersion(c *gin.Context) (int64, error) {
	raw := strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`)

	if raw == "" {
		return 0, ErrVersionConflict
	}

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, ErrVersionConflict
	}

	return value, nil
}

func (h *Handler) actor(c *gin.Context) (production.Actor, bool) {
	staff, ok := authctx.FromContext(c)
	if !ok {
		return production.Actor{}, false
	}

	return production.Actor{
		StaffID:       staff.StaffID,
		VendorID:      staff.VendorID,
		SessionID:     staff.SessionID,
		CorrelationID: httpx.GetRequestID(c),
		CausationID:   c.GetHeader("X-Causation-ID"),
		CommandRef:    c.GetHeader("Idempotency-Key"),
	}, true
}

func respondError(c *gin.Context, err error) {
	status, code, message := http.StatusInternalServerError,
		"DELIVERY_POLICY_UNAVAILABLE",
		"Delivery policy is temporarily unavailable."

	switch {
	case errors.Is(err, ErrNotFound):
		status, code, message = http.StatusNotFound,
			"DELIVERY_POLICY_NOT_FOUND",
			"Delivery policy was not found."

	case errors.Is(err, ErrForbidden):
		status, code, message = http.StatusForbidden,
			"DELIVERY_POLICY_FORBIDDEN",
			"You do not have access to this delivery policy."

	case errors.Is(err, ErrInvalid):
		status, code, message = http.StatusUnprocessableEntity,
			"DELIVERY_POLICY_VALIDATION_FAILED",
			"Check the submitted delivery policy values."

	case errors.Is(err, ErrVersionConflict):
		status, code, message = http.StatusConflict,
			"DELIVERY_POLICY_VERSION_CONFLICT",
			"This delivery policy changed. Reload and try again."

	case errors.Is(err, ErrDisabled):
		status, code, message = http.StatusServiceUnavailable,
			"VENDOR_OWN_DELIVERY_DISABLED",
			"Vendor own delivery is not enabled."
	}

	c.JSON(status, gin.H{
		"success": false,
		"error": gin.H{
			"code":       code,
			"message":    message,
			"request_id": httpx.GetRequestID(c),
		},
	})
}

func unauthorized(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, gin.H{
		"success": false,
		"error": gin.H{
			"code":       "UNAUTHORIZED",
			"message":    "Authentication is required.",
			"request_id": httpx.GetRequestID(c),
		},
	})
}

func (h *Handler) getPolicy(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		unauthorized(c)
		return
	}

	policy, err := h.service.GetPolicy(
		c.Request.Context(), actor, c.Param("branchId"),
	)
	if err != nil {
		respondError(c, err)
		return
	}

	c.Header("ETag", strconv.FormatInt(policy.Version, 10))
	c.JSON(http.StatusOK, gin.H{"success": true, "data": policy})
}

func (h *Handler) upsertPolicy(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		unauthorized(c)
		return
	}

	version, err := expectedVersion(c)
	if err != nil {
		respondError(c, err)
		return
	}

	var in PolicyInput

	if err = strictJSON(c, &in); err != nil {
		respondError(c, err)
		return
	}

	policy, err := h.service.UpsertPolicy(
		c.Request.Context(), actor, c.Param("branchId"), version, in,
	)
	if err != nil {
		respondError(c, err)
		return
	}

	c.Header("ETag", strconv.FormatInt(policy.Version, 10))
	c.JSON(http.StatusOK, gin.H{"success": true, "data": policy})
}

// RegisterRoutes mounts the vendor-own delivery policy API.
//
// enabled is the platform capability switch. With it false no route is
// registered at all, so a disabled capability has no surface rather than
// routes that answer 503 and confirm it exists.
func RegisterRoutes(root *gin.RouterGroup, h *Handler, enabled bool) {
	if root == nil || h == nil || h.service == nil || !enabled {
		return
	}

	group := root.Group("/delivery/branches/:branchId/policy")
	group.GET("", h.getPolicy)
	group.PUT("", h.upsertPolicy)
}
