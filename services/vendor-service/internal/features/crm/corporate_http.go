package crm

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/shared/go/httpx"
)

// Section 8G permission codes.
//
// Five, not nine. Corporate interactions and tasks deliberately reuse the
// Section 8C activity permissions: recording a call is the same authority
// whoever it was with, and a parallel corporate set would create roles that can
// log an individual call but not a corporate one for no reason anybody could
// explain. Reaching a corporate subject additionally requires
// PermCorporateRead, so the corporate wall is a separate grant without
// duplicating the activity vocabulary.
const (
	PermCorporateRead             = "vendor.crm.corporate.read"
	PermCorporateManage           = "vendor.crm.corporate.manage"
	PermCorporateContactsManage   = "vendor.crm.corporate.contacts.manage"
	PermCorporateLocationsManage  = "vendor.crm.corporate.locations.manage"
	PermCorporateCostCentersManag = "vendor.crm.corporate.cost_centers.manage"
)

const corporateMutationRateMax = 120

// CorporateHandler exposes the Section 8G vendor surface. It reuses the
// accepted Section 8B command edge — the same Idempotency-Key store, the same
// actor resolution, the same rate limiter — rather than inventing another.
type CorporateHandler struct {
	*SegmentationHandler
	limiter ActivityRateLimiter
}

// NewCorporateHandler wires the Section 8G handler.
func NewCorporateHandler(service *Service, store *idempotency.Store, limiters ...ActivityRateLimiter) *CorporateHandler {
	var limiter ActivityRateLimiter
	if len(limiters) > 0 {
		limiter = limiters[0]
	}
	return &CorporateHandler{SegmentationHandler: NewSegmentationHandler(service, store), limiter: limiter}
}

func (h *CorporateHandler) rateLimit(c *gin.Context, actor Actor, bucket string) error {
	if h.limiter == nil {
		return nil
	}
	return h.limiter.Allow(c, bucket, actor.VendorID+":"+actor.StaffID, corporateMutationRateMax, activityRateWindow)
}

// corporateActivityScope requires BOTH the Section 8C activity permission and
// corporate read. A caller who may log interactions does not thereby gain sight
// of which organizations this vendor works with.
func (h *CorporateHandler) corporateActivityScope(c *gin.Context, activityPermission string) (Actor, Scope, bool) {
	actor, scope, ok := h.authorized(c, activityPermission)
	if !ok {
		return Actor{}, Scope{}, false
	}
	if _, err := h.service.Authorize(c, actor, PermCorporateRead); err != nil {
		httpx.RespondError(c, err)
		return Actor{}, Scope{}, false
	}
	return actor, scope, true
}

func corporateID(c *gin.Context) string { return strings.TrimSpace(c.Param("corporateId")) }

// ---------------------------------------------------------------------------
// Wire shapes
// ---------------------------------------------------------------------------

type corporateProfileRequest struct {
	OrganizationName       string     `json:"organization_name"`
	DisplayName            string     `json:"display_name"`
	OrganizationType       string     `json:"organization_type"`
	ScopeMode              string     `json:"scope_mode"`
	BranchRefs             []string   `json:"branch_refs"`
	AccountManagerStaffRef string     `json:"account_manager_staff_ref"`
	ClearAccountManager    bool       `json:"clear_account_manager"`
	RelationshipStartedAt  *time.Time `json:"relationship_started_at"`
	ExternalOrganizationID string     `json:"external_organization_ref"`
	ExpectedVersion        int64      `json:"expected_version"`
}

func (r corporateProfileRequest) input() CorporateProfileInput {
	return CorporateProfileInput{
		OrganizationName: r.OrganizationName, DisplayName: r.DisplayName,
		OrganizationType: r.OrganizationType, ScopeMode: r.ScopeMode, BranchRefs: r.BranchRefs,
		AccountManagerStaffRef: r.AccountManagerStaffRef, ClearAccountManager: r.ClearAccountManager,
		RelationshipStartedAt: r.RelationshipStartedAt, ExternalOrganizationID: r.ExternalOrganizationID,
	}
}

type expectedVersionRequest struct {
	ExpectedVersion int64 `json:"expected_version"`
}

type branchScopeRequest struct {
	ScopeMode       string   `json:"scope_mode"`
	BranchRefs      []string `json:"branch_refs"`
	ExpectedVersion int64    `json:"expected_version"`
}

type corporateContactRequest struct {
	CustomerProfileRef string `json:"customer_profile_ref"`
	RelationshipRole   string `json:"relationship_role"`
	IsPrimary          *bool  `json:"is_primary_contact"`
	Status             string `json:"status"`
	ExpectedVersion    int64  `json:"expected_version"`
}

type corporateMembershipRequest struct {
	ContactRef      string `json:"contact_ref"`
	LocationRef     string `json:"location_ref"`
	CostCenterRef   string `json:"cost_center_ref"`
	ClearLocation   bool   `json:"clear_location"`
	ClearCostCenter bool   `json:"clear_cost_center"`
	MembershipRole  string `json:"membership_role"`
	Status          string `json:"status"`
	ExpectedVersion int64  `json:"expected_version"`
}

type corporateLocationRequest struct {
	Code                string `json:"location_code"`
	Name                string `json:"name"`
	LocationType        string `json:"location_type"`
	AddressLine1        string `json:"address_line1"`
	AddressLine2        string `json:"address_line2"`
	City                string `json:"city"`
	StateRegion         string `json:"state_region"`
	PostalCode          string `json:"postal_code"`
	CountryCode         string `json:"country_code"`
	Timezone            string `json:"timezone"`
	ExternalLocationRef string `json:"external_location_ref"`
	Status              string `json:"status"`
	ExpectedVersion     int64  `json:"expected_version"`
}

type corporateCostCenterRequest struct {
	Code              string `json:"code"`
	Name              string `json:"name"`
	Status            string `json:"status"`
	LocationRef       string `json:"location_ref"`
	ClearLocation     bool   `json:"clear_location"`
	ExternalReference string `json:"external_reference"`
	ExpectedVersion   int64  `json:"expected_version"`
}

