// Package http exposes the corporate eligibility API.
//
// Every route is tenant-scoped, branch-filtered and permission-gated, and the
// permissions are deliberately separate: reading an eligibility answer, forcing
// a re-evaluation and publishing the policy that decides the answer are three
// different powers. Whoever can rewrite the policy can make anything eligible.
package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Permissions guarding the routes.
const (
	PermissionDecisionRead    = "eligibility.decisions.read"
	PermissionDecisionRefresh = "eligibility.decisions.refresh"
	PermissionPolicyRead      = "eligibility.policies.read"
	PermissionPolicyManage    = "eligibility.policies.manage"
)

// Handler serves the corporate eligibility routes.
type Handler struct {
	service    *usecases.Service
	authorizer *accesscontrol.Authorizer
}

// NewHandler builds the handler.
func NewHandler(service *usecases.Service, authorizer *accesscontrol.Authorizer) *Handler {
	return &Handler{service: service, authorizer: authorizer}
}

// RegisterTenantRoutes mounts routes that require resolved organization scope.
func (h *Handler) RegisterTenantRoutes(group *gin.RouterGroup) {
	group.POST("/eligibility/evaluations", h.evaluate)
	group.GET("/eligibility/decisions", h.listDecisions)
	group.GET("/eligibility/decisions/:decisionId", h.getDecision)
	group.GET("/eligibility/decisions/:decisionId/history", h.decisionHistory)
	group.GET("/eligibility/policies", h.listPolicies)
	group.POST("/eligibility/policies", h.publishPolicy)
}

// requireTenant resolves the tenant context or fails closed.
func requireTenant(c *gin.Context) (tenant.Context, bool) {
	tenantCtx, ok := middleware.TenantFromGin(c)
	if !ok {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return tenant.Context{}, false
	}
	return tenantCtx, true
}

