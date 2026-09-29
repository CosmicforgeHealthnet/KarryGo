package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/providercompat"
	"cosmicforge/logistics/shared/go/serviceauth"
	"sort"

	"cosmicforge/logistics/shared/go/networkanalytics"
)

const (
	ChannelPush      = "push"
	ChannelEmail     = "email"
	ChannelWebSocket = "websocket"
	ChannelInApp     = "in_app"

	PriorityLow    = "low"
	PriorityNormal = "normal"
	PriorityHigh   = "high"

	RequestStream    = "notification:requests"
	DeliveryStream   = "notification:deliveries"
	DeadLetterStream = "notification:dead_letters"
	ConsumerGroup    = "notification-service"
)

// Recipient types. These are the canonical audiences notification requests
// target. Using these constants keeps recipient_type values consistent across
// every service that sends notifications.
const (
	RecipientCustomer = "customer"
	RecipientProvider = "provider"
	RecipientVendor   = "vendor"
	RecipientStaff    = "vendor_staff"
)

// Event types grouped by domain. Senders should reference these constants
// instead of hand-typing event_type strings so the catalog stays consistent
// and templates can key off a known set of events.
const (
	// customer-service
	EventCustomerAuthOTP                 = "customer.auth.otp"
	EventCustomerProfileUpdated          = "customer.profile_updated"
	EventCustomerPhotoUpdated            = "customer.photo_updated"
	EventCustomerEmergencyContactAdded   = "customer.emergency_contact_added"
	EventCustomerEmergencyContactRemoved = "customer.emergency_contact_removed"

	// driver-hauling-service booking lifecycle
	EventBookingMatched             = "booking.matched"
	EventBookingAccepted            = "booking.accepted"
	EventBookingUnmatched           = "booking.unmatched"
	EventDriverEnRoutePickup        = "booking.en_route_pickup"
	EventDriverArrivedPickup        = "booking.arrived_at_pickup"
	EventDriverEnRouteDelivery      = "booking.en_route_delivery"
	EventCargoPickedUp              = "cargo.picked_up"
	EventCargoDelivered             = "cargo.delivered"
	EventBookingCompleted           = "booking.completed"
	EventBookingCancelled           = "booking.cancelled"
	EventBookingCancelledByProvider = "booking.cancelled_by_provider"

	// driver-hauling-service handover verification (QR/OTP pickup + delivery
	// confirmation). Pickup verification is checked by the customer against a
	// credential the driver's app displays, so this event never carries a
	// code. Delivery verification is checked by the driver against a code the
	// customer's app displays, so that one does.
	EventBookingPickupVerificationRequired = "booking.pickup_verification_required"
	EventBookingPickupVerified             = "booking.pickup_verified"
	EventBookingDeliveryCodeReady          = "booking.delivery_code_ready"

	// driver-dispatch-delivery-service booking lifecycle. Kept distinct from
	// the hauling keys above — sharing event_type strings across services
	// meant a bike-delivery customer received hauling's "cargo delivered"
	// copy and a rider got "New haulage request" for a bike job.
	EventDeliveryMatched             = "delivery.matched"
	EventDeliveryAccepted            = "delivery.accepted"
	EventDeliveryUnmatched           = "delivery.unmatched"
	EventDeliveryEnRoutePickup       = "delivery.en_route_pickup"
	EventDeliveryArrivedPickup       = "delivery.arrived_at_pickup"
	EventPackagePickedUp             = "package.picked_up"
	EventPackageDelivered            = "package.delivered"
	EventDeliveryCompleted           = "delivery.completed"
	EventDeliveryCancelled           = "delivery.cancelled"
	EventDeliveryCancelledByProvider = "delivery.cancelled_by_provider"

	// driver-dispatch-delivery-service handover verification (QR/OTP pickup +
	// delivery confirmation) — mirrors hauling's pair, kept in this service's
	// own namespace. Pickup verification is checked by the customer against
	// a credential the rider's app displays, so this event never carries a
	// code. Delivery verification is checked by the rider against a code the
	// customer's app displays, so that one does.
	EventDeliveryPickupVerificationRequired = "delivery.pickup_verification_required"
	EventPackageDeliveryCodeReady           = "package.delivery_code_ready"

	// payment-wallet-service
	EventPaymentTopupSuccess = "payment.topup_success"
	EventPaymentSuccess      = "payment.success"
	EventPaymentFailed       = "payment.failed"
	EventWithdrawalCompleted = "withdrawal.completed"
	EventWithdrawalFailed    = "withdrawal.failed"
	EventWithdrawalReversed  = "withdrawal.reversed"
	EventRefundCompleted     = "refund.completed"
	EventRefundFailed        = "refund.failed"

	// commerce
	EventCommerceCustomerETAChanged = "commerce.customer_eta.changed"
)

