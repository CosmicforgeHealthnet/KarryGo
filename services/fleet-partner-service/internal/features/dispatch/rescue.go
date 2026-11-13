package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	privatemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/models"
	privateusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Rescue case statuses (spec 16.14, 45.2). A replacement provider is offered
// through an ordinary Section 15 private assignment: REPLACEMENT_PENDING
// means the provider has not yet accepted, and nothing is forced on them.
const (
	RescueOpen                = "OPEN"
	RescueReplacementPending  = "REPLACEMENT_PENDING"
	RescueReplacementAccepted = "REPLACEMENT_ACCEPTED"
	RescueResolved            = "RESOLVED"
	RescueCancelled           = "CANCELLED"
)

var rescueTriggers = map[string]bool{"BREAKDOWN": true, "PROVIDER_UNAVAILABLE": true, "DELAY": true, "FAILED_DELIVERY": true, "UNSAFE_VEHICLE": true, "ACCIDENT": true, "CARGO_TRANSFER": true, "OTHER": true}
var rescueSeverities = map[string]bool{"LOW": true, "MEDIUM": true, "HIGH": true, "CRITICAL": true}
var rescueResolutionCodes = map[string]bool{"REPLACEMENT_COMPLETED": true, "RECOVERED_ON_SITE": true, "CARGO_TRANSFERRED": true, "TRIP_CANCELLED_AT_SOURCE": true, "NO_ACTION_REQUIRED": true}

// RescueAssignmentPort is the narrow Section 15 surface rescue uses to offer
// replacement work. Both calls join the caller's transaction and keep every
// private-assignment gate (eligibility, history, audit, outbox).
type RescueAssignmentPort interface {
	CreateInTransaction(context.Context, pgx.Tx, tenant.Context, privateusecases.CreateInput) (privatemodels.Assignment, error)
	ReassignInTransaction(context.Context, pgx.Tx, tenant.Context, string, privateusecases.ReassignInput) (privatemodels.Assignment, error)
}

type RescueCase struct {
	ID                          string          `json:"id"`
	OrganizationID              string          `json:"organizationId"`
	BranchID                    string          `json:"branchId"`
	TriggerType                 string          `json:"triggerType"`
	Severity                    string          `json:"severity"`
	Status                      string          `json:"status"`
	PrivateAssignmentID         string          `json:"privateAssignmentId,omitempty"`
	CanonicalTripReference      string          `json:"canonicalTripReference,omitempty"`
	AssetID                     string          `json:"assetId,omitempty"`
	BreakdownID                 string          `json:"breakdownId,omitempty"`
	Reason                      string          `json:"reason"`
	AssignedToUserID            string          `json:"assignedToUserId,omitempty"`
	CurrentAssignmentID         string          `json:"currentAssignmentId,omitempty"`
	ReplacementAssignmentID     string          `json:"replacementAssignmentId,omitempty"`
	ReplacementAssignmentStatus string          `json:"replacementAssignmentStatus,omitempty"`
	ReplacementMembershipID     string          `json:"replacementMembershipId,omitempty"`
	ReplacementAssetReference   string          `json:"replacementAssetReference,omitempty"`
	ReplacementAttempts         int             `json:"replacementAttempts"`
	AlertID                     string          `json:"alertId,omitempty"`
	ResolutionCode              string          `json:"resolutionCode,omitempty"`
	Resolution                  json.RawMessage `json:"resolution"`
	ResolvedBy                  string          `json:"resolvedBy,omitempty"`
	ResolvedAt                  *time.Time      `json:"resolvedAt,omitempty"`
	CancelledAt                 *time.Time      `json:"cancelledAt,omitempty"`
	CreatedBy                   string          `json:"createdBy"`
	Version                     int64           `json:"version"`
	CreatedAt                   time.Time       `json:"createdAt"`
	UpdatedAt                   time.Time       `json:"updatedAt"`
}

