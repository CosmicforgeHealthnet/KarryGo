package usecases

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/assets/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// InsertDefectTx records a defect inside the caller's transaction. System
// sources (inspection items, safety declines, tyre readings) are deduplicated
// per source and item; inserted reports false for a duplicate.
func (s *Service) InsertDefectTx(ctx context.Context, tx pgx.Tx, t tenant.Context, branchID string, in models.ReportDefectInput, workOrderID string) (models.Defect, bool, error) {
	v, e := s.repo.InsertDefect(ctx, tx, t.OrganizationID, branchID, in, workOrderID, t.ActorID)
	if errors.Is(e, pgx.ErrNoRows) {
		return models.Defect{}, false, nil
	}
	if e != nil {
		return models.Defect{}, false, e
	}
	if e = s.repo.InsertDefectHistory(ctx, tx, t.OrganizationID, v.ID, "", v.Status, t.ActorID, "REPORTED", v.Version); e != nil {
		return v, true, e
	}
	return v, true, s.record(ctx, tx, t, "defect.reported.v1", "defect", v.ID, v.Version, map[string]any{"defectId": v.ID, "assetId": v.AssetID, "severity": v.Severity, "critical": v.Critical, "sourceType": v.SourceType, "sourceReference": v.SourceReference})
}

// blockAssetForDefectTx applies the operational restriction a critical defect
// requires: a repair work order, a safety block and BLOCKED status, all in the
// caller's transaction.
func (s *Service) blockAssetForDefectTx(ctx context.Context, tx pgx.Tx, t tenant.Context, d *models.Defect) error {
	workID, workVersion, x := s.c.WorkOrders.CreateSystemWorkOrderTx(ctx, tx, t.OrganizationID, d.AssetID, "DEFECT", d.ID, "CRITICAL", "Repair critical defect: "+d.Description)
	if x != nil {
		return x
	}
	if x := s.repo.LinkDefectWorkOrder(ctx, tx, t.OrganizationID, d, workID); x != nil {
		return x
	}
	d.WorkOrderID = workID
	if x := s.record(ctx, tx, t, "work_order.created.v1", "work_order", workID, workVersion, map[string]any{"workOrderId": workID, "assetId": d.AssetID, "defectId": d.ID}); x != nil {
		return x
	}
	if x := s.repo.InsertSafetyBlock(ctx, tx, t.OrganizationID, d.AssetID, "DEFECT", d.ID, "Critical defect reported", t.ActorID, false); x != nil {
		return x
	}
	previous, lifecycle, x := s.repo.AssetStatus(ctx, tx, t.OrganizationID, d.AssetID)
	if x != nil {
		return x
	}
	if lifecycle != "ACTIVE" || previous == "BLOCKED" {
		return nil
	}
	assetVersion, x := s.repo.BlockAsset(ctx, tx, t.OrganizationID, d.AssetID)
	if x != nil {
		return x
	}
	if x := s.assetHistory(ctx, tx, t, d.AssetID, previous, "BLOCKED", lifecycle, "CRITICAL_DEFECT", assetVersion, map[string]any{"defectId": d.ID}); x != nil {
		return x
	}
	return s.record(ctx, tx, t, "asset.blocked.v1", "asset", d.AssetID, assetVersion, map[string]any{"assetId": d.AssetID, "defectId": d.ID, "operationalStatus": "BLOCKED"})
}

