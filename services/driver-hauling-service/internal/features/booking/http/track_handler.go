package bookinghttp

import (
	"context"
	"encoding/json"
	nethttp "net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"

	bookingusecases "cosmicforge/logistics/services/hauling-service/internal/features/booking/usecases"
	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/livetrack"
)

// trackWriteTimeout bounds a single frame write so one wedged client cannot
// hold the relay goroutine open indefinitely.
const trackWriteTimeout = 5 * time.Second

// TrackHandler streams the assigned provider's live position to the customer
// watching a booking.
//
// This is the push half of live tracking. The REST snapshot at
// GET /customer/bookings/:id/location stays as the fallback: an app that
// cannot reach this socket keeps polling and still moves its map, just in
// coarser steps.
type TrackHandler struct {
	svc            *bookingusecases.BookingService
	customerSigner *sharedauth.TokenSigner
	redis          *redis.Client
	upgrader       websocket.Upgrader
}

func NewTrackHandler(
	svc *bookingusecases.BookingService,
	customerSigner *sharedauth.TokenSigner,
	redisClient *redis.Client,
) *TrackHandler {
	return &TrackHandler{
		svc:            svc,
		customerSigner: customerSigner,
		redis:          redisClient,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			// The customer apps are native clients, not browsers, so there
			// is no meaningful Origin to police here; authorisation is the
			// access token plus the booking-ownership check below.
			CheckOrigin: func(*nethttp.Request) bool { return true },
		},
	}
}

// StreamBookingTrack upgrades to a websocket carrying the assigned provider's
// position updates for one booking.
//
// The access token arrives as a query parameter rather than a header because
// websocket clients cannot set headers on the handshake, which is why this
// route sits outside the bearer middleware group and validates the token
// itself.
func (h *TrackHandler) StreamBookingTrack(c *gin.Context) {
	claims, ok := h.authenticate(c)
	if !ok {
		return
	}

	bookingID := c.Param("id")
	target, err := h.svc.ResolveTrackTarget(c.Request.Context(), bookingID, claims.Subject)
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
	// the read and the subscribe is delivered rather than dropped.
	pubsub := livetrack.Subscribe(ctx, h.redis, availabilityrepositories.TrackingNamespace, target.ProviderID)
	defer func() { _ = pubsub.Close() }()

	h.writeCurrentPosition(ctx, conn, bookingID, claims.Subject, target.ProviderID)

	// A reader is required for the connection to observe client-side closes;
	// inbound frames themselves are ignored, this stream is server to client.
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
// map has a vehicle to draw without waiting for the next report.
func (h *TrackHandler) writeCurrentPosition(ctx context.Context, conn *websocket.Conn, bookingID, customerID, providerID string) {
	_ = conn.SetWriteDeadline(time.Now().Add(trackWriteTimeout))

	location, err := h.svc.GetBookingLocation(ctx, bookingID, customerID)
	if err != nil || !location.Available {
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
		RecordedAt: time.Unix(location.UpdatedAt, 0).UTC(),
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
