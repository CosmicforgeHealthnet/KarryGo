package admincommand

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// Client is the Bridge's fixed-path caller for one owner. It never retries a
// POST: every transport, refusal or proof failure after sending is reported
// so the caller records UNKNOWN_OUTCOME and reconciles by original identity.
type Client struct {
	spec              Spec
	origin            *url.URL
	writeKey, readKey []byte
	http              *http.Client
}

func NewClient(owner, origin string, writeKey, readKey []byte) (*Client, error) {
	spec, ok := ForOwner(owner)
	u, err := url.Parse(origin)
	if !ok || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" || len(readKey) < 32 || len(writeKey) > 0 && len(writeKey) < 32 || bytes.Equal(writeKey, readKey) {
		return nil, ErrInvalid
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: time.Second}).DialContext, ResponseHeaderTimeout: 3 * time.Second, MaxConnsPerHost: 8, MaxIdleConnsPerHost: 4, IdleConnTimeout: 30 * time.Second}
	return &Client{spec: spec, origin: u, writeKey: append([]byte{}, writeKey...), readKey: append([]byte{}, readKey...), http: &http.Client{Transport: transport, Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) Spec() Spec { return c.spec }
func (c *Client) Close()     { c.http.CloseIdleConnections() }

func QueryScope(q adminread.Query) adminpolicy.Scope {
	return adminpolicy.Scope{Product: "karryam", Market: q.Scope.Market, Zone: q.Scope.Zone, Organization: q.Scope.Organization, Tenant: q.Scope.Tenant}
}

func (c *Client) do(ctx context.Context, method, path string, q adminread.Query, body, key []byte, limit int64) ([]byte, error) {
	if _, err := adminread.Parse(q.Values(), time.Now()); err != nil || q.Cursor != "" {
		return nil, ErrInvalid
	}
	u := *c.origin
	u.Path, u.RawPath, u.RawQuery = path, "", q.Values().Encode()
	request, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, ErrInvalid
	}
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	if err := serviceauth.SignRequest(request, adminread.Caller, key, body, time.Now()); err != nil {
		return nil, ErrUnavailable
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode == 404 && method == http.MethodGet:
		return nil, ErrAbsent
	case response.StatusCode == 401 || response.StatusCode == 403 || response.StatusCode == 409 || response.StatusCode == 422:
		return nil, ErrRefused
	case response.StatusCode != 200:
		return nil, ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return nil, ErrInvalid
	}
	return raw, nil
}

// Execute sends exactly one POST and verifies every echoed identity field and
// the intent digest against what was sent before accepting the receipt.
func (c *Client) Execute(ctx context.Context, q adminread.Query, cmd Command) (Receipt, error) {
	if len(c.writeKey) < 32 {
		return Receipt{}, ErrInvalid
	}
	if _, err := cmd.Validate(c.spec, time.Now()); err != nil {
		return Receipt{}, err
	}
	body, err := json.Marshal(cmd)
	if err != nil {
		return Receipt{}, ErrInvalid
	}
	raw, err := c.do(ctx, http.MethodPost, c.spec.Path(), q, body, c.writeKey, maxBody)
	if err != nil {
		return Receipt{}, err
	}
	out, err := DecodeReceipt(raw)
	if err != nil || !Matches(out, cmd, q) {
		return Receipt{}, ErrInvalid
	}
	return out, nil
}

// Matches proves a receipt is the outcome of exactly this command.
func Matches(r Receipt, cmd Command, q adminread.Query) bool {
	approval := ""
	if cmd.Approval != nil {
		approval = cmd.Approval.ApprovalID
	}
	return r.Owner != "" && r.ContractVersion == cmd.ContractVersion && r.OperationID == cmd.OperationID && r.SubjectKind == cmd.SubjectKind && r.SubjectID == cmd.SubjectID && r.CaseRef == cmd.CaseRef && r.Action == cmd.Action && r.IdempotencyKey == cmd.IdempotencyKey && r.ApprovalID == approval && r.RecordedVersion == cmd.ExpectedVersion+1 && r.IntentDigest == cmd.IntentDigest(q.Staff, q.Purpose, QueryScope(q)) && r.Scope == QueryScope(q)
}

func (c *Client) Readback(ctx context.Context, q adminread.Query, kind, id, key string) (Receipt, error) {
	if !Segment(kind) || !Segment(id) || !Segment(key) {
		return Receipt{}, ErrInvalid
	}
	raw, err := c.do(ctx, http.MethodGet, c.spec.Path()+"/"+kind+"/"+id+"/"+key, q, nil, c.readKey, maxBody)
	if err != nil {
		return Receipt{}, err
	}
	out, err := DecodeReceipt(raw)
	if err != nil || out.Owner != c.spec.Owner || out.SubjectKind != kind || out.SubjectID != id || out.IdempotencyKey != key || out.Scope != QueryScope(q) {
		return Receipt{}, ErrInvalid
	}
	return out, nil
}

func (c *Client) History(ctx context.Context, q adminread.Query, kind, id string) (History, error) {
	if !Segment(kind) || !Segment(id) {
		return History{}, ErrInvalid
	}
	raw, err := c.do(ctx, http.MethodGet, c.spec.Path()+"/"+kind+"/"+id, q, nil, c.readKey, 262144)
	if err != nil {
		return History{}, err
	}
	out, err := DecodeHistory(raw)
	if err != nil || out.Owner != c.spec.Owner || out.SubjectKind != kind || out.SubjectID != id || out.Scope != QueryScope(q) {
		return History{}, ErrInvalid
	}
	return out, nil
}
