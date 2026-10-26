package main

import (
	"context"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/bridge"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/clients"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/config"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/customerreviewops"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/customerworkitemops"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/hqevents"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/hqops"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/ownercommandops"
	"cosmicforge/logistics/services/admin-backoffice-service/internal/persistence"
	"cosmicforge/logistics/services/admin-backoffice-service/migrations"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminpolicyenv"
	"cosmicforge/logistics/shared/go/serviceauth"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"go/version"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--probe" {
		client := http.Client{Timeout: 3 * time.Second}
		res, err := client.Get("http://127.0.0.1:8116/ready")
		if err != nil {
			os.Exit(1)
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && !(len(os.Args) == 2 && os.Args[1] == "--migrate") {
		os.Exit(2)
	}
	// Structured, sanitized logs: correlation ids and outcome codes only; no
	// request bodies, tokens, nonces, OTPs, coordinates or bank data (Section 27).
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "admin-backoffice-service", "product", "karryam", "build_sha", buildSHA()))
	if err := run(); err != nil {
		slog.Error("admin_bridge_startup_failed")
		os.Exit(1)
	}
}

func buildSHA() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && len(setting.Value) >= 7 {
				return setting.Value
			}
		}
	}
	return "unknown"
}

func run() error {
	if version.Compare(runtime.Version(), "go1.26.9") < 0 {
		return errors.New("bridge requires patched Go 1.26.9 or newer")
	}
	var cfg config.Config
	var err error
	if len(os.Args) == 2 && os.Args[1] == "--migrate" {
		cfg.DatabaseURL = os.Getenv("ADMIN_BACKOFFICE_DATABASE_URL")
		if cfg.DatabaseURL == "" {
			return errors.New("migration database required")
		}
	} else {
		cfg, err = config.Load()
		if err != nil {
			return err
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return errors.New("invalid database configuration")
	}
	poolCfg.MaxConns = 10
	poolCfg.ConnConfig.ConnectTimeout = 3 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return errors.New("database unavailable")
	}
	defer pool.Close()
	if len(os.Args) == 2 && os.Args[1] == "--migrate" {
		migrationCtx, done := context.WithTimeout(ctx, 30*time.Second)
		defer done()
		return migrations.Apply(migrationCtx, pool)
	}
	repo := &persistence.Store{Pool: pool}
	readyCtx, done := context.WithTimeout(ctx, 5*time.Second)
	err = repo.Ready(readyCtx)
	done()
	if err != nil {
		return errors.New("bridge schema unavailable; apply migrations with migration role")
	}
	roleCtx, roleDone := context.WithTimeout(ctx, 3*time.Second)
	err = repo.ValidateRuntime(roleCtx)
	roleDone()
	if err != nil {
		return errors.New("unsafe runtime database role")
	}
	tlsConfig, err := cfg.TLS()
	if err != nil {
		return err
	}
	app, err := bridge.New(repo, cfg.Trust, cfg.RemoteEnabled)
	if err != nil {
		return err
	}
	app.ConfigureDomainFlags(cfg.Features)
	if len(cfg.OwnerResources) > 0 {
		approved, e := adminpolicy.LoadFiles(cfg.AccessPolicyFile, cfg.AccessPolicyKeysFile, adminpolicyenv.Environment, time.Now())
		if e != nil && !errors.Is(e, adminpolicy.ErrBlocked) {
			return errors.New("invalid owner-approved resource policy")
		}
		if approved != nil {
			if e = adminpolicy.PinRevision(ctx, pool, "admin_policy_revisions", approved); e != nil {
				return errors.New("resource access policy revision refused")
			}
		}
		for _, owner := range cfg.OwnerResources {
			encoded, e := os.ReadFile(owner.SecretFile)
			if e != nil {
				return errors.New("owner resource secret file unavailable")
			}
			if len(encoded) > 4096 {
				return errors.New("invalid owner resource secret file")
			}
			secret, e := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(encoded)))
			if e != nil {
				return errors.New("invalid owner resource secret encoding")
			}
			client, e := clients.NewResources(owner.Owner, owner.Origin, secret)
			if e != nil {
				return e
			}
			defer client.Close()
			if e = app.ConfigureResources(client, approved); e != nil {
				return e
			}
		}
	}
	if client, write, read, e := cfg.CustomerReviewClient(); e != nil {
		return e
	} else if client != nil {
		defer client.Close()
		approved, e := adminpolicy.LoadFiles(cfg.AccessPolicyFile, cfg.AccessPolicyKeysFile, adminpolicyenv.Environment, time.Now())
		if e != nil && !errors.Is(e, adminpolicy.ErrBlocked) {
			return errors.New("invalid Customer review policy")
		}
		if e = app.ConfigureCustomerReview(&customerreviewops.Service{DB: pool, Owner: client, Policy: approved, WriteEnabled: write, ReadEnabled: read}); e != nil {
			return e
		}
		workWrite := os.Getenv("ADMIN_CUSTOMER_REVIEW_WORK_ITEM_WRITES_ENABLED") == "true"
		workRead := os.Getenv("ADMIN_CUSTOMER_REVIEW_WORK_ITEM_READBACK_ENABLED") == "true"
		if workWrite || workRead {
			var ready bool
			if e := pool.QueryRow(ctx, `SELECT to_regclass('admin_customer_work_item_operations') IS NOT NULL AND to_regclass('admin_customer_work_item_operation_audit') IS NOT NULL`).Scan(&ready); e != nil || !ready {
				return errors.New("Customer work-item orchestration schema unavailable")
			}
			if e := app.ConfigureCustomerWorkItems(&customerworkitemops.Service{DB: pool, Owner: client, Policy: approved, WriteEnabled: workWrite, ReadEnabled: workRead}); e != nil {
				return e
			}
		}

	}
	if owners, e := cfg.OwnerCommandClients(); e != nil {
		return e
	} else if len(owners) > 0 {
		var ready bool
		if e := pool.QueryRow(ctx, `SELECT to_regclass('admin_owner_command_operations') IS NOT NULL AND to_regclass('admin_owner_command_operation_audit') IS NOT NULL AND to_regclass('admin_command_approvals') IS NOT NULL AND to_regclass('admin_command_approval_audit') IS NOT NULL`).Scan(&ready); e != nil || !ready {
			return errors.New("owner command orchestration schema unavailable")
		}
		approved, e := adminpolicy.LoadFiles(cfg.AccessPolicyFile, cfg.AccessPolicyKeysFile, adminpolicyenv.Environment, time.Now())
		if e != nil && !errors.Is(e, adminpolicy.ErrBlocked) {
			return errors.New("invalid owner command policy")
		}
		if approved != nil {
			if e = adminpolicy.PinRevision(ctx, pool, "admin_policy_revisions", approved); e != nil {
				return errors.New("owner command policy revision refused")
			}
		}
		for _, owner := range owners {
			defer owner.Client.Close()
			if e := app.ConfigureOwnerCommands(&ownercommandops.Service{DB: pool, Owner: owner.Client, Policy: approved, WriteEnabled: owner.WriteEnabled, ReadEnabled: owner.ReadEnabled}); e != nil {
				return e
			}
		}
	}
	if client, e := cfg.ScopedAnalyticsClient(); e != nil {
		return e
	} else if client != nil {
		defer client.Close()
		approved, e := adminpolicy.LoadFiles(cfg.AccessPolicyFile, cfg.AccessPolicyKeysFile, adminpolicyenv.Environment, time.Now())
		if e != nil && !errors.Is(e, adminpolicy.ErrBlocked) {
			return errors.New("invalid scoped Analytics policy")
		}
		if approved != nil {
			if e = adminpolicy.PinRevision(ctx, pool, "admin_policy_revisions", approved); e != nil {
				return errors.New("scoped Analytics policy revision refused")
			}
		}
		if e = app.ConfigureScopedAnalytics(client, approved, os.Getenv("ADMIN_ANALYTICS_SCOPED_RATE_READ_ENABLED") == "true"); e != nil {
			return e
		}
	}
	if cfg.PricingReadEnabled || cfg.Features.PricingWrites {
		encoded, e := os.ReadFile(cfg.PricingSecretFile)
		if e != nil || len(encoded) > 4096 {
			return errors.New("Pricing Admin secret file unavailable")
		}
		secret, e := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(encoded)))
		if e != nil {
			return errors.New("invalid Pricing Admin secret encoding")
		}
		pricing, e := clients.NewPricing(cfg.PricingOrigin, secret)
		if e != nil {
			return e
		}
		defer pricing.Close()
		approved, e := adminpolicy.LoadFiles(cfg.AccessPolicyFile, cfg.AccessPolicyKeysFile, adminpolicyenv.Environment, time.Now())
		if e != nil && !errors.Is(e, adminpolicy.ErrBlocked) {
			return errors.New("invalid Pricing Admin policy")
		}
		if approved != nil {
			if e = adminpolicy.PinRevision(ctx, pool, "admin_policy_revisions", approved); e != nil {
				return errors.New("Pricing Admin policy revision refused")
			}
		}
		if e = app.ConfigurePricing(pricing, approved, cfg.PricingReadEnabled, cfg.Features.PricingWrites); e != nil {
			return e
		}
	}
	if cfg.TaxiReadEnabled {
		encoded, e := os.ReadFile(cfg.TaxiSecretFile)
		if e != nil {
			return errors.New("Taxi secret file unavailable")
		}
		secret, e := base64.RawURLEncoding.DecodeString(strings.TrimSpace(string(encoded)))
		if e != nil {
			return errors.New("invalid Taxi secret encoding")
		}
		taxi, e := clients.NewTaxi(cfg.TaxiOrigin, secret)
		if e != nil {
			return e
		}
		defer taxi.Close()
		approved, e := adminpolicy.LoadFiles(cfg.AccessPolicyFile, cfg.AccessPolicyKeysFile, adminpolicyenv.Environment, time.Now())
		if e != nil && !errors.Is(e, adminpolicy.ErrBlocked) {
			return errors.New("invalid owner-approved access policy")
		}
		if approved != nil {
			if e = adminpolicy.PinRevision(ctx, pool, "admin_policy_revisions", approved); e != nil {
				return errors.New("access policy revision refused")
			}
		}
		if e = app.ConfigureTaxi(taxi, cfg.TaxiReadRoles, approved); e != nil {
			return e
		}
	}
	if client, reads, revokes, e := cfg.MediaCaseClient(); e != nil {
		return e
	} else if client != nil {
		defer client.Close()
		var approved *adminpolicy.Engine
		if reads {
			approved, e = adminpolicy.LoadFiles(cfg.AccessPolicyFile, cfg.AccessPolicyKeysFile, adminpolicyenv.Environment, time.Now())
			if e != nil && !errors.Is(e, adminpolicy.ErrBlocked) {
				return errors.New("invalid Media case policy")
			}
			if approved != nil {
				if e = adminpolicy.PinRevision(ctx, pool, "admin_policy_revisions", approved); e != nil {
					return errors.New("Media case policy revision refused")
				}
			}
		}
		if e = app.ConfigureMediaCases(client, approved, reads, revokes); e != nil {
			return e
		}
	}
	hqCfg, err := cfg.LoadHQ()
	if err != nil {
		return err
	}
	var ingress *http.Server
	if hqCfg.Events || hqCfg.SupportQueue || hqCfg.Approvals || hqCfg.Campaigns || hqCfg.Finance || hqCfg.Privacy || hqCfg.Exports || hqCfg.IngressEnabled || hqCfg.BackfillEnabled {
		var ready bool
		if e := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM admin_schema_versions WHERE version=7)`).Scan(&ready); e != nil || !ready {
			return errors.New("HQ integration schema unavailable; apply migration 007")
		}
		store := &hqevents.Store{DB: pool}
		metrics := &hqevents.Metrics{}
		ops := &hqops.Service{DB: pool, CampaignReads: hqCfg.Campaigns, CampaignWrites: hqCfg.CampaignWrites, FinanceReads: hqCfg.Finance, PrivacyWrites: hqCfg.Privacy, ExportKey: hqCfg.ExportKey, ExportKeyID: hqCfg.ExportKeyID}
		if hqCfg.Campaigns {
			c, e := clients.NewCampaigns(hqCfg.NotificationOrigin, hqCfg.CampaignSecret)
			if e != nil {
				return e
			}
			defer c.Close()
			ops.Campaigns = c
		}
		if hqCfg.Finance {
			c, e := clients.NewFinance(hqCfg.FinanceOrigin, hqCfg.FinanceSecret)
			if e != nil {
				return e
			}
			defer c.Close()
			ops.Finance = c
		}
		approved, e := adminpolicy.LoadFiles(cfg.AccessPolicyFile, cfg.AccessPolicyKeysFile, adminpolicyenv.Environment, time.Now())
		if e != nil && !errors.Is(e, adminpolicy.ErrBlocked) {
			return errors.New("invalid HQ integration policy")
		}
		if approved != nil {
			if e = adminpolicy.PinRevision(ctx, pool, "admin_policy_revisions", approved); e != nil {
				return errors.New("HQ integration policy revision refused")
			}
		}
		if e = app.ConfigureHQ(store, ops, approved, bridge.HQFlags{Events: hqCfg.Events, SupportQueue: hqCfg.SupportQueue, Approvals: hqCfg.Approvals, Campaigns: hqCfg.Campaigns, Finance: hqCfg.Finance, Privacy: hqCfg.Privacy, Exports: hqCfg.Exports, MediaMapping: hqCfg.MediaMapping}); e != nil {
			return e
		}
		app.EventMetrics = metrics
		if hqCfg.IngressEnabled {
			raw, e := os.ReadFile(hqCfg.SourceSecretsFile)
			if e != nil {
				return errors.New("event source secrets unavailable")
			}
			secrets, e := hqevents.SourceSecrets(raw)
			if e != nil {
				return e
			}
			for _, secret := range secrets {
				for _, encoded := range cfg.Trust.ServiceKeys {
					hqKey, _ := base64.RawURLEncoding.DecodeString(encoded)
					if string(hqKey) == string(secret) {
						return errors.New("event source secret reused an HQ workload key")
					}
				}
			}
			handler := &hqevents.IngressHandler{Store: store, Enabled: true, Metrics: metrics, Verifier: serviceauth.NewVerifier(secrets, 30*time.Second)}
			ingress = &http.Server{Addr: hqCfg.IngressAddr, Handler: handler, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
		}
		if hqCfg.BackfillEnabled && len(hqCfg.Snapshots) > 0 {
			var sources []hqevents.SnapshotSource
			for owner, src := range hqCfg.Snapshots {
				c, e := hqevents.NewSnapshotClient(owner, src.Origin, src.Secret)
				if e != nil {
					return e
				}
				sources = append(sources, c)
			}
			go hqevents.RunSweeps(ctx, store, sources, hqCfg.ReconcileInterval)
		}
	}
	srv := &http.Server{Addr: cfg.Address, Handler: app, TLSConfig: tlsConfig, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 32768}
	probe := &http.Server{Addr: "127.0.0.1:8116", Handler: http.HandlerFunc(app.Probe), ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 4096}
	failures := make(chan error, 3)
	if ingress != nil {
		go func() { failures <- ingress.ListenAndServe() }()
		defer ingress.Close()
	}
	go func() { failures <- srv.ListenAndServeTLS("", "") }()
	go func() { failures <- probe.ListenAndServe() }()
	slog.Info("admin_bridge_started", "contract_version", "1.0", "remote_enabled", cfg.RemoteEnabled, "remote_writes", "DISABLED")
	leaf, _ := x509.ParseCertificate(tlsConfig.Certificates[0].Certificate[0])
	slog.Info("admin_credential_expiry", "certificate_expires_at", leaf.NotAfter)
	for kid, expiry := range cfg.Trust.StaffKeyExpires {
		slog.Info("admin_credential_expiry", "kind", "staff_public_key", "key_id", kid, "expires_at", time.Unix(expiry, 0).UTC())
	}
	for kid, expiry := range cfg.Trust.ServiceKeyExpires {
		slog.Info("admin_credential_expiry", "kind", "workload_signing_key", "key_id", kid, "expires_at", time.Unix(expiry, 0).UTC())
	}
	select {
	case <-ctx.Done():
	case err = <-failures:
	}
	shutdownCtx, stop := context.WithTimeout(context.Background(), 20*time.Second)
	defer stop()
	_ = srv.Shutdown(shutdownCtx)
	_ = probe.Shutdown(shutdownCtx)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
