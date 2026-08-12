package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	fleetevents "cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	approvalhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/http"
	approvalrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/repositories"
	approvalusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/usecases"
	eligibilityhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/http"
	eligibilitymodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// WP9 corporate routes, driven through the real handlers, the real authorizer
// and real PostgreSQL.
//
// The tenant context is injected rather than produced by a login, because what
// is under test is what happens AFTER identity is established: default deny,
// branch scope, and whether an identifier from elsewhere becomes a way in.

type gateHTTPFixture struct {
	pool       *pgxpool.Pool
	orgID      string
	authority  *stubAuthority
	approvals  *approvalusecases.Service
	membership string
}

func newGateHTTPFixture(t *testing.T, name string, registration string) *gateHTTPFixture {
	t.Helper()
	pool := testPool(t)
	org := createOrganization(t, pool, name, registration)
	membership := seedProviderMembership(t, pool, org.ID, "drv-"+registration)
	authority := healthyAuthority(*membership.ProviderID, "veh-"+registration)

	approvals := approvalusecases.NewService(approvalusecases.Options{
		Pool:        pool,
		Approvals:   approvalrepos.NewRepository(pool),
		Eligibility: newEligibilityService(pool, authority),
		Roles:       staffrepos.NewRoleRepository(pool),
		Subjects:    approvalrepos.NewSubjectVersions(pool),
		Events:      fleetevents.NewDomainPublisher(fleetevents.NewOutboxRepository(pool)),
	})
	return &gateHTTPFixture{
		pool: pool, orgID: org.ID, authority: authority,
		approvals: approvals, membership: membership.ID,
	}
}

// router mounts the real eligibility and approval routes for one member.
func (f *gateHTTPFixture) router(t *testing.T, organizationID string, memberID string,
	branchIDs []string) http.Handler {
	t.Helper()
	gin.SetMode(gin.TestMode)
	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(f.pool), nil, 0)

	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(middleware.TenantContextKey, tenant.Context{
			OrganizationID: organizationID, OrganizationUserID: memberID,
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: memberID,
			PermittedBranchIDs: branchIDs})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	group := engine.Group("/fleet-partner/v1")
	eligibilityhttp.NewHandler(newEligibilityService(f.pool, f.authority),
		authorizer).RegisterTenantRoutes(group)
	approvalhttp.NewHandler(f.approvals, authorizer).RegisterTenantRoutes(group)
	return engine
}

// gateCall performs one request and returns the status and decoded body.
func gateCall(t *testing.T, router http.Handler, method string, path string,
	body any, headers map[string]string) (int, map[string]any) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	request := httptest.NewRequest(method, path, reader)
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	response := map[string]any{}
	_ = json.Unmarshal(recorder.Body.Bytes(), &response)
	return recorder.Code, response
}

// TestEligibilityRoutesDenyByDefault proves a member of the organization with
// no relevant role reaches nothing. Membership is not authority.
func TestEligibilityRoutesDenyByDefault(t *testing.T) {
	f := newGateHTTPFixture(t, "Route Deny Co", "RC-HTTP-1")
	nobody := seedApprovalMember(t, f.pool, f.orgID, "", "nobody")
	router := f.router(t, f.orgID, nobody, nil)

	for _, route := range []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodGet, "/fleet-partner/v1/eligibility/decisions", nil},
		{http.MethodGet, "/fleet-partner/v1/eligibility/policies", nil},
		{http.MethodPost, "/fleet-partner/v1/eligibility/evaluations",
			map[string]any{"subjectType": "COMPANY", "subjectId": f.orgID,
				"operationType": eligibilitymodels.OperationCompanyActivation}},
		{http.MethodPost, "/fleet-partner/v1/eligibility/policies",
			map[string]any{"policyKey": "PAID_FEATURE"}},
		{http.MethodGet, "/fleet-partner/v1/approvals/requests", nil},
		{http.MethodPost, "/fleet-partner/v1/approvals/requests", map[string]any{}},
	} {
		status, _ := gateCall(t, router, route.method, route.path, route.body,
			map[string]string{"Idempotency-Key": "deny-key"})
		if status != http.StatusForbidden {
			t.Fatalf("%s %s status=%d, want 403", route.method, route.path, status)
		}
	}
}

