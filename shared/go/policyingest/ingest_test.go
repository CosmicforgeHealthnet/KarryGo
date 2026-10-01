package policyingest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/events"
)

// memoryStore is an in-memory Store with the same atomicity guarantees the
// real one must have.
type memoryStore struct {
	claimed   map[string]EventRecord
	terminal  map[string]Outcome
	pending   map[string]EventRecord
	snapshots []Snapshot
	applied   map[string]int64
	claimErr  error
	applyErr  error
}

func newStore() *memoryStore {
	return &memoryStore{
		claimed: map[string]EventRecord{}, terminal: map[string]Outcome{},
		pending: map[string]EventRecord{}, applied: map[string]int64{},
	}
}

func (s *memoryStore) ClaimEvent(_ context.Context, record EventRecord) (bool, error) {
	if s.claimErr != nil {
		return false, s.claimErr
	}
	if _, seen := s.claimed[record.EventID]; seen {
		return false, nil
	}
	s.claimed[record.EventID] = record
	return true, nil
}

func (s *memoryStore) AppliedSourceVersion(_ context.Context, kind string) (int64, bool, error) {
	version, ok := s.applied[kind]
	return version, ok, nil
}

func (s *memoryStore) ApplySnapshot(_ context.Context, snapshot Snapshot) error {
	if s.applyErr != nil {
		return s.applyErr
	}
	s.snapshots = append(s.snapshots, snapshot)
	s.applied[snapshot.PolicyKind] = snapshot.SourceVersion
	s.terminal[snapshot.EventID] = OutcomeApplied
	delete(s.pending, snapshot.EventID)
	return nil
}

func (s *memoryStore) MarkTerminal(_ context.Context, eventID string, outcome Outcome, _ string) error {
	s.terminal[eventID] = outcome
	delete(s.pending, eventID)
	return nil
}

func (s *memoryStore) MarkPendingReadBack(_ context.Context, eventID, _ string, _ time.Time) error {
	record := s.claimed[eventID]
	record.Attempts++
	s.pending[eventID] = record
	return nil
}

func (s *memoryStore) PendingReadBacks(_ context.Context, _ time.Time, limit int) ([]EventRecord, error) {
	out := make([]EventRecord, 0, len(s.pending))
	for _, record := range s.pending {
		if len(out) >= limit {
			break
		}
		out = append(out, record)
	}
	return out, nil
}

// stubReader is a controllable DocumentReader.
type stubReader struct {
	document  json.RawMessage
	retryable bool
	err       error
	calls     int
}

func (r *stubReader) ReadDocument(context.Context, string) (json.RawMessage, bool, error) {
	r.calls++
	return r.document, r.retryable, r.err
}

const capabilityKind = "HAULING_CAPABILITY"

func catalogue(version string) json.RawMessage {
	return json.RawMessage(`{"version":"` + version + `","products":[{"code":"HAUL_GENERAL","certification_required":false}]}`)
}

func publicationEnvelope(t *testing.T, eventID string, sourceVersion int64, publication Publication) events.Envelope {
	t.Helper()
	payload, err := json.Marshal(publication)
	if err != nil {
		t.Fatalf("marshal publication: %v", err)
	}
	return events.Envelope{
		EventID: eventID, EventName: EventName, SchemaVersion: SchemaVersion,
		SourceService: "admin-backoffice-service", SourceEntityType: "network_catalogue",
		SourceEntityID: "catalogue", SourceVersion: sourceVersion,
		OccurredAt: time.Unix(1_700_000_000, 0).UTC(), CorrelationID: "corr-1",
		Payload: payload,
	}
}

func inlinePublication(t *testing.T, version string) Publication {
	t.Helper()
	document := catalogue(version)
	checksum, err := Checksum(document)
	if err != nil {
		t.Fatalf("checksum: %v", err)
	}
	return Publication{
		PolicyKind: capabilityKind, PolicyVersion: version, Checksum: checksum,
		EffectiveAt: time.Unix(1_700_000_000, 0).UTC(), Document: document,
	}
}

func ingestorFor(store *memoryStore, reader DocumentReader) *Ingestor {
	return New(Options{
		Store: store, KnownKinds: []string{capabilityKind}, Reader: reader,
		Now: func() time.Time { return time.Unix(1_700_000_500, 0).UTC() },
	})
}

