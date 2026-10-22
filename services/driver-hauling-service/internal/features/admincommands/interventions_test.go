package admincommands

import (
	"context"
	"sync"
	"testing"
	"time"

	availabilityusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_availability/usecases"
	"cosmicforge/logistics/services/hauling-service/internal/features/restrictions"
	"cosmicforge/logistics/shared/go/admincommand"
	"github.com/google/uuid"
)

func TestPostgresCargoSuitabilityUsesCurrentInputsAndBlocksAssignment(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	gate := SuitabilityGate{DB: w.db}
	w.allow("booking-cargo-inputs", w.booking, w.scope)
	param := func(decision, truck string) map[string]string {
		return map[string]string{"decision": decision, "truck_id": truck}
	}
	// 5 t of cargo cannot be declared SUITABLE for a 1 t truck: current fit fails.
	w.expect(w.command("booking-cargo-inputs", w.booking, "RECORD_CARGO_SUITABILITY", "", 0, param("SUITABLE", w.truckSmall)), admincommand.ErrRejected)
	ok := w.must(w.command("booking-cargo-inputs", w.booking, "RECORD_CARGO_SUITABILITY", "", 0, param("SUITABLE", w.truck)))
	if ok.ResultingState != "SUITABLE" || ok.PriorState != "NO_DECISION" {
		t.Fatal(ok)
	}
	// Current inputs change: the earlier SUITABLE no longer authorizes, and a
	// fresh SUITABLE is refused because CURRENT fit fails.
	w.exec1(`UPDATE haulage_bookings SET cargo_weight_kg=30000 WHERE id=$1`, w.booking)
	w.expect(w.command("booking-cargo-inputs", w.booking, "RECORD_CARGO_SUITABILITY", "", 1, param("SUITABLE", w.truck)), admincommand.ErrRejected)
	w.exec1(`UPDATE haulage_bookings SET cargo_weight_kg=5000 WHERE id=$1`, w.booking)
	if b, _ := gate.Blocked(ctx, w.booking, w.provider, w.truck); b {
		t.Fatal("SUITABLE blocks")
	}
	// UNSUITABLE blocks that truck (and, with no truck fixed, its provider).
	w.must(w.command("booking-cargo-inputs", w.booking, "RECORD_CARGO_SUITABILITY", "", 1, param("UNSUITABLE", w.truck)))
	for _, truck := range []string{w.truck, ""} {
		if b, err := gate.Blocked(ctx, w.booking, w.provider, truck); err != nil || !b {
			t.Fatal("UNSUITABLE did not block assignment", truck, err)
		}
	}
	if b, _ := gate.Blocked(ctx, w.booking, w.provider2, w.truck2); b {
		t.Fatal("UNSUITABLE leaked to another truck")
	}
	// Remediation: pending blocks, one pending per booking, completion requires
	// current fit, and completion unblocks.
	w.must(w.command("booking-cargo-inputs", w.booking, "RECORD_CARGO_SUITABILITY", "", 2, param("SUITABLE_WITH_REMEDIATION", w.truck2)))
	if b, _ := gate.Blocked(ctx, w.booking, w.provider2, w.truck2); !b {
		t.Fatal("pending remediation did not block")
	}
	w.expect(w.command("booking-cargo-inputs", w.booking, "RECORD_CARGO_SUITABILITY", "", 3, param("SUITABLE_WITH_REMEDIATION", w.truckSmall)), admincommand.ErrRejected)
	w.expect(w.command("booking-cargo-inputs", w.booking, "RECORD_CARGO_SUITABILITY", "", 3, param("SUITABLE", w.truck2)), admincommand.ErrRejected)
	w.exec1(`UPDATE haulage_bookings SET cargo_weight_kg=30000 WHERE id=$1`, w.booking)
	w.expect(w.command("booking-cargo-inputs", w.booking, "COMPLETE_CARGO_REMEDIATION", "", 3, map[string]string{"evidence_ref": "rem-1"}), admincommand.ErrRejected)
	w.exec1(`UPDATE haulage_bookings SET cargo_weight_kg=5000 WHERE id=$1`, w.booking)
	done := w.must(w.command("booking-cargo-inputs", w.booking, "COMPLETE_CARGO_REMEDIATION", "", 3, map[string]string{"evidence_ref": "rem-1"}))
	if done.ResultingState != "REMEDIATION_COMPLETED" {
		t.Fatal(done)
	}
	if b, _ := gate.Blocked(ctx, w.booking, w.provider2, w.truck2); b {
		t.Fatal("completed remediation still blocks")
	}
	w.expect(w.command("booking-cargo-inputs", w.booking, "COMPLETE_CARGO_REMEDIATION", "", 4, map[string]string{"evidence_ref": "rem-2"}), admincommand.ErrRejected)
	// Stale version.
	w.expect(w.command("booking-cargo-inputs", w.booking, "RECORD_CARGO_SUITABILITY", "", 1, param("UNSUITABLE", w.truck2)), admincommand.ErrConflict)
	// Terminal booking: no further decisions.
	w.exec1(`UPDATE haulage_bookings SET status='cancelled' WHERE id=$1`, w.booking)
	w.expect(w.command("booking-cargo-inputs", w.booking, "RECORD_CARGO_SUITABILITY", "", 4, param("UNSUITABLE", w.truck2)), admincommand.ErrRejected)
	if _, err := w.db.Exec(ctx, `DELETE FROM hauling_cargo_suitability_history`); err == nil {
		t.Fatal("suitability history mutable")
	}
	// Cross-tenant: the booking's planning organization is org A.
	other := w.seedBooking("pending_match", "", "", 100)
	if _, err := w.svc.Execute(ctx, w.otherActor, purpose, w.otherScope, w.command("booking-cargo-inputs", other, "RECORD_CARGO_SUITABILITY", "", 0, param("UNSUITABLE", w.truck))); err == nil {
		t.Fatal("foreign tenant decided on a booking")
	}
}

