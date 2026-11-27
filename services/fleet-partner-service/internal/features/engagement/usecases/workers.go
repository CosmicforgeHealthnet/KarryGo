package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

const (
	jobSupportReconcile   = "SECTION23_SUPPORT_RECONCILE"
	jobSLAGenerate        = "SECTION23_SLA_GENERATE"
	jobQualityFollowup    = "SECTION23_QUALITY_FOLLOWUP"
	jobEscalationSchedule = "SECTION23_ESCALATION_SCHEDULE"
)

// RegisterWorkers attaches the engagement job handlers.
func (s *Service) RegisterWorkers(w *jobs.Worker) {
	if w == nil {
		return
	}
	w.Register(jobSupportReconcile, s.handleSupportReconcile)
	w.Register(jobSLAGenerate, s.handleSLAGenerate)
	w.Register(jobQualityFollowup, s.handleQualityFollowup)
	w.Register(jobEscalationSchedule, s.handleEscalationSchedule)
	w.Register(jobExceptionSLA, s.handleExceptionSLA)
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

// ScheduleMaintenance enqueues the per-organization engagement jobs.
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
		types := []string{jobSupportReconcile, jobSLAGenerate, jobQualityFollowup, jobEscalationSchedule}
		if s.o.Flags.Exceptions {
			types = append(types, jobExceptionSLA)
		}
		for _, typ := range types {
			org, typ := org, typ
			if err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
				_, err := s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: org, JobType: typ, DedupeKey: typ + ":" + bucket, Payload: map[string]string{"organizationId": org}, MaxAttempts: 8})
				return err
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) handleSupportReconcile(ctx context.Context, job jobs.Job) error {
	v, err := jobPayload(job)
	if err != nil {
		return err
	}
	if s.o.Support == nil || !s.o.Support.Configured() {
		return errors.New("support service is not configured")
	}
	cases, err := s.o.Store.SupportCasesNeedingReconcile(ctx, v["organizationId"])
	if err != nil {
		return err
	}
	for _, caseID := range cases {
		p, err := s.o.Support.Read(ctx, v["organizationId"], caseID)
		if err != nil {
			return err
		}
		if err := s.o.Store.ReconcileSupportCase(ctx, v["organizationId"], caseID, p); err != nil {
			return err
		}
	}
	return s.reconcileSupportMessages(ctx, v["organizationId"])
}

func (s *Service) handleSLAGenerate(ctx context.Context, job jobs.Job) error {
	v, err := jobPayload(job)
	if err != nil {
		return err
	}
	items, err := s.o.Store.LatestSLAObservations(ctx, v["organizationId"])
	if err != nil {
		return err
	}
	for _, x := range items {
		inputs, _ := json.Marshal(map[string]any{"sourceType": x.SourceType, "sourceReference": x.SourceReference})
		versions, _ := json.Marshal(map[string]any{"sourceVersion": x.SourceVersion})
		if _, err := s.RecordMeasurement(ctx, models.MeasurementInput{OrganizationID: v["organizationId"], AgreementReference: x.Agreement, ContractID: x.Contract, CustomerID: x.Customer, MetricCode: x.Metric, FormulaVersion: x.FormulaVersion, PeriodStart: x.Start, PeriodEnd: x.End, TargetValue: x.Target, MeasuredValue: x.Measured, Unit: x.Unit, Complete: x.Complete(), SourceInputs: inputs, SourceVersions: versions}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) handleQualityFollowup(ctx context.Context, job jobs.Job) error {
	v, err := jobPayload(job)
	if err != nil {
		return err
	}
	overdue, err := s.o.Store.OverdueSevereQualityCases(ctx, v["organizationId"])
	if err != nil {
		return err
	}
	t := tenant.Context{OrganizationID: v["organizationId"], ActorType: tenant.ActorTypeSystem, ActorID: "section23-quality-worker"}
	for _, id := range overdue {
		if _, err := s.CreateEscalation(ctx, t, models.EscalationInput{SourceType: "QUALITY_CASE", SourceReference: id, Level: 1, RecipientType: "ORGANIZATION_ROLE", RecipientReference: "OPERATIONS", DueAt: s.o.Now().UTC()}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) handleEscalationSchedule(ctx context.Context, job jobs.Job) error {
	v, err := jobPayload(job)
	if err != nil {
		return err
	}
	org := v["organizationId"]
	due, err := s.o.Store.DueEscalations(ctx, org)
	if err != nil {
		return err
	}
	if len(due) > 0 && s.o.Notifications == nil {
		return errors.New("notification intents are not configured")
	}
	for _, x := range due {
		x := x
		err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
			payload, _ := json.Marshal(map[string]any{"escalationId": x.ID, "level": "due"})
			intentID, err := s.o.Notifications.QueueEscalationNotice(ctx, tx, EscalationNotice{OrganizationID: org, EscalationID: x.ID, RecipientType: x.RecipientType, RecipientReference: x.RecipientReference, Channels: json.RawMessage(`["IN_APP"]`), SafePayload: payload, IdempotencyKey: "escalation:" + x.ID + ":" + fmt.Sprint(x.Version)})
			if err != nil {
				return err
			}
			return s.o.Store.MarkEscalationNotified(ctx, tx, org, x.ID, intentID)
		})
		if err != nil {
			return err
		}
	}
	return nil
}