func TestPublishedVersionIsStored(t *testing.T) {
	store := newStore()
	result, err := ingestorFor(store, nil).Ingest(context.Background(),
		publicationEnvelope(t, "evt-1", 7, inlinePublication(t, "v3")))
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if result.Outcome != OutcomeApplied || !result.Applied {
		t.Fatalf("outcome = %s applied=%v", result.Outcome, result.Applied)
	}
	if len(store.snapshots) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(store.snapshots))
	}
	snapshot := store.snapshots[0]
	if snapshot.PolicyVersion != "v3" || snapshot.SourceVersion != 7 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	// applied_at is recorded so an operator can see when this build adopted
	// the version, which is not the same as when Admin published it.
	if snapshot.AppliedAt.IsZero() {
		t.Fatal("snapshot has no applied_at")
	}
	if !snapshot.EffectiveAt.Equal(time.Unix(1_700_000_000, 0).UTC()) {
		t.Fatalf("effective_at = %v", snapshot.EffectiveAt)
	}
}

// Delivery is at-least-once. A replay must be a no-op success, not an error
// that makes the publisher retry the same message forever.
func TestDuplicateEventIsIdempotent(t *testing.T) {
	store := newStore()
	ingestor := ingestorFor(store, nil)
	publication := inlinePublication(t, "v3")

	first, _ := ingestor.Ingest(context.Background(), publicationEnvelope(t, "evt-1", 7, publication))
	second, err := ingestor.Ingest(context.Background(), publicationEnvelope(t, "evt-1", 7, publication))
	if err != nil {
		t.Fatalf("replay errored: %v", err)
	}
	if first.Outcome != OutcomeApplied {
		t.Fatalf("first outcome = %s", first.Outcome)
	}
	if second.Outcome != OutcomeDuplicate || second.Applied {
		t.Fatalf("replay outcome = %s applied=%v", second.Outcome, second.Applied)
	}
	if len(store.snapshots) != 1 {
		t.Fatalf("replay created a second snapshot: %d", len(store.snapshots))
	}
}

// A delayed publication overtaking a newer one would silently roll the network
// back to rules it has already moved past.
func TestStaleSourceVersionIsRejected(t *testing.T) {
	store := newStore()
	ingestor := ingestorFor(store, nil)

	ingestor.Ingest(context.Background(), publicationEnvelope(t, "evt-new", 9, inlinePublication(t, "v9")))
	result, err := ingestor.Ingest(context.Background(),
		publicationEnvelope(t, "evt-old", 4, inlinePublication(t, "v4")))
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if result.Outcome != OutcomeStale || result.Applied {
		t.Fatalf("outcome = %s applied=%v", result.Outcome, result.Applied)
	}
	if len(store.snapshots) != 1 || store.snapshots[0].PolicyVersion != "v9" {
		t.Fatalf("stale publication changed the applied policy: %+v", store.snapshots)
	}
	// Equal versions are stale too: a re-publication of the same source
	// version carries no new information and must not bump anything.
	same, _ := ingestor.Ingest(context.Background(),
		publicationEnvelope(t, "evt-same", 9, inlinePublication(t, "v9-again")))
	if same.Outcome != OutcomeStale {
		t.Fatalf("equal source version = %s, want STALE", same.Outcome)
	}
}

func TestNewerPolicyBecomesAvailableAndOldSnapshotSurvives(t *testing.T) {
	store := newStore()
	ingestor := ingestorFor(store, nil)

	ingestor.Ingest(context.Background(), publicationEnvelope(t, "evt-1", 1, inlinePublication(t, "v1")))
	ingestor.Ingest(context.Background(), publicationEnvelope(t, "evt-2", 2, inlinePublication(t, "v2")))

	if len(store.snapshots) != 2 {
		t.Fatalf("snapshots = %d, want both versions kept", len(store.snapshots))
	}
	// The older snapshot is untouched: committed work pins a version, and
	// rewriting it in place would rewrite the rules a past decision was made
	// under.
	if store.snapshots[0].PolicyVersion != "v1" || store.snapshots[1].PolicyVersion != "v2" {
		t.Fatalf("snapshots = %+v", store.snapshots)
	}
	if store.applied[capabilityKind] != 2 {
		t.Fatalf("applied source version = %d, want 2", store.applied[capabilityKind])
	}
}

// A corrupted policy is worse than an absent one: the first is applied and
// believed.
func TestChecksumMismatchIsRefused(t *testing.T) {
	store := newStore()
	publication := inlinePublication(t, "v3")
	publication.Checksum = strings.Repeat("0", 64)

	result, err := ingestorFor(store, nil).Ingest(context.Background(),
		publicationEnvelope(t, "evt-1", 7, publication))
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if result.Outcome != OutcomeRejected || result.ReasonCode != ReasonChecksumMismatch {
		t.Fatalf("outcome = %s / %s", result.Outcome, result.ReasonCode)
	}
	if len(store.snapshots) != 0 {
		t.Fatal("a corrupt document was applied")
	}
}

