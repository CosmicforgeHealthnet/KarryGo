package integration_test

// FLEET-20.7: cold-chain temperature readings.
//
// Two defects are pinned here: replay was keyed on (organization, sensor,
// source version) without the record, so the same sensor reading sent
// against a second record returned the first record's event; and the
// excursion thresholds defaulted to 0 when the requirement record lacked a
// usable value.

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/specialized"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

type coldChainFixture struct {
	pool  *pgxpool.Pool
	s     *specialized.Service
	scope tenant.Context
}

func newColdChainFixture(t *testing.T, pool *pgxpool.Pool) *coldChainFixture {
	t.Helper()
	org := createOrganization(t, pool, "Cold Chain "+ids.New("x"), "CC-"+ids.New("reg"))
	actor := ids.New("usr")
	scope := tenant.Context{OrganizationID: org.ID, OrganizationUserID: actor, ActorType: tenant.ActorTypeOrganizationUser, ActorID: actor, CorrelationID: "cc"}
	s := specialized.NewService(specialized.Options{Pool: pool, Events: events.NewDomainPublisher(events.NewOutboxRepository(pool)),
		Flags: specialized.Flags{Enabled: true, Modules: map[string]bool{specialized.ColdChain: true}}})
	if _, err := s.SetCapability(context.Background(), scope, specialized.ColdChain, "ENABLED", "cold chain rollout", 0, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("enable cold chain: %v", err)
	}
	return &coldChainFixture{pool: pool, s: s, scope: scope}
}

func (f *coldChainFixture) requirement(t *testing.T, payload string) specialized.Record {
	t.Helper()
	record, err := f.s.CreateRecord(context.Background(), f.scope, specialized.Record{
		CapabilityCode: specialized.ColdChain, RecordType: "TEMPERATURE_REQUIREMENT", SubjectType: "SHIPMENT",
		SubjectReference: "shp-" + ids.New("x"), Payload: json.RawMessage(payload), IdempotencyKey: "cc-" + ids.New("k")})
	if err != nil {
		t.Fatalf("create requirement: %v", err)
	}
	return record
}

