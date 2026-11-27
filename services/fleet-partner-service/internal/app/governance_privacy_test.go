package app

import (
	"sort"
	"testing"

	governancemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/governance/models"
	retentionmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/retentionpolicy/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/retentionexec"
)

// TestPrivacySettingsOfferOnlyEnforcedRetentionClasses pins the classes an
// organization may tune through /privacy-settings to exactly the classes a
// retention executor implements, so no setting is offered without
// enforcement behind it, and keeps protected classes valid and disjoint.
func TestPrivacySettingsOfferOnlyEnforcedRetentionClasses(t *testing.T) {
	executors := []string{
		retentionexec.NewOrgContactExecutor().Class(),
		retentionexec.NewExpiredExportExecutor(nil).Class(),
		retentionexec.NewSupportMetadataExecutor(nil).Class(),
		retentionexec.NewSupersededProjectionExecutor(nil).Class(),
	}
	offered := append([]string{}, governancemodels.ConfigurableRetentionClasses...)
	sort.Strings(executors)
	sort.Strings(offered)
	if len(executors) != len(offered) {
		t.Fatalf("configurable classes %v, executor classes %v", offered, executors)
	}
	for i := range executors {
		if executors[i] != offered[i] {
			t.Fatalf("configurable classes %v, executor classes %v", offered, executors)
		}
	}
	seen := map[string]bool{}
	for _, c := range offered {
		seen[c] = true
	}
	for _, c := range governancemodels.ProtectedRetentionClasses {
		if !retentionmodels.ValidClass(c) {
			t.Fatalf("protected class %q is not a retention class", c)
		}
		if seen[c] {
			t.Fatalf("class %q is both configurable and protected", c)
		}
	}
}
