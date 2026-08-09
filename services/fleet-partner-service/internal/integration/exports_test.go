package integration_test

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	exportshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/http"
	exportsmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/models"
	exportsrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/repositories"
	exportsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/usecases"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ---------------------------------------------------------------------------
// Section 25D generic export framework — fixture and test doubles.
//
// The Exporter and MediaPort ports are the seam this feature was built
// around specifically so a generic export test never has to reach into
// another domain's tables. These fakes exercise exactly that seam.
// ---------------------------------------------------------------------------

type fakeExporter struct {
	rows [][]string
	err  error
}

func (f *fakeExporter) Type() string      { return exportsmodels.TypeOrders }
func (f *fakeExporter) Columns() []string { return []string{"id", "name"} }
func (f *fakeExporter) Rows(_ context.Context, _ tenant.Context, _ map[string]any,
	maxRows int) ([][]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	rows := f.rows
	if len(rows) > maxRows {
		rows = rows[:maxRows]
	}
	return rows, nil
}

type fakeExportMedia struct {
	mu           sync.Mutex
	files        map[string][]byte
	failWrite    bool
	failDownload bool
}

func newFakeExportMedia() *fakeExportMedia { return &fakeExportMedia{files: map[string][]byte{}} }

func (m *fakeExportMedia) WriteExportFile(_ context.Context, _ tenant.Context, exportID string,
	content []byte) (string, error) {
	if m.failWrite {
		return "", errors.New("write failed")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	id := "mobj_" + exportID
	m.files[id] = content
	return id, nil
}

func (m *fakeExportMedia) AuthorizeDownload(_ context.Context, _ tenant.Context,
	mediaFileID string) (exportsusecases.DownloadGrant, error) {
	if m.failDownload {
		return exportsusecases.DownloadGrant{}, errors.New("download failed")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	content, ok := m.files[mediaFileID]
	if !ok {
		return exportsusecases.DownloadGrant{}, errors.New("media object not found")
	}
	return exportsusecases.DownloadGrant{
		Token: "tok_" + mediaFileID, ExpiresAt: time.Now().Add(5 * time.Minute),
		ContentType: "text/csv", SizeBytes: int64(len(content)),
	}, nil
}

func (m *fakeExportMedia) content(mediaFileID string) []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.files[mediaFileID]
}

type exportsFixture struct {
	pool    *pgxpool.Pool
	service *exportsusecases.Service
	media   *fakeExportMedia
	orgID   string
}

func newExportsFixture(t *testing.T, name, registration string, exporter exportsusecases.Exporter) *exportsFixture {
	t.Helper()
	pool := testPool(t)
	org := createOrganization(t, pool, name, registration)
	media := newFakeExportMedia()
	exporters := map[string]exportsusecases.Exporter{}
	if exporter != nil {
		exporters[exporter.Type()] = exporter
	}
	service := exportsusecases.NewService(exportsusecases.Options{
		Pool: pool, Repository: exportsrepos.NewRepository(pool), Exporters: exporters, Media: media,
		Metrics: observability.NewMetrics(),
		Flags:   exportsusecases.Flags{Enabled: true, WritesEnabled: true, WorkerEnabled: true},
		MaxRows: 10, RetentionTTL: time.Hour,
		Now: time.Now,
	})
	return &exportsFixture{pool: pool, service: service, media: media, orgID: org.ID}
}

func (f *exportsFixture) actor(memberID string) tenant.Context {
	return tenant.Context{OrganizationID: f.orgID, OrganizationUserID: memberID,
		ActorType: tenant.ActorTypeOrganizationUser, ActorID: memberID}
}

func (f *exportsFixture) router(t *testing.T, memberID string) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(f.pool), nil, 0)
	store := idempotency.NewStore(f.pool, 0)

	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(middleware.TenantContextKey, tenant.Context{
			OrganizationID: f.orgID, OrganizationUserID: memberID,
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: memberID,
		})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	group := engine.Group("/fleet-partner/v1")
	exportshttp.NewHandler(f.service, authorizer, store).RegisterTenantRoutes(group)
	return engine
}

