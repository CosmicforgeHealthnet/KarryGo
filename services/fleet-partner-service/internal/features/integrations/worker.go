package integrations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

func secureHTTPClient(validator *DestinationValidator, timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, ErrUnsafeDestination
			}
			var ips []net.IP
			if literal := net.ParseIP(host); literal != nil {
				ips = []net.IP{literal}
			} else {
				resolver := validator.Resolver
				if resolver == nil {
					resolver = net.DefaultResolver
				}
				resolved, lookupErr := resolver.LookupIPAddr(ctx, host)
				if lookupErr != nil {
					return nil, ErrUnsafeDestination
				}
				for _, item := range resolved {
					ips = append(ips, item.IP)
				}
			}
			if err = validator.validateIPs(ips); err != nil {
				return nil, err
			}
			for _, ip := range ips {
				conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if dialErr == nil {
					return conn, nil
				}
			}
			return nil, errors.New("integrations: webhook destination connection failed")
		},
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		ExpectContinueTimeout: time.Second,
		MaxIdleConns:          50,
		IdleConnTimeout:       30 * time.Second,
	}
	client := &http.Client{Transport: transport, Timeout: timeout}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("integrations: webhook redirect limit exceeded")
		}
		_, _, err := validator.Validate(req.Context(), req.URL.String())
		return err
	}
	return client
}

