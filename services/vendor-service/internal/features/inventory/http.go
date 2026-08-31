package inventory

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

type AccessScope struct {
	Global    bool
	BranchIDs []string
}

func (s AccessScope) Covers(
	branchID string,
) bool {
	if s.Global {
		return true
	}

	for _, allowed := range s.BranchIDs {

		if allowed == branchID {
			return true
		}
	}

	return false
}

type ScopeResolver interface {
	ResolveScope(
		ctx context.Context,
		staffID string,
		permission string,
	) (AccessScope, error)
}

type Handler struct {
	service *StockService
	scopes  ScopeResolver
}

func NewHandler(
	service *StockService,
	scopes ScopeResolver,
) *Handler {
	return &Handler{
		service: service,
		scopes:  scopes,
	}
}

func inventoryRespond(
	c *gin.Context,
	status int,
	data any,
) {
	c.JSON(
		status,
		gin.H{
			"success": true,
			"data":    data,
		},
	)
}

func (h *Handler) actor(
	c *gin.Context,
) (
	Actor,
	error,
) {
	staff, ok :=
		authctx.FromContext(c)

	if !ok {
		return Actor{},
			apperrors.Unauthorized(
				"Authentication is required.",
				nil,
			)
	}

	return Actor{
		StaffID: staff.StaffID,

		BusinessID: staff.VendorID,

		CorrelationID: httpx.GetRequestID(c),

		CausationID: c.GetHeader(
			"X-Causation-ID",
		),

		RequestID: httpx.GetRequestID(c),
	}, nil
}

func (h *Handler) authorizedBranch(
	c *gin.Context,
	branchID string,
) (
	Actor,
	BranchScope,
	bool,
) {
	actor, err :=
		h.actor(c)

	if err != nil {
		httpx.RespondError(c, err)

		return Actor{},
			BranchScope{},
			false
	}

	if h.scopes == nil {
		httpx.RespondError(
			c,
			apperrors.Forbidden(
				"Inventory access is not permitted.",
				nil,
			),
		)

		return Actor{},
			BranchScope{},
			false
	}

	access, err :=
		h.scopes.ResolveScope(
			c,
			actor.StaffID,
			PermissionInventoryRead,
		)

	if err != nil {
		httpx.RespondError(c, err)

		return Actor{},
			BranchScope{},
			false
	}

	if !access.Covers(branchID) {
		httpx.RespondError(
			c,
			apperrors.Forbidden(
				"Inventory access is not permitted for this branch.",
				nil,
			),
		)

		return Actor{},
			BranchScope{},
			false
	}

	scope, err :=
		ExactBranchScope(
			actor.BusinessID,
			branchID,
		)

	if err != nil {
		httpx.RespondError(c, err)

		return Actor{},
			BranchScope{},
			false
	}

	return actor,
		scope,
		true
}

func pageValues(
	c *gin.Context,
) (
	int,
	int,
	error,
) {
	limit := 100
	offset := 0

	var err error

	if raw := c.Query("limit"); raw != "" {

		limit, err =
			strconv.Atoi(raw)

		if err != nil ||
			limit <= 0 {
			return 0,
				0,
				apperrors.BadRequest(
					"limit must be a positive integer.",
					err,
				)
		}
	}

	if raw := c.Query("offset"); raw != "" {

		offset, err =
			strconv.Atoi(raw)

		if err != nil ||
			offset < 0 {
			return 0,
				0,
				apperrors.BadRequest(
					"offset must be a non-negative integer.",
					err,
				)
		}
	}

	if limit > 500 {
		limit = 500
	}

	return limit,
		offset,
		nil
}

func (h *Handler) balances(
	c *gin.Context,
) {
	branchID :=
		c.Param("branchId")

	actor,
		scope,
		ok :=
		h.authorizedBranch(
			c,
			branchID,
		)

	if !ok {
		return
	}

	limit,
		offset,
		err :=
		pageValues(c)

	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	items, err :=
		h.service.ListBalances(
			c,
			actor,
			scope,
			branchID,
			limit,
			offset,
		)

	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	inventoryRespond(
		c,
		http.StatusOK,
		gin.H{
			"items": items,
		},
	)
}

func (h *Handler) movements(
	c *gin.Context,
) {
	branchID :=
		c.Param("branchId")

	actor,
		scope,
		ok :=
		h.authorizedBranch(
			c,
			branchID,
		)

	if !ok {
		return
	}

	limit,
		offset,
		err :=
		pageValues(c)

	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	items, err :=
		h.service.ListMovements(
			c,
			actor,
			scope,
			branchID,
			MovementListOptions{
				MovementType: c.Query(
					"movement_type",
				),

				SourceRef: c.Query(
					"source_ref",
				),

				Limit: limit,

				Offset: offset,
			},
		)

	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	inventoryRespond(
		c,
		http.StatusOK,
		gin.H{
			"items": items,
		},
	)
}

func (h *Handler) availability(
	c *gin.Context,
) {
	actor, err :=
		h.actor(c)

	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	itemID :=
		c.Param(
			"inventoryItemId",
		)

	branchID, err :=
		h.service.ItemBranch(
			c,
			actor.BusinessID,
			itemID,
		)

	if err != nil {
		if errors.Is(
			err,
			ErrItemNotFound,
		) {
			httpx.RespondError(
				c,
				apperrors.NotFound(
					"Inventory item was not found.",
					err,
				),
			)

			return
		}

		httpx.RespondError(c, err)
		return
	}

	actor,
		scope,
		ok :=
		h.authorizedBranch(
			c,
			branchID,
		)

	if !ok {
		return
	}

	item, err :=
		h.service.Availability(
			c,
			actor,
			scope,
			itemID,
		)

	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	inventoryRespond(
		c,
		http.StatusOK,
		item,
	)
}

func (h *Handler) revision(
	c *gin.Context,
) {
	branchID :=
		c.Param("branchId")

	actor,
		scope,
		ok :=
		h.authorizedBranch(
			c,
			branchID,
		)

	if !ok {
		return
	}

	revision,
		updatedAt,
		err :=
		h.service.Revision(
			c,
			actor,
			scope,
			branchID,
		)

	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	inventoryRespond(
		c,
		http.StatusOK,
		gin.H{
			"revision": revision,

			"updated_at": updatedAt,

			"domain": "INVENTORY",
		},
	)
}

func RegisterRoutes(
	root *gin.RouterGroup,
	h *Handler,
	flags Flags,
) {
	if !flags.Enabled ||
		h == nil {
		return
	}

	inventory :=
		root.Group(
			"/inventory",
		)

	inventory.GET(
		"/branches/:branchId/balances",
		h.balances,
	)

	inventory.GET(
		"/branches/:branchId/movements",
		h.movements,
	)

	inventory.GET(
		"/items/:inventoryItemId/availability",
		h.availability,
	)

	inventory.GET(
		"/branches/:branchId/revision",
		h.revision,
	)
}