const genOwner = "exports-test-worker"

func (f *exportsFixture) generate(cfg exportsusecases.WorkerConfig) (exportsusecases.WorkerResult, error) {
	if cfg.Owner == "" {
		cfg.Owner = genOwner
	}
	return f.service.RunGenerationPass(context.Background(), cfg)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestExports_RequestIsIdempotentAndFingerprinted(t *testing.T) {
	f := newExportsFixture(t, "Exports Idem Co", "RC-EXP-1", &fakeExporter{rows: [][]string{{"1", "a"}}})
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "requester")
	ctx := context.Background()

	input := exportsusecases.RequestExportInput{
		Actor: f.actor(member), ExportType: exportsmodels.TypeOrders, Format: exportsmodels.FormatCSV,
		Filters: map[string]any{"status": "CONFIRMED"}, IdempotencyKey: "fixed-key",
	}
	first, err := f.service.RequestExport(ctx, input)
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	if first.Status != exportsmodels.StatusPending {
		t.Fatalf("status = %s, want PENDING", first.Status)
	}
	second, err := f.service.RequestExport(ctx, input)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if second.ID != first.ID {
		t.Fatal("a retried request with the same key must replay the same job")
	}

	conflicting := input
	conflicting.Filters = map[string]any{"status": "CANCELLED"}
	if _, err := f.service.RequestExport(ctx, conflicting); err == nil {
		t.Fatal("a reused idempotency key describing a different request must be refused")
	}
}

func TestExports_UnregisteredExporterIsRefused(t *testing.T) {
	// No exporter registered for ORDERS in this fixture — the type is
	// recognized by the database CHECK constraint but nothing serves it yet.
	f := newExportsFixture(t, "Exports Unregistered Co", "RC-EXP-2", nil)
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "requester")

	_, err := f.service.RequestExport(context.Background(), exportsusecases.RequestExportInput{
		Actor: f.actor(member), ExportType: exportsmodels.TypeOrders, IdempotencyKey: "key-1",
	})
	if err == nil {
		t.Fatal("expected an export type with no registered adapter to be refused")
	}
}

func TestExports_InvalidTypeAndFormatAreRejected(t *testing.T) {
	f := newExportsFixture(t, "Exports Invalid Co", "RC-EXP-3", &fakeExporter{})
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "requester")

	if _, err := f.service.RequestExport(context.Background(), exportsusecases.RequestExportInput{
		Actor: f.actor(member), ExportType: "SHIPMENTS", IdempotencyKey: "key-a",
	}); err == nil {
		t.Fatal("an unrecognized export type must be rejected")
	}
	if _, err := f.service.RequestExport(context.Background(), exportsusecases.RequestExportInput{
		Actor: f.actor(member), ExportType: exportsmodels.TypeOrders, Format: "XLSX", IdempotencyKey: "key-b",
	}); err == nil {
		t.Fatal("an unsupported format must be rejected")
	}
}

func TestExports_WorkerGeneratesFileAndDownloadReauthorizes(t *testing.T) {
	f := newExportsFixture(t, "Exports Generate Co", "RC-EXP-4",
		&fakeExporter{rows: [][]string{{"1", "Alpha"}, {"2", "Bravo"}}})
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "requester")
	ctx := context.Background()

	job, err := f.service.RequestExport(ctx, exportsusecases.RequestExportInput{
		Actor: f.actor(member), ExportType: exportsmodels.TypeOrders, IdempotencyKey: "gen-key",
	})
	if err != nil {
		t.Fatalf("request: %v", err)
	}

	result, err := f.generate(exportsusecases.WorkerConfig{Lease: time.Minute, MaxAttempts: 8})
	if err != nil {
		t.Fatalf("generation pass: %v", err)
	}
	if result.Claimed != 1 || result.Completed != 1 {
		t.Fatalf("generation result = %+v, want 1 claimed and 1 completed", result)
	}

	completed, err := f.service.Get(ctx, f.orgID, job.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if completed.Status != exportsmodels.StatusReady {
		t.Fatalf("status = %s, want READY", completed.Status)
	}
	if completed.RowCount != 2 {
		t.Fatalf("rowCount = %d, want 2", completed.RowCount)
	}
	if completed.MediaFileID == "" {
		t.Fatal("no media file id recorded")
	}

	content := f.media.content(completed.MediaFileID)
	if content == nil {
		t.Fatal("no file content was written to media")
	}
	if got := string(content); got != "id,name\n1,Alpha\n2,Bravo\n" {
		t.Fatalf("csv content = %q", got)
	}

	grant, err := f.service.RequestDownloadURL(ctx, f.actor(member), job.ID)
	if err != nil {
		t.Fatalf("download url: %v", err)
	}
	if grant.Token == "" {
		t.Fatal("no download token issued")
	}
}

