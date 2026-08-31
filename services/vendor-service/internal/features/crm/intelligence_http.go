package crm

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/httpx"

	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
)

// Section 8H — intelligence HTTP surface.
//
// Five routes, four of them reads. There is deliberately no route that writes
// a score: no POST /score, no PATCH /churn-score, no way for a staff member to
// declare a customer high-value or low-risk. Intelligence is derived, and a
// hand-set score would make every other score unfalsifiable — a reader could
// no longer tell which numbers came from the model.

// Section 8H intelligence permissions.
const (
	PermIntelligenceRead    = "vendor.crm.intelligence.read"
	PermIntelligenceRebuild = "vendor.crm.intelligence.rebuild"
)

// IntelligenceHandler serves the Section 8H intelligence routes.
type IntelligenceHandler struct {
	*SegmentationHandler
}

// NewIntelligenceHandler wires the handler.
func NewIntelligenceHandler(service *Service, store *idempotency.Store) *IntelligenceHandler {
	return &IntelligenceHandler{SegmentationHandler: NewSegmentationHandler(service, store)}
}

func branchQuery(c *gin.Context) string { return strings.TrimSpace(c.Query("branch_ref")) }

// GetIntelligenceOverview reports availability and freshness per model.
func (h *IntelligenceHandler) GetIntelligenceOverview(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermIntelligenceRead)
	if !ok {
		return
	}
	overview, err := h.service.IntelligenceOverviewFor(c, actor, scope, branchQuery(c))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": overview})
}

// GetRFMDistribution reports the RFM histograms.
func (h *IntelligenceHandler) GetRFMDistribution(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermIntelligenceRead)
	if !ok {
		return
	}
	distribution, err := h.service.RFMDistributionFor(c, actor, scope, branchQuery(c))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": distribution})
}

// GetChurnDistribution reports the churn risk histograms.
func (h *IntelligenceHandler) GetChurnDistribution(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermIntelligenceRead)
	if !ok {
		return
	}
	distribution, err := h.service.ChurnDistributionFor(c, actor, scope, branchQuery(c))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": distribution})
}

// GetCustomerIntelligence reports one relationship's scores.
func (h *IntelligenceHandler) GetCustomerIntelligence(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermIntelligenceRead)
	if !ok {
		return
	}
	// The profile is resolved through the accepted Section 8A read path, so
	// tenancy, redaction and branch reach are decided before any score is
	// touched.
	profile, err := h.service.GetProfile(c, actor, scope, strings.TrimSpace(c.Param("profileId")))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	intelligence, err := h.service.CustomerIntelligenceFor(c, actor, scope, profile, branchQuery(c))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": intelligence})
}

type rebuildIntelligenceRequest struct {
	ModelKey  string `json:"model_key"`
	ScopeType string `json:"scope_type"`
	BranchRef string `json:"branch_ref"`
}

// RebuildIntelligence enqueues a durable rebuild.
func (h *IntelligenceHandler) RebuildIntelligence(c *gin.Context) {
	h.command(c, http.StatusAccepted, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermIntelligenceRebuild)
		if err != nil {
			return nil, err
		}
		var req rebuildIntelligenceRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		evaluation, err := h.service.RequestIntelligenceRebuild(c, actor, scope, RebuildIntelligenceInput{
			ModelKey: req.ModelKey, ScopeType: req.ScopeType, BranchRef: req.BranchRef})
		if err != nil {
			return nil, err
		}
		return evaluationViewOf(evaluation), nil
	})
}

// IntelligenceEvaluationView is the wire shape of a rebuild job. It carries model identity
// and counts, never a customer.
type IntelligenceEvaluationView struct {
	EvaluationRef string     `json:"evaluation_ref"`
	ModelKey      string     `json:"model_key"`
	ModelVersion  int        `json:"model_version"`
	Scope         string     `json:"scope"`
	BranchRef     *string    `json:"branch_ref,omitempty"`
	Generation    int64      `json:"generation"`
	Status        string     `json:"status"`
	SourceAsOf    time.Time  `json:"source_as_of"`
	RequestedAt   time.Time  `json:"requested_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`

	ProfileCount     int `json:"profile_count"`
	AvailableCount   int `json:"available_count"`
	UnavailableCount int `json:"unavailable_count"`
}

func evaluationViewOf(e IntelligenceEvaluation) IntelligenceEvaluationView {
	return IntelligenceEvaluationView{
		EvaluationRef: e.ID, ModelKey: e.ModelKey, ModelVersion: e.ModelVersion,
		Scope: e.Scope, BranchRef: e.BranchID, Generation: e.Generation, Status: e.Status,
		SourceAsOf: e.SourceAsOf, RequestedAt: e.CreatedAt, CompletedAt: e.CompletedAt,
		ProfileCount: e.ProfileCount, AvailableCount: e.AvailableCount,
		UnavailableCount: e.UnavailableCount,
	}
}

// RegisterIntelligenceRoutes wires Section 8H intelligence.
//
// Five routes and no internal one: nothing outside this service produces a
// score, so there is no ingress for one to arrive on.
func RegisterIntelligenceRoutes(group *gin.RouterGroup, h *IntelligenceHandler, flags Flags) {
	if !flags.CRMEnabled || !flags.IntelligenceEnabled {
		return
	}
	crm := group.Group("/crm")
	crm.GET("/intelligence/overview", h.GetIntelligenceOverview)
	crm.GET("/intelligence/rfm", h.GetRFMDistribution)
	crm.GET("/intelligence/churn", h.GetChurnDistribution)
	crm.GET("/customers/:profileId/intelligence", h.GetCustomerIntelligence)
	// Registered regardless of WritesEnabled, matching the accepted sections:
	// the write switch is enforced in the usecase so a disabled rollout answers
	// "temporarily unavailable" rather than "no such endpoint".
	crm.POST("/intelligence/rebuild", h.RebuildIntelligence)
}
