package analyticsadmin

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminpolicy"
	sharedevents "cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/networkanalytics"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var testScope = adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "O1", Tenant: "T1"}

func testSigner(t *testing.T, owner string) (Signer, admincontext.Authority) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return Signer{KeyID: "test-only", Key: key}, admincontext.Authority{Owner: owner, Environment: adminpolicy.TestOnly, Keys: map[string]ed25519.PublicKey{"test-only": pub}}
}

func testMembers(start time.Time, n int) []ManifestMember {
	out := []ManifestMember{}
	for i := 0; i < n; i++ {
		out = append(out, ManifestMember{EventID: uuid.NewString(), EventName: "taxi.booking.completed", EntityType: "booking", EntityID: uuid.NewString(), Version: 1, OccurredAt: start.Add(time.Duration(i+1) * time.Minute)})
	}
	return out
}

func signedManifest(t *testing.T, signer Signer, owner, family string, scope adminpolicy.Scope, start, end time.Time, revision int64, members []ManifestMember) []byte {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	a, err := ManifestApproval{Owner: owner, Environment: adminpolicy.TestOnly, Family: family, Scope: scope, Start: start, End: end, Revision: revision, ProvenanceRef: "TEST_ONLY", ApprovalRef: "TEST_ONLY", EffectiveAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)}.Document(members)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := signer.SignManifest(a, members)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestManifestDigestBindsEveryMemberAndRejectsAggregates(t *testing.T) {
	start := time.Now().UTC().Truncate(time.Hour).Add(-2 * time.Hour)
	end := start.Add(time.Hour)
	members := testMembers(start, 3)
	base, err := ManifestDigest("driver-taxi-service", FamilyNetworkEvents, testScope, start, end, members)
	if err != nil {
		t.Fatal(err)
	}
	reordered := []ManifestMember{members[2], members[0], members[1]}
	if d, _ := ManifestDigest("driver-taxi-service", FamilyNetworkEvents, testScope, start, end, reordered); d != base {
		t.Fatal("digest depends on member order")
	}
	for name, mutate := range map[string]func([]ManifestMember){
		"version": func(m []ManifestMember) { m[0].Version = 2 },
		"time":    func(m []ManifestMember) { m[0].OccurredAt = m[0].OccurredAt.Add(time.Microsecond) },
		"entity":  func(m []ManifestMember) { m[0].EntityID = "other" },
	} {
		copyMembers := append([]ManifestMember{}, members...)
		mutate(copyMembers)
		if d, _ := ManifestDigest("driver-taxi-service", FamilyNetworkEvents, testScope, start, end, copyMembers); d == base {
			t.Fatal(name, "not bound by digest")
		}
	}
	if d, _ := ManifestDigest("driver-taxi-service", FamilyNetworkEvents, adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "O1", Tenant: "OTHER"}, start, end, members); d == base {
		t.Fatal("scope not bound")
	}
	if d, _ := ManifestDigest("driver-taxi-service", FamilyNetworkEvents, testScope, start, end, members[:2]); d == base {
		t.Fatal("membership count not bound")
	}
	bad := map[string][]ManifestMember{
		"duplicate":      {members[0], members[0]},
		"outside window": {{EventID: "x", EventName: "taxi.booking.completed", EntityType: "booking", EntityID: "e", Version: 1, OccurredAt: end}},
		"aggregate":      {{EventID: "x", EventName: networkanalytics.EventSupplySnapshot, EntityType: "network_supply", EntityID: "e", Version: 1, OccurredAt: start}},
		"wrong family":   {{EventID: "x", EventName: networkanalytics.EventSupplyRoster, EntityType: networkanalytics.SupplyRosterEntityType, EntityID: "e", Version: 1, OccurredAt: start}},
	}
	for name, list := range bad {
		if _, err := ManifestDigest("driver-taxi-service", FamilyNetworkEvents, testScope, start, end, list); err == nil {
			t.Fatal(name, "accepted")
		}
	}
	if _, err := (ManifestApproval{Owner: "o", Family: FamilyNetworkEventsGlobal, Scope: testScope, Start: start, End: end}).Document(nil); err == nil {
		t.Fatal("global family with a real scope")
	}
}

