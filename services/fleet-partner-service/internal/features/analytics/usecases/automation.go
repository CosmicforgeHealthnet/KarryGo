package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/analytics/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Job types. JobRuleExecute keeps the name Section 27 reserved for it.
const (
	JobReportGenerate = "REPORT_RUN_GENERATE"
	JobAutomationEval = "AUTOMATION_EVALUATE"
	JobRuleExecute    = "AUTOMATION_RULE_EXECUTE"
	JobHousekeeping   = "ANALYTICS_HOUSEKEEPING"
)

// CreateAutomationRule drafts a rule. Only approved triggers, predicates and
// non-protected actions are accepted; arbitrary code is impossible because a
// rule is data interpreted by a fixed evaluator.
func (s *Service) CreateAutomationRule(ctx context.Context, t tenant.Context, in models.AutomationRuleInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Automation, true); err != nil {
		return nil, err
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 200 {
		return nil, apperrors.Validation("A rule name of at most 200 characters is required.", nil)
	}
	if _, _, _, err := models.ValidateRule(in); err != nil {
		return nil, apperrors.Validation(err.Error(), []apperrors.FieldViolation{{Field: "actionPlan", Message: err.Error()}})
	}
	if id, err := s.o.Store.RuleByName(ctx, t.OrganizationID, in.Name); err == nil {
		return s.o.Store.GetRule(ctx, t.OrganizationID, id)
	}
	id := ids.New("aur")
	err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		if err := s.o.Store.InsertRule(ctx, tx, id, t.OrganizationID, in, t.ActorID); err != nil {
			return err
		}
		if err := s.o.Store.InsertRuleVersion(ctx, tx, t.OrganizationID, id, 1, "DRAFT", t.ActorID); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "automation_rule.created.v1", "automation_rule", id, 1, map[string]any{"ruleId": id, "status": "DRAFT"})
	})
	if err != nil {
		return nil, err
	}
	return s.o.Store.GetRule(ctx, t.OrganizationID, id)
}

func (s *Service) GetAutomationRule(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Automation, false); err != nil {
		return nil, err
	}
	return s.o.Store.GetRule(ctx, t.OrganizationID, id)
}

func (s *Service) ListAutomationRules(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Automation, false); err != nil {
		return nil, err
	}
	return s.o.Store.ListRules(ctx, t.OrganizationID)
}

func (s *Service) ListAutomationExecutions(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Automation, false); err != nil {
		return nil, err
	}
	return s.o.Store.ListExecutions(ctx, t.OrganizationID)
}

