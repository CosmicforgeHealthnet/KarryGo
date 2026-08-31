package config

import (
	"strings"
	"testing"
	"time"
)

// Section 17B configuration rules.
//
// The point of every case here is the same: a checkout that starts on
// incoherent configuration would hold a customer's stock for a duration nobody
// chose, or expose a surface that can only ever refuse. Both fail at startup
// instead.

func section17BBaseConfig() Config {
	cfg := withAuthSecrets(Load())

	cfg.Flags.VendorServiceEnabled = true
	cfg.Flags.CommerceShopEnabled = true
	cfg.Flags.VendorInventoryEnabled = true
	cfg.Flags.VendorInventoryWritesEnabled = true
	cfg.Flags.CommerceCheckoutEnabled = true

	cfg.Checkout = CheckoutConfig{
		ReservationTTL:   10 * time.Minute,
		StrandedGrace:    2 * time.Minute,
		RecoveryInterval: 30 * time.Second,
	}

	return cfg
}

func TestLoad_Section17BFlagsDefaultOff(t *testing.T) {
	cfg := Load()

	if cfg.Flags.CommerceCheckoutEnabled {
		t.Fatal("COMMERCE_CHECKOUT_ENABLED must default to false")
	}

	if cfg.Flags.CommerceCheckoutRecoveryEnabled {
		t.Fatal("COMMERCE_CHECKOUT_RECOVERY_ENABLED must default to false")
	}

	// The default TTL exists so a deployment that turns checkout on has a
	// sane starting point, but nothing in the code path assumes it.
	if cfg.Checkout.ReservationTTL <= 0 ||
		cfg.Checkout.StrandedGrace <= 0 ||
		cfg.Checkout.RecoveryInterval <= 0 {
		t.Fatalf("expected positive Section 17B defaults: %+v", cfg.Checkout)
	}

	if cfg.Checkout.StrandedGrace >= cfg.Checkout.ReservationTTL {
		t.Fatal("the default stranded grace must be shorter than the default TTL")
	}
}

func TestValidate_Section17BBaselineIsValid(t *testing.T) {
	if err := section17BBaseConfig().Validate(); err != nil {
		t.Fatalf("a coherent Section 17B configuration must validate: %v", err)
	}
}

// A Shop checkout holds real physical stock through Section 14. Enabling it
// without Inventory writes would produce a surface that can only refuse.
func TestValidate_Section17BShopCheckoutRequiresInventoryWrites(t *testing.T) {
	for name, disable := range map[string]func(*Config){
		"inventory disabled": func(c *Config) {
			c.Flags.VendorInventoryEnabled = false
		},
		"inventory writes disabled": func(c *Config) {
			c.Flags.VendorInventoryWritesEnabled = false
		},
	} {
		cfg := section17BBaseConfig()
		disable(&cfg)

		err := cfg.Validate()

		if err == nil {
			t.Fatalf("%s: checkout was accepted without Section 14 writes", name)
		}

		if !strings.Contains(err.Error(), "VENDOR_INVENTORY_WRITES_ENABLED") {
			t.Fatalf("%s: err=%v", name, err)
		}
	}
}

// Checkout validates against Food and Shop. With neither, there is nothing to
// validate.
func TestValidate_Section17BRequiresACommerceDomain(t *testing.T) {
	cfg := section17BBaseConfig()
	cfg.Flags.CommerceShopEnabled = false
	cfg.Flags.CommerceFoodEnabled = false

	err := cfg.Validate()

	if err == nil {
		t.Fatal("checkout was accepted with no commerce domain to validate")
	}

	if !strings.Contains(err.Error(), "COMMERCE_FOOD_ENABLED") {
		t.Fatalf("err=%v", err)
	}
}

// Food-only checkout is legitimate and reserves nothing, so it must not
// require Inventory writes.
func TestValidate_Section17BFoodOnlyCheckoutNeedsNoInventoryWrites(t *testing.T) {
	cfg := section17BBaseConfig()
	cfg.Flags.CommerceShopEnabled = false
	cfg.Flags.CommerceFoodEnabled = true
	cfg.Flags.VendorInventoryEnabled = false
	cfg.Flags.VendorInventoryWritesEnabled = false

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Food-only checkout must validate without Section 14 writes: %v", err)
	}
}

func TestValidate_Section17BRefusesAnAbsurdReservationTTL(t *testing.T) {
	for name, ttl := range map[string]time.Duration{
		"unset":     0,
		"negative":  -time.Minute,
		"too short": 59 * time.Second,
		"too long":  25 * time.Hour,
	} {
		cfg := section17BBaseConfig()
		cfg.Checkout.ReservationTTL = ttl
		cfg.Checkout.StrandedGrace = time.Second

		err := cfg.Validate()

		if err == nil {
			t.Fatalf("%s TTL was accepted", name)
		}

		if !strings.Contains(err.Error(), "COMMERCE_CHECKOUT_RESERVATION_TTL_SECONDS") {
			t.Fatalf("%s: err=%v", name, err)
		}
	}
}

// A grace period that is not shorter than the TTL would let recovery declare a
// perfectly healthy checkout abandoned, or never reach it at all.
func TestValidate_Section17BRefusesAGraceThatIsNotShorterThanTheTTL(t *testing.T) {
	for name, grace := range map[string]time.Duration{
		"zero":            0,
		"equal to TTL":    10 * time.Minute,
		"longer than TTL": 11 * time.Minute,
	} {
		cfg := section17BBaseConfig()
		cfg.Checkout.StrandedGrace = grace

		err := cfg.Validate()

		if err == nil {
			t.Fatalf("%s grace was accepted", name)
		}

		if !strings.Contains(err.Error(), "COMMERCE_CHECKOUT_STRANDED_GRACE_SECONDS") {
			t.Fatalf("%s: err=%v", name, err)
		}
	}
}

func TestValidate_Section17BRefusesAnAbsurdRecoveryInterval(t *testing.T) {
	for name, interval := range map[string]time.Duration{
		"unset":     0,
		"too short": 4 * time.Second,
		"too long":  2 * time.Hour,
	} {
		cfg := section17BBaseConfig()
		cfg.Checkout.RecoveryInterval = interval

		err := cfg.Validate()

		if err == nil {
			t.Fatalf("%s recovery interval was accepted", name)
		}

		if !strings.Contains(err.Error(), "COMMERCE_CHECKOUT_RECOVERY_INTERVAL_SECONDS") {
			t.Fatalf("%s: err=%v", name, err)
		}
	}
}

// The recovery sweep exists to release checkout holds. Enabling it without
// checkout is a flag with nothing to act on.
func TestValidate_Section17BRecoveryRequiresCheckout(t *testing.T) {
	cfg := section17BBaseConfig()
	cfg.Flags.CommerceCheckoutEnabled = false
	cfg.Flags.CommerceCheckoutRecoveryEnabled = true

	err := cfg.Validate()

	if err == nil {
		t.Fatal("the recovery sweep was accepted without checkout")
	}

	if !strings.Contains(err.Error(), "COMMERCE_CHECKOUT_ENABLED") {
		t.Fatalf("err=%v", err)
	}
}

func TestValidate_Section17BRequiresTheServiceItself(t *testing.T) {
	cfg := section17BBaseConfig()
	cfg.Flags.VendorServiceEnabled = false

	if err := cfg.Validate(); err == nil {
		t.Fatal("checkout was accepted while the vendor service itself is off")
	}
}
