package providercompat

import "cosmicforge/logistics/shared/go/logging"

// This file connects the financial classification registry to the platform
// logger.
//
// shared/go/logging redacts a field when its name sounds secret (token,
// password, otp) or when its value is not a plain scalar. Neither rule
// catches corporate economics: organizationShareMinor is an ordinary-looking
// name holding an ordinary integer, and platformFeeMinor likewise. Section 14
// spends an entire visibility policy keeping those away from providers, and a
// log line that prints them undoes it — logs are read by more people, and
// kept for longer, than any API response.
//
// The registration runs from providercompat rather than from logging so the
// dependency points the right way: the domain package that owns the
// classification teaches the infrastructure package, not the reverse. Every
// service already imports providercompat, so every service's logs get this.
func init() {
	logging.RegisterFieldRedactor(func(name string) bool {
		class, classified := ClassifyFinancialField(name)
		return classified && class.AlwaysConcealed()
	})
}
