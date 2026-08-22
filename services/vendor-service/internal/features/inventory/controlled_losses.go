package inventory

import (
	"context"
	"strings"
)

// Damage removes damaged sellable stock through the immutable movement ledger.
// It never edits the balance silently.
func (s *StockService) Damage(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	input QuantityMutationInput,
) (
	Balance,
	error,
) {
	if strings.TrimSpace(
		input.ReasonCode,
	) == "" {
		return Balance{},
			ErrReasonRequired
	}

	return s.decreasePhysical(
		ctx,
		actor,
		scope,
		input,
		MovementDamage,
	)
}
