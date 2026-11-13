package integration_test

import (
	"bytes"
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/app"
	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	authhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/http"
	authrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/repositories"
	authusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/usecases"
	orghttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/http"
	orgrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	orgusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/usecases"
	providerusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/usecases"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	staffusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/browserauth"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestOnboardingRepresentativeWireContract(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Onboarding wire test", "RC-ONBOARD-WIRE")
	if _, err := pool.Exec(context.Background(), `UPDATE fleet_partner_organizations SET status='DRAFT' WHERE id=$1`, org.ID); err != nil {
		t.Fatal(err)
	}
	user := seedAuditRouteUser(t, pool, org.ID, "rol_system_owner")
	router := closureRouter(t, pool, org.ID, user)
	payload := map[string]any{"fullName": "Test Representative", "roleTitle": "Managing Director", "email": "representative@example.com", "phone": "+2348012345678", "authorityType": "LEGAL_REPRESENTATIVE", "isOnboardingOwner": false}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/fleet-partner/v1/organizations/"+org.ID+"/representatives", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "onboarding-step3-baseline-authority")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	t.Logf("actual representative HTTP response: status=%d body=%s", response.Code, response.Body.String())
	if response.Code != 422 {
		t.Fatalf("expected field validation, got %d", response.Code)
	}
}

// Fully wired identity/session/tenant/permission boundary, real PostgreSQL, and
// the real HMAC-signed media client against a test authority. No runtime bypass.
func buildOnboardingRouter(t *testing.T, configureMedia ...func(*mediaTestServer)) (*gin.Engine, *authHarness) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	var org *orgusecases.Service
	var staff *staffusecases.Service
	h := newAuthHarness(t, nil, func(opts *authusecases.Options) {
		pool := opts.Pool
		publisher := events.NewDomainPublisher(events.NewOutboxRepository(pool))
		media := newMediaTestServer(t)
		for _, configure := range configureMedia {
			configure(media)
		}
		org = orgusecases.NewService(orgusecases.Options{Pool: pool, Organizations: orgrepos.NewOrganizationRepository(pool), Onboarding: orgrepos.NewOnboardingRepository(pool), Branches: orgrepos.NewBranchRepository(pool), Settings: orgrepos.NewSettingsRepository(pool), Events: publisher, Contracts: platformclients.ContractSet{Media: platformclients.NewMediaFileClient(platformclients.Options{ServiceName: "media-file-service", BaseURL: media.URL(), Secret: []byte(documentMediaSecret), Timeout: time.Second * 5}), Verification: platformclients.UnconfiguredVerification{Service: "verification-compliance-service"}}})
		staff = staffusecases.NewService(staffusecases.Options{Pool: pool, Members: staffrepos.NewMemberRepository(pool), Roles: staffrepos.NewRoleRepository(pool), Invitations: staffrepos.NewInvitationRepository(pool), Organizations: org, Sessions: opts.Sessions, Sender: opts.Sender, Events: publisher, InvitationSecret: []byte("invitation-test-secret")})
		org.SetMembershipCreator(staff)
		opts.Memberships = staff
		opts.Invitations = staff
	})
	ah := authhttp.NewHandler(h.service, []byte("test-contact-secret"), browserauth.Policy{Name: "karrygo_fleet_refresh", Path: "/fleet-partner/v1/auth", Secure: true, SameSite: http.SameSiteLaxMode, Origins: []string{"https://fleet.example.test", "http://127.0.0.1:3417"}})
	oh := orghttp.NewHandler(org, staff, providerusecases.NewService(providerusecases.Options{Pool: h.pool}), accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(h.pool), nil, 0), nil)
	return app.BuildRouter(app.Options{Metrics: observability.NewMetrics(), Config: config.Config{AppEnv: "test", MaxRequestBodyBytes: 32 << 20}, TokenSigner: auth.NewTokenSigner([]byte("test-access-secret")), Resolver: staffusecases.NewMembershipResolver(staffrepos.NewMemberRepository(h.pool), org), SessionStatus: authrepos.NewSessionRepository(h.pool), OrganizationStatus: app.OrganizationStatusFromPool(h.pool), PublicAuthRoutes: ah.RegisterPublicRoutes, IdentityRoutes: func(g *gin.RouterGroup) { ah.RegisterAuthenticatedRoutes(g); oh.RegisterOnboardingRoutes(g) }, TenantRoutes: oh.RegisterTenantRoutes}), h
}

