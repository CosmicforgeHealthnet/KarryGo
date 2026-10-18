package clients

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"cosmicforge/logistics/services/admin-backoffice-service/internal/security"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/google/uuid"
)

type Taxi struct {
	origin *url.URL
	secret []byte
	client *http.Client
}

func NewTaxi(origin string, secret []byte) (*Taxi, error) {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || u.RawPath != "" || len(secret) < 32 || len(secret) > 128 {
		return nil, errors.New("invalid Taxi owner configuration")
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: time.Second}).DialContext, ResponseHeaderTimeout: 2 * time.Second, MaxConnsPerHost: 8, MaxIdleConnsPerHost: 4, IdleConnTimeout: 30 * time.Second}
	return &Taxi{origin: u, secret: append([]byte(nil), secret...), client: &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (t *Taxi) Close() { t.client.CloseIdleConnections() }
func (t *Taxi) Rides(ctx context.Context, q adminread.Query, id string) (adminread.TaxiSnapshot, error) {
	var out adminread.TaxiSnapshot
	if q.Validate(time.Now()) != nil {
		return out, ErrInvalid
	}
	if id != "" {
		u, err := uuid.Parse(id)
		if err != nil || u.String() != id || q.Cursor != "" {
			return out, ErrInvalid
		}
	}
	u := *t.origin
	u.Path = adminread.TaxiPath
	if id != "" {
		u.Path += "/" + id
	}
	u.RawQuery = q.Values().Encode()
	r, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return out, ErrInvalid
	}
	if serviceauth.SignRequest(r, adminread.Caller, t.secret, nil, time.Now()) != nil {
		return out, ErrUnavailable
	}
	res, err := t.client.Do(r)
	if err != nil {
		return out, ErrUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode == 404 {
		return out, ErrNotFound
	}
	if res.StatusCode == 401 || res.StatusCode == 403 || res.StatusCode == 409 {
		return out, ErrRefused
	}
	if res.StatusCode != 200 {
		return out, ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1048577))
	if err != nil || len(raw) > 1048576 || security.Strict(raw, &out) != nil {
		return out, ErrInvalid
	}
	if out.ContractVersion != "1.0" || out.Scope != q.Scope || out.Freshness != "LIVE" || out.Coverage != "PARTIAL_COVERAGE" || out.AsOf.Before(time.Now().Add(-time.Minute)) || out.AsOf.After(time.Now().Add(time.Minute)) || out.Items == nil || out.Events == nil || len(out.Items) > q.Limit || len(out.Events) > 200 || len(out.NextCursor) > 4096 {
		return out, ErrInvalid
	}
	if id != "" && (len(out.Items) != 1 || out.Items[0].ID != id || out.NextCursor != "") {
		return out, ErrInvalid
	}
	if id == "" && (len(out.Events) != 0 || out.HistoryTruncated) {
		return out, ErrInvalid
	}
	seen := map[string]bool{}
	for _, item := range out.Items {
		ref, e := uuid.Parse(item.ID)
		if e != nil || ref.String() != item.ID || seen[item.ID] || item.CreatedAt.Before(q.Start) || !item.CreatedAt.Before(q.End) || item.UpdatedAt.Before(item.CreatedAt) || item.SourceVersion != nil || !adminread.Opaque(item.Status) || !adminread.Opaque(item.Mode) || !adminread.Opaque(item.SourceContext) {
			return out, ErrInvalid
		}
		seen[item.ID] = true
	}
	seen = map[string]bool{}
	for _, event := range out.Events {
		ref, e := uuid.Parse(event.ID)
		if e != nil || ref.String() != event.ID || seen[event.ID] || !adminread.Opaque(event.Type) || event.At.IsZero() || event.At.After(out.AsOf.Add(time.Minute)) {
			return out, ErrInvalid
		}
		seen[event.ID] = true
	}
	// Strict decoding rejects new fields instead of leaking an owner's future
	// expansion through an untyped response. No token or caller header forwarded.
	return out, nil
}

var ErrNotFound = errors.New("owner entity not found")
