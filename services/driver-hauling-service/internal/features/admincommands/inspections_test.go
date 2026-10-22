package admincommands

import (
	"context"
	"testing"

	"cosmicforge/logistics/services/hauling-service/internal/features/restrictions"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/adminowner"
	fixture "cosmicforge/logistics/shared/go/adminowner/testfixture"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"github.com/google/uuid"
)

func TestPostgresInspectionLifecycleFailRestrictsTruckAndServesCaseAuthority(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	w.allow("trucks", w.truck, w.scope)
	inspection := "insp-" + uuid.NewString()
	first, second := uuid.NewString(), uuid.NewString()
	record := w.command("trucks", w.truck, "RECORD_INSPECTION", inspection, 0, map[string]string{"inspection_kind": "ROADWORTHINESS", "performed_on": "2026-10-01", "media_object_id": first})
	r := w.must(record)
	if r.PriorState != "NONE" || r.ResultingState != "RECORDED" || r.RecordedVersion != 1 {
		t.Fatal("record receipt", r)
	}
	// A case is created once; a second RECORD of the same case_ref conflicts.
	w.expect(w.command("trucks", w.truck, "RECORD_INSPECTION", inspection, 0, map[string]string{"inspection_kind": "CARGO_BAY", "performed_on": "2026-10-01"}), admincommand.ErrConflict)
	// Future-dated inspection is not a fact.
	w.expect(w.command("trucks", w.truck, "RECORD_INSPECTION", "insp-"+uuid.NewString(), 0, map[string]string{"inspection_kind": "CARGO_BAY", "performed_on": "2099-01-01"}), admincommand.ErrRejected)

	// Evidence association: unique per inspection, immutable.
	w.expect(w.command("trucks", w.truck, "ATTACH_INSPECTION_EVIDENCE", inspection, 1, map[string]string{"media_object_id": first}), admincommand.ErrRejected)
	w.must(w.command("trucks", w.truck, "ATTACH_INSPECTION_EVIDENCE", inspection, 1, map[string]string{"media_object_id": second}))
	w.expect(w.command("trucks", w.truck, "ATTACH_INSPECTION_EVIDENCE", inspection, 1, map[string]string{"media_object_id": uuid.NewString()}), admincommand.ErrConflict)
	if _, err := w.db.Exec(ctx, `INSERT INTO hauling_admin_inspection_evidence(inspection_id,media_object_id,attached_by) VALUES($1,$2,$3)`, inspection, second, uuid.NewString()); err == nil {
		t.Fatal("duplicate media association accepted")
	}
	if _, err := w.db.Exec(ctx, `DELETE FROM hauling_admin_inspection_evidence`); err == nil || !contains(err.Error(), "append-only") {
		t.Fatal("evidence association mutable", err)
	}
	if w.scalar(`SELECT evidence_count::text FROM hauling_admin_inspections WHERE id=$1`, inspection) != "2" {
		t.Fatal("evidence count")
	}

	// Case authority proof: the hauling inspection case is a real native case
	// Media can ask about (CASE and TRUCK subjects), only within its tenant.
	t.Run("case-authority", func(t *testing.T) {
		fixture.CaseBoundary(t, w.db, w.source, "driver-hauling-service", "inspections", inspection, true)
	})
	tx, _ := w.db.Begin(ctx)
	item, err := w.source.Read(ctx, tx, "inspections", inspection, true)
	binding, bindErr := w.source.Binding(ctx, tx, "inspections", inspection)
	tx.Rollback(ctx)
	if err != nil || bindErr != nil {
		t.Fatal(err, bindErr)
	}
	def, _ := adminowner.ForOwner("driver-hauling-service")
	if err := adminowner.ValidateResource(item, def.Definitions["inspections"], true); err != nil || binding.Canonical.Organization != w.orgA || binding.Canonical.Tenant != w.orgA || len(item.Timeline) != 2 {
		t.Fatal("inspection resource", err, binding.Canonical, item)
	}

	// Conclusion requires an independent approval; self-approval is refused.
	conclude := w.command("trucks", w.truck, "CONCLUDE_INSPECTION", inspection, 2, map[string]string{"conclusion": "FAIL"})
	unapproved := conclude
	if _, err := w.svc.Execute(ctx, w.actor, purpose, w.scope, unapproved); err != admincommand.ErrInvalid {
		t.Fatal("conclusion without approval", err)
	}
	self := conclude
	w.approve(&self, "staff-a", w.scope)
	if _, err := w.svc.Execute(ctx, w.actor, purpose, w.scope, self); err != adminpolicy.ErrDenied {
		t.Fatal("self approval", err)
	}
	// FAIL with a validity window is illegal and leaves no side effect.
	bad := w.command("trucks", w.truck, "CONCLUDE_INSPECTION", inspection, 2, map[string]string{"conclusion": "FAIL", "valid_until": "2027-01-01"})
	w.expect(bad, admincommand.ErrRejected)
	if restricted, _ := restrictions.New(w.db).Restricted(ctx, "", "ASSET", w.truck); restricted {
		t.Fatal("rejected conclusion leaked a restriction")
	}
	done := w.must(conclude)
	if done.ResultingState != "CONCLUDED" || done.Outcome != "INSPECTION_FAILED_TRUCK_RESTRICTED" {
		t.Fatal("conclusion receipt", done)
	}
	view, err := restrictions.New(w.db).Read(ctx, w.orgA, "ASSET", w.truck)
	if err != nil || view.State != "APPLIED" || view.SourceReference != "karryam-admin-inspection:"+inspection || view.OrganizationID != w.orgA {
		t.Fatal("FAIL did not restrict the truck under its owning organization", err, view)
	}
	// Terminal: no further conclusion or evidence.
	w.expect(w.command("trucks", w.truck, "CONCLUDE_INSPECTION", inspection, 3, map[string]string{"conclusion": "PASS"}), admincommand.ErrRejected)
	w.expect(w.command("trucks", w.truck, "ATTACH_INSPECTION_EVIDENCE", inspection, 3, map[string]string{"media_object_id": uuid.NewString()}), admincommand.ErrRejected)
	if _, err := w.db.Exec(ctx, `UPDATE hauling_admin_inspections SET conclusion='PASS',version=version+1 WHERE id=$1`, inspection); err == nil {
		t.Fatal("concluded inspection mutable")
	}
	// The truck's standing advanced with the restriction.
	tx, _ = w.db.Begin(ctx)
	v, state, err := w.exec.Current(ctx, tx, "trucks", w.truck)
	tx.Rollback(ctx)
	if err != nil || v != 1 || state != "RESTRICTED" {
		t.Fatal("truck standing", v, state, err)
	}
	// A PASS on a new inspection never lifts the restriction.
	pass := "insp-" + uuid.NewString()
	w.must(w.command("trucks", w.truck, "RECORD_INSPECTION", pass, 0, map[string]string{"inspection_kind": "ROADWORTHINESS", "performed_on": "2026-10-02"}))
	w.must(w.command("trucks", w.truck, "CONCLUDE_INSPECTION", pass, 1, map[string]string{"conclusion": "PASS", "valid_until": "2027-10-02"}))
	if restricted, _ := restrictions.New(w.db).Restricted(ctx, "", "ASSET", w.truck); !restricted {
		t.Fatal("PASS lifted a restriction")
	}
	// The inspection-applied restriction is Hauling's, so LIFT clears it.
	lift := w.must(w.command("trucks", w.truck, "LIFT_TRUCK_RESTRICTION", "", 1, nil))
	if lift.ResultingState != "ACTIVE" {
		t.Fatal("lift", lift)
	}
}

