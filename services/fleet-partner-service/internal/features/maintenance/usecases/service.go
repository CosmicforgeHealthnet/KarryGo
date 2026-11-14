// Package usecases implements the Section 18 maintenance workflows:
// preventive schedules, work orders, authorized return to service, the
// system work orders raised by safety sources, and the due-schedule sweep.
// Safety-block and return-to-service transaction boundaries are preserved by
// composing the asset safety primitives into the same transaction.
package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	assetmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/assets/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/maintenance/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

type EventPublisher interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}
type AlertSink interface {
	CreateSystemAlert(context.Context, string, string, string, string, string, string) error
}
type RestrictionGate interface {
	Restricted(context.Context, string, string, string) (bool, error)
}

// Assets is the slice of the asset feature maintenance composes: visibility,
// media validation and the asset safety primitives, the latter inside the
// caller's transaction.
type Assets interface {
	GetAsset(ctx context.Context, t tenant.Context, id string) (assetmodels.Asset, error)
	ValidateMedia(ctx context.Context, t tenant.Context, id string) error
	LockAssetTx(ctx context.Context, tx pgx.Tx, org, id string) (assetmodels.AssetLock, error)
	RequireSafetyBlocksCoveredTx(ctx context.Context, tx pgx.Tx, org, assetID string) error
	ResolveRepairedCriticalDefectsTx(ctx context.Context, tx pgx.Tx, t tenant.Context, assetID, workOrderID string) error
	ReleaseSafetyBlockTx(ctx context.Context, tx pgx.Tx, t tenant.Context, assetID, workOrderID, reason string) (int64, error)
	BlockActiveAssetTx(ctx context.Context, tx pgx.Tx, org, assetID, sourceType, sourceRef, reason, createdBy string) (int64, error)
}

// InspectionChecks are the inspection facts return to service depends on.
type InspectionChecks interface {
	CriticalFollowUpUnresolved(ctx context.Context, q database.Querier, org, assetID, workOrderID string) (bool, error)
	SourceInspectionCleared(ctx context.Context, q database.Querier, org, inspectionID, assetID, workOrderID string) (sourceCritical, followupPassed bool, e error)
}

// Repository is the persistence maintenance depends on.
type Repository interface {
	InsertSchedule(ctx context.Context, q database.Querier, v models.Schedule) (models.Schedule, error)
	UpdateSchedule(ctx context.Context, q database.Querier, org, id string, expected int64, v models.Schedule) (models.Schedule, error)
	GetSchedule(ctx context.Context, org, id string) (models.Schedule, error)
	ListSchedules(ctx context.Context, org string, limit int, after *time.Time, afterID string) ([]models.Schedule, error)
	ListDueSchedules(ctx context.Context, org string) ([]models.DueSchedule, error)
	InsertWorkOrder(ctx context.Context, q database.Querier, v models.WorkOrder) (models.WorkOrder, error)
	InsertSystemWorkOrder(ctx context.Context, q database.Querier, id, org, assetID, sourceType, sourceRef, priority, requiredWork string) (int64, error)
	EnsureSystemWorkOrder(ctx context.Context, q database.Querier, id, org, assetID, sourceType, sourceRef, priority, requiredWork string) (int64, error)
	SystemWorkOrderID(ctx context.Context, q database.Querier, org, sourceType, sourceRef string) (string, error)
	GetWorkOrder(ctx context.Context, org, id string) (models.WorkOrder, error)
	ListWorkOrders(ctx context.Context, org string) ([]models.WorkOrder, error)
	UpdateWorkOrder(ctx context.Context, q database.Querier, org, id string, expected int64, in models.WorkAction, to, approver string, evidence []byte) (models.WorkOrder, error)
	InsertWorkOrderHistory(ctx context.Context, q database.Querier, org, id, previous, next, actor, reason string, version int64) error
	ApproveReturnToService(ctx context.Context, q database.Querier, org, id string, expected int64) (models.WorkOrder, error)
	InsertReturnToServiceDecision(ctx context.Context, q database.Querier, org, workOrderID, assetID, reason string, evidence []byte, actor string) error
}

