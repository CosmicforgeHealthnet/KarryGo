package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	fleetevents "cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	documenthttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/http"
	documentrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/repositories"
	documentusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/usecases"
	eligibilityrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/repositories"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// The WP10 document suite.
//
// Media's own behaviour — file signature validation, checksum comparison,
// safety states, storage — is proven by media-file-service's own PostgreSQL
// suite against real files. What is proven HERE is the fleet half and the wire
// between them: that fleet signs correctly, that the contract fields line up,
// that fleet's projection of media's answer obeys the ordering and conflict
// rules, and that no fleet path treats an unconfirmed file as usable.
//
// The media endpoint below is a real HTTP server verifying real HMAC
// signatures with the shared serviceauth verifier and enforcing the same
// caller allowlist and nonce replay gate the live service does. It is not a
// mock of fleet's client: fleet's actual client code makes actual signed HTTP
// calls to it, and a signing regression fails these tests.

const (
	documentMediaCaller = "fleet-partner-service"
	documentMediaSecret = "fleet-media-integration-secret"
)

// mediaObjectRecord is the stub authority's view of one object.
type mediaObjectRecord struct {
	ID            string
	Organization  string
	OwnerType     string
	OwnerRef      string
	UploadStatus  string
	SafetyStatus  string
	DeletionState string
	DetectedMIME  string
	Checksum      string
	SizeBytes     int64
	SourceVersion int64
	LegalHold     bool
	Retention     *time.Time
	Content       []byte
}

// mediaTestServer is a real HTTP implementation of the private media contract.
type mediaTestServer struct {
	t        *testing.T
	server   *httptest.Server
	verifier *serviceauth.Verifier

	mu      sync.Mutex
	objects map[string]*mediaObjectRecord
	grants  map[string]string // upload authorization id -> media object id
	nonces  map[string]bool
	tokens  map[string]time.Time

	// Behaviour switches used by individual tests.
	failDeletion      bool
	timeoutDeletion   bool
	blockDeletion     string
	refuseDownload    bool
	unavailable       bool
	deletionCalls     int
	uploadAuthCalls   int
	readObjectCalls   int
	nextSafetyOnRead  string
	nextVersionOnRead int64
}

