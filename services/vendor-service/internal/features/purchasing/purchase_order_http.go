package purchasing

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

type PurchaseOrderHandler struct {
	service *PurchaseOrderService

	base *Handler

	idempotency *idempotency.Store
}

func NewPurchaseOrderHandler(
	service *PurchaseOrderService,
	base *Handler,
	store *idempotency.Store,
) *PurchaseOrderHandler {
	return &PurchaseOrderHandler{
		service:     service,
		base:        base,
		idempotency: store,
	}
}

func parseIfMatchVersion(
	c *gin.Context,
	bodyVersion int64,
) (
	int64,
	error,
) {
	raw :=
		strings.TrimSpace(
			c.GetHeader(
				"If-Match",
			),
		)

	if raw == "" {
		if bodyVersion <= 0 {
			return 0,
				apperrors.Conflict(
					"Provide If-Match or expected_version.",
					nil,
				)
		}

		return bodyVersion, nil
	}

	if strings.HasPrefix(
		raw,
		"W/",
	) {
		raw =
			strings.TrimPrefix(
				raw,
				"W/",
			)
	}

	raw =
		strings.Trim(
			raw,
			`"`,
		)

	if raw == "" ||
		raw == "*" {

		return 0,
			apperrors.BadRequest(
				"If-Match must contain the PurchaseOrder version.",
				nil,
			)
	}

	headerVersion, err :=
		strconv.ParseInt(
			raw,
			10,
			64,
		)

	if err != nil ||
		headerVersion <= 0 {

		return 0,
			apperrors.BadRequest(
				"If-Match must contain a positive PurchaseOrder version.",
				err,
			)
	}

	if bodyVersion > 0 &&
		bodyVersion !=
			headerVersion {

		return 0,
			apperrors.Conflict(
				"If-Match and expected_version disagree.",
				nil,
			)
	}

	return headerVersion, nil
}

func purchaseOrderResponse(
	c *gin.Context,
	status int,
	order PurchaseOrder,
) {
	c.Header(
		"ETag",
		fmt.Sprintf(
			`"%d"`,
			order.Version,
		),
	)

	c.JSON(
		status,
		gin.H{
			"success": true,
			"data":    order,
		},
	)
}

func (h *PurchaseOrderHandler) actor(
	c *gin.Context,
) (
	Actor,
	bool,
) {
	if h == nil ||
		h.base == nil {

		httpx.RespondError(
			c,
			apperrors.Forbidden(
				"Purchasing access is not permitted.",
				nil,
			),
		)

		return Actor{}, false
	}

	actor, err :=
		h.base.actor(
			c,
		)

	if err != nil {
		httpx.RespondError(
			c,
			err,
		)

		return Actor{}, false
	}

	return actor, true
}

func (h *PurchaseOrderHandler) getApprovalPolicy(
	c *gin.Context,
) {
	actor, ok :=
		h.actor(
			c,
		)

	if !ok {
		return
	}

	out, err :=
		h.service.GetApprovalPolicy(
			c,
			actor,
		)

	if err != nil {
		httpx.RespondError(
			c,
			err,
		)

		return
	}

	purchasingOK(
		c,
		out,
	)
}

func (h *PurchaseOrderHandler) replaceApprovalPolicy(
	c *gin.Context,
) {
	actor, ok :=
		h.actor(
			c,
		)

	if !ok {
		return
	}

	var input ReplaceApprovalPolicyInput

	if err :=
		strictJSON(
			c,
			&input,
		); err != nil {

		httpx.RespondError(
			c,
			err,
		)

		return
	}

	out, err :=
		h.service.ReplaceApprovalPolicy(
			c,
			actor,
			input,
		)

	if err != nil {
		httpx.RespondError(
			c,
			err,
		)

		return
	}

	purchasingOK(
		c,
		out,
	)
}

func (h *PurchaseOrderHandler) listPurchaseOrders(
	c *gin.Context,
) {
	actor, ok :=
		h.actor(
			c,
		)

	if !ok {
		return
	}

	limit,
		offset,
		err :=
		page(
			c,
		)

	if err != nil {
		httpx.RespondError(
			c,
			err,
		)

		return
	}

	out, err :=
		h.service.ListPurchaseOrders(
			c,
			actor,
			c.Query(
				"branch_id",
			),
			c.Query(
				"status",
			),
			limit,
			offset,
		)

	if err != nil {
		httpx.RespondError(
			c,
			err,
		)

		return
	}

	purchasingOK(
		c,
		out,
	)
}

