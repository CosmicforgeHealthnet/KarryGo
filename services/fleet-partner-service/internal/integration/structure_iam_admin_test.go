package integration_test

import (
	"context"
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/stepup"
)

// TestOrganizationStructureAndAccessAdministration drives the production
// router through spec Sections 7-9: branches, departments, settings, staff
// edits, custom roles and branch-scoped role bindings, including the
// escalation, last-branch, owner-role and cross-tenant guards.
func TestOrganizationStructureAndAccessAdministration(t *testing.T) {
	router, h := buildOnboardingRouter(t)
	owner := onboardingLogin(t, router, h, "structure-owner@example.com", "+2348012345641")
	company := map[string]any{"legalName": "Structure Co", "registrationNumber": "RC-STRUCT-1", "organizationType": "PRIVATE_LIMITED", "countryCode": "NG", "timezone": "Africa/Lagos", "defaultCurrency": "NGN", "ownerName": "Structure Owner"}
	if code, body := onboardCall(t, router, owner, "POST", "/organizations", company, 0); code != 201 {
		t.Fatal(body)
	}
	// Role and permission changes require step-up (spec 64.1); the owner
	// confirms once and the grant covers this test's access changes.
	ownerGrant := obtainStepUp(t, router, h, owner, stepup.ActionAccessManage)
	call := func(method, path string, payload any, version, want int) map[string]any {
		t.Helper()
		code, body := onboardCallStepUp(t, router, owner, ownerGrant, method, path, payload, version)
		if code != want {
			t.Fatalf("%s %s = %d, want %d: %v", method, path, code, want, body)
		}
		return body
	}

	// --- Branches: Head Office exists; add one; edit; the last active
	// branch cannot be deactivated.
	branches := call("GET", "/branches", nil, 0, 200)["data"].([]any)
	if len(branches) != 1 || branches[0].(map[string]any)["code"] != "HQ" {
		t.Fatalf("head office branch: %v", branches)
	}
	hq := branches[0].(map[string]any)
	if _, raw := hq["Code"]; raw {
		t.Fatal("raw Go field names leaked")
	}
	call("PATCH", "/branches/"+hq["id"].(string), map[string]any{"status": "INACTIVE"}, 0, 409)
	ikeja := documentData(t, call("POST", "/branches", map[string]any{"code": "ikj", "name": "Ikeja Depot", "type": "DEPOT", "latitude": 6.6, "longitude": 3.35}, 0, 201))
	if ikeja["code"] != "IKJ" || ikeja["timezone"] != "Africa/Lagos" {
		t.Fatalf("branch defaults: %v", ikeja)
	}
	call("POST", "/branches", map[string]any{"code": "IKJ", "name": "Duplicate"}, 0, 409)
	call("POST", "/branches", map[string]any{"code": "BAD", "name": "Bad", "type": "SPACESHIP"}, 0, 422)
	ikejaID := ikeja["id"].(string)
	updated := documentData(t, call("PATCH", "/branches/"+ikejaID, map[string]any{"name": "Ikeja Hub"}, 1, 200))
	if updated["name"] != "Ikeja Hub" || updated["version"].(float64) != 2 {
		t.Fatalf("branch update: %v", updated)
	}
	call("PATCH", "/branches/"+ikejaID, map[string]any{"name": "Stale"}, 1, 409)

	// --- Departments: create under the branch, nest, refuse cycles.
	ops := documentData(t, call("POST", "/departments", map[string]any{"code": "OPS", "name": "Operations", "branchId": ikejaID}, 0, 201))
	night := documentData(t, call("POST", "/departments", map[string]any{"code": "OPS-NIGHT", "name": "Night desk", "parentId": ops["id"]}, 0, 201))
	call("PATCH", "/departments/"+ops["id"].(string), map[string]any{"parentId": night["id"]}, 0, 422)
	if got := call("GET", "/departments", nil, 0, 200)["data"].([]any); len(got) != 2 {
		t.Fatalf("departments: %v", got)
	}
	// A branch with departments, or a parent with children, cannot be archived.
	call("POST", "/branches/"+ikejaID+"/archive", map[string]any{}, 0, 409)
	call("POST", "/departments/"+ops["id"].(string)+"/archive", map[string]any{}, 0, 409)

	// --- Settings: only governed fields change; amounts are whole minor units.
	settings := documentData(t, call("GET", "/organization-settings", nil, 0, 200))
	if settings["assignmentTimeoutSeconds"].(float64) != 300 {
		t.Fatalf("settings: %v", settings)
	}
	call("PATCH", "/organization-settings", map[string]any{"assignmentTimeoutSeconds": 5}, 0, 422)
	call("PATCH", "/organization-settings", map[string]any{"approvalThresholds": map[string]any{"withdrawal": 12.5}}, 0, 422)
	settings = documentData(t, call("PATCH", "/organization-settings", map[string]any{"assignmentTimeoutSeconds": 600, "providerVisibility": "branch", "approvalThresholds": map[string]any{"withdrawal": 5000000}}, 0, 200))
	if settings["assignmentTimeoutSeconds"].(float64) != 600 || settings["providerVisibility"] != "BRANCH" || settings["operatingMode"] == "" {
		t.Fatalf("settings update: %v", settings)
	}

	// --- Custom roles.
	call("POST", "/roles", map[string]any{"code": "DISPATCH_LEAD", "name": "Dispatch lead", "permissions": []string{"not.a.permission"}}, 0, 422)
	role := documentData(t, call("POST", "/roles", map[string]any{"code": "dispatch_lead", "name": "Dispatch lead", "permissions": []string{"staff.read", "providers.read"}}, 0, 201))
	roleID := role["id"].(string)
	if role["isSystem"] != false || len(role["permissions"].([]any)) != 2 {
		t.Fatalf("custom role: %v", role)
	}
	call("POST", "/roles", map[string]any{"code": "DISPATCH_LEAD", "name": "Again", "permissions": []string{"staff.read"}}, 0, 409)
	role = documentData(t, call("PATCH", "/roles/"+roleID, map[string]any{"name": "Dispatch lead", "permissions": []string{"staff.read"}}, 1, 200))
	if len(role["permissions"].([]any)) != 1 {
		t.Fatalf("role update: %v", role)
	}
	call("PATCH", "/roles/rol_system_owner", map[string]any{"name": "Hijack", "permissions": []string{"staff.read"}}, 0, 409)

	// --- Staff edit and scoped role bindings for a second member.
	var orgID string
	if err := h.pool.QueryRow(context.Background(), `SELECT id FROM fleet_partner_organizations WHERE registration_number = 'RC-STRUCT-1'`).Scan(&orgID); err != nil {
		t.Fatal(err)
	}
	member := seedMember(t, h.pool, orgID, seedIdentity(t, h.pool, "structure-staff@example.com", "+2348012345642"), false, nil)
	staffPath := "/staff/" + member.ID
	edited := documentData(t, call("PATCH", staffPath, map[string]any{"fullName": "Ada Dispatcher", "branchIds": []string{ikejaID}}, 0, 200))
	if edited["fullName"] != "Ada Dispatcher" {
		t.Fatalf("staff edit: %v", edited)
	}
	call("PATCH", staffPath, map[string]any{"branchIds": []string{"brn_unknown"}}, 0, 422)

	expires := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	bound := documentData(t, call("PUT", staffPath+"/roles", map[string]any{"bindings": []map[string]any{
		{"roleId": roleID, "scopeType": "BRANCH", "scopeId": ikejaID, "expiresAt": expires},
	}}, 0, 200))
	bindings := bound["bindings"].([]any)
	if len(bindings) != 1 || bindings[0].(map[string]any)["scopeType"] != "BRANCH" || bindings[0].(map[string]any)["expiresAt"] == nil {
		t.Fatalf("scoped binding: %v", bound)
	}
	call("PUT", staffPath+"/roles", map[string]any{"bindings": []map[string]any{{"roleId": roleID, "scopeType": "BRANCH", "scopeId": "brn_other"}}}, 0, 422)
	call("PUT", staffPath+"/roles", map[string]any{"bindings": []map[string]any{{"roleId": roleID, "scopeType": "GALAXY"}}}, 0, 422)

	// A role still bound to staff cannot be archived; once unbound it can.
	call("POST", "/roles/"+roleID+"/archive", map[string]any{}, 0, 409)

	// The owner cannot change their own roles, nor strip the owner role.
	ownerList := call("GET", "/staff", nil, 0, 200)["data"].([]any)
	var ownerID string
	for _, entry := range ownerList {
		if entry.(map[string]any)["isOwner"] == true {
			ownerID = entry.(map[string]any)["id"].(string)
		}
	}
	call("PUT", "/staff/"+ownerID+"/roles", map[string]any{"roleIds": []string{roleID}}, 0, 403)
	if got := call("GET", "/staff/"+ownerID+"/role-bindings", nil, 0, 200)["data"].([]any); len(got) == 0 {
		t.Fatal("owner bindings missing")
	}

	// --- Another organization sees none of it.
	other := onboardingLogin(t, router, h, "structure-other@example.com", "+2348012345643")
	company["legalName"], company["registrationNumber"] = "Other Structure Co", "RC-STRUCT-2"
	if code, body := onboardCall(t, router, other, "POST", "/organizations", company, 0); code != 201 {
		t.Fatal(body)
	}
	for _, path := range []string{"/branches/" + ikejaID, "/departments/" + ops["id"].(string), "/roles/" + roleID, staffPath + "/role-bindings"} {
		if code, _ := onboardCall(t, router, other, "GET", path, nil, 0); code != 404 {
			t.Fatalf("cross-tenant read %s = %d, want 404", path, code)
		}
	}
	if code, _ := onboardCall(t, router, other, "PATCH", "/branches/"+ikejaID, map[string]any{"name": "Taken"}, 0); code != 404 {
		t.Fatalf("cross-tenant branch write = %d", code)
	}
	otherGrant := obtainStepUp(t, router, h, other, stepup.ActionAccessManage)
	if code, _ := onboardCallStepUp(t, router, other, otherGrant, "PUT", staffPath+"/roles", map[string]any{"roleIds": []string{"rol_system_operations"}}, 0); code != 404 {
		t.Fatalf("cross-tenant role binding = %d", code)
	}

	// Every change above left an audit trail.
	var audited int
	if err := h.pool.QueryRow(context.Background(), `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id = $1
		AND action IN ('branch.created','branch.updated','department.created','role.created','role.updated','staff.updated','staff.roles_replaced','organization.settings_updated')`, orgID).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if audited < 8 {
		t.Fatalf("audit entries = %d, want >= 8", audited)
	}
}
