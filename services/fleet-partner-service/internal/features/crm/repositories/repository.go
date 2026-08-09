// Package repositories implements PostgreSQL persistence for Section 14.
//
// Every read and every write carries organization_id in SQL. A handler check is
// a convenience; the WHERE clause is the control, because a query that forgot
// one is a cross-tenant read that no amount of handler discipline prevents.
//
// Nested resources carry the parent ID as well as their own, so naming another
// customer's contact yields the same not-found a nonexistent contact does. The
// caller is never told that the row exists but belongs to somebody else.
package repositories

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
)

// Errors returned by the CRM repositories.
var (
	// ErrNotFound covers both "no such row" and "not yours". They are the same
	// answer on purpose.
	ErrNotFound = errors.New("crm: record not found")
	// ErrVersionConflict is an optimistic-concurrency failure.
	ErrVersionConflict = errors.New("crm: expected version does not match")
	// ErrDuplicate is a uniqueness violation inside the organization.
	ErrDuplicate = errors.New("crm: conflicting record")
	// ErrFingerprint is an idempotency key reused with different inputs.
	ErrFingerprint = errors.New("crm: idempotency key reused with different request")
	// ErrImmutable is an attempted change to an append-only record.
	ErrImmutable = errors.New("crm: record is immutable")
)

// ID prefixes for Section 14 records.
const (
	PrefixCustomer          = "cst"
	PrefixContact           = "cnt"
	PrefixLocation          = "cln"
	PrefixOpportunity       = "opp"
	PrefixQuotation         = "qot"
	PrefixQuotationVersion  = "qvr"
	PrefixLineItem          = "qli"
	PrefixAcceptance        = "qac"
	PrefixConversionIntent  = "cvi"
	PrefixQuotationToken    = "qtk"
	PrefixQuotationDocument = "qdc"
	PrefixContract          = "ctr"
	PrefixContractVersion   = "cvr"
	PrefixContractDocument  = "cdc"
	PrefixSLA               = "sla"
	PrefixObservation       = "slo"
	PrefixCreditProfile     = "crp"
	PrefixHistory           = "crh"
	PrefixNotification      = "cno"
)

// Repository persists every Section 14 aggregate.
//
// It is one type rather than nine because the aggregates share a transaction
// boundary constantly: issuing a quotation writes the quotation, its immutable
// version, its history, an audit row, an outbox row and a notification
// operation, and splitting that across repositories would only move the
// coupling somewhere less visible.
type Repository struct{ db database.Querier }

// NewRepository builds the repository.
func NewRepository(db database.Querier) *Repository { return &Repository{db: db} }

func (r *Repository) q(q database.Querier) database.Querier {
	if q == nil {
		return r.db
	}
	return q
}

func itoa(value int) string { return fmt.Sprintf("%d", value) }

// branchClause renders the branch restriction and returns the extended args.
//
// An empty branch list means organization-wide authority. A restricted caller
// sees rows in their own branches plus rows with no branch, which are
// organization-level records nobody's branch owns.
func branchClause(branchIDs []string, column string, args []any) (string, []any) {
	if len(branchIDs) == 0 {
		return "TRUE", args
	}
	args = append(args, branchIDs)
	return fmt.Sprintf("(%s IS NULL OR %s = ANY($%d))", column, column, len(args)), args
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "SQLSTATE 23505")
}

// isImmutableViolation detects the append-only trigger refusing a write. The
// trigger raises a plain exception, so this is matched on its message rather
// than a SQLSTATE, and the message never reaches the caller.
func isImmutableViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "immutable")
}

// classify turns a driver error into a domain error so no SQLSTATE, table name
// or constraint name ever escapes the repository.
func classify(err error, operation string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	case isUniqueViolation(err):
		return ErrDuplicate
	case isImmutableViolation(err):
		return ErrImmutable
	default:
		return fmt.Errorf("crm: %s: %w", operation, err)
	}
}

// ---------------------------------------------------------------------------
// jsonb and array helpers
// ---------------------------------------------------------------------------

func encodeJSON(value any) ([]byte, error) {
	if value == nil {
		return []byte("{}"), nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("crm: encode json: %w", err)
	}
	return encoded, nil
}

func encodeJSONArray(value any) ([]byte, error) {
	if value == nil {
		return []byte("[]"), nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("crm: encode json array: %w", err)
	}
	return encoded, nil
}

func decodeJSONMap(raw []byte) models.JSONMap {
	if len(raw) == 0 {
		return models.JSONMap{}
	}
	var out models.JSONMap
	if err := json.Unmarshal(raw, &out); err != nil {
		// A jsonb column that will not decode is corrupt data, not a caller
		// error. Returning an empty map keeps the read honest without
		// pretending the stored bytes said something they did not.
		return models.JSONMap{}
	}
	if out == nil {
		return models.JSONMap{}
	}
	return out
}

func decodeStringSlice(raw []byte) []string {
	if len(raw) == 0 {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

func nullable(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func textOf(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// pageWhere appends the cursor predicate for a (created_at DESC, id DESC) page.
func pageWhere(where string, args []any, c cursor.Cursor, hasCursor bool,
	sortColumn string) (string, []any) {
	if !hasCursor {
		return where, args
	}
	args = append(args, c.SortTime(), c.LastID)
	return where + fmt.Sprintf(" AND (%s, id) < ($%d, $%d)", sortColumn, len(args)-1, len(args)), args
}

// ---------------------------------------------------------------------------
// Transaction helper
// ---------------------------------------------------------------------------

// Querier exposes the underlying querier for callers that already hold a
// transaction and want to reuse this repository inside it.
func (r *Repository) Querier() database.Querier { return r.db }

// scanErr maps pgx's no-rows into the domain not-found.
func scanErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
