package adminprivacy

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"cosmicforge/logistics/services/analytics-service/internal/features/networkanalytics/repositories"
	"cosmicforge/logistics/services/analytics-service/internal/features/networkanalytics/usecases"
	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/analyticsadmin"
	"cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/networkanalytics"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEST_ONLY owner key: an ephemeral Ed25519 pair pinned as the owner
// authority. Production builds reject the TEST_ONLY environment.
type ownerKey struct {
	authority admincontext.Authority
	signer    analyticsadmin.Signer
}

func newOwnerKey(t *testing.T, owner string) ownerKey {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return ownerKey{authority: admincontext.Authority{Owner: owner, Environment: adminpolicy.TestOnly, Keys: map[string]ed25519.PublicKey{"test-only-owner": pub}}, signer: analyticsadmin.Signer{KeyID: "test-only-owner", Key: key}}
}

func (k ownerKey) manifest(t *testing.T, family string, scope adminpolicy.Scope, start, end time.Time, revision int64, members []analyticsadmin.ManifestMember) []byte {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	a, err := analyticsadmin.ManifestApproval{Owner: k.authority.Owner, Environment: adminpolicy.TestOnly, Family: family, Scope: scope, Start: start, End: end, Revision: revision, ProvenanceRef: "TEST_ONLY_EXPORT", ApprovalRef: "TEST_ONLY_APPROVAL", EffectiveAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)}.Document(members)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := k.signer.SignManifest(a, members)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func (k ownerKey) signApproval(t *testing.T, a admincontext.Approval) []byte {
	t.Helper()
	document, _ := json.Marshal(a)
	raw, _ := json.Marshal(admincontext.Envelope{Document: document, KeyID: k.signer.KeyID, Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(k.signer.Key, append([]byte(admincontext.SigningDomain), document...)))})
	return raw
}

func ingestFact(t *testing.T, db *pgxpool.Pool, owner, name, entityType, entityID string, version int64, at time.Time, payload map[string]any) analyticsadmin.ManifestMember {
	t.Helper()
	raw, _ := json.Marshal(payload)
	id := uuid.NewString()
	status, err := usecases.NewService(repositories.NewRepository(db)).Ingest(context.Background(), owner, events.Envelope{EventID: id, EventName: name, SchemaVersion: 1, SourceService: owner, SourceEntityType: entityType, SourceEntityID: entityID, SourceVersion: version, OccurredAt: at, Payload: raw})
	if err != nil || status != "PROCESSED" {
		t.Fatal(status, err)
	}
	return analyticsadmin.ManifestMember{EventID: id, EventName: name, EntityType: entityType, EntityID: entityID, Version: version, OccurredAt: at.UTC().Truncate(time.Microsecond)}
}

type reportFixture struct {
	db      *pgxpool.Pool
	policy  *policyfixture.Fixture
	actor   adminpolicy.Actor
	q       Query
	key     ownerKey
	store   *ManifestStore
	service *Service
	source  Source
}

func newReportFixture(t *testing.T, report, owner, authority, product, field string) *reportFixture {
	t.Helper()
	db := lineageDB(t)
	f, actor, q, _ := privacyFixture(t)
	r, _ := analyticsadmin.ReportNamed(report)
	q.Metric, q.Capability = report, r.Permission
	q.Filters = map[string]string{"authority": authority, "product_code": product}
	actor.Permissions = []string{q.Capability}
	f.Document.Rules[0].Capability = q.Capability
	f.Document.Rules[0].Permission = q.Capability
	f.Document.Rules[0].SourceContract = r.Contract + ":" + field
	f.Document.Analytics.AllowedCellKeys = r.Keys
	f.Document.Analytics.AllowedFilterValues["authority"] = []string{authority}
	f.Document.Analytics.AllowedFilterValues["product_code"] = []string{product}
	f.Sign(t)
	key := newOwnerKey(t, owner)
	authorities := map[string]admincontext.Authority{owner: key.authority}
	var source Source
	switch report {
	case "mission-success":
		source = &MissionSuccessSource{Authorities: authorities, UnitField: field}
	case "readiness":
		source = &ReadinessSource{Authorities: authorities, UnitField: field}
	case "supply", "demand-forecast", "recovery-resolution", "provider-quality-score":
		source = &CoveredSource{Report: report, Authorities: authorities, UnitField: field}
	default:
		source = &ReportSource{Report: report, Authorities: authorities, UnitField: field}
	}
	return &reportFixture{db: db, policy: f, actor: actor, q: q, key: key, store: &ManifestStore{DB: db, Authority: key.authority}, service: &Service{DB: db, Policy: f.Engine, Source: source}, source: source}
}

func (f *reportFixture) publish(t *testing.T, family string, scope adminpolicy.Scope, start, end time.Time, revision int64, members []analyticsadmin.ManifestMember) {
	t.Helper()
	if err := f.store.Import(context.Background(), f.key.manifest(t, family, scope, start, end, revision, members), time.Now()); err != nil {
		t.Fatal(err)
	}
}

