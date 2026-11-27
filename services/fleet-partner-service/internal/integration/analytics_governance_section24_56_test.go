package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	analyticsmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/analytics/models"
	analyticsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/analytics/usecases"
	authmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/models"
	authrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/repositories"
	communicationsmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/communications/models"
	communicationsrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/communications/repositories"
	communicationsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/communications/usecases"
	engagementmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/models"
	engagementrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/repositories"
	engagementusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement/usecases"
	governancemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/governance/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

type s24 struct {
	t      *testing.T
	pool   *pgxpool.Pool
	queue  *jobs.Queue
	worker *jobs.Worker
	org    string
}

// runJob enqueues one job and runs the real durable worker until it
// succeeds.
func (h *s24) runJob(jobType string, payload map[string]string) {
	h.t.Helper()
	ctx := context.Background()
	payload["organizationId"] = h.org
	id, err := h.queue.Enqueue(ctx, h.pool, jobs.EnqueueRequest{OrganizationID: h.org, JobType: jobType, DedupeKey: jobType + ":" + ids.New("k"), Payload: payload, MaxAttempts: 2})
	if err != nil {
		h.t.Fatal(err)
	}
	h.waitJob(id)
}

func (h *s24) waitJob(id string) {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() { h.worker.Run(ctx); close(done) }()
	for {
		var status string
		_ = h.pool.QueryRow(context.Background(), `SELECT status FROM fleet_partner_jobs WHERE id=$1`, id).Scan(&status)
		if status == "SUCCEEDED" {
			break
		}
		if status == "DEAD_LETTERED" || ctx.Err() != nil {
			h.t.Fatalf("job %s ended %q", id, status)
		}
		time.Sleep(25 * time.Millisecond)
	}
	cancel()
	<-done
}

// drain runs the worker until no pending job of the types remains.
func (h *s24) drain(types ...string) {
	h.t.Helper()
	for i := 0; i < 40; i++ {
		var pending int
		_ = h.pool.QueryRow(context.Background(), `SELECT count(*) FROM fleet_partner_jobs WHERE organization_id=$1 AND job_type=ANY($2) AND status NOT IN('SUCCEEDED','DEAD_LETTERED','CANCELLED')`, h.org, types).Scan(&pending)
		if pending == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		h.worker.Run(ctx)
		cancel()
	}
	h.t.Fatalf("jobs %v did not drain", types)
}

