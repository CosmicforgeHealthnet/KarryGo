package restrictions_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/serviceauth"

	"cosmicforge/logistics/services/taxi-service/internal/database"
	"cosmicforge/logistics/services/taxi-service/internal/features/managedexec"
	"cosmicforge/logistics/services/taxi-service/internal/features/organizationproviders"
	"cosmicforge/logistics/services/taxi-service/internal/features/privateexecution"
	"cosmicforge/logistics/services/taxi-service/internal/features/restrictions"
)

// Taxi's Fleet compliance-restriction authority against real PostgreSQL.
// Run with TAXI_TEST_DATABASE_URL; every taxi migration is applied first.

const (
	fleetCaller = "fleet-partner-service"
	fleetSecret = "taxi-restrictions-test-secret"
	base        = "/api/v1/taxi/internal/restrictions"
)

var (
	migrateOnce sync.Once
	migrateErr  error
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TAXI_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TAXI_TEST_DATABASE_URL is required for taxi restriction tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	migrateOnce.Do(func() {
		// ApplyMigrations reads a relative "migrations" directory, so it only
		// resolves from the service root.
		root, err := os.Getwd()
		if err != nil {
			migrateErr = err
			return
		}
		if err = os.Chdir(filepath.Join("..", "..", "..")); err != nil {
			migrateErr = err
			return
		}
		defer func() { _ = os.Chdir(root) }()
		migrateErr = database.ApplyMigrations(context.Background(), pool)
	})
	if migrateErr != nil {
		t.Fatalf("apply migrations: %v", migrateErr)
	}
	return pool
}

type fixture struct {
	pool   *pgxpool.Pool
	server *httptest.Server
	svc    *restrictions.Service
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool := testPool(t)
	svc := restrictions.New(pool)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{fleetCaller: []byte(fleetSecret), "vendor-service": []byte(fleetSecret)}, time.Minute)
	restrictions.Register(engine.Group(base), svc,
		organizationproviders.Boundary(verifier, []string{fleetCaller}, organizationproviders.NewRepository(pool))...)
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	return &fixture{pool: pool, server: server, svc: svc}
}

type response struct {
	status int
	data   map[string]any
	code   string
}

