// Package usecases implements the Section 24/54 analytics workflows:
// tenant-scoped reports and dashboards computed from authoritative
// operational tables, explainable rule-based recommendations that only ever
// propose (a human decides), and human-governed automation whose actions
// come from an approved, non-protected catalogue.
package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/analytics/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/scheduler"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Events publishes domain events inside the caller's transaction.
type Events interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}

// Enqueuer schedules durable jobs inside the caller's transaction.
type Enqueuer interface {
	Enqueue(ctx context.Context, q database.Querier, request jobs.EnqueueRequest) (string, error)
}

// ExceptionRaiser opens an operational exception (engagement feature). The
// composition root adapts it; a live exception for the same source is
// returned rather than duplicated, which makes the automation action
// idempotent.
type ExceptionRaiser interface {
	RaiseAutomationException(ctx context.Context, organizationID, executionID, exceptionType, severity, summary string) (string, error)
}

// NoticeQueuer queues a durable notification intent in the caller's
// transaction (communications feature). The idempotency key makes a retried
// execution queue the notice once.
type NoticeQueuer interface {
	QueueAutomationNotice(ctx context.Context, tx pgx.Tx, organizationID, executionID, recipientRole, templateCode, idempotencyKey string, payload json.RawMessage) (string, error)
}

// Options configures the service.
type Options struct {
	Store      Repository
	Events     Events
	Queue      Enqueuer
	Exceptions ExceptionRaiser
	Notices    NoticeQueuer
	Flags      models.Flags
	Now        func() time.Time
	// RunTTL is how long a generated report result is kept; zero means 24h.
	RunTTL time.Duration
	// RecommendationTTL is how long an undecided recommendation stays open;
	// zero means 7 days.
	RecommendationTTL time.Duration
}

// Service implements the analytics workflows.
type Service struct {
	o     Options
	audit *audit.Recorder
}

// NewService builds the service.
func NewService(o Options) *Service {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.RunTTL <= 0 {
		o.RunTTL = 24 * time.Hour
	}
	if o.RecommendationTTL <= 0 {
		o.RecommendationTTL = 7 * 24 * time.Hour
	}
	if o.Queue == nil && o.Store != nil {
		o.Queue = jobs.NewQueue(nil)
	}
	return &Service{o: o, audit: audit.NewRecorder()}
}

func (s *Service) gate(on, write bool) error {
	if s.o.Flags.KillSwitch {
		return apperrors.Unavailable("Fleet integrations are disabled.", nil)
	}
	if !on {
		if write {
			return apperrors.Forbidden("This analytics capability is disabled.", nil)
		}
		return apperrors.NotFound("Resource not found.", nil)
	}
	return nil
}

func (s *Service) record(ctx context.Context, tx pgx.Tx, t tenant.Context, event, kind, id string, v int64, p any) error {
	if err := s.audit.Record(ctx, tx, audit.EntryFromTenant(t, strings.TrimSuffix(event, ".v1"), kind, id)); err != nil {
		return err
	}
	if s.o.Events != nil {
		return s.o.Events.Publish(ctx, tx, event, kind, id, v, t.OrganizationID, t.CorrelationID, p)
	}
	return nil
}

func conflict(code, msg string) error {
	return apperrors.New(http.StatusConflict, apperrors.Code(code), msg, nil)
}

func period(f models.ReportFilters, now time.Time) (time.Time, time.Time) {
	end := now.UTC().Truncate(24 * time.Hour)
	start := end.AddDate(0, 0, -30)
	if f.From != "" {
		start, _ = time.Parse("2006-01-02", f.From)
	}
	if f.To != "" {
		end, _ = time.Parse("2006-01-02", f.To)
	}
	return start, end
}

// branchesFor narrows the permitted branches by an optional branch filter.
// A filter outside the caller's scope is refused, never widened.
func branchesFor(permitted []string, filter string) ([]string, error) {
	if filter == "" {
		return permitted, nil
	}
	if len(permitted) > 0 {
		ok := false
		for _, b := range permitted {
			ok = ok || b == filter
		}
		if !ok {
			return nil, apperrors.Forbidden("The branch filter is outside your scope.", nil)
		}
	}
	return []string{filter}, nil
}

