package managedmembers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// EventPublisher appends a domain event to the transactional outbox.
type EventPublisher interface {
	Publish(ctx context.Context, tx pgx.Tx, eventType, aggregateType, aggregateID string,
		aggregateVersion int64, organizationID, correlationID string, payload any) error
}

// Service holds Section 10's member rules.
//
// Every mutation runs in one transaction with its audit record and its outbox
// event (spec 6l): a member suspended without a trace, or a link no consumer
// ever hears about, is a state change nobody can reconstruct.
type Service struct {
	repo      *Repository
	pool      *pgxpool.Pool
	audit     *audit.Recorder
	events    EventPublisher
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

	// Pool runs the write transactions. Defaults to the repository's pool.
	Pool *pgxpool.Pool
	// Audit defaults to the platform recorder.
	Audit *audit.Recorder
	// Events defaults to the transactional outbox over Pool.
	Events EventPublisher

	Now func() time.Time
}

// NewService builds the service.
func NewService(options Options) *Service {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	pool := options.Pool
	if pool == nil {
		pool = options.Repository.pool()
	}
	recorder := options.Audit
	if recorder == nil {
		recorder = audit.NewRecorder()
	}
	publisher := options.Events
	if publisher == nil && pool != nil {
		publisher = events.NewDomainPublisher(events.NewOutboxRepository(pool))
	}
	return &Service{repo: options.Repository, pool: pool, audit: recorder, events: publisher,
		authority: options.Authority, enabled: options.Enabled, now: now}
}

// mutate runs fn in a transaction and records its audit entry and outbox
// event in the same transaction. fn returns the aggregate id and version and
// the values to record; a zero id means fn made no change (an idempotent
// retry) and nothing is recorded.
func (s *Service) mutate(ctx context.Context, t tenant.Context, event, aggregate string,
	fn func(r *Repository) (id string, version int64, previous, next map[string]any, err error)) error {
	if s.pool == nil {
		return fmt.Errorf("managedmembers: no database pool configured")
	}
	return database.WithinTx(ctx, s.pool, func(tx pgx.Tx) error {
		id, version, previous, next, err := fn(s.repo.with(tx))
		if err != nil || id == "" {
			return err
		}
		entry := audit.EntryFromTenant(t, strings.TrimSuffix(event, ".v1"), aggregate, id)
		entry.PreviousValues = previous
		entry.NewValues = next
		if err := s.audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		if s.events == nil {
			return nil
		}
		payload := map[string]any{"id": id}
		for k, v := range next {
			payload[k] = v
		}
		return s.events.Publish(ctx, tx, event, aggregate, id, version, t.OrganizationID, t.CorrelationID, payload)
	})
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
	var created Member
	err := s.mutate(ctx, t, "managed_member.created.v1", "managed_member",
		func(r *Repository) (string, int64, map[string]any, map[string]any, error) {
			var err error
			created, err = r.CreateMember(ctx, Member{
				ID:             ids.New("mbr"),
				OrganizationID: t.OrganizationID,
				MemberRef:      strings.TrimSpace(memberRef),
				DisplayName:    strings.TrimSpace(displayName),
				Status:         MemberActive,
				ConsentStatus:  ConsentPending,
			})
			if err != nil {
				return "", 0, nil, nil, err
			}
			return created.ID, created.Version, nil, map[string]any{
				"memberRef": created.MemberRef, "status": created.Status, "consentStatus": created.ConsentStatus,
			}, nil
		})
	if err != nil {
		return Member{}, err
	}
	return created, nil
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
	assets, _, err := s.ListAssetsPage(ctx, t, memberID, Page{})
	if err != nil {
		return MemberView{}, err
	}
	return MemberView{Member: member, Links: links, Assets: assets}, nil
}

// Page selects one keyset page of a list. A zero Limit means MaxPageSize.
type Page struct {
	Limit   int
	After   *time.Time
	AfterID string
}

func (p Page) size() int {
	if p.Limit <= 0 || p.Limit > MaxPageSize {
		return MaxPageSize
	}
	return p.Limit
}

// ListMembers returns the first page (newest first) of the organization's
// members, at most limit (default and maximum MaxPageSize).
func (s *Service) ListMembers(ctx context.Context, t tenant.Context, limit int) ([]Member, error) {
	members, _, err := s.ListMembersPage(ctx, t, Page{Limit: limit})
	return members, err
}

// ListMembersPage returns one page of members and whether more exist.
func (s *Service) ListMembersPage(ctx context.Context, t tenant.Context, page Page) ([]Member, bool, error) {
	size := page.size()
	members, err := s.repo.ListMembers(ctx, t.OrganizationID, size+1, page.After, page.AfterID)
	if err != nil || len(members) <= size {
		return members, false, err
	}
	return members[:size], true, nil
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

	var attached ServiceLink
	err = s.mutate(ctx, t, "managed_member.service_linked.v1", "managed_member",
		func(r *Repository) (string, int64, map[string]any, map[string]any, error) {
			var err error
			attached, err = r.AttachLink(ctx, t.OrganizationID, member.ID, request.MembershipID)
			if err != nil {
				return "", 0, nil, nil, err
			}
			return member.ID, member.Version, nil, map[string]any{
				"memberId": member.ID, "membershipId": attached.MembershipID,
				"authority": string(attached.Authority), "providerId": attached.ProviderID,
			}, nil
		})
	if err != nil {
		return ServiceLink{}, err
	}
	return attached, nil
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
	return s.setMemberStatus(ctx, t, "managed_member.suspended.v1", memberID, expectedVersion, MemberSuspended)
}

