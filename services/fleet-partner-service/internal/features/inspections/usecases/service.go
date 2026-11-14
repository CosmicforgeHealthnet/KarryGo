// Package usecases implements the Section 18 inspection workflows: checklist
// templates, inspections, completion (which blocks the asset, raises the
// repair work order and records defects in one transaction with the
// inspection) and the unsafe-vehicle decline reconciler.
package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	assetmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/assets/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/inspections/models"
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

// Assets is the slice of the asset feature inspections compose.
type Assets interface {
	GetAsset(ctx context.Context, t tenant.Context, id string) (assetmodels.Asset, error)
	ActiveMembership(ctx context.Context, org, id string) bool
	ValidateMedia(ctx context.Context, t tenant.Context, id string) error
	LockAssetTx(ctx context.Context, tx pgx.Tx, org, id string) (assetmodels.AssetLock, error)
	BlockAssetTx(ctx context.Context, tx pgx.Tx, org, assetID, sourceType, sourceRef, reason, createdBy string) (int64, error)
	InsertSafetyBlockTx(ctx context.Context, tx pgx.Tx, org, assetID, sourceType, sourceRef, reason, createdBy string) error
	BlockAssetForDeclineTx(ctx context.Context, tx pgx.Tx, org, assetID string) error
	ResolveAssetTx(ctx context.Context, tx pgx.Tx, org, reference string) (id, branch string, found bool, e error)
	InsertDefectTx(ctx context.Context, tx pgx.Tx, t tenant.Context, branchID string, in assetmodels.ReportDefectInput, workOrderID string) (assetmodels.Defect, bool, error)
}

// WorkOrders is the slice of the maintenance feature inspections compose.
type WorkOrders interface {
	CreateSystemWorkOrderTx(ctx context.Context, tx pgx.Tx, org, assetID, sourceType, sourceRef, priority, requiredWork string) (string, int64, error)
	EnsureSystemWorkOrderTx(ctx context.Context, tx pgx.Tx, org, assetID, sourceType, sourceRef, priority, requiredWork string) (int64, bool, error)
	SystemWorkOrderIDTx(ctx context.Context, tx pgx.Tx, org, sourceType, sourceRef string) (string, error)
}

// Repository is the persistence inspections depend on.
type Repository interface {
	InsertTemplate(ctx context.Context, v *models.Template) error
	UpdateTemplate(ctx context.Context, org, id string, expected int64, v *models.Template) error
	GetTemplate(ctx context.Context, org, id string) (models.Template, error)
	ListTemplates(ctx context.Context, org string) ([]models.Template, error)
	InsertInspection(ctx context.Context, v *models.Inspection) error
	GetInspection(ctx context.Context, org, id string) (models.Inspection, error)
	ListInspections(ctx context.Context, org, status string, limit int, after *time.Time, afterID string) ([]models.Inspection, error)
	CompleteInspection(ctx context.Context, q database.Querier, org, id string, expected int64, result string, defectSummary json.RawMessage) (models.Inspection, error)
	InsertResult(ctx context.Context, q database.Querier, org, inspectionID string, a models.Answer, result string, critical bool) error
	PendingSafetyDeclines(ctx context.Context) ([]models.SafetyDecline, error)
	InsertDeclineInbox(ctx context.Context, q database.Querier, v models.SafetyDecline, payloadHash string) (bool, error)
	LockSystemTemplate(ctx context.Context, q database.Querier, org string) error
	SystemTemplateID(ctx context.Context, q database.Querier, org string) (string, error)
	InsertSystemTemplate(ctx context.Context, q database.Querier, id, org string, items, rules []byte) error
	InsertDeclineInspection(ctx context.Context, q database.Querier, id, org, assetID, templateID, membershipID, assignmentID string, defectSummary []byte) error
}

type Options struct {
	Pool       *pgxpool.Pool
	Repository Repository
	Assets     Assets
	WorkOrders WorkOrders
	Events     EventPublisher
	Audit      *audit.Recorder
	Alerts     AlertSink
	Flags      assetmodels.Flags
	Now        func() time.Time
}
type Service struct {
	o    Options
	repo Repository
}

// MaxPageSize bounds an inspection list page; DefaultPageSize keeps the page
// the list always served.
const (
	MaxPageSize     = 200
	DefaultPageSize = 50
)

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