func (s *Service) generate(ctx context.Context, organizationID, reportType string, f models.ReportFilters, permitted []string) (models.ReportResult, error) {
	branches, err := branchesFor(permitted, f.BranchID)
	if err != nil {
		return models.ReportResult{}, err
	}
	from, to := period(f, s.o.Now())
	if to.Before(from) || to.Sub(from) > 366*24*time.Hour {
		return models.ReportResult{}, apperrors.Validation("The report period must be at most one year with from on or before to.", nil)
	}
	sections, sources, err := s.o.Store.GenerateReport(ctx, organizationID, reportType, from, to, branches)
	if err != nil {
		return models.ReportResult{}, err
	}
	return models.ReportResult{ReportType: reportType, GeneratedAt: s.o.Now().UTC(), Filters: models.ReportFilters{BranchID: f.BranchID, From: from.Format("2006-01-02"), To: to.Format("2006-01-02")}, Sections: sections, DataSources: sources}, nil
}

// Dashboards are the synchronous, bounded read models (spec 24: executive
// dashboard, operations/fleet/workforce reports). Each carries its
// generation time so freshness is explicit.
var Dashboards = map[string]string{"executive": "EXECUTIVE", "operations": "OPERATIONS", "finance": "FINANCE", "fleet": "FLEET", "warehouse": "WAREHOUSE", "workforce": "WORKFORCE", "sla": "SLA", "customers": "CUSTOMER"}

// Dashboard computes one dashboard for the caller's branch scope.
func (s *Service) Dashboard(ctx context.Context, t tenant.Context, name string, f models.ReportFilters) (models.ReportResult, error) {
	if err := s.gate(s.o.Flags.Reports, false); err != nil {
		return models.ReportResult{}, err
	}
	reportType, ok := Dashboards[strings.ToLower(name)]
	if !ok {
		return models.ReportResult{}, apperrors.NotFound("Dashboard not found.", nil)
	}
	return s.generate(ctx, t.OrganizationID, reportType, f, t.PermittedBranchIDs)
}

// CreateReportDefinition defines a report, optionally scheduled by cron in
// an IANA timezone. The name is the natural idempotency key.
func (s *Service) CreateReportDefinition(ctx context.Context, t tenant.Context, in models.ReportDefinitionInput) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Reports, true); err != nil {
		return nil, err
	}
	in.Name = strings.TrimSpace(in.Name)
	in.ReportType = strings.ToUpper(strings.TrimSpace(in.ReportType))
	if in.Name == "" || !models.ReportTypes[in.ReportType] {
		return nil, apperrors.Validation("A name and a supported reportType are required.", nil)
	}
	f, err := models.ParseReportFilters(in.Filters)
	if err != nil {
		return nil, apperrors.Validation(err.Error(), nil)
	}
	if _, err = branchesFor(t.PermittedBranchIDs, f.BranchID); err != nil {
		return nil, err
	}
	// A scheduled run has no requester present to capture a scope from, so a
	// branch-restricted user's scheduled report must name its branch.
	if in.ScheduleCron != "" && len(t.PermittedBranchIDs) > 0 && f.BranchID == "" {
		return nil, apperrors.Validation("A scheduled report from a branch-scoped user must set filters.branchId.", nil)
	}
	if id, err := s.o.Store.DefinitionByName(ctx, t.OrganizationID, in.Name); err == nil {
		return s.o.Store.GetDefinition(ctx, t.OrganizationID, id)
	}
	var next *time.Time
	if in.ScheduleTimezone == "" {
		in.ScheduleTimezone = "UTC"
	}
	if in.ScheduleCron != "" {
		loc, err := time.LoadLocation(in.ScheduleTimezone)
		if err != nil {
			return nil, apperrors.Validation("scheduleTimezone must be an IANA name.", nil)
		}
		cron, err := scheduler.ParseCron(in.ScheduleCron)
		if err != nil {
			return nil, apperrors.Validation("scheduleCron is invalid: "+err.Error(), nil)
		}
		n, err := cron.Next(s.o.Now(), loc)
		if err != nil {
			return nil, apperrors.Validation("scheduleCron never fires.", nil)
		}
		next = &n
	}
	filters, _ := json.Marshal(f)
	id := ids.New("rpd")
	err = s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		if err := s.o.Store.InsertDefinition(ctx, tx, id, t.OrganizationID, in, filters, next, t.ActorID); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "report_definition.created.v1", "report_definition", id, 1, map[string]any{"reportDefinitionId": id, "reportType": in.ReportType, "scheduled": next != nil})
	})
	if err != nil {
		return nil, err
	}
	return s.o.Store.GetDefinition(ctx, t.OrganizationID, id)
}

func (s *Service) GetReportDefinition(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Reports, false); err != nil {
		return nil, err
	}
	return s.o.Store.GetDefinition(ctx, t.OrganizationID, id)
}

