package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/compliance"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/proof"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/safety"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// Section 19 requirement rows that previously had no PostgreSQL test:
// compliance status and requirement reads, provider/asset compliance,
// safety report reads and commands, incident reads/PATCH, incident
// evidence, corrective actions, proof policy reads and proof record list.

type rows19 struct {
	rowsOrgs
	compliance *compliance.Service
	safety     *safety.OperationsService
	proof      *proof.Service
	router     http.Handler
	branchA    string
	branchX    string
	now        time.Time
}

func newRows19(t *testing.T) *rows19 {
	t.Helper()
	orgs := newRowsOrgs(t, "Rows 19", "")
	now := time.Now().UTC().Truncate(time.Second)
	publisher := events.NewDomainPublisher(events.NewOutboxRepository(orgs.pool))
	media := sectionMedia{files: map[string]platformclients.FileMetadata{
		"a-photo":      {FileID: "a-photo", OwnerID: orgs.A.Org, UploadState: "COMPLETED", ScanState: "CLEAN"},
		"a-quarantine": {FileID: "a-quarantine", OwnerID: orgs.A.Org, UploadState: "COMPLETED", ScanState: "QUARANTINED"},
		"b-photo":      {FileID: "b-photo", OwnerID: orgs.B.Org, UploadState: "COMPLETED", ScanState: "CLEAN"},
	}}
	f := &rows19{rowsOrgs: orgs, now: now}
	f.compliance = compliance.NewService(compliance.Options{Pool: orgs.pool, Events: publisher, Flags: compliance.Flags{Compliance: true}, Now: func() time.Time { return now }})
	f.safety = safety.NewOperationsService(safety.OperationsOptions{Pool: orgs.pool, Events: publisher, Media: media, Flags: safety.OperationsFlags{Reports: true, Incidents: true}, Now: func() time.Time { return now }})
	f.proof = proof.NewService(proof.Options{Pool: orgs.pool, Events: publisher, HashSecret: []byte("0123456789abcdef0123456789abcdef"), Flags: proof.Flags{Policies: true, Records: true}, Now: func() time.Time { return now }})
	f.NoPermission = rowsActor{Org: orgs.A.Org, Member: seedApprovalMember(t, orgs.pool, orgs.A.Org, "", "no-role")}
	f.branchA = seedBranch(t, orgs.pool, orgs.A.Org, "Rows19 Main")
	f.branchX = seedBranch(t, orgs.pool, orgs.A.Org, "Rows19 Other")
	f.router = rowsRouter(t, orgs.pool, func(g *gin.RouterGroup, a *accesscontrol.Authorizer, s *idempotency.Store) {
		compliance.NewHandler(f.compliance, a, s).RegisterTenantRoutes(g)
		safety.NewOperationsHandler(f.safety, a, s).RegisterTenantRoutes(g)
		proof.NewHandler(f.proof, a, s).RegisterTenantRoutes(g)
	})
	return f
}

