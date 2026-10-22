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
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/database"
	"cosmicforge/logistics/services/vendor-service/internal/events"
	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/analyticsadmin"
	"cosmicforge/logistics/shared/go/networkanalytics"
)

func TestMigrationEmbedsSharedProducerSchemaVerbatim(t *testing.T) {
	raw, err := os.ReadFile("../../../migrations/000132_analytics_coverage_producer.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.Contains(text, analyticsadmin.ProducerSchema) || !strings.Contains(text, "-- shared-template-sha256: "+analyticsadmin.ProducerSchemaSHA256()) {
		t.Fatal("000132 drifted from shared/go/analyticsadmin/producer_schema.sql")
	}
}

type capture struct{ bodies [][]byte }

func (c *capture) PublishManifest(_ context.Context, raw []byte) error {
	c.bodies = append(c.bodies, raw)
	return nil
}

func TestPostgresVendorGlobalProducerAttestsDeliveredReadinessEnvelopes(t *testing.T) {
	dsn := os.Getenv("VENDOR_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("VENDOR_SERVICE_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.ApplyMigrations(ctx, db, "../../../migrations"); err != nil {
		t.Fatal(err)
	}
	outbox := events.NewOutboxRepository(db)
	window := time.Now().UTC().Truncate(time.Hour)
	ids := map[string]bool{}
	for i := 0; i < 3; i++ {
		envelope, err := events.NewEnvelope(networkanalytics.EventBranchReadiness, 1, "vendor_order", uuid.NewString(), 1, uuid.NewString(), "", "", map[string]any{"branch_id": uuid.NewString(), "on_time": true, "lateness_seconds": 0, "prep_seconds": 30}, false)
		if err != nil {
			t.Fatal(err)
		}
		if err := outbox.Append(ctx, db, envelope); err != nil {
			t.Fatal(err)
		}
		ids[envelope.EventID] = true
	}
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	authority := admincontext.Authority{Owner: Owner, Environment: adminpolicy.TestOnly, Keys: map[string]ed25519.PublicKey{"k": pub}}
	publisher := &capture{}
	p := &analyticsadmin.Producer{Owner: Owner, Environment: adminpolicy.TestOnly, DB: db, Source: Facts{DB: db}, Global: true, Signer: analyticsadmin.Signer{KeyID: "k", Key: key}, Publisher: publisher,
		Period: time.Hour, Lookback: time.Hour, TTL: 72 * time.Hour, ProvenanceRef: "TEST_ONLY", ApprovalRef: "TEST_ONLY", Now: func() time.Time { return window.Add(time.Hour) }}
	if _, err := p.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if len(publisher.bodies) != 1 {
		t.Fatal("one global manifest per window", len(publisher.bodies))
	}
	v, err := analyticsadmin.VerifyManifest(authority, publisher.bodies[0], window.Add(time.Hour))
	if err != nil || v.Family != analyticsadmin.FamilyNetworkEventsGlobal || v.Approval.Scope != analyticsadmin.GlobalScope {
		t.Fatal(v, err)
	}
	attested := 0
	for _, m := range v.Members {
		if ids[m.EventID] {
			attested++
			if m.EntityType != "vendor_order" || m.EventName != networkanalytics.EventBranchReadiness {
				t.Fatal("identity diverges from delivered envelope", m)
			}
		}
	}
	if attested != 3 {
		t.Fatal("readiness facts missing from global manifest", attested)
	}
}