func TestVerifyManifestRejectsForgeryTamperAndIdentitySwap(t *testing.T) {
	owner := "driver-hauling-service"
	signer, authority := testSigner(t, owner)
	start := time.Now().UTC().Truncate(time.Hour).Add(-2 * time.Hour)
	end := start.Add(time.Hour)
	members := testMembers(start, 4)
	raw := signedManifest(t, signer, owner, FamilyNetworkEvents, testScope, start, end, 1, members)
	verified, err := VerifyManifest(authority, raw, time.Now())
	if err != nil || len(verified.Members) != 4 || verified.Family != FamilyNetworkEvents {
		t.Fatal(err)
	}
	other, _ := testSigner(t, owner)
	if _, err := VerifyManifest(authority, signedManifest(t, other, owner, FamilyNetworkEvents, testScope, start, end, 1, members), time.Now()); err == nil {
		t.Fatal("forged signature accepted")
	}
	var wire Manifest
	json.Unmarshal(raw, &wire)
	wire.Members = append(wire.Members, testMembers(start, 1)...)
	tampered, _ := json.Marshal(wire)
	if _, err := VerifyManifest(authority, tampered, time.Now()); err == nil {
		t.Fatal("extra member accepted")
	}
	if _, err := VerifyManifest(authority, raw, time.Now().Add(2*time.Hour)); err == nil {
		t.Fatal("expired manifest accepted")
	}
	// Identity swap: context id for one window/scope reused for another.
	now := time.Now().UTC().Truncate(time.Second)
	a, _ := ManifestApproval{Owner: owner, Environment: adminpolicy.TestOnly, Family: FamilyNetworkEvents, Scope: testScope, Start: start, End: end, Revision: 1, ProvenanceRef: "TEST_ONLY", ApprovalRef: "TEST_ONLY", EffectiveAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)}.Document(members)
	a.ID = ManifestContextID(owner, FamilyNetworkEvents, testScope, start.Add(-time.Hour), start)
	swapped, _ := signer.SignManifest(a, members)
	if _, err := VerifyManifest(authority, swapped, time.Now()); err == nil {
		t.Fatal("context id swap accepted")
	}
	a.ID = ManifestContextID(owner, FamilyNetworkEvents, testScope, start, end)
	a.Capabilities = []string{"analytics.cohorts.mission-success.export"}
	wrongCap, _ := signer.SignManifest(a, members)
	if _, err := VerifyManifest(authority, wrongCap, time.Now()); err == nil {
		t.Fatal("non-attestation capability accepted")
	}
	if _, _, err := DecodeManifest(append(raw[:len(raw)-1], []byte(`,"members":[]}`)...)); err == nil {
		t.Fatal("duplicate members key accepted")
	}
}

func TestPublishManifestFixedPathAndOutcomes(t *testing.T) {
	owner := "driver-dispatch-delivery-service"
	signer, _ := testSigner(t, owner)
	start := time.Now().UTC().Truncate(time.Hour).Add(-2 * time.Hour)
	raw := signedManifest(t, signer, owner, FamilyNetworkEvents, testScope, start, start.Add(time.Hour), 1, testMembers(start, 2))
	key := bytes.Repeat([]byte{91}, 32)
	for _, tc := range []struct {
		status int
		body   string
		want   error
	}{{200, `{"status":"RECORDED"}`, nil}, {409, "", ErrStaleManifest}, {403, "", ErrDenied}, {503, "", ErrUnavailable}, {200, `{"status":"RECORDED","x":1}`, ErrInvalid}} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != CoverageManifestPath+owner {
				t.Error("path", r.URL.Path)
			}
			if _, err := serviceauth.NewVerifier(serviceauth.Secrets{ProducerPublisherCaller: key}, time.Minute).VerifyRequest(r); err != nil {
				t.Error(err)
			}
			w.WriteHeader(tc.status)
			w.Write([]byte(tc.body))
		}))
		client, _ := NewPublisherClient(server.URL, owner, key)
		if err := client.PublishManifest(context.Background(), raw); !errors.Is(err, tc.want) || tc.want == nil && err != nil {
			t.Fatal(tc.status, err)
		}
		client.Close()
		server.Close()
	}
	client, _ := NewPublisherClient("http://127.0.0.1:1", "driver-taxi-service", key)
	if err := client.PublishManifest(context.Background(), raw); !errors.Is(err, ErrInvalid) {
		t.Fatal("cross-owner manifest sent", err)
	}
}

