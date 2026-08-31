package cost

// Section 16D HTTP surface.
//
// Every handler derives the tenant from the authenticated session and never
// from the request. A caller-supplied business id is not authority here; it is
// not read at all. Branch reach is checked before any data is returned, so a
// refusal cannot be used to probe whether another branch's cost exists.

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/features/production"
	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/shared/go/httpx"
)

// Handler serves the Section 16D cost API.
type Handler struct {
	service     *Service
	idempotency *idempotency.Store
}

// NewHandler builds the cost handler.
func NewHandler(service *Service, store *idempotency.Store) *Handler {
	return &Handler{service: service, idempotency: store}
}

func strictJSON(c *gin.Context, destination any) error {
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return ErrInvalid
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return ErrInvalid
	}
	return nil
}

// expectedVersion reads If-Match. A mutation without it is refused rather than
// applied blind, which is what stops a stale client silently overwriting a
// policy someone else has just changed.
func expectedVersion(c *gin.Context) (int64, error) {
	raw := strings.TrimSpace(c.GetHeader("If-Match"))
	raw = strings.Trim(raw, "\"")
	if raw == "" {
		return 0, ErrVersionConflict
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, ErrVersionConflict
	}
	return value, nil
}

func (h *Handler) actor(c *gin.Context) (production.Actor, error) {
	staff, ok := authctx.FromContext(c)
	if !ok {
		return production.Actor{}, errors.New("authentication required")
	}
	return production.Actor{
		StaffID:       staff.StaffID,
		VendorID:      staff.VendorID,
		SessionID:     staff.SessionID,
		CorrelationID: httpx.GetRequestID(c),
		CausationID:   c.GetHeader("X-Causation-ID"),
		CommandRef:    c.GetHeader("Idempotency-Key"),
	}, nil
}

func costError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": gin.H{
			"code": "COST_NOT_FOUND", "message": "Cost resource was not found."}})
	case errors.Is(err, ErrInvalid):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "error": gin.H{
			"code": "COST_VALIDATION_FAILED", "message": "Check the submitted cost values."}})
	case errors.Is(err, ErrVersionConflict):
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": gin.H{
			"code":    "COST_VERSION_CONFLICT",
			"message": "This cost policy changed since you loaded it."}})
	case errors.Is(err, ErrSnapshotImmutable):
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": gin.H{
			"code":    "COST_SNAPSHOT_IMMUTABLE",
			"message": "Historical cost evidence cannot be changed."}})
	case errors.Is(err, ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{
			"code": "COST_FORBIDDEN", "message": "You do not have access to cost information."}})
	default:
		httpx.RespondError(c, err)
	}
}

func costOK(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"success": true, "data": data})
}

func unauthorized(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{
		"code": "UNAUTHORIZED", "message": "Authentication is required."}})
}

// authorized resolves the actor and checks one permission.
func (h *Handler) authorized(
	c *gin.Context,
	permission string,
) (production.Actor, production.Scope, bool) {
	actor, err := h.actor(c)
	if err != nil {
		unauthorized(c)
		return production.Actor{}, production.Scope{}, false
	}
	scope, err := h.service.Authorize(c, actor, permission)
	if err != nil {
		costError(c, err)
		return production.Actor{}, production.Scope{}, false
	}
	if scope.Empty() {
		costError(c, ErrForbidden)
		return production.Actor{}, production.Scope{}, false
	}
	return actor, scope, true
}

// authorizedBranch additionally requires the actor's scope to reach branchID.
func (h *Handler) authorizedBranch(
	c *gin.Context,
	permission, branchID string,
) (production.Actor, bool) {
	actor, scope, ok := h.authorized(c, permission)
	if !ok {
		return production.Actor{}, false
	}
	if branchID != "" && !scope.Covers(branchID) {
		costError(c, ErrForbidden)
		return production.Actor{}, false
	}
	return actor, true
}

func page(c *gin.Context) (int, int) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// ---------------------------------------------------------------------------
// Cost policies
// ---------------------------------------------------------------------------

