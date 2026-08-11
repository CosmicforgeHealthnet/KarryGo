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
	organizations "cosmicforge/logistics/services/verification-compliance-service/internal/features/organizations"
	vendors "cosmicforge/logistics/services/verification-compliance-service/internal/features/vendors"
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
		},
		Workers: workers,
	})
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
