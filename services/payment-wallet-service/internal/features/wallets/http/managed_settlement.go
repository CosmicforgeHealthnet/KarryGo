package wallethttp

// Section 12: the internal managed-settlement surface.
//
// Every route here sits inside the HMAC-protected /internal group and then
// narrows further by caller: posting is Fleet's alone, reading is open to
// Fleet and the three execution services, because a service reconciling its
// own completion needs to know whether the money moved.

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
)

// managedSettlementRequest is Fleet's instruction on the wire.
//
// It mirrors Fleet's ManagedWorkSettlementInstruction exactly. Nothing is
// derived here: a field Fleet did not send is a field Wallet does not have.
type managedSettlementRequest struct {
	InstructionID      string `json:"instruction_id"`
	InstructionVersion int64  `json:"instruction_version"`

	OrganizationID string `json:"organization_id"`
	MembershipID   string `json:"membership_id"`
	ProviderID     string `json:"provider_id"`
	ProviderType   string `json:"provider_type"`

	SourceService      string `json:"source_service"`
	SourceJobID        string `json:"source_job_id"`
	SourceAssignmentID string `json:"source_assignment_id"`

	Currency               string `json:"currency"`
	GrossBasisMinor        int64  `json:"gross_basis_minor"`
	ProviderShareMinor     int64  `json:"provider_share_minor"`
	OrganizationShareMinor int64  `json:"organization_share_minor"`
	PlatformShareMinor     *int64 `json:"platform_share_minor"`

	CompensationPlanID            string `json:"compensation_plan_id"`
	CompensationPlanVersion       int64  `json:"compensation_plan_version"`
	CompensationResolutionVersion int64  `json:"compensation_resolution_version"`

	CorrelationID  string `json:"correlation_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

func (r managedSettlementRequest) instruction() walletmodels.ManagedSettlementInstruction {
	return walletmodels.ManagedSettlementInstruction{
		InstructionID: r.InstructionID, InstructionVersion: r.InstructionVersion,
		OrganizationID: r.OrganizationID, MembershipID: r.MembershipID,
		ProviderID: r.ProviderID, ProviderType: r.ProviderType,
		SourceService: r.SourceService, SourceJobID: r.SourceJobID,
		SourceAssignmentID: r.SourceAssignmentID, Currency: r.Currency,
		GrossBasisMinor: r.GrossBasisMinor, ProviderShareMinor: r.ProviderShareMinor,
		OrganizationShareMinor: r.OrganizationShareMinor, PlatformShareMinor: r.PlatformShareMinor,
		CompensationPlanID: r.CompensationPlanID, CompensationPlanVersion: r.CompensationPlanVersion,
		CompensationResolutionVersion: r.CompensationResolutionVersion,
		CorrelationID:                 r.CorrelationID, IdempotencyKey: r.IdempotencyKey,
	}
}

// serviceCaller reads the identity the HMAC middleware verified.
func serviceCaller(c *gin.Context) string {
	value, _ := c.Get("service_name")
	name, _ := value.(string)
	return name
}

// PostManagedSettlement posts one Fleet settlement instruction.
func (h *Handler) PostManagedSettlement(c *gin.Context) {
	var request managedSettlementRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	settlement, err := h.service.PostManagedSettlement(
		c.Request.Context(), serviceCaller(c), request.instruction())
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, settlement)
}

// GetManagedSettlement reads a settlement back by instruction identity.
func (h *Handler) GetManagedSettlement(c *gin.Context) {
	version, err := strconv.ParseInt(c.Param("instructionVersion"), 10, 64)
	if err != nil {
		httpx.Abort(c, apperrors.BadRequest("Instruction version must be a number.", err))
		return
	}
	settlement, err := h.service.GetManagedSettlement(
		c.Request.Context(), serviceCaller(c), c.Param("instructionId"), version)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, settlement)
}

// GetManagedSettlementByKey reads a settlement back by idempotency key.
func (h *Handler) GetManagedSettlementByKey(c *gin.Context) {
	settlement, err := h.service.GetManagedSettlementByKey(
		c.Request.Context(), serviceCaller(c), c.Param("idempotencyKey"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, settlement)
}

// ListManagedSettlementsForJob answers whether one execution job settled.
func (h *Handler) ListManagedSettlementsForJob(c *gin.Context) {
	settlements, err := h.service.ListManagedSettlementsForJob(
		c.Request.Context(), serviceCaller(c), c.Param("sourceService"), c.Param("sourceJobId"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	if settlements == nil {
		settlements = []walletmodels.ManagedSettlement{}
	}
	respondOK(c, gin.H{"settlements": settlements})
}

// MarkJobManaged closes the legacy direct-payout path for one job.
func (h *Handler) MarkJobManaged(c *gin.Context) {
	if err := h.service.MarkJobManaged(
		c.Request.Context(), serviceCaller(c), c.Param("sourceService"), c.Param("sourceJobId"),
	); err != nil {
		httpx.Abort(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"source_service":  c.Param("sourceService"),
		"source_job_id":   c.Param("sourceJobId"),
		"settlement_mode": walletmodels.SettlementModeManaged,
	}})
}

type reverseManagedSettlementRequest struct {
	ReasonCode string `json:"reason_code"`
}

// ReverseManagedSettlement posts a compensating journal for one settlement.
func (h *Handler) ReverseManagedSettlement(c *gin.Context) {
	version, err := strconv.ParseInt(c.Param("instructionVersion"), 10, 64)
	if err != nil {
		httpx.Abort(c, apperrors.BadRequest("Instruction version must be a number.", err))
		return
	}
	var request reverseManagedSettlementRequest
	if bindErr := c.ShouldBindJSON(&request); bindErr != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", bindErr))
		return
	}
	settlement, err := h.service.ReverseManagedSettlement(
		c.Request.Context(), serviceCaller(c), c.Param("instructionId"), version, request.ReasonCode)
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, settlement)
}
