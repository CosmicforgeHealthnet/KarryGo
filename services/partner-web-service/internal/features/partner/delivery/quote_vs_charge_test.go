package delivery

import (
	"testing"

	gatewaypricing "cosmicforge/logistics/services/api-gateway/internal/pkg/pricing"
)

// Characterization test for a KNOWN DEFECT, pinned ahead of the Central Pricing
// Engine migration.
//
// The partner API prices the same delivery with two different formulas:
//
//	POST /api/v1/partner/v1/quotes     -> internal/pkg/pricing.CalculateFare
//	                                      (config-driven: PRICING_BASE_KOBO etc.)
//	POST /api/v1/partner/v1/deliveries -> calculateFareKobo (shared/go/dispatchpricing)
//
// So a partner is QUOTED one amount and CHARGED a higher one. The comment in
// pricing.go ("the single source of truth ... can never quote different fares")
// is true only of the charge path; it does not cover the quote handler.
//
// This test documents the gap so that it cannot widen unnoticed, and so the
// shadow-comparison phase expects a MATERIAL difference on this one surface
// rather than treating it as a regression. At cutover both surfaces move to the
// central engine and the quote is corrected upward to match what is charged
// (plan decision D2), at which point this test is deleted along with
// internal/pkg/pricing.
func TestPartnerQuoteAndChargeDisagree(t *testing.T) {
	// The production default configuration of the quote engine.
	quoteCfg := gatewaypricing.Config{
		BaseKobo:          50000,
		StandardPerKmKobo: 15000,
		ExpressPerKmKobo:  25000,
		LargeMultiplier:   1.5, // NOTE: .env.example ships 1.2 instead.
	}

	cases := []struct {
		name        string
		km          float64
		size        string
		tier        string
		wantQuote   int64
		wantCharged int64
	}{
		{"5km small standard", 5, "small", "standard", 125_000, 170_000},
		{"10km small standard", 10, "small", "standard", 200_000, 260_000},
		{"10km large standard", 10, "large", "standard", 300_000, 330_000},
		{"10km small express", 10, "small", "express", 300_000, 351_000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			quoted := gatewaypricing.CalculateFare(quoteCfg, tc.km, tc.tier, tc.size)
			charged := calculateFareKobo(tc.km, tc.size, tc.tier)

			if quoted != tc.wantQuote {
				t.Fatalf("quoted = %d kobo, want %d", quoted, tc.wantQuote)
			}
			if charged != tc.wantCharged {
				t.Fatalf("charged = %d kobo, want %d", charged, tc.wantCharged)
			}
			if quoted == charged {
				t.Fatalf("quote and charge now agree (%d kobo) -- if this was fixed "+
					"deliberately, delete this test and update plan decision D2", quoted)
			}

			t.Logf("partner is under-quoted by %d kobo (quoted %d, charged %d)",
				charged-quoted, quoted, charged)
		})
	}
}
