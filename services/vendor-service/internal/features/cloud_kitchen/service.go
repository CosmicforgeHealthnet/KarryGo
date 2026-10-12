package cloudkitchen

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/production"
	"cosmicforge/logistics/shared/go/apperrors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	PermRead   = "vendor.production.read"
	PermManage = "vendor.cloud_kitchen.manage"
)

type Service struct {
	db         *pgxpool.Pool
	production *production.Service
}

func NewService(db *pgxpool.Pool, p *production.Service) *Service {
	return &Service{db: db, production: p}
}

func snapshotPackage(ctx context.Context, tx pgx.Tx, packageID, reason string) error {
	_, err := tx.Exec(ctx, `INSERT INTO cloud_kitchen_package_versions(id,package_id,version,definition,reason)
		SELECT $1,p.id,p.version,jsonb_build_object(
			'package',to_jsonb(p),
			'components',COALESCE((SELECT jsonb_agg(to_jsonb(c) ORDER BY c.position,c.id) FROM cloud_kitchen_package_components c WHERE c.package_id=p.id),'[]'::jsonb),
			'bands',COALESCE((SELECT jsonb_agg(to_jsonb(b) ORDER BY b.min_quantity,b.id) FROM cloud_kitchen_quantity_bands b WHERE b.package_id=p.id),'[]'::jsonb)
		),$3 FROM cloud_kitchen_packages p WHERE p.id=$2`, "ckpv_"+uuid.NewString(), packageID, reason)
	return err
}
func (s *Service) DB() *pgxpool.Pool               { return s.db }
func (s *Service) Production() *production.Service { return s.production }
func (s *Service) Actor(ctx context.Context, staffID, vendorID string) (production.Actor, production.Scope, error) {
	a := production.Actor{StaffID: staffID, VendorID: vendorID}
	scope, err := s.production.Authorize(ctx, a, PermManage)
	if err != nil {
		scope, err = s.production.Authorize(ctx, a, PermRead)
	}
	return a, scope, err
}

type Settings struct {
	BranchID                        string  `json:"branch_id"`
	BulkOrderThreshold              int     `json:"bulk_order_threshold"`
	MinimumLeadTimeMinutes          int     `json:"minimum_lead_time_minutes"`
	DefaultCapacityPoolRef          *string `json:"default_capacity_pool_ref"`
	ReservationHoldDurationMinutes  int     `json:"reservation_hold_duration_minutes"`
	CorporateReservationHorizonDays int     `json:"corporate_reservation_horizon_days"`
	Version                         int64   `json:"version"`
}

func (s *Service) GetSettings(ctx context.Context, business, branch string) (Settings, error) {
	var v Settings
	v.BranchID = branch
	err := s.db.QueryRow(ctx, `SELECT bulk_order_threshold,minimum_lead_time_minutes,default_capacity_pool_ref,reservation_hold_duration_minutes,corporate_reservation_horizon_days,version FROM cloud_kitchen_settings WHERE business_id=$1 AND branch_id=$2`, business, branch).Scan(&v.BulkOrderThreshold, &v.MinimumLeadTimeMinutes, &v.DefaultCapacityPoolRef, &v.ReservationHoldDurationMinutes, &v.CorporateReservationHorizonDays, &v.Version)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return Settings{BranchID: branch, BulkOrderThreshold: 20, MinimumLeadTimeMinutes: 60, ReservationHoldDurationMinutes: 30, CorporateReservationHorizonDays: 90, Version: 0}, nil
		}
		return v, err
	}
	return v, nil
}
func (s *Service) PutSettings(ctx context.Context, business string, v Settings) (Settings, error) {
	if v.BulkOrderThreshold <= 0 || v.BulkOrderThreshold > production.MaxCapacity || v.MinimumLeadTimeMinutes < 0 || v.MinimumLeadTimeMinutes > 30*24*60 || v.ReservationHoldDurationMinutes <= 0 || v.ReservationHoldDurationMinutes > 24*60 || v.CorporateReservationHorizonDays <= 0 || v.CorporateReservationHorizonDays > 366 {
		return v, fmt.Errorf("invalid cloud kitchen settings")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return v, err
	}
	defer tx.Rollback(ctx)
	if err = s.production.ValidateCapacityPoolReference(ctx, tx, business, v.BranchID, "CLOUD_KITCHEN", v.DefaultCapacityPoolRef); err != nil {
		return v, err
	}
	var before Settings
	_ = tx.QueryRow(ctx, `SELECT bulk_order_threshold,minimum_lead_time_minutes,default_capacity_pool_ref,reservation_hold_duration_minutes,corporate_reservation_horizon_days,version FROM cloud_kitchen_settings WHERE business_id=$1 AND branch_id=$2 FOR UPDATE`, business, v.BranchID).Scan(&before.BulkOrderThreshold, &before.MinimumLeadTimeMinutes, &before.DefaultCapacityPoolRef, &before.ReservationHoldDurationMinutes, &before.CorporateReservationHorizonDays, &before.Version)
	if v.Version != before.Version {
		return v, fmt.Errorf("stale cloud kitchen settings version")
	}
	var tag pgconn.CommandTag
	if before.Version == 0 {
		tag, err = tx.Exec(ctx, `INSERT INTO cloud_kitchen_settings(business_id,branch_id,bulk_order_threshold,minimum_lead_time_minutes,default_capacity_pool_ref,reservation_hold_duration_minutes,corporate_reservation_horizon_days) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(business_id,branch_id) DO NOTHING`, business, v.BranchID, v.BulkOrderThreshold, v.MinimumLeadTimeMinutes, v.DefaultCapacityPoolRef, v.ReservationHoldDurationMinutes, v.CorporateReservationHorizonDays)
	} else {
		tag, err = tx.Exec(ctx, `UPDATE cloud_kitchen_settings SET bulk_order_threshold=$3,minimum_lead_time_minutes=$4,default_capacity_pool_ref=$5,reservation_hold_duration_minutes=$6,corporate_reservation_horizon_days=$7,version=version+1,updated_at=now() WHERE business_id=$1 AND branch_id=$2 AND version=$8`, business, v.BranchID, v.BulkOrderThreshold, v.MinimumLeadTimeMinutes, v.DefaultCapacityPoolRef, v.ReservationHoldDurationMinutes, v.CorporateReservationHorizonDays, before.Version)
	}
	if err != nil {
		return v, err
	}
	if tag.RowsAffected() == 0 {
		return v, fmt.Errorf("stale cloud kitchen settings version")
	}
	if s.production != nil {
		ver := before.Version + 1
		if ver == 1 && before.Version == 0 {
			ver = 1
		}
		if err = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "cloud_kitchen.settings.updated", "cloud_kitchen.settings.updated", "cloud_kitchen_settings", business+":"+v.BranchID, v.BranchID, ver, before, v, map[string]any{"branch_id": v.BranchID}, ""); err != nil {
			return v, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return v, err
	}
	return s.GetSettings(ctx, business, v.BranchID)
}

type Package struct {
	ID                   string         `json:"id"`
	BranchID             string         `json:"branch_id"`
	Name                 string         `json:"name"`
	Description          string         `json:"description"`
	Status               string         `json:"status"`
	MinimumQuantity      int            `json:"minimum_quantity"`
	MaximumQuantity      *int           `json:"maximum_quantity"`
	CapacityPoolID       *string        `json:"capacity_pool_id"`
	ProductionClass      *string        `json:"production_class"`
	LeadTimeMinutes      *int           `json:"lead_time_minutes"`
	SetupBufferMinutes   int            `json:"setup_buffer_minutes"`
	CleanupBufferMinutes int            `json:"cleanup_buffer_minutes"`
	Logistics            map[string]any `json:"logistics"`
	Version              int64          `json:"version"`
}

