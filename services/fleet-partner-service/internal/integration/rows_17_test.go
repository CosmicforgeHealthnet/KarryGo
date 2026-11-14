package integration_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// Section 17 requirement rows that previously had no PostgreSQL test: the
// workforce read surfaces (list/get), the PATCH/archive commands, shift
// assignment reads and performance reads. Seeding goes through the real
// workforce use cases; every assertion below is made through the production
// tenant handler.

type rows17 struct {
	rowsOrgs
	svc     *workforce.Service
	router  http.Handler
	branchA string
	branchX string
	now     time.Time
}

func newRows17(t *testing.T) *rows17 {
	t.Helper()
	orgs := newRowsOrgs(t, "Rows 17", "")
	now := time.Now().UTC().Truncate(time.Second)
	svc := workforce.NewService(workforce.Options{
		Pool: orgs.pool, Events: events.NewDomainPublisher(events.NewOutboxRepository(orgs.pool)),
		Flags: workforce.Flags{Workforce: true, Shifts: true, Leave: true, Qualifications: true,
			Training: true, Performance: true},
		Now: func() time.Time { return now },
	})
	f := &rows17{rowsOrgs: orgs, svc: svc, now: now}
	f.NoPermission = rowsActor{Org: orgs.A.Org, Member: seedApprovalMember(t, orgs.pool, orgs.A.Org, "", "no-role")}
	f.branchA = seedBranch(t, orgs.pool, orgs.A.Org, "Rows17 Main")
	f.branchX = seedBranch(t, orgs.pool, orgs.A.Org, "Rows17 Other")
	f.router = rowsRouter(t, orgs.pool, func(g *gin.RouterGroup, a *accesscontrol.Authorizer, s *idempotency.Store) {
		workforce.NewHandler(svc, a, s).RegisterTenantRoutes(g)
	})
	return f
}

func (f *rows17) providerMembership(t *testing.T, org string) string {
	t.Helper()
	id := ids.New("mem")
	if _, err := f.pool.Exec(context.Background(), `INSERT INTO fleet_partner_organization_provider_memberships(id,organization_id,provider_service,service_category,provider_id,onboarding_source,status)VALUES($1,$2,'driver-dispatch-delivery-service','DISPATCH',$3,'ORGANIZATION_SPONSORED','ACTIVE')`, id, org, "prv-"+id); err != nil {
		t.Fatal(err)
	}
	return id
}

