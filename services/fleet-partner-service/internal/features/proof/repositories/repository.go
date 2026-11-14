// Package repositories is the PostgreSQL persistence of Section 19 proof
// policies, policy snapshots, proof records and verifications. Every
// statement is organization-scoped; a missing row is pgx.ErrNoRows.
package repositories

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/proof/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// Repository persists proof aggregates. Methods take the Querier to run on so
// they compose into the caller's transaction; nil means the pool.
type Repository struct{ db database.Querier }

func NewRepository(db database.Querier) *Repository { return &Repository{db: db} }

func (r *Repository) q(q database.Querier) database.Querier {
	if q == nil {
		return r.db
	}
	return q
}

const policyCols = `id,organization_id,policy_scope,scope_reference,service_type,stop_type,required_proof_types,minimum_count,location_required,time_window_rules,failure_policy,status,version,created_by,created_at,updated_at`

func scanPolicy(row pgx.Row) (models.Policy, error) {
	var v models.Policy
	e := row.Scan(&v.ID, &v.OrganizationID, &v.PolicyScope, &v.ScopeReference, &v.ServiceType, &v.StopType, &v.RequiredProofTypes, &v.MinimumCount, &v.LocationRequired, &v.TimeWindowRules, &v.FailurePolicy, &v.Status, &v.Version, &v.CreatedBy, &v.CreatedAt, &v.UpdatedAt)
	return v, e
}

func (r *Repository) InsertPolicy(ctx context.Context, q database.Querier, v models.Policy) (models.Policy, error) {
	return scanPolicy(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_proof_policies(id,organization_id,policy_scope,scope_reference,service_type,stop_type,required_proof_types,minimum_count,location_required,time_window_rules,failure_policy,status,created_by)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING `+policyCols, v.ID, v.OrganizationID, v.PolicyScope, v.ScopeReference, v.ServiceType, v.StopType, v.RequiredProofTypes, v.MinimumCount, v.LocationRequired, v.TimeWindowRules, v.FailurePolicy, v.Status, v.CreatedBy))
}

func (r *Repository) UpdatePolicy(ctx context.Context, q database.Querier, org, id string, expected int64, v models.Policy) (models.Policy, error) {
	return scanPolicy(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_proof_policies SET required_proof_types=$4,minimum_count=$5,location_required=$6,time_window_rules=$7,failure_policy=$8,status=$9,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND version=$3 AND status<>'ARCHIVED' RETURNING `+policyCols, org, id, expected, v.RequiredProofTypes, v.MinimumCount, v.LocationRequired, v.TimeWindowRules, v.FailurePolicy, v.Status))
}

