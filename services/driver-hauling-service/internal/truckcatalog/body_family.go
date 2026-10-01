package truckcatalog

// BodyFamily groups truck types by the shape of their load space.
//
// The family, not the type, is what a cargo requirement actually cares about.
// A customer asking for "something enclosed" does not mean "a box truck
// specifically"; they mean any body that keeps rain off the goods. Matching on
// the type alone is what made the old exact-string comparison reject a
// perfectly suitable van because the booking happened to say "box".
type BodyFamily string

const (
	// FamilyOpenDeck is a flat load bed with no sides or roof: flatbed,
	// lowbed. Nothing on it is enclosed or weather-protected.
	FamilyOpenDeck BodyFamily = "OPEN_DECK"

	// FamilyEnclosedBody is a hard-sided, roofed cargo box: van, box truck.
	FamilyEnclosedBody BodyFamily = "ENCLOSED_BODY"

	// FamilyContainer carries ISO containers, which are themselves enclosed
	// and weatherproof.
	FamilyContainer BodyFamily = "CONTAINER"

	// FamilyReefer is an enclosed body with active temperature control.
	FamilyReefer BodyFamily = "REEFER"

	// FamilyTipper discharges by tilting the bed: tipper, dump. Open by
	// construction and unsuitable for anything that must stay upright.
	FamilyTipper BodyFamily = "TIPPER"

	// FamilyTank carries bulk liquid or gas in a sealed vessel.
	FamilyTank BodyFamily = "TANK"

	// FamilyArticulated is a tractor plus a separate trailer whose body
	// varies with the trailer fitted, so the family alone implies nothing
	// about enclosure.
	FamilyArticulated BodyFamily = "ARTICULATED"

	// FamilyLightOpen is a light open-bed vehicle: pickup.
	FamilyLightOpen BodyFamily = "LIGHT_OPEN"

	// FamilySpecialized is equipment-led rather than body-led: crane.
	FamilySpecialized BodyFamily = "SPECIALIZED"

	// FamilyOther is the escape hatch for a truck whose body the platform has
	// no model for. It implies nothing, which is the point: an unmodelled
	// body must never be assumed to have a property.
	FamilyOther BodyFamily = "OTHER"
)

// Valid reports whether the family is one the catalogue defines.
func (f BodyFamily) Valid() bool {
	_, ok := familyLabels[f]
	return ok
}

// Label is the customer-facing name for a family.
func (f BodyFamily) Label() string {
	if label, ok := familyLabels[f]; ok {
		return label
	}
	return string(f)
}

var familyLabels = map[BodyFamily]string{
	FamilyOpenDeck:     "Open deck",
	FamilyEnclosedBody: "Enclosed body",
	FamilyContainer:    "Container carrier",
	FamilyReefer:       "Refrigerated",
	FamilyTipper:       "Tipper",
	FamilyTank:         "Tanker",
	FamilyArticulated:  "Articulated",
	FamilyLightOpen:    "Light open bed",
	FamilySpecialized:  "Specialized equipment",
	FamilyOther:        "Other",
}

// Families lists every family in a stable order, for product listings.
func Families() []BodyFamily {
	return []BodyFamily{
		FamilyLightOpen, FamilyEnclosedBody, FamilyOpenDeck, FamilyContainer,
		FamilyReefer, FamilyTipper, FamilyTank, FamilyArticulated,
		FamilySpecialized, FamilyOther,
	}
}
