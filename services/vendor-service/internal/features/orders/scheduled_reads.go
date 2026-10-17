package orders

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

type ScheduledReadFilter struct {
	BranchID      string
	View          string
	Limit, Offset int
}

var errScheduleView = errors.New("invalid schedule view")

func scheduleViewPredicate(view string, recurring bool) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(view)) {
	case "", "ALL":
		return "", nil
	case "ACTIVE", "PAUSED":
		if recurring {
			if strings.EqualFold(strings.TrimSpace(view), "ACTIVE") {
				return " AND status='ACTIVE'", nil
			}
			return " AND status='PAUSED'", nil
		}
	case "UPCOMING":
		if !recurring {
			return " AND scheduling_status<>'CANCELLED' AND desired_delivery_at>=now()", nil
		}
	case "PAST":
		if !recurring {
			return " AND scheduling_status<>'CANCELLED' AND desired_delivery_at<now()", nil
		}
	case "CANCELLED":
		if !recurring {
			return " AND scheduling_status='CANCELLED'", nil
		}
	}
	return "", errScheduleView
}

func (s *Section21Service) scheduledReadScope(ctx context.Context, actor Actor, branch string) (Scope, error) {
	if err := s.available(); err != nil {
		return Scope{}, err
	}
	if s.scopes == nil || strings.TrimSpace(actor.VendorID) == "" || strings.TrimSpace(actor.StaffID) == "" {
		return Scope{}, ErrSection21Forbidden
	}
	scope, err := s.scopes.ResolveScope(ctx, actor.StaffID, PermOrdersRead)
	if err != nil {
		return Scope{}, err
	}
	if scope.Empty() || branch != "" && !scope.Covers(branch) {
		return Scope{}, ErrSection21Forbidden
	}
	return scope, nil
}

func normalizeScheduledFilter(f ScheduledReadFilter) ScheduledReadFilter {
	f.BranchID = strings.TrimSpace(f.BranchID)
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	return f
}

const scheduledReadColumns = `id,business_id,customer_id,COALESCE(quote_id,''),COALESCE(vendor_order_id,''),branch_id,desired_delivery_at,delivery_window_start,delivery_window_end,production_ready_by,expected_ready_at,capacity_reservation_id,capacity_status,COALESCE(fulfilment_plan_id,''),COALESCE(tentative_vehicle_class,''),scheduling_status,bulk_requirement,source_ref,source_version,idempotency_key,created_at,accepted_at,updated_at`
const recurringReadColumns = `id,business_id,requester_id,corporate_ref,branch_id,weekdays,meal_time::text,timezone,headcount,allowance_minor,destination_snapshot,menu_rotation,production_requirement,COALESCE(tentative_vehicle_class,''),advance_horizon_days,starts_on,ends_on,status,version,created_at,updated_at`
const scheduledReadPredicate = ` WHERE business_id=$1 AND ($2::boolean OR branch_id=ANY($3::text[])) AND ($4::text='' OR branch_id=$4) AND ($5::text='' OR id=$5)`

func scanScheduled(row pgx.Row) (ScheduledOrder, error) {
	var out ScheduledOrder
	var capacity *string
	var bulk []byte
	err := row.Scan(&out.ID, &out.BusinessID, &out.CustomerID, &out.QuoteID, &out.VendorOrderID, &out.BranchID, &out.DesiredDeliveryAt, &out.DeliveryWindowStart, &out.DeliveryWindowEnd, &out.ProductionReadyBy, &out.ExpectedReadyAt, &capacity, &out.CapacityStatus, &out.FulfilmentPlanID, &out.TentativeVehicleClass, &out.SchedulingStatus, &bulk, &out.SourceRef, &out.SourceVersion, &out.IdempotencyKey, &out.CreatedAt, &out.AcceptedAt, &out.UpdatedAt)
	if err != nil {
		return out, err
	}
	if capacity != nil {
		out.CapacityReservationID = *capacity
	}
	if len(bulk) > 0 {
		err = json.Unmarshal(bulk, &out.BulkRequirement)
	}
	return out, err
}

