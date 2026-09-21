package managedmembers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Permissions this feature checks.
const (
	PermissionMembersRead   = "managed_members.read"
	PermissionMembersManage = "managed_members.manage"
	PermissionAssetsRead    = "managed_assets.read"
	PermissionAssetsManage  = "managed_assets.manage"
)

// Route mirrors the shape the rest of this service uses to declare its
// corporate surface, so the route table stays one thing a reader can scan.
type Route struct {
	Method, Path, OperationID, Permission, FeatureFlag, AuthClass string
	IdempotencyRequired, ExpectedVersionRequired                  bool
}

// TenantRoutes is the organization-scoped surface.
func TenantRoutes() []Route {
	const flag = "FLEET_MULTI_SERVICE_PROVIDER_LINKS_ENABLED"
	return []Route{
		{"GET", "/managed-members", "listManagedMembers", PermissionMembersRead, flag, "corporate", false, false},
		{"POST", "/managed-members", "createManagedMember", PermissionMembersManage, flag, "corporate", true, false},
		{"GET", "/managed-members/:memberId", "getManagedMember", PermissionMembersRead, flag, "corporate", false, false},
		{"POST", "/managed-members/:memberId/links", "linkManagedMemberService", PermissionMembersManage, flag, "corporate", true, false},
		{"POST", "/managed-members/:memberId/suspend", "suspendManagedMember", PermissionMembersManage, flag, "corporate", false, true},
		{"POST", "/managed-members/:memberId/reinstate", "reinstateManagedMember", PermissionMembersManage, flag, "corporate", false, true},
		{"POST", "/managed-members/:memberId/consent", "grantManagedMemberConsent", PermissionMembersManage, flag, "corporate", false, true},
		{"GET", "/managed-assets", "listManagedAssets", PermissionAssetsRead, flag, "corporate", false, false},
		{"POST", "/managed-assets", "linkManagedAsset", PermissionAssetsManage, flag, "corporate", true, false},
		{"POST", "/managed-assets/:linkId/status", "setManagedAssetLinkStatus", PermissionAssetsManage, flag, "corporate", false, true},
	}
}

// Handler serves the member surface.
type Handler struct {
	s *Service
	a *accesscontrol.Authorizer
}

// NewHandler builds the handler.
func NewHandler(s *Service, a *accesscontrol.Authorizer) *Handler {
	return &Handler{s: s, a: a}
}

// TenantRoutes exposes the route table for the router and its contract test.
func (h *Handler) TenantRoutes() []Route { return TenantRoutes() }

// RegisterTenantRoutes mounts the surface on an already-authenticated group.
func (h *Handler) RegisterTenantRoutes(g *gin.RouterGroup) {
	handlers := map[string]gin.HandlerFunc{
		"listManagedMembers":        h.listMembers,
		"createManagedMember":       h.createMember,
		"getManagedMember":          h.getMember,
		"linkManagedMemberService":  h.linkService,
		"suspendManagedMember":      h.suspend,
		"reinstateManagedMember":    h.reinstate,
		"grantManagedMemberConsent": h.grantConsent,
		"listManagedAssets":         h.listAssets,
		"linkManagedAsset":          h.linkAsset,
		"setManagedAssetLinkStatus": h.setAssetStatus,
	}
	for _, route := range TenantRoutes() {
		if handler, ok := handlers[route.OperationID]; ok {
			g.Handle(route.Method, route.Path, handler)
		}
	}
}

// gate resolves the tenant and checks the permission.
//
// The organization comes from the authenticated session, never from the body
// or a query parameter. A body-supplied organization id is a tenant boundary
// one typo wide.
func (h *Handler) gate(c *gin.Context, permission string) (tenant.Context, bool) {
	t, ok := middleware.TenantFromGin(c)
	if !ok {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return t, false
	}
	if h.a == nil {
		respond.Error(c, apperrors.Forbidden("Permission denied.", nil))
		return t, false
	}
	roleVersion := int64(1)
	if raw, present := c.Get(middleware.TenantRoleVersionKey); present {
		roleVersion, _ = raw.(int64)
	}
	if err := h.a.Require(c, t.OrganizationID, t.OrganizationUserID, roleVersion, permission,
		accesscontrol.Target{}); err != nil {
		respond.Error(c, err)
		return t, false
	}
	return t, true
}

// write maps a domain error onto the wire.
//
// The two authority errors are separated deliberately. A refusal is 422: the
// owning service said no and no retry will change it. An outage is 503: the
// question was never answered, and treating it as a refusal would record a
// negative fact about a provider nobody was able to ask about.
func write(c *gin.Context, value any, err error) {
	if err == nil {
		respond.OK(c, value)
		return
	}
	switch {
	case errors.Is(err, ErrDisabled):
		respond.Error(c, apperrors.Unavailable("Multi-service provider links are not enabled.", err))
	case errors.Is(err, ErrNotFound):
		respond.Error(c, apperrors.NotFound("No such member or link.", err))
	case errors.Is(err, ErrVersion):
		respond.Error(c, apperrors.Conflict("The record moved since it was read; re-read and retry.", err))
	case errors.Is(err, ErrConflict):
		respond.Error(c, apperrors.Conflict("That provider is already linked in this organization.", err))
	case errors.Is(err, ErrAuthorityRefused):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "error": gin.H{
			"code": "AUTHORITY_REFUSED", "message": "The owning service declined this provider."}})
	case errors.Is(err, ErrAuthorityUnavailable):
		respond.Error(c, apperrors.Unavailable("The owning service could not be reached.", err))
	case errors.Is(err, ErrValidation):
		respond.Error(c, apperrors.BadRequest(err.Error(), err))
	default:
		respond.Error(c, err)
	}
}

