// Package contract contains the pinned, source-controlled HQ v1 contract.
package contract

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"strings"
)

//go:embed KARRYGO_ADMIN_CAPABILITY_MATRIX.json
var matrix []byte

//go:embed openapi.json
var specification []byte

type Capability struct {
	ID                    string   `json:"id"`
	Version               string   `json:"version"`
	Owner                 string   `json:"owner"`
	SourceEndpoint        string   `json:"source_endpoint"`
	BridgeEndpoint        string   `json:"bridge_endpoint"`
	Method                string   `json:"method"`
	State                 string   `json:"state"`
	Permission            string   `json:"permission"`
	Scope                 string   `json:"scope"`
	Risk                  string   `json:"risk"`
	Privacy               string   `json:"privacy"`
	SourceVersionRequired bool     `json:"source_version_required"`
	IdempotencyRequired   bool     `json:"idempotency_required"`
	Approval              string   `json:"approval"`
	Evidence              string   `json:"evidence"`
	FeatureFlag           string   `json:"feature_flag"`
	Tests                 []string `json:"tests"`
	Blocker               string   `json:"blocker"`
	Fallback              string   `json:"fallback"`
}

func Matrix() []Capability {
	var values []Capability
	if err := json.Unmarshal(matrix, &values); err != nil {
		panic("invalid embedded capability matrix")
	}
	return values
}
func Match(method, path string) (Capability, bool) {
	for _, c := range Matrix() {
		if c.Method != method {
			continue
		}
		pattern := strings.Split(c.BridgeEndpoint, "/")
		parts := strings.Split(path, "/")
		if len(pattern) != len(parts) {
			continue
		}
		ok := true
		for i, p := range pattern {
			if strings.HasPrefix(p, "{") && strings.HasSuffix(p, "}") {
				if parts[i] == "" {
					ok = false
				}
				continue
			}
			if p != parts[i] {
				ok = false
				break
			}
		}
		if ok {
			return c, true
		}
	}
	return Capability{}, false
}
func Discover(permissions []string) any {
	return DiscoverWithAvailability(permissions, nil)
}

// Availability is evaluated at startup/runtime in addition to the pinned
// implementation matrix. A compiled adapter is not an enabled deployment.
func DiscoverWithAvailability(permissions []string, enabled map[string]bool) any {
	values := []Capability{}
	for _, c := range Matrix() {
		if available, exists := enabled[c.Permission]; exists && !available {
			c.State = "DISABLED"
			c.Blocker = "Owner adapter, approved role policy, geographic delegation or feature flag unavailable."
		}
		for _, permission := range permissions {
			if c.Permission == permission {
				values = append(values, c)
				break
			}
		}
	}
	sum := sha256.Sum256(specification)
	return struct {
		Version      string       `json:"contract_version"`
		SHA          string       `json:"openapi_sha256"`
		Capabilities []Capability `json:"capabilities"`
	}{"1.0", hex.EncodeToString(sum[:]), values}
}
