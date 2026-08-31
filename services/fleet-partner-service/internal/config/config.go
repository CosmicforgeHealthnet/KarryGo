// Package config loads and validates the service configuration.
//
// Mandatory settings fail startup when missing. Secrets are never printed.
// Env names follow the repository prefix convention (FLEET_PARTNER_*) with
// the common unprefixed vars APP_ENV, HTTP_ADDR and MIGRATION; the mapping
// from the specification's generic names is documented in
// docs/configuration.md.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/browserauth"
	"cosmicforge/logistics/shared/go/redisx"
)

// ServiceName is the canonical service identity used in logs, events,
// service-auth headers and metrics.
const ServiceName = "fleet-partner-service"

// CorporateRole is the access-token role carried by corporate users
// (organization owners and staff).
const CorporateRole = "organization_user"

// ClientConfig configures one downstream platform service client.
type ClientConfig struct {
	BaseURL string
	Secret  []byte
}

// ExecutionCapabilities is the single validated source gate used by HTTP
// use cases and workers. Taxi intentionally has no configurable true state.
type ExecutionCapabilities struct{ Dispatch, Taxi, Hauling bool }

func (c Config) ExecutionCapabilities() ExecutionCapabilities {
	if c.IntegrationsKillSwitch {
		return ExecutionCapabilities{}
	}
	dispatchConfigured := c.DispatchServiceURL != "" && len(c.DispatchServiceSecret) > 0
	hauling := c.Clients["driver-hauling-service"]
	return ExecutionCapabilities{
		Dispatch: c.DispatchExecutionStartEnabled && dispatchConfigured,
		Taxi:     false,
		Hauling:  c.HaulingIntegrationEnabled && hauling.Configured(),
	}
}

func (c ExecutionCapabilities) Any() bool { return c.Dispatch || c.Hauling }

// Configured reports whether the client can make live calls.
func (c ClientConfig) Configured() bool {
	return c.BaseURL != "" && len(c.Secret) > 0
}