type corporateInteractionRequest struct {
	Type          string     `json:"type"`
	Direction     string     `json:"direction"`
	Outcome       string     `json:"outcome"`
	Subject       string     `json:"subject"`
	Note          string     `json:"note"`
	OccurredAt    *time.Time `json:"occurred_at"`
	ContactRef    string     `json:"contact_ref"`
	LocationRef   string     `json:"location_ref"`
	CostCenterRef string     `json:"cost_center_ref"`
}

type corporateTaskRequest struct {
	Title                string     `json:"title"`
	Description          string     `json:"description"`
	Priority             string     `json:"priority"`
	AssignedStaffRef     string     `json:"assigned_staff_ref"`
	DueAt                *time.Time `json:"due_at"`
	ClearDueAt           bool       `json:"clear_due_at"`
	ContactRef           string     `json:"contact_ref"`
	LocationRef          string     `json:"location_ref"`
	CostCenterRef        string     `json:"cost_center_ref"`
	SourceInteractionRef string     `json:"source_interaction_ref"`
	ExpectedVersion      int64      `json:"expected_version"`
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

// CorporateView is the safe wire projection of an account. Organizational
// relationship data only; no individual identity appears anywhere on it.
type CorporateView struct {
	CorporateRef           string     `json:"corporate_ref"`
	CorporateCustomerCode  string     `json:"corporate_customer_code"`
	OrganizationName       string     `json:"organization_name"`
	DisplayName            *string    `json:"display_name,omitempty"`
	OrganizationType       string     `json:"organization_type"`
	Status                 string     `json:"status"`
	ScopeMode              string     `json:"scope_mode"`
	BranchRefs             []string   `json:"branch_refs"`
	AccountManagerStaffRef *string    `json:"account_manager_staff_ref,omitempty"`
	RelationshipStartedAt  *time.Time `json:"relationship_started_at,omitempty"`
	ExternalOrganizationID *string    `json:"external_organization_ref,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
	ArchivedAt             *time.Time `json:"archived_at,omitempty"`
	Version                int64      `json:"version"`
}

// CorporateViewOf renders an account for the wire.
func CorporateViewOf(c CorporateProfile, branches []string) CorporateView {
	if branches == nil {
		branches = []string{}
	}
	return CorporateView{
		CorporateRef: c.ID, CorporateCustomerCode: c.Code, OrganizationName: c.OrganizationName,
		DisplayName: c.DisplayName, OrganizationType: c.OrganizationType, Status: c.Status,
		ScopeMode: c.ScopeMode, BranchRefs: branches, AccountManagerStaffRef: c.AccountManagerStaffID,
		RelationshipStartedAt: c.RelationshipStartedAt, ExternalOrganizationID: c.ExternalOrganizationID,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, ArchivedAt: c.ArchivedAt, Version: c.Version,
	}
}

// CorporateContactView is the safe wire projection of a contact.
//
// It carries an opaque CRM profile reference, a role and a status. There is no
// email, phone or address field — not because one was omitted, but because
// Section 8G stores none and must not appear to.
type CorporateContactView struct {
	ContactRef         string     `json:"contact_ref"`
	CustomerProfileRef string     `json:"customer_profile_ref"`
	DisplayLabel       *string    `json:"display_label"`
	RelationshipRole   string     `json:"relationship_role"`
	IsPrimaryContact   bool       `json:"is_primary_contact"`
	Status             string     `json:"status"`
	PrivacySuppressed  bool       `json:"privacy_suppressed"`
	Actionable         bool       `json:"actionable"`
	CreatedAt          time.Time  `json:"created_at"`
	ArchivedAt         *time.Time `json:"archived_at,omitempty"`
	Version            int64      `json:"version"`
}

// CorporateContactViewOf renders a contact for the wire.
func CorporateContactViewOf(c CorporateContact) CorporateContactView {
	return CorporateContactView{
		ContactRef: c.ID, CustomerProfileRef: c.CustomerProfileID, DisplayLabel: c.SafeDisplayLabel,
		RelationshipRole: c.RelationshipRole, IsPrimaryContact: c.IsPrimary, Status: c.Status,
		PrivacySuppressed: c.PrivacySuppressed, Actionable: c.Actionable(),
		CreatedAt: c.CreatedAt, ArchivedAt: c.ArchivedAt, Version: c.Version,
	}
}

// CorporateMembershipView is the safe wire projection of a membership.
type CorporateMembershipView struct {
	MembershipRef  string     `json:"membership_ref"`
	ContactRef     string     `json:"contact_ref"`
	LocationRef    *string    `json:"location_ref,omitempty"`
	CostCenterRef  *string    `json:"cost_center_ref,omitempty"`
	MembershipRole string     `json:"membership_role"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	ArchivedAt     *time.Time `json:"archived_at,omitempty"`
	Version        int64      `json:"version"`
}

// CorporateMembershipViewOf renders a membership for the wire.
func CorporateMembershipViewOf(m CorporateMembership) CorporateMembershipView {
	return CorporateMembershipView{
		MembershipRef: m.ID, ContactRef: m.ContactID, LocationRef: m.LocationID, CostCenterRef: m.CostCenterID,
		MembershipRole: m.MembershipRole, Status: m.Status, CreatedAt: m.CreatedAt,
		ArchivedAt: m.ArchivedAt, Version: m.Version,
	}
}

// CorporateLocationView is the safe wire projection of a site.
type CorporateLocationView struct {
	LocationRef         string     `json:"location_ref"`
	LocationCode        string     `json:"location_code"`
	Name                string     `json:"name"`
	LocationType        string     `json:"location_type"`
	Status              string     `json:"status"`
	AddressLine1        *string    `json:"address_line1,omitempty"`
	AddressLine2        *string    `json:"address_line2,omitempty"`
	City                *string    `json:"city,omitempty"`
	StateRegion         *string    `json:"state_region,omitempty"`
	PostalCode          *string    `json:"postal_code,omitempty"`
	CountryCode         *string    `json:"country_code,omitempty"`
	Timezone            *string    `json:"timezone,omitempty"`
	ExternalLocationRef *string    `json:"external_location_ref,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	ArchivedAt          *time.Time `json:"archived_at,omitempty"`
	Version             int64      `json:"version"`
}

// CorporateLocationViewOf renders a site for the wire.
func CorporateLocationViewOf(l CorporateLocation) CorporateLocationView {
	return CorporateLocationView{
		LocationRef: l.ID, LocationCode: l.Code, Name: l.Name, LocationType: l.LocationType, Status: l.Status,
		AddressLine1: l.AddressLine1, AddressLine2: l.AddressLine2, City: l.City, StateRegion: l.StateRegion,
		PostalCode: l.PostalCode, CountryCode: l.CountryCode, Timezone: l.Timezone,
		ExternalLocationRef: l.ExternalLocationRef, CreatedAt: l.CreatedAt, ArchivedAt: l.ArchivedAt,
		Version: l.Version,
	}
}

// CorporateCostCenterView is the safe wire projection of a label.
//
// No amount, currency, limit or balance field exists on it, matching the table
// it comes from.
type CorporateCostCenterView struct {
	CostCenterRef     string     `json:"cost_center_ref"`
	Code              string     `json:"code"`
	Name              string     `json:"name"`
	Status            string     `json:"status"`
	LocationRef       *string    `json:"location_ref,omitempty"`
	ExternalReference *string    `json:"external_reference,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	ArchivedAt        *time.Time `json:"archived_at,omitempty"`
	Version           int64      `json:"version"`
}

// CorporateCostCenterViewOf renders a label for the wire.
func CorporateCostCenterViewOf(c CorporateCostCenter) CorporateCostCenterView {
	return CorporateCostCenterView{
		CostCenterRef: c.ID, Code: c.Code, Name: c.Name, Status: c.Status, LocationRef: c.LocationID,
		ExternalReference: c.ExternalReference, CreatedAt: c.CreatedAt, ArchivedAt: c.ArchivedAt,
		Version: c.Version,
	}
}

// CorporateInteractionView is the corporate projection of a Section 8C
// interaction: the same shape, with the corporate subject in place of the
// individual one.
type CorporateInteractionView struct {
	InteractionRef string    `json:"interaction_ref"`
	CorporateRef   string    `json:"corporate_ref"`
	ContactRef     *string   `json:"contact_ref,omitempty"`
	LocationRef    *string   `json:"location_ref,omitempty"`
	CostCenterRef  *string   `json:"cost_center_ref,omitempty"`
	Type           string    `json:"type"`
	Direction      string    `json:"direction"`
	Outcome        string    `json:"outcome"`
	Subject        string    `json:"subject"`
	Note           string    `json:"note"`
	OccurredAt     time.Time `json:"occurred_at"`
	RecordedByRef  string    `json:"recorded_by_staff_ref"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Version        int64     `json:"version"`
}

// CorporateInteractionViewOf renders a corporate interaction for the wire.
func CorporateInteractionViewOf(v Interaction) CorporateInteractionView {
	return CorporateInteractionView{
		InteractionRef: v.ID, CorporateRef: derefOr(v.CorporateCustomerID), ContactRef: v.CorporateContactID,
		LocationRef: v.CorporateLocationID, CostCenterRef: v.CorporateCostCenterID,
		Type: v.Type, Direction: v.Direction, Outcome: v.Outcome, Subject: v.Subject, Note: v.NoteBody,
		OccurredAt: v.OccurredAt, RecordedByRef: v.RecordedByStaffID, CreatedAt: v.CreatedAt,
		UpdatedAt: v.UpdatedAt, Version: v.Version,
	}
}

// CorporateTaskView is the corporate projection of a Section 8C task.
type CorporateTaskView struct {
	TaskRef              string     `json:"task_ref"`
	CorporateRef         string     `json:"corporate_ref"`
	ContactRef           *string    `json:"contact_ref,omitempty"`
	LocationRef          *string    `json:"location_ref,omitempty"`
	CostCenterRef        *string    `json:"cost_center_ref,omitempty"`
	Title                string     `json:"title"`
	Description          string     `json:"description"`
	Status               string     `json:"status"`
	Priority             string     `json:"priority"`
	AssignedStaffRef     *string    `json:"assigned_staff_ref,omitempty"`
	AssigneeActive       *bool      `json:"assignee_active,omitempty"`
	CreatedByStaffRef    string     `json:"created_by_staff_ref"`
	DueAt                *time.Time `json:"due_at,omitempty"`
	CompletedAt          *time.Time `json:"completed_at,omitempty"`
	CancelledAt          *time.Time `json:"cancelled_at,omitempty"`
	SourceInteractionRef *string    `json:"source_interaction_ref,omitempty"`
	IsOverdue            bool       `json:"is_overdue"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	Version              int64      `json:"version"`
}

// CorporateTaskViewOf renders a corporate task for the wire.
func CorporateTaskViewOf(t Task, now time.Time) CorporateTaskView {
	return CorporateTaskView{
		TaskRef: t.ID, CorporateRef: derefOr(t.CorporateCustomerID), ContactRef: t.CorporateContactID,
		LocationRef: t.CorporateLocationID, CostCenterRef: t.CorporateCostCenterID,
		Title: t.Title, Description: t.Description, Status: t.Status, Priority: t.Priority,
		AssignedStaffRef: t.AssignedStaffID, AssigneeActive: t.AssigneeActive,
		CreatedByStaffRef: t.CreatedByStaffID, DueAt: t.DueAt, CompletedAt: t.CompletedAt,
		CancelledAt: t.CancelledAt, SourceInteractionRef: t.SourceInteractionID,
		IsOverdue: t.IsOverdue(now), CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, Version: t.Version,
	}
}

// ---------------------------------------------------------------------------
// Profile handlers
// ---------------------------------------------------------------------------

// ListCorporateCustomers pages the accounts visible to the caller.
func (h *CorporateHandler) ListCorporateCustomers(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermCorporateRead)
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
	profiles, hasMore, err := h.service.ListCorporateCustomers(c, actor, scope, CorporateListOptions{
		Limit: limit, Offset: offset, Status: c.Query("status"), OrganizationType: c.Query("organization_type"),
		BranchRef: c.Query("branch_ref"), ManagerRef: c.Query("account_manager_staff_ref"),
		Search: c.Query("search"), CreatedFrom: from, CreatedTo: to,
		Sort: c.Query("sort"), Direction: c.Query("direction"),
	})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items := make([]CorporateView, 0, len(profiles))
	for _, profile := range profiles {
		// Branch scopes are not fetched per row: the list is a directory, and
		// an N+1 to decorate every entry would be paid on every page.
		items = append(items, CorporateViewOf(profile, nil))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"corporate_customers": items, "has_more": hasMore, "limit": limit, "offset": offset}})
}