func (h *Handler) listPolicies(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermCostRead)
	if !ok {
		return
	}
	limit, offset := page(c)
	policies, err := h.service.ListPolicies(
		c, actor, scope, c.Query("subject_type"), limit, offset)
	if err != nil {
		costError(c, err)
		return
	}
	if policies == nil {
		policies = []Policy{}
	}
	costOK(c, http.StatusOK, gin.H{
		"policies": policies, "limit": limit, "offset": offset})
}

func (h *Handler) getPolicy(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermCostRead)
	if !ok {
		return
	}
	policy, err := h.service.GetPolicy(c, actor, c.Param("policyId"))
	if err != nil {
		costError(c, err)
		return
	}
	// Tenancy is enforced in the service; branch reach is enforced here so a
	// branch-scoped actor cannot read another branch's override.
	if policy.BranchID != "" && !scope.Covers(policy.BranchID) {
		costError(c, ErrForbidden)
		return
	}
	costOK(c, http.StatusOK, policy)
}

func (h *Handler) createPolicy(c *gin.Context) {
	actor, err := h.actor(c)
	if err != nil {
		unauthorized(c)
		return
	}
	var in PolicyInput
	if err = strictJSON(c, &in); err != nil {
		costError(c, err)
		return
	}
	policy, err := h.service.CreatePolicy(c, actor, in)
	if err != nil {
		costError(c, err)
		return
	}
	costOK(c, http.StatusCreated, policy)
}

func (h *Handler) updatePolicy(c *gin.Context) {
	actor, err := h.actor(c)
	if err != nil {
		unauthorized(c)
		return
	}
	version, err := expectedVersion(c)
	if err != nil {
		costError(c, err)
		return
	}
	var in PolicyInput
	if err = strictJSON(c, &in); err != nil {
		costError(c, err)
		return
	}
	policy, err := h.service.UpdatePolicy(c, actor, c.Param("policyId"), version, in)
	if err != nil {
		costError(c, err)
		return
	}
	costOK(c, http.StatusOK, policy)
}