// The checksum must survive reformatting: a publisher that emits different key
// order or whitespace is not a publisher sending a different policy.
func TestChecksumIsCanonicalNotByteExact(t *testing.T) {
	compact := json.RawMessage(`{"a":1,"b":[2,3]}`)
	reordered := json.RawMessage("{\n  \"b\": [2, 3],\n  \"a\": 1\n}")

	first, err := Checksum(compact)
	if err != nil {
		t.Fatalf("checksum: %v", err)
	}
	second, err := Checksum(reordered)
	if err != nil {
		t.Fatalf("checksum: %v", err)
	}
	if first != second {
		t.Fatalf("reformatting changed the checksum: %s vs %s", first, second)
	}
	// A genuine content change still changes it.
	changed, _ := Checksum(json.RawMessage(`{"a":2,"b":[2,3]}`))
	if changed == first {
		t.Fatal("a content change did not change the checksum")
	}
}

func TestUnsupportedSchemaVersionIsRefused(t *testing.T) {
	store := newStore()
	envelope := publicationEnvelope(t, "evt-1", 7, inlinePublication(t, "v3"))
	envelope.SchemaVersion = SchemaVersion + 1

	result, _ := ingestorFor(store, nil).Ingest(context.Background(), envelope)
	if result.Outcome != OutcomeRejected || result.ReasonCode != ReasonUnsupportedSchema {
		t.Fatalf("outcome = %s / %s", result.Outcome, result.ReasonCode)
	}
	if len(store.claimed) != 0 {
		t.Fatal("an unparseable event consumed an event id")
	}
}

// Admin broadcasts one catalogue to every authority. Taking another's kind --
// or worse, consuming its event id -- would corrupt this service's own inbox.
func TestForeignPolicyKindIsIgnoredWithoutConsumingTheEventID(t *testing.T) {
	store := newStore()
	publication := inlinePublication(t, "v3")
	publication.PolicyKind = "DISPATCH_CAPABILITY"

	result, _ := ingestorFor(store, nil).Ingest(context.Background(),
		publicationEnvelope(t, "evt-shared", 7, publication))

	if result.Outcome != OutcomeIgnored {
		t.Fatalf("outcome = %s, want IGNORED", result.Outcome)
	}
	if len(store.claimed) != 0 {
		t.Fatal("a foreign catalogue consumed this service's event id")
	}
}

// A read-back that times out has NOT failed. Recording it as a rejection would
// abandon a policy version the network has genuinely moved to.
func TestUnknownReadBackOutcomeParksForReconciliation(t *testing.T) {
	store := newStore()
	reader := &stubReader{retryable: true, err: errors.New("connection reset")}
	publication := inlinePublication(t, "v3")
	publication.Document = nil
	publication.DocumentRef = "https://admin.internal/catalogue/v3"

	result, err := ingestorFor(store, reader).Ingest(context.Background(),
		publicationEnvelope(t, "evt-1", 7, publication))
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if result.Outcome != OutcomeReconciliationRequired {
		t.Fatalf("outcome = %s, want RECONCILIATION_REQUIRED", result.Outcome)
	}
	if result.Outcome.Terminal() {
		t.Fatal("RECONCILIATION_REQUIRED reported itself terminal")
	}
	if len(store.pending) != 1 {
		t.Fatalf("pending = %d, want the publication parked", len(store.pending))
	}
	if len(store.snapshots) != 0 {
		t.Fatal("an unresolved document was applied")
	}
}

// Reconciliation asks the authority again rather than guessing, and the
// retried publication goes through exactly the same checks.
func TestReconciliationCompletesAParkedPublication(t *testing.T) {
	store := newStore()
	reader := &stubReader{retryable: true, err: errors.New("timeout")}
	ingestor := ingestorFor(store, reader)

	publication := inlinePublication(t, "v3")
	document := publication.Document
	publication.Document = nil
	publication.DocumentRef = "https://admin.internal/catalogue/v3"
	ingestor.Ingest(context.Background(), publicationEnvelope(t, "evt-1", 7, publication))

	// Admin comes back.
	reader.err, reader.retryable, reader.document = nil, false, document
	reconciled, err := ingestor.ReconcilePending(context.Background(), 10)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if reconciled != 1 {
		t.Fatalf("reconciled = %d, want 1", reconciled)
	}
	if len(store.snapshots) != 1 || store.snapshots[0].PolicyVersion != "v3" {
		t.Fatalf("snapshots = %+v", store.snapshots)
	}
	if len(store.pending) != 0 {
		t.Fatal("the publication is still parked after a successful reconcile")
	}
}

