package managedmembers

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository owns this feature's statements.
//
// Every query is organization-scoped in SQL, not by a caller remembering to
// filter. Tenant isolation that depends on each call site passing the right
// argument fails the first time somebody adds a call site.
type Repository struct{ db *pgxpool.Pool }

// NewRepository builds the repository.
func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const memberColumns = `id, organization_id, member_ref, display_name, status,
	consent_status, suspended_at, ended_at, version, created_at, updated_at`

func scanMember(row pgx.Row) (Member, error) {
	var m Member
	err := row.Scan(&m.ID, &m.OrganizationID, &m.MemberRef, &m.DisplayName, &m.Status,
		&m.ConsentStatus, &m.SuspendedAt, &m.EndedAt, &m.Version, &m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, err
}

// CreateMember records a person the organization employs.
func (r *Repository) CreateMember(ctx context.Context, m Member) (Member, error) {
	created, err := scanMember(r.db.QueryRow(ctx, `
		INSERT INTO fleet_partner_managed_provider_members(
			id, organization_id, member_ref, display_name, status, consent_status)
		VALUES($1,$2,$3,$4,$5,$6)
		RETURNING `+memberColumns,
		m.ID, m.OrganizationID, m.MemberRef, m.DisplayName, m.Status, m.ConsentStatus))
	if isUniqueViolation(err) {
		return Member{}, ErrConflict
	}
	return created, err
}

// GetMember reads one member, scoped to the organization that owns it.
func (r *Repository) GetMember(ctx context.Context, organizationID, memberID string) (Member, error) {
	return scanMember(r.db.QueryRow(ctx,
		`SELECT `+memberColumns+` FROM fleet_partner_managed_provider_members
		  WHERE id=$1 AND organization_id=$2`, memberID, organizationID))
}

// ListMembers returns an organization's members.
func (r *Repository) ListMembers(ctx context.Context, organizationID string, limit int) ([]Member, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.db.Query(ctx,
		`SELECT `+memberColumns+` FROM fleet_partner_managed_provider_members
		  WHERE organization_id=$1 ORDER BY created_at DESC LIMIT $2`, organizationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var m Member
		if err = rows.Scan(&m.ID, &m.OrganizationID, &m.MemberRef, &m.DisplayName, &m.Status,
			&m.ConsentStatus, &m.SuspendedAt, &m.EndedAt, &m.Version, &m.CreatedAt,
			&m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SetMemberStatus moves a member's organization-level standing.
//
// It guards on version so two operators acting on a stale read cannot both
// win. A suspension lost to a concurrent reactivation is a person still being
// offered work after somebody decided they should not be.
func (r *Repository) SetMemberStatus(ctx context.Context, organizationID, memberID string,
	expectedVersion int64, status, consent string, now time.Time) (Member, error) {
	var suspendedAt, endedAt *time.Time
	switch status {
	case MemberSuspended:
		suspendedAt = &now
	case MemberEnded:
		endedAt = &now
	}
	// The timestamp parameters are cast explicitly. Inside a CASE expression
	// PostgreSQL has nothing to infer a bare parameter's type from and settles
	// on text, which then fails against a timestamptz column -- a failure that
	// looks like a schema mismatch rather than a missing annotation.
	updated, err := scanMember(r.db.QueryRow(ctx, `
		UPDATE fleet_partner_managed_provider_members
		   SET status=$4, consent_status=$5,
		       suspended_at=CASE WHEN $4='SUSPENDED' THEN $6::timestamptz ELSE NULL END,
		       ended_at=COALESCE($7::timestamptz, ended_at),
		       version=version+1, updated_at=now()
		 WHERE id=$1 AND organization_id=$2 AND version=$3
		RETURNING `+memberColumns,
		memberID, organizationID, expectedVersion, status, consent, suspendedAt, endedAt))
	if errors.Is(err, ErrNotFound) {
		// Either no such member in this tenant, or the version moved. Both are
		// reported as a version conflict rather than "not found": telling an
		// unauthorized caller which of the two it was would confirm whether a
		// member id exists in another organization.
		return Member{}, ErrVersion
	}
	return updated, err
}

const linkColumns = `id, COALESCE(member_id,''), organization_id, service_category,
	COALESCE(provider_id,''), status, source_version, joined_at, suspended_at, updated_at`

func scanLink(row pgx.Row) (ServiceLink, error) {
	var l ServiceLink
	err := row.Scan(&l.MembershipID, &l.MemberID, &l.OrganizationID, &l.Authority,
		&l.ProviderID, &l.LinkStatus, &l.SourceVersion, &l.LinkedAt, &l.SuspendedAt, &l.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return l, ErrNotFound
	}
	return l, err
}

// ListLinks returns every service link a member holds.
func (r *Repository) ListLinks(ctx context.Context, organizationID, memberID string) ([]ServiceLink, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+linkColumns+` FROM fleet_partner_organization_provider_memberships
		  WHERE organization_id=$1 AND member_id=$2 ORDER BY service_category`,
		organizationID, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ServiceLink{}
	for rows.Next() {
		var l ServiceLink
		if err = rows.Scan(&l.MembershipID, &l.MemberID, &l.OrganizationID, &l.Authority,
			&l.ProviderID, &l.LinkStatus, &l.SourceVersion, &l.LinkedAt, &l.SuspendedAt,
			&l.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// AttachLink binds an existing membership row to a member.
//
// The membership is matched on organization as well as id, so a membership
// belonging to another organization cannot be pulled into this one's member
// even with a correct membership id.
func (r *Repository) AttachLink(ctx context.Context, organizationID, memberID, membershipID string) (ServiceLink, error) {
	attached, err := scanLink(r.db.QueryRow(ctx, `
		UPDATE fleet_partner_organization_provider_memberships
		   SET member_id=$3, updated_at=now()
		 WHERE id=$1 AND organization_id=$2
		RETURNING `+linkColumns, membershipID, organizationID, memberID))
	if isUniqueViolation(err) {
		// The member already holds a live link for this authority.
		return ServiceLink{}, ErrConflict
	}
	return attached, err
}

// LinkForAuthority finds a member's live link for one authority.
func (r *Repository) LinkForAuthority(ctx context.Context, organizationID, memberID string,
	authority Authority) (ServiceLink, error) {
	return scanLink(r.db.QueryRow(ctx,
		`SELECT `+linkColumns+` FROM fleet_partner_organization_provider_memberships
		  WHERE organization_id=$1 AND member_id=$2 AND service_category=$3
		    AND status IN ('PENDING_PROVIDER_ACCEPTANCE','PENDING_SETUP','ACTIVE','SUSPENDED','ENDING')`,
		organizationID, memberID, authority))
}

// GetMembership reads one membership row, scoped to its organization.
//
// It is read before attaching so the link can be checked against what the
// authority actually resolved: attaching a membership for provider X while
// claiming to have resolved provider Y would record a corporate identity
// nobody verified.
func (r *Repository) GetMembership(ctx context.Context, organizationID, membershipID string) (ServiceLink, error) {
	return scanLink(r.db.QueryRow(ctx,
		`SELECT `+linkColumns+` FROM fleet_partner_organization_provider_memberships
		  WHERE id=$1 AND organization_id=$2`, membershipID, organizationID))
}

// ProviderHeldElsewhere reports whether some OTHER live membership in this
// organization already holds the same authority's provider.
//
// The exclusion of the membership being attached is the whole point. A
// membership row for this provider is expected to exist -- it is what is being
// attached -- so a check that merely asked "is this provider linked?" would
// answer yes to the row it is about to link and refuse every first link.
func (r *Repository) ProviderHeldElsewhere(ctx context.Context, organizationID string,
	authority Authority, providerID, excludeMembershipID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM fleet_partner_organization_provider_memberships
			 WHERE organization_id=$1 AND service_category=$2 AND provider_id=$3
			   AND id <> $4
			   AND status IN ('PENDING_PROVIDER_ACCEPTANCE','PENDING_SETUP','ACTIVE','SUSPENDED','ENDING'))`,
		organizationID, authority, providerID, excludeMembershipID).Scan(&exists)
	return exists, err
}

const assetColumns = `id, organization_id, authority, asset_id, COALESCE(member_id,''),
	link_status, source_version, linked_at, suspended_at, removed_at, version, updated_at`

func scanAsset(row pgx.Row) (AssetLink, error) {
	var a AssetLink
	err := row.Scan(&a.ID, &a.OrganizationID, &a.Authority, &a.AssetID, &a.MemberID,
		&a.LinkStatus, &a.SourceVersion, &a.LinkedAt, &a.SuspendedAt, &a.RemovedAt,
		&a.Version, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// LinkAsset records an asset the organization operates.
func (r *Repository) LinkAsset(ctx context.Context, a AssetLink) (AssetLink, error) {
	var memberID *string
	if a.MemberID != "" {
		memberID = &a.MemberID
	}
	created, err := scanAsset(r.db.QueryRow(ctx, `
		INSERT INTO fleet_partner_managed_asset_links(
			id, organization_id, authority, asset_id, member_id, link_status, source_version)
		VALUES($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (organization_id, authority, asset_id) DO UPDATE
		   SET member_id=EXCLUDED.member_id, link_status=EXCLUDED.link_status,
		       source_version=EXCLUDED.source_version,
		       suspended_at=NULL, removed_at=NULL,
		       version=fleet_partner_managed_asset_links.version+1, updated_at=now()
		 WHERE fleet_partner_managed_asset_links.source_version <= EXCLUDED.source_version
		RETURNING `+assetColumns,
		a.ID, a.OrganizationID, a.Authority, a.AssetID, memberID, a.LinkStatus, a.SourceVersion))
	if errors.Is(err, ErrNotFound) {
		// The row exists at a newer source version: a stale report must not
		// overwrite a fresher one.
		return AssetLink{}, ErrVersion
	}
	return created, err
}

// ListAssets returns an organization's asset links, optionally for one member.
func (r *Repository) ListAssets(ctx context.Context, organizationID, memberID string) ([]AssetLink, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+assetColumns+` FROM fleet_partner_managed_asset_links
		  WHERE organization_id=$1 AND ($2='' OR member_id=$2)
		  ORDER BY authority, asset_id`, organizationID, memberID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AssetLink{}
	for rows.Next() {
		var a AssetLink
		if err = rows.Scan(&a.ID, &a.OrganizationID, &a.Authority, &a.AssetID, &a.MemberID,
			&a.LinkStatus, &a.SourceVersion, &a.LinkedAt, &a.SuspendedAt, &a.RemovedAt,
			&a.Version, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetAssetLinkStatus suspends or removes an asset link.
func (r *Repository) SetAssetLinkStatus(ctx context.Context, organizationID, linkID string,
	expectedVersion int64, status string, now time.Time) (AssetLink, error) {
	updated, err := scanAsset(r.db.QueryRow(ctx, `
		UPDATE fleet_partner_managed_asset_links
		   SET link_status=$4,
		       suspended_at=CASE WHEN $4='SUSPENDED' THEN $5 ELSE suspended_at END,
		       removed_at=CASE WHEN $4='REMOVED' THEN $5 ELSE removed_at END,
		       version=version+1, updated_at=now()
		 WHERE id=$1 AND organization_id=$2 AND version=$3
		RETURNING `+assetColumns, linkID, organizationID, expectedVersion, status, now))
	if errors.Is(err, ErrNotFound) {
		return AssetLink{}, ErrVersion
	}
	return updated, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
