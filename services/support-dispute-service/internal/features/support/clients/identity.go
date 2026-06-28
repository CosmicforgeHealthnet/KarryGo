package supportclients

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	supportmodels "cosmicforge/logistics/services/support-dispute-service/internal/features/support/models"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// Identity is the display identity resolved from an owning service. Found is
// false when the user could not be resolved (service unconfigured, down, or the
// user does not exist) — callers treat that as "no snapshot", never an error.
type Identity struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Phone  string `json:"phone"`
	Email  string `json:"email"`
	Status string `json:"status"`
	Found  bool   `json:"-"`
}

// IdentityResolver resolves a complainant/respondent identity from its owning
// service, routed by complainant type.
type IdentityResolver interface {
	Resolve(ctx context.Context, complainantType supportmodels.ComplainantType, id string) (Identity, error)
}

// serviceIdentityClient calls one owning service's internal lookup endpoint,
// signing the request with the support service's HMAC identity.
type serviceIdentityClient struct {
	origin     string // bare origin, e.g. http://localhost:8101
	pathPrefix string // e.g. /api/v1/customer/internal/customers/
	secret     []byte
	httpClient *http.Client
}

func (c *serviceIdentityClient) lookup(ctx context.Context, id string) (Identity, error) {
	url := strings.TrimRight(c.origin, "/") + c.pathPrefix + id
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Identity{}, err
	}
	req.Header.Set("Accept", "application/json")
	if err := serviceauth.SignRequest(req, sourceService, c.secret, []byte{}, time.Now()); err != nil {
		return Identity{}, err
	}

	client := c.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return Identity{}, err
	}
	defer resp.Body.Close()

	var envelope struct {
		Success bool     `json:"success"`
		Data    Identity `json:"data"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return Identity{}, err
	}
	if resp.StatusCode >= http.StatusBadRequest || !envelope.Success {
		if envelope.Error.Message == "" {
			envelope.Error.Message = "Identity lookup failed."
		}
		return Identity{}, errors.New(envelope.Error.Message)
	}
	envelope.Data.Found = true
	return envelope.Data, nil
}

// HTTPIdentityResolver routes lookups to per-service clients by complainant type.
// Types with no configured client (e.g. taxi/dispatch scaffolds) resolve to an
// empty, not-found identity with no error.
type HTTPIdentityResolver struct {
	clients map[supportmodels.ComplainantType]*serviceIdentityClient
}

// IdentityConfig configures the owning-service endpoints. Each URL is a bare
// origin; an empty URL or secret disables that service (resolver no-ops for it).
type IdentityConfig struct {
	CustomerURL    string
	CustomerSecret []byte
	HaulingURL     string
	HaulingSecret  []byte
	TaxiURL        string
	TaxiSecret     []byte
	DispatchURL    string
	DispatchSecret []byte
}

func NewHTTPIdentityResolver(cfg IdentityConfig) *HTTPIdentityResolver {
	httpClient := &http.Client{Timeout: 5 * time.Second}
	clients := map[supportmodels.ComplainantType]*serviceIdentityClient{}

	add := func(t supportmodels.ComplainantType, url string, secret []byte, prefix string) {
		if url == "" || len(secret) == 0 {
			return
		}
		clients[t] = &serviceIdentityClient{origin: url, pathPrefix: prefix, secret: secret, httpClient: httpClient}
	}
	add(supportmodels.ComplainantCustomer, cfg.CustomerURL, cfg.CustomerSecret, "/api/v1/customer/internal/customers/")
	add(supportmodels.ComplainantHaulingProvider, cfg.HaulingURL, cfg.HaulingSecret, "/api/v1/hauling/internal/providers/")
	add(supportmodels.ComplainantTaxiProvider, cfg.TaxiURL, cfg.TaxiSecret, "/api/v1/taxi/internal/providers/")
	add(supportmodels.ComplainantDispatchProvider, cfg.DispatchURL, cfg.DispatchSecret, "/api/v1/dispatch/internal/providers/")

	return &HTTPIdentityResolver{clients: clients}
}

func (r *HTTPIdentityResolver) Resolve(ctx context.Context, complainantType supportmodels.ComplainantType, id string) (Identity, error) {
	client, ok := r.clients[complainantType]
	if !ok || id == "" {
		return Identity{}, nil // unconfigured type → empty, not-found, no error
	}
	return client.lookup(ctx, id)
}

// NoopIdentityResolver resolves nothing. Used in tests / when no owning service
// is configured.
type NoopIdentityResolver struct{}

func (NoopIdentityResolver) Resolve(context.Context, supportmodels.ComplainantType, string) (Identity, error) {
	return Identity{}, nil
}
