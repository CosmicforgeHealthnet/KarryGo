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
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

type sectionMedia struct {
	files map[string]platformclients.FileMetadata
}

func (m sectionMedia) Configured() bool { return true }
func (m sectionMedia) GetFile(_ context.Context, _ platformclients.CallMeta, id string) (platformclients.FileMetadata, error) {
	return m.files[id], nil
}
func (m sectionMedia) AuthorizeUpload(_ context.Context, _ platformclients.CallMeta, _ platformclients.MediaUploadRequest) (platformclients.MediaUploadGrant, error) {
	return platformclients.MediaUploadGrant{}, nil
}
func (m sectionMedia) CompleteUpload(_ context.Context, _ platformclients.CallMeta, _ platformclients.MediaUploadGrant, _, _ string, _ []byte) (platformclients.MediaObjectState, error) {
	return platformclients.MediaObjectState{}, nil
}
func (m sectionMedia) ReadObject(_ context.Context, _ platformclients.CallMeta, _ string) (platformclients.MediaObjectState, error) {
	return platformclients.MediaObjectState{}, nil
}
func (m sectionMedia) AuthorizeDownload(_ context.Context, _ platformclients.CallMeta, _ string) (platformclients.MediaDownloadGrant, error) {
	return platformclients.MediaDownloadGrant{}, nil
}
func (m sectionMedia) ReadContent(_ context.Context, _ platformclients.CallMeta, _ platformclients.MediaDownloadGrant, _ int64) ([]byte, error) {
	return nil, nil
}
func (m sectionMedia) RequestDeletion(_ context.Context, _ platformclients.CallMeta, _, _ string) (platformclients.MediaDeletionOutcome, error) {
	return platformclients.MediaDeletionOutcome{}, nil
}

