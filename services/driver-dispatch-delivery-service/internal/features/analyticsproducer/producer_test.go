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

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/adminresources"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/analyticsadmin"
	"cosmicforge/logistics/shared/go/networkanalytics"
)

func TestMigrationEmbedsSharedProducerSchemaVerbatim(t *testing.T) {
	raw, err := os.ReadFile("../../../migrations/000075_analytics_coverage_producer.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.Contains(text, analyticsadmin.ProducerSchema) || !strings.Contains(text, "-- shared-template-sha256: "+analyticsadmin.ProducerSchemaSHA256()) {
		t.Fatal("000075 drifted from shared/go/analyticsadmin/producer_schema.sql")
	}
}

type capture struct{ bodies [][]byte }

func (c *capture) PublishManifest(_ context.Context, raw []byte) error {
	c.bodies = append(c.bodies, raw)
	return nil
}

func TestPostgresDispatchProducerAttestsScopedTripsAndRiderRoster(t *testing.T) {
	db := fixture.Database(t, "../../../migrations", true)
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	rider, unverified, customer, org := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	exec(`INSERT INTO providers(id,phone,full_name) VALUES($1,'P1','N1'),($2,'P2','N2')`, rider, unverified)
	exec(`INSERT INTO provider_availability(provider_id,status,verified_to_go_online) VALUES($1,'busy',true),($2,'online',false)`, rider, unverified)
	var booking, trip string
	if err := db.QueryRow(ctx, `INSERT INTO dispatch_bookings(customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone) VALUES($1,'a',0,0,'b',0,0,100,'r','p') RETURNING id::text`, customer).Scan(&booking); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO trips(booking_id,provider_id,customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,fare_amount,receiver_name,receiver_phone,organization_id) VALUES($1,$2,$3,'a',0,0,'b',0,0,100,'r','p',$4) RETURNING id::text`, booking, rider, customer, org).Scan(&trip); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO dispatch_marketplace_outbox(event_type,aggregate_type,aggregate_id,aggregate_version,payload,payload_hash,target_service) VALUES('trip.completed.v1','analytics:trip',$1,9,$2::jsonb,'x','analytics-service'),('trip.completed.v1','analytics:dispatch_booking',$3,1,$2::jsonb,'x','analytics-service')`, trip, `{"provider_id":"`+rider+`"}`, booking)
	scope := adminpolicy.Scope{Product: "karryam", Market: "lagos", Zone: "island", Organization: org, Tenant: org}
	issuer := contextfixture.New(t, Owner)
	store := &admincontext.Store{DB: db, Reader: adminresources.Source(), Authority: issuer.Authority}
	for kind, ref := range map[string]string{"missions": trip, "riders": rider} {
		tx, _ := db.Begin(ctx)
		binding, err := adminresources.Source().Binding(ctx, tx, kind, ref)
		tx.Rollback(ctx)
		if err != nil {
			t.Fatal(kind, err)
		}
		if err := store.Import(ctx, issuer.Sign(t, issuer.Approval("ctx-"+ref, kind, ref, analyticsadmin.CoverageExportCapability, binding, scope)), time.Now()); err != nil {
			t.Fatal(kind, err)
		}
	}
	// A context for another organization than the trip's own is refused at
	// import, so the trip can never be attested into a foreign scope.
	foreign := scope
	foreign.Organization, foreign.Tenant = "other-org", "other-org"
	tx, _ := db.Begin(ctx)
	binding, _ := adminresources.Source().Binding(ctx, tx, "missions", trip)
	tx.Rollback(ctx)
	if err := store.Import(ctx, issuer.Sign(t, issuer.Approval("ctx-foreign", "missions", trip, analyticsadmin.CoverageExportCapability, binding, foreign)), time.Now()); err == nil {
		t.Fatal("foreign-organization coverage context accepted")
	}

	window := time.Now().UTC().Truncate(time.Hour)
	recorder := &analyticsadmin.RosterRecorder{SourceService: Owner, PolicyVersion: PolicyVersion, DB: db, Source: Roster{DB: db}, Outbox: Outbox{}}
	for bucket := window; bucket.Before(window.Add(time.Hour)); bucket = bucket.Add(networkanalytics.SupplyBucket) {
		at := bucket
		recorder.Now = func() time.Time { return at }
		if n, err := recorder.RunOnce(ctx); err != nil || n != 2 {
			t.Fatal(n, err)
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
		if v.Approval.Scope != scope {
			t.Fatal("manifest for unexpected scope", v.Approval.Scope)
		}
		found[v.Family] = v
	}
	events := found[analyticsadmin.FamilyNetworkEvents]
	hasCompleted := false
	for _, m := range events.Members {
		if m.EntityID != trip || m.EntityType != "trip" {
			t.Fatal("non-scoped fact attested", m)
		}
		hasCompleted = hasCompleted || m.EventName == "trip.completed.v1" && m.Version == 9
	}
	if !hasCompleted {
		t.Fatal("scoped trip fact missing", events.Members)
	}
	if roster := found[analyticsadmin.FamilySupplyRoster]; len(roster.Members) != 12 {
		t.Fatal("rider roster (scoped rider only)", len(roster.Members))
	}
}
