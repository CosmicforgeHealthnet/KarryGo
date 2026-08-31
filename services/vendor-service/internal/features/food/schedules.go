package food

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// ---------------------------------------------------------------------------
// Menu schedules
// ---------------------------------------------------------------------------
//
// A schedule answers "when is this menu operationally available". It never
// answers "is this menu published" — that is the Section 9A administrative
// status, decided a layer above, and no schedule is allowed to argue with it.

// Schedule is one availability window on a menu.
type Schedule struct {
	ID                 string
	BusinessID         string
	MenuID             string
	Label              string
	DaysOfWeek         []int
	StartLocal         string // HH:MM in the branch's local time
	EndLocal           string
	EffectiveStartDate *string // YYYY-MM-DD, nil = unbounded
	EffectiveEndDate   *string
	Status             string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	ArchivedAt         *time.Time
	Version            int64
}

// Overnight reports a window that crosses midnight, such as 22:00-02:00.
//
// The window belongs to the day it STARTS on: a Friday 22:00-02:00 schedule is
// Friday's, and an instant at 01:00 on Saturday matches it. Any other rule
// would make "open Friday night" unrepresentable.
func (s Schedule) Overnight() bool { return s.StartLocal > s.EndLocal }

type ScheduleView struct {
	ID                 string     `json:"id"`
	MenuID             string     `json:"menu_id"`
	Label              string     `json:"label"`
	DaysOfWeek         []int      `json:"days_of_week"`
	StartLocalTime     string     `json:"start_local_time"`
	EndLocalTime       string     `json:"end_local_time"`
	Overnight          bool       `json:"overnight"`
	EffectiveStartDate *string    `json:"effective_start_date"`
	EffectiveEndDate   *string    `json:"effective_end_date"`
	Status             string     `json:"status"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	ArchivedAt         *time.Time `json:"archived_at,omitempty"`
	Version            int64      `json:"version"`
}

func ScheduleViewOf(s Schedule) ScheduleView {
	days := s.DaysOfWeek
	if days == nil {
		days = []int{}
	}
	return ScheduleView{ID: s.ID, MenuID: s.MenuID, Label: s.Label, DaysOfWeek: days,
		StartLocalTime: s.StartLocal, EndLocalTime: s.EndLocal, Overnight: s.Overnight(),
		EffectiveStartDate: s.EffectiveStartDate, EffectiveEndDate: s.EffectiveEndDate,
		Status: s.Status, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
		ArchivedAt: s.ArchivedAt, Version: s.Version}
}

const scheduleCols = `id,business_id,menu_id,label,days_of_week,
	to_char(start_local_time,'HH24:MI'),to_char(end_local_time,'HH24:MI'),
	to_char(effective_start_date,'YYYY-MM-DD'),to_char(effective_end_date,'YYYY-MM-DD'),
	status,created_at,updated_at,archived_at,version`

func scanSchedule(row rowScanner) (Schedule, error) {
	var s Schedule
	var days []int16
	err := row.Scan(&s.ID, &s.BusinessID, &s.MenuID, &s.Label, &days, &s.StartLocal, &s.EndLocal,
		&s.EffectiveStartDate, &s.EffectiveEndDate, &s.Status, &s.CreatedAt, &s.UpdatedAt,
		&s.ArchivedAt, &s.Version)
	if err != nil {
		return Schedule{}, err
	}
	s.DaysOfWeek = make([]int, 0, len(days))
	for _, day := range days {
		s.DaysOfWeek = append(s.DaysOfWeek, int(day))
	}
	return s, nil
}

func loadSchedule(ctx context.Context, q querier, businessID, menuID, scheduleID string, lock bool) (Schedule, error) {
	sql := `SELECT ` + scheduleCols + ` FROM food_menu_schedules
		WHERE business_id=$1 AND menu_id=$2 AND id=$3`
	if lock {
		sql += ` FOR UPDATE`
	}
	schedule, err := scanSchedule(q.QueryRow(ctx, sql, businessID, strings.TrimSpace(menuID), strings.TrimSpace(scheduleID)))
	if err != nil {
		return Schedule{}, mapDB(err, "Menu schedule", CodeScheduleNotFound)
	}
	return schedule, nil
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

var scheduleLabels = map[string]bool{
	ScheduleBreakfast: true, ScheduleLunch: true, ScheduleDinner: true,
	ScheduleSeasonal: true, ScheduleCustom: true,
}

// parseLocalClock accepts HH:MM (24-hour). Seconds are deliberately not
// accepted: a menu that opens at 09:00:30 is a configuration accident.
func parseLocalClock(field, value string) (string, *apperrors.FieldViolation) {
	trimmed := strings.TrimSpace(value)
	parsed, err := time.Parse("15:04", trimmed)
	if err != nil {
		return "", &apperrors.FieldViolation{Field: field, Message: "Time must be HH:MM in 24-hour form."}
	}
	return parsed.Format("15:04"), nil
}

func parseLocalDate(field, value string) (string, *apperrors.FieldViolation) {
	trimmed := strings.TrimSpace(value)
	parsed, err := time.Parse("2006-01-02", trimmed)
	if err != nil {
		return "", &apperrors.FieldViolation{Field: field, Message: "Date must be YYYY-MM-DD."}
	}
	return parsed.Format("2006-01-02"), nil
}

// normalizeDays validates ISO weekdays (1 = Monday .. 7 = Sunday), rejecting
// duplicates and returning them sorted so two equivalent schedules store
// identically.
func normalizeDays(values []int) ([]int, *apperrors.FieldViolation) {
	if len(values) == 0 || len(values) > 7 {
		return nil, &apperrors.FieldViolation{Field: "days_of_week",
			Message: "Provide between one and seven weekdays."}
	}
	seen := map[int]bool{}
	out := make([]int, 0, len(values))
	for _, day := range values {
		if day < 1 || day > 7 {
			return nil, &apperrors.FieldViolation{Field: "days_of_week",
				Message: "Weekdays are 1 (Monday) to 7 (Sunday)."}
		}
		if seen[day] {
			return nil, &apperrors.FieldViolation{Field: "days_of_week",
				Message: "Duplicate weekdays are not allowed."}
		}
		seen[day] = true
		out = append(out, day)
	}
	sort.Ints(out)
	return out, nil
}

type scheduleFields struct {
	label      string
	days       []int
	start, end string
	from, to   *string
}

func validateScheduleFields(label string, days []int, start, end string, from, to *string) (scheduleFields, error) {
	var violations []apperrors.FieldViolation
	fields := scheduleFields{}

	fields.label = strings.ToUpper(strings.TrimSpace(label))
	if !scheduleLabels[fields.label] {
		violations = append(violations, apperrors.FieldViolation{Field: "label",
			Message: "Label must be BREAKFAST, LUNCH, DINNER, SEASONAL or CUSTOM."})
	}
	normalized, violation := normalizeDays(days)
	if violation != nil {
		violations = append(violations, *violation)
	}
	fields.days = normalized

	startClock, violation := parseLocalClock("start_local_time", start)
	if violation != nil {
		violations = append(violations, *violation)
	}
	fields.start = startClock
	endClock, violation := parseLocalClock("end_local_time", end)
	if violation != nil {
		violations = append(violations, *violation)
	}
	fields.end = endClock
	if startClock != "" && startClock == endClock {
		violations = append(violations, apperrors.FieldViolation{Field: "end_local_time",
			Message: "Start and end must differ. Use a full-day window rather than a zero-length one."})
	}

	if from != nil {
		value, dateViolation := parseLocalDate("effective_start_date", *from)
		if dateViolation != nil {
			violations = append(violations, *dateViolation)
		} else {
			fields.from = &value
		}
	}
	if to != nil {
		value, dateViolation := parseLocalDate("effective_end_date", *to)
		if dateViolation != nil {
			violations = append(violations, *dateViolation)
		} else {
			fields.to = &value
		}
	}
	if fields.from != nil && fields.to != nil && *fields.from > *fields.to {
		violations = append(violations, apperrors.FieldViolation{Field: "effective_end_date",
			Message: "The end date must not be before the start date."})
	}

	if len(violations) > 0 {
		return scheduleFields{}, coded(apperrors.Validation("Check your details.", violations), CodeInvalidSchedule)
	}
	return fields, nil
}

// ---------------------------------------------------------------------------
// Evaluation
// ---------------------------------------------------------------------------

// branchLocation loads a branch's IANA timezone and turns it into a Go
// location.
//
// The timezone is Section 6's, read here and never copied onto a schedule row.
// Go's zoneinfo database does the daylight-saving arithmetic; this package does
// not attempt any of its own, because hand-rolled DST maths is how a menu ends
// up an hour wrong twice a year.
func branchLocation(ctx context.Context, q querier, businessID, branchID string) (*time.Location, string, error) {
	var name string
	err := q.QueryRow(ctx, `SELECT timezone FROM vendor_branches WHERE business_id=$1 AND id=$2`,
		businessID, branchID).Scan(&name)
	if err != nil {
		return nil, "", mapDB(err, "Branch", CodeMenuNotFound)
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		// A branch carrying an unloadable zone is a data fault, not a client
		// error, and it must not silently degrade to UTC: evaluating a Lagos
		// menu in UTC would be wrong by an hour and look right.
		return nil, name, apperrors.Internal("The branch timezone could not be resolved.", err)
	}
	return location, name, nil
}

// scheduleMatches reports whether an instant falls inside one window, in the
// branch's local calendar.
//
// Everything is decided after converting the instant into the branch zone, so a
// DST transition is handled by the zone database rather than by arithmetic
// here. An overnight window is tested against the day it started on, which is
// why the second branch below looks one day back.
func scheduleMatches(schedule Schedule, at time.Time, location *time.Location) bool {
	local := at.In(location)
	clock := local.Format("15:04")
	date := local.Format("2006-01-02")

	dayMatches := func(day time.Weekday) bool {
		iso := int(day)
		if iso == 0 {
			iso = 7 // Go counts Sunday as 0; ISO counts it as 7.
		}
		for _, allowed := range schedule.DaysOfWeek {
			if allowed == iso {
				return true
			}
		}
		return false
	}
	withinDates := func(day string) bool {
		if schedule.EffectiveStartDate != nil && day < *schedule.EffectiveStartDate {
			return false
		}
		if schedule.EffectiveEndDate != nil && day > *schedule.EffectiveEndDate {
			return false
		}
		return true
	}

	if !schedule.Overnight() {
		return dayMatches(local.Weekday()) && withinDates(date) &&
			clock >= schedule.StartLocal && clock < schedule.EndLocal
	}
	// Overnight: either the evening half of today's window, or the small-hours
	// half of the window that opened yesterday.
	if clock >= schedule.StartLocal && dayMatches(local.Weekday()) && withinDates(date) {
		return true
	}
	if clock < schedule.EndLocal {
		yesterday := local.AddDate(0, 0, -1)
		return dayMatches(yesterday.Weekday()) && withinDates(yesterday.Format("2006-01-02"))
	}
	return false
}

// scheduleDecision is the outcome of evaluating a menu's whole schedule set.
type scheduleDecision struct {
	Allowed  bool
	MatchRef string
	Timezone string
}

// evaluateSchedules decides whether a menu is inside its configured windows.
//
// A menu with no active schedule is unrestricted. That is the deliberate
// no-schedule semantic: a vendor who has configured nothing has said nothing
// about time, and inventing a default window would close menus nobody asked to
// close. Once one active schedule exists, the instant must match at least one
// of them — overlapping windows are a union, not a conflict.
func (s *Service) evaluateSchedules(ctx context.Context, q querier, menu Menu, at time.Time) (scheduleDecision, error) {
	location, zoneName, err := branchLocation(ctx, q, menu.BusinessID, menu.BranchID)
	if err != nil {
		return scheduleDecision{}, err
	}
	rows, err := q.Query(ctx, `SELECT `+scheduleCols+` FROM food_menu_schedules
		WHERE business_id=$1 AND menu_id=$2 AND status='ACTIVE' ORDER BY id`,
		menu.BusinessID, menu.ID)
	if err != nil {
		return scheduleDecision{}, err
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		schedule, scanErr := scanSchedule(rows)
		if scanErr != nil {
			return scheduleDecision{}, scanErr
		}
		found = true
		if scheduleMatches(schedule, at, location) {
			return scheduleDecision{Allowed: true, MatchRef: schedule.ID, Timezone: zoneName}, nil
		}
	}
	if err = rows.Err(); err != nil {
		return scheduleDecision{}, err
	}
	return scheduleDecision{Allowed: !found, Timezone: zoneName}, nil
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

type CreateScheduleInput struct {
	Label              string
	DaysOfWeek         []int
	StartLocalTime     string
	EndLocalTime       string
	EffectiveStartDate *string
	EffectiveEndDate   *string
}

// CreateSchedule adds an availability window to a menu.
func (s *Service) CreateSchedule(ctx context.Context, actor Actor, scope Scope, menuID string, in CreateScheduleInput) (Schedule, error) {
	if err := s.requireWrites(); err != nil {
		return Schedule{}, err
	}
	fields, err := validateScheduleFields(in.Label, in.DaysOfWeek, in.StartLocalTime, in.EndLocalTime,
		in.EffectiveStartDate, in.EffectiveEndDate)
	if err != nil {
		return Schedule{}, err
	}

	var schedule Schedule
	err = s.inTx(ctx, func(tx pgx.Tx) error {
		menu, err := s.lockedMenu(ctx, tx, actor, scope, menuID)
		if err != nil {
			return err
		}
		if menu.Status == StatusArchived {
			return errArchived("Menu")
		}
		// The branch timezone must resolve before a schedule can exist, or the
		// window would be uninterpretable the moment anyone evaluated it.
		if _, _, err = branchLocation(ctx, tx, menu.BusinessID, menu.BranchID); err != nil {
			return err
		}
		days := make([]int16, 0, len(fields.days))
		for _, day := range fields.days {
			days = append(days, int16(day))
		}
		schedule, err = scanSchedule(tx.QueryRow(ctx, `
			INSERT INTO food_menu_schedules
				(id,business_id,menu_id,label,days_of_week,start_local_time,end_local_time,
				 effective_start_date,effective_end_date,status)
			VALUES ($1,$2,$3,$4,$5,$6::time,$7::time,$8::date,$9::date,'ACTIVE')
			RETURNING `+scheduleCols,
			newID("fsc_"), actor.VendorID, menu.ID, fields.label, days,
			fields.start, fields.end, fields.from, fields.to))
		if err != nil {
			return mapDB(err, "Menu schedule", CodeScheduleNotFound)
		}
		revision, err := bumpRevision(ctx, tx, scopeOfMenu(menu))
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.menu.schedule.created", eventType: "food.menu.schedule.changed",
			aggregateType: "food_menu_schedule", aggregateID: schedule.ID, branchID: menu.BranchID,
			version: schedule.Version, after: ScheduleViewOf(schedule),
			payload: map[string]any{"menu_id": menu.ID, "schedule": ScheduleViewOf(schedule),
				"change": "CREATED", "catalogue_revision": revision},
		})
	})
	return schedule, err
}

// ListSchedules returns a menu's windows.
func (s *Service) ListSchedules(ctx context.Context, actor Actor, scope Scope, menuID string, includeArchived bool) ([]Schedule, string, error) {
	menu, err := s.GetMenu(ctx, actor, scope, menuID)
	if err != nil {
		return nil, "", err
	}
	_, zoneName, err := branchLocation(ctx, s.db, menu.BusinessID, menu.BranchID)
	if err != nil {
		return nil, "", err
	}
	rows, err := s.db.Query(ctx, `SELECT `+scheduleCols+` FROM food_menu_schedules
		WHERE business_id=$1 AND menu_id=$2 AND ($3 OR status <> 'ARCHIVED')
		ORDER BY (status = 'ARCHIVED'), start_local_time, id`,
		actor.VendorID, menu.ID, includeArchived)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	schedules := []Schedule{}
	for rows.Next() {
		schedule, scanErr := scanSchedule(rows)
		if scanErr != nil {
			return nil, "", scanErr
		}
		schedules = append(schedules, schedule)
	}
	return schedules, zoneName, rows.Err()
}

// GetSchedule reads one window addressed through its own menu.
func (s *Service) GetSchedule(ctx context.Context, actor Actor, scope Scope, menuID, scheduleID string) (Schedule, error) {
	menu, err := s.GetMenu(ctx, actor, scope, menuID)
	if err != nil {
		return Schedule{}, err
	}
	return loadSchedule(ctx, s.db, actor.VendorID, menu.ID, scheduleID, false)
}

type UpdateScheduleInput struct {
	Label              *string
	DaysOfWeek         *[]int
	StartLocalTime     *string
	EndLocalTime       *string
	EffectiveStartDate *string
	EffectiveEndDate   *string
	ClearStartDate     bool
	ClearEndDate       bool
	ExpectedVersion    int64
}

// UpdateSchedule edits a window under optimistic concurrency.
func (s *Service) UpdateSchedule(ctx context.Context, actor Actor, scope Scope, menuID, scheduleID string, in UpdateScheduleInput) (Schedule, error) {
	if err := s.requireWrites(); err != nil {
		return Schedule{}, err
	}
	var updated Schedule
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		menu, err := s.lockedMenu(ctx, tx, actor, scope, menuID)
		if err != nil {
			return err
		}
		current, err := loadSchedule(ctx, tx, actor.VendorID, menu.ID, scheduleID, true)
		if err != nil {
			return err
		}
		if current.Status == ChildArchived {
			return errArchived("Menu schedule")
		}
		if current.Version != in.ExpectedVersion {
			return errStaleVersion()
		}

		label, start, end := current.Label, current.StartLocal, current.EndLocal
		days := current.DaysOfWeek
		from, to := current.EffectiveStartDate, current.EffectiveEndDate
		if in.Label != nil {
			label = *in.Label
		}
		if in.DaysOfWeek != nil {
			days = *in.DaysOfWeek
		}
		if in.StartLocalTime != nil {
			start = *in.StartLocalTime
		}
		if in.EndLocalTime != nil {
			end = *in.EndLocalTime
		}
		switch {
		case in.ClearStartDate:
			from = nil
		case in.EffectiveStartDate != nil:
			from = in.EffectiveStartDate
		}
		switch {
		case in.ClearEndDate:
			to = nil
		case in.EffectiveEndDate != nil:
			to = in.EffectiveEndDate
		}

		fields, err := validateScheduleFields(label, days, start, end, from, to)
		if err != nil {
			return err
		}
		encoded := make([]int16, 0, len(fields.days))
		for _, day := range fields.days {
			encoded = append(encoded, int16(day))
		}
		updated, err = scanSchedule(tx.QueryRow(ctx, `
			UPDATE food_menu_schedules SET label=$3, days_of_week=$4,
				start_local_time=$5::time, end_local_time=$6::time,
				effective_start_date=$7::date, effective_end_date=$8::date,
				updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+scheduleCols,
			actor.VendorID, current.ID, fields.label, encoded, fields.start, fields.end,
			fields.from, fields.to))
		if err != nil {
			return mapDB(err, "Menu schedule", CodeScheduleNotFound)
		}
		revision, err := bumpRevision(ctx, tx, scopeOfMenu(menu))
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.menu.schedule.updated", eventType: "food.menu.schedule.changed",
			aggregateType: "food_menu_schedule", aggregateID: updated.ID, branchID: menu.BranchID,
			version: updated.Version, before: ScheduleViewOf(current), after: ScheduleViewOf(updated),
			payload: map[string]any{"menu_id": menu.ID, "schedule": ScheduleViewOf(updated),
				"change": "UPDATED", "catalogue_revision": revision},
		})
	})
	return updated, err
}

