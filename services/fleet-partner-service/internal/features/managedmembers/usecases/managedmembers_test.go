package usecases_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/managedmembers/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/managedmembers/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/managedmembers/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Section 10's member layer. The invariants worth testing are not "a row can be
// written" -- they are the ones a mistake would violate silently: that one
// person can hold three authorities, that Fleet cannot invent a provider
// identity, that one organization cannot reach another's member, and that
// suspending one authority does not suspend the rest.

// ── Unit: the rules that need no database ───────────────────────────────────

// emptyAuthority answers without naming a provider: the shape a buggy or
// half-migrated owning service would return.
type emptyAuthority struct{}

func (emptyAuthority) ResolveProvider(context.Context, models.Authority, string) (models.ProviderResolution, error) {
	return models.ProviderResolution{}, nil
}

// TestAuthorityOutageIsNotARefusal keeps the two failures apart.
//
// An unreachable authority has said nothing about the provider. Recording that
// as a refusal would be Fleet inventing a negative fact about a driver nobody
// was able to ask about, and the driver would stay unlinkable until somebody
// noticed and retried by hand.
func TestAuthorityOutageIsNotARefusal(t *testing.T) {
	if errors.Is(models.ErrAuthorityUnavailable, models.ErrAuthorityRefused) {
		t.Fatal("an outage must not satisfy a refusal check")
	}
	if errors.Is(models.ErrAuthorityRefused, models.ErrAuthorityUnavailable) {
		t.Fatal("a refusal must not satisfy an outage check")
	}
}

// TestOwningServiceMapping pins which service issues which authority's
// identities. A wrong entry would send a resolve to a service that has never
// heard of the provider and read its honest "no" as a refusal.
func TestOwningServiceMapping(t *testing.T) {
	expected := map[models.Authority]string{
		models.AuthorityDispatch: "driver-dispatch-delivery-service",
		models.AuthorityTaxi:     "driver-taxi-service",
		models.AuthorityHauling:  "driver-hauling-service",
	}
	for authority, service := range expected {
		if got := authority.OwningService(); got != service {
			t.Fatalf("%s owning service = %q, want %q", authority, got, service)
		}
	}
	if models.Authority("FLEET").OwningService() != "" {
		t.Fatal("an unknown authority must own no service")
	}
}

// TestMemberAssignabilityIsAnOrganizationGate documents what the member layer
// adds on top of the per-authority links: one switch for the whole person.
func TestMemberAssignabilityIsAnOrganizationGate(t *testing.T) {
	active := models.Member{Status: models.MemberActive, ConsentStatus: models.ConsentGranted}
	if !active.Assignable() {
		t.Fatal("an active consenting member should be assignable")
	}
	for _, member := range []models.Member{
		{Status: models.MemberSuspended, ConsentStatus: models.ConsentGranted},
		{Status: models.MemberActive, ConsentStatus: models.ConsentPending},
		{Status: models.MemberActive, ConsentStatus: models.ConsentWithdrawn},
		{Status: models.MemberEnded, ConsentStatus: models.ConsentGranted},
	} {
		if member.Assignable() {
			t.Fatalf("%+v should not be assignable", member)
		}
	}
}

// TestDisabledFeatureRefusesWrites keeps the rollout flag meaningful.
func TestDisabledFeatureRefusesWrites(t *testing.T) {
	service := usecases.NewService(usecases.Options{Repository: &repositories.Repository{}, Enabled: false})
	_, err := service.CreateMember(context.Background(),
		tenant.Context{OrganizationID: "org-1", ActorType: "USER", ActorID: "u1"}, "EMP-1", "A")
	if !errors.Is(err, models.ErrDisabled) {
		t.Fatalf("create with the flag off = %v, want ErrDisabled", err)
	}
}

