// Package repositories reads the append-only audit trail written by
// internal/platform/audit.Recorder.
//
// This package never writes fleet_partner_audit_records — the table's own
// BEFORE UPDATE OR DELETE trigger (fleet_partner_forbid_mutation, migration
// 000001) already refuses mutation at the database layer, and duplicating an
// "insert" path here would just be a second, weaker audit writer. Reads are
// tenant-scoped in SQL, not filtered after the rows are loaded: a filter
// applied after the fact is one refactor away from being forgotten.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
)

// ErrNotFound means the audit record does not exist within the caller's
// organization scope.
var ErrNotFound = errors.New("audit: record not found")

// Record is one append-only audit entry, exactly as stored.
type Record struct {
	ID              string
	OrganizationID  string
	BranchID        string
	ActorType       string
	ActorID         string
	Action          string
	ResourceType    string
	ResourceID      string
	PreviousSummary map[string]any
	NewSummary      map[string]any
	Reason          string
	RequestID       string
	CorrelationID   string
	NetworkMeta     map[string]string
	OccurredAt      time.Time
}

// highRiskActionPattern matches audit actions the spec calls out as
// inherently sensitive (session/credential/role/legal-hold/retention/replay
// operations), independent of which domain feature produced them. Kept as a
// single allowlist here rather than a new column on every domain's audit
// call site, so classification stays centralized and additive.
var highRiskActionSubstrings = []string{
	"suspend", "revoke", "legal_hold", "credential", "role_change",
	"permission_change", "retention_execute", "security_setting",
	"dead_letter_replay", "session.revoke", "organization.close",
	"membership.remove",
}

