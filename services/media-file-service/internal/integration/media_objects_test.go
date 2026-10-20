package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/media-file-service/internal/events"
	mediahttp "cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/http"
	mediamodels "cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/models"
	mediarepositories "cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/repositories"
	mediausecases "cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/usecases"
	"cosmicforge/logistics/services/media-file-service/internal/features/mediaobjects/validation"
	uploadclients "cosmicforge/logistics/services/media-file-service/internal/features/uploads/clients"
)

// The private media contract is exercised end to end: real PostgreSQL, the
// real gin handlers, the real HMAC verifier and real bytes on a real
// filesystem. Nothing in this file asserts against a stubbed repository,
// because the behaviour under test is precisely what the database and the
// stored file do.

const (
	testCaller     = "fleet-partner-service"
	testSecret     = "media-integration-test-secret"
	testOrgID      = "org_media_wp10"
	testSignSecret = "media-download-signing-test-secret"
)

// --- real generated test files -------------------------------------------

// pdfFile returns a real PDF whose leading bytes carry the %PDF- signature.
func pdfFile(padding int) []byte {
	body := []byte("%PDF-1.7\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF\n")
	if padding > 0 {
		body = append(body, bytes.Repeat([]byte("A"), padding)...)
	}
	return body
}

// pngFile returns a real PNG: the 8-byte signature plus an IHDR chunk.
func pngFile() []byte {
	file := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	file = append(file, 0x00, 0x00, 0x00, 0x0D, 'I', 'H', 'D', 'R')
	file = append(file, bytes.Repeat([]byte{0x01}, 13)...)
	file = append(file, 'I', 'E', 'N', 'D')
	return file
}

// executableFile returns bytes carrying a recognised executable signature.
func executableFile() []byte {
	return append([]byte("MZ"), bytes.Repeat([]byte{0x90}, 64)...)
}

// unknownFile returns bytes that match no signature at all.
func unknownFile() []byte {
	return bytes.Repeat([]byte{0x7A}, 256)
}

// --- scanners -------------------------------------------------------------

type flaggingScanner struct{}

func (flaggingScanner) Name() string { return "test-flagging-scanner" }
func (flaggingScanner) Scan(context.Context, string, []byte) (mediausecases.ScanResult, error) {
	return mediausecases.ScanResult{Safe: false, Reason: "TEST_SIGNATURE_MATCH"}, nil
}

type unavailableScanner struct{}

func (unavailableScanner) Name() string { return "test-unavailable-scanner" }
func (unavailableScanner) Scan(context.Context, string, []byte) (mediausecases.ScanResult, error) {
	return mediausecases.ScanResult{}, errors.New("scanner endpoint unreachable")
}

type cleanScanner struct{}

func (cleanScanner) Name() string { return "test-clean-scanner" }
func (cleanScanner) Scan(context.Context, string, []byte) (mediausecases.ScanResult, error) {
	return mediausecases.ScanResult{Safe: true}, nil
}

// --- fixture --------------------------------------------------------------

type mediaFixture struct {
	pool    *pgxpool.Pool
	service *mediausecases.Service
	router  http.Handler
	storage *uploadclients.FilesystemPrivateStorage
	now     *time.Time
	secret  string
	caller  string
	nonce   int
}

type fixtureOptions struct {
	scanner                mediausecases.Scanner
	policy                 mediausecases.Policy
	requireSafeForDownload bool
	downloadSecret         string
	routeMaxUploadBytes    int64
}

