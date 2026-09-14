package walletmodels

import "fmt"

// Section 12 WAL-029/030: dividing a delivery refund between the parties that
// actually received the money.
//
// A platform Commerce delivery used to have exactly one recipient, so a refund
// simply took the money back from the provider. A corporate delivery has two:
// the provider received what their compensation plan entitled them to -- often
// nothing at all, because a salaried rider is paid by their employer -- and the
// organization received the rest.
//
// Recovering the whole amount from the provider would therefore claw back money
// a salaried rider never had, and leave the organization holding money that was
// refunded to the customer. Both are real losses to a real party, and neither
// is detectable from the totals.

// DeliveryRecoverySplit is how much of a refund each party gives back.
type DeliveryRecoverySplit struct {
	ProviderMinor     int64 `json:"providerMinor"`
	OrganizationMinor int64 `json:"organizationMinor"`
}

// TotalMinor is what the split recovers in all.
func (s DeliveryRecoverySplit) TotalMinor() int64 { return s.ProviderMinor + s.OrganizationMinor }

// SplitDeliveryRecovery divides an amount to recover in proportion to what each
// party actually received.
//
// Money is neither created nor destroyed: the two parts always sum to exactly
// the requested amount, and neither part can exceed what that party received.
// Proportional division rarely lands on whole minor units, so each share is
// floored and the leftover units are then handed out by largest fractional
// remainder -- the same rule an apportionment problem uses, and the only one
// that both sums exactly and does not systematically favour one party.
//
// A tie is broken toward the provider. The choice matters less than it being
// fixed: an unstable tie-break would make the same refund recover different
// amounts on a replay, and a replay is exactly when it must not.
func SplitDeliveryRecovery(providerReceived, organizationReceived, recoverMinor int64) (DeliveryRecoverySplit, error) {
	if providerReceived < 0 || organizationReceived < 0 {
		return DeliveryRecoverySplit{}, fmt.Errorf("wallet: a party cannot have received a negative amount")
	}
	if recoverMinor < 0 {
		return DeliveryRecoverySplit{}, fmt.Errorf("wallet: cannot recover a negative amount")
	}
	total := providerReceived + organizationReceived
	if recoverMinor > total {
		return DeliveryRecoverySplit{}, fmt.Errorf(
			"wallet: cannot recover %d minor units from the %d that was paid out", recoverMinor, total)
	}
	if recoverMinor == 0 || total == 0 {
		return DeliveryRecoverySplit{}, nil
	}

	// Floor each share. Integer arithmetic throughout: a float here would make
	// the same refund round differently on different machines, which is how a
	// ledger stops balancing.
	providerShare := providerReceived * recoverMinor / total
	organizationShare := organizationReceived * recoverMinor / total
	leftover := recoverMinor - providerShare - organizationShare

	// Largest fractional remainder first. Comparing the remainders scaled by
	// the total avoids division entirely.
	providerRemainder := providerReceived*recoverMinor - providerShare*total
	organizationRemainder := organizationReceived*recoverMinor - organizationShare*total
	for leftover > 0 {
		if providerRemainder >= organizationRemainder && providerShare < providerReceived {
			providerShare++
			providerRemainder = -1
		} else if organizationShare < organizationReceived {
			organizationShare++
			organizationRemainder = -1
		} else if providerShare < providerReceived {
			providerShare++
		} else {
			// Unreachable while recoverMinor <= total, but a silent wrong
			// answer here would be a phantom credit.
			return DeliveryRecoverySplit{}, fmt.Errorf("wallet: delivery recovery could not be apportioned")
		}
		leftover--
	}

	split := DeliveryRecoverySplit{ProviderMinor: providerShare, OrganizationMinor: organizationShare}
	if split.TotalMinor() != recoverMinor {
		return DeliveryRecoverySplit{}, fmt.Errorf(
			"wallet: delivery recovery split %d does not reconcile to %d", split.TotalMinor(), recoverMinor)
	}
	if split.ProviderMinor > providerReceived || split.OrganizationMinor > organizationReceived {
		return DeliveryRecoverySplit{}, fmt.Errorf("wallet: delivery recovery split exceeds what a party received")
	}
	return split, nil
}
