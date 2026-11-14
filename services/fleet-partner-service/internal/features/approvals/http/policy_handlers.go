package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// PermissionPoliciesManage guards the approval-policy routes (spec 57.2 /
// 59.3) and lets its holder revoke any delegation.
const PermissionPoliciesManage = "approval_policies.manage"

// registerPolicyRoutes mounts the organization approval-policy routes.
func (h *Handler) registerPolicyRoutes(group *gin.RouterGroup) {
	group.GET("/approval-policies", h.listPolicies)
	group.POST("/approval-policies", h.createPolicy)
	group.GET("/approval-policies/:policyId", h.getPolicy)
	group.PATCH("/approval-policies/:policyId", h.updatePolicy)
	group.POST("/approval-policies/:policyId/archive", h.archivePolicy)
	group.GET("/approval-policies/:policyId/versions", h.policyVersions)
}

// registerDelegationRoutes mounts the approval-delegation routes.
func (h *Handler) registerDelegationRoutes(group *gin.RouterGroup) {
	group.GET("/approval-delegations", h.listDelegations)
	group.POST("/approval-delegations", h.createDelegation)
	group.GET("/approval-delegations/:delegationId", h.getDelegation)
	group.POST("/approval-delegations/:delegationId/revoke", h.revokeDelegation)
}

func (h *Handler) tenantWith(c *gin.Context, permission string) (tenant.Context, bool) {
	tenantCtx, ok := requireTenant(c)
	if !ok || !h.authorize(c, tenantCtx, permission) {
		return tenant.Context{}, false
	}
	return tenantCtx, true
}

func ifMatch(c *gin.Context) (int64, bool) {
	version, err := concurrency.ParseIfMatch(c.GetHeader("If-Match"))
	if err != nil {
		respond.Error(c, err)
		return 0, false
	}
	return version, true
}

// ---- policies ----

type stepBody struct {
	Quorum          int      `json:"quorum"`
	EligibleRoleIDs []string `json:"eligibleRoleIds"`
}

type policyBody struct {
	Code                 string     `json:"code"`
	Name                 *string    `json:"name"`
	Description          *string    `json:"description"`
	ResourceType         string     `json:"resourceType"`
	Action               string     `json:"action"`
	BranchID             *string    `json:"branchId"`
	ThresholdAmountMinor *int64     `json:"thresholdAmountMinor"`
	ThresholdCurrency    *string    `json:"thresholdCurrency"`
	ClearThreshold       bool       `json:"clearThreshold"`
	Steps                []stepBody `json:"steps"`
	RequesterMayApprove  *bool      `json:"requesterMayApprove"`
	SeparationOfDuties   *bool      `json:"separationOfDuties"`
	ExpirySeconds        *int       `json:"expirySeconds"`
	Reason               string     `json:"reason"`
}

func (b policyBody) steps() []models.StepSpec {
	if b.Steps == nil {
		return nil
	}
	steps := make([]models.StepSpec, 0, len(b.Steps))
	for _, step := range b.Steps {
		steps = append(steps, models.StepSpec{Quorum: step.Quorum, EligibleRoleIDs: step.EligibleRoleIDs})
	}
	return steps
}

func policyView(policy *models.Policy) gin.H {
	steps := make([]gin.H, 0, len(policy.Steps))
	for _, step := range policy.Steps {
		roles := step.EligibleRoleIDs
		if roles == nil {
			roles = []string{}
		}
		steps = append(steps, gin.H{"quorum": step.Quorum, "eligibleRoleIds": roles})
	}
	view := gin.H{
		"id": policy.ID, "organizationId": policy.OrganizationID, "code": policy.Code,
		"name": policy.Name, "description": policy.Description,
		"resourceType": policy.ResourceType, "action": policy.Action,
		"scopeType": policy.ScopeType, "branchId": policy.ScopeID,
		"thresholdAmountMinor": policy.ThresholdAmountMinor,
		"thresholdCurrency":    nil,
		"steps":                steps, "requesterMayApprove": policy.RequesterMayApprove,
		"separationOfDuties": policy.SeparationOfDuties, "expirySeconds": policy.ExpirySeconds,
		"origin": policy.Origin, "status": policy.Status, "version": policy.Version,
		"createdBy": policy.CreatedBy, "updatedBy": policy.UpdatedBy,
		"createdAt": policy.CreatedAt, "updatedAt": policy.UpdatedAt, "archivedAt": policy.ArchivedAt,
	}
	if policy.ThresholdCurrency != "" {
		view["thresholdCurrency"] = policy.ThresholdCurrency
	}
	return view
}

