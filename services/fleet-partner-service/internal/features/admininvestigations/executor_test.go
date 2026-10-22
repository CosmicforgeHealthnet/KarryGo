package admininvestigations_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/admininvestigations"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/adminresources"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminowner"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const purpose = "case-1"

type env struct {
	db      *pgxpool.Pool
	svc     *admincommand.Service
	actor   adminpolicy.Actor
	scope   adminpolicy.Scope
	orgA    string
	orgB    string
	subject string // private assignment of orgA
	foreign string // private assignment of orgB
	issuer  *contextfixture.Fixture
	store   *admincontext.Store
}

func seedAssignment(t *testing.T, db *pgxpool.Pool) (org, assignment string) {
	t.Helper()
	ctx := context.Background()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	org, branch, membership, assignment := "org_"+suffix, "brn_"+suffix, "mem_"+suffix, "asn_"+suffix
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO fleet_partner_organizations(id,legal_name,registration_number,country_code,timezone,default_currency,status,created_at,updated_at)VALUES($1,'Investigation Test Co',$2,'NG','Africa/Lagos','NGN','ACTIVE',now(),now())`, []any{org, "RC-INV-" + suffix}},
		{`INSERT INTO fleet_partner_branches(id,organization_id,code,name,timezone)VALUES($1,$2,$3,'HQ','Africa/Lagos')`, []any{branch, org, "B" + suffix[:8]}},
		{`INSERT INTO fleet_partner_organization_provider_memberships(id,organization_id,provider_service,service_category,provider_id,onboarding_source,status)VALUES($1,$2,'driver-dispatch-delivery-service','DISPATCH',$3,'EXISTING_PROVIDER_LINKED','ACTIVE')`, []any{membership, org, "pro_" + suffix}},
		{`INSERT INTO fleet_partner_private_assignments(id,organization_id,branch_id,work_origin,route_stop_id,provider_membership_id,provider_id,execution_service,status,response_deadline,created_by,updated_at)VALUES($1,$2,$3,'ROUTE_STOP',$4,$5,$6,'DISPATCH','COMPLETED',now()+interval '1 hour','usr_test',now())`, []any{assignment, org, branch, "stop-" + suffix, membership, "pro_" + suffix}},
	} {
		if _, err := db.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	return org, assignment
}

func importContext(t *testing.T, e *env, kind, ref string, scope adminpolicy.Scope, capabilities ...string) {
	t.Helper()
	ctx := context.Background()
	tx, err := e.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := adminresources.Source().Binding(ctx, tx, kind, ref)
	tx.Rollback(ctx)
	if err != nil {
		t.Fatal(err)
	}
	approval := e.issuer.Approval("ctx-"+ref, kind, ref, capabilities[0], binding, scope)
	approval.Capabilities = capabilities
	if err := e.store.Import(ctx, e.issuer.Sign(t, approval), time.Now()); err != nil {
		t.Fatal(err)
	}
}

func setup(t *testing.T) *env {
	t.Helper()
	db := fixture.Database(t, "../../../migrations")
	e := &env{db: db}
	e.orgA, e.subject = seedAssignment(t, db)
	e.orgB, e.foreign = seedAssignment(t, db)
	spec := admininvestigations.Spec()
	e.scope = adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: e.orgA, Tenant: e.orgA}
	p := policyfixture.New(t, e.scope, spec.Owner, spec.Capability, spec.Contract)
	read := p.Document.Rules[0]
	read.Capability, read.Permission = spec.ReadCapability, spec.ReadCapability
	p.Document.Rules = append(p.Document.Rules, read)
	p.Sign(t)
	e.issuer = contextfixture.New(t, spec.Owner)
	e.store = &admincontext.Store{DB: db, Reader: adminresources.Source(), Authority: e.issuer.Authority}
	importContext(t, e, "private-assignments", e.subject, e.scope, spec.Capability, spec.ReadCapability)
	foreignScope := e.scope
	foreignScope.Organization, foreignScope.Tenant = e.orgB, e.orgB
	importContext(t, e, "private-assignments", e.foreign, foreignScope, spec.Capability, spec.ReadCapability)
	e.actor = adminpolicy.Actor{Product: "karryam", Staff: "staff-a", Roles: []string{"staff"}, Permissions: []string{spec.Capability, spec.ReadCapability}, Markets: []string{e.scope.Market}, Zones: []string{e.scope.Zone}, Organizations: []string{e.scope.Organization}, Tenants: []string{e.scope.Tenant}}
	e.svc = &admincommand.Service{Spec: spec, DB: db, Policy: p.Engine, Contexts: e.store, Executor: admininvestigations.Executor{}, WriteEnabled: true, ReadEnabled: true}
	return e
}

func (e *env) command(action, caseRef string, expected int64) admincommand.Command {
	c := admincommand.Command{ContractVersion: e.svc.Spec.Contract, OperationID: uuid.NewString(), SubjectKind: "private-assignments", SubjectID: e.subject, CaseRef: caseRef, Action: action, ExpectedVersion: expected, IdempotencyKey: uuid.NewString(), ReasonCode: "CARGO_REVIEW", Params: map[string]string{}, RequestedAt: time.Now().UTC().Add(-time.Second)}
	switch action {
	case "OPEN_INVESTIGATION":
		c.Params = map[string]string{"category": "CARGO_DAMAGE", "severity": "HIGH", "occurred_on": "2026-10-01"}
	case "RECORD_FINDING":
		c.Params = map[string]string{"finding": "SUBSTANTIATED", "note": "Pallet crushed at loading"}
	case "CLOSE_INVESTIGATION":
		c.Params = map[string]string{"closure": "RESOLVED"}
	}
	return c
}
func (e *env) approve(c *admincommand.Command, approver string) {
	c.Approval = &admincommand.Approval{ApprovalID: uuid.NewString(), ApproverRef: approver, IntentDigest: c.IntentDigest(e.actor.Staff, purpose, e.scope), ApprovedAt: time.Now().UTC().Add(-time.Second)}
}
func (e *env) caseState(t *testing.T, ref string) (state string, version int64, org string) {
	t.Helper()
	if err := e.db.QueryRow(context.Background(), `SELECT state,version,organization_id FROM fleet_partner_admin_investigations WHERE case_ref=$1`, ref).Scan(&state, &version, &org); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal(err)
	}
	return
}

func TestFleetLedgerMigrationEmbedsSharedTemplateVerbatim(t *testing.T) {
	raw, err := os.ReadFile("../../../migrations/000082_admin_command_ledger.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, "-- shared-template-sha256: "+admincommand.SchemaSHA256()+"\n") || !strings.Contains(body, admincommand.Schema) {
		t.Fatal("Fleet command ledger migration drifted from shared/go/admincommand/schema.sql")
	}
	spec := admininvestigations.Spec()
	profile, _ := adminowner.ForOwner(spec.Owner)
	if spec.Path() != profile.Path+"/commands" {
		t.Fatal("command path outside Fleet's private admin group", spec.Path())
	}
}

func TestPostgresFleetInvestigationLifecycleApprovalAndRealEffects(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	ref := "INV-" + uuid.NewString()
	open, err := e.svc.Execute(ctx, e.actor, purpose, e.scope, e.command("OPEN_INVESTIGATION", ref, 0))
	if err != nil || open.RecordedVersion != 1 || open.PriorState != "NONE" || open.ResultingState != "OPEN" {
		t.Fatal("open", err, open)
	}
	// Tenant ownership comes from the subject's authoritative record.
	if state, version, org := e.caseState(t, ref); state != "OPEN" || version != 1 || org != e.orgA {
		t.Fatal("case not recorded under the subject's organization", state, version, org)
	}
	// The investigations resource is the real downstream read model.
	tx, _ := e.db.Begin(ctx)
	item, err := adminresources.Source().Read(ctx, tx, "investigations", ref, false)
	binding, bindErr := adminresources.Source().Binding(ctx, tx, "investigations", ref)
	tx.Rollback(ctx)
	if err != nil || bindErr != nil || item.Status == nil || *item.Status != "OPEN" || item.SourceVersion == nil || *item.SourceVersion != 1 || binding.Canonical.Organization != e.orgA || binding.Canonical.Tenant != e.orgA {
		t.Fatal("investigation resource", err, bindErr, item, binding)
	}
	refs := map[string]string{}
	for _, r := range item.References {
		refs[r.Kind] = r.Reference
	}
	if refs["organization"] != e.orgA || refs["private_assignment"] != e.subject || refs["shipment"] != "" {
		t.Fatal("investigation references", refs)
	}
	// Stale expected version -> conflict, no effect.
	if _, err := e.svc.Execute(ctx, e.actor, purpose, e.scope, e.command("RECORD_FINDING", ref, 0)); !errors.Is(err, admincommand.ErrConflict) {
		t.Fatal("stale version accepted", err)
	}
	// Closing before a finding is an illegal transition with no side effects.
	early := e.command("CLOSE_INVESTIGATION", ref, 1)
	e.approve(&early, "staff-b")
	if _, err := e.svc.Execute(ctx, e.actor, purpose, e.scope, early); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("close from OPEN accepted", err)
	}
	if state, version, _ := e.caseState(t, ref); state != "OPEN" || version != 1 {
		t.Fatal("rejected transition leaked", state, version)
	}
	reopen := e.command("OPEN_INVESTIGATION", ref, 1)
	if _, err := e.svc.Execute(ctx, e.actor, purpose, e.scope, reopen); !errors.Is(err, admincommand.ErrInvalid) && !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("re-open accepted", err)
	}
	finding := e.command("RECORD_FINDING", ref, 1)
	recorded, err := e.svc.Execute(ctx, e.actor, purpose, e.scope, finding)
	if err != nil || recorded.ResultingState != "FINDING_RECORDED" || recorded.Outcome != "FINDING_SUBSTANTIATED" {
		t.Fatal("finding", err, recorded)
	}
	replay, err := e.svc.Execute(ctx, e.actor, purpose, e.scope, finding)
	if err != nil || replay != recorded {
		t.Fatal("idempotent replay did not return the original receipt", err)
	}
	// Approval required, not self-approved, bound to the exact intent.
	closeCmd := e.command("CLOSE_INVESTIGATION", ref, 2)
	if _, err := e.svc.Execute(ctx, e.actor, purpose, e.scope, closeCmd); !errors.Is(err, admincommand.ErrInvalid) {
		t.Fatal("close without approval accepted", err)
	}
	e.approve(&closeCmd, e.actor.Staff)
	if _, err := e.svc.Execute(ctx, e.actor, purpose, e.scope, closeCmd); !errors.Is(err, adminpolicy.ErrDenied) {
		t.Fatal("self approval accepted", err)
	}
	e.approve(&closeCmd, "staff-b")
	closeCmd.Approval.IntentDigest = strings.Repeat("0", 64)
	if _, err := e.svc.Execute(ctx, e.actor, purpose, e.scope, closeCmd); !errors.Is(err, adminpolicy.ErrDenied) {
		t.Fatal("approval of another intent accepted", err)
	}
	e.approve(&closeCmd, "staff-b")
	closed, err := e.svc.Execute(ctx, e.actor, purpose, e.scope, closeCmd)
	if err != nil || closed.ResultingState != "CLOSED" || closed.RecordedVersion != 3 || closed.ApprovalID == "" {
		t.Fatal("close", err, closed)
	}
	if _, err := e.svc.Execute(ctx, e.actor, purpose, e.scope, e.command("RECORD_FINDING", ref, 3)); !errors.Is(err, admincommand.ErrRejected) {
		t.Fatal("finding on closed case accepted", err)
	}
	var history int
	e.db.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_admin_investigation_history WHERE case_ref=$1`, ref).Scan(&history)
	if history != 3 {
		t.Fatal("history rows", history)
	}
	for _, q := range []string{`UPDATE fleet_partner_admin_investigation_history SET reason_code='X'`, `DELETE FROM fleet_partner_admin_investigation_history`, `DELETE FROM fleet_partner_admin_investigations`} {
		if _, err := e.db.Exec(ctx, q); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Fatal("investigation evidence mutable", q, err)
		}
	}
	back, err := e.svc.Readback(ctx, e.actor, purpose, e.scope, "private-assignments", e.subject, closeCmd.IdempotencyKey)
	if err != nil || back != closed {
		t.Fatal("readback", err)
	}
	h, err := e.svc.History(ctx, e.actor, purpose, e.scope, "private-assignments", e.subject)
	if err != nil || h.CurrentState != "CLOSED" || h.CurrentVersion != 3 || len(h.Decisions) != 3 {
		t.Fatal("history", err, h)
	}
}

