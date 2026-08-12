// Package audit implements the append-only, tamper-evident audit
// foundation.
//
// Sensitive mutations record who did what to which resource, with redacted
// before/after summaries, in the same transaction as the domain mutation.
// Audit rows are never updated or deleted by runtime code — enforced twice
// over: fleet_partner_forbid_mutation (migration 000001, a real database
// trigger) and, additively, a hash chain (migration 000069) that can
// DETECT a modification that somehow bypassed the trigger (a superuser
// UPDATE, a restored backup missing rows). See chain.go for the chaining
// mechanism itself.
//
// CausationID/ApprovalReference/IdempotencyKeyReference/StepUpReference are
// new, optional fields: only internal/features/legalhold and
// internal/features/securitysettings populate StepUpReference today (the
// two features with a step-up reference actually in scope). Every other
// existing caller of Record leaves them empty, which is valid (the columns
// are nullable) — populating them everywhere a domain feature has the
// relevant reference is real, disclosed follow-up work, not done in this
// pass.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/redact"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Entry is one append-only audit record.
type Entry struct {
	OrganizationID string
	BranchID       string
	ActorType      string
	ActorID        string
	Action         string
	ResourceType   string
	ResourceID     string
	PreviousValues map[string]any
	NewValues      map[string]any
	Reason         string
	RequestID      string
	CorrelationID  string
	NetworkMeta    map[string]string

	// CausationID identifies the event/command that CAUSED this action, as
	// distinct from CorrelationID (which threads an entire request/workflow
	// together). Optional.
	CausationID string
	// ApprovalReference, IdempotencyKeyReference and StepUpReference cite
	// the specific approval/idempotency/step-up record a high-risk action
	// relied on, so a chain-verification pass or an auditor can trace
	// exactly which grant authorized the action. Optional.
	ApprovalReference       string
	IdempotencyKeyReference string
	StepUpReference         string
}

// Redact returns a copy of values with sensitive keys masked and everything
// serialized shallowly; it never returns secrets, identity documents or raw
// financial account details. Sensitive-key detection is centralized in
// internal/platform/redact so this list cannot drift from the one
// internal/observability's log line redaction uses.
func Redact(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	redacted := make(map[string]any, len(values))
	for key, value := range values {
		if redact.IsSensitiveKey(key) {
			redacted[key] = "[REDACTED]"
			continue
		}
		redacted[key] = value
	}
	return redacted
}

// Recorder appends audit entries.
type Recorder struct{}

// NewRecorder constructs the append-only recorder.
func NewRecorder() *Recorder {
	return &Recorder{}
}

// Record appends the entry using q — pass the transaction of the domain
// mutation for atomicity when one exists. Summaries are redacted before
// storage, and a tamper-evident hash chain entry is computed and stored in
// the same statement — see chain.go.
//
// The chain's concurrent-append safety depends on pg_advisory_xact_lock
// (chain.go), which is TRANSACTION-scoped: it only actually serializes
// concurrent callers when the lock acquisition, the chain-tip read and the
// insert all run on the SAME transaction/connection. A caller with no
// natural domain-mutation transaction to join (e.g. an audit event for a
// read-authorization action, not a write) would otherwise pass q as a bare
// *pgxpool.Pool, where each statement can be handed a different pooled
// connection and the "lock" would not serialize anything. Record closes
// that gap itself rather than relying on every caller to remember to wrap a
// standalone call in database.WithinTx: when q is a bare pool, Record opens
// its own transaction internally and runs the whole sequence inside it.
func (r *Recorder) Record(ctx context.Context, q database.Querier, entry Entry) error {
	if pool, ok := q.(*pgxpool.Pool); ok {
		return database.WithinTx(ctx, pool, func(tx pgx.Tx) error {
			return r.record(ctx, tx, entry)
		})
	}
	return r.record(ctx, q, entry)
}

