// Package usecases implements the Section 18 fuel and energy workflows:
// recording fuel against an asset and approving or rejecting it. Fuel records
// are organization reporting data and never a wallet or ledger mutation.
package usecases

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	assetmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/assets/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/fuel/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

type EventPublisher interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}

// Assets is the slice of the asset feature fuel depends on.
type Assets interface {
	GetAsset(ctx context.Context, t tenant.Context, id string) (assetmodels.Asset, error)
	ValidateMedia(ctx context.Context, t tenant.Context, id string) error
}

// Repository is the persistence fuel depends on.
type Repository interface {
	InsertFuel(ctx context.Context, q database.Querier, v *models.FuelRecord) error
	GetFuel(ctx context.Context, org, id string) (models.FuelRecord, error)
	ListFuel(ctx context.Context, org string, limit int, after *time.Time, afterID string) ([]models.FuelRecord, error)
	DecideFuel(ctx context.Context, q database.Querier, org, id string, expected int64, to, actor string) (models.FuelRecord, error)
	InsertApprovalHistory(ctx context.Context, q database.Querier, org, id, previous, next, actor, reason string, version int64) error
}

type Options struct {
	Pool       *pgxpool.Pool
	Repository Repository
	Assets     Assets
	Events     EventPublisher
	Audit      *audit.Recorder
	Flags      assetmodels.Flags
	Now        func() time.Time
}
type Service struct {
	o    Options
	repo Repository
}

// MaxPageSize bounds a fuel list page.
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
func (s *Service) record(ctx context.Context, tx pgx.Tx, t tenant.Context, event, kind, id string, v int64, p any) error {
	if e := s.o.Audit.Record(ctx, tx, audit.EntryFromTenant(t, strings.TrimSuffix(event, ".v1"), kind, id)); e != nil {
		return e
	}
	if s.o.Events != nil {
		return s.o.Events.Publish(ctx, tx, event, kind, id, v, t.OrganizationID, t.CorrelationID, p)
	}
	return nil
}

func (s *Service) CreateFuel(ctx context.Context, t tenant.Context, v models.FuelRecord) (models.FuelRecord, error) {
	if e := s.gate(s.o.Flags.Fuel, true); e != nil {
		return v, e
	}
	v.FuelOrEnergyType = strings.ToUpper(v.FuelOrEnergyType)
	v.Unit = strings.ToUpper(v.Unit)
	if !models.ValidQuantity(v.Quantity, v.FuelOrEnergyType, v.Unit) {
		return v, apperrors.BadRequest("Fuel quantity, type or unit is invalid.", nil)
	}
	a, e := s.o.Assets.GetAsset(ctx, t, v.AssetID)
	if e != nil {
		return v, e
	}
	if v.Odometer != nil && *v.Odometer < a.Odometer || v.OperatingHours != nil && *v.OperatingHours < a.OperatingHours {
		return v, apperrors.Conflict("Odometer or operating hours cannot regress.", nil)
	}
	if v.ReceiptMediaID != "" {
		if e = s.o.Assets.ValidateMedia(ctx, t, v.ReceiptMediaID); e != nil {
			return v, e
		}
	}
	v.ID = ids.New("fur")
	v.OrganizationID = t.OrganizationID
	if v.RecordedAt.IsZero() {
		v.RecordedAt = s.o.Now()
	}
	e = database.WithinTx(ctx, s.o.Pool, func(tx pgx.Tx) error {
		if e := s.repo.InsertFuel(ctx, tx, &v); e != nil {
			return e
		}
		return s.record(ctx, tx, t, "fuel_record.created.v1", "fuel_record", v.ID, v.Version, map[string]any{"fuelRecord": v, "walletMutation": false, "ledgerMutation": false, "automaticProviderPenalty": false})
	})
	return v, e
}
func (s *Service) GetFuel(ctx context.Context, t tenant.Context, id string) (models.FuelRecord, error) {
	if e := s.gate(s.o.Flags.Fuel, false); e != nil {
		return models.FuelRecord{}, e
	}
	v, e := s.repo.GetFuel(ctx, t.OrganizationID, id)
	if errors.Is(e, pgx.ErrNoRows) {
		return v, apperrors.NotFound("Resource not found.", nil)
	}
	if e == nil {
		_, e = s.o.Assets.GetAsset(ctx, t, v.AssetID)
	}
	return v, e
}

// ListFuel returns the first page of fuel records.
func (s *Service) ListFuel(ctx context.Context, t tenant.Context) ([]models.FuelRecord, error) {
	out, _, e := s.ListFuelPage(ctx, t, models.Page{})
	return out, e
}

// ListFuelPage returns one keyset page of the fuel records the caller can see
// (newest recordedAt first) and, when more rows exist, the last row read.
// Records on assets outside the caller's branches are skipped without
// affecting the cursor.
func (s *Service) ListFuelPage(ctx context.Context, t tenant.Context, p models.Page) ([]models.FuelRecord, *models.FuelRecord, error) {
	if e := s.gate(s.o.Flags.Fuel, false); e != nil {
		return nil, nil, e
	}
	size := p.Limit
	if size <= 0 || size > MaxPageSize {
		size = MaxPageSize
	}
	rows, e := s.repo.ListFuel(ctx, t.OrganizationID, size+1, p.After, p.AfterID)
	if e != nil {
		return nil, nil, e
	}
	var last *models.FuelRecord
	if len(rows) > size {
		rows = rows[:size]
		last = &rows[size-1]
	}
	out := []models.FuelRecord{}
	for _, v := range rows {
		if _, e = s.o.Assets.GetAsset(ctx, t, v.AssetID); e != nil {
			continue
		}
		out = append(out, v)
	}
	return out, last, nil
}
func (s *Service) FuelAction(ctx context.Context, t tenant.Context, id, to, reason string, expected int64) (models.FuelRecord, error) {
	old, e := s.GetFuel(ctx, t, id)
	if e != nil {
		return old, e
	}
	if old.ApprovalStatus != "PENDING" || expected <= 0 {
		return old, apperrors.Conflict("Fuel record is already decided or version is missing.", nil)
	}
	var v models.FuelRecord
	e = database.WithinTx(ctx, s.o.Pool, func(tx pgx.Tx) error {
		var x error
		v, x = s.repo.DecideFuel(ctx, tx, t.OrganizationID, id, expected, to, t.OrganizationUserID)
		if x != nil {
			return apperrors.Conflict("Fuel approval conflict.", x)
		}
		if x = s.repo.InsertApprovalHistory(ctx, tx, t.OrganizationID, id, old.ApprovalStatus, to, t.ActorID, reason, v.Version); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "fuel_record."+strings.ToLower(to)+".v1", "fuel_record", id, v.Version, v)
	})
	return v, e
}
