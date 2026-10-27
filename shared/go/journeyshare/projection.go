// Package journeyshare defines a bounded read contract, never execution authority.
package journeyshare

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/serviceauth"
)

type Place struct {
	Label string  `json:"label"`
	Lat   float64 `json:"lat"`
	Lng   float64 `json:"lng"`
}
type Vehicle struct {
	Make    string `json:"make,omitempty"`
	Model   string `json:"model,omitempty"`
	Display string `json:"display,omitempty"`
	Plate   string `json:"plate,omitempty"`
}
type Provider struct {
	DisplayName string   `json:"display_name"`
	Vehicle     *Vehicle `json:"vehicle,omitempty"`
}
type Location struct {
	Available bool       `json:"available"`
	State     string     `json:"state"`
	Lat       *float64   `json:"lat,omitempty"`
	Lng       *float64   `json:"lng,omitempty"`
	Heading   *float64   `json:"heading,omitempty"`
	AccuracyM *float64   `json:"accuracy_m,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}
type Progress struct {
	Stage string `json:"stage"`
}

// Projection is internal-only. PublicSnapshot is the explicit privacy boundary.
type Projection struct {
	Authority     string     `json:"authority"`
	SourceRef     string     `json:"source_ref"`
	ProductCode   string     `json:"product_code"`
	CustomerID    string     `json:"customer_id"`
	Eligible      bool       `json:"eligible"`
	Status        string     `json:"status"`
	PublicStatus  string     `json:"public_status"`
	Pickup        *Place     `json:"pickup,omitempty"`
	Dropoff       *Place     `json:"dropoff,omitempty"`
	Provider      *Provider  `json:"provider,omitempty"`
	Location      Location   `json:"location"`
	Progress      Progress   `json:"progress"`
	ETA           *time.Time `json:"eta"`
	PickupETA     *time.Time `json:"pickup_eta,omitempty"`
	DropoffETA    *time.Time `json:"dropoff_eta,omitempty"`
	SourceVersion int64      `json:"source_version"`
	Terminal      bool       `json:"terminal"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
}
type PublicSnapshot struct {
	ProductCode  string     `json:"product_code"`
	PublicStatus string     `json:"public_status"`
	Pickup       *Place     `json:"pickup,omitempty"`
	Dropoff      *Place     `json:"dropoff,omitempty"`
	Provider     *Provider  `json:"provider,omitempty"`
	Location     Location   `json:"location"`
	Progress     Progress   `json:"progress"`
	ETA          *time.Time `json:"eta"`
	PickupETA    *time.Time `json:"pickup_eta,omitempty"`
	DropoffETA   *time.Time `json:"dropoff_eta,omitempty"`
	Terminal     bool       `json:"terminal"`
}

func (p Projection) Public(permissions []string) PublicSnapshot {
	out := PublicSnapshot{ProductCode: p.ProductCode, PublicStatus: p.PublicStatus, Location: Location{State: "UNAVAILABLE"}, Progress: p.Progress, Terminal: p.Terminal}
	for _, field := range permissions {
		switch field {
		case "route":
			out.Pickup = p.Pickup
			out.Dropoff = p.Dropoff
		case "provider":
			out.Provider = p.Provider
		case "location":
			if !p.Terminal {
				out.Location = p.Location
			}
		case "eta":
			out.ETA = p.ETA
			out.PickupETA = p.PickupETA
			out.DropoffETA = p.DropoffETA
		}
	}
	return out
}
func ValidCombination(authority, product string) bool {
	return authority == "TAXI" && product == "RIDE" || authority == "DISPATCH" && product == "SEND" || authority == "HAULING" && product == "TRUCK" || authority == "VENDOR_COMMERCE" && (product == "FOOD" || product == "SHOP")
}

// ValidReadCombination includes the private commerce executor vocabulary.
// These executor projections cannot be used to create Customer shares directly.
func ValidReadCombination(authority, product string) bool {
	return ValidCombination(authority, product) || authority == "TAXI_GOODS" && product == "COMMERCE_MISSION"
}

type Reader interface {
	Read(context.Context, string, string, string) (Projection, error)
}
type Endpoint struct {
	BaseURL string
	Secret  []byte
}

// defaultClient is shared so authority reads reuse connections. Go's default
// transport keeps only two idle connections per host, which under concurrent
// share reads churns a new TCP connection per request and exhausts ephemeral
// ports. Redirects are never followed: a projection is read from one place.
var defaultClient = &http.Client{
	Timeout: 5 * time.Second,
	Transport: func() http.RoundTripper {
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.MaxIdleConns = 256
		t.MaxIdleConnsPerHost = 64
		return t
	}(),
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
}

type Client struct {
	Endpoints map[string]Endpoint
	HTTP      *http.Client
	Caller    string
	Paths     map[string]string
}

func (c *Client) Read(ctx context.Context, authority, ref, customer string) (Projection, error) {
	start := time.Now()
	defer func() { Observe("authority_read", time.Since(start)) }()
	ep, ok := c.Endpoints[authority]
	if !ok || ep.BaseURL == "" || len(ep.Secret) == 0 {
		return Projection{}, apperrors.Unavailable("Journey authority is unavailable.", nil)
	}
	paths := map[string]string{"TAXI": "/internal/taxi/v1/customer-share/bookings/", "DISPATCH": "/internal/dispatch/v1/customer-share/trips/", "HAULING": "/internal/hauling/v1/customer-share/bookings/", "VENDOR_COMMERCE": "/internal/vendor/v1/customer-share/missions/"}
	if c.Paths != nil {
		paths = c.Paths
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(ep.BaseURL, "/")+paths[authority]+url.PathEscape(ref)+"?customer_id="+url.QueryEscape(customer), nil)
	if err != nil {
		Observe("authority_request_error", 0)
		return Projection{}, apperrors.Unavailable("Journey authority is unavailable.", nil)
	}
	caller := c.Caller
	if caller == "" {
		caller = "customer-service"
	}
	if err = serviceauth.SignRequest(req, caller, ep.Secret, nil, time.Now().UTC()); err != nil {
		return Projection{}, err
	}
	client := c.HTTP
	if client == nil {
		client = defaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		Observe("authority_transport_error", 0)
		return Projection{}, apperrors.Unavailable("Journey authority is unavailable.", nil)
	}
	defer res.Body.Close()
	if res.StatusCode == 404 || res.StatusCode == 403 {
		return Projection{}, apperrors.NotFound("Journey was not found.", nil)
	}
	if res.StatusCode != 200 {
		Observe("authority_read_error", 0)
		return Projection{}, apperrors.Unavailable("Journey authority is unavailable.", nil)
	}
	var envelope struct {
		Success bool       `json:"success"`
		Data    Projection `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 32<<10)).Decode(&envelope) != nil || !envelope.Success {
		return Projection{}, apperrors.Unavailable("Journey authority returned an invalid response.", nil)
	}
	p := envelope.Data
	if p.CustomerID != customer || p.SourceRef != ref || p.Authority != authority || !ValidReadCombination(authority, p.ProductCode) || p.SourceVersion < 1 {
		return Projection{}, apperrors.NotFound("Journey was not found.", nil)
	}
	return p, nil
}
func (p Projection) Validate() error {
	if !ValidCombination(p.Authority, p.ProductCode) || p.CustomerID == "" || p.SourceRef == "" || p.SourceVersion < 1 {
		return fmt.Errorf("invalid journey projection")
	}
	return nil
}
