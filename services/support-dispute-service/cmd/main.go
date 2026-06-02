package main

import (
	"karrygo/services/support-dispute-service/internal/config"
	"karrygo/shared/go/serviceapp"
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
