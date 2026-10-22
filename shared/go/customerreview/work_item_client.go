package customerreview

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func (c *Client) callWorkItem(ctx context.Context, method, path string, q adminread.Query, body, key []byte) (WorkItemReceipt, error) {
	var empty WorkItemReceipt
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
	out, err := DecodeWorkItemReceipt(raw)
	if err != nil || out.Scope != queryScope(q) {
		return empty, ErrInvalid
	}
	return out, nil
}

func (c *Client) TransitionWorkItem(ctx context.Context, q adminread.Query, input WorkItemCommand) (WorkItemReceipt, error) {
	if len(c.writeKey) < 32 || input.Validate(time.Now()) != nil {
		return WorkItemReceipt{}, ErrInvalid
	}
	body, err := json.Marshal(input)
	if err != nil {
		return WorkItemReceipt{}, ErrInvalid
	}
	out, err := c.callWorkItem(ctx, http.MethodPost, WorkItemPath, q, body, c.writeKey)
	if err == nil && (out.OperationID != input.OperationID || out.ReviewRequestID != input.ReviewRequestID || out.CustomerID != input.CustomerID || out.IdempotencyKey != input.IdempotencyKey || out.RecordedVersion != input.ExpectedVersion+1 || out.IntentDigest != input.Digest(q.Staff, q.Purpose, queryScope(q)) || input.Action == "ACKNOWLEDGE" && out.State != "ACKNOWLEDGED" || input.Action == "CLOSE" && out.State != "CLOSED") {
		return WorkItemReceipt{}, ErrInvalid
	}
	return out, err
}
func (c *Client) WorkItemReadback(ctx context.Context, q adminread.Query, customer, key string) (WorkItemReceipt, error) {
	if len(c.readKey) < 32 || !CanonicalID(customer) || !Segment(key) {
		return WorkItemReceipt{}, ErrInvalid
	}
	out, err := c.callWorkItem(ctx, http.MethodGet, WorkItemPath+"/"+customer+"/"+key, q, nil, c.readKey)
	if err == nil && (out.CustomerID != customer || out.IdempotencyKey != key) {
		return WorkItemReceipt{}, ErrInvalid
	}
	return out, err
}
