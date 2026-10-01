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

// FinancialVisibilityPolicy is Fleet's typed corporate provider financial
// visibility contract. Provider services use it to project responses
// server-side instead of returning sensitive organization/platform values and
// relying on the app to hide them.
type FinancialVisibilityPolicy struct {
	ShowCustomerFare   bool `json:"showCustomerFare"`
	ShowOwnEarning     bool `json:"showOwnEarning"`
	ShowCommissionRate bool `json:"showCommissionRate"`
	ShowProviderShare  bool `json:"showProviderShare"`
	ShowPayroll        bool `json:"showPayroll"`
	ShowTips           bool `json:"showTips"`
	ShowAllowances     bool `json:"showAllowances"`
	ShowBonuses        bool `json:"showBonuses"`
}

// CorporateContext is the canonical Section 7 context returned by Fleet for
// Taxi, Dispatch and Hauling provider-session and managed-work decisions.
type CorporateContext struct {
	ProviderID                string                    `json:"providerId"`
	IsManagedProvider         bool                      `json:"isManagedProvider"`
	MembershipID              string                    `json:"membershipId,omitempty"`
	OrganizationID            string                    `json:"organizationId,omitempty"`
	MembershipStatus          string                    `json:"membershipStatus,omitempty"`
	OrganizationRole          string                    `json:"organizationRole,omitempty"`
	OperatingMode             string                    `json:"operatingMode,omitempty"`
	ServiceEligibility        map[string]bool           `json:"serviceEligibility"`
	CompensationContext       map[string]any            `json:"compensationContext,omitempty"`
	FinancialVisibilityPolicy FinancialVisibilityPolicy `json:"financialVisibilityPolicy"`
	Version                   int64                     `json:"version"`
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

// CorporateContextReader is implemented by clients that understand the
// canonical Fleet corporate-context endpoint.
type CorporateContextReader interface {
	ReadCorporateContext(ctx context.Context, service, providerID string) (CorporateContext, error)
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
		url.PathEscape(providerID) + "/services/" + url.PathEscape(providerService) + "/membership-summary"
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

func (c *Client) ReadCorporateContext(ctx context.Context, service, providerID string) (CorporateContext, error) {
	if c == nil || strings.TrimSpace(c.BaseURL) == "" || strings.TrimSpace(c.ServiceName) == "" || len(c.Secret) == 0 {
		return CorporateContext{}, fmt.Errorf("provider corporate-context client is not configured")
	}
	endpoint := c.BaseURL + "/internal/fleet-partner/v1/providers/" +
		url.PathEscape(providerID) + "/corporate-context?service=" + url.QueryEscape(service)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return CorporateContext{}, fmt.Errorf("build corporate-context request: %w", err)
	}
	if err := serviceauth.SignRequest(req, c.ServiceName, c.Secret, nil, time.Now()); err != nil {
		return CorporateContext{}, fmt.Errorf("sign corporate-context request: %w", err)
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return CorporateContext{}, fmt.Errorf("read corporate context: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return CorporateContext{}, fmt.Errorf("read corporate-context response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return CorporateContext{}, fmt.Errorf("corporate-context response status %d", resp.StatusCode)
	}
	var envelope struct {
		Data CorporateContext `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Data.ProviderID != "" {
		return normalizeCorporateContext(envelope.Data), nil
	}
	var direct CorporateContext
	if err := json.Unmarshal(body, &direct); err != nil {
		return CorporateContext{}, fmt.Errorf("decode corporate-context response: %w", err)
	}
	if direct.ProviderID == "" {
		return CorporateContext{}, fmt.Errorf("corporate-context response is incomplete")
	}
	return normalizeCorporateContext(direct), nil
}

func normalizeCorporateContext(ctx CorporateContext) CorporateContext {
	if ctx.ServiceEligibility == nil {
		ctx.ServiceEligibility = map[string]bool{}
	}
	if !ctx.IsManagedProvider {
		ctx.MembershipID = ""
		ctx.OrganizationID = ""
		ctx.MembershipStatus = ""
		ctx.OrganizationRole = ""
		ctx.OperatingMode = ""
		ctx.CompensationContext = nil
	}
	return ctx
}
