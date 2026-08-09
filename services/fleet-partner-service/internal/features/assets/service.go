package assets

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
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
type Options struct {
	Pool         *pgxpool.Pool
	Events       EventPublisher
	Audit        *audit.Recorder
	Media        platformclients.MediaContract
	Alerts       AlertSink
	Restrictions RestrictionGate
	Queue        *jobs.Queue
	Flags        Flags
	Now          func() time.Time
}
type Service struct{ o Options }

func NewService(o Options) *Service {
	if o.Audit == nil {
		o.Audit = audit.NewRecorder()
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Queue == nil {
		o.Queue = jobs.NewQueue(o.Pool)
	}
	return &Service{o: o}
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
func lim(n int) int {
	if n <= 0 {
		return 50
	}
	if n > 200 {
		return 200
	}
	return n
}
func (s *Service) member(ctx context.Context, org, id string) bool {
	var ok bool
	_ = s.o.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_organization_provider_memberships WHERE organization_id=$1 AND id=$2 AND status='ACTIVE')`, org, id).Scan(&ok)
	return ok
}
func (s *Service) media(ctx context.Context, t tenant.Context, id string) error {
	if id == "" {
		return apperrors.BadRequest("Media ID is required.", nil)
	}
	if s.o.Media == nil || !s.o.Media.Configured() {
		return apperrors.Unavailable("Media authority is unavailable.", nil)
	}
	m, e := s.o.Media.GetFile(ctx, platformclients.CallMeta{OrganizationID: t.OrganizationID, CorrelationID: t.CorrelationID}, id)
	if e != nil {
		return e
	}
	if !m.Ready() || m.OwnerID != t.OrganizationID {
		return apperrors.Forbidden("Media ownership or safety validation failed.", nil)
	}
	return nil
}

const assetCols = `id,organization_id,branch_id,asset_code,asset_type,platform_vehicle_id,ownership_type,registration_reference,manufacturer,model,year,capacity,capacity_unit,odometer,operating_hours,operational_status,lifecycle_status,version,created_at,updated_at,archived_at,retired_at`

func scanAsset(r pgx.Row) (Asset, error) {
	var v Asset
	e := r.Scan(&v.ID, &v.OrganizationID, &v.BranchID, &v.AssetCode, &v.AssetType, &v.PlatformVehicleID, &v.OwnershipType, &v.RegistrationReference, &v.Manufacturer, &v.Model, &v.Year, &v.Capacity, &v.CapacityUnit, &v.Odometer, &v.OperatingHours, &v.OperationalStatus, &v.LifecycleStatus, &v.Version, &v.CreatedAt, &v.UpdatedAt, &v.ArchivedAt, &v.RetiredAt)
	return v, e
}
func (s *Service) CreateAsset(ctx context.Context, t tenant.Context, v Asset) (Asset, error) {
	if e := s.gate(s.o.Flags.Assets, true); e != nil {
		return v, e
	}
	if !t.BranchPermitted(v.BranchID) || v.AssetCode == "" || v.AssetType == "" {
		return v, apperrors.BadRequest("Branch, asset code and type are required.", nil)
	}
	v.ID = ids.New("ast")
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var e error
		v, e = scanAsset(tx.QueryRow(ctx, `INSERT INTO fleet_partner_assets(id,organization_id,branch_id,asset_code,asset_type,platform_vehicle_id,ownership_type,registration_reference,manufacturer,model,year,capacity,capacity_unit,odometer,operating_hours)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)RETURNING `+assetCols, v.ID, t.OrganizationID, v.BranchID, v.AssetCode, strings.ToUpper(v.AssetType), v.PlatformVehicleID, v.OwnershipType, v.RegistrationReference, v.Manufacturer, v.Model, v.Year, v.Capacity, v.CapacityUnit, v.Odometer, v.OperatingHours))
		if e != nil {
			return e
		}
		return s.record(ctx, tx, t, "asset.created.v1", "asset", v.ID, v.Version, v)
	})
	return v, e
}
func (s *Service) GetAsset(ctx context.Context, t tenant.Context, id string) (Asset, error) {
	if e := s.gate(s.o.Flags.Assets, false); e != nil {
		return Asset{}, e
	}
	v, e := scanAsset(s.o.Pool.QueryRow(ctx, `SELECT `+assetCols+` FROM fleet_partner_assets WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if e != nil || !t.BranchPermitted(v.BranchID) {
		return v, apperrors.NotFound("Resource not found.", e)
	}
	return v, nil
}
func (s *Service) ListAssets(ctx context.Context, t tenant.Context, f Page) ([]Asset, error) {
	if e := s.gate(s.o.Flags.Assets, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+assetCols+` FROM fleet_partner_assets WHERE organization_id=$1 AND($2=''OR branch_id=$2)AND($3=''OR operational_status=$3 OR lifecycle_status=$3)ORDER BY created_at DESC LIMIT $4 OFFSET $5`, t.OrganizationID, f.BranchID, f.Status, lim(f.Limit), f.Offset)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Asset{}
	for rows.Next() {
		v, e := scanAsset(rows)
		if e != nil {
			return nil, e
		}
		if t.BranchPermitted(v.BranchID) {
			out = append(out, v)
		}
	}
	return out, rows.Err()
}
func (s *Service) UpdateAsset(ctx context.Context, t tenant.Context, id string, expected int64, v Asset, action string) (Asset, error) {
	old, e := s.GetAsset(ctx, t, id)
	if e != nil {
		return v, e
	}
	if expected <= 0 {
		return v, apperrors.BadRequest("expectedVersion is required.", nil)
	}
	op := old.OperationalStatus
	life := old.LifecycleStatus
	ev := "asset.updated.v1"
	switch action {
	case "ARCHIVE":
		life = "ARCHIVED"
		ev = "asset.archived.v1"
	case "RETIRE":
		life = "RETIRED"
		ev = "asset.retired.v1"
	}
	if action != "" {
		var active bool
		_ = s.o.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_asset_assignments WHERE asset_id=$1 AND status='ACTIVE')`, id).Scan(&active)
		if active {
			return v, apperrors.Conflict("Asset has an active assignment.", nil)
		}
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		if action != "" {
			var lockedVersion int64
			if x := tx.QueryRow(ctx, `SELECT version FROM fleet_partner_assets WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, id).Scan(&lockedVersion); x != nil {
				return x
			}
			if lockedVersion != expected {
				return apperrors.Conflict("Version conflict.", nil)
			}
			var active bool
			if x := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_asset_assignments WHERE organization_id=$1 AND asset_id=$2 AND status='ACTIVE')`, t.OrganizationID, id).Scan(&active); x != nil {
				return x
			}
			if active {
				return apperrors.Conflict("Asset has an active assignment.", nil)
			}
		}
		var x error
		v, x = scanAsset(tx.QueryRow(ctx, `UPDATE fleet_partner_assets SET manufacturer=COALESCE(NULLIF($4,''),manufacturer),model=COALESCE(NULLIF($5,''),model),capacity=CASE WHEN $6>=0 THEN $6 ELSE capacity END,odometer=CASE WHEN $7>=odometer THEN $7 ELSE odometer END,operating_hours=CASE WHEN $8>=operating_hours THEN $8 ELSE operating_hours END,lifecycle_status=$9,archived_at=CASE WHEN $9='ARCHIVED' THEN now() ELSE archived_at END,retired_at=CASE WHEN $9='RETIRED' THEN now() ELSE retired_at END,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING `+assetCols, t.OrganizationID, id, expected, v.Manufacturer, v.Model, v.Capacity, v.Odometer, v.OperatingHours, life))
		if x != nil {
			return apperrors.Conflict("Version conflict.", x)
		}
		_, x = tx.Exec(ctx, `INSERT INTO fleet_partner_asset_history(id,organization_id,asset_id,previous_operational_status,new_operational_status,previous_lifecycle_status,new_lifecycle_status,reason,actor_id,version,snapshot)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, ids.New("ash"), t.OrganizationID, id, op, v.OperationalStatus, old.LifecycleStatus, v.LifecycleStatus, action, t.ActorID, v.Version, mustJSON(v))
		if x != nil {
			return x
		}
		return s.record(ctx, tx, t, ev, "asset", id, v.Version, v)
	})
	return v, e
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

