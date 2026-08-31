package bookingusecases

import (
	"testing"

	"cosmicforge/logistics/services/taxi-service/internal/outbox"
)

func TestActorForTransition_MatchedAndUnmatchedAreSystem(t *testing.T) {
	for _, evt := range []string{outbox.EventBookingMatched, outbox.EventBookingUnmatched} {
		actorType, actorID := actorForTransition(evt, "", "provider-1", "customer-1")
		if actorType != "system" || actorID != "matcher" {
			t.Errorf("event=%s: actor = (%s,%s), want (system,matcher)", evt, actorType, actorID)
		}
	}
}

func TestActorForTransition_OfferExpiredDistinguishesRejectFromTimeout(t *testing.T) {
	actorType, actorID := actorForTransition(outbox.EventBookingOfferExpired, "offer_rejected", "provider-1", "customer-1")
	if actorType != "provider" || actorID != "provider-1" {
		t.Errorf("reject: actor = (%s,%s), want (provider,provider-1)", actorType, actorID)
	}

	actorType, actorID = actorForTransition(outbox.EventBookingOfferExpired, "offer_timed_out", "provider-1", "customer-1")
	if actorType != "system" || actorID != "matcher" {
		t.Errorf("timeout: actor = (%s,%s), want (system,matcher)", actorType, actorID)
	}
}

func TestActorForTransition_CancelledDistinguishesCustomerFromProvider(t *testing.T) {
	actorType, actorID := actorForTransition(outbox.EventBookingCancelled, "cancelled_by_customer: changed my mind", "provider-1", "customer-1")
	if actorType != "customer" || actorID != "customer-1" {
		t.Errorf("customer cancel: actor = (%s,%s), want (customer,customer-1)", actorType, actorID)
	}

	actorType, actorID = actorForTransition(outbox.EventBookingCancelled, "cancelled_by_provider: no-show", "provider-1", "customer-1")
	if actorType != "provider" || actorID != "provider-1" {
		t.Errorf("provider cancel: actor = (%s,%s), want (provider,provider-1)", actorType, actorID)
	}
}

func TestActorForTransition_DefaultsToProviderWhenPresent(t *testing.T) {
	for _, evt := range []string{outbox.EventBookingAccepted, outbox.EventBookingEnRoutePickup, outbox.EventBookingArrived, outbox.EventBookingStarted, outbox.EventBookingCompleted} {
		actorType, actorID := actorForTransition(evt, "", "provider-1", "customer-1")
		if actorType != "provider" || actorID != "provider-1" {
			t.Errorf("event=%s: actor = (%s,%s), want (provider,provider-1)", evt, actorType, actorID)
		}
	}
}

func TestActorForTransition_DefaultsToSystemWhenNoProvider(t *testing.T) {
	actorType, actorID := actorForTransition(outbox.EventBookingUnmatched, "", "", "customer-1")
	if actorType != "system" || actorID != "matcher" {
		t.Errorf("actor = (%s,%s), want (system,matcher)", actorType, actorID)
	}
}
