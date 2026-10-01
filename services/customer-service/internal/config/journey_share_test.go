package config

import (
	"strings"
	"testing"
	"time"
)

func TestJourneySharingStartupFailsClosedWithoutLeakingSecrets(t *testing.T) {
	if err := (Config{}).ValidateJourneySharing(); err != nil {
		t.Fatal("disabled config must start", err)
	}
	c := Config{ShareJourneyEnabled: true, ShareJourneyTTL: time.Hour, ShareCompletedRetention: time.Minute, ShareViewerOrigin: "https://viewer.example.test", ShareTaxiURL: "http://taxi", ShareDispatchURL: "http://dispatch", ShareHaulingURL: "http://hauling", ShareVendorURL: "http://vendor", ShareTaxiSecret: []byte("fixture"), ShareDispatchSecret: []byte("fixture"), ShareHaulingSecret: []byte("fixture"), ShareVendorSecret: []byte("fixture"), ShareTrustedGatewayCIDRs: []string{"10.0.0.0/24"}}
	if err := c.ValidateJourneySharing(); err != nil {
		t.Fatal(err)
	}
	c.ShareTaxiURL = "http://private-user:private-password@taxi"
	c.ShareVendorSecret = nil
	c.ShareTrustedGatewayCIDRs = []string{"untrusted"}
	err := c.ValidateJourneySharing()
	if err == nil {
		t.Fatal("invalid config accepted")
	}
	for _, private := range []string{"private-user", "private-password", "fixture", "untrusted"} {
		if strings.Contains(err.Error(), private) {
			t.Fatal("config leaked", private)
		}
	}
}

// Each required boundary independently fails closed when sharing is enabled.
func TestJourneySharingEachRequiredSettingFailsClosed(t *testing.T) {
	valid := func() Config {
		return Config{ShareJourneyEnabled: true, ShareJourneyTTL: time.Hour, ShareCompletedRetention: time.Minute, ShareViewerOrigin: "https://viewer.example.test", ShareTaxiURL: "http://taxi", ShareDispatchURL: "http://dispatch", ShareHaulingURL: "http://hauling", ShareVendorURL: "http://vendor", ShareTaxiSecret: []byte("s"), ShareDispatchSecret: []byte("s"), ShareHaulingSecret: []byte("s"), ShareVendorSecret: []byte("s"), ShareTrustedGatewayCIDRs: []string{"10.0.0.0/24"}}
	}
	for name, mutate := range map[string]func(*Config){
		"missing viewer origin":     func(c *Config) { c.ShareViewerOrigin = "" },
		"viewer origin with path":   func(c *Config) { c.ShareViewerOrigin = "https://viewer.example.test/share" },
		"viewer origin wildcard":    func(c *Config) { c.ShareViewerOrigin = "*" },
		"missing taxi origin":       func(c *Config) { c.ShareTaxiURL = "" },
		"dispatch origin with path": func(c *Config) { c.ShareDispatchURL = "http://dispatch/api/v1" },
		"hauling non-http scheme":   func(c *Config) { c.ShareHaulingURL = "ftp://hauling" },
		"vendor origin with query":  func(c *Config) { c.ShareVendorURL = "http://vendor?x=1" },
		"missing taxi secret":       func(c *Config) { c.ShareTaxiSecret = nil },
		"missing dispatch secret":   func(c *Config) { c.ShareDispatchSecret = nil },
		"missing hauling secret":    func(c *Config) { c.ShareHaulingSecret = nil },
		"missing vendor secret":     func(c *Config) { c.ShareVendorSecret = nil },
		"zero ttl":                  func(c *Config) { c.ShareJourneyTTL = 0 },
		"ttl over 30 days":          func(c *Config) { c.ShareJourneyTTL = 31 * 24 * time.Hour },
		"zero retention":            func(c *Config) { c.ShareCompletedRetention = 0 },
		"retention beyond ttl":      func(c *Config) { c.ShareCompletedRetention = 2 * time.Hour },
		"invalid gateway cidr":      func(c *Config) { c.ShareTrustedGatewayCIDRs = []string{"10.0.0.0/33"} },
	} {
		c := valid()
		mutate(&c)
		if err := c.ValidateJourneySharing(); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
