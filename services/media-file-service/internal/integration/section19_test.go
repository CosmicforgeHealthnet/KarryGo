package integration_test

// Section 19 - media, documents and evidence hardening for Secure Admin.
//
// Every test here runs against real PostgreSQL (MEDIA_FILE_TEST_DATABASE_URL)
// and the real handlers. The scanners below are TEST_ONLY verdict sources:
// they isolate Media's fail-closed decisions and are never a substitute for
// the external ClamAV production verification.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	filemetadatarepositories "cosmicforge/logistics/services/media-file-service/internal/features/file_metadata/repositories"
	mediahttp "cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/http"
	mediamodels "cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/models"
	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/usecases"
	uploadclients "cosmicforge/logistics/services/media-file-service/internal/features/uploads/clients"
	uploadhttp "cosmicforge/logistics/services/media-file-service/internal/features/uploads/http"
	uploadusecases "cosmicforge/logistics/services/media-file-service/internal/features/uploads/usecases"
	"cosmicforge/logistics/shared/go/admincase"
	"cosmicforge/logistics/shared/go/adminmedia"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// --- legacy harness ----------------------------------------------------------

// dirObjectStorage is a real filesystem implementation of the legacy storage
// port, laid out exactly like fileLegacyContent so adoption reads the same
// bytes the legacy routes serve.
type dirObjectStorage struct{ dir string }

func (s dirObjectStorage) full(p string) string { return filepath.Join(s.dir, filepath.Clean("/"+p)) }
func (s dirObjectStorage) Upload(_ context.Context, in uploadclients.UploadObjectInput) (uploadclients.StorageObject, error) {
	if err := os.MkdirAll(filepath.Dir(s.full(in.Path)), 0o700); err != nil {
		return uploadclients.StorageObject{}, err
	}
	raw, err := io.ReadAll(in.Body)
	if err != nil {
		return uploadclients.StorageObject{}, err
	}
	if err := os.WriteFile(s.full(in.Path), raw, 0o600); err != nil {
		return uploadclients.StorageObject{}, err
	}
	return uploadclients.StorageObject{Bucket: "test-legacy", Path: in.Path}, nil
}
func (s dirObjectStorage) Download(_ context.Context, p string) (io.ReadCloser, error) {
	return os.Open(s.full(p))
}
func (s dirObjectStorage) Delete(_ context.Context, p string) error { return os.Remove(s.full(p)) }
func (s dirObjectStorage) Check(context.Context) error              { return nil }

// TEST_ONLY legacy verdict sources.
type legacyVerdict struct {
	safe  bool
	err   error
	calls *int32
}

func (v legacyVerdict) ScanLegacy(context.Context, string, []byte) (bool, error) {
	if v.calls != nil {
		atomic.AddInt32(v.calls, 1)
	}
	return v.safe, v.err
}

type legacyEnv struct {
	f      *mediaFixture
	dir    string
	router *gin.Engine
}

const (
	dispatchLegacy = "dispatch-delivery-service"
	dispatchToken  = "test-dispatch-token"
	customerLegacy = "customer-service"
	customerToken  = "test-customer-token"
)

func newLegacyEnv(t *testing.T, f *mediaFixture, scanner uploadusecases.LegacyScanner, publicSensitive bool, dir string) *legacyEnv {
	t.Helper()
	if dir == "" {
		dir = t.TempDir()
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpx.RequestID(), httpx.ErrorHandler())
	svc := uploadusecases.NewUploadService(uploadusecases.Options{
		Storage:                 dirObjectStorage{dir: dir},
		Assets:                  filemetadatarepositories.NewPostgresMediaAssetRepository(f.pool),
		MaxUploadBytes:          1 << 20,
		Governance:              filemetadatarepositories.NewPostgresLegacyGovernance(f.pool),
		Scanner:                 scanner,
		PublicSensitiveDownload: publicSensitive,
	})
	uploadhttp.RegisterUploadRoutes(router.Group("/api/v1/media-files"), svc,
		map[string]string{dispatchLegacy: dispatchToken, customerLegacy: customerToken}, 1<<20, "")
	return &legacyEnv{f: f, dir: dir, router: router}
}

func (e *legacyEnv) do(t *testing.T, method, path, service, token string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "/api/v1/media-files"+path, nil)
	if service != "" {
		r.Header.Set("X-Karrygo-Service", service)
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, r)
	return w
}

