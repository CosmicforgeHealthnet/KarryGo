// Package usecases implements the Section 18 asset workflows: the asset
// register and lifecycle, provider assignments, documents, eligibility,
// meter readings, operating costs, breakdowns, parts and stock movements,
// defects, tyres, the asset safety-block primitives other Section 18
// features compose into their own transactions, and the section's
// maintenance job.
package usecases

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/assets/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/pagination"
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

// WorkOrders is the slice of the maintenance feature assets depend on: the
// repair work orders that defects and breakdowns raise in the same
// transaction, and work-order visibility for stock issues.
type WorkOrders interface {
	CreateSystemWorkOrderTx(ctx context.Context, tx pgx.Tx, org, assetID, sourceType, sourceRef, priority, requiredWork string) (string, int64, error)
	EnsureSystemWorkOrderTx(ctx context.Context, tx pgx.Tx, org, assetID, sourceType, sourceRef, priority, requiredWork string) (int64, bool, error)
	WorkOrderVisible(ctx context.Context, t tenant.Context, id string) error
}

// DueSchedules turns due preventive-maintenance schedules into work orders.
type DueSchedules interface {
	RunDueSchedules(ctx context.Context, organizationID string) error
}

// SafetyDeclines turns unsafe-vehicle declines into the inspection workflow.
type SafetyDeclines interface {
	ReconcileSafetyDeclines(ctx context.Context) error
}

// Collaborators are the Section 18 features this one calls. They depend on
// the asset service themselves, so the composition root sets them after
// construction.
type Collaborators struct {
	WorkOrders     WorkOrders
	DueSchedules   DueSchedules
	SafetyDeclines SafetyDeclines
}

type Options struct {
	Pool         *pgxpool.Pool
	Repository   Repository
	Events       EventPublisher
	Audit        *audit.Recorder
	Media        platformclients.MediaContract
	Alerts       AlertSink
	Restrictions RestrictionGate
	Queue        *jobs.Queue
	Flags        models.Flags
	Now          func() time.Time
}
type Service struct {
	o    Options
	repo Repository
	c    Collaborators
}

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
	return &Service{o: o, repo: o.Repository}
}
func (s *Service) SetAlertSink(alerts AlertSink)    { s.o.Alerts = alerts }
func (s *Service) SetCollaborators(c Collaborators) { s.c = c }
func (s *Service) Flags() models.Flags              { return s.o.Flags }
func uniqueViolation(e error) bool {
	var pg *pgconn.PgError
	return errors.As(e, &pg) && pg.Code == "23505"
}
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
func (s *Service) alert(ctx context.Context, org, kind, subject, ref, severity, message string) {
	if s.o.Alerts != nil {
		_ = s.o.Alerts.CreateSystemAlert(ctx, org, kind, subject, ref, severity, message)
	}
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

// ActiveMembership reports an ACTIVE provider membership of the organization.
func (s *Service) ActiveMembership(ctx context.Context, org, id string) bool {
	return s.repo.ActiveMembership(ctx, org, id)
}

// ValidateMedia requires a ready Media object owned by the organization.
func (s *Service) ValidateMedia(ctx context.Context, t tenant.Context, id string) error {
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

func (s *Service) CreateAsset(ctx context.Context, t tenant.Context, v models.Asset) (models.Asset, error) {
	if e := s.gate(s.o.Flags.Assets, true); e != nil {
		return v, e
	}
	if !t.BranchPermitted(v.BranchID) || v.AssetCode == "" || v.AssetType == "" {
		return v, apperrors.BadRequest("Branch, asset code and type are required.", nil)
	}
	v.ID = ids.New("ast")
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var e error
		v, e = s.repo.InsertAsset(ctx, tx, t.OrganizationID, v)
		if e != nil {
			return e
		}
		return s.record(ctx, tx, t, "asset.created.v1", "asset", v.ID, v.Version, v)
	})
	return v, e
}
func (s *Service) GetAsset(ctx context.Context, t tenant.Context, id string) (models.Asset, error) {
	if e := s.gate(s.o.Flags.Assets, false); e != nil {
		return models.Asset{}, e
	}
	v, e := s.repo.GetAsset(ctx, t.OrganizationID, id)
	if e != nil || !t.BranchPermitted(v.BranchID) {
		return v, apperrors.NotFound("Resource not found.", e)
	}
	return v, nil
}

