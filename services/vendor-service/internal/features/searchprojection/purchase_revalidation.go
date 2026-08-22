package searchprojection

import (
	"context"
	"errors"
	"strings"
)

var (
	ErrAuthoritativeRevalidationRequired = errors.New(
		"authoritative checkout revalidation required",
	)
)

type ShopReservationRequest struct {
	BusinessRef string
	BranchRef   string

	ProductRef string
	VariantRef string
	ListingRef string

	Quantity int64

	SearchInventoryRevision  int64
	SearchProjectionRevision int64
}

type ShopReservationDecision struct {
	Authorized bool

	Reason string

	AuthoritativeInventoryRevision int64
}

type ShopInventoryReservationAuthority interface {
	RevalidateAndReserve(
		ctx context.Context,
		request ShopReservationRequest,
	) (
		ShopReservationDecision,
		error,
	)
}

type ShopCheckoutGuard struct {
	authority ShopInventoryReservationAuthority
}

func NewShopCheckoutGuard(
	authority ShopInventoryReservationAuthority,
) *ShopCheckoutGuard {
	return &ShopCheckoutGuard{
		authority: authority,
	}
}

func (
	guard *ShopCheckoutGuard,
) RevalidateAndReserve(
	ctx context.Context,
	request ShopReservationRequest,
) (
	ShopReservationDecision,
	error,
) {
	if guard == nil ||
		guard.authority == nil ||
		strings.TrimSpace(
			request.ProductRef,
		) == "" ||
		request.Quantity <= 0 {
		return ShopReservationDecision{},
			ErrInvalidArgument
	}

	// Search revisions are evidence only.
	//
	// This function deliberately does not infer authorization
	// from IN_STOCK, price or projection state.
	//
	// The authoritative Section 12 inventory implementation is
	// the only component allowed to perform the reservation.
	return guard.authority.
		RevalidateAndReserve(
			ctx,
			request,
		)
}

type FoodCheckoutRequest struct {
	BusinessRef string
	BranchRef   string
	FoodItemRef string

	Quantity int64

	SearchCatalogueRevision    int64
	SearchAvailabilityRevision int64
	SearchProjectionRevision   int64
}

type FoodCheckoutDecision struct {
	Authorized bool

	Reason string

	AuthoritativeCatalogueRevision    int64
	AuthoritativeAvailabilityRevision int64
}

type FoodSellabilityAuthority interface {
	RevalidateFoodSellability(
		ctx context.Context,
		request FoodCheckoutRequest,
	) (
		FoodCheckoutDecision,
		error,
	)
}

type FoodCheckoutGuard struct {
	authority FoodSellabilityAuthority
}

func NewFoodCheckoutGuard(
	authority FoodSellabilityAuthority,
) *FoodCheckoutGuard {
	return &FoodCheckoutGuard{
		authority: authority,
	}
}

func (
	guard *FoodCheckoutGuard,
) Revalidate(
	ctx context.Context,
	request FoodCheckoutRequest,
) (
	FoodCheckoutDecision,
	error,
) {
	if guard == nil ||
		guard.authority == nil ||
		strings.TrimSpace(
			request.FoodItemRef,
		) == "" ||
		request.Quantity <= 0 {
		return FoodCheckoutDecision{},
			ErrInvalidArgument
	}

	// Food search availability is advisory discovery state.
	// Food domain remains sellability/checkout authority.
	return guard.authority.
		RevalidateFoodSellability(
			ctx,
			request,
		)
}
