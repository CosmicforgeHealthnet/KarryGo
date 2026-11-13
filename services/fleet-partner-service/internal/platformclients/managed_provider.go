package platformclients

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"cosmicforge/logistics/shared/go/orgprovider"
)

// Private base paths of the managed-provider contract in each owning service.
// All three expose the same routes beneath them (see shared/go/orgprovider):
//
//	POST /providers/resolve
//	POST /providers/sponsored
//	GET  /providers/:providerId
//	POST /provider-links
//	GET  /provider-links/:linkId
//	GET  /provider-links/by-provider/:providerId
//	POST /provider-links/:linkId/transitions
const (
	DispatchManagedProviderBase = "/api/v1/internal/organization-providers"
	TaxiManagedProviderBase     = "/api/v1/taxi/internal/organization-providers"
	HaulingManagedProviderBase  = haulingInternalBase
)

// ManagedProviderClient implements ProviderContract against one owning
// provider service. Fleet stores only the opaque ids and versions it returns.
type ManagedProviderClient struct {
	client   *Client
	apiBase  string
	category string
}

// NewManagedProviderClient builds a client for one owning service.
func NewManagedProviderClient(opts Options, apiBase string, category string) *ManagedProviderClient {
	return &ManagedProviderClient{client: New(opts), apiBase: apiBase, category: category}
}

// NewDispatchProviderClient is the Dispatch managed-provider client.
func NewDispatchProviderClient(opts Options) *ManagedProviderClient {
	if opts.ServiceName == "" {
		opts.ServiceName = "driver-dispatch-delivery-service"
	}
	return NewManagedProviderClient(opts, DispatchManagedProviderBase, CategoryDispatch)
}

// NewTaxiProviderClient is the Taxi managed-provider client.
func NewTaxiProviderClient(opts Options) *ManagedProviderClient {
	if opts.ServiceName == "" {
		opts.ServiceName = "driver-taxi-service"
	}
	return NewManagedProviderClient(opts, TaxiManagedProviderBase, CategoryTaxi)
}

// Configured reports whether live calls can be made.
func (c *ManagedProviderClient) Configured() bool {
	return c != nil && c.client != nil && c.client.Configured()
}

// Category is the service category this client answers for.
func (c *ManagedProviderClient) Category() string { return c.category }

type envelope[T any] struct {
	Data T `json:"data"`
}

