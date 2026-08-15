package crm

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/shared/go/httpx"
)

// Section 8E permission codes.
//
// Five, and deliberately not six: there is no vendor.crm.campaigns.send. This
// section cannot send, and a permission naming an authority the service does
// not hold would be granted long before the guard rails around it existed.
//
// Listing targets is separated from reading a campaign for the same reason
// Section 8B separates member reads from segment reads: the customer-by-customer
// list is a strictly larger disclosure than the definition that produced it.
const (
	PermCampaignsRead       = "vendor.crm.campaigns.read"
	PermCampaignsManage     = "vendor.crm.campaigns.manage"
	PermCampaignsPreview    = "vendor.crm.campaigns.preview"
	PermCampaignsPrepare    = "vendor.crm.campaigns.prepare"
	PermCampaignTargetsRead = "vendor.crm.campaigns.targets.read"
)

const campaignMutationRateMax = 60

// CampaignHandler exposes the Section 8E vendor surface. It reuses the accepted
// Section 8B/8C/8D command edge — the same Idempotency-Key store, the same
// actor resolution, the same rate limiter — rather than inventing a fourth
// convention.
type CampaignHandler struct {
	*SegmentationHandler
	limiter ActivityRateLimiter
}

// NewCampaignHandler wires the Section 8E handler.
func NewCampaignHandler(service *Service, store *idempotency.Store, limiters ...ActivityRateLimiter) *CampaignHandler {
	var limiter ActivityRateLimiter
	if len(limiters) > 0 {
		limiter = limiters[0]
	}
	return &CampaignHandler{SegmentationHandler: NewSegmentationHandler(service, store), limiter: limiter}
}

func (h *CampaignHandler) rateLimit(c *gin.Context, actor Actor, bucket string) error {
	if h.limiter == nil {
		return nil
	}
	return h.limiter.Allow(c, bucket, actor.VendorID+":"+actor.StaffID, campaignMutationRateMax, activityRateWindow)
}

// ---------------------------------------------------------------------------
// Wire shapes
// ---------------------------------------------------------------------------

type experimentVariantRequest struct {
	VariantKey    string `json:"variant_key"`
	DisplayName   string `json:"display_name"`
	AllocationBps int    `json:"allocation_bps"`
	IsControl     bool   `json:"is_control"`
}

type campaignDefinitionRequest struct {
	Name                       string                     `json:"name"`
	Objective                  string                     `json:"objective"`
	ScopeType                  string                     `json:"scope_type"`
	BranchRef                  string                     `json:"branch_ref"`
	Channels                   []string                   `json:"channels"`
	SourceSegmentRef           string                     `json:"source_segment_ref"`
	ExclusionSegmentRefs       []string                   `json:"exclusion_segment_refs"`
	FrequencyCapCount          *int                       `json:"frequency_cap_count"`
	FrequencyCapWindowMinutes  *int                       `json:"frequency_cap_window_minutes"`
	QuietHoursEnabled          *bool                      `json:"quiet_hours_enabled"`
	QuietHoursStart            string                     `json:"quiet_hours_start"`
	QuietHoursEnd              string                     `json:"quiet_hours_end"`
	QuietHoursTimezone         string                     `json:"quiet_hours_timezone"`
	ServiceRecoverySuppression *bool                      `json:"service_recovery_suppression_enabled"`
	ExperimentEnabled          *bool                      `json:"experiment_enabled"`
	Variants                   []experimentVariantRequest `json:"experiment_variants"`
}

