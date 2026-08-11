package verification

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

type CreateInput struct {
	ClientReference string   `json:"client_reference"`
	BusinessID      string   `json:"business_id"`
	Dimension       string   `json:"dimension"`
	SubjectType     string   `json:"subject_type"`
	SubjectRef      string   `json:"subject_ref"`
	DocumentRefs    []string `json:"document_refs"`
}
type Result struct {
	SourceRef     string     `json:"source_ref"`
	SourceVersion int64      `json:"source_version"`
	Status        string     `json:"status"`
	ObservedAt    time.Time  `json:"observed_at"`
	ExpiresAt     *time.Time `json:"expires_at"`
	ReasonCode    string     `json:"reason_code"`
	Remediation   string     `json:"remediation"`
}
type envelope struct {
	Success bool   `json:"success"`
	Data    Result `json:"data"`
}

func (c *Client) Create(ctx context.Context, input CreateInput) (Result, error) {
	body, _ := json.Marshal(input)
	return c.call(ctx, http.MethodPost, "/api/v1/verification-compliance/internal/vendor-cases/cases", body, input.ClientReference)
}
func (c *Client) GetByClientReference(ctx context.Context, reference string) (Result, error) {
	return c.call(ctx, http.MethodGet, "/api/v1/verification-compliance/internal/vendor-cases/cases/by-client-reference/"+url.PathEscape(reference), nil, "")
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
		return Result{}, fmt.Errorf("verification authority returned status %d", response.StatusCode)
	}
	var out envelope
	if err = json.Unmarshal(raw, &out); err != nil {
		return Result{}, err
	}
	if out.Data.SourceRef == "" || out.Data.SourceVersion < 1 || out.Data.ObservedAt.IsZero() {
		return Result{}, fmt.Errorf("verification authority response is missing source metadata")
	}
	return out.Data, nil
}
