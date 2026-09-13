package app

import (
	"context"
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/shared/go/auth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/events"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/accesscontrol"
	approvalhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/http"
	approvalrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/repositories"
	approvalusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/usecases"
	assetsfeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/assets"
	audithttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/audit/http"
	auditrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/audit/repositories"
	authclients "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/clients"
	authhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/http"
	authrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/repositories"
	authusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/auth/usecases"
	backgroundopshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/backgroundops/http"
	bulkhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/http"
	bulkusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/bulkimports/usecases"
	wallethttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/http"
	walletusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/companywallet/usecases"
	compliancefeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/compliance"
	crmhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/http"
	crmusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/usecases"
	dashboardhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/dashboard/http"
	dashboardusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/dashboard/usecases"
	dispatchfeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch"
	documenthttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/http"
	documentrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/repositories"
	documentusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/usecases"
	eligibilityhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/http"
	eligibilityrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/repositories"
	eligibilityusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/engagement"
	exportshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/http"
	exportsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/exports/usecases"
	financefeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/finance"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/financeops"
	integrationsfeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/integrations"
	legalholdhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/legalhold/http"
	legalholdusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/legalhold/usecases"
	manifestshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/manifests/http"
	manifestsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/manifests/usecases"
	marketplacehttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/http"
	mktusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	ordershttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/http"
	ordersusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/orders/usecases"
	orghttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/http"
	orgrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	orgusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/usecases"
	packageshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/packages/http"
	packagesusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/packages/usecases"
	privatehttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/http"
	privateusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/usecases"
	prooffeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/proof"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	providerusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/usecases"
	recurringhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/recurringorders/http"
	recurringusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/recurringorders/usecases"
	retentionexecutionhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionexecution/http"
	retentionpolicyhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/http"
	retentionpolicyusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/usecases"
	safetyfeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/safety"
	safetyhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/safety/http"
	safetyrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/safety/repositories"
	safetyusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/safety/usecases"
	searchhttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/search/http"
	searchusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/search/usecases"
	securitysettingshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/securitysettings/http"
	securitysettingsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/securitysettings/usecases"
	cataloghttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/http"
	catalogusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/servicecatalog/usecases"
	shipmentshttp "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/http"
	shipmentsusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/shipments/usecases"
	specializedfeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/specialized"
	staffrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/repositories"
	staffusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/synchronization"
	warehousefeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/warehouse"
	workforcefeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/workforce"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cache"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/circuitbreaker"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ratelimit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/retentionexec"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
	"cosmicforge/logistics/services/fleet-partner-service/internal/scheduler"
)

// Components bundles the wired application services so both the API binary
// and the worker binary build the same object graph.
type Components struct {
	Pool *pgxpool.Pool
	// Metrics backs observability for anything Registrars wires up directly
	// (currently: rate-limit denial counters). Everything else in this
	// struct already threads its own *observability.Metrics through its own
	// Build* constructor; this field exists only because Registrars itself
	// has no metrics parameter.
	Metrics       *observability.Metrics
	Auth          *authusecases.Service
	Staff         *staffusecases.Service
	Organizations *orgusecases.Service
	Providers     *providerusecases.Service
	Resolver      *staffusecases.MembershipResolver
	Authorizer    *accesscontrol.Authorizer
	Sessions      *authrepos.SessionRepository
	Challenges    *authrepos.ChallengeRepository
	Invitations   *staffrepos.InvitationRepository
	ProviderRepo  *providerrepos.Repository
	Contracts     platformclients.ContractSet
	// Safety records fleet-side incident and dispute references. Always
	// constructed; whether it can VERIFY a reference depends on the
	// support-dispute client being configured, and an unconfigured client
	// makes the write fail closed rather than the feature disappear.
	Safety  *safetyusecases.Service
	Finance *financefeature.Service
	// Documents is always constructed. Whether a document can be UPLOADED or
	// DOWNLOADED depends on the media client being configured and the flags
	// being on; an unconfigured client makes those paths fail closed, and it
	// never makes the read surface or the eligibility rule disappear.
	Documents *documentusecases.Service
	// Eligibility and Approvals are always constructed. Whether a vehicle or
	// provider question can be ANSWERED depends on the authority client being
	// configured, and an unconfigured client makes the decision UNKNOWN — it
	// never makes the gate disappear.
	Eligibility *eligibilityusecases.Service
	Approvals   *approvalusecases.Service
	// CRM is the Section 14 customer relationship, quotation, contract, SLA
	// and credit-policy service. Always constructed; its writes fail closed
	// when the flags are off, when the kill switch is engaged, or when Media
	// is unconfigured — it never makes the read surface disappear.
	CRM *crmusecases.Service
	// ServiceCatalog and Orders are the Section 15 (WP14) private-order core.
	// Orders reads through the CRM and ServiceCatalog repositories directly —
	// sibling feature packages of the same organization database — for
	// customer/credit/contract validation, catalogue activity and Section 14's
	// quotation-conversion-intent handoff. Always constructed; both fail
	// closed to not-found when their enablement flag is off.
	ServiceCatalog *catalogusecases.Service
	Orders         *ordersusecases.Service
	// Shipments is Section 15 slice A4. Always constructed; fails closed to
	// not-found when its enablement flag is off.
	Shipments         *shipmentsusecases.Service
	Packages          *packagesusecases.Service
	Manifests         *manifestsusecases.Service
	RecurringOrders   *recurringusecases.Service
	BulkImports       *bulkusecases.Service
	PrivateOperations *privateusecases.Service
	DispatchControl   *dispatchfeature.Service
	Workforce         *workforcefeature.Service
	Assets            *assetsfeature.Service
	Compliance        *compliancefeature.Service
	SafetyOperations  *safetyfeature.OperationsService
	Proof             *prooffeature.Service
	Specialized       *specializedfeature.Service
	Warehouse         *warehousefeature.Service
	FinanceOperations *financeops.Service
	Engagement        *engagement.Service
	Marketplace       *mktusecases.Service
	// Search is the Section 25 (partial slice) full-text search projection.
	// Always constructed; with the enablement flag off its route answers
	// not-found. It is a read-side projection only — nothing else in this
	// service is called from inside Search's transaction, and Search is
	// called from inside no other feature's transaction either.
	Search *searchusecases.Service
	// Exports is the Section 25D generic asynchronous export framework.
	// Always constructed; with the enablement flag off its routes answer
	// not-found. Only the ORDERS dataset adapter is registered so far — see
	// BuildExportsService.
	Exports         *exportsusecases.Service
	Synchronization *synchronization.Service
	// Idempotency backs HTTP-level command replay (Idempotency-Key) for
	// Section 15 creation routes that require it, starting with shipments.
	Idempotency *idempotency.Store
	// Limiter bounds the unauthenticated customer quotation routes. Without
	// it those routes are not mounted at all: an unlimited token-validation
	// endpoint is a brute-force oracle.
	Limiter ratelimit.Limiter
	// RetentionRunner is Section 28's executable-retention engine. Always
	// constructed (same posture as every other Components field); whether
	// its durable job handler is actually registered on the worker depends
	// on cfg.RetentionExecutionEnabled, wired in cmd/main.go -- an
	// unregistered handler means EXECUTE-mode runs stay PENDING forever
	// (never silently run), and DRY_RUN previews (synchronous, no job
	// queue involved) are unaffected either way.
	RetentionRunner *retentionexec.Runner
	// RetentionExpiredExportReady, RetentionSupportMetadataReady and
	// RetentionSupersededProjectionReady report whether each of the three
	// extra retention class executors (see Registrars, where they are
	// set) found its real mandatory dependency configured and was
	// therefore actually registered on RetentionRunner -- OrgContact,
	// this package's original executor, has no external dependency and is
	// always registered, so it has no corresponding field. cmd/main.go
	// reads these to build a readiness check that fails closed when
	// cfg.RetentionExecutionEnabled is true but one of these is false,
	// rather than silently running with fewer classes than a deployment
	// believes it enabled.
	RetentionExpiredExportReady        bool
	RetentionSupportMetadataReady      bool
	RetentionSupersededProjectionReady bool
	// CompanyWallet is the Section 13 company wallet and provider sub-wallet
	// service. Always constructed; its writes fail closed when the Wallet
	// client is unconfigured or its flags are off.
	CompanyWallet *walletusecases.Service
	Integrations  *integrationsfeature.Service
}

