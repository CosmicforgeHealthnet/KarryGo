package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	authmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/models"
	authrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/repositories"
	safetyhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/safety/http"
	safetymodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/safety/models"
	staffmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"

	"github.com/jackc/pgx/v5"
)

// seedOrganizationUser creates a corporate member and optionally grants it a
// seeded system role.
//
// The role is granted through the real binding repository rather than a raw
// INSERT so the test exercises the same authority path production does.
func seedOrganizationUser(t *testing.T, f *safetyFixture, roleID string) string {
	t.Helper()
	ctx := context.Background()
	members := staffrepos.NewMemberRepository(f.pool)
	roles := staffrepos.NewRoleRepository(f.pool)

	// An organization user is always anchored to a corporate identity; the
	// foreign key is what keeps a membership from existing without one.
	identity := &authmodels.Identity{
		EmailNormalized: "safety-" + ids.New("id") + "@example.com",
		EmailDisplay:    "safety@example.com",
		PhoneE164:       "+234800" + ids.New("p")[len(ids.New("p"))-7:],
		PhoneDisplay:    "08000000000",
		Status:          authmodels.IdentityStatusActive,
	}
	member := &staffmodels.OrganizationUser{
		Status: staffmodels.UserStatusActive,
	}
	if err := database.WithinTx(ctx, f.pool, func(tx pgx.Tx) error {
		if err := authrepos.NewIdentityRepository(f.pool).Create(ctx, tx, identity); err != nil {
			return err
		}
		member.IdentityID = identity.ID
		if err := members.Create(ctx, tx, f.orgID, member); err != nil {
			return err
		}
		if roleID == "" {
			return nil
		}
		return roles.CreateBinding(ctx, tx, f.orgID, &staffmodels.RoleBinding{
			OrganizationUserID: member.ID,
			RoleID:             roleID,
			ScopeType:          staffmodels.ScopeOrganization,
			GrantedBy:          "test",
		})
	}); err != nil {
		t.Fatalf("seed organization user: %v", err)
	}
	return member.ID
}

// safetyRouter mounts the real safety routes behind an injected tenant
// context and the real authorizer.
//
// The tenant middleware itself is exercised by the auth suite; what this test
// needs to prove is that the safety routes deny by default and enforce the
// two distinct permissions.
func safetyRouter(t *testing.T, f *safetyFixture, organizationUserID string) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	// The dispute stub must mirror production: support-dispute-service exposes
	// no by-identifier dispute lookup, so SupportDisputeReferenceClient always
	// answers ErrValidationEndpointAbsent. A stub that reports disputes as
	// verifiable would let these route tests assert a VERIFIED state that the
	// real client can never produce.
	service := newSafetyService(f.pool, &stubValidator{
		incidentExists: true,
		configured:     true,
		disputeErr:     platformclients.ErrValidationEndpointAbsent,
	})
	handler := safetyhttp.NewHandler(service,
		accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(f.pool), nil, 0))

	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(middleware.TenantContextKey, tenant.Context{
			OrganizationID:     f.orgID,
			OrganizationUserID: organizationUserID,
			ActorType:          tenant.ActorTypeOrganizationUser,
			ActorID:            organizationUserID,
		})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	group := engine.Group("/fleet-partner/v1")
	handler.RegisterTenantRoutes(group)
	return engine
}

func postSafetyReference(t *testing.T, router http.Handler, idempotencyKey string,
	body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost,
		"/fleet-partner/v1/safety/references", bytes.NewReader(encoded))
	req.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