func newMediaFixture(t *testing.T, opts fixtureOptions) *mediaFixture {
	t.Helper()
	pool := testPool(t)

	dir := t.TempDir()
	storage, err := uploadclients.NewFilesystemPrivateStorage(filepath.Join(dir, "objects"))
	if err != nil {
		t.Fatalf("create private storage: %v", err)
	}

	clock := time.Now().UTC()
	fixture := &mediaFixture{
		pool:    pool,
		storage: storage,
		now:     &clock,
		secret:  testSecret,
		caller:  testCaller,
	}

	policy := opts.policy
	if policy.MaxSizeBytes == 0 {
		policy.MaxSizeBytes = 1 << 20
	}
	if len(policy.AllowedMIMETypes) == 0 {
		policy.AllowedMIMETypes = []string{"application/pdf", "image/png"}
	}
	if policy.UploadAuthorizationTTL == 0 {
		policy.UploadAuthorizationTTL = 15 * time.Minute
	}
	if policy.DownloadTTL == 0 {
		policy.DownloadTTL = 5 * time.Minute
	}
	policy.ProhibitExecutableContent = true
	policy.RequireSafeForDownload = opts.requireSafeForDownload

	downloadSecret := opts.downloadSecret
	if downloadSecret == "" {
		downloadSecret = testSignSecret
	}

	fixture.service = mediausecases.NewService(mediausecases.Options{
		Pool:           pool,
		Objects:        mediarepositories.NewRepository(pool),
		Storage:        storage,
		Events:         events.NewDomainPublisher(events.NewOutboxRepository(pool)),
		Scanner:        opts.scanner,
		DownloadSecret: []byte(downloadSecret),
		StorageBucket:  "local:test",
		Policy:         policy,
		Now:            func() time.Time { return *fixture.now },
	})

	routeMax := opts.routeMaxUploadBytes
	if routeMax <= 0 {
		routeMax = 8 << 20
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	mediahttp.RegisterPrivateRoutes(engine.Group("/api/v1/media-files/internal/media"),
		mediahttp.NewHandler(fixture.service, pool),
		serviceauth.NewVerifier(serviceauth.Secrets{testCaller: []byte(testSecret)}, 5*time.Minute),
		[]string{testCaller}, routeMax)
	mediahttp.RegisterCustomerUploadRoute(engine.Group("/api/v1/media-files"), fixture.service, routeMax)
	fixture.router = engine
	return fixture
}

func TestCustomerGrantRedeemsWithoutServiceCredentialAndCannotReplay(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{scanner: cleanScanner{}})
	grant, err := f.service.AuthorizeUpload(context.Background(), mediausecases.AuthorizeUploadInput{
		Caller:    mediausecases.Caller{Service: "customer-service", OrganizationID: "customer_customer-1"},
		OwnerType: "CUSTOMER", OwnerReference: "customer-1",
		UploadPurpose: mediamodels.PurposeDisputeAttachment,
		BindingType:   "COMPLAINT", BindingID: "complaint-1",
		AllowedMIMETypes: []string{"application/pdf"}, MaxSizeBytes: 1 << 20,
		IdempotencyKey: "customer-test-1", RequestFingerprint: "fp-customer-test-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	content := pdfFile(8)
	upload := func(token string) int {
		request := httptest.NewRequest(http.MethodPost,
			"/api/v1/media-files/customer-uploads/"+grant.MediaObjectID+"/content", bytes.NewReader(content))
		request.Header.Set("X-Upload-Authorization-ID", grant.AuthorizationID)
		request.Header.Set("X-Upload-Token", token)
		request.Header.Set("X-Declared-Size", fmt.Sprint(len(content)))
		request.Header.Set("Content-Type", "application/pdf")
		recorder := httptest.NewRecorder()
		f.router.ServeHTTP(recorder, request)
		return recorder.Code
	}
	if got := upload("wrong-token"); got != http.StatusNotFound {
		t.Fatalf("wrong token: %d", got)
	}
	if got := upload(grant.UploadToken); got != http.StatusOK {
		t.Fatalf("valid token: %d", got)
	}
	if got := upload(grant.UploadToken); got != http.StatusConflict {
		t.Fatalf("replay: %d", got)
	}
	object, err := f.service.GetObject(context.Background(), mediausecases.Caller{
		Service: "customer-service", OrganizationID: "customer_customer-1"}, grant.MediaObjectID)
	if err != nil || object.UploadStatus != mediamodels.UploadCompleted {
		t.Fatalf("object %+v: %v", object, err)
	}
}

// call performs one signed private request.
type callOptions struct {
	method    string
	path      string
	body      []byte
	headers   map[string]string
	caller    string
	secret    string
	signedAt  time.Time
	omitNonce bool
	nonce     string
}

func (f *mediaFixture) call(t *testing.T, opts callOptions) (int, map[string]any) {
	t.Helper()
	base := "/api/v1/media-files/internal/media"
	request := httptest.NewRequest(opts.method, base+opts.path, bytes.NewReader(opts.body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Organization-ID", testOrgID)
	for key, value := range opts.headers {
		request.Header.Set(key, value)
	}

	if !opts.omitNonce {
		nonce := opts.nonce
		if nonce == "" {
			f.nonce++
			nonce = fmt.Sprintf("nonce-%d-%d", time.Now().UnixNano(), f.nonce)
		}
		request.Header.Set("X-Service-Nonce", nonce)
	}

	caller := opts.caller
	if caller == "" {
		caller = f.caller
	}
	secret := opts.secret
	if secret == "" {
		secret = f.secret
	}
	if err := serviceauth.SignRequest(request, caller, []byte(secret), opts.body, opts.signedAt); err != nil {
		t.Fatalf("sign request: %v", err)
	}
	if opts.omitNonce {
		request.Header.Del(serviceauth.HeaderNonce)
	}

	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, request)

	decoded := map[string]any{}
	_ = json.Unmarshal(recorder.Body.Bytes(), &decoded)
	return recorder.Code, decoded
}

// authorizeUpload issues a grant with sensible defaults.
//
// The fingerprint is derived from the idempotency key, which mirrors what a
// caller does: two requests describing the same thing carry the same
// fingerprint. Tests that need a deliberately conflicting replay override it.
func (f *mediaFixture) authorizeUpload(t *testing.T, key string, overrides map[string]any) (int, map[string]any) {
	return f.authorizeUploadWithFingerprint(t, key, "fp-"+key, overrides)
}

func (f *mediaFixture) authorizeUploadWithFingerprint(t *testing.T, key string, fingerprint string,
	overrides map[string]any) (int, map[string]any) {
	t.Helper()
	body := map[string]any{
		"ownerType":        "COMPANY",
		"ownerReference":   "doc_" + key,
		"documentCategory": "REGISTRATION",
		"uploadPurpose":    mediamodels.PurposeCompanyDocument,
		"allowedMimeTypes": []string{"application/pdf"},
		"maxSizeBytes":     1 << 20,
	}
	for name, value := range overrides {
		body[name] = value
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return f.call(t, callOptions{
		method: http.MethodPost,
		path:   "/upload-authorizations",
		body:   encoded,
		headers: map[string]string{
			"Idempotency-Key":       key,
			"X-Request-Fingerprint": fingerprint,
			"X-Payload-Version":     "1",
		},
	})
}

// completeUpload redeems a grant with the supplied bytes.
func (f *mediaFixture) completeUpload(t *testing.T, grant map[string]any, content []byte,
	extraHeaders map[string]string) (int, map[string]any) {
	t.Helper()
	headers := map[string]string{
		"X-Upload-Authorization-ID": grant["uploadAuthorizationId"].(string),
		"X-Upload-Token":            grant["uploadToken"].(string),
		"X-Declared-Content-Type":   "application/pdf",
	}
	for key, value := range extraHeaders {
		headers[key] = value
	}
	return f.call(t, callOptions{
		method:  http.MethodPost,
		path:    "/objects/" + grant["mediaObjectId"].(string) + "/content",
		body:    content,
		headers: headers,
	})
}

func dataOf(t *testing.T, response map[string]any) map[string]any {
	t.Helper()
	data, ok := response["data"].(map[string]any)
	if !ok {
		t.Fatalf("response carries no data object: %v", response)
	}
	return data
}

func errorCode(response map[string]any) string {
	failure, ok := response["error"].(map[string]any)
	if !ok {
		return ""
	}
	code, _ := failure["code"].(string)
	return code
}

// --- authorization and transport security ---------------------------------

// TestUploadAuthorizationIssuesSingleUseGrant proves the happy path returns a
// bounded grant and no permanent storage credential.
func TestUploadAuthorizationIssuesSingleUseGrant(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{})

	status, response := f.authorizeUpload(t, "auth-happy", nil)
	if status != http.StatusOK {
		t.Fatalf("authorize upload status = %d body = %v", status, response)
	}
	grant := dataOf(t, response)

	for _, required := range []string{"uploadAuthorizationId", "mediaObjectId", "uploadToken", "expiresAt"} {
		if value, _ := grant[required].(string); value == "" {
			t.Fatalf("grant is missing %s: %v", required, grant)
		}
	}
	// Nothing resembling a bucket, a key or a permanent URL may be handed out.
	for _, forbidden := range []string{"storageBucket", "storageObjectKey", "publicUrl", "url", "signedUrl"} {
		if _, present := grant[forbidden]; present {
			t.Fatalf("grant leaks %s: %v", forbidden, grant)
		}
	}

	expiresAt, err := time.Parse(time.RFC3339, grant["expiresAt"].(string))
	if err != nil {
		t.Fatalf("parse expiry: %v", err)
	}
	if !expiresAt.After(*f.now) || expiresAt.After(f.now.Add(time.Hour)) {
		t.Fatalf("grant expiry %s is not short-lived relative to %s", expiresAt, *f.now)
	}

	// The token is a bearer credential; only its hash is persisted.
	var storedHash string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT upload_token_hash FROM media_upload_authorizations WHERE id = $1`,
		grant["uploadAuthorizationId"]).Scan(&storedHash); err != nil {
		t.Fatalf("read stored authorization: %v", err)
	}
	if storedHash == "" || storedHash == grant["uploadToken"].(string) {
		t.Fatal("upload token must be stored only as a hash")
	}
}

// TestPrivateContractRejectsInvalidHMAC proves a wrong secret cannot reach the
// contract at all.
func TestPrivateContractRejectsInvalidHMAC(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{})

	body, _ := json.Marshal(map[string]any{"uploadPurpose": mediamodels.PurposeCompanyDocument})
	status, _ := f.call(t, callOptions{
		method: http.MethodPost, path: "/upload-authorizations", body: body,
		secret: "not-the-configured-secret",
		headers: map[string]string{
			"Idempotency-Key": "bad-hmac", "X-Request-Fingerprint": "fp",
		},
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("invalid HMAC status = %d, want 401", status)
	}
}

// TestPrivateContractRejectsUnknownCaller proves an authenticated service that
// is not on the allowlist is still refused.
func TestPrivateContractRejectsUnknownCaller(t *testing.T) {
	pool := testPool(t)
	dir := t.TempDir()
	storage, err := uploadclients.NewFilesystemPrivateStorage(filepath.Join(dir, "objects"))
	if err != nil {
		t.Fatal(err)
	}
	service := mediausecases.NewService(mediausecases.Options{
		Pool: pool, Objects: mediarepositories.NewRepository(pool), Storage: storage,
		DownloadSecret: []byte(testSignSecret),
	})

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	// Two services hold valid secrets; only one is allowlisted.
	mediahttp.RegisterPrivateRoutes(engine.Group("/internal/media"),
		mediahttp.NewHandler(service, pool),
		serviceauth.NewVerifier(serviceauth.Secrets{
			testCaller:          []byte(testSecret),
			"analytics-service": []byte("analytics-secret"),
		}, 5*time.Minute),
		[]string{testCaller}, 1<<20)

	body, _ := json.Marshal(map[string]any{"uploadPurpose": mediamodels.PurposeCompanyDocument})
	request := httptest.NewRequest(http.MethodPost, "/internal/media/upload-authorizations", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Service-Nonce", "nonce-unknown-caller")
	if err := serviceauth.SignRequest(request, "analytics-service", []byte("analytics-secret"), body, time.Time{}); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("unlisted caller status = %d, want 403", recorder.Code)
	}
}

// TestPrivateContractRejectsStaleTimestamp proves a captured request goes
// stale even while the signature stays mathematically valid.
func TestPrivateContractRejectsStaleTimestamp(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{})

	body, _ := json.Marshal(map[string]any{"uploadPurpose": mediamodels.PurposeCompanyDocument})
	status, _ := f.call(t, callOptions{
		method: http.MethodPost, path: "/upload-authorizations", body: body,
		signedAt: time.Now().Add(-30 * time.Minute),
		headers: map[string]string{
			"Idempotency-Key": "stale", "X-Request-Fingerprint": "fp",
		},
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("stale timestamp status = %d, want 401", status)
	}
}

// TestPrivateContractRejectsReplayedNonce proves the same signed request
// cannot be sent twice inside the freshness window.
func TestPrivateContractRejectsReplayedNonce(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{})

	body, _ := json.Marshal(map[string]any{
		"ownerType": "COMPANY", "ownerReference": "doc_replay",
		"uploadPurpose":    mediamodels.PurposeCompanyDocument,
		"allowedMimeTypes": []string{"application/pdf"}, "maxSizeBytes": 1 << 20,
	})
	headers := map[string]string{"Idempotency-Key": "replay-1", "X-Request-Fingerprint": "fp-replay"}

	status, response := f.call(t, callOptions{
		method: http.MethodPost, path: "/upload-authorizations", body: body,
		headers: headers, nonce: "fixed-replay-nonce",
	})
	if status != http.StatusOK {
		t.Fatalf("first call status = %d body = %v", status, response)
	}

	status, response = f.call(t, callOptions{
		method: http.MethodPost, path: "/upload-authorizations", body: body,
		headers: headers, nonce: "fixed-replay-nonce",
	})
	if status != http.StatusConflict || errorCode(response) != "replayed_request" {
		t.Fatalf("replayed nonce status = %d code = %q, want 409 replayed_request", status, errorCode(response))
	}
}

// TestPrivateContractRequiresNonce proves the replay gate cannot be skipped by
// simply omitting the header.
func TestPrivateContractRequiresNonce(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{})

	body, _ := json.Marshal(map[string]any{"uploadPurpose": mediamodels.PurposeCompanyDocument})
	status, response := f.call(t, callOptions{
		method: http.MethodPost, path: "/upload-authorizations", body: body,
		omitNonce: true,
		headers:   map[string]string{"Idempotency-Key": "no-nonce", "X-Request-Fingerprint": "fp"},
	})
	if status != http.StatusUnauthorized || errorCode(response) != "service_authentication_required" {
		t.Fatalf("missing nonce status = %d code = %q, want 401 service_authentication_required", status, errorCode(response))
	}
}

// TestUploadAuthorizationIdempotencyConflict proves a reused key describing a
// different request is refused rather than silently returning the first grant.
func TestUploadAuthorizationIdempotencyConflict(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{})

	if status, response := f.authorizeUpload(t, "idem-1", nil); status != http.StatusOK {
		t.Fatalf("first authorize status = %d body = %v", status, response)
	}
	// Same key, different request: the caller's fingerprint differs, and the
	// grant it already holds is for constraints it is no longer asking for.
	status, response := f.authorizeUploadWithFingerprint(t, "idem-1", "fp-different",
		map[string]any{"maxSizeBytes": 64})
	if status != http.StatusConflict {
		t.Fatalf("conflicting replay status = %d body = %v, want 409", status, response)
	}

	// An identical retry still replays rather than minting a second grant.
	status, response = f.authorizeUpload(t, "idem-1", nil)
	if status != http.StatusOK {
		t.Fatalf("identical retry status = %d body = %v, want 200", status, response)
	}
	if replayed, _ := dataOf(t, response)["replayed"].(bool); !replayed {
		t.Fatal("identical retry should replay the existing authorization")
	}
}

// --- upload completion and file validation --------------------------------

// TestCompleteUploadValidatesRealFile proves a real PDF is accepted and every
// property is derived from the stored bytes.
func TestCompleteUploadValidatesRealFile(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{})
	_, response := f.authorizeUpload(t, "complete-ok", nil)
	grant := dataOf(t, response)

	content := pdfFile(128)
	status, completion := f.completeUpload(t, grant, content, map[string]string{
		"X-Declared-Checksum-SHA256": validation.Checksum(content),
		"X-Declared-Size":            fmt.Sprintf("%d", len(content)),
	})
	if status != http.StatusOK {
		t.Fatalf("complete upload status = %d body = %v", status, completion)
	}
	object := dataOf(t, completion)

	if object["uploadStatus"] != mediamodels.UploadCompleted {
		t.Fatalf("uploadStatus = %v, want COMPLETED", object["uploadStatus"])
	}
	if object["detectedMime"] != "application/pdf" {
		t.Fatalf("detectedMime = %v, want application/pdf", object["detectedMime"])
	}
	if object["checksumSha256"] != validation.Checksum(content) {
		t.Fatal("checksum was not derived from the stored bytes")
	}
	if object["signatureValidation"] != mediamodels.SignaturePassed {
		t.Fatalf("signatureValidation = %v, want PASSED", object["signatureValidation"])
	}
	// With no scanner configured the object is PENDING, never SAFE.
	if object["safetyStatus"] != mediamodels.SafetyPending {
		t.Fatalf("safetyStatus = %v, want PENDING with no scanner", object["safetyStatus"])
	}
	if size, _ := object["sizeBytes"].(float64); int(size) != len(content) {
		t.Fatalf("sizeBytes = %v, want %d", object["sizeBytes"], len(content))
	}
	if object["captureMethod"] != "MEDIA_VALIDATED_UPLOAD" || object["integrityHash"] != validation.Checksum(content) {
		t.Fatalf("capture metadata is not Media-derived: %v", object)
	}
	if object["actorReference"] != "fleet-partner-service" || object["deviceReference"] != "" {
		t.Fatalf("capture actor/device metadata is not minimized: %v", object)
	}
	if object["capturedAt"] == "" || object["sourceTimestamp"] == "" || object["originalMimeType"] != "application/pdf" {
		t.Fatalf("authoritative capture timestamps/MIME missing: %v", object)
	}

	// The event stream carries the standing but never the bytes or a URL.
	var payload []byte
	if err := f.pool.QueryRow(context.Background(),
		`SELECT payload::text FROM media_outbox WHERE event_type = $1 AND aggregate_id = $2`,
		events.EventUploadCompleted, grant["mediaObjectId"]).Scan(&payload); err != nil {
		t.Fatalf("read outbox event: %v", err)
	}
	encoded := string(payload)
	for _, forbidden := range []string{"storageObjectKey", "uploadToken", "downloadToken", "http://", "https://"} {
		if bytes.Contains([]byte(encoded), []byte(forbidden)) {
			t.Fatalf("media event leaks %q: %s", forbidden, encoded)
		}
	}
	var envelope map[string]any
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatal(err)
	}
	eventPayload, _ := envelope["payload"].(map[string]any)
	for _, field := range []string{"capturedAt", "sourceTimestamp", "captureMethod", "actorReference", "deviceReference", "originalMimeType", "originalSizeBytes", "integrityHash", "sourceVersion"} {
		if fmt.Sprint(eventPayload[field]) != fmt.Sprint(object[field]) {
			t.Errorf("event/read-back %s mismatch: event=%v response=%v", field, eventPayload[field], object[field])
		}
	}
}

// TestCompleteUploadRejectsOversizedFile proves the grant's ceiling is real.
func TestCompleteUploadRejectsOversizedFile(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{})
	_, response := f.authorizeUpload(t, "oversized", map[string]any{"maxSizeBytes": 256})
	grant := dataOf(t, response)

	status, completion := f.completeUpload(t, grant, pdfFile(4096), nil)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("oversized status = %d body = %v, want 422", status, completion)
	}
	if errorCode(completion) != "size_exceeded" {
		t.Fatalf("oversized code = %q, want size_exceeded", errorCode(completion))
	}
	assertObjectState(t, f.pool, grant["mediaObjectId"].(string), mediamodels.UploadFailed, mediamodels.SafetyUnknown)
}

// TestCompleteUploadRejectsDeclaredMIMEMismatch proves the client's claim is
// compared against the bytes rather than believed.
func TestCompleteUploadRejectsDeclaredMIMEMismatch(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{})
	_, response := f.authorizeUpload(t, "mime-mismatch", map[string]any{
		"allowedMimeTypes": []string{"application/pdf", "image/png"},
	})
	grant := dataOf(t, response)

	// A real PNG declared as a PDF.
	status, completion := f.completeUpload(t, grant, pngFile(), map[string]string{
		"X-Declared-Content-Type": "application/pdf",
	})
	if status != http.StatusUnprocessableEntity || errorCode(completion) != "mime_mismatch" {
		t.Fatalf("mime mismatch status = %d code = %q, want 422 mime_mismatch", status, errorCode(completion))
	}
}

// TestCompleteUploadRejectsUnrecognisedSignature proves bytes matching no
// known signature are refused rather than accepted on the declared type.
func TestCompleteUploadRejectsUnrecognisedSignature(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{})
	_, response := f.authorizeUpload(t, "signature-mismatch", nil)
	grant := dataOf(t, response)

	status, completion := f.completeUpload(t, grant, unknownFile(), nil)
	if status != http.StatusUnprocessableEntity || errorCode(completion) != "signature_mismatch" {
		t.Fatalf("signature mismatch status = %d code = %q, want 422 signature_mismatch", status, errorCode(completion))
	}
}

// TestCompleteUploadRejectsExecutableContent proves an executable renamed as a
// document does not get through.
func TestCompleteUploadRejectsExecutableContent(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{})
	_, response := f.authorizeUpload(t, "executable", nil)
	grant := dataOf(t, response)

	status, completion := f.completeUpload(t, grant, executableFile(), nil)
	if status != http.StatusUnprocessableEntity || errorCode(completion) != "executable_content" {
		t.Fatalf("executable status = %d code = %q, want 422 executable_content", status, errorCode(completion))
	}
}

// TestCompleteUploadRejectsChecksumMismatch proves a corrupted transfer is
// caught even when the bytes are otherwise a valid file.
func TestCompleteUploadRejectsChecksumMismatch(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{})
	_, response := f.authorizeUpload(t, "checksum", nil)
	grant := dataOf(t, response)

	status, completion := f.completeUpload(t, grant, pdfFile(32), map[string]string{
		"X-Declared-Checksum-SHA256": validation.Checksum(pdfFile(64)),
	})
	if status != http.StatusUnprocessableEntity || errorCode(completion) != "checksum_mismatch" {
		t.Fatalf("checksum mismatch status = %d code = %q, want 422 checksum_mismatch", status, errorCode(completion))
	}
}

// TestCompleteUploadRejectsIncompleteTransfer proves a declared size that
// disagrees with the received bytes is treated as a truncated upload.
func TestCompleteUploadRejectsIncompleteTransfer(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{})
	_, response := f.authorizeUpload(t, "incomplete", nil)
	grant := dataOf(t, response)

	content := pdfFile(32)
	status, completion := f.completeUpload(t, grant, content, map[string]string{
		"X-Declared-Size": fmt.Sprintf("%d", len(content)+500),
	})
	if status != http.StatusUnprocessableEntity || errorCode(completion) != "incomplete_upload" {
		t.Fatalf("incomplete upload status = %d code = %q, want 422 incomplete_upload", status, errorCode(completion))
	}
}

// TestCompleteUploadRejectsTypeOutsideGrant proves a grant narrows the
// service-wide allowlist rather than being ignored.
func TestCompleteUploadRejectsTypeOutsideGrant(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{})
	// The grant permits PDF only; policy also permits PNG.
	_, response := f.authorizeUpload(t, "narrow-grant", map[string]any{
		"allowedMimeTypes": []string{"application/pdf"},
	})
	grant := dataOf(t, response)

	status, completion := f.completeUpload(t, grant, pngFile(), map[string]string{
		"X-Declared-Content-Type": "image/png",
	})
	if status != http.StatusUnprocessableEntity || errorCode(completion) != "unsupported_type" {
		t.Fatalf("narrowed grant status = %d code = %q, want 422 unsupported_type", status, errorCode(completion))
	}
}

// TestUploadAuthorizationIsSingleUse proves a spent grant cannot be redeemed
// a second time with different bytes.
func TestUploadAuthorizationIsSingleUse(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{})
	_, response := f.authorizeUpload(t, "single-use", nil)
	grant := dataOf(t, response)

	if status, body := f.completeUpload(t, grant, pdfFile(16), nil); status != http.StatusOK {
		t.Fatalf("first completion status = %d body = %v", status, body)
	}
	status, completion := f.completeUpload(t, grant, pdfFile(999), nil)
	if status != http.StatusConflict || errorCode(completion) != "authorization_already_used" {
		t.Fatalf("reuse status = %d code = %q, want 409 authorization_already_used", status, errorCode(completion))
	}
}

// TestExpiredUploadAuthorizationIsRefused proves a lapsed grant is gone even
// though its row still exists.
func TestExpiredUploadAuthorizationIsRefused(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{
		policy: mediausecases.Policy{UploadAuthorizationTTL: time.Minute},
	})
	_, response := f.authorizeUpload(t, "expiring", nil)
	grant := dataOf(t, response)

	*f.now = f.now.Add(2 * time.Minute)

	status, completion := f.completeUpload(t, grant, pdfFile(16), nil)
	if status != http.StatusGone || errorCode(completion) != "authorization_expired" {
		t.Fatalf("expired grant status = %d code = %q, want 410 authorization_expired", status, errorCode(completion))
	}
}

// TestForgedUploadTokenIsRefused proves the grant id alone is not enough.
func TestForgedUploadTokenIsRefused(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{})
	_, response := f.authorizeUpload(t, "forged-token", nil)
	grant := dataOf(t, response)
	grant["uploadToken"] = "obviously-not-the-issued-token"

	status, completion := f.completeUpload(t, grant, pdfFile(16), nil)
	if status != http.StatusNotFound {
		t.Fatalf("forged token status = %d body = %v, want 404", status, completion)
	}
}

// --- safety ---------------------------------------------------------------

// TestUnsafeContentIsQuarantinedAndBytesRemoved proves a flagged file is not
// merely labelled: the bytes are destroyed and the object is unreachable.
func TestUnsafeContentIsQuarantinedAndBytesRemoved(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{scanner: flaggingScanner{}})
	_, response := f.authorizeUpload(t, "unsafe", nil)
	grant := dataOf(t, response)

	status, completion := f.completeUpload(t, grant, pdfFile(16), nil)
	if status != http.StatusOK {
		t.Fatalf("completion status = %d body = %v", status, completion)
	}
	object := dataOf(t, completion)
	if object["safetyStatus"] != mediamodels.SafetyQuarantined {
		t.Fatalf("safetyStatus = %v, want QUARANTINED", object["safetyStatus"])
	}

	var key string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT storage_object_key FROM media_objects WHERE id = $1`, grant["mediaObjectId"]).Scan(&key); err != nil {
		t.Fatalf("read storage key: %v", err)
	}
	if _, err := f.storage.Get(context.Background(), key); !errors.Is(err, uploadclients.ErrObjectMissing) {
		t.Fatalf("quarantined bytes are still retrievable: %v", err)
	}

	status, download := f.call(t, callOptions{
		method: http.MethodPost,
		path:   "/objects/" + grant["mediaObjectId"].(string) + "/download-authorizations",
	})
	if status != http.StatusForbidden || errorCode(download) != "not_downloadable" {
		t.Fatalf("quarantined download status = %d code = %q, want 403 not_downloadable", status, errorCode(download))
	}
}

// TestScannerUnavailableFailsClosed proves an unreachable scanner produces
// UNKNOWN, and that a policy requiring SAFE then refuses the download.
func TestScannerUnavailableFailsClosed(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{
		scanner:                unavailableScanner{},
		requireSafeForDownload: true,
	})
	_, response := f.authorizeUpload(t, "scanner-down", nil)
	grant := dataOf(t, response)

	status, completion := f.completeUpload(t, grant, pdfFile(16), nil)
	if status != http.StatusOK {
		t.Fatalf("completion status = %d body = %v", status, completion)
	}
	object := dataOf(t, completion)
	if object["safetyStatus"] != mediamodels.SafetyUnknown {
		t.Fatalf("safetyStatus = %v, want UNKNOWN when the scanner cannot answer", object["safetyStatus"])
	}
	if object["safetyStatus"] == mediamodels.SafetySafe {
		t.Fatal("an unavailable scanner must never report SAFE")
	}

	status, download := f.call(t, callOptions{
		method: http.MethodPost,
		path:   "/objects/" + grant["mediaObjectId"].(string) + "/download-authorizations",
	})
	if status != http.StatusForbidden || errorCode(download) != "not_downloadable" {
		t.Fatalf("fail-closed download status = %d code = %q, want 403 not_downloadable", status, errorCode(download))
	}
}

// TestNoScannerLeavesSafetyPending proves the absence of a provider is visible
// rather than assumed benign.
func TestNoScannerLeavesSafetyPending(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{requireSafeForDownload: true})
	_, response := f.authorizeUpload(t, "no-scanner", nil)
	grant := dataOf(t, response)

	_, completion := f.completeUpload(t, grant, pdfFile(16), nil)
	object := dataOf(t, completion)
	if object["safetyStatus"] != mediamodels.SafetyPending {
		t.Fatalf("safetyStatus = %v, want PENDING with no scanner", object["safetyStatus"])
	}

	status, denial := f.call(t, callOptions{
		method: http.MethodPost,
		path:   "/objects/" + grant["mediaObjectId"].(string) + "/download-authorizations",
	})
	if status != http.StatusForbidden || errorCode(denial) != "not_downloadable" {
		t.Fatalf("PENDING download status = %d code = %q, want 403 not_downloadable under a SAFE-required policy",
			status, errorCode(denial))
	}
}

