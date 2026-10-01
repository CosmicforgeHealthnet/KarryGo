// Package managedmembers is Section 10's organization-member layer: one
// logical person who may hold a Dispatch, a Taxi and a Hauling provider
// identity at the same time.
//
// Fleet has always modelled the relationship between an organization and a
// provider, one row per service category. What it could not say was that three
// of those rows are the same human. An organization employing someone who
// rides a bike in the morning and drives a taxi in the afternoon held two
// unrelated memberships and no way to connect them, so "suspend this person"
// meant finding every row by hand and hoping none was missed.
//
// This package adds only the layer above. It does not replace the membership
// rows, does not duplicate provider identity, and emphatically does not mint
// one: provider ids come from the owning driver service through a
// resolve-before-link call, and Fleet stores the answer as an opaque
// reference. A member record is an organization's view of a person, never an
// operational profile.
package managedmembers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Authority is a driver service that owns provider identity.
//
// The values match the service_category column that already exists on
// memberships, because they are the same concept and a second vocabulary would
// need translating on every read.
type Authority string

const (
	AuthorityDispatch Authority = "DISPATCH"
	AuthorityTaxi     Authority = "TAXI"
	AuthorityHauling  Authority = "HAULING"
)

// Authorities lists every authority in a stable order.
func Authorities() []Authority {
	return []Authority{AuthorityDispatch, AuthorityTaxi, AuthorityHauling}
}

// ParseAuthority resolves a wire value.
func ParseAuthority(value string) (Authority, error) {
	switch Authority(strings.ToUpper(strings.TrimSpace(value))) {
	case AuthorityDispatch:
		return AuthorityDispatch, nil
	case AuthorityTaxi:
		return AuthorityTaxi, nil
	case AuthorityHauling:
		return AuthorityHauling, nil
	}
	return "", fmt.Errorf("%w: %q is not a provider authority", ErrValidation, value)
}

// OwningService is the service that holds this authority's canonical provider
// identity. Fleet calls it to resolve a provider; it never mints one itself.
func (a Authority) OwningService() string {
	switch a {
	case AuthorityDispatch:
		return "driver-dispatch-delivery-service"
	case AuthorityTaxi:
		return "driver-taxi-service"
	case AuthorityHauling:
		return "driver-hauling-service"
	}
	return ""
}

// Member statuses.
const (
	MemberActive    = "ACTIVE"
	MemberSuspended = "SUSPENDED"
	MemberEnded     = "ENDED"
)

// Consent statuses.
const (
	ConsentPending   = "PENDING"
	ConsentGranted   = "GRANTED"
	ConsentWithdrawn = "WITHDRAWN"
)

// Asset link statuses.
const (
	AssetLinkActive    = "ACTIVE"
	AssetLinkSuspended = "SUSPENDED"
	AssetLinkRemoved   = "REMOVED"
)

// Sentinel errors.
var (
	ErrValidation = errors.New("managedmembers: validation failed")
	ErrNotFound   = errors.New("managedmembers: not found")
	ErrConflict   = errors.New("managedmembers: conflict")
	ErrVersion    = errors.New("managedmembers: version conflict")
	ErrDisabled   = errors.New("managedmembers: multi-service provider links are not enabled")
	// ErrAuthorityRefused means the owning driver service declined to resolve
	// or qualify the provider. It is NOT overridable: an organization may add
	// its own restrictions on top of an authority's yes, but it may never turn
	// an authority's no into a yes about a provider it does not own.
	ErrAuthorityRefused = errors.New("managedmembers: the owning authority refused this provider")
	// ErrAuthorityUnavailable means the owning service could not be reached.
	// Distinct from a refusal on purpose: an outage is retryable and a refusal
	// is not, and reporting one as the other either loses work or invents a
	// negative fact about a provider nobody was able to ask about.
	ErrAuthorityUnavailable = errors.New("managedmembers: the owning authority could not be reached")
)

