package fleetprovider

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

	"cosmicforge/logistics/shared/go/serviceauth"
)

const (
	HeaderIdempotencyKey = "Idempotency-Key"
	HeaderCorrelationID  = "X-Correlation-ID"
	HeaderRequestID      = "X-Request-ID"

	SourceServiceTaxi = "TAXI"
)

var ErrFleetUnavailable = errors.New("fleet provider work is unavailable")

type Client struct {
	baseURL     string
	serviceName string
	secret      []byte
	httpClient  *http.Client
}

type Config struct {
	BaseURL     string
	ServiceName string
	Secret      []byte
	Timeout     time.Duration
}

func NewClient(cfg Config) (*Client, bool) {
	if strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.ServiceName) == "" || len(cfg.Secret) == 0 {
		return nil, false
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{
		baseURL:     strings.TrimRight(cfg.BaseURL, "/"),
		serviceName: cfg.ServiceName,
		secret:      append([]byte(nil), cfg.Secret...),
		httpClient:  &http.Client{Timeout: timeout},
	}, true
}

type OfferSummary struct {
	OfferID        string   `json:"offerId"`
	RequestID      string   `json:"requestId"`
	ServiceType    string   `json:"serviceType"`
	Status         string   `json:"status,omitempty"`
	ExpiresAt      string   `json:"expiresAt"`
	AllowedActions []string `json:"allowedActions"`
}

type AssignmentSummary struct {
	AssignmentID     string   `json:"assignmentId"`
	RequestID        string   `json:"requestId"`
	Status           string   `json:"status"`
	ResponseDeadline string   `json:"responseDeadline"`
	AllowedActions   []string `json:"allowedActions"`
}

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

type Meta struct {
	RequestID      string
	CorrelationID  string
	IdempotencyKey string
}

func (c *Client) ListOffers(ctx context.Context, meta Meta, providerID string) ([]OfferSummary, error) {
	var page struct {
		Data struct {
			Items []OfferSummary `json:"items"`
		} `json:"data"`
	}
	path := fmt.Sprintf("/internal/fleet-partner/v1/providers/%s/offers", url.PathEscape(providerID))
	if err := c.get(ctx, meta, path, &page); err != nil {
		return nil, err
	}
	return nonNilOffers(page.Data.Items), nil
}

func (c *Client) GetOffer(ctx context.Context, meta Meta, providerID, offerID string) (OfferSummary, error) {
	var one struct {
		Data OfferSummary `json:"data"`
	}
	path := fmt.Sprintf("/internal/fleet-partner/v1/providers/%s/offers/%s", url.PathEscape(providerID), url.PathEscape(offerID))
	if err := c.get(ctx, meta, path, &one); err != nil {
		return OfferSummary{}, err
	}
	return one.Data, nil
}

func (c *Client) ListAssignments(ctx context.Context, meta Meta, providerID string) ([]AssignmentSummary, error) {
	var page struct {
		Data struct {
			Items []AssignmentSummary `json:"items"`
		} `json:"data"`
	}
	path := fmt.Sprintf("/internal/fleet-partner/v1/providers/%s/assignments", url.PathEscape(providerID))
	if err := c.get(ctx, meta, path, &page); err != nil {
		return nil, err
	}
	return nonNilAssignments(page.Data.Items), nil
}

func (c *Client) GetAssignment(ctx context.Context, meta Meta, providerID, assignmentID string) (AssignmentSummary, error) {
	var one struct {
		Data AssignmentSummary `json:"data"`
	}
	path := fmt.Sprintf("/internal/fleet-partner/v1/providers/%s/assignments/%s", url.PathEscape(providerID), url.PathEscape(assignmentID))
	if err := c.get(ctx, meta, path, &one); err != nil {
		return AssignmentSummary{}, err
	}
	return one.Data, nil
}

func (c *Client) AcceptOffer(ctx context.Context, meta Meta, providerID, sessionID, offerID string, decision Decision) (json.RawMessage, error) {
	return c.decision(ctx, meta, "/internal/fleet-partner/v1/marketplace-offers/"+url.PathEscape(offerID)+"/provider-accept", providerID, sessionID, decision)
}

func (c *Client) DeclineOffer(ctx context.Context, meta Meta, providerID, sessionID, offerID string, decision Decision) (json.RawMessage, error) {
	return c.decision(ctx, meta, "/internal/fleet-partner/v1/marketplace-offers/"+url.PathEscape(offerID)+"/provider-decline", providerID, sessionID, decision)
}

func (c *Client) AcceptAssignment(ctx context.Context, meta Meta, providerID, sessionID, assignmentID string, decision Decision) (json.RawMessage, error) {
	return c.decision(ctx, meta, "/internal/fleet-partner/v1/assignments/"+url.PathEscape(assignmentID)+"/provider-accept", providerID, sessionID, decision)
}

func (c *Client) DeclineAssignment(ctx context.Context, meta Meta, providerID, sessionID, assignmentID string, decision Decision) (json.RawMessage, error) {
	return c.decision(ctx, meta, "/internal/fleet-partner/v1/assignments/"+url.PathEscape(assignmentID)+"/provider-decline", providerID, sessionID, decision)
}

func (c *Client) get(ctx context.Context, meta Meta, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return ErrFleetUnavailable
	}
	if err := c.decorate(req, meta, nil); err != nil {
		return ErrFleetUnavailable
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ErrFleetUnavailable
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ErrFleetUnavailable
	}
	if err := json.Unmarshal(body, out); err != nil {
		return ErrFleetUnavailable
	}
	return nil
}

func (c *Client) decision(ctx context.Context, meta Meta, path, providerID, sessionID string, decision Decision) (json.RawMessage, error) {
	payload := decisionPayload{
		ProviderID: providerID, ProviderService: SourceServiceTaxi, ProviderSessionID: sessionID,
		ClientResponseID: decision.ClientResponseID, ExpectedVersion: decision.ExpectedVersion,
		ReasonCode: decision.ReasonCode, Note: decision.Note, EvidenceFileIDs: decision.EvidenceFileIDs,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, ErrFleetUnavailable
	}
	if err := c.decorate(req, meta, body); err != nil {
		return nil, ErrFleetUnavailable
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, ErrFleetUnavailable
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, ErrFleetUnavailable
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

func nonNilOffers(in []OfferSummary) []OfferSummary {
	if in == nil {
		return []OfferSummary{}
	}
	return in
}

func nonNilAssignments(in []AssignmentSummary) []AssignmentSummary {
	if in == nil {
		return []AssignmentSummary{}
	}
	return in
}
