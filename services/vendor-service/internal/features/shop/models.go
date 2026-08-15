package shop

import (
	"regexp"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
)

// ---------------------------------------------------------------------------
// Vocabularies
// ---------------------------------------------------------------------------

// Lifecycle states. Products carry the full four because a Shop product is
// prepared before it sells and may be withheld without being retired;
// everything else needs only "in the catalogue" or "retired from it", with
// listings adding INACTIVE for a branch that has stopped selling something it
// has not stopped stocking.
const (
	StatusDraft    = "DRAFT"
	StatusActive   = "ACTIVE"
	StatusPaused   = "PAUSED"
	StatusInactive = "INACTIVE"
	StatusArchived = "ARCHIVED"
)

// Operating models refine Shop behaviour for one branch. They are not
// capabilities, not services and not separate catalogues.
const (
	ModelSupermarket     = "SUPERMARKET"
	ModelGroceryShop     = "GROCERY_SHOP"
	ModelFoodstuffDealer = "FOODSTUFF_DEALER"
	ModelMarketTrader    = "MARKET_TRADER"
	ModelWholesaler      = "WHOLESALER"
)

// OperatingModels is the closed set, in presentation order.
var OperatingModels = []string{
	ModelSupermarket, ModelGroceryShop, ModelFoodstuffDealer, ModelMarketTrader, ModelWholesaler,
}

// ShopCapabilities is the exact Vendor capability boundary for this section.
// RESTAURANT, FAST_FOOD, CLOUD_KITCHEN and PASTRY_BAKERY are real KarryGo
// capabilities and are emphatically NOT this section's: a business that owns
// both a restaurant and a shop owns two catalogues, not one.
var ShopCapabilities = []string{"SHOP_GROCERY", "MARKET", "WHOLESALE"}

// operatingModelCapabilities maps each model to the capabilities that admit
// it. A branch must hold one of them, on that branch, before the model can be
// enabled — holding it at another branch of the same business proves nothing.
var operatingModelCapabilities = map[string][]string{
	ModelSupermarket:     {"SHOP_GROCERY"},
	ModelGroceryShop:     {"SHOP_GROCERY"},
	ModelFoodstuffDealer: {"SHOP_GROCERY", "WHOLESALE"},
	ModelMarketTrader:    {"MARKET"},
	ModelWholesaler:      {"WHOLESALE"},
}

// IsOperatingModel reports membership of the closed set.
func IsOperatingModel(value string) bool {
	_, ok := operatingModelCapabilities[value]
	return ok
}

// CapabilitiesForOperatingModel returns the capabilities that admit a model.
func CapabilitiesForOperatingModel(model string) []string {
	return operatingModelCapabilities[model]
}

// IsShopCapability reports whether a capability belongs to the Shop domain.
func IsShopCapability(value string) bool {
	for _, capability := range ShopCapabilities {
		if capability == value {
			return true
		}
	}
	return false
}

// Measurement families. A family decides whether a canonical base unit exists
// at all: MASS, VOLUME and COUNT convert exactly, and nothing else does.
const (
	FamilyMass           = "MASS"
	FamilyVolume         = "VOLUME"
	FamilyCount          = "COUNT"
	FamilyLocalContainer = "LOCAL_CONTAINER"
	FamilyPackage        = "PACKAGE"
	FamilyGroupedCount   = "GROUPED_COUNT"
	FamilyLoad           = "LOAD"
	FamilyVariableWeight = "VARIABLE_WEIGHT"
	FamilyVariableCount  = "VARIABLE_COUNT"
	FamilyPricePortion   = "PRICE_PORTION"
	FamilyOther          = "OTHER_CONTROLLED"
)

// MeasurementFamilies is the closed family set.
var MeasurementFamilies = []string{
	FamilyMass, FamilyVolume, FamilyCount, FamilyLocalContainer, FamilyPackage,
	FamilyGroupedCount, FamilyLoad, FamilyVariableWeight, FamilyVariableCount,
	FamilyPricePortion, FamilyOther,
}

// ConvertibleFamilies are the only families with a canonical integer base:
// MASS in grams, VOLUME in millilitres, COUNT in units.
var ConvertibleFamilies = []string{FamilyMass, FamilyVolume, FamilyCount}

// IsMeasurementFamily reports membership of the closed set.
func IsMeasurementFamily(value string) bool { return contains(MeasurementFamilies, value) }

