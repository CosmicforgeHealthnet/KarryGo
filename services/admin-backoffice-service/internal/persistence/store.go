package persistence

import (
	"context"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/config"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/operations"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/security"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

var ErrReplay = errors.New("replayed security material")

type Audit struct {
	Zone                                                                                                                                                            string
	Correlation, Principal, Staff, Permission, PolicyVersion, TargetService, TargetRef, RequestType, Market, Organization, ReasonCode, Outcome, Status, EvidenceRef string
	SourceVersion                                                                                                                                                   int64
}
type Store struct{ Pool *pgxpool.Pool }

func (s *Store) Ready(ctx context.Context) error {
	var version int
	err := s.Pool.QueryRow(ctx, `SELECT version FROM admin_schema_versions WHERE version=3`).Scan(&version)
	return err
}

// ValidateRuntime forbids the application from running with the evidence owner
// or an administrative database identity, even if an operator misconfigures it.
func (s *Store) ValidateRuntime(ctx context.Context) error {
	var safe bool
	err := s.Pool.QueryRow(ctx, `SELECT NOT r.rolsuper AND NOT r.rolcreatedb AND NOT r.rolcreaterole AND NOT r.rolbypassrls AND NOT pg_has_role(current_user,c.relowner,'MEMBER') FROM pg_roles r CROSS JOIN pg_class c WHERE r.rolname=current_user AND c.oid='admin_audit_entries'::regclass`).Scan(&safe)
	if err != nil {
		return err
	}
	if !safe {
		return errors.New("runtime database role is privileged")
	}
	return nil
}
func (s *Store) Principal(ctx context.Context, fingerprint, uri, audience string, now time.Time) error {
	var ok bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admin_remote_principals WHERE principal=$1 AND certificate_sha256=$2 AND workload_uri=$3 AND product='karryam' AND audience=$4 AND revoked_at IS NULL AND expires_at>$5)`, config.HQPrincipal, fingerprint, uri, audience, now).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return security.ErrDenied
	}
	return nil
}
func (s *Store) Revoked(ctx context.Context, c security.Claims) (bool, error) {
	var revoked bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admin_revocations WHERE (kind='staff' AND reference=$1) OR (kind='session' AND reference=$2))`, c.Subject, c.SessionID).Scan(&revoked)
	return revoked, err
}

// Claim registers both identities atomically; a conflict consumes neither new key.
func (s *Store) Claim(ctx context.Context, nonce, jti string, expires time.Time) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// A fixed lock order prevents nonce/JTI deadlocks between concurrent claims.
	for _, claim := range [][2]string{{"nonce", nonce}, {"jti", jti}} {
		result, e := tx.Exec(ctx, `INSERT INTO admin_security_claims(kind,claim_hash,expires_at) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, claim[0], security.Hash(config.HQPrincipal+":"+claim[1]), expires)
		if e != nil {
			return e
		}
		if result.RowsAffected() != 1 {
			return ErrReplay
		}
	}
	return tx.Commit(ctx)
}
func (s *Store) Rate(ctx context.Context, key string, now time.Time, limit int) (bool, error) {
	var count int
	err := s.Pool.QueryRow(ctx, `INSERT INTO admin_rate_limits(key_hash,window_start,count) VALUES($1,$2,1) ON CONFLICT(key_hash,window_start) DO UPDATE SET count=LEAST(admin_rate_limits.count+1,$3+1) RETURNING count`, security.Hash(key), now.UTC().Truncate(time.Minute), limit).Scan(&count)
	return count <= limit, err
}
func (s *Store) Audit(ctx context.Context, a Audit) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO admin_audit_entries(correlation_id,product,principal,staff_ref,permission,policy_version,target_service,target_ref,request_type,market,organization,reason_code,outcome,source_version,operation_status,evidence_ref,zone) VALUES($1,'karryam',$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, a.Correlation, a.Principal, a.Staff, a.Permission, a.PolicyVersion, a.TargetService, a.TargetRef, a.RequestType, a.Market, a.Organization, a.ReasonCode, a.Outcome, a.SourceVersion, a.Status, a.EvidenceRef, a.Zone)
	return err
}

// RecordRejected is the only submission path until an owner command is certified.
// The request is durable, but no domain operation is implied or attempted.
func (s *Store) RecordRejected(ctx context.Context, c operations.Command, staff, digest string) (operations.Operation, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return operations.Operation{}, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO admin_delegated_operations(id,principal,staff_ref,market,organization,idempotency_key,request_digest,action,target_type,target_ref,expected_version,status) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'REJECTED') ON CONFLICT DO NOTHING`, c.CommandID, config.HQPrincipal, staff, c.Scope.Market, c.Scope.Organization, c.IdempotencyKey, digest, c.Action, c.Target.Type, c.Target.ID, c.Target.ExpectedVersion)
	if err != nil {
		return operations.Operation{}, err
	}
	var o operations.Operation
	var storedDigest, storedStaff string
	err = tx.QueryRow(ctx, `SELECT id::text,action,target_type,target_ref,expected_version,source_version,status,market,organization,created_at,updated_at,request_digest,staff_ref FROM admin_delegated_operations WHERE principal=$1 AND market=$2 AND organization=$3 AND idempotency_key=$4`, config.HQPrincipal, c.Scope.Market, c.Scope.Organization, c.IdempotencyKey).Scan(&o.ID, &o.Action, &o.Target.Type, &o.Target.ID, &o.Target.ExpectedVersion, &o.SourceVersion, &o.Status, &o.Scope.Market, &o.Scope.Organization, &o.CreatedAt, &o.AsOf, &storedDigest, &storedStaff)
	if errors.Is(err, pgx.ErrNoRows) {
		return o, operations.ErrConflict
	}
	if err != nil {
		return o, err
	}
	if o.ID != c.CommandID || storedDigest != digest || storedStaff != staff {
		return operations.Operation{}, operations.ErrConflict
	}
	if err = tx.Commit(ctx); err != nil {
		return operations.Operation{}, err
	}
	return o, nil
}
func (s *Store) Operation(ctx context.Context, id, staff string, scope security.Scope) (operations.Operation, error) {
	var o operations.Operation
	err := s.Pool.QueryRow(ctx, `SELECT id::text,action,target_type,target_ref,expected_version,source_version,status,market,organization,created_at,updated_at FROM admin_delegated_operations WHERE id=$1 AND staff_ref=$2 AND market=$3 AND organization=$4 AND principal=$5`, id, staff, scope.Market, scope.Organization, config.HQPrincipal).Scan(&o.ID, &o.Action, &o.Target.Type, &o.Target.ID, &o.Target.ExpectedVersion, &o.SourceVersion, &o.Status, &o.Scope.Market, &o.Scope.Organization, &o.CreatedAt, &o.AsOf)
	if errors.Is(err, pgx.ErrNoRows) {
		return o, operations.ErrNotFound
	}
	return o, err
}
