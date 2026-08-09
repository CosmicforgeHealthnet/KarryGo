package http

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/scheduler"
)

// worker_operations.* (service-global jobs: organization_id IS NULL) sit
// behind the internal route class, which has already applied HMAC service
// authentication (including timestamp freshness) and the nonce replay gate
// -- the same "an existing internal/platform-operator authorization
// mechanism" every other cross-tenant-adjacent surface in this service
// uses (see documents/http/internal.go). What this adds is the per-route
// caller allowlist: holding a valid service secret proves who you are, not
// that you may operate on platform-scoped jobs.
//
// This deliberately does NOT expose other organizations' tenant-scoped
// jobs/tasks -- every method below is hardcoded to organizationID="" (the
// platform scope), never a caller-supplied organization. Cross-tenant
// visibility into ANY specific organization's jobs is out of scope; that
// would be a materially larger privilege-escalation surface than "service-
// global jobs" in the master specification calls for.

// InternalHandler serves the service-to-service worker-operations surface.
type InternalHandler struct {
	jobs      *jobs.Queue
	scheduler *scheduler.Repository
	outbox    *events.OutboxRepository
	allowed   map[string]bool
}

// NewInternalHandler builds the handler with a caller allowlist. outbox may
// be nil (e.g. in a composition that never wires the outbox repository);
// the two Section 29 operational routes that use it degrade to a safe 503
// rather than a nil-pointer panic — see requireOutbox.
func NewInternalHandler(jobQueue *jobs.Queue, schedulerRepo *scheduler.Repository, outbox *events.OutboxRepository, allowedCallers []string) *InternalHandler {
	allowed := map[string]bool{}
	for _, caller := range allowedCallers {
		if name := strings.TrimSpace(caller); name != "" {
			allowed[name] = true
		}
	}
	return &InternalHandler{jobs: jobQueue, scheduler: schedulerRepo, outbox: outbox, allowed: allowed}
}

// InternalRoutes is the HMAC-authenticated worker-operations surface,
// mounted under /internal/fleet-partner/v1 by the composition root.
func InternalRoutes() []Route {
	return []Route{
		{Method: "GET", Path: "/worker-operations/jobs", OperationID: "listPlatformJobs"},
		{Method: "GET", Path: "/worker-operations/jobs/:jobId", OperationID: "getPlatformJob"},
		{Method: "GET", Path: "/worker-operations/jobs/:jobId/attempts", OperationID: "listPlatformJobAttempts"},
		{Method: "POST", Path: "/worker-operations/jobs/:jobId/retry", OperationID: "retryPlatformJob"},
		{Method: "POST", Path: "/worker-operations/jobs/:jobId/cancel", OperationID: "cancelPlatformJob"},
		{Method: "GET", Path: "/worker-operations/scheduled-tasks", OperationID: "listPlatformScheduledTasks"},
		{Method: "GET", Path: "/worker-operations/scheduled-tasks/:taskId", OperationID: "getPlatformScheduledTask"},
		{Method: "GET", Path: "/worker-operations/scheduled-tasks/:taskId/runs", OperationID: "listPlatformScheduledTaskRuns"},
		{Method: "POST", Path: "/worker-operations/scheduled-tasks/:taskId/enable", OperationID: "enablePlatformScheduledTask"},
		{Method: "POST", Path: "/worker-operations/scheduled-tasks/:taskId/disable", OperationID: "disablePlatformScheduledTask"},
		{Method: "POST", Path: "/worker-operations/scheduled-tasks/:taskId/run-now", OperationID: "runPlatformScheduledTaskNow"},

		// Section 29 operational surface: platform-wide backlog visibility
		// and dead-lettered-event replay. Deliberately read-mostly — the
		// only mutation is replaying a SPECIFIC already-dead-lettered
		// record by ID, mirroring retryPlatformJob's own scope (never a
		// bulk/blind replay-everything operation).
		{Method: "GET", Path: "/operations/backlogs", OperationID: "getPlatformBacklogs"},
		{Method: "POST", Path: "/operations/outbox/replay", OperationID: "replayOutboxRecord"},
	}
}

// RegisterInternalRoutes mounts the worker-operations routes.
func RegisterInternalRoutes(group *gin.RouterGroup, handler *InternalHandler) {
	for _, route := range InternalRoutes() {
		group.Handle(route.Method, route.Path, handler.dispatch(route.OperationID))
	}
}

