package crm

import (
	"context"
	"testing"
)

func TestCorporateLocation_LifecycleValidationAndArchive(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	profile := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")

	location, err := env.service.CreateCorporateLocation(ctx, actor, globalScope(), profile.ID,
		CorporateLocationInput{Code: "hq-1", Name: "Head office", LocationType: LocationHeadquarters,
			AddressLine1: "1 Marina Road", City: "Lagos", CountryCode: "ng", Timezone: "Africa/Lagos"})
	if err != nil {
		t.Fatalf("create location: %v", err)
	}
	if location.CountryCode == nil || *location.CountryCode != "NG" {
		t.Fatalf("the country code was not normalised: %v", location.CountryCode)
	}
	if location.NormalizedCode != "HQ1" {
		t.Fatalf("the location code was not normalised: %s", location.NormalizedCode)
	}

	// The normalised code is unique per account, so "hq 1" and "HQ-1" are not
	// two sites.
	if _, err = env.service.CreateCorporateLocation(ctx, actor, globalScope(), profile.ID,
		CorporateLocationInput{Code: "HQ 1", Name: "Duplicate"}); err == nil {
		t.Fatal("a duplicate normalised location code was accepted")
	} else if !isCRMCode(err, CodeCorporateDuplicate) {
		t.Fatalf("expected a duplicate rejection, got %v", err)
	}

	t.Run("validation", func(t *testing.T) {
		cases := map[string]CorporateLocationInput{
			"unsupported type":  {Code: "A1", Name: "Site", LocationType: "MOON_BASE"},
			"bad country code":  {Code: "A2", Name: "Site", CountryCode: "NGA"},
			"unknown timezone":  {Code: "A3", Name: "Site", Timezone: "Mars/Olympus"},
			"non-iana timezone": {Code: "A4", Name: "Site", Timezone: "Local"},
			"missing name":      {Code: "A5"},
			"missing code":      {Name: "Site"},
			"markup in name":    {Code: "A6", Name: "<b>Site</b>"},
		}
		for name, input := range cases {
			if _, err := env.service.CreateCorporateLocation(ctx, actor, globalScope(), profile.ID, input); err == nil {
				t.Errorf("expected %s to be rejected", name)
			}
		}
		long := make([]byte, 201)
		for index := range long {
			long[index] = 'a'
		}
		if _, err := env.service.CreateCorporateLocation(ctx, actor, globalScope(), profile.ID,
			CorporateLocationInput{Code: "A7", Name: "Site", AddressLine1: string(long)}); err == nil {
			t.Error("an over-long address line should be rejected")
		}
	})

	updated, err := env.service.UpdateCorporateLocation(ctx, actor, globalScope(), profile.ID, location.ID,
		CorporateLocationInput{Code: "hq-1", Name: "Head office (Marina)", LocationType: LocationOffice,
			ExpectedVersion: location.Version})
	if err != nil {
		t.Fatalf("update location: %v", err)
	}
	if updated.Name != "Head office (Marina)" {
		t.Fatalf("the location was not updated: %s", updated.Name)
	}
	if _, err = env.service.UpdateCorporateLocation(ctx, actor, globalScope(), profile.ID, location.ID,
		CorporateLocationInput{Code: "hq-1", Name: "Stale", ExpectedVersion: location.Version}); err == nil {
		t.Fatal("a stale location version should conflict")
	}

	// Archiving preserves the row, because a cost centre or a historical
	// interaction may name it.
	center := env.addCorporateCostCenter(t, businessA, "vs_a", profile.ID, "OPS", "Operations")
	if _, err = env.service.UpdateCorporateCostCenter(ctx, actor, globalScope(), profile.ID, center.ID,
		CorporateCostCenterInput{Code: "OPS", Name: "Operations", LocationRef: updated.ID,
			ExpectedVersion: center.Version}); err != nil {
		t.Fatalf("link the cost centre to the location: %v", err)
	}
	archived, err := env.service.ArchiveCorporateLocation(ctx, actor, globalScope(), profile.ID, location.ID,
		updated.Version)
	if err != nil {
		t.Fatalf("archive location: %v", err)
	}
	if archived.Status != CorporateChildArchived || archived.ArchivedAt == nil {
		t.Fatalf("unexpected archived state: %+v", archived)
	}
	var stillThere bool
	if err = env.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM corporate_customer_locations
		WHERE business_id=$1 AND id=$2)`, businessA, location.ID).Scan(&stillThere); err != nil {
		t.Fatal(err)
	}
	if !stillThere {
		t.Fatal("an archived location was hard-deleted despite being referenced")
	}
	// An archived site cannot be attached to anything new.
	if _, err = env.service.CreateCorporateCostCenter(ctx, actor, globalScope(), profile.ID,
		CorporateCostCenterInput{Code: "FIN", Name: "Finance", LocationRef: location.ID}); err == nil {
		t.Fatal("an archived location was attached to a new cost centre")
	}
}

func TestCorporateCostCenter_IsALabelWithNoFinancialAuthority(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	profile := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")

	center, err := env.service.CreateCorporateCostCenter(ctx, actor, globalScope(), profile.ID,
		CorporateCostCenterInput{Code: "ops-1", Name: "Operations"})
	if err != nil {
		t.Fatalf("create cost centre: %v", err)
	}
	if center.NormalizedCode != "OPS1" {
		t.Fatalf("the cost centre code was not normalised: %s", center.NormalizedCode)
	}
	if _, err = env.service.CreateCorporateCostCenter(ctx, actor, globalScope(), profile.ID,
		CorporateCostCenterInput{Code: "OPS 1", Name: "Duplicate"}); err == nil {
		t.Fatal("a duplicate normalised cost centre code was accepted")
	}

	// A cost centre is a label. There is no budget, limit, currency or balance
	// column anywhere on it, and its absence is the boundary that keeps
	// vendor-service from becoming a second financial authority.
	var financial int
	if err = env.pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name='corporate_customer_cost_centers'
		  AND column_name ~ '(budget|limit|currency|balance|amount|credit|spend|wallet|invoice|payment|minor|kobo)'`).
		Scan(&financial); err != nil {
		t.Fatal(err)
	}
	if financial != 0 {
		t.Fatalf("the cost centre schema holds %d financial columns", financial)
	}

	// The optional location link must belong to the same account.
	other := env.activeCorporate(t, businessA, "vs_a", "Beta Schools")
	otherSite := env.addCorporateLocation(t, businessA, "vs_a", other.ID, "HQ", "Campus")
	if _, err = env.service.CreateCorporateCostCenter(ctx, actor, globalScope(), profile.ID,
		CorporateCostCenterInput{Code: "FIN", Name: "Finance", LocationRef: otherSite.ID}); err == nil {
		t.Fatal("a cost centre was linked to another account's location")
	}

	site := env.addCorporateLocation(t, businessA, "vs_a", profile.ID, "HQ", "Head office")
	linked, err := env.service.UpdateCorporateCostCenter(ctx, actor, globalScope(), profile.ID, center.ID,
		CorporateCostCenterInput{Code: "ops-1", Name: "Operations", LocationRef: site.ID,
			ExpectedVersion: center.Version})
	if err != nil {
		t.Fatalf("link cost centre to location: %v", err)
	}
	if linked.LocationID == nil || *linked.LocationID != site.ID {
		t.Fatalf("the location link was not applied: %v", linked.LocationID)
	}
	cleared, err := env.service.UpdateCorporateCostCenter(ctx, actor, globalScope(), profile.ID, center.ID,
		CorporateCostCenterInput{Code: "ops-1", Name: "Operations", ClearLocation: true,
			ExpectedVersion: linked.Version})
	if err != nil {
		t.Fatalf("clear cost centre location: %v", err)
	}
	if cleared.LocationID != nil {
		t.Fatal("the location link was not cleared")
	}

	archived, err := env.service.ArchiveCorporateCostCenter(ctx, actor, globalScope(), profile.ID, center.ID,
		cleared.Version)
	if err != nil {
		t.Fatalf("archive cost centre: %v", err)
	}
	if archived.Status != CorporateChildArchived {
		t.Fatalf("unexpected archived state: %s", archived.Status)
	}
	if _, err = env.service.UpdateCorporateCostCenter(ctx, actor, globalScope(), profile.ID, center.ID,
		CorporateCostCenterInput{Code: "ops-1", Name: "Revived", ExpectedVersion: archived.Version}); err == nil {
		t.Fatal("an archived cost centre was edited")
	}
}