func (f *fixture) call(t *testing.T, caller, method, path string, body any) response {
	t.Helper()
	var payload []byte
	if body != nil {
		payload, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest(method, f.server.URL+base+path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(serviceauth.HeaderNonce, uuid.NewString())
	if err := serviceauth.SignRequest(req, caller, []byte(fleetSecret), payload, time.Now()); err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var envelope struct {
		Data  map[string]any `json:"data"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &envelope)
	return response{status: res.StatusCode, data: envelope.Data, code: envelope.Error.Code}
}

// command is the body Fleet's platformclients.RestrictionClient sends.
func command(op, org, typ, ref, reason, source string, version int64) map[string]any {
	return map[string]any{"operationReference": op, "organizationId": org, "subjectType": typ,
		"subjectReference": ref, "reasonCode": reason, "sourceReference": source, "sourceVersion": version}
}

func TestTaxiRestrictionAuthorityApplyReplayConflictReadClear(t *testing.T) {
	f := newFixture(t)
	org := "org_" + uuid.NewString()
	driver := uuid.NewString()
	source := "rst_" + uuid.NewString()
	apply := command(source+":apply", org, "PROVIDER", driver, "SAFETY_BLOCK", source, 1)

	first := f.call(t, fleetCaller, http.MethodPost, "/apply", apply)
	if first.status != http.StatusOK || first.data["requestedState"] != "APPLIED" || first.data["outcome"] != "CONFIRMED" || first.data["restrictionId"] == "" {
		t.Fatalf("apply = %+v", first)
	}
	// Idempotent replay returns the original outcome and changes nothing.
	replay := f.call(t, fleetCaller, http.MethodPost, "/apply", apply)
	if replay.status != http.StatusOK || replay.data["restrictionId"] != first.data["restrictionId"] ||
		replay.data["restrictionVersion"] != first.data["restrictionVersion"] {
		t.Fatalf("replay = %+v, first = %+v", replay, first)
	}
	// The same operation reference with a different command is a conflict.
	changed := command(source+":apply", org, "PROVIDER", driver, "OTHER_REASON", source, 1)
	if got := f.call(t, fleetCaller, http.MethodPost, "/apply", changed); got.status != http.StatusConflict || got.code != "CONFLICT" {
		t.Fatalf("conflicting replay = %+v", got)
	}
	// Fleet's read-back of an ambiguous apply.
	op := f.call(t, fleetCaller, http.MethodGet, "/operations/"+source+":apply", nil)
	if op.status != http.StatusOK || op.data["requestedState"] != "APPLIED" || op.data["sourceVersion"] != float64(1) {
		t.Fatalf("operation read-back = %+v", op)
	}
	if got := f.call(t, fleetCaller, http.MethodGet, "/operations/never-sent", nil); got.status != http.StatusNotFound {
		t.Fatalf("unknown operation = %+v", got)
	}
	read := f.call(t, fleetCaller, http.MethodGet, "/"+org+"/PROVIDER/"+driver, nil)
	if read.status != http.StatusOK || read.data["state"] != "APPLIED" || read.data["reasonCode"] != "SAFETY_BLOCK" {
		t.Fatalf("subject read = %+v", read)
	}
	if restricted, err := f.svc.Restricted(context.Background(), org, "PROVIDER", driver); err != nil || !restricted {
		t.Fatalf("restricted = %v %v", restricted, err)
	}
	// A restriction binds only the organization that applied it.
	if restricted, err := f.svc.Restricted(context.Background(), "org_other", "PROVIDER", driver); err != nil || restricted {
		t.Fatalf("restricted for another organization = %v %v", restricted, err)
	}

	clear := command(source+":clear", org, "PROVIDER", driver, "SAFETY_BLOCK", source, 2)
	cleared := f.call(t, fleetCaller, http.MethodPost, "/clear", clear)
	if cleared.status != http.StatusOK || cleared.data["requestedState"] != "CLEARED" || cleared.data["outcome"] != "CONFIRMED" {
		t.Fatalf("clear = %+v", cleared)
	}
	if got := f.call(t, fleetCaller, http.MethodPost, "/clear", clear); got.status != http.StatusOK || got.data["restrictionVersion"] != cleared.data["restrictionVersion"] {
		t.Fatalf("clear replay = %+v", got)
	}
	if read = f.call(t, fleetCaller, http.MethodGet, "/"+org+"/PROVIDER/"+driver, nil); read.data["state"] != "CLEARED" {
		t.Fatalf("subject after clear = %+v", read)
	}
	if restricted, err := f.svc.Restricted(context.Background(), org, "PROVIDER", driver); err != nil || restricted {
		t.Fatalf("restricted after clear = %v %v", restricted, err)
	}
	// An apply that Fleet has already superseded changes nothing.
	stale := f.call(t, fleetCaller, http.MethodPost, "/apply", command(source+":stale", org, "PROVIDER", driver, "SAFETY_BLOCK", source, 1))
	if stale.status != http.StatusOK || stale.data["outcome"] != "SUPERSEDED" {
		t.Fatalf("stale apply = %+v", stale)
	}
	if restricted, _ := f.svc.Restricted(context.Background(), org, "PROVIDER", driver); restricted {
		t.Fatal("a superseded apply re-restricted the driver")
	}
	// Clearing a subject Taxi never restricted has nothing to confirm.
	if got := f.call(t, fleetCaller, http.MethodPost, "/clear", command("op-"+uuid.NewString(), org, "ASSET", "never", "X", "s", 1)); got.status != http.StatusNotFound {
		t.Fatalf("clear unknown = %+v", got)
	}
	if got := f.call(t, fleetCaller, http.MethodPost, "/apply", map[string]any{"operationReference": "x"}); got.status != http.StatusBadRequest {
		t.Fatalf("invalid command = %+v", got)
	}
	// History and the operation ledger are append-only.
	if _, err := f.pool.Exec(context.Background(), `UPDATE taxi_authority_restriction_history SET reason_code='tampered' WHERE organization_id=$1`, org); err == nil {
		t.Fatal("history update must be rejected")
	}
	if _, err := f.pool.Exec(context.Background(), `DELETE FROM taxi_authority_restriction_operations WHERE operation_reference=$1`, source+":apply"); err == nil {
		t.Fatal("operation delete must be rejected")
	}
}

func TestTaxiRestrictionAuthorityRequiresFleetBoundary(t *testing.T) {
	f := newFixture(t)
	body := command("op-"+uuid.NewString(), "org_"+uuid.NewString(), "PROVIDER", uuid.NewString(), "X", "s", 1)
	if got := f.call(t, "vendor-service", http.MethodPost, "/apply", body); got.status != http.StatusForbidden {
		t.Fatalf("non-Fleet caller = %+v", got)
	}
	req, _ := http.NewRequest(http.MethodPost, f.server.URL+base+"/apply", bytes.NewReader([]byte(`{}`)))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unsigned call = %d", res.StatusCode)
	}
	// With no Fleet boundary the surface is not mounted at all.
	engine := gin.New()
	restrictions.Register(engine.Group(base), f.svc)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, base+"/apply", bytes.NewReader([]byte(`{}`))))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unbounded surface status = %d", rec.Code)
	}
}

// A restricted driver is not eligible for that organization's managed or
// marketplace work, nor for its private execution; clearing restores them.
func TestRestrictedProviderExcludedFromCandidateSelectionUntilCleared(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	org := "org_" + uuid.NewString()
	driver := uuid.NewString()
	vehicle := uuid.NewString()
	source := "rst_" + uuid.NewString()

	managed := managedexec.NewService(managedexec.Options{Repo: managedexec.NewRepository(f.pool), Restrictions: f.svc})
	private := privateexecution.NewService(privateexecution.NewRepository(f.pool), nil, nil, true, false).WithRestrictions(f.svc)
	privateCmd := privateexecution.CreateRequest{
		OrganizationID: org, SettlementOrganizationID: org, ExecutingProviderID: driver,
		OrganizationMembershipID: "mbr_" + uuid.NewString(), PrivateOrderID: "po_" + uuid.NewString(),
		ShipmentID: "sh_" + uuid.NewString(), PrivateAssignmentID: "pa_" + uuid.NewString(),
		ProviderResponseID: "pr_" + uuid.NewString(), FleetOperationID: "fo_" + uuid.NewString(),
		ServiceCategory: privateexecution.ServiceCategoryTaxi, VehicleID: vehicle,
		Route: privateexecution.RouteSnapshot{PickupAddress: "Ikeja", DropoffAddress: "Lekki",
			PickupLatitude: 6.6, PickupLongitude: 3.35, DropoffLatitude: 6.45, DropoffLongitude: 3.5},
		IdempotencyKey: "key-" + uuid.NewString(),
	}

	eligible := func(organization string) managedexec.EligibilityAnswer {
		t.Helper()
		answer, err := managed.EligibilityFor(ctx, organization, uuid.NewString(), driver, "")
		if err != nil {
			t.Fatal(err)
		}
		return answer
	}
	if answer := eligible(org); !answer.Eligible {
		t.Fatalf("unrestricted driver = %+v", answer)
	}

	if _, err := f.svc.Apply(ctx, restrictions.Command{OperationReference: source + ":apply", OrganizationID: org,
		SubjectType: "PROVIDER", SubjectReference: driver, ReasonCode: "SAFETY_BLOCK", SourceReference: source, SourceVersion: 1}); err != nil {
		t.Fatal(err)
	}
	if answer := eligible(org); answer.Eligible || answer.ReasonCode != managedexec.ReasonComplianceRestricted {
		t.Fatalf("restricted driver = %+v", answer)
	}
	// Another organization's work is not affected by this organization's
	// restriction; an unnamed organization treats any restriction as binding.
	if answer := eligible("org_other_" + uuid.NewString()); !answer.Eligible {
		t.Fatalf("driver for another organization = %+v", answer)
	}
	if answer := eligible(""); answer.Eligible {
		t.Fatalf("driver with no organization named = %+v", answer)
	}
	var err error
	if _, err = private.Create(ctx, privateCmd); !errors.Is(err, privateexecution.ErrIneligible) {
		t.Fatalf("private execution for restricted driver = %v", err)
	}

	// A restricted vehicle is excluded the same way.
	vsource := "rst_" + uuid.NewString()
	if _, err = f.svc.Apply(ctx, restrictions.Command{OperationReference: vsource + ":apply", OrganizationID: org,
		SubjectType: "ASSET", SubjectReference: vehicle, ReasonCode: "INSPECTION_FAILED", SourceReference: vsource, SourceVersion: 1}); err != nil {
		t.Fatal(err)
	}
	if answer, err := managed.EligibilityFor(ctx, org, uuid.NewString(), uuid.NewString(), vehicle); err != nil || answer.Eligible {
		t.Fatalf("restricted vehicle = %+v %v", answer, err)
	}

	// Clearing restores eligibility.
	for _, c := range []restrictions.Command{
		{OperationReference: source + ":clear", OrganizationID: org, SubjectType: "PROVIDER", SubjectReference: driver, ReasonCode: "SAFETY_BLOCK", SourceReference: source, SourceVersion: 2},
		{OperationReference: vsource + ":clear", OrganizationID: org, SubjectType: "ASSET", SubjectReference: vehicle, ReasonCode: "INSPECTION_FAILED", SourceReference: vsource, SourceVersion: 2},
	} {
		if _, err = f.svc.Clear(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	if answer := eligible(org); !answer.Eligible {
		t.Fatalf("driver after clear = %+v", answer)
	}
	// Private execution now passes the restriction gate and stops only at the
	// missing acceptance verifier this test does not wire.
	if _, err = private.Create(ctx, privateCmd); !errors.Is(err, privateexecution.ErrDisabled) {
		t.Fatalf("private execution after clear = %v", err)
	}
}

func TestTaxiRestrictionMigrationRollbackReplay(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `SELECT pg_advisory_lock(hashtext('taxi-restriction-migration-test'))`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `SELECT pg_advisory_unlock(hashtext('taxi-restriction-migration-test'))`)
	}()
	read := func(name string) string {
		raw, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	exists := func() bool {
		var ok bool
		if err := pool.QueryRow(ctx, `SELECT to_regclass('public.taxi_authority_restrictions') IS NOT NULL`).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		return ok
	}
	if _, err := pool.Exec(ctx, read("031_authority_restrictions.down.sql")); err != nil {
		t.Fatal(err)
	}
	if exists() {
		t.Fatal("restriction table survived rollback")
	}
	if _, err := pool.Exec(ctx, read("031_authority_restrictions.up.sql")); err != nil {
		t.Fatal(err)
	}
	if !exists() {
		t.Fatal("restriction table missing after replay")
	}
	// The up file is re-runnable.
	if _, err := pool.Exec(ctx, read("031_authority_restrictions.up.sql")); err != nil {
		t.Fatal(err)
	}
}
