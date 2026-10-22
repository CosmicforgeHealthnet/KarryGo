package admincommand

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"cosmicforge/logistics/shared/go/adminread"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type subjectReader struct{}

func (subjectReader) Binding(ctx context.Context, tx pgx.Tx, kind, ref string) (admincontext.RecordBinding, error) {
	b := admincontext.RecordBinding{SourceContract: "customer.admin-resources.v1"}
	err := tx.QueryRow(ctx, `SELECT fingerprint,tenant_ref FROM test_subjects WHERE kind=$1 AND id=$2`, kind, ref).Scan(&b.Fingerprint, &b.Canonical.Tenant)
	if errors.Is(err, pgx.ErrNoRows) {
		return b, admincontext.ErrUnmapped
	}
	return b, err
}

// standing is a deliberately small but real owner state machine used only to
// certify the shared framework: ACTIVE <-> RESTRICTED with a version fence.
type standing struct{}

func (standing) Apply(ctx context.Context, tx pgx.Tx, in Execution) (Transition, error) {
	var version int64
	var state string
	if err := tx.QueryRow(ctx, `SELECT version,state FROM test_subjects WHERE kind=$1 AND id=$2 FOR UPDATE`, in.SubjectKind, in.SubjectID).Scan(&version, &state); err != nil {
		return Transition{}, ErrNotFound
	}
	if version != in.ExpectedVersion {
		return Transition{}, ErrConflict
	}
	next := ""
	switch in.Action {
	case "RESTRICT_ACCOUNT":
		if state != "ACTIVE" {
			return Transition{}, ErrRejected
		}
		next = "RESTRICTED"
	case "LIFT_RESTRICTION":
		if state != "RESTRICTED" {
			return Transition{}, ErrRejected
		}
		next = "ACTIVE"
	default:
		return Transition{}, ErrRejected
	}
	if _, err := tx.Exec(ctx, `UPDATE test_subjects SET version=version+1,state=$3 WHERE kind=$1 AND id=$2`, in.SubjectKind, in.SubjectID, next); err != nil {
		return Transition{}, err
	}
	return Transition{PriorVersion: version, PriorState: state, ResultingState: next, Outcome: "STANDING_" + next}, nil
}
func (standing) Current(ctx context.Context, tx pgx.Tx, kind, id string) (int64, string, error) {
	var v int64
	var s string
	err := tx.QueryRow(ctx, `SELECT version,state FROM test_subjects WHERE kind=$1 AND id=$2`, kind, id).Scan(&v, &s)
	return v, s, err
}

type fixture struct {
	svc     *Service
	pool    *pgxpool.Pool
	actor   adminpolicy.Actor
	scope   adminpolicy.Scope
	subject string
	issuer  *contextfixture.Fixture
}

func database(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ADMIN_OWNER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("ADMIN_OWNER_TEST_DATABASE_URL required for owner command ledger tests")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil || !strings.Contains(config.ConnConfig.Database, "test") {
		t.Fatal("disposable test database required")
	}
	ctx := context.Background()
	root, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "owner_command_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close(); root.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); root.Close() })
	if _, err = pool.Exec(ctx, admincontext.Schema+";"+adminowner.Schema+";"+Schema+`;CREATE TABLE test_subjects(kind text NOT NULL,id text NOT NULL,fingerprint text NOT NULL,tenant_ref text NOT NULL,version bigint NOT NULL,state text NOT NULL,PRIMARY KEY(kind,id))`); err != nil {
		t.Fatal(err)
	}
	return pool
}

