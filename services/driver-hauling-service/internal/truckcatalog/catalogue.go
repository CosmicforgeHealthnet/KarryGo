// Package truckcatalog is the single source of truth for what a truck type IS
// in this service.
//
// It exists because the service previously carried two independent copies of
// the same taxonomy -- bookingmodels.ValidPreferredTruckTypes and
// providerprofilemodels.ValidTruckTypes -- whose own comments asked future
// authors to keep them in sync by hand. They did not diverge by luck rather
// than by design, and the matcher compared them with exact string equality, so
// the first divergence would have produced bookings that silently matched no
// provider at all: stored happily, dispatched never.
//
// Everything that needs to know about truck types now reads this package:
// booking validation, provider registration, capability validation, matching
// and product mapping. internal/truckcatalog/architecture_test.go fails the
// build if a second list reappears.
//
// What this package DOES infer, and what it deliberately does not, matters.
// A type name settles some facts by definition -- a flatbed has no roof, a
// refrigerated truck refrigerates -- and those are recorded here as declared
// facts because they are true of every vehicle of that type. Measurements are
// never inferred: no payload, no internal dimensions, no opening size. A
// "box truck" tells you the body is enclosed; it tells you nothing about
// whether a 3.2m beam fits through the door.
package truckcatalog

import (
	"sort"
	"strings"

	"cosmicforge/logistics/shared/go/capabilityfit"
	"cosmicforge/logistics/shared/go/commercequote"
)

// Type is a canonical, persisted truck type code.
//
// These are the values already stored in trucks.truck_type and
// haulage_bookings.preferred_truck_type. They are lowercase because that is
// what the existing rows contain; renaming them would be a data migration for
// no behavioural gain.
type Type string

const (
	TypePickup       Type = "pickup"
	TypeVan          Type = "van"
	TypeBox          Type = "box"
	TypeFlatbed      Type = "flatbed"
	TypeLowbed       Type = "lowbed"
	TypeContainer    Type = "container"
	TypeRefrigerated Type = "refrigerated"
	TypeTipper       Type = "tipper"
	TypeDump         Type = "dump"
	TypeTanker       Type = "tanker"
	TypeTrailer      Type = "trailer"
	TypeCrane        Type = "crane"
	TypeOther        Type = "other"
)

// Entry is everything the platform knows about a truck type from its name
// alone.
//
// The capability fields are capabilityfit.Tri rather than bool precisely so
// that "the name does not settle this" is representable. TypeTrailer is the
// clearest case: an articulated unit's body depends entirely on the trailer
// coupled to it, so Enclosed is Unknown and a job needing enclosure must look
// at the truck's own recorded capability row rather than trusting the type.
type Entry struct {
	// Code is the persisted value.
	Code Type

	// Family groups the type by load-space shape.
	Family BodyFamily

	// Label is the customer-facing name.
	Label string

	// VehicleClass maps the type onto the cross-service quote/mission
	// vocabulary, so one truck type resolves to one class everywhere.
	VehicleClass commercequote.VehicleClass

	// Enclosed / WeatherProtected / Refrigerated are the facts the type name
	// settles by definition. Unknown means the name settles nothing and the
	// truck's own capability record must answer.
	Enclosed         capabilityfit.Tri
	WeatherProtected capabilityfit.Tri
	Refrigerated     capabilityfit.Tri

	// UprightSafe is No for bodies that discharge by tilting: a tipper cannot
	// promise a load stays the way up it was loaded.
	UprightSafe capabilityfit.Tri

	// CarriesContainers reports whether the type is built to carry ISO
	// containers.
	CarriesContainers bool

	// BulkLiquidOnly marks a body that carries nothing but bulk liquid or
	// gas, so general cargo can never be assigned to it.
	BulkLiquidOnly bool

	// ProviderSelectable reports whether a provider may register a truck of
	// this type. Every current type is selectable; the field exists so a type
	// can be retired from new registrations without breaking the historical
	// rows that still reference it.
	ProviderSelectable bool

	// CustomerSelectable reports whether a booking may request this type.
	CustomerSelectable bool
}