// ── Integration: the invariants the database enforces ───────────────────────

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("FLEET_PARTNER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("FLEET_PARTNER_TEST_DATABASE_URL is required")
	}
	if !strings.Contains(strings.ToLower(dsn), "test") {
		t.Fatal("refusing a database whose name does not identify it as a test database")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// seedOrganizations creates the tenants a test needs, clearing anything a
// previous run left behind.
//
// The cleanup is not tidiness. Memberships carry a platform-wide unique index
// on the live (service, provider) pair, so a provider id left over from an
// earlier run makes the next run's first link look like a duplicate -- a
// failure that says "conflict" and points at nothing.
func seedOrganizations(t *testing.T, pool *pgxpool.Pool, ids ...string) {
	t.Helper()
	ctx := context.Background()
	for _, id := range ids {
		for _, statement := range []string{
			`DELETE FROM fleet_partner_managed_asset_links WHERE organization_id=$1`,
			`DELETE FROM fleet_partner_organization_provider_memberships WHERE organization_id=$1`,
			`DELETE FROM fleet_partner_managed_provider_members WHERE organization_id=$1`,
		} {
			if _, err := pool.Exec(ctx, statement, id); err != nil {
				t.Fatalf("clearing %s: %v", id, err)
			}
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO fleet_partner_organizations(
				id, legal_name, registration_number, country_code, timezone, default_currency)
			VALUES($1,$1,$1,'NG','Africa/Lagos','NGN')
			ON CONFLICT (id) DO NOTHING`, id); err != nil {
			t.Fatal(err)
		}
	}
}

// seedMembership inserts a membership row the way the existing provider
// feature does, so the member layer is tested against real rows rather than
// ones shaped to suit it.
func seedMembership(t *testing.T, pool *pgxpool.Pool, id, organizationID string,
	authority models.Authority, providerID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO fleet_partner_organization_provider_memberships(
			id, organization_id, provider_service, service_category, provider_id,
			onboarding_source, status)
		VALUES($1,$2,$3,$4,$5,'EXISTING_PROVIDER_LINKED','ACTIVE')`,
		id, organizationID, authority.OwningService(), authority, providerID); err != nil {
		t.Fatal(err)
	}
}

type stubAuthority struct {
	resolutions map[string]models.ProviderResolution
	err         error
}

func (s stubAuthority) ResolveProvider(_ context.Context, authority models.Authority,
	providerRef string) (models.ProviderResolution, error) {
	if s.err != nil {
		return models.ProviderResolution{}, s.err
	}
	resolution, ok := s.resolutions[string(authority)+":"+providerRef]
	if !ok {
		return models.ProviderResolution{}, models.ErrAuthorityRefused
	}
	return resolution, nil
}

// bindRepository binds the Postgres repository to a transaction, as the
// composition root does.
func bindRepository(q database.Querier) usecases.Repository {
	return repositories.NewRepository(nil).With(q)
}

func newTestService(pool *pgxpool.Pool, authority usecases.ProviderAuthority) *usecases.Service {
	return usecases.NewService(usecases.Options{
		Repository: repositories.NewRepository(pool), Bind: bindRepository, Pool: pool,
		Authority: authority, Enabled: true,
		Now: func() time.Time { return time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC) },
	})
}

func tenantOf(organizationID string) tenant.Context {
	return tenant.Context{OrganizationID: organizationID, OrganizationUserID: "usr-1",
		ActorType: "USER", ActorID: "usr-1"}
}

// TestFleetCannotMintAProviderIdentity is mandatory test 6, and the
// load-bearing authority rule of the whole section.
//
// Every provider id in this service arrives as the answer to a question asked
// of the service that issued it. There is no constructor, no generator and no
// fallback that produces one, so an authority answering without naming a
// provider is a refusal rather than a blank to fill in. Recording it would
// give the platform a corporate identity that no driver service has ever heard
// of, and the mistake would only surface as work dispatched to nobody.
//
// The member is read from the database before the authority is asked -- a
// tenant check costs nothing and must not be skipped for an external call --
// so this test needs a real member to exist.
func TestFleetCannotMintAProviderIdentity(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	seedOrganizations(t, pool, "org-mint-1")
	scope := tenantOf("org-mint-1")

	service := newTestService(pool, emptyAuthority{})
	member, err := service.CreateMember(ctx, scope, "EMP-MINT-1", "Unknown")
	if err != nil {
		t.Fatal(err)
	}
	seedMembership(t, pool, "mb-mint-1", "org-mint-1", models.AuthorityTaxi, "prv-mint-t")

	_, err = service.LinkServiceIdentity(ctx, scope, usecases.LinkRequest{
		MemberID: member.ID, Authority: models.AuthorityTaxi,
		ProviderRef: "whoever", MembershipID: "mb-mint-1"})
	if !errors.Is(err, models.ErrAuthorityRefused) {
		t.Fatalf("an unnamed provider was accepted: %v", err)
	}

	// Nothing was recorded.
	view, err := service.ReadMember(ctx, scope, member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Links) != 0 {
		t.Fatalf("links = %d, want none: a refused resolution must leave no link", len(view.Links))
	}
}

// TestOneMemberHoldsThreeAuthorities is mandatory test 1, and the reason the
// member layer exists.
func TestOneMemberHoldsThreeAuthorities(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	seedOrganizations(t, pool, "org-e2e-1")

	authority := stubAuthority{resolutions: map[string]models.ProviderResolution{
		"DISPATCH:ref-d": {ProviderID: "prv-d", QualificationStatus: "ACTIVE", SourceVersion: 4},
		"TAXI:ref-t":     {ProviderID: "prv-t", QualificationStatus: "ACTIVE", SourceVersion: 7},
		"HAULING:ref-h":  {ProviderID: "prv-h", QualificationStatus: "ACTIVE", SourceVersion: 2},
	}}
	service := newTestService(pool, authority)
	scope := tenantOf("org-e2e-1")

	member, err := service.CreateMember(ctx, scope, "EMP-E2E-1", "A. Driver")
	if err != nil {
		t.Fatal(err)
	}

	for _, link := range []struct {
		authority  models.Authority
		membership string
		ref        string
	}{
		{models.AuthorityDispatch, "mb-e2e-d", "ref-d"},
		{models.AuthorityTaxi, "mb-e2e-t", "ref-t"},
		{models.AuthorityHauling, "mb-e2e-h", "ref-h"},
	} {
		seedMembership(t, pool, link.membership, "org-e2e-1", link.authority,
			authority.resolutions[string(link.authority)+":"+link.ref].ProviderID)
		if _, err = service.LinkServiceIdentity(ctx, scope, usecases.LinkRequest{
			MemberID: member.ID, Authority: link.authority,
			ProviderRef: link.ref, MembershipID: link.membership,
		}); err != nil {
			t.Fatalf("link %s: %v", link.authority, err)
		}
	}

	view, err := service.ReadMember(ctx, scope, member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Links) != 3 {
		t.Fatalf("links = %d, want 3: one member must be able to hold all three authorities",
			len(view.Links))
	}
	for _, expected := range models.Authorities() {
		link, ok := view.AuthorityLink(expected)
		if !ok {
			t.Fatalf("no live %s link on the member", expected)
		}
		if link.ProviderID == "" {
			t.Fatalf("%s link carries no provider id", expected)
		}
	}
}

// TestDuplicateProviderLinkIsRefused is mandatory test 2.
func TestDuplicateProviderLinkIsRefused(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	seedOrganizations(t, pool, "org-dup-1")

	authority := stubAuthority{resolutions: map[string]models.ProviderResolution{
		"TAXI:ref-t": {ProviderID: "prv-dup-t", QualificationStatus: "ACTIVE"},
	}}
	service := newTestService(pool, authority)
	scope := tenantOf("org-dup-1")

	first, err := service.CreateMember(ctx, scope, "EMP-DUP-1", "First")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateMember(ctx, scope, "EMP-DUP-2", "Second")
	if err != nil {
		t.Fatal(err)
	}

	seedMembership(t, pool, "mb-dup-1", "org-dup-1", models.AuthorityTaxi, "prv-dup-t")
	if _, err = service.LinkServiceIdentity(ctx, scope, usecases.LinkRequest{
		MemberID: first.ID, Authority: models.AuthorityTaxi,
		ProviderRef: "ref-t", MembershipID: "mb-dup-1"}); err != nil {
		t.Fatal(err)
	}

	// Two defences, and both are worth pinning because they fail differently.
	//
	// The first is the platform's own: a second LIVE membership for the same
	// taxi provider is refused by the database, so one driver can never hold
	// two corporate identities and two settlement slots. This predates the
	// member layer and must keep holding.
	if _, err = pool.Exec(ctx, `
		INSERT INTO fleet_partner_organization_provider_memberships(
			id, organization_id, provider_service, service_category, provider_id,
			onboarding_source, status)
		VALUES('mb-dup-2','org-dup-1','driver-taxi-service','TAXI','prv-dup-t',
		       'EXISTING_PROVIDER_LINKED','ACTIVE')`); err == nil {
		t.Fatal("a second live membership for one taxi provider was accepted")
	}

	// The second is this feature's: a membership may only be attached to a
	// member when it already carries the provider the authority resolved.
	// Without it a caller could resolve one provider and attach a membership
	// for another, recording a corporate identity nobody verified.
	seedMembership(t, pool, "mb-dup-3", "org-dup-1", models.AuthorityTaxi, "prv-dup-other")
	_, err = service.LinkServiceIdentity(ctx, scope, usecases.LinkRequest{
		MemberID: second.ID, Authority: models.AuthorityTaxi,
		ProviderRef: "ref-t", MembershipID: "mb-dup-3"})
	if !errors.Is(err, models.ErrValidation) {
		t.Fatalf("attaching a membership for a different provider = %v, want a refusal", err)
	}

	// And a membership already attached to someone else is a conflict.
	//
	// The original membership is retired first: while it is live the platform
	// index above will not let a second row carry the same provider, which is
	// exactly the guarantee just asserted.
	if _, err = pool.Exec(ctx, `
		UPDATE fleet_partner_organization_provider_memberships
		   SET status='REMOVED' WHERE id='mb-dup-1'`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `
		UPDATE fleet_partner_organization_provider_memberships
		   SET provider_id='prv-dup-t' WHERE id='mb-dup-3'`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `
		UPDATE fleet_partner_organization_provider_memberships
		   SET member_id=$1 WHERE id='mb-dup-3'`, first.ID); err != nil {
		t.Fatal(err)
	}
	_, err = service.LinkServiceIdentity(ctx, scope, usecases.LinkRequest{
		MemberID: second.ID, Authority: models.AuthorityTaxi,
		ProviderRef: "ref-t", MembershipID: "mb-dup-3"})
	if !errors.Is(err, models.ErrConflict) {
		t.Fatalf("claiming another member's membership = %v, want ErrConflict", err)
	}

	// A retry of the ORIGINAL link is not a duplicate: it converges.
	replay, err := service.LinkServiceIdentity(ctx, scope, usecases.LinkRequest{
		MemberID: first.ID, Authority: models.AuthorityTaxi,
		ProviderRef: "ref-t", MembershipID: "mb-dup-1"})
	if err != nil {
		t.Fatalf("replaying the original link = %v, want it to converge", err)
	}
	if replay.ProviderID != "prv-dup-t" {
		t.Fatalf("replayed link provider = %q", replay.ProviderID)
	}
}

// TestCrossTenantMemberAccessIsImpossible is mandatory test 3.
func TestCrossTenantMemberAccessIsImpossible(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	seedOrganizations(t, pool, "org-iso-1", "org-iso-2")
	service := newTestService(pool, stubAuthority{})

	member, err := service.CreateMember(ctx, tenantOf("org-iso-1"), "EMP-ISO-1", "Theirs")
	if err != nil {
		t.Fatal(err)
	}

	// The rival organization knows the id and still cannot read it.
	if _, err = service.ReadMember(ctx, tenantOf("org-iso-2"), member.ID); !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("cross-tenant read = %v, want ErrNotFound", err)
	}
	// Nor suspend it.
	if _, err = service.SuspendMember(ctx, tenantOf("org-iso-2"), member.ID, 1); !errors.Is(err, models.ErrVersion) {
		t.Fatalf("cross-tenant suspend = %v, want a refusal", err)
	}
	// And it does not appear in their list.
	members, err := service.ListMembers(ctx, tenantOf("org-iso-2"), 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, listed := range members {
		if listed.ID == member.ID {
			t.Fatal("another organization's member appeared in this organization's list")
		}
	}
}

// TestSuspendingOneAuthorityLeavesTheOthersAlone is mandatory test 4.
//
// The failure it guards is quiet: a driver whose taxi qualification lapses
// stops being dispatched as a rider too, and nobody connects the two events.
func TestSuspendingOneAuthorityLeavesTheOthersAlone(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	seedOrganizations(t, pool, "org-susp-1")

	authority := stubAuthority{resolutions: map[string]models.ProviderResolution{
		"DISPATCH:d": {ProviderID: "prv-susp-d"},
		"TAXI:t":     {ProviderID: "prv-susp-t"},
		"HAULING:h":  {ProviderID: "prv-susp-h"},
	}}
	service := newTestService(pool, authority)
	scope := tenantOf("org-susp-1")
	member, err := service.CreateMember(ctx, scope, "EMP-SUSP-1", "Multi")
	if err != nil {
		t.Fatal(err)
	}
	for _, link := range []struct {
		authority  models.Authority
		membership string
		ref        string
		provider   string
	}{
		{models.AuthorityDispatch, "mb-susp-d", "d", "prv-susp-d"},
		{models.AuthorityTaxi, "mb-susp-t", "t", "prv-susp-t"},
		{models.AuthorityHauling, "mb-susp-h", "h", "prv-susp-h"},
	} {
		seedMembership(t, pool, link.membership, "org-susp-1", link.authority, link.provider)
		if _, err = service.LinkServiceIdentity(ctx, scope, usecases.LinkRequest{
			MemberID: member.ID, Authority: link.authority,
			ProviderRef: link.ref, MembershipID: link.membership}); err != nil {
			t.Fatal(err)
		}
	}

	// Suspend the TAXI link the way the owning authority's own status change
	// would: on the membership row, not on the member.
	if _, err = pool.Exec(ctx, `
		UPDATE fleet_partner_organization_provider_memberships
		   SET status='SUSPENDED', suspended_at=now() WHERE id='mb-susp-t'`); err != nil {
		t.Fatal(err)
	}

	view, err := service.ReadMember(ctx, scope, member.ID)
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[models.Authority]string{}
	for _, link := range view.Links {
		statuses[link.Authority] = link.LinkStatus
	}
	if statuses[models.AuthorityTaxi] != "SUSPENDED" {
		t.Fatalf("taxi link = %q, want SUSPENDED", statuses[models.AuthorityTaxi])
	}
	if statuses[models.AuthorityDispatch] != "ACTIVE" || statuses[models.AuthorityHauling] != "ACTIVE" {
		t.Fatalf("suspending taxi disturbed the others: dispatch=%q hauling=%q",
			statuses[models.AuthorityDispatch], statuses[models.AuthorityHauling])
	}

	// Assignability follows each authority independently.
	for _, expected := range []struct {
		authority models.Authority
		want      bool
	}{{models.AuthorityDispatch, true}, {models.AuthorityHauling, true}, {models.AuthorityTaxi, false}} {
		// Consent must be granted before the member gate opens at all.
		if _, err = service.GrantConsent(ctx, scope, member.ID, view.Member.Version); err != nil &&
			!errors.Is(err, models.ErrVersion) {
			t.Fatal(err)
		}
		assignable, reason, checkErr := service.AssignableForAuthority(ctx, scope, member.ID, expected.authority)
		if checkErr != nil {
			t.Fatal(checkErr)
		}
		if assignable != expected.want {
			t.Fatalf("%s assignable = %t (%s), want %t", expected.authority, assignable, reason, expected.want)
		}
	}
}

// TestOrganizationSuspensionBlocksEveryAuthorityWithoutDeletingLinks is
// mandatory test 5.
//
// The second half matters as much as the first: a reinstated member must come
// back with the same three identities, not with three links an operator has to
// rebuild by hand.
func TestOrganizationSuspensionBlocksEveryAuthorityWithoutDeletingLinks(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	seedOrganizations(t, pool, "org-global-1")

	authority := stubAuthority{resolutions: map[string]models.ProviderResolution{
		"DISPATCH:d": {ProviderID: "prv-glob-d"},
		"TAXI:t":     {ProviderID: "prv-glob-t"},
	}}
	service := newTestService(pool, authority)
	scope := tenantOf("org-global-1")
	member, err := service.CreateMember(ctx, scope, "EMP-GLOB-1", "Both")
	if err != nil {
		t.Fatal(err)
	}
	seedMembership(t, pool, "mb-glob-d", "org-global-1", models.AuthorityDispatch, "prv-glob-d")
	seedMembership(t, pool, "mb-glob-t", "org-global-1", models.AuthorityTaxi, "prv-glob-t")
	for _, link := range []struct {
		authority  models.Authority
		membership string
		ref        string
	}{{models.AuthorityDispatch, "mb-glob-d", "d"}, {models.AuthorityTaxi, "mb-glob-t", "t"}} {
		if _, err = service.LinkServiceIdentity(ctx, scope, usecases.LinkRequest{
			MemberID: member.ID, Authority: link.authority,
			ProviderRef: link.ref, MembershipID: link.membership}); err != nil {
			t.Fatal(err)
		}
	}

	granted, err := service.GrantConsent(ctx, scope, member.ID, member.Version)
	if err != nil {
		t.Fatal(err)
	}
	suspended, err := service.SuspendMember(ctx, scope, member.ID, granted.Version)
	if err != nil {
		t.Fatal(err)
	}

	for _, expected := range []models.Authority{models.AuthorityDispatch, models.AuthorityTaxi} {
		assignable, reason, checkErr := service.AssignableForAuthority(ctx, scope, member.ID, expected)
		if checkErr != nil {
			t.Fatal(checkErr)
		}
		if assignable {
			t.Fatalf("%s still assignable after an organization-wide suspension", expected)
		}
		if reason != "MEMBER_NOT_ASSIGNABLE" {
			t.Fatalf("%s refusal reason = %q, want the member gate", expected, reason)
		}
	}

	view, err := service.ReadMember(ctx, scope, member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Links) != 2 {
		t.Fatalf("links after suspension = %d, want both preserved: a suspension is not a deletion",
			len(view.Links))
	}

	// Reinstatement restores assignment without re-resolving anything.
	if _, err = service.ReinstateMember(ctx, scope, member.ID, suspended.Version); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []models.Authority{models.AuthorityDispatch, models.AuthorityTaxi} {
		assignable, reason, checkErr := service.AssignableForAuthority(ctx, scope, member.ID, expected)
		if checkErr != nil {
			t.Fatal(checkErr)
		}
		if !assignable {
			t.Fatalf("%s not assignable after reinstatement (%s)", expected, reason)
		}
	}
}

// TestStaleVersionCannotOverwriteANewerDecision guards the concurrency rule.
func TestStaleVersionCannotOverwriteANewerDecision(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	seedOrganizations(t, pool, "org-ver-1")
	service := newTestService(pool, stubAuthority{})
	scope := tenantOf("org-ver-1")

	member, err := service.CreateMember(ctx, scope, "EMP-VER-1", "Versioned")
	if err != nil {
		t.Fatal(err)
	}
	staleVersion := member.Version
	if _, err = service.SuspendMember(ctx, scope, member.ID, staleVersion); err != nil {
		t.Fatal(err)
	}
	// A second operator acting on the version they read before the suspension
	// must lose, or a suspension could be silently undone by a reinstatement
	// decided without knowledge of it.
	if _, err = service.ReinstateMember(ctx, scope, member.ID, staleVersion); !errors.Is(err, models.ErrVersion) {
		t.Fatalf("stale reinstatement = %v, want ErrVersion", err)
	}
}

// TestAssetLinksCarryNoFitnessClaim pins the ownership/fitness boundary.
//
// An organization saying "this truck is ours" is not the organization saying
// the truck is roadworthy, refrigerated or certified. Those answers belong to
// the owning service, and a cached copy here would be read after it went stale.
func TestAssetLinksCarryNoFitnessClaim(t *testing.T) {
	pool := newTestPool(t)
	ctx := context.Background()
	seedOrganizations(t, pool, "org-asset-1")
	service := newTestService(pool, stubAuthority{})
	scope := tenantOf("org-asset-1")

	member, err := service.CreateMember(ctx, scope, "EMP-ASSET-1", "Owner")
	if err != nil {
		t.Fatal(err)
	}
	link, err := service.LinkAsset(ctx, scope, usecases.LinkAssetRequest{
		Authority: models.AuthorityHauling, AssetID: "truck-1",
		MemberID: member.ID, SourceVersion: 3})
	if err != nil {
		t.Fatal(err)
	}
	if link.AssetID != "truck-1" || link.LinkStatus != models.AssetLinkActive {
		t.Fatalf("asset link = %+v", link)
	}

	// A stale report must not overwrite a newer one.
	if _, err = service.LinkAsset(ctx, scope, usecases.LinkAssetRequest{
		Authority: models.AuthorityHauling, AssetID: "truck-1",
		MemberID: member.ID, SourceVersion: 1}); !errors.Is(err, models.ErrVersion) {
		t.Fatalf("stale asset report = %v, want ErrVersion", err)
	}

	// Another organization cannot see it.
	seedOrganizations(t, pool, "org-asset-2")
	assets, err := service.ListAssets(ctx, tenantOf("org-asset-2"), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, listed := range assets {
		if listed.AssetID == "truck-1" {
			t.Fatal("another organization's asset link leaked")
		}
	}
}
