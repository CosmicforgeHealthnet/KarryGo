package crm

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

const (
	TaskOpen       = "OPEN"
	TaskInProgress = "IN_PROGRESS"
	TaskCompleted  = "COMPLETED"
	TaskCancelled  = "CANCELLED"

	PriorityLow    = "LOW"
	PriorityNormal = "NORMAL"
	PriorityHigh   = "HIGH"
	PriorityUrgent = "URGENT"

	CodeTaskNotFound        = "CRM_TASK_NOT_FOUND"
	CodeTaskScopeDenied     = "CRM_TASK_SCOPE_DENIED"
	CodeTaskInvalidState    = "CRM_TASK_INVALID_STATE_TRANSITION"
	CodeTaskAssigneeInvalid = "CRM_TASK_ASSIGNEE_INVALID"
	CodeTaskVersionConflict = "CRM_TASK_VERSION_CONFLICT"
	CodeTaskAlreadyComplete = "CRM_TASK_ALREADY_COMPLETED"
	CodeTaskAlreadyCancel   = "CRM_TASK_ALREADY_CANCELLED"
)

var taskPriorities = map[string]bool{PriorityLow: true, PriorityNormal: true, PriorityHigh: true, PriorityUrgent: true}

// Task is an internal Vendor staff task about a relationship.
//
// It has exactly one subject: either an individual customer relationship or
// (from Section 8G) a corporate customer. The state machine, priorities,
// assignment rules, overdue derivation and immutable history below are shared
// by both — a second task state machine would be two places for OPEN,
// IN_PROGRESS, COMPLETED and CANCELLED to disagree.
type Task struct {
	ID, BusinessID, CustomerProfileID, OriginalProfileID  string
	BranchID                                              *string
	VisibilityScope, Title, Description, Status, Priority string
	AssignedStaffID                                       *string
	CreatedByStaffID                                      string
	DueAt, CompletedAt, CancelledAt                       *time.Time
	SourceInteractionID                                   *string
	DisplaySuppressed                                     bool
	CreatedAt, UpdatedAt                                  time.Time
	Version                                               int64
	AssigneeActive                                        *bool

	// Section 8G corporate subject. Nil on every individual task.
	CorporateCustomerID   *string
	CorporateContactID    *string
	CorporateLocationID   *string
	CorporateCostCenterID *string
	RelatedOrderRef       *string
	RelatedRFQRef         *string
	RelatedSupportCaseRef *string
	TriggerKey            *string
	TriggerVersion        *int
}

// IsCorporate reports whether this task is about an organization rather than an
// individual customer relationship.
func (t Task) IsCorporate() bool { return t.CorporateCustomerID != nil }

func (t Task) IsOverdue(now time.Time) bool {
	return t.DueAt != nil && (t.Status == TaskOpen || t.Status == TaskInProgress) && t.DueAt.Before(now)
}

