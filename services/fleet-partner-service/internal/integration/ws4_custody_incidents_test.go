package integration_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	packagesmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/packages/models"
	packagesrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/packages/repositories"
	packagesusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/packages/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/safety"
	shipmentsrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

func ws4CountOutbox(t *testing.T, pool *pgxpool.Pool, org, eventType string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM fleet_partner_outbox WHERE organization_id=$1 AND event_type=$2`, org, eventType).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func ws4CountAudit(t *testing.T, pool *pgxpool.Pool, org, action string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id=$1 AND action=$2`, org, action).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func ws4Branch(t *testing.T, pool *pgxpool.Pool, org string) string {
	t.Helper()
	branch := ids.New("brn")
	if _, err := pool.Exec(context.Background(), `INSERT INTO fleet_partner_branches(id,organization_id,code,name,timezone)VALUES($1,$2,$3,'Ops','Africa/Lagos')`, branch, org, "B"+branch[len(branch)-6:]); err != nil {
		t.Fatal(err)
	}
	return branch
}

func ws4Asset(t *testing.T, pool *pgxpool.Pool, org, branch string) string {
	t.Helper()
	asset := ids.New("ast")
	if _, err := pool.Exec(context.Background(), `INSERT INTO fleet_partner_assets(id,organization_id,branch_id,asset_code,asset_type,ownership_type)VALUES($1,$2,$3,$4,'TRUCK','OWNED')`, asset, org, branch, "T-"+asset[len(asset)-8:]); err != nil {
		t.Fatal(err)
	}
	return asset
}