// GetCorporateCustomer returns one account with its relationship summary.
func (h *CorporateHandler) GetCorporateCustomer(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermCorporateRead)
	if !ok {
		return
	}
	profile, err := h.service.GetCorporateCustomer(c, actor, scope, corporateID(c))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	summary, err := h.service.CorporateSummaryFor(c, actor, profile)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	recent, err := h.service.RecentCorporateInteractions(c, actor, profile.ID, 5)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	interactions := make([]CorporateInteractionView, 0, len(recent))
	for _, interaction := range recent {
		interactions = append(interactions, CorporateInteractionViewOf(interaction))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"corporate_customer": CorporateViewOf(profile, summary.BranchRefs),
		"summary": gin.H{
			"contact_count": summary.ContactCount, "location_count": summary.LocationCount,
			"cost_center_count": summary.CostCenterCount, "membership_count": summary.MembershipCount,
			"open_task_count": summary.OpenTaskCount, "overdue_task_count": summary.OverdueTaskCount,
			"commercial": summary.Commercial, "rfq": summary.RFQ,
		},
		"recent_interactions": interactions,
	}})
}

// CreateCorporateCustomer opens a corporate relationship.
func (h *CorporateHandler) CreateCorporateCustomer(c *gin.Context) {
	h.command(c, http.StatusCreated, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCorporateManage)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_corporate_manage"); err != nil {
			return nil, err
		}
		var req corporateProfileRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		profile, err := h.service.CreateCorporateCustomer(c, actor, scope, req.input())
		if err != nil {
			return nil, err
		}
		branches, err := h.service.BranchScopesFor(c, h.service.db, actor.VendorID, profile.ID)
		if err != nil {
			return nil, err
		}
		return CorporateViewOf(profile, branches), nil
	})
}

