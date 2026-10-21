// Package analyticsadmin defines scoped, suppressed owner responses. Legacy
// unscoped Analytics aggregate endpoints are never used by this contract.
package analyticsadmin

import (
	"cosmicforge/logistics/shared/go/adminpolicy"
	"time"
)

const MissionSuccessPermission = "analytics.scoped.mission-success.read"
const MissionSuccessPath = "/internal/analytics/v1/admin/scoped-network/mission-success"

func ValidMissionSourceContract(value string) bool {
	for _, field := range []string{"provider_id", "original_provider_id", "replacement_provider_id", "branch_id", "source_entity_id"} {
		if value == "analytics.scoped-mission-success.v1:"+field {
			return true
		}
	}
	return false
}

type Filters struct{ Authority, ProductCode string }
type Cell struct {
	Key         string `json:"key"`
	Value       *int64 `json:"value"`
	Suppression string `json:"suppression"`
}
type Release struct {
	PolicyID       string            `json:"policy_id"`
	PolicyVersion  int64             `json:"policy_version"`
	PolicyChecksum string            `json:"policy_checksum"`
	Scope          adminpolicy.Scope `json:"scope"`
	Metric         string            `json:"metric"`
	SourceVersion  string            `json:"source_version"`
	AsOf           time.Time         `json:"as_of"`
	Freshness      string            `json:"freshness"`
	Coverage       string            `json:"coverage"`
	Cells          []Cell            `json:"cells"`
}
