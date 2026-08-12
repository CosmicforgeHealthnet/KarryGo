package integration_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/assets"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

func concurrentResults[T any](left, right func() (T, error)) ([]T, []error) {
	type result struct {
		value T
		err   error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, run := range []func() (T, error){left, right} {
		wg.Add(1)
		go func(run func() (T, error)) {
			defer wg.Done()
			value, err := run()
			results <- result{value: value, err: err}
		}(run)
	}
	wg.Wait()
	close(results)
	values, errs := []T{}, []error{}
	for result := range results {
		values = append(values, result.value)
		errs = append(errs, result.err)
	}
	return values, errs
}

func requireSuccesses(t *testing.T, want int, errs []error) {
	t.Helper()
	got := 0
	for _, err := range errs {
		if err == nil {
			got++
		}
	}
	if got != want {
		t.Fatalf("successful concurrent operations=%d want=%d errors=%v", got, want, errs)
	}
}

func TestSection17And18Concurrency(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Sections 17 18 concurrency", "S1718C-"+ids.New("reg"))
	branch := ids.New("brn")
	memberA, memberB := ids.New("mem"), ids.New("mem")
	if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_branches(id,organization_id,code,name,timezone)VALUES($1,$2,'S1718C','Concurrency','UTC')`, branch, org.ID); err != nil {
		t.Fatal(err)
	}
	for i, member := range []string{memberA, memberB} {
		if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_organization_provider_memberships(id,organization_id,provider_service,service_category,provider_id,onboarding_source,status)VALUES($1,$2,'driver-dispatch-delivery-service','DISPATCH',$3,'ORGANIZATION_SPONSORED','ACTIVE')`, member, org.ID, "provider-concurrency-"+string(rune('A'+i))); err != nil {
			t.Fatal(err)
		}
	}
	scope := tenant.Context{OrganizationID: org.ID, OrganizationUserID: memberA, ActorType: tenant.ActorTypeOrganizationUser, ActorID: memberA, CorrelationID: "sections-17-18-concurrency"}
	now := time.Now().UTC().Truncate(time.Second)
	publisher := events.NewDomainPublisher(events.NewOutboxRepository(pool))
	w := workforce.NewService(workforce.Options{Pool: pool, Events: publisher, Flags: workforce.Flags{Workforce: true, Shifts: true, Leave: true, Qualifications: true, Training: true, Performance: true, Workers: true}, Now: func() time.Time { return now }})
	if _, err := w.CreateProfile(ctx, scope, workforce.CreateProfileInput{BranchID: branch, MembershipID: memberA, EmployeeNumber: "EMP-CONCURRENCY", RelationshipType: "MANAGED_PROVIDER", Role: "DRIVER", StartDate: now}); err != nil {
		t.Fatal(err)
	}

	shiftA, err := w.CreateShift(ctx, scope, workforce.CreateShiftInput{BranchID: branch, Name: "Overlap A", Role: "DRIVER", StartAt: now.Add(time.Hour), EndAt: now.Add(2 * time.Hour), Timezone: "UTC", RequiredCapacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	shiftB, err := w.CreateShift(ctx, scope, workforce.CreateShiftInput{BranchID: branch, Name: "Overlap B", Role: "DRIVER", StartAt: now.Add(90 * time.Minute), EndAt: now.Add(3 * time.Hour), Timezone: "UTC", RequiredCapacity: 1})
	if err != nil {
		t.Fatal(err)
	}
	shiftA, err = w.UpdateShift(ctx, scope, shiftA.ID, shiftA.Version, "", nil, nil, "PUBLISHED")
	if err != nil {
		t.Fatal(err)
	}
	shiftB, err = w.UpdateShift(ctx, scope, shiftB.ID, shiftB.Version, "", nil, nil, "PUBLISHED")
	if err != nil {
		t.Fatal(err)
	}
	assignments, assignmentErrs := concurrentResults(
		func() (workforce.ShiftAssignment, error) {
			return w.AssignShift(ctx, scope, shiftA.ID, workforce.AssignShiftInput{MembershipID: memberA})
		},
		func() (workforce.ShiftAssignment, error) {
			return w.AssignShift(ctx, scope, shiftB.ID, workforce.AssignShiftInput{MembershipID: memberA})
		},
	)
	requireSuccesses(t, 1, assignmentErrs)
	var assigned workforce.ShiftAssignment
	for i, assignmentErr := range assignmentErrs {
		if assignmentErr == nil {
			assigned = assignments[i]
		}
	}
	_, attendanceErrs := concurrentResults(
		func() (workforce.ShiftAssignment, error) {
			return w.AssignmentAction(ctx, scope, assigned.ID, "CHECKED_IN", "MANUAL", assigned.Version)
		},
		func() (workforce.ShiftAssignment, error) {
			return w.AssignmentAction(ctx, scope, assigned.ID, "CANCELLED", "MANUAL", assigned.Version)
		},
	)
	requireSuccesses(t, 1, attendanceErrs)

	leave, err := w.CreateLeave(ctx, scope, workforce.CreateLeaveInput{MembershipID: memberA, LeaveType: "ANNUAL", StartAt: now.Add(24 * time.Hour), EndAt: now.Add(25 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	_, leaveErrs := concurrentResults(
		func() (workforce.Leave, error) { return w.LeaveAction(ctx, scope, leave.ID, "APPROVED", leave.Version) },
		func() (workforce.Leave, error) { return w.LeaveAction(ctx, scope, leave.ID, "APPROVED", leave.Version) },
	)
	requireSuccesses(t, 1, leaveErrs)

	qualification, err := w.CreateQualification(ctx, scope, workforce.Qualification{Code: "CONCURRENT", Name: "Concurrent qualification"})
	if err != nil {
		t.Fatal(err)
	}
	_, qualificationCreateErrs := concurrentResults(
		func() (workforce.ProviderQualification, error) {
			return w.AddProviderQualification(ctx, scope, memberA, workforce.ProviderQualification{QualificationID: qualification.ID})
		},
		func() (workforce.ProviderQualification, error) {
			return w.AddProviderQualification(ctx, scope, memberA, workforce.ProviderQualification{QualificationID: qualification.ID})
		},
	)
	requireSuccesses(t, 1, qualificationCreateErrs)
	providerQualifications, err := w.ListProviderQualifications(ctx, scope, memberA)
	if err != nil || len(providerQualifications) != 1 {
		t.Fatalf("provider qualifications=%d err=%v", len(providerQualifications), err)
	}
	providerQualification := providerQualifications[0]
	_, verificationErrs := concurrentResults(
		func() (workforce.ProviderQualification, error) {
			return w.QualificationAction(ctx, scope, providerQualification.ID, "VERIFIED", "verification-a", providerQualification.Version)
		},
		func() (workforce.ProviderQualification, error) {
			return w.QualificationAction(ctx, scope, providerQualification.ID, "REJECTED", "", providerQualification.Version)
		},
	)
	requireSuccesses(t, 1, verificationErrs)

	course, err := w.CreateCourse(ctx, scope, workforce.Course{Code: "CONCURRENT", Title: "Concurrent completion", PassingScore: 70})
	if err != nil {
		t.Fatal(err)
	}
	_, completionErrs := concurrentResults(
		func() (workforce.Completion, error) {
			return w.CompleteCourse(ctx, scope, course.ID, workforce.Completion{MembershipID: memberA, Score: 90, CompletedAt: now})
		},
		func() (workforce.Completion, error) {
			return w.CompleteCourse(ctx, scope, course.ID, workforce.Completion{MembershipID: memberA, Score: 90, CompletedAt: now})
		},
	)
	requireSuccesses(t, 1, completionErrs)
	performance, performanceErrs := concurrentResults(
		func() (workforce.PerformanceSnapshot, error) {
			return w.GeneratePerformance(ctx, scope, memberA, now.Add(-time.Hour), now)
		},
		func() (workforce.PerformanceSnapshot, error) {
			return w.GeneratePerformance(ctx, scope, memberA, now.Add(-time.Hour), now)
		},
	)
	requireSuccesses(t, 2, performanceErrs)
	if performance[0].ID != performance[1].ID {
		t.Fatalf("performance replay IDs differ: %s %s", performance[0].ID, performance[1].ID)
	}

	a := assets.NewService(assets.Options{Pool: pool, Events: publisher, Flags: assets.Flags{Assets: true, Assignments: true, Inspections: true, Maintenance: true, Breakdowns: true, Inventory: true, Fuel: true, Workers: true}, Now: func() time.Time { return now }})
	asset, err := a.CreateAsset(ctx, scope, assets.Asset{BranchID: branch, AssetCode: "CONCURRENT-ASSET", AssetType: "TRUCK", OwnershipType: "OWNED"})
	if err != nil {
		t.Fatal(err)
	}
	_, assetAssignmentErrs := concurrentResults(
		func() (assets.Assignment, error) {
			return a.Assign(ctx, scope, asset.ID, assets.Assignment{ProviderMembershipID: memberA, AssignmentType: "PRIMARY", StartAt: now})
		},
		func() (assets.Assignment, error) {
			return a.Assign(ctx, scope, asset.ID, assets.Assignment{ProviderMembershipID: memberB, AssignmentType: "PRIMARY", StartAt: now})
		},
	)
	requireSuccesses(t, 1, assetAssignmentErrs)

	template, err := a.SaveTemplate(ctx, scope, "", 0, assets.Template{Name: "Concurrent safety", ChecklistItems: json.RawMessage(`[{"code":"brakes","required":true,"critical":true}]`), CriticalityRules: json.RawMessage(`{"criticalFailure":"BLOCK_ASSET"}`)})
	if err != nil {
		t.Fatal(err)
	}
	inspection, err := a.CreateInspection(ctx, scope, assets.Inspection{AssetID: asset.ID, TemplateID: template.ID, InspectorMembershipID: memberA})
	if err != nil {
		t.Fatal(err)
	}
	complete := func() (assets.Inspection, error) {
		return a.CompleteInspection(ctx, scope, inspection.ID, assets.CompleteInspectionInput{ExpectedVersion: inspection.Version, Answers: []assets.Answer{{ItemCode: "brakes", Passed: false}}, DefectSummary: json.RawMessage(`[{"code":"brakes"}]`)})
	}
	_, inspectionErrs := concurrentResults(complete, complete)
	requireSuccesses(t, 1, inspectionErrs)
	var workOrder assets.WorkOrder
	workOrders, err := a.ListWorkOrders(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range workOrders {
		if candidate.SourceType == "INSPECTION" && candidate.SourceReference == inspection.ID {
			workOrder = candidate
		}
	}
	if workOrder.ID == "" {
		t.Fatal("critical inspection did not create work order")
	}
	workOrder, err = a.UpdateWorkOrder(ctx, scope, workOrder.ID, "", assets.WorkAction{ExpectedVersion: workOrder.Version, Status: "PENDING_APPROVAL"})
	if err != nil {
		t.Fatal(err)
	}
	_, workOrderErrs := concurrentResults(
		func() (assets.WorkOrder, error) {
			return a.UpdateWorkOrder(ctx, scope, workOrder.ID, "APPROVE", assets.WorkAction{ExpectedVersion: workOrder.Version})
		},
		func() (assets.WorkOrder, error) {
			return a.UpdateWorkOrder(ctx, scope, workOrder.ID, "", assets.WorkAction{ExpectedVersion: workOrder.Version, Status: "CANCELLED"})
		},
	)
	requireSuccesses(t, 1, workOrderErrs)

	_, fuelErrs := concurrentResults(
		func() (assets.FuelRecord, error) {
			return a.CreateFuel(ctx, scope, assets.FuelRecord{AssetID: asset.ID, ProviderMembershipID: memberA, FuelOrEnergyType: "DIESEL", Quantity: 10, Unit: "L", SupplierReference: "duplicate-receipt", RecordedAt: now})
		},
		func() (assets.FuelRecord, error) {
			return a.CreateFuel(ctx, scope, assets.FuelRecord{AssetID: asset.ID, ProviderMembershipID: memberA, FuelOrEnergyType: "DIESEL", Quantity: 10, Unit: "L", SupplierReference: "duplicate-receipt", RecordedAt: now})
		},
	)
	requireSuccesses(t, 1, fuelErrs)

	archiveAsset, err := a.CreateAsset(ctx, scope, assets.Asset{BranchID: branch, AssetCode: "CONCURRENT-LIFECYCLE", AssetType: "TRUCK", OwnershipType: "OWNED"})
	if err != nil {
		t.Fatal(err)
	}
	patch := assets.Asset{Capacity: -1, Odometer: -1, OperatingHours: -1}
	_, lifecycleErrs := concurrentResults(
		func() (assets.Asset, error) {
			return a.UpdateAsset(ctx, scope, archiveAsset.ID, archiveAsset.Version, patch, "ARCHIVE")
		},
		func() (assets.Asset, error) {
			return a.UpdateAsset(ctx, scope, archiveAsset.ID, archiveAsset.Version, patch, "RETIRE")
		},
	)
	requireSuccesses(t, 1, lifecycleErrs)
}