func onboardingLogin(t *testing.T, router http.Handler, h *authHarness, email, phone string) string {
	t.Helper()
	start := postJSON(router.(*gin.Engine), "/fleet-partner/v1/auth/sign-up/start", map[string]string{"email": email, "phone": phone}, nil)
	if start.Code != 202 {
		t.Fatal(start.Body.String())
	}
	data := decodeEnvelope(t, start)["data"].(map[string]any)
	otp, _ := h.sender.LastOTP()
	verified := postJSON(router.(*gin.Engine), "/fleet-partner/v1/auth/verify", map[string]string{"challengeId": data["challengeId"].(string), "code": otp.Code}, nil)
	if verified.Code != 200 {
		t.Fatal(verified.Body.String())
	}
	return decodeEnvelope(t, verified)["data"].(map[string]any)["tokens"].(map[string]any)["accessToken"].(string)
}
func onboardCall(t *testing.T, router http.Handler, token, method, path string, payload any, version int) (int, map[string]any) {
	t.Helper()
	headers := map[string]string{"Authorization": "Bearer " + token, "Origin": "https://fleet.example.test", "Idempotency-Key": "onboarding-test"}
	if version > 0 {
		headers["If-Match"] = fmt.Sprint(version)
	}
	return documentCall(t, router, method, "/fleet-partner/v1"+path, payload, headers)
}
func TestOnboardingV2PersistedJourneyAndBoundaries(t *testing.T) {
	router, h := buildOnboardingRouter(t)
	token := onboardingLogin(t, router, h, "onboarding@example.com", "+2348012345601")
	call := func(method, path string, payload any, version, want int) map[string]any {
		t.Helper()
		code, body := onboardCall(t, router, token, method, path, payload, version)
		if code != want {
			t.Fatalf("%s %s status %d want %d: %v", method, path, code, want, body)
		}
		return body
	}
	company := map[string]any{"legalName": "Onboarding Company", "tradingName": "Trading Co", "registrationNumber": "RC-ONBOARD-V2", "organizationType": "PRIVATE_LIMITED", "countryCode": "NG", "timezone": "Africa/Lagos", "defaultCurrency": "NGN", "ownerName": "Test Owner"}
	created := documentData(t, call("POST", "/organizations", company, 0, 201))
	id := created["organizationId"].(string)
	base := "/organizations/" + id
	again := documentData(t, call("POST", "/organizations", company, 0, 201))
	if again["organizationId"] != id {
		t.Fatal("duplicate company created")
	}
	identity := documentData(t, call("GET", "/identity/profile", nil, 0, 200))
	if identity["email"] != "onboarding@example.com" || identity["fullName"] != "Test Owner" || identity["isOrganizationOwner"] != true {
		t.Fatal(identity)
	}
	current := documentData(t, call("GET", "/organizations/current", nil, 0, 200))
	if current["registrationNumber"] != "RC-ONBOARD-V2" || current["organizationType"] != "PRIVATE_LIMITED" {
		t.Fatal(current)
	}
	company["legalName"] = "Updated Company"
	delete(company, "registrationNumber")
	delete(company, "organizationType")
	updated := documentData(t, call("PUT", base, company, 1, 200))
	if updated["registrationNumber"] != "RC-ONBOARD-V2" || updated["organizationType"] != "PRIVATE_LIMITED" || updated["version"] != float64(2) {
		t.Fatal(updated)
	}
	call("PUT", base, company, 1, 409)
	createOrganization(t, h.pool, "Other Registration Owner", "RC-REGISTRATION-TAKEN")
	duplicateRegistration := map[string]any{"legalName": "Updated Company", "registrationNumber": "RC-REGISTRATION-TAKEN", "countryCode": "NG", "timezone": "Africa/Lagos", "defaultCurrency": "NGN"}
	call("PUT", base, duplicateRegistration, 2, 409)
	badRegionalSettings := map[string]any{"legalName": "Updated Company", "countryCode": "NG", "timezone": "Not/AZone", "defaultCurrency": "NGN"}
	call("PUT", base, badRegionalSettings, 2, 422)
	badRegionalSettings["timezone"] = "Africa/Lagos"
	badRegionalSettings["defaultCurrency"] = "INVALID"
	call("PUT", base, badRegionalSettings, 2, 422)
	badRegionalSettings["defaultCurrency"] = "NGN"
	badRegionalSettings["countryCode"] = "NGA"
	call("PUT", base, badRegionalSettings, 2, 422)

	address := map[string]any{"street": "1 Test Road", "city": "Lagos", "state": "Lagos", "country": "NG"}
	operating := map[string]any{"street": "2 Depot Road", "city": "Abuja", "state": "FCT", "country": "NG"}
	profile := documentData(t, call("PUT", base+"/profile", map[string]any{"registeredAddress": address, "operatingAddress": operating, "businessEmail": "business@example.com", "businessPhone": "+2348012345699", "taxReference": "TAX-RETAIN"}, 0, 200))
	if profile["version"] != float64(1) {
		t.Fatal(profile)
	}
	call("PUT", base+"/profile", map[string]any{"website": "https://example.com"}, 0, 409)
	saved := documentData(t, call("PUT", base+"/profile", map[string]any{"website": "https://example.com"}, 1, 200))
	if saved["taxReference"] != "TAX-RETAIN" || saved["businessEmail"] != "business@example.com" || saved["version"] != float64(2) {
		t.Fatal(saved)
	}
	call("PUT", base+"/profile", map[string]any{"website": "https://stale.example.com"}, 1, 409)
	rep := map[string]any{"fullName": "Test Owner", "roleTitle": "Director", "email": "onboarding@example.com", "phone": "+2348012345601", "authorityType": "DIRECTOR", "isOnboardingOwner": true}
	representative := documentData(t, call("POST", base+"/representatives", rep, 0, 201))
	repID := representative["id"].(string)
	if representative["version"] != float64(1) || representative["identityReference"] != identity["identityId"] {
		t.Fatal(representative)
	}
	duplicate := documentData(t, call("POST", base+"/representatives", rep, 0, 201))
	if duplicate["id"] != repID {
		t.Fatal("duplicate representative created")
	}
	rep["roleTitle"] = "Managing Director"
	rep["status"] = "ACTIVE"
	edited := documentData(t, call("PUT", base+"/representatives/"+repID, rep, 1, 200))
	if edited["version"] != float64(2) || edited["status"] != "ACTIVE" {
		t.Fatal(edited)
	}
	call("PUT", base+"/representatives/"+repID, rep, 1, 409)
	rep["authorityType"] = "LEGAL_REPRESENTATIVE"
	call("PUT", base+"/representatives/"+repID, rep, 2, 422)
	call("PUT", base+"/capabilities", map[string]any{"capabilities": []string{"UNSUPPORTED"}}, 0, 422)
	call("PUT", base+"/capabilities", map[string]any{"capabilities": []string{"PACKAGE_DELIVERY"}}, 0, 200)
	call("PUT", base+"/stages/ACTIVATION", map[string]any{"status": "COMPLETE"}, 0, 403)
	call("PUT", base+"/stages/BUSINESS_DOCUMENTS", map[string]any{"status": "COMPLETE"}, 0, 422)
	call("POST", base+"/submit", map[string]any{}, 2, 422)
	other := onboardingLogin(t, router, h, "other-onboarding@example.com", "+2348012345602")
	code, _ := onboardCall(t, router, other, "GET", base+"/representatives", nil, 0)
	if code < 400 {
		t.Fatal("cross-tenant read succeeded")
	}
	// Same identity contact projection ignores requester-supplied identity query.
	self := documentData(t, call("GET", "/identity/profile?identityId=someone-else", nil, 0, 200))
	if self["identityId"] != identity["identityId"] {
		t.Fatal("identity projection escaped caller")
	}
	var count int
	h.pool.QueryRow(context.Background(), `SELECT count(*) FROM fleet_partner_organization_authorized_representatives WHERE organization_id=$1`, id).Scan(&count)
	if count != 1 {
		t.Fatal("unexpected representative count", count)
	}
}
func TestOnboardingV2ConcurrentCompanyCreation(t *testing.T) {
	router, h := buildOnboardingRouter(t)
	token := onboardingLogin(t, router, h, "race-onboarding@example.com", "+2348012345603")
	payload := map[string]any{"legalName": "Race Company", "registrationNumber": "RC-ONBOARD-RACE", "organizationType": "LLC", "countryCode": "NG", "timezone": "Africa/Lagos", "defaultCurrency": "NGN", "ownerName": "Race Owner"}
	var wg sync.WaitGroup
	results := make(chan map[string]any, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, body := onboardCall(t, router, token, "POST", "/organizations", payload, 0)
			if code != 201 {
				t.Errorf("concurrent create %d %v", code, body)
			}
			results <- body
		}()
	}
	wg.Wait()
	close(results)
	ids := map[any]bool{}
	for result := range results {
		ids[documentData(t, result)["organizationId"]] = true
	}
	if len(ids) != 1 {
		t.Fatal("concurrent company duplicates", ids)
	}
}

