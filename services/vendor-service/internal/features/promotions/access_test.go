package promotions

import (
	"cosmicforge/logistics/services/vendor-service/internal/features/production"
	"cosmicforge/logistics/shared/go/apperrors"
	"errors"
	"testing"
)

func TestDuplicateCouponIsActionableValidation(t *testing.T) {
	var appErr *apperrors.Error
	if !errors.As(promotionError(ErrCouponExists), &appErr) || appErr.Status != 422 || appErr.Message != "This coupon code is already in use." {
		t.Fatal("duplicate code must be a clear terminal validation response")
	}
}

func TestCampaignAccessRequiresCoveredBranches(t *testing.T) {
	scope := production.Scope{BranchIDs: []string{"branch-a"}}
	for _, tt := range []struct {
		name         string
		targets      []Scope
		read, manage bool
	}{
		{"global campaign", nil, false, false},
		{"covered", []Scope{{Type: "BRANCH", Ref: "branch-a"}}, true, true},
		{"outside", []Scope{{Type: "BRANCH", Ref: "branch-b"}}, false, false},
		{"shared campaign", []Scope{{Type: "BRANCH", Ref: "branch-a"}, {Type: "BRANCH", Ref: "branch-b"}}, true, false},
		{"item only", []Scope{{Type: "ITEM", Ref: "item-a"}}, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			campaign := Campaign{Scopes: tt.targets}
			if canReadCampaign(scope, campaign) != tt.read || canManageCampaign(scope, campaign) != tt.manage {
				t.Fatal("branch access mismatch")
			}
			global := production.Scope{Global: true}
			if !canReadCampaign(global, campaign) || !canManageCampaign(global, campaign) {
				t.Fatal("business-wide access refused")
			}
		})
	}
}
