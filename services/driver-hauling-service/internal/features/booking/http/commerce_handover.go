package bookinghttp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	bookingusecases "cosmicforge/logistics/services/hauling-service/internal/features/booking/usecases"
	"cosmicforge/logistics/services/hauling-service/internal/features/fleetauthority"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// RegisterCommerceHandover adds the narrow Vendor relay to the existing
// HMAC-v2 + caller allowlist + nonce boundary used by Commerce missions.
func RegisterCommerceHandover(group *gin.RouterGroup, svc *bookingusecases.BookingService, db *pgxpool.Pool, verifier *serviceauth.Verifier, allowed []string) {
	if group == nil || svc == nil || db == nil || verifier == nil {
		return
	}
	routes := group.Group("/internal/commerce/missions")
	routes.Use(fleetauthority.BoundaryWithDB(db, verifier, allowed)...)
	routes.GET("/:missionRef/handover/:stage", func(c *gin.Context) {
		result, err := svc.ReadCommerceHandover(c.Request.Context(), c.Param("missionRef"), c.Param("stage"))
		if err != nil {
			httpx.Abort(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
	})
	routes.POST("/:missionRef/handover/PICKUP/confirm", func(c *gin.Context) {
		var input struct {
			ChallengeRef string `json:"challenge_ref"`
			Method       string `json:"method"`
			Credential   string `json:"credential"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil {
			httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", nil))
			return
		}
		var trailing any
		if !errors.Is(decoder.Decode(&trailing), io.EOF) {
			httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", nil))
			return
		}
		result, err := svc.ConfirmCommercePickup(c.Request.Context(), c.Param("missionRef"), input.ChallengeRef, strings.ToUpper(strings.TrimSpace(input.Method)), input.Credential, c.GetHeader("Idempotency-Key"))
		if err != nil {
			httpx.Abort(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
	})
}
