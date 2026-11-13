package platformclients

import (
	"context"
	"errors"
	"net/http"
	"net/url"
)

var ErrMissingIdempotencyKey = errors.New("platformclients: idempotency key is required for a provider-service mutation")

// HaulingProviderClient is the Hauling managed-provider client plus the
// truck-link half of the contract that only Hauling has.
type HaulingProviderClient struct {
	*ManagedProviderClient
}

func NewHaulingProviderClient(opts Options) *HaulingProviderClient {
	if opts.ServiceName == "" {
		opts.ServiceName = "driver-hauling-service"
	}
	return &HaulingProviderClient{ManagedProviderClient: NewManagedProviderClient(opts, HaulingManagedProviderBase, CategoryHauling)}
}

type haulingEnvelope[T any] struct {
	Data T `json:"data"`
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
	err := c.client.Do(ctx, writeMeta(meta), http.MethodPost, c.apiBase+"/"+kind+"-links", map[string]any{
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
	err := c.client.Do(ctx, writeMeta(meta), http.MethodPost, c.apiBase+"/"+kind+"-links/"+url.PathEscape(linkID)+"/transitions", map[string]any{
		"state": transition.State, "expectedVersion": transition.ExpectedVersion,
		"organizationId": transition.OrganizationID, "branchId": transition.BranchID, "reasonCode": transition.ReasonCode,
	}, &out)
	return out.Data, err
}

var _ ProviderContract = (*HaulingProviderClient)(nil)
var _ HaulingLinkContract = (*HaulingProviderClient)(nil)
