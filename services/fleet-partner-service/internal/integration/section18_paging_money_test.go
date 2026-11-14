package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	assetsmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/assets/models"
	fuelmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/fuel/models"
	inspectionsmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/inspections/models"
	maintenancemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/maintenance/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// section18PagingFixture is an organization with two branches. Branch A holds
// seven assets, each with an inspection, a maintenance schedule and a fuel
// record; branch B holds three of each. A member scoped to branch A must page
// through exactly branch A's rows.
func section18PagingFixture(t *testing.T) (*assetLifecycleFixture, string, map[string]bool) {
	t.Helper()
	f := newAssetLifecycleFixture(t)
	ctx := context.Background()
	other := ids.New("brn")
	if _, e := f.pool.Exec(ctx, `INSERT INTO fleet_partner_branches(id,organization_id,code,name,timezone)VALUES($1,$2,$3,'Other','Africa/Lagos')`, other, f.org, "S18P-"+other[len(other)-6:]); e != nil {
		t.Fatal(e)
	}
	tpl, e := f.svc.SaveTemplate(ctx, f.t, "", 0, inspectionsmodels.Template{Name: "Paging", ChecklistItems: []byte(`[{"code":"a","required":true}]`), CriticalityRules: []byte(`{}`)})
	if e != nil {
		t.Fatal(e)
	}
	inA := map[string]bool{}
	due := f.now.Add(240 * time.Hour)
	for i := 0; i < 10; i++ {
		branch := f.branch
		if i >= 7 {
			branch = other
		}
		a, e := f.svc.CreateAsset(ctx, f.t, assetsmodels.Asset{BranchID: branch, AssetCode: fmt.Sprintf("PG-%d-%s", i, ids.New("c")), AssetType: "TRUCK"})
		if e != nil {
			t.Fatal(e)
		}
		if branch == f.branch {
			inA[a.ID] = true
		}
		if _, e = f.svc.CreateInspection(ctx, f.t, inspectionsmodels.Inspection{AssetID: a.ID, TemplateID: tpl.ID, InspectorMembershipID: f.member}); e != nil {
			t.Fatal(e)
		}
		if _, e = f.svc.SaveSchedule(ctx, f.t, "", 0, maintenancemodels.Schedule{AssetID: a.ID, TriggerType: "DATE", NextDueDate: &due}); e != nil {
			t.Fatal(e)
		}
		if _, e = f.svc.CreateFuel(ctx, f.t, fuelmodels.FuelRecord{AssetID: a.ID, FuelOrEnergyType: "DIESEL", Unit: "L", Quantity: 10, RecordedAt: f.now.Add(-time.Duration(i%3) * time.Hour)}); e != nil {
			t.Fatal(e)
		}
	}
	// Equal timestamps exercise the id tie-breaker.
	for _, table := range []string{"fleet_partner_assets", "fleet_partner_inspections", "fleet_partner_maintenance_schedules"} {
		if _, e := f.pool.Exec(ctx, `UPDATE `+table+` SET created_at=$2 WHERE organization_id=$1`, f.org, f.now); e != nil {
			t.Fatal(e)
		}
	}
	return f, other, inA
}

// collectPages walks a keyset list from the first page and checks that no
// row repeats; it returns the asset id of every row in order.
func collectPages(t *testing.T, name string, page func(after *time.Time, afterID string) ([]string, []string, *time.Time, string)) []string {
	t.Helper()
	seen := map[string]bool{}
	var assets []string
	var after *time.Time
	afterID := ""
	for i := 0; i < 20; i++ {
		rowIDs, assetIDs, nextAfter, nextID := page(after, afterID)
		for j, id := range rowIDs {
			if seen[id] {
				t.Fatalf("%s: row %s returned twice", name, id)
			}
			seen[id] = true
			assets = append(assets, assetIDs[j])
		}
		if nextAfter == nil {
			return assets
		}
		after, afterID = nextAfter, nextID
	}
	t.Fatalf("%s: paging did not terminate", name)
	return nil
}

