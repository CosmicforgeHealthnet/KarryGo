package searchprojection

import (
	"context"
	"testing"
)

type section13FInventoryAuthority struct {
	quantity int64
	revision int64
}

func (
	authority *section13FInventoryAuthority,
) RevalidateAndReserve(
	ctx context.Context,
	request ShopReservationRequest,
) (
	ShopReservationDecision,
	error,
) {
	if request.Quantity <= 0 {
		return ShopReservationDecision{},
			ErrInvalidArgument
	}

	if authority.quantity <
		request.Quantity {
		return ShopReservationDecision{
			Authorized: false,

			Reason: "OUT_OF_STOCK",

			AuthoritativeInventoryRevision: authority.revision,
		}, nil
	}

	authority.quantity -=
		request.Quantity

	return ShopReservationDecision{
		Authorized: true,

		Reason: "RESERVED",

		AuthoritativeInventoryRevision: authority.revision,
	}, nil
}

func TestSection13FStaleSearchCannotAuthorizeInventory(
	t *testing.T,
) {
	// Search can legitimately still contain stale IN_STOCK data.
	// It is discovery evidence, not reservation authority.
	searchResult :=
		ShopEngineHit{
			ProductRef: "shop-rice",

			VariantRef: "rice-50kg",

			AvailabilityStatus: string(
				ShopInventoryInStock,
			),

			SourceEvidence: SearchSourceEvidence{
				InventoryRevision: 10,

				ProjectionRevision: 5,
			},
		}

	if searchResult.AvailabilityStatus !=
		"IN_STOCK" {
		t.Fatal(
			"fixture must represent stale IN_STOCK result",
		)
	}

	// Authoritative Section 12 inventory has already moved
	// to revision 11 with quantity zero.
	authority :=
		&section13FInventoryAuthority{
			quantity: 0,

			revision: 11,
		}

	guard :=
		NewShopCheckoutGuard(
			authority,
		)

	decision, err :=
		guard.RevalidateAndReserve(
			context.Background(),
			ShopReservationRequest{
				ProductRef: searchResult.ProductRef,

				VariantRef: searchResult.VariantRef,

				Quantity: 1,

				SearchInventoryRevision: searchResult.
					SourceEvidence.
					InventoryRevision,

				SearchProjectionRevision: searchResult.
					SourceEvidence.
					ProjectionRevision,
			},
		)
	if err != nil {
		t.Fatal(err)
	}

	if decision.Authorized {
		t.Fatal(
			"stale search result authorized stock reservation",
		)
	}

	if decision.Reason !=
		"OUT_OF_STOCK" {
		t.Fatalf(
			"reason=%q",
			decision.Reason,
		)
	}

	if decision.AuthoritativeInventoryRevision !=
		11 {
		t.Fatalf(
			"authority revision=%d want=11",
			decision.AuthoritativeInventoryRevision,
		)
	}

	if authority.quantity != 0 {
		t.Fatalf(
			"inventory changed or became negative: %d",
			authority.quantity,
		)
	}
}

func TestSection13FUnknownSignalsRemainUnknown(
	t *testing.T,
) {
	shop :=
		ShopEngineDocument{}

	if status :=
		shopServiceabilityStatusFromDocument(
			shop,
		); status !=
		ServiceabilityUnknown {
		t.Fatalf(
			"missing Shop serviceability became %q",
			status,
		)
	}

	if status :=
		shopInventoryStatusFromDocument(
			shop,
		); status !=
		ShopInventoryUnknown {
		t.Fatalf(
			"missing Shop inventory state became %q",
			status,
		)
	}

	food :=
		FoodEngineDocument{}

	if status :=
		foodAvailabilityStatusFromDocument(
			food,
		); status !=
		FoodAvailabilityUnknown {
		t.Fatalf(
			"missing Food availability became %q",
			status,
		)
	}

	if status :=
		foodServiceabilityStatusFromDocument(
			food,
		); status !=
		ServiceabilityUnknown {
		t.Fatalf(
			"missing Food serviceability became %q",
			status,
		)
	}
}

func TestSection13FUnknownReliabilityHasNoPerfectScore(
	t *testing.T,
) {
	signal :=
		ReliabilitySignal{
			Status: SearchReliabilityUnknown,

			Revision: 1,
		}

	if signal.CompletionBasisPoints != nil ||
		signal.CancellationBasisPoints != nil ||
		signal.SLABasisPoints != nil {
		t.Fatal(
			"UNKNOWN reliability contains synthetic score",
		)
	}
}
