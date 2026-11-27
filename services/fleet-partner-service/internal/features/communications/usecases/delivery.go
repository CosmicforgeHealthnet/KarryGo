package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/communications/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

const (
	jobNotificationSubmit    = "SECTION23_NOTIFICATION_SUBMIT"
	jobNotificationReconcile = "SECTION23_NOTIFICATION_RECONCILE"
)

// RegisterWorkers attaches the notification delivery handlers.
func (s *Service) RegisterWorkers(w *jobs.Worker) {
	if w == nil {
		return
	}
	w.Register(jobNotificationSubmit, s.handleNotificationSubmit)
	w.Register(jobNotificationReconcile, s.handleNotificationReconcile)
}

func jobPayload(job jobs.Job) (map[string]string, error) {
	var v map[string]string
	if err := json.Unmarshal(job.Payload, &v); err != nil {
		return nil, err
	}
	if v["organizationId"] == "" || v["organizationId"] != job.OrganizationID {
		return nil, jobs.ErrTenantMismatch
	}
	return v, nil
}

// ScheduleMaintenance enqueues the per-organization notification reconcile.
func (s *Service) ScheduleMaintenance(ctx context.Context) error {
	if !s.o.Flags.Workers || s.o.Flags.KillSwitch {
		return nil
	}
	orgs, err := s.o.Store.ActiveOrganizations(ctx)
	if err != nil {
		return err
	}
	bucket := s.o.Now().UTC().Truncate(time.Minute).Format(time.RFC3339)
	for _, org := range orgs {
		org := org
		if err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
			_, err := s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: org, JobType: jobNotificationReconcile, DedupeKey: jobNotificationReconcile + ":" + bucket, Payload: map[string]string{"organizationId": org}, MaxAttempts: 8})
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) handleNotificationSubmit(ctx context.Context, job jobs.Job) error {
	v, err := jobPayload(job)
	if err != nil {
		return err
	}
	return s.submitIntent(ctx, v["organizationId"], v["intentId"], false)
}

func (s *Service) handleNotificationReconcile(ctx context.Context, job jobs.Job) error {
	v, err := jobPayload(job)
	if err != nil {
		return err
	}
	due, err := s.o.Store.DueIntents(ctx, v["organizationId"])
	if err != nil {
		return err
	}
	for _, id := range due {
		if err := s.submitIntent(ctx, v["organizationId"], id, true); err != nil {
			return err
		}
	}
	return nil
}

// submitIntent hands one intent to notification-service. An ambiguous submit
// is read back by its idempotency key rather than assumed failed.
func (s *Service) submitIntent(ctx context.Context, org, id string, readFirst bool) error {
	if s.o.Notification == nil || !s.o.Notification.Configured() {
		return errors.New("notification service is not configured")
	}
	intent, err := s.o.Store.GetIntent(ctx, org, id)
	if err != nil {
		return err
	}
	if intent.Closed() {
		return nil
	}
	start := s.o.Now().UTC()
	if start.Sub(intent.CreatedAt) > s.o.IntentMaxAge {
		return s.expireIntent(ctx, org, id, intent, start)
	}
	var channelValues []string
	_ = json.Unmarshal(intent.Channels, &channelValues)
	for i := range channelValues {
		channelValues[i] = strings.ToLower(channelValues[i])
	}
	var data map[string]any
	_ = json.Unmarshal(intent.SafePayload, &data)
	key := intent.IdempotencyKey
	payload, _ := json.Marshal(map[string]any{"idempotency_key": key, "source_service": "fleet-partner-service", "event_type": intent.TemplateCode, "recipient": map[string]any{"type": strings.ToLower(intent.RecipientType), "id": intent.RecipientReference}, "channels": channelValues, "template_key": intent.TemplateCode, "data": data, "priority": "normal"})
	var out models.DeliveryOutcome
	if readFirst {
		out, err = s.o.Notification.Read(ctx, org, key)
		if err != nil {
			out, err = s.o.Notification.Submit(ctx, org, key, payload)
		}
	} else {
		out, err = s.o.Notification.Submit(ctx, org, key, payload)
		var ce *platformclients.Error
		if errors.As(err, &ce) && ce.Outcome == platformclients.OutcomeUnknown {
			out, err = s.o.Notification.Read(ctx, org, key)
		}
	}
	outcome := "CONFIRMED"
	newStatus := "ACCEPTED"
	code := ""
	if err != nil {
		outcome = "FAILED"
		newStatus = "FAILED"
		code = "notification_unavailable"
		var ce *platformclients.Error
		if errors.As(err, &ce) && ce.Outcome == platformclients.OutcomeUnknown {
			outcome = "OUTCOME_UNKNOWN"
			newStatus = "OUTCOME_UNKNOWN"
		}
		// Retries are bounded: after the last allowed attempt the intent is
		// dead-lettered (kept, with its attempts, as evidence) instead of
		// being retried every 30 seconds forever.
		if intent.Attempts+1 >= s.o.IntentMaxAttempts {
			newStatus = "DEAD_LETTERED"
			code = "attempts_exhausted"
		}
	}
	txErr := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		return s.o.Store.RecordDeliveryAttempt(ctx, tx, org, id, intent.Attempts+1, outcome, out.Reference, code, start, newStatus)
	})
	if txErr != nil {
		return txErr
	}
	if newStatus == "DEAD_LETTERED" {
		return nil
	}
	return err
}

// expireIntent closes an undelivered intent that is older than IntentMaxAge
// without sending it. An intent that was never submitted is dead-lettered as
// expired straight away. One that may already have reached
// notification-service (an earlier attempt, or an unknown outcome) is read
// back by its idempotency key first: if notification-service holds it, it is
// recorded as accepted; if it authoritatively does not, it is dead-lettered
// as expired. Nothing is deleted and every decision is an attempt row.
func (s *Service) expireIntent(ctx context.Context, org, id string, intent models.Intent, start time.Time) error {
	outcome, status, code, reference := "EXPIRED", "DEAD_LETTERED", "expired", ""
	if intent.Attempts > 0 || intent.Status == "OUTCOME_UNKNOWN" {
		out, err := s.o.Notification.Read(ctx, org, intent.IdempotencyKey)
		var ce *platformclients.Error
		switch {
		case err == nil:
			outcome, status, code, reference = "CONFIRMED", "ACCEPTED", "", out.Reference
		case errors.As(err, &ce) && ce.Outcome == platformclients.OutcomeNotFound:
			// Authoritatively never received: expired.
		default:
			// The read-back itself failed: the outcome stays unknown and is
			// read again later, until the attempt cap closes it.
			outcome, status, code = "OUTCOME_UNKNOWN", "OUTCOME_UNKNOWN", "expired_readback_failed"
			if intent.Attempts+1 >= s.o.IntentMaxAttempts {
				status, code = "DEAD_LETTERED", "expired_unverified"
			}
		}
	}
	return s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		return s.o.Store.RecordDeliveryAttempt(ctx, tx, org, id, intent.Attempts+1, outcome, reference, code, start, status)
	})
}
