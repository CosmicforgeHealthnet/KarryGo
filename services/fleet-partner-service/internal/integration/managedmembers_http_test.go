package integration_test

// Spec 6j/6k/6l for the managed-member surface (FLEET-10.15): every write
// commits with an audit record and an outbox event, the routes the table
// marks IdempotencyRequired really require and honour an Idempotency-Key, and
// the expected version is accepted as expectedVersion (the API's spelling),
// expected_version (the legacy spelling) or If-Match.

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/managedmembers"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

type memberAuthority struct{ providers map[string]string }

func (a memberAuthority) ResolveProvider(_ context.Context, authority managedmembers.Authority, ref string) (managedmembers.ProviderResolution, error) {
	id, ok := a.providers[string(authority)+":"+ref]
	if !ok {
		return managedmembers.ProviderResolution{}, managedmembers.ErrAuthorityRefused
	}
	return managedmembers.ProviderResolution{ProviderID: id, QualificationStatus: "QUALIFIED"}, nil
}

type memberFixture struct {
	pool      *pgxpool.Pool
	orgID     string
	member    string
	authority memberAuthority
}

func newMemberFixture(t *testing.T) *memberFixture {
	t.Helper()
	return newMemberFixtureOn(t, testPool(t))
}

// newMemberFixtureOn adds a second organization on an already-held pool
// (testPool serializes on a non-reentrant lock).
func newMemberFixtureOn(t *testing.T, pool *pgxpool.Pool) *memberFixture {
	t.Helper()
	org := createOrganization(t, pool, "Managed Members "+ids.New("x"), ids.New("rc"))
	return &memberFixture{
		pool: pool, orgID: org.ID,
		member:    seedApprovalMember(t, pool, org.ID, "rol_system_operations", "mmops"),
		authority: memberAuthority{providers: map[string]string{}},
	}
}

func (f *memberFixture) router(t *testing.T, organizationID, memberID string) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	service := managedmembers.NewService(managedmembers.Options{
		Repository: managedmembers.NewRepository(f.pool), Authority: f.authority,
		Enabled: true, Pool: f.pool,
	})
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(middleware.TenantContextKey, tenant.Context{
			OrganizationID: organizationID, OrganizationUserID: memberID,
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: memberID,
			CorrelationID: "corr-mm"})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	managedmembers.NewHandler(service, accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(f.pool), nil, 0),
		idempotency.NewStore(f.pool, 0)).RegisterTenantRoutes(engine.Group("/fleet-partner/v1"))
	return engine
}

func (f *memberFixture) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func (f *memberFixture) audits(t *testing.T, action, resourceID string) int {
	return f.count(t, `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id=$1 AND action=$2 AND resource_id=$3`, f.orgID, action, resourceID)
}

func (f *memberFixture) outbox(t *testing.T, eventType, aggregateID string) int {
	return f.count(t, `SELECT count(*) FROM fleet_partner_outbox WHERE organization_id=$1 AND event_type=$2 AND aggregate_id=$3`, f.orgID, eventType, aggregateID)
}

func memberData(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("no data in %v", body)
	}
	return data
}

func versionOf(data map[string]any) int64 {
	switch v := data["version"].(type) {
	case float64:
		return int64(v)
	}
	if v, ok := data["Version"].(float64); ok {
		return int64(v)
	}
	return 0
}

func idOf(data map[string]any) string {
	if v, ok := data["id"].(string); ok {
		return v
	}
	v, _ := data["ID"].(string)
	return v
}

func TestManagedMemberCreateRequiresAndHonoursIdempotencyKey(t *testing.T) {
	f := newMemberFixture(t)
	router := f.router(t, f.orgID, f.member)
	body := map[string]any{"member_ref": "EMP-" + ids.New("x"), "display_name": "Ada"}

	if status, _ := gateCall(t, router, http.MethodPost, "/fleet-partner/v1/managed-members", body, nil); status != http.StatusBadRequest {
		t.Fatalf("create without Idempotency-Key: status %d, want 400", status)
	}
	key := map[string]string{"Idempotency-Key": "mm-create-" + ids.New("k")}
	status, first := gateCall(t, router, http.MethodPost, "/fleet-partner/v1/managed-members", body, key)
	if status != http.StatusOK {
		t.Fatalf("create: status %d body %v", status, first)
	}
	memberID := idOf(memberData(t, first))
	if memberID == "" {
		t.Fatalf("no member id in %v", first)
	}
	status, replay := gateCall(t, router, http.MethodPost, "/fleet-partner/v1/managed-members", body, key)
	if status != http.StatusOK || idOf(memberData(t, replay)) != memberID {
		t.Fatalf("replay: status %d body %v, want the original member %s", status, replay, memberID)
	}
	if n := f.count(t, `SELECT count(*) FROM fleet_partner_managed_provider_members WHERE organization_id=$1`, f.orgID); n != 1 {
		t.Fatalf("members = %d after a replay, want 1", n)
	}
	other := map[string]any{"member_ref": "EMP-OTHER", "display_name": "Ada"}
	if status, _ := gateCall(t, router, http.MethodPost, "/fleet-partner/v1/managed-members", other, key); status != http.StatusConflict {
		t.Fatalf("same key, different body: status %d, want 409", status)
	}
	if f.audits(t, "managed_member.created", memberID) != 1 || f.outbox(t, "managed_member.created.v1", memberID) != 1 {
		t.Fatal("a created member must have exactly one audit record and one outbox event")
	}
}