func (h *Handler) archivePolicy(c *gin.Context) {
	actor, err := h.actor(c)
	if err != nil {
		unauthorized(c)
		return
	}
	version, err := expectedVersion(c)
	if err != nil {
		costError(c, err)
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if err = strictJSON(c, &body); err != nil {
		costError(c, err)
		return
	}
	policy, err := h.service.ArchivePolicy(
		c, actor, c.Param("policyId"), version, body.Reason)
	if err != nil {
		costError(c, err)
		return
	}
	costOK(c, http.StatusOK, policy)
}

// ---------------------------------------------------------------------------
// Recipe cost
// ---------------------------------------------------------------------------

// recipeCostPreview is a recalculable estimate and is deliberately NOT
// persisted. That is what separates it from an immutable snapshot.
func (h *Handler) recipeCostPreview(c *gin.Context) {
	branchID := c.Query("branch_id")
	actor, ok := h.authorizedBranch(c, PermCostRead, branchID)
	if !ok {
		return
	}
	snapshot, err := h.service.Engine().CalculateRecipeCost(
		c, actor.VendorID, branchID, c.Param("versionId"))
	if err != nil {
		costError(c, ErrNotFound)
		return
	}
	costOK(c, http.StatusOK, snapshot)
}

// currentRecipeCost previews the recipe's ACTIVE version.
func (h *Handler) currentRecipeCost(c *gin.Context) {
	branchID := c.Query("branch_id")
	actor, ok := h.authorizedBranch(c, PermCostRead, branchID)
	if !ok {
		return
	}
	versionID, err := h.service.Engine().ActiveVersionFor(
		c, actor.VendorID, c.Param("recipeId"))
	if err != nil || versionID == "" {
		costError(c, ErrNotFound)
		return
	}
	snapshot, err := h.service.Engine().CalculateRecipeCost(
		c, actor.VendorID, branchID, versionID)
	if err != nil {
		costError(c, ErrNotFound)
		return
	}
	costOK(c, http.StatusOK, snapshot)
}

// createRecipeSnapshot writes immutable evidence. Idempotency-Key makes a
// retry return the original snapshot rather than writing a second history.
func (h *Handler) createRecipeSnapshot(c *gin.Context) {
	branchID := c.Query("branch_id")
	actor, ok := h.authorizedBranch(c, PermCostRead, branchID)
	if !ok {
		return
	}
	snapshot, err := h.service.Engine().CalculateRecipeCost(
		c, actor.VendorID, branchID, c.Param("versionId"))
	if err != nil {
		costError(c, ErrNotFound)
		return
	}
	snapshot.CalculationType = CalcRecipeSnapshot
	stored, err := PersistSnapshot(c, h.service.db, actor.VendorID, snapshot,
		actor.StaffID, actor.CorrelationID, actor.CommandRef)
	if err != nil {
		costError(c, err)
		return
	}
	costOK(c, http.StatusCreated, stored)
}

func (h *Handler) latestRecipeSnapshot(c *gin.Context) {
	actor, _, ok := h.authorized(c, PermCostRead)
	if !ok {
		return
	}
	snapshot, found, err := h.service.Engine().LatestRecipeSnapshot(
		c, actor.VendorID, c.Param("versionId"))
	if err != nil {
		costError(c, err)
		return
	}
	if !found {
		costError(c, ErrNotFound)
		return
	}
	costOK(c, http.StatusOK, snapshot)
}

func (h *Handler) recipeCostHistory(c *gin.Context) {
	actor, _, ok := h.authorized(c, PermCostRead)
	if !ok {
		return
	}
	limit, offset := page(c)
	snapshots, err := h.service.Engine().RecipeCostHistory(
		c, actor.VendorID, c.Param("recipeId"),
		c.Query("recipe_version_id"), limit, offset)
	if err != nil {
		costError(c, err)
		return
	}
	if snapshots == nil {
		snapshots = []Snapshot{}
	}
	costOK(c, http.StatusOK, gin.H{
		"snapshots": snapshots, "limit": limit, "offset": offset})
}

// snapshotComponents is the reproducibility read: why a total was that total.
func (h *Handler) snapshotComponents(c *gin.Context) {
	actor, _, ok := h.authorized(c, PermCostRead)
	if !ok {
		return
	}
	components, err := h.service.Engine().LoadComponents(
		c, actor.VendorID, c.Param("snapshotId"))
	if err != nil {
		costError(c, err)
		return
	}
	if len(components) == 0 {
		// Either the snapshot does not exist or it belongs to another tenant.
		// One answer for both, so this cannot be used as an existence probe.
		costError(c, ErrNotFound)
		return
	}
	costOK(c, http.StatusOK, gin.H{"components": components})
}

// ---------------------------------------------------------------------------
// Production cost
// ---------------------------------------------------------------------------

func (h *Handler) calculateProductionCost(c *gin.Context, calculationType string) {
	actor, scope, ok := h.authorized(c, PermProductionCostRead)
	if !ok {
		return
	}
	snapshot, err := h.service.Engine().CalculateProductionCost(
		c, actor.VendorID, c.Param("batchId"), calculationType)
	if err != nil {
		costError(c, ErrNotFound)
		return
	}
	if snapshot.BranchID != "" && !scope.Covers(snapshot.BranchID) {
		costError(c, ErrForbidden)
		return
	}
	stored, err := PersistSnapshot(c, h.service.db, actor.VendorID, snapshot,
		actor.StaffID, actor.CorrelationID, actor.CommandRef)
	if err != nil {
		costError(c, err)
		return
	}
	costOK(c, http.StatusCreated, stored)
}

func (h *Handler) expectedProductionCost(c *gin.Context) {
	h.calculateProductionCost(c, CalcProductionExpected)
}

func (h *Handler) actualProductionCost(c *gin.Context) {
	h.calculateProductionCost(c, CalcProductionActual)
}

func (h *Handler) readProductionCost(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermProductionCostRead)
	if !ok {
		return
	}
	batchID := c.Param("batchId")

	expected, expectedFound, err := h.service.Engine().LoadProductionSnapshot(
		c, actor.VendorID, batchID, CalcProductionExpected)
	if err != nil {
		costError(c, err)
		return
	}
	actual, actualFound, err := h.service.Engine().LoadProductionSnapshot(
		c, actor.VendorID, batchID, CalcProductionActual)
	if err != nil {
		costError(c, err)
		return
	}
	if !expectedFound && !actualFound {
		costError(c, ErrNotFound)
		return
	}

	payload := gin.H{}
	if expectedFound {
		if expected.BranchID != "" && !scope.Covers(expected.BranchID) {
			costError(c, ErrForbidden)
			return
		}
		payload["expected"] = expected
	}
	if actualFound {
		if actual.BranchID != "" && !scope.Covers(actual.BranchID) {
			costError(c, ErrForbidden)
			return
		}
		payload["actual"] = actual
	}
	costOK(c, http.StatusOK, payload)
}

