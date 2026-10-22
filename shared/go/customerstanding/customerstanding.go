// Package customerstanding lets a booking authority (Taxi, Dispatch,
// Haulage) ask customer-service, the owner of a customer's administrative
// account standing, whether that customer may create a NEW booking.
//
// The answer is read live on every call over the HMAC-protected internal
// context route with only the account_standing scope. Nothing is cached:
// a hold applied by an operator must refuse the very next booking, and a
// lifted hold must allow the very next one.
//
// Every failure fails closed. A transport error, timeout, non-200, malformed
// body, missing standing, an answer for a different customer or a stale
// as_of is reported as ErrUnavailable (HTTP 503), never as "eligible". A
// customer whose standing is not NONE is reported as ErrBookingHold (403).
package customerstanding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// ContextPath is customer-service's internal context route, asking for the
// account_standing scope only.
const ContextPath = "/api/v1/customer/internal/customers/%s/context?scopes=account_standing"

// Stable API error codes a booking authority returns to its customer.
const (
	CodeBookingHold apperrors.Code = "customer_booking_hold"
	CodeUnavailable apperrors.Code = "customer_standing_unavailable"
)

// Defaults. MaxAge bounds how old customer-service's as_of may be; MaxSkew
// tolerates small clock differences in the other direction.
const (
	DefaultTimeout = 2 * time.Second
	DefaultMaxAge  = 30 * time.Second
	DefaultMaxSkew = 5 * time.Second
	maxBodyBytes   = 64 << 10
)

// Sentinel errors, reachable with errors.Is through the returned
// *apperrors.Error.
var (
	ErrBookingHold = errors.New("customerstanding: customer is not eligible to book")
	ErrUnavailable = errors.New("customerstanding: customer standing is unavailable")
)

// Standing is customer-service's answer for one customer.
type Standing struct {
	CustomerID      string
	BookingEligible bool
	Restriction     string
	StandingVersion int64
	AsOf            time.Time
}

// Checker is what a booking usecase depends on. *Client implements it.
type Checker interface {
	Check(ctx context.Context, customerID string) (Standing, error)
}

// Client reads standing from customer-service. BaseURL is a bare origin.
type Client struct {
	BaseURL     string
	ServiceName string
	Secret      []byte
	HTTPClient  *http.Client
	Timeout     time.Duration
	MaxAge      time.Duration
	MaxSkew     time.Duration
	Now         func() time.Time
}

// Config is the constructor input for NewClient.
type Config struct {
	BaseURL     string
	ServiceName string
	Secret      []byte
	Timeout     time.Duration
	MaxAge      time.Duration
}

// NewClient validates the configuration so a misconfigured enforcing
// service fails at startup rather than refusing every booking at runtime.
func NewClient(cfg Config) (*Client, error) {
	base := strings.TrimSpace(cfg.BaseURL)
	if base == "" || len(cfg.Secret) == 0 || strings.TrimSpace(cfg.ServiceName) == "" {
		return nil, errors.New("customerstanding: base URL, service name and secret are required")
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("customerstanding: invalid base URL %q", base)
	}
	if p := strings.Trim(parsed.Path, "/"); p != "" {
		return nil, fmt.Errorf("customerstanding: base URL must be a bare origin, got path %q", parsed.Path)
	}
	return &Client{BaseURL: base, ServiceName: strings.TrimSpace(cfg.ServiceName), Secret: cfg.Secret, Timeout: cfg.Timeout, MaxAge: cfg.MaxAge}, nil
}

type envelope struct {
	Success bool `json:"success"`
	Data    struct {
		CustomerID      string `json:"customer_id"`
		AccountStanding *struct {
			BookingEligible *bool  `json:"booking_eligible"`
			Restriction     string `json:"restriction"`
			StandingVersion *int64 `json:"standing_version"`
			AsOf            string `json:"as_of"`
		} `json:"account_standing"`
	} `json:"data"`
}

// Check returns the customer's standing when booking is allowed. A customer
// on hold returns the standing AND an ErrBookingHold error; anything else
// that is not a clean, fresh answer for exactly this customer returns
// ErrUnavailable.
func (c *Client) Check(ctx context.Context, customerID string) (Standing, error) {
	customerID = strings.TrimSpace(customerID)
	if c == nil || strings.TrimSpace(c.BaseURL) == "" || len(c.Secret) == 0 {
		return Standing{}, Unavailable(errors.New("client not configured"))
	}
	if customerID == "" {
		return Standing{}, Unavailable(errors.New("customer id is empty"))
	}
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	target := strings.TrimRight(c.BaseURL, "/") + fmt.Sprintf(ContextPath, url.PathEscape(customerID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return Standing{}, Unavailable(err)
	}
	if err := serviceauth.SignRequest(req, c.ServiceName, c.Secret, nil, now()); err != nil {
		return Standing{}, Unavailable(err)
	}
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return Standing{}, Unavailable(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBodyBytes))
		return Standing{}, Unavailable(fmt.Errorf("customer-service answered %d", resp.StatusCode))
	}
	var body envelope
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBodyBytes)).Decode(&body); err != nil {
		return Standing{}, Unavailable(fmt.Errorf("malformed response: %w", err))
	}
	standing, err := c.interpret(body, customerID, now())
	if err != nil {
		return Standing{}, Unavailable(err)
	}
	if !standing.BookingEligible {
		return standing, BookingHold(standing)
	}
	return standing, nil
}

