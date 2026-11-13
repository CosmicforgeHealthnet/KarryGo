package integration_test

import (
	"strings"
	"testing"
)

// TestStaffDirectoryAndInvitations drives the production router: the staff
// directory returns camelCase records with verified contacts and current
// roles, and pending invitations can be listed, resent and cancelled without
// ever exposing invitation token material.
func TestStaffDirectoryAndInvitations(t *testing.T) {
	router, h := buildOnboardingRouter(t)
	owner := onboardingLogin(t, router, h, "staff-owner@example.com", "+2348012345631")
	company := map[string]any{"legalName": "Staff Directory Co", "registrationNumber": "RC-STAFF-DIR", "organizationType": "PRIVATE_LIMITED", "countryCode": "NG", "timezone": "Africa/Lagos", "defaultCurrency": "NGN", "ownerName": "Staff Owner"}
	if code, body := onboardCall(t, router, owner, "POST", "/organizations", company, 0); code != 201 {
		t.Fatal(body)
	}

	code, body := onboardCall(t, router, owner, "GET", "/staff", nil, 0)
	if code != 200 {
		t.Fatal(body)
	}
	members := body["data"].([]any)
	if len(members) != 1 {
		t.Fatalf("expected the owner only: %v", members)
	}
	me := members[0].(map[string]any)
	if me["fullName"] != "Staff Owner" || me["email"] != "staff-owner@example.com" || me["isOwner"] != true || me["status"] != "ACTIVE" {
		t.Fatalf("owner entry: %v", me)
	}
	if _, raw := me["FullName"]; raw {
		t.Fatal("raw Go field names leaked")
	}
	roles := me["roles"].([]any)
	if len(roles) == 0 || roles[0].(map[string]any)["name"] == "" {
		t.Fatalf("owner roles missing: %v", me["roles"])
	}
	if code, body := onboardCall(t, router, owner, "GET", "/staff/"+me["id"].(string), nil, 0); code != 200 || documentData(t, body)["email"] != "staff-owner@example.com" {
		t.Fatalf("staff detail %d %v", code, body)
	}
	if code, _ := onboardCall(t, router, owner, "GET", "/staff/oru_missing", nil, 0); code != 404 {
		t.Fatalf("unknown staff member: %d", code)
	}

	code, body = onboardCall(t, router, owner, "GET", "/roles", nil, 0)
	if code != 200 {
		t.Fatal(body)
	}
	var operationsRole string
	for _, r := range body["data"].([]any) {
		role := r.(map[string]any)
		if role["id"] == nil || role["name"] == nil {
			t.Fatalf("role shape: %v", role)
		}
		if role["id"] == "rol_system_operations" {
			operationsRole = role["id"].(string)
		}
	}
	if operationsRole == "" {
		t.Fatal("system operations role not listed")
	}
	if code, body := onboardCall(t, router, owner, "GET", "/permissions", nil, 0); code != 200 || body["data"].([]any)[0].(map[string]any)["code"] == nil {
		t.Fatalf("permissions shape %d %v", code, body)
	}

	invite := map[string]any{"fullName": "New Dispatcher", "email": "dispatcher@example.com", "phone": "+2348012345632", "roleIds": []string{operationsRole}}
	if code, body := onboardCall(t, router, owner, "POST", "/staff/invitations", invite, 0); code != 201 {
		t.Fatalf("invite %d %v", code, body)
	}
	code, body = onboardCall(t, router, owner, "GET", "/staff/invitations", nil, 0)
	if code != 200 {
		t.Fatal(body)
	}
	invitations := body["data"].([]any)
	if len(invitations) != 1 {
		t.Fatalf("invitations: %v", invitations)
	}
	pending := invitations[0].(map[string]any)
	if pending["email"] != "dispatcher@example.com" || pending["status"] != "PENDING" || pending["roleIds"].([]any)[0] != operationsRole {
		t.Fatalf("invitation view: %v", pending)
	}
	for key := range pending {
		if strings.Contains(strings.ToLower(key), "token") {
			t.Fatalf("invitation exposes %s", key)
		}
	}
	id := pending["id"].(string)
	if code, body := onboardCall(t, router, owner, "POST", "/staff/invitations/"+id+"/resend", map[string]any{}, 0); code != 200 {
		t.Fatalf("resend %d %v", code, body)
	}
	if code, body := onboardCall(t, router, owner, "DELETE", "/staff/invitations/"+id, nil, 0); code != 200 {
		t.Fatalf("cancel %d %v", code, body)
	}
	_, body = onboardCall(t, router, owner, "GET", "/staff/invitations", nil, 0)
	if body["data"].([]any)[0].(map[string]any)["status"] != "CANCELLED" {
		t.Fatalf("cancelled invitation still pending: %v", body)
	}
	if code, _ := onboardCall(t, router, owner, "POST", "/staff/invitations/"+id+"/resend", map[string]any{}, 0); code != 409 {
		t.Fatalf("resending a cancelled invitation: %d", code)
	}

	// Another company cannot see this company's staff or invitations.
	other := onboardingLogin(t, router, h, "staff-other@example.com", "+2348012345633")
	company["legalName"], company["registrationNumber"] = "Other Staff Co", "RC-STAFF-OTHER"
	if code, body := onboardCall(t, router, other, "POST", "/organizations", company, 0); code != 201 {
		t.Fatal(body)
	}
	if code, _ := onboardCall(t, router, other, "GET", "/staff/"+me["id"].(string), nil, 0); code != 404 {
		t.Fatalf("cross-company staff read: %d", code)
	}
	_, body = onboardCall(t, router, other, "GET", "/staff/invitations", nil, 0)
	if len(body["data"].([]any)) != 0 {
		t.Fatal("cross-company invitations visible")
	}
}
