package integration_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	companywallethttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/http"
	companywalletmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/models"
	companywalletusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/usecases"
	documenthttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/http"
	documentrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/repositories"
	documentusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops"
	marketplaceusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/safety"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/middleware"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Section 28 gap-1: field-visibility registry serialization tests, against
// internal/platform/fieldvisibility's declarations. Two distinct claims,
// tested two different ways per that package's own documented distinction:
//
//  1. SHAPED_VIEW claims (provider-facing DTOs, workforce.Profile,
//     documents' checksum omission): proven by constructing the real Go
//     struct with every field populated to a maximally revealing value,
//     marshaling it with the real encoding/json tags, and asserting the
//     forbidden field NAMES cannot appear in the wire bytes -- true
//     regardless of what data anyone ever puts into the fields that DO
//     exist, and regardless of the caller's permissions.
//  2. PERMISSION_ONLY claims (dispatcher/finance/support against
//     financeops/documents/safety/engagement/companywallet): proven at the
//     real HTTP surface -- a role holding none of the target permission
//     gets 403 before the handler ever touches the sensitive row, exactly
//     the boundary fieldvisibility's package doc says is the ONLY thing
//     protecting these routes today.

// forbiddenFieldsAbsent fails the test if any of the given field-name
// fragments appears (case-insensitively) as a JSON key in the encoded
// value -- catching both an exact key and a same-idea key spelled
// differently (e.g. "companyMargin" vs "margin").
func forbiddenFieldsAbsent(t *testing.T, label string, v any, forbidden []string) {
	t.Helper()
	encoded, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("%s: marshal: %v", label, err)
	}
	lower := strings.ToLower(string(encoded))
	for _, field := range forbidden {
		if strings.Contains(lower, strings.ToLower(field)) {
			t.Fatalf("%s: serialized JSON contains forbidden field %q: %s", label, field, encoded)
		}
	}
}

// TestProviderFacingViews_NeverSerializeCommercialOrInternalFields proves
// fieldvisibility's two ActorProvider/MechanismShapedView declarations:
// companywallet's ProviderStatement/SubWalletView and marketplace's
// ProviderOfferView/ProviderAssignmentSummaryView. Every field that DOES
// exist on each struct is populated with a real, non-zero value first, so
// this is not merely "an empty struct has no keys" -- it proves a fully
// populated instance still carries none of the forbidden data.
func TestProviderFacingViews_NeverSerializeCommercialOrInternalFields(t *testing.T) {
	forbiddenForProvider := []string{
		"customerFare", "customerPrice", "customerPayment", "paymentMethod",
		"organizationSettlement", "walletBalance", "companyMargin", "margin",
		"platformCommission", "commission", "contractPrice", "quotationPrice",
		"organizationProfit", "internalFinanceNote", "internalNote",
		"otherProviderTransaction", "bankAccountNumber", "bankSortCode",
		"destinationReference",
	}

	balance := int64(50000)
	synced := time.Now()
	statement := companywalletusecases.ProviderStatement{
		ProviderSubWalletID: "psw_1", MembershipID: "mem_1", Currency: "NGN", Status: "ACTIVE",
		AvailableBalanceMinor: &balance, SourceVersion: 4, FreshnessState: "FRESH", SyncedAt: &synced,
		GeneratedAt: time.Now(),
		Entries: []companywalletmodels.ProviderStatementEntry{{
			ID: "pse_1", EntryType: "ALLOCATION", AllocationType: "FUEL", Direction: "CREDIT",
			AmountMinor: 25000, Currency: "NGN", BusinessReferenceType: "TRIP", BusinessReferenceID: "trip_1",
			ReasonCode: "FUEL_ADVANCE", Explanation: "Fuel advance for trip", Status: "SETTLED",
			OccurredAt: time.Now(), SourceVersion: 1,
		}},
	}
	forbiddenFieldsAbsent(t, "companywallet.ProviderStatement", statement, forbiddenForProvider)

	subWallet := companywalletusecases.SubWalletView{
		ProviderSubWalletID: "psw_1", MembershipID: "mem_1", Currency: "NGN", Status: "ACTIVE",
		SourceVersion: 4, SyncedAt: &synced, FreshnessState: "FRESH",
	}
	forbiddenFieldsAbsent(t, "companywallet.SubWalletView", subWallet, forbiddenForProvider)

	offer := marketplaceusecases.ProviderOfferView{
		OfferID: "off_1", RequestID: "req_1", SourceService: "driver-dispatch-delivery-service",
		ServiceType: "DISPATCH", Status: "OFFERED", ExpiresAt: time.Now().Add(30 * time.Second),
		Version: 1, AllowedActions: []string{"ACCEPT", "DECLINE"},
	}
	forbiddenFieldsAbsent(t, "marketplace.ProviderOfferView", offer, forbiddenForProvider)

	assignment := marketplaceusecases.ProviderAssignmentSummaryView{
		AssignmentID: "asg_1", RequestID: "req_1", SourceService: "driver-dispatch-delivery-service",
		Status: "ASSIGNED", ResponseDeadline: time.Now().Add(30 * time.Second),
		Version: 1, AllowedActions: []string{"ACKNOWLEDGE"},
	}
	forbiddenFieldsAbsent(t, "marketplace.ProviderAssignmentSummaryView", assignment, forbiddenForProvider)
}