// TestSection24AnalyticsRecommendationsAutomationAndGovernance is Journey F
// with the Section 56 governance checks:
//   - dashboards and asynchronous report runs read real operational data,
//     carry their generation time, honour the requester's captured branch
//     scope and are invisible across tenants and to narrower scopes;
//   - rule-based recommendations are explainable, deduplicated, decided by
//     a human, and accepting one changes no protected domain;
//   - automation refuses protected actions, needs a different approver,
//     runs once per event (replay-safe), records skipped evaluations and
//     never feeds on its own output;
//   - privacy erasure is blocked by a legal hold and completion needs a
//     report; security events never cross tenants.
func TestSection24AnalyticsRecommendationsAutomationAndGovernance(t *testing.T) {
	pool := testPool(t)
	scope, branchA, _ := section2223Scope(t, pool)
	// An organization-wide caller (the helper returns a branch-scoped one).
	scope.PermittedBranchIDs = nil
	ctx := context.Background()
	org := scope.OrganizationID
	branchB := ids.New("brn")
	if _, err := pool.Exec(ctx, `INSERT INTO fleet_partner_branches(id,organization_id,code,name,timezone)VALUES($1,$2,$3,'Second','Africa/Lagos')`, branchB, org, "B"+branchB[len(branchB)-6:]); err != nil {
		t.Fatal(err)
	}
	queue := jobs.NewQueue(pool)
	publisher := events.NewDomainPublisher(events.NewOutboxRepository(pool))
	comms := communicationsusecases.NewService(communicationsusecases.Options{Store: communicationsrepos.NewRepository(pool), Events: publisher, Queue: queue, Flags: communicationsmodels.Flags{Communications: true, NotificationRules: true}})
	eng := engagementusecases.NewService(engagementusecases.Options{Store: engagementrepos.NewRepository(pool), Events: publisher, Queue: queue, Notifications: app.EscalationNotices(comms), Flags: engagementmodels.Flags{Support: true, Quality: true, SLA: true, Escalations: true, Exceptions: true}})
	cfg := config.Config{AnalyticsReportsEnabled: true, RecommendationsEnabled: true, AutomationEnabled: true, Section24WorkersEnabled: true, PrivacyRequestsEnabled: true, SecurityEventsEnabled: true}
	svc := app.NewAnalyticsService(cfg, pool, publisher, eng, comms)
	gov := app.NewGovernanceService(cfg, pool, publisher, authrepos.NewSessionRepository(pool))
	worker := &jobs.Worker{Queue: queue, Metrics: observability.NewMetrics(), Owner: "s24-worker", PollInterval: 20 * time.Millisecond, LeaseDuration: 5 * time.Second, HeartbeatInterval: time.Second}
	svc.RegisterWorkers(worker)
	h := &s24{t: t, pool: pool, queue: queue, worker: worker, org: org}
	approver := scope
	approver.ActorID, approver.OrganizationUserID = ids.New("usr"), ids.New("usr")
	scopeA := scope
	scopeA.PermittedBranchIDs = []string{branchA}

	// Operational data in two branches.
	for _, b := range []string{branchA, branchA, branchB} {
		if _, err := eng.RaiseException(ctx, tenant.Context{OrganizationID: org, ActorType: tenant.ActorTypeOrganizationUser, ActorID: scope.ActorID, CorrelationID: ids.New("c")}, engagementmodels.ExceptionInput{BranchID: b, ExceptionType: "DAMAGE", Severity: "LOW", SourceType: "PACKAGE", SourceReference: ids.New("pkg"), Summary: "Carton dented"}); err != nil {
			t.Fatal(err)
		}
	}

	// --- Dashboards and reports --------------------------------------------------
	dash, err := svc.Dashboard(ctx, scope, "executive", analyticsmodels.ReportFilters{})
	if err != nil || dash.GeneratedAt.IsZero() || len(dash.DataSources) == 0 {
		t.Fatalf("executive dashboard = %+v %v", dash, err)
	}
	openExceptions := func(r analyticsmodels.ReportResult) float64 {
		var total float64
		for _, s := range r.Sections {
			if s.Title == "Open operational exceptions" {
				for _, row := range s.Rows {
					total += row["count"].(float64)
				}
			}
		}
		return total
	}
	if got := openExceptions(dash); got != 3 {
		t.Fatalf("org-wide open exceptions = %v, want 3", got)
	}
	if got, _ := svc.Dashboard(ctx, scopeA, "executive", analyticsmodels.ReportFilters{}); openExceptions(got) != 2 {
		t.Fatalf("branch-scoped open exceptions = %v, want 2", openExceptions(got))
	}
	if _, err = svc.Dashboard(ctx, scopeA, "executive", analyticsmodels.ReportFilters{BranchID: branchB}); err == nil {
		t.Fatal("branch filter widened the caller's scope")
	}
	def, err := svc.CreateReportDefinition(ctx, scope, analyticsmodels.ReportDefinitionInput{Name: "Exec weekly", ReportType: "executive", ScheduleCron: "0 7 * * 1", ScheduleTimezone: "Africa/Lagos"})
	if err != nil || def["next_run_at"] == nil {
		t.Fatalf("definition = %v %v", def, err)
	}
	if _, err = svc.CreateReportDefinition(ctx, scopeA, analyticsmodels.ReportDefinitionInput{Name: "A sched", ReportType: "executive", ScheduleCron: "0 7 * * 1"}); appCode(err) != "validation_failed" {
		t.Fatalf("branch-scoped scheduled report without branch filter = %v", err)
	}
	if _, err = svc.CreateReportDefinition(ctx, scope, analyticsmodels.ReportDefinitionInput{Name: "Bad filter", ReportType: "FINANCE", Filters: json.RawMessage(`{"secretColumn":"x"}`)}); appCode(err) != "validation_failed" {
		t.Fatalf("unknown report filter = %v", err)
	}
	runA, err := svc.RunReport(ctx, scopeA, def["id"].(string), "run-a")
	if err != nil {
		t.Fatal(err)
	}
	runOrg, err := svc.RunReport(ctx, scope, def["id"].(string), "run-org")
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := svc.RunReport(ctx, scopeA, def["id"].(string), "run-a"); again["id"] != runA["id"] {
		t.Fatal("same idempotency key queued a second run")
	}
	h.drain(analyticsusecases.JobReportGenerate)
	// The scoped run honours its captured branch scope.
	done, err := svc.GetReportRun(ctx, scopeA, runA["id"].(string))
	if err != nil || done["status"] != "COMPLETED" {
		t.Fatalf("run = %v %v", done, err)
	}
	// Spec 54.4: a finished run announces itself exactly once, in the same
	// transaction that stored the result.
	var readyEvents int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_outbox WHERE event_type='report_run.ready.v1' AND aggregate_id=$1`, runA["id"]).Scan(&readyEvents); err != nil || readyEvents != 1 {
		t.Fatalf("report_run.ready.v1 events for run = %d (%v), want 1", readyEvents, err)
	}
	var result analyticsmodels.ReportResult
	raw, _ := json.Marshal(done["result"])
	_ = json.Unmarshal(raw, &result)
	if openExceptions(result) != 2 {
		t.Fatalf("scoped run open exceptions = %v, want 2", openExceptions(result))
	}
	// A branch-scoped caller cannot read the org-wide run; another tenant
	// cannot read either.
	if _, err = svc.GetReportRun(ctx, scopeA, runOrg["id"].(string)); err == nil {
		t.Fatal("branch-scoped caller read an organization-wide run")
	}
	otherOrg := createOrganization(t, pool, "Section 24 Other "+ids.New("o"), "S24O-"+ids.New("r"))
	if _, err = svc.GetReportRun(ctx, tenant.Context{OrganizationID: otherOrg.ID, ActorID: "x"}, runA["id"].(string)); err == nil {
		t.Fatal("cross-tenant report run read")
	}

	// --- Recommendations ---------------------------------------------------------
	asset := ids.New("ast")
	if _, err = pool.Exec(ctx, `INSERT INTO fleet_partner_assets(id,organization_id,branch_id,asset_code,asset_type,ownership_type)VALUES($1,$2,$3,$4,'TRUCK','COMPANY_OWNED')`, asset, org, branchA, "TRK-"+asset[len(asset)-6:]); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO fleet_partner_asset_defects(id,organization_id,branch_id,asset_id,source_type,description,severity,reported_by)VALUES($1,$2,$3,$4,'MANUAL_REPORT','Brake fade','CRITICAL','driver')`, ids.New("def"), org, branchA, asset); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for i, odo := range []float64{1000, 900} {
		if _, err = pool.Exec(ctx, `INSERT INTO fleet_partner_fuel_records(id,organization_id,asset_id,provider_membership_id,fuel_or_energy_type,quantity,unit,odometer,recorded_at,supplier_reference)VALUES($1,$2,$3,'mem-x','DIESEL',40,'L',$4,$5,$6)`, ids.New("ful"), org, asset, odo, now.Add(time.Duration(i)*time.Hour), "sup"); err != nil {
			t.Fatal(err)
		}
	}
	gen, err := svc.GenerateRecommendations(ctx, scope, analyticsmodels.GenerateInput{Types: []string{"PREVENTIVE_MAINTENANCE", "ANOMALY_REVIEW"}})
	if err != nil || gen["created"] != 2 {
		t.Fatalf("generate = %v %v", gen, err)
	}
	if gen, _ = svc.GenerateRecommendations(ctx, scope, analyticsmodels.GenerateInput{Types: []string{"PREVENTIVE_MAINTENANCE", "ANOMALY_REVIEW"}}); gen["created"] != 0 {
		t.Fatalf("regeneration duplicated recommendations: %v", gen)
	}
	recs, _ := svc.ListRecommendations(ctx, scope)
	var maintenance map[string]any
	for _, r := range recs {
		if r["recommendation_type"] == "PREVENTIVE_MAINTENANCE" {
			maintenance = r
		}
		if r["model_code"] == "" || r["explanation"] == nil || r["inputs_hash"] == "" {
			t.Fatalf("recommendation not explainable: %v", r)
		}
	}
	accepted, err := svc.DecideRecommendation(ctx, approver, maintenance["id"].(string), analyticsmodels.RecommendationDecision{Decision: "accept", Reason: "Book the workshop", ExpectedVersion: v64(maintenance, "version")})
	if err != nil || accepted["status"] != "ACCEPTED" || accepted["action_draft"] == nil {
		t.Fatalf("accept = %v %v", accepted, err)
	}
	var workOrders int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_work_orders WHERE organization_id=$1`, org).Scan(&workOrders)
	if workOrders != 0 {
		t.Fatal("accepting a recommendation executed a protected action")
	}
	if _, err = svc.DecideRecommendation(ctx, approver, maintenance["id"].(string), analyticsmodels.RecommendationDecision{Decision: "dismiss", ExpectedVersion: v64(accepted, "version")}); err == nil {
		t.Fatal("decided recommendation decided again")
	}

	// --- Automation ----------------------------------------------------------------
	trigger := json.RawMessage(`{"eventType":"exception.opened.v1"}`)
	critical := json.RawMessage(`{"all":[{"field":"payload.severity","op":"eq","value":"CRITICAL"}]}`)
	if _, err = svc.CreateAutomationRule(ctx, scope, analyticsmodels.AutomationRuleInput{Name: "Pay driver", TriggerType: "EVENT", TriggerConfig: trigger, ActionPlan: json.RawMessage(`{"actions":[{"type":"ALLOCATE_WALLET","params":{"amountMinor":100}}]}`)}); appCode(err) != "validation_failed" {
		t.Fatalf("protected automation action accepted: %v", err)
	}
	if _, err = svc.CreateAutomationRule(ctx, scope, analyticsmodels.AutomationRuleInput{Name: "Untrusted trigger", TriggerType: "EVENT", TriggerConfig: json.RawMessage(`{"eventType":"wallet.entry.posted.v1"}`), ActionPlan: json.RawMessage(`{"actions":[{"type":"QUEUE_NOTIFICATION","params":{"recipientRole":"OPS","templateCode":"x"}}]}`)}); appCode(err) != "validation_failed" {
		t.Fatalf("non-catalogue trigger accepted: %v", err)
	}
	rule, err := svc.CreateAutomationRule(ctx, scope, analyticsmodels.AutomationRuleInput{Name: "Critical escalation", TriggerType: "EVENT", TriggerConfig: trigger, ConditionTree: critical,
		ActionPlan: json.RawMessage(`{"actions":[{"type":"QUEUE_NOTIFICATION","params":{"recipientRole":"OPERATIONS","templateCode":"fleet.exception.critical"}},{"type":"RAISE_EXCEPTION","params":{"exceptionType":"OTHER","severity":"CRITICAL","summary":"Follow-up for a critical exception"}}]}`)})
	if err != nil {
		t.Fatal(err)
	}
	rid := rule["id"].(string)
	if rule, err = svc.AutomationRuleCommand(ctx, scope, rid, "SUBMIT", analyticsmodels.AutomationRuleInput{ExpectedVersion: v64(rule, "version")}); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.AutomationRuleCommand(ctx, scope, rid, "APPROVE", analyticsmodels.AutomationRuleInput{ExpectedVersion: v64(rule, "version")}); appCode(err) != "segregation_of_duties_violation" {
		t.Fatalf("self-approval = %v", err)
	}
	if rule, err = svc.AutomationRuleCommand(ctx, approver, rid, "APPROVE", analyticsmodels.AutomationRuleInput{ExpectedVersion: v64(rule, "version")}); err != nil || rule["status"] != "ACTIVE" {
		t.Fatalf("approve = %v %v", rule, err)
	}
	// The evaluator starts at activation: initialise the cursor, then raise
	// one CRITICAL and one LOW exception.
	h.runJob(analyticsusecases.JobAutomationEval, map[string]string{})
	sys := tenant.Context{OrganizationID: org, ActorType: tenant.ActorTypeOrganizationUser, ActorID: scope.ActorID, CorrelationID: ids.New("c")}
	crit, err := eng.RaiseException(ctx, sys, engagementmodels.ExceptionInput{ExceptionType: "DELAY", Severity: "CRITICAL", SourceType: "SHIPMENT", SourceReference: ids.New("shp"), Summary: "Cold chain at risk"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = eng.RaiseException(ctx, sys, engagementmodels.ExceptionInput{ExceptionType: "DELAY", Severity: "LOW", SourceType: "SHIPMENT", SourceReference: ids.New("shp"), Summary: "Minor delay"}); err != nil {
		t.Fatal(err)
	}
	h.runJob(analyticsusecases.JobAutomationEval, map[string]string{})
	h.drain(analyticsusecases.JobRuleExecute)
	// Replay: re-evaluating creates nothing new; the automation-raised
	// CRITICAL exception does not trigger the rule again.
	h.runJob(analyticsusecases.JobAutomationEval, map[string]string{})
	h.drain(analyticsusecases.JobRuleExecute)
	execs, _ := svc.ListAutomationExecutions(ctx, scope)
	completed, skipped := 0, 0
	for _, e := range execs {
		switch e["status"] {
		case "COMPLETED":
			completed++
		case "SKIPPED":
			skipped++
		}
	}
	if completed != 1 || skipped != 1 || len(execs) != 2 {
		t.Fatalf("executions completed=%d skipped=%d total=%d: %v", completed, skipped, len(execs), execs)
	}
	var autoExceptions, intents int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_operational_exceptions WHERE organization_id=$1 AND source_type='AUTOMATION_EXECUTION'`, org).Scan(&autoExceptions)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_notification_intents WHERE organization_id=$1 AND source_type='AUTOMATION_EXECUTION'`, org).Scan(&intents)
	if autoExceptions != 1 || intents != 1 {
		t.Fatalf("automation effects exceptions=%d intents=%d, want 1 and 1 (crit=%v)", autoExceptions, intents, crit["id"])
	}
	if _, err = pool.Exec(ctx, `UPDATE fleet_partner_automation_rule_versions SET status='X' WHERE rule_id=$1`, rid); err == nil {
		t.Fatal("automation rule versions were mutable")
	}

	// --- Privacy requests and security events -------------------------------------
	provider := ids.New("prv")
	erasure, err := gov.SubmitPrivacyRequest(ctx, scope, governancemodels.PrivacyRequestInput{RequestType: "erasure", SubjectType: "provider", SubjectReference: provider, Reason: "Former driver asked for erasure"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = gov.DecidePrivacyRequest(ctx, scope, erasure["id"].(string), governancemodels.PrivacyDecisionInput{Decision: "APPROVE", ExpectedVersion: v64(erasure, "version")}); appCode(err) != "segregation_of_duties_violation" {
		t.Fatalf("requester decided own privacy request: %v", err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO fleet_partner_legal_holds(id,organization_id,scope_type,scope_reference,reason,created_by_actor_type,created_by_actor_id)VALUES($1,$2,'PROVIDER',$3,'Open claim','ORGANIZATION_USER','legal')`, ids.New("lgh"), org, provider); err != nil {
		t.Fatal(err)
	}
	blocked, err := gov.DecidePrivacyRequest(ctx, approver, erasure["id"].(string), governancemodels.PrivacyDecisionInput{Decision: "APPROVE", ExpectedVersion: v64(erasure, "version")})
	if err != nil || blocked["status"] != "BLOCKED_BY_LEGAL_HOLD" {
		t.Fatalf("erasure under legal hold = %v %v", blocked["status"], err)
	}
	export, err := gov.SubmitPrivacyRequest(ctx, scope, governancemodels.PrivacyRequestInput{RequestType: "EXPORT", SubjectType: "CUSTOMER", SubjectReference: ids.New("cus"), Reason: "Subject access request"})
	if err != nil {
		t.Fatal(err)
	}
	if export, err = gov.DecidePrivacyRequest(ctx, approver, export["id"].(string), governancemodels.PrivacyDecisionInput{Decision: "APPROVE", ExpectedVersion: v64(export, "version")}); err != nil {
		t.Fatal(err)
	}
	if _, err = gov.CompletePrivacyRequest(ctx, scope, export["id"].(string), governancemodels.PrivacyDecisionInput{ExpectedVersion: v64(export, "version")}); appCode(err) != "validation_failed" {
		t.Fatalf("completion without report = %v", err)
	}
	if export, err = gov.CompletePrivacyRequest(ctx, scope, export["id"].(string), governancemodels.PrivacyDecisionInput{ExpectedVersion: v64(export, "version"), CompletionReport: json.RawMessage(`{"exportJobId":"exp_1","recordsIncluded":12}`)}); err != nil || export["status"] != "COMPLETED" {
		t.Fatalf("complete = %v %v", export["status"], err)
	}
	for _, o := range []string{org, otherOrg.ID} {
		if _, err = pool.Exec(ctx, `INSERT INTO fleet_partner_auth_security_events(id,event_type,severity,organization_id,ip_hash)VALUES($1,'LOGIN_FAILED','WARN',$2,'h')`, ids.New("sev"), o); err != nil {
			t.Fatal(err)
		}
	}
	secEvents, err := gov.SecurityEvents(ctx, scope)
	if err != nil || len(secEvents) != 1 {
		t.Fatalf("security events = %d %v, want 1 (no cross-tenant)", len(secEvents), err)
	}

	// --- Organization staff sessions (spec 59.10) --------------------------------
	// Disabled by default; then organization-wide security managers list and
	// revoke their own organization's sessions only, through the auth
	// feature's one session table.
	if _, err = gov.ListOrganizationSessions(ctx, scope, ""); appCode(err) != "not_found" {
		t.Fatalf("session management while disabled = %v, want not_found", err)
	}
	cfg.SecuritySessionsEnabled = true
	gov = app.NewGovernanceService(cfg, pool, publisher, authrepos.NewSessionRepository(pool))
	sessions := authrepos.NewSessionRepository(pool)
	seedSession := func(orgID, identity, family string) string {
		id, userID := ids.New("ses"), ids.New("usr")
		s := &authmodels.Session{ID: id, IdentityID: identity, OrganizationUserID: &userID, OrganizationID: &orgID, SessionFamilyID: family,
			RefreshTokenHash: ids.New("hash"), Status: authmodels.SessionStatusActive, RefreshExpiresAt: time.Now().Add(time.Hour)}
		if err := sessions.Create(ctx, pool, s); err != nil {
			t.Fatal(err)
		}
		return id
	}
	staff := seedIdentity(t, pool, ids.New("s")+"@example.test", "+23480"+fmt.Sprint(time.Now().UnixNano()%100000000))
	family := ids.New("fam")
	mine := seedSession(org, staff, family)
	// The same browser family later switched to another organization: an
	// organization may end its own access, never the other one's.
	elsewhere := seedSession(otherOrg.ID, staff, family)
	foreign := seedSession(otherOrg.ID, staff, ids.New("fam"))
	if _, err = gov.ListOrganizationSessions(ctx, scopeA, ""); appCode(err) != "forbidden" {
		t.Fatalf("branch-scoped session listing = %v, want forbidden", err)
	}
	listed, err := gov.ListOrganizationSessions(ctx, scope, mine)
	if err != nil || len(listed) != 1 || listed[0].ID != mine || !listed[0].Current {
		t.Fatalf("organization sessions = %+v %v, want only %s marked current", listed, err, mine)
	}
	if _, err = gov.RevokeOrganizationSession(ctx, scope, mine, governancemodels.SessionRevokeInput{}); appCode(err) != "validation_failed" {
		t.Fatalf("revoke without reason = %v", err)
	}
	if _, err = gov.RevokeOrganizationSession(ctx, scope, foreign, governancemodels.SessionRevokeInput{Reason: "lost laptop"}); appCode(err) != "not_found" {
		t.Fatalf("revoking another organization's session = %v, want not_found", err)
	}
	revoked, err := gov.RevokeOrganizationSession(ctx, scope, mine, governancemodels.SessionRevokeInput{Reason: "lost laptop"})
	if err != nil || revoked.Status != authmodels.SessionStatusRevoked {
		t.Fatalf("revoke = %+v %v", revoked, err)
	}
	if _, err = gov.RevokeOrganizationSession(ctx, scope, mine, governancemodels.SessionRevokeInput{Reason: "lost laptop"}); err != nil {
		t.Fatalf("repeat revoke = %v, want a no-op success", err)
	}
	for id, want := range map[string]string{mine: "REVOKED", elsewhere: "ACTIVE", foreign: "ACTIVE"} {
		var status string
		if err = pool.QueryRow(ctx, `SELECT status FROM fleet_partner_corporate_sessions WHERE id=$1`, id).Scan(&status); err != nil || status != want {
			t.Fatalf("session %s status = %s (%v), want %s", id, status, err, want)
		}
	}
	var revokedEvents, revokedAudit int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_outbox WHERE event_type='security.session_revoked.v1' AND aggregate_id=$1`, mine).Scan(&revokedEvents); err != nil || revokedEvents != 1 {
		t.Fatalf("security.session_revoked.v1 events = %d (%v), want exactly 1", revokedEvents, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_audit_records WHERE resource_id=$1 AND action='security.session_revoked' AND reason='lost laptop'`, mine).Scan(&revokedAudit); err != nil || revokedAudit != 1 {
		t.Fatalf("session revocation audit entries with reason = %d (%v), want 1", revokedAudit, err)
	}
}
