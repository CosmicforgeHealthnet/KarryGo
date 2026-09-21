package fulfilment_readiness

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Section 13: the readiness fact analytics measures branches by.

type signalRecorder struct{ signals []ReadySignal }

func (r *signalRecorder) RecordReady(_ context.Context, tx pgx.Tx, signal ReadySignal) error {
	if tx == nil {
		panic("a readiness signal must be recorded inside the readiness transaction")
	}
	r.signals = append(r.signals, signal)
	return nil
}

func serviceWithSignals(pool *pgxpool.Pool, now func() time.Time, sink ReadySignalSink) *Service {
	return NewService(Options{Repository: NewRepository(pool), Enabled: true, Now: now, Signals: sink})
}

func TestReadyOnTimeAndLateAreMeasuredAgainstThePromise(t *testing.T) {
	pool := readinessTestPool(t)
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		delay time.Duration
		late  bool
	}{{"on time", 0, false}, {"late", 6 * time.Hour, true}} {
		start := time.Now().UTC()
		clock := start
		sink := &signalRecorder{}
		service := serviceWithSignals(pool, func() time.Time { return clock }, sink)
		vendorOrderID, parentOrderID := seedOrderGraph(t, pool)
		preparing := startOrder(t, service, vendorOrderID, parentOrderID)
		if preparing.PredictedReadyEnd == nil && preparing.VendorEstimatedReadyAt == nil {
			t.Skipf("%s: no readiness promise is produced by the default rules", tc.name)
		}
		clock = start.Add(tc.delay)
		if _, err := service.Advance(ctx, AdvanceRequest{VendorOrderID: vendorOrderID,
			ExpectedVersion: preparing.Version, Move: Move{To: StateReady}, Actor: staffActor()}); err != nil {
			t.Fatalf("%s: ready: %v", tc.name, err)
		}
		if len(sink.signals) != 1 {
			t.Fatalf("%s: signals=%d, want 1", tc.name, len(sink.signals))
		}
		signal := sink.signals[0]
		if signal.OnTime == tc.late || signal.BranchID != "br-1" || signal.BusinessID != "biz-1" ||
			signal.VendorOrderID != vendorOrderID {
			t.Fatalf("%s: signal %+v", tc.name, signal)
		}
		if tc.late && signal.LatenessSeconds <= 0 {
			t.Fatalf("%s: a late order must carry how late it was", tc.name)
		}
		if tc.late && signal.PrepSeconds < int64(tc.delay.Seconds())-1 {
			t.Fatalf("%s: prep seconds %d", tc.name, signal.PrepSeconds)
		}
	}
}

// An order that bounces back and becomes ready again was only late once.
func TestAnOrderIsMeasuredOnlyOnItsFirstReady(t *testing.T) {
	pool := readinessTestPool(t)
	ctx := context.Background()
	sink := &signalRecorder{}
	now := time.Now().UTC()
	service := serviceWithSignals(pool, func() time.Time { return now }, sink)
	vendorOrderID, parentOrderID := seedOrderGraph(t, pool)
	current := startOrder(t, service, vendorOrderID, parentOrderID)
	if current.PredictedReadyEnd == nil && current.VendorEstimatedReadyAt == nil {
		t.Skip("no readiness promise is produced by the default rules")
	}
	for _, move := range []Move{{To: StateReady}, {To: StateDelayed, ReasonCode: "ITEM_MISSING"}, {To: StateReady}} {
		next, err := service.Advance(ctx, AdvanceRequest{VendorOrderID: vendorOrderID,
			ExpectedVersion: current.Version, Move: move, Actor: staffActor()})
		if err != nil {
			t.Fatalf("move to %s: %v", move.To, err)
		}
		current = next
	}
	if len(sink.signals) != 1 {
		t.Fatalf("signals=%d, want exactly one for the first ready", len(sink.signals))
	}
}

// With no sink, readiness behaves exactly as it always did.
func TestReadinessWithoutASinkIsUnchanged(t *testing.T) {
	pool := readinessTestPool(t)
	service := serviceWithSignals(pool, func() time.Time { return time.Now().UTC() }, nil)
	vendorOrderID, parentOrderID := seedOrderGraph(t, pool)
	preparing := startOrder(t, service, vendorOrderID, parentOrderID)
	if _, err := service.Advance(context.Background(), AdvanceRequest{VendorOrderID: vendorOrderID,
		ExpectedVersion: preparing.Version, Move: Move{To: StateReady}, Actor: staffActor()}); err != nil {
		t.Fatalf("ready without a sink: %v", err)
	}
}