func (s *Service) ListPackages(ctx context.Context, business string, scope production.Scope) ([]Package, error) {
	rows, err := s.db.Query(ctx, `SELECT id,branch_id,name,description,status,minimum_quantity,maximum_quantity,capacity_pool_id,production_class,lead_time_minutes,setup_buffer_minutes,cleanup_buffer_minutes,logistics,version FROM cloud_kitchen_packages WHERE business_id=$1 AND ($2 OR branch_id=ANY($3)) ORDER BY branch_id,name`, business, scope.Global, scope.BranchIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Package{}
	for rows.Next() {
		var p Package
		var raw []byte
		if err := rows.Scan(&p.ID, &p.BranchID, &p.Name, &p.Description, &p.Status, &p.MinimumQuantity, &p.MaximumQuantity, &p.CapacityPoolID, &p.ProductionClass, &p.LeadTimeMinutes, &p.SetupBufferMinutes, &p.CleanupBufferMinutes, &raw, &p.Version); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &p.Logistics)
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Service) CreatePackage(ctx context.Context, business string, p Package) (string, error) {
	id := uuid.NewString()
	logistics, err := json.Marshal(p.Logistics)
	if err != nil {
		return "", err
	}
	if p.MinimumQuantity <= 0 || p.SetupBufferMinutes < 0 || p.SetupBufferMinutes > production.MaxBufferMinutes || p.CleanupBufferMinutes < 0 || p.CleanupBufferMinutes > production.MaxBufferMinutes {
		return "", fmt.Errorf("minimum_quantity must be positive")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if err = s.production.ValidateCapacityPoolReference(ctx, tx, business, p.BranchID, "CLOUD_KITCHEN", p.CapacityPoolID); err != nil {
		return "", err
	}
	status := func() string {
		if p.Status == "" {
			return "DRAFT"
		}
		return p.Status
	}()
	if _, err = tx.Exec(ctx, `INSERT INTO cloud_kitchen_packages(id,business_id,branch_id,name,description,status,minimum_quantity,maximum_quantity,capacity_pool_id,production_class,lead_time_minutes,setup_buffer_minutes,cleanup_buffer_minutes,logistics) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, id, business, p.BranchID, p.Name, p.Description, status, p.MinimumQuantity, p.MaximumQuantity, p.CapacityPoolID, p.ProductionClass, p.LeadTimeMinutes, p.SetupBufferMinutes, p.CleanupBufferMinutes, logistics); err != nil {
		return "", err
	}
	if err = snapshotPackage(ctx, tx, id, "CREATED"); err != nil {
		return "", err
	}
	if s.production != nil {
		if err = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "cloud_kitchen.package.created", "cloud_kitchen.package.created", "cloud_kitchen_package", id, p.BranchID, 1, nil, p, map[string]any{"id": id}, ""); err != nil {
			return "", err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}
func (s *Service) AddBand(ctx context.Context, packageID string, min int, max *int, unit, pack int64, lead *int, capacityUnitsPerItem int, expectedVersion int64) (string, error) {
	if capacityUnitsPerItem == 0 {
		capacityUnitsPerItem = 1
	}
	if min <= 0 || (max != nil && *max < min) || unit < 0 || pack < 0 || capacityUnitsPerItem < 1 || capacityUnitsPerItem > 1000 {
		return "", fmt.Errorf("invalid quantity band")
	}
	id := uuid.NewString()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var business, branch, status string
	var packageVersion int64
	if err = tx.QueryRow(ctx, `SELECT business_id,branch_id,status,version FROM cloud_kitchen_packages WHERE id=$1 FOR UPDATE`, packageID).Scan(&business, &branch, &status, &packageVersion); err != nil {
		return "", err
	}
	if status == "ARCHIVED" || expectedVersion <= 0 || expectedVersion != packageVersion {
		return "", fmt.Errorf("stale package version")
	}
	var overlap bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_kitchen_quantity_bands WHERE package_id=$1 AND status='ACTIVE' AND min_quantity <= COALESCE($2,2147483647) AND COALESCE(max_quantity,2147483647) >= $3)`, packageID, max, min).Scan(&overlap); err != nil {
		return "", err
	}
	if overlap {
		return "", fmt.Errorf("quantity band overlaps existing band")
	}
	if _, err = tx.Exec(ctx, `INSERT INTO cloud_kitchen_quantity_bands(id,package_id,min_quantity,max_quantity,unit_price_minor,package_price_minor,lead_time_minutes,capacity_units_per_item) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, packageID, min, max, unit, pack, lead, capacityUnitsPerItem); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO cloud_kitchen_price_history(id,package_id,band_id,unit_price_minor,package_price_minor,version) SELECT $1,$2,$3,$4,$5,version FROM cloud_kitchen_quantity_bands WHERE id=$3`, uuid.NewString(), packageID, id, unit, pack); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `UPDATE cloud_kitchen_packages SET version=version+1,updated_at=now() WHERE id=$1 AND version=$2`, packageID, packageVersion); err != nil {
		return "", err
	}
	if err = snapshotPackage(ctx, tx, packageID, "BAND_UPDATED"); err != nil {
		return "", err
	}
	if s.production != nil {
		if err = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "cloud_kitchen.quantity_band.created", "cloud_kitchen.quantity_band.created", "cloud_kitchen_quantity_band", id, branch, 1, nil, map[string]any{"package_id": packageID, "min_quantity": min, "max_quantity": max, "unit_price_minor": unit}, map[string]any{"package_id": packageID, "min_quantity": min, "max_quantity": max, "unit_price_minor": unit}, ""); err != nil {
			return "", err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Service) UpdateBand(ctx context.Context, business, packageID, bandID string, min int, max *int, unit, pack int64, lead *int, capacityUnitsPerItem int, expectedPackageVersion, expectedBandVersion int64) error {
	if min <= 0 || (max != nil && *max < min) || unit < 0 || pack < 0 || capacityUnitsPerItem < 1 || capacityUnitsPerItem > 1000 {
		return fmt.Errorf("invalid quantity band")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var branch, packageStatus string
	var packageVersion int64
	if err = tx.QueryRow(ctx, `SELECT branch_id,status,version FROM cloud_kitchen_packages WHERE business_id=$1 AND id=$2 FOR UPDATE`, business, packageID).Scan(&branch, &packageStatus, &packageVersion); err != nil {
		return err
	}
	if packageStatus == "ARCHIVED" || expectedPackageVersion <= 0 || expectedPackageVersion != packageVersion {
		return fmt.Errorf("stale package version")
	}
	var bandStatus string
	var bandVersion int64
	if err = tx.QueryRow(ctx, `SELECT status,version FROM cloud_kitchen_quantity_bands WHERE package_id=$1 AND id=$2 FOR UPDATE`, packageID, bandID).Scan(&bandStatus, &bandVersion); err != nil {
		return err
	}
	if bandStatus != "ACTIVE" || expectedBandVersion <= 0 || expectedBandVersion != bandVersion {
		return fmt.Errorf("stale quantity band version")
	}
	var overlap bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM cloud_kitchen_quantity_bands WHERE package_id=$1 AND id<>$2 AND status='ACTIVE' AND min_quantity <= COALESCE($3,2147483647) AND COALESCE(max_quantity,2147483647) >= $4)`, packageID, bandID, max, min).Scan(&overlap); err != nil {
		return err
	}
	if overlap {
		return fmt.Errorf("quantity band overlaps existing band")
	}
	if _, err = tx.Exec(ctx, `UPDATE cloud_kitchen_quantity_bands SET min_quantity=$1,max_quantity=$2,unit_price_minor=$3,package_price_minor=$4,lead_time_minutes=$5,capacity_units_per_item=$6,version=version+1 WHERE package_id=$7 AND id=$8 AND version=$9 AND status='ACTIVE'`, min, max, unit, pack, lead, capacityUnitsPerItem, packageID, bandID, bandVersion); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO cloud_kitchen_price_history(id,package_id,band_id,unit_price_minor,package_price_minor,version) VALUES($1,$2,$3,$4,$5,$6)`, uuid.NewString(), packageID, bandID, unit, pack, bandVersion+1); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE cloud_kitchen_packages SET version=version+1,updated_at=now() WHERE id=$1 AND version=$2`, packageID, packageVersion); err != nil {
		return err
	}
	if err = snapshotPackage(ctx, tx, packageID, "BAND_UPDATED"); err != nil {
		return err
	}
	if s.production != nil {
		if err = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "cloud_kitchen.quantity_band.updated", "cloud_kitchen.quantity_band.updated", "cloud_kitchen_quantity_band", bandID, branch, bandVersion+1, map[string]any{"version": bandVersion}, map[string]any{"min_quantity": min, "max_quantity": max, "unit_price_minor": unit, "version": bandVersion + 1}, map[string]any{"package_id": packageID, "package_version": packageVersion + 1}, ""); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Service) ArchiveBand(ctx context.Context, business, packageID, bandID string, expectedPackageVersion, expectedBandVersion int64) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var branch, packageStatus string
	var packageVersion int64
	if err = tx.QueryRow(ctx, `SELECT branch_id,status,version FROM cloud_kitchen_packages WHERE business_id=$1 AND id=$2 FOR UPDATE`, business, packageID).Scan(&branch, &packageStatus, &packageVersion); err != nil {
		return err
	}
	if packageStatus == "ARCHIVED" || expectedPackageVersion <= 0 || expectedPackageVersion != packageVersion {
		return fmt.Errorf("stale package version")
	}
	var bandVersion int64
	if err = tx.QueryRow(ctx, `SELECT version FROM cloud_kitchen_quantity_bands WHERE package_id=$1 AND id=$2 AND status='ACTIVE' FOR UPDATE`, packageID, bandID).Scan(&bandVersion); err != nil {
		return err
	}
	if expectedBandVersion <= 0 || expectedBandVersion != bandVersion {
		return fmt.Errorf("stale quantity band version")
	}
	if _, err = tx.Exec(ctx, `UPDATE cloud_kitchen_quantity_bands SET status='ARCHIVED',archived_at=now(),version=version+1 WHERE package_id=$1 AND id=$2 AND version=$3 AND status='ACTIVE'`, packageID, bandID, bandVersion); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE cloud_kitchen_packages SET version=version+1,updated_at=now() WHERE id=$1 AND version=$2`, packageID, packageVersion); err != nil {
		return err
	}
	if err = snapshotPackage(ctx, tx, packageID, "BAND_ARCHIVED"); err != nil {
		return err
	}
	if s.production != nil {
		if err = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "cloud_kitchen.quantity_band.archived", "cloud_kitchen.quantity_band.archived", "cloud_kitchen_quantity_band", bandID, branch, bandVersion+1, map[string]any{"status": "ACTIVE"}, map[string]any{"status": "ARCHIVED"}, map[string]any{"package_id": packageID, "package_version": packageVersion + 1}, ""); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Service) UpdatePackage(ctx context.Context, business, id string, p Package, expected int64) error {
	logistics, err := json.Marshal(p.Logistics)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var branch, status string
	var version int64
	if err = tx.QueryRow(ctx, `SELECT branch_id,status,version FROM cloud_kitchen_packages WHERE business_id=$1 AND id=$2 FOR UPDATE`, business, id).Scan(&branch, &status, &version); err != nil {
		return err
	}
	if status == "ARCHIVED" {
		return fmt.Errorf("package is archived")
	}
	if expected <= 0 || expected != version {
		return fmt.Errorf("stale package version")
	}
	if p.Status == "" {
		p.Status = status
	}
	if p.Status != "DRAFT" && p.Status != "ACTIVE" {
		return fmt.Errorf("invalid package status")
	}
	if p.SetupBufferMinutes < 0 || p.SetupBufferMinutes > production.MaxBufferMinutes || p.CleanupBufferMinutes < 0 || p.CleanupBufferMinutes > production.MaxBufferMinutes {
		return fmt.Errorf("invalid package buffers")
	}
	if err = s.production.ValidateCapacityPoolReference(ctx, tx, business, branch, "CLOUD_KITCHEN", p.CapacityPoolID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE cloud_kitchen_packages SET name=$1,description=$2,status=$3,minimum_quantity=$4,maximum_quantity=$5,capacity_pool_id=$6,production_class=$7,lead_time_minutes=$8,setup_buffer_minutes=$9,cleanup_buffer_minutes=$10,logistics=$11,updated_at=now(),version=version+1 WHERE business_id=$12 AND id=$13 AND version=$14`, p.Name, p.Description, p.Status, p.MinimumQuantity, p.MaximumQuantity, p.CapacityPoolID, p.ProductionClass, p.LeadTimeMinutes, p.SetupBufferMinutes, p.CleanupBufferMinutes, logistics, business, id, version); err != nil {
		return err
	}
	if err = snapshotPackage(ctx, tx, id, "CONFIG_UPDATED"); err != nil {
		return err
	}
	if s.production != nil {
		if err = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "cloud_kitchen.package.updated", "cloud_kitchen.package.updated", "cloud_kitchen_package", id, branch, version+1, nil, p, map[string]any{"id": id}, ""); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Service) ArchivePackage(ctx context.Context, business, id string, expectedVersion int64) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var branch string
	var version int64
	if err = tx.QueryRow(ctx, `SELECT branch_id,version FROM cloud_kitchen_packages WHERE business_id=$1 AND id=$2 FOR UPDATE`, business, id).Scan(&branch, &version); err != nil {
		return err
	}
	if expectedVersion <= 0 || expectedVersion != version {
		return fmt.Errorf("stale package version")
	}
	if _, err = tx.Exec(ctx, `UPDATE cloud_kitchen_packages SET status='ARCHIVED',archived_at=now(),updated_at=now(),version=version+1 WHERE business_id=$1 AND id=$2 AND version=$3 AND status<>'ARCHIVED'`, business, id, version); err != nil {
		return err
	}
	if err = snapshotPackage(ctx, tx, id, "ARCHIVED"); err != nil {
		return err
	}
	if s.production != nil {
		if err = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "cloud_kitchen.package.archived", "cloud_kitchen.package.archived", "cloud_kitchen_package", id, branch, version+1, map[string]any{"status": "ACTIVE"}, map[string]any{"status": "ARCHIVED"}, nil, ""); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Service) AddComponent(ctx context.Context, business, packageID, itemRef string, quantity int, variant *string, position int, expectedVersion int64) (string, error) {
	if quantity <= 0 {
		return "", fmt.Errorf("default quantity must be positive")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	var branch, status string
	var packageVersion int64
	if err = tx.QueryRow(ctx, `SELECT branch_id,status,version FROM cloud_kitchen_packages WHERE business_id=$1 AND id=$2 FOR UPDATE`, business, packageID).Scan(&branch, &status, &packageVersion); err != nil {
		return "", err
	}
	if status == "ARCHIVED" || expectedVersion <= 0 || expectedVersion != packageVersion {
		return "", fmt.Errorf("stale package version")
	}
	var itemExists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM food_items WHERE id=$1 AND business_id=$2 AND branch_id=$3 AND status<>'ARCHIVED')`, itemRef, business, branch).Scan(&itemExists); err != nil {
		return "", err
	}
	if !itemExists {
		return "", fmt.Errorf("active catalogue item not found for package branch")
	}
	if variant != nil && strings.TrimSpace(*variant) != "" {
		var variantExists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM food_item_variants WHERE id=$1 AND item_id=$2 AND business_id=$3 AND status='ACTIVE')`, strings.TrimSpace(*variant), itemRef, business).Scan(&variantExists); err != nil {
			return "", err
		}
		if !variantExists {
			return "", fmt.Errorf("active catalogue variant not found for package item")
		}
	}
	id := uuid.NewString()
	if _, err = tx.Exec(ctx, `INSERT INTO cloud_kitchen_package_components(id,package_id,item_ref,default_quantity,variant_ref,position) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(package_id,item_ref,variant_ref) DO UPDATE SET default_quantity=EXCLUDED.default_quantity,position=EXCLUDED.position`, id, packageID, itemRef, quantity, variant, position); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `UPDATE cloud_kitchen_packages SET version=version+1,updated_at=now() WHERE id=$1 AND version=$2`, packageID, packageVersion); err != nil {
		return "", err
	}
	if err = snapshotPackage(ctx, tx, packageID, "COMPONENT_UPDATED"); err != nil {
		return "", err
	}
	if s.production != nil {
		if err = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "cloud_kitchen.package_component.upserted", "cloud_kitchen.package_component.upserted", "cloud_kitchen_package_component", packageID+":"+itemRef, branch, 1, nil, map[string]any{"package_id": packageID, "item_ref": itemRef, "quantity": quantity}, map[string]any{"id": id}, ""); err != nil {
			return "", err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Service) RemoveComponent(ctx context.Context, business, packageID, componentID string, expectedVersion int64) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var branch, status string
	var packageVersion int64
	if err = tx.QueryRow(ctx, `SELECT branch_id,status,version FROM cloud_kitchen_packages WHERE business_id=$1 AND id=$2 FOR UPDATE`, business, packageID).Scan(&branch, &status, &packageVersion); err != nil {
		return err
	}
	if status == "ARCHIVED" || expectedVersion <= 0 || expectedVersion != packageVersion {
		return fmt.Errorf("stale package version")
	}
	var itemRef string
	if err = tx.QueryRow(ctx, `DELETE FROM cloud_kitchen_package_components WHERE package_id=$1 AND id=$2 RETURNING item_ref`, packageID, componentID).Scan(&itemRef); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE cloud_kitchen_packages SET version=version+1,updated_at=now() WHERE id=$1 AND version=$2`, packageID, packageVersion); err != nil {
		return err
	}
	if err = snapshotPackage(ctx, tx, packageID, "COMPONENT_REMOVED"); err != nil {
		return err
	}
	if s.production != nil {
		if err = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "cloud_kitchen.package_component.removed", "cloud_kitchen.package_component.removed", "cloud_kitchen_package_component", componentID, branch, 1, map[string]any{"item_ref": itemRef}, map[string]any{"removed": true}, map[string]any{"package_id": packageID, "package_version": packageVersion + 1}, ""); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

