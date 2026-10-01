package profilehttp

import (
	"bytes"
	"encoding/json"
	"io"
	nethttp "net/http"
	"strings"

	"github.com/gin-gonic/gin"

	authusecases "cosmicforge/logistics/services/customer-service/internal/features/auth/usecases"
	profileusecases "cosmicforge/logistics/services/customer-service/internal/features/profile/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

// RegisterTrustedContactRoutes mounts /trusted-contacts. It serves the same
// records as /profile/emergency-contacts, which stays mounted unchanged.
func RegisterTrustedContactRoutes(group *gin.RouterGroup, service *profileusecases.TrustedContactService, signer *sharedauth.TokenSigner) {
	if group == nil || service == nil || signer == nil {
		return
	}
	g := group.Group("/trusted-contacts", sharedauth.BearerMiddleware(signer, authusecases.CustomerRole, authusecases.CustomerService))
	g.GET("", func(c *gin.Context) {
		id, ok := subject(c)
		if !ok {
			return
		}
		out, err := service.List(c.Request.Context(), id)
		reply(c, nethttp.StatusOK, gin.H{"contacts": out}, err)
	})
	g.POST("", func(c *gin.Context) {
		id, ok := subject(c)
		if !ok {
			return
		}
		var req trustedContactRequest
		if !decodeStrict(c, &req) {
			return
		}
		if req.ExpectedVersion != nil {
			httpx.Abort(c, apperrors.BadRequest("expected_version is not allowed when creating a contact.", nil))
			return
		}
		out, replayed, err := service.Create(c.Request.Context(), id, req.input(), c.GetHeader("Idempotency-Key"))
		status := nethttp.StatusCreated
		if replayed {
			status = nethttp.StatusOK
		}
		reply(c, status, out, err)
	})
	g.PUT("/:id", func(c *gin.Context) {
		id, ok := subject(c)
		if !ok {
			return
		}
		var req trustedContactRequest
		if !decodeStrict(c, &req) {
			return
		}
		out, err := service.Update(c.Request.Context(), id, c.Param("id"), req.input(), req.ExpectedVersion)
		reply(c, nethttp.StatusOK, out, err)
	})
	g.DELETE("/:id", func(c *gin.Context) {
		id, ok := subject(c)
		if !ok {
			return
		}
		reply(c, nethttp.StatusOK, gin.H{}, service.Delete(c.Request.Context(), id, c.Param("id")))
	})
}

type trustedContactRequest struct {
	Name                   string  `json:"name"`
	Phone                  string  `json:"phone"`
	RelationshipLabel      *string `json:"relationship_label"`
	EmergencyAlertsEnabled *bool   `json:"emergency_alerts_enabled"`
	TripSharingEnabled     *bool   `json:"trip_sharing_enabled"`
	ExpectedVersion        *int64  `json:"expected_version,omitempty"`
}

func (r trustedContactRequest) input() profileusecases.TrustedContactInput {
	return profileusecases.TrustedContactInput{Name: r.Name, Phone: r.Phone, RelationshipLabel: r.RelationshipLabel,
		EmergencyAlertsEnabled: r.EmergencyAlertsEnabled, TripSharingEnabled: r.TripSharingEnabled}
}

func subject(c *gin.Context) (string, bool) {
	claims, ok := sharedauth.ClaimsFromContext(c)
	if !ok || strings.TrimSpace(claims.Subject) == "" {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return "", false
	}
	return strings.TrimSpace(claims.Subject), true
}

func decodeStrict(c *gin.Context, target any) bool {
	raw, err := io.ReadAll(nethttp.MaxBytesReader(c.Writer, c.Request.Body, 16<<10))
	if err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is too large or unreadable.", err))
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return false
	}
	return true
}

func reply(c *gin.Context, status int, data any, err error) {
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(status, gin.H{"success": true, "data": data})
}
