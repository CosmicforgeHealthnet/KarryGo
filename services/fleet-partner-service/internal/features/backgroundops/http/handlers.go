package http

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/scheduler"
)

// Permission codes, exactly as seeded by migrations 000063 and 000064.
const (
	PermissionJobsRead   = "jobs.read"
	PermissionJobsManage = "jobs.manage"
	PermissionJobsRetry  = "jobs.retry"
	PermissionJobsCancel = "jobs.cancel"

	PermissionScheduledTasksRead   = "scheduled_tasks.read"
	PermissionScheduledTasksManage = "scheduled_tasks.manage"
	PermissionScheduledTasksRun    = "scheduled_tasks.run"
)

// Handler serves the Section 27 operator surface.
type Handler struct {
	jobs       *jobs.Queue
	scheduler  *scheduler.Repository
	authorizer *accesscontrol.Authorizer
}

// NewHandler builds the handler.
func NewHandler(jobQueue *jobs.Queue, schedulerRepo *scheduler.Repository, authorizer *accesscontrol.Authorizer) *Handler {
	return &Handler{jobs: jobQueue, scheduler: schedulerRepo, authorizer: authorizer}
}

// RegisterTenantRoutes mounts the corporate background-ops routes.
func (h *Handler) RegisterTenantRoutes(group *gin.RouterGroup) {
	if h.jobs == nil || h.scheduler == nil {
		return
	}
	mount(group, h.TenantRoutes(), map[string]gin.HandlerFunc{
		"listJobs":              h.listJobs,
		"getJob":                h.getJob,
		"listJobAttempts":       h.listJobAttempts,
		"retryJob":              h.retryJob,
		"cancelJob":             h.cancelJob,
		"listScheduledTasks":    h.listScheduledTasks,
		"getScheduledTask":      h.getScheduledTask,
		"listScheduledTaskRuns": h.listScheduledTaskRuns,
		"enableScheduledTask":   h.enableScheduledTask,
		"disableScheduledTask":  h.disableScheduledTask,
		"runScheduledTaskNow":   h.runScheduledTaskNow,
	})
}

func mount(group *gin.RouterGroup, routes []Route, handlers map[string]gin.HandlerFunc) {
	for _, route := range routes {
		handler, ok := handlers[route.OperationID]
		if !ok || handler == nil {
			panic("backgroundops: no handler registered for operation " + route.OperationID)
		}
		group.Handle(route.Method, route.Path, handler)
	}
}

func requireTenant(c *gin.Context) (tenant.Context, bool) {
	tenantCtx, ok := middleware.TenantFromGin(c)
	if !ok {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return tenant.Context{}, false
	}
	return tenantCtx, true
}

func (h *Handler) authorize(c *gin.Context, t tenant.Context, permission string) bool {
	if h.authorizer == nil {
		respond.Error(c, apperrors.Forbidden("You do not have permission to perform this action.", nil))
		return false
	}
	if err := h.authorizer.Require(c.Request.Context(), t.OrganizationID, t.OrganizationUserID,
		roleVersionFrom(c), permission, accesscontrol.Target{BranchID: c.Query("branchId")}); err != nil {
		respond.Error(c, err)
		return false
	}
	return true
}

func roleVersionFrom(c *gin.Context) int64 {
	if value, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		if version, ok := value.(int64); ok {
			return version
		}
	}
	return 1
}

// JobView is the safe wire summary of a job -- deliberately excludes the
// raw payload (which may carry references not meant for broad operator
// display) and any unredacted error text; last_error is always the
// already-redacted, size-bounded safe message the queue itself produced.
type JobView struct {
	ID                    string     `json:"id"`
	JobType               string     `json:"jobType"`
	Status                string     `json:"status"`
	Priority              int        `json:"priority"`
	Attempts              int        `json:"attempts"`
	MaxAttempts           int        `json:"maxAttempts"`
	ScheduledAt           time.Time  `json:"scheduledAt"`
	NextAttemptAt         *time.Time `json:"nextAttemptAt,omitempty"`
	LastErrorClass        string     `json:"lastErrorClass,omitempty"`
	LastErrorCode         string     `json:"lastErrorCode,omitempty"`
	LastSafeErrorMessage  string     `json:"lastSafeErrorMessage,omitempty"`
	ExternalOutcomeStatus string     `json:"externalOutcomeStatus,omitempty"`
	ScheduledTaskID       string     `json:"scheduledTaskId,omitempty"`
	Version               int64      `json:"version"`
	CreatedAt             time.Time  `json:"createdAt"`
	CompletedAt           *time.Time `json:"completedAt,omitempty"`
	CancelledAt           *time.Time `json:"cancelledAt,omitempty"`
	DeadLetteredAt        *time.Time `json:"deadLetteredAt,omitempty"`
}