type BulkPreviewInput struct {
	PackageID   string
	Quantity    int
	WindowStart time.Time
	WindowEnd   time.Time
}

// Preview combines commercial selection and the shared capacity preview. It
// does not reserve capacity and it never creates an Order or payment object.
func (s *Service) Preview(ctx context.Context, actor production.Actor, scope production.Scope, in BulkPreviewInput) (map[string]any, error) {
	if in.Quantity <= 0 || in.Quantity > production.MaxCapacity {
		return nil, fmt.Errorf("quantity is outside the supported range")
	}
	var branch, status string
	var pool *string
	var packageLead *int
	var packageVersion int64
	var defaultLead int
	if err := s.db.QueryRow(ctx, `SELECT p.branch_id,p.status,p.capacity_pool_id,p.lead_time_minutes,p.version,COALESCE(s.minimum_lead_time_minutes,60) FROM cloud_kitchen_packages p LEFT JOIN cloud_kitchen_settings s ON s.business_id=p.business_id AND s.branch_id=p.branch_id WHERE p.business_id=$1 AND p.id=$2`, actor.VendorID, in.PackageID).Scan(&branch, &status, &pool, &packageLead, &packageVersion, &defaultLead); err != nil {
		return nil, err
	}
	if !scope.Covers(branch) || status != "ACTIVE" {
		return nil, fmt.Errorf("active package not found")
	}
	var bandID string
	var min, weight int
	var max *int
	var unit, pack int64
	var bandLead *int
	var bandVersion int64
	if err := s.db.QueryRow(ctx, `SELECT id,min_quantity,max_quantity,unit_price_minor,COALESCE(package_price_minor,0),lead_time_minutes,capacity_units_per_item,version FROM cloud_kitchen_quantity_bands WHERE package_id=$1 AND status='ACTIVE' AND min_quantity<=$2 AND (max_quantity IS NULL OR max_quantity>=$2) ORDER BY min_quantity DESC LIMIT 1`, in.PackageID, in.Quantity).Scan(&bandID, &min, &max, &unit, &pack, &bandLead, &weight, &bandVersion); err != nil {
		return nil, err
	}
	lead := defaultLead
	if packageLead != nil {
		lead = *packageLead
	}
	if bandLead != nil {
		lead = *bandLead
	}
	now := s.production.Now()
	earliest := now.Add(time.Duration(lead) * time.Minute)
	start := in.WindowStart
	if start.IsZero() {
		start = earliest
	}
	end := in.WindowEnd
	if end.IsZero() {
		end = start.Add(time.Hour)
	}
	if !end.After(start) {
		return nil, fmt.Errorf("invalid production window")
	}
	workload := in.Quantity * weight
	if workload > production.MaxCapacity {
		return nil, fmt.Errorf("capacity workload is outside the supported range")
	}
	result := map[string]any{
		"package_id": in.PackageID, "package_version": packageVersion, "quantity": in.Quantity,
		"matched_band":      map[string]any{"id": bandID, "version": bandVersion, "min_quantity": min, "max_quantity": max, "unit_price_minor": unit, "package_price_minor": pack, "capacity_units_per_item": weight},
		"lead_time_minutes": lead, "lead_time_satisfied": !start.Before(earliest), "earliest_start_at": earliest,
		"estimated_production_window": map[string]any{"start_at": start, "end_at": end},
	}
	if pool == nil || *pool == "" {
		result["required_pools"] = []any{}
		result["available"] = false
		result["blocking_reason"] = "CAPACITY_POOL_NOT_CONFIGURED"
		return result, nil
	}
	allocation := production.AllocationRequest{CapacityPoolID: *pool, Quantity: workload}
	capacity, err := s.production.PreviewCapacity(ctx, actor, scope, production.ReserveInput{BranchID: branch, Capability: "CLOUD_KITCHEN", WindowStart: start, WindowEnd: end, Allocations: []production.AllocationRequest{allocation}, SourceType: "BULK_PACKAGE", SourceRef: in.PackageID})
	if err != nil {
		return nil, err
	}
	result["required_pools"] = []any{map[string]any{"capacity_pool_id": *pool, "quantity": workload}}
	result["capacity"] = capacity
	result["available"] = capacity.Available && !start.Before(earliest)
	return result, nil
}

