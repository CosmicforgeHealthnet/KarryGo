package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/assets"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// Section 18 requirement rows that previously had no PostgreSQL test: asset
// list, archive/retire, asset documents, template/inspection lists,
// maintenance schedule reads, breakdowns, parts list and fuel decisions.

type rows18 struct {
	rowsOrgs
	svc     *assets.Service
	router  http.Handler
	branchA string
	branchX string
	branchB string
	now     time.Time
}

func newRows18(t *testing.T) *rows18 {
	t.Helper()
	orgs := newRowsOrgs(t, "Rows 18", "")
	now := time.Now().UTC().Truncate(time.Second)
	media := sectionMedia{files: map[string]platformclients.FileMetadata{
		"a-clean":      {FileID: "a-clean", OwnerID: orgs.A.Org, UploadState: "COMPLETED", ScanState: "CLEAN"},
		"a-quarantine": {FileID: "a-quarantine", OwnerID: orgs.A.Org, UploadState: "COMPLETED", ScanState: "QUARANTINED"},
		"b-clean":      {FileID: "b-clean", OwnerID: orgs.B.Org, UploadState: "COMPLETED", ScanState: "CLEAN"},
	}}
	svc := assets.NewService(assets.Options{
		Pool: orgs.pool, Events: events.NewDomainPublisher(events.NewOutboxRepository(orgs.pool)), Media: media,
		Flags: assets.Flags{Assets: true, Assignments: true, Inspections: true, Maintenance: true, Breakdowns: true, Inventory: true, Fuel: true},
		Now:   func() time.Time { return now },
	})
	f := &rows18{rowsOrgs: orgs, svc: svc, now: now}
	f.NoPermission = rowsActor{Org: orgs.A.Org, Member: seedApprovalMember(t, orgs.pool, orgs.A.Org, "", "no-role")}
	f.branchA = seedBranch(t, orgs.pool, orgs.A.Org, "Rows18 Main")
	f.branchX = seedBranch(t, orgs.pool, orgs.A.Org, "Rows18 Other")
	f.branchB = seedBranch(t, orgs.pool, orgs.B.Org, "Rows18 B")
	f.router = rowsRouter(t, orgs.pool, func(g *gin.RouterGroup, a *accesscontrol.Authorizer, s *idempotency.Store) {
		assets.NewHandler(svc, a, s).RegisterTenantRoutes(g)
	})
	return f
}