func newJobView(j jobs.Job) JobView {
	return JobView{
		ID: j.ID, JobType: j.JobType, Status: j.Status, Priority: j.Priority,
		Attempts: j.Attempts, MaxAttempts: j.MaxAttempts, ScheduledAt: j.ScheduledAt,
		NextAttemptAt: j.NextAttemptAt, LastErrorClass: j.LastErrorClass, LastErrorCode: j.LastErrorCode,
		LastSafeErrorMessage: j.LastError, ExternalOutcomeStatus: j.ExternalOutcomeStatus,
		ScheduledTaskID: j.ScheduledTaskID, Version: j.Version, CreatedAt: j.CreatedAt,
		CompletedAt: j.CompletedAt, CancelledAt: j.CancelledAt, DeadLetteredAt: j.DeadLetteredAt,
	}
}

// AttemptView is the safe wire summary of one immutable execution attempt.
type AttemptView struct {
	ID            string     `json:"id"`
	AttemptNumber int        `json:"attemptNumber"`
	WorkerOwner   string     `json:"workerOwner"`
	Status        string     `json:"status"`
	ErrorClass    string     `json:"errorClass,omitempty"`
	ErrorCode     string     `json:"errorCode,omitempty"`
	SafeMessage   string     `json:"safeMessage,omitempty"`
	StartedAt     time.Time  `json:"startedAt"`
	HeartbeatAt   *time.Time `json:"heartbeatAt,omitempty"`
	CompletedAt   *time.Time `json:"completedAt,omitempty"`
}

func newAttemptView(a jobs.Attempt) AttemptView {
	return AttemptView{
		ID: a.ID, AttemptNumber: a.AttemptNumber, WorkerOwner: a.WorkerOwner, Status: a.Status,
		ErrorClass: a.ErrorClass, ErrorCode: a.ErrorCode, SafeMessage: a.ErrorRedacted,
		StartedAt: a.StartedAt, HeartbeatAt: a.HeartbeatAt, CompletedAt: a.CompletedAt,
	}
}

func (h *Handler) listJobs(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionJobsRead) {
		return
	}
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			respond.Error(c, apperrors.BadRequest("limit must be an integer.", nil))
			return
		}
		limit = parsed
	}
	items, err := h.jobs.List(c.Request.Context(), t.OrganizationID, jobs.ListFilter{
		Status: c.Query("status"), JobType: c.Query("jobType"), Limit: limit,
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]JobView, 0, len(items))
	for _, item := range items {
		views = append(views, newJobView(item))
	}
	respond.OK(c, views)
}

func (h *Handler) getJob(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionJobsRead) {
		return
	}
	item, err := h.jobs.Get(c.Request.Context(), t.OrganizationID, c.Param("jobId"))
	if err != nil {
		respond.Error(c, apperrors.NotFound("Job not found.", err))
		return
	}
	respond.OK(c, newJobView(item))
}