// UpdateCorporateCustomer edits an account's descriptive fields.
func (h *CorporateHandler) UpdateCorporateCustomer(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCorporateManage)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_corporate_manage"); err != nil {
			return nil, err
		}
		var req corporateProfileRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		profile, err := h.service.UpdateCorporateCustomer(c, actor, scope, corporateID(c),
			UpdateCorporateCustomerInput{Profile: req.input(), ExpectedVersion: req.ExpectedVersion})
		if err != nil {
			return nil, err
		}
		branches, err := h.service.BranchScopesFor(c, h.service.db, actor.VendorID, profile.ID)
		if err != nil {
			return nil, err
		}
		return CorporateViewOf(profile, branches), nil
	})
}

// transition is the shared body of the four lifecycle commands. Explicit action
// routes rather than a status PATCH, matching Sections 8C-8F, so a caller
// cannot ask for a jump the state machine would refuse without saying which
// jump they meant.
func (h *CorporateHandler) transition(c *gin.Context, target string) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCorporateManage)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_corporate_manage"); err != nil {
			return nil, err
		}
		var req expectedVersionRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		profile, err := h.service.TransitionCorporateCustomer(c, actor, scope, corporateID(c), target, req.ExpectedVersion)
		if err != nil {
			return nil, err
		}
		branches, err := h.service.BranchScopesFor(c, h.service.db, actor.VendorID, profile.ID)
		if err != nil {
			return nil, err
		}
		return CorporateViewOf(profile, branches), nil
	})
}

func (h *CorporateHandler) ActivateCorporateCustomer(c *gin.Context) {
	h.transition(c, CorporateActive)
}
func (h *CorporateHandler) HoldCorporateCustomer(c *gin.Context) { h.transition(c, CorporateOnHold) }
func (h *CorporateHandler) DeactivateCorporateCustomer(c *gin.Context) {
	h.transition(c, CorporateInactive)
}
func (h *CorporateHandler) ArchiveCorporateCustomer(c *gin.Context) {
	h.transition(c, CorporateArchived)
}

// GetCorporateBranchScopes lists the branches an account is limited to.
func (h *CorporateHandler) GetCorporateBranchScopes(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermCorporateRead)
	if !ok {
		return
	}
	profile, err := h.service.GetCorporateCustomer(c, actor, scope, corporateID(c))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	branches, err := h.service.BranchScopesFor(c, h.service.db, actor.VendorID, profile.ID)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"scope_mode": profile.ScopeMode, "branch_refs": branches, "version": profile.Version}})
}

// ReplaceCorporateBranchScopes sets the exact branches an account is limited to.
func (h *CorporateHandler) ReplaceCorporateBranchScopes(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCorporateManage)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_corporate_manage"); err != nil {
			return nil, err
		}
		var req branchScopeRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		profile, branches, err := h.service.ReplaceBranchScopes(c, actor, scope, corporateID(c),
			req.ScopeMode, req.BranchRefs, req.ExpectedVersion)
		if err != nil {
			return nil, err
		}
		return gin.H{"scope_mode": profile.ScopeMode, "branch_refs": branches, "version": profile.Version}, nil
	})
}

// ---------------------------------------------------------------------------
// Contact handlers
// ---------------------------------------------------------------------------

func (h *CorporateHandler) ListCorporateContacts(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermCorporateRead)
	if !ok {
		return
	}
	limit, offset, err := parsePage(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	contacts, hasMore, err := h.service.ListCorporateContacts(c, actor, scope, corporateID(c),
		c.Query("status"), c.Query("relationship_role"),
		strings.EqualFold(c.Query("include_privacy_suppressed"), "true"), limit, offset)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items := make([]CorporateContactView, 0, len(contacts))
	for _, contact := range contacts {
		items = append(items, CorporateContactViewOf(contact))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"contacts": items, "has_more": hasMore, "limit": limit, "offset": offset}})
}

