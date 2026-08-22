package inventory

import "testing"

func TestSection14DMovementProjectionSequence(
	t *testing.T,
) {
	balance := Balance{}

	history := []Movement{
		{
			Type: MovementReceive,

			Quantity: 10,

			OnHandDelta: 10,

			BalanceRevision: 1,
		},
		{
			Type: MovementReserve,

			Quantity: 3,

			ReservedDelta: 3,

			BalanceRevision: 2,
		},
		{
			Type: MovementQuarantineIn,

			Quantity: 2,

			QuarantinedDelta: 2,

			BalanceRevision: 3,
		},
		{
			Type: MovementRelease,

			Quantity: 3,

			ReservedDelta: -3,

			BalanceRevision: 4,
		},
		{
			Type: MovementWaste,

			Quantity: 2,

			OnHandDelta: -2,

			BalanceRevision: 5,
		},
	}

	var err error

	balance, err =
		ApplyMovementProjection(
			balance,
			history[0],
		)

	if err != nil {
		t.Fatal(err)
	}

	if balance.OnHandQuantity != 10 {
		t.Fatalf(
			"receive on_hand=%d; want 10",
			balance.OnHandQuantity,
		)
	}

	if balance.AvailableQuantity != 10 {
		t.Fatalf(
			"receive available=%d; want 10",
			balance.AvailableQuantity,
		)
	}

	balance, err =
		ApplyMovementProjection(
			balance,
			history[1],
		)

	if err != nil {
		t.Fatal(err)
	}

	if balance.AvailableQuantity != 7 {
		t.Fatalf(
			"reserve available=%d; want 7",
			balance.AvailableQuantity,
		)
	}

	balance, err =
		ApplyMovementProjection(
			balance,
			history[2],
		)

	if err != nil {
		t.Fatal(err)
	}

	if balance.AvailableQuantity != 5 {
		t.Fatalf(
			"quarantine available=%d; want 5",
			balance.AvailableQuantity,
		)
	}

	balance, err =
		ApplyMovementProjection(
			balance,
			history[3],
		)

	if err != nil {
		t.Fatal(err)
	}

	if balance.AvailableQuantity != 8 {
		t.Fatalf(
			"release available=%d; want 8",
			balance.AvailableQuantity,
		)
	}

	balance, err =
		ApplyMovementProjection(
			balance,
			history[4],
		)

	if err != nil {
		t.Fatal(err)
	}

	if balance.OnHandQuantity != 8 {
		t.Fatalf(
			"waste on_hand=%d; want 8",
			balance.OnHandQuantity,
		)
	}

	if balance.ReservedQuantity != 0 {
		t.Fatalf(
			"reserved=%d; want 0",
			balance.ReservedQuantity,
		)
	}

	if balance.QuarantinedQuantity != 2 {
		t.Fatalf(
			"quarantined=%d; want 2",
			balance.QuarantinedQuantity,
		)
	}
}

func TestSection14DMovementHistoryReconcilesExactly(
	t *testing.T,
) {
	history := []Movement{
		{
			Type: MovementReceive,

			Quantity: 20,

			OnHandDelta: 20,
		},
		{
			Type: MovementReserve,

			Quantity: 5,

			ReservedDelta: 5,
		},
		{
			Type: MovementRelease,

			Quantity: 2,

			ReservedDelta: -2,
		},
		{
			Type: MovementWaste,

			Quantity: 4,

			OnHandDelta: -4,
		},
	}

	var projected Balance

	for _, movement := range history {

		var err error

		projected, err =
			ApplyMovementProjection(
				projected,
				movement,
			)

		if err != nil {
			t.Fatal(err)
		}
	}

	if projected.OnHandQuantity != 16 {
		t.Fatalf(
			"on_hand=%d; want 16",
			projected.OnHandQuantity,
		)
	}

	if projected.ReservedQuantity != 3 {
		t.Fatalf(
			"reserved=%d; want 3",
			projected.ReservedQuantity,
		)
	}

	if projected.AvailableQuantity != 13 {
		t.Fatalf(
			"available=%d; want 13",
			projected.AvailableQuantity,
		)
	}
}

func TestSection14DProjectionRejectsNegativePhysicalTruth(
	t *testing.T,
) {
	_, err :=
		ApplyMovementProjection(
			Balance{},
			Movement{
				Type: MovementWaste,

				Quantity: 1,

				OnHandDelta: -1,
			},
		)

	if err == nil {
		t.Fatal(
			"negative stock projection was accepted",
		)
	}
}