// ReportDefect records a manually or provider-reported defect. Inspection,
// safety-decline and tyre-reading defects are created only by those flows.
func (s *Service) ReportDefect(ctx context.Context, t tenant.Context, in models.ReportDefectInput) (models.Defect, error) {
	if e := s.gate(s.o.Flags.Maintenance, true); e != nil {
		return models.Defect{}, e
	}
	in.SourceType = strings.ToUpper(strings.TrimSpace(in.SourceType))
	if in.SourceType == "" {
		in.SourceType = "MANUAL_REPORT"
	}
	in.Severity = strings.ToUpper(strings.TrimSpace(in.Severity))
	in.Description = strings.TrimSpace(in.Description)
	if (in.SourceType != "MANUAL_REPORT" && in.SourceType != "PROVIDER_REPORT") || !models.DefectSeverities[in.Severity] || in.Description == "" || len(in.Description) > 2000 {
		return models.Defect{}, apperrors.BadRequest("sourceType, severity or description is invalid.", nil)
	}
	a, e := s.GetAsset(ctx, t, in.AssetID)
	if e != nil {
		return models.Defect{}, e
	}
	var out models.Defect
	e = s.tx(ctx, func(tx pgx.Tx) error {
		if x := s.repo.LockAssetRow(ctx, tx, t.OrganizationID, in.AssetID); x != nil {
			return x
		}
		var x error
		out, _, x = s.InsertDefectTx(ctx, tx, t, a.BranchID, in, "")
		if x != nil {
			return x
		}
		if out.Critical {
			return s.blockAssetForDefectTx(ctx, tx, t, &out)
		}
		return nil
	})
	if e == nil && out.Critical {
		s.alert(ctx, t.OrganizationID, "CRITICAL_DEFECT", in.AssetID, out.ID, "CRITICAL", "Critical defect reported; asset blocked")
	}
	return out, e
}

func (s *Service) GetDefect(ctx context.Context, t tenant.Context, id string) (models.Defect, error) {
	if e := s.gate(s.o.Flags.Maintenance, false); e != nil {
		return models.Defect{}, e
	}
	v, e := s.repo.GetDefect(ctx, nil, t.OrganizationID, id, false)
	if e != nil || !t.BranchPermitted(v.BranchID) {
		return models.Defect{}, apperrors.NotFound("Resource not found.", nil)
	}
	return v, nil
}

func (s *Service) ListDefects(ctx context.Context, t tenant.Context, f models.DefectFilter) ([]models.Defect, error) {
	if e := s.gate(s.o.Flags.Maintenance, false); e != nil {
		return nil, e
	}
	rows, e := s.repo.ListDefects(ctx, t.OrganizationID, f)
	if e != nil {
		return nil, e
	}
	out := []models.Defect{}
	for _, v := range rows {
		if t.BranchPermitted(v.BranchID) {
			out = append(out, v)
		}
	}
	return out, nil
}

