package http

// Section 18: presentation-only aggregation.
//
// An aggregate saves a client round trips; it does not create meaning. It
// reads from the owning services with the caller's own credentials, places
// their answers side by side untouched, and reports which parts could not be
// fetched. It never computes, filters, joins, or decides on what it reads,
// and it never writes.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/api-gateway/internal/features/routing/models"
)

// Part is one read an aggregate composes.
type Part struct {
	Key      string
	Upstream string
	Path     string
}

// CustomerOverviewParts is the customer app's home screen: who I am and what
// my wallet holds, in one call.
var CustomerOverviewParts = []Part{
	{Key: "profile", Upstream: models.UpstreamCustomer, Path: "/api/v1/customer/me"},
	{Key: "wallet", Upstream: models.UpstreamWallet, Path: "/api/v1/payment-wallet/wallets/me"},
}

// Aggregate serves one composed read.
func (e *Edge) Aggregate(parts []Part, client *http.Client) gin.HandlerFunc {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet {
			c.AbortWithStatus(http.StatusMethodNotAllowed)
			return
		}
		correlation := correlationFor(c)
		results := make(map[string]json.RawMessage, len(parts))
		unavailable := []string{}
		var mu sync.Mutex
		var wg sync.WaitGroup
		for _, part := range parts {
			part := part
			wg.Add(1)
			go func() {
				defer wg.Done()
				data, ok := e.fetch(c.Request.Context(), client, part, c.GetHeader("Authorization"), correlation)
				mu.Lock()
				defer mu.Unlock()
				if !ok {
					unavailable = append(unavailable, part.Key)
					results[part.Key] = json.RawMessage("null")
					return
				}
				results[part.Key] = data
			}()
		}
		wg.Wait()
		data := gin.H{"unavailable": unavailable}
		for key, value := range results {
			data[key] = value
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
	}
}

// fetch reads one part and returns its data unmodified. Any failure --
// breaker open, transport, non-2xx, unreadable envelope -- makes the part
// unavailable; it never fails the aggregate, and it never substitutes a
// guessed value.
func (e *Edge) fetch(ctx context.Context, client *http.Client, part Part, authorization, correlation string) (json.RawMessage, bool) {
	upstream, ok := e.upstreams[part.Upstream]
	if !ok {
		return nil, false
	}
	if allowed, _ := upstream.Breaker.Allow(); !allowed {
		return nil, false
	}
	ctx, cancel := context.WithTimeout(ctx, upstream.Timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, upstream.Origin.String()+part.Path, nil)
	if err != nil {
		return nil, false
	}
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	request.Header.Set(HeaderRequestID, correlation)
	request.Header.Set(HeaderCorrelationID, correlation)
	response, err := client.Do(request)
	if err != nil {
		upstream.Breaker.Failure()
		return nil, false
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		upstream.Breaker.Failure()
		return nil, false
	}
	upstream.Breaker.Success()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, false
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil || json.Unmarshal(raw, &envelope) != nil || len(envelope.Data) == 0 {
		return nil, false
	}
	return envelope.Data, true
}
