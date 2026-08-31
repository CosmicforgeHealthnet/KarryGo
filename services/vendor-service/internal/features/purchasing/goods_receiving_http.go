package purchasing

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/features/inventory"
	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
)

type GoodsReceivingHandler struct {
	service *GoodsReceivingService
	scopes  inventory.ScopeResolver
	idem    *idempotency.Store
}

func NewGoodsReceivingHandler(
	service *GoodsReceivingService,
	scopes inventory.ScopeResolver,
	idem *idempotency.Store,
) *GoodsReceivingHandler {
	return &GoodsReceivingHandler{
		service: service,
		scopes:  scopes,
		idem:    idem,
	}
}

func receivingHTTPError(err error) error {
	switch {
	case errors.Is(err, ErrGoodsReceiptNotFound),
		errors.Is(err, ErrSupplierDiscrepancyNotFound):
		return apperrors.NotFound(
			"Purchasing receiving resource was not found.",
			err,
		)

	case errors.Is(err, ErrGoodsReceiptVersion):
		return apperrors.Conflict(
			"The resource changed before this operation completed.",
			err,
		)

	case errors.Is(err, ErrGoodsReceiptState),
		errors.Is(err, ErrSupplierDiscrepancyState),
		errors.Is(err, ErrPurchasingOverDelivery),
		errors.Is(err, ErrPurchasingReversalUnsafe):
		return apperrors.Conflict(
			"The receiving operation cannot be completed in the current state.",
			err,
		)

	case errors.Is(err, ErrPurchasingIdempotency):
		return apperrors.Conflict(
			"This idempotency key was already used for a different receiving command.",
			err,
		)

	case errors.Is(err, ErrPurchasingExactConversion):
		return apperrors.BadRequest(
			"Purchase quantity cannot be converted exactly into Inventory units.",
			err,
		)

	case errors.Is(err, ErrGoodsReceiptInvalid):
		return apperrors.BadRequest(
			"The goods receiving request is invalid.",
			err,
		)

	default:
		return err
	}
}

func receivingJSON(
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

func decodeReceivingJSON(
	c *gin.Context,
	dst any,
) error {
	body, err := io.ReadAll(
		io.LimitReader(
			c.Request.Body,
			1<<20,
		),
	)
	if err != nil {
		return err
	}

	decoder := json.NewDecoder(
		strings.NewReader(string(body)),
	)
	decoder.DisallowUnknownFields()

	if err = decoder.Decode(dst); err != nil {
		return err
	}

	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("request body contains more than one JSON value")
	}

	return nil
}

func receivingExpectedVersion(
	c *gin.Context,
) (int64, error) {
	raw := strings.TrimSpace(
		c.GetHeader("If-Match"),
	)

	raw = strings.TrimPrefix(raw, "W/")
	raw = strings.Trim(raw, `"`)

	version, err := strconv.ParseInt(
		raw,
		10,
		64,
	)
	if err != nil || version <= 0 {
		return 0, errors.New(
			"If-Match must contain the current positive resource version",
		)
	}

	return version, nil
}

func (h *GoodsReceivingHandler) actor(
	c *gin.Context,
) (GoodsReceivingActor, error) {
	staff, ok := authctx.FromContext(c)

	if !ok {
		return GoodsReceivingActor{},
			apperrors.Unauthorized(
				"Authentication is required.",
				nil,
			)
	}

	return GoodsReceivingActor{
		StaffID:       staff.StaffID,
		BusinessID:    staff.VendorID,
		SessionID:     staff.SessionID,
		CorrelationID: httpx.GetRequestID(c),
		CausationID:   c.GetHeader("X-Causation-ID"),
		CommandRef:    c.GetHeader("Idempotency-Key"),
		RequestID:     httpx.GetRequestID(c),
	}, nil
}