// AutomationRuleCommand runs UPDATE (draft only), SUBMIT, APPROVE, DISABLE.
// Approval must come from someone other than the submitter (segregation of
// duties) and is the only way a rule becomes ACTIVE; any edit returns it to
// DRAFT and needs approval again.
func (s *Service) AutomationRuleCommand(ctx context.Context, t tenant.Context, id, command string, in models.AutomationRuleInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Automation, true); err != nil {
		return nil, err
	}
	if in.ExpectedVersion <= 0 {
		return nil, apperrors.Validation("expectedVersion is required.", nil)
	}
	err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		status, version, _, submitter, err := s.o.Store.LockRule(ctx, tx, t.OrganizationID, id)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.NotFound("Resource not found.", nil)
		}
		if err != nil {
			return err
		}
		if version != in.ExpectedVersion {
			return conflict("version_conflict", "Automation rule version changed.")
		}
		var next string
		var v int64
		switch command {
		case "UPDATE":
			if status != "DRAFT" && status != "DISABLED" {
				return conflict("state_transition_invalid", "Only a draft or disabled rule can be edited.")
			}
			if _, _, _, err := models.ValidateRule(in); err != nil {
				return apperrors.Validation(err.Error(), nil)
			}
			if _, err = s.o.Store.UpdateRuleDefinition(ctx, tx, t.OrganizationID, id, in); err != nil {
				return err
			}
			next = "DRAFT"
		case "SUBMIT":
			if status != "DRAFT" && status != "DISABLED" {
				return conflict("state_transition_invalid", "Only a draft or disabled rule can be submitted.")
			}
			next = "PENDING_APPROVAL"
		case "APPROVE":
			if status != "PENDING_APPROVAL" {
				return conflict("approval_required", "The rule must be submitted for approval first.")
			}
			if submitter == t.ActorID {
				return apperrors.New(403, "segregation_of_duties_violation", "The submitter cannot approve their own automation rule.", nil)
			}
			next = "ACTIVE"
		case "DISABLE":
			if status == "DISABLED" {
				return conflict("state_transition_invalid", "The rule is already disabled.")
			}
			next = "DISABLED"
		default:
			return apperrors.BadRequest("Unsupported automation command.", nil)
		}
		if v, err = s.o.Store.SetRuleStatus(ctx, tx, t.OrganizationID, id, next, t.ActorID); err != nil {
			return err
		}
		if err = s.o.Store.InsertRuleVersion(ctx, tx, t.OrganizationID, id, v, next, t.ActorID); err != nil {
			return err
		}
		event := map[string]string{"UPDATE": "automation_rule.updated.v1", "SUBMIT": "automation_rule.approval_required.v1", "APPROVE": "automation_rule.activated.v1", "DISABLE": "automation_rule.disabled.v1"}[command]
		return s.record(ctx, tx, t, event, "automation_rule", id, v, map[string]any{"ruleId": id, "status": next})
	})
	if err != nil {
		return nil, err
	}
	return s.o.Store.GetRule(ctx, t.OrganizationID, id)
}

type ruleRow struct {
	ID            string               `json:"id"`
	Version       int64                `json:"version"`
	TriggerConfig models.TriggerConfig `json:"triggerConfig"`
	ConditionTree models.ConditionTree `json:"conditionTree"`
	ActionPlan    json.RawMessage      `json:"actionPlan"`
}

// evaluate reads the organization's committed events after its cursor and
// records one execution per (rule, event). Matching executions are queued
// for the durable executor; non-matching ones are recorded as SKIPPED with
// their trace. The cursor and executions commit together, so a crash
// replays the same events and the (rule, trigger) uniqueness makes the
// replay a no-op.
func (s *Service) evaluate(ctx context.Context, organizationID string) error {
	rawRules, err := s.o.Store.ActiveEventRules(ctx, organizationID)
	if err != nil || len(rawRules) == 0 {
		return err
	}
	var rules []ruleRow
	types := map[string]bool{}
	for _, raw := range rawRules {
		var r ruleRow
		b, _ := json.Marshal(raw)
		if err := json.Unmarshal(b, &r); err != nil {
			return err
		}
		rules = append(rules, r)
		types[r.TriggerConfig.EventType] = true
	}
	var typeList []string
	for k := range types {
		typeList = append(typeList, k)
	}
	t := tenant.Context{OrganizationID: organizationID, ActorType: tenant.ActorTypeSystem, ActorID: "automation-evaluator"}
	return s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		at, after, err := s.o.Store.Cursor(ctx, tx, organizationID, s.o.Now().UTC())
		if err != nil {
			return err
		}
		evts, err := s.o.Store.OutboxAfter(ctx, organizationID, at, after, typeList, 200)
		if err != nil || len(evts) == 0 {
			return err
		}
		for _, e := range evts {
			// Never evaluate an event an automation action caused: a rule
			// that raises an exception on exception.opened would otherwise
			// feed itself forever.
			if src, _ := e.Payload["sourceType"].(string); src == "AUTOMATION_EXECUTION" {
				continue
			}
			for _, r := range rules {
				if r.TriggerConfig.EventType != e.Type {
					continue
				}
				matched, trace := models.Evaluate(r.ConditionTree, e.Payload)
				status := "SKIPPED"
				if matched {
					status = "STARTED"
				}
				execID := ids.New("aex")
				traceJSON, _ := json.Marshal(map[string]any{"eventId": e.ID, "eventType": e.Type, "matched": matched, "conditions": trace})
				created, err := s.o.Store.InsertExecution(ctx, tx, execID, organizationID, r.ID, r.Version, e.ID, traceJSON, status)
				if err != nil {
					return err
				}
				if !created || !matched {
					continue
				}
				if _, err = s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: organizationID, JobType: JobRuleExecute, DedupeKey: "automation-exec:" + execID, Payload: map[string]string{"organizationId": organizationID, "executionId": execID}, MaxAttempts: 5}); err != nil {
					return err
				}
				t.CorrelationID = e.ID
				if err = s.record(ctx, tx, t, "automation_execution.started.v1", "automation_execution", execID, 1, map[string]any{"executionId": execID, "ruleId": r.ID, "eventId": e.ID}); err != nil {
					return err
				}
			}
		}
		last := evts[len(evts)-1]
		return s.o.Store.AdvanceCursor(ctx, tx, organizationID, last.CreatedAt, last.ID)
	})
}

