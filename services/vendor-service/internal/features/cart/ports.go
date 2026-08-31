package cart

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// FoodAuthority resolves Food cart inputs against authoritative Food state.
//
// The implementation belongs in composition/adapters, not in cart.
// Customer-provided business/branch/price values are never accepted here.
type FoodAuthority interface {
	ValidateFoodCartItem(
		ctx context.Context,
		input AddItemInput,
	) (ValidatedItem, error)
}

// ShopAuthority resolves Shop cart inputs against authoritative Shop state.
type ShopAuthority interface {
	ValidateShopCartItem(
		ctx context.Context,
		input AddItemInput,
	) (ValidatedItem, error)
}

// CustomerAddressAuthority resolves one saved-location reference for the
// authenticated customer and returns the delivery snapshot.
//
// The customerID argument MUST come from verified Customer Service bearer
// claims. It is deliberately separate from any HTTP request body.
type CustomerAddressAuthority interface {
	ResolveSavedAddress(
		ctx context.Context,
		customerID string,
		addressRef string,
	) (AddressSnapshot, error)
}

// EventPublisher appends cart domain events to the existing Vendor Service
// transactional outbox. Implementations receive the same pgx transaction as
// the cart mutation, so state, history and event evidence commit or roll back
// together.
type EventPublisher interface {
	Publish(
		ctx context.Context,
		tx pgx.Tx,
		eventType string,
		aggregateType string,
		aggregateID string,
		aggregateVersion int64,
		vendorID string,
		correlationID string,
		causationID string,
		payload any,
	) error
}