// --- private download -----------------------------------------------------

// TestPrivateDownloadGrantIsShortLivedAndUnstored proves content is reachable
// only through a bounded grant that leaves no durable copy behind.
func TestPrivateDownloadGrantIsShortLivedAndUnstored(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{
		scanner: cleanScanner{},
		policy:  mediausecases.Policy{DownloadTTL: 2 * time.Minute},
	})
	_, response := f.authorizeUpload(t, "download", nil)
	grant := dataOf(t, response)
	content := pdfFile(64)
	if status, body := f.completeUpload(t, grant, content, nil); status != http.StatusOK {
		t.Fatalf("completion status = %d body = %v", status, body)
	}
	objectID := grant["mediaObjectId"].(string)

	status, response := f.call(t, callOptions{
		method: http.MethodPost, path: "/objects/" + objectID + "/download-authorizations",
	})
	if status != http.StatusOK {
		t.Fatalf("download authorization status = %d body = %v", status, response)
	}
	download := dataOf(t, response)
	token, _ := download["downloadToken"].(string)
	if token == "" {
		t.Fatal("download grant carries no token")
	}

	// The grant is never persisted anywhere.
	assertTokenNotPersisted(t, f.pool, token)

	// It works while it is alive.
	base := "/api/v1/media-files/internal/media"
	request := httptest.NewRequest(http.MethodGet, base+"/objects/"+objectID+"/content", nil)
	request.Header.Set("X-Organization-ID", testOrgID)
	request.Header.Set("X-Service-Nonce", "content-nonce-1")
	request.Header.Set("X-Download-Token", token)
	if err := serviceauth.SignRequest(request, testCaller, []byte(testSecret), nil, time.Time{}); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("content status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if !bytes.Equal(recorder.Body.Bytes(), content) {
		t.Fatal("served content does not match the stored bytes")
	}

	// And stops working the moment it lapses.
	*f.now = f.now.Add(5 * time.Minute)
	request = httptest.NewRequest(http.MethodGet, base+"/objects/"+objectID+"/content", nil)
	request.Header.Set("X-Organization-ID", testOrgID)
	request.Header.Set("X-Service-Nonce", "content-nonce-2")
	request.Header.Set("X-Download-Token", token)
	if err := serviceauth.SignRequest(request, testCaller, []byte(testSecret), nil, time.Time{}); err != nil {
		t.Fatal(err)
	}
	recorder = httptest.NewRecorder()
	f.router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expired grant status = %d, want 404", recorder.Code)
	}
}