func (f *rows19) requirement(t *testing.T, scope tenant.Context, code, subject, docType string) compliance.Requirement {
	t.Helper()
	ctx := context.Background()
	r, err := f.compliance.CreateRequirement(ctx, scope, compliance.Requirement{Code: code, Name: code + " requirement", SubjectType: subject,
		RequirementType: "DOCUMENT", RequiredDocumentType: docType, Severity: "HIGH", EffectiveFrom: f.now.Add(-time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestRow19_1_And_19_3_ComplianceStatusProviderAndAssetCompliance covers
// FLEET-19.1 (GET /compliance/status) and FLEET-19.3
// (GET /providers/:id/compliance, GET /assets/:id/compliance).
func TestRow19_1_And_19_3_ComplianceStatusProviderAndAssetCompliance(t *testing.T) {
	f := newRows19(t)
	ctx := context.Background()
	assetReq := f.requirement(t, f.tenantA(), "ROADWORTHY", "ASSET", "ROADWORTHINESS")
	providerReq := f.requirement(t, f.tenantA(), "PROVIDER_CHECK", "PROVIDER", "")
	for _, r := range []compliance.Requirement{assetReq, providerReq} {
		if _, err := f.compliance.UpdateRequirement(ctx, f.tenantA(), r.ID, r.Version, compliance.Requirement{}, "ACTIVATE"); err != nil {
			t.Fatal(err)
		}
	}
	assetStatus, err := f.compliance.Evaluate(ctx, f.tenantA(), compliance.EvaluateInput{SubjectType: "ASSET", SubjectReference: "ast-19", RequirementID: assetReq.ID, SourceType: "MANUAL", SourceVersion: 1})
	if err != nil || assetStatus.State != compliance.StateNonCompliant {
		t.Fatalf("asset evaluation = %+v %v", assetStatus, err)
	}
	providerStatus, err := f.compliance.Evaluate(ctx, f.tenantA(), compliance.EvaluateInput{SubjectType: "PROVIDER", SubjectReference: "mem-19", RequirementID: providerReq.ID, SourceType: "MANUAL", SourceVersion: 1})
	if err != nil || providerStatus.State != compliance.StateUnknown {
		t.Fatalf("provider evaluation = %+v %v", providerStatus, err)
	}
	foreignReq := f.requirement(t, f.tenantB(), "ROADWORTHY", "ASSET", "ROADWORTHINESS")
	if _, err = f.compliance.UpdateRequirement(ctx, f.tenantB(), foreignReq.ID, foreignReq.Version, compliance.Requirement{}, "ACTIVATE"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.compliance.Evaluate(ctx, f.tenantB(), compliance.EvaluateInput{SubjectType: "ASSET", SubjectReference: "ast-19", RequirementID: foreignReq.ID, SourceType: "MANUAL", SourceVersion: 1}); err != nil {
		t.Fatal(err)
	}

	all := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/compliance/status", nil, nil), 200, "status").Items(t)
	if ids := rowsIDs(all, "id"); len(all) != 2 || !ids[assetStatus.ID] || !ids[providerStatus.ID] {
		t.Fatalf("compliance status: %v", ids)
	}
	byType := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/compliance/status?subjectType=asset", nil, nil), 200, "by type").Items(t)
	if len(byType) != 1 || byType[0]["id"] != assetStatus.ID {
		t.Fatalf("subjectType filter: %v", byType)
	}
	byRef := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/compliance/status?subjectReference=mem-19", nil, nil), 200, "by ref").Items(t)
	if len(byRef) != 1 || byRef[0]["id"] != providerStatus.ID {
		t.Fatalf("subjectReference filter: %v", byRef)
	}
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", "/fleet-partner/v1/compliance/status", nil, nil), 403, "no permission")

	asset := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/assets/ast-19/compliance", nil, nil), 200, "asset compliance").Items(t)
	if len(asset) != 1 || asset[0]["id"] != assetStatus.ID || asset[0]["state"] != compliance.StateNonCompliant {
		t.Fatalf("asset compliance: %v", asset)
	}
	var reasons []string
	_ = json.Unmarshal([]byte(mustMarshal(t, asset[0]["reasonCodes"])), &reasons)
	if len(reasons) != 1 || reasons[0] != "REQUIRED_EVIDENCE_MISSING" {
		t.Fatalf("asset compliance reasons: %v", asset[0]["reasonCodes"])
	}
	provider := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/providers/mem-19/compliance", nil, nil), 200, "provider compliance").Items(t)
	if len(provider) != 1 || provider[0]["id"] != providerStatus.ID || provider[0]["state"] != compliance.StateUnknown {
		t.Fatalf("provider compliance: %v", provider)
	}
	// The provider path never returns an asset status with the same reference.
	if none := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/providers/ast-19/compliance", nil, nil), 200, "provider by asset ref").Items(t); len(none) != 0 {
		t.Fatalf("provider compliance leaked an asset status: %v", none)
	}
	// Organization B evaluated the same subject reference; A never sees it
	// and B never sees A's.
	if b := rowsExpect(t, rowsDo(t, f.router, f.B, "GET", "/fleet-partner/v1/assets/ast-19/compliance", nil, nil), 200, "other org").Items(t); len(b) != 1 || b[0]["id"] == assetStatus.ID {
		t.Fatalf("cross-tenant asset compliance: %v", b)
	}
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", "/fleet-partner/v1/assets/ast-19/compliance", nil, nil), 403, "no permission asset")
}

func mustMarshal(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// TestRow19_2b_ComplianceRequirementListGet covers FLEET-19.2b:
// GET /compliance/requirements and GET /compliance/requirements/:id.
func TestRow19_2b_ComplianceRequirementListGet(t *testing.T) {
	f := newRows19(t)
	first := f.requirement(t, f.tenantA(), "INSURANCE", "ASSET", "INSURANCE")
	second := f.requirement(t, f.tenantA(), "LICENCE", "PROVIDER", "")
	foreign := f.requirement(t, f.tenantB(), "INSURANCE", "ASSET", "INSURANCE")
	list := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/compliance/requirements", nil, nil), 200, "list").Items(t)
	if len(list) != 2 || list[0]["id"] != second.ID || list[1]["id"] != first.ID {
		t.Fatalf("requirements newest first: %v", rowsIDs(list, "id"))
	}
	got := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/compliance/requirements/"+first.ID, nil, nil), 200, "get").Object(t)
	if got["code"] != "INSURANCE" || got["status"] != "DRAFT" || got["subjectType"] != "ASSET" || got["requiredDocumentType"] != "INSURANCE" {
		t.Fatalf("requirement: %v", got)
	}
	rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/compliance/requirements/"+foreign.ID, nil, nil), 404, "cross-tenant get")
	rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/compliance/requirements/cmpreq_missing", nil, nil), 404, "unknown")
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", "/fleet-partner/v1/compliance/requirements", nil, nil), 403, "no permission")
}

// TestRow19_6b_SafetyReportListGetPatchResolve covers FLEET-19.6b:
// GET /safety/reports, GET/PATCH /safety/reports/:id and
// POST /safety/reports/:id/resolve.
func TestRow19_6b_SafetyReportListGetPatchResolve(t *testing.T) {
	f := newRows19(t)
	ctx := context.Background()
	anonymous, err := f.safety.CreateReport(ctx, f.tenantA(), safety.Report{Anonymous: true, BranchID: f.branchA, ReportType: "HAZARD", Severity: "HIGH", Description: "Oil spill on ramp"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.safety.CreateReport(ctx, f.tenantA(), safety.Report{BranchID: f.branchX, ReportType: "NEAR_MISS", Severity: "LOW", Description: "Reversing truck"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := f.safety.CreateReport(ctx, f.tenantB(), safety.Report{ReportType: "HAZARD", Severity: "LOW", Description: "Elsewhere"})
	if err != nil {
		t.Fatal(err)
	}
	list := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/safety/reports", nil, nil), 200, "list")
	items := list.Items(t)
	if ids := rowsIDs(items, "id"); len(items) != 2 || !ids[anonymous.ID] || !ids[other.ID] || ids[foreign.ID] {
		t.Fatalf("reports: %v", ids)
	}
	// An anonymous reporter is never disclosed, even to the organization
	// owner; a named reporter is.
	for _, item := range items {
		if item["id"] == anonymous.ID && item["reporterReference"] != "" {
			t.Fatalf("anonymous reporter reference leaked in the report list: %v", item)
		}
		if item["id"] == other.ID && item["reporterReference"] != f.A.Member {
			t.Fatalf("named reporter missing: %v", item)
		}
	}
	confined := rowsActor{Org: f.A.Org, Member: f.A.Member, Branch: f.branchA}
	if c := rowsExpect(t, rowsDo(t, f.router, confined, "GET", "/fleet-partner/v1/safety/reports", nil, nil), 200, "confined").Items(t); len(c) != 1 || c[0]["id"] != anonymous.ID {
		t.Fatalf("confined reports: %v", rowsIDs(c, "id"))
	}
	path := "/fleet-partner/v1/safety/reports/" + anonymous.ID
	got := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", path, nil, nil), 200, "get").Object(t)
	if got["reporterReference"] != "" || got["anonymous"] != true || got["severity"] != "HIGH" {
		t.Fatalf("anonymous report: %v", got)
	}
	rowsExpect(t, rowsDo(t, f.router, f.B, "GET", path, nil, nil), 404, "cross-tenant get")
	rowsExpect(t, rowsDo(t, f.router, confined, "GET", "/fleet-partner/v1/safety/reports/"+other.ID, nil, nil), 404, "cross-branch get")
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", path, nil, nil), 403, "no permission")

	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"Status": "UNDER_REVIEW"}, nil), 400, "patch without version")
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"Version": 9, "Status": "UNDER_REVIEW"}, nil), 409, "patch stale")
	rowsExpect(t, rowsDo(t, f.router, f.B, "PATCH", path, map[string]any{"Version": 1, "Status": "UNDER_REVIEW"}, nil), 404, "cross-tenant patch")
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"Version": 1, "Status": "BOGUS"}, nil), 400, "unknown status")
	// Resolution needs its own command and its action + reason.
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"Version": 1, "Status": "RESOLVED"}, nil), 409, "resolve through PATCH")
	patched := rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"Version": 1, "Status": "UNDER_REVIEW", "OwnerUserID": f.A.Member}, nil), 200, "patch").Object(t)
	if patched["status"] != "UNDER_REVIEW" || patched["ownerUserId"] != f.A.Member || patched["version"] != float64(2) {
		t.Fatalf("patched report: %v", patched)
	}
	if patched["reporterReference"] != "" {
		t.Fatalf("PATCH response disclosed the anonymous reporter: %v", patched)
	}
	resolvePath := path + "/resolve"
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", resolvePath, map[string]any{"expectedVersion": 2, "resolutionAction": "Spill cleaned"}, nil), 400, "resolve without reason")
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", resolvePath, map[string]any{"expectedVersion": 1, "resolutionAction": "a", "resolutionReason": "b"}, nil), 409, "resolve stale")
	resolved := rowsExpect(t, rowsDo(t, f.router, f.A, "POST", resolvePath, map[string]any{"expectedVersion": 2, "resolutionAction": "Spill cleaned", "resolutionReason": "Hazard removed"}, nil), 200, "resolve").Object(t)
	if resolved["status"] != "RESOLVED" || resolved["closedAt"] == nil || resolved["resolutionReason"] != "Hazard removed" || resolved["reporterReference"] != "" {
		t.Fatalf("resolved report: %v", resolved)
	}
	// A resolved report is closed.
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"Version": 3, "Status": "OPEN"}, nil), 409, "reopen resolved report")
	if rowsOutboxCount(t, f.pool, anonymous.ID, "safety_report.resolved.v1") != 1 || rowsOutboxCount(t, f.pool, anonymous.ID, "safety_report.updated.v1") != 1 {
		t.Fatal("safety report events missing")
	}
	if n := rowsAuditCount(t, f.pool, f.A.Org, anonymous.ID); n != 3 {
		t.Fatalf("safety report audit records = %d", n)
	}
}