// catalogue is the one definition. Order here is presentation order.
var catalogue = []Entry{
	{
		Code: TypePickup, Family: FamilyLightOpen, Label: "Pickup",
		VehicleClass: commercequote.VehicleVan,
		Enclosed:     capabilityfit.No, WeatherProtected: capabilityfit.No,
		Refrigerated: capabilityfit.No, UprightSafe: capabilityfit.Yes,
		ProviderSelectable: true, CustomerSelectable: true,
	},
	{
		Code: TypeVan, Family: FamilyEnclosedBody, Label: "Van",
		VehicleClass: commercequote.VehicleVan,
		Enclosed:     capabilityfit.Yes, WeatherProtected: capabilityfit.Yes,
		Refrigerated: capabilityfit.No, UprightSafe: capabilityfit.Yes,
		ProviderSelectable: true, CustomerSelectable: true,
	},
	{
		Code: TypeBox, Family: FamilyEnclosedBody, Label: "Box truck",
		VehicleClass: commercequote.VehicleTruck,
		Enclosed:     capabilityfit.Yes, WeatherProtected: capabilityfit.Yes,
		Refrigerated: capabilityfit.No, UprightSafe: capabilityfit.Yes,
		ProviderSelectable: true, CustomerSelectable: true,
	},
	{
		Code: TypeFlatbed, Family: FamilyOpenDeck, Label: "Flatbed",
		VehicleClass: commercequote.VehicleTruck,
		Enclosed:     capabilityfit.No, WeatherProtected: capabilityfit.No,
		Refrigerated: capabilityfit.No, UprightSafe: capabilityfit.Yes,
		ProviderSelectable: true, CustomerSelectable: true,
	},
	{
		Code: TypeLowbed, Family: FamilyOpenDeck, Label: "Lowbed",
		VehicleClass: commercequote.VehicleTruck,
		Enclosed:     capabilityfit.No, WeatherProtected: capabilityfit.No,
		Refrigerated: capabilityfit.No, UprightSafe: capabilityfit.Yes,
		ProviderSelectable: true, CustomerSelectable: true,
	},
	{
		Code: TypeContainer, Family: FamilyContainer, Label: "Container carrier",
		VehicleClass: commercequote.VehicleTruck,
		Enclosed:     capabilityfit.Yes, WeatherProtected: capabilityfit.Yes,
		Refrigerated: capabilityfit.No, UprightSafe: capabilityfit.Yes,
		CarriesContainers: true,
		// A container carrier is a chassis for an ISO box, so "enclosed" is a
		// fact about the container it carries. That is still the truthful
		// answer for cargo purposes: goods inside the box are enclosed and
		// weatherproof.
		ProviderSelectable: true, CustomerSelectable: true,
	},
	{
		Code: TypeRefrigerated, Family: FamilyReefer, Label: "Refrigerated",
		VehicleClass: commercequote.VehicleTruck,
		Enclosed:     capabilityfit.Yes, WeatherProtected: capabilityfit.Yes,
		// The one type whose name settles refrigeration. Its actual
		// temperature RANGE is still a measurement and is never inferred.
		Refrigerated: capabilityfit.Yes, UprightSafe: capabilityfit.Yes,
		ProviderSelectable: true, CustomerSelectable: true,
	},
	{
		Code: TypeTipper, Family: FamilyTipper, Label: "Tipper",
		VehicleClass: commercequote.VehicleTruck,
		Enclosed:     capabilityfit.No, WeatherProtected: capabilityfit.No,
		Refrigerated: capabilityfit.No,
		// A body that discharges by tilting cannot promise orientation.
		UprightSafe:        capabilityfit.No,
		ProviderSelectable: true, CustomerSelectable: true,
	},
	{
		Code: TypeDump, Family: FamilyTipper, Label: "Dump truck",
		VehicleClass: commercequote.VehicleTruck,
		Enclosed:     capabilityfit.No, WeatherProtected: capabilityfit.No,
		Refrigerated: capabilityfit.No, UprightSafe: capabilityfit.No,
		ProviderSelectable: true, CustomerSelectable: true,
	},
	{
		Code: TypeTanker, Family: FamilyTank, Label: "Tanker",
		VehicleClass: commercequote.VehicleTruck,
		Enclosed:     capabilityfit.Yes, WeatherProtected: capabilityfit.Yes,
		Refrigerated: capabilityfit.No, UprightSafe: capabilityfit.No,
		BulkLiquidOnly:     true,
		ProviderSelectable: true, CustomerSelectable: true,
	},
	{
		Code: TypeTrailer, Family: FamilyArticulated, Label: "Articulated trailer",
		VehicleClass: commercequote.VehicleTruck,
		// An articulated unit's body is whatever trailer is coupled to it, so
		// the type name settles nothing about enclosure. Leaving these Unknown
		// is what forces a job that needs enclosure to consult the truck's own
		// capability record instead of guessing from the word "trailer".
		Enclosed: capabilityfit.Unknown, WeatherProtected: capabilityfit.Unknown,
		Refrigerated: capabilityfit.Unknown, UprightSafe: capabilityfit.Unknown,
		CarriesContainers:  true,
		ProviderSelectable: true, CustomerSelectable: true,
	},
	{
		Code: TypeCrane, Family: FamilySpecialized, Label: "Crane truck",
		VehicleClass: commercequote.VehicleTruck,
		Enclosed:     capabilityfit.No, WeatherProtected: capabilityfit.No,
		Refrigerated: capabilityfit.No, UprightSafe: capabilityfit.Yes,
		ProviderSelectable: true, CustomerSelectable: true,
	},
	{
		Code: TypeOther, Family: FamilyOther, Label: "Other",
		VehicleClass: commercequote.VehicleTruck,
		// "Other" is the absence of a model, so it declares nothing at all.
		// Every capability question about such a truck must be answered by
		// its own recorded row or refused as unknown.
		Enclosed: capabilityfit.Unknown, WeatherProtected: capabilityfit.Unknown,
		Refrigerated: capabilityfit.Unknown, UprightSafe: capabilityfit.Unknown,
		ProviderSelectable: true, CustomerSelectable: true,
	},
}