func (s *Service) SaveTemplate(ctx context.Context, t tenant.Context, id string, expected int64, v models.Template) (models.Template, error) {
	if e := s.gate(s.o.Flags.Inspections, true); e != nil {
		return v, e
	}
	if len(v.ChecklistItems) == 0 || len(v.CriticalityRules) == 0 {
		return v, apperrors.BadRequest("Checklist and criticality rules are required.", nil)
	}
	if id == "" {
		v.ID = ids.New("ipt")
		v.OrganizationID = t.OrganizationID
		e := s.repo.InsertTemplate(ctx, &v)
		return v, e
	}
	e := s.repo.UpdateTemplate(ctx, t.OrganizationID, id, expected, &v)
	v.ID = id
	return v, e
}
func (s *Service) GetTemplate(ctx context.Context, t tenant.Context, id string) (models.Template, error) {
	if e := s.gate(s.o.Flags.Inspections, false); e != nil {
		return models.Template{}, e
	}
	v, e := s.repo.GetTemplate(ctx, t.OrganizationID, id)
	if e != nil {
		return v, apperrors.NotFound("Resource not found.", e)
	}
	return v, nil
}
func (s *Service) ListTemplates(ctx context.Context, t tenant.Context) ([]models.Template, error) {
	if e := s.gate(s.o.Flags.Inspections, false); e != nil {
		return nil, e
	}
	return s.repo.ListTemplates(ctx, t.OrganizationID)
}

func (s *Service) CreateInspection(ctx context.Context, t tenant.Context, v models.Inspection) (models.Inspection, error) {
	if e := s.gate(s.o.Flags.Inspections, true); e != nil {
		return v, e
	}
	if _, e := s.o.Assets.GetAsset(ctx, t, v.AssetID); e != nil {
		return v, e
	}
	if _, e := s.GetTemplate(ctx, t, v.TemplateID); e != nil {
		return v, e
	}
	if !s.o.Assets.ActiveMembership(ctx, t.OrganizationID, v.InspectorMembershipID) {
		return v, apperrors.NotFound("Inspector membership not found.", nil)
	}
	v.ID = ids.New("ins")
	v.OrganizationID = t.OrganizationID
	v.Status = "IN_PROGRESS"
	now := s.o.Now()
	v.StartedAt = &now
	e := s.repo.InsertInspection(ctx, &v)
	return v, e
}
func (s *Service) GetInspection(ctx context.Context, t tenant.Context, id string) (models.Inspection, error) {
	if e := s.gate(s.o.Flags.Inspections, false); e != nil {
		return models.Inspection{}, e
	}
	v, e := s.repo.GetInspection(ctx, t.OrganizationID, id)
	if e != nil {
		return v, apperrors.NotFound("Resource not found.", e)
	}
	if _, e = s.o.Assets.GetAsset(ctx, t, v.AssetID); e != nil {
		return v, e
	}
	return v, nil
}

// ListInspections returns the first page of inspections.
func (s *Service) ListInspections(ctx context.Context, t tenant.Context, p models.Page) ([]models.Inspection, error) {
	out, _, e := s.ListInspectionsPage(ctx, t, p)
	return out, e
}

// ListInspectionsPage returns one keyset page of the inspections the caller
// can see (newest first) and, when more rows exist, the last row read, which
// positions the next page. Inspections of assets outside the caller's
// branches are skipped without affecting the cursor, so pages never overlap
// or skip rows.
func (s *Service) ListInspectionsPage(ctx context.Context, t tenant.Context, p models.Page) ([]models.Inspection, *models.Inspection, error) {
	if e := s.gate(s.o.Flags.Inspections, false); e != nil {
		return nil, nil, e
	}
	size := p.Limit
	if size <= 0 {
		size = DefaultPageSize
	}
	if size > MaxPageSize {
		size = MaxPageSize
	}
	rows, e := s.repo.ListInspections(ctx, t.OrganizationID, p.Status, size+1, p.After, p.AfterID)
	if e != nil {
		return nil, nil, e
	}
	var last *models.Inspection
	if len(rows) > size {
		rows = rows[:size]
		last = &rows[size-1]
	}
	out := []models.Inspection{}
	for _, v := range rows {
		if _, e = s.o.Assets.GetAsset(ctx, t, v.AssetID); e != nil {
			continue
		}
		out = append(out, v)
	}
	return out, last, nil
}