func (h *GoodsReceivingHandler) authorize(
	c *gin.Context,
	actor GoodsReceivingActor,
	branchID string,
	permission string,
) bool {
	if h.scopes == nil {
		httpx.RespondError(
			c,
			apperrors.Forbidden(
				"Purchasing access is not permitted.",
				nil,
			),
		)
		return false
	}

	scope, err := h.scopes.ResolveScope(
		c,
		actor.StaffID,
		permission,
	)
	if err != nil {
		httpx.RespondError(c, err)
		return false
	}

	if !scope.Covers(
		strings.TrimSpace(branchID),
	) {
		httpx.RespondError(
			c,
			apperrors.Forbidden(
				"Purchasing access is not permitted for this branch.",
				nil,
			),
		)
		return false
	}

	return true
}

func (h *GoodsReceivingHandler) createReceipt(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	var input CreateGoodsReceiptInput

	if err = decodeReceivingJSON(
		c,
		&input,
	); err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"Request body is invalid.",
				err,
			),
		)
		return
	}

	branchID, err := h.service.PurchaseOrderBranch(
		c,
		actor.BusinessID,
		input.PurchaseOrderID,
	)
	if err != nil {
		httpx.RespondError(
			c,
			receivingHTTPError(err),
		)
		return
	}

	if !h.authorize(
		c,
		actor,
		branchID,
		PermPurchasingReceive,
	) {
		return
	}

	out, err := h.service.CreateGoodsReceipt(
		c,
		actor,
		input,
	)
	if err != nil {
		httpx.RespondError(
			c,
			receivingHTTPError(err),
		)
		return
	}

	receivingJSON(
		c,
		http.StatusCreated,
		out,
	)
}

func (h *GoodsReceivingHandler) inspectReceipt(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	branchID, err := h.service.GoodsReceiptBranch(
		c,
		actor.BusinessID,
		c.Param("receiptId"),
	)
	if err != nil {
		httpx.RespondError(
			c,
			receivingHTTPError(err),
		)
		return
	}

	if !h.authorize(
		c,
		actor,
		branchID,
		PermPurchasingReceive,
	) {
		return
	}

	version, err := receivingExpectedVersion(c)
	if err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				err.Error(),
				err,
			),
		)
		return
	}

	var input InspectGoodsReceiptInput

	if err = decodeReceivingJSON(
		c,
		&input,
	); err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"Request body is invalid.",
				err,
			),
		)
		return
	}

	out, err := h.service.InspectGoodsReceipt(
		c,
		actor,
		c.Param("receiptId"),
		version,
		input,
	)
	if err != nil {
		httpx.RespondError(
			c,
			receivingHTTPError(err),
		)
		return
	}

	receivingJSON(
		c,
		http.StatusOK,
		out,
	)
}

func (h *GoodsReceivingHandler) confirmReceipt(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	branchID, err := h.service.GoodsReceiptBranch(
		c,
		actor.BusinessID,
		c.Param("receiptId"),
	)
	if err != nil {
		httpx.RespondError(
			c,
			receivingHTTPError(err),
		)
		return
	}

	if !h.authorize(
		c,
		actor,
		branchID,
		PermPurchasingReceive,
	) {
		return
	}

	version, err := receivingExpectedVersion(c)
	if err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				err.Error(),
				err,
			),
		)
		return
	}

	out, err := h.service.ConfirmGoodsReceipt(
		c,
		actor,
		c.Param("receiptId"),
		version,
	)
	if err != nil {
		httpx.RespondError(
			c,
			receivingHTTPError(err),
		)
		return
	}

	receivingJSON(
		c,
		http.StatusOK,
		out,
	)
}

