package analyticsproducer

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/services/taxi-service/internal/features/adminresources"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/services/taxi-service/internal/outbox"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/analyticsadmin"
	"cosmicforge/logistics/shared/go/networkanalytics"
)

func TestMigrationEmbedsSharedProducerSchemaVerbatim(t *testing.T) {
	raw, err := os.ReadFile("../../../migrations/028_analytics_coverage_producer.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.Contains(text, analyticsadmin.ProducerSchema) || !strings.Contains(text, "-- shared-template-sha256: "+analyticsadmin.ProducerSchemaSHA256()) {
		t.Fatal("028 drifted from shared/go/analyticsadmin/producer_schema.sql")
	}
}

type onlineSet []availabilityrepositories.ProviderStatus

func (o onlineSet) GetOnlineProviders(context.Context) ([]availabilityrepositories.ProviderStatus, error) {
	return o, nil
}

type capture struct{ bodies [][]byte }

func (c *capture) PublishManifest(_ context.Context, raw []byte) error {
	c.bodies = append(c.bodies, raw)
	return nil
}

func TestPostgresTaxiProducerUsesDeliveredIdentityScopedContextsAndRoster(t *testing.T) {
	db := fixture.Database(t, "../../../migrations")
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	market, zone, booking, corporate, provider, offline := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec(`INSERT INTO markets(id,country,name,code) VALUES($1,'TEST','N','M1')`, market)
	exec(`INSERT INTO operating_zones(id,market_id,name,code,center_lat,center_lng) VALUES($1,$2,'Z','Z1',10,20)`, zone, market)
	exec(`INSERT INTO taxi_bookings(id,customer_id,market_id,operating_zone_id,status) VALUES($1,$2,$3,$4,'completed'),($5,$6,$3,$4,'completed')`, booking, uuid.NewString(), market, zone, corporate, uuid.NewString())
	exec(`INSERT INTO taxi_providers(id,first_name) VALUES($1,'A'),($2,'B')`, provider, offline)
	exec(`INSERT INTO provider_operational_state(provider_id,state) VALUES($1,'ON_TRIP'),($2,'OFFLINE')`, provider, offline)
	for _, id := range []string{booking, corporate} {
		if err := outbox.Write(ctx, db, "booking", id, "taxi.booking.completed", []byte(`{"provider_id":"`+provider+`"}`)); err != nil {
			t.Fatal(err)
		}
	}
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "o1", Tenant: "t1"}
	issuer := contextfixture.New(t, Owner)
	store := &admincontext.Store{DB: db, Reader: adminresources.Source(), Authority: issuer.Authority}
	allocate := func(kind, ref, capability string) {
		tx, _ := db.Begin(ctx)
		binding, err := adminresources.Source().Binding(ctx, tx, kind, ref)
		tx.Rollback(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Import(ctx, issuer.Sign(t, issuer.Approval("ctx-"+ref, kind, ref, capability, binding, scope)), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	allocate("independent-rides", booking, analyticsadmin.CoverageExportCapability)
	allocate("providers", provider, analyticsadmin.CoverageExportCapability)

	now := time.Now().UTC()
	window := now.Truncate(time.Hour)
	recorder := &analyticsadmin.RosterRecorder{SourceService: Owner, PolicyVersion: PolicyVersion, DB: db, Source: Roster{DB: db, Online: onlineSet{{ProviderID: provider}, {ProviderID: offline}}}, Outbox: Outbox{}}
	for bucket := window; bucket.Before(window.Add(time.Hour)); bucket = bucket.Add(networkanalytics.SupplyBucket) {
		at := bucket
		recorder.Now = func() time.Time { return at }
		if n, err := recorder.RunOnce(ctx); err != nil || n != 1 {
			t.Fatal("OFFLINE provider must not be supply", n, err)
		}
	}
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	authority := admincontext.Authority{Owner: Owner, Environment: adminpolicy.TestOnly, Keys: map[string]ed25519.PublicKey{"k": pub}}
	publisher := &capture{}
	p := &analyticsadmin.Producer{Owner: Owner, Environment: adminpolicy.TestOnly, DB: db, Source: Facts{DB: db}, Resolver: &analyticsadmin.ContextScopeResolver{DB: db, Store: store}, AnchorKinds: AnchorKinds(), Signer: analyticsadmin.Signer{KeyID: "k", Key: key}, Publisher: publisher,
		Period: time.Hour, Lookback: time.Hour, TTL: 72 * time.Hour, ProvenanceRef: "TEST_ONLY", ApprovalRef: "TEST_ONLY", RosterEnabled: true, Now: func() time.Time { return window.Add(time.Hour) }}
	if _, err := p.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	found := map[string]analyticsadmin.VerifiedManifest{}
	for _, body := range publisher.bodies {
		v, err := analyticsadmin.VerifyManifest(authority, body, window.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		found[v.Family] = v
	}
	events := found[analyticsadmin.FamilyNetworkEvents]
	if len(events.Members) != 1 || events.Members[0].EntityID != booking {
		t.Fatal("only the scoped independent ride is attested", events.Members)
	}
	// The member identity is exactly what the delivery path sends.
	var record outbox.Record
	if err := db.QueryRow(ctx, `SELECT id::text,aggregate_type,aggregate_id::text,event_type,payload,occurred_at FROM taxi_outbox WHERE target='analytics-service' AND aggregate_id=$1`, booking).Scan(&record.ID, &record.AggregateType, &record.AggregateID, &record.EventType, &record.Payload, &record.OccurredAt); err != nil {
		t.Fatal(err)
	}
	envelope := outbox.AnalyticsEnvelope(record)
	if events.Members[0].EventID != envelope.EventID || events.Members[0].Version != envelope.SourceVersion {
		t.Fatal("manifest identity diverges from delivered envelope")
	}
	roster := found[analyticsadmin.FamilySupplyRoster]
	if len(roster.Members) != 12 {
		t.Fatal("roster members", len(roster.Members))
	}
}