func (h *CorporateHandler) GetCorporateContact(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermCorporateRead)
	if !ok {
		return
	}
	contact, err := h.service.GetCorporateContact(c, actor, scope, corporateID(c),
		strings.TrimSpace(c.Param("contactId")))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": CorporateContactViewOf(contact)})
}

func (h *CorporateHandler) CreateCorporateContact(c *gin.Context) {
	h.command(c, http.StatusCreated, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCorporateContactsManage)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_corporate_contacts"); err != nil {
			return nil, err
		}
		var req corporateContactRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		contact, err := h.service.CreateCorporateContact(c, actor, scope, corporateID(c), CorporateContactInput{
			CustomerProfileRef: req.CustomerProfileRef, RelationshipRole: req.RelationshipRole,
			IsPrimary: req.IsPrimary, Status: req.Status})
		if err != nil {
			return nil, err
		}
		return CorporateContactViewOf(contact), nil
	})
}

func (h *CorporateHandler) UpdateCorporateContact(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCorporateContactsManage)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_corporate_contacts"); err != nil {
			return nil, err
		}
		var req corporateContactRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		contact, err := h.service.UpdateCorporateContact(c, actor, scope, corporateID(c),
			strings.TrimSpace(c.Param("contactId")), CorporateContactInput{
				RelationshipRole: req.RelationshipRole, IsPrimary: req.IsPrimary,
				Status: req.Status, ExpectedVersion: req.ExpectedVersion})
		if err != nil {
			return nil, err
		}
		return CorporateContactViewOf(contact), nil
	})
}

func (h *CorporateHandler) ArchiveCorporateContact(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCorporateContactsManage)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_corporate_contacts"); err != nil {
			return nil, err
		}
		var req expectedVersionRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		contact, err := h.service.ArchiveCorporateContact(c, actor, scope, corporateID(c),
			strings.TrimSpace(c.Param("contactId")), req.ExpectedVersion)
		if err != nil {
			return nil, err
		}
		return CorporateContactViewOf(contact), nil
	})
}

// ---------------------------------------------------------------------------
// Membership handlers
// ---------------------------------------------------------------------------

func (h *CorporateHandler) ListCorporateMemberships(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermCorporateRead)
	if !ok {
		return
	}
	limit, offset, err := parsePage(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	memberships, hasMore, err := h.service.ListCorporateMemberships(c, actor, scope, corporateID(c),
		c.Query("contact_ref"), c.Query("status"), limit, offset)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items := make([]CorporateMembershipView, 0, len(memberships))
	for _, membership := range memberships {
		items = append(items, CorporateMembershipViewOf(membership))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"memberships": items, "has_more": hasMore, "limit": limit, "offset": offset}})
}

func (h *CorporateHandler) CreateCorporateMembership(c *gin.Context) {
	h.command(c, http.StatusCreated, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCorporateContactsManage)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_corporate_contacts"); err != nil {
			return nil, err
		}
		var req corporateMembershipRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		membership, err := h.service.CreateCorporateMembership(c, actor, scope, corporateID(c),
			CorporateMembershipInput{ContactRef: req.ContactRef, LocationRef: req.LocationRef,
				CostCenterRef: req.CostCenterRef, MembershipRole: req.MembershipRole})
		if err != nil {
			return nil, err
		}
		return CorporateMembershipViewOf(membership), nil
	})
}

func (h *CorporateHandler) UpdateCorporateMembership(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCorporateContactsManage)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_corporate_contacts"); err != nil {
			return nil, err
		}
		var req corporateMembershipRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		membership, err := h.service.UpdateCorporateMembership(c, actor, scope, corporateID(c),
			strings.TrimSpace(c.Param("membershipId")), CorporateMembershipInput{
				LocationRef: req.LocationRef, CostCenterRef: req.CostCenterRef,
				ClearLocation: req.ClearLocation, ClearCostCenter: req.ClearCostCenter,
				MembershipRole: req.MembershipRole, Status: req.Status, ExpectedVersion: req.ExpectedVersion})
		if err != nil {
			return nil, err
		}
		return CorporateMembershipViewOf(membership), nil
	})
}

func (h *CorporateHandler) ArchiveCorporateMembership(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCorporateContactsManage)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_corporate_contacts"); err != nil {
			return nil, err
		}
		var req expectedVersionRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		membership, err := h.service.ArchiveCorporateMembership(c, actor, scope, corporateID(c),
			strings.TrimSpace(c.Param("membershipId")), req.ExpectedVersion)
		if err != nil {
			return nil, err
		}
		return CorporateMembershipViewOf(membership), nil
	})
}

// ---------------------------------------------------------------------------
// Location handlers
// ---------------------------------------------------------------------------

func (h *CorporateHandler) ListCorporateLocations(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermCorporateRead)
	if !ok {
		return
	}
	limit, offset, err := parsePage(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	locations, hasMore, err := h.service.ListCorporateLocations(c, actor, scope, corporateID(c),
		c.Query("status"), limit, offset)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items := make([]CorporateLocationView, 0, len(locations))
	for _, location := range locations {
		items = append(items, CorporateLocationViewOf(location))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"locations": items, "has_more": hasMore, "limit": limit, "offset": offset}})
}

func (h *CorporateHandler) GetCorporateLocation(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermCorporateRead)
	if !ok {
		return
	}
	location, err := h.service.GetCorporateLocation(c, actor, scope, corporateID(c),
		strings.TrimSpace(c.Param("locationId")))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": CorporateLocationViewOf(location)})
}

func (h *CorporateHandler) CreateCorporateLocation(c *gin.Context) {
	h.command(c, http.StatusCreated, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCorporateLocationsManage)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_corporate_locations"); err != nil {
			return nil, err
		}
		var req corporateLocationRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		location, err := h.service.CreateCorporateLocation(c, actor, scope, corporateID(c), locationInput(req))
		if err != nil {
			return nil, err
		}
		return CorporateLocationViewOf(location), nil
	})
}

