package crm

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

const (
	PermInteractionsRead   = "vendor.crm.interactions.read"
	PermInteractionsCreate = "vendor.crm.interactions.create"
	PermInteractionsManage = "vendor.crm.interactions.manage"
	PermTasksRead          = "vendor.crm.tasks.read"
	PermTasksCreate        = "vendor.crm.tasks.create"
	PermTasksManage        = "vendor.crm.tasks.manage"
)

const (
	activityCreateRateMax   = 60
	activityMutationRateMax = 120
	activityRateWindow      = time.Minute
)

// ActivityRateLimiter is the existing Redis-backed platform limiter boundary.
// It remains a port here so the CRM domain does not import platform wiring.
type ActivityRateLimiter interface {
	Allow(context.Context, string, string, int, time.Duration) error
}

// ActivityHandler reuses the established command/idempotency and auth edge
// from Section 8B without changing that accepted handler.
type ActivityHandler struct {
	*SegmentationHandler
	limiter ActivityRateLimiter
}

func NewActivityHandler(service *Service, store *idempotency.Store, limiters ...ActivityRateLimiter) *ActivityHandler {
	var limiter ActivityRateLimiter
	if len(limiters) > 0 {
		limiter = limiters[0]
	}
	return &ActivityHandler{SegmentationHandler: NewSegmentationHandler(service, store), limiter: limiter}
}

func (h *ActivityHandler) rateLimit(ctx context.Context, actor Actor, bucket string, max int) error {
	if h.limiter == nil {
		return nil
	}
	return h.limiter.Allow(ctx, bucket, actor.VendorID+":"+actor.StaffID, max, activityRateWindow)
}

type createInteractionRequest struct {
	CustomerProfileRef    string    `json:"customer_profile_ref"`
	BranchRef             string    `json:"branch_ref,omitempty"`
	Type                  string    `json:"type"`
	Direction             string    `json:"direction"`
	Outcome               string    `json:"outcome"`
	Subject               string    `json:"subject"`
	Note                  string    `json:"note"`
	Visibility            string    `json:"visibility"`
	OccurredAt            time.Time `json:"occurred_at"`
	RelatedOrderRef       string    `json:"related_order_ref,omitempty"`
	RelatedRFQRef         string    `json:"related_rfq_ref,omitempty"`
	RelatedSupportCaseRef string    `json:"related_support_case_ref,omitempty"`
}
type correctInteractionRequest struct {
	Subject          *string    `json:"subject,omitempty"`
	Note             *string    `json:"note,omitempty"`
	Outcome          *string    `json:"outcome,omitempty"`
	OccurredAt       *time.Time `json:"occurred_at,omitempty"`
	CorrectionReason string     `json:"correction_reason"`
	ExpectedVersion  int64      `json:"expected_version"`
}

