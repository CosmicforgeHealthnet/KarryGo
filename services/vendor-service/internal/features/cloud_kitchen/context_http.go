package cloudkitchen

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/features/crm"
	"cosmicforge/logistics/services/vendor-service/internal/features/production"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"strconv"
)

func (h *Handler) UseCRM(service *crm.Service) *Handler { h.crm = service; return h }
func referenceActor(actor production.Actor) crm.Actor {
	return crm.Actor{VendorID: actor.VendorID, StaffID: actor.StaffID, SessionID: actor.SessionID}
}
func scopeCovers(scope crm.Scope, branch string) bool {
	if scope.Global {
		return true
	}
	for _, ref := range scope.BranchIDs {
		if ref == branch {
			return true
		}
	}
	return false
}

func (h *Handler) context(c *gin.Context) {
	a, scope, ok := h.auth(c, false)
	if !ok {
		return
	}
	branch := c.Param("branchId")
	if !scope.Covers(branch) {
		httpx.RespondError(c, apperrors.Forbidden("This kitchen is outside your access.", nil))
		return
	}
	var currency string
	err := h.service.DB().QueryRow(c, `SELECT b.currency FROM vendor_businesses b JOIN vendor_branches r ON r.business_id=b.id WHERE b.id=$1 AND r.id=$2`, a.VendorID, branch).Scan(&currency)
	if err != nil {
		httpx.RespondError(c, apperrors.NotFound("Kitchen context unavailable.", nil))
		return
	}
	productionOK(c, gin.H{"branch_id": branch, "currency": currency})
}
func (h *Handler) corporateScope(c *gin.Context, actor production.Actor, branch string) (crm.Scope, bool) {
	if h.crm == nil {
		httpx.RespondError(c, apperrors.Unavailable("Corporate selection is unavailable.", nil))
		return crm.Scope{}, false
	}
	scope, err := h.crm.Authorize(c, referenceActor(actor), crm.PermCorporateRead)
	if err != nil {
		httpx.RespondError(c, err)
		return scope, false
	}
	if !scopeCovers(scope, branch) {
		httpx.RespondError(c, apperrors.Forbidden("This kitchen is outside your corporate customer access.", nil))
		return scope, false
	}
	return scope, true
}
func (h *Handler) corporateAllowed(c *gin.Context, business, branch string, p crm.CorporateProfile) (bool, error) {
	if p.Status != crm.CorporateActive {
		return false, nil
	}
	if p.ScopeMode == crm.CorporateScopeBusiness {
		return true, nil
	}
	var allowed bool
	err := h.service.DB().QueryRow(c, `SELECT EXISTS(SELECT 1 FROM corporate_customer_branch_scopes WHERE business_id=$1 AND corporate_customer_id=$2 AND branch_id=$3)`, business, p.ID, branch).Scan(&allowed)
	return allowed, err
}
func (h *Handler) corporateReferences(c *gin.Context) {
	a, kitchenScope, ok := h.auth(c, true)
	if !ok {
		return
	}
	branch := c.Query("branch_id")
	if branch == "" || !kitchenScope.Covers(branch) {
		httpx.RespondError(c, apperrors.Forbidden("Choose an accessible kitchen.", nil))
		return
	}
	var exists bool
	if err := h.service.DB().QueryRow(c, `SELECT EXISTS(SELECT 1 FROM vendor_branches WHERE business_id=$1 AND id=$2)`, a.VendorID, branch).Scan(&exists); err != nil {
		httpx.RespondError(c, err)
		return
	}
	if !exists {
		httpx.RespondError(c, apperrors.NotFound("Kitchen not found.", nil))
		return
	}
	scope, ok := h.corporateScope(c, a, branch)
	if !ok {
		return
	}
	limit, offset := 50, 0
	var err error
	if raw := c.Query("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 200 {
			httpx.RespondError(c, apperrors.BadRequest("Invalid page size.", nil))
			return
		}
	}
	if raw := c.Query("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 {
			httpx.RespondError(c, apperrors.BadRequest("Invalid page offset.", nil))
			return
		}
	}
	profiles, more, err := h.crm.ListCorporateCustomers(c, referenceActor(a), scope, crm.CorporateListOptions{Limit: limit, Offset: offset, Status: "ACTIVE"})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items := []gin.H{}
	for _, p := range profiles {
		allowed, err := h.corporateAllowed(c, a.VendorID, branch, p)
		if err != nil {
			httpx.RespondError(c, err)
			return
		}
		if allowed {
			items = append(items, gin.H{"reference": p.ID, "display_label": p.OrganizationName})
		}
	}
	productionOK(c, gin.H{"items": items, "has_more": more, "next_offset": offset + limit, "branch_id": branch})
}
func (h *Handler) validateCorporate(c *gin.Context, a production.Actor, branch, ref string) bool {
	scope, ok := h.corporateScope(c, a, branch)
	if !ok {
		return false
	}
	p, err := h.crm.GetCorporateCustomer(c, referenceActor(a), scope, ref)
	if err == nil {
		var allowed bool
		allowed, err = h.corporateAllowed(c, a.VendorID, branch, p)
		if err == nil && allowed {
			return true
		}
	}
	httpx.RespondError(c, apperrors.BadRequest("Choose an active corporate customer available to this kitchen.", nil))
	return false
}

func lockCorporate(ctx context.Context, tx pgx.Tx, business, branch, ref string) error {
	var id string
	err := tx.QueryRow(ctx, `SELECT c.id FROM corporate_customer_profiles c WHERE c.business_id=$1 AND c.id=$2 AND c.status='ACTIVE' AND (c.scope_mode='BUSINESS' OR EXISTS(SELECT 1 FROM corporate_customer_branch_scopes b WHERE b.business_id=c.business_id AND b.corporate_customer_id=c.id AND b.branch_id=$3)) FOR SHARE OF c`, business, ref, branch).Scan(&id)
	if err != nil {
		return apperrors.BadRequest("The selected corporate customer is no longer available.", nil)
	}
	return nil
}
