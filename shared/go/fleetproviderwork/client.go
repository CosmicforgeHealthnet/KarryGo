package fleetproviderwork

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// Headers relayed to Fleet on every call.
const (
	HeaderIdempotencyKey = "Idempotency-Key"
	HeaderCorrelationID  = "X-Correlation-ID"
	HeaderRequestID      = "X-Request-ID"
)

const maxResponseBytes = 1 << 20

// Config builds a client for one execution service.
type Config struct {
	BaseURL string
	// ServiceName is this execution service's HMAC identity, which is also
	// what Fleet uses to decide which provider service the caller may speak
	// for (driver-taxi-service -> TAXI, driver-hauling-service -> HAULING).
	ServiceName string
	Secret      []byte
	// ProviderService is the service category asserted on provider decisions.
	ProviderService string
	Timeout         time.Duration
	HTTP            *http.Client
}

// Client reads and relays provider-facing corporate work.
type Client struct {
	baseURL         string
	serviceName     string
	providerService string
	secret          []byte
	http            *http.Client
}

// NewClient builds the client, reporting whether it is usable. An unusable
// client is never returned as usable with a nil secret: the facade refuses
// rather than signing nothing.
func NewClient(cfg Config) (*Client, bool) {
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.ServiceName) == "" || len(cfg.Secret) == 0 {
		return nil, false
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	httpClient := cfg.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	return &Client{
		baseURL:         strings.TrimRight(cfg.BaseURL, "/"),
		serviceName:     cfg.ServiceName,
		providerService: strings.TrimSpace(cfg.ProviderService),
		secret:          append([]byte(nil), cfg.Secret...),
		http:            httpClient,
	}, true
}

// OfferSummary is the provider-safe marketplace offer projection.
//
// It is an explicit allowlist, not a filtered copy of Fleet's own model: a
// stripping filter fails open the moment Fleet's model gains a commercial
// field, which is exactly how organization margin leaks to a driver.
type OfferSummary struct {
	OfferID        string   `json:"offerId"`
	RequestID      string   `json:"requestId"`
	ServiceType    string   `json:"serviceType"`
	Status         string   `json:"status,omitempty"`
	ExpiresAt      string   `json:"expiresAt"`
	AllowedActions []string `json:"allowedActions"`
}

// AssignmentSummary is the provider-safe company-assignment projection.
type AssignmentSummary struct {
	AssignmentID     string   `json:"assignmentId"`
	RequestID        string   `json:"requestId"`
	Status           string   `json:"status"`
	ResponseDeadline string   `json:"responseDeadline"`
	AllowedActions   []string `json:"allowedActions"`
	SourceService    string   `json:"sourceService,omitempty"`
	Version          int64    `json:"version,omitempty"`
}

// Decision is a provider's accept/decline.
type Decision struct {
	ClientResponseID string   `json:"clientResponseId"`
	ExpectedVersion  int64    `json:"expectedVersion,omitempty"`
	ReasonCode       string   `json:"reasonCode,omitempty"`
	Note             string   `json:"note,omitempty"`
	EvidenceFileIDs  []string `json:"evidenceFileIds,omitempty"`
}

type decisionPayload struct {
	ProviderID        string   `json:"providerId"`
	ProviderService   string   `json:"providerService"`
	ProviderSessionID string   `json:"providerSessionId"`
	ClientResponseID  string   `json:"clientResponseId"`
	ExpectedVersion   int64    `json:"expectedVersion,omitempty"`
	ReasonCode        string   `json:"reasonCode,omitempty"`
	Note              string   `json:"note,omitempty"`
	EvidenceFileIDs   []string `json:"evidenceFileIds,omitempty"`
}

// Meta carries the tracing and idempotency identity of one relayed call.
type Meta struct {
	RequestID      string
	CorrelationID  string
	IdempotencyKey string
}

// ListQuery is the optional filter/pagination a provider list read supports.
//
// Fleet decides which of these it honours; an unsupported filter is simply
// not reflected in the answer rather than erroring, so a provider app can ask
// consistently across services.
type ListQuery struct {
	Status string
	Limit  int
	Cursor string
}

func (q ListQuery) encode() string {
	values := url.Values{}
	if status := strings.TrimSpace(q.Status); status != "" {
		values.Set("status", status)
	}
	if q.Limit > 0 {
		values.Set("limit", strconv.Itoa(q.Limit))
	}
	if cursor := strings.TrimSpace(q.Cursor); cursor != "" {
		values.Set("cursor", cursor)
	}
	if len(values) == 0 {
		return ""
	}
	return "?" + values.Encode()
}

// Page is one page of provider-facing corporate work.
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// ListOffers reads the provider's organization marketplace offers.
func (c *Client) ListOffers(ctx context.Context, meta Meta, providerID string, query ListQuery) (Page[OfferSummary], error) {
	var page struct {
		Data struct {
			Items []OfferSummary `json:"items"`
		} `json:"data"`
		Meta struct {
			NextCursor string `json:"nextCursor"`
		} `json:"meta"`
	}
	path := fmt.Sprintf("/internal/fleet-partner/v1/providers/%s/offers%s", url.PathEscape(providerID), query.encode())
	if err := c.get(ctx, meta, path, &page); err != nil {
		return Page[OfferSummary]{}, err
	}
	return Page[OfferSummary]{Items: nonNil(page.Data.Items), NextCursor: page.Meta.NextCursor}, nil
}

