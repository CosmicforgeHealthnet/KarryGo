package pricingclient

import (
	"errors"
	"testing"
	"time"
)

func TestFuelReportValidation(t *testing.T) {
	ok := FuelReport{ProviderRef: "p1", FuelType: FuelPetrol, PricePerLitreKobo: 95_000, LatE6: 6_524_379, LngE6: 3_379_206, ReportedAt: time.Now()}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid report refused: %v", err)
	}
	for name, mutate := range map[string]func(*FuelReport){
		"no driver":    func(r *FuelReport) { r.ProviderRef = "" },
		"unknown fuel": func(r *FuelReport) { r.FuelType = "KEROSENE" },
		"typo price":   func(r *FuelReport) { r.PricePerLitreKobo = 950 },
		"absurd price": func(r *FuelReport) { r.PricePerLitreKobo = 95_000_000 },
		"null island":  func(r *FuelReport) { r.LatE6, r.LngE6 = 0, 0 },
		"no time":      func(r *FuelReport) { r.ReportedAt = time.Time{} },
	} {
		r := ok
		mutate(&r)
		if err := r.Validate(); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("%s: want ErrInvalidRequest, got %v", name, err)
		}
	}
	if NormalizeFuelType(" pms ") != FuelPetrol || NormalizeFuelType("ago") != FuelDiesel || NormalizeFuelType("x") != "" {
		t.Fatal("fuel type normalization")
	}
}
