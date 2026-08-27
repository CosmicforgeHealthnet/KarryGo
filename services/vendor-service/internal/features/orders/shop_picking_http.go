package orders

import (
	"net/http"
	"strconv"

	"cosmicforge/logistics/shared/go/httpx"
	"github.com/gin-gonic/gin"
)

type qualityResultRequest struct {
	Result        QualityResult `json:"result"`
	ObservedValue string        `json:"observed_value"`
	Reason        string        `json:"reason"`
}
type actualQuantityRequest struct {
	ActualQuantity int64 `json:"actual_quantity"`
}

func (h *Handler) pickingQueue(c *gin.Context) {
	actor, ok := vendorActor(c)
	if !ok {
		return
	}
	queue, err := h.service.PickingQueue(c.Request.Context(), actor, c.Param("vendorOrderId"))
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": queue})
}

func (h *Handler) recordQualityResult(c *gin.Context) {
	actor, ok := vendorActor(c)
	if !ok {
		return
	}
	version, err := expectedVersion(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	var request qualityResultRequest
	if err = strictJSON(c, &request); err != nil {
		httpx.RespondError(c, err)
		return
	}
	err = h.service.RecordQualityResult(c.Request.Context(), actor, c.Param("vendorOrderId"), c.Param("lineId"), version, request.Result, request.ObservedValue, request.Reason)
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) recordActualQuantity(c *gin.Context) {
	actor, ok := vendorActor(c)
	if !ok {
		return
	}
	version, err := expectedVersion(c)
	if err != nil {
		httpx.RespondError(c, err)
		return
	}
	var request actualQuantityRequest
	if err = strictJSON(c, &request); err != nil {
		httpx.RespondError(c, err)
		return
	}
	order, err := h.service.store.GetVendorOrderForVendor(c.Request.Context(), actor.VendorID, c.Param("vendorOrderId"))
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	measurement, err := h.service.RecordActualQuantity(c.Request.Context(), actor, c.Param("vendorOrderId"), c.Param("lineId"), version, request.ActualQuantity, order.MaxProductSubtotalMinor)
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": measurement})
}

func (h *Handler) pendingSubstitutionCases(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}
	cases, err := h.service.store.ListSubstitutionCasesForCustomer(c.Request.Context(), customerID, 100)
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": cases})
}

func (h *Handler) getSubstitutionCase(c *gin.Context) {
	customerID, ok := customerSubject(c)
	if !ok {
		return
	}
	caseEvidence, err := h.service.store.GetSubstitutionCaseForCustomer(c.Request.Context(), customerID, c.Param("caseId"))
	if err != nil {
		httpx.RespondError(c, orderHTTPError(err))
		return
	}
	c.Header("ETag", strconv.FormatInt(caseEvidence.Version, 10))
	c.JSON(http.StatusOK, gin.H{"success": true, "data": caseEvidence})
}

func (h *Handler) respondSubstitutionCase(accept bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		customerID, ok := customerSubject(c)
		if !ok {
			return
		}
		version, err := expectedVersion(c)
		if err != nil {
			httpx.RespondError(c, err)
			return
		}
		caseEvidence, err := h.service.store.RespondSubstitutionCase(c.Request.Context(), customerID, c.Param("caseId"), version, accept, customerID, h.service.now().UTC())
		if err != nil {
			httpx.RespondError(c, orderHTTPError(err))
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": caseEvidence})
	}
}