func (r campaignDefinitionRequest) input() CampaignDefinitionInput {
	variants := make([]ExperimentVariantInput, 0, len(r.Variants))
	for _, variant := range r.Variants {
		variants = append(variants, ExperimentVariantInput{
			VariantKey: variant.VariantKey, DisplayName: variant.DisplayName,
			AllocationBps: variant.AllocationBps, IsControl: variant.IsControl,
		})
	}
	return CampaignDefinitionInput{
		Name: r.Name, Objective: r.Objective, ScopeType: r.ScopeType, BranchRef: r.BranchRef,
		Channels: r.Channels, SourceSegmentRef: r.SourceSegmentRef, ExclusionSegmentRefs: r.ExclusionSegmentRefs,
		FrequencyCapCount: r.FrequencyCapCount, FrequencyCapWindowMinutes: r.FrequencyCapWindowMinutes,
		QuietHoursEnabled: r.QuietHoursEnabled, QuietHoursStart: r.QuietHoursStart,
		QuietHoursEnd: r.QuietHoursEnd, QuietHoursTimezone: r.QuietHoursTimezone,
		ServiceRecoverySuppression: r.ServiceRecoverySuppression,
		ExperimentEnabled:          r.ExperimentEnabled, Variants: variants,
	}
}

type updateCampaignRequest struct {
	campaignDefinitionRequest
	ChangeReason    string `json:"change_reason"`
	ExpectedVersion int64  `json:"expected_version"`
}

type archiveCampaignRequest struct {
	ExpectedVersion int64 `json:"expected_version"`
}

type previewCampaignRequest struct {
	SampleLimit int `json:"sample_limit"`
	// HypotheticalSendAt asks the quiet-hours question for one instant. It
	// schedules nothing; Section 8E has no scheduler.
	HypotheticalSendAt string `json:"hypothetical_send_at"`
}