func (h *InternalHandler) dispatch(operationID string) gin.HandlerFunc {
	handlers := map[string]gin.HandlerFunc{
		"listPlatformJobs":              h.listJobs,
		"getPlatformJob":                h.getJob,
		"listPlatformJobAttempts":       h.listJobAttempts,
		"retryPlatformJob":              h.retryJob,
		"cancelPlatformJob":             h.cancelJob,
		"listPlatformScheduledTasks":    h.listScheduledTasks,
		"getPlatformScheduledTask":      h.getScheduledTask,
		"listPlatformScheduledTaskRuns": h.listScheduledTaskRuns,
		"enablePlatformScheduledTask":   h.enableScheduledTask,
		"disablePlatformScheduledTask":  h.disableScheduledTask,
		"runPlatformScheduledTaskNow":   h.runScheduledTaskNow,
		"getPlatformBacklogs":           h.getBacklogs,
		"replayOutboxRecord":            h.replayOutboxRecord,
	}
	handler, ok := handlers[operationID]
	if !ok {
		panic("backgroundops: no internal handler registered for operation " + operationID)
	}
	return handler
}

func (h *InternalHandler) requireAllowedCaller(c *gin.Context) bool {
	caller := c.GetString(middleware.ServiceCallerKey)
	if !h.allowed[caller] {
		c.JSON(http.StatusForbidden, gin.H{"status": "REJECTED", "code": "caller_not_allowed"})
		return false
	}
	return true
}

func (h *InternalHandler) listJobs(c *gin.Context) {
	if !h.requireAllowedCaller(c) {
		return
	}
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	items, err := h.jobs.List(c.Request.Context(), "", jobs.ListFilter{
		Status: c.Query("status"), JobType: c.Query("jobType"), Limit: limit,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "ERROR"})
		return
	}
	views := make([]JobView, 0, len(items))
	for _, item := range items {
		views = append(views, newJobView(item))
	}
	c.JSON(http.StatusOK, gin.H{"jobs": views})
}

func (h *InternalHandler) getJob(c *gin.Context) {
	if !h.requireAllowedCaller(c) {
		return
	}
	item, err := h.jobs.Get(c.Request.Context(), "", c.Param("jobId"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "NOT_FOUND"})
		return
	}
	c.JSON(http.StatusOK, newJobView(item))
}

func (h *InternalHandler) listJobAttempts(c *gin.Context) {
	if !h.requireAllowedCaller(c) {
		return
	}
	attempts, err := h.jobs.ListAttempts(c.Request.Context(), "", c.Param("jobId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "ERROR"})
		return
	}
	views := make([]AttemptView, 0, len(attempts))
	for _, attempt := range attempts {
		views = append(views, newAttemptView(attempt))
	}
	c.JSON(http.StatusOK, gin.H{"attempts": views})
}

func (h *InternalHandler) retryJob(c *gin.Context) {
	if !h.requireAllowedCaller(c) {
		return
	}
	if err := h.jobs.Replay(c.Request.Context(), "", c.Param("jobId")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "NOT_FOUND", "code": "not_dead_lettered"})
		return
	}
	item, err := h.jobs.Get(c.Request.Context(), "", c.Param("jobId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "ERROR"})
		return
	}
	c.JSON(http.StatusOK, newJobView(item))
}

func (h *InternalHandler) cancelJob(c *gin.Context) {
	if !h.requireAllowedCaller(c) {
		return
	}
	if err := h.jobs.Cancel(c.Request.Context(), "", c.Param("jobId")); err != nil {
		c.JSON(http.StatusConflict, gin.H{"status": "REJECTED", "code": "not_cancellable"})
		return
	}
	item, err := h.jobs.Get(c.Request.Context(), "", c.Param("jobId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "ERROR"})
		return
	}
	c.JSON(http.StatusOK, newJobView(item))
}

func (h *InternalHandler) listScheduledTasks(c *gin.Context) {
	if !h.requireAllowedCaller(c) {
		return
	}
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	tasks, err := h.scheduler.List(c.Request.Context(), "", limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "ERROR"})
		return
	}
	views := make([]TaskView, 0, len(tasks))
	for _, task := range tasks {
		views = append(views, newTaskView(task))
	}
	c.JSON(http.StatusOK, gin.H{"scheduledTasks": views})
}

func (h *InternalHandler) getScheduledTask(c *gin.Context) {
	if !h.requireAllowedCaller(c) {
		return
	}
	task, err := h.scheduler.Get(c.Request.Context(), "", c.Param("taskId"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"status": "NOT_FOUND"})
		return
	}
	c.JSON(http.StatusOK, newTaskView(task))
}

