package food

import (
	"context"
	"strings"
)

const (
	InventorySupportManualBypass = "MANUAL_BYPASS"

	InventorySupportAvailable     = "AVAILABLE"
	InventorySupportUnavailable   = "UNAVAILABLE"
	InventorySupportNotConfigured = "NOT_CONFIGURED"
	InventorySupportNotSupported  = "NOT_SUPPORTED"
	InventorySupportUnknown       = "UNKNOWN"
)

type InventoryAvailabilityRequest struct {
	BusinessID string
	BranchID   string

	FoodItemRef string

	Mode string

	InventoryRecipeRef string
}

type InventoryAvailabilityDecision struct {
	InventoryControlled bool

	Support string

	Sellable bool
	Reason   string

	InventoryRevision int64
}

// InventoryAvailabilityAuthority is Food's only dependency on inventory
// availability.
//
// Food does not import the Inventory implementation package.
type InventoryAvailabilityAuthority interface {
	ResolveFoodAvailability(
		ctx context.Context,
		request InventoryAvailabilityRequest,
	) (InventoryAvailabilityDecision, error)
}

func (s *Service) SetInventoryAvailabilityAuthority(
	authority InventoryAvailabilityAuthority,
) {
	s.inventory = authority
}

func (s *Service) resolveInventoryAvailability(
	ctx context.Context,
	item Item,
	source AvailabilitySource,
) InventoryAvailabilityDecision {
	if source.Mode !=
		SourceInventoryLinked {

		return InventoryAvailabilityDecision{
			InventoryControlled: false,

			Support: InventorySupportManualBypass,

			Sellable: true,

			Reason: "MANUAL_AVAILABILITY",
		}
	}

	if source.InventoryRecipeRef == nil ||
		strings.TrimSpace(
			*source.InventoryRecipeRef,
		) == "" {

		return InventoryAvailabilityDecision{
			InventoryControlled: true,

			Support: InventorySupportNotConfigured,

			Sellable: false,

			Reason: "RECIPE_REFERENCE_NOT_CONFIGURED",
		}
	}

	if s == nil ||
		s.inventory == nil {

		return InventoryAvailabilityDecision{
			InventoryControlled: true,

			Support: InventorySupportNotSupported,

			Sellable: false,

			Reason: "INVENTORY_AUTHORITY_NOT_CONNECTED",
		}
	}

	decision, err :=
		s.inventory.
			ResolveFoodAvailability(
				ctx,
				InventoryAvailabilityRequest{
					BusinessID: item.BusinessID,

					BranchID: item.BranchID,

					FoodItemRef: item.ID,

					Mode: source.Mode,

					InventoryRecipeRef: strings.TrimSpace(
						*source.InventoryRecipeRef,
					),
				},
			)

	if err != nil {
		return InventoryAvailabilityDecision{
			InventoryControlled: true,

			Support: InventorySupportUnknown,

			Sellable: false,

			Reason: "INVENTORY_AVAILABILITY_UNKNOWN",
		}
	}

	// The adapter can only positively permit Food when it explicitly says
	// AVAILABLE and Sellable=true.
	if decision.Support ==
		InventorySupportAvailable &&
		decision.Sellable {

		return decision
	}

	decision.Sellable = false

	switch decision.Support {
	case InventorySupportUnavailable,
		InventorySupportNotConfigured,
		InventorySupportNotSupported,
		InventorySupportUnknown:
		return decision

	default:
		decision.Support =
			InventorySupportUnknown

		if decision.Reason == "" {
			decision.Reason =
				"INVENTORY_AVAILABILITY_UNKNOWN"
		}

		return decision
	}
}
