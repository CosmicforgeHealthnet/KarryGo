package walletusecases

// Section 17: who a held payment is released to after provider replacement.
//
// Wallet does not decide that a replacement happened, or that it was
// justified. The execution service that owns the job ran its own recovery and
// chose the replacement; Wallet records that decision against the held money
// and refuses anything that would change what is held rather than who holds
// the claim to it.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"

	walletmodels "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/models"
	walletrepositories "cosmicforge/logistics/services/payment-wallet-service/internal/features/wallets/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
)

// rebindAuthorities maps a payment's source service to the identities allowed
// to rebind it.
//
// Only the service that owns the job may move its beneficiary: it is the one
// that ran the recovery. Fleet is deliberately absent -- corporate work
// settles through Fleet's own instruction, never through the intent's bound
// provider -- and so is every other execution service, which holds a valid
// signature for its OWN jobs and nothing else. Both the provisioned and the
// directory-style spellings of each identity are listed, for the reason given
// on requireFinanceReader.
var rebindAuthorities = map[string][]string{
	"driver-hauling-service":           {"driver-hauling-service", "hauling-service"},
	"dispatch-delivery-service":        {"dispatch-delivery-service", "driver-dispatch-delivery-service"},
	"driver-dispatch-delivery-service": {"dispatch-delivery-service", "driver-dispatch-delivery-service"},
	"taxi-service":                     {"taxi-service", "driver-taxi-service"},
	"driver-taxi-service":              {"taxi-service", "driver-taxi-service"},
}

func mayRebind(caller, sourceService string) bool {
	for _, allowed := range rebindAuthorities[sourceService] {
		if caller == allowed {
			return true
		}
	}
	return false
}

// Rebind error codes. Distinct codes let the caller tell "someone else moved
// it first" (re-read and decide) from "the job is past rebinding" (stop).
const (
	CodeRebindVersionConflict = apperrors.Code("binding_version_conflict")
	CodeRebindNotHeld         = apperrors.Code("payment_not_held")
	CodeRebindKeyReused       = apperrors.Code("idempotency_key_reused")
	CodeRebindNoChange        = apperrors.Code("provider_already_bound")
)

// RebindExecutionProvider moves a held payment to the replacement provider.
func (s *WalletService) RebindExecutionProvider(ctx context.Context,
	request walletmodels.ExecutionProviderRebindRequest) (walletmodels.ExecutionProviderRebind, error) {
	request = normalizeRebind(request)
	if !mayRebind(request.CallerService, request.SourceService) {
		return walletmodels.ExecutionProviderRebind{}, apperrors.Forbidden(
			"Only the service that owns this job may rebind its payment.", nil)
	}
	if fields := validateRebind(request); len(fields) > 0 {
		return walletmodels.ExecutionProviderRebind{}, apperrors.Validation("Check the rebind request.", fields)
	}
	rebind, err := s.repository.RebindExecutionProvider(ctx, request, rebindFingerprint(request))
	switch {
	case errors.Is(err, walletrepositories.ErrRebindVersionConflict):
		return rebind, apperrors.New(http.StatusConflict, CodeRebindVersionConflict,
			"The payment binding changed since it was read. Read it again before rebinding.", err)
	case errors.Is(err, walletrepositories.ErrRebindNotHeld):
		return rebind, apperrors.New(http.StatusConflict, CodeRebindNotHeld,
			"Only a held payment can be rebound.", err)
	case errors.Is(err, walletrepositories.ErrRebindKeyReused):
		return rebind, apperrors.New(http.StatusConflict, CodeRebindKeyReused,
			"This idempotency key was already used for a different rebind.", err)
	case errors.Is(err, walletrepositories.ErrRebindNoChange):
		return rebind, apperrors.New(http.StatusConflict, CodeRebindNoChange,
			"The payment is already bound to this provider.", err)
	}
	return rebind, err
}

// ExecutionProviderBinding is what a caller needs before rebinding.
type ExecutionProviderBinding struct {
	Version      int64                                  `json:"execution_binding_version"`
	ProviderType string                                 `json:"provider_type"`
	ProviderID   string                                 `json:"provider_id"`
	Status       string                                 `json:"status"`
	History      []walletmodels.ExecutionProviderRebind `json:"rebinds"`
}

// GetExecutionProviderBinding returns the current binding version and the
// rebind history, under the same caller rule as the rebind itself.
func (s *WalletService) GetExecutionProviderBinding(ctx context.Context,
	caller, sourceService, sourceReference string) (ExecutionProviderBinding, error) {
	sourceService, sourceReference = strings.TrimSpace(sourceService), strings.TrimSpace(sourceReference)
	if !mayRebind(strings.TrimSpace(caller), sourceService) {
		return ExecutionProviderBinding{}, apperrors.Forbidden(
			"Only the service that owns this job may read its payment binding.", nil)
	}
	version, err := s.repository.ExecutionBindingVersion(ctx, sourceService, sourceReference)
	if err != nil {
		return ExecutionProviderBinding{}, err
	}
	intent, err := s.repository.GetPaymentIntentBySource(ctx, sourceService, sourceReference)
	if err != nil {
		return ExecutionProviderBinding{}, err
	}
	history, err := s.repository.ListExecutionProviderRebinds(ctx, sourceService, sourceReference)
	if err != nil {
		return ExecutionProviderBinding{}, err
	}
	return ExecutionProviderBinding{Version: version, ProviderType: intent.ProviderType,
		ProviderID: intent.ProviderID, Status: intent.Status, History: history}, nil
}

func normalizeRebind(request walletmodels.ExecutionProviderRebindRequest) walletmodels.ExecutionProviderRebindRequest {
	request.SourceService = strings.TrimSpace(request.SourceService)
	request.SourceReference = strings.TrimSpace(request.SourceReference)
	request.CallerService = strings.TrimSpace(request.CallerService)
	request.NewProviderType = strings.TrimSpace(request.NewProviderType)
	request.NewProviderID = strings.TrimSpace(request.NewProviderID)
	request.RecoveryCaseRef = strings.TrimSpace(request.RecoveryCaseRef)
	request.ReasonCode = strings.ToUpper(strings.TrimSpace(request.ReasonCode))
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	return request
}

func validateRebind(request walletmodels.ExecutionProviderRebindRequest) []apperrors.FieldViolation {
	var fields []apperrors.FieldViolation
	require := func(field, value, message string) {
		if value == "" {
			fields = append(fields, apperrors.FieldViolation{Field: field, Message: message})
		}
	}
	require("source_reference", request.SourceReference, "Source reference is required.")
	require("provider_type", request.NewProviderType, "Provider type is required.")
	require("provider_id", request.NewProviderID, "Provider id is required.")
	// A rebind with no recovery case behind it is indistinguishable from
	// someone redirecting a payout.
	require("recovery_case_ref", request.RecoveryCaseRef, "Recovery case reference is required.")
	require("idempotency_key", request.IdempotencyKey, "Idempotency key is required.")
	if request.ExpectedVersion < 1 {
		fields = append(fields, apperrors.FieldViolation{Field: "expected_version",
			Message: "Expected binding version is required."})
	}
	return fields
}

// rebindFingerprint identifies the request's content, so a reused key with
// different content is refused rather than silently answered with the
// original.
func rebindFingerprint(request walletmodels.ExecutionProviderRebindRequest) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		request.SourceService, request.SourceReference, request.CallerService,
		request.NewProviderType, request.NewProviderID, strconv.FormatInt(request.ExpectedVersion, 10),
		request.RecoveryCaseRef, request.ReasonCode,
	}, "\x1f")))
	return hex.EncodeToString(sum[:])
}
