package engagement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

const (
	jobNotificationSubmit    = "SECTION23_NOTIFICATION_SUBMIT"
	jobNotificationReconcile = "SECTION23_NOTIFICATION_RECONCILE"
	jobSupportReconcile      = "SECTION23_SUPPORT_RECONCILE"
	jobSLAGenerate           = "SECTION23_SLA_GENERATE"
	jobQualityFollowup       = "SECTION23_QUALITY_FOLLOWUP"
	jobEscalationSchedule    = "SECTION23_ESCALATION_SCHEDULE"
)

func (s *Service) RegisterWorkers(w *jobs.Worker) {
	if w == nil {
		return
	}
	w.Register(jobNotificationSubmit, s.handleNotificationSubmit)
	w.Register(jobNotificationReconcile, s.handleNotificationReconcile)
	w.Register(jobSupportReconcile, s.handleSupportReconcile)
	w.Register(jobSLAGenerate, s.handleSLAGenerate)
	w.Register(jobQualityFollowup, s.handleQualityFollowup)
	w.Register(jobEscalationSchedule, s.handleEscalationSchedule)
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
func (s *Service) ScheduleMaintenance(ctx context.Context) error {
	if !s.o.Flags.Workers || s.o.Flags.KillSwitch {
		return nil
	}
	rows, err := s.o.Pool.Query(ctx, `SELECT id FROM fleet_partner_organizations WHERE status='ACTIVE' ORDER BY id`)
	if err != nil {
		return err
	}
	var orgs []string
	for rows.Next() {
		var org string
		if err := rows.Scan(&org); err != nil {
			rows.Close()
			return err
		}
		orgs = append(orgs, org)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	bucket := s.o.Now().UTC().Truncate(time.Minute).Format(time.RFC3339)
	for _, org := range orgs {
		for _, typ := range []string{jobNotificationReconcile, jobSupportReconcile, jobSLAGenerate, jobQualityFollowup, jobEscalationSchedule} {
			if _, err := s.o.Queue.Enqueue(ctx, s.o.Pool, jobs.EnqueueRequest{OrganizationID: org, JobType: typ, DedupeKey: typ + ":" + bucket, Payload: map[string]string{"organizationId": org}, MaxAttempts: 8}); err != nil {
				return err
			}
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
	rows, err := s.o.Pool.Query(ctx, `SELECT id FROM fleet_partner_notification_intents WHERE organization_id=$1 AND status IN('OUTCOME_UNKNOWN','FAILED') AND next_attempt_at<=now() ORDER BY next_attempt_at LIMIT 50`, v["organizationId"])
	if err != nil {
		return err
	}
	var idsList []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		idsList = append(idsList, id)
	}
	rows.Close()
	for _, id := range idsList {
		if err := s.submitIntent(ctx, v["organizationId"], id, true); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) submitIntent(ctx context.Context, org, id string, readFirst bool) error {
	if s.o.Notification == nil || !s.o.Notification.Configured() {
		return errors.New("notification service is not configured")
	}
	var key, status, recipientType, recipientReference, templateCode string
	var safePayload, channels []byte
	var attempts int
	if err := s.o.Pool.QueryRow(ctx, `SELECT idempotency_key,status,recipient_type,recipient_reference,template_code,safe_payload,channels,attempts FROM fleet_partner_notification_intents WHERE organization_id=$1 AND id=$2`, org, id).Scan(&key, &status, &recipientType, &recipientReference, &templateCode, &safePayload, &channels, &attempts); err != nil {
		return err
	}
	if status == "ACCEPTED" || status == "DELIVERED" || status == "NOT_APPLICABLE" {
		return nil
	}
	start := s.o.Now().UTC()
	var channelValues []string
	_ = json.Unmarshal(channels, &channelValues)
	for i := range channelValues {
		channelValues[i] = strings.ToLower(channelValues[i])
	}
	var data map[string]any
	_ = json.Unmarshal(safePayload, &data)
	payload, _ := json.Marshal(map[string]any{"idempotency_key": key, "source_service": "fleet-partner-service", "event_type": templateCode, "recipient": map[string]any{"type": strings.ToLower(recipientType), "id": recipientReference}, "channels": channelValues, "template_key": templateCode, "data": data, "priority": "normal"})
	var out DeliveryOutcome
	var err error
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
	txErr := s.tx(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, `INSERT INTO fleet_partner_notification_delivery_attempts(id,organization_id,intent_id,attempt_number,outcome,operation_reference,error_code,started_at,completed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,now())`, ids.New("nda"), org, id, attempts+1, outcome, out.Reference, code, start); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, `UPDATE fleet_partner_notification_intents SET status=$3,delivery_operation_reference=$4,attempts=attempts+1,last_error_code=$5,next_attempt_at=CASE WHEN $3 IN('FAILED','OUTCOME_UNKNOWN') THEN now()+interval '30 seconds' ELSE next_attempt_at END,updated_at=now() WHERE organization_id=$1 AND id=$2`, org, id, newStatus, out.Reference, code)
		return e
	})
	if txErr != nil {
		return txErr
	}
	return err
}

func (s *Service) handleSupportReconcile(ctx context.Context, job jobs.Job) error {
	v, err := jobPayload(job)
	if err != nil {
		return err
	}
	if s.o.Support == nil || !s.o.Support.Configured() {
		return errors.New("support service is not configured")
	}
	rows, err := s.o.Pool.Query(ctx, `SELECT support_service_case_id FROM fleet_partner_support_case_projections WHERE organization_id=$1 AND gap_status IN('GAP_DETECTED','RECONCILING','UNKNOWN') ORDER BY updated_at LIMIT 50`, v["organizationId"])
	if err != nil {
		return err
	}
	var cases []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		cases = append(cases, id)
	}
	rows.Close()
	for _, caseID := range cases {
		p, err := s.o.Support.Read(ctx, v["organizationId"], caseID)
		if err != nil {
			return err
		}
		if _, err := s.o.Pool.Exec(ctx, `UPDATE fleet_partner_support_case_projections SET status=$3,priority=$4,owner_reference=$5,source_version=$6,last_synced_at=now(),gap_status='CURRENT',version=version+1,updated_at=now() WHERE organization_id=$1 AND support_service_case_id=$2 AND source_version<=$6`, v["organizationId"], caseID, p.Status, p.Priority, p.OwnerReference, p.SourceVersion); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) handleSLAGenerate(ctx context.Context, job jobs.Job) error {
	v, err := jobPayload(job)
	if err != nil {
		return err
	}
	rows, err := s.o.Pool.Query(ctx, `SELECT a.id,a.contract_id,a.customer_id,a.metric_code,a.target_value::text,a.measurement_unit,a.version,o.window_start,o.window_end,COALESCE(o.measured_value::text,''),o.outcome,o.source_type,o.source_reference,o.source_version FROM fleet_partner_service_level_agreements a JOIN LATERAL(SELECT * FROM fleet_partner_sla_observations x WHERE x.organization_id=a.organization_id AND x.sla_id=a.id ORDER BY x.measured_at DESC LIMIT 1)o ON true WHERE a.organization_id=$1 AND a.status='ACTIVE'`, v["organizationId"])
	if err != nil {
		return err
	}
	type item struct {
		agreement, contract, customer, metric, target, unit, measured, outcome, sourceType, sourceRef string
		formula, sourceVersion                                                                        int64
		start, end                                                                                    time.Time
	}
	var items []item
	for rows.Next() {
		var x item
		if err := rows.Scan(&x.agreement, &x.contract, &x.customer, &x.metric, &x.target, &x.unit, &x.formula, &x.start, &x.end, &x.measured, &x.outcome, &x.sourceType, &x.sourceRef, &x.sourceVersion); err != nil {
			rows.Close()
			return err
		}
		items = append(items, x)
	}
	rows.Close()
	for _, x := range items {
		inputs, _ := json.Marshal(map[string]any{"sourceType": x.sourceType, "sourceReference": x.sourceRef})
		versions, _ := json.Marshal(map[string]any{"sourceVersion": x.sourceVersion})
		complete := x.outcome != "UNKNOWN" && x.outcome != "INSUFFICIENT_DATA" && x.outcome != "NOT_EVALUATED" && x.measured != ""
		if _, err := s.RecordMeasurement(ctx, MeasurementInput{OrganizationID: v["organizationId"], AgreementReference: x.agreement, ContractID: x.contract, CustomerID: x.customer, MetricCode: x.metric, FormulaVersion: x.formula, PeriodStart: x.start, PeriodEnd: x.end, TargetValue: x.target, MeasuredValue: x.measured, Unit: x.unit, Complete: complete, SourceInputs: inputs, SourceVersions: versions}); err != nil {
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
	rows, err := s.o.Pool.Query(ctx, `SELECT id FROM fleet_partner_quality_cases WHERE organization_id=$1 AND status IN('OPEN','UNDER_REVIEW','ACTION_REQUIRED') AND severity IN('HIGH','CRITICAL') AND created_at<now()-interval '24 hours' ORDER BY created_at LIMIT 100`, v["organizationId"])
	if err != nil {
		return err
	}
	var idsList []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		idsList = append(idsList, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	t := tenant.Context{OrganizationID: v["organizationId"], ActorType: tenant.ActorTypeSystem, ActorID: "section23-quality-worker"}
	for _, id := range idsList {
		if _, err := s.CreateEscalation(ctx, t, EscalationInput{SourceType: "QUALITY_CASE", SourceReference: id, Level: 1, RecipientType: "ORGANIZATION_ROLE", RecipientReference: "OPERATIONS", DueAt: s.o.Now().UTC()}); err != nil {
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
	rows, err := s.o.Pool.Query(ctx, `SELECT id,recipient_type,recipient_reference,version FROM fleet_partner_escalations WHERE organization_id=$1 AND status='PENDING' AND due_at<=now() ORDER BY due_at LIMIT 50`, org)
	if err != nil {
		return err
	}
	type due struct {
		id, typ, ref string
		version      int64
	}
	var all []due
	for rows.Next() {
		var x due
		if err := rows.Scan(&x.id, &x.typ, &x.ref, &x.version); err != nil {
			rows.Close()
			return err
		}
		all = append(all, x)
	}
	rows.Close()
	for _, x := range all {
		err := s.tx(ctx, func(tx pgx.Tx) error {
			intentID := ids.New("nti")
			payload, _ := json.Marshal(map[string]any{"escalationId": x.id, "level": "due"})
			channels := json.RawMessage(`["IN_APP"]`)
			key := "escalation:" + x.id + ":" + fmt.Sprint(x.version)
			if _, err := tx.Exec(ctx, `INSERT INTO fleet_partner_notification_intents(id,organization_id,source_type,source_reference,recipient_type,recipient_reference,channels,template_code,safe_payload,idempotency_key) VALUES($1,$2,'ESCALATION',$3,$4,$5,$6,'fleet.escalation.due',$7,$8) ON CONFLICT(organization_id,idempotency_key) DO NOTHING`, intentID, org, x.id, x.typ, x.ref, channels, payload, key); err != nil {
				return err
			}
			if _, err := s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: org, JobType: jobNotificationSubmit, DedupeKey: intentID, Payload: map[string]string{"organizationId": org, "intentId": intentID}, MaxAttempts: 8}); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `UPDATE fleet_partner_escalations SET status='NOTIFIED',notification_operation_reference=$3,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 AND status='PENDING'`, org, x.id, intentID)
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}
