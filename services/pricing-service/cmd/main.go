// Command pricing-service is KarryGo's central pricing authority.
//
// It decides the economic price of a trip, delivery or haulage job and how that
// amount is split between customer, merchant, provider, KarryGo and tax. It does
// not hold money, own bookings or call a routing provider: callers supply a
// trusted route snapshot, the wallet executes the split, and the booking service
// keeps the trip.
//
// Booking services ask it for quotes over signed internal routes; operators
// administer policy through signed admin routes. Nothing here is public.
package main

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"cosmicforge/logistics/services/pricing-service/internal/config"
	"cosmicforge/logistics/services/pricing-service/internal/database"
	approvalrepos "cosmicforge/logistics/services/pricing-service/internal/features/approvals/repositories"
	approvalusecases "cosmicforge/logistics/services/pricing-service/internal/features/approvals/usecases"
	policyhttp "cosmicforge/logistics/services/pricing-service/internal/features/policies/http"
	policyrepos "cosmicforge/logistics/services/pricing-service/internal/features/policies/repositories"
	policyusecases "cosmicforge/logistics/services/pricing-service/internal/features/policies/usecases"
	quotehttp "cosmicforge/logistics/services/pricing-service/internal/features/quotes/http"
	quoterepos "cosmicforge/logistics/services/pricing-service/internal/features/quotes/repositories"
	quoteusecases "cosmicforge/logistics/services/pricing-service/internal/features/quotes/usecases"
	rolloutrepos "cosmicforge/logistics/services/pricing-service/internal/features/rollouts/repositories"
	"cosmicforge/logistics/services/pricing-service/internal/jobs"
	"cosmicforge/logistics/services/pricing-service/internal/platform/audit"
	"cosmicforge/logistics/services/pricing-service/internal/platform/idempotency"
	"cosmicforge/logistics/services/pricing-service/internal/platform/operators"
	"cosmicforge/logistics/services/pricing-service/internal/platform/servicetrust"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/serviceapp"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		logging.Fatal("configuration", "%v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		logging.Fatal("database", "create pool: %v", err)
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		logging.Error("database", "ping failed (service will report not-ready): %v", err)
	} else {
		logging.Success("database", "connected")
	}

	if cfg.Migration {
		migrationCtx, migrationCancel := context.WithTimeout(ctx, 60*time.Second)
		if err := database.ApplyMigrations(migrationCtx, db, "migrations"); err != nil {
			migrationCancel()
			logging.Fatal("migration", "apply pricing-service migrations: %v", err)
		}
		migrationCancel()
		logging.Success("migration", "applied successfully")
	}

	if len(cfg.ServiceSecrets) == 0 {
		logging.Notice("service-auth", "PRICING_SERVICE_SECRETS is empty: no service can request a quote yet")
	}
	if !cfg.FourEyeRequired {
		logging.Notice("governance", "PRICING_FOUR_EYE_REQUIRED is false: one operator can approve their own change (never permitted in production)")
	}

	// Readiness includes "are my migrations current". A replica running older
	// schema than its policy data expects must not take quote traffic.
	readyChecks := []func(context.Context) error{
		db.Ping,
		func(checkCtx context.Context) error { return database.MigrationsCurrent(checkCtx, db, "migrations") },
	}

	idempotencyStore := idempotency.NewStore(db, idempotency.DefaultExpiry)

	// Composition root. Stores are passed as factories so a policy change, its
	// approval decision, its lifecycle event and its audit row all commit in one
	// transaction.
	policyService := policyusecases.NewService(policyusecases.Options{
		Pool:  db,
		Store: func(q database.Querier) policyusecases.Store { return policyrepos.NewStore(q) },
		Approvals: func(q database.Querier) policyusecases.Approvals {
			return approvalusecases.NewService(approvalrepos.NewStore(q))
		},
		Ops:             operators.NewStore(db),
		Audit:           audit.NewPostgresRecorder(),
		FourEyeRequired: cfg.FourEyeRequired,
	})
	adminHandler := policyhttp.NewHandler(
		policyService,
		approvalusecases.NewService(approvalrepos.NewStore(db)),
		rolloutrepos.NewStore(db),
	)
	verifier := serviceauth.NewVerifier(cfg.ServiceSecrets, 5*time.Minute)

	quoteService := quoteusecases.NewService(quoterepos.NewStore(db), policyService, rolloutrepos.NewStore(db))

	onWorkerError := func(name string, err error) { logging.Error("worker", "%s: %v", name, err) }
	workers := []func(context.Context) error{
		func(runCtx context.Context) error {
			jobs.Job{Name: "nonce-prune", Interval: cfg.Workers.NoncePruneInterval, Pass: func(passCtx context.Context) error {
				return servicetrust.PruneNonces(passCtx, db)
			}}.Run(runCtx, onWorkerError)
			return nil
		},
		func(runCtx context.Context) error {
			jobs.Job{Name: "idempotency-prune", Interval: cfg.Workers.RetentionInterval, Pass: func(passCtx context.Context) error {
				return idempotencyStore.Prune(passCtx)
			}}.Run(runCtx, onWorkerError)
			return nil
		},
	}

	serviceapp.Run(serviceapp.Options{
		Name:        "pricing-service",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/pricing",
		Capabilities: []string{
			"pricing geography: country, state, market, zone",
			"versioned typed pricing policy with four-eye approval",
			"scheduled activation, rollback and market freeze",
			"price simulation against any version",
			"exact whole-kobo arithmetic",
			"append-only audit trail",
			"immutable quotes: issue, bind and reconcile against the original policy versions",
			"shadow comparison against the fares callers charge",
		},
		ReadyChecks: readyChecks,
		Register: func(group *gin.RouterGroup) {
			// Customers and drivers never call pricing. The public group carries
			// only the standard /meta that serviceapp provides.
			_ = group
		},
		RegisterRoot: func(router *gin.Engine) {
			policyhttp.RegisterAdminRoutes(router, policyhttp.AdminOptions{
				Handler:      adminHandler,
				Verifier:     verifier,
				NonceStore:   db,
				Idempotency:  idempotencyStore,
				AdminCallers: cfg.AdminCallers,
			})
			// Same signing and nonce checks, a different allowlist: a booking
			// service may price, never approve.
			quotehttp.RegisterRoutes(router, quotehttp.Options{
				Handler:      quotehttp.NewHandler(quoteService),
				Verifier:     verifier,
				NonceStore:   db,
				QuoteCallers: cfg.QuoteCallers,
				AdminCallers: cfg.AdminCallers,
			})
		},
		Workers: workers,
	})
}
