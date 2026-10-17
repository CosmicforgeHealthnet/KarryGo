package productionbatch

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/features/production"
	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/shared/go/httpx"
)

type Handler struct {
	service     *Service
	idempotency *idempotency.Store
}

func NewHandler(
	service *Service,
	store *idempotency.Store,
) *Handler {
	return &Handler{
		service:     service,
		idempotency: store,
	}
}

func strictJSON(c *gin.Context, destination any) error {
	decoder := json.NewDecoder(
		io.LimitReader(c.Request.Body, 1<<20),
	)

	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		return err
	}

	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return ErrInvalidInput
	}

	return nil
}

func expectedVersion(c *gin.Context, allowZero bool) (int64, error) {
	raw := strings.TrimSpace(c.GetHeader("If-Match"))

	if raw == "" {
		if allowZero {
			return 0, nil
		}
		return 0, ErrVersionConflict
	}

	raw = strings.Trim(raw, `"`)

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 || (!allowZero && value == 0) {
		return 0, ErrVersionConflict
	}

	return value, nil
}

func (h *Handler) actor(c *gin.Context) (production.Actor, error) {
	staff, ok := authctx.FromContext(c)
	if !ok {
		return production.Actor{}, errors.New("authentication required")
	}

	return production.Actor{
		StaffID:       staff.StaffID,
		VendorID:      staff.VendorID,
		SessionID:     staff.SessionID,
		CorrelationID: httpx.GetRequestID(c),
		CausationID:   c.GetHeader("X-Causation-ID"),
		CommandRef:    c.GetHeader("Idempotency-Key"),
	}, nil
}

func productionBatchError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "PRODUCTION_BATCH_NOT_FOUND",
				"message": "Production batch resource was not found.",
			},
		})

	case errors.Is(err, ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "PRODUCTION_BATCH_INVALID_INPUT",
				"message": "Production batch input is invalid.",
			},
		})

	case errors.Is(err, ErrVersionConflict):
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "PRODUCTION_BATCH_VERSION_CONFLICT",
				"message": "Production batch version is stale or If-Match is missing.",
			},
		})

	case errors.Is(err, ErrTopUpRequired):
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "PRODUCTION_INVENTORY_TOP_UP_REQUIRED",
				"message": "Actual production use exceeds reserved stock. Reserve the additional quantity explicitly before continuing.",
			},
		})

	case errors.Is(err, ErrInventoryShortage):
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "PRODUCTION_INVENTORY_UNAVAILABLE",
				"message": "Required production Inventory could not be reserved atomically.",
			},
		})

	case errors.Is(err, ErrInventoryRequired):
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "PRODUCTION_INVENTORY_MAPPING_REQUIRED",
				"message": "Production requires an exact active Inventory mapping.",
			},
		})

	case errors.Is(err, ErrCorrectionRequired):
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "PRODUCTION_CORRECTION_REQUIRED",
				"message": "Physical stock has already been posted. Use explicit Inventory correction evidence instead of cancelling and restoring stock.",
			},
		})

	case errors.Is(err, ErrConsumptionRequired):
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "PRODUCTION_CONSUMPTION_REQUIRED",
				"message": "Record how much of each ingredient was used before completing this batch.",
			},
		})

	case errors.Is(err, ErrStateConflict),
		errors.Is(err, ErrCapacityConflict):
		c.JSON(http.StatusConflict, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "PRODUCTION_BATCH_STATE_CONFLICT",
				"message": "The requested production transition is not valid in the current state.",
			},
		})

	default:
		httpx.RespondError(c, err)
	}
}

func ok(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{
		"success": true,
		"data":    data,
	})
}

func (h *Handler) authorized(
	c *gin.Context,
	permission string,
) (production.Actor, production.Scope, bool) {
	actor, err := h.actor(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "UNAUTHORIZED",
				"message": "Authentication is required.",
			},
		})
		return production.Actor{}, production.Scope{}, false
	}

	scope, err := h.service.Authorize(
		c,
		actor,
		permission,
	)
	if err != nil {
		httpx.RespondError(c, err)
		return production.Actor{}, production.Scope{}, false
	}

	return actor, scope, true
}

