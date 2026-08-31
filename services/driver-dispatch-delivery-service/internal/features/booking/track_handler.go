package booking

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/availability"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/livetrack"
)

// trackWriteTimeout bounds a single frame write so one wedged client cannot
// hold the relay goroutine open indefinitely.
const trackWriteTimeout = 5 * time.Second

// TrackHandler streams the assigned rider's live position to the customer
// watching a booking.
//
// The service already had a rider-facing location socket, but it rejects
// anyone who is not the rider themselves or an admin, which is why the
// customer app was left polling. This is the customer's equivalent, authorised
// on booking ownership rather than on identity.
type TrackHandler struct {
	service        *Service
	customerSigner *sharedauth.TokenSigner
	redis          *redis.Client
	upgrader       websocket.Upgrader
}

func NewTrackHandler(service *Service, customerSigner *sharedauth.TokenSigner, redisClient *redis.Client) *TrackHandler {
	return &TrackHandler{
		service:        service,
		customerSigner: customerSigner,
		redis:          redisClient,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			// The customer app is a native client, not a browser, so there is
			// no meaningful Origin to police; authorisation is the access
			// token plus the booking-ownership check below.
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}
}

// RegisterTrackRoute mounts the live-track socket.
//
// It sits outside the bearer middleware group because a websocket handshake
// cannot carry an Authorization header; the handler validates the token from
// the query string itself.
func RegisterTrackRoute(engine *gin.Engine, handler *TrackHandler) {
	engine.GET("/api/v1/customer/dispatch/bookings/:id/track", handler.StreamBookingTrack)
}

// StreamBookingTrack upgrades to a websocket carrying the assigned rider's
// position updates for one booking.
func (h *TrackHandler) StreamBookingTrack(c *gin.Context) {
	claims, ok := h.authenticate(c)
	if !ok {
		return
	}

	bookingID := c.Param("id")
	providerID, err := h.service.ResolveTrackTarget(c.Request.Context(), claims.Subject, bookingID)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	if h.redis == nil {
		httpx.Abort(c, apperrors.Unavailable("Live tracking is unavailable.", nil))
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	// Subscribe before reading the current position, so a fix landing between
	// the read and the subscribe is delivered rather than dropped. The channel
	// is the rider location channel the availability store already publishes
	// to, so customers and riders read the same stream.
	pubsub := h.redis.Subscribe(ctx, availability.ProviderLocationChannel(providerID))
	defer func() { _ = pubsub.Close() }()

	h.writeCurrentPosition(ctx, conn, claims.Subject, bookingID, providerID)

	// A reader is required for the connection to observe client-side closes;
	// inbound frames are ignored, this stream is server to client.
	go drainClient(conn, cancel)

	messages := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, open := <-messages:
			if !open {
				return
			}
			_ = conn.SetWriteDeadline(time.Now().Add(trackWriteTimeout))
			if err := conn.WriteMessage(websocket.TextMessage, []byte(msg.Payload)); err != nil {
				return
			}
		}
	}
}

// authenticate validates the customer access token carried in the query
// string, mirroring what the bearer middleware would have done.
func (h *TrackHandler) authenticate(c *gin.Context) (sharedauth.Claims, bool) {
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		token = strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
	}
	if token == "" || h.customerSigner == nil {
		httpx.Abort(c, apperrors.Unauthorized("Authentication is required.", nil))
		return sharedauth.Claims{}, false
	}

	claims, err := h.customerSigner.Verify(token)
	if err != nil {
		httpx.Abort(c, apperrors.Unauthorized("Your session has expired. Please sign in again.", err))
		return sharedauth.Claims{}, false
	}
	if claims.Type != sharedauth.TokenTypeAccess || claims.Role != "customer" || claims.Service != "customer" {
		httpx.Abort(c, apperrors.Forbidden("You do not have access to this action.", nil))
		return sharedauth.Claims{}, false
	}
	return claims, true
}

// writeCurrentPosition sends the last known fix immediately on connect, so the
// map has a rider to draw without waiting for the next report.
func (h *TrackHandler) writeCurrentPosition(ctx context.Context, conn *websocket.Conn, customerID, bookingID, providerID string) {
	_ = conn.SetWriteDeadline(time.Now().Add(trackWriteTimeout))

	location, err := h.service.GetLiveLocation(ctx, customerID, bookingID)
	if err != nil {
		payload, marshalErr := json.Marshal(livetrack.OfflineUpdate(providerID))
		if marshalErr == nil {
			_ = conn.WriteMessage(websocket.TextMessage, payload)
		}
		return
	}

	payload, marshalErr := json.Marshal(livetrack.LocationUpdate(livetrack.Fix{
		EntityID:   providerID,
		Lat:        location.Lat,
		Lng:        location.Lng,
		Heading:    location.Heading,
		Speed:      location.Speed,
		Accuracy:   location.Accuracy,
		RecordedAt: location.UpdatedAt,
	}))
	if marshalErr == nil {
		_ = conn.WriteMessage(websocket.TextMessage, payload)
	}
}

// drainClient reads and discards inbound frames until the peer disconnects,
// then cancels the relay.
func drainClient(conn *websocket.Conn, cancel context.CancelFunc) {
	defer cancel()
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
