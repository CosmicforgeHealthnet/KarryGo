package managedmembers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Service holds Section 10's member rules.
type Service struct {
	repo      *Repository
	authority ProviderAuthority
	enabled   bool
	now       func() time.Time
}

// Options configures the service.
type Options struct {
	Repository *Repository

	// Authority resolves a provider with its owning service before Fleet
	// records a link. Nil means no authority is reachable, and linking is
	// refused rather than recorded on trust.
	Authority ProviderAuthority

	// Enabled is FLEET_MULTI_SERVICE_PROVIDER_LINKS_ENABLED.
	Enabled bool

	Now func() time.Time
}

// NewService builds the service.
func NewService(options Options) *Service {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Service{repo: options.Repository, authority: options.Authority,
		enabled: options.Enabled, now: now}
}

// Enabled reports whether the member layer is active.
func (s *Service) Enabled() bool { return s != nil && s.enabled }

func (s *Service) gate() error {
	if !s.Enabled() {
		return ErrDisabled
	}
	return nil
}

// CreateMember records a person the organization employs.
func (s *Service) CreateMember(ctx context.Context, t tenant.Context, memberRef, displayName string) (Member, error) {
	if err := s.gate(); err != nil {
		return Member{}, err
	}
	if err := t.Validate(); err != nil {
		return Member{}, fmt.Errorf("%w: %s", ErrValidation, err)
	}
	if strings.TrimSpace(memberRef) == "" {
		return Member{}, fmt.Errorf("%w: a member reference is required", ErrValidation)
	}
	return s.repo.CreateMember(ctx, Member{
		ID:             ids.New("mbr"),
		OrganizationID: t.OrganizationID,
		MemberRef:      strings.TrimSpace(memberRef),
		DisplayName:    strings.TrimSpace(displayName),
		Status:         MemberActive,
		ConsentStatus:  ConsentPending,
	})
}

// ReadMember returns a member with every authority they hold.
//
// It is the read that makes the member layer worth having: one call answers
// "who is this person, and which of our three services do they work in", which
// previously required three queries and a human to correlate them.
func (s *Service) ReadMember(ctx context.Context, t tenant.Context, memberID string) (MemberView, error) {
	member, err := s.repo.GetMember(ctx, t.OrganizationID, memberID)
	if err != nil {
		return MemberView{}, err
	}
	links, err := s.repo.ListLinks(ctx, t.OrganizationID, memberID)
	if err != nil {
		return MemberView{}, err
	}
	assets, err := s.repo.ListAssets(ctx, t.OrganizationID, memberID)
	if err != nil {
		return MemberView{}, err
	}
	return MemberView{Member: member, Links: links, Assets: assets}, nil
}

// ListMembers returns the organization's members.
func (s *Service) ListMembers(ctx context.Context, t tenant.Context, limit int) ([]Member, error) {
	return s.repo.ListMembers(ctx, t.OrganizationID, limit)
}

// LinkRequest asks Fleet to bind one authority's provider to a member.
type LinkRequest struct {
	MemberID  string
	Authority Authority
	// ProviderRef is whatever the owning service can resolve: its own provider
	// id, or an identifier the organization supplies for it to look up. Fleet
	// does not interpret it and never generates one.
	ProviderRef string
	// MembershipID is the existing membership row this link corresponds to.
	MembershipID string
}

