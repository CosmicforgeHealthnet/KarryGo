package commercequote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

const maxQuoteResponse = 32 << 10

var (
	// ErrNotConfigured means this deployment has no credentials for the
	// authority, so no quote can be requested from it. It is distinct from a
	// refusal: nothing was asked.
	ErrNotConfigured = errors.New("commercequote: authority is not configured")

	// ErrInvalidRequest is a request this package refuses to send.
	ErrInvalidRequest = errors.New("commercequote: invalid request")

	// ErrUnavailable is a transport or server fault. The caller does not know
	// whether a quote exists, so it must re-ask by reference rather than
	// assume either outcome.
	ErrUnavailable = errors.New("commercequote: authority is unavailable")

	// ErrContractViolation means the authority answered in a shape this
	// contract forbids — most importantly, a fee on an unserviceable answer.
	// It is treated as a fault rather than quietly accepted, because
	// accepting it is how a fake fee would reach a customer.
	ErrContractViolation = errors.New("commercequote: authority response violates the contract")
)

// Client requests quotes from one logistics authority.
//
// One type serves every authority because the contract is one contract: only
// the base URL and the shared secret differ. Two near-identical clients would
// be two places for the verification below to drift.
type Client struct {
	// baseURL is a BARE ORIGIN. The API base and QuotePath are appended here,
	// which is deliberate: a caller pasting a full path into a base URL is a
	// mistake this platform has already made once, and it produced a doubled
	// path that 404'd every internal call.
	baseURL string

	// apiBase is the authority's own API base, e.g. "/api/v1/hauling". The
	// quote endpoint is mounted at apiBase + QuotePath.
	apiBase string

	// service names the answering authority for the caller's own records.
	service string

	secret []byte

	http *http.Client
	now  func() time.Time
}

// Options configures a Client.
type Options struct {
	BaseURL string
	APIBase string
	Service string
	Secret  string
	Timeout time.Duration
}

// New builds a quote client. A missing base URL or secret yields a client that
// answers ErrNotConfigured rather than a nil pointer, so a deployment with the
// integration switched off degrades to an explicit refusal.
func New(options Options) *Client {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 8 * time.Second
	}

	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(options.BaseURL), "/"),
		apiBase: "/" + strings.Trim(strings.TrimSpace(options.APIBase), "/"),
		service: strings.TrimSpace(options.Service),
		secret:  []byte(options.Secret),
		http:    &http.Client{Timeout: timeout},
		now:     time.Now,
	}
}

// Configured reports whether this client can reach its authority.
func (c *Client) Configured() bool {
	return c != nil && c.baseURL != "" && len(c.secret) > 0
}

// Service names the authority this client speaks to.
func (c *Client) Service() string {
	if c == nil {
		return ""
	}

	return c.service
}

// RequestQuote asks the authority for an authoritative quote.
//
// An unserviceable answer is returned as a Quote with Serviceable false and a
// reason code — NOT as an error. Refusing to serve a route is a normal,
// expected answer that the planner must be able to report to the customer;
// only a fault is an error.
func (c *Client) RequestQuote(
	ctx context.Context,
	request Request,
) (Quote, error) {
	if !c.Configured() {
		return Quote{}, ErrNotConfigured
	}

	if message, ok := request.Validate(); !ok {
		return Quote{}, fmt.Errorf("%w: %s", ErrInvalidRequest, message)
	}

	body, err := json.Marshal(request)
	if err != nil {
		return Quote{}, fmt.Errorf("%w: encode request", ErrInvalidRequest)
	}

	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+c.apiBase+QuotePath,
		bytes.NewReader(body),
	)
	if err != nil {
		return Quote{}, fmt.Errorf("%w: build request", ErrUnavailable)
	}

	httpRequest.Header.Set("Content-Type", "application/json")

	if err = serviceauth.SignRequest(
		httpRequest,
		SignatureCaller,
		c.secret,
		body,
		c.now().UTC(),
	); err != nil {
		return Quote{}, fmt.Errorf("%w: sign request", ErrUnavailable)
	}

	response, err := c.http.Do(httpRequest)
	if err != nil {
		return Quote{}, fmt.Errorf("%w: transport", ErrUnavailable)
	}

	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, maxQuoteResponse+1))
	if err != nil {
		return Quote{}, fmt.Errorf("%w: read response", ErrUnavailable)
	}

	if len(raw) > maxQuoteResponse {
		return Quote{}, fmt.Errorf("%w: oversized response", ErrUnavailable)
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Quote{}, fmt.Errorf(
			"%w: authority answered %d", ErrUnavailable, response.StatusCode,
		)
	}

	var envelope Envelope

	if err = json.Unmarshal(raw, &envelope); err != nil {
		return Quote{}, fmt.Errorf("%w: decode response", ErrUnavailable)
	}

	quote := envelope.Data

	if quote.Service == "" {
		quote.Service = c.service
	}

	if err = verifyQuote(request, quote); err != nil {
		return Quote{}, err
	}

	return quote, nil
}