func (f *rows18) asset(t *testing.T, org, branch, code string) assets.Asset {
	t.Helper()
	scope := f.tenantA()
	if org == f.B.Org {
		scope = f.tenantB()
	}
	a, err := f.svc.CreateAsset(context.Background(), scope, assets.Asset{BranchID: branch, AssetCode: code, AssetType: "TRUCK", OwnershipType: "OWNED", Capacity: 1000, CapacityUnit: "KG"})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func (f *rows18) membership(t *testing.T, org string) string {
	t.Helper()
	id := ids.New("mem")
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO fleet_partner_organization_provider_memberships(id,organization_id,provider_service,service_category,provider_id,onboarding_source,status)VALUES($1,$2,'driver-hauling-service','HAULING',$3,'ORGANIZATION_SPONSORED','ACTIVE')`, id, org, "prv-"+id); err != nil {
		t.Fatal(err)
	}
	return id
}

// TestRow18_1b_AssetList covers FLEET-18.1b: GET /assets with branch and
// status filters, paging, tenant and branch isolation, and refusal of an
// invalid page window.
func TestRow18_1b_AssetList(t *testing.T) {
	f := newRows18(t)
	first := f.asset(t, f.A.Org, f.branchA, "AST-1")
	second := f.asset(t, f.A.Org, f.branchA, "AST-2")
	other := f.asset(t, f.A.Org, f.branchX, "AST-X")
	foreign := f.asset(t, f.B.Org, f.branchB, "AST-B")
	if _, err := f.svc.UpdateAsset(context.Background(), f.tenantA(), second.ID, second.Version, assets.Asset{Capacity: -1, Odometer: -1, OperatingHours: -1}, "ARCHIVE"); err != nil {
		t.Fatal(err)
	}

	items := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/assets", nil, nil), 200, "list").Items(t)
	got := rowsIDs(items, "ID")
	if len(items) != 3 || !got[first.ID] || !got[second.ID] || !got[other.ID] || got[foreign.ID] {
		t.Fatalf("asset list: %v", got)
	}
	if items[0]["ID"] != other.ID {
		t.Fatalf("assets must be newest first, got %v", items[0]["ID"])
	}
	archived := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/assets?status=archived", nil, nil), 200, "status").Items(t)
	if len(archived) != 1 || archived[0]["ID"] != second.ID {
		t.Fatalf("lifecycle status filter: %v", archived)
	}
	available := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/assets?status=AVAILABLE&branchId="+f.branchA, nil, nil), 200, "operational+branch").Items(t)
	if len(available) != 2 {
		t.Fatalf("operational status + branch filter: %v", rowsIDs(available, "ID"))
	}
	paged := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/assets?limit=1&offset=2", nil, nil), 200, "paged").Items(t)
	if len(paged) != 1 || paged[0]["ID"] != first.ID {
		t.Fatalf("paging: %v", rowsIDs(paged, "ID"))
	}
	confined := rowsActor{Org: f.A.Org, Member: f.A.Member, Branch: f.branchX}
	if c := rowsExpect(t, rowsDo(t, f.router, confined, "GET", "/fleet-partner/v1/assets", nil, nil), 200, "confined").Items(t); len(c) != 1 || c[0]["ID"] != other.ID {
		t.Fatalf("branch-confined list: %v", rowsIDs(c, "ID"))
	}
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", "/fleet-partner/v1/assets", nil, nil), 403, "no permission")
	// An invalid page window is a client error, never a server error.
	rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/assets?offset=-1", nil, nil), 400, "negative offset")
	rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/assets?limit=-5", nil, nil), 400, "negative limit")
}

// TestRow18_2_AssetArchiveRetire covers FLEET-18.2: POST /assets/:id/archive
// and /retire, with version, active-assignment and tenant refusals.
func TestRow18_2_AssetArchiveRetire(t *testing.T) {
	f := newRows18(t)
	ctx := context.Background()
	toArchive := f.asset(t, f.A.Org, f.branchA, "ARC-1")
	toRetire := f.asset(t, f.A.Org, f.branchA, "RET-1")
	busy := f.asset(t, f.A.Org, f.branchA, "BUSY-1")
	member := f.membership(t, f.A.Org)
	if _, err := f.svc.Assign(ctx, f.tenantA(), busy.ID, assets.Assignment{ProviderMembershipID: member, AssignmentType: "PRIMARY", StartAt: f.now}); err != nil {
		t.Fatal(err)
	}
	busy, _ = f.svc.GetAsset(ctx, f.tenantA(), busy.ID)

	archivePath := "/fleet-partner/v1/assets/" + toArchive.ID + "/archive"
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", archivePath, map[string]any{}, nil), 400, "archive without version")
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", archivePath, map[string]any{"expectedVersion": 9}, nil), 409, "archive stale version")
	rowsExpect(t, rowsDo(t, f.router, f.B, "POST", archivePath, map[string]any{"expectedVersion": 1}, nil), 404, "cross-tenant archive")
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "POST", archivePath, map[string]any{"expectedVersion": 1}, nil), 403, "archive without permission")
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", "/fleet-partner/v1/assets/"+busy.ID+"/archive", map[string]any{"expectedVersion": busy.Version}, nil), 409, "archive with active assignment")

	archived := rowsExpect(t, rowsDo(t, f.router, f.A, "POST", archivePath, map[string]any{"expectedVersion": 1}, nil), 200, "archive").Object(t)
	if archived["LifecycleStatus"] != "ARCHIVED" || archived["ArchivedAt"] == nil || archived["Version"] != float64(2) {
		t.Fatalf("archive result: %v", archived)
	}
	retired := rowsExpect(t, rowsDo(t, f.router, f.A, "POST", "/fleet-partner/v1/assets/"+toRetire.ID+"/retire", map[string]any{"expectedVersion": 1}, nil), 200, "retire").Object(t)
	if retired["LifecycleStatus"] != "RETIRED" || retired["RetiredAt"] == nil {
		t.Fatalf("retire result: %v", retired)
	}
	if rowsOutboxCount(t, f.pool, toArchive.ID, "asset.archived.v1") != 1 || rowsOutboxCount(t, f.pool, toRetire.ID, "asset.retired.v1") != 1 {
		t.Fatal("archive/retire must each publish one event")
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_asset_history WHERE asset_id=$1 AND new_lifecycle_status='ARCHIVED'`, toArchive.ID); n != 1 {
		t.Fatalf("archive history rows = %d", n)
	}
	// An archived or retired asset is not assignable.
	for _, id := range []string{toArchive.ID, toRetire.ID} {
		if _, err := f.svc.Assign(ctx, f.tenantA(), id, assets.Assignment{ProviderMembershipID: member, AssignmentType: "PRIMARY", StartAt: f.now}); crmErrorCode(err) != "conflict" {
			t.Fatalf("assign to %s: want conflict, got %v", id, err)
		}
	}
}

