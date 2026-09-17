package capabilities

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/taxi-service/internal/capability"
	providerauthusecases "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/usecases"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const (
	certificationCaller = "verification-compliance-service"
	policyCaller        = "admin-backoffice-service"
)

// RegisterRoutes mounts provider, internal and public capability routes.
func RegisterRoutes(group *gin.RouterGroup, service *Service, providerSigner, customerSigner *sharedauth.TokenSigner, verifier *serviceauth.Verifier, internalCallers []string) {
	if group == nil || service == nil {
		return
	}
	if providerSigner != nil {
		provider := group.Group("/provider/vehicles", sharedauth.BearerMiddleware(providerSigner, providerauthusecases.ProviderRole, providerauthusecases.ProviderService))
		provider.GET("/:id/capabilities", func(c *gin.Context) {
			claims, _ := sharedauth.ClaimsFromContext(c)
			v, eligibility, err := service.GetProviderCapability(c, claims.Subject, c.Param("id"))
			respond(c, gin.H{"capability": v, "eligibility": eligibility}, err)
		})
		provider.PUT("/:id/capabilities", func(c *gin.Context) {
			claims, _ := sharedauth.ClaimsFromContext(c)
			var body struct {
				capability.VehicleCapability
				ExpectedVersion int64 `json:"expected_version"`
			}
			if !decode(c, &body) {
				return
			}
			body.VehicleCapability.VehicleID = c.Param("id")
			body.VehicleCapability.VerifiedAt, body.VehicleCapability.ObservedAt = nil, nil
			v, pending, err := service.PutProviderCapability(c, claims.Subject, body.VehicleCapability, body.ExpectedVersion)
			respond(c, gin.H{"capability": v, "products_pending_review": pending}, err)
		})
	}
	if customerSigner != nil && providerSigner != nil {
		group.GET("/service-products", func(c *gin.Context) {
			if !authenticated(c, customerSigner, providerSigner) {
				return
			}
			count, _ := strconv.Atoi(c.Query("passenger_count"))
			products, version, err := service.ListProducts(c, ProductQuery{MarketID: c.Query("market_id"), ZoneID: c.Query("zone_id"), PassengerCount: count,
				Purpose: c.Query("purpose"), Requirements: strings.Split(c.Query("requirements"), ",")})
			respond(c, gin.H{"products": products, "policy_version": version}, err)
		})
	}
	if verifier != nil {
		allowed := map[string]bool{}
		for _, name := range internalCallers {
			allowed[strings.TrimSpace(name)] = true
		}
		internal := group.Group("/internal", serviceauth.Middleware(verifier))
		internal.GET("/vehicles/:id/capabilities", func(c *gin.Context) {
			if !allowed[c.GetString("service_name")] {
				httpx.RespondError(c, apperrors.Forbidden("This caller may not read vehicle capabilities.", nil))
				return
			}
			v, eligibility, err := service.GetInternalCapability(c, c.Param("id"))
			respond(c, gin.H{"capability": v, "eligibility": eligibility}, err)
		})
		internal.POST("/verification/certification-events", func(c *gin.Context) {
			if c.GetString("service_name") != certificationCaller {
				httpx.RespondError(c, apperrors.Forbidden("Only verification-compliance may certify vehicles.", nil))
				return
			}
			var event CertificationEvent
			if !decode(c, &event) {
				return
			}
			outcome, err := service.ApplyCertification(c, event)
			respond(c, gin.H{"event_id": event.EventID, "outcome": outcome}, err)
		})
		internal.POST("/policies/service-products", func(c *gin.Context) {
			if c.GetString("service_name") != policyCaller {
				httpx.RespondError(c, apperrors.Forbidden("Only the control plane may publish Taxi policy.", nil))
				return
			}
			raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10))
			if err != nil {
				httpx.RespondError(c, apperrors.BadRequest("Policy body is invalid.", err))
				return
			}
			policy, created, err := service.ApplyPolicy(c, raw)
			respond(c, gin.H{"policy_version": policy.Version, "created": created, "effective_at": policy.EffectiveAt.Format(time.RFC3339)}, err)
		})
	}
}

// authenticated accepts either a passenger or a provider bearer token.
func authenticated(c *gin.Context, signers ...*sharedauth.TokenSigner) bool {
	token := strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
	for _, signer := range signers {
		if claims, err := signer.Verify(token); err == nil && claims.Type == sharedauth.TokenTypeAccess {
			return true
		}
	}
	httpx.RespondError(c, apperrors.Unauthorized("Authentication is required.", nil))
	return false
}

func decode(c *gin.Context, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		httpx.RespondError(c, apperrors.BadRequest("Request body is invalid.", err))
		return false
	}
	return true
}

func respond(c *gin.Context, data any, err error) {
	switch {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
	case errors.Is(err, ErrNotFound):
		httpx.RespondError(c, apperrors.NotFound("Vehicle capability was not found.", err))
	case errors.Is(err, ErrConflict):
		httpx.RespondError(c, apperrors.Conflict("Vehicle capability changed elsewhere.", err))
	case errors.Is(err, ErrInvalid):
		httpx.RespondError(c, apperrors.BadRequest("Vehicle capability request is invalid.", err))
	case errors.Is(err, ErrDisabled):
		httpx.RespondError(c, apperrors.Unavailable("Service products are not available yet.", err))
	default:
		httpx.RespondError(c, apperrors.Internal("Vehicle capability request failed.", err))
	}
}