// TestEligibilityEvaluationRouteRecordsADecision proves the route produces the
// same durable decision the service does, with reasons and sources visible.
func TestEligibilityEvaluationRouteRecordsADecision(t *testing.T) {
	f := newGateHTTPFixture(t, "Route Evaluate Co", "RC-HTTP-2")
	operator := seedApprovalMember(t, f.pool, f.orgID, "rol_system_operations", "operator")
	router := f.router(t, f.orgID, operator, nil)

	status, response := gateCall(t, router, http.MethodPost,
		"/fleet-partner/v1/eligibility/evaluations", map[string]any{
			"subjectType":   "PROVIDER",
			"subjectId":     f.membership,
			"operationType": eligibilitymodels.OperationAssignment,
			"refresh":       true,
		}, nil)
	if status != http.StatusOK {
		t.Fatalf("evaluate status=%d body=%#v", status, response)
	}
	data, _ := response["data"].(map[string]any)
	if data["status"] != eligibilitymodels.StatusEligible {
		t.Fatalf("status = %v, want ELIGIBLE (%#v)", data["status"], data)
	}
	if data["authorizes"] != true {
		t.Fatalf("authorizes = %v; the response must state this rather than "+
			"making the client derive it", data["authorizes"])
	}
	// The response names the authorities consulted; it never echoes their
	// payloads.
	sources, _ := data["sources"].([]any)
	if len(sources) == 0 {
		t.Fatal("the decision view must name the sources it relied on")
	}

	// The decision is then readable through the read route.
	status, listed := gateCall(t, router, http.MethodGet,
		"/fleet-partner/v1/eligibility/decisions?currentOnly=true", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("list status=%d", status)
	}
	payload, _ := listed["data"].(map[string]any)
	decisions, _ := payload["decisions"].([]any)
	if len(decisions) == 0 {
		t.Fatal("the recorded decision must be readable")
	}
}

// TestEligibilityBranchScopeIsEnforced proves a branch-scoped operator cannot
// act on another branch, and that the refusal does not confirm the branch
// exists.
func TestEligibilityBranchScopeIsEnforced(t *testing.T) {
	f := newGateHTTPFixture(t, "Route Branch Co", "RC-HTTP-3")
	ctx := context.Background()

	var permitted, forbidden string
	for _, branch := range []struct {
		code  string
		into  *string
		label string
	}{{"BR1", &permitted, "Permitted"}, {"BR2", &forbidden, "Forbidden"}} {
		id := "brn_" + branch.code + f.orgID[len(f.orgID)-6:]
		if _, err := f.pool.Exec(ctx, `
			INSERT INTO fleet_partner_branches
				(id, organization_id, code, name, type, address, timezone, status)
			VALUES ($1, $2, $3, $4, 'HUB', 'Lagos', 'Africa/Lagos', 'ACTIVE')`,
			id, f.orgID, branch.code, branch.label); err != nil {
			t.Fatalf("seed branch: %v", err)
		}
		*branch.into = id
	}

	operator := seedApprovalMember(t, f.pool, f.orgID, "rol_system_operations", "branchop")
	router := f.router(t, f.orgID, operator, []string{permitted})

	status, _ := gateCall(t, router, http.MethodPost,
		"/fleet-partner/v1/eligibility/evaluations", map[string]any{
			"subjectType": "COMPANY", "subjectId": f.orgID,
			"operationType": eligibilitymodels.OperationCompanyActivation,
			"branchId":      permitted, "refresh": true,
		}, nil)
	if status != http.StatusOK {
		t.Fatalf("permitted branch status=%d, want 200", status)
	}

	status, _ = gateCall(t, router, http.MethodPost,
		"/fleet-partner/v1/eligibility/evaluations", map[string]any{
			"subjectType": "COMPANY", "subjectId": f.orgID,
			"operationType": eligibilitymodels.OperationCompanyActivation,
			"branchId":      forbidden, "refresh": true,
		}, nil)
	if status != http.StatusNotFound {
		t.Fatalf("out-of-scope branch status=%d, want 404", status)
	}
}

// TestEligibilityDecisionIDOR proves an identifier belonging to another
// organization answers as missing, not as forbidden.
func TestEligibilityDecisionIDOR(t *testing.T) {
	f := newGateHTTPFixture(t, "Route IDOR A", "RC-HTTP-4")
	victimOperator := seedApprovalMember(t, f.pool, f.orgID, "rol_system_operations", "victimop")
	victimRouter := f.router(t, f.orgID, victimOperator, nil)

	status, response := gateCall(t, victimRouter, http.MethodPost,
		"/fleet-partner/v1/eligibility/evaluations", map[string]any{
			"subjectType": "COMPANY", "subjectId": f.orgID,
			"operationType": eligibilitymodels.OperationCompanyActivation, "refresh": true,
		}, nil)
	if status != http.StatusOK {
		t.Fatalf("seed decision status=%d", status)
	}
	data, _ := response["data"].(map[string]any)
	decisionID, _ := data["id"].(string)

	other := createOrganization(t, f.pool, "Route IDOR B", "RC-HTTP-5")
	seedEligibleOrganization(t, f.pool, other.ID)
	intruder := seedApprovalMember(t, f.pool, other.ID, "rol_system_owner", "intruder")
	intruderRouter := f.router(t, other.ID, intruder, nil)

	for _, path := range []string{
		"/fleet-partner/v1/eligibility/decisions/" + decisionID,
		"/fleet-partner/v1/eligibility/decisions/" + decisionID + "/history",
	} {
		status, _ := gateCall(t, intruderRouter, http.MethodGet, path, nil, nil)
		if status != http.StatusNotFound {
			t.Fatalf("GET %s status=%d, want 404", path, status)
		}
	}
}

