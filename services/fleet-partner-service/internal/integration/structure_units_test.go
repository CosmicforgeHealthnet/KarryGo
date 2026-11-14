package integration_test

import (
	"context"
	"testing"
)

// TestOrganizationStructureUnits drives the production router through spec
// 37.3 / 57.1 / 59.2 for cost centres, teams and operating locations
// (FLEET-8.15): tenant-matched references, the department cost-centre link,
// hierarchy guards on archive, If-Match conflicts, cross-tenant 404, audit and
// outbox. The company is still onboarding, so every write here also proves
// the route is on the pre-activation allowlist.
func TestOrganizationStructureUnits(t *testing.T) {
	router, h := buildOnboardingRouter(t)
	owner := onboardingLogin(t, router, h, "units-owner@example.com", "+2348012345661")
	company := map[string]any{"legalName": "Units Co", "registrationNumber": "RC-WS1-UNITS-1", "organizationType": "PRIVATE_LIMITED", "countryCode": "NG", "timezone": "Africa/Lagos", "defaultCurrency": "NGN", "ownerName": "Units Owner"}
	if code, body := onboardCall(t, router, owner, "POST", "/organizations", company, 0); code != 201 {
		t.Fatal(body)
	}
	call := func(method, path string, payload any, version, want int) map[string]any {
		t.Helper()
		code, body := onboardCall(t, router, owner, method, path, payload, version)
		if code != want {
			t.Fatalf("%s %s = %d, want %d: %v", method, path, code, want, body)
		}
		return body
	}
	ctx := context.Background()
	var orgID string
	if err := h.pool.QueryRow(ctx, `SELECT id FROM fleet_partner_organizations WHERE registration_number = 'RC-WS1-UNITS-1'`).Scan(&orgID); err != nil {
		t.Fatal(err)
	}
	var hqID string
	if err := h.pool.QueryRow(ctx, `SELECT id FROM fleet_partner_branches WHERE organization_id = $1 AND code = 'HQ'`, orgID).Scan(&hqID); err != nil {
		t.Fatal(err)
	}
	depot := documentData(t, call("POST", "/branches", map[string]any{"code": "DEP", "name": "Apapa Depot", "type": "DEPOT"}, 0, 201))
	depotID := depot["id"].(string)
	staff := seedMember(t, h.pool, orgID, seedIdentity(t, h.pool, "units-staff@example.com", "+2348012345662"), false, nil)

	// --- Cost centres.
	cc := documentData(t, call("POST", "/cost-centres", map[string]any{"code": "cc-fleet", "name": "Fleet costs", "branchId": depotID, "budgetOwnerUserId": staff.ID}, 0, 201))
	ccID := cc["id"].(string)
	if cc["code"] != "CC-FLEET" || cc["status"] != "ACTIVE" || cc["budgetOwnerUserId"] != staff.ID {
		t.Fatalf("cost centre: %v", cc)
	}
	call("POST", "/cost-centres", map[string]any{"code": "CC-FLEET", "name": "Duplicate"}, 0, 409)
	call("POST", "/cost-centres", map[string]any{"code": "bad code!", "name": "Bad"}, 0, 422)
	call("POST", "/cost-centres", map[string]any{"code": "CC-X", "name": "Bad owner", "budgetOwnerUserId": "usr_missing"}, 0, 422)
	call("POST", "/cost-centres", map[string]any{"code": "CC-Y", "name": "Bad branch", "branchId": "brn_missing"}, 0, 422)
	cc = documentData(t, call("PATCH", "/cost-centres/"+ccID, map[string]any{"name": "Fleet operating costs"}, 1, 200))
	if cc["name"] != "Fleet operating costs" || cc["version"].(float64) != 2 {
		t.Fatalf("cost centre update: %v", cc)
	}
	call("PATCH", "/cost-centres/"+ccID, map[string]any{"name": "Stale"}, 1, 409)

	// --- Departments now reference the cost-centre register.
	call("POST", "/departments", map[string]any{"code": "OPS-X", "name": "Ops", "costCentreId": "cc_unknown"}, 0, 422)
	ops := documentData(t, call("POST", "/departments", map[string]any{"code": "OPS", "name": "Operations", "branchId": depotID, "costCentreId": ccID}, 0, 201))
	opsID := ops["id"].(string)
	if ops["costCentreId"] != ccID {
		t.Fatalf("department cost centre: %v", ops)
	}
	// A cost centre still used by an active department cannot be archived.
	call("POST", "/cost-centres/"+ccID+"/archive", map[string]any{}, 0, 409)

	// --- Teams: department and branch must agree; supervisor must be an
	// active member of this organization.
	call("POST", "/teams", map[string]any{"code": "T-BAD", "name": "Wrong branch", "departmentId": opsID, "branchId": hqID}, 0, 422)
	call("POST", "/teams", map[string]any{"code": "T-BAD2", "name": "Unknown supervisor", "supervisorUserId": "usr_missing"}, 0, 422)
	team := documentData(t, call("POST", "/teams", map[string]any{"code": "night-shift", "name": "Night shift", "departmentId": opsID, "branchId": depotID, "supervisorUserId": staff.ID}, 0, 201))
	teamID := team["id"].(string)
	if team["code"] != "NIGHT-SHIFT" || team["departmentId"] != opsID || team["supervisorUserId"] != staff.ID {
		t.Fatalf("team: %v", team)
	}
	team = documentData(t, call("PATCH", "/teams/"+teamID, map[string]any{"name": "Night crew"}, 1, 200))
	if team["name"] != "Night crew" || team["version"].(float64) != 2 {
		t.Fatalf("team update: %v", team)
	}
	call("PATCH", "/teams/"+teamID, map[string]any{"name": "Stale"}, 1, 409)
	// A department with an active team cannot be archived.
	call("POST", "/departments/"+opsID+"/archive", map[string]any{}, 0, 409)
	if got := call("GET", "/teams?departmentId="+opsID, nil, 0, 200)["data"].([]any); len(got) != 1 {
		t.Fatalf("teams: %v", got)
	}

	// --- Operating locations.
	call("POST", "/locations", map[string]any{"code": "Y1", "name": "Yard", "locationType": "SPACESHIP"}, 0, 422)
	call("POST", "/locations", map[string]any{"code": "Y1", "name": "Yard", "locationType": "YARD", "latitude": 6.45}, 0, 422)
	yard := documentData(t, call("POST", "/locations", map[string]any{"code": "yard-1", "name": "Truck yard", "locationType": "yard",
		"branchId": depotID, "latitude": 6.44, "longitude": 3.36, "address": map[string]any{"line1": "Wharf Road", "city": "Lagos"},
		"operatingHours": map[string]any{"mon": "06:00-22:00"}}, 0, 201))
	yardID := yard["id"].(string)
	if yard["locationType"] != "YARD" || yard["address"].(map[string]any)["city"] != "Lagos" {
		t.Fatalf("location: %v", yard)
	}
	call("POST", "/locations", map[string]any{"code": "YARD-1", "name": "Duplicate", "locationType": "YARD"}, 0, 409)
	yard = documentData(t, call("PATCH", "/locations/"+yardID, map[string]any{"status": "inactive"}, 1, 200))
	if yard["status"] != "INACTIVE" || yard["version"].(float64) != 2 {
		t.Fatalf("location update: %v", yard)
	}
	call("PATCH", "/locations/"+yardID, map[string]any{"status": "ARCHIVED"}, 2, 422)
	if got := call("GET", "/locations?locationType=YARD", nil, 0, 200)["data"].([]any); len(got) != 1 {
		t.Fatalf("locations: %v", got)
	}

	// --- A branch carrying live units cannot be archived out from under them.
	call("POST", "/branches/"+depotID+"/archive", map[string]any{}, 0, 409)

	// --- Archive in dependency order.
	call("POST", "/teams/"+teamID+"/archive", map[string]any{}, 2, 200)
	call("POST", "/departments/"+opsID+"/archive", map[string]any{}, 0, 200)
	call("POST", "/cost-centres/"+ccID+"/archive", map[string]any{}, 2, 200)
	call("POST", "/locations/"+yardID+"/archive", map[string]any{}, 2, 200)
	call("PATCH", "/teams/"+teamID, map[string]any{"name": "Revived"}, 3, 409)
	// An archived cost centre cannot be newly assigned.
	call("POST", "/departments", map[string]any{"code": "OPS-2", "name": "Ops 2", "costCentreId": ccID}, 0, 422)
	// Its code is free again once archived.
	documentData(t, call("POST", "/cost-centres", map[string]any{"code": "CC-FLEET", "name": "Fleet costs (new)"}, 0, 201))
	if got := call("GET", "/teams?includeArchived=true", nil, 0, 200)["data"].([]any); len(got) != 1 {
		t.Fatalf("archived teams: %v", got)
	}

	// --- Another organization sees and touches none of it.
	other := onboardingLogin(t, router, h, "units-other@example.com", "+2348012345663")
	company["legalName"], company["registrationNumber"] = "Units Other Co", "RC-WS1-UNITS-2"
	if code, body := onboardCall(t, router, other, "POST", "/organizations", company, 0); code != 201 {
		t.Fatal(body)
	}
	for _, path := range []string{"/cost-centres/" + ccID, "/teams/" + teamID, "/locations/" + yardID} {
		if code, _ := onboardCall(t, router, other, "GET", path, nil, 0); code != 404 {
			t.Fatalf("cross-tenant read %s = %d, want 404", path, code)
		}
		if code, _ := onboardCall(t, router, other, "PATCH", path, map[string]any{"name": "Taken"}, 0); code != 404 {
			t.Fatalf("cross-tenant write %s = %d, want 404", path, code)
		}
	}
	// Referencing another organization's units is refused.
	if code, _ := onboardCall(t, router, other, "POST", "/teams", map[string]any{"code": "T1", "name": "Borrowed", "branchId": depotID}, 0); code != 404 && code != 422 {
		t.Fatalf("cross-tenant branch reference = %d", code)
	}
	if code, _ := onboardCall(t, router, other, "POST", "/departments", map[string]any{"code": "D1", "name": "Borrowed", "costCentreId": ccID}, 0); code != 422 {
		t.Fatalf("cross-tenant cost centre reference = %d", code)
	}

	// --- Audit and outbox.
	var audits, events int
	if err := h.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id = $1
		AND action IN ('cost_centre.created','cost_centre.updated','cost_centre.archived','team.created','team.updated',
		'team.archived','location.created','location.updated','location.archived')`, orgID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err := h.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_outbox WHERE organization_id = $1
		AND event_type IN ('cost_centre.created.v1','team.created.v1','location.created.v1','cost_centre.archived.v1')`, orgID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if audits != 10 || events != 5 {
		t.Fatalf("audit=%d outbox=%d; want 10/5", audits, events)
	}
}