// A reconciled read-back that returns a corrupted document is still refused.
func TestReconciliationStillVerifiesChecksum(t *testing.T) {
	store := newStore()
	reader := &stubReader{retryable: true, err: errors.New("timeout")}
	ingestor := ingestorFor(store, reader)

	publication := inlinePublication(t, "v3")
	publication.Document = nil
	publication.DocumentRef = "https://admin.internal/catalogue/v3"
	ingestor.Ingest(context.Background(), publicationEnvelope(t, "evt-1", 7, publication))

	reader.err, reader.retryable = nil, false
	reader.document = json.RawMessage(`{"version":"tampered"}`)
	if _, err := ingestor.ReconcilePending(context.Background(), 10); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(store.snapshots) != 0 {
		t.Fatal("a tampered read-back was applied")
	}
	if store.terminal["evt-1"] != OutcomeRejected {
		t.Fatalf("terminal = %s, want REJECTED", store.terminal["evt-1"])
	}
}

// A definite refusal -- a 4xx, a gone document -- is terminal. Parking it
// would retry the same answer forever.
func TestNonRetryableReadBackFailureIsTerminal(t *testing.T) {
	store := newStore()
	reader := &stubReader{retryable: false, err: errors.New("404")}
	publication := inlinePublication(t, "v3")
	publication.Document = nil
	publication.DocumentRef = "https://admin.internal/catalogue/v3"

	result, _ := ingestorFor(store, reader).Ingest(context.Background(),
		publicationEnvelope(t, "evt-1", 7, publication))

	if result.Outcome != OutcomeRejected || result.ReasonCode != ReasonReadBackRefused {
		t.Fatalf("outcome = %s / %s", result.Outcome, result.ReasonCode)
	}
	if len(store.pending) != 0 {
		t.Fatal("a definite refusal was parked for retry")
	}
}

// An unconfigured reader is a deployment gap, not a bad event: wiring the
// client later must complete the publication rather than lose it.
func TestMissingReaderParksRatherThanRejects(t *testing.T) {
	store := newStore()
	publication := inlinePublication(t, "v3")
	publication.Document = nil
	publication.DocumentRef = "https://admin.internal/catalogue/v3"

	result, _ := ingestorFor(store, nil).Ingest(context.Background(),
		publicationEnvelope(t, "evt-1", 7, publication))

	if result.Outcome != OutcomeReconciliationRequired {
		t.Fatalf("outcome = %s", result.Outcome)
	}
	if len(store.pending) != 1 {
		t.Fatal("publication was not parked")
	}
}

// Policy describes what an asset may do, never what work costs.
func TestPricingFieldsAreRefusedAtAnyDepth(t *testing.T) {
	cases := []string{
		`{"products":[{"code":"X","base_fare":500}]}`,
		`{"pricing":{"per_km":120}}`,
		`{"markets":[{"zones":[{"surge":1.4}]}]}`,
		`{"products":[{"code":"X","commission":0.1}]}`,
	}
	for _, document := range cases {
		if err := RejectPricingFields(json.RawMessage(document)); err == nil {
			t.Fatalf("pricing document accepted: %s", document)
		}
	}
	if err := RejectPricingFields(catalogue("v1")); err != nil {
		t.Fatalf("a clean catalogue was refused: %v", err)
	}
}

func TestPricingFieldInPublicationIsRejectedAndNotApplied(t *testing.T) {
	store := newStore()
	document := json.RawMessage(`{"products":[{"code":"HAUL_GENERAL","base_fare":900}]}`)
	checksum, _ := Checksum(document)
	publication := Publication{
		PolicyKind: capabilityKind, PolicyVersion: "v3", Checksum: checksum,
		EffectiveAt: time.Unix(1_700_000_000, 0).UTC(), Document: document,
	}

	result, _ := ingestorFor(store, nil).Ingest(context.Background(),
		publicationEnvelope(t, "evt-1", 7, publication))

	if result.Outcome != OutcomeRejected || result.ReasonCode != ReasonPricingFieldRejected {
		t.Fatalf("outcome = %s / %s", result.Outcome, result.ReasonCode)
	}
	if len(store.snapshots) != 0 {
		t.Fatal("a pricing-bearing policy was applied")
	}
}