func setup(t *testing.T) fixture {
	pool := database(t)
	ctx := context.Background()
	spec, _ := ForOwner("customer-service")
	scope := adminpolicy.Scope{Product: "karryam", Market: "NG-LAG", Zone: "lekki", Organization: "org-1", Tenant: "tenant-1"}
	p := policyfixture.New(t, scope, spec.Owner, spec.Capability, spec.Contract)
	read := p.Document.Rules[0]
	read.Capability, read.Permission = spec.ReadCapability, spec.ReadCapability
	p.Document.Rules = append(p.Document.Rules, read)
	p.Sign(t)
	subject := uuid.NewString()
	fingerprint := strings.Repeat("ab", 32)
	if _, err := pool.Exec(ctx, `INSERT INTO test_subjects VALUES('customers',$1,$2,'tenant-1',0,'ACTIVE')`, subject, fingerprint); err != nil {
		t.Fatal(err)
	}
	issuer := contextfixture.New(t, spec.Owner)
	store := &admincontext.Store{DB: pool, Authority: issuer.Authority, Reader: subjectReader{}}
	approval := issuer.Approval("ctx-"+subject, "customers", subject, spec.Capability, admincontext.RecordBinding{SourceContract: "customer.admin-resources.v1", Fingerprint: fingerprint}, scope)
	approval.Capabilities = []string{spec.Capability, spec.ReadCapability}
	if err := store.Import(ctx, issuer.Sign(t, approval), time.Now()); err != nil {
		t.Fatal(err)
	}
	actor := adminpolicy.Actor{Product: "karryam", Staff: "staff-a", Roles: []string{"staff"}, Permissions: []string{spec.Capability, spec.ReadCapability}, Markets: []string{scope.Market}, Zones: []string{scope.Zone}, Organizations: []string{scope.Organization}, Tenants: []string{scope.Tenant}}
	svc := &Service{Spec: spec, DB: pool, Policy: p.Engine, Contexts: store, Executor: standing{}, WriteEnabled: true, ReadEnabled: true}
	return fixture{svc: svc, pool: pool, actor: actor, scope: scope, subject: subject, issuer: issuer}
}

func (f fixture) command(action string, expected int64) Command {
	c := Command{ContractVersion: f.svc.Spec.Contract, OperationID: uuid.NewString(), SubjectKind: "customers", SubjectID: f.subject, Action: action, ExpectedVersion: expected, IdempotencyKey: uuid.NewString(), ReasonCode: "FRAUD_REVIEW", Params: map[string]string{}, RequestedAt: time.Now().UTC().Add(-time.Second)}
	if action == "RESTRICT_ACCOUNT" {
		c.Params["restriction"] = "LOGIN_BLOCKED"
	}
	return c
}
func (f fixture) approve(c *Command, approver string) {
	c.Approval = &Approval{ApprovalID: uuid.NewString(), ApproverRef: approver, IntentDigest: c.IntentDigest(f.actor.Staff, "case-1", f.scope), ApprovedAt: time.Now().UTC().Add(-time.Second)}
}

func TestOwnerCommandDictionaryRejectsUnknownActionsParamsAndUnboundApproval(t *testing.T) {
	spec, _ := ForOwner("customer-service")
	now := time.Now()
	base := Command{ContractVersion: spec.Contract, OperationID: uuid.NewString(), SubjectKind: "customers", SubjectID: "c-1", Action: "RESTRICT_ACCOUNT", IdempotencyKey: "k-1", ReasonCode: "FRAUD_REVIEW", Params: map[string]string{"restriction": "LOGIN_BLOCKED"}, RequestedAt: now}
	if _, err := base.Validate(spec, now); !errors.Is(err, ErrInvalid) {
		t.Fatal("approval-required action accepted without approval", err)
	}
	for name, mutate := range map[string]func(*Command){
		"generic state setter": func(c *Command) { c.Action = "SET_STATE" },
		"unknown param":        func(c *Command) { c.Params["state"] = "ACTIVE" },
		"out of enum":          func(c *Command) { c.Params["restriction"] = "DELETE_ACCOUNT" },
		"wrong subject kind":   func(c *Command) { c.SubjectKind = "consents" },
		"stale request":        func(c *Command) { c.RequestedAt = now.Add(-time.Hour) },
		"negative version":     func(c *Command) { c.ExpectedVersion = -1 },
	} {
		c := base
		c.Params = map[string]string{"restriction": "LOGIN_BLOCKED"}
		c.Approval = &Approval{ApprovalID: uuid.NewString(), ApproverRef: "staff-b", IntentDigest: strings.Repeat("a", 64), ApprovedAt: now}
		mutate(&c)
		if _, err := c.Validate(spec, now); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s accepted", name)
		}
	}
	for _, raw := range []string{
		`{"contract_version":"customer.admin-command.v1","Contract_version":"x"}`,
		`{"contract_version":"a","contract_version":"b"}`,
	} {
		if _, err := DecodeCommand([]byte(raw)); !errors.Is(err, ErrInvalid) {
			t.Fatalf("ambiguous body accepted: %s", raw)
		}
	}
	for _, s := range Specs() {
		for name := range s.Actions {
			if strings.HasPrefix(name, "SET_") || strings.Contains(name, "OVERRIDE") || strings.Contains(name, "FORCE") {
				t.Fatalf("generic state setter %s in %s", name, s.Owner)
			}
		}
		for _, kind := range s.SubjectKinds() {
			if _, ok := s.SubjectContract(kind); !ok {
				t.Fatalf("%s subject kind %s is not an enrolled owner resource", s.Owner, kind)
			}
		}
	}
}