func TestExports_BoundedGenerationTruncatesToMaxRows(t *testing.T) {
	var rows [][]string
	for i := 0; i < 25; i++ {
		rows = append(rows, []string{"row", "x"})
	}
	f := newExportsFixture(t, "Exports Bound Co", "RC-EXP-5", &fakeExporter{rows: rows})
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "requester")
	ctx := context.Background()

	job, err := f.service.RequestExport(ctx, exportsusecases.RequestExportInput{
		Actor: f.actor(member), ExportType: exportsmodels.TypeOrders, IdempotencyKey: "bound-key",
	})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := f.generate(exportsusecases.WorkerConfig{Lease: time.Minute, MaxAttempts: 8}); err != nil {
		t.Fatalf("generation pass: %v", err)
	}
	completed, err := f.service.Get(ctx, f.orgID, job.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if completed.RowCount != 10 {
		t.Fatalf("rowCount = %d, want the fixture's MaxRows bound of 10", completed.RowCount)
	}
}

func TestExports_FailedGenerationRetriesThenDeadLettersThenReplays(t *testing.T) {
	exporter := &fakeExporter{err: errors.New("dataset unavailable")}
	f := newExportsFixture(t, "Exports Retry Co", "RC-EXP-6", exporter)
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "requester")
	ctx := context.Background()

	job, err := f.service.RequestExport(ctx, exportsusecases.RequestExportInput{
		Actor: f.actor(member), ExportType: exportsmodels.TypeOrders, IdempotencyKey: "retry-key",
	})
	if err != nil {
		t.Fatalf("request: %v", err)
	}

	cfg := exportsusecases.WorkerConfig{Lease: time.Minute, Backoff: time.Millisecond, MaxAttempts: 2}
	// Attempt 1: FAILED (retryable).
	if _, err := f.generate(cfg); err != nil {
		t.Fatalf("pass 1: %v", err)
	}
	after1, err := f.service.Get(ctx, f.orgID, job.ID)
	if err != nil {
		t.Fatalf("get after pass 1: %v", err)
	}
	if after1.Status != exportsmodels.StatusFailed {
		t.Fatalf("status after pass 1 = %s, want FAILED", after1.Status)
	}

	time.Sleep(5 * time.Millisecond) // clear the (tiny) backoff window
	// Attempt 2 reaches MaxAttempts: DEAD_LETTER.
	if _, err := f.generate(cfg); err != nil {
		t.Fatalf("pass 2: %v", err)
	}
	after2, err := f.service.Get(ctx, f.orgID, job.ID)
	if err != nil {
		t.Fatalf("get after pass 2: %v", err)
	}
	if after2.Status != exportsmodels.StatusDeadLetter {
		t.Fatalf("status after pass 2 = %s, want DEAD_LETTER", after2.Status)
	}

	// A dead-lettered job is never picked up by an ordinary generation pass.
	if _, err := f.generate(cfg); err != nil {
		t.Fatalf("pass 3: %v", err)
	}
	stillDead, err := f.service.Get(ctx, f.orgID, job.ID)
	if err != nil {
		t.Fatalf("get after pass 3: %v", err)
	}
	if stillDead.Status != exportsmodels.StatusDeadLetter {
		t.Fatalf("a dead-lettered job must not be auto-claimed, got %s", stillDead.Status)
	}

	// Operator replay brings it back, and the underlying problem being fixed
	// (a working exporter now) lets it complete.
	if err := f.service.Replay(ctx, f.orgID, job.ID); err != nil {
		t.Fatalf("replay: %v", err)
	}
	exporter.err = nil
	exporter.rows = [][]string{{"1", "Recovered"}}
	if _, err := f.generate(cfg); err != nil {
		t.Fatalf("pass 4: %v", err)
	}
	recovered, err := f.service.Get(ctx, f.orgID, job.ID)
	if err != nil {
		t.Fatalf("get after pass 4: %v", err)
	}
	if recovered.Status != exportsmodels.StatusReady {
		t.Fatalf("status after replay+success = %s, want READY", recovered.Status)
	}

	// Replaying anything that is not dead-lettered is refused.
	if err := f.service.Replay(ctx, f.orgID, job.ID); err == nil {
		t.Fatal("replaying a READY job must be refused")
	}
}

