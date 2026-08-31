package searchprojection

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

var ErrProjectionEventCollision = errors.New(
	"search projection event identity collision",
)

type SearchProjectionEventType string

const (
	ProjectionEventFoodItemChanged SearchProjectionEventType = "FOOD_ITEM_CHANGED"

	ProjectionEventFoodAvailabilityChanged SearchProjectionEventType = "FOOD_AVAILABILITY_CHANGED"

	ProjectionEventCloudKitchenProductionChanged SearchProjectionEventType = "CLOUD_KITCHEN_PRODUCTION_CHANGED"

	ProjectionEventPastryAvailabilityChanged SearchProjectionEventType = "PASTRY_AVAILABILITY_CHANGED"

	ProjectionEventShopProductChanged SearchProjectionEventType = "SHOP_PRODUCT_CHANGED"

	ProjectionEventShopListingChanged SearchProjectionEventType = "SHOP_LISTING_CHANGED"

	ProjectionEventShopInventoryChanged SearchProjectionEventType = "SHOP_INVENTORY_CHANGED"

	ProjectionEventBranchStateChanged SearchProjectionEventType = "BRANCH_STATE_CHANGED"

	ProjectionEventTaxonomyChanged SearchProjectionEventType = "TAXONOMY_CHANGED"

	ProjectionEventVerificationChanged SearchProjectionEventType = "VERIFICATION_CHANGED"

	ProjectionEventReliabilityChanged SearchProjectionEventType = "RELIABILITY_CHANGED"

	ProjectionEventCommercialSignalChanged SearchProjectionEventType = "COMMERCIAL_SIGNAL_CHANGED"
)

const maxIncrementalProjectionUpdates = 500

type SearchProjectionEvent struct {
	EventID string

	Namespace SearchCacheNamespace

	EventType SearchProjectionEventType

	SourceKind string
	SourceRef  string

	SourceRevision int64

	Generation int64

	// Payload is event metadata only.
	// It is not treated as catalogue/inventory authority.
	Payload json.RawMessage
}

type IncrementalProjectionUpdate struct {
	FoodDocument *FoodProjectionInput

	ShopDocument *ShopProjectionInput

	FoodSignals *FoodOperationalSignals

	ShopSignals *ShopOperationalSignals
}

// AuthoritativeIncrementalProjectionSource must fetch the
// CURRENT authoritative Food/Shop state.
//
// Event payloads identify change; they do not become catalogue,
// inventory, verification or reliability authority.
type AuthoritativeIncrementalProjectionSource interface {
	ResolveIncrementalProjection(
		ctx context.Context,
		event SearchProjectionEvent,
	) (
		[]IncrementalProjectionUpdate,
		error,
	)
}

type IncrementalProjectionApplier interface {
	ApplyIncrementalProjection(
		ctx context.Context,
		event SearchProjectionEvent,
		updates []IncrementalProjectionUpdate,
	) error
}

type DefaultIncrementalProjectionApplier struct {
	foodProjector *FoodProjector

	shopProjector *ShopProjector

	foodBackend *PostgresFoodSearchBackend

	shopBackend *PostgresShopSearchBackend
}

func NewDefaultIncrementalProjectionApplier(
	foodProjector *FoodProjector,
	shopProjector *ShopProjector,
	foodBackend *PostgresFoodSearchBackend,
	shopBackend *PostgresShopSearchBackend,
) *DefaultIncrementalProjectionApplier {
	return &DefaultIncrementalProjectionApplier{
		foodProjector: foodProjector,

		shopProjector: shopProjector,

		foodBackend: foodBackend,

		shopBackend: shopBackend,
	}
}

func validProjectionEventType(
	namespace SearchCacheNamespace,
	eventType SearchProjectionEventType,
) bool {
	switch namespace {
	case SearchCacheNamespaceFood:
		switch eventType {
		case ProjectionEventFoodItemChanged,
			ProjectionEventFoodAvailabilityChanged,
			ProjectionEventCloudKitchenProductionChanged,
			ProjectionEventPastryAvailabilityChanged,
			ProjectionEventBranchStateChanged,
			ProjectionEventTaxonomyChanged,
			ProjectionEventVerificationChanged,
			ProjectionEventReliabilityChanged,
			ProjectionEventCommercialSignalChanged:

			return true
		}

	case SearchCacheNamespaceShop:
		switch eventType {
		case ProjectionEventShopProductChanged,
			ProjectionEventShopListingChanged,
			ProjectionEventShopInventoryChanged,
			ProjectionEventBranchStateChanged,
			ProjectionEventTaxonomyChanged,
			ProjectionEventVerificationChanged,
			ProjectionEventReliabilityChanged,
			ProjectionEventCommercialSignalChanged:

			return true
		}
	}

	return false
}

