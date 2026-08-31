package commercemission

// The commerce mission client.
//
// It is deliberately thin. It signs, sends, and verifies that the answer
// belongs to the request and was not re-priced. It derives no fare, chooses no
// authority, and never retries on its own: retry policy belongs to the caller's
// durable workflow, not to an HTTP client that would lose its state on restart.

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

const maxResponseBytes = 1 << 20

// Client calls one logistics authority's commerce mission endpoint.
type Client struct {
	baseURL string
	apiBase string
	service string
	secret  []byte
	http    *http.Client
}

// Options configures a client.
type Options struct {
	// BaseURL is a BARE ORIGIN. The client appends the authority's own API
	// base and path itself, so a full path here would produce a doubled path
	// that 404s every call.
	BaseURL string
	APIBase string
	Service string
	Secret  string
	Timeout time.Duration
}

// New builds a mission client.
func New(options Options) *Client {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(options.BaseURL), "/"),
		apiBase: strings.TrimRight(strings.TrimSpace(options.APIBase), "/"),
		service: strings.TrimSpace(options.Service),
		secret:  []byte(options.Secret),
		http:    &http.Client{Timeout: timeout},
	}
}

// Configured reports whether this authority can be called at all. An
// unconfigured authority has no credentials, and calling it would be a
// guaranteed failure rather than an integration.
func (c *Client) Configured() bool {
	return c != nil && c.baseURL != "" && len(c.secret) > 0 && c.service != ""
}

// Service names the authority this client speaks to.
func (c *Client) Service() string {
	if c == nil {
		return ""
	}
	return c.service
}

// CreateMission asks the authority to execute one mission.
//
// Idempotency is the request's own MissionRef, which the authority keys on, so
// a repeated call returns the same mission rather than creating a second one.
// That is what makes this safe to call from a durable workflow that may run
// the same step more than once.
func (c *Client) CreateMission(ctx context.Context, request Request) (Mission, error) {
	if !c.Configured() {
		return Mission{}, ErrNotConfigured
	}
	if reason, ok := request.Validate(); !ok {
		return Mission{}, fmt.Errorf("%w: %s", ErrInvalidRequest, reason)
	}

	body, err := json.Marshal(request)
	if err != nil {
		return Mission{}, err
	}

	httpRequest, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.baseURL+c.apiBase+Path, bytes.NewReader(body),
	)
	if err != nil {
		return Mission{}, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	// The mission reference doubles as the replay-safe command key, so the
	// authority can recognise a retry at its own boundary too.
	httpRequest.Header.Set("Idempotency-Key", request.MissionRef)

	if err = serviceauth.SignRequest(
		httpRequest, SignatureCaller, c.secret, body, time.Now(),
	); err != nil {
		return Mission{}, err
	}

	response, err := c.http.Do(httpRequest)
	if err != nil {
		// A transport failure is an UNKNOWN outcome: the authority may have
		// accepted the mission. The caller's durable workflow retries with the
		// same MissionRef rather than assuming either way.
		return Mission{}, fmt.Errorf("%w: %v", ErrAuthorityUnavailable, err)
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return Mission{}, fmt.Errorf("%w: %v", ErrAuthorityUnavailable, err)
	}

	var envelope Envelope
	if err = json.Unmarshal(raw, &envelope); err != nil {
		return Mission{}, fmt.Errorf("%w: malformed response", ErrAuthorityUnavailable)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || !envelope.Success {
		message := strings.TrimSpace(envelope.Error.Message)
		if message == "" {
			message = fmt.Sprintf("status %d", response.StatusCode)
		}
		if response.StatusCode >= 400 && response.StatusCode < 500 {
			// A 4xx is the authority's own decision about this request. It is
			// a definite answer, not a transport problem to retry forever.
			return Mission{}, fmt.Errorf("%w: %s", ErrInvalidRequest, message)
		}
		return Mission{}, fmt.Errorf("%w: %s", ErrAuthorityUnavailable, message)
	}

	if err = VerifyMission(request, envelope.Data); err != nil {
		return Mission{}, err
	}
	return envelope.Data, nil
}

// ReadMission resolves the caller's stable operation reference after an
// unknown create/cancel outcome. It is deliberately read-only and still signed,
// so only the allowed commerce caller can observe the mission.
func (c *Client) ReadMission(ctx context.Context, operationRef string) (Mission, error) {
	if !c.Configured() {
		return Mission{}, ErrNotConfigured
	}
	operationRef = strings.TrimSpace(operationRef)
	if operationRef == "" {
		return Mission{}, ErrInvalidRequest
	}
	endpoint := c.baseURL + c.apiBase + OperationPath + "/" + url.PathEscape(operationRef)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Mission{}, err
	}
	request.Header.Set("Accept", "application/json")
	if err = serviceauth.SignRequest(request, SignatureCaller, c.secret, nil, time.Now()); err != nil {
		return Mission{}, err
	}
	return c.doMission(request)
}

func (c *Client) CancelMission(ctx context.Context, request CancelRequest) (Mission, error) {
	if !c.Configured() {
		return Mission{}, ErrNotConfigured
	}
	operationRef := strings.TrimSpace(request.OperationRef)
	if operationRef == "" {
		operationRef = strings.TrimSpace(request.MissionRef)
		request.OperationRef = operationRef
	}
	if operationRef == "" {
		return Mission{}, ErrInvalidRequest
	}
	body, err := json.Marshal(request)
	if err != nil {
		return Mission{}, err
	}
	endpoint := c.baseURL + c.apiBase + OperationPath + "/" + url.PathEscape(operationRef) + "/cancel"
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Mission{}, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("Idempotency-Key", operationRef+"-cancel")
	if err = serviceauth.SignRequest(httpRequest, SignatureCaller, c.secret, body, time.Now()); err != nil {
		return Mission{}, err
	}
	mission, err := c.doMission(httpRequest)
	if err != nil {
		return Mission{}, err
	}
	if strings.TrimSpace(mission.MissionRef) == "" || strings.TrimSpace(mission.ExternalRef) == "" {
		return Mission{}, ErrMissionMismatch
	}
	return mission, nil
}

func (c *Client) doMission(request *http.Request) (Mission, error) {
	response, err := c.http.Do(request)
	if err != nil {
		return Mission{}, fmt.Errorf("%w: %v", ErrAuthorityUnavailable, err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return Mission{}, fmt.Errorf("%w: %v", ErrAuthorityUnavailable, err)
	}
	var envelope Envelope
	if err = json.Unmarshal(raw, &envelope); err != nil {
		return Mission{}, fmt.Errorf("%w: malformed response", ErrAuthorityUnavailable)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || !envelope.Success {
		message := strings.TrimSpace(envelope.Error.Message)
		if message == "" {
			message = fmt.Sprintf("status %d", response.StatusCode)
		}
		if response.StatusCode == http.StatusNotFound ||
			(response.StatusCode >= 400 && response.StatusCode < 500) {
			return Mission{}, fmt.Errorf("%w: %s", ErrInvalidRequest, message)
		}
		return Mission{}, fmt.Errorf("%w: %s", ErrAuthorityUnavailable, message)
	}
	return envelope.Data, nil
}