func (e *legacyEnv) upload(t *testing.T, service, token, ownerID, purpose, filename, contentType string, content []byte) (int, map[string]any) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for k, v := range map[string]string{"owner_service": service, "owner_id": ownerID, "purpose": purpose} {
		if err := writer.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	part.Write(content)
	writer.Close()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/media-files/uploads", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	r.Header.Set("X-Karrygo-Service", service)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, r)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func (e *legacyEnv) uploadID(t *testing.T, service, token, ownerID, purpose, filename, contentType string, content []byte) string {
	t.Helper()
	status, out := e.upload(t, service, token, ownerID, purpose, filename, contentType, content)
	if status != http.StatusCreated {
		t.Fatalf("upload %s: %d %v", purpose, status, out)
	}
	return dataOf(t, out)["id"].(string)
}

func jpegFile() []byte {
	return append([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}, bytes.Repeat([]byte{0x01}, 64)...)
}

func assertSensitiveHeaders(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("missing nosniff/no-store: %v", w.Header())
	}
}

// --- 1. public legacy download ---------------------------------------------

func TestSection19PublicLegacyDownloadServesOnlyProfilePhotos(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{})
	e := newLegacyEnv(t, f, legacyVerdict{safe: true}, false, "")
	photo := e.uploadID(t, customerLegacy, customerToken, "cust-1", "profile_photo", "me.jpg", "image/jpeg", jpegFile())
	sensitive := map[string]string{
		"document_file": e.uploadID(t, dispatchLegacy, dispatchToken, "rider-1", "document_file", "licence.pdf", "application/pdf", pdfFile(32)),
		"proof_image":   e.uploadID(t, dispatchLegacy, dispatchToken, "trip-1", "proof_image", "proof.jpg", "image/jpeg", jpegFile()),
		"signature":     e.uploadID(t, dispatchLegacy, dispatchToken, "trip-1", "signature", "sig.png", "image/png", pngFile()),
	}

	w := e.do(t, http.MethodGet, "/"+photo+"/download", "", "")
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), jpegFile()) || w.Header().Get("Content-Disposition") != "inline" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("profile photo no longer public: %d %v", w.Code, w.Header())
	}
	for purpose, id := range sensitive {
		w := e.do(t, http.MethodGet, "/"+id+"/download", "", "")
		if w.Code != http.StatusNotFound || w.Body.Len() > 0 && bytes.Contains(w.Body.Bytes(), []byte("PDF")) {
			t.Fatalf("%s publicly downloadable: %d", purpose, w.Code)
		}
		assertSensitiveHeaders(t, w)
		// The reference handed to the owner is the authenticated route, not
		// a public link.
		var url string
		f.pool.QueryRow(context.Background(), `SELECT public_url FROM media_assets WHERE id=$1::uuid`, id).Scan(&url)
		if url != "/files/"+id+"/content" {
			t.Fatalf("%s reference %q", purpose, url)
		}
	}

	// The compatibility flag re-opens the route, but never inline, never
	// cached, and only with a SAFE scan.
	open := newLegacyEnv(t, f, legacyVerdict{safe: true}, true, e.dir)
	w = open.do(t, http.MethodGet, "/"+sensitive["document_file"]+"/download", "", "")
	if w.Code != 200 || w.Header().Get("Content-Disposition") != "attachment" {
		t.Fatalf("flagged legacy download: %d %v", w.Code, w.Header())
	}
	assertSensitiveHeaders(t, w)
	unscanned := newLegacyEnv(t, f, nil, true, e.dir)
	if w := unscanned.do(t, http.MethodGet, "/"+sensitive["proof_image"]+"/download", "", ""); w.Code != http.StatusForbidden {
		t.Fatalf("flag without scanner served sensitive bytes: %d", w.Code)
	}
}

// --- 2. owner route, copied links, scanner fail-closed, quarantine ---------