// TestSafetyRoutesDenyWithoutPermission proves deny-by-default: a member of
// the organization holding no role reaches nothing.
func TestSafetyRoutesDenyWithoutPermission(t *testing.T) {
	f := newSafetyFixture(t, "Safety RBAC Deny", "RC-SAFH-1")
	memberID := seedOrganizationUser(t, f, "")
	router := safetyRouter(t, f, memberID)

	recorder := postSafetyReference(t, router, "safety-http-key-1", map[string]any{
		"referenceType":     safetymodels.ReferenceIncident,
		"externalReference": "cmp_rbac_1",
		"assignmentId":      f.assignment.ID,
	})
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("write status = %d, want 403; body = %s", recorder.Code, recorder.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/fleet-partner/v1/safety/references", nil)
	listRecorder := httptest.NewRecorder()
	router.ServeHTTP(listRecorder, listReq)
	if listRecorder.Code != http.StatusForbidden {
		t.Fatalf("read status = %d, want 403", listRecorder.Code)
	}
}

// TestSafetyReadPermissionDoesNotGrantWrite proves the two permissions are
// genuinely separate: someone who may look up a case may not assert one.
func TestSafetyReadPermissionDoesNotGrantWrite(t *testing.T) {
	f := newSafetyFixture(t, "Safety RBAC Read", "RC-SAFH-2")
	// The compliance role is seeded with safety.references.read only.
	memberID := seedOrganizationUser(t, f, "rol_system_compliance")
	router := safetyRouter(t, f, memberID)

	listReq := httptest.NewRequest(http.MethodGet, "/fleet-partner/v1/safety/references", nil)
	listRecorder := httptest.NewRecorder()
	router.ServeHTTP(listRecorder, listReq)
	if listRecorder.Code != http.StatusOK {
		t.Fatalf("read status = %d, want 200; body = %s",
			listRecorder.Code, listRecorder.Body.String())
	}

	recorder := postSafetyReference(t, router, "safety-http-key-2", map[string]any{
		"referenceType":     safetymodels.ReferenceIncident,
		"externalReference": "cmp_rbac_2",
		"assignmentId":      f.assignment.ID,
	})
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("write status = %d, want 403; read must not imply write", recorder.Code)
	}
}

// TestSafetyManagePermissionRecordsReference proves the happy path through the
// real routes, and that an unverified record is labelled in the response.
func TestSafetyManagePermissionRecordsReference(t *testing.T) {
	f := newSafetyFixture(t, "Safety RBAC Manage", "RC-SAFH-3")
	memberID := seedOrganizationUser(t, f, "rol_system_dispatcher")
	router := safetyRouter(t, f, memberID)

	recorder := postSafetyReference(t, router, "safety-http-key-3", map[string]any{
		"referenceType":     safetymodels.ReferenceIncident,
		"externalReference": "cmp_rbac_3",
		"assignmentId":      f.assignment.ID,
	})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Data safetyhttp.ReferenceView `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if response.Data.VerificationState != safetymodels.VerificationVerified {
		t.Fatalf("verificationState = %q", response.Data.VerificationState)
	}
	if !response.Data.AuthorizesConsequences {
		t.Fatal("a verified reference must report that it may justify action")
	}
	if response.Data.Warning != nil {
		t.Fatalf("a verified reference must carry no warning, got %q", *response.Data.Warning)
	}

	// A retry replays rather than creating a second record.
	replay := postSafetyReference(t, router, "safety-http-key-3", map[string]any{
		"referenceType":     safetymodels.ReferenceIncident,
		"externalReference": "cmp_rbac_3",
		"assignmentId":      f.assignment.ID,
	})
	if replay.Code != http.StatusOK {
		t.Fatalf("replay status = %d, want 200; body = %s", replay.Code, replay.Body.String())
	}
}

// TestSafetyRouteRequiresIdempotencyKey proves the write refuses a command it
// cannot deduplicate.
func TestSafetyRouteRequiresIdempotencyKey(t *testing.T) {
	f := newSafetyFixture(t, "Safety RBAC Key", "RC-SAFH-4")
	memberID := seedOrganizationUser(t, f, "rol_system_dispatcher")
	router := safetyRouter(t, f, memberID)

	recorder := postSafetyReference(t, router, "", map[string]any{
		"referenceType":     safetymodels.ReferenceIncident,
		"externalReference": "cmp_rbac_4",
		"assignmentId":      f.assignment.ID,
	})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", recorder.Code, recorder.Body.String())
	}
}

// TestUnverifiedReferenceIsLabelledInTheResponse proves a client cannot miss
// that a record is unconfirmed.
func TestUnverifiedReferenceIsLabelledInTheResponse(t *testing.T) {
	f := newSafetyFixture(t, "Safety RBAC Warning", "RC-SAFH-5")
	memberID := seedOrganizationUser(t, f, "rol_system_dispatcher")
	router := safetyRouter(t, f, memberID)

	recorder := postSafetyReference(t, router, "safety-http-key-5", map[string]any{
		// DISPUTE has no authoritative by-identifier lookup, so it is stored
		// unverified.
		"referenceType":     safetymodels.ReferenceDispute,
		"externalReference": "dsp_rbac_5",
		"assignmentId":      f.assignment.ID,
	})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Data safetyhttp.ReferenceView `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if response.Data.VerificationState != safetymodels.VerificationUnverified {
		t.Fatalf("verificationState = %q, want UNVERIFIED", response.Data.VerificationState)
	}
	if response.Data.AuthorizesConsequences {
		t.Fatal("an unverified reference must not report that it may justify action")
	}
	if response.Data.Warning == nil {
		t.Fatal("an unverified reference must carry an explicit warning")
	}
}
