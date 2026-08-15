package crm

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// Vendor to Media contract test.
//
// The store below is not a permissive stub: it re-implements the rules
// media-file-service actually enforces on the DATA_EXPORT purpose — private
// only, exact type and extension pairing, a size ceiling, no signature-bearing
// or markup content, and real structural parsing of CSV and JSONL. That makes
// this a contract test rather than a mock: if the Vendor generator ever emits
// something the real store would refuse, this fails here instead of in
// production.
//
// It deliberately does not reach a network service. The behaviour under test is
// Vendor's — what it generates, what it sends, when it marks an export READY —
// and the media service proves its own half against real PostgreSQL in its own
// integration suite.

// mediaContractStore mirrors the media DATA_EXPORT gate.
type mediaContractStore struct {
	mu        sync.Mutex
	objects   map[string][]byte
	names     map[string]string
	types     map[string]string
	deleted   map[string]string
	refusals  []string
	maxBytes  int64
	failNext  bool
	grantTTL  time.Duration
	putCalls  int
	accessLog []string
}

func newMediaContractStore() *mediaContractStore {
	return &mediaContractStore{
		objects: map[string][]byte{}, names: map[string]string{}, types: map[string]string{},
		deleted: map[string]string{}, maxBytes: 20 * 1024 * 1024, grantTTL: 5 * time.Minute,
	}
}

// canonicalExportType is the media service's exact allowlist.
func canonicalExportType(declared string) string {
	switch strings.ToLower(strings.TrimSpace(declared)) {
	case "text/csv":
		return "text/csv"
	case "application/x-ndjson":
		return "application/x-ndjson"
	}
	return ""
}

func (m *mediaContractStore) refuse(reason string) error {
	m.refusals = append(m.refusals, reason)
	return fmt.Errorf("media refused the artifact: %s", reason)
}

