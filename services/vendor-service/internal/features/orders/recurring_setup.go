package orders

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
)

type RecurringSetupContext struct {
	BusinessID, BranchID, RequesterID, CorporateRef, OrganizationName string
	DestinationSnapshot, MenuRotation                                 map[string]any
}

func recurringSetupContext(q SchedulingQuote, corporate, name string) (RecurringSetupContext, error) {
	if q.BusinessID == "" || q.BranchID == "" || q.CustomerID == "" || q.QuoteID == "" || q.Currency == "" || q.ApprovedAt == nil || corporate == "" || len(q.Lines) == 0 || q.Address.SourceAddressRef == "" || q.Address.Address == "" {
		return RecurringSetupContext{}, ErrSection21Schedule
	}
	for _, line := range q.Lines {
		if line.BusinessID != q.BusinessID || line.BranchID != q.BranchID || line.Domain != "FOOD" || line.ItemRef == "" || line.Quantity <= 0 {
			return RecurringSetupContext{}, ErrSection21Schedule
		}
	}
	raw, err := json.Marshal(q.Address)
	if err != nil {
		return RecurringSetupContext{}, err
	}
	var address map[string]any
	if err = json.Unmarshal(raw, &address); err != nil {
		return RecurringSetupContext{}, err
	}
	return RecurringSetupContext{BusinessID: q.BusinessID, BranchID: q.BranchID, RequesterID: q.CustomerID, CorporateRef: corporate, OrganizationName: name, DestinationSnapshot: address, MenuRotation: map[string]any{"schema_version": 1, "mode": "FIXED", "source_quote_id": q.QuoteID, "quote_version": q.QuoteVersion, "currency": q.Currency, "lines": q.Lines}}, nil
}

func (s *Section21Service) recurringCorporateScope(ctx context.Context, actor Actor) (Scope, error) {
	if err := s.available(); err != nil {
		return Scope{}, err
	}
	if s.scopes == nil || actor.StaffID == "" || actor.VendorID == "" {
		return Scope{}, ErrSection21Forbidden
	}
	scope, err := s.scopes.ResolveScope(ctx, actor.StaffID, "vendor.crm.corporate.read")
	if err != nil {
		return Scope{}, err
	}
	if scope.Empty() {
		return Scope{}, ErrSection21Forbidden
	}
	return scope, nil
}

const recurringCorporateBinding = ` FROM corporate_customer_profiles c WHERE c.business_id=$1 AND c.status='ACTIVE'
 AND ($4::boolean OR (c.scope_mode='BRANCH_SET' AND EXISTS(SELECT 1 FROM corporate_customer_branch_scopes bs WHERE bs.business_id=c.business_id AND bs.corporate_customer_id=c.id AND bs.branch_id=ANY($5::text[]))))
 AND (c.scope_mode='BUSINESS' OR EXISTS(SELECT 1 FROM corporate_customer_branch_scopes bs WHERE bs.business_id=c.business_id AND bs.corporate_customer_id=c.id AND bs.branch_id=$3))
 AND EXISTS(SELECT 1 FROM corporate_customer_contacts ct JOIN vendor_customer_profiles p ON p.business_id=ct.business_id AND p.id=ct.customer_profile_id WHERE ct.business_id=c.business_id AND ct.corporate_customer_id=c.id AND ct.status='ACTIVE' AND ct.privacy_suppressed=false AND p.customer_ref=$2 AND p.status='ACTIVE' AND p.privacy_state='ACTIVE' AND p.redacted_at IS NULL AND p.merged_into_profile_id IS NULL AND p.subject_type='CUSTOMER' AND p.merge_state IN ('NONE','MERGED_TARGET'))`