func TestPublicationValidation(t *testing.T) {
	valid := inlinePublication(t, "v1")
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid publication refused: %v", err)
	}

	cases := map[string]func(*Publication){
		"no kind":      func(p *Publication) { p.PolicyKind = "" },
		"no version":   func(p *Publication) { p.PolicyVersion = "" },
		"no checksum":  func(p *Publication) { p.Checksum = "" },
		"no effective": func(p *Publication) { p.EffectiveAt = time.Time{} },
		"no document":  func(p *Publication) { p.Document = nil; p.DocumentRef = "" },
		"relative ref": func(p *Publication) { p.Document = nil; p.DocumentRef = "/catalogue/v3" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			publication := inlinePublication(t, "v1")
			mutate(&publication)
			if err := publication.Validate(); err == nil {
				t.Fatal("invalid publication accepted")
			}
		})
	}
}

// A store failure must surface as an error, never as a silently swallowed
// no-op that would make the publisher believe the policy landed.
func TestStoreFailureSurfaces(t *testing.T) {
	store := newStore()
	store.applyErr = errors.New("postgres unavailable")

	_, err := ingestorFor(store, nil).Ingest(context.Background(),
		publicationEnvelope(t, "evt-1", 7, inlinePublication(t, "v3")))
	if err == nil {
		t.Fatal("a store failure was reported as success")
	}
}

// ── Read-back client ──────────────────────────────────────────────────────────

func TestHTTPReaderRequiresAnAllowlist(t *testing.T) {
	if NewHTTPReader(ReaderOptions{CallerName: "svc", Secret: []byte("s")}) != nil {
		t.Fatal("a reader with no host allowlist was constructed")
	}
	if NewHTTPReader(ReaderOptions{Secret: []byte("s"), AllowedHosts: []string{"admin"}}) != nil {
		t.Fatal("a reader with no caller name was constructed")
	}
}

// A publication must not be able to point this service at an arbitrary host
// and have it sign an authenticated request there.
func TestHTTPReaderRefusesAnUnlistedHost(t *testing.T) {
	reader := NewHTTPReader(ReaderOptions{
		CallerName: "driver-hauling-service", Secret: []byte("secret"),
		AllowedHosts: []string{"admin.internal"},
	})
	_, retryable, err := reader.ReadDocument(context.Background(), "https://evil.example/catalogue")
	if err == nil {
		t.Fatal("an unlisted host was fetched")
	}
	if retryable {
		t.Fatal("a host-allowlist refusal was marked retryable; it would be retried forever")
	}
}

func TestHTTPReaderSignsAndReadsBack(t *testing.T) {
	var sawSignature, sawService bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawSignature = r.Header.Get("X-Service-Signature") != ""
		sawService = r.Header.Get("X-Service-Name") == "driver-hauling-service"
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(catalogue("v3"))
	}))
	defer server.Close()

	reader := NewHTTPReader(ReaderOptions{
		CallerName: "driver-hauling-service", Secret: []byte("secret"),
		AllowedHosts: []string{"127.0.0.1"},
	})
	document, _, err := reader.ReadDocument(context.Background(), server.URL+"/catalogue/v3")
	if err != nil {
		t.Fatalf("read-back: %v", err)
	}
	if !sawSignature || !sawService {
		t.Fatalf("read-back was not signed: signature=%v service=%v", sawSignature, sawService)
	}
	if string(document) != string(catalogue("v3")) {
		t.Fatalf("document = %s", document)
	}
}

// 5xx and 429 might have been served; a 4xx is a decided refusal.
func TestHTTPReaderClassifiesStatusCodes(t *testing.T) {
	cases := []struct {
		status        int
		wantRetryable bool
	}{
		{http.StatusInternalServerError, true},
		{http.StatusServiceUnavailable, true},
		{http.StatusTooManyRequests, true},
		{http.StatusUnauthorized, false},
		{http.StatusNotFound, false},
	}
	for _, tc := range cases {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
		}))
		reader := NewHTTPReader(ReaderOptions{
			CallerName: "svc", Secret: []byte("secret"), AllowedHosts: []string{"127.0.0.1"},
		})
		_, retryable, err := reader.ReadDocument(context.Background(), server.URL+"/c")
		server.Close()
		if err == nil {
			t.Fatalf("status %d was accepted", tc.status)
		}
		if retryable != tc.wantRetryable {
			t.Fatalf("status %d retryable = %v, want %v", tc.status, retryable, tc.wantRetryable)
		}
	}
}

func TestHTTPReaderRefusesNonJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>not a catalogue</html>"))
	}))
	defer server.Close()

	reader := NewHTTPReader(ReaderOptions{
		CallerName: "svc", Secret: []byte("secret"), AllowedHosts: []string{"127.0.0.1"},
	})
	_, retryable, err := reader.ReadDocument(context.Background(), server.URL+"/c")
	if err == nil {
		t.Fatal("a non-JSON body was accepted as a catalogue")
	}
	if retryable {
		t.Fatal("a non-JSON body was marked retryable")
	}
}
