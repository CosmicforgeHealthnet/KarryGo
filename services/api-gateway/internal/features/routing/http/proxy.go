// Package http is the Section 18 edge: a pass-through reverse proxy onto the
// platform's canonical services.
package http

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/api-gateway/internal/features/routing/models"
	"cosmicforge/logistics/services/api-gateway/internal/features/routing/usecases"
	"cosmicforge/logistics/shared/go/httpx"
)

// Correlation headers the edge guarantees on every forwarded request.
const (
	HeaderRequestID     = "X-Request-ID"
	HeaderCorrelationID = "X-Correlation-ID"
	HeaderScope         = "X-Gateway-Scope"
)

// Upstream is one canonical service the edge forwards to.
type Upstream struct {
	Name    string
	Origin  *url.URL
	Timeout time.Duration
	Breaker *usecases.Breaker
	proxy   *httputil.ReverseProxy
}

// Options configures the edge.
type Options struct {
	Routes []models.Route
	// Origins maps an upstream name to its bare origin. An upstream with no
	// origin is simply not routed: its paths answer 404, as they do today.
	Origins          map[string]string
	Timeout          time.Duration
	BreakerThreshold int
	BreakerCooldown  time.Duration
	Transport        http.RoundTripper
}

// Edge is the proxy.
type Edge struct {
	table     models.Table
	upstreams map[string]*Upstream
}

// NewEdge builds the edge. It returns an error only for an origin that is not
// a valid absolute URL -- a misconfiguration worth refusing to start over.
func NewEdge(options Options) (*Edge, error) {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	edge := &Edge{table: models.NewTable(options.Routes), upstreams: map[string]*Upstream{}}
	for name, raw := range options.Origins {
		// Admin is never an edge destination, even under custom route tables.
		if name == models.UpstreamAdmin {
			continue
		}
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		origin, err := url.Parse(raw)
		if err != nil || origin.Scheme == "" || origin.Host == "" {
			return nil, errors.New("upstream " + name + " has an invalid origin")
		}
		upstream := &Upstream{Name: name, Origin: origin, Timeout: timeout,
			Breaker: usecases.NewBreaker(options.BreakerThreshold, options.BreakerCooldown)}
		upstream.proxy = edge.reverseProxy(upstream, options.Transport)
		edge.upstreams[name] = upstream
	}
	return edge, nil
}

// Upstream returns a configured upstream, for health reporting.
func (e *Edge) Upstream(name string) (*Upstream, bool) {
	upstream, ok := e.upstreams[name]
	return upstream, ok
}

// Handle forwards one request, or answers it at the edge.
//
// It is mounted as the router's fallback, so every route the gateway owns
// itself -- health, the partner API, its own callbacks -- is matched first
// and never reaches here.
func (e *Edge) Handle(c *gin.Context) {
	path := c.Request.URL.Path
	if models.IsInternalPath(path) {
		// Not "forbidden": the edge does not admit these routes exist.
		notFound(c)
		return
	}
	route, ok := e.table.Match(path)
	if !ok || route.Upstream == models.UpstreamAdmin {
		notFound(c)
		return
	}
	if requestsUpgrade(c.Request) && !upgradeAllowed(c.Request, route) {
		// A protocol switch is only ever carried to an allowlisted websocket
		// endpoint. Anything else is refused here, before an upstream (or
		// its breaker) is touched, exactly like an unknown path.
		notFound(c)
		return
	}
	upstream, ok := e.upstreams[route.Upstream]
	if !ok {
		notFound(c)
		return
	}
	if allowed, retryAfter := upstream.Breaker.Allow(); !allowed {
		seconds := int(retryAfter.Seconds()) + 1
		c.Header("Retry-After", strconv.Itoa(seconds))
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, envelope(c, "upstream_unavailable",
			"This service is temporarily unavailable. Please retry shortly."))
		return
	}

	request := c.Request
	if !isUpgrade(request) {
		// A websocket stays open for as long as the user does; only ordinary
		// requests get a deadline.
		ctx, cancel := context.WithTimeout(request.Context(), upstream.Timeout)
		defer cancel()
		request = request.WithContext(ctx)
	}
	request = request.WithContext(context.WithValue(request.Context(), scopeKey{}, route.Scope))
	request = request.WithContext(context.WithValue(request.Context(), correlationKey{}, correlationFor(c)))
	upstream.proxy.ServeHTTP(c.Writer, request)
	c.Abort()
}

type scopeKey struct{}
type correlationKey struct{}

// untrustedHeaderPrefixes can only ever be asserted by the platform itself.
// A client that sends one is either confused or attempting to impersonate a
// service; either way it is dropped before forwarding, and the edge sets the
// ones it owns afresh.
var untrustedHeaderPrefixes = []string{"x-service-", "x-internal-", "x-gateway-", "x-forwarded-", "x-hq-", "x-admin-", "x-delegated-"}
var untrustedHeaders = map[string]bool{"forwarded": true, "x-real-ip": true, "x-analytics-business-id": true}

