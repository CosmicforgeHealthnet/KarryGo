package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	sharedevents "cosmicforge/logistics/shared/go/events"
)

// The trigger decides what reaches analytics; the shared classification
// decides what analytics understands. They must name the same events.
func TestTheFanoutTriggerMatchesTheSharedClassification(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/019_network_analytics_fanout.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)event_type IN \((.*?)\)`).FindStringSubmatch(string(raw))
	if block == nil {
		t.Fatal("trigger event list not found")
	}
	var inTrigger []string
	for _, match := range regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(block[1], -1) {
		inTrigger = append(inTrigger, match[1])
	}
	want := append([]string(nil), AnalyticsEventNames...)
	sort.Strings(inTrigger)
	sort.Strings(want)
	if strings.Join(inTrigger, ",") != strings.Join(want, ",") {
		t.Fatalf("trigger copies %v\nanalytics classifies %v", inTrigger, want)
	}
}

func TestAnalyticsEnvelopeIsBuiltFromTheStoredRow(t *testing.T) {
	at := time.Date(2026, 9, 1, 10, 0, 0, 123000, time.UTC)
	record := Record{ID: uuid.NewString(), AggregateType: "taxi_goods_mission", AggregateID: uuid.NewString(),
		EventType: "taxi.goods.completed.v1", Payload: json.RawMessage(`{"provider_id":"p1"}`), OccurredAt: at}
	envelope := AnalyticsEnvelope(record)
	if err := envelope.Validate(); err != nil {
		t.Fatal(err)
	}
	if envelope.SourceService != "driver-taxi-service" || envelope.SourceVersion != at.UnixMicro() ||
		envelope.EventID != record.ID || envelope.SourceEntityType != "taxi_goods_mission" {
		t.Fatalf("envelope %+v", envelope)
	}
	if AnalyticsEnvelope(record).SourceVersion != envelope.SourceVersion {
		t.Fatal("a redelivery must carry the same version")
	}
}

type envelopeSink struct{ got []sharedevents.Envelope }

func (s *envelopeSink) Deliver(_ context.Context, envelope sharedevents.Envelope) error {
	s.got = append(s.got, envelope)
	return nil
}

func TestAnUnconfiguredAnalyticsIsAVisibleTerminalFailure(t *testing.T) {
	sink := &envelopeSink{}
	record := Record{ID: uuid.NewString(), AggregateID: uuid.NewString(), EventType: "taxi.recovery.started",
		Payload: json.RawMessage(`{}`), OccurredAt: time.Now()}
	var terminal *TerminalDeliveryError
	if err := (AnalyticsDestination{Deliverer: sink}).Deliver(context.Background(), record); !errors.As(err, &terminal) {
		t.Fatalf("got %v, want a terminal failure", err)
	}
	if err := (AnalyticsDestination{Deliverer: sink, Enabled: true}).Deliver(context.Background(), record); err != nil || len(sink.got) != 1 {
		t.Fatalf("enabled delivery: err=%v delivered=%d", err, len(sink.got))
	}
}

// PostgreSQL: the trigger copies a network fact once, and nothing else.
func TestTheFanoutTriggerCopiesOnce(t *testing.T) {
	pool := testDB(t)
	ctx := context.Background()
	booking := uuid.NewString()
	if err := Write(ctx, pool, "booking", booking, "taxi.recovery.started", []byte(`{"original_provider_id":"p1"}`)); err != nil {
		t.Fatal(err)
	}
	if err := Write(ctx, pool, "booking", booking, "taxi.booking.created", []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	var copies, others int
	if err := pool.QueryRow(ctx, `SELECT
		count(*) FILTER (WHERE event_type='taxi.recovery.started'),
		count(*) FILTER (WHERE event_type<>'taxi.recovery.started')
		FROM taxi_outbox WHERE aggregate_id=$1::uuid AND target='analytics-service'`, booking).Scan(&copies, &others); err != nil {
		t.Fatal(err)
	}
	if copies != 1 || others != 0 {
		t.Fatalf("analytics copies=%d others=%d, want 1/0", copies, others)
	}
}