// IdempotencyKey builds a deterministic idempotency key for a notification
// request from the source service, event type, and the domain entity the event
// is about. Because it is deterministic (no timestamp), a genuine retry of the
// same logical event dedupes against the existing notification message instead
// of creating a duplicate.
func IdempotencyKey(sourceService, eventType, entityID string) string {
	return fmt.Sprintf("%s:%s:%s", sourceService, eventType, entityID)
}

var DefaultChannels = []string{ChannelPush, ChannelWebSocket, ChannelInApp}

// isProviderRecipient reports whether a notification is addressed to a
// provider, whose financial visibility is policy-bound.
func isProviderRecipient(recipientType string) bool {
	switch strings.ToLower(strings.TrimSpace(recipientType)) {
	case RecipientProvider, "truck_provider", "taxi_provider", "dispatch_provider", "rider", "driver":
		return true
	default:
		return false
	}
}

type Recipient struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
}

type Request struct {
	IDempotencyKey      string                 `json:"idempotency_key"`
	SourceService       string                 `json:"source_service"`
	SourceAggregateType string                 `json:"source_aggregate_type,omitempty"`
	SourceAggregateID   string                 `json:"source_aggregate_id,omitempty"`
	SourceEventID       string                 `json:"source_event_id,omitempty"`
	SourceVersion       int64                  `json:"source_version,omitempty"`
	CorrelationRef      string                 `json:"correlation_ref,omitempty"`
	EventType           string                 `json:"event_type"`
	Recipient           Recipient              `json:"recipient"`
	Channels            []string               `json:"channels,omitempty"`
	TemplateKey         string                 `json:"template_key,omitempty"`
	Locale              string                 `json:"locale,omitempty"`
	Title               string                 `json:"title,omitempty"`
	Body                string                 `json:"body,omitempty"`
	Data                map[string]interface{} `json:"data,omitempty"`
	TemplateData        map[string]interface{} `json:"template_data,omitempty"`
	Priority            string                 `json:"priority,omitempty"`
}

type SendResponse struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}

