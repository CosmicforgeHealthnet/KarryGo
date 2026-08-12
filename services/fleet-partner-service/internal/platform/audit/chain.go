package audit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
)

// currentHashVersion identifies the chaining SCHEME (canonical field set and
// serialization), not a per-key rotation counter. Bump this if the fields
// hashed, or how they are serialized, ever changes — a verification pass
// keyed to an OLDER hash_version must recompute using that version's exact
// scheme, not today's.
const currentHashVersion = 1

// chainPartition bounds one hash chain to an organization and a calendar
// month, so verification and the per-append advisory lock never serialize
// across unrelated organizations or grow into one unbounded, forever-
// growing chain. Exported spelling kept lowercase/unexported: callers use
// Record and VerifyChain, never construct a partition key by hand.
func chainPartition(organizationID string, occurredAt time.Time) string {
	return organizationID + ":" + occurredAt.UTC().Format("2006-01")
}

var (
	chainKeyOnce  sync.Once
	chainKeyValue []byte
)

// chainKey returns the HMAC key used to compute entry hashes. Read once
// from FLEET_PARTNER_AUDIT_CHAIN_SECRET; an unconfigured deployment falls
// back to a fixed, clearly-labelled non-secret so chaining still produces a
// SELF-CONSISTENT chain (verification still detects tampering) even though
// that chain is not confidentially keyed. Production readiness
// (internal/app's readiness checks) fails closed when this env var is unset
// and AppEnv=="production" — see docs/section28-security-privacy-audit.md's
// "Security readiness" section. Kept package-internal (not threaded through
// NewRecorder's constructor) deliberately: audit.NewRecorder() has ~30
// existing call sites across every domain feature in this service, and
// none of them should need to know this key exists.
func chainKey() []byte {
	chainKeyOnce.Do(func() {
		if raw := os.Getenv("FLEET_PARTNER_AUDIT_CHAIN_SECRET"); raw != "" {
			chainKeyValue = []byte(raw)
			return
		}
		chainKeyValue = []byte("INSECURE-DEV-ONLY-AUDIT-CHAIN-KEY-SET-FLEET_PARTNER_AUDIT_CHAIN_SECRET-IN-PRODUCTION")
	})
	return chainKeyValue
}

// chainedFields is the canonical, minimized input to computeEntryHash --
// deliberately its own type (not audit.Entry) so the hashed field set is
// explicit and reviewable independent of whatever fields Entry happens to
// grow later.
type chainedFields struct {
	id                      string
	organizationID          string
	actorType               string
	actorID                 string
	action                  string
	resourceType            string
	resourceID              string
	previousSummary         []byte // already redacted+marshaled JSON
	newSummary              []byte // already redacted+marshaled JSON
	requestID               string
	correlationID           string
	causationID             string
	approvalReference       string
	idempotencyKeyReference string
	stepUpReference         string
	occurredAt              time.Time
}

// canonicalizeJSON re-derives a stable byte form of a JSON value by
// unmarshaling then re-marshaling it. This is required, not cosmetic:
// previous_summary/new_summary are stored as Postgres jsonb, and jsonb does
// NOT preserve the exact bytes it was given -- it reparses and reserializes
// into its own internal key ordering, which is deterministic but NOT
// guaranteed to match Go's json.Marshal output byte-for-byte. Applying this
// same normalization at both write time (Record) and read time
// (VerifyChain) guarantees they derive the identical canonical bytes from
// the same logical value regardless of what happened in between at the
// storage layer -- without it, storing a value as jsonb and then verifying
// against a hash computed before storage would show every entry as
// "tampered" even with zero actual modification.
func canonicalizeJSON(raw []byte) string {
	if len(raw) == 0 {
		return "null"
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		// Not valid JSON (should be unreachable -- callers only ever pass
		// json.Marshal output or a jsonb column's own text form, both
		// always valid JSON) -- fall back to the raw bytes so hashing never
		// panics; a corrupted value will simply fail to verify, which is
		// the correct outcome anyway.
		return string(raw)
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return string(raw)
	}
	return string(canonical)
}