func (r *Repository) InsertSnapshot(ctx context.Context, q database.Querier, snap *models.PolicySnapshot) error {
	return r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_proof_policy_versions(id,organization_id,policy_id,policy_version,snapshot,published_by)VALUES($1,$2,$3,$4,$5,$6) RETURNING published_at`, snap.ID, snap.OrganizationID, snap.PolicyID, snap.PolicyVersion, snap.Snapshot, snap.PublishedBy).Scan(&snap.PublishedAt)
}

func (r *Repository) GetPolicy(ctx context.Context, org, id string) (models.Policy, error) {
	return scanPolicy(r.db.QueryRow(ctx, `SELECT `+policyCols+` FROM fleet_partner_proof_policies WHERE organization_id=$1 AND id=$2`, org, id))
}

func (r *Repository) ListPolicies(ctx context.Context, org string) ([]models.Policy, error) {
	rows, e := r.db.Query(ctx, `SELECT `+policyCols+` FROM fleet_partner_proof_policies WHERE organization_id=$1 ORDER BY created_at DESC`, org)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []models.Policy{}
	for rows.Next() {
		v, e := scanPolicy(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repository) GetSnapshot(ctx context.Context, org, id string) (models.PolicySnapshot, error) {
	var snap models.PolicySnapshot
	e := r.db.QueryRow(ctx, `SELECT id,organization_id,policy_id,policy_version,snapshot,published_by,published_at FROM fleet_partner_proof_policy_versions WHERE organization_id=$1 AND id=$2`, org, id).Scan(&snap.ID, &snap.OrganizationID, &snap.PolicyID, &snap.PolicyVersion, &snap.Snapshot, &snap.PublishedBy, &snap.PublishedAt)
	return snap, e
}

const recordCols = `id,organization_id,policy_snapshot_reference,proof_type,order_id,shipment_id,COALESCE(package_id,''),route_stop_id,assignment_id,canonical_trip_reference,actor_type,actor_reference,media_file_id,value_hash,safe_value_metadata,latitude,longitude,accuracy,source_timestamp,received_at,source,verification_status,source_version,idempotency_key,version,created_at`

func scanRecord(row pgx.Row) (models.Record, error) {
	var v models.Record
	e := row.Scan(&v.ID, &v.OrganizationID, &v.PolicySnapshotReference, &v.ProofType, &v.OrderID, &v.ShipmentID, &v.PackageID, &v.RouteStopID, &v.AssignmentID, &v.CanonicalTripReference, &v.ActorType, &v.ActorReference, &v.MediaFileID, &v.ValueHash, &v.SafeValueMetadata, &v.Latitude, &v.Longitude, &v.Accuracy, &v.SourceTimestamp, &v.ReceivedAt, &v.Source, &v.VerificationStatus, &v.SourceVersion, &v.IdempotencyKey, &v.Version, &v.CreatedAt)
	return v, e
}

func (r *Repository) GetRecordByIdempotencyKey(ctx context.Context, org, key string) (models.Record, error) {
	return scanRecord(r.db.QueryRow(ctx, `SELECT `+recordCols+` FROM fleet_partner_proof_records WHERE organization_id=$1 AND idempotency_key=$2`, org, key))
}

// GetRecord reads one record; share takes a FOR SHARE row lock.
func (r *Repository) GetRecord(ctx context.Context, q database.Querier, org, id string, share bool) (models.Record, error) {
	lock := ""
	if share {
		lock = " FOR SHARE"
	}
	return scanRecord(r.q(q).QueryRow(ctx, `SELECT `+recordCols+` FROM fleet_partner_proof_records WHERE organization_id=$1 AND id=$2`+lock, org, id))
}

func (r *Repository) InsertRecord(ctx context.Context, q database.Querier, v models.Record) (models.Record, error) {
	return scanRecord(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_proof_records(id,organization_id,policy_snapshot_reference,proof_type,order_id,shipment_id,package_id,route_stop_id,assignment_id,canonical_trip_reference,actor_type,actor_reference,media_file_id,value_hash,safe_value_metadata,latitude,longitude,accuracy,source_timestamp,source,verification_status,source_version,idempotency_key)VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23) RETURNING `+recordCols, v.ID, v.OrganizationID, v.PolicySnapshotReference, v.ProofType, v.OrderID, v.ShipmentID, v.PackageID, v.RouteStopID, v.AssignmentID, v.CanonicalTripReference, v.ActorType, v.ActorReference, v.MediaFileID, v.ValueHash, v.SafeValueMetadata, v.Latitude, v.Longitude, v.Accuracy, v.SourceTimestamp, v.Source, v.VerificationStatus, v.SourceVersion, v.IdempotencyKey))
}