func scanRecurring(row pgx.Row) (RecurringSchedule, error) {
	var out RecurringSchedule
	var weekdays []int16
	var destination, rotation, production []byte
	err := row.Scan(&out.ID, &out.BusinessID, &out.RequesterID, &out.CorporateRef, &out.BranchID, &weekdays, &out.MealTime, &out.Timezone, &out.Headcount, &out.AllowanceMinor, &destination, &rotation, &production, &out.TentativeVehicleClass, &out.AdvanceHorizonDays, &out.StartsOn, &out.EndsOn, &out.Status, &out.Version, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		return out, err
	}
	out.Weekdays = make([]int, len(weekdays))
	for i, d := range weekdays {
		out.Weekdays[i] = int(d)
	}
	for _, v := range []struct {
		raw    []byte
		target *map[string]any
	}{{destination, &out.DestinationSnapshot}, {rotation, &out.MenuRotation}, {production, &out.ProductionRequirement}} {
		if err = json.Unmarshal(v.raw, v.target); err != nil {
			return out, err
		}
	}
	return out, nil
}

func scheduledReadArgs(actor Actor, scope Scope, f ScheduledReadFilter, id string) []any {
	branches := scope.BranchIDs
	if branches == nil {
		branches = []string{}
	}
	return []any{actor.VendorID, scope.Global, branches, f.BranchID, id}
}

func (s *Section21Service) ListScheduledByStaff(ctx context.Context, actor Actor, f ScheduledReadFilter) ([]ScheduledOrder, error) {
	f = normalizeScheduledFilter(f)
	view, err := scheduleViewPredicate(f.View, false)
	if err != nil {
		return nil, err
	}
	scope, err := s.scheduledReadScope(ctx, actor, f.BranchID)
	if err != nil {
		return nil, err
	}
	args := append(scheduledReadArgs(actor, scope, f, ""), f.Limit+1, f.Offset)
	rows, err := s.db.Query(ctx, `SELECT `+scheduledReadColumns+` FROM commerce_scheduled_orders`+scheduledReadPredicate+view+` ORDER BY desired_delivery_at ASC,id ASC LIMIT $6 OFFSET $7`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ScheduledOrder{}
	for rows.Next() {
		v, e := scanScheduled(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Section21Service) GetScheduledByStaff(ctx context.Context, actor Actor, id string) (ScheduledOrder, error) {
	scope, err := s.scheduledReadScope(ctx, actor, "")
	if err != nil {
		return ScheduledOrder{}, err
	}
	if strings.TrimSpace(id) == "" {
		return ScheduledOrder{}, ErrSection21NotFound
	}
	out, err := scanScheduled(s.db.QueryRow(ctx, `SELECT `+scheduledReadColumns+` FROM commerce_scheduled_orders`+scheduledReadPredicate, scheduledReadArgs(actor, scope, ScheduledReadFilter{}, id)...))
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrSection21NotFound
	}
	return out, err
}

func (s *Section21Service) ListRecurringByStaff(ctx context.Context, actor Actor, f ScheduledReadFilter) ([]RecurringSchedule, error) {
	f = normalizeScheduledFilter(f)
	view, err := scheduleViewPredicate(f.View, true)
	if err != nil {
		return nil, err
	}
	scope, err := s.scheduledReadScope(ctx, actor, f.BranchID)
	if err != nil {
		return nil, err
	}
	args := append(scheduledReadArgs(actor, scope, f, ""), f.Limit+1, f.Offset)
	rows, err := s.db.Query(ctx, `SELECT `+recurringReadColumns+` FROM commerce_recurring_schedules`+scheduledReadPredicate+view+` ORDER BY starts_on ASC,id ASC LIMIT $6 OFFSET $7`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RecurringSchedule{}
	for rows.Next() {
		v, e := scanRecurring(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Section21Service) GetRecurringByStaff(ctx context.Context, actor Actor, id string) (RecurringSchedule, error) {
	scope, err := s.scheduledReadScope(ctx, actor, "")
	if err != nil {
		return RecurringSchedule{}, err
	}
	if strings.TrimSpace(id) == "" {
		return RecurringSchedule{}, ErrSection21NotFound
	}
	out, err := scanRecurring(s.db.QueryRow(ctx, `SELECT `+recurringReadColumns+` FROM commerce_recurring_schedules`+scheduledReadPredicate, scheduledReadArgs(actor, scope, ScheduledReadFilter{}, id)...))
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrSection21NotFound
	}
	return out, err
}
