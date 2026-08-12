package marketplace

import (
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

// HTTPProviderReader is the REAL ProviderReader.
//
// Dispatch does not own organization offers or company assignments, so
// rendering them for a provider is an authenticated cross-service read.
type HTTPProviderReader struct {
	baseURL     string
	serviceName string
	secret      []byte
	client      *http.Client
}

// NewHTTPProviderReader builds the real reader, or reports incomplete
// configuration so the caller keeps the fail-closed fallback.
func NewHTTPProviderReader(config ResponderConfig) (ProviderReader, bool) {
	if strings.TrimSpace(config.BaseURL) == "" ||
		strings.TrimSpace(config.ServiceName) == "" || len(config.Secret) == 0 {
		return UnconfiguredProviderReader{}, false
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &HTTPProviderReader{
		baseURL:     strings.TrimRight(config.BaseURL, "/"),
		serviceName: config.ServiceName,
		secret:      config.Secret,
		client:      &http.Client{Timeout: timeout},
	}, true
}

// Configured reports true.
func (r *HTTPProviderReader) Configured() bool { return true }

// fleetOfferPage is fleet's paged offer response.
type fleetOfferPage struct {
	Data struct {
		Items []struct {
			OfferID        string   `json:"offerId"`
			RequestID      string   `json:"requestId"`
			ServiceType    string   `json:"serviceType"`
			Status         string   `json:"status"`
			ExpiresAt      string   `json:"expiresAt"`
			AllowedActions []string `json:"allowedActions"`
		} `json:"items"`
	} `json:"data"`
}

type fleetOfferOne struct {
	Data struct {
		OfferID        string   `json:"offerId"`
		RequestID      string   `json:"requestId"`
		ServiceType    string   `json:"serviceType"`
		Status         string   `json:"status"`
		ExpiresAt      string   `json:"expiresAt"`
		AllowedActions []string `json:"allowedActions"`
	} `json:"data"`
}

type fleetAssignmentPage struct {
	Data struct {
		Items []struct {
			AssignmentID     string   `json:"assignmentId"`
			RequestID        string   `json:"requestId"`
			Status           string   `json:"status"`
			ResponseDeadline string   `json:"responseDeadline"`
			AllowedActions   []string `json:"allowedActions"`
		} `json:"items"`
	} `json:"data"`
}

type fleetAssignmentOne struct {
	Data struct {
		AssignmentID     string   `json:"assignmentId"`
		RequestID        string   `json:"requestId"`
		Status           string   `json:"status"`
		ResponseDeadline string   `json:"responseDeadline"`
		AllowedActions   []string `json:"allowedActions"`
	} `json:"data"`
}

// get performs one signed, bounded GET against fleet.
//
// Any non-2xx or unreadable answer is an error, never an empty success: a
// provider app rendering an empty list would tell the provider they have no
// work, which is indistinguishable from genuinely having none.
func (r *HTTPProviderReader) get(ctx context.Context, meta CallMeta, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL+path, nil)
	if err != nil {
		return ErrFleetUnavailable
	}
	req.Header.Set("Accept", "application/json")
	if meta.RequestID != "" {
		req.Header.Set(HeaderRequestID, meta.RequestID)
	}
	if meta.CorrelationID != "" {
		req.Header.Set(HeaderCorrelationID, meta.CorrelationID)
	}
	if err := serviceauth.SignRequest(req, r.serviceName, r.secret, nil, time.Now()); err != nil {
		return ErrFleetUnavailable
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return ErrFleetUnavailable
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFleetResponseBytes))
	if err != nil {
		return ErrFleetUnavailable
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ErrFleetUnavailable
	}
	if err := json.Unmarshal(body, out); err != nil {
		return ErrFleetUnavailable
	}
	return nil
}

