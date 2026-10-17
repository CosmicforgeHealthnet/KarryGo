package wallet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

type Client struct {
	baseURL string
	secret  []byte
	http    *http.Client
}

func NewClient(baseURL, secret string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), secret: []byte(secret), http: &http.Client{Timeout: timeout}}
}

type Result struct {
	ExternalRef         string    `json:"external_ref"`
	BankCode            string    `json:"bank_code"`
	MaskedAccount       string    `json:"masked_account"`
	ResolvedAccountName string    `json:"resolved_account_name"`
	SourceVersion       int64     `json:"source_version"`
	Status              string    `json:"status"`
	ObservedAt          time.Time `json:"observed_at"`
}
// AuthorityError is a definitive HTTP answer from the wallet authority. A 4xx
// status means the request was refused (nothing was registered), so callers
// may surface the message instead of treating the outcome as unknown.
type AuthorityError struct {
	Status  int
	Code    string
	Message string
}

func (e *AuthorityError) Error() string {
	return fmt.Sprintf("wallet authority returned status %d (%s)", e.Status, e.Code)
}

// Refused reports whether the authority definitively rejected the request.
func (e *AuthorityError) Refused() bool { return e.Status >= 400 && e.Status < 500 }

type envelope struct {
	Success bool   `json:"success"`
	Data    Result `json:"data"`
}

func (c *Client) ResolveAndRegister(ctx context.Context, businessID, accountNumber, bankCode, key string) (Result, error) {
	body, _ := json.Marshal(map[string]string{"account_number": accountNumber, "bank_code": bankCode, "currency": "NGN"})
	return c.call(ctx, http.MethodPut, "/api/v1/payment-wallet/internal/vendor-bank-profiles/"+url.PathEscape(businessID), body, key)
}
func (c *Client) Get(ctx context.Context, businessID string) (Result, error) {
	return c.call(ctx, http.MethodGet, "/api/v1/payment-wallet/internal/vendor-bank-profiles/"+url.PathEscape(businessID), nil, "")
}
func (c *Client) call(ctx context.Context, method, path string, body []byte, key string) (Result, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	if err = serviceauth.SignRequest(req, "vendor-service", c.secret, body, time.Now()); err != nil {
		return Result{}, err
	}
	response, err := c.http.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return Result{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		authErr := &AuthorityError{Status: response.StatusCode}
		var failure struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(raw, &failure) == nil {
			authErr.Code, authErr.Message = failure.Error.Code, failure.Error.Message
		}
		return Result{}, authErr
	}
	var out envelope
	if err = json.Unmarshal(raw, &out); err != nil {
		return Result{}, err
	}
	if out.Data.ExternalRef == "" || out.Data.SourceVersion < 1 || out.Data.ObservedAt.IsZero() {
		return Result{}, fmt.Errorf("wallet authority response is missing source metadata")
	}
	return out.Data, nil
}
