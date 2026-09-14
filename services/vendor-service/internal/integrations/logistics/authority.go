// Package logistics is Vendor Service's single vocabulary for the logistics
// authorities that execute commerce fulfilment.
//
// Vendor Service grew four separate authority enums — one in orders, one in
// fulfilmentgroup for plans, one in fulfilmentgroup for custody reporting, and
// one in fulfilment for vehicle selection. They happened to agree on their
// wire values, which is luck rather than design: nothing held them together,
// and the first divergence would have been silent.
//
// This package is that holding. It owns the canonical vocabulary, the parse
// rules, the normalized execution state, and the normalized error categories,
// and it deliberately imports no Vendor feature so every feature can depend on
// it without a cycle. A consistency test proves the feature-local enums still
// spell the same authorities this package does.
//
// Two things this package is NOT. It never names a provider: an authority is
// the service that will execute, never the driver, rider or vehicle that ends
// up doing it, and provider identity is late-bound by the execution service or
// Fleet. And it never prices: capability answers what can be carried, not what
// it costs.
package logistics

import (
	"sort"
	"strings"

	"cosmicforge/logistics/shared/go/logisticscapability"
)

// Authority is the canonical name of an execution authority.
//
// It is the shared platform vocabulary rather than a Vendor-local copy, so a
// value that travels to a logistics service, a capability query and a Vendor
// projection is the same value throughout.
type Authority = logisticscapability.Authority

const (
	// Dispatch is driver-dispatch-delivery-service.
	Dispatch = logisticscapability.AuthorityDispatch

	// Taxi is driver-taxi-service's goods product, never its passenger
	// booking surface. The wire value stays TAXI because every service,
	// signature allowlist and persisted row already spells it that way;
	// TAXI_GOODS is accepted as an input alias so the product name and the
	// stored name can never drift apart.
	Taxi = logisticscapability.AuthorityTaxi

	// Hauling is driver-hauling-service, which owns both van and truck.
	Hauling = logisticscapability.AuthorityHauling

	// VendorOwn is the vendor's own delivery. It is not a platform logistics
	// authority: it creates no mission in any driver service.
	VendorOwn = logisticscapability.AuthorityVendorOwn
)

// TaxiGoodsAlias is the product-facing spelling of the Taxi goods authority.
const TaxiGoodsAlias = logisticscapability.TaxiGoodsAlias

// VendorOwnLegacyAlias is the value fulfilmentgroup persists for vendor-owned
// delivery. It is accepted on input so existing plan rows keep parsing.
const VendorOwnLegacyAlias = "VENDOR"

// Parse resolves any accepted spelling to the canonical authority.
//
// It accepts the aliases rather than rejecting them because the alternative is
// a data migration across persisted plans and signed payloads for no
// behavioural gain, and because refusing a spelling the platform itself writes
// would fail closed on correct data.
func Parse(value string) (Authority, error) {
	return logisticscapability.ParseAuthority(value)
}

// PlatformExecuted reports whether an authority is a platform logistics
// service that receives a mission. Vendor-owned delivery is not.
func PlatformExecuted(authority Authority) bool { return authority.PlatformExecuted() }

// PlatformAuthorities are the three platform execution authorities, in a
// stable order so callers and tests never depend on map iteration.
func PlatformAuthorities() []Authority {
	return []Authority{Dispatch, Taxi, Hauling}
}

// Equal reports whether two spellings name the same authority.
//
// Comparing raw strings is what allowed TAXI and TAXI_GOODS, and VENDOR and
// VENDOR_OWN_DELIVERY, to be treated as different authorities in different
// corners of the service. Callers compare through here instead.
func Equal(left, right string) bool {
	first, err := Parse(left)
	if err != nil {
		return false
	}
	second, secondErr := Parse(right)
	if secondErr != nil {
		return false
	}
	return first == second
}

// Normalize returns the canonical spelling of an accepted value, or the empty
// string when the value names no authority.
func Normalize(value string) string {
	authority, err := Parse(value)
	if err != nil {
		return ""
	}
	return string(authority)
}

// AcceptedSpellings lists every input spelling that resolves to an authority.
// It exists so a test can prove the accepted set is exactly what is intended
// rather than whatever the parser happens to tolerate.
func AcceptedSpellings() []string {
	spellings := []string{
		string(Dispatch),
		string(Taxi), TaxiGoodsAlias,
		string(Hauling),
		string(VendorOwn), VendorOwnLegacyAlias,
	}
	sort.Strings(spellings)
	return spellings
}

// SelectionSource records how an authority came to be chosen, so a mission can
// prove the choice was a frozen commerce decision rather than a later runtime
// heuristic (VND-004).
type SelectionSource string

const (
	// SelectedAtCheckout is the customer-approved fulfilment plan's own
	// choice. It is the only source that may be used to create a first
	// mission.
	SelectedAtCheckout SelectionSource = "CHECKOUT"

	// SelectedByReplan is an explicit, recorded replan or recovery operation.
	SelectedByReplan SelectionSource = "REPLAN"
)

// Valid reports whether a selection source is one Vendor recognises.
func (s SelectionSource) Valid() bool {
	return s == SelectedAtCheckout || s == SelectedByReplan
}

// TrimUpper is the one normalization applied to authority-adjacent free text
// before comparison, kept here so every call site does it identically.
func TrimUpper(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}
