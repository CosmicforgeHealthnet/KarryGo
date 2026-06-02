package main

import (
	"karrygo/services/admin-backoffice-service/internal/config"
	"karrygo/shared/go/serviceapp"
)

func main() {
	cfg := config.Load()

	serviceapp.Run(serviceapp.Options{
		Name:        "admin-backoffice-service",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/admin",
		Capabilities: []string{
			"admin dashboards",
			"user actions",
			"operational monitoring",
			"moderation workflows",
		},
	})
}
