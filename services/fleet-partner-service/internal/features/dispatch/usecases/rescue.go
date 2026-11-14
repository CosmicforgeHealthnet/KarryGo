package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch/models"
	privatemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/models"
	privateusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

func (s *Service) rescueGate(write bool) error {
	return s.gate(s.o.Flags.DispatchPlanning, write, "rescue_disabled")
}

func (s *Service) lockRescue(ctx context.Context, tx pgx.Tx, t tenant.Context, id string) (models.RescueCase, error) {
	v, e := s.repo.GetRescue(ctx, tx, t.OrganizationID, id, true)
	if e != nil || !t.BranchPermitted(v.BranchID) {
		if e != nil && !errors.Is(e, models.ErrNotFound) {
			return v, e
		}
		return v, nf()
	}
	return v, nil
}

func (s *Service) rescueHistory(ctx context.Context, tx pgx.Tx, t tenant.Context, v models.RescueCase, action, from, to, reason string) error {
	return s.repo.RescueHistory(ctx, tx, t.OrganizationID, v, action, from, to, t.ActorID, reason)
}

// OpenRescueCase records a rescue need against company work. Every linked
// object is resolved server-side inside the caller's organization and branch
// scope; a reference the caller cannot see is reported as not found.
func (s *Service) OpenRescueCase(ctx context.Context, t tenant.Context, in models.OpenRescueCaseInput) (models.RescueCase, error) {
	if e := s.rescueGate(true); e != nil {
		return models.RescueCase{}, e
	}
	in.TriggerType = strings.ToUpper(strings.TrimSpace(in.TriggerType))
	in.Severity = strings.ToUpper(strings.TrimSpace(in.Severity))
	in.Reason = strings.TrimSpace(in.Reason)
	in.CanonicalTripReference = strings.TrimSpace(in.CanonicalTripReference)
	if !models.ValidRescueTrigger(in.TriggerType) || !models.ValidRescueSeverity(in.Severity) {
		return models.RescueCase{}, errorcodes.BadRequest("rescue_invalid", "triggerType or severity is invalid.")
	}
	if in.Reason == "" || len(in.Reason) > 2000 || strings.TrimSpace(in.IdempotencyKey) == "" {
		return models.RescueCase{}, errorcodes.BadRequest("rescue_invalid", "reason and Idempotency-Key are required.")
	}
	if in.PrivateAssignmentID == "" && in.CanonicalTripReference == "" && in.AssetID == "" && in.BreakdownID == "" {
		return models.RescueCase{}, errorcodes.BadRequest("rescue_invalid", "A rescue case must reference an assignment, trip, asset or breakdown.")
	}
	var out models.RescueCase
	e := s.tx(ctx, func(tx pgx.Tx) error {
		derivedBranch := ""
		if in.PrivateAssignmentID != "" {
			branchID, x := s.repo.AssignmentBranch(ctx, tx, t.OrganizationID, in.PrivateAssignmentID)
			if x != nil || !t.BranchPermitted(branchID) {
				return nf()
			}
			derivedBranch = branchID
		}
		if in.BreakdownID != "" {
			assetID, branchID, x := s.repo.BreakdownAsset(ctx, tx, t.OrganizationID, in.BreakdownID)
			if x != nil || !t.BranchPermitted(branchID) {
				return nf()
			}
			if in.AssetID != "" && in.AssetID != assetID {
				return errorcodes.BadRequest("rescue_invalid", "assetId does not match the breakdown's asset.")
			}
			in.AssetID = assetID
		}
		if in.AssetID != "" {
			branchID, x := s.repo.AssetBranch(ctx, tx, t.OrganizationID, in.AssetID)
			if x != nil || !t.BranchPermitted(branchID) {
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
			exists, x := s.repo.BranchExists(ctx, tx, t.OrganizationID, in.BranchID)
			if x != nil || !exists || !t.BranchPermitted(in.BranchID) {
				return nf()
			}
			branchID = in.BranchID
		} else if in.BranchID != "" && in.BranchID != branchID {
			return errorcodes.BadRequest("rescue_invalid", "branchId does not match the linked work.")
		}
		id, inserted, x := s.repo.InsertRescue(ctx, tx, t.OrganizationID, branchID, in, models.RescueSubjectKey(in), t.ActorID, models.Fingerprint(in))
		if errors.Is(x, models.ErrConflict) {
			return models.ErrConflict
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
				alert, x := s.createAlert(ctx, tx, t, models.AlertRequest{Kind: "RESCUE_REQUIRED", Severity: in.Severity, SourceType: "RESCUE_CASE", SourceID: id, BranchID: branchID, AssignmentID: in.PrivateAssignmentID, CanonicalTripReference: in.CanonicalTripReference, Condition: "rescue:" + id})
				if x != nil {
					return x
				}
				if x = s.repo.SetRescueAlert(ctx, tx, t.OrganizationID, id, alert.ID); x != nil {
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

func (s *Service) GetRescueCase(ctx context.Context, t tenant.Context, id string) (models.RescueCase, error) {
	if e := s.rescueGate(false); e != nil {
		return models.RescueCase{}, e
	}
	v, e := s.repo.GetRescue(ctx, nil, t.OrganizationID, id, false)
	if e != nil || !t.BranchPermitted(v.BranchID) {
		if e != nil && !errors.Is(e, models.ErrNotFound) {
			return models.RescueCase{}, e
		}
		return models.RescueCase{}, nf()
	}
	return v, nil
}

// ListRescueCases returns one page of rescue cases in the caller's branch
// scope and the cursor of the next page ("" on the last page).
func (s *Service) ListRescueCases(ctx context.Context, t tenant.Context, f models.RescueFilter) ([]models.RescueCase, string, error) {
	if e := s.rescueGate(false); e != nil {
		return nil, "", e
	}
	if f.BranchID != "" && !t.BranchPermitted(f.BranchID) {
		return []models.RescueCase{}, "", nil
	}
	f.Status, f.TriggerType = strings.ToUpper(f.Status), strings.ToUpper(f.TriggerType)
	size := models.BoundedLimit(f.Limit)
	items, e := s.repo.ListRescues(ctx, t.OrganizationID, f, size+1)
	if e != nil {
		return nil, "", e
	}
	items, next := models.TrimPage(items, size, func(v models.RescueCase) (time.Time, string) { return v.CreatedAt, v.ID })
	out := []models.RescueCase{}
	for _, v := range items {
		if t.BranchPermitted(v.BranchID) {
			out = append(out, v)
		}
	}
	return out, next, nil
}

// syncRescueReplacement projects the replacement assignment's provider
// decision onto the case. Accepted (or later) moves the case to
// REPLACEMENT_ACCEPTED; a decline, expiry or cancellation reopens it so the
// dispatcher can offer the work to someone else.
func (s *Service) syncRescueReplacement(ctx context.Context, tx pgx.Tx, t tenant.Context, v *models.RescueCase) error {
	if v.Status != models.RescueReplacementPending || v.ReplacementAssignmentID == "" {
		return nil
	}
	to := ""
	event := ""
	switch v.ReplacementAssignmentStatus {
	case privatemodels.StatusAccepted, privatemodels.StatusReady, privatemodels.StatusInProgress, privatemodels.StatusCompleted:
		to, event = models.RescueReplacementAccepted, "rescue_case.replacement_accepted.v1"
	case privatemodels.StatusDeclined, privatemodels.StatusExpired, privatemodels.StatusCancelled, privatemodels.StatusReassignment:
		to, event = models.RescueOpen, "rescue_case.replacement_declined.v1"
	default:
		return nil
	}
	from := v.Status
	version, updated, e := s.repo.SetRescueStatus(ctx, tx, v.OrganizationID, v.ID, v.Version, to)
	if e != nil {
		return e
	}
	v.Version, v.UpdatedAt = version, updated
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
func (s *Service) AssignRescueReplacement(ctx context.Context, t tenant.Context, id string, in models.RescueReplacementInput) (models.RescueCase, error) {
	if e := s.rescueGate(true); e != nil {
		return models.RescueCase{}, e
	}
	if e := models.RequireVersion(in.ExpectedVersion); e != nil {
		return models.RescueCase{}, e
	}
	if strings.TrimSpace(in.ProviderMembershipID) == "" || strings.TrimSpace(in.ProviderID) == "" {
		return models.RescueCase{}, errorcodes.BadRequest("rescue_invalid", "providerMembershipId and providerId are required.")
	}
	if s.o.Rescue == nil {
		return models.RescueCase{}, errorcodes.Unavailable("rescue_assignments_unavailable", "Replacement assignments are unavailable.")
	}
	var out models.RescueCase
	e := s.tx(ctx, func(tx pgx.Tx) error {
		v, x := s.lockRescue(ctx, tx, t, id)
		if x != nil {
			return x
		}
		if v.Version != in.ExpectedVersion {
			return models.ErrConflict
		}
		if x = s.syncRescueReplacement(ctx, tx, t, &v); x != nil {
			return x
		}
		if v.Status != models.RescueOpen {
			return errorcodes.Conflict("rescue_replacement_not_allowed", "A replacement can only be offered while the rescue case is open and no offer is pending.")
		}
		if v.CurrentAssignmentID == "" {
			return errorcodes.Conflict("rescue_not_linked_to_assignment", "A replacement provider can only be offered for a rescue case linked to a private assignment.")
		}
		current, x := s.repo.LockRescueAssignment(ctx, tx, t.OrganizationID, v.CurrentAssignmentID)
		if x != nil {
			return nf()
		}
		if current.ProviderMembershipID == in.ProviderMembershipID {
			return errorcodes.BadRequest("rescue_invalid", "The replacement provider must differ from the provider currently holding the work.")
		}
		if strings.TrimSpace(in.ExecutionService) == "" {
			in.ExecutionService = current.ExecutionService
		}
		var replacement privatemodels.Assignment
		switch current.Status {
		case privatemodels.StatusDeclined, privatemodels.StatusExpired:
			replacement, x = s.o.Rescue.ReassignInTransaction(ctx, tx, t, v.CurrentAssignmentID, privateusecases.ReassignInput{ProviderMembershipID: in.ProviderMembershipID, ProviderID: in.ProviderID, VehicleOrTruckReference: in.VehicleOrTruckReference, ExecutionService: in.ExecutionService, ExpectedVersion: current.Version, ResponseDeadline: in.ResponseDeadline})
		case privatemodels.StatusCancelled:
			replacement, x = s.o.Rescue.CreateInTransaction(ctx, tx, t, privateusecases.CreateInput{BranchID: current.BranchID, WorkOrigin: current.WorkOrigin, WorkID: current.WorkID, ProviderMembershipID: in.ProviderMembershipID, ProviderID: in.ProviderID, VehicleOrTruckReference: in.VehicleOrTruckReference, ExecutionService: in.ExecutionService, ResponseDeadline: in.ResponseDeadline})
		default:
			return errorcodes.Conflict("rescue_original_assignment_active", "The current assignment still holds the work. Cancel it, or wait for the provider's decline or expiry, before offering a replacement.")
		}
		if x != nil {
			return x
		}
		from := v.Status
		if x = s.repo.OfferRescueReplacement(ctx, tx, t.OrganizationID, id, v.Version, replacement.ID, in.ProviderMembershipID, in.VehicleOrTruckReference); x != nil {
			return models.ErrConflict
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

func (s *Service) ResolveRescueCase(ctx context.Context, t tenant.Context, id string, in models.ResolveRescueCaseInput) (models.RescueCase, error) {
	if e := s.rescueGate(true); e != nil {
		return models.RescueCase{}, e
	}
	if e := models.RequireVersion(in.ExpectedVersion); e != nil {
		return models.RescueCase{}, e
	}
	in.ResolutionCode = strings.ToUpper(strings.TrimSpace(in.ResolutionCode))
	in.Notes = strings.TrimSpace(in.Notes)
	if !models.ValidRescueResolution(in.ResolutionCode) || in.Notes == "" {
		return models.RescueCase{}, errorcodes.BadRequest("rescue_invalid", "A valid resolutionCode and notes are required.")
	}
	var out models.RescueCase
	e := s.tx(ctx, func(tx pgx.Tx) error {
		v, x := s.lockRescue(ctx, tx, t, id)
		if x != nil {
			return x
		}
		if v.Version != in.ExpectedVersion {
			return models.ErrConflict
		}
		if x = s.syncRescueReplacement(ctx, tx, t, &v); x != nil {
			return x
		}
		switch v.Status {
		case models.RescueResolved, models.RescueCancelled:
			return errorcodes.Conflict("rescue_case_closed", "The rescue case is already closed.")
		case models.RescueReplacementPending:
			return errorcodes.Conflict("rescue_replacement_pending", "The replacement provider has not responded; the case cannot be resolved yet.")
		}
		if in.ResolutionCode == "REPLACEMENT_COMPLETED" && v.Status != models.RescueReplacementAccepted {
			return errorcodes.Conflict("rescue_replacement_not_accepted", "REPLACEMENT_COMPLETED requires a replacement the provider accepted.")
		}
		resolution, _ := json.Marshal(map[string]any{"code": in.ResolutionCode, "notes": in.Notes, "replacementAssignmentId": v.ReplacementAssignmentID})
		from := v.Status
		if x = s.repo.ResolveRescue(ctx, tx, t.OrganizationID, id, v.Version, in.ResolutionCode, resolution, t.ActorID); x != nil {
			return models.ErrConflict
		}
		if x = s.closeRescueAlert(ctx, tx, t, v.AlertID, models.AlertResolved, "RESCUE_CASE_RESOLVED", in.Notes); x != nil {
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

func (s *Service) CancelRescueCase(ctx context.Context, t tenant.Context, id string, in models.CancelRescueCaseInput) (models.RescueCase, error) {
	if e := s.rescueGate(true); e != nil {
		return models.RescueCase{}, e
	}
	if e := models.RequireVersion(in.ExpectedVersion); e != nil {
		return models.RescueCase{}, e
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Reason == "" {
		return models.RescueCase{}, errorcodes.BadRequest("rescue_invalid", "reason is required.")
	}
	var out models.RescueCase
	e := s.tx(ctx, func(tx pgx.Tx) error {
		v, x := s.lockRescue(ctx, tx, t, id)
		if x != nil {
			return x
		}
		if v.Version != in.ExpectedVersion {
			return models.ErrConflict
		}
		if x = s.syncRescueReplacement(ctx, tx, t, &v); x != nil {
			return x
		}
		if v.Status != models.RescueOpen {
			return errorcodes.Conflict("rescue_cancel_not_allowed", "Only an open rescue case without a pending or accepted replacement can be cancelled.")
		}
		resolution, _ := json.Marshal(map[string]any{"cancelReason": in.Reason})
		if x = s.repo.CancelRescue(ctx, tx, t.OrganizationID, id, v.Version, resolution); x != nil {
			return models.ErrConflict
		}
		if x = s.closeRescueAlert(ctx, tx, t, v.AlertID, models.AlertDismissed, "RESCUE_CASE_CANCELLED", in.Reason); x != nil {
			return x
		}
		out, x = s.lockRescue(ctx, tx, t, id)
		if x != nil {
			return x
		}
		if x = s.rescueHistory(ctx, tx, t, out, "CANCELLED", models.RescueOpen, out.Status, in.Reason); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "rescue_case.cancelled.v1", "rescue_case", out.ID, out.Version, map[string]any{"caseId": out.ID, "reason": in.Reason})
	})
	return out, e
}

// closeRescueAlert closes the alert raised for a case once the case itself is
// closed: the alert's source condition no longer exists.
func (s *Service) closeRescueAlert(ctx context.Context, tx pgx.Tx, t tenant.Context, alertID, to, code, reason string) error {
	if alertID == "" {
		return nil
	}
	from, x := s.repo.LockAlertStatus(ctx, tx, t.OrganizationID, alertID)
	if errors.Is(x, models.ErrNotFound) || from == models.AlertResolved || from == models.AlertDismissed {
		return nil
	}
	if x != nil {
		return x
	}
	version, x := s.repo.CloseAlert(ctx, tx, t.OrganizationID, alertID, to, code, reason)
	if x != nil {
		return x
	}
	if x = s.repo.AlertHistory(ctx, tx, t.OrganizationID, alertID, from, to, t.ActorID, reason, version); x != nil {
		return x
	}
	return s.record(ctx, tx, t, "operational_alert."+strings.ToLower(to)+".v1", "operational_alert", alertID, version, map[string]any{"alertId": alertID, "status": to, "resolutionCode": code})
}

// SyncRescueReplacements is the durable sweep that projects provider
// responses onto rescue cases even when no dispatcher touches the case.
func (s *Service) SyncRescueReplacements(ctx context.Context, tx pgx.Tx, limit int) error {
	due, e := s.repo.RescuesAwaitingSync(ctx, tx, limit)
	if e != nil {
		return e
	}
	for _, r := range due {
		t := tenant.Context{OrganizationID: r.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "rescue-replacement-sync", CorrelationID: r.ID}
		v, x := s.lockRescue(ctx, tx, t, r.ID)
		if x != nil {
			return x
		}
		if x = s.syncRescueReplacement(ctx, tx, t, &v); x != nil {
			return x
		}
	}
	return nil
}