type Options struct {
	Pool         *pgxpool.Pool
	Repository   Repository
	Assets       Assets
	Inspections  InspectionChecks
	Events       EventPublisher
	Audit        *audit.Recorder
	Alerts       AlertSink
	Restrictions RestrictionGate
	Flags        assetmodels.Flags
	Now          func() time.Time
}
type Service struct {
	o    Options
	repo Repository
}

// MaxPageSize bounds a schedule list page.
const MaxPageSize = 200

func NewService(o Options) *Service {
	if o.Audit == nil {
		o.Audit = audit.NewRecorder()
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Service{o: o, repo: o.Repository}
}
func (s *Service) SetAlertSink(alerts AlertSink) { s.o.Alerts = alerts }
func (s *Service) gate(on, write bool) error {
	if s.o.Flags.KillSwitch {
		return apperrors.Unavailable("Fleet operations are disabled.", nil)
	}
	if !on {
		if write {
			return apperrors.Forbidden("Capability is disabled.", nil)
		}
		return apperrors.NotFound("Resource not found.", nil)
	}
	return nil
}
func (s *Service) tx(ctx context.Context, f func(pgx.Tx) error) error {
	return database.WithinTx(ctx, s.o.Pool, f)
}
func (s *Service) record(ctx context.Context, tx pgx.Tx, t tenant.Context, event, kind, id string, v int64, p any) error {
	if e := s.o.Audit.Record(ctx, tx, audit.EntryFromTenant(t, strings.TrimSuffix(event, ".v1"), kind, id)); e != nil {
		return e
	}
	if s.o.Events != nil {
		return s.o.Events.Publish(ctx, tx, event, kind, id, v, t.OrganizationID, t.CorrelationID, p)
	}
	return nil
}

func (s *Service) SaveSchedule(ctx context.Context, t tenant.Context, id string, expected int64, v models.Schedule) (models.Schedule, error) {
	if e := s.gate(s.o.Flags.Maintenance, true); e != nil {
		return v, e
	}
	if id == "" {
		if _, e := s.o.Assets.GetAsset(ctx, t, v.AssetID); e != nil {
			return v, e
		}
		v.ID = ids.New("msc")
		v.OrganizationID = t.OrganizationID
		e := s.tx(ctx, func(tx pgx.Tx) error {
			var x error
			if v, x = s.repo.InsertSchedule(ctx, tx, v); x != nil {
				return x
			}
			return s.record(ctx, tx, t, "maintenance_schedule.created.v1", "maintenance_schedule", v.ID, v.Version, v)
		})
		return v, e
	}
	if _, e := s.GetSchedule(ctx, t, id); e != nil {
		return v, e
	}
	if expected <= 0 {
		return v, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = s.repo.UpdateSchedule(ctx, tx, t.OrganizationID, id, expected, v)
		if errors.Is(x, pgx.ErrNoRows) {
			return apperrors.Conflict("Maintenance schedule version conflict.", nil)
		}
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "maintenance_schedule.updated.v1", "maintenance_schedule", id, v.Version, v)
	})
	return v, e
}
func (s *Service) GetSchedule(ctx context.Context, t tenant.Context, id string) (models.Schedule, error) {
	if e := s.gate(s.o.Flags.Maintenance, false); e != nil {
		return models.Schedule{}, e
	}
	v, e := s.repo.GetSchedule(ctx, t.OrganizationID, id)
	if e != nil {
		return v, apperrors.NotFound("Resource not found.", e)
	}
	if _, e = s.o.Assets.GetAsset(ctx, t, v.AssetID); e != nil {
		return v, e
	}
	return v, nil
}

// ListSchedules returns the first page of schedules.
func (s *Service) ListSchedules(ctx context.Context, t tenant.Context) ([]models.Schedule, error) {
	out, _, e := s.ListSchedulesPage(ctx, t, models.Page{})
	return out, e
}

