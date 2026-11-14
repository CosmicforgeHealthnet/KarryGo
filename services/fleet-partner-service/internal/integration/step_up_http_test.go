package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	authusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/usecases"
	companywallethttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/http"
	companywalletusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/usecases"
	exportshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/http"
	exportsmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/models"
	exportsrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/repositories"
	exportsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/integrations"
	legalholdmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/legalhold/models"
	legalholdusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/legalhold/usecases"
	orgmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	orgrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	orgusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/usecases"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/stepup"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// authStepUpVerifier checks protected routes against the real auth service's
// grants, the same thing production's composition-root adapter does.
type authStepUpVerifier struct{ service *authusecases.Service }

func (v authStepUpVerifier) Verify(ctx context.Context, reference, identityID, sessionID, organizationID, actionClass string) error {
	return v.service.VerifyStepUpGrant(ctx, reference, identityID, sessionID, organizationID, actionClass)
}

const stepUpOrigin = "https://fleet.example.test"

// obtainStepUp runs the real email-OTP step-up flow over HTTP
// (POST /auth/step-up/start then /auth/step-up/verify) and returns the grant
// reference.
func obtainStepUp(t *testing.T, router http.Handler, h *authHarness, token, actionClass string) string {
	t.Helper()
	headers := map[string]string{"Authorization": "Bearer " + token, "Origin": stepUpOrigin}
	code, body := documentCall(t, router, "POST", "/fleet-partner/v1/auth/step-up/start", map[string]any{"actionClass": actionClass}, headers)
	if code != http.StatusAccepted {
		t.Fatalf("step-up start = %d: %v", code, body)
	}
	challengeID := documentData(t, body)["challengeId"].(string)
	otp, _ := h.sender.LastOTP()
	code, body = documentCall(t, router, "POST", "/fleet-partner/v1/auth/step-up/verify", map[string]any{"challengeId": challengeID, "code": otp.Code}, headers)
	if code != http.StatusOK {
		t.Fatalf("step-up verify = %d: %v", code, body)
	}
	reference, _ := documentData(t, body)["stepUpReference"].(string)
	if reference == "" {
		t.Fatalf("step-up verify returned no reference: %v", body)
	}
	return reference
}

// stepUpCall sends a corporate request, attaching the step-up reference when
// one is given.
func stepUpCall(t *testing.T, router http.Handler, token, reference, method, path string, payload any, extra map[string]string) (int, map[string]any) {
	t.Helper()
	headers := map[string]string{"Authorization": "Bearer " + token, "Origin": stepUpOrigin, "Idempotency-Key": "stepup-" + method + path + reference}
	if reference != "" {
		headers[stepup.HeaderReference] = reference
	}
	for key, value := range extra {
		headers[key] = value
	}
	return documentCall(t, router, method, "/fleet-partner/v1"+path, payload, headers)
}

// envelopeErrorCode returns the envelope's error code, or "".
func envelopeErrorCode(body map[string]any) string {
	if envelope, ok := body["error"].(map[string]any); ok {
		code, _ := envelope["code"].(string)
		return code
	}
	return ""
}

// requireStepUpRefusal asserts a step_up_required/step_up_expired refusal
// that names the action class the client must verify for.
func requireStepUpRefusal(t *testing.T, label string, code int, body map[string]any, wantCode, actionClass string) {
	t.Helper()
	if code != http.StatusForbidden || envelopeErrorCode(body) != wantCode {
		t.Fatalf("%s = %d %v, want 403 %s", label, code, body, wantCode)
	}
	details, _ := body["error"].(map[string]any)["details"].([]any)
	for _, detail := range details {
		entry, _ := detail.(map[string]any)
		if entry["field"] == stepup.DetailActionClass && entry["message"] == actionClass {
			return
		}
	}
	t.Fatalf("%s: refusal does not name action class %q: %v", label, actionClass, body)
}

func createStepUpOrganization(t *testing.T, router http.Handler, h *authHarness, email, phone, registration string) (token, organizationID string) {
	t.Helper()
	token = onboardingLogin(t, router, h, email, phone)
	company := map[string]any{"legalName": "Step-up " + registration, "registrationNumber": registration, "organizationType": "PRIVATE_LIMITED", "countryCode": "NG", "timezone": "Africa/Lagos", "defaultCurrency": "NGN", "ownerName": "Step-up Owner"}
	code, body := onboardCall(t, router, token, "POST", "/organizations", company, 0)
	if code != http.StatusCreated {
		t.Fatalf("create organization: %d %v", code, body)
	}
	organizationID = documentData(t, body)["organizationId"].(string)
	return token, organizationID
}