// canonicalBytes serializes chained fields deterministically: a fixed-order
// JSON object (Go's encoding/json emits struct fields in declaration order,
// never map-iteration order) so the same logical entry always hashes to the
// same bytes regardless of process, platform or Go version.
func canonicalBytes(partition, previousHash string, f chainedFields) []byte {
	type canonical struct {
		HashVersion             int    `json:"hashVersion"`
		Partition               string `json:"partition"`
		PreviousEntryHash       string `json:"previousEntryHash"`
		ID                      string `json:"id"`
		OrganizationID          string `json:"organizationId"`
		ActorType               string `json:"actorType"`
		ActorID                 string `json:"actorId"`
		Action                  string `json:"action"`
		ResourceType            string `json:"resourceType"`
		ResourceID              string `json:"resourceId"`
		PreviousSummary         string `json:"previousSummary"`
		NewSummary              string `json:"newSummary"`
		RequestID               string `json:"requestId"`
		CorrelationID           string `json:"correlationId"`
		CausationID             string `json:"causationId"`
		ApprovalReference       string `json:"approvalReference"`
		IdempotencyKeyReference string `json:"idempotencyKeyReference"`
		StepUpReference         string `json:"stepUpReference"`
		OccurredAtUnixNano      int64  `json:"occurredAtUnixNano"`
	}
	payload, _ := json.Marshal(canonical{
		HashVersion: currentHashVersion, Partition: partition, PreviousEntryHash: previousHash,
		ID: f.id, OrganizationID: f.organizationID, ActorType: f.actorType, ActorID: f.actorID,
		Action: f.action, ResourceType: f.resourceType, ResourceID: f.resourceID,
		PreviousSummary: canonicalizeJSON(f.previousSummary), NewSummary: canonicalizeJSON(f.newSummary),
		RequestID: f.requestID, CorrelationID: f.correlationID, CausationID: f.causationID,
		ApprovalReference: f.approvalReference, IdempotencyKeyReference: f.idempotencyKeyReference,
		StepUpReference: f.stepUpReference, OccurredAtUnixNano: f.occurredAt.UnixNano(),
	})
	return payload
}

// computeEntryHash is an HMAC-SHA256 over canonicalBytes, hex-encoded. HMAC
// (a keyed MAC), not a bare hash and not reversible encryption, per Section
// 28's "do not use reversible encryption as a hash" / "no invented custom
// cryptographic primitives" rules -- this is a standard, well-reviewed
// primitive from the Go standard library.
func computeEntryHash(key []byte, partition, previousHash string, f chainedFields) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(canonicalBytes(partition, previousHash, f))
	return hex.EncodeToString(mac.Sum(nil))
}