// BuildComponents assembles the domain services.
//
// The OTP sender is always the notification-service adapter: no logging or
// no-op sender is selectable from configuration, so OTP delivery can never
// falsely succeed in production. Tests inject a fake sender directly.
func BuildComponents(cfg config.Config, pool *pgxpool.Pool, redisClient *redis.Client, metrics *observability.Metrics) *Components {
	outbox := events.NewOutboxRepository(pool)
	publisher := events.NewDomainPublisher(outbox)

	notificationCfg := cfg.Clients["notification-service"]
	if cfg.IntegrationsKillSwitch {
		notificationCfg.BaseURL = ""
		notificationCfg.Secret = nil
	}
	sender := authclients.NewNotificationSender(notificationCfg.BaseURL, notificationCfg.Secret, cfg.HTTPClientTimeout)

	var entryCache *cache.Cache
	var limiter ratelimit.Limiter = ratelimit.NewMemoryLimiter()
	if redisClient != nil {
		entryCache = cache.New(redisClient)
		limiter = ratelimit.NewRedisLimiter(redisCounter{client: redisClient})
	}

	// Contracts for capabilities the authoritative services do not expose
	// yet: fail-closed adapters plus reconciliation (see the audit doc).
	contracts := platformclients.NewUnconfiguredContracts()
	walletCfg := cfg.Clients["payment-wallet-service"]
	if !cfg.IntegrationsKillSwitch && walletCfg.BaseURL != "" && len(walletCfg.Secret) > 0 {
		contracts.Wallet = platformclients.NewWalletClient(platformclients.Options{ServiceName: "payment-wallet-service", BaseURL: walletCfg.BaseURL, Secret: walletCfg.Secret, Timeout: cfg.HTTPClientTimeout, Metrics: metrics, Environment: cfg.AppEnv, MaxRetries: cfg.ExternalClientMaxRetries, RetryBackoffBase: cfg.ExternalClientRetryBackoffBase, RetryBackoffMax: cfg.ExternalClientRetryBackoffMax, Breaker: circuitbreaker.Config{FailureThreshold: cfg.CircuitBreakerFailureThreshold, ObservationWindow: cfg.CircuitBreakerObservationWindow, OpenDuration: cfg.CircuitBreakerOpenDuration, HalfOpenProbes: cfg.CircuitBreakerHalfOpenProbes, SuccessThreshold: cfg.CircuitBreakerSuccessThreshold}})
	}
	mediaCfg := cfg.Clients["media-file-service"]
	if !cfg.IntegrationsKillSwitch && mediaCfg.Configured() {
		contracts.Media = platformclients.NewMediaFileClient(platformclients.Options{ServiceName: "media-file-service", BaseURL: mediaCfg.BaseURL, Secret: mediaCfg.Secret, Timeout: cfg.HTTPClientTimeout, Metrics: metrics, Environment: cfg.AppEnv, MaxRetries: cfg.ExternalClientMaxRetries, RetryBackoffBase: cfg.ExternalClientRetryBackoffBase, RetryBackoffMax: cfg.ExternalClientRetryBackoffMax, Breaker: circuitbreaker.Config{FailureThreshold: cfg.CircuitBreakerFailureThreshold, ObservationWindow: cfg.CircuitBreakerObservationWindow, OpenDuration: cfg.CircuitBreakerOpenDuration, HalfOpenProbes: cfg.CircuitBreakerHalfOpenProbes, SuccessThreshold: cfg.CircuitBreakerSuccessThreshold}})
	}
	verificationCfg := cfg.Clients["verification-compliance-service"]
	if !cfg.IntegrationsKillSwitch && verificationCfg.Configured() {
		contracts.Verification = platformclients.NewVerificationClient(platformclients.Options{ServiceName: "verification-compliance-service", BaseURL: verificationCfg.BaseURL, Secret: verificationCfg.Secret, Timeout: cfg.HTTPClientTimeout, Metrics: metrics, Environment: cfg.AppEnv, MaxRetries: cfg.ExternalClientMaxRetries, RetryBackoffBase: cfg.ExternalClientRetryBackoffBase, RetryBackoffMax: cfg.ExternalClientRetryBackoffMax, Breaker: circuitbreaker.Config{FailureThreshold: cfg.CircuitBreakerFailureThreshold, ObservationWindow: cfg.CircuitBreakerObservationWindow, OpenDuration: cfg.CircuitBreakerOpenDuration, HalfOpenProbes: cfg.CircuitBreakerHalfOpenProbes, SuccessThreshold: cfg.CircuitBreakerSuccessThreshold}})
	}
	haulingCfg := cfg.Clients["driver-hauling-service"]
	if haulingCfg.Configured() && cfg.ExecutionCapabilities().Hauling {
		haulingClient := platformclients.NewHaulingProviderClient(platformclients.Options{
			ServiceName: "driver-hauling-service", BaseURL: haulingCfg.BaseURL, Secret: haulingCfg.Secret,
			Timeout: cfg.HTTPClientTimeout, Metrics: metrics, Environment: cfg.AppEnv,
			MaxRetries: cfg.ExternalClientMaxRetries, RetryBackoffBase: cfg.ExternalClientRetryBackoffBase, RetryBackoffMax: cfg.ExternalClientRetryBackoffMax,
			Breaker: circuitbreaker.Config{FailureThreshold: cfg.CircuitBreakerFailureThreshold, ObservationWindow: cfg.CircuitBreakerObservationWindow, OpenDuration: cfg.CircuitBreakerOpenDuration, HalfOpenProbes: cfg.CircuitBreakerHalfOpenProbes, SuccessThreshold: cfg.CircuitBreakerSuccessThreshold},
		})
		contracts.Providers[platformclients.CategoryHauling] = haulingClient
		contracts.HaulingLinks = haulingClient
	}

	members := staffrepos.NewMemberRepository(pool)
	roles := staffrepos.NewRoleRepository(pool)
	invitations := staffrepos.NewInvitationRepository(pool)
	sessions := authrepos.NewSessionRepository(pool)
	challenges := authrepos.NewChallengeRepository(pool)

	// The durable eligibility gate. Built from the shared helper so the API
	// binary and the marketplace worker cannot end up with different rules.
	eligibilityService := eligibilityusecases.Build(cfg, pool, metrics)

	organizations := orgusecases.NewService(orgusecases.Options{
		Pool:          pool,
		Organizations: orgrepos.NewOrganizationRepository(pool),
		Onboarding:    orgrepos.NewOnboardingRepository(pool),
		Branches:      orgrepos.NewBranchRepository(pool),
		Settings:      orgrepos.NewSettingsRepository(pool),
		Contracts:     contracts,
		Eligibility:   eligibilityService,
		Events:        publisher,
		Metrics:       metrics,
	})

	staffService := staffusecases.NewService(staffusecases.Options{
		Pool:               pool,
		Members:            members,
		Invitations:        invitations,
		Roles:              roles,
		Organizations:      organizations,
		Sender:             sender,
		Sessions:           sessions,
		Events:             publisher,
		Metrics:            metrics,
		InvitationSecret:   cfg.SessionSecret,
		InvitationTTL:      cfg.StaffInvitationTTL,
		AcceptURLTemplate:  cfg.StaffAcceptURL,
		DefaultCountryCode: cfg.DefaultCountryCode,
	})
	// The organization service needs the staff service to create the founding
	// owner membership; wiring it after construction avoids an import cycle.
	organizations.SetMembershipCreator(staffService)

	providerRepo := providerrepos.NewRepository(pool)
	providers := providerusecases.NewService(providerusecases.Options{
		Pool:               pool,
		Providers:          providerRepo,
		Organizations:      organizations,
		Contracts:          contracts,
		Eligibility:        eligibilityService,
		Sender:             sender,
		Events:             publisher,
		Metrics:            metrics,
		InvitationSecret:   cfg.SessionSecret,
		InvitationTTL:      cfg.ProviderInvitationTTL,
		AcceptURLTemplate:  cfg.ProviderAcceptURL,
		DefaultCountryCode: cfg.DefaultCountryCode,
	})

	approvalService := approvalusecases.NewService(approvalusecases.Options{
		Pool:        pool,
		Approvals:   approvalrepos.NewRepository(pool),
		Eligibility: eligibilityService,
		Roles:       roles,
		Subjects:    approvalrepos.NewSubjectVersions(pool),
		Events:      publisher,
		Metrics:     metrics,
	})

	authService := authusecases.NewService(authusecases.Options{
		Pool:               pool,
		Identities:         authrepos.NewIdentityRepository(pool),
		Challenges:         challenges,
		Sessions:           sessions,
		SecurityEvents:     authrepos.NewSecurityEventRepository(pool),
		Sender:             sender,
		Limiter:            limiter,
		Metrics:            metrics,
		Memberships:        staffService,
		Invitations:        staffService,
		OTPSecret:          cfg.OTPSecret,
		SessionSecret:      cfg.SessionSecret,
		ContactSecret:      cfg.ContactSecret,
		AccessSigner:       auth.NewTokenSigner(cfg.AccessTokenSecret),
		OTPLength:          cfg.OTPLength,
		OTPExpiry:          cfg.OTPExpiry,
		OTPMaxAttempts:     cfg.OTPMaxAttempts,
		OTPResendCooldown:  cfg.OTPResendCooldown,
		OTPMaxPerHour:      cfg.OTPMaxPerHour,
		AccessTokenTTL:     cfg.AccessTokenTTL,
		RefreshTokenTTL:    cfg.RefreshTokenTTL,
		StepUpTTL:          cfg.StepUpTTL,
		DefaultCountryCode: cfg.DefaultCountryCode,
	})

	// Safety references. The validator is a real client whenever the
	// support-dispute base URL and secret are configured; otherwise it is nil,
	// and every attempt to record a verifiable reference is refused rather
	// than downgraded.
	supportCfg := cfg.Clients["support-dispute-service"]
	var safetyValidator platformclients.SafetyReferenceValidator
	if supportCfg.BaseURL != "" && len(supportCfg.Secret) > 0 {
		safetyValidator = platformclients.NewSupportDisputeReferenceClient(platformclients.Options{
			ServiceName: "support-dispute-service",
			BaseURL:     supportCfg.BaseURL,
			Secret:      supportCfg.Secret,
			Timeout:     cfg.HTTPClientTimeout,
			Metrics:     metrics,
			Environment: cfg.AppEnv,
			MaxRetries:  cfg.ExternalClientMaxRetries, RetryBackoffBase: cfg.ExternalClientRetryBackoffBase, RetryBackoffMax: cfg.ExternalClientRetryBackoffMax,
			Breaker: circuitbreaker.Config{FailureThreshold: cfg.CircuitBreakerFailureThreshold, ObservationWindow: cfg.CircuitBreakerObservationWindow, OpenDuration: cfg.CircuitBreakerOpenDuration, HalfOpenProbes: cfg.CircuitBreakerHalfOpenProbes, SuccessThreshold: cfg.CircuitBreakerSuccessThreshold},
		})
	}
	safetyService := safetyusecases.NewService(safetyusecases.Options{
		Pool:       pool,
		References: safetyrepos.NewReferenceRepository(pool),
		Validator:  safetyValidator,
		Events:     publisher,
		Metrics:    metrics,
	})

	var dispatchRestrictions, haulingRestrictions platformclients.RestrictionAuthority
	if !cfg.IntegrationsKillSwitch {
		if dc := cfg.Clients["driver-dispatch-delivery-service"]; dc.Configured() {
			dispatchRestrictions = platformclients.NewRestrictionClient(platformclients.New(platformclients.Options{ServiceName: "driver-dispatch-delivery-service", BaseURL: dc.BaseURL, Secret: dc.Secret, Timeout: cfg.HTTPClientTimeout, Metrics: metrics, Environment: cfg.AppEnv, MaxRetries: cfg.ExternalClientMaxRetries, RetryBackoffBase: cfg.ExternalClientRetryBackoffBase, RetryBackoffMax: cfg.ExternalClientRetryBackoffMax, Breaker: circuitbreaker.Config{FailureThreshold: cfg.CircuitBreakerFailureThreshold, ObservationWindow: cfg.CircuitBreakerObservationWindow, OpenDuration: cfg.CircuitBreakerOpenDuration, HalfOpenProbes: cfg.CircuitBreakerHalfOpenProbes, SuccessThreshold: cfg.CircuitBreakerSuccessThreshold}}), "/api/v1/internal/restrictions")
		}
		if hc := cfg.Clients["driver-hauling-service"]; hc.Configured() {
			haulingRestrictions = platformclients.NewRestrictionClient(platformclients.New(platformclients.Options{ServiceName: "driver-hauling-service", BaseURL: hc.BaseURL, Secret: hc.Secret, Timeout: cfg.HTTPClientTimeout, Metrics: metrics, Environment: cfg.AppEnv, MaxRetries: cfg.ExternalClientMaxRetries, RetryBackoffBase: cfg.ExternalClientRetryBackoffBase, RetryBackoffMax: cfg.ExternalClientRetryBackoffMax, Breaker: circuitbreaker.Config{FailureThreshold: cfg.CircuitBreakerFailureThreshold, ObservationWindow: cfg.CircuitBreakerObservationWindow, OpenDuration: cfg.CircuitBreakerOpenDuration, HalfOpenProbes: cfg.CircuitBreakerHalfOpenProbes, SuccessThreshold: cfg.CircuitBreakerSuccessThreshold}}), "/api/v1/hauling/internal/restrictions")
		}
	}
	complianceService := compliancefeature.NewService(compliancefeature.Options{Pool: pool, Events: publisher, Dispatch: dispatchRestrictions, Hauling: haulingRestrictions, Flags: compliancefeature.Flags{Compliance: cfg.ComplianceEnabled, Restrictions: cfg.RestrictionPropagationEnabled, Workers: cfg.Section19WorkersEnabled, KillSwitch: cfg.IntegrationsKillSwitch}})
	financeService := BuildFinanceService(cfg, pool, metrics).WithEligibility(eligibilityService)
	restrictionGate := complianceRestrictionGate{service: complianceService}
	workforceService := workforcefeature.NewService(workforcefeature.Options{Pool: pool, Events: publisher, Media: contracts.Media, Restrictions: restrictionGate, Flags: workforcefeature.Flags{Workforce: cfg.WorkforceEnabled, Shifts: cfg.ShiftsEnabled, Leave: cfg.LeaveEnabled, Qualifications: cfg.QualificationsEnabled, Training: cfg.TrainingEnabled, Performance: cfg.PerformanceSnapshotsEnabled, Workers: cfg.Section17WorkersEnabled, KillSwitch: cfg.IntegrationsKillSwitch}})
	assetsService := assetsfeature.NewService(assetsfeature.Options{Pool: pool, Events: publisher, Media: contracts.Media, Restrictions: restrictionGate, Flags: assetsfeature.Flags{Assets: cfg.AssetsEnabled, Assignments: cfg.AssetAssignmentsEnabled, Inspections: cfg.InspectionsEnabled, Maintenance: cfg.MaintenanceEnabled, Breakdowns: cfg.BreakdownsEnabled, Inventory: cfg.PartsInventoryEnabled, Fuel: cfg.FuelRecordsEnabled, Workers: cfg.Section18WorkersEnabled, KillSwitch: cfg.IntegrationsKillSwitch}})
	privateOperations := BuildPrivateOperationsService(cfg, pool, publisher, metrics, workforceService, assetsService, restrictionGate)
	dispatchControl := BuildSection16Service(cfg, pool, publisher, privateOperations, eligibilityService, workforceService, assetsService, restrictionGate)
	assetsService.SetAlertSink(dispatchControl)

	safetyOperations := safetyfeature.NewOperationsService(safetyfeature.OperationsOptions{Pool: pool, Events: publisher, Media: contracts.Media, Flags: safetyfeature.OperationsFlags{Reports: cfg.SafetyReportsEnabled, Incidents: cfg.IncidentsEnabled, Workers: cfg.Section19WorkersEnabled, KillSwitch: cfg.IntegrationsKillSwitch}})
	proofService := prooffeature.NewService(prooffeature.Options{Pool: pool, Events: publisher, Media: contracts.Media, HashSecret: cfg.ProofHMACSecret, Flags: prooffeature.Flags{Policies: cfg.ProofPoliciesEnabled, Records: cfg.ProofRecordsEnabled, Workers: cfg.Section19WorkersEnabled, KillSwitch: cfg.IntegrationsKillSwitch}})
	specializedService := specializedfeature.NewService(specializedfeature.Options{Pool: pool, Events: publisher, Media: contracts.Media, Flags: specializedfeature.Flags{Enabled: cfg.SpecializedLogisticsEnabled, Workers: cfg.Section20WorkersEnabled, KillSwitch: cfg.IntegrationsKillSwitch, Modules: map[string]bool{specializedfeature.Courier: cfg.CourierLastMileEnabled, specializedfeature.Taxi: cfg.TaxiPassengerEnabled, specializedfeature.Hauling: cfg.TruckingHaulageEnabled, specializedfeature.Heavy: cfg.HeavySpecializedEnabled, specializedfeature.Freight: cfg.FreightForwardingEnabled, specializedfeature.ColdChain: cfg.ColdChainEnabled, specializedfeature.Medical: cfg.MedicalSensitiveEnabled, specializedfeature.Agricultural: cfg.AgriculturalFoodEnabled, specializedfeature.Moving: cfg.MovingRelocationEnabled, specializedfeature.Towing: cfg.TowingRecoveryEnabled}}})
	warehouseService := warehousefeature.NewService(warehousefeature.Options{Pool: pool, Events: publisher, Proof: proofService, Restrictions: restrictionGate, Flags: warehousefeature.Flags{Warehouses: cfg.WarehousesEnabled, Inventory: cfg.WarehouseInventoryEnabled, Fulfilment: cfg.FulfilmentEnabled, Dock: cfg.DockOperationsEnabled, Yard: cfg.YardOperationsEnabled, Terminal: cfg.TerminalTransfersEnabled, Returns: cfg.ReturnsEnabled, Workers: cfg.Section21WorkersEnabled, KillSwitch: cfg.IntegrationsKillSwitch}})
	walletHTTP := platformclients.New(platformclients.Options{ServiceName: "payment-wallet-service", BaseURL: walletCfg.BaseURL, Secret: walletCfg.Secret, Timeout: cfg.HTTPClientTimeout, Metrics: metrics, Environment: cfg.AppEnv, MaxRetries: cfg.ExternalClientMaxRetries, RetryBackoffBase: cfg.ExternalClientRetryBackoffBase, RetryBackoffMax: cfg.ExternalClientRetryBackoffMax, Breaker: circuitbreaker.Config{FailureThreshold: cfg.CircuitBreakerFailureThreshold, ObservationWindow: cfg.CircuitBreakerObservationWindow, OpenDuration: cfg.CircuitBreakerOpenDuration, HalfOpenProbes: cfg.CircuitBreakerHalfOpenProbes, SuccessThreshold: cfg.CircuitBreakerSuccessThreshold}})
	accountingCfg := cfg.Clients["accounting-service"]
	accountingHTTP := platformclients.New(platformclients.Options{ServiceName: "accounting-service", BaseURL: accountingCfg.BaseURL, Secret: accountingCfg.Secret, Timeout: cfg.HTTPClientTimeout, Metrics: metrics, Environment: cfg.AppEnv, MaxRetries: cfg.ExternalClientMaxRetries, RetryBackoffBase: cfg.ExternalClientRetryBackoffBase, RetryBackoffMax: cfg.ExternalClientRetryBackoffMax, Breaker: circuitbreaker.Config{FailureThreshold: cfg.CircuitBreakerFailureThreshold, ObservationWindow: cfg.CircuitBreakerObservationWindow, OpenDuration: cfg.CircuitBreakerOpenDuration, HalfOpenProbes: cfg.CircuitBreakerHalfOpenProbes, SuccessThreshold: cfg.CircuitBreakerSuccessThreshold}})
	notificationHTTP := platformclients.New(platformclients.Options{ServiceName: "notification-service", BaseURL: notificationCfg.BaseURL, Secret: notificationCfg.Secret, Timeout: cfg.HTTPClientTimeout, Metrics: metrics, Environment: cfg.AppEnv, MaxRetries: cfg.ExternalClientMaxRetries, RetryBackoffBase: cfg.ExternalClientRetryBackoffBase, RetryBackoffMax: cfg.ExternalClientRetryBackoffMax, Breaker: circuitbreaker.Config{FailureThreshold: cfg.CircuitBreakerFailureThreshold, ObservationWindow: cfg.CircuitBreakerObservationWindow, OpenDuration: cfg.CircuitBreakerOpenDuration, HalfOpenProbes: cfg.CircuitBreakerHalfOpenProbes, SuccessThreshold: cfg.CircuitBreakerSuccessThreshold}})
	supportHTTP := platformclients.New(platformclients.Options{ServiceName: "support-dispute-service", BaseURL: supportCfg.BaseURL, Secret: supportCfg.Secret, Timeout: cfg.HTTPClientTimeout, Metrics: metrics, Environment: cfg.AppEnv, MaxRetries: cfg.ExternalClientMaxRetries, RetryBackoffBase: cfg.ExternalClientRetryBackoffBase, RetryBackoffMax: cfg.ExternalClientRetryBackoffMax, Breaker: circuitbreaker.Config{FailureThreshold: cfg.CircuitBreakerFailureThreshold, ObservationWindow: cfg.CircuitBreakerObservationWindow, OpenDuration: cfg.CircuitBreakerOpenDuration, HalfOpenProbes: cfg.CircuitBreakerHalfOpenProbes, SuccessThreshold: cfg.CircuitBreakerSuccessThreshold}})
	financePosting := financialPostingAdapter{wallet: walletPostingAdapter{client: walletHTTP}, accounting: accountingPostingAdapter{client: accountingHTTP}}
	financeOperations := financeops.NewService(financeops.Options{Pool: pool, Events: publisher, Media: contracts.Media, Wallet: financePosting, Flags: financeops.Flags{Invoices: cfg.InvoicesEnabled, Receivables: cfg.ReceivablesEnabled, Expenses: cfg.ExpensesEnabled, Budgets: cfg.BudgetsEnabled, Compensation: cfg.CompensationEnabled, Payroll: cfg.PayrollEnabled, Settlements: cfg.SubcontractorSettlementsEnabled, Claims: cfg.ClaimsEnabled, Posting: cfg.FinancePostingEnabled, Workers: cfg.Section22WorkersEnabled, KillSwitch: cfg.IntegrationsKillSwitch}})
	engagementService := engagement.NewService(engagement.Options{Pool: pool, Events: publisher, Media: contracts.Media, Support: supportCaseAdapter{client: supportHTTP}, Notification: notificationDeliveryAdapter{client: notificationHTTP}, Flags: engagement.Flags{Communications: cfg.CommunicationsEnabled, NotificationRules: cfg.NotificationRulesEnabled, Support: cfg.SupportProjectionsEnabled, Quality: cfg.QualityCasesEnabled, SLA: cfg.SLAMeasurementsEnabled, Escalations: cfg.EscalationsEnabled, Workers: cfg.Section23WorkersEnabled, KillSwitch: cfg.IntegrationsKillSwitch}})
	authorizer := accesscontrol.NewAuthorizer(roles, entryCache, cfg.AuthorizationCacheTTL)
	searchService := BuildSearchService(cfg, pool, authorizer, metrics)
	shipmentsService := BuildShipmentsService(cfg, pool, metrics, publisher)
	exportsService := BuildExportsService(cfg, pool, publisher, metrics, searchService,
		shipmentsService, workforceService, assetsService, warehouseService,
		safetyOperations, financeOperations, engagementService)
	synchronizationService := synchronization.New(pool, synchronization.Flags{
		Enabled: cfg.SynchronizationEnabled, WorkerEnabled: cfg.SynchronizationWorkerEnabled,
		KillSwitch: cfg.SynchronizationKillSwitch || cfg.IntegrationsKillSwitch,
	})
	registerSection26Authorities(cfg, metrics, synchronizationService, supportCaseAdapter{client: supportHTTP})

	return &Components{
		Pool:    pool,
		Metrics: metrics,
		CompanyWallet: BuildCompanyWalletService(cfg, pool, metrics, publisher, approvalService,
			eligibilityService, financeService),
		CRM:               BuildCRMService(cfg, pool, metrics, publisher, approvalService),
		ServiceCatalog:    BuildServiceCatalogService(cfg, pool, metrics, publisher),
		Orders:            BuildOrdersService(cfg, pool, metrics, publisher, searchService),
		Shipments:         shipmentsService,
		Packages:          BuildPackagesService(cfg, pool, publisher, contracts.Media),
		Manifests:         BuildManifestsService(cfg, pool, publisher),
		RecurringOrders:   BuildRecurringOrdersService(cfg, pool, publisher, searchService),
		BulkImports:       BuildBulkImportsService(cfg, pool, publisher, metrics, workforceService, assetsService, warehouseService, searchService),
		PrivateOperations: privateOperations,
		DispatchControl:   dispatchControl,
		Workforce:         workforceService,
		Assets:            assetsService,
		Compliance:        complianceService,
		SafetyOperations:  safetyOperations,
		Proof:             proofService,
		Specialized:       specializedService,
		Warehouse:         warehouseService,
		FinanceOperations: financeOperations,
		Engagement:        engagementService,
		Marketplace:       BuildMarketplaceService(cfg, pool, metrics),
		Search:            searchService,
		Exports:           exportsService,
		Synchronization:   synchronizationService,
		Idempotency:       idempotency.NewStore(pool, 0),
		Limiter:           limiter,
		Documents:         BuildDocumentService(cfg, pool, metrics, publisher),
		Auth:              authService,
		Staff:             staffService,
		Organizations:     organizations,
		Providers:         providers,
		Resolver:          staffusecases.NewMembershipResolver(members, organizations),
		Authorizer:        authorizer,
		Sessions:          sessions,
		Challenges:        challenges,
		Invitations:       invitations,
		ProviderRepo:      providerRepo,
		Contracts:         contracts,
		Safety:            safetyService,
		Finance:           financeService,
		Eligibility:       eligibilityService,
		Approvals:         approvalService,
	}
}

