package main

import (
	"testing"

	"cosmicforge/logistics/services/vendor-service/internal/config"
	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
)

func TestCustomerDomainGateNeedsBothSwitchesAndNoKillSwitch(t *testing.T) {
	var cfg config.Config
	gate := customerDomainGate(cfg)
	if gate(cart.DomainFood) || gate(cart.DomainShop) {
		t.Fatal("default gate must be closed")
	}
	cfg.Flags.CommerceFoodEnabled, cfg.Flags.CustomerFoodEnabled = true, true
	cfg.Flags.CommerceShopEnabled = true // domain live for vendors only
	gate = customerDomainGate(cfg)
	if !gate(cart.DomainFood) || gate(cart.DomainShop) {
		t.Fatal("food should be open and shop closed")
	}
	cfg.Flags.CommerceGlobalKillSwitch = true
	if customerDomainGate(cfg)(cart.DomainFood) {
		t.Fatal("kill switch ignored")
	}
}