// TestApprovalRoutesEndToEnd drives request, read, approve and execute through
// the real routes, and proves the separate permissions actually separate.
func TestApprovalRoutesEndToEnd(t *testing.T) {
	f := newGateHTTPFixture(t, "Route Approval Co", "RC-HTTP-6")
	requester := seedApprovalMember(t, f.pool, f.orgID, "rol_system_operations", "reqop")
	requesterRouter := f.router(t, f.orgID, requester, nil)

	payload := map[string]any{"reason": "ready"}
	status, response := gateCall(t, requesterRouter, http.MethodPost,
		"/fleet-partner/v1/approvals/requests", map[string]any{
			"resourceType":  approvalrepos.ResourceOrganization,
			"resourceId":    f.orgID,
			"action":        "ACTIVATE",
			"operationType": eligibilitymodels.OperationCompanyActivation,
			"payload":       payload,
		}, map[string]string{"Idempotency-Key": "http-approval-1"})
	if status != http.StatusCreated {
		t.Fatalf("request status=%d body=%#v", status, response)
	}
	data, _ := response["data"].(map[string]any)
	approvalID, _ := data["id"].(string)
	if data["requestFingerprint"] == "" {
		t.Fatal("the response must carry the fingerprint execution will be checked against")
	}
	// The payload snapshot must not be echoed to every holder of the read
	// permission.
	if _, leaked := data["payloadSnapshot"]; leaked {
		t.Fatal("the approval view must not republish the payload snapshot")
	}

	// Operations may request and execute but not decide.
	status, _ = gateCall(t, requesterRouter, http.MethodPost,
		"/fleet-partner/v1/approvals/requests/"+approvalID+"/approve", map[string]any{}, nil)
	if status != http.StatusForbidden {
		t.Fatalf("operations approving status=%d, want 403", status)
	}

	// Compliance may decide but may not request.
	compliance := seedApprovalMember(t, f.pool, f.orgID, "rol_system_compliance", "complianceop")
	complianceRouter := f.router(t, f.orgID, compliance, nil)
	status, _ = gateCall(t, complianceRouter, http.MethodPost,
		"/fleet-partner/v1/approvals/requests", map[string]any{
			"resourceType": "ORGANIZATION", "resourceId": f.orgID, "action": "ACTIVATE",
			"operationType": eligibilitymodels.OperationCompanyActivation,
		}, map[string]string{"Idempotency-Key": "http-approval-2"})
	if status != http.StatusForbidden {
		t.Fatalf("compliance requesting status=%d, want 403", status)
	}

	// An owner approves, then operations executes with the exact payload.
	owner := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "ownerop")
	ownerRouter := f.router(t, f.orgID, owner, nil)
	status, decided := gateCall(t, ownerRouter, http.MethodPost,
		"/fleet-partner/v1/approvals/requests/"+approvalID+"/approve",
		map[string]any{"reason": "looks right"}, nil)
	if status != http.StatusOK {
		t.Fatalf("approve status=%d body=%#v", status, decided)
	}

	status, executed := gateCall(t, requesterRouter, http.MethodPost,
		"/fleet-partner/v1/approvals/requests/"+approvalID+"/execute",
		map[string]any{"payload": payload, "executionReference": "http-exec-1"},
		map[string]string{"Idempotency-Key": "http-exec-idem-1"})
	if status != http.StatusOK {
		t.Fatalf("execute status=%d body=%#v", status, executed)
	}
	executedData, _ := executed["data"].(map[string]any)
	if executedData["status"] != "EXECUTED" {
		t.Fatalf("status = %v, want EXECUTED", executedData["status"])
	}

	// The immutable decisions are readable.
	status, decisions := gateCall(t, ownerRouter, http.MethodGet,
		"/fleet-partner/v1/approvals/requests/"+approvalID+"/decisions", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("decisions status=%d", status)
	}
	decisionPayload, _ := decisions["data"].(map[string]any)
	recorded, _ := decisionPayload["decisions"].([]any)
	if len(recorded) != 1 {
		t.Fatalf("decisions = %d, want 1", len(recorded))
	}
}