func (h *Handler) authorizedBatch(
	c *gin.Context,
	permission string,
) (production.Actor, production.Scope, Batch, bool) {
	actor, scope, ok := h.authorized(c, permission)
	if !ok {
		return production.Actor{}, production.Scope{}, Batch{}, false
	}

	batch, err := h.service.GetBatch(
		c,
		actor,
		c.Param("batchId"),
	)
	if err != nil {
		productionBatchError(c, err)
		return production.Actor{}, production.Scope{}, Batch{}, false
	}

	if !scope.Covers(batch.BranchID) {
		productionBatchError(c, ErrNotFound)
		return production.Actor{}, production.Scope{}, Batch{}, false
	}

	return actor, scope, batch, true
}

func (h *Handler) createBatch(c *gin.Context) {
	actor, scope, authorized := h.authorized(c, PermissionManage)
	if !authorized {
		return
	}

	var input CreateBatchInput
	if err := strictJSON(c, &input); err != nil {
		productionBatchError(c, ErrInvalidInput)
		return
	}

	if !scope.Covers(strings.TrimSpace(input.BranchID)) {
		productionBatchError(c, ErrNotFound)
		return
	}

	out, err := h.service.CreateBatch(c, actor, input)
	if err != nil {
		productionBatchError(c, err)
		return
	}

	ok(c, http.StatusCreated, out)
}

func (h *Handler) listBatches(c *gin.Context) {
	actor, scope, authorized := h.authorized(c, PermissionRead)
	if !authorized {
		return
	}

	limit := 50
	offset := 0

	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			productionBatchError(c, ErrInvalidInput)
			return
		}
		limit = value
	}

	if raw := strings.TrimSpace(c.Query("offset")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			productionBatchError(c, ErrInvalidInput)
			return
		}
		offset = value
	}

	items, more, err := h.service.ListBatches(
		c,
		actor,
		scope,
		c.Query("branch_id"),
		c.Query("status"),
		limit,
		offset,
	)
	if err != nil {
		productionBatchError(c, err)
		return
	}

	ok(c, http.StatusOK, gin.H{
		"items": items,
		"pagination": gin.H{
			"limit":    limit,
			"offset":   offset,
			"has_more": more,
		},
	})
}

func (h *Handler) getBatch(c *gin.Context) {
	actor, _, _, authorized :=
		h.authorizedBatch(c, PermissionRead)
	if !authorized {
		return
	}

	out, err := h.service.GetBatchDetail(
		c,
		actor,
		c.Param("batchId"),
	)
	if err != nil {
		productionBatchError(c, err)
		return
	}

	ok(c, http.StatusOK, out)
}

func (h *Handler) preview(c *gin.Context) {
	actor, _, _, authorized :=
		h.authorizedBatch(c, PermissionManage)
	if !authorized {
		return
	}

	out, err := h.service.PreviewBatch(
		c,
		actor,
		c.Param("batchId"),
	)
	if err != nil {
		productionBatchError(c, err)
		return
	}

	ok(c, http.StatusOK, out)
}

func (h *Handler) plan(c *gin.Context) {
	actor, _, _, authorized :=
		h.authorizedBatch(c, PermissionManage)
	if !authorized {
		return
	}

	version, err := expectedVersion(c, false)
	if err != nil {
		productionBatchError(c, err)
		return
	}

	out, err := h.service.PlanBatch(
		c,
		actor,
		c.Param("batchId"),
		version,
	)
	if err != nil {
		productionBatchError(c, err)
		return
	}

	ok(c, http.StatusOK, out)
}

