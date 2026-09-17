package internal_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Section 7 must not introduce pricing: fares, per-km/metre rates, surge,
// fuel, commission, provider earnings or discounts belong to the separate
// pricing boundary. These packages were added or extended by Section 7 and
// must stay free of monetary computation.
var section7Files = []string{
	"capability/capability.go",
	"features/capabilities/repository.go",
	"features/capabilities/service.go",
	"features/capabilities/http.go",
	"features/goods/multistop.go",
	"features/goods/multistop_repository.go",
	"features/goods/recovery.go",
	"features/booking/usecases/scheduled_risk.go",
	"features/booking/repositories/scheduled_risk.go",
	"movement/movement.go",
}

func TestSection7IntroducesNoPricingLogic(t *testing.T) {
	forbidden := regexp.MustCompile(`(?i)(fare_?estimate|fare_?final|per_?km|per_?met(re|er)|surge|fuel|commission|provider_?earning|discount|subsid|\.Price\(|pricing\.|PricingPolicy|FeeMinor\s*[+*]|fee_minor\s*[+*])`)
	for _, file := range section7Files {
		raw, err := os.ReadFile(filepath.FromSlash(file))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for i, line := range strings.Split(string(raw), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if match := forbidden.FindString(line); match != "" {
				t.Errorf("%s:%d introduces pricing-related logic %q", file, i+1, match)
			}
		}
	}
}

// Vendor commerce contracts must never let a caller name the provider or
// vehicle that executes a Taxi mission.
func TestCommerceContractsCarryNoProviderIdentity(t *testing.T) {
	identity := regexp.MustCompile(`json:"(provider_id|driver_id|rider_id|vehicle_id|fleet_member_id|assignee_id)"`)
	for _, file := range []string{
		filepath.Join("..", "..", "..", "shared", "go", "commercemission", "contract.go"),
		filepath.Join("..", "..", "..", "shared", "go", "commercequote", "contract.go"),
	} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Skipf("NOT EXECUTED: shared contracts unavailable beside the service (%v)", err)
		}
		if match := identity.FindString(string(raw)); match != "" {
			t.Errorf("%s exposes execution identity %s", file, match)
		}
	}
	raw, err := os.ReadFile(filepath.FromSlash("features/goods/multistop.go"))
	if err != nil {
		t.Fatal(err)
	}
	section := string(raw)
	start := strings.Index(section, "type ServiceabilityRequest struct")
	end := strings.Index(section[start:], "}")
	if identity.MatchString(section[start : start+end]) {
		t.Error("ServiceabilityRequest must not accept provider or vehicle identity")
	}
}
