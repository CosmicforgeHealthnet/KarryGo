package integration_test

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	approvalhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/http"
	approvalrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/repositories"
	approvalusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/usecases"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/stepup"
)

// TestStepUp_ApprovalDecisionsRequireAFreshGrant: spec 64.1 makes step-up
// mandatory for approval decisions. Through the production middleware an
// approve or reject without a grant (or with a grant for another action)
// is refused before the handler runs; with the right grant the handler is
// reached (404 for an unknown request).
func TestStepUp_ApprovalDecisionsRequireAFreshGrant(t *testing.T) {
	router, h := buildOnboardingRouterWith(t, func(pool *pgxpool.Pool, g *gin.RouterGroup) {
		authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0)
		service := approvalusecases.NewService(approvalusecases.Options{
			Pool: pool, Approvals: approvalrepos.NewRepository(pool), Roles: staffrepos.NewRoleRepository(pool),
			Subjects: approvalrepos.NewSubjectVersions(pool),
		})
		approvalhttp.NewHandler(service, authorizer).RegisterTenantRoutes(g)
	})
	owner, orgID := createStepUpOrganization(t, router, h, "stepup-approvals@example.com", "+2348012347091", "RC-STEPUP-APPROVALS")
	activateOrganization(t, h, orgID)

	otherGrant := obtainStepUp(t, router, h, owner, stepup.ActionProviderRemove)
	for _, action := range []string{"approve", "reject"} {
		path := "/approvals/requests/apr_unknown/" + action
		body := map[string]any{"reason": "checked"}
		code, resp := stepUpCall(t, router, owner, "", "POST", path, body, nil)
		requireStepUpRefusal(t, action+" without step-up", code, resp, "step_up_required", stepup.ActionApprovalDecide)
		code, resp = stepUpCall(t, router, owner, otherGrant, "POST", path, body, nil)
		requireStepUpRefusal(t, action+" with another action's grant", code, resp, "step_up_required", stepup.ActionApprovalDecide)
	}
	grant := obtainStepUp(t, router, h, owner, stepup.ActionApprovalDecide)
	for _, action := range []string{"approve", "reject"} {
		code, resp := stepUpCall(t, router, owner, grant, "POST", "/approvals/requests/apr_unknown/"+action, map[string]any{"reason": "checked"}, nil)
		if code != http.StatusNotFound {
			t.Fatalf("%s with an approvals grant = %d %v, want 404 from the handler", action, code, resp)
		}
	}
}
