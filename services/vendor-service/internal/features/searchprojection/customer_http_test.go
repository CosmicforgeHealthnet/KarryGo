package searchprojection

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCustomerSearchNeverSerializesRankingOrOperationalSourceEvidence(t *testing.T) {
	food := FoodEngineSearchResponse{ProfileCode: "private-profile", ProfileVersion: 99, Hits: []FoodEngineHit{{DocumentID: "private-document", Name: "Meal", Score: 123, SourceEvidence: SearchSourceEvidence{InventorySource: "private-stock-authority", CommercialSource: "private-commercial"}}}}
	shop := ShopEngineSearchResponse{ProfileCode: "private-profile", Hits: []ShopEngineHit{{DocumentID: "private-document", Name: "Goods", Score: 321, SourceEvidence: SearchSourceEvidence{AvailabilitySource: "private-availability"}}}}
	for _, view := range []any{customerFoodSearchView(food), customerShopSearchView(shop)} {
		raw, err := json.Marshal(view)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"private-", "profile_code", "profile_version", "source_evidence", "score", "document_id"} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("internal search field leaked: %s", forbidden)
			}
		}
	}
}
