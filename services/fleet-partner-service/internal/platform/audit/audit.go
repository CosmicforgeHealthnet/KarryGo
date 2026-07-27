// Package audit implements the append-only audit foundation.
//
// Sensitive future mutations record who did what to which resource, with
// redacted before/after summaries, in the same transaction as the domain
// mutation. Audit rows are never updated or deleted by runtime code.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
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
}

// Sensitive keys whose values are always redacted in summaries.
var sensitiveKeys = []string{
	"password", "secret", "token", "authorization", "account_number",
	"bank", "document", "nin", "bvn", "passport", "ssn", "card",
}

// Redact returns a copy of values with sensitive keys masked and everything
// serialized shallowly; it never returns secrets, identity documents or raw
// financial account details.
func Redact(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	redacted := make(map[string]any, len(values))
	for key, value := range values {
		if isSensitiveKey(key) {
			redacted[key] = "[REDACTED]"
			continue
		}
		redacted[key] = value
	}
	return redacted
}

func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, needle := range sensitiveKeys {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

// Recorder appends audit entries.
type Recorder struct{}

// NewRecorder constructs the append-only recorder.
func NewRecorder() *Recorder {
	return &Recorder{}
}

// Record appends the entry using q — pass the transaction of the domain
// mutation for atomicity. Summaries are redacted before storage.
func (r *Recorder) Record(ctx context.Context, q database.Querier, entry Entry) error {
	if entry.OrganizationID == "" {
		return fmt.Errorf("audit: organization id is required")
	}
	if entry.Action == "" || entry.ResourceType == "" || entry.ResourceID == "" {
		return fmt.Errorf("audit: action and resource identity are required")
	}
	if entry.ActorType == "" || entry.ActorID == "" {
		return fmt.Errorf("audit: actor is required")
	}

	previous, err := json.Marshal(Redact(entry.PreviousValues))
	if err != nil {
		return fmt.Errorf("audit: marshal previous values: %w", err)
	}
	next, err := json.Marshal(Redact(entry.NewValues))
	if err != nil {
		return fmt.Errorf("audit: marshal new values: %w", err)
	}
	network, err := json.Marshal(entry.NetworkMeta)
	if err != nil {
		return fmt.Errorf("audit: marshal network metadata: %w", err)
	}

	_, err = q.Exec(ctx, `
		INSERT INTO fleet_partner_audit_records
			(id, organization_id, branch_id, actor_type, actor_id, action,
			 resource_type, resource_id, previous_summary, new_summary, reason,
			 request_id, correlation_id, network_meta, occurred_at)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, $8, $9, $10, NULLIF($11, ''), $12, $13, $14, $15)`,
		ids.New(ids.PrefixAudit), entry.OrganizationID, entry.BranchID,
		entry.ActorType, entry.ActorID, entry.Action,
		entry.ResourceType, entry.ResourceID, previous, next, entry.Reason,
		entry.RequestID, entry.CorrelationID, network, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("audit: append record: %w", err)
	}
	return nil
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
