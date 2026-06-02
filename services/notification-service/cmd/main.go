package main

import (
	"karrygo/services/notification-service/internal/config"
	"karrygo/shared/go/serviceapp"
)

func main() {
	cfg := config.Load()

	serviceapp.Run(serviceapp.Options{
		Name:        "notification-service",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/notifications",
		Capabilities: []string{
			"push notifications",
			"sms notifications",
			"email notifications",
			"in-app notifications",
			"retry handling",
		},
	})
}
