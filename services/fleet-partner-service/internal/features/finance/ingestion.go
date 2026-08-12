package finance

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"strings"
)

var ErrEventConflict = errors.New("finance event: source version payload conflict")

func (s *Service) IngestWalletEvent(ctx context.Context, raw []byte) (string, error) {
	var envelope events.Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "", err
	}
	if envelope.Producer != "payment-wallet-service" || envelope.EventID == "" || envelope.AggregateVersion < 1 || envelope.OrganizationID == "" {
		return "", fmt.Errorf("finance event: invalid envelope")
	}
	payloadHash := events.HashPayload(envelope.Payload)
	inbox := events.NewInboxRepository(s.pool)
	outcome := "PROCESSED"
	err := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := inbox.Begin(ctx, tx, "fleet-finance", envelope.EventID, envelope.EventType, envelope.SchemaVersion, raw); err != nil {
			if errors.Is(err, events.ErrDuplicateEvent) {
				outcome = "DUPLICATE"
				return nil
			}
			return err
		}
		var prior int64
		var priorHash string
		err := tx.QueryRow(ctx, `SELECT source_version,payload_hash FROM fleet_partner_finance_source_versions WHERE aggregate_type=$1 AND aggregate_id=$2 FOR UPDATE`, envelope.AggregateType, envelope.AggregateID).Scan(&prior, &priorHash)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err == nil {
			if envelope.AggregateVersion < prior {
				outcome = "OLDER"
				return inbox.MarkProcessed(ctx, tx, "fleet-finance", envelope.EventID)
			}
			if envelope.AggregateVersion == prior {
				if payloadHash != priorHash {
					return ErrEventConflict
				}
				outcome = "DUPLICATE_VERSION"
				return inbox.MarkProcessed(ctx, tx, "fleet-finance", envelope.EventID)
			}
		}
		freshness := "CURRENT"
		if prior > 0 && envelope.AggregateVersion > prior+1 {
			freshness = "GAP_DETECTED"
			outcome = "GAP_DETECTED"
		}
		projectionType := projectionTypeForEvent(envelope.EventType)
		if projectionType == "" {
			return fmt.Errorf("finance event: unsupported event type")
		}
		var payload map[string]any
		if err = json.Unmarshal(envelope.Payload, &payload); err != nil {
			return err
		}
		safeStatus, _ := payload["status"].(string)
		if safeStatus == "" {
			safeStatus = statusForEvent(envelope.EventType)
		}
		subjectType, subjectID := "AGGREGATE", envelope.AggregateID
		if value, ok := payload["jobReference"].(string); ok && value != "" {
			subjectType, subjectID = "JOB", value
		}
		var amount *int64
		if value, ok := payload["amountMinor"].(float64); ok && value >= 0 {
			v := int64(value)
			amount = &v
		} else if value, ok := payload["amountPaidMinor"].(float64); ok && value >= 0 {
			v := int64(value)
			amount = &v
		}
		currency, _ := payload["currency"].(string)
		if amount == nil {
			currency = ""
		}
		reconciliationStatus := "IDLE"
		if freshness == "GAP_DETECTED" {
			reconciliationStatus = "PENDING"
		}
		_, err = tx.Exec(ctx, `INSERT INTO fleet_partner_finance_projections(id,organization_id,projection_type,subject_type,subject_id,wallet_reference,amount_minor,currency,safe_status,source_version,freshness_state,last_event_id,reconciliation_status,reconciliation_next_attempt_at) VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9,$10,$11,$12,$13,now()) ON CONFLICT(organization_id,projection_type,wallet_reference) DO UPDATE SET subject_type=EXCLUDED.subject_type,subject_id=EXCLUDED.subject_id,amount_minor=EXCLUDED.amount_minor,currency=EXCLUDED.currency,safe_status=EXCLUDED.safe_status,source_version=EXCLUDED.source_version,freshness_state=EXCLUDED.freshness_state,last_event_id=EXCLUDED.last_event_id,reconciliation_status=EXCLUDED.reconciliation_status,reconciliation_next_attempt_at=now(),reconciliation_attempts=CASE WHEN EXCLUDED.reconciliation_status='IDLE' THEN 0 ELSE fleet_partner_finance_projections.reconciliation_attempts END,reconciliation_safe_error_code='',last_synced_at=now(),version=fleet_partner_finance_projections.version+1,updated_at=now()`, ids.New("fpr"), envelope.OrganizationID, projectionType, subjectType, subjectID, envelope.AggregateID, amount, currency, safeStatus, envelope.AggregateVersion, freshness, envelope.EventID, reconciliationStatus)
		if err != nil {
			return err
		}
		// A paid-invoice event carries Wallet's opaque payment record. Project it
		// separately so job/payment gates never infer payment from invoice shape.
		if paymentID, ok := payload["paymentId"].(string); ok && paymentID != "" {
			_, err = tx.Exec(ctx, `INSERT INTO fleet_partner_finance_projections
				(id,organization_id,projection_type,subject_type,subject_id,wallet_reference,amount_minor,currency,safe_status,source_version,freshness_state,last_event_id,reconciliation_status,reconciliation_next_attempt_at)
				VALUES($1,$2,'PAYMENT_VERIFICATION',$3,$4,$5,$6,NULLIF($7,''),$8,$9,$10,$11,$12,now())
				ON CONFLICT(organization_id,projection_type,wallet_reference) DO UPDATE SET
				 amount_minor=EXCLUDED.amount_minor,currency=EXCLUDED.currency,safe_status=EXCLUDED.safe_status,
				 source_version=EXCLUDED.source_version,freshness_state=EXCLUDED.freshness_state,last_event_id=EXCLUDED.last_event_id,
				 reconciliation_status=EXCLUDED.reconciliation_status,reconciliation_next_attempt_at=now(),
				 last_synced_at=now(),version=fleet_partner_finance_projections.version+1,updated_at=now()`,
				ids.New("fpr"), envelope.OrganizationID, subjectType, subjectID, paymentID, amount, currency,
				map[bool]string{true: "SUCCEEDED", false: safeStatus}[strings.Contains(envelope.EventType, ".paid.")],
				envelope.AggregateVersion, freshness, envelope.EventID, reconciliationStatus)
			if err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `INSERT INTO fleet_partner_finance_source_versions(aggregate_type,aggregate_id,source_version,last_event_id,payload_hash) VALUES($1,$2,$3,$4,$5) ON CONFLICT(aggregate_type,aggregate_id) DO UPDATE SET source_version=EXCLUDED.source_version,last_event_id=EXCLUDED.last_event_id,payload_hash=EXCLUDED.payload_hash,updated_at=now()`, envelope.AggregateType, envelope.AggregateID, envelope.AggregateVersion, envelope.EventID, payloadHash)
		if err != nil {
			return err
		}
		if err = s.audit.Record(ctx, tx, audit.Entry{OrganizationID: envelope.OrganizationID, ActorType: "SERVICE", ActorID: "payment-wallet-service", Action: "finance.projection.updated", ResourceType: "finance_projection", ResourceID: envelope.AggregateID, NewValues: map[string]any{"type": projectionType, "status": safeStatus, "freshness": freshness, "sourceVersion": envelope.AggregateVersion}, CorrelationID: envelope.CorrelationID}); err != nil {
			return err
		}
		if err = s.events.Publish(ctx, tx, "fleet.finance.projection.updated.v1", "FINANCE_PROJECTION", envelope.AggregateID, envelope.AggregateVersion, envelope.OrganizationID, envelope.CorrelationID, map[string]any{"projectionType": projectionType, "walletReference": envelope.AggregateID, "safeStatus": safeStatus, "freshness": freshness}); err != nil {
			return err
		}
		return inbox.MarkProcessed(ctx, tx, "fleet-finance", envelope.EventID)
	})
	if errors.Is(err, events.ErrDuplicateEvent) {
		return "DUPLICATE", nil
	}
	return outcome, err
}
func projectionTypeForEvent(t string) string {
	// Ordered, entity-specific matching is intentional. All of the newer event
	// names begin with "wallet.", so a generic wallet prefix would also match
	// wallet.invoice and wallet.refund and make routing depend on map iteration.
	for _, route := range []struct{ needle, value string }{
		{"earning_account.", "EARNING_ACCOUNT"},
		{"subscription.", "SUBSCRIPTION"}, {"invoice.", "INVOICE"},
		{"hold.", "HOLD"}, {"commission.", "COMMISSION"},
		{"earning.", "EARNING"}, {"settlement.", "SETTLEMENT"},
		{"refund.", "REFUND"}, {"reversal.", "REVERSAL"},
		{"wallet.organization.", "WALLET"}, {"finance.wallet.", "WALLET"},
	} {
		if strings.Contains(t, route.needle) {
			return route.value
		}
	}
	return ""
}
func statusForEvent(t string) string {
	parts := strings.Split(t, ".")
	if len(parts) > 2 {
		return strings.ToUpper(parts[len(parts)-2])
	}
	return "UNKNOWN"
}
