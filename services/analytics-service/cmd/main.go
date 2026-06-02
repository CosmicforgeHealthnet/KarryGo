package main

import (
	"karrygo/services/analytics-service/internal/config"
	"karrygo/shared/go/serviceapp"
)

func main() {
	cfg := config.Load()

	serviceapp.Run(serviceapp.Options{
		Name:        "analytics-service",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/analytics",
		Capabilities: []string{
			"reports",
			"metrics",
			"revenue dashboards",
			"service performance dashboards",
		},
	})
}
