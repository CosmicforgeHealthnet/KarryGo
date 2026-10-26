// Package campaignclients holds the customer-service consent authority
// adapter used by campaign fulfilment.
package campaignclients

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

const (
	AudiencePath = "/internal/customer/v1/marketing-audience"
	CheckPath    = "/internal/customer/v1/marketing-consent/check"
	ServiceName  = "notification-service"
	MaxBatch     = 500
)

// ErrUnavailable means the consent authority could not answer. It is never
// read as consent.
var ErrUnavailable = errors.New("customer consent authority unavailable")

type AudienceMember struct {
	CustomerID     string `json:"customer_id"`
	ConsentVersion int64  `json:"consent_version"`
}

type AudiencePage struct {
	Channel    string           `json:"channel"`
	Market     string           `json:"market"`
	AsOf       time.Time        `json:"as_of"`
	Members    []AudienceMember `json:"members"`
	NextCursor string           `json:"next_cursor"`
}

type Eligibility struct {
	CustomerID     string  `json:"customer_id"`
	Eligible       bool    `json:"eligible"`
	Reason         string  `json:"reason"`
	ConsentVersion int64   `json:"consent_version"`
	Market         *string `json:"market"`
	Email          *string `json:"email,omitempty"`
}

type CheckResult struct {
	Channel string        `json:"channel"`
	AsOf    time.Time     `json:"as_of"`
	Results []Eligibility `json:"results"`
}

// ConsentAuthority is the narrow contract campaign fulfilment needs.
type ConsentAuthority interface {
	Audience(ctx context.Context, channel, market, cursor string, limit int) (AudiencePage, error)
	Check(ctx context.Context, channel string, customerIDs []string) (CheckResult, error)
}

// CustomerAudienceClient signs HMAC v2 requests as notification-service.
type CustomerAudienceClient struct {
	BaseURL string // bare origin, e.g. http://customer-service:8101
	Secret  []byte
	HTTP    *http.Client
}

func (c *CustomerAudienceClient) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func (c *CustomerAudienceClient) do(ctx context.Context, method, target string, body []byte, out any) error {
	if c == nil || c.BaseURL == "" || len(c.Secret) == 0 {
		return ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+target, bytes.NewReader(body))
	if err != nil {
		return ErrUnavailable
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := serviceauth.SignRequest(req, ServiceName, c.Secret, body, time.Now()); err != nil {
		return ErrUnavailable
	}
	res, err := c.client().Do(req)
	if err != nil {
		return fmt.Errorf("%w: transport", ErrUnavailable)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil || res.StatusCode != http.StatusOK {
		// The status code only; the body may echo identifiers.
		return fmt.Errorf("%w: status %d", ErrUnavailable, res.StatusCode)
	}
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &envelope) != nil || !envelope.Success || json.Unmarshal(envelope.Data, out) != nil {
		return fmt.Errorf("%w: malformed response", ErrUnavailable)
	}
	return nil
}

func (c *CustomerAudienceClient) Audience(ctx context.Context, channel, market, cursor string, limit int) (AudiencePage, error) {
	q := url.Values{"channel": {channel}, "market": {market}, "limit": {strconv.Itoa(limit)}}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	var page AudiencePage
	if err := c.do(ctx, http.MethodGet, AudiencePath+"?"+q.Encode(), nil, &page); err != nil {
		return AudiencePage{}, err
	}
	if page.Channel != channel || page.Market != market || len(page.Members) > limit || page.AsOf.IsZero() {
		return AudiencePage{}, fmt.Errorf("%w: response does not match the query", ErrUnavailable)
	}
	return page, nil
}

func (c *CustomerAudienceClient) Check(ctx context.Context, channel string, ids []string) (CheckResult, error) {
	if len(ids) == 0 || len(ids) > MaxBatch {
		return CheckResult{}, errors.New("eligibility batch must hold 1..500 ids")
	}
	body, _ := json.Marshal(map[string]any{"channel": channel, "customer_ids": ids})
	var res CheckResult
	if err := c.do(ctx, http.MethodPost, CheckPath, body, &res); err != nil {
		return CheckResult{}, err
	}
	if res.Channel != channel || res.AsOf.IsZero() {
		return CheckResult{}, fmt.Errorf("%w: response does not match the query", ErrUnavailable)
	}
	return res, nil
}
