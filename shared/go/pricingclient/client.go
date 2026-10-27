package pricingclient

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

const maxResponse = 256 << 10

var (
	// ErrNotConfigured means this deployment has no address or secret for
	// pricing, so nothing was asked.
	ErrNotConfigured = errors.New("pricingclient: pricing is not configured")
	// ErrInvalidRequest is a request this package refuses to send.
	ErrInvalidRequest = errors.New("pricingclient: invalid request")
	// ErrUnavailable is a transport or server fault: the caller cannot know
	// whether pricing acted, and must ask again by the same reference.
	ErrUnavailable = errors.New("pricingclient: pricing is unavailable")
	// ErrRefused is pricing answering "no" (4xx): an unknown market, a quote
	// that cannot be bound, a caller it does not trust.
	ErrRefused = errors.New("pricingclient: pricing refused the request")
)

// Client talks to pricing-service as one signed calling service.
type Client struct {
	// baseURL is a BARE ORIGIN; the paths above are appended to it.
	baseURL     string
	serviceName string
	secret      []byte
	http        *http.Client
	now         func() time.Time
}

// Options configures a Client.
type Options struct {
	BaseURL     string
	ServiceName string
	Secret      string
	Timeout     time.Duration
}

// New builds a client. A missing address, name or secret yields a client that
// answers ErrNotConfigured, so an integration that is switched off degrades to
// an explicit "not asked".
func New(options Options) *Client {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &Client{
		baseURL:     strings.TrimRight(strings.TrimSpace(options.BaseURL), "/"),
		serviceName: strings.TrimSpace(options.ServiceName),
		secret:      []byte(options.Secret),
		http:        &http.Client{Timeout: timeout},
		now:         time.Now,
	}
}

// Configured reports whether this client can reach pricing.
func (c *Client) Configured() bool {
	return c != nil && c.baseURL != "" && c.serviceName != "" && len(c.secret) > 0
}

// Quote asks for one quote per requested class.
func (c *Client) Quote(ctx context.Context, request QuoteRequest) (QuoteResponse, error) {
	if message, ok := request.Validate(); !ok {
		return QuoteResponse{}, fmt.Errorf("%w: %s", ErrInvalidRequest, message)
	}
	var response QuoteResponse
	err := c.post(ctx, QuotesPath, request, &response)
	return response, err
}

// FindByRef returns the quote this caller was issued for reference and class.
func (c *Client) FindByRef(ctx context.Context, consumerRef, class string) (Quote, error) {
	if strings.TrimSpace(consumerRef) == "" || strings.TrimSpace(class) == "" {
		return Quote{}, fmt.Errorf("%w: reference and class are required", ErrInvalidRequest)
	}
	var quote Quote
	query := url.Values{"consumer_ref": {consumerRef}, "class": {class}}
	err := c.send(ctx, http.MethodGet, ByRefPath+"?"+query.Encode(), nil, &quote)
	return quote, err
}

// Bind ties a quote to the booking that used it. Binding the same quote to the
// same booking again is answered with the existing binding.
func (c *Client) Bind(ctx context.Context, quoteID string, request BindRequest) (Binding, error) {
	if strings.TrimSpace(quoteID) == "" || strings.TrimSpace(request.BookingRef) == "" {
		return Binding{}, fmt.Errorf("%w: quote id and booking ref are required", ErrInvalidRequest)
	}
	var binding Binding
	err := c.post(ctx, BindPath(quoteID), request, &binding)
	return binding, err
}

// Reconcile prices a bound quote again from the actual route.
func (c *Client) Reconcile(ctx context.Context, quoteID string, request ReconcileRequest) (Reconciliation, error) {
	if strings.TrimSpace(quoteID) == "" {
		return Reconciliation{}, fmt.Errorf("%w: quote id is required", ErrInvalidRequest)
	}
	if request.Route.DistanceMeters < 0 || request.Route.DurationSeconds < 0 {
		return Reconciliation{}, fmt.Errorf("%w: route figures cannot be negative", ErrInvalidRequest)
	}
	var reconciliation Reconciliation
	err := c.post(ctx, ReconcilePath(quoteID), request, &reconciliation)
	return reconciliation, err
}

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) post(ctx context.Context, path string, payload, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("%w: encode request", ErrInvalidRequest)
	}
	return c.send(ctx, http.MethodPost, path, body, out)
}

func (c *Client) send(ctx context.Context, method, path string, body []byte, out any) error {
	if !c.Configured() {
		return ErrNotConfigured
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: build request", ErrUnavailable)
	}
	request.Header.Set("Content-Type", "application/json")
	if err := serviceauth.SignRequest(request, c.serviceName, c.secret, body, c.now().UTC()); err != nil {
		return fmt.Errorf("%w: sign request", ErrUnavailable)
	}

	response, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("%w: transport", ErrUnavailable)
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if err != nil || len(raw) > maxResponse {
		return fmt.Errorf("%w: read response", ErrUnavailable)
	}

	var decoded envelope
	_ = json.Unmarshal(raw, &decoded)

	switch {
	case response.StatusCode >= 500:
		return fmt.Errorf("%w: pricing answered %d", ErrUnavailable, response.StatusCode)
	case response.StatusCode >= 400:
		message := http.StatusText(response.StatusCode)
		if decoded.Error != nil && decoded.Error.Message != "" {
			message = decoded.Error.Message
		}
		return fmt.Errorf("%w (%d): %s", ErrRefused, response.StatusCode, message)
	}
	if !decoded.Success || len(decoded.Data) == 0 {
		return fmt.Errorf("%w: unexpected response", ErrUnavailable)
	}
	if err := json.Unmarshal(decoded.Data, out); err != nil {
		return fmt.Errorf("%w: decode response", ErrUnavailable)
	}
	return nil
}