// CampaignView is the safe wire projection of a campaign.
type CampaignView struct {
	CampaignRef        string     `json:"campaign_ref"`
	Name               string     `json:"name"`
	Objective          string     `json:"objective"`
	ScopeType          string     `json:"scope_type"`
	BranchRef          *string    `json:"branch_ref,omitempty"`
	Status             string     `json:"status"`
	DefinitionVersion  int64      `json:"definition_version"`
	CurrentSnapshotRef *string    `json:"current_snapshot_ref,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	ArchivedAt         *time.Time `json:"archived_at,omitempty"`
	Version            int64      `json:"version"`
}

// CampaignViewOf renders a campaign for the wire.
func CampaignViewOf(c Campaign) CampaignView {
	return CampaignView{
		CampaignRef: c.ID, Name: c.Name, Objective: c.Objective, ScopeType: c.ScopeType,
		BranchRef: c.BranchID, Status: c.Status, DefinitionVersion: c.CurrentDefinitionVersion,
		CurrentSnapshotRef: c.CurrentSnapshotID, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
		ArchivedAt: c.ArchivedAt, Version: c.Version,
	}
}

// CampaignVersionView is the safe wire projection of one immutable definition.
type CampaignVersionView struct {
	DefinitionVersion    int64                        `json:"definition_version"`
	DefinitionHash       string                       `json:"definition_hash"`
	Name                 string                       `json:"name"`
	Objective            string                       `json:"objective"`
	ScopeType            string                       `json:"scope_type"`
	BranchRef            *string                      `json:"branch_ref,omitempty"`
	Channels             []string                     `json:"channels"`
	SourceSegmentRef     string                       `json:"source_segment_ref"`
	ExclusionSegmentRefs []string                     `json:"exclusion_segment_refs"`
	Frequency            CampaignFrequencyProjection  `json:"frequency_policy"`
	QuietHours           CampaignQuietHoursProjection `json:"quiet_hours_policy"`
	ServiceRecovery      bool                         `json:"service_recovery_suppression_enabled"`
	ExperimentEnabled    bool                         `json:"experiment_enabled"`
	Variants             []experimentVariantView      `json:"experiment_variants"`
	CreatedAt            time.Time                    `json:"created_at"`
	ChangeReason         *string                      `json:"change_reason,omitempty"`
}

type experimentVariantView struct {
	VariantKey    string `json:"variant_key"`
	DisplayName   string `json:"display_name"`
	AllocationBps int    `json:"allocation_bps"`
	IsControl     bool   `json:"is_control"`
}

// CampaignVersionViewOf renders a definition for the wire.
func CampaignVersionViewOf(v CampaignVersion) CampaignVersionView {
	variants := make([]experimentVariantView, 0, len(v.Variants))
	for _, variant := range v.Variants {
		variants = append(variants, experimentVariantView{
			VariantKey: variant.VariantKey, DisplayName: variant.DisplayName,
			AllocationBps: variant.AllocationBps, IsControl: variant.IsControl,
		})
	}
	quietHours, _ := quietHoursProjection(v.QuietHours, nil)
	exclusions := v.ExclusionSegmentIDs
	if exclusions == nil {
		exclusions = []string{}
	}
	return CampaignVersionView{
		DefinitionVersion: v.DefinitionVersion, DefinitionHash: v.DefinitionHash, Name: v.Name,
		Objective: v.Objective, ScopeType: v.ScopeType, BranchRef: v.BranchID, Channels: v.Channels,
		SourceSegmentRef: v.SourceSegmentID, ExclusionSegmentRefs: exclusions,
		Frequency: frequencyProjection(v.Frequency), QuietHours: quietHours,
		ServiceRecovery: v.ServiceRecoverySuppression, ExperimentEnabled: v.ExperimentEnabled,
		Variants: variants, CreatedAt: v.CreatedAt, ChangeReason: v.ChangeReason,
	}
}

// CampaignSnapshotView is the safe wire projection of a frozen audience.
//
// It reports what the audience is and how it was reached. It never reports who
// is in it: that is the separately-permissioned target list.
type CampaignSnapshotView struct {
	SnapshotRef               string    `json:"snapshot_ref"`
	CampaignRef               string    `json:"campaign_ref"`
	CampaignDefinitionVersion int64     `json:"campaign_definition_version"`
	DefinitionHash            string    `json:"definition_hash"`
	AudienceHash              *string   `json:"audience_hash,omitempty"`
	Status                    string    `json:"status"`
	EvaluationAsOf            time.Time `json:"evaluation_as_of"`

	SourceSegmentRef               string `json:"source_segment_ref"`
	SourceSegmentDefinitionVersion int64  `json:"source_segment_definition_version"`
	SourceSegmentGeneration        *int64 `json:"source_segment_generation,omitempty"`

	SourceCount                    int `json:"source_count"`
	SegmentExclusionCount          int `json:"segment_exclusion_count"`
	PrivacySuppressedCount         int `json:"privacy_suppressed_count"`
	ServiceRecoverySuppressedCount int `json:"service_recovery_suppressed_count"`
	ConsentSuppressedCount         int `json:"consent_suppressed_count"`
	TargetCount                    int `json:"target_count"`
	ControlCount                   int `json:"control_count"`
	TreatmentCount                 int `json:"treatment_count"`

	Channels []CampaignChannelProjection `json:"channels,omitempty"`

	// A prepared audience is not permission to contact it. These three are
	// always true, and Section 8F must honour all of them.
	SendTimeConsentRecheckRequired    bool `json:"send_time_consent_recheck_required"`
	SendTimeFrequencyRecheckRequired  bool `json:"send_time_frequency_recheck_required"`
	SendTimeQuietHoursRecheckRequired bool `json:"send_time_quiet_hours_recheck_required"`

	AttemptCount int        `json:"attempt_count"`
	DeadLettered bool       `json:"dead_lettered"`
	ErrorCode    *string    `json:"error_code,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

// CampaignSnapshotViewOf renders a snapshot for the wire.
func CampaignSnapshotViewOf(s CampaignSnapshot) CampaignSnapshotView {
	return CampaignSnapshotView{
		SnapshotRef: s.ID, CampaignRef: s.CampaignID, CampaignDefinitionVersion: s.CampaignDefinitionVersion,
		DefinitionHash: s.DefinitionHash, AudienceHash: s.AudienceHash, Status: s.Status,
		EvaluationAsOf: s.EvaluationAsOf, SourceSegmentRef: s.SourceSegmentID,
		SourceSegmentDefinitionVersion:    s.SourceSegmentDefinitionVersion,
		SourceSegmentGeneration:           s.SourceSegmentGeneration,
		SourceCount:                       s.SourceCount,
		SegmentExclusionCount:             s.SegmentExclusionCount,
		PrivacySuppressedCount:            s.PrivacySuppressedCount,
		ServiceRecoverySuppressedCount:    s.ServiceRecoverySuppressedCount,
		ConsentSuppressedCount:            s.ConsentSuppressedCount,
		TargetCount:                       s.TargetCount,
		ControlCount:                      s.ControlCount,
		TreatmentCount:                    s.TreatmentCount,
		SendTimeConsentRecheckRequired:    true,
		SendTimeFrequencyRecheckRequired:  true,
		SendTimeQuietHoursRecheckRequired: true,
		AttemptCount:                      s.AttemptCount, DeadLettered: s.DeadLettered, ErrorCode: s.ErrorCode,
		CreatedAt: s.CreatedAt, CompletedAt: s.CompletedAt,
	}
}

// CampaignTargetView is the safe wire projection of one selected relationship.
//
// There is no contact field of any kind. A caller that needs to reach this
// customer must ask the identity authority and then the delivery authority;
// this row says only that the relationship was selected, into which cohort, and
// what consent looked like at the time.
type CampaignTargetView struct {
	TargetRef             string                  `json:"target_ref"`
	ProfileRef            string                  `json:"profile_ref"`
	DisplayLabel          *string                 `json:"display_label"`
	ExperimentVariantKey  *string                 `json:"experiment_variant_key,omitempty"`
	IsControl             bool                    `json:"is_control"`
	ExperimentBucket      *int                    `json:"experiment_bucket,omitempty"`
	CurrentlyActionable   bool                    `json:"currently_actionable"`
	SuppressionReasonCode *string                 `json:"suppression_reason_code,omitempty"`
	ServiceRecoveryActive bool                    `json:"service_recovery_active"`
	Channels              []CampaignTargetChannel `json:"channels"`
}

// CampaignTargetViewOf renders a target for the wire.
func CampaignTargetViewOf(t CampaignTarget) CampaignTargetView {
	channels := t.Channels
	if channels == nil {
		channels = []CampaignTargetChannel{}
	}
	return CampaignTargetView{
		TargetRef: t.ID, ProfileRef: t.ProfileID, DisplayLabel: t.DisplayLabel,
		ExperimentVariantKey: t.VariantKey, IsControl: t.IsControl, ExperimentBucket: t.Bucket,
		CurrentlyActionable: t.Actionable(), SuppressionReasonCode: t.SuppressionReason,
		ServiceRecoveryActive: t.ServiceRecoveryActive, Channels: channels,
	}
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// ListCampaigns returns campaigns visible to the caller's scope.
func (h *CampaignHandler) ListCampaigns(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermCampaignsRead)
	if !ok {
		return
	}
	limit, offset, err := parsePage(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	from, err := parseQueryTime(c, "created_from")
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	to, err := parseQueryTime(c, "created_to")
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	campaigns, hasMore, err := h.service.ListCampaigns(c, actor, scope, CampaignListOptions{
		Limit: limit, Offset: offset, Status: c.Query("status"), ScopeType: c.Query("scope_type"),
		BranchRef: c.Query("branch_ref"), Objective: c.Query("objective"), Channel: c.Query("channel"),
		CreatedFrom: from, CreatedTo: to, Search: c.Query("search"),
	})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items := make([]CampaignView, 0, len(campaigns))
	for _, campaign := range campaigns {
		items = append(items, CampaignViewOf(campaign))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"campaigns": items, "has_more": hasMore, "limit": limit, "offset": offset}})
}