// latestEntryHash returns the entry_hash of the most recent row in
// partition, or "" if the partition has no rows yet (the first entry in a
// new chain hashes against an empty previous hash — the chain's genesis).
// Caller must already hold the partition's advisory lock (see Record).
func latestEntryHash(ctx context.Context, q database.Querier, partition string) (string, error) {
	var hash string
	err := q.QueryRow(ctx, `
		SELECT entry_hash FROM fleet_partner_audit_records
		WHERE chain_partition = $1 AND entry_hash IS NOT NULL
		ORDER BY chain_sequence DESC LIMIT 1`, partition).Scan(&hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return hash, nil
}

// PartitionKey builds the exported chain-partition key for organizationID
// and month (must be "YYYY-MM"). It is the only supported way to construct a
// partition string from outside this package -- callers such as
// internal/features/audit/http's verify-chain route must never build
// "<organizationId>:<month>" by hand, so the format and its validation stay
// in exactly one place. Rejects a malformed month rather than silently
// treating it as an empty/wrong partition, which would otherwise report a
// suspicious-looking empty chain instead of a caller input error.
func PartitionKey(organizationID, month string) (string, error) {
	if organizationID == "" {
		return "", fmt.Errorf("audit: organization id is required")
	}
	if _, err := time.Parse("2006-01", month); err != nil {
		return "", fmt.Errorf("audit: month must be formatted YYYY-MM")
	}
	return organizationID + ":" + month, nil
}

// ChainRecord is one row read back for verification.
type ChainRecord struct {
	ID                string
	OccurredAt        time.Time
	ChainPartition    string
	PreviousEntryHash string
	EntryHash         string
	HashVersion       int
}

// ChainBreak describes one detected integrity failure.
type ChainBreak struct {
	RecordID string
	Reason   string
}

// VerifyChainResult summarizes one partition's verification pass.
type VerifyChainResult struct {
	Partition      string
	EntriesChecked int
	LegacyEntries  int // hash_version = 0, expected and skipped, not a failure
	Breaks         []ChainBreak
}

// Valid reports whether verification found zero breaks.
func (r VerifyChainResult) Valid() bool { return len(r.Breaks) == 0 }

// VerifyChain recomputes every hash_version >= 1 entry in partition, in
// order, and confirms each one's stored previous_entry_hash matches the
// prior entry's actual entry_hash, and each stored entry_hash matches a
// fresh recomputation from that row's own (redacted, already-minimized)
// stored fields. A hash_version = 0 (legacy, pre-chaining) row is counted
// but never flagged — see the package doc comment's "legacy boundary".
//
// This detects: a modified previous_summary/new_summary/actor/action/
// resource (entry_hash recomputation mismatches), a deleted-and-reinserted
// row (previous_entry_hash chain link breaks), and a row inserted out of
// order or into the wrong partition (chain link breaks against a hash that
// was never actually adjacent).
func VerifyChain(ctx context.Context, q database.Querier, partition string) (VerifyChainResult, error) {
	result := VerifyChainResult{Partition: partition}

	// Legacy (pre-migration-000069) rows have chain_partition IS NULL, so
	// "WHERE chain_partition = $1" alone would silently exclude them
	// entirely rather than counting them as legacy -- they need to be found
	// by organization + the same calendar month instead, reconstructed from
	// partition's own "<organizationId>:<YYYY-MM>" format (organization IDs
	// never contain ':', so splitting on the first one is unambiguous).
	organizationID, month, hasMonth := strings.Cut(partition, ":")

	query := `
		SELECT id, occurred_at, actor_type, actor_id, action, resource_type, resource_id,
		       previous_summary, new_summary, request_id, correlation_id,
		       COALESCE(causation_id, ''), COALESCE(approval_reference, ''),
		       COALESCE(idempotency_key_reference, ''), COALESCE(step_up_reference, ''),
		       COALESCE(previous_entry_hash, ''), COALESCE(entry_hash, ''), hash_version, organization_id
		FROM fleet_partner_audit_records
		WHERE chain_partition = $1
		   OR (organization_id = $2 AND chain_partition IS NULL AND to_char(occurred_at, 'YYYY-MM') = $3)
		ORDER BY chain_sequence ASC`
	if !hasMonth {
		// Malformed partition string (should be unreachable from
		// chainPartition's own output) -- fall back to the exact-match-only
		// query so this never panics or scans every legacy row ever
		// written across every organization.
		query = `
			SELECT id, occurred_at, actor_type, actor_id, action, resource_type, resource_id,
			       previous_summary, new_summary, request_id, correlation_id,
			       COALESCE(causation_id, ''), COALESCE(approval_reference, ''),
			       COALESCE(idempotency_key_reference, ''), COALESCE(step_up_reference, ''),
			       COALESCE(previous_entry_hash, ''), COALESCE(entry_hash, ''), hash_version, organization_id
			FROM fleet_partner_audit_records
			WHERE chain_partition = $1
			ORDER BY chain_sequence ASC`
	}

	var rows pgx.Rows
	var err error
	if hasMonth {
		rows, err = q.Query(ctx, query, partition, organizationID, month)
	} else {
		rows, err = q.Query(ctx, query, partition)
	}
	if err != nil {
		return result, fmt.Errorf("audit: verify chain query: %w", err)
	}
	defer rows.Close()

	expectedPrevious := ""
	for rows.Next() {
		var (
			f                           chainedFields
			previousSummary, newSummary []byte
			storedPrevious, storedHash  string
			hashVersion                 int
		)
		if err := rows.Scan(&f.id, &f.occurredAt, &f.actorType, &f.actorID, &f.action, &f.resourceType, &f.resourceID,
			&previousSummary, &newSummary, &f.requestID, &f.correlationID,
			&f.causationID, &f.approvalReference, &f.idempotencyKeyReference, &f.stepUpReference,
			&storedPrevious, &storedHash, &hashVersion, &f.organizationID,
		); err != nil {
			return result, fmt.Errorf("audit: verify chain scan: %w", err)
		}
		if hashVersion == 0 {
			result.LegacyEntries++
			continue
		}
		f.previousSummary, f.newSummary = previousSummary, newSummary
		result.EntriesChecked++

		if storedPrevious != expectedPrevious {
			result.Breaks = append(result.Breaks, ChainBreak{RecordID: f.id,
				Reason: "previous_entry_hash does not match the prior entry's actual hash"})
		}
		recomputed := computeEntryHash(chainKey(), partition, storedPrevious, f)
		if recomputed != storedHash {
			result.Breaks = append(result.Breaks, ChainBreak{RecordID: f.id,
				Reason: "entry_hash does not match a recomputation from this row's own stored fields"})
		}
		expectedPrevious = storedHash
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("audit: verify chain rows: %w", err)
	}
	return result, nil
}