// TestRow17_1b_WorkforceProfileListGetPatchArchive covers FLEET-17.1b:
// GET /workforce/profiles, GET/PATCH /workforce/profiles/:id and
// POST /workforce/profiles/:id/archive.
func TestRow17_1b_WorkforceProfileListGetPatchArchive(t *testing.T) {
	f := newRows17(t)
	ctx := context.Background()
	memberA := f.providerMembership(t, f.A.Org)
	profile, err := f.svc.CreateProfile(ctx, f.tenantA(), workforce.CreateProfileInput{BranchID: f.branchA,
		MembershipID: memberA, EmployeeNumber: "EMP-17-1", RelationshipType: "MANAGED_PROVIDER", Role: "DRIVER", StartDate: f.now})
	if err != nil {
		t.Fatal(err)
	}
	branchB := seedBranch(t, f.pool, f.B.Org, "Rows17 B")
	memberB := f.providerMembership(t, f.B.Org)
	foreign, err := f.svc.CreateProfile(ctx, f.tenantB(), workforce.CreateProfileInput{BranchID: branchB,
		MembershipID: memberB, EmployeeNumber: "EMP-17-B", RelationshipType: "MANAGED_PROVIDER", Role: "DRIVER", StartDate: f.now})
	if err != nil {
		t.Fatal(err)
	}

	list := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/workforce/profiles", nil, nil), 200, "list profiles").Items(t)
	got := rowsIDs(list, "id")
	if !got[profile.ID] || got[foreign.ID] {
		t.Fatalf("list must contain own profile only: %v", got)
	}
	if items := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/workforce/profiles?status=ENDED", nil, nil), 200, "filtered").Items(t); len(items) != 0 {
		t.Fatalf("status filter ignored: %v", items)
	}
	// A member confined to another branch sees neither the list row nor the object.
	confined := rowsActor{Org: f.A.Org, Member: f.A.Member, Branch: f.branchX}
	if items := rowsExpect(t, rowsDo(t, f.router, confined, "GET", "/fleet-partner/v1/workforce/profiles", nil, nil), 200, "confined list").Items(t); rowsIDs(items, "id")[profile.ID] {
		t.Fatal("branch-confined member listed another branch's profile")
	}
	rowsExpect(t, rowsDo(t, f.router, confined, "GET", "/fleet-partner/v1/workforce/profiles/"+profile.ID, nil, nil), 404, "confined get")

	get := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/workforce/profiles/"+profile.ID, nil, nil), 200, "get profile").Object(t)
	if get["employeeNumber"] != "EMP-17-1" || get["branchId"] != f.branchA {
		t.Fatalf("get profile: %v", get)
	}
	rowsExpect(t, rowsDo(t, f.router, f.B, "GET", "/fleet-partner/v1/workforce/profiles/"+profile.ID, nil, nil), 404, "cross-tenant get")
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", "/fleet-partner/v1/workforce/profiles", nil, nil), 403, "no permission list")

	path := "/fleet-partner/v1/workforce/profiles/" + profile.ID
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"role": "SUPERVISOR"}, nil), 400, "patch without expectedVersion")
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"expectedVersion": 9, "role": "SUPERVISOR"}, nil), 409, "patch stale version")
	rowsExpect(t, rowsDo(t, f.router, f.B, "PATCH", path, map[string]any{"expectedVersion": 1, "role": "SUPERVISOR"}, nil), 404, "cross-tenant patch")
	patched := rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"expectedVersion": 1, "role": "SUPERVISOR", "status": "INACTIVE"}, nil), 200, "patch").Object(t)
	if patched["role"] != "SUPERVISOR" || patched["status"] != "INACTIVE" || patched["version"] != float64(2) {
		t.Fatalf("patch result: %v", patched)
	}
	archived := rowsExpect(t, rowsDo(t, f.router, f.A, "POST", path+"/archive", map[string]any{"expectedVersion": 2}, nil), 200, "archive").Object(t)
	if archived["status"] != "ARCHIVED" {
		t.Fatalf("archive result: %v", archived)
	}
	// ARCHIVED is terminal.
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"expectedVersion": 3, "status": "ACTIVE"}, nil), 409, "reactivate archived")
	if n := rowsOutboxCount(t, f.pool, profile.ID, "workforce_profile.ended.v1"); n != 1 {
		t.Fatalf("archive outbox events = %d", n)
	}
	if n := rowsAuditCount(t, f.pool, f.A.Org, profile.ID); n != 3 {
		t.Fatalf("audit records for profile = %d, want create+patch+archive", n)
	}
	if n := countWhere(t, f.pool, `SELECT count(*) FROM fleet_partner_workforce_history WHERE aggregate_id=$1`, profile.ID); n != 3 {
		t.Fatalf("history rows = %d", n)
	}
}