func policyInput(c *gin.Context, tenantCtx tenant.Context, body policyBody) usecases.PolicyInput {
	return usecases.PolicyInput{
		Actor:                actorFrom(tenantCtx, ""),
		PermittedBranchIDs:   tenantCtx.PermittedBranchIDs,
		PolicyID:             c.Param("policyId"),
		Code:                 body.Code,
		Name:                 body.Name,
		Description:          body.Description,
		ResourceType:         body.ResourceType,
		Action:               body.Action,
		BranchID:             body.BranchID,
		ThresholdAmountMinor: body.ThresholdAmountMinor,
		ThresholdCurrency:    body.ThresholdCurrency,
		ClearThreshold:       body.ClearThreshold,
		Steps:                body.steps(),
		RequesterMayApprove:  body.RequesterMayApprove,
		SeparationOfDuties:   body.SeparationOfDuties,
		ExpirySeconds:        body.ExpirySeconds,
		Reason:               body.Reason,
		IdempotencyKey:       c.GetHeader("Idempotency-Key"),
		CorrelationID:        middleware.GetCorrelationID(c),
	}
}

func (h *Handler) listPolicies(c *gin.Context) {
	tenantCtx, ok := h.tenantWith(c, PermissionPoliciesManage)
	if !ok {
		return
	}
	policies, err := h.service.ListPolicies(c.Request.Context(), tenantCtx.OrganizationID,
		tenantCtx.PermittedBranchIDs, repositories.PolicyFilter{
			ResourceType: c.Query("resourceType"), Action: c.Query("action"),
			Status: c.Query("status"), Origin: c.Query("origin")})
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]gin.H, 0, len(policies))
	for index := range policies {
		views = append(views, policyView(&policies[index]))
	}
	respond.List(c, views, "")
}

func (h *Handler) createPolicy(c *gin.Context) {
	tenantCtx, ok := h.tenantWith(c, PermissionPoliciesManage)
	if !ok {
		return
	}
	var body policyBody
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	result, err := h.service.CreatePolicy(c.Request.Context(), policyInput(c, tenantCtx, body))
	if err != nil {
		respond.Error(c, err)
		return
	}
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	c.JSON(status, gin.H{"data": policyView(result.Policy),
		"meta": gin.H{"replayed": result.Replayed}})
}