func validateProjectionEvent(
	event SearchProjectionEvent,
) error {
	event.EventID =
		strings.TrimSpace(
			event.EventID,
		)

	event.SourceKind =
		strings.TrimSpace(
			event.SourceKind,
		)

	event.SourceRef =
		strings.TrimSpace(
			event.SourceRef,
		)

	if event.EventID == "" ||
		event.SourceKind == "" ||
		event.SourceRef == "" ||
		event.SourceRevision <= 0 ||
		event.Generation <= 0 ||
		!validProjectionEventType(
			event.Namespace,
			event.EventType,
		) ||
		len(event.Payload) > 64*1024 {
		return ErrInvalidArgument
	}

	if len(event.Payload) > 0 {
		var value any

		if json.Unmarshal(
			event.Payload,
			&value,
		) != nil {
			return ErrInvalidArgument
		}
	}

	return nil
}

func canonicalProjectionEventHash(
	event SearchProjectionEvent,
) (
	string,
	error,
) {
	var payload any

	if len(event.Payload) > 0 {
		if err :=
			json.Unmarshal(
				event.Payload,
				&payload,
			); err != nil {
			return "",
				ErrInvalidArgument
		}
	}

	raw, err :=
		json.Marshal(
			struct {
				EventID string `json:"event_id"`

				Namespace SearchCacheNamespace `json:"namespace"`

				EventType SearchProjectionEventType `json:"event_type"`

				SourceKind string `json:"source_kind"`
				SourceRef  string `json:"source_ref"`

				SourceRevision int64 `json:"source_revision"`

				Generation int64 `json:"generation"`

				Payload any `json:"payload,omitempty"`
			}{
				EventID: strings.TrimSpace(
					event.EventID,
				),

				Namespace: event.Namespace,

				EventType: event.EventType,

				SourceKind: strings.TrimSpace(
					event.SourceKind,
				),

				SourceRef: strings.TrimSpace(
					event.SourceRef,
				),

				SourceRevision: event.SourceRevision,

				Generation: event.Generation,

				Payload: payload,
			},
		)
	if err != nil {
		return "", err
	}

	sum :=
		sha256.Sum256(raw)

	return hex.EncodeToString(
		sum[:],
	), nil
}

func validateIncrementalProjectionUpdates(
	event SearchProjectionEvent,
	updates []IncrementalProjectionUpdate,
) error {
	if len(updates) >
		maxIncrementalProjectionUpdates {
		return ErrInvalidArgument
	}

	for _, update := range updates {
		count := 0

		if update.FoodDocument != nil {
			count++
		}

		if update.ShopDocument != nil {
			count++
		}

		if update.FoodSignals != nil {
			count++
		}

		if update.ShopSignals != nil {
			count++
		}

		if count != 1 {
			return ErrInvalidArgument
		}

		switch event.Namespace {
		case SearchCacheNamespaceFood:
			if update.ShopDocument != nil ||
				update.ShopSignals != nil {
				return ErrInvalidArgument
			}

			if update.FoodDocument != nil &&
				update.FoodDocument.Generation !=
					event.Generation {
				return ErrInvalidArgument
			}

			if update.FoodSignals != nil &&
				update.FoodSignals.Generation !=
					event.Generation {
				return ErrInvalidArgument
			}

		case SearchCacheNamespaceShop:
			if update.FoodDocument != nil ||
				update.FoodSignals != nil {
				return ErrInvalidArgument
			}

			if update.ShopDocument != nil &&
				update.ShopDocument.Generation !=
					event.Generation {
				return ErrInvalidArgument
			}

			if update.ShopSignals != nil &&
				update.ShopSignals.Generation !=
					event.Generation {
				return ErrInvalidArgument
			}

		default:
			return ErrInvalidArgument
		}
	}

	return nil
}

func (
	applier *DefaultIncrementalProjectionApplier,
) ApplyIncrementalProjection(
	ctx context.Context,
	event SearchProjectionEvent,
	updates []IncrementalProjectionUpdate,
) error {
	if applier == nil {
		return ErrInvalidArgument
	}

	if err :=
		validateIncrementalProjectionUpdates(
			event,
			updates,
		); err != nil {
		return err
	}

	for _, update := range updates {
		switch {
		case update.FoodDocument != nil:
			if applier.foodProjector == nil {
				return ErrInvalidArgument
			}

			if _, err :=
				applier.foodProjector.Project(
					ctx,
					*update.FoodDocument,
				); err != nil {
				return err
			}

		case update.ShopDocument != nil:
			if applier.shopProjector == nil {
				return ErrInvalidArgument
			}

			if _, err :=
				applier.shopProjector.Project(
					ctx,
					*update.ShopDocument,
				); err != nil {
				return err
			}

		case update.FoodSignals != nil:
			if applier.foodBackend == nil {
				return ErrInvalidArgument
			}

			if _, err :=
				applier.foodBackend.
					ApplyOperationalSignals(
						ctx,
						*update.FoodSignals,
					); err != nil {
				return err
			}

		case update.ShopSignals != nil:
			if applier.shopBackend == nil {
				return ErrInvalidArgument
			}

			if _, err :=
				applier.shopBackend.
					ApplyOperationalSignals(
						ctx,
						*update.ShopSignals,
					); err != nil {
				return err
			}
		}
	}

	return nil
}

var _ IncrementalProjectionApplier = (*DefaultIncrementalProjectionApplier)(nil)