type Plan struct {
	ID                     string     `json:"id"`
	BranchID               string     `json:"branch_id"`
	CorporateProfileRef    string     `json:"corporate_profile_ref"`
	Name                   string     `json:"name"`
	PackageID              string     `json:"package_id"`
	RecurrenceType         string     `json:"recurrence_type"`
	Timezone               string     `json:"timezone"`
	LocalTime              string     `json:"local_time"`
	Status                 string     `json:"status"`
	Quantity               int        `json:"quantity"`
	Weekdays               []int      `json:"weekdays"`
	EffectiveFrom          time.Time  `json:"effective_from"`
	EffectiveUntil         *time.Time `json:"effective_until"`
	ProductionMinutes      int        `json:"production_minutes"`
	LeadTimeMinutes        *int       `json:"lead_time_minutes"`
	ReservationHorizonDays *int       `json:"reservation_horizon_days"`
	Version                int64      `json:"version"`
}

func (s *Service) ListPlans(ctx context.Context, business string, scope production.Scope) ([]Plan, error) {
	rows, err := s.db.Query(ctx, `SELECT id,branch_id,corporate_profile_ref,name,package_id,recurrence_type,quantity,weekdays,timezone,to_char(local_time,'HH24:MI'),effective_from,effective_until,production_minutes,lead_time_minutes,reservation_horizon_days,status,version FROM cloud_kitchen_corporate_plans WHERE business_id=$1 AND ($2 OR branch_id=ANY($3)) ORDER BY effective_from DESC`, business, scope.Global, scope.BranchIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Plan{}
	for rows.Next() {
		var p Plan
		if err := rows.Scan(&p.ID, &p.BranchID, &p.CorporateProfileRef, &p.Name, &p.PackageID, &p.RecurrenceType, &p.Quantity, &p.Weekdays, &p.Timezone, &p.LocalTime, &p.EffectiveFrom, &p.EffectiveUntil, &p.ProductionMinutes, &p.LeadTimeMinutes, &p.ReservationHorizonDays, &p.Status, &p.Version); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Service) CreatePlan(ctx context.Context, business string, p Plan) (string, error) {
	var branchTimezone string
	if err := s.db.QueryRow(ctx, `SELECT timezone FROM vendor_branches WHERE business_id=$1 AND id=$2`, business, p.BranchID).Scan(&branchTimezone); err != nil {
		return "", fmt.Errorf("branch not found")
	}
	if p.Timezone != "" && p.Timezone != branchTimezone {
		return "", fmt.Errorf("timezone must match branch timezone")
	}
	p.Timezone = branchTimezone
	if _, err := time.LoadLocation(p.Timezone); err != nil {
		return "", fmt.Errorf("invalid branch IANA timezone")
	}
	if _, err := time.Parse("15:04", p.LocalTime); err != nil {
		return "", fmt.Errorf("local_time must use HH:MM")
	}
	if p.Quantity <= 0 || (p.ProductionMinutes < 0 || p.ProductionMinutes > 24*60) || (p.LeadTimeMinutes != nil && (*p.LeadTimeMinutes < 0 || *p.LeadTimeMinutes > 30*24*60)) || (p.ReservationHorizonDays != nil && (*p.ReservationHorizonDays < 1 || *p.ReservationHorizonDays > 366)) {
		return "", fmt.Errorf("quantity must be positive")
	}
	if p.ProductionMinutes == 0 {
		p.ProductionMinutes = 60
	}
	if p.RecurrenceType != "DAILY" && p.RecurrenceType != "WEEKLY" && p.RecurrenceType != "SELECTED_WEEKDAYS" {
		return "", fmt.Errorf("invalid recurrence type")
	}
	if p.RecurrenceType == "SELECTED_WEEKDAYS" && len(p.Weekdays) == 0 {
		return "", fmt.Errorf("weekdays are required")
	}
	seenWeekdays := map[int]bool{}
	for _, weekday := range p.Weekdays {
		if weekday < 1 || weekday > 7 || seenWeekdays[weekday] {
			return "", fmt.Errorf("weekdays must contain unique ISO weekdays from 1 to 7")
		}
		seenWeekdays[weekday] = true
	}
	if p.EffectiveFrom.IsZero() || (p.EffectiveUntil != nil && p.EffectiveUntil.Before(p.EffectiveFrom)) {
		return "", fmt.Errorf("invalid effective date range")
	}
	var packageBranch, packageStatus string
	if err := s.db.QueryRow(ctx, `SELECT branch_id,status FROM cloud_kitchen_packages WHERE business_id=$1 AND id=$2`, business, p.PackageID).Scan(&packageBranch, &packageStatus); err != nil || packageBranch != p.BranchID || packageStatus != "ACTIVE" {
		return "", fmt.Errorf("active package not found for branch")
	}
	id := uuid.NewString()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if err = lockCorporate(ctx, tx, business, p.BranchID, p.CorporateProfileRef); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO cloud_kitchen_corporate_plans(id,business_id,branch_id,corporate_profile_ref,name,package_id,quantity,recurrence_type,weekdays,timezone,local_time,effective_from,effective_until,production_minutes,lead_time_minutes,reservation_horizon_days,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,'DRAFT')`, id, business, p.BranchID, p.CorporateProfileRef, p.Name, p.PackageID, p.Quantity, p.RecurrenceType, p.Weekdays, p.Timezone, p.LocalTime, p.EffectiveFrom, p.EffectiveUntil, p.ProductionMinutes, p.LeadTimeMinutes, p.ReservationHorizonDays); err != nil {
		return "", err
	}
	if s.production != nil {
		if err = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "cloud_kitchen.plan.created", "cloud_kitchen.plan.created", "cloud_kitchen_corporate_plan", id, p.BranchID, 1, nil, p, map[string]any{"id": id}, ""); err != nil {
			return "", err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return id, nil
}
func (s *Service) SetPlanStatus(ctx context.Context, business, id, status string, expectedVersion int64, scope production.Scope) error {
	var branch string
	if err := s.db.QueryRow(ctx, `SELECT branch_id FROM cloud_kitchen_corporate_plans WHERE business_id=$1 AND id=$2`, business, id).Scan(&branch); err != nil {
		return err
	}
	if !scope.Covers(branch) {
		return fmt.Errorf("forbidden")
	}
	if status != "ACTIVE" && status != "SUSPENDED" && status != "ARCHIVED" {
		return fmt.Errorf("invalid plan status")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var currentStatus string
	var ver int64
	if err = tx.QueryRow(ctx, `SELECT status,version FROM cloud_kitchen_corporate_plans WHERE business_id=$1 AND id=$2 FOR UPDATE`, business, id).Scan(&currentStatus, &ver); err != nil {
		return err
	}
	if expectedVersion <= 0 || expectedVersion != ver {
		return fmt.Errorf("stale plan version")
	}
	valid := map[string]map[string]bool{"DRAFT": {"ACTIVE": true, "ARCHIVED": true}, "ACTIVE": {"SUSPENDED": true, "ARCHIVED": true}, "SUSPENDED": {"ACTIVE": true, "ARCHIVED": true}, "ARCHIVED": {}}
	if !valid[currentStatus][status] {
		return fmt.Errorf("invalid plan transition")
	}
	tag, err := tx.Exec(ctx, `UPDATE cloud_kitchen_corporate_plans SET status=$1,version=version+1,updated_at=now() WHERE business_id=$2 AND id=$3 AND version=$4`, status, business, id, ver)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("stale plan version")
	}
	if s.production != nil {
		if err = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "cloud_kitchen.plan."+strings.ToLower(status), "cloud_kitchen.plan."+strings.ToLower(status), "cloud_kitchen_corporate_plan", id, branch, ver+1, map[string]any{"status": currentStatus, "version": ver}, map[string]any{"status": status, "version": ver + 1}, map[string]any{"id": id, "status": status}, ""); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Service) UpdatePlan(ctx context.Context, business, id string, p Plan, scope production.Scope) (int64, error) {
	var branch, status string
	var version int64
	if err := s.db.QueryRow(ctx, `SELECT branch_id,status,version FROM cloud_kitchen_corporate_plans WHERE business_id=$1 AND id=$2`, business, id).Scan(&branch, &status, &version); err != nil {
		return 0, err
	}
	if !scope.Covers(branch) {
		return 0, fmt.Errorf("forbidden")
	}
	if p.Version <= 0 || p.Version != version {
		return 0, fmt.Errorf("stale plan version")
	}
	p.BranchID = branch
	if status != "DRAFT" && status != "SUSPENDED" {
		return 0, fmt.Errorf("only draft or suspended plans can be edited")
	}
	// Reuse creation validation without writing by checking the same bounded
	// recurrence, package, branch-timezone and production fields here.
	var branchTimezone, packageBranch, packageStatus string
	if err := s.db.QueryRow(ctx, `SELECT timezone FROM vendor_branches WHERE business_id=$1 AND id=$2`, business, branch).Scan(&branchTimezone); err != nil {
		return 0, err
	}
	if p.Timezone != "" && p.Timezone != branchTimezone {
		return 0, fmt.Errorf("timezone must match branch timezone")
	}
	p.Timezone = branchTimezone
	if _, err := time.Parse("15:04", p.LocalTime); err != nil || p.Quantity <= 0 || p.ProductionMinutes < 1 || p.ProductionMinutes > 24*60 || p.EffectiveFrom.IsZero() || (p.EffectiveUntil != nil && p.EffectiveUntil.Before(p.EffectiveFrom)) {
		return 0, fmt.Errorf("invalid plan configuration")
	}
	if p.RecurrenceType != "DAILY" && p.RecurrenceType != "WEEKLY" && p.RecurrenceType != "SELECTED_WEEKDAYS" {
		return 0, fmt.Errorf("invalid recurrence type")
	}
	seen := map[int]bool{}
	for _, weekday := range p.Weekdays {
		if weekday < 1 || weekday > 7 || seen[weekday] {
			return 0, fmt.Errorf("invalid weekdays")
		}
		seen[weekday] = true
	}
	if p.RecurrenceType == "SELECTED_WEEKDAYS" && len(p.Weekdays) == 0 {
		return 0, fmt.Errorf("weekdays are required")
	}
	if p.LeadTimeMinutes != nil && (*p.LeadTimeMinutes < 0 || *p.LeadTimeMinutes > 30*24*60) {
		return 0, fmt.Errorf("invalid lead time")
	}
	if p.ReservationHorizonDays != nil && (*p.ReservationHorizonDays < 1 || *p.ReservationHorizonDays > 366) {
		return 0, fmt.Errorf("invalid reservation horizon")
	}
	if err := s.db.QueryRow(ctx, `SELECT branch_id,status FROM cloud_kitchen_packages WHERE business_id=$1 AND id=$2`, business, p.PackageID).Scan(&packageBranch, &packageStatus); err != nil || packageBranch != branch || packageStatus != "ACTIVE" {
		return 0, fmt.Errorf("active package not found for branch")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var previousRef string
	if err = tx.QueryRow(ctx, `SELECT corporate_profile_ref FROM cloud_kitchen_corporate_plans WHERE business_id=$1 AND id=$2 FOR UPDATE`, business, id).Scan(&previousRef); err != nil {
		return 0, err
	}
	if previousRef != p.CorporateProfileRef {
		if err = lockCorporate(ctx, tx, business, branch, p.CorporateProfileRef); err != nil {
			return 0, err
		}
	}
	tag, err := tx.Exec(ctx, `UPDATE cloud_kitchen_corporate_plans SET corporate_profile_ref=$3,name=$4,package_id=$5,quantity=$6,recurrence_type=$7,weekdays=$8,timezone=$9,local_time=$10::time,effective_from=$11,effective_until=$12,production_minutes=$13,lead_time_minutes=$14,reservation_horizon_days=$15,updated_at=now(),version=version+1 WHERE business_id=$1 AND id=$2 AND version=$16 AND status IN ('DRAFT','SUSPENDED')`, business, id, p.CorporateProfileRef, p.Name, p.PackageID, p.Quantity, p.RecurrenceType, p.Weekdays, p.Timezone, p.LocalTime, p.EffectiveFrom, p.EffectiveUntil, p.ProductionMinutes, p.LeadTimeMinutes, p.ReservationHorizonDays, version)
	if err != nil {
		return 0, err
	}
	if tag.RowsAffected() == 0 {
		return 0, fmt.Errorf("stale plan version")
	}
	if err = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "cloud_kitchen.plan.updated", "cloud_kitchen.plan.updated", "cloud_kitchen_corporate_plan", id, branch, version+1, map[string]any{"status": status, "version": version}, p, map[string]any{"id": id}, ""); err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return version + 1, nil
}
func (s *Service) AddException(ctx context.Context, plan string, date time.Time, action string, replacement *time.Time, qty *int, pkg *string) error {
	if action != "SKIP" && action != "RESCHEDULE" && action != "QUANTITY_OVERRIDE" && action != "PACKAGE_OVERRIDE" {
		return fmt.Errorf("invalid exception action")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var business, branch string
	if err = tx.QueryRow(ctx, `SELECT business_id,branch_id FROM cloud_kitchen_corporate_plans WHERE id=$1 FOR UPDATE`, plan).Scan(&business, &branch); err != nil {
		return err
	}
	id := uuid.NewString()
	if _, err = tx.Exec(ctx, `INSERT INTO cloud_kitchen_plan_exceptions(id,plan_id,occurrence_date,action,replacement_date,quantity_override,package_override) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(plan_id,occurrence_date) DO UPDATE SET action=EXCLUDED.action,replacement_date=EXCLUDED.replacement_date,quantity_override=EXCLUDED.quantity_override,package_override=EXCLUDED.package_override`, id, plan, date, action, replacement, qty, pkg); err != nil {
		return err
	}
	if s.production != nil {
		if err = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "cloud_kitchen.exception.upserted", "cloud_kitchen.exception.upserted", "cloud_kitchen_plan_exception", id, branch, 1, nil, map[string]any{"plan_id": plan, "occurrence_date": date, "action": action}, map[string]any{"id": id}, ""); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (s *Service) Materialize(ctx context.Context, business string, limit int) (int, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT p.id,p.branch_id,p.package_id,p.quantity,p.recurrence_type,p.weekdays,p.timezone,to_char(p.local_time,'HH24:MI'),p.effective_from,p.effective_until,p.production_minutes,COALESCE(p.lead_time_minutes,pkg.lead_time_minutes,s.minimum_lead_time_minutes,60),COALESCE(p.reservation_horizon_days,s.corporate_reservation_horizon_days,90) FROM cloud_kitchen_corporate_plans p JOIN cloud_kitchen_packages pkg ON pkg.id=p.package_id LEFT JOIN cloud_kitchen_settings s ON s.business_id=p.business_id AND s.branch_id=p.branch_id WHERE p.business_id=$1 AND p.status='ACTIVE' AND p.effective_from<=current_date ORDER BY p.id LIMIT $2`, business, limit)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id, branch, pkg, recurrence, tz, lt string
		var qty, productionMinutes, leadMinutes, horizonDays int
		var weekdays []int16
		var from time.Time
		var until *time.Time
		if err := rows.Scan(&id, &branch, &pkg, &qty, &recurrence, &weekdays, &tz, &lt, &from, &until, &productionMinutes, &leadMinutes, &horizonDays); err != nil {
			return n, err
		}
		loc, e := time.LoadLocation(tz)
		if e != nil {
			continue
		}
		now := s.production.Now().In(loc)
		first := from
		if first.Before(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)) {
			first = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		}
		baseQty, basePackage := qty, pkg
		for day := 0; day < horizonDays; day++ {
			d := first.AddDate(0, 0, day)
			if until != nil && d.After(*until) {
				break
			}
			if !matchesRecurrence(recurrence, weekdays, from, d) {
				continue
			}
			key := id + ":" + d.Format("2006-01-02")
			var exceptionAction string
			var replacement *time.Time
			var overrideQty *int
			var overridePackage *string
			_ = s.db.QueryRow(ctx, `SELECT action,replacement_date,quantity_override,package_override FROM cloud_kitchen_plan_exceptions WHERE plan_id=$1 AND occurrence_date=$2`, id, d).Scan(&exceptionAction, &replacement, &overrideQty, &overridePackage)
			if exceptionAction == "SKIP" {
				created, skippedErr := s.insertSkippedOccurrence(ctx, business, branch, id, key, d, qty, pkg)
				if skippedErr != nil {
					return n, skippedErr
				}
				if created {
					n++
				}
				continue
			}
			if replacement != nil {
				d = *replacement
			}
			occurrenceQty, occurrencePackage := baseQty, basePackage
			if overrideQty != nil {
				occurrenceQty = *overrideQty
			}
			if overridePackage != nil {
				occurrencePackage = *overridePackage
			}
			start, e := time.ParseInLocation("2006-01-02 15:04", d.Format("2006-01-02")+" "+lt, loc)
			if e != nil {
				continue
			}
			end := start.Add(time.Duration(productionMinutes) * time.Minute)
			if start.Before(s.production.Now().Add(time.Duration(leadMinutes) * time.Minute)) {
				created, conflictErr := s.insertOccurrenceConflict(ctx, business, branch, id, key, d, start, end, occurrenceQty, occurrencePackage, "LEAD_TIME")
				if conflictErr != nil {
					return n, conflictErr
				}
				if created {
					n++
				}
				continue
			}
			created, materializeErr := s.materializeReservedOccurrence(ctx, business, branch, id, key, d, start, end, occurrenceQty, occurrencePackage)
			if materializeErr != nil {
				return n, materializeErr
			}
			if created {
				n++
			}
		}
	}
	return n, rows.Err()
}

