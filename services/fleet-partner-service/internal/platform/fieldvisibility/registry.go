// Package fieldvisibility is Section 28's field-level authorization
// registry: which of this codebase's nine named actor classes (provider,
// dispatcher, fleet manager, finance, support, compliance, organization
// owner, internal service, platform operator) must never receive which
// categories of sensitive field, and -- the fact that actually determines
// whether that boundary holds -- HOW each boundary is enforced today.
//
// Section 28's taxonomy names nine actor classes, but (as
// internal/platform/objectauth's own package doc already established for
// object-level authorization) this codebase's real authentication system
// recognizes only four actor TYPES: organization_user, internal_service,
// provider and public_token. Dispatcher, fleet manager, finance, support,
// compliance and organization owner are PERMISSION-gated ROLES within the
// single organization_user type (accesscontrol.Authorizer.Require checks a
// permission string, never a role name) -- not independent identities the
// way provider and internal_service are. "Platform operator" corresponds
// to internal_service (the HMAC-authenticated worker-operations/backoffice
// surface). This registry names both: the six role-classes as they are --
// permission bundles, not separate credentials -- and the boundary
// mechanism that actually protects each one.
//
// Enforcement, once discovered by reading every resource this file names,
// falls into exactly two mechanisms, and conflating them would overstate
// how safe this service is:
//
//  1. STRUCTURAL (Mechanism = MechanismShapedView): the HTTP response is an
//     explicit, hand-written, allowlisted view/DTO struct (companywallet's
//     ProviderStatement/SubWalletView, marketplace's ProviderOfferView/
//     ProviderAssignmentSummaryView, documents' externalDocumentView) whose
//     Go struct definition makes the forbidden field impossible to
//     serialize regardless of what permission grants exist. This is safe
//     even against a future migration that grants an unexpected permission
//     to an unexpected role.
//  2. PERMISSION-ONLY (Mechanism = MechanismPermissionOnly): the handler
//     calls respond.OK with the raw domain struct or a row_to_json map (no
//     field-level shaping at all), and the ONLY thing keeping a forbidden
//     actor class away from the sensitive fields is that no migration
//     currently grants that actor class's role the route's permission. A
//     single future INSERT INTO fleet_partner_role_permissions could
//     silently remove this protection; nothing at the handler layer would
//     notice or resist it. Declarations of this kind are deliberately
//     marked, not hidden behind a reassuring PASS -- see
//     internal/integration/field_visibility_test.go's PERMISSION-ONLY
//     tests, which prove today's grant table keeps the boundary closed
//     without claiming the handler code itself would.
package fieldvisibility

// Actor classes, matching Section 28's own nine-class taxonomy. See the
// package doc comment for which are real authentication types and which
// are permission bundles within organization_user.
const (
	ActorProvider          = "provider"
	ActorDispatcher        = "dispatcher"
	ActorFleetManager      = "fleet_manager"
	ActorFinance           = "finance"
	ActorSupport           = "support"
	ActorCompliance        = "compliance"
	ActorOrganizationOwner = "organization_owner"
	ActorInternalService   = "internal_service"
	ActorPlatformOperator  = "platform_operator"
)

// Mechanism names how a ForbiddenFieldSet is actually enforced.
const (
	MechanismShapedView     = "SHAPED_VIEW"
	MechanismPermissionOnly = "PERMISSION_ONLY"
	MechanismUnreachable    = "UNREACHABLE"
)

// ForbiddenFieldSet is one actor class's declared boundary against one
// resource family.
type ForbiddenFieldSet struct {
	Actor     string
	Resource  string
	Forbidden []string
	Mechanism string
	// Evidence names the exact Go type (for MechanismShapedView) or the
	// exact permission string plus the seeded roles that hold it (for
	// MechanismPermissionOnly) backing this declaration -- so a reviewer
	// does not have to re-derive it from source.
	Evidence string
}

