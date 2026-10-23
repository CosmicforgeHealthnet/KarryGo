package main

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/features/adminresources"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilmentgroup"
	"cosmicforge/logistics/shared/go/adminowner"
	"errors"
	"github.com/jackc/pgx/v5"
	"strings"
	"testing"
	"time"
)

func TestVendorSecureAdminDeliveryReadsPreserveAllocationAndOwnerAuthority(t *testing.T) {
	e := newVendorAdminFixture(t)
	ctx := context.Background()
	now := time.Now().UTC()
	parent, _, err := e.f.service.CreateFromQuote(ctx, e.f.customerID, e.f.quoteID, e.f.finger)
	if err != nil {
		t.Fatal(err)
	}
	store := fulfilmentgroup.NewStore(e.f.env.pool, nil)
	group, err := store.EnsureGroup(ctx, parent.ID, e.f.customerID, now)
	if err != nil {
		t.Fatal(err)
	}
	units := map[string]fulfilmentgroup.Unit{}
	parcels := map[string]fulfilmentgroup.Parcel{}
	for _, v := range parent.VendorOrders {
		u, err := store.EnsureUnit(ctx, group, fulfilmentgroup.UnitInput{VendorOrderID: v.ID}, now)
		if err != nil {
			t.Fatal(err)
		}
		units[v.BusinessID] = u
		p, err := store.CreateParcel(ctx, v.BusinessID, u.ID, fulfilmentgroup.ParcelInput{IdempotencyKey: "fixture-pack-" + u.ID, WeightGrams: 1200}, now)
		if err != nil {
			t.Fatal(err)
		}
		parcels[v.BusinessID] = p
	}
	a, b := units[e.f.env.businessA], units[e.f.env.businessB]
	slot := func(u fulfilmentgroup.Unit, owner fulfilmentgroup.Owner, authority fulfilmentgroup.Authority, sequence int) fulfilmentgroup.PlanMission {
		return fulfilmentgroup.PlanMission{SlotRef: "slot-" + u.ID, Sequence: sequence, Owner: owner, Authority: authority, Currency: "NGN", ParcelIDs: []string{parcels[u.BusinessID].ID}, Stops: []fulfilmentgroup.Stop{{Sequence: 1, Type: fulfilmentgroup.StopPickup, UnitID: u.ID}, {Sequence: 2, Type: fulfilmentgroup.StopDropoff, Address: "Disposable delivery fixture"}}}
	}
	plan, err := store.ActivatePlan(ctx, group.ID, fulfilmentgroup.PlanInput{ExpectedVersion: 0, Reason: fulfilmentgroup.ReasonInitialPlan, DeliveryMode: fulfilmentgroup.ModeHybrid, IdempotencyKey: "section11-plan", Missions: []fulfilmentgroup.PlanMission{slot(a, fulfilmentgroup.OwnerPlatform, fulfilmentgroup.AuthorityDispatch, 1), slot(b, fulfilmentgroup.OwnerVendor, fulfilmentgroup.AuthorityVendor, 2)}}, now)
	if err != nil {
		t.Fatal(err)
	}
	var platform fulfilmentgroup.PlatformMissionRef
	var own fulfilmentgroup.VendorMission
	for _, m := range plan.Missions {
		if m.Owner == fulfilmentgroup.OwnerPlatform {
			platform, _, err = store.EnsurePlatformMissionRef(ctx, group.ID, plan.ID, m.ID, m.Authority, now)
		} else {
			own, err = store.EnsureVendorMission(ctx, group.ID, plan.ID, m.ID, now)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	tx, err := e.f.env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	source := adminresources.Source()
	read := func(kind, id string) adminowner.Resource {
		t.Helper()
		r, err := source.Read(ctx, tx, kind, id, true)
		if err != nil {
			t.Fatal(kind, err)
		}
		if err = adminowner.ValidateResource(r, adminowner.VendorProfile().Definitions[kind], true); err != nil {
			t.Fatal(kind, err)
		}
		return r
	}
	unit := read("fulfilment-units", a.ID)
	if unit.Status == nil || *unit.Status != "NOT_READY" {
		t.Fatal("manufactured readiness", unit)
	}
	linkID := platform.ID + ":" + a.VendorOrderID
	link := read("delivery-mission-links", linkID)
	observed := false
	for _, d := range link.Dimensions {
		if d.Name == "data_authority" && d.Value != nil && *d.Value == "OWNER_PROJECTION_ONLY" {
			observed = true
		}
		if d.Name == "observed_status" && d.Value != nil {
			t.Fatal("invented Dispatch current status", d)
		}
	}
	if !observed {
		t.Fatal("cached delivery observation mislabeled as owner authority")
	}
	if err = source.AuthorizeStaff(ctx, tx, e.actor.Staff, "delivery-mission-links", linkID); err != nil {
		t.Fatal("own allocation denied", err)
	}
	if _, err = source.Read(ctx, tx, "delivery-mission-links", platform.ID+":"+b.VendorOrderID, true); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("foreign allocation joined unrelated mission", err)
	}
	ownRead := read("vendor-delivery-status", own.ID)
	if ownRead.Status == nil || *ownRead.Status != "PLANNED" {
		t.Fatal("invented vendor delivery completion", ownRead)
	}
	if err = source.AuthorizeStaff(ctx, tx, e.actor.Staff, "vendor-delivery-status", own.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("foreign merchant delivery disclosed", err)
	}
	for _, r := range ownRead.References {
		if strings.Contains(r.Kind, "proof") || strings.Contains(r.Kind, "handover") {
			t.Fatal("handover secret/reference disclosed", r)
		}
	}
}
