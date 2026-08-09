// Package repositories persists manifests and immutable closure snapshots.
package repositories

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/manifests/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound        = errors.New("manifests: not found")
	ErrConflict        = errors.New("manifests: conflict")
	ErrVersionConflict = errors.New("manifests: version conflict")
)

const (
	PrefixManifest = "mft"
	PrefixItem     = "mfi"
	PrefixHistory  = "mfh"
	PrefixSnapshot = "mfs"
)

type Repository struct{ db database.Querier }

func NewRepository(db database.Querier) *Repository { return &Repository{db} }
func (r *Repository) q(q database.Querier) database.Querier {
	if q != nil {
		return q
	}
	return r.db
}

const cols = `id,organization_id,branch_id,manifest_reference,route_reference,COALESCE(future_route_plan_id,''),vehicle_reference,provider_membership_id,seal_number,status,version,created_by,updated_by,created_at,updated_at,closed_at,dispatched_at,completed_at,cancelled_at`

func scan(row pgx.Row) (models.Manifest, error) {
	var m models.Manifest
	e := row.Scan(&m.ID, &m.OrganizationID, &m.BranchID, &m.ManifestReference, &m.RouteReference, &m.FutureRoutePlanID, &m.VehicleReference, &m.ProviderMembershipID, &m.SealNumber, &m.Status, &m.Version, &m.CreatedBy, &m.UpdatedBy, &m.CreatedAt, &m.UpdatedAt, &m.ClosedAt, &m.DispatchedAt, &m.CompletedAt, &m.CancelledAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, e
}
func (r *Repository) Create(ctx context.Context, q database.Querier, m models.Manifest) (models.Manifest, error) {
	v, e := scan(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_manifests(id,organization_id,branch_id,manifest_reference,route_reference,vehicle_reference,provider_membership_id,seal_number,status,created_by,updated_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10) RETURNING `+cols, m.ID, m.OrganizationID, m.BranchID, m.ManifestReference, m.RouteReference, m.VehicleReference, m.ProviderMembershipID, m.SealNumber, m.Status, m.CreatedBy))
	if e != nil {
		var pe *pgconn.PgError
		if errors.As(e, &pe) && pe.Code == "23505" {
			return v, ErrConflict
		}
	}
	return v, e
}
func (r *Repository) Get(ctx context.Context, q database.Querier, org, id string) (models.Manifest, error) {
	return scan(r.q(q).QueryRow(ctx, `SELECT `+cols+` FROM fleet_partner_manifests WHERE organization_id=$1 AND id=$2`, org, id))
}
func (r *Repository) Lock(ctx context.Context, q database.Querier, org, id string) (models.Manifest, error) {
	return scan(r.q(q).QueryRow(ctx, `SELECT `+cols+` FROM fleet_partner_manifests WHERE organization_id=$1 AND id=$2 FOR UPDATE`, org, id))
}
func (r *Repository) List(ctx context.Context, org, status, branch string) ([]models.Manifest, error) {
	rows, e := r.db.Query(ctx, `SELECT `+cols+` FROM fleet_partner_manifests WHERE organization_id=$1 AND($2='' OR status=$2)AND($3='' OR branch_id=$3) ORDER BY created_at DESC,id DESC LIMIT 100`, org, status, branch)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []models.Manifest
	for rows.Next() {
		m, x := scan(rows)
		if x != nil {
			return nil, x
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (r *Repository) Update(ctx context.Context, q database.Querier, org, id string, version int64, route, vehicle, provider, seal, actor string) (models.Manifest, error) {
	m, e := scan(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_manifests SET route_reference=$1,vehicle_reference=$2,provider_membership_id=$3,seal_number=$4,updated_by=$5,version=version+1,updated_at=now() WHERE organization_id=$6 AND id=$7 AND version=$8 RETURNING `+cols, route, vehicle, provider, seal, actor, org, id, version))
	if errors.Is(e, ErrNotFound) {
		return m, ErrVersionConflict
	}
	return m, e
}
func (r *Repository) SetStatus(ctx context.Context, q database.Querier, org, id string, version int64, status, actor string) (models.Manifest, error) {
	m, e := scan(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_manifests SET status=$1,updated_by=$2,version=version+1,updated_at=now(),closed_at=CASE WHEN $1='CLOSED' THEN now() ELSE closed_at END,dispatched_at=CASE WHEN $1='DISPATCHED' THEN now() ELSE dispatched_at END,completed_at=CASE WHEN $1='COMPLETED' THEN now() ELSE completed_at END,cancelled_at=CASE WHEN $1='CANCELLED' THEN now() ELSE cancelled_at END WHERE organization_id=$3 AND id=$4 AND version=$5 RETURNING `+cols, status, actor, org, id, version))
	if errors.Is(e, ErrNotFound) {
		return m, ErrVersionConflict
	}
	return m, e
}

func (r *Repository) AmendSeal(ctx context.Context, q database.Querier, org, id string,
	version int64, seal, actor string) (models.Manifest, error) {
	m, e := scan(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_manifests SET seal_number=$1,
		updated_by=$2,version=version+1,updated_at=now() WHERE organization_id=$3 AND id=$4
		AND version=$5 AND status='CLOSED' RETURNING `+cols, seal, actor, org, id, version))
	if errors.Is(e, ErrNotFound) {
		return m, ErrVersionConflict
	}
	return m, e
}

