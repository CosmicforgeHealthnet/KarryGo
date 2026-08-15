package food

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
)

// ---------------------------------------------------------------------------
// Vocabularies
// ---------------------------------------------------------------------------

// Menu and item lifecycle. One closed set, shared by both, because a menu and
// an item answer the same administrative question: is this a draft, is it
// released, is it temporarily withheld, is it retired.
//
// PAUSED here is administrative. Section 9B's sold-out/effective availability
// is a different and finer-grained concept and is not this state.
const (
	StatusDraft    = "DRAFT"
	StatusActive   = "ACTIVE"
	StatusPaused   = "PAUSED"
	StatusArchived = "ARCHIVED"
)

// Child entities (sections, variants, modifier groups, modifiers) carry only
// the two states that mean anything for them: they are part of the catalogue,
// or they have been retired from it. A "draft section" is indistinguishable
// from a section on a draft menu, so the extra state would be noise.
const (
	ChildActive   = "ACTIVE"
	ChildArchived = "ARCHIVED"
)

// FoodCapabilities is the exact Section 9A capability boundary.
//
// CLOUD_KITCHEN, PASTRY_BAKERY, SHOP_GROCERY, MARKET and WHOLESALE are real
// KarryGo capabilities but are NOT this section's, and no repository
// specification assigns them to it. The schema takes new values by additive
// CHECK change, so a later section widens this without a destructive
// migration.
var FoodCapabilities = []string{"RESTAURANT", "FAST_FOOD"}