// --- producer with a real PostgreSQL ledger -------------------------------

func producerDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ANALYTICS_ADMIN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ANALYTICS_ADMIN_TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	root, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "producer_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); root.Close() })
	cfg, _ := pgxpool.ParseConfig(dsn)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	for i := 0; i < 2; i++ {
		if _, err := pool.Exec(ctx, ProducerSchema); err != nil {
			t.Fatal(err)
		}
	}
	return pool
}

type staticFacts []ProducerFact

func (s staticFacts) Facts(_ context.Context, start, end time.Time) ([]ProducerFact, error) {
	out := []ProducerFact{}
	for _, f := range s {
		if !f.Member.OccurredAt.Before(start) && f.Member.OccurredAt.Before(end) {
			out = append(out, f)
		}
	}
	return out, nil
}

type staticResolver map[string][]adminpolicy.Scope

func (r staticResolver) AnchorScopes(_ context.Context, _ string, ref string, _ time.Time) ([]adminpolicy.Scope, error) {
	return r[ref], nil
}
func (r staticResolver) KnownScopes(context.Context, []string) ([]adminpolicy.Scope, error) {
	return []adminpolicy.Scope{testScope}, nil
}

type capturePublisher struct {
	mu     sync.Mutex
	bodies [][]byte
	fail   error
}

func (c *capturePublisher) PublishManifest(_ context.Context, raw []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bodies = append(c.bodies, raw)
	return c.fail
}

func TestPostgresProducerAttestsScopedMembersIdempotentlyAndRetriesUnknownOutcome(t *testing.T) {
	db := producerDB(t)
	ctx := context.Background()
	owner := "driver-hauling-service"
	signer, authority := testSigner(t, owner)
	now := time.Now().UTC().Truncate(time.Hour).Add(30 * time.Minute)
	window := now.Truncate(time.Hour).Add(-time.Hour)
	members := testMembers(window, 3)
	facts := staticFacts{}
	for i, m := range members {
		ref := "anchored"
		if i == 2 {
			ref = "unscoped"
		}
		facts = append(facts, ProducerFact{Member: m, Family: FamilyNetworkEvents, AnchorKind: "missions", AnchorRef: ref})
	}
	publisher := &capturePublisher{fail: ErrUnavailable}
	p := &Producer{Owner: owner, Environment: adminpolicy.TestOnly, DB: db, Source: facts, Resolver: staticResolver{"anchored": {testScope}}, AnchorKinds: []string{"missions"}, Signer: signer, Publisher: publisher, Period: time.Hour, Lookback: 2 * time.Hour, Settle: 10 * time.Minute, TTL: 48 * time.Hour, ProvenanceRef: "TEST_ONLY", ApprovalRef: "TEST_ONLY", Now: func() time.Time { return now }}
	if _, err := p.RunOnce(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatal("unknown outcome swallowed", err)
	}
	publisher.fail = nil
	result, err := p.RunOnce(ctx)
	if err != nil || result.Unscoped != 1 {
		t.Fatal(result, err)
	}
	// The retry resent the identical signed body (same revision/checksum).
	if !bytes.Equal(publisher.bodies[0], publisher.bodies[1]) {
		t.Fatal("unknown outcome retried with a different document")
	}
	var found *VerifiedManifest
	for _, body := range publisher.bodies {
		v, err := VerifyManifest(authority, body, now)
		if err != nil {
			t.Fatal(err)
		}
		if v.Start.Equal(window) {
			found = &v
		}
	}
	if found == nil || len(found.Members) != 2 || found.Approval.Scope != testScope {
		t.Fatal("scoped manifest must hold exactly the anchored members", found)
	}
	count := len(publisher.bodies)
	if r, err := p.RunOnce(ctx); err != nil || r.Published != 0 || len(publisher.bodies) != count {
		t.Fatal("unchanged window republished", r, err)
	}
	// A late fact changes the digest: a strictly higher revision is minted.
	late := testMembers(window, 1)[0]
	p.Source = append(facts, ProducerFact{Member: late, Family: FamilyNetworkEvents, AnchorKind: "missions", AnchorRef: "anchored"})
	if _, err := p.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	last, err := VerifyManifest(authority, publisher.bodies[len(publisher.bodies)-1], now)
	if err != nil || last.Approval.Revision <= found.Approval.Revision || len(last.Members) != 3 {
		t.Fatal("late fact not attested with a newer revision", err)
	}
	// Roster manifests are withheld for windows the owner did not fully observe.
	p.RosterEnabled = true
	r, err := p.RunOnce(ctx)
	if err != nil || r.SkippedRoster == 0 {
		t.Fatal("unobserved roster window attested", r, err)
	}
}

