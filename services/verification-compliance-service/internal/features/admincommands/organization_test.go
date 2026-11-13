package admincommands

import (
	"testing"

	"cosmicforge/logistics/shared/go/admincommand"
)

// Operator actions on a company submission map onto organization decisions
// (Fleet ADR-006), and every one of them is in the shared dictionary.
func TestOrganizationDecisionMapping(t *testing.T) {
	spec := admincommand.VerificationSpec()
	cases := []struct {
		action, status, restriction string
		issues                      int
		params                      map[string]string
	}{
		{"START_ORGANIZATION_REVIEW", "IN_PROGRESS", "NONE", 0, nil},
		{"REQUEST_ORGANIZATION_CHANGES", "CHANGES_REQUIRED", "NONE", 1, map[string]string{"issue": "DOCUMENT_EXPIRED", "remediation": "RESUBMIT_DOCUMENT", "document": "TAX_CLEARANCE"}},
		{"APPROVE_ORGANIZATION", "VERIFIED", "NONE", 0, nil},
		{"REJECT_ORGANIZATION", "REJECTED", "NONE", 1, map[string]string{"reason": "FRAUD_SIGNAL"}},
		{"SUSPEND_ORGANIZATION", "SUSPENDED", "SUSPENDED", 1, map[string]string{"reason": "COMPLIANCE_LAPSE"}},
	}
	for _, c := range cases {
		action, ok := spec.Actions[c.action]
		if !ok || len(action.SubjectKinds) != 1 || action.SubjectKinds[0] != OrganizationSubjectKind {
			t.Fatalf("%s not in the dictionary for organization submissions", c.action)
		}
		status, restriction, issues, err := OrganizationDecision(admincommand.Execution{Command: admincommand.Command{Action: c.action, Params: c.params}})
		if err != nil || status != c.status || restriction != c.restriction || len(issues) != c.issues {
			t.Fatalf("%s -> %s %s %d %v", c.action, status, restriction, len(issues), err)
		}
	}
	if !spec.Actions["APPROVE_ORGANIZATION"].RequiresApproval || !spec.Actions["REJECT_ORGANIZATION"].RequiresApproval {
		t.Fatal("final company decisions must require a second approver")
	}
	if _, _, _, err := OrganizationDecision(admincommand.Execution{Command: admincommand.Command{Action: "APPROVE_CASE"}}); err != admincommand.ErrRejected {
		t.Fatal("a vendor action must not decide a company")
	}
	if permissionFor("APPROVE_ORGANIZATION") == permissionFor("START_ORGANIZATION_REVIEW") {
		t.Fatal("deciding needs a stronger grant than reviewing")
	}
}