type OpenRescueCaseInput struct {
	BranchID               string `json:"branchId"`
	TriggerType            string `json:"triggerType"`
	Severity               string `json:"severity"`
	PrivateAssignmentID    string `json:"privateAssignmentId"`
	CanonicalTripReference string `json:"canonicalTripReference"`
	AssetID                string `json:"assetId"`
	BreakdownID            string `json:"breakdownId"`
	Reason                 string `json:"reason"`
	AssignedToUserID       string `json:"assignedToUserId"`
	IdempotencyKey         string `json:"-"`
}

type RescueReplacementInput struct {
	ProviderMembershipID    string     `json:"providerMembershipId"`
	ProviderID              string     `json:"providerId"`
	VehicleOrTruckReference string     `json:"vehicleOrTruckReference"`
	ExecutionService        string     `json:"executionService"`
	ResponseDeadline        *time.Time `json:"responseDeadline"`
	ExpectedVersion         int64      `json:"expectedVersion"`
}

type ResolveRescueCaseInput struct {
	ResolutionCode  string `json:"resolutionCode"`
	Notes           string `json:"notes"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

type CancelRescueCaseInput struct {
	Reason          string `json:"reason"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

type RescueFilter struct {
	BranchID, Status, TriggerType string
	Limit, Offset                 int
}

const rescueColumns = `rc.id,rc.organization_id,rc.branch_id,rc.trigger_type,rc.severity,rc.status,COALESCE(rc.private_assignment_id,''),rc.canonical_trip_reference,COALESCE(rc.asset_id,''),COALESCE(rc.breakdown_id,''),rc.reason,rc.assigned_to_user_id,COALESCE(rc.current_assignment_id,''),COALESCE(rc.replacement_assignment_id,''),COALESCE(ra.status,''),rc.replacement_membership_id,rc.replacement_asset_reference,rc.replacement_attempts,rc.alert_id,rc.resolution_code,rc.resolution,rc.resolved_by,rc.resolved_at,rc.cancelled_at,rc.created_by,rc.version,rc.created_at,rc.updated_at`
const rescueFrom = ` FROM fleet_partner_rescue_cases rc LEFT JOIN fleet_partner_private_assignments ra ON ra.organization_id=rc.organization_id AND ra.id=rc.replacement_assignment_id`

func scanRescue(row pgx.Row) (RescueCase, error) {
	var v RescueCase
	var resolution []byte
	e := row.Scan(&v.ID, &v.OrganizationID, &v.BranchID, &v.TriggerType, &v.Severity, &v.Status, &v.PrivateAssignmentID, &v.CanonicalTripReference, &v.AssetID, &v.BreakdownID, &v.Reason, &v.AssignedToUserID, &v.CurrentAssignmentID, &v.ReplacementAssignmentID, &v.ReplacementAssignmentStatus, &v.ReplacementMembershipID, &v.ReplacementAssetReference, &v.ReplacementAttempts, &v.AlertID, &v.ResolutionCode, &resolution, &v.ResolvedBy, &v.ResolvedAt, &v.CancelledAt, &v.CreatedBy, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	v.Resolution = json.RawMessage(resolution)
	if len(v.Resolution) == 0 {
		v.Resolution = json.RawMessage(`{}`)
	}
	return v, e
}

func (s *Service) rescueGate(write bool) error {
	return s.gate(s.o.Flags.DispatchPlanning, write, "rescue_disabled")
}

func (s *Service) lockRescue(ctx context.Context, tx pgx.Tx, t tenant.Context, id string) (RescueCase, error) {
	v, e := scanRescue(tx.QueryRow(ctx, `SELECT `+rescueColumns+rescueFrom+` WHERE rc.organization_id=$1 AND rc.id=$2 FOR UPDATE OF rc`, t.OrganizationID, id))
	if e != nil || !t.BranchPermitted(v.BranchID) {
		if e != nil && !errors.Is(e, ErrNotFound) {
			return v, e
		}
		return v, nf()
	}
	return v, nil
}

// OpenRescueCase records a rescue need against company work. Every linked
// object is resolved server-side inside the caller's organization and branch
// scope; a reference the caller cannot see is reported as not found.
func (s *Service) OpenRescueCase(ctx context.Context, t tenant.Context, in OpenRescueCaseInput) (RescueCase, error) {
	if e := s.rescueGate(true); e != nil {
		return RescueCase{}, e
	}
	in.TriggerType = strings.ToUpper(strings.TrimSpace(in.TriggerType))
	in.Severity = strings.ToUpper(strings.TrimSpace(in.Severity))
	in.Reason = strings.TrimSpace(in.Reason)
	in.CanonicalTripReference = strings.TrimSpace(in.CanonicalTripReference)
	if !rescueTriggers[in.TriggerType] || !rescueSeverities[in.Severity] {
		return RescueCase{}, errorcodes.BadRequest("rescue_invalid", "triggerType or severity is invalid.")
	}
	if in.Reason == "" || len(in.Reason) > 2000 || strings.TrimSpace(in.IdempotencyKey) == "" {
		return RescueCase{}, errorcodes.BadRequest("rescue_invalid", "reason and Idempotency-Key are required.")
	}
	if in.PrivateAssignmentID == "" && in.CanonicalTripReference == "" && in.AssetID == "" && in.BreakdownID == "" {
		return RescueCase{}, errorcodes.BadRequest("rescue_invalid", "A rescue case must reference an assignment, trip, asset or breakdown.")
	}
	var out RescueCase
	e := s.tx(ctx, func(tx pgx.Tx) error {
		derivedBranch := ""
		if in.PrivateAssignmentID != "" {
			var branchID string
			if x := tx.QueryRow(ctx, `SELECT branch_id FROM fleet_partner_private_assignments WHERE organization_id=$1 AND id=$2`, t.OrganizationID, in.PrivateAssignmentID).Scan(&branchID); x != nil || !t.BranchPermitted(branchID) {
				return nf()
			}
			derivedBranch = branchID
		}
		if in.BreakdownID != "" {
			var assetID, branchID string
			if x := tx.QueryRow(ctx, `SELECT b.asset_id,a.branch_id FROM fleet_partner_breakdowns b JOIN fleet_partner_assets a ON a.organization_id=b.organization_id AND a.id=b.asset_id WHERE b.organization_id=$1 AND b.id=$2`, t.OrganizationID, in.BreakdownID).Scan(&assetID, &branchID); x != nil || !t.BranchPermitted(branchID) {
				return nf()
			}
			if in.AssetID != "" && in.AssetID != assetID {
				return errorcodes.BadRequest("rescue_invalid", "assetId does not match the breakdown's asset.")
			}
			in.AssetID = assetID
		}
		if in.AssetID != "" {
			var branchID string
			if x := tx.QueryRow(ctx, `SELECT branch_id FROM fleet_partner_assets WHERE organization_id=$1 AND id=$2`, t.OrganizationID, in.AssetID).Scan(&branchID); x != nil || !t.BranchPermitted(branchID) {
				return nf()
			}
			if derivedBranch == "" {
				derivedBranch = branchID
			}
		}
		branchID := derivedBranch
		if branchID == "" {
			if in.BranchID == "" {
				return errorcodes.BadRequest("rescue_invalid", "branchId is required when no assignment or asset is linked.")
			}
			var exists bool
			if x := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_branches WHERE organization_id=$1 AND id=$2)`, t.OrganizationID, in.BranchID).Scan(&exists); x != nil || !exists || !t.BranchPermitted(in.BranchID) {
				return nf()
			}
			branchID = in.BranchID
		} else if in.BranchID != "" && in.BranchID != branchID {
			return errorcodes.BadRequest("rescue_invalid", "branchId does not match the linked work.")
		}
		subject := ""
		switch {
		case in.CanonicalTripReference != "":
			subject = "trip:" + in.CanonicalTripReference
		case in.PrivateAssignmentID != "":
			subject = "assignment:" + in.PrivateAssignmentID
		case in.BreakdownID != "":
			subject = "breakdown:" + in.BreakdownID
		default:
			subject = "asset:" + in.AssetID
		}
		var inserted bool
		var id string
		x := tx.QueryRow(ctx, `INSERT INTO fleet_partner_rescue_cases(id,organization_id,branch_id,trigger_type,severity,subject_key,private_assignment_id,canonical_trip_reference,asset_id,breakdown_id,reason,assigned_to_user_id,current_assignment_id,created_by,idempotency_key,request_fingerprint)
VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8,NULLIF($9,''),NULLIF($10,''),$11,$12,NULLIF($7,''),$13,$14,$15)
ON CONFLICT(organization_id,idempotency_key) DO UPDATE SET idempotency_key=EXCLUDED.idempotency_key WHERE fleet_partner_rescue_cases.request_fingerprint=EXCLUDED.request_fingerprint
RETURNING id,(xmax=0)`, ids.New("rsc"), t.OrganizationID, branchID, in.TriggerType, in.Severity, subject, in.PrivateAssignmentID, in.CanonicalTripReference, in.AssetID, in.BreakdownID, in.Reason, in.AssignedToUserID, t.ActorID, in.IdempotencyKey, fingerprint(in)).Scan(&id, &inserted)
		if errors.Is(x, pgx.ErrNoRows) {
			return ErrConflict
		}
		var pg *pgconn.PgError
		if errors.As(x, &pg) && pg.Code == "23505" {
			return errorcodes.Conflict("rescue_case_already_open", "An open rescue case already exists for this work and trigger.")
		}
		if x != nil {
			return x
		}
		if inserted {
			if s.o.Flags.OperationalAlerts {
				alert, x := s.createAlert(ctx, tx, t, "RESCUE_REQUIRED", in.Severity, "RESCUE_CASE", id, branchID, "", in.PrivateAssignmentID, in.CanonicalTripReference, "rescue:"+id)
				if x != nil {
					return x
				}
				if _, x = tx.Exec(ctx, `UPDATE fleet_partner_rescue_cases SET alert_id=$3 WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id, alert.ID); x != nil {
					return x
				}
			}
		}
		out, x = s.lockRescue(ctx, tx, t, id)
		if x != nil {
			return x
		}
		if !inserted {
			return nil
		}
		if x = s.rescueHistory(ctx, tx, t, out, "OPENED", "", out.Status, in.Reason); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "rescue_case.opened.v1", "rescue_case", out.ID, out.Version, map[string]any{"caseId": out.ID, "tripId": out.CanonicalTripReference, "assignmentId": out.PrivateAssignmentID, "assetId": out.AssetID, "breakdownId": out.BreakdownID, "triggerType": out.TriggerType, "severity": out.Severity})
	})
	return out, e
}

