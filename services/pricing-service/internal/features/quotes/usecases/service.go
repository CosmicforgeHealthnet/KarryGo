// Package usecases issues, binds and reconciles quotes.
//
// Every number comes from the engine, priced against policy versions resolved
// for the place and moment of the request; this package only decides WHICH
// versions apply and records what was said. A quote stores the versions it used,
// so reconciliation re-prices with the original rules even after they have been
// replaced.
package usecases

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cosmicforge/logistics/services/pricing-service/internal/engine"
	policymodels "cosmicforge/logistics/services/pricing-service/internal/features/policies/models"
	"cosmicforge/logistics/services/pricing-service/internal/features/quotes/models"
	rolloutmodels "cosmicforge/logistics/services/pricing-service/internal/features/rollouts/models"
	"cosmicforge/logistics/services/pricing-service/internal/platform/canonical"
	"cosmicforge/logistics/services/pricing-service/internal/platform/ids"
	"cosmicforge/logistics/shared/go/geo"
	"cosmicforge/logistics/shared/go/pricingclient"
)

// DefaultTTL applies when no quote-lifecycle policy is active for the place.
const DefaultTTL = 15 * time.Minute

// Store persists quotes and reads geography.
type Store interface {
	ZoneForConsumer(ctx context.Context, consumerService, consumerZoneID string, asOf time.Time) (string, error)
	ZoneCandidates(ctx context.Context) ([]models.ZoneCandidate, error)
	PlaceForZone(ctx context.Context, zoneID string) (models.Place, error)
	InsertQuote(ctx context.Context, q models.Quote) (models.Quote, bool, error)
	GetQuote(ctx context.Context, id, consumerService string) (models.Quote, error)
	FindByRef(ctx context.Context, consumerService, consumerRef, class string) (models.Quote, error)
	Bind(ctx context.Context, binding models.Binding) (models.Binding, error)
	BindingFor(ctx context.Context, quoteID string) (models.Binding, error)
	Reconcile(ctx context.Context, r models.Reconciliation) (models.Reconciliation, error)
	ShadowSummary(ctx context.Context, since time.Time) ([]models.ShadowSummaryRow, error)
}

// Policies resolves and loads policy versions.
type Policies interface {
	Resolve(ctx context.Context, kind string, chain policymodels.GeographyChain, asOf time.Time) (policymodels.Resolution, error)
	GetVersion(ctx context.Context, id string) (policymodels.Version, error)
}

// Rollouts reports whether a market quotes centrally.
type Rollouts interface {
	ModeFor(ctx context.Context, scope, capability, marketCode string) (string, error)
}

// Service issues, binds and reconciles quotes.
type Service struct {
	store    Store
	policies Policies
	rollouts Rollouts
	now      func() time.Time
}

// NewService builds the quote service.
func NewService(store Store, policies Policies, rollouts Rollouts) *Service {
	return &Service{store: store, policies: policies, rollouts: rollouts, now: func() time.Time { return time.Now().UTC() }}
}