func TestPostgresFleetInvestigationCrossTenantAndConcurrency(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	// Foreign subject under our scope: no context binds it here.
	foreign := e.command("OPEN_INVESTIGATION", "INV-"+uuid.NewString(), 0)
	foreign.SubjectID = e.foreign
	if _, err := e.svc.Execute(ctx, e.actor, purpose, e.scope, foreign); err == nil {
		t.Fatal("cross-tenant subject accepted")
	}
	// The executor independently derives the organization from the subject row.
	tx, err := e.db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = admininvestigations.Executor{}.Apply(ctx, tx, admincommand.Execution{Command: foreign, DecisionID: uuid.NewString(), Staff: "staff-a", Scope: e.scope, Now: time.Now().UTC()})
	tx.Rollback(ctx)
	if !errors.Is(err, admincommand.ErrNotFound) {
		t.Fatal("executor accepted subject of another organization", err)
	}
	var cases int
	e.db.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_admin_investigations WHERE subject_id=$1`, e.foreign).Scan(&cases)
	if cases != 0 {
		t.Fatal("foreign case created")
	}
	// A case of one subject cannot be addressed through another subject.
	ref := "INV-" + uuid.NewString()
	if _, err := e.svc.Execute(ctx, e.actor, purpose, e.scope, e.command("OPEN_INVESTIGATION", ref, 0)); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.svc.Execute(ctx, e.actor, purpose, e.scope, e.command("RECORD_FINDING", ref, 1))
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
			t.Fatal("unexpected concurrent error", err)
		}
	}
	if state, version, _ := e.caseState(t, ref); ok != 1 || state != "FINDING_RECORDED" || version != 2 {
		t.Fatal("concurrent findings", ok, state, version)
	}
	// Concurrent opens of the same new case reference: exactly one wins.
	shared := "INV-" + uuid.NewString()
	results = make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.svc.Execute(ctx, e.actor, purpose, e.scope, e.command("OPEN_INVESTIGATION", shared, 0))
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	ok = 0
	for err := range results {
		if err == nil {
			ok++
		}
	}
	if ok != 1 {
		t.Fatal("concurrent opens", ok)
	}
}

func TestPostgresFleetInvestigationSignedHTTPBoundary(t *testing.T) {
	e := setup(t)
	gin.SetMode(gin.TestMode)
	write, read := bytes.Repeat([]byte("w"), 32), bytes.Repeat([]byte("r"), 32)
	h := &admincommand.Handler{Service: e.svc, WriteSecret: write, ReadSecret: read}
	router := gin.New()
	h.RegisterOn(router.Group("/internal/fleet-partner/v1"))
	server := httptest.NewServer(router)
	defer server.Close()
	client, err := admincommand.NewClient(admininvestigations.Owner, server.URL, write, read)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	q := adminread.Query{Scope: adminread.Scope{Market: e.scope.Market, Zone: e.scope.Zone, Organization: e.scope.Organization, Tenant: e.scope.Tenant}, Staff: e.actor.Staff, Role: "staff", Purpose: purpose, PolicyID: e.svc.Policy.ID(), PolicyVersion: e.svc.Policy.Version(), PolicyChecksum: e.svc.Policy.Checksum(), Start: time.Now().Add(-time.Hour), End: time.Now(), Limit: 1}
	ref := "INV-" + uuid.NewString()
	open := e.command("OPEN_INVESTIGATION", ref, 0)
	receipt, err := client.Execute(context.Background(), q, open)
	if err != nil || receipt.ResultingState != "OPEN" {
		t.Fatal("signed open", err)
	}
	if back, err := client.Readback(context.Background(), q, "private-assignments", e.subject, open.IdempotencyKey); err != nil || back != receipt {
		t.Fatal("signed readback", err)
	}
	wrong, _ := admincommand.NewClient(admininvestigations.Owner, server.URL, bytes.Repeat([]byte("x"), 32), read)
	if _, err := wrong.Execute(context.Background(), q, e.command("RECORD_FINDING", ref, 1)); err == nil {
		t.Fatal("wrong write key accepted")
	}
	// A captured signed readback cannot be replayed.
	address := server.URL + e.svc.Spec.Path() + "/private-assignments/" + e.subject + "?" + q.Values().Encode()
	request, _ := http.NewRequest("GET", address, nil)
	if err := serviceauth.SignRequest(request, adminread.Caller, read, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	first, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	first.Body.Close()
	again, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	again.Body.Close()
	if first.StatusCode != 200 || again.StatusCode != 409 {
		t.Fatal("nonce replay", first.StatusCode, again.StatusCode)
	}
	history, err := client.History(context.Background(), q, "private-assignments", e.subject)
	if err != nil || history.CurrentState != "OPEN" || len(history.Decisions) != 1 {
		t.Fatal("signed history", err)
	}
}

// Media case authority: the real Fleet source proof route over actual
// migrations and native case rows. Closing the case ends evidence access.
func TestPostgresFleetInvestigationCaseAuthorityProof(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	active := "INV-" + uuid.NewString()
	if _, err := e.svc.Execute(ctx, e.actor, purpose, e.scope, e.command("OPEN_INVESTIGATION", active, 0)); err != nil {
		t.Fatal(err)
	}
	fixture.CaseBoundary(t, e.db, adminresources.Source(), admininvestigations.Owner, "investigations", active, true)
	closed := "INV-" + uuid.NewString()
	steps := []admincommand.Command{e.command("OPEN_INVESTIGATION", closed, 0), e.command("RECORD_FINDING", closed, 1), e.command("CLOSE_INVESTIGATION", closed, 2)}
	e.approve(&steps[2], "staff-b")
	for _, c := range steps {
		if _, err := e.svc.Execute(ctx, e.actor, purpose, e.scope, c); err != nil {
			t.Fatal(c.Action, err)
		}
	}
	fixture.CaseBoundary(t, e.db, adminresources.Source(), admininvestigations.Owner, "investigations", closed, false)
}
