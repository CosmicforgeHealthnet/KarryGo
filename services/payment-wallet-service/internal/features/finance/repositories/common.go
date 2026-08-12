// Package repositories implements PostgreSQL persistence for the financial
// domain.
//
// Every method takes an explicit database.Querier so a caller can compose the
// mutation, its history, its audit record and its outbox event into a single
// transaction. No method opens its own transaction.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
)

// ErrNotFound means the requested record does not exist in the caller's scope.
var ErrNotFound = errors.New("finance: record not found")

// ErrConflict means a uniqueness or state constraint refused the write.
var ErrConflict = errors.New("finance: conflicting record")

// ErrVersionMismatch means the caller's expected version is stale; another
// writer changed the row first.
var ErrVersionMismatch = errors.New("finance: expected version does not match")

// AppendHistory records one state transition. History is written in the same
// transaction as the transition it describes.
func AppendHistory(ctx context.Context, q database.Querier, organizationID string,
	entityType string, entityID string, fromState string, toState string,
	actorType string, actorID string, reason string, metadata map[string]any,
	correlationID string, causationID string) error {
	if actorType == "" {
		actorType = "SYSTEM"
	}
	if actorID == "" {
		actorID = "system"
	}
	encoded, err := json.Marshal(orEmptyMap(metadata))
	if err != nil {
		return fmt.Errorf("finance: marshal history metadata: %w", err)
	}
	_, err = q.Exec(ctx, `
		INSERT INTO finance_state_history
			(organization_id, entity_type, entity_id, from_state, to_state,
			 actor_type, actor_id, reason, metadata, correlation_id, causation_id, occurred_at)
		VALUES (NULLIF($1, ''), $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now())`,
		organizationID, entityType, entityID, fromState, toState,
		actorType, actorID, reason, encoded, correlationID, causationID)
	if err != nil {
		return fmt.Errorf("finance: append history: %w", err)
	}
	return nil
}

// HistoryEntry is one recorded transition.
type HistoryEntry struct {
	EntityType    string          `json:"entityType"`
	EntityID      string          `json:"entityId"`
	FromState     string          `json:"fromState"`
	ToState       string          `json:"toState"`
	ActorType     string          `json:"actorType"`
	ActorID       string          `json:"actorId"`
	Reason        string          `json:"reason,omitempty"`
	Metadata      json.RawMessage `json:"metadata"`
	CorrelationID string          `json:"correlationId,omitempty"`
	OccurredAt    string          `json:"occurredAt"`
}

// ListHistory returns the append-only history of one entity, oldest first.
func ListHistory(ctx context.Context, q database.Querier, entityType string,
	entityID string) ([]HistoryEntry, error) {
	rows, err := q.Query(ctx, `
		SELECT entity_type, entity_id, from_state, to_state, actor_type, actor_id,
		       reason, metadata, correlation_id, to_char(occurred_at, 'YYYY-MM-DD"T"HH24:MI:SSOF')
		FROM finance_state_history
		WHERE entity_type = $1 AND entity_id = $2
		ORDER BY occurred_at, id`, entityType, entityID)
	if err != nil {
		return nil, fmt.Errorf("finance: list history: %w", err)
	}
	defer rows.Close()

	var entries []HistoryEntry
	for rows.Next() {
		var entry HistoryEntry
		if err := rows.Scan(&entry.EntityType, &entry.EntityID, &entry.FromState, &entry.ToState,
			&entry.ActorType, &entry.ActorID, &entry.Reason, &entry.Metadata,
			&entry.CorrelationID, &entry.OccurredAt); err != nil {
			return nil, fmt.Errorf("finance: scan history: %w", err)
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func orEmptyMap(values map[string]any) map[string]any {
	if values == nil {
		return map[string]any{}
	}
	return values
}
