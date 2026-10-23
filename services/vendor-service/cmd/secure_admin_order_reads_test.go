package main

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/features/adminresources"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"cosmicforge/logistics/shared/go/adminowner"
	"errors"
	"github.com/jackc/pgx/v5"
	"testing"
)

func TestVendorSecureAdminOrderAllocationGraph(t *testing.T) {
	e := newVendorAdminFixture(t)
	ctx := context.Background()
	parent, _, err := e.f.service.CreateFromQuote(ctx, e.f.customerID, e.f.quoteID, e.f.finger)
	if err != nil {
		t.Fatal(err)
	}
	if len(parent.VendorOrders) != 2 {
		t.Fatal("multi-vendor fixture required")
	}
	var own, other orders.VendorOrder
	for _, v := range parent.VendorOrders {
		if v.BusinessID == e.f.env.businessA {
			own = v
		} else {
			other = v
		}
	}
	own, _, err = e.f.service.Advance(ctx, e.f.actor(e.f.staffA, own.BusinessID), own.ID, orders.TransitionAccept, "", own.Version)
	if err != nil {
		t.Fatal(err)
	}
	other, _, err = e.f.service.Advance(ctx, e.f.actor(e.f.staffB, other.BusinessID), other.ID, orders.TransitionCancel, "SELLER_UNAVAILABLE", other.Version)
	if err != nil {
		t.Fatal(err)
	}
	if own.Status == other.Status {
		t.Fatal("divergent child outcomes were flattened")
	}
	tx, err := e.f.env.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	source := adminresources.Source()
	for _, v := range []orders.VendorOrder{own, other} {
		item, err := source.Read(ctx, tx, "vendor-orders", v.ID, true)
		if err != nil {
			t.Fatal(err)
		}
		if err = adminowner.ValidateResource(item, adminowner.VendorProfile().Definitions[item.Kind], true); err != nil {
			t.Fatal(err)
		}
		if item.Status == nil || *item.Status != string(v.Status) {
			t.Fatal("wrong owner outcome", item)
		}
		refs := map[string]string{}
		for _, r := range item.References {
			refs[r.Kind] = r.Reference
		}
		if refs["parent_order"] != parent.ID || refs["quote"] != e.f.quoteID {
			t.Fatal("broken authoritative lineage", refs)
		}
		found := false
		for _, d := range item.Dimensions {
			if d.Name == "parent_view_coverage" && d.Value != nil && *d.Value == "ALLOCATION_ONLY_SIBLINGS_WITHHELD" {
				found = true
			}
		}
		if !found {
			t.Fatal("missing explicit sibling withholding")
		}
	}
	if err = source.AuthorizeStaff(ctx, tx, e.actor.Staff, "vendor-orders", other.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("foreign allocation disclosed", err)
	}
	var lineID string
	if err = tx.QueryRow(ctx, `SELECT id FROM commerce_vendor_order_lines WHERE vendor_order_id=$1 LIMIT 1`, own.ID).Scan(&lineID); err != nil {
		t.Fatal(err)
	}
	line, err := source.Read(ctx, tx, "order-lines", lineID, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = adminowner.ValidateResource(line, adminowner.VendorProfile().Definitions[line.Kind], true); err != nil {
		t.Fatal(err)
	}
	if line.SourceVersion != nil {
		t.Fatal("immutable line invented mutation version")
	}
	if err = source.AuthorizeStaff(ctx, tx, e.f.staffB, "order-lines", lineID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("foreign line disclosed", err)
	}
	var eventID string
	if err = tx.QueryRow(ctx, `SELECT id FROM commerce_order_events WHERE vendor_order_id=$1 LIMIT 1`, own.ID).Scan(&eventID); err != nil {
		t.Fatal(err)
	}
	event, err := source.Read(ctx, tx, "order-events", eventID, true)
	if err != nil {
		t.Fatal(err)
	}
	if err = adminowner.ValidateResource(event, adminowner.VendorProfile().Definitions[event.Kind], true); err != nil {
		t.Fatal(err)
	}
}