// ListSchedulesPage returns one keyset page of the schedules the caller can
// see (newest first), the keyset position of the last row read and whether
// more rows exist. Rows on assets outside the caller's branches are skipped,
// never counted against the cursor, so pages have no gaps or duplicates.
func (s *Service) ListSchedulesPage(ctx context.Context, t tenant.Context, p models.Page) ([]models.Schedule, *models.Schedule, error) {
	if e := s.gate(s.o.Flags.Maintenance, false); e != nil {
		return nil, nil, e
	}
	size := p.Limit
	if size <= 0 || size > MaxPageSize {
		size = MaxPageSize
	}
	rows, e := s.repo.ListSchedules(ctx, t.OrganizationID, size+1, p.After, p.AfterID)
	if e != nil {
		return nil, nil, e
	}
	var last *models.Schedule
	if len(rows) > size {
		rows = rows[:size]
		last = &rows[size-1]
	}
	out := []models.Schedule{}
	for _, v := range rows {
		if _, e = s.o.Assets.GetAsset(ctx, t, v.AssetID); e != nil {
			continue
		}
		out = append(out, v)
	}
	return out, last, nil
}

func (s *Service) CreateWorkOrder(ctx context.Context, t tenant.Context, v models.WorkOrder) (models.WorkOrder, error) {
	if e := s.gate(s.o.Flags.Maintenance, true); e != nil {
		return v, e
	}
	if _, e := s.o.Assets.GetAsset(ctx, t, v.AssetID); e != nil {
		return v, e
	}
	v.ID = ids.New("wor")
	v.OrganizationID = t.OrganizationID
	if v.Priority == "" {
		v.Priority = "MEDIUM"
	}
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		if v, x = s.repo.InsertWorkOrder(ctx, tx, v); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "work_order.created.v1", "work_order", v.ID, v.Version, v)
	})
	return v, e
}
func (s *Service) GetWorkOrder(ctx context.Context, t tenant.Context, id string) (models.WorkOrder, error) {
	if e := s.gate(s.o.Flags.Maintenance, false); e != nil {
		return models.WorkOrder{}, e
	}
	v, e := s.repo.GetWorkOrder(ctx, t.OrganizationID, id)
	if e != nil {
		return v, apperrors.NotFound("Resource not found.", e)
	}
	if _, e = s.o.Assets.GetAsset(ctx, t, v.AssetID); e != nil {
		return v, e
	}
	return v, nil
}

// WorkOrderVisible reports whether the caller can see the work order.
func (s *Service) WorkOrderVisible(ctx context.Context, t tenant.Context, id string) error {
	_, e := s.GetWorkOrder(ctx, t, id)
	return e
}
func (s *Service) ListWorkOrders(ctx context.Context, t tenant.Context) ([]models.WorkOrder, error) {
	if e := s.gate(s.o.Flags.Maintenance, false); e != nil {
		return nil, e
	}
	rows, e := s.repo.ListWorkOrders(ctx, t.OrganizationID)
	if e != nil {
		return nil, e
	}
	out := []models.WorkOrder{}
	for _, v := range rows {
		if _, e = s.o.Assets.GetAsset(ctx, t, v.AssetID); e != nil {
			continue
		}
		out = append(out, v)
	}
	return out, nil
}

// CreateSystemWorkOrderTx raises a repair work order for a safety source in
// the caller's transaction and returns its id and version.
func (s *Service) CreateSystemWorkOrderTx(ctx context.Context, tx pgx.Tx, org, assetID, sourceType, sourceRef, priority, requiredWork string) (string, int64, error) {
	id := ids.New("wor")
	version, e := s.repo.InsertSystemWorkOrder(ctx, tx, id, org, assetID, sourceType, sourceRef, priority, requiredWork)
	return id, version, e
}