// IsFoodCapability reports whether a capability belongs to Section 9A.
func IsFoodCapability(value string) bool {
	for _, capability := range FoodCapabilities {
		if capability == value {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Domain models
// ---------------------------------------------------------------------------

// Menu is one branch's menu for one food capability.
type Menu struct {
	ID             string
	BusinessID     string
	BranchID       string
	CapabilityType string
	Name           string
	Description    string
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ArchivedAt     *time.Time
	Version        int64
}

// Section is an ordered group inside a menu.
type Section struct {
	ID          string
	BusinessID  string
	MenuID      string
	Name        string
	Description string
	Position    int
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ArchivedAt  *time.Time
	Version     int64
}

// Item is one sellable food item as the vendor configures it.
//
// BasePriceMinor is integer minor units of the business's own currency
// (vendor_businesses.currency). The currency is never restated here: one
// currency truth, resolved for the response rather than copied into the row.
type Item struct {
	ID             string
	BusinessID     string
	BranchID       string
	MenuID         string
	SectionID      string
	Name           string
	Description    string
	BasePriceMinor int64
	DietaryCodes   []string
	AllergenCodes  []string
	Status         string
	Position       int
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ArchivedAt     *time.Time
	Version        int64
}

// ItemMedia is one opaque Media File Service reference in display order.
type ItemMedia struct {
	ID         string
	BusinessID string
	ItemID     string
	MediaRef   string
	Position   int
	IsPrimary  bool
	CreatedAt  time.Time
}

// Variant is a size/portion of an item.
//
// PriceOverrideMinor nil means "inherit the item's base price". Section 9B
// layers effective-price resolution over this without changing the shape.
type Variant struct {
	ID                 string
	BusinessID         string
	ItemID             string
	Name               string
	PriceOverrideMinor *int64
	Position           int
	Status             string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	ArchivedAt         *time.Time
	Version            int64
}

// ModifierGroup is a business-level reusable choice group.
//
// MinSelect > 0 is what "required" means. There is no separate boolean,
// because two columns encoding one fact eventually disagree.
type ModifierGroup struct {
	ID          string
	BusinessID  string
	Name        string
	Description string
	MinSelect   int
	MaxSelect   int
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ArchivedAt  *time.Time
	Version     int64
}

// Modifier is one choice inside a group, priced as a non-negative surcharge.
type Modifier struct {
	ID              string
	BusinessID      string
	ModifierGroupID string
	Name            string
	PriceDeltaMinor int64
	Position        int
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ArchivedAt      *time.Time
	Version         int64
}

// ItemModifierGroup is the attachment of a group to an item, in display order.
type ItemModifierGroup struct {
	ID              string
	BusinessID      string
	ItemID          string
	ModifierGroupID string
	Position        int
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Version         int64
}

// ---------------------------------------------------------------------------
// Response views
// ---------------------------------------------------------------------------
//
// Database rows never leave the service directly. Every view below is an
// explicit projection, which is what keeps a column added for internal reasons
// from silently becoming part of the public contract.

type MenuView struct {
	ID             string     `json:"id"`
	BranchID       string     `json:"branch_id"`
	CapabilityType string     `json:"capability_type"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	ArchivedAt     *time.Time `json:"archived_at,omitempty"`
	Version        int64      `json:"version"`
}

func MenuViewOf(m Menu) MenuView {
	return MenuView{ID: m.ID, BranchID: m.BranchID, CapabilityType: m.CapabilityType,
		Name: m.Name, Description: m.Description, Status: m.Status,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt, ArchivedAt: m.ArchivedAt, Version: m.Version}
}

type SectionView struct {
	ID          string     `json:"id"`
	MenuID      string     `json:"menu_id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Position    int        `json:"position"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ArchivedAt  *time.Time `json:"archived_at,omitempty"`
	Version     int64      `json:"version"`
}

func SectionViewOf(s Section) SectionView {
	return SectionView{ID: s.ID, MenuID: s.MenuID, Name: s.Name, Description: s.Description,
		Position: s.Position, Status: s.Status, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
		ArchivedAt: s.ArchivedAt, Version: s.Version}
}

type ItemMediaView struct {
	MediaRef  string `json:"media_ref"`
	Position  int    `json:"position"`
	IsPrimary bool   `json:"is_primary"`
}

type ItemView struct {
	ID             string          `json:"id"`
	BranchID       string          `json:"branch_id"`
	MenuID         string          `json:"menu_id"`
	SectionID      string          `json:"section_id"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	BasePriceMinor int64           `json:"base_price_minor"`
	Currency       string          `json:"currency"`
	DietaryCodes   []string        `json:"dietary_codes"`
	AllergenCodes  []string        `json:"allergen_codes"`
	Media          []ItemMediaView `json:"media"`
	Status         string          `json:"status"`
	Position       int             `json:"position"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	ArchivedAt     *time.Time      `json:"archived_at,omitempty"`
	Version        int64           `json:"version"`
}

// ItemViewOf projects an item. currency comes from the business, media from
// the ordered reference rows; both are passed in rather than looked up here so
// a list endpoint can resolve them once for the whole page.
func ItemViewOf(i Item, currency string, media []ItemMedia) ItemView {
	views := make([]ItemMediaView, 0, len(media))
	for _, entry := range media {
		views = append(views, ItemMediaView{MediaRef: entry.MediaRef, Position: entry.Position, IsPrimary: entry.IsPrimary})
	}
	dietary := i.DietaryCodes
	if dietary == nil {
		dietary = []string{}
	}
	allergens := i.AllergenCodes
	if allergens == nil {
		allergens = []string{}
	}
	return ItemView{ID: i.ID, BranchID: i.BranchID, MenuID: i.MenuID, SectionID: i.SectionID,
		Name: i.Name, Description: i.Description, BasePriceMinor: i.BasePriceMinor, Currency: currency,
		DietaryCodes: dietary, AllergenCodes: allergens, Media: views, Status: i.Status,
		Position: i.Position, CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt,
		ArchivedAt: i.ArchivedAt, Version: i.Version}
}

type VariantView struct {
	ID                 string     `json:"id"`
	ItemID             string     `json:"item_id"`
	Name               string     `json:"name"`
	PriceOverrideMinor *int64     `json:"price_override_minor"`
	InheritsBasePrice  bool       `json:"inherits_base_price"`
	Position           int        `json:"position"`
	Status             string     `json:"status"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	ArchivedAt         *time.Time `json:"archived_at,omitempty"`
	Version            int64      `json:"version"`
}

func VariantViewOf(v Variant) VariantView {
	return VariantView{ID: v.ID, ItemID: v.ItemID, Name: v.Name, PriceOverrideMinor: v.PriceOverrideMinor,
		InheritsBasePrice: v.PriceOverrideMinor == nil, Position: v.Position, Status: v.Status,
		CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, ArchivedAt: v.ArchivedAt, Version: v.Version}
}

type ModifierGroupView struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	MinSelect   int        `json:"min_select"`
	MaxSelect   int        `json:"max_select"`
	Required    bool       `json:"required"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ArchivedAt  *time.Time `json:"archived_at,omitempty"`
	Version     int64      `json:"version"`
}

// ModifierGroupViewOf derives `required` rather than storing it, so the flag
// can never contradict the selection limits it is derived from.
func ModifierGroupViewOf(g ModifierGroup) ModifierGroupView {
	return ModifierGroupView{ID: g.ID, Name: g.Name, Description: g.Description,
		MinSelect: g.MinSelect, MaxSelect: g.MaxSelect, Required: g.MinSelect > 0, Status: g.Status,
		CreatedAt: g.CreatedAt, UpdatedAt: g.UpdatedAt, ArchivedAt: g.ArchivedAt, Version: g.Version}
}

type ModifierView struct {
	ID              string     `json:"id"`
	ModifierGroupID string     `json:"modifier_group_id"`
	Name            string     `json:"name"`
	PriceDeltaMinor int64      `json:"price_delta_minor"`
	Position        int        `json:"position"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	ArchivedAt      *time.Time `json:"archived_at,omitempty"`
	Version         int64      `json:"version"`
}

func ModifierViewOf(m Modifier) ModifierView {
	return ModifierView{ID: m.ID, ModifierGroupID: m.ModifierGroupID, Name: m.Name,
		PriceDeltaMinor: m.PriceDeltaMinor, Position: m.Position, Status: m.Status,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt, ArchivedAt: m.ArchivedAt, Version: m.Version}
}

// AttachedGroupView is the relationship plus enough of the group to render it,
// so a client does not have to fetch every group separately (the N+1 the list
// query deliberately avoids).
type AttachedGroupView struct {
	ModifierGroupID string            `json:"modifier_group_id"`
	Position        int               `json:"position"`
	Version         int64             `json:"version"`
	Group           ModifierGroupView `json:"group"`
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

// MaxPriceMinor mirrors the database ceiling. It is a sanity bound against a
// mistyped price, not a business limit, and sits far inside int64.
const MaxPriceMinor int64 = 99999999999

// Metadata bounds. Both are also enforced by CHECK constraints, so a direct
// writer cannot bypass them either.
const (
	MaxDietaryCodes   = 16
	MaxAllergenCodes  = 24
	MaxMediaRefs      = 12
	MaxReorderEntries = 500
)

// metadataCodePattern is a deliberately narrow shape: uppercase letters,
// digits and underscore only. That is enough to express any vendor vocabulary
// while making it impossible to smuggle markup, SQL or a URL into what is
// displayed as a dietary claim.
var metadataCodePattern = regexp.MustCompile(`^[A-Z0-9_]{2,40}$`)

// mediaRefPattern accepts the opaque identifier shapes Media File Service
// issues. It is a format bound, not an ownership check — ownership is proven
// against the Media private API, never inferred from the string.
var mediaRefPattern = regexp.MustCompile(`^[A-Za-z0-9_\-:.]{1,200}$`)

func validation(violations ...apperrors.FieldViolation) error {
	return apperrors.Validation("Check your details.", violations)
}

// normalizeName trims and collapses interior whitespace so "Extra  Cheese" and
// "Extra Cheese " cannot both exist as separate live rows.
func normalizeName(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func validateName(field, value string, min, max int) (string, *apperrors.FieldViolation) {
	name := normalizeName(value)
	if len(name) < min || len(name) > max {
		return "", &apperrors.FieldViolation{Field: field,
			Message: "Name must be between " + itoa(min) + " and " + itoa(max) + " characters."}
	}
	return name, nil
}

func validateDescription(field, value string, max int) (string, *apperrors.FieldViolation) {
	description := strings.TrimSpace(value)
	if len(description) > max {
		return "", &apperrors.FieldViolation{Field: field, Message: "Description is too long."}
	}
	return description, nil
}

func validatePriceMinor(field string, value int64) *apperrors.FieldViolation {
	if value < 0 {
		return &apperrors.FieldViolation{Field: field, Message: "Price must not be negative."}
	}
	if value > MaxPriceMinor {
		return &apperrors.FieldViolation{Field: field, Message: "Price is outside the supported range."}
	}
	return nil
}

// normalizeCodes upper-cases, de-duplicates and sorts a metadata code set.
//
// Sorting makes the stored value canonical, so two vendors who typed the same
// allergens in a different order produce the same row and the same audit diff.
func normalizeCodes(field string, values []string, max int) ([]string, *apperrors.FieldViolation) {
	if len(values) > max {
		return nil, &apperrors.FieldViolation{Field: field, Message: "Too many codes were supplied."}
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, raw := range values {
		code := strings.ToUpper(strings.TrimSpace(raw))
		if !metadataCodePattern.MatchString(code) {
			return nil, &apperrors.FieldViolation{Field: field,
				Message: "Codes must be 2-40 characters of A-Z, 0-9 or underscore."}
		}
		if seen[code] {
			return nil, &apperrors.FieldViolation{Field: field, Message: "Duplicate codes are not allowed."}
		}
		seen[code] = true
		out = append(out, code)
	}
	sort.Strings(out)
	return out, nil
}

// validateSelection enforces the only two rules a choice group needs. Anything
// richer would be a rule engine, which this domain deliberately does not have.
func validateSelection(min, max int) *apperrors.FieldViolation {
	if min < 0 || min > 50 {
		return &apperrors.FieldViolation{Field: "min_select", Message: "Minimum selections must be between 0 and 50."}
	}
	if max < 1 || max > 50 {
		return &apperrors.FieldViolation{Field: "max_select", Message: "Maximum selections must be between 1 and 50."}
	}
	if max < min {
		return &apperrors.FieldViolation{Field: "max_select", Message: "Maximum selections must be at least the minimum."}
	}
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if negative {
		return "-" + string(digits)
	}
	return string(digits)
}
