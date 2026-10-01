package referralcontract

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// WithdrawalEligibilityPath is where payment-wallet asks referral-service
// whether a driver may cash out referral earnings, just before it reserves
// the money. Referral-service owns the fraud picture (reviews, blocks,
// duplicate bank accounts); the wallet owns the balance.
const WithdrawalEligibilityPath = "/internal/referral/v1/withdrawal-eligibility"

// WithdrawalEligibilityRequest describes one attempted referral cash-out.
type WithdrawalEligibilityRequest struct {
	// ActorType is the driver account's type: hauling, dispatch or taxi.
	ActorType  string `json:"actor_type"`
	ActorID    string `json:"actor_id"`
	AmountKobo int64  `json:"amount_kobo"`
	// BankAccountHash is HashBankAccount of the destination account, so one
	// account receiving several people's referral money can be spotted
	// without referral-service ever holding the account number.
	BankAccountHash string `json:"bank_account_hash"`
}

// Validate checks the request is complete.
func (r WithdrawalEligibilityRequest) Validate() error {
	if !IsProviderActor(r.ActorType) || strings.TrimSpace(r.ActorID) == "" {
		return errors.New("referralcontract: withdrawal eligibility needs a driver account")
	}
	if r.AmountKobo <= 0 || strings.TrimSpace(r.BankAccountHash) == "" {
		return errors.New("referralcontract: withdrawal eligibility needs an amount and a bank account hash")
	}
	return nil
}

// Reasons a referral cash-out is refused.
const (
	// IneligibleUnknownAccount: referral-service has not heard of the
	// account (its announcement may still be in flight).
	IneligibleUnknownAccount = "unknown_account"
	// IneligibleUnderReview: the person is frozen, or something about them
	// is waiting for a reviewer.
	IneligibleUnderReview = "under_review"
	// IneligibleBlocked: the person was removed from the programme.
	IneligibleBlocked = "blocked"
)

// WithdrawalEligibility is referral-service's answer.
type WithdrawalEligibility struct {
	Eligible bool   `json:"eligible"`
	Reason   string `json:"reason,omitempty"`
}

// HashBankAccount returns the keyed hash of a bank account (bank code and
// account number), with the same pepper as every other identity hash.
func HashBankAccount(pepper []byte, bankCode, accountNumber string) string {
	accountNumber = strings.TrimSpace(accountNumber)
	if accountNumber == "" {
		return ""
	}
	return keyedHash(pepper, "bank", strings.TrimSpace(bankCode)+"/"+accountNumber)
}

// Client calls referral-service's internal API, signing each request as the
// calling service.
type Client struct {
	// BaseURL is referral-service's bare origin (no path).
	BaseURL     string
	ServiceName string
	Secret      []byte
	HTTPClient  *http.Client
}

// CheckWithdrawal asks whether a driver may cash out referral earnings.
// Any error means "do not pay": the caller must fail closed.
func (c Client) CheckWithdrawal(ctx context.Context, request WithdrawalEligibilityRequest) (WithdrawalEligibility, error) {
	if err := request.Validate(); err != nil {
		return WithdrawalEligibility{}, err
	}
	body, err := json.Marshal(request)
	if err != nil {
		return WithdrawalEligibility{}, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+WithdrawalEligibilityPath, bytes.NewReader(body))
	if err != nil {
		return WithdrawalEligibility{}, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	if err := serviceauth.SignRequest(httpRequest, c.ServiceName, c.Secret, body, time.Now()); err != nil {
		return WithdrawalEligibility{}, err
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	response, err := client.Do(httpRequest)
	if err != nil {
		return WithdrawalEligibility{}, err
	}
	defer response.Body.Close()
	var envelope struct {
		Success bool                  `json:"success"`
		Data    WithdrawalEligibility `json:"data"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return WithdrawalEligibility{}, fmt.Errorf("referralcontract: decode eligibility (HTTP %d): %w", response.StatusCode, err)
	}
	if response.StatusCode >= http.StatusBadRequest || !envelope.Success {
		message := envelope.Error.Message
		if message == "" {
			message = fmt.Sprintf("referral-service returned HTTP %d", response.StatusCode)
		}
		return WithdrawalEligibility{}, errors.New(message)
	}
	return envelope.Data, nil
}