type createMemberBody struct {
	MemberRef   string `json:"member_ref"`
	DisplayName string `json:"display_name"`
}

func (h *Handler) createMember(c *gin.Context) {
	t, ok := h.gate(c, PermissionMembersManage)
	if !ok {
		return
	}
	var body createMemberBody
	if c.ShouldBindJSON(&body) != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", nil))
		return
	}
	member, err := h.s.CreateMember(c, t, body.MemberRef, body.DisplayName)
	write(c, member, err)
}

func (h *Handler) listMembers(c *gin.Context) {
	t, ok := h.gate(c, PermissionMembersRead)
	if !ok {
		return
	}
	members, err := h.s.ListMembers(c, t, 0)
	write(c, gin.H{"members": members}, err)
}

func (h *Handler) getMember(c *gin.Context) {
	t, ok := h.gate(c, PermissionMembersRead)
	if !ok {
		return
	}
	view, err := h.s.ReadMember(c, t, c.Param("memberId"))
	write(c, view, err)
}

type linkServiceBody struct {
	Authority    string `json:"authority"`
	ProviderRef  string `json:"provider_ref"`
	MembershipID string `json:"membership_id"`
}

func (h *Handler) linkService(c *gin.Context) {
	t, ok := h.gate(c, PermissionMembersManage)
	if !ok {
		return
	}
	var body linkServiceBody
	if c.ShouldBindJSON(&body) != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", nil))
		return
	}
	authority, err := ParseAuthority(body.Authority)
	if err != nil {
		write(c, nil, err)
		return
	}
	link, err := h.s.LinkServiceIdentity(c, t, LinkRequest{
		MemberID: c.Param("memberId"), Authority: authority,
		ProviderRef: body.ProviderRef, MembershipID: body.MembershipID,
	})
	write(c, link, err)
}

type versionBody struct {
	ExpectedVersion int64 `json:"expected_version"`
}

func (h *Handler) suspend(c *gin.Context) {
	h.memberStatus(c, func(t tenant.Context, memberID string, version int64) (Member, error) {
		return h.s.SuspendMember(c, t, memberID, version)
	})
}

func (h *Handler) reinstate(c *gin.Context) {
	h.memberStatus(c, func(t tenant.Context, memberID string, version int64) (Member, error) {
		return h.s.ReinstateMember(c, t, memberID, version)
	})
}

func (h *Handler) grantConsent(c *gin.Context) {
	h.memberStatus(c, func(t tenant.Context, memberID string, version int64) (Member, error) {
		return h.s.GrantConsent(c, t, memberID, version)
	})
}

func (h *Handler) memberStatus(c *gin.Context, apply func(tenant.Context, string, int64) (Member, error)) {
	t, ok := h.gate(c, PermissionMembersManage)
	if !ok {
		return
	}
	var body versionBody
	if c.ShouldBindJSON(&body) != nil || body.ExpectedVersion < 1 {
		respond.Error(c, apperrors.BadRequest("expected_version is required.", nil))
		return
	}
	member, err := apply(t, c.Param("memberId"), body.ExpectedVersion)
	write(c, member, err)
}

type linkAssetBody struct {
	Authority     string `json:"authority"`
	AssetID       string `json:"asset_id"`
	MemberID      string `json:"member_id"`
	SourceVersion int64  `json:"source_version"`
}

func (h *Handler) linkAsset(c *gin.Context) {
	t, ok := h.gate(c, PermissionAssetsManage)
	if !ok {
		return
	}
	var body linkAssetBody
	if c.ShouldBindJSON(&body) != nil {
		respond.Error(c, apperrors.BadRequest("Request body is invalid.", nil))
		return
	}
	authority, err := ParseAuthority(body.Authority)
	if err != nil {
		write(c, nil, err)
		return
	}
	link, err := h.s.LinkAsset(c, t, LinkAssetRequest{
		Authority: authority, AssetID: body.AssetID,
		MemberID: body.MemberID, SourceVersion: body.SourceVersion,
	})
	write(c, link, err)
}

func (h *Handler) listAssets(c *gin.Context) {
	t, ok := h.gate(c, PermissionAssetsRead)
	if !ok {
		return
	}
	assets, err := h.s.ListAssets(c, t, c.Query("memberId"))
	write(c, gin.H{"assets": assets}, err)
}

type assetStatusBody struct {
	Status          string `json:"status"`
	ExpectedVersion int64  `json:"expected_version"`
}

func (h *Handler) setAssetStatus(c *gin.Context) {
	t, ok := h.gate(c, PermissionAssetsManage)
	if !ok {
		return
	}
	var body assetStatusBody
	if c.ShouldBindJSON(&body) != nil || body.ExpectedVersion < 1 {
		respond.Error(c, apperrors.BadRequest("expected_version is required.", nil))
		return
	}
	link, err := h.s.SetAssetLinkStatus(c, t, c.Param("linkId"), body.ExpectedVersion, body.Status)
	write(c, link, err)
}
