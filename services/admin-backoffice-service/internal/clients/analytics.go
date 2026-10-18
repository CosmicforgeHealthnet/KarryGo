// Package clients contains fixed, typed owner contracts. No arbitrary proxy API.
package clients

import (
	"context"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/config"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/security"
	"cosmicforge/logistics/shared/go/networkanalytics"
	"cosmicforge/logistics/shared/go/serviceauth"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

var ErrUnavailable = errors.New("owner unavailable")
var ErrRefused = errors.New("owner refused request")
var ErrInvalid = errors.New("invalid owner response")

type Analytics struct {
	origin *url.URL
	secret []byte
	client *http.Client
}
type MissionSuccessDay struct {
	Authority         string    `json:"authority"`
	Date              time.Time `json:"date"`
	Completed         int64     `json:"completed"`
	Failed            int64     `json:"failed"`
	Cancelled         int64     `json:"cancelled"`
	GuaranteeBreaches int64     `json:"guarantee_breaches"`
	SuccessRate       float64   `json:"success_rate"`
}

// Analytics is deliberately NOT exposed by the remote bridge: the existing
// owner aggregate lacks market/tenant filters. Registering it remotely requires
// an owner-approved scoped contract and authorization tests first.
func NewAnalytics(origin string, dedicatedSecret []byte) (*Analytics, error) {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || len(dedicatedSecret) < 32 {
		return nil, errors.New("invalid analytics client configuration")
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: time.Second}).DialContext, ResponseHeaderTimeout: 2 * time.Second, MaxConnsPerHost: 8, MaxIdleConnsPerHost: 4, IdleConnTimeout: 30 * time.Second}
	return &Analytics{origin: u, secret: append([]byte(nil), dedicatedSecret...), client: &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (a *Analytics) Close() { a.client.CloseIdleConnections() }
func (a *Analytics) MissionSuccess(ctx context.Context, authority string, days int, correlation string) ([]MissionSuccessDay, error) {
	if !security.Opaque(correlation) || days < 1 || days > 90 || !networkanalytics.ValidAuthority(authority) {
		return nil, ErrInvalid
	}
	u := *a.origin
	u.Path = "/api/v1/analytics/network/mission-success"
	u.RawQuery = url.Values{"authority": {authority}, "days": {strconv.Itoa(days)}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, ErrInvalid
	}
	req.Header.Set("X-Correlation-ID", correlation)
	if serviceauth.SignRequest(req, config.Caller, a.secret, nil, time.Now()) != nil {
		return nil, ErrUnavailable
	}
	// One attempt only. Owner commands will require separate typed adapters;
	// timeout/transport errors must never imply safe mutation replay.
	res, err := a.client.Do(req)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode == 401 || res.StatusCode == 403 || res.StatusCode == 409 {
		return nil, ErrRefused
	}
	if res.StatusCode != 200 {
		return nil, ErrUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 1048577))
	if err != nil || len(data) > 1048576 {
		return nil, ErrInvalid
	}
	var envelope struct {
		Success bool                `json:"success"`
		Data    []MissionSuccessDay `json:"data"`
	}
	if json.Unmarshal(data, &envelope) != nil || !envelope.Success || envelope.Data == nil {
		return nil, ErrInvalid
	}
	// Typed decoding minimizes fields. This contract carries no source version
	// or authoritative data-as-of marker; callers must report freshness UNKNOWN.
	return envelope.Data, nil
}
