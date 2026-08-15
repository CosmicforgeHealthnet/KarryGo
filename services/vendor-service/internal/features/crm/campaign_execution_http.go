package crm

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/httpx"
)

const (
	PermCampaignsExecute       = "vendor.crm.campaigns.execute"
	PermCampaignRunsRead       = "vendor.crm.campaigns.runs.read"
	PermCampaignRunsManage     = "vendor.crm.campaigns.runs.manage"
	PermCampaignDeliveriesRead = "vendor.crm.campaigns.deliveries.read"
)

type campaignRunRequest struct {
	SnapshotRef     string `json:"snapshot_ref"`
	Mode            string `json:"mode"`
	ScheduledFor    string `json:"scheduled_for"`
	ExpectedVersion int64  `json:"expected_version"`
	TemplateKey     string `json:"template_key"`
	Locale          string `json:"locale"`
	Title           string `json:"title"`
	Body            string `json:"body"`
}

type campaignRunMutationRequest struct {
	ExpectedVersion int64 `json:"expected_version"`
}

type CampaignRunView struct {
	RunRef                    string     `json:"run_ref"`
	CampaignRef               string     `json:"campaign_ref"`
	CampaignVersionRef        string     `json:"campaign_version_ref"`
	SnapshotRef               string     `json:"snapshot_ref"`
	CampaignDefinitionVersion int64      `json:"campaign_definition_version"`
	TriggerType               string     `json:"trigger_type"`
	Mode                      string     `json:"mode"`
	Status                    string     `json:"status"`
	ScheduledFor              time.Time  `json:"scheduled_for"`
	TotalDeliveries           int        `json:"total_deliveries"`
	HandedOffCount            int        `json:"handed_off_count"`
	DeliveredCount            int        `json:"delivered_count"`
	SuppressedCount           int        `json:"suppressed_count"`
	FailedCount               int        `json:"failed_count"`
	CreatedAt                 time.Time  `json:"created_at"`
	UpdatedAt                 time.Time  `json:"updated_at"`
	StartedAt                 *time.Time `json:"started_at,omitempty"`
	CompletedAt               *time.Time `json:"completed_at,omitempty"`
	CancelledAt               *time.Time `json:"cancelled_at,omitempty"`
	Version                   int64      `json:"version"`
}

func campaignRunView(r CampaignRun) CampaignRunView {
	return CampaignRunView{
		RunRef: r.ID, CampaignRef: r.CampaignID, CampaignVersionRef: r.CampaignVersionID, SnapshotRef: r.SnapshotID,
		CampaignDefinitionVersion: r.CampaignDefinitionVersion, TriggerType: r.TriggerType, Mode: r.Mode, Status: r.Status,
		ScheduledFor: r.ScheduledFor, TotalDeliveries: r.TotalDeliveries, HandedOffCount: r.HandedOffCount,
		DeliveredCount: r.DeliveredCount, SuppressedCount: r.SuppressedCount, FailedCount: r.FailedCount,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, StartedAt: r.StartedAt, CompletedAt: r.CompletedAt, CancelledAt: r.CancelledAt, Version: r.Version}
}

