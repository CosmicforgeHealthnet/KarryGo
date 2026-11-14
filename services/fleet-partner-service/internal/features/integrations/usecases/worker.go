package usecases

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/integrations/clients"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/integrations/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// RegisterWorker reuses the Section 27 durable job/lease worker. No separate
// delivery queue or in-memory retry loop is introduced.
func (s *Service) RegisterWorker(worker *jobs.Worker) {
	if s == nil || worker == nil || !s.cfg.WebhookDeliveryWorkerEnabled || s.cfg.IntegrationsKillSwitch {
		return
	}
	worker.Register(models.JobTypeWebhookDelivery, s.HandleDeliveryJob)
	s.workerRegistered.Store(true)
}

// StartFanoutCoordinator periodically expands already-committed outbox events
// into durable delivery jobs. Delivery retries and leases remain entirely in
// the shared jobs.Worker; this coordinator performs only transactional fanout.
func (s *Service) StartFanoutCoordinator(ctx context.Context, wg *sync.WaitGroup) {
	if s == nil || !s.cfg.WebhookDeliveryWorkerEnabled || s.cfg.IntegrationsKillSwitch || wg == nil {
		return
	}
	s.fanoutStarted.Store(true)
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(s.cfg.WebhookFanoutInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = s.FanoutOnce(ctx, s.cfg.WebhookFanoutBatchSize)
			}
		}
	}()
}

func (s *Service) FanoutOnce(ctx context.Context, limit int) (int, error) {
	if !s.cfg.WebhooksEnabled || s.cfg.IntegrationsKillSwitch {
		return 0, nil
	}
	if limit <= 0 {
		limit = 50
	}
	public := events.PublicIntegrationTypes()
	if len(public) == 0 {
		return 0, nil
	}
	items, err := s.store.PendingPublicEvents(ctx, public, limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, item := range items {
		var envelope events.Envelope
		if err = json.Unmarshal(item.Raw, &envelope); err != nil {
			return processed, err
		}
		if !events.IsPublicIntegration(envelope.EventType) {
			continue
		}
		if err = events.ValidateEnvelope(envelope); err != nil {
			return processed, fmt.Errorf("integrations: public event contract invalid: %w", err)
		}
		err = s.store.WithinTx(ctx, func(tx pgx.Tx) error {
			claimed, claimErr := s.store.ClaimFanout(ctx, tx, envelope.EventID, envelope.OrganizationID, envelope.EventType)
			if claimErr != nil || !claimed {
				return claimErr
			}
			subscriptionIDs, queryErr := s.store.ActiveSubscriptionsFor(ctx, tx, envelope.OrganizationID, envelope.EventType)
			if queryErr != nil {
				return queryErr
			}
			// pgx transactions use one connection: consume and close the
			// subscription result set before issuing inserts on that connection.
			for _, subID := range subscriptionIDs {
				deliveryID := ids.New("whd")
				inserted, insertErr := s.store.InsertFirstDelivery(ctx, tx, deliveryID, envelope.OrganizationID, subID, envelope.EventID, envelope.EventType, envelope.CorrelationID)
				if insertErr != nil {
					return insertErr
				}
				if !inserted {
					continue
				}
				jobID, enqueueErr := s.queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: envelope.OrganizationID, JobType: models.JobTypeWebhookDelivery, DedupeKey: "webhook:" + deliveryID, Payload: map[string]any{"organizationId": envelope.OrganizationID, "deliveryId": deliveryID}, CorrelationID: envelope.CorrelationID, CausationID: envelope.EventID, MaxAttempts: s.cfg.WebhookDeliveryMaxAttempts})
				if enqueueErr != nil {
					return enqueueErr
				}
				if updateErr := s.store.SetDeliveryJob(ctx, tx, deliveryID, jobID); updateErr != nil {
					return updateErr
				}
			}
			return nil
		})
		if err != nil {
			return processed, err
		}
		processed++
	}
	if s.metrics != nil {
		s.metrics.IncCounter("fleet_partner_webhook_fanout_events_total", nil, float64(processed))
		s.refreshWebhookMetrics(ctx)
	}
	return processed, nil
}

type deliveryJobPayload struct {
	OrganizationID string `json:"organizationId"`
	DeliveryID     string `json:"deliveryId"`
}

