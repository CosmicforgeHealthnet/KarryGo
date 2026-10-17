package walletclients

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type PaystackClient interface {
	InitializeTransaction(context.Context, InitializeTransactionRequest) (InitializeTransactionResponse, error)
	VerifyTransaction(context.Context, string) (VerifyTransactionResponse, error)
	ResolveBankAccount(context.Context, ResolveBankAccountRequest) (ResolveBankAccountResponse, error)
	CreateTransferRecipient(context.Context, CreateTransferRecipientRequest) (CreateTransferRecipientResponse, error)
	InitiateTransfer(context.Context, InitiateTransferRequest) (InitiateTransferResponse, error)
	FinalizeTransfer(context.Context, FinalizeTransferRequest) error
	CreateRefund(context.Context, CreateRefundRequest) (CreateRefundResponse, error)
	CheckBalance(context.Context) (BalanceResponse, error)
	VerifyWebhookSignature(rawBody []byte, signature string) bool
}

type HTTPPaystackClient struct {
	BaseURL    string
	SecretKey  string
	HTTPClient *http.Client
}

type InitializeTransactionRequest struct {
	Email       string                 `json:"email"`
	AmountKobo  int64                  `json:"amount"`
	Reference   string                 `json:"reference"`
	CallbackURL string                 `json:"callback_url,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type InitializeTransactionResponse struct {
	AuthorizationURL string `json:"authorization_url"`
	AccessCode       string `json:"access_code"`
	Reference        string `json:"reference"`
}

type VerifyTransactionResponse struct {
	Status     string `json:"status"`
	Reference  string `json:"reference"`
	AmountKobo int64  `json:"amount"`
	Currency   string `json:"currency"`
	PaidAt     string `json:"paid_at"`
	Customer   struct {
		Email string `json:"email"`
	} `json:"customer"`
}

type ResolveBankAccountRequest struct {
	AccountNumber string
	BankCode      string
}

type ResolveBankAccountResponse struct {
	AccountNumber string `json:"account_number"`
	AccountName   string `json:"account_name"`
	BankID        int64  `json:"bank_id"`
}

type CreateTransferRecipientRequest struct {
	Name          string `json:"name"`
	AccountNumber string `json:"account_number"`
	BankCode      string `json:"bank_code"`
	Currency      string `json:"currency"`
}

type CreateTransferRecipientResponse struct {
	RecipientCode string `json:"recipient_code"`
	Active        bool   `json:"active"`
}

type InitiateTransferRequest struct {
	AmountKobo    int64  `json:"amount"`
	RecipientCode string `json:"recipient"`
	Reason        string `json:"reason,omitempty"`
	Reference     string `json:"reference"`
}

type InitiateTransferResponse struct {
	ID           int64  `json:"id"`
	Reference    string `json:"reference"`
	TransferCode string `json:"transfer_code"`
	Status       string `json:"status"`
}

type FinalizeTransferRequest struct {
	TransferCode string `json:"transfer_code"`
	OTP          string `json:"otp"`
}

type CreateRefundRequest struct {
	TransactionReference string `json:"transaction"`
	AmountKobo           int64  `json:"amount"`
	Currency             string `json:"currency,omitempty"`
	CustomerNote         string `json:"customer_note,omitempty"`
	MerchantNote         string `json:"merchant_note,omitempty"`
}

type CreateRefundResponse struct {
	ID        int64  `json:"id"`
	Reference string `json:"reference"`
	Status    string `json:"status"`
}

type TransferStatusResponse struct {
	Reference string `json:"reference"`
	Status    string `json:"status"`
}

type RefundStatusResponse struct {
	Reference string `json:"reference"`
	Status    string `json:"status"`
}

type BalanceResponse struct {
	Items []BalanceItem `json:"items"`
}

type BalanceItem struct {
	Currency string `json:"currency"`
	Balance  int64  `json:"balance"`
}

type WebhookEvent struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

type WebhookData struct {
	ID           interface{} `json:"id"`
	Reference    string      `json:"reference"`
	Status       string      `json:"status"`
	AmountKobo   int64       `json:"amount"`
	Currency     string      `json:"currency"`
	TransferCode string      `json:"transfer_code"`
	Reason       string      `json:"reason"`
}

func (c HTTPPaystackClient) InitializeTransaction(ctx context.Context, request InitializeTransactionRequest) (InitializeTransactionResponse, error) {
	var response InitializeTransactionResponse
	return response, c.post(ctx, "/transaction/initialize", request, &response)
}

func (c HTTPPaystackClient) VerifyTransaction(ctx context.Context, reference string) (VerifyTransactionResponse, error) {
	var response VerifyTransactionResponse
	return response, c.get(ctx, "/transaction/verify/"+reference, &response)
}

func (c HTTPPaystackClient) ResolveBankAccount(ctx context.Context, request ResolveBankAccountRequest) (ResolveBankAccountResponse, error) {
	var response ResolveBankAccountResponse
	path := fmt.Sprintf("/bank/resolve?account_number=%s&bank_code=%s", request.AccountNumber, request.BankCode)
	return response, c.get(ctx, path, &response)
}

func (c HTTPPaystackClient) CreateTransferRecipient(ctx context.Context, request CreateTransferRecipientRequest) (CreateTransferRecipientResponse, error) {
	payload := map[string]interface{}{
		"type":           "nuban",
		"name":           request.Name,
		"account_number": request.AccountNumber,
		"bank_code":      request.BankCode,
		"currency":       request.Currency,
	}
	var response CreateTransferRecipientResponse
	return response, c.post(ctx, "/transferrecipient", payload, &response)
}

func (c HTTPPaystackClient) InitiateTransfer(ctx context.Context, request InitiateTransferRequest) (InitiateTransferResponse, error) {
	payload := map[string]interface{}{
		"source":    "balance",
		"amount":    request.AmountKobo,
		"recipient": request.RecipientCode,
		"reason":    request.Reason,
		"reference": request.Reference,
	}
	var response InitiateTransferResponse
	return response, c.post(ctx, "/transfer", payload, &response)
}

func (c HTTPPaystackClient) FinalizeTransfer(ctx context.Context, request FinalizeTransferRequest) error {
	return c.post(ctx, "/transfer/finalize_transfer", request, &struct{}{})
}

func (c HTTPPaystackClient) CreateRefund(ctx context.Context, request CreateRefundRequest) (CreateRefundResponse, error) {
	var response CreateRefundResponse
	return response, c.post(ctx, "/refund", request, &response)
}

// VerifyTransfer reads an existing transfer by the idempotent reference. It is
// deliberately separate from PaystackClient so existing consumer-wallet fakes
// are not forced to implement the WP7 settlement surface.
func (c HTTPPaystackClient) VerifyTransfer(ctx context.Context, reference string) (TransferStatusResponse, error) {
	var response TransferStatusResponse
	return response, c.get(ctx, "/transfer/verify/"+reference, &response)
}

// VerifyRefund reads an existing refund after an ambiguous create response.
func (c HTTPPaystackClient) VerifyRefund(ctx context.Context, reference string) (RefundStatusResponse, error) {
	var response RefundStatusResponse
	return response, c.get(ctx, "/refund/"+reference, &response)
}

func (c HTTPPaystackClient) CheckBalance(ctx context.Context) (BalanceResponse, error) {
	var response BalanceResponse
	return response, c.get(ctx, "/balance", &response.Items)
}

func (c HTTPPaystackClient) VerifyWebhookSignature(rawBody []byte, signature string) bool {
	if c.SecretKey == "" || signature == "" {
		return false
	}
	mac := hmac.New(sha512.New, []byte(c.SecretKey))
	_, _ = mac.Write(rawBody)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func ParseWebhook(rawBody []byte) (WebhookEvent, WebhookData, error) {
	var event WebhookEvent
	if err := json.Unmarshal(rawBody, &event); err != nil {
		return WebhookEvent{}, WebhookData{}, err
	}
	var data WebhookData
	if len(event.Data) > 0 {
		if err := json.Unmarshal(event.Data, &data); err != nil {
			return WebhookEvent{}, WebhookData{}, err
		}
	}
	return event, data, nil
}

func WebhookEventKey(event WebhookEvent, data WebhookData) string {
	if data.ID != nil {
		return fmt.Sprintf("%s:%v", event.Event, data.ID)
	}
	if data.Reference != "" {
		return event.Event + ":" + data.Reference + ":" + data.Status
	}
	if data.TransferCode != "" {
		return event.Event + ":" + data.TransferCode + ":" + data.Status
	}
	return event.Event
}

func (c HTTPPaystackClient) get(ctx context.Context, path string, data interface{}) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(path), nil)
	if err != nil {
		return err
	}
	return c.do(request, data)
}

func (c HTTPPaystackClient) post(ctx context.Context, path string, payload interface{}, data interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(path), bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	return c.do(request, data)
}

func (c HTTPPaystackClient) do(request *http.Request, data interface{}) error {
	if c.SecretKey == "" {
		return fmt.Errorf("paystack secret key is required")
	}
	request.Header.Set("Authorization", "Bearer "+c.SecretKey)
	request.Header.Set("Accept", "application/json")

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	var envelope struct {
		Status  bool            `json:"status"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &envelope); err != nil {
			return err
		}
	}
	if response.StatusCode >= http.StatusBadRequest || !envelope.Status {
		if envelope.Message == "" {
			envelope.Message = "Paystack request failed."
		}
		return &PaystackError{StatusCode: response.StatusCode, Message: envelope.Message}
	}
	if data != nil && len(envelope.Data) > 0 {
		return json.Unmarshal(envelope.Data, data)
	}
	return nil
}

func (c HTTPPaystackClient) url(path string) string {
	baseURL := strings.TrimRight(c.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.paystack.co"
	}
	return baseURL + path
}

// PaystackError is a definite answer from Paystack (not a transport failure).
// Its text is Paystack's own message, unchanged.
type PaystackError struct {
	StatusCode int
	Message    string
}

func (e *PaystackError) Error() string { return e.Message }

// IsPaystackRejection reports whether Paystack definitively refused the
// request's input (for example an account number it cannot resolve), as
// opposed to being unreachable or failing internally.
func IsPaystackRejection(err error) bool {
	var pe *PaystackError
	if !errors.As(err, &pe) {
		return false
	}
	return pe.StatusCode >= 400 && pe.StatusCode < 500 && pe.StatusCode != 401 && pe.StatusCode != 403 && pe.StatusCode != 429
}