func TestExistingDownloadGrantRechecksCurrentSafetyPolicy(t *testing.T) {
	for _, requireSafe := range []bool{true, false} {
		t.Run(fmt.Sprintf("require_safe_%v", requireSafe), func(t *testing.T) {
			f := newMediaFixture(t, fixtureOptions{scanner: cleanScanner{}, requireSafeForDownload: requireSafe})
			_, response := f.authorizeUpload(t, "current-safety", nil)
			upload := dataOf(t, response)
			content := pdfFile(64)
			if status, body := f.completeUpload(t, upload, content, nil); status != http.StatusOK {
				t.Fatal(status, body)
			}
			id := upload["mediaObjectId"].(string)
			status, response := f.call(t, callOptions{method: http.MethodPost, path: "/objects/" + id + "/download-authorizations"})
			if status != http.StatusOK {
				t.Fatal(status, response)
			}
			token := dataOf(t, response)["downloadToken"].(string)
			// Stored native safety is authoritative at redemption, even when a
			// still-valid grant was minted while the file was confirmed SAFE.
			for _, state := range []string{mediamodels.SafetyUnknown, mediamodels.SafetyPending, mediamodels.SafetyUnsafe, mediamodels.SafetyQuarantined, mediamodels.SafetySafe} {
				if _, err := f.pool.Exec(context.Background(), `UPDATE media_objects SET safety_status=$2,version=version+1 WHERE id=$1`, id, state); err != nil {
					t.Fatal(err)
				}
				data, _, err := f.service.ReadContent(context.Background(), id, token)
				blocked := state == mediamodels.SafetyUnsafe || state == mediamodels.SafetyQuarantined || requireSafe && state != mediamodels.SafetySafe
				if blocked {
					if !errors.Is(err, mediausecases.ErrNotDownloadable) || len(data) != 0 {
						t.Fatalf("existing grant bypassed current policy: state=%s error=%v bytes=%d", state, err, len(data))
					}
				} else if err != nil || !bytes.Equal(data, content) {
					t.Fatalf("native allowed content semantics changed: state=%s error=%v", state, err)
				}
			}
		})
	}
}

