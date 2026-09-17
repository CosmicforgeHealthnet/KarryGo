// Package commercecapability is the internal contract a logistics authority
// uses to declare what it can actually execute right now, and the client
// commerce callers use to read it.
//
// It carries capability only. It never carries a price, and it never carries
// the identity of a provider or vehicle: choosing who executes a mission
// belongs to the authority alone.
package commercecapability

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// ContractVersion is the version of this capability contract.
const ContractVersion = 2

// Capability is one authority's execution envelope.
type Capability struct {
	Authority              string              `json:"authority"`
	ContractVersion        int                 `json:"contract_version"`
	CapabilityVersion      string              `json:"capability_version"`
	PolicyVersion          string              `json:"policy_version"`
	MultiPickupSupported   bool                `json:"multi_pickup_supported"`
	MaximumPickups         int                 `json:"maximum_pickups"`
	MaximumDropoffs        int                 `json:"maximum_dropoffs"`
	SecurePickupHandover   bool                `json:"secure_pickup_handover"`
	SecureDeliveryHandover bool                `json:"secure_delivery_handover"`
	Products               []ProductCapability `json:"products"`
	CargoPredicates        []string            `json:"cargo_predicates"`
	ServiceabilityRules    []string            `json:"serviceability_constraints"`
}

// ProductCapability is one service product's own envelope.
type ProductCapability struct {
	ProductCode            string `json:"product_code"`
	ProductVersion         int64  `json:"product_version"`
	MaximumPickups         int    `json:"maximum_pickups"`
	SecurePickupHandover   bool   `json:"secure_pickup_handover"`
	SecureDeliveryHandover bool   `json:"secure_delivery_handover"`
}

// ErrUnavailable means the authority did not answer. Callers must fail
// closed on it: an unread capability is never an assumed capability.
var ErrUnavailable = errors.New("logistics capability is unavailable")

// Client reads one authority's capability over signed internal HTTP and
// caches the answer for a short TTL, because planning asks for it on every
// checkout while it changes only on a policy or flag change.
type Client struct {
	BaseURL     string
	Path        string
	ServiceName string
	Secret      []byte
	TTL         time.Duration
	HTTPClient  *http.Client
	Now         func() time.Time

	mu       sync.Mutex
	cached   Capability
	cachedAt time.Time
	hasValue bool
}

// DefaultTTL keeps a capability answer usable without hiding a flag change
// for long.
const DefaultTTL = 30 * time.Second

func (c *Client) now() time.Time {
	if c == nil || c.Now == nil {
		return time.Now()
	}
	return c.Now()
}

// Capability returns the authority's current envelope. A cached answer inside
// the TTL is returned without a call; any failure returns ErrUnavailable and
// never a partially populated envelope.
func (c *Client) Capability(ctx context.Context) (Capability, error) {
	if c == nil || strings.TrimSpace(c.BaseURL) == "" || len(c.Secret) == 0 {
		return Capability{}, ErrUnavailable
	}
	ttl := c.TTL
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	now := c.now()
	c.mu.Lock()
	if c.hasValue && now.Sub(c.cachedAt) < ttl {
		cached := c.cached
		c.mu.Unlock()
		return cached, nil
	}
	c.mu.Unlock()

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	path := c.Path
	if strings.TrimSpace(path) == "" {
		return Capability{}, ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BaseURL, "/")+path, nil)
	if err != nil {
		return Capability{}, ErrUnavailable
	}
	if err = serviceauth.SignRequest(req, c.ServiceName, c.Secret, nil, now); err != nil {
		return Capability{}, ErrUnavailable
	}
	resp, err := client.Do(req)
	if err != nil {
		return Capability{}, ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Capability{}, ErrUnavailable
	}
	var envelope struct {
		Success bool       `json:"success"`
		Data    Capability `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 256<<10)).Decode(&envelope) != nil || !envelope.Success {
		return Capability{}, ErrUnavailable
	}
	capability := envelope.Data
	// A newer contract version may mean fields this caller cannot interpret.
	// Reading it as capability would be guessing, so it fails closed.
	if capability.ContractVersion != ContractVersion {
		return Capability{}, ErrUnavailable
	}
	c.mu.Lock()
	c.cached, c.cachedAt, c.hasValue = capability, now, true
	c.mu.Unlock()
	return capability, nil
}