func TestPostgresOwnerCommandApprovalVersionFenceIdempotencyAndImmutableLedger(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	c := f.command("RESTRICT_ACCOUNT", 0)
	f.approve(&c, f.actor.Staff)
	if _, err := f.svc.Execute(ctx, f.actor, "case-1", f.scope, c); !errors.Is(err, adminpolicy.ErrDenied) {
		t.Fatal("requester approved own command", err)
	}
	f.approve(&c, "staff-b")
	c.Approval.IntentDigest = strings.Repeat("0", 64)
	if _, err := f.svc.Execute(ctx, f.actor, "case-1", f.scope, c); !errors.Is(err, adminpolicy.ErrDenied) {
		t.Fatal("approval for another intent accepted", err)
	}
	f.approve(&c, "staff-b")
	// Concurrent distinct operations against the same expected version: only
	// one can commit; every loser observes a version conflict, never a duplicate.
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cmd := c
			if i > 0 {
				cmd = f.command("RESTRICT_ACCOUNT", 0)
				f.approve(&cmd, "staff-b")
			}
			_, err := f.svc.Execute(ctx, f.actor, "case-1", f.scope, cmd)
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrConflict) && !errors.Is(err, ErrRejected) {
			t.Fatal("unexpected concurrent error", err)
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one committed transition, got %d", successes)
	}
	version, state, _ := standing{}.Current(ctx, mustTx(t, f.pool), "customers", f.subject)
	if version != 1 || state != "RESTRICTED" {
		t.Fatal("owner state not transitioned exactly once", version, state)
	}
	// Approval identity is single use even for a new operation.
	lift := f.command("LIFT_RESTRICTION", 1)
	f.approve(&lift, "staff-b")
	first, err := f.svc.Execute(ctx, f.actor, "case-1", f.scope, lift)
	if err != nil || first.RecordedVersion != 2 || first.PriorState != "RESTRICTED" || first.ResultingState != "ACTIVE" {
		t.Fatal("lift failed", err, first)
	}
	replay, err := f.svc.Execute(ctx, f.actor, "case-1", f.scope, lift)
	if err != nil || replay != first {
		t.Fatal("identical replay did not return the original receipt", err)
	}
	reuse := lift
	reuse.OperationID = uuid.NewString()
	if _, err := f.svc.Execute(ctx, f.actor, "case-1", f.scope, reuse); !errors.Is(err, ErrConflict) {
		t.Fatal("idempotency key reused for a different operation", err)
	}
	again := f.command("RESTRICT_ACCOUNT", 2)
	again.Approval = lift.Approval
	again.Approval = &Approval{ApprovalID: lift.Approval.ApprovalID, ApproverRef: "staff-b", IntentDigest: again.IntentDigest(f.actor.Staff, "case-1", f.scope), ApprovedAt: time.Now().UTC()}
	if _, err := f.svc.Execute(ctx, f.actor, "case-1", f.scope, again); !errors.Is(err, ErrConflict) {
		t.Fatal("approval identity consumed twice", err)
	}
	if v, s, _ := (standing{}).Current(ctx, mustTx(t, f.pool), "customers", f.subject); v != 2 || s != "ACTIVE" {
		t.Fatal("refused command leaked a domain effect", v, s)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE admin_owner_command_decisions SET action='X'`); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatal("ledger mutable")
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM admin_owner_command_decisions`); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatal("ledger deletable", err)
	}
	back, err := f.svc.Readback(ctx, f.actor, "case-1", f.scope, "customers", f.subject, lift.IdempotencyKey)
	if err != nil || back != first {
		t.Fatal("readback did not return original receipt", err)
	}
	history, err := f.svc.History(ctx, f.actor, "case-1", f.scope, "customers", f.subject)
	if err != nil || history.CurrentVersion != 2 || history.CurrentState != "ACTIVE" || len(history.Decisions) != 2 || history.Decisions[0].DecisionID != first.DecisionID {
		t.Fatal("history wrong", err, history)
	}
	other := f.scope
	other.Tenant = "tenant-2"
	otherActor := f.actor
	otherActor.Tenants = []string{"tenant-2"}
	if _, err := f.svc.Readback(ctx, otherActor, "case-1", other, "customers", f.subject, lift.IdempotencyKey); err == nil {
		t.Fatal("cross-tenant readback disclosed")
	}
	var audits int
	f.pool.QueryRow(ctx, `SELECT count(*) FROM admin_owner_read_audit WHERE resource_kind='admin-command-decisions'`).Scan(&audits)
	if audits != 2 {
		t.Fatal("disclosures not audited", audits)
	}
}