type staticRoster []networkanalytics.SupplyRosterMember

func (s staticRoster) Observe(_ context.Context, bucket time.Time) ([]networkanalytics.SupplyRosterMember, error) {
	out := []networkanalytics.SupplyRosterMember{}
	for _, m := range s {
		m.BucketStart = bucket
		out = append(out, m)
	}
	return out, nil
}

type tableOutbox struct{}

func (tableOutbox) Enqueue(ctx context.Context, tx pgx.Tx, e sharedevents.Envelope) error {
	_, err := tx.Exec(ctx, `INSERT INTO test_outbox(event_id) VALUES($1)`, e.EventID)
	return err
}

func TestPostgresRosterRecorderRecordsIndividualMembersOncePerBucket(t *testing.T) {
	db := producerDB(t)
	ctx := context.Background()
	if _, err := db.Exec(ctx, `CREATE TABLE test_outbox(event_id text PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	roster := staticRoster{
		{Authority: "HAULING", ProductCode: "HAULAGE", ProviderID: uuid.NewString(), Eligible: networkanalytics.Bool(true), Occupied: networkanalytics.Bool(false), PolicyVersion: "v1"},
		{Authority: "HAULING", ProductCode: "HAULAGE", ProviderID: uuid.NewString(), Occupied: networkanalytics.Bool(true), PolicyVersion: "v1"},
	}
	recorder := &RosterRecorder{SourceService: "driver-hauling-service", PolicyVersion: "v1", DB: db, Source: roster, Outbox: tableOutbox{}, Now: func() time.Time { return now }}
	var wg sync.WaitGroup
	results := make(chan int, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := recorder.RunOnce(ctx)
			if err != nil {
				t.Error(err)
			}
			results <- n
		}()
	}
	wg.Wait()
	close(results)
	total := 0
	for n := range results {
		total += n
	}
	var members, outbox int
	db.QueryRow(ctx, `SELECT count(*) FROM analytics_supply_roster_members`).Scan(&members)
	db.QueryRow(ctx, `SELECT count(*) FROM test_outbox`).Scan(&outbox)
	if total != 2 || members != 2 || outbox != 2 {
		t.Fatal("bucket recorded more than once", total, members, outbox)
	}
	var eligible *bool
	db.QueryRow(ctx, `SELECT eligible FROM analytics_supply_roster_members WHERE occupied`).Scan(&eligible)
	if eligible != nil {
		t.Fatal("unknown eligibility became a value")
	}
	if _, err := db.Exec(ctx, `DELETE FROM analytics_supply_roster_members`); err == nil {
		t.Fatal("roster evidence mutable")
	}
	bad := staticRoster{{Authority: "HAULING", ProductCode: "HAULAGE", ProviderID: "not-a-uuid", PolicyVersion: "v1"}}
	recorder.Source, recorder.Now = bad, func() time.Time { return now.Add(networkanalytics.SupplyBucket) }
	if _, err := recorder.RunOnce(ctx); err == nil {
		t.Fatal("invalid roster member recorded")
	}
}
