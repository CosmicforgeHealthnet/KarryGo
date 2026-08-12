// Package repositories persists Section 15 packages, scans and custody events.
package repositories

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/packages/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

var (
	ErrNotFound           = errors.New("packages: not found")
	ErrIdentifierConflict = errors.New("packages: identifier conflict")
	ErrVersionConflict    = errors.New("packages: version conflict")
	ErrScanConflict       = errors.New("packages: client scan conflict")
)

const (
	PrefixPackage = "pkg"
	PrefixHistory = "pkh"
	PrefixScan    = "pks"
	PrefixCustody = "cdy"
)

type Repository struct{ db database.Querier }

func NewRepository(db database.Querier) *Repository { return &Repository{db: db} }
func (r *Repository) q(q database.Querier) database.Querier {
	if q != nil {
		return q
	}
	return r.db
}
func nullable(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

const packageColumns = `id, organization_id, COALESCE(branch_id,''), shipment_id, package_code,
 barcode_value, qr_value, package_type, length::text, width::text, height::text,
 dimension_unit, weight::text, weight_unit, declared_value_amount_minor,
 declared_value_currency, handling_instructions, condition, status, version,
 created_by, updated_by, created_at, updated_at, archived_at`

func scanPackage(row pgx.Row) (models.Package, error) {
	var p models.Package
	err := row.Scan(&p.ID, &p.OrganizationID, &p.BranchID, &p.ShipmentID, &p.PackageCode,
		&p.BarcodeValue, &p.QRValue, &p.PackageType, &p.Length, &p.Width, &p.Height, &p.DimensionUnit,
		&p.Weight, &p.WeightUnit, &p.DeclaredValueAmountMinor, &p.DeclaredValueCurrency,
		&p.HandlingInstructions, &p.Condition, &p.Status, &p.Version, &p.CreatedBy, &p.UpdatedBy,
		&p.CreatedAt, &p.UpdatedAt, &p.ArchivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

func (r *Repository) Create(ctx context.Context, q database.Querier, p models.Package) (models.Package, error) {
	created, err := scanPackage(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_packages
 (id,organization_id,branch_id,shipment_id,package_code,barcode_value,qr_value,package_type,
 length,width,height,dimension_unit,weight,weight_unit,declared_value_amount_minor,
 declared_value_currency,handling_instructions,condition,status,created_by,updated_by)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::numeric,$10::numeric,$11::numeric,$12,$13::numeric,$14,$15,$16,$17,$18,$19,$20,$20)
 RETURNING `+packageColumns, p.ID, p.OrganizationID, nullable(p.BranchID), p.ShipmentID, p.PackageCode,
		p.BarcodeValue, p.QRValue, p.PackageType, p.Length, p.Width, p.Height, p.DimensionUnit, p.Weight,
		p.WeightUnit, p.DeclaredValueAmountMinor, p.DeclaredValueCurrency, p.HandlingInstructions, p.Condition, p.Status, p.CreatedBy))
	if err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" {
			return models.Package{}, ErrIdentifierConflict
		}
		return models.Package{}, fmt.Errorf("packages: create: %w", err)
	}
	return created, nil
}

func (r *Repository) Get(ctx context.Context, q database.Querier, org, id string) (models.Package, error) {
	return scanPackage(r.q(q).QueryRow(ctx, `SELECT `+packageColumns+` FROM fleet_partner_packages WHERE organization_id=$1 AND id=$2`, org, id))
}
func (r *Repository) Lock(ctx context.Context, q database.Querier, org, id string) (models.Package, error) {
	return scanPackage(r.q(q).QueryRow(ctx, `SELECT `+packageColumns+` FROM fleet_partner_packages WHERE organization_id=$1 AND id=$2 FOR UPDATE`, org, id))
}
func (r *Repository) ListByShipment(ctx context.Context, q database.Querier, org, shipment string) ([]models.Package, error) {
	rows, err := r.q(q).Query(ctx, `SELECT `+packageColumns+` FROM fleet_partner_packages WHERE organization_id=$1 AND shipment_id=$2 ORDER BY created_at,id`, org, shipment)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Package
	for rows.Next() {
		p, e := scanPackage(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *Repository) Transition(ctx context.Context, q database.Querier, org, id string, version int64, status, condition, actor string) (models.Package, error) {
	p, err := scanPackage(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_packages SET status=$1,condition=$2,updated_by=$3,version=version+1,updated_at=now() WHERE organization_id=$4 AND id=$5 AND version=$6 RETURNING `+packageColumns, status, condition, actor, org, id, version))
	if errors.Is(err, ErrNotFound) {
		return p, ErrVersionConflict
	}
	return p, err
}

func (r *Repository) IncrementShipmentPackageCount(ctx context.Context, q database.Querier, org, shipment string) error {
	cmd, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_shipments SET package_count=package_count+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, org, shipment)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) AppendHistory(ctx context.Context, q database.Querier, h models.History) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_package_history
 (id,organization_id,package_id,previous_status,new_status,reason_code,safe_note,actor_type,actor_reference,source,request_id,correlation_id,occurred_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, ids.New(PrefixHistory), h.OrganizationID, h.PackageID, h.PreviousStatus, h.NewStatus, h.ReasonCode, h.SafeNote, h.ActorType, h.ActorReference, h.Source, h.RequestID, h.CorrelationID, h.OccurredAt)
	return err
}
func (r *Repository) ListHistory(ctx context.Context, org, pkg string) ([]models.History, error) {
	rows, err := r.db.Query(ctx, `SELECT id,organization_id,package_id,previous_status,new_status,reason_code,safe_note,actor_type,actor_reference,source,request_id,correlation_id,occurred_at,created_at FROM fleet_partner_package_history WHERE organization_id=$1 AND package_id=$2 ORDER BY occurred_at,id`, org, pkg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.History
	for rows.Next() {
		var h models.History
		if e := rows.Scan(&h.ID, &h.OrganizationID, &h.PackageID, &h.PreviousStatus, &h.NewStatus, &h.ReasonCode, &h.SafeNote, &h.ActorType, &h.ActorReference, &h.Source, &h.RequestID, &h.CorrelationID, &h.OccurredAt, &h.CreatedAt); e != nil {
			return nil, e
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

const scanColumns = `id,organization_id,package_id,scan_type,barcode_value,qr_value,location_reference,device_reference,client_scan_id,request_fingerprint,disposition,actor_type,actor_reference,occurred_at,received_at`

func scanScan(row pgx.Row) (models.Scan, error) {
	var s models.Scan
	err := row.Scan(&s.ID, &s.OrganizationID, &s.PackageID, &s.ScanType, &s.BarcodeValue, &s.QRValue, &s.LocationReference, &s.DeviceReference, &s.ClientScanID, &s.RequestFingerprint, &s.Disposition, &s.ActorType, &s.ActorReference, &s.OccurredAt, &s.ReceivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, ErrNotFound
	}
	return s, err
}
func (r *Repository) GetScanByClientID(ctx context.Context, q database.Querier, org, clientID string) (models.Scan, error) {
	return scanScan(r.q(q).QueryRow(ctx, `SELECT `+scanColumns+` FROM fleet_partner_package_scans WHERE organization_id=$1 AND client_scan_id=$2`, org, clientID))
}
func (r *Repository) LatestScan(ctx context.Context, q database.Querier, org, pkg string) (models.Scan, error) {
	return scanScan(r.q(q).QueryRow(ctx, `SELECT `+scanColumns+` FROM fleet_partner_package_scans WHERE organization_id=$1 AND package_id=$2 ORDER BY occurred_at DESC,received_at DESC LIMIT 1`, org, pkg))
}
func (r *Repository) CreateScan(ctx context.Context, q database.Querier, s models.Scan) (models.Scan, error) {
	created, err := scanScan(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_package_scans(id,organization_id,package_id,scan_type,barcode_value,qr_value,location_reference,device_reference,client_scan_id,request_fingerprint,disposition,actor_type,actor_reference,occurred_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING `+scanColumns, s.ID, s.OrganizationID, s.PackageID, s.ScanType, s.BarcodeValue, s.QRValue, s.LocationReference, s.DeviceReference, s.ClientScanID, s.RequestFingerprint, s.Disposition, s.ActorType, s.ActorReference, s.OccurredAt))
	if err != nil {
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" {
			return models.Scan{}, ErrScanConflict
		}
		return models.Scan{}, err
	}
	return created, nil
}

func (r *Repository) CreateCustody(ctx context.Context, q database.Querier, e models.CustodyEvent) (models.CustodyEvent, error) {
	e.ID = ids.New(PrefixCustody)
	err := r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_custody_events(id,organization_id,package_id,shipment_id,manifest_id,from_actor_type,from_actor_reference,to_actor_type,to_actor_reference,from_location_reference,to_location_reference,condition,evidence_media_id,correction_of_event_id,occurred_at,source,correlation_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17) RETURNING received_at,created_at`, e.ID, e.OrganizationID, e.PackageID, e.ShipmentID, nullable(e.ManifestID), e.FromActorType, e.FromActorReference, e.ToActorType, e.ToActorReference, e.FromLocationReference, e.ToLocationReference, e.Condition, e.EvidenceMediaID, nullable(e.CorrectionOfEventID), e.OccurredAt, e.Source, e.CorrelationID).Scan(&e.ReceivedAt, &e.CreatedAt)
	return e, err
}

const custodyColumns = `id,organization_id,package_id,shipment_id,COALESCE(manifest_id,''),from_actor_type,from_actor_reference,to_actor_type,to_actor_reference,from_location_reference,to_location_reference,condition,evidence_media_id,COALESCE(correction_of_event_id,''),occurred_at,received_at,source,correlation_id,created_at`

func scanCustody(row pgx.Row) (models.CustodyEvent, error) {
	var e models.CustodyEvent
	err := row.Scan(&e.ID, &e.OrganizationID, &e.PackageID, &e.ShipmentID, &e.ManifestID,
		&e.FromActorType, &e.FromActorReference, &e.ToActorType, &e.ToActorReference,
		&e.FromLocationReference, &e.ToLocationReference, &e.Condition, &e.EvidenceMediaID,
		&e.CorrectionOfEventID, &e.OccurredAt, &e.ReceivedAt, &e.Source, &e.CorrelationID, &e.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, ErrNotFound
	}
	return e, err
}

func (r *Repository) LatestCustody(ctx context.Context, q database.Querier, org, pkg string) (models.CustodyEvent, error) {
	return scanCustody(r.q(q).QueryRow(ctx, `SELECT `+custodyColumns+` FROM fleet_partner_custody_events
		WHERE organization_id=$1 AND package_id=$2 ORDER BY occurred_at DESC,id DESC LIMIT 1`, org, pkg))
}

func (r *Repository) GetCustody(ctx context.Context, q database.Querier, org, pkg, id string) (models.CustodyEvent, error) {
	return scanCustody(r.q(q).QueryRow(ctx, `SELECT `+custodyColumns+` FROM fleet_partner_custody_events
		WHERE organization_id=$1 AND package_id=$2 AND id=$3`, org, pkg, id))
}

func (r *Repository) ListCustody(ctx context.Context, org, pkg string) ([]models.CustodyEvent, error) {
	rows, err := r.db.Query(ctx, `SELECT `+custodyColumns+` FROM fleet_partner_custody_events WHERE organization_id=$1 AND package_id=$2 ORDER BY occurred_at,id`, org, pkg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.CustodyEvent
	for rows.Next() {
		e, x := scanCustody(rows)
		if x != nil {
			return nil, x
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
