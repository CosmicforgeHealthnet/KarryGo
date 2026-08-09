package integration_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	bulkmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/models"
	bulkrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/repositories"
	bulkusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"github.com/jackc/pgx/v5/pgxpool"
)

type bulkMediaStub struct {
	file   bulkusecases.MediaFile
	err    error
	delay  time.Duration
	report []byte
}

func (s *bulkMediaStub) LoadImport(context.Context, tenant.Context, string, int64) (bulkusecases.MediaFile, error) {
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	return s.file, s.err
}
func (s *bulkMediaStub) WriteErrorReport(_ context.Context, _ tenant.Context, _ string, b []byte) (string, error) {
	s.report = append([]byte(nil), b...)
	return "media_error_report", nil
}

type bulkOrdersStub struct {
	mu    sync.Mutex
	calls int
	fail  map[int]bool
	delay time.Duration
	refs  map[string]string
}

func (s *bulkOrdersStub) CreateImported(_ context.Context, _ tenant.Context, in bulkusecases.OrderInput) (string, error) {
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.refs == nil {
		s.refs = map[string]string{}
	}
	if id := s.refs[in.SourceReference]; id != "" {
		return id, nil
	}
	if s.fail[s.calls] {
		return "", errors.New("row rejected")
	}
	id := "ord_" + strings.ReplaceAll(in.SourceReference, ":", "_")
	s.refs[in.SourceReference] = id
	return id, nil
}

type bulkFixture struct {
	pool    *pgxpool.Pool
	service *bulkusecases.Service
	repo    *bulkrepos.Repository
	tenant  tenant.Context
	media   *bulkMediaStub
	orders  *bulkOrdersStub
}

func newBulkFixture(t *testing.T, content string, maxRows int) (*bulkFixture, bulkmodels.Import) {
	t.Helper()
	pool := testPool(t)
	org := createOrganization(t, pool, "Bulk Matrix Co", "RC-BULK-MATRIX")
	branch := "brn_bulk_matrix"
	if _, e := pool.Exec(context.Background(), `INSERT INTO fleet_partner_branches(id,organization_id,code,name,timezone)VALUES($1,$2,'BULK-MATRIX','Bulk Matrix','Africa/Lagos')`, branch, org.ID); e != nil {
		t.Fatal(e)
	}
	tc := tenant.Context{OrganizationID: org.ID, OrganizationUserID: "usr_bulk", ActorType: tenant.ActorTypeOrganizationUser, ActorID: "usr_bulk", PermittedBranchIDs: []string{branch}}
	media := &bulkMediaStub{file: bulkusecases.MediaFile{Format: "CSV", ContentType: "text/csv", SizeBytes: int64(len(content)), Content: []byte(content)}}
	orders := &bulkOrdersStub{}
	repo := bulkrepos.NewRepository(pool)
	svc := bulkusecases.NewService(bulkusecases.Options{Pool: pool, Repository: repo, Media: media, Orders: orders, Audit: audit.NewRecorder(), Flags: bulkusecases.Flags{Enabled: true, WritesEnabled: true, WorkerEnabled: true}, MaxBytes: 1024, MaxRows: maxRows})
	imp, e := svc.Create(context.Background(), tc, bulkusecases.CreateInput{BranchID: branch, MediaFileID: "media_bulk_matrix", FileFormat: "CSV"})
	if e != nil {
		t.Fatal(e)
	}
	return &bulkFixture{pool, svc, repo, tc, media, orders}, imp
}

const bulkCSV = "rowReference,customerId,serviceCatalogItemId,timezone\nrow-1,cus-1,sci-1,Africa/Lagos\nrow-2,cus-2,sci-2,Africa/Lagos\n"

func TestBulkImports_ValidationLimitsStableRowsPartialSuccessAndReportMinimization(t *testing.T) {
	f, imp := newBulkFixture(t, bulkCSV, 10)
	ctx := context.Background()
	if _, e := f.service.Create(ctx, f.tenant, bulkusecases.CreateInput{BranchID: imp.BranchID, MediaFileID: imp.MediaFileID, FileFormat: "JSON"}); ordersErrorCode(e) != "bulk_import_conflict" {
		t.Fatalf("changed fingerprint err=%v", e)
	}
	r, e := f.service.RunWorkerPass(ctx, bulkusecases.WorkerConfig{Owner: "bulk-validate", Lease: time.Second, BatchSize: 1, MaxAttempts: 3})
	if e != nil || r.Validated != 1 {
		t.Fatalf("validate=%+v err=%v", r, e)
	}
	rows, e := f.repo.ListRows(ctx, nil, f.tenant.OrganizationID, imp.ID, "")
	if e != nil || len(rows) != 2 || rows[0].RowReference != "row-1" {
		t.Fatalf("rows=%+v err=%v", rows, e)
	}
	// Identical retry is harmless; changed data under the same stable row key conflicts.
	if e = f.repo.InsertRow(ctx, nil, rows[0]); e != nil {
		t.Fatal(e)
	}
	changed := rows[0]
	changed.RowFingerprint = "changed"
	if e = f.repo.InsertRow(ctx, nil, changed); !errors.Is(e, bulkrepos.ErrConflict) {
		t.Fatalf("changed row err=%v", e)
	}
	f.orders.fail = map[int]bool{2: true}
	r, e = f.service.RunWorkerPass(ctx, bulkusecases.WorkerConfig{Owner: "bulk-import", Lease: time.Second, BatchSize: 1, MaxAttempts: 3})
	if e != nil || r.Completed != 1 {
		t.Fatalf("import=%+v err=%v", r, e)
	}
	current, _, e := f.service.Get(ctx, f.tenant, imp.ID)
	if e != nil || current.Status != bulkmodels.StatusPartiallyCompleted || current.ImportedRows != 1 {
		t.Fatalf("current=%+v err=%v", current, e)
	}
	if strings.Contains(string(f.media.report), "cus-2") || !strings.Contains(string(f.media.report), "row-2") {
		t.Fatalf("unsafe report=%s", f.media.report)
	}

}