func activateOrganization(t *testing.T, h *authHarness, organizationID string) {
	t.Helper()
	if _, err := h.pool.Exec(context.Background(), `UPDATE fleet_partner_organizations SET status='ACTIVE' WHERE id=$1`, organizationID); err != nil {
		t.Fatal(err)
	}
}

// TestStepUp_RoleAndPermissionChangesRequireAFreshGrant drives the production
// router (tenant resolution, lifecycle gate, step-up middleware) and the real
// auth service: role and permission changes refuse without a grant, refuse a
// grant for another action class, another identity, another session, a forged
// or an expired reference, and succeed with a fresh grant for this exact
// session, organization and action class. The audit row cites the grant.
func TestStepUp_RoleAndPermissionChangesRequireAFreshGrant(t *testing.T) {
	router, h := buildOnboardingRouter(t)
	owner, orgID := createStepUpOrganization(t, router, h, "stepup-iam@example.com", "+2348012347001", "RC-STEPUP-IAM")
	role := map[string]any{"code": "stepup_lead", "name": "Step-up lead", "permissions": []string{"staff.read"}}

	// No grant at all.
	code, body := stepUpCall(t, router, owner, "", "POST", "/roles", role, nil)
	requireStepUpRefusal(t, "create role without step-up", code, body, "step_up_required", stepup.ActionAccessManage)

	// A forged reference.
	code, body = stepUpCall(t, router, owner, "chl_forged", "POST", "/roles", role, nil)
	requireStepUpRefusal(t, "create role with forged reference", code, body, "step_up_required", stepup.ActionAccessManage)

	// A real grant for a different action class is single-purpose.
	closureGrant := obtainStepUp(t, router, h, owner, stepup.ActionOrganizationClosure)
	code, body = stepUpCall(t, router, owner, closureGrant, "POST", "/roles", role, nil)
	requireStepUpRefusal(t, "create role with a closure grant", code, body, "step_up_required", stepup.ActionAccessManage)

	// The right grant.
	grant := obtainStepUp(t, router, h, owner, stepup.ActionAccessManage)
	code, body = stepUpCall(t, router, owner, grant, "POST", "/roles", role, nil)
	if code != http.StatusCreated {
		t.Fatalf("create role with step-up = %d %v", code, body)
	}
	roleID := documentData(t, body)["id"].(string)
	var cited string
	if err := h.pool.QueryRow(context.Background(), `SELECT COALESCE(step_up_reference,'') FROM fleet_partner_audit_records WHERE organization_id=$1 AND action='role.created' AND resource_id=$2`, orgID, roleID).Scan(&cited); err != nil {
		t.Fatalf("role.created audit row: %v", err)
	}
	if cited != grant {
		t.Fatalf("audit step_up_reference = %q, want the verified grant %q", cited, grant)
	}

	// Within its lifetime the grant covers the same action class again.
	code, body = stepUpCall(t, router, owner, grant, "PATCH", "/roles/"+roleID, map[string]any{"name": "Step-up lead 2", "permissions": []string{"staff.read"}}, map[string]string{"If-Match": "1"})
	if code != http.StatusOK {
		t.Fatalf("update role with step-up = %d %v", code, body)
	}

	// A second session of the same person cannot borrow the first session's grant.
	second := signInExisting(t, router, h, "stepup-iam@example.com")
	code, body = stepUpCall(t, router, second, grant, "POST", "/roles/"+roleID+"/archive", map[string]any{}, nil)
	requireStepUpRefusal(t, "archive role with another session's grant", code, body, "step_up_required", stepup.ActionAccessManage)

	// Another organization's owner cannot use it either.
	other, _ := createStepUpOrganization(t, router, h, "stepup-iam-other@example.com", "+2348012347002", "RC-STEPUP-IAM-2")
	code, body = stepUpCall(t, router, other, grant, "POST", "/roles", role, nil)
	requireStepUpRefusal(t, "create role with another identity's grant", code, body, "step_up_required", stepup.ActionAccessManage)

	// Once the grant ages past the step-up lifetime it is expired.
	if _, err := h.pool.Exec(context.Background(), `UPDATE fleet_partner_auth_otp_challenges SET verified_at = now() - interval '10 minutes' WHERE id=$1`, grant); err != nil {
		t.Fatal(err)
	}
	code, body = stepUpCall(t, router, owner, grant, "POST", "/roles/"+roleID+"/archive", map[string]any{}, nil)
	requireStepUpRefusal(t, "archive role with expired grant", code, body, "step_up_expired", stepup.ActionAccessManage)
	var archived string
	if err := h.pool.QueryRow(context.Background(), `SELECT status FROM fleet_partner_roles WHERE id=$1`, roleID).Scan(&archived); err != nil {
		t.Fatal(err)
	}
	if archived == "ARCHIVED" {
		t.Fatal("role archived without a valid step-up")
	}

	// Staff role bindings, suspension and reactivation are covered by the
	// same class; refused without, accepted with a fresh grant.
	member := seedMember(t, h.pool, orgID, seedIdentity(t, h.pool, "stepup-iam-staff@example.com", "+2348012347003"), false, nil)
	bindings := map[string]any{"bindings": []map[string]any{{"roleId": roleID, "scopeType": "ORGANIZATION"}}}
	code, body = stepUpCall(t, router, owner, "", "PUT", "/staff/"+member.ID+"/roles", bindings, nil)
	requireStepUpRefusal(t, "replace staff roles without step-up", code, body, "step_up_required", stepup.ActionAccessManage)
	code, body = stepUpCall(t, router, owner, "", "POST", "/staff/"+member.ID+"/suspend", map[string]any{"reason": "test"}, nil)
	requireStepUpRefusal(t, "suspend staff without step-up", code, body, "step_up_required", stepup.ActionAccessManage)

	if _, err := h.pool.Exec(context.Background(), `UPDATE fleet_partner_auth_otp_challenges SET resend_available_at=now()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	fresh := obtainStepUp(t, router, h, owner, stepup.ActionAccessManage)
	if code, body = stepUpCall(t, router, owner, fresh, "PUT", "/staff/"+member.ID+"/roles", bindings, nil); code != http.StatusOK {
		t.Fatalf("replace staff roles with step-up = %d %v", code, body)
	}
	memberVersion := func() map[string]string {
		t.Helper()
		var version int64
		if err := h.pool.QueryRow(context.Background(), `SELECT version FROM fleet_partner_organization_users WHERE id=$1`, member.ID).Scan(&version); err != nil {
			t.Fatal(err)
		}
		return map[string]string{"If-Match": fmt.Sprint(version)}
	}
	if code, body = stepUpCall(t, router, owner, fresh, "POST", "/staff/"+member.ID+"/suspend", map[string]any{"reason": "test"}, memberVersion()); code != http.StatusOK {
		t.Fatalf("suspend staff with step-up = %d %v", code, body)
	}
	code, body = stepUpCall(t, router, owner, "", "POST", "/staff/"+member.ID+"/reactivate", map[string]any{}, memberVersion())
	requireStepUpRefusal(t, "reactivate staff without step-up", code, body, "step_up_required", stepup.ActionAccessManage)
	if code, body = stepUpCall(t, router, owner, fresh, "POST", "/staff/"+member.ID+"/reactivate", map[string]any{}, memberVersion()); code != http.StatusOK {
		t.Fatalf("reactivate staff with step-up = %d %v", code, body)
	}
	var withEvidence int
	if err := h.pool.QueryRow(context.Background(), `SELECT count(*) FROM fleet_partner_audit_records WHERE organization_id=$1 AND resource_id=$2 AND action IN ('staff.roles_replaced','staff.status_changed') AND step_up_reference=$3`, orgID, member.ID, fresh).Scan(&withEvidence); err != nil {
		t.Fatal(err)
	}
	if withEvidence < 3 {
		t.Fatalf("staff audit rows citing the grant = %d, want 3", withEvidence)
	}

	// A caller in another organization who completes step-up for their own
	// organization still gets 404 for this organization's staff member.
	otherGrant := obtainStepUp(t, router, h, other, stepup.ActionAccessManage)
	if code, _ := stepUpCall(t, router, other, otherGrant, "PUT", "/staff/"+member.ID+"/roles", map[string]any{"roleIds": []string{"rol_system_operations"}}, nil); code != http.StatusNotFound {
		t.Fatalf("cross-tenant staff roles with own step-up = %d, want 404", code)
	}
}

// TestStepUp_OrganizationClosureAndProviderRemoval: closure request, approval
// and execution each refuse without a closure grant and run with one; provider
// removal refuses without its own grant and reaches the handler with one.
func TestStepUp_OrganizationClosureAndProviderRemoval(t *testing.T) {
	router, h := buildOnboardingRouter(t)
	owner, orgID := createStepUpOrganization(t, router, h, "stepup-close@example.com", "+2348012347011", "RC-STEPUP-CLOSE")
	activateOrganization(t, h, orgID)
	base := "/organizations/" + orgID

	// Provider removal.
	code, body := stepUpCall(t, router, owner, "", "POST", "/providers/pm_unknown/remove", map[string]any{"reason": "left"}, nil)
	requireStepUpRefusal(t, "remove provider without step-up", code, body, "step_up_required", stepup.ActionProviderRemove)
	removeGrant := obtainStepUp(t, router, h, owner, stepup.ActionProviderRemove)
	code, body = stepUpCall(t, router, owner, removeGrant, "POST", "/providers/pm_unknown/remove", map[string]any{"reason": "left"}, nil)
	if code != http.StatusNotFound {
		t.Fatalf("remove unknown provider with step-up = %d %v, want 404 from the handler", code, body)
	}

	code, body = stepUpCall(t, router, owner, "", "POST", base+"/close-request", map[string]any{"reasonCode": "test_wind_down"}, nil)
	requireStepUpRefusal(t, "close-request without step-up", code, body, "step_up_required", stepup.ActionOrganizationClosure)
	code, body = stepUpCall(t, router, owner, removeGrant, "POST", base+"/close-request", map[string]any{"reasonCode": "test_wind_down"}, nil)
	requireStepUpRefusal(t, "close-request with a provider-removal grant", code, body, "step_up_required", stepup.ActionOrganizationClosure)

	grant := obtainStepUp(t, router, h, owner, stepup.ActionOrganizationClosure)
	if code, body = stepUpCall(t, router, owner, grant, "POST", base+"/close-request", map[string]any{"reasonCode": "test_wind_down"}, nil); code != http.StatusCreated {
		t.Fatalf("close-request with step-up = %d %v", code, body)
	}
	// Assessment is not a closure decision and needs no step-up.
	if code, body = stepUpCall(t, router, owner, "", "POST", base+"/closure/assess", map[string]any{}, nil); code != http.StatusOK {
		t.Fatalf("assess = %d %v", code, body)
	}
	organizations := orgusecases.NewService(orgusecases.Options{
		Pool: h.pool, Organizations: orgrepos.NewOrganizationRepository(h.pool), Onboarding: orgrepos.NewOnboardingRepository(h.pool),
		Branches: orgrepos.NewBranchRepository(h.pool), Settings: orgrepos.NewSettingsRepository(h.pool),
		Events: events.NewDomainPublisher(events.NewOutboxRepository(h.pool)),
	})
	if _, blockers, err := organizations.AssessClosure(context.Background(), orgID, nil, nil); err != nil || len(blockers) != 0 {
		t.Fatalf("re-assess with no checkers: %v %v", blockers, err)
	}

	code, body = stepUpCall(t, router, owner, "", "POST", base+"/closure/approve", map[string]any{}, nil)
	requireStepUpRefusal(t, "approve closure without step-up", code, body, "step_up_required", stepup.ActionOrganizationClosure)
	if code, body = stepUpCall(t, router, owner, grant, "POST", base+"/closure/approve", map[string]any{}, nil); code != http.StatusOK {
		t.Fatalf("approve closure with step-up = %d %v", code, body)
	}
	code, body = stepUpCall(t, router, owner, "", "POST", base+"/closure/execute", map[string]any{}, nil)
	requireStepUpRefusal(t, "execute closure without step-up", code, body, "step_up_required", stepup.ActionOrganizationClosure)
	if code, body = stepUpCall(t, router, owner, grant, "POST", base+"/closure/execute", map[string]any{}, nil); code != http.StatusOK {
		t.Fatalf("execute closure with step-up = %d %v", code, body)
	}
	var status string
	if err := h.pool.QueryRow(context.Background(), `SELECT status FROM fleet_partner_organization_closure_requests WHERE organization_id=$1 ORDER BY created_at DESC LIMIT 1`, orgID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != orgmodels.ClosureStatusCompleted {
		t.Fatalf("closure status = %q, want COMPLETED", status)
	}
}

// stepUpTestExporter serves a sensitive dataset with fixed columns so the
// export path can run end to end.
type stepUpTestExporter struct{}

func (stepUpTestExporter) Type() string      { return exportsmodels.TypeProvidersWorkforce }
func (stepUpTestExporter) Columns() []string { return []string{"id"} }
func (stepUpTestExporter) Rows(context.Context, tenant.Context, map[string]any, int) ([][]string, error) {
	return [][]string{{"wp_1"}}, nil
}

type stepUpOrdersExporter struct{}

func (stepUpOrdersExporter) Type() string      { return exportsmodels.TypeOrders }
func (stepUpOrdersExporter) Columns() []string { return []string{"id"} }
func (stepUpOrdersExporter) Rows(context.Context, tenant.Context, map[string]any, int) ([][]string, error) {
	return [][]string{{"ord_1"}}, nil
}

// TestStepUp_FinancialActionsAndSensitiveExports mounts the real wallet,
// payroll and export handlers on the production tenant group. Wallet
// allocations/withdrawals and payroll approve/post refuse without their grant
// and reach the handler with it. A sensitive export category refuses create,
// replay and download without an exports.sensitive grant; an operational
// category needs none.
func TestStepUp_FinancialActionsAndSensitiveExports(t *testing.T) {
	router, h := buildOnboardingRouterWith(t, func(pool *pgxpool.Pool, g *gin.RouterGroup) {
		authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0)
		exportService := exportsusecases.NewService(exportsusecases.Options{
			Pool: pool, Repository: exportsrepos.NewRepository(pool),
			Exporters: map[string]exportsusecases.Exporter{exportsmodels.TypeProvidersWorkforce: stepUpTestExporter{}, exportsmodels.TypeOrders: stepUpOrdersExporter{}},
			Media:     newFakeExportMedia(), Metrics: observability.NewMetrics(),
			Flags: exportsusecases.Flags{Enabled: true, WritesEnabled: true}, MaxRows: 10, RetentionTTL: time.Hour, Now: time.Now,
		})
		companywallethttp.NewHandler(companywalletusecases.NewService(companywalletusecases.Options{Pool: pool}), authorizer).RegisterTenantRoutes(g)
		financeops.NewHandler(financeops.NewService(financeops.Options{Pool: pool, Flags: financeops.Flags{Payroll: true}}), authorizer, idempotency.NewStore(pool, 0)).RegisterTenantRoutes(g)
		exportshttp.NewHandler(exportService, authorizer, idempotency.NewStore(pool, 0)).RegisterTenantRoutes(g)
	})
	pool := h.pool
	owner, orgID := createStepUpOrganization(t, router, h, "stepup-money@example.com", "+2348012347021", "RC-STEPUP-MONEY")
	activateOrganization(t, h, orgID)

	gates := []struct {
		label, method, path, actionClass string
	}{
		{"create allocation", "POST", "/providers/pm_unknown/sub-wallet/allocations", stepup.ActionWalletAllocation},
		{"approve allocation", "POST", "/providers/pm_unknown/sub-wallet/allocations/alc_unknown/approve", stepup.ActionWalletAllocation},
		{"create withdrawal", "POST", "/providers/pm_unknown/sub-wallet/withdrawals", stepup.ActionWalletWithdrawal},
		{"approve withdrawal", "POST", "/providers/pm_unknown/sub-wallet/withdrawals/wdr_unknown/approve", stepup.ActionWalletWithdrawal},
		{"approve payroll", "POST", "/payroll-periods/pp_unknown/approve", stepup.ActionPayrollFinalize},
		{"post payroll", "POST", "/payroll-periods/pp_unknown/post", stepup.ActionPayrollFinalize},
	}
	grants := map[string]string{}
	for _, gate := range gates {
		code, body := stepUpCall(t, router, owner, "", gate.method, gate.path, map[string]any{"amountMinor": 1000, "currency": "NGN", "expectedVersion": 1}, nil)
		requireStepUpRefusal(t, gate.label+" without step-up", code, body, "step_up_required", gate.actionClass)
		if grants[gate.actionClass] == "" {
			grants[gate.actionClass] = obtainStepUp(t, router, h, owner, gate.actionClass)
		}
		code, body = stepUpCall(t, router, owner, grants[gate.actionClass], gate.method, gate.path, map[string]any{"amountMinor": 1000, "currency": "NGN", "expectedVersion": 1}, nil)
		if envelopeErrorCode(body) == "step_up_required" || envelopeErrorCode(body) == "step_up_expired" {
			t.Fatalf("%s with step-up still refused by step-up: %d %v", gate.label, code, body)
		}
		if code < 400 {
			t.Fatalf("%s against unknown ids unexpectedly succeeded: %d %v", gate.label, code, body)
		}
	}
	// A wallet allocation grant does not cover withdrawals.
	code, body := stepUpCall(t, router, owner, grants[stepup.ActionWalletAllocation], "POST", "/providers/pm_unknown/sub-wallet/withdrawals", map[string]any{}, nil)
	requireStepUpRefusal(t, "withdrawal with an allocation grant", code, body, "step_up_required", stepup.ActionWalletWithdrawal)

	// Operational export: no step-up.
	code, body = stepUpCall(t, router, owner, "", "POST", "/exports", map[string]any{"exportType": exportsmodels.TypeOrders}, map[string]string{"Idempotency-Key": "stepup-orders-1"})
	if code != http.StatusAccepted {
		t.Fatalf("operational export without step-up = %d %v", code, body)
	}

	// Sensitive export: refused without, accepted with.
	sensitive := map[string]any{"exportType": exportsmodels.TypeProvidersWorkforce}
	code, body = stepUpCall(t, router, owner, "", "POST", "/exports", sensitive, map[string]string{"Idempotency-Key": "stepup-wf-1"})
	requireStepUpRefusal(t, "sensitive export without step-up", code, body, "step_up_required", stepup.ActionSensitiveExport)
	exportGrant := obtainStepUp(t, router, h, owner, stepup.ActionSensitiveExport)
	code, body = stepUpCall(t, router, owner, exportGrant, "POST", "/exports", sensitive, map[string]string{"Idempotency-Key": "stepup-wf-2"})
	if code != http.StatusAccepted {
		t.Fatalf("sensitive export with step-up = %d %v", code, body)
	}
	exportID := documentData(t, body)["id"].(string)

	code, body = stepUpCall(t, router, owner, "", "POST", "/exports/"+exportID+"/download-url", map[string]any{}, nil)
	requireStepUpRefusal(t, "sensitive download without step-up", code, body, "step_up_required", stepup.ActionSensitiveExport)
	code, body = stepUpCall(t, router, owner, exportGrant, "POST", "/exports/"+exportID+"/download-url", map[string]any{}, nil)
	if envelopeErrorCode(body) == "step_up_required" || code == http.StatusOK {
		t.Fatalf("download of a PENDING sensitive export with step-up = %d %v, want the not-ready refusal", code, body)
	}

	if _, err := pool.Exec(context.Background(), `UPDATE fleet_partner_export_jobs SET status='DEAD_LETTER' WHERE id=$1`, exportID); err != nil {
		t.Fatal(err)
	}
	code, body = stepUpCall(t, router, owner, "", "POST", "/exports/"+exportID+"/replay", map[string]any{}, map[string]string{"Idempotency-Key": "stepup-replay-1"})
	requireStepUpRefusal(t, "sensitive replay without step-up", code, body, "step_up_required", stepup.ActionSensitiveExport)
	var status string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM fleet_partner_export_jobs WHERE id=$1`, exportID).Scan(&status); err != nil || status != exportsmodels.StatusDeadLetter {
		t.Fatalf("refused replay changed the job: %q %v", status, err)
	}
	code, body = stepUpCall(t, router, owner, exportGrant, "POST", "/exports/"+exportID+"/replay", map[string]any{}, map[string]string{"Idempotency-Key": "stepup-replay-2"})
	if code >= 400 {
		t.Fatalf("sensitive replay with step-up = %d %v", code, body)
	}
}