func (h *GoodsReceivingHandler) reverseReceipt(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	branchID, err := h.service.GoodsReceiptBranch(
		c,
		actor.BusinessID,
		c.Param("receiptId"),
	)
	if err != nil {
		httpx.RespondError(
			c,
			receivingHTTPError(err),
		)
		return
	}

	if !h.authorize(
		c,
		actor,
		branchID,
		PermPurchasingReceive,
	) {
		return
	}

	version, err := receivingExpectedVersion(c)
	if err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				err.Error(),
				err,
			),
		)
		return
	}

	var input ReverseGoodsReceiptInput

	if err = decodeReceivingJSON(
		c,
		&input,
	); err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"Request body is invalid.",
				err,
			),
		)
		return
	}

	out, err := h.service.ReverseGoodsReceipt(
		c,
		actor,
		c.Param("receiptId"),
		version,
		input.Reason,
	)
	if err != nil {
		httpx.RespondError(
			c,
			receivingHTTPError(err),
		)
		return
	}

	receivingJSON(
		c,
		http.StatusOK,
		out,
	)
}

func (h *GoodsReceivingHandler) getReceipt(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	branchID, err := h.service.GoodsReceiptBranch(
		c,
		actor.BusinessID,
		c.Param("receiptId"),
	)
	if err != nil {
		httpx.RespondError(
			c,
			receivingHTTPError(err),
		)
		return
	}

	if !h.authorize(
		c,
		actor,
		branchID,
		PermPurchasingBaseRead,
	) {
		return
	}

	out, err := h.service.GetGoodsReceipt(
		c,
		actor,
		c.Param("receiptId"),
	)
	if err != nil {
		httpx.RespondError(
			c,
			receivingHTTPError(err),
		)
		return
	}

	receivingJSON(
		c,
		http.StatusOK,
		out,
	)
}

func (h *GoodsReceivingHandler) listReceipts(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	branchID := strings.TrimSpace(
		c.Query("branch_id"),
	)
	if branchID == "" {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"branch_id is required.",
				nil,
			),
		)
		return
	}

	if !h.authorize(
		c,
		actor,
		branchID,
		PermPurchasingBaseRead,
	) {
		return
	}

	limit, _ := strconv.Atoi(
		c.DefaultQuery("limit", "50"),
	)

	out, err := h.service.ListGoodsReceipts(
		c,
		actor,
		branchID,
		c.Query("status"),
		limit,
	)
	if err != nil {
		httpx.RespondError(
			c,
			receivingHTTPError(err),
		)
		return
	}

	receivingJSON(
		c,
		http.StatusOK,
		out,
	)
}

func (h *GoodsReceivingHandler) attachDocument(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	branchID, err := h.service.GoodsReceiptBranch(
		c,
		actor.BusinessID,
		c.Param("receiptId"),
	)
	if err != nil {
		httpx.RespondError(
			c,
			receivingHTTPError(err),
		)
		return
	}

	if !h.authorize(
		c,
		actor,
		branchID,
		PermPurchasingReceive,
	) {
		return
	}

	var input AttachGoodsReceiptDocumentInput

	if err = decodeReceivingJSON(
		c,
		&input,
	); err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"Request body is invalid.",
				err,
			),
		)
		return
	}

	out, err :=
		h.service.AttachGoodsReceiptDocument(
			c,
			actor,
			c.Param("receiptId"),
			input,
		)
	if err != nil {
		httpx.RespondError(
			c,
			receivingHTTPError(err),
		)
		return
	}

	receivingJSON(
		c,
		http.StatusCreated,
		out,
	)
}

func (h *GoodsReceivingHandler) listDocuments(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	branchID, err := h.service.GoodsReceiptBranch(
		c,
		actor.BusinessID,
		c.Param("receiptId"),
	)
	if err != nil {
		httpx.RespondError(
			c,
			receivingHTTPError(err),
		)
		return
	}

	if !h.authorize(
		c,
		actor,
		branchID,
		PermPurchasingBaseRead,
	) {
		return
	}

	out, err :=
		h.service.ListGoodsReceiptDocuments(
			c,
			actor,
			c.Param("receiptId"),
		)
	if err != nil {
		httpx.RespondError(
			c,
			receivingHTTPError(err),
		)
		return
	}

	receivingJSON(
		c,
		http.StatusOK,
		out,
	)
}

