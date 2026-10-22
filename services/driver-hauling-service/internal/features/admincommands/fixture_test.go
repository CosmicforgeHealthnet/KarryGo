package admincommands

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/services/hauling-service/internal/features/adminresources"
	"cosmicforge/logistics/services/hauling-service/internal/features/recovery"
	"cosmicforge/logistics/services/hauling-service/internal/features/truckcapability"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/admincontext"
	contextfixture "cosmicforge/logistics/shared/go/admincontext/testfixture"
	"cosmicforge/logistics/shared/go/adminowner"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"cosmicforge/logistics/shared/go/adminpolicy"
	policyfixture "cosmicforge/logistics/shared/go/adminpolicy/testfixture"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// world is a real Hauling owner database built from the service's actual
// migrations, with two corporate tenants and real trucks/providers/bookings.
type world struct {
	t                         *testing.T
	db                        *pgxpool.Pool
	svc                       *admincommand.Service
	exec                      *Executor
	source                    adminowner.Source
	store                     *admincontext.Store
	issuer                    *contextfixture.Fixture
	spec                      admincommand.Spec
	actor, otherActor         adminpolicy.Actor
	scope, otherScope         adminpolicy.Scope
	orgA, orgB                string
	provider, truck           string // org A, large flatbed
	provider2, truck2         string // org A, large flatbed (replacement)
	providerSmall, truckSmall string // org A, 1 tonne
	providerB, truckB         string // org B
	booking                   string // org A corporate booking, pending
}

const purpose = "case-1"

func newWorld(t *testing.T) *world {
	t.Helper()
	db := fixture.Database(t, "../../../migrations")
	ctx := context.Background()
	w := &world{t: t, db: db, orgA: uuid.NewString(), orgB: uuid.NewString()}
	w.spec, _ = admincommand.ForOwner(adminresources.Owner)
	w.provider, w.truck = w.seedProvider(w.orgA, 20_000_000)
	w.provider2, w.truck2 = w.seedProvider(w.orgA, 20_000_000)
	w.providerSmall, w.truckSmall = w.seedProvider(w.orgA, 1_000_000)
	w.providerB, w.truckB = w.seedProvider(w.orgB, 20_000_000)
	w.booking = w.seedBooking("pending_match", "", "", 5000)
	w.scope = adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: w.orgA, Tenant: w.orgA}
	w.otherScope = adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: w.orgB, Tenant: w.orgB}
	p := policyfixture.New(t, w.scope, adminresources.Owner, w.spec.Capability, w.spec.Contract)
	p.Document.Rules[0].Scopes = append(p.Document.Rules[0].Scopes, w.otherScope)
	read := p.Document.Rules[0]
	read.Capability, read.Permission = w.spec.ReadCapability, w.spec.ReadCapability
	p.Document.Rules = append(p.Document.Rules, read)
	p.Sign(t)
	w.source = adminresources.Source()
	w.issuer = contextfixture.New(t, adminresources.Owner)
	w.store = &admincontext.Store{DB: db, Authority: w.issuer.Authority, Reader: w.source}
	capability := truckcapability.New(truckcapability.Options{Repository: truckcapability.NewRepository(db)})
	rec := recovery.New(recovery.Options{Repository: recovery.NewRepository(db), Capability: capability, Events: recovery.NewOutboxSink(db, "analytics-service", true, nil), Enabled: true})
	w.exec = New(capability, rec)
	w.svc = &admincommand.Service{Spec: w.spec, DB: db, Policy: p.Engine, Contexts: w.store, Executor: w.exec, WriteEnabled: true, ReadEnabled: true}
	mk := func(s adminpolicy.Scope) adminpolicy.Actor {
		return adminpolicy.Actor{Product: "karryam", Staff: "staff-a", Roles: []string{"staff"}, Permissions: []string{w.spec.Capability, w.spec.ReadCapability}, Markets: []string{s.Market}, Zones: []string{s.Zone}, Organizations: []string{s.Organization}, Tenants: []string{s.Tenant}}
	}
	w.actor, w.otherActor = mk(w.scope), mk(w.otherScope)
	_ = ctx
	return w
}

func (w *world) exec1(sql string, args ...any) {
	w.t.Helper()
	if _, err := w.db.Exec(context.Background(), sql, args...); err != nil {
		w.t.Fatal(sql, err)
	}
}

