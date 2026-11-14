package integration_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/assets"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/specialized"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// Section 20 requirement rows that previously had no PostgreSQL test: the
// capability list, courier record get/patch, persistence of every other
// specialized module through the real service with the module enabled,
// cold-chain temperature events, and record milestones and commands.

type rows20 struct {
	rowsOrgs
	svc     *specialized.Service
	router  http.Handler
	branchA string
	branchX string
}

var rows20AllModules = map[string]bool{
	specialized.Courier: true, specialized.Hauling: true, specialized.Heavy: true, specialized.Freight: true,
	specialized.ColdChain: true, specialized.Medical: true, specialized.Agricultural: true,
	specialized.Moving: true, specialized.Towing: true,
}

func newRows20(t *testing.T, modules map[string]bool) *rows20 {
	t.Helper()
	orgs := newRowsOrgs(t, "Rows 20", "")
	svc := specialized.NewService(specialized.Options{Pool: orgs.pool, Events: events.NewDomainPublisher(events.NewOutboxRepository(orgs.pool)),
		Flags: specialized.Flags{Enabled: true, Modules: modules}})
	f := &rows20{rowsOrgs: orgs, svc: svc}
	f.NoPermission = rowsActor{Org: orgs.A.Org, Member: seedApprovalMember(t, orgs.pool, orgs.A.Org, "", "no-role")}
	f.branchA = seedBranch(t, orgs.pool, orgs.A.Org, "Rows20 Main")
	f.branchX = seedBranch(t, orgs.pool, orgs.A.Org, "Rows20 Other")
	f.router = rowsRouter(t, orgs.pool, func(g *gin.RouterGroup, a *accesscontrol.Authorizer, s *idempotency.Store) {
		specialized.NewHandler(svc, a, s).RegisterTenantRoutes(g)
	})
	return f
}

func (f *rows20) enable(t *testing.T, actor rowsActor, code string) map[string]any {
	t.Helper()
	return rowsExpect(t, rowsDo(t, f.router, actor, "POST", "/fleet-partner/v1/specialized/capabilities/"+code+"/enable",
		map[string]any{"reason": "approved rollout", "configuration": map[string]any{}}, nil), 200, "enable "+code).Object(t)
}

func (f *rows20) create(t *testing.T, actor rowsActor, code string, body map[string]any) rowsResponse {
	t.Helper()
	if _, ok := body["SubjectType"]; !ok {
		body["SubjectType"] = "SHIPMENT"
	}
	if _, ok := body["SubjectReference"]; !ok {
		body["SubjectReference"] = "shp-" + ids.New("x")
	}
	return rowsDo(t, f.router, actor, "POST", "/fleet-partner/v1/specialized/"+code+"/records", body, nil)
}

func (f *rows20) recordPath(code, id string) string {
	return "/fleet-partner/v1/specialized/" + code + "/records/" + id
}

// TestRow20_1b_CapabilityList covers FLEET-20.1b: GET /specialized/capabilities.
func TestRow20_1b_CapabilityList(t *testing.T) {
	f := newRows20(t, map[string]bool{specialized.Courier: true, specialized.ColdChain: true, specialized.Moving: true})
	f.enable(t, f.A, specialized.ColdChain)
	courier := f.enable(t, f.A, specialized.Courier)
	f.enable(t, f.B, specialized.Moving)
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", "/fleet-partner/v1/specialized/capabilities/COURIER_LAST_MILE/disable",
		map[string]any{"expectedVersion": courier["Version"], "reason": "paused"}, nil), 200, "disable courier")

	items := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/specialized/capabilities", nil, nil), 200, "list").Items(t)
	if len(items) != 2 || items[0]["CapabilityCode"] != specialized.ColdChain || items[1]["CapabilityCode"] != specialized.Courier {
		t.Fatalf("capabilities ordered by code: %v", items)
	}
	if items[0]["Status"] != "ENABLED" || items[1]["Status"] != "DISABLED" || items[1]["DisabledAt"] == nil {
		t.Fatalf("capability states: %v", items)
	}
	if b := rowsExpect(t, rowsDo(t, f.router, f.B, "GET", "/fleet-partner/v1/specialized/capabilities", nil, nil), 200, "other org").Items(t); len(b) != 1 || b[0]["CapabilityCode"] != specialized.Moving {
		t.Fatalf("other organization's capabilities: %v", b)
	}
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", "/fleet-partner/v1/specialized/capabilities", nil, nil), 403, "no permission")

	// A module switched off for the deployment drops out of the list even
	// though the organization row still says ENABLED.
	narrowed := specialized.NewService(specialized.Options{Pool: f.pool, Flags: specialized.Flags{Enabled: true, Modules: map[string]bool{specialized.Courier: true}}})
	list, err := narrowed.ListCapabilities(context.Background(), f.tenantA())
	if err != nil || len(list) != 1 || list[0].CapabilityCode != specialized.Courier {
		t.Fatalf("deployment-disabled module listed: %+v %v", list, err)
	}
}