// TestStepUp_LegalHoldWithoutVerifierFailsClosed: legal-hold create used to
// succeed whenever no step-up verifier was wired, even though the
// organization's policy required step-up. It now refuses and writes nothing.
func TestStepUp_LegalHoldWithoutVerifierFailsClosed(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Hold Fail Closed Co", "RC-HOLD-FAILCLOSED")
	svc := legalholdusecases.NewService(pool, nil, nil)
	_, err := svc.Create(ctx, legalholdusecases.CreateInput{
		Actor:     legalholdusecases.Actor{OrganizationID: org.ID, ActorType: "organization_user", ActorID: "ou_1"},
		ScopeType: legalholdmodels.ScopeTrip, ScopeReference: "trp_failclosed", Reason: "litigation hold",
		StepUpReference: "chl_anything",
	})
	if apperrors.From(err).Status != http.StatusServiceUnavailable {
		t.Fatalf("create hold with no verifier: err = %v, want 503 unavailable", err)
	}
	if under, err := svc.IsUnderHold(ctx, org.ID, legalholdmodels.ScopeTrip, "trp_failclosed"); err != nil || under {
		t.Fatalf("refused create still placed a hold: %v %v", under, err)
	}
}

type mfaNotRequired struct{}

func (mfaNotRequired) MFARequiredForSensitiveActions(context.Context, string) (bool, error) {
	return false, nil
}