// EnsureSystemWorkOrderTx raises a work order once per source in the caller's
// transaction; inserted is false when the source already has one.
func (s *Service) EnsureSystemWorkOrderTx(ctx context.Context, tx pgx.Tx, org, assetID, sourceType, sourceRef, priority, requiredWork string) (int64, bool, error) {
	version, e := s.repo.EnsureSystemWorkOrder(ctx, tx, ids.New("wor"), org, assetID, sourceType, sourceRef, priority, requiredWork)
	if errors.Is(e, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return version, e == nil, e
}

// SystemWorkOrderIDTx is the work order raised for one source.
func (s *Service) SystemWorkOrderIDTx(ctx context.Context, tx pgx.Tx, org, sourceType, sourceRef string) (string, error) {
	return s.repo.SystemWorkOrderID(ctx, tx, org, sourceType, sourceRef)
}

func (s *Service) UpdateWorkOrder(ctx context.Context, t tenant.Context, id, action string, in models.WorkAction) (models.WorkOrder, error) {
	old, e := s.GetWorkOrder(ctx, t, id)
	if e != nil {
		return old, e
	}
	if in.ExpectedVersion <= 0 {
		return old, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	to := old.Status
	ev := "work_order.updated.v1"
	switch action {
	case "":
		if in.Status != "" {
			if !models.ManualTransitions[old.Status][in.Status] {
				return old, apperrors.Conflict("Invalid work-order transition.", nil)
			}
			to = in.Status
		}
	case "APPROVE":
		if old.Status != "DRAFT" && old.Status != "PENDING_APPROVAL" {
			return old, apperrors.Conflict("Work order is not approvable.", nil)
		}
		to = "APPROVED"
		ev = "work_order.approved.v1"
	case "COMPLETE":
		if old.Status != "APPROVED" && old.Status != "IN_PROGRESS" {
			return old, apperrors.Conflict("Work order is not completable.", nil)
		}
		if in.RequiredWork == "" && old.RequiredWork == "" {
			return old, apperrors.BadRequest("Work summary is required.", nil)
		}
		for _, mid := range in.EvidenceMediaIDs {
			if e = s.o.Assets.ValidateMedia(ctx, t, mid); e != nil {
				return old, e
			}
		}
		to = "COMPLETED"
		ev = "work_order.completed.v1"
	}
	raw, _ := json.Marshal(in.EvidenceMediaIDs)
	var v models.WorkOrder
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		v, x = s.repo.UpdateWorkOrder(ctx, tx, t.OrganizationID, id, in.ExpectedVersion, in, to, t.OrganizationUserID, raw)
		if x != nil {
			return apperrors.Conflict("Work order version conflict.", x)
		}
		if x = s.repo.InsertWorkOrderHistory(ctx, tx, t.OrganizationID, id, old.Status, to, t.ActorID, in.Reason, v.Version); x != nil {
			return x
		}
		return s.record(ctx, tx, t, ev, "work_order", id, v.Version, v)
	})
	return v, e
}
func (s *Service) ReturnToService(ctx context.Context, t tenant.Context, id string, in models.WorkAction) (models.WorkOrder, error) {
	old, e := s.GetWorkOrder(ctx, t, id)
	if e != nil {
		return old, e
	}
	if old.Status != "COMPLETED" || in.ExpectedVersion <= 0 {
		return old, apperrors.Conflict("Completed work order and expected version are required.", nil)
	}
	if s.o.Restrictions != nil {
		restricted, gateErr := s.o.Restrictions.Restricted(ctx, t.OrganizationID, "ASSET", old.AssetID)
		if gateErr != nil {
			return old, apperrors.Unavailable("Restriction authority is unavailable.", gateErr)
		}
		if restricted {
			return old, apperrors.Conflict("Asset has an active compliance restriction.", nil)
		}
	}
	if len(in.EvidenceMediaIDs) == 0 && len(old.EvidenceMediaIDs) <= 2 {
		return old, apperrors.BadRequest("Return-to-service evidence is required.", nil)
	}
	for _, mid := range in.EvidenceMediaIDs {
		if e = s.o.Assets.ValidateMedia(ctx, t, mid); e != nil {
			return old, e
		}
	}
	raw, _ := json.Marshal(in.EvidenceMediaIDs)
	var v models.WorkOrder
	e = s.tx(ctx, func(tx pgx.Tx) error {
		// Serialize with every other writer of this asset's safety state.
		lock, x := s.o.Assets.LockAssetTx(ctx, tx, t.OrganizationID, old.AssetID)
		if x != nil {
			return x
		}
		if lock.OperationalStatus != "BLOCKED" {
			return apperrors.Conflict("Asset is not under a safety block.", nil)
		}
		// An authorized release must cover every restriction in force, not only
		// the one this work order was raised for (spec 48.2).
		if x = s.o.Assets.RequireSafetyBlocksCoveredTx(ctx, tx, t.OrganizationID, old.AssetID); x != nil {
			return x
		}
		if x = s.o.Assets.ResolveRepairedCriticalDefectsTx(ctx, tx, t, old.AssetID, id); x != nil {
			return x
		}
		unresolved, x := s.o.Inspections.CriticalFollowUpUnresolved(ctx, tx, t.OrganizationID, old.AssetID, id)
		if x != nil {
			return x
		}
		if unresolved {
			return apperrors.Conflict("A follow-up critical inspection remains unresolved.", nil)
		}
		if old.SourceType == "INSPECTION" {
			sourceCritical, followupPassed, x := s.o.Inspections.SourceInspectionCleared(ctx, tx, t.OrganizationID, old.SourceReference, old.AssetID, id)
			if x != nil {
				return x
			}
			if sourceCritical && !followupPassed {
				return apperrors.Conflict("A passing follow-up inspection is required before return to service.", nil)
			}
		}
		v, x = s.repo.ApproveReturnToService(ctx, tx, t.OrganizationID, id, in.ExpectedVersion)
		if x != nil {
			return apperrors.Conflict("Return-to-service conflict.", x)
		}
		if x = s.repo.InsertReturnToServiceDecision(ctx, tx, t.OrganizationID, id, old.AssetID, in.Reason, raw, t.ActorID); x != nil {
			return x
		}
		assetVersion, x := s.o.Assets.ReleaseSafetyBlockTx(ctx, tx, t, old.AssetID, id, in.Reason)
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, "asset.returned_to_service.v1", "asset", old.AssetID, assetVersion, map[string]any{"assetId": old.AssetID, "workOrderId": id, "returnToServiceStatus": "APPROVED"})
	})
	return v, e
}

