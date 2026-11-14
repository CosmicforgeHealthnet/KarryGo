package integration_test

// FLEET-9.15b: a role change invalidates cached authorization at once.
//
// The authorizer caches allow/deny decisions keyed by the membership's role
// version, which the tenant middleware publishes from PostgreSQL on every
// request. This test runs the production router with the authorizer cache
// configured as production configures it, so a refusal after a role change
// proves the cached "allow" was bypassed rather than never stored.

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	staffmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cache"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/stepup"
)

// memoryRedis is an in-process stand-in for the Redis client the cache wraps
// in production. It honours TTLs and counts authorization-cache hits.
type memoryRedis struct {
	mu        sync.Mutex
	values    map[string]string
	expiries  map[string]time.Time
	authzHits int
}

func newMemoryRedis() *memoryRedis {
	return &memoryRedis{values: map[string]string{}, expiries: map[string]time.Time{}}
}

func (m *memoryRedis) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[key] = value.(string)
	m.expiries[key] = time.Now().Add(expiration)
	return redis.NewStatusResult("OK", nil)
}

func (m *memoryRedis) Get(ctx context.Context, key string) *redis.StringCmd {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.values[key]
	if !ok || time.Now().After(m.expiries[key]) {
		return redis.NewStringResult("", redis.Nil)
	}
	if strings.Contains(key, "authz:") {
		m.authzHits++
	}
	return redis.NewStringResult(value, nil)
}

func (m *memoryRedis) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range keys {
		delete(m.values, key)
	}
	return redis.NewIntResult(int64(len(keys)), nil)
}

func (m *memoryRedis) hits() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.authzHits
}

func TestRow9_15b_RoleChangeInvalidatesCachedAuthorization(t *testing.T) {
	store := newMemoryRedis()
	router, h := buildOnboardingRouterWithCache(t, cache.New(store), nil)
	owner := onboardingLogin(t, router, h, "r915b-owner@example.com", "+2348012347501")
	orgID := rowsActiveCompany(t, router, h, owner, "Rows915b Cache Co", "RC-R915B-1")

	// The view-only role grants staff.read, which GET /staff needs.
	member := seedMember(t, h.pool, orgID, seedIdentity(t, h.pool, "r915b-staff@example.com", "+2348012347502"), false, nil)
	bindRole(t, h.pool, orgID, member.ID, "rol_system_view_only", staffmodels.ScopeOrganization, "")
	staff := rowsSignIn(t, router, h, "r915b-staff@example.com")

	staffList := func() int {
		t.Helper()
		code, _ := onboardCall(t, router, staff.token, "GET", "/staff", nil, 0)
		return code
	}
	if code := staffList(); code != http.StatusOK {
		t.Fatalf("member with staff.read = %d, want 200", code)
	}
	before := store.hits()
	if code := staffList(); code != http.StatusOK {
		t.Fatalf("repeat request = %d, want 200", code)
	}
	if store.hits() <= before {
		t.Fatal("the repeat request was not served from the authorization cache; the test would not prove invalidation")
	}

	grant := obtainStepUp(t, router, h, owner, stepup.ActionAccessManage)
	replaceRoles := func(roleID string) {
		t.Helper()
		code, body := onboardCallStepUp(t, router, owner, grant, "PUT", "/staff/"+member.ID+"/roles", map[string]any{"roleIds": []string{roleID}}, 0)
		if code != http.StatusOK {
			t.Fatalf("PUT /staff/%s/roles %s = %d %v", member.ID, roleID, code, body)
		}
	}

	// Remove staff.read: the CRM role does not hold it.
	replaceRoles("rol_system_crm")
	if code := staffList(); code != http.StatusForbidden {
		t.Fatalf("same session after losing staff.read = %d, want 403 (stale cached allow)", code)
	}
	if status := rowsSessionStatus(t, h, staff.sessionID); status != "ACTIVE" {
		t.Fatalf("session = %s; the refusal must come from the role change, not a revoked session", status)
	}

	// Grant it back: the same session regains access, no new login.
	replaceRoles("rol_system_view_only")
	if code := staffList(); code != http.StatusOK {
		t.Fatalf("same session after regaining staff.read = %d, want 200 (stale cached deny)", code)
	}
}