// TestDownloadGrantIsBoundToOneObject proves a grant for one object cannot
// unlock another.
func TestDownloadGrantIsBoundToOneObject(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{scanner: cleanScanner{}})

	_, first := f.authorizeUpload(t, "bind-a", nil)
	grantA := dataOf(t, first)
	f.completeUpload(t, grantA, pdfFile(16), nil)

	_, second := f.authorizeUpload(t, "bind-b", nil)
	grantB := dataOf(t, second)
	f.completeUpload(t, grantB, pdfFile(32), nil)

	_, response := f.call(t, callOptions{
		method: http.MethodPost,
		path:   "/objects/" + grantA["mediaObjectId"].(string) + "/download-authorizations",
	})
	token := dataOf(t, response)["downloadToken"].(string)

	base := "/api/v1/media-files/internal/media"
	request := httptest.NewRequest(http.MethodGet,
		base+"/objects/"+grantB["mediaObjectId"].(string)+"/content", nil)
	request.Header.Set("X-Organization-ID", testOrgID)
	request.Header.Set("X-Service-Nonce", "cross-object-nonce")
	request.Header.Set("X-Download-Token", token)
	if err := serviceauth.SignRequest(request, testCaller, []byte(testSecret), nil, time.Time{}); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("cross-object grant status = %d, want 404", recorder.Code)
	}
}