// TestRow18_4_AssetDocuments covers FLEET-18.4: GET/POST /assets/:id/documents.
func TestRow18_4_AssetDocuments(t *testing.T) {
	f := newRows18(t)
	asset := f.asset(t, f.A.Org, f.branchA, "DOC-1")
	path := "/fleet-partner/v1/assets/" + asset.ID + "/documents"
	expires := f.now.Add(365 * 24 * time.Hour)
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", path, map[string]any{"DocumentType": "ROADWORTHINESS"}, nil), 400, "missing media")
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", path, map[string]any{"MediaFileID": "b-clean", "DocumentType": "ROADWORTHINESS"}, nil), 403, "foreign media")
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", path, map[string]any{"MediaFileID": "a-quarantine", "DocumentType": "ROADWORTHINESS"}, nil), 403, "unsafe media")
	rowsExpect(t, rowsDo(t, f.router, f.B, "POST", path, map[string]any{"MediaFileID": "b-clean", "DocumentType": "ROADWORTHINESS"}, nil), 404, "cross-tenant add")
	doc := rowsExpect(t, rowsDo(t, f.router, f.A, "POST", path, map[string]any{"MediaFileID": "a-clean", "DocumentType": "ROADWORTHINESS", "ExpiresAt": expires}, nil), 201, "add").Object(t)
	if doc["VerificationStatus"] != "PENDING" || doc["AssetID"] != asset.ID {
		t.Fatalf("document: %v", doc)
	}
	docID := doc["ID"].(string)
	list := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", path, nil, nil), 200, "list").Items(t)
	if len(list) != 1 || list[0]["ID"] != docID || list[0]["MediaFileID"] != "a-clean" {
		t.Fatalf("documents: %v", list)
	}
	rowsExpect(t, rowsDo(t, f.router, f.B, "GET", path, nil, nil), 404, "cross-tenant list")
	rowsExpect(t, rowsDo(t, f.router, rowsActor{Org: f.A.Org, Member: f.A.Member, Branch: f.branchX}, "GET", path, nil, nil), 404, "cross-branch list")
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", path, nil, nil), 403, "no permission")
	if rowsAuditCount(t, f.pool, f.A.Org, docID) != 1 || rowsOutboxCount(t, f.pool, docID, "asset.document_added.v1") != 1 {
		t.Fatal("adding an asset document must write an audit record and an event")
	}
}

