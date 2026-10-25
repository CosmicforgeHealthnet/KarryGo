package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const PricingOwner = "pricing-service"

type PricingOperation struct {
	Method, Path, Target, Staff, Reason, IdempotencyKey string
	Payload                                             json.RawMessage
	Scope                                               adminread.Scope
}

type Pricing struct {
	origin *url.URL
	secret []byte
	client *http.Client
}

func NewPricing(origin string, secret []byte) (*Pricing, error) {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawPath != "" || u.RawQuery != "" || u.Fragment != "" || len(secret) < 32 || len(secret) > 128 {
		return nil, ErrInvalid
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: time.Second}).DialContext, ResponseHeaderTimeout: 5 * time.Second, MaxConnsPerHost: 8, MaxIdleConnsPerHost: 4, IdleConnTimeout: 30 * time.Second}
	return &Pricing{origin: u, secret: append([]byte(nil), secret...), client: &http.Client{Transport: transport, Timeout: 7 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (c *Pricing) Close() { c.client.CloseIdleConnections() }

func pricingRoute(operation PricingOperation) (string, bool) {
	if operation.Target != "" && (!opaquePricing(operation.Target) || strings.ContainsAny(operation.Target, "/\\")) {
		return "", false
	}
	switch operation.Path {
	case "policies/drafts", "simulate", "rollouts":
		return operation.Path, operation.Target == "" && operation.Method == http.MethodPost
	case "resolve":
		return operation.Path, operation.Target == "" && operation.Method == http.MethodPost
	case "approvals":
		return operation.Path, operation.Target == "" && operation.Method == http.MethodGet
	case "policy.versions":
		return "policies/" + operation.Target + "/versions", operation.Target != "" && operation.Method == http.MethodGet
	case "policy.version":
		return "policy-versions/" + operation.Target, operation.Target != "" && (operation.Method == http.MethodGet || operation.Method == http.MethodPut)
	case "policy.submit", "policy.approve", "policy.reject", "policy.activate":
		return "policy-versions/" + operation.Target + "/" + strings.TrimPrefix(operation.Path, "policy."), operation.Target != "" && operation.Method == http.MethodPost
	case "market.freeze":
		return "markets/" + operation.Target + "/freeze", operation.Target != "" && (operation.Method == http.MethodGet || operation.Method == http.MethodPost)
	case "market.unfreeze":
		return "markets/" + operation.Target + "/unfreeze", operation.Target != "" && operation.Method == http.MethodPost
	default:
		return "", false
	}
}

func opaquePricing(value string) bool {
	if len(value) == 0 || len(value) > 128 || value != strings.TrimSpace(value) {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._:@-", r)) {
			return false
		}
	}
	return true
}

func (c *Pricing) Execute(ctx context.Context, operation PricingOperation) (json.RawMessage, error) {
	route, ok := pricingRoute(operation)
	if !ok || operation.Method != http.MethodGet && operation.Method != http.MethodPost && operation.Method != http.MethodPut {
		return nil, ErrInvalid
	}
	u := *c.origin
	u.Path = "/internal/pricing/v1/admin/" + route
	if operation.Scope.Market != "" || operation.Scope.Zone != "" || operation.Scope.Organization != "" || operation.Scope.Tenant != "" {
		if !adminread.Opaque(operation.Scope.Market) || !adminread.Opaque(operation.Scope.Zone) || !adminread.Opaque(operation.Scope.Organization) || !adminread.Opaque(operation.Scope.Tenant) {
			return nil, ErrInvalid
		}
		query := u.Query()
		query.Set("admin_market", operation.Scope.Market)
		query.Set("admin_zone", operation.Scope.Zone)
		query.Set("admin_organization", operation.Scope.Organization)
		query.Set("admin_tenant", operation.Scope.Tenant)
		u.RawQuery = query.Encode()
	}
	body := []byte(operation.Payload)
	sourceMethod := operation.Method
	if operation.Method == http.MethodGet {
		if len(body) != 0 {
			return nil, ErrInvalid
		}
	} else if len(body) == 0 || !json.Valid(body) || !opaquePricing(operation.Staff) || !opaquePricing(operation.Reason) || !opaquePricing(operation.IdempotencyKey) {
		return nil, ErrInvalid
	}
	if operation.Path == "resolve" {
		var values map[string]any
		if json.Unmarshal(body, &values) != nil {
			return nil, ErrInvalid
		}
		query := u.Query()
		for _, name := range []string{"kind", "zone_id", "market_id", "state_id", "country_code", "as_of"} {
			if value, ok := values[name].(string); ok && value != "" {
				query.Set(name, value)
			}
		}
		u.RawQuery, sourceMethod, body = query.Encode(), http.MethodGet, nil
	}
	request, err := http.NewRequestWithContext(ctx, sourceMethod, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, ErrInvalid
	}
	request.Header.Set("Content-Type", "application/json")
	if operation.IdempotencyKey != "" {
		request.Header.Set("X-Idempotency-Key", operation.IdempotencyKey)
	}
	if err = serviceauth.SignRequest(request, "admin-backoffice-service", c.secret, body, time.Now()); err != nil {
		return nil, ErrUnavailable
	}
	response, err := c.client.Do(request)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1048577))
	if err != nil || len(raw) > 1048576 || !json.Valid(raw) {
		return nil, ErrInvalid
	}
	switch response.StatusCode {
	case http.StatusOK, http.StatusCreated:
		return json.RawMessage(raw), nil
	case http.StatusNotFound:
		return nil, ErrNotFound
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, ErrScopeDenied
	case http.StatusConflict, http.StatusPreconditionFailed:
		return nil, ErrRefused
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return nil, ErrInvalid
	default:
		return nil, ErrUnavailable
	}
}