func TestManagedMemberStatusAcceptsEveryVersionSpelling(t *testing.T) {
	f := newMemberFixture(t)
	router := f.router(t, f.orgID, f.member)
	_, created := gateCall(t, router, http.MethodPost, "/fleet-partner/v1/managed-members",
		map[string]any{"member_ref": "EMP-" + ids.New("x")}, map[string]string{"Idempotency-Key": "mm-" + ids.New("k")})
	member := memberData(t, created)
	memberID, version := idOf(member), versionOf(member)
	base := "/fleet-partner/v1/managed-members/" + memberID

	// Missing and conflicting versions are refused before anything moves.
	if status, _ := gateCall(t, router, http.MethodPost, base+"/consent", map[string]any{}, nil); status != http.StatusBadRequest {
		t.Fatalf("no version: status %d, want 400", status)
	}
	if status, _ := gateCall(t, router, http.MethodPost, base+"/consent",
		map[string]any{"expectedVersion": version, "expected_version": version + 1}, nil); status != http.StatusBadRequest {
		t.Fatalf("conflicting versions: status %d, want 400", status)
	}

	// camelCase (the API's spelling).
	status, body := gateCall(t, router, http.MethodPost, base+"/consent", map[string]any{"expectedVersion": version}, nil)
	if status != http.StatusOK {
		t.Fatalf("consent with expectedVersion: %d %v", status, body)
	}
	version = versionOf(memberData(t, body))
	// Legacy snake_case.
	status, body = gateCall(t, router, http.MethodPost, base+"/suspend", map[string]any{"expected_version": version}, nil)
	if status != http.StatusOK || memberData(t, body)["status"] != "SUSPENDED" {
		t.Fatalf("suspend with expected_version: %d %v", status, body)
	}
	stale := version
	version = versionOf(memberData(t, body))
	// If-Match, no body.
	status, body = gateCall(t, router, http.MethodPost, base+"/reinstate", nil, map[string]string{"If-Match": fmt.Sprintf(`"%d"`, version)})
	if status != http.StatusOK || memberData(t, body)["status"] != "ACTIVE" {
		t.Fatalf("reinstate with If-Match: %d %v", status, body)
	}
	// A stale version loses.
	if status, _ := gateCall(t, router, http.MethodPost, base+"/suspend", map[string]any{"expectedVersion": stale}, nil); status != http.StatusConflict {
		t.Fatalf("stale version: status %d, want 409", status)
	}

	for action, want := range map[string]int{
		"managed_member.consent_granted": 1, "managed_member.suspended": 1, "managed_member.reinstated": 1,
	} {
		if got := f.audits(t, action, memberID); got != want {
			t.Fatalf("audit %s = %d, want %d", action, got, want)
		}
		if got := f.outbox(t, action+".v1", memberID); got != want {
			t.Fatalf("outbox %s.v1 = %d, want %d", action, got, want)
		}
	}
	// The suspension's audit record carries the standing it replaced.
	var previous string
	if err := f.pool.QueryRow(context.Background(), `SELECT previous_summary::text FROM fleet_partner_audit_records WHERE organization_id=$1 AND action='managed_member.suspended' AND resource_id=$2`, f.orgID, memberID).Scan(&previous); err != nil {
		t.Fatal(err)
	}
	if previous == "" || previous == "{}" || previous == "null" {
		t.Fatalf("suspension audit has no previous summary: %q", previous)
	}
	// The refused stale write left no trace.
	if f.audits(t, "managed_member.suspended", memberID) != 1 {
		t.Fatal("a refused write must not be audited as a change")
	}
}