type complianceRestrictionGate struct{ service *compliancefeature.Service }

func (g complianceRestrictionGate) Restricted(ctx context.Context, org, typ, ref string) (bool, error) {
	if g.service == nil {
		return false, errors.New("compliance restriction service unavailable")
	}
	v, err := g.service.CheckSubject(ctx, org, typ, ref)
	return !v.Eligible, err
}

// BuildDocumentService assembles the WP10 document domain.
//
// The media client is built only when it is fully configured. A nil client is
// deliberate rather than a placeholder: every path that needs an authoritative
// answer about a file then fails closed with "unavailable", which is honest,
// instead of proceeding on a guess.
func BuildDocumentService(cfg config.Config, pool *pgxpool.Pool,
	metrics *observability.Metrics, publisher documentusecases.EventPublisher) *documentusecases.Service {
	mediaCfg := cfg.Clients["media-file-service"]
	var media platformclients.MediaAuthority
	if !cfg.IntegrationsKillSwitch && mediaCfg.Configured() {
		media = platformclients.NewMediaFileClient(platformclients.Options{
			ServiceName: "media-file-service",
			BaseURL:     mediaCfg.BaseURL,
			Secret:      mediaCfg.Secret,
			Timeout:     cfg.HTTPClientTimeout,
			Metrics:     metrics,
			Environment: cfg.AppEnv,
			MaxRetries:  cfg.ExternalClientMaxRetries, RetryBackoffBase: cfg.ExternalClientRetryBackoffBase, RetryBackoffMax: cfg.ExternalClientRetryBackoffMax,
			Breaker: circuitbreaker.Config{FailureThreshold: cfg.CircuitBreakerFailureThreshold, ObservationWindow: cfg.CircuitBreakerObservationWindow, OpenDuration: cfg.CircuitBreakerOpenDuration, HalfOpenProbes: cfg.CircuitBreakerHalfOpenProbes, SuccessThreshold: cfg.CircuitBreakerSuccessThreshold},
		})
	}

	return documentusecases.NewService(documentusecases.Options{
		Pool:        pool,
		Documents:   documentrepos.NewRepository(pool),
		Media:       media,
		Events:      publisher,
		Eligibility: eligibilityrepos.NewRepository(pool),
		Memberships: NewDocumentsMembershipLookup(providerrepos.NewRepository(pool)),
		Metrics:     metrics,
		Flags: documentusecases.Flags{
			WritesEnabled:         cfg.DocumentsWritesEnabled,
			DownloadEnabled:       cfg.DocumentsDownloadEnabled,
			EventIngestionEnabled: cfg.DocumentsEventIngestionEnabled,
			KillSwitch:            cfg.DocumentsKillSwitch || cfg.IntegrationsKillSwitch,
		},
		StaleAfter:              cfg.DocumentProjectionStaleAfter,
		MaxDownloadBytes:        cfg.DocumentsMaxDownloadBytes,
		SelfVerificationAllowed: cfg.DocumentsSelfVerification,
	})
}

