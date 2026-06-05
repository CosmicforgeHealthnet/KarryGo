package main

import (
	"cosmicforge/logistics/services/media-file-service/internal/config"
	"cosmicforge/logistics/shared/go/serviceapp"
)

func main() {
	cfg := config.Load()

	serviceapp.Run(serviceapp.Options{
		Name:        "media-file-service",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/media-files",
		Capabilities: []string{
			"profile photos",
			"document uploads",
			"delivery proof images",
			"recipient signatures",
		},
	})
}