// TestCrossTenantObjectReadsAsAbsent proves an identifier from another tenant
// answers not-found rather than forbidden.
func TestCrossTenantObjectReadsAsAbsent(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{})
	_, response := f.authorizeUpload(t, "tenant-isolation", nil)
	grant := dataOf(t, response)

	status, body := f.call(t, callOptions{
		method:  http.MethodGet,
		path:    "/objects/" + grant["mediaObjectId"].(string),
		headers: map[string]string{"X-Organization-ID": "org_someone_else"},
	})
	if status != http.StatusNotFound {
		t.Fatalf("cross-tenant read status = %d body = %v, want 404", status, body)
	}
}

// --- deletion, retention and legal hold -----------------------------------

// TestDeletionDestroysBytesAndRecordsEvidence proves the bytes go and the
// audit trail stays.
func TestDeletionDestroysBytesAndRecordsEvidence(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{scanner: cleanScanner{}})
	_, response := f.authorizeUpload(t, "delete", nil)
	grant := dataOf(t, response)
	f.completeUpload(t, grant, pdfFile(16), nil)
	objectID := grant["mediaObjectId"].(string)

	body, _ := json.Marshal(map[string]any{"reason": "document replaced"})
	status, deletion := f.call(t, callOptions{
		method: http.MethodPost, path: "/objects/" + objectID + "/deletions", body: body,
		headers: map[string]string{"Idempotency-Key": "del-1", "X-Request-Fingerprint": "fp-del-1"},
	})
	if status != http.StatusOK {
		t.Fatalf("deletion status = %d body = %v", status, deletion)
	}
	if got := dataOf(t, deletion)["status"]; got != mediamodels.DeletionRequestCompleted {
		t.Fatalf("deletion request status = %v, want COMPLETED", got)
	}

	var key, deletionState string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT storage_object_key, deletion_state FROM media_objects WHERE id = $1`,
		objectID).Scan(&key, &deletionState); err != nil {
		t.Fatalf("read object: %v", err)
	}
	if deletionState != mediamodels.DeletionDeleted {
		t.Fatalf("deletion_state = %s, want DELETED", deletionState)
	}
	if _, err := f.storage.Get(context.Background(), key); !errors.Is(err, uploadclients.ErrObjectMissing) {
		t.Fatalf("deleted bytes are still retrievable: %v", err)
	}

	// Audit evidence outlives the file.
	var auditCount int
	if err := f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM media_audit_records WHERE resource_id = $1`, objectID).Scan(&auditCount); err != nil {
		t.Fatalf("count audit records: %v", err)
	}
	if auditCount == 0 {
		t.Fatal("expected audit evidence to survive physical deletion")
	}

	// And a deleted object can no longer be released for download.
	status, denial := f.call(t, callOptions{
		method: http.MethodPost, path: "/objects/" + objectID + "/download-authorizations",
	})
	if status != http.StatusForbidden || errorCode(denial) != "not_downloadable" {
		t.Fatalf("deleted download status = %d code = %q, want 403 not_downloadable", status, errorCode(denial))
	}
}