func (c *Client) interpret(body envelope, customerID string, now time.Time) (Standing, error) {
	if !body.Success {
		return Standing{}, errors.New("response not successful")
	}
	if body.Data.CustomerID != customerID {
		return Standing{}, fmt.Errorf("answer for customer %q, asked for %q", body.Data.CustomerID, customerID)
	}
	s := body.Data.AccountStanding
	if s == nil || s.BookingEligible == nil || s.StandingVersion == nil || strings.TrimSpace(s.Restriction) == "" {
		return Standing{}, errors.New("account standing missing or incomplete")
	}
	asOf, err := time.Parse(time.RFC3339Nano, s.AsOf)
	if err != nil {
		return Standing{}, fmt.Errorf("invalid as_of %q", s.AsOf)
	}
	maxAge, maxSkew := c.MaxAge, c.MaxSkew
	if maxAge <= 0 {
		maxAge = DefaultMaxAge
	}
	if maxSkew <= 0 {
		maxSkew = DefaultMaxSkew
	}
	if now.Sub(asOf) > maxAge {
		return Standing{}, fmt.Errorf("standing as_of %s is older than %s", asOf.UTC().Format(time.RFC3339Nano), maxAge)
	}
	if asOf.Sub(now) > maxSkew {
		return Standing{}, fmt.Errorf("standing as_of %s is in the future", asOf.UTC().Format(time.RFC3339Nano))
	}
	restriction := strings.TrimSpace(s.Restriction)
	// Eligibility is accepted only when both facts agree: an "eligible"
	// flag next to a restriction is contradictory and is not trusted.
	eligible := *s.BookingEligible && restriction == "NONE"
	if *s.BookingEligible && !eligible {
		return Standing{}, fmt.Errorf("contradictory standing: eligible with restriction %q", restriction)
	}
	return Standing{CustomerID: customerID, BookingEligible: eligible, Restriction: restriction,
		StandingVersion: *s.StandingVersion, AsOf: asOf.UTC()}, nil
}

// BookingHold is the stable 403 a booking authority returns for a customer
// who may not create new bookings. The administrative reason is never
// disclosed.
func BookingHold(standing Standing) *apperrors.Error {
	err := apperrors.New(http.StatusForbidden, CodeBookingHold,
		"Your account cannot make new bookings right now. Please contact support.",
		fmt.Errorf("%w (restriction %s, version %d)", ErrBookingHold, standing.Restriction, standing.StandingVersion))
	return err
}

// Unavailable is the stable, retryable 503 returned when standing cannot be
// confirmed. Booking is refused rather than allowed.
func Unavailable(cause error) *apperrors.Error {
	err := apperrors.New(http.StatusServiceUnavailable, CodeUnavailable,
		"We could not confirm your account right now. Please try again shortly.",
		fmt.Errorf("%w: %v", ErrUnavailable, cause))
	err.Retryable = true
	return err
}

// Gate is the optional enforcement point a booking usecase holds. A nil
// Gate, or one with no Checker, enforces nothing, which keeps services that
// have not enabled enforcement byte-for-byte unchanged.
type Gate struct {
	Checker Checker
}

// NewGate returns nil when checker is nil so callers can wire it
// unconditionally.
func NewGate(checker Checker) *Gate {
	if checker == nil {
		return nil
	}
	return &Gate{Checker: checker}
}

// Enabled reports whether the gate enforces anything.
func (g *Gate) Enabled() bool { return g != nil && g.Checker != nil }

// Allow returns nil when the customer may create a new booking. When the
// gate is disabled it returns a zero Standing and nil.
func (g *Gate) Allow(ctx context.Context, customerID string) (Standing, error) {
	if !g.Enabled() {
		return Standing{}, nil
	}
	standing, err := g.Checker.Check(ctx, customerID)
	switch {
	case err == nil && !standing.BookingEligible:
		// A checker that answers "not eligible" without an error is still
		// a refusal; eligibility is never assumed.
		return standing, BookingHold(standing)
	case err == nil:
		return standing, nil
	case errors.Is(err, ErrBookingHold) || errors.Is(err, ErrUnavailable):
		return standing, err
	default:
		return Standing{}, Unavailable(err)
	}
}