func (s *Section21Service) ListRecurringSetupContexts(ctx context.Context, actor Actor, f ScheduledReadFilter) ([]RecurringSetupContext, bool, error) {
	scope, err := s.recurringCorporateScope(ctx, actor)
	if err != nil {
		return nil, false, err
	}
	f = normalizeScheduledFilter(f)
	quotes, err := s.ListSchedulingQuotes(ctx, actor, f)
	if err != nil {
		return nil, false, err
	}
	branches := scope.BranchIDs
	if branches == nil {
		branches = []string{}
	}
	more := len(quotes) > f.Limit
	if more {
		quotes = quotes[:f.Limit]
	}
	out := []RecurringSetupContext{}
	for _, q := range quotes {
		rows, err := s.db.Query(ctx, `SELECT c.id,c.organization_name`+recurringCorporateBinding+` ORDER BY c.id`, actor.VendorID, q.CustomerID, q.BranchID, scope.Global, branches)
		if err != nil {
			return nil, false, err
		}
		for rows.Next() {
			var id, name string
			if err = rows.Scan(&id, &name); err != nil {
				rows.Close()
				return nil, false, err
			}
			item, e := recurringSetupContext(q, id, name)
			if e == nil {
				out = append(out, item)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, false, err
		}
	}
	return out, more, nil
}

func equalRecurringSnapshot(a, b any) bool {
	left, e1 := json.Marshal(a)
	right, e2 := json.Marshal(b)
	if e1 != nil || e2 != nil {
		return false
	}
	var l, r any
	ld, rd := json.NewDecoder(bytes.NewReader(left)), json.NewDecoder(bytes.NewReader(right))
	ld.UseNumber()
	rd.UseNumber()
	if ld.Decode(&l) != nil || rd.Decode(&r) != nil {
		return false
	}
	left, _ = json.Marshal(l)
	right, _ = json.Marshal(r)
	return string(left) == string(right)
}

func (s *Section21Service) CreateRecurringByStaff(ctx context.Context, actor Actor, in RecurringScheduleInput) (RecurringSchedule, bool, error) {
	if err := s.AuthorizeStaff(ctx, actor, in.BranchID); err != nil {
		return RecurringSchedule{}, false, err
	}
	in.BusinessID = actor.VendorID
	existing, err := scanRecurring(s.db.QueryRow(ctx, `SELECT `+recurringReadColumns+` FROM commerce_recurring_schedules WHERE business_id=$1 AND idempotency_key=$2`, actor.VendorID, in.IdempotencyKey))
	if err == nil {
		if existing.BranchID != in.BranchID || existing.CorporateRef != in.CorporateRef || existing.RequesterID != in.RequesterID {
			return RecurringSchedule{}, false, ErrSection21Conflict
		}
		return existing, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return RecurringSchedule{}, false, err
	}
	if _, typed := in.MenuRotation["schema_version"]; typed {
		scope, err := s.recurringCorporateScope(ctx, actor)
		if err != nil {
			return RecurringSchedule{}, false, err
		}
		branches := scope.BranchIDs
		if branches == nil {
			branches = []string{}
		}
		var name string
		if err = s.db.QueryRow(ctx, `SELECT c.organization_name`+recurringCorporateBinding+` AND c.id=$6`, actor.VendorID, in.RequesterID, in.BranchID, scope.Global, branches, in.CorporateRef).Scan(&name); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return RecurringSchedule{}, false, ErrSection21Forbidden
			}
			return RecurringSchedule{}, false, err
		}
		var preserved bool
		err = s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM commerce_recurring_schedules WHERE business_id=$1 AND branch_id=$2 AND requester_id=$3 AND corporate_ref=$4 AND menu_rotation=$5::jsonb AND destination_snapshot=$6::jsonb)`, actor.VendorID, in.BranchID, in.RequesterID, in.CorporateRef, in.MenuRotation, in.DestinationSnapshot).Scan(&preserved)
		if err != nil {
			return RecurringSchedule{}, false, err
		}
		if !preserved {
			quoteID, _ := in.MenuRotation["source_quote_id"].(string)
			q, err := s.GetSchedulingQuote(ctx, actor, quoteID, in.BranchID)
			if err != nil {
				return RecurringSchedule{}, false, err
			}
			if q.CustomerID != in.RequesterID {
				return RecurringSchedule{}, false, ErrSection21Forbidden
			}
			expected, err := recurringSetupContext(q, in.CorporateRef, name)
			if err != nil {
				return RecurringSchedule{}, false, err
			}
			if !equalRecurringSnapshot(in.MenuRotation, expected.MenuRotation) || !equalRecurringSnapshot(in.DestinationSnapshot, expected.DestinationSnapshot) {
				return RecurringSchedule{}, false, ErrSection21Schedule
			}
		}
	}
	return s.CreateRecurringSchedule(ctx, in)
}
