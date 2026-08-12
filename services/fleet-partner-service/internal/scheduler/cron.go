// Package scheduler implements Section 27 scheduled tasks: computing when a
// task is next due (CRON/INTERVAL/ONCE, timezone- and DST-aware), claiming
// due tasks with FOR UPDATE SKIP LOCKED, and enqueueing exactly one durable
// fleet_partner_jobs row per occurrence via the existing
// internal/jobs.Queue — the scheduler never executes domain work itself.
package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// cronField is one parsed field of a 5-field cron expression: minute, hour,
// day-of-month, month, day-of-week. Supported syntax: "*", a single integer,
// a comma-separated list of integers, and a step ("*/N"). Ranges ("1-5") and
// combined list-of-ranges are intentionally not supported — this is the
// bounded subset Section 27's own required schedules (interval-like
// sweeps, daily/hourly maintenance windows) actually need, documented
// explicitly rather than silently accepting syntax it does not fully honor.
type cronField struct {
	wildcard bool
	step     int
	values   map[int]bool
}

func parseCronField(raw string, min, max int) (cronField, error) {
	raw = strings.TrimSpace(raw)
	if raw == "*" {
		return cronField{wildcard: true}, nil
	}
	if strings.HasPrefix(raw, "*/") {
		step, err := strconv.Atoi(raw[2:])
		if err != nil || step <= 0 {
			return cronField{}, fmt.Errorf("scheduler: invalid step %q", raw)
		}
		return cronField{step: step}, nil
	}
	values := map[int]bool{}
	for _, part := range strings.Split(raw, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < min || n > max {
			return cronField{}, fmt.Errorf("scheduler: invalid field value %q (want %d-%d)", part, min, max)
		}
		values[n] = true
	}
	if len(values) == 0 {
		return cronField{}, fmt.Errorf("scheduler: empty field")
	}
	return cronField{values: values}, nil
}

func (f cronField) matches(v int) bool {
	if f.wildcard {
		return true
	}
	if f.step > 0 {
		return v%f.step == 0
	}
	return f.values[v]
}

// CronSchedule is a parsed 5-field cron expression: minute hour dom month dow.
type CronSchedule struct {
	minute, hour, dom, month, dow cronField
	raw                           string
}

// ParseCron parses a standard 5-field cron expression.
func ParseCron(expr string) (CronSchedule, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return CronSchedule{}, fmt.Errorf("scheduler: cron expression must have 5 fields, got %d", len(fields))
	}
	minute, err := parseCronField(fields[0], 0, 59)
	if err != nil {
		return CronSchedule{}, err
	}
	hour, err := parseCronField(fields[1], 0, 23)
	if err != nil {
		return CronSchedule{}, err
	}
	dom, err := parseCronField(fields[2], 1, 31)
	if err != nil {
		return CronSchedule{}, err
	}
	month, err := parseCronField(fields[3], 1, 12)
	if err != nil {
		return CronSchedule{}, err
	}
	dow, err := parseCronField(fields[4], 0, 6)
	if err != nil {
		return CronSchedule{}, err
	}
	return CronSchedule{minute: minute, hour: hour, dom: dom, month: month, dow: dow, raw: expr}, nil
}

// maxCronLookahead bounds how far into the future Next searches before
// concluding no occurrence exists (guards against an unsatisfiable
// expression, e.g. day-of-month 31 combined with a month field that
// excludes every 31-day month, spinning forever).
const maxCronLookaheadMinutes = 366 * 24 * 60

// Next returns the first occurrence strictly after `after`, evaluated in
// loc (the task's own IANA timezone) so a schedule like "0 9 * * *" always
// means 9am local time through a daylight-saving transition, not a fixed
// UTC offset that silently drifts by an hour twice a year.
//
// Deterministic DST handling: candidate is stepped forward one real minute
// of absolute time at a time (never reconstructed from wall-clock fields
// mid-walk), so a spring-forward gap is skipped automatically — no absolute
// instant's local reading ever falls inside a gap that does not exist. A
// fall-back overlap (a local wall-clock reading that two different absolute
// instants both produce) is resolved to its first occurrence only: for each
// candidate we ask time.Date to reconstruct the instant from that same
// wall-clock reading, which Go always resolves to the pre-transition
// offset; if that reconstruction does not equal the candidate itself, the
// candidate is the repeated (second-pass) instant and is skipped rather
// than treated as a second valid occurrence of a nominally-once-daily
// schedule.
func (s CronSchedule) Next(after time.Time, loc *time.Location) (time.Time, error) {
	candidate := after.In(loc).Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < maxCronLookaheadMinutes; i++ {
		local := candidate.In(loc)
		canonical := time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), local.Minute(), 0, 0, loc)
		if canonical.Equal(candidate) &&
			s.minute.matches(local.Minute()) && s.hour.matches(local.Hour()) &&
			s.dom.matches(local.Day()) && s.month.matches(int(local.Month())) &&
			s.dow.matches(int(local.Weekday())) {
			return canonical.UTC(), nil
		}
		candidate = candidate.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("scheduler: no occurrence of %q found within lookahead window", s.raw)
}
