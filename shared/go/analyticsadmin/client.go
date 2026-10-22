package analyticsadmin

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
	"time"

	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/networkanalytics"
	"cosmicforge/logistics/shared/go/serviceauth"
)

var ErrInvalid = errors.New("invalid scoped Analytics contract")
var ErrUnavailable = errors.New("scoped Analytics unavailable")
var ErrDenied = errors.New("scoped Analytics denied")
var ErrRate = errors.New("scoped Analytics query budget exhausted")

type Client struct {
	origin *url.URL
	secret []byte
	http   *http.Client
}

func NewClient(origin string, secret []byte) (*Client, error) {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" || len(secret) < 32 {
		return nil, ErrInvalid
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: time.Second}).DialContext, ResponseHeaderTimeout: 2 * time.Second, MaxConnsPerHost: 8, MaxIdleConnsPerHost: 4, IdleConnTimeout: 30 * time.Second}
	return &Client{origin: u, secret: append([]byte{}, secret...), http: &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) Close() { c.http.CloseIdleConnections() }
func Values(q adminread.Query, f Filters) (url.Values, error) {
	r, _ := ReportNamed("mission-success")
	return r.Values(q, f)
}
func (report Report) Values(q adminread.Query, f Filters) (url.Values, error) {
	if _, err := adminread.Parse(q.Values(), time.Now()); err != nil || q.Cursor != "" || q.Limit != 1 || !(report.Name == "readiness" && f.Authority == networkanalytics.AuthorityVendor || report.Name != "readiness" && networkanalytics.ValidAuthority(f.Authority)) || !adminread.Opaque(f.ProductCode) {
		return nil, ErrInvalid
	}
	values := q.Values()
	values.Set("authority", f.Authority)
	values.Set("product_code", f.ProductCode)
	return values, nil
}
func (c *Client) MissionSuccess(ctx context.Context, q adminread.Query, f Filters) (Release, error) {
	return c.report(ctx, q, f, "mission-success")
}
func (c *Client) report(ctx context.Context, q adminread.Query, f Filters, name string) (Release, error) {
	report, ok := ReportNamed(name)
	if !ok {
		return Release{}, ErrInvalid
	}
	var empty Release
	values, err := report.Values(q, f)
	if err != nil {
		return empty, err
	}
	u := *c.origin
	u.Path = report.Path()
	u.RawPath = ""
	u.RawQuery = values.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return empty, ErrInvalid
	}
	if err := serviceauth.SignRequest(request, adminread.Caller, c.secret, nil, time.Now()); err != nil {
		return empty, ErrUnavailable
	}
	response, err := c.http.Do(request)
	if err != nil {
		return empty, ErrUnavailable
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case 200:
	case 401, 403:
		return empty, ErrDenied
	case 429:
		return empty, ErrRate
	default:
		return empty, ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(raw) > 65536 {
		return empty, ErrInvalid
	}
	out, err := decodeReport(raw, report)
	expected := adminpolicy.Scope{Product: "karryam", Market: q.Scope.Market, Zone: q.Scope.Zone, Organization: q.Scope.Organization, Tenant: q.Scope.Tenant}
	if err != nil || out.Scope != expected || out.PolicyID != q.PolicyID || out.PolicyVersion != q.PolicyVersion || out.PolicyChecksum != q.PolicyChecksum {
		return empty, ErrInvalid
	}
	return out, nil
}
func Decode(raw []byte) (Release, error) {
	r, _ := ReportNamed("mission-success")
	return decodeReport(raw, r)
}
func decodeReport(raw []byte, report Report) (Release, error) {
	var out Release
	if len(raw) == 0 || len(raw) > 65536 {
		return out, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if unique(d, 0) != nil {
		return out, ErrInvalid
	}
	if _, err := d.Token(); err != io.EOF {
		return out, ErrInvalid
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&out) != nil {
		return Release{}, ErrInvalid
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(raw, &root) != nil || len(root) != 10 {
		return Release{}, ErrInvalid
	}
	var cells []map[string]json.RawMessage
	if json.Unmarshal(root["cells"], &cells) != nil {
		return Release{}, ErrInvalid
	}
	for _, cell := range cells {
		if len(cell) != 3 {
			return Release{}, ErrInvalid
		}
	}
	checksum, err := hex.DecodeString(out.PolicyChecksum)
	if err != nil || len(checksum) != 32 || out.Scope.Product != "karryam" || !adminread.Opaque(out.Scope.Market) || !adminread.Opaque(out.Scope.Zone) || !adminread.Opaque(out.Scope.Organization) || !adminread.Opaque(out.Scope.Tenant) {
		return Release{}, ErrInvalid
	}
	if out.Metric != report.Name || !adminread.Opaque(out.SourceVersion) || !adminread.Opaque(out.PolicyID) || out.PolicyVersion < 1 || len(out.PolicyChecksum) != 64 || out.AsOf.IsZero() || out.AsOf.After(time.Now().Add(time.Minute)) || out.Freshness != "LIVE" && out.Freshness != "STALE" || out.Coverage != "COMPLETE" && out.Coverage != "PARTIAL_COVERAGE" || len(out.Cells) != len(report.Keys) {
		return Release{}, ErrInvalid
	}
	keys := map[string]bool{}
	for _, key := range report.Keys {
		keys[key] = true
	}
	seen := map[string]bool{}
	for _, cell := range out.Cells {
		if !keys[cell.Key] || seen[cell.Key] || cell.Value != nil && *cell.Value < 0 {
			return Release{}, ErrInvalid
		}
		seen[cell.Key] = true
		switch cell.Suppression {
		case "NONE":
			if cell.Value == nil || out.Coverage != "COMPLETE" || out.Freshness != "LIVE" {
				return Release{}, ErrInvalid
			}
		case "UNKNOWN", "SMALL_CELL", "COMPLEMENTARY", "PARTIAL_COVERAGE", "STALE":
			if cell.Value != nil {
				return Release{}, ErrInvalid
			}
		default:
			return Release{}, ErrInvalid
		}
	}
	return out, nil
}

// Reject duplicates and Go JSON case aliases before ordinary typed decoding.
func unique(d *json.Decoder, depth int) error {
	if depth > 8 {
		return ErrInvalid
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	switch token {
	case json.Delim('{'):
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return ErrInvalid
			}
			for _, r := range name {
				if r >= 'A' && r <= 'Z' {
					return ErrInvalid
				}
			}
			seen[name] = true
			if err := unique(d, depth+1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim('}') {
			return ErrInvalid
		}
	case json.Delim('['):
		for d.More() {
			if err := unique(d, depth+1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(']') {
			return ErrInvalid
		}
	case json.Delim('}'), json.Delim(']'):
		return ErrInvalid
	}
	return nil
}

func (c *Client) MissionOutcomes(ctx context.Context, q adminread.Query, f Filters) (Release, error) {
	return c.report(ctx, q, f, "mission-outcomes")
}
func (c *Client) Recovery(ctx context.Context, q adminread.Query, f Filters) (Release, error) {
	return c.report(ctx, q, f, "recovery")
}
func (c *Client) CapabilityMismatches(ctx context.Context, q adminread.Query, f Filters) (Release, error) {
	return c.report(ctx, q, f, "capability-mismatches")
}
func (c *Client) Declines(ctx context.Context, q adminread.Query, f Filters) (Release, error) {
	return c.report(ctx, q, f, "declines")
}
func (c *Client) Demand(ctx context.Context, q adminread.Query, f Filters) (Release, error) {
	return c.report(ctx, q, f, "demand")
}
func (c *Client) DemandForecast(ctx context.Context, q adminread.Query, f Filters) (Release, error) {
	return c.report(ctx, q, f, "demand-forecast")
}
func (c *Client) Supply(ctx context.Context, q adminread.Query, f Filters) (Release, error) {
	return c.report(ctx, q, f, "supply")
}
func (c *Client) ProviderQuality(ctx context.Context, q adminread.Query, f Filters) (Release, error) {
	return c.report(ctx, q, f, "provider-quality")
}
func (c *Client) Readiness(ctx context.Context, q adminread.Query, f Filters) (Release, error) {
	return c.report(ctx, q, f, "readiness")
}
