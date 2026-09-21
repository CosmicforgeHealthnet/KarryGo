package supplypreview

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

// Client asks one authority the read-only planning question.
//
// The contract already had a server; it had no client, which meant the only
// service that needed to ASK -- Vendor's just-in-time orchestration -- had no
// way to. Without it the alternative is to create a mission and see what
// happens, which is exactly the premature-dispatch problem the preview exists
// to avoid.
type Client struct {
	authority   string
	baseURL     string
	serviceName string
	secret      []byte
	http        *http.Client
}

// Config configures a Client.
type Config struct {
	// Authority names the service being asked, for error messages and for the
	// Set below.
	Authority string

	// BaseURL is the authority's internal commerce base -- the same base its
	// quote, mission and capability paths hang from. Path is appended here, so
	// a base that already carries the path would produce a doubled one.
	BaseURL string

	// ServiceName is the caller's own signing identity.
	ServiceName string

	Secret  []byte
	Timeout time.Duration
}

// New builds a client. A missing base URL or secret yields a client that
// refuses rather than one that silently fails at call time.
func New(config Config) *Client {
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{
		authority:   config.Authority,
		baseURL:     strings.TrimRight(config.BaseURL, "/"),
		serviceName: config.ServiceName,
		secret:      config.Secret,
		http:        &http.Client{Timeout: timeout},
	}
}

// Configured reports whether this client can actually reach its authority.
func (c *Client) Configured() bool {
	return c != nil && c.baseURL != "" && c.serviceName != "" && len(c.secret) > 0
}

// ErrNotConfigured means this deployment cannot reach the authority.
var ErrNotConfigured = errors.New("supplypreview: authority is not configured")

// Preview asks the authority about supply.
//
// A transport failure returns Unknown rather than an error, and that is the
// single most important decision in this file. UNKNOWN is not "none": an
// authority whose availability index blinked is not an authority that cannot
// carry the goods. Returning an error would make every caller write its own
// fallback, and the first one to get it wrong would quietly report a city as
// unserved because one request timed out.
//
// The error return is reserved for a caller mistake -- an unconfigured client
// or an unencodable request -- which is a bug rather than a condition.
func (c *Client) Preview(ctx context.Context, request Request) (Response, error) {
	if !c.Configured() {
		return Unknown(c.authority, time.Now(), 0), ErrNotConfigured
	}
	body, err := json.Marshal(request)
	if err != nil {
		return Unknown(c.authority, time.Now(), 0), err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+Path,
		bytes.NewReader(body))
	if err != nil {
		return Unknown(c.authority, time.Now(), 0), err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	if err = serviceauth.SignRequest(httpRequest, c.serviceName, c.secret, body, time.Now()); err != nil {
		return Unknown(c.authority, time.Now(), 0), err
	}

	response, err := c.http.Do(httpRequest)
	if err != nil {
		return Unknown(c.authority, time.Now(), 0), nil
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return Unknown(c.authority, time.Now(), 0), nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		// A refusing or erroring authority has still told us nothing about
		// supply. Reporting NOT_SERVICEABLE here would turn one 503 into a
		// standing belief that this authority does not operate.
		return Unknown(c.authority, time.Now(), 0), nil
	}

	var envelope struct {
		Success bool     `json:"success"`
		Data    Response `json:"data"`
	}
	if err = json.Unmarshal(raw, &envelope); err != nil {
		return Unknown(c.authority, time.Now(), 0), nil
	}
	answer := envelope.Data
	if answer.Authority == "" {
		answer.Authority = c.authority
	}
	if answer.CheckedAt.IsZero() {
		answer.CheckedAt = time.Now().UTC()
	}
	if answer.SupplyStatus == "" {
		// An authority that answered without a supply status has not answered
		// the question that was asked.
		answer.SupplyStatus = StatusUnknown
		answer.Serviceable = false
		if answer.ReasonCode == "" {
			answer.ReasonCode = ReasonDependencyUnavailable
		}
	}
	return answer, nil
}

// Set is the caller's per-authority client bundle.
//
// It is a map rather than three fields so a caller iterating authorities does
// not need to know how many there are, and so an authority a deployment has not
// configured is simply absent rather than present-and-broken.
type Set map[string]*Client

// For returns the client for an authority, if this deployment has one.
func (s Set) For(authority string) (*Client, bool) {
	client, ok := s[strings.ToUpper(strings.TrimSpace(authority))]
	if !ok || !client.Configured() {
		return nil, false
	}
	return client, true
}

// Describe renders an authority's configuration state for an operator surface.
func (s Set) Describe(authority string) string {
	if _, ok := s.For(authority); ok {
		return fmt.Sprintf("%s: configured", authority)
	}
	return fmt.Sprintf("%s: not configured", authority)
}