func (h *ActivityHandler) CreateInteraction(c *gin.Context) {
	h.command(c, http.StatusCreated, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermInteractionsCreate)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_interaction_create", activityCreateRateMax); err != nil {
			return nil, err
		}
		var req createInteractionRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		v, err := h.service.CreateInteraction(c, actor, scope, CreateInteractionInput{
			CustomerProfileRef: req.CustomerProfileRef, BranchRef: req.BranchRef, Type: req.Type, Direction: req.Direction, Outcome: req.Outcome,
			Subject: req.Subject, Note: req.Note, Visibility: req.Visibility, OccurredAt: req.OccurredAt, RelatedOrderRef: req.RelatedOrderRef, RelatedRFQRef: req.RelatedRFQRef, RelatedSupportCaseRef: req.RelatedSupportCaseRef})
		if err != nil {
			return nil, err
		}
		return InteractionViewOf(v), nil
	})
}
func (h *ActivityHandler) CorrectInteraction(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermInteractionsManage)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_interaction_correct", activityMutationRateMax); err != nil {
			return nil, err
		}
		var req correctInteractionRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		v, err := h.service.CorrectInteraction(c, actor, scope, c.Param("interactionId"), CorrectInteractionInput{Subject: req.Subject, Note: req.Note, Outcome: req.Outcome, OccurredAt: req.OccurredAt, CorrectionReason: req.CorrectionReason, ExpectedVersion: req.ExpectedVersion})
		if err != nil {
			return nil, err
		}
		return InteractionViewOf(v), nil
	})
}
func (h *ActivityHandler) GetInteraction(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermInteractionsRead)
	if !ok {
		return
	}
	v, err := h.service.GetInteraction(c, actor, scope, c.Param("interactionId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	okResponse(c, InteractionViewOf(v))
}
func (h *ActivityHandler) ListInteractions(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermInteractionsRead)
	if !ok {
		return
	}
	opts, err := interactionOptionsFrom(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	h.listInteractions(c, actor, scope, opts)
}
func (h *ActivityHandler) ListCustomerInteractions(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermInteractionsRead)
	if !ok {
		return
	}
	if _, err := h.service.GetProfile(c, actor, scope, c.Param("profileId")); err != nil {
		httpx.RespondError(c, err)
		return
	}
	opts, err := interactionOptionsFrom(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	opts.CustomerProfileRef = c.Param("profileId")
	h.listInteractions(c, actor, scope, opts)
}
func (h *ActivityHandler) listInteractions(c *gin.Context, actor Actor, scope Scope, opts InteractionListOptions) {
	items, more, err := h.service.ListInteractions(c, actor, scope, opts)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	views := make([]InteractionView, 0, len(items))
	for _, v := range items {
		views = append(views, InteractionViewOf(v))
	}
	opts = opts.bounded()
	okResponse(c, gin.H{"items": views, "pagination": gin.H{"limit": opts.Limit, "offset": opts.Offset, "has_more": more}})
}
func (h *ActivityHandler) ListInteractionRevisions(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermInteractionsRead)
	if !ok {
		return
	}
	items, err := h.service.ListInteractionRevisions(c, actor, scope, c.Param("interactionId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	views := make([]InteractionRevisionView, 0, len(items))
	for _, v := range items {
		views = append(views, InteractionRevisionView{RevisionRef: v.RevisionRef, Version: v.RevisionVersion, Subject: v.Subject, Note: v.NoteBody, Outcome: v.Outcome, OccurredAt: v.OccurredAt, CorrectionReason: v.CorrectionReason, ChangedByRef: v.ChangedByStaffID, CreatedAt: v.CreatedAt})
	}
	okResponse(c, gin.H{"items": views})
}

type createTaskRequest struct {
	CustomerProfileRef    string     `json:"customer_profile_ref"`
	BranchRef             string     `json:"branch_ref,omitempty"`
	Visibility            string     `json:"visibility"`
	Title                 string     `json:"title"`
	Description           string     `json:"description"`
	Priority              string     `json:"priority"`
	AssignedStaffRef      string     `json:"assigned_staff_ref,omitempty"`
	DueAt                 *time.Time `json:"due_at,omitempty"`
	SourceInteractionRef  string     `json:"source_interaction_ref,omitempty"`
	RelatedOrderRef       string     `json:"related_order_ref,omitempty"`
	RelatedRFQRef         string     `json:"related_rfq_ref,omitempty"`
	RelatedSupportCaseRef string     `json:"related_support_case_ref,omitempty"`
}
type updateTaskRequest struct {
	Title           *string    `json:"title,omitempty"`
	Description     *string    `json:"description,omitempty"`
	Priority        *string    `json:"priority,omitempty"`
	DueAt           *time.Time `json:"due_at,omitempty"`
	ClearDueAt      bool       `json:"clear_due_at,omitempty"`
	ExpectedVersion int64      `json:"expected_version"`
}
type versionRequest struct {
	ExpectedVersion int64 `json:"expected_version"`
}
type reassignTaskRequest struct {
	AssignedStaffRef string `json:"assigned_staff_ref,omitempty"`
	ExpectedVersion  int64  `json:"expected_version"`
}

func (h *ActivityHandler) CreateTask(c *gin.Context) {
	h.command(c, http.StatusCreated, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermTasksCreate)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_task_create", activityCreateRateMax); err != nil {
			return nil, err
		}
		var req createTaskRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		v, err := h.service.CreateTask(c, actor, scope, CreateTaskInput{CustomerProfileRef: req.CustomerProfileRef, BranchRef: req.BranchRef, Visibility: req.Visibility, Title: req.Title, Description: req.Description, Priority: req.Priority, AssignedStaffRef: req.AssignedStaffRef, DueAt: req.DueAt, SourceInteractionRef: req.SourceInteractionRef, RelatedOrderRef: req.RelatedOrderRef, RelatedRFQRef: req.RelatedRFQRef, RelatedSupportCaseRef: req.RelatedSupportCaseRef})
		if err != nil {
			return nil, err
		}
		return TaskViewOf(v, h.service.now()), nil
	})
}
func (h *ActivityHandler) UpdateTask(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermTasksManage)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_task_mutation", activityMutationRateMax); err != nil {
			return nil, err
		}
		var req updateTaskRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		v, err := h.service.UpdateTask(c, actor, scope, c.Param("taskId"), UpdateTaskInput{Title: req.Title, Description: req.Description, Priority: req.Priority, DueAt: req.DueAt, ClearDueAt: req.ClearDueAt, ExpectedVersion: req.ExpectedVersion})
		if err != nil {
			return nil, err
		}
		return TaskViewOf(v, h.service.now()), nil
	})
}
func (h *ActivityHandler) transition(c *gin.Context, target string) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermTasksManage)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_task_mutation", activityMutationRateMax); err != nil {
			return nil, err
		}
		var req versionRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		v, err := h.service.TransitionTask(c, actor, scope, c.Param("taskId"), target, req.ExpectedVersion)
		if err != nil {
			return nil, err
		}
		return TaskViewOf(v, h.service.now()), nil
	})
}
func (h *ActivityHandler) StartTask(c *gin.Context)    { h.transition(c, TaskInProgress) }
func (h *ActivityHandler) CompleteTask(c *gin.Context) { h.transition(c, TaskCompleted) }
func (h *ActivityHandler) CancelTask(c *gin.Context)   { h.transition(c, TaskCancelled) }
func (h *ActivityHandler) ReassignTask(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermTasksManage)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_task_mutation", activityMutationRateMax); err != nil {
			return nil, err
		}
		var req reassignTaskRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		v, err := h.service.ReassignTask(c, actor, scope, c.Param("taskId"), ReassignTaskInput{AssignedStaffRef: req.AssignedStaffRef, ExpectedVersion: req.ExpectedVersion})
		if err != nil {
			return nil, err
		}
		return TaskViewOf(v, h.service.now()), nil
	})
}
func (h *ActivityHandler) GetTask(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermTasksRead)
	if !ok {
		return
	}
	v, err := h.service.GetTask(c, actor, scope, c.Param("taskId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	okResponse(c, TaskViewOf(v, h.service.now()))
}
func (h *ActivityHandler) ListTasks(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermTasksRead)
	if !ok {
		return
	}
	opts, err := taskOptionsFrom(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	h.listTasks(c, actor, scope, opts)
}
func (h *ActivityHandler) ListCustomerTasks(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermTasksRead)
	if !ok {
		return
	}
	if _, err := h.service.GetProfile(c, actor, scope, c.Param("profileId")); err != nil {
		httpx.RespondError(c, err)
		return
	}
	opts, err := taskOptionsFrom(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	opts.CustomerProfileRef = c.Param("profileId")
	h.listTasks(c, actor, scope, opts)
}
func (h *ActivityHandler) listTasks(c *gin.Context, actor Actor, scope Scope, opts TaskListOptions) {
	items, more, err := h.service.ListTasks(c, actor, scope, opts)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	views := make([]TaskView, 0, len(items))
	for _, v := range items {
		views = append(views, TaskViewOf(v, h.service.now()))
	}
	opts = opts.bounded()
	okResponse(c, gin.H{"items": views, "pagination": gin.H{"limit": opts.Limit, "offset": opts.Offset, "has_more": more}})
}
func (h *ActivityHandler) ListTaskHistory(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermTasksRead)
	if !ok {
		return
	}
	items, err := h.service.ListTaskHistory(c, actor, scope, c.Param("taskId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	views := make([]TaskHistoryView, 0, len(items))
	for _, v := range items {
		views = append(views, TaskHistoryView{HistoryRef: v.HistoryRef, Action: v.Action, Status: v.Status, Priority: v.Priority, AssignedStaffRef: v.AssignedStaffID, DueAt: v.DueAt, Title: v.Title, Description: v.Description, Version: v.TaskVersion, ChangedByRef: v.ChangedByStaffID, OccurredAt: v.OccurredAt})
	}
	okResponse(c, gin.H{"items": views})
}

func okResponse(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}
func parseQueryTime(c *gin.Context, name string) (*time.Time, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, apperrors.Validation("Check your filters.", []apperrors.FieldViolation{{Field: name, Message: "Use an RFC3339 timestamp."}})
	}
	value = value.UTC()
	return &value, nil
}
func parsePage(c *gin.Context) (int, int, error) {
	limit, offset := 50, 0
	var violations []apperrors.FieldViolation
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 || v > 200 {
			violations = append(violations, apperrors.FieldViolation{Field: "limit", Message: "Limit must be from 1 to 200."})
		} else {
			limit = v
		}
	}
	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 {
			violations = append(violations, apperrors.FieldViolation{Field: "offset", Message: "Offset must be zero or greater."})
		} else {
			offset = v
		}
	}
	if len(violations) > 0 {
		return 0, 0, apperrors.Validation("Check your filters.", violations)
	}
	return limit, offset, nil
}
func interactionOptionsFrom(c *gin.Context) (InteractionListOptions, error) {
	limit, offset, err := parsePage(c)
	if err != nil {
		return InteractionListOptions{}, err
	}
	from, err := parseQueryTime(c, "occurred_from")
	if err != nil {
		return InteractionListOptions{}, err
	}
	to, err := parseQueryTime(c, "occurred_to")
	if err != nil {
		return InteractionListOptions{}, err
	}
	return InteractionListOptions{CustomerProfileRef: strings.TrimSpace(c.Query("customer_profile_ref")), Type: strings.ToUpper(strings.TrimSpace(c.Query("type"))), Direction: strings.ToUpper(strings.TrimSpace(c.Query("direction"))), Outcome: strings.ToUpper(strings.TrimSpace(c.Query("outcome"))), BranchRef: strings.TrimSpace(c.Query("branch_ref")), RecordedBy: strings.TrimSpace(c.Query("recorded_by")), Search: strings.TrimSpace(c.Query("search")), OccurredFrom: from, OccurredTo: to, Limit: limit, Offset: offset}, nil
}
func taskOptionsFrom(c *gin.Context) (TaskListOptions, error) {
	limit, offset, err := parsePage(c)
	if err != nil {
		return TaskListOptions{}, err
	}
	before, err := parseQueryTime(c, "due_before")
	if err != nil {
		return TaskListOptions{}, err
	}
	after, err := parseQueryTime(c, "due_after")
	if err != nil {
		return TaskListOptions{}, err
	}
	createdFrom, err := parseQueryTime(c, "created_from")
	if err != nil {
		return TaskListOptions{}, err
	}
	createdTo, err := parseQueryTime(c, "created_to")
	if err != nil {
		return TaskListOptions{}, err
	}
	var overdue *bool
	if raw := strings.TrimSpace(c.Query("overdue")); raw != "" {
		value, parseErr := strconv.ParseBool(raw)
		if parseErr != nil {
			return TaskListOptions{}, apperrors.Validation("Check your filters.", []apperrors.FieldViolation{{Field: "overdue", Message: "Use true or false."}})
		}
		overdue = &value
	}
	return TaskListOptions{CustomerProfileRef: strings.TrimSpace(c.Query("customer_profile_ref")), Status: strings.ToUpper(strings.TrimSpace(c.Query("status"))), Priority: strings.ToUpper(strings.TrimSpace(c.Query("priority"))), AssignedStaffRef: strings.TrimSpace(c.Query("assigned_staff_ref")), BranchRef: strings.TrimSpace(c.Query("branch_ref")), DueBefore: before, DueAfter: after, CreatedFrom: createdFrom, CreatedTo: createdTo, Overdue: overdue, Limit: limit, Offset: offset}, nil
}