func TestSection18ListsPageByCursorWithinBranchScope(t *testing.T) {
	f, _, inA := section18PagingFixture(t)
	ctx := context.Background()
	scoped := f.t
	scoped.PermittedBranchIDs = []string{f.branch}
	check := func(name string, assets []string) {
		t.Helper()
		if len(assets) != len(inA) {
			t.Fatalf("%s: paged %d rows, want %d (gap or out-of-scope row)", name, len(assets), len(inA))
		}
		got := map[string]bool{}
		for _, id := range assets {
			if !inA[id] {
				t.Fatalf("%s: row of out-of-scope asset %s", name, id)
			}
			got[id] = true
		}
		if len(got) != len(inA) {
			t.Fatalf("%s: duplicate assets across pages", name)
		}
	}

	check("assets", collectPages(t, "assets", func(after *time.Time, afterID string) ([]string, []string, *time.Time, string) {
		rows, more, e := f.svc.ListAssetsPage(ctx, scoped, assetsmodels.Page{Limit: 3, After: after, AfterID: afterID})
		if e != nil {
			t.Fatal(e)
		}
		var ids []string
		for _, a := range rows {
			ids = append(ids, a.ID)
		}
		if !more {
			return ids, ids, nil, ""
		}
		last := rows[len(rows)-1]
		c, _, _ := cursor.Decode(cursor.EncodeTime(last.CreatedAt, last.ID))
		at := c.SortTime()
		return ids, ids, &at, c.LastID
	}))
	check("inspections", collectPages(t, "inspections", func(after *time.Time, afterID string) ([]string, []string, *time.Time, string) {
		rows, last, e := f.svc.ListInspectionsPage(ctx, scoped, inspectionsmodels.Page{Limit: 3, After: after, AfterID: afterID})
		if e != nil {
			t.Fatal(e)
		}
		var ids, assets []string
		for _, v := range rows {
			ids, assets = append(ids, v.ID), append(assets, v.AssetID)
		}
		if last == nil {
			return ids, assets, nil, ""
		}
		at := last.CreatedAt
		return ids, assets, &at, last.ID
	}))
	check("schedules", collectPages(t, "schedules", func(after *time.Time, afterID string) ([]string, []string, *time.Time, string) {
		rows, last, e := f.svc.ListSchedulesPage(ctx, scoped, maintenancemodels.Page{Limit: 3, After: after, AfterID: afterID})
		if e != nil {
			t.Fatal(e)
		}
		var ids, assets []string
		for _, v := range rows {
			ids, assets = append(ids, v.ID), append(assets, v.AssetID)
		}
		if last == nil {
			return ids, assets, nil, ""
		}
		at := last.CreatedAt
		return ids, assets, &at, last.ID
	}))
	check("fuel", collectPages(t, "fuel", func(after *time.Time, afterID string) ([]string, []string, *time.Time, string) {
		rows, last, e := f.svc.ListFuelPage(ctx, scoped, fuelmodels.Page{Limit: 3, After: after, AfterID: afterID})
		if e != nil {
			t.Fatal(e)
		}
		var ids, assets []string
		for _, v := range rows {
			ids, assets = append(ids, v.ID), append(assets, v.AssetID)
		}
		if last == nil {
			return ids, assets, nil, ""
		}
		at := last.RecordedAt
		return ids, assets, &at, last.ID
	}))
}

// TestSection18AssetListCursorOverHTTP follows meta.nextCursor through the
// real route, keeps offset paging working and refuses cursor plus offset.
func TestSection18AssetListCursorOverHTTP(t *testing.T) {
	f, _, inA := section18PagingFixture(t)
	router := section18Router(t, f.pool, f.svc)
	owner := seedApprovalMember(t, f.pool, f.org, "rol_system_owner", "s18p-owner")
	call := func(path string) (int, map[string]any) {
		req := httptest.NewRequest("GET", "/fleet-partner/v1"+path, nil)
		req.Header.Set("X-Test-Org", f.org)
		req.Header.Set("X-Test-Member", owner)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		out := map[string]any{}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	seen := map[string]bool{}
	path := "/assets?branchId=" + f.branch + "&limit=3"
	for i := 0; i < 10; i++ {
		code, body := call(path)
		if code != 200 {
			t.Fatalf("page %d = %d %v", i, code, body)
		}
		for _, row := range body["data"].([]any) {
			id := row.(map[string]any)["id"].(string)
			if seen[id] || !inA[id] {
				t.Fatalf("page %d: repeated or out-of-branch asset %s", i, id)
			}
			seen[id] = true
		}
		next, _ := body["meta"].(map[string]any)["nextCursor"].(string)
		if next == "" {
			break
		}
		path = "/assets?branchId=" + f.branch + "&limit=3&cursor=" + next
	}
	if len(seen) != len(inA) {
		t.Fatalf("cursor paging returned %d assets, want %d", len(seen), len(inA))
	}
	if code, body := call("/assets?branchId=" + f.branch + "&limit=3&offset=3"); code != 200 || len(body["data"].([]any)) != 3 {
		t.Fatalf("offset page = %d %v", code, body)
	}
	if code, _ := call("/assets?limit=3&offset=3&cursor=" + cursor.EncodeTime(f.now, "ast_x")); code != 422 {
		t.Fatalf("cursor plus offset = %d, want 422", code)
	}
	if code, _ := call("/assets?cursor=not-a-cursor"); code != 422 {
		t.Fatalf("malformed cursor = %d, want 422", code)
	}
}

// TestSection18AssetCostsCarryMoneyObject: every cost entry and total keeps
// its flat amountMinor/currency and also carries an equal Money object.
func TestSection18AssetCostsCarryMoneyObject(t *testing.T) {
	f := newAssetLifecycleFixture(t)
	router := section18Router(t, f.pool, f.svc)
	owner := seedApprovalMember(t, f.pool, f.org, "rol_system_owner", "s18m-owner")
	asset := f.asset(t, "MONEY-1")
	money := func(v map[string]any) {
		t.Helper()
		m, ok := v["amount"].(map[string]any)
		if !ok || num(m["amountMinor"]) != num(v["amountMinor"]) || m["currency"] != v["currency"] || num(v["amountMinor"]) == 0 {
			t.Fatalf("money object does not match flat fields: %v", v)
		}
	}
	code, entry := section18Call(t, router, "POST", "/assets/"+asset.ID+"/costs", f.org, owner, map[string]any{"costType": "OTHER", "amountMinor": 125050, "currency": "NGN", "description": "Wash"}, 0)
	if code != 201 {
		t.Fatalf("record cost = %d %v", code, entry)
	}
	money(entry)
	code, history := section18Call(t, router, "GET", "/assets/"+asset.ID+"/costs", f.org, owner, nil, 0)
	if code != 200 {
		t.Fatalf("list costs = %d %v", code, history)
	}
	for _, key := range []string{"entries", "totals"} {
		rows := history[key].([]any)
		if len(rows) != 1 {
			t.Fatalf("%s = %v", key, rows)
		}
		money(rows[0].(map[string]any))
	}
}