// ListOffers returns the provider's organization offers.
func (r *HTTPProviderReader) ListOffers(ctx context.Context, meta CallMeta,
	providerID string) ([]ProviderOfferSummary, error) {
	var page fleetOfferPage
	path := fmt.Sprintf("/internal/fleet-partner/v1/providers/%s/offers",
		url.PathEscape(providerID))
	if err := r.get(ctx, meta, path, &page); err != nil {
		return nil, err
	}
	// Rebuilt field by field from fleet's already-safe view, so a future field
	// added upstream cannot ride along into a provider payload.
	summaries := make([]ProviderOfferSummary, 0, len(page.Data.Items))
	for _, item := range page.Data.Items {
		summaries = append(summaries, ProviderOfferSummary{
			OfferID: item.OfferID, RequestID: item.RequestID,
			ServiceType: item.ServiceType, ExpiresAt: item.ExpiresAt,
			AllowedActions: orEmptyActions(item.AllowedActions),
		})
	}
	return summaries, nil
}

// GetOffer returns one offer belonging to the provider.
func (r *HTTPProviderReader) GetOffer(ctx context.Context, meta CallMeta,
	providerID string, offerID string) (*ProviderOfferSummary, error) {
	var one fleetOfferOne
	path := fmt.Sprintf("/internal/fleet-partner/v1/providers/%s/offers/%s",
		url.PathEscape(providerID), url.PathEscape(offerID))
	if err := r.get(ctx, meta, path, &one); err != nil {
		return nil, err
	}
	if one.Data.OfferID == "" {
		return nil, ErrFleetUnavailable
	}
	return &ProviderOfferSummary{
		OfferID: one.Data.OfferID, RequestID: one.Data.RequestID,
		ServiceType: one.Data.ServiceType, ExpiresAt: one.Data.ExpiresAt,
		AllowedActions: orEmptyActions(one.Data.AllowedActions),
	}, nil
}

// ListAssignments returns the provider's company assignments.
func (r *HTTPProviderReader) ListAssignments(ctx context.Context, meta CallMeta,
	providerID string) ([]ProviderAssignmentSummary, error) {
	var page fleetAssignmentPage
	path := fmt.Sprintf("/internal/fleet-partner/v1/providers/%s/assignments",
		url.PathEscape(providerID))
	if err := r.get(ctx, meta, path, &page); err != nil {
		return nil, err
	}
	summaries := make([]ProviderAssignmentSummary, 0, len(page.Data.Items))
	for _, item := range page.Data.Items {
		summaries = append(summaries, ProviderAssignmentSummary{
			AssignmentID: item.AssignmentID, RequestID: item.RequestID,
			Status: item.Status, ResponseDeadline: item.ResponseDeadline,
			AllowedActions: orEmptyActions(item.AllowedActions),
		})
	}
	return summaries, nil
}

// GetAssignment returns one assignment belonging to the provider.
func (r *HTTPProviderReader) GetAssignment(ctx context.Context, meta CallMeta,
	providerID string, assignmentID string) (*ProviderAssignmentSummary, error) {
	var one fleetAssignmentOne
	path := fmt.Sprintf("/internal/fleet-partner/v1/providers/%s/assignments/%s",
		url.PathEscape(providerID), url.PathEscape(assignmentID))
	if privateAssignmentID(assignmentID) {
		path = fmt.Sprintf("/internal/fleet-partner/v1/private-assignments/%s?providerId=%s",
			url.PathEscape(assignmentID), url.QueryEscape(providerID))
	}
	if err := r.get(ctx, meta, path, &one); err != nil {
		return nil, err
	}
	if one.Data.AssignmentID == "" {
		return nil, ErrFleetUnavailable
	}
	return &ProviderAssignmentSummary{
		AssignmentID: one.Data.AssignmentID, RequestID: one.Data.RequestID,
		Status: one.Data.Status, ResponseDeadline: one.Data.ResponseDeadline,
		AllowedActions: orEmptyActions(one.Data.AllowedActions),
	}, nil
}

// orEmptyActions guarantees allowedActions is always an array, never null.
func orEmptyActions(actions []string) []string {
	if actions == nil {
		return []string{}
	}
	return actions
}
