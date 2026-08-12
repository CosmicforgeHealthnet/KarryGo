package platformclients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const walletFinanceBase = "/api/v1/payment-wallet/internal/finance"

type walletResponse[T any] struct {
	Success bool `json:"success"`
	Data    T    `json:"data"`
}

// WalletOperationRequest is Fleet's versioned command envelope. Payload is a
// workflow-specific object; the client adds the security/concurrency fields so
// callers cannot accidentally omit them.
type WalletOperationRequest struct {
	OperationType   string
	OrganizationID  string
	SubjectID       string
	Payload         json.RawMessage
	PayloadVersion  int
	ExpectedVersion int64
	IdempotencyKey  string
}
type WalletOperation struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	Version        int64  `json:"version"`
	OrganizationID string `json:"organizationId"`
	AmountMinor    *int64 `json:"amountMinor,omitempty"`
	Currency       string `json:"currency,omitempty"`
}

// JobPaymentVerification is the minimum Wallet discloses about a job payment.
//
// There is deliberately no balance, ledger identifier or provider reference
// here: Fleet decides whether work was paid for, and nothing more.
type JobPaymentVerification struct {
	PaymentVerificationID string     `json:"paymentVerificationId"`
	OrganizationID        string     `json:"organizationId"`
	RequestReference      string     `json:"requestReference"`
	TripReference         string     `json:"tripReference"`
	AmountMinor           int64      `json:"amountMinor"`
	Currency              string     `json:"currency"`
	PaymentStatus         string     `json:"paymentStatus"`
	SourceVersion         int64      `json:"sourceVersion"`
	PaidAt                *time.Time `json:"paidAt"`
	RefundState           string     `json:"refundState"`
	ReversalState         string     `json:"reversalState"`
	OperationReference    string     `json:"operationReference"`
	Settled               bool       `json:"settled"`
}

// Supported authoritative job payment lookup keys.
const (
	JobPaymentByDispatchRequest = "DISPATCH_REQUEST"
	JobPaymentByTrip            = "TRIP"
	JobPaymentByBooking         = "BOOKING"
	JobPaymentByPaymentRef      = "PAYMENT_REFERENCE"
)

// WalletReversal is the durable reversal contract as Fleet observes it.
type WalletReversal struct {
	ID                         string `json:"id"`
	OrganizationID             string `json:"organizationId"`
	OriginalJournalID          string `json:"originalJournalId"`
	AmountMinor                int64  `json:"amountMinor"`
	Currency                   string `json:"currency"`
	Status                     string `json:"status"`
	ExternalOperationReference string `json:"externalOperationReference"`
	ResultingJournalID         string `json:"resultingJournalId"`
	SafeErrorCode              string `json:"safeErrorCode"`
	Version                    int64  `json:"version"`
}

type FinanceWalletContract interface {
	WalletContract
	SubmitOperation(context.Context, CallMeta, WalletOperationRequest) (WalletOperation, error)
	ReadOperation(context.Context, CallMeta, string, string, string) (WalletOperation, error)
	ReadOperationByKey(context.Context, CallMeta, string, string, string) (WalletOperation, error)
	ReadProjection(context.Context, CallMeta, string, string, string) (WalletOperation, error)
	// VerifyJobPayment is the authoritative payment check for a dispatch
	// request or canonical trip.
	VerifyJobPayment(context.Context, CallMeta, string, string, string) (JobPaymentVerification, error)
	// ReadReversal reads one durable reversal back by its Wallet identifier.
	ReadReversal(context.Context, CallMeta, string, string) (WalletReversal, error)
}

type WalletClient struct{ http *Client }

func NewWalletClient(options Options) *WalletClient { return &WalletClient{http: New(options)} }
func (c *WalletClient) Configured() bool            { return c != nil && c.http.Configured() }