func (s *Service) rescueHistory(ctx context.Context, tx pgx.Tx, t tenant.Context, v RescueCase, action, from, to, reason string) error {
	snapshot, _ := json.Marshal(map[string]any{"currentAssignmentId": v.CurrentAssignmentID, "replacementAssignmentId": v.ReplacementAssignmentID, "replacementAttempts": v.ReplacementAttempts, "resolutionCode": v.ResolutionCode})
	_, e := tx.Exec(ctx, `INSERT INTO fleet_partner_rescue_case_history(id,organization_id,rescue_case_id,action,previous_status,new_status,actor_id,reason,version,snapshot) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, ids.New("rch"), t.OrganizationID, v.ID, action, from, to, t.ActorID, reason, v.Version, snapshot)
	return e
}

func (s *Service) GetRescueCase(ctx context.Context, t tenant.Context, id string) (RescueCase, error) {
	if e := s.rescueGate(false); e != nil {
		return RescueCase{}, e
	}
	v, e := scanRescue(s.o.Pool.QueryRow(ctx, `SELECT `+rescueColumns+rescueFrom+` WHERE rc.organization_id=$1 AND rc.id=$2`, t.OrganizationID, id))
	if e != nil || !t.BranchPermitted(v.BranchID) {
		if e != nil && !errors.Is(e, ErrNotFound) {
			return RescueCase{}, e
		}
		return RescueCase{}, nf()
	}
	return v, nil
}

func (s *Service) ListRescueCases(ctx context.Context, t tenant.Context, f RescueFilter) ([]RescueCase, error) {
	if e := s.rescueGate(false); e != nil {
		return nil, e
	}
	if f.BranchID != "" && !t.BranchPermitted(f.BranchID) {
		return []RescueCase{}, nil
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+rescueColumns+rescueFrom+` WHERE rc.organization_id=$1 AND ($2='' OR rc.branch_id=$2) AND ($3='' OR rc.status=$3) AND ($4='' OR rc.trigger_type=$4) ORDER BY rc.created_at DESC,rc.id LIMIT $5 OFFSET $6`, t.OrganizationID, f.BranchID, strings.ToUpper(f.Status), strings.ToUpper(f.TriggerType), bounded(f.Limit), max(f.Offset, 0))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []RescueCase{}
	for rows.Next() {
		v, e := scanRescue(rows)
		if e != nil {
			return nil, e
		}
		if t.BranchPermitted(v.BranchID) {
			out = append(out, v)
		}
	}
	return out, rows.Err()
}

// syncRescueReplacement projects the replacement assignment's provider
// decision onto the case. Accepted (or later) moves the case to
// REPLACEMENT_ACCEPTED; a decline, expiry or cancellation reopens it so the
// dispatcher can offer the work to someone else.
func (s *Service) syncRescueReplacement(ctx context.Context, tx pgx.Tx, t tenant.Context, v *RescueCase) error {
	if v.Status != RescueReplacementPending || v.ReplacementAssignmentID == "" {
		return nil
	}
	to := ""
	event := ""
	switch v.ReplacementAssignmentStatus {
	case privatemodels.StatusAccepted, privatemodels.StatusReady, privatemodels.StatusInProgress, privatemodels.StatusCompleted:
		to, event = RescueReplacementAccepted, "rescue_case.replacement_accepted.v1"
	case privatemodels.StatusDeclined, privatemodels.StatusExpired, privatemodels.StatusCancelled, privatemodels.StatusReassignment:
		to, event = RescueOpen, "rescue_case.replacement_declined.v1"
	default:
		return nil
	}
	from := v.Status
	if e := tx.QueryRow(ctx, `UPDATE fleet_partner_rescue_cases SET status=$4,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING version,updated_at`, v.OrganizationID, v.ID, v.Version, to).Scan(&v.Version, &v.UpdatedAt); e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrConflict
		}
		return e
	}
	v.Status = to
	if e := s.rescueHistory(ctx, tx, t, *v, "REPLACEMENT_"+v.ReplacementAssignmentStatus, from, to, ""); e != nil {
		return e
	}
	return s.record(ctx, tx, t, event, "rescue_case", v.ID, v.Version, map[string]any{"caseId": v.ID, "replacementAssignmentId": v.ReplacementAssignmentID, "assignmentStatus": v.ReplacementAssignmentStatus})
}

// AssignRescueReplacement offers the case's work to a replacement provider.
// The offer is a normal pending private assignment: the provider accepts or
// declines it through the Section 15 provider endpoints, and the case only
// becomes REPLACEMENT_ACCEPTED after an explicit acceptance.
func (s *Service) AssignRescueReplacement(ctx context.Context, t tenant.Context, id string, in RescueReplacementInput) (RescueCase, error) {
	if e := s.rescueGate(true); e != nil {
		return RescueCase{}, e
	}
	if e := requireVersion(in.ExpectedVersion); e != nil {
		return RescueCase{}, e
	}
	if strings.TrimSpace(in.ProviderMembershipID) == "" || strings.TrimSpace(in.ProviderID) == "" {
		return RescueCase{}, errorcodes.BadRequest("rescue_invalid", "providerMembershipId and providerId are required.")
	}
	if s.o.Rescue == nil {
		return RescueCase{}, errorcodes.Unavailable("rescue_assignments_unavailable", "Replacement assignments are unavailable.")
	}
	var out RescueCase
	e := s.tx(ctx, func(tx pgx.Tx) error {
		v, x := s.lockRescue(ctx, tx, t, id)
		if x != nil {
			return x
		}
		if v.Version != in.ExpectedVersion {
			return ErrConflict
		}
		if x = s.syncRescueReplacement(ctx, tx, t, &v); x != nil {
			return x
		}
		if v.Status != RescueOpen {
			return errorcodes.Conflict("rescue_replacement_not_allowed", "A replacement can only be offered while the rescue case is open and no offer is pending.")
		}
		if v.CurrentAssignmentID == "" {
			return errorcodes.Conflict("rescue_not_linked_to_assignment", "A replacement provider can only be offered for a rescue case linked to a private assignment.")
		}
		var status, origin, workID, membership, service, branchID string
		var currentVersion int64
		if x = tx.QueryRow(ctx, `SELECT status,work_origin,COALESCE(private_order_id,shipment_id,route_stop_id,''),provider_membership_id,execution_service,branch_id,version FROM fleet_partner_private_assignments WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, v.CurrentAssignmentID).Scan(&status, &origin, &workID, &membership, &service, &branchID, &currentVersion); x != nil {
			return nf()
		}
		if membership == in.ProviderMembershipID {
			return errorcodes.BadRequest("rescue_invalid", "The replacement provider must differ from the provider currently holding the work.")
		}
		if strings.TrimSpace(in.ExecutionService) == "" {
			in.ExecutionService = service
		}
		var replacement privatemodels.Assignment
		switch status {
		case privatemodels.StatusDeclined, privatemodels.StatusExpired:
			replacement, x = s.o.Rescue.ReassignInTransaction(ctx, tx, t, v.CurrentAssignmentID, privateusecases.ReassignInput{ProviderMembershipID: in.ProviderMembershipID, ProviderID: in.ProviderID, VehicleOrTruckReference: in.VehicleOrTruckReference, ExecutionService: in.ExecutionService, ExpectedVersion: currentVersion, ResponseDeadline: in.ResponseDeadline})
		case privatemodels.StatusCancelled:
			replacement, x = s.o.Rescue.CreateInTransaction(ctx, tx, t, privateusecases.CreateInput{BranchID: branchID, WorkOrigin: origin, WorkID: workID, ProviderMembershipID: in.ProviderMembershipID, ProviderID: in.ProviderID, VehicleOrTruckReference: in.VehicleOrTruckReference, ExecutionService: in.ExecutionService, ResponseDeadline: in.ResponseDeadline})
		default:
			return errorcodes.Conflict("rescue_original_assignment_active", "The current assignment still holds the work. Cancel it, or wait for the provider's decline or expiry, before offering a replacement.")
		}
		if x != nil {
			return x
		}
		from := v.Status
		if x = tx.QueryRow(ctx, `UPDATE fleet_partner_rescue_cases SET status='REPLACEMENT_PENDING',current_assignment_id=$4,replacement_assignment_id=$4,replacement_membership_id=$5,replacement_asset_reference=$6,replacement_attempts=replacement_attempts+1,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING version`, t.OrganizationID, id, v.Version, replacement.ID, in.ProviderMembershipID, in.VehicleOrTruckReference).Scan(&v.Version); x != nil {
			return ErrConflict
		}
		out, x = s.lockRescue(ctx, tx, t, id)
		if x != nil {
			return x
		}
		if x = s.rescueHistory(ctx, tx, t, out, "REPLACEMENT_OFFERED", from, out.Status, ""); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "rescue_case.assigned.v1", "rescue_case", out.ID, out.Version, map[string]any{"caseId": out.ID, "replacementAssignmentId": replacement.ID, "replacementMembershipId": in.ProviderMembershipID, "providerAcceptanceRequired": true, "attempt": out.ReplacementAttempts})
	})
	return out, e
}