// TestRow20_2_CourierRecordGetPatch covers the untested part of FLEET-20.2:
// GET/PATCH /specialized/COURIER_LAST_MILE/records/:id and the list.
func TestRow20_2_CourierRecordGetPatch(t *testing.T) {
	f := newRows20(t, map[string]bool{specialized.Courier: true})
	f.enable(t, f.A, specialized.Courier)
	f.enable(t, f.B, specialized.Courier)
	created := rowsExpect(t, f.create(t, f.A, specialized.Courier, map[string]any{"BranchID": f.branchA, "RecordType": "DELIVERY_ATTEMPT",
		"Payload": map[string]any{"packageId": "pkg-1", "attemptNumber": 1, "outcome": "FAILED", "failureReasonCode": "CUSTOMER_ABSENT"}}), 201, "create").Object(t)
	id := created["ID"].(string)
	path := f.recordPath(specialized.Courier, id)
	got := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", path, nil, nil), 200, "get").Object(t)
	payload, _ := got["Payload"].(map[string]any)
	if got["RecordType"] != "DELIVERY_ATTEMPT" || got["Status"] != "DRAFT" || payload["failureReasonCode"] != "CUSTOMER_ABSENT" || got["SchemaID"] != "COURIER_LAST_MILE.v2" {
		t.Fatalf("courier record: %v", got)
	}
	rowsExpect(t, rowsDo(t, f.router, f.B, "GET", path, nil, nil), 404, "cross-tenant get")
	rowsExpect(t, rowsDo(t, f.router, rowsActor{Org: f.A.Org, Member: f.A.Member, Branch: f.branchX}, "GET", path, nil, nil), 404, "cross-branch get")
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", path, nil, nil), 403, "no permission")
	if list := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/specialized/COURIER_LAST_MILE/records", nil, nil), 200, "list").Items(t); len(list) != 1 || list[0]["ID"] != id {
		t.Fatalf("courier list: %v", list)
	}

	newPayload := map[string]any{"packageId": "pkg-1", "attemptNumber": 2, "outcome": "DELIVERED"}
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"Payload": newPayload}, nil), 400, "patch without version")
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"Version": 4, "Payload": newPayload}, nil), 409, "patch stale")
	rowsExpect(t, rowsDo(t, f.router, f.B, "PATCH", path, map[string]any{"Version": 1, "Payload": newPayload}, nil), 404, "cross-tenant patch")
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"Version": 1, "Payload": map[string]any{"packageId": "pkg-1", "attemptNumber": 0}}, nil), 400, "schema violation")
	// Financial values are external references only.
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"Version": 1, "Payload": map[string]any{"packageId": "pkg-1", "attemptNumber": 2, "amountMinor": 5000}}, nil), 400, "finance field")
	patched := rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"Version": 1, "Payload": newPayload, "ExternalFinancialReference": "inv-77"}, nil), 200, "patch").Object(t)
	p2, _ := patched["Payload"].(map[string]any)
	if patched["Version"] != float64(2) || p2["outcome"] != "DELIVERED" || patched["ExternalFinancialReference"] != "inv-77" || patched["RecordType"] != "DELIVERY_ATTEMPT" {
		t.Fatalf("patched courier record: %v", patched)
	}
	if rowsOutboxCount(t, f.pool, id, "specialized_record.updated.v1") != 1 {
		t.Fatal("specialized_record.updated event missing")
	}
}

