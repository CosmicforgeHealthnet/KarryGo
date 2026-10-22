package admininterventions

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/taxi-service/internal/features/adminresources"
	marketrepositories "cosmicforge/logistics/services/taxi-service/internal/features/markets/repositories"
	providerauthrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_auth/repositories"
	availabilityrepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/repositories"
	availabilityusecases "cosmicforge/logistics/services/taxi-service/internal/features/provider_availability/usecases"
	providerprofilerepositories "cosmicforge/logistics/services/taxi-service/internal/features/provider_profile/repositories"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	ownerfixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type env struct {
	t      *testing.T
	db     *pgxpool.Pool
	store  *availabilityrepositories.RedisAvailabilityStore
	exec   *Executor
	svc    *admincommand.Service
	avail  *availabilityusecases.AvailabilityService
	gate   *Gate
	ctxs   *admincontext.Store
	issuer *contextfixture.Fixture
	scope  adminpolicy.Scope
	other  adminpolicy.Scope
	actor  adminpolicy.Actor
}

func redisClient(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("TAXI_TEST_REDIS_ADDR")
	if addr == "" {
		addr = os.Getenv("TAXI_REDIS_ADDR")
	}
	if addr == "" {
		t.Skip("TAXI_TEST_REDIS_ADDR (or TAXI_REDIS_ADDR) required for the real availability path")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := ownerfixture.Database(t, "../../../migrations")
	rdb := redisClient(t)
	ctx := context.Background()
	spec, _ := admincommand.ForOwner(adminresources.Owner)
	scope := adminpolicy.Scope{Product: "karryam", Market: "NG-LAG", Zone: "lekki", Organization: "org-1", Tenant: "tenant-1"}
	other := adminpolicy.Scope{Product: "karryam", Market: "NG-LAG", Zone: "lekki", Organization: "org-2", Tenant: "tenant-2"}
	p := policyfixture.New(t, scope, spec.Owner, spec.Capability, spec.Contract)
	p.Document.Rules[0].Scopes = append(p.Document.Rules[0].Scopes, other)
	read := p.Document.Rules[0]
	read.Capability, read.Permission = spec.ReadCapability, spec.ReadCapability
	p.Document.Rules = append(p.Document.Rules, read)
	p.Sign(t)
	issuer := contextfixture.New(t, spec.Owner)
	store := &admincontext.Store{DB: db, Authority: issuer.Authority, Reader: adminresources.Source()}
	avStore := availabilityrepositories.NewRedisAvailabilityStore(rdb)
	exec := &Executor{Availability: avStore}
	gate := NewGate(db)
	avail := availabilityusecases.NewAvailabilityService(avStore, availabilityrepositories.NewPostgresOperationalStateRepository(db),
		providerauthrepositories.NewPostgresProviderRepository(db), providerprofilerepositories.NewPostgresVehicleRepository(db),
		marketrepositories.NewPostgresMarketRepository(db), time.Minute).WithAdminGate(gate)
	if _, err := db.Exec(ctx, `INSERT INTO markets(id,country,name,code) VALUES($1,'NG','Test',$2)`, uuid.NewString(), scope.Market); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO operating_zones(id,market_id,name,code,center_lat,center_lng,match_radius_km,supported_classes) SELECT $1,id,'Zone',$2,1.0,-10.0,15,'{economy,comfort,xl}' FROM markets`, uuid.NewString(), scope.Zone); err != nil {
		t.Fatal(err)
	}
	actor := adminpolicy.Actor{Product: "karryam", Staff: "staff-a", Roles: []string{"staff"}, Permissions: []string{spec.Capability, spec.ReadCapability}, Markets: []string{scope.Market}, Zones: []string{scope.Zone}, Organizations: []string{scope.Organization}, Tenants: []string{scope.Tenant}}
	svc := &admincommand.Service{Spec: spec, DB: db, Policy: p.Engine, Contexts: store, Executor: exec, WriteEnabled: true, ReadEnabled: true}
	return &env{t: t, db: db, store: avStore, exec: exec, svc: svc, avail: avail, gate: gate, ctxs: store, issuer: issuer, scope: scope, other: other, actor: actor}
}

func (e *env) enroll(kind, id string) {
	e.t.Helper()
	ctx := context.Background()
	tx, err := e.db.Begin(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	binding, err := adminresources.Source().Binding(ctx, tx, kind, id)
	tx.Rollback(ctx)
	if err != nil {
		e.t.Fatal(err)
	}
	approval := e.issuer.Approval("ctx-"+kind+"-"+id, kind, id, e.svc.Spec.Capability, binding, e.scope)
	approval.Capabilities = []string{e.svc.Spec.Capability, e.svc.Spec.ReadCapability}
	if err := e.ctxs.Import(ctx, e.issuer.Sign(e.t, approval), time.Now()); err != nil {
		e.t.Fatal(err)
	}
}

// seedProvider inserts a fully compliant provider with two active vehicles,
// enrolls all three as signed owner contexts and returns their ids.
func (e *env) seedProvider() (string, string, string) {
	e.t.Helper()
	ctx := context.Background()
	provider, v1, v2 := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := e.db.Exec(ctx, `INSERT INTO taxi_providers(id,phone,account_status,onboarding_status,identity_verification_status,vehicle_verification_status,document_compliance_status,rating) VALUES($1,$2,'active','complete','verified','verified','compliant',4.8)`, provider, "+234"+provider[:10]); err != nil {
		e.t.Fatal(err)
	}
	for i, v := range []string{v1, v2} {
		if _, err := e.db.Exec(ctx, `INSERT INTO vehicles(id,provider_id,vehicle_class,plate_number,seats,status,created_at) VALUES($1,$2,'economy',$3,4,'active',now()+make_interval(secs=>$4))`, v, provider, "PL-"+v[:8], i); err != nil {
			e.t.Fatal(err)
		}
	}
	e.t.Cleanup(func() { _ = e.store.SetOffline(context.Background(), provider) })
	e.enroll(KindProviders, provider)
	e.enroll(KindVehicles, v1)
	e.enroll(KindVehicles, v2)
	return provider, v1, v2
}

func (e *env) online(provider, vehicle string) error {
	_, err := e.avail.SetAvailability(context.Background(), availabilityusecases.SetAvailabilityInput{ProviderID: provider, VehicleID: vehicle, Status: "online", Lat: 1.0005, Lng: -9.9995})
	return err
}

func (e *env) opState(provider string) string {
	var state string
	if err := e.db.QueryRow(context.Background(), `SELECT COALESCE((SELECT state FROM provider_operational_state WHERE provider_id=$1::uuid),'OFFLINE')`, provider).Scan(&state); err != nil {
		e.t.Fatal(err)
	}
	return state
}

func (e *env) live(provider string) bool {
	_, ok, err := e.store.GetStatus(context.Background(), provider)
	if err != nil {
		e.t.Fatal(err)
	}
	return ok
}

func (e *env) cmd(kind, id, caseRef, action string, expected int64, params map[string]string) admincommand.Command {
	if params == nil {
		params = map[string]string{}
	}
	return admincommand.Command{ContractVersion: e.svc.Spec.Contract, OperationID: uuid.NewString(), SubjectKind: kind, SubjectID: id, CaseRef: caseRef, Action: action, ExpectedVersion: expected, IdempotencyKey: uuid.NewString(), ReasonCode: "SAFETY_REVIEW", Params: params, RequestedAt: time.Now().UTC().Add(-time.Second)}
}

func (e *env) approve(c *admincommand.Command, approver string) {
	c.Approval = &admincommand.Approval{ApprovalID: uuid.NewString(), ApproverRef: approver, IntentDigest: c.IntentDigest(e.actor.Staff, "case-1", e.scope), ApprovedAt: time.Now().UTC().Add(-time.Second)}
}

func (e *env) run(c admincommand.Command) (admincommand.Receipt, error) {
	return e.svc.Execute(context.Background(), e.actor, "case-1", e.scope, c)
}

func (e *env) must(c admincommand.Command) admincommand.Receipt {
	e.t.Helper()
	r, err := e.run(c)
	if err != nil {
		e.t.Fatalf("%s: %v", c.Action, err)
	}
	return r
}

func gateReason(err error) string {
	var app *apperrors.Error
	if errors.As(err, &app) && app.Details != nil {
		if r, ok := app.Details["reason"].(string); ok {
			return r
		}
	}
	return ""
}

func TestLedgerMigrationEmbedsSharedTemplateVerbatim(t *testing.T) {
	raw, err := os.ReadFile("../../../migrations/026_admin_command_ledger.sql")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, admincommand.Schema) || !strings.Contains(body, "-- shared-template-sha256: "+admincommand.SchemaSHA256()+"\n") {
		t.Fatal("ledger migration drifted from shared/go/admincommand/schema.sql")
	}
}

func TestCanonicalExpiryIsEndOfDayInLagos(t *testing.T) {
	at, err := ExpiryInstant("2026-10-22")
	if err != nil {
		t.Fatal(err)
	}
	// Africa/Lagos is UTC+1: the end of 22 Oct in Lagos is 23:00Z on 22 Oct.
	if want := time.Date(2026, 10, 22, 23, 0, 0, 0, time.UTC); !at.Equal(want) {
		t.Fatalf("canonical expiry %s, want %s", at.UTC(), want)
	}
	if _, err := ExpiryInstant("2026-02-30"); err == nil {
		t.Fatal("impossible date accepted")
	}
}

func TestIncidentLifecycleAndIllegalTransitions(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	provider, vehicle, _ := e.seedProvider()
	today := time.Now().In(lagos).Format("2006-01-02")
	tomorrow := time.Now().In(lagos).AddDate(0, 0, 1).Format("2006-01-02")
	open := func(kind, id, ref string) admincommand.Command {
		return e.cmd(kind, id, ref, "OPEN_INCIDENT", 0, map[string]string{"category": "SAFETY", "severity": "HIGH", "occurred_on": today})
	}
	caseA := "case-" + uuid.NewString()
	r := e.must(open(KindProviders, provider, caseA))
	if r.PriorState != "NONE" || r.ResultingState != IncidentOpen || r.RecordedVersion != 1 {
		t.Fatal("open", r)
	}
	// The same case cannot be opened again, nor reused for another subject.
	if _, err := e.run(open(KindProviders, provider, caseA)); !errors.Is(err, admincommand.ErrConflict) {
		t.Fatal("case reopened", err)
	}
	if _, err := e.run(e.cmd(KindVehicles, vehicle, caseA, "CLOSE_INCIDENT", 1, nil)); !errors.Is(err, admincommand.ErrNotFound) {
		t.Fatal("case bound to another subject disclosed/changed", err)
	}
	// Illegal: REMEDIATED without remediation, CLOSE before RESOLVE, a future occurrence, a past due date.
	if _, err := e.run(e.cmd(KindProviders, provider, caseA, "RESOLVE_INCIDENT", 1, map[string]string{"resolution": "REMEDIATED"})); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("remediated without remediation", err)
	}
	if _, err := e.run(e.cmd(KindProviders, provider, caseA, "CLOSE_INCIDENT", 1, nil)); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("closed while open", err)
	}
	future := open(KindProviders, provider, "case-"+uuid.NewString())
	future.Params["occurred_on"] = tomorrow
	if _, err := e.run(future); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("future incident accepted", err)
	}
	if _, err := e.run(e.cmd(KindProviders, provider, caseA, "START_REMEDIATION", 1, map[string]string{"remediation": "RETRAINING", "due_on": "2001-01-01"})); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("overdue remediation accepted", err)
	}
	// Stale version.
	if _, err := e.run(e.cmd(KindProviders, provider, caseA, "START_REMEDIATION", 0, map[string]string{"remediation": "RETRAINING", "due_on": tomorrow})); !errors.Is(err, admincommand.ErrConflict) {
		t.Fatal("stale incident version", err)
	}
	e.must(e.cmd(KindProviders, provider, caseA, "START_REMEDIATION", 1, map[string]string{"remediation": "RETRAINING", "due_on": tomorrow}))
	r = e.must(e.cmd(KindProviders, provider, caseA, "RESOLVE_INCIDENT", 2, map[string]string{"resolution": "REMEDIATED"}))
	if r.PriorState != IncidentRemediation || r.ResultingState != IncidentResolved {
		t.Fatal("resolve", r)
	}
	if _, err := e.run(e.cmd(KindProviders, provider, caseA, "START_REMEDIATION", 3, map[string]string{"remediation": "RETRAINING", "due_on": tomorrow})); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("remediation after resolution", err)
	}
	r = e.must(e.cmd(KindProviders, provider, caseA, "CLOSE_INCIDENT", 3, nil))
	if r.ResultingState != IncidentClosed || r.RecordedVersion != 4 {
		t.Fatal("close", r)
	}
	if _, err := e.run(e.cmd(KindProviders, provider, caseA, "RESOLVE_INCIDENT", 4, map[string]string{"resolution": "NO_FINDING"})); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("closed incident reopened", err)
	}
	// NO_FINDING may resolve directly from OPEN, on an independent ride too.
	ride := uuid.NewString()
	if _, err := e.db.Exec(ctx, `INSERT INTO taxi_bookings(id,customer_id,market_id,operating_zone_id,status) SELECT $1,$2,m.id,z.id,'completed' FROM markets m JOIN operating_zones z ON z.market_id=m.id`, ride, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	e.enroll(KindIndependentRides, ride)
	caseB := "case-" + uuid.NewString()
	e.must(open(KindIndependentRides, ride, caseB))
	if v, s := currentOf(t, e, KindIndependentRides, ride); v != 0 || s != StandingIncidentOpen {
		t.Fatal("ride standing", v, s)
	}
	e.must(e.cmd(KindIndependentRides, ride, caseB, "RESOLVE_INCIDENT", 1, map[string]string{"resolution": "NO_FINDING"}))
	if _, s := currentOf(t, e, KindIndependentRides, ride); s != StandingActive {
		t.Fatal("resolved ride standing", s)
	}
	var rideStatus string
	e.db.QueryRow(ctx, `SELECT status FROM taxi_bookings WHERE id=$1`, ride).Scan(&rideStatus)
	if rideStatus != "completed" {
		t.Fatal("incident touched trip status", rideStatus)
	}
	var transitions, remediations int
	e.db.QueryRow(ctx, `SELECT count(*) FROM taxi_admin_incident_transitions WHERE case_ref=$1`, caseA).Scan(&transitions)
	e.db.QueryRow(ctx, `SELECT count(*) FROM taxi_admin_incident_remediations WHERE case_ref=$1`, caseA).Scan(&remediations)
	if transitions != 4 || remediations != 1 {
		t.Fatal("history", transitions, remediations)
	}
	for _, sql := range []string{`UPDATE taxi_admin_incident_transitions SET to_state='OPEN'`, `DELETE FROM taxi_admin_incident_remediations`, `UPDATE taxi_admin_incidents SET state='OPEN',version=version+1 WHERE case_ref='` + caseA + `'`, `DELETE FROM taxi_admin_incidents`} {
		if _, err := e.db.Exec(ctx, sql); err == nil {
			t.Fatal("incident history mutable:", sql)
		}
	}
	// Incidents never change the subject's online standing.
	if err := e.online(provider, vehicle); err != nil {
		t.Fatal("incident blocked online", err)
	}
}

func TestDocumentDecisionCanonicalExpiryAndGating(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	provider, vehicle, _ := e.seedProvider()
	lagosNow := time.Now().In(lagos)
	yesterday := lagosNow.AddDate(0, 0, -1).Format("2006-01-02")
	today := lagosNow.Format("2006-01-02")
	nextYear := lagosNow.AddDate(1, 0, 0).Format("2006-01-02")
	decide := func(kind, id string, v int64, document, decision, expires string) admincommand.Command {
		p := map[string]string{"document": document, "decision": decision}
		if expires != "" {
			p["expires_on"] = expires
		}
		return e.cmd(kind, id, "", "RECORD_DOCUMENT_DECISION", v, p)
	}
	for name, c := range map[string]admincommand.Command{
		"verified in the past":    decide(KindProviders, provider, 0, "DRIVER_LICENSE", "VERIFIED", yesterday),
		"verified without expiry": decide(KindProviders, provider, 0, "DRIVER_LICENSE", "VERIFIED", ""),
		"licence on a vehicle":    decide(KindVehicles, vehicle, 0, "DRIVER_LICENSE", "VERIFIED", nextYear),
		"insurance on a provider": decide(KindProviders, provider, 0, "INSURANCE", "VERIFIED", nextYear),
		"expired in the future":   decide(KindProviders, provider, 0, "DRIVER_LICENSE", "EXPIRED", nextYear),
	} {
		if _, err := e.run(c); !errors.Is(err, admincommand.ErrRejected) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	// VERIFIED through the end of today in Lagos.
	r := e.must(decide(KindProviders, provider, 0, "DRIVER_LICENSE", "VERIFIED", today))
	if r.ResultingState != StandingActive || r.Outcome != "DOCUMENT_VERIFIED" {
		t.Fatal(r)
	}
	var stored time.Time
	e.db.QueryRow(ctx, `SELECT expires_at FROM taxi_admin_document_decisions WHERE subject_id=$1`, provider).Scan(&stored)
	want, _ := ExpiryInstant(today)
	if !stored.Equal(want) {
		t.Fatal("stored expiry is not the canonical Lagos end of day", stored, want)
	}
	var before, after string
	e.db.QueryRow(ctx, `SELECT COALESCE(taxi_admin_online_block($1,NULL,$2),''),COALESCE(taxi_admin_online_block($1,NULL,$3),'')`, provider, want.Add(-time.Microsecond), want).Scan(&before, &after)
	if before != "" || after != ReasonProviderDocumentInvalid {
		t.Fatal("expiry boundary", before, after)
	}
	if err := e.online(provider, vehicle); err != nil {
		t.Fatal("verified provider refused", err)
	}
	// EXPIRED on the licence forces the idle online provider offline and blocks going online.
	r = e.must(decide(KindProviders, provider, 1, "DRIVER_LICENSE", "EXPIRED", ""))
	if r.ResultingState != StandingDocumentInvalid {
		t.Fatal(r)
	}
	if e.opState(provider) != "OFFLINE" || e.live(provider) {
		t.Fatal("blocked provider left in availability")
	}
	if err := e.online(provider, vehicle); gateReason(err) != ReasonProviderDocumentInvalid {
		t.Fatal("expired licence went online", err)
	}
	e.must(decide(KindProviders, provider, 2, "DRIVER_LICENSE", "VERIFIED", nextYear))
	if err := e.online(provider, vehicle); err != nil {
		t.Fatal("re-verified provider refused", err)
	}
	// REJECTED on the vehicle's insurance blocks that vehicle only.
	e.must(decide(KindVehicles, vehicle, 0, "INSURANCE", "REJECTED", ""))
	if e.live(provider) {
		t.Fatal("provider online on rejected vehicle left in availability")
	}
	if err := e.online(provider, vehicle); gateReason(err) != ReasonVehicleDocumentInvalid {
		t.Fatal("rejected vehicle went online", err)
	}
	if _, err := e.db.Exec(ctx, `DELETE FROM taxi_admin_document_decisions`); err == nil {
		t.Fatal("decision history deletable")
	}
}

func TestRestrictProviderSuspendOnlineRequiresApprovalAndReallyBlocks(t *testing.T) {
	e := newEnv(t)
	provider, vehicle, _ := e.seedProvider()
	if err := e.online(provider, vehicle); err != nil {
		t.Fatal(err)
	}
	if e.opState(provider) != "AVAILABLE" || !e.live(provider) {
		t.Fatal("seed not online")
	}
	restrict := e.cmd(KindProviders, provider, "", "RESTRICT_PROVIDER", 0, map[string]string{"restriction": "SUSPEND_ONLINE"})
	if _, err := e.run(restrict); !errors.Is(err, admincommand.ErrInvalid) {
		t.Fatal("restriction without approval", err)
	}
	e.approve(&restrict, e.actor.Staff)
	if _, err := e.run(restrict); !errors.Is(err, adminpolicy.ErrDenied) {
		t.Fatal("self approval", err)
	}
	if e.opState(provider) != "AVAILABLE" {
		t.Fatal("refused command leaked an effect")
	}
	e.approve(&restrict, "staff-b")
	r := e.must(restrict)
	if r.PriorState != StandingActive || r.ResultingState != StandingOnlineSuspended || r.ApprovalID == "" {
		t.Fatal(r)
	}
	// Idempotent replay returns the original receipt, nothing re-executed.
	if again, err := e.run(restrict); err != nil || again != r {
		t.Fatal("replay", err)
	}
	if e.opState(provider) != "OFFLINE" || e.live(provider) {
		t.Fatal("suspended provider still available")
	}
	if err := e.online(provider, vehicle); gateReason(err) != ReasonProviderOnlineSuspended {
		t.Fatal("suspended provider went online", err)
	}
	// Restricting twice is an illegal transition with no side effects.
	twice := e.cmd(KindProviders, provider, "", "RESTRICT_PROVIDER", 1, map[string]string{"restriction": "NO_NEW_ASSIGNMENTS"})
	e.approve(&twice, "staff-b")
	if _, err := e.run(twice); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("double restriction", err)
	}
	stale := e.cmd(KindProviders, provider, "", "LIFT_PROVIDER_RESTRICTION", 0, nil)
	e.approve(&stale, "staff-b")
	if _, err := e.run(stale); !errors.Is(err, admincommand.ErrConflict) {
		t.Fatal("stale lift", err)
	}
	lift := e.cmd(KindProviders, provider, "", "LIFT_PROVIDER_RESTRICTION", 1, nil)
	e.approve(&lift, "staff-b")
	if r := e.must(lift); r.ResultingState != StandingActive {
		t.Fatal(r)
	}
	if err := e.online(provider, vehicle); err != nil {
		t.Fatal("lift did not restore eligibility", err)
	}
}

func TestSuspensionConvergesOnHeartbeatWhenLivePoolWasNotReachable(t *testing.T) {
	e := newEnv(t)
	provider, vehicle, _ := e.seedProvider()
	if err := e.online(provider, vehicle); err != nil {
		t.Fatal(err)
	}
	e.svc.Executor = &Executor{} // no live-pool access during the command
	restrict := e.cmd(KindProviders, provider, "", "RESTRICT_PROVIDER", 0, map[string]string{"restriction": "SUSPEND_ONLINE"})
	e.approve(&restrict, "staff-b")
	e.must(restrict)
	if e.opState(provider) != "OFFLINE" || !e.live(provider) {
		t.Fatal("expected durable OFFLINE with a stale live entry")
	}
	_, err := e.avail.Heartbeat(context.Background(), availabilityusecases.SetAvailabilityInput{ProviderID: provider, Lat: 1.0005, Lng: -9.9995}, false)
	if gateReason(err) != ReasonProviderOnlineSuspended || e.live(provider) {
		t.Fatal("heartbeat did not converge the suspended provider", err)
	}
}

func TestNoNewAssignmentsKeepsOnlineButBlocksAssignment(t *testing.T) {
	e := newEnv(t)
	provider, vehicle, _ := e.seedProvider()
	if err := e.online(provider, vehicle); err != nil {
		t.Fatal(err)
	}
	c := e.cmd(KindProviders, provider, "", "RESTRICT_PROVIDER", 0, map[string]string{"restriction": "NO_NEW_ASSIGNMENTS"})
	e.approve(&c, "staff-b")
	if r := e.must(c); r.ResultingState != StandingAssignmentsSuspended {
		t.Fatal(r)
	}
	if e.opState(provider) != "AVAILABLE" {
		t.Fatal("NO_NEW_ASSIGNMENTS must not force offline")
	}
	if reason, err := e.gate.AssignmentBlock(context.Background(), provider, vehicle); err != nil || reason != ReasonProviderNoNewAssignments {
		t.Fatal("assignment not blocked", reason, err)
	}
	if reason, _ := e.gate.OnlineBlock(context.Background(), provider, vehicle); reason != "" {
		t.Fatal("online wrongly blocked", reason)
	}
}

func TestVehicleOutOfServiceAndReverification(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	provider, v1, v2 := e.seedProvider()
	if err := e.online(provider, v1); err != nil {
		t.Fatal(err)
	}
	c := e.cmd(KindVehicles, v1, "", "RESTRICT_VEHICLE", 0, map[string]string{"restriction": "OUT_OF_SERVICE"})
	e.approve(&c, "staff-b")
	if r := e.must(c); r.ResultingState != StandingOutOfService {
		t.Fatal(r)
	}
	if e.opState(provider) != "OFFLINE" || e.live(provider) {
		t.Fatal("provider still online on an out-of-service vehicle")
	}
	if err := e.online(provider, v1); gateReason(err) != ReasonVehicleOutOfService {
		t.Fatal("out-of-service vehicle went online", err)
	}
	if err := e.online(provider, v2); err != nil {
		t.Fatal("unaffected vehicle refused", err)
	}
	// Restricting v1 while online on v2 leaves the provider online.
	lift := e.cmd(KindVehicles, v1, "", "LIFT_VEHICLE_RESTRICTION", 1, nil)
	e.approve(&lift, "staff-b")
	e.must(lift)
	again := e.cmd(KindVehicles, v1, "", "RESTRICT_VEHICLE", 2, map[string]string{"restriction": "OUT_OF_SERVICE"})
	e.approve(&again, "staff-b")
	e.must(again)
	if !e.live(provider) || e.opState(provider) != "AVAILABLE" {
		t.Fatal("restricting an unused vehicle kicked the provider offline")
	}
	// Reverification of the licence blocks until a later VERIFIED clears it.
	req := e.must(e.cmd(KindProviders, provider, "", "REQUEST_REVERIFICATION", 0, map[string]string{"document": "DRIVER_LICENSE"}))
	if req.ResultingState != StandingReverificationRequired || e.live(provider) {
		t.Fatal("reverification did not take provider offline", req)
	}
	if _, err := e.run(e.cmd(KindProviders, provider, "", "REQUEST_REVERIFICATION", 1, map[string]string{"document": "DRIVER_LICENSE"})); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("duplicate open request", err)
	}
	if err := e.online(provider, v2); gateReason(err) != ReasonProviderReverificationPending {
		t.Fatal("reverification did not block", err)
	}
	// A REJECTED decision does not clear the request.
	e.must(e.cmd(KindProviders, provider, "", "RECORD_DOCUMENT_DECISION", 1, map[string]string{"document": "DRIVER_LICENSE", "decision": "REJECTED"}))
	if err := e.online(provider, v2); gateReason(err) != ReasonProviderReverificationPending {
		t.Fatal("rejection cleared reverification", err)
	}
	next := time.Now().In(lagos).AddDate(1, 0, 0).Format("2006-01-02")
	if r := e.must(e.cmd(KindProviders, provider, "", "RECORD_DOCUMENT_DECISION", 2, map[string]string{"document": "DRIVER_LICENSE", "decision": "VERIFIED", "expires_on": next, "evidence_ref": "media-1"})); r.ResultingState != StandingActive {
		t.Fatal(r)
	}
	if err := e.online(provider, v2); err != nil {
		t.Fatal("verified licence did not clear reverification", err)
	}
	// Vehicle-level reverification blocks only that vehicle.
	e.must(e.cmd(KindVehicles, v2, "", "REQUEST_REVERIFICATION", 0, map[string]string{"document": "ROADWORTHINESS"}))
	if err := e.online(provider, v2); gateReason(err) != ReasonVehicleReverificationPending {
		t.Fatal("vehicle reverification did not block", err)
	}
	var free string
	e.db.QueryRow(ctx, `SELECT COALESCE(taxi_admin_online_block($1,NULL,now()),'')`, provider).Scan(&free)
	if free != "" {
		t.Fatal("vehicle request leaked onto provider", free)
	}
	// Current reflects standing.
	tx, _ := e.db.Begin(ctx)
	defer tx.Rollback(ctx)
	if v, s, err := e.exec.Current(ctx, tx, KindProviders, provider); err != nil || v != 3 || s != StandingActive {
		t.Fatal("current", v, s, err)
	}
	if v, s, _ := e.exec.Current(ctx, tx, KindProviders, uuid.NewString()); v != 0 || s != StandingActive {
		t.Fatal("absent standing", v, s)
	}
}

func TestConcurrentConflictingCommandsCommitExactlyOnce(t *testing.T) {
	e := newEnv(t)
	provider, _, _ := e.seedProvider()
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := e.cmd(KindProviders, provider, "", "RESTRICT_PROVIDER", 0, map[string]string{"restriction": "SUSPEND_ONLINE"})
			e.approve(&c, "staff-b")
			_, err := e.run(c)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	ok := 0
	for err := range results {
		if err == nil {
			ok++
		} else if !errors.Is(err, admincommand.ErrConflict) && !errors.Is(err, admincommand.ErrRejected) {
			t.Fatal(err)
		}
	}
	var history int
	e.db.QueryRow(context.Background(), `SELECT count(*) FROM taxi_admin_restriction_history WHERE subject_id=$1`, provider).Scan(&history)
	if ok != 1 || history != 1 {
		t.Fatal("exactly one transition expected", ok, history)
	}
	// Concurrent incident opens on one case_ref across subjects: one wins.
	_, v1, v2 := e.seedProvider()
	ref := "case-" + uuid.NewString()
	errs := make(chan error, 2)
	for _, v := range []string{v1, v2} {
		go func(v string) {
			_, err := e.run(e.cmd(KindVehicles, v, ref, "OPEN_INCIDENT", 0, map[string]string{"category": "VEHICLE_CONDITION", "severity": "LOW", "occurred_on": time.Now().In(lagos).Format("2006-01-02")}))
			errs <- err
		}(v)
	}
	a, b := <-errs, <-errs
	if (a == nil) == (b == nil) {
		t.Fatal("one case opened for two subjects", a, b)
	}
}

func TestCrossTenantSubjectDenied(t *testing.T) {
	e := newEnv(t)
	provider, _, _ := e.seedProvider()
	actor := e.actor
	actor.Organizations, actor.Tenants = []string{e.other.Organization}, []string{e.other.Tenant}
	c := e.cmd(KindProviders, provider, "", "REQUEST_REVERIFICATION", 0, map[string]string{"document": "DRIVER_LICENSE"})
	if _, err := e.svc.Execute(context.Background(), actor, "case-1", e.other, c); err == nil {
		t.Fatal("foreign-tenant command executed")
	}
	if _, err := e.svc.Readback(context.Background(), actor, "case-1", e.other, KindProviders, provider, c.IdempotencyKey); err == nil {
		t.Fatal("foreign-tenant readback")
	}
	if reason, _ := e.gate.OnlineBlock(context.Background(), provider, ""); reason != "" {
		t.Fatal("denied command leaked", reason)
	}
}

func TestSignedHTTPBoundaryReadbackAndHistory(t *testing.T) {
	e := newEnv(t)
	provider, vehicle, _ := e.seedProvider()
	gin.SetMode(gin.TestMode)
	write, read := []byte(strings.Repeat("w", 32)), []byte(strings.Repeat("r", 32))
	h := &admincommand.Handler{Service: e.svc, WriteSecret: write, ReadSecret: read}
	router := gin.New()
	h.Register(router)
	server := httptest.NewServer(router)
	defer server.Close()
	client, err := admincommand.NewClient(adminresources.Owner, server.URL, write, read)
	if err != nil {
		t.Fatal(err)
	}
	q := adminread.Query{Scope: adminread.Scope{Market: e.scope.Market, Zone: e.scope.Zone, Organization: e.scope.Organization, Tenant: e.scope.Tenant}, Staff: e.actor.Staff, Purpose: "case-1", Start: time.Now().Add(-time.Hour), End: time.Now(), Limit: 1, Role: "staff", PolicyID: e.svc.Policy.ID(), PolicyVersion: e.svc.Policy.Version(), PolicyChecksum: e.svc.Policy.Checksum()}
	c := e.cmd(KindVehicles, vehicle, "", "RESTRICT_VEHICLE", 0, map[string]string{"restriction": "OUT_OF_SERVICE"})
	e.approve(&c, "staff-b")
	receipt, err := client.Execute(context.Background(), q, c)
	if err != nil || receipt.ResultingState != StandingOutOfService {
		t.Fatal("signed command", err)
	}
	if err := e.online(provider, vehicle); gateReason(err) != ReasonVehicleOutOfService {
		t.Fatal("signed restriction had no effect", err)
	}
	back, err := client.Readback(context.Background(), q, KindVehicles, vehicle, c.IdempotencyKey)
	if err != nil || back != receipt {
		t.Fatal("readback", err)
	}
	if _, err := client.Readback(context.Background(), q, KindVehicles, vehicle, uuid.NewString()); !errors.Is(err, admincommand.ErrAbsent) {
		t.Fatal("absent readback", err)
	}
	history, err := client.History(context.Background(), q, KindVehicles, vehicle)
	if err != nil || history.CurrentVersion != 1 || history.CurrentState != StandingOutOfService || len(history.Decisions) != 1 {
		t.Fatal("history", err, history)
	}
	stale := e.cmd(KindVehicles, vehicle, "", "LIFT_VEHICLE_RESTRICTION", 0, nil)
	e.approve(&stale, "staff-b")
	if _, err := client.Execute(context.Background(), q, stale); !errors.Is(err, admincommand.ErrRefused) {
		t.Fatal("stale over the wire", err)
	}
	wrong, _ := admincommand.NewClient(adminresources.Owner, server.URL, []byte(strings.Repeat("x", 32)), read)
	lift := e.cmd(KindVehicles, vehicle, "", "LIFT_VEHICLE_RESTRICTION", 1, nil)
	e.approve(&lift, "staff-b")
	if _, err := wrong.Execute(context.Background(), q, lift); err == nil {
		t.Fatal("wrong write key accepted")
	}
	// A captured signed request cannot be replayed.
	address := server.URL + e.svc.Spec.Path() + "/" + KindVehicles + "/" + vehicle + "?" + q.Values().Encode()
	request, _ := http.NewRequest(http.MethodGet, address, nil)
	if err := serviceauth.SignRequest(request, adminread.Caller, read, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	first, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	first.Body.Close()
	replay, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	replay.Body.Close()
	if first.StatusCode != 200 || replay.StatusCode != 409 {
		t.Fatal("nonce replay", first.StatusCode, replay.StatusCode)
	}
	if v, s := currentOf(t, e, KindVehicles, vehicle); v != 1 || s != StandingOutOfService {
		t.Fatal("refused commands changed state", v, s)
	}
}

func currentOf(t *testing.T, e *env, kind, id string) (int64, string) {
	ctx := context.Background()
	tx, err := e.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	v, s, err := e.exec.Current(ctx, tx, kind, id)
	if err != nil {
		t.Fatal(err)
	}
	return v, s
}
