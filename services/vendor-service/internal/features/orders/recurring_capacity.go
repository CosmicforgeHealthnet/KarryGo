package orders

import (
	"encoding/json"
	"time"
)

func recurringGenerationDates(schedule RecurringSchedule, now time.Time, loc *time.Location) (time.Time, time.Time) {
	date := func(value time.Time) time.Time {
		return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, loc)
	}
	start := date(schedule.StartsOn)
	today := date(now.In(loc))
	if start.Before(today) {
		start = today
	}
	end := today.AddDate(0, 0, int(schedule.AdvanceHorizonDays))
	if schedule.EndsOn != nil && date(*schedule.EndsOn).Before(end) {
		end = date(*schedule.EndsOn)
	}
	return start, end
}

func recurringOccurrenceCapacity(schedule RecurringSchedule, localDate time.Time) (map[string]any, CapacityRequirement, error) {
	raw, err := json.Marshal(schedule.ProductionRequirement)
	var cap CapacityRequirement
	if err != nil || json.Unmarshal(raw, &cap) != nil {
		return nil, cap, ErrSection21Capacity
	}
	if !cap.Required {
		return schedule.ProductionRequirement, cap, nil
	}
	loc, err := time.LoadLocation(schedule.Timezone)
	if err != nil || schedule.StartsOn.IsZero() || localDate.IsZero() || cap.WindowStart.IsZero() || !cap.WindowEnd.After(cap.WindowStart) || len(cap.Allocations) == 0 || cap.BranchID != "" && cap.BranchID != schedule.BranchID {
		return nil, cap, ErrSection21Capacity
	}
	// StartsOn is a database DATE. Its calendar components anchor the template.
	anchor := time.Date(schedule.StartsOn.Year(), schedule.StartsOn.Month(), schedule.StartsOn.Day(), 0, 0, 0, 0, time.UTC)
	target := time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 0, 0, 0, 0, time.UTC)
	days := int(target.Sub(anchor) / (24 * time.Hour))
	shift := func(value time.Time) (time.Time, bool) {
		local := value.In(loc)
		date := time.Date(local.Year(), local.Month(), local.Day()+days, 0, 0, 0, 0, time.UTC)
		out := time.Date(date.Year(), date.Month(), date.Day(), local.Hour(), local.Minute(), local.Second(), local.Nanosecond(), loc)
		valid := out.Year() == date.Year() && out.Month() == date.Month() && out.Day() == date.Day() && out.Hour() == local.Hour() && out.Minute() == local.Minute()
		return out.UTC(), valid
	}
	var startOK, endOK bool
	cap.WindowStart, startOK = shift(cap.WindowStart)
	cap.WindowEnd, endOK = shift(cap.WindowEnd)
	if !startOK || !endOK || !cap.WindowEnd.After(cap.WindowStart) {
		return nil, cap, ErrSection21Capacity
	}
	snapshot := make(map[string]any, len(schedule.ProductionRequirement))
	for key, value := range schedule.ProductionRequirement {
		snapshot[key] = value
	}
	snapshot["window_start"] = cap.WindowStart
	snapshot["window_end"] = cap.WindowEnd
	return snapshot, cap, nil
}