// TestRow19_7b_IncidentListGetPatch covers FLEET-19.7b:
// GET /incidents and GET/PATCH /incidents/:id.
func TestRow19_7b_IncidentListGetPatch(t *testing.T) {
	f := newRows19(t)
	ctx := context.Background()
	older, err := f.safety.CreateIncident(ctx, f.tenantA(), safety.Incident{BranchID: f.branchA, IncidentType: "ACCIDENT", Severity: "HIGH", Description: "Collision", OccurredAt: f.now.Add(-2 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	newer, err := f.safety.CreateIncident(ctx, f.tenantA(), safety.Incident{BranchID: f.branchX, IncidentType: "CARGO_DAMAGE", Severity: "LOW", Description: "Crushed carton", OccurredAt: f.now.Add(-time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.safety.CreateIncident(ctx, f.tenantB(), safety.Incident{IncidentType: "ACCIDENT", Severity: "LOW", Description: "Foreign"}); err != nil {
		t.Fatal(err)
	}
	list := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/incidents", nil, nil), 200, "list").Items(t)
	if len(list) != 2 || list[0]["id"] != newer.ID || list[1]["id"] != older.ID {
		t.Fatalf("incidents newest occurrence first: %v", rowsIDs(list, "id"))
	}
	confined := rowsActor{Org: f.A.Org, Member: f.A.Member, Branch: f.branchA}
	if c := rowsExpect(t, rowsDo(t, f.router, confined, "GET", "/fleet-partner/v1/incidents", nil, nil), 200, "confined").Items(t); len(c) != 1 || c[0]["id"] != older.ID {
		t.Fatalf("confined incidents: %v", rowsIDs(c, "id"))
	}
	path := "/fleet-partner/v1/incidents/" + older.ID
	got := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", path, nil, nil), 200, "get").Object(t)
	if got["incidentType"] != "ACCIDENT" || got["status"] != safety.IncidentOpen || got["reportedBy"] != f.A.Member {
		t.Fatalf("incident: %v", got)
	}
	rowsExpect(t, rowsDo(t, f.router, f.B, "GET", path, nil, nil), 404, "cross-tenant get")
	rowsExpect(t, rowsDo(t, f.router, confined, "GET", "/fleet-partner/v1/incidents/"+newer.ID, nil, nil), 404, "cross-branch get")
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", "/fleet-partner/v1/incidents", nil, nil), 403, "no permission")

	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"OwnerUserID": f.A.Member}, nil), 400, "patch without version")
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"Version": 5, "OwnerUserID": f.A.Member}, nil), 409, "patch stale")
	rowsExpect(t, rowsDo(t, f.router, f.B, "PATCH", path, map[string]any{"Version": 1, "OwnerUserID": f.B.Member}, nil), 404, "cross-tenant patch")
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"Version": 1, "Status": "CLOSED"}, nil), 409, "status through PATCH")
	patched := rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"Version": 1, "OwnerUserID": f.A.Member}, nil), 200, "patch").Object(t)
	if patched["ownerUserId"] != f.A.Member || patched["status"] != safety.IncidentOpen || patched["version"] != float64(2) {
		t.Fatalf("patched incident: %v", patched)
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_incident_history WHERE incident_id=$1 AND action='UPDATED'`, older.ID); n != 1 {
		t.Fatalf("incident update history = %d", n)
	}
	if rowsOutboxCount(t, f.pool, older.ID, "incident.updated.v1") != 1 {
		t.Fatal("incident.updated event missing")
	}
}

// TestRow19_8_IncidentEvidence covers FLEET-19.8: POST /incidents/:id/evidence.
func TestRow19_8_IncidentEvidence(t *testing.T) {
	f := newRows19(t)
	ctx := context.Background()
	inc, err := f.safety.CreateIncident(ctx, f.tenantA(), safety.Incident{BranchID: f.branchA, IncidentType: "ACCIDENT", Severity: "HIGH", Description: "Side swipe"})
	if err != nil {
		t.Fatal(err)
	}
	path := "/fleet-partner/v1/incidents/" + inc.ID + "/evidence"
	source := f.now.Add(-30 * time.Minute)
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", path, map[string]any{}, nil), 400, "missing media")
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", path, map[string]any{"MediaFileID": "b-photo"}, nil), 403, "foreign media")
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", path, map[string]any{"MediaFileID": "a-quarantine"}, nil), 403, "unsafe media")
	rowsExpect(t, rowsDo(t, f.router, f.B, "POST", path, map[string]any{"MediaFileID": "b-photo"}, nil), 404, "cross-tenant")
	rowsExpect(t, rowsDo(t, f.router, rowsActor{Org: f.A.Org, Member: f.A.Member, Branch: f.branchX}, "POST", path, map[string]any{"MediaFileID": "a-photo"}, nil), 404, "cross-branch")
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "POST", path, map[string]any{"MediaFileID": "a-photo"}, nil), 403, "no permission")
	evidence := rowsExpect(t, rowsDo(t, f.router, f.A, "POST", path, map[string]any{"MediaFileID": "a-photo", "SourceTime": source, "HashMetadata": map[string]any{"sha256": "abc"}}, nil), 201, "attach").Object(t)
	if evidence["incidentId"] != inc.ID || evidence["capturedBy"] != f.A.Member || evidence["mediaFileId"] != "a-photo" {
		t.Fatalf("evidence: %v", evidence)
	}
	id := evidence["id"].(string)
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_incident_evidence WHERE id=$1 AND organization_id=$2 AND incident_id=$3`, id, f.A.Org, inc.ID); n != 1 {
		t.Fatalf("evidence rows = %d", n)
	}
	if rowsOutboxCount(t, f.pool, inc.ID, "incident.evidence_attached.v1") != 1 {
		t.Fatal("incident.evidence_attached event missing")
	}
	// Evidence is immutable once captured.
	if _, err = f.pool.Exec(ctx, `UPDATE fleet_partner_incident_evidence SET media_file_id='x' WHERE id=$1`, id); err == nil {
		t.Fatal("incident evidence accepted an UPDATE")
	}
}

