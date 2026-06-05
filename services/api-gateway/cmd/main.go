package main

import (
	"cosmicforge/logistics/services/api-gateway/internal/config"
	"cosmicforge/logistics/shared/go/serviceapp"
)

func main() {
	cfg := config.Load()

	serviceapp.Run(serviceapp.Options{
		Name:        "api-gateway",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/gateway",
		Capabilities: []string{
			"routes app traffic to the correct service",
			"centralizes request IDs and edge health checks",
			"keeps mobile and admin apps from calling internal services directly",
		},
	})
}