func (s *Service) ListReportDefinitions(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Reports, false); err != nil {
		return nil, err
	}
	return s.o.Store.ListDefinitions(ctx, t.OrganizationID)
}

// RunReport queues an asynchronous run of a definition with the caller's
// permission snapshot. The idempotency key binds one run per client request.
func (s *Service) RunReport(ctx context.Context, t tenant.Context, definitionID, key string) (map[string]any, error) {
	def, err := s.GetReportDefinition(ctx, t, definitionID)
	if err != nil {
		return nil, err
	}
	if def["status"] != "ACTIVE" {
		return nil, conflict("state_transition_invalid", "Only an active report can run.")
	}
	raw, _ := json.Marshal(def["filters"])
	f, err := models.ParseReportFilters(raw)
	if err != nil {
		return nil, apperrors.Validation(err.Error(), nil)
	}
	if _, err = branchesFor(t.PermittedBranchIDs, f.BranchID); err != nil {
		return nil, err
	}
	return s.queueRun(ctx, t, definitionID, def["report_type"].(string), raw, t.PermittedBranchIDs, "request:"+definitionID+":"+key)
}

func (s *Service) queueRun(ctx context.Context, t tenant.Context, definitionID, reportType string, filters []byte, permitted []string, trigger string) (map[string]any, error) {
	if permitted == nil {
		permitted = []string{}
	}
	snapshot, _ := json.Marshal(models.PermissionSnapshot{ActorID: t.ActorID, PermittedBranchIDs: permitted, CapturedAt: s.o.Now().UTC().Format(time.RFC3339)})
	id := ids.New("rpr")
	err := s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		stored, created, err := s.o.Store.InsertRun(ctx, tx, id, t.OrganizationID, definitionID, reportType, filters, snapshot, trigger, t.ActorID, s.o.Now().Add(s.o.RunTTL))
		if err != nil {
			return err
		}
		id = stored
		if !created {
			return nil
		}
		if _, err = s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: t.OrganizationID, JobType: JobReportGenerate, DedupeKey: "report-run:" + id, Payload: map[string]string{"organizationId": t.OrganizationID, "runId": id}, MaxAttempts: 3}); err != nil {
			return err
		}
		return s.record(ctx, tx, t, "report_run.requested.v1", "report_run", id, 1, map[string]any{"reportRunId": id, "reportType": reportType})
	})
	if err != nil {
		return nil, err
	}
	return s.o.Store.GetRun(ctx, t.OrganizationID, id)
}

// GetReportRun reads a run; an expired run has no result.
func (s *Service) GetReportRun(ctx context.Context, t tenant.Context, id string) (map[string]any, error) {
	if err := s.gate(s.o.Flags.Reports, false); err != nil {
		return nil, err
	}
	run, err := s.o.Store.GetRun(ctx, t.OrganizationID, id)
	if err != nil {
		return nil, err
	}
	if !runVisible(t, run) {
		return nil, apperrors.NotFound("Resource not found.", nil)
	}
	if exp, _ := time.Parse(time.RFC3339Nano, toString(run["expires_at"])); !exp.IsZero() && !exp.After(s.o.Now()) {
		run["result"] = nil
		run["status"] = "EXPIRED"
	}
	return run, nil
}

func toString(v any) string { s, _ := v.(string); return s }

func (s *Service) ListReportRuns(ctx context.Context, t tenant.Context) ([]map[string]any, error) {
	if err := s.gate(s.o.Flags.Reports, false); err != nil {
		return nil, err
	}
	runs, err := s.o.Store.ListRuns(ctx, t.OrganizationID, t.ActorID)
	if err != nil {
		return nil, err
	}
	out := runs[:0]
	for _, r := range runs {
		if runVisible(t, r) {
			out = append(out, r)
		}
	}
	return out, nil
}

// runVisible reports whether a caller may see a run: an unrestricted caller
// sees every run; a branch-scoped caller only runs whose captured scope is a
// non-empty subset of their own branches.
func runVisible(t tenant.Context, run map[string]any) bool {
	if len(t.PermittedBranchIDs) == 0 {
		return true
	}
	snap, _ := run["permission_snapshot"].(map[string]any)
	captured, _ := snap["permittedBranchIds"].([]any)
	if len(captured) == 0 {
		return false
	}
	for _, b := range captured {
		if !t.BranchPermitted(toString(b)) {
			return false
		}
	}
	return true
}