// LinkServiceIdentity binds one authority's provider identity to a member.
//
// The sequence is the whole point. Fleet asks the OWNING service to resolve
// the provider, refuses on a definite no, distinguishes an outage from a no,
// checks the organization does not already hold that provider, and only then
// records the link. At no step does Fleet create a provider id: doing so would
// give the platform two answers to "who is this driver", and the one Fleet
// invented would be the one nobody could dispatch.
func (s *Service) LinkServiceIdentity(ctx context.Context, t tenant.Context, request LinkRequest) (ServiceLink, error) {
	if err := s.gate(); err != nil {
		return ServiceLink{}, err
	}
	if err := t.Validate(); err != nil {
		return ServiceLink{}, fmt.Errorf("%w: %s", ErrValidation, err)
	}
	if strings.TrimSpace(request.MembershipID) == "" {
		return ServiceLink{}, fmt.Errorf("%w: a membership reference is required", ErrValidation)
	}
	if request.Authority.OwningService() == "" {
		return ServiceLink{}, fmt.Errorf("%w: %q is not a provider authority", ErrValidation, request.Authority)
	}

	// The member must exist in THIS organization. Reading it first is what
	// turns a mistyped member id into a not-found rather than a link quietly
	// attached to somebody else's person.
	member, err := s.repo.GetMember(ctx, t.OrganizationID, request.MemberID)
	if err != nil {
		return ServiceLink{}, err
	}

	if s.authority == nil {
		return ServiceLink{}, ErrAuthorityUnavailable
	}
	resolution, err := s.authority.ResolveProvider(ctx, request.Authority, request.ProviderRef)
	if err != nil {
		return ServiceLink{}, err
	}
	if strings.TrimSpace(resolution.ProviderID) == "" {
		// A resolution with no id is not a provider. Recording it would create
		// a link to nothing that later reads as a real corporate identity.
		return ServiceLink{}, ErrAuthorityRefused
	}

	// The membership being attached must be this organization's, must be for
	// the authority the caller named, and must already carry the provider the
	// authority just resolved. Skipping the last check would let a caller
	// resolve one provider and attach a membership for another, producing a
	// link whose provider nobody verified.
	membership, err := s.repo.GetMembership(ctx, t.OrganizationID, request.MembershipID)
	if err != nil {
		return ServiceLink{}, err
	}
	if membership.Authority != request.Authority {
		return ServiceLink{}, fmt.Errorf("%w: membership %s is a %s link, not %s",
			ErrValidation, request.MembershipID, membership.Authority, request.Authority)
	}
	if membership.ProviderID != resolution.ProviderID {
		return ServiceLink{}, fmt.Errorf("%w: membership %s names a different provider than the authority resolved",
			ErrValidation, request.MembershipID)
	}
	if membership.MemberID != "" {
		if membership.MemberID == member.ID {
			// Already attached to this very member: the caller retried.
			return membership, nil
		}
		return ServiceLink{}, ErrConflict
	}

	heldElsewhere, err := s.repo.ProviderHeldElsewhere(ctx, t.OrganizationID,
		request.Authority, resolution.ProviderID, request.MembershipID)
	if err != nil {
		return ServiceLink{}, err
	}
	if heldElsewhere {
		// The same provider already occupies this organization's slot for this
		// authority through a different membership. Linking again would give
		// one driver two corporate identities and two settlement slots.
		return ServiceLink{}, ErrConflict
	}

	return s.repo.AttachLink(ctx, t.OrganizationID, member.ID, request.MembershipID)
}

// SuspendMember withdraws a person from assignment organization-wide.
//
// It deliberately does NOT touch their individual service links. Those links
// are each authority's record of a qualification that has not changed, and
// clearing them would mean a reactivation had to re-resolve three providers
// and could silently fail on one. Assignability is the member gate; the links
// stay as they are, which is also what makes "suspended, then reinstated"
// return the person to exactly the state they left.
func (s *Service) SuspendMember(ctx context.Context, t tenant.Context, memberID string, expectedVersion int64) (Member, error) {
	if err := s.gate(); err != nil {
		return Member{}, err
	}
	return s.repo.SetMemberStatus(ctx, t.OrganizationID, memberID, expectedVersion,
		MemberSuspended, ConsentGranted, s.now().UTC())
}

// ReinstateMember returns a suspended member to assignment.
func (s *Service) ReinstateMember(ctx context.Context, t tenant.Context, memberID string, expectedVersion int64) (Member, error) {
	if err := s.gate(); err != nil {
		return Member{}, err
	}
	return s.repo.SetMemberStatus(ctx, t.OrganizationID, memberID, expectedVersion,
		MemberActive, ConsentGranted, s.now().UTC())
}

