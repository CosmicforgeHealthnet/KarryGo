package platformclients

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

var ErrMissingIdempotencyKey = errors.New("platformclients: idempotency key is required for a Hauling mutation")

// HaulingProviderClient implements Fleet's managed-provider contract against
// Hauling's private authority. Fleet stores only opaque IDs and versions.
type HaulingProviderClient struct {
	client  *Client
	apiBase string
}

func NewHaulingProviderClient(opts Options) *HaulingProviderClient {
	if opts.ServiceName == "" {
		opts.ServiceName = "driver-hauling-service"
	}
	return &HaulingProviderClient{client: New(opts), apiBase: haulingInternalBase}
}

func (c *HaulingProviderClient) Configured() bool { return c != nil && c.client.Configured() }

type haulingEnvelope[T any] struct {
	Data T `json:"data"`
}

func (c *HaulingProviderClient) Resolve(ctx context.Context, meta CallMeta, request ProviderResolveRequest) (ProviderResolution, error) {
	if !c.Configured() {
		return ProviderResolution{}, ErrNotConfigured
	}
	var out haulingEnvelope[ProviderResolution]
	err := c.client.Do(ctx, meta, http.MethodPost, c.apiBase+"/providers/resolve", map[string]any{
		"emailNormalized": request.EmailNormalized,
		"phoneE164":       request.PhoneE164,
	}, &out)
	return out.Data, err
}

func (c *HaulingProviderClient) ProvisionSponsored(ctx context.Context, meta CallMeta, request ProviderProvisionRequest) (ProviderIdentity, error) {
	if !c.Configured() {
		return ProviderIdentity{}, ErrNotConfigured
	}
	meta.OrganizationID, meta.IdempotencyKey = request.OrganizationID, request.IdempotencyKey
	var out haulingEnvelope[ProviderIdentity]
	err := c.client.Do(ctx, meta, http.MethodPost, c.apiBase+"/providers/sponsored", map[string]any{
		"organizationId": request.OrganizationID, "legalName": request.LegalName,
		"emailNormalized": request.EmailNormalized, "phoneE164": request.PhoneE164,
		"serviceCategory": request.ServiceCategory, "onboardingMode": request.OnboardingMode,
		"providerMode": request.ProviderMode,
	}, &out)
	return out.Data, err
}

func (c *HaulingProviderClient) ReadProvider(ctx context.Context, meta CallMeta, providerID string) (ProviderIdentity, error) {
	if !c.Configured() {
		return ProviderIdentity{}, ErrNotConfigured
	}
	var out haulingEnvelope[struct {
		ProviderID    string `json:"providerId"`
		AccountState  string `json:"accountState"`
		SourceVersion int64  `json:"sourceVersion"`
	}]
	err := c.client.Do(ctx, meta, http.MethodGet, c.apiBase+"/providers/"+url.PathEscape(providerID), nil, &out)
	return ProviderIdentity{ProviderID: out.Data.ProviderID, PlatformStatus: out.Data.AccountState, SourceVersion: out.Data.SourceVersion}, err
}

func (c *HaulingProviderClient) CreateAssociation(ctx context.Context, meta CallMeta, request ProviderAssociationRequest) (ProviderAssociation, error) {
	if !c.Configured() {
		return ProviderAssociation{}, ErrNotConfigured
	}
	meta.OrganizationID, meta.IdempotencyKey = request.OrganizationID, request.IdempotencyKey
	var out haulingEnvelope[struct {
		LinkID        string `json:"linkId"`
		Status        string `json:"status"`
		SourceVersion int64  `json:"sourceVersion"`
	}]
	err := c.client.Do(ctx, meta, http.MethodPost, c.apiBase+"/provider-links", map[string]any{
		"organizationId": request.OrganizationID, "providerId": request.ProviderID,
		"membershipId": request.MembershipID,
	}, &out)
	return ProviderAssociation{AssociationID: out.Data.LinkID, Status: associationStatus(out.Data.Status), SourceVersion: out.Data.SourceVersion}, err
}

func (c *HaulingProviderClient) ReadAssociation(ctx context.Context, meta CallMeta, associationID string) (ProviderAssociation, error) {
	if !c.Configured() {
		return ProviderAssociation{}, ErrNotConfigured
	}
	var out haulingEnvelope[struct {
		LinkID        string `json:"linkId"`
		Status        string `json:"status"`
		SourceVersion int64  `json:"sourceVersion"`
	}]
	err := c.client.Do(ctx, meta, http.MethodGet, c.apiBase+"/provider-links/"+url.PathEscape(associationID), nil, &out)
	return ProviderAssociation{AssociationID: out.Data.LinkID, Status: associationStatus(out.Data.Status), SourceVersion: out.Data.SourceVersion}, err
}