// RegisterWorker reuses the Section 27 durable job/lease worker. No separate
// delivery queue or in-memory retry loop is introduced.
func (s *Service) RegisterWorker(worker *jobs.Worker) {
	if s == nil || worker == nil || !s.cfg.WebhookDeliveryWorkerEnabled || s.cfg.IntegrationsKillSwitch {
		return
	}
	worker.Register(JobTypeWebhookDelivery, s.HandleDeliveryJob)
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
	rows, err := s.pool.Query(ctx, `SELECT id,payload FROM fleet_partner_outbox WHERE event_type=ANY($1)
		AND NOT EXISTS(SELECT 1 FROM fleet_partner_webhook_fanout_events f WHERE f.source_event_id=fleet_partner_outbox.id)
		ORDER BY created_at LIMIT $2`, public, limit)
	if err != nil {
		return 0, err
	}
	type pending struct {
		id  string
		raw []byte
	}
	var items []pending
	for rows.Next() {
		var item pending
		if scanErr := rows.Scan(&item.id, &item.raw); scanErr != nil {
			rows.Close()
			return 0, scanErr
		}
		items = append(items, item)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	processed := 0
	for _, item := range items {
		var envelope events.Envelope
		if err = json.Unmarshal(item.raw, &envelope); err != nil {
			return processed, err
		}
		if !events.IsPublicIntegration(envelope.EventType) {
			continue
		}
		if err = events.ValidateEnvelope(envelope); err != nil {
			return processed, fmt.Errorf("integrations: public event contract invalid: %w", err)
		}
		err = database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
			var claimed string
			claimErr := tx.QueryRow(ctx, `INSERT INTO fleet_partner_webhook_fanout_events(source_event_id,organization_id,event_type) VALUES($1,$2,$3) ON CONFLICT DO NOTHING RETURNING source_event_id`, envelope.EventID, envelope.OrganizationID, envelope.EventType).Scan(&claimed)
			if errors.Is(claimErr, pgx.ErrNoRows) {
				return nil
			}
			if claimErr != nil {
				return claimErr
			}
			subs, queryErr := tx.Query(ctx, `SELECT id FROM fleet_partner_webhook_subscriptions WHERE organization_id=$1 AND status='ACTIVE' AND event_types @> ARRAY[$2]::text[]`, envelope.OrganizationID, envelope.EventType)
			if queryErr != nil {
				return queryErr
			}
			var subscriptionIDs []string
			for subs.Next() {
				var subID string
				if scanErr := subs.Scan(&subID); scanErr != nil {
					subs.Close()
					return scanErr
				}
				subscriptionIDs = append(subscriptionIDs, subID)
			}
			rowsErr := subs.Err()
			subs.Close()
			if rowsErr != nil {
				return rowsErr
			}
			// pgx transactions use one connection: consume and close the
			// subscription result set before issuing inserts on that connection.
			for _, subID := range subscriptionIDs {
				deliveryID := ids.New("whd")
				var inserted string
				insertErr := tx.QueryRow(ctx, `INSERT INTO fleet_partner_webhook_deliveries(id,organization_id,subscription_id,source_event_id,event_type,attempt_no,status,scheduled_at,correlation_id) VALUES($1,$2,$3,$4,$5,1,'PENDING',now(),$6) ON CONFLICT(subscription_id,source_event_id,attempt_no) DO NOTHING RETURNING id`, deliveryID, envelope.OrganizationID, subID, envelope.EventID, envelope.EventType, envelope.CorrelationID).Scan(&inserted)
				if errors.Is(insertErr, pgx.ErrNoRows) {
					continue
				}
				if insertErr != nil {
					return insertErr
				}
				jobID, enqueueErr := s.queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: envelope.OrganizationID, JobType: JobTypeWebhookDelivery, DedupeKey: "webhook:" + deliveryID, Payload: map[string]any{"organizationId": envelope.OrganizationID, "deliveryId": deliveryID}, CorrelationID: envelope.CorrelationID, CausationID: envelope.EventID, MaxAttempts: s.cfg.WebhookDeliveryMaxAttempts})
				if enqueueErr != nil {
					return enqueueErr
				}
				if _, updateErr := tx.Exec(ctx, `UPDATE fleet_partner_webhook_deliveries SET job_id=$1 WHERE id=$2`, jobID, deliveryID); updateErr != nil {
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
	var subID, eventID, eventType, endpoint, correlation, status string
	var currentCipher, previousCipher, rawEvent []byte
	var currentVersion int
	var previousVersion *int
	var previousUntil *time.Time
	err := s.pool.QueryRow(ctx, `SELECT d.subscription_id,d.source_event_id,d.event_type,d.correlation_id,d.status,
		s.endpoint_url,s.secret_ciphertext,s.secret_version,s.previous_secret_ciphertext,s.previous_secret_version,s.previous_secret_valid_until,o.payload
		FROM fleet_partner_webhook_deliveries d JOIN fleet_partner_webhook_subscriptions s ON s.id=d.subscription_id AND s.organization_id=d.organization_id
		JOIN fleet_partner_outbox o ON o.id=d.source_event_id WHERE d.organization_id=$1 AND d.id=$2 AND s.status='ACTIVE'`, job.OrganizationID, payload.DeliveryID).Scan(&subID, &eventID, &eventType, &correlation, &status, &endpoint, &currentCipher, &currentVersion, &previousCipher, &previousVersion, &previousUntil, &rawEvent)
	if err != nil {
		return &jobs.ClassifiedError{Class: jobs.ErrorClassPermanent, Code: "webhook_delivery_not_found", SafeMessage: "webhook delivery is unavailable", Err: err}
	}
	if status == DeliverySucceeded {
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
	body, err := json.Marshal(PublicWebhookEnvelope{EventID: eventID, EventType: eventType, SchemaVersion: envelope.SchemaVersion, OrganizationID: job.OrganizationID, CorrelationID: correlation, OccurredAt: envelope.OccurredAt, Data: data})
	if err != nil {
		return err
	}
	if err = validateWebhookPayload(body); err != nil {
		return s.deliveryFailure(ctx, job, payload.DeliveryID, "PAYLOAD_MINIMIZATION_FAILED", jobs.ErrorClassPermanent, 0, 0, err)
	}
	currentSecret, err := decryptSecret(s.cfg.WebhookSecretEncryptionKey, currentCipher)
	if err != nil {
		return s.deliveryFailure(ctx, job, payload.DeliveryID, "SECRET_DECRYPTION_FAILED", jobs.ErrorClassPermanent, 0, 0, err)
	}
	secrets := []signingSecret{{Version: currentVersion, Secret: currentSecret}}
	if len(previousCipher) > 0 && previousVersion != nil && previousUntil != nil && previousUntil.After(time.Now().UTC()) {
		previousSecret, decryptErr := decryptSecret(s.cfg.WebhookSecretEncryptionKey, previousCipher)
		if decryptErr != nil {
			return s.deliveryFailure(ctx, job, payload.DeliveryID, "SECRET_DECRYPTION_FAILED", jobs.ErrorClassPermanent, 0, 0, decryptErr)
		}
		secrets = append(secrets, signingSecret{Version: *previousVersion, Secret: previousSecret})
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
	request.Header.Set("X-KarryGo-Webhook-Signature", signWebhook(subID, payload.DeliveryID, eventID, timestamp, body, secrets))
	request.Header.Set("Idempotency-Key", payload.DeliveryID)
	request.Header.Set("X-Correlation-ID", correlation)
	_, _ = s.pool.Exec(ctx, `UPDATE fleet_partner_webhook_deliveries SET status='DELIVERING',updated_at=now(),version=version+1 WHERE organization_id=$1 AND id=$2`, job.OrganizationID, payload.DeliveryID)
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
	retryAfter := parseRetryAfter(response.Header.Get("Retry-After"), time.Now().UTC())
	if response.StatusCode == 408 || response.StatusCode == 429 || response.StatusCode == 502 || response.StatusCode == 503 || response.StatusCode == 504 {
		return s.deliveryFailure(ctx, job, payload.DeliveryID, "HTTP_"+strconv.Itoa(response.StatusCode), jobs.ErrorClassTransient, response.StatusCode, retryAfter, errors.New("transient webhook response"))
	}
	return s.deliveryFailure(ctx, job, payload.DeliveryID, "HTTP_"+strconv.Itoa(response.StatusCode), jobs.ErrorClassPermanent, response.StatusCode, 0, errors.New("permanent webhook response"))
}

func parseRetryAfter(raw string, now time.Time) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
		delay := time.Duration(seconds) * time.Second
		if delay > 24*time.Hour {
			return 24 * time.Hour
		}
		return delay
	}
	if when, err := http.ParseTime(raw); err == nil && when.After(now) {
		delay := when.Sub(now)
		if delay > 24*time.Hour {
			return 24 * time.Hour
		}
		return delay
	}
	return 0
}

func (s *Service) deliverySuccess(ctx context.Context, job jobs.Job, id string, status int, excerpt string) error {
	err := database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		tag, updateErr := tx.Exec(ctx, `UPDATE fleet_partner_webhook_deliveries SET status='SUCCEEDED',response_status=$1,response_body_redacted=$2,delivered_at=now(),next_attempt_at=NULL,error_class=NULL,error_code=NULL,updated_at=now(),version=version+1 WHERE organization_id=$3 AND id=$4`, status, excerpt, job.OrganizationID, id)
		if updateErr != nil || tag.RowsAffected() != 1 {
			return updateErr
		}
		_, _ = tx.Exec(ctx, `UPDATE fleet_partner_webhook_subscriptions SET failure_count=0,updated_at=now() WHERE organization_id=$1 AND id=(SELECT subscription_id FROM fleet_partner_webhook_deliveries WHERE id=$2)`, job.OrganizationID, id)
		actor := Actor{Tenant: tenant.Context{OrganizationID: job.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "webhook-delivery-worker", CorrelationID: job.CorrelationID}}
		return s.recordAndPublish(ctx, tx, actor, "webhook.delivery_succeeded", "webhook.delivery_succeeded.v1", "webhook_delivery", id, 1, map[string]any{"status": DeliverySucceeded, "eventId": job.CausationID, "attempt": job.Attempts})
	})
	if err == nil && s.metrics != nil {
		s.metrics.IncCounter("fleet_partner_webhook_deliveries_total", map[string]string{"status": "succeeded"}, 1)
		s.refreshWebhookMetrics(ctx)
	}
	return err
}