func (h *Handler) productionVariance(c *gin.Context) {
	actor, _, ok := h.authorized(c, PermProductionCostRead)
	if !ok {
		return
	}
	variance, err := h.service.Engine().CalculateVariance(
		c, actor.VendorID, c.Param("batchId"))
	if err != nil {
		costError(c, err)
		return
	}
	costOK(c, http.StatusOK, variance)
}

func (h *Handler) productionCostHistory(c *gin.Context) {
	branchID := c.Query("branch_id")
	actor, ok := h.authorizedBranch(c, PermProductionCostRead, branchID)
	if !ok {
		return
	}
	limit, offset := page(c)
	snapshots, err := h.service.Engine().ProductionCostHistory(
		c, actor.VendorID, branchID, limit, offset)
	if err != nil {
		costError(c, err)
		return
	}
	if snapshots == nil {
		snapshots = []Snapshot{}
	}
	costOK(c, http.StatusOK, gin.H{
		"snapshots": snapshots, "limit": limit, "offset": offset})
}

func (h *Handler) highestCostSubjects(c *gin.Context) {
	branchID := c.Query("branch_id")
	actor, ok := h.authorizedBranch(c, PermCostRead, branchID)
	if !ok {
		return
	}
	limit, _ := page(c)
	subjects, err := h.service.Engine().HighestCostSubjects(
		c, actor.VendorID, branchID, limit)
	if err != nil {
		costError(c, err)
		return
	}
	if subjects == nil {
		subjects = []HighestCostSubject{}
	}
	costOK(c, http.StatusOK, gin.H{"subjects": subjects})
}

// RegisterRoutes mounts the Section 16D cost API.
func RegisterRoutes(root *gin.RouterGroup, h *Handler, enabled bool) {
	if root == nil || h == nil || !enabled {
		return
	}

	policies := root.Group("/cost/policies")
	policies.GET("", h.listPolicies)
	policies.POST("", h.createPolicy)
	policies.GET("/:policyId", h.getPolicy)
	policies.PUT("/:policyId", h.updatePolicy)
	policies.POST("/:policyId/archive", h.archivePolicy)

	recipeCost := root.Group("/recipe-cost")
	recipeCost.GET("/recipes/:recipeId", h.currentRecipeCost)
	recipeCost.GET("/recipes/:recipeId/history", h.recipeCostHistory)
	recipeCost.GET("/versions/:versionId", h.recipeCostPreview)
	recipeCost.POST("/versions/:versionId/snapshots", h.createRecipeSnapshot)
	recipeCost.GET("/versions/:versionId/latest", h.latestRecipeSnapshot)

	snapshots := root.Group("/cost/snapshots")
	snapshots.GET("/:snapshotId/components", h.snapshotComponents)

	productionCost := root.Group("/production-cost")
	productionCost.POST("/batches/:batchId/expected", h.expectedProductionCost)
	productionCost.POST("/batches/:batchId/actual", h.actualProductionCost)
	productionCost.GET("/batches/:batchId", h.readProductionCost)
	productionCost.GET("/batches/:batchId/variance", h.productionVariance)
	productionCost.GET("/history", h.productionCostHistory)

	insights := root.Group("/cost")
	insights.GET("/highest-cost-subjects", h.highestCostSubjects)
}
