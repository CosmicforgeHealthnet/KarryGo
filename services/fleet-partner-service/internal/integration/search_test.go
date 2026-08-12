package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	searchrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/search/repositories"
	searchusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/search/usecases"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ---------------------------------------------------------------------------
// Fixture
// ---------------------------------------------------------------------------

type searchFixture struct {
	pool    *pgxpool.Pool
	service *searchusecases.Service
	orgID   string
	otherID string
}

func newSearchFixture(t *testing.T) *searchFixture {
	t.Helper()
	pool := testPool(t)
	primary := createOrganization(t, pool, "Search Primary Fleet", "RC-SRCH-1")
	secondary := createOrganization(t, pool, "Search Secondary Fleet", "RC-SRCH-2")

	roles := staffrepos.NewRoleRepository(pool)
	authorizer := accesscontrol.NewAuthorizer(roles, nil, 0)
	service := searchusecases.NewService(searchusecases.Options{
		Pool: pool, Repository: searchrepos.NewRepository(pool), Authorizer: authorizer,
		Metrics: observability.NewMetrics(), Flags: searchusecases.Flags{Enabled: true},
	})

	return &searchFixture{pool: pool, service: service, orgID: primary.ID, otherID: secondary.ID}
}

func (f *searchFixture) tenantWithRole(t *testing.T, label, roleID string) tenant.Context {
	t.Helper()
	memberID := seedApprovalMember(t, f.pool, f.orgID, roleID, label)
	return tenant.Context{OrganizationID: f.orgID, OrganizationUserID: memberID,
		ActorType: tenant.ActorTypeOrganizationUser, ActorID: memberID}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestSearch_PermissionFiltering(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()

	if _, err := f.service.Index(ctx, f.orgID, searchusecases.IndexInput{
		SubjectType: "ORDER", SubjectReference: "ord_visible", Title: "Visible order ORD-VISIBLE",
		RequiredPermission: "search.read", SourceVersion: 1,
	}); err != nil {
		t.Fatalf("index visible-permission document: %v", err)
	}
	if _, err := f.service.Index(ctx, f.orgID, searchusecases.IndexInput{
		SubjectType: "ORDER", SubjectReference: "ord_restricted", Title: "Restricted order ORD-RESTRICTED",
		RequiredPermission: "orders.manage", SourceVersion: 1,
	}); err != nil {
		t.Fatalf("index restricted-permission document: %v", err)
	}

	owner := f.tenantWithRole(t, "owner", "rol_system_owner")
	ownerResults, err := f.service.Search(ctx, owner, searchusecases.SearchInput{Text: "order"})
	if err != nil {
		t.Fatalf("owner search: %v", err)
	}
	if len(ownerResults) != 2 {
		t.Fatalf("expected the owner to see both documents, got %d", len(ownerResults))
	}

	noRole := f.tenantWithRole(t, "norole", "")
	noRoleResults, err := f.service.Search(ctx, noRole, searchusecases.SearchInput{Text: "order"})
	if err != nil {
		t.Fatalf("no-role search: %v", err)
	}
	if len(noRoleResults) != 0 {
		t.Fatalf("expected a member with no role bindings to see zero documents, got %d: %+v",
			len(noRoleResults), noRoleResults)
	}
}

func TestSearch_TenantIsolation(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()

	if _, err := f.service.Index(ctx, f.orgID, searchusecases.IndexInput{
		SubjectType: "ORDER", SubjectReference: "ord_tenant_a", Title: "Tenant A order",
		RequiredPermission: "search.read", SourceVersion: 1,
	}); err != nil {
		t.Fatalf("index for primary tenant: %v", err)
	}

	owner := f.tenantWithRole(t, "owner", "rol_system_owner")
	results, err := f.service.Search(ctx, owner, searchusecases.SearchInput{Text: "order"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected exactly one result for the owning tenant, got %d", len(results))
	}

	// A second organization's member, even with full permissions in their own
	// organization, must not see the first organization's document — the
	// query is scoped by organization_id in SQL, not filtered afterward.
	otherOwnerMemberID := seedApprovalMember(t, f.pool, f.otherID, "rol_system_owner", "other-owner")
	otherOwner := tenant.Context{OrganizationID: f.otherID, OrganizationUserID: otherOwnerMemberID,
		ActorType: tenant.ActorTypeOrganizationUser, ActorID: otherOwnerMemberID}
	otherResults, err := f.service.Search(ctx, otherOwner, searchusecases.SearchInput{Text: "order"})
	if err != nil {
		t.Fatalf("search from other tenant: %v", err)
	}
	if len(otherResults) != 0 {
		t.Fatalf("expected zero results from a different organization, got %d", len(otherResults))
	}
}

func TestSearch_StaleReindexIsIgnored(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()

	if _, err := f.service.Index(ctx, f.orgID, searchusecases.IndexInput{
		SubjectType: "SHIPMENT", SubjectReference: "shp_1", Title: "Current title", RequiredPermission: "search.read",
		SourceVersion: 5,
	}); err != nil {
		t.Fatalf("index v5: %v", err)
	}
	stale, err := f.service.Index(ctx, f.orgID, searchusecases.IndexInput{
		SubjectType: "SHIPMENT", SubjectReference: "shp_1", Title: "Stale title that must not apply",
		RequiredPermission: "search.read", SourceVersion: 3,
	})
	if err != nil {
		t.Fatalf("index v3 (stale, expected to be silently ignored): %v", err)
	}
	if stale.Title != "Current title" {
		t.Fatalf("a lower source_version must not overwrite the stored projection, got title %q", stale.Title)
	}
	if stale.SourceVersion != 5 {
		t.Fatalf("expected the stored source_version to remain 5, got %d", stale.SourceVersion)
	}
}

func TestSearch_RemoveFromIndex(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()
	owner := f.tenantWithRole(t, "owner", "rol_system_owner")

	if _, err := f.service.Index(ctx, f.orgID, searchusecases.IndexInput{
		SubjectType: "MANIFEST", SubjectReference: "mnf_1", Title: "Manifest to remove", RequiredPermission: "search.read",
		SourceVersion: 1,
	}); err != nil {
		t.Fatalf("index: %v", err)
	}
	before, err := f.service.Search(ctx, owner, searchusecases.SearchInput{Text: "manifest"})
	if err != nil || len(before) != 1 {
		t.Fatalf("expected one result before removal, got %d (%v)", len(before), err)
	}

	if err := f.service.RemoveFromIndex(ctx, f.orgID, "MANIFEST", "mnf_1"); err != nil {
		t.Fatalf("remove from index: %v", err)
	}
	after, err := f.service.Search(ctx, owner, searchusecases.SearchInput{Text: "manifest"})
	if err != nil {
		t.Fatalf("search after removal: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("expected zero results after removal, got %d", len(after))
	}
}

func TestSearch_FullTextRelevance(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()
	owner := f.tenantWithRole(t, "owner", "rol_system_owner")

	if _, err := f.service.Index(ctx, f.orgID, searchusecases.IndexInput{
		SubjectType: "PACKAGE", SubjectReference: "pkg_apples", Title: "Crate of apples",
		SearchableText: "fresh red apples from the farm", RequiredPermission: "search.read", SourceVersion: 1,
	}); err != nil {
		t.Fatalf("index apples: %v", err)
	}
	if _, err := f.service.Index(ctx, f.orgID, searchusecases.IndexInput{
		SubjectType: "PACKAGE", SubjectReference: "pkg_bananas", Title: "Crate of bananas",
		SearchableText: "ripe yellow bananas", RequiredPermission: "search.read", SourceVersion: 1,
	}); err != nil {
		t.Fatalf("index bananas: %v", err)
	}

	results, err := f.service.Search(ctx, owner, searchusecases.SearchInput{Text: "apples"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 || results[0].SubjectReference != "pkg_apples" {
		t.Fatalf("expected exactly the apples package, got %+v", results)
	}
}

func TestSection25SearchIndexDeadLetterReplayAndImmutableAttempts(t *testing.T) {
	f := newSearchFixture(t)
	ctx := context.Background()
	const jobID = "sij_test_dead_letter"
	if _, err := f.pool.Exec(ctx, `INSERT INTO fleet_partner_search_index_jobs(id,organization_id,subject_type,subject_reference,source_version,operation) VALUES($1,$2,'UNSUPPORTED','subject_1',1,'UPSERT')`, jobID, f.orgID); err != nil {
		t.Fatal(err)
	}
	processed, err := f.service.RunIndexingPass(ctx, searchusecases.IndexWorkerConfig{Owner: "section25-test", BatchSize: 1, MaxAttempts: 1, Lease: time.Second})
	if err != nil || processed != 1 {
		t.Fatalf("index pass processed=%d err=%v", processed, err)
	}
	var status string
	if err = f.pool.QueryRow(ctx, `SELECT status FROM fleet_partner_search_index_jobs WHERE id=$1`, jobID).Scan(&status); err != nil || status != "DEAD_LETTER" {
		t.Fatalf("job status=%s err=%v", status, err)
	}
	if _, err = f.pool.Exec(ctx, `UPDATE fleet_partner_search_index_attempts SET outcome='TAMPERED' WHERE job_id=$1`, jobID); err == nil {
		t.Fatal("search attempt history accepted an update")
	}
	if err = f.service.ReplayIndexJob(ctx, f.orgID, jobID); err != nil {
		t.Fatal(err)
	}
	if err = f.pool.QueryRow(ctx, `SELECT status FROM fleet_partner_search_index_jobs WHERE id=$1`, jobID).Scan(&status); err != nil || status != "PENDING" {
		t.Fatalf("replayed job status=%s err=%v", status, err)
	}
}
