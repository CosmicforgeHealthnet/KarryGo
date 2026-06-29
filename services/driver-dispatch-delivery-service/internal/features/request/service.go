package request

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/shared/go/apperrors"
)

type NearbyProviderFinder interface {
	FindNearby(ctx context.Context, lat, lng, radius float64, limit int) ([]NearbyProvider, error)
}

type NotificationSender interface {
	SendRequestBroadcast(ctx context.Context, providerID string, payload RequestPushPayload) error
}

var ErrNoFCMToken = errors.New("provider has no FCM token")

type EventPublisher interface {
	PublishRequestAccepted(ctx context.Context, event RequestAcceptedEvent) error
	PublishRequestRejected(ctx context.Context, event RequestRejectedEvent) error
	PublishNoProviderFound(ctx context.Context, event NoProviderFoundEvent) error
}

type TaskEnqueuer interface {
	Enqueue(task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

type Config struct {
	InitialRadiusKM   float64
	RadiusIncrementKM float64
	MaxAttempts       int
	BroadcastWindow   time.Duration
}

type Service struct {
	repository    Repository
	redis         *redis.Client
	nearby        NearbyProviderFinder
	notifications NotificationSender
	events        EventPublisher
	tasks         TaskEnqueuer
	config        Config
	now           func() time.Time
}

func NewService(repository Repository, redisClient *redis.Client, nearby NearbyProviderFinder, notifications NotificationSender, events EventPublisher, tasks TaskEnqueuer, cfg Config) *Service {
	if cfg.InitialRadiusKM <= 0 {
		cfg.InitialRadiusKM = 5
	}
	if cfg.RadiusIncrementKM <= 0 {
		cfg.RadiusIncrementKM = 3
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.BroadcastWindow <= 0 {
		cfg.BroadcastWindow = 30 * time.Second
	}
	if notifications == nil {
		notifications = LoggingNotificationSender{}
	}
	return &Service{
		repository: repository, redis: redisClient, nearby: nearby, notifications: notifications,
		events: events, tasks: tasks, config: cfg, now: func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) StartBroadcast(ctx context.Context, event BookingDispatchCreatedEvent) (RequestBroadcast, error) {
	if err := validateBookingEvent(event); err != nil {
		return RequestBroadcast{}, err
	}
	if event.ServiceType == "" {
		event.ServiceType = "dispatch"
	}
	if existing, ok, err := s.repository.GetBroadcastByBookingID(ctx, event.BookingID); err != nil {
		return RequestBroadcast{}, err
	} else if ok {
		return existing, nil
	}
	now := s.now()
	payload, err := json.Marshal(event)
	if err != nil {
		return RequestBroadcast{}, err
	}
	broadcast, err := s.repository.CreateBroadcast(ctx, CreateBroadcastInput{
		BookingID: event.BookingID, ServiceType: event.ServiceType, RadiusKM: s.config.InitialRadiusKM,
		Attempt: 1, BroadcastAt: now, ExpiresAt: now.Add(s.config.BroadcastWindow), BookingPayload: payload,
	})
	if err != nil {
		if appErr := apperrors.From(err); appErr.Code == apperrors.CodeConflict {
			if existing, ok, getErr := s.repository.GetBroadcastByBookingID(ctx, event.BookingID); getErr == nil && ok {
				return existing, nil
			}
		}
		return RequestBroadcast{}, err
	}
	if err := s.broadcastToNearby(ctx, &broadcast, event, 1, s.config.InitialRadiusKM); err != nil {
		return RequestBroadcast{}, err
	}
	return broadcast, s.scheduleExpire(broadcast)
}

func (s *Service) ListInbox(ctx context.Context, providerID string, options ListInboxOptions) ([]ProviderRequestInboxItem, error) {
	if err := validateUUID(providerID, "provider_id"); err != nil {
		return nil, err
	}
	if options.Status == "" {
		options.Status = InboxStatusPending
	}
	inboxes, err := s.repository.ListProviderInbox(ctx, providerID, options)
	if err != nil {
		return nil, err
	}
	result := make([]ProviderRequestInboxItem, 0, len(inboxes))
	now := s.now()
	for _, inbox := range inboxes {
		item, active, err := NewProviderRequestInboxItem(inbox, now)
		if err != nil {
			return nil, err
		}
		if active {
			result = append(result, item)
		}
	}
	return result, nil
}

func (s *Service) GetInbox(ctx context.Context, inboxID, providerID string) (ProviderRequestInbox, error) {
	if err := validateUUID(inboxID, "id"); err != nil {
		return ProviderRequestInbox{}, err
	}
	inbox, ok, err := s.repository.GetProviderInboxByID(ctx, inboxID, providerID)
	if err != nil {
		return ProviderRequestInbox{}, err
	}
	if !ok {
		return ProviderRequestInbox{}, apperrors.NotFound("Request not found.", nil)
	}
	return inbox, nil
}

// GetRequestDetail returns the full parsed detail for a specific inbox entry (Phase 6F).
// receiver_phone is included; cross-provider access returns 404.
func (s *Service) GetRequestDetail(ctx context.Context, inboxID, providerID string) (RequestDetailResponse, error) {
	inbox, err := s.GetInbox(ctx, inboxID, providerID)
	if err != nil {
		return RequestDetailResponse{}, err
	}
	var booking BookingDispatchCreatedEvent
	if err := json.Unmarshal(inbox.BookingPayload, &booking); err != nil {
		return RequestDetailResponse{}, apperrors.Internal("Booking data is corrupted.", err)
	}
	now := s.now()
	remaining := int64(math.Floor(inbox.ExpiresAt.Sub(now).Seconds()))
	if remaining < 0 {
		remaining = 0
	}
	return RequestDetailResponse{
		InboxID: inbox.ID, BroadcastID: inbox.BroadcastID, BookingID: inbox.BookingID,
		Status: inbox.Status, FareAmount: booking.FareAmount, Currency: booking.Currency,
		PickupAddress: booking.PickupAddress, PickupLat: booking.PickupLat, PickupLng: booking.PickupLng,
		DropoffAddress: booking.DropoffAddress, DropoffLat: booking.DropoffLat, DropoffLng: booking.DropoffLng,
		PackageDesc: booking.PackageDesc, PackageWeight: booking.PackageWeight,
		PackageType: booking.PackageType, PackageSize: booking.PackageSize, IsFragile: booking.IsFragile,
		ServiceTier: normalizeServiceTier(booking.ServiceTier), ServiceTierLabel: serviceTierLabel(booking.ServiceTier),
		ReceiverName: booking.ReceiverName, ReceiverPhone: booking.ReceiverPhone,
		RemainingSeconds: remaining, ExpiresAt: inbox.ExpiresAt, ReceivedAt: inbox.ReceivedAt,
	}, nil
}

// Reject marks the provider's inbox entry as rejected and publishes request.rejected (Phase 6H).
// reason is optional ("" defaults to "other" in event logging).
// Broadcast status is intentionally NOT changed — rebroadcast is handled by the expire-window task.
// Phase 6K: rate limited; returns 410 if broadcast window has expired while inbox is still pending.
func (s *Service) Reject(ctx context.Context, inboxID, providerID, reason string) (RejectResponse, error) {
	if err := s.checkRejectRateLimit(ctx, providerID); err != nil {
		return RejectResponse{}, err
	}
	inbox, err := s.GetInbox(ctx, inboxID, providerID)
	if err != nil {
		return RejectResponse{}, err
	}
	if inbox.Status != InboxStatusPending {
		return RejectResponse{}, apperrors.Conflict("Request already responded to or expired.", nil)
	}
	// Phase 6K: pending inbox whose broadcast window has passed returns 410, not a silent reject.
	if !inbox.ExpiresAt.After(s.now()) {
		return RejectResponse{}, apperrors.New(http.StatusGone, "gone", "Request window has expired.", nil)
	}
	now := s.now()
	updated, err := s.repository.MarkInboxRejected(ctx, inboxID, providerID, now)
	if err != nil {
		return RejectResponse{}, err
	}
	if !updated {
		return RejectResponse{}, apperrors.Conflict("Request already responded to or expired.", nil)
	}
	effectiveReason := reason
	if effectiveReason == "" {
		effectiveReason = "other"
	}
	if s.events != nil {
		_ = s.events.PublishRequestRejected(ctx, RequestRejectedEvent{
			Event: TopicRequestRejected, BookingID: inbox.BookingID, BroadcastID: inbox.BroadcastID,
			InboxID: inbox.ID, ProviderID: providerID, Reason: effectiveReason,
			RejectedAt: now, OccurredAt: now,
		})
	}
	return RejectResponse{Message: "Request declined."}, nil
}

// Accept is the core accept flow with Redis atomic lock (Phase 6G).
// Phase 6K: rate limited (5 per 10 s per provider).
// Returns AcceptResponse with parsed booking details on success.
func (s *Service) Accept(ctx context.Context, inboxID, providerID string) (AcceptResponse, error) {
	if err := s.checkAcceptRateLimit(ctx, providerID); err != nil {
		return AcceptResponse{}, err
	}
	inbox, err := s.GetInbox(ctx, inboxID, providerID)
	if err != nil {
		return AcceptResponse{}, err
	}
	if inbox.Status != InboxStatusPending {
		if s.redis != nil {
			if accepted, redisErr := s.redis.Exists(ctx, RequestAcceptedKey(inbox.BookingID)).Result(); redisErr != nil {
				return AcceptResponse{}, redisErr
			} else if accepted > 0 {
				return AcceptResponse{}, apperrors.New(http.StatusConflict, "request_taken", "Another rider accepted this request first.", nil)
			}
		}
		return AcceptResponse{}, apperrors.Conflict("Request already responded to or expired.", nil)
	}
	// Check broadcast window expiry (DB clock).
	broadcast, ok, err := s.repository.GetBroadcastByID(ctx, inbox.BroadcastID)
	if err != nil {
		return AcceptResponse{}, err
	}
	if !ok {
		return AcceptResponse{}, apperrors.Internal("Broadcast record not found.", nil)
	}
	if broadcast.Status != BroadcastStatusBroadcasting {
		conflictCode := apperrors.CodeConflict
		if broadcast.Status == BroadcastStatusAccepted {
			conflictCode = "request_taken"
		}
		appErr := apperrors.New(http.StatusConflict, conflictCode, "This request is no longer available.", nil)
		return AcceptResponse{}, appErr
	}
	if !broadcast.ExpiresAt.After(s.now()) {
		return AcceptResponse{}, apperrors.New(http.StatusGone, "gone", "Request window has expired.", nil)
	}
	if s.redis == nil {
		return AcceptResponse{}, apperrors.Unavailable("Request acceptance is temporarily unavailable.", nil)
	}
	// Check Redis broadcasting window key (tracks live broadcast TTL).
	if exists, err := s.redis.Exists(ctx, RequestBroadcastingKey(inbox.BookingID)).Result(); err != nil {
		return AcceptResponse{}, err
	} else if exists == 0 {
		return AcceptResponse{}, apperrors.New(http.StatusGone, "gone", "Request window has expired.", nil)
	}
	// Check permanent accepted marker (fast-path: already taken).
	if accepted, err := s.redis.Exists(ctx, RequestAcceptedKey(inbox.BookingID)).Result(); err != nil {
		return AcceptResponse{}, err
	} else if accepted > 0 {
		return AcceptResponse{}, apperrors.New(http.StatusConflict, "request_taken", "Another rider accepted this request first.", nil)
	}
	// Acquire atomic lock — first writer wins.
	acquired, err := s.redis.SetNX(ctx, RequestLockKey(inbox.BookingID), providerID, AcceptLockTTL).Result()
	if err != nil {
		return AcceptResponse{}, err
	}
	if !acquired {
		return AcceptResponse{}, apperrors.New(http.StatusConflict, "request_taken", "Another rider accepted this request first.", nil)
	}
	defer releaseOwnedLock(ctx, s.redis, RequestLockKey(inbox.BookingID), providerID)

	// DB transaction: accept inbox, expire others, accept broadcast.
	now := s.now()
	if err := s.repository.MarkBroadcastAccepted(ctx, inbox.BroadcastID, inbox.BookingID, inbox.ID, providerID, now); err != nil {
		return AcceptResponse{}, err
	}
	// Set permanent accepted marker — do this only after successful DB transaction.
	if err := s.redis.Set(ctx, RequestAcceptedKey(inbox.BookingID), providerID, AcceptedMarkerTTL).Err(); err != nil {
		return AcceptResponse{}, err
	}
	_ = s.redis.Del(ctx, RequestBroadcastingKey(inbox.BookingID)).Err()

	// Parse booking payload for the response and event.
	var booking BookingDispatchCreatedEvent
	if jsonErr := json.Unmarshal(inbox.BookingPayload, &booking); jsonErr != nil {
		// Log and continue — event payload may be incomplete but the accept succeeded.
		log.Printf("request.accept: failed to parse booking_payload inbox_id=%s: %v", inbox.ID, jsonErr)
	}

	if s.events != nil {
		_ = s.events.PublishRequestAccepted(ctx, RequestAcceptedEvent{
			Event: TopicRequestAccepted, CorrelationID: booking.CorrelationID,
			BookingID: inbox.BookingID, BroadcastID: inbox.BroadcastID, InboxID: inbox.ID,
			ProviderID: providerID,
			FareAmount: booking.FareAmount, Currency: booking.Currency,
			PickupLat: booking.PickupLat, PickupLng: booking.PickupLng, PickupAddress: booking.PickupAddress,
			DropoffLat: booking.DropoffLat, DropoffLng: booking.DropoffLng, DropoffAddress: booking.DropoffAddress,
			DistanceKm:   0, // TODO: persist provider distance in inbox during broadcast
			ReceiverName: booking.ReceiverName, ReceiverPhone: booking.ReceiverPhone,
			PackageDesc: booking.PackageDesc, PackageWeight: booking.PackageWeight,
			PackageType: booking.PackageType, PackageSize: booking.PackageSize, IsFragile: booking.IsFragile,
			ServiceTier: normalizeServiceTier(booking.ServiceTier),
			AcceptedAt: now, OccurredAt: now,
		})
	}
	return AcceptResponse{
		BookingID: inbox.BookingID, BroadcastID: inbox.BroadcastID, InboxID: inbox.ID,
		Message:       "Request accepted. Head to the pickup location.",
		PickupAddress: booking.PickupAddress, PickupLat: booking.PickupLat, PickupLng: booking.PickupLng,
		DropoffAddress: booking.DropoffAddress, DropoffLat: booking.DropoffLat, DropoffLng: booking.DropoffLng,
		ReceiverName: booking.ReceiverName, ReceiverPhone: booking.ReceiverPhone,
		FareAmount: booking.FareAmount, Currency: booking.Currency,
	}, nil
}

// CancelBroadcastForBooking processes booking.dispatch.cancelled (Phase 6I).
// Idempotent: no-op if broadcast is missing, already accepted, cancelled, or no_provider_found.
func (s *Service) CancelBroadcastForBooking(ctx context.Context, bookingID string) error {
	broadcast, ok, err := s.repository.GetBroadcastByBookingID(ctx, bookingID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	switch broadcast.Status {
	case BroadcastStatusAccepted, BroadcastStatusCancelled, BroadcastStatusNoProviderFound:
		return nil
	}
	if err := s.repository.CancelBroadcast(ctx, broadcast.ID); err != nil {
		return err
	}
	if err := s.repository.MarkPendingInboxExpired(ctx, broadcast.ID); err != nil {
		return err
	}
	if s.redis != nil {
		_ = s.redis.Del(ctx, RequestBroadcastingKey(bookingID)).Err()
	}
	return nil
}

// checkAcceptRateLimit enforces 5 accept attempts per 10 s per provider (Phase 6K).
func (s *Service) checkAcceptRateLimit(ctx context.Context, providerID string) error {
	if s.redis == nil {
		return nil
	}
	key := AcceptRateLimitKey(providerID)
	count, err := s.redis.Incr(ctx, key).Result()
	if err != nil {
		return nil // fail open on transient Redis errors
	}
	if count == 1 {
		_ = s.redis.Expire(ctx, key, AcceptRateLimitWindow).Err()
	}
	if count > int64(AcceptRateLimitMax) {
		return apperrors.RateLimited("Too many accept attempts. Please slow down.", nil)
	}
	return nil
}

// checkRejectRateLimit enforces 10 reject attempts per 60 s per provider (Phase 6K).
func (s *Service) checkRejectRateLimit(ctx context.Context, providerID string) error {
	if s.redis == nil {
		return nil
	}
	key := RejectRateLimitKey(providerID)
	count, err := s.redis.Incr(ctx, key).Result()
	if err != nil {
		return nil // fail open on transient Redis errors
	}
	if count == 1 {
		_ = s.redis.Expire(ctx, key, RejectRateLimitWindow).Err()
	}
	if count > int64(RejectRateLimitMax) {
		return apperrors.RateLimited("Too many reject attempts. Please slow down.", nil)
	}
	return nil
}

func (s *Service) broadcastToNearby(ctx context.Context, broadcast *RequestBroadcast, event BookingDispatchCreatedEvent, attempt int, radius float64) error {
	if s.nearby == nil {
		return apperrors.Unavailable("Nearby provider lookup is unavailable.", nil)
	}
	providers, err := s.nearby.FindNearby(ctx, event.PickupLat, event.PickupLng, radius, 20)
	if err != nil {
		log.Printf("request nearby lookup failed booking_id=%s attempt=%d radius_km=%.2f error=%v; continuing with empty attempt", event.BookingID, attempt, radius, err)
		providers = []NearbyProvider{}
	}
	already, err := s.repository.ListAlreadyNotifiedProviders(ctx, event.BookingID)
	if err != nil {
		return err
	}
	excluded := map[string]struct{}{}
	for _, id := range already {
		excluded[id] = struct{}{}
	}
	ids := make([]string, 0, len(providers))
	distanceByProvider := make(map[string]float64, len(providers))
	for _, provider := range providers {
		if _, exists := excluded[provider.ProviderID]; !exists {
			ids = append(ids, provider.ProviderID)
			distanceByProvider[provider.ProviderID] = provider.DistanceKM
		}
	}
	inboxes, err := s.repository.CreateInboxRows(ctx, broadcast.ID, event.BookingID, ids)
	if err != nil {
		return err
	}
	for _, inbox := range inboxes {
		task, err := NewSendPushTask(SendPushPayload{
			ProviderID: inbox.ProviderID, InboxID: inbox.ID, BroadcastID: broadcast.ID, BookingID: event.BookingID,
			FareAmount: event.FareAmount, PickupAddress: event.PickupAddress, DropoffAddress: event.DropoffAddress,
			DistanceKM: distanceByProvider[inbox.ProviderID], PackageDesc: event.PackageDesc, ReceiverName: event.ReceiverName,
			ExpiresIn: int(s.config.BroadcastWindow.Seconds()),
		})
		if err != nil {
			return err
		}
		if s.tasks != nil {
			if _, err := s.tasks.Enqueue(task, asynq.Queue("critical")); err != nil {
				return err
			}
		}
	}
	broadcast.AttemptNumber = attempt
	broadcast.BroadcastRadiusKM = radius
	broadcast.ProvidersNotified += len(inboxes)
	if err := s.repository.UpdateBroadcastAttempt(ctx, broadcast.ID, attempt, radius, len(inboxes), broadcast.BroadcastAt, broadcast.ExpiresAt); err != nil {
		return err
	}
	if s.redis != nil {
		ttl := broadcast.ExpiresAt.Sub(s.now()) + 5*time.Second
		if ttl <= 0 {
			ttl = 5 * time.Second
		}
		if err := s.redis.Set(ctx, RequestBroadcastingKey(event.BookingID), broadcast.ID, ttl).Err(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) scheduleExpire(broadcast RequestBroadcast) error {
	if s.tasks == nil {
		return nil
	}
	task, err := NewExpireWindowTask(ExpireWindowPayload{
		BroadcastID: broadcast.ID, BookingID: broadcast.BookingID, AttemptNumber: broadcast.AttemptNumber,
	})
	if err != nil {
		return err
	}
	_, err = s.tasks.Enqueue(task, asynq.Queue("default"), asynq.ProcessAt(broadcast.ExpiresAt))
	return err
}

func releaseOwnedLock(ctx context.Context, client *redis.Client, key, value string) {
	if client == nil {
		return
	}
	_ = client.Eval(ctx, `if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) end return 0`, []string{key}, value).Err()
}

type HTTPNearbyClient struct {
	baseURL    string
	serviceKey string
	client     *http.Client
}

func NewHTTPNearbyClient(baseURL, serviceKey string) *HTTPNearbyClient {
	return &HTTPNearbyClient{baseURL: strings.TrimRight(baseURL, "/"), serviceKey: serviceKey, client: &http.Client{Timeout: 5 * time.Second}}
}

func (c *HTTPNearbyClient) FindNearby(ctx context.Context, lat, lng, radius float64, limit int) ([]NearbyProvider, error) {
	query := url.Values{}
	query.Set("lat", strconv.FormatFloat(lat, 'f', -1, 64))
	query.Set("lng", strconv.FormatFloat(lng, 'f', -1, 64))
	query.Set("radius", strconv.FormatFloat(radius, 'f', -1, 64))
	query.Set("limit", strconv.Itoa(limit))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/internal/nearby?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Internal-Service-Key", c.serviceKey)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var envelope struct {
		Success bool           `json:"success"`
		Data    NearbyResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK || !envelope.Success {
		return nil, fmt.Errorf("availability nearby returned status %d", resp.StatusCode)
	}
	return envelope.Data.Providers, nil
}

type LoggingNotificationSender struct{}

func (LoggingNotificationSender) SendRequestBroadcast(_ context.Context, providerID string, payload RequestPushPayload) error {
	log.Printf("request notification FCM integration pending provider_id=%s inbox_id=%s booking_id=%s", providerID, payload.InboxID, payload.BookingID)
	return nil
}

func validateBookingEvent(event BookingDispatchCreatedEvent) error {
	if err := validateUUID(event.BookingID, "booking_id"); err != nil {
		return err
	}
	if err := validateUUID(event.CustomerID, "customer_id"); err != nil {
		return err
	}
	if event.ServiceType != "" && event.ServiceType != "dispatch" {
		return apperrors.BadRequest("Unsupported service type.", nil)
	}
	if event.FareAmount < 0 {
		return apperrors.BadRequest("Fare amount cannot be negative.", nil)
	}
	if event.PickupLat < -90 || event.PickupLat > 90 || event.PickupLng < -180 || event.PickupLng > 180 {
		return apperrors.BadRequest("Pickup coordinates are invalid.", nil)
	}
	return nil
}

func validateUUID(value, field string) error {
	if _, err := uuid.Parse(strings.TrimSpace(value)); err != nil {
		appErr := apperrors.BadRequest("Check your details.", err)
		appErr.Fields = []apperrors.FieldViolation{{Field: field, Message: "Must be a valid UUID."}}
		appErr.Code = apperrors.CodeValidationFailed
		return appErr
	}
	return nil
}
