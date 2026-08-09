package serviceauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
)

// ErrNotFound means the credential does not exist.
var ErrNotFound = errors.New("serviceauth: credential not found")

// Store persists fleet_partner_service_credentials. Never reads or writes a
// secret -- there is no secret column (see model.go's doc comment).
type Store struct {
	db database.Querier
}

// NewStore wires the store over a pool or transaction.
func NewStore(db database.Querier) *Store { return &Store{db: db} }

const credentialColumns = `id, service_name, version, allowed_methods, allowed_route_patterns,
	allowed_event_types, active_from, expires_at, revoked_at, COALESCE(revoked_reason, ''), last_used_at,
	created_by, version_note, created_at, updated_at`

// ActiveCandidates returns every non-revoked credential for serviceName,
// newest version first -- the candidate order Resolve tries HMAC signature
// verification against. Includes not-yet-active and already-expired rows
// (the caller checks IsActive itself): excluding them here would hide a
// "signature is valid but the credential's window has not started/has
// ended" distinction the caller needs to report precisely.
func (s *Store) ActiveCandidates(ctx context.Context, serviceName string) ([]Credential, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+credentialColumns+`
		FROM fleet_partner_service_credentials
		WHERE service_name = $1 AND revoked_at IS NULL
		ORDER BY version DESC`, serviceName)
	if err != nil {
		return nil, fmt.Errorf("serviceauth: list active candidates: %w", err)
	}
	defer rows.Close()
	return scanCredentials(rows)
}

// List returns every credential for serviceName, including revoked ones,
// newest version first -- the administration surface's "safe listing"
// (no secret column exists to leak; every row this returns is already
// safe-by-construction).
func (s *Store) List(ctx context.Context, serviceName string) ([]Credential, error) {
	rows, err := s.db.Query(ctx, `
		SELECT `+credentialColumns+`
		FROM fleet_partner_service_credentials
		WHERE service_name = $1
		ORDER BY version DESC`, serviceName)
	if err != nil {
		return nil, fmt.Errorf("serviceauth: list: %w", err)
	}
	defer rows.Close()
	return scanCredentials(rows)
}

// ListAllServices returns every distinct service_name with at least one
// credential -- backs an administration "list callers" view.
func (s *Store) ListAllServices(ctx context.Context) ([]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT service_name FROM fleet_partner_service_credentials ORDER BY service_name`)
	if err != nil {
		return nil, fmt.Errorf("serviceauth: list services: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("serviceauth: scan service name: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// GetByID loads one credential by its opaque ID.
func (s *Store) GetByID(ctx context.Context, id string) (Credential, error) {
	row := s.db.QueryRow(ctx, `SELECT `+credentialColumns+`
		FROM fleet_partner_service_credentials WHERE id = $1`, id)
	return scanCredentialRow(row)
}

// LatestVersion returns the highest version number for serviceName across
// every row (including revoked ones -- a version number is never reused),
// or 0 if none exist yet.
func (s *Store) LatestVersion(ctx context.Context, serviceName string) (int, error) {
	var version int
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(MAX(version), 0) FROM fleet_partner_service_credentials
		WHERE service_name = $1`, serviceName).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("serviceauth: latest version: %w", err)
	}
	return version, nil
}

