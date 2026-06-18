package walletclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

const (
	MethodWallet   = "wallet"
	MethodPaystack = "paystack"
)

type Client struct {
	BaseURL     string
	HTTPClient  *http.Client
	ServiceName string
	Secret      []byte
}

type PaymentIntentRequest struct {
	SourceService   string                 `json:"source_service"`
	SourceReference string                 `json:"source_reference"`
	CustomerID      string                 `json:"customer_id"`
	CustomerEmail   string                 `json:"customer_email"`
	ProviderID      string                 `json:"provider_id"`
	ProviderType    string                 `json:"provider_type"`
	AmountKobo      int64                  `json:"amount_kobo"`
	Currency        string                 `json:"currency"`
	PaymentMethod   string                 `json:"payment_method"`
	IdempotencyKey  string                 `json:"idempotency_key"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

type PaymentIntent struct {
	ID               string                 `json:"id"`
	Reference        string                 `json:"reference"`
	SourceService    string                 `json:"source_service"`
	SourceReference  string                 `json:"source_reference"`
	CustomerID       string                 `json:"customer_id"`
	CustomerEmail    string                 `json:"customer_email,omitempty"`
	ProviderID       string                 `json:"provider_id,omitempty"`
	ProviderType     string                 `json:"provider_type,omitempty"`
	AmountKobo       int64                  `json:"amount_kobo"`
	PlatformFeeKobo  int64                  `json:"platform_fee_kobo"`
	Currency         string                 `json:"currency"`
	PaymentMethod    string                 `json:"payment_method"`
	Status           string                 `json:"status"`
	PaystackRef      string                 `json:"paystack_reference,omitempty"`
	AuthorizationURL string                 `json:"authorization_url,omitempty"`
	AccessCode       string                 `json:"access_code,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

type WalletPayRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
}

type RefundRequest struct {
	PaymentReference string `json:"payment_reference"`
	AmountKobo       int64  `json:"amount_kobo"`
	Currency         string `json:"currency"`
	Reason           string `json:"reason"`
	IdempotencyKey   string `json:"idempotency_key"`
}

type Refund struct {
	ID                string `json:"id"`
	Reference         string `json:"reference"`
	PaymentIntentID   string `json:"payment_intent_id"`
	AmountKobo        int64  `json:"amount_kobo"`
	Currency          string `json:"currency"`
	Reason            string `json:"reason,omitempty"`
	Status            string `json:"status"`
	PaystackRefundRef string `json:"paystack_refund_reference,omitempty"`
}

func (c Client) CreatePaymentIntent(ctx context.Context, request PaymentIntentRequest) (PaymentIntent, error) {
	var response PaymentIntent
	err := c.post(ctx, "/api/v1/payment-wallet/internal/payment-intents", request, &response)
	return response, err
}

func (c Client) PayFromWallet(ctx context.Context, paymentIntentID string, idempotencyKey string) (PaymentIntent, error) {
	var response PaymentIntent
	err := c.post(ctx, "/api/v1/payment-wallet/internal/payment-intents/"+paymentIntentID+"/pay-from-wallet", WalletPayRequest{IdempotencyKey: idempotencyKey}, &response)
	return response, err
}

func (c Client) CompleteJob(ctx context.Context, sourceService string, sourceReference string) (PaymentIntent, error) {
	var response PaymentIntent
	path := "/api/v1/payment-wallet/internal/jobs/" + url.PathEscape(sourceService) + "/" + url.PathEscape(sourceReference) + "/complete"
	err := c.post(ctx, path, map[string]interface{}{}, &response)
	return response, err
}

func (c Client) RequestRefund(ctx context.Context, request RefundRequest) (Refund, error) {
	var response Refund
	err := c.post(ctx, "/api/v1/payment-wallet/internal/refunds", request, &response)
	return response, err
}

func (c Client) GetPayment(ctx context.Context, reference string) (PaymentIntent, error) {
	var response PaymentIntent
	err := c.get(ctx, "/api/v1/payment-wallet/internal/payments/"+url.PathEscape(reference), &response)
	return response, err
}

func (c Client) get(ctx context.Context, path string, data interface{}) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(path), nil)
	if err != nil {
		return err
	}
	return c.do(request, nil, data)
}

func (c Client) post(ctx context.Context, path string, payload interface{}, data interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(path), bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	return c.do(request, body, data)
}

func (c Client) do(request *http.Request, body []byte, data interface{}) error {
	if body == nil {
		body = []byte{}
	}
	if err := serviceauth.SignRequest(request, c.ServiceName, c.Secret, body, time.Now()); err != nil {
		return err
	}
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

	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return err
	}
	if response.StatusCode >= http.StatusBadRequest || !envelope.Success {
		if envelope.Error.Message == "" {
			envelope.Error.Message = "Wallet request failed."
		}
		return errors.New(envelope.Error.Message)
	}
	if data != nil && len(envelope.Data) > 0 {
		return json.Unmarshal(envelope.Data, data)
	}
	return nil
}

func (c Client) url(path string) string {
	return strings.TrimRight(c.BaseURL, "/") + path
}
