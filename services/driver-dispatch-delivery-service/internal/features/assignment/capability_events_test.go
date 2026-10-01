package assignment

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/parcelfit"
	"cosmicforge/logistics/shared/go/capabilityfit"
)

func eventPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("DISPATCH_TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("DISPATCH_TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`DELETE FROM dispatch_marketplace_outbox WHERE event_type=$1`, EventCapabilityMismatch); err != nil {
		t.Fatalf("clean: %v", err)
	}
	return pool
}

func gateWithEvents(t *testing.T, pool *pgxpool.Pool, authority CapabilityAuthority) *CapabilityGate {
	t.Helper()
	return NewCapabilityGate(authority, CapabilityConfig{Enabled: true, Enforce: true}).
		WithEvents(NewCapabilityEventSink(pool, true,
			func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }))
}

// A mission that found nobody is worth an event, and the payload must carry
// the envelope fields a consumer needs for idempotency and ordering.
func TestCapabilityMismatchIsPublishedWhenNobodyFits(t *testing.T) {
	pool := eventPool(t)
	authority := &fakeAuthority{bikes: map[string][]parcelfit.Capability{
		"p1": {smallBike("b1")}, "p2": {smallBike("b2")},
	}}
	gate := gateWithEvents(t, pool, authority)

	result := gate.Apply(context.Background(), "mission-nobody", tenKgParcel(), candidates("p1", "p2"))
	if len(result) != 0 {
		t.Fatalf("expected an empty candidate set, got %d", len(result))
	}

	var payload []byte
	if err := pool.QueryRow(context.Background(),
		`SELECT payload FROM dispatch_marketplace_outbox
		  WHERE event_type=$1 AND aggregate_id='mission-nobody'`, EventCapabilityMismatch).Scan(&payload); err != nil {
		t.Fatalf("event was not published: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("payload: %v", err)
	}
	for _, field := range []string{
		"event_id", "schema_version", "source_service", "source_entity_type",
		"source_entity_id", "source_version", "occurred_at", "reason_codes",
	} {
		if _, ok := decoded[field]; !ok {
			t.Fatalf("payload missing %q: %v", field, decoded)
		}
	}
	reasons, _ := decoded["reason_codes"].(string)
	if !strings.Contains(reasons, capabilityfit.ReasonCapacityExceeded) {
		t.Fatalf("reason_codes = %q, want the stable fit code", reasons)
	}
	// No customer or parcel content travels on this fact.
	for _, forbidden := range []string{"customer_id", "receiver_name", "address", "parcel_ref"} {
		if _, present := decoded[forbidden]; present {
			t.Fatalf("payload carries %q", forbidden)
		}
	}
}

// A mission that lost some candidates but still matched is not a mismatch, and
// publishing it would drown the signal that needs acting on.
func TestNoEventWhenSomeCandidatesStillFit(t *testing.T) {
	pool := eventPool(t)
	authority := &fakeAuthority{bikes: map[string][]parcelfit.Capability{
		"fits": {capableBike("b1")}, "too-small": {smallBike("b2")},
	}}
	gate := gateWithEvents(t, pool, authority)

	result := gate.Apply(context.Background(), "mission-partial", tenKgParcel(),
		candidates("fits", "too-small"))
	if len(result) != 1 {
		t.Fatalf("expected one survivor, got %d", len(result))
	}

	var published int
	_ = pool.QueryRow(context.Background(),
		`SELECT count(*) FROM dispatch_marketplace_outbox
		  WHERE event_type=$1 AND aggregate_id='mission-partial'`, EventCapabilityMismatch).Scan(&published)
	if published != 0 {
		t.Fatalf("a partially-filtered mission published %d mismatch events", published)
	}
}

// Shadow mode changes no assignment, so it must not publish a fact claiming
// one failed.
func TestShadowModePublishesNoMismatchEvent(t *testing.T) {
	pool := eventPool(t)
	authority := &fakeAuthority{bikes: map[string][]parcelfit.Capability{"p1": {smallBike("b1")}}}
	gate := NewCapabilityGate(authority, CapabilityConfig{Enabled: true, Enforce: false}).
		WithEvents(NewCapabilityEventSink(pool, true, nil))

	gate.Apply(context.Background(), "mission-shadow", tenKgParcel(), candidates("p1"))

	var published int
	_ = pool.QueryRow(context.Background(),
		`SELECT count(*) FROM dispatch_marketplace_outbox
		  WHERE event_type=$1 AND aggregate_id='mission-shadow'`, EventCapabilityMismatch).Scan(&published)
	if published != 0 {
		t.Fatalf("shadow mode published %d mismatch events", published)
	}
}

// An unwired or disabled sink must be a silent no-op, never a panic and never
// a failed assignment.
func TestDisabledSinkIsASilentNoOp(t *testing.T) {
	authority := &fakeAuthority{bikes: map[string][]parcelfit.Capability{"p1": {smallBike("b1")}}}
	for _, sink := range []*CapabilityEventSink{
		nil,
		NewCapabilityEventSink(nil, true, nil),
		NewCapabilityEventSink(nil, false, nil),
	} {
		gate := NewCapabilityGate(authority, CapabilityConfig{Enabled: true, Enforce: true}).WithEvents(sink)
		if result := gate.Apply(context.Background(), "m", tenKgParcel(), candidates("p1")); len(result) != 0 {
			t.Fatalf("gate behaviour changed with sink %v", sink)
		}
	}
}
