package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	authrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/repositories"
	governancehttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/governance/http"
	governancemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/governance/models"
	retentionusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/usecases"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

func days(v int) *int { return &v }

// TestPrivacySettingsViewTheEnforcedRetentionPolicy proves GET/PATCH
// /privacy-settings (spec 59.10) is a versioned view over the one retention
// store: only enforced classes are writable, protected classes and values
// below the platform minimum are refused, If-Match is required and checked,
// a write to the retention store from elsewhere invalidates the version, the
// change is audited and announced once, other tenants are untouched, and the
// route needs privacy.manage and an organization-wide caller.
func TestPrivacySettingsViewTheEnforcedRetentionPolicy(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Privacy Settings "+ids.New("o"), "PRV-"+ids.New("r"))
	other := createOrganization(t, pool, "Privacy Settings Other "+ids.New("o"), "PRVO-"+ids.New("r"))
	stamp := fmt.Sprint(time.Now().UnixNano() % 100000000)
	owner := seedApprovalMember(t, pool, org.ID, "rol_system_owner", "privacy-owner-"+stamp)
	nobody := seedMember(t, pool, org.ID, seedIdentity(t, pool, ids.New("p")+"@example.test", "+23483"+stamp), false, nil).ID

	publisher := events.NewDomainPublisher(events.NewOutboxRepository(pool))
	newGov := func(enabled bool) interface {
		GetPrivacySettings(context.Context, tenant.Context) (governancemodels.PrivacySettings, error)
		UpdatePrivacySettings(context.Context, tenant.Context, governancemodels.PrivacySettingsPatch) (governancemodels.PrivacySettings, error)
	} {
		g := app.NewGovernanceService(config.Config{PrivacySettingsEnabled: enabled}, pool, publisher, authrepos.NewSessionRepository(pool))
		return g.WithPrivacyRetention(app.NewGovernancePrivacyRetention(pool, nil))
	}
	scope := tenant.Context{OrganizationID: org.ID, OrganizationUserID: owner, ActorType: tenant.ActorTypeOrganizationUser, ActorID: owner, CorrelationID: ids.New("c")}

	if _, err := newGov(false).GetPrivacySettings(ctx, scope); appCode(err) != "not_found" {
		t.Fatalf("disabled read = %v, want not_found", err)
	}
	gov := newGov(true)
	initial, err := gov.GetPrivacySettings(ctx, scope)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Version != 0 || len(initial.Retention) != 4 || len(initial.ProtectedRetention) != 6 || !initial.LegalHoldsOverrideRetention {
		t.Fatalf("initial settings = %+v", initial)
	}
	if got := initial.Retention["OPERATIONAL"]; got.RetentionDays != 90 || got.PlatformMinimumDays != 90 || !got.Configurable {
		t.Fatalf("OPERATIONAL default = %+v", got)
	}
	if got := initial.ProtectedRetention["FINANCIAL"]; got.Configurable || got.RetentionDays != 2555 {
		t.Fatalf("FINANCIAL = %+v, want read-only platform minimum", got)
	}

	refusals := []struct {
		name string
		in   governancemodels.PrivacySettingsPatch
		code string
	}{
		{"missing If-Match", governancemodels.PrivacySettingsPatch{ExpectedVersion: -1, Retention: map[string]governancemodels.RetentionPatch{"OPERATIONAL": {RetentionDays: days(120)}}}, "bad_request"},
		{"protected class", governancemodels.PrivacySettingsPatch{Retention: map[string]governancemodels.RetentionPatch{"FINANCIAL": {RetentionDays: days(9000)}}}, "validation_failed"},
		{"unknown class", governancemodels.PrivacySettingsPatch{Retention: map[string]governancemodels.RetentionPatch{"NOT_A_CLASS": {RetentionDays: days(100)}}}, "validation_failed"},
		{"below platform minimum", governancemodels.PrivacySettingsPatch{Retention: map[string]governancemodels.RetentionPatch{"OPERATIONAL": {RetentionDays: days(30)}}}, "validation_failed"},
		{"missing days", governancemodels.PrivacySettingsPatch{Retention: map[string]governancemodels.RetentionPatch{"OPERATIONAL": {}}}, "validation_failed"},
		{"empty patch", governancemodels.PrivacySettingsPatch{}, "validation_failed"},
	}
	for _, r := range refusals {
		if _, err := gov.UpdatePrivacySettings(ctx, scope, r.in); appCode(err) != r.code {
			t.Fatalf("%s: err = %v, want %s", r.name, err, r.code)
		}
	}
	branchScoped := scope
	branchScoped.PermittedBranchIDs = []string{ids.New("brn")}
	if _, err := gov.UpdatePrivacySettings(ctx, branchScoped, governancemodels.PrivacySettingsPatch{Retention: map[string]governancemodels.RetentionPatch{"OPERATIONAL": {RetentionDays: days(120)}}}); appCode(err) != "forbidden" {
		t.Fatalf("branch-scoped write = %v, want forbidden", err)
	}

	updated, err := gov.UpdatePrivacySettings(ctx, scope, governancemodels.PrivacySettingsPatch{ExpectedVersion: 0,
		Retention: map[string]governancemodels.RetentionPatch{"OPERATIONAL": {RetentionDays: days(120)}, "SUPPORT": {RetentionDays: days(1000)}}})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 || updated.Retention["OPERATIONAL"].RetentionDays != 120 || updated.Retention["SUPPORT"].RetentionDays != 1000 {
		t.Fatalf("updated = %+v", updated)
	}
	// The write landed in the retention store the executors read.
	policy, err := retentionusecases.NewService(pool).Get(ctx, org.ID, "OPERATIONAL")
	if err != nil || policy.EffectiveDays() != 120 {
		t.Fatalf("retention store OPERATIONAL = %+v, %v", policy, err)
	}
	if _, err := gov.UpdatePrivacySettings(ctx, scope, governancemodels.PrivacySettingsPatch{ExpectedVersion: 0,
		Retention: map[string]governancemodels.RetentionPatch{"OPERATIONAL": {RetentionDays: days(150)}}}); appCode(err) != "conflict" {
		t.Fatalf("stale If-Match = %v, want conflict", err)
	}
	// A change made through /retention-policies moves the version too.
	if _, err := retentionusecases.NewService(pool).Update(ctx, retentionusecases.UpdateInput{Actor: retentionusecases.Actor{OrganizationID: org.ID, ActorType: "organization_user", ActorID: owner},
		RetentionClass: "TEMPORARY", RetentionDays: 7, RequestID: ids.New("req")}); err != nil {
		t.Fatal(err)
	}
	if _, err := gov.UpdatePrivacySettings(ctx, scope, governancemodels.PrivacySettingsPatch{ExpectedVersion: updated.Version,
		Retention: map[string]governancemodels.RetentionPatch{"OPERATIONAL": {RetentionDays: days(150)}}}); appCode(err) != "conflict" {
		t.Fatalf("version after an external retention change = %v, want conflict", err)
	}
	current, err := gov.GetPrivacySettings(ctx, scope)
	if err != nil || current.Version != 3 || current.Retention["TEMPORARY"].RetentionDays != 7 {
		t.Fatalf("current = %+v, %v", current, err)
	}
	// Re-sending the current value changes nothing and announces nothing.
	same, err := gov.UpdatePrivacySettings(ctx, scope, governancemodels.PrivacySettingsPatch{ExpectedVersion: current.Version,
		Retention: map[string]governancemodels.RetentionPatch{"OPERATIONAL": {RetentionDays: days(120)}}})
	if err != nil || same.Version != current.Version {
		t.Fatalf("no-op patch = %+v, %v", same, err)
	}
	var announced, audited int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_outbox WHERE event_type='privacy_settings.updated.v1' AND aggregate_id=$1`, org.ID).Scan(&announced); err != nil || announced != 1 {
		t.Fatalf("privacy_settings.updated.v1 events = %d, %v; want 1", announced, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id=$1 AND action='privacy_settings.updated'`, org.ID).Scan(&audited); err != nil || audited != 1 {
		t.Fatalf("privacy_settings.updated audit records = %d, %v; want 1", audited, err)
	}
	otherScope := scope
	otherScope.OrganizationID = other.ID
	if theirs, err := gov.GetPrivacySettings(ctx, otherScope); err != nil || theirs.Version != 0 || theirs.Retention["OPERATIONAL"].RetentionDays != 90 {
		t.Fatalf("other tenant settings = %+v, %v", theirs, err)
	}

	// HTTP: permission, strict merge-patch decoding and If-Match parsing.
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		member := c.GetHeader("X-Test-Member")
		c.Set(middleware.TenantContextKey, tenant.Context{OrganizationID: org.ID, OrganizationUserID: member, ActorType: tenant.ActorTypeOrganizationUser, ActorID: member})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0)
	httpGov := app.NewGovernanceService(config.Config{PrivacySettingsEnabled: true}, pool, publisher, authrepos.NewSessionRepository(pool))
	httpGov.WithPrivacyRetention(app.NewGovernancePrivacyRetention(pool, nil))
	governancehttp.NewHandler(httpGov, authorizer, idempotency.NewStore(pool, 0)).RegisterTenantRoutes(engine.Group("/fleet-partner/v1"))
	call := func(method, member, ifMatch, body string) (int, map[string]any) {
		req := httptest.NewRequest(method, "/fleet-partner/v1/privacy-settings", bytes.NewBufferString(body))
		req.Header.Set("X-Test-Member", member)
		req.Header.Set("Content-Type", "application/json")
		if method != http.MethodGet {
			req.Header.Set("Idempotency-Key", ids.New("idem"))
		}
		if ifMatch != "" {
			req.Header.Set("If-Match", ifMatch)
		}
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		var envelope map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &envelope)
		return rec.Code, envelope
	}
	if code, _ := call(http.MethodGet, nobody, "", ""); code != http.StatusForbidden {
		t.Fatalf("GET without privacy.manage = %d, want 403", code)
	}
	if code, _ := call(http.MethodPatch, nobody, "3", `{"retention":{"OPERATIONAL":{"retentionDays":200}}}`); code != http.StatusForbidden {
		t.Fatalf("PATCH without privacy.manage = %d, want 403", code)
	}
	code, got := call(http.MethodGet, owner, "", "")
	if code != http.StatusOK {
		t.Fatalf("GET = %d %v", code, got)
	}
	if v := got["data"].(map[string]any)["version"].(float64); v != 3 {
		t.Fatalf("GET version = %v, want 3", v)
	}
	if code, _ := call(http.MethodPatch, owner, "", `{"retention":{"OPERATIONAL":{"retentionDays":200}}}`); code != http.StatusBadRequest {
		t.Fatalf("PATCH without If-Match = %d, want 400", code)
	}
	if code, _ := call(http.MethodPatch, owner, "3", `{"version":9,"retention":{"OPERATIONAL":{"retentionDays":200}}}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("PATCH with a read-only field = %d, want 422", code)
	}
	if code, _ := call(http.MethodPatch, owner, "3", `{"retention":{"OPERATIONAL":{"retentionDays":200,"platformMinimumDays":1}}}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("PATCH changing a platform minimum = %d, want 422", code)
	}
	code, got = call(http.MethodPatch, owner, `"3"`, `{"retention":{"OPERATIONAL":{"retentionDays":200}}}`)
	if code != http.StatusOK {
		t.Fatalf("PATCH = %d %v", code, got)
	}
	if v := got["data"].(map[string]any)["version"].(float64); v != 4 {
		t.Fatalf("PATCH version = %v, want 4", v)
	}
}
