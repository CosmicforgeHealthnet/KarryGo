package main

import (
	"cosmicforge/logistics/services/taxi-service/internal/config"
	"cosmicforge/logistics/shared/go/serviceapp"
)

func main() {
	cfg := config.Load()

	serviceapp.Run(serviceapp.Options{
		Name:        "taxi-service",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/taxi",
		Capabilities: []string{
			"taxi provider auth entry using shared auth helpers",
			"taxi provider profiles and car records",
			"taxi ride bookings",
			"taxi provider matching",
			"taxi trip lifecycle",
		},
	})
}