// GetCampaign returns one campaign together with its current definition.
func (h *CampaignHandler) GetCampaign(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermCampaignsRead)
	if !ok {
		return
	}
	campaign, err := h.service.GetCampaign(c, actor, scope, strings.TrimSpace(c.Param("campaignId")))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	version, err := h.service.CampaignVersionAt(c, h.service.db, actor.VendorID, campaign.ID, campaign.CurrentDefinitionVersion)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"campaign": CampaignViewOf(campaign), "definition": CampaignVersionViewOf(version)}})
}

// CreateCampaign defines a campaign in DRAFT.
func (h *CampaignHandler) CreateCampaign(c *gin.Context) {
	h.command(c, http.StatusCreated, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCampaignsManage)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_campaign_manage"); err != nil {
			return nil, err
		}
		var req campaignDefinitionRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		campaign, version, err := h.service.CreateCampaign(c, actor, scope, req.input())
		if err != nil {
			return nil, err
		}
		return gin.H{"campaign": CampaignViewOf(campaign), "definition": CampaignVersionViewOf(version)}, nil
	})
}

// UpdateCampaign replaces a campaign definition, minting a new version when
// anything meaningful changed.
func (h *CampaignHandler) UpdateCampaign(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCampaignsManage)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_campaign_manage"); err != nil {
			return nil, err
		}
		var req updateCampaignRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		campaign, version, err := h.service.UpdateCampaign(c, actor, scope, strings.TrimSpace(c.Param("campaignId")),
			UpdateCampaignInput{Definition: req.input(), ChangeReason: req.ChangeReason, ExpectedVersion: req.ExpectedVersion})
		if err != nil {
			return nil, err
		}
		return gin.H{"campaign": CampaignViewOf(campaign), "definition": CampaignVersionViewOf(version)}, nil
	})
}

