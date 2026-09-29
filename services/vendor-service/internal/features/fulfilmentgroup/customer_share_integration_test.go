package fulfilmentgroup

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/features/customershare"
	"encoding/json"
	"strings"
	"testing"
)

func TestOwnDeliveryCustomerShareUsesCanonicalRuntime(t *testing.T) {
	f := seedVendorDeliveryRuntime(t, 2)
	ctx := context.Background()
	var owner string
	if err := f.store.db.QueryRow(ctx, `SELECT customer_id FROM commerce_fulfilment_groups WHERE id=$1`, f.mission.GroupID).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	s := &customershare.Service{DB: f.store.db, Enabled: true, EnabledAuthorities: map[string]bool{"VENDOR_OWN_DELIVERY": true}}
	p, err := s.Read(ctx, f.mission.ID, owner)
	if err != nil || p.Status != "PLANNED" || !p.Eligible || p.Location.Available || p.Location.State != "UNAVAILABLE" {
		t.Fatal(p, err)
	}
	if _, err = s.Read(ctx, f.mission.ID, "foreign"); err == nil {
		t.Fatal("IDOR")
	}
	mission := assignFixtureRider(t, f, 0)
	p, err = s.Read(ctx, f.mission.ID, owner)
	if err != nil || p.Status != "ASSIGNED" {
		t.Fatal(p, err)
	}
	raw, _ := json.Marshal(p.Public([]string{"route", "provider", "location", "eta"}))
	for _, private := range []string{owner, f.businessID, f.branchID, mission.AssignedStaffID, "handover", "proof", "payment"} {
		if strings.Contains(string(raw), private) {
			t.Fatal("leak", private)
		}
	}
	for _, action := range []RiderAction{ActionAcknowledge, ActionStartPickup, ActionArrivePickup, ActionCompletePickup, ActionStartReceiver, ActionArriveReceiver, ActionCompleteDelivery} {
		runtime := applyRuntimeAction(t, f, mission, action)
		mission = runtime.Mission
	}
	p, err = s.Read(ctx, f.mission.ID, owner)
	if err != nil || !p.Terminal || p.Eligible || p.Status != "COMPLETED" || p.Location.Available || p.CompletedAt == nil {
		t.Fatal(p, err)
	}
	s.EnabledAuthorities["VENDOR_OWN_DELIVERY"] = false
	if _, err = s.Read(ctx, f.mission.ID, owner); err == nil {
		t.Fatal("flag ignored")
	}
}