func TestSection19LegacyOwnerRouteFailsClosedAndRefusesCopiedLinks(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{})
	e := newLegacyEnv(t, f, legacyVerdict{safe: true}, false, "")
	doc := e.uploadID(t, dispatchLegacy, dispatchToken, "rider-1", "document_file", "licence.pdf", "application/pdf", pdfFile(32))

	if w := e.do(t, http.MethodGet, "/files/"+doc+"/content", "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous owner route: %d", w.Code)
	}
	// A link copied to an unrelated (authenticated) service is refused.
	if w := e.do(t, http.MethodGet, "/files/"+doc+"/content", customerLegacy, customerToken); w.Code != http.StatusForbidden {
		t.Fatalf("copied link served to unrelated caller: %d", w.Code)
	}
	w := e.do(t, http.MethodGet, "/files/"+doc+"/content", dispatchLegacy, dispatchToken)
	if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), pdfFile(32)) || w.Header().Get("Content-Disposition") != "attachment" {
		t.Fatalf("owner read: %d %v", w.Code, w.Header())
	}
	assertSensitiveHeaders(t, w)

	for name, scanner := range map[string]uploadusecases.LegacyScanner{
		"no scanner":          nil,
		"scanner unavailable": legacyVerdict{err: errors.New("TEST_ONLY scanner unreachable")},
	} {
		env := newLegacyEnv(t, f, scanner, false, e.dir)
		if w := env.do(t, http.MethodGet, "/files/"+doc+"/content", dispatchLegacy, dispatchToken); w.Code != http.StatusForbidden || w.Body.Len() > 0 && bytes.Contains(w.Body.Bytes(), []byte("%PDF")) {
			t.Fatalf("%s did not fail closed: %d", name, w.Code)
		}
	}
	infected := newLegacyEnv(t, f, legacyVerdict{safe: false}, false, e.dir)
	if w := infected.do(t, http.MethodGet, "/files/"+doc+"/content", dispatchLegacy, dispatchToken); w.Code != http.StatusForbidden {
		t.Fatalf("malicious legacy file served: %d", w.Code)
	}
	var status string
	f.pool.QueryRow(context.Background(), `SELECT status FROM media_assets WHERE id=$1::uuid`, doc).Scan(&status)
	if status != "quarantined" {
		t.Fatalf("malicious legacy file not quarantined: %s", status)
	}
	if w := e.do(t, http.MethodGet, "/files/"+doc+"/content", dispatchLegacy, dispatchToken); w.Code != http.StatusNotFound {
		t.Fatalf("quarantined legacy file served after quarantine: %d", w.Code)
	}
}

// --- 3. SVG, path traversal, MIME/length substitution ----------------------

func TestSection19LegacyUploadRejectsSVGAndPathManipulation(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{})
	e := newLegacyEnv(t, f, legacyVerdict{safe: true}, false, "")
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	if status, _ := e.upload(t, dispatchLegacy, dispatchToken, "trip-1", "signature", "sig.svg", "image/svg+xml", svg); status != http.StatusUnprocessableEntity {
		t.Fatalf("SVG signature accepted: %d", status)
	}
	for _, owner := range []string{"../../etc", "..", "a/b", `a\b`, "trip 1", "x/../../y", ".hidden", "trip%2F..", strings.Repeat("a", 129)} {
		if status, _ := e.upload(t, dispatchLegacy, dispatchToken, owner, "proof_image", "p.jpg", "image/jpeg", jpegFile()); status != http.StatusUnprocessableEntity {
			t.Fatalf("owner id %q accepted: %d", owner, status)
		}
	}
	var count int
	f.pool.QueryRow(context.Background(), `SELECT count(*) FROM media_assets`).Scan(&count)
	if count != 0 {
		t.Fatalf("rejected uploads persisted %d rows", count)
	}
	// Nothing escaped the storage root.
	entries, _ := os.ReadDir(filepath.Dir(e.dir))
	for _, entry := range entries {
		if entry.Name() == "etc" || entry.Name() == "y" {
			t.Fatal("upload escaped the storage root")
		}
	}
	// A legitimate opaque owner id still works and stays inside its segment.
	id := e.uploadID(t, dispatchLegacy, dispatchToken, "trip:7f3a", "proof_image", "../../p.jpg", "image/jpeg", jpegFile())
	var path string
	f.pool.QueryRow(context.Background(), `SELECT storage_path FROM media_assets WHERE id=$1::uuid`, id).Scan(&path)
	if path != "media/"+dispatchLegacy+"/proof_image/trip:7f3a/"+id+"/p.jpg" {
		t.Fatalf("storage key manipulated: %s", path)
	}

	// A pre-existing legacy SVG signature is never rendered.
	legacySVG := insertRawLegacy(t, f, e.dir, dispatchLegacy, "trip-1", "signature", "image/svg+xml", svg, len(svg))
	w := e.do(t, http.MethodGet, "/files/"+legacySVG+"/content", dispatchLegacy, dispatchToken)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/octet-stream" || w.Header().Get("Content-Disposition") != "attachment" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("legacy SVG served renderable: %d %v", w.Code, w.Header())
	}
	// MIME substitution: HTML bytes recorded as a JPEG profile photo.
	html := []byte("<html><script>alert(1)</script></html>")
	disguised := insertRawLegacy(t, f, e.dir, customerLegacy, "cust-1", "profile_photo", "image/jpeg", html, len(html))
	if w := e.do(t, http.MethodGet, "/"+disguised+"/download", "", ""); w.Code != http.StatusConflict {
		t.Fatalf("MIME-mismatched legacy file served: %d", w.Code)
	}
	// Content-length mismatch: recorded size differs from the stored bytes.
	short := insertRawLegacy(t, f, e.dir, customerLegacy, "cust-1", "profile_photo", "image/jpeg", jpegFile(), len(jpegFile())-1)
	if w := e.do(t, http.MethodGet, "/"+short+"/download", "", ""); w.Code != http.StatusConflict {
		t.Fatalf("length-mismatched legacy file served: %d", w.Code)
	}
}

