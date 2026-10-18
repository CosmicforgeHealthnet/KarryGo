// Package clients contains fixed, typed owner contracts. No arbitrary proxy API.
package clients

import (
	"errors"
	"net"
	"net/http"
	"net/url"
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
