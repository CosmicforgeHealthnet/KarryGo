package pastry

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
func selectableCustomer(profile crm.Profile) bool {
	return profile.SubjectType == crm.SubjectCustomer && profile.Status == crm.StatusActive && profile.PrivacyState == crm.PrivacyActive && profile.RedactedAt == nil && profile.MergedIntoProfileID == nil && profile.PrimarySubjectRef() != "" && (profile.MergeState == crm.MergeNone || profile.MergeState == crm.MergeTarget)
}
func (h *Handler) requestReferences(c *gin.Context) {
	actor, pastryScope, ok := h.auth(c, PermManage)
	if !ok {
		return
	}
	branch, kind := c.Query("branch_id"), c.Query("kind")
	if branch == "" || !pastryScope.Covers(branch) || (kind != "CUSTOMER" && kind != "CORPORATE") {
		httpx.RespondError(c, apperrors.BadRequest("Choose a supported reference type and branch.", nil))
		return
	}
	var branchExists bool
	if err := h.s.DB().QueryRow(c, `SELECT EXISTS(SELECT 1 FROM vendor_branches WHERE business_id=$1 AND id=$2)`, actor.VendorID, branch).Scan(&branchExists); err != nil {
		httpx.RespondError(c, err)
		return
	}
	if !branchExists {
		httpx.RespondError(c, apperrors.BadRequest("Choose an available branch.", nil))
		return
	}
	if h.crm == nil {
		httpx.RespondError(c, apperrors.Unavailable("CRM reference discovery is unavailable.", nil))
		return
	}
	permission := crm.PermCustomersRead
	if kind == "CORPORATE" {
		permission = crm.PermCorporateRead
	}
	a := referenceActor(actor)
	scope, err := h.crm.Authorize(c, a, permission)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	if !scopeCovers(scope, branch) {
		httpx.RespondError(c, apperrors.Forbidden("This branch is outside your CRM access.", nil))
		return
	}
	limit, offset := 50, 0
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
	items := []gin.H{}
	more := false
	if kind == "CUSTOMER" {
		var profiles []crm.Profile
		profiles, more, err = h.crm.ListProfiles(c, a, scope, crm.ListOptions{Limit: limit, Offset: offset, BranchRef: branch, SubjectType: "CUSTOMER", Status: "ACTIVE", PrivacyState: "ACTIVE"})
		if err == nil {
			for _, p := range profiles {
				if selectableCustomer(p) {
					items = append(items, gin.H{"reference": p.PrimarySubjectRef(), "display_label": p.SafeDisplayLabel})
				}
			}
		}
	} else {
		var profiles []crm.CorporateProfile
		profiles, more, err = h.crm.ListCorporateCustomers(c, a, scope, crm.CorporateListOptions{Limit: limit, Offset: offset, Status: "ACTIVE"})
		if err == nil {
			for _, p := range profiles {
				allowed := p.ScopeMode == crm.CorporateScopeBusiness
				if !allowed {
					err = h.s.DB().QueryRow(c, `SELECT EXISTS(SELECT 1 FROM corporate_customer_branch_scopes WHERE business_id=$1 AND corporate_customer_id=$2 AND branch_id=$3)`, actor.VendorID, p.ID, branch).Scan(&allowed)
					if err != nil {
						break
					}
				}
				if allowed {
					items = append(items, gin.H{"reference": p.ID, "display_label": p.OrganizationName})
				}
			}
		}
	}
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"items": items, "has_more": more, "next_offset": offset + limit, "branch_id": branch, "kind": kind}})
}
func (h *Handler) validateRequestReferences(c *gin.Context, actor production.Actor, branch, customer, corporate string) bool {
	if customer == "" && corporate == "" {
		return true
	}
	reject := func() bool {
		httpx.RespondError(c, apperrors.BadRequest("The selected relationship is unavailable for this branch.", nil))
		return false
	}
	if h.crm == nil {
		return reject()
	}
	a := referenceActor(actor)
	if customer != "" {
		scope, err := h.crm.Authorize(c, a, crm.PermCustomersRead)
		if err != nil {
			httpx.RespondError(c, err)
			return false
		}
		if !scopeCovers(scope, branch) {
			return reject()
		}
		var profileID string
		err = h.s.DB().QueryRow(c, `SELECT p.id FROM vendor_customer_profiles p WHERE p.business_id=$1 AND p.customer_ref=$2 AND EXISTS(SELECT 1 FROM crm_customer_branch_relations r WHERE r.business_id=p.business_id AND r.profile_id=p.id AND r.branch_id=$3)`, actor.VendorID, customer, branch).Scan(&profileID)
		if err != nil {
			return reject()
		}
		p, err := h.crm.GetProfile(c, a, scope, profileID)
		if err != nil || !selectableCustomer(p) {
			return reject()
		}
	}
	if corporate != "" {
		scope, err := h.crm.Authorize(c, a, crm.PermCorporateRead)
		if err != nil {
			httpx.RespondError(c, err)
			return false
		}
		if !scopeCovers(scope, branch) {
			return reject()
		}
		p, err := h.crm.GetCorporateCustomer(c, a, scope, corporate)
		if err != nil || p.Status != crm.CorporateActive {
			return reject()
		}
		if p.ScopeMode != crm.CorporateScopeBusiness {
			var allowed bool
			err = h.s.DB().QueryRow(c, `SELECT EXISTS(SELECT 1 FROM corporate_customer_branch_scopes WHERE business_id=$1 AND corporate_customer_id=$2 AND branch_id=$3)`, actor.VendorID, p.ID, branch).Scan(&allowed)
			if err != nil || !allowed {
				return reject()
			}
		}
	}
	return true
}

func lockRequestReferences(ctx context.Context, tx pgx.Tx, business string, in CustomRequestInput) error {
	var id string
	if in.CustomerRef != "" {
		err := tx.QueryRow(ctx, `SELECT p.id FROM vendor_customer_profiles p WHERE p.business_id=$1 AND p.customer_ref=$2 AND p.subject_type='CUSTOMER' AND p.status='ACTIVE' AND p.privacy_state='ACTIVE' AND p.redacted_at IS NULL AND p.merged_into_profile_id IS NULL AND p.merge_state IN ('NONE','MERGED_TARGET') AND EXISTS(SELECT 1 FROM crm_customer_branch_relations r WHERE r.business_id=p.business_id AND r.profile_id=p.id AND r.branch_id=$3) FOR SHARE OF p`, business, in.CustomerRef, in.BranchID).Scan(&id)
		if err != nil {
			return apperrors.BadRequest("The selected customer is no longer available.", nil)
		}
	}
	if in.CorporateProfileRef != "" {
		err := tx.QueryRow(ctx, `SELECT c.id FROM corporate_customer_profiles c WHERE c.business_id=$1 AND c.id=$2 AND c.status='ACTIVE' AND (c.scope_mode='BUSINESS' OR EXISTS(SELECT 1 FROM corporate_customer_branch_scopes b WHERE b.business_id=c.business_id AND b.corporate_customer_id=c.id AND b.branch_id=$3)) FOR SHARE OF c`, business, in.CorporateProfileRef, in.BranchID).Scan(&id)
		if err != nil {
			return apperrors.BadRequest("The selected corporate account is no longer available.", nil)
		}
	}
	return nil
}
