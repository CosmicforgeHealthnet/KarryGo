package main

import (
	"karrygo/services/payment-wallet-service/internal/config"
	"karrygo/shared/go/serviceapp"
)

func main() {
	cfg := config.Load()

	serviceapp.Run(serviceapp.Options{
		Name:        "payment-wallet-service",
		DefaultAddr: cfg.HTTPAddr,
		APIBase:     "/api/v1/payment-wallet",
		Capabilities: []string{
			"customer wallets",
			"payments and refunds",
			"provider earnings",
			"withdrawals",
			"fleet settlement",
		},
	})
}
