package analyticsproducer

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/services/hauling-service/internal/features/adminresources"
	availabilityrepositories "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/repositories"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/analyticsadmin"
	"cosmicforge/logistics/shared/go/networkanalytics"
)

func TestMigrationEmbedsSharedProducerSchemaVerbatim(t *testing.T) {
	raw, err := os.ReadFile("../../../migrations/031_analytics_coverage_producer.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.Contains(text, analyticsadmin.ProducerSchema) || !strings.Contains(text, "-- shared-template-sha256: "+analyticsadmin.ProducerSchemaSHA256()) {
		t.Fatal("031 drifted from shared/go/analyticsadmin/producer_schema.sql")
	}
}

type onlineSet []availabilityrepositories.ProviderStatus

func (o onlineSet) GetOnlineProviders(context.Context) ([]availabilityrepositories.ProviderStatus, error) {
	return o, nil
}

type capture struct {
	mu     sync.Mutex
	bodies [][]byte
}

func (c *capture) PublishManifest(_ context.Context, raw []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bodies = append(c.bodies, raw)
	return nil
}

func TestPostgresHaulingProducerAttestsScopedFactsAndIndividualRoster(t *testing.T) {
	db := fixture.Database(t, "../../../migrations")
	ctx := context.Background()
	scope := adminpolicy.Scope{Product: "karryam", Market: "lagos", Zone: "mainland", Organization: "org-1", Tenant: "org-1"}
	providerA, providerB := uuid.NewString(), uuid.NewString()
	truckA := uuid.NewString()
	scoped, unscoped := uuid.NewString(), uuid.NewString()
	for _, stmt := range []string{
		`INSERT INTO truck_providers(id,first_name) VALUES('` + providerA + `','A'),('` + providerB + `','B')`,
		`INSERT INTO trucks(id,provider_id,truck_type,capacity_kg,plate_number,status) VALUES('` + truckA + `','` + providerA + `','flatbed',1000,'P-` + truckA[:8] + `','active')`,
		`INSERT INTO haulage_bookings(id,customer_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,cargo_type,cargo_weight_kg) VALUES('` + scoped + `',gen_random_uuid(),'x',0,0,'y',0,0,'general',50),('` + unscoped + `',gen_random_uuid(),'x',0,0,'y',0,0,'general',50)`,
		`UPDATE haulage_bookings SET provider_id='` + providerA + `',status='accepted' WHERE id='` + scoped + `'`,
	} {
		if _, err := db.Exec(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	// Owner contract: lifecycle facts now name the provider that held the job.
	var payload []byte
	if err := db.QueryRow(ctx, `SELECT payload FROM hauling_domain_outbox WHERE aggregate_id=$1 AND event_type='hauling.booking.assigned.v1'`, scoped).Scan(&payload); err != nil || !strings.Contains(string(payload), providerA) {
		t.Fatal("assigned fact does not carry the provider", string(payload), err)
	}
	issuer := contextfixture.New(t, Owner)
	store := &admincontext.Store{DB: db, Reader: adminresources.Source(), Authority: issuer.Authority}
	allocate := func(kind, ref string) {
		tx, _ := db.Begin(ctx)
		binding, err := adminresources.Source().Binding(ctx, tx, kind, ref)
		tx.Rollback(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Import(ctx, issuer.Sign(t, issuer.Approval("ctx-"+ref, kind, ref, analyticsadmin.CoverageExportCapability, binding, scope)), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	allocate("missions", scoped)
	allocate("providers", providerA)
	// A context WITHOUT the export capability does not place a fact in scope.
	tx, _ := db.Begin(ctx)
	binding, _ := adminresources.Source().Binding(ctx, tx, "missions", unscoped)
	tx.Rollback(ctx)
	if err := store.Import(ctx, issuer.Sign(t, issuer.Approval("ctx-"+unscoped, "missions", unscoped, "hauling.missions.read", binding, scope)), time.Now()); err != nil {
		t.Fatal(err)
	}

	// Roster: every bucket of the current hour observed; provider B is online
	// without a truck (UNKNOWN eligibility) and has no export context.
	now := time.Now().UTC()
	window := now.Truncate(time.Hour)
	recorder := &analyticsadmin.RosterRecorder{SourceService: Owner, PolicyVersion: PolicyVersion, DB: db, Source: Roster{DB: db, Online: onlineSet{{ProviderID: providerA, TruckID: truckA}, {ProviderID: providerB}}}, Outbox: Outbox{}}
	for bucket := window; bucket.Before(window.Add(time.Hour)); bucket = bucket.Add(networkanalytics.SupplyBucket) {
		at := bucket
		recorder.Now = func() time.Time { return at }
		if _, err := recorder.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if n, _ := recorder.RunOnce(ctx); n != 0 {
		t.Fatal("bucket recorded twice")
	}
	var rows int
	db.QueryRow(ctx, `SELECT count(*) FROM hauling_domain_outbox WHERE event_type=$1`, networkanalytics.EventSupplyRoster).Scan(&rows)
	if rows != 24 {
		t.Fatal("roster outbox rows", rows)
	}

	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	authority := admincontext.Authority{Owner: Owner, Environment: adminpolicy.TestOnly, Keys: map[string]ed25519.PublicKey{"k": pub}}
	publisher := &capture{}
	p := &analyticsadmin.Producer{Owner: Owner, Environment: adminpolicy.TestOnly, DB: db, Source: Facts{DB: db}, Resolver: &analyticsadmin.ContextScopeResolver{DB: db, Store: store}, AnchorKinds: AnchorKinds(), Signer: analyticsadmin.Signer{KeyID: "k", Key: key}, Publisher: publisher,
		Period: time.Hour, Lookback: 2 * time.Hour, Settle: 0, TTL: 72 * time.Hour, ProvenanceRef: "TEST_ONLY", ApprovalRef: "TEST_ONLY", RosterEnabled: true, Now: func() time.Time { return window.Add(time.Hour) }}
	result, err := p.RunOnce(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Unscoped == 0 {
		t.Fatal("unscoped facts must be excluded, not attested", result)
	}
	found := map[string]analyticsadmin.VerifiedManifest{}
	for _, body := range publisher.bodies {
		v, err := analyticsadmin.VerifyManifest(authority, body, window.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if v.Start.Equal(window) {
			found[v.Family] = v
		}
	}
	events, roster := found[analyticsadmin.FamilyNetworkEvents], found[analyticsadmin.FamilySupplyRoster]
	if events.Approval.Scope != scope || len(events.Members) != 2 {
		t.Fatal("network-events manifest must hold exactly the scoped booking's committed+assigned facts", events.Members)
	}
	for _, m := range events.Members {
		if m.EntityID != scoped {
			t.Fatal("foreign fact attested", m)
		}
	}
	if len(roster.Members) != 12 {
		t.Fatal("roster manifest must hold provider A once per observed bucket", len(roster.Members))
	}
	for _, m := range roster.Members {
		if m.EntityID != providerA || m.EventName != networkanalytics.EventSupplyRoster {
			t.Fatal("non-member attested", m)
		}
	}
	var eligible *bool
	db.QueryRow(ctx, `SELECT eligible FROM analytics_supply_roster_members WHERE provider_id=$1 LIMIT 1`, providerB).Scan(&eligible)
	if eligible != nil {
		t.Fatal("unknown eligibility recorded as a value")
	}
	// The outbox rows rebuild exactly the identity the delivery worker sends.
	var stamped struct {
		EventID string `json:"event_id"`
	}
	db.QueryRow(ctx, `SELECT payload FROM hauling_domain_outbox WHERE event_type=$1 AND aggregate_id=$2 LIMIT 1`, networkanalytics.EventSupplyRoster, providerA).Scan(&payload)
	json.Unmarshal(payload, &stamped)
	ok := false
	for _, m := range roster.Members {
		ok = ok || m.EventID == stamped.EventID
	}
	if !ok {
		t.Fatal("roster event identity not preserved")
	}
}