// InsertMediaVerified records the media-safety verification taken at
// submission time.
func (r *Repository) InsertMediaVerified(ctx context.Context, q database.Querier, org, recordID, mediaID string, sourceVersion int64) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_proof_verifications(id,organization_id,proof_record_id,status,authority_type,authority_reference,source_version,reason_codes)VALUES($1,$2,$3,'VERIFIED','MEDIA',$4,$5,'["MEDIA_SAFE_AND_OWNED"]')`, ids.New("prfver"), org, recordID, mediaID, sourceVersion)
	return e
}

func (r *Repository) ListRecords(ctx context.Context, org string) ([]models.Record, error) {
	rows, e := r.db.Query(ctx, `SELECT `+recordCols+` FROM fleet_partner_proof_records WHERE organization_id=$1 ORDER BY created_at DESC LIMIT 200`, org)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []models.Record{}
	for rows.Next() {
		v, e := scanRecord(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// CountSatisfying counts verified, not rejected records of one proof type for
// a snapshot and subject.
func (r *Repository) CountSatisfying(ctx context.Context, org, snapshot, proofType, order, shipment, pkg, stop string) (int, error) {
	var n int
	e := r.db.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_proof_records r WHERE r.organization_id=$1 AND r.policy_snapshot_reference=$2 AND r.proof_type=$3 AND($4=''OR r.order_id=$4)AND($5=''OR r.shipment_id=$5)AND($6=''OR r.package_id=$6)AND($7=''OR r.route_stop_id=$7) AND(r.verification_status='VERIFIED' OR EXISTS(SELECT 1 FROM fleet_partner_proof_verifications v WHERE v.proof_record_id=r.id AND v.status='VERIFIED')) AND NOT EXISTS(SELECT 1 FROM fleet_partner_proof_verifications v WHERE v.proof_record_id=r.id AND v.status='REJECTED')`, org, snapshot, strings.ToUpper(proofType), order, shipment, pkg, stop).Scan(&n)
	return n, e
}

func (r *Repository) ListVerifications(ctx context.Context, q database.Querier, org, id string) ([]models.Verification, error) {
	rows, e := r.q(q).Query(ctx, `SELECT id,organization_id,proof_record_id,status,authority_type,authority_reference,source_version,reason_codes,verified_at FROM fleet_partner_proof_verifications WHERE organization_id=$1 AND proof_record_id=$2 ORDER BY verified_at,id`, org, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []models.Verification{}
	for rows.Next() {
		var v models.Verification
		if e = rows.Scan(&v.ID, &v.OrganizationID, &v.ProofRecordID, &v.Status, &v.AuthorityType, &v.AuthorityReference, &v.SourceVersion, &v.ReasonCodes, &v.VerifiedAt); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// LockDecision serializes decisions on one record for the transaction.
func (r *Repository) LockDecision(ctx context.Context, q database.Querier, recordID string) error {
	_, e := r.q(q).Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "proof-decision:"+recordID)
	return e
}

func (r *Repository) InsertDecision(ctx context.Context, q database.Querier, org, recordID, decision, authority, actor string, reasonCodes []byte) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_proof_verifications(id,organization_id,proof_record_id,status,authority_type,authority_reference,source_version,reason_codes)VALUES($1,$2,$3,$4,$5,$6,0,$7)`, ids.New("prfver"), org, recordID, decision, authority, actor, reasonCodes)
	return e
}

// ActiveOrganizationIDs lists organizations eligible for proof sweeps.
func (r *Repository) ActiveOrganizationIDs(ctx context.Context) ([]string, error) {
	rows, e := r.db.Query(ctx, `SELECT id FROM fleet_partner_organizations WHERE status='ACTIVE'`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var org string
		if e = rows.Scan(&org); e != nil {
			return nil, e
		}
		out = append(out, org)
	}
	return out, rows.Err()
}

func (r *Repository) ListPendingMediaRecords(ctx context.Context, org string) ([]models.PendingMediaRecord, error) {
	rows, e := r.db.Query(ctx, `SELECT r.id,r.media_file_id,r.source_version FROM fleet_partner_proof_records r WHERE r.organization_id=$1 AND r.media_file_id<>'' AND NOT EXISTS(SELECT 1 FROM fleet_partner_proof_verifications v WHERE v.proof_record_id=r.id AND v.status='VERIFIED') LIMIT 100`, org)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []models.PendingMediaRecord{}
	for rows.Next() {
		var v models.PendingMediaRecord
		if e = rows.Scan(&v.ID, &v.MediaFileID, &v.SourceVersion); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repository) InsertMediaReconciliation(ctx context.Context, org, recordID, status, mediaID string, sourceVersion int64, reason string) error {
	_, e := r.db.Exec(ctx, `INSERT INTO fleet_partner_proof_verifications(id,organization_id,proof_record_id,status,authority_type,authority_reference,source_version,reason_codes)VALUES($1,$2,$3,$4,'MEDIA',$5,$6,$7) ON CONFLICT DO NOTHING`, ids.New("prfver"), org, recordID, status, mediaID, sourceVersion, reason)
	return e
}
