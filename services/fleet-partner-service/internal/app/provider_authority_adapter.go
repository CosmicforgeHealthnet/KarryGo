package app

import (
	"context"
	"errors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/managedmembers"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// providerAuthorityAdapter resolves a provider with the service that owns it,
// before Fleet records any link to it.
//
// This is the whole of Section 10's "Fleet must never mint a provider id"
// rule, expressed as a dependency rather than a convention. Fleet has no code
// path that produces a Dispatch, Taxi or Hauling provider identity; the only
// way one enters this service is as the answer to a question asked of the
// service that issued it.
type providerAuthorityAdapter struct {
	providers map[string]platformclients.ProviderContract
}

// BuildProviderAuthorityAdapter binds the member layer to the provider
// contracts this service already builds.
//
// It takes the composition root's own contract set rather than making a new
// one, so an authority that has a live client here gets it, and one that does
// not keeps the fail-closed adapter that refuses rather than returning a usable
// zero value. That distinction matters here more than anywhere: a zero-valued
// "success" would let Fleet record an organization link to a provider no
// service has ever heard of, and the error would surface later, as work
// dispatched to nobody.
func BuildProviderAuthorityAdapter(contracts platformclients.ContractSet) managedmembers.ProviderAuthority {
	providers := contracts.Providers
	if providers == nil {
		providers = platformclients.NewUnconfiguredContracts().Providers
	}
	return &providerAuthorityAdapter{providers: providers}
}

// ResolveProvider asks one authority to identify a provider.
func (a *providerAuthorityAdapter) ResolveProvider(ctx context.Context,
	authority managedmembers.Authority, providerRef string) (managedmembers.ProviderResolution, error) {
	contract, ok := a.providers[string(authority)]
	if !ok || contract == nil {
		return managedmembers.ProviderResolution{}, managedmembers.ErrAuthorityUnavailable
	}
	view, err := contract.ReadProvider(ctx, platformclients.CallMeta{}, providerRef)
	if err != nil {
		return managedmembers.ProviderResolution{}, classifyAuthorityError(err)
	}
	if view.ProviderID == "" {
		// The authority answered without naming a provider. That is a refusal
		// with extra steps, and recording it as a resolution would create a
		// link to an identity that does not exist.
		return managedmembers.ProviderResolution{}, managedmembers.ErrAuthorityRefused
	}
	return managedmembers.ProviderResolution{
		ProviderID:          view.ProviderID,
		QualificationStatus: view.PlatformStatus,
		SourceVersion:       view.SourceVersion,
	}, nil
}

// classifyAuthorityError separates "the authority said no" from "the authority
// did not answer".
//
// The platform client already carries this distinction as an outcome, and
// preserving it is what stops an outage being recorded as a negative fact
// about a provider. A refusal must never be retried and an outage must never
// be treated as a decision.
func classifyAuthorityError(err error) error {
	var contractErr *platformclients.Error
	if errors.As(err, &contractErr) {
		switch contractErr.Outcome {
		case platformclients.OutcomeUnknown, platformclients.OutcomeTransient:
			return managedmembers.ErrAuthorityUnavailable
		default:
			return managedmembers.ErrAuthorityRefused
		}
	}
	if errors.Is(err, platformclients.ErrNotConfigured) {
		return managedmembers.ErrAuthorityUnavailable
	}
	return managedmembers.ErrAuthorityUnavailable
}