// GrantConsent records the member's consent to work for the organization.
func (s *Service) GrantConsent(ctx context.Context, t tenant.Context, memberID string, expectedVersion int64) (Member, error) {
	if err := s.gate(); err != nil {
		return Member{}, err
	}
	return s.repo.SetMemberStatus(ctx, t.OrganizationID, memberID, expectedVersion,
		MemberActive, ConsentGranted, s.now().UTC())
}

// LinkAssetRequest records an asset the organization operates.
type LinkAssetRequest struct {
	Authority     Authority
	AssetID       string
	MemberID      string
	SourceVersion int64
}

// LinkAsset records an organization's claim on an asset.
//
// Fleet stores a reference and a source version. It stores no capability, no
// certification and no availability, because every one of those is the owning
// service's answer and a cached copy here would be consulted after it went
// stale. An organization saying "this truck is ours" is not an organization
// saying the truck is roadworthy.
func (s *Service) LinkAsset(ctx context.Context, t tenant.Context, request LinkAssetRequest) (AssetLink, error) {
	if err := s.gate(); err != nil {
		return AssetLink{}, err
	}
	if request.Authority.OwningService() == "" {
		return AssetLink{}, fmt.Errorf("%w: %q is not an asset authority", ErrValidation, request.Authority)
	}
	if strings.TrimSpace(request.AssetID) == "" {
		return AssetLink{}, fmt.Errorf("%w: an asset reference is required", ErrValidation)
	}
	if request.MemberID != "" {
		if _, err := s.repo.GetMember(ctx, t.OrganizationID, request.MemberID); err != nil {
			return AssetLink{}, err
		}
	}
	return s.repo.LinkAsset(ctx, AssetLink{
		ID: ids.New("mal"), OrganizationID: t.OrganizationID,
		Authority: request.Authority, AssetID: strings.TrimSpace(request.AssetID),
		MemberID: request.MemberID, LinkStatus: AssetLinkActive,
		SourceVersion: request.SourceVersion,
	})
}

// ListAssets returns the organization's asset links.
func (s *Service) ListAssets(ctx context.Context, t tenant.Context, memberID string) ([]AssetLink, error) {
	return s.repo.ListAssets(ctx, t.OrganizationID, memberID)
}

// SetAssetLinkStatus suspends or removes an asset link.
func (s *Service) SetAssetLinkStatus(ctx context.Context, t tenant.Context, linkID string,
	expectedVersion int64, status string) (AssetLink, error) {
	if err := s.gate(); err != nil {
		return AssetLink{}, err
	}
	switch status {
	case AssetLinkActive, AssetLinkSuspended, AssetLinkRemoved:
	default:
		return AssetLink{}, fmt.Errorf("%w: %q is not an asset link status", ErrValidation, status)
	}
	return s.repo.SetAssetLinkStatus(ctx, t.OrganizationID, linkID, expectedVersion,
		status, s.now().UTC())
}

// AssignableForAuthority answers whether a member may be offered work by one
// authority right now.
//
// Two gates, in this order. The organization's own gate comes first because it
// is Fleet's to decide and costs nothing. The authority's link comes second,
// and its absence is final: Fleet may narrow who an authority has qualified,
// and may never widen it. An organization cannot decide that a driver a taxi
// service has not qualified is nonetheless a taxi driver.
func (s *Service) AssignableForAuthority(ctx context.Context, t tenant.Context,
	memberID string, authority Authority) (bool, string, error) {
	member, err := s.repo.GetMember(ctx, t.OrganizationID, memberID)
	if err != nil {
		return false, "", err
	}
	if !member.Assignable() {
		return false, "MEMBER_NOT_ASSIGNABLE", nil
	}
	link, err := s.repo.LinkForAuthority(ctx, t.OrganizationID, memberID, authority)
	if err != nil {
		if err == ErrNotFound {
			return false, "NO_LINK_FOR_AUTHORITY", nil
		}
		return false, "", err
	}
	if link.LinkStatus != "ACTIVE" {
		return false, "AUTHORITY_LINK_" + link.LinkStatus, nil
	}
	return true, "", nil
}
