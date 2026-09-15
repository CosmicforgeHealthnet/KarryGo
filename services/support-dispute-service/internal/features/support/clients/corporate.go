package supportclients

import (
	"context"
	"strings"
	"time"

	supportmodels "cosmicforge/logistics/services/support-dispute-service/internal/features/support/models"
	"cosmicforge/logistics/shared/go/providercompat"
)

// CorporateSnapshot is the organization dimension of a provider-raised case.
//
// It is a SNAPSHOT, recorded once when the case is created. An operator
// handling an emergency needs to know which organization's provider it is at
// the moment it happened; re-reading Fleet on every view would silently
// rewrite history the first time a provider changes organization, and would
// make an old case unreadable whenever Fleet is down.
type CorporateSnapshot struct {
	OrganizationReference string
	MembershipID          string
	Resolved              bool
}

// OrganizationResolver resolves the corporate organization behind a provider.
type OrganizationResolver interface {
	ResolveOrganization(ctx context.Context, complainantType supportmodels.ComplainantType, providerID string) CorporateSnapshot
}

// fleetServiceFor maps a complainant type to the service name Fleet knows it
// by. Customers have no corporate context, so they map to nothing.
func fleetServiceFor(complainantType supportmodels.ComplainantType) string {
	switch complainantType {
	case supportmodels.ComplainantTaxiProvider:
		return "taxi"
	case supportmodels.ComplainantDispatchProvider:
		return "dispatch"
	case supportmodels.ComplainantHaulingProvider:
		return "hauling"
	default:
		return ""
	}
}

// FleetOrganizationResolver reads Fleet's corporate context over the same
// HMAC v2 contract every execution service uses.
//
// It deliberately reuses providercompat.Client rather than adding a second
// corporate-context client: two readers of one contract drift, and the one
// that drifts is always the one nobody is looking at.
type FleetOrganizationResolver struct {
	reader providercompat.CorporateContextReader
	// timeout bounds the lookup independently of the client's own timeout.
	// A support case must not wait on Fleet: the case is the urgent thing,
	// the organization label is not.
	timeout time.Duration
}

// NewFleetOrganizationResolver builds a resolver, or nil when Fleet is not
// configured. A nil resolver is a working configuration, not an error: the
// platform ran without this dimension until now.
func NewFleetOrganizationResolver(baseURL string, secret []byte, timeout time.Duration) *FleetOrganizationResolver {
	if strings.TrimSpace(baseURL) == "" || len(secret) == 0 {
		return nil
	}
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &FleetOrganizationResolver{
		reader:  providercompat.NewClient(baseURL, sourceService, secret, timeout),
		timeout: timeout,
	}
}

// ResolveOrganization returns the provider's organization, or an unresolved
// snapshot.
//
// Every failure path is soft and silent to the caller. An unreachable Fleet,
// an unknown provider and an independent provider are all "no organization" —
// different causes, but the same correct outcome for the case, which is that
// it is created without the label. Blocking an SOS because a reporting
// dimension could not be filled in would be the wrong trade every time.
func (r *FleetOrganizationResolver) ResolveOrganization(
	ctx context.Context, complainantType supportmodels.ComplainantType, providerID string,
) CorporateSnapshot {
	if r == nil || r.reader == nil || strings.TrimSpace(providerID) == "" {
		return CorporateSnapshot{}
	}
	service := fleetServiceFor(complainantType)
	if service == "" {
		return CorporateSnapshot{}
	}

	lookup, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	corporate, err := r.reader.ReadCorporateContext(lookup, service, providerID)
	if err != nil {
		return CorporateSnapshot{}
	}
	if !corporate.IsManagedProvider || strings.TrimSpace(corporate.OrganizationID) == "" {
		// An independent provider genuinely has no organization. Recording one
		// would be a fabrication, not a default.
		return CorporateSnapshot{}
	}
	return CorporateSnapshot{
		OrganizationReference: strings.TrimSpace(corporate.OrganizationID),
		MembershipID:          strings.TrimSpace(corporate.MembershipID),
		Resolved:              true,
	}
}

// NoopOrganizationResolver resolves nothing. Used when Fleet is unconfigured
// and in tests.
type NoopOrganizationResolver struct{}

func (NoopOrganizationResolver) ResolveOrganization(
	context.Context, supportmodels.ComplainantType, string,
) CorporateSnapshot {
	return CorporateSnapshot{}
}
