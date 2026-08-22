package inventory

import (
	"context"
	"strings"
)

const (
	FoodAvailabilityManualBypass = "MANUAL_BYPASS"

	FoodAvailabilityAvailable     = "AVAILABLE"
	FoodAvailabilityUnavailable   = "UNAVAILABLE"
	FoodAvailabilityNotConfigured = "NOT_CONFIGURED"
	FoodAvailabilityNotSupported  = "NOT_SUPPORTED"
	FoodAvailabilityUnknown       = "UNKNOWN"
)

const (
	FoodAvailabilityModeManual          = "MANUAL"
	FoodAvailabilityModeInventoryLinked = "INVENTORY_LINKED"
)

// RecipeAvailabilityRequest is deliberately small.
//
// Inventory knows only that some future recipe authority may resolve an opaque
// recipe reference into stock requirements.
//
// Inventory does NOT own recipe composition.
type RecipeAvailabilityRequest struct {
	BusinessID string
	BranchID   string

	FoodItemRef string
	RecipeRef   string
}

// RecipeAvailabilityResult is the future Section 16 answer.
//
// Section 14G consumes this contract but does not implement the authority that
// produces it.
type RecipeAvailabilityResult struct {
	Support string

	Sellable bool
	Reason   string

	InventoryRevision int64
}

// RecipeAvailabilityResolver is the Section 16 extension point.
//
// Implementations may later resolve:
//
//	recipe -> ingredient requirements -> generic Inventory availability.
//
// Section 14G intentionally provides no production implementation.
type RecipeAvailabilityResolver interface {
	ResolveRecipeAvailability(
		ctx context.Context,
		request RecipeAvailabilityRequest,
	) (RecipeAvailabilityResult, error)
}

type FoodAvailabilityRequest struct {
	BusinessID string
	BranchID   string

	FoodItemRef string

	Mode string

	InventoryRecipeRef string
}

type FoodAvailabilityResult struct {
	InventoryControlled bool

	Support string

	Sellable bool
	Reason   string

	InventoryRevision int64
}

// ResolveFoodAvailability is the Inventory-side Food availability seam.
//
// MANUAL never delegates to Inventory.
//
// INVENTORY_LINKED always fails closed unless an authoritative recipe resolver
// can answer positively.
func (s *StockService) ResolveFoodAvailability(
	ctx context.Context,
	request FoodAvailabilityRequest,
) (
	FoodAvailabilityResult,
	error,
) {
	request.BusinessID =
		strings.TrimSpace(
			request.BusinessID,
		)

	request.BranchID =
		strings.TrimSpace(
			request.BranchID,
		)

	request.FoodItemRef =
		strings.TrimSpace(
			request.FoodItemRef,
		)

	request.Mode =
		strings.ToUpper(
			strings.TrimSpace(
				request.Mode,
			),
		)

	request.InventoryRecipeRef =
		strings.TrimSpace(
			request.InventoryRecipeRef,
		)

	switch request.Mode {
	case FoodAvailabilityModeManual:
		return FoodAvailabilityResult{
			InventoryControlled: false,

			Support: FoodAvailabilityManualBypass,

			Sellable: true,

			Reason: "MANUAL_AVAILABILITY",
		}, nil

	case FoodAvailabilityModeInventoryLinked:
		// Continue below.

	default:
		return FoodAvailabilityResult{
			InventoryControlled: true,

			Support: FoodAvailabilityUnknown,

			Sellable: false,

			Reason: "UNKNOWN_AVAILABILITY_MODE",
		}, nil
	}

	if request.BusinessID == "" ||
		request.BranchID == "" ||
		request.FoodItemRef == "" {

		return FoodAvailabilityResult{
			InventoryControlled: true,

			Support: FoodAvailabilityUnknown,

			Sellable: false,

			Reason: "INVALID_INVENTORY_CONTEXT",
		}, nil
	}

	if request.InventoryRecipeRef == "" {
		return FoodAvailabilityResult{
			InventoryControlled: true,

			Support: FoodAvailabilityNotConfigured,

			Sellable: false,

			Reason: "RECIPE_REFERENCE_NOT_CONFIGURED",
		}, nil
	}

	// Section 16 has not supplied recipe authority yet.
	if s == nil ||
		s.recipeAvailability == nil {

		return FoodAvailabilityResult{
			InventoryControlled: true,

			Support: FoodAvailabilityNotSupported,

			Sellable: false,

			Reason: "RECIPE_AUTHORITY_NOT_CONNECTED",
		}, nil
	}

	resolved, err :=
		s.recipeAvailability.
			ResolveRecipeAvailability(
				ctx,
				RecipeAvailabilityRequest{
					BusinessID: request.BusinessID,

					BranchID: request.BranchID,

					FoodItemRef: request.FoodItemRef,

					RecipeRef: request.InventoryRecipeRef,
				},
			)

	if err != nil {
		// Availability uncertainty must never become optimistic sellability.
		return FoodAvailabilityResult{
			InventoryControlled: true,

			Support: FoodAvailabilityUnknown,

			Sellable: false,

			Reason: "RECIPE_AVAILABILITY_UNKNOWN",
		}, nil
	}

	switch resolved.Support {
	case FoodAvailabilityAvailable:
		if !resolved.Sellable {
			return FoodAvailabilityResult{
				InventoryControlled: true,

				Support: FoodAvailabilityUnavailable,

				Sellable: false,

				Reason: resolved.Reason,

				InventoryRevision: resolved.InventoryRevision,
			}, nil
		}

		return FoodAvailabilityResult{
			InventoryControlled: true,

			Support: FoodAvailabilityAvailable,

			Sellable: true,

			Reason: resolved.Reason,

			InventoryRevision: resolved.InventoryRevision,
		}, nil

	case FoodAvailabilityUnavailable:
		return FoodAvailabilityResult{
			InventoryControlled: true,

			Support: FoodAvailabilityUnavailable,

			Sellable: false,

			Reason: resolved.Reason,

			InventoryRevision: resolved.InventoryRevision,
		}, nil

	case FoodAvailabilityNotConfigured:
		return FoodAvailabilityResult{
			InventoryControlled: true,

			Support: FoodAvailabilityNotConfigured,

			Sellable: false,

			Reason: resolved.Reason,

			InventoryRevision: resolved.InventoryRevision,
		}, nil

	case FoodAvailabilityNotSupported:
		return FoodAvailabilityResult{
			InventoryControlled: true,

			Support: FoodAvailabilityNotSupported,

			Sellable: false,

			Reason: resolved.Reason,

			InventoryRevision: resolved.InventoryRevision,
		}, nil

	default:
		return FoodAvailabilityResult{
			InventoryControlled: true,

			Support: FoodAvailabilityUnknown,

			Sellable: false,

			Reason: "RECIPE_AVAILABILITY_UNKNOWN",

			InventoryRevision: resolved.InventoryRevision,
		}, nil
	}
}