// IsConvertibleFamily reports whether the family has a canonical base unit.
func IsConvertibleFamily(value string) bool { return contains(ConvertibleFamilies, value) }

// Measure origin.
const (
	OriginStandard      = "STANDARD"
	OriginLocal         = "LOCAL"
	OriginPackage       = "PACKAGE"
	OriginVendorDefined = "VENDOR_DEFINED"
)

// PlatformOwnerKey is the owner key every platform-seeded measure carries. A
// referencing row may only name this or its own business, which is what stops
// one tenant's custom measure appearing in another's catalogue.
const PlatformOwnerKey = "PLATFORM"

// Product classes.
var ProductClasses = []string{
	"PACKAGED", "DRY_BULK", "FRESH_PRODUCE", "MEAT", "FISH", "FROZEN", "CHILLED", "DAIRY",
	"BEVERAGE", "BAKED_PACKAGED", "HOUSEHOLD_GROCERY", "LOCAL_FOODSTUFF", "OTHER",
}

// IsProductClass reports membership of the closed set.
func IsProductClass(value string) bool { return contains(ProductClasses, value) }

// PackageLevels is the packaging vocabulary. Levels do not imply ratios:
// 1 carton = 24 packs is configured in shop_package_relations, never inferred
// from the fact that a carton sounds bigger than a pack.
var PackageLevels = []string{
	"EACH", "SACHET", "PACK", "ROLL", "BUNDLE", "TRAY", "BAG", "SACK", "CARTON", "CASE",
	"CRATE", "DRUM", "PALLET", "OTHER_CONTROLLED",
}

// IsPackageLevel reports membership of the closed set.
func IsPackageLevel(value string) bool { return contains(PackageLevels, value) }

// IdentifierTypes is the trade-identifier vocabulary. Storing a GTIN is
// storage; it is not a claim that KarryGo verified it with GS1 or anyone else.
var IdentifierTypes = []string{
	"INTERNAL_SKU", "GTIN", "EAN", "UPC", "BARCODE", "MANUFACTURER_CODE", "OTHER_CONTROLLED",
}

// IsIdentifierType reports membership of the closed set.
func IsIdentifierType(value string) bool { return contains(IdentifierTypes, value) }

// identifierLengths bounds the numeric trade identifiers to their real
// standards, so a mistyped EAN is refused at entry rather than discovered at a
// till.
var identifierLengths = map[string][]int{
	"GTIN": {8, 12, 13, 14},
	"EAN":  {8, 13},
	"UPC":  {12},
}

// Conversion semantics. The distinction is the section's central safety
// property: an approximate calibration must never be consumed as exact.
const (
	ConversionExact       = "EXACT"
	ConversionCalibrated  = "CALIBRATED_APPROXIMATE"
	ConversionNone        = "NONE"
	CalibrationVendor     = "VENDOR_MEASURED"
	CalibrationDeclared   = "MANUFACTURER_DECLARED"
	CalibrationPackage    = "PACKAGE_DECLARED"
	CalibrationVerified   = "PLATFORM_VERIFIED"
	CalibrationControlled = "OTHER_CONTROLLED"
)

// ConversionTypes is the closed set.
var ConversionTypes = []string{ConversionExact, ConversionCalibrated, ConversionNone}

// CalibrationSources is the closed set.
var CalibrationSources = []string{
	CalibrationVendor, CalibrationDeclared, CalibrationPackage, CalibrationVerified, CalibrationControlled,
}

// Measurement modes describe how a listing's quantity is established.
const (
	ModeFixed          = "FIXED"
	ModeVariableWeight = "VARIABLE_WEIGHT"
	ModeVariableCount  = "VARIABLE_COUNT"
	ModeLocalContainer = "LOCAL_CONTAINER"
	ModePackageCount   = "PACKAGE_COUNT"
	ModePricePortion   = "PRICE_PORTION"
)

// MeasurementModes is the closed set.
var MeasurementModes = []string{
	ModeFixed, ModeVariableWeight, ModeVariableCount, ModeLocalContainer, ModePackageCount, ModePricePortion,
}

// TemperatureZones and HandlingFlags are Vendor's published handling truth.
// Vendor states what the goods need; a logistics authority decides what to do
// about it. Nothing here assigns a route, a vehicle or a provider.
var TemperatureZones = []string{"AMBIENT", "FRESH", "CHILLED", "FROZEN"}

