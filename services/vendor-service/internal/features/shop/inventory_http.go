package shop

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/httpx"
)

type stockBasisCommand struct{ ConfigureStockBasisInput }
type receiptCommand struct{ CreateReceiptInput }
type reservationCommand struct{ ReserveInventoryInput }
type adjustmentCommand struct{ AdjustInventoryInput }
type quarantineCommand struct {
	StockBasisID string `json:"stock_basis_id"`
	Quantity     int64  `json:"quantity"`
	ReasonCode   string `json:"reason_code"`
	Note         string `json:"note"`
}
type quarantineResolutionCommand struct {
	Action string `json:"action"`
}
type transferCommand struct{ CreateTransferInput }
type transferReceiptCommand struct{ Lines []ReceiveTransferLineInput }
type stocktakeCommand struct {
	StocktakeType string   `json:"stocktake_type"`
	StockBasisIDs []string `json:"stock_basis_ids"`
}
type stocktakeCountCommand struct {
	CountedQuantity int64 `json:"counted_quantity"`
}
type inventoryPolicyCommand struct{ SetInventoryPolicyInput }
type fulfilmentCommand struct{ RecordFulfilmentInput }
type reorderRuleCommand struct {
	MinimumQuantity   int64 `json:"minimum_quantity"`
	TargetQuantity    int64 `json:"target_quantity"`
	ExpiringSoonHours *int  `json:"expiring_soon_hours"`
}
type bulkOperationCommand struct{ BulkOperationInput }