func TestPostgresRestrictLiftOwnershipApprovalConcurrencyAndReplay(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	gate := restrictions.New(w.db)
	w.allow("trucks", w.truck, w.scope)
	w.allow("providers", w.provider, w.scope)
	// Approval required, and bound to the exact intent.
	unapproved := w.command("trucks", w.truck, "RESTRICT_TRUCK", "", 0, map[string]string{"reason": "SAFETY_CONCERN"})
	if _, err := w.svc.Execute(ctx, w.actor, purpose, w.scope, unapproved); err != admincommand.ErrInvalid {
		t.Fatal(err)
	}
	// Concurrent conflicting commands: exactly one commits.
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := w.run(w.command("trucks", w.truck, "RESTRICT_TRUCK", "", 0, map[string]string{"reason": "SAFETY_CONCERN"}))
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		} else if err != admincommand.ErrConflict && err != admincommand.ErrRejected {
			t.Fatal("unexpected", err)
		}
	}
	if wins != 1 || w.scalar(`SELECT count(*)::text FROM hauling_authority_restrictions WHERE subject_reference=$1`, w.truck) != "1" {
		t.Fatal("concurrent restrictions", wins)
	}
	if view, err := gate.Read(ctx, w.orgA, "ASSET", w.truck); err != nil || view.State != "APPLIED" || view.ReasonCode != "SAFETY_CONCERN" {
		t.Fatal("restriction not owned by the truck's organization", view, err)
	}
	// Already restricted: illegal transition, no side effect.
	w.expect(w.command("trucks", w.truck, "RESTRICT_TRUCK", "", 1, map[string]string{"reason": "DOCUMENT_LAPSE"}), admincommand.ErrRejected)
	// Lift, with idempotent replay returning the original receipt.
	lift := w.command("trucks", w.truck, "LIFT_TRUCK_RESTRICTION", "", 1, nil)
	w.approve(&lift, "staff-b", w.scope)
	first := w.must(lift)
	replay, err := w.run(lift)
	if err != nil || replay != first {
		t.Fatal("replay", err)
	}
	if r, _ := gate.Restricted(ctx, "", "ASSET", w.truck); r {
		t.Fatal("lift ineffective")
	}
	w.expect(w.command("trucks", w.truck, "LIFT_TRUCK_RESTRICTION", "", 2, nil), admincommand.ErrRejected)
	// A restriction owned by another authority (Fleet) cannot be lifted here.
	if _, err := gate.Apply(ctx, restrictions.Command{OperationReference: "fleet-" + uuid.NewString(), OrganizationID: w.orgA, SubjectType: "PROVIDER", SubjectReference: w.provider, ReasonCode: "FLEET", SourceReference: "fleet-case-1", SourceVersion: 1}); err != nil {
		t.Fatal(err)
	}
	w.expect(w.command("providers", w.provider, "RESTRICT_PROVIDER", "", 0, map[string]string{"reason": "CONDUCT"}), admincommand.ErrRejected)
	w.expect(w.command("providers", w.provider, "LIFT_PROVIDER_RESTRICTION", "", 0, nil), admincommand.ErrRejected)
	// Provider restriction through the owner path.
	w.allow("providers", w.provider2, w.scope)
	w.must(w.command("providers", w.provider2, "RESTRICT_PROVIDER", "", 0, map[string]string{"reason": "CONDUCT"}))
	if r, _ := gate.Restricted(ctx, "", "PROVIDER", w.provider2); !r {
		t.Fatal("provider restriction ineffective")
	}
	// The existing availability path now refuses to put the provider online.
	online := availabilityusecases.NewAvailabilityService(nil, nil, time.Minute).WithRestrictionGate(gate)
	if _, err := online.SetAvailability(ctx, availabilityusecases.SetAvailabilityInput{ProviderID: w.provider2, Status: "online", TruckID: w.truck2, Lat: 6.5, Lng: 3.3}); err == nil || !contains(err.Error(), "restricted") {
		t.Fatal("restricted provider could go online", err)
	}
	// Readback and history.
	back, err := w.svc.Readback(ctx, w.actor, purpose, w.scope, "trucks", w.truck, lift.IdempotencyKey)
	if err != nil || back != first {
		t.Fatal("readback", err)
	}
	h, err := w.svc.History(ctx, w.actor, purpose, w.scope, "trucks", w.truck)
	if err != nil || h.CurrentVersion != 2 || h.CurrentState != "ACTIVE" || len(h.Decisions) != 2 {
		t.Fatal("history", err, h)
	}
	// Cross tenant: an org B context on org A's provider is denied by ownership.
	w.allow("providers", w.provider, w.otherScope)
	c := w.command("providers", w.provider, "RESTRICT_PROVIDER", "", 0, map[string]string{"reason": "CONDUCT"})
	w.approve(&c, "staff-b", w.otherScope)
	if _, err := w.svc.Execute(ctx, w.otherActor, purpose, w.otherScope, c); err == nil {
		t.Fatal("foreign tenant restricted a provider")
	}
}