// HandlingFlags is the closed handling vocabulary.
var HandlingFlags = []string{
	"FRAGILE", "UPRIGHT_REQUIRED", "CRUSH_SENSITIVE", "LEAK_RISK", "LIQUID",
	"ODOR_SENSITIVE", "RAW_PROTEIN", "KEEP_DRY", "KEEP_COLD", "STACK_RESTRICTED",
}

// ConsolidationClasses groups goods that may travel together.
var ConsolidationClasses = []string{
	"GENERAL", "DRY_GOODS", "FRESH_PRODUCE", "RAW_PROTEIN", "FROZEN", "CHILLED", "LIQUID", "FRAGILE", "BULK_LOAD",
}

// Fresh-attribute types and their bounded option vocabularies.
const (
	AttrRipeness   = "RIPENESS"
	AttrQuality    = "QUALITY_GRADE"
	AttrSize       = "SIZE_GRADE"
	AttrColour     = "COLOUR"
	AttrFirmness   = "FIRMNESS"
	AttrCut        = "CUT"
	AttrCleaning   = "CLEANING"
	AttrPeeling    = "PEELING"
	AttrTrimming   = "TRIMMING"
	AttrGutting    = "GUTTING"
	AttrDescaling  = "DESCALING"
	AttrHead       = "HEAD"
	AttrBone       = "BONE"
	AttrPreserving = "PRESERVATION"
	AttrSelection  = "SELECTION"
	AttrPackaging  = "PACKAGING_PREFERENCE"
)

// FreshAttributeOptions bounds each attribute's legal values. A free-text
// option would make "ripe" unqueryable and untranslatable.
var FreshAttributeOptions = map[string][]string{
	AttrRipeness:   {"GREEN", "TURNING", "RIPE", "VERY_RIPE"},
	AttrQuality:    {"STANDARD", "SELECTED", "PREMIUM", "PROCESSING_GRADE"},
	AttrSize:       {"SMALL", "MEDIUM", "LARGE"},
	AttrColour:     {"RED", "GREEN", "YELLOW", "WHITE", "DARK", "MIXED"},
	AttrFirmness:   {"SOFT", "FIRM", "VERY_FIRM"},
	AttrCut:        {"WHOLE", "CUT", "SLICED", "DICED", "CHOPPED", "PORTIONED"},
	AttrCleaning:   {"CLEANED", "UNCLEANED", "WASHED", "UNWASHED"},
	AttrPeeling:    {"PEELED", "UNPEELED"},
	AttrTrimming:   {"TRIMMED", "UNTRIMMED"},
	AttrGutting:    {"GUTTED", "UNGUTTED"},
	AttrDescaling:  {"DESCALED", "NOT_DESCALED"},
	AttrHead:       {"HEAD_ON", "HEAD_OFF"},
	AttrBone:       {"BONE_IN", "BONELESS"},
	AttrPreserving: {"FRESH", "FROZEN", "SMOKED", "DRIED"},
	AttrSelection:  {"SELECTED", "UNSELECTED"},
	AttrPackaging:  {"BAGGED", "WRAPPED", "BOXED", "NONE"},
}

// classAttributes decides which attribute types a product class may carry.
//
// This is the "invalid attribute does not attach to an inappropriate product"
// rule made concrete: descaling belongs to fish, bone to meat, ripeness to
// produce. Rice has a quality grade and a cleaning preference; it has neither
// a head nor a ripeness.
var classAttributes = map[string][]string{
	"FRESH_PRODUCE": {AttrRipeness, AttrQuality, AttrSize, AttrColour, AttrFirmness, AttrCut,
		AttrCleaning, AttrPeeling, AttrTrimming, AttrSelection, AttrPackaging},
	"MEAT": {AttrQuality, AttrSize, AttrCut, AttrCleaning, AttrBone, AttrPreserving,
		AttrSelection, AttrPackaging},
	"FISH": {AttrQuality, AttrSize, AttrCut, AttrCleaning, AttrGutting, AttrDescaling,
		AttrHead, AttrPreserving, AttrSelection, AttrPackaging},
	"FROZEN":            {AttrQuality, AttrSize, AttrCut, AttrPreserving, AttrPackaging},
	"CHILLED":           {AttrQuality, AttrSize, AttrCut, AttrPreserving, AttrPackaging},
	"DRY_BULK":          {AttrQuality, AttrSize, AttrCleaning, AttrSelection, AttrPackaging},
	"LOCAL_FOODSTUFF":   {AttrQuality, AttrSize, AttrCleaning, AttrSelection, AttrPackaging},
	"DAIRY":             {AttrQuality, AttrPackaging},
	"BEVERAGE":          {AttrQuality, AttrPackaging},
	"PACKAGED":          {AttrQuality, AttrPackaging},
	"BAKED_PACKAGED":    {AttrQuality, AttrPackaging},
	"HOUSEHOLD_GROCERY": {AttrQuality, AttrPackaging},
	"OTHER":             {AttrQuality, AttrPackaging},
}

