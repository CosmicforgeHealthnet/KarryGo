package app

import (
	"context"
	"os"
	"testing"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	retentionpolicymodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/retentionexec"
)

// Section 28 final wiring: proves the REAL production composition path
// (BuildComponents -> Registrars, the exact call sequence cmd/main.go and
// cmd/worker/main.go both use) registers all four retention class
// executors under real, distinct dependencies -- never a test double, and
// never two executors silently sharing one class.

func fullyConfiguredRetentionCfg() config.Config {
	return config.Config{
		HTTPClientTimeout:      time.Second,
		SessionSecret:          []byte("unit-session"),
		OTPSecret:              []byte("unit-otp"),
		ContactSecret:          []byte("unit-contact"),
		AccessTokenSecret:      []byte("unit-access"),
		SynchronizationEnabled: true,
		Clients: map[string]config.ClientConfig{
			"media-file-service":      {BaseURL: "http://media.invalid", Secret: []byte("media")},
			"support-dispute-service": {BaseURL: "http://support.invalid", Secret: []byte("support")},
		},
	}
}

// TestRetentionProduction_AllFourClassesRegisteredWhenFullyConfigured
// proves that with every mandatory dependency configured, the REAL
// production Registrars call registers OrgContact (no external
// dependency, always on) plus all three new executors -- and that a
// retention class this package never implements an executor for
// (ClassAudit is valid CONFIGURATION -- retentionpolicy lets an
// organization set a day count for it -- but has no ClassExecutor) stays
// rejected, exactly as internal/platform/retentionexec's own package doc
// requires.
func TestRetentionProduction_AllFourClassesRegisteredWhenFullyConfigured(t *testing.T) {
	cfg := fullyConfiguredRetentionCfg()
	components := BuildComponents(cfg, nil, nil, observability.NewMetrics())
	_, _, _ = components.Registrars(cfg)

	if components.RetentionRunner == nil {
		t.Fatal("RetentionRunner must always be constructed")
	}
	if !components.RetentionExpiredExportReady {
		t.Fatal("expired-export retention must be ready when media-file-service is configured")
	}
	if !components.RetentionSupportMetadataReady {
		t.Fatal("support-metadata retention must be ready when support-dispute-service is configured")
	}
	if !components.RetentionSupersededProjectionReady {
		t.Fatal("superseded-projection retention must be ready when synchronization is enabled with media configured (registers the media.asset.status_changed.v1 authority)")
	}
	for _, class := range []string{
		retentionpolicymodels.ClassOperational,         // OrgContactExecutor
		retentionpolicymodels.ClassTemporary,           // ExpiredExportExecutor
		retentionpolicymodels.ClassSupport,             // SupportMetadataExecutor
		retentionpolicymodels.ClassCompanyConfidential, // SupersededProjectionExecutor
	} {
		if !components.RetentionRunner.SupportsClass(class) {
			t.Fatalf("class %s must be registered when fully configured", class)
		}
	}
	// A valid retentionpolicy class that genuinely has no ClassExecutor
	// (see retentionexec's package doc: financial/trip-execution/safety/
	// incident/compliance tables are categorically excluded) must remain
	// rejected regardless of how much else is configured.
	if components.RetentionRunner.SupportsClass(retentionpolicymodels.ClassAudit) {
		t.Fatal("ClassAudit has no registered executor and must never report supported")
	}
}

// TestRetentionProduction_PartiallyConfiguredOnlyRegistersAvailableClasses
// proves each of the three new executors is INDEPENDENTLY gated -- a
// deployment with only media-file-service configured gets expired-export
// retention and nothing else new, never a silent all-or-nothing switch.
func TestRetentionProduction_PartiallyConfiguredOnlyRegistersAvailableClasses(t *testing.T) {
	cfg := config.Config{
		HTTPClientTimeout: time.Second,
		SessionSecret:     []byte("unit-session"),
		OTPSecret:         []byte("unit-otp"),
		ContactSecret:     []byte("unit-contact"),
		AccessTokenSecret: []byte("unit-access"),
		Clients: map[string]config.ClientConfig{
			"media-file-service": {BaseURL: "http://media.invalid", Secret: []byte("media")},
		},
	}
	components := BuildComponents(cfg, nil, nil, observability.NewMetrics())
	_, _, _ = components.Registrars(cfg)

	if !components.RetentionExpiredExportReady {
		t.Fatal("expired-export retention must be ready when media-file-service is configured")
	}
	if components.RetentionSupportMetadataReady {
		t.Fatal("support-metadata retention must NOT be ready when support-dispute-service is unconfigured")
	}
	if components.RetentionSupersededProjectionReady {
		t.Fatal("superseded-projection retention must NOT be ready when synchronization is disabled")
	}
	if !components.RetentionRunner.SupportsClass(retentionpolicymodels.ClassTemporary) {
		t.Fatal("ClassTemporary must be registered")
	}
	if components.RetentionRunner.SupportsClass(retentionpolicymodels.ClassSupport) {
		t.Fatal("ClassSupport must NOT be registered without a configured support-dispute-service client")
	}
	if components.RetentionRunner.SupportsClass(retentionpolicymodels.ClassCompanyConfidential) {
		t.Fatal("ClassCompanyConfidential must NOT be registered without synchronization enabled")
	}
}