// ArchiveCampaign retires a campaign.
func (h *CampaignHandler) ArchiveCampaign(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCampaignsManage)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_campaign_manage"); err != nil {
			return nil, err
		}
		var req archiveCampaignRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		campaign, err := h.service.ArchiveCampaign(c, actor, scope, strings.TrimSpace(c.Param("campaignId")), req.ExpectedVersion)
		if err != nil {
			return nil, err
		}
		return CampaignViewOf(campaign), nil
	})
}

// PreviewCampaign evaluates the current audience without persisting anything.
//
// It is a POST because it carries a request body, not because it changes
// state: no target row and no snapshot is written, and running it twice is
// indistinguishable from running it once.
func (h *CampaignHandler) PreviewCampaign(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermCampaignsPreview)
	if !ok {
		return
	}
	if err := h.rateLimit(c, actor, "crm_campaign_preview"); err != nil {
		httpx.RespondError(c, err)
		return
	}
	var req previewCampaignRequest
	if c.Request.Body != nil {
		if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
			httpx.RespondError(c, campaignValidation("body", "Could not read the preview request.", ""))
			return
		}
	}
	input := CampaignPreviewInput{SampleLimit: req.SampleLimit}
	if trimmed := strings.TrimSpace(req.HypotheticalSendAt); trimmed != "" {
		instant, err := time.Parse(time.RFC3339, trimmed)
		if err != nil {
			httpx.RespondError(c, campaignValidation("hypothetical_send_at",
				"Use an RFC3339 timestamp, such as 2026-08-12T21:00:00Z.", ""))
			return
		}
		input.HypotheticalSendAt = &instant
	}
	result, err := h.service.PreviewCampaign(c, actor, scope, strings.TrimSpace(c.Param("campaignId")), input)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// PrepareCampaign enqueues a durable audience preparation.