func mustTx(t *testing.T, pool *pgxpool.Pool) pgx.Tx {
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tx.Rollback(context.Background()) })
	return tx
}

func TestPostgresOwnerCommandSignedHTTPBoundaryAndClientProof(t *testing.T) {
	f := setup(t)
	gin.SetMode(gin.TestMode)
	write, read := []byte(strings.Repeat("w", 32)), []byte(strings.Repeat("r", 32))
	h := &Handler{Service: f.svc, WriteSecret: write, ReadSecret: read}
	router := gin.New()
	h.Register(router)
	server := httptest.NewServer(router)
	defer server.Close()
	client, err := NewClient("customer-service", server.URL, write, read)
	if err != nil {
		t.Fatal(err)
	}
	q := adminread.Query{Scope: adminread.Scope{Market: f.scope.Market, Zone: f.scope.Zone, Organization: f.scope.Organization, Tenant: f.scope.Tenant}, Staff: f.actor.Staff, Purpose: "case-1", Start: time.Now().Add(-time.Hour), End: time.Now(), Limit: 1, Role: "staff", PolicyID: f.svc.Policy.ID(), PolicyVersion: f.svc.Policy.Version(), PolicyChecksum: f.svc.Policy.Checksum()}
	c := f.command("RESTRICT_ACCOUNT", 0)
	f.approve(&c, "staff-b")
	receipt, err := client.Execute(context.Background(), q, c)
	if err != nil || receipt.ResultingState != "RESTRICTED" {
		t.Fatal("signed command failed", err)
	}
	back, err := client.Readback(context.Background(), q, "customers", f.subject, c.IdempotencyKey)
	if err != nil || back != receipt {
		t.Fatal("signed readback failed", err)
	}
	if _, err := client.Readback(context.Background(), q, "customers", f.subject, uuid.NewString()); !errors.Is(err, ErrAbsent) {
		t.Fatal("absent readback not distinguished", err)
	}
	stale := f.command("LIFT_RESTRICTION", 0)
	f.approve(&stale, "staff-b")
	if _, err := client.Execute(context.Background(), q, stale); !errors.Is(err, ErrRefused) {
		t.Fatal("stale version not refused over the wire", err)
	}
	wrongKey, _ := NewClient("customer-service", server.URL, []byte(strings.Repeat("x", 32)), read)
	if _, err := wrongKey.Execute(context.Background(), q, f.command("LIFT_RESTRICTION", 1)); err == nil {
		t.Fatal("unsigned writer accepted")
	}
	readOnly, _ := NewClient("customer-service", server.URL, nil, read)
	if _, err := readOnly.Execute(context.Background(), q, f.command("LIFT_RESTRICTION", 1)); !errors.Is(err, ErrInvalid) {
		t.Fatal("read key used for a write")
	}
	history, err := client.History(context.Background(), q, "customers", f.subject)
	if err != nil || len(history.Decisions) != 1 {
		t.Fatal("history over the wire failed", err)
	}
	raw, _ := json.Marshal(history)
	if strings.Contains(string(raw), "staff-a") {
		t.Fatal("history discloses requesting staff identity")
	}
}