func (c *WalletClient) EnsureOrganizationWallet(ctx context.Context, meta CallMeta, request WalletProvisionRequest) (WalletAccount, error) {
	body := map[string]any{"payloadVersion": 1, "organizationId": request.OrganizationID, "currency": request.Currency, "idempotencyKey": request.IdempotencyKey}
	var response walletResponse[struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		Version int64  `json:"version"`
	}]
	err := c.http.Do(ctx, meta, http.MethodPost, walletFinanceBase+"/wallets/ensure", body, &response)
	return WalletAccount{AccountID: response.Data.ID, Status: response.Data.Status, SourceVersion: response.Data.Version}, err
}
func (c *WalletClient) EnsureProviderSubWallet(ctx context.Context, meta CallMeta, request WalletProvisionRequest) (WalletAccount, error) {
	body := map[string]any{"payloadVersion": 1, "organizationId": request.OrganizationID, "providerService": "fleet-partner-service", "providerReference": request.ProviderMembershipID, "currency": request.Currency, "idempotencyKey": request.IdempotencyKey}
	var response walletResponse[struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		Version int64  `json:"version"`
	}]
	err := c.http.Do(ctx, meta, http.MethodPost, walletFinanceBase+"/earning-accounts/ensure", body, &response)
	return WalletAccount{AccountID: response.Data.ID, Status: response.Data.Status, SourceVersion: response.Data.Version}, err
}
func (c *WalletClient) ReadAccount(ctx context.Context, meta CallMeta, accountID string) (WalletAccount, error) {
	if meta.OrganizationID == "" {
		return WalletAccount{}, fmt.Errorf("wallet client: organization is required for account read")
	}
	var response walletResponse[struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		Version int64  `json:"version"`
	}]
	path := walletFinanceBase + "/earning-accounts/" + url.PathEscape(accountID) + "?organizationId=" + url.QueryEscape(meta.OrganizationID)
	err := c.http.Do(ctx, meta, http.MethodGet, path, nil, &response)
	if typed, ok := err.(*Error); ok && typed.Outcome == OutcomeNotFound {
		path = walletFinanceBase + "/wallets/" + url.PathEscape(accountID) + "?organizationId=" + url.QueryEscape(meta.OrganizationID)
		err = c.http.Do(ctx, meta, http.MethodGet, path, nil, &response)
	}
	return WalletAccount{AccountID: response.Data.ID, Status: response.Data.Status, SourceVersion: response.Data.Version}, err
}
func (c *WalletClient) DisableForClosure(ctx context.Context, meta CallMeta, accountID, reason string) error {
	account, err := c.ReadAccount(ctx, meta, accountID)
	if err != nil {
		return err
	}
	body := map[string]any{"payloadVersion": 1, "organizationId": meta.OrganizationID, "idempotencyKey": meta.IdempotencyKey, "expectedVersion": account.SourceVersion, "status": "RESTRICTED", "restrictions": []string{reason}}
	var response walletResponse[WalletOperation]
	return c.http.Do(ctx, meta, http.MethodPost, walletFinanceBase+"/wallets/"+url.PathEscape(accountID)+"/status", body, &response)
}

