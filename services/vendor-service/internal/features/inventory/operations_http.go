package inventory

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

func operationalHTTPError(
	err error,
) error {
	switch {
	case errors.Is(
		err,
		ErrStocktakeStale,
	):
		return apperrors.Conflict(
			"Inventory moved after this stocktake snapshot; recount is required.",
			err,
		)

	case errors.Is(
		err,
		ErrStocktakeStateConflict,
	),
		errors.Is(
			err,
			ErrTransferStateConflict,
		),
		errors.Is(
			err,
			ErrQuarantineStateConflict,
		):
		return apperrors.Conflict(
			"The inventory operation can no longer perform this transition.",
			err,
		)

	case errors.Is(
		err,
		ErrTransferIdempotencyConflict,
	):
		return apperrors.Conflict(
			"This transfer receipt source reference was already used with different details.",
			err,
		)

	case errors.Is(
		err,
		ErrTransferIdentityMismatch,
	):
		return apperrors.Conflict(
			"Source and destination inventory items do not represent the same stock identity.",
			err,
		)

	case errors.Is(
		err,
		ErrInsufficientStock,
	):
		return apperrors.Conflict(
			"Insufficient sellable inventory is available.",
			err,
		)

	case errors.Is(
		err,
		ErrStocktakeNotFound,
	),
		errors.Is(
			err,
			ErrTransferNotFound,
		),
		errors.Is(
			err,
			ErrQuarantineNotFound,
		),
		errors.Is(
			err,
			ErrAlertNotFound,
		):
		return apperrors.NotFound(
			"Inventory operation was not found.",
			err,
		)

	case errors.Is(
		err,
		ErrInvalidQuantity,
	),
		errors.Is(
			err,
			ErrInvalidSource,
		),
		errors.Is(
			err,
			ErrReorderRuleInvalid,
		),
		errors.Is(
			err,
			ErrReasonRequired,
		),
		errors.Is(
			err,
			ErrStocktakeCountIncomplete,
		):
		return apperrors.BadRequest(
			"The inventory operation request is invalid.",
			err,
		)

	default:
		return err
	}
}

func (h *Handler) authorizedTransferScope(
	c *gin.Context,
	sourceBranch string,
	destinationBranch string,
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
				"Inventory transfer access is not permitted.",
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
			PermissionInventoryTransfer,
		)

	if err != nil {
		httpx.RespondError(c, err)

		return Actor{},
			BranchScope{},
			false
	}

	if !access.Covers(
		sourceBranch,
	) ||
		!access.Covers(
			destinationBranch,
		) {

		httpx.RespondError(
			c,
			apperrors.Forbidden(
				"Inventory transfer access is not permitted for both branches.",
				nil,
			),
		)

		return Actor{},
			BranchScope{},
			false
	}

	scope, err :=
		BusinessScope(
			actor.BusinessID,
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

func (h *Handler) createStocktake(
	c *gin.Context,
) {
	var input CreateStocktakeInput

	if err := c.ShouldBindJSON(
		&input,
	); err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"Invalid stocktake request.",
				err,
			),
		)

		return
	}

	actor,
		scope,
		ok :=
		h.authorizedBranchPermission(
			c,
			input.BranchID,
			PermissionInventoryStocktake,
		)

	if !ok {
		return
	}

	out, err :=
		h.service.CreateStocktake(
			c,
			actor,
			scope,
			input,
		)

	if err != nil {
		httpx.RespondError(
			c,
			operationalHTTPError(
				err,
			),
		)

		return
	}

	inventoryRespond(
		c,
		http.StatusCreated,
		out,
	)
}

