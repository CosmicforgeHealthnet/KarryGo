package orders

import (
	"errors"
	"testing"
)

func TestRFQRestrictedGoodsGate(t *testing.T) {
	blocked := []map[string]any{
		{"request": "prescription medicine"},
		{"request": "OTC___MEDICINE"},
		{"request": "pharmaceutical product"},
	}
	for _, menu := range blocked {
		if err := validateRFQRestrictedGoods(RFQInput{MenuRequirements: menu}); !errors.Is(err, ErrRestrictedGoods) {
			t.Fatalf("restricted RFQ accepted: %#v err=%v", menu, err)
		}
	}
	for _, menu := range []map[string]any{
		{"request": "catering for 20 people"},
		{"request": "toothbrush and personal care pack"},
	} {
		if err := validateRFQRestrictedGoods(RFQInput{MenuRequirements: menu}); err != nil {
			t.Fatalf("ordinary RFQ rejected: %#v err=%v", menu, err)
		}
	}
}

func TestSnapshotRestrictedGoodsGateBlocksLegacyFulfilment(t *testing.T) {
	authority := snapshotRestrictedGoodsAuthority{}
	legacy := ParentOrder{VendorOrders: []VendorOrder{{Domain: "SHOP", Lines: []OrderLine{{ItemName: "Legacy prescription medicine"}}}}}
	if err := authority.ValidateOrderForFulfilment(t.Context(), legacy); !errors.Is(err, ErrRestrictedGoods) {
		t.Fatalf("legacy prohibited order was not blocked: %v", err)
	}
	ordinary := ParentOrder{VendorOrders: []VendorOrder{{Domain: "SHOP", Lines: []OrderLine{{ItemName: "Toothbrush"}}}}}
	if err := authority.ValidateOrderForFulfilment(t.Context(), ordinary); err != nil {
		t.Fatalf("ordinary order was blocked: %v", err)
	}
}
