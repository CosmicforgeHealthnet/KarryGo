// Package ids mints prefixed identifiers.
//
// Prefixes are worth the few bytes: a pricing incident is investigated by reading
// identifiers out of logs, quotes and wallet records, and "pv_..." versus
// "act_..." versus "pq_..." tells a responder what they are looking at without a
// lookup.
package ids

import "github.com/google/uuid"

// Prefixes for each kind of record.
const (
	PrefixPolicy         = "pol"
	PrefixPolicyVersion  = "pv"
	PrefixActivation     = "act"
	PrefixApproval       = "ar"
	PrefixDecision       = "ad"
	PrefixQuote          = "pq"
	PrefixQuoteGroup     = "pqg"
	PrefixMarket         = "mkt"
	PrefixState          = "st"
	PrefixZone           = "zn"
	PrefixZoneVersion    = "znv"
	PrefixZoneMapping    = "znm"
	PrefixFreeze         = "frz"
	PrefixRollout        = "rol"
	PrefixReconciliation = "prc"
	PrefixFuelReport     = "fr"
	PrefixFuelProposal   = "fp"
)

// New returns a new prefixed identifier.
func New(prefix string) string { return prefix + "_" + uuid.NewString() }