// ReinstateMember returns a suspended member to assignment.
func (s *Service) ReinstateMember(ctx context.Context, t tenant.Context, memberID string, expectedVersion int64) (Member, error) {
	if err := s.gate(); err != nil {
		return Member{}, err
	}
	return s.setMemberStatus(ctx, t, "managed_member.reinstated.v1", memberID, expectedVersion, MemberActive)
}

// GrantConsent records the member's consent to work for the organization.
func (s *Service) GrantConsent(ctx context.Context, t tenant.Context, memberID string, expectedVersion int64) (Member, error) {
	if err := s.gate(); err != nil {
		return Member{}, err
	}
	return s.setMemberStatus(ctx, t, "managed_member.consent_granted.v1", memberID, expectedVersion, MemberActive)
}

// setMemberStatus applies one organization-level standing change under its
// version guard, recording the previous standing for the audit trail.
func (s *Service) setMemberStatus(ctx context.Context, t tenant.Context, event, memberID string,
	expectedVersion int64, status string) (Member, error) {
	var updated Member
	err := s.mutate(ctx, t, event, "managed_member",
		func(r *Repository) (string, int64, map[string]any, map[string]any, error) {
			previous, err := r.GetMember(ctx, t.OrganizationID, memberID)
			if err != nil {
				// Reported as a version conflict, like the update itself, so a
				// caller cannot probe other organizations' member ids.
				return "", 0, nil, nil, ErrVersion
			}
			updated, err = r.SetMemberStatus(ctx, t.OrganizationID, memberID, expectedVersion,
				status, ConsentGranted, s.now().UTC())
			if err != nil {
				return "", 0, nil, nil, err
			}
			return updated.ID, updated.Version,
				map[string]any{"status": previous.Status, "consentStatus": previous.ConsentStatus, "version": previous.Version},
				map[string]any{"status": updated.Status, "consentStatus": updated.ConsentStatus, "version": updated.Version},
				nil
		})
	if err != nil {
		return Member{}, err
	}
	return updated, nil
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
	var linked AssetLink
	err := s.mutate(ctx, t, "managed_asset_link.linked.v1", "managed_asset_link",
		func(r *Repository) (string, int64, map[string]any, map[string]any, error) {
			var err error
			linked, err = r.LinkAsset(ctx, AssetLink{
				ID: ids.New("mal"), OrganizationID: t.OrganizationID,
				Authority: request.Authority, AssetID: strings.TrimSpace(request.AssetID),
				MemberID: request.MemberID, LinkStatus: AssetLinkActive,
				SourceVersion: request.SourceVersion,
			})
			if err != nil {
				return "", 0, nil, nil, err
			}
			return linked.ID, linked.Version, nil, map[string]any{
				"authority": string(linked.Authority), "assetId": linked.AssetID,
				"memberId": linked.MemberID, "linkStatus": linked.LinkStatus, "sourceVersion": linked.SourceVersion,
			}, nil
		})
	if err != nil {
		return AssetLink{}, err
	}
	return linked, nil
}

// ListAssets returns the first page (newest first) of the organization's
// asset links, optionally for one member.
func (s *Service) ListAssets(ctx context.Context, t tenant.Context, memberID string) ([]AssetLink, error) {
	assets, _, err := s.ListAssetsPage(ctx, t, memberID, Page{})
	return assets, err
}

// ListAssetsPage returns one page of asset links and whether more exist.
func (s *Service) ListAssetsPage(ctx context.Context, t tenant.Context, memberID string, page Page) ([]AssetLink, bool, error) {
	size := page.size()
	assets, err := s.repo.ListAssets(ctx, t.OrganizationID, memberID, size+1, page.After, page.AfterID)
	if err != nil || len(assets) <= size {
		return assets, false, err
	}
	return assets[:size], true, nil
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
	var updated AssetLink
	err := s.mutate(ctx, t, "managed_asset_link.status_changed.v1", "managed_asset_link",
		func(r *Repository) (string, int64, map[string]any, map[string]any, error) {
			var err error
			updated, err = r.SetAssetLinkStatus(ctx, t.OrganizationID, linkID, expectedVersion,
				status, s.now().UTC())
			if err != nil {
				return "", 0, nil, nil, err
			}
			return updated.ID, updated.Version, map[string]any{"version": expectedVersion},
				map[string]any{"linkStatus": updated.LinkStatus, "version": updated.Version}, nil
		})
	if err != nil {
		return AssetLink{}, err
	}
	return updated, nil
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
