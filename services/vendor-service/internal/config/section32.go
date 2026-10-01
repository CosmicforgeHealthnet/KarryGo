package config

// Section32TaxiHandoverEnabled is independent of execution enablement. The
// default exposes no relay while each authority integration is certified.
func Section32TaxiHandoverEnabled() bool {
	return getEnvBool("VENDOR_TAXI_HANDOVER_RELAY_ENABLED", false)
}

func Section32DispatchHandoverEnabled() bool {
	return getEnvBool("VENDOR_DISPATCH_HANDOVER_RELAY_ENABLED", false)
}

func Section32HaulingHandoverEnabled() bool {
	return getEnvBool("VENDOR_HAULING_HANDOVER_RELAY_ENABLED", false)
}

func Section32VendorOwnedHandoverEnabled() bool {
	return getEnvBool("VENDOR_OWNED_HANDOVER_RELAY_ENABLED", false)
}