// Member is one organization's record of a person who works for it.
type Member struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	MemberRef      string     `json:"member_ref"`
	DisplayName    string     `json:"display_name,omitempty"`
	Status         string     `json:"status"`
	ConsentStatus  string     `json:"consent_status"`
	SuspendedAt    *time.Time `json:"suspended_at,omitempty"`
	EndedAt        *time.Time `json:"ended_at,omitempty"`
	Version        int64      `json:"version"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// Assignable reports whether this member may be offered work at all.
//
// It is an organization-level gate that sits ON TOP of each service link's own
// standing. A member suspended by their organization is not assignable even
// where every individual authority still qualifies them, which is how
// "suspend this person" becomes one action rather than three.
func (m Member) Assignable() bool {
	return m.Status == MemberActive && m.ConsentStatus == ConsentGranted
}

// ServiceLink is one authority's provider identity, as the organization holds
// it. It projects the existing membership row rather than duplicating it.
type ServiceLink struct {
	MembershipID   string     `json:"membership_id"`
	MemberID       string     `json:"member_id,omitempty"`
	OrganizationID string     `json:"organization_id"`
	Authority      Authority  `json:"authority"`
	ProviderID     string     `json:"provider_id,omitempty"`
	LinkStatus     string     `json:"link_status"`
	SourceVersion  int64      `json:"source_version"`
	LinkedAt       *time.Time `json:"linked_at,omitempty"`
	SuspendedAt    *time.Time `json:"suspended_at,omitempty"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// Live reports whether the link still occupies the provider's settlement slot
// for its authority.
func (l ServiceLink) Live() bool {
	switch l.LinkStatus {
	case "PENDING_PROVIDER_ACCEPTANCE", "PENDING_SETUP", "ACTIVE", "SUSPENDED", "ENDING":
		return true
	}
	return false
}

// MemberView is a member together with every authority they hold.
type MemberView struct {
	Member Member        `json:"member"`
	Links  []ServiceLink `json:"links"`
	Assets []AssetLink   `json:"assets"`
}

// AuthorityLink returns the member's link for one authority.
func (v MemberView) AuthorityLink(authority Authority) (ServiceLink, bool) {
	for _, link := range v.Links {
		if link.Authority == authority && link.Live() {
			return link, true
		}
	}
	return ServiceLink{}, false
}

// AssetLink is an asset the organization considers its own.
//
// Fleet references the asset; the owning service keeps every fact about it.
// There is deliberately no capability, no certification and no availability
// field here: an asset link asserts ownership, never fitness.
type AssetLink struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	Authority      Authority  `json:"authority"`
	AssetID        string     `json:"asset_id"`
	MemberID       string     `json:"member_id,omitempty"`
	LinkStatus     string     `json:"link_status"`
	SourceVersion  int64      `json:"source_version"`
	LinkedAt       time.Time  `json:"linked_at"`
	SuspendedAt    *time.Time `json:"suspended_at,omitempty"`
	RemovedAt      *time.Time `json:"removed_at,omitempty"`
	Version        int64      `json:"version"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// ProviderResolution is what an owning service answers when Fleet asks it to
// resolve a provider before linking.
//
// Fleet stores the id and the version and nothing else that matters
// operationally. QualificationStatus is a projection for display and for
// candidate filtering; it is never treated as permission, because permission
// is the authority's to grant at the moment work is offered, not Fleet's to
// cache from an earlier conversation.
type ProviderResolution struct {
	ProviderID          string
	QualificationStatus string
	DisplayName         string
	SourceVersion       int64
}

// ProviderAuthority is the resolve-before-link contract.
//
// It returns a resolution or one of the two distinguishable failures. An
// implementation that cannot reach its service must return
// ErrAuthorityUnavailable rather than an empty resolution: a zero ProviderID
// that reads as success would let Fleet record a link to a provider that no
// service has ever heard of.
type ProviderAuthority interface {
	ResolveProvider(ctx context.Context, authority Authority, providerRef string) (ProviderResolution, error)
}
