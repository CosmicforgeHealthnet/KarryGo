package bridge

import (
	"cosmicforge/logistics/services/admin-backoffice-service/internal/features/customerreviewops"
	"cosmicforge/logistics/shared/go/customerreview"
	"errors"
)

// ConfigureCustomerReview is startup-only. Compiled code and transport settings
// cannot supply missing approved policy or source-owner authorization.
func (b *Bridge) ConfigureCustomerReview(service *customerreviewops.Service) error {
	if service == nil || service.DB == nil || service.Owner == nil || b.customerReview != nil {
		return errors.New("invalid Customer review configuration")
	}
	b.customerReview = service
	return nil
}
func (b *Bridge) customerReviewAvailability(values map[string]bool, roles, zones, tenants []string) {
	for _, capability := range []string{customerreview.RequestCapability, customerreview.ReadCapability} {
		values[capability] = false
		if b.customerReview == nil || !b.customerReview.ReadEnabled || len(zones) == 0 || len(tenants) == 0 {
			continue
		}
		if capability == customerreview.RequestCapability && !b.customerReview.WriteEnabled {
			continue
		}
		rule, err := b.customerReview.Policy.Rule(capability, b.now())
		expected := customerreview.Contract
		if capability == customerreview.ReadCapability {
			expected = customerreview.StateContract
		}
		values[capability] = err == nil && rule.Owner == customerreview.Owner && rule.SourceContract == expected && hasRole(roles, rule.Roles)
	}
}