// AttributeTypesForClass lists the attribute types a product class admits.
func AttributeTypesForClass(productClass string) []string { return classAttributes[productClass] }

// AttributeAllowedForClass reports whether an attribute type may be attached
// to a product of this class.
func AttributeAllowedForClass(productClass, attributeType string) bool {
	return contains(classAttributes[productClass], attributeType)
}

// Price-history subjects.
const (
	SubjectVariantRetail      = "VARIANT_RETAIL"
	SubjectVariantWholesale   = "VARIANT_WHOLESALE"
	SubjectSellUnitRetail     = "SELL_UNIT_RETAIL"
	SubjectSellUnitWholesale  = "SELL_UNIT_WHOLESALE"
	SubjectListingRetail      = "LISTING_RETAIL_OVERRIDE"
	SubjectListingWholesale   = "LISTING_WHOLESALE_OVERRIDE"
	SubjectListingUnitRetail  = "LISTING_SELL_UNIT_RETAIL"
	SubjectListingUnitWholesa = "LISTING_SELL_UNIT_WHOLESALE"
	SubjectQuantityBreak      = "QUANTITY_BREAK"
)

// Price-change reasons.
const (
	ReasonBaseline        = "BASELINE_IMPORT"
	ReasonCreated         = "CREATED"
	ReasonPriceChanged    = "PRICE_CHANGED"
	ReasonOverrideSet     = "OVERRIDE_SET"
	ReasonOverrideCleared = "OVERRIDE_CLEARED"
	ReasonArchived        = "ARCHIVED"
)

// Bounds. Every collection in this section is finite by construction.
const (
	MaxCategoryDepth       = 5
	MaxProductMedia        = 12
	MaxSellUnitsPerVariant = 24
	MaxQuantityBreaks      = 20
	MaxFreshOptions        = 24
	MaxAliasesPerMeasure   = 12
	MaxHandlingFlags       = 10
	MaxOperatingModels     = 5
	MaxPageSize            = 100
	MaxMoneyMinor          = 99999999999
	MaxFractionNumerator   = 1000000
	MaxFractionDenominator = 1000
)

// ---------------------------------------------------------------------------
// Domain models
// ---------------------------------------------------------------------------

// MeasureDefinition is one trade measure. BusinessID empty means a
// platform-seeded definition visible to every tenant.
//
// BaseQuantity is the exact amount of the family's canonical base that one of
// this measure equals, and is nil whenever no universal truth exists. A Paint,
// a Derica, a Mudu and a Bag all have nil here, permanently: their content
// depends on the commodity and the container, so a global constant would be
// wrong for every product that used it.
type MeasureDefinition struct {
	ID           string
	BusinessID   string
	OwnerKey     string
	Code         string
	DisplayName  string
	Family       string
	Origin       string
	RegionCode   string
	BaseQuantity *int64
	Status       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	ArchivedAt   *time.Time
	Version      int64
}

// MeasureAlias is a presentation name for a measure. It is never calculation
// identity: only the canonical code is.
type MeasureAlias struct {
	ID         string
	MeasureID  string
	OwnerKey   string
	BusinessID string
	Alias      string
	RegionCode string
	CreatedAt  time.Time
}

// BranchOperatingModel is one branch's Shop operating configuration.
type BranchOperatingModel struct {
	ID           string
	BusinessID   string
	BranchID     string
	PrimaryModel string
	Models       []BranchOperatingMode
	Status       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	ArchivedAt   *time.Time
	Version      int64
}

// BranchOperatingMode is one enabled model and the capability that admitted it.
type BranchOperatingMode struct {
	Model              string
	AdmittedCapability string
}