// execute runs one STARTED execution's actions. The plan comes from the
// rule version the execution was evaluated against, and every action is
// re-checked against the approved catalogue before it runs.
func (s *Service) execute(ctx context.Context, organizationID, executionID string) error {
	var results []map[string]any
	var actions models.ActionPlan
	var raiseQueue []models.Action
	err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		status, _, _, plan, _, err := s.o.Store.ExecutionForRun(ctx, tx, organizationID, executionID)
		if errors.Is(err, pgx.ErrNoRows) || status != "STARTED" {
			return nil
		}
		if err != nil {
			return err
		}
		if err = json.Unmarshal(plan, &actions); err != nil {
			return err
		}
		for i, a := range actions.Actions {
			if !models.AutomationActions[a.Type] || models.ValidateActionParams(a) != nil {
				results = append(results, map[string]any{"action": a.Type, "status": "REFUSED", "reason": "not in the approved action catalogue"})
				continue
			}
			switch a.Type {
			case "QUEUE_NOTIFICATION":
				if s.o.Notices == nil {
					return errors.New("notification intents are not configured")
				}
				role, _ := a.Params["recipientRole"].(string)
				template, _ := a.Params["templateCode"].(string)
				intentID, err := s.o.Notices.QueueAutomationNotice(ctx, tx, organizationID, executionID, role, template, fmt.Sprintf("automation:%s:%d", executionID, i), encode(map[string]any{"executionId": executionID}))
				if err != nil {
					return err
				}
				results = append(results, map[string]any{"action": a.Type, "status": "QUEUED", "notificationIntentId": intentID})
			case "RAISE_EXCEPTION":
				raiseQueue = append(raiseQueue, a)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Exceptions are raised through the engagement feature's own
	// transaction; the exception's source is this execution, so a retry
	// returns the same live exception instead of opening another.
	for _, a := range raiseQueue {
		if s.o.Exceptions == nil {
			return errors.New("operational exceptions are not configured")
		}
		p := func(k string) string { v, _ := a.Params[k].(string); return v }
		exceptionID, err := s.o.Exceptions.RaiseAutomationException(ctx, organizationID, executionID, p("exceptionType"), p("severity"), p("summary"))
		if err != nil {
			return err
		}
		results = append(results, map[string]any{"action": a.Type, "status": "COMPLETED", "exceptionId": exceptionID})
	}
	final := "COMPLETED"
	code := ""
	for _, r := range results {
		if r["status"] == "REFUSED" {
			final, code = "FAILED", "action_not_permitted"
		}
	}
	raw, _ := json.Marshal(results)
	t := tenant.Context{OrganizationID: organizationID, ActorType: tenant.ActorTypeSystem, ActorID: "automation-executor", CorrelationID: executionID}
	return s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		if err := s.o.Store.FinishExecution(ctx, tx, organizationID, executionID, final, raw, code); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "automation_execution."+strings.ToLower(final)+".v1", "automation_execution", executionID, 2, map[string]any{"executionId": executionID, "status": final})
	})
}