func (h *Handler) listJobAttempts(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionJobsRead) {
		return
	}
	attempts, err := h.jobs.ListAttempts(c.Request.Context(), t.OrganizationID, c.Param("jobId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]AttemptView, 0, len(attempts))
	for _, attempt := range attempts {
		views = append(views, newAttemptView(attempt))
	}
	respond.OK(c, views)
}

// retryJob replays a dead-lettered job back to PENDING. Only a
// DEAD_LETTERED job is eligible -- Replay itself enforces that and refuses
// otherwise, so a successful job can never be forced to re-execute through
// this route.
func (h *Handler) retryJob(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionJobsRetry) {
		return
	}
	if err := h.jobs.Replay(c.Request.Context(), t.OrganizationID, c.Param("jobId")); err != nil {
		respond.Error(c, apperrors.NotFound("Job not found, or not eligible for retry (must be dead-lettered).", err))
		return
	}
	item, err := h.jobs.Get(c.Request.Context(), t.OrganizationID, c.Param("jobId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, newJobView(item))
}

func (h *Handler) cancelJob(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionJobsCancel) {
		return
	}
	if err := h.jobs.Cancel(c.Request.Context(), t.OrganizationID, c.Param("jobId")); err != nil {
		respond.Error(c, apperrors.Conflict("Job not found, or not cancellable (must be pending or retry-scheduled).", err))
		return
	}
	item, err := h.jobs.Get(c.Request.Context(), t.OrganizationID, c.Param("jobId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, newJobView(item))
}

// TaskView is the safe wire summary of a scheduled task -- excludes the raw
// payload for the same reason JobView does.
type TaskView struct {
	ID                 string     `json:"id"`
	TaskType           string     `json:"taskType"`
	JobType            string     `json:"jobType"`
	ScheduleType       string     `json:"scheduleType"`
	ScheduleExpression string     `json:"scheduleExpression"`
	Timezone           string     `json:"timezone"`
	Enabled            bool       `json:"enabled"`
	NextRunAt          *time.Time `json:"nextRunAt,omitempty"`
	LastScheduledAt    *time.Time `json:"lastScheduledAt,omitempty"`
	LastResult         string     `json:"lastResult,omitempty"`
	FailureCount       int        `json:"failureCount"`
	ConcurrencyPolicy  string     `json:"concurrencyPolicy"`
	MisfirePolicy      string     `json:"misfirePolicy"`
	MaxCatchUp         int        `json:"maxCatchUp"`
	Version            int64      `json:"version"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

func newTaskView(task scheduler.Task) TaskView {
	return TaskView{
		ID: task.ID, TaskType: task.TaskType, JobType: task.JobType, ScheduleType: task.ScheduleType,
		ScheduleExpression: task.ScheduleExpression, Timezone: task.Timezone, Enabled: task.Enabled,
		NextRunAt: task.NextRunAt, LastScheduledAt: task.LastScheduledAt, LastResult: task.LastResult,
		FailureCount: task.FailureCount, ConcurrencyPolicy: task.ConcurrencyPolicy,
		MisfirePolicy: task.MisfirePolicy, MaxCatchUp: task.MaxCatchUp, Version: task.Version,
		CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt,
	}
}

// RunView is the safe wire summary of one scheduler decision record.
type RunView struct {
	ID            string    `json:"id"`
	ScheduledFor  time.Time `json:"scheduledFor"`
	Outcome       string    `json:"outcome"`
	JobID         string    `json:"jobId,omitempty"`
	SafeErrorCode string    `json:"safeErrorCode,omitempty"`
	OccurredAt    time.Time `json:"occurredAt"`
}

func newRunView(run scheduler.RunRecord) RunView {
	return RunView{
		ID: run.ID, ScheduledFor: run.ScheduledFor, Outcome: run.Outcome,
		JobID: run.JobID, SafeErrorCode: run.SafeErrorCode, OccurredAt: run.OccurredAt,
	}
}

func (h *Handler) listScheduledTasks(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionScheduledTasksRead) {
		return
	}
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			respond.Error(c, apperrors.BadRequest("limit must be an integer.", nil))
			return
		}
		limit = parsed
	}
	tasks, err := h.scheduler.List(c.Request.Context(), t.OrganizationID, limit)
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]TaskView, 0, len(tasks))
	for _, task := range tasks {
		views = append(views, newTaskView(task))
	}
	respond.OK(c, views)
}

func (h *Handler) getScheduledTask(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionScheduledTasksRead) {
		return
	}
	task, err := h.scheduler.Get(c.Request.Context(), t.OrganizationID, c.Param("taskId"))
	if err != nil {
		respond.Error(c, apperrors.NotFound("Scheduled task not found.", err))
		return
	}
	respond.OK(c, newTaskView(task))
}

func (h *Handler) listScheduledTaskRuns(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionScheduledTasksRead) {
		return
	}
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			respond.Error(c, apperrors.BadRequest("limit must be an integer.", nil))
			return
		}
		limit = parsed
	}
	runs, err := h.scheduler.ListRuns(c.Request.Context(), t.OrganizationID, c.Param("taskId"), limit)
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]RunView, 0, len(runs))
	for _, run := range runs {
		views = append(views, newRunView(run))
	}
	respond.OK(c, views)
}

type expectedVersionBody struct {
	ExpectedVersion *int64 `json:"expectedVersion"`
}

func expectedVersionFrom(c *gin.Context) (int64, bool) {
	var body expectedVersionBody
	_ = c.ShouldBindJSON(&body)
	version, err := concurrency.ExpectedVersion(c.GetHeader("If-Match"), body.ExpectedVersion)
	if err != nil {
		respond.Error(c, err)
		return 0, false
	}
	return version, true
}

func schedulerConflict(err error) error {
	if err == scheduler.ErrVersionConflict {
		return apperrors.Conflict("The scheduled task was modified by someone else. Reload and retry.", err)
	}
	return err
}

func actorFrom(t tenant.Context) string {
	if t.OrganizationUserID != "" {
		return t.OrganizationUserID
	}
	return "operator"
}

func (h *Handler) enableScheduledTask(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionScheduledTasksManage) {
		return
	}
	version, ok := expectedVersionFrom(c)
	if !ok {
		return
	}
	task, err := h.scheduler.SetEnabled(c.Request.Context(), t.OrganizationID, c.Param("taskId"), true, version, actorFrom(t))
	if err != nil {
		respond.Error(c, schedulerConflict(err))
		return
	}
	respond.OK(c, newTaskView(task))
}

func (h *Handler) disableScheduledTask(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionScheduledTasksManage) {
		return
	}
	version, ok := expectedVersionFrom(c)
	if !ok {
		return
	}
	task, err := h.scheduler.SetEnabled(c.Request.Context(), t.OrganizationID, c.Param("taskId"), false, version, actorFrom(t))
	if err != nil {
		respond.Error(c, schedulerConflict(err))
		return
	}
	respond.OK(c, newTaskView(task))
}

// runScheduledTaskNow forces the task's next_run_at to now, so the running
// scheduler picks it up on its next tick -- it does not run the job inline
// on this request, and it does not alter the task's ordinary schedule.
func (h *Handler) runScheduledTaskNow(c *gin.Context) {
	t, ok := requireTenant(c)
	if !ok || !h.authorize(c, t, PermissionScheduledTasksRun) {
		return
	}
	version, ok := expectedVersionFrom(c)
	if !ok {
		return
	}
	task, err := h.scheduler.RunNow(c.Request.Context(), t.OrganizationID, c.Param("taskId"), version, actorFrom(t))
	if err != nil {
		respond.Error(c, schedulerConflict(err))
		return
	}
	respond.OK(c, newTaskView(task))
}
