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
	if intent.Delivered() {
		return nil
	}
	start := s.o.Now().UTC()
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
	}
	txErr := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		return s.o.Store.RecordDeliveryAttempt(ctx, tx, org, id, intent.Attempts+1, outcome, out.Reference, code, start, newStatus)
	})
	if txErr != nil {
		return txErr
	}
	return err
}
