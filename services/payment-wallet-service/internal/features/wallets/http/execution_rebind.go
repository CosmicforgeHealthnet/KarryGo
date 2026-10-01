package wallethttp

// Section 17: the internal execution-provider rebind surface.

import (
	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
)

// executionProviderRebindRequest is the rebind on the wire.
//
// AmountKobo and Currency exist only so a body that carries them is refused.
// Silently ignoring them would let a caller believe it had changed the held
// amount when it had not.
type executionProviderRebindRequest struct {
	ProviderType    string  `json:"provider_type"`
	ProviderID      string  `json:"provider_id"`
	ExpectedVersion int64   `json:"expected_version"`
	RecoveryCaseRef string  `json:"recovery_case_ref"`
	ReasonCode      string  `json:"reason_code"`
	IdempotencyKey  string  `json:"idempotency_key"`
	AmountKobo      *int64  `json:"amount_kobo"`
	Currency        *string `json:"currency"`
}

// RebindExecutionProvider moves a held job payment to the replacement provider.
func (h *Handler) RebindExecutionProvider(c *gin.Context) {
	var request executionProviderRebindRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Abort(c, apperrors.BadRequest("Request body is invalid.", err))
		return
	}
	if request.AmountKobo != nil || request.Currency != nil {
		httpx.Abort(c, apperrors.Validation("A rebind cannot change the held amount or currency.",
			[]apperrors.FieldViolation{{Field: "amount_kobo", Message: "Not accepted on a rebind."}}))
		return
	}
	key := request.IdempotencyKey
	if key == "" {
		key = c.GetHeader("Idempotency-Key")
	}
	rebind, err := h.service.RebindExecutionProvider(c.Request.Context(), walletmodels.ExecutionProviderRebindRequest{
		SourceService: c.Param("source_service"), SourceReference: c.Param("source_reference"),
		CallerService:   serviceCaller(c),
		NewProviderType: request.ProviderType, NewProviderID: request.ProviderID,
		ExpectedVersion: request.ExpectedVersion, RecoveryCaseRef: request.RecoveryCaseRef,
		ReasonCode: request.ReasonCode, IdempotencyKey: key,
	})
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, rebind)
}

// GetExecutionProviderBinding returns the binding version and rebind history.
func (h *Handler) GetExecutionProviderBinding(c *gin.Context) {
	binding, err := h.service.GetExecutionProviderBinding(c.Request.Context(), serviceCaller(c),
		c.Param("source_service"), c.Param("source_reference"))
	if err != nil {
		httpx.Abort(c, err)
		return
	}
	respondOK(c, binding)
}
