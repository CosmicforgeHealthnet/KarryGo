package crm

import "testing"

func TestExportCreateFingerprint(t *testing.T) {
	actor := Actor{StaffID: "staff-a"}
	req := createExportRequest{ExportType: "CUSTOMER_SUMMARY", Format: "CSV", ScopeType: "BUSINESS", RowLimit: 100, StepUpGrantID: "grant-a"}
	first := exportCreateFingerprint("POST", "/exports", actor, req)
	req.StepUpGrantID = "fresh-grant"
	if first != exportCreateFingerprint("POST", "/exports", actor, req) {
		t.Fatal("fresh verification changed recovery identity")
	}
	req.RowLimit++
	if first == exportCreateFingerprint("POST", "/exports", actor, req) {
		t.Fatal("changed settings reused identity")
	}
	req.RowLimit--
	actor.StaffID = "staff-b"
	if first == exportCreateFingerprint("POST", "/exports", actor, req) {
		t.Fatal("different staff reused identity")
	}
}