// TestRow18_5c_InspectionTemplateAndInspectionReads covers FLEET-18.5c:
// GET /inspection-templates, GET /inspection-templates/:id, GET /inspections.
func TestRow18_5c_InspectionTemplateAndInspectionReads(t *testing.T) {
	f := newRows18(t)
	ctx := context.Background()
	checklist := json.RawMessage(`[{"code":"lights","required":true}]`)
	rules := json.RawMessage(`{"criticalFailure":"BLOCK_ASSET"}`)
	tpl, err := f.svc.SaveTemplate(ctx, f.tenantA(), "", 0, assets.Template{Name: "Daily", AssetTypes: []string{"TRUCK"}, ChecklistItems: checklist, CriticalityRules: rules})
	if err != nil {
		t.Fatal(err)
	}
	foreignTpl, err := f.svc.SaveTemplate(ctx, f.tenantB(), "", 0, assets.Template{Name: "Foreign", ChecklistItems: checklist, CriticalityRules: rules})
	if err != nil {
		t.Fatal(err)
	}
	templates := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/inspection-templates", nil, nil), 200, "templates").Items(t)
	if len(templates) != 1 || templates[0]["ID"] != tpl.ID {
		t.Fatalf("templates: %v", templates)
	}
	got := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/inspection-templates/"+tpl.ID, nil, nil), 200, "get template").Object(t)
	if got["Name"] != "Daily" || got["Status"] != "ACTIVE" {
		t.Fatalf("template: %v", got)
	}
	rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/inspection-templates/"+foreignTpl.ID, nil, nil), 404, "cross-tenant template")
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", "/fleet-partner/v1/inspection-templates", nil, nil), 403, "no permission")

	inspector := f.membership(t, f.A.Org)
	main := f.asset(t, f.A.Org, f.branchA, "INS-1")
	elsewhere := f.asset(t, f.A.Org, f.branchX, "INS-X")
	open, err := f.svc.CreateInspection(ctx, f.tenantA(), assets.Inspection{AssetID: main.ID, TemplateID: tpl.ID, InspectorMembershipID: inspector})
	if err != nil {
		t.Fatal(err)
	}
	done, err := f.svc.CreateInspection(ctx, f.tenantA(), assets.Inspection{AssetID: elsewhere.ID, TemplateID: tpl.ID, InspectorMembershipID: inspector})
	if err != nil {
		t.Fatal(err)
	}
	if done, err = f.svc.CompleteInspection(ctx, f.tenantA(), done.ID, assets.CompleteInspectionInput{ExpectedVersion: done.Version, Answers: []assets.Answer{{ItemCode: "lights", Passed: true}}}); err != nil {
		t.Fatal(err)
	}
	all := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/inspections", nil, nil), 200, "inspections").Items(t)
	if ids := rowsIDs(all, "ID"); len(all) != 2 || !ids[open.ID] || !ids[done.ID] {
		t.Fatalf("inspections: %v", ids)
	}
	completed := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/inspections?status=completed", nil, nil), 200, "status filter").Items(t)
	if len(completed) != 1 || completed[0]["ID"] != done.ID || completed[0]["OverallResult"] != "PASS" {
		t.Fatalf("completed inspections: %v", completed)
	}
	confined := rowsActor{Org: f.A.Org, Member: f.A.Member, Branch: f.branchA}
	if c := rowsExpect(t, rowsDo(t, f.router, confined, "GET", "/fleet-partner/v1/inspections", nil, nil), 200, "confined").Items(t); len(c) != 1 || c[0]["ID"] != open.ID {
		t.Fatalf("branch-confined inspections: %v", rowsIDs(c, "ID"))
	}
	if b := rowsExpect(t, rowsDo(t, f.router, f.B, "GET", "/fleet-partner/v1/inspections", nil, nil), 200, "other org").Items(t); len(b) != 0 {
		t.Fatalf("other org inspections: %v", b)
	}
}