func (h *Handler) countStocktake(
	c *gin.Context,
) {
	actor, err :=
		h.actor(c)

	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	stocktakeID :=
		c.Param(
			"stocktakeId",
		)

	branchID, err :=
		h.service.StocktakeBranch(
			c,
			actor.BusinessID,
			stocktakeID,
		)

	if err != nil {
		httpx.RespondError(
			c,
			operationalHTTPError(
				err,
			),
		)

		return
	}

	actor,
		scope,
		ok :=
		h.authorizedBranchPermission(
			c,
			branchID,
			PermissionInventoryStocktake,
		)

	if !ok {
		return
	}

	var body struct {
		CountedQuantity int64 `json:"counted_quantity"`
	}

	if err := c.ShouldBindJSON(
		&body,
	); err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"Invalid stocktake count.",
				err,
			),
		)

		return
	}

	err =
		h.service.CountStocktakeLine(
			c,
			actor,
			scope,
			stocktakeID,
			c.Param(
				"inventoryItemId",
			),
			body.CountedQuantity,
		)

	if err != nil {
		httpx.RespondError(
			c,
			operationalHTTPError(
				err,
			),
		)

		return
	}

	inventoryRespond(
		c,
		http.StatusOK,
		gin.H{
			"counted": true,
		},
	)
}

func (h *Handler) stocktakeTransition(
	cancel bool,
) gin.HandlerFunc {
	return func(
		c *gin.Context,
	) {
		actor, err :=
			h.actor(c)

		if err != nil {
			httpx.RespondError(c, err)
			return
		}

		id :=
			c.Param(
				"stocktakeId",
			)

		branch, err :=
			h.service.StocktakeBranch(
				c,
				actor.BusinessID,
				id,
			)

		if err != nil {
			httpx.RespondError(
				c,
				operationalHTTPError(
					err,
				),
			)

			return
		}

		actor,
			scope,
			ok :=
			h.authorizedBranchPermission(
				c,
				branch,
				PermissionInventoryStocktake,
			)

		if !ok {
			return
		}

		var out Stocktake

		if cancel {
			out, err =
				h.service.CancelStocktake(
					c,
					actor,
					scope,
					id,
				)
		} else {
			out, err =
				h.service.PostStocktake(
					c,
					actor,
					scope,
					id,
				)
		}

		if err != nil {
			httpx.RespondError(
				c,
				operationalHTTPError(
					err,
				),
			)

			return
		}

		inventoryRespond(
			c,
			http.StatusOK,
			out,
		)
	}
}

func (h *Handler) createTransfer(
	c *gin.Context,
) {
	var input CreateTransferInput

	if err := c.ShouldBindJSON(
		&input,
	); err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"Invalid transfer request.",
				err,
			),
		)

		return
	}

	actor,
		scope,
		ok :=
		h.authorizedTransferScope(
			c,
			input.SourceBranchID,
			input.DestinationBranchID,
		)

	if !ok {
		return
	}

	out, err :=
		h.service.CreateTransfer(
			c,
			actor,
			scope,
			input,
		)

	if err != nil {
		httpx.RespondError(
			c,
			operationalHTTPError(
				err,
			),
		)

		return
	}

	inventoryRespond(
		c,
		http.StatusCreated,
		out,
	)
}

func (h *Handler) transferTransition(
	action string,
) gin.HandlerFunc {
	return func(
		c *gin.Context,
	) {
		actor, err :=
			h.actor(c)

		if err != nil {
			httpx.RespondError(c, err)
			return
		}

		id :=
			c.Param(
				"transferId",
			)

		source,
			destination,
			err :=
			h.service.TransferBranches(
				c,
				actor.BusinessID,
				id,
			)

		if err != nil {
			httpx.RespondError(
				c,
				operationalHTTPError(
					err,
				),
			)

			return
		}

		actor,
			scope,
			ok :=
			h.authorizedTransferScope(
				c,
				source,
				destination,
			)

		if !ok {
			return
		}

		var out Transfer

		switch action {
		case "APPROVE":
			out, err =
				h.service.ApproveTransfer(
					c,
					actor,
					scope,
					id,
				)

		case "DISPATCH":
			out, err =
				h.service.DispatchTransfer(
					c,
					actor,
					scope,
					id,
				)

		case "CANCEL":
			out, err =
				h.service.CancelTransfer(
					c,
					actor,
					scope,
					id,
				)

		default:
			err =
				ErrTransferStateConflict
		}

		if err != nil {
			httpx.RespondError(
				c,
				operationalHTTPError(
					err,
				),
			)

			return
		}

		inventoryRespond(
			c,
			http.StatusOK,
			out,
		)
	}
}