func (h *InternalHandler) listScheduledTaskRuns(c *gin.Context) {
	if !h.requireAllowedCaller(c) {
		return
	}
	limit := 0
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	runs, err := h.scheduler.ListRuns(c.Request.Context(), "", c.Param("taskId"), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "ERROR"})
		return
	}
	views := make([]RunView, 0, len(runs))
	for _, run := range runs {
		views = append(views, newRunView(run))
	}
	c.JSON(http.StatusOK, gin.H{"runs": views})
}

type internalExpectedVersionBody struct {
	ExpectedVersion int64 `json:"expectedVersion"`
}

func (h *InternalHandler) enableScheduledTask(c *gin.Context) {
	h.setEnabled(c, true)
}

func (h *InternalHandler) disableScheduledTask(c *gin.Context) {
	h.setEnabled(c, false)
}

func (h *InternalHandler) setEnabled(c *gin.Context, enabled bool) {
	if !h.requireAllowedCaller(c) {
		return
	}
	var body internalExpectedVersionBody
	if err := c.ShouldBindJSON(&body); err != nil || body.ExpectedVersion < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "REJECTED", "code": "expected_version_required"})
		return
	}
	task, err := h.scheduler.SetEnabled(c.Request.Context(), "", c.Param("taskId"), enabled,
		body.ExpectedVersion, c.GetString(middleware.ServiceCallerKey))
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"status": "REJECTED", "code": "version_conflict"})
		return
	}
	c.JSON(http.StatusOK, newTaskView(task))
}

func (h *InternalHandler) runScheduledTaskNow(c *gin.Context) {
	if !h.requireAllowedCaller(c) {
		return
	}
	var body internalExpectedVersionBody
	if err := c.ShouldBindJSON(&body); err != nil || body.ExpectedVersion < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"status": "REJECTED", "code": "expected_version_required"})
		return
	}
	task, err := h.scheduler.RunNow(c.Request.Context(), "", c.Param("taskId"),
		body.ExpectedVersion, c.GetString(middleware.ServiceCallerKey))
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"status": "REJECTED", "code": "version_conflict"})
		return
	}
	c.JSON(http.StatusOK, newTaskView(task))
}

func (h *InternalHandler) requireOutbox(c *gin.Context) bool {
	if h.outbox == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "UNAVAILABLE", "code": "outbox_not_configured"})
		return false
	}
	return true
}

// getBacklogs is the Section 29 operational dashboard read: platform-wide
// job and outbox backlog/staleness counters, the same figures readiness
// already gates on (see cmd/main.go's job_stale_leases/job_dead_letter_
// backlog/outbox_backlog checks) exposed for an operator to inspect
// on-demand rather than only at a pass/fail readiness boundary.
func (h *InternalHandler) getBacklogs(c *gin.Context) {
	if !h.requireAllowedCaller(c) {
		return
	}
	ctx := c.Request.Context()
	body := gin.H{}
	if h.jobs != nil {
		counts, err := h.jobs.Counts(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "ERROR"})
			return
		}
		deadLetter, err := h.jobs.DeadLetterCount(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "ERROR"})
			return
		}
		body["jobs"] = gin.H{"byStatus": counts, "deadLettered": deadLetter}
	}
	if h.outbox != nil {
		pending, err := h.outbox.PendingCount(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "ERROR"})
			return
		}
		oldestAge, err := h.outbox.OldestPendingAge(ctx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "ERROR"})
			return
		}
		body["outbox"] = gin.H{"pending": pending, "oldestPendingAgeSeconds": oldestAge.Seconds()}
	}
	c.JSON(http.StatusOK, body)
}

type replayOutboxRequestBody struct {
	RecordID string `json:"recordId"`
}

// replayOutboxRecord returns exactly one dead-lettered outbox record to
// PENDING, by ID — never a bulk/blind "replay everything," mirroring
// retryPlatformJob's own per-record scope.
func (h *InternalHandler) replayOutboxRecord(c *gin.Context) {
	if !h.requireAllowedCaller(c) || !h.requireOutbox(c) {
		return
	}
	var body replayOutboxRequestBody
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.RecordID) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "REJECTED", "code": "record_id_required"})
		return
	}
	if err := h.outbox.ReplayDeadLettered(c.Request.Context(), body.RecordID); err != nil {
		if err == events.ErrOutboxRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"status": "NOT_FOUND", "code": "not_dead_lettered"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"status": "ERROR"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "REPLAYED", "recordId": body.RecordID})
}