// record performs the actual validation, hashing and insert against q,
// assuming q already provides the transactional semantics the chain lock
// requires (either a real pgx.Tx passed in by the caller, or one Record
// itself opened in the bare-pool branch above).
func (r *Recorder) record(ctx context.Context, q database.Querier, entry Entry) error {
	if entry.OrganizationID == "" {
		return fmt.Errorf("audit: organization id is required")
	}
	if entry.Action == "" || entry.ResourceType == "" || entry.ResourceID == "" {
		return fmt.Errorf("audit: action and resource identity are required")
	}
	if entry.ActorType == "" || entry.ActorID == "" {
		return fmt.Errorf("audit: actor is required")
	}

	previousValues := Redact(entry.PreviousValues)
	newValues := Redact(entry.NewValues)
	previous, err := json.Marshal(previousValues)
	if err != nil {
		return fmt.Errorf("audit: marshal previous values: %w", err)
	}
	next, err := json.Marshal(newValues)
	if err != nil {
		return fmt.Errorf("audit: marshal new values: %w", err)
	}
	network, err := json.Marshal(entry.NetworkMeta)
	if err != nil {
		return fmt.Errorf("audit: marshal network metadata: %w", err)
	}

	id := ids.New(ids.PrefixAudit)

	// Serialize concurrent appends to the SAME partition (see chain.go) so
	// two transactions computing "the current tip" at once cannot both
	// build on the same previous hash. A different partition (different
	// organization, or the same organization a month apart) never blocks
	// this one. The provisional partition uses "now" only to pick WHICH
	// lock to acquire; the timestamp actually stamped on this row and fed
	// into the chain hash is taken AFTER the lock is held (below), so that
	// under concurrency the occurred_at ordering VerifyChain relies on
	// always matches the true serialization order the lock enforced --
	// computing it before the lock would let a goroutine that "won" the
	// lock race still be timestamped earlier than one that lost it,
	// desyncing occurred_at order from actual chain order and making every
	// later entry's previous_entry_hash link look broken even though
	// nothing was tampered with.
	provisionalPartition := chainPartition(entry.OrganizationID, time.Now().UTC())
	if _, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, provisionalPartition); err != nil {
		return fmt.Errorf("audit: acquire chain partition lock: %w", err)
	}
	// Truncated to microsecond precision BEFORE it is hashed: Postgres
	// timestamptz only stores microsecond precision, so a nanosecond-
	// precision Go time.Time hashed as-is would never match what
	// VerifyChain reads back and recomputes from later -- the same class
	// of "storage round-trip changes the value" issue canonicalizeJSON
	// exists to close for previous_summary/new_summary, just for time
	// instead of JSON.
	occurredAt := time.Now().UTC().Truncate(time.Microsecond)
	partition := chainPartition(entry.OrganizationID, occurredAt)
	previousHash, err := latestEntryHash(ctx, q, partition)
	if err != nil {
		return fmt.Errorf("audit: read chain tip: %w", err)
	}
	chained := chainedFields{
		id: id, organizationID: entry.OrganizationID, actorType: entry.ActorType, actorID: entry.ActorID,
		action: entry.Action, resourceType: entry.ResourceType, resourceID: entry.ResourceID,
		previousSummary: previous, newSummary: next, requestID: entry.RequestID, correlationID: entry.CorrelationID,
		causationID: entry.CausationID, approvalReference: entry.ApprovalReference,
		idempotencyKeyReference: entry.IdempotencyKeyReference, stepUpReference: entry.StepUpReference,
		occurredAt: occurredAt,
	}
	entryHash := computeEntryHash(chainKey(), partition, previousHash, chained)

	_, err = q.Exec(ctx, `
		INSERT INTO fleet_partner_audit_records
			(id, organization_id, branch_id, actor_type, actor_id, action,
			 resource_type, resource_id, previous_summary, new_summary, reason,
			 request_id, correlation_id, network_meta, occurred_at,
			 causation_id, approval_reference, idempotency_key_reference, step_up_reference,
			 chain_partition, previous_entry_hash, entry_hash, hash_version)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, $8, $9, $10, NULLIF($11, ''), $12, $13, $14, $15,
			NULLIF($16, ''), NULLIF($17, ''), NULLIF($18, ''), NULLIF($19, ''),
			$20, $21, $22, $23)`,
		id, entry.OrganizationID, entry.BranchID,
		entry.ActorType, entry.ActorID, entry.Action,
		entry.ResourceType, entry.ResourceID, previous, next, entry.Reason,
		entry.RequestID, entry.CorrelationID, network, occurredAt,
		entry.CausationID, entry.ApprovalReference, entry.IdempotencyKeyReference, entry.StepUpReference,
		partition, nullableString(previousHash), entryHash, currentHashVersion,
	)
	if err != nil {
		return fmt.Errorf("audit: append record: %w", err)
	}
	return nil
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// EntryFromTenant prefills actor and tracing fields from a tenant context.
func EntryFromTenant(t tenant.Context, action string, resourceType string, resourceID string) Entry {
	return Entry{
		OrganizationID: t.OrganizationID,
		ActorType:      string(t.ActorType),
		ActorID:        t.ActorID,
		Action:         action,
		ResourceType:   resourceType,
		ResourceID:     resourceID,
		RequestID:      t.RequestID,
		CorrelationID:  t.CorrelationID,
	}
}