func TestPostgresLegacyInspectionMappingIsExactSameTruckConcludedAndSingle(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	w.allow("trucks", w.truck, w.scope)
	w.allow("trucks", w.truck2, w.scope)
	w.exec1(`UPDATE trucks SET inspection_ref='LEGACY-INSP-7' WHERE id=$1`, w.truck)
	var eligibility string
	if err := w.db.QueryRow(ctx, `INSERT INTO hauling_truck_product_eligibility(truck_id,provider_id,product_code,status) VALUES($1,$2,'HAUL_GENERAL','ELIGIBLE') RETURNING id::text`, w.truck, w.provider).Scan(&eligibility); err != nil {
		t.Fatal(err)
	}
	subject := eligibility + ":" + w.orgA
	w.allow("product-eligibility", subject, w.scope)
	concluded, open, foreign := "insp-"+uuid.NewString(), "insp-"+uuid.NewString(), "insp-"+uuid.NewString()
	for _, c := range []struct{ id, truck string }{{concluded, w.truck}, {open, w.truck}, {foreign, w.truck2}} {
		w.must(w.command("trucks", c.truck, "RECORD_INSPECTION", c.id, 0, map[string]string{"inspection_kind": "ROADWORTHINESS", "performed_on": "2026-10-01"}))
	}
	w.must(w.command("trucks", w.truck, "CONCLUDE_INSPECTION", concluded, 1, map[string]string{"conclusion": "PASS"}))
	w.must(w.command("trucks", w.truck2, "CONCLUDE_INSPECTION", foreign, 1, map[string]string{"conclusion": "PASS"}))
	link := func(caseRef, ref string, expected int64) admincommand.Command {
		return w.command("product-eligibility", subject, "LINK_LEGACY_INSPECTION_REFERENCE", caseRef, expected, map[string]string{"legacy_inspection_ref": ref})
	}
	w.expect(link(concluded, "LEGACY-INSP-07", 0), admincommand.ErrRejected) // not exactly equal
	w.expect(link(concluded, "legacy-insp-7", 0), admincommand.ErrRejected)  // no case folding
	w.expect(link(concluded, "https://x.example/LEGACY-INSP-7", 0), admincommand.ErrInvalid)
	w.expect(link(open, "LEGACY-INSP-7", 0), admincommand.ErrRejected)           // not concluded
	w.expect(link(foreign, "LEGACY-INSP-7", 0), admincommand.ErrRejected)        // other truck
	w.expect(link("insp-missing", "LEGACY-INSP-7", 0), admincommand.ErrRejected) // no such case
	r := w.must(link(concluded, "LEGACY-INSP-7", 0))
	if r.ResultingState != "LEGACY_LINKED" || r.PriorState != "UNLINKED" {
		t.Fatal(r)
	}
	w.expect(link(concluded, "LEGACY-INSP-7", 1), admincommand.ErrRejected) // single mapping
	if w.scalar(`SELECT inspection_id FROM hauling_admin_legacy_inspection_mappings WHERE truck_id=$1 AND legacy_inspection_ref='LEGACY-INSP-7'`, w.truck) != concluded {
		t.Fatal("mapping")
	}
	if _, err := w.db.Exec(ctx, `UPDATE hauling_admin_legacy_inspection_mappings SET inspection_id=$1`, open); err == nil {
		t.Fatal("mapping mutable")
	}
}

