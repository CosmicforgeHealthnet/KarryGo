package proof

import (
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
	"github.com/gin-gonic/gin"
	"net/http"
	"strconv"
	"strings"
)

type Route struct {
	Method, Path, OperationID, Permission, FeatureFlag, AuthClass string
	IdempotencyRequired, ExpectedVersionRequired                  bool
}

func TenantRoutes() []Route {
	return []Route{{"GET", "/proof-policies", "listProofPolicies", "proof.read", "PROOF_POLICIES_ENABLED", "corporate", false, false}, {"PUT", "/proof-policies", "putProofPolicy", "proof.manage_policies", "PROOF_POLICIES_ENABLED", "corporate", true, true}, {"GET", "/proof-policies/:policyId", "getProofPolicy", "proof.read", "PROOF_POLICIES_ENABLED", "corporate", false, false}, {"POST", "/proof-records", "createProofRecord", "proof.record", "PROOF_RECORDS_ENABLED", "corporate", true, false}, {"GET", "/proof-records", "listProofRecords", "proof.read", "PROOF_RECORDS_ENABLED", "corporate", false, false}}
}
func (r Route) FullPath() string { return "/fleet-partner/v1" + r.Path }

type Handler struct {
	s    *Service
	a    *accesscontrol.Authorizer
	idem *idempotency.Store
}

func NewHandler(s *Service, a *accesscontrol.Authorizer, i *idempotency.Store) *Handler {
	return &Handler{s: s, a: a, idem: i}
}
func (h *Handler) TenantRoutes() []Route { return TenantRoutes() }
func (h *Handler) RegisterTenantRoutes(g *gin.RouterGroup) {
	g.GET("/proof-policies", h.listPolicies)
	g.PUT("/proof-policies", h.putPolicy)
	g.GET("/proof-policies/:policyId", h.getPolicy)
	g.POST("/proof-records", h.createRecord)
	g.GET("/proof-records", h.listRecords)
}
func (h *Handler) gate(c *gin.Context, p string) (tenant.Context, bool) {
	t, ok := middleware.TenantFromGin(c)
	if !ok || h.a == nil {
		respond.Error(c, apperrors.Forbidden("Organization scope is required.", nil))
		return t, false
	}
	rv := int64(1)
	if x, ok := c.Get(middleware.TenantRoleVersionKey); ok {
		rv, _ = x.(int64)
	}
	if e := h.a.Require(c, t.OrganizationID, t.OrganizationUserID, rv, p, accesscontrol.Target{}); e != nil {
		respond.Error(c, e)
		return t, false
	}
	return t, true
}
func proofVer(c *gin.Context, v int64) int64 {
	if v > 0 {
		return v
	}
	n, _ := strconv.ParseInt(strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`), 10, 64)
	return n
}
func (h *Handler) listPolicies(c *gin.Context) {
	t, ok := h.gate(c, "proof.read")
	if !ok {
		return
	}
	v, e := h.s.ListPolicies(c, t)
	if e != nil {
		respond.Error(c, e)
		return
	}
	respond.OK(c, v)
}
func (h *Handler) getPolicy(c *gin.Context) {
	t, ok := h.gate(c, "proof.read")
	if !ok {
		return
	}
	v, e := h.s.GetPolicy(c, t, c.Param("policyId"))
	if e != nil {
		respond.Error(c, e)
		return
	}
	respond.OK(c, v)
}
func (h *Handler) putPolicy(c *gin.Context) {
	t, ok := h.gate(c, "proof.manage_policies")
	if !ok {
		return
	}
	idempotency.ExecuteHTTP(c, h.idem, t.OrganizationID, "PUT /proof-policies", func() {
		var b Policy
		if c.ShouldBindJSON(&b) != nil {
			respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
			return
		}
		v, s, e := h.s.SavePolicy(c, t, b.ID, proofVer(c, b.Version), b)
		if e != nil {
			respond.Error(c, e)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"policy": v, "snapshot": s}})
	})
}

type recordView struct {
	ID                      string `json:"id"`
	PolicySnapshotReference string `json:"policySnapshotReference"`
	ProofType               string `json:"proofType"`
	OrderID                 string `json:"orderId,omitempty"`
	ShipmentID              string `json:"shipmentId,omitempty"`
	PackageID               string `json:"packageId,omitempty"`
	RouteStopID             string `json:"routeStopId,omitempty"`
	AssignmentID            string `json:"assignmentId,omitempty"`
	CanonicalTripReference  string `json:"canonicalTripReference,omitempty"`
	ActorType               string `json:"actorType"`
	ActorReference          string `json:"actorReference"`
	MediaFileID             string `json:"mediaFileId,omitempty"`
	SafeValueMetadata       any    `json:"safeValueMetadata"`
	SourceTimestamp         any    `json:"sourceTimestamp"`
	ReceivedAt              any    `json:"receivedAt"`
	Source                  string `json:"source"`
	VerificationStatus      string `json:"verificationStatus"`
	Version                 int64  `json:"version"`
}

func view(r Record) recordView {
	return recordView{r.ID, r.PolicySnapshotReference, r.ProofType, r.OrderID, r.ShipmentID, r.PackageID, r.RouteStopID, r.AssignmentID, r.CanonicalTripReference, r.ActorType, r.ActorReference, r.MediaFileID, r.SafeValueMetadata, r.SourceTimestamp, r.ReceivedAt, r.Source, r.VerificationStatus, r.Version}
}
func (h *Handler) createRecord(c *gin.Context) {
	t, ok := h.gate(c, "proof.record")
	if !ok {
		return
	}
	idempotency.ExecuteHTTP(c, h.idem, t.OrganizationID, "POST /proof-records", func() {
		var b RecordInput
		if c.ShouldBindJSON(&b) != nil {
			respond.Error(c, apperrors.BadRequest("Request body must be valid JSON.", nil))
			return
		}
		b.IdempotencyKey = strings.TrimSpace(c.GetHeader("Idempotency-Key"))
		v, e := h.s.CreateRecord(c, t, b)
		if e != nil {
			respond.Error(c, e)
			return
		}
		c.JSON(http.StatusCreated, gin.H{"data": view(v)})
	})
}
func (h *Handler) listRecords(c *gin.Context) {
	t, ok := h.gate(c, "proof.read")
	if !ok {
		return
	}
	records, e := h.s.ListRecords(c, t)
	if e != nil {
		respond.Error(c, e)
		return
	}
	out := make([]recordView, 0, len(records))
	for _, r := range records {
		out = append(out, view(r))
	}
	respond.OK(c, out)
}
