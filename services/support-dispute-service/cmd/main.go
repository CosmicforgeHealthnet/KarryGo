package main

import (
	"cosmicforge/logistics/services/support-dispute-service/internal/config"
	"cosmicforge/logistics/shared/go/serviceapp"
)

func main() {
	cfg := config.Load()

	serviceapp.Run(serviceapp.Options{
		Name:        "support-dispute-service",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/support-disputes",
		Capabilities: []string{
			"complaints",
			"disputes",
			"evidence collection",
			"issue resolution status",
		},
	})
}
