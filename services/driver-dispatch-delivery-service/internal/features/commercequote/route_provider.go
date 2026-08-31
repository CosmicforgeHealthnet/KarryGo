package commercequote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/dispatchcommerce"
)

var ErrRouteServiceUnavailable = errors.New("commercequote: route service unavailable")

type RoutePoint struct {
	Ref       string  `json:"ref"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type RoutePlan struct {
	Provider        string   `json:"provider"`
	EvidenceVersion string   `json:"evidence_version"`
	DistanceMeters  int64    `json:"distance_meters"`
	DurationSeconds int64    `json:"duration_seconds"`
	OrderedRefs     []string `json:"ordered_refs"`
}

type RoutePlanner interface {
	Plan(ctx context.Context, points []RoutePoint, mode dispatchcommerce.Mode) (RoutePlan, error)
}

// HTTPRoutePlanner is the narrow adapter to the platform route service. There
// is intentionally no Haversine fallback: a route-provider fault is reported
// as unavailable instead of being disguised as authoritative road distance.
type HTTPRoutePlanner struct {
	baseURL string
	http    *http.Client
}

func NewHTTPRoutePlanner(baseURL string, timeout time.Duration) *HTTPRoutePlanner {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &HTTPRoutePlanner{baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), http: &http.Client{Timeout: timeout}}
}

func (p *HTTPRoutePlanner) Configured() bool { return p != nil && p.baseURL != "" && p.http != nil }

func (p *HTTPRoutePlanner) Plan(ctx context.Context, points []RoutePoint, mode dispatchcommerce.Mode) (RoutePlan, error) {
	if !p.Configured() {
		return RoutePlan{}, ErrRouteServiceUnavailable
	}
	body, err := json.Marshal(map[string]any{"points": points, "mode": mode, "final_point_fixed": true})
	if err != nil {
		return RoutePlan{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/routes/multi-stop", bytes.NewReader(body))
	if err != nil {
		return RoutePlan{}, ErrRouteServiceUnavailable
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := p.http.Do(request)
	if err != nil {
		return RoutePlan{}, fmt.Errorf("%w: %v", ErrRouteServiceUnavailable, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return RoutePlan{}, ErrRouteServiceUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 128<<10))
	if err != nil {
		return RoutePlan{}, ErrRouteServiceUnavailable
	}
	var envelope struct {
		Success bool      `json:"success"`
		Data    RoutePlan `json:"data"`
	}
	if err = json.Unmarshal(raw, &envelope); err != nil || !envelope.Success {
		return RoutePlan{}, ErrRouteServiceUnavailable
	}
	plan := envelope.Data
	if plan.DistanceMeters <= 0 || plan.DurationSeconds <= 0 || len(plan.OrderedRefs) != len(points) ||
		strings.TrimSpace(plan.Provider) == "" || strings.TrimSpace(plan.EvidenceVersion) == "" {
		return RoutePlan{}, ErrRouteServiceUnavailable
	}
	seen := map[string]bool{}
	for _, ref := range plan.OrderedRefs {
		if seen[ref] {
			return RoutePlan{}, ErrRouteServiceUnavailable
		}
		seen[ref] = true
	}
	for _, point := range points {
		if !seen[point.Ref] {
			return RoutePlan{}, ErrRouteServiceUnavailable
		}
	}
	if plan.OrderedRefs[len(plan.OrderedRefs)-1] != points[len(points)-1].Ref {
		return RoutePlan{}, ErrRouteServiceUnavailable
	}
	return plan, nil
}
