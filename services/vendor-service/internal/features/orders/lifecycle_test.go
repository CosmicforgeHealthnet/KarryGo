package orders

import (
	"errors"
	"testing"
)

func vendorOrder(status VendorStatus, responsibility DeliveryResponsibility) VendorOrder {
	state := VendorOwnNotApplicable
	if responsibility == DeliveryVendorOwn {
		state = VendorOwnPending
	}
	return VendorOrder{
		ID:                     "cvo_1",
		Status:                 status,
		DeliveryResponsibility: responsibility,
		VendorOwnDeliveryState: state,
		CustomerPayableMinor:   120000,
		VendorPayableMinor:     100000,
	}
}

func TestApplyTransition_VendorOwnDeliveryWalksTheWholeLifecycle(t *testing.T) {
	order := vendorOrder(VendorStatusCreated, DeliveryVendorOwn)

	steps := []struct {
		transition Transition
		want       VendorStatus
		wantOwn    VendorOwnDeliveryState
	}{
		{TransitionAccept, VendorStatusPreparing, VendorOwnPending},
		{TransitionReady, VendorStatusReady, VendorOwnPending},
		{TransitionHandover, VendorStatusHandedOver, VendorOwnDispatched},
		{TransitionDeliver, VendorStatusDelivered, VendorOwnDelivered},
		{TransitionComplete, VendorStatusCompleted, VendorOwnDelivered},
	}

	for _, step := range steps {
		outcome, err := ApplyTransition(order, step.transition, "")
		if err != nil {
			t.Fatalf("%s: %v", step.transition, err)
		}
		if outcome.Status != step.want {
			t.Fatalf("%s: status=%q, want %q", step.transition, outcome.Status, step.want)
		}
		if outcome.VendorOwnDelivery != step.wantOwn {
			t.Fatalf("%s: vendor-own state=%q, want %q",
				step.transition, outcome.VendorOwnDelivery, step.wantOwn)
		}
		order.Status = outcome.Status
		order.VendorOwnDeliveryState = outcome.VendorOwnDelivery

		if step.transition == TransitionComplete {
			if !outcome.Terminal {
				t.Error("completion is not terminal")
			}
			// Settlement is PREPARED at completion. Section 17E moves no money.
			if outcome.SettlementAllocatedMinor != 100000 {
				t.Errorf("settlement prepared=%d, want 100000", outcome.SettlementAllocatedMinor)
			}
			if outcome.RefundAllocatedMinor != 0 {
				t.Errorf("completion prepared a refund of %d", outcome.RefundAllocatedMinor)
			}
		}
	}
}

// A vendor cannot assert a delivery it did not perform and cannot observe.
func TestApplyTransition_PlatformDeliveryCannotBeConfirmedByTheVendor(t *testing.T) {
	order := vendorOrder(VendorStatusHandedOver, DeliveryPlatform)

	if _, err := ApplyTransition(order, TransitionDeliver, ""); !errors.Is(err, ErrForbidden) {
		t.Fatalf("err=%v, want ErrForbidden", err)
	}
}

func TestApplyTransition_OutOfOrderCommandsAreRefused(t *testing.T) {
	cases := []struct {
		status     VendorStatus
		transition Transition
	}{
		{VendorStatusCreated, TransitionReady},
		{VendorStatusCreated, TransitionComplete},
		{VendorStatusPreparing, TransitionAccept},
		{VendorStatusPreparing, TransitionHandover},
		{VendorStatusReady, TransitionDeliver},
		{VendorStatusHandedOver, TransitionComplete},
		{VendorStatusCompleted, TransitionCancel},
		{VendorStatusCancelled, TransitionAccept},
	}

	for _, testCase := range cases {
		order := vendorOrder(testCase.status, DeliveryVendorOwn)
		if _, err := ApplyTransition(order, testCase.transition, CancelByVendor); err == nil {
			t.Errorf("%s from %s was accepted", testCase.transition, testCase.status)
		}
	}
}

func TestApplyTransition_CancelPreparesARefundOfWhatTheCustomerPaid(t *testing.T) {
	order := vendorOrder(VendorStatusPreparing, DeliveryVendorOwn)

	outcome, err := ApplyTransition(order, TransitionCancel, CancelByVendor)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if outcome.Status != VendorStatusCancelled || !outcome.Terminal {
		t.Fatalf("status=%q terminal=%v", outcome.Status, outcome.Terminal)
	}
	if outcome.RefundAllocatedMinor != 120000 {
		t.Errorf("refund prepared=%d, want 120000", outcome.RefundAllocatedMinor)
	}
	if outcome.SettlementAllocatedMinor != 0 {
		t.Errorf("cancellation prepared a settlement of %d", outcome.SettlementAllocatedMinor)
	}
	if outcome.VendorOwnDelivery != VendorOwnCancelled {
		t.Errorf("vendor-own state=%q, want CANCELLED", outcome.VendorOwnDelivery)
	}
}

