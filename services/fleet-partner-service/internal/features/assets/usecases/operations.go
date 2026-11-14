package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/assets/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

func (s *Service) SaveBreakdown(ctx context.Context, t tenant.Context, id string, expected int64, v models.Breakdown) (models.Breakdown, error) {
	if e := s.gate(s.o.Flags.Breakdowns, true); e != nil {
		return v, e
	}
	if v.ReplacementAssetID != "" {
		// The replacement must be an asset this caller can see; the foreign
		// key alone would accept another organization's asset.
		if _, e := s.GetAsset(ctx, t, v.ReplacementAssetID); e != nil {
			return v, e
		}
	}
	if id == "" {
		if _, e := s.GetAsset(ctx, t, v.AssetID); e != nil {
			return v, e
		}
		v.ID = ids.New("brk")
		v.OrganizationID = t.OrganizationID
		if v.OccurredAt.IsZero() {
			v.OccurredAt = s.o.Now()
		}
		e := s.tx(ctx, func(tx pgx.Tx) error {
			if e := s.repo.InsertBreakdown(ctx, tx, &v); e != nil {
				return e
			}
			priority := "HIGH"
			if v.Severity == "CRITICAL" {
				priority = "CRITICAL"
			}
			if _, _, e := s.c.WorkOrders.EnsureSystemWorkOrderTx(ctx, tx, t.OrganizationID, v.AssetID, "BREAKDOWN", v.ID, priority, "Diagnose and resolve breakdown"); e != nil {
				return e
			}
			if v.Severity == "HIGH" || v.Severity == "CRITICAL" {
				if e := s.repo.MarkOutOfService(ctx, tx, t.OrganizationID, v.AssetID); e != nil {
					return e
				}
			}
			return s.record(ctx, tx, t, "breakdown.created.v1", "breakdown", v.ID, v.Version, map[string]any{"breakdown": v, "automaticProviderPenalty": false, "canonicalTripMutation": false})
		})
		if e == nil {
			s.alert(ctx, t.OrganizationID, "ASSET_BREAKDOWN", v.AssetID, v.ID, v.Severity, "Asset breakdown requires rescue, towing or replacement review")
		}
		return v, e
	}
	old, e := s.GetBreakdown(ctx, t, id)
	if e != nil {
		return v, e
	}
	if expected <= 0 {
		return v, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		if e := s.repo.UpdateBreakdown(ctx, tx, t.OrganizationID, id, expected, &v); e != nil {
			return apperrors.Conflict("Breakdown version conflict.", e)
		}
		v.ID = id
		ev := "breakdown.updated.v1"
		if v.Status == "RESOLVED" && old.Status != "RESOLVED" {
			ev = "breakdown.resolved.v1"
		}
		return s.record(ctx, tx, t, ev, "breakdown", id, v.Version, map[string]any{"breakdown": v, "automaticProviderPenalty": false, "canonicalTripMutation": false})
	})
	return v, e
}
func (s *Service) GetBreakdown(ctx context.Context, t tenant.Context, id string) (models.Breakdown, error) {
	if e := s.gate(s.o.Flags.Breakdowns, false); e != nil {
		return models.Breakdown{}, e
	}
	v, e := s.repo.GetBreakdown(ctx, t.OrganizationID, id)
	if errors.Is(e, pgx.ErrNoRows) {
		return v, apperrors.NotFound("Resource not found.", nil)
	}
	if e == nil {
		_, e = s.GetAsset(ctx, t, v.AssetID)
	}
	return v, e
}
func (s *Service) ListBreakdowns(ctx context.Context, t tenant.Context) ([]models.Breakdown, error) {
	if e := s.gate(s.o.Flags.Breakdowns, false); e != nil {
		return nil, e
	}
	idsList, e := s.repo.ListBreakdownIDs(ctx, t.OrganizationID)
	if e != nil {
		return nil, e
	}
	out := []models.Breakdown{}
	for _, id := range idsList {
		v, e := s.GetBreakdown(ctx, t, id)
		if e != nil {
			continue
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *Service) SavePart(ctx context.Context, t tenant.Context, id string, expected int64, v models.Part) (models.Part, error) {
	if e := s.gate(s.o.Flags.Inventory, true); e != nil {
		return v, e
	}
	if id == "" {
		if !t.BranchPermitted(v.BranchID) {
			return v, apperrors.NotFound("Resource not found.", nil)
		}
		v.ID = ids.New("prt")
		v.OrganizationID = t.OrganizationID
		e := s.tx(ctx, func(tx pgx.Tx) error {
			if x := s.repo.InsertPart(ctx, tx, &v); x != nil {
				return x
			}
			return s.record(ctx, tx, t, "part.created.v1", "part", v.ID, v.Version, v)
		})
		return v, e
	}
	if _, e := s.GetPart(ctx, t, id); e != nil {
		return v, e
	}
	if expected <= 0 {
		return v, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	e := s.tx(ctx, func(tx pgx.Tx) error {
		x := s.repo.UpdatePart(ctx, tx, t.OrganizationID, id, expected, &v)
		if errors.Is(x, pgx.ErrNoRows) {
			return apperrors.Conflict("Part version conflict.", nil)
		}
		if x != nil {
			return x
		}
		v.ID = id
		return s.record(ctx, tx, t, "part.updated.v1", "part", id, v.Version, v)
	})
	return v, e
}
func (s *Service) GetPart(ctx context.Context, t tenant.Context, id string) (models.Part, error) {
	if e := s.gate(s.o.Flags.Inventory, false); e != nil {
		return models.Part{}, e
	}
	v, e := s.repo.GetPart(ctx, t.OrganizationID, id)
	if errors.Is(e, pgx.ErrNoRows) {
		return v, apperrors.NotFound("Resource not found.", nil)
	}
	if e == nil && !t.BranchPermitted(v.BranchID) {
		return v, apperrors.NotFound("Resource not found.", nil)
	}
	return v, e
}
func (s *Service) ListParts(ctx context.Context, t tenant.Context) ([]models.Part, error) {
	if e := s.gate(s.o.Flags.Inventory, false); e != nil {
		return nil, e
	}
	idsList, e := s.repo.ListPartIDs(ctx, t.OrganizationID)
	if e != nil {
		return nil, e
	}
	out := []models.Part{}
	for _, id := range idsList {
		v, e := s.GetPart(ctx, t, id)
		if e != nil {
			continue
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Service) CreateMovement(ctx context.Context, t tenant.Context, v models.StockMovement) (models.StockMovement, error) {
	if e := s.gate(s.o.Flags.Inventory, true); e != nil {
		return v, e
	}
	if v.Quantity <= 0 || v.Reason == "" {
		return v, apperrors.BadRequest("Positive quantity and reason are required.", nil)
	}
	if _, e := s.GetPart(ctx, t, v.PartID); e != nil {
		return v, e
	}
	if v.WorkOrderID != "" {
		if e := s.c.WorkOrders.WorkOrderVisible(ctx, t, v.WorkOrderID); e != nil {
			return v, e
		}
	}
	v.ID = ids.New("stm")
	v.OrganizationID = t.OrganizationID
	v.CreatedBy = t.ActorID
	if v.CorrelationID == "" {
		v.CorrelationID = t.CorrelationID
	}
	sign := models.StockSign(v.MovementType)
	reorderRequired := false
	e := s.tx(ctx, func(tx pgx.Tx) error {
		q, reorder, x := s.repo.LockPartStock(ctx, tx, t.OrganizationID, v.PartID)
		if x != nil {
			return x
		}
		projected := q + sign*v.Quantity
		if projected < 0 {
			return apperrors.Conflict("Stock cannot become negative.", nil)
		}
		partVersion, x := s.repo.SetPartProjection(ctx, tx, t.OrganizationID, v.PartID, projected)
		if x != nil {
			return x
		}
		if x = s.repo.InsertStockMovement(ctx, tx, &v); x != nil {
			return x
		}
		if x = s.record(ctx, tx, t, "stock_movement.recorded.v1", "stock_movement", v.ID, 1, v); x != nil {
			return x
		}
		reorderRequired = q > reorder && projected <= reorder
		if reorderRequired {
			return s.record(ctx, tx, t, "part.reorder_required.v1", "part", v.PartID, partVersion, map[string]any{"partId": v.PartID, "stockMovementId": v.ID, "projectedQuantity": projected, "reorderLevel": reorder})
		}
		return nil
	})
	if e == nil && reorderRequired {
		s.alert(ctx, t.OrganizationID, "PART_REORDER_REQUIRED", v.PartID, v.ID, "MEDIUM", "Part stock is at or below reorder level")
	}
	return v, e
}
func (s *Service) ListMovements(ctx context.Context, t tenant.Context) ([]models.StockMovement, error) {
	if e := s.gate(s.o.Flags.Inventory, false); e != nil {
		return nil, e
	}
	rows, e := s.repo.ListStockMovements(ctx, t.OrganizationID)
	if e != nil {
		return nil, e
	}
	out := []models.StockMovement{}
	for _, v := range rows {
		if _, e = s.GetPart(ctx, t, v.PartID); e != nil {
			continue
		}
		out = append(out, v)
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Section 18 maintenance job
// ---------------------------------------------------------------------------

const JobMaintenance = "section18.maintenance"

func (s *Service) RegisterWorkers(w *jobs.Worker) { w.Register(JobMaintenance, s.handleMaintenance) }
func (s *Service) ScheduleMaintenance(ctx context.Context) error {
	orgs, err := s.repo.ActiveOrganizationIDs(ctx)
	if err != nil {
		return err
	}
	for _, org := range orgs {
		if _, err = s.o.Queue.Enqueue(ctx, s.o.Pool, jobs.EnqueueRequest{OrganizationID: org, JobType: JobMaintenance, DedupeKey: "section18:" + org + ":" + time.Now().UTC().Format("200601021504"), Payload: map[string]string{"organizationId": org}, MaxAttempts: 8}); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) handleMaintenance(ctx context.Context, j jobs.Job) error {
	var p struct {
		OrganizationID string `json:"organizationId"`
	}
	if e := json.Unmarshal(j.Payload, &p); e != nil {
		return e
	}
	if p.OrganizationID != j.OrganizationID {
		return jobs.ErrTenantMismatch
	}
	if e := s.RunMaintenance(ctx, p.OrganizationID); e != nil {
		return e
	}
	if s.c.SafetyDeclines == nil {
		return nil
	}
	return s.c.SafetyDeclines.ReconcileSafetyDeclines(ctx)
}

// RunMaintenance is one organization's Section 18 sweep: expire asset
// documents and ended assignments, turn due preventive maintenance into work
// orders, then raise reorder alerts for parts at or below their level.
func (s *Service) RunMaintenance(ctx context.Context, organizationID string) error {
	if !s.o.Flags.Workers || s.o.Flags.KillSwitch {
		return nil
	}
	if e := s.repo.ExpireDocumentsAndAssignments(ctx, organizationID); e != nil {
		return e
	}
	if s.c.DueSchedules != nil {
		if e := s.c.DueSchedules.RunDueSchedules(ctx, organizationID); e != nil {
			return e
		}
	}
	if s.o.Alerts != nil {
		parts, e := s.repo.PartsAtReorderLevel(ctx, organizationID)
		if e != nil {
			return e
		}
		for _, id := range parts {
			s.alert(ctx, organizationID, "PART_REORDER_REQUIRED", id, id, "MEDIUM", "Part stock is at or below reorder level")
		}
	}
	return nil
}