func (s *Service) HandleDeliveryJob(ctx context.Context, job jobs.Job) error {
	startedAt := time.Now()
	if s.metrics != nil {
		defer func() {
			s.metrics.ObserveDuration("fleet_partner_webhook_delivery_latency", nil, time.Since(startedAt))
		}()
	}
	if !s.cfg.WebhookDeliveryWorkerEnabled || s.cfg.IntegrationsKillSwitch {
		return &jobs.ClassifiedError{Class: jobs.ErrorClassPermanent, Code: "webhook_worker_disabled", SafeMessage: "webhook worker disabled"}
	}
	var payload deliveryJobPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil || payload.OrganizationID != job.OrganizationID || payload.DeliveryID == "" {
		return &jobs.ClassifiedError{Class: jobs.ErrorClassValidation, Code: "invalid_webhook_job", SafeMessage: "invalid webhook job payload"}
	}
	if ctx.Err() != nil {
		return jobs.ErrShutdownSafeToRelease
	}
	delivery, err := s.store.DeliveryContext(ctx, job.OrganizationID, payload.DeliveryID)
	subID, eventID, eventType, endpoint, correlation, status := delivery.SubscriptionID, delivery.EventID, delivery.EventType, delivery.Endpoint, delivery.CorrelationID, delivery.Status
	currentCipher, previousCipher, rawEvent := delivery.CurrentCipher, delivery.PreviousCipher, delivery.RawEvent
	currentVersion, previousVersion, previousUntil := delivery.CurrentVersion, delivery.PreviousVersion, delivery.PreviousUntil
	if err != nil {
		return &jobs.ClassifiedError{Class: jobs.ErrorClassPermanent, Code: "webhook_delivery_not_found", SafeMessage: "webhook delivery is unavailable", Err: err}
	}
	if status == models.DeliverySucceeded {
		return nil
	}
	if _, _, err = s.destination.Validate(ctx, endpoint); err != nil {
		return s.deliveryFailure(ctx, job, payload.DeliveryID, "SSRF_BLOCKED", jobs.ErrorClassPermanent, 0, 0, err)
	}
	var envelope events.Envelope
	if err = json.Unmarshal(rawEvent, &envelope); err != nil || envelope.EventID != eventID || envelope.EventType != eventType || envelope.OrganizationID != job.OrganizationID || !events.IsPublicIntegration(eventType) {
		return s.deliveryFailure(ctx, job, payload.DeliveryID, "INVALID_EVENT_CONTRACT", jobs.ErrorClassPermanent, 0, 0, err)
	}
	if err = events.ValidateEnvelope(envelope); err != nil {
		return s.deliveryFailure(ctx, job, payload.DeliveryID, "INVALID_EVENT_CONTRACT", jobs.ErrorClassPermanent, 0, 0, err)
	}
	var data any
	if err = json.Unmarshal(envelope.Payload, &data); err != nil {
		return s.deliveryFailure(ctx, job, payload.DeliveryID, "INVALID_EVENT_PAYLOAD", jobs.ErrorClassPermanent, 0, 0, err)
	}
	body, err := json.Marshal(models.PublicWebhookEnvelope{EventID: eventID, EventType: eventType, SchemaVersion: envelope.SchemaVersion, OrganizationID: job.OrganizationID, CorrelationID: correlation, OccurredAt: envelope.OccurredAt, Data: data})
	if err != nil {
		return err
	}
	if err = models.ValidateWebhookPayload(body); err != nil {
		return s.deliveryFailure(ctx, job, payload.DeliveryID, "PAYLOAD_MINIMIZATION_FAILED", jobs.ErrorClassPermanent, 0, 0, err)
	}
	currentSecret, err := models.DecryptSecret(s.cfg.WebhookSecretEncryptionKey, currentCipher)
	if err != nil {
		return s.deliveryFailure(ctx, job, payload.DeliveryID, "SECRET_DECRYPTION_FAILED", jobs.ErrorClassPermanent, 0, 0, err)
	}
	secrets := []models.SigningSecret{{Version: currentVersion, Secret: currentSecret}}
	if len(previousCipher) > 0 && previousVersion != nil && previousUntil != nil && previousUntil.After(time.Now().UTC()) {
		previousSecret, decryptErr := models.DecryptSecret(s.cfg.WebhookSecretEncryptionKey, previousCipher)
		if decryptErr != nil {
			return s.deliveryFailure(ctx, job, payload.DeliveryID, "SECRET_DECRYPTION_FAILED", jobs.ErrorClassPermanent, 0, 0, decryptErr)
		}
		secrets = append(secrets, models.SigningSecret{Version: *previousVersion, Secret: previousSecret})
	}
	timestamp := time.Now().UTC().Unix()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return s.deliveryFailure(ctx, job, payload.DeliveryID, "INVALID_ENDPOINT", jobs.ErrorClassPermanent, 0, 0, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "KarryGo-Webhook/1.0")
	request.Header.Set("X-KarryGo-Webhook-Id", subID)
	request.Header.Set("X-KarryGo-Delivery-Id", payload.DeliveryID)
	request.Header.Set("X-KarryGo-Event-Id", eventID)
	request.Header.Set("X-KarryGo-Webhook-Timestamp", strconv.FormatInt(timestamp, 10))
	request.Header.Set("X-KarryGo-Webhook-Signature", models.SignWebhook(subID, payload.DeliveryID, eventID, timestamp, body, secrets))
	request.Header.Set("Idempotency-Key", payload.DeliveryID)
	request.Header.Set("X-Correlation-ID", correlation)
	_ = s.store.MarkDelivering(ctx, job.OrganizationID, payload.DeliveryID)
	response, err := s.httpClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return s.deliveryFailure(context.Background(), job, payload.DeliveryID, "UNKNOWN_OUTCOME", jobs.ErrorClassUnknownExternalOutcome, 0, 0, err)
		}
		return s.deliveryFailure(ctx, job, payload.DeliveryID, "NETWORK_OR_TIMEOUT", jobs.ErrorClassUnknownExternalOutcome, 0, 0, err)
	}
	defer response.Body.Close()
	limited, readErr := io.ReadAll(io.LimitReader(response.Body, 1025))
	if readErr != nil {
		return s.deliveryFailure(ctx, job, payload.DeliveryID, "RESPONSE_READ_FAILED", jobs.ErrorClassUnknownExternalOutcome, response.StatusCode, 0, readErr)
	}
	excerpt := fmt.Sprintf("response body omitted (%d bytes sampled)", len(limited))
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return s.deliverySuccess(ctx, job, payload.DeliveryID, response.StatusCode, excerpt)
	}
	retryAfter := clients.ParseRetryAfter(response.Header.Get("Retry-After"), time.Now().UTC())
	if response.StatusCode == 408 || response.StatusCode == 429 || response.StatusCode == 502 || response.StatusCode == 503 || response.StatusCode == 504 {
		return s.deliveryFailure(ctx, job, payload.DeliveryID, "HTTP_"+strconv.Itoa(response.StatusCode), jobs.ErrorClassTransient, response.StatusCode, retryAfter, errors.New("transient webhook response"))
	}
	return s.deliveryFailure(ctx, job, payload.DeliveryID, "HTTP_"+strconv.Itoa(response.StatusCode), jobs.ErrorClassPermanent, response.StatusCode, 0, errors.New("permanent webhook response"))
}