func TestApplyTransition_CancelRequiresAKnownActor(t *testing.T) {
	order := vendorOrder(VendorStatusCreated, DeliveryNone)

	if _, err := ApplyTransition(order, TransitionCancel, CancelActor("SOMEBODY")); err == nil {
		t.Fatal("an unknown cancelling actor was accepted")
	}
}

func TestCancellableByCustomer(t *testing.T) {
	allowed := map[VendorStatus]bool{
		VendorStatusCreated:   true,
		VendorStatusPreparing: true,
	}
	for _, status := range []VendorStatus{
		VendorStatusCreated, VendorStatusPreparing, VendorStatusReady,
		VendorStatusHandedOver, VendorStatusDelivered, VendorStatusCompleted,
		VendorStatusCancelled,
	} {
		if CancellableByCustomer(status) != allowed[status] {
			t.Errorf("CancellableByCustomer(%s)=%v", status, CancellableByCustomer(status))
		}
	}
}

// The aggregate must never report a clean outcome for a mixed one.
func TestDeriveParentState(t *testing.T) {
	cases := []struct {
		name     string
		statuses []VendorStatus
		want     ParentState
	}{
		{"all created", []VendorStatus{VendorStatusCreated, VendorStatusCreated}, ParentStateCreated},
		{"all completed", []VendorStatus{VendorStatusCompleted, VendorStatusCompleted}, ParentStateCompleted},
		{"all cancelled", []VendorStatus{VendorStatusCancelled, VendorStatusCancelled}, ParentStateCancelled},
		{
			"mixed terminal",
			[]VendorStatus{VendorStatusCompleted, VendorStatusCancelled},
			ParentStatePartiallyCompleted,
		},
		{
			"mixed terminal with three vendors",
			[]VendorStatus{VendorStatusCompleted, VendorStatusCompleted, VendorStatusCancelled},
			ParentStatePartiallyCompleted,
		},
		{
			"one finished, one still running",
			[]VendorStatus{VendorStatusCompleted, VendorStatusPreparing},
			ParentStateInProgress,
		},
		{
			"one cancelled, others untouched",
			[]VendorStatus{VendorStatusCancelled, VendorStatusCreated},
			ParentStateInProgress,
		},
		{"single vendor completed", []VendorStatus{VendorStatusCompleted}, ParentStateCompleted},
		{"single vendor preparing", []VendorStatus{VendorStatusPreparing}, ParentStateInProgress},
	}

	for _, testCase := range cases {
		if got := DeriveParentState(testCase.statuses); got != testCase.want {
			t.Errorf("%s: state=%q, want %q", testCase.name, got, testCase.want)
		}
	}
}

func TestDeriveParentPreparedAmounts(t *testing.T) {
	// A partly-cancelled purchase refunds only the cancelled allocation. The
	// order-level fee still applies, because the customer is still buying.
	partial := []VendorOrder{
		{Status: VendorStatusCancelled, RefundAllocatedMinor: 30000},
		{Status: VendorStatusCompleted, SettlementAllocatedMinor: 50000},
	}
	refund, settlement := DeriveParentPreparedAmounts(partial, 7500)
	if refund != 30000 {
		t.Errorf("partial refund=%d, want 30000", refund)
	}
	if settlement != 50000 {
		t.Errorf("partial settlement=%d, want 50000", settlement)
	}

	// A wholly cancelled purchase refunds the order-level fee too.
	whole := []VendorOrder{
		{Status: VendorStatusCancelled, RefundAllocatedMinor: 30000},
		{Status: VendorStatusCancelled, RefundAllocatedMinor: 20000},
	}
	refund, settlement = DeriveParentPreparedAmounts(whole, 7500)
	if refund != 57500 {
		t.Errorf("whole refund=%d, want 57500", refund)
	}
	if settlement != 0 {
		t.Errorf("whole settlement=%d, want 0", settlement)
	}
}

func TestTerminalParentState(t *testing.T) {
	terminal := map[ParentState]bool{
		ParentStateCompleted:          true,
		ParentStatePartiallyCompleted: true,
		ParentStateCancelled:          true,
	}
	for _, state := range []ParentState{
		ParentStateCreated, ParentStateInProgress, ParentStateCompleted,
		ParentStatePartiallyCompleted, ParentStateCancelled,
	} {
		if TerminalParentState(state) != terminal[state] {
			t.Errorf("TerminalParentState(%s)=%v", state, TerminalParentState(state))
		}
	}
}