func (h *PurchaseOrderHandler) createPurchaseOrder(
	c *gin.Context,
) {
	actor, ok :=
		h.actor(
			c,
		)

	if !ok {
		return
	}

	var input CreatePurchaseOrderInput

	if err :=
		strictJSON(
			c,
			&input,
		); err != nil {

		httpx.RespondError(
			c,
			err,
		)

		return
	}

	out, err :=
		h.service.CreatePurchaseOrder(
			c,
			actor,
			input,
		)

	if err != nil {
		httpx.RespondError(
			c,
			err,
		)

		return
	}

	purchaseOrderResponse(
		c,
		http.StatusCreated,
		out,
	)
}

func (h *PurchaseOrderHandler) getPurchaseOrder(
	c *gin.Context,
) {
	actor, ok :=
		h.actor(
			c,
		)

	if !ok {
		return
	}

	out, err :=
		h.service.GetPurchaseOrder(
			c,
			actor,
			c.Param(
				"purchaseOrderId",
			),
		)

	if err != nil {
		httpx.RespondError(
			c,
			err,
		)

		return
	}

	purchaseOrderResponse(
		c,
		http.StatusOK,
		out,
	)
}

func (h *PurchaseOrderHandler) updatePurchaseOrder(
	c *gin.Context,
) {
	actor, ok :=
		h.actor(
			c,
		)

	if !ok {
		return
	}

	var input UpdatePurchaseOrderInput

	if err :=
		strictJSON(
			c,
			&input,
		); err != nil {

		httpx.RespondError(
			c,
			err,
		)

		return
	}

	version, err :=
		parseIfMatchVersion(
			c,
			input.ExpectedVersion,
		)

	if err != nil {
		httpx.RespondError(
			c,
			err,
		)

		return
	}

	input.ExpectedVersion =
		version

	out, err :=
		h.service.UpdatePurchaseOrder(
			c,
			actor,
			c.Param(
				"purchaseOrderId",
			),
			input,
		)

	if err != nil {
		httpx.RespondError(
			c,
			err,
		)

		return
	}

	purchaseOrderResponse(
		c,
		http.StatusOK,
		out,
	)
}

func (h *PurchaseOrderHandler) replacePurchaseOrderLines(
	c *gin.Context,
) {
	actor, ok :=
		h.actor(
			c,
		)

	if !ok {
		return
	}

	var input ReplacePurchaseOrderLinesInput

	if err :=
		strictJSON(
			c,
			&input,
		); err != nil {

		httpx.RespondError(
			c,
			err,
		)

		return
	}

	version, err :=
		parseIfMatchVersion(
			c,
			input.ExpectedVersion,
		)

	if err != nil {
		httpx.RespondError(
			c,
			err,
		)

		return
	}

	input.ExpectedVersion =
		version

	out, err :=
		h.service.ReplacePurchaseOrderLines(
			c,
			actor,
			c.Param(
				"purchaseOrderId",
			),
			input,
		)

	if err != nil {
		httpx.RespondError(
			c,
			err,
		)

		return
	}

	purchaseOrderResponse(
		c,
		http.StatusOK,
		out,
	)
}

func (h *PurchaseOrderHandler) transition(
	method func(
		context.Context,
		Actor,
		string,
		PurchaseOrderTransitionInput,
	) (
		PurchaseOrder,
		error,
	),
) gin.HandlerFunc {
	return func(
		c *gin.Context,
	) {
		actor, ok :=
			h.actor(
				c,
			)

		if !ok {
			return
		}

		var input PurchaseOrderTransitionInput

		if err :=
			strictJSON(
				c,
				&input,
			); err != nil {

			httpx.RespondError(
				c,
				err,
			)

			return
		}

		version, err :=
			parseIfMatchVersion(
				c,
				input.ExpectedVersion,
			)

		if err != nil {
			httpx.RespondError(
				c,
				err,
			)

			return
		}

		input.ExpectedVersion =
			version

		out, err :=
			method(
				c,
				actor,
				c.Param(
					"purchaseOrderId",
				),
				input,
			)

		if err != nil {
			httpx.RespondError(
				c,
				err,
			)

			return
		}

		purchaseOrderResponse(
			c,
			http.StatusOK,
			out,
		)
	}
}

