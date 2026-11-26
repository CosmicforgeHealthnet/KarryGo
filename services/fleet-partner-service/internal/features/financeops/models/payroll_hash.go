package models

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// PayrollCalculationHash is the deterministic fingerprint of a payroll
// calculation: the set of compensation items it produced, independent of
// their order and of the calculation version number. Approval recomputes the
// items from current inputs and compares fingerprints, so a rule, membership
// or completed-work change after calculation blocks approval until the
// period is recalculated (spec 22.1, 52.2, FIN-003).
func PayrollCalculationHash(items []CompensationItem) string {
	lines := make([]string, 0, len(items))
	for _, i := range items {
		lines = append(lines, strings.Join([]string{
			i.MembershipID, i.SourceType, i.SourceReference, i.RuleID,
			fmt.Sprint(i.RuleVersion), fmt.Sprint(i.AmountMinor), i.Currency, i.ReasonCode, string(i.SourceInputs),
		}, "\x1f"))
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte("payroll-calculation-v1\x1e" + strings.Join(lines, "\x1e")))
	return hex.EncodeToString(sum[:])
}