// authorize enforces a permission for the caller's membership.
func (h *Handler) authorize(c *gin.Context, tenantCtx tenant.Context, permission string,
	branchID string) bool {
	if h.authorizer == nil {
		// No authorizer means no way to prove authority. A missing gate must
		// never read as an open one.
		respond.Error(c, apperrors.Forbidden("You do not have permission to perform this action.", nil))
		return false
	}
	// A branch-scoped operator acting on a branch outside their scope is
	// refused here, before the service sees the request.
	if branchID != "" && !tenantCtx.BranchPermitted(branchID) {
		respond.Error(c, apperrors.NotFound("Branch not found.", nil))
		return false
	}
	if err := h.authorizer.Require(c.Request.Context(), tenantCtx.OrganizationID,
		tenantCtx.OrganizationUserID, roleVersionFrom(c), permission,
		accesscontrol.Target{BranchID: branchID}); err != nil {
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

// evaluateRequest is the corporate evaluation body.
type evaluateRequest struct {
	SubjectType         string `json:"subjectType"`
	SubjectID           string `json:"subjectId"`
	OperationType       string `json:"operationType"`
	BranchID            string `json:"branchId"`
	MembershipID        string `json:"membershipId"`
	ServiceCategory     string `json:"serviceCategory"`
	ExpectedVehicleType string `json:"expectedVehicleType"`
	AssetReference      string `json:"assetReference"`
	// Refresh forces a new evaluation even when a current decision exists.
	Refresh bool `json:"refresh"`
}

// DecisionView is the allowlisted response shape.
//
// Built field by field so a later column cannot become a response field by
// accident. Reasons are controlled codes, never free text, and no
// authoritative payload is echoed back.
type DecisionView struct {
	ID            string       `json:"id"`
	SubjectType   string       `json:"subjectType"`
	SubjectID     string       `json:"subjectId"`
	OperationType string       `json:"operationType"`
	BranchID      string       `json:"branchId,omitempty"`
	Status        string       `json:"status"`
	Authorizes    bool         `json:"authorizes"`
	Reasons       []string     `json:"reasons"`
	Sources       []SourceView `json:"sources"`
	PolicyKey     string       `json:"policyKey"`
	PolicyVersion int64        `json:"policyVersion"`
	CheckedAt     string       `json:"checkedAt"`
	ExpiresAt     string       `json:"expiresAt"`
	Superseded    bool         `json:"superseded"`
	CorrelationID string       `json:"correlationId,omitempty"`
}

// SourceView names an authority consulted and the version it answered at.
type SourceView struct {
	AuthorityService string `json:"authorityService"`
	SourceKind       string `json:"sourceKind"`
	SourceVersion    int64  `json:"sourceVersion"`
	FreshnessState   string `json:"freshnessState"`
	ObservedAt       string `json:"observedAt"`
}

const timeFormat = "2006-01-02T15:04:05Z07:00"

func newDecisionView(decision *models.Decision, now int64) DecisionView {
	view := DecisionView{
		ID:            decision.ID,
		SubjectType:   decision.SubjectType,
		SubjectID:     decision.SubjectID,
		OperationType: decision.OperationType,
		Status:        decision.Status,
		Reasons:       decision.Reasons,
		PolicyKey:     decision.PolicyKey,
		PolicyVersion: decision.PolicyVersion,
		CheckedAt:     decision.CheckedAt.UTC().Format(timeFormat),
		ExpiresAt:     decision.ExpiresAt.UTC().Format(timeFormat),
		Superseded:    !decision.Current(),
		CorrelationID: decision.CorrelationID,
	}
	if decision.BranchID != nil {
		view.BranchID = *decision.BranchID
	}
	if view.Reasons == nil {
		view.Reasons = []string{}
	}
	// Stated in the payload rather than left for the client to derive from
	// status and expiry. A client that has to combine three fields to learn
	// whether it may proceed will eventually combine them wrongly.
	view.Authorizes = decision.Status == models.StatusEligible && decision.Current() &&
		decision.ExpiresAt.UTC().Unix() > now
	view.Sources = make([]SourceView, 0, len(decision.Sources))
	for _, source := range decision.Sources {
		view.Sources = append(view.Sources, SourceView{
			AuthorityService: source.AuthorityService,
			SourceKind:       source.SourceKind,
			SourceVersion:    source.SourceVersion,
			FreshnessState:   source.FreshnessState,
			ObservedAt:       source.ObservedAt.UTC().Format(timeFormat),
		})
	}
	return view
}

// evaluate calculates or refreshes an eligibility decision.
func (h *Handler) evaluate(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}

	var body evaluateRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	if !h.authorize(c, tenantCtx, PermissionDecisionRefresh, body.BranchID) {
		return
	}

	input := usecases.EvaluateInput{
		Actor: usecases.Actor{
			OrganizationID: tenantCtx.OrganizationID,
			ActorType:      models.ActorOrganizationUser,
			ActorID:        tenantCtx.OrganizationUserID,
		},
		SubjectType:         body.SubjectType,
		SubjectID:           body.SubjectID,
		OperationType:       body.OperationType,
		BranchID:            body.BranchID,
		MembershipID:        body.MembershipID,
		ServiceCategory:     body.ServiceCategory,
		ExpectedVehicleType: body.ExpectedVehicleType,
		AssetReference:      body.AssetReference,
		CorrelationID:       middleware.GetCorrelationID(c),
		CausationID:         c.GetHeader("X-Causation-ID"),
	}

	var decision *models.Decision
	var err error
	if body.Refresh {
		decision, err = h.service.Evaluate(c.Request.Context(), input)
	} else {
		// Require returns the decision even when it blocks, so the caller can
		// see the reasons rather than only a refusal.
		decision, err = h.service.Require(c.Request.Context(), input)
		if err != nil && decision != nil {
			respond.OK(c, newDecisionView(decision, nowUnix(c)))
			return
		}
	}
	if err != nil {
		respond.Error(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": newDecisionView(decision, nowUnix(c)),
		"meta": gin.H{"requestId": c.GetHeader("X-Request-ID")}})
}

// listDecisions returns the organization's recorded decisions.
func (h *Handler) listDecisions(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionDecisionRead, "") {
		return
	}

	limit, _ := strconv.Atoi(c.Query("limit"))
	decisions, err := h.service.List(c.Request.Context(), tenant.Query{
		OrganizationID:     tenantCtx.OrganizationID,
		PermittedBranchIDs: tenantCtx.PermittedBranchIDs,
		Limit:              limit,
	}, repositories.ListFilter{
		SubjectType:   c.Query("subjectType"),
		SubjectID:     c.Query("subjectId"),
		OperationType: c.Query("operationType"),
		Status:        c.Query("status"),
		CurrentOnly:   c.Query("currentOnly") == "true",
	})
	if err != nil {
		respond.Error(c, err)
		return
	}

	now := nowUnix(c)
	views := make([]DecisionView, 0, len(decisions))
	for index := range decisions {
		views = append(views, newDecisionView(&decisions[index], now))
	}
	respond.OK(c, gin.H{"decisions": views})
}