func (h *Handler) receiveTransfer(
	c *gin.Context,
) {
	actor, err :=
		h.actor(c)

	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	id :=
		c.Param(
			"transferId",
		)

	source,
		destination,
		err :=
		h.service.TransferBranches(
			c,
			actor.BusinessID,
			id,
		)

	if err != nil {
		httpx.RespondError(
			c,
			operationalHTTPError(
				err,
			),
		)

		return
	}

	actor,
		scope,
		ok :=
		h.authorizedTransferScope(
			c,
			source,
			destination,
		)

	if !ok {
		return
	}

	var input ReceiveTransferInput

	if err := c.ShouldBindJSON(
		&input,
	); err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"Invalid transfer receipt.",
				err,
			),
		)

		return
	}

	out, err :=
		h.service.ReceiveTransfer(
			c,
			actor,
			scope,
			id,
			input,
		)

	if err != nil {
		httpx.RespondError(
			c,
			operationalHTTPError(
				err,
			),
		)

		return
	}

	inventoryRespond(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) createQuarantine(
	c *gin.Context,
) {
	var input CreateQuarantineInput

	if err := c.ShouldBindJSON(
		&input,
	); err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"Invalid quarantine request.",
				err,
			),
		)

		return
	}

	actor,
		scope,
		ok :=
		h.authorizedBranchPermission(
			c,
			input.BranchID,
			PermissionInventoryQuarantine,
		)

	if !ok {
		return
	}

	out, err :=
		h.service.CreateQuarantine(
			c,
			actor,
			scope,
			input,
		)

	if err != nil {
		httpx.RespondError(
			c,
			operationalHTTPError(
				err,
			),
		)

		return
	}

	inventoryRespond(
		c,
		http.StatusCreated,
		out,
	)
}

func (h *Handler) quarantineTransition(
	dispose bool,
) gin.HandlerFunc {
	return func(
		c *gin.Context,
	) {
		actor, err :=
			h.actor(c)

		if err != nil {
			httpx.RespondError(c, err)
			return
		}

		id :=
			c.Param(
				"quarantineId",
			)

		branch, err :=
			h.service.QuarantineBranch(
				c,
				actor.BusinessID,
				id,
			)

		if err != nil {
			httpx.RespondError(
				c,
				operationalHTTPError(
					err,
				),
			)

			return
		}

		actor,
			scope,
			ok :=
			h.authorizedBranchPermission(
				c,
				branch,
				PermissionInventoryQuarantine,
			)

		if !ok {
			return
		}

		var body struct {
			ReasonCode string `json:"reason_code"`
		}

		if dispose {
			if err :=
				c.ShouldBindJSON(
					&body,
				); err != nil {

				httpx.RespondError(
					c,
					apperrors.BadRequest(
						"Disposal reason is required.",
						err,
					),
				)

				return
			}
		}

		out, err :=
			h.service.ResolveQuarantine(
				c,
				actor,
				scope,
				id,
				dispose,
				body.ReasonCode,
			)

		if err != nil {
			httpx.RespondError(
				c,
				operationalHTTPError(
					err,
				),
			)

			return
		}

		inventoryRespond(
			c,
			http.StatusOK,
			out,
		)
	}
}

func (h *Handler) setReorderRule(
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
		httpx.RespondError(
			c,
			operationalHTTPError(
				err,
			),
		)

		return
	}

	actor,
		scope,
		ok :=
		h.authorizedBranchPermission(
			c,
			branchID,
			PermissionInventoryPolicy,
		)

	if !ok {
		return
	}

	var input SetReorderRuleInput

	if err := c.ShouldBindJSON(
		&input,
	); err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"Invalid reorder rule.",
				err,
			),
		)

		return
	}

	input.BranchID =
		branchID

	input.InventoryItemID =
		itemID

	out, err :=
		h.service.SetReorderRule(
			c,
			actor,
			scope,
			input,
		)

	if err != nil {
		httpx.RespondError(
			c,
			operationalHTTPError(
				err,
			),
		)

		return
	}

	inventoryRespond(
		c,
		http.StatusOK,
		out,
	)
}