// ListAssets returns one page of assets (offset or keyset).
func (s *Service) ListAssets(ctx context.Context, t tenant.Context, f models.Page) ([]models.Asset, error) {
	out, _, e := s.ListAssetsPage(ctx, t, f)
	return out, e
}

// ListAssetsPage returns one page of assets newest first and whether more
// exist. With f.After set it continues from that keyset position; otherwise
// it honours the compatibility offset.
func (s *Service) ListAssetsPage(ctx context.Context, t tenant.Context, f models.Page) ([]models.Asset, bool, error) {
	if e := s.gate(s.o.Flags.Assets, false); e != nil {
		return nil, false, e
	}
	size := lim(f.Limit)
	offset := pagination.ClampOffset(f.Offset)
	if f.After != nil {
		offset = 0
	}
	rows, e := s.repo.ListAssets(ctx, t.OrganizationID, f, t.PermittedBranchIDs, size+1, offset)
	if e != nil {
		return nil, false, e
	}
	more := len(rows) > size
	if more {
		rows = rows[:size]
	}
	out := []models.Asset{}
	for _, v := range rows {
		if t.BranchPermitted(v.BranchID) {
			out = append(out, v)
		}
	}
	return out, more, nil
}
func (s *Service) UpdateAsset(ctx context.Context, t tenant.Context, id string, expected int64, v models.Asset, action string) (models.Asset, error) {
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
		if active, _ := s.repo.ActiveAssignmentExists(ctx, nil, t.OrganizationID, id); active {
			return v, apperrors.Conflict("Asset has an active assignment.", nil)
		}
	}
	e = s.tx(ctx, func(tx pgx.Tx) error {
		if action != "" {
			lock, x := s.repo.LockAsset(ctx, tx, t.OrganizationID, id)
			if x != nil {
				return x
			}
			if lock.Version != expected {
				return apperrors.Conflict("Version conflict.", nil)
			}
			active, x := s.repo.ActiveAssignmentExists(ctx, tx, t.OrganizationID, id)
			if x != nil {
				return x
			}
			if active {
				return apperrors.Conflict("Asset has an active assignment.", nil)
			}
		}
		var x error
		v, x = s.repo.UpdateAsset(ctx, tx, t.OrganizationID, id, expected, v, life)
		if x != nil {
			return apperrors.Conflict("Version conflict.", x)
		}
		if x = s.repo.InsertAssetHistory(ctx, tx, t.OrganizationID, id, op, v.OperationalStatus, old.LifecycleStatus, v.LifecycleStatus, action, t.ActorID, v.Version, v); x != nil {
			return x
		}
		return s.record(ctx, tx, t, ev, "asset", id, v.Version, v)
	})
	return v, e
}