func BuildFinanceService(cfg config.Config, pool *pgxpool.Pool, metrics *observability.Metrics) *financefeature.Service {
	if cfg.IntegrationsKillSwitch || cfg.FinanceExecutionKillSwitch {
		return nil
	}
	walletCfg := cfg.Clients["payment-wallet-service"]
	if !walletCfg.Configured() {
		return nil
	}
	client := platformclients.NewWalletClient(platformclients.Options{ServiceName: "payment-wallet-service", BaseURL: walletCfg.BaseURL, Secret: walletCfg.Secret, Timeout: cfg.HTTPClientTimeout, Metrics: metrics, Environment: cfg.AppEnv, MaxRetries: cfg.ExternalClientMaxRetries, RetryBackoffBase: cfg.ExternalClientRetryBackoffBase, RetryBackoffMax: cfg.ExternalClientRetryBackoffMax, Breaker: circuitbreaker.Config{FailureThreshold: cfg.CircuitBreakerFailureThreshold, ObservationWindow: cfg.CircuitBreakerObservationWindow, OpenDuration: cfg.CircuitBreakerOpenDuration, HalfOpenProbes: cfg.CircuitBreakerHalfOpenProbes, SuccessThreshold: cfg.CircuitBreakerSuccessThreshold}})
	return financefeature.NewService(pool, financefeature.NewRepository(pool), client, events.NewDomainPublisher(events.NewOutboxRepository(pool)))
}