func TestExports_CancelIsVersionGuarded(t *testing.T) {
	f := newExportsFixture(t, "Exports Cancel Co", "RC-EXP-7", &fakeExporter{})
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "requester")
	ctx := context.Background()

	job, err := f.service.RequestExport(ctx, exportsusecases.RequestExportInput{
		Actor: f.actor(member), ExportType: exportsmodels.TypeOrders, IdempotencyKey: "cancel-key",
	})
	if err != nil {
		t.Fatalf("request: %v", err)
	}

	if _, err := f.service.Cancel(ctx, f.orgID, job.ID, job.Version+1); err == nil {
		t.Fatal("a stale expected version must be refused")
	}
	cancelled, err := f.service.Cancel(ctx, f.orgID, job.ID, job.Version)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if cancelled.Status != exportsmodels.StatusCancelled {
		t.Fatalf("status = %s, want CANCELLED", cancelled.Status)
	}
	if _, err := f.service.Cancel(ctx, f.orgID, job.ID, cancelled.Version); err == nil {
		t.Fatal("cancelling an already-terminal job must be refused")
	}

	if _, err := f.service.RequestDownloadURL(ctx, f.actor(member), job.ID); err == nil {
		t.Fatal("a cancelled export must not be downloadable")
	}
}

func TestExports_ExpiryBlocksDownloadOfStaleReadyJobs(t *testing.T) {
	f := newExportsFixture(t, "Exports Expiry Co", "RC-EXP-8", &fakeExporter{rows: [][]string{{"1", "A"}}})
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "requester")
	ctx := context.Background()

	job, err := f.service.RequestExport(ctx, exportsusecases.RequestExportInput{
		Actor: f.actor(member), ExportType: exportsmodels.TypeOrders, IdempotencyKey: "expiry-key",
	})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := f.generate(exportsusecases.WorkerConfig{Lease: time.Minute, MaxAttempts: 8}); err != nil {
		t.Fatalf("generation pass: %v", err)
	}
	if _, err := f.pool.Exec(ctx,
		`UPDATE fleet_partner_export_jobs SET expires_at = now() - interval '1 hour' WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("force expiry: %v", err)
	}

	affected, err := f.service.RunExpiryPass(ctx, 200)
	if err != nil {
		t.Fatalf("expiry pass: %v", err)
	}
	if affected < 1 {
		t.Fatalf("expiry pass affected %d rows, want at least 1", affected)
	}

	expired, err := f.service.Get(ctx, f.orgID, job.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if expired.Status != exportsmodels.StatusExpired {
		t.Fatalf("status = %s, want EXPIRED", expired.Status)
	}
	if _, err := f.service.RequestDownloadURL(ctx, f.actor(member), job.ID); err == nil {
		t.Fatal("an expired export must not be downloadable")
	}
}

func TestExports_TenantIsolation(t *testing.T) {
	f := newExportsFixture(t, "Exports Tenant A Co", "RC-EXP-9A", &fakeExporter{})
	other := createOrganization(t, f.pool, "Exports Tenant B Co", "RC-EXP-9B")
	member := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "requester")
	ctx := context.Background()

	job, err := f.service.RequestExport(ctx, exportsusecases.RequestExportInput{
		Actor: f.actor(member), ExportType: exportsmodels.TypeOrders, IdempotencyKey: "tenant-key",
	})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := f.service.Get(ctx, other.ID, job.ID); err == nil {
		t.Fatal("a job created in one organization must not be readable from another")
	}
}

// TestExports_HTTPLifecycle exercises the real routes end to end: creation
// requires an Idempotency-Key, permission is enforced per route, and cancel
// is reachable through the wired handler exactly as a client would use it.
func TestExports_HTTPLifecycle(t *testing.T) {
	f := newExportsFixture(t, "Exports HTTP Co", "RC-EXP-10", &fakeExporter{rows: [][]string{{"1", "A"}}})
	owner := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "owner")
	router := f.router(t, owner)

	// Creating without an Idempotency-Key is refused.
	if status, _ := documentCall(t, router, http.MethodPost, "/fleet-partner/v1/exports",
		map[string]any{"exportType": exportsmodels.TypeOrders}, nil); status != http.StatusBadRequest {
		t.Fatalf("create without idempotency key status = %d, want 400", status)
	}

	status, response := documentCall(t, router, http.MethodPost, "/fleet-partner/v1/exports",
		map[string]any{"exportType": exportsmodels.TypeOrders},
		map[string]string{"Idempotency-Key": "http-key-1"})
	if status != http.StatusAccepted {
		t.Fatalf("create status = %d body=%v, want 202", status, response)
	}
	created := documentData(t, response)
	exportID, _ := created["id"].(string)
	if exportID == "" {
		t.Fatalf("create response carries no id: %v", created)
	}

	// A retried create with the same key replays the same job rather than
	// creating a second one.
	status, replay := documentCall(t, router, http.MethodPost, "/fleet-partner/v1/exports",
		map[string]any{"exportType": exportsmodels.TypeOrders},
		map[string]string{"Idempotency-Key": "http-key-1"})
	if status != http.StatusAccepted {
		t.Fatalf("replay status = %d, want 202", status)
	}
	if documentData(t, replay)["id"] != exportID {
		t.Fatal("a retried create must replay the same export job")
	}

	if status, response := documentCall(t, router, http.MethodGet,
		"/fleet-partner/v1/exports/"+exportID, nil, nil); status != http.StatusOK {
		t.Fatalf("get status = %d body=%v, want 200", status, response)
	}
	if status, response := documentCall(t, router, http.MethodGet,
		"/fleet-partner/v1/exports", nil, nil); status != http.StatusOK {
		t.Fatalf("list status = %d body=%v, want 200", status, response)
	}

	// A member with no role bindings reaches nothing.
	nobody := seedApprovalMember(t, f.pool, f.orgID, "", "nobody")
	nobodyRouter := f.router(t, nobody)
	if status, _ := documentCall(t, nobodyRouter, http.MethodGet,
		"/fleet-partner/v1/exports/"+exportID, nil, nil); status != http.StatusForbidden {
		t.Fatalf("unroled get status = %d, want 403", status)
	}

	status, cancelled := documentCall(t, router, http.MethodPost,
		"/fleet-partner/v1/exports/"+exportID+"/cancel", map[string]any{"expectedVersion": 1},
		map[string]string{"Idempotency-Key": "http-cancel-1"})
	if status != http.StatusOK {
		t.Fatalf("cancel status = %d body=%v, want 200", status, cancelled)
	}
	if documentData(t, cancelled)["status"] != exportsmodels.StatusCancelled {
		t.Fatalf("cancelled export status = %v, want CANCELLED", documentData(t, cancelled)["status"])
	}
}