func (s *Service) CompleteInspection(ctx context.Context, t tenant.Context, id string, in models.CompleteInspectionInput) (models.Inspection, error) {
	old, e := s.GetInspection(ctx, t, id)
	if e != nil {
		return old, e
	}
	if in.ExpectedVersion <= 0 {
		return old, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	tpl, e := s.GetTemplate(ctx, t, old.TemplateID)
	if e != nil {
		return old, e
	}
	var items []models.ChecklistItem
	if e = json.Unmarshal(tpl.ChecklistItems, &items); e != nil || len(items) == 0 {
		return old, apperrors.BadRequest("Inspection template checklist is invalid.", e)
	}
	result, e := models.Evaluate(items, in.Answers, func(a models.Answer) error {
		for _, mid := range a.EvidenceMediaIDs {
			if x := s.o.Assets.ValidateMedia(ctx, t, mid); x != nil {
				return x
			}
		}
		return nil
	})
	switch {
	case errors.Is(e, models.ErrMandatoryAnswerMissing):
		return old, apperrors.BadRequest("Every mandatory inspection item requires an answer.", nil)
	case errors.Is(e, models.ErrEvidenceMissing):
		return old, apperrors.BadRequest("Required inspection evidence is missing.", nil)
	case e != nil:
		return old, e
	}
	if len(in.DefectSummary) == 0 {
		in.DefectSummary = json.RawMessage(`[]`)
	}
	var v models.Inspection
	workID := ""
	e = s.tx(ctx, func(tx pgx.Tx) error {
		lock, x := s.o.Assets.LockAssetTx(ctx, tx, t.OrganizationID, old.AssetID)
		if x != nil {
			return x
		}
		v, x = s.repo.CompleteInspection(ctx, tx, t.OrganizationID, id, in.ExpectedVersion, result, in.DefectSummary)
		if x != nil {
			return apperrors.Conflict("Inspection completion conflict.", x)
		}
		for _, a := range in.Answers {
			r := "PASS"
			if !a.Passed {
				r = "FAIL"
			}
			if x = s.repo.InsertResult(ctx, tx, t.OrganizationID, id, a, r, models.ItemFor(items, a.ItemCode).Critical); x != nil {
				return x
			}
		}
		if result == models.ResultCriticalFail {
			assetVersion, x := s.o.Assets.BlockAssetTx(ctx, tx, t.OrganizationID, v.AssetID, "INSPECTION", id, "Critical inspection failure", t.ActorID)
			if x != nil {
				return x
			}
			var workVersion int64
			if workID, workVersion, x = s.o.WorkOrders.CreateSystemWorkOrderTx(ctx, tx, t.OrganizationID, v.AssetID, "INSPECTION", id, "CRITICAL", "Resolve critical inspection defects"); x != nil {
				return x
			}
			if x = s.record(ctx, tx, t, "work_order.created.v1", "work_order", workID, workVersion, map[string]any{"workOrderId": workID, "assetId": v.AssetID, "inspectionId": id}); x != nil {
				return x
			}
			if x = s.record(ctx, tx, t, "asset.blocked.v1", "asset", v.AssetID, assetVersion, map[string]any{"assetId": v.AssetID, "inspectionId": id, "operationalStatus": "BLOCKED"}); x != nil {
				return x
			}
		}
		// Every failed item becomes an asset defect (spec 48.2). Critical items
		// are linked to the repair work order the critical failure created, so
		// they are released only by its authorized return to service.
		for _, a := range in.Answers {
			if a.Passed {
				continue
			}
			severity, linked := "MEDIUM", ""
			for _, it := range items {
				if it.Code == a.ItemCode && it.Critical {
					severity, linked = "CRITICAL", workID
				}
			}
			if _, _, x = s.o.Assets.InsertDefectTx(ctx, tx, t, lock.BranchID, assetmodels.ReportDefectInput{AssetID: v.AssetID, SourceType: "INSPECTION", SourceReference: id, ItemCode: a.ItemCode, Description: "Inspection item " + a.ItemCode + " failed", Severity: severity}, linked); x != nil {
				return x
			}
		}
		ev := "inspection.completed.v1"
		if result == models.ResultCriticalFail {
			ev = "inspection.critical_failure.v1"
		}
		return s.record(ctx, tx, t, ev, "inspection", id, v.Version, v)
	})
	if e == nil && result == models.ResultCriticalFail && s.o.Alerts != nil {
		_ = s.o.Alerts.CreateSystemAlert(ctx, t.OrganizationID, "CRITICAL_INSPECTION", v.AssetID, id, "CRITICAL", "Asset blocked after critical inspection")
	}
	return v, e
}

// ReconcileSafetyDeclines consumes the durable Section 15 assignment projection.
// The inbox and source uniqueness constraints make repeated sweeps idempotent.
func (s *Service) ReconcileSafetyDeclines(ctx context.Context) error {
	if !s.o.Flags.Workers || s.o.Flags.KillSwitch || !s.o.Flags.Inspections || !s.o.Flags.Maintenance {
		return nil
	}
	items, e := s.repo.PendingSafetyDeclines(ctx)
	if e != nil {
		return e
	}
	for _, v := range items {
		if e = s.processSafetyDecline(ctx, v); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) processSafetyDecline(ctx context.Context, v models.SafetyDecline) error {
	var alertAsset string
	e := s.tx(ctx, func(tx pgx.Tx) error {
		h := sha256.Sum256([]byte(v.OrganizationID + "|" + v.AssignmentID + "|" + v.ReasonCode + "|" + v.Note))
		hash := hex.EncodeToString(h[:])
		assetID, assetBranch, found, x := s.o.Assets.ResolveAssetTx(ctx, tx, v.OrganizationID, v.AssetReference)
		if x != nil {
			return x
		}
		if !found {
			// The vehicle is not a Fleet-registered asset (for example a
			// provider's own vehicle on marketplace work). There is nothing
			// here to block; the owning execution service handles its own
			// vehicle safety. Record the decline as handled so one such
			// item can never wedge the sweep.
			_, x = s.repo.InsertDeclineInbox(ctx, tx, v, hash)
			return x
		}
		alertAsset = assetID
		inserted, x := s.repo.InsertDeclineInbox(ctx, tx, v, hash)
		if x != nil || !inserted {
			return x
		}
		if x = s.repo.LockSystemTemplate(ctx, tx, v.OrganizationID); x != nil {
			return x
		}
		templateID, x := s.repo.SystemTemplateID(ctx, tx, v.OrganizationID)
		if x == pgx.ErrNoRows {
			templateID = ids.New("ipt")
			items, _ := json.Marshal([]map[string]any{{"code": "vehicle_safety", "required": true, "evidenceRequired": true, "critical": true}})
			rules, _ := json.Marshal(map[string]any{"criticalFailure": "BLOCK_ASSET"})
			if x = s.repo.InsertSystemTemplate(ctx, tx, templateID, v.OrganizationID, items, rules); x != nil {
				return x
			}
		} else if x != nil {
			return x
		}
		inspectionID := ids.New("ins")
		summary, _ := json.Marshal([]map[string]string{{"reasonCode": v.ReasonCode, "safeNote": v.Note}})
		if x = s.repo.InsertDeclineInspection(ctx, tx, inspectionID, v.OrganizationID, assetID, templateID, v.MembershipID, v.AssignmentID, summary); x != nil {
			return x
		}
		if _, _, x = s.o.WorkOrders.EnsureSystemWorkOrderTx(ctx, tx, v.OrganizationID, assetID, v.Source, v.AssignmentID, "HIGH", "Inspect provider-reported unsafe vehicle condition"); x != nil {
			return x
		}
		if x = s.o.Assets.InsertSafetyBlockTx(ctx, tx, v.OrganizationID, assetID, v.Source, v.AssignmentID, v.ReasonCode, "safety-decline-reconciler"); x != nil {
			return x
		}
		// The reported condition is a critical defect linked to the repair
		// work order (spec 70.9): the asset is released only by authorized
		// return to service.
		repairID, x := s.o.WorkOrders.SystemWorkOrderIDTx(ctx, tx, v.OrganizationID, v.Source, v.AssignmentID)
		if x != nil {
			return x
		}
		reporter := tenant.Context{OrganizationID: v.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "safety-decline-reconciler", CorrelationID: v.AssignmentID}
		description := "Provider declined work: " + v.ReasonCode
		if v.Note != "" {
			description += " (" + v.Note + ")"
		}
		if _, _, x = s.o.Assets.InsertDefectTx(ctx, tx, reporter, assetBranch, assetmodels.ReportDefectInput{AssetID: assetID, SourceType: "SAFETY_DECLINE", SourceReference: v.AssignmentID, ItemCode: v.ReasonCode, Description: description, Severity: "CRITICAL"}, repairID); x != nil {
			return x
		}
		if x = s.o.Assets.BlockAssetForDeclineTx(ctx, tx, v.OrganizationID, assetID); x != nil {
			return x
		}
		return s.record(ctx, tx, reporter, "asset.blocked.v1", "asset", assetID, 1, map[string]any{"assetId": assetID, "assignmentId": v.AssignmentID, "reasonCode": v.ReasonCode, "automaticProviderPenalty": false, "inspectionId": inspectionID})
	})
	if e == nil && alertAsset != "" && s.o.Alerts != nil {
		_ = s.o.Alerts.CreateSystemAlert(ctx, v.OrganizationID, "UNSAFE_VEHICLE_DECLINE", alertAsset, v.AssignmentID, "HIGH", "Provider reported an unsafe vehicle; inspection and maintenance review required")
	}
	return e
}