// TestDocumentView_NeverSerializesChecksum proves the SHAPED_VIEW
// declaration for documents: DocumentView omits MediaChecksumSHA256 even
// though the underlying Document model carries it -- true for every actor
// class this route is reachable by, not just finance/support (who cannot
// reach it at all today, proven separately below).
func TestDocumentView_NeverSerializesChecksum(t *testing.T) {
	view := documenthttp.DocumentView{
		ID: "doc_1", OwnerType: "ASSET", OwnerID: "ast_1", Category: "COMPLIANCE",
		DocumentType: "INSURANCE", LifecycleState: "ACTIVE", VerificationState: "VERIFIED",
		SafetyProjection: "CLEAN", ProjectionFreshness: "FRESH", MediaSourceVersion: 3,
		RetentionClass: "STANDARD", Visibility: "INTERNAL", Verified: true, Downloadable: true,
	}
	forbiddenFieldsAbsent(t, "documents.DocumentView", view, []string{"checksum", "sha256"})
}

// TestDispatcherPermission_CannotReachPayrollOrCompensation proves
// fieldvisibility's ActorDispatcher/MechanismPermissionOnly declarations:
// payroll-periods and compensation-rules are both refused before the
// sensitive row is ever read.
func TestDispatcherPermission_CannotReachPayrollOrCompensation(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Field Visibility Dispatcher Co", "RC-FV-DISP-1")
	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0)
	service := financeops.NewService(financeops.Options{Pool: pool, Flags: financeops.Flags{Payroll: true, Compensation: true}})

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		member := c.GetHeader("X-Test-Member")
		c.Set(middleware.TenantContextKey, tenant.Context{
			OrganizationID: org.ID, OrganizationUserID: member,
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: member,
		})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	group := engine.Group("/fleet-partner/v1")
	financeops.NewHandler(service, authorizer, nil).RegisterTenantRoutes(group)

	// rol_system_dispatcher holds workforce.*/shifts.*/dispatch-shaped
	// permissions but never payroll.*/compensation.* (migrations 000050:
	// 86-87 grant those to owner/operations/finance only).
	dispatcher := seedApprovalMember(t, pool, org.ID, "rol_system_dispatcher", "dispatcher")

	for _, path := range []string{"/fleet-partner/v1/payroll-periods", "/fleet-partner/v1/compensation-rules"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Test-Member", dispatcher)
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("dispatcher GET %s status = %d body=%s, want 403", path, recorder.Code, recorder.Body.String())
		}
	}
}