func (h *GoodsReceivingHandler) listDiscrepancies(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	branchID := strings.TrimSpace(
		c.Query("branch_id"),
	)
	if branchID == "" {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"branch_id is required.",
				nil,
			),
		)
		return
	}

	if !h.authorize(
		c,
		actor,
		branchID,
		PermPurchasingBaseRead,
	) {
		return
	}

	limit, _ := strconv.Atoi(
		c.DefaultQuery("limit", "50"),
	)

	out, err := h.service.ListDiscrepancies(
		c,
		actor,
		branchID,
		c.Query("status"),
		c.Query("type"),
		limit,
	)
	if err != nil {
		httpx.RespondError(
			c,
			receivingHTTPError(err),
		)
		return
	}

	receivingJSON(
		c,
		http.StatusOK,
		out,
	)
}

func (h *GoodsReceivingHandler) getDiscrepancy(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	branchID, err := h.service.DiscrepancyBranch(
		c,
		actor.BusinessID,
		c.Param("discrepancyId"),
	)
	if err != nil {
		httpx.RespondError(
			c,
			receivingHTTPError(err),
		)
		return
	}

	if !h.authorize(
		c,
		actor,
		branchID,
		PermPurchasingBaseRead,
	) {
		return
	}

	out, err := h.service.GetDiscrepancy(
		c,
		actor,
		c.Param("discrepancyId"),
	)
	if err != nil {
		httpx.RespondError(
			c,
			receivingHTTPError(err),
		)
		return
	}

	receivingJSON(
		c,
		http.StatusOK,
		out,
	)
}

func (h *GoodsReceivingHandler) discrepancyTransition(
	target string,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, err := h.actor(c)
		if err != nil {
			httpx.RespondError(c, err)
			return
		}

		branchID, err := h.service.DiscrepancyBranch(
			c,
			actor.BusinessID,
			c.Param("discrepancyId"),
		)
		if err != nil {
			httpx.RespondError(
				c,
				receivingHTTPError(err),
			)
			return
		}

		permission :=
			PermPurchasingDiscrepancyManage

		if target == DiscrepancyApproved {
			current, loadErr :=
				h.service.GetDiscrepancy(
					c,
					actor,
					c.Param("discrepancyId"),
				)
			if loadErr != nil {
				httpx.RespondError(
					c,
					receivingHTTPError(loadErr),
				)
				return
			}

			if current.Type ==
				DiscrepancyCostVariance {
				permission =
					PermPurchasingCostVarianceApprove
			}
		}

		if !h.authorize(
			c,
			actor,
			branchID,
			permission,
		) {
			return
		}

		version, err :=
			receivingExpectedVersion(c)
		if err != nil {
			httpx.RespondError(
				c,
				apperrors.BadRequest(
					err.Error(),
					err,
				),
			)
			return
		}

		var input DiscrepancyDecisionInput

		if c.Request.ContentLength != 0 {
			if err = decodeReceivingJSON(
				c,
				&input,
			); err != nil {
				httpx.RespondError(
					c,
					apperrors.BadRequest(
						"Request body is invalid.",
						err,
					),
				)
				return
			}
		}

		out, err :=
			h.service.TransitionDiscrepancy(
				c,
				actor,
				c.Param("discrepancyId"),
				version,
				target,
				input.Notes,
			)
		if err != nil {
			httpx.RespondError(
				c,
				receivingHTTPError(err),
			)
			return
		}

		receivingJSON(
			c,
			http.StatusOK,
			out,
		)
	}
}

