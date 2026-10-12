package fulfilmentgroup

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/features/production"
	"github.com/gin-gonic/gin"
	"strconv"
)

type RiderBranch struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

func (s *Service) RiderBranches(ctx context.Context, actor production.Actor, limit, offset int) ([]RiderBranch, bool, error) {
	if err := s.guard(); err != nil {
		return nil, false, err
	}
	scope, err := s.authorize(ctx, actor, PermRiderExecute)
	if err != nil {
		return nil, false, err
	}
	if limit < 1 || limit > 200 || offset < 0 {
		return nil, false, ErrInvalid
	}
	rows, err := s.store.db.Query(ctx, `SELECT b.id,b.display_name FROM vendor_branches b
 WHERE b.business_id=$1 AND b.status='ACTIVE' AND ($3 OR b.id=ANY($4))
 AND EXISTS(SELECT 1 FROM vendor_staff st JOIN vendor_staff_role_assignments a
 ON a.staff_id=st.id AND a.revoked_at IS NULL WHERE st.id=$2 AND st.vendor_id=$1
 AND st.status='active' AND a.role_id='role_vendor_rider' AND (a.branch_id IS NULL OR a.branch_id=b.id))
 ORDER BY b.id OFFSET $5 LIMIT $6`, actor.VendorID, actor.StaffID, scope.Global, scope.BranchIDs, offset, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := []RiderBranch{}
	for rows.Next() {
		var b RiderBranch
		if err = rows.Scan(&b.ID, &b.DisplayName); err != nil {
			return nil, false, err
		}
		items = append(items, b)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(items) > limit
	if more {
		items = items[:limit]
	}
	return items, more, nil
}
func (h *Handler) riderBranches(c *gin.Context) {
	actor, valid := h.actor(c)
	if !valid {
		unauthorized(c)
		return
	}
	limit, offset := 200, 0
	var err error
	if raw := c.Query("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil {
			respondError(c, ErrInvalid)
			return
		}
	}
	if raw := c.Query("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil {
			respondError(c, ErrInvalid)
			return
		}
	}
	items, more, err := h.service.RiderBranches(c, actor, limit, offset)
	if err != nil {
		respondError(c, err)
		return
	}
	ok(c, gin.H{"items": items, "has_more": more, "next_offset": offset + limit})
}