// TestLegalHoldBlocksPhysicalDeletion proves media refuses to destroy bytes it
// is required to keep, and says why.
func TestLegalHoldBlocksPhysicalDeletion(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{scanner: cleanScanner{}})
	_, response := f.authorizeUpload(t, "legal-hold", nil)
	grant := dataOf(t, response)
	f.completeUpload(t, grant, pdfFile(16), nil)
	objectID := grant["mediaObjectId"].(string)

	holdBody, _ := json.Marshal(map[string]any{"legalHold": true})
	if status, body := f.call(t, callOptions{
		method: http.MethodPost, path: "/objects/" + objectID + "/retention", body: holdBody,
	}); status != http.StatusOK {
		t.Fatalf("set legal hold status = %d body = %v", status, body)
	}

	deleteBody, _ := json.Marshal(map[string]any{"reason": "cleanup"})
	status, deletion := f.call(t, callOptions{
		method: http.MethodPost, path: "/objects/" + objectID + "/deletions", body: deleteBody,
		headers: map[string]string{"Idempotency-Key": "del-hold", "X-Request-Fingerprint": "fp-del-hold"},
	})
	if status != http.StatusOK {
		t.Fatalf("blocked deletion status = %d body = %v", status, deletion)
	}
	request := dataOf(t, deletion)
	if request["status"] != mediamodels.DeletionRequestBlocked || request["blockReason"] != "LEGAL_HOLD" {
		t.Fatalf("expected BLOCKED/LEGAL_HOLD, got %v/%v", request["status"], request["blockReason"])
	}

	var key, deletionState string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT storage_object_key, deletion_state FROM media_objects WHERE id = $1`,
		objectID).Scan(&key, &deletionState); err != nil {
		t.Fatalf("read object: %v", err)
	}
	if deletionState != mediamodels.DeletionActive {
		t.Fatalf("deletion_state = %s, want ACTIVE while a legal hold is in force", deletionState)
	}
	if _, err := f.storage.Get(context.Background(), key); err != nil {
		t.Fatalf("bytes under legal hold must remain: %v", err)
	}
}

// TestRetentionWindowBlocksDeletionUntilItLapses proves the window is enforced
// and then stops being enforced.
func TestRetentionWindowBlocksDeletionUntilItLapses(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{scanner: cleanScanner{}})
	_, response := f.authorizeUpload(t, "retention", nil)
	grant := dataOf(t, response)
	f.completeUpload(t, grant, pdfFile(16), nil)
	objectID := grant["mediaObjectId"].(string)

	until := f.now.Add(time.Hour)
	retentionBody, _ := json.Marshal(map[string]any{
		"retentionClass": mediamodels.RetentionExtended,
		"retentionUntil": until.Format(time.RFC3339),
	})
	if status, body := f.call(t, callOptions{
		method: http.MethodPost, path: "/objects/" + objectID + "/retention", body: retentionBody,
	}); status != http.StatusOK {
		t.Fatalf("set retention status = %d body = %v", status, body)
	}

	deleteBody, _ := json.Marshal(map[string]any{"reason": "cleanup"})
	_, deletion := f.call(t, callOptions{
		method: http.MethodPost, path: "/objects/" + objectID + "/deletions", body: deleteBody,
		headers: map[string]string{"Idempotency-Key": "del-ret-1", "X-Request-Fingerprint": "fp-ret-1"},
	})
	if got := dataOf(t, deletion); got["status"] != mediamodels.DeletionRequestBlocked ||
		got["blockReason"] != "RETENTION_ACTIVE" {
		t.Fatalf("expected BLOCKED/RETENTION_ACTIVE, got %v/%v", got["status"], got["blockReason"])
	}

	*f.now = f.now.Add(2 * time.Hour)
	_, deletion = f.call(t, callOptions{
		method: http.MethodPost, path: "/objects/" + objectID + "/deletions", body: deleteBody,
		headers: map[string]string{"Idempotency-Key": "del-ret-2", "X-Request-Fingerprint": "fp-ret-2"},
	})
	if got := dataOf(t, deletion)["status"]; got != mediamodels.DeletionRequestCompleted {
		t.Fatalf("expected COMPLETED after retention lapsed, got %v", got)
	}
}

// TestDeletionReadBackResolvesUnknownOutcome proves a caller that timed out
// mid-deletion can establish the authoritative outcome afterwards.
func TestDeletionReadBackResolvesUnknownOutcome(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{scanner: cleanScanner{}})
	_, response := f.authorizeUpload(t, "unknown-deletion", nil)
	grant := dataOf(t, response)
	f.completeUpload(t, grant, pdfFile(16), nil)
	objectID := grant["mediaObjectId"].(string)

	deleteBody, _ := json.Marshal(map[string]any{"reason": "cleanup"})
	if status, body := f.call(t, callOptions{
		method: http.MethodPost, path: "/objects/" + objectID + "/deletions", body: deleteBody,
		headers: map[string]string{"Idempotency-Key": "del-unknown", "X-Request-Fingerprint": "fp-unknown"},
	}); status != http.StatusOK {
		t.Fatalf("deletion status = %d body = %v", status, body)
	}

	// The caller never saw the response. It reads back instead of assuming.
	status, readBack := f.call(t, callOptions{
		method: http.MethodGet, path: "/objects/" + objectID + "/deletion",
	})
	if status != http.StatusOK {
		t.Fatalf("deletion read-back status = %d body = %v", status, readBack)
	}
	data := dataOf(t, readBack)
	if data["deletionState"] != mediamodels.DeletionDeleted {
		t.Fatalf("read-back deletionState = %v, want DELETED", data["deletionState"])
	}

	// And the idempotent retry returns the same authoritative answer rather
	// than starting a second deletion.
	_, retry := f.call(t, callOptions{
		method: http.MethodPost, path: "/objects/" + objectID + "/deletions", body: deleteBody,
		headers: map[string]string{"Idempotency-Key": "del-unknown", "X-Request-Fingerprint": "fp-unknown"},
	})
	if got := dataOf(t, retry)["status"]; got != mediamodels.DeletionRequestCompleted {
		t.Fatalf("idempotent deletion retry status = %v, want COMPLETED", got)
	}
}

// TestDeletionFailureIsNotReportedAsSuccess proves a storage backend that
// refuses to delete leaves the object DELETION_FAILED, never DELETED.
func TestDeletionFailureIsNotReportedAsSuccess(t *testing.T) {
	pool := testPool(t)
	dir := t.TempDir()
	backing, err := uploadclients.NewFilesystemPrivateStorage(filepath.Join(dir, "objects"))
	if err != nil {
		t.Fatal(err)
	}
	storage := &failingDeleteStorage{PrivateStorage: backing}

	clock := time.Now().UTC()
	service := mediausecases.NewService(mediausecases.Options{
		Pool:           pool,
		Objects:        mediarepositories.NewRepository(pool),
		Storage:        storage,
		Events:         events.NewDomainPublisher(events.NewOutboxRepository(pool)),
		DownloadSecret: []byte(testSignSecret),
		StorageBucket:  "local:test",
		Policy: mediausecases.Policy{
			MaxSizeBytes: 1 << 20, AllowedMIMETypes: []string{"application/pdf"},
		},
		Now: func() time.Time { return clock },
	})

	caller := mediausecases.Caller{Service: testCaller, OrganizationID: testOrgID}
	grant, err := service.AuthorizeUpload(context.Background(), mediausecases.AuthorizeUploadInput{
		Caller: caller, OwnerType: "COMPANY", OwnerReference: "doc_fail",
		UploadPurpose:    mediamodels.PurposeCompanyDocument,
		AllowedMIMETypes: []string{"application/pdf"}, MaxSizeBytes: 1 << 20,
		IdempotencyKey: "fail-del", RequestFingerprint: "fp-fail-del",
	})
	if err != nil {
		t.Fatalf("authorize upload: %v", err)
	}
	content := pdfFile(16)
	if _, err := service.CompleteUpload(context.Background(), mediausecases.CompleteUploadInput{
		Caller: caller, MediaObjectID: grant.MediaObjectID, AuthorizationID: grant.AuthorizationID,
		UploadToken: grant.UploadToken, Content: content,
	}); err != nil {
		t.Fatalf("complete upload: %v", err)
	}

	storage.fail = true
	if _, err := service.RequestDeletion(context.Background(), mediausecases.RequestDeletionInput{
		Caller: caller, MediaObjectID: grant.MediaObjectID,
		IdempotencyKey: "fail-del-req", RequestFingerprint: "fp-fail-del-req",
	}); err != nil {
		t.Fatalf("request deletion: %v", err)
	}

	var deletionState string
	if err := pool.QueryRow(context.Background(),
		`SELECT deletion_state FROM media_objects WHERE id = $1`, grant.MediaObjectID).Scan(&deletionState); err != nil {
		t.Fatalf("read deletion state: %v", err)
	}
	if deletionState != mediamodels.DeletionFailed {
		t.Fatalf("deletion_state = %s, want DELETION_FAILED", deletionState)
	}

	var eventType string
	if err := pool.QueryRow(context.Background(),
		`SELECT event_type FROM media_outbox WHERE aggregate_id = $1 AND event_type IN ($2, $3)`,
		grant.MediaObjectID, events.EventDeletionCompleted, events.EventDeletionFailed).Scan(&eventType); err != nil {
		t.Fatalf("read deletion event: %v", err)
	}
	if eventType != events.EventDeletionFailed {
		t.Fatalf("emitted %s, want %s", eventType, events.EventDeletionFailed)
	}
}

type failingDeleteStorage struct {
	uploadclients.PrivateStorage
	fail bool
}

func (s *failingDeleteStorage) Delete(ctx context.Context, key string) error {
	if s.fail {
		return errors.New("storage delete refused")
	}
	return s.PrivateStorage.Delete(ctx, key)
}

// --- workers --------------------------------------------------------------

// TestMaintenanceSweepExpiresLapsedGrants proves the sweep turns a lapsed
// grant into an explicit state rather than leaving it nominally issued.
func TestMaintenanceSweepExpiresLapsedGrants(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{
		policy: mediausecases.Policy{UploadAuthorizationTTL: time.Second},
	})
	_, response := f.authorizeUpload(t, "sweep", nil)
	grant := dataOf(t, response)

	// The grant lapses in wall-clock terms, which is what the SQL sweep uses.
	if _, err := f.pool.Exec(context.Background(),
		`UPDATE media_upload_authorizations SET expires_at = now() - interval '1 hour' WHERE id = $1`,
		grant["uploadAuthorizationId"]); err != nil {
		t.Fatalf("age the grant: %v", err)
	}
	if err := f.service.RunMaintenancePass(context.Background(), 50); err != nil {
		t.Fatalf("maintenance pass: %v", err)
	}

	var state, uploadStatus string
	if err := f.pool.QueryRow(context.Background(), `
		SELECT a.state, o.upload_status
		FROM media_upload_authorizations a
		JOIN media_objects o ON o.id = a.media_object_id
		WHERE a.id = $1`, grant["uploadAuthorizationId"]).Scan(&state, &uploadStatus); err != nil {
		t.Fatalf("read swept state: %v", err)
	}
	if state != mediamodels.AuthorizationExpired {
		t.Fatalf("authorization state = %s, want EXPIRED", state)
	}
	if uploadStatus != mediamodels.UploadExpired {
		t.Fatalf("upload status = %s, want EXPIRED", uploadStatus)
	}
}

// TestOutboxWorkerRecoversAbandonedLease proves a publisher that died holding
// a lease does not strand the event.
func TestOutboxWorkerRecoversAbandonedLease(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{})
	_, response := f.authorizeUpload(t, "lease", nil)
	grant := dataOf(t, response)
	f.completeUpload(t, grant, pdfFile(16), nil)

	repo := events.NewOutboxRepository(f.pool)
	if _, err := f.pool.Exec(context.Background(), `
		UPDATE media_outbox
		SET lease_owner = 'dead-publisher', lease_expires_at = now() - interval '1 hour'`); err != nil {
		t.Fatalf("simulate abandoned lease: %v", err)
	}

	published := &recordingPublisher{}
	worker := events.NewWorker(repo, published, events.WorkerConfig{Owner: "recovery-test", BatchSize: 10})
	if err := worker.RunPass(context.Background()); err != nil {
		t.Fatalf("worker pass: %v", err)
	}
	if published.count == 0 {
		t.Fatal("expected the abandoned records to be reclaimed and published")
	}

	var pending int
	if err := f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM media_outbox WHERE status = 'PENDING'`).Scan(&pending); err != nil {
		t.Fatalf("count pending: %v", err)
	}
	if pending != 0 {
		t.Fatalf("pending outbox records = %d, want 0", pending)
	}
}