func (f *coldChainFixture) status(t *testing.T, recordID string) string {
	t.Helper()
	var status string
	if err := f.pool.QueryRow(context.Background(), `SELECT status FROM fleet_partner_specialized_records WHERE organization_id=$1 AND id=$2`, f.scope.OrganizationID, recordID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func reading(sensor string, version int64, value float64) specialized.TemperatureEvent {
	return specialized.TemperatureEvent{SensorReference: sensor, Temperature: value, Unit: "C",
		SourceVersion: version, SourceTime: time.Date(2026, 11, 1, 8, 0, int(version), 0, time.UTC)}
}

func TestColdChainReplayIsPerRecord(t *testing.T) {
	f := newColdChainFixture(t, testPool(t))
	ctx := context.Background()
	payload := `{"minimumTemperature":2,"maximumTemperature":8,"unit":"C"}`
	first, second := f.requirement(t, payload), f.requirement(t, payload)
	sensor := "sensor-" + ids.New("x")

	a, err := f.s.AddTemperature(ctx, f.scope, first.ID, reading(sensor, 7, 5))
	if err != nil {
		t.Fatalf("first record reading: %v", err)
	}
	// The same sensor and source version against another record is a new
	// reading for that record, not the first record's event.
	b, err := f.s.AddTemperature(ctx, f.scope, second.ID, reading(sensor, 7, 5))
	if err != nil {
		t.Fatalf("second record reading: %v", err)
	}
	if b.ID == a.ID || b.RecordID != second.ID {
		t.Fatalf("second record got event %s for record %s; want its own", b.ID, b.RecordID)
	}
	// A true replay returns the stored event.
	replay, err := f.s.AddTemperature(ctx, f.scope, first.ID, reading(sensor, 7, 5))
	if err != nil || replay.ID != a.ID {
		t.Fatalf("replay = %s err %v, want %s", replay.ID, err, a.ID)
	}
	// The same identity with a different value is a conflict, not a replay.
	if _, err := f.s.AddTemperature(ctx, f.scope, first.ID, reading(sensor, 7, 6)); appStatus(err) != http.StatusConflict {
		t.Fatalf("different value for one source version: %v, want 409", err)
	}
	var stored int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_temperature_events WHERE organization_id=$1 AND sensor_reference=$2`, f.scope.OrganizationID, sensor).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 2 {
		t.Fatalf("stored readings = %d, want one per record", stored)
	}
	var recorded int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_outbox WHERE organization_id=$1 AND event_type='cold_chain.temperature_recorded.v1'`, f.scope.OrganizationID).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	if recorded != 2 {
		t.Fatalf("temperature_recorded events = %d, want 2 (replays add none)", recorded)
	}
}

func TestColdChainExcursionUsesStatedThresholds(t *testing.T) {
	f := newColdChainFixture(t, testPool(t))
	ctx := context.Background()

	// A frozen-goods requirement: -25..-18. Under the old zero default a
	// -20 reading against a record whose bounds were unreadable was an
	// excursion; against these stated bounds it is in range.
	frozen := f.requirement(t, `{"minimumTemperature":-25,"maximumTemperature":-18,"unit":"C"}`)
	in, err := f.s.AddTemperature(ctx, f.scope, frozen.ID, reading("frz-"+ids.New("x"), 1, -20))
	if err != nil || in.Excursion {
		t.Fatalf("in-range frozen reading: excursion=%v err=%v", in.Excursion, err)
	}
	if f.status(t, frozen.ID) == "QUARANTINED" {
		t.Fatal("an in-range reading must not quarantine")
	}
	out, err := f.s.AddTemperature(ctx, f.scope, frozen.ID, reading("frz-"+ids.New("x"), 2, -10))
	if err != nil || !out.Excursion || f.status(t, frozen.ID) != "QUARANTINED" {
		t.Fatalf("out-of-range reading: excursion=%v status=%s err=%v", out.Excursion, f.status(t, frozen.ID), err)
	}

	// A legacy (schema v1) record may hold a numeric string bound; it is
	// read as its number. The current schema refuses strings at creation,
	// so the legacy shape is written directly.
	stringy := f.requirement(t, `{"minimumTemperature":2,"maximumTemperature":8,"unit":"C"}`)
	if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_specialized_records SET payload=$3 WHERE organization_id=$1 AND id=$2`,
		f.scope.OrganizationID, stringy.ID, `{"minimumTemperature":"2","maximumTemperature":"8","unit":"C"}`); err != nil {
		t.Fatal(err)
	}
	if ev, err := f.s.AddTemperature(ctx, f.scope, stringy.ID, reading("str-"+ids.New("x"), 1, 5)); err != nil || ev.Excursion {
		t.Fatalf("numeric-string bounds: excursion=%v err=%v", ev.Excursion, err)
	}

	// Records whose bounds are not usable refuse readings instead of
	// defaulting to 0. They are written past the schema directly, as a
	// legacy record could have been.
	for name, payload := range map[string]string{
		"missing maximum": `{"minimumTemperature":2,"unit":"C"}`,
		"non-numeric":     `{"minimumTemperature":"cold","maximumTemperature":8,"unit":"C"}`,
		"inverted":        `{"minimumTemperature":9,"maximumTemperature":8,"unit":"C"}`,
	} {
		record := f.requirement(t, `{"minimumTemperature":2,"maximumTemperature":8,"unit":"C"}`)
		if _, err := f.pool.Exec(ctx, `UPDATE fleet_partner_specialized_records SET payload=$3 WHERE organization_id=$1 AND id=$2`, f.scope.OrganizationID, record.ID, payload); err != nil {
			t.Fatal(err)
		}
		sensor := "bad-" + ids.New("x")
		if _, err := f.s.AddTemperature(ctx, f.scope, record.ID, reading(sensor, 1, 5)); appStatus(err) != http.StatusUnprocessableEntity {
			t.Fatalf("%s: err %v (status %d), want 422", name, err, appStatus(err))
		}
		var n int
		if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_temperature_events WHERE organization_id=$1 AND record_id=$2`, f.scope.OrganizationID, record.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 || f.status(t, record.ID) == "QUARANTINED" {
			t.Fatalf("%s: a refused reading must store nothing and quarantine nothing", name)
		}
	}
}

func TestColdChainConcurrentDuplicateReadingsStoreOnce(t *testing.T) {
	f := newColdChainFixture(t, testPool(t))
	record := f.requirement(t, `{"minimumTemperature":2,"maximumTemperature":8,"unit":"C"}`)
	sensor := "dup-" + ids.New("x")
	var wg sync.WaitGroup
	results := make(chan specialized.TemperatureEvent, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ev, err := f.s.AddTemperature(context.Background(), f.scope, record.ID, reading(sensor, 3, 12))
			if err != nil {
				t.Errorf("concurrent reading: %v", err)
				return
			}
			results <- ev
		}()
	}
	wg.Wait()
	close(results)
	id := ""
	for ev := range results {
		if id == "" {
			id = ev.ID
		}
		if ev.ID != id {
			t.Fatalf("concurrent duplicates returned different events %s and %s", id, ev.ID)
		}
	}
	var stored, excursions int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM fleet_partner_temperature_events WHERE organization_id=$1 AND record_id=$2`, f.scope.OrganizationID, record.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM fleet_partner_outbox WHERE organization_id=$1 AND event_type='cold_chain.quarantined.v1' AND aggregate_id=$2`, f.scope.OrganizationID, record.ID).Scan(&excursions); err != nil {
		t.Fatal(err)
	}
	if stored != 1 || excursions != 1 {
		t.Fatalf("stored=%d quarantine events=%d, want 1/1", stored, excursions)
	}
}

func TestColdChainReadingCannotTargetAnotherTenantsRecord(t *testing.T) {
	pool := testPool(t)
	owner := newColdChainFixture(t, pool)
	record := owner.requirement(t, `{"minimumTemperature":2,"maximumTemperature":8,"unit":"C"}`)
	other := newColdChainFixture(t, pool)
	if _, err := other.s.AddTemperature(context.Background(), other.scope, record.ID, reading("x-"+ids.New("x"), 1, 5)); appStatus(err) != http.StatusNotFound {
		t.Fatalf("cross-tenant reading: %v, want 404", err)
	}
}