func (r Request) Validate() error {
	var fields []apperrors.FieldViolation
	if strings.TrimSpace(r.IDempotencyKey) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "idempotency_key", Message: "Idempotency key is required."})
	}
	if strings.TrimSpace(r.SourceService) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "source_service", Message: "Source service is required."})
	}
	if strings.TrimSpace(r.EventType) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "event_type", Message: "Event type is required."})
	}
	if r.SourceVersion < 0 {
		fields = append(fields, apperrors.FieldViolation{Field: "source_version", Message: "Source version cannot be negative."})
	}
	if (strings.TrimSpace(r.SourceAggregateType) == "") != (strings.TrimSpace(r.SourceAggregateID) == "") {
		fields = append(fields, apperrors.FieldViolation{Field: "source_aggregate", Message: "Source aggregate type and id must be supplied together."})
	}
	if strings.TrimSpace(r.Recipient.Type) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "recipient.type", Message: "Recipient type is required."})
	}
	if strings.TrimSpace(r.Recipient.ID) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "recipient.id", Message: "Recipient id is required."})
	}
	if strings.TrimSpace(r.TemplateKey) == "" && strings.TrimSpace(r.Title) == "" && strings.TrimSpace(r.Body) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "template_key", Message: "Template key or inline title/body is required."})
	}
	for _, channel := range r.Channels {
		if !IsValidChannel(channel) {
			fields = append(fields, apperrors.FieldViolation{Field: "channels", Message: "Unsupported notification channel."})
			break
		}
	}
	if r.Priority != "" && r.Priority != PriorityLow && r.Priority != PriorityNormal && r.Priority != PriorityHigh {
		fields = append(fields, apperrors.FieldViolation{Field: "priority", Message: "Priority must be low, normal, or high."})
	}
	// Section 14 VIS-017. A notification's data bag takes anything, which is
	// what makes it a way around the provider API's own care: a field the
	// provider surface withholds can reach a provider's phone through a push
	// payload that nobody classified.
	//
	// Refused rather than quietly stripped. A caller that put the
	// organization's share in a provider notification has a bug, and silently
	// sending the rest would leave it in place for the next field.
	for _, bag := range []struct {
		field   string
		payload map[string]interface{}
	}{{"data", r.Data}, {"template_data", r.TemplateData}} {
		if !legacyCredentialDeliveryEvent(r.EventType) {
			leaked := unsafeCredentialKeys(bag.payload)
			if custodyNotificationEvent(r.EventType) {
				leaked = append(leaked, unsafeCustodyKeys(bag.payload)...)
			}
			if len(leaked) > 0 {
				fields = append(fields, apperrors.FieldViolation{Field: bag.field, Message: "Notification payload cannot carry credentials: " + strings.Join(leaked, ", ")})
			}
		}
		if leaked := providercompat.ConcealedFinancialFields(bag.payload); len(leaked) > 0 && isProviderRecipient(r.Recipient.Type) {
			fields = append(fields, apperrors.FieldViolation{
				Field:   bag.field,
				Message: "Provider notifications cannot carry organization or platform financial fields: " + strings.Join(leaked, ", "),
			})
		}
	}
	// Section 14: a network notification is about work, not people or money.
	// It carries references -- a booking, a case, a provider id -- and the
	// template supplies the words. A location, a contact detail, an amount or
	// a credential in its data is refused, for the same reason as above.
	if strings.HasPrefix(r.EventType, "network.") {
		for _, bag := range []struct {
			field   string
			payload map[string]interface{}
		}{{"data", r.Data}, {"template_data", r.TemplateData}} {
			if leaked := unsafeNetworkKeys(bag.payload); len(leaked) > 0 {
				fields = append(fields, apperrors.FieldViolation{
					Field:   bag.field,
					Message: "Network notifications carry references only; refused: " + strings.Join(leaked, ", "),
				})
			}
		}
	}
	if len(fields) > 0 {
		return apperrors.Validation("Check your notification request.", fields)
	}
	return nil
}

func custodyNotificationEvent(event string) bool {
	return event == "booking.delivery_code_ready" || event == "package.delivery_code_ready" ||
		event == "taxi.goods.pickup_credential_ready" || event == "dispatch.delivery_handover_challenge" ||
		event == "dispatch.handover_resend" || strings.HasPrefix(event, "dispatch.vendor_delivery.handover_")
}

func unsafeCustodyKeys(payload map[string]interface{}) []string {
	var leaked []string
	var walk func(any, string, int)
	walk = func(value any, prefix string, depth int) {
		if depth > 8 {
			leaked = append(leaked, prefix)
			return
		}
		switch values := value.(type) {
		case map[string]interface{}:
			for key, nested := range values {
				path := prefix + key
				switch strings.ToLower(strings.ReplaceAll(key, "-", "_")) {
				case "code", "qr", "credential", "delivery_code", "pickup_code":
					leaked = append(leaked, path)
				}
				walk(nested, path+".", depth+1)
			}
		case []interface{}:
			for _, nested := range values {
				walk(nested, prefix, depth+1)
			}
		}
	}
	walk(payload, "", 0)
	return leaked
}