func (h *Handler) topUp(c *gin.Context) {
	actor, _, _, authorized :=
		h.authorizedBatch(c, PermissionManage)
	if !authorized {
		return
	}

	version, err := expectedVersion(c, false)
	if err != nil {
		productionBatchError(c, err)
		return
	}

	var input TopUpInput
	if err = strictJSON(c, &input); err != nil {
		productionBatchError(c, ErrInvalidInput)
		return
	}

	out, err := h.service.TopUpInventory(
		c,
		actor,
		c.Param("batchId"),
		input,
		version,
	)
	if err != nil {
		productionBatchError(c, err)
		return
	}

	ok(c, http.StatusOK, out)
}

func (h *Handler) start(c *gin.Context) {
	actor, _, _, authorized :=
		h.authorizedBatch(c, PermissionOperate)
	if !authorized {
		return
	}

	version, err := expectedVersion(c, false)
	if err != nil {
		productionBatchError(c, err)
		return
	}

	out, err := h.service.StartBatch(
		c,
		actor,
		c.Param("batchId"),
		version,
	)
	if err != nil {
		productionBatchError(c, err)
		return
	}

	ok(c, http.StatusOK, out)
}

func (h *Handler) actual(c *gin.Context) {
	actor, _, _, authorized :=
		h.authorizedBatch(c, PermissionOperate)
	if !authorized {
		return
	}

	version, err := expectedVersion(c, false)
	if err != nil {
		productionBatchError(c, err)
		return
	}

	var input ActualConsumptionInput
	if err = strictJSON(c, &input); err != nil {
		productionBatchError(c, ErrInvalidInput)
		return
	}

	out, err := h.service.RecordActualConsumption(
		c,
		actor,
		c.Param("batchId"),
		input,
		version,
	)
	if err != nil {
		productionBatchError(c, err)
		return
	}

	ok(c, http.StatusOK, out)
}

func (h *Handler) waste(c *gin.Context) {
	actor, _, _, authorized :=
		h.authorizedBatch(c, PermissionWasteRecord)
	if !authorized {
		return
	}

	version, err := expectedVersion(c, false)
	if err != nil {
		productionBatchError(c, err)
		return
	}

	var input WasteInput
	if err = strictJSON(c, &input); err != nil {
		productionBatchError(c, ErrInvalidInput)
		return
	}

	out, err := h.service.RecordWaste(
		c,
		actor,
		c.Param("batchId"),
		input,
		version,
	)
	if err != nil {
		productionBatchError(c, err)
		return
	}

	ok(c, http.StatusOK, out)
}

func (h *Handler) complete(c *gin.Context) {
	actor, _, batch, authorized :=
		h.authorizedBatch(c, PermissionOperate)
	if !authorized {
		return
	}

	if batch.OperatingModel == OperatingBatchStock {
		preparedScope, err := h.service.Authorize(
			c,
			actor,
			PermissionPreparedStock,
		)
		if err != nil {
			httpx.RespondError(c, err)
			return
		}
		if !preparedScope.Covers(batch.BranchID) {
			productionBatchError(c, ErrNotFound)
			return
		}
	}

	version, err := expectedVersion(c, false)
	if err != nil {
		productionBatchError(c, err)
		return
	}

	var input CompleteInput
	if err = strictJSON(c, &input); err != nil {
		productionBatchError(c, ErrInvalidInput)
		return
	}

	out, err := h.service.CompleteBatch(
		c,
		actor,
		c.Param("batchId"),
		input,
		version,
	)
	if err != nil {
		productionBatchError(c, err)
		return
	}

	ok(c, http.StatusOK, out)
}

func (h *Handler) cancel(c *gin.Context) {
	actor, _, _, authorized :=
		h.authorizedBatch(c, PermissionManage)
	if !authorized {
		return
	}

	version, err := expectedVersion(c, false)
	if err != nil {
		productionBatchError(c, err)
		return
	}

	var input CancelInput
	if err = strictJSON(c, &input); err != nil {
		productionBatchError(c, ErrInvalidInput)
		return
	}

	out, err := h.service.CancelBatch(
		c,
		actor,
		c.Param("batchId"),
		input,
		version,
	)
	if err != nil {
		productionBatchError(c, err)
		return
	}

	ok(c, http.StatusOK, out)
}