func (c *HaulingProviderClient) RevokeOrganizationEligibility(ctx context.Context, meta CallMeta, providerID, organizationID, reason string) error {
	if !c.Configured() {
		return ErrNotConfigured
	}
	meta.OrganizationID = organizationID
	var current haulingEnvelope[struct {
		LinkID        string `json:"linkId"`
		SourceVersion int64  `json:"sourceVersion"`
	}]
	err := c.client.Do(ctx, meta, http.MethodGet, c.apiBase+"/provider-links/by-provider/"+url.PathEscape(providerID), nil, &current)
	if err != nil {
		return err
	}
	if strings.TrimSpace(meta.IdempotencyKey) == "" {
		meta.IdempotencyKey = strings.TrimSpace(meta.RequestID)
	}
	if meta.IdempotencyKey == "" {
		return ErrMissingIdempotencyKey
	}
	var out haulingEnvelope[any]
	return c.client.Do(ctx, meta, http.MethodPost, c.apiBase+"/provider-links/"+url.PathEscape(current.Data.LinkID)+"/transitions", map[string]any{
		"state": "REVOKED", "expectedVersion": current.Data.SourceVersion,
		"organizationId": organizationID, "reasonCode": reason,
	}, &out)
}

func associationStatus(status string) string {
	switch strings.ToUpper(status) {
	case "ACTIVE":
		return AssociationAccepted
	case "REJECTED":
		return AssociationDeclined
	case "REVOKED":
		return AssociationCancelled
	default:
		return AssociationAwaitingProvider
	}
}

func (c *HaulingProviderClient) CreateProviderLink(ctx context.Context, meta CallMeta, request HaulingLinkRequest) (HaulingLink, error) {
	return c.createLink(ctx, meta, "provider", request)
}

func (c *HaulingProviderClient) CreateTruckLink(ctx context.Context, meta CallMeta, request HaulingLinkRequest) (HaulingLink, error) {
	return c.createLink(ctx, meta, "truck", request)
}

func (c *HaulingProviderClient) createLink(ctx context.Context, meta CallMeta, kind string, request HaulingLinkRequest) (HaulingLink, error) {
	if !c.Configured() {
		return HaulingLink{}, ErrNotConfigured
	}
	meta.OrganizationID, meta.IdempotencyKey = request.OrganizationID, request.IdempotencyKey
	var out haulingEnvelope[HaulingLink]
	err := c.client.Do(ctx, meta, http.MethodPost, c.apiBase+"/"+kind+"-links", map[string]any{
		"organizationId": request.OrganizationID, "branchId": request.BranchID,
		"membershipId": request.MembershipID, "providerId": request.ProviderID, "truckId": request.TruckID,
	}, &out)
	return out.Data, err
}

func (c *HaulingProviderClient) ReadProviderLink(ctx context.Context, meta CallMeta, linkID string) (HaulingLink, error) {
	return c.readLink(ctx, meta, "provider", linkID)
}

func (c *HaulingProviderClient) ReadTruckLink(ctx context.Context, meta CallMeta, linkID string) (HaulingLink, error) {
	return c.readLink(ctx, meta, "truck", linkID)
}

func (c *HaulingProviderClient) readLink(ctx context.Context, meta CallMeta, kind, linkID string) (HaulingLink, error) {
	if !c.Configured() {
		return HaulingLink{}, ErrNotConfigured
	}
	var out haulingEnvelope[HaulingLink]
	err := c.client.Do(ctx, meta, http.MethodGet, c.apiBase+"/"+kind+"-links/"+url.PathEscape(linkID), nil, &out)
	return out.Data, err
}

func (c *HaulingProviderClient) TransitionProviderLink(ctx context.Context, meta CallMeta, linkID string, transition HaulingLinkTransition) (HaulingLink, error) {
	return c.transitionLink(ctx, meta, "provider", linkID, transition)
}

func (c *HaulingProviderClient) TransitionTruckLink(ctx context.Context, meta CallMeta, linkID string, transition HaulingLinkTransition) (HaulingLink, error) {
	return c.transitionLink(ctx, meta, "truck", linkID, transition)
}

func (c *HaulingProviderClient) transitionLink(ctx context.Context, meta CallMeta, kind, linkID string, transition HaulingLinkTransition) (HaulingLink, error) {
	if !c.Configured() {
		return HaulingLink{}, ErrNotConfigured
	}
	meta.OrganizationID, meta.IdempotencyKey = transition.OrganizationID, transition.IdempotencyKey
	var out haulingEnvelope[HaulingLink]
	err := c.client.Do(ctx, meta, http.MethodPost, c.apiBase+"/"+kind+"-links/"+url.PathEscape(linkID)+"/transitions", map[string]any{
		"state": transition.State, "expectedVersion": transition.ExpectedVersion,
		"organizationId": transition.OrganizationID, "branchId": transition.BranchID, "reasonCode": transition.ReasonCode,
	}, &out)
	return out.Data, err
}

var _ ProviderContract = (*HaulingProviderClient)(nil)
var _ HaulingLinkContract = (*HaulingProviderClient)(nil)
