package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"cosmicforge/logistics/shared/go/admincampaign"
	"cosmicforge/logistics/shared/go/financereport"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/google/uuid"
)

// ErrOwnerRefused carries an owner's deliberate 4xx refusal (no retry loop).
var ErrOwnerRefused = errors.New("owner refused request")
var ErrOwnerConflict = errors.New("owner reported conflicting identity")

// fixedClient is a typed HMAC client bound to one owner origin. Paths are
// compiled constants; no caller-chosen URL, host, method or service name.
type fixedClient struct {
	origin *url.URL
	secret []byte
	http   *http.Client
}

func newFixed(origin string, secret []byte) (*fixedClient, error) {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawPath != "" || u.RawQuery != "" || u.Fragment != "" || len(secret) < 32 || len(secret) > 128 {
		return nil, ErrInvalid
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: time.Second}).DialContext, ResponseHeaderTimeout: 6 * time.Second, MaxConnsPerHost: 8, MaxIdleConnsPerHost: 4, IdleConnTimeout: 30 * time.Second}
	return &fixedClient{origin: u, secret: append([]byte(nil), secret...), http: &http.Client{Transport: transport, Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (c *fixedClient) Close() { c.http.CloseIdleConnections() }

// do returns (status, body). A transport failure after send is reported as
// ErrUnavailable so callers treat the outcome as UNKNOWN and read back.
func (c *fixedClient) do(ctx context.Context, method, path string, query url.Values, body []byte) (int, []byte, error) {
	target := *c.origin
	target.Path = path
	target.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, method, target.String(), bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set(serviceauth.HeaderNonce, uuid.NewString())
	if err = serviceauth.SignRequest(req, "admin-backoffice-service", c.secret, body, time.Now().UTC()); err != nil {
		return 0, nil, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return 0, nil, ErrUnavailable
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return 0, nil, ErrUnavailable
	}
	return res.StatusCode, raw, nil
}

func strictJSON(raw []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(out) != nil {
		return ErrInvalid
	}
	if _, err := dec.Token(); err != io.EOF {
		return ErrInvalid
	}
	return nil
}

// Campaigns is the fixed notification-service campaign client.
type Campaigns struct{ *fixedClient }

func NewCampaigns(origin string, secret []byte) (*Campaigns, error) {
	c, err := newFixed(origin, secret)
	if err != nil {
		return nil, err
	}
	return &Campaigns{c}, nil
}

func campaignStatus(status int, raw []byte) (admincampaign.Status, error) {
	var out admincampaign.Status
	switch {
	case status == 200 || status == 201 || status == 202:
		var envelope struct {
			Data admincampaign.Status `json:"data"`
		}
		if json.Unmarshal(raw, &envelope) == nil && envelope.Data.Contract != "" {
			out = envelope.Data
		} else if strictJSON(raw, &out) != nil {
			return out, ErrInvalid
		}
		if out.Contract != admincampaign.Contract {
			return out, ErrInvalid
		}
		return out, nil
	case status == 404:
		return out, ErrNotFound
	case status == 409:
		return out, ErrOwnerConflict
	case status >= 400 && status < 500:
		return out, ErrOwnerRefused
	}
	return out, ErrUnavailable
}

func (c *Campaigns) Submit(ctx context.Context, intent admincampaign.Intent) (admincampaign.Status, error) {
	body, err := json.Marshal(intent)
	if err != nil {
		return admincampaign.Status{}, ErrInvalid
	}
	code, raw, err := c.do(ctx, http.MethodPost, admincampaign.IntentPath, url.Values{}, body)
	if err != nil {
		return admincampaign.Status{}, err
	}
	return campaignStatus(code, raw)
}

func (c *Campaigns) Status(ctx context.Context, campaignID string, revision int64) (admincampaign.Status, error) {
	if _, err := uuid.Parse(campaignID); err != nil || revision < 1 {
		return admincampaign.Status{}, ErrInvalid
	}
	code, raw, err := c.do(ctx, http.MethodGet, admincampaign.IntentPath+"/"+campaignID+"/revisions/"+strconv.FormatInt(revision, 10), url.Values{}, nil)
	if err != nil {
		return admincampaign.Status{}, err
	}
	return campaignStatus(code, raw)
}

func (c *Campaigns) Cancel(ctx context.Context, cancel admincampaign.Cancellation) (admincampaign.Status, error) {
	if cancel.Validate() != nil {
		return admincampaign.Status{}, ErrInvalid
	}
	body, _ := json.Marshal(cancel)
	code, raw, err := c.do(ctx, http.MethodPost, admincampaign.IntentPath+"/"+cancel.CampaignID+"/cancellations", url.Values{}, body)
	if err != nil {
		return admincampaign.Status{}, err
	}
	return campaignStatus(code, raw)
}

// Finance is the fixed payment-wallet finance reporting client (read-only).
type Finance struct{ *fixedClient }

func NewFinance(origin string, secret []byte) (*Finance, error) {
	c, err := newFixed(origin, secret)
	if err != nil {
		return nil, err
	}
	return &Finance{c}, nil
}

func (c *Finance) Report(ctx context.Context, start, end time.Time, timezone string) (financereport.Report, error) {
	var out financereport.Report
	q := url.Values{"period_start": {start.UTC().Format(time.RFC3339)}, "period_end": {end.UTC().Format(time.RFC3339)}, "timezone": {timezone}}
	code, raw, err := c.do(ctx, http.MethodGet, financereport.Path, q, nil)
	if err != nil {
		return out, err
	}
	if code != 200 {
		if code >= 400 && code < 500 {
			return out, ErrOwnerRefused
		}
		return out, ErrUnavailable
	}
	var envelope struct {
		Data financereport.Report `json:"data"`
	}
	if json.Unmarshal(raw, &envelope) == nil && envelope.Data.Contract != "" {
		out = envelope.Data
	} else if strictJSON(raw, &out) != nil {
		return out, ErrInvalid
	}
	if out.Validate() != nil {
		return out, ErrInvalid
	}
	return out, nil
}