func TestSection17And18EndToEnd(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Sections 17 18 E2E", "S1718-"+ids.New("reg"))
	branch := ids.New("brn")
	member := ids.New("mem")
	if _, e := pool.Exec(ctx, `INSERT INTO fleet_partner_branches(id,organization_id,code,name,timezone)VALUES($1,$2,'S1718','Operations','Europe/Rome')`, branch, org.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := pool.Exec(ctx, `INSERT INTO fleet_partner_organization_provider_memberships(id,organization_id,provider_service,service_category,provider_id,onboarding_source,status)VALUES($1,$2,'driver-dispatch-delivery-service','DISPATCH','provider-1718','ORGANIZATION_SPONSORED','ACTIVE')`, member, org.ID); e != nil {
		t.Fatal(e)
	}
	scope := tenant.Context{OrganizationID: org.ID, OrganizationUserID: member, ActorType: tenant.ActorTypeOrganizationUser, ActorID: member, CorrelationID: "sections-17-18-test"}
	media := sectionMedia{files: map[string]platformclients.FileMetadata{
		"member-clean":   {FileID: "member-clean", OwnerID: member, UploadState: "COMPLETE", ScanState: "CLEAN"},
		"org-clean":      {FileID: "org-clean", OwnerID: org.ID, UploadState: "COMPLETE", ScanState: "CLEAN"},
		"other-clean":    {FileID: "other-clean", OwnerID: "different-owner", UploadState: "COMPLETE", ScanState: "CLEAN"},
		"org-quarantine": {FileID: "org-quarantine", OwnerID: org.ID, UploadState: "COMPLETE", ScanState: "QUARANTINED"},
	}}
	publisher := events.NewDomainPublisher(events.NewOutboxRepository(pool))
	now := time.Now().UTC().Truncate(time.Second)
	w := workforce.NewService(workforce.Options{Pool: pool, Events: publisher, Media: media, Flags: workforce.Flags{Workforce: true, Shifts: true, Leave: true, Qualifications: true, Training: true, Performance: true, Workers: true}, Now: func() time.Time { return now }})
	p, e := w.CreateProfile(ctx, scope, workforce.CreateProfileInput{BranchID: branch, MembershipID: member, EmployeeNumber: "EMP-1718", RelationshipType: "MANAGED_PROVIDER", Role: "DRIVER", StartDate: now})
	if e != nil {
		t.Fatal(e)
	}
	if p.Status != "ACTIVE" {
		t.Fatal(p.Status)
	}
	if _, e = w.CreateProfile(ctx, scope, workforce.CreateProfileInput{BranchID: branch, MembershipID: member, EmployeeNumber: "EMP-1718", RelationshipType: "MANAGED_PROVIDER", Role: "DRIVER", StartDate: now}); e == nil {
		t.Fatal("duplicate workforce identity accepted")
	}
	shift, e := w.CreateShift(ctx, scope, workforce.CreateShiftInput{BranchID: branch, Name: "Day", Role: "DRIVER", Timezone: "Europe/Rome", StartAt: now.Add(-time.Hour), EndAt: now.Add(7 * time.Hour), RequiredCapacity: 1})
	if e != nil {
		t.Fatal(e)
	}
	shift, e = w.UpdateShift(ctx, scope, shift.ID, shift.Version, "", nil, nil, "PUBLISHED")
	if e != nil {
		t.Fatal(e)
	}
	assignment, e := w.AssignShift(ctx, scope, shift.ID, workforce.AssignShiftInput{MembershipID: member, AssignmentType: "PRIMARY"})
	if e != nil {
		t.Fatal(e)
	}
	assignment, e = w.AssignmentAction(ctx, scope, assignment.ID, "CHECKED_IN", "MANUAL", assignment.Version)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = w.AssignmentAction(ctx, scope, assignment.ID, "CHECKED_OUT", "MANUAL", assignment.Version); e != nil {
		t.Fatal(e)
	}
	leave, e := w.CreateLeave(ctx, scope, workforce.CreateLeaveInput{MembershipID: member, LeaveType: "ANNUAL", Reason: "planned", StartAt: now.Add(24 * time.Hour), EndAt: now.Add(48 * time.Hour)})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = w.LeaveAction(ctx, scope, leave.ID, "APPROVED", leave.Version); e != nil {
		t.Fatal(e)
	}
	if _, e = w.AssignShift(ctx, scope, shift.ID, workforce.AssignShiftInput{MembershipID: member, ExpectedStart: now.Add(25 * time.Hour), ExpectedEnd: now.Add(26 * time.Hour)}); e == nil {
		t.Fatal("approved leave did not block assignment")
	}
	q, e := w.CreateQualification(ctx, scope, workforce.Qualification{Code: "LIC", Name: "Licence", ApplicableServices: []string{"DISPATCH"}})
	if e != nil {
		t.Fatal(e)
	}
	pq, e := w.AddProviderQualification(ctx, scope, member, workforce.ProviderQualification{QualificationID: q.ID})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = w.QualificationAction(ctx, scope, pq.ID, "VERIFIED", "verification-1", pq.Version); e != nil {
		t.Fatal(e)
	}
	evidenceQualification, e := w.CreateQualification(ctx, scope, workforce.Qualification{Code: "EVIDENCE", Name: "Evidence ownership", EvidenceRequired: true})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = w.AddProviderQualification(ctx, scope, member, workforce.ProviderQualification{QualificationID: evidenceQualification.ID, EvidenceMediaID: "other-clean"}); e == nil {
		t.Fatal("foreign qualification evidence accepted")
	}
	if _, e = w.AddProviderQualification(ctx, scope, member, workforce.ProviderQualification{QualificationID: evidenceQualification.ID, EvidenceMediaID: "member-clean"}); e != nil {
		t.Fatal(e)
	}
	course, e := w.CreateCourse(ctx, scope, workforce.Course{Code: "SAFE", Title: "Safety", RequiredServices: []string{"DISPATCH"}, PassingScore: 70})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = w.CompleteCourse(ctx, scope, course.ID, workforce.Completion{MembershipID: member, Score: 90, CompletedAt: now}); e != nil {
		t.Fatal(e)
	}
	elig, e := w.Evaluate(ctx, org.ID, member, "DISPATCH", "", now.Add(8*time.Hour))
	if e != nil || !elig.Eligible {
		t.Fatalf("workforce eligibility=%+v err=%v", elig, e)
	}
	if _, e = pool.Exec(ctx, `INSERT INTO fleet_partner_private_assignments(id,organization_id,branch_id,work_origin,route_stop_id,provider_membership_id,provider_id,execution_service,status,response_deadline,decline_reason,declined_at,created_by,created_at)VALUES($1,$2,$3,'ROUTE_STOP','stop-perf',$4,'provider-1718','DISPATCH','DECLINED',now()+interval '1 hour','UNSAFE_VEHICLE',now(),$4,$5)`, ids.New("pas"), org.ID, branch, member, now); e != nil {
		t.Fatal(e)
	}
	snap, e := w.GeneratePerformance(ctx, scope, member, now.Add(-time.Hour), now.Add(time.Hour))
	if e != nil {
		t.Fatal(e)
	}
	again, e := w.GeneratePerformance(ctx, scope, member, now.Add(-time.Hour), now.Add(time.Hour))
	if e != nil || again.ID != snap.ID {
		t.Fatalf("performance replay %+v %v", again, e)
	}
	if _, e = pool.Exec(ctx, `UPDATE fleet_partner_performance_snapshots SET version=version+1 WHERE id=$1`, snap.ID); e == nil {
		t.Fatal("performance snapshot append-only trigger allowed mutation")
	}
	var explanation map[string]any
	_ = json.Unmarshal(snap.Explanation, &explanation)
	if explanation["disciplinaryAction"] != false || explanation["safetyDeclinesAreMisconduct"] != false {
		t.Fatal(explanation)
	}

	a := assets.NewService(assets.Options{Pool: pool, Events: publisher, Media: media, Flags: assets.Flags{Assets: true, Assignments: true, Inspections: true, Maintenance: true, Breakdowns: true, Inventory: true, Fuel: true, Workers: true}, Now: func() time.Time { return now }})
	asset, e := a.CreateAsset(ctx, scope, assets.Asset{BranchID: branch, AssetCode: "TRUCK-1", AssetType: "TRUCK", OwnershipType: "OWNED", Capacity: 100, CapacityUnit: "KG"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = a.Assign(ctx, scope, asset.ID, assets.Assignment{ProviderMembershipID: member, AssignmentType: "PRIMARY", StartAt: now}); e != nil {
		t.Fatal(e)
	}
	tpl, e := a.SaveTemplate(ctx, scope, "", 0, assets.Template{Name: "Safety", ChecklistItems: json.RawMessage(`[{"code":"brakes","required":true,"critical":true}]`), CriticalityRules: json.RawMessage(`{"criticalFailure":"BLOCK_ASSET"}`)})
	if e != nil {
		t.Fatal(e)
	}
	inspection, e := a.CreateInspection(ctx, scope, assets.Inspection{AssetID: asset.ID, TemplateID: tpl.ID, InspectorMembershipID: member})
	if e != nil {
		t.Fatal(e)
	}
	inspection, e = a.CompleteInspection(ctx, scope, inspection.ID, assets.CompleteInspectionInput{ExpectedVersion: inspection.Version, Answers: []assets.Answer{{ItemCode: "brakes", Passed: false}}, DefectSummary: json.RawMessage(`[{"code":"brakes"}]`)})
	if e != nil {
		t.Fatal(e)
	}
	if inspection.OverallResult != "CRITICAL_FAIL" {
		t.Fatal(inspection.OverallResult)
	}
	if _, e = pool.Exec(ctx, `UPDATE fleet_partner_inspection_results SET result='PASS' WHERE inspection_id=$1`, inspection.ID); e == nil {
		t.Fatal("inspection result append-only trigger allowed mutation")
	}
	blocked, e := a.GetAsset(ctx, scope, asset.ID)
	if e != nil || blocked.OperationalStatus != "BLOCKED" {
		t.Fatalf("asset=%+v err=%v", blocked, e)
	}
	decision, e := a.Evaluate(ctx, org.ID, asset.ID, member, "TRUCK", 50, "DISPATCH", now)
	if e != nil || decision.Eligible {
		t.Fatalf("blocked eligibility=%+v err=%v", decision, e)
	}
	var workID string
	var workVersion int64
	if e = pool.QueryRow(ctx, `UPDATE fleet_partner_work_orders SET status='COMPLETED',completed_at=now(),evidence_media_ids='["proof"]',version=version+1 WHERE organization_id=$1 AND source_type='INSPECTION' AND source_reference=$2 RETURNING id,version`, org.ID, inspection.ID).Scan(&workID, &workVersion); e != nil {
		t.Fatal(e)
	}
	followup, e := a.CreateInspection(ctx, scope, assets.Inspection{AssetID: asset.ID, TemplateID: tpl.ID, InspectorMembershipID: member})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = a.CompleteInspection(ctx, scope, followup.ID, assets.CompleteInspectionInput{ExpectedVersion: followup.Version, Answers: []assets.Answer{{ItemCode: "brakes", Passed: true}}, DefectSummary: json.RawMessage(`[]`)}); e != nil {
		t.Fatal(e)
	}
	if _, e = a.ReturnToService(ctx, scope, workID, assets.WorkAction{ExpectedVersion: workVersion, Reason: "repairs verified"}); e != nil {
		t.Fatal(e)
	}
	available, _ := a.GetAsset(ctx, scope, asset.ID)
	if available.OperationalStatus != "ASSIGNED" {
		t.Fatal(available.OperationalStatus)
	}
	returned, e := a.Evaluate(ctx, org.ID, asset.ID, member, "TRUCK", 50, "DISPATCH", now)
	if e != nil || !returned.Eligible {
		t.Fatalf("returned asset eligibility=%+v err=%v", returned, e)
	}
	part, e := a.SavePart(ctx, scope, "", 0, assets.Part{BranchID: branch, SKU: "FILTER-1", Name: "Filter", StockUnit: "EA", ReorderLevel: 5})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = a.CreateMovement(ctx, scope, assets.StockMovement{PartID: part.ID, LocationReference: "MAIN", MovementType: "RECEIPT", Quantity: 10, Reason: "opening", CorrelationID: "stock-receipt"}); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, x := a.CreateMovement(ctx, scope, assets.StockMovement{PartID: part.ID, LocationReference: "MAIN", MovementType: "ISSUE", Quantity: 3, Reason: "work", CorrelationID: string(rune('A' + i))})
			errs <- x
		}(i)
	}
	wg.Wait()
	close(errs)
	for x := range errs {
		if x != nil {
			t.Fatal(x)
		}
	}
	part, _ = a.GetPart(ctx, scope, part.ID)
	if part.CurrentQuantityProjection != 4 {
		t.Fatalf("stock=%v", part.CurrentQuantityProjection)
	}
	if _, e = pool.Exec(ctx, `UPDATE fleet_partner_stock_movements SET reason='tampered' WHERE part_id=$1`, part.ID); e == nil {
		t.Fatal("stock movement append-only trigger allowed mutation")
	}
	var reorderEvents int
	if e = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_outbox WHERE event_type='part.reorder_required.v1' AND aggregate_id=$1`, part.ID).Scan(&reorderEvents); e != nil || reorderEvents != 1 {
		t.Fatalf("reorder events=%d err=%v", reorderEvents, e)
	}
	fuel, e := a.CreateFuel(ctx, scope, assets.FuelRecord{AssetID: asset.ID, ProviderMembershipID: member, FuelOrEnergyType: "DIESEL", Quantity: 20, Unit: "L", RecordedAt: now})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = a.CreateFuel(ctx, scope, assets.FuelRecord{AssetID: asset.ID, ProviderMembershipID: member, FuelOrEnergyType: "DIESEL", Quantity: 0, Unit: "L", RecordedAt: now}); e == nil {
		t.Fatal("invalid fuel accepted")
	}
	if _, e = a.CreateFuel(ctx, scope, assets.FuelRecord{AssetID: asset.ID, ProviderMembershipID: member, FuelOrEnergyType: "DIESEL", Quantity: 5, Unit: "L", ReceiptMediaID: "other-clean", SupplierReference: "foreign-receipt", RecordedAt: now}); e == nil {
		t.Fatal("foreign fuel receipt accepted")
	}
	if _, e = a.CreateFuel(ctx, scope, assets.FuelRecord{AssetID: asset.ID, ProviderMembershipID: member, FuelOrEnergyType: "DIESEL", Quantity: 5, Unit: "L", ReceiptMediaID: "org-quarantine", SupplierReference: "quarantined-receipt", RecordedAt: now}); e == nil {
		t.Fatal("unsafe fuel receipt accepted")
	}
	if _, e = a.CreateFuel(ctx, scope, assets.FuelRecord{AssetID: asset.ID, ProviderMembershipID: member, FuelOrEnergyType: "DIESEL", Quantity: 5, Unit: "L", ReceiptMediaID: "org-clean", SupplierReference: "safe-receipt", RecordedAt: now}); e != nil {
		t.Fatal(e)
	}
	restricted := scope
	restricted.PermittedBranchIDs = []string{ids.New("brn")}
	for name, read := range map[string]func() error{
		"asset":      func() error { _, x := a.GetAsset(ctx, restricted, asset.ID); return x },
		"inspection": func() error { _, x := a.GetInspection(ctx, restricted, inspection.ID); return x },
		"work order": func() error { _, x := a.GetWorkOrder(ctx, restricted, workID); return x },
		"part":       func() error { _, x := a.GetPart(ctx, restricted, part.ID); return x },
		"fuel":       func() error { _, x := a.GetFuel(ctx, restricted, fuel.ID); return x },
	} {
		if x := read(); x == nil {
			t.Fatalf("branch-scoped %s was exposed", name)
		}
	}
	if list, x := a.ListWorkOrders(ctx, restricted); x != nil || len(list) != 0 {
		t.Fatalf("branch-scoped work orders leaked: %d, %v", len(list), x)
	}
	if list, x := a.ListMovements(ctx, restricted); x != nil || len(list) != 0 {
		t.Fatalf("branch-scoped stock movements leaked: %d, %v", len(list), x)
	}

	// Exercise the tenant-scoped maintenance workers and their idempotent
	// projections, not just the synchronous command paths.
	expiredAt := now.Add(-time.Minute)
	issuedAt := now.Add(-time.Hour)
	expiringQualification, e := w.CreateQualification(ctx, scope, workforce.Qualification{Code: "EXP", Name: "Expiring qualification"})
	if e != nil {
		t.Fatal(e)
	}
	expiringProviderQualification, e := w.AddProviderQualification(ctx, scope, member, workforce.ProviderQualification{QualificationID: expiringQualification.ID, IssuedAt: &issuedAt, ExpiresAt: &expiredAt})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = w.QualificationAction(ctx, scope, expiringProviderQualification.ID, "VERIFIED", "verification-expired", expiringProviderQualification.Version); e != nil {
		t.Fatal(e)
	}
	oneSecond := int64(1)
	expiringCourse, e := w.CreateCourse(ctx, scope, workforce.Course{Code: "EXP-TRAIN", Title: "Expiring training", ValidityPeriodSeconds: &oneSecond, PassingScore: 70})
	if e != nil {
		t.Fatal(e)
	}
	expiringCompletion, e := w.CompleteCourse(ctx, scope, expiringCourse.ID, workforce.Completion{MembershipID: member, Score: 90, CompletedAt: now.Add(-time.Minute)})
	if e != nil {
		t.Fatal(e)
	}
	pastShiftID, pastAssignmentID := ids.New("sft"), ids.New("sfa")
	if _, e = pool.Exec(ctx, `INSERT INTO fleet_partner_shifts(id,organization_id,branch_id,name,role,start_at,end_at,timezone,required_capacity,status,planner_user_id)VALUES($1,$2,$3,'Past shift','DRIVER',$4,$5,'Europe/Rome',1,'PUBLISHED',$6)`, pastShiftID, org.ID, branch, now.Add(-2*time.Hour), now.Add(-time.Hour), member); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, `INSERT INTO fleet_partner_shift_assignments(id,organization_id,shift_id,membership_id,assignment_type,status,expected_start,expected_end)VALUES($1,$2,$3,$4,'PRIMARY','CONFIRMED',$5,$6)`, pastAssignmentID, org.ID, pastShiftID, member, now.Add(-2*time.Hour), now.Add(-time.Hour)); e != nil {
		t.Fatal(e)
	}
	if e = w.RunMaintenance(ctx, org.ID); e != nil {
		t.Fatal(e)
	}
	var noShowStatus, qualificationStatus, completionStatus string
	if e = pool.QueryRow(ctx, `SELECT(SELECT status FROM fleet_partner_shift_assignments WHERE id=$1),(SELECT status FROM fleet_partner_provider_qualifications WHERE id=$2),(SELECT status FROM fleet_partner_training_completions WHERE id=$3)`, pastAssignmentID, expiringProviderQualification.ID, expiringCompletion.ID).Scan(&noShowStatus, &qualificationStatus, &completionStatus); e != nil {
		t.Fatal(e)
	}
	if noShowStatus != "NO_SHOW" || qualificationStatus != "EXPIRED" || completionStatus != "EXPIRED" {
		t.Fatalf("worker statuses noShow=%s qualification=%s training=%s", noShowStatus, qualificationStatus, completionStatus)
	}

	if _, e = pool.Exec(ctx, `INSERT INTO fleet_partner_asset_documents(id,organization_id,asset_id,media_file_id,document_type,expires_at,verification_status)VALUES($1,$2,$3,'media-expired','REGISTRATION',$4,'VERIFIED')`, ids.New("asd"), org.ID, asset.ID, expiredAt); e != nil {
		t.Fatal(e)
	}
	pastDate := now.Add(-time.Hour)
	dueOdometer, dueHours := float64(0), float64(0)
	for _, schedule := range []assets.Schedule{
		{AssetID: asset.ID, TriggerType: "DATE", NextDueDate: &pastDate},
		{AssetID: asset.ID, TriggerType: "MILEAGE", NextDueOdometer: &dueOdometer},
		{AssetID: asset.ID, TriggerType: "OPERATING_HOURS", NextDueHours: &dueHours},
		{AssetID: asset.ID, TriggerType: "COMBINED", NextDueDate: &pastDate, NextDueOdometer: &dueOdometer, NextDueHours: &dueHours},
	} {
		if _, e = a.SaveSchedule(ctx, scope, "", 0, schedule); e != nil {
			t.Fatal(e)
		}
	}
	if e = a.RunMaintenance(ctx, org.ID); e != nil {
		t.Fatal(e)
	}
	if e = a.RunMaintenance(ctx, org.ID); e != nil {
		t.Fatal(e)
	}
	var expiredDocuments, scheduleOrders int
	if e = pool.QueryRow(ctx, `SELECT(SELECT count(*) FROM fleet_partner_asset_documents WHERE organization_id=$1 AND verification_status='EXPIRED'),(SELECT count(*) FROM fleet_partner_work_orders WHERE organization_id=$1 AND source_type='MAINTENANCE_SCHEDULE')`, org.ID).Scan(&expiredDocuments, &scheduleOrders); e != nil {
		t.Fatal(e)
	}
	if expiredDocuments != 1 || scheduleOrders != 4 {
		t.Fatalf("asset worker expiredDocuments=%d scheduleOrders=%d", expiredDocuments, scheduleOrders)
	}

	unsafeID := ids.New("pas")
	if _, e = pool.Exec(ctx, `INSERT INTO fleet_partner_private_assignments(id,organization_id,branch_id,work_origin,route_stop_id,provider_membership_id,provider_id,vehicle_or_truck_reference,execution_service,status,response_deadline,decline_reason,decline_note,declined_at,created_by)VALUES($1,$2,$3,'ROUTE_STOP','stop-unsafe',$4,'provider-1718',$5,'DISPATCH','DECLINED',now()+interval '1 hour','UNSAFE_VEHICLE','brake concern',now(),$4)`, unsafeID, org.ID, branch, member, asset.ID); e != nil {
		t.Fatal(e)
	}
	if e = a.ReconcileSafetyDeclines(ctx); e != nil {
		t.Fatal(e)
	}
	if e = a.ReconcileSafetyDeclines(ctx); e != nil {
		t.Fatal(e)
	}
	var inbox, inspections, orders int
	if e = pool.QueryRow(ctx, `SELECT(SELECT count(*) FROM fleet_partner_safety_decline_inbox WHERE assignment_id=$1),(SELECT count(*) FROM fleet_partner_inspections WHERE assignment_id=$1),(SELECT count(*) FROM fleet_partner_work_orders WHERE source_type='PRIVATE_ASSIGNMENT' AND source_reference=$1)`, unsafeID).Scan(&inbox, &inspections, &orders); e != nil {
		t.Fatal(e)
	}
	if inbox != 1 || inspections != 1 || orders != 1 {
		t.Fatalf("unsafe decline inbox=%d inspections=%d orders=%d", inbox, inspections, orders)
	}
}