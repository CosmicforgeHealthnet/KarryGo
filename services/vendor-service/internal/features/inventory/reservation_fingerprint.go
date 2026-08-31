package inventory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

func validReservationSource(
	sourceType string,
) bool {
	switch sourceType {
	case ReservationSourceOrder,
		ReservationSourceCheckout,
		ReservationSourceRFQ,
		ReservationSourceManual,
		ReservationSourceInternal:
		return true
	default:
		return false
	}
}

func normalizeReservationInput(
	input CreateReservationInput,
) (
	CreateReservationInput,
	string,
	error,
) {
	input.BranchID =
		strings.TrimSpace(
			input.BranchID,
		)

	input.SourceType =
		strings.ToUpper(
			strings.TrimSpace(
				input.SourceType,
			),
		)

	input.SourceRef =
		strings.TrimSpace(
			input.SourceRef,
		)

	if input.BranchID == "" ||
		input.SourceRef == "" ||
		!validReservationSource(
			input.SourceType,
		) {
		return CreateReservationInput{},
			"",
			ErrReservationSourceInvalid
	}

	if len(input.Lines) == 0 ||
		len(input.Lines) > 500 {
		return CreateReservationInput{},
			"",
			ErrInvalidQuantity
	}

	aggregated :=
		make(
			map[string]int64,
			len(input.Lines),
		)

	for _, line := range input.Lines {

		itemID :=
			strings.TrimSpace(
				line.InventoryItemID,
			)

		if itemID == "" ||
			line.Quantity <= 0 ||
			line.Quantity > maxStockQuantity {

			return CreateReservationInput{},
				"",
				ErrInvalidQuantity
		}

		next :=
			aggregated[itemID] +
				line.Quantity

		if next <= 0 ||
			next > maxStockQuantity {
			return CreateReservationInput{},
				"",
				ErrInvalidQuantity
		}

		aggregated[itemID] =
			next
	}

	itemIDs :=
		make(
			[]string,
			0,
			len(aggregated),
		)

	for itemID := range aggregated {

		itemIDs =
			append(
				itemIDs,
				itemID,
			)
	}

	sort.Strings(itemIDs)

	input.Lines =
		make(
			[]ReservationLineInput,
			0,
			len(itemIDs),
		)

	for _, itemID := range itemIDs {

		input.Lines =
			append(
				input.Lines,
				ReservationLineInput{
					InventoryItemID: itemID,

					Quantity: aggregated[itemID],
				},
			)
	}

	type canonicalLine struct {
		InventoryItemID string `json:"inventory_item_id"`
		Quantity        int64  `json:"quantity"`
	}

	type canonicalRequest struct {
		BranchID string `json:"branch_id"`

		SourceType string `json:"source_type"`
		SourceRef  string `json:"source_ref"`

		ExpiresAt string `json:"expires_at"`

		Lines []canonicalLine `json:"lines"`
	}

	canonical :=
		canonicalRequest{
			BranchID: input.BranchID,

			SourceType: input.SourceType,

			SourceRef: input.SourceRef,
		}

	if input.ExpiresAt != nil {
		canonical.ExpiresAt =
			input.ExpiresAt.
				UTC().
				Format(
					"2006-01-02T15:04:05.999999999Z07:00",
				)
	}

	for _, line := range input.Lines {

		canonical.Lines =
			append(
				canonical.Lines,
				canonicalLine{
					InventoryItemID: line.InventoryItemID,

					Quantity: line.Quantity,
				},
			)
	}

	payload, err :=
		json.Marshal(
			canonical,
		)

	if err != nil {
		return CreateReservationInput{},
			"",
			err
	}

	sum :=
		sha256.Sum256(
			payload,
		)

	return input,
		hex.EncodeToString(
			sum[:],
		),
		nil
}
