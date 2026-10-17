package orders

// Corporate recurring feeding stays in the Orders bounded context. CRM owns
// the corporate relationship identity; this package stores only an opaque
// relationship reference and immutable commercial occurrence evidence.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
	"cosmicforge/logistics/services/vendor-service/internal/features/production"
	"github.com/jackc/pgx/v5"
)

type RecurringScheduleInput struct {
	BusinessID            string
	RequesterID           string
	CorporateRef          string
	BranchID              string
	Weekdays              []int
	MealTime              string
	Timezone              string
	Headcount             int64
	AllowanceMinor        int64
	DestinationSnapshot   map[string]any
	MenuRotation          map[string]any
	ProductionRequirement CapacityRequirement
	TentativeVehicleClass string
	AdvanceHorizonDays    int
	StartsOn              time.Time
	EndsOn                *time.Time
	IdempotencyKey        string
}

type RecurringSchedule struct {
	ID, BusinessID, RequesterID, CorporateRef, BranchID      string
	Weekdays                                                 []int
	MealTime, Timezone, Status, TentativeVehicleClass        string
	Headcount, AllowanceMinor, AdvanceHorizonDays            int64
	DestinationSnapshot, MenuRotation, ProductionRequirement map[string]any
	StartsOn                                                 time.Time
	EndsOn                                                   *time.Time
	Version                                                  int64
	CreatedAt, UpdatedAt                                     time.Time
}

type RecurringOccurrence struct {
	ID, ScheduleID, BusinessID, BranchID, OccurrenceKey, Timezone        string
	OccurrenceAt, LocalDate                                              time.Time
	Headcount, AllowanceMinor                                            int64
	DestinationSnapshot, MenuSnapshot, ProductionRequirement             map[string]any
	CapacityReservationID, CapacityStatus, TentativeVehicleClass, Status string
	ProposalID                                                           string
	Version                                                              int64
	CreatedAt, UpdatedAt                                                 time.Time
}

var ErrSection21Schedule = errors.New("section21: invalid recurring schedule")

func validateRecurringInput(in RecurringScheduleInput, now time.Time) (*time.Location, string, error) {
	if strings.TrimSpace(in.BusinessID) == "" || strings.TrimSpace(in.CorporateRef) == "" || strings.TrimSpace(in.BranchID) == "" || strings.TrimSpace(in.IdempotencyKey) == "" || in.Headcount <= 0 || in.Headcount > 1000000 || in.AllowanceMinor < 0 || len(in.Weekdays) == 0 || in.StartsOn.IsZero() {
		return nil, "", ErrSection21Schedule
	}
	loc, err := time.LoadLocation(strings.TrimSpace(in.Timezone))
	if err != nil {
		return nil, "", ErrSection21Schedule
	}
	seen := map[int]bool{}
	for _, day := range in.Weekdays {
		if day < 1 || day > 7 || seen[day] {
			return nil, "", ErrSection21Schedule
		}
		seen[day] = true
	}
	meal, err := time.Parse("15:04", strings.TrimSpace(in.MealTime))
	if err != nil {
		// Reads return meal_time::text ("12:30:00"); an edit echoes it back.
		if meal, err = time.Parse("15:04:05", strings.TrimSpace(in.MealTime)); err != nil {
			return nil, "", ErrSection21Schedule
		}
	}
	if in.AdvanceHorizonDays == 0 {
		in.AdvanceHorizonDays = 14
	}
	if in.AdvanceHorizonDays < 1 || in.AdvanceHorizonDays > 90 || (in.EndsOn != nil && in.EndsOn.Before(in.StartsOn)) {
		return nil, "", ErrSection21Schedule
	}
	if in.TentativeVehicleClass != "" {
		v := strings.ToUpper(strings.TrimSpace(in.TentativeVehicleClass))
		if v != string(fulfilment.VehicleMotorcycle) && v != string(fulfilment.VehicleVan) && v != string(fulfilment.VehicleTruck) {
			return nil, "", ErrSection21Schedule
		}
	}
	startDate := time.Date(in.StartsOn.Year(), in.StartsOn.Month(), in.StartsOn.Day(), 0, 0, 0, 0, loc)
	todayDate := time.Date(now.In(loc).Year(), now.In(loc).Month(), now.In(loc).Day(), 0, 0, 0, 0, loc)
	if startDate.Before(todayDate) {
		return nil, "", ErrSection21Schedule
	}
	return loc, meal.Format("15:04"), nil
}

