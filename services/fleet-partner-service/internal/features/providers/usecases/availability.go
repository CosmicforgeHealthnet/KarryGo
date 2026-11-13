package usecases

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Provider availability (spec 39.3 GET /providers/{id}/availability,
// FLEET-10.11).
//
// Availability is a read-only derivation, never a stored flag a dispatcher
// could set: it is recomputed on every request from the facts that decide
// it -- membership status, approved leave, shift assignment, mandatory
// qualification expiry, live company work and the latest tracking
// projection. Every input is returned beside the answer, and every reason
// names the input it came from, so the result is explainable and can be
// checked against the source records.

// Availability outcomes.
const (
	AvailabilityAvailable   = "AVAILABLE"
	AvailabilityBusy        = "BUSY"
	AvailabilityUnavailable = "UNAVAILABLE"
)

// Reason effects.
const (
	EffectBlocking      = "BLOCKING"
	EffectBusy          = "BUSY"
	EffectInformational = "INFORMATIONAL"
)

// AvailabilityExpiryWarning is how far ahead an expiring mandatory
// qualification is flagged (informational only).
const AvailabilityExpiryWarning = 30 * 24 * time.Hour

// AvailabilityReason is one explained contribution to the answer.
type AvailabilityReason struct {
	Code   string         `json:"code"`
	Effect string         `json:"effect"`
	Source string         `json:"source"`
	Detail map[string]any `json:"detail,omitempty"`
}

// LeaveWindow is an approved leave record covering the evaluation instant.
type LeaveWindow struct {
	LeaveID   string    `json:"leaveId"`
	LeaveType string    `json:"leaveType"`
	StartAt   time.Time `json:"startAt"`
	EndAt     time.Time `json:"endAt"`
}

// ShiftInput is the shift side of availability.
type ShiftInput struct {
	// ScheduleInForce is true when the organization runs a published shift
	// covering the evaluation instant; only then is being off shift a block.
	ScheduleInForce          bool       `json:"scheduleInForce"`
	OnShift                  bool       `json:"onShift"`
	CurrentShiftAssignmentID *string    `json:"currentShiftAssignmentId,omitempty"`
	CurrentShiftEndsAt       *time.Time `json:"currentShiftEndsAt,omitempty"`
	NextShiftStartsAt        *time.Time `json:"nextShiftStartsAt,omitempty"`
}

// QualificationRef names one mandatory qualification.
type QualificationRef struct {
	QualificationID string     `json:"qualificationId"`
	Code            string     `json:"code"`
	Name            string     `json:"name"`
	ExpiredAt       *time.Time `json:"expiredAt,omitempty"`
	ValidUntil      *time.Time `json:"validUntil,omitempty"`
}

// QualificationInput is the qualification side of availability. Mandatory
// means ACTIVE and applicable to the provider's service category or
// workforce role.
type QualificationInput struct {
	MandatoryCount int                `json:"mandatoryCount"`
	Expired        []QualificationRef `json:"expired"`
	Missing        []QualificationRef `json:"missing"`
	ExpiringSoon   []QualificationRef `json:"expiringSoon"`
}

// WorkInput counts live company work held by the provider.
type WorkInput struct {
	Marketplace int `json:"marketplace"`
	Private     int `json:"private"`
	Total       int `json:"total"`
}

// TrackingInput is the provider's latest tracking projection.
type TrackingInput struct {
	FreshnessStatus string     `json:"freshnessStatus"`
	LastObservedAt  *time.Time `json:"lastObservedAt,omitempty"`
	SourceService   string     `json:"sourceService,omitempty"`
}

// AvailabilityInputs is every fact the answer was derived from.
type AvailabilityInputs struct {
	MembershipStatus string             `json:"membershipStatus"`
	ApprovedLeave    *LeaveWindow       `json:"approvedLeave,omitempty"`
	Shift            ShiftInput         `json:"shift"`
	Qualifications   QualificationInput `json:"qualifications"`
	ActiveWork       WorkInput          `json:"activeWork"`
	Tracking         TrackingInput      `json:"tracking"`
}

// ProviderAvailability is the derived answer with its explanation.
type ProviderAvailability struct {
	MembershipID string               `json:"membershipId"`
	Status       string               `json:"status"`
	EvaluatedAt  time.Time            `json:"evaluatedAt"`
	Reasons      []AvailabilityReason `json:"reasons"`
	Inputs       AvailabilityInputs   `json:"inputs"`
	Derivation   string               `json:"derivation"`
}

const availabilityDerivation = "UNAVAILABLE when any BLOCKING reason applies (membership not active, approved leave, " +
	"off shift while a published schedule is in force, missing or expired mandatory qualification); otherwise BUSY " +
	"when the provider holds live company work; otherwise AVAILABLE. INFORMATIONAL reasons never change the outcome. " +
	"Read-only: nothing is stored and no provider is penalised."

// Availability derives the provider's availability at the current instant.
func (s *Service) Availability(ctx context.Context, tenantCtx tenant.Context, membershipID string) (*ProviderAvailability, error) {
	membership, err := s.VisibleMembership(ctx, tenantCtx, membershipID)
	if err != nil {
		return nil, err
	}
	now := s.opts.Now().UTC()
	org := tenantCtx.OrganizationID
	result := &ProviderAvailability{MembershipID: membership.ID, EvaluatedAt: now, Reasons: []AvailabilityReason{},
		Derivation: availabilityDerivation}
	inputs := &result.Inputs
	inputs.MembershipStatus = membership.Status
	inputs.Qualifications = QualificationInput{Expired: []QualificationRef{}, Missing: []QualificationRef{}, ExpiringSoon: []QualificationRef{}}
	add := func(code, effect, source string, detail map[string]any) {
		result.Reasons = append(result.Reasons, AvailabilityReason{Code: code, Effect: effect, Source: source, Detail: detail})
	}

	if membership.Status != models.StatusActive {
		add("MEMBERSHIP_NOT_ACTIVE", EffectBlocking, "membership", map[string]any{"status": membership.Status})
	}

	// Approved leave covering now.
	var leave LeaveWindow
	err = s.opts.Pool.QueryRow(ctx, `SELECT id, leave_type, start_at, end_at FROM fleet_partner_leave_records
		WHERE organization_id = $1 AND membership_id = $2 AND status = 'APPROVED' AND start_at <= $3 AND end_at > $3
		ORDER BY end_at DESC LIMIT 1`, org, membershipID, now).Scan(&leave.LeaveID, &leave.LeaveType, &leave.StartAt, &leave.EndAt)
	switch {
	case err == nil:
		inputs.ApprovedLeave = &leave
		add("APPROVED_LEAVE", EffectBlocking, "leave", map[string]any{"leaveId": leave.LeaveID, "until": leave.EndAt})
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, err
	}

	// Shift: the same rule the workforce eligibility gate applies -- off shift
	// only blocks while the organization runs a published shift right now.
	shift := &inputs.Shift
	if err := s.opts.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM fleet_partner_shifts
		WHERE organization_id = $1 AND status IN ('PUBLISHED', 'IN_PROGRESS') AND start_at <= $2 AND end_at > $2)`,
		org, now).Scan(&shift.ScheduleInForce); err != nil {
		return nil, err
	}
	var currentID string
	var currentEnd time.Time
	err = s.opts.Pool.QueryRow(ctx, `SELECT id, expected_end FROM fleet_partner_shift_assignments
		WHERE organization_id = $1 AND membership_id = $2 AND status IN ('CONFIRMED', 'CHECKED_IN')
		  AND expected_start <= $3 AND expected_end > $3
		ORDER BY expected_end DESC LIMIT 1`, org, membershipID, now).Scan(&currentID, &currentEnd)
	switch {
	case err == nil:
		shift.OnShift, shift.CurrentShiftAssignmentID, shift.CurrentShiftEndsAt = true, &currentID, &currentEnd
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, err
	}
	var next *time.Time
	if err := s.opts.Pool.QueryRow(ctx, `SELECT min(expected_start) FROM fleet_partner_shift_assignments
		WHERE organization_id = $1 AND membership_id = $2 AND status IN ('PENDING', 'CONFIRMED') AND expected_start > $3`,
		org, membershipID, now).Scan(&next); err != nil {
		return nil, err
	}
	shift.NextShiftStartsAt = next
	switch {
	case shift.ScheduleInForce && !shift.OnShift:
		add("OFF_SHIFT", EffectBlocking, "shift", map[string]any{"nextShiftStartsAt": next})
	case !shift.ScheduleInForce && !shift.OnShift:
		add("NO_SHIFT_SCHEDULE_IN_FORCE", EffectInformational, "shift", nil)
	}

	// Mandatory qualifications for the provider's service category or role.
	var role string
	err = s.opts.Pool.QueryRow(ctx, `SELECT role FROM fleet_partner_workforce_profiles
		WHERE organization_id = $1 AND membership_id = $2 AND status = 'ACTIVE' LIMIT 1`, org, membershipID).Scan(&role)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	rows, err := s.opts.Pool.Query(ctx, `
		SELECT q.id, q.code, q.name,
		  (SELECT max(pq.expires_at) FROM fleet_partner_provider_qualifications pq
		    WHERE pq.organization_id = $1 AND pq.membership_id = $2 AND pq.qualification_id = q.id
		      AND pq.status = 'VERIFIED' AND (pq.expires_at IS NULL OR pq.expires_at > $5)) AS valid_until,
		  EXISTS (SELECT 1 FROM fleet_partner_provider_qualifications pq
		    WHERE pq.organization_id = $1 AND pq.membership_id = $2 AND pq.qualification_id = q.id
		      AND pq.status = 'VERIFIED' AND (pq.expires_at IS NULL OR pq.expires_at > $5)) AS valid,
		  (SELECT max(pq.expires_at) FROM fleet_partner_provider_qualifications pq
		    WHERE pq.organization_id = $1 AND pq.membership_id = $2 AND pq.qualification_id = q.id
		      AND (pq.status = 'EXPIRED' OR (pq.status = 'VERIFIED' AND pq.expires_at <= $5))) AS expired_at
		FROM fleet_partner_qualifications q
		WHERE q.organization_id = $1 AND q.status = 'ACTIVE'
		  AND (q.applicable_services ? $3 OR ($4 <> '' AND q.applicable_roles ? $4))
		ORDER BY q.code, q.id`, org, membershipID, membership.ServiceCategory, role, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	quals := &inputs.Qualifications
	for rows.Next() {
		var ref QualificationRef
		var valid bool
		if err := rows.Scan(&ref.QualificationID, &ref.Code, &ref.Name, &ref.ValidUntil, &valid, &ref.ExpiredAt); err != nil {
			return nil, err
		}
		quals.MandatoryCount++
		switch {
		case valid:
			if ref.ValidUntil != nil && ref.ValidUntil.Sub(now) <= AvailabilityExpiryWarning {
				ref.ExpiredAt = nil
				quals.ExpiringSoon = append(quals.ExpiringSoon, ref)
			}
		case ref.ExpiredAt != nil:
			ref.ValidUntil = nil
			quals.Expired = append(quals.Expired, ref)
		default:
			ref.ValidUntil = nil
			quals.Missing = append(quals.Missing, ref)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for _, ref := range quals.Expired {
		add("QUALIFICATION_EXPIRED", EffectBlocking, "qualifications", map[string]any{"code": ref.Code, "expiredAt": ref.ExpiredAt})
	}
	for _, ref := range quals.Missing {
		add("QUALIFICATION_MISSING", EffectBlocking, "qualifications", map[string]any{"code": ref.Code})
	}
	for _, ref := range quals.ExpiringSoon {
		add("QUALIFICATION_EXPIRING_SOON", EffectInformational, "qualifications", map[string]any{"code": ref.Code, "validUntil": ref.ValidUntil})
	}

	// Live company work, marketplace and private.
	if err := s.opts.Pool.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM fleet_partner_provider_assignments
		    WHERE organization_id = $1 AND membership_id = $2 AND status = ANY($3::text[])),
		  (SELECT count(*) FROM fleet_partner_private_assignments
		    WHERE organization_id = $1 AND provider_membership_id = $2 AND status = ANY($3::text[]))`,
		org, membershipID, liveWorkStatuses).Scan(&inputs.ActiveWork.Marketplace, &inputs.ActiveWork.Private); err != nil {
		return nil, err
	}
	inputs.ActiveWork.Total = inputs.ActiveWork.Marketplace + inputs.ActiveWork.Private
	if inputs.ActiveWork.Total > 0 {
		add("ACTIVE_COMPANY_WORK", EffectBusy, "assignments", map[string]any{"count": inputs.ActiveWork.Total})
	}

	// Tracking freshness of the provider's latest projection. Tracking is per
	// trip, so it is informational: a provider with no live trip has no feed.
	inputs.Tracking.FreshnessStatus = "UNKNOWN"
	if membership.ProviderID != nil {
		var observed time.Time
		var freshness, source string
		err = s.opts.Pool.QueryRow(ctx, `SELECT source_timestamp, freshness_status, source_service
			FROM fleet_partner_tracking_projections
			WHERE organization_id = $1 AND provider_id = $2
			ORDER BY source_timestamp DESC LIMIT 1`, org, *membership.ProviderID).Scan(&observed, &freshness, &source)
		switch {
		case err == nil:
			inputs.Tracking = TrackingInput{FreshnessStatus: freshness, LastObservedAt: &observed, SourceService: source}
		case !errors.Is(err, pgx.ErrNoRows):
			return nil, err
		}
	}
	if inputs.ActiveWork.Total > 0 && inputs.Tracking.FreshnessStatus != "FRESH" && inputs.Tracking.FreshnessStatus != "AGING" {
		add("TRACKING_NOT_FRESH", EffectInformational, "tracking", map[string]any{"freshnessStatus": inputs.Tracking.FreshnessStatus})
	}

	result.Status = AvailabilityAvailable
	for _, reason := range result.Reasons {
		if reason.Effect == EffectBlocking {
			result.Status = AvailabilityUnavailable
			break
		}
		if reason.Effect == EffectBusy {
			result.Status = AvailabilityBusy
		}
	}
	return result, nil
}