// The legacy custody and login templates still need their own verified-read
// migration. All other producers fail closed if they attempt to add a secret.
func legacyCredentialDeliveryEvent(event string) bool {
	switch event {
	case "corporate.auth.otp":
		return true
	}
	return false
}

func unsafeCredentialKeys(payload map[string]interface{}) []string {
	var leaked []string
	var walk func(map[string]interface{}, string, int)
	walk = func(values map[string]interface{}, prefix string, depth int) {
		if depth > 8 {
			leaked = append(leaked, prefix)
			return
		}
		for key, value := range values {
			path := prefix + key
			switch strings.ToLower(strings.ReplaceAll(key, "-", "_")) {
			case "otp", "qr_token", "qrtoken", "share_token", "bearer_token", "hmac_secret", "service_token", "account_number", "bank_account", "access_code", "cvv", "pan":
				leaked = append(leaked, path)
			}
			switch nested := value.(type) {
			case map[string]interface{}:
				walk(nested, path+".", depth+1)
			case []interface{}:
				for _, item := range nested {
					if object, ok := item.(map[string]interface{}); ok {
						walk(object, path+"[].", depth+1)
					}
				}
			}
		}
	}
	walk(payload, "", 0)
	return leaked
}

// unsafeNetworkKeys lists the keys the network minimization would drop.
func unsafeNetworkKeys(payload map[string]interface{}) []string {
	kept := networkanalytics.Minimize(payload)
	var leaked []string
	for key := range payload {
		if _, ok := kept[key]; !ok {
			leaked = append(leaked, key)
		}
	}
	sort.Strings(leaked)
	return leaked
}

func (r Request) EventName() string {
	if r.EventType == "" {
		return events.NotificationSend
	}
	return r.EventType
}

func IsValidChannel(channel string) bool {
	switch channel {
	case ChannelPush, ChannelEmail, ChannelWebSocket, ChannelInApp:
		return true
	default:
		return false
	}
}

// Notifier is the minimal interface service usecases should depend on when they
// need to send notifications. Client implements it; tests can supply a fake.
type Notifier interface {
	Send(ctx context.Context, request Request) (SendResponse, error)
}

type Client struct {
	BaseURL     string
	HTTPClient  *http.Client
	ServiceName string
	Secret      []byte
}

func (c Client) Send(ctx context.Context, request Request) (SendResponse, error) {
	if err := request.Validate(); err != nil {
		return SendResponse{}, err
	}
	body, err := json.Marshal(request)
	if err != nil {
		return SendResponse{}, err
	}

	url := strings.TrimRight(c.BaseURL, "/") + "/api/v1/notifications/send"
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return SendResponse{}, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	if err := serviceauth.SignRequest(httpRequest, c.ServiceName, c.Secret, body, time.Now()); err != nil {
		return SendResponse{}, err
	}

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(httpRequest)
	if err != nil {
		return SendResponse{}, err
	}
	defer response.Body.Close()

	var envelope struct {
		Success bool         `json:"success"`
		Data    SendResponse `json:"data"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return SendResponse{}, err
	}
	if response.StatusCode >= http.StatusBadRequest || !envelope.Success {
		if envelope.Error.Message == "" {
			envelope.Error.Message = "Notification request failed."
		}
		return SendResponse{}, errors.New(envelope.Error.Message)
	}
	return envelope.Data, nil
}

type RedisPublisher struct {
	Client *redis.Client
	Stream string
}

func (p RedisPublisher) Publish(ctx context.Context, request Request) error {
	if err := request.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}

	stream := p.Stream
	if stream == "" {
		stream = RequestStream
	}
	return p.Client.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]interface{}{
			"event":   events.NotificationSend,
			"payload": string(payload),
		},
	}).Err()
}