// ArchiveSchedule retires a window. An archived schedule is ignored by the
// resolver but retained, because "this menu used to open at six" is evidence a
// later dispute may need.
func (s *Service) ArchiveSchedule(ctx context.Context, actor Actor, scope Scope, menuID, scheduleID string, expectedVersion int64) (Schedule, error) {
	if err := s.requireWrites(); err != nil {
		return Schedule{}, err
	}
	var archived Schedule
	err := s.inTx(ctx, func(tx pgx.Tx) error {
		menu, err := s.lockedMenu(ctx, tx, actor, scope, menuID)
		if err != nil {
			return err
		}
		current, err := loadSchedule(ctx, tx, actor.VendorID, menu.ID, scheduleID, true)
		if err != nil {
			return err
		}
		if current.Status == ChildArchived {
			return errArchived("Menu schedule")
		}
		if current.Version != expectedVersion {
			return errStaleVersion()
		}
		archived, err = scanSchedule(tx.QueryRow(ctx, `
			UPDATE food_menu_schedules SET status='ARCHIVED', archived_at=now(),
				updated_at=now(), version=version+1
			WHERE business_id=$1 AND id=$2 RETURNING `+scheduleCols,
			actor.VendorID, current.ID))
		if err != nil {
			return mapDB(err, "Menu schedule", CodeScheduleNotFound)
		}
		revision, err := bumpRevision(ctx, tx, scopeOfMenu(menu))
		if err != nil {
			return err
		}
		return s.record(ctx, tx, actor, evidence{
			action: "food.menu.schedule.archived", eventType: "food.menu.schedule.changed",
			aggregateType: "food_menu_schedule", aggregateID: archived.ID, branchID: menu.BranchID,
			version: archived.Version, before: ScheduleViewOf(current), after: ScheduleViewOf(archived),
			payload: map[string]any{"menu_id": menu.ID, "schedule": ScheduleViewOf(archived),
				"change": "ARCHIVED", "catalogue_revision": revision},
		})
	})
	return archived, err
}
