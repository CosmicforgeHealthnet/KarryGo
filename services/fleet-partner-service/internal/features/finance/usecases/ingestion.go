package usecases

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/finance/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
)

func (s *Service) IngestWalletEvent(ctx context.Context, raw []byte) (string, error) {
	var envelope events.Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "", err
	}
	if envelope.Producer != "payment-wallet-service" || envelope.EventID == "" || envelope.AggregateVersion < 1 || envelope.OrganizationID == "" {
		return "", fmt.Errorf("finance event: invalid envelope")
	}
	payloadHash := events.HashPayload(envelope.Payload)
	inbox := s.inbox
	outcome := "PROCESSED"
	err := s.repo.WithinTx(ctx, func(tx pgx.Tx) error {
		if err := inbox.Begin(ctx, tx, "fleet-finance", envelope.EventID, envelope.EventType, envelope.SchemaVersion, raw); err != nil {
			if errors.Is(err, events.ErrDuplicateEvent) {
				outcome = "DUPLICATE"
				return nil
			}
			return err
		}
		prior, priorHash, found, err := s.repo.LockSourceVersion(ctx, tx, envelope.AggregateType, envelope.AggregateID)
		if err != nil {
			return err
		}
		if found {
			if envelope.AggregateVersion < prior {
				outcome = "OLDER"
				return inbox.MarkProcessed(ctx, tx, "fleet-finance", envelope.EventID)
			}
			if envelope.AggregateVersion == prior {
				if payloadHash != priorHash {
					return models.ErrEventConflict
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
		projectionType := models.ProjectionTypeForEvent(envelope.EventType)
		if projectionType == "" {
			return fmt.Errorf("finance event: unsupported event type")
		}
		var payload map[string]any
		// UseNumber keeps money exact: through float64, amounts above 2^53
		// minor units would be silently rounded.
		decoder := json.NewDecoder(bytes.NewReader(envelope.Payload))
		decoder.UseNumber()
		if err = decoder.Decode(&payload); err != nil {
			return err
		}
		safeStatus, _ := payload["status"].(string)
		if safeStatus == "" {
			safeStatus = models.StatusForEvent(envelope.EventType)
		}
		subjectType, subjectID := "AGGREGATE", envelope.AggregateID
		if value, ok := payload["jobReference"].(string); ok && value != "" {
			subjectType, subjectID = "JOB", value
		}
		amount, err := models.MinorAmount(payload, "amountMinor", "amountPaidMinor")
		if err != nil {
			return err
		}
		currency, _ := payload["currency"].(string)
		if amount == nil {
			currency = ""
		}
		reconciliationStatus := "IDLE"
		if freshness == "GAP_DETECTED" {
			reconciliationStatus = "PENDING"
		}
		write := models.ProjectionWrite{OrganizationID: envelope.OrganizationID, ProjectionType: projectionType,
			SubjectType: subjectType, SubjectID: subjectID, WalletReference: envelope.AggregateID, AmountMinor: amount,
			Currency: currency, SafeStatus: safeStatus, SourceVersion: envelope.AggregateVersion, Freshness: freshness,
			LastEventID: envelope.EventID, ReconciliationStatus: reconciliationStatus}
		if err = s.repo.UpsertProjection(ctx, tx, write); err != nil {
			return err
		}
		// A paid-invoice event carries Wallet's opaque payment record. Project it
		// separately so job/payment gates never infer payment from invoice shape.
		if paymentID, ok := payload["paymentId"].(string); ok && paymentID != "" {
			payment := write
			payment.WalletReference = paymentID
			payment.SafeStatus = map[bool]string{true: "SUCCEEDED", false: safeStatus}[strings.Contains(envelope.EventType, ".paid.")]
			if err = s.repo.UpsertPaymentVerification(ctx, tx, payment); err != nil {
				return err
			}
		}
		if err = s.repo.UpsertSourceVersion(ctx, tx, envelope.AggregateType, envelope.AggregateID, envelope.AggregateVersion, envelope.EventID, payloadHash); err != nil {
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