func (h *Handler) configureStockBasis(c *gin.Context) {
	h.command(c, PermInventoryPolicy, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in stockBasisCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return h.service.ConfigureStockBasis(c, a, s, c.Param("branchId"), in.ConfigureStockBasisInput)
	})
}
func (h *Handler) inventoryBalances(c *gin.Context) {
	a, s, ok := h.authorized(c, PermInventoryRead)
	if !ok {
		return
	}
	items, err := h.service.ListInventoryBalances(c, a, s, c.Param("branchId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, items)
}
func (h *Handler) inventoryBalance(c *gin.Context) {
	a, s, ok := h.authorized(c, PermInventoryRead)
	if !ok {
		return
	}
	item, err := h.service.InventoryBalance(c, a, s, c.Param("stockBasisId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	if item.BranchRef != c.Param("branchId") {
		httpx.RespondError(c, errNotFound("Inventory balance", CodeStockBasisNotFound))
		return
	}
	respond(c, http.StatusOK, item)
}
func (h *Handler) inventoryRevision(c *gin.Context) {
	a, s, ok := h.authorized(c, PermInventoryRead)
	if !ok {
		return
	}
	revision, updated, err := h.service.InventoryRevision(c, a, s, c.Param("branchId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, gin.H{"revision": revision, "updated_at": updated, "domain": "SHOP_INVENTORY"})
}
func (h *Handler) inventoryMovements(c *gin.Context) {
	a, s, ok := h.authorized(c, PermInventoryRead)
	if !ok {
		return
	}
	limit, offset, err := page(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	parseTime := func(name string) (*time.Time, error) {
		raw := strings.TrimSpace(c.Query(name))
		if raw == "" {
			return nil, nil
		}
		value, parseErr := time.Parse(time.RFC3339, raw)
		if parseErr != nil {
			return nil, fieldError(name, "Time must be RFC3339.")
		}
		return &value, nil
	}
	from, err := parseTime("from")
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	to, err := parseTime("to")
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items, more, err := h.service.ListInventoryMovements(c, a, s, c.Param("branchId"), MovementListOptions{VariantID: c.Query("variant_id"), MovementType: c.Query("movement_type"), SourceRef: c.Query("source_ref"), LotID: c.Query("lot_id"), From: from, To: to, Limit: limit, Offset: offset})
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respondPage(c, items, more)
}
func (h *Handler) createReceipt(c *gin.Context) {
	h.command(c, PermInventoryReceive, http.StatusCreated, func(a Actor, s Scope, raw []byte) (any, error) {
		var in receiptCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return h.service.CreateReceipt(c, a, s, in.CreateReceiptInput)
	})
}
func (h *Handler) getReceipt(c *gin.Context) {
	a, s, ok := h.authorized(c, PermInventoryRead)
	if !ok {
		return
	}
	item, err := h.service.GetReceipt(c, a, s, c.Param("receiptId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, item)
}
func (h *Handler) listReceipts(c *gin.Context) {
	a, s, ok := h.authorized(c, PermInventoryRead)
	if !ok {
		return
	}
	limit, offset, err := page(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items, more, err := h.service.ListReceipts(c, a, s, c.Param("branchId"), c.Query("status"), limit, offset)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respondPage(c, items, more)
}
func (h *Handler) postReceipt(c *gin.Context) {
	h.command(c, PermInventoryReceive, http.StatusOK, func(a Actor, s Scope, _ []byte) (any, error) {
		return h.service.PostReceipt(c, a, s, c.Param("receiptId"))
	})
}
func (h *Handler) cancelReceipt(c *gin.Context) {
	h.command(c, PermInventoryReceive, http.StatusOK, func(a Actor, s Scope, _ []byte) (any, error) {
		return h.service.CancelReceipt(c, a, s, c.Param("receiptId"))
	})
}
func (h *Handler) reserveInventory(c *gin.Context) {
	h.command(c, PermInventoryReserve, http.StatusCreated, func(a Actor, s Scope, raw []byte) (any, error) {
		var in reservationCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return h.service.ReserveInventory(c, a, s, in.ReserveInventoryInput)
	})
}
func (h *Handler) getReservation(c *gin.Context) {
	a, s, ok := h.authorized(c, PermInventoryRead)
	if !ok {
		return
	}
	item, err := h.service.GetInventoryReservation(c, a, s, c.Param("reservationId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, item)
}
func (h *Handler) reservationTransition(target string) gin.HandlerFunc {
	return func(c *gin.Context) {
		h.command(c, PermInventoryReserve, http.StatusOK, func(a Actor, s Scope, _ []byte) (any, error) {
			switch target {
			case ReservationConfirmed:
				return h.service.ConfirmReservation(c, a, s, c.Param("reservationId"))
			case ReservationCommitted:
				return h.service.CommitReservation(c, a, s, c.Param("reservationId"))
			default:
				return h.service.ReleaseReservation(c, a, s, c.Param("reservationId"))
			}
		})
	}
}
func (h *Handler) adjustInventory(c *gin.Context) {
	h.command(c, PermInventoryAdjust, http.StatusCreated, func(a Actor, s Scope, raw []byte) (any, error) {
		var in adjustmentCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return h.service.AdjustInventory(c, a, s, in.AdjustInventoryInput)
	})
}
func (h *Handler) quarantineInventory(c *gin.Context) {
	h.command(c, PermInventoryQuarantine, http.StatusCreated, func(a Actor, s Scope, raw []byte) (any, error) {
		var in quarantineCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return h.service.QuarantineInventory(c, a, s, in.StockBasisID, in.Quantity, in.ReasonCode, in.Note)
	})
}
func (h *Handler) resolveQuarantine(c *gin.Context) {
	h.command(c, PermInventoryQuarantine, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in quarantineResolutionCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return h.service.ResolveQuarantine(c, a, s, c.Param("quarantineId"), in.Action)
	})
}
func (h *Handler) createTransfer(c *gin.Context) {
	h.command(c, PermInventoryTransfer, http.StatusCreated, func(a Actor, s Scope, raw []byte) (any, error) {
		var in transferCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return h.service.CreateTransfer(c, a, s, in.CreateTransferInput)
	})
}
func (h *Handler) dispatchTransfer(c *gin.Context) {
	h.command(c, PermInventoryTransfer, http.StatusOK, func(a Actor, s Scope, _ []byte) (any, error) {
		return h.service.DispatchTransfer(c, a, s, c.Param("transferId"))
	})
}
func (h *Handler) getTransfer(c *gin.Context) {
	a, s, ok := h.authorized(c, PermInventoryRead)
	if !ok {
		return
	}
	item, err := h.service.GetTransfer(c, a, s, c.Param("transferId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, item)
}
func (h *Handler) listTransfers(c *gin.Context) {
	a, s, ok := h.authorized(c, PermInventoryRead)
	if !ok {
		return
	}
	limit, offset, err := page(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items, more, err := h.service.ListTransfers(c, a, s, c.Param("branchId"), c.Query("status"), limit, offset)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respondPage(c, items, more)
}
func (h *Handler) cancelTransfer(c *gin.Context) {
	h.command(c, PermInventoryTransfer, http.StatusOK, func(a Actor, s Scope, _ []byte) (any, error) {
		return h.service.CancelTransfer(c, a, s, c.Param("transferId"))
	})
}
func (h *Handler) receiveTransfer(c *gin.Context) {
	h.command(c, PermInventoryTransfer, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in transferReceiptCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return h.service.ReceiveTransfer(c, a, s, c.Param("transferId"), in.Lines)
	})
}
func (h *Handler) createStocktake(c *gin.Context) {
	h.command(c, PermInventoryStocktake, http.StatusCreated, func(a Actor, s Scope, raw []byte) (any, error) {
		var in stocktakeCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return h.service.CreateStocktake(c, a, s, c.Param("branchId"), in.StocktakeType, in.StockBasisIDs)
	})
}
func (h *Handler) countStocktake(c *gin.Context) {
	h.command(c, PermInventoryStocktake, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in stocktakeCountCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		if err := h.service.CountStocktakeLine(c, a, s, c.Param("stocktakeId"), c.Param("stockBasisId"), in.CountedQuantity); err != nil {
			return nil, err
		}
		return gin.H{"counted": true}, nil
	})
}
func (h *Handler) postStocktake(c *gin.Context) {
	h.command(c, PermInventoryStocktake, http.StatusOK, func(a Actor, s Scope, _ []byte) (any, error) {
		return h.service.PostStocktake(c, a, s, c.Param("stocktakeId"))
	})
}
func (h *Handler) getStocktake(c *gin.Context) {
	a, s, ok := h.authorized(c, PermInventoryRead)
	if !ok {
		return
	}
	item, err := h.service.GetStocktake(c, a, s, c.Param("stocktakeId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, item)
}
func (h *Handler) listStocktakes(c *gin.Context) {
	a, s, ok := h.authorized(c, PermInventoryRead)
	if !ok {
		return
	}
	limit, offset, err := page(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items, more, err := h.service.ListStocktakes(c, a, s, c.Param("branchId"), c.Query("status"), limit, offset)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respondPage(c, items, more)
}
func (h *Handler) listQuarantines(c *gin.Context) {
	a, s, ok := h.authorized(c, PermInventoryRead)
	if !ok {
		return
	}
	limit, offset, err := page(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items, more, err := h.service.ListQuarantines(c, a, s, c.Param("branchId"), c.Query("status"), limit, offset)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respondPage(c, items, more)
}
func (h *Handler) listReorderRules(c *gin.Context) {
	a, s, ok := h.authorized(c, PermInventoryRead)
	if !ok {
		return
	}
	limit, offset, err := page(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items, more, err := h.service.ListReorderRules(c, a, s, c.Param("branchId"), c.Query("status"), limit, offset)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respondPage(c, items, more)
}
func (h *Handler) listInventoryAlerts(c *gin.Context) {
	a, s, ok := h.authorized(c, PermInventoryRead)
	if !ok {
		return
	}
	limit, offset, err := page(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	items, more, err := h.service.ListInventoryAlerts(c, a, s, c.Param("branchId"), c.Query("status"), c.Query("alert_type"), limit, offset)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respondPage(c, items, more)
}
func (h *Handler) transitionInventoryAlert(target string) gin.HandlerFunc {
	return func(c *gin.Context) {
		h.command(c, PermInventoryPolicy, http.StatusOK, func(a Actor, s Scope, _ []byte) (any, error) {
			return h.service.TransitionInventoryAlert(c, a, s, c.Param("alertId"), target)
		})
	}
}
func (h *Handler) setInventoryPolicy(c *gin.Context) {
	h.command(c, PermInventoryPolicy, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in inventoryPolicyCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return h.service.SetInventoryPolicy(c, a, s, c.Param("listingId"), in.SetInventoryPolicyInput)
	})
}
func (h *Handler) recordFulfilment(c *gin.Context) {
	h.command(c, PermInventoryReserve, http.StatusCreated, func(a Actor, s Scope, raw []byte) (any, error) {
		var in fulfilmentCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return h.service.RecordFulfilmentMeasurement(c, a, s, in.RecordFulfilmentInput)
	})
}
func (h *Handler) setReorderRule(c *gin.Context) {
	h.command(c, PermInventoryPolicy, http.StatusOK, func(a Actor, s Scope, raw []byte) (any, error) {
		var in reorderRuleCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return h.service.SetReorderRule(c, a, s, c.Param("stockBasisId"), in.MinimumQuantity, in.TargetQuantity, in.ExpiringSoonHours)
	})
}
func (h *Handler) availabilityPreview(c *gin.Context) {
	a, s, ok := h.authorized(c, PermInventoryRead)
	if !ok {
		return
	}
	item, err := h.service.EffectiveAvailability(c, a, s, c.Param("branchId"), c.Param("variantId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, item)
}
func (h *Handler) sellabilityPreview(c *gin.Context) {
	a, s, ok := h.authorized(c, PermInventoryRead)
	if !ok {
		return
	}
	quantity, err := strconv.ParseInt(c.DefaultQuery("quantity", "1"), 10, 64)
	if err != nil {
		httpx.RespondError(c, fieldError("quantity", "Quantity must be an integer."))
		return
	}
	item, err := h.service.ResolveSellability(c, a, s, c.Param("branchId"), c.Param("variantId"), c.Query("sell_unit_id"), quantity, strings.EqualFold(c.Query("channel"), "wholesale"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, item)
}
func (h *Handler) submitBulk(c *gin.Context) {
	h.command(c, PermBulkImportManage, http.StatusCreated, func(a Actor, s Scope, raw []byte) (any, error) {
		var in bulkOperationCommand
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return h.service.SubmitBulkOperation(c, a, s, in.BulkOperationInput)
	})
}
func (h *Handler) applyBulk(c *gin.Context) {
	h.command(c, PermBulkImportManage, http.StatusOK, func(a Actor, s Scope, _ []byte) (any, error) {
		return h.service.ApplyBulkOperation(c, a, s, c.Param("jobId"))
	})
}
func (h *Handler) getBulk(c *gin.Context) {
	a, s, ok := h.authorized(c, PermBulkImportManage)
	if !ok {
		return
	}
	item, err := h.service.GetBulkOperation(c, a, s, c.Param("jobId"))
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	respond(c, http.StatusOK, item)
}

func RegisterInventoryRoutes(root *gin.RouterGroup, h *Handler) {
	branches := root.Group("/branches/:branchId/inventory")
	branches.PUT("/stock-bases", h.configureStockBasis)
	branches.GET("/balances", h.inventoryBalances)
	branches.GET("/balances/:stockBasisId", h.inventoryBalance)
	branches.GET("/movements", h.inventoryMovements)
	branches.GET("/revision", h.inventoryRevision)
	branches.GET("/variants/:variantId/availability", h.availabilityPreview)
	branches.GET("/variants/:variantId/sellability", h.sellabilityPreview)
	branches.GET("/receipts", h.listReceipts)
	branches.GET("/transfers", h.listTransfers)
	branches.GET("/stocktakes", h.listStocktakes)
	branches.GET("/quarantines", h.listQuarantines)
	branches.GET("/reorder-rules", h.listReorderRules)
	branches.GET("/alerts", h.listInventoryAlerts)
	branches.POST("/stocktakes", h.createStocktake)
	receipts := root.Group("/inventory/receipts")
	receipts.POST("", h.createReceipt)
	receipts.GET("/:receiptId", h.getReceipt)
	receipts.POST("/:receiptId/post", h.postReceipt)
	receipts.POST("/:receiptId/cancel", h.cancelReceipt)
	reservations := root.Group("/inventory/reservations")
	reservations.POST("", h.reserveInventory)
	reservations.GET("/:reservationId", h.getReservation)
	reservations.POST("/:reservationId/confirm", h.reservationTransition(ReservationConfirmed))
	reservations.POST("/:reservationId/commit", h.reservationTransition(ReservationCommitted))
	reservations.POST("/:reservationId/release", h.reservationTransition(ReservationReleased))
	root.POST("/inventory/adjustments", h.adjustInventory)
	quarantines := root.Group("/inventory/quarantines")
	quarantines.POST("", h.quarantineInventory)
	quarantines.POST("/:quarantineId/resolve", h.resolveQuarantine)
	transfers := root.Group("/inventory/transfers")
	transfers.POST("", h.createTransfer)
	transfers.GET("/:transferId", h.getTransfer)
	transfers.POST("/:transferId/dispatch", h.dispatchTransfer)
	transfers.POST("/:transferId/receive", h.receiveTransfer)
	transfers.POST("/:transferId/cancel", h.cancelTransfer)
	stocktakes := root.Group("/inventory/stocktakes")
	stocktakes.GET("/:stocktakeId", h.getStocktake)
	stocktakes.PUT("/:stocktakeId/lines/:stockBasisId", h.countStocktake)
	stocktakes.POST("/:stocktakeId/post", h.postStocktake)
	root.PUT("/listings/:listingId/inventory-policy", h.setInventoryPolicy)
	root.POST("/inventory/fulfilment-measurements", h.recordFulfilment)
	root.PUT("/inventory/stock-bases/:stockBasisId/reorder-rule", h.setReorderRule)
	root.POST("/inventory/alerts/:alertId/acknowledge", h.transitionInventoryAlert("ACKNOWLEDGED"))
	root.POST("/inventory/alerts/:alertId/resolve", h.transitionInventoryAlert("RESOLVED"))
	bulk := root.Group("/bulk-operations")
	bulk.POST("", h.submitBulk)
	bulk.GET("/:jobId", h.getBulk)
	bulk.POST("/:jobId/apply", h.applyBulk)
}