// Config is the validated service configuration.
type Config struct {
	AppEnv        string
	BrowserPolicy browserauth.Policy
	HTTPAddr      string
	Migration     bool
	WorkerEnabled bool
	LogLevel      string

	DatabaseURL                string
	DatabaseMaxOpenConnections int32
	DatabaseMaxIdleConnections int32
	DatabaseConnectionMaxLife  time.Duration
	RedisRequired              bool
	Redis                      redisx.Config

	AccessTokenSecret []byte
	// ServiceSecrets holds per-caller HMAC secrets for internal routes
	// ("name=secret,..." format, parsed by shared serviceauth). Also parsed
	// by internal/platform/serviceauth.ParseVersionedSecrets, which
	// additionally accepts an optional "name:vN=secret" form for a rotated
	// version N -- a bare "name=secret" entry is always version 1, so this
	// single env var stays the one source of truth for every caller secret
	// whether or not ServiceCredentialScopingEnabled is on.
	ServiceSecrets string
	// ServiceCredentialScopingEnabled switches the internal route class from
	// single-secret HMAC verification (every caller in ServiceSecrets may
	// call any internal route, exactly today's behavior) to Section 28's
	// version-aware, route/method/event-scoped credential resolution
	// (fleet_partner_service_credentials, migration 000071). Default false:
	// this repository cannot enumerate every caller name a real, already-
	// running deployment might have configured beyond the ones its own
	// source code references (media-file-service, payment-wallet-service,
	// driver-dispatch-delivery-service, driver-taxi-service,
	// driver-hauling-service -- the five migration 000071 seeds
	// wildcard-scoped), so turning this on unconditionally could 403 an
	// undocumented caller this codebase has no way to know about. An
	// operator enables it deliberately, once every real caller has a
	// credential row (seeded ones already exist; see migration 000071).
	ServiceCredentialScopingEnabled bool
	// RetentionExecutionEnabled registers internal/platform/retentionexec's
	// durable job handler (retention.execute) on the worker. Default false:
	// EXECUTE-mode runs can still be INITIATED (the row is created,
	// PENDING) but never actually processed until an operator turns this
	// on -- real anonymize/archive mutations must be an explicit,
	// reviewed deployment decision, not a side effect of enabling the
	// worker in general. DRY_RUN previews run synchronously and are
	// unaffected by this flag either way.
	RetentionExecutionEnabled bool

	// Dispatch execution service, used by the execution-start worker to create
	// the canonical trip. Empty secret => the adapter is unconfigured and the
	// worker does not start.
	DispatchServiceURL     string
	DispatchServiceSecret  []byte
	DispatchServiceTimeout time.Duration
	// Cross-service execution gates default closed. The global switch wins over
	// every category flag and is intended for incident response.
	IntegrationsKillSwitch    bool
	HaulingIntegrationEnabled bool
	TaxiIntegrationEnabled    bool

	// Marketplace event ingestion and projection reconciliation.
	MarketplaceEventIngestionEnabled bool
	MarketplaceAllowedCallers        string
	MarketplaceReconciliationEnabled bool
	MarketplaceProjectionStaleAfter  time.Duration
	MarketplaceReconcileInterval     time.Duration
	MarketplaceReconcileBatchSize    int
	MarketplaceReconcileLease        time.Duration
	MarketplaceReconcileMaxAttempts  int
	MarketplaceReconcileBackoffBase  time.Duration
	MarketplaceReconcileBackoffMax   time.Duration
	MarketplaceReconcileSweepLimit   int

	// Section 27 assignment expiry and reassignment staleness workers.
	AssignmentExpiryWorkerEnabled bool
	AssignmentExpiryInterval      time.Duration
	AssignmentExpiryBatchSize     int
	ReassignmentWorkerEnabled     bool
	ReassignmentScanInterval      time.Duration
	ReassignmentStaleAfter        time.Duration
	// MarketplaceReassignmentEnabled gates the Reassign use case itself
	// (both the manual dispatcher-driven path and the automatic coordinator
	// below) -- this flag existed in mktusecases.Flags but was never wired
	// to any config source, so Reassign always refused with "not enabled"
	// in every deployment until now.
	MarketplaceReassignmentEnabled          bool
	MarketplaceAutomaticReassignmentEnabled bool
	AutomationRunnerWorkerEnabled           bool
	DataRetentionWorkerEnabled              bool
	DataRetentionScanInterval               time.Duration
	DataRetentionBatchSize                  int
	DataRetentionDryRun                     bool
	WorkerOperationsAllowedCallers          string

	// Execution-start worker.
	DispatchExecutionStartEnabled bool
	ExecutionStartBatchSize       int
	ExecutionStartLease           time.Duration
	ExecutionStartMaxAttempts     int
	ExecutionStartBackoffBase     time.Duration
	ExecutionStartInterval        time.Duration
	FinanceIntegrationEnabled     bool
	FinanceExecutionKillSwitch    bool
	FinanceWorkerBatchSize        int
	FinanceWorkerInterval         time.Duration
	FinanceWorkerLease            time.Duration
	FinanceWorkerMaxAttempts      int
	FinanceWorkerBackoff          time.Duration
	FinanceProjectionStaleAfter   time.Duration
	FinanceEventIngestionEnabled  bool
	FinanceAllowedCallers         string

	// --- WP10 document domain ---
	//
	// Every write and worker flag defaults false. A deployment that has not
	// configured the media client, the callers and the flags exposes the read
	// surface and refuses every state change, which is the correct posture for
	// a domain whose whole job is to be trustworthy.
	DocumentsWritesEnabled         bool
	DocumentsDownloadEnabled       bool
	DocumentsEventIngestionEnabled bool
	DocumentsKillSwitch            bool
	DocumentsAllowedCallers        string
	DocumentsSelfVerification      bool
	DocumentsMaxDownloadBytes      int64
	DocumentProjectionStaleAfter   time.Duration

	// DocumentsProviderAllowedCallers/DocumentsCustomerAllowedCallers gate the
	// provider-facing and customer-facing document read/download routes.
	// Separate from DocumentsAllowedCallers (media event ingestion): a caller
	// trusted to relay provider or customer identity is not automatically
	// trusted to publish media events, and vice versa.
	DocumentsProviderAllowedCallers string
	DocumentsCustomerAllowedCallers string

	// ProviderMembershipAllowedCallers gates the Section 30 internal
	// membership-summary read (providerMode, safe organization summary,
	// assignment policy, field-visibility policy for one provider) that
	// driver-hauling-service/driver-dispatch-delivery-service/
	// driver-taxi-service call to enrich their OWN login/session responses
	// -- fleet-partner-service never issues a provider session itself (it
	// owns no provider identity or auth, see README's "what it never
	// owns"). Same three-caller default as DocumentsProviderAllowedCallers,
	// since these are the same three owning provider services.
	ProviderMembershipAllowedCallers string

	DocumentReconcileEnabled     bool
	DocumentReconcileInterval    time.Duration
	DocumentReconcileBatchSize   int
	DocumentReconcileLease       time.Duration
	DocumentReconcileMaxAttempts int
	DocumentReconcileBackoff     time.Duration
	DocumentReconcileSweepLimit  int

	DocumentExpiryEnabled   bool
	DocumentExpiryInterval  time.Duration
	DocumentExpiryBatchSize int

	DocumentDeletionEnabled     bool
	DocumentDeletionInterval    time.Duration
	DocumentDeletionBatchSize   int
	DocumentDeletionLease       time.Duration
	DocumentDeletionBackoff     time.Duration
	DocumentDeletionMaxAttempts int

	// Section 13 company wallet, provider sub-wallet, allocation and
	// withdrawal switches.
	//
	// Every write and every worker defaults false. The read surface can be
	// enabled on its own, so an organization can see its wallet before anyone
	// is allowed to move money out of it.
	OrganizationWalletReadEnabled  bool
	ProviderSubWalletEnabled       bool
	ProviderAllocationEnabled      bool
	ProviderWithdrawalEnabled      bool
	WalletProjectionRefreshEnabled bool
	WalletReconciliationEnabled    bool
	CompanyWalletKillSwitch        bool
	CompanyWalletStaleAfter        time.Duration
	CompanyWalletWorkerInterval    time.Duration
	CompanyWalletWorkerBatchSize   int
	CompanyWalletWorkerMaxAttempts int

	// Section 14 CRM switches. Every write and every worker defaults false;
	// the read surface is enabled separately from the write surface so a
	// tenant can be given visibility without being given the ability to
	// commit to a customer.
	CRMEnabled                   bool
	CRMCustomerWritesEnabled     bool
	CRMContactWritesEnabled      bool
	CRMLocationWritesEnabled     bool
	CRMOpportunitiesEnabled      bool
	QuotationsEnabled            bool
	QuotationIssueEnabled        bool
	QuotationAcceptanceEnabled   bool
	ContractsEnabled             bool
	ContractActivationEnabled    bool
	SLAEnabled                   bool
	CustomerCreditEnabled        bool
	CRMKillSwitch                bool
	CRMNotificationWorkerEnabled bool
	QuotationExpiryWorkerEnabled bool
	ContractExpiryWorkerEnabled  bool
	ContractRenewalWorkerEnabled bool
	SLAEvaluationWorkerEnabled   bool
	SLAEscalationWorkerEnabled   bool

	CRMWorkerInterval    time.Duration
	CRMWorkerBatchSize   int
	CRMWorkerMaxAttempts int
	// How long an issued quotation's customer access token stays usable.
	QuotationTokenTTL time.Duration

	// Section 15 (WP14) switches: service catalogue and private orders. The
	// read surface is enabled separately from writes, matching the Section 14
	// convention, so a tenant can be given visibility without commit ability.
	ServiceCatalogEnabled       bool
	ServiceCatalogWritesEnabled bool
	OrdersEnabled               bool
	OrdersWritesEnabled         bool
	OrdersKillSwitch            bool
	// Slice A4: shipments and cargo items.
	ShipmentsEnabled       bool
	ShipmentsWritesEnabled bool
	ShipmentsKillSwitch    bool
	// Slices A5-A6: packages, scans and custody. All new writes default off.
	PackagesEnabled                 bool
	PackagesWritesEnabled           bool
	PackageScanningEnabled          bool
	PackagesKillSwitch              bool
	ManifestsEnabled                bool
	ManifestsWritesEnabled          bool
	ManifestsKillSwitch             bool
	RecurringOrdersEnabled          bool
	RecurringOrdersWritesEnabled    bool
	RecurringOrdersWorkerEnabled    bool
	RecurringOrdersKillSwitch       bool
	BulkImportsEnabled              bool
	BulkImportsWritesEnabled        bool
	BulkImportsWorkerEnabled        bool
	BulkImportsKillSwitch           bool
	BulkImportsMaxBytes             int64
	BulkImportsMaxRows              int
	ExportsEnabled                  bool
	ExportsWritesEnabled            bool
	ExportsWorkerEnabled            bool
	ExportsKillSwitch               bool
	ExportsMaxRows                  int
	ExportsRetentionHours           int
	ImportsEnabled                  bool
	ImportsWritesEnabled            bool
	ImportsValidationWorkerEnabled  bool
	ImportsCommitWorkerEnabled      bool
	ImportsRetryWorkerEnabled       bool
	PrivateOrdersEnabled            bool
	PrivateAssignmentsWritesEnabled bool
	PrivateAssignmentsWorkerEnabled bool
	PrivateDispatchExecutionEnabled bool
	PrivateHaulingExecutionEnabled  bool
	PrivateTaxiExecutionEnabled     bool
	PrivateOperationsKillSwitch     bool
	PrivateAssignmentResponseTTL    time.Duration
	PrivateExecutionWorkerEnabled   bool

	// Section 16 planning and live-control capabilities. Every write and every
	// worker defaults closed; the Fleet integration kill switch overrides all.
	DispatchPlanningEnabled     bool
	RoutePlanningEnabled        bool
	RouteOptimizationEnabled    bool
	RoutePublicationEnabled     bool
	ControlTowerEnabled         bool
	TrackingProjectionsEnabled  bool
	GeofencesEnabled            bool
	OperationalAlertsEnabled    bool
	Section16WorkersEnabled     bool
	WorkforceEnabled            bool
	ShiftsEnabled               bool
	LeaveEnabled                bool
	QualificationsEnabled       bool
	TrainingEnabled             bool
	PerformanceSnapshotsEnabled bool
	AssetsEnabled               bool
	AssetAssignmentsEnabled     bool
	InspectionsEnabled          bool
	MaintenanceEnabled          bool
	BreakdownsEnabled           bool
	PartsInventoryEnabled       bool
	FuelRecordsEnabled          bool
	Section17WorkersEnabled     bool
	Section18WorkersEnabled     bool
	Section16CandidateTTL       time.Duration
	Section16TrackingFreshAfter time.Duration
	Section16TrackingStaleAfter time.Duration
	Section16TrackingRetention  time.Duration
	Section16AlertSLA           time.Duration

	// Sections 19-21. Every write and worker defaults closed. Restrictions are
	// still read and enforced when creation/propagation flags are disabled.
	ComplianceEnabled, SafetyReportsEnabled, IncidentsEnabled bool
	ProofPoliciesEnabled, ProofRecordsEnabled                 bool
	RestrictionPropagationEnabled                             bool
	SpecializedLogisticsEnabled                               bool
	CourierLastMileEnabled, TaxiPassengerEnabled              bool
	TruckingHaulageEnabled                                    bool
	HeavySpecializedEnabled, FreightForwardingEnabled         bool
	ColdChainEnabled, MedicalSensitiveEnabled                 bool
	AgriculturalFoodEnabled, MovingRelocationEnabled          bool
	TowingRecoveryEnabled                                     bool
	WarehousesEnabled, WarehouseInventoryEnabled              bool
	FulfilmentEnabled, DockOperationsEnabled                  bool
	YardOperationsEnabled, TerminalTransfersEnabled           bool
	ReturnsEnabled                                            bool
	Section19WorkersEnabled, Section20WorkersEnabled          bool
	Section21WorkersEnabled                                   bool
	ProofHMACSecret                                           []byte

	// Sections 22-23. Writes, external posting and workers default closed.
	InvoicesEnabled, ReceivablesEnabled, ExpensesEnabled, BudgetsEnabled       bool
	CompensationEnabled, PayrollEnabled                                        bool
	SubcontractorSettlementsEnabled, ClaimsEnabled, FinancePostingEnabled      bool
	CommunicationsEnabled, NotificationRulesEnabled, SupportProjectionsEnabled bool
	QualityCasesEnabled, SLAMeasurementsEnabled, EscalationsEnabled            bool
	Section22WorkersEnabled, Section23WorkersEnabled                           bool

	// Section 25 (partial slice): search projection. Read-only surface;
	// indexing is a Go-level API other features call directly, not yet a
	// caller-facing write route, so there is no separate writes flag here.
	SearchEnabled                bool
	SynchronizationEnabled       bool
	SynchronizationWorkerEnabled bool
	SynchronizationKillSwitch    bool

	// Section 55 organization integration surface. Management routes remain
	// mounted when disabled so operators receive an explicit disabled response;
	// no credential can authenticate and no delivery worker can run until the
	// corresponding fail-closed flag and key material are configured.
	OrganizationAPIClientsEnabled      bool
	WebhooksEnabled                    bool
	WebhookDeliveryWorkerEnabled       bool
	APIClientHashKey                   []byte
	WebhookSecretEncryptionKey         []byte
	WebhookAllowHTTP                   bool
	WebhookPrivateDestinationAllowlist []string
	WebhookDeliveryTimeout             time.Duration
	WebhookReplayWindow                time.Duration
	WebhookRotationOverlapMax          time.Duration
	WebhookDisableAfterFailures        int
	WebhookDeliveryMaxAttempts         int
	WebhookFanoutInterval              time.Duration
	WebhookFanoutBatchSize             int

	MetricsEnabled   bool
	MetricsAuthToken string

	Clients map[string]ClientConfig

	HTTPClientTimeout  time.Duration
	ShutdownTimeout    time.Duration
	OutboxPollInterval time.Duration
	// OutboxStreamKey is the Redis stream the outbox worker delivers to when
	// a Redis client is configured. Empty defaults to events.DefaultStreamKey.
	OutboxStreamKey      string
	JobPollInterval      time.Duration
	JobLeaseDuration     time.Duration
	JobHeartbeatInterval time.Duration

	MaxRequestBodyBytes int64
	// TrustedProxies lists the IPs/CIDRs whose X-Forwarded-For header gin
	// will honor when computing ClientIP() for rate-limit keys and safe
	// logging. Empty (the default) means trust none: gin uses the raw
	// connection's RemoteAddr only. An arbitrary caller can set any
	// X-Forwarded-For value it likes, so trusting it without an explicit,
	// operator-configured proxy list would let that caller pick which
	// rate-limit bucket its own requests land in.
	TrustedProxies []string

	// --- Section 8–10 authentication settings ---

	// OTPSecret peppers the keyed OTP digest. Mandatory: without it, a
	// six-digit code would be brute-forceable from a leaked table.
	OTPSecret []byte
	// SessionSecret hashes refresh tokens and invitation tokens.
	SessionSecret []byte
	// ContactSecret hashes email/phone for rate limits and security events.
	ContactSecret []byte

	OTPLength         int
	OTPExpiry         time.Duration
	OTPMaxAttempts    int
	OTPResendCooldown time.Duration
	OTPMaxPerHour     int

	AccessTokenTTL        time.Duration
	RefreshTokenTTL       time.Duration
	StepUpTTL             time.Duration
	StaffInvitationTTL    time.Duration
	ProviderInvitationTTL time.Duration

	DefaultCountryCode    string
	StaffAcceptURL        string
	ProviderAcceptURL     string
	AuthorizationCacheTTL time.Duration

	// --- Section 29 reliability, observability and operational readiness ---

	// TraceSampleRate controls what fraction of NEW root traces (no inbound
	// traceparent) are marked sampled; a trace propagated from an upstream
	// caller always keeps the upstream's own decision regardless of this
	// value. See internal/platform/tracing.
	TraceSampleRate float64

	// External-client automatic retry: classified-transient outcomes only
	// (internal/platformclients.Outcome.Retryable), bounded attempts with
	// exponential backoff and jitter. See internal/platformclients/retry.go.
	ExternalClientMaxRetries       int
	ExternalClientRetryBackoffBase time.Duration
	ExternalClientRetryBackoffMax  time.Duration

	// Per-downstream-service circuit breaker tuning
	// (internal/platform/circuitbreaker), applied by internal/platformclients.
	// State is in-memory only: a restart resets to CLOSED, which is the safe
	// default and needs no durable persistence.
	CircuitBreakerFailureThreshold  int
	CircuitBreakerObservationWindow time.Duration
	CircuitBreakerOpenDuration      time.Duration
	CircuitBreakerHalfOpenProbes    int
	CircuitBreakerSuccessThreshold  int

	// Readiness backlog/staleness thresholds: /ready fails closed when an
	// enabled, mandatory subsystem crosses one of these bounds. See
	// cmd/main.go's readiness wiring.
	ReadinessOutboxStaleAfter      time.Duration
	ReadinessOutboxBacklogCritical int64
	ReadinessJobDeadLetterCritical int64
	ReadinessStaleLeaseThreshold   time.Duration
}