func (c *WalletClient) SubmitOperation(ctx context.Context, meta CallMeta, request WalletOperationRequest) (WalletOperation, error) {
	var fields map[string]any
	if len(request.Payload) > 0 {
		if err := json.Unmarshal(request.Payload, &fields); err != nil {
			return WalletOperation{}, err
		}
	} else {
		fields = map[string]any{}
	}
	if request.PayloadVersion == 0 {
		request.PayloadVersion = 1
	}
	fields["payloadVersion"] = request.PayloadVersion
	fields["organizationId"] = request.OrganizationID
	fields["idempotencyKey"] = request.IdempotencyKey
	fields["expectedVersion"] = request.ExpectedVersion
	path := ""
	method := http.MethodPost
	switch request.OperationType {
	case "SUBSCRIPTION_INVOICE":
		path = "/subscription-invoices"
	case "HOLD_CREATE":
		path = "/holds"
	case "HOLD_RELEASE":
		path = "/holds/" + url.PathEscape(request.SubjectID) + "/release"
	case "COMMISSION_POST", "EARNING_ALLOCATION", "FLEET_FEE_POST":
		path = "/earning-allocations"
	case "PROVIDER_ALLOCATION":
		path = "/provider-allocations"
	case "SETTLEMENT_REQUEST":
		path = "/settlements"
	case "REFUND_REQUEST":
		path = "/refunds"
	case "REVERSAL_REQUEST":
		path = "/reversals"
	default:
		return WalletOperation{}, fmt.Errorf("wallet client: unsupported operation %s", request.OperationType)
	}
	var response walletResponse[WalletOperation]
	err := c.http.Do(ctx, meta, method, walletFinanceBase+path, fields, &response)
	return response.Data, err
}
func (c *WalletClient) ReadOperation(ctx context.Context, meta CallMeta, organizationID, operationType, id string) (WalletOperation, error) {
	var response walletResponse[WalletOperation]
	path := walletFinanceBase + "/operations/" + url.PathEscape(operationType) + "/" + url.PathEscape(id) + "?organizationId=" + url.QueryEscape(organizationID)
	err := c.http.Do(ctx, meta, http.MethodGet, path, nil, &response)
	return response.Data, err
}
func (c *WalletClient) ReadOperationByKey(ctx context.Context, meta CallMeta, organizationID, operationType, key string) (WalletOperation, error) {
	var response walletResponse[WalletOperation]
	path := walletFinanceBase + "/operations/" + url.PathEscape(operationType) + "/by-idempotency?organizationId=" + url.QueryEscape(organizationID) + "&idempotencyKey=" + url.QueryEscape(key)
	err := c.http.Do(ctx, meta, http.MethodGet, path, nil, &response)
	return response.Data, err
}

// ReadProjection resolves Fleet's safe projection types through Wallet's
// tenant-scoped authoritative readers. It intentionally returns no balances or
// journal lines.
func (c *WalletClient) ReadProjection(ctx context.Context, meta CallMeta, organizationID, projectionType, reference string) (WalletOperation, error) {
	operationType := projectionType
	switch projectionType {
	case "COMMISSION", "EARNING":
		operationType = "EARNING_ALLOCATION"
	case "REVERSAL":
		operationType = "REVERSAL"
	}
	return c.ReadOperation(ctx, meta, organizationID, operationType, reference)
}

// VerifyJobPayment asks Wallet whether a unit of dispatched work was paid for.
//
// referenceType names which reference is being presented; Fleet never invents
// an identifier of its own for this, it presents one the authoritative service
// already knows.
func (c *WalletClient) VerifyJobPayment(ctx context.Context, meta CallMeta,
	organizationID, referenceType, reference string) (JobPaymentVerification, error) {
	if strings.TrimSpace(organizationID) == "" || strings.TrimSpace(reference) == "" {
		return JobPaymentVerification{}, fmt.Errorf("wallet client: organization and reference are required")
	}
	var response walletResponse[JobPaymentVerification]
	path := walletFinanceBase + "/job-payments/lookup?organizationId=" + url.QueryEscape(organizationID) +
		"&referenceType=" + url.QueryEscape(strings.ToUpper(referenceType)) +
		"&reference=" + url.QueryEscape(reference)
	err := c.http.Do(ctx, meta, http.MethodGet, path, nil, &response)
	return response.Data, err
}

// ReadReversal reads one durable reversal back.
func (c *WalletClient) ReadReversal(ctx context.Context, meta CallMeta,
	organizationID, reversalID string) (WalletReversal, error) {
	var response walletResponse[WalletReversal]
	path := walletFinanceBase + "/organizations/" + url.PathEscape(organizationID) +
		"/reversals/" + url.PathEscape(reversalID)
	err := c.http.Do(ctx, meta, http.MethodGet, path, nil, &response)
	return response.Data, err
}

var _ FinanceWalletContract = (*WalletClient)(nil)
