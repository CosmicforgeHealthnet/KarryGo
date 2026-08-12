// Package providercompat defines the additive provider-session extension used
// by the provider services to expose organization-managed work without
// changing the legacy independent-provider contract.
package providercompat

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

const (
	ModeIndependent         = "INDEPENDENT"
	ModeOrganizationManaged = "ORGANIZATION_MANAGED"
	ContractVersion         = "1"
)

// Summary is the tenant-safe response returned by fleet-partner-service.
// The JSON names deliberately match the provider login/session extension.
type Summary struct {
	ProviderMode          string            `json:"providerMode"`
	Organization          *Organization     `json:"organization,omitempty"`
	Membership            *Membership       `json:"membership,omitempty"`
	AssignmentPolicy      *AssignmentPolicy `json:"assignmentPolicy,omitempty"`
	FieldVisibilityPolicy string            `json:"fieldVisibilityPolicy,omitempty"`
}

type Organization struct {
	ID          string  `json:"id"`
	DisplayName string  `json:"displayName"`
	Status      string  `json:"status"`
	Branch      *Branch `json:"branch,omitempty"`
}

type Branch struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type Membership struct {
	Status          string `json:"status"`
	Role            string `json:"role"`
	ServiceCategory string `json:"serviceCategory"`
}

type AssignmentPolicy struct {
	AssignmentMode          string `json:"assignmentMode"`
	ProviderDeclineRequired bool   `json:"providerDeclineRequired"`
}

// Reader is injected into provider authentication so the legacy contract is
// byte-for-byte unchanged when organization mode is disabled.
type Reader interface {
	ReadMembershipSummary(ctx context.Context, providerService, providerID string) (Summary, error)
}

// Client is an HMAC-authenticated fleet-partner-service reader.
type Client struct {
	BaseURL     string
	ServiceName string
	Secret      []byte
	HTTPClient  *http.Client
}

func NewClient(baseURL, serviceName string, secret []byte, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"), ServiceName: serviceName,
		Secret: append([]byte(nil), secret...), HTTPClient: &http.Client{Timeout: timeout},
	}
}

func (c *Client) ReadMembershipSummary(ctx context.Context, providerService, providerID string) (Summary, error) {
	if c == nil || strings.TrimSpace(c.BaseURL) == "" || strings.TrimSpace(c.ServiceName) == "" || len(c.Secret) == 0 {
		return Summary{}, fmt.Errorf("provider compatibility client is not configured")
	}
	endpoint := c.BaseURL + "/internal/fleet-partner/v1/providers/" +
		url.PathEscape(providerService) + "/" + url.PathEscape(providerID) + "/membership-summary"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Summary{}, fmt.Errorf("build membership-summary request: %w", err)
	}
	if err := serviceauth.SignRequest(req, c.ServiceName, c.Secret, nil, time.Now()); err != nil {
		return Summary{}, fmt.Errorf("sign membership-summary request: %w", err)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return Summary{}, fmt.Errorf("read membership summary: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Summary{}, fmt.Errorf("read membership-summary response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Summary{}, fmt.Errorf("membership-summary response status %d", resp.StatusCode)
	}
	var summary Summary
	if err := json.Unmarshal(body, &summary); err != nil {
		return Summary{}, fmt.Errorf("decode membership-summary response: %w", err)
	}
	if summary.ProviderMode != ModeIndependent && summary.ProviderMode != ModeOrganizationManaged {
		return Summary{}, fmt.Errorf("membership-summary returned unsupported providerMode %q", summary.ProviderMode)
	}
	if summary.ProviderMode == ModeOrganizationManaged && (summary.Organization == nil || summary.Membership == nil || summary.AssignmentPolicy == nil) {
		return Summary{}, fmt.Errorf("organization-managed membership-summary is incomplete")
	}
	if summary.ProviderMode == ModeIndependent {
		// Defensive normalization prevents a malformed downstream response from
		// leaking organization-only data into an independent login contract.
		summary.Organization = nil
		summary.Membership = nil
		summary.AssignmentPolicy = nil
		summary.FieldVisibilityPolicy = ""
	}
	return summary, nil
}
