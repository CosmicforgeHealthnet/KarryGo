package integration

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"testing"
	"time"

	"cosmicforge/logistics/services/pricing-service/internal/engine"
	fuelrepos "cosmicforge/logistics/services/pricing-service/internal/features/fuel/repositories"
	fuelusecases "cosmicforge/logistics/services/pricing-service/internal/features/fuel/usecases"
	geomodels "cosmicforge/logistics/services/pricing-service/internal/features/geography/models"
	policymodels "cosmicforge/logistics/services/pricing-service/internal/features/policies/models"
	policyusecases "cosmicforge/logistics/services/pricing-service/internal/features/policies/usecases"
	quotemodels "cosmicforge/logistics/services/pricing-service/internal/features/quotes/models"
	quoterepos "cosmicforge/logistics/services/pricing-service/internal/features/quotes/repositories"
	quoteusecases "cosmicforge/logistics/services/pricing-service/internal/features/quotes/usecases"
	"cosmicforge/logistics/services/pricing-service/internal/platform/canonical"
	"cosmicforge/logistics/services/pricing-service/internal/platform/ids"
	"cosmicforge/logistics/shared/go/pricingclient"
)

type fuelLocator struct{ quotes *quoteusecases.Service }

func (l fuelLocator) Locate(ctx context.Context, lat, lng float64) (fuelusecases.Place, bool, error) {
	p, err := l.quotes.Locate(ctx, lat, lng)
	if errors.Is(err, quotemodels.ErrNoMarket) {
		return fuelusecases.Place{}, false, nil
	}
	return fuelusecases.Place{MarketID: p.MarketID, MarketCode: p.MarketCode, ZoneID: p.ZoneID}, err == nil, err
}

