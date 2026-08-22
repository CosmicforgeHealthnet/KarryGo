package savedlocationusecases

import (
	"context"
	"strings"

	savedlocationmodels "cosmicforge/logistics/services/customer-service/internal/features/saved_locations/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

// GetLocation returns one saved location owned by customerID.
//
// This method is intentionally ownership-scoped. A location identifier by
// itself is never sufficient authority: customerID must be supplied by an
// already-authenticated caller or a trusted service-to-service contract.
//
// The repository currently exposes an ownership-scoped List operation rather
// than a point Get. Saved-address collections are deliberately small, so this
// bounded ownership-first lookup reuses the current repository contract rather
// than expanding repository interfaces and breaking unrelated test doubles.
//
// A future repository optimization may add a direct
// WHERE id = ? AND customer_id = ? read without changing this use-case contract.
func (s *SavedLocationService) GetLocation(
	ctx context.Context,
	id string,
	customerID string,
) (savedlocationmodels.SavedLocation, error) {
	id = strings.TrimSpace(id)
	customerID = strings.TrimSpace(customerID)

	if id == "" || customerID == "" {
		return savedlocationmodels.SavedLocation{},
			apperrors.NotFound("Saved location not found.", nil)
	}

	locations, err := s.locations.List(ctx, customerID)
	if err != nil {
		return savedlocationmodels.SavedLocation{}, err
	}

	for _, location := range locations {
		if location.ID == id && location.CustomerID == customerID {
			return location, nil
		}
	}

	return savedlocationmodels.SavedLocation{},
		apperrors.NotFound("Saved location not found.", nil)
}
