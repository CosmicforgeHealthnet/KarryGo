package support

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"cosmicforge/logistics/shared/go/serviceauth"
	"cosmicforge/logistics/shared/go/supportcase"
)

const maxResponseBytes = 64 << 10

type responseError struct{ status int }

func (e responseError) Error() string {
	return fmt.Sprintf("support authority returned status %d", e.status)
}

// Client reuses Support/Dispute Service's organization-case authority. Vendor
// Service sends only immutable commerce references and the bounded policy
// reason; Support owns assignment, evidence, adjudication and case lifecycle.
type Client struct {
	baseURL string
	secret  []byte
	http    *http.Client
	now     func() time.Time
}

func NewClient(baseURL, secret string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), secret: []byte(secret), http: &http.Client{Timeout: timeout}, now: time.Now}
}

func (c *Client) CreateRefundCase(ctx context.Context, input orders.RefundSupportRequest) (orders.RefundSupportResult, error) {
	value, err := c.CreateCommerceCase(ctx, supportcase.CreateRequest{
		SchemaVersion: supportcase.SchemaVersion, OperationRef: input.IdempotencyKey,
		Category: supportcase.RefundProblem, Priority: "high",
		Subject: "Commerce refund review required", Description: input.ReasonCode,
		CustomerRef: input.CustomerReference,
		Context: supportcase.Context{CommerceOrderRef: input.ParentOrderReference, VendorOrderRef: input.VendorOrderReference,
			VendorRef: input.OrganizationReference, MissionRef: input.MissionReference, RefundRef: input.IdempotencyKey},
	})
	if err != nil {
		return orders.RefundSupportResult{}, err
	}
	return orders.RefundSupportResult{CaseID: value.CaseRef, Status: value.Status}, nil
}

func (c *Client) CreateCommerceCase(ctx context.Context, input supportcase.CreateRequest) (supportcase.Case, error) {
	if c == nil || c.baseURL == "" || len(c.secret) == 0 {
		return supportcase.Case{}, errors.New("support authority is not configured")
	}
	body, err := json.Marshal(input)
	if err != nil {
		return supportcase.Case{}, err
	}
	path := "/api/v1/support-disputes/internal/commerce-cases"
	result, err := c.doCommerceCase(ctx, http.MethodPost, path, body)
	if err == nil {
		return result, nil
	}
	var statusErr responseError
	if errors.As(err, &statusErr) && statusErr.status < 500 {
		return supportcase.Case{}, err
	}
	// An interrupted/unknown POST may have committed. The operation read-back
	// is authoritative and prevents a blind second create.
	read, readErr := c.GetCommerceCaseByOperation(ctx, input.Context.VendorRef, input.OperationRef)
	if readErr == nil {
		return read, nil
	}
	return supportcase.Case{}, err
}

func (c *Client) GetCommerceCaseByOperation(ctx context.Context, vendorRef, operationRef string) (supportcase.Case, error) {
	path := "/api/v1/support-disputes/internal/commerce-cases/operations/" + url.PathEscape(operationRef) + "?vendor_ref=" + url.QueryEscape(vendorRef)
	return c.doCommerceCase(ctx, http.MethodGet, path, nil)
}

func (c *Client) doCommerceCase(ctx context.Context, method, path string, body []byte) (supportcase.Case, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return supportcase.Case{}, err
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if err = serviceauth.SignRequest(req, "vendor-service", c.secret, body, c.now().UTC()); err != nil {
		return supportcase.Case{}, err
	}
	response, err := c.http.Do(req)
	if err != nil {
		return supportcase.Case{}, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || len(raw) > maxResponseBytes {
		return supportcase.Case{}, errors.New("support authority response is invalid")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return supportcase.Case{}, responseError{status: response.StatusCode}
	}
	var envelope struct {
		Success bool             `json:"success"`
		Data    supportcase.Case `json:"data"`
	}
	if err = json.Unmarshal(raw, &envelope); err != nil || !envelope.Success || envelope.Data.CaseRef == "" {
		return supportcase.Case{}, errors.New("support authority response is invalid")
	}
	return envelope.Data, nil
}

var _ orders.RefundSupportAuthority = (*Client)(nil)
var _ orders.CommerceCaseAuthority = (*Client)(nil)