func (h *PurchaseOrderHandler) approval(
	decision string,
) gin.HandlerFunc {
	return func(
		c *gin.Context,
	) {
		actor, ok :=
			h.actor(
				c,
			)

		if !ok {
			return
		}

		var input PurchaseOrderApprovalInput

		if err :=
			strictJSON(
				c,
				&input,
			); err != nil {

			httpx.RespondError(
				c,
				err,
			)

			return
		}

		version, err :=
			parseIfMatchVersion(
				c,
				input.ExpectedVersion,
			)

		if err != nil {
			httpx.RespondError(
				c,
				err,
			)

			return
		}

		input.ExpectedVersion =
			version

		var out PurchaseOrder

		if decision ==
			ApprovalDecisionApproved {

			out, err =
				h.service.ApprovePurchaseOrder(
					c,
					actor,
					c.Param(
						"purchaseOrderId",
					),
					input,
				)

		} else {
			out, err =
				h.service.RejectPurchaseOrder(
					c,
					actor,
					c.Param(
						"purchaseOrderId",
					),
					input,
				)
		}

		if err != nil {
			httpx.RespondError(
				c,
				err,
			)

			return
		}

		purchaseOrderResponse(
			c,
			http.StatusOK,
			out,
		)
	}
}

func (h *PurchaseOrderHandler) listApprovals(
	c *gin.Context,
) {
	actor, ok :=
		h.actor(
			c,
		)

	if !ok {
		return
	}

	out, err :=
		h.service.ListPurchaseOrderApprovals(
			c,
			actor,
			c.Param(
				"purchaseOrderId",
			),
		)

	if err != nil {
		httpx.RespondError(
			c,
			err,
		)

		return
	}

	purchasingOK(
		c,
		out,
	)
}

func RegisterPurchaseOrderRoutes(
	root *gin.RouterGroup,
	handler *PurchaseOrderHandler,
	enabled bool,
) {
	if root == nil ||
		handler == nil ||
		!enabled {

		return
	}

	group :=
		root.Group(
			"/purchasing",
		)

	group.Use(
		idempotency.CommandMiddleware(
			handler.idempotency,
		),
	)

	group.GET(
		"/approval-policy",
		handler.getApprovalPolicy,
	)

	group.PUT(
		"/approval-policy",
		handler.replaceApprovalPolicy,
	)

	orders :=
		group.Group(
			"/purchase-orders",
		)

	orders.GET(
		"",
		handler.listPurchaseOrders,
	)

	orders.POST(
		"",
		handler.createPurchaseOrder,
	)

	orders.GET(
		"/:purchaseOrderId",
		handler.getPurchaseOrder,
	)

	orders.PATCH(
		"/:purchaseOrderId",
		handler.updatePurchaseOrder,
	)

	orders.PUT(
		"/:purchaseOrderId/lines",
		handler.replacePurchaseOrderLines,
	)

	orders.GET(
		"/:purchaseOrderId/approvals",
		handler.listApprovals,
	)

	orders.POST(
		"/:purchaseOrderId/submit",
		handler.transition(
			handler.service.SubmitPurchaseOrder,
		),
	)

	orders.POST(
		"/:purchaseOrderId/approve",
		handler.approval(
			ApprovalDecisionApproved,
		),
	)

	orders.POST(
		"/:purchaseOrderId/reject",
		handler.approval(
			ApprovalDecisionRejected,
		),
	)

	orders.POST(
		"/:purchaseOrderId/order",
		handler.transition(
			handler.service.MarkPurchaseOrderOrdered,
		),
	)

	orders.POST(
		"/:purchaseOrderId/cancel",
		handler.transition(
			handler.service.CancelPurchaseOrder,
		),
	)

	orders.POST(
		"/:purchaseOrderId/close",
		handler.transition(
			handler.service.ClosePurchaseOrder,
		),
	)
}
