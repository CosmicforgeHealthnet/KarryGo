package main

import (
	"bytes"
	"cosmicforge/logistics/services/vendor-service/internal/config"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/serviceauth"
	"errors"
)

// New Admin audiences never reuse application, standing, or integration keys.
// Only already-loaded keys participate; disabled channels open no secret files.
func vendorAdminCredentialIsolation(cfg config.Config, owner *adminowner.Handler, commands *admincommand.Handler) error {
	keys := [][]byte{}
	if owner != nil {
		keys = append(keys, owner.Secret, owner.PublishSecret)
		if owner.Cases != nil {
			keys = append(keys, owner.Cases.Secret)
			if owner.Cases.Publisher != nil {
				keys = append(keys, owner.Cases.Publisher.PublishSecret)
			}
		}
	}
	if commands != nil {
		keys = append(keys, commands.WriteSecret, commands.ReadSecret)
	}
	known := [][]byte{cfg.CustomerAccessTokenSecret, cfg.Auth.AccessTokenSecret, cfg.Auth.RefreshTokenSecret, cfg.Auth.OTPSecret}
	for _, value := range []string{cfg.CustomerStandingSecret, cfg.CustomerSecret, cfg.WalletSecret, cfg.VerificationSecret, cfg.MediaSecret, cfg.NotificationSecret, cfg.SupportSecret, cfg.AdminSecret, cfg.AnalyticsSecret, cfg.DispatchSecret, cfg.TaxiSecret, cfg.HaulingSecret} {
		known = append(known, []byte(value))
	}
	for _, value := range serviceauth.ParseSecrets(cfg.ServiceSecrets) {
		known = append(known, value)
	}
	for i, key := range keys {
		if len(key) == 0 {
			continue
		}
		for _, other := range keys[:i] {
			if len(other) > 0 && bytes.Equal(key, other) {
				return errors.New("Vendor private Admin credentials require separate audiences")
			}
		}
		for _, other := range known {
			if len(other) > 0 && bytes.Equal(key, other) {
				return errors.New("Vendor private Admin credentials must differ from application credentials")
			}
		}
	}
	return nil
}
