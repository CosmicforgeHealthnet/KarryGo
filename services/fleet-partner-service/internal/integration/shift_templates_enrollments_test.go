package integration_test

import (
	"context"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// TestShiftTemplatesAndTrainingEnrollments covers spec 17 / 46: weekly
// templates generate shifts in the branch timezone, skip a DST gap instead
// of silently moving the shift, handle overnight shifts, never duplicate a
// day; training enrolments are single-open per course and close through
// the existing completion record.
func TestShiftTemplatesAndTrainingEnrollments(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Templates Co", "TPL-"+ids.New("reg"))
	branch, member := ids.New("brn"), ids.New("mem")
	if _, e := pool.Exec(ctx, `INSERT INTO fleet_partner_branches(id,organization_id,code,name,timezone)VALUES($1,$2,'ROM','Rome','Europe/Rome')`, branch, org.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := pool.Exec(ctx, `INSERT INTO fleet_partner_organization_provider_memberships(id,organization_id,provider_service,service_category,provider_id,onboarding_source,status)VALUES($1,$2,'driver-dispatch-delivery-service','DISPATCH','provider-tpl','ORGANIZATION_SPONSORED','ACTIVE')`, member, org.ID); e != nil {
		t.Fatal(e)
	}
	scope := tenant.Context{OrganizationID: org.ID, OrganizationUserID: member, ActorType: tenant.ActorTypeOrganizationUser, ActorID: member, CorrelationID: "templates-test"}
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	w := workforce.NewService(workforce.Options{Pool: pool, Events: events.NewDomainPublisher(events.NewOutboxRepository(pool)),
		Flags: workforce.Flags{Workforce: true, Shifts: true, Training: true}, Now: func() time.Time { return now }})

	if _, e := w.CreateShiftTemplate(ctx, scope, workforce.ShiftTemplate{BranchID: branch, Name: "Bad", Timezone: "Europe/Rome", Weekdays: []int{9}, StartLocalTime: "08:00", EndLocalTime: "16:00"}); e == nil {
		t.Fatal("weekday 9 accepted")
	}
	// Sunday 02:30-06:00: on 2026-03-29 Rome jumps 02:00 -> 03:00, so 02:30 does not exist.
	night, e := w.CreateShiftTemplate(ctx, scope, workforce.ShiftTemplate{BranchID: branch, Name: "Sunday early", Role: "DRIVER", Timezone: "Europe/Rome", Weekdays: []int{7, 7}, StartLocalTime: "02:30", EndLocalTime: "06:00"})
	if e != nil || len(night.Weekdays) != 1 {
		t.Fatalf("template: %+v %v", night, e)
	}
	generated, e := w.GenerateShifts(ctx, scope, night.ID, workforce.GenerateInput{From: "2026-03-22", To: "2026-04-05"})
	if e != nil {
		t.Fatal(e)
	}
	if len(generated.Created) != 2 || len(generated.SkippedDays) != 1 || generated.SkippedDays[0] != "2026-03-29" {
		t.Fatalf("DST generation: created=%d skipped=%v", len(generated.Created), generated.SkippedDays)
	}
	// 2026-03-22 is before DST (UTC+1): 02:30 local = 01:30 UTC.
	if got := generated.Created[0].StartAt.UTC(); got.Hour() != 1 || got.Minute() != 30 {
		t.Fatalf("first shift start %v", got)
	}
	// After DST (UTC+2) the same local time is 00:30 UTC.
	if got := generated.Created[1].StartAt.UTC(); got.Hour() != 0 || got.Minute() != 30 {
		t.Fatalf("post-DST shift start %v", got)
	}
	again, e := w.GenerateShifts(ctx, scope, night.ID, workforce.GenerateInput{From: "2026-03-22", To: "2026-04-05"})
	if e != nil || len(again.Created) != 0 || len(again.AlreadyExist) != 2 {
		t.Fatalf("regeneration duplicated days: %+v %v", again, e)
	}

	// Overnight 22:00-06:00 ends the next morning.
	overnight, e := w.CreateShiftTemplate(ctx, scope, workforce.ShiftTemplate{BranchID: branch, Name: "Night", Timezone: "Europe/Rome", Weekdays: []int{1}, StartLocalTime: "22:00", EndLocalTime: "06:00"})
	if e != nil {
		t.Fatal(e)
	}
	g, e := w.GenerateShifts(ctx, scope, overnight.ID, workforce.GenerateInput{From: "2026-04-06", To: "2026-04-06"})
	if e != nil || len(g.Created) != 1 || g.Created[0].EndAt.Sub(g.Created[0].StartAt) != 8*time.Hour {
		t.Fatalf("overnight: %+v %v", g, e)
	}
	archived, e := w.UpdateShiftTemplate(ctx, scope, overnight.ID, overnight.Version, workforce.TemplatePatch{Status: "archived"})
	if e != nil || archived.Status != "ARCHIVED" {
		t.Fatalf("archive: %+v %v", archived, e)
	}
	if _, e := w.GenerateShifts(ctx, scope, overnight.ID, workforce.GenerateInput{From: "2026-04-13", To: "2026-04-13"}); e == nil {
		t.Fatal("archived template generated shifts")
	}
	if _, e := w.UpdateShiftTemplate(ctx, scope, overnight.ID, 1, workforce.TemplatePatch{Name: "stale"}); e == nil {
		t.Fatal("stale version accepted")
	}
	// Tenant isolation.
	if _, e := w.GetShiftTemplate(ctx, tenant.Context{OrganizationID: "org_other"}, night.ID); e == nil {
		t.Fatal("cross-tenant template read")
	}

	// Training enrolments.
	course, e := w.CreateCourse(ctx, scope, workforce.Course{Code: "SAFE-1", Title: "Safe loading", PassingScore: 70})
	if e != nil {
		t.Fatal(e)
	}
	enrollment, e := w.Enroll(ctx, scope, workforce.EnrollInput{CourseID: course.ID, MembershipID: member})
	if e != nil || enrollment.Status != "ASSIGNED" || enrollment.CourseVersion != course.Version {
		t.Fatalf("enroll: %+v %v", enrollment, e)
	}
	if _, e := w.Enroll(ctx, scope, workforce.EnrollInput{CourseID: course.ID, MembershipID: member}); appStatus(e) != 409 {
		t.Fatalf("second open enrolment = %v", e)
	}
	if _, e := w.Enroll(ctx, scope, workforce.EnrollInput{CourseID: course.ID, MembershipID: "mem_unknown"}); e == nil {
		t.Fatal("unknown member enrolled")
	}
	done, e := w.CompleteEnrollment(ctx, scope, enrollment.ID, workforce.Completion{Score: 55})
	if e != nil || done.Status != "FAILED" || done.CompletionID == nil {
		t.Fatalf("complete: %+v %v", done, e)
	}
	if _, e := w.CompleteEnrollment(ctx, scope, enrollment.ID, workforce.Completion{Score: 90}); appStatus(e) != 409 {
		t.Fatalf("completing twice = %v", e)
	}
	// After a failed attempt a new enrolment can be opened and cancelled.
	retry, e := w.Enroll(ctx, scope, workforce.EnrollInput{CourseID: course.ID, MembershipID: member})
	if e != nil {
		t.Fatal(e)
	}
	if cancelled, e := w.CancelEnrollment(ctx, scope, retry.ID); e != nil || cancelled.Status != "CANCELLED" {
		t.Fatalf("cancel: %+v %v", cancelled, e)
	}
	list, e := w.ListEnrollments(ctx, scope, member, workforce.Page{})
	if e != nil || len(list) != 2 {
		t.Fatalf("list: %d %v", len(list), e)
	}
	var completions int
	if e := pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_training_completions WHERE organization_id=$1`, org.ID).Scan(&completions); e != nil || completions != 1 {
		t.Fatalf("completion records = %d %v", completions, e)
	}
	_ = apperrors.NotFound
}