// TestRow19_9_CorrectiveActions covers FLEET-19.9:
// POST /incidents/:id/corrective-actions, POST /corrective-actions/:id/complete
// and /verify.
func TestRow19_9_CorrectiveActions(t *testing.T) {
	f := newRows19(t)
	ctx := context.Background()
	investigatorID := seedApprovalMember(t, f.pool, f.A.Org, "rol_system_owner", "investigator")
	investigator := tenant.Context{OrganizationID: f.A.Org, OrganizationUserID: investigatorID, ActorType: tenant.ActorTypeOrganizationUser, ActorID: investigatorID}
	officer := f.tenantA()
	inc, err := f.safety.CreateIncident(ctx, officer, safety.Incident{BranchID: f.branchA, IncidentType: "ACCIDENT", Severity: "MEDIUM", Description: "Slip at depot"})
	if err != nil {
		t.Fatal(err)
	}
	createPath := "/fleet-partner/v1/incidents/" + inc.ID + "/corrective-actions"
	due := f.now.Add(72 * time.Hour)
	// Before findings are approved the incident is not in its corrective stage.
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", createPath, map[string]any{"ActionType": "TRAINING", "OwnerUserID": f.A.Member, "DueAt": due}, nil), 409, "before findings")

	triaged, err := f.safety.TriageIncident(ctx, officer, inc.ID, inc.Version, safety.TriageInput{Severity: "MEDIUM", LocationReference: "depot", ImmediateActions: "Area dried"})
	if err != nil {
		t.Fatal(err)
	}
	contained, err := f.safety.ContainIncident(ctx, officer, inc.ID, triaged.Version, "Area cordoned")
	if err != nil {
		t.Fatal(err)
	}
	inv, investigating, err := f.safety.OpenInvestigation(ctx, officer, inc.ID, contained.Version, safety.InvestigationInput{InvestigatorUserID: investigatorID, Scope: "Depot floor", EvidencePreservation: "Photos kept"})
	if err != nil {
		t.Fatal(err)
	}
	_ = investigating
	if inv, err = f.safety.SubmitFindings(ctx, investigator, inv.ID, inv.Version, safety.FindingsInput{Findings: "Leaking roof", RootCause: "Gutter blocked"}); err != nil {
		t.Fatal(err)
	}
	_, corrective, approved, err := f.safety.ApproveFindings(ctx, officer, inv.ID, inv.Version, []safety.CorrectiveAction{{ActionType: "FIX_GUTTER", OwnerUserID: f.A.Member, DueAt: due}})
	if err != nil || corrective.Status != safety.IncidentCorrectiveAction || len(approved) != 1 {
		t.Fatalf("approve findings = %+v %v", corrective, err)
	}

	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", createPath, map[string]any{"ActionType": "TRAINING", "OwnerUserID": f.A.Member}, nil), 400, "missing due date")
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", createPath, map[string]any{"ActionType": "TRAINING", "OwnerUserID": f.A.Member, "DueAt": f.now.Add(-time.Hour)}, nil), 400, "past due date")
	rowsExpect(t, rowsDo(t, f.router, f.B, "POST", createPath, map[string]any{"ActionType": "TRAINING", "OwnerUserID": f.B.Member, "DueAt": due}, nil), 404, "cross-tenant create")
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "POST", createPath, map[string]any{"ActionType": "TRAINING", "OwnerUserID": f.A.Member, "DueAt": due}, nil), 403, "no permission")
	created := rowsExpect(t, rowsDo(t, f.router, f.A, "POST", createPath, map[string]any{"ActionType": "TRAINING", "OwnerUserID": f.A.Member, "DueAt": due}, nil), 201, "create").Object(t)
	if created["status"] != "OPEN" || created["incidentId"] != inc.ID {
		t.Fatalf("corrective action: %v", created)
	}
	actionID := created["id"].(string)
	if rowsOutboxCount(t, f.pool, actionID, "corrective_action.created.v1") != 1 {
		t.Fatal("corrective_action.created event missing")
	}

	completePath := "/fleet-partner/v1/corrective-actions/" + actionID + "/complete"
	verifyPath := "/fleet-partner/v1/corrective-actions/" + actionID + "/verify"
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", verifyPath, map[string]any{"expectedVersion": 1}, nil), 409, "verify before complete")
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", completePath, map[string]any{"notes": "done"}, nil), 400, "complete without version")
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", completePath, map[string]any{"expectedVersion": 4, "notes": "done"}, nil), 409, "complete stale")
	rowsExpect(t, rowsDo(t, f.router, f.B, "POST", completePath, map[string]any{"expectedVersion": 1, "notes": "done"}, nil), 404, "cross-tenant complete")
	rowsExpect(t, rowsDo(t, f.router, rowsActor{Org: f.A.Org, Member: f.A.Member, Branch: f.branchX}, "POST", completePath, map[string]any{"expectedVersion": 1, "notes": "done"}, nil), 404, "cross-branch complete")
	completed := rowsExpect(t, rowsDo(t, f.router, f.A, "POST", completePath, map[string]any{"expectedVersion": 1, "notes": "Staff trained"}, nil), 200, "complete").Object(t)
	if completed["status"] != "COMPLETED" || completed["completionNotes"] != "Staff trained" || completed["completedAt"] == nil {
		t.Fatalf("completed: %v", completed)
	}
	verified := rowsExpect(t, rowsDo(t, f.router, rowsActor{Org: f.A.Org, Member: investigatorID}, "POST", verifyPath, map[string]any{"expectedVersion": 2}, nil), 200, "verify").Object(t)
	if verified["status"] != "VERIFIED" || verified["verifiedBy"] != investigatorID {
		t.Fatalf("verified: %v", verified)
	}
	// A verified action cannot be pushed back to COMPLETED.
	rowsExpect(t, rowsDo(t, f.router, f.A, "POST", completePath, map[string]any{"expectedVersion": 3, "notes": "again"}, nil), 409, "complete after verify")
	if rowsOutboxCount(t, f.pool, actionID, "corrective_action.completed.v1") != 1 || rowsOutboxCount(t, f.pool, actionID, "corrective_action.verified.v1") != 1 {
		t.Fatal("corrective action completion/verification events missing")
	}
}