func TestBulkImports_FileSizeAndRowCountLimits(t *testing.T) {
	for name, setup := range map[string]func(*bulkFixture){"file-size": func(f *bulkFixture) { f.media.file.SizeBytes = 2048 }, "row-count": func(f *bulkFixture) {}} {
		t.Run(name, func(t *testing.T) {
			maxRows := 10
			if name == "row-count" {
				maxRows = 1
			}
			f, _ := newBulkFixture(t, bulkCSV, maxRows)
			setup(f)
			r, e := f.service.RunWorkerPass(context.Background(), bulkusecases.WorkerConfig{Owner: name, BatchSize: 1, MaxAttempts: 1})
			if e != nil || r.Failed != 1 {
				t.Fatalf("result=%+v err=%v", r, e)
			}
		})
	}
}

func TestBulkImports_HeartbeatCancellationDuringImportAndCommittedRowsPreserved(t *testing.T) {
	f, imp := newBulkFixture(t, bulkCSV, 10)
	ctx := context.Background()
	if _, e := f.service.RunWorkerPass(ctx, bulkusecases.WorkerConfig{Owner: "validate", BatchSize: 1, MaxAttempts: 3}); e != nil {
		t.Fatal(e)
	}
	f.orders.delay = 700 * time.Millisecond
	done := make(chan error, 1)
	go func() {
		_, e := f.service.RunWorkerPass(ctx, bulkusecases.WorkerConfig{Owner: "slow", Lease: 300 * time.Millisecond, BatchSize: 1, MaxAttempts: 3})
		done <- e
	}()
	time.Sleep(250 * time.Millisecond)
	current, _, e := f.service.Get(ctx, f.tenant, imp.ID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.service.Cancel(ctx, f.tenant, imp.ID, current.Version); e != nil {
		t.Fatal(e)
	}
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	current, rows, e := f.service.Get(ctx, f.tenant, imp.ID)
	if e != nil || current.Status != bulkmodels.StatusCancelled {
		t.Fatalf("current=%+v err=%v", current, e)
	}
	imported := 0
	for _, row := range rows {
		if row.Status == "IMPORTED" {
			imported++
		}
	}
	if imported != 1 {
		t.Fatalf("committed rows=%d", imported)
	}
}

func TestBulkImports_DeadLetterReplayLeaseRecoveryAndFeatureControls(t *testing.T) {
	f, imp := newBulkFixture(t, bulkCSV, 10)
	ctx := context.Background()
	f.media.err = errors.New("media unavailable")
	r, e := f.service.RunWorkerPass(ctx, bulkusecases.WorkerConfig{Owner: "dead", BatchSize: 1, MaxAttempts: 1})
	if e != nil || r.Failed != 1 {
		t.Fatalf("dead=%+v err=%v", r, e)
	}
	current, _, e := f.service.Get(ctx, f.tenant, imp.ID)
	if e != nil || current.Status != bulkmodels.StatusFailed {
		t.Fatalf("current=%+v err=%v", current, e)
	}
	f.media.err = nil
	if e = f.service.Replay(ctx, f.tenant, imp.ID); e != nil {
		t.Fatal(e)
	}
	r, e = f.service.RunWorkerPass(ctx, bulkusecases.WorkerConfig{Owner: "replay", BatchSize: 1, MaxAttempts: 3})
	if e != nil || r.Validated != 1 {
		t.Fatalf("replay=%+v err=%v", r, e)
	}
	for name, flags := range map[string]bulkusecases.Flags{"disabled": {Enabled: true, WritesEnabled: true}, "kill": {Enabled: true, WritesEnabled: true, WorkerEnabled: true, KillSwitch: true}} {
		t.Run(name, func(t *testing.T) {
			svc := bulkusecases.NewService(bulkusecases.Options{Repository: f.repo, Flags: flags})
			out, e := svc.RunWorkerPass(ctx, bulkusecases.WorkerConfig{})
			if e != nil || out.Claimed != 0 {
				t.Fatalf("out=%+v err=%v", out, e)
			}
		})
	}
}
