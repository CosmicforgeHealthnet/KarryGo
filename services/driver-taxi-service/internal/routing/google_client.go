package routing

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/geo"
	sharedrouting "cosmicforge/logistics/shared/go/routing"
)

// defaultMatrixConcurrency bounds how many Route calls run in parallel for
// one Matrix request, so a large candidate set cannot open hundreds of
// simultaneous connections to the Routes API at once.
const defaultMatrixConcurrency = 8

// GoogleRoutingClient adapts shared/go/routing.Client to the RoutingClient
// interface. Route is reused unchanged from that package. Matrix has no
// equivalent there yet, so it is implemented here as a bounded-concurrency
// fan-out of Route calls rather than a Google Routes Matrix API integration
// -- extending the shared package to wrap that batch endpoint is out of scope
// here, since shared/go/routing is also used by the hauling and dispatch
// services and changing its contract is a larger, separate decision.
type GoogleRoutingClient struct {
	fetcher           sharedrouting.Fetcher
	matrixConcurrency int
}

// NewGoogleRoutingClient wraps a shared/go/routing.Fetcher. Passing a nil
// *routing.Client (no API key configured) is valid -- every Route call then
// returns sharedrouting.ErrNotConfigured, matching that package's own
// "optional by design" contract, and Matrix degrades the same way per-cell.
func NewGoogleRoutingClient(fetcher sharedrouting.Fetcher) *GoogleRoutingClient {
	return &GoogleRoutingClient{fetcher: fetcher, matrixConcurrency: defaultMatrixConcurrency}
}

func (c *GoogleRoutingClient) Route(ctx context.Context, origin, destination geo.Point) (RouteResult, error) {
	route, err := c.fetcher.Route(ctx, origin, destination, sharedrouting.TravelModeDrive)
	if err != nil {
		return RouteResult{}, err
	}
	return RouteResult{
		DistanceM: route.DistanceM,
		DurationS: route.DurationS,
		// TrafficDurationS stays 0 (unknown): shared/go/routing.Route does not
		// currently carry a separate traffic-adjusted duration field. Never
		// fabricated from DurationS -- addendum §8's own table requires poor or
		// absent signals to be marked, not silently substituted.
		Polyline:    route.Polyline,
		Confidence:  1.0,
		Provider:    "google-routes",
		GeneratedAt: time.Now(),
		RequestID:   uuid.NewString(),
	}, nil
}

// Matrix fans Route out across every (origin, destination) pair, bounded by
// matrixConcurrency. A single leg failing (a bad coordinate, a transient
// blip) does not fail the whole matrix -- that cell comes back as an
// explicitly unusable placeholder (Provider "unavailable", Confidence 0) so a
// scoring loop over candidates simply skips that one candidate. Only a
// complete outage -- every single leg failing -- returns a top-level error,
// since a matrix with zero usable cells is not a partial result worth
// returning.
func (c *GoogleRoutingClient) Matrix(ctx context.Context, origins, destinations []geo.Point) (MatrixResult, error) {
	if err := ctx.Err(); err != nil {
		return MatrixResult{}, err
	}
	if len(origins) == 0 || len(destinations) == 0 {
		return MatrixResult{Rows: make([][]RouteResult, len(origins))}, nil
	}

	rows := make([][]RouteResult, len(origins))
	for i := range rows {
		rows[i] = make([]RouteResult, len(destinations))
	}

	type cellResult struct {
		i, j   int
		result RouteResult
		err    error
	}
	total := len(origins) * len(destinations)
	results := make(chan cellResult, total)
	sem := make(chan struct{}, c.matrixConcurrency)
	var wg sync.WaitGroup

	for i, origin := range origins {
		for j, dest := range destinations {
			wg.Add(1)
			go func(i, j int, origin, dest geo.Point) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				result, err := c.Route(ctx, origin, dest)
				results <- cellResult{i: i, j: j, result: result, err: err}
			}(i, j, origin, dest)
		}
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	failures := 0
	for cr := range results {
		if cr.err != nil {
			failures++
			rows[cr.i][cr.j] = RouteResult{Provider: "unavailable", Confidence: 0, GeneratedAt: time.Now()}
			continue
		}
		rows[cr.i][cr.j] = cr.result
	}

	if failures == total {
		return MatrixResult{}, fmt.Errorf("routing: matrix request failed for all %d legs", total)
	}
	return MatrixResult{Rows: rows}, nil
}

// SnapToRoad has no P0 caller (see routing.go's ErrNotImplemented doc).
func (c *GoogleRoutingClient) SnapToRoad(context.Context, []geo.Point) (SnappedRoute, error) {
	return SnappedRoute{}, ErrNotImplemented
}

// ReverseGeocode has no P0 caller (see routing.go's ErrNotImplemented doc).
func (c *GoogleRoutingClient) ReverseGeocode(context.Context, float64, float64) (AddressResult, error) {
	return AddressResult{}, ErrNotImplemented
}