func (s *Section21Service) CreateRecurringSchedule(ctx context.Context, in RecurringScheduleInput) (RecurringSchedule, bool, error) {
	if err := s.available(); err != nil {
		return RecurringSchedule{}, false, err
	}
	now := s.now().UTC()
	loc, meal, err := validateRecurringInput(in, now)
	if err != nil {
		return RecurringSchedule{}, false, err
	}
	destination, _ := json.Marshal(in.DestinationSnapshot)
	rotation, _ := json.Marshal(in.MenuRotation)
	productionRaw, _ := json.Marshal(in.ProductionRequirement)
	vehicle := strings.ToUpper(strings.TrimSpace(in.TentativeVehicleClass))
	weekdaysParam := make([]int16, len(in.Weekdays))
	for i, day := range in.Weekdays {
		weekdaysParam[i] = int16(day)
	}
	var out RecurringSchedule
	var weekdaysDB []int16
	var destinationRaw, rotationRaw, productionRawDB []byte
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return RecurringSchedule{}, false, err
	}
	defer tx.Rollback(ctx)
	var existingID string
	existed := tx.QueryRow(ctx, `SELECT id FROM commerce_recurring_schedules WHERE business_id=$1 AND idempotency_key=$2 FOR UPDATE`, in.BusinessID, in.IdempotencyKey).Scan(&existingID) == nil
	err = tx.QueryRow(ctx, `INSERT INTO commerce_recurring_schedules(id,business_id,requester_id,corporate_ref,branch_id,weekdays,meal_time,timezone,headcount,allowance_minor,destination_snapshot,menu_rotation,production_requirement,tentative_vehicle_class,advance_horizon_days,starts_on,ends_on,idempotency_key) VALUES($1,$2,$3,$4,$5,$6::smallint[],$7,$8,$9,$10,$11,$12,$13,NULLIF($14,''),$15,$16,$17,$18) ON CONFLICT (business_id,idempotency_key) DO UPDATE SET id=commerce_recurring_schedules.id RETURNING id,business_id,requester_id,corporate_ref,branch_id,weekdays,meal_time::text,timezone,headcount,allowance_minor,destination_snapshot,menu_rotation,production_requirement,COALESCE(tentative_vehicle_class,''),advance_horizon_days,starts_on,ends_on,status,version,created_at,updated_at`, sid("crs_"), in.BusinessID, in.RequesterID, in.CorporateRef, in.BranchID, weekdaysParam, meal, in.Timezone, in.Headcount, in.AllowanceMinor, destination, rotation, productionRaw, vehicle, in.AdvanceHorizonDays, in.StartsOn.Format("2006-01-02"), nullableDate(in.EndsOn, loc), in.IdempotencyKey).Scan(&out.ID, &out.BusinessID, &out.RequesterID, &out.CorporateRef, &out.BranchID, &weekdaysDB, &meal, &out.Timezone, &out.Headcount, &out.AllowanceMinor, &destinationRaw, &rotationRaw, &productionRawDB, &out.TentativeVehicleClass, &out.AdvanceHorizonDays, &out.StartsOn, &out.EndsOn, &out.Status, &out.Version, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		return RecurringSchedule{}, false, err
	}
	if out.BranchID != in.BranchID || out.CorporateRef != in.CorporateRef || out.RequesterID != in.RequesterID {
		return RecurringSchedule{}, false, ErrSection21Conflict
	}
	out.Weekdays = make([]int, len(weekdaysDB))
	for i, day := range weekdaysDB {
		out.Weekdays[i] = int(day)
	}
	_ = json.Unmarshal(destinationRaw, &out.DestinationSnapshot)
	_ = json.Unmarshal(rotationRaw, &out.MenuRotation)
	_ = json.Unmarshal(productionRawDB, &out.ProductionRequirement)
	if out.Status == "" {
		out.Status = "ACTIVE"
	}
	created := !existed
	if created {
		if err = s.appendEvent(ctx, tx, "commerce.recurring_schedule.active", "commerce_recurring_schedule", out.ID, in.BusinessID, out.Version, map[string]any{"corporate_ref": in.CorporateRef, "branch_id": in.BranchID}); err != nil {
			return RecurringSchedule{}, false, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return RecurringSchedule{}, false, err
	}
	return out, created, nil
}

func nullableDate(value *time.Time, loc *time.Location) any {
	if value == nil {
		return nil
	}
	return value.Format("2006-01-02")
}