func (s *Service) Assign(ctx context.Context, t tenant.Context, assetID string, v models.Assignment) (models.Assignment, error) {
	if e := s.gate(s.o.Flags.Assignments, true); e != nil {
		return v, e
	}
	if _, e := s.GetAsset(ctx, t, assetID); e != nil {
		return v, e
	}
	el, e := s.Evaluate(ctx, t.OrganizationID, assetID, "", "", 0, "", v.StartAt)
	if e != nil || !el.Eligible {
		return v, apperrors.Conflict("Asset is not eligible for assignment.", e)
	}
	if !s.repo.ActiveMembership(ctx, t.OrganizationID, v.ProviderMembershipID) {
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
		lock, x := s.repo.LockAsset(ctx, tx, t.OrganizationID, assetID)
		if x != nil {
			return x
		}
		if lock.LifecycleStatus != "ACTIVE" || lock.OperationalStatus != "AVAILABLE" {
			return apperrors.Conflict("Asset is no longer available for assignment.", nil)
		}
		if e := s.repo.InsertAssignment(ctx, tx, &v); e != nil {
			return apperrors.Conflict("Asset assignment conflict.", e)
		}
		if e := s.repo.MarkAssigned(ctx, tx, assetID); e != nil {
			return e
		}
		return s.record(ctx, tx, t, "asset.assigned.v1", "asset_assignment", v.ID, v.Version, v)
	})
	return v, e
}
func (s *Service) AddDocument(ctx context.Context, t tenant.Context, assetID string, v models.Document) (models.Document, error) {
	if e := s.gate(s.o.Flags.Assets, true); e != nil {
		return v, e
	}
	if _, e := s.GetAsset(ctx, t, assetID); e != nil {
		return v, e
	}
	if e := s.ValidateMedia(ctx, t, v.MediaFileID); e != nil {
		return v, e
	}
	v.ID = ids.New("asd")
	v.OrganizationID = t.OrganizationID
	v.AssetID = assetID
	v.VerificationStatus = "PENDING"
	e := s.tx(ctx, func(tx pgx.Tx) error {
		if x := s.repo.InsertDocument(ctx, tx, &v); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "asset.document_added.v1", "asset_document", v.ID, v.Version, v)
	})
	return v, e
}
func (s *Service) ListDocuments(ctx context.Context, t tenant.Context, assetID string) ([]models.Document, error) {
	if _, e := s.GetAsset(ctx, t, assetID); e != nil {
		return nil, e
	}
	return s.repo.ListDocuments(ctx, t.OrganizationID, assetID)
}

// Evaluate decides whether an asset may take company work: compliance
// restrictions, lifecycle and operational state, type and capacity fit,
// assignment conflicts and authorization, document expiry and overdue
// maintenance.
func (s *Service) Evaluate(ctx context.Context, org, assetID, provider, requiredType string, requiredCapacity float64, service string, at time.Time) (models.Eligibility, error) {
	if s.o.Restrictions != nil {
		for _, subject := range [][2]string{{"ORGANIZATION", org}, {"ASSET", assetID}} {
			restricted, err := s.o.Restrictions.Restricted(ctx, org, subject[0], subject[1])
			if err != nil {
				return models.Eligibility{Eligible: false, HardBlocks: []string{"RESTRICTION_AUTHORITY_UNAVAILABLE"}}, err
			}
			if restricted {
				return models.Eligibility{Eligible: false, HardBlocks: []string{"COMPLIANCE_RESTRICTION_ACTIVE"}}, nil
			}
		}
	}
	if !s.o.Flags.Assets {
		return models.Eligibility{Eligible: true, Reasons: []string{"asset_policy_disabled"}}, nil
	}
	res := models.Eligibility{}
	a, e := s.repo.FindAssetByReference(ctx, org, assetID)
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
	if s.repo.AssignmentConflict(ctx, org, a.ID, provider) {
		res.HardBlocks = append(res.HardBlocks, "ASSET_ASSIGNMENT_CONFLICT")
	}
	if provider != "" && !s.repo.ProviderAuthorized(ctx, org, a.ID, provider, at) {
		res.HardBlocks = append(res.HardBlocks, "PROVIDER_NOT_AUTHORIZED_FOR_ASSET")
	}
	if s.repo.DocumentExpired(ctx, org, a.ID) {
		res.HardBlocks = append(res.HardBlocks, "ASSET_DOCUMENT_EXPIRED")
	}
	if s.repo.MaintenanceOverdue(ctx, org, a.ID, a.Odometer, a.OperatingHours) {
		res.HardBlocks = append(res.HardBlocks, "MAINTENANCE_OVERDUE")
	}
	res.Eligible = len(res.HardBlocks) == 0
	if res.Eligible {
		res.Reasons = append(res.Reasons, "ASSET_REQUIREMENTS_SATISFIED")
	}
	return res, nil
}