// Create inserts a new credential row. c.ID must already be set by the
// caller (admin.go's Administrator assigns one via ids.New before calling
// this) -- Create deliberately does NOT generate one itself: c is passed by
// value, so a fallback generated here would silently vanish the moment
// this returns, leaving the caller (for example an audit.Entry
// ResourceID) referencing an empty string. That exact bug existed here
// once and was caught by TestServiceCredentialAdmin_CreateAcceptsRealRegisteredRoute
// failing with "audit: action and resource identity are required" --
// fixed by making the precondition explicit and fatal instead of
// papering over it with a fallback that could not possibly work given
// this method's value-receiver signature.
//
// Store itself does not decide the next version number either, so it can
// be called from within a transaction that has already resolved it under
// a lock.
func (s *Store) Create(ctx context.Context, q database.Querier, c Credential) error {
	if c.ID == "" {
		return fmt.Errorf("serviceauth: credential id is required")
	}
	methods, err := json.Marshal(orEmptyStrings(c.AllowedMethods))
	if err != nil {
		return fmt.Errorf("serviceauth: marshal allowed methods: %w", err)
	}
	routes, err := json.Marshal(orEmptyStrings(c.AllowedRoutePatterns))
	if err != nil {
		return fmt.Errorf("serviceauth: marshal allowed routes: %w", err)
	}
	events, err := json.Marshal(orEmptyStrings(c.AllowedEventTypes))
	if err != nil {
		return fmt.Errorf("serviceauth: marshal allowed events: %w", err)
	}
	activeFrom := c.ActiveFrom
	if activeFrom.IsZero() {
		activeFrom = time.Now().UTC()
	}
	_, err = q.Exec(ctx, `
		INSERT INTO fleet_partner_service_credentials
			(id, service_name, version, allowed_methods, allowed_route_patterns, allowed_event_types,
			 active_from, expires_at, created_by, version_note, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now(), now())`,
		c.ID, c.ServiceName, c.Version, methods, routes, events,
		activeFrom, c.ExpiresAt, c.CreatedBy, c.VersionNote,
	)
	if err != nil {
		return fmt.Errorf("serviceauth: create credential: %w", err)
	}
	return nil
}

// SetExpiry updates ONLY expires_at -- used to bound a superseded
// credential's remaining lifetime during a rotation-overlap window, without
// touching anything else about the row (including its own revoked_at,
// which stays independent: an expired credential is not the same thing as
// a revoked one, see Credential.IsActive).
func (s *Store) SetExpiry(ctx context.Context, q database.Querier, id string, expiresAt time.Time) error {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_service_credentials SET expires_at = $2, updated_at = now()
		WHERE id = $1`, id, expiresAt)
	if err != nil {
		return fmt.Errorf("serviceauth: set expiry: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Revoke sets revoked_at immediately (now), independent of any configured
// expiry -- immediate revocation, not a scheduled one.
func (s *Store) Revoke(ctx context.Context, q database.Querier, id string, reason string) error {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_service_credentials
		SET revoked_at = now(), revoked_reason = $2, updated_at = now()
		WHERE id = $1 AND revoked_at IS NULL`, id, reason)
	if err != nil {
		return fmt.Errorf("serviceauth: revoke: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// TouchLastUsed records that id successfully authorized a request just now.
// Called outside the request's own transaction (a failed last-used write
// must never fail an otherwise-authorized request) -- callers should log,
// not propagate, any error this returns.
func (s *Store) TouchLastUsed(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE fleet_partner_service_credentials SET last_used_at = now() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("serviceauth: touch last used: %w", err)
	}
	return nil
}

func scanCredentials(rows pgx.Rows) ([]Credential, error) {
	var creds []Credential
	for rows.Next() {
		c, err := scanCredentialFields(rows)
		if err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	return creds, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanCredentialRow(row pgx.Row) (Credential, error) {
	c, err := scanCredentialFields(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Credential{}, ErrNotFound
	}
	return c, err
}

func scanCredentialFields(row scannable) (Credential, error) {
	var c Credential
	var methods, routes, events []byte
	err := row.Scan(
		&c.ID, &c.ServiceName, &c.Version, &methods, &routes, &events,
		&c.ActiveFrom, &c.ExpiresAt, &c.RevokedAt, &c.RevokedReason, &c.LastUsedAt,
		&c.CreatedBy, &c.VersionNote, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return Credential{}, fmt.Errorf("serviceauth: scan credential: %w", err)
	}
	_ = json.Unmarshal(methods, &c.AllowedMethods)
	_ = json.Unmarshal(routes, &c.AllowedRoutePatterns)
	_ = json.Unmarshal(events, &c.AllowedEventTypes)
	return c, nil
}

func orEmptyStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