// RunDueSchedules turns each due preventive-maintenance schedule into a work
// order (once per schedule) and blocks the asset for critical maintenance.
func (s *Service) RunDueSchedules(ctx context.Context, organizationID string) error {
	items, e := s.repo.ListDueSchedules(ctx, organizationID)
	if e != nil {
		return e
	}
	for _, v := range items {
		scope := tenant.Context{OrganizationID: organizationID, ActorType: tenant.ActorTypeSystem, ActorID: "section18-maintenance-worker", CorrelationID: v.ID}
		if e = s.tx(ctx, func(tx pgx.Tx) error {
			workID := ids.New("wor")
			priority := "MEDIUM"
			if v.Critical {
				priority = "CRITICAL"
			}
			version, x := s.repo.EnsureSystemWorkOrder(ctx, tx, workID, organizationID, v.AssetID, "MAINTENANCE_SCHEDULE", v.ID, priority, "Perform due preventive maintenance")
			if x != nil && x != pgx.ErrNoRows {
				return x
			}
			if x == nil {
				if x = s.record(ctx, tx, scope, "maintenance_schedule.due.v1", "maintenance_schedule", v.ID, 1, map[string]any{"scheduleId": v.ID, "assetId": v.AssetID}); x != nil {
					return x
				}
				if x = s.record(ctx, tx, scope, "work_order.created.v1", "work_order", workID, version, map[string]any{"workOrderId": workID, "scheduleId": v.ID, "assetId": v.AssetID}); x != nil {
					return x
				}
			}
			if v.Critical {
				assetVersion, x := s.o.Assets.BlockActiveAssetTx(ctx, tx, organizationID, v.AssetID, "MAINTENANCE_SCHEDULE", v.ID, "Critical maintenance overdue", "section18-maintenance-worker")
				if x != nil {
					return x
				}
				if assetVersion > 0 {
					if x = s.record(ctx, tx, scope, "asset.blocked.v1", "asset", v.AssetID, assetVersion, map[string]any{"assetId": v.AssetID, "scheduleId": v.ID, "operationalStatus": "BLOCKED"}); x != nil {
						return x
					}
				}
			}
			return nil
		}); e != nil {
			return e
		}
		if v.Critical && s.o.Alerts != nil {
			_ = s.o.Alerts.CreateSystemAlert(ctx, organizationID, "CRITICAL_MAINTENANCE_OVERDUE", v.AssetID, v.ID, "HIGH", "Critical preventive maintenance is overdue")
		}
	}
	return nil
}
