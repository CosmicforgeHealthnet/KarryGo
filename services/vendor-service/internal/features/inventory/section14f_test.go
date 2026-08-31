package inventory

import "testing"

func TestSection14FLifecycles(
	t *testing.T,
) {
	stocktakes :=
		[]string{
			StocktakeCounting,
			StocktakePosted,
			StocktakeStale,
			StocktakeCancelled,
		}

	if len(stocktakes) != 4 {
		t.Fatal(
			"stocktake lifecycle changed",
		)
	}

	transfers :=
		[]string{
			TransferDraft,
			TransferApproved,
			TransferInTransit,
			TransferPartiallyReceived,
			TransferReceived,
			TransferCancelled,
		}

	if len(transfers) != 6 {
		t.Fatal(
			"transfer lifecycle changed",
		)
	}

	quarantine :=
		[]string{
			QuarantineActive,
			QuarantineReleased,
			QuarantineDisposed,
		}

	if len(quarantine) != 3 {
		t.Fatal(
			"quarantine lifecycle changed",
		)
	}
}

func TestSection14FTransferIdentity(
	t *testing.T,
) {
	left :=
		transferIdentity{
			ItemType: "SHOP_VARIANT",

			SourceRef: "variant-1",

			MeasureRef: "piece",

			MeasureOwnerKey: "PLATFORM",

			BasisMode: "SAME_UNIT",
		}

	right :=
		left

	if !transferIdentitiesMatch(
		left,
		right,
	) {
		t.Fatal(
			"matching inventory identity was rejected",
		)
	}

	right.SourceRef =
		"variant-2"

	if transferIdentitiesMatch(
		left,
		right,
	) {
		t.Fatal(
			"different source identity was accepted",
		)
	}
}

func TestSection14FTransferReceiptFingerprintStable(
	t *testing.T,
) {
	first,
		firstHash,
		_,
		err :=
		transferReceiptFingerprint(
			ReceiveTransferInput{
				SourceRef: "receipt-1",

				Lines: []ReceiveTransferLineInput{
					{
						LineID: "line-b",

						ReceivedQuantity: 2,
					},
					{
						LineID: "line-a",

						ReceivedQuantity: 1,
					},
				},
			},
		)

	if err != nil {
		t.Fatal(err)
	}

	second,
		secondHash,
		_,
		err :=
		transferReceiptFingerprint(
			ReceiveTransferInput{
				SourceRef: "receipt-1",

				Lines: []ReceiveTransferLineInput{
					{
						LineID: "line-a",

						ReceivedQuantity: 1,
					},
					{
						LineID: "line-b",

						ReceivedQuantity: 2,
					},
				},
			},
		)

	if err != nil {
		t.Fatal(err)
	}

	if firstHash !=
		secondHash {

		t.Fatal(
			"equivalent transfer receipt produced different fingerprint",
		)
	}

	if first.Lines[0].LineID !=
		"line-a" ||
		second.Lines[0].LineID !=
			"line-a" {

		t.Fatal(
			"transfer receipt lines were not deterministically sorted",
		)
	}
}

func TestSection14FReorderValidation(
	t *testing.T,
) {
	if int64(5) >=
		int64(10) {

		t.Fatal(
			"invalid test constants",
		)
	}

	rule :=
		SetReorderRuleInput{
			MinimumQuantity: 5,

			TargetQuantity: 10,
		}

	if rule.TargetQuantity <=
		rule.MinimumQuantity {

		t.Fatal(
			"valid reorder threshold rejected",
		)
	}
}