// getDecision returns one recorded decision.
func (h *Handler) getDecision(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionDecisionRead, "") {
		return
	}
	decision, err := h.service.Get(c.Request.Context(), tenantCtx.OrganizationID,
		c.Param("decisionId"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	// A decision scoped to a branch the caller cannot see is reported as
	// missing rather than forbidden, so the identifier reveals nothing.
	if decision.BranchID != nil && !tenantCtx.BranchPermitted(*decision.BranchID) {
		respond.Error(c, apperrors.NotFound("Eligibility decision not found.", nil))
		return
	}
	respond.OK(c, newDecisionView(decision, nowUnix(c)))
}

// decisionHistory returns the append-only history for one decision.
func (h *Handler) decisionHistory(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionDecisionRead, "") {
		return
	}
	entries, err := h.service.History(c.Request.Context(), tenantCtx.OrganizationID,
		c.Param("decisionId"))
	if err != nil {
		respond.Error(c, err)
		return
	}

	views := make([]gin.H, 0, len(entries))
	for _, entry := range entries {
		views = append(views, gin.H{
			"id":         entry.ID,
			"action":     entry.Action,
			"fromStatus": entry.FromStatus,
			"toStatus":   entry.ToStatus,
			"actorType":  entry.ActorType,
			"reasonCode": entry.ReasonCode,
			"occurredAt": entry.OccurredAt,
		})
	}
	respond.OK(c, gin.H{"history": views})
}

// PolicyView is the allowlisted policy response.
type PolicyView struct {
	ID                      string   `json:"id"`
	Scope                   string   `json:"scope"`
	PolicyKey               string   `json:"policyKey"`
	OperationType           string   `json:"operationType"`
	Version                 int64    `json:"version"`
	Status                  string   `json:"status"`
	RequiredCompanyChecks   []string `json:"requiredCompanyChecks"`
	RequiredProviderChecks  []string `json:"requiredProviderChecks"`
	RequiredVehicleChecks   []string `json:"requiredVehicleChecks"`
	RequiredDocuments       []string `json:"requiredDocuments"`
	DecisionTTLSeconds      int      `json:"decisionTtlSeconds"`
	SourceMaxAgeSeconds     int      `json:"sourceMaxAgeSeconds"`
	ApprovalRequired        bool     `json:"approvalRequired"`
	MinimumApprovals        int      `json:"minimumApprovals"`
	EligibleRoleIDs         []string `json:"eligibleRoleIds"`
	SelfApprovalAllowed     bool     `json:"selfApprovalAllowed"`
	SeparationOfDuties      bool     `json:"separationOfDuties"`
	ApprovalTTLSeconds      int      `json:"approvalTtlSeconds"`
	AcknowledgementRequired bool     `json:"acknowledgementRequired"`
	SubscriptionRequired    bool     `json:"subscriptionRequired"`
	EffectiveFrom           string   `json:"effectiveFrom"`
}

func newPolicyView(policy *models.Policy) PolicyView {
	scope := "PLATFORM_DEFAULT"
	if policy.OrganizationID != "" {
		scope = "ORGANIZATION"
	}
	return PolicyView{
		ID:                      policy.ID,
		Scope:                   scope,
		PolicyKey:               policy.PolicyKey,
		OperationType:           policy.OperationType,
		Version:                 policy.Version,
		Status:                  policy.Status,
		RequiredCompanyChecks:   orEmpty(policy.RequiredCompanyChecks),
		RequiredProviderChecks:  orEmpty(policy.RequiredProviderChecks),
		RequiredVehicleChecks:   orEmpty(policy.RequiredVehicleChecks),
		RequiredDocuments:       orEmpty(policy.RequiredDocuments),
		DecisionTTLSeconds:      policy.DecisionTTLSeconds,
		SourceMaxAgeSeconds:     policy.SourceMaxAgeSeconds,
		ApprovalRequired:        policy.ApprovalRequired,
		MinimumApprovals:        policy.MinimumApprovals,
		EligibleRoleIDs:         orEmpty(policy.EligibleRoleIDs),
		SelfApprovalAllowed:     policy.SelfApprovalAllowed,
		SeparationOfDuties:      policy.SeparationOfDuties,
		ApprovalTTLSeconds:      policy.ApprovalTTLSeconds,
		AcknowledgementRequired: policy.AcknowledgementRequired,
		SubscriptionRequired:    policy.SubscriptionRequired,
		EffectiveFrom:           policy.EffectiveFrom.UTC().Format(timeFormat),
	}
}