// RegisterActivityRoutes wires Section 8C only when its specific surface is
// enabled. There are intentionally no internal routes and no notification
// delivery endpoints.
func RegisterActivityRoutes(group *gin.RouterGroup, h *ActivityHandler, flags Flags) {
	if !flags.CRMEnabled {
		return
	}
	crm := group.Group("/crm")
	if flags.InteractionsEnabled {
		interactions := crm.Group("/interactions")
		interactions.GET("", h.ListInteractions)
		interactions.POST("", h.CreateInteraction)
		interactions.GET("/:interactionId", h.GetInteraction)
		interactions.POST("/:interactionId/correct", h.CorrectInteraction)
		interactions.GET("/:interactionId/revisions", h.ListInteractionRevisions)
		crm.GET("/customers/:profileId/interactions", h.ListCustomerInteractions)
	}
	if flags.TasksEnabled {
		tasks := crm.Group("/tasks")
		tasks.GET("", h.ListTasks)
		tasks.POST("", h.CreateTask)
		tasks.GET("/:taskId", h.GetTask)
		tasks.PATCH("/:taskId", h.UpdateTask)
		tasks.POST("/:taskId/start", h.StartTask)
		tasks.POST("/:taskId/complete", h.CompleteTask)
		tasks.POST("/:taskId/cancel", h.CancelTask)
		tasks.POST("/:taskId/reassign", h.ReassignTask)
		tasks.GET("/:taskId/history", h.ListTaskHistory)
		crm.GET("/customers/:profileId/tasks", h.ListCustomerTasks)
	}
}
