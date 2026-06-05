package main

import (
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/config"
	"cosmicforge/logistics/shared/go/serviceapp"
)

func main() {
	cfg := config.Load()

	serviceapp.Run(serviceapp.Options{
		Name:        "dispatch-delivery-service",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/dispatch-delivery",
		Capabilities: []string{
			"dispatch rider auth entry using shared auth helpers",
			"dispatch rider profiles and bike records",
			"package delivery bookings",
			"dispatch rider matching",
			"proof of delivery workflow",
		},
	})
}
