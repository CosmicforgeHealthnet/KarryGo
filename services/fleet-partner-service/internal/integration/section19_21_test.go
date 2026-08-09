package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/compliance"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/proof"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/safety"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/specialized"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/warehouse"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

type restrictionAuthority struct{}

func (restrictionAuthority) Configured() bool { return true }
func (restrictionAuthority) Apply(_ context.Context, _ platformclients.CallMeta, in platformclients.RestrictionCommand) (platformclients.RestrictionOutcome, error) {
	return platformclients.RestrictionOutcome{RestrictionID: in.RestrictionID, State: "APPLIED", SourceVersion: in.ExpectedVersion, SubjectType: in.SubjectType, SubjectReference: in.SubjectReference}, nil
}
func (restrictionAuthority) Read(_ context.Context, _ platformclients.CallMeta, id string) (platformclients.RestrictionOutcome, error) {
	return platformclients.RestrictionOutcome{RestrictionID: id, State: "APPLIED", SourceVersion: 1}, nil
}
func (restrictionAuthority) Clear(_ context.Context, _ platformclients.CallMeta, in platformclients.RestrictionCommand) (platformclients.RestrictionOutcome, error) {
	return platformclients.RestrictionOutcome{RestrictionID: in.RestrictionID, State: "CLEARED", SourceVersion: in.ExpectedVersion}, nil
}
func (restrictionAuthority) ReadClear(_ context.Context, _ platformclients.CallMeta, id string) (platformclients.RestrictionOutcome, error) {
	return platformclients.RestrictionOutcome{RestrictionID: id, State: "CLEARED", SourceVersion: 2}, nil
}

type ambiguousRestrictionAuthority struct {
	mu         sync.Mutex
	applyCalls int
	readCalls  int
}

type unrestrictedWarehouseGate struct{}

func (unrestrictedWarehouseGate) Restricted(context.Context, string, string, string) (bool, error) {
	return false, nil
}

func (*ambiguousRestrictionAuthority) Configured() bool { return true }
func (a *ambiguousRestrictionAuthority) Apply(_ context.Context, _ platformclients.CallMeta, in platformclients.RestrictionCommand) (platformclients.RestrictionOutcome, error) {
	a.mu.Lock()
	a.applyCalls++
	a.mu.Unlock()
	return platformclients.RestrictionOutcome{}, &platformclients.Error{Service: "restriction-test", Outcome: platformclients.OutcomeUnknown}
}
func (a *ambiguousRestrictionAuthority) Read(_ context.Context, _ platformclients.CallMeta, id string) (platformclients.RestrictionOutcome, error) {
	a.mu.Lock()
	a.readCalls++
	a.mu.Unlock()
	return platformclients.RestrictionOutcome{RestrictionID: id, State: "APPLIED", SourceVersion: 1}, nil
}
func (a *ambiguousRestrictionAuthority) Clear(_ context.Context, _ platformclients.CallMeta, _ platformclients.RestrictionCommand) (platformclients.RestrictionOutcome, error) {
	return platformclients.RestrictionOutcome{}, errors.New("unexpected clear")
}
func (a *ambiguousRestrictionAuthority) ReadClear(_ context.Context, _ platformclients.CallMeta, _ string) (platformclients.RestrictionOutcome, error) {
	return platformclients.RestrictionOutcome{}, errors.New("unexpected clear read-back")
}

func section1921Scope(t *testing.T, pool *pgxpool.Pool) (tenant.Context, string) {
	t.Helper()
	org := createOrganization(t, pool, "Sections 19 21 "+ids.New("org"), "S1921-"+ids.New("reg"))
	branch := ids.New("brn")
	if _, err := pool.Exec(context.Background(), `INSERT INTO fleet_partner_branches(id,organization_id,code,name,timezone)VALUES($1,$2,$3,'Operations','Europe/Rome')`, branch, org.ID, "B"+branch[len(branch)-6:]); err != nil {
		t.Fatal(err)
	}
	actor := ids.New("usr")
	return tenant.Context{OrganizationID: org.ID, OrganizationUserID: actor, ActorType: tenant.ActorTypeOrganizationUser, ActorID: actor, CorrelationID: ids.New("cor"), PermittedBranchIDs: []string{branch}}, branch
}