// TestRetentionProduction_NoDuplicateExecutorRegistration guards against
// exactly the bug this session already found and fixed once (see
// internal/platform/retentionexec/executor_superseded_projection.go's
// own Class() doc comment): two executors silently claiming the same
// retention class, where the second registered in NewRunner's
// map-keyed registry would silently shadow the first. Every executor
// this package constructs must report a distinct Class().
func TestRetentionProduction_NoDuplicateExecutorRegistration(t *testing.T) {
	classes := []string{
		retentionexec.NewOrgContactExecutor().Class(),
		retentionexec.NewExpiredExportExecutor(nil).Class(),
		retentionexec.NewSupportMetadataExecutor(nil).Class(),
		retentionexec.NewSupersededProjectionExecutor(nil).Class(),
	}
	seen := map[string]bool{}
	for _, class := range classes {
		if seen[class] {
			t.Fatalf("class %s is claimed by more than one executor -- NewRunner's map-keyed registry would silently drop one", class)
		}
		seen[class] = true
	}
}

// TestRetentionProduction_EnabledModeReachesRealRegisteredExecutor proves
// the production wiring path is not merely present but actually
// REACHABLE: a DRY_RUN pass through the real, fully-composed
// RetentionRunner -- built by the exact same BuildComponents+Registrars
// sequence cmd/main.go runs -- for a real EXPIRED export row resolves
// through ExpiredExportExecutor.Preview (not a hand-rolled test
// executor) and reports ARCHIVED. DRY_RUN is used deliberately: Preview
// never calls the media client (see ExpiredExportExecutor's own doc
// comment, "a preview must never itself request an irreversible action
// against another service"), so this proves reachability through the
// real production object graph without requiring a live media-file-
// service to answer a real deletion request.
func TestRetentionProduction_EnabledModeReachesRealRegisteredExecutor(t *testing.T) {
	rawURL := os.Getenv("FLEET_PARTNER_TEST_DATABASE_URL")
	if rawURL == "" {
		t.Skip("FLEET_PARTNER_TEST_DATABASE_URL not set; skipping production-composition PostgreSQL test")
	}
	ctx := context.Background()
	pool, err := database.NewPool(ctx, rawURL, database.PoolOptions{MaxConns: 5})
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	defer pool.Close()

	cfg := fullyConfiguredRetentionCfg()
	components := BuildComponents(cfg, pool, nil, observability.NewMetrics())
	_, _, _ = components.Registrars(cfg)
	if components.RetentionRunner == nil || !components.RetentionExpiredExportReady {
		t.Fatal("expected expired-export retention to be ready in this test's config")
	}

	orgID := ids.New("org")
	if _, err := pool.Exec(ctx, `
		INSERT INTO fleet_partner_organizations
			(id, legal_name, registration_number, country_code, timezone, default_currency, status, created_at, updated_at)
		VALUES ($1, 'Retention Production Test Co', $2, 'NG', 'Africa/Lagos', 'NGN', 'ACTIVE', now(), now())`,
		orgID, "RC-RETPROD-"+orgID); err != nil {
		t.Fatalf("seed organization: %v", err)
	}
	exportID := ids.New("exp")
	if _, err := pool.Exec(ctx, `
		INSERT INTO fleet_partner_export_jobs
			(id, organization_id, export_type, format, status, media_file_id, expires_at, created_at, updated_at)
		VALUES ($1, $2, 'ORDERS', 'CSV', 'EXPIRED', 'med_prod_test_1', now() - interval '2 days', now(), now())`,
		exportID, orgID); err != nil {
		t.Fatalf("seed expired export: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM fleet_partner_export_jobs WHERE id = $1`, exportID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM fleet_partner_organizations WHERE id = $1`, orgID)
	})

	admin := retentionexec.NewAdministrator(pool, components.RetentionRunner, jobs.NewQueue(pool), nil, nil, nil)
	run, err := admin.Initiate(ctx, retentionexec.InitiateInput{
		Actor:          retentionexec.Actor{OrganizationID: orgID, ActorType: "organization_user", ActorID: "test_owner"},
		RetentionClass: retentionpolicymodels.ClassTemporary, Mode: retentionexec.ModeDryRun,
		IdempotencyKey: "production-wiring-proof-1",
	})
	if err != nil {
		t.Fatalf("initiate dry run through real production wiring: %v", err)
	}
	if run.RecordsArchived != 1 {
		t.Fatalf("recordsArchived = %d, want 1 -- the real registered ExpiredExportExecutor must have previewed the seeded export", run.RecordsArchived)
	}
}
