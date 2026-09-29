package searchprojection

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/publictrust"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestCustomerTrustOverlaysCachedSearchIndependentlyOfRating(t *testing.T) {
	status := "VERIFIED"
	h := &InternalHandler{}
	h.SetPublicTrustReader(func(_ context.Context, business, branch string) ([]publictrust.Fact, error) {
		if business != "b" || branch != "br" {
			t.Fatalf("incorrect scope %s/%s", business, branch)
		}
		return []publictrust.Fact{{Dimension: "BUSINESS", Status: status}}, nil
	})
	cached := FoodEngineSearchResponse{Hits: []FoodEngineHit{{BusinessRef: "b", BranchRef: "br", Score: 1}}}
	for _, state := range []string{"VERIFIED", "SUSPENDED", "EXPIRED", "REJECTED", "NOT_STARTED"} {
		status = state
		for _, rating := range []int64{1, 100000} {
			cached.Hits[0].Score = rating
			view := customerFoodSearchView(cached)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			if err := h.addPublicTrust(c, view); err != nil {
				t.Fatal(err)
			}
			facts := view["hits"].([]gin.H)[0]["trust"].([]publictrust.Fact)
			if facts[0].Status != state {
				t.Fatalf("cached/rating badge: %+v", facts)
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("cacheable trust response")
			}
		}
	}
}