func (s *Service) insertSkippedOccurrence(ctx context.Context, business, branch, planID, key string, occurrenceDate time.Time, quantity int, packageID string) (bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	id := uuid.NewString()
	err = tx.QueryRow(ctx, `INSERT INTO cloud_kitchen_occurrences(id,plan_id,occurrence_key,occurrence_date,starts_at,ends_at,quantity,package_id,state) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'SKIPPED') ON CONFLICT(plan_id,occurrence_key) DO NOTHING RETURNING id`, id, planID, key, occurrenceDate, occurrenceDate, occurrenceDate.Add(time.Hour), quantity, packageID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "cloud_kitchen.occurrence.skipped", "cloud_kitchen.occurrence.skipped", "cloud_kitchen_occurrence", id, branch, 1, nil, map[string]any{"state": "SKIPPED"}, map[string]any{"occurrence_id": id, "plan_id": planID}, ""); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func (s *Service) materializeReservedOccurrence(ctx context.Context, business, branch, planID, key string, occurrenceDate, start, end time.Time, quantity int, packageID string) (bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	occurrenceID := uuid.NewString()
	err = tx.QueryRow(ctx, `INSERT INTO cloud_kitchen_occurrences(id,plan_id,occurrence_key,occurrence_date,starts_at,ends_at,quantity,package_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(plan_id,occurrence_key) DO NOTHING RETURNING id`, occurrenceID, planID, key, occurrenceDate, start, end, quantity, packageID).Scan(&occurrenceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var poolID *string
	var capacityWeight int
	var packageVersion int64
	if err = tx.QueryRow(ctx, `SELECT p.capacity_pool_id,COALESCE((SELECT b.capacity_units_per_item FROM cloud_kitchen_quantity_bands b WHERE b.package_id=p.id AND b.status='ACTIVE' AND b.min_quantity<=$3 AND (b.max_quantity IS NULL OR b.max_quantity>=$3) ORDER BY b.min_quantity DESC LIMIT 1),1),p.version FROM cloud_kitchen_packages p WHERE p.id=$1 AND p.business_id=$2 AND p.status='ACTIVE'`, packageID, business, quantity).Scan(&poolID, &capacityWeight, &packageVersion); err != nil {
		return false, err
	}
	if poolID == nil || *poolID == "" {
		return s.recordOccurrenceConflict(ctx, tx, business, branch, occurrenceID, "CAPACITY_POOL_NOT_CONFIGURED")
	}
	workload := quantity * capacityWeight
	if workload <= 0 || workload > production.MaxCapacity {
		return s.recordOccurrenceConflict(ctx, tx, business, branch, occurrenceID, "CAPACITY_WORKLOAD_INVALID")
	}
	actor := production.Actor{VendorID: business, CommandRef: "occurrence:" + occurrenceID}
	reservation, _, reserveErr := s.production.ReserveInTx(ctx, tx, actor, production.Scope{Global: true}, production.ReserveInput{BranchID: branch, Capability: "CLOUD_KITCHEN", WindowStart: start, WindowEnd: end, Allocations: []production.AllocationRequest{{CapacityPoolID: *poolID, Quantity: workload}}, SourceType: "CORPORATE_OCCURRENCE", SourceRef: occurrenceID})
	if reserveErr != nil {
		_ = tx.Rollback(ctx)
		conflictTx, beginErr := s.db.Begin(ctx)
		if beginErr != nil {
			return false, beginErr
		}
		defer conflictTx.Rollback(ctx)
		conflictID := uuid.NewString()
		var insertedID string
		err = conflictTx.QueryRow(ctx, `INSERT INTO cloud_kitchen_occurrences(id,plan_id,occurrence_key,occurrence_date,starts_at,ends_at,quantity,package_id,state,conflict_reason,last_error) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'CAPACITY_CONFLICT','CAPACITY_UNAVAILABLE','CAPACITY_UNAVAILABLE') ON CONFLICT(plan_id,occurrence_key) DO NOTHING RETURNING id`, conflictID, planID, key, occurrenceDate, start, end, quantity, packageID).Scan(&insertedID)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if err = s.production.RecordMutation(ctx, conflictTx, production.Actor{VendorID: business}, "cloud_kitchen.occurrence.capacity_conflict", "cloud_kitchen.occurrence.capacity_conflict", "cloud_kitchen_occurrence", insertedID, branch, 1, nil, map[string]any{"state": "CAPACITY_CONFLICT", "reason": "CAPACITY_UNAVAILABLE"}, map[string]any{"occurrence_id": insertedID, "reason": "CAPACITY_UNAVAILABLE"}, ""); err != nil {
			return false, err
		}
		return true, conflictTx.Commit(ctx)
	}
	jobID := "job_occ_" + occurrenceID
	if _, err = tx.Exec(ctx, `INSERT INTO production_jobs(id,business_id,branch_id,capability,reservation_id,source_type,source_ref,source_version,title,planned_quantity,scheduled_start,scheduled_end) VALUES($1,$2,$3,'CLOUD_KITCHEN',$4,'CORPORATE_OCCURRENCE',$5,$6,$7,$8,$9,$10)`, jobID, business, branch, reservation.ID, occurrenceID, packageVersion, "Corporate package "+occurrenceID, quantity, start, end); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO production_job_events(id,business_id,job_id,event_type,to_status,actor_type,note) VALUES($1,$2,$3,'CREATED','PLANNED','system','corporate occurrence materialized')`, "pje_"+uuid.NewString(), business, jobID); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE cloud_kitchen_occurrences SET state='RESERVED',reservation_id=$1,production_job_id=$2,lease_owner=NULL,lease_expires_at=NULL,version=version+1 WHERE id=$3`, reservation.ID, jobID, occurrenceID); err != nil {
		return false, err
	}
	if err = s.production.RecordMutation(ctx, tx, actor, "cloud_kitchen.occurrence.reserved", "cloud_kitchen.occurrence.reserved", "cloud_kitchen_occurrence", occurrenceID, branch, 1, map[string]any{"state": "PLANNED"}, map[string]any{"state": "RESERVED", "reservation_id": reservation.ID, "production_job_id": jobID}, map[string]any{"occurrence_id": occurrenceID, "reservation_id": reservation.ID, "production_job_id": jobID}, ""); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func (s *Service) insertOccurrenceConflict(ctx context.Context, business, branch, planID, key string, occurrenceDate, start, end time.Time, quantity int, packageID, reason string) (bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	id := uuid.NewString()
	err = tx.QueryRow(ctx, `INSERT INTO cloud_kitchen_occurrences(id,plan_id,occurrence_key,occurrence_date,starts_at,ends_at,quantity,package_id,state,conflict_reason,last_error) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'CAPACITY_CONFLICT',$9,$9) ON CONFLICT(plan_id,occurrence_key) DO NOTHING RETURNING id`, id, planID, key, occurrenceDate, start, end, quantity, packageID, reason).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "cloud_kitchen.occurrence.capacity_conflict", "cloud_kitchen.occurrence.capacity_conflict", "cloud_kitchen_occurrence", id, branch, 1, nil, map[string]any{"state": "CAPACITY_CONFLICT", "reason": reason}, map[string]any{"occurrence_id": id, "reason": reason}, ""); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func (s *Service) recordOccurrenceConflict(ctx context.Context, tx pgx.Tx, business, branch, occurrenceID, reason string) (bool, error) {
	if _, err := tx.Exec(ctx, `UPDATE cloud_kitchen_occurrences SET state='CAPACITY_CONFLICT',conflict_reason=$1,last_error=$1,version=version+1 WHERE id=$2`, reason, occurrenceID); err != nil {
		return false, err
	}
	actor := production.Actor{VendorID: business}
	if err := s.production.RecordMutation(ctx, tx, actor, "cloud_kitchen.occurrence.capacity_conflict", "cloud_kitchen.occurrence.capacity_conflict", "cloud_kitchen_occurrence", occurrenceID, branch, 1, map[string]any{"state": "PLANNED"}, map[string]any{"state": "CAPACITY_CONFLICT", "reason": reason}, map[string]any{"occurrence_id": occurrenceID, "reason": reason}, ""); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func matchesRecurrence(kind string, weekdays []int16, from, day time.Time) bool {
	switch kind {
	case "DAILY":
		return true
	case "WEEKLY":
		return day.Weekday() == from.Weekday()
	case "EVERY_N_WEEKS":
		weeks := int(day.Sub(from).Hours() / (24 * 7))
		return weeks >= 0 && weeks%2 == 0 && day.Weekday() == from.Weekday()
	case "SELECTED_WEEKDAYS":
		wd := int16(day.Weekday())
		if wd == 0 {
			wd = 7
		}
		for _, v := range weekdays {
			if v == wd {
				return true
			}
		}
	}
	return false
}

// LeaseNextOccurrence claims one due occurrence for a worker.  The lease is
// the coordination boundary; attempts are bounded and a permanently failing
// occurrence is moved to a durable dead-letter state rather than retried
// forever.
func (s *Service) LeaseNextOccurrence(ctx context.Context, worker string, ttl time.Duration) (string, error) {
	if worker == "" {
		return "", fmt.Errorf("worker is required")
	}
	if ttl <= 0 || ttl > 15*time.Minute {
		ttl = 2 * time.Minute
	}
	var id string
	err := s.db.QueryRow(ctx, `WITH candidate AS (
		SELECT id FROM cloud_kitchen_occurrences
		WHERE state IN ('PLANNED','CAPACITY_CONFLICT')
		  AND (dead_lettered_at IS NULL)
		  AND (lease_expires_at IS NULL OR lease_expires_at < now())
		ORDER BY starts_at,id FOR UPDATE SKIP LOCKED LIMIT 1
	) UPDATE cloud_kitchen_occurrences o
	SET lease_owner=$1, lease_expires_at=now()+$2::interval,
		attempt_count=o.attempt_count+1, version=o.version+1
	FROM candidate c WHERE o.id=c.id RETURNING o.id`, worker, fmt.Sprintf("%f seconds", ttl.Seconds())).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *Service) HeartbeatOccurrence(ctx context.Context, worker, occurrenceID string, ttl time.Duration) error {
	if ttl <= 0 || ttl > 15*time.Minute {
		ttl = 2 * time.Minute
	}
	r, err := s.db.Exec(ctx, `UPDATE cloud_kitchen_occurrences SET lease_expires_at=now()+$1::interval WHERE id=$2 AND lease_owner=$3 AND dead_lettered_at IS NULL`, fmt.Sprintf("%f seconds", ttl.Seconds()), occurrenceID, worker)
	if err != nil {
		return err
	}
	if r.RowsAffected() == 0 {
		return fmt.Errorf("occurrence lease lost")
	}
	return nil
}

// RunOccurrenceRecoveryPass converts leased PLANNED/conflicted occurrences.
// A failed reservation rolls its transaction back before durable retry state
// is written, so even failures after an insert can never leak partial capacity.
func (s *Service) RunOccurrenceRecoveryPass(ctx context.Context, worker string, limit int) (int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	completed := 0
	for i := 0; i < limit; i++ {
		id, err := s.LeaseNextOccurrence(ctx, worker, 2*time.Minute)
		if errors.Is(err, pgx.ErrNoRows) {
			break
		}
		if err != nil {
			return completed, err
		}
		ok, err := s.processLeasedOccurrence(ctx, worker, id)
		if err != nil {
			return completed, err
		}
		if ok {
			completed++
		}
	}
	return completed, nil
}

func (s *Service) processLeasedOccurrence(ctx context.Context, worker, occurrenceID string) (bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var business, branch, packageID, planStatus string
	var start, end time.Time
	var quantity, attempt int
	if err = tx.QueryRow(ctx, `SELECT p.business_id,p.branch_id,o.package_id,p.status,o.starts_at,o.ends_at,o.quantity,o.attempt_count FROM cloud_kitchen_occurrences o JOIN cloud_kitchen_corporate_plans p ON p.id=o.plan_id WHERE o.id=$1 AND o.lease_owner=$2 AND o.state IN ('PLANNED','CAPACITY_CONFLICT') AND o.dead_lettered_at IS NULL FOR UPDATE`, occurrenceID, worker).Scan(&business, &branch, &packageID, &planStatus, &start, &end, &quantity, &attempt); err != nil {
		return false, err
	}
	if planStatus != "ACTIVE" {
		_, err = tx.Exec(ctx, `UPDATE cloud_kitchen_occurrences SET lease_owner=NULL,lease_expires_at=now()+interval '1 hour',version=version+1 WHERE id=$1 AND lease_owner=$2`, occurrenceID, worker)
		if err != nil {
			return false, err
		}
		return false, tx.Commit(ctx)
	}
	var poolID *string
	var weight int
	var packageVersion int64
	if err = tx.QueryRow(ctx, `SELECT p.capacity_pool_id,COALESCE((SELECT b.capacity_units_per_item FROM cloud_kitchen_quantity_bands b WHERE b.package_id=p.id AND b.status='ACTIVE' AND b.min_quantity<=$3 AND (b.max_quantity IS NULL OR b.max_quantity>=$3) ORDER BY b.min_quantity DESC LIMIT 1),1),p.version FROM cloud_kitchen_packages p WHERE p.id=$1 AND p.business_id=$2 AND p.status='ACTIVE'`, packageID, business, quantity).Scan(&poolID, &weight, &packageVersion); err != nil {
		return false, s.failOccurrenceLease(ctx, tx, business, branch, worker, occurrenceID, attempt, "PACKAGE_NOT_AVAILABLE")
	}
	if poolID == nil || *poolID == "" || quantity*weight <= 0 || quantity*weight > production.MaxCapacity {
		return false, s.failOccurrenceLease(ctx, tx, business, branch, worker, occurrenceID, attempt, "CAPACITY_CONFIGURATION_INVALID")
	}
	actor := production.Actor{VendorID: business, CommandRef: "occurrence:" + occurrenceID}
	reservation, _, reserveErr := s.production.ReserveInTx(ctx, tx, actor, production.Scope{Global: true}, production.ReserveInput{BranchID: branch, Capability: "CLOUD_KITCHEN", WindowStart: start, WindowEnd: end, Allocations: []production.AllocationRequest{{CapacityPoolID: *poolID, Quantity: quantity * weight}}, SourceType: "CORPORATE_OCCURRENCE", SourceRef: occurrenceID})
	if reserveErr != nil {
		_ = tx.Rollback(ctx)
		failureTx, beginErr := s.db.Begin(ctx)
		if beginErr != nil {
			return false, beginErr
		}
		defer failureTx.Rollback(ctx)
		return false, s.failOccurrenceLease(ctx, failureTx, business, branch, worker, occurrenceID, attempt, "CAPACITY_UNAVAILABLE")
	}
	jobID := "job_occ_" + occurrenceID
	if _, err = tx.Exec(ctx, `INSERT INTO production_jobs(id,business_id,branch_id,capability,reservation_id,source_type,source_ref,source_version,title,planned_quantity,scheduled_start,scheduled_end) VALUES($1,$2,$3,'CLOUD_KITCHEN',$4,'CORPORATE_OCCURRENCE',$5,$6,$7,$8,$9,$10)`, jobID, business, branch, reservation.ID, occurrenceID, packageVersion, "Corporate package "+occurrenceID, quantity, start, end); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO production_job_events(id,business_id,job_id,event_type,to_status,actor_type,note) VALUES($1,$2,$3,'CREATED','PLANNED','system','corporate occurrence recovered')`, "pje_"+uuid.NewString(), business, jobID); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE cloud_kitchen_occurrences SET state='RESERVED',reservation_id=$1,production_job_id=$2,conflict_reason=NULL,last_error='',lease_owner=NULL,lease_expires_at=NULL,version=version+1 WHERE id=$3 AND lease_owner=$4`, reservation.ID, jobID, occurrenceID, worker); err != nil {
		return false, err
	}
	if err = s.production.RecordMutation(ctx, tx, actor, "cloud_kitchen.occurrence.reserved", "cloud_kitchen.occurrence.reserved", "cloud_kitchen_occurrence", occurrenceID, branch, 1, nil, map[string]any{"state": "RESERVED", "reservation_id": reservation.ID, "production_job_id": jobID}, map[string]any{"occurrence_id": occurrenceID, "reservation_id": reservation.ID, "production_job_id": jobID}, ""); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func (s *Service) failOccurrenceLease(ctx context.Context, tx pgx.Tx, business, branch, worker, occurrenceID string, attempt int, reason string) error {
	deadLetter := attempt >= 5
	tag, err := tx.Exec(ctx, `UPDATE cloud_kitchen_occurrences SET state='CAPACITY_CONFLICT',conflict_reason=$1,last_error=$1,lease_owner=NULL,lease_expires_at=CASE WHEN $4 THEN NULL ELSE now()+interval '5 minutes' END,dead_lettered_at=CASE WHEN $4 THEN now() ELSE dead_lettered_at END,version=version+1 WHERE id=$2 AND lease_owner=$3`, reason, occurrenceID, worker, deadLetter)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("occurrence lease lost")
	}
	action := "cloud_kitchen.occurrence.capacity_conflict"
	if deadLetter {
		action = "cloud_kitchen.occurrence.dead_lettered"
	}
	if err = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, action, action, "cloud_kitchen_occurrence", occurrenceID, branch, 1, nil, map[string]any{"state": "CAPACITY_CONFLICT", "reason": reason, "dead_lettered": deadLetter, "attempt_count": attempt}, map[string]any{"occurrence_id": occurrenceID, "reason": reason, "dead_lettered": deadLetter}, ""); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) ReplayDeadLetter(ctx context.Context, occurrenceID string) error {
	_, err := s.db.Exec(ctx, `UPDATE cloud_kitchen_occurrences SET dead_lettered_at=NULL,last_error='',replay_count=replay_count+1,lease_owner=NULL,lease_expires_at=NULL,state='PLANNED',version=version+1 WHERE id=$1`, occurrenceID)
	return err
}

func (s *Service) ReplayDeadLetterForBusiness(ctx context.Context, business, planID, occurrenceID, reason string, expectedVersion int64) error {
	if len(reason) > 280 {
		return fmt.Errorf("reason is too long")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var branch string
	var attemptCount, replayCount int
	var version int64
	if err = tx.QueryRow(ctx, `SELECT p.branch_id,o.attempt_count,o.replay_count,o.version FROM cloud_kitchen_occurrences o JOIN cloud_kitchen_corporate_plans p ON p.id=o.plan_id WHERE o.id=$1 AND o.plan_id=$2 AND p.business_id=$3 AND o.dead_lettered_at IS NOT NULL FOR UPDATE`, occurrenceID, planID, business).Scan(&branch, &attemptCount, &replayCount, &version); err != nil {
		return err
	}
	if expectedVersion <= 0 || expectedVersion != version {
		return apperrors.Conflict("The occurrence changed; reload and retry.", nil)
	}
	tag, err := tx.Exec(ctx, `UPDATE cloud_kitchen_occurrences SET dead_lettered_at=NULL,last_error=$3,replay_count=replay_count+1,attempt_count=0,lease_owner=NULL,lease_expires_at=NULL,state='PLANNED',conflict_reason=NULL,version=version+1 WHERE id=$1 AND plan_id=$2 AND dead_lettered_at IS NOT NULL AND version=$4`, occurrenceID, planID, reason, expectedVersion)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("dead-lettered occurrence not found")
	}
	if err = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "cloud_kitchen.occurrence.replayed", "cloud_kitchen.occurrence.replayed", "cloud_kitchen_occurrence", occurrenceID, branch, version+1, map[string]any{"dead_lettered": true, "attempt_count": attemptCount, "replay_count": replayCount, "version": version}, map[string]any{"state": "PLANNED", "replay_count": replayCount + 1, "version": version + 1}, map[string]any{"occurrence_id": occurrenceID, "reason": reason}, ""); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) RetryOccurrence(ctx context.Context, occurrenceID, reason string) error {
	if len(reason) > 280 {
		return fmt.Errorf("reason is too long")
	}
	_, err := s.db.Exec(ctx, `UPDATE cloud_kitchen_occurrences SET state='PLANNED',conflict_reason=NULL,last_error=$2,lease_owner=NULL,lease_expires_at=NULL,version=version+1 WHERE id=$1 AND dead_lettered_at IS NULL`, occurrenceID, reason)
	return err
}

