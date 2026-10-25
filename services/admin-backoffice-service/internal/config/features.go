package config

import (
	"os"
	"strings"
)

// DomainFlags request rollout, never grant authorization. Owner adapters,
// approved policy, source facts and command approval remain separate gates.
type DomainFlags struct {
	AnalyticsReads         bool
	CustomerSensitiveReads bool
	CustomerWrites         bool
	TaxiWrites             bool
	DispatchRecovery       bool
	HaulingRecovery        bool
	FleetWrites            bool
	VendorWrites           bool
	WalletWrites           bool
	RemoteWrites           bool
}

func loadDomainFlags() DomainFlags {
	return DomainFlags{
		AnalyticsReads:         os.Getenv("ADMIN_ANALYTICS_READ_ENABLED") == "true",
		CustomerSensitiveReads: os.Getenv("ADMIN_CUSTOMER_SENSITIVE_READ_ENABLED") == "true",
		CustomerWrites:         os.Getenv("ADMIN_CUSTOMER_WRITES_ENABLED") == "true",
		TaxiWrites:             os.Getenv("ADMIN_TAXI_WRITES_ENABLED") == "true",
		DispatchRecovery:       os.Getenv("ADMIN_DISPATCH_RECOVERY_ENABLED") == "true",
		HaulingRecovery:        os.Getenv("ADMIN_HAULING_RECOVERY_ENABLED") == "true",
		FleetWrites:            os.Getenv("ADMIN_FLEET_WRITES_ENABLED") == "true",
		VendorWrites:           os.Getenv("ADMIN_VENDOR_WRITES_ENABLED") == "true",
		WalletWrites:           os.Getenv("ADMIN_WALLET_WRITES_ENABLED") == "true",
		RemoteWrites:           os.Getenv("ADMIN_REMOTE_WRITES_ENABLED") == "true",
	}
}
func (f DomainFlags) WriteFamilyRequested(action string) bool {
	if !f.RemoteWrites {
		return false
	}
	switch {
	case strings.HasPrefix(action, "customer."):
		return f.CustomerWrites
	case strings.HasPrefix(action, "taxi."):
		return f.TaxiWrites
	case strings.HasPrefix(action, "dispatch."):
		return f.DispatchRecovery
	case strings.HasPrefix(action, "hauling."):
		return f.HaulingRecovery
	case strings.HasPrefix(action, "vendor."):
		return f.VendorWrites
	case strings.HasPrefix(action, "fleet."):
		return f.FleetWrites
	case strings.HasPrefix(action, "wallet."):
		return f.WalletWrites
	}
	return false
}
