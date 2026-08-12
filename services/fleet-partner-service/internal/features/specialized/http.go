package specialized

import (
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
	"encoding/json"
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
	return []Route{{"GET", "/specialized/capabilities", "listSpecializedCapabilities", "specialized.read", "SPECIALIZED_LOGISTICS_ENABLED", "corporate", false, false}, {"POST", "/specialized/capabilities/:capabilityCode/enable", "enableSpecializedCapability", "specialized.manage", "SPECIALIZED_LOGISTICS_ENABLED", "corporate", true, true}, {"POST", "/specialized/capabilities/:capabilityCode/disable", "disableSpecializedCapability", "specialized.manage", "SPECIALIZED_LOGISTICS_ENABLED", "corporate", true, true}, {"GET", "/specialized/:capabilityCode/records", "listSpecializedRecords", "specialized.read", "SPECIALIZED_LOGISTICS_ENABLED", "corporate", false, false}, {"POST", "/specialized/:capabilityCode/records", "createSpecializedRecord", "specialized.manage", "SPECIALIZED_LOGISTICS_ENABLED", "corporate", true, false}, {"GET", "/specialized/:capabilityCode/records/:recordId", "getSpecializedRecord", "specialized.read", "SPECIALIZED_LOGISTICS_ENABLED", "corporate", false, false}, {"PATCH", "/specialized/:capabilityCode/records/:recordId", "updateSpecializedRecord", "specialized.manage", "SPECIALIZED_LOGISTICS_ENABLED", "corporate", false, true}, {"POST", "/specialized/:capabilityCode/records/:recordId/commands/:command", "commandSpecializedRecord", "specialized.manage", "SPECIALIZED_LOGISTICS_ENABLED", "corporate", true, true}, {"POST", "/specialized/:capabilityCode/records/:recordId/milestones", "addSpecializedMilestone", "specialized.manage", "SPECIALIZED_LOGISTICS_ENABLED", "corporate", true, false}, {"POST", "/specialized/COLD_CHAIN/records/:recordId/temperature-events", "addTemperatureEvent", "specialized.manage", "COLD_CHAIN_ENABLED", "corporate", true, false}}
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
	for _, r := range TenantRoutes() {
		r := r
		g.Handle(r.Method, r.Path, func(c *gin.Context) { h.handle(c, r) })
	}
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
	if e := h.a.Require(c, t.OrganizationID, t.OrganizationUserID, rv, p, accesscontrol.Target{BranchID: c.Query("branchId")}); e != nil {
		respond.Error(c, e)
		return t, false
	}
	return t, true
}
func specVer(c *gin.Context, v int64) int64 {
	if v > 0 {
		return v
	}
	n, _ := strconv.ParseInt(strings.Trim(strings.TrimSpace(c.GetHeader("If-Match")), `"`), 10, 64)
	return n
}

type capabilityBody struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason"`
	Configuration   any    `json:"configuration"`
}
type commandBody struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	Reason          string `json:"reason"`
}

func (h *Handler) handle(c *gin.Context, r Route) {
	t, ok := h.gate(c, r.Permission)
	if !ok {
		return
	}
	if r.IdempotencyRequired {
		idempotency.ExecuteHTTP(c, h.idem, t.OrganizationID, r.Method+" "+r.Path, func() { h.execute(c, t, r) })
		return
	}
	h.execute(c, t, r)
}

func (h *Handler) execute(c *gin.Context, t tenant.Context, r Route) {
	code := strings.ToUpper(c.Param("capabilityCode"))
	var v any
	var e error
	switch r.OperationID {
	case "listSpecializedCapabilities":
		v, e = h.s.ListCapabilities(c, t)
	case "enableSpecializedCapability", "disableSpecializedCapability":
		var b capabilityBody
		if c.ShouldBindJSON(&b) != nil {
			e = apperrors.BadRequest("Request body must be valid JSON.", nil)
			break
		}
		raw, _ := jsonMarshal(b.Configuration)
		status := "ENABLED"
		if r.OperationID == "disableSpecializedCapability" {
			status = "DISABLED"
		}
		v, e = h.s.SetCapability(c, t, code, status, b.Reason, specVer(c, b.ExpectedVersion), raw)
	case "listSpecializedRecords":
		v, e = h.s.ListRecords(c, t, code)
	case "createSpecializedRecord":
		var b Record
		if c.ShouldBindJSON(&b) != nil {
			e = apperrors.BadRequest("Request body must be valid JSON.", nil)
			break
		}
		b.CapabilityCode = code
		b.IdempotencyKey = c.GetHeader("Idempotency-Key")
		v, e = h.s.CreateRecord(c, t, b)
	case "getSpecializedRecord":
		v, e = h.s.GetRecord(c, t, code, c.Param("recordId"))
	case "updateSpecializedRecord":
		var b Record
		if c.ShouldBindJSON(&b) != nil {
			e = apperrors.BadRequest("Request body must be valid JSON.", nil)
			break
		}
		v, e = h.s.UpdateRecord(c, t, code, c.Param("recordId"), specVer(c, b.Version), b, "")
	case "commandSpecializedRecord":
		var b commandBody
		if c.ShouldBindJSON(&b) != nil {
			e = apperrors.BadRequest("Request body must be valid JSON.", nil)
			break
		}
		v, e = h.s.UpdateRecord(c, t, code, c.Param("recordId"), specVer(c, b.ExpectedVersion), Record{}, strings.ToUpper(c.Param("command")))
	case "addSpecializedMilestone":
		var b Milestone
		if c.ShouldBindJSON(&b) != nil {
			e = apperrors.BadRequest("Request body must be valid JSON.", nil)
			break
		}
		b.CorrelationID = c.GetHeader("Idempotency-Key")
		v, e = h.s.AddMilestone(c, t, code, c.Param("recordId"), b)
	case "addTemperatureEvent":
		var b TemperatureEvent
		if c.ShouldBindJSON(&b) != nil {
			e = apperrors.BadRequest("Request body must be valid JSON.", nil)
			break
		}
		v, e = h.s.AddTemperature(c, t, c.Param("recordId"), b)
	}
	if e != nil {
		respond.Error(c, e)
		return
	}
	status := http.StatusOK
	if strings.HasPrefix(r.OperationID, "create") || strings.HasPrefix(r.OperationID, "add") {
		status = http.StatusCreated
	}
	c.JSON(status, gin.H{"data": v})
}
func jsonMarshal(v any) ([]byte, error) {
	if v == nil {
		return []byte(`{}`), nil
	}
	return json.Marshal(v)
}
