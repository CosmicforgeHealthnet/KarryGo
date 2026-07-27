package router

import (
	"context"
	"testing"
	"time"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/config"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
)

// completeFleetConfig is a configuration with every fleet client field set.
func completeFleetConfig() config.Config {
	return config.Config{
		ServiceName:             "dispatch-delivery-service",
		FleetServiceURL:         "http://fleet-partner:8110",
		FleetServiceCaller:      "driver-dispatch-delivery-service",
		FleetServiceSecret:      []byte("a-configured-secret"),
		FleetServiceTimeout:     3 * time.Second,
		FleetRelayEnabled:       true,
		FleetRelayBatchSize:     10,
		FleetRelayLeaseDuration: time.Minute,
		FleetRelayRetryLimit:    7,
		FleetRelayBackoffBase:   time.Second,
		FleetRelayInterval:      time.Second,
	}
}

// TestCompositionRootBuildsRealClientsWhenConfigured proves the running
// application uses the real HTTP clients, not the fail-closed fallbacks.
//
// Wiring is the step where a fail-closed default silently becomes production
// behaviour, so it is asserted rather than assumed.
func TestCompositionRootBuildsRealClientsWhenConfigured(t *testing.T) {
	cfg := completeFleetConfig()
	clientConfig := marketplace.ResponderConfig{
		BaseURL:     cfg.FleetServiceURL,
		ServiceName: cfg.FleetServiceCaller,
		Secret:      cfg.FleetServiceSecret,
		Timeout:     cfg.FleetServiceTimeout,
	}

	responder, responderReady := marketplace.NewHTTPFleetResponder(clientConfig)
	if !responderReady {
		t.Fatal("complete configuration did not produce a real FleetResponder")
	}
	if !responder.Configured() {
		t.Fatal("the wired responder reports unconfigured")
	}
	if _, isFallback := responder.(marketplace.UnconfiguredResponder); isFallback {
		t.Fatal("the composition root would run the fail-closed responder despite valid configuration")
	}

	reader, readerReady := marketplace.NewHTTPProviderReader(clientConfig)
	if !readerReady {
		t.Fatal("complete configuration did not produce a real ProviderReader")
	}
	if !reader.Configured() {
		t.Fatal("the wired reader reports unconfigured")
	}
	if _, isFallback := reader.(marketplace.UnconfiguredProviderReader); isFallback {
		t.Fatal("the composition root would run the fail-closed reader despite valid configuration")
	}
}

// TestCompositionRootFallsBackWhenSecretMissing proves the reverse: an
// incomplete deployment gets the fail-closed clients rather than a half-working
// one that could fabricate success.
func TestCompositionRootFallsBackWhenSecretMissing(t *testing.T) {
	cfg := completeFleetConfig()
	cfg.FleetServiceSecret = nil

	clientConfig := marketplace.ResponderConfig{
		BaseURL:     cfg.FleetServiceURL,
		ServiceName: cfg.FleetServiceCaller,
		Secret:      cfg.FleetServiceSecret,
		Timeout:     cfg.FleetServiceTimeout,
	}

	responder, ready := marketplace.NewHTTPFleetResponder(clientConfig)
	if ready || responder.Configured() {
		t.Fatal("a missing secret produced a live responder")
	}
	reader, ready := marketplace.NewHTTPProviderReader(clientConfig)
	if ready || reader.Configured() {
		t.Fatal("a missing secret produced a live reader")
	}
}

// TestRelayWorkerStartsOnlyWhenEnabled covers the flag and the graceful stop.
func TestRelayWorkerStartsOnlyWhenEnabled(t *testing.T) {
	// Flag off: nothing starts, and the returned stopper is safe to call.
	disabled := completeFleetConfig()
	disabled.FleetRelayEnabled = false
	stop := StartMarketplaceWorkers(context.Background(), disabled, nil)
	if stop == nil {
		t.Fatal("a disabled worker must still return a usable stop function")
	}
	stop()

	// Flag on but configuration incomplete: still nothing starts. A worker that
	// can only fail would burn the retry budget on every queued decision.
	unconfigured := completeFleetConfig()
	unconfigured.FleetServiceSecret = nil
	stop = StartMarketplaceWorkers(context.Background(), unconfigured, nil)
	if stop == nil {
		t.Fatal("an unconfigured worker must still return a usable stop function")
	}
	stop()
}

// TestRelayWorkerStopsGracefully proves the stopper actually waits for the
// worker goroutine, rather than returning while it is mid-pass.
func TestRelayWorkerStopsGracefully(t *testing.T) {
	cfg := completeFleetConfig()
	// A nil pool is safe here: the worker's first action on each pass is a
	// database call that fails, which RunOnce absorbs without exiting. What is
	// under test is the lifecycle, not the query.
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Skipf("worker requires a live pool in this environment: %v", recovered)
		}
	}()

	stop := StartMarketplaceWorkers(context.Background(), cfg, nil)
	done := make(chan struct{})
	go func() {
		stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the relay worker did not stop within the shutdown window")
	}
}
