package platformclients

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/circuitbreaker"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/sourceref"
)

// Foundational per-service interfaces. Each currently exposes only the
// contract surface needed to prove configuration, authentication, tracing,
// timeouts, error classification and instrumentation. Business methods are
// added by later work packages on these interfaces — never by importing
// another service's internals.

// CustomerServiceClient references canonical customer identity.
type CustomerServiceClient interface {
	Ping(ctx context.Context, meta CallMeta) error
	Configured() bool
}

// DispatchServiceClient reaches driver-dispatch-delivery-service.
type DispatchServiceClient interface {
	Ping(ctx context.Context, meta CallMeta) error
	Configured() bool
}

// TaxiServiceClient reaches driver-taxi-service.
type TaxiServiceClient interface {
	Ping(ctx context.Context, meta CallMeta) error
	Configured() bool
}

// HaulingServiceClient reaches driver-hauling-service.
type HaulingServiceClient interface {
	Ping(ctx context.Context, meta CallMeta) error
	Configured() bool
}

// PaymentWalletServiceClient reaches payment-wallet-service. Wallet ledgers
// stay in payment-wallet-service; this service stores projections only.
type PaymentWalletServiceClient interface {
	Ping(ctx context.Context, meta CallMeta) error
	Configured() bool
}

// NotificationServiceClient creates notification intents.
type NotificationServiceClient interface {
	Ping(ctx context.Context, meta CallMeta) error
	Configured() bool
}

// MediaFileServiceClient references files and evidence by ID.
type MediaFileServiceClient interface {
	Ping(ctx context.Context, meta CallMeta) error
	Configured() bool
}

// VerificationServiceClient consumes verification outcomes.
type VerificationServiceClient interface {
	Ping(ctx context.Context, meta CallMeta) error
	Configured() bool
}

// SupportServiceClient adds organization context to platform support cases.
type SupportServiceClient interface {
	Ping(ctx context.Context, meta CallMeta) error
	Configured() bool
}

// AnalyticsServiceClient publishes corporate events and consumes aggregates.
type AnalyticsServiceClient interface {
	Ping(ctx context.Context, meta CallMeta) error
	Configured() bool
}

// AdminBackofficeServiceClient exposes protected internal review flows.
type AdminBackofficeServiceClient interface {
	Ping(ctx context.Context, meta CallMeta) error
	Configured() bool
}

// Set bundles every platform client adapter.
type Set struct {
	Customer     CustomerServiceClient
	Dispatch     DispatchServiceClient
	Taxi         TaxiServiceClient
	Hauling      HaulingServiceClient
	Payment      PaymentWalletServiceClient
	Notification NotificationServiceClient
	Media        MediaAuthority 
	Verification VerificationServiceClient
	Support      SupportServiceClient
	Analytics    AnalyticsServiceClient
	Admin        AdminBackofficeServiceClient
}

// OptionsFromConfig fills the Section 29 reliability knobs (timeout,
// environment, retry, circuit breaker) plus metrics from cfg, leaving only
// the per-client identity (ServiceName/BaseURL/Secret) for the caller —
// every platformclients.New call site in this service's composition root
// should build its Options through this helper rather than repeating these
// fields, so a single config change (an env var) actually reaches every
// downstream client instead of only the ones a call site happened to spell
// out.
func OptionsFromConfig(cfg config.Config, metrics *observability.Metrics) Options {
	return Options{
		Timeout:          cfg.HTTPClientTimeout,
		Metrics:          metrics,
		Environment:      cfg.AppEnv,
		MaxRetries:       cfg.ExternalClientMaxRetries,
		RetryBackoffBase: cfg.ExternalClientRetryBackoffBase,
		RetryBackoffMax:  cfg.ExternalClientRetryBackoffMax,
		Breaker: circuitbreaker.Config{
			FailureThreshold:  cfg.CircuitBreakerFailureThreshold,
			ObservationWindow: cfg.CircuitBreakerObservationWindow,
			OpenDuration:      cfg.CircuitBreakerOpenDuration,
			HalfOpenProbes:    cfg.CircuitBreakerHalfOpenProbes,
			SuccessThreshold:  cfg.CircuitBreakerSuccessThreshold,
		},
	}
}

// NewSet builds foundational adapters for every platform service from
// configuration. Unconfigured services yield adapters whose calls return
// ErrNotConfigured.
func NewSet(cfg config.Config, metrics *observability.Metrics) Set {
	base := OptionsFromConfig(cfg, metrics)
	
	build := func(serviceName string) *Client {
		clientCfg := cfg.Clients[serviceName]
		opts := base
		opts.ServiceName = serviceName
		opts.BaseURL = clientCfg.BaseURL
		opts.Secret = clientCfg.Secret
		return New(opts)
	}
	
	// Build media client separately (MediaAuthority interface)
	mediaCfg := cfg.Clients[sourceref.ServiceMedia]
	mediaOpts := base
	mediaOpts.ServiceName = sourceref.ServiceMedia
	mediaOpts.BaseURL = mediaCfg.BaseURL
	mediaOpts.Secret = mediaCfg.Secret
	mediaClient := NewMediaFileClient(mediaOpts)

	return Set{
		Customer:     build(sourceref.ServiceCustomer),
		Dispatch:     build(sourceref.ServiceDispatch),
		Taxi:         build(sourceref.ServiceTaxi),
		Hauling:      build(sourceref.ServiceHauling),
		Payment:      build(sourceref.ServicePayment),
		Notification: build(sourceref.ServiceNotification),
		Media:        mediaClient,
		Verification: build(sourceref.ServiceVerification),
		Support:      build(sourceref.ServiceSupport),
		Analytics:    build(sourceref.ServiceAnalytics),
		Admin:        build(sourceref.ServiceAdmin),
	}
}