func (s *Section21Service) SetRecurringScheduleStatus(ctx context.Context, businessID, scheduleID, status string, expectedVersion int64) error {
	if err := s.available(); err != nil {
		return err
	}
	status = strings.ToUpper(strings.TrimSpace(status))
	if status != "ACTIVE" && status != "PAUSED" && status != "CANCELLED" && status != "ENDED" {
		return ErrSection21Schedule
	}
	tag, err := s.db.Exec(ctx, `UPDATE commerce_recurring_schedules SET status=$3,version=version+1,updated_at=now() WHERE id=$1 AND business_id=$2 AND version=$4 AND status NOT IN ('CANCELLED','ENDED')`, scheduleID, businessID, status, expectedVersion)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSection21Conflict
	}
	return nil
}

func (s *Section21Service) SetRecurringScheduleStatusByStaff(ctx context.Context, actor Actor, scheduleID, status string, expectedVersion int64) error {
	var branchID string
	if err := s.db.QueryRow(ctx, `SELECT branch_id FROM commerce_recurring_schedules WHERE id=$1 AND business_id=$2`, scheduleID, actor.VendorID).Scan(&branchID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSection21NotFound
		}
		return err
	}
	if err := s.AuthorizeStaff(ctx, actor, branchID); err != nil {
		return err
	}
	return s.SetRecurringScheduleStatus(ctx, actor.VendorID, scheduleID, status, expectedVersion)
}

func (s *Section21Service) UpdateRecurringSchedule(ctx context.Context, actor Actor, scheduleID string, in RecurringScheduleInput, expectedVersion int64) error {
	if err := s.available(); err != nil {
		return err
	}
	in.BusinessID = actor.VendorID
	if err := s.AuthorizeStaff(ctx, actor, in.BranchID); err != nil {
		return err
	}
	var existingBranch string
	if err := s.db.QueryRow(ctx, `SELECT branch_id FROM commerce_recurring_schedules WHERE id=$1 AND business_id=$2`, scheduleID, actor.VendorID).Scan(&existingBranch); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSection21NotFound
		}
		return err
	}
	if err := s.AuthorizeStaff(ctx, actor, existingBranch); err != nil {
		return err
	}

	existing, err := scanRecurring(s.db.QueryRow(ctx, `SELECT `+recurringReadColumns+` FROM commerce_recurring_schedules WHERE business_id=$1 AND id=$2`, actor.VendorID, scheduleID))
	if err != nil {
		return err
	}
	_, oldTyped := existing.MenuRotation["schema_version"]
	_, newTyped := in.MenuRotation["schema_version"]
	if oldTyped || newTyped {
		if !oldTyped || !newTyped || in.BranchID != existing.BranchID || in.RequesterID != existing.RequesterID || in.CorporateRef != existing.CorporateRef || !equalRecurringSnapshot(in.MenuRotation, existing.MenuRotation) || !equalRecurringSnapshot(in.DestinationSnapshot, existing.DestinationSnapshot) {
			return ErrSection21Schedule
		}
	}
	// A running schedule keeps its original start date; only a moved start
	// date must lie in the future, otherwise an edit of any started
	// schedule would be refused.
	validationNow := s.now().UTC()
	if sameCalendarDay(in.StartsOn, existing.StartsOn) && validationNow.After(existing.StartsOn) {
		validationNow = existing.StartsOn
	}
	loc, meal, err := validateRecurringInput(in, validationNow)
	if err != nil {
		return err
	}
	weekdays := make([]int16, len(in.Weekdays))
	for i, day := range in.Weekdays {
		weekdays[i] = int16(day)
	}
	destination, _ := json.Marshal(in.DestinationSnapshot)
	rotation, _ := json.Marshal(in.MenuRotation)
	productionRaw, _ := json.Marshal(in.ProductionRequirement)
	vehicle := strings.ToUpper(strings.TrimSpace(in.TentativeVehicleClass))
	tag, err := s.db.Exec(ctx, `UPDATE commerce_recurring_schedules SET branch_id=$3,weekdays=$4::smallint[],meal_time=$5,timezone=$6,headcount=$7,allowance_minor=$8,destination_snapshot=$9,menu_rotation=$10,production_requirement=$11,tentative_vehicle_class=NULLIF($12,''),advance_horizon_days=$13,starts_on=$14,ends_on=$15,version=version+1,updated_at=now() WHERE id=$1 AND business_id=$2 AND version=$16 AND status NOT IN ('CANCELLED','ENDED')`, scheduleID, actor.VendorID, in.BranchID, weekdays, meal, in.Timezone, in.Headcount, in.AllowanceMinor, destination, rotation, productionRaw, vehicle, in.AdvanceHorizonDays, in.StartsOn.Format("2006-01-02"), nullableDate(in.EndsOn, loc), expectedVersion)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSection21Conflict
	}
	return nil
}