// TestStepUp_APISecretChangesIgnoreTheOrganizationSetting: spec 64.1 makes
// step-up mandatory for API secret rotation, so an organization that turned
// mfaRequiredForSensitiveActions off still cannot create or rotate a secret
// without a grant.
func TestStepUp_APISecretChangesIgnoreTheOrganizationSetting(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	org := createOrganization(t, pool, "Secret StepUp Co", "RC-SECRET-STEPUP")
	verifier := &fakeStepUpVerifier{validReference: "step_ok"}
	svc := integrations.NewService(integrations.Options{
		Pool: pool,
		Config: config.Config{
			AppEnv: "test", OrganizationAPIClientsEnabled: true,
			APIClientHashKey: []byte("0123456789abcdef0123456789abcdef"),
		},
		StepUp: verifier, SecurityPolicy: mfaNotRequired{},
	})
	actor := integrations.Actor{Tenant: tenant.Context{OrganizationID: org.ID, OrganizationUserID: "ou_secret", ActorType: tenant.ActorTypeOrganizationUser, ActorID: "ou_secret"}, IdentityID: "idn_secret", SessionID: "ses_secret"}
	input := integrations.CreateAPIClientInput{Name: "Connector", Scopes: []string{integrations.ScopeIntegrationContextRead}, IPAllowlist: []string{"203.0.113.0/24"}, IdempotencyKey: "idem-secret-stepup"}
	if _, _, _, err := svc.CreateAPIClient(ctx, actor, input); err == nil {
		t.Fatal("API client created without step-up while the organization setting was off")
	}
	actor.StepUpReference = "step_ok"
	if _, secret, _, err := svc.CreateAPIClient(ctx, actor, input); err != nil || secret == "" {
		t.Fatalf("API client with step-up: secret=%t err=%v", secret != "", err)
	}
	if len(verifier.calls) == 0 || verifier.calls[len(verifier.calls)-1].organizationID != org.ID {
		t.Fatalf("verifier calls = %+v", verifier.calls)
	}
}

// onboardCallStepUp is onboardCall plus a step-up reference, for tests that
// drive step-up-protected routes through the production router.
func onboardCallStepUp(t *testing.T, router http.Handler, token, reference, method, path string, payload any, version int) (int, map[string]any) {
	t.Helper()
	headers := map[string]string{"Authorization": "Bearer " + token, "Origin": stepUpOrigin, "Idempotency-Key": "onboarding-test", stepup.HeaderReference: reference}
	if version > 0 {
		headers["If-Match"] = fmt.Sprint(version)
	}
	return documentCall(t, router, method, "/fleet-partner/v1"+path, payload, headers)
}
