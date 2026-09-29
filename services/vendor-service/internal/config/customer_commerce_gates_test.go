package config

import "testing"

// Karryam V4 §24: the public Food/Shop contract gates and the external
// repricing cutover are release gates. They default off, cannot be enabled
// without their domain, and repricing cannot be enabled without an adapter.
func TestCustomerCommerceGates(t *testing.T) {
	cfg := withAuthSecrets(Load())
	if cfg.Flags.CustomerFoodEnabled || cfg.Flags.CustomerShopEnabled || cfg.Flags.ExternalReferenceRepricingEnabled {
		t.Fatal("customer commerce gates must default to false")
	}

	for name, mutate := range map[string]func(*Config){
		"food without domain":       func(c *Config) { c.Flags.CustomerFoodEnabled = true },
		"shop without domain":       func(c *Config) { c.Flags.CustomerShopEnabled = true },
		"repricing without adapter": func(c *Config) { c.Flags.ExternalReferenceRepricingEnabled = true },
	} {
		c := withAuthSecrets(Load())
		mutate(&c)
		if err := c.Validate(); err == nil {
			t.Errorf("%s accepted", name)
		}
	}

	c := withAuthSecrets(Load())
	c.Flags.VendorServiceEnabled = true
	c.Flags.CommerceFoodEnabled, c.Flags.CustomerFoodEnabled = true, true
	c.Flags.CommerceShopEnabled, c.Flags.CustomerShopEnabled = true, true
	if err := c.Validate(); err != nil {
		t.Fatalf("both domains enabled: %v", err)
	}

	t.Setenv("CUSTOMER_FOOD_ENABLED", "yes-please")
	if err := withAuthSecrets(Load()).Validate(); err == nil {
		t.Fatal("a malformed release gate must fail startup")
	}
}
