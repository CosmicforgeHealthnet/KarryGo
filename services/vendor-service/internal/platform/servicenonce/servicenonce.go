// Package servicenonce is the durable replay guard for internal HMAC-v2
// ingress.
//
// serviceauth's signature binds the caller, method, request URI, timestamp,
// body digest and a nonce, and the verifier rejects a stale timestamp. Inside
// that timestamp window, though, a captured request is still a valid request:
// what stops it being replayed is consuming the nonce exactly once, durably.
// That is what this package does, against the vendor_service_nonces table
// Section 7 introduced.
//
// Sections 7 through 12 each inlined this same INSERT in their own internal
// handler. Section 13 mounts two more internal surfaces (platform taxonomy
// administration and internal search), and a fifth and sixth copy of a
// security check is a liability rather than a convention, so it is factored
// here. The existing copies are left alone deliberately: they are certified,
// and rewriting them would put four passing sections at risk for a tidiness
// gain.
package servicenonce

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrReplayed reports a nonce this caller has already spent.
var ErrReplayed = errors.New("servicenonce: nonce already used")

// DefaultRetention is how long a spent nonce is remembered. It must comfortably
// exceed the verifier's clock-skew window, or a nonce could be forgotten while
// a captured request bearing it is still inside its valid timestamp range.
const DefaultRetention = 10 * time.Minute

// Store consumes nonces against PostgreSQL.
type Store struct {
	pool      *pgxpool.Pool
	retention time.Duration
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, retention: DefaultRetention}
}

// Consume records (caller, nonce) exactly once.
//
// The uniqueness is the database's, not the process's: ON CONFLICT DO NOTHING
// means two concurrent replays of the same captured request race in PostgreSQL
// and exactly one inserts a row. The loser sees zero rows affected and is
// rejected, which is what makes this safe across several service instances
// rather than only within one.
func (s *Store) Consume(ctx context.Context, caller string, nonce string) error {
	if s == nil || s.pool == nil {
		return ErrReplayed
	}
	caller = strings.TrimSpace(caller)
	nonce = strings.TrimSpace(nonce)
	if caller == "" || nonce == "" {
		return ErrReplayed
	}

	retention := s.retention
	if retention <= 0 {
		retention = DefaultRetention
	}

	tag, err := s.pool.Exec(
		ctx,
		`INSERT INTO vendor_service_nonces (caller, nonce, expires_at)
		 VALUES ($1, $2, now() + $3::interval)
		 ON CONFLICT DO NOTHING`,
		caller,
		nonce,
		retention.String(),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrReplayed
	}
	return nil
}