func (h *CorporateHandler) UpdateCorporateLocation(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCorporateLocationsManage)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_corporate_locations"); err != nil {
			return nil, err
		}
		var req corporateLocationRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		location, err := h.service.UpdateCorporateLocation(c, actor, scope, corporateID(c),
			strings.TrimSpace(c.Param("locationId")), locationInput(req))
		if err != nil {
			return nil, err
		}
		return CorporateLocationViewOf(location), nil
	})
}

func locationInput(r corporateLocationRequest) CorporateLocationInput {
	return CorporateLocationInput{
		Code: r.Code, Name: r.Name, LocationType: r.LocationType, AddressLine1: r.AddressLine1,
		AddressLine2: r.AddressLine2, City: r.City, StateRegion: r.StateRegion, PostalCode: r.PostalCode,
		CountryCode: r.CountryCode, Timezone: r.Timezone, ExternalLocationRef: r.ExternalLocationRef,
		Status: r.Status, ExpectedVersion: r.ExpectedVersion,
	}
}

func (h *CorporateHandler) ArchiveCorporateLocation(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCorporateLocationsManage)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_corporate_locations"); err != nil {
			return nil, err
		}
		var req expectedVersionRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		location, err := h.service.ArchiveCorporateLocation(c, actor, scope, corporateID(c),
			strings.TrimSpace(c.Param("locationId")), req.ExpectedVersion)
		if err != nil {
			return nil, err
		}
		return CorporateLocationViewOf(location), nil
	})
}

// ---------------------------------------------------------------------------
// Cost centre handlers
// ---------------------------------------------------------------------------

func (h *CorporateHandler) ListCorporateCostCenters(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermCorporateRead)
	if !ok {
		return
	}
	limit, offset, err := parsePage(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	centers, hasMore, err := h.service.ListCorporateCostCenters(c, actor, scope, corporateID(c),
		c.Query("status"), limit, offset)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items := make([]CorporateCostCenterView, 0, len(centers))
	for _, center := range centers {
		items = append(items, CorporateCostCenterViewOf(center))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"cost_centers": items, "has_more": hasMore, "limit": limit, "offset": offset}})
}

func (h *CorporateHandler) GetCorporateCostCenter(c *gin.Context) {
	actor, scope, ok := h.authorized(c, PermCorporateRead)
	if !ok {
		return
	}
	center, err := h.service.GetCorporateCostCenter(c, actor, scope, corporateID(c),
		strings.TrimSpace(c.Param("costCenterId")))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": CorporateCostCenterViewOf(center)})
}

func (h *CorporateHandler) CreateCorporateCostCenter(c *gin.Context) {
	h.command(c, http.StatusCreated, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCorporateCostCentersManag)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_corporate_cost_centers"); err != nil {
			return nil, err
		}
		var req corporateCostCenterRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		center, err := h.service.CreateCorporateCostCenter(c, actor, scope, corporateID(c),
			CorporateCostCenterInput{Code: req.Code, Name: req.Name, Status: req.Status,
				LocationRef: req.LocationRef, ExternalReference: req.ExternalReference})
		if err != nil {
			return nil, err
		}
		return CorporateCostCenterViewOf(center), nil
	})
}

func (h *CorporateHandler) UpdateCorporateCostCenter(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCorporateCostCentersManag)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_corporate_cost_centers"); err != nil {
			return nil, err
		}
		var req corporateCostCenterRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		center, err := h.service.UpdateCorporateCostCenter(c, actor, scope, corporateID(c),
			strings.TrimSpace(c.Param("costCenterId")), CorporateCostCenterInput{
				Code: req.Code, Name: req.Name, Status: req.Status, LocationRef: req.LocationRef,
				ClearLocation: req.ClearLocation, ExternalReference: req.ExternalReference,
				ExpectedVersion: req.ExpectedVersion})
		if err != nil {
			return nil, err
		}
		return CorporateCostCenterViewOf(center), nil
	})
}

func (h *CorporateHandler) ArchiveCorporateCostCenter(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermCorporateCostCentersManag)
		if err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_corporate_cost_centers"); err != nil {
			return nil, err
		}
		var req expectedVersionRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		center, err := h.service.ArchiveCorporateCostCenter(c, actor, scope, corporateID(c),
			strings.TrimSpace(c.Param("costCenterId")), req.ExpectedVersion)
		if err != nil {
			return nil, err
		}
		return CorporateCostCenterViewOf(center), nil
	})
}

// ---------------------------------------------------------------------------
// Activity handlers
// ---------------------------------------------------------------------------

func (h *CorporateHandler) ListCorporateInteractions(c *gin.Context) {
	actor, scope, ok := h.corporateActivityScope(c, PermInteractionsRead)
	if !ok {
		return
	}
	limit, offset, err := parsePage(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	from, err := parseQueryTime(c, "occurred_from")
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	to, err := parseQueryTime(c, "occurred_to")
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	interactions, hasMore, err := h.service.ListCorporateInteractions(c, actor, scope, corporateID(c),
		InteractionListOptions{Limit: limit, Offset: offset, Type: c.Query("type"),
			Direction: c.Query("direction"), Outcome: c.Query("outcome"), Search: c.Query("search"),
			OccurredFrom: from, OccurredTo: to})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items := make([]CorporateInteractionView, 0, len(interactions))
	for _, interaction := range interactions {
		items = append(items, CorporateInteractionViewOf(interaction))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"interactions": items, "has_more": hasMore, "limit": limit, "offset": offset}})
}

func (h *CorporateHandler) GetCorporateInteraction(c *gin.Context) {
	actor, scope, ok := h.corporateActivityScope(c, PermInteractionsRead)
	if !ok {
		return
	}
	interaction, err := h.service.GetCorporateInteraction(c, actor, scope, corporateID(c),
		strings.TrimSpace(c.Param("interactionId")))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": CorporateInteractionViewOf(interaction)})
}