type TaskView struct {
	TaskRef               string     `json:"task_ref"`
	CustomerProfileRef    string     `json:"customer_profile_ref"`
	BranchRef             *string    `json:"branch_ref,omitempty"`
	Visibility            string     `json:"visibility"`
	Title                 string     `json:"title"`
	Description           string     `json:"description"`
	Status                string     `json:"status"`
	Priority              string     `json:"priority"`
	AssignedStaffRef      *string    `json:"assigned_staff_ref,omitempty"`
	AssigneeActive        *bool      `json:"assignee_active,omitempty"`
	CreatedByStaffRef     string     `json:"created_by_staff_ref"`
	DueAt                 *time.Time `json:"due_at,omitempty"`
	CompletedAt           *time.Time `json:"completed_at,omitempty"`
	CancelledAt           *time.Time `json:"cancelled_at,omitempty"`
	SourceInteractionRef  *string    `json:"source_interaction_ref,omitempty"`
	IsOverdue             bool       `json:"is_overdue"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
	Version               int64      `json:"version"`
	RelatedOrderRef       *string    `json:"related_order_ref,omitempty"`
	RelatedRFQRef         *string    `json:"related_rfq_ref,omitempty"`
	RelatedSupportCaseRef *string    `json:"related_support_case_ref,omitempty"`
	TriggerKey            *string    `json:"trigger_key,omitempty"`
}

func TaskViewOf(t Task, now time.Time) TaskView {
	return TaskView{
		TaskRef: t.ID, CustomerProfileRef: t.CustomerProfileID, BranchRef: t.BranchID,
		Visibility: t.VisibilityScope, Title: t.Title, Description: t.Description,
		Status: t.Status, Priority: t.Priority, AssignedStaffRef: t.AssignedStaffID,
		AssigneeActive: t.AssigneeActive, CreatedByStaffRef: t.CreatedByStaffID, DueAt: t.DueAt,
		CompletedAt: t.CompletedAt, CancelledAt: t.CancelledAt, SourceInteractionRef: t.SourceInteractionID,
		IsOverdue: t.IsOverdue(now), CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, Version: t.Version,
		RelatedOrderRef: t.RelatedOrderRef, RelatedRFQRef: t.RelatedRFQRef, RelatedSupportCaseRef: t.RelatedSupportCaseRef, TriggerKey: t.TriggerKey,
	}
}

type TaskHistory struct {
	HistoryRef, TaskID, Action, Status, Priority, Title, Description string
	AssignedStaffID, ChangedByStaffID                                *string
	DueAt                                                            *time.Time
	TaskVersion                                                      int64
	OccurredAt                                                       time.Time
}

type TaskHistoryView struct {
	HistoryRef       string     `json:"history_ref"`
	Action           string     `json:"action"`
	Status           string     `json:"status"`
	Priority         string     `json:"priority"`
	AssignedStaffRef *string    `json:"assigned_staff_ref,omitempty"`
	DueAt            *time.Time `json:"due_at,omitempty"`
	Title            string     `json:"title"`
	Description      string     `json:"description"`
	Version          int64      `json:"version"`
	ChangedByRef     *string    `json:"changed_by_staff_ref,omitempty"`
	OccurredAt       time.Time  `json:"occurred_at"`
}

type CreateTaskInput struct {
	CustomerProfileRef, BranchRef, Visibility, Title, Description, Priority string
	AssignedStaffRef, SourceInteractionRef                                  string
	DueAt                                                                   *time.Time
	RelatedOrderRef, RelatedRFQRef, RelatedSupportCaseRef                   string
}

type UpdateTaskInput struct {
	Title, Description, Priority *string
	DueAt                        *time.Time
	ClearDueAt                   bool
	ExpectedVersion              int64
}

type ReassignTaskInput struct {
	AssignedStaffRef string
	ExpectedVersion  int64
}

type TaskListOptions struct {
	CustomerProfileRef, Status, Priority, AssignedStaffRef, BranchRef string
	DueBefore, DueAfter, CreatedFrom, CreatedTo                       *time.Time
	Overdue                                                           *bool
	Limit, Offset                                                     int

	// CorporateCustomerRef selects the corporate task list instead of the
	// individual one. Empty — every accepted Section 8C caller — restricts the
	// list to individual tasks.
	CorporateCustomerRef string
}

func (o TaskListOptions) bounded() TaskListOptions {
	if o.Limit <= 0 {
		o.Limit = 50
	}
	if o.Limit > 200 {
		o.Limit = 200
	}
	if o.Offset < 0 {
		o.Offset = 0
	}
	return o
}

// taskCols reads a task whatever its subject. As with interactions, the two
// individual columns are COALESCEd rather than made pointers so every accepted
// Section 8C call site keeps working unchanged.
const taskCols = `id,business_id,COALESCE(customer_profile_id,'') AS customer_profile_id,
COALESCE(original_profile_id,'') AS original_profile_id,branch_id,visibility_scope,
title,description,status,priority,assigned_staff_id,COALESCE(created_by_staff_id,''),due_at,completed_at,cancelled_at,
source_interaction_id,display_suppressed,created_at,updated_at,version,
corporate_customer_id,corporate_contact_id,corporate_location_id,corporate_cost_center_id,
related_order_ref,related_rfq_ref,related_support_case_ref,trigger_key,trigger_version`

func scanTask(row rowScanner) (Task, error) {
	var t Task
	err := row.Scan(&t.ID, &t.BusinessID, &t.CustomerProfileID, &t.OriginalProfileID, &t.BranchID, &t.VisibilityScope,
		&t.Title, &t.Description, &t.Status, &t.Priority, &t.AssignedStaffID, &t.CreatedByStaffID, &t.DueAt,
		&t.CompletedAt, &t.CancelledAt, &t.SourceInteractionID, &t.DisplaySuppressed, &t.CreatedAt, &t.UpdatedAt, &t.Version,
		&t.CorporateCustomerID, &t.CorporateContactID, &t.CorporateLocationID, &t.CorporateCostCenterID,
		&t.RelatedOrderRef, &t.RelatedRFQRef, &t.RelatedSupportCaseRef, &t.TriggerKey, &t.TriggerVersion)
	return t, err
}

func scanTaskRead(row rowScanner) (Task, error) {
	t, err := scanTaskWithExtra(row, true)
	return t, err
}

func scanTaskWithExtra(row rowScanner, active bool) (Task, error) {
	var t Task
	if active {
		err := row.Scan(&t.ID, &t.BusinessID, &t.CustomerProfileID, &t.OriginalProfileID, &t.BranchID, &t.VisibilityScope,
			&t.Title, &t.Description, &t.Status, &t.Priority, &t.AssignedStaffID, &t.CreatedByStaffID, &t.DueAt,
			&t.CompletedAt, &t.CancelledAt, &t.SourceInteractionID, &t.DisplaySuppressed, &t.CreatedAt, &t.UpdatedAt, &t.Version,
			&t.CorporateCustomerID, &t.CorporateContactID, &t.CorporateLocationID, &t.CorporateCostCenterID,
			&t.RelatedOrderRef, &t.RelatedRFQRef, &t.RelatedSupportCaseRef, &t.TriggerKey, &t.TriggerVersion,
			&t.AssigneeActive)
		return t, err
	}
	return scanTask(row)
}

func taskReadCols(alias string) string {
	return taskCols + `,CASE WHEN ` + alias + `.assigned_staff_id IS NULL THEN NULL ELSE
		EXISTS(SELECT 1 FROM vendor_staff vs WHERE vs.id=` + alias + `.assigned_staff_id AND vs.vendor_id=` + alias + `.business_id AND vs.status='active') END`
}

func taskNotFound() error {
	return crmError(apperrors.NotFound("Task not found.", nil), CodeTaskNotFound)
}
func taskValidation(field, message, code string) error {
	err := apperrors.Validation("Check your task details.", []apperrors.FieldViolation{{Field: field, Message: message}})
	if code != "" {
		err.Details = map[string]any{"code": code}
	}
	return err
}

func validateTaskInput(in CreateTaskInput) (CreateTaskInput, error) {
	in.Visibility = strings.ToUpper(strings.TrimSpace(in.Visibility))
	if in.Visibility == "" {
		in.Visibility = VisibilityBusiness
	}
	in.Priority = strings.ToUpper(strings.TrimSpace(in.Priority))
	if in.Priority == "" {
		in.Priority = PriorityNormal
	}
	if in.Visibility != VisibilityBusiness && in.Visibility != VisibilityBranch {
		return in, taskValidation("visibility", "Visibility must be BUSINESS or BRANCH.", CodeTaskScopeDenied)
	}
	if in.Visibility == VisibilityBranch && strings.TrimSpace(in.BranchRef) == "" {
		return in, taskValidation("branch_ref", "A branch is required.", CodeTaskScopeDenied)
	}
	if in.Visibility == VisibilityBusiness && strings.TrimSpace(in.BranchRef) != "" {
		return in, taskValidation("branch_ref", "A business task cannot name a branch.", CodeTaskScopeDenied)
	}
	if !taskPriorities[in.Priority] {
		return in, taskValidation("priority", "Unsupported task priority.", CodeTaskInvalidState)
	}
	var err error
	if in.Title, err = normalizePlainText(in.Title, 200, true, true, "title"); err != nil {
		return in, err
	}
	if in.Description, err = normalizePlainText(in.Description, 5000, false, false, "description"); err != nil {
		return in, err
	}
	if in.DueAt != nil {
		value := in.DueAt.UTC()
		in.DueAt = &value
	}
	return in, nil
}

func (s *Service) assertTaskAssignee(ctx context.Context, tx pgx.Tx, businessID string, assignee *string, branchID *string) error {
	if assignee == nil {
		return nil
	}
	var eligible bool
	if branchID == nil {
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_staff s
			WHERE s.id=$1 AND s.vendor_id=$2 AND s.status='active' AND EXISTS(
			 SELECT 1 FROM vendor_staff_role_assignments a WHERE a.staff_id=s.id AND a.revoked_at IS NULL AND a.branch_id IS NULL))`,
			*assignee, businessID).Scan(&eligible); err != nil {
			return err
		}
	} else {
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vendor_staff s
			WHERE s.id=$1 AND s.vendor_id=$2 AND s.status='active' AND EXISTS(
			 SELECT 1 FROM vendor_staff_role_assignments a WHERE a.staff_id=s.id AND a.revoked_at IS NULL
			 AND (a.branch_id IS NULL OR a.branch_id=$3)))`, *assignee, businessID, *branchID).Scan(&eligible); err != nil {
			return err
		}
	}
	if !eligible {
		return crmError(apperrors.Validation("The selected assignee is not available for this task scope.", nil), CodeTaskAssigneeInvalid)
	}
	return nil
}

func (s *Service) assertSourceInteraction(ctx context.Context, tx pgx.Tx, actor Actor, profileID, sourceID, visibility string, branchID *string) error {
	if sourceID == "" {
		return nil
	}
	v, err := scanInteraction(tx.QueryRow(ctx, `SELECT `+interactionCols+` FROM crm_interactions
		WHERE business_id=$1 AND id=$2 AND display_suppressed=false`, actor.VendorID, sourceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return interactionNotFound()
	}
	if err != nil {
		return err
	}
	if v.CustomerProfileID != profileID || v.VisibilityScope != visibility || derefOr(v.BranchID) != derefOr(branchID) {
		return crmError(apperrors.NotFound("Source interaction not found.", nil), CodeInteractionNotFound)
	}
	return nil
}

func (s *Service) CreateTask(ctx context.Context, actor Actor, scope Scope, in CreateTaskInput) (Task, error) {
	if !s.flags.TasksEnabled || !s.flags.WritesEnabled || s.flags.CommerceKillSwitch {
		return Task{}, errWritesDisabled()
	}
	var err error
	if in, err = validateTaskInput(in); err != nil {
		return Task{}, err
	}
	var branchID, assignee, source *string
	if in.Visibility == VisibilityBranch {
		value := strings.TrimSpace(in.BranchRef)
		branchID = &value
	}
	if value := strings.TrimSpace(in.AssignedStaffRef); value != "" {
		assignee = &value
	}
	if value := strings.TrimSpace(in.SourceInteractionRef); value != "" {
		source = &value
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Task{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	profileID := strings.TrimSpace(in.CustomerProfileRef)
	if _, err = s.lockActionableProfile(ctx, tx, actor, scope, profileID, in.Visibility, branchID); err != nil {
		return Task{}, err
	}
	if err = s.assertTaskAssignee(ctx, tx, actor.VendorID, assignee, branchID); err != nil {
		return Task{}, err
	}
	if source != nil {
		if err = s.assertSourceInteraction(ctx, tx, actor, profileID, *source, in.Visibility, branchID); err != nil {
			return Task{}, err
		}
	}
	task, err := scanTask(tx.QueryRow(ctx, `INSERT INTO crm_tasks
		(id,business_id,customer_profile_id,original_profile_id,branch_id,visibility_scope,title,description,
		 status,priority,assigned_staff_id,created_by_staff_id,due_at,source_interaction_id,related_order_ref,related_rfq_ref,related_support_case_ref)
		VALUES($1,$2,$3,$3,$4,$5,$6,$7,'OPEN',$8,$9,$10,$11,$12,$13,$14,$15) RETURNING `+taskCols,
		newID("crmt_"), actor.VendorID, profileID, branchID, in.Visibility, in.Title, in.Description, in.Priority, assignee, actor.StaffID, in.DueAt, source, optional(in.RelatedOrderRef), optional(in.RelatedRFQRef), optional(in.RelatedSupportCaseRef)))
	if err != nil {
		return Task{}, err
	}
	if assignee != nil {
		yes := true
		task.AssigneeActive = &yes
	}
	if err = s.appendTaskHistory(ctx, tx, actor, task, "CREATED"); err != nil {
		return Task{}, err
	}
	if err = s.auditTask(ctx, tx, actor, "vendor.crm.task.created", Task{}, task); err != nil {
		return Task{}, err
	}
	if err = s.publishTask(ctx, tx, actor, "vendor.crm.task.created", task); err != nil {
		return Task{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Task{}, err
	}
	s.metrics.Inc("crm_task_create_total", "priority", task.Priority)
	return task, nil
}

func (s *Service) appendTaskHistory(ctx context.Context, tx pgx.Tx, actor Actor, t Task, action string) error {
	var changedBy *string
	if actor.StaffID != "" {
		changedBy = &actor.StaffID
	}
	_, err := tx.Exec(ctx, `INSERT INTO crm_task_history
		(id,business_id,task_id,action,status,priority,assigned_staff_id,due_at,title,description,task_version,changed_by_staff_id)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, newID("crmth_"), t.BusinessID, t.ID, action, t.Status, t.Priority,
		t.AssignedStaffID, t.DueAt, t.Title, t.Description, t.Version, changedBy)
	return err
}

