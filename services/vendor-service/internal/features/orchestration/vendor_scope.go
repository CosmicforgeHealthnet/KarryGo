package orchestration

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (h *Handler) scoped(c *gin.Context, permission string, manage bool) (Orchestration, []Pickup, bool) {
	staff, ok := authctx.FromContext(c)
	if !ok || staff.VendorID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": gin.H{"code": "UNAUTHORIZED", "message": "Authentication is required."}})
		return Orchestration{}, nil, false
	}
	current, err := h.repo.GetByGroup(c, c.Param("groupId"))
	if err != nil {
		respond(c, nil, err)
		return Orchestration{}, nil, false
	}
	pickups, err := h.repo.Pickups(c, current.ID)
	if err == nil {
		pickups, err = h.vendorPickups(c, staff.StaffID, staff.VendorID, permission, pickups, manage)
	}
	if err != nil {
		respond(c, nil, err)
		return Orchestration{}, nil, false
	}
	return current, pickups, true
}

func (h *Handler) vendorPickups(ctx context.Context, staff, vendor, permission string, pickups []Pickup, manage bool) ([]Pickup, error) {
	owned := []Pickup{}
	for _, pickup := range pickups {
		if pickup.BusinessID == vendor {
			owned = append(owned, pickup)
		}
	}
	if vendor == "" || len(owned) == 0 {
		return nil, ErrNotFound
	}
	if h.authorizer == nil {
		return nil, ErrForbidden
	}
	if manage && len(owned) != len(pickups) {
		return nil, ErrForbidden
	}
	visible := []Pickup{}
	for _, pickup := range owned {
		if pickup.BranchID == "" {
			return nil, ErrForbidden
		}
		allowed, err := h.authorizer.Authorize(ctx, staff, permission, &pickup.BranchID)
		if err != nil {
			return nil, err
		}
		if allowed {
			visible = append(visible, pickup)
		} else if manage {
			return nil, ErrForbidden
		}
	}
	if len(visible) == 0 {
		return nil, ErrForbidden
	}
	return visible, nil
}

func overrideStateError(current Orchestration) error {
	if current.State.Terminal() {
		return ErrTransition
	}
	if current.State.Committed() || current.State == StateCommitting || current.CurrentMissionRef != "" {
		return ErrCommitted
	}
	return nil
}