func (s *Service) ResolveRescueCase(ctx context.Context, t tenant.Context, id string, in ResolveRescueCaseInput) (RescueCase, error) {
	if e := s.rescueGate(true); e != nil {
		return RescueCase{}, e
	}
	if e := requireVersion(in.ExpectedVersion); e != nil {
		return RescueCase{}, e
	}
	in.ResolutionCode = strings.ToUpper(strings.TrimSpace(in.ResolutionCode))
	in.Notes = strings.TrimSpace(in.Notes)
	if !rescueResolutionCodes[in.ResolutionCode] || in.Notes == "" {
		return RescueCase{}, errorcodes.BadRequest("rescue_invalid", "A valid resolutionCode and notes are required.")
	}
	var out RescueCase
	e := s.tx(ctx, func(tx pgx.Tx) error {
		v, x := s.lockRescue(ctx, tx, t, id)
		if x != nil {
			return x
		}
		if v.Version != in.ExpectedVersion {
			return ErrConflict
		}
		if x = s.syncRescueReplacement(ctx, tx, t, &v); x != nil {
			return x
		}
		switch v.Status {
		case RescueResolved, RescueCancelled:
			return errorcodes.Conflict("rescue_case_closed", "The rescue case is already closed.")
		case RescueReplacementPending:
			return errorcodes.Conflict("rescue_replacement_pending", "The replacement provider has not responded; the case cannot be resolved yet.")
		}
		if in.ResolutionCode == "REPLACEMENT_COMPLETED" && v.Status != RescueReplacementAccepted {
			return errorcodes.Conflict("rescue_replacement_not_accepted", "REPLACEMENT_COMPLETED requires a replacement the provider accepted.")
		}
		resolution, _ := json.Marshal(map[string]any{"code": in.ResolutionCode, "notes": in.Notes, "replacementAssignmentId": v.ReplacementAssignmentID})
		from := v.Status
		if x = tx.QueryRow(ctx, `UPDATE fleet_partner_rescue_cases SET status='RESOLVED',resolution_code=$4,resolution=$5,resolved_by=$6,resolved_at=now(),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING version`, t.OrganizationID, id, v.Version, in.ResolutionCode, resolution, t.ActorID).Scan(&v.Version); x != nil {
			return ErrConflict
		}
		if x = s.closeRescueAlert(ctx, tx, t, v.AlertID, AlertResolved, "RESCUE_CASE_RESOLVED", in.Notes); x != nil {
			return x
		}
		out, x = s.lockRescue(ctx, tx, t, id)
		if x != nil {
			return x
		}
		if x = s.rescueHistory(ctx, tx, t, out, "RESOLVED", from, out.Status, in.Notes); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "rescue_case.resolved.v1", "rescue_case", out.ID, out.Version, map[string]any{"caseId": out.ID, "tripId": out.CanonicalTripReference, "severity": out.Severity, "resolution": in.ResolutionCode})
	})
	return out, e
}

