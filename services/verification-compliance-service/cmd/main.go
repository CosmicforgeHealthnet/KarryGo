package main

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"cosmicforge/logistics/services/verification-compliance-service/internal/config"
	"cosmicforge/logistics/services/verification-compliance-service/internal/database"
	organizations "cosmicforge/logistics/services/verification-compliance-service/internal/features/organizations"
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
			organizations.Register(group, organizations.New(db, cfg.PrivateAPIEnabled), serviceauth.NewVerifier(serviceauth.ParseSecrets(cfg.ServiceSecrets), 5*time.Minute), allowed)
		},
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