func (f *reportFixture) snapshot(t *testing.T, q Query) (Snapshot, error) {
	t.Helper()
	tx, err := f.db.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	return f.source.Snapshot(context.Background(), tx, q)
}

func cellValue(t *testing.T, s Snapshot, key string) *int64 {
	t.Helper()
	for _, c := range s.Cells {
		if c.Key == key {
			return c.Count
		}
	}
	t.Fatalf("cell %s missing", key)
	return nil
}

func provider(i int) string { return fmt.Sprintf("aaaaaaaa-aaaa-4aaa-8aaa-%012d", i) }

func taxiOutcomes(t *testing.T, f *reportFixture, counts ...int) []analyticsadmin.ManifestMember {
	names := []string{"taxi.booking.completed", "taxi.booking.unmatched", "taxi.booking.cancelled", "taxi.guarantee.breached"}
	members := []analyticsadmin.ManifestMember{}
	for n, count := range counts {
		for i := 0; i < count; i++ {
			members = append(members, ingestFact(t, f.db, "driver-taxi-service", names[n], "booking", uuid.NewString(), 1, f.q.Start.Add(time.Duration(n*10+i+1)*time.Minute/2), map[string]any{"provider_id": provider(n*100 + i), "product_code": "TAXI_PASSENGER"}))
		}
	}
	return members
}

func TestPostgresCompleteManifestReleasesMissionSuccessAndFreezesMembership(t *testing.T) {
	f := newReportFixture(t, "mission-success", "driver-taxi-service", "TAXI", "TAXI_PASSENGER", "provider_id")
	ctx := context.Background()
	members := taxiOutcomes(t, f, 12, 5, 6, 7)
	// Two half-hour manifests tile the one-hour release window exactly.
	mid := f.q.Start.Add(30 * time.Minute)
	first, second := []analyticsadmin.ManifestMember{}, []analyticsadmin.ManifestMember{}
	for _, m := range members {
		if m.OccurredAt.Before(mid) {
			first = append(first, m)
		} else {
			second = append(second, m)
		}
	}
	f.publish(t, analyticsadmin.FamilyNetworkEvents, f.q.Scope, f.q.Start, mid, 1, first)
	f.publish(t, analyticsadmin.FamilyNetworkEvents, f.q.Scope, mid, f.q.End, 1, second)
	snapshot, err := f.snapshot(t, f.q)
	if err != nil || snapshot.Partial || *cellValue(t, snapshot, "completed") != 12 || *cellValue(t, snapshot, "failed") != 5 {
		t.Fatal("complete reduction wrong", snapshot, err)
	}
	release, err := f.service.Disclose(ctx, f.actor, f.q)
	if err != nil || release.Coverage != "COMPLETE" {
		t.Fatal(release, err)
	}
	for _, c := range release.Cells {
		if c.Suppression != "NONE" || c.Value == nil {
			t.Fatal("complete cell withheld", c)
		}
		if c.Key == "completed" && *c.Value != 10 {
			t.Fatal("policy rounding not applied", *c.Value)
		}
	}
	// A frozen release re-verifies its exact manifest revisions: a later
	// owner revision (even a superset) denies the old release, never resamples.
	extra := ingestFact(t, f.db, "driver-taxi-service", "taxi.booking.completed", "booking", uuid.NewString(), 1, mid.Add(time.Minute), map[string]any{"provider_id": provider(50), "product_code": "TAXI_PASSENGER"})
	f.publish(t, analyticsadmin.FamilyNetworkEvents, f.q.Scope, mid, f.q.End, 2, append(second, extra))
	if _, err := f.service.Disclose(ctx, f.actor, f.q); !errors.Is(err, ErrUnmapped) {
		t.Fatal("superseded frozen release disclosed", err)
	}
}

func TestPostgresCoverageMismatchKeepsPartialAndForeignScopeDenied(t *testing.T) {
	f := newReportFixture(t, "mission-outcomes", "driver-taxi-service", "TAXI", "TAXI_PASSENGER", "provider_id")
	ctx := context.Background()
	members := taxiOutcomes(t, f, 6, 0)
	// The owner attests a fact Analytics never ingested: coverage mismatch.
	ghost := members[0]
	ghost.EventID = uuid.NewString()
	f.publish(t, analyticsadmin.FamilyNetworkEvents, f.q.Scope, f.q.Start, f.q.End, 1, append(members, ghost))
	// Bounded per-fact allocation exists for one member, so the partial path runs.
	kind, _ := LineageKind("mission-outcomes", "provider_id")
	reader := LineageReader{Owner: "driver-taxi-service"}
	tx, _ := f.db.Begin(ctx)
	binding, err := reader.Binding(ctx, tx, kind, members[0].EventID)
	tx.Rollback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	approval := admincontext.Approval{SchemaVersion: 1, ID: "allocation-" + members[0].EventID, Revision: 1, Owner: "driver-taxi-service", Environment: adminpolicy.TestOnly, State: "APPROVED", ResourceKind: kind, ResourceRef: members[0].EventID, SourceContract: binding.SourceContract, BindingFingerprint: binding.Fingerprint, Scope: f.q.Scope, Capabilities: []string{"analytics.cohorts.mission-outcomes.export"}, ProvenanceRef: "TEST_ONLY", ApprovalRef: "TEST_ONLY", EffectiveAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)}
	if err := (&admincontext.Store{DB: f.db, Reader: reader, Authority: f.key.authority}).Import(ctx, f.key.signApproval(t, approval), time.Now()); err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.snapshot(t, f.q)
	if err != nil || !snapshot.Partial {
		t.Fatal("coverage mismatch was not kept partial", snapshot, err)
	}
	release, err := f.service.Disclose(ctx, f.actor, f.q)
	if err != nil || release.Coverage != "PARTIAL_COVERAGE" {
		t.Fatal(release, err)
	}
	for _, c := range release.Cells {
		if c.Value != nil {
			t.Fatal("partial value released")
		}
	}
	foreign := f.q
	foreign.Scope.Tenant = "foreign-tenant"
	if _, err := f.service.Disclose(ctx, f.actor, foreign); err == nil {
		t.Fatal("cross-tenant release")
	}
}

