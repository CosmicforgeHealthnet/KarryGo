package main

import (
	"cosmicforge/logistics/services/verification-compliance-service/internal/config"
	"cosmicforge/logistics/shared/go/serviceapp"
)

func main() {
	cfg := config.Load()

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
	})
}