// GetOffer reads one offer.
func (c *Client) GetOffer(ctx context.Context, meta Meta, providerID, offerID string) (OfferSummary, error) {
	var one struct {
		Data OfferSummary `json:"data"`
	}
	path := fmt.Sprintf("/internal/fleet-partner/v1/providers/%s/offers/%s",
		url.PathEscape(providerID), url.PathEscape(offerID))
	if err := c.get(ctx, meta, path, &one); err != nil {
		return OfferSummary{}, err
	}
	return one.Data, nil
}

// ListAssignments reads the provider's company assignments.
func (c *Client) ListAssignments(ctx context.Context, meta Meta, providerID string, query ListQuery) (Page[AssignmentSummary], error) {
	var page struct {
		Data struct {
			Items []AssignmentSummary `json:"items"`
		} `json:"data"`
		Meta struct {
			NextCursor string `json:"nextCursor"`
		} `json:"meta"`
	}
	path := fmt.Sprintf("/internal/fleet-partner/v1/providers/%s/assignments%s", url.PathEscape(providerID), query.encode())
	if err := c.get(ctx, meta, path, &page); err != nil {
		return Page[AssignmentSummary]{}, err
	}
	return Page[AssignmentSummary]{Items: nonNil(page.Data.Items), NextCursor: page.Meta.NextCursor}, nil
}

// GetAssignment reads one company assignment.
func (c *Client) GetAssignment(ctx context.Context, meta Meta, providerID, assignmentID string) (AssignmentSummary, error) {
	var one struct {
		Data AssignmentSummary `json:"data"`
	}
	path := fmt.Sprintf("/internal/fleet-partner/v1/providers/%s/assignments/%s",
		url.PathEscape(providerID), url.PathEscape(assignmentID))
	if err := c.get(ctx, meta, path, &one); err != nil {
		return AssignmentSummary{}, err
	}
	return one.Data, nil
}

// AcceptOffer relays a provider's offer acceptance.
func (c *Client) AcceptOffer(ctx context.Context, meta Meta, providerID, sessionID, offerID string, decision Decision) (json.RawMessage, error) {
	return c.decision(ctx, meta, "/internal/fleet-partner/v1/marketplace-offers/"+url.PathEscape(offerID)+"/provider-accept", providerID, sessionID, decision)
}

// DeclineOffer relays a provider's offer decline.
func (c *Client) DeclineOffer(ctx context.Context, meta Meta, providerID, sessionID, offerID string, decision Decision) (json.RawMessage, error) {
	return c.decision(ctx, meta, "/internal/fleet-partner/v1/marketplace-offers/"+url.PathEscape(offerID)+"/provider-decline", providerID, sessionID, decision)
}

// AcceptAssignment relays a provider's assignment acceptance.
func (c *Client) AcceptAssignment(ctx context.Context, meta Meta, providerID, sessionID, assignmentID string, decision Decision) (json.RawMessage, error) {
	return c.decision(ctx, meta, "/internal/fleet-partner/v1/assignments/"+url.PathEscape(assignmentID)+"/provider-accept", providerID, sessionID, decision)
}

// DeclineAssignment relays a provider's assignment decline.
func (c *Client) DeclineAssignment(ctx context.Context, meta Meta, providerID, sessionID, assignmentID string, decision Decision) (json.RawMessage, error) {
	return c.decision(ctx, meta, "/internal/fleet-partner/v1/assignments/"+url.PathEscape(assignmentID)+"/provider-decline", providerID, sessionID, decision)
}

// get performs a signed read and classifies every failure. A transport or 5xx
// failure is a retryable dependency error; an authoritative 4xx is a definite
// rejection. Neither is ever reported as "provider is independent".
func (c *Client) get(ctx context.Context, meta Meta, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return DependencyError(err)
	}
	if err := c.decorate(req, meta, nil); err != nil {
		return DependencyError(err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return DependencyError(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return DependencyError(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ClassifyResponse(resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return DependencyError(err)
	}
	return nil
}

func (c *Client) decision(ctx context.Context, meta Meta, path, providerID, sessionID string, decision Decision) (json.RawMessage, error) {
	payload := decisionPayload{
		ProviderID: providerID, ProviderService: c.providerService, ProviderSessionID: sessionID,
		ClientResponseID: decision.ClientResponseID, ExpectedVersion: decision.ExpectedVersion,
		ReasonCode: decision.ReasonCode, Note: decision.Note, EvidenceFileIDs: decision.EvidenceFileIDs,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, DependencyError(err)
	}
	if err := c.decorate(req, meta, body); err != nil {
		return nil, DependencyError(err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, DependencyError(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, DependencyError(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, ClassifyResponse(resp.StatusCode, raw)
	}
	return raw, nil
}

func (c *Client) decorate(req *http.Request, meta Meta, body []byte) error {
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if meta.RequestID != "" {
		req.Header.Set(HeaderRequestID, meta.RequestID)
	}
	if meta.CorrelationID != "" {
		req.Header.Set(HeaderCorrelationID, meta.CorrelationID)
	}
	if meta.IdempotencyKey != "" {
		req.Header.Set(HeaderIdempotencyKey, meta.IdempotencyKey)
	}
	return serviceauth.SignRequest(req, c.serviceName, c.secret, body, time.Now())
}

func nonNil[T any](in []T) []T {
	if in == nil {
		return []T{}
	}
	return in
}