// Category is one node of the Shop taxonomy.
type Category struct {
	ID          string
	BusinessID  string
	ParentID    string
	Name        string
	Slug        string
	Description string
	Position    int
	Depth       int
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ArchivedAt  *time.Time
	Version     int64
}

// Product is the Shop product master. Brand, manufacturer and origin are
// optional because a market trader's tomatoes have none of them.
type Product struct {
	ID                  string
	BusinessID          string
	CategoryID          string
	Name                string
	Description         string
	Brand               string
	Manufacturer        string
	OriginCountry       string
	ProductClass        string
	RegulatoryReference string
	ManufacturerCode    string
	OperatingModels     []string
	MediaRefs           []string
	Status              string
	CreatedAt           time.Time
	UpdatedAt           time.Time
	ArchivedAt          *time.Time
	Version             int64
}

// Variant is one sellable SKU shape of a product.
type Variant struct {
	ID                  string
	BusinessID          string
	ProductID           string
	SKU                 string
	Name                string
	PackageLevel        string
	PackQuantity        int
	NetWeightGrams      *int64
	NetVolumeML         *int64
	LengthMM            *int
	WidthMM             *int
	HeightMM            *int
	RetailPriceMinor    *int64
	WholesalePriceMinor *int64
	TemperatureZone     string
	ConsolidationClass  string
	SeparationGroup     string
	Stackable           bool
	LargeLoad           bool
	HandlingFlags       []string
	Status              string
	Position            int
	CreatedAt           time.Time
	UpdatedAt           time.Time
	ArchivedAt          *time.Time
	Version             int64
}

// ProductIdentifier is one trade identifier for one variant.
type ProductIdentifier struct {
	ID           string
	BusinessID   string
	ProductID    string
	VariantID    string
	Type         string
	Value        string
	PackageLevel string
	Status       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	ArchivedAt   *time.Time
	Version      int64
}

// PackageRelation is one configured "parent contains N children" fact.
type PackageRelation struct {
	ID             string
	BusinessID     string
	ProductID      string
	ParentVariant  string
	ChildVariant   string
	UnitsPerParent int
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ArchivedAt     *time.Time
	Version        int64
}

// MeasureCalibration is a vendor's measured, APPROXIMATE answer to "how much
// of this product fits in this container". It is evidence, never an exact
// conversion, and carries its own tolerance and source so a reader can judge
// how much to trust it.
type MeasureCalibration struct {
	ID              string
	BusinessID      string
	BranchID        string
	ProductID       string
	VariantID       string
	MeasureID       string
	MeasureOwnerKey string
	WeightGrams     *int64
	VolumeML        *int64
	ToleranceBps    int
	Source          string
	EffectiveFrom   time.Time
	EffectiveTo     *time.Time
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ArchivedAt      *time.Time
	Version         int64
}

// SellUnit is one way a customer may buy a variant: 1 Derica, ½ Paint, a
// 50 kg bag, a ₦500 portion.
type SellUnit struct {
	ID                  string
	BusinessID          string
	ProductID           string
	VariantID           string
	MeasureID           string
	MeasureOwnerKey     string
	FractionNumerator   int
	FractionDenominator int
	DisplayName         string
	MeasurementMode     string
	ConversionType      string
	ExactBaseFamily     string
	ExactBaseQuantity   *int64
	CalibrationID       string
	EstimatedBaseQty    *int64
	UnderfillBps        int
	OverfillBps         int
	MeasurementRequired bool
	DisplayPrecision    int
	PriceMinor          *int64
	WholesalePriceMinor *int64
	RetailEnabled       bool
	WholesaleEnabled    bool
	MinimumOrderQty     int64
	OrderMultiple       int64
	MaximumOrderQty     *int64
	Status              string
	Position            int
	CreatedAt           time.Time
	UpdatedAt           time.Time
	ArchivedAt          *time.Time
	Version             int64
}

// BranchListing is one branch's commercial decision about one variant.
// It carries no stock quantity: Part B owns availability.
type BranchListing struct {
	ID                   string
	BusinessID           string
	BranchID             string
	ProductID            string
	VariantID            string
	OperatingModel       string
	RetailEnabled        bool
	WholesaleEnabled     bool
	RetailOverrideMinor  *int64
	WholesaleOverrideMin *int64
	MinimumOrderQty      int64
	OrderMultiple        int64
	MaximumOrderQty      *int64
	PreorderPolicy       string
	BackorderPolicy      string
	AutoHideUnavailable  bool
	LogisticsAvailable   bool
	Status               string
	CreatedAt            time.Time
	UpdatedAt            time.Time
	ArchivedAt           *time.Time
	Version              int64
}

