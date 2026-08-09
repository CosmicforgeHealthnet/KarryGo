package dispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// HTTPOptimizer is a provider-neutral adapter. The domain sees only the
// OptimizationPort; this transport can be replaced without changing routes.
type HTTPOptimizer struct {
	baseURL string
	secret  []byte
	client  *http.Client
}

func NewHTTPOptimizer(baseURL string, secret []byte, timeout time.Duration) *HTTPOptimizer {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &HTTPOptimizer{strings.TrimRight(baseURL, "/"), secret, &http.Client{Timeout: timeout}}
}
func (c *HTTPOptimizer) Submit(ctx context.Context, input OptimizationInput) (string, bool, error) {
	raw, _ := json.Marshal(input)
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/internal/optimization/v1/routes", bytes.NewReader(raw))
	if e != nil {
		return "", false, e
	}
	req.Header.Set("Content-Type", "application/json")
	if e = serviceauth.SignRequest(req, "fleet-partner-service", c.secret, raw, time.Time{}); e != nil {
		return "", false, e
	}
	resp, e := c.client.Do(req)
	if e != nil {
		return "", true, e
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusOK {
		var out struct {
			ProviderJobReference string `json:"providerJobReference"`
			Data                 struct {
				ProviderJobReference string `json:"providerJobReference"`
			} `json:"data"`
		}
		if e = json.Unmarshal(body, &out); e != nil {
			return "", true, e
		}
		if out.ProviderJobReference == "" {
			out.ProviderJobReference = out.Data.ProviderJobReference
		}
		if out.ProviderJobReference == "" {
			return "", true, errors.New("optimization provider omitted job reference")
		}
		return out.ProviderJobReference, false, nil
	}
	if resp.StatusCode >= 500 {
		return "", true, errors.New("optimization provider outcome unknown")
	}
	return "", false, errors.New("optimization provider rejected request")
}
func (c *HTTPOptimizer) ReadBack(ctx context.Context, ref string) (OptimizationResult, bool, error) {
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/internal/optimization/v1/routes/"+url.PathEscape(ref), nil)
	if e != nil {
		return OptimizationResult{}, false, e
	}
	if e = serviceauth.SignRequest(req, "fleet-partner-service", c.secret, nil, time.Time{}); e != nil {
		return OptimizationResult{}, false, e
	}
	resp, e := c.client.Do(req)
	if e != nil {
		return OptimizationResult{}, false, e
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusAccepted {
		return OptimizationResult{}, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return OptimizationResult{}, false, errors.New("optimization read-back failed")
	}
	var envelope struct {
		Data OptimizationResult `json:"data"`
	}
	if e = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&envelope); e != nil {
		return OptimizationResult{}, false, e
	}
	return envelope.Data, true, nil
}