// Drivers report petrol 10% above what the taxi rates were set at: the index
// proposes a per-km rise for approval, proposes nothing more while that waits,
// and treats the approved rates as the new starting point.
func TestFuelIndexProposesATariffChangeForApproval(t *testing.T) {
	h := newPolicyHarness(t)
	ctx := context.Background()
	where := h.newPlace(t)

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	lat, lng := -50+rng.Float64()*100, -150+rng.Float64()*300
	shape := geomodels.CircleShape{Type: geomodels.ShapeCircle, CenterLatE6: canonical.Micro(lat), CenterLngE6: canonical.Micro(lng), RadiusM: 3000}
	raw, _ := json.Marshal(shape)
	hash, _ := canonical.Hash(shape)
	if err := h.geo.AddZoneVersion(ctx, geomodels.ZoneVersion{
		ID: ids.New(ids.PrefixZoneVersion), ZoneID: where.zoneID, VersionNo: 1, Shape: raw, ShapeHash: hash, CreatedBy: h.editor,
	}); err != nil {
		t.Fatal(err)
	}

	tariff := h.draft(t, engine.KindTaxiTariff, policymodels.LevelMarket, where.marketID, seedBody(t, "taxi_tariff_ng.json", nil), "V1 baseline")
	h.approveAndActivate(t, tariff, time.Time{})
	rules := h.draft(t, engine.KindFuelAdjustment, policymodels.LevelMarket, where.marketID, seedBody(t, "fuel_adjustment_ng.json", map[string]any{
		"fuel_for": map[string]string{"taxi.tariff": "PETROL"}, "min_reports": 3,
	}), "fuel rules")
	h.approveAndActivate(t, rules, time.Time{})

	quotes := quoteusecases.NewService(quoterepos.NewStore(h.pool), h.policies, h.rollouts)
	fuel := fuelusecases.NewService(fuelrepos.NewStore(h.pool), fuelLocator{quotes}, h.policies, 3)

	report := func(driver string, kobo int64, atLat, atLng float64) pricingclient.FuelReportResult {
		t.Helper()
		result, err := fuel.Report(ctx, "driver-taxi-service", pricingclient.FuelReport{
			ProviderRef: driver, FuelType: "pms", PricePerLitreKobo: kobo,
			LatE6: canonical.Micro(atLat), LngE6: canonical.Micro(atLng), ReportedAt: time.Now(),
		})
		if err != nil {
			t.Fatalf("report: %v", err)
		}
		return result
	}
	drivers := []string{ids.New("d"), ids.New("d"), ids.New("d"), ids.New("d")}
	for i, kobo := range []int64{104_500, 104_500, 104_500, 104_000} {
		if r := report(drivers[i], kobo, lat, lng); r.Duplicate || r.MarketCode == "" {
			t.Fatalf("report %d = %+v", i, r)
		}
	}
	// A second report the same day does not count twice.
	if r := report(drivers[0], 300_000, lat, lng); !r.Duplicate {
		t.Fatal("a driver reported twice in a day")
	}
	if _, err := fuel.Report(ctx, "driver-taxi-service", pricingclient.FuelReport{
		ProviderRef: drivers[1], FuelType: "PETROL", PricePerLitreKobo: 900, LatE6: 1, LngE6: 1, ReportedAt: time.Now(),
	}); err == nil {
		t.Fatal("a typo price was accepted")
	}

	if _, err := fuel.ComputeIndex(ctx); err != nil {
		t.Fatal(err)
	}
	var median int64
	var count int
	if err := h.pool.QueryRow(ctx, `SELECT median_kobo, report_count FROM pricing_fuel_index WHERE market_id = $1 AND fuel_type = 'PETROL'`,
		where.marketID).Scan(&median, &count); err != nil {
		t.Fatal(err)
	}
	if median != 104_500 || count != 4 {
		t.Fatalf("index = %d from %d reports", median, count)
	}

	if _, err := fuel.Propose(ctx); err != nil {
		t.Fatal(err)
	}
	proposals := func() int {
		var n int
		_ = h.pool.QueryRow(ctx, `SELECT count(*) FROM pricing_fuel_proposals WHERE market_id = $1`, where.marketID).Scan(&n)
		return n
	}
	if proposals() != 1 {
		t.Fatalf("proposals = %d, want 1", proposals())
	}
	var versionID, status, createdBy string
	var changeBPS int64
	if err := h.pool.QueryRow(ctx, `
		SELECT p.proposed_version_id, v.status, v.created_by, p.change_bps
		FROM pricing_fuel_proposals p JOIN pricing_policy_versions v ON v.id = p.proposed_version_id
		WHERE p.market_id = $1`, where.marketID).Scan(&versionID, &status, &createdBy, &changeBPS); err != nil {
		t.Fatal(err)
	}
	if status != policymodels.StatusSubmitted || createdBy != fuelusecases.SystemOperator || changeBPS != 1000 {
		t.Fatalf("proposal version %s by %s, change %d bps", status, createdBy, changeBPS)
	}

	// The fuel index cannot approve its own proposal.
	if err := h.policies.Approve(ctx, versionID, fuelusecases.SystemOperator, "pricing-ops-cli", "self", unique("req")); err == nil {
		t.Fatal("the fuel index approved its own proposal")
	}
	// Nothing more is proposed while it waits for a decision.
	if _, err := fuel.Propose(ctx); err != nil {
		t.Fatal(err)
	}
	if proposals() != 1 {
		t.Fatal("re-proposed while a proposal was waiting")
	}

	// A person approves and activates it: per-km is up 4% (40% of +10%).
	if err := h.policies.Approve(ctx, versionID, h.approver, "pricing-ops-cli", "fuel moved", unique("req")); err != nil {
		t.Fatal(err)
	}
	if _, err := h.policies.Activate(ctx, policyusecases.ActivateInput{
		VersionID: versionID, PerformedBy: h.approver, ActorCaller: "pricing-ops-cli", Reason: "fuel moved", RequestID: unique("req"),
	}); err != nil {
		t.Fatal(err)
	}
	live, err := h.policies.Resolve(ctx, engine.KindTaxiTariff, policymodels.GeographyChain{MarketID: where.marketID}, time.Time{})
	if err != nil || live.Version.ID != versionID {
		t.Fatalf("live tariff = %s, %v", live.Version.ID, err)
	}
	before, _ := engine.Decode(engine.KindTaxiTariff, 1, tariff.Body)
	after, _ := engine.Decode(engine.KindTaxiTariff, 1, live.Version.Body)
	for name, rate := range before.(engine.TaxiTariffV1).Classes {
		want := (rate.PerKmKobo*104/100 + 50) / 100 * 100
		if got := after.(engine.TaxiTariffV1).Classes[name].PerKmKobo; got != want {
			t.Fatalf("%s per-km %d, want %d", name, got, want)
		}
	}
	// The new rates were set at today's fuel price: no further change.
	if _, err := fuel.Propose(ctx); err != nil {
		t.Fatal(err)
	}
	if proposals() != 1 {
		t.Fatal("proposed again against rates already set at this fuel price")
	}
}