func TestSection19AmbiguousRestrictionUsesReadBackWithoutResubmission(t *testing.T) {
	pool := testPool(t)
	scope, _ := section1921Scope(t, pool)
	authority := &ambiguousRestrictionAuthority{}
	svc := compliance.NewService(compliance.Options{
		Pool: pool, Events: events.NewDomainPublisher(events.NewOutboxRepository(pool)),
		Dispatch: authority, Hauling: authority,
		Flags: compliance.Flags{Compliance: true, Restrictions: true},
	})
	restriction, err := svc.ApplyRestriction(context.Background(), scope, compliance.RestrictInput{
		SubjectType: "PROVIDER", SubjectReference: "membership-ambiguous", ReasonCode: "SAFETY_BLOCK",
		SourceType: "INCIDENT", SourceReference: "incident-ambiguous", Severity: "CRITICAL",
	})
	if err != nil {
		t.Fatal(err)
	}
	if authority.applyCalls != 2 {
		t.Fatalf("initial apply calls=%d, want one per authority", authority.applyCalls)
	}
	if err = svc.ProcessRestriction(context.Background(), scope.OrganizationID, restriction.ID); err != nil {
		t.Fatal(err)
	}
	if authority.applyCalls != 2 || authority.readCalls != 2 {
		t.Fatalf("apply=%d read=%d; ambiguous outcomes must be read back without resubmission", authority.applyCalls, authority.readCalls)
	}
	var pending int
	if err = pool.QueryRow(context.Background(), `SELECT count(*) FROM fleet_partner_restriction_operations WHERE restriction_id=$1 AND status<>'CONFIRMED'`, restriction.ID).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Fatalf("pending restriction operations=%d", pending)
	}
}