func TestPostgresManifestForgeryReplayStaleAndRevocation(t *testing.T) {
	f := newReportFixture(t, "mission-success", "driver-taxi-service", "TAXI", "TAXI_PASSENGER", "provider_id")
	ctx := context.Background()
	members := taxiOutcomes(t, f, 6, 6)
	good := f.key.manifest(t, analyticsadmin.FamilyNetworkEvents, f.q.Scope, f.q.Start, f.q.End, 2, members)
	// Forged: another key signs the same content.
	forger := newOwnerKey(t, "driver-taxi-service")
	if err := f.store.Import(ctx, forger.manifest(t, analyticsadmin.FamilyNetworkEvents, f.q.Scope, f.q.Start, f.q.End, 2, members), time.Now()); err == nil {
		t.Fatal("forged manifest accepted")
	}
	// Tampered: a member dropped after signing breaks the signed digest.
	var wire analyticsadmin.Manifest
	json.Unmarshal(good, &wire)
	wire.Members = wire.Members[1:]
	tampered, _ := json.Marshal(wire)
	if err := f.store.Import(ctx, tampered, time.Now()); err == nil {
		t.Fatal("tampered member list accepted")
	}
	// Wrong owner authority: Hauling's pin cannot accept Taxi's document.
	hauling := &ManifestStore{DB: f.db, Authority: admincontext.Authority{Owner: "driver-hauling-service", Environment: adminpolicy.TestOnly, Keys: f.key.authority.Keys}}
	if err := hauling.Import(ctx, good, time.Now()); err == nil {
		t.Fatal("cross-owner manifest accepted")
	}
	// Expired at import time.
	if err := f.store.Import(ctx, good, time.Now().Add(2*time.Hour)); err == nil {
		t.Fatal("expired manifest accepted")
	}
	if err := f.store.Import(ctx, good, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Import(ctx, good, time.Now()); err != nil {
		t.Fatal("identical revision should be idempotent", err)
	}
	// Stale revision is fenced, even with different content.
	if err := f.store.Import(ctx, f.key.manifest(t, analyticsadmin.FamilyNetworkEvents, f.q.Scope, f.q.Start, f.q.End, 1, members[:3]), time.Now()); !errors.Is(err, errStaleManifest) {
		t.Fatal("stale revision accepted", err)
	}
	// Same revision, different content: conflict.
	if err := f.store.Import(ctx, f.key.manifest(t, analyticsadmin.FamilyNetworkEvents, f.q.Scope, f.q.Start, f.q.End, 2, members[:3]), time.Now()); !errors.Is(err, errStaleManifest) {
		t.Fatal("conflicting same revision accepted", err)
	}
	if s, err := f.snapshot(t, f.q); err != nil || s.Partial {
		t.Fatal("complete coverage expected", err)
	}
	// Revocation withdraws coverage immediately; with no per-fact allocations
	// the source has no membership at all.
	now := time.Now().UTC().Truncate(time.Second)
	revoked, _ := analyticsadmin.ManifestApproval{Owner: "driver-taxi-service", Environment: adminpolicy.TestOnly, Family: analyticsadmin.FamilyNetworkEvents, Scope: f.q.Scope, Start: f.q.Start, End: f.q.End, Revision: 3, ProvenanceRef: "TEST_ONLY", ApprovalRef: "TEST_ONLY", EffectiveAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour), Revoked: true}.Document(members)
	raw, _ := f.key.signer.SignManifest(revoked, members)
	if err := f.store.Import(ctx, raw, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.snapshot(t, f.q); !errors.Is(err, ErrUnmapped) {
		t.Fatal("revoked coverage still used", err)
	}
	// History is append-only evidence.
	if _, err := f.db.Exec(ctx, `DELETE FROM analytics_admin_coverage_history`); err == nil {
		t.Fatal("coverage history mutable")
	}
}

func TestPostgresCompleteSuppressionAndAuditFailureWithholding(t *testing.T) {
	f := newReportFixture(t, "mission-success", "driver-taxi-service", "TAXI", "TAXI_PASSENGER", "provider_id")
	ctx := context.Background()
	members := taxiOutcomes(t, f, 3, 6, 6, 6)
	f.publish(t, analyticsadmin.FamilyNetworkEvents, f.q.Scope, f.q.Start, f.q.End, 1, members)
	if _, err := f.db.Exec(ctx, `CREATE FUNCTION fail_cov_audit() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN RAISE EXCEPTION 'TEST_ONLY_AUDIT_FAILURE';END$$; CREATE TRIGGER fail_cov_audit BEFORE INSERT ON analytics_admin_query_audit FOR EACH ROW EXECUTE FUNCTION fail_cov_audit()`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Disclose(ctx, f.actor, f.q); err == nil {
		t.Fatal("released without audit")
	}
	f.db.Exec(ctx, `DROP TRIGGER fail_cov_audit ON analytics_admin_query_audit`)
	release, err := f.service.Disclose(ctx, f.actor, f.q)
	if err != nil || release.Coverage != "COMPLETE" {
		t.Fatal(release, err)
	}
	// Three completed units are below the minimum cohort: everything withheld.
	for _, c := range release.Cells {
		if c.Value != nil || c.Suppression != "SMALL_CELL" && c.Suppression != "COMPLEMENTARY" {
			t.Fatal("small cohort disclosed", c)
		}
	}
}

func TestPostgresRecoveryResolutionDurationsFromSourceEvents(t *testing.T) {
	f := newReportFixture(t, "recovery-resolution", "driver-hauling-service", "HAULING", "HAULAGE", "source_entity_id")
	members := []analyticsadmin.ManifestMember{}
	base := f.q.Start.Add(time.Minute)
	for i, seconds := range []int{60, 120, 180, 240, 300} {
		id := uuid.NewString()
		at := base.Add(time.Duration(i) * time.Minute)
		members = append(members, ingestFact(t, f.db, "driver-hauling-service", "hauling.recovery.started", "recovery_case", id, 1, at, map[string]any{"product_code": "HAULAGE"}))
		members = append(members, ingestFact(t, f.db, "driver-hauling-service", "hauling.recovery.resolved", "recovery_case", id, 2, at.Add(time.Duration(seconds)*time.Second), map[string]any{"product_code": "HAULAGE"}))
	}
	failedCase, openCase := uuid.NewString(), uuid.NewString()
	members = append(members, ingestFact(t, f.db, "driver-hauling-service", "hauling.recovery.started", "recovery_case", failedCase, 1, base, map[string]any{"product_code": "HAULAGE"}))
	members = append(members, ingestFact(t, f.db, "driver-hauling-service", "hauling.recovery.failed", "recovery_case", failedCase, 2, base.Add(time.Hour/3), map[string]any{"product_code": "HAULAGE"}))
	members = append(members, ingestFact(t, f.db, "driver-hauling-service", "hauling.recovery.started", "recovery_case", openCase, 1, base, map[string]any{"product_code": "HAULAGE"}))
	f.publish(t, analyticsadmin.FamilyNetworkEvents, f.q.Scope, f.q.Start, f.q.End, 1, members)
	s, err := f.snapshot(t, f.q)
	if err != nil || s.Partial {
		t.Fatal(err)
	}
	for key, want := range map[string]int64{"resolved_with_duration": 5, "resolution_seconds_mean": 180, "resolution_seconds_median": 180} {
		if got := cellValue(t, s, key); got == nil || *got != want {
			t.Fatal(key, got, want)
		}
	}
	// Typed SDK over the signed native route.
	key := bytes.Repeat([]byte{84}, 32)
	engine := gin.New()
	(&Handler{Enabled: true, Secret: key, Service: f.service, Reports: map[string]*Service{"recovery-resolution": f.service}}).Register(engine)
	server := httptest.NewServer(engine)
	defer server.Close()
	client, _ := analyticsadmin.NewClient(server.URL, key)
	defer client.Close()
	request := adminread.Query{Scope: adminread.Scope{Market: f.q.Scope.Market, Zone: f.q.Scope.Zone, Organization: f.q.Scope.Organization, Tenant: f.q.Scope.Tenant}, Staff: f.actor.Staff, Role: "staff", Purpose: f.q.Purpose, PolicyID: f.policy.Engine.ID(), PolicyVersion: f.policy.Engine.Version(), PolicyChecksum: f.policy.Engine.Checksum(), Start: f.q.Start, End: f.q.End, Limit: 1}
	out, err := client.RecoveryResolution(context.Background(), request, analyticsadmin.Filters{Authority: "HAULING", ProductCode: "HAULAGE"})
	if err != nil || out.Coverage != "COMPLETE" || out.Metric != "recovery-resolution" {
		t.Fatal(out, err)
	}
	for _, c := range out.Cells {
		if c.Key == "resolution_seconds_mean" && (c.Value == nil || *c.Value != 180) {
			t.Fatal("mean not released", c)
		}
	}
}

func TestPostgresSupplyRosterMembershipAndForecastHistory(t *testing.T) {
	t.Run("supply", func(t *testing.T) {
		f := newReportFixture(t, "supply", "driver-hauling-service", "HAULING", "HAULAGE", "provider_id")
		if _, err := f.snapshot(t, f.q); !errors.Is(err, ErrUnmapped) {
			t.Fatal("supply released without a roster", err)
		}
		members := []analyticsadmin.ManifestMember{}
		bucket := f.q.Start.Add(10 * time.Minute)
		for i := 0; i < 10; i++ {
			m := networkanalytics.SupplyRosterMember{Authority: "HAULING", ProductCode: "HAULAGE", ProviderID: provider(i), Eligible: networkanalytics.Bool(true), Occupied: networkanalytics.Bool(i >= 5), BucketStart: bucket, PolicyVersion: "hauling_active_truck_v1"}
			envelope, _ := networkanalytics.RosterEnvelope("driver-hauling-service", m)
			var payload map[string]any
			json.Unmarshal(envelope.Payload, &payload)
			members = append(members, ingestFact(t, f.db, "driver-hauling-service", envelope.EventName, envelope.SourceEntityType, envelope.SourceEntityID, envelope.SourceVersion, envelope.OccurredAt, payload))
		}
		// An aggregate supply snapshot can never be a roster member.
		snapshotMember := members[0]
		snapshotMember.EventName = networkanalytics.EventSupplySnapshot
		if _, err := analyticsadmin.CanonicalMembers(analyticsadmin.FamilySupplyRoster, []analyticsadmin.ManifestMember{snapshotMember}, f.q.Start, f.q.End); err == nil {
			t.Fatal("aggregate accepted as roster membership")
		}
		f.publish(t, analyticsadmin.FamilySupplyRoster, f.q.Scope, f.q.Start, f.q.End, 1, members)
		s, err := f.snapshot(t, f.q)
		if err != nil || s.Partial || *cellValue(t, s, "available_providers") != 5 || *cellValue(t, s, "busy_providers") != 5 {
			t.Fatal(s, err)
		}
		release, err := f.service.Disclose(context.Background(), f.actor, f.q)
		if err != nil || release.Coverage != "COMPLETE" {
			t.Fatal(release, err)
		}
	})
	t.Run("forecast", func(t *testing.T) {
		f := newReportFixture(t, "demand-forecast", "driver-hauling-service", "HAULING", "HAULAGE", "source_entity_id")
		for week, n := range []int{5, 6, 7, 8} {
			shift := time.Duration(week+1) * 7 * 24 * time.Hour
			start, end := f.q.Start.Add(-shift), f.q.End.Add(-shift)
			members := []analyticsadmin.ManifestMember{}
			for i := 0; i < n; i++ {
				members = append(members, ingestFact(t, f.db, "driver-hauling-service", "hauling.booking.committed.v1", "haulage_booking", uuid.NewString(), 1, start.Add(time.Duration(i+1)*time.Minute), map[string]any{"product_code": "HAULAGE"}))
			}
			if week == 3 {
				if _, err := f.snapshot(t, f.q); !errors.Is(err, ErrUnmapped) {
					t.Fatal("forecast fed with incomplete history", err)
				}
			}
			f.publish(t, analyticsadmin.FamilyNetworkEvents, f.q.Scope, start, end, 1, members)
		}
		s, err := f.snapshot(t, f.q)
		if err != nil || s.Partial || *cellValue(t, s, "forecast_requests") != 6 {
			t.Fatal(s, err)
		}
	})
}

func TestPostgresProviderQualityScoresVerifiedRosterMembersOnly(t *testing.T) {
	f := newReportFixture(t, "provider-quality-score", "driver-taxi-service", "TAXI", "TAXI_PASSENGER", "provider_id")
	at := f.q.Start.Add(time.Minute)
	events := []analyticsadmin.ManifestMember{}
	add := func(name, provider, role string) {
		events = append(events, ingestFact(t, f.db, "driver-taxi-service", name, "booking", uuid.NewString(), 1, at, map[string]any{role: provider, "product_code": "TAXI_PASSENGER"}))
	}
	for i := 0; i < 5; i++ {
		add("taxi.booking.completed", provider(1), "provider_id")
		add("taxi.booking.completed", provider(2), "provider_id") // not on the roster
	}
	add("taxi.booking.unmatched", provider(1), "provider_id")
	add("taxi.recovery.started", provider(1), "original_provider_id")
	m := networkanalytics.SupplyRosterMember{Authority: "TAXI", ProductCode: "TAXI_PASSENGER", ProviderID: provider(1), Eligible: networkanalytics.Bool(true), Occupied: networkanalytics.Bool(false), BucketStart: f.q.Start, PolicyVersion: "taxi_operational_state_v1"}
	envelope, _ := networkanalytics.RosterEnvelope("driver-taxi-service", m)
	var payload map[string]any
	json.Unmarshal(envelope.Payload, &payload)
	roster := []analyticsadmin.ManifestMember{ingestFact(t, f.db, "driver-taxi-service", envelope.EventName, envelope.SourceEntityType, envelope.SourceEntityID, envelope.SourceVersion, envelope.OccurredAt, payload)}
	f.publish(t, analyticsadmin.FamilyNetworkEvents, f.q.Scope, f.q.Start, f.q.End, 1, events)
	// Without roster coverage there is no complete release, and the score has
	// no partial form: it denies.
	if _, err := f.snapshot(t, f.q); !errors.Is(err, ErrUnmapped) {
		t.Fatal("quality released without verified roster", err)
	}
	f.publish(t, analyticsadmin.FamilySupplyRoster, f.q.Scope, f.q.Start, f.q.End, 1, roster)
	s, err := f.snapshot(t, f.q)
	if err != nil || s.Partial {
		t.Fatal(s, err)
	}
	// Provider 1: completed 5, failed 1, recoveries 1 => held 7,
	// score = 10000 * 5/6 * (1 - 1/7) = 7142 (floor). Provider 2 unscored.
	for key, want := range map[string]int64{"quality_scored_providers": 1, "quality_score_v1_mean_bp": 7142, "quality_score_v1_median_bp": 7142} {
		if got := cellValue(t, s, key); got == nil || *got != want {
			t.Fatal(key, got, want)
		}
	}
}

func TestPostgresGlobalManifestRequiresEveryMemberAllocated(t *testing.T) {
	f := newReportFixture(t, "readiness", "vendor-service", "VENDOR", "VENDOR_FOOD", "source_entity_id")
	ctx := context.Background()
	reader := LineageReader{Owner: "vendor-service"}
	contexts := &admincontext.Store{DB: f.db, Reader: reader, Authority: f.key.authority}
	kind, _ := LineageKind("readiness", "source_entity_id")
	members := []analyticsadmin.ManifestMember{}
	allocate := func(m analyticsadmin.ManifestMember, scope adminpolicy.Scope) {
		tx, _ := f.db.Begin(ctx)
		binding, err := reader.Binding(ctx, tx, kind, m.EventID)
		tx.Rollback(ctx)
		if err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC().Truncate(time.Second)
		a := admincontext.Approval{SchemaVersion: 1, ID: "alloc-" + m.EventID, Revision: 1, Owner: "vendor-service", Environment: adminpolicy.TestOnly, State: "APPROVED", ResourceKind: kind, ResourceRef: m.EventID, SourceContract: binding.SourceContract, BindingFingerprint: binding.Fingerprint, Scope: scope, Capabilities: []string{"analytics.cohorts.readiness.export"}, ProvenanceRef: "TEST_ONLY", ApprovalRef: "TEST_ONLY", EffectiveAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)}
		if err := contexts.Import(ctx, f.key.signApproval(t, a), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	other := f.q.Scope
	other.Tenant = "other-tenant"
	for i := 0; i < 7; i++ {
		m := ingestFact(t, f.db, "vendor-service", networkanalytics.EventBranchReadiness, "vendor_order", uuid.NewString(), 1, f.q.Start.Add(time.Duration(i+1)*time.Minute), map[string]any{"branch_id": provider(i), "product_code": "VENDOR_FOOD", "on_time": true, "lateness_seconds": 0, "prep_seconds": 30})
		members = append(members, m)
		if i < 6 {
			scope := f.q.Scope
			if i == 5 {
				scope = other
			}
			allocate(m, scope)
		}
	}
	f.publish(t, analyticsadmin.FamilyNetworkEventsGlobal, analyticsadmin.GlobalScope, f.q.Start, f.q.End, 1, members)
	// One attested readiness fact has no allocation anywhere: incomplete.
	if s, err := f.snapshot(t, f.q); err != nil || !s.Partial {
		t.Fatal("unallocated member did not keep the report partial", err)
	}
	allocate(members[6], f.q.Scope)
	s, err := f.snapshot(t, f.q)
	if err != nil || s.Partial || *cellValue(t, s, "ready_count") != 6 {
		t.Fatal("global coverage with complete allocations", s, err)
	}
}

func TestPostgresCoveragePublicationHTTPBoundary(t *testing.T) {
	f := newReportFixture(t, "mission-success", "driver-taxi-service", "TAXI", "TAXI_PASSENGER", "provider_id")
	ctx := context.Background()
	members := taxiOutcomes(t, f, 2, 0)
	secret := bytes.Repeat([]byte{77}, 32)
	engine := gin.New()
	(&LineagePublisher{coverage: map[string]*CoverageHandler{"driver-taxi-service": {Owner: "driver-taxi-service", DB: f.db, Secret: secret, Store: f.store, Enabled: true}}}).Register(engine)
	server := httptest.NewServer(engine)
	defer server.Close()
	client, err := analyticsadmin.NewPublisherClient(server.URL, "driver-taxi-service", secret)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	raw := f.key.manifest(t, analyticsadmin.FamilyNetworkEvents, f.q.Scope, f.q.Start, f.q.End, 2, members)
	if err := client.PublishManifest(ctx, raw); err != nil {
		t.Fatal(err)
	}
	if err := client.PublishManifest(ctx, f.key.manifest(t, analyticsadmin.FamilyNetworkEvents, f.q.Scope, f.q.Start, f.q.End, 1, members)); !errors.Is(err, analyticsadmin.ErrDenied) {
		t.Fatal("stale revision not fenced over HTTP", err)
	}
	forger := newOwnerKey(t, "driver-taxi-service")
	if err := client.PublishManifest(ctx, forger.manifest(t, analyticsadmin.FamilyNetworkEvents, f.q.Scope, f.q.Start, f.q.End, 3, members)); !errors.Is(err, analyticsadmin.ErrDenied) {
		t.Fatal("forged manifest accepted over HTTP", err)
	}
	wrong, _ := analyticsadmin.NewPublisherClient(server.URL, "driver-taxi-service", bytes.Repeat([]byte{78}, 32))
	defer wrong.Close()
	if err := wrong.PublishManifest(ctx, raw); !errors.Is(err, analyticsadmin.ErrDenied) {
		t.Fatal("wrong workload key accepted", err)
	}
	// HQ read credentials cannot publish (different caller identity).
	request := httptest.NewRequest("POST", analyticsadmin.CoverageManifestPath+"driver-taxi-service", bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	if recorder.Code != 401 {
		t.Fatal("unsigned publication accepted", recorder.Code)
	}
	// A captured, validly signed request replayed verbatim is refused.
	fresh := f.key.manifest(t, analyticsadmin.FamilyNetworkEvents, f.q.Scope, f.q.Start, f.q.End, 5, members)
	signed := httptest.NewRequest("POST", analyticsadmin.CoverageManifestPath+"driver-taxi-service", bytes.NewReader(fresh))
	signed.Header.Set("Content-Type", "application/json")
	if err := serviceauth.SignRequest(signed, analyticsadmin.ProducerPublisherCaller, secret, fresh, time.Now()); err != nil {
		t.Fatal(err)
	}
	for i, want := range []int{200, 409} {
		replay := signed.Clone(ctx)
		replay.Body = io.NopCloser(bytes.NewReader(fresh))
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, replay)
		if recorder.Code != want {
			t.Fatal("attempt", i, recorder.Code, recorder.Body.String())
		}
	}
	// HQ read workload identity cannot publish manifests.
	hq := httptest.NewRequest("POST", analyticsadmin.CoverageManifestPath+"driver-taxi-service", bytes.NewReader(fresh))
	hq.Header.Set("Content-Type", "application/json")
	serviceauth.SignRequest(hq, adminread.Caller, secret, fresh, time.Now())
	recorder = httptest.NewRecorder()
	engine.ServeHTTP(recorder, hq)
	if recorder.Code != 401 {
		t.Fatal("HQ identity published coverage", recorder.Code)
	}
}

func TestPostgresCompleteCountingReportsAndTypedSDK(t *testing.T) {
	cases := []struct {
		report, owner, authority, product, field, event, entity string
		payload                                                 map[string]any
		want                                                    map[string]int64
	}{
		// Dispatch delivery-mission mismatches and declines are covered by
		// TestPostgresEnrolledPopulationsReleaseCompleteOnlyWhenTiled.
		{"capability-mismatches", "driver-hauling-service", "HAULING", "HAULAGE", "source_entity_id", "hauling.capability_mismatch", "recovery_case", map[string]any{"reason_code": "CAPACITY"}, map[string]int64{"mismatches": 6}},
		{"demand", "driver-hauling-service", "HAULING", "HAULAGE", "source_entity_id", "hauling.booking.committed.v1", "haulage_booking", map[string]any{}, map[string]int64{"requests": 6, "unmatched": 0}},
		{"mission-outcomes", "driver-hauling-service", "HAULING", "HAULAGE", "source_entity_id", "hauling.booking.completed.v1", "haulage_booking", map[string]any{}, map[string]int64{"completed": 6, "committed": 0}},
	}
	for _, tc := range cases {
		t.Run(tc.report, func(t *testing.T) {
			f := newReportFixture(t, tc.report, tc.owner, tc.authority, tc.product, tc.field)
			members := []analyticsadmin.ManifestMember{}
			for i := 0; i < 6; i++ {
				payload := map[string]any{"provider_id": provider(i), "product_code": tc.product}
				for k, v := range tc.payload {
					payload[k] = v
				}
				members = append(members, ingestFact(t, f.db, tc.owner, tc.event, tc.entity, uuid.NewString(), 1, f.q.Start.Add(time.Duration(i+1)*time.Minute), payload))
			}
			f.publish(t, analyticsadmin.FamilyNetworkEvents, f.q.Scope, f.q.Start, f.q.End, 1, members)
			s, err := f.snapshot(t, f.q)
			if err != nil || s.Partial {
				t.Fatal(s, err)
			}
			for key, want := range tc.want {
				if got := cellValue(t, s, key); got == nil || *got != want {
					t.Fatal(key, got, want)
				}
			}
			// Typed SDK over the signed native route returns COMPLETE coverage.
			key := bytes.Repeat([]byte{83}, 32)
			engine := gin.New()
			(&Handler{Enabled: true, Secret: key, Service: f.service, Reports: map[string]*Service{tc.report: f.service}}).Register(engine)
			server := httptest.NewServer(engine)
			defer server.Close()
			client, err := analyticsadmin.NewClient(server.URL, key)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			request := adminread.Query{Scope: adminread.Scope{Market: f.q.Scope.Market, Zone: f.q.Scope.Zone, Organization: f.q.Scope.Organization, Tenant: f.q.Scope.Tenant}, Staff: f.actor.Staff, Role: "staff", Purpose: f.q.Purpose, PolicyID: f.policy.Engine.ID(), PolicyVersion: f.policy.Engine.Version(), PolicyChecksum: f.policy.Engine.Checksum(), Start: f.q.Start, End: f.q.End, Limit: 1}
			readers := map[string]func(context.Context, adminread.Query, analyticsadmin.Filters) (analyticsadmin.Release, error){"capability-mismatches": client.CapabilityMismatches, "declines": client.Declines, "demand": client.Demand, "mission-outcomes": client.MissionOutcomes}
			out, err := readers[tc.report](context.Background(), request, analyticsadmin.Filters{Authority: tc.authority, ProductCode: tc.product})
			if err != nil || out.Coverage != "COMPLETE" {
				t.Fatal(out, err)
			}
		})
	}
}

// An authority with no authoritative unmatched event keeps an UNKNOWN cell; a
// Dispatch delivery failure is never read as unmatched demand.
func TestDemandUnmatchedUnknownWithoutAuthoritativeEvent(t *testing.T) {
	cells, err := reduceComplete(completeInput{Report: "demand", Field: "source_entity_id", Query: Query{Filters: map[string]string{"authority": "VENDOR"}}, Sets: map[string]coveredSet{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cells {
		if c.Key == "unmatched" && c.Count != nil {
			t.Fatal("unmatched invented for an authority with no unmatched outcome")
		}
	}
	failed := LineageFact{EventID: "f1", EventName: "trip.delivery_failed.v1", Authority: "DISPATCH", SubjectHash: "s1"}
	cells, err = reduceComplete(completeInput{Report: "demand", Field: "source_entity_id", Query: Query{Filters: map[string]string{"authority": "DISPATCH"}}, Sets: map[string]coveredSet{analyticsadmin.FamilyNetworkEvents: {Facts: []LineageFact{failed}, Payloads: map[string][]byte{"f1": []byte(`{}`)}}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cells {
		if c.Key == "unmatched" && (c.Count == nil || *c.Count != 0) {
			t.Fatal("a delivery failure was counted as unmatched demand", c)
		}
	}
	if m, md := meanMedian([]int64{4, 1, 3, 2}); m != 2 || md != 2 {
		t.Fatal(m, md)
	}
}

func TestPostgresRecoveryAndQualityComponentsUnchangedAndScoredReportsHaveNoPartialForm(t *testing.T) {
	f := newReportFixture(t, "provider-quality", "driver-taxi-service", "TAXI", "TAXI_PASSENGER", "provider_id")
	events := []analyticsadmin.ManifestMember{}
	for i := 0; i < 5; i++ {
		events = append(events, ingestFact(t, f.db, "driver-taxi-service", "taxi.booking.completed", "booking", uuid.NewString(), 1, f.q.Start.Add(time.Minute), map[string]any{"provider_id": provider(1), "product_code": "TAXI_PASSENGER"}))
	}
	f.publish(t, analyticsadmin.FamilyNetworkEvents, f.q.Scope, f.q.Start, f.q.End, 1, events)
	// provider-quality keeps its eight component cells and needs no roster.
	s, err := f.snapshot(t, f.q)
	if err != nil || s.Partial || len(s.Cells) != 8 || *cellValue(t, s, "completed") != 5 {
		t.Fatal(s, err)
	}
	for _, name := range []string{"recovery-resolution", "provider-quality-score"} {
		report, _ := analyticsadmin.ReportNamed(name)
		q := Query{Metric: name, Capability: report.Permission}
		if _, err := (&ReportSource{Report: name}).Snapshot(context.Background(), nil, q); !errors.Is(err, ErrUnmapped) {
			t.Fatal(name, "has a partial path", err)
		}
	}
	if len(analyticsadmin.Reports()) != 12 {
		t.Fatal("scoped report catalog drift")
	}
	for _, report := range analyticsadmin.Reports() {
		if _, err := LineageKind(report.Name, "provider_id"); err != nil {
			t.Fatal(report.Name, "has no native lineage", err)
		}
	}
}