func (s *Service) CancelRescueCase(ctx context.Context, t tenant.Context, id string, in CancelRescueCaseInput) (RescueCase, error) {
	if e := s.rescueGate(true); e != nil {
		return RescueCase{}, e
	}
	if e := requireVersion(in.ExpectedVersion); e != nil {
		return RescueCase{}, e
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Reason == "" {
		return RescueCase{}, errorcodes.BadRequest("rescue_invalid", "reason is required.")
	}
	var out RescueCase
	e := s.tx(ctx, func(tx pgx.Tx) error {
		v, x := s.lockRescue(ctx, tx, t, id)
		if x != nil {
			return x
		}
		if v.Version != in.ExpectedVersion {
			return ErrConflict
		}
		if x = s.syncRescueReplacement(ctx, tx, t, &v); x != nil {
			return x
		}
		if v.Status != RescueOpen {
			return errorcodes.Conflict("rescue_cancel_not_allowed", "Only an open rescue case without a pending or accepted replacement can be cancelled.")
		}
		if x = tx.QueryRow(ctx, `UPDATE fleet_partner_rescue_cases SET status='CANCELLED',cancelled_at=now(),resolution=$4,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING version`, t.OrganizationID, id, v.Version, mustJSONMap(map[string]any{"cancelReason": in.Reason})).Scan(&v.Version); x != nil {
			return ErrConflict
		}
		if x = s.closeRescueAlert(ctx, tx, t, v.AlertID, AlertDismissed, "RESCUE_CASE_CANCELLED", in.Reason); x != nil {
			return x
		}
		out, x = s.lockRescue(ctx, tx, t, id)
		if x != nil {
			return x
		}
		if x = s.rescueHistory(ctx, tx, t, out, "CANCELLED", RescueOpen, out.Status, in.Reason); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "rescue_case.cancelled.v1", "rescue_case", out.ID, out.Version, map[string]any{"caseId": out.ID, "reason": in.Reason})
	})
	return out, e
}