func (h *Handler) getPolicy(c *gin.Context) {
	tenantCtx, ok := h.tenantWith(c, PermissionPoliciesManage)
	if !ok {
		return
	}
	policy, err := h.service.GetPolicy(c.Request.Context(), tenantCtx.OrganizationID,
		tenantCtx.PermittedBranchIDs, c.Param("policyId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, policyView(policy))
}

func (h *Handler) updatePolicy(c *gin.Context) {
	tenantCtx, ok := h.tenantWith(c, PermissionPoliciesManage)
	if !ok {
		return
	}
	expected, ok := ifMatch(c)
	if !ok {
		return
	}
	var body policyBody
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	input := policyInput(c, tenantCtx, body)
	input.ExpectedVersion = expected
	policy, err := h.service.UpdatePolicy(c.Request.Context(), input)
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, policyView(policy))
}

type reasonBody struct {
	Reason string `json:"reason"`
}

func (h *Handler) archivePolicy(c *gin.Context) {
	tenantCtx, ok := h.tenantWith(c, PermissionPoliciesManage)
	if !ok {
		return
	}
	expected, ok := ifMatch(c)
	if !ok {
		return
	}
	var body reasonBody
	if err := c.ShouldBindJSON(&body); err != nil && c.Request.ContentLength > 0 {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	policy, err := h.service.ArchivePolicy(c.Request.Context(), usecases.PolicyInput{
		Actor: actorFrom(tenantCtx, ""), PermittedBranchIDs: tenantCtx.PermittedBranchIDs,
		PolicyID: c.Param("policyId"), Reason: body.Reason, ExpectedVersion: expected,
		CorrelationID: middleware.GetCorrelationID(c)})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, policyView(policy))
}

func (h *Handler) policyVersions(c *gin.Context) {
	tenantCtx, ok := h.tenantWith(c, PermissionPoliciesManage)
	if !ok {
		return
	}
	versions, err := h.service.PolicyVersions(c.Request.Context(), tenantCtx.OrganizationID,
		tenantCtx.PermittedBranchIDs, c.Param("policyId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]gin.H, 0, len(versions))
	for _, version := range versions {
		views = append(views, gin.H{
			"id": version.ID, "policyId": version.PolicyID, "version": version.Version,
			"status": version.Status, "rules": version.Rules, "changedBy": version.ChangedBy,
			"changeReason": version.ChangeReason, "createdAt": version.CreatedAt,
		})
	}
	respond.List(c, views, "")
}

// ---- delegations ----

type delegationBody struct {
	DelegateID    string     `json:"delegateId"`
	ResourceType  string     `json:"resourceType"`
	Action        string     `json:"action"`
	BranchID      string     `json:"branchId"`
	EffectiveFrom *time.Time `json:"effectiveFrom"`
	ExpiresAt     time.Time  `json:"expiresAt"`
	Reason        string     `json:"reason"`
}

func (h *Handler) delegationView(delegation *models.Delegation) gin.H {
	return gin.H{
		"id": delegation.ID, "organizationId": delegation.OrganizationID,
		"delegatorId": delegation.DelegatorID, "delegateId": delegation.DelegateID,
		"resourceType": delegation.ResourceType, "action": delegation.Action,
		"branchId": delegation.BranchID, "effectiveFrom": delegation.EffectiveFrom,
		"expiresAt": delegation.ExpiresAt, "reason": delegation.Reason,
		"status":    delegation.Status(h.service.Now()),
		"revokedAt": delegation.RevokedAt, "revokedBy": delegation.RevokedBy,
		"revocationReason": delegation.RevocationReason, "version": delegation.Version,
		"createdAt": delegation.CreatedAt, "updatedAt": delegation.UpdatedAt,
	}
}

func (h *Handler) listDelegations(c *gin.Context) {
	tenantCtx, ok := h.tenantWith(c, PermissionRead)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	delegations, err := h.service.ListDelegations(c.Request.Context(), tenantCtx.OrganizationID,
		tenantCtx.PermittedBranchIDs, repositories.DelegationFilter{
			DelegatorID: c.Query("delegatorId"), DelegateID: c.Query("delegateId"),
			Status: c.Query("status"), Limit: limit})
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]gin.H, 0, len(delegations))
	for index := range delegations {
		views = append(views, h.delegationView(&delegations[index]))
	}
	respond.List(c, views, "")
}

func (h *Handler) createDelegation(c *gin.Context) {
	tenantCtx, ok := h.tenantWith(c, PermissionDecide)
	if !ok {
		return
	}
	var body delegationBody
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	result, err := h.service.CreateDelegation(c.Request.Context(), usecases.DelegationInput{
		Actor: actorFrom(tenantCtx, ""), PermittedBranchIDs: tenantCtx.PermittedBranchIDs,
		DelegateID: body.DelegateID, ResourceType: body.ResourceType, Action: body.Action,
		BranchID: body.BranchID, EffectiveFrom: body.EffectiveFrom, ExpiresAt: body.ExpiresAt,
		Reason: body.Reason, IdempotencyKey: c.GetHeader("Idempotency-Key"),
		CorrelationID: middleware.GetCorrelationID(c)})
	if err != nil {
		respond.Error(c, err)
		return
	}
	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}
	c.JSON(status, gin.H{"data": h.delegationView(result.Delegation),
		"meta": gin.H{"replayed": result.Replayed}})
}

func (h *Handler) getDelegation(c *gin.Context) {
	tenantCtx, ok := h.tenantWith(c, PermissionRead)
	if !ok {
		return
	}
	delegation, err := h.service.GetDelegation(c.Request.Context(), tenantCtx.OrganizationID,
		tenantCtx.PermittedBranchIDs, c.Param("delegationId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, h.delegationView(delegation))
}

func (h *Handler) revokeDelegation(c *gin.Context) {
	tenantCtx, ok := h.tenantWith(c, PermissionDecide)
	if !ok {
		return
	}
	expected, ok := ifMatch(c)
	if !ok {
		return
	}
	var body reasonBody
	if err := c.ShouldBindJSON(&body); err != nil && c.Request.ContentLength > 0 {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	mayRevokeAny := false
	if decision, err := h.authorizer.Authorize(c.Request.Context(), tenantCtx.OrganizationID,
		tenantCtx.OrganizationUserID, roleVersionFrom(c), PermissionPoliciesManage,
		accesscontrol.Target{}); err == nil {
		mayRevokeAny = decision.Allowed
	}
	delegation, err := h.service.RevokeDelegation(c.Request.Context(), usecases.RevokeDelegationInput{
		Actor: actorFrom(tenantCtx, ""), PermittedBranchIDs: tenantCtx.PermittedBranchIDs,
		DelegationID: c.Param("delegationId"), ExpectedVersion: expected, Reason: body.Reason,
		MayRevokeAny: mayRevokeAny, CorrelationID: middleware.GetCorrelationID(c)})
	if err != nil {
		respond.Error(c, err)
		return
	}
	respond.OK(c, h.delegationView(delegation))
}