// TestRow18_6_MaintenanceScheduleReadsAndPatch covers the untested part of
// FLEET-18.6: GET /maintenance/schedules, GET /maintenance/schedules/:id and
// the PATCH version contract.
func TestRow18_6_MaintenanceScheduleReadsAndPatch(t *testing.T) {
	f := newRows18(t)
	ctx := context.Background()
	main := f.asset(t, f.A.Org, f.branchA, "MS-1")
	elsewhere := f.asset(t, f.A.Org, f.branchX, "MS-X")
	due := f.now.Add(30 * 24 * time.Hour)
	interval := 5000.0
	sched, err := f.svc.SaveSchedule(ctx, f.tenantA(), "", 0, assets.Schedule{AssetID: main.ID, TriggerType: "DATE", NextDueDate: &due, Critical: true})
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.svc.SaveSchedule(ctx, f.tenantA(), "", 0, assets.Schedule{AssetID: elsewhere.ID, TriggerType: "MILEAGE", IntervalValue: &interval, NextDueOdometer: &interval})
	if err != nil {
		t.Fatal(err)
	}
	foreignAsset := f.asset(t, f.B.Org, f.branchB, "MS-B")
	if _, err = f.svc.SaveSchedule(ctx, f.tenantB(), "", 0, assets.Schedule{AssetID: foreignAsset.ID, TriggerType: "DATE", NextDueDate: &due}); err != nil {
		t.Fatal(err)
	}
	list := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/maintenance/schedules", nil, nil), 200, "list").Items(t)
	if ids := rowsIDs(list, "ID"); len(list) != 2 || !ids[sched.ID] || !ids[other.ID] {
		t.Fatalf("schedules: %v", ids)
	}
	confined := rowsActor{Org: f.A.Org, Member: f.A.Member, Branch: f.branchA}
	if c := rowsExpect(t, rowsDo(t, f.router, confined, "GET", "/fleet-partner/v1/maintenance/schedules", nil, nil), 200, "confined").Items(t); len(c) != 1 || c[0]["ID"] != sched.ID {
		t.Fatalf("confined schedules: %v", rowsIDs(c, "ID"))
	}
	path := "/fleet-partner/v1/maintenance/schedules/" + sched.ID
	got := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", path, nil, nil), 200, "get").Object(t)
	if got["TriggerType"] != "DATE" || got["Critical"] != true || got["AssetID"] != main.ID {
		t.Fatalf("schedule: %v", got)
	}
	rowsExpect(t, rowsDo(t, f.router, f.B, "GET", path, nil, nil), 404, "cross-tenant get")
	rowsExpect(t, rowsDo(t, f.router, confined, "GET", "/fleet-partner/v1/maintenance/schedules/"+other.ID, nil, nil), 404, "cross-branch get")
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", path, nil, nil), 403, "no permission")

	later := f.now.Add(60 * 24 * time.Hour)
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"NextDueDate": later}, nil), 400, "patch without version")
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"Version": 7, "NextDueDate": later}, nil), 409, "patch stale version")
	patched := rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"Version": 1, "NextDueDate": later}, nil), 200, "patch").Object(t)
	if patched["Version"] != float64(2) || patched["Critical"] != true {
		t.Fatalf("patched schedule: %v", patched)
	}
	if rowsAuditCount(t, f.pool, f.A.Org, sched.ID) != 2 || rowsOutboxCount(t, f.pool, sched.ID, "maintenance_schedule.updated.v1") != 1 {
		t.Fatal("schedule create/update must be audited and evented")
	}
}

