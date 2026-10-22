package bridge

import (
	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/customerworkitemops"
	"cosmicforge/logistics/shared/go/customerreview"
	"errors"
)

func (b *Bridge) ConfigureCustomerWorkItems(s *customerworkitemops.Service) error {
	if s == nil || s.DB == nil || s.Owner == nil || b.customerWorkItems != nil {
		return errors.New("invalid Customer work-item configuration")
	}
	b.customerWorkItems = s
	return nil
}
func (b *Bridge) customerWorkItemAvailability(values map[string]bool, roles, zones, tenants []string) {
	for _, cap := range []string{customerreview.WorkItemCapability, customerreview.WorkItemReadCapability} {
		values[cap] = false
		s := b.customerWorkItems
		if s == nil || !s.ReadEnabled || len(zones) == 0 || len(tenants) == 0 {
			continue
		}
		if cap == customerreview.WorkItemCapability && (!s.WriteEnabled || !b.features.WriteFamilyRequested(cap)) {
			continue
		}
		rule, err := s.Policy.Rule(cap, b.now())
		values[cap] = err == nil && rule.Owner == customerreview.Owner && rule.SourceContract == customerreview.WorkItemContract && hasRole(roles, rule.Roles)
	}
}