// TestRow20_4_To_20_11_SpecializedModulesPersistAndIsolate covers FLEET-20.4
// (trucking/haulage), 20.5 (heavy), 20.6 (freight), 20.7 (cold-chain
// records), 20.8 (medical/sensitive), 20.9 (agricultural/food), 20.10
// (moving/relocation) and 20.11 (towing/recovery): with the module enabled
// each capability persists its records through the real service, reads them
// back, rejects an invalid payload and keeps them inside the tenant.
func TestRow20_4_To_20_11_SpecializedModulesPersistAndIsolate(t *testing.T) {
	f := newRows20(t, rows20AllModules)
	ctx := context.Background()
	qualification := ids.New("qlf")
	if _, err := f.pool.Exec(ctx, `INSERT INTO fleet_partner_qualifications(id,organization_id,code,name)VALUES($1,$2,'MED_HANDLING','Medical handling')`, qualification, f.A.Org); err != nil {
		t.Fatal(err)
	}
	assetService := assets.NewService(assets.Options{Pool: f.pool, Flags: assets.Flags{Assets: true}})
	towTruck, err := assetService.CreateAsset(ctx, f.tenantA(), assets.Asset{BranchID: f.branchA, AssetCode: "TOW-1", AssetType: "TOW_TRUCK", OwnershipType: "OWNED"})
	if err != nil {
		t.Fatal(err)
	}
	foreignBranch := seedBranch(t, f.pool, f.B.Org, "Rows20 B")
	foreignTow, err := assetService.CreateAsset(ctx, f.tenantB(), assets.Asset{BranchID: foreignBranch, AssetCode: "TOW-B", AssetType: "TOW_TRUCK", OwnershipType: "OWNED"})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		row, code, recordType string
		payload, invalid      map[string]any
		check                 string
		restricted            bool
	}{
		{"20.4", specialized.Hauling, "HAULAGE_PROFILE", map[string]any{"loadType": "FTL", "haulingAuthorityReference": "HA-1", "palletCount": 24}, map[string]any{"loadType": "BULK", "haulingAuthorityReference": "HA-1"}, "haulingAuthorityReference", false},
		{"20.4", specialized.Hauling, "DETENTION", map[string]any{"startedAt": "2026-11-01T08:00:00Z", "detentionType": "DEMURRAGE"}, map[string]any{"detentionType": "DEMURRAGE"}, "detentionType", false},
		{"20.5", specialized.Heavy, "HEAVY_PROFILE", map[string]any{"dimensions": map[string]any{"length": 18.5, "width": 3.2, "height": 4.1, "unit": "M"}, "weight": 62000, "axleRequirements": "7-axle", "trailerRequirements": "lowbed", "permitReference": "PRM-9", "routeSurveyReference": "RS-3"}, map[string]any{"dimensions": map[string]any{"length": -1, "width": 3, "height": 4}, "weight": 1, "axleRequirements": "a", "trailerRequirements": "t", "permitReference": "p", "routeSurveyReference": "r"}, "permitReference", false},
		{"20.6", specialized.Freight, "COUNTRY_LEG", map[string]any{"originCountry": "NG", "destinationCountry": "GH", "mode": "ROAD", "sequence": 1}, map[string]any{"originCountry": "Nigeria", "destinationCountry": "GH"}, "destinationCountry", false},
		{"20.7", specialized.ColdChain, "TEMPERATURE_REQUIREMENT", map[string]any{"minimumTemperature": 2, "maximumTemperature": 8, "unit": "C"}, map[string]any{"minimumTemperature": 9, "maximumTemperature": 2, "unit": "C"}, "unit", false},
		{"20.8", specialized.Medical, "MEDICAL_HANDLING", map[string]any{"requiredQualificationIds": []string{qualification}, "handlingInstructions": "Keep upright"}, map[string]any{"requiredQualificationIds": []string{"qlf_unknown"}, "handlingInstructions": "x"}, "handlingInstructions", true},
		{"20.9", specialized.Agricultural, "AGRICULTURAL_PROFILE", map[string]any{"batchReference": "B-1", "lotReference": "L-1", "perishability": "HIGH"}, map[string]any{"batchReference": "B-1", "lotReference": "L-1", "perishability": "EXTREME"}, "lotReference", false},
		{"20.10", specialized.Moving, "MOVING_INVENTORY", map[string]any{"items": []map[string]any{{"itemReference": "sofa-1", "room": "lounge", "quantity": 1}}}, map[string]any{"items": []map[string]any{}}, "items", false},
		{"20.11", specialized.Towing, "TOWING_RECOVERY", map[string]any{"breakdownReference": "brk-1", "disabledVehicleReference": "veh-1", "incidentLocation": "Third Mainland Bridge", "towingAssetReference": towTruck.ID, "destination": "Ikeja depot"}, map[string]any{"breakdownReference": "brk-1", "disabledVehicleReference": "veh-1", "incidentLocation": "x", "towingAssetReference": foreignTow.ID, "destination": "y"}, "destination", false},
	}
	enabled := map[string]bool{}
	for _, tc := range cases {
		if !enabled[tc.code] {
			f.enable(t, f.A, tc.code)
			f.enable(t, f.B, tc.code)
			enabled[tc.code] = true
		}
		// Invalid payloads (schema, cross-field rule or a reference outside
		// the organization) are refused and persist nothing.
		before := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_specialized_records WHERE organization_id=$1 AND capability_code=$2`, f.A.Org, tc.code)
		rowsExpect(t, f.create(t, f.A, tc.code, map[string]any{"BranchID": f.branchA, "RecordType": tc.recordType, "Payload": tc.invalid}), 400, tc.row+" invalid "+tc.recordType)
		if after := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_specialized_records WHERE organization_id=$1 AND capability_code=$2`, f.A.Org, tc.code); after != before {
			t.Fatalf("%s: rejected record was persisted", tc.row)
		}
		rowsExpect(t, f.create(t, f.A, tc.code, map[string]any{"BranchID": f.branchA, "RecordType": "NOT_A_TYPE", "Payload": tc.payload}), 400, tc.row+" unknown record type")

		created := rowsExpect(t, f.create(t, f.A, tc.code, map[string]any{"BranchID": f.branchA, "RecordType": tc.recordType, "Payload": tc.payload}), 201, tc.row+" create "+tc.recordType).Object(t)
		id := created["ID"].(string)
		if created["CapabilityCode"] != tc.code || created["SchemaID"] != tc.code+".v2" || created["RestrictedVisibility"] != tc.restricted {
			t.Fatalf("%s created: %v", tc.row, created)
		}
		var org, capability, recordType string
		if err := f.pool.QueryRow(ctx, `SELECT organization_id,capability_code,record_type FROM fleet_partner_specialized_records WHERE id=$1`, id).Scan(&org, &capability, &recordType); err != nil || org != f.A.Org || capability != tc.code || recordType != tc.recordType {
			t.Fatalf("%s persisted row: %s %s %s %v", tc.row, org, capability, recordType, err)
		}
		got := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", f.recordPath(tc.code, id), nil, nil), 200, tc.row+" get").Object(t)
		payload, _ := got["Payload"].(map[string]any)
		if mustMarshal(t, payload[tc.check]) != mustMarshal(t, tc.payload[tc.check]) {
			t.Fatalf("%s read back %s=%v, want %v", tc.row, tc.check, payload[tc.check], tc.payload[tc.check])
		}
		list := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/specialized/"+tc.code+"/records", nil, nil), 200, tc.row+" list")
		if !rowsIDs(list.Items(t), "ID")[id] {
			t.Fatalf("%s: record missing from list", tc.row)
		}
		if tc.restricted {
			// Restricted payloads are redacted in lists and every full read
			// is audited.
			if strings.Contains(string(list.Raw), "Keep upright") {
				t.Fatalf("%s: restricted payload leaked in list", tc.row)
			}
			if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_sensitive_access_audit WHERE specialized_record_id=$1 AND actor_reference=$2`, id, f.A.Member); n != 1 {
				t.Fatalf("%s: sensitive access audit rows = %d", tc.row, n)
			}
		}
		// Tenant isolation: organization B has the same module enabled but
		// can neither read nor list A's record.
		rowsExpect(t, rowsDo(t, f.router, f.B, "GET", f.recordPath(tc.code, id), nil, nil), 404, tc.row+" cross-tenant get")
		if rowsIDs(rowsExpect(t, rowsDo(t, f.router, f.B, "GET", "/fleet-partner/v1/specialized/"+tc.code+"/records", nil, nil), 200, tc.row+" other org list").Items(t), "ID")[id] {
			t.Fatalf("%s: record visible to another organization", tc.row)
		}
		rowsExpect(t, rowsDo(t, f.router, rowsActor{Org: f.A.Org, Member: f.A.Member, Branch: f.branchX}, "GET", f.recordPath(tc.code, id), nil, nil), 404, tc.row+" cross-branch get")
		if rowsOutboxCount(t, f.pool, id, "specialized_record.created.v1") != 1 || rowsAuditCount(t, f.pool, f.A.Org, id) != 1 {
			t.Fatalf("%s: create must write one audit record and one event", tc.row)
		}
	}

	// A module the organization has not enabled is closed even when the
	// deployment has it on.
	orgC := createOrganization(t, f.pool, "Rows 20 C", "RC-"+ids.New("rows"))
	ownerC := rowsActor{Org: orgC.ID, Member: seedApprovalMember(t, f.pool, orgC.ID, "rol_system_owner", "owner-c")}
	rowsExpect(t, f.create(t, ownerC, specialized.Hauling, map[string]any{"RecordType": "BACKHAUL", "Payload": map[string]any{"originReference": "a", "destinationReference": "b"}}), 404, "capability not enabled")
}

// TestRow20_7_ColdChainTemperatureEvents covers the temperature-event part of
// FLEET-20.7: POST /specialized/COLD_CHAIN/records/:id/temperature-events.
func TestRow20_7_ColdChainTemperatureEvents(t *testing.T) {
	f := newRows20(t, map[string]bool{specialized.ColdChain: true})
	f.enable(t, f.A, specialized.ColdChain)
	f.enable(t, f.B, specialized.ColdChain)
	mk := func(recordType string, payload map[string]any) string {
		created := rowsExpect(t, f.create(t, f.A, specialized.ColdChain, map[string]any{"BranchID": f.branchA, "RecordType": recordType, "Payload": payload}), 201, "create "+recordType).Object(t)
		return created["ID"].(string)
	}
	vaccines := mk("TEMPERATURE_REQUIREMENT", map[string]any{"minimumTemperature": 2, "maximumTemperature": 8, "unit": "C"})
	produce := mk("TEMPERATURE_REQUIREMENT", map[string]any{"minimumTemperature": 0, "maximumTemperature": 4, "unit": "C"})
	sensorRecord := mk("SENSOR_REFERENCE", map[string]any{"sensorReference": "sensor-9"})
	events := func(id string) string { return f.recordPath(specialized.ColdChain, id) + "/temperature-events" }
	at := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)

	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", events(vaccines), map[string]any{"SensorReference": "s-1", "Temperature": 5, "Unit": "C", "SourceVersion": 1}, nil), 400, "missing source time")
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", events(vaccines), map[string]any{"SensorReference": "s-1", "Temperature": 41, "Unit": "F", "SourceVersion": 1, "SourceTime": at}, nil), 400, "unit mismatch")
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", events(sensorRecord), map[string]any{"SensorReference": "s-1", "Temperature": 5, "Unit": "C", "SourceVersion": 1, "SourceTime": at}, nil), 400, "not a requirement record")
	rowsExpect(t, rowsDo(t, f.router, f.B, "POST", events(vaccines), map[string]any{"SensorReference": "s-1", "Temperature": 5, "Unit": "C", "SourceVersion": 1, "SourceTime": at}, nil), 404, "cross-tenant")

	inRange := rowsExpect(t, rowsDo(t, f.router, f.A, "POST", events(vaccines), map[string]any{"SensorReference": "s-1", "Temperature": 5, "Unit": "C", "SourceVersion": 1, "SourceTime": at}, nil), 201, "in range").Object(t)
	if inRange["Excursion"] != false || inRange["RecordID"] != vaccines {
		t.Fatalf("in-range event: %v", inRange)
	}
	// A different value under the same source version is a conflict, not a
	// silent replay; the identical reading replays to the stored event.
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", events(vaccines), map[string]any{"SensorReference": "s-1", "Temperature": 30, "Unit": "C", "SourceVersion": 1, "SourceTime": at}, nil), 409, "changed reading")
	replay := rowsExpect(t, rowsDo(t, f.router, f.A, "POST", events(vaccines), map[string]any{"SensorReference": "s-1", "Temperature": 5, "Unit": "C", "SourceVersion": 1, "SourceTime": at}, nil), 201, "replay").Object(t)
	if replay["ID"] != inRange["ID"] || replay["Excursion"] != false {
		t.Fatalf("replay must return the stored event: %v", replay)
	}
	// One sensor may serve several records, so the same (sensor, version)
	// against another record is that record's own reading (migration 000096).
	// It must never hand back the first record's event.
	shared := rowsExpect(t, rowsDo(t, f.router, f.A, "POST", events(produce), map[string]any{"SensorReference": "s-1", "Temperature": 3, "Unit": "C", "SourceVersion": 1, "SourceTime": at}, nil), 201, "sensor version on another record").Object(t)
	if shared["RecordID"] != produce || shared["ID"] == inRange["ID"] {
		t.Fatalf("shared sensor reading must belong to its own record: %v", shared)
	}

	excursion := rowsExpect(t, rowsDo(t, f.router, f.A, "POST", events(vaccines), map[string]any{"SensorReference": "s-1", "Temperature": 11.5, "Unit": "C", "SourceVersion": 2, "SourceTime": at}, nil), 201, "excursion").Object(t)
	if excursion["Excursion"] != true {
		t.Fatalf("excursion event: %v", excursion)
	}
	quarantined := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", f.recordPath(specialized.ColdChain, vaccines), nil, nil), 200, "reread").Object(t)
	if quarantined["Status"] != "QUARANTINED" {
		t.Fatalf("excursion must quarantine the record: %v", quarantined)
	}
	if rowsOutboxCount(t, f.pool, vaccines, "cold_chain.excursion.v1") != 1 || rowsOutboxCount(t, f.pool, vaccines, "cold_chain.quarantined.v1") != 1 {
		t.Fatal("excursion/quarantine events missing")
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_temperature_events WHERE record_id=$1`, vaccines); n != 2 {
		t.Fatalf("temperature events stored = %d", n)
	}
	version := quarantined["Version"]
	// Quarantined goods cannot be completed without an authorized release.
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", f.recordPath(specialized.ColdChain, vaccines)+"/commands/complete", map[string]any{"expectedVersion": version}, nil), 409, "complete while quarantined")
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", f.recordPath(specialized.ColdChain, vaccines)+"/commands/release", map[string]any{"expectedVersion": version}, nil), 400, "release without evidence")
	withEvidence := rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", f.recordPath(specialized.ColdChain, vaccines), map[string]any{"Version": version,
		"Payload": map[string]any{"minimumTemperature": 2, "maximumTemperature": 8, "unit": "C", "releaseEvidenceReference": "lab-report-4", "authorizedBy": f.A.Member}}, nil), 200, "attach release evidence").Object(t)
	released := rowsExpect(t, rowsDo(t, f.router, f.A, "POST", f.recordPath(specialized.ColdChain, vaccines)+"/commands/release", map[string]any{"expectedVersion": withEvidence["Version"]}, nil), 200, "release").Object(t)
	if released["Status"] != "RELEASED" || rowsOutboxCount(t, f.pool, vaccines, "specialized_record.released.v1") != 1 {
		t.Fatalf("release: %v", released)
	}
}

