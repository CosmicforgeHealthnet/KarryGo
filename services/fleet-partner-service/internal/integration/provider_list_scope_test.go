package integration_test

import (
	"fmt"
	"testing"

	providermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	staffmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
)

// TestProviderListPaginatesAndDetailRespectsBranchScope covers spec 6
// (cursor pagination, no silent truncation) and 7d/10.3 (object reads stay
// inside the caller's branch scope; unknown ids are 404, not empty 200).
func TestProviderListPaginatesAndDetailRespectsBranchScope(t *testing.T) {
	f := newWS5Fixture(t, "Provider list scope")
	member := seedMember(t, f.pool, f.org, seedIdentity(t, f.pool, "plist-"+randomHex()[:6]+"@example.com", "+23480"+randomHex()[:8]), true, nil)
	bindRole(t, f.pool, f.org, member.ID, staffmodels.SystemOwnerRoleID, staffmodels.ScopeOrganization, "")

	inScope := map[string]bool{}
	for i := 0; i < 5; i++ {
		inScope[f.membership(t, providermodels.StatusActive).ID] = true
	}
	otherBranch := seedBranch(t, f.pool, f.org, "Other "+randomHex()[:6])
	outside := f.membership(t, providermodels.StatusActive)
	if _, err := f.pool.Exec(t.Context(), `UPDATE fleet_partner_organization_provider_memberships SET home_branch_id = $2 WHERE id = $1`, outside.ID, otherBranch); err != nil {
		t.Fatal(err)
	}

	// A branch-limited caller pages through exactly its five providers.
	router := providerAdminRouter(t, f.pool, f.service, f.org, member.ID, []string{f.branch})
	seen := map[string]bool{}
	next := ""
	for page := 0; page < 5; page++ {
		path := "/fleet-partner/v1/providers?limit=2"
		if next != "" {
			path += "&cursor=" + next
		}
		code, body := ws5Call(t, router, "GET", path, nil)
		if code != 200 {
			t.Fatalf("list page %d = %d %v", page, code, body)
		}
		for _, item := range body["data"].([]any) {
			id := item.(map[string]any)["membershipId"].(string)
			if seen[id] {
				t.Fatalf("duplicate %s across pages", id)
			}
			seen[id] = true
		}
		meta, _ := body["meta"].(map[string]any)
		next, _ = meta["nextCursor"].(string)
		if next == "" {
			break
		}
	}
	if len(seen) != 5 || seen[outside.ID] {
		t.Fatalf("paged %d providers (outside branch seen=%v), want the 5 in scope", len(seen), seen[outside.ID])
	}
	for id := range inScope {
		if !seen[id] {
			t.Fatalf("missing %s", id)
		}
	}
	if code, _ := ws5Call(t, router, "GET", "/fleet-partner/v1/providers?limit=500", nil); code != 422 {
		t.Fatalf("limit 500 = %d", code)
	}
	if code, _ := ws5Call(t, router, "GET", "/fleet-partner/v1/providers?cursor=not-a-cursor", nil); code != 422 {
		t.Fatalf("bad cursor = %d", code)
	}

	// Out-of-branch provider: detail, history, provisioning, writes all 404.
	for _, call := range [][2]string{
		{"GET", "/fleet-partner/v1/providers/" + outside.ID},
		{"GET", "/fleet-partner/v1/providers/" + outside.ID + "/history"},
		{"GET", "/fleet-partner/v1/providers/" + outside.ID + "/provisioning-status"},
		{"GET", "/fleet-partner/v1/providers/" + outside.ID + "/capabilities"},
		{"POST", "/fleet-partner/v1/providers/" + outside.ID + "/suspend"},
	} {
		if code, body := ws5Call(t, router, call[0], call[1], nil); code != 404 {
			t.Fatalf("%s %s = %d %v, want 404", call[0], call[1], code, body)
		}
	}
	// Unknown id: 404 for detail and history (history used to be 200 []).
	for _, path := range []string{"/fleet-partner/v1/providers/opm_unknown", "/fleet-partner/v1/providers/opm_unknown/history"} {
		if code, _ := ws5Call(t, router, "GET", path, nil); code != 404 {
			t.Fatalf("%s = %d, want 404", path, code)
		}
	}
	// An organization-wide caller does see the other branch's provider.
	wide := providerAdminRouter(t, f.pool, f.service, f.org, member.ID, nil)
	if code, body := ws5Call(t, wide, "GET", "/fleet-partner/v1/providers/"+outside.ID, nil); code != 200 {
		t.Fatalf("org-wide detail = %d %v", code, body)
	}
	_ = fmt.Sprint
}