// TestRow19_10b_ProofPolicyListGet covers FLEET-19.10b:
// GET /proof-policies and GET /proof-policies/:id.
func TestRow19_10b_ProofPolicyListGet(t *testing.T) {
	f := newRows19(t)
	ctx := context.Background()
	policy, _, err := f.proof.SavePolicy(ctx, f.tenantA(), "", 0, proof.Policy{PolicyScope: "ORGANIZATION", ScopeReference: f.A.Org, RequiredProofTypes: json.RawMessage(`["SIGNATURE","PHOTO"]`), MinimumCount: 2, FailurePolicy: "BLOCK"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, _, err := f.proof.SavePolicy(ctx, f.tenantB(), "", 0, proof.Policy{PolicyScope: "ORGANIZATION", ScopeReference: f.B.Org, RequiredProofTypes: json.RawMessage(`["OTP"]`), MinimumCount: 1, FailurePolicy: "BLOCK"})
	if err != nil {
		t.Fatal(err)
	}
	list := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/proof-policies", nil, nil), 200, "list").Items(t)
	if len(list) != 1 || list[0]["id"] != policy.ID {
		t.Fatalf("policies: %v", list)
	}
	got := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/proof-policies/"+policy.ID, nil, nil), 200, "get").Object(t)
	if got["minimumCount"] != float64(2) || got["failurePolicy"] != "BLOCK" || got["policyScope"] != "ORGANIZATION" {
		t.Fatalf("policy: %v", got)
	}
	rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/proof-policies/"+foreign.ID, nil, nil), 404, "cross-tenant get")
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", "/fleet-partner/v1/proof-policies", nil, nil), 403, "no permission")
}

// TestRow19_11b_ProofRecordList covers FLEET-19.11b: GET /proof-records.
func TestRow19_11b_ProofRecordList(t *testing.T) {
	f := newRows19(t)
	ctx := context.Background()
	_, snap, err := f.proof.SavePolicy(ctx, f.tenantA(), "", 0, proof.Policy{PolicyScope: "ORGANIZATION", ScopeReference: f.A.Org, RequiredProofTypes: json.RawMessage(`["OTP"]`), MinimumCount: 1, FailurePolicy: "BLOCK"})
	if err != nil {
		t.Fatal(err)
	}
	record, err := f.proof.CreateRecord(ctx, f.tenantA(), proof.RecordInput{PolicySnapshotReference: snap.ID, ProofType: "OTP", OrderID: "order-19", ActorType: "ORGANIZATION_USER", Value: "731905", SourceTimestamp: f.now, Source: "MANUAL", SourceVersion: 1, IdempotencyKey: "rows19-proof"})
	if err != nil {
		t.Fatal(err)
	}
	_, foreignSnap, err := f.proof.SavePolicy(ctx, f.tenantB(), "", 0, proof.Policy{PolicyScope: "ORGANIZATION", ScopeReference: f.B.Org, RequiredProofTypes: json.RawMessage(`["OTP"]`), MinimumCount: 1, FailurePolicy: "BLOCK"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.proof.CreateRecord(ctx, f.tenantB(), proof.RecordInput{PolicySnapshotReference: foreignSnap.ID, ProofType: "OTP", OrderID: "order-b", ActorType: "ORGANIZATION_USER", Value: "111111", SourceTimestamp: f.now, Source: "MANUAL", SourceVersion: 1, IdempotencyKey: "rows19-proof-b"}); err != nil {
		t.Fatal(err)
	}
	resp := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/proof-records", nil, nil), 200, "list")
	items := resp.Items(t)
	if len(items) != 1 || items[0]["id"] != record.ID || items[0]["orderId"] != "order-19" || items[0]["proofType"] != "OTP" {
		t.Fatalf("proof records: %v", items)
	}
	// Neither the raw proof value nor its keyed hash leaves the service.
	if strings.Contains(string(resp.Raw), "731905") || strings.Contains(string(resp.Raw), record.ValueHash) || strings.Contains(strings.ToLower(string(resp.Raw)), "valuehash") {
		t.Fatalf("proof record list disclosed the proof value or hash: %s", resp.Raw)
	}
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", "/fleet-partner/v1/proof-records", nil, nil), 403, "no permission")
}