// TestRow17_2b_ShiftListGet covers FLEET-17.2b: GET /shifts and GET /shifts/:id.
func TestRow17_2b_ShiftListGet(t *testing.T) {
	f := newRows17(t)
	ctx := context.Background()
	mk := func(branch, name string, offset time.Duration) workforce.Shift {
		s, err := f.svc.CreateShift(ctx, f.tenantA(), workforce.CreateShiftInput{BranchID: branch, Name: name, Role: "DRIVER",
			Timezone: "Africa/Lagos", StartAt: f.now.Add(offset), EndAt: f.now.Add(offset + 8*time.Hour), RequiredCapacity: 2})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	early := mk(f.branchA, "Early", time.Hour)
	late := mk(f.branchA, "Late", 10*time.Hour)
	other := mk(f.branchX, "Other branch", 2*time.Hour)
	if _, err := f.svc.UpdateShift(ctx, f.tenantA(), late.ID, late.Version, "", nil, nil, "PUBLISHED"); err != nil {
		t.Fatal(err)
	}

	items := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/shifts", nil, nil), 200, "list shifts").Items(t)
	if len(items) != 3 || items[0]["id"] != late.ID {
		t.Fatalf("shifts must be newest start first: %v", items)
	}
	published := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/shifts?status=published", nil, nil), 200, "status filter").Items(t)
	if len(published) != 1 || published[0]["id"] != late.ID {
		t.Fatalf("status filter: %v", published)
	}
	byBranch := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/shifts?branchId="+f.branchX, nil, nil), 200, "branch filter").Items(t)
	if len(byBranch) != 1 || byBranch[0]["id"] != other.ID {
		t.Fatalf("branch filter: %v", byBranch)
	}
	limited := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/shifts?limit=1&offset=1", nil, nil), 200, "paged").Items(t)
	if len(limited) != 1 || limited[0]["id"] != other.ID {
		t.Fatalf("paging: %v", limited)
	}
	// An invalid page window is a client error, never a server error.
	rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/shifts?offset=-1", nil, nil), 400, "negative offset")
	rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/workforce/profiles?offset=-3", nil, nil), 400, "negative profile offset")
	rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/leave?offset=-3", nil, nil), 400, "negative leave offset")
	if items := rowsExpect(t, rowsDo(t, f.router, f.B, "GET", "/fleet-partner/v1/shifts", nil, nil), 200, "other org").Items(t); len(items) != 0 {
		t.Fatalf("other organization saw shifts: %v", items)
	}
	got := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/shifts/"+early.ID, nil, nil), 200, "get").Object(t)
	if got["name"] != "Early" || got["status"] != "DRAFT" || got["requiredCapacity"] != float64(2) {
		t.Fatalf("get shift: %v", got)
	}
	rowsExpect(t, rowsDo(t, f.router, f.B, "GET", "/fleet-partner/v1/shifts/"+early.ID, nil, nil), 404, "cross-tenant")
	rowsExpect(t, rowsDo(t, f.router, rowsActor{Org: f.A.Org, Member: f.A.Member, Branch: f.branchA}, "GET", "/fleet-partner/v1/shifts/"+other.ID, nil, nil), 404, "cross-branch")
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", "/fleet-partner/v1/shifts/"+early.ID, nil, nil), 403, "no permission")
}

// TestRow17_4b_GetShiftAssignment covers FLEET-17.4b: GET /shift-assignments/:id.
func TestRow17_4b_GetShiftAssignment(t *testing.T) {
	f := newRows17(t)
	ctx := context.Background()
	member := f.providerMembership(t, f.A.Org)
	shift, err := f.svc.CreateShift(ctx, f.tenantA(), workforce.CreateShiftInput{BranchID: f.branchA, Name: "Day", Role: "DRIVER",
		Timezone: "Africa/Lagos", StartAt: f.now.Add(time.Hour), EndAt: f.now.Add(9 * time.Hour), RequiredCapacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	if shift, err = f.svc.UpdateShift(ctx, f.tenantA(), shift.ID, shift.Version, "", nil, nil, "PUBLISHED"); err != nil {
		t.Fatal(err)
	}
	assignment, err := f.svc.AssignShift(ctx, f.tenantA(), shift.ID, workforce.AssignShiftInput{MembershipID: member, AssignmentType: "PRIMARY"})
	if err != nil {
		t.Fatal(err)
	}
	got := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/shift-assignments/"+assignment.ID, nil, nil), 200, "get assignment").Object(t)
	if got["shiftId"] != shift.ID || got["membershipId"] != member || got["assignmentType"] != "PRIMARY" {
		t.Fatalf("assignment: %v", got)
	}
	rowsExpect(t, rowsDo(t, f.router, f.B, "GET", "/fleet-partner/v1/shift-assignments/"+assignment.ID, nil, nil), 404, "cross-tenant")
	// The assignment inherits its shift's branch scope.
	rowsExpect(t, rowsDo(t, f.router, rowsActor{Org: f.A.Org, Member: f.A.Member, Branch: f.branchX}, "GET", "/fleet-partner/v1/shift-assignments/"+assignment.ID, nil, nil), 404, "cross-branch")
	rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/shift-assignments/sas_missing", nil, nil), 404, "unknown")
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", "/fleet-partner/v1/shift-assignments/"+assignment.ID, nil, nil), 403, "no permission")
}

// TestRow17_5b_LeaveListGet covers FLEET-17.5b: GET /leave and GET /leave/:id.
func TestRow17_5b_LeaveListGet(t *testing.T) {
	f := newRows17(t)
	ctx := context.Background()
	member := f.providerMembership(t, f.A.Org)
	first, err := f.svc.CreateLeave(ctx, f.tenantA(), workforce.CreateLeaveInput{MembershipID: member, LeaveType: "ANNUAL",
		Reason: "holiday", StartAt: f.now.Add(24 * time.Hour), EndAt: f.now.Add(72 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.svc.CreateLeave(ctx, f.tenantA(), workforce.CreateLeaveInput{MembershipID: member, LeaveType: "SICK",
		Reason: "flu", StartAt: f.now.Add(240 * time.Hour), EndAt: f.now.Add(250 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.svc.LeaveAction(ctx, f.tenantA(), second.ID, "APPROVED", second.Version); err != nil {
		t.Fatal(err)
	}
	all := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/leave", nil, nil), 200, "list leave").Items(t)
	if len(all) != 2 || all[0]["id"] != second.ID {
		t.Fatalf("leave list newest start first: %v", all)
	}
	approved := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/leave?status=APPROVED", nil, nil), 200, "filter").Items(t)
	if len(approved) != 1 || approved[0]["id"] != second.ID || approved[0]["approvedBy"] != f.A.Member {
		t.Fatalf("approved filter: %v", approved)
	}
	if items := rowsExpect(t, rowsDo(t, f.router, f.B, "GET", "/fleet-partner/v1/leave", nil, nil), 200, "other org").Items(t); len(items) != 0 {
		t.Fatalf("other org saw leave: %v", items)
	}
	got := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/leave/"+first.ID, nil, nil), 200, "get leave").Object(t)
	if got["leaveType"] != "ANNUAL" || got["status"] != "REQUESTED" || got["membershipId"] != member {
		t.Fatalf("get leave: %v", got)
	}
	rowsExpect(t, rowsDo(t, f.router, f.B, "GET", "/fleet-partner/v1/leave/"+first.ID, nil, nil), 404, "cross-tenant")
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", "/fleet-partner/v1/leave", nil, nil), 403, "no permission")
}

// TestRow17_6b_QualificationListGetPatch covers FLEET-17.6b:
// GET /qualifications, GET/PATCH /qualifications/:id.
func TestRow17_6b_QualificationListGetPatch(t *testing.T) {
	f := newRows17(t)
	ctx := context.Background()
	q, err := f.svc.CreateQualification(ctx, f.tenantA(), workforce.Qualification{Code: "HGV", Name: "Heavy goods licence",
		ApplicableServices: []string{"HAULAGE"}, EvidenceRequired: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.svc.CreateQualification(ctx, f.tenantA(), workforce.Qualification{Code: "ADR", Name: "Dangerous goods"}); err != nil {
		t.Fatal(err)
	}
	foreign, err := f.svc.CreateQualification(ctx, f.tenantB(), workforce.Qualification{Code: "HGV", Name: "Other org"})
	if err != nil {
		t.Fatal(err)
	}
	items := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/qualifications", nil, nil), 200, "list").Items(t)
	if len(items) != 2 || items[0]["code"] != "ADR" || items[1]["code"] != "HGV" {
		t.Fatalf("qualifications ordered by code: %v", items)
	}
	if rowsIDs(items, "id")[foreign.ID] {
		t.Fatal("foreign qualification listed")
	}
	path := "/fleet-partner/v1/qualifications/" + q.ID
	got := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", path, nil, nil), 200, "get").Object(t)
	if got["name"] != "Heavy goods licence" || got["evidenceRequired"] != true {
		t.Fatalf("get: %v", got)
	}
	rowsExpect(t, rowsDo(t, f.router, f.B, "GET", path, nil, nil), 404, "cross-tenant get")
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"name": "x"}, nil), 400, "patch without version")
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"version": 7, "name": "x"}, nil), 409, "stale version")
	rowsExpect(t, rowsDo(t, f.router, f.B, "PATCH", path, map[string]any{"version": 1, "name": "x"}, nil), 404, "cross-tenant patch")
	// A rename must not silently drop the evidence requirement.
	patched := rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"version": 1, "name": "HGV Class 1"}, nil), 200, "patch").Object(t)
	if patched["name"] != "HGV Class 1" || patched["code"] != "HGV" || patched["version"] != float64(2) {
		t.Fatalf("patch: %v", patched)
	}
	after := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", path, nil, nil), 200, "reread").Object(t)
	if after["evidenceRequired"] != true {
		t.Fatalf("partial PATCH cleared evidenceRequired: %v", after)
	}
	if svc := after["applicableServices"].([]any); len(svc) != 1 || svc[0] != "HAULAGE" {
		t.Fatalf("partial PATCH cleared applicableServices: %v", after)
	}
	// An explicit false is still honoured.
	if off := rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"version": 2, "evidenceRequired": false}, nil), 200, "explicit false").Object(t); off["evidenceRequired"] != false {
		t.Fatalf("explicit evidenceRequired=false ignored: %v", off)
	}
	if n := rowsOutboxCount(t, f.pool, q.ID, "qualification.updated.v1"); n != 2 {
		t.Fatalf("qualification.updated events = %d", n)
	}
}

