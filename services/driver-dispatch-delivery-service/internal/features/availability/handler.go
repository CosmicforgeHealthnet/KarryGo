package availability

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/apiresponse"
	authhttp "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/http"
	authmodels "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/models"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/middleware"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

type ServicePort interface {
	SetStatus(ctx context.Context, providerID string, request SetAvailabilityRequest) (AvailabilityResponse, error)
	GetStatus(ctx context.Context, providerID string) (AvailabilityStatusResponse, error)
	GetCurrentSession(ctx context.Context, providerID string) (CurrentSessionResponse, error)
	UpdateLocation(ctx context.Context, providerID string, request UpdateLocationRequest) (LocationUpdateResponse, error)
	GetLocation(ctx context.Context, providerID string) (LocationResponse, error)
	GetNearbyProviders(ctx context.Context, request NearbyProvidersRequest) (NearbyResponse, error)
}

type Handler struct {
	service  ServicePort
	tokens   *authusecases.TokenUsecase
	redis    *redis.Client
	upgrader websocket.Upgrader
}

func NewHandler(pool *pgxpool.Pool, redisClient *redis.Client, tokens *authusecases.TokenUsecase) *Handler {
	repository := NewPostgresRepository(pool)
	service := NewService(repository, NewRedisLiveStore(redisClient), WithEventPublisher(NewRedisEventPublisher(redisClient)))
	return NewHandlerWithService(service, tokens, redisClient)
}

func NewHandlerWithService(service ServicePort, tokens *authusecases.TokenUsecase, redisClient *redis.Client) *Handler {
	return &Handler{
		service: service,
		tokens:  tokens,
		redis:   redisClient,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}
}

func RegisterRoutes(engine *gin.Engine, tokens *authusecases.TokenUsecase, internalServiceKey string, handler *Handler) {
	availability := engine.Group("/api/v1/provider/availability")
	availability.Use(authhttp.DispatchRiderAuthRequired(tokens), requireDispatchProviderRole())
	availability.PATCH("", handler.PatchAvailability)
	availability.GET("", handler.GetAvailability)
	availability.GET("/session/current", handler.GetCurrentSession)

	location := engine.Group("/api/v1/provider/location")
	location.Use(authhttp.DispatchRiderAuthRequired(tokens), requireDispatchProviderRole())
	location.POST("", handler.PostLocation)
	location.GET("", handler.GetLocation)

	internal := engine.Group("/api/v1/internal")
	internal.Use(middleware.RequireServiceKey(internalServiceKey))
	internal.GET("/nearby", handler.GetNearby)

	engine.GET("/ws/provider/:id/location", handler.StreamLocation)
}

func (h *Handler) PatchAvailability(c *gin.Context) {
	var request SetAvailabilityRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, validationErrors([]apperrors.FieldViolation{
			{Field: "body", Message: "Request body must be valid JSON."},
		}))
		return
	}
	// Phase 5K §5: provider cannot set busy via the API — only trip.started can.
	if !IsProviderSettableStatus(request.Status) {
		httpx.Abort(c, validationErrors([]apperrors.FieldViolation{
			{Field: "status", Message: "Status must be online or offline."},
		}))
		return
	}
	response, err := h.service.SetStatus(c.Request.Context(), authhttp.DispatchRiderID(c), request)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, "Availability updated.", response)
}

func (h *Handler) GetAvailability(c *gin.Context) {
	response, err := h.service.GetStatus(c.Request.Context(), authhttp.DispatchRiderID(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, "Availability loaded.", response)
}

func (h *Handler) GetCurrentSession(c *gin.Context) {
	response, err := h.service.GetCurrentSession(c.Request.Context(), authhttp.DispatchRiderID(c))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, "Current availability session loaded.", response)
}

func (h *Handler) PostLocation(c *gin.Context) {
	providerID := authhttp.DispatchRiderID(c)

	// Phase 5K: rate limit — 30 GPS pings per 60 s per provider.
	// All authenticated attempts are counted, including those that fail validation.
	if h.redis != nil {
		if limited, err := h.checkLocationRateLimit(c.Request.Context(), providerID); err != nil {
			log.Printf("availability location rate limit error provider_id=%s error=%v", providerID, err)
			// On Redis error, log and continue — do not block the GPS endpoint.
		} else if limited {
			httpx.Abort(c, apperrors.RateLimited("Too many location updates. Please slow down.", nil))
			return
		}
	}

	var request UpdateLocationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, validationErrors([]apperrors.FieldViolation{
			{Field: "body", Message: "Request body must be valid JSON with lat and lng fields."},
		}))
		return
	}
	if _, err := h.service.UpdateLocation(c.Request.Context(), providerID, request); err != nil {
		httpx.Abort(c, err)
		return
	}
	// Minimal response — this endpoint is called every 5–10 s from the provider app.
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"updated": true}})
}

// checkLocationRateLimit increments the per-provider GPS ping counter and
// returns true if the provider has exceeded LocationRateLimitMaxPerMin within
// the current LocationRateLimitWindow.
func (h *Handler) checkLocationRateLimit(ctx context.Context, providerID string) (bool, error) {
	key := ProviderLocationRateLimitKey(providerID)
	count, err := h.redis.Incr(ctx, key).Result()
	if err != nil {
		return false, err
	}
	if count == 1 {
		// First request in window — set TTL so counter expires automatically.
		if expErr := h.redis.Expire(ctx, key, LocationRateLimitWindow).Err(); expErr != nil {
			log.Printf("availability rate limit set expire error key=%s error=%v", key, expErr)
		}
	}
	return count > int64(LocationRateLimitMaxPerMin), nil
}