// listPolicies returns the policy versions visible to the organization.
func (h *Handler) listPolicies(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionPolicyRead, "") {
		return
	}
	policies, err := h.service.ListPolicies(c.Request.Context(),
		tenantCtx.OrganizationID, c.Query("policyKey"))
	if err != nil {
		respond.Error(c, err)
		return
	}
	views := make([]PolicyView, 0, len(policies))
	for index := range policies {
		views = append(views, newPolicyView(&policies[index]))
	}
	respond.OK(c, gin.H{"policies": views})
}

// publishPolicyRequest is the corporate policy publication body.
type publishPolicyRequest struct {
	PolicyKey               string   `json:"policyKey"`
	OperationType           string   `json:"operationType"`
	RequiredCompanyChecks   []string `json:"requiredCompanyChecks"`
	RequiredProviderChecks  []string `json:"requiredProviderChecks"`
	RequiredVehicleChecks   []string `json:"requiredVehicleChecks"`
	RequiredDocuments       []string `json:"requiredDocuments"`
	DecisionTTLSeconds      int      `json:"decisionTtlSeconds"`
	SourceMaxAgeSeconds     int      `json:"sourceMaxAgeSeconds"`
	ApprovalRequired        bool     `json:"approvalRequired"`
	MinimumApprovals        int      `json:"minimumApprovals"`
	EligibleRoleIDs         []string `json:"eligibleRoleIds"`
	SelfApprovalAllowed     bool     `json:"selfApprovalAllowed"`
	SeparationOfDuties      *bool    `json:"separationOfDuties"`
	ApprovalTTLSeconds      int      `json:"approvalTtlSeconds"`
	AcknowledgementRequired bool     `json:"acknowledgementRequired"`
	SubscriptionRequired    bool     `json:"subscriptionRequired"`
}

// publishPolicy records a new tenant policy version.
func (h *Handler) publishPolicy(c *gin.Context) {
	tenantCtx, ok := requireTenant(c)
	if !ok {
		return
	}
	if !h.authorize(c, tenantCtx, PermissionPolicyManage, "") {
		return
	}

	var body publishPolicyRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}

	// Separation of duties defaults ON when unstated. A policy author who
	// forgets the field gets the safer rule, not the weaker one.
	separationOfDuties := true
	if body.SeparationOfDuties != nil {
		separationOfDuties = *body.SeparationOfDuties
	}

	policy, err := h.service.PublishPolicy(c.Request.Context(), usecases.Actor{
		OrganizationID: tenantCtx.OrganizationID,
		ActorType:      models.ActorOrganizationUser,
		ActorID:        tenantCtx.OrganizationUserID,
	}, &models.Policy{
		PolicyKey:               body.PolicyKey,
		OperationType:           body.OperationType,
		RequiredCompanyChecks:   body.RequiredCompanyChecks,
		RequiredProviderChecks:  body.RequiredProviderChecks,
		RequiredVehicleChecks:   body.RequiredVehicleChecks,
		RequiredDocuments:       body.RequiredDocuments,
		DecisionTTLSeconds:      body.DecisionTTLSeconds,
		SourceMaxAgeSeconds:     body.SourceMaxAgeSeconds,
		ApprovalRequired:        body.ApprovalRequired,
		MinimumApprovals:        body.MinimumApprovals,
		EligibleRoleIDs:         body.EligibleRoleIDs,
		SelfApprovalAllowed:     body.SelfApprovalAllowed,
		SeparationOfDuties:      separationOfDuties,
		ApprovalTTLSeconds:      body.ApprovalTTLSeconds,
		AcknowledgementRequired: body.AcknowledgementRequired,
		SubscriptionRequired:    body.SubscriptionRequired,
	})
	if err != nil {
		respond.Error(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": newPolicyView(policy)})
}

func orEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// nowUnix reads the request time once so every view in one response agrees
// about whether a decision has expired.
func nowUnix(c *gin.Context) int64 {
	if value, ok := c.Get("request_time_unix"); ok {
		if unix, ok := value.(int64); ok {
			return unix
		}
	}
	return time.Now().Unix()
}