func (s *Service) Assign(ctx context.Context, t tenant.Context, assetID string, v Assignment) (Assignment, error) {
	if e := s.gate(s.o.Flags.Assignments, true); e != nil {
		return v, e
	}
	a, e := s.GetAsset(ctx, t, assetID)
	if e != nil {
		return v, e
	}
	el, e := s.Evaluate(ctx, t.OrganizationID, assetID, "", "", 0, "", v.StartAt)
	if e != nil || !el.Eligible {
		return v, apperrors.Conflict("Asset is not eligible for assignment.", e)
	}
	if !s.member(ctx, t.OrganizationID, v.ProviderMembershipID) {
		return v, apperrors.NotFound("Provider membership not found.", nil)
	}
	if v.StartAt.IsZero() {
		v.StartAt = s.o.Now()
	}
	v.ID = ids.New("asa")
	v.OrganizationID = t.OrganizationID
	v.AssetID = assetID
	v.Status = "ACTIVE"
	e = s.tx(ctx, func(tx pgx.Tx) error {
		var operational, lifecycle string
		if x := tx.QueryRow(ctx, `SELECT operational_status,lifecycle_status FROM fleet_partner_assets WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, assetID).Scan(&operational, &lifecycle); x != nil {
			return x
		}
		if lifecycle != "ACTIVE" || operational != "AVAILABLE" {
			return apperrors.Conflict("Asset is no longer available for assignment.", nil)
		}
		e := tx.QueryRow(ctx, `INSERT INTO fleet_partner_asset_assignments(id,organization_id,asset_id,provider_membership_id,shift_id,assignment_type,start_at,end_at,status)VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8,'ACTIVE')RETURNING version,created_at,updated_at`, v.ID, v.OrganizationID, assetID, v.ProviderMembershipID, v.ShiftID, v.AssignmentType, v.StartAt, v.EndAt).Scan(&v.Version, &v.CreatedAt, &v.UpdatedAt)
		if e != nil {
			return apperrors.Conflict("Asset assignment conflict.", e)
		}
		_, e = tx.Exec(ctx, `UPDATE fleet_partner_assets SET operational_status='ASSIGNED',version=version+1,updated_at=now() WHERE id=$1 AND operational_status='AVAILABLE'`, assetID)
		if e != nil {
			return e
		}
		return s.record(ctx, tx, t, "asset.assigned.v1", "asset_assignment", v.ID, v.Version, v)
	})
	_ = a
	return v, e
}
func (s *Service) AddDocument(ctx context.Context, t tenant.Context, assetID string, v Document) (Document, error) {
	if e := s.gate(s.o.Flags.Assets, true); e != nil {
		return v, e
	}
	if _, e := s.GetAsset(ctx, t, assetID); e != nil {
		return v, e
	}
	if e := s.media(ctx, t, v.MediaFileID); e != nil {
		return v, e
	}
	v.ID = ids.New("asd")
	v.OrganizationID = t.OrganizationID
	v.AssetID = assetID
	v.VerificationStatus = "PENDING"
	e := s.o.Pool.QueryRow(ctx, `INSERT INTO fleet_partner_asset_documents(id,organization_id,asset_id,media_file_id,document_type,issued_at,expires_at)VALUES($1,$2,$3,$4,$5,$6,$7)RETURNING version,created_at,updated_at`, v.ID, v.OrganizationID, assetID, v.MediaFileID, v.DocumentType, v.IssuedAt, v.ExpiresAt).Scan(&v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func (s *Service) ListDocuments(ctx context.Context, t tenant.Context, assetID string) ([]Document, error) {
	if _, e := s.GetAsset(ctx, t, assetID); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT id,organization_id,asset_id,media_file_id,document_type,issued_at,expires_at,verification_reference,verification_status,version,created_at,updated_at FROM fleet_partner_asset_documents WHERE organization_id=$1 AND asset_id=$2`, t.OrganizationID, assetID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Document{}
	for rows.Next() {
		var v Document
		if e = rows.Scan(&v.ID, &v.OrganizationID, &v.AssetID, &v.MediaFileID, &v.DocumentType, &v.IssuedAt, &v.ExpiresAt, &v.VerificationReference, &v.VerificationStatus, &v.Version, &v.CreatedAt, &v.UpdatedAt); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Service) SaveTemplate(ctx context.Context, t tenant.Context, id string, expected int64, v Template) (Template, error) {
	if e := s.gate(s.o.Flags.Inspections, true); e != nil {
		return v, e
	}
	if len(v.ChecklistItems) == 0 || len(v.CriticalityRules) == 0 {
		return v, apperrors.BadRequest("Checklist and criticality rules are required.", nil)
	}
	a, _ := json.Marshal(v.AssetTypes)
	b, _ := json.Marshal(v.ServiceTypes)
	if id == "" {
		v.ID = ids.New("ipt")
		v.OrganizationID = t.OrganizationID
		e := s.o.Pool.QueryRow(ctx, `INSERT INTO fleet_partner_inspection_templates(id,organization_id,name,asset_types,service_types,checklist_items,criticality_rules)VALUES($1,$2,$3,$4,$5,$6,$7)RETURNING status,version,created_at,updated_at`, v.ID, v.OrganizationID, v.Name, a, b, v.ChecklistItems, v.CriticalityRules).Scan(&v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt)
		return v, e
	}
	e := s.o.Pool.QueryRow(ctx, `UPDATE fleet_partner_inspection_templates SET name=COALESCE(NULLIF($4,''),name),checklist_items=COALESCE($5,checklist_items),criticality_rules=COALESCE($6,criticality_rules),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 RETURNING organization_id,status,version,created_at,updated_at`, t.OrganizationID, id, expected, v.Name, v.ChecklistItems, v.CriticalityRules).Scan(&v.OrganizationID, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	v.ID = id
	return v, e
}
func scanTemplate(r pgx.Row) (Template, error) {
	var v Template
	var a, b []byte
	e := r.Scan(&v.ID, &v.OrganizationID, &v.Name, &a, &b, &v.ChecklistItems, &v.CriticalityRules, &v.Status, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	_ = json.Unmarshal(a, &v.AssetTypes)
	_ = json.Unmarshal(b, &v.ServiceTypes)
	return v, e
}
func (s *Service) GetTemplate(ctx context.Context, t tenant.Context, id string) (Template, error) {
	if e := s.gate(s.o.Flags.Inspections, false); e != nil {
		return Template{}, e
	}
	v, e := scanTemplate(s.o.Pool.QueryRow(ctx, `SELECT id,organization_id,name,asset_types,service_types,checklist_items,criticality_rules,status,version,created_at,updated_at FROM fleet_partner_inspection_templates WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if e != nil {
		return v, apperrors.NotFound("Resource not found.", e)
	}
	return v, nil
}
func (s *Service) ListTemplates(ctx context.Context, t tenant.Context) ([]Template, error) {
	if e := s.gate(s.o.Flags.Inspections, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT id,organization_id,name,asset_types,service_types,checklist_items,criticality_rules,status,version,created_at,updated_at FROM fleet_partner_inspection_templates WHERE organization_id=$1 ORDER BY created_at DESC`, t.OrganizationID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Template{}
	for rows.Next() {
		v, e := scanTemplate(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Service) CreateInspection(ctx context.Context, t tenant.Context, v Inspection) (Inspection, error) {
	if e := s.gate(s.o.Flags.Inspections, true); e != nil {
		return v, e
	}
	if _, e := s.GetAsset(ctx, t, v.AssetID); e != nil {
		return v, e
	}
	if _, e := s.GetTemplate(ctx, t, v.TemplateID); e != nil {
		return v, e
	}
	if !s.member(ctx, t.OrganizationID, v.InspectorMembershipID) {
		return v, apperrors.NotFound("Inspector membership not found.", nil)
	}
	v.ID = ids.New("ins")
	v.OrganizationID = t.OrganizationID
	v.Status = "IN_PROGRESS"
	now := s.o.Now()
	v.StartedAt = &now
	e := s.o.Pool.QueryRow(ctx, `INSERT INTO fleet_partner_inspections(id,organization_id,asset_id,template_id,inspector_membership_id,assignment_id,trip_reference,status,started_at)VALUES($1,$2,$3,$4,$5,$6,$7,'IN_PROGRESS',$8)RETURNING version,created_at,updated_at`, v.ID, v.OrganizationID, v.AssetID, v.TemplateID, v.InspectorMembershipID, v.AssignmentID, v.TripReference, v.StartedAt).Scan(&v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}
func scanInspection(r pgx.Row) (Inspection, error) {
	var v Inspection
	e := r.Scan(&v.ID, &v.OrganizationID, &v.AssetID, &v.TemplateID, &v.InspectorMembershipID, &v.AssignmentID, &v.TripReference, &v.Status, &v.OverallResult, &v.DefectSummary, &v.EvidenceMediaIDs, &v.StartedAt, &v.CompletedAt, &v.Version, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

const inspectionCols = `id,organization_id,asset_id,template_id,inspector_membership_id,assignment_id,trip_reference,status,COALESCE(overall_result,''),defect_summary,evidence_media_ids,started_at,completed_at,version,created_at,updated_at`

func (s *Service) GetInspection(ctx context.Context, t tenant.Context, id string) (Inspection, error) {
	if e := s.gate(s.o.Flags.Inspections, false); e != nil {
		return Inspection{}, e
	}
	v, e := scanInspection(s.o.Pool.QueryRow(ctx, `SELECT `+inspectionCols+` FROM fleet_partner_inspections WHERE organization_id=$1 AND id=$2`, t.OrganizationID, id))
	if e != nil {
		return v, apperrors.NotFound("Resource not found.", e)
	}
	if _, e = s.GetAsset(ctx, t, v.AssetID); e != nil {
		return v, e
	}
	return v, nil
}
func (s *Service) ListInspections(ctx context.Context, t tenant.Context, f Page) ([]Inspection, error) {
	if e := s.gate(s.o.Flags.Inspections, false); e != nil {
		return nil, e
	}
	rows, e := s.o.Pool.Query(ctx, `SELECT `+inspectionCols+` FROM fleet_partner_inspections WHERE organization_id=$1 AND($2=''OR status=$2)ORDER BY created_at DESC LIMIT $3 OFFSET $4`, t.OrganizationID, f.Status, lim(f.Limit), f.Offset)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Inspection{}
	for rows.Next() {
		v, e := scanInspection(rows)
		if e != nil {
			return nil, e
		}
		if _, e = s.GetAsset(ctx, t, v.AssetID); e != nil {
			continue
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

type CompleteInspectionInput struct {
	ExpectedVersion int64           `json:"expectedVersion"`
	Answers         []Answer        `json:"answers"`
	DefectSummary   json.RawMessage `json:"defectSummary"`
}
type checklistItem struct {
	Code                                 string `json:"code"`
	Required, EvidenceRequired, Critical bool
}

func (s *Service) CompleteInspection(ctx context.Context, t tenant.Context, id string, in CompleteInspectionInput) (Inspection, error) {
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
	var items []checklistItem
	if e = json.Unmarshal(tpl.ChecklistItems, &items); e != nil || len(items) == 0 {
		return old, apperrors.BadRequest("Inspection template checklist is invalid.", e)
	}
	answers := map[string]Answer{}
	for _, a := range in.Answers {
		answers[a.ItemCode] = a
	}
	result := "PASS"
	for _, it := range items {
		a, ok := answers[it.Code]
		if it.Required && !ok {
			return old, apperrors.BadRequest("Every mandatory inspection item requires an answer.", nil)
		}
		if !ok {
			continue
		}
		if it.EvidenceRequired && len(a.EvidenceMediaIDs) == 0 {
			return old, apperrors.BadRequest("Required inspection evidence is missing.", nil)
		}
		for _, mid := range a.EvidenceMediaIDs {
			if e = s.media(ctx, t, mid); e != nil {
				return old, e
			}
		}
		if !a.Passed {
			if it.Critical {
				result = "CRITICAL_FAIL"
			} else if result != "CRITICAL_FAIL" {
				result = "FAIL"
			}
		} else if a.Advisory && result == "PASS" {
			result = "PASS_WITH_ADVISORY"
		}
	}
	var v Inspection
	e = s.tx(ctx, func(tx pgx.Tx) error {
		if _, x := tx.Exec(ctx, `SELECT id FROM fleet_partner_assets WHERE organization_id=$1 AND id=$2 FOR UPDATE`, t.OrganizationID, old.AssetID); x != nil {
			return x
		}
		var x error
		v, x = scanInspection(tx.QueryRow(ctx, `UPDATE fleet_partner_inspections SET status='COMPLETED',overall_result=$4,defect_summary=$5,completed_at=now(),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 AND status IN('DRAFT','IN_PROGRESS') RETURNING `+inspectionCols, t.OrganizationID, id, in.ExpectedVersion, result, in.DefectSummary))
		if x != nil {
			return apperrors.Conflict("Inspection completion conflict.", x)
		}
		for _, a := range in.Answers {
			it := checklistItem{}
			for _, x := range items {
				if x.Code == a.ItemCode {
					it = x
				}
			}
			raw, _ := json.Marshal(a)
			ev, _ := json.Marshal(a.EvidenceMediaIDs)
			r := "PASS"
			if !a.Passed {
				r = "FAIL"
			}
			if _, x = tx.Exec(ctx, `INSERT INTO fleet_partner_inspection_results(id,organization_id,inspection_id,item_code,answer,evidence_media_ids,result,critical)VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, ids.New("inr"), t.OrganizationID, id, a.ItemCode, raw, ev, r, it.Critical); x != nil {
				return x
			}
		}
		if result == "CRITICAL_FAIL" {
			var assetVersion int64
			if x = tx.QueryRow(ctx, `UPDATE fleet_partner_assets SET operational_status='BLOCKED',version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, t.OrganizationID, v.AssetID).Scan(&assetVersion); x != nil {
				return x
			}
			if _, x = tx.Exec(ctx, `INSERT INTO fleet_partner_asset_safety_blocks(id,organization_id,asset_id,source_type,source_reference,status,reason,created_by)VALUES($1,$2,$3,'INSPECTION',$4,'ACTIVE','Critical inspection failure',$5)`, ids.New("asb"), t.OrganizationID, v.AssetID, id, t.ActorID); x != nil {
				return x
			}
			workID := ids.New("wor")
			var workVersion int64
			if x = tx.QueryRow(ctx, `INSERT INTO fleet_partner_work_orders(id,organization_id,asset_id,source_type,source_reference,priority,required_work)VALUES($1,$2,$3,'INSPECTION',$4,'CRITICAL','Resolve critical inspection defects') RETURNING version`, workID, t.OrganizationID, v.AssetID, id).Scan(&workVersion); x != nil {
				return x
			}
			if x = s.record(ctx, tx, t, "work_order.created.v1", "work_order", workID, workVersion, map[string]any{"workOrderId": workID, "assetId": v.AssetID, "inspectionId": id}); x != nil {
				return x
			}
			if x = s.record(ctx, tx, t, "asset.blocked.v1", "asset", v.AssetID, assetVersion, map[string]any{"assetId": v.AssetID, "inspectionId": id, "operationalStatus": "BLOCKED"}); x != nil {
				return x
			}
		}
		ev := "inspection.completed.v1"
		if result == "CRITICAL_FAIL" {
			ev = "inspection.critical_failure.v1"
		}
		return s.record(ctx, tx, t, ev, "inspection", id, v.Version, v)
	})
	if e == nil && result == "CRITICAL_FAIL" && s.o.Alerts != nil {
		_ = s.o.Alerts.CreateSystemAlert(ctx, t.OrganizationID, "CRITICAL_INSPECTION", v.AssetID, id, "CRITICAL", "Asset blocked after critical inspection")
	}
	return v, e
}

func (s *Service) Evaluate(ctx context.Context, org, assetID, provider, requiredType string, requiredCapacity float64, service string, at time.Time) (Eligibility, error) {
	if s.o.Restrictions != nil {
		for _, subject := range [][2]string{{"ORGANIZATION", org}, {"ASSET", assetID}} {
			restricted, err := s.o.Restrictions.Restricted(ctx, org, subject[0], subject[1])
			if err != nil {
				return Eligibility{Eligible: false, HardBlocks: []string{"RESTRICTION_AUTHORITY_UNAVAILABLE"}}, err
			}
			if restricted {
				return Eligibility{Eligible: false, HardBlocks: []string{"COMPLIANCE_RESTRICTION_ACTIVE"}}, nil
			}
		}
	}
	if !s.o.Flags.Assets {
		return Eligibility{Eligible: true, Reasons: []string{"asset_policy_disabled"}}, nil
	}
	res := Eligibility{}
	var a Asset
	e := s.o.Pool.QueryRow(ctx, `SELECT `+assetCols+` FROM fleet_partner_assets WHERE organization_id=$1 AND(id=$2 OR platform_vehicle_id=$2)`, org, assetID).Scan(&a.ID, &a.OrganizationID, &a.BranchID, &a.AssetCode, &a.AssetType, &a.PlatformVehicleID, &a.OwnershipType, &a.RegistrationReference, &a.Manufacturer, &a.Model, &a.Year, &a.Capacity, &a.CapacityUnit, &a.Odometer, &a.OperatingHours, &a.OperationalStatus, &a.LifecycleStatus, &a.Version, &a.CreatedAt, &a.UpdatedAt, &a.ArchivedAt, &a.RetiredAt)
	if e != nil {
		return res, e
	}
	if a.LifecycleStatus != "ACTIVE" {
		res.HardBlocks = append(res.HardBlocks, "ASSET_"+a.LifecycleStatus)
	}
	if a.OperationalStatus == "BLOCKED" || a.OperationalStatus == "UNDER_MAINTENANCE" || a.OperationalStatus == "OUT_OF_SERVICE" {
		res.HardBlocks = append(res.HardBlocks, "ASSET_"+a.OperationalStatus)
	}
	if requiredType != "" && a.AssetType != requiredType {
		res.HardBlocks = append(res.HardBlocks, "ASSET_TYPE_MISMATCH")
	}
	if requiredCapacity > 0 && a.Capacity < requiredCapacity {
		res.HardBlocks = append(res.HardBlocks, "ASSET_CAPACITY_INSUFFICIENT")
	}
	var conflict bool
	_ = s.o.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_asset_assignments WHERE organization_id=$1 AND asset_id=$2 AND status='ACTIVE' AND($3=''OR provider_membership_id<>$3))`, org, a.ID, provider).Scan(&conflict)
	if conflict {
		res.HardBlocks = append(res.HardBlocks, "ASSET_ASSIGNMENT_CONFLICT")
	}
	if provider != "" {
		var authorized bool
		_ = s.o.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_asset_assignments WHERE organization_id=$1 AND asset_id=$2 AND provider_membership_id=$3 AND status IN('PLANNED','ACTIVE') AND start_at<=$4 AND(end_at IS NULL OR end_at>$4))`, org, a.ID, provider, at).Scan(&authorized)
		if !authorized {
			res.HardBlocks = append(res.HardBlocks, "PROVIDER_NOT_AUTHORIZED_FOR_ASSET")
		}
	}
	var expired bool
	_ = s.o.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_asset_documents WHERE organization_id=$1 AND asset_id=$2 AND(verification_status IN('EXPIRED','REVOKED','REJECTED') OR expires_at<=now()))`, org, a.ID).Scan(&expired)
	if expired {
		res.HardBlocks = append(res.HardBlocks, "ASSET_DOCUMENT_EXPIRED")
	}
	var overdue bool
	_ = s.o.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_maintenance_schedules WHERE organization_id=$1 AND asset_id=$2 AND status='ACTIVE' AND((next_due_date IS NOT NULL AND next_due_date<=CURRENT_DATE)OR(next_due_odometer IS NOT NULL AND next_due_odometer<=$3)OR(next_due_hours IS NOT NULL AND next_due_hours<=$4)))`, org, a.ID, a.Odometer, a.OperatingHours).Scan(&overdue)
	if overdue {
		res.HardBlocks = append(res.HardBlocks, "MAINTENANCE_OVERDUE")
	}
	res.Eligible = len(res.HardBlocks) == 0
	if res.Eligible {
		res.Reasons = append(res.Reasons, "ASSET_REQUIREMENTS_SATISFIED")
	}
	return res, nil
}
