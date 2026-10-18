package infra

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/billing"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// chargeAuthorizationPath is the payment-wallet internal charge endpoint.
// shared/go/walletclient does not expose this call yet, so the HTTP call is
// implemented here with the same HMAC signing + envelope conventions.
const chargeAuthorizationPath = "/api/v1/payment-wallet/internal/charge-authorization"

// WalletChargeClient implements billing.ChargeClient against
// payment-wallet-service using serviceauth HMAC request signing. api-gateway
// never holds Paystack secret keys.
type WalletChargeClient struct {
	baseURL     string
	serviceName string
	secret      []byte
	httpClient  *http.Client
}

// NewWalletChargeClient constructs a WalletChargeClient. baseURL is a bare
// origin (e.g. http://localhost:8105), matching the shared walletclient.
func NewWalletChargeClient(baseURL, serviceName string, secret []byte) *WalletChargeClient {
	return &WalletChargeClient{
		baseURL:     baseURL,
		serviceName: serviceName,
		secret:      secret,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *WalletChargeClient) ChargeAuthorization(ctx context.Context, in billing.ChargeAuthorizationInput) (billing.ChargeAuthorizationResult, error) {
	body, err := json.Marshal(in)
	if err != nil {
		return billing.ChargeAuthorizationResult{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(chargeAuthorizationPath), bytes.NewReader(body))
	if err != nil {
		return billing.ChargeAuthorizationResult{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if err := serviceauth.SignRequest(request, c.serviceName, c.secret, body, time.Now()); err != nil {
		return billing.ChargeAuthorizationResult{}, err
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return billing.ChargeAuthorizationResult{}, err
	}
	defer response.Body.Close()

	var envelope struct {
		Success bool                              `json:"success"`
		Data    billing.ChargeAuthorizationResult `json:"data"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return billing.ChargeAuthorizationResult{}, err
	}
	if response.StatusCode >= http.StatusBadRequest || !envelope.Success {
		if envelope.Error.Message == "" {
			envelope.Error.Message = "Wallet charge request failed."
		}
		return billing.ChargeAuthorizationResult{}, errors.New(envelope.Error.Message)
	}
	return envelope.Data, nil
}

// url mirrors shared/go/walletclient's tolerance for a baseURL that already
// includes the payment-wallet api base path.
func (c *WalletChargeClient) url(path string) string {
	baseURL := strings.TrimRight(c.baseURL, "/")
	const apiBasePath = "/api/v1/payment-wallet"
	if strings.HasSuffix(baseURL, apiBasePath) && strings.HasPrefix(path, apiBasePath) {
		path = strings.TrimPrefix(path, apiBasePath)
	}
	return baseURL + path
}
