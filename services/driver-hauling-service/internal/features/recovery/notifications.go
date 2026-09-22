package recovery

// Section 14: telling the people a recovery concerns.
//
// Every intent is sent after the fact it announces has committed -- publish
// is only ever called once the transaction is done -- and a send that fails
// is logged, never returned. A customer whose replacement driver is on the
// way still has one if the notification did not go out.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/notifications"
)

const notificationSource = "driver-hauling-service"

// Audience is who a booking's recovery concerns beyond its providers.
type Audience struct {
	CustomerID string
	// OrganizationID is set only for organization-managed work.
	OrganizationID string
}

// NotificationAudience reads a booking's customer and managing organization.
func (r *Repository) NotificationAudience(ctx context.Context, bookingID string) (Audience, error) {
	var audience Audience
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(customer_id::text, ''),
		       CASE WHEN is_corporate_managed THEN COALESCE(corporate_organization_id, '') ELSE '' END
		  FROM haulage_bookings WHERE id::text = $1`, bookingID).
		Scan(&audience.CustomerID, &audience.OrganizationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Audience{}, nil
	}
	return audience, err
}

// recoveryIntents maps one recovery fact to the notifications it warrants.
//
// Pure, so every audience rule is testable without a database. Each intent's
// SourceEventID names the fact precisely enough that re-publishing it -- the
// custody path publishes a replacement twice, once when chosen and once when
// the transfer completes -- still notifies each person once.
func recoveryIntents(name string, current Case, audience Audience) []notifications.NetworkIntent {
	var intents []notifications.NetworkIntent
	add := func(event, recipientType, recipientID, fact string) {
		if recipientID == "" {
			return
		}
		intents = append(intents, notifications.NetworkIntent{
			EventType: event, RecipientType: recipientType, RecipientID: recipientID,
			SourceEventID: current.ID + ":" + fact, SourceAggregateType: "recovery_case",
			SourceAggregateID: current.ID, SourceVersion: current.Version,
			CorrelationRef: current.CorrelationID, Priority: notifications.PriorityHigh,
			Data: map[string]interface{}{"booking_id": current.BookingID, "recovery_case_id": current.ID},
		})
	}
	switch name {
	case EventRecoveryStarted:
		add(notifications.EventNetworkCustomerRecoveryStarted, notifications.RecipientCustomer, audience.CustomerID, "started")
		add(notifications.EventNetworkFleetRecoveryStarted, notifications.RecipientFleetOrganization, audience.OrganizationID, "started")
	case EventGuaranteeBreached:
		add(notifications.EventNetworkCustomerGuaranteeBreached, notifications.RecipientCustomer, audience.CustomerID, "breached")
	case EventRecoveryProviderReplaced:
		fact := "replaced:" + current.ReplacementProviderID
		add(notifications.EventNetworkCustomerProviderReplaced, notifications.RecipientCustomer, audience.CustomerID, fact)
		add(notifications.EventNetworkProviderJobReassigned, notifications.RecipientProvider, current.OriginalProviderID, fact)
		add(notifications.EventNetworkProviderReplacementAssigned, notifications.RecipientProvider, current.ReplacementProviderID, fact)
		add(notifications.EventNetworkFleetProviderReplaced, notifications.RecipientFleetOrganization, audience.OrganizationID, fact)
	case EventRecoveryFailed:
		add(notifications.EventNetworkCustomerRecoveryFailed, notifications.RecipientCustomer, audience.CustomerID, "failed")
	case EventCapabilityMismatch:
		// No compatible replacement: the case is parked for an operator.
		add(notifications.EventNetworkAdminOperatorRequired, notifications.RecipientAdminOperations,
			notifications.AdminOperationsQueue, "operator_required")
	}
	return intents
}

// notify sends the intents for one committed recovery fact.
func (s *Service) notify(ctx context.Context, name string, current Case) {
	if s.notifier == nil {
		return
	}
	audience, err := s.repo.NotificationAudience(ctx, current.BookingID)
	if err != nil {
		// Operations and providers can still be told even when the
		// customer cannot be found.
		logging.Error("hauling-recovery", "notification audience for booking %s: %v", current.BookingID, err)
	}
	s.notifyWithAudience(ctx, name, current, audience)
}

func (s *Service) notifyWithAudience(ctx context.Context, name string, current Case, audience Audience) {
	for _, intent := range recoveryIntents(name, current, audience) {
		if _, sendErr := s.notifier.Send(ctx, notifications.NetworkRequest(notificationSource, intent)); sendErr != nil {
			logging.Error("hauling-recovery", "notify %s about case %s: %v", intent.EventType, current.ID, sendErr)
		}
	}
}