func TestPostgresRecoveryReplacementRequiresCurrentOwnerValidation(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	booking := w.seedBooking("accepted", w.provider, w.truck, 5000)
	recoveryID := uuid.NewString()
	w.exec1(`INSERT INTO hauling_recovery_cases(id,booking_id,trigger_code,state,operation_ref,original_provider_id,original_truck_id,detected_at,audit_json) VALUES($1,$2,'PROVIDER_BREAKDOWN','DETECTED',$3,$4,$5,now(),'[]')`, recoveryID, booking, "op-"+recoveryID, w.provider, w.truck)
	w.allow("recoveries", recoveryID, w.scope)
	replace := func(provider, truck string) admincommand.Command {
		return w.command("recoveries", recoveryID, "REQUEST_RECOVERY_REPLACEMENT", "", 0, map[string]string{"replacement_provider_id": provider, "replacement_truck_id": truck})
	}
	w.expect(replace(w.providerSmall, w.truckSmall), admincommand.ErrRejected) // does not currently fit
	w.expect(replace(w.providerB, w.truckB), admincommand.ErrRejected)         // other organization
	w.expect(replace(w.provider2, w.truck), admincommand.ErrRejected)          // truck not provider's
	if _, err := restrictions.New(w.db).Apply(ctx, restrictions.Command{OperationReference: "r-" + uuid.NewString(), OrganizationID: w.orgA, SubjectType: "ASSET", SubjectReference: w.truck2, ReasonCode: "X", SourceReference: "fleet", SourceVersion: 1}); err != nil {
		t.Fatal(err)
	}
	w.expect(replace(w.provider2, w.truck2), admincommand.ErrRejected) // restricted truck
	w.exec1(`UPDATE hauling_authority_restrictions SET state='CLEARED' WHERE subject_reference=$1`, w.truck2)
	if w.scalar(`SELECT state FROM hauling_recovery_cases WHERE id=$1`, recoveryID) != "DETECTED" {
		t.Fatal("rejected replacement moved the case")
	}
	if w.scalar(`SELECT count(*)::text FROM hauling_payment_rebind_obligations WHERE booking_id=$1`, booking) != "0" {
		t.Fatal("a rejected replacement wrote a payment rebind obligation")
	}
	r := w.must(replace(w.provider2, w.truck2))
	if r.PriorState != "DETECTED" || r.ResultingState != "IN_PROGRESS" || r.Outcome != "REPLACEMENT_BOUND" {
		t.Fatal(r)
	}
	if w.scalar(`SELECT provider_id::text||'/'||truck_id::text FROM haulage_bookings WHERE id=$1`, booking) != w.provider2+"/"+w.truck2 {
		t.Fatal("booking not rebound")
	}
	// The held payment must follow: the obligation commits with the command.
	if w.scalar(`SELECT state||'/'||provider_id::text||'/'||recovery_case_id::text FROM hauling_payment_rebind_obligations WHERE booking_id=$1`, booking) != "PENDING/"+w.provider2+"/"+recoveryID {
		t.Fatal("operator replacement did not write the payment rebind obligation")
	}
	w.expect(w.command("recoveries", recoveryID, "REQUEST_RECOVERY_REPLACEMENT", "", 1, map[string]string{"replacement_provider_id": w.provider, "replacement_truck_id": w.truck}), admincommand.ErrRejected)
	tx, _ := w.db.Begin(ctx)
	defer tx.Rollback(ctx)
	if v, s, err := w.exec.Current(ctx, tx, "recoveries", recoveryID); err != nil || v != 1 || s != "IN_PROGRESS" {
		t.Fatal(v, s, err)
	}
}
