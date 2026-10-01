package logisticscapability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

const maxResponseBytes = 1 << 20

// ErrNotConfigured means this deployment has no credentials for the
// authority, so it cannot be asked and must not be selected.
var ErrNotConfigured = errors.New("logisticscapability: authority client is not configured")

// ErrAuthorityUnavailable means the authority could not be reached or did not
// answer usefully. It is deliberately distinct from a declared
// "not serviceable": an unreachable authority is unknown, not a refusal.
var ErrAuthorityUnavailable = errors.New("logisticscapability: authority unavailable")

// Options configure one authority client.
type Options struct {
	BaseURL string
	// APIBase is the authority's own internal commerce base path, the same one
	// the quote and mission clients already use (for example "/api/v1/hauling").
	APIBase string
	// Caller is the signing identity. Commerce callers sign as vendor-service.
	Caller    string
	Secret    []byte
	Authority Authority
	Timeout   time.Duration
	HTTP      *http.Client
}

// Client reads one authority's declared capability.
type Client struct {
	baseURL   string
	apiBase   string
	caller    string
	secret    []byte
	authority Authority
	http      *http.Client
}

// New builds a capability client. A client missing a URL or secret reports
// Configured() == false and refuses every call rather than guessing.
func New(options Options) *Client {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	httpClient := options.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	caller := strings.TrimSpace(options.Caller)
	if caller == "" {
		caller = "vendor-service"
	}
	return &Client{
		baseURL:   strings.TrimRight(strings.TrimSpace(options.BaseURL), "/"),
		apiBase:   strings.TrimRight(strings.TrimSpace(options.APIBase), "/"),
		caller:    caller,
		secret:    append([]byte(nil), options.Secret...),
		authority: options.Authority,
		http:      httpClient,
	}
}

// Configured reports whether live calls are possible.
func (c *Client) Configured() bool {
	return c != nil && c.baseURL != "" && len(c.secret) > 0
}

// Authority names the authority this client speaks to.
func (c *Client) Authority() Authority {
	if c == nil {
		return ""
	}
	return c.authority
}

type envelope[T any] struct {
	Success bool `json:"success"`
	Data    T    `json:"data"`
	Error   struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Capability reads the authority's declared capability.
func (c *Client) Capability(ctx context.Context) (Capability, error) {
	if !c.Configured() {
		return Capability{}, ErrNotConfigured
	}
	var out envelope[Capability]
	if err := c.do(ctx, http.MethodGet, c.apiBase+Path, nil, &out); err != nil {
		return Capability{}, err
	}
	if out.Data.Authority == "" {
		out.Data.Authority = c.authority
	}
	return out.Data, nil
}

// Serviceability asks the authority whether it can execute one requirement.
func (c *Client) Serviceability(ctx context.Context, requirement Requirement) (Serviceability, error) {
	if !c.Configured() {
		return Serviceability{}, ErrNotConfigured
	}
	body, err := json.Marshal(requirement)
	if err != nil {
		return Serviceability{}, err
	}
	var out envelope[Serviceability]
	if err := c.do(ctx, http.MethodPost, c.apiBase+ServiceabilityPath, body, &out); err != nil {
		return Serviceability{}, err
	}
	if out.Data.Authority == "" {
		out.Data.Authority = c.authority
	}
	return out.Data, nil
}

func (c *Client) do(ctx context.Context, method, path string, body []byte, out any) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAuthorityUnavailable, err)
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if err = serviceauth.SignRequest(request, c.caller, c.secret, body, time.Now()); err != nil {
		return fmt.Errorf("%w: %v", ErrAuthorityUnavailable, err)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAuthorityUnavailable, err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAuthorityUnavailable, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%w: status %d", ErrAuthorityUnavailable, response.StatusCode)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%w: malformed response", ErrAuthorityUnavailable)
	}
	return nil
}

// Set is the collection of authority capability clients a consumer holds.
type Set map[Authority]*Client

// For returns the client for an authority.
func (s Set) For(authority Authority) (*Client, bool) {
	client, ok := s[authority]
	return client, ok && client.Configured()
}

// Available returns the authorities this deployment can actually ask. An
// authority whose integration is switched off is absent, which is what stops
// Vendor selecting an authority it cannot reach (VND-031).
func (s Set) Available() []Authority {
	var out []Authority
	for _, authority := range []Authority{AuthorityDispatch, AuthorityTaxi, AuthorityHauling} {
		if client, ok := s[authority]; ok && client.Configured() {
			out = append(out, authority)
		}
	}
	return out
}