func (h *CorporateHandler) CreateCorporateInteraction(c *gin.Context) {
	h.command(c, http.StatusCreated, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermInteractionsCreate)
		if err != nil {
			return nil, err
		}
		if _, err = h.service.Authorize(c, actor, PermCorporateRead); err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_corporate_activity"); err != nil {
			return nil, err
		}
		var req corporateInteractionRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		occurred := h.service.now()
		if req.OccurredAt != nil {
			occurred = *req.OccurredAt
		}
		interaction, err := h.service.CreateCorporateInteraction(c, actor, scope, corporateID(c),
			CreateCorporateInteractionInput{Type: req.Type, Direction: req.Direction, Outcome: req.Outcome,
				Subject: req.Subject, Note: req.Note, OccurredAt: occurred, ContactRef: req.ContactRef,
				LocationRef: req.LocationRef, CostCenterRef: req.CostCenterRef})
		if err != nil {
			return nil, err
		}
		return CorporateInteractionViewOf(interaction), nil
	})
}

// CorrectCorporateInteraction appends a correction through the same immutable
// revision path individual interactions use.
func (h *CorporateHandler) CorrectCorporateInteraction(c *gin.Context) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermInteractionsManage)
		if err != nil {
			return nil, err
		}
		if _, err = h.service.Authorize(c, actor, PermCorporateRead); err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_corporate_activity"); err != nil {
			return nil, err
		}
		interactionID := strings.TrimSpace(c.Param("interactionId"))
		// The nested-resource proof: an interaction belonging to another
		// account, presented under this one's path, is refused before any
		// mutation is delegated.
		if err = h.service.assertCorporateOwnsInteraction(c, actor, scope, corporateID(c), interactionID); err != nil {
			return nil, err
		}
		var req correctInteractionRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		interaction, err := h.service.CorrectInteraction(c, actor, scope, interactionID, CorrectInteractionInput{
			Subject: req.Subject, Note: req.Note, Outcome: req.Outcome, OccurredAt: req.OccurredAt,
			CorrectionReason: req.CorrectionReason, ExpectedVersion: req.ExpectedVersion})
		if err != nil {
			return nil, err
		}
		return CorporateInteractionViewOf(interaction), nil
	})
}

func (h *CorporateHandler) ListCorporateTasks(c *gin.Context) {
	actor, scope, ok := h.corporateActivityScope(c, PermTasksRead)
	if !ok {
		return
	}
	limit, offset, err := parsePage(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	tasks, hasMore, err := h.service.ListCorporateTasks(c, actor, scope, corporateID(c), TaskListOptions{
		Limit: limit, Offset: offset, Status: c.Query("status"), Priority: c.Query("priority"),
		AssignedStaffRef: c.Query("assigned_staff_ref")})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	now := h.service.now()
	items := make([]CorporateTaskView, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, CorporateTaskViewOf(task, now))
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"tasks": items, "has_more": hasMore, "limit": limit, "offset": offset}})
}

func (h *CorporateHandler) GetCorporateTask(c *gin.Context) {
	actor, scope, ok := h.corporateActivityScope(c, PermTasksRead)
	if !ok {
		return
	}
	task, err := h.service.GetCorporateTask(c, actor, scope, corporateID(c), strings.TrimSpace(c.Param("taskId")))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": CorporateTaskViewOf(task, h.service.now())})
}

func (h *CorporateHandler) CreateCorporateTask(c *gin.Context) {
	h.command(c, http.StatusCreated, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, PermTasksCreate)
		if err != nil {
			return nil, err
		}
		if _, err = h.service.Authorize(c, actor, PermCorporateRead); err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_corporate_activity"); err != nil {
			return nil, err
		}
		var req corporateTaskRequest
		if err = decodeBody(raw, &req); err != nil {
			return nil, err
		}
		task, err := h.service.CreateCorporateTask(c, actor, scope, corporateID(c), CreateCorporateTaskInput{
			Title: req.Title, Description: req.Description, Priority: req.Priority,
			AssignedStaffRef: req.AssignedStaffRef, DueAt: req.DueAt, ContactRef: req.ContactRef,
			LocationRef: req.LocationRef, CostCenterRef: req.CostCenterRef,
			SourceInteractionRef: req.SourceInteractionRef})
		if err != nil {
			return nil, err
		}
		return CorporateTaskViewOf(task, h.service.now()), nil
	})
}

// corporateTaskCommand is the shared edge for every corporate task mutation.
// Each one proves the task belongs to the account in the URL and then delegates
// to the Section 8C state machine — there is no second implementation of it.
func (h *CorporateHandler) corporateTaskCommand(c *gin.Context, permission string, fn func(Actor, Scope, string, []byte) (Task, error)) {
	h.command(c, http.StatusOK, func(actor Actor, raw []byte) (any, error) {
		scope, err := h.service.Authorize(c, actor, permission)
		if err != nil {
			return nil, err
		}
		if _, err = h.service.Authorize(c, actor, PermCorporateRead); err != nil {
			return nil, err
		}
		if err = h.rateLimit(c, actor, "crm_corporate_activity"); err != nil {
			return nil, err
		}
		taskID := strings.TrimSpace(c.Param("taskId"))
		if err = h.service.assertCorporateOwnsTask(c, actor, scope, corporateID(c), taskID); err != nil {
			return nil, err
		}
		task, err := fn(actor, scope, taskID, raw)
		if err != nil {
			return nil, err
		}
		return CorporateTaskViewOf(task, h.service.now()), nil
	})
}

func (h *CorporateHandler) UpdateCorporateTask(c *gin.Context) {
	h.corporateTaskCommand(c, PermTasksManage, func(actor Actor, scope Scope, taskID string, raw []byte) (Task, error) {
		var req corporateTaskRequest
		if err := decodeBody(raw, &req); err != nil {
			return Task{}, err
		}
		in := UpdateTaskInput{DueAt: req.DueAt, ClearDueAt: req.ClearDueAt, ExpectedVersion: req.ExpectedVersion}
		if strings.TrimSpace(req.Title) != "" {
			in.Title = &req.Title
		}
		if req.Description != "" {
			in.Description = &req.Description
		}
		if strings.TrimSpace(req.Priority) != "" {
			in.Priority = &req.Priority
		}
		return h.service.UpdateTask(c, actor, scope, taskID, in)
	})
}