func insertRawLegacy(t *testing.T, f *mediaFixture, dir, ownerService, ownerID, purpose, contentType string, content []byte, recordedSize int) string {
	t.Helper()
	id := uuid.NewString()
	path := "legacy/" + id
	if err := os.MkdirAll(filepath.Join(dir, "legacy"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, path), content, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO media_assets(id,owner_service,owner_id,purpose,original_filename,content_type,size_bytes,checksum_sha256,storage_bucket,storage_path,public_url)VALUES($1::uuid,$2,$3,$4,'file',$5,$6,$7,'test-legacy',$8,'')`, id, ownerService, ownerID, purpose, contentType, recordedSize, hex.EncodeToString(sum[:]), path); err != nil {
		t.Fatal(err)
	}
	return id
}

// --- 4. adoption: legacy serving stops, private mapping lookup -------------

func TestSection19AdoptedLegacyAssetStopsServingAndMappingIsPrivate(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{scanner: cleanScanner{}})
	ctx := context.Background()
	dir := t.TempDir()
	owner := "driver-dispatch-delivery-service"
	spec, _ := admincase.Find(owner, "delivery-recoveries")
	adoption := f.service.NewLegacyAdoption(usecases.LegacyAdoptionConfig{Content: fileLegacyContent{dir}, Bucket: "test-legacy"})
	mission, trip, recovery := "mission-"+uuid.NewString(), uuid.NewString(), uuid.NewString()
	payload := pdfFile(96)
	legacyID := insertLegacyAsset(t, f, dir, dispatchLegacy, mission, "proof_image", "active", payload)
	// Before adoption, the compatibility flag would still serve it.
	e := newLegacyEnv(t, f, legacyVerdict{safe: true}, true, dir)
	if w := e.do(t, http.MethodGet, "/"+legacyID+"/download", "", ""); w.Code != 200 {
		t.Fatalf("pre-adoption flagged read: %d", w.Code)
	}
	attest := adminmedia.LegacyAdoption{ContractVersion: adminmedia.LegacyAdoptionContract, LegacyAssetID: legacyID, LegacyOwnerReference: mission, OrganizationID: testOrgID, CaseKind: spec.Kind, CaseReference: recovery, SubjectType: "TRIP", SubjectReference: trip, Purpose: "CARGO_CONDITION"}
	receipt, err := adoption.Adopt(ctx, owner, attest)
	if err != nil {
		t.Fatal("adopt", err)
	}
	// After adoption no legacy route serves the bytes, flag or not.
	for _, path := range []string{"/" + legacyID + "/download", "/files/" + legacyID + "/content"} {
		w := e.do(t, http.MethodGet, path, dispatchLegacy, dispatchToken)
		if w.Code != http.StatusGone || bytes.Contains(w.Body.Bytes(), []byte("%PDF")) || bytes.Contains(w.Body.Bytes(), []byte(receipt.MediaObjectID)) {
			t.Fatalf("adopted legacy asset served at %s: %d %s", path, w.Code, w.Body.String())
		}
	}

	// document_file and signature legacy assets have no authoritative case
	// association: adoption is refused and nothing is mapped.
	for _, purpose := range []string{"document_file", "signature", "profile_photo"} {
		a := attest
		a.LegacyAssetID = insertLegacyAsset(t, f, dir, dispatchLegacy, mission, purpose, "active", payload)
		if _, err := adoption.Adopt(ctx, owner, a); !errors.Is(err, usecases.ErrLegacyDenied) {
			t.Fatalf("%s adopted: %v", purpose, err)
		}
	}

	// Private lookup: Bridge caller only, durable nonce, flag-gated.
	key := bytes.Repeat([]byte("m"), 32)
	router := gin.New()
	mediahttp.NewLegacyMappingHandler(f.service.NewLegacyAdoption(usecases.LegacyAdoptionConfig{}), f.pool, key, true).Register(router)
	server := httptest.NewServer(router)
	defer server.Close()
	client, err := adminmedia.NewClient(server.URL, key)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	mapping, err := client.LegacyMapping(ctx, legacyID)
	if err != nil || mapping.MediaObjectID != receipt.MediaObjectID || mapping.AttestationDigest != receipt.AttestationDigest {
		t.Fatal("mapping lookup", err, mapping)
	}
	raw, _ := json.Marshal(mapping)
	for _, leak := range []string{"case", "subject", "url", "checksum", "organization", "%PDF"} {
		if strings.Contains(strings.ToLower(string(raw)), strings.ToLower(leak)) {
			t.Fatalf("mapping disclosed %q: %s", leak, raw)
		}
	}
	if _, err := client.LegacyMapping(ctx, uuid.NewString()); !errors.Is(err, adminmedia.ErrNotFound) {
		t.Fatal("unadopted legacy id resolved", err)
	}
	wrong, _ := adminmedia.NewClient(server.URL, bytes.Repeat([]byte("x"), 32))
	if _, err := wrong.LegacyMapping(ctx, legacyID); !errors.Is(err, adminmedia.ErrDenied) {
		t.Fatal("foreign key resolved mapping", err)
	}
	wrong.Close()
	// An owner persona with the right key is still not the Bridge.
	body, _ := json.Marshal(adminmedia.LegacyMappingRequest{ContractVersion: adminmedia.LegacyMappingContract, LegacyAssetID: legacyID})
	sign := func(caller string) *http.Request {
		r, _ := http.NewRequest("POST", server.URL+adminmedia.LegacyMappingPath, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if err := serviceauth.SignRequest(r, caller, key, body, time.Now()); err != nil {
			t.Fatal(err)
		}
		return r
	}
	status := func(r *http.Request) int {
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if code := status(sign(owner)); code != 401 {
		t.Fatal("non-Bridge caller resolved mapping", code)
	}
	first := sign(adminmedia.Caller)
	replay, _ := http.NewRequest("POST", server.URL+adminmedia.LegacyMappingPath, bytes.NewReader(body))
	replay.Header = first.Header.Clone()
	if a, b := status(first), status(replay); a != 200 || b != 409 {
		t.Fatal("mapping nonce replay", a, b)
	}
	disabled := gin.New()
	mediahttp.NewLegacyMappingHandler(nil, f.pool, key, false).Register(disabled)
	off := httptest.NewServer(disabled)
	defer off.Close()
	offClient, _ := adminmedia.NewClient(off.URL, key)
	if _, err := offClient.LegacyMapping(ctx, legacyID); !errors.Is(err, adminmedia.ErrUnavailable) {
		t.Fatal("disabled mapping lookup answered", err)
	}
	offClient.Close()

	// A forged mapping row (written outside the adoption pipeline, pointing a
	// different legacy asset at the same or another modern object) is never
	// served, even though the table accepts inserts.
	other := f.uploadEvidence(t, mediamodels.PurposeCargoCondition, spec.BindingType, recovery, nil)
	forgedLegacy := insertLegacyAsset(t, f, dir, dispatchLegacy, mission, "proof_image", "active", pdfFile(97))
	if _, err := f.pool.Exec(ctx, `INSERT INTO media_legacy_adoptions(legacy_asset_id,media_object_id,owner_service,legacy_owner_service,legacy_owner_reference,legacy_purpose,organization_id,case_kind,case_reference,subject_type,subject_reference,purpose,checksum_sha256,attestation_digest)
		SELECT $1::uuid,$2,owner_service,legacy_owner_service,legacy_owner_reference,legacy_purpose,organization_id,case_kind,case_reference,subject_type,subject_reference,purpose,checksum_sha256,attestation_digest FROM media_legacy_adoptions WHERE legacy_asset_id=$3::uuid`,
		forgedLegacy, other["mediaObjectId"], legacyID); err != nil {
		t.Fatal(err)
	}
	if _, err := client.LegacyMapping(ctx, forgedLegacy); !errors.Is(err, adminmedia.ErrUnavailable) {
		t.Fatal("forged legacy mapping served", err)
	}
	if _, err := adoption.LookupMapping(ctx, adminmedia.Caller, forgedLegacy); !errors.Is(err, usecases.ErrLegacyMappingInconsistent) {
		t.Fatal("forged mapping not detected", err)
	}
	var audits int
	f.pool.QueryRow(ctx, `SELECT count(*) FROM media_audit_records WHERE action='media.legacy_mapping.resolved'`).Scan(&audits)
	if audits != 2 {
		t.Fatalf("mapping lookups audited %d times, want 2", audits)
	}
}

// --- 5. legacy privacy deletion: tombstone, legal hold, retention ----------

func TestSection19LegacyTombstoneRedactsAndRespectsHolds(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{})
	ctx := context.Background()
	e := newLegacyEnv(t, f, legacyVerdict{safe: true}, false, "")
	id := e.uploadID(t, dispatchLegacy, dispatchToken, "rider-9", "document_file", "ada-okafor-licence.pdf", "application/pdf", pdfFile(40))
	var storagePath string
	f.pool.QueryRow(ctx, `SELECT storage_path FROM media_assets WHERE id=$1::uuid`, id).Scan(&storagePath)

	if _, err := f.pool.Exec(ctx, `UPDATE media_assets SET legal_hold=true WHERE id=$1::uuid`, id); err == nil {
		t.Fatal("legal hold without a recorded reason accepted")
	}
	if _, err := f.pool.Exec(ctx, `UPDATE media_assets SET legal_hold=true, legal_hold_reason='TEST_ONLY_LITIGATION' WHERE id=$1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	if w := e.do(t, http.MethodDelete, "/files/"+id, dispatchLegacy, dispatchToken); w.Code != http.StatusConflict {
		t.Fatalf("legal hold ignored: %d", w.Code)
	}
	if _, err := os.Stat(filepath.Join(e.dir, storagePath)); err != nil {
		t.Fatal("held bytes destroyed", err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE media_assets SET legal_hold=false, legal_hold_reason=NULL, retention_until=now()+interval '1 day' WHERE id=$1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	if w := e.do(t, http.MethodDelete, "/files/"+id, dispatchLegacy, dispatchToken); w.Code != http.StatusConflict {
		t.Fatalf("recorded retention ignored: %d", w.Code)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE media_assets SET retention_until=NULL WHERE id=$1::uuid`, id); err != nil {
		t.Fatal(err)
	}
	// Another owner cannot delete it.
	if w := e.do(t, http.MethodDelete, "/files/"+id, customerLegacy, customerToken); w.Code != http.StatusForbidden {
		t.Fatalf("foreign deletion: %d", w.Code)
	}
	if w := e.do(t, http.MethodDelete, "/files/"+id, dispatchLegacy, dispatchToken); w.Code != 200 {
		t.Fatalf("tombstone: %d %s", w.Code, w.Body.String())
	}
	var filename, url, path, status, metadata, checksum string
	var tomb *time.Time
	if err := f.pool.QueryRow(ctx, `SELECT original_filename,public_url,storage_path,status,metadata::text,checksum_sha256,tombstoned_at FROM media_assets WHERE id=$1::uuid`, id).Scan(&filename, &url, &path, &status, &metadata, &checksum, &tomb); err != nil {
		t.Fatal(err)
	}
	if filename != "" || url != "" || path != "tombstoned/"+id || status != "deleted" || metadata != "{}" || tomb == nil || checksum == "" {
		t.Fatalf("tombstone not redacted: %q %q %q %q %q %v", filename, url, path, status, metadata, tomb)
	}
	if _, err := os.Stat(filepath.Join(e.dir, storagePath)); !os.IsNotExist(err) {
		t.Fatal("tombstoned bytes remain", err)
	}
	if w := e.do(t, http.MethodGet, "/files/"+id+"/content", dispatchLegacy, dispatchToken); w.Code != http.StatusNotFound {
		t.Fatalf("tombstoned asset readable: %d", w.Code)
	}
	var events []string
	rows, _ := f.pool.Query(ctx, `SELECT event_type||':'||reason_code||':'||actor_service FROM media_legacy_asset_events WHERE asset_id=$1::uuid ORDER BY id`, id)
	for rows.Next() {
		var v string
		rows.Scan(&v)
		events = append(events, v)
	}
	rows.Close()
	want := []string{"DELETION_BLOCKED:LEGAL_HOLD:" + dispatchLegacy, "DELETION_BLOCKED:RETENTION_UNEXPIRED:" + dispatchLegacy, "TOMBSTONED:OWNER_DELETION:" + dispatchLegacy}
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("event trail %v want %v", events, want)
	}
	var all string
	f.pool.QueryRow(ctx, `SELECT string_agg(row_to_json(e)::text,'') FROM media_legacy_asset_events e`).Scan(&all)
	if strings.Contains(all, "ada-okafor") {
		t.Fatal("event trail recorded the redacted filename")
	}
	for _, sql := range []string{`UPDATE media_legacy_asset_events SET reason_code='x'`, `DELETE FROM media_legacy_asset_events`, `TRUNCATE media_legacy_asset_events`} {
		if _, err := f.pool.Exec(ctx, sql); err == nil {
			t.Fatal("legacy event trail mutable:", sql)
		}
	}
}

// --- 6. high-risk purposes fail closed on every modern read path -----------

func TestSection19HighRiskPurposesFailClosedWithoutSafeVerdict(t *testing.T) {
	for _, tc := range []struct {
		name    string
		scanner usecases.Scanner
		state   string
	}{
		{"scanner unavailable", unavailableScanner{}, mediamodels.SafetyUnknown},
		{"no scanner", nil, mediamodels.SafetyPending},
		{"malicious", flaggingScanner{}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Deployment flag OFF: the high-risk floor must still apply.
			f := newMediaFixture(t, fixtureOptions{scanner: tc.scanner, requireSafeForDownload: false})
			_, response := f.authorizeUpload(t, "s19-"+strings.ReplaceAll(tc.name, " ", "-"), nil)
			grant := dataOf(t, response)
			f.completeUpload(t, grant, pdfFile(48), nil)
			id := grant["mediaObjectId"].(string)
			var state string
			f.pool.QueryRow(context.Background(), `SELECT safety_status FROM media_objects WHERE id=$1`, id).Scan(&state)
			if tc.state != "" && state != tc.state {
				t.Fatalf("safety %s want %s", state, tc.state)
			}
			status, body := f.call(t, callOptions{method: http.MethodPost, path: "/objects/" + id + "/download-authorizations"})
			if status == http.StatusOK || len(body) > 0 && errorCode(body) == "" {
				t.Fatalf("high-risk COMPANY_DOCUMENT released with safety %s: %d %v", state, status, body)
			}
		})
	}
	if !mediamodels.HighRiskPurpose(mediamodels.PurposeDataExport) || !mediamodels.HighRiskPurpose(mediamodels.PurposeProviderDocument) ||
		!mediamodels.HighRiskPurpose(mediamodels.PurposeDamageEvidence) || mediamodels.HighRiskPurpose(mediamodels.PurposeProductImage) {
		t.Fatal("high-risk purpose set changed")
	}
}

// --- 7. one-time case grants, closure revocation, disclosure history -------

type countingAuthority struct {
	inner *testCaseAuthority
	calls int32
}

func (a *countingAuthority) AuthorizeCase(ctx context.Context, actor usecases.CaseActor, q usecases.CaseRequest) (usecases.CaseProof, error) {
	atomic.AddInt32(&a.calls, 1)
	return a.inner.AuthorizeCase(ctx, actor, q)
}

func TestSection19CaseGrantIsOneTimeAndConcurrentRedemptionSucceedsOnce(t *testing.T) {
	f := newCaseFixture(t)
	ctx := context.Background()
	g := f.issue(t)
	f.read(t, g)
	if b, err := f.access.Read(ctx, f.actor, f.request, g.ID, g.Token); !errors.Is(err, usecases.ErrNotFound) || len(b) > 0 {
		t.Fatal("replayed one-time token served", err)
	}
	concurrent := f.issue(t)
	const n = 8
	var wg sync.WaitGroup
	var ok int32
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if b, err := f.access.Read(ctx, f.actor, f.request, concurrent.ID, concurrent.Token); err == nil && len(b) > 0 {
				atomic.AddInt32(&ok, 1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if ok != 1 {
		t.Fatalf("concurrent redemption succeeded %d times, want exactly 1", ok)
	}
	var redemptions, redeemedAudits int
	f.media.pool.QueryRow(ctx, `SELECT count(*) FROM media_admin_case_redemptions WHERE grant_id=$1`, concurrent.ID).Scan(&redemptions)
	f.media.pool.QueryRow(ctx, `SELECT count(*) FROM media_audit_records WHERE resource_id=$1 AND action='media.admin_case.redeemed'`, concurrent.ID).Scan(&redeemedAudits)
	if redemptions != 1 || redeemedAudits != 1 {
		t.Fatalf("redemption evidence %d/%d, want 1/1", redemptions, redeemedAudits)
	}
	for _, sql := range []string{`UPDATE media_admin_case_redemptions SET staff_ref='x'`, `DELETE FROM media_admin_case_redemptions`, `TRUNCATE media_admin_case_redemptions`} {
		if _, err := f.media.pool.Exec(ctx, sql); err == nil {
			t.Fatal("redemption evidence mutable:", sql)
		}
	}
}

func TestSection19ExpiredCrossSubjectAndCrossTenantGrantsDenied(t *testing.T) {
	f := newCaseFixture(t)
	ctx := context.Background()
	g := f.issue(t)
	for name, mutate := range map[string]func(*usecases.CaseRequest){
		"subject": func(q *usecases.CaseRequest) { q.SubjectReference = "other-subject" },
		"tenant":  func(q *usecases.CaseRequest) { q.Scope.Tenant = "other-tenant"; q.Scope.Organization = "other-org" },
	} {
		q := f.request
		mutate(&q)
		if b, err := f.access.Read(ctx, f.actor, q, g.ID, g.Token); err == nil || len(b) > 0 {
			t.Fatal("cross-"+name+" grant redeemed", err)
		}
	}
	// Neither attempt spent nor revoked the holder's grant.
	var spent bool
	f.media.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_admin_case_redemptions WHERE grant_id=$1) OR EXISTS(SELECT 1 FROM media_admin_case_revocations WHERE grant_id=$1)`, g.ID).Scan(&spent)
	if spent {
		t.Fatal("mismatched attempt spent the holder's grant")
	}
	*f.media.now = f.media.now.Add(61 * time.Second)
	if b, err := f.access.Read(ctx, f.actor, f.request, g.ID, g.Token); err == nil || len(b) > 0 {
		t.Fatal("expired grant redeemed", err)
	}
}

func TestSection19ClosedCaseRecordsImmutableRevocationAndFailsFast(t *testing.T) {
	f := newCaseFixture(t)
	ctx := context.Background()
	counting := &countingAuthority{inner: f.authority}
	access := f.media.service.NewCaseAccess(usecases.CaseAccessConfig{Enabled: true, Policy: f.policy.Engine, Authority: counting})
	g, err := access.Issue(ctx, f.actor, f.request)
	if err != nil {
		t.Fatal(err)
	}
	other, err := access.Issue(ctx, f.actor, f.request)
	if err != nil {
		t.Fatal(err)
	}
	f.authority.denied = true
	// Unrelated staff holding a copied grant reference cannot cause (or
	// observe) a revocation of someone else's grant.
	stranger := f.actor
	stranger.Delegation.Staff = "staff-2"
	if b, err := access.Read(ctx, stranger, f.request, other.ID, other.Token); err == nil || len(b) > 0 {
		t.Fatal("stranger redeemed", err)
	}
	if b, err := access.Read(ctx, f.actor, f.request, g.ID, g.Token); !errors.Is(err, adminpolicy.ErrDenied) || len(b) > 0 {
		t.Fatal("closed case served bytes", err)
	}
	var reason string
	if err := f.media.pool.QueryRow(ctx, `SELECT reason_code FROM media_admin_case_revocations WHERE grant_id=$1`, g.ID).Scan(&reason); err != nil || reason != "CASE_PROOF_REFUSED" {
		t.Fatal("case closure not recorded as revocation", reason, err)
	}
	var strangerRevoked bool
	f.media.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_admin_case_revocations WHERE grant_id=$1)`, other.ID).Scan(&strangerRevoked)
	if strangerRevoked {
		t.Fatal("non-holder attempt revoked another staff member's grant")
	}
	// Later reads fail fast without consulting the source again, even if the
	// source would now answer.
	f.authority.denied = false
	before := atomic.LoadInt32(&counting.calls)
	if b, err := access.Read(ctx, f.actor, f.request, g.ID, g.Token); !errors.Is(err, usecases.ErrNotFound) || len(b) > 0 {
		t.Fatal("revoked grant served after case reopened", err)
	}
	if atomic.LoadInt32(&counting.calls) != before {
		t.Fatal("revoked grant consulted the case authority again")
	}
	// The disclosure history for the grant is complete and immutable.
	var history []string
	rows, _ := f.media.pool.Query(ctx, `SELECT action FROM media_audit_records WHERE resource_id=$1 ORDER BY occurred_at, action`, g.ID)
	for rows.Next() {
		var a string
		rows.Scan(&a)
		history = append(history, a)
	}
	rows.Close()
	if strings.Join(history, ",") != "media.admin_case.authorized,media.admin_case.revoked_case_closed" {
		t.Fatalf("disclosure history %v", history)
	}
	var tokenLeak bool
	f.media.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_audit_records WHERE new_summary::text LIKE '%'||$1||'%')`, g.Token).Scan(&tokenLeak)
	if tokenLeak {
		t.Fatal("grant token entered the disclosure history")
	}
	for _, sql := range []string{`UPDATE media_audit_records SET action='rewritten'`, `DELETE FROM media_audit_records`, `UPDATE media_admin_case_revocations SET reason_code='x'`, `DELETE FROM media_admin_case_revocations`} {
		if _, err := f.media.pool.Exec(ctx, sql); err == nil {
			t.Fatal("disclosure history mutable:", sql)
		}
	}
}

// --- 8. HQ events never carry bytes or long-lived URLs ----------------------

func TestSection19OutboxEventsCarryNoBytesOrURLs(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{scanner: cleanScanner{}})
	view := f.uploadEvidence(t, mediamodels.PurposeRecoveryTransfer, mediamodels.BindingMission, "mission-s19-events", nil)
	id := view["mediaObjectId"].(string)
	f.call(t, callOptions{method: http.MethodPost, path: "/objects/" + id + "/download-authorizations"})
	var payloads string
	if err := f.pool.QueryRow(context.Background(), `SELECT COALESCE(string_agg(payload::text,''),'') FROM media_outbox`).Scan(&payloads); err != nil {
		t.Fatal(err)
	}
	if payloads == "" {
		t.Fatal("no media events recorded to inspect")
	}
	for _, leak := range []string{"%PDF", "http://", "https://", "downloadToken", "uploadToken", "storage_object_key", base64.StdEncoding.EncodeToString(pdfFile(0))[:12]} {
		if strings.Contains(payloads, leak) {
			t.Fatalf("media event carried %q", leak)
		}
	}
}
