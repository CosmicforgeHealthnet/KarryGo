package main

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"cosmicforge/logistics/services/verification-compliance-service/internal/config"
	"cosmicforge/logistics/services/verification-compliance-service/internal/database"
	"cosmicforge/logistics/services/verification-compliance-service/internal/features/admincommands"
	"cosmicforge/logistics/services/verification-compliance-service/internal/features/adminresources"
	"cosmicforge/logistics/services/verification-compliance-service/internal/features/adminsnapshot"
	assetcapability "cosmicforge/logistics/services/verification-compliance-service/internal/features/assetcapability"
	organizations "cosmicforge/logistics/services/verification-compliance-service/internal/features/organizations"
	vendors "cosmicforge/logistics/services/verification-compliance-service/internal/features/vendors"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/adminevents"
	"cosmicforge/logistics/shared/go/mediaclient"
	"cosmicforge/logistics/shared/go/serviceapp"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		panic(err)
	}
	ctx := context.Background()
	db, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	if cfg.Migration {
		mctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		if err := database.Apply(mctx, db, "migrations"); err != nil {
			panic(err)
		}
	}
	allowed := split(cfg.AllowedCallers)
	workers := []func(context.Context) error{}
	if cfg.VendorCallbackURL != "" {
		callbackWorker := vendors.NewCallbackWorker(db, cfg.VendorCallbackURL, cfg.VendorCallbackSecret, 5*time.Second)
		workers = append(workers, func(runCtx context.Context) error {
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for {
				if err := callbackWorker.Process(runCtx, "verification-vendor-callback", 25); err != nil && runCtx.Err() == nil {
					log.Printf("verification vendor callback worker error=%v", err)
				}
				select {
				case <-runCtx.Done():
					return runCtx.Err()
				case <-ticker.C:
				}
			}
		})
	}

	// Section 12: asset capability and service-tier certification.
	//
	// The service is built whether or not the flag is on, because reads must
	// keep answering during a rollout pause. Only the workers are conditional:
	// a delivery worker with no configured authority has nothing to deliver,
	// and a sweep with no certifications has nothing to lapse.
	assetRepo := assetcapability.NewRepository(db)
	var evidenceStore assetcapability.EvidenceStore
	if cfg.MediaBaseURL != "" {
		evidenceStore = mediaclient.New(mediaclient.Config{
			BaseURL:     cfg.MediaBaseURL,
			ServiceName: assetcapability.ServiceName,
			Token:       cfg.MediaToken,
		})
	}
	assetService := assetcapability.NewService(assetcapability.Options{
		Repository: assetRepo,
		Evidence:   assetcapability.NewEvidenceValidator(evidenceStore),
		Enabled:    cfg.AssetCapabilityCertificationEnabled,
	})
	assetHandler := assetcapability.NewHandler(assetService, db, allowed)

	certificationWorker := assetcapability.NewCallbackWorker(db, assetCallbackEndpoints(cfg), 5*time.Second)
	if len(certificationWorker.Targets()) > 0 {
		workers = append(workers, func(runCtx context.Context) error {
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for {
				if err := certificationWorker.Process(runCtx, "verification-certification-callback", 25); err != nil && runCtx.Err() == nil {
					log.Printf("verification certification callback worker error=%v", err)
				}
				select {
				case <-runCtx.Done():
					return runCtx.Err()
				case <-ticker.C:
				}
			}
		})
	}
	workers = append(workers, func(runCtx context.Context) error {
		ticker := time.NewTicker(assetcapability.ExpirySweepInterval)
		defer ticker.Stop()
		for {
			if _, err := assetService.RunExpirySweep(runCtx); err != nil && runCtx.Err() == nil {
				log.Printf("verification certification expiry worker error=%v", err)
			}
			select {
			case <-runCtx.Done():
				return runCtx.Err()
			case <-ticker.C:
			}
		}
	})

	// Section 18: automated pre-verification (advisory only) and vendor case
	// expiry. Both default off.
	if cfg.AutomationEnabled {
		automation := &vendors.Automation{DB: db, External: vendors.DisabledAuthority{}, Owner: "verification-automation"}
		if cfg.MediaBaseURL != "" {
			automation.Media = mediaclient.New(mediaclient.Config{BaseURL: cfg.MediaBaseURL, ServiceName: assetcapability.ServiceName, Token: cfg.MediaToken})
		}
		workers = append(workers, tickerWorker("verification vendor automation", time.Minute, func(runCtx context.Context) error {
			_, err := automation.RunOnce(runCtx, 25)
			return err
		}))
	}
	if cfg.CaseExpiryEnabled {
		workers = append(workers, tickerWorker("verification vendor case expiry", 5*time.Minute, func(runCtx context.Context) error {
			_, err := vendors.RunExpirySweep(runCtx, db, 100, nil)
			return err
		}))
	}

	// Section 18: Secure Admin owner boundary. Every switch defaults off; with
	// all of them off no credential, policy or schema is touched.
	verificationAdmin, err := adminresources.Runtime(ctx, db)
	if err != nil {
		log.Fatalf("verification secure admin configuration invalid: %v", err)
	}
	verificationCommands, err := admincommand.Runtime(verificationAdmin, admincommands.Spec(), admincommands.Executor{}, adminresources.Prefix)
	if err != nil {
		log.Fatalf("verification secure admin command configuration invalid: %v", err)
	}
	snapshot, err := adminsnapshot.FromEnvironment(db)
	if err != nil {
		log.Fatalf("verification admin event snapshot configuration invalid: %v", err)
	}
	relay, err := adminevents.RelayFromEnvironment(db, adminresources.Prefix, "verification-compliance-service")
	if err != nil {
		log.Fatalf("verification admin event relay configuration invalid: %v", err)
	}
	if relay != nil {
		workers = append(workers, relay.Run)
	}

	serviceapp.Run(serviceapp.Options{
		Name:        "verification-compliance-service",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/verification-compliance",
		Capabilities: []string{
			"id checks",
			"driver license checks",
			"vehicle document checks",
			"provider verification status",
		},
		ReadyChecks: []func(context.Context) error{db.Ping},
		Register: func(group *gin.RouterGroup) {
			verifier := serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute)
			organizations.Register(group, organizations.New(db, cfg.PrivateAPIEnabled), verifier, allowed)
			vendors.Register(group, vendors.New(db, cfg.PrivateAPIEnabled, allowed), verifier)
			assetcapability.Register(group, assetHandler, verifier)
		},
		RegisterRoot: func(router *gin.Engine) {
			verificationAdmin.RegisterOn(router.Group(""))
			verificationCommands.Register(router)
			snapshot.Register(router)
		},
		Workers: workers,
	})
}