// IsHighRisk reports whether an audit action matches the high-risk allowlist.
func IsHighRisk(action string) bool {
	lower := strings.ToLower(action)
	for _, needle := range highRiskActionSubstrings {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

// ListFilter narrows a tenant-scoped audit listing. Every non-empty field is
// ANDed together. OrganizationID and PermittedBranchIDs come only from the
// resolved tenant context, never from caller input.
type ListFilter struct {
	OrganizationID     string
	PermittedBranchIDs []string
	BranchID           string
	ActorType          string
	ActorID            string
	Action             string
	ResourceType       string
	ResourceID         string
	RequestID          string
	CorrelationID      string
	HighRiskOnly       bool
	From               time.Time
	To                 time.Time
	// BeforeOccurredAt/BeforeID form a keyset cursor: rows strictly before
	// this (occurred_at, id) pair in the DESC ordering. Both empty means the
	// first page.
	BeforeOccurredAt time.Time
	BeforeID         string
	Limit            int
}

// MaxDateRange is the widest From/To span a caller may request. A bounded
// range keeps a single audit query from scanning an organization's entire
// history.
const MaxDateRange = 90 * 24 * time.Hour

// DefaultLimit and MaxLimit bound page size.
const (
	DefaultLimit = 50
	MaxLimit     = 200
)

// Repository reads fleet_partner_audit_records.
type Repository struct {
	db database.Querier
}

// NewRepository wires the repository over a pool.
func NewRepository(db database.Querier) *Repository { return &Repository{db: db} }

// VerifyChain recomputes and checks the tamper-evident hash chain (see
// internal/platform/audit's chain.go) for the given pre-built partition key
// -- build one with audit.PartitionKey(organizationID, month) so this
// package never has to know the "<orgId>:<month>" format itself.
func (r *Repository) VerifyChain(ctx context.Context, partition string) (audit.VerifyChainResult, error) {
	return audit.VerifyChain(ctx, r.db, partition)
}

// List returns records matching filter, newest first, plus a cursor for the
// next page (empty when exhausted).
func (r *Repository) List(ctx context.Context, filter ListFilter) ([]Record, string, error) {
	if filter.OrganizationID == "" {
		return nil, "", fmt.Errorf("audit: organization id is required")
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	var (
		conditions []string
		args       []any
	)
	args = append(args, filter.OrganizationID)
	conditions = append(conditions, "organization_id = $1")

	args = append(args, filter.PermittedBranchIDs)
	conditions = append(conditions,
		fmt.Sprintf("(COALESCE(cardinality($%d::text[]),0)=0 OR branch_id = ANY($%d::text[]) OR branch_id IS NULL)",
			len(args), len(args)))

	if filter.BranchID != "" {
		args = append(args, filter.BranchID)
		conditions = append(conditions, fmt.Sprintf("branch_id = $%d", len(args)))
	}
	if filter.ActorType != "" {
		args = append(args, filter.ActorType)
		conditions = append(conditions, fmt.Sprintf("actor_type = $%d", len(args)))
	}
	if filter.ActorID != "" {
		args = append(args, filter.ActorID)
		conditions = append(conditions, fmt.Sprintf("actor_id = $%d", len(args)))
	}
	if filter.Action != "" {
		args = append(args, filter.Action)
		conditions = append(conditions, fmt.Sprintf("action = $%d", len(args)))
	}
	if filter.ResourceType != "" {
		args = append(args, filter.ResourceType)
		conditions = append(conditions, fmt.Sprintf("resource_type = $%d", len(args)))
	}
	if filter.ResourceID != "" {
		args = append(args, filter.ResourceID)
		conditions = append(conditions, fmt.Sprintf("resource_id = $%d", len(args)))
	}
	if filter.RequestID != "" {
		args = append(args, filter.RequestID)
		conditions = append(conditions, fmt.Sprintf("request_id = $%d", len(args)))
	}
	if filter.CorrelationID != "" {
		args = append(args, filter.CorrelationID)
		conditions = append(conditions, fmt.Sprintf("correlation_id = $%d", len(args)))
	}
	if !filter.From.IsZero() {
		args = append(args, filter.From.UTC())
		conditions = append(conditions, fmt.Sprintf("occurred_at >= $%d", len(args)))
	}
	if !filter.To.IsZero() {
		args = append(args, filter.To.UTC())
		conditions = append(conditions, fmt.Sprintf("occurred_at <= $%d", len(args)))
	}
	if !filter.BeforeOccurredAt.IsZero() && filter.BeforeID != "" {
		args = append(args, filter.BeforeOccurredAt.UTC(), filter.BeforeID)
		conditions = append(conditions, fmt.Sprintf("(occurred_at, id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	if filter.HighRiskOnly {
		conditions = append(conditions, "("+highRiskSQLPredicate()+")")
	}

	args = append(args, limit+1)
	query := fmt.Sprintf(`
		SELECT id, organization_id, COALESCE(branch_id, ''), actor_type, actor_id, action,
		       resource_type, resource_id, previous_summary, new_summary, COALESCE(reason, ''),
		       request_id, correlation_id, network_meta, occurred_at
		FROM fleet_partner_audit_records
		WHERE %s
		ORDER BY occurred_at DESC, id DESC
		LIMIT $%d`, strings.Join(conditions, " AND "), len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("audit: list: %w", err)
	}
	defer rows.Close()

	var records []Record
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return nil, "", fmt.Errorf("audit: scan: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("audit: rows: %w", err)
	}

	var nextCursor string
	if len(records) > limit {
		records = records[:limit]
		last := records[len(records)-1]
		nextCursor = fmt.Sprintf("%s|%s", last.OccurredAt.UTC().Format(time.RFC3339Nano), last.ID)
	}
	return records, nextCursor, nil
}

// highRiskSQLPredicate mirrors IsHighRisk as a SQL ILIKE OR-chain, so the
// high-risk filter narrows in the database rather than in Go after paging.
func highRiskSQLPredicate() string {
	parts := make([]string, 0, len(highRiskActionSubstrings))
	for _, needle := range highRiskActionSubstrings {
		parts = append(parts, fmt.Sprintf("action ILIKE '%%%s%%'", strings.ReplaceAll(needle, "'", "''")))
	}
	return strings.Join(parts, " OR ")
}

// Get returns one audit record scoped to the caller's organization. A record
// belonging to a different organization answers identically to a record that
// does not exist.
func (r *Repository) Get(ctx context.Context, organizationID, id string) (Record, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, organization_id, COALESCE(branch_id, ''), actor_type, actor_id, action,
		       resource_type, resource_id, previous_summary, new_summary, COALESCE(reason, ''),
		       request_id, correlation_id, network_meta, occurred_at
		FROM fleet_partner_audit_records
		WHERE organization_id = $1 AND id = $2`, organizationID, id)
	record, err := scanRecord(row)
	if err != nil {
		return Record{}, ErrNotFound
	}
	return record, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanRecord(row scannable) (Record, error) {
	var (
		record          Record
		previousSummary []byte
		newSummary      []byte
		networkMeta     []byte
	)
	if err := row.Scan(
		&record.ID, &record.OrganizationID, &record.BranchID, &record.ActorType, &record.ActorID,
		&record.Action, &record.ResourceType, &record.ResourceID, &previousSummary, &newSummary,
		&record.Reason, &record.RequestID, &record.CorrelationID, &networkMeta, &record.OccurredAt,
	); err != nil {
		return Record{}, err
	}
	if len(previousSummary) > 0 {
		_ = json.Unmarshal(previousSummary, &record.PreviousSummary)
	}
	if len(newSummary) > 0 {
		_ = json.Unmarshal(newSummary, &record.NewSummary)
	}
	if len(networkMeta) > 0 {
		_ = json.Unmarshal(networkMeta, &record.NetworkMeta)
	}
	return record, nil
}
