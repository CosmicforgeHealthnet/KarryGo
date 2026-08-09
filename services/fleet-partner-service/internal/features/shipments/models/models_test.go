package models

import "testing"

func allShipmentStatuses() []string {
	return []string{
		ShipmentDraft, ShipmentReady, ShipmentPlanned, ShipmentAssigned, ShipmentInTransit,
		ShipmentCompleted, ShipmentFailed, ShipmentPartiallyCompleted, ShipmentCancelled,
	}
}

func TestCanTransitionShipment_Matrix(t *testing.T) {
	allowed := map[[2]string]bool{
		{ShipmentDraft, ShipmentReady}:                  true,
		{ShipmentDraft, ShipmentCancelled}:              true,
		{ShipmentReady, ShipmentPlanned}:                true,
		{ShipmentReady, ShipmentCancelled}:              true,
		{ShipmentPlanned, ShipmentAssigned}:             true,
		{ShipmentPlanned, ShipmentCancelled}:            true,
		{ShipmentAssigned, ShipmentInTransit}:           true,
		{ShipmentAssigned, ShipmentFailed}:              true,
		{ShipmentAssigned, ShipmentPartiallyCompleted}:  true,
		{ShipmentInTransit, ShipmentCompleted}:          true,
		{ShipmentInTransit, ShipmentFailed}:             true,
		{ShipmentInTransit, ShipmentPartiallyCompleted}: true,
	}
	for _, from := range allShipmentStatuses() {
		for _, to := range allShipmentStatuses() {
			want := allowed[[2]string{from, to}]
			if got := CanTransitionShipment(from, to); got != want {
				t.Errorf("CanTransitionShipment(%s, %s) = %v, want %v", from, to, got, want)
			}
		}
	}
}

// TestAssignedHasNoDirectCancelEdge pins the deliberate gap: once a shipment
// is ASSIGNED a private assignment or canonical execution may already exist,
// so a direct cancel is refused there — only a later orchestration slice may
// stop it, through the owning execution service.
func TestAssignedHasNoDirectCancelEdge(t *testing.T) {
	if CanTransitionShipment(ShipmentAssigned, ShipmentCancelled) {
		t.Error("ASSIGNED must not have a direct edge to CANCELLED")
	}
	if CanTransitionShipment(ShipmentInTransit, ShipmentCancelled) {
		t.Error("IN_TRANSIT must not have a direct edge to CANCELLED")
	}
}

func TestTerminalShipment(t *testing.T) {
	for _, status := range []string{ShipmentCompleted, ShipmentFailed, ShipmentPartiallyCompleted, ShipmentCancelled} {
		if !TerminalShipment(status) {
			t.Errorf("%s must be terminal", status)
		}
	}
	for _, status := range []string{ShipmentDraft, ShipmentReady, ShipmentPlanned, ShipmentAssigned, ShipmentInTransit} {
		if TerminalShipment(status) {
			t.Errorf("%s must not be terminal", status)
		}
	}
}

func TestShipmentLockedAndEditable(t *testing.T) {
	if ShipmentLocked(ShipmentDraft) {
		t.Error("DRAFT must not be locked")
	}
	for _, status := range []string{ShipmentReady, ShipmentPlanned, ShipmentAssigned, ShipmentCompleted} {
		if !ShipmentLocked(status) {
			t.Errorf("%s must be locked", status)
		}
	}
	if !ShipmentEditable(ShipmentDraft) || !ShipmentEditable(ShipmentReady) {
		t.Error("DRAFT and READY must remain editable for cargo")
	}
	for _, status := range []string{ShipmentPlanned, ShipmentAssigned, ShipmentCompleted, ShipmentCancelled} {
		if ShipmentEditable(status) {
			t.Errorf("%s must not be editable for cargo", status)
		}
	}
}

func TestValidHandlingCategory(t *testing.T) {
	for _, category := range []string{HandlingGeneral, HandlingDangerousGoods, HandlingMedical, HandlingColdChain,
		HandlingFragile, HandlingHighValue, HandlingFoodAgriculture, HandlingRestricted} {
		if !ValidHandlingCategory(category) {
			t.Errorf("%s must be a valid handling category", category)
		}
	}
	if ValidHandlingCategory("NOT_A_CATEGORY") {
		t.Error("an unrecognized category must not validate")
	}
}

func TestRequiresComplianceReview(t *testing.T) {
	for _, category := range []string{HandlingDangerousGoods, HandlingMedical, HandlingRestricted} {
		if !RequiresComplianceReview(category) {
			t.Errorf("%s must require compliance review", category)
		}
	}
	for _, category := range []string{HandlingGeneral, HandlingColdChain, HandlingFragile, HandlingHighValue, HandlingFoodAgriculture} {
		if RequiresComplianceReview(category) {
			t.Errorf("%s must not require compliance review", category)
		}
	}
}

func TestRestrictionBlocksShipment(t *testing.T) {
	blocking := []string{RestrictionRequiresReview, RestrictionRejected, RestrictionUnknown}
	for _, status := range blocking {
		if !RestrictionBlocksShipment(status) {
			t.Errorf("%s must block the shipment — fail closed on uncertainty", status)
		}
	}
	nonBlocking := []string{RestrictionNotRestricted, RestrictionApproved}
	for _, status := range nonBlocking {
		if RestrictionBlocksShipment(status) {
			t.Errorf("%s must not block the shipment", status)
		}
	}
}

func TestValidateQuantity(t *testing.T) {
	valid := []string{"1", "0.5", "2.5", "1000000", "0.000001", "123456789012"}
	for _, q := range valid {
		if err := ValidateQuantity(q); err != nil {
			t.Errorf("ValidateQuantity(%q) unexpected error: %v", q, err)
		}
	}
	invalid := []string{"0", "-1", "abc", "", "1.2.3", "1,5", "1e10"}
	for _, q := range invalid {
		if err := ValidateQuantity(q); err == nil {
			t.Errorf("ValidateQuantity(%q) expected an error", q)
		}
	}
}

func TestValidateCurrency(t *testing.T) {
	if !ValidateCurrency("NGN") {
		t.Error("NGN must be a valid currency")
	}
	for _, code := range []string{"ngn", "NG", "NGNN", "123", ""} {
		if ValidateCurrency(code) {
			t.Errorf("%q must not be a valid currency", code)
		}
	}
}