func mustJSONMap(v map[string]any) []byte { b, _ := json.Marshal(v); return b }

// closeRescueAlert closes the alert raised for a case once the case itself is
// closed: the alert's source condition no longer exists.
func (s *Service) closeRescueAlert(ctx context.Context, tx pgx.Tx, t tenant.Context, alertID, to, code, reason string) error {
	if alertID == "" {
		return nil
	}
	var from string
	var version int64
	x := tx.QueryRow(ctx, `SELECT status FROM fleet_partner_operational_alerts WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, alertID).Scan(&from)
	if errors.Is(x, pgx.ErrNoRows) || from == AlertResolved || from == AlertDismissed {
		return nil
	}
	if x != nil {
		return x
	}
	if x = tx.QueryRow(ctx, `UPDATE fleet_partner_operational_alerts SET status=$3,resolved_at=CASE WHEN $3='RESOLVED' THEN now() ELSE resolved_at END,resolution_code=$4,resolution_reason=$5,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, t.OrganizationID, alertID, to, code, reason).Scan(&version); x != nil {
		return x
	}
	if _, x = tx.Exec(ctx, `INSERT INTO fleet_partner_operational_alert_history(id,organization_id,alert_id,previous_status,new_status,actor_id,reason,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, ids.New("alh"), t.OrganizationID, alertID, from, to, t.ActorID, reason, version); x != nil {
		return x
	}
	return s.record(ctx, tx, t, "operational_alert."+strings.ToLower(to)+".v1", "operational_alert", alertID, version, map[string]any{"alertId": alertID, "status": to, "resolutionCode": code})
}

// SyncRescueReplacements is the durable sweep that projects provider
// responses onto rescue cases even when no dispatcher touches the case.
func (s *Service) SyncRescueReplacements(ctx context.Context, tx pgx.Tx, limit int) error {
	rows, e := tx.Query(ctx, `SELECT rc.organization_id,rc.id FROM fleet_partner_rescue_cases rc JOIN fleet_partner_private_assignments ra ON ra.organization_id=rc.organization_id AND ra.id=rc.replacement_assignment_id
WHERE rc.status='REPLACEMENT_PENDING' AND ra.status<>'PENDING_PROVIDER_RESPONSE' ORDER BY rc.updated_at LIMIT $1`, limit)
	if e != nil {
		return e
	}
	type ref struct{ org, id string }
	due := []ref{}
	for rows.Next() {
		var r ref
		if e = rows.Scan(&r.org, &r.id); e != nil {
			rows.Close()
			return e
		}
		due = append(due, r)
	}
	if e = rows.Err(); e != nil {
		rows.Close()
		return e
	}
	rows.Close()
	for _, r := range due {
		t := tenant.Context{OrganizationID: r.org, ActorType: tenant.ActorTypeSystem, ActorID: "rescue-replacement-sync", CorrelationID: r.id}
		v, x := s.lockRescue(ctx, tx, t, r.id)
		if x != nil {
			return x
		}
		if x = s.syncRescueReplacement(ctx, tx, t, &v); x != nil {
			return x
		}
	}
	return nil
}

var _ = apperrors.BadRequest