// Load reads, validates and returns the configuration. It returns an error
// (never logs secret values) when mandatory settings are missing or invalid.
func Load() (Config, error) {
	cfg := Config{
		AppEnv:        getEnv("APP_ENV", "development"),
		HTTPAddr:      getEnv("HTTP_ADDR", ":8113"),
		Migration:     getEnvBool("MIGRATION", false),
		WorkerEnabled: getEnvBool("FLEET_PARTNER_WORKER_ENABLED", true),
		LogLevel:      getEnv("FLEET_PARTNER_LOG_LEVEL", "info"),

		DatabaseURL:                getEnv("FLEET_PARTNER_DATABASE_URL", ""),
		DatabaseMaxOpenConnections: int32(getEnvInt("FLEET_PARTNER_DATABASE_MAX_OPEN_CONNECTIONS", 10)),
		DatabaseMaxIdleConnections: int32(getEnvInt("FLEET_PARTNER_DATABASE_MAX_IDLE_CONNECTIONS", 2)),
		DatabaseConnectionMaxLife:  getEnvDuration("FLEET_PARTNER_DATABASE_CONNECTION_MAX_LIFETIME", time.Hour),

		RedisRequired: getEnvBool("FLEET_PARTNER_REDIS_REQUIRED", false),
		Redis: redisx.Config{
			Addr:     getEnv("FLEET_PARTNER_REDIS_ADDR", ""),
			Password: os.Getenv("FLEET_PARTNER_REDIS_PASSWORD"),
			DB:       getEnvInt("FLEET_PARTNER_REDIS_DB", 0),
		},

		AccessTokenSecret:               []byte(getEnv("FLEET_PARTNER_ACCESS_TOKEN_SECRET", "")),
		ServiceSecrets:                  getEnv("FLEET_PARTNER_SERVICE_SECRETS", ""),
		ServiceCredentialScopingEnabled: getEnvBool("FLEET_PARTNER_SERVICE_CREDENTIAL_SCOPING_ENABLED", false),
		RetentionExecutionEnabled:       getEnvBool("FLEET_PARTNER_RETENTION_EXECUTION_ENABLED", false),

		DispatchServiceURL:        getEnv("DISPATCH_SERVICE_URL", ""),
		DispatchServiceSecret:     []byte(getEnv("DISPATCH_SERVICE_SECRET", "")),
		DispatchServiceTimeout:    time.Duration(getEnvInt("DISPATCH_SERVICE_TIMEOUT_SECONDS", 10)) * time.Second,
		IntegrationsKillSwitch:    getEnvBool("FLEET_INTEGRATIONS_KILL_SWITCH", false),
		HaulingIntegrationEnabled: getEnvBool("FLEET_HAULING_INTEGRATION_ENABLED", false),
		TaxiIntegrationEnabled:    getEnvBool("FLEET_TAXI_INTEGRATION_ENABLED", false),

		MarketplaceEventIngestionEnabled: getEnvBool("FLEET_MARKETPLACE_EVENT_INGESTION_ENABLED", false),
		MarketplaceAllowedCallers:        getEnv("FLEET_MARKETPLACE_ALLOWED_CALLERS", ""),
		MarketplaceReconciliationEnabled: getEnvBool("FLEET_MARKETPLACE_RECONCILIATION_ENABLED", false),
		MarketplaceProjectionStaleAfter:  time.Duration(getEnvInt("FLEET_MARKETPLACE_PROJECTION_STALE_AFTER_SECONDS", 900)) * time.Second,
		MarketplaceReconcileInterval:     time.Duration(getEnvInt("FLEET_MARKETPLACE_RECONCILE_INTERVAL_SECONDS", 30)) * time.Second,
		MarketplaceReconcileBatchSize:    getEnvInt("FLEET_MARKETPLACE_RECONCILE_BATCH_SIZE", 25),
		MarketplaceReconcileLease:        time.Duration(getEnvInt("FLEET_MARKETPLACE_RECONCILE_LEASE_SECONDS", 120)) * time.Second,
		MarketplaceReconcileMaxAttempts:  getEnvInt("FLEET_MARKETPLACE_RECONCILE_MAX_ATTEMPTS", 8),
		MarketplaceReconcileBackoffBase:  time.Duration(getEnvInt("FLEET_MARKETPLACE_RECONCILE_BACKOFF_BASE_SECONDS", 10)) * time.Second,
		MarketplaceReconcileBackoffMax:   time.Duration(getEnvInt("FLEET_MARKETPLACE_RECONCILE_BACKOFF_MAX_SECONDS", 900)) * time.Second,
		MarketplaceReconcileSweepLimit:   getEnvInt("FLEET_MARKETPLACE_RECONCILE_SWEEP_LIMIT", 500),

		AssignmentExpiryWorkerEnabled:           getEnvBool("ASSIGNMENT_EXPIRY_WORKER_ENABLED", false),
		AssignmentExpiryInterval:                time.Duration(getEnvInt("FLEET_ASSIGNMENT_EXPIRY_INTERVAL_SECONDS", 15)) * time.Second,
		AssignmentExpiryBatchSize:               getEnvInt("FLEET_ASSIGNMENT_EXPIRY_BATCH_SIZE", 50),
		ReassignmentWorkerEnabled:               getEnvBool("REASSIGNMENT_WORKER_ENABLED", false),
		ReassignmentScanInterval:                time.Duration(getEnvInt("FLEET_REASSIGNMENT_SCAN_INTERVAL_SECONDS", 60)) * time.Second,
		ReassignmentStaleAfter:                  time.Duration(getEnvInt("FLEET_REASSIGNMENT_STALE_AFTER_SECONDS", 300)) * time.Second,
		MarketplaceReassignmentEnabled:          getEnvBool("FLEET_REASSIGNMENT_ENABLED", false),
		MarketplaceAutomaticReassignmentEnabled: getEnvBool("FLEET_AUTOMATIC_REASSIGNMENT_ENABLED", false),
		AutomationRunnerWorkerEnabled:           getEnvBool("AUTOMATION_RUNNER_WORKER_ENABLED", false),
		DataRetentionWorkerEnabled:              getEnvBool("DATA_RETENTION_WORKER_ENABLED", false),
		DataRetentionScanInterval:               time.Duration(getEnvInt("FLEET_DATA_RETENTION_SCAN_INTERVAL_SECONDS", 3600)) * time.Second,
		DataRetentionBatchSize:                  getEnvInt("FLEET_DATA_RETENTION_BATCH_SIZE", 200),
		DataRetentionDryRun:                     getEnvBool("FLEET_DATA_RETENTION_DRY_RUN", true),
		WorkerOperationsAllowedCallers:          getEnv("WORKER_OPERATIONS_ALLOWED_CALLERS", ""),

		DispatchExecutionStartEnabled: getEnvBool("FLEET_DISPATCH_EXECUTION_START_ENABLED", false),
		ExecutionStartBatchSize:       getEnvInt("FLEET_EXECUTION_START_BATCH_SIZE", 10),
		ExecutionStartLease:           time.Duration(getEnvInt("FLEET_EXECUTION_START_LEASE_SECONDS", 120)) * time.Second,
		ExecutionStartMaxAttempts:     getEnvInt("FLEET_EXECUTION_START_MAX_ATTEMPTS", 8),
		ExecutionStartBackoffBase:     time.Duration(getEnvInt("FLEET_EXECUTION_START_BACKOFF_SECONDS", 5)) * time.Second,
		ExecutionStartInterval:        time.Duration(getEnvInt("FLEET_EXECUTION_START_INTERVAL_SECONDS", 5)) * time.Second,
		FinanceIntegrationEnabled:     getEnvBool("FLEET_FINANCE_INTEGRATION_ENABLED", false),
		FinanceExecutionKillSwitch:    getEnvBool("FLEET_FINANCE_EXECUTION_KILL_SWITCH", false),
		FinanceWorkerBatchSize:        getEnvInt("FLEET_FINANCE_WORKER_BATCH_SIZE", 25),
		FinanceWorkerInterval:         time.Duration(getEnvInt("FLEET_FINANCE_WORKER_INTERVAL_SECONDS", 5)) * time.Second,
		FinanceWorkerLease:            time.Duration(getEnvInt("FLEET_FINANCE_WORKER_LEASE_SECONDS", 30)) * time.Second,
		FinanceWorkerMaxAttempts:      getEnvInt("FLEET_FINANCE_WORKER_MAX_ATTEMPTS", 8),
		FinanceWorkerBackoff:          time.Duration(getEnvInt("FLEET_FINANCE_WORKER_BACKOFF_SECONDS", 2)) * time.Second,
		FinanceProjectionStaleAfter:   time.Duration(getEnvInt("FLEET_FINANCE_PROJECTION_STALE_SECONDS", 900)) * time.Second,
		FinanceEventIngestionEnabled:  getEnvBool("FLEET_FINANCE_EVENT_INGESTION_ENABLED", false),
		FinanceAllowedCallers:         getEnv("FLEET_FINANCE_ALLOWED_CALLERS", "payment-wallet-service"),

		DocumentsWritesEnabled:         getEnvBool("FLEET_DOCUMENTS_WRITES_ENABLED", false),
		DocumentsDownloadEnabled:       getEnvBool("FLEET_DOCUMENTS_DOWNLOAD_ENABLED", false),
		DocumentsEventIngestionEnabled: getEnvBool("FLEET_DOCUMENTS_EVENT_INGESTION_ENABLED", false),
		DocumentsKillSwitch:            getEnvBool("FLEET_DOCUMENTS_KILL_SWITCH", false),
		DocumentsAllowedCallers:        getEnv("FLEET_DOCUMENTS_ALLOWED_CALLERS", "media-file-service"),
		DocumentsSelfVerification:      getEnvBool("FLEET_DOCUMENTS_SELF_VERIFICATION_ALLOWED", false),
		DocumentsMaxDownloadBytes:      int64(getEnvInt("FLEET_DOCUMENTS_MAX_DOWNLOAD_BYTES", 32<<20)),
		DocumentProjectionStaleAfter:   time.Duration(getEnvInt("FLEET_DOCUMENTS_PROJECTION_STALE_SECONDS", 900)) * time.Second,

		DocumentsProviderAllowedCallers: getEnv("FLEET_DOCUMENTS_PROVIDER_ALLOWED_CALLERS",
			"driver-dispatch-delivery-service,driver-taxi-service,driver-hauling-service"),
		DocumentsCustomerAllowedCallers: getEnv("FLEET_DOCUMENTS_CUSTOMER_ALLOWED_CALLERS", ""),

		ProviderMembershipAllowedCallers: getEnv("PROVIDER_MEMBERSHIP_ALLOWED_CALLERS",
			"driver-dispatch-delivery-service,driver-taxi-service,driver-hauling-service"),

		DocumentReconcileEnabled:     getEnvBool("FLEET_DOCUMENTS_RECONCILIATION_ENABLED", false),
		DocumentReconcileInterval:    time.Duration(getEnvInt("FLEET_DOCUMENTS_RECONCILE_INTERVAL_SECONDS", 30)) * time.Second,
		DocumentReconcileBatchSize:   getEnvInt("FLEET_DOCUMENTS_RECONCILE_BATCH_SIZE", 25),
		DocumentReconcileLease:       time.Duration(getEnvInt("FLEET_DOCUMENTS_RECONCILE_LEASE_SECONDS", 120)) * time.Second,
		DocumentReconcileMaxAttempts: getEnvInt("FLEET_DOCUMENTS_RECONCILE_MAX_ATTEMPTS", 8),
		DocumentReconcileBackoff:     time.Duration(getEnvInt("FLEET_DOCUMENTS_RECONCILE_BACKOFF_SECONDS", 30)) * time.Second,
		DocumentReconcileSweepLimit:  getEnvInt("FLEET_DOCUMENTS_RECONCILE_SWEEP_LIMIT", 200),

		DocumentExpiryEnabled:   getEnvBool("FLEET_DOCUMENTS_EXPIRY_WORKER_ENABLED", false),
		DocumentExpiryInterval:  time.Duration(getEnvInt("FLEET_DOCUMENTS_EXPIRY_INTERVAL_SECONDS", 3600)) * time.Second,
		DocumentExpiryBatchSize: getEnvInt("FLEET_DOCUMENTS_EXPIRY_BATCH_SIZE", 200),

		DocumentDeletionEnabled:     getEnvBool("FLEET_DOCUMENTS_DELETION_WORKER_ENABLED", false),
		DocumentDeletionInterval:    time.Duration(getEnvInt("FLEET_DOCUMENTS_DELETION_INTERVAL_SECONDS", 60)) * time.Second,
		DocumentDeletionBatchSize:   getEnvInt("FLEET_DOCUMENTS_DELETION_BATCH_SIZE", 25),
		DocumentDeletionLease:       time.Duration(getEnvInt("FLEET_DOCUMENTS_DELETION_LEASE_SECONDS", 120)) * time.Second,
		DocumentDeletionBackoff:     time.Duration(getEnvInt("FLEET_DOCUMENTS_DELETION_BACKOFF_SECONDS", 60)) * time.Second,
		DocumentDeletionMaxAttempts: getEnvInt("FLEET_DOCUMENTS_DELETION_MAX_ATTEMPTS", 8),

		OrganizationWalletReadEnabled:  getEnvBool("ORGANIZATION_WALLET_READ_ENABLED", false),
		ProviderSubWalletEnabled:       getEnvBool("PROVIDER_SUBWALLET_ENABLED", false),
		ProviderAllocationEnabled:      getEnvBool("PROVIDER_ALLOCATION_ENABLED", false),
		ProviderWithdrawalEnabled:      getEnvBool("PROVIDER_WITHDRAWAL_ENABLED", false),
		WalletProjectionRefreshEnabled: getEnvBool("WALLET_PROJECTION_REFRESH_ENABLED", false),
		WalletReconciliationEnabled:    getEnvBool("WALLET_RECONCILIATION_ENABLED", false),
		CompanyWalletKillSwitch:        getEnvBool("FLEET_COMPANY_WALLET_KILL_SWITCH", false),
		CompanyWalletStaleAfter:        time.Duration(getEnvInt("FLEET_COMPANY_WALLET_STALE_SECONDS", 900)) * time.Second,
		CompanyWalletWorkerInterval:    time.Duration(getEnvInt("FLEET_COMPANY_WALLET_WORKER_INTERVAL_SECONDS", 10)) * time.Second,
		CompanyWalletWorkerBatchSize:   getEnvInt("FLEET_COMPANY_WALLET_WORKER_BATCH_SIZE", 25),
		CompanyWalletWorkerMaxAttempts: getEnvInt("FLEET_COMPANY_WALLET_WORKER_MAX_ATTEMPTS", 8),

		CRMEnabled:                   getEnvBool("CRM_ENABLED", false),
		CRMCustomerWritesEnabled:     getEnvBool("CRM_CUSTOMER_WRITES_ENABLED", false),
		CRMContactWritesEnabled:      getEnvBool("CRM_CONTACT_WRITES_ENABLED", false),
		CRMLocationWritesEnabled:     getEnvBool("CRM_LOCATION_WRITES_ENABLED", false),
		CRMOpportunitiesEnabled:      getEnvBool("CRM_OPPORTUNITIES_ENABLED", false),
		QuotationsEnabled:            getEnvBool("QUOTATIONS_ENABLED", false),
		QuotationIssueEnabled:        getEnvBool("QUOTATION_ISSUE_ENABLED", false),
		QuotationAcceptanceEnabled:   getEnvBool("QUOTATION_ACCEPTANCE_ENABLED", false),
		ContractsEnabled:             getEnvBool("CONTRACTS_ENABLED", false),
		ContractActivationEnabled:    getEnvBool("CONTRACT_ACTIVATION_ENABLED", false),
		SLAEnabled:                   getEnvBool("SLA_ENABLED", false),
		CustomerCreditEnabled:        getEnvBool("CUSTOMER_CREDIT_ENABLED", false),
		CRMKillSwitch:                getEnvBool("FLEET_CRM_KILL_SWITCH", false),
		CRMNotificationWorkerEnabled: getEnvBool("CRM_NOTIFICATION_WORKER_ENABLED", false),
		QuotationExpiryWorkerEnabled: getEnvBool("QUOTATION_EXPIRY_WORKER_ENABLED", false),
		ContractExpiryWorkerEnabled:  getEnvBool("CONTRACT_EXPIRY_WORKER_ENABLED", false),
		ContractRenewalWorkerEnabled: getEnvBool("CONTRACT_RENEWAL_WORKER_ENABLED", false),
		SLAEvaluationWorkerEnabled:   getEnvBool("SLA_EVALUATION_WORKER_ENABLED", false),
		SLAEscalationWorkerEnabled:   getEnvBool("SLA_ESCALATION_WORKER_ENABLED", false),

		CRMWorkerInterval:    time.Duration(getEnvInt("FLEET_CRM_WORKER_INTERVAL_SECONDS", 30)) * time.Second,
		CRMWorkerBatchSize:   getEnvInt("FLEET_CRM_WORKER_BATCH_SIZE", 25),
		CRMWorkerMaxAttempts: getEnvInt("FLEET_CRM_WORKER_MAX_ATTEMPTS", 8),
		QuotationTokenTTL:    time.Duration(getEnvInt("FLEET_QUOTATION_TOKEN_TTL_SECONDS", 1209600)) * time.Second,

		ServiceCatalogEnabled:           getEnvBool("SERVICE_CATALOG_ENABLED", false),
		ServiceCatalogWritesEnabled:     getEnvBool("SERVICE_CATALOG_WRITES_ENABLED", false),
		OrdersEnabled:                   getEnvBool("ORDERS_ENABLED", false),
		OrdersWritesEnabled:             getEnvBool("ORDERS_WRITES_ENABLED", false),
		OrdersKillSwitch:                getEnvBool("FLEET_ORDERS_KILL_SWITCH", false),
		ShipmentsEnabled:                getEnvBool("SHIPMENTS_ENABLED", false),
		ShipmentsWritesEnabled:          getEnvBool("SHIPMENTS_WRITES_ENABLED", false),
		ShipmentsKillSwitch:             getEnvBool("FLEET_SHIPMENTS_KILL_SWITCH", false),
		PackagesEnabled:                 getEnvBool("PACKAGES_ENABLED", false),
		PackagesWritesEnabled:           getEnvBool("PACKAGES_WRITES_ENABLED", false),
		PackageScanningEnabled:          getEnvBool("PACKAGE_SCANNING_ENABLED", false),
		PackagesKillSwitch:              getEnvBool("FLEET_PACKAGES_KILL_SWITCH", false),
		ManifestsEnabled:                getEnvBool("MANIFESTS_ENABLED", false),
		ManifestsWritesEnabled:          getEnvBool("MANIFESTS_WRITES_ENABLED", false),
		ManifestsKillSwitch:             getEnvBool("FLEET_MANIFESTS_KILL_SWITCH", false),
		RecurringOrdersEnabled:          getEnvBool("RECURRING_ORDERS_ENABLED", false),
		RecurringOrdersWritesEnabled:    getEnvBool("RECURRING_ORDERS_WRITES_ENABLED", false),
		RecurringOrdersWorkerEnabled:    getEnvBool("RECURRING_ORDERS_WORKER_ENABLED", false),
		RecurringOrdersKillSwitch:       getEnvBool("FLEET_RECURRING_ORDERS_KILL_SWITCH", false),
		BulkImportsEnabled:              getEnvBool("BULK_IMPORTS_ENABLED", false),
		BulkImportsWritesEnabled:        getEnvBool("BULK_IMPORTS_WRITES_ENABLED", false),
		BulkImportsWorkerEnabled:        getEnvBool("BULK_IMPORTS_WORKER_ENABLED", false),
		BulkImportsKillSwitch:           getEnvBool("FLEET_BULK_IMPORTS_KILL_SWITCH", false),
		BulkImportsMaxBytes:             int64(getEnvInt("FLEET_BULK_IMPORT_MAX_BYTES", 10485760)),
		BulkImportsMaxRows:              getEnvInt("FLEET_BULK_IMPORT_MAX_ROWS", 5000),
		ExportsEnabled:                  getEnvBool("EXPORTS_ENABLED", false),
		ExportsWritesEnabled:            getEnvBool("EXPORTS_WRITES_ENABLED", false),
		ExportsWorkerEnabled:            getEnvBool("EXPORTS_WORKER_ENABLED", false),
		ExportsKillSwitch:               getEnvBool("FLEET_EXPORTS_KILL_SWITCH", false),
		ExportsMaxRows:                  getEnvInt("FLEET_EXPORTS_MAX_ROWS", 50000),
		ExportsRetentionHours:           getEnvInt("FLEET_EXPORTS_RETENTION_HOURS", 168),
		ImportsEnabled:                  getEnvBool("IMPORTS_ENABLED", false),
		ImportsWritesEnabled:            getEnvBool("IMPORTS_WRITES_ENABLED", false),
		ImportsValidationWorkerEnabled:  getEnvBool("IMPORTS_VALIDATION_WORKER_ENABLED", false),
		ImportsCommitWorkerEnabled:      getEnvBool("IMPORTS_COMMIT_WORKER_ENABLED", false),
		ImportsRetryWorkerEnabled:       getEnvBool("IMPORTS_RETRY_WORKER_ENABLED", false),
		PrivateOrdersEnabled:            getEnvBool("PRIVATE_ORDERS_ENABLED", false),
		PrivateAssignmentsWritesEnabled: getEnvBool("PRIVATE_ASSIGNMENTS_WRITES_ENABLED", false),
		PrivateAssignmentsWorkerEnabled: getEnvBool("PRIVATE_ASSIGNMENTS_WORKER_ENABLED", false),
		PrivateDispatchExecutionEnabled: getEnvBool("PRIVATE_DISPATCH_EXECUTION_ENABLED", false),
		PrivateHaulingExecutionEnabled:  getEnvBool("PRIVATE_HAULING_EXECUTION_ENABLED", false),
		// Taxi has no production authority contract. Configuration cannot turn it on.
		PrivateTaxiExecutionEnabled:   false,
		PrivateOperationsKillSwitch:   getEnvBool("FLEET_PRIVATE_OPERATIONS_KILL_SWITCH", false),
		PrivateAssignmentResponseTTL:  time.Duration(getEnvInt("FLEET_PRIVATE_ASSIGNMENT_RESPONSE_TTL_SECONDS", 1800)) * time.Second,
		PrivateExecutionWorkerEnabled: getEnvBool("PRIVATE_EXECUTION_WORKER_ENABLED", false),

		DispatchPlanningEnabled:     getEnvBool("DISPATCH_PLANNING_ENABLED", false),
		RoutePlanningEnabled:        getEnvBool("ROUTE_PLANNING_ENABLED", false),
		RouteOptimizationEnabled:    getEnvBool("ROUTE_OPTIMIZATION_ENABLED", false),
		RoutePublicationEnabled:     getEnvBool("ROUTE_PUBLICATION_ENABLED", false),
		ControlTowerEnabled:         getEnvBool("CONTROL_TOWER_ENABLED", false),
		TrackingProjectionsEnabled:  getEnvBool("TRACKING_PROJECTIONS_ENABLED", false),
		GeofencesEnabled:            getEnvBool("GEOFENCES_ENABLED", false),
		OperationalAlertsEnabled:    getEnvBool("OPERATIONAL_ALERTS_ENABLED", false),
		Section16WorkersEnabled:     getEnvBool("FLEET_SECTION16_WORKERS_ENABLED", false),
		WorkforceEnabled:            getEnvBool("WORKFORCE_ENABLED", false),
		ShiftsEnabled:               getEnvBool("SHIFTS_ENABLED", false),
		LeaveEnabled:                getEnvBool("LEAVE_ENABLED", false),
		QualificationsEnabled:       getEnvBool("QUALIFICATIONS_ENABLED", false),
		TrainingEnabled:             getEnvBool("TRAINING_ENABLED", false),
		PerformanceSnapshotsEnabled: getEnvBool("PERFORMANCE_SNAPSHOTS_ENABLED", false),
		AssetsEnabled:               getEnvBool("ASSETS_ENABLED", false),
		AssetAssignmentsEnabled:     getEnvBool("ASSET_ASSIGNMENTS_ENABLED", false),
		InspectionsEnabled:          getEnvBool("INSPECTIONS_ENABLED", false),
		MaintenanceEnabled:          getEnvBool("MAINTENANCE_ENABLED", false),
		BreakdownsEnabled:           getEnvBool("BREAKDOWNS_ENABLED", false),
		PartsInventoryEnabled:       getEnvBool("PARTS_INVENTORY_ENABLED", false),
		FuelRecordsEnabled:          getEnvBool("FUEL_RECORDS_ENABLED", false),
		Section17WorkersEnabled:     getEnvBool("SECTION17_WORKERS_ENABLED", false),
		Section18WorkersEnabled:     getEnvBool("SECTION18_WORKERS_ENABLED", false),
		Section16CandidateTTL:       time.Duration(getEnvInt("FLEET_SECTION16_CANDIDATE_TTL_SECONDS", 300)) * time.Second,
		Section16TrackingFreshAfter: time.Duration(getEnvInt("FLEET_SECTION16_TRACKING_FRESH_SECONDS", 120)) * time.Second,
		Section16TrackingStaleAfter: time.Duration(getEnvInt("FLEET_SECTION16_TRACKING_STALE_SECONDS", 600)) * time.Second,
		Section16TrackingRetention:  time.Duration(getEnvInt("FLEET_SECTION16_TRACKING_RETENTION_SECONDS", 86400)) * time.Second,
		Section16AlertSLA:           time.Duration(getEnvInt("FLEET_SECTION16_ALERT_SLA_SECONDS", 1800)) * time.Second,

		ComplianceEnabled:             getEnvBool("COMPLIANCE_ENABLED", false),
		SafetyReportsEnabled:          getEnvBool("SAFETY_REPORTS_ENABLED", false),
		IncidentsEnabled:              getEnvBool("INCIDENTS_ENABLED", false),
		ProofPoliciesEnabled:          getEnvBool("PROOF_POLICIES_ENABLED", false),
		ProofRecordsEnabled:           getEnvBool("PROOF_RECORDS_ENABLED", false),
		RestrictionPropagationEnabled: getEnvBool("RESTRICTION_PROPAGATION_ENABLED", false),
		SpecializedLogisticsEnabled:   getEnvBool("SPECIALIZED_LOGISTICS_ENABLED", false),
		CourierLastMileEnabled:        getEnvBool("COURIER_LAST_MILE_ENABLED", false),
		// This flag exposes only the safe Section 20 data foundations. Taxi
		// execution remains separately hard-disabled below and in its adapters.
		TaxiPassengerEnabled:      getEnvBool("TAXI_PASSENGER_ENABLED", false),
		TruckingHaulageEnabled:    getEnvBool("TRUCKING_HAULAGE_ENABLED", false),
		HeavySpecializedEnabled:   getEnvBool("HEAVY_SPECIALIZED_ENABLED", false),
		FreightForwardingEnabled:  getEnvBool("FREIGHT_FORWARDING_ENABLED", false),
		ColdChainEnabled:          getEnvBool("COLD_CHAIN_ENABLED", false),
		MedicalSensitiveEnabled:   getEnvBool("MEDICAL_SENSITIVE_ENABLED", false),
		AgriculturalFoodEnabled:   getEnvBool("AGRICULTURAL_FOOD_ENABLED", false),
		MovingRelocationEnabled:   getEnvBool("MOVING_RELOCATION_ENABLED", false),
		TowingRecoveryEnabled:     getEnvBool("TOWING_RECOVERY_ENABLED", false),
		WarehousesEnabled:         getEnvBool("WAREHOUSES_ENABLED", false),
		WarehouseInventoryEnabled: getEnvBool("WAREHOUSE_INVENTORY_ENABLED", false),
		FulfilmentEnabled:         getEnvBool("FULFILMENT_ENABLED", false),
		DockOperationsEnabled:     getEnvBool("DOCK_OPERATIONS_ENABLED", false),
		YardOperationsEnabled:     getEnvBool("YARD_OPERATIONS_ENABLED", false),
		TerminalTransfersEnabled:  getEnvBool("TERMINAL_TRANSFERS_ENABLED", false),
		ReturnsEnabled:            getEnvBool("RETURNS_ENABLED", false),
		Section19WorkersEnabled:   getEnvBool("SECTION19_WORKERS_ENABLED", false),
		Section20WorkersEnabled:   getEnvBool("SECTION20_WORKERS_ENABLED", false),
		Section21WorkersEnabled:   getEnvBool("SECTION21_WORKERS_ENABLED", false),
		ProofHMACSecret:           secret("FLEET_PROOF_HMAC_SECRET"),

		InvoicesEnabled:                    getEnvBool("INVOICES_ENABLED", false),
		ReceivablesEnabled:                 getEnvBool("RECEIVABLES_ENABLED", false),
		ExpensesEnabled:                    getEnvBool("EXPENSES_ENABLED", false),
		BudgetsEnabled:                     getEnvBool("BUDGETS_ENABLED", false),
		CompensationEnabled:                getEnvBool("COMPENSATION_ENABLED", false),
		PayrollEnabled:                     getEnvBool("PAYROLL_ENABLED", false),
		SubcontractorSettlementsEnabled:    getEnvBool("SUBCONTRACTOR_SETTLEMENTS_ENABLED", false),
		ClaimsEnabled:                      getEnvBool("CLAIMS_ENABLED", false),
		FinancePostingEnabled:              getEnvBool("FINANCE_POSTING_ENABLED", false),
		CommunicationsEnabled:              getEnvBool("COMMUNICATIONS_ENABLED", false),
		NotificationRulesEnabled:           getEnvBool("NOTIFICATION_RULES_ENABLED", false),
		SupportProjectionsEnabled:          getEnvBool("SUPPORT_PROJECTIONS_ENABLED", false),
		QualityCasesEnabled:                getEnvBool("QUALITY_CASES_ENABLED", false),
		SLAMeasurementsEnabled:             getEnvBool("SLA_MEASUREMENTS_ENABLED", false),
		EscalationsEnabled:                 getEnvBool("ESCALATIONS_ENABLED", false),
		Section22WorkersEnabled:            getEnvBool("SECTION22_WORKERS_ENABLED", false),
		Section23WorkersEnabled:            getEnvBool("SECTION23_WORKERS_ENABLED", false),
		SearchEnabled:                      getEnvBool("SEARCH_ENABLED", false),
		SynchronizationEnabled:             getEnvBool("FLEET_SYNCHRONIZATION_ENABLED", false),
		SynchronizationWorkerEnabled:       getEnvBool("FLEET_SYNCHRONIZATION_WORKER_ENABLED", false),
		SynchronizationKillSwitch:          getEnvBool("FLEET_SYNCHRONIZATION_KILL_SWITCH", false),
		OrganizationAPIClientsEnabled:      getEnvBool("FLEET_PARTNER_ORGANIZATION_API_CLIENTS_ENABLED", false),
		WebhooksEnabled:                    getEnvBool("FLEET_PARTNER_WEBHOOKS_ENABLED", false),
		WebhookDeliveryWorkerEnabled:       getEnvBool("FLEET_PARTNER_WEBHOOK_DELIVERY_WORKER_ENABLED", false),
		APIClientHashKey:                   secret("FLEET_PARTNER_API_CLIENT_HASH_KEY"),
		WebhookSecretEncryptionKey:         secret("FLEET_PARTNER_WEBHOOK_SECRET_ENCRYPTION_KEY"),
		WebhookAllowHTTP:                   getEnvBool("FLEET_PARTNER_WEBHOOK_ALLOW_HTTP", false),
		WebhookPrivateDestinationAllowlist: getEnvList("FLEET_PARTNER_WEBHOOK_PRIVATE_DESTINATION_ALLOWLIST"),
		WebhookDeliveryTimeout:             getEnvDuration("FLEET_PARTNER_WEBHOOK_DELIVERY_TIMEOUT", 15*time.Second),
		WebhookReplayWindow:                getEnvDuration("FLEET_PARTNER_WEBHOOK_REPLAY_WINDOW", 5*time.Minute),
		WebhookRotationOverlapMax:          getEnvDuration("FLEET_PARTNER_WEBHOOK_ROTATION_OVERLAP_MAX", time.Hour),
		WebhookDisableAfterFailures:        getEnvInt("FLEET_PARTNER_WEBHOOK_DISABLE_AFTER_FAILURES", 5),
		WebhookDeliveryMaxAttempts:         getEnvInt("FLEET_PARTNER_WEBHOOK_DELIVERY_MAX_ATTEMPTS", 12),
		WebhookFanoutInterval:              getEnvDuration("FLEET_PARTNER_WEBHOOK_FANOUT_INTERVAL", 2*time.Second),
		WebhookFanoutBatchSize:             getEnvInt("FLEET_PARTNER_WEBHOOK_FANOUT_BATCH_SIZE", 50),

		MetricsEnabled:   getEnvBool("FLEET_PARTNER_METRICS_ENABLED", true),
		MetricsAuthToken: getEnv("FLEET_PARTNER_METRICS_AUTH_TOKEN", ""),

		HTTPClientTimeout:    getEnvDuration("FLEET_PARTNER_HTTP_CLIENT_TIMEOUT", 10*time.Second),
		ShutdownTimeout:      getEnvDuration("FLEET_PARTNER_SHUTDOWN_TIMEOUT", 15*time.Second),
		OutboxPollInterval:   getEnvDuration("FLEET_PARTNER_OUTBOX_POLL_INTERVAL", 2*time.Second),
		OutboxStreamKey:      getEnv("FLEET_PARTNER_OUTBOX_STREAM_KEY", ""),
		JobPollInterval:      getEnvDuration("FLEET_PARTNER_JOB_POLL_INTERVAL", 2*time.Second),
		JobLeaseDuration:     getEnvDuration("FLEET_PARTNER_JOB_LEASE_DURATION", 60*time.Second),
		JobHeartbeatInterval: getEnvDuration("FLEET_PARTNER_JOB_HEARTBEAT_INTERVAL", 15*time.Second),

		MaxRequestBodyBytes: int64(getEnvInt("FLEET_PARTNER_MAX_REQUEST_BODY_BYTES", 1<<20)),
		TrustedProxies:      getEnvList("FLEET_PARTNER_TRUSTED_PROXIES"),

		OTPSecret:     secret("FLEET_PARTNER_OTP_HMAC_SECRET"),
		SessionSecret: secret("FLEET_PARTNER_SESSION_HASH_SECRET"),
		ContactSecret: secret("FLEET_PARTNER_CONTACT_HASH_SECRET"),

		OTPLength:         getEnvInt("FLEET_PARTNER_OTP_LENGTH", 6),
		OTPExpiry:         getEnvDuration("FLEET_PARTNER_OTP_EXPIRY", 10*time.Minute),
		OTPMaxAttempts:    getEnvInt("FLEET_PARTNER_OTP_MAX_ATTEMPTS", 5),
		OTPResendCooldown: getEnvDuration("FLEET_PARTNER_OTP_RESEND_COOLDOWN", 60*time.Second),
		OTPMaxPerHour:     getEnvInt("FLEET_PARTNER_OTP_MAX_REQUESTS_PER_HOUR", 10),

		AccessTokenTTL:        getEnvDuration("FLEET_PARTNER_ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL:       getEnvDuration("FLEET_PARTNER_REFRESH_TOKEN_TTL", 720*time.Hour),
		StepUpTTL:             getEnvDuration("FLEET_PARTNER_STEP_UP_TTL", 5*time.Minute),
		StaffInvitationTTL:    getEnvDuration("FLEET_PARTNER_STAFF_INVITATION_TTL", 168*time.Hour),
		ProviderInvitationTTL: getEnvDuration("FLEET_PARTNER_PROVIDER_INVITATION_TTL", 336*time.Hour),

		DefaultCountryCode:    getEnv("FLEET_PARTNER_DEFAULT_COUNTRY_CODE", "234"),
		StaffAcceptURL:        getEnv("FLEET_PARTNER_STAFF_ACCEPT_URL", ""),
		ProviderAcceptURL:     getEnv("FLEET_PARTNER_PROVIDER_ACCEPT_URL", ""),
		AuthorizationCacheTTL: getEnvDuration("FLEET_PARTNER_AUTHORIZATION_CACHE_TTL", 30*time.Second),

		TraceSampleRate: getEnvFloat("FLEET_PARTNER_TRACE_SAMPLE_RATE", 1.0),

		ExternalClientMaxRetries:       getEnvInt("FLEET_PARTNER_EXTERNAL_CLIENT_MAX_RETRIES", 2),
		ExternalClientRetryBackoffBase: getEnvDuration("FLEET_PARTNER_EXTERNAL_CLIENT_RETRY_BACKOFF_BASE", 200*time.Millisecond),
		ExternalClientRetryBackoffMax:  getEnvDuration("FLEET_PARTNER_EXTERNAL_CLIENT_RETRY_BACKOFF_MAX", 5*time.Second),

		CircuitBreakerFailureThreshold:  getEnvInt("FLEET_PARTNER_CIRCUIT_BREAKER_FAILURE_THRESHOLD", 5),
		CircuitBreakerObservationWindow: getEnvDuration("FLEET_PARTNER_CIRCUIT_BREAKER_OBSERVATION_WINDOW", 30*time.Second),
		CircuitBreakerOpenDuration:      getEnvDuration("FLEET_PARTNER_CIRCUIT_BREAKER_OPEN_DURATION", 30*time.Second),
		CircuitBreakerHalfOpenProbes:    getEnvInt("FLEET_PARTNER_CIRCUIT_BREAKER_HALF_OPEN_PROBES", 2),
		CircuitBreakerSuccessThreshold:  getEnvInt("FLEET_PARTNER_CIRCUIT_BREAKER_SUCCESS_THRESHOLD", 2),

		ReadinessOutboxStaleAfter:      getEnvDuration("FLEET_PARTNER_READINESS_OUTBOX_STALE_AFTER", 10*time.Minute),
		ReadinessOutboxBacklogCritical: int64(getEnvInt("FLEET_PARTNER_READINESS_OUTBOX_BACKLOG_CRITICAL", 5000)),
		ReadinessJobDeadLetterCritical: int64(getEnvInt("FLEET_PARTNER_READINESS_JOB_DEAD_LETTER_CRITICAL", 500)),
		ReadinessStaleLeaseThreshold:   getEnvDuration("FLEET_PARTNER_READINESS_STALE_LEASE_THRESHOLD", 10*time.Minute),
	}

	cfg.Clients = map[string]ClientConfig{
		"customer-service":                 {BaseURL: getEnv("FLEET_PARTNER_CUSTOMER_SERVICE_BASE_URL", ""), Secret: secret("FLEET_PARTNER_CUSTOMER_SERVICE_SECRET")},
		"driver-dispatch-delivery-service": {BaseURL: getEnv("FLEET_PARTNER_DISPATCH_SERVICE_BASE_URL", ""), Secret: secret("FLEET_PARTNER_DISPATCH_SERVICE_SECRET")},
		"driver-taxi-service":              {BaseURL: getEnv("FLEET_PARTNER_TAXI_SERVICE_BASE_URL", ""), Secret: secret("FLEET_PARTNER_TAXI_SERVICE_SECRET")},
		"driver-hauling-service":           {BaseURL: getEnv("FLEET_PARTNER_HAULING_SERVICE_BASE_URL", ""), Secret: secret("FLEET_PARTNER_HAULING_SERVICE_SECRET")},
		"payment-wallet-service":           {BaseURL: getEnv("FLEET_PARTNER_PAYMENT_WALLET_SERVICE_BASE_URL", ""), Secret: secret("FLEET_PARTNER_PAYMENT_WALLET_SERVICE_SECRET")},
		"accounting-service":               {BaseURL: getEnv("FLEET_PARTNER_ACCOUNTING_SERVICE_BASE_URL", ""), Secret: secret("FLEET_PARTNER_ACCOUNTING_SERVICE_SECRET")},
		"notification-service":             {BaseURL: getEnv("FLEET_PARTNER_NOTIFICATION_SERVICE_BASE_URL", ""), Secret: secret("FLEET_PARTNER_NOTIFICATION_SERVICE_SECRET")},
		"media-file-service":               {BaseURL: getEnv("FLEET_PARTNER_MEDIA_FILE_SERVICE_BASE_URL", ""), Secret: secret("FLEET_PARTNER_MEDIA_FILE_SERVICE_SECRET")},
		"verification-compliance-service":  {BaseURL: getEnv("FLEET_PARTNER_VERIFICATION_COMPLIANCE_SERVICE_BASE_URL", ""), Secret: secret("FLEET_PARTNER_VERIFICATION_COMPLIANCE_SERVICE_SECRET")},
		"support-dispute-service":          {BaseURL: getEnv("FLEET_PARTNER_SUPPORT_DISPUTE_SERVICE_BASE_URL", ""), Secret: secret("FLEET_PARTNER_SUPPORT_DISPUTE_SERVICE_SECRET")},
		"analytics-service":                {BaseURL: getEnv("FLEET_PARTNER_ANALYTICS_SERVICE_BASE_URL", ""), Secret: secret("FLEET_PARTNER_ANALYTICS_SERVICE_SECRET")},
		"admin-backoffice-service":         {BaseURL: getEnv("FLEET_PARTNER_ADMIN_BACKOFFICE_SERVICE_BASE_URL", ""), Secret: secret("FLEET_PARTNER_ADMIN_BACKOFFICE_SERVICE_SECRET")},
		"route-optimization-service":       {BaseURL: getEnv("FLEET_PARTNER_ROUTE_OPTIMIZATION_SERVICE_BASE_URL", ""), Secret: secret("FLEET_PARTNER_ROUTE_OPTIMIZATION_SERVICE_SECRET")},
	}

	browserPolicy, err := browserauth.Load("FLEET_PARTNER_AUTH", cfg.AppEnv, "karrygo_fleet_refresh", "/fleet-partner/v1/auth")
	if err != nil {
		return Config{}, err
	}
	cfg.BrowserPolicy = browserPolicy
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate enforces mandatory settings and value constraints.
func (c Config) Validate() error {
	var problems []string

	if c.DatabaseURL == "" {
		problems = append(problems, "FLEET_PARTNER_DATABASE_URL is required")
	}
	if len(c.AccessTokenSecret) == 0 {
		problems = append(problems, "FLEET_PARTNER_ACCESS_TOKEN_SECRET is required")
	}
	// Authentication secrets have no defaults in any environment: a
	// predictable OTP pepper would make a six-digit code guessable offline.
	if len(c.OTPSecret) == 0 {
		problems = append(problems, "FLEET_PARTNER_OTP_HMAC_SECRET is required")
	}
	if c.ProofRecordsEnabled && len(c.ProofHMACSecret) == 0 {
		problems = append(problems, "FLEET_PROOF_HMAC_SECRET is required when PROOF_RECORDS_ENABLED is set")
	}
	if c.ProofRecordsEnabled && !c.ProofPoliciesEnabled {
		problems = append(problems, "PROOF_POLICIES_ENABLED is required when PROOF_RECORDS_ENABLED is set")
	}
	if c.RestrictionPropagationEnabled {
		for _, name := range []string{"driver-dispatch-delivery-service", "driver-hauling-service"} {
			if !c.Clients[name].Configured() {
				problems = append(problems, name+" base URL and secret are required when RESTRICTION_PROPAGATION_ENABLED is set")
			}
		}
	}
	if len(c.SessionSecret) == 0 {
		problems = append(problems, "FLEET_PARTNER_SESSION_HASH_SECRET is required")
	}
	if len(c.ContactSecret) == 0 {
		problems = append(problems, "FLEET_PARTNER_CONTACT_HASH_SECRET is required")
	}
	if c.OTPLength < 6 || c.OTPLength > 10 {
		problems = append(problems, "FLEET_PARTNER_OTP_LENGTH must be between 6 and 10")
	}
	if c.OTPMaxAttempts < 1 || c.OTPMaxAttempts > 10 {
		problems = append(problems, "FLEET_PARTNER_OTP_MAX_ATTEMPTS must be between 1 and 10")
	}
	if c.OTPExpiry < time.Minute || c.OTPExpiry > time.Hour {
		problems = append(problems, "FLEET_PARTNER_OTP_EXPIRY must be between 1m and 1h")
	}
	if c.AccessTokenTTL <= 0 || c.AccessTokenTTL > time.Hour {
		problems = append(problems, "FLEET_PARTNER_ACCESS_TOKEN_TTL must be positive and at most 1h")
	}
	if c.RefreshTokenTTL <= c.AccessTokenTTL {
		problems = append(problems, "FLEET_PARTNER_REFRESH_TOKEN_TTL must exceed FLEET_PARTNER_ACCESS_TOKEN_TTL")
	}
	if c.MetricsEnabled && c.MetricsAuthToken == "" {
		problems = append(problems, "FLEET_PARTNER_METRICS_AUTH_TOKEN is required when metrics are enabled")
	}
	if c.OrganizationAPIClientsEnabled && len(c.APIClientHashKey) < 32 {
		problems = append(problems, "FLEET_PARTNER_API_CLIENT_HASH_KEY must contain at least 32 bytes when organization API clients are enabled")
	}
	if c.WebhooksEnabled && len(c.WebhookSecretEncryptionKey) != 32 {
		problems = append(problems, "FLEET_PARTNER_WEBHOOK_SECRET_ENCRYPTION_KEY must contain exactly 32 bytes when webhooks are enabled")
	}
	if c.WebhookDeliveryWorkerEnabled && !c.WebhooksEnabled {
		problems = append(problems, "FLEET_PARTNER_WEBHOOKS_ENABLED is required when the webhook delivery worker is enabled")
	}
	if c.AppEnv == "production" && c.WebhookAllowHTTP {
		problems = append(problems, "FLEET_PARTNER_WEBHOOK_ALLOW_HTTP cannot be enabled in production")
	}
	if c.WebhooksEnabled || c.WebhookDeliveryWorkerEnabled {
		if c.WebhookDeliveryTimeout <= 0 || c.WebhookDeliveryTimeout > time.Minute {
			problems = append(problems, "FLEET_PARTNER_WEBHOOK_DELIVERY_TIMEOUT must be positive and at most 1m")
		}
		if c.WebhookReplayWindow < time.Minute || c.WebhookReplayWindow > 15*time.Minute {
			problems = append(problems, "FLEET_PARTNER_WEBHOOK_REPLAY_WINDOW must be between 1m and 15m")
		}
		if c.WebhookRotationOverlapMax < 0 || c.WebhookRotationOverlapMax > 24*time.Hour {
			problems = append(problems, "FLEET_PARTNER_WEBHOOK_ROTATION_OVERLAP_MAX must be between 0 and 24h")
		}
		if c.WebhookDisableAfterFailures < 1 || c.WebhookDisableAfterFailures > 100 {
			problems = append(problems, "FLEET_PARTNER_WEBHOOK_DISABLE_AFTER_FAILURES must be between 1 and 100")
		}
		if c.WebhookDeliveryMaxAttempts < 1 || c.WebhookDeliveryMaxAttempts > 50 {
			problems = append(problems, "FLEET_PARTNER_WEBHOOK_DELIVERY_MAX_ATTEMPTS must be between 1 and 50")
		}
		if c.WebhookFanoutInterval <= 0 || c.WebhookFanoutBatchSize < 1 || c.WebhookFanoutBatchSize > 500 {
			problems = append(problems, "webhook fanout interval must be positive and batch size must be between 1 and 500")
		}
	}
	if c.AppEnv == "production" && c.ServiceSecrets == "" {
		problems = append(problems, "FLEET_PARTNER_SERVICE_SECRETS is required in production")
	}
	if err := validateAddr(c.HTTPAddr); err != nil {
		problems = append(problems, "HTTP_ADDR is invalid: "+err.Error())
	}
	if c.RedisRequired && c.Redis.Addr == "" {
		problems = append(problems, "FLEET_PARTNER_REDIS_ADDR is required when FLEET_PARTNER_REDIS_REQUIRED=true")
	}
	// Taxi is intentionally unavailable until driver-taxi-service publishes a
	// production contract. Rejecting the flag is safer than accepting config
	// that can only fail later or silently fall back locally.
	if c.TaxiIntegrationEnabled {
		problems = append(problems, "FLEET_TAXI_INTEGRATION_ENABLED cannot be enabled: driver-taxi-service has no production contract")
	}
	if c.PrivateTaxiExecutionEnabled {
		problems = append(problems, "PRIVATE_TAXI_EXECUTION_ENABLED cannot be enabled: driver-taxi-service has no production private execution contract")
	}
	if c.PrivateDispatchExecutionEnabled && !c.IntegrationsKillSwitch && !c.PrivateOperationsKillSwitch {
		if strings.TrimSpace(c.DispatchServiceURL) == "" || len(c.DispatchServiceSecret) == 0 || c.DispatchServiceTimeout <= 0 {
			problems = append(problems, "DISPATCH_SERVICE_URL, DISPATCH_SERVICE_SECRET and a positive timeout are required when private Dispatch execution is enabled")
		}
	}
	if c.PrivateHaulingExecutionEnabled && !c.IntegrationsKillSwitch && !c.PrivateOperationsKillSwitch {
		hauling := c.Clients["driver-hauling-service"]
		if !hauling.Configured() || c.HTTPClientTimeout <= 0 {
			problems = append(problems, "driver-hauling-service URL, secret and a positive timeout are required when private Hauling execution is enabled")
		}
	}
	if c.PrivateExecutionWorkerEnabled && !c.PrivateDispatchExecutionEnabled && !c.PrivateHaulingExecutionEnabled {
		problems = append(problems, "PRIVATE_EXECUTION_WORKER_ENABLED requires private Dispatch or Hauling execution")
	}
	if c.RouteOptimizationEnabled && !c.IntegrationsKillSwitch && !c.PrivateOperationsKillSwitch && !c.Clients["route-optimization-service"].Configured() {
		problems = append(problems, "ROUTE_OPTIMIZATION_ENABLED requires a configured provider-neutral route optimization client")
	}
	if c.RoutePublicationEnabled && (!c.RoutePlanningEnabled || !c.PrivateAssignmentsWritesEnabled) {
		problems = append(problems, "ROUTE_PUBLICATION_ENABLED requires ROUTE_PLANNING_ENABLED and PRIVATE_ASSIGNMENTS_WRITES_ENABLED")
	}
	if c.Section16WorkersEnabled && !c.WorkerEnabled {
		problems = append(problems, "FLEET_SECTION16_WORKERS_ENABLED requires FLEET_PARTNER_WORKER_ENABLED")
	}
	if (c.Section17WorkersEnabled || c.Section18WorkersEnabled) && !c.WorkerEnabled {
		problems = append(problems, "SECTION17_WORKERS_ENABLED and SECTION18_WORKERS_ENABLED require FLEET_PARTNER_WORKER_ENABLED")
	}
	if (c.Section19WorkersEnabled || c.Section20WorkersEnabled || c.Section21WorkersEnabled) && !c.WorkerEnabled {
		problems = append(problems, "SECTION19_WORKERS_ENABLED, SECTION20_WORKERS_ENABLED and SECTION21_WORKERS_ENABLED require FLEET_PARTNER_WORKER_ENABLED")
	}
	if (c.Section22WorkersEnabled || c.Section23WorkersEnabled) && !c.WorkerEnabled {
		problems = append(problems, "SECTION22_WORKERS_ENABLED and SECTION23_WORKERS_ENABLED require FLEET_PARTNER_WORKER_ENABLED")
	}
	if c.ReceivablesEnabled && !c.InvoicesEnabled {
		problems = append(problems, "INVOICES_ENABLED is required when RECEIVABLES_ENABLED is set")
	}
	if c.PayrollEnabled && !c.CompensationEnabled {
		problems = append(problems, "COMPENSATION_ENABLED is required when PAYROLL_ENABLED is set")
	}
	if c.FinancePostingEnabled && !c.IntegrationsKillSwitch {
		if c.PayrollEnabled && !c.Clients["payment-wallet-service"].Configured() {
			problems = append(problems, "payment-wallet-service URL and secret are required when FINANCE_POSTING_ENABLED and PAYROLL_ENABLED are set")
		}
		if (c.ExpensesEnabled || c.SubcontractorSettlementsEnabled) && !c.Clients["accounting-service"].Configured() {
			problems = append(problems, "accounting-service URL and secret are required when FINANCE_POSTING_ENABLED and expense or subcontractor posting is enabled")
		}
	}
	if c.CommunicationsEnabled && !c.IntegrationsKillSwitch && !c.Clients["notification-service"].Configured() {
		problems = append(problems, "notification-service URL and secret are required when COMMUNICATIONS_ENABLED is set")
	}
	if c.SupportProjectionsEnabled && !c.IntegrationsKillSwitch && !c.Clients["support-dispute-service"].Configured() {
		problems = append(problems, "support-dispute-service URL and secret are required when SUPPORT_PROJECTIONS_ENABLED is set")
	}
	if c.SLAMeasurementsEnabled && !c.SLAEnabled {
		problems = append(problems, "SLA_ENABLED is required when SLA_MEASUREMENTS_ENABLED is set")
	}
	if c.WarehouseInventoryEnabled && !c.WarehousesEnabled {
		problems = append(problems, "WAREHOUSES_ENABLED is required when WAREHOUSE_INVENTORY_ENABLED is set")
	}
	if c.FulfilmentEnabled && (!c.WarehousesEnabled || !c.WarehouseInventoryEnabled) {
		problems = append(problems, "WAREHOUSES_ENABLED and WAREHOUSE_INVENTORY_ENABLED are required when FULFILMENT_ENABLED is set")
	}
	if (c.DockOperationsEnabled || c.YardOperationsEnabled || c.TerminalTransfersEnabled || c.ReturnsEnabled) && !c.WarehousesEnabled {
		problems = append(problems, "WAREHOUSES_ENABLED is required for dock, yard, terminal-transfer and return operations")
	}
	section16Configured := c.DispatchPlanningEnabled || c.RoutePlanningEnabled || c.RouteOptimizationEnabled || c.RoutePublicationEnabled || c.ControlTowerEnabled || c.TrackingProjectionsEnabled || c.GeofencesEnabled || c.OperationalAlertsEnabled || c.Section16WorkersEnabled
	if section16Configured && (c.Section16TrackingFreshAfter <= 0 || c.Section16TrackingStaleAfter <= c.Section16TrackingFreshAfter || c.Section16TrackingRetention <= c.Section16TrackingStaleAfter) {
		problems = append(problems, "Section 16 tracking thresholds must be positive and ordered fresh < stale < retention")
	}
	if c.HaulingIntegrationEnabled && !c.IntegrationsKillSwitch {
		hauling := c.Clients["driver-hauling-service"]
		if strings.TrimSpace(hauling.BaseURL) == "" {
			problems = append(problems, "FLEET_PARTNER_HAULING_SERVICE_BASE_URL is required when hauling integration is enabled")
		}
		if len(hauling.Secret) == 0 {
			problems = append(problems, "FLEET_PARTNER_HAULING_SERVICE_SECRET is required when hauling integration is enabled")
		}
	}
	if c.DispatchExecutionStartEnabled && !c.IntegrationsKillSwitch {
		if strings.TrimSpace(c.DispatchServiceURL) == "" {
			problems = append(problems, "DISPATCH_SERVICE_URL is required when Dispatch execution is enabled")
		}
		if len(c.DispatchServiceSecret) == 0 {
			problems = append(problems, "DISPATCH_SERVICE_SECRET is required when Dispatch execution is enabled")
		}
	}
	// Section 13 writes and workers move real money through
	// payment-wallet-service. Enabling one without a configured Wallet client
	// is refused at startup rather than tolerated: a half-configured financial
	// integration that starts successfully is one that fails at the moment a
	// payment is expected.
	if (c.ProviderAllocationEnabled || c.ProviderWithdrawalEnabled ||
		c.WalletProjectionRefreshEnabled || c.WalletReconciliationEnabled) &&
		!c.IntegrationsKillSwitch && !c.CompanyWalletKillSwitch {
		wallet := c.Clients["payment-wallet-service"]
		if strings.TrimSpace(wallet.BaseURL) == "" {
			problems = append(problems,
				"FLEET_PARTNER_PAYMENT_WALLET_SERVICE_BASE_URL is required when provider allocations, withdrawals or wallet reconciliation are enabled")
		}
		if len(wallet.Secret) == 0 {
			problems = append(problems,
				"FLEET_PARTNER_PAYMENT_WALLET_SERVICE_SECRET is required when provider allocations, withdrawals or wallet reconciliation are enabled")
		}
	}
	// A write surface without its read surface is incoherent: an operator
	// cannot check the result of an allocation they were allowed to make.
	if (c.ProviderAllocationEnabled || c.ProviderWithdrawalEnabled) && !c.ProviderSubWalletEnabled {
		problems = append(problems,
			"PROVIDER_SUBWALLET_ENABLED is required when PROVIDER_ALLOCATION_ENABLED or PROVIDER_WITHDRAWAL_ENABLED is set")
	}
	// The staleness window only has to be stated once something depends on it.
	// A negative value is always wrong; zero means "use the service default"
	// and is how a config built in a test that does not exercise Section 13
	// stays valid.
	if c.CompanyWalletStaleAfter < 0 ||
		((c.ProviderAllocationEnabled || c.ProviderWithdrawalEnabled) && c.CompanyWalletStaleAfter == 0) {
		problems = append(problems, "FLEET_COMPANY_WALLET_STALE_SECONDS must be a positive duration when provider allocations or withdrawals are enabled")
	}
	if c.JobLeaseDuration <= c.JobHeartbeatInterval {
		problems = append(problems, "FLEET_PARTNER_JOB_LEASE_DURATION must be greater than FLEET_PARTNER_JOB_HEARTBEAT_INTERVAL")
	}
	for _, d := range []struct {
		name  string
		value time.Duration
	}{
		{"FLEET_PARTNER_HTTP_CLIENT_TIMEOUT", c.HTTPClientTimeout},
		{"FLEET_PARTNER_SHUTDOWN_TIMEOUT", c.ShutdownTimeout},
		{"FLEET_PARTNER_OUTBOX_POLL_INTERVAL", c.OutboxPollInterval},
		{"FLEET_PARTNER_JOB_POLL_INTERVAL", c.JobPollInterval},
		{"FLEET_PARTNER_JOB_LEASE_DURATION", c.JobLeaseDuration},
		{"FLEET_PARTNER_JOB_HEARTBEAT_INTERVAL", c.JobHeartbeatInterval},
		{"FLEET_PARTNER_EXTERNAL_CLIENT_RETRY_BACKOFF_BASE", c.ExternalClientRetryBackoffBase},
		{"FLEET_PARTNER_EXTERNAL_CLIENT_RETRY_BACKOFF_MAX", c.ExternalClientRetryBackoffMax},
		{"FLEET_PARTNER_CIRCUIT_BREAKER_OBSERVATION_WINDOW", c.CircuitBreakerObservationWindow},
		{"FLEET_PARTNER_CIRCUIT_BREAKER_OPEN_DURATION", c.CircuitBreakerOpenDuration},
		{"FLEET_PARTNER_READINESS_OUTBOX_STALE_AFTER", c.ReadinessOutboxStaleAfter},
		{"FLEET_PARTNER_READINESS_STALE_LEASE_THRESHOLD", c.ReadinessStaleLeaseThreshold},
	} {
		if d.value <= 0 {
			problems = append(problems, d.name+" must be a positive duration")
		}
	}
	// Section 29: bounded, non-zero-unbounded values only. A retry count of 0
	// is a deliberate, safe "never automatically retry" choice, but a
	// negative or absurdly large value is rejected the same way an unbounded
	// external timeout would be.
	if c.TraceSampleRate < 0 || c.TraceSampleRate > 1 {
		problems = append(problems, "FLEET_PARTNER_TRACE_SAMPLE_RATE must be between 0 and 1")
	}
	if c.ExternalClientMaxRetries < 0 || c.ExternalClientMaxRetries > 10 {
		problems = append(problems, "FLEET_PARTNER_EXTERNAL_CLIENT_MAX_RETRIES must be between 0 and 10")
	}
	if c.ExternalClientRetryBackoffMax > 0 && c.ExternalClientRetryBackoffBase > 0 &&
		c.ExternalClientRetryBackoffMax < c.ExternalClientRetryBackoffBase {
		problems = append(problems, "FLEET_PARTNER_EXTERNAL_CLIENT_RETRY_BACKOFF_MAX must be >= FLEET_PARTNER_EXTERNAL_CLIENT_RETRY_BACKOFF_BASE")
	}
	if c.CircuitBreakerFailureThreshold < 1 || c.CircuitBreakerFailureThreshold > 1000 {
		problems = append(problems, "FLEET_PARTNER_CIRCUIT_BREAKER_FAILURE_THRESHOLD must be between 1 and 1000")
	}
	if c.CircuitBreakerHalfOpenProbes < 1 || c.CircuitBreakerHalfOpenProbes > 100 {
		problems = append(problems, "FLEET_PARTNER_CIRCUIT_BREAKER_HALF_OPEN_PROBES must be between 1 and 100")
	}
	if c.CircuitBreakerSuccessThreshold < 1 || c.CircuitBreakerSuccessThreshold > 100 {
		problems = append(problems, "FLEET_PARTNER_CIRCUIT_BREAKER_SUCCESS_THRESHOLD must be between 1 and 100")
	}
	if c.ReadinessOutboxBacklogCritical < 1 {
		problems = append(problems, "FLEET_PARTNER_READINESS_OUTBOX_BACKLOG_CRITICAL must be at least 1")
	}
	if c.ReadinessJobDeadLetterCritical < 1 {
		problems = append(problems, "FLEET_PARTNER_READINESS_JOB_DEAD_LETTER_CRITICAL must be at least 1")
	}
	if c.DatabaseMaxOpenConnections < 1 {
		problems = append(problems, "FLEET_PARTNER_DATABASE_MAX_OPEN_CONNECTIONS must be at least 1")
	}
	if c.MaxRequestBodyBytes < 1024 {
		problems = append(problems, "FLEET_PARTNER_MAX_REQUEST_BODY_BYTES must be at least 1024")
	}
	for name, client := range c.Clients {
		if client.BaseURL == "" {
			continue
		}
		parsed, err := url.Parse(client.BaseURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			problems = append(problems, fmt.Sprintf("base URL for %s is invalid", name))
		}
	}
	// Section 28 audit hash-chaining (internal/platform/audit/chain.go) falls
	// back to a fixed, publicly-known dev key when FLEET_PARTNER_AUDIT_CHAIN_
	// SECRET is unset, so the chain stays SELF-consistent (still detects
	// tampering) but is not confidentially keyed -- anyone who can write to
	// fleet_partner_audit_records directly (a compromised superuser
	// credential, a restored-and-edited backup) could recompute a fully
	// "valid" chain over their own tampering. Read directly rather than
	// threaded through a new Config field: chain.go's key is deliberately a
	// package-level lazy singleton, not passed through audit.NewRecorder's
	// constructor (which has ~30 existing call sites across every domain
	// feature) -- this check only needs to confirm the same env var chain.go
	// itself will read is actually set before a production process accepts
	// traffic, not become the source of truth for the runtime value.
	if c.AppEnv == "production" && os.Getenv("FLEET_PARTNER_AUDIT_CHAIN_SECRET") == "" {
		problems = append(problems, "FLEET_PARTNER_AUDIT_CHAIN_SECRET is required in production (audit hash-chain HMAC key)")
	}

	if len(problems) > 0 {
		return errors.New("configuration invalid: " + strings.Join(problems, "; "))
	}
	return nil
}

func validateAddr(addr string) error {
	host, port, ok := strings.Cut(addr, ":")
	if !ok {
		return fmt.Errorf("expected host:port or :port")
	}
	_ = host
	parsed, err := strconv.Atoi(port)
	if err != nil || parsed < 1 || parsed > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	return nil
}

func secret(name string) []byte {
	value := os.Getenv(name)
	if value == "" {
		return nil
	}
	return []byte(value)
}

func getEnv(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

// getEnvList splits a comma-separated env var into trimmed, non-empty
// entries. An unset or empty variable returns nil, not a slice with one
// empty string.
func getEnvList(key string) []string {
	value := os.Getenv(key)
	if value == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func getEnvFloat(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