func (h *GoodsReceivingHandler) listCostHistory(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	branchID := strings.TrimSpace(
		c.Query("branch_id"),
	)
	if branchID == "" {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"branch_id is required.",
				nil,
			),
		)
		return
	}

	if !h.authorize(
		c,
		actor,
		branchID,
		PermPurchasingCostRead,
	) {
		return
	}

	limit, _ := strconv.Atoi(
		c.DefaultQuery("limit", "50"),
	)

	out, err :=
		h.service.ListPurchaseCostHistory(
			c,
			actor,
			branchID,
			c.Query("inventory_item_id"),
			limit,
		)
	if err != nil {
		httpx.RespondError(
			c,
			receivingHTTPError(err),
		)
		return
	}

	receivingJSON(
		c,
		http.StatusOK,
		out,
	)
}

func (h *GoodsReceivingHandler) getPolicy(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	branchID := c.Param("branchId")

	if !h.authorize(
		c,
		actor,
		branchID,
		PermPurchasingBaseRead,
	) {
		return
	}

	out, err := h.service.GetReceivingPolicy(
		c,
		actor,
		branchID,
	)
	if err != nil {
		httpx.RespondError(
			c,
			receivingHTTPError(err),
		)
		return
	}

	receivingJSON(
		c,
		http.StatusOK,
		out,
	)
}

func (h *GoodsReceivingHandler) putPolicy(
	c *gin.Context,
) {
	actor, err := h.actor(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}

	branchID := c.Param("branchId")

	if !h.authorize(
		c,
		actor,
		branchID,
		PermPurchasingBaseManage,
	) {
		return
	}

	var input PutGoodsReceivingPolicyInput

	if err = decodeReceivingJSON(
		c,
		&input,
	); err != nil {
		httpx.RespondError(
			c,
			apperrors.BadRequest(
				"Request body is invalid.",
				err,
			),
		)
		return
	}

	out, err := h.service.PutReceivingPolicy(
		c,
		actor,
		branchID,
		input,
	)
	if err != nil {
		httpx.RespondError(
			c,
			receivingHTTPError(err),
		)
		return
	}

	receivingJSON(
		c,
		http.StatusOK,
		out,
	)
}

func RegisterGoodsReceivingRoutes(
	root *gin.RouterGroup,
	h *GoodsReceivingHandler,
	enabled bool,
) {
	if !enabled || h == nil {
		return
	}

	group := root.Group("/purchasing")
	group.Use(
		idempotency.CommandMiddleware(
			h.idem,
		),
	)

	receipts := group.Group("/goods-receipts")

	receipts.POST("", h.createReceipt)
	receipts.GET("", h.listReceipts)
	receipts.GET("/:receiptId", h.getReceipt)

	receipts.PUT(
		"/:receiptId/inspection",
		h.inspectReceipt,
	)
	receipts.POST(
		"/:receiptId/confirm",
		h.confirmReceipt,
	)
	receipts.POST(
		"/:receiptId/reverse",
		h.reverseReceipt,
	)

	receipts.POST(
		"/:receiptId/documents",
		h.attachDocument,
	)
	receipts.GET(
		"/:receiptId/documents",
		h.listDocuments,
	)

	discrepancies :=
		group.Group(
			"/supplier-discrepancies",
		)

	discrepancies.GET(
		"",
		h.listDiscrepancies,
	)
	discrepancies.GET(
		"/:discrepancyId",
		h.getDiscrepancy,
	)
	discrepancies.POST(
		"/:discrepancyId/approve",
		h.discrepancyTransition(
			DiscrepancyApproved,
		),
	)
	discrepancies.POST(
		"/:discrepancyId/reject",
		h.discrepancyTransition(
			DiscrepancyRejectedStatus,
		),
	)
	discrepancies.POST(
		"/:discrepancyId/resolve",
		h.discrepancyTransition(
			DiscrepancyResolved,
		),
	)

	group.GET(
		"/purchase-cost-history",
		h.listCostHistory,
	)

	group.GET(
		"/receiving-policies/branches/:branchId",
		h.getPolicy,
	)
	group.PUT(
		"/receiving-policies/branches/:branchId",
		h.putPolicy,
	)
}