// Registrars builds the route registrars for the router.
func (c *Components) Registrars(cfg config.Config) (public RouteRegistrar, identity RouteRegistrar, tenantScoped RouteRegistrar) {
	authHandler := authhttp.NewHandler(c.Auth, cfg.ContactSecret, cfg.BrowserPolicy)
	corporateHandler := orghttp.NewHandler(c.Organizations, c.Staff, c.Providers, c.Authorizer, c.Sessions)

	public = func(group *gin.RouterGroup) {
		authHandler.RegisterPublicRoutes(group)
	}
	identity = func(group *gin.RouterGroup) {
		authHandler.RegisterAuthenticatedRoutes(group)
		corporateHandler.RegisterOnboardingRoutes(group)
	}
	safetyHandler := safetyhttp.NewHandler(c.Safety, c.Authorizer)
	// The finance operator routes are mounted whenever the service exists.
	// Where the Wallet client is unconfigured the routes still answer, and they
	// answer "unavailable" — an absent integration must be visible rather than
	// silently missing from the API surface.
	financeHandler := financefeature.NewOperatorHandler(c.Finance, c.Authorizer)
	eligibilityHandler := eligibilityhttp.NewHandler(c.Eligibility, c.Authorizer)
	approvalHandler := approvalhttp.NewHandler(c.Approvals, c.Authorizer)
	// The document routes are mounted whenever the service exists. Where the
	// media client is unconfigured they still answer, and they answer
	// "unavailable" — an absent integration must be visible rather than
	// silently missing from the API surface.
	documentHandler := documenthttp.NewHandler(c.Documents, c.Authorizer)
	// Section 13 routes are mounted whenever the service exists. With the flags
	// off they answer not-found rather than disappearing from the surface, so a
	// disabled capability is observable instead of indistinguishable from a
	// deployment that never had it.
	companyWalletHandler := wallethttp.NewHandler(c.CompanyWallet, c.Authorizer)
	// Section 14 routes follow the same rule: mounted whenever the service
	// exists, answering not-found with the flags off, so a disabled capability
	// is observable rather than indistinguishable from a deployment that never
	// had it.
	crmHandler := crmhttp.NewHandler(c.CRM, c.Authorizer)
	// Section 15 routes follow the same rule as Section 14: mounted whenever
	// the service exists, answering not-found with the flag off.
	catalogHandler := cataloghttp.NewHandler(c.ServiceCatalog, c.Authorizer)
	ordersHandler := ordershttp.NewHandler(c.Orders, c.Authorizer)
	shipmentsHandler := shipmentshttp.NewHandler(c.Shipments, c.Authorizer, c.Idempotency)
	packagesHandler := packageshttp.NewHandler(c.Packages, c.Authorizer, c.Idempotency)
	manifestsHandler := manifestshttp.NewHandler(c.Manifests, c.Authorizer, c.Idempotency)
	recurringHandler := recurringhttp.NewHandler(c.RecurringOrders, c.Authorizer, c.Idempotency)
	bulkHandler := bulkhttp.NewHandler(c.BulkImports, c.Authorizer, c.Idempotency)
	privateHandler := privatehttp.NewHandler(c.PrivateOperations, c.Authorizer, c.Idempotency)
	dispatchHandler := dispatchfeature.NewHandler(c.DispatchControl, c.Authorizer, c.Idempotency)
	workforceHandler := workforcefeature.NewHandler(c.Workforce, c.Authorizer, c.Idempotency)
	assetsHandler := assetsfeature.NewHandler(c.Assets, c.Authorizer, c.Idempotency)
	complianceHandler := compliancefeature.NewHandler(c.Compliance, c.Authorizer, c.Idempotency)
	safetyOperationsHandler := safetyfeature.NewOperationsHandler(c.SafetyOperations, c.Authorizer, c.Idempotency)
	proofHandler := prooffeature.NewHandler(c.Proof, c.Authorizer, c.Idempotency)
	specializedHandler := specializedfeature.NewHandler(c.Specialized, c.Authorizer, c.Idempotency)
	warehouseHandler := warehousefeature.NewHandler(c.Warehouse, c.Authorizer, c.Idempotency)
	financeOperationsHandler := financeops.NewHandler(c.FinanceOperations, c.Authorizer, c.Idempotency)
	engagementHandler := engagement.NewHandler(c.Engagement, c.Authorizer, c.Idempotency)
	searchHandler := searchhttp.NewHandler(c.Search, c.Authorizer)
	exportsHandler := exportshttp.NewHandler(c.Exports, c.Authorizer, c.Idempotency)
	synchronizationHandler := synchronization.NewHandler(c.Synchronization, c.Authorizer)
	// Section 27 background-jobs/scheduled-tasks operator surface. Mounted
	// whenever the pool exists, same rule as every other feature: a disabled
	// worker still leaves its job/task rows readable, so an operator can
	// always see queue state even with FLEET_PARTNER_WORKER_ENABLED off.
	backgroundOpsHandler := backgroundopshttp.NewHandler(jobs.NewQueue(c.Pool), scheduler.NewRepository(c.Pool), c.Authorizer)
	// Section 28 audit-log read surface. Mounted whenever the pool exists;
	// the underlying table and its append-only trigger have existed since
	// migration 000001, this is only the first HTTP read path onto it.
	auditHandler := audithttp.NewHandler(auditrepos.NewRepository(c.Pool), c.Authorizer)
	// Section 28 generic (non-document) legal hold. Mounted whenever the
	// pool exists, same rule as every other feature in this composition
	// root.
	// Section 28 step-up enforcement: both legal-hold and security-settings
	// mutations verify a caller-supplied step-up reference against the auth
	// feature's own authoritative grants (never a second, parallel MFA
	// check) whenever the target organization's own security settings say
	// MFA is required for sensitive actions -- see
	// docs/section28-security-privacy-audit.md.
	stepUpVerifier := stepUpAdapter{auth: c.Auth}
	securitySettingsService := securitysettingsusecases.NewService(c.Pool, stepUpVerifier)
	integrationService := integrationsfeature.NewService(integrationsfeature.Options{
		Pool: c.Pool, Config: cfg, Metrics: c.Metrics, StepUp: stepUpVerifier,
		SecurityPolicy: securitySettingsService,
	})
	c.Integrations = integrationService
	integrationHandler := integrationsfeature.NewHandler(integrationService, c.Authorizer)
	legalHoldService := legalholdusecases.NewService(c.Pool, stepUpVerifier, securitySettingsService)
	legalHoldHandler := legalholdhttp.NewHandler(legalHoldService, c.Authorizer)
	securitySettingsHandler := securitysettingshttp.NewHandler(securitySettingsService, c.Authorizer)
	retentionPolicyService := retentionpolicyusecases.NewService(c.Pool)
	retentionPolicyHandler := retentionpolicyhttp.NewHandler(retentionPolicyService, c.Authorizer)
	// Section 28 executable retention: real archive/anonymize execution
	// behind retentionPolicyService's existing configuration and
	// legalHoldService's existing hold system (both untouched -- see
	// internal/platform/retentionexec's package doc for the closed
	// executor allowlist this stays within). retentionRunner is also
	// exposed on Components so cmd/main.go can register its durable-job
	// handler on the worker when FLEET_PARTNER_RETENTION_EXECUTION_ENABLED
	// is set -- same opt-in-by-default posture as
	// ServiceCredentialScopingEnabled: a deployment that has not reviewed
	// which organizations/classes this would touch should see zero
	// behavior change.
	//
	// The three classes registered below each require a real external
	// confirmation dependency (see internal/platform/retentionexec's own
	// NewExpiredExportExecutor/NewSupersededProjectionExecutor/
	// NewSupportMetadataExecutor doc comments: a nil port makes
	// FindCandidates return nothing, fail closed, never silently
	// permissive) -- so each is registered ONLY when its real production
	// dependency is actually configured. c.Contracts.Media (built in
	// BuildComponents) is deliberately NOT reused here: it is typed
	// platformclients.MediaContract, a narrower interface (GetFile,
	// Configured only) than the RequestDeletion/ReadDeletionState surface
	// this executor needs, so a real *platformclients.MediaFileClient is
	// built fresh, the exact same construction BuildDocumentService
	// already uses for the identical reason. The support-dispute-service
	// client is likewise rebuilt fresh, mirroring BuildComponents' own
	// supportHTTP construction line for line -- Registrars already
	// reconstructs several services fresh from cfg/c.Pool rather than
	// reading BuildComponents' local variables (legalHoldService,
	// securitySettingsService, retentionPolicyService immediately above),
	// so this follows the method's own established convention rather than
	// inventing a new one. c.Synchronization IS reused directly (a real
	// Components field, already populated with registerSection26Authorities'
	// real authority registry). No test double anywhere in this block.
	// The three booleans are exposed on Components so cmd/main.go can
	// build a readiness check that fails closed -- not merely skips
	// registration silently -- when retention execution is enabled but
	// one of these is missing.
	retentionExecutors := []retentionexec.ClassExecutor{retentionexec.NewOrgContactExecutor()}
	retentionMediaCfg := cfg.Clients["media-file-service"]
	var retentionMedia retentionexec.ExportMediaDeleter
	if !cfg.IntegrationsKillSwitch && retentionMediaCfg.Configured() {
		retentionMedia = platformclients.NewMediaFileClient(platformclients.Options{
			ServiceName: "media-file-service", BaseURL: retentionMediaCfg.BaseURL,
			Secret: retentionMediaCfg.Secret, Timeout: cfg.HTTPClientTimeout, Metrics: c.Metrics, Environment: cfg.AppEnv,
			MaxRetries: cfg.ExternalClientMaxRetries, RetryBackoffBase: cfg.ExternalClientRetryBackoffBase, RetryBackoffMax: cfg.ExternalClientRetryBackoffMax,
			Breaker: circuitbreaker.Config{FailureThreshold: cfg.CircuitBreakerFailureThreshold, ObservationWindow: cfg.CircuitBreakerObservationWindow, OpenDuration: cfg.CircuitBreakerOpenDuration, HalfOpenProbes: cfg.CircuitBreakerHalfOpenProbes, SuccessThreshold: cfg.CircuitBreakerSuccessThreshold},
		})
	}
	c.RetentionExpiredExportReady = retentionMedia != nil && retentionMedia.Configured()
	if c.RetentionExpiredExportReady {
		retentionExecutors = append(retentionExecutors, retentionexec.NewExpiredExportExecutor(retentionMedia))
	}
	retentionSupportCfg := cfg.Clients["support-dispute-service"]
	retentionSupportHTTP := platformclients.New(platformclients.Options{
		ServiceName: "support-dispute-service", BaseURL: retentionSupportCfg.BaseURL,
		Secret: retentionSupportCfg.Secret, Timeout: cfg.HTTPClientTimeout, Metrics: c.Metrics, Environment: cfg.AppEnv,
		MaxRetries: cfg.ExternalClientMaxRetries, RetryBackoffBase: cfg.ExternalClientRetryBackoffBase, RetryBackoffMax: cfg.ExternalClientRetryBackoffMax,
		Breaker: circuitbreaker.Config{FailureThreshold: cfg.CircuitBreakerFailureThreshold, ObservationWindow: cfg.CircuitBreakerObservationWindow, OpenDuration: cfg.CircuitBreakerOpenDuration, HalfOpenProbes: cfg.CircuitBreakerHalfOpenProbes, SuccessThreshold: cfg.CircuitBreakerSuccessThreshold},
	})
	c.RetentionSupportMetadataReady = !cfg.IntegrationsKillSwitch && retentionSupportCfg.Configured()
	if c.RetentionSupportMetadataReady {
		retentionExecutors = append(retentionExecutors,
			retentionexec.NewSupportMetadataExecutor(retentionSupportAuthorityAdapter{port: supportCaseAdapter{client: retentionSupportHTTP}}))
	}
	// cfg.SynchronizationEnabled is checked in addition to
	// AuthorityConfigured(): registerSection26Authorities populates the
	// authority registry whenever media/support are configured,
	// independent of this flag (it gates only ApplyTx's own event
	// ingestion), so checking AuthorityConfigured() alone would let this
	// executor activate even for a deployment that has deliberately left
	// synchronization itself disabled -- a disabled base feature must
	// never implicitly enable a dependent one.
	c.RetentionSupersededProjectionReady = cfg.SynchronizationEnabled &&
		c.Synchronization != nil && c.Synchronization.AuthorityConfigured()
	if c.RetentionSupersededProjectionReady {
		retentionExecutors = append(retentionExecutors,
			retentionexec.NewSupersededProjectionExecutor(retentionSynchronizationAuthorityAdapter{service: c.Synchronization}))
	}
	retentionRunner := retentionexec.NewRunner(c.Pool,
		retentionExecutors,
		legalHoldService, retentionDaysAdapter{policy: retentionPolicyService}, nil, 0)
	// Exposed on the receiver (not just a local var) so cmd/main.go, which
	// calls Registrars before it wires up the job worker, can read
	// components.RetentionRunner to register the durable-job handler.
	c.RetentionRunner = retentionRunner
	retentionExecutionHandler := retentionexecutionhttp.NewHandler(
		retentionexec.NewAdministrator(c.Pool, retentionRunner, jobs.NewQueue(c.Pool),
			stepUpVerifier, securitySettingsService, nil),
		c.Authorizer)

	// ============================================================
	// DASHBOARD ROUTES - ADDED FOR PRODUCTION DASHBOARD
	// ============================================================
	dashboardService := dashboardusecases.NewService(c.Pool, c.DispatchControl)
	dashboardHandler := dashboardhttp.NewHandler(dashboardService, c.Authorizer)

	// ============================================================
	// MARKETPLACE CORPORATE ROUTES - FLEET MANAGER SURFACE
	//
	// The marketplace usecases and repositories have existed since the
	// feature was first written, but only the internal (HMAC service-to-
	// service) HTTP layer was ever mounted. This constructs the corporate
	// (Bearer + tenant-scoped) handler so the Fleet manager's web app can
	// list offers, accept or decline them for the organization, and
	// manage the resulting assignment.
	//
	// Feature flags: the write paths (accept, assign, cancel, reassign)
	// consult the marketplace Flags (OrganizationAcceptEnabled,
	// AssignmentEnabled, ReassignmentEnabled) on the Service. Those flags
	// are wired by BuildMarketplaceService; when they are off the write
	// endpoints return an "unavailable" error. The read endpoints do not
	// consult any flag and work as soon as the marketplace.read permission
	// is seeded.
	// ============================================================
	marketplaceHandler := marketplacehttp.NewHandler(c.Marketplace, c.Authorizer, c.Idempotency)

	tenantScoped = func(group *gin.RouterGroup) {
		corporateHandler.RegisterTenantRoutes(group)
		safetyHandler.RegisterTenantRoutes(group)
		financeHandler.RegisterTenantRoutes(group)
		eligibilityHandler.RegisterTenantRoutes(group)
		approvalHandler.RegisterTenantRoutes(group)
		documentHandler.RegisterTenantRoutes(group)
		companyWalletHandler.RegisterTenantRoutes(group)
		crmHandler.RegisterTenantRoutes(group)
		catalogHandler.RegisterTenantRoutes(group)
		ordersHandler.RegisterTenantRoutes(group)
		shipmentsHandler.RegisterTenantRoutes(group)
		packagesHandler.RegisterTenantRoutes(group)
		manifestsHandler.RegisterTenantRoutes(group)
		recurringHandler.RegisterTenantRoutes(group)
		bulkHandler.RegisterTenantRoutes(group)
		privateHandler.RegisterTenantRoutes(group)
		dispatchHandler.RegisterTenantRoutes(group)
		workforceHandler.RegisterTenantRoutes(group)
		assetsHandler.RegisterTenantRoutes(group)
		complianceHandler.RegisterTenantRoutes(group)
		safetyOperationsHandler.RegisterTenantRoutes(group)
		proofHandler.RegisterTenantRoutes(group)
		specializedHandler.RegisterTenantRoutes(group)
		warehouseHandler.RegisterTenantRoutes(group)
		financeOperationsHandler.RegisterTenantRoutes(group)
		engagementHandler.RegisterTenantRoutes(group)
		searchHandler.RegisterTenantRoutes(group)
		exportsHandler.RegisterTenantRoutes(group)
		synchronizationHandler.RegisterTenantRoutes(group)
		backgroundOpsHandler.RegisterTenantRoutes(group)
		auditHandler.RegisterTenantRoutes(group)
		legalHoldHandler.RegisterTenantRoutes(group)
		securitySettingsHandler.RegisterTenantRoutes(group)
		retentionPolicyHandler.RegisterTenantRoutes(group)
		retentionExecutionHandler.RegisterTenantRoutes(group)
		integrationHandler.RegisterTenantRoutes(group)
		// ============================================================
		// DASHBOARD ROUTES - REGISTERED HERE
		// ============================================================
		dashboardHandler.RegisterTenantRoutes(group)
		// ============================================================
		// MARKETPLACE CORPORATE ROUTES - REGISTERED HERE
		// ============================================================
		marketplaceHandler.RegisterTenantRoutes(group)
		// Rate limiting for every route registered above, including these
		// four, is no longer applied per-feature here: middleware.
		// GlobalRateLimit (internal/app/app.go, mounted once per route
		// class — public/tenantScoped/internal) now classifies every
		// request by internal/platform/riskregistry by its actual
		// method+FullPath(), covering the full ~250-route surface rather
		// than four hand-wrapped features. See
		// docs/section28-security-privacy-audit.md's "Rate limiting"
		// section for why per-route-class placement (not one single
		// router.Use) is required for identity-dimension keying to work.
	}
	return public, identity, tenantScoped
}