func (h *Handler) GetLocation(c *gin.Context) {
	response, err := h.service.GetLocation(c.Request.Context(), authhttp.DispatchRiderID(c))
	if err != nil {
		httpx.Abort(c, err) // 404 not_found when location key has expired or never existed
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, "Location loaded.", response.Location)
}

func (h *Handler) GetNearby(c *gin.Context) {
	request, err := nearbyRequestFromQuery(c)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	response, err := h.service.GetNearbyProviders(c.Request.Context(), request)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	apiresponse.RespondSuccess(c, http.StatusOK, "Nearby providers loaded.", response)
}

func (h *Handler) StreamLocation(c *gin.Context) {
	providerID := c.Param("id")
	if err := validateProviderID(providerID); err != nil {
		httpx.Abort(c, err)
		return
	}
	rawToken := strings.TrimSpace(c.Query("token"))
	if rawToken == "" {
		httpx.Abort(c, apperrors.Unauthorized("Access token is invalid.", nil))
		return
	}
	if h.tokens == nil {
		httpx.Abort(c, apperrors.Unavailable("Token validation is unavailable.", nil))
		return
	}
	claims, err := h.tokens.ValidateAccessToken(rawToken)
	if err != nil {
		if errors.Is(err, authusecases.ErrExpiredToken) {
			httpx.Abort(c, apperrors.Unauthorized("Access token has expired.", nil))
			return
		}
		httpx.Abort(c, apperrors.Unauthorized("Access token is invalid.", nil))
		return
	}
	if claims.DispatchRiderID != providerID && !canStreamOtherProvider(claims.Role) {
		httpx.Abort(c, apperrors.Forbidden("This location stream is not available for the token subject.", nil))
		return
	}
	if h.redis == nil {
		httpx.Abort(c, apperrors.Unavailable("Location stream is unavailable.", nil))
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	// Subscribe before reading initial location to avoid a race window where a
	// ping arrives after GetLocation but before Subscribe.
	pubsub := h.redis.Subscribe(ctx, ProviderLocationChannel(providerID))
	defer pubsub.Close()

	// Send current location immediately so the client does not have to wait for
	// the next GPS ping.
	locResp, locErr := h.service.GetLocation(ctx, providerID)
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if locErr != nil || locResp.Location == nil {
		_ = conn.WriteJSON(WSLocationUnavailable{Type: "location_unavailable", ProviderID: providerID})
	} else {
		loc := locResp.Location
		_ = conn.WriteJSON(WSLocationUpdate{
			Type:       "location_update",
			ProviderID: providerID,
			Lat:        loc.Lat,
			Lng:        loc.Lng,
			Heading:    loc.Heading,
			Speed:      loc.Speed,
			Accuracy:   loc.Accuracy,
			UpdatedAt:  loc.UpdatedAt,
		})
	}

	channel := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-channel:
			if !ok {
				return
			}
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteMessage(websocket.TextMessage, []byte(msg.Payload)); err != nil {
				return
			}
		}
	}
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

func canStreamOtherProvider(role string) bool {
	return role == "platform_admin" || role == "internal_service"
}

func nearbyRequestFromQuery(c *gin.Context) (NearbyProvidersRequest, error) {
	// Accept lat/lng as primary names; latitude/longitude as fallbacks for
	// backward compatibility.
	latitude, err := requiredFloatQuery(c, "lat", "latitude")
	if err != nil {
		return NearbyProvidersRequest{}, err
	}
	longitude, err := requiredFloatQuery(c, "lng", "longitude")
	if err != nil {
		return NearbyProvidersRequest{}, err
	}
	// Accept radius as primary; radius_km as fallback.
	radius, err := optionalFloatQuery(c, "radius", "radius_km")
	if err != nil {
		return NearbyProvidersRequest{}, err
	}
	limit, err := optionalIntQuery(c, "limit")
	if err != nil {
		return NearbyProvidersRequest{}, err
	}

	// Early handler-level range checks so validation errors are returned even
	// when fakeService (or any service that skips validateNearbyInput) is used.
	req := NearbyProvidersRequest{
		Latitude:  latitude,
		Longitude: longitude,
		RadiusKM:  radius,
		Limit:     limit,
	}
	if err := validateNearbyInput(req); err != nil {
		return NearbyProvidersRequest{}, err
	}
	return req, nil
}

func requiredFloatQuery(c *gin.Context, primary string, fallback string) (float64, error) {
	value := strings.TrimSpace(c.Query(primary))
	if value == "" && fallback != "" {
		value = strings.TrimSpace(c.Query(fallback))
	}
	if value == "" {
		return 0, validationErrors([]apperrors.FieldViolation{
			{Field: primary, Message: "This query parameter is required."},
		})
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, validationErrors([]apperrors.FieldViolation{
			{Field: primary, Message: "This query parameter must be a number."},
		})
	}
	return parsed, nil
}

func optionalFloatQuery(c *gin.Context, key string, fallbacks ...string) (float64, error) {
	value := strings.TrimSpace(c.Query(key))
	for _, fb := range fallbacks {
		if value == "" {
			value = strings.TrimSpace(c.Query(fb))
		}
	}
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, validationErrors([]apperrors.FieldViolation{
			{Field: key, Message: "This query parameter must be a number."},
		})
	}
	return parsed, nil
}

func optionalIntQuery(c *gin.Context, key string) (int, error) {
	value := strings.TrimSpace(c.Query(key))
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, validationErrors([]apperrors.FieldViolation{
			{Field: key, Message: "This query parameter must be an integer."},
		})
	}
	return parsed, nil
}