func (s *Section21Service) GenerateRecurringOccurrences(ctx context.Context, limit int) (int64, error) {
	if err := s.available(); err != nil {
		return 0, err
	}
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	rows, err := s.db.Query(ctx, `SELECT id,business_id,requester_id,corporate_ref,branch_id,weekdays,meal_time,timezone,headcount,allowance_minor,destination_snapshot,menu_rotation,production_requirement,COALESCE(tentative_vehicle_class,''),advance_horizon_days,starts_on,ends_on,status,version FROM commerce_recurring_schedules WHERE status='ACTIVE' ORDER BY id LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var generated int64
	for rows.Next() {
		var schedule RecurringSchedule
		var weekdaysDB []int16
		var destinationRaw, rotationRaw, productionRaw []byte
		var meal string
		if err = rows.Scan(&schedule.ID, &schedule.BusinessID, &schedule.RequesterID, &schedule.CorporateRef, &schedule.BranchID, &weekdaysDB, &meal, &schedule.Timezone, &schedule.Headcount, &schedule.AllowanceMinor, &destinationRaw, &rotationRaw, &productionRaw, &schedule.TentativeVehicleClass, &schedule.AdvanceHorizonDays, &schedule.StartsOn, &schedule.EndsOn, &schedule.Status, &schedule.Version); err != nil {
			return generated, err
		}
		schedule.Weekdays = make([]int, len(weekdaysDB))
		for i, day := range weekdaysDB {
			schedule.Weekdays[i] = int(day)
		}
		_ = json.Unmarshal(destinationRaw, &schedule.DestinationSnapshot)
		_ = json.Unmarshal(rotationRaw, &schedule.MenuRotation)
		_ = json.Unmarshal(productionRaw, &schedule.ProductionRequirement)
		loc, loadErr := time.LoadLocation(schedule.Timezone)
		if loadErr != nil {
			return generated, ErrSection21Schedule
		}
		parsedMeal, parseErr := time.Parse("15:04", meal)
		if parseErr != nil {
			return generated, ErrSection21Schedule
		}
		start, end := recurringGenerationDates(schedule, s.now(), loc)
		for day := start; !day.After(end) && generated < int64(limit); day = day.AddDate(0, 0, 1) {
			localDay := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
			weekday := int(localDay.Weekday())
			if weekday == 0 {
				weekday = 7
			}
			if !containsInt(schedule.Weekdays, weekday) {
				continue
			}
			occurrenceAt := time.Date(localDay.Year(), localDay.Month(), localDay.Day(), parsedMeal.Hour(), parsedMeal.Minute(), 0, 0, loc)
			if occurrenceAt.Before(s.now()) {
				continue
			}
			key := occurrenceAt.Format("2006-01-02T15:04") + "@" + schedule.Timezone
			created, createErr := s.insertRecurringOccurrence(ctx, schedule, key, occurrenceAt, localDay)
			if createErr != nil {
				return generated, createErr
			}
			if created {
				generated++
			}
		}
	}
	return generated, rows.Err()
}

func containsInt(values []int, needle int) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func (s *Section21Service) insertRecurringOccurrence(ctx context.Context, schedule RecurringSchedule, key string, at, localDate time.Time) (bool, error) {
	capacitySnapshot, cap, err := recurringOccurrenceCapacity(schedule, localDate)
	if err != nil {
		return false, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	destination, _ := json.Marshal(schedule.DestinationSnapshot)
	menu, _ := json.Marshal(schedule.MenuRotation)
	productionRaw, _ := json.Marshal(capacitySnapshot)
	var occurrenceID string
	err = tx.QueryRow(ctx, `INSERT INTO commerce_recurring_occurrences(id,schedule_id,business_id,branch_id,occurrence_key,occurrence_at,local_date,timezone,headcount,allowance_minor,destination_snapshot,menu_snapshot,production_requirement,tentative_vehicle_class) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,NULLIF($14,'')) ON CONFLICT (schedule_id,occurrence_key) DO NOTHING RETURNING id`, sid("cro_"), schedule.ID, schedule.BusinessID, schedule.BranchID, key, at, localDate.Format("2006-01-02"), schedule.Timezone, schedule.Headcount, schedule.AllowanceMinor, destination, menu, productionRaw, schedule.TentativeVehicleClass).Scan(&occurrenceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, tx.Commit(ctx)
	}
	if err != nil {
		return false, err
	}
	if cap.Required {
		if s.production == nil || len(cap.Allocations) == 0 || cap.WindowStart.IsZero() || !cap.WindowEnd.After(cap.WindowStart) {
			return false, ErrSection21Capacity
		}
		reservation, _, reserveErr := s.production.ReserveInTx(ctx, tx, production.Actor{VendorID: schedule.BusinessID, CommandRef: occurrenceID}, production.Scope{Global: true}, production.ReserveInput{BranchID: schedule.BranchID, Capability: cap.Capability, WindowStart: cap.WindowStart, WindowEnd: cap.WindowEnd, Allocations: cap.Allocations, SourceType: "RECURRING_OCCURRENCE", SourceRef: occurrenceID, Note: "corporate recurring feeding"})
		if reserveErr != nil {
			return false, fmt.Errorf("%w: %v", ErrSection21Capacity, reserveErr)
		}
		if _, err = tx.Exec(ctx, `UPDATE commerce_recurring_occurrences SET capacity_reservation_id=$2,capacity_status='RESERVED',status='RESERVED',version=version+1,updated_at=now() WHERE id=$1`, occurrenceID, reservation.ID); err != nil {
			return false, err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO commerce_recurring_order_proposals(id,occurrence_id,business_id,proposal_key) VALUES($1,$2,$3,$4)`, sid("crp_"), occurrenceID, schedule.BusinessID, schedule.ID+":"+key); err != nil {
		return false, err
	}
	if err = s.appendEvent(ctx, tx, "commerce.recurring_occurrence.generated", "commerce_recurring_occurrence", occurrenceID, schedule.BusinessID, 1, map[string]any{"schedule_id": schedule.ID, "corporate_ref": schedule.CorporateRef, "occurrence_key": key}); err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Section21Service) ListRecurringOccurrences(ctx context.Context, businessID, scheduleID string, limit int) ([]RecurringOccurrence, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT o.id,o.schedule_id,o.business_id,o.branch_id,o.occurrence_key,o.occurrence_at,o.local_date,o.timezone,o.headcount,o.allowance_minor,o.destination_snapshot,o.menu_snapshot,o.production_requirement,COALESCE(o.capacity_reservation_id,''),o.capacity_status,COALESCE(o.tentative_vehicle_class,''),o.status,o.version,o.created_at,o.updated_at,p.id FROM commerce_recurring_occurrences o LEFT JOIN commerce_recurring_order_proposals p ON p.occurrence_id=o.id WHERE o.business_id=$1 AND o.schedule_id=$2 ORDER BY o.occurrence_at,o.id LIMIT $3`, businessID, scheduleID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]RecurringOccurrence, 0)
	for rows.Next() {
		var occurrence RecurringOccurrence
		var destination, menu, productionRaw []byte
		if err = rows.Scan(&occurrence.ID, &occurrence.ScheduleID, &occurrence.BusinessID, &occurrence.BranchID, &occurrence.OccurrenceKey, &occurrence.OccurrenceAt, &occurrence.LocalDate, &occurrence.Timezone, &occurrence.Headcount, &occurrence.AllowanceMinor, &destination, &menu, &productionRaw, &occurrence.CapacityReservationID, &occurrence.CapacityStatus, &occurrence.TentativeVehicleClass, &occurrence.Status, &occurrence.Version, &occurrence.CreatedAt, &occurrence.UpdatedAt, &occurrence.ProposalID); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(destination, &occurrence.DestinationSnapshot)
		_ = json.Unmarshal(menu, &occurrence.MenuSnapshot)
		_ = json.Unmarshal(productionRaw, &occurrence.ProductionRequirement)
		out = append(out, occurrence)
	}
	return out, rows.Err()
}

func (s *Section21Service) ListRecurringOccurrencesByStaff(ctx context.Context, actor Actor, scheduleID string, limit int) ([]RecurringOccurrence, error) {
	if _, err := s.GetRecurringByStaff(ctx, actor, scheduleID); err != nil {
		return nil, err
	}
	return s.ListRecurringOccurrences(ctx, actor.VendorID, scheduleID, limit)
}

func sameCalendarDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