// TestRow17_8b_TrainingCourseListGetPatch covers FLEET-17.8b:
// GET /training/courses, GET/PATCH /training/courses/:id.
func TestRow17_8b_TrainingCourseListGetPatch(t *testing.T) {
	f := newRows17(t)
	ctx := context.Background()
	course, err := f.svc.CreateCourse(ctx, f.tenantA(), workforce.Course{Code: "SAFE", Title: "Road safety", RequiredServices: []string{"DISPATCH"}, PassingScore: 70})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := f.svc.CreateCourse(ctx, f.tenantB(), workforce.Course{Code: "SAFE", Title: "Other", PassingScore: 50})
	if err != nil {
		t.Fatal(err)
	}
	items := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/training/courses", nil, nil), 200, "list").Items(t)
	if len(items) != 1 || items[0]["id"] != course.ID {
		t.Fatalf("course list: %v", items)
	}
	path := "/fleet-partner/v1/training/courses/" + course.ID
	got := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", path, nil, nil), 200, "get").Object(t)
	if got["title"] != "Road safety" || got["passingScore"] != float64(70) {
		t.Fatalf("get: %v", got)
	}
	rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/training/courses/"+foreign.ID, nil, nil), 404, "cross-tenant get")
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"title": "x"}, nil), 400, "patch without version")
	rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"version": 4, "title": "x"}, nil), 409, "stale")
	// A title-only PATCH must keep the passing threshold: silently dropping
	// it to zero would let every attempt pass the course.
	patched := rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"version": 1, "title": "Road safety v2"}, nil), 200, "patch").Object(t)
	if patched["title"] != "Road safety v2" || patched["version"] != float64(2) {
		t.Fatalf("patch: %v", patched)
	}
	after := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", path, nil, nil), 200, "reread").Object(t)
	if after["passingScore"] != float64(70) {
		t.Fatalf("title-only PATCH changed passingScore: %v", after)
	}
	raised := rowsExpect(t, rowsDo(t, f.router, f.A, "PATCH", path, map[string]any{"version": 2, "passingScore": 85}, nil), 200, "raise score").Object(t)
	if raised["passingScore"] != float64(85) {
		t.Fatalf("explicit passingScore ignored: %v", raised)
	}
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "PATCH", path, map[string]any{"version": 3, "title": "x"}, nil), 403, "no permission")
	if n := rowsOutboxCount(t, f.pool, course.ID, "training.course_updated.v1"); n != 2 {
		t.Fatalf("course updated events = %d", n)
	}
}