func (h *Handler) inventory(c *gin.Context) {
	actor, _, _, authorized :=
		h.authorizedBatch(c, PermissionRead)
	if !authorized {
		return
	}

	out, err := h.service.ListReservations(
		c,
		actor,
		c.Param("batchId"),
	)
	if err != nil {
		productionBatchError(c, err)
		return
	}

	ok(c, http.StatusOK, gin.H{
		"reservations": out,
	})
}

func (h *Handler) output(c *gin.Context) {
	actor, _, _, authorized :=
		h.authorizedBatch(c, PermissionRead)
	if !authorized {
		return
	}

	out, err := h.service.GetOutput(
		c,
		actor,
		c.Param("batchId"),
	)
	if err != nil {
		productionBatchError(c, err)
		return
	}

	ok(c, http.StatusOK, out)
}

func (h *Handler) getPolicy(c *gin.Context) {
	actor, scope, authorized :=
		h.authorized(c, PermissionRead)
	if !authorized {
		return
	}

	branchID := strings.TrimSpace(c.Param("branchId"))
	if !scope.Covers(branchID) {
		productionBatchError(c, ErrNotFound)
		return
	}

	out, err := h.service.GetStockPolicy(
		c,
		actor,
		branchID,
		c.Query("target_type"),
		c.Query("target_ref"),
		c.Query("target_variant_ref"),
	)
	if err != nil {
		productionBatchError(c, err)
		return
	}

	ok(c, http.StatusOK, out)
}

func (h *Handler) putPolicy(c *gin.Context) {
	actor, scope, authorized :=
		h.authorized(c, PermissionPreparedStock)
	if !authorized {
		return
	}

	branchID := strings.TrimSpace(c.Param("branchId"))
	if !scope.Covers(branchID) {
		productionBatchError(c, ErrNotFound)
		return
	}

	version, err := expectedVersion(c, true)
	if err != nil {
		productionBatchError(c, err)
		return
	}

	var input PutStockPolicyInput
	if err = strictJSON(c, &input); err != nil {
		productionBatchError(c, ErrInvalidInput)
		return
	}

	out, err := h.service.PutStockPolicy(
		c,
		actor,
		branchID,
		input,
		version,
	)
	if err != nil {
		productionBatchError(c, err)
		return
	}

	ok(c, http.StatusOK, out)
}

func RegisterRoutes(
	root *gin.RouterGroup,
	h *Handler,
	enabled bool,
) {
	if root == nil || h == nil || !enabled {
		return
	}

	// Reads do not require Idempotency-Key.
	read := root.Group("/production-batches")
	read.GET("", h.listBatches)
	read.GET("/:batchId", h.getBatch)
	read.GET("/:batchId/inventory", h.inventory)
	read.GET("/:batchId/output", h.output)

	policyRead := root.Group("/production-stock-policies")
	policyRead.GET("/branches/:branchId", h.getPolicy)

	// Command edge.
	write := root.Group("/production-batches")
	write.Use(idempotency.CommandMiddleware(h.idempotency))
	write.POST("", h.createBatch)
	write.POST("/:batchId/preview", h.preview)
	write.POST("/:batchId/plan", h.plan)
	write.POST("/:batchId/inventory-adjustments", h.topUp)
	write.POST("/:batchId/start", h.start)
	write.POST("/:batchId/actual-consumption", h.actual)
	write.POST("/:batchId/waste", h.waste)
	write.POST("/:batchId/complete", h.complete)
	write.POST("/:batchId/cancel", h.cancel)

	policyWrite := root.Group("/production-stock-policies")
	policyWrite.Use(idempotency.CommandMiddleware(h.idempotency))
	policyWrite.PUT("/branches/:branchId", h.putPolicy)
}