func TestOnboardingV2RepresentativeSelectionAndStatus(t *testing.T) {
	router, h := buildOnboardingRouter(t, func(media *mediaTestServer) {
		media.mu.Lock()
		defer media.mu.Unlock()
		media.objects["mobj_foreign_authority"] = &mediaObjectRecord{ID: "mobj_foreign_authority", Organization: "org_other_tenant", OwnerType: "organization", OwnerRef: "org_other_tenant", UploadStatus: "COMPLETED", SafetyStatus: "SAFE", DeletionState: "ACTIVE", SourceVersion: 1}
	})
	token := onboardingLogin(t, router, h, "history-onboarding@example.com", "+2348012345604")
	code, body := onboardCall(t, router, token, "POST", "/organizations", map[string]any{"legalName": "Representative History", "registrationNumber": "RC-REP-HISTORY", "organizationType": "LLC", "countryCode": "NG", "timezone": "Africa/Lagos", "defaultCurrency": "NGN", "ownerName": "Owner"}, 0)
	if code != 201 {
		t.Fatal(body)
	}
	base := "/organizations/" + documentData(t, body)["organizationId"].(string) + "/representatives"
	foreignEvidence := map[string]any{"fullName": "Invalid Evidence Owner", "roleTitle": "Director", "email": "evidence-owner@example.com", "phone": "+2348012345697", "authorityType": "DIRECTOR", "authorityDocumentFileId": "mobj_foreign_authority"}
	code, body = onboardCall(t, router, token, "POST", base, foreignEvidence, 0)
	if code != 404 {
		t.Fatal("foreign representative evidence was accepted", code, body)
	}
	records := []map[string]any{}
	for i := 0; i < 2; i++ {
		payload := map[string]any{"fullName": fmt.Sprintf("Representative %d", i), "roleTitle": "Officer", "email": fmt.Sprintf("rep%d@example.com", i), "phone": fmt.Sprintf("+234801234561%d", i), "authorityType": "OFFICER", "isOnboardingOwner": false}
		code, body = onboardCall(t, router, token, "POST", base, payload, 0)
		if code != 201 {
			t.Fatal(body)
		}
		record := documentData(t, body)
		if record["identityReference"] != nil {
			t.Fatal("another person was linked to owner")
		}
		records = append(records, record)
	}
	selected := records[1]
	selected["fullName"] = "Intended Second Record"
	selected["status"] = "ENDED"
	code, body = onboardCall(t, router, token, "PUT", base+"/"+selected["id"].(string), selected, 1)
	if code != 200 {
		t.Fatal(body)
	}
	selected = documentData(t, body)
	delete(selected, "status")
	code, body = onboardCall(t, router, token, "PUT", base+"/"+selected["id"].(string), selected, 2)
	if code != 200 || documentData(t, body)["status"] != "ENDED" {
		t.Fatal("omitted status was not preserved", body)
	}
	_, listed := onboardCall(t, router, token, "GET", base, nil, 0)
	list := listed["data"].([]any)
	if len(list) != 2 || list[0].(map[string]any)["fullName"] != records[0]["fullName"] {
		t.Fatal("wrong representative changed", listed)
	}
	selected["email"] = "not-an-email"
	code, body = onboardCall(t, router, token, "PUT", base+"/"+selected["id"].(string), selected, 3)
	if code != 422 {
		t.Fatal("invalid email accepted", body)
	}
}