// TestOutboxDeadLetterAndOperatorReplay proves a permanently failing consumer
// dead-letters the record and an operator can put it back.
func TestOutboxDeadLetterAndOperatorReplay(t *testing.T) {
	f := newMediaFixture(t, fixtureOptions{})
	_, response := f.authorizeUpload(t, "dead-letter", nil)
	grant := dataOf(t, response)
	f.completeUpload(t, grant, pdfFile(16), nil)

	repo := events.NewOutboxRepository(f.pool)
	worker := events.NewWorker(repo, failingPublisher{}, events.WorkerConfig{
		Owner: "dead-letter-test", BatchSize: 10, MaxAttempts: 1,
	})
	if err := worker.RunPass(context.Background()); err != nil {
		t.Fatalf("worker pass: %v", err)
	}

	var deadLettered int
	if err := f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM media_outbox WHERE status = 'DEAD_LETTERED'`).Scan(&deadLettered); err != nil {
		t.Fatalf("count dead-lettered: %v", err)
	}
	if deadLettered == 0 {
		t.Fatal("expected records to dead-letter after exhausting attempts")
	}

	var id string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT id FROM media_outbox WHERE status = 'DEAD_LETTERED' LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("read dead-lettered id: %v", err)
	}
	if err := repo.Replay(context.Background(), id); err != nil {
		t.Fatalf("operator replay: %v", err)
	}
	var status string
	if err := f.pool.QueryRow(context.Background(),
		`SELECT status FROM media_outbox WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatalf("read replayed status: %v", err)
	}
	if status != events.OutboxStatusPending {
		t.Fatalf("replayed status = %s, want PENDING", status)
	}
}

type recordingPublisher struct{ count int }

func (p *recordingPublisher) Publish(context.Context, events.OutboxRecord) error {
	p.count++
	return nil
}

type failingPublisher struct{}

func (failingPublisher) Publish(context.Context, events.OutboxRecord) error {
	return errors.New("consumer permanently unavailable")
}

// --- helpers --------------------------------------------------------------

func assertObjectState(t *testing.T, pool *pgxpool.Pool, id string, uploadStatus string, safetyStatus string) {
	t.Helper()
	var gotUpload, gotSafety string
	if err := pool.QueryRow(context.Background(),
		`SELECT upload_status, safety_status FROM media_objects WHERE id = $1`, id).Scan(&gotUpload, &gotSafety); err != nil {
		t.Fatalf("read object state: %v", err)
	}
	if gotUpload != uploadStatus || gotSafety != safetyStatus {
		t.Fatalf("object state = %s/%s, want %s/%s", gotUpload, gotSafety, uploadStatus, safetyStatus)
	}
}

// assertTokenNotPersisted proves a short-lived grant left no durable copy in
// any media table.
func assertTokenNotPersisted(t *testing.T, pool *pgxpool.Pool, token string) {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT tablename FROM pg_tables
		WHERE schemaname = 'public' AND tablename LIKE 'media\_%'`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if scanErr := rows.Scan(&name); scanErr != nil {
			rows.Close()
			t.Fatal(scanErr)
		}
		tables = append(tables, name)
	}
	rows.Close()

	for _, table := range tables {
		var hits int
		query := fmt.Sprintf(
			`SELECT count(*) FROM %s t WHERE t::text LIKE '%%' || $1 || '%%'`, quoteIdentifier(table))
		if err := pool.QueryRow(context.Background(), query, token).Scan(&hits); err != nil {
			t.Fatalf("scan %s for the grant: %v", table, err)
		}
		if hits > 0 {
			t.Fatalf("download grant is persisted in %s", table)
		}
	}
}

func quoteIdentifier(name string) string {
	return `"` + name + `"`
}

// TestPrivateStorageRefusesTraversalOutsideRoot proves the storage adapter
// cannot be walked out of its root.
//
// The object key is server-minted today, so this is defence in depth: a defect
// elsewhere that let a caller influence the key must not become a write into
// another tenant's directory or outside the storage root entirely.
func TestPrivateStorageRefusesTraversalOutsideRoot(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "objects")
	storage, err := uploadclients.NewFilesystemPrivateStorage(root)
	if err != nil {
		t.Fatal(err)
	}

	if err := storage.Put(context.Background(), "../../escaped", "application/pdf", pdfFile(8)); err != nil {
		t.Fatalf("put object: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "escaped")); !os.IsNotExist(err) {
		t.Fatal("traversal escaped the private storage root")
	}
	if _, err := os.Stat(filepath.Join(root, "escaped")); err != nil {
		t.Fatalf("expected the write to be confined to the root: %v", err)
	}
}