func TestManagedMemberLinkAndAssetWritesAreAuditedAndIdempotent(t *testing.T) {
	f := newMemberFixture(t)
	providerID := ids.New("prov")
	membershipID := ids.New("mem")
	if _, err := f.pool.Exec(context.Background(), `
		INSERT INTO fleet_partner_organization_provider_memberships
			(id, organization_id, provider_service, service_category, provider_id, onboarding_source, status)
		VALUES ($1,$2,'driver-taxi-service','TAXI',$3,'EXISTING_PROVIDER_LINKED','ACTIVE')`,
		membershipID, f.orgID, providerID); err != nil {
		t.Fatal(err)
	}
	f.authority.providers["TAXI:drv-ref"] = providerID
	router := f.router(t, f.orgID, f.member)
	_, created := gateCall(t, router, http.MethodPost, "/fleet-partner/v1/managed-members",
		map[string]any{"member_ref": "EMP-" + ids.New("x")}, map[string]string{"Idempotency-Key": "mm-" + ids.New("k")})
	memberID := idOf(memberData(t, created))

	link := map[string]any{"authority": "TAXI", "provider_ref": "drv-ref", "membership_id": membershipID}
	path := "/fleet-partner/v1/managed-members/" + memberID + "/links"
	if status, _ := gateCall(t, router, http.MethodPost, path, link, nil); status != http.StatusBadRequest {
		t.Fatalf("link without Idempotency-Key: status %d, want 400", status)
	}
	key := map[string]string{"Idempotency-Key": "mm-link-" + ids.New("k")}
	if status, body := gateCall(t, router, http.MethodPost, path, link, key); status != http.StatusOK {
		t.Fatalf("link: %d %v", status, body)
	}
	if status, _ := gateCall(t, router, http.MethodPost, path, link, key); status != http.StatusOK {
		t.Fatal("link replay must return the recorded response")
	}
	if f.audits(t, "managed_member.service_linked", memberID) != 1 || f.outbox(t, "managed_member.service_linked.v1", memberID) != 1 {
		t.Fatal("a service link must have exactly one audit record and one outbox event, replays included")
	}

	asset := map[string]any{"authority": "TAXI", "asset_id": "veh-" + ids.New("x"), "member_id": memberID, "source_version": 1}
	if status, _ := gateCall(t, router, http.MethodPost, "/fleet-partner/v1/managed-assets", asset, nil); status != http.StatusBadRequest {
		t.Fatalf("asset link without Idempotency-Key: status %d, want 400", status)
	}
	status, body := gateCall(t, router, http.MethodPost, "/fleet-partner/v1/managed-assets", asset, map[string]string{"Idempotency-Key": "mm-asset-" + ids.New("k")})
	if status != http.StatusOK {
		t.Fatalf("asset link: %d %v", status, body)
	}
	assetLink := memberData(t, body)
	linkID := idOf(assetLink)
	if f.audits(t, "managed_asset_link.linked", linkID) != 1 || f.outbox(t, "managed_asset_link.linked.v1", linkID) != 1 {
		t.Fatal("an asset link must be audited and announced")
	}
	status, body = gateCall(t, router, http.MethodPost, "/fleet-partner/v1/managed-assets/"+linkID+"/status",
		map[string]any{"status": "SUSPENDED", "expectedVersion": versionOf(assetLink)}, nil)
	if status != http.StatusOK {
		t.Fatalf("asset status: %d %v", status, body)
	}
	if f.audits(t, "managed_asset_link.status_changed", linkID) != 1 || f.outbox(t, "managed_asset_link.status_changed.v1", linkID) != 1 {
		t.Fatal("an asset status change must be audited and announced")
	}
}

func TestManagedMemberCrossTenantWritesLeaveNoTrace(t *testing.T) {
	f := newMemberFixture(t)
	router := f.router(t, f.orgID, f.member)
	_, created := gateCall(t, router, http.MethodPost, "/fleet-partner/v1/managed-members",
		map[string]any{"member_ref": "EMP-" + ids.New("x")}, map[string]string{"Idempotency-Key": "mm-" + ids.New("k")})
	member := memberData(t, created)
	memberID := idOf(member)

	other := newMemberFixtureOn(t, f.pool)
	outsider := other.router(t, other.orgID, other.member)
	if status, _ := gateCall(t, outsider, http.MethodGet, "/fleet-partner/v1/managed-members/"+memberID, nil, nil); status != http.StatusNotFound {
		t.Fatalf("cross-tenant read: status %d, want 404", status)
	}
	// A cross-tenant write reads as a version conflict, never as "exists".
	if status, _ := gateCall(t, outsider, http.MethodPost, "/fleet-partner/v1/managed-members/"+memberID+"/suspend",
		map[string]any{"expectedVersion": versionOf(member)}, nil); status != http.StatusConflict {
		t.Fatalf("cross-tenant suspend: status %d, want 409", status)
	}
	if n := other.count(t, `SELECT count(*) FROM fleet_partner_audit_records WHERE resource_id=$1 AND action LIKE 'managed_member.%' AND action <> 'managed_member.created'`, memberID); n != 0 {
		t.Fatalf("cross-tenant write produced %d audit rows", n)
	}
	// A member of the organization without the permission reaches nothing.
	plain := seedApprovalMember(t, f.pool, f.orgID, "", "plain")
	if status, _ := gateCall(t, f.router(t, f.orgID, plain), http.MethodPost, "/fleet-partner/v1/managed-members",
		map[string]any{"member_ref": "EMP-X"}, map[string]string{"Idempotency-Key": "mm-" + ids.New("k")}); status != http.StatusForbidden {
		t.Fatalf("no permission: status %d, want 403", status)
	}
}