// queueScheduledReports queues one run per due scheduled definition and
// advances its next run. The trigger is the scheduled slot, so a crash or a
// second scheduler never queues the same slot twice.
func (s *Service) queueScheduledReports(ctx context.Context, now time.Time) error {
	return s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		due, err := s.o.Store.DueDefinitions(ctx, tx, now)
		if err != nil {
			return err
		}
		for _, d := range due {
			org, id := toString(d["organization_id"]), toString(d["id"])
			raw, _ := json.Marshal(d["filters"])
			f, err := models.ParseReportFilters(raw)
			if err != nil {
				continue
			}
			permitted := []string{}
			if f.BranchID != "" {
				permitted = []string{f.BranchID}
			}
			slot := toString(d["next_run_at"])
			t := tenant.Context{OrganizationID: org, ActorType: tenant.ActorTypeSystem, ActorID: "report-scheduler", PermittedBranchIDs: permitted}
			runID := ids.New("rpr")
			snapshot, _ := json.Marshal(models.PermissionSnapshot{ActorID: t.ActorID, PermittedBranchIDs: permitted, CapturedAt: now.Format(time.RFC3339)})
			stored, created, err := s.o.Store.InsertRun(ctx, tx, runID, org, id, toString(d["report_type"]), raw, snapshot, "schedule:"+id+":"+slot, t.ActorID, now.Add(s.o.RunTTL))
			if err != nil {
				return err
			}
			if created {
				if _, err = s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: org, JobType: JobReportGenerate, DedupeKey: "report-run:" + stored, Payload: map[string]string{"organizationId": org, "runId": stored}, MaxAttempts: 3}); err != nil {
					return err
				}
			}
			loc, err := time.LoadLocation(toString(d["schedule_timezone"]))
			if err != nil {
				loc = time.UTC
			}
			cron, err := scheduler.ParseCron(toString(d["schedule_cron"]))
			if err != nil {
				continue
			}
			next, err := cron.Next(now, loc)
			if err != nil {
				continue
			}
			if err = s.o.Store.AdvanceSchedule(ctx, tx, id, next); err != nil {
				return err
			}
		}
		return nil
	})
}

// generateRun produces a queued run from its own snapshots: the permitted
// branches captured at request time, never the requester's current access.
func (s *Service) generateRun(ctx context.Context, organizationID, runID string) error {
	reportType, rawFilters, rawPerms, ok, err := s.o.Store.ClaimRun(ctx, organizationID, runID)
	if err != nil || !ok {
		return err
	}
	f, err := models.ParseReportFilters(rawFilters)
	if err != nil {
		return s.failRun(ctx, organizationID, runID, reportType, "invalid_filters")
	}
	var perms models.PermissionSnapshot
	if err = json.Unmarshal(rawPerms, &perms); err != nil {
		return s.failRun(ctx, organizationID, runID, reportType, "invalid_permission_snapshot")
	}
	result, err := s.generate(ctx, organizationID, reportType, f, perms.PermittedBranchIDs)
	if err != nil {
		var ae *apperrors.Error
		if errors.As(err, &ae) {
			return s.failRun(ctx, organizationID, runID, reportType, string(ae.Code))
		}
		return err
	}
	raw, _ := json.Marshal(result)
	t := reportGeneratorTenant(organizationID, runID)
	return s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		changed, err := s.o.Store.CompleteRun(ctx, tx, organizationID, runID, raw, result.RowCount())
		if err != nil || !changed {
			return err
		}
		return s.record(ctx, tx, t, "report_run.ready.v1", "report_run", runID, 2, map[string]any{"reportRunId": runID, "reportType": reportType, "rowCount": result.RowCount()})
	})
}

// failRun marks a run failed and emits report_run.failed.v1 in the same
// transaction (spec 54.4), once.
func (s *Service) failRun(ctx context.Context, organizationID, runID, reportType, code string) error {
	t := reportGeneratorTenant(organizationID, runID)
	return s.o.Store.WithinTx(ctx, func(tx pgx.Tx) error {
		changed, err := s.o.Store.FailRun(ctx, tx, organizationID, runID, code)
		if err != nil || !changed {
			return err
		}
		return s.record(ctx, tx, t, "report_run.failed.v1", "report_run", runID, 2, map[string]any{"reportRunId": runID, "reportType": reportType, "errorCode": code})
	})
}

func reportGeneratorTenant(organizationID, runID string) tenant.Context {
	return tenant.Context{OrganizationID: organizationID, ActorType: tenant.ActorTypeSystem, ActorID: "report-generator", CorrelationID: runID}
}
