package movement

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	bookingmodels "cosmicforge/logistics/services/taxi-service/internal/features/booking/models"
	"cosmicforge/logistics/services/taxi-service/internal/outbox"
	"cosmicforge/logistics/shared/go/events"
)

func TestBookingStatesMapToCanonicalEvents(t *testing.T) {
	cases := map[bookingmodels.Status][2]string{
		bookingmodels.StatusPendingMatch:    {events.MovementStateSearching, events.TaxiBookingCreated},
		bookingmodels.StatusAccepted:        {events.MovementStateAssigned, events.TaxiBookingAssigned},
		bookingmodels.StatusArrivedAtPickup: {events.MovementStateArrived, events.TaxiBookingProviderArrived},
		bookingmodels.StatusInProgress:      {events.MovementStateInProgress, events.TaxiBookingStarted},
		bookingmodels.StatusCompleted:       {events.MovementStateCompleted, events.TaxiBookingCompleted},
		bookingmodels.StatusCancelled:       {events.MovementStateCancelled, events.TaxiBookingCancelled},
		bookingmodels.StatusUnmatched:       {events.MovementStateUnmatched, events.TaxiBookingUnmatched},
	}
	for status, want := range cases {
		state, ok := BookingState(status)
		if !ok || state != want[0] || BookingEventName(state) != want[1] {
			t.Errorf("%s -> %s/%s, want %v", status, state, BookingEventName(state), want)
		}
	}
	for status, want := range map[string]string{"MATCHING": events.MovementStateSearching, "PICKUP_HANDOVER_REQUIRED": events.MovementStateArrived,
		"IN_TRANSIT": events.MovementStateInProgress, "DELIVERED": events.MovementStateDelivered, "COMPLETED": events.MovementStateCompleted} {
		if got, ok := GoodsState(status); !ok || got != want {
			t.Errorf("goods %s -> %s, want %s", status, got, want)
		}
	}
}

func TestDisabledWriterIsANoOp(t *testing.T) {
	var writer *Writer
	if err := writer.WriteBooking(context.Background(), nil, bookingmodels.Booking{Status: bookingmodels.StatusAccepted}, ""); err != nil {
		t.Fatalf("nil writer: %v", err)
	}
	if err := (&Writer{}).WriteGoodsMission(context.Background(), nil, GoodsMissionRefs{Status: "COMPLETED"}, "x", ""); err != nil {
		t.Fatalf("disabled writer: %v", err)
	}
}

func TestDestinationRejectsNonEnvelopeRows(t *testing.T) {
	d := Destination{}
	if err := d.Deliver(context.Background(), outbox.Record{Payload: json.RawMessage(`not json`)}); err == nil {
		t.Fatal("a corrupt outbox row must fail delivery so it is retried and eventually dead-lettered")
	}
	envelope, err := events.NewMovementEnvelope("e1", events.TaxiBookingCreated, SourceService, "booking", 1, time.Now(), "", "",
		events.MovementSummary{CustomerID: "c", Authority: events.MovementAuthorityTaxi, SourceRef: "b", MovementType: events.MovementTypePassenger, State: events.MovementStateSearching})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(envelope)
	if err := d.Deliver(context.Background(), outbox.Record{Payload: raw}); err == nil {
		t.Fatal("an unconfigured destination must report failure instead of acknowledging")
	}
}