// TestRow20_12_RecordMilestonesAndCommands covers FLEET-20.12:
// POST .../records/:id/milestones and .../records/:id/commands/:command.
func TestRow20_12_RecordMilestonesAndCommands(t *testing.T) {
	f := newRows20(t, map[string]bool{specialized.Courier: true, specialized.Moving: true})
	f.enable(t, f.A, specialized.Courier)
	f.enable(t, f.B, specialized.Courier)
	created := rowsExpect(t, f.create(t, f.A, specialized.Courier, map[string]any{"BranchID": f.branchA, "RecordType": "DELIVERY_WAVE",
		"Payload": map[string]any{"waveReference": "WAVE-1"}}), 201, "create").Object(t)
	id := created["ID"].(string)
	path := f.recordPath(specialized.Courier, id)

	milestones := path + "/milestones"
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", milestones, map[string]any{"Status": "DONE"}, nil), 400, "milestone without type")
	rowsExpect(t, rowsDo(t, f.router, f.B, "POST", milestones, map[string]any{"MilestoneType": "LOADED", "Status": "DONE"}, nil), 404, "cross-tenant milestone")
	rowsExpect(t, rowsDo(t, f.router, rowsActor{Org: f.A.Org, Member: f.A.Member, Branch: f.branchX}, "POST", milestones, map[string]any{"MilestoneType": "LOADED", "Status": "DONE"}, nil), 404, "cross-branch milestone")
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "POST", milestones, map[string]any{"MilestoneType": "LOADED", "Status": "DONE"}, nil), 403, "no permission")
	key := map[string]string{"Idempotency-Key": "wave-loaded-1"}
	first := rowsExpect(t, rowsDo(t, f.router, f.A, "POST", milestones, map[string]any{"MilestoneType": "LOADED", "Status": "DONE", "SafeMetadata": map[string]any{"bay": "3"}}, key), 201, "milestone").Object(t)
	if first["RecordID"] != id || first["ActorReference"] != f.A.Member || first["CorrelationID"] != "wave-loaded-1" {
		t.Fatalf("milestone: %v", first)
	}
	again := rowsExpect(t, rowsDo(t, f.router, f.A, "POST", milestones, map[string]any{"MilestoneType": "LOADED", "Status": "DONE", "SafeMetadata": map[string]any{"bay": "3"}}, key), 201, "milestone replay").Object(t)
	if again["ID"] != first["ID"] {
		t.Fatalf("milestone replay created a second row: %v", again)
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_specialized_milestones WHERE record_id=$1`, id); n != 1 {
		t.Fatalf("milestones stored = %d", n)
	}
	if rowsOutboxCount(t, f.pool, id, "specialized_milestone.recorded.v1") != 1 {
		t.Fatal("milestone event missing")
	}

	commands := path + "/commands/"
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", commands+"teleport", map[string]any{"expectedVersion": 1}, nil), 400, "unsupported command")
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", commands+"quarantine", map[string]any{"expectedVersion": 1}, nil), 400, "quarantine outside cold chain")
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", commands+"complete", map[string]any{}, nil), 400, "command without version")
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", commands+"complete", map[string]any{"expectedVersion": 3}, nil), 409, "command stale")
	rowsExpect(t, rowsDo(t, f.router, f.B, "POST", commands+"complete", map[string]any{"expectedVersion": 1}, nil), 404, "cross-tenant command")
	completed := rowsExpect(t, rowsDo(t, f.router, f.A, "POST", commands+"complete", map[string]any{"expectedVersion": 1}, nil), 200, "complete").Object(t)
	if completed["Status"] != "COMPLETED" || completed["CompletedAt"] == nil || completed["Version"] != float64(2) {
		t.Fatalf("completed: %v", completed)
	}
	// A completed record is terminal.
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", commands+"complete", map[string]any{"expectedVersion": 2}, nil), 409, "complete twice")
	if rowsOutboxCount(t, f.pool, id, "specialized_record.completed.v1") != 1 {
		t.Fatal("specialized_record.completed event missing")
	}
}