// TestWS4CustodyEventsGenericChain covers spec 19.12: the generic chain of
// custody for non-package subjects, its continuity and chronology rules,
// corrections, Media evidence references, append-only storage, tenant and
// branch isolation, concurrency, audit/outbox, and that a PACKAGE subject
// shares the existing package chain rather than a second one.
func TestWS4CustodyEventsGenericChain(t *testing.T) {
	f := newPackagesMatrixFixture(t)
	ctx := context.Background()
	org := f.tenant.OrganizationID
	// The fixture's service has no outbox; this suite asserts the events.
	f.service = packagesusecases.NewService(packagesusecases.Options{
		Pool: f.pool, Repository: f.repository,
		Shipments: app.NewPackagesShipmentsAdapter(shipmentsrepos.NewRepository(f.pool)),
		Devices:   trustedDeviceStub{valid: true}, ProviderScope: providerScopeStub{valid: true},
		Media: evidenceStub{valid: true}, Audit: audit.NewRecorder(),
		Events: events.NewDomainPublisher(events.NewOutboxRepository(f.pool)),
		Flags:  packagesusecases.Flags{Enabled: true, WritesEnabled: true, ScanningEnabled: true},
	})
	branch := ws4Branch(t, f.pool, org)
	asset := ws4Asset(t, f.pool, org, branch)
	at := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)

	in := func(from, to string, when time.Time, evidence ...string) packagesusecases.SubjectCustodyInput {
		return packagesusecases.SubjectCustodyInput{SubjectType: "asset", SubjectReference: asset, CustodyInput: packagesusecases.CustodyInput{
			FromActorType: "LOCATION", FromActorReference: from, ToActorType: "DRIVER", ToActorReference: to,
			Condition: "GOOD", OccurredAt: when, EvidenceMediaIDs: evidence}}
	}

	first, err := f.service.RecordSubjectCustody(ctx, f.tenant, in("", "driver-1", at, "media-1", "media-2", "media-1"))
	if err != nil {
		t.Fatal(err)
	}
	if first.SubjectType != packagesmodels.CustodySubjectAsset || first.SubjectReference != asset || first.PackageID != "" || len(first.EvidenceMediaIDs) != 2 || first.EvidenceMediaID != "media-1" {
		t.Fatalf("first event = %+v", first)
	}
	// The next handover must continue from the current holder, in order.
	if _, err = f.service.RecordSubjectCustody(ctx, f.tenant, in("someone-else", "driver-2", at.Add(time.Minute))); appStatus(err) != 409 {
		t.Fatalf("handover from a non-holder = %v", err)
	}
	cont := in("", "driver-2", at.Add(-time.Minute))
	cont.FromActorType, cont.FromActorReference = "DRIVER", "driver-1"
	if _, err = f.service.RecordSubjectCustody(ctx, f.tenant, cont); appStatus(err) != 409 {
		t.Fatalf("out-of-order handover = %v", err)
	}
	cont.OccurredAt = at.Add(10 * time.Minute)
	second, err := f.service.RecordSubjectCustody(ctx, f.tenant, cont)
	if err != nil {
		t.Fatal(err)
	}

	// Validation.
	bad := in("", "x", at)
	bad.SubjectType = "WAREHOUSE"
	if _, err = f.service.RecordSubjectCustody(ctx, f.tenant, bad); appStatus(err) != 400 {
		t.Fatalf("unsupported subject = %v", err)
	}
	tooMuch := in("DRIVER", "driver-3", at.Add(20*time.Minute), "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11")
	if _, err = f.service.RecordSubjectCustody(ctx, f.tenant, tooMuch); appStatus(err) != 400 {
		t.Fatalf("eleven evidence references = %v", err)
	}
	noReceiver := in("", "", at)
	if _, err = f.service.RecordSubjectCustody(ctx, f.tenant, noReceiver); appStatus(err) != 400 {
		t.Fatalf("missing receiver = %v", err)
	}

	// Evidence references are validated with the Media authority first.
	rejecting := packagesusecases.NewService(packagesusecases.Options{Pool: f.pool, Repository: packagesrepos.NewRepository(f.pool), Media: evidenceStub{valid: false}, Audit: audit.NewRecorder(), Flags: packagesusecases.Flags{Enabled: true, WritesEnabled: true}})
	unsafe := in("", "driver-3", at.Add(30*time.Minute), "unsafe-media")
	unsafe.FromActorType, unsafe.FromActorReference = "DRIVER", "driver-2"
	if _, err = rejecting.RecordSubjectCustody(ctx, f.tenant, unsafe); appStatus(err) != 503 {
		t.Fatalf("unvalidated evidence = %v", err)
	}

	// A correction names an event of the same subject.
	otherAsset := ws4Asset(t, f.pool, org, branch)
	foreign, err := f.service.RecordSubjectCustody(ctx, f.tenant, packagesusecases.SubjectCustodyInput{SubjectType: "ASSET", SubjectReference: otherAsset, CustodyInput: packagesusecases.CustodyInput{ToActorType: "YARD", ToActorReference: "yard-1", OccurredAt: at}})
	if err != nil {
		t.Fatal(err)
	}
	correction := in("", "driver-2b", at.Add(11*time.Minute))
	correction.CorrectionOfEventID = foreign.ID
	if _, err = f.service.RecordSubjectCustody(ctx, f.tenant, correction); appStatus(err) != 409 {
		t.Fatalf("correction of another subject's event = %v", err)
	}
	correction.CorrectionOfEventID = second.ID
	if _, err = f.service.RecordSubjectCustody(ctx, f.tenant, correction); err != nil {
		t.Fatalf("correction of own event: %v", err)
	}

	chain, err := f.service.SubjectCustody(ctx, f.tenant, "ASSET", asset)
	if err != nil || len(chain) != 3 || chain[0].ID != first.ID {
		t.Fatalf("chain = %d %v", len(chain), err)
	}
	got, err := f.service.GetCustodyEvent(ctx, f.tenant, second.ID)
	if err != nil || got.ToActorReference != "driver-2" {
		t.Fatalf("get = %+v %v", got, err)
	}

	// Append-only in the database.
	if _, err = f.pool.Exec(ctx, `UPDATE fleet_partner_custody_events SET condition='DAMAGED' WHERE id=$1`, first.ID); err == nil {
		t.Fatal("custody event was mutable")
	}
	if _, err = f.pool.Exec(ctx, `DELETE FROM fleet_partner_custody_events WHERE id=$1`, first.ID); err == nil {
		t.Fatal("custody event was deletable")
	}

	// Tenant and branch isolation: the same safe 404.
	if _, err = f.service.RecordSubjectCustody(ctx, f.other, in("", "intruder", at)); appStatus(err) != 404 {
		t.Fatalf("cross-tenant record = %v", err)
	}
	if _, err = f.service.SubjectCustody(ctx, f.other, "ASSET", asset); appStatus(err) != 404 {
		t.Fatalf("cross-tenant list = %v", err)
	}
	if _, err = f.service.GetCustodyEvent(ctx, f.other, first.ID); appStatus(err) != 404 {
		t.Fatalf("cross-tenant get = %v", err)
	}
	narrow := f.tenant
	narrow.PermittedBranchIDs = []string{ws4Branch(t, f.pool, org)}
	if _, err = f.service.SubjectCustody(ctx, narrow, "ASSET", asset); appStatus(err) != 404 {
		t.Fatalf("other-branch list = %v", err)
	}
	if _, err = f.service.GetCustodyEvent(ctx, narrow, first.ID); appStatus(err) != 404 {
		t.Fatalf("other-branch get = %v", err)
	}

	// Concurrency: two handovers from the same holder; exactly one lands.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			race := in("", "racer", at.Add(time.Duration(40+i)*time.Minute))
			race.FromActorType, race.FromActorReference = "DRIVER", "driver-2b"
			race.ToActorReference = "racer-" + string(rune('a'+i))
			_, e := f.service.RecordSubjectCustody(ctx, f.tenant, race)
			results <- e
		}(i)
	}
	wg.Wait()
	close(results)
	ok, conflicts := 0, 0
	for e := range results {
		switch appStatus(e) {
		case 0:
			ok++
		case 409:
			conflicts++
		default:
			t.Fatalf("race error = %v", e)
		}
	}
	if ok != 1 || conflicts != 1 {
		t.Fatalf("concurrent handovers ok=%d conflicts=%d", ok, conflicts)
	}

	if n := ws4CountAudit(t, f.pool, org, "custody.recorded"); n < 5 {
		t.Fatalf("custody audit rows = %d", n)
	}
	if n := ws4CountOutbox(t, f.pool, org, "custody_event.recorded.v1"); n < 5 {
		t.Fatalf("custody outbox rows = %d", n)
	}

	// A PACKAGE subject is the existing package chain, not a second one.
	pkg := f.createPackage(t)
	viaGeneric, err := f.service.RecordSubjectCustody(ctx, f.tenant, packagesusecases.SubjectCustodyInput{SubjectType: "PACKAGE", SubjectReference: pkg.ID, CustodyInput: packagesusecases.CustodyInput{ToActorType: "DRIVER", ToActorReference: "driver-p", OccurredAt: at, EvidenceMediaID: "media-p"}})
	if err != nil {
		t.Fatal(err)
	}
	packageChain, err := f.service.Custody(ctx, f.tenant, pkg.ID)
	if err != nil || len(packageChain) != 1 || packageChain[0].ID != viaGeneric.ID || packageChain[0].ShipmentID != pkg.ShipmentID || packageChain[0].SubjectType != "PACKAGE" {
		t.Fatalf("package chain = %+v %v", packageChain, err)
	}
	if _, err = f.service.RecordSubjectCustody(ctx, f.tenant, packagesusecases.SubjectCustodyInput{SubjectType: "PACKAGE", SubjectReference: "pkg_missing", CustodyInput: packagesusecases.CustodyInput{ToActorType: "DRIVER", ToActorReference: "d", OccurredAt: at}}); appStatus(err) != 404 {
		t.Fatalf("unknown package = %v", err)
	}
}