// DefectTransition moves a defect through DEFER, START_REPAIR or RESOLVE.
func (s *Service) DefectTransition(ctx context.Context, t tenant.Context, id, action string, in models.DefectAction) (models.Defect, error) {
	if e := s.gate(s.o.Flags.Maintenance, true); e != nil {
		return models.Defect{}, e
	}
	if in.ExpectedVersion <= 0 {
		return models.Defect{}, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	if _, e := s.GetDefect(ctx, t, id); e != nil {
		return models.Defect{}, e
	}
	var out models.Defect
	e := s.tx(ctx, func(tx pgx.Tx) error {
		cur, x := s.repo.GetDefect(ctx, tx, t.OrganizationID, id, true)
		if x != nil {
			return x
		}
		if cur.Version != in.ExpectedVersion {
			return apperrors.Conflict("Defect version conflict.", nil)
		}
		to, event, reason := "", "", ""
		workOrder := cur.WorkOrderID
		switch action {
		case "DEFER":
			if cur.Critical {
				return apperrors.Conflict("A critical defect cannot be deferred.", nil)
			}
			if cur.Status != models.DefectOpen {
				return apperrors.Conflict("Only an open defect can be deferred.", nil)
			}
			reason = strings.TrimSpace(in.Reason)
			if reason == "" || in.DeferredUntil == nil || !in.DeferredUntil.After(s.o.Now()) {
				return apperrors.BadRequest("reason and a future deferredUntil are required.", nil)
			}
			to, event = models.DefectDeferred, "defect.deferred.v1"
		case "START_REPAIR":
			if cur.Status != models.DefectOpen && cur.Status != models.DefectDeferred {
				return apperrors.Conflict("Only an open or deferred defect can enter repair.", nil)
			}
			if in.WorkOrderID != "" {
				ok, x := s.repo.WorkOrderUsable(ctx, tx, t.OrganizationID, in.WorkOrderID, cur.AssetID)
				if x != nil {
					return x
				}
				if !ok {
					return apperrors.NotFound("Work order not found for this asset.", nil)
				}
				workOrder = in.WorkOrderID
			}
			if workOrder == "" {
				return apperrors.BadRequest("A repair work order is required.", nil)
			}
			reason = strings.TrimSpace(in.Reason)
			to, event = models.DefectRepairing, "defect.repair_started.v1"
		case "RESOLVE":
			if cur.Status == models.DefectResolved {
				return apperrors.Conflict("Defect is already resolved.", nil)
			}
			reason = strings.TrimSpace(in.Notes)
			if reason == "" {
				return apperrors.BadRequest("Resolution notes are required.", nil)
			}
			if cur.Critical {
				completed := false
				if workOrder != "" {
					if completed, x = s.repo.WorkOrderCompleted(ctx, tx, t.OrganizationID, workOrder); x != nil {
						return x
					}
				}
				if !completed {
					return apperrors.Conflict("A critical defect can only be resolved after its repair work order is completed.", nil)
				}
			}
			to, event = models.DefectResolved, "defect.resolved.v1"
		default:
			return apperrors.NotFound("Operation not found.", nil)
		}
		out, x = s.repo.TransitionDefect(ctx, tx, t.OrganizationID, id, cur.Version, to, workOrder, in, reason, t.ActorID)
		if x != nil {
			return apperrors.Conflict("Defect version conflict.", x)
		}
		if x = s.repo.InsertDefectHistory(ctx, tx, t.OrganizationID, id, cur.Status, to, t.ActorID, reason, out.Version); x != nil {
			return x
		}
		return s.record(ctx, tx, t, event, "defect", id, out.Version, map[string]any{"defectId": id, "assetId": out.AssetID, "status": to, "critical": out.Critical, "assetReleased": false})
	})
	return out, e
}

// ---------------------------------------------------------------------------
// Safety primitives composed by inspections and maintenance
// ---------------------------------------------------------------------------

// LockAssetTx locks the asset row and returns its safety state, serializing
// every writer of that state.
func (s *Service) LockAssetTx(ctx context.Context, tx pgx.Tx, org, id string) (models.AssetLock, error) {
	return s.repo.LockAsset(ctx, tx, org, id)
}

// BlockAssetTx sets the asset BLOCKED and records the safety block, returning
// the asset's new version.
func (s *Service) BlockAssetTx(ctx context.Context, tx pgx.Tx, org, assetID, sourceType, sourceRef, reason, createdBy string) (int64, error) {
	version, e := s.repo.BlockAsset(ctx, tx, org, assetID)
	if e != nil {
		return 0, e
	}
	return version, s.repo.InsertSafetyBlock(ctx, tx, org, assetID, sourceType, sourceRef, reason, createdBy, false)
}

// BlockActiveAssetTx blocks an active asset that is not yet blocked and
// records a deduplicated safety block. version is 0 when the asset did not
// change.
func (s *Service) BlockActiveAssetTx(ctx context.Context, tx pgx.Tx, org, assetID, sourceType, sourceRef, reason, createdBy string) (int64, error) {
	version, e := s.repo.BlockActiveAsset(ctx, tx, org, assetID)
	if e != nil && e != pgx.ErrNoRows {
		return 0, e
	}
	return version, s.repo.InsertSafetyBlock(ctx, tx, org, assetID, sourceType, sourceRef, reason, createdBy, true)
}

// InsertSafetyBlockTx records a deduplicated ACTIVE safety block.
func (s *Service) InsertSafetyBlockTx(ctx context.Context, tx pgx.Tx, org, assetID, sourceType, sourceRef, reason, createdBy string) error {
	return s.repo.InsertSafetyBlock(ctx, tx, org, assetID, sourceType, sourceRef, reason, createdBy, true)
}

// BlockAssetForDeclineTx blocks an asset reported unsafe unless it is already
// blocked or out of use.
func (s *Service) BlockAssetForDeclineTx(ctx context.Context, tx pgx.Tx, org, assetID string) error {
	return s.repo.BlockAssetForDecline(ctx, tx, org, assetID)
}

// ResolveAssetTx resolves an asset by id or platform vehicle id; found is
// false when the vehicle is not a registered asset.
func (s *Service) ResolveAssetTx(ctx context.Context, tx pgx.Tx, org, reference string) (id, branch string, found bool, e error) {
	id, branch, e = s.repo.ResolveAssetID(ctx, tx, org, reference)
	if e == pgx.ErrNoRows {
		return "", "", false, nil
	}
	return id, branch, e == nil, e
}

// RequireSafetyBlocksCoveredTx enforces that every safety block currently in
// force on the asset (created after the last authorized clearance) has its
// own repair work order completed. Blocks and their work orders share the
// same (source_type, source_reference), so an unrelated work order can never
// be used to release an asset.
func (s *Service) RequireSafetyBlocksCoveredTx(ctx context.Context, tx pgx.Tx, org, assetID string) error {
	uncovered, x := s.repo.UncoveredSafetyBlocks(ctx, tx, org, assetID)
	if x != nil {
		return x
	}
	if uncovered > 0 {
		return apperrors.Conflict("Every active safety block requires its own completed work order before return to service.", nil)
	}
	return nil
}

// ResolveRepairedCriticalDefectsTx is the return-to-service side of the
// defect lifecycle: it refuses while any critical defect lacks a completed
// repair, then resolves the critical defects the authorized decision covers.
func (s *Service) ResolveRepairedCriticalDefectsTx(ctx context.Context, tx pgx.Tx, t tenant.Context, assetID, workOrderID string) error {
	unrepaired, x := s.repo.UnrepairedCriticalDefects(ctx, tx, t.OrganizationID, assetID)
	if x != nil {
		return x
	}
	if unrepaired > 0 {
		return apperrors.Conflict("Every critical defect on the asset requires a completed repair work order before return to service.", nil)
	}
	items, x := s.repo.ResolveCriticalDefects(ctx, tx, t.OrganizationID, assetID, "Released by authorized return to service of work order "+workOrderID, t.ActorID)
	if x != nil {
		return x
	}
	for _, r := range items {
		if x = s.repo.InsertDefectHistory(ctx, tx, t.OrganizationID, r.ID, "UNRESOLVED", "RESOLVED", t.ActorID, "RETURN_TO_SERVICE", r.Version); x != nil {
			return x
		}
		if x = s.record(ctx, tx, t, "defect.resolved.v1", "defect", r.ID, r.Version, map[string]any{"defectId": r.ID, "assetId": assetID, "workOrderId": workOrderID, "returnToService": true}); x != nil {
			return x
		}
	}
	return nil
}

// ReleaseSafetyBlockTx returns a BLOCKED asset to service and records the
// authorized clearance, returning the asset's new version.
func (s *Service) ReleaseSafetyBlockTx(ctx context.Context, tx pgx.Tx, t tenant.Context, assetID, workOrderID, reason string) (int64, error) {
	return s.repo.ReleaseBlock(ctx, tx, t.OrganizationID, assetID, workOrderID, t.ActorID, reason)
}

// ---------------------------------------------------------------------------
// Tyres (spec 48.3 GET/POST /tyres)
// ---------------------------------------------------------------------------

func (s *Service) RegisterTyre(ctx context.Context, t tenant.Context, in models.RegisterTyreInput) (models.Tyre, error) {
	if e := s.gate(s.o.Flags.Maintenance, true); e != nil {
		return models.Tyre{}, e
	}
	in.SerialNumber = strings.TrimSpace(in.SerialNumber)
	if in.SerialNumber == "" || len(in.SerialNumber) > 120 || in.BranchID == "" {
		return models.Tyre{}, apperrors.BadRequest("branchId and serialNumber are required.", nil)
	}
	if !t.BranchPermitted(in.BranchID) {
		return models.Tyre{}, apperrors.NotFound("Resource not found.", nil)
	}
	min := 1.6
	if in.MinTreadDepthMM != nil {
		min = *in.MinTreadDepthMM
	}
	if min < 0 || in.TreadDepthMM != nil && *in.TreadDepthMM < 0 {
		return models.Tyre{}, apperrors.BadRequest("Tread depths cannot be negative.", nil)
	}
	var out models.Tyre
	e := s.tx(ctx, func(tx pgx.Tx) error {
		if exists, x := s.repo.BranchExists(ctx, tx, t.OrganizationID, in.BranchID); x != nil || !exists {
			return apperrors.NotFound("Resource not found.", x)
		}
		var x error
		out, x = s.repo.InsertTyre(ctx, tx, t.OrganizationID, in, min)
		if uniqueViolation(x) {
			return apperrors.Conflict("A tyre with this serial number already exists.", nil)
		}
		if x != nil {
			return x
		}
		if _, x = s.repo.InsertTyreEvent(ctx, tx, t.OrganizationID, out.ID, "REGISTERED", "", "", "", "", t.ActorID, in.TreadDepthMM, nil); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "tyre.registered.v1", "tyre", out.ID, out.Version, out)
	})
	return out, e
}