// OrganizationAPIRoutes is the separately authenticated organization-client
// route class. Management remains on the corporate bearer/tenant route class;
// a client credential can never call its own creation or rotation endpoints.
func (c *Components) OrganizationAPIRoutes() RouteRegistrar {
	return func(group *gin.RouterGroup) {
		if c.Integrations != nil {
			integrationsfeature.NewHandler(c.Integrations, nil).RegisterOrganizationAPIRoutes(group)
		}
	}
}

func (c *Components) OrganizationAPIAuth() gin.HandlerFunc {
	if c.Integrations == nil {
		return func(ctx *gin.Context) { ctx.AbortWithStatus(503) }
	}
	return c.Integrations.ClientAuthMiddleware(integrationsfeature.ScopeIntegrationContextRead)
}

// CustomerRoutes mounts the unauthenticated customer quotation routes.
//
// They live on their own route class because they are reached with a secure
// link rather than a corporate session: no bearer token, no tenant resolution,
// and the token itself is the entire authorization decision.
func (c *Components) CustomerRoutes() RouteRegistrar {
	handler := crmhttp.NewPublicHandler(c.CRM, c.Limiter)
	return func(group *gin.RouterGroup) {
		handler.RegisterPublicRoutes(group)
	}
}

// InternalCRMRoutes mounts the HMAC-authenticated CRM ingestion routes.
func (c *Components) InternalCRMRoutes() RouteRegistrar {
	handler := crmhttp.NewInternalHandler(c.CRM)
	return func(group *gin.RouterGroup) {
		handler.RegisterInternalRoutes(group)
	}
}