func (s *Service) RetryOccurrenceForBusiness(ctx context.Context, business, planID, occurrenceID, reason string, expectedVersion int64) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var branch, state string
	var version int64
	if err = tx.QueryRow(ctx, `SELECT p.branch_id,o.state,o.version FROM cloud_kitchen_occurrences o JOIN cloud_kitchen_corporate_plans p ON p.id=o.plan_id WHERE o.id=$1 AND o.plan_id=$2 AND p.business_id=$3 FOR UPDATE`, occurrenceID, planID, business).Scan(&branch, &state, &version); err != nil {
		return err
	}
	if expectedVersion <= 0 || expectedVersion != version {
		return apperrors.Conflict("The occurrence changed; reload and retry.", nil)
	}
	tag, err := tx.Exec(ctx, `UPDATE cloud_kitchen_occurrences SET state='PLANNED',conflict_reason=NULL,last_error=$3,lease_owner=NULL,lease_expires_at=NULL,version=version+1 WHERE id=$1 AND plan_id=$2 AND state='CAPACITY_CONFLICT' AND version=$4`, occurrenceID, planID, reason, expectedVersion)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperrors.Conflict("The occurrence is not retryable.", nil)
	}
	if s.production != nil {
		if err = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "cloud_kitchen.occurrence.retried", "cloud_kitchen.occurrence.retried", "cloud_kitchen_occurrence", occurrenceID, branch, version+1, map[string]any{"state": state, "version": version}, map[string]any{"state": "PLANNED", "version": version + 1}, nil, ""); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Service) RescheduleOccurrence(ctx context.Context, business, planID, occurrenceID string, start, end time.Time, expectedVersion int64) error {
	if !end.After(start) {
		return fmt.Errorf("invalid occurrence window")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var branch, state string
	var previousStart, previousEnd time.Time
	var version int64
	if err = tx.QueryRow(ctx, `SELECT p.branch_id,o.state,o.starts_at,o.ends_at,o.version FROM cloud_kitchen_occurrences o JOIN cloud_kitchen_corporate_plans p ON p.id=o.plan_id WHERE o.id=$1 AND o.plan_id=$2 AND p.business_id=$3 FOR UPDATE`, occurrenceID, planID, business).Scan(&branch, &state, &previousStart, &previousEnd, &version); err != nil {
		return err
	}
	if expectedVersion <= 0 || expectedVersion != version {
		return apperrors.Conflict("The occurrence changed; reload and retry.", nil)
	}
	tag, err := tx.Exec(ctx, `UPDATE cloud_kitchen_occurrences SET starts_at=$1,ends_at=$2,state='PLANNED',conflict_reason=NULL,last_error='',lease_owner=NULL,lease_expires_at=NULL,version=version+1 WHERE id=$3 AND plan_id=$4 AND state IN ('PLANNED','CAPACITY_CONFLICT') AND version=$5`, start, end, occurrenceID, planID, expectedVersion)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperrors.Conflict("The occurrence is not reschedulable.", nil)
	}
	if s.production != nil {
		if err = s.production.RecordMutation(ctx, tx, production.Actor{VendorID: business}, "cloud_kitchen.occurrence.rescheduled", "cloud_kitchen.occurrence.rescheduled", "cloud_kitchen_occurrence", occurrenceID, branch, version+1, map[string]any{"starts_at": previousStart, "ends_at": previousEnd, "state": state, "version": version}, map[string]any{"starts_at": start, "ends_at": end, "state": "PLANNED", "version": version + 1}, nil, ""); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Service) CancelOccurrence(ctx context.Context, actor production.Actor, scope production.Scope, planID, occurrenceID, reason string, expectedVersion int64) error {
	if len(strings.TrimSpace(reason)) == 0 || len(reason) > 280 {
		return fmt.Errorf("a bounded cancellation reason is required")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var branch, state string
	var reservationID, productionJobID *string
	var version int64
	if err = tx.QueryRow(ctx, `SELECT p.branch_id,o.state,o.reservation_id,o.production_job_id,o.version FROM cloud_kitchen_occurrences o JOIN cloud_kitchen_corporate_plans p ON p.id=o.plan_id WHERE o.id=$1 AND o.plan_id=$2 AND p.business_id=$3 FOR UPDATE`, occurrenceID, planID, actor.VendorID).Scan(&branch, &state, &reservationID, &productionJobID, &version); err != nil || !scope.Covers(branch) {
		return fmt.Errorf("occurrence not found")
	}
	if expectedVersion <= 0 || expectedVersion != version {
		return apperrors.Conflict("The occurrence changed; reload and retry.", nil)
	}
	if state == "COMPLETED" {
		return fmt.Errorf("completed occurrence cannot be cancelled")
	}
	if productionJobID != nil && *productionJobID != "" {
		var jobStatus string
		if err = tx.QueryRow(ctx, `SELECT status FROM production_jobs WHERE business_id=$1 AND id=$2 FOR UPDATE`, actor.VendorID, *productionJobID).Scan(&jobStatus); err != nil {
			return err
		}
		if jobStatus == "IN_PROGRESS" || jobStatus == "READY" || jobStatus == "COMPLETED" {
			return fmt.Errorf("production already started")
		}
		if jobStatus != "CANCELLED" {
			if _, err = tx.Exec(ctx, `UPDATE production_jobs SET status='CANCELLED',updated_at=now(),version=version+1 WHERE business_id=$1 AND id=$2`, actor.VendorID, *productionJobID); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO production_job_events(id,business_id,job_id,event_type,from_status,to_status,actor_type,actor_id,correlation_id,note) VALUES($1,$2,$3,'CANCELLED',$4,'CANCELLED','vendor_staff',NULLIF($5,''),NULLIF($6,''),$7)`, "pje_"+uuid.NewString(), actor.VendorID, *productionJobID, jobStatus, actor.StaffID, actor.CorrelationID, reason); err != nil {
				return err
			}
		}
	}
	if reservationID != nil && *reservationID != "" {
		if _, _, err = s.production.CloseReservationInTx(ctx, tx, actor, scope, *reservationID, production.ReservationCancelled, reason); err != nil {
			return err
		}
	}
	tag, err := tx.Exec(ctx, `UPDATE cloud_kitchen_occurrences SET state='CANCELLED',conflict_reason=NULL,last_error='',lease_owner=NULL,lease_expires_at=NULL,version=version+1 WHERE id=$1 AND plan_id=$2 AND version=$3 AND state<>'CANCELLED'`, occurrenceID, planID, expectedVersion)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperrors.Conflict("The occurrence cannot be cancelled.", nil)
	}
	if err = s.production.RecordMutation(ctx, tx, actor, "cloud_kitchen.occurrence.cancelled", "cloud_kitchen.occurrence.cancelled", "cloud_kitchen_occurrence", occurrenceID, branch, version+1, map[string]any{"state": state, "version": version}, map[string]any{"state": "CANCELLED", "version": version + 1}, map[string]any{"occurrence_id": occurrenceID, "reason": reason}, ""); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// MaterializeAll is a bounded, idempotent worker pass. The occurrence key and
// unique constraint are the correctness boundary, so two instances cannot
// duplicate the same future occurrence.
func (s *Service) MaterializeAll(ctx context.Context, limit int) (int, error) {
	rows, err := s.db.Query(ctx, `SELECT DISTINCT business_id FROM cloud_kitchen_corporate_plans WHERE status='ACTIVE' LIMIT 100`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	total := 0
	for rows.Next() {
		var business string
		if err := rows.Scan(&business); err != nil {
			return total, err
		}
		n, err := s.Materialize(ctx, business, limit)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, rows.Err()
}