func (s *Service) GetTyre(ctx context.Context, t tenant.Context, id string) (models.Tyre, error) {
	if e := s.gate(s.o.Flags.Maintenance, false); e != nil {
		return models.Tyre{}, e
	}
	v, e := s.repo.GetTyre(ctx, nil, t.OrganizationID, id, false)
	if e != nil || !t.BranchPermitted(v.BranchID) {
		return models.Tyre{}, apperrors.NotFound("Resource not found.", nil)
	}
	return v, nil
}

func (s *Service) ListTyres(ctx context.Context, t tenant.Context, assetID, status string) ([]models.Tyre, error) {
	if e := s.gate(s.o.Flags.Maintenance, false); e != nil {
		return nil, e
	}
	rows, e := s.repo.ListTyres(ctx, t.OrganizationID, assetID, status)
	if e != nil {
		return nil, e
	}
	out := []models.Tyre{}
	for _, v := range rows {
		if t.BranchPermitted(v.BranchID) {
			out = append(out, v)
		}
	}
	return out, nil
}

// TyreTransition applies INSTALL, REMOVE, ROTATE or TREAD to a tyre. At most
// one installed tyre may occupy an asset position; a tread reading under the
// tyre's minimum on an installed tyre opens a HIGH defect for review.
func (s *Service) TyreTransition(ctx context.Context, t tenant.Context, id, action string, in models.TyreAction) (models.Tyre, error) {
	if e := s.gate(s.o.Flags.Maintenance, true); e != nil {
		return models.Tyre{}, e
	}
	if in.ExpectedVersion <= 0 {
		return models.Tyre{}, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	if _, e := s.GetTyre(ctx, t, id); e != nil {
		return models.Tyre{}, e
	}
	var out models.Tyre
	e := s.tx(ctx, func(tx pgx.Tx) error {
		cur, x := s.repo.GetTyre(ctx, tx, t.OrganizationID, id, true)
		if x != nil {
			return x
		}
		if cur.Version != in.ExpectedVersion {
			return apperrors.Conflict("Tyre version conflict.", nil)
		}
		status, assetID, position, event, kind := cur.Status, cur.AssetID, cur.Position, "", ""
		depth := cur.LastTreadDepthMM
		switch action {
		case "INSTALL":
			if cur.Status != "IN_STOCK" && cur.Status != "REMOVED" {
				return apperrors.Conflict("Only a stocked or removed tyre can be installed.", nil)
			}
			in.Position = strings.ToUpper(strings.TrimSpace(in.Position))
			if in.AssetID == "" || in.Position == "" {
				return apperrors.BadRequest("assetId and position are required.", nil)
			}
			lifecycle, assetBranch, x := s.repo.AssetLifecycleAndBranch(ctx, tx, t.OrganizationID, in.AssetID)
			if x != nil || !t.BranchPermitted(assetBranch) {
				return apperrors.NotFound("Asset not found.", nil)
			}
			if lifecycle != "ACTIVE" {
				return apperrors.Conflict("Tyres cannot be installed on an archived or retired asset.", nil)
			}
			status, assetID, position, event, kind = "INSTALLED", in.AssetID, in.Position, "tyre.installed.v1", "INSTALLED"
		case "REMOVE":
			if cur.Status != "INSTALLED" {
				return apperrors.Conflict("Only an installed tyre can be removed.", nil)
			}
			if strings.TrimSpace(in.Reason) == "" {
				return apperrors.BadRequest("reason is required.", nil)
			}
			status, assetID, position, event, kind = "REMOVED", "", "", "tyre.removed.v1", "REMOVED"
			if in.Scrap {
				status, event, kind = "SCRAPPED", "tyre.scrapped.v1", "SCRAPPED"
			}
		case "ROTATE":
			if cur.Status != "INSTALLED" {
				return apperrors.Conflict("Only an installed tyre can be rotated.", nil)
			}
			in.ToPosition = strings.ToUpper(strings.TrimSpace(in.ToPosition))
			if in.ToPosition == "" || in.ToPosition == cur.Position {
				return apperrors.BadRequest("A different toPosition is required.", nil)
			}
			position, event, kind = in.ToPosition, "tyre.rotated.v1", "ROTATED"
		case "TREAD":
			if cur.Status == "SCRAPPED" {
				return apperrors.Conflict("A scrapped tyre cannot be measured.", nil)
			}
			if in.TreadDepthMM == nil || *in.TreadDepthMM < 0 {
				return apperrors.BadRequest("treadDepthMm is required and cannot be negative.", nil)
			}
			depth, event, kind = in.TreadDepthMM, "tyre.tread_recorded.v1", "TREAD_READING"
		default:
			return apperrors.NotFound("Operation not found.", nil)
		}
		out, x = s.repo.UpdateTyre(ctx, tx, t.OrganizationID, id, cur.Version, status, assetID, position, depth)
		if uniqueViolation(x) {
			return apperrors.Conflict("Another tyre is already installed at that position.", nil)
		}
		if x != nil {
			return apperrors.Conflict("Tyre version conflict.", x)
		}
		eventAsset := assetID
		if eventAsset == "" {
			eventAsset = cur.AssetID
		}
		tyreEventID, x := s.repo.InsertTyreEvent(ctx, tx, t.OrganizationID, id, kind, eventAsset, cur.Position, position, strings.TrimSpace(in.Reason), t.ActorID, in.TreadDepthMM, in.Odometer)
		if x != nil {
			return x
		}
		if x = s.record(ctx, tx, t, event, "tyre", id, out.Version, map[string]any{"tyre": out, "fromPosition": cur.Position}); x != nil {
			return x
		}
		if action == "TREAD" && out.Status == "INSTALLED" && *in.TreadDepthMM < out.MinTreadDepthMM {
			open, x := s.repo.OpenTyreDefectExists(ctx, tx, t.OrganizationID, "TYRE:"+id)
			if x != nil {
				return x
			}
			if !open {
				d, _, x := s.InsertDefectTx(ctx, tx, t, out.BranchID, models.ReportDefectInput{AssetID: out.AssetID, SourceType: "TYRE_READING", SourceReference: tyreEventID, ItemCode: "TYRE:" + id, Description: "Tyre " + out.SerialNumber + " at " + out.Position + " is below minimum tread depth", Severity: "HIGH"}, "")
				if x != nil {
					return x
				}
				out.LowTreadDefectID = d.ID
			}
		}
		return nil
	})
	return out, e
}
