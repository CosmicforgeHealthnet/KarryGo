package main

import (
	"karrygo/services/hauling-service/internal/config"
	"karrygo/shared/go/serviceapp"
)

func main() {
	cfg := config.Load()

	serviceapp.Run(serviceapp.Options{
		Name:        "hauling-service",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/hauling",
		Capabilities: []string{
			"truck provider auth entry using shared auth helpers",
			"truck provider profiles and truck records",
			"haulage bookings",
			"truck matching",
			"cargo and haulage workflow",
		},
	})
}
