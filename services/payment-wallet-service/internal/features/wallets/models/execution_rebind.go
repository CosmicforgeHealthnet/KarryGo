package walletmodels

import "time"

// ExecutionProviderRebindRequest moves a held payment's beneficiary to the
// provider who replaced the original one during the owning service's
// recovery (Section 17).
//
// It carries no amount and no currency on purpose: a rebind cannot change
// what is held, only who it will be released to.
type ExecutionProviderRebindRequest struct {
	SourceService   string
	SourceReference string
	CallerService   string
	NewProviderType string
	NewProviderID   string
	// ExpectedVersion is the binding version the caller last saw. A mismatch
	// means another rebind landed first and this one was decided on stale
	// information.
	ExpectedVersion int64
	RecoveryCaseRef string
	ReasonCode      string
	IdempotencyKey  string
}

// ExecutionProviderRebind is one recorded rebind.
type ExecutionProviderRebind struct {
	ID                   string    `json:"id"`
	PaymentIntentID      string    `json:"payment_intent_id"`
	SourceService        string    `json:"source_service"`
	SourceReference      string    `json:"source_reference"`
	CallerService        string    `json:"caller_service"`
	PreviousProviderType string    `json:"previous_provider_type"`
	PreviousProviderID   string    `json:"previous_provider_id"`
	NewProviderType      string    `json:"new_provider_type"`
	NewProviderID        string    `json:"new_provider_id"`
	FromVersion          int64     `json:"from_version"`
	ToVersion            int64     `json:"to_version"`
	RecoveryCaseRef      string    `json:"recovery_case_ref"`
	ReasonCode           string    `json:"reason_code,omitempty"`
	AmountKobo           int64     `json:"amount_kobo"`
	Currency             string    `json:"currency"`
	IdempotencyKey       string    `json:"idempotency_key"`
	CreatedAt            time.Time `json:"created_at"`
	// Replayed is true when this answer is the original result of an earlier
	// request with the same idempotency key.
	Replayed bool `json:"replayed"`
}