var byCode = func() map[Type]Entry {
	index := make(map[Type]Entry, len(catalogue))
	for _, entry := range catalogue {
		index[entry.Code] = entry
	}
	return index
}()

// Lookup resolves a raw stored or submitted value to its catalogue entry.
//
// It normalizes case and surrounding whitespace, because both the provider
// form and the booking API have historically stored mixed-case values.
func Lookup(raw string) (Entry, bool) {
	entry, ok := byCode[Normalize(raw)]
	return entry, ok
}

// Normalize converts a raw value to its canonical persisted form without
// asserting that it exists.
func Normalize(raw string) Type {
	return Type(strings.ToLower(strings.TrimSpace(raw)))
}

// All returns every entry in presentation order.
func All() []Entry {
	out := make([]Entry, len(catalogue))
	copy(out, catalogue)
	return out
}

// ProviderSelectable returns the types a provider may register, in
// presentation order.
func ProviderSelectable() []Entry {
	return filter(func(e Entry) bool { return e.ProviderSelectable })
}

// CustomerSelectable returns the types a booking may request.
func CustomerSelectable() []Entry {
	return filter(func(e Entry) bool { return e.CustomerSelectable })
}

func filter(keep func(Entry) bool) []Entry {
	out := make([]Entry, 0, len(catalogue))
	for _, entry := range catalogue {
		if keep(entry) {
			out = append(out, entry)
		}
	}
	return out
}

// InFamily returns every type belonging to a family, sorted by code so the
// answer is stable across calls.
func InFamily(family BodyFamily) []Entry {
	out := filter(func(e Entry) bool { return e.Family == family })
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// Codes renders entries as their persisted string values.
func Codes(entries []Entry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, string(entry.Code))
	}
	return out
}