func (w *world) seedProvider(org string, payloadGrams int64) (string, string) {
	provider, truck := uuid.NewString(), uuid.NewString()
	w.exec1(`INSERT INTO truck_providers(id,first_name,status) VALUES($1,'TEST_ONLY','active')`, provider)
	w.exec1(`INSERT INTO trucks(id,provider_id,truck_type,capacity_kg,plate_number,status,max_payload_grams) VALUES($1,$2,'flatbed',$3,$4,'active',$5)`, truck, provider, int(payloadGrams/1000), "TP-"+truck[:8], payloadGrams)
	w.exec1(`INSERT INTO hauling_organization_provider_links(organization_id,provider_id,membership_id,state) VALUES($1,$2,$3,'ACTIVE')`, org, provider, uuid.NewString())
	w.exec1(`INSERT INTO hauling_organization_truck_links(organization_id,truck_id,state) VALUES($1,$2,'ACTIVE')`, org, truck)
	return provider, truck
}

func (w *world) seedBooking(status, provider, truck string, weightKg int) string {
	id := uuid.NewString()
	var p, v any
	if provider != "" {
		p, v = provider, truck
	}
	w.exec1(`INSERT INTO haulage_bookings(id,customer_id,provider_id,truck_id,pickup_address,pickup_lat,pickup_lng,dropoff_address,dropoff_lat,dropoff_lng,cargo_type,cargo_weight_kg,status,preferred_truck_type,corporate_organization_id)
 VALUES($1,gen_random_uuid(),$2,$3,'A',6.52,3.37,'B',6.43,3.42,'general',$4,$5,'flatbed',$6)`, id, p, v, weightKg, status, w.orgA)
	return id
}

// allow imports an owner-signed authorization context for (kind,id) at scope.
func (w *world) allow(kind, id string, scope adminpolicy.Scope) {
	w.t.Helper()
	ctx := context.Background()
	tx, err := w.db.Begin(ctx)
	if err != nil {
		w.t.Fatal(err)
	}
	binding, err := w.source.Binding(ctx, tx, kind, id)
	tx.Rollback(ctx)
	if err != nil {
		w.t.Fatal(kind, err)
	}
	a := w.issuer.Approval(uuid.NewString(), kind, id, w.spec.Capability, binding, scope)
	a.Capabilities = []string{w.spec.Capability, w.spec.ReadCapability}
	if err := w.store.Import(ctx, w.issuer.Sign(w.t, a), time.Now()); err != nil {
		w.t.Fatal(kind, err)
	}
}

func (w *world) command(kind, id, action, caseRef string, expected int64, params map[string]string) admincommand.Command {
	if params == nil {
		params = map[string]string{}
	}
	return admincommand.Command{ContractVersion: w.spec.Contract, OperationID: uuid.NewString(), SubjectKind: kind, SubjectID: id, CaseRef: caseRef, Action: action, ExpectedVersion: expected, IdempotencyKey: uuid.NewString(), ReasonCode: "SAFETY_REVIEW", Params: params, RequestedAt: time.Now().UTC().Add(-time.Second)}
}

func (w *world) approve(c *admincommand.Command, approver string, scope adminpolicy.Scope) {
	c.Approval = &admincommand.Approval{ApprovalID: uuid.NewString(), ApproverRef: approver, IntentDigest: c.IntentDigest("staff-a", purpose, scope), ApprovedAt: time.Now().UTC().Add(-time.Second)}
}

func (w *world) run(c admincommand.Command) (admincommand.Receipt, error) {
	if w.spec.Actions[c.Action].RequiresApproval && c.Approval == nil {
		w.approve(&c, "staff-b", w.scope)
	}
	return w.svc.Execute(context.Background(), w.actor, purpose, w.scope, c)
}

func (w *world) must(c admincommand.Command) admincommand.Receipt {
	w.t.Helper()
	r, err := w.run(c)
	if err != nil {
		w.t.Fatal(c.Action, err)
	}
	return r
}

func (w *world) expect(c admincommand.Command, want error) {
	w.t.Helper()
	if _, err := w.run(c); !errors.Is(err, want) {
		w.t.Fatalf("%s: got %v want %v", c.Action, err, want)
	}
}

func (w *world) scalar(sql string, args ...any) string {
	w.t.Helper()
	var out string
	if err := w.db.QueryRow(context.Background(), sql, args...).Scan(&out); err != nil {
		w.t.Fatal(sql, err)
	}
	return out
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