type CampaignDeliveryView struct {
	DeliveryRef              string     `json:"delivery_ref"`
	RunRef                   string     `json:"run_ref"`
	CampaignRef              string     `json:"campaign_ref"`
	SnapshotRef              string     `json:"snapshot_ref"`
	TargetRef                string     `json:"target_ref"`
	Channel                  string     `json:"channel"`
	VariantKey               *string    `json:"variant_key,omitempty"`
	IsControl                bool       `json:"is_control"`
	Status                   string     `json:"status"`
	SuppressionReason        *string    `json:"suppression_reason,omitempty"`
	NotificationRequestRef   string     `json:"notification_request_ref"`
	NotificationMessageRef   *string    `json:"notification_message_ref,omitempty"`
	AttributionRef           string     `json:"attribution_ref"`
	ConsentDecisionAtHandoff *string    `json:"consent_decision_at_handoff,omitempty"`
	ConsentEligibleAtHandoff *bool      `json:"consent_eligible_at_handoff,omitempty"`
	ConsentCheckedAt         *time.Time `json:"consent_checked_at,omitempty"`
	ConsentAsOf              *time.Time `json:"consent_as_of,omitempty"`
	ConsentSourceVersion     *int64     `json:"consent_source_version,omitempty"`
	PolicyCheckedAt          *time.Time `json:"policy_checked_at,omitempty"`
	NextAttemptAt            time.Time  `json:"next_attempt_at"`
	AttemptCount             int        `json:"attempt_count"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
	HandedOffAt              *time.Time `json:"handed_off_at,omitempty"`
	CompletedAt              *time.Time `json:"completed_at,omitempty"`
	Version                  int64      `json:"version"`
}

func campaignDeliveryView(d CampaignDelivery) CampaignDeliveryView {
	return CampaignDeliveryView{DeliveryRef: d.ID, RunRef: d.RunID, CampaignRef: d.CampaignID, SnapshotRef: d.SnapshotID, TargetRef: d.TargetID, Channel: d.Channel, VariantKey: d.VariantKey, IsControl: d.IsControl, Status: d.Status, SuppressionReason: d.SuppressionReason, NotificationRequestRef: d.NotificationRequestRef, NotificationMessageRef: d.NotificationMessageRef, AttributionRef: d.AttributionRef, ConsentDecisionAtHandoff: d.ConsentDecisionAtHandoff, ConsentEligibleAtHandoff: d.ConsentEligibleAtHandoff, ConsentCheckedAt: d.ConsentCheckedAt, ConsentAsOf: d.ConsentAsOf, ConsentSourceVersion: d.ConsentSourceVersion, PolicyCheckedAt: d.PolicyCheckedAt, NextAttemptAt: d.NextAttemptAt, AttemptCount: d.AttemptCount, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt, HandedOffAt: d.HandedOffAt, CompletedAt: d.CompletedAt, Version: d.Version}
}

func (h *CampaignHandler) CreateCampaignRun(c *gin.Context) {
	h.command(c, http.StatusCreated, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCampaignsExecute)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_campaign_execute"); err != nil {
			return nil, err
		}
		var req campaignRunRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		var scheduled *time.Time
		if strings.TrimSpace(req.ScheduledFor) != "" {
			v, e := time.Parse(time.RFC3339, req.ScheduledFor)
			if e != nil {
				return nil, campaignValidation("scheduled_for", "Use an RFC3339 timestamp.", CodeCampaignRunInvalidState)
			}
			scheduled = &v
		}
		r, err := h.service.CreateCampaignRun(c, actor, scope, strings.TrimSpace(c.Param("campaignId")), CreateCampaignRunInput{SnapshotRef: req.SnapshotRef, Mode: req.Mode, ScheduledFor: scheduled, ExpectedVersion: req.ExpectedVersion, TemplateKey: req.TemplateKey, Locale: req.Locale, Title: req.Title, Body: req.Body})
		if err != nil {
			return nil, err
		}
		return campaignRunView(r), nil
	})
}
func (h *CampaignHandler) ListCampaignRuns(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermCampaignRunsRead)
	if !ok {
		return
	}
	limit, offset, err := parsePage(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	runs, more, err := h.service.ListCampaignRuns(c, actor, scope, c.Param("campaignId"), limit, offset)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items := make([]CampaignRunView, 0, len(runs))
	for _, r := range runs {
		items = append(items, campaignRunView(r))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"runs": items, "has_more": more, "limit": limit, "offset": offset}})
}
func (h *CampaignHandler) GetCampaignRun(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermCampaignRunsRead)
	if !ok {
		return
	}
	r, err := h.service.GetCampaignRun(c, actor, scope, c.Param("campaignId"), c.Param("runId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": campaignRunView(r)})
}
func (h *CampaignHandler) mutateCampaignRun(c *gin.Context, action string) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCampaignRunsManage)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_campaign_run_manage"); err != nil {
			return nil, err
		}
		var req campaignRunMutationRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		var r CampaignRun
		switch action {
		case "pause":
			r, err = h.service.PauseCampaignRun(c, actor, scope, c.Param("campaignId"), c.Param("runId"), req.ExpectedVersion)
		case "resume":
			r, err = h.service.ResumeCampaignRun(c, actor, scope, c.Param("campaignId"), c.Param("runId"), req.ExpectedVersion)
		default:
			r, err = h.service.CancelCampaignRun(c, actor, scope, c.Param("campaignId"), c.Param("runId"), req.ExpectedVersion)
		}
		if err != nil {
			return nil, err
		}
		return campaignRunView(r), nil
	})
}
func (h *CampaignHandler) PauseCampaignRun(c *gin.Context)  { h.mutateCampaignRun(c, "pause") }
func (h *CampaignHandler) ResumeCampaignRun(c *gin.Context) { h.mutateCampaignRun(c, "resume") }
func (h *CampaignHandler) CancelCampaignRun(c *gin.Context) { h.mutateCampaignRun(c, "cancel") }
func (h *CampaignHandler) ListCampaignDeliveries(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermCampaignDeliveriesRead)
	if !ok {
		return
	}
	limit, offset, err := parsePage(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	ds, more, err := h.service.ListCampaignDeliveries(c, actor, scope, c.Param("campaignId"), c.Param("runId"), limit, offset)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items := make([]CampaignDeliveryView, 0, len(ds))
	for _, d := range ds {
		items = append(items, campaignDeliveryView(d))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"deliveries": items, "has_more": more, "limit": limit, "offset": offset}})
}
func (h *CampaignHandler) GetCampaignDelivery(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermCampaignDeliveriesRead)
	if !ok {
		return
	}
	d, err := h.service.GetCampaignDelivery(c, actor, scope, c.Param("campaignId"), c.Param("runId"), c.Param("deliveryId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": campaignDeliveryView(d)})
}
func (h *CampaignHandler) GetCampaignDeliveryHistory(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermCampaignDeliveriesRead)
	if !ok {
		return
	}
	limit, offset, err := parsePage(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	hs, more, err := h.service.CampaignDeliveryHistory(c, actor, scope, c.Param("campaignId"), c.Param("runId"), c.Param("deliveryId"), limit, offset)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"history": hs, "has_more": more, "limit": limit, "offset": offset}})
}

func RegisterCampaignExecutionRoutes(group *gin.RouterGroup, h *CampaignHandler, flags Flags) {
	if !flags.CRMEnabled || !flags.CampaignsEnabled {
		return
	}
	runs := group.Group("/crm/campaigns/:campaignId/runs")
	runs.POST("", h.CreateCampaignRun)
	runs.GET("", h.ListCampaignRuns)
	runs.GET("/:runId", h.GetCampaignRun)
	runs.POST("/:runId/cancel", h.CancelCampaignRun)
	runs.POST("/:runId/pause", h.PauseCampaignRun)
	runs.POST("/:runId/resume", h.ResumeCampaignRun)
	runs.GET("/:runId/deliveries", h.ListCampaignDeliveries)
	runs.GET("/:runId/deliveries/:deliveryId", h.GetCampaignDelivery)
	runs.GET("/:runId/deliveries/:deliveryId/history", h.GetCampaignDeliveryHistory)
}