// TestRow18_8_BreakdownLifecycle covers FLEET-18.8: GET/POST /breakdowns,
// GET/PATCH /breakdowns/:id.
func TestRow18_8_BreakdownLifecycle(t *testing.T) {
	f := newRows18(t)
	ctx := context.Background()
	asset := f.asset(t, f.A.Org, f.branchA, "BRK-1")
	replacement := f.asset(t, f.A.Org, f.branchA, "BRK-REPL")
	foreignAsset := f.asset(t, f.B.Org, f.branchB, "BRK-B")
	member := f.membership(t, f.A.Org)

	rowsExpect(t, rowsDo(t, f.router, f.B, "POST", "/fleet-partner/v1/breakdowns", map[string]any{"AssetID": asset.ID, "ProviderMembershipID": member, "LocationReference": "loc", "Severity": "HIGH", "TripImpact": "DELAYED"}, nil), 404, "cross-tenant create")
	created := rowsExpect(t, rowsDo(t, f.router, f.A, "POST", "/fleet-partner/v1/breakdowns", map[string]any{"AssetID": asset.ID, "ProviderMembershipID": member, "TripReference": "trip-9", "LocationReference": "Ikeja", "Severity": "HIGH", "TripImpact": "DELAYED"}, nil), 201, "create").Object(t)
	id := created["ID"].(string)
	if created["Status"] != "OPEN" || created["RescueStatus"] != "REQUESTED" {
		t.Fatalf("breakdown: %v", created)
	}
	// A HIGH breakdown takes the asset out of service and opens one work order.
	if a, _ := f.svc.GetAsset(ctx, f.tenantA(), asset.ID); a.OperationalStatus != "OUT_OF_SERVICE" {
		t.Fatalf("asset after breakdown: %s", a.OperationalStatus)
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_work_orders WHERE source_type='BREAKDOWN' AND source_reference=$1`, id); n != 1 {
		t.Fatalf("breakdown work orders = %d", n)
	}
	if rowsOutboxCount(t, f.pool, id, "breakdown.created.v1") != 1 {
		t.Fatal("breakdown.created event missing")
	}

	list := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/breakdowns", nil, nil), 200, "list").Items(t)
	if len(list) != 1 || list[0]["ID"] != id {
		t.Fatalf("breakdowns: %v", list)
	}
	if b := rowsExpect(t, rowsDo(t, f.router, f.B, "GET", "/fleet-partner/v1/breakdowns", nil, nil), 200, "other org").Items(t); len(b) != 0 {
		t.Fatalf("other org breakdowns: %v", b)
	}
	path := "/fleet-partner/v1/breakdowns/" + id
	rowsExpect(t, rowsDo(t, f.router, f.A, "GET", path, nil, nil), 200, "get")
	rowsExpect(t, rowsDo(t, f.router, f.B, "GET", path, nil, nil), 404, "cross-tenant get")
	rowsExpect(t, rowsDo(t, f.router, rowsActor{Org: f.A.Org, Member: f.A.Member, Branch: f.branchX}, "GET", path, nil, nil), 404, "cross-branch get")
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", path, nil, nil), 403, "no permission")

	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"RescueStatus": "DISPATCHED"}, nil), 400, "patch without version")
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"Version": 5, "RescueStatus": "DISPATCHED"}, nil), 409, "patch stale version")
	// The replacement must be one of this organization's assets.
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"Version": 1, "ReplacementAssetID": foreignAsset.ID}, nil), 404, "foreign replacement asset")
	patched := rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"Version": 1, "RescueStatus": "DISPATCHED", "TowingReference": "TOW-1", "ReplacementAssetID": replacement.ID, "Status": "IN_PROGRESS"}, nil), 200, "patch").Object(t)
	if patched["Status"] != "IN_PROGRESS" || patched["ReplacementAssetID"] != replacement.ID || patched["TowingReference"] != "TOW-1" {
		t.Fatalf("patched breakdown: %v", patched)
	}
	resolved := rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"Version": 2, "Status": "RESOLVED"}, nil), 200, "resolve").Object(t)
	if resolved["Status"] != "RESOLVED" || rowsOutboxCount(t, f.pool, id, "breakdown.resolved.v1") != 1 {
		t.Fatalf("resolve: %v", resolved)
	}
}

// TestRow18_9_PartsList covers the untested part of FLEET-18.9:
// GET /inventory/parts, plus the PATCH version contract.
func TestRow18_9_PartsList(t *testing.T) {
	f := newRows18(t)
	ctx := context.Background()
	b, err := f.svc.SavePart(ctx, f.tenantA(), "", 0, assets.Part{BranchID: f.branchA, SKU: "SKU-B", Name: "Brake pad", StockUnit: "EA", ReorderLevel: 4})
	if err != nil {
		t.Fatal(err)
	}
	a, err := f.svc.SavePart(ctx, f.tenantA(), "", 0, assets.Part{BranchID: f.branchX, SKU: "SKU-A", Name: "Air filter", StockUnit: "EA", ReorderLevel: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.svc.SavePart(ctx, f.tenantB(), "", 0, assets.Part{BranchID: f.branchB, SKU: "SKU-A", Name: "Foreign", StockUnit: "EA"}); err != nil {
		t.Fatal(err)
	}
	list := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/inventory/parts", nil, nil), 200, "list").Items(t)
	if len(list) != 2 || list[0]["ID"] != a.ID || list[1]["ID"] != b.ID {
		t.Fatalf("parts ordered by SKU: %v", list)
	}
	confined := rowsActor{Org: f.A.Org, Member: f.A.Member, Branch: f.branchA}
	if c := rowsExpect(t, rowsDo(t, f.router, confined, "GET", "/fleet-partner/v1/inventory/parts", nil, nil), 200, "confined").Items(t); len(c) != 1 || c[0]["ID"] != b.ID {
		t.Fatalf("confined parts: %v", rowsIDs(c, "ID"))
	}
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", "/fleet-partner/v1/inventory/parts", nil, nil), 403, "no permission")
	path := "/fleet-partner/v1/inventory/parts/" + b.ID
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"Name": "x", "ReorderLevel": -1}, nil), 400, "patch without version")
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"Version": 9, "Name": "x", "ReorderLevel": -1}, nil), 409, "patch stale")
	rowsExpect(t, rowsDo(t, f.router, f.B, "PATCH", path, map[string]any{"Version": 1, "Name": "x", "ReorderLevel": -1}, nil), 404, "cross-tenant patch")
	patched := rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"Version": 1, "Name": "Brake pad (front)", "ReorderLevel": -1}, nil), 200, "patch").Object(t)
	if patched["Name"] != "Brake pad (front)" || patched["ReorderLevel"] != float64(4) {
		t.Fatalf("patched part: %v", patched)
	}
	if rowsAuditCount(t, f.pool, f.A.Org, b.ID) != 2 || rowsOutboxCount(t, f.pool, b.ID, "part.updated.v1") != 1 {
		t.Fatal("part create/update must be audited and evented")
	}
}

// TestRow18_10b_FuelApproveRejectList covers FLEET-18.10b:
// POST /fuel-records/:id/approve, /reject and GET /fuel-records.
func TestRow18_10b_FuelApproveRejectList(t *testing.T) {
	f := newRows18(t)
	ctx := context.Background()
	asset := f.asset(t, f.A.Org, f.branchA, "FUEL-1")
	elsewhere := f.asset(t, f.A.Org, f.branchX, "FUEL-X")
	member := f.membership(t, f.A.Org)
	mk := func(assetID string, at time.Time) assets.FuelRecord {
		r, err := f.svc.CreateFuel(ctx, f.tenantA(), assets.FuelRecord{AssetID: assetID, ProviderMembershipID: member, FuelOrEnergyType: "DIESEL", Quantity: 120.5, Unit: "L", RecordedAt: at})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	approveMe := mk(asset.ID, f.now.Add(-2*time.Hour))
	rejectMe := mk(asset.ID, f.now.Add(-time.Hour))
	other := mk(elsewhere.ID, f.now)

	list := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/fuel-records", nil, nil), 200, "list").Items(t)
	if len(list) != 3 || list[0]["ID"] != other.ID || list[2]["ID"] != approveMe.ID {
		t.Fatalf("fuel records newest first: %v", rowsIDs(list, "ID"))
	}
	confined := rowsActor{Org: f.A.Org, Member: f.A.Member, Branch: f.branchA}
	if c := rowsExpect(t, rowsDo(t, f.router, confined, "GET", "/fleet-partner/v1/fuel-records", nil, nil), 200, "confined").Items(t); len(c) != 2 {
		t.Fatalf("confined fuel records: %v", rowsIDs(c, "ID"))
	}
	if b := rowsExpect(t, rowsDo(t, f.router, f.B, "GET", "/fleet-partner/v1/fuel-records", nil, nil), 200, "other org").Items(t); len(b) != 0 {
		t.Fatalf("other org fuel: %v", b)
	}

	approvePath := "/fleet-partner/v1/fuel-records/" + approveMe.ID + "/approve"
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", approvePath, map[string]any{}, nil), 409, "approve without version")
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", approvePath, map[string]any{"expectedVersion": 4}, nil), 409, "approve stale")
	rowsExpect(t, rowsDo(t, f.router, f.B, "POST", approvePath, map[string]any{"expectedVersion": 1}, nil), 404, "cross-tenant approve")
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "POST", approvePath, map[string]any{"expectedVersion": 1}, nil), 403, "approve without permission")
	approved := rowsExpect(t, rowsDo(t, f.router, f.A, "POST", approvePath, map[string]any{"expectedVersion": 1}, nil), 200, "approve").Object(t)
	if approved["ApprovalStatus"] != "APPROVED" || approved["ApprovedBy"] != f.A.Member {
		t.Fatalf("approved: %v", approved)
	}
	// A decided record cannot be decided again.
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", "/fleet-partner/v1/fuel-records/"+approveMe.ID+"/reject", map[string]any{"expectedVersion": 2, "reason": "late"}, nil), 409, "reject after approve")
	rejected := rowsExpect(t, rowsDo(t, f.router, f.A, "POST", "/fleet-partner/v1/fuel-records/"+rejectMe.ID+"/reject", map[string]any{"expectedVersion": 1, "reason": "receipt mismatch"}, nil), 200, "reject").Object(t)
	if rejected["ApprovalStatus"] != "REJECTED" {
		t.Fatalf("rejected: %v", rejected)
	}
	if rowsOutboxCount(t, f.pool, approveMe.ID, "fuel_record.approved.v1") != 1 || rowsOutboxCount(t, f.pool, rejectMe.ID, "fuel_record.rejected.v1") != 1 {
		t.Fatal("fuel decision events missing")
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_fuel_approval_history WHERE fuel_record_id=$1 AND new_status='REJECTED' AND reason='receipt mismatch'`, rejectMe.ID); n != 1 {
		t.Fatalf("fuel approval history = %d", n)
	}
}