func (m *mediaContractStore) Put(ctx context.Context, in PutArtifactInput) (StoredArtifact, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.putCalls++
	if m.failNext {
		return StoredArtifact{}, m.refuse("STORAGE_UNAVAILABLE")
	}

	// 1. Exactly two content types, and the extension must pair with the one
	//    declared. Vendor must send both, and they must agree.
	canonical := canonicalExportType(in.MediaType)
	if canonical == "" {
		return StoredArtifact{}, m.refuse("EXPORT_UNSUPPORTED_TYPE")
	}
	wantExtension := ".csv"
	if canonical == "application/x-ndjson" {
		wantExtension = ".jsonl"
	}
	if in.Filename == "" || strings.ContainsAny(in.Filename, `/\`) ||
		!strings.HasSuffix(strings.ToLower(in.Filename), wantExtension) {
		return StoredArtifact{}, m.refuse("EXPORT_EXTENSION_MISMATCH")
	}
	// 2. Bounded.
	if int64(len(in.Content)) > m.maxBytes {
		return StoredArtifact{}, m.refuse("EXPORT_SIZE_EXCEEDED")
	}
	if len(in.Content) == 0 {
		return StoredArtifact{}, m.refuse("EXPORT_EMPTY")
	}
	// 3. No signature-bearing content and no markup: an export never opens
	//    with a magic number or a `<`.
	head := in.Content
	if len(head) > 512 {
		head = head[:512]
	}
	for _, signature := range [][]byte{
		[]byte("%PDF-"), {0x89, 'P', 'N', 'G'}, {0xFF, 0xD8, 0xFF},
		[]byte("PK\x03\x04"), {0x7F, 'E', 'L', 'F'}, []byte("#!"), []byte("MZ"),
	} {
		if bytes.HasPrefix(head, signature) {
			return StoredArtifact{}, m.refuse("EXPORT_SIGNATURE_CONTENT")
		}
	}
	if trimmed := bytes.TrimLeft(head, " \t\r\n"); len(trimmed) > 0 && trimmed[0] == '<' {
		return StoredArtifact{}, m.refuse("EXPORT_UNSUPPORTED_TYPE")
	}
	// 4. Printable UTF-8 only.
	for _, b := range in.Content {
		if b == 0 || (b < 0x20 && b != 0x09 && b != 0x0A && b != 0x0D) {
			return StoredArtifact{}, m.refuse("EXPORT_CONTROL_BYTES")
		}
	}
	// 5. Structurally the format it claims to be.
	if canonical == "text/csv" {
		reader := csv.NewReader(bytes.NewReader(in.Content))
		rows := 0
		for {
			if _, err := reader.Read(); err != nil {
				if err.Error() == "EOF" {
					break
				}
				return StoredArtifact{}, m.refuse("EXPORT_MALFORMED_STRUCTURE")
			}
			rows++
		}
		if rows == 0 {
			return StoredArtifact{}, m.refuse("EXPORT_EMPTY")
		}
	} else {
		scanner := bufio.NewScanner(bytes.NewReader(in.Content))
		scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
		records := 0
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			if line[0] != '{' || !json.Valid([]byte(line)) {
				return StoredArtifact{}, m.refuse("EXPORT_MALFORMED_STRUCTURE")
			}
			records++
		}
		if scanner.Err() != nil || records == 0 {
			return StoredArtifact{}, m.refuse("EXPORT_MALFORMED_STRUCTURE")
		}
	}

	// Idempotent on the Vendor-supplied key, exactly as the real contract is.
	if in.IdempotencyKey == "" {
		return StoredArtifact{}, m.refuse("IDEMPOTENCY_KEY_REQUIRED")
	}
	ref := "media_" + in.IdempotencyKey
	m.objects[ref] = append([]byte(nil), in.Content...)
	m.names[ref] = in.Filename
	m.types[ref] = canonical
	delete(m.deleted, ref)
	sum := sha256.Sum256(in.Content)
	return StoredArtifact{Ref: ref, Bytes: int64(len(in.Content)), Checksum: hex.EncodeToString(sum[:])}, nil
}

func (m *mediaContractStore) Access(ctx context.Context, businessID, ref string) (ArtifactAccess, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	content, ok := m.objects[ref]
	if !ok {
		return ArtifactAccess{}, fmt.Errorf("no such object")
	}
	m.accessLog = append(m.accessLog, ref)
	// Short-lived, minted per request, never persisted.
	return ArtifactAccess{
		Ref: ref, Path: "/api/v1/media-files/internal/media/objects/" + ref + "/content",
		Token:     "dl_" + hex.EncodeToString([]byte(ref))[:16],
		ExpiresAt: time.Now().UTC().Add(m.grantTTL),
		MediaType: m.types[ref], Bytes: int64(len(content)),
	}, nil
}

func (m *mediaContractStore) Delete(ctx context.Context, businessID, ref, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, ref)
	m.deleted[ref] = reason
	return nil
}

func (m *mediaContractStore) held(ref string) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	content, ok := m.objects[ref]
	return content, ok
}

// TestVendorMediaContract_ExportRoundTripsThroughThePrivateStore is the
// end-to-end contract: generate, checksum, upload as DATA_EXPORT, receive a
// reference, become READY only after the artifact is safely held, and release
// only a short-lived grant.
func TestVendorMediaContract_ExportRoundTripsThroughThePrivateStore(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	store := newMediaContractStore()
	stepUp := newFakeStepUp()
	env.service.artifacts = store
	env.service.stepUp = stepUp
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	env.seedExportFixture(t)

	for _, format := range []string{FormatCSV, FormatJSONL} {
		export := env.createExport(t, stepUp, CreateExportInput{
			ExportType: ExportCustomerSummary, Format: format,
			ScopeType: IntelligenceScopeBusiness})
		env.generateExports(t)
		ready := env.reloadExport(t, export.ID)
		if ready.Status != ExportReady {
			t.Fatalf("%s export finished as %s (error=%v); media refusals=%v",
				format, ready.Status, ready.ErrorCode, store.refusals)
		}

		// The reference is an opaque store handle; no URL is persisted.
		if ready.ArtifactRef == nil || strings.Contains(*ready.ArtifactRef, "http") {
			t.Fatalf("%s export stored a URL rather than a reference: %v", format, ready.ArtifactRef)
		}
		// The checksum Vendor recorded describes the bytes the store holds.
		content, held := store.held(*ready.ArtifactRef)
		if !held {
			t.Fatalf("%s export is READY but the store holds nothing", format)
		}
		sum := sha256.Sum256(content)
		if ready.Checksum == nil || *ready.Checksum != hex.EncodeToString(sum[:]) {
			t.Fatalf("%s export checksum does not match the stored artifact", format)
		}
		if ready.ArtifactBytes == nil || *ready.ArtifactBytes != int64(len(content)) {
			t.Fatalf("%s export size does not match the stored artifact", format)
		}
		// The file name the store validated carries the right extension.
		store.mu.Lock()
		name := store.names[*ready.ArtifactRef]
		store.mu.Unlock()
		wantExtension := ".csv"
		if format == FormatJSONL {
			wantExtension = ".jsonl"
		}
		if !strings.HasSuffix(name, wantExtension) {
			t.Fatalf("%s export was named %q", format, name)
		}

		// Download releases a short-lived grant and nothing durable.
		stepUp.issue("grant_dl_"+format, StepUpPurposeExportDownload)
		access, _, err := env.service.AuthorizeExportDownload(ctx, actor, globalScope(), export.ID,
			ExportAccessInput{StepUpGrantID: "grant_dl_" + format, SessionID: "s"})
		if err != nil {
			t.Fatalf("%s download: %v", format, err)
		}
		if access.Token == "" || !access.ExpiresAt.After(time.Now()) {
			t.Fatalf("%s access grant is unusable: %+v", format, access)
		}
		if access.ExpiresAt.After(time.Now().Add(time.Hour)) {
			t.Fatalf("%s access grant lives too long: %v", format, access.ExpiresAt)
		}
		if strings.HasPrefix(access.Path, "http") {
			t.Fatalf("%s access returned an absolute URL: %s", format, access.Path)
		}
	}
	if len(store.refusals) != 0 {
		t.Fatalf("the store refused a generated export: %v", store.refusals)
	}
}

// TestVendorMediaContract_StorageFailureNeverProducesAReadyExport is the
// failure-safety rule: if the artifact cannot be held, no downloadable export
// exists.
func TestVendorMediaContract_StorageFailureNeverProducesAReadyExport(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	store := newMediaContractStore()
	stepUp := newFakeStepUp()
	env.service.artifacts = store
	env.service.stepUp = stepUp
	ctx := context.Background()
	env.seedExportFixture(t)

	store.failNext = true
	export := env.createExport(t, stepUp, CreateExportInput{
		ExportType: ExportCustomerSummary, Format: FormatCSV, ScopeType: IntelligenceScopeBusiness})
	env.generateExports(t)
	blocked := env.reloadExport(t, export.ID)
	if blocked.Status == ExportReady {
		t.Fatal("an export became READY while the store refused it")
	}
	if blocked.ArtifactRef != nil {
		t.Fatalf("a failed export references an artifact: %+v", blocked)
	}
	stepUp.issue("grant_blocked", StepUpPurposeExportDownload)
	if _, _, err := env.service.AuthorizeExportDownload(ctx, actorFor(businessA, "vs_a"),
		globalScope(), export.ID,
		ExportAccessInput{StepUpGrantID: "grant_blocked", SessionID: "s"}); err == nil {
		t.Fatal("an export with no artifact was downloadable")
	}

	// Once storage recovers the retry succeeds, and the stable upload identity
	// means the retry does not leave a second artifact behind.
	store.failNext = false
	if _, err := env.pool.Exec(ctx, `UPDATE crm_exports SET next_attempt_at=now() WHERE id=$1`,
		export.ID); err != nil {
		t.Fatal(err)
	}
	env.generateExports(t)
	recovered := env.reloadExport(t, export.ID)
	if recovered.Status != ExportReady {
		t.Fatalf("the retried export finished as %s (error=%v)", recovered.Status, recovered.ErrorCode)
	}
	store.mu.Lock()
	objects := len(store.objects)
	store.mu.Unlock()
	if objects != 1 {
		t.Fatalf("the retry left %d artifacts; the upload identity must be stable", objects)
	}
}

// TestVendorMediaContract_ExpiredExportCannotObtainAccess closes the retrieval
// loop against the contract store.
func TestVendorMediaContract_ExpiredExportCannotObtainAccess(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	store := newMediaContractStore()
	stepUp := newFakeStepUp()
	env.service.artifacts = store
	env.service.stepUp = stepUp
	ctx := context.Background()
	env.seedExportFixture(t)

	export := env.createExport(t, stepUp, CreateExportInput{
		ExportType: ExportIntelligenceScores, Format: FormatJSONL,
		ScopeType: IntelligenceScopeBusiness})
	env.generateExports(t)
	ready := env.reloadExport(t, export.ID)
	if ready.Status != ExportReady {
		t.Fatalf("export finished as %s; refusals=%v", ready.Status, store.refusals)
	}
	ref := *ready.ArtifactRef

	if _, err := env.pool.Exec(ctx, `UPDATE crm_exports
		SET completed_at=now() - interval '30 hours', expires_at=now() - interval '6 hours'
		WHERE id=$1`, export.ID); err != nil {
		t.Fatal(err)
	}
	stepUp.issue("grant_expired", StepUpPurposeExportDownload)
	if _, _, err := env.service.AuthorizeExportDownload(ctx, actorFor(businessA, "vs_a"),
		globalScope(), export.ID,
		ExportAccessInput{StepUpGrantID: "grant_expired", SessionID: "s"}); codeOf(t, err) != CodeExportExpired {
		t.Fatalf("an expired export obtained access (err=%v)", err)
	}
	// And the artifact was revoked in the store rather than merely marked.
	if _, held := store.held(ref); held {
		t.Fatal("the store still holds an expired artifact")
	}
	store.mu.Lock()
	reason := store.deleted[ref]
	store.mu.Unlock()
	if reason != "expired" {
		t.Fatalf("the artifact was deleted for %q; want expired", reason)
	}
}
