package walletclient

// Section 17: moving a held job payment to the provider who replaced the
// original one during the owning service's recovery.

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
)

// RequestError is Wallet's answer when it refused a request. Error() is the
// message alone, exactly as before this type existed, so callers that only
// print an error are unaffected; callers that need to act on the refusal read
// Status and Code.
type RequestError struct {
	Status  int
	Code    string
	Message string
}

func (e *RequestError) Error() string { return e.Message }

// RequestErrorCode returns the code of a Wallet refusal, or "" when err is not
// one -- a transport failure, whose outcome is unknown.
func RequestErrorCode(err error) string {
	var requestErr *RequestError
	if errors.As(err, &requestErr) {
		return requestErr.Code
	}
	return ""
}

// Rebind refusal codes.
const (
	CodeBindingVersionConflict = "binding_version_conflict"
	CodePaymentNotHeld         = "payment_not_held"
	CodeProviderAlreadyBound   = "provider_already_bound"
)

// ErrPaymentNotHeld: the payment is not held, so there is no beneficiary left
// to move -- it was settled, refunded, or never secured.
var ErrPaymentNotHeld = errors.New("payment is not held; nothing to rebind")

// ExecutionProviderBinding is a job payment's current binding.
type ExecutionProviderBinding struct {
	Version      int64                     `json:"execution_binding_version"`
	ProviderType string                    `json:"provider_type"`
	ProviderID   string                    `json:"provider_id"`
	Status       string                    `json:"status"`
	History      []ExecutionProviderRebind `json:"rebinds"`
}

// ExecutionProviderRebindRequest is one rebind. It carries no amount or
// currency: a rebind cannot change what is held.
type ExecutionProviderRebindRequest struct {
	ProviderType    string `json:"provider_type"`
	ProviderID      string `json:"provider_id"`
	ExpectedVersion int64  `json:"expected_version"`
	RecoveryCaseRef string `json:"recovery_case_ref"`
	ReasonCode      string `json:"reason_code,omitempty"`
	IdempotencyKey  string `json:"idempotency_key"`
}

// ExecutionProviderRebind is Wallet's record of one rebind.
type ExecutionProviderRebind struct {
	ID                 string `json:"id"`
	PreviousProviderID string `json:"previous_provider_id"`
	NewProviderType    string `json:"new_provider_type"`
	NewProviderID      string `json:"new_provider_id"`
	FromVersion        int64  `json:"from_version"`
	ToVersion          int64  `json:"to_version"`
	RecoveryCaseRef    string `json:"recovery_case_ref"`
	Replayed           bool   `json:"replayed"`
}

func executionProviderPath(sourceService, sourceReference string) string {
	return "/api/v1/payment-wallet/internal/jobs/" + url.PathEscape(sourceService) + "/" +
		url.PathEscape(sourceReference) + "/execution-provider"
}

// ExecutionProviderBinding reads a job payment's binding.
func (c Client) ExecutionProviderBinding(ctx context.Context, sourceService, sourceReference string) (ExecutionProviderBinding, error) {
	var binding ExecutionProviderBinding
	err := c.get(ctx, executionProviderPath(sourceService, sourceReference), &binding)
	return binding, err
}

// RebindExecutionProvider sends one rebind.
func (c Client) RebindExecutionProvider(ctx context.Context, sourceService, sourceReference string,
	request ExecutionProviderRebindRequest) (ExecutionProviderRebind, error) {
	var rebind ExecutionProviderRebind
	err := c.post(ctx, executionProviderPath(sourceService, sourceReference), request, &rebind)
	return rebind, err
}

// RebindToReplacement makes the job's held payment belong to the replacement.
//
// It reads the binding, then rebinds against the version it read. The
// idempotency key is derived from the recovery case AND that version, so:
//
//   - a retry after a lost response re-reads the same version and sends the
//     same key, which Wallet answers with the original rebind;
//   - if another rebind landed in between, the version moved, and the next
//     pass reads the new state and decides again.
//
// Already bound to the replacement is success. A payment that is no longer
// held returns ErrPaymentNotHeld: the caller decides whether that matters (a
// refunded booking has nothing to pay anyone). Any other error leaves the
// outcome unknown; retrying with the same arguments is always safe.
func (c Client) RebindToReplacement(ctx context.Context, sourceService, sourceReference,
	providerType, providerID, recoveryCaseRef, reasonCode string) error {
	for attempt := 0; attempt < 3; attempt++ {
		binding, err := c.ExecutionProviderBinding(ctx, sourceService, sourceReference)
		if err != nil {
			return fmt.Errorf("read payment binding: %w", err)
		}
		if binding.ProviderID == providerID && binding.ProviderType == providerType {
			return nil
		}
		if binding.Status != "held" {
			return ErrPaymentNotHeld
		}
		_, err = c.RebindExecutionProvider(ctx, sourceService, sourceReference, ExecutionProviderRebindRequest{
			ProviderType: providerType, ProviderID: providerID, ExpectedVersion: binding.Version,
			RecoveryCaseRef: recoveryCaseRef, ReasonCode: reasonCode,
			IdempotencyKey: "rebind-" + sourceReference + "-" + recoveryCaseRef + "-v" +
				strconv.FormatInt(binding.Version, 10),
		})
		switch RequestErrorCode(err) {
		case "":
			return err
		case CodeProviderAlreadyBound:
			return nil
		case CodePaymentNotHeld:
			return ErrPaymentNotHeld
		case CodeBindingVersionConflict:
			continue
		default:
			return err
		}
	}
	return errors.New("payment binding kept changing; rebind not applied")
}