func (s *Service) deliverySuccess(ctx context.Context, job jobs.Job, id string, status int, excerpt string) error {
	err := s.store.WithinTx(ctx, func(tx pgx.Tx) error {
		changed, updateErr := s.store.MarkDeliverySucceeded(ctx, tx, job.OrganizationID, id, status, excerpt)
		if updateErr != nil || !changed {
			return updateErr
		}
		_ = s.store.ResetFailureCount(ctx, tx, job.OrganizationID, id)
		actor := models.Actor{Tenant: tenant.Context{OrganizationID: job.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "webhook-delivery-worker", CorrelationID: job.CorrelationID}}
		return s.recordAndPublish(ctx, tx, actor, "webhook.delivery_succeeded", "webhook.delivery_succeeded.v1", "webhook_delivery", id, 1, map[string]any{"status": models.DeliverySucceeded, "eventId": job.CausationID, "attempt": job.Attempts})
	})
	if err == nil && s.metrics != nil {
		s.metrics.IncCounter("fleet_partner_webhook_deliveries_total", map[string]string{"status": "succeeded"}, 1)
		s.refreshWebhookMetrics(ctx)
	}
	return err
}

func (s *Service) deliveryFailure(ctx context.Context, job jobs.Job, id, code string, class jobs.ErrorClass, responseStatus int, retryAfter time.Duration, cause error) error {
	terminal := !class.Retryable() || job.Attempts >= job.MaxAttempts
	status := models.DeliveryRetryScheduled
	if terminal {
		status = models.DeliveryDeadLettered
	}
	next := time.Now().UTC().Add(time.Duration(1<<min(job.Attempts, 10)) * time.Second)
	if retryAfter > 0 {
		next = time.Now().UTC().Add(retryAfter)
	}
	dbCtx := ctx
	if dbCtx == nil || dbCtx.Err() != nil {
		var cancel context.CancelFunc
		dbCtx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
	}
	persistErr := s.store.WithinTx(dbCtx, func(tx pgx.Tx) error {
		var nextValue any = next
		if terminal {
			nextValue = nil
		}
		updateErr := s.store.MarkDeliveryFailed(dbCtx, tx, job.OrganizationID, id, status, responseStatus, nextValue, string(class), code)
		if updateErr != nil {
			return updateErr
		}
		var disabledSubscription string
		var subscriptionVersion int64
		if terminal {
			disabledSubscription, subscriptionVersion, updateErr = s.store.RecordSubscriptionFailure(dbCtx, tx, s.cfg.WebhookDisableAfterFailures, job.OrganizationID, id)
			if updateErr != nil {
				return updateErr
			}
		}
		actor := models.Actor{Tenant: tenant.Context{OrganizationID: job.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "webhook-delivery-worker", CorrelationID: job.CorrelationID}}
		if eventErr := s.recordAndPublish(dbCtx, tx, actor, "webhook.delivery_failed", "webhook.delivery_failed.v1", "webhook_delivery", id, 1, map[string]any{"status": status, "eventId": job.CausationID, "attempt": job.Attempts}); eventErr != nil {
			return eventErr
		}
		if disabledSubscription != "" {
			return s.recordAndPublish(dbCtx, tx, actor, "webhook.disabled", "webhook.disabled.v1", "webhook_subscription", disabledSubscription, subscriptionVersion, map[string]any{"status": models.WebhookStatusDisabled})
		}
		return nil
	})
	if persistErr != nil {
		return &jobs.ClassifiedError{Class: jobs.ErrorClassDependencyUnavailable, Code: "WEBHOOK_DELIVERY_STATE_PERSIST_FAILED", SafeMessage: "webhook delivery state could not be persisted", Err: persistErr}
	}
	if s.metrics != nil {
		s.metrics.IncCounter("fleet_partner_webhook_deliveries_total", map[string]string{"status": strings.ToLower(status), "failure_class": strings.ToLower(string(class))}, 1)
		if !terminal {
			s.metrics.IncCounter("fleet_partner_webhook_delivery_retries_total", map[string]string{"failure_class": strings.ToLower(string(class))}, 1)
		}
		s.refreshWebhookMetrics(dbCtx)
	}
	return &jobs.ClassifiedError{Class: class, Code: code, SafeMessage: "webhook delivery failed", RetryAfter: retryAfter, Err: cause}
}

// refreshWebhookMetrics exports only fleet-wide bounded aggregates. Tenant,
// client, subscription, event, and destination URL never become labels.
func (s *Service) refreshWebhookMetrics(ctx context.Context) {
	if s == nil || s.metrics == nil || s.store == nil || !s.store.Configured() || ctx == nil || ctx.Err() != nil {
		return
	}
	backlog, oldest, deadLetters, disabled, err := s.store.WebhookAggregates(ctx)
	if err != nil {
		return
	}
	oldestAge := time.Duration(0)
	if oldest != nil {
		oldestAge = time.Since(oldest.UTC())
		if oldestAge < 0 {
			oldestAge = 0
		}
	}
	s.metrics.SetGauge("fleet_partner_webhook_delivery_backlog", nil, float64(backlog))
	s.metrics.SetGauge("fleet_partner_webhook_oldest_delivery_age_seconds", nil, oldestAge.Seconds())
	s.metrics.SetGauge("fleet_partner_webhook_dead_letter_count", nil, float64(deadLetters))
	s.metrics.SetGauge("fleet_partner_webhook_disabled_subscriptions", nil, float64(disabled))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