func TestCorporateSites_ListsArePagedAndFiltered(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	profile := env.activeCorporate(t, businessA, "vs_a", "Acme Industries")

	first := env.addCorporateLocation(t, businessA, "vs_a", profile.ID, "HQ", "Head office")
	env.addCorporateLocation(t, businessA, "vs_a", profile.ID, "WH", "Warehouse")
	if _, err := env.service.ArchiveCorporateLocation(ctx, actor, globalScope(), profile.ID, first.ID, first.Version); err != nil {
		t.Fatalf("archive: %v", err)
	}

	all, _, err := env.service.ListCorporateLocations(ctx, actor, globalScope(), profile.ID, "", 0, 0)
	if err != nil || len(all) != 2 {
		t.Fatalf("expected both locations, got %d err=%v", len(all), err)
	}
	active, _, err := env.service.ListCorporateLocations(ctx, actor, globalScope(), profile.ID, CorporateChildActive, 0, 0)
	if err != nil || len(active) != 1 {
		t.Fatalf("expected one active location, got %d err=%v", len(active), err)
	}
	if _, _, err = env.service.ListCorporateLocations(ctx, actor, globalScope(), profile.ID, "RETIRED", 0, 0); err == nil {
		t.Fatal("an unsupported status filter should be rejected")
	}
	page, hasMore, err := env.service.ListCorporateLocations(ctx, actor, globalScope(), profile.ID, "", 1, 0)
	if err != nil || len(page) != 1 || !hasMore {
		t.Fatalf("pagination: %d results hasMore=%t err=%v", len(page), hasMore, err)
	}

	env.addCorporateCostCenter(t, businessA, "vs_a", profile.ID, "OPS", "Operations")
	env.addCorporateCostCenter(t, businessA, "vs_a", profile.ID, "FIN", "Finance")
	centers, hasMore, err := env.service.ListCorporateCostCenters(ctx, actor, globalScope(), profile.ID, "", 1, 0)
	if err != nil || len(centers) != 1 || !hasMore {
		t.Fatalf("cost centre pagination: %d results hasMore=%t err=%v", len(centers), hasMore, err)
	}
}