func (h *CampaignHandler) PrepareCampaign(c *gin.Context) {
	h.command(c, http.StatusAccepted, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCampaignsPrepare)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_campaign_prepare"); err != nil {
			return nil, err
		}
		snapshot, err := h.service.PrepareCampaign(c, actor, scope, strings.TrimSpace(c.Param("campaignId")))
		if err != nil {
			return nil, err
		}
		return CampaignSnapshotViewOf(snapshot), nil
	})
}

// ListSnapshots pages a campaign's snapshot history.
func (h *CampaignHandler) ListSnapshots(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermCampaignsRead)
	if !ok {
		return
	}
	limit, offset, err := parsePage(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	snapshots, hasMore, err := h.service.ListSnapshots(c, actor, scope, strings.TrimSpace(c.Param("campaignId")), limit, offset)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items := make([]CampaignSnapshotView, 0, len(snapshots))
	for _, snapshot := range snapshots {
		items = append(items, CampaignSnapshotViewOf(snapshot))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"snapshots": items, "has_more": hasMore, "limit": limit, "offset": offset}})
}

// GetSnapshot returns one snapshot with its per-channel eligibility figures.
func (h *CampaignHandler) GetSnapshot(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermCampaignsRead)
	if !ok {
		return
	}
	snapshot, err := h.service.GetSnapshot(c, actor, scope, strings.TrimSpace(c.Param("campaignId")),
		strings.TrimSpace(c.Param("snapshotId")))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	view := CampaignSnapshotViewOf(snapshot)
	if view.Channels, err = h.service.SnapshotChannelCounts(c, actor.VendorID, snapshot.ID); err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": view})
}

// ListSnapshotTargets pages the relationships inside one snapshot.
func (h *CampaignHandler) ListSnapshotTargets(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermCampaignTargetsRead)
	if !ok {
		return
	}
	limit, offset, err := parsePage(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	targets, hasMore, err := h.service.ListSnapshotTargets(c, actor, scope,
		strings.TrimSpace(c.Param("campaignId")), strings.TrimSpace(c.Param("snapshotId")),
		TargetListOptions{
			Limit: limit, Offset: offset, Cohort: c.Query("cohort"),
			ActionableOnly:    strings.EqualFold(c.Query("actionable_only"), "true"),
			EligibleOnChannel: c.Query("eligible_on_channel"),
		})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items := make([]CampaignTargetView, 0, len(targets))
	for _, target := range targets {
		items = append(items, CampaignTargetViewOf(target))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"targets": items, "has_more": hasMore, "limit": limit, "offset": offset,
		// Restated on every page so a client paging a large audience cannot
		// lose sight of it between requests.
		"send_time_recheck_required": true}})
}

// RegisterCampaignRoutes wires Section 8E.
//
// Ten routes, and no eleventh. There is no schedule endpoint, no send endpoint,
// no notification handoff and no internal route: Section 8E has nothing to
// receive on, because no producer contract exists for it, and nothing to hand
// off, because delivery is Section 8F's to own.
func RegisterCampaignRoutes(group *gin.RouterGroup, h *CampaignHandler, flags Flags) {
	if !flags.CRMEnabled || !flags.CampaignsEnabled {
		return
	}
	campaigns := group.Group("/crm/campaigns")
	campaigns.GET("", h.ListCampaigns)
	campaigns.POST("", h.CreateCampaign)
	campaigns.GET("/:campaignId", h.GetCampaign)
	campaigns.PATCH("/:campaignId", h.UpdateCampaign)
	campaigns.POST("/:campaignId/archive", h.ArchiveCampaign)
	campaigns.POST("/:campaignId/preview", h.PreviewCampaign)
	campaigns.POST("/:campaignId/prepare", h.PrepareCampaign)
	campaigns.GET("/:campaignId/snapshots", h.ListSnapshots)
	campaigns.GET("/:campaignId/snapshots/:snapshotId", h.GetSnapshot)
	campaigns.GET("/:campaignId/snapshots/:snapshotId/targets", h.ListSnapshotTargets)
}