// writeMeta completes the headers every owning service requires on a write:
// an idempotency key, a correlation id and a causation id. A provisioning
// saga often runs without a request (the reconciler), so the stable
// idempotency key stands in for the missing ones rather than letting the
// write fail on a header.
func writeMeta(meta CallMeta) CallMeta {
	if strings.TrimSpace(meta.CorrelationID) == "" {
		meta.CorrelationID = firstNonEmpty(meta.RequestID, meta.IdempotencyKey)
	}
	if strings.TrimSpace(meta.CausationID) == "" {
		meta.CausationID = firstNonEmpty(meta.IdempotencyKey, meta.RequestID)
	}
	return meta
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// Resolve looks a person up by both contacts.
func (c *ManagedProviderClient) Resolve(ctx context.Context, meta CallMeta, request ProviderResolveRequest) (ProviderResolution, error) {
	if !c.Configured() {
		return ProviderResolution{}, ErrNotConfigured
	}
	var out envelope[orgprovider.ResolveResponse]
	err := c.client.Do(ctx, meta, http.MethodPost, c.apiBase+"/providers/resolve", orgprovider.ResolveRequest{
		EmailNormalized: request.EmailNormalized, PhoneE164: request.PhoneE164,
	}, &out)
	if err != nil {
		return ProviderResolution{}, err
	}
	return ProviderResolution{Outcome: out.Data.Outcome, ProviderID: out.Data.ProviderID,
		PlatformStatus: out.Data.PlatformStatus, ManagedByOrgID: out.Data.ManagedByOrgID,
		ServiceCategory: out.Data.ServiceCategory, SourceVersion: out.Data.SourceVersion}, nil
}

// ProvisionSponsored asks for one organization-sponsored identity.
func (c *ManagedProviderClient) ProvisionSponsored(ctx context.Context, meta CallMeta, request ProviderProvisionRequest) (ProviderIdentity, error) {
	if !c.Configured() {
		return ProviderIdentity{}, ErrNotConfigured
	}
	meta.OrganizationID, meta.IdempotencyKey = request.OrganizationID, request.IdempotencyKey
	var out envelope[orgprovider.ProviderView]
	err := c.client.Do(ctx, writeMeta(meta), http.MethodPost, c.apiBase+"/providers/sponsored", orgprovider.SponsoredRequest{
		OrganizationID: request.OrganizationID, LegalName: request.LegalName,
		EmailNormalized: request.EmailNormalized, PhoneE164: request.PhoneE164,
		ServiceCategory: request.ServiceCategory, OnboardingMode: request.OnboardingMode,
		ProviderMode: request.ProviderMode,
	}, &out)
	if err != nil {
		return ProviderIdentity{}, err
	}
	return ProviderIdentity{ProviderID: out.Data.ProviderID, PlatformStatus: out.Data.AccountState,
		SourceVersion: out.Data.SourceVersion}, nil
}

// ReadProvider reads the provider's account state.
func (c *ManagedProviderClient) ReadProvider(ctx context.Context, meta CallMeta, providerID string) (ProviderIdentity, error) {
	if !c.Configured() {
		return ProviderIdentity{}, ErrNotConfigured
	}
	var out envelope[orgprovider.ProviderView]
	err := c.client.Do(ctx, meta, http.MethodGet, c.apiBase+"/providers/"+url.PathEscape(providerID), nil, &out)
	return ProviderIdentity{ProviderID: out.Data.ProviderID, PlatformStatus: out.Data.AccountState,
		SourceVersion: out.Data.SourceVersion}, err
}

// CreateAssociation records a REQUESTED association the provider must
// accept. The same idempotency key returns the association in its current
// state, so a retry doubles as a read-back.
func (c *ManagedProviderClient) CreateAssociation(ctx context.Context, meta CallMeta, request ProviderAssociationRequest) (ProviderAssociation, error) {
	if !c.Configured() {
		return ProviderAssociation{}, ErrNotConfigured
	}
	meta.OrganizationID, meta.IdempotencyKey = request.OrganizationID, request.IdempotencyKey
	var out envelope[orgprovider.LinkView]
	err := c.client.Do(ctx, writeMeta(meta), http.MethodPost, c.apiBase+"/provider-links", orgprovider.LinkRequest{
		OrganizationID: request.OrganizationID, ProviderID: request.ProviderID, MembershipID: request.MembershipID,
	}, &out)
	return associationFrom(out.Data), err
}

// ReadAssociation reads the association inside meta.OrganizationID.
func (c *ManagedProviderClient) ReadAssociation(ctx context.Context, meta CallMeta, associationID string) (ProviderAssociation, error) {
	if !c.Configured() {
		return ProviderAssociation{}, ErrNotConfigured
	}
	var out envelope[orgprovider.LinkView]
	err := c.client.Do(ctx, meta, http.MethodGet, c.apiBase+"/provider-links/"+url.PathEscape(associationID), nil, &out)
	return associationFrom(out.Data), err
}

// RevokeOrganizationEligibility revokes the provider's live association with
// the organization. A provider with no live association has nothing left to
// revoke.
func (c *ManagedProviderClient) RevokeOrganizationEligibility(ctx context.Context, meta CallMeta, providerID, organizationID, reason string) error {
	if !c.Configured() {
		return ErrNotConfigured
	}
	meta.OrganizationID = organizationID
	var current envelope[orgprovider.LinkView]
	err := c.client.Do(ctx, meta, http.MethodGet, c.apiBase+"/provider-links/by-provider/"+url.PathEscape(providerID), nil, &current)
	if err != nil {
		var clientErr *Error
		if errors.As(err, &clientErr) && clientErr.Outcome == OutcomeNotFound {
			return nil
		}
		return err
	}
	if strings.TrimSpace(meta.IdempotencyKey) == "" {
		meta.IdempotencyKey = strings.TrimSpace(meta.RequestID)
	}
	if meta.IdempotencyKey == "" {
		return ErrMissingIdempotencyKey
	}
	var out envelope[orgprovider.LinkView]
	return c.client.Do(ctx, writeMeta(meta), http.MethodPost, c.apiBase+"/provider-links/"+url.PathEscape(current.Data.LinkID)+"/transitions",
		orgprovider.TransitionRequest{State: orgprovider.StateRevoked, ExpectedVersion: current.Data.SourceVersion,
			OrganizationID: organizationID, ReasonCode: reason}, &out)
}

func associationFrom(link orgprovider.LinkView) ProviderAssociation {
	out := ProviderAssociation{AssociationID: link.LinkID, Status: associationStatus(link.Status), SourceVersion: link.SourceVersion}
	if link.DecidedAt != nil {
		out.DecidedAt = link.DecidedAt.UTC()
	}
	return out
}

// associationStatus maps the owning service's link state onto Fleet's
// association vocabulary.
func associationStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case orgprovider.StateActive, orgprovider.StateSuspended:
		return AssociationAccepted
	case orgprovider.StateRejected:
		return AssociationDeclined
	case orgprovider.StateRevoked:
		return AssociationCancelled
	default:
		return AssociationAwaitingProvider
	}
}

var _ ProviderContract = (*ManagedProviderClient)(nil)