func newMediaTestServer(t *testing.T) *mediaTestServer {
	t.Helper()
	media := &mediaTestServer{
		t:        t,
		verifier: serviceauth.NewVerifier(serviceauth.Secrets{documentMediaCaller: []byte(documentMediaSecret)}, 5*time.Minute),
		objects:  map[string]*mediaObjectRecord{},
		grants:   map[string]string{},
		nonces:   map[string]bool{},
		tokens:   map[string]time.Time{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", media.route)
	media.server = httptest.NewServer(mux)
	t.Cleanup(media.server.Close)
	return media
}

func (m *mediaTestServer) URL() string { return m.server.URL }

// authenticate applies the same gates the live private contract applies.
func (m *mediaTestServer) authenticate(w http.ResponseWriter, r *http.Request) bool {
	name, err := m.verifier.VerifyRequest(r)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	if name != documentMediaCaller {
		w.WriteHeader(http.StatusForbidden)
		return false
	}
	nonce := r.Header.Get("X-Service-Nonce")
	if nonce == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.nonces[nonce] {
		w.WriteHeader(http.StatusConflict)
		return false
	}
	m.nonces[nonce] = true
	return true
}

func (m *mediaTestServer) writeData(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": value})
}

func (m *mediaTestServer) route(w http.ResponseWriter, r *http.Request) {
	if !m.authenticate(w, r) {
		return
	}
	if m.unavailable {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	const base = "/api/v1/media-files/internal/media"
	path := strings.TrimPrefix(r.URL.Path, base)

	switch {
	case r.Method == http.MethodPost && path == "/upload-authorizations":
		m.handleAuthorizeUpload(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/download-authorizations"):
		m.handleAuthorizeDownload(w, objectIDFrom(path, "/download-authorizations"))
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/deletions"):
		m.handleDeletion(w, objectIDFrom(path, "/deletions"))
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/deletion"):
		m.handleDeletionState(w, objectIDFrom(path, "/deletion"))
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/retention"):
		m.handleRetention(w, r, objectIDFrom(path, "/retention"))
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/content"):
		m.handleContent(w, r, objectIDFrom(path, "/content"))
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/objects/"):
		m.handleReadObject(w, strings.TrimPrefix(path, "/objects/"))
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func objectIDFrom(path string, suffix string) string {
	trimmed := strings.TrimSuffix(path, suffix)
	return strings.TrimPrefix(trimmed, "/objects/")
}

func (m *mediaTestServer) handleAuthorizeUpload(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OwnerType      string `json:"ownerType"`
		OwnerReference string `json:"ownerReference"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	// The contract's own requirements, enforced so a fleet regression that
	// stopped sending them fails here rather than in production.
	if r.Header.Get("X-Request-Fingerprint") == "" || r.Header.Get("Idempotency-Key") == "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}
	if r.Header.Get("X-Organization-ID") == "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	m.mu.Lock()
	m.uploadAuthCalls++
	objectID := "mobj_" + randomHex()
	authID := "mauz_" + randomHex()
	m.objects[objectID] = &mediaObjectRecord{
		ID: objectID, Organization: r.Header.Get("X-Organization-ID"),
		OwnerType: body.OwnerType, OwnerRef: body.OwnerReference,
		UploadStatus: "AUTHORIZED", SafetyStatus: "PENDING", DeletionState: "ACTIVE",
		SourceVersion: 1,
	}
	m.grants[authID] = objectID
	m.mu.Unlock()

	m.writeData(w, map[string]any{
		"uploadAuthorizationId": authID,
		"mediaObjectId":         objectID,
		"uploadToken":           "upl_" + randomHex(),
		"uploadPath":            "/objects/" + objectID + "/content",
		"expiresAt":             time.Now().UTC().Add(15 * time.Minute).Format(time.RFC3339),
		"maxSizeBytes":          int64(1 << 20),
		"allowedMimeTypes":      []string{"application/pdf"},
	})
}

func (m *mediaTestServer) handleReadObject(w http.ResponseWriter, objectID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readObjectCalls++
	object, ok := m.objects[objectID]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if m.nextSafetyOnRead != "" {
		object.SafetyStatus = m.nextSafetyOnRead
	}
	if m.nextVersionOnRead > 0 {
		object.SourceVersion = m.nextVersionOnRead
	}
	m.writeData(w, m.viewLocked(object))
}

func (m *mediaTestServer) viewLocked(object *mediaObjectRecord) map[string]any {
	return map[string]any{
		"mediaObjectId":  object.ID,
		"organizationId": object.Organization,
		"ownerService":   documentMediaCaller,
		"ownerType":      object.OwnerType,
		"ownerReference": object.OwnerRef,
		"detectedMime":   object.DetectedMIME,
		"sizeBytes":      object.SizeBytes,
		"checksumSha256": object.Checksum,
		"uploadStatus":   object.UploadStatus,
		"safetyStatus":   object.SafetyStatus,
		"deletionState":  object.DeletionState,
		"legalHold":      object.LegalHold,
		"sourceVersion":  object.SourceVersion,
	}
}

func (m *mediaTestServer) handleAuthorizeDownload(w http.ResponseWriter, objectID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	object, ok := m.objects[objectID]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if m.refuseDownload || object.SafetyStatus == "UNSAFE" ||
		object.SafetyStatus == "QUARANTINED" || object.DeletionState == "DELETED" {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	token := strconv.FormatInt(time.Now().Add(5*time.Minute).Unix(), 10) + "." + randomHex()
	m.tokens[token] = time.Now().Add(5 * time.Minute)
	m.writeData(w, map[string]any{
		"mediaObjectId": objectID,
		"downloadToken": token,
		"downloadPath":  "/objects/" + objectID + "/content",
		"expiresAt":     time.Now().UTC().Add(5 * time.Minute).Format(time.RFC3339),
		"contentType":   object.DetectedMIME,
		"sizeBytes":     object.SizeBytes,
	})
}

func (m *mediaTestServer) handleContent(w http.ResponseWriter, r *http.Request, objectID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	token := r.Header.Get("X-Download-Token")
	expiry, ok := m.tokens[token]
	if !ok || time.Now().After(expiry) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	object, ok := m.objects[objectID]
	if !ok || object.DeletionState == "DELETED" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	_, _ = w.Write(object.Content)
}

func (m *mediaTestServer) handleDeletion(w http.ResponseWriter, objectID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deletionCalls++
	object, ok := m.objects[objectID]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if m.timeoutDeletion {
		// Simulate a call whose outcome fleet never learns. The connection is
		// closed without a response, which the client classifies as transient
		// or unknown depending on where it failed.
		hijacker, hijackable := w.(http.Hijacker)
		if hijackable {
			conn, _, _ := hijacker.Hijack()
			_ = conn.Close()
			return
		}
		w.WriteHeader(http.StatusGatewayTimeout)
		return
	}
	if m.failDeletion {
		m.writeData(w, map[string]any{
			"deletionRequestId": "mdel_" + randomHex(), "mediaObjectId": objectID,
			"status": "FAILED",
		})
		return
	}
	if m.blockDeletion != "" || object.LegalHold {
		reason := m.blockDeletion
		if reason == "" {
			reason = "LEGAL_HOLD"
		}
		m.writeData(w, map[string]any{
			"deletionRequestId": "mdel_" + randomHex(), "mediaObjectId": objectID,
			"status": "BLOCKED", "blockReason": reason,
		})
		return
	}
	object.DeletionState = "DELETED"
	object.SourceVersion++
	object.Content = nil
	m.writeData(w, map[string]any{
		"deletionRequestId": "mdel_" + randomHex(), "mediaObjectId": objectID,
		"status": "COMPLETED",
	})
}

func (m *mediaTestServer) handleDeletionState(w http.ResponseWriter, objectID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	object, ok := m.objects[objectID]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	m.writeData(w, map[string]any{
		"mediaObjectId": objectID,
		"deletionState": object.DeletionState,
		"sourceVersion": object.SourceVersion,
	})
}

func (m *mediaTestServer) handleRetention(w http.ResponseWriter, r *http.Request, objectID string) {
	var body struct {
		RetentionClass string `json:"retentionClass"`
		RetentionUntil string `json:"retentionUntil"`
		LegalHold      *bool  `json:"legalHold"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	m.mu.Lock()
	defer m.mu.Unlock()
	object, ok := m.objects[objectID]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if body.LegalHold != nil {
		object.LegalHold = *body.LegalHold
	}
	if body.RetentionUntil != "" {
		if parsed, err := time.Parse(time.RFC3339, body.RetentionUntil); err == nil {
			object.Retention = &parsed
		}
	}
	object.SourceVersion++
	m.writeData(w, m.viewLocked(object))
}

// completeUpload moves a media object to the state a successful upload leaves
// it in, as media itself would after validating the bytes.
func (m *mediaTestServer) completeUpload(objectID string, safety string, content []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	object, ok := m.objects[objectID]
	if !ok {
		m.t.Fatalf("media object %s does not exist", objectID)
	}
	sum := sha256.Sum256(content)
	object.UploadStatus = "COMPLETED"
	object.SafetyStatus = safety
	object.DetectedMIME = "application/pdf"
	object.SizeBytes = int64(len(content))
	object.Checksum = hex.EncodeToString(sum[:])
	object.Content = content
	object.SourceVersion++
}

func (m *mediaTestServer) objectVersion(objectID string) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if object, ok := m.objects[objectID]; ok {
		return object.SourceVersion
	}
	return 0
}

func (m *mediaTestServer) setSafety(objectID string, safety string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if object, ok := m.objects[objectID]; ok {
		object.SafetyStatus = safety
		object.SourceVersion++
	}
}

func (m *mediaTestServer) setVersion(objectID string, version int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if object, ok := m.objects[objectID]; ok {
		object.SourceVersion = version
	}
}

func randomHex() string {
	var raw [16]byte
	_, _ = rand.Read(raw[:])
	return hex.EncodeToString(raw[:])
}

// --- fleet fixture --------------------------------------------------------

type documentFixture struct {
	pool     *pgxpool.Pool
	media    *mediaTestServer
	service  *documentusecases.Service
	orgID    string
	branchID string
	client   *platformclients.MediaFileClient
}

func newDocumentFixture(t *testing.T, name string, registration string) *documentFixture {
	t.Helper()
	pool := testPool(t)
	org := createOrganization(t, pool, name, registration)
	media := newMediaTestServer(t)

	client := platformclients.NewMediaFileClient(platformclients.Options{
		ServiceName: "media-file-service",
		BaseURL:     media.URL(),
		Secret:      []byte(documentMediaSecret),
		Timeout:     5 * time.Second,
	})

	fixture := &documentFixture{pool: pool, media: media, orgID: org.ID, client: client}
	fixture.service = fixture.newService(client)
	return fixture
}

func (f *documentFixture) newService(media platformclients.MediaAuthority) *documentusecases.Service {
	return documentusecases.NewService(documentusecases.Options{
		Pool:        f.pool,
		Documents:   documentrepos.NewRepository(f.pool),
		Media:       media,
		Events:      fleetevents.NewDomainPublisher(fleetevents.NewOutboxRepository(f.pool)),
		Eligibility: eligibilityrepos.NewRepository(f.pool),
		Memberships: app.NewDocumentsMembershipLookup(providerrepos.NewRepository(f.pool)),
		Flags: documentusecases.Flags{
			WritesEnabled:         true,
			DownloadEnabled:       true,
			EventIngestionEnabled: true,
		},
		StaleAfter: time.Minute,
	})
}

// actor returns a corporate actor for the fixture's organization.
func (f *documentFixture) actor(memberID string) documentusecases.Actor {
	return documentusecases.Actor{
		OrganizationID: f.orgID,
		ActorType:      "ORGANIZATION_USER",
		ActorID:        memberID,
	}
}

// createDocument runs the real upload orchestration against the media server.
func (f *documentFixture) createDocument(t *testing.T, memberID string, documentType string,
	expiry *time.Time) *documentusecases.UploadTicket {
	t.Helper()
	ticket, err := f.service.RequestUpload(context.Background(), documentusecases.RequestUploadInput{
		Actor:            f.actor(memberID),
		OwnerType:        "COMPANY",
		OwnerID:          f.orgID,
		Category:         "INSURANCE",
		DocumentType:     documentType,
		ExpiryDate:       expiry,
		AllowedMIMETypes: []string{"application/pdf"},
		MaxSizeBytes:     1 << 20,
		IdempotencyKey:   "doc-" + documentType + "-" + randomHex(),
		CorrelationID:    "cor-" + randomHex(),
	})
	if err != nil {
		t.Fatalf("request upload: %v", err)
	}
	return ticket
}

// deliverMediaEvent hands fleet a media event exactly as the outbox publisher
// would, through the real ingestion path.
func (f *documentFixture) deliverMediaEvent(t *testing.T, mediaObjectID string, eventType string,
	sourceVersion int64, overrides map[string]any) (string, error) {
	t.Helper()
	payload := map[string]any{
		"mediaObjectId":  mediaObjectID,
		"organizationId": f.orgID,
		"uploadStatus":   "COMPLETED",
		"safetyStatus":   "SAFE",
		"deletionState":  "ACTIVE",
		"detectedMime":   "application/pdf",
		"sizeBytes":      1024,
		"checksumSha256": strings.Repeat("a", 64),
		"sourceVersion":  sourceVersion,
	}
	for key, value := range overrides {
		payload[key] = value
	}
	return f.deliverMediaEventWithID(t, mediaObjectID, eventType, sourceVersion,
		"mevt_"+randomHex(), payload)
}

func (f *documentFixture) deliverMediaEventWithID(t *testing.T, mediaObjectID string,
	eventType string, sourceVersion int64, eventID string, payload map[string]any) (string, error) {
	t.Helper()
	encodedPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	envelope := fleetevents.Envelope{
		EventID:          eventID,
		EventType:        eventType,
		SchemaVersion:    1,
		AggregateType:    "MEDIA_OBJECT",
		AggregateID:      mediaObjectID,
		AggregateVersion: sourceVersion,
		OrganizationID:   f.orgID,
		CorrelationID:    "cor-" + randomHex(),
		OccurredAt:       time.Now().UTC(),
		Producer:         "media-file-service",
		Payload:          encodedPayload,
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return f.service.IngestMediaEvent(context.Background(), raw)
}

// router mounts the real document routes for one member.
func (f *documentFixture) router(t *testing.T, memberID string, branchIDs []string) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(f.pool), nil, 0)

	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(middleware.TenantContextKey, tenant.Context{
			OrganizationID: f.orgID, OrganizationUserID: memberID,
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: memberID,
			PermittedBranchIDs: branchIDs})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	group := engine.Group("/fleet-partner/v1")
	documenthttp.NewHandler(f.service, authorizer).RegisterTenantRoutes(group)
	return engine
}

// documentCall performs one request against the corporate routes.
func documentCall(t *testing.T, router http.Handler, method string, path string,
	body any, headers map[string]string) (int, map[string]any) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	response := map[string]any{}
	_ = json.Unmarshal(recorder.Body.Bytes(), &response)
	return recorder.Code, response
}

func documentData(t *testing.T, response map[string]any) map[string]any {
	t.Helper()
	data, ok := response["data"].(map[string]any)
	if !ok {
		t.Fatalf("response carries no data object: %v", response)
	}
	return data
}

// readDocumentRow reads one column from the document row.
func readDocumentColumn(t *testing.T, pool *pgxpool.Pool, documentID string, column string) string {
	t.Helper()
	var value string
	if err := pool.QueryRow(context.Background(),
		"SELECT COALESCE("+column+"::text, '') FROM fleet_partner_documents WHERE id = $1",
		documentID).Scan(&value); err != nil {
		t.Fatalf("read %s: %v", column, err)
	}
	return value
}

// assertNotPersisted proves a short-lived credential left no durable copy in
// any fleet document table, the audit trail or the outbox.
func assertNotPersisted(t *testing.T, pool *pgxpool.Pool, secret string) {
	t.Helper()
	if secret == "" {
		t.Fatal("assertNotPersisted needs a non-empty value to look for")
	}
	tables := []string{
		"fleet_partner_documents",
		"fleet_partner_document_history",
		"fleet_partner_document_verifications",
		"fleet_partner_document_deletion_requests",
		"fleet_partner_document_reconciliation_cases",
		"fleet_partner_document_media_versions",
		"fleet_partner_audit_records",
		"fleet_partner_outbox",
	}
	for _, table := range tables {
		var hits int
		query := "SELECT count(*) FROM " + table + " t WHERE t::text LIKE '%' || $1 || '%'"
		if err := pool.QueryRow(context.Background(), query, secret).Scan(&hits); err != nil {
			t.Fatalf("scan %s: %v", table, err)
		}
		if hits > 0 {
			t.Fatalf("short-lived credential is persisted in %s", table)
		}
	}
}

// captureLogs redirects the standard logger so a test can prove a credential
// never reaches a log line.
//
// The shared logging package writes through the standard logger, so capturing
// it covers every log statement the request path can emit.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buffer := &bytes.Buffer{}
	log.SetOutput(io.MultiWriter(buffer, os.Stderr))
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return buffer
}
