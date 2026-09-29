package outbox

// Section 14: telling the people a Taxi recovery concerns.
//
// Rows reach here only through migration 020's copy of a committed recovery
// fact, so a notification is never sent about a change that rolled back, and
// a failed send leaves the recovery exactly as it is: the row is retried, the
// recovery state is never touched.

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/notifications"
)

// TargetNotification is the outbox target for recovery notifications.
const TargetNotification = "notification-service"

// NotificationPublisher queues a notification (the durable Redis queue).
type NotificationPublisher interface {
	Publish(ctx context.Context, request notifications.Request) error
}

// RecoveryAudience is who one recovery concerns.
type RecoveryAudience struct {
	CustomerID            string
	OrganizationID        string
	OriginalProviderID    string
	ReplacementProviderID string
	Goods                 bool
}

// AudienceReader resolves a recovery's audience from Taxi's own rows.
type AudienceReader interface {
	RecoveryAudience(ctx context.Context, aggregateType, aggregateID string) (RecoveryAudience, error)
}

// RecoveryNotificationDestination turns a recovery fact into notifications.
type RecoveryNotificationDestination struct {
	Publisher NotificationPublisher
	Audience  AudienceReader
	Enabled   bool
}

// Deliver sends the notifications one recovery fact warrants.
func (d RecoveryNotificationDestination) Deliver(ctx context.Context, record Record) error {
	if !d.Enabled || d.Publisher == nil || d.Audience == nil {
		return &TerminalDeliveryError{Code: "recovery_notifications_not_configured"}
	}
	audience, err := d.Audience.RecoveryAudience(ctx, record.AggregateType, record.AggregateID)
	if err != nil {
		// Could not read who to tell: retryable, the fact is not lost.
		return err
	}
	intents := RecoveryIntents(record, audience)
	if strings.HasPrefix(record.EventType, "taxi.booking.") {
		intents = RideLifecycleIntents(record, audience)
	}
	for _, intent := range intents {
		if err = d.Publisher.Publish(ctx, notifications.NetworkRequest("driver-taxi-service", intent)); err != nil {
			return err
		}
	}
	return nil
}

// RideLifecycleIntents announces committed passenger-ride milestones. The
// outbox row supplies the immutable fact; Taxi's booking row supplies the
// customer. No event payload or contact/driver detail is forwarded.
func RideLifecycleIntents(record Record, audience RecoveryAudience) []notifications.NetworkIntent {
	if audience.Goods || audience.CustomerID == "" {
		return nil
	}
	var event string
	switch record.EventType {
	case EventBookingAccepted, EventBookingEnRoutePickup, EventBookingArrived, EventBookingStarted,
		EventBookingCompleted, EventBookingCancelled,
		"taxi.booking.auto_completed", "taxi.booking.admin_cancelled":
		event = record.EventType
	default:
		return nil
	}
	source := record.IdempotencyKey
	if source == "" {
		source = record.ID
	}
	return []notifications.NetworkIntent{{
		EventType: event, RecipientType: notifications.RecipientCustomer,
		RecipientID: audience.CustomerID, SourceEventID: source,
		SourceAggregateType: record.AggregateType, SourceAggregateID: record.AggregateID,
		Data:     map[string]interface{}{"booking_id": record.AggregateID},
		Priority: notifications.PriorityNormal,
	}}
}

// RecoveryIntents maps one committed recovery fact to its notifications.
// Pure, so every audience rule is testable without a database or a queue.
func RecoveryIntents(record Record, audience RecoveryAudience) []notifications.NetworkIntent {
	fact := strings.TrimSuffix(strings.TrimSpace(record.EventType), ".v1")
	source := record.IdempotencyKey
	if source == "" {
		source = record.ID
	}
	subjectKey := "booking_id"
	if audience.Goods {
		subjectKey = "mission_id"
	}
	var intents []notifications.NetworkIntent
	add := func(event, recipientType, recipientID string) {
		if recipientID == "" {
			return
		}
		intents = append(intents, notifications.NetworkIntent{
			EventType: event, RecipientType: recipientType, RecipientID: recipientID,
			SourceEventID: source, SourceAggregateType: record.AggregateType, SourceAggregateID: record.AggregateID,
			Priority: notifications.PriorityHigh,
			Data:     map[string]interface{}{subjectKey: record.AggregateID},
		})
	}
	// A Taxi Goods customer belongs to the vendor that sold the order; the
	// vendor's own notifications tell them. Taxi tells passengers directly.
	customer := audience.CustomerID
	if audience.Goods {
		customer = ""
	}
	switch fact {
	case "taxi.recovery.started":
		add(notifications.EventNetworkCustomerRecoveryStarted, notifications.RecipientCustomer, customer)
		add(notifications.EventNetworkFleetRecoveryStarted, notifications.RecipientFleetOrganization, audience.OrganizationID)
	case "taxi.recovery.provider_replaced":
		add(notifications.EventNetworkCustomerProviderReplaced, notifications.RecipientCustomer, customer)
		add(notifications.EventNetworkProviderJobReassigned, notifications.RecipientProvider, audience.OriginalProviderID)
		add(notifications.EventNetworkProviderReplacementAssigned, notifications.RecipientProvider, audience.ReplacementProviderID)
		add(notifications.EventNetworkFleetProviderReplaced, notifications.RecipientFleetOrganization, audience.OrganizationID)
	case "taxi.recovery.custody_transfer_required":
		add(notifications.EventNetworkProviderRecoveryTransfer, notifications.RecipientProvider, audience.OriginalProviderID)
		add(notifications.EventNetworkProviderRecoveryTransfer, notifications.RecipientProvider, audience.ReplacementProviderID)
	case "taxi.recovery.failed":
		add(notifications.EventNetworkCustomerRecoveryFailed, notifications.RecipientCustomer, customer)
		add(notifications.EventNetworkAdminRecoveryFailed, notifications.RecipientAdminOperations, notifications.AdminOperationsQueue)
	case "taxi.guarantee.breached":
		add(notifications.EventNetworkCustomerGuaranteeBreached, notifications.RecipientCustomer, customer)
	}
	return intents
}

// PostgresAudience reads the audience from taxi_bookings, taxi_goods_missions
// and the newest recovery case for the subject.
type PostgresAudience struct{ DB *pgxpool.Pool }

// RecoveryAudience implements AudienceReader.
func (a PostgresAudience) RecoveryAudience(ctx context.Context, aggregateType, aggregateID string) (RecoveryAudience, error) {
	var audience RecoveryAudience
	goods := strings.EqualFold(aggregateType, "taxi_goods_mission")
	audience.Goods = goods
	caseColumn := "booking_id"
	if goods {
		caseColumn = "goods_mission_id"
	}
	err := a.DB.QueryRow(ctx, `SELECT COALESCE(original_provider_id::text,''), COALESCE(replacement_provider_id::text,'')
		FROM taxi_recovery_cases WHERE `+caseColumn+`::text = $1 ORDER BY detected_at DESC LIMIT 1`, aggregateID).
		Scan(&audience.OriginalProviderID, &audience.ReplacementProviderID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return audience, err
	}
	if !goods {
		err = a.DB.QueryRow(ctx, `SELECT customer_id::text, COALESCE(organization_id,'') FROM taxi_bookings
			WHERE id::text = $1`, aggregateID).Scan(&audience.CustomerID, &audience.OrganizationID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return audience, err
		}
	}
	return audience, nil
}