func TestPostgresInspectionCrossTenantAndRevokedOwnership(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	// A context for org B on org A's truck (trucks carry no canonical tenant)
	// still cannot act: the truck's owning organization is org A.
	w.allow("trucks", w.truck, w.otherScope)
	c := w.command("trucks", w.truck, "RECORD_INSPECTION", "insp-"+uuid.NewString(), 0, map[string]string{"inspection_kind": "CARGO_BAY", "performed_on": "2026-10-01"})
	if _, err := w.svc.Execute(ctx, w.otherActor, purpose, w.otherScope, c); err != adminpolicy.ErrDenied {
		t.Fatal("foreign tenant recorded an inspection", err)
	}
	// No context at all: unmapped.
	if _, err := w.run(w.command("trucks", w.truckB, "RECORD_INSPECTION", "insp-"+uuid.NewString(), 0, map[string]string{"inspection_kind": "CARGO_BAY", "performed_on": "2026-10-01"})); err == nil {
		t.Fatal("unauthorized subject accepted")
	}
	// Ownership revoked after recording: the case is withdrawn.
	w.allow("trucks", w.truck, w.scope)
	inspection := "insp-" + uuid.NewString()
	w.must(w.command("trucks", w.truck, "RECORD_INSPECTION", inspection, 0, map[string]string{"inspection_kind": "CARGO_BAY", "performed_on": "2026-10-01"}))
	w.exec1(`UPDATE hauling_organization_truck_links SET state='REVOKED',version=version+1 WHERE truck_id=$1`, w.truck)
	tx, _ := w.db.Begin(ctx)
	defer tx.Rollback(ctx)
	if _, err := w.source.Read(ctx, tx, "inspections", inspection, false); err == nil {
		t.Fatal("inspection visible after ownership link revoked")
	}
}