func (s *Service) deliveryFailure(ctx context.Context, job jobs.Job, id, code string, class jobs.ErrorClass, responseStatus int, retryAfter time.Duration, cause error) error {
	terminal := !class.Retryable() || job.Attempts >= job.MaxAttempts
	status := DeliveryRetryScheduled
	if terminal {
		status = DeliveryDeadLettered
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
	persistErr := database.WithinTx(dbCtx, s.pool, func(tx pgx.Tx) error {
		var nextValue any = next
		if terminal {
			nextValue = nil
		}
		_, updateErr := tx.Exec(dbCtx, `UPDATE fleet_partner_webhook_deliveries SET status=$1,response_status=NULLIF($2,0),response_body_redacted='response body omitted',next_attempt_at=$3,error_class=$4,error_code=$5,updated_at=now(),version=version+1 WHERE organization_id=$6 AND id=$7`, status, responseStatus, nextValue, string(class), code, job.OrganizationID, id)
		if updateErr != nil {
			return updateErr
		}
		var disabledSubscription string
		var subscriptionVersion int64
		if terminal {
			updateErr = tx.QueryRow(dbCtx, `UPDATE fleet_partner_webhook_subscriptions SET failure_count=failure_count+1,status=CASE WHEN failure_count+1 >= $1 THEN 'DISABLED' ELSE status END,disabled_at=CASE WHEN failure_count+1 >= $1 THEN COALESCE(disabled_at,now()) ELSE disabled_at END,updated_at=now(),version=version+1 WHERE organization_id=$2 AND id=(SELECT subscription_id FROM fleet_partner_webhook_deliveries WHERE id=$3) RETURNING CASE WHEN status='DISABLED' THEN id ELSE '' END,version`, s.cfg.WebhookDisableAfterFailures, job.OrganizationID, id).Scan(&disabledSubscription, &subscriptionVersion)
			if updateErr != nil {
				return updateErr
			}
		}
		actor := Actor{Tenant: tenant.Context{OrganizationID: job.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "webhook-delivery-worker", CorrelationID: job.CorrelationID}}
		if eventErr := s.recordAndPublish(dbCtx, tx, actor, "webhook.delivery_failed", "webhook.delivery_failed.v1", "webhook_delivery", id, 1, map[string]any{"status": status, "eventId": job.CausationID, "attempt": job.Attempts}); eventErr != nil {
			return eventErr
		}
		if disabledSubscription != "" {
			return s.recordAndPublish(dbCtx, tx, actor, "webhook.disabled", "webhook.disabled.v1", "webhook_subscription", disabledSubscription, subscriptionVersion, map[string]any{"status": WebhookStatusDisabled})
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
	if s == nil || s.metrics == nil || s.pool == nil || ctx == nil || ctx.Err() != nil {
		return
	}
	var backlog, deadLetters, disabled int64
	var oldest *time.Time
	if err := s.pool.QueryRow(ctx, `SELECT
		count(*) FILTER (WHERE status IN ('PENDING','DELIVERING','RETRY_SCHEDULED')),
		min(scheduled_at) FILTER (WHERE status IN ('PENDING','DELIVERING','RETRY_SCHEDULED')),
		count(*) FILTER (WHERE status='DEAD_LETTERED')
		FROM fleet_partner_webhook_deliveries`).Scan(&backlog, &oldest, &deadLetters); err != nil {
		return
	}
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_webhook_subscriptions WHERE status='DISABLED'`).Scan(&disabled); err != nil {
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