// TestFinancePermission_CannotReachIncidentEvidenceSupportContentOrDocuments
// proves fieldvisibility's three ActorFinance/MechanismPermissionOnly
// declarations against safety incidents, engagement support cases, and
// documents.
func TestFinancePermission_CannotReachIncidentEvidenceSupportContentOrDocuments(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Field Visibility Finance Co", "RC-FV-FIN-1")
	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		member := c.GetHeader("X-Test-Member")
		c.Set(middleware.TenantContextKey, tenant.Context{
			OrganizationID: org.ID, OrganizationUserID: member,
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: member,
		})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	group := engine.Group("/fleet-partner/v1")

	safetyService := safety.NewOperationsService(safety.OperationsOptions{Pool: pool, Flags: safety.OperationsFlags{Reports: true, Incidents: true}})
	safety.NewOperationsHandler(safetyService, authorizer, nil).RegisterTenantRoutes(group)

	engagementService := engagement.NewService(engagement.Options{Pool: pool, Flags: engagement.Flags{Support: true}})
	engagement.NewHandler(engagementService, authorizer, nil).RegisterTenantRoutes(group)

	documentService := documentusecases.NewService(documentusecases.Options{
		Pool: pool, Documents: documentrepos.NewRepository(pool), Flags: documentusecases.Flags{},
	})
	documenthttp.NewHandler(documentService, authorizer).RegisterTenantRoutes(group)

	// rol_system_finance holds finance.*/wallet.*/invoices.*/payroll.* but
	// never incidents.*/support.read/documents.* (migrations 000044, 000052,
	// 000021, 000054 all exclude it from their grant blocks).
	financeMember := seedApprovalMember(t, pool, org.ID, "rol_system_finance", "finance")

	for _, path := range []string{
		"/fleet-partner/v1/incidents",
		"/fleet-partner/v1/support/cases",
		"/fleet-partner/v1/documents",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Test-Member", financeMember)
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("finance GET %s status = %d body=%s, want 403", path, recorder.Code, recorder.Body.String())
		}
	}
}

// TestSupportPermission_CannotReachWalletPayrollOrDocuments proves
// fieldvisibility's three ActorSupport/MechanismPermissionOnly
// declarations. rol_system_support's real grant surface (organization.read,
// providers.read, assignments.read, trips.read only -- migrations 000005,
// 000010) is far narrower than its role name implies; this test proves the
// three sensitive-data routes specifically, not an exhaustive audit of
// everything it lacks.
func TestSupportPermission_CannotReachWalletPayrollOrDocuments(t *testing.T) {
	pool := testPool(t)
	org := createOrganization(t, pool, "Field Visibility Support Co", "RC-FV-SUP-1")
	authorizer := accesscontrol.NewAuthorizer(staffrepos.NewRoleRepository(pool), nil, 0)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		member := c.GetHeader("X-Test-Member")
		c.Set(middleware.TenantContextKey, tenant.Context{
			OrganizationID: org.ID, OrganizationUserID: member,
			ActorType: tenant.ActorTypeOrganizationUser, ActorID: member,
		})
		c.Set(middleware.TenantRoleVersionKey, int64(1))
		c.Next()
	})
	group := engine.Group("/fleet-partner/v1")

	walletService := companywalletusecases.NewService(companywalletusecases.Options{Pool: pool, Flags: companywalletusecases.Flags{}})
	companywallethttp.NewHandler(walletService, authorizer).RegisterTenantRoutes(group)

	financeopsService := financeops.NewService(financeops.Options{Pool: pool, Flags: financeops.Flags{Payroll: true}})
	financeops.NewHandler(financeopsService, authorizer, nil).RegisterTenantRoutes(group)

	documentService := documentusecases.NewService(documentusecases.Options{
		Pool: pool, Documents: documentrepos.NewRepository(pool), Flags: documentusecases.Flags{},
	})
	documenthttp.NewHandler(documentService, authorizer).RegisterTenantRoutes(group)

	supportMember := seedApprovalMember(t, pool, org.ID, "rol_system_support", "support")

	for _, path := range []string{
		"/fleet-partner/v1/wallet",
		"/fleet-partner/v1/payroll-periods",
		"/fleet-partner/v1/documents",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Test-Member", supportMember)
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("support GET %s status = %d body=%s, want 403", path, recorder.Code, recorder.Body.String())
		}
	}
}