func (h *Handler) listAlerts(
	c *gin.Context,
) {
	branchID :=
		c.Param(
			"branchId",
		)

	actor,
		scope,
		ok :=
		h.authorizedBranchPermission(
			c,
			branchID,
			PermissionInventoryRead,
		)

	if !ok {
		return
	}

	limit :=
		100

	if raw :=
		c.Query(
			"limit",
		); raw != "" {

		value, err :=
			strconv.Atoi(
				raw,
			)

		if err != nil ||
			value <= 0 {

			httpx.RespondError(
				c,
				apperrors.BadRequest(
					"limit must be positive.",
					err,
				),
			)

			return
		}

		limit = value
	}

	items, err :=
		h.service.ListAlerts(
			c,
			actor,
			scope,
			branchID,
			c.Query(
				"status",
			),
			limit,
		)

	if err != nil {
		httpx.RespondError(
			c,
			operationalHTTPError(
				err,
			),
		)

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

func (h *Handler) alertTransition(
	status string,
) gin.HandlerFunc {
	return func(
		c *gin.Context,
	) {
		actor, err :=
			h.actor(c)

		if err != nil {
			httpx.RespondError(c, err)
			return
		}

		id :=
			c.Param(
				"alertId",
			)

		branch, err :=
			h.service.AlertBranch(
				c,
				actor.BusinessID,
				id,
			)

		if err != nil {
			httpx.RespondError(
				c,
				operationalHTTPError(
					err,
				),
			)

			return
		}

		actor,
			scope,
			ok :=
			h.authorizedBranchPermission(
				c,
				branch,
				PermissionInventoryManage,
			)

		if !ok {
			return
		}

		out, err :=
			h.service.SetAlertStatus(
				c,
				actor,
				scope,
				id,
				status,
			)

		if err != nil {
			httpx.RespondError(
				c,
				operationalHTTPError(
					err,
				),
			)

			return
		}

		inventoryRespond(
			c,
			http.StatusOK,
			out,
		)
	}
}

func RegisterOperationalRoutes(
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

	inventory.POST(
		"/stocktakes",
		h.createStocktake,
	)

	inventory.PUT(
		"/stocktakes/:stocktakeId/items/:inventoryItemId/count",
		h.countStocktake,
	)

	inventory.POST(
		"/stocktakes/:stocktakeId/post",
		h.stocktakeTransition(
			false,
		),
	)

	inventory.POST(
		"/stocktakes/:stocktakeId/cancel",
		h.stocktakeTransition(
			true,
		),
	)

	inventory.POST(
		"/transfers",
		h.createTransfer,
	)

	inventory.POST(
		"/transfers/:transferId/approve",
		h.transferTransition(
			"APPROVE",
		),
	)

	inventory.POST(
		"/transfers/:transferId/dispatch",
		h.transferTransition(
			"DISPATCH",
		),
	)

	inventory.POST(
		"/transfers/:transferId/receive",
		h.receiveTransfer,
	)

	inventory.POST(
		"/transfers/:transferId/cancel",
		h.transferTransition(
			"CANCEL",
		),
	)

	inventory.POST(
		"/quarantines",
		h.createQuarantine,
	)

	inventory.POST(
		"/quarantines/:quarantineId/release",
		h.quarantineTransition(
			false,
		),
	)

	inventory.POST(
		"/quarantines/:quarantineId/dispose",
		h.quarantineTransition(
			true,
		),
	)

	inventory.PUT(
		"/items/:inventoryItemId/reorder-rule",
		h.setReorderRule,
	)

	inventory.GET(
		"/branches/:branchId/alerts",
		h.listAlerts,
	)

	inventory.POST(
		"/alerts/:alertId/acknowledge",
		h.alertTransition(
			AlertAcknowledged,
		),
	)

	inventory.POST(
		"/alerts/:alertId/resolve",
		h.alertTransition(
			AlertResolved,
		),
	)
}
