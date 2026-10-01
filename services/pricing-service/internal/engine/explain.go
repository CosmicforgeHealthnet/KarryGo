package engine

import "fmt"

// explain renders the derivation in plain language.
//
// It is stored with the quote so that Operations, Finance or a dispute reviewer
// can read what happened without a developer and without re-running the
// arithmetic. It is generated from the same values that were charged, so it
// cannot drift from them.
func explain(req Request, result Result, commissionBPS int64) []string {
	lines := []string{
		fmt.Sprintf("%s %s priced for the %s channel.", req.Scope, req.Class, req.Channel),
		fmt.Sprintf("Route: %s metres, %s seconds (route confidence %s%s).",
			withThousands(req.DistanceMeters), withThousands(req.DurationSeconds),
			orUnknown(req.RouteConfidence), degradedSuffix(req.RouteDegraded)),
	}

	for _, component := range result.Components {
		lines = append(lines, fmt.Sprintf("%s: %s -- %s", component.Code, kobo(component.AmountKobo), component.Basis))
	}

	lines = append(lines, fmt.Sprintf("Economic fare: %s.", kobo(result.GrossKobo)))
	lines = append(lines, fmt.Sprintf("Funded by: customer %s, merchant %s, KarryGo %s.",
		kobo(result.Funding.CustomerKobo), kobo(result.Funding.MerchantKobo), kobo(result.Funding.PlatformSubsidyKobo)))
	lines = append(lines, fmt.Sprintf("Allocated to: provider %s, KarryGo fee %s (%s), tax %s.",
		kobo(result.Allocation.ProviderKobo), kobo(result.Allocation.PlatformFeeKobo),
		basisPointsLabel(commissionBPS), kobo(result.Allocation.TaxKobo)))

	for _, flag := range result.Flags {
		lines = append(lines, "Flag: "+flag)
	}
	return lines
}

func basisPointsLabel(bps int64) string {
	whole := bps / 100
	fraction := bps % 100
	if fraction == 0 {
		return fmt.Sprintf("%d%%", whole)
	}
	return fmt.Sprintf("%d.%02d%%", whole, fraction)
}

func orUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

func degradedSuffix(degraded bool) string {
	if degraded {
		return ", degraded"
	}
	return ""
}