func TestSection19ComplianceRestrictionSafetyAndProof(t *testing.T) {
	pool := testPool(t)
	scope, _ := section1921Scope(t, pool)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	publisher := events.NewDomainPublisher(events.NewOutboxRepository(pool))
	authority := restrictionAuthority{}
	c := compliance.NewService(compliance.Options{Pool: pool, Events: publisher, Dispatch: authority, Hauling: authority, Flags: compliance.Flags{Compliance: true, Restrictions: true}, Now: func() time.Time { return now }})
	req, err := c.CreateRequirement(ctx, scope, compliance.Requirement{Code: "ORG_ACTIVE", Name: "Organization active", SubjectType: "ORGANIZATION", RequirementType: "PLATFORM_STATUS", Severity: "CRITICAL", BlocksOperation: true, EffectiveFrom: now.Add(-time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	req, err = c.UpdateRequirement(ctx, scope, req.ID, req.Version, compliance.Requirement{}, "ACTIVATE")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE fleet_partner_organizations SET status='SUSPENDED' WHERE id=$1`, scope.OrganizationID); err != nil {
		t.Fatal(err)
	}
	status, err := c.Evaluate(ctx, scope, compliance.EvaluateInput{SubjectType: "ORGANIZATION", SubjectReference: scope.OrganizationID, RequirementID: req.ID, SourceType: "PLATFORM", SourceReference: "org-state", SourceVersion: 1})
	if err != nil || status.State != compliance.StateRestricted {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	gate, err := c.CheckSubject(ctx, scope.OrganizationID, "ORGANIZATION", scope.OrganizationID)
	if err != nil || gate.Eligible {
		t.Fatalf("gate=%+v err=%v", gate, err)
	}
	restriction, err := c.GetRestriction(ctx, scope.OrganizationID, status.RestrictionReference)
	if err != nil {
		t.Fatal(err)
	}
	restriction, err = c.ClearRestriction(ctx, scope, restriction.ID, "evidence restored", restriction.Version)
	if err != nil || restriction.State != "CLEARED" {
		t.Fatalf("clear=%+v err=%v", restriction, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE fleet_partner_organizations SET status='ACTIVE' WHERE id=$1`, scope.OrganizationID); err != nil {
		t.Fatal(err)
	}

	safe := safety.NewOperationsService(safety.OperationsOptions{Pool: pool, Events: publisher, Flags: safety.OperationsFlags{Reports: true, Incidents: true}, Now: func() time.Time { return now }})
	report, err := safe.CreateReport(ctx, scope, safety.Report{Anonymous: true, ReportType: "HAZARD", Severity: "HIGH", Description: "Unsecured loading area"})
	if err != nil || report.Status != "OPEN" {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	incident, err := safe.CreateIncident(ctx, scope, safety.Incident{IncidentType: "ACCIDENT", Severity: "HIGH", Description: "Controlled test incident"})
	if err != nil || incident.Status != "OPEN" {
		t.Fatalf("incident=%+v err=%v", incident, err)
	}

	p := proof.NewService(proof.Options{Pool: pool, Events: publisher, HashSecret: []byte("0123456789abcdef0123456789abcdef"), Flags: proof.Flags{Policies: true, Records: true}, Now: func() time.Time { return now }})
	_, snap, err := p.SavePolicy(ctx, scope, "", 0, proof.Policy{PolicyScope: "ORGANIZATION", ScopeReference: scope.OrganizationID, RequiredProofTypes: json.RawMessage(`["OTP"]`), MinimumCount: 1, FailurePolicy: "BLOCK"})
	if err != nil {
		t.Fatal(err)
	}
	record, err := p.CreateRecord(ctx, scope, proof.RecordInput{PolicySnapshotReference: snap.ID, ProofType: "OTP", OrderID: "order-proof-1", ActorType: "ORGANIZATION_USER", Value: "246810", SourceTimestamp: now, Source: "MANUAL", SourceVersion: 1, IdempotencyKey: "proof-once"})
	if err != nil || record.ValueHash == "" {
		t.Fatalf("record=%+v err=%v", record, err)
	}
	replay, err := p.CreateRecord(ctx, scope, proof.RecordInput{PolicySnapshotReference: snap.ID, ProofType: "OTP", OrderID: "order-proof-1", ActorType: "ORGANIZATION_USER", Value: "different", SourceTimestamp: now, Source: "MANUAL", SourceVersion: 1, IdempotencyKey: "proof-once"})
	if err != nil || replay.ID != record.ID {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
}

func TestSection20CapabilityIsolationValidationAndIdempotency(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Section 20", "S20-"+ids.New("reg"))
	actor := ids.New("usr")
	scope := tenant.Context{OrganizationID: org.ID, OrganizationUserID: actor, ActorType: tenant.ActorTypeOrganizationUser, ActorID: actor, CorrelationID: "s20"}
	s := specialized.NewService(specialized.Options{Pool: pool, Events: events.NewDomainPublisher(events.NewOutboxRepository(pool)), Flags: specialized.Flags{Enabled: true, Modules: map[string]bool{specialized.Courier: true, specialized.Taxi: false}}})
	capability, err := s.SetCapability(context.Background(), scope, specialized.Courier, "ENABLED", "approved rollout", 0, json.RawMessage(`{}`))
	if err != nil || capability.Status != "ENABLED" {
		t.Fatalf("capability=%+v err=%v", capability, err)
	}
	record, err := s.CreateRecord(context.Background(), scope, specialized.Record{CapabilityCode: specialized.Courier, RecordType: "SHIPPING_LABEL", SubjectType: "PACKAGE", SubjectReference: "pkg-1", Payload: json.RawMessage(`{"labelReference":"lbl-1","barcodeReference":"bc-1"}`), IdempotencyKey: "courier-once"})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.CreateRecord(context.Background(), scope, specialized.Record{CapabilityCode: specialized.Courier, RecordType: "SHIPPING_LABEL", SubjectType: "PACKAGE", SubjectReference: "pkg-1", Payload: json.RawMessage(`{"labelReference":"lbl-1","barcodeReference":"bc-1"}`), IdempotencyKey: "courier-once"})
	if err != nil || replay.ID != record.ID {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	if _, err = s.CreateRecord(context.Background(), scope, specialized.Record{CapabilityCode: specialized.Courier, RecordType: "SHIPPING_LABEL", SubjectType: "PACKAGE", SubjectReference: "pkg-2", Payload: json.RawMessage(`{"labelReference":"lbl-2","barcodeReference":"bc-2","amount":100}`), IdempotencyKey: "finance-reject"}); err == nil {
		t.Fatal("financial field accepted")
	}
	if _, err = s.SetCapability(context.Background(), scope, specialized.Taxi, "ENABLED", "must stay closed", 0, json.RawMessage(`{}`)); err == nil {
		t.Fatal("taxi capability enabled")
	}
}

func TestSection21WarehouseReceiptConcurrencyAndNegativeStock(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Section 21", "S21-"+ids.New("reg"))
	branch := ids.New("brn")
	if _, err := pool.Exec(context.Background(), `INSERT INTO fleet_partner_branches(id,organization_id,code,name,timezone)VALUES($1,$2,'S21','Warehouse','Europe/Rome')`, branch, org.ID); err != nil {
		t.Fatal(err)
	}
	actor := ids.New("usr")
	scope := tenant.Context{OrganizationID: org.ID, OrganizationUserID: actor, ActorType: tenant.ActorTypeOrganizationUser, ActorID: actor, CorrelationID: "s21", PermittedBranchIDs: []string{branch}}
	svc := warehouse.NewService(warehouse.Options{Pool: pool, Events: events.NewDomainPublisher(events.NewOutboxRepository(pool)), Restrictions: unrestrictedWarehouseGate{}, Flags: warehouse.Flags{Warehouses: true, Inventory: true, Fulfilment: true, Dock: true, Yard: true, Terminal: true, Returns: true}})
	w, err := svc.CreateWarehouse(context.Background(), scope, warehouse.Warehouse{BranchID: branch, Code: "WH1", Name: "Main", AddressReference: "addr-1", Timezone: "Europe/Rome"})
	if err != nil {
		t.Fatal(err)
	}
	w, err = svc.UpdateWarehouse(context.Background(), scope, w.ID, w.Version, warehouse.Warehouse{Status: "ACTIVE"}, false)
	if err != nil {
		t.Fatal(err)
	}
	loc, err := svc.CreateLocation(context.Background(), scope, w.ID, warehouse.Location{Code: "A1", Name: "Bin 1", LocationType: "BIN"})
	if err != nil {
		t.Fatal(err)
	}
	item, err := svc.Receipt(context.Background(), scope, warehouse.ReceiptInput{WarehouseID: w.ID, LocationID: loc.ID, OwnerType: "ORGANIZATION", OwnerReference: org.ID, SKU: "SKU-1", Unit: "EA", Condition: "GOOD", Quantity: 10, CorrelationID: "receipt-once"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.Receipt(context.Background(), scope, warehouse.ReceiptInput{WarehouseID: w.ID, LocationID: loc.ID, OwnerType: "ORGANIZATION", OwnerReference: org.ID, SKU: "SKU-1", Unit: "EA", Condition: "GOOD", Quantity: 10, CorrelationID: "receipt-once"})
	if err != nil || again.ID != item.ID || again.QuantityProjection != 10 {
		t.Fatalf("receipt replay=%+v err=%v", again, err)
	}
	if _, err = svc.Move(context.Background(), scope, item.ID, warehouse.MovementInput{MovementType: "ADJUSTMENT", Quantity: 11, ReasonCode: "COUNT", CorrelationID: "negative", ExpectedVersion: item.Version}); err == nil {
		t.Fatal("negative inventory accepted")
	}
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, e := svc.Move(context.Background(), scope, item.ID, warehouse.MovementInput{MovementType: "ADJUSTMENT", Quantity: 6, ReasonCode: "COUNT", CorrelationID: "concurrent-" + string(rune('a'+i)), ExpectedVersion: item.Version})
			errs <- e
		}(i)
	}
	wg.Wait()
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("concurrent movement successes=%d", success)
	}
	final, err := svc.GetItem(context.Background(), scope, item.ID)
	if err != nil || final.QuantityProjection != 4 {
		t.Fatalf("final=%+v err=%v", final, err)
	}
}
