// Package models defines recurring private-order schedules and occurrences.
package models

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	TypeDaily       = "DAILY"
	TypeWeekly      = "WEEKLY"
	TypeMonthly     = "MONTHLY"
	TypeCustom      = "CUSTOM_RRULE"
	StatusDraft     = "DRAFT"
	StatusActive    = "ACTIVE"
	StatusInactive  = "INACTIVE"
	StatusCompleted = "COMPLETED"
	StatusCancelled = "CANCELLED"
)

type TemplateData struct {
	CustomerReference   string         `json:"customerReference"`
	ServiceFields       map[string]any `json:"serviceFields"`
	Priority            string         `json:"priority"`
	SpecialInstructions string         `json:"specialInstructions"`
	DurationMinutes     int            `json:"durationMinutes"`
}
type Template struct {
	ID, OrganizationID, BranchID, CustomerID, ServiceCatalogItemID, ContractID string
	Data                                                                       TemplateData
	RecurrenceType, NormalizedRRule, Timezone                                  string
	StartDate                                                                  time.Time
	EndDate                                                                    *time.Time
	NextRunAt                                                                  time.Time
	MissedRunPolicy, GeneratedOrderInitialState, Status                        string
	Version                                                                    int64
	Attempts                                                                   int
	LastErrorCode                                                              string
	CreatedBy, UpdatedBy                                                       string
	CreatedAt, UpdatedAt                                                       time.Time
	ActivatedAt, DeactivatedAt                                                 *time.Time
}
type Occurrence struct {
	ID, OrganizationID, TemplateID, OccurrenceKey string
	ScheduledFor                                  time.Time
	Status, GeneratedOrderID                      string
	Attempts                                      int
	LastErrorCode, LeaseOwner                     string
	LeaseExpiresAt                                *time.Time
	CreatedAt, UpdatedAt                          time.Time
}

func ParseRule(kind, raw string) (freq string, interval int, normalized string, err error) {
	interval = 1
	kind = strings.ToUpper(strings.TrimSpace(kind))
	switch kind {
	case TypeDaily:
		freq = "DAILY"
	case TypeWeekly:
		freq = "WEEKLY"
	case TypeMonthly:
		freq = "MONTHLY"
	case TypeCustom:
		parts := strings.Split(strings.ToUpper(strings.TrimSpace(raw)), ";")
		for _, p := range parts {
			kv := strings.SplitN(p, "=", 2)
			if len(kv) != 2 {
				continue
			}
			switch kv[0] {
			case "FREQ":
				freq = kv[1]
			case "INTERVAL":
				interval, _ = strconv.Atoi(kv[1])
			}
		}
	default:
		return "", 0, "", fmt.Errorf("unsupported recurrence type")
	}
	if freq != "DAILY" && freq != "WEEKLY" && freq != "MONTHLY" || interval < 1 || interval > 365 {
		return "", 0, "", fmt.Errorf("invalid recurrence rule")
	}
	normalized = fmt.Sprintf("FREQ=%s;INTERVAL=%d", freq, interval)
	return
}
func Next(after time.Time, timezone, freq string, interval int) (time.Time, error) {
	loc, e := time.LoadLocation(timezone)
	if e != nil {
		return time.Time{}, e
	}
	local := after.In(loc)
	switch freq {
	case "DAILY":
		local = local.AddDate(0, 0, interval)
	case "WEEKLY":
		local = local.AddDate(0, 0, 7*interval)
	case "MONTHLY":
		day := local.Day()
		next := local.AddDate(0, interval, 0)
		for next.Day() != day && next.Day() > 1 {
			next = next.AddDate(0, 0, -1)
		}
		local = next
	default:
		return time.Time{}, fmt.Errorf("unsupported frequency")
	}
	return local.UTC(), nil
}
func OccurrenceKey(templateID string, scheduled time.Time) string {
	return templateID + ":" + scheduled.UTC().Format("20060102T150405.000000000Z")
}