// ListingSellUnit is one sell unit enabled for one branch listing, with its
// optional branch-level price overrides.
type ListingSellUnit struct {
	ID                   string
	BusinessID           string
	ListingID            string
	SellUnitID           string
	VariantID            string
	CalibrationID        string
	Enabled              bool
	PriceOverrideMinor   *int64
	WholesaleOverrideMin *int64
	Position             int
	CreatedAt            time.Time
	UpdatedAt            time.Time
	Version              int64
}

// QuantityBreak is one wholesale price tier.
type QuantityBreak struct {
	ID             string
	BusinessID     string
	ListingID      string
	SellUnitID     string
	MinQuantity    int64
	MaxQuantity    *int64
	UnitPriceMinor int64
	WholesaleOnly  bool
	LeadTimeDays   *int
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ArchivedAt     *time.Time
	Version        int64
}

// FreshAttribute is one configurable fresh-goods choice on a product.
type FreshAttribute struct {
	ID         string
	BusinessID string
	ProductID  string
	Type       string
	Required   bool
	Position   int
	Options    []FreshAttributeOption
	Status     string
	CreatedAt  time.Time
	UpdatedAt  time.Time
	ArchivedAt *time.Time
	Version    int64
}

// FreshAttributeOption is one bounded value of a fresh attribute.
type FreshAttributeOption struct {
	ID          string
	BusinessID  string
	AttributeID string
	Value       string
	DisplayName string
	IsDefault   bool
	Position    int
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Version     int64
}

// PriceVersion is one append-only commercial price record.
type PriceVersion struct {
	ID                string
	BusinessID        string
	BranchID          string
	SubjectType       string
	SubjectRef        string
	ProductRef        string
	VariantRef        string
	Currency          string
	PreviousAmount    *int64
	Amount            *int64
	PreviousInherited bool
	Inherited         bool
	ChangeReason      string
	SubjectVersion    int64
	Revision          int64
	ActorType         string
	ActorID           string
	CorrelationID     string
	CommandRef        string
	RecordedAt        time.Time
	EffectiveAt       time.Time
}

// ---------------------------------------------------------------------------
// Validation helpers
// ---------------------------------------------------------------------------

var (
	slugPattern        = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,79}$`)
	measureCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,39}$`)
	identifierPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,63}$`)
	skuPattern         = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,63}$`)
	regionPattern      = regexp.MustCompile(`^[A-Z]{2}(-[A-Z0-9]{1,10})?$`)
	countryPattern     = regexp.MustCompile(`^[A-Z]{2}$`)
	optionPattern      = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,39}$`)
	nonSlugRunes       = regexp.MustCompile(`[^a-z0-9]+`)
)

func contains(set []string, value string) bool {
	for _, entry := range set {
		if entry == value {
			return true
		}
	}
	return false
}

func validateName(field, raw string, minimum, maximum int) (string, *apperrors.FieldViolation) {
	value := strings.TrimSpace(raw)
	if length := len([]rune(value)); length < minimum || length > maximum {
		return "", &apperrors.FieldViolation{Field: field,
			Message: "Provide between " + itoa(minimum) + " and " + itoa(maximum) + " characters."}
	}
	return value, nil
}

func validateOptionalText(field, raw string, maximum int) (string, *apperrors.FieldViolation) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	if len([]rune(value)) > maximum {
		return "", &apperrors.FieldViolation{Field: field,
			Message: "Use at most " + itoa(maximum) + " characters."}
	}
	return value, nil
}

// slugify derives a stable url-safe key from a name when the caller supplies
// none. It never invents uniqueness; the database decides that.
func slugify(value string) string {
	lowered := strings.ToLower(strings.TrimSpace(value))
	cleaned := strings.Trim(nonSlugRunes.ReplaceAllString(lowered, "-"), "-")
	if len(cleaned) > 80 {
		cleaned = strings.Trim(cleaned[:80], "-")
	}
	return cleaned
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var digits []byte
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	if negative {
		return "-" + string(digits)
	}
	return string(digits)
}

// gcd reduces a fraction so "2/4 Paint" and "1/2 Paint" are the same sell unit
// rather than two rows that quietly disagree.
func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}
