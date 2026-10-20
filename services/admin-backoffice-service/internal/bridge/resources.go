package bridge

import (
	"cosmicforge/logistics/services/admin-backoffice-service/internal/clients"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"errors"
)

// ConfigureResources is startup-only. A configured transport never supplies
// missing source facts or an approved governance policy.
func (b *Bridge) ConfigureResources(client *clients.Resources, policy *adminpolicy.Engine) error {
	if client == nil {
		return errors.New("owner resource client required")
	}
	if b.resources == nil {
		b.resources = map[string]*clients.Resources{}
		b.resourcePolicies = map[string]*adminpolicy.Engine{}
	}
	if b.resources[client.Owner()] != nil {
		return errors.New("duplicate owner resource configuration")
	}
	b.resources[client.Owner()] = client
	b.resourcePolicies[client.Owner()] = policy
	return nil
}

func (b *Bridge) resourceAvailability(roles []string, zones, tenants []string) map[string]bool {
	enabled := map[string]bool{}
	for _, profile := range adminowner.Profiles() {
		for _, definition := range profile.Definitions {
			rule, err := b.resourcePolicies[profile.Owner].Rule(definition.Capability, b.now())
			enabled[definition.Capability] = b.resources[profile.Owner] != nil && err == nil && rule.Owner == profile.Owner && rule.SourceContract == definition.SourceContract && hasRole(roles, rule.Roles) && len(zones) > 0 && len(tenants) > 0
		}
	}
	return enabled
}