// TestRow17_12b_ProviderPerformanceRead covers FLEET-17.12b:
// GET /performance/providers/:membershipId.
func TestRow17_12b_ProviderPerformanceRead(t *testing.T) {
	f := newRows17(t)
	ctx := context.Background()
	member := f.providerMembership(t, f.A.Org)
	if _, err := f.pool.Exec(ctx, `INSERT INTO fleet_partner_private_assignments(id,organization_id,branch_id,work_origin,route_stop_id,provider_membership_id,provider_id,execution_service,status,response_deadline,decline_reason,declined_at,created_by,created_at)VALUES($1,$2,$3,'ROUTE_STOP','stop-perf',$4,'prv','DISPATCH','DECLINED',now()+interval '1 hour','UNSAFE_VEHICLE',now(),$4,$5)`,
		ids.New("pas"), f.A.Org, f.branchA, member, f.now); err != nil {
		t.Fatal(err)
	}
	snap, err := f.svc.GeneratePerformance(ctx, f.tenantA(), member, f.now.Add(-time.Hour), f.now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	items := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/performance/providers/"+member, nil, nil), 200, "performance").Items(t)
	if len(items) != 1 || items[0]["id"] != snap.ID || items[0]["membershipId"] != member {
		t.Fatalf("performance: %v", items)
	}
	// The snapshot is explanatory only: no automatic discipline.
	explanation, _ := items[0]["explanation"].(map[string]any)
	if explanation["disciplinaryAction"] != false {
		t.Fatalf("explanation: %v", items[0]["explanation"])
	}
	if other := rowsExpect(t, rowsDo(t, f.router, f.B, "GET", "/fleet-partner/v1/performance/providers/"+member, nil, nil), 200, "other org").Items(t); len(other) != 0 {
		t.Fatalf("other organization read performance: %v", other)
	}
	rowsExpect(t, rowsDo(t, f.router, f.NoPermission, "GET", "/fleet-partner/v1/performance/providers/"+member, nil, nil), 403, "no permission")
}