// Declarations is the full registry, one entry per actor-class/resource
// boundary this pass verified by reading the real handler and, for
// MechanismShapedView entries, the real DTO struct.
var Declarations = []ForbiddenFieldSet{
	// --- Provider: structurally safe, not just permission-gated. ---
	{
		Actor:    ActorProvider,
		Resource: "companywallet.ProviderStatement / SubWalletView",
		Forbidden: []string{
			"customerFare", "customerPrice", "customerPayment", "paymentMethod",
			"organizationSettlement", "organizationWalletBalance", "companyMargin",
			"platformCommission", "contractPrice", "quotationPrice", "organizationProfit",
			"internalFinanceNote", "internalApprovalDiscussion", "otherProviderTransaction",
			"bankAccountNumber", "bankSortCode",
		},
		Mechanism: MechanismShapedView,
		Evidence:  "internal/features/companywallet/usecases/statement.go:15-44 (ProviderStatement, doc comment names this exact exclusion list); service.go:363-371 (SubWalletView)",
	},
	{
		Actor:    ActorProvider,
		Resource: "marketplace.ProviderOfferView / ProviderAssignmentSummaryView",
		Forbidden: []string{
			"fare", "price", "customerFare", "commission", "margin",
		},
		Mechanism: MechanismShapedView,
		Evidence:  "internal/features/marketplace/usecases/provider_reads.go:13-38; underlying models.Offer/Assignment carry no price field at all (models/models.go)",
	},
	{
		// Finance's tenant-scoped operator surface is corporate-bearer-only
		// (internal/app/wiring.go tenantScoped registrar); its internal-HMAC
		// counterpart returns only {"status": ...} bodies, never an
		// Operation/Projection. A provider has no path to either.
		Actor:     ActorProvider,
		Resource:  "finance.OperatorHandler (all routes)",
		Forbidden: []string{"*"},
		Mechanism: MechanismUnreachable,
		Evidence:  "internal/features/finance/operator_http.go mounted only in tenantScoped (corporate bearer); internal/features/finance/http.go internal routes return {\"status\":...} only, never Operation/Projection bodies",
	},

	// --- Dispatcher: permission-table-only today. ---
	{
		Actor:     ActorDispatcher,
		Resource:  "financeops payroll-periods (GET/POST /payroll-periods*)",
		Forbidden: []string{"*"},
		Mechanism: MechanismPermissionOnly,
		Evidence:  "permission payroll.* gated at internal/features/financeops/http.go; granted only to rol_system_owner/rol_system_operations/rol_system_finance (migrations/000050_compensation_payroll_settlements.up.sql:86-87) -- rol_system_dispatcher absent. GetPayrollPeriod (financeops/payroll.go) returns row_to_json of the full row with no DTO shaping, so this boundary is the grant table alone.",
	},
	{
		Actor:     ActorDispatcher,
		Resource:  "financeops compensation-rules (GET/POST /compensation-rules*)",
		Forbidden: []string{"*"},
		Mechanism: MechanismPermissionOnly,
		Evidence:  "permission compensation.* granted only to owner/operations/finance (migrations/000050:86-87); rol_system_dispatcher absent",
	},
	{
		Actor:     ActorDispatcher,
		Resource:  "workforce.Profile (bank/payroll fields)",
		Forbidden: []string{"bankReference", "payrollAmount"},
		Mechanism: MechanismShapedView,
		Evidence:  "internal/features/workforce/domain.go:12-17 Profile struct carries no bank/payroll field at all -- structurally absent, not merely unreached",
	},

	// --- Finance: permission-table-only for incidents/support; documents
	// additionally strips the one field (checksum) that would matter even
	// for a role that CAN read documents. ---
	{
		Actor:     ActorFinance,
		Resource:  "safety incidents/evidence (GET /incidents*, /corrective-actions/*)",
		Forbidden: []string{"*"},
		Mechanism: MechanismPermissionOnly,
		Evidence:  "permission incidents.* (migrations/000044_safety_incidents_proof.up.sql:111-119) granted to owner/operations/compliance only -- rol_system_finance absent",
	},
	{
		Actor:     ActorFinance,
		Resource:  "engagement support cases (GET /support/cases*)",
		Forbidden: []string{"*"},
		Mechanism: MechanismPermissionOnly,
		Evidence:  "permission support.read (migrations/000052_support_quality_sla_escalations.up.sql:82-84) granted to owner/operations/compliance only -- rol_system_finance absent",
	},
	{
		Actor:     ActorFinance,
		Resource:  "documents.DocumentView (identity-document content)",
		Forbidden: []string{"mediaChecksumSha256"},
		Mechanism: MechanismShapedView,
		Evidence:  "internal/features/documents/http/handlers.go DocumentView omits MediaChecksumSHA256 even though Document (models/models.go:305) carries it -- true for every role, finance included, and finance additionally has no documents.* grant at all (migrations/000021:338-363, migrations/000054:44)",
	},

	// --- Support: permission-table-only, and its own real grant surface is
	// far narrower than its role name implies (organization.read,
	// providers.read, assignments.read, trips.read only). ---
	{
		Actor:     ActorSupport,
		Resource:  "companywallet (GET /wallet, sub-wallet, transactions)",
		Forbidden: []string{"*"},
		Mechanism: MechanismPermissionOnly,
		Evidence:  "permission wallet.read and every other wallet.* permission (migrations/000022_..._wallet.up.sql grant list) never names rol_system_support",
	},
	{
		Actor:     ActorSupport,
		Resource:  "financeops payroll-periods",
		Forbidden: []string{"*"},
		Mechanism: MechanismPermissionOnly,
		Evidence:  "payroll.* granted only to owner/operations/finance (migrations/000050:86-87) -- rol_system_support absent",
	},
	{
		Actor:     ActorSupport,
		Resource:  "documents (GET /documents*)",
		Forbidden: []string{"*"},
		Mechanism: MechanismPermissionOnly,
		Evidence:  "documents.* never granted to rol_system_support (migrations/000021:338-363, migrations/000054:44)",
	},
	{
		Actor:     ActorSupport,
		Resource:  "companywallet.WithdrawalView (bank/destination reference)",
		Forbidden: []string{"destinationReference"},
		Mechanism: MechanismShapedView,
		Evidence:  "internal/features/companywallet/http/handlers.go:495-512 WithdrawalView deliberately omits DestinationReference (comment: \"a handle that resolves to somebody's bank account does not belong in a listing that a broad read permission can reach\") -- structurally true for every role, moot for support specifically since it also holds zero wallet.* permission",
	},

	// --- The remaining five classes: lighter-touch declarations, no
	// serialization proof mandated by Section 28 for these, but named so
	// the registry is complete for all nine. ---
	{
		Actor:     ActorFleetManager,
		Resource:  "assets/warehouse/dispatch operational data",
		Forbidden: []string{},
		Mechanism: MechanismPermissionOnly,
		Evidence:  "fleet-manager-shaped roles (rol_system_operations) hold the full assets.*/warehouse.*/dispatch.* surface by design -- this is the role this data is FOR, not a boundary",
	},
	{
		Actor:     ActorCompliance,
		Resource:  "financial settlement/payroll amounts",
		Forbidden: []string{"payrollAmount", "settlementAmount"},
		Mechanism: MechanismPermissionOnly,
		Evidence:  "payroll.*/subcontractor_settlements.* not granted to rol_system_compliance (migrations/000050, checked against the compliance grant block)",
	},
	{
		Actor:     ActorOrganizationOwner,
		Resource:  "(none -- owner is the maximal-permission system role)",
		Forbidden: []string{},
		Mechanism: MechanismPermissionOnly,
		Evidence:  "rol_system_owner holds every permission this registry's other entries restrict; deliberately not a boundary to prove",
	},
	{
		Actor:     ActorInternalService,
		Resource:  "customer/provider identity beyond the caller's own allowlisted scope",
		Forbidden: []string{"otherServiceData"},
		Mechanism: MechanismPermissionOnly,
		Evidence:  "internal routes are gated by an explicit HMAC caller allowlist per package (documents/backgroundops/marketplace internal.go files), not a permission string -- see internal/platform/objectauth's internalCallerAllowlistSentinel entries for the full route list",
	},
	{
		Actor:     ActorPlatformOperator,
		Resource:  "tenant business data outside worker-operations scope",
		Forbidden: []string{"tenantBusinessData"},
		Mechanism: MechanismUnreachable,
		Evidence:  "worker-operations (internal/features/backgroundops/http/internal.go) exposes only Job/Task/Attempt/Run views (JobView etc.), never tenant business resources -- confirmed by internal/section28contract's object-authorization registry, internalCallerAllowlistSentinel entries",
	},
}