func (e *Edge) reverseProxy(upstream *Upstream, transport http.RoundTripper) *httputil.ReverseProxy {
	if transport == nil {
		transport = &http.Transport{
			Proxy:                 nil,
			DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			ResponseHeaderTimeout: upstream.Timeout,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConnsPerHost:   64,
		}
	}
	return &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(r *httputil.ProxyRequest) {
			// Path and body are forwarded untouched: the upstream is the
			// authority on both, and a payment webhook's signature is over the
			// exact bytes sent.
			r.SetURL(upstream.Origin)
			r.Out.Host = upstream.Origin.Host
			for name := range r.Out.Header {
				lowered := strings.ToLower(name)
				if untrustedHeaders[lowered] {
					r.Out.Header.Del(name)
					continue
				}
				for _, prefix := range untrustedHeaderPrefixes {
					if strings.HasPrefix(lowered, prefix) {
						r.Out.Header.Del(name)
						break
					}
				}
			}
			r.SetXForwarded()
			// The inbound Host is client-chosen (including absolute-form
			// request targets) and no upstream needs it. Never relay it as a
			// platform-asserted header.
			r.Out.Header.Del("X-Forwarded-Host")
			correlation, _ := r.In.Context().Value(correlationKey{}).(string)
			if correlation != "" {
				r.Out.Header.Set(HeaderRequestID, correlation)
				r.Out.Header.Set(HeaderCorrelationID, correlation)
			}
			if scope, ok := r.In.Context().Value(scopeKey{}).(models.Scope); ok {
				r.Out.Header.Set(HeaderScope, string(scope))
			}
		},
		ModifyResponse: func(response *http.Response) error {
			switch response.StatusCode {
			case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
				upstream.Breaker.Failure()
			default:
				upstream.Breaker.Success()
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			upstream.Breaker.Failure()
			status, code, message := http.StatusBadGateway, "upstream_unreachable", "This service could not be reached."
			if errors.Is(err, context.DeadlineExceeded) {
				status, code, message = http.StatusGatewayTimeout, "upstream_timeout", "This service did not answer in time."
			}
			correlation, _ := r.Context().Value(correlationKey{}).(string)
			writeJSON(w, status, code, message, correlation)
		},
	}
}

// correlationFor keeps a caller's correlation id when it is well formed, so
// one trace spans the client and every service; otherwise it is the edge's
// own request id.
func correlationFor(c *gin.Context) string {
	if inbound := c.GetHeader(HeaderCorrelationID); wellFormed(inbound) {
		return inbound
	}
	if id, ok := c.Get(httpx.RequestIDKey); ok {
		if value, isString := id.(string); isString {
			return value
		}
	}
	return c.GetHeader(HeaderRequestID)
}

func wellFormed(id string) bool {
	return id != "" && len(id) <= 128 && strings.IndexFunc(id, func(r rune) bool { return r < 33 || r > 126 }) < 0
}

func isUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// requestsUpgrade reports any attempt to switch protocols: an Upgrade header
// of any value (websocket, h2c, ...) or an "upgrade" Connection token.
func requestsUpgrade(r *http.Request) bool {
	if len(r.Header.Values("Upgrade")) > 0 {
		return true
	}
	for _, value := range r.Header.Values("Connection") {
		for _, token := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
				return true
			}
		}
	}
	return false
}

// upgradeAllowed admits exactly one shape: GET, a single "websocket" Upgrade
// value, on an allowlisted endpoint owned by the route's upstream.
func upgradeAllowed(r *http.Request, route models.Route) bool {
	if r.Method != http.MethodGet {
		return false
	}
	if values := r.Header.Values("Upgrade"); len(values) != 1 || !strings.EqualFold(strings.TrimSpace(values[0]), "websocket") {
		return false
	}
	if r.URL.RawPath != "" && r.URL.RawPath != r.URL.Path {
		return false // encoded characters in a websocket path are never legitimate
	}
	endpoint, ok := models.WebsocketEndpointFor(r.URL.Path)
	return ok && endpoint.Upstream == route.Upstream
}

func notFound(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusNotFound, envelope(c, "not_found", "The requested resource was not found."))
}

func envelope(c *gin.Context, code, message string) gin.H {
	return gin.H{"success": false, "error": gin.H{"code": code, "message": message, "request_id": correlationFor(c)}}
}

func writeJSON(w http.ResponseWriter, status int, code, message, requestID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"success":false,"error":{"code":"` + code + `","message":"` + message +
		`","request_id":"` + strings.ReplaceAll(requestID, `"`, "") + `"}}`))
}