func ws4Member(t *testing.T, pool *pgxpool.Pool, org, role, label string) tenant.Context {
	t.Helper()
	id := seedApprovalMember(t, pool, org, role, label)
	return tenant.Context{OrganizationID: org, OrganizationUserID: id, ActorType: tenant.ActorTypeOrganizationUser, ActorID: id, CorrelationID: ids.New("cor")}
}

// TestWS4IncidentLifecycleInvestigationAndClosure covers spec 19.13 / 58.11:
// every transition, its preconditions and refusals, the organization's own
// investigation with separation of duties, corrective actions, claim-gated
// closure, version conflicts, tenant isolation, history, audit and outbox.
func TestWS4IncidentLifecycleInvestigationAndClosure(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "WS4 Incidents", "RC-WS4-INC").ID
	other := createOrganization(t, pool, "WS4 Incidents Other", "RC-WS4-INC-2").ID
	officer := ws4Member(t, pool, org, "rol_system_compliance", "officer")
	investigator := ws4Member(t, pool, org, "rol_system_operations", "investigator")
	outsider := ws4Member(t, pool, other, "rol_system_owner", "outsider")
	now := time.Now().UTC()
	s := safety.NewOperationsService(safety.OperationsOptions{Pool: pool, Events: events.NewDomainPublisher(events.NewOutboxRepository(pool)), Flags: safety.OperationsFlags{Reports: true, Incidents: true}})

	inc, err := s.CreateIncident(ctx, officer, safety.Incident{IncidentType: "ACCIDENT", Severity: "MEDIUM", Description: "Forklift contact at dock 3"})
	if err != nil || inc.Status != safety.IncidentOpen {
		t.Fatalf("create = %+v %v", inc, err)
	}
	// The state machine is enforced: PATCH cannot change status.
	if _, err = s.UpdateIncident(ctx, officer, inc.ID, inc.Version, safety.Incident{Status: "RESOLVED", ResolutionAction: "x", ResolutionReason: "y"}); appStatus(err) != 409 {
		t.Fatalf("status change by PATCH = %v", err)
	}
	if _, err = s.ContainIncident(ctx, officer, inc.ID, inc.Version, "barrier placed"); appStatus(err) != 409 {
		t.Fatalf("contain before triage = %v", err)
	}
	if _, err = s.TriageIncident(ctx, officer, inc.ID, inc.Version, safety.TriageInput{Severity: "HIGH", LocationReference: "dock-3"}); appStatus(err) != 400 {
		t.Fatalf("triage without immediate actions = %v", err)
	}
	if _, err = s.TriageIncident(ctx, officer, inc.ID, inc.Version, safety.TriageInput{Severity: "HIGH", LocationReference: "dock-3", ImmediateActions: "a", PeopleReferences: json.RawMessage(`{"p":1}`)}); appStatus(err) != 400 {
		t.Fatalf("triage with non-array people = %v", err)
	}
	if _, err = s.TriageIncident(ctx, outsider, inc.ID, inc.Version, safety.TriageInput{Severity: "HIGH", LocationReference: "dock-3", ImmediateActions: "a"}); appStatus(err) != 404 {
		t.Fatalf("cross-tenant triage = %v", err)
	}
	triaged, err := s.TriageIncident(ctx, officer, inc.ID, inc.Version, safety.TriageInput{Severity: "HIGH", LocationReference: "dock-3", ImmediateActions: "Area cordoned, first aid given", PeopleReferences: json.RawMessage(`["person-1"]`), AssetReferences: json.RawMessage(`["forklift-7"]`)})
	if err != nil || triaged.Status != safety.IncidentTriaged || triaged.Severity != "HIGH" || triaged.TriagedAt == nil || triaged.TriagedBy != officer.ActorID {
		t.Fatalf("triage = %+v %v", triaged, err)
	}
	if _, err = s.TriageIncident(ctx, officer, inc.ID, inc.Version, safety.TriageInput{Severity: "HIGH", LocationReference: "dock-3", ImmediateActions: "a"}); appStatus(err) != 409 {
		t.Fatalf("stale version = %v", err)
	}

	// Escalation keeps the stage it escalated from and resumes there.
	escalated, err := s.EscalateIncident(ctx, officer, inc.ID, triaged.Version, "regulatory", "Reportable injury")
	if err != nil || escalated.Status != safety.IncidentEscalated || escalated.PreEscalationStatus != safety.IncidentTriaged {
		t.Fatalf("escalate = %+v %v", escalated, err)
	}
	if _, err = s.EscalateIncident(ctx, officer, inc.ID, escalated.Version, "OTHER", "again"); appStatus(err) != 409 {
		t.Fatalf("escalating an escalated incident = %v", err)
	}
	contained, err := s.ContainIncident(ctx, officer, inc.ID, escalated.Version, "Dock closed, forklift isolated")
	if err != nil || contained.Status != safety.IncidentContained {
		t.Fatalf("contain = %+v %v", contained, err)
	}
	if _, err = s.CreateCorrectiveAction(ctx, officer, inc.ID, safety.CorrectiveAction{ActionType: "TRAINING", OwnerUserID: officer.ActorID, DueAt: now.Add(48 * time.Hour)}); appStatus(err) != 409 {
		t.Fatalf("corrective action before findings = %v", err)
	}

	// Investigation.
	if _, _, err = s.OpenInvestigation(ctx, officer, inc.ID, contained.Version, safety.InvestigationInput{InvestigatorUserID: outsider.OrganizationUserID, Scope: "s", EvidencePreservation: "e"}); appStatus(err) != 400 {
		t.Fatalf("foreign investigator = %v", err)
	}
	inv, investigating, err := s.OpenInvestigation(ctx, officer, inc.ID, contained.Version, safety.InvestigationInput{InvestigatorUserID: investigator.OrganizationUserID, Scope: "Dock 3 forklift operations", EvidencePreservation: "CCTV export retained, forklift tagged out"})
	if err != nil || investigating.Status != safety.IncidentInvestigating || inv.Status != "OPEN" {
		t.Fatalf("open investigation = %+v %+v %v", inv, investigating, err)
	}
	if _, err = s.SubmitFindings(ctx, officer, inv.ID, inv.Version, safety.FindingsInput{Findings: "f", RootCause: "r"}); appStatus(err) != 403 {
		t.Fatalf("findings by non-investigator = %v", err)
	}
	if _, err = s.SubmitFindings(ctx, investigator, inv.ID, inv.Version, safety.FindingsInput{Findings: "f"}); appStatus(err) != 400 {
		t.Fatalf("findings without root cause = %v", err)
	}
	inv, err = s.SubmitFindings(ctx, investigator, inv.ID, inv.Version, safety.FindingsInput{Findings: "Pedestrian lane unmarked", RootCause: "No segregation between forklifts and pedestrians", ContributingFactors: json.RawMessage(`["shift change"]`)})
	if err != nil || inv.Status != "FINDINGS_SUBMITTED" {
		t.Fatalf("findings = %+v %v", inv, err)
	}
	action := safety.CorrectiveAction{ActionType: "MARK_PEDESTRIAN_LANES", OwnerUserID: officer.OrganizationUserID, DueAt: now.Add(72 * time.Hour)}
	if _, _, _, err = s.ApproveFindings(ctx, investigator, inv.ID, inv.Version, []safety.CorrectiveAction{action}); appStatus(err) != 403 {
		t.Fatalf("investigator approving own findings = %v", err)
	}
	if _, _, _, err = s.ApproveFindings(ctx, officer, inv.ID, inv.Version, nil); appStatus(err) != 400 {
		t.Fatalf("approval without actions = %v", err)
	}
	if _, _, _, err = s.ApproveFindings(ctx, officer, inv.ID, inv.Version, []safety.CorrectiveAction{{ActionType: "x", OwnerUserID: "y", DueAt: now.Add(-time.Hour)}}); appStatus(err) != 400 {
		t.Fatalf("past due corrective action = %v", err)
	}
	inv, corrective, actions, err := s.ApproveFindings(ctx, officer, inv.ID, inv.Version, []safety.CorrectiveAction{action})
	if err != nil || inv.Status != "APPROVED" || corrective.Status != safety.IncidentCorrectiveAction || len(actions) != 1 {
		t.Fatalf("approve = %+v %+v %d %v", inv, corrective, len(actions), err)
	}

	// Resolution needs every action verified or cancelled.
	if _, err = s.ResolveIncident(ctx, officer, inc.ID, corrective.Version, "Lanes marked", "Root cause removed"); appStatus(err) != 409 {
		t.Fatalf("resolve with open actions = %v", err)
	}
	completed, err := s.Action(ctx, officer, actions[0].ID, "COMPLETE", "Lanes painted", actions[0].Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Action(ctx, investigator, completed.ID, "VERIFY", "", completed.Version); err != nil {
		t.Fatal(err)
	}
	resolved, err := s.ResolveIncident(ctx, officer, inc.ID, corrective.Version, "Lanes marked", "Root cause removed")
	if err != nil || resolved.Status != safety.IncidentResolved {
		t.Fatalf("resolve = %+v %v", resolved, err)
	}

	// Closure is gated on linked claims.
	claim := ids.New("clm")
	if _, err = pool.Exec(ctx, `INSERT INTO fleet_partner_claims(id,organization_id,claim_type,incident_id,currency,status)VALUES($1,$2,'PERSONAL_INJURY',$3,'NGN','SUBMITTED')`, claim, org, inc.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CloseIncident(ctx, officer, inc.ID, resolved.Version, "Reporting complete"); appStatus(err) != 409 {
		t.Fatalf("close with an open claim = %v", err)
	}
	if _, err = pool.Exec(ctx, `UPDATE fleet_partner_claims SET status='SETTLED' WHERE id=$1`, claim); err != nil {
		t.Fatal(err)
	}
	closed, err := s.CloseIncident(ctx, officer, inc.ID, resolved.Version, "Reporting complete")
	if err != nil || closed.Status != safety.IncidentClosed || closed.ClosedAt == nil || closed.ClosureReason == "" {
		t.Fatalf("close = %+v %v", closed, err)
	}
	if _, err = s.EscalateIncident(ctx, officer, inc.ID, closed.Version, "OTHER", "late"); appStatus(err) != 409 {
		t.Fatalf("escalating a closed incident = %v", err)
	}
	investigations, err := s.ListInvestigations(ctx, officer, inc.ID)
	if err != nil || len(investigations) != 1 || investigations[0].Status != "CLOSED" {
		t.Fatalf("investigations = %+v %v", investigations, err)
	}
	if _, err = s.ListInvestigations(ctx, outsider, inc.ID); appStatus(err) != 404 {
		t.Fatalf("cross-tenant investigations = %v", err)
	}

	var history int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_incident_history WHERE incident_id=$1`, inc.ID).Scan(&history); err != nil || history != 8 {
		t.Fatalf("history rows = %d %v", history, err)
	}
	for _, event := range []string{"incident.triaged.v1", "incident.escalated.v1", "incident.contained.v1", "incident.investigation_started.v1", "incident_investigation.findings_submitted.v1", "incident_investigation.findings_approved.v1", "incident.corrective_action_started.v1", "corrective_action.created.v1", "incident.resolved.v1", "incident.closed.v1"} {
		if ws4CountOutbox(t, pool, org, event) != 1 {
			t.Errorf("outbox %s missing", event)
		}
	}
	if ws4CountAudit(t, pool, org, "incident.closed") != 1 {
		t.Error("close audit missing")
	}
}

// TestWS4IncidentConcurrentTransitionsSerialize proves two triage commands
// against the same version produce one transition and one conflict.
func TestWS4IncidentConcurrentTransitionsSerialize(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "WS4 Incident Race", "RC-WS4-RACE").ID
	officer := ws4Member(t, pool, org, "rol_system_compliance", "officer")
	s := safety.NewOperationsService(safety.OperationsOptions{Pool: pool, Flags: safety.OperationsFlags{Incidents: true}})
	inc, err := s.CreateIncident(ctx, officer, safety.Incident{IncidentType: "SECURITY", Severity: "LOW", Description: "Gate left open"})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.TriageIncident(ctx, officer, inc.ID, inc.Version, safety.TriageInput{Severity: "LOW", LocationReference: "gate", ImmediateActions: "gate closed"})
			results <- e
		}()
	}
	wg.Wait()
	close(results)
	statuses := map[int]int{}
	for e := range results {
		statuses[appStatus(e)]++
	}
	if statuses[0] != 1 || statuses[409] != 1 {
		t.Fatalf("concurrent triage outcomes = %v", statuses)
	}
}
