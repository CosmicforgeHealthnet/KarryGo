package sharerepositories

import (
	"context"
	m "cosmicforge/logistics/services/customer-service/internal/features/journey_sharing/models"
	"cosmicforge/logistics/shared/go/apperrors"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Command struct {
	Owner, KeyHash, Fingerprint, Operation string
	Session                                m.Session
	ExpectedVersion                        int64
}
type Repository interface {
	Replay(context.Context, Command) (m.Session, bool, error)
	Execute(context.Context, Command) (m.Session, bool, error)
	Get(context.Context, string, string) (m.Session, error)
	ByToken(context.Context, string) (m.Session, error)
	List(context.Context, string, string, string, string, int, int) ([]m.Session, error)
	ObserveCompletion(context.Context, string, int64, time.Time, time.Time) (m.Session, error)
	Expire(context.Context, time.Time, int) (int64, error)
	Candidates(context.Context, time.Time, int) ([]m.Session, error)
	Checked(context.Context, string, time.Time) error
	Touch(context.Context, string, time.Time) error
}

func (r *Postgres) Candidates(ctx context.Context, now time.Time, batch int) ([]m.Session, error) {
	rows, err := r.DB.Query(ctx, `SELECT `+columns+` FROM journey_share_sessions WHERE status='ACTIVE' AND expires_at>$1 AND (source_checked_at IS NULL OR source_checked_at<$2) ORDER BY source_checked_at NULLS FIRST,created_at LIMIT $3`, now, now.Add(-time.Minute), batch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []m.Session{}
	for rows.Next() {
		s, e := scan(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
func (r *Postgres) Checked(ctx context.Context, id string, now time.Time) error {
	_, err := r.DB.Exec(ctx, `UPDATE journey_share_sessions SET source_checked_at=$2 WHERE id=$1`, id, now)
	return err
}

type Postgres struct{ DB *pgxpool.Pool }

func (r *Postgres) Replay(ctx context.Context, c Command) (m.Session, bool, error) {
	var fingerprint string
	var raw []byte
	err := r.DB.QueryRow(ctx, `SELECT fingerprint,result FROM journey_share_commands WHERE customer_id=$1 AND key_hash=$2`, c.Owner, c.KeyHash).Scan(&fingerprint, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return m.Session{}, false, nil
	}
	if err != nil {
		return m.Session{}, false, err
	}
	if fingerprint != c.Fingerprint {
		return m.Session{}, false, apperrors.Conflict("Idempotency key was used for another command.", nil)
	}
	var s m.Session
	err = json.Unmarshal(raw, &s)
	s.CustomerID = c.Owner
	return s, true, err
}

const columns = `id::text,customer_id::text,authority,source_ref,product_code,token_hash,permissions,status,expires_at,source_completed_at,version,created_at,updated_at,last_access_at`

func scan(row pgx.Row) (m.Session, error) {
	var s m.Session
	var permissions []byte
	err := row.Scan(&s.ID, &s.CustomerID, &s.Authority, &s.SourceRef, &s.ProductCode, &s.TokenHash, &permissions, &s.Status, &s.ExpiresAt, &s.SourceCompletedAt, &s.Version, &s.CreatedAt, &s.UpdatedAt, &s.LastAccessAt)
	if err == nil {
		err = json.Unmarshal(permissions, &s.Permissions)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		err = apperrors.NotFound("Share was not found.", nil)
	}
	return s, err
}
func (r *Postgres) Get(ctx context.Context, id, owner string) (m.Session, error) {
	if _, err := uuid.Parse(id); err != nil {
		return m.Session{}, apperrors.NotFound("Share was not found.", nil)
	}
	return scan(r.DB.QueryRow(ctx, `SELECT `+columns+` FROM journey_share_sessions WHERE id=$1 AND customer_id=$2`, id, owner))
}
func (r *Postgres) Touch(ctx context.Context, id string, at time.Time) error {
	_, err := r.DB.Exec(ctx, `UPDATE journey_share_sessions SET last_access_at=$2 WHERE id=$1 AND (last_access_at IS NULL OR last_access_at<$2-interval '1 minute')`, id, at)
	return err
}
func (r *Postgres) ByToken(ctx context.Context, hash string) (m.Session, error) {
	return scan(r.DB.QueryRow(ctx, `SELECT `+columns+` FROM journey_share_sessions WHERE token_hash=$1`, hash))
}
func (r *Postgres) List(ctx context.Context, owner, authority, ref, status string, limit, offset int) ([]m.Session, error) {
	rows, err := r.DB.Query(ctx, `SELECT `+columns+` FROM journey_share_sessions WHERE customer_id=$1 AND ($2='' OR authority=$2) AND ($3='' OR source_ref=$3) AND ($4='' OR status=$4) ORDER BY created_at DESC,id LIMIT $5 OFFSET $6`, owner, authority, ref, status, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []m.Session{}
	for rows.Next() {
		s, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
func (r *Postgres) Execute(ctx context.Context, c Command) (m.Session, bool, error) {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return m.Session{}, false, err
	}
	defer tx.Rollback(ctx)
	// Serialize all commands for one customer, including active source uniqueness.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, c.Owner); err != nil {
		return m.Session{}, false, err
	}
	var fingerprint string
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT fingerprint,result FROM journey_share_commands WHERE customer_id=$1 AND key_hash=$2`, c.Owner, c.KeyHash).Scan(&fingerprint, &raw)
	if err == nil {
		if fingerprint != c.Fingerprint {
			return m.Session{}, false, apperrors.Conflict("Idempotency key was used for another command.", nil)
		}
		var s m.Session
		if err = json.Unmarshal(raw, &s); err != nil {
			return s, false, err
		}
		s.CustomerID = c.Owner
		return s, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return m.Session{}, false, err
	}
	s := c.Session
	if c.Operation == "create" {
		_, err = tx.Exec(ctx, `UPDATE journey_share_sessions SET status='EXPIRED',version=version+1,updated_at=$4 WHERE customer_id=$1 AND authority=$2 AND source_ref=$3 AND status='ACTIVE' AND expires_at<=$4`, c.Owner, s.Authority, s.SourceRef, s.CreatedAt)
		if err != nil {
			return s, false, err
		}
		p, _ := json.Marshal(s.Permissions)
		_, err = tx.Exec(ctx, `INSERT INTO journey_share_sessions(id,customer_id,authority,source_ref,product_code,token_hash,permissions,status,expires_at,version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,'ACTIVE',$8,1,$9,$9)`, s.ID, c.Owner, s.Authority, s.SourceRef, s.ProductCode, s.TokenHash, p, s.ExpiresAt, s.CreatedAt)
	} else {
		current, e := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM journey_share_sessions WHERE id=$1 AND customer_id=$2 FOR UPDATE`, s.ID, c.Owner))
		if e != nil {
			return s, false, e
		}
		if current.Version != c.ExpectedVersion {
			return current, false, apperrors.Conflict("Share version is stale.", nil)
		}
		if c.Operation == "rotate" && (!current.Readable(s.UpdatedAt) || current.Status != "ACTIVE") {
			return current, false, apperrors.Conflict("Share cannot be rotated.", nil)
		}
		current.Version++
		current.UpdatedAt = s.UpdatedAt
		if c.Operation == "revoke" {
			current.Status = "REVOKED"
		} else {
			current.TokenHash = s.TokenHash
		}
		s = current
		_, err = tx.Exec(ctx, `UPDATE journey_share_sessions SET status=$2,token_hash=$3,version=$4,updated_at=$5 WHERE id=$1`, s.ID, s.Status, s.TokenHash, s.Version, s.UpdatedAt)
	}
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			err = apperrors.Conflict("An active share already exists for this journey.", nil)
		}
		return s, false, err
	}
	raw, _ = json.Marshal(s) // Session serialization excludes customer and token digest.
	_, err = tx.Exec(ctx, `INSERT INTO journey_share_commands(customer_id,key_hash,fingerprint,session_id,operation,result,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, c.Owner, c.KeyHash, c.Fingerprint, s.ID, c.Operation, raw, s.UpdatedAt)
	if err != nil {
		return s, false, err
	}
	if c.Operation == "create" || c.Operation == "revoke" {
		event := "share.created"
		if c.Operation == "revoke" {
			event = "share.revoked"
		}
		if _, err = tx.Exec(ctx, `INSERT INTO journey_share_notification_outbox(id,session_id,customer_id,event_type,created_at,available_at) VALUES($1,$2,$3,$4,$5,$5)`, uuid.NewString(), s.ID, c.Owner, event, s.UpdatedAt); err != nil {
			return s, false, err
		}
	}
	err = tx.Commit(ctx)
	return s, false, err
}
func (r *Postgres) ObserveCompletion(ctx context.Context, id string, version int64, completed, expiry time.Time) (m.Session, error) {
	_, err := r.DB.Exec(ctx, `UPDATE journey_share_sessions SET status='COMPLETED',source_completed_at=$3,expires_at=LEAST(expires_at,$4),version=version+1,updated_at=now() WHERE id=$1 AND version=$2 AND status='ACTIVE'`, id, version, completed, expiry)
	if err != nil {
		return m.Session{}, err
	}
	return scan(r.DB.QueryRow(ctx, `SELECT `+columns+` FROM journey_share_sessions WHERE id=$1`, id))
}
func (r *Postgres) Expire(ctx context.Context, now time.Time, batch int) (int64, error) {
	var count int64
	err := r.DB.QueryRow(ctx, `WITH expired AS (
	 UPDATE journey_share_sessions SET status='EXPIRED',version=version+1,updated_at=$1
	 WHERE id IN (SELECT id FROM journey_share_sessions WHERE status IN ('ACTIVE','COMPLETED') AND expires_at<=$1 ORDER BY expires_at LIMIT $2 FOR UPDATE SKIP LOCKED)
	 RETURNING id,customer_id
	), intents AS (
	 INSERT INTO journey_share_notification_outbox(id,session_id,customer_id,event_type,created_at,available_at)
	 SELECT gen_random_uuid(),id,customer_id,'share.expired',$1,$1 FROM expired RETURNING id
	) SELECT count(*) FROM intents`, now, batch).Scan(&count)
	return count, err
}