const itemCols = `id,organization_id,manifest_id,item_type,item_id,COALESCE(shipment_id,''),added_by,created_at`

func scanItem(row pgx.Row) (models.Item, error) {
	var i models.Item
	e := row.Scan(&i.ID, &i.OrganizationID, &i.ManifestID, &i.ItemType, &i.ItemID, &i.ShipmentID, &i.AddedBy, &i.CreatedAt)
	return i, e
}
func (r *Repository) AddItem(ctx context.Context, q database.Querier, i models.Item) (models.Item, error) {
	x, e := scanItem(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_manifest_items(id,organization_id,manifest_id,item_type,item_id,shipment_id,added_by)VALUES($1,$2,$3,$4,$5,NULLIF($6,''),$7)RETURNING `+itemCols, i.ID, i.OrganizationID, i.ManifestID, i.ItemType, i.ItemID, i.ShipmentID, i.AddedBy))
	if e != nil {
		var pe *pgconn.PgError
		if errors.As(e, &pe) && pe.Code == "23505" {
			return x, ErrConflict
		}
	}
	return x, e
}
func (r *Repository) ListItems(ctx context.Context, q database.Querier, org, id string) ([]models.Item, error) {
	rows, e := r.q(q).Query(ctx, `SELECT `+itemCols+` FROM fleet_partner_manifest_items WHERE organization_id=$1 AND manifest_id=$2 ORDER BY created_at,id`, org, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []models.Item
	for rows.Next() {
		i, x := scanItem(rows)
		if x != nil {
			return nil, x
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
func (r *Repository) HasActiveMembership(ctx context.Context, q database.Querier, org, kind, item, except string) (bool, error) {
	var yes bool
	e := r.q(q).QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_manifest_items i JOIN fleet_partner_manifests m ON m.id=i.manifest_id WHERE i.organization_id=$1 AND i.item_type=$2 AND i.item_id=$3 AND i.manifest_id<>$4 AND m.status IN('DRAFT','OPEN','CLOSED','DISPATCHED'))`, org, kind, item, except).Scan(&yes)
	return yes, e
}
func (r *Repository) AppendHistory(ctx context.Context, q database.Querier, org, id, from, to, reason, actorType, actor, corr string) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_manifest_history(id,organization_id,manifest_id,previous_status,new_status,reason_code,actor_type,actor_reference,correlation_id)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, ids.New(PrefixHistory), org, id, from, to, reason, actorType, actor, corr)
	return e
}
func (r *Repository) CreateSnapshot(ctx context.Context, q database.Querier, s models.Snapshot) (models.Snapshot, error) {
	raw, e := json.Marshal(s.Snapshot)
	if e != nil {
		return s, e
	}
	s.ID = ids.New(PrefixSnapshot)
	e = r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_manifest_snapshots(id,organization_id,manifest_id,manifest_version,snapshot,request_fingerprint,created_by)VALUES($1,$2,$3,$4,$5,$6,$7)RETURNING created_at`, s.ID, s.OrganizationID, s.ManifestID, s.ManifestVersion, raw, s.RequestFingerprint, s.CreatedBy).Scan(&s.CreatedAt)
	if e != nil {
		return s, fmt.Errorf("manifests: snapshot: %w", e)
	}
	return s, nil
}

func (r *Repository) GetSnapshot(ctx context.Context, q database.Querier, org, manifest string) (models.Snapshot, error) {
	var s models.Snapshot
	var raw []byte
	err := r.q(q).QueryRow(ctx, `SELECT id,organization_id,manifest_id,manifest_version,snapshot,
		request_fingerprint,created_by,created_at FROM fleet_partner_manifest_snapshots
		WHERE organization_id=$1 AND manifest_id=$2`, org, manifest).Scan(&s.ID, &s.OrganizationID,
		&s.ManifestID, &s.ManifestVersion, &raw, &s.RequestFingerprint, &s.CreatedBy, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(raw, &s.Snapshot)
	}
	return s, err
}