// verifyQuote is the caller-side guard against an authority answering
// something the contract forbids.
//
// It exists because Vendor Service is the party that would present a bad
// answer to a customer as a delivery fee. Trusting the authority to be correct
// is right; trusting it to be correct WITHOUT CHECKING is how a fee with no
// serviceable delivery behind it reaches a checkout.
func verifyQuote(request Request, quote Quote) error {
	if strings.TrimSpace(quote.QuoteReference) != strings.TrimSpace(request.QuoteReference) {
		return fmt.Errorf(
			"%w: answered a different quote reference", ErrContractViolation,
		)
	}

	if !quote.Serviceable {
		// The critical invariant. An unserviceable answer carrying a fee is
		// exactly the "fake fee" this section forbids.
		if quote.FeeMinor != 0 {
			return fmt.Errorf(
				"%w: unserviceable quote carries a fee", ErrContractViolation,
			)
		}

		if strings.TrimSpace(quote.ReasonCode) == "" ||
			quote.ReasonCode == ReasonServiceable {
			return fmt.Errorf(
				"%w: unserviceable quote names no reason", ErrContractViolation,
			)
		}

		return nil
	}

	if strings.TrimSpace(quote.QuoteID) == "" {
		return fmt.Errorf(
			"%w: serviceable quote has no authority reference",
			ErrContractViolation,
		)
	}

	if quote.FeeMinor < 0 {
		return fmt.Errorf("%w: negative fee", ErrContractViolation)
	}

	if !strings.EqualFold(
		strings.TrimSpace(quote.Currency), strings.TrimSpace(request.Currency),
	) {
		return fmt.Errorf(
			"%w: quoted in a different currency", ErrContractViolation,
		)
	}

	if quote.VehicleClass != request.VehicleClass {
		return fmt.Errorf(
			"%w: quoted a different vehicle class", ErrContractViolation,
		)
	}

	if quote.ExpiresAt.IsZero() {
		return fmt.Errorf("%w: quote has no expiry", ErrContractViolation)
	}

	if quote.AvailableProviders <= 0 {
		return fmt.Errorf(
			"%w: serviceable quote reports no available provider",
			ErrContractViolation,
		)
	}

	if math.IsNaN(quote.DistanceKm) || math.IsInf(quote.DistanceKm, 0) ||
		quote.DistanceKm < 0 {
		return fmt.Errorf("%w: implausible distance", ErrContractViolation)
	}

	return nil
}

func trim(value string) string { return strings.TrimSpace(value) }

func validCoordinate(latitude, longitude float64) bool {
	if math.IsNaN(latitude) || math.IsNaN(longitude) ||
		math.IsInf(latitude, 0) || math.IsInf(longitude, 0) {
		return false
	}

	if latitude < -90 || latitude > 90 ||
		longitude < -180 || longitude > 180 {
		return false
	}

	// Null island is what an unset or failed geocode looks like. Quoting a
	// delivery to it is worse than refusing to quote one.
	return latitude != 0 || longitude != 0
}