// RegisterWorkers attaches the analytics job handlers.
func (s *Service) RegisterWorkers(w *jobs.Worker) {
	if w == nil {
		return
	}
	w.Register(JobReportGenerate, s.handleReportGenerate)
	w.Register(JobAutomationEval, s.handleEvaluate)
	w.Register(JobRuleExecute, s.handleExecute)
	w.Register(JobHousekeeping, s.handleHousekeeping)
}

func payload(job jobs.Job) (map[string]string, error) {
	var v map[string]string
	if err := json.Unmarshal(job.Payload, &v); err != nil {
		return nil, err
	}
	if v["organizationId"] == "" || v["organizationId"] != job.OrganizationID {
		return nil, jobs.ErrTenantMismatch
	}
	return v, nil
}

func (s *Service) handleReportGenerate(ctx context.Context, job jobs.Job) error {
	v, err := payload(job)
	if err != nil {
		return err
	}
	return s.generateRun(ctx, v["organizationId"], v["runId"])
}

func (s *Service) handleEvaluate(ctx context.Context, job jobs.Job) error {
	v, err := payload(job)
	if err != nil {
		return err
	}
	if !s.o.Flags.Automation {
		return nil
	}
	return s.evaluate(ctx, v["organizationId"])
}

func (s *Service) handleExecute(ctx context.Context, job jobs.Job) error {
	v, err := payload(job)
	if err != nil {
		return err
	}
	return s.execute(ctx, v["organizationId"], v["executionId"])
}

// handleHousekeeping expires old report results and undecided
// recommendations, and refreshes recommendations from the models.
func (s *Service) handleHousekeeping(ctx context.Context, job jobs.Job) error {
	v, err := payload(job)
	if err != nil {
		return err
	}
	org := v["organizationId"]
	now := s.o.Now()
	if _, err = s.o.Store.ExpireRuns(ctx, org, now); err != nil {
		return err
	}
	if _, err = s.o.Store.ExpireRecommendations(ctx, org, now); err != nil {
		return err
	}
	if !s.o.Flags.Recommendations {
		return nil
	}
	_, err = s.generateFor(ctx, tenant.Context{OrganizationID: org, ActorType: tenant.ActorTypeSystem, ActorID: "recommendation-models", CorrelationID: job.ID}, map[string]bool{"PREVENTIVE_MAINTENANCE": true, "ANOMALY_REVIEW": true, "DELAY_RISK": true})
	return err
}

// ScheduleMaintenance queues scheduled report runs that are due and the
// per-organization evaluation and housekeeping jobs. Run, evaluation and
// housekeeping jobs dedupe per time bucket, so overlapping schedulers never
// double-queue.
func (s *Service) ScheduleMaintenance(ctx context.Context, organizations []string) error {
	if !s.o.Flags.Workers || s.o.Flags.KillSwitch {
		return nil
	}
	now := s.o.Now().UTC()
	if s.o.Flags.Reports {
		if err := s.queueScheduledReports(ctx, now); err != nil {
			return err
		}
	}
	bucket := now.Truncate(time.Minute).Format(time.RFC3339)
	hour := now.Truncate(time.Hour).Format(time.RFC3339)
	more, err := s.o.Store.OrganizationsWithAnalytics(ctx)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, org := range append(organizations, more...) {
		if seen[org] {
			continue
		}
		seen[org] = true
		org := org
		err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
			if s.o.Flags.Automation {
				if _, err := s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: org, JobType: JobAutomationEval, DedupeKey: JobAutomationEval + ":" + org + ":" + bucket, Payload: map[string]string{"organizationId": org}, MaxAttempts: 5}); err != nil {
					return err
				}
			}
			_, err := s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: org, JobType: JobHousekeeping, DedupeKey: JobHousekeeping + ":" + org + ":" + hour, Payload: map[string]string{"organizationId": org}, MaxAttempts: 5})
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}
