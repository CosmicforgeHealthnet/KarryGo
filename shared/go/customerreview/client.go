package customerreview

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
)

var ErrUnavailable = errors.New("Customer review outcome unavailable; use original-identity readback")
var ErrRefused = errors.New("Customer review request refused")
var ErrAbsent = errors.New("Customer review outcome not yet found; not proof of rejection")

type Client struct {
	origin            *url.URL
	writeKey, readKey []byte
	http              *http.Client
}

func NewClient(origin string, writeKey, readKey []byte) (*Client, error) {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" || len(writeKey) > 0 && len(writeKey) < 32 || len(readKey) > 0 && len(readKey) < 32 || len(writeKey) == 0 && len(readKey) == 0 || len(writeKey) > 0 && bytes.Equal(writeKey, readKey) {
		return nil, ErrInvalid
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: time.Second}).DialContext, ResponseHeaderTimeout: 2 * time.Second, MaxConnsPerHost: 8, MaxIdleConnsPerHost: 4, IdleConnTimeout: 30 * time.Second}
	return &Client{origin: u, writeKey: append([]byte{}, writeKey...), readKey: append([]byte{}, readKey...), http: &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) Close() { c.http.CloseIdleConnections() }
func queryScope(q adminread.Query) adminpolicy.Scope {
	return adminpolicy.Scope{Product: "karryam", Market: q.Scope.Market, Zone: q.Scope.Zone, Organization: q.Scope.Organization, Tenant: q.Scope.Tenant}
}
func (c *Client) Request(ctx context.Context, q adminread.Query, input Request) (Receipt, error) {
	var empty Receipt
	if len(c.writeKey) < 32 || input.Validate(time.Now()) != nil {
		return empty, ErrInvalid
	}
	body, err := json.Marshal(input)
	if err != nil {
		return empty, ErrInvalid
	}
	out, err := c.call(ctx, http.MethodPost, Path, q, body, c.writeKey)
	if err != nil {
		return empty, err
	}
	if out.RequestID != input.RequestID || out.CustomerID != input.CustomerID || out.IdempotencyKey != input.IdempotencyKey || out.RecordedReviewRevision != input.ExpectedReviewRevision+1 || out.IntentDigest != input.Digest(q.Staff, q.Purpose, queryScope(q)) {
		return empty, ErrInvalid
	}
	return out, nil
}

// Readback is GET-only. Absence, timeout and malformed proof never issue a POST
// or authorize a fresh key, status change, account restriction or fulfillment.
func (c *Client) Readback(ctx context.Context, q adminread.Query, customer, key string) (Receipt, error) {
	var empty Receipt
	if len(c.readKey) < 32 || !CanonicalID(customer) || !Segment(key) {
		return empty, ErrInvalid
	}
	out, err := c.call(ctx, http.MethodGet, Path+"/"+customer+"/"+key, q, nil, c.readKey)
	if err == nil && (out.CustomerID != customer || out.IdempotencyKey != key) {
		return empty, ErrInvalid
	}
	return out, err
}
func (c *Client) call(ctx context.Context, method, path string, q adminread.Query, body, key []byte) (Receipt, error) {
	var empty Receipt
	if _, err := adminread.Parse(q.Values(), time.Now()); err != nil || q.Cursor != "" {
		return empty, ErrInvalid
	}
	u := *c.origin
	u.Path = path
	u.RawPath = ""
	u.RawQuery = q.Values().Encode()
	request, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return empty, ErrInvalid
	}
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	// The stable operation key is in the signed body, never an HTTP retry hint.
	if err := serviceauth.SignRequest(request, adminread.Caller, key, body, time.Now()); err != nil {
		return empty, ErrUnavailable
	}
	response, err := c.http.Do(request)
	if err != nil {
		return empty, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode == 404 && method == http.MethodGet {
		return empty, ErrAbsent
	}
	if response.StatusCode == 401 || response.StatusCode == 403 || response.StatusCode == 409 {
		return empty, ErrRefused
	}
	if response.StatusCode != 200 {
		return empty, ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 16385))
	if err != nil || len(raw) > 16384 {
		return empty, ErrInvalid
	}
	out, err := decodeReceipt(raw)
	if err != nil || out.Scope != queryScope(q) {
		return empty, ErrInvalid
	}
	return out, nil
}
func decodeReceipt(raw []byte) (Receipt, error) {
	var empty Receipt
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var object func(int) error
	object = func(depth int) error {
		if depth > 2 {
			return ErrInvalid
		}
		token, err := decoder.Token()
		if err != nil || token != json.Delim('{') {
			return ErrInvalid
		}
		seen := map[string]bool{}
		for decoder.More() {
			token, err = decoder.Token()
			if err != nil {
				return ErrInvalid
			}
			name, ok := token.(string)
			if !ok || name != strings.ToLower(name) || seen[name] {
				return ErrInvalid
			}
			seen[name] = true
			if name == "scope" && depth == 1 {
				if err := object(depth + 1); err != nil {
					return err
				}
			} else {
				var value json.RawMessage
				if decoder.Decode(&value) != nil {
					return ErrInvalid
				}
			}
		}
		_, err = decoder.Token()
		return err
	}
	if object(1) != nil {
		return empty, ErrInvalid
	}
	if _, err := decoder.Token(); err != io.EOF {
		return empty, ErrInvalid
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var out Receipt
	if decoder.Decode(&out) != nil || out.ContractVersion != Contract || out.Status != Recorded || !CanonicalID(out.ReviewRequestID) || !CanonicalID(out.RequestID) || !CanonicalID(out.CustomerID) || !Segment(out.IdempotencyKey) || out.RecordedReviewRevision < 2 || out.RecordedAt.IsZero() || out.RecordedAt.After(time.Now().Add(time.Minute)) {
		return empty, ErrInvalid
	}
	digest, err := hex.DecodeString(out.IntentDigest)
	if err != nil || len(digest) != 32 || hex.EncodeToString(digest) != out.IntentDigest {
		return empty, ErrInvalid
	}
	return out, nil
}