// assetCallbackEndpoints maps each authority to its certification ingress.
//
// The path per authority is fixed rather than configurable: it is part of the
// contract those three services already publish, and letting a deployment
// override it would turn a wrong environment variable into a silently
// undelivered certification.
func assetCallbackEndpoints(cfg config.Config) map[assetcapability.Authority]assetcapability.AuthorityEndpoint {
	paths := assetcapability.DefaultPaths()
	return map[assetcapability.Authority]assetcapability.AuthorityEndpoint{
		assetcapability.AuthorityTaxi: {
			BaseURL: cfg.TaxiCallbackURL, Path: paths[assetcapability.AuthorityTaxi],
			Secret: []byte(cfg.TaxiCallbackSecret),
		},
		assetcapability.AuthorityDispatch: {
			BaseURL: cfg.DispatchCallbackURL, Path: paths[assetcapability.AuthorityDispatch],
			Secret: []byte(cfg.DispatchCallbackSecret),
		},
		assetcapability.AuthorityHauling: {
			BaseURL: cfg.HaulingCallbackURL, Path: paths[assetcapability.AuthorityHauling],
			Secret: []byte(cfg.HaulingCallbackSecret),
		},
	}
}

func tickerWorker(name string, every time.Duration, pass func(context.Context) error) func(context.Context) error {
	return func(runCtx context.Context) error {
		ticker := time.NewTicker(every)
		defer ticker.Stop()
		for {
			if err := pass(runCtx); err != nil && runCtx.Err() == nil {
				log.Printf("%s worker error=%v", name, err)
			}
			select {
			case <-runCtx.Done():
				return runCtx.Err()
			case <-ticker.C:
			}
		}
	}
}

func split(v string) []string {
	var out []string
	for _, value := range strings.Split(v, ",") {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	return out
}