func safeTaskAudit(t Task) map[string]any {
	if t.ID == "" {
		return nil
	}
	return map[string]any{"task_ref": t.ID, "profile_ref": t.CustomerProfileID, "branch_ref": t.BranchID, "status": t.Status,
		"priority": t.Priority, "assigned_staff_ref": t.AssignedStaffID, "due_at": t.DueAt, "version": t.Version}
}
func (s *Service) auditTask(ctx context.Context, tx pgx.Tx, actor Actor, action string, before, after Task) error {
	return s.auditTx(ctx, tx, actor, action, "crm_task", after.ID, derefOr(after.BranchID), safeTaskAudit(before), safeTaskAudit(after), "")
}
func (s *Service) publishTask(ctx context.Context, tx pgx.Tx, actor Actor, event string, t Task) error {
	return s.publishTx(ctx, tx, actor, event, "crm_task", t.ID, t.Version, map[string]any{"task_ref": t.ID, "customer_profile_ref": t.CustomerProfileID,
		"branch_ref": t.BranchID, "status": t.Status, "priority": t.Priority, "assigned_staff_ref": t.AssignedStaffID, "due_at": t.DueAt, "version": t.Version})
}

func (s *Service) ListTasks(ctx context.Context, actor Actor, scope Scope, opts TaskListOptions) ([]Task, bool, error) {
	opts = opts.bounded()
	args := []any{actor.VendorID}
	where := []string{"t.business_id=$1", "t.display_suppressed=false"}
	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, strings.Replace(clause, "?", "$"+itoa(len(args)), 1))
	}
	// The subject is always pinned, so a corporate task can never surface in an
	// individual task list. The branch rule differs by subject: an individual
	// task is reached through its own visibility scope, a corporate task
	// through the corporate account's branch scope.
	if opts.CorporateCustomerRef != "" {
		add("t.corporate_customer_id=?", opts.CorporateCustomerRef)
	} else {
		where = append(where, "t.corporate_customer_id IS NULL", activityScopeSQL("t", scope, &args))
	}
	if opts.CustomerProfileRef != "" {
		add("t.customer_profile_id=?", opts.CustomerProfileRef)
	}
	if opts.Status != "" {
		value := strings.ToUpper(opts.Status)
		if value != TaskOpen && value != TaskInProgress && value != TaskCompleted && value != TaskCancelled {
			return nil, false, taskValidation("status", "Unsupported task status.", CodeTaskInvalidState)
		}
		add("t.status=?", value)
	}
	if opts.Priority != "" {
		value := strings.ToUpper(opts.Priority)
		if !taskPriorities[value] {
			return nil, false, taskValidation("priority", "Unsupported task priority.", CodeTaskInvalidState)
		}
		add("t.priority=?", value)
	}
	if opts.AssignedStaffRef != "" {
		add("t.assigned_staff_id=?", opts.AssignedStaffRef)
	}
	if opts.BranchRef != "" {
		if !scope.Global && !containsString(scope.BranchIDs, opts.BranchRef) {
			return nil, false, taskNotFound()
		}
		add("t.branch_id=?", opts.BranchRef)
	}
	if opts.DueBefore != nil {
		add("t.due_at<=?", opts.DueBefore.UTC())
	}
	if opts.DueAfter != nil {
		add("t.due_at>=?", opts.DueAfter.UTC())
	}
	if opts.CreatedFrom != nil {
		add("t.created_at>=?", opts.CreatedFrom.UTC())
	}
	if opts.CreatedTo != nil {
		add("t.created_at<=?", opts.CreatedTo.UTC())
	}
	if opts.Overdue != nil {
		if *opts.Overdue {
			args = append(args, s.now())
			where = append(where, "t.status IN ('OPEN','IN_PROGRESS') AND t.due_at < $"+itoa(len(args)))
		} else {
			args = append(args, s.now())
			where = append(where, "NOT (t.status IN ('OPEN','IN_PROGRESS') AND t.due_at < $"+itoa(len(args))+")")
		}
	}
	args = append(args, opts.Limit+1, opts.Offset)
	rows, err := s.db.Query(ctx, `SELECT `+taskReadCols("t")+` FROM crm_tasks t WHERE `+strings.Join(where, " AND ")+`
		ORDER BY t.due_at ASC NULLS LAST,t.created_at DESC,t.id LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)), args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := make([]Task, 0, opts.Limit)
	for rows.Next() {
		t, scanErr := scanTaskRead(rows)
		if scanErr != nil {
			return nil, false, scanErr
		}
		items = append(items, t)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(items) > opts.Limit
	if more {
		items = items[:opts.Limit]
	}
	return items, more, nil
}

// ListOpenTasksForProfile is the bounded Customer 360 read. The count and
// items share the exact same SQL scope predicate so pagination metadata cannot
// reveal business-wide or another branch's task volume.
func (s *Service) ListOpenTasksForProfile(ctx context.Context, actor Actor, scope Scope, profileID string, limit int) ([]Task, int, error) {
	if limit <= 0 || limit > 20 {
		limit = 5
	}
	args := []any{actor.VendorID, profileID}
	where := []string{"t.business_id=$1", "t.customer_profile_id=$2", "t.corporate_customer_id IS NULL",
		"t.display_suppressed=false", "t.status IN ('OPEN','IN_PROGRESS')", activityScopeSQL("t", scope, &args)}
	var count int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM crm_tasks t WHERE `+strings.Join(where, " AND "), args...).Scan(&count); err != nil {
		return nil, 0, err
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+taskReadCols("t")+` FROM crm_tasks t WHERE `+strings.Join(where, " AND ")+`
		ORDER BY t.due_at ASC NULLS LAST,t.created_at DESC,t.id LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]Task, 0, limit)
	for rows.Next() {
		task, scanErr := scanTaskRead(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, task)
	}
	return items, count, rows.Err()
}

// GetTask reads one INDIVIDUAL task. A corporate task is reported not found
// here and is reachable only through its corporate account.
func (s *Service) GetTask(ctx context.Context, actor Actor, scope Scope, id string) (Task, error) {
	t, err := scanTaskRead(s.db.QueryRow(ctx, `SELECT `+taskReadCols("t")+` FROM crm_tasks t
		WHERE business_id=$1 AND id=$2 AND display_suppressed=false AND corporate_customer_id IS NULL`,
		actor.VendorID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, taskNotFound()
	}
	if err != nil {
		return Task{}, err
	}
	if authorizeActivityScope(scope, t.VisibilityScope, t.BranchID) != nil {
		return Task{}, taskNotFound()
	}
	return t, nil
}

// taskAnySubject resolves a task whichever kind of subject it has, authorising
// each kind by its own rule.
func (s *Service) taskAnySubject(ctx context.Context, actor Actor, scope Scope, id string) (Task, error) {
	t, err := scanTaskRead(s.db.QueryRow(ctx, `SELECT `+taskReadCols("t")+` FROM crm_tasks t
		WHERE business_id=$1 AND id=$2 AND display_suppressed=false`, actor.VendorID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, taskNotFound()
	}
	if err != nil {
		return Task{}, err
	}
	if t.IsCorporate() {
		if err = s.authorizeCorporateActivity(ctx, s.db, scope, actor.VendorID, *t.CorporateCustomerID); err != nil {
			return Task{}, taskNotFound()
		}
		return t, nil
	}
	if authorizeActivityScope(scope, t.VisibilityScope, t.BranchID) != nil {
		return Task{}, taskNotFound()
	}
	return t, nil
}

func (s *Service) taskForMutation(ctx context.Context, tx pgx.Tx, actor Actor, scope Scope, id string) (Task, error) {
	probe, err := scanTask(tx.QueryRow(ctx, `SELECT `+taskCols+` FROM crm_tasks WHERE business_id=$1 AND id=$2`, actor.VendorID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, taskNotFound()
	}
	if err != nil {
		return Task{}, err
	}
	// The fence follows the subject: an individual task locks the customer's
	// profile row, a corporate task locks the corporate account. Both give the
	// same guarantee — whatever else is happening to the thing this task is
	// about, it serializes against this mutation.
	if probe.IsCorporate() {
		if _, err = s.corporateForWrite(ctx, tx, actor, scope, *probe.CorporateCustomerID); err != nil {
			return Task{}, err
		}
	} else if _, err = s.lockActionableProfile(ctx, tx, actor, scope, probe.CustomerProfileID, probe.VisibilityScope, probe.BranchID); err != nil {
		return Task{}, err
	}
	current, err := scanTask(tx.QueryRow(ctx, `SELECT `+taskCols+` FROM crm_tasks WHERE business_id=$1 AND id=$2 AND display_suppressed=false FOR UPDATE`, actor.VendorID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, taskNotFound()
	}
	return current, err
}

func (s *Service) UpdateTask(ctx context.Context, actor Actor, scope Scope, id string, in UpdateTaskInput) (Task, error) {
	if !s.flags.TasksEnabled || !s.flags.WritesEnabled || s.flags.CommerceKillSwitch {
		return Task{}, errWritesDisabled()
	}
	if in.ExpectedVersion < 1 {
		return Task{}, taskValidation("expected_version", "Expected version is required.", CodeTaskVersionConflict)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Task{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	current, err := s.taskForMutation(ctx, tx, actor, scope, id)
	if err != nil {
		return Task{}, err
	}
	if current.Version != in.ExpectedVersion {
		return Task{}, crmError(apperrors.Conflict("Task changed since it was loaded.", nil), CodeTaskVersionConflict)
	}
	if current.Status == TaskCompleted || current.Status == TaskCancelled {
		return Task{}, crmError(apperrors.Conflict("A terminal task cannot be edited.", nil), CodeTaskInvalidState)
	}
	updated := current
	if in.Title != nil {
		updated.Title, err = normalizePlainText(*in.Title, 200, true, true, "title")
	}
	if err == nil && in.Description != nil {
		updated.Description, err = normalizePlainText(*in.Description, 5000, false, false, "description")
	}
	if err == nil && in.Priority != nil {
		value := strings.ToUpper(strings.TrimSpace(*in.Priority))
		if !taskPriorities[value] {
			err = taskValidation("priority", "Unsupported task priority.", CodeTaskInvalidState)
		} else {
			updated.Priority = value
		}
	}
	if in.ClearDueAt {
		updated.DueAt = nil
	} else if in.DueAt != nil {
		value := in.DueAt.UTC()
		updated.DueAt = &value
	}
	if err != nil {
		return Task{}, err
	}
	updated, err = scanTask(tx.QueryRow(ctx, `UPDATE crm_tasks SET title=$1,description=$2,priority=$3,due_at=$4,updated_at=now(),version=version+1
		WHERE business_id=$5 AND id=$6 AND version=$7 RETURNING `+taskCols, updated.Title, updated.Description, updated.Priority, updated.DueAt, actor.VendorID, id, in.ExpectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, crmError(apperrors.Conflict("Task changed since it was loaded.", nil), CodeTaskVersionConflict)
	}
	if err != nil {
		return Task{}, err
	}
	if err = s.appendTaskHistory(ctx, tx, actor, updated, "UPDATED"); err != nil {
		return Task{}, err
	}
	if err = s.auditTask(ctx, tx, actor, "vendor.crm.task.updated", current, updated); err != nil {
		return Task{}, err
	}
	if err = s.publishTask(ctx, tx, actor, "vendor.crm.task.updated", updated); err != nil {
		return Task{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Task{}, err
	}
	return updated, nil
}

func (s *Service) TransitionTask(ctx context.Context, actor Actor, scope Scope, id, target string, expectedVersion int64) (Task, error) {
	if !s.flags.TasksEnabled || !s.flags.WritesEnabled || s.flags.CommerceKillSwitch {
		return Task{}, errWritesDisabled()
	}
	if expectedVersion < 1 {
		return Task{}, taskValidation("expected_version", "Expected version is required.", CodeTaskVersionConflict)
	}
	target = strings.ToUpper(strings.TrimSpace(target))
	if target != TaskInProgress && target != TaskCompleted && target != TaskCancelled {
		return Task{}, taskValidation("status", "Unsupported task transition.", CodeTaskInvalidState)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Task{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	current, err := s.taskForMutation(ctx, tx, actor, scope, id)
	if err != nil {
		return Task{}, err
	}
	// Repeated delivery of the same terminal/start command returns the settled
	// record even when the caller still carries its pre-transition version.
	if current.Status == target {
		if err = tx.Commit(ctx); err != nil {
			return Task{}, err
		}
		return current, nil
	}
	if current.Version != expectedVersion {
		return Task{}, crmError(apperrors.Conflict("Task changed since it was loaded.", nil), CodeTaskVersionConflict)
	}
	if current.Status == TaskCompleted {
		return Task{}, crmError(apperrors.Conflict("Task is already completed.", nil), CodeTaskAlreadyComplete)
	}
	if current.Status == TaskCancelled {
		return Task{}, crmError(apperrors.Conflict("Task is already cancelled.", nil), CodeTaskAlreadyCancel)
	}
	if target == TaskInProgress && current.Status != TaskOpen {
		return Task{}, crmError(apperrors.Conflict("Task cannot be started from its current state.", nil), CodeTaskInvalidState)
	}
	var completed, cancelled *time.Time
	now := s.now().UTC()
	if target == TaskCompleted {
		completed = &now
	}
	if target == TaskCancelled {
		cancelled = &now
	}
	updated, err := scanTask(tx.QueryRow(ctx, `UPDATE crm_tasks SET status=$1,completed_at=$2,cancelled_at=$3,updated_at=now(),version=version+1
		WHERE business_id=$4 AND id=$5 AND version=$6 RETURNING `+taskCols, target, completed, cancelled, actor.VendorID, id, expectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, crmError(apperrors.Conflict("Task changed since it was loaded.", nil), CodeTaskVersionConflict)
	}
	if err != nil {
		return Task{}, err
	}
	action := map[string]string{TaskInProgress: "STARTED", TaskCompleted: "COMPLETED", TaskCancelled: "CANCELLED"}[target]
	event := map[string]string{TaskInProgress: "vendor.crm.task.started", TaskCompleted: "vendor.crm.task.completed", TaskCancelled: "vendor.crm.task.cancelled"}[target]
	if err = s.appendTaskHistory(ctx, tx, actor, updated, action); err != nil {
		return Task{}, err
	}
	if err = s.auditTask(ctx, tx, actor, event, current, updated); err != nil {
		return Task{}, err
	}
	if err = s.publishTask(ctx, tx, actor, event, updated); err != nil {
		return Task{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Task{}, err
	}
	s.metrics.Inc("crm_task_transition_total", "status", target)
	return updated, nil
}

func (s *Service) ReassignTask(ctx context.Context, actor Actor, scope Scope, id string, in ReassignTaskInput) (Task, error) {
	if !s.flags.TasksEnabled || !s.flags.WritesEnabled || s.flags.CommerceKillSwitch {
		return Task{}, errWritesDisabled()
	}
	if in.ExpectedVersion < 1 {
		return Task{}, taskValidation("expected_version", "Expected version is required.", CodeTaskVersionConflict)
	}
	var assignee *string
	if value := strings.TrimSpace(in.AssignedStaffRef); value != "" {
		assignee = &value
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Task{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	current, err := s.taskForMutation(ctx, tx, actor, scope, id)
	if err != nil {
		return Task{}, err
	}
	if current.Version != in.ExpectedVersion {
		return Task{}, crmError(apperrors.Conflict("Task changed since it was loaded.", nil), CodeTaskVersionConflict)
	}
	if current.Status == TaskCompleted || current.Status == TaskCancelled {
		return Task{}, crmError(apperrors.Conflict("A terminal task cannot be reassigned.", nil), CodeTaskInvalidState)
	}
	if err = s.assertTaskAssignee(ctx, tx, actor.VendorID, assignee, current.BranchID); err != nil {
		return Task{}, err
	}
	updated, err := scanTask(tx.QueryRow(ctx, `UPDATE crm_tasks SET assigned_staff_id=$1,updated_at=now(),version=version+1 WHERE business_id=$2 AND id=$3 AND version=$4 RETURNING `+taskCols, assignee, actor.VendorID, id, in.ExpectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, crmError(apperrors.Conflict("Task changed since it was loaded.", nil), CodeTaskVersionConflict)
	}
	if err != nil {
		return Task{}, err
	}
	if assignee != nil {
		yes := true
		updated.AssigneeActive = &yes
	}
	if err = s.appendTaskHistory(ctx, tx, actor, updated, "REASSIGNED"); err != nil {
		return Task{}, err
	}
	if err = s.auditTask(ctx, tx, actor, "vendor.crm.task.reassigned", current, updated); err != nil {
		return Task{}, err
	}
	if err = s.publishTask(ctx, tx, actor, "vendor.crm.task.reassigned", updated); err != nil {
		return Task{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Task{}, err
	}
	return updated, nil
}

func (s *Service) ListTaskHistory(ctx context.Context, actor Actor, scope Scope, id string) ([]TaskHistory, error) {
	// Subject-agnostic: immutable task activity reads the same way for a
	// corporate task as for an individual one.
	if _, err := s.taskAnySubject(ctx, actor, scope, id); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT id,task_id,action,status,priority,assigned_staff_id,due_at,title,description,task_version,changed_by_staff_id,occurred_at
		FROM crm_task_history WHERE business_id=$1 AND task_id=$2 ORDER BY task_version`, actor.VendorID, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []TaskHistory
	for rows.Next() {
		var h TaskHistory
		if err = rows.Scan(&h.HistoryRef, &h.TaskID, &h.Action, &h.Status, &h.Priority, &h.AssignedStaffID, &h.DueAt, &h.Title, &h.Description, &h.TaskVersion, &h.ChangedByStaffID, &h.OccurredAt); err != nil {
			return nil, err
		}
		items = append(items, h)
	}
	return items, rows.Err()
}