// TestApprovalExecuteRouteRefusesAChangedPayload proves the exact-payload rule
// is enforced at the route, not only in the service.
func TestApprovalExecuteRouteRefusesAChangedPayload(t *testing.T) {
	f := newGateHTTPFixture(t, "Route Payload Co", "RC-HTTP-7")
	requester := seedApprovalMember(t, f.pool, f.orgID, "rol_system_operations", "payloadreq")
	owner := seedApprovalMember(t, f.pool, f.orgID, "rol_system_owner", "payloadowner")
	requesterRouter := f.router(t, f.orgID, requester, nil)
	ownerRouter := f.router(t, f.orgID, owner, nil)

	status, response := gateCall(t, requesterRouter, http.MethodPost,
		"/fleet-partner/v1/approvals/requests", map[string]any{
			"resourceType":  approvalrepos.ResourceOrganization,
			"resourceId":    f.orgID,
			"action":        "ACTIVATE",
			"operationType": eligibilitymodels.OperationCompanyActivation,
			"payload":       map[string]any{"amountMinor": 1000},
		}, map[string]string{"Idempotency-Key": "http-payload-1"})
	if status != http.StatusCreated {
		t.Fatalf("request status=%d body=%#v", status, response)
	}
	data, _ := response["data"].(map[string]any)
	approvalID, _ := data["id"].(string)

	if status, _ := gateCall(t, ownerRouter, http.MethodPost,
		"/fleet-partner/v1/approvals/requests/"+approvalID+"/approve",
		map[string]any{}, nil); status != http.StatusOK {
		t.Fatalf("approve status=%d", status)
	}

	status, _ = gateCall(t, requesterRouter, http.MethodPost,
		"/fleet-partner/v1/approvals/requests/"+approvalID+"/execute",
		map[string]any{"payload": map[string]any{"amountMinor": 999999},
			"executionReference": "http-exec-changed"},
		map[string]string{"Idempotency-Key": "http-exec-changed"})
	if status != http.StatusConflict {
		t.Fatalf("changed payload status=%d, want 409", status)
	}
}

// TestApprovalRoutesRequireAnIdempotencyKey proves the write routes refuse an
// unkeyed request rather than creating a duplicate on every retry.
func TestApprovalRoutesRequireAnIdempotencyKey(t *testing.T) {
	f := newGateHTTPFixture(t, "Route Idem Co", "RC-HTTP-8")
	requester := seedApprovalMember(t, f.pool, f.orgID, "rol_system_operations", "idemreq")
	router := f.router(t, f.orgID, requester, nil)

	status, _ := gateCall(t, router, http.MethodPost,
		"/fleet-partner/v1/approvals/requests", map[string]any{
			"resourceType": approvalrepos.ResourceOrganization, "resourceId": f.orgID,
			"action": "ACTIVATE", "operationType": eligibilitymodels.OperationCompanyActivation,
		}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("unkeyed request status=%d, want 400", status)
	}
}

// TestPolicyPublicationRouteRequiresItsOwnPermission proves rewriting the rule
// that decides eligibility is a separate power from reading the answers.
func TestPolicyPublicationRouteRequiresItsOwnPermission(t *testing.T) {
	f := newGateHTTPFixture(t, "Route Policy Co", "RC-HTTP-9")
	dispatcher := seedApprovalMember(t, f.pool, f.orgID, "rol_system_dispatcher", "policydispatcher")
	admin := seedApprovalMember(t, f.pool, f.orgID, "rol_system_admin", "policyadmin")

	body := map[string]any{
		"policyKey":             eligibilitymodels.OperationPaidFeature,
		"requiredCompanyChecks": []string{eligibilitymodels.CheckLifecycle},
		"decisionTtlSeconds":    600,
	}

	// A dispatcher can read decisions but must not be able to change the rule
	// that produces them.
	status, _ := gateCall(t, f.router(t, f.orgID, dispatcher, nil), http.MethodPost,
		"/fleet-partner/v1/eligibility/policies", body, nil)
	if status != http.StatusForbidden {
		t.Fatalf("dispatcher publishing status=%d, want 403", status)
	}

	status, response := gateCall(t, f.router(t, f.orgID, admin, nil), http.MethodPost,
		"/fleet-partner/v1/eligibility/policies", body, nil)
	if status != http.StatusCreated {
		t.Fatalf("admin publishing status=%d body=%#v", status, response)
	}
	data, _ := response["data"].(map[string]any)
	if data["scope"] != "ORGANIZATION" {
		t.Fatalf("scope = %v, want ORGANIZATION", data["scope"])
	}
}