func (h *CorporateHandler) transitionCorporateTask(c *gin.Context, target string) {
	h.corporateTaskCommand(c, PermTasksManage, func(actor Actor, scope Scope, taskID string, raw []byte) (Task, error) {
		var req expectedVersionRequest
		if err := decodeBody(raw, &req); err != nil {
			return Task{}, err
		}
		return h.service.TransitionTask(c, actor, scope, taskID, target, req.ExpectedVersion)
	})
}

func (h *CorporateHandler) StartCorporateTask(c *gin.Context) {
	h.transitionCorporateTask(c, TaskInProgress)
}
func (h *CorporateHandler) CompleteCorporateTask(c *gin.Context) {
	h.transitionCorporateTask(c, TaskCompleted)
}
func (h *CorporateHandler) CancelCorporateTask(c *gin.Context) {
	h.transitionCorporateTask(c, TaskCancelled)
}

func (h *CorporateHandler) ReassignCorporateTask(c *gin.Context) {
	h.corporateTaskCommand(c, PermTasksManage, func(actor Actor, scope Scope, taskID string, raw []byte) (Task, error) {
		var req corporateTaskRequest
		if err := decodeBody(raw, &req); err != nil {
			return Task{}, err
		}
		return h.service.ReassignTask(c, actor, scope, taskID, ReassignTaskInput{
			AssignedStaffRef: req.AssignedStaffRef, ExpectedVersion: req.ExpectedVersion})
	})
}

// RegisterCorporateRoutes wires Section 8G.
//
// Forty-one routes and no internal one. Corporate contact privacy and identity
// merge are handled through the existing Section 8A event ingress and its
// PrivacyHandler port, so there is nothing for a corporate-events endpoint to
// receive — and no authoritative producer that would call one.
func RegisterCorporateRoutes(group *gin.RouterGroup, h *CorporateHandler, flags Flags) {
	if !flags.CRMEnabled || !flags.CorporateEnabled {
		return
	}
	corporate := group.Group("/crm/corporate-customers")

	corporate.GET("", h.ListCorporateCustomers)
	corporate.POST("", h.CreateCorporateCustomer)
	corporate.GET("/:corporateId", h.GetCorporateCustomer)
	corporate.PATCH("/:corporateId", h.UpdateCorporateCustomer)
	corporate.POST("/:corporateId/activate", h.ActivateCorporateCustomer)
	corporate.POST("/:corporateId/hold", h.HoldCorporateCustomer)
	corporate.POST("/:corporateId/deactivate", h.DeactivateCorporateCustomer)
	corporate.POST("/:corporateId/archive", h.ArchiveCorporateCustomer)

	corporate.GET("/:corporateId/branches", h.GetCorporateBranchScopes)
	corporate.PUT("/:corporateId/branches", h.ReplaceCorporateBranchScopes)

	corporate.GET("/:corporateId/contacts", h.ListCorporateContacts)
	corporate.POST("/:corporateId/contacts", h.CreateCorporateContact)
	corporate.GET("/:corporateId/contacts/:contactId", h.GetCorporateContact)
	corporate.PATCH("/:corporateId/contacts/:contactId", h.UpdateCorporateContact)
	corporate.POST("/:corporateId/contacts/:contactId/archive", h.ArchiveCorporateContact)

	corporate.GET("/:corporateId/memberships", h.ListCorporateMemberships)
	corporate.POST("/:corporateId/memberships", h.CreateCorporateMembership)
	corporate.PATCH("/:corporateId/memberships/:membershipId", h.UpdateCorporateMembership)
	corporate.POST("/:corporateId/memberships/:membershipId/archive", h.ArchiveCorporateMembership)

	corporate.GET("/:corporateId/locations", h.ListCorporateLocations)
	corporate.POST("/:corporateId/locations", h.CreateCorporateLocation)
	corporate.GET("/:corporateId/locations/:locationId", h.GetCorporateLocation)
	corporate.PATCH("/:corporateId/locations/:locationId", h.UpdateCorporateLocation)
	corporate.POST("/:corporateId/locations/:locationId/archive", h.ArchiveCorporateLocation)

	corporate.GET("/:corporateId/cost-centers", h.ListCorporateCostCenters)
	corporate.POST("/:corporateId/cost-centers", h.CreateCorporateCostCenter)
	corporate.GET("/:corporateId/cost-centers/:costCenterId", h.GetCorporateCostCenter)
	corporate.PATCH("/:corporateId/cost-centers/:costCenterId", h.UpdateCorporateCostCenter)
	corporate.POST("/:corporateId/cost-centers/:costCenterId/archive", h.ArchiveCorporateCostCenter)

	// Corporate activity is registered only when the Section 8C engine it
	// delegates to is itself enabled: a route that could only ever answer
	// "writes disabled" would be worse than not existing.
	if flags.InteractionsEnabled {
		corporate.GET("/:corporateId/interactions", h.ListCorporateInteractions)
		corporate.POST("/:corporateId/interactions", h.CreateCorporateInteraction)
		corporate.GET("/:corporateId/interactions/:interactionId", h.GetCorporateInteraction)
		corporate.POST("/:corporateId/interactions/:interactionId/correct", h.CorrectCorporateInteraction)
	}
	if flags.TasksEnabled {
		corporate.GET("/:corporateId/tasks", h.ListCorporateTasks)
		corporate.POST("/:corporateId/tasks", h.CreateCorporateTask)
		corporate.GET("/:corporateId/tasks/:taskId", h.GetCorporateTask)
		corporate.PATCH("/:corporateId/tasks/:taskId", h.UpdateCorporateTask)
		corporate.POST("/:corporateId/tasks/:taskId/start", h.StartCorporateTask)
		corporate.POST("/:corporateId/tasks/:taskId/complete", h.CompleteCorporateTask)
		corporate.POST("/:corporateId/tasks/:taskId/cancel", h.CancelCorporateTask)
		corporate.POST("/:corporateId/tasks/:taskId/reassign", h.ReassignCorporateTask)
	}
}
