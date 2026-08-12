package repositories

import (
	"context"
	"encoding/json"
	"fmt"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

// SecurityEventRepository appends authentication security events. The table
// is append-only at the database level.
type SecurityEventRepository struct {
	db database.Querier
}

// NewSecurityEventRepository wires the repository.
func NewSecurityEventRepository(db database.Querier) *SecurityEventRepository {
	return &SecurityEventRepository{db: db}
}

// Record appends one security event. Contact values are recorded as keyed
// hashes only, so the table cannot be used to enumerate accounts.
func (r *SecurityEventRepository) Record(ctx context.Context, q database.Querier, event models.SecurityEvent) error {
	if q == nil {
		q = r.db
	}
	if event.Severity == "" {
		event.Severity = models.SeverityInfo
	}
	detail := event.Detail
	if detail == nil {
		detail = map[string]any{}
	}
	encoded, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("auth: marshal security event detail: %w", err)
	}

	_, err = q.Exec(ctx, `
		INSERT INTO fleet_partner_auth_security_events
			(id, event_type, severity, identity_id, organization_id, email_hash, phone_hash,
			 ip_hash, user_agent_hash, detail, request_id, correlation_id, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, now())`,
		ids.New("sec"), event.EventType, event.Severity, event.IdentityID, event.OrganizationID,
		event.EmailHash, event.PhoneHash, event.IPHash, event.UserAgentHash, encoded,
		event.RequestID, event.CorrelationID,
	)
	if err != nil {
		return fmt.Errorf("auth: record security event: %w", err)
	}
	return nil
}

// CountByType returns the number of events of a type (used by tests and
// operational queries).
func (r *SecurityEventRepository) CountByType(ctx context.Context, eventType string) (int64, error) {
	var count int64
	err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM fleet_partner_auth_security_events WHERE event_type = $1`, eventType,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("auth: count security events: %w", err)
	}
	return count, nil
}