// InternalPrivateOperationsRoutes mounts provider decision routes only on the
// HMAC-authenticated service surface. Corporate users cannot reach them.
func (c *Components) InternalPrivateOperationsRoutes() RouteRegistrar {
	handler := privatehttp.NewHandler(c.PrivateOperations, c.Authorizer, c.Idempotency)
	return func(group *gin.RouterGroup) {
		handler.RegisterInternalRoutes(group)
	}
}

// InternalSection16Routes mounts authoritative Dispatch/Hauling tracking
// ingestion only on the HMAC-authenticated internal surface.
func (c *Components) InternalSection16Routes() RouteRegistrar {
	handler := dispatchfeature.NewHandler(c.DispatchControl, c.Authorizer, c.Idempotency)
	return func(group *gin.RouterGroup) { handler.RegisterInternalRoutes(group) }
}

// InternalSection22To23Routes mounts payment, support-event and SLA ingestion
// only on the HMAC-authenticated service surface.
func (c *Components) InternalSection22To23Routes() RouteRegistrar {
	financeHandler := financeops.NewInternalHandler(c.FinanceOperations)
	engagementHandler := engagement.NewInternalHandler(c.Engagement)
	return func(group *gin.RouterGroup) {
		financeHandler.RegisterInternalRoutes(group)
		engagementHandler.RegisterInternalRoutes(group)
	}
}

// redisCounter adapts go-redis to the rate-limit counter port with a
// mandatory TTL on every key.
type redisCounter struct {
	client *redis.Client
}

func (r redisCounter) Incr(ctx context.Context, key string, window time.Duration) (int64, error) {
	pipe := r.client.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, window)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incr.Val(), nil
}