// Issue prices every requested class and stores one quote per class. Asking
// again with the same reference returns the quotes already issued.
func (s *Service) Issue(ctx context.Context, caller string, request pricingclient.QuoteRequest) (pricingclient.QuoteResponse, error) {
	if message, ok := request.Validate(); !ok {
		return pricingclient.QuoteResponse{}, fmt.Errorf("%w: %s", ErrInvalid, message)
	}
	scope := engine.Scope(request.Scope)
	channel := engine.Channel(request.Channel)
	if !channel.Valid() {
		return pricingclient.QuoteResponse{}, fmt.Errorf("%w: unknown channel", ErrInvalid)
	}
	now := s.now()

	place, err := s.place(ctx, caller, request, now)
	if err != nil {
		return pricingclient.QuoteResponse{}, err
	}
	chain := policymodels.GeographyChain{
		ZoneID: place.ZoneID, MarketID: place.MarketID, StateID: place.StateID, CountryCode: place.CountryCode,
	}

	tariffKind, err := engine.TariffKindFor(scope)
	if err != nil {
		return pricingclient.QuoteResponse{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	tariffResolution, err := s.policies.Resolve(ctx, tariffKind, chain, now)
	if errors.Is(err, policymodels.ErrNoActivePolicy) {
		return pricingclient.QuoteResponse{}, models.ErrNoActiveTariff
	}
	if err != nil {
		return pricingclient.QuoteResponse{}, err
	}
	tariff, err := engine.DecodeTariff(tariffResolution.Version.Kind, tariffResolution.Version.SchemaVersion, tariffResolution.Version.Body)
	if err != nil {
		return pricingclient.QuoteResponse{}, err
	}
	commission, commissionVersionID, err := s.commission(ctx, chain, now)
	if err != nil {
		return pricingclient.QuoteResponse{}, err
	}
	ttl, err := s.ttl(ctx, chain, scope, channel, now)
	if err != nil {
		return pricingclient.QuoteResponse{}, err
	}
	mode, err := s.mode(ctx, request.Scope, place.MarketCode)
	if err != nil {
		return pricingclient.QuoteResponse{}, err
	}

	response := pricingclient.QuoteResponse{}
	for _, class := range uniqueClasses(request.Classes) {
		priced := normalize(request, class)
		result, err := engine.Evaluate(priced.EngineRequest(now), engine.PolicySet{Tariff: tariff, Commission: commission})
		if errors.Is(err, engine.ErrUnsupported) {
			response.Unpriced = append(response.Unpriced, pricingclient.Unpriced{Class: class, Reason: models.ReasonClassNotPriced})
			continue
		}
		if err != nil {
			return pricingclient.QuoteResponse{}, err
		}
		hash, err := canonical.Hash(result)
		if err != nil {
			return pricingclient.QuoteResponse{}, err
		}
		quote := models.Quote{
			ID:                  ids.New(ids.PrefixQuote),
			ConsumerService:     caller,
			ConsumerRef:         request.ConsumerRef,
			Scope:               request.Scope,
			Channel:             request.Channel,
			Class:               class,
			Mode:                mode,
			Place:               place,
			TariffVersionID:     tariffResolution.Version.ID,
			CommissionVersionID: commissionVersionID,
			Request:             priced,
			Result:              result,
			ResultHash:          hash,
			LegacyFareKobo:      legacyFor(request.LegacyFareKobo, class),
			IssuedAt:            now,
			ExpiresAt:           now.Add(ttl),
		}
		stored, _, err := s.store.InsertQuote(ctx, quote)
		if err != nil {
			return pricingclient.QuoteResponse{}, err
		}
		response.Quotes = append(response.Quotes, toContract(stored))
	}
	return response, nil
}

// FindByRef returns a caller's quote by its own reference and class.
func (s *Service) FindByRef(ctx context.Context, caller, consumerRef, class string) (pricingclient.Quote, error) {
	quote, err := s.store.FindByRef(ctx, caller, consumerRef, class)
	if err != nil {
		return pricingclient.Quote{}, err
	}
	return toContract(quote), nil
}

// Bind ties a caller's quote to the booking that used it. Binding again to the
// same booking returns the existing binding; to a different booking is refused.
func (s *Service) Bind(ctx context.Context, caller, quoteID, bookingRef string) (pricingclient.Binding, error) {
	bookingRef = strings.TrimSpace(bookingRef)
	if bookingRef == "" {
		return pricingclient.Binding{}, fmt.Errorf("%w: booking_ref is required", ErrInvalid)
	}
	quote, err := s.store.GetQuote(ctx, quoteID, caller)
	if err != nil {
		return pricingclient.Binding{}, err
	}
	if existing, err := s.store.BindingFor(ctx, quoteID); err == nil {
		return bindingFor(existing, bookingRef)
	} else if !errors.Is(err, models.ErrNotFound) {
		return pricingclient.Binding{}, err
	}

	grace, err := s.bindGrace(ctx, quote)
	if err != nil {
		return pricingclient.Binding{}, err
	}
	now := s.now()
	if now.After(quote.ExpiresAt.Add(grace)) {
		return pricingclient.Binding{}, models.ErrBindExpired
	}
	stored, err := s.store.Bind(ctx, models.Binding{
		ID: ids.New("pqb"), QuoteID: quoteID, ConsumerService: caller, BookingRef: bookingRef, BoundAt: now,
	})
	if err != nil {
		return pricingclient.Binding{}, err
	}
	return bindingFor(stored, bookingRef)
}

// Reconcile prices a bound quote again from the actual route, with the quote's
// ORIGINAL policy versions. Reconciling twice returns the first result.
func (s *Service) Reconcile(ctx context.Context, caller, quoteID string, request pricingclient.ReconcileRequest) (pricingclient.Reconciliation, error) {
	if request.Route.DistanceMeters < 0 || request.Route.DurationSeconds < 0 || request.Route.TrafficDurationSeconds < 0 {
		return pricingclient.Reconciliation{}, fmt.Errorf("%w: route figures cannot be negative", ErrInvalid)
	}
	quote, err := s.store.GetQuote(ctx, quoteID, caller)
	if err != nil {
		return pricingclient.Reconciliation{}, err
	}
	if _, err := s.store.BindingFor(ctx, quoteID); errors.Is(err, models.ErrNotFound) {
		return pricingclient.Reconciliation{}, models.ErrNotBound
	} else if err != nil {
		return pricingclient.Reconciliation{}, err
	}

	tariffVersion, err := s.policies.GetVersion(ctx, quote.TariffVersionID)
	if err != nil {
		return pricingclient.Reconciliation{}, err
	}
	tariff, err := engine.DecodeTariff(tariffVersion.Kind, tariffVersion.SchemaVersion, tariffVersion.Body)
	if err != nil {
		return pricingclient.Reconciliation{}, err
	}
	commission := noCommission()
	if quote.CommissionVersionID != "" {
		if commission, err = s.commissionVersion(ctx, quote.CommissionVersionID); err != nil {
			return pricingclient.Reconciliation{}, err
		}
	}

	actual := quote.Request
	actual.DistanceMeters = request.Route.DistanceMeters
	actual.DurationSeconds = request.Route.DurationSeconds
	actual.TrafficDurationSeconds = request.Route.TrafficDurationSeconds
	actual.RouteConfidence = request.Route.Confidence
	actual.RouteDegraded = request.Route.Degraded

	now := s.now()
	result, err := engine.Evaluate(actual.EngineRequest(now), engine.PolicySet{Tariff: tariff, Commission: commission})
	if err != nil {
		return pricingclient.Reconciliation{}, err
	}
	hash, err := canonical.Hash(result)
	if err != nil {
		return pricingclient.Reconciliation{}, err
	}
	stored, err := s.store.Reconcile(ctx, models.Reconciliation{
		ID: ids.New(ids.PrefixReconciliation), QuoteID: quoteID, Request: actual, Result: result, ResultHash: hash,
		LegacyFinalKobo: request.LegacyFinalKobo, ReconciledAt: now,
	})
	if err != nil {
		return pricingclient.Reconciliation{}, err
	}
	return reconciliationToContract(stored), nil
}

// ShadowSummary reports how pricing compares with what callers charged.
func (s *Service) ShadowSummary(ctx context.Context, since time.Time) ([]models.ShadowSummaryRow, error) {
	return s.store.ShadowSummary(ctx, since)
}

// ErrInvalid is a request pricing cannot price as asked.
var ErrInvalid = errors.New("quotes: invalid request")

// place resolves the caller's zone mapping first, then the zone containing the
// pickup. The smallest containing circle wins, so a city-centre zone inside a
// metro-wide one is chosen over it.
func (s *Service) place(ctx context.Context, caller string, request pricingclient.QuoteRequest, asOf time.Time) (models.Place, error) {
	if zone := strings.TrimSpace(request.ConsumerZoneID); zone != "" {
		zoneID, err := s.store.ZoneForConsumer(ctx, caller, zone, asOf)
		if err != nil {
			return models.Place{}, err
		}
		if zoneID != "" {
			return s.store.PlaceForZone(ctx, zoneID)
		}
	}
	if request.Pickup == nil {
		return models.Place{}, models.ErrNoMarket
	}
	candidates, err := s.store.ZoneCandidates(ctx)
	if err != nil {
		return models.Place{}, err
	}
	pickup := geo.Point{Lat: request.Pickup.Lat, Lng: request.Pickup.Lng}
	var best *models.ZoneCandidate
	for i := range candidates {
		candidate := candidates[i]
		center := geo.Point{Lat: float64(candidate.CenterLatE6) / 1e6, Lng: float64(candidate.CenterLngE6) / 1e6}
		if geo.HaversineMeters(pickup, center) > float64(candidate.RadiusM) {
			continue
		}
		if best == nil || candidate.RadiusM < best.RadiusM {
			best = &candidate
		}
	}
	if best == nil {
		return models.Place{}, models.ErrNoMarket
	}
	return best.Place, nil
}

func (s *Service) commission(ctx context.Context, chain policymodels.GeographyChain, asOf time.Time) (engine.CommissionV1, string, error) {
	resolution, err := s.policies.Resolve(ctx, engine.KindCommission, chain, asOf)
	if errors.Is(err, policymodels.ErrNoActivePolicy) {
		// No active commission: price the economic fare, and say so by leaving
		// the commission version empty, rather than pretending the fee is zero.
		return noCommission(), "", nil
	}
	if err != nil {
		return engine.CommissionV1{}, "", err
	}
	commission, err := s.commissionVersion(ctx, resolution.Version.ID)
	return commission, resolution.Version.ID, err
}

func (s *Service) commissionVersion(ctx context.Context, versionID string) (engine.CommissionV1, error) {
	version, err := s.policies.GetVersion(ctx, versionID)
	if err != nil {
		return engine.CommissionV1{}, err
	}
	body, err := engine.Decode(version.Kind, version.SchemaVersion, version.Body)
	if err != nil {
		return engine.CommissionV1{}, err
	}
	commission, ok := body.(engine.CommissionV1)
	if !ok {
		return engine.CommissionV1{}, fmt.Errorf("quotes: version %s is not a commission", versionID)
	}
	return commission, nil
}

func (s *Service) lifecycle(ctx context.Context, chain policymodels.GeographyChain, asOf time.Time) (engine.QuoteLifecycleV1, bool, error) {
	resolution, err := s.policies.Resolve(ctx, engine.KindQuoteLifecycle, chain, asOf)
	if errors.Is(err, policymodels.ErrNoActivePolicy) {
		return engine.QuoteLifecycleV1{}, false, nil
	}
	if err != nil {
		return engine.QuoteLifecycleV1{}, false, err
	}
	body, err := engine.Decode(resolution.Version.Kind, resolution.Version.SchemaVersion, resolution.Version.Body)
	if err != nil {
		return engine.QuoteLifecycleV1{}, false, err
	}
	lifecycle, ok := body.(engine.QuoteLifecycleV1)
	return lifecycle, ok, nil
}

func (s *Service) ttl(ctx context.Context, chain policymodels.GeographyChain, scope engine.Scope, channel engine.Channel, asOf time.Time) (time.Duration, error) {
	lifecycle, ok, err := s.lifecycle(ctx, chain, asOf)
	if err != nil {
		return 0, err
	}
	if !ok {
		return DefaultTTL, nil
	}
	if ttl := lifecycle.TTLFor(scope, channel); ttl > 0 {
		return ttl, nil
	}
	return DefaultTTL, nil
}

func (s *Service) bindGrace(ctx context.Context, quote models.Quote) (time.Duration, error) {
	if quote.Place.ZoneID == "" {
		return 0, nil
	}
	place, err := s.store.PlaceForZone(ctx, quote.Place.ZoneID)
	if err != nil {
		// The zone has since been retired: no grace, the quote's own expiry rules.
		return 0, nil
	}
	chain := policymodels.GeographyChain{
		ZoneID: place.ZoneID, MarketID: place.MarketID, StateID: place.StateID, CountryCode: place.CountryCode,
	}
	lifecycle, ok, err := s.lifecycle(ctx, chain, quote.IssuedAt)
	if err != nil || !ok {
		return 0, err
	}
	return lifecycle.BindGraceFor(engine.Channel(quote.Channel)), nil
}

// mode is CENTRAL only when pricing has been switched on for this market's
// quotes; everywhere else a quote is shadow evidence. The caller's own flag is
// the other key: a caller in shadow mode keeps charging its own fare whatever
// this says.
func (s *Service) mode(ctx context.Context, scope, marketCode string) (string, error) {
	mode, err := s.rollouts.ModeFor(ctx, scope, rolloutmodels.CapabilityQuote, marketCode)
	if err != nil {
		return "", err
	}
	if mode == rolloutmodels.ModeCentral {
		return models.ModeCentral, nil
	}
	return models.ModeShadow, nil
}

func noCommission() engine.CommissionV1 {
	return engine.CommissionV1{SchemaVersion: 1, AppliedTo: engine.CommissionOnCustomerPayable, Rounding: "FLOOR"}
}

func normalize(request pricingclient.QuoteRequest, class string) models.PricedRequest {
	priced := models.PricedRequest{
		Scope:                  request.Scope,
		Channel:                request.Channel,
		Class:                  class,
		DistanceMeters:         request.Route.DistanceMeters,
		DurationSeconds:        request.Route.DurationSeconds,
		TrafficDurationSeconds: request.Route.TrafficDurationSeconds,
		RouteConfidence:        request.Route.Confidence,
		RouteDegraded:          request.Route.Degraded,
		PackageSize:            strings.ToLower(strings.TrimSpace(request.PackageSize)),
		ServiceTier:            strings.ToLower(strings.TrimSpace(request.ServiceTier)),
		WeightGrams:            request.WeightGrams,
		Helpers:                request.Helpers,
		ConsumerZoneID:         strings.TrimSpace(request.ConsumerZoneID),
	}
	if request.Pickup != nil {
		priced.HasPickup = true
		priced.PickupLatE6 = canonical.Micro(request.Pickup.Lat)
		priced.PickupLngE6 = canonical.Micro(request.Pickup.Lng)
	}
	return priced
}

func uniqueClasses(classes []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(classes))
	for _, class := range classes {
		class = strings.TrimSpace(class)
		if class == "" || seen[class] {
			continue
		}
		seen[class] = true
		out = append(out, class)
	}
	return out
}

func legacyFor(fares map[string]int64, class string) *int64 {
	if fare, ok := fares[class]; ok && fare >= 0 {
		return &fare
	}
	return nil
}

func difference(gross int64, legacy *int64) *int64 {
	if legacy == nil {
		return nil
	}
	d := gross - *legacy
	return &d
}

func toContract(q models.Quote) pricingclient.Quote {
	components := make([]pricingclient.Component, 0, len(q.Result.Components))
	for _, c := range q.Result.Components {
		components = append(components, pricingclient.Component{Code: c.Code, AmountKobo: c.AmountKobo, Basis: c.Basis})
	}
	return pricingclient.Quote{
		ID: q.ID, Class: q.Class, Mode: q.Mode, Currency: q.Result.Currency,
		GrossKobo: q.Result.GrossKobo, ProviderKobo: q.Result.Allocation.ProviderKobo,
		PlatformFeeKobo: q.Result.Allocation.PlatformFeeKobo, TaxKobo: q.Result.Allocation.TaxKobo,
		Components: components, Flags: q.Result.Flags,
		LegacyFareKobo: q.LegacyFareKobo, DifferenceKobo: difference(q.Result.GrossKobo, q.LegacyFareKobo),
		TariffVersionID: q.TariffVersionID, CommissionVersionID: q.CommissionVersionID,
		IssuedAt: q.IssuedAt, ExpiresAt: q.ExpiresAt,
	}
}

func bindingFor(b models.Binding, bookingRef string) (pricingclient.Binding, error) {
	if b.BookingRef != bookingRef {
		return pricingclient.Binding{}, models.ErrBindConflict
	}
	return pricingclient.Binding{QuoteID: b.QuoteID, BookingRef: b.BookingRef, BoundAt: b.BoundAt}, nil
}

func reconciliationToContract(r models.Reconciliation) pricingclient.Reconciliation {
	components := make([]pricingclient.Component, 0, len(r.Result.Components))
	for _, c := range r.Result.Components {
		components = append(components, pricingclient.Component{Code: c.Code, AmountKobo: c.AmountKobo, Basis: c.Basis})
	}
	return pricingclient.Reconciliation{
		QuoteID: r.QuoteID, GrossKobo: r.Result.GrossKobo, ProviderKobo: r.Result.Allocation.ProviderKobo,
		PlatformFeeKobo: r.Result.Allocation.PlatformFeeKobo, Components: components,
		LegacyFinalKobo: r.LegacyFinalKobo, DifferenceKobo: difference(r.Result.GrossKobo, r.LegacyFinalKobo),
		ReconciledAt: r.ReconciledAt,
	}
}
