package pastry

import (
	"cosmicforge/logistics/services/vendor-service/internal/features/crm"
	"testing"
	"time"
)

func TestSelectableCustomer(t *testing.T) {
	ref := "canonical-customer"
	valid := crm.Profile{SubjectType: crm.SubjectCustomer, CustomerRef: &ref, Status: crm.StatusActive, PrivacyState: crm.PrivacyActive, MergeState: crm.MergeNone}
	if !selectableCustomer(valid) {
		t.Fatal("active canonical relationship refused")
	}
	tests := []func(*crm.Profile){func(p *crm.Profile) { p.SubjectType = crm.SubjectGuest }, func(p *crm.Profile) { p.CustomerRef = nil }, func(p *crm.Profile) { p.Status = crm.StatusArchived }, func(p *crm.Profile) { p.PrivacyState = crm.PrivacyRestricted }, func(p *crm.Profile) { p.PrivacyState = crm.PrivacyRedacted }, func(p *crm.Profile) { p.MergeState = crm.MergeSource }, func(p *crm.Profile) { p.MergeState = crm.MergeLinkPending }, func(p *crm.Profile) { now := time.Now(); p.RedactedAt = &now }, func(p *crm.Profile) { p.MergedIntoProfileID = &ref }}
	for i, change := range tests {
		p := valid
		change(&p)
		if selectableCustomer(p) {
			t.Fatalf("ineligible relationship %d allowed", i)
		}
	}
	if scopeCovers(crm.Scope{BranchIDs: []string{"branch-a"}}, "branch-b") {
		t.Fatal("foreign branch allowed")
	}
}
