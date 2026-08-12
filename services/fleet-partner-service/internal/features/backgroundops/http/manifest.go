// Package http implements the Section 27 operator surface for background
// jobs and scheduled tasks: tenant-scoped read/manage/retry/cancel over
// fleet_partner_jobs and read/manage/run over fleet_partner_scheduled_tasks.
//
// Service-global (organization_id IS NULL) job operations -- the
// worker_operations.* permission family -- are NOT exposed through this
// package. The master specification requires those to sit behind "an
// existing internal/platform-operator authorization mechanism" distinct
// from ordinary organization admin permissions; no such mechanism exists
// anywhere in this codebase (confirmed by inspection: fleet_partner_roles
// are entirely organization-scoped, including the highest-privilege
// system-template roles), and inventing one is an accesscontrol/IAM
// architecture change outside Section 27's boundary. This is the same
// dependency-gating posture applied to the automation runner and to
// business-domain data retention: infrastructure that is safe and correct
// today ships now, and the precise missing dependency is reported rather
// than worked around with an invented pattern.
package http

// Route is one mounted Section 27 operator operation, following the
// established manifest convention used by every other feature's HTTP
// surface in this service.
type Route struct {
	Method      string
	Path        string
	OperationID string
	Permission  string
}

// CorporatePrefix is the tenant-scoped route-class prefix, matching the
// composition root's mounted group.
const CorporatePrefix = "/fleet-partner/v1"

// TenantRoutes is the corporate Section 27 background-jobs/scheduled-tasks
// operator surface.
func (h *Handler) TenantRoutes() []Route {
	return []Route{
		{Method: "GET", Path: "/jobs", OperationID: "listJobs", Permission: PermissionJobsRead},
		{Method: "GET", Path: "/jobs/:jobId", OperationID: "getJob", Permission: PermissionJobsRead},
		{Method: "GET", Path: "/jobs/:jobId/attempts", OperationID: "listJobAttempts", Permission: PermissionJobsRead},
		{Method: "POST", Path: "/jobs/:jobId/retry", OperationID: "retryJob", Permission: PermissionJobsRetry},
		{Method: "POST", Path: "/jobs/:jobId/cancel", OperationID: "cancelJob", Permission: PermissionJobsCancel},

		{Method: "GET", Path: "/scheduled-tasks", OperationID: "listScheduledTasks", Permission: PermissionScheduledTasksRead},
		{Method: "GET", Path: "/scheduled-tasks/:taskId", OperationID: "getScheduledTask", Permission: PermissionScheduledTasksRead},
		{Method: "GET", Path: "/scheduled-tasks/:taskId/runs", OperationID: "listScheduledTaskRuns", Permission: PermissionScheduledTasksRead},
		{Method: "POST", Path: "/scheduled-tasks/:taskId/enable", OperationID: "enableScheduledTask", Permission: PermissionScheduledTasksManage},
		{Method: "POST", Path: "/scheduled-tasks/:taskId/disable", OperationID: "disableScheduledTask", Permission: PermissionScheduledTasksManage},
		{Method: "POST", Path: "/scheduled-tasks/:taskId/run-now", OperationID: "runScheduledTaskNow", Permission: PermissionScheduledTasksRun},
	}
}

// FullPath returns the gin-style path as the router registers it.
func (r Route) FullPath() string { return CorporatePrefix + r.Path }
