package taxonomy

import "testing"

func TestRestrictedGoodsPolicyMedicineAndGeneralGoods(t *testing.T) {
	tests := []struct {
		name  string
		facts RestrictedGoodsFacts
		want  GoodsClassification
	}{
		{"prescription", RestrictedGoodsFacts{Name: "Prescription medicine refill"}, GoodsRegulatedExternalDomain},
		{"otc formatted", RestrictedGoodsFacts{Description: "OTC-MEDICINE"}, GoodsRegulatedExternalDomain},
		{"pharmaceutical", RestrictedGoodsFacts{ProductClass: "pharmaceutical product"}, GoodsRegulatedExternalDomain},
		{"prohibited", RestrictedGoodsFacts{Name: "Firearm ammunition"}, GoodsProhibited},
		{"restricted", RestrictedGoodsFacts{ProductClass: "Alcoholic beverage"}, GoodsRestricted},
		{"personal care", RestrictedGoodsFacts{Name: "Toothbrush", Description: "non-medicinal personal care"}, GoodsAllowed},
		{"food", RestrictedGoodsFacts{Name: "Jollof rice catering tray"}, GoodsAllowed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := EvaluateRestrictedGoods(test.facts)
			if got.Classification != test.want || got.Allowed != (test.want == GoodsAllowed) {
				t.Fatalf("decision=%+v want classification %s", got, test.want)
			}
			if got.PolicyVersion != RestrictedGoodsPolicyVersion || got.Source == "" {
				t.Fatalf("decision missing policy evidence: %+v", got)
			}
		})
	}
}

func TestRestrictedGoodsPolicyRFQPayloadIsCaseAndFormattingSafe(t *testing.T) {
	blocked := EvaluateRestrictedGoodsPayload(map[string]any{
		"menu": map[string]any{"requested_product": "PRESCRIPTION___MEDICINE"},
	})
	if blocked.Allowed || blocked.ReasonCode != ReasonMedicineExternalDomain {
		t.Fatalf("medicine RFQ was allowed: %+v", blocked)
	}
	allowed := EvaluateRestrictedGoodsPayload(map[string]any{
		"menu": []any{"catering", "toothbrush", "personal care"},
	})
	if !allowed.Allowed {
		t.Fatalf("ordinary RFQ was blocked: %+v", allowed)
	}
}
