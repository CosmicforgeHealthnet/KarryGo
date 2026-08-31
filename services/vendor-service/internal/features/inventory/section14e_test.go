package inventory

import (
	"errors"
	"testing"
	"time"
)

func TestSection14EFingerprintIsStableAcrossLineOrdering(
	t *testing.T,
) {
	expiry :=
		time.Date(
			2027,
			1,
			2,
			3,
			4,
			5,
			0,
			time.UTC,
		)

	first,
		firstFingerprint,
		err :=
		normalizeReservationInput(
			CreateReservationInput{
				BranchID: "branch-1",

				SourceType: "checkout",

				SourceRef: "checkout-1",

				ExpiresAt: &expiry,

				Lines: []ReservationLineInput{
					{
						InventoryItemID: "item-b",

						Quantity: 2,
					},
					{
						InventoryItemID: "item-a",

						Quantity: 3,
					},
				},
			},
		)

	if err != nil {
		t.Fatal(err)
	}

	second,
		secondFingerprint,
		err :=
		normalizeReservationInput(
			CreateReservationInput{
				BranchID: "branch-1",

				SourceType: "CHECKOUT",

				SourceRef: "checkout-1",

				ExpiresAt: &expiry,

				Lines: []ReservationLineInput{
					{
						InventoryItemID: "item-a",

						Quantity: 3,
					},
					{
						InventoryItemID: "item-b",

						Quantity: 2,
					},
				},
			},
		)

	if err != nil {
		t.Fatal(err)
	}

	if firstFingerprint !=
		secondFingerprint {

		t.Fatal(
			"semantically identical reservation requests produced different fingerprints",
		)
	}

	if first.Lines[0].InventoryItemID !=
		"item-a" ||
		second.Lines[0].InventoryItemID !=
			"item-a" {

		t.Fatal(
			"reservation lines were not sorted by stable inventory-item ID",
		)
	}
}

func TestSection14EAggregatesDuplicateItemLines(
	t *testing.T,
) {
	input,
		_,
		err :=
		normalizeReservationInput(
			CreateReservationInput{
				BranchID: "branch",

				SourceType: ReservationSourceInternal,

				SourceRef: "source",

				Lines: []ReservationLineInput{
					{
						InventoryItemID: "item-a",

						Quantity: 2,
					},
					{
						InventoryItemID: "item-a",

						Quantity: 3,
					},
				},
			},
		)

	if err != nil {
		t.Fatal(err)
	}

	if len(input.Lines) != 1 ||
		input.Lines[0].Quantity != 5 {

		t.Fatalf(
			"duplicate item lines were not normalized: %+v",
			input.Lines,
		)
	}
}

func TestSection14ERejectsInvalidReservationSource(
	t *testing.T,
) {
	_,
		_,
		err :=
		normalizeReservationInput(
			CreateReservationInput{
				BranchID: "branch",

				SourceType: "ORDER_DATABASE",

				SourceRef: "source",

				Lines: []ReservationLineInput{
					{
						InventoryItemID: "item",

						Quantity: 1,
					},
				},
			},
		)

	if !errors.Is(
		err,
		ErrReservationSourceInvalid,
	) {
		t.Fatalf(
			"invalid source error=%v",
			err,
		)
	}
}

func TestSection14EReservationTerminalStates(
	t *testing.T,
) {
	for _, status := range []string{
		ReservationCommitted,
		ReservationReleased,
		ReservationExpired,
		ReservationCancelled,
	} {

		if !reservationTerminal(
			status,
		) {
			t.Fatalf(
				"%s should be terminal",
				status,
			)
		}
	}

	if reservationTerminal(
		ReservationHeld,
	) ||
		reservationTerminal(
			ReservationConfirmed,
		) {

		t.Fatal(
			"active reservation state incorrectly treated as terminal",
		)
	}
}
