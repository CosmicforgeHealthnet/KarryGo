package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
	"time"

	workforcemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce/models"
	workforceusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce/usecases"
)

// FLEET-6f: the workforce lists continue by cursor. Each walk below follows
// meta.nextCursor from the first page to the last and checks that the pages
// together are exactly the expected rows: no row twice, none missing, none
// from a branch the caller may not see, and no nextCursor on the last page.

// walkWorkforcePages follows nextCursor through a list and returns the ids in
// the order served and the number of pages.
func walkWorkforcePages(t *testing.T, f *rows17, actor rowsActor, path string, limit int) ([]string, int) {
	t.Helper()
	var ids []string
	seen := map[string]bool{}
	next := ""
	for page := 1; ; page++ {
		q := url.Values{"limit": {fmt.Sprint(limit)}}
		if next != "" {
			q.Set("cursor", next)
		}
		r := rowsExpect(t, rowsDo(t, f.router, actor, "GET", path+"?"+q.Encode(), nil, nil), 200, path)
		items := r.Items(t)
		if len(items) > limit {
			t.Fatalf("%s page %d served %d rows over limit %d", path, page, len(items), limit)
		}
		for _, item := range items {
			id, _ := item["id"].(string)
			if seen[id] {
				t.Fatalf("%s page %d repeated %s", path, page, id)
			}
			seen[id] = true
			ids = append(ids, id)
		}
		var body struct {
			Meta struct {
				NextCursor *string `json:"nextCursor"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(r.Raw, &body); err != nil {
			t.Fatal(err)
		}
		if body.Meta.NextCursor == nil {
			return ids, page
		}
		if len(items) == 0 {
			t.Fatalf("%s page %d is empty but carries a nextCursor", path, page)
		}
		next = *body.Meta.NextCursor
		if page > 20 {
			t.Fatalf("%s pages never ended", path)
		}
	}
}

func sameIDSet(t *testing.T, what string, got []string, want map[string]bool) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: served %d rows, want %d (%v)", what, len(got), len(want), got)
	}
	for _, id := range got {
		if !want[id] {
			t.Fatalf("%s: served unexpected row %s", what, id)
		}
	}
}

func TestWorkforceListsPageByCursorWithinBranchScope(t *testing.T) {
	f := newRows17(t)
	ctx := context.Background()
	confined := rowsActor{Org: f.A.Org, Member: f.A.Member, Branch: f.branchA}

	// Profiles: five in branch A, two in branch X. The test clock is fixed,
	// so every row shares created_at and only the id tie-breaker keeps the
	// pages disjoint.
	profilesA, profilesAll := map[string]bool{}, map[string]bool{}
	for i := 0; i < 7; i++ {
		branch := f.branchA
		if i >= 5 {
			branch = f.branchX
		}
		p, err := f.svc.CreateProfile(ctx, f.tenantA(), workforceusecases.CreateProfileInput{BranchID: branch,
			MembershipID: f.providerMembership(t, f.A.Org), EmployeeNumber: fmt.Sprintf("EMP-CUR-%d", i),
			RelationshipType: "MANAGED_PROVIDER", Role: "DRIVER", StartDate: f.now})
		if err != nil {
			t.Fatal(err)
		}
		profilesAll[p.ID] = true
		if branch == f.branchA {
			profilesA[p.ID] = true
		}
	}
	got, pages := walkWorkforcePages(t, f, f.A, "/fleet-partner/v1/workforce/profiles", 3)
	sameIDSet(t, "profiles (organization)", got, profilesAll)
	if pages != 3 {
		t.Fatalf("7 profiles at 3 per page took %d pages", pages)
	}
	got, _ = walkWorkforcePages(t, f, confined, "/fleet-partner/v1/workforce/profiles", 2)
	sameIDSet(t, "profiles (branch-confined)", got, profilesA)

	// Shifts page by (start_at DESC, id DESC).
	shiftsA := map[string]bool{}
	for i := 0; i < 5; i++ {
		branch := f.branchA
		if i == 4 {
			branch = f.branchX
		}
		s, err := f.svc.CreateShift(ctx, f.tenantA(), workforceusecases.CreateShiftInput{BranchID: branch,
			Name: fmt.Sprintf("Cursor %d", i), Role: "DRIVER", Timezone: "Africa/Lagos",
			StartAt: f.now.Add(time.Duration(i%2) * time.Hour), EndAt: f.now.Add(9 * time.Hour), RequiredCapacity: 1})
		if err != nil {
			t.Fatal(err)
		}
		if branch == f.branchA {
			shiftsA[s.ID] = true
		}
	}
	got, _ = walkWorkforcePages(t, f, confined, "/fleet-partner/v1/shifts", 2)
	sameIDSet(t, "shifts (branch-confined)", got, shiftsA)

	// Shift templates page by (name, id).
	templatesA := map[string]bool{}
	for i := 0; i < 4; i++ {
		branch := f.branchA
		if i == 3 {
			branch = f.branchX
		}
		tpl, err := f.svc.CreateShiftTemplate(ctx, f.tenantA(), workforcemodels.ShiftTemplate{BranchID: branch,
			Name: "Same name", Timezone: "Africa/Lagos", Weekdays: []int{1}, StartLocalTime: "08:00", EndLocalTime: "16:00"})
		if err != nil {
			t.Fatal(err)
		}
		if branch == f.branchA {
			templatesA[tpl.ID] = true
		}
	}
	got, _ = walkWorkforcePages(t, f, confined, "/fleet-partner/v1/shift-templates", 1)
	sameIDSet(t, "shift templates (branch-confined)", got, templatesA)

	// Leave, qualifications, courses and enrolments.
	leave := map[string]bool{}
	member := f.providerMembership(t, f.A.Org)
	for i := 0; i < 3; i++ {
		v, err := f.svc.CreateLeave(ctx, f.tenantA(), workforceusecases.CreateLeaveInput{MembershipID: member,
			LeaveType: "ANNUAL", StartAt: f.now.Add(time.Duration(i) * 24 * time.Hour), EndAt: f.now.Add(time.Duration(i)*24*time.Hour + time.Hour)})
		if err != nil {
			t.Fatal(err)
		}
		leave[v.ID] = true
	}
	got, _ = walkWorkforcePages(t, f, f.A, "/fleet-partner/v1/leave", 2)
	sameIDSet(t, "leave", got, leave)

	qualifications, courses, enrollments := map[string]bool{}, map[string]bool{}, map[string]bool{}
	var firstCourse workforcemodels.Course
	for i := 0; i < 3; i++ {
		q, err := f.svc.CreateQualification(ctx, f.tenantA(), workforcemodels.Qualification{Code: fmt.Sprintf("Q-%d", 2-i), Name: "Q"})
		if err != nil {
			t.Fatal(err)
		}
		qualifications[q.ID] = true
		c, err := f.svc.CreateCourse(ctx, f.tenantA(), workforcemodels.Course{Code: fmt.Sprintf("C-%d", i), Title: "C"})
		if err != nil {
			t.Fatal(err)
		}
		courses[c.ID] = true
		if i == 0 {
			firstCourse = c
		}
	}
	got, _ = walkWorkforcePages(t, f, f.A, "/fleet-partner/v1/qualifications", 2)
	sameIDSet(t, "qualifications", got, qualifications)
	got, _ = walkWorkforcePages(t, f, f.A, "/fleet-partner/v1/training/courses", 2)
	sameIDSet(t, "training courses", got, courses)
	for i := 0; i < 3; i++ {
		e, err := f.svc.Enroll(ctx, f.tenantA(), workforceusecases.EnrollInput{CourseID: firstCourse.ID, MembershipID: f.providerMembership(t, f.A.Org)})
		if err != nil {
			t.Fatal(err)
		}
		enrollments[e.ID] = true
	}
	got, _ = walkWorkforcePages(t, f, f.A, "/fleet-partner/v1/training/enrollments", 2)
	sameIDSet(t, "training enrolments", got, enrollments)

	// Without limit or cursor the catalogues still return every row, in code
	// order, with no continuation.
	all := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/qualifications", nil, nil), 200, "full qualification list")
	if items := all.Items(t); len(items) != 3 || items[0]["code"] != "Q-0" {
		t.Fatalf("unpaged qualification list changed: %v", items)
	}
	var unpaged struct {
		Meta map[string]any `json:"meta"`
	}
	_ = json.Unmarshal(all.Raw, &unpaged)
	if _, present := unpaged.Meta["nextCursor"]; present {
		t.Fatalf("unpaged list carries a cursor: %s", all.Raw)
	}

	// Offset keeps working; offset together with a cursor is refused.
	if items := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/workforce/profiles?limit=3&offset=6", nil, nil), 200, "offset page").Items(t); len(items) != 1 {
		t.Fatalf("offset page served %d rows, want 1", len(items))
	}
	first := rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/workforce/profiles?limit=1", nil, nil), 200, "first page")
	var meta struct {
		Meta struct {
			NextCursor string `json:"nextCursor"`
		} `json:"meta"`
	}
	_ = json.Unmarshal(first.Raw, &meta)
	rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/workforce/profiles?offset=0&cursor="+url.QueryEscape(meta.Meta.NextCursor), nil, nil), 422, "cursor with offset")
	rowsExpect(t, rowsDo(t, f.router, f.A, "GET", "/fleet-partner/v1/workforce/profiles?cursor=not-a-cursor", nil, nil), 422, "malformed cursor")
}
