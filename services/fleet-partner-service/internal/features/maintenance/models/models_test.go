package models

import (
	"testing"
	"time"

	assetmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/assets/models"
)

func ptr(v float64) *float64 { return &v }

func TestMaintenanceScheduleDueModes(t *testing.T) {
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)
	asset := assetmodels.Asset{Odometer: 100, OperatingHours: 50}
	cases := []struct {
		s    Schedule
		want bool
	}{{Schedule{TriggerType: "DATE", NextDueDate: &past}, true}, {Schedule{TriggerType: "DATE", NextDueDate: &future}, false}, {Schedule{TriggerType: "MILEAGE", NextDueOdometer: ptr(100)}, true}, {Schedule{TriggerType: "OPERATING_HOURS", NextDueHours: ptr(51)}, false}, {Schedule{TriggerType: "COMBINED", NextDueDate: &future, NextDueOdometer: ptr(99)}, true}}
	for _, c := range cases {
		if got := ScheduleDue(c.s, asset, now); got != c.want {
			t.Errorf("%+v=%v", c.s, got)
		}
	}
}
