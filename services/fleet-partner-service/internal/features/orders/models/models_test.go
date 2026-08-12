package models

import "testing"

func allOrderStatuses() []string {
	return []string{
		StatusDraft, StatusAwaitingApproval, StatusConfirmed, StatusPlanned, StatusAssigned,
		StatusInProgress, StatusCompleted, StatusCancelled, StatusFailed, StatusPartiallyCompleted,
	}
}

func TestCanTransition_Matrix(t *testing.T) {
	allowed := map[[2]string]bool{
		{StatusDraft, StatusAwaitingApproval}:        true,
		{StatusDraft, StatusConfirmed}:               true,
		{StatusDraft, StatusCancelled}:               true,
		{StatusAwaitingApproval, StatusConfirmed}:    true,
		{StatusAwaitingApproval, StatusCancelled}:    true,
		{StatusConfirmed, StatusPlanned}:             true,
		{StatusConfirmed, StatusCancelled}:           true,
		{StatusPlanned, StatusAssigned}:              true,
		{StatusPlanned, StatusCancelled}:             true,
		{StatusAssigned, StatusInProgress}:           true,
		{StatusAssigned, StatusFailed}:               true,
		{StatusAssigned, StatusPartiallyCompleted}:   true,
		{StatusInProgress, StatusCompleted}:          true,
		{StatusInProgress, StatusFailed}:             true,
		{StatusInProgress, StatusPartiallyCompleted}: true,
	}
	for _, from := range allOrderStatuses() {
		for _, to := range allOrderStatuses() {
			want := allowed[[2]string{from, to}]
			if got := CanTransition(from, to); got != want {
				t.Errorf("CanTransition(%s, %s) = %v, want %v", from, to, got, want)
			}
		}
	}
}

// TestAssignedAndInProgressHaveNoDirectCancelEdge pins the deliberate gap in
// the matrix: once an order is ASSIGNED or IN_PROGRESS an execution request
// may already exist, so a direct order-level cancel is refused — only a
// later orchestration slice, going through the owning execution service, may
// stop it.
func TestAssignedAndInProgressHaveNoDirectCancelEdge(t *testing.T) {
	if CanTransition(StatusAssigned, StatusCancelled) {
		t.Error("ASSIGNED must not have a direct edge to CANCELLED")
	}
	if CanTransition(StatusInProgress, StatusCancelled) {
		t.Error("IN_PROGRESS must not have a direct edge to CANCELLED")
	}
}

func TestTerminal(t *testing.T) {
	for _, status := range []string{StatusCompleted, StatusCancelled, StatusFailed, StatusPartiallyCompleted} {
		if !Terminal(status) {
			t.Errorf("%s must be terminal", status)
		}
	}
	for _, status := range []string{StatusDraft, StatusAwaitingApproval, StatusConfirmed, StatusPlanned, StatusAssigned, StatusInProgress} {
		if Terminal(status) {
			t.Errorf("%s must not be terminal", status)
		}
	}
}

func TestCancellable(t *testing.T) {
	for _, status := range []string{StatusDraft, StatusAwaitingApproval, StatusConfirmed, StatusPlanned} {
		if !Cancellable(status) {
			t.Errorf("%s must be directly cancellable", status)
		}
	}
	for _, status := range []string{StatusAssigned, StatusInProgress, StatusCompleted, StatusCancelled, StatusFailed, StatusPartiallyCompleted} {
		if Cancellable(status) {
			t.Errorf("%s must not be directly cancellable", status)
		}
	}
}

func TestValidSourcePriorityStopType(t *testing.T) {
	for _, source := range []string{SourceManual, SourceQuotation, SourceContract, SourceRecurringTemplate, SourceBulkImport, SourcePartnerIntegration} {
		if !ValidSource(source) {
			t.Errorf("%s must be a valid source", source)
		}
	}
	if ValidSource("NOT_A_SOURCE") {
		t.Error("an unrecognized source must not validate")
	}

	for _, priority := range []string{PriorityLow, PriorityStandard, PriorityHigh, PriorityUrgent} {
		if !ValidPriority(priority) {
			t.Errorf("%s must be a valid priority", priority)
		}
	}
	if ValidPriority("CRITICAL") {
		t.Error("an unrecognized priority must not validate")
	}

	for _, stopType := range []string{StopTypePickup, StopTypeDelivery, StopTypeWaypoint, StopTypeDepot,
		StopTypeWarehouse, StopTypeCustoms, StopTypeInspection, StopTypeService, StopTypeReturn} {
		if !ValidStopType(stopType) {
			t.Errorf("%s must be a valid stop type", stopType)
		}
	}
	if ValidStopType("TELEPORT") {
		t.Error("an unrecognized stop type must not validate")
	}
}

func float64Ptr(v float64) *float64 { return &v }

func TestValidateCoordinates(t *testing.T) {
	cases := []struct {
		name    string
		lat     *float64
		lng     *float64
		wantErr bool
	}{
		{"both nil", nil, nil, false},
		{"valid pair", float64Ptr(6.5244), float64Ptr(3.3792), false},
		{"lat without lng", float64Ptr(6.5244), nil, true},
		{"lng without lat", nil, float64Ptr(3.3792), true},
		{"lat out of range", float64Ptr(120), float64Ptr(3.3792), true},
		{"lng out of range", float64Ptr(6.5244), float64Ptr(-200), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCoordinates(tc.lat, tc.lng)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateCoordinates(%v, %v) error = %v, wantErr %v", tc.lat, tc.lng, err, tc.wantErr)
			}
		})
	}
}

func TestValidateServiceFields(t *testing.T) {
	schema := JSONMap{
		"receiverName": map[string]any{"type": "string", "required": true},
		"fragile":      map[string]any{"type": "boolean", "required": false},
	}
	t.Run("required field present", func(t *testing.T) {
		fields := JSONMap{"receiverName": "Ada Okafor"}
		if err := ValidateServiceFields(schema, fields); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
	t.Run("required field missing", func(t *testing.T) {
		fields := JSONMap{}
		if err := ValidateServiceFields(schema, fields); err == nil {
			t.Error("expected an error for a missing required field")
		}
	})
	t.Run("required field empty string", func(t *testing.T) {
		fields := JSONMap{"receiverName": ""}
		if err := ValidateServiceFields(schema, fields); err == nil {
			t.Error("expected an error for an empty required field")
		}
	})
	t.Run("optional field omitted is fine", func(t *testing.T) {
		fields := JSONMap{"receiverName": "Ada Okafor"}
		if err := ValidateServiceFields(schema, fields); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}
