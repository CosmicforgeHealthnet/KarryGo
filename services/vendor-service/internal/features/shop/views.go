package shop

import "time"

// ---------------------------------------------------------------------------
// Response views
// ---------------------------------------------------------------------------
//
// One shape per entity, used by the HTTP layer, the audit record and the
// outbox payload alike, so what a client sees, what an auditor reads and what
// a consumer receives can never drift apart.
//
// Money is always an integer minor amount in the business's own currency, and
// the currency is never restated on a row: Section 6 owns it.

// CategoryView is one taxonomy node.
type CategoryView struct {
	ID          string     `json:"id"`
	ParentRef   *string    `json:"parent_ref"`
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	Description string     `json:"description,omitempty"`
	Position    int        `json:"position"`
	Depth       int        `json:"depth"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ArchivedAt  *time.Time `json:"archived_at,omitempty"`
	Version     int64      `json:"version"`
}

// CategoryViewOf renders a category.
func CategoryViewOf(c Category) CategoryView {
	return CategoryView{ID: c.ID, ParentRef: stringPtrOrNil(c.ParentID), Name: c.Name, Slug: c.Slug,
		Description: c.Description, Position: c.Position, Depth: c.Depth, Status: c.Status,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, ArchivedAt: c.ArchivedAt, Version: c.Version}
}

// MeasureView is one trade measure.
//
// base_quantity is null for every container, package, grouped-count and load
// measure, and that null is a deliberate statement: no universal conversion
// exists, so the vendor's own calibration is the only honest answer.
type MeasureView struct {
	ID            string     `json:"id"`
	Code          string     `json:"code"`
	DisplayName   string     `json:"display_name"`
	Family        string     `json:"measurement_family"`
	Origin        string     `json:"measure_origin"`
	Scope         string     `json:"scope"`
	RegionCode    string     `json:"region_code,omitempty"`
	BaseUnit      string     `json:"base_unit,omitempty"`
	BaseQuantity  *int64     `json:"base_quantity"`
	ConvertsExact bool       `json:"converts_exactly"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	ArchivedAt    *time.Time `json:"archived_at,omitempty"`
	Version       int64      `json:"version"`
}

// baseUnitNames name the canonical integer base of each convertible family.
var baseUnitNames = map[string]string{
	FamilyMass:   "GRAM",
	FamilyVolume: "MILLILITRE",
	FamilyCount:  "UNIT",
}

// MeasureViewOf renders a measure definition.
func MeasureViewOf(m MeasureDefinition) MeasureView {
	scope := "PLATFORM"
	if m.BusinessID != "" {
		scope = "BUSINESS"
	}
	return MeasureView{ID: m.ID, Code: m.Code, DisplayName: m.DisplayName, Family: m.Family,
		Origin: m.Origin, Scope: scope, RegionCode: m.RegionCode, BaseUnit: baseUnitNames[m.Family],
		BaseQuantity: m.BaseQuantity, ConvertsExact: m.BaseQuantity != nil, Status: m.Status,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt, ArchivedAt: m.ArchivedAt, Version: m.Version}
}

// AliasView is one presentation name for a measure.
type AliasView struct {
	ID         string    `json:"id"`
	MeasureRef string    `json:"measure_ref"`
	Alias      string    `json:"alias"`
	Scope      string    `json:"scope"`
	RegionCode string    `json:"region_code,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// AliasViewOf renders a measure alias.
func AliasViewOf(a MeasureAlias) AliasView {
	scope := "PLATFORM"
	if a.BusinessID != "" {
		scope = "BUSINESS"
	}
	return AliasView{ID: a.ID, MeasureRef: a.MeasureID, Alias: a.Alias, Scope: scope,
		RegionCode: a.RegionCode, CreatedAt: a.CreatedAt}
}

// CalibrationView is one approximate container measurement.
type CalibrationView struct {
	ID            string     `json:"id"`
	BranchRef     *string    `json:"branch_ref"`
	ProductRef    string     `json:"product_ref"`
	VariantRef    *string    `json:"variant_ref"`
	MeasureRef    string     `json:"measure_ref"`
	WeightGrams   *int64     `json:"approximate_weight_grams"`
	VolumeML      *int64     `json:"approximate_volume_ml"`
	ToleranceBps  int        `json:"tolerance_bps"`
	Source        string     `json:"calibration_source"`
	Approximate   bool       `json:"approximate"`
	EffectiveFrom time.Time  `json:"effective_from"`
	EffectiveTo   *time.Time `json:"effective_to,omitempty"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	Version       int64      `json:"version"`
}

// CalibrationViewOf renders a calibration. Approximate is always true: a
// calibration is measured evidence about one vendor's container, never an
// exact conversion, and no reader should have to infer that.
func CalibrationViewOf(c MeasureCalibration) CalibrationView {
	return CalibrationView{ID: c.ID, BranchRef: stringPtrOrNil(c.BranchID), ProductRef: c.ProductID,
		VariantRef: stringPtrOrNil(c.VariantID), MeasureRef: c.MeasureID, WeightGrams: c.WeightGrams,
		VolumeML: c.VolumeML, ToleranceBps: c.ToleranceBps, Source: c.Source, Approximate: true,
		EffectiveFrom: c.EffectiveFrom, EffectiveTo: c.EffectiveTo, Status: c.Status,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, Version: c.Version}
}

// OperatingModelView is a branch's Shop operating configuration.
type OperatingModelView struct {
	ID           string              `json:"id"`
	BranchRef    string              `json:"branch_ref"`
	PrimaryModel *string             `json:"primary_model"`
	Models       []OperatingModeView `json:"models"`
	Status       string              `json:"status"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
	Version      int64               `json:"version"`
}

// OperatingModeView is one enabled model and the capability that admitted it.
type OperatingModeView struct {
	Model              string `json:"operating_model"`
	AdmittedCapability string `json:"admitted_capability"`
}

// OperatingModelViewOf renders a branch operating configuration.
func OperatingModelViewOf(c BranchOperatingModel) OperatingModelView {
	modes := make([]OperatingModeView, 0, len(c.Models))
	for _, mode := range c.Models {
		modes = append(modes, OperatingModeView{Model: mode.Model, AdmittedCapability: mode.AdmittedCapability})
	}
	return OperatingModelView{ID: c.ID, BranchRef: c.BranchID, PrimaryModel: stringPtrOrNil(c.PrimaryModel),
		Models: modes, Status: c.Status, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, Version: c.Version}
}

// ProductView is one Shop product.
type ProductView struct {
	ID                  string     `json:"id"`
	CategoryRef         string     `json:"category_ref"`
	Name                string     `json:"name"`
	Description         string     `json:"description,omitempty"`
	Brand               *string    `json:"brand"`
	Manufacturer        *string    `json:"manufacturer"`
	OriginCountry       *string    `json:"origin_country"`
	ProductClass        string     `json:"product_class"`
	RegulatoryReference *string    `json:"regulatory_reference"`
	ManufacturerCode    *string    `json:"manufacturer_code"`
	OperatingModels     []string   `json:"operating_models"`
	MediaRefs           []string   `json:"media_refs"`
	Status              string     `json:"status"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	ArchivedAt          *time.Time `json:"archived_at,omitempty"`
	Version             int64      `json:"version"`
}

// ProductViewOf renders a product.
func ProductViewOf(p Product) ProductView {
	models := p.OperatingModels
	if models == nil {
		models = []string{}
	}
	media := p.MediaRefs
	if media == nil {
		media = []string{}
	}
	return ProductView{ID: p.ID, CategoryRef: p.CategoryID, Name: p.Name, Description: p.Description,
		Brand: stringPtrOrNil(p.Brand), Manufacturer: stringPtrOrNil(p.Manufacturer),
		OriginCountry: stringPtrOrNil(p.OriginCountry), ProductClass: p.ProductClass,
		RegulatoryReference: stringPtrOrNil(p.RegulatoryReference),
		ManufacturerCode:    stringPtrOrNil(p.ManufacturerCode), OperatingModels: models, MediaRefs: media,
		Status: p.Status, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, ArchivedAt: p.ArchivedAt,
		Version: p.Version}
}

// VariantView is one SKU, including the handling truth Vendor publishes for a
// future logistics authority to consume.
type VariantView struct {
	ID                  string     `json:"id"`
	ProductRef          string     `json:"product_ref"`
	SKU                 *string    `json:"sku"`
	Name                string     `json:"name"`
	PackageLevel        string     `json:"package_level"`
	PackQuantity        int        `json:"pack_quantity"`
	NetWeightGrams      *int64     `json:"net_weight_grams"`
	NetVolumeML         *int64     `json:"net_volume_ml"`
	LengthMM            *int       `json:"length_mm"`
	WidthMM             *int       `json:"width_mm"`
	HeightMM            *int       `json:"height_mm"`
	RetailPriceMinor    *int64     `json:"retail_price_minor"`
	WholesalePriceMinor *int64     `json:"wholesale_price_minor"`
	TemperatureZone     string     `json:"temperature_zone"`
	ConsolidationClass  *string    `json:"consolidation_class"`
	SeparationGroup     *string    `json:"separation_group"`
	Stackable           bool       `json:"stackable"`
	LargeLoad           bool       `json:"large_load"`
	HandlingFlags       []string   `json:"handling_flags"`
	Status              string     `json:"status"`
	Position            int        `json:"position"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	ArchivedAt          *time.Time `json:"archived_at,omitempty"`
	Version             int64      `json:"version"`
}

// VariantViewOf renders a variant.
func VariantViewOf(v Variant) VariantView {
	flags := v.HandlingFlags
	if flags == nil {
		flags = []string{}
	}
	return VariantView{ID: v.ID, ProductRef: v.ProductID, SKU: stringPtrOrNil(v.SKU), Name: v.Name,
		PackageLevel: v.PackageLevel, PackQuantity: v.PackQuantity, NetWeightGrams: v.NetWeightGrams,
		NetVolumeML: v.NetVolumeML, LengthMM: v.LengthMM, WidthMM: v.WidthMM, HeightMM: v.HeightMM,
		RetailPriceMinor: v.RetailPriceMinor, WholesalePriceMinor: v.WholesalePriceMinor,
		TemperatureZone: v.TemperatureZone, ConsolidationClass: stringPtrOrNil(v.ConsolidationClass),
		SeparationGroup: stringPtrOrNil(v.SeparationGroup), Stackable: v.Stackable, LargeLoad: v.LargeLoad,
		HandlingFlags: flags, Status: v.Status, Position: v.Position, CreatedAt: v.CreatedAt,
		UpdatedAt: v.UpdatedAt, ArchivedAt: v.ArchivedAt, Version: v.Version}
}

// IdentifierView is one trade identifier.
type IdentifierView struct {
	ID           string  `json:"id"`
	ProductRef   string  `json:"product_ref"`
	VariantRef   string  `json:"variant_ref"`
	Type         string  `json:"identifier_type"`
	Value        string  `json:"identifier_value"`
	PackageLevel *string `json:"package_level"`
	// ExternallyVerified is always false: KarryGo stores trade identifiers, it
	// does not certify them with GS1, NAFDAC or anyone else.
	ExternallyVerified bool      `json:"externally_verified"`
	Status             string    `json:"status"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	Version            int64     `json:"version"`
}

// IdentifierViewOf renders a trade identifier.
func IdentifierViewOf(i ProductIdentifier) IdentifierView {
	return IdentifierView{ID: i.ID, ProductRef: i.ProductID, VariantRef: i.VariantID, Type: i.Type,
		Value: i.Value, PackageLevel: stringPtrOrNil(i.PackageLevel), ExternallyVerified: false,
		Status: i.Status, CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt, Version: i.Version}
}

// PackageRelationView is one configured containment fact.
type PackageRelationView struct {
	ID             string    `json:"id"`
	ProductRef     string    `json:"product_ref"`
	ParentVariant  string    `json:"parent_variant_ref"`
	ChildVariant   string    `json:"child_variant_ref"`
	UnitsPerParent int       `json:"units_per_parent"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Version        int64     `json:"version"`
}

// PackageRelationViewOf renders a packaging relationship.
func PackageRelationViewOf(p PackageRelation) PackageRelationView {
	return PackageRelationView{ID: p.ID, ProductRef: p.ProductID, ParentVariant: p.ParentVariant,
		ChildVariant: p.ChildVariant, UnitsPerParent: p.UnitsPerParent, Status: p.Status,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, Version: p.Version}
}

// SellUnitView is one way to buy a variant.
type SellUnitView struct {
	ID                  string     `json:"id"`
	ProductRef          string     `json:"product_ref"`
	VariantRef          string     `json:"variant_ref"`
	MeasureRef          string     `json:"measure_ref"`
	FractionNumerator   int        `json:"fraction_numerator"`
	FractionDenominator int        `json:"fraction_denominator"`
	DisplayName         string     `json:"display_name"`
	MeasurementMode     string     `json:"measurement_mode"`
	ConversionType      string     `json:"conversion_type"`
	ExactBaseFamily     *string    `json:"exact_base_family"`
	ExactBaseQuantity   *int64     `json:"exact_base_quantity"`
	CalibrationRef      *string    `json:"calibration_ref"`
	EstimatedBaseQty    *int64     `json:"estimated_base_quantity"`
	UnderfillBps        int        `json:"underfill_tolerance_bps"`
	OverfillBps         int        `json:"overfill_tolerance_bps"`
	MeasurementRequired bool       `json:"actual_measurement_required"`
	DisplayPrecision    int        `json:"display_precision"`
	PriceMinor          *int64     `json:"price_minor"`
	WholesalePriceMinor *int64     `json:"wholesale_price_minor"`
	RetailEnabled       bool       `json:"retail_enabled"`
	WholesaleEnabled    bool       `json:"wholesale_enabled"`
	MinimumOrderQty     int64      `json:"minimum_order_quantity"`
	OrderMultiple       int64      `json:"order_multiple"`
	MaximumOrderQty     *int64     `json:"maximum_order_quantity"`
	Status              string     `json:"status"`
	Position            int        `json:"position"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	ArchivedAt          *time.Time `json:"archived_at,omitempty"`
	Version             int64      `json:"version"`
}

// SellUnitViewOf renders a sell unit.
func SellUnitViewOf(u SellUnit) SellUnitView {
	return SellUnitView{ID: u.ID, ProductRef: u.ProductID, VariantRef: u.VariantID, MeasureRef: u.MeasureID,
		FractionNumerator: u.FractionNumerator, FractionDenominator: u.FractionDenominator,
		DisplayName: u.DisplayName, MeasurementMode: u.MeasurementMode, ConversionType: u.ConversionType,
		ExactBaseFamily: stringPtrOrNil(u.ExactBaseFamily), ExactBaseQuantity: u.ExactBaseQuantity,
		CalibrationRef: stringPtrOrNil(u.CalibrationID), EstimatedBaseQty: u.EstimatedBaseQty,
		UnderfillBps: u.UnderfillBps, OverfillBps: u.OverfillBps,
		MeasurementRequired: u.MeasurementRequired, DisplayPrecision: u.DisplayPrecision,
		PriceMinor: u.PriceMinor, WholesalePriceMinor: u.WholesalePriceMinor,
		RetailEnabled: u.RetailEnabled, WholesaleEnabled: u.WholesaleEnabled,
		MinimumOrderQty: u.MinimumOrderQty, OrderMultiple: u.OrderMultiple, MaximumOrderQty: u.MaximumOrderQty,
		Status: u.Status, Position: u.Position, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
		ArchivedAt: u.ArchivedAt, Version: u.Version}
}

// ListingView is one branch's commercial decision about one SKU.
type ListingView struct {
	ID                  string  `json:"id"`
	BranchRef           string  `json:"branch_ref"`
	ProductRef          string  `json:"product_ref"`
	VariantRef          string  `json:"variant_ref"`
	OperatingModel      *string `json:"operating_model"`
	RetailEnabled       bool    `json:"retail_enabled"`
	WholesaleEnabled    bool    `json:"wholesale_enabled"`
	RetailOverride      *int64  `json:"retail_price_override_minor"`
	WholesaleOverride   *int64  `json:"wholesale_price_override_minor"`
	MinimumOrderQty     int64   `json:"minimum_order_quantity"`
	OrderMultiple       int64   `json:"order_multiple"`
	MaximumOrderQty     *int64  `json:"maximum_order_quantity"`
	PreorderPolicy      string  `json:"preorder_policy"`
	BackorderPolicy     string  `json:"backorder_policy"`
	AutoHideUnavailable bool    `json:"auto_hide_when_unavailable"`
	LogisticsAvailable  bool    `json:"logistics_available"`
	// InventoryAuthority names where availability actually lives. It is
	// reported honestly as unavailable rather than as a fabricated zero,
	// because Section 12B has not been implemented.
	InventoryAuthority string     `json:"inventory_authority"`
	Status             string     `json:"status"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	ArchivedAt         *time.Time `json:"archived_at,omitempty"`
	Version            int64      `json:"version"`
}

// ListingViewOf renders a branch listing.
func ListingViewOf(l BranchListing) ListingView {
	return ListingView{ID: l.ID, BranchRef: l.BranchID, ProductRef: l.ProductID, VariantRef: l.VariantID,
		OperatingModel: stringPtrOrNil(l.OperatingModel), RetailEnabled: l.RetailEnabled,
		WholesaleEnabled: l.WholesaleEnabled, RetailOverride: l.RetailOverrideMinor,
		WholesaleOverride: l.WholesaleOverrideMin, MinimumOrderQty: l.MinimumOrderQty,
		OrderMultiple: l.OrderMultiple, MaximumOrderQty: l.MaximumOrderQty,
		PreorderPolicy: l.PreorderPolicy, BackorderPolicy: l.BackorderPolicy,
		AutoHideUnavailable: l.AutoHideUnavailable, LogisticsAvailable: l.LogisticsAvailable,
		InventoryAuthority: "NOT_IMPLEMENTED", Status: l.Status, CreatedAt: l.CreatedAt,
		UpdatedAt: l.UpdatedAt, ArchivedAt: l.ArchivedAt, Version: l.Version}
}

// ListingSellUnitView is one sell unit offered by one listing.
type ListingSellUnitView struct {
	ID                string    `json:"id"`
	ListingRef        string    `json:"listing_ref"`
	SellUnitRef       string    `json:"sell_unit_ref"`
	CalibrationRef    *string   `json:"calibration_ref"`
	Enabled           bool      `json:"enabled"`
	PriceOverride     *int64    `json:"price_override_minor"`
	WholesaleOverride *int64    `json:"wholesale_price_override_minor"`
	Position          int       `json:"position"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	Version           int64     `json:"version"`
}

// ListingSellUnitViewOf renders a listing sell unit.
func ListingSellUnitViewOf(u ListingSellUnit) ListingSellUnitView {
	return ListingSellUnitView{ID: u.ID, ListingRef: u.ListingID, SellUnitRef: u.SellUnitID,
		CalibrationRef: stringPtrOrNil(u.CalibrationID),
		Enabled:        u.Enabled, PriceOverride: u.PriceOverrideMinor, WholesaleOverride: u.WholesaleOverrideMin,
		Position: u.Position, CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt, Version: u.Version}
}

// QuantityBreakView is one wholesale tier.
type QuantityBreakView struct {
	ID             string     `json:"id"`
	ListingRef     string     `json:"listing_ref"`
	SellUnitRef    *string    `json:"sell_unit_ref"`
	MinQuantity    int64      `json:"min_quantity"`
	MaxQuantity    *int64     `json:"max_quantity"`
	UnitPriceMinor int64      `json:"unit_price_minor"`
	WholesaleOnly  bool       `json:"wholesale_only"`
	LeadTimeDays   *int       `json:"lead_time_days"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	ArchivedAt     *time.Time `json:"archived_at,omitempty"`
	Version        int64      `json:"version"`
}

// QuantityBreakViewOf renders a quantity break.
func QuantityBreakViewOf(b QuantityBreak) QuantityBreakView {
	return QuantityBreakView{ID: b.ID, ListingRef: b.ListingID, SellUnitRef: stringPtrOrNil(b.SellUnitID),
		MinQuantity: b.MinQuantity, MaxQuantity: b.MaxQuantity, UnitPriceMinor: b.UnitPriceMinor,
		WholesaleOnly: b.WholesaleOnly, LeadTimeDays: b.LeadTimeDays, Status: b.Status,
		CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt, ArchivedAt: b.ArchivedAt, Version: b.Version}
}

// FreshAttributeView is one fresh-goods choice and its bounded options.
type FreshAttributeView struct {
	ID         string                     `json:"id"`
	ProductRef string                     `json:"product_ref"`
	Type       string                     `json:"attribute_type"`
	Required   bool                       `json:"selection_required"`
	Position   int                        `json:"position"`
	Options    []FreshAttributeOptionView `json:"options"`
	Status     string                     `json:"status"`
	CreatedAt  time.Time                  `json:"created_at"`
	UpdatedAt  time.Time                  `json:"updated_at"`
	Version    int64                      `json:"version"`
}

// FreshAttributeOptionView is one bounded option.
type FreshAttributeOptionView struct {
	ID          string `json:"id"`
	Value       string `json:"option_value"`
	DisplayName string `json:"display_name"`
	IsDefault   bool   `json:"is_default"`
	Position    int    `json:"position"`
	Status      string `json:"status"`
}

// FreshAttributeViewOf renders a fresh attribute.
func FreshAttributeViewOf(a FreshAttribute) FreshAttributeView {
	options := make([]FreshAttributeOptionView, 0, len(a.Options))
	for _, option := range a.Options {
		options = append(options, FreshAttributeOptionView{ID: option.ID, Value: option.Value,
			DisplayName: option.DisplayName, IsDefault: option.IsDefault, Position: option.Position,
			Status: option.Status})
	}
	return FreshAttributeView{ID: a.ID, ProductRef: a.ProductID, Type: a.Type, Required: a.Required,
		Position: a.Position, Options: options, Status: a.Status, CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt, Version: a.Version}
}

// PriceVersionView is one immutable price record.
type PriceVersionView struct {
	ID             string    `json:"id"`
	BranchRef      *string   `json:"branch_ref"`
	SubjectType    string    `json:"subject_type"`
	SubjectRef     string    `json:"subject_ref"`
	ProductRef     *string   `json:"product_ref"`
	VariantRef     *string   `json:"variant_ref"`
	Currency       string    `json:"currency"`
	PreviousAmount *int64    `json:"previous_amount_minor"`
	Amount         *int64    `json:"amount_minor"`
	Inherited      bool      `json:"inherited"`
	ChangeReason   string    `json:"change_reason"`
	SubjectVersion int64     `json:"subject_version"`
	Revision       int64     `json:"revision"`
	ActorType      string    `json:"actor_type"`
	CommandRef     *string   `json:"command_ref"`
	RecordedAt     time.Time `json:"recorded_at"`
	EffectiveAt    time.Time `json:"effective_at"`
}

// PriceVersionViewOf renders a price version. The actor id is deliberately not
// exposed: an audit trail carries it, a catalogue response does not need it.
func PriceVersionViewOf(v PriceVersion) PriceVersionView {
	return PriceVersionView{ID: v.ID, BranchRef: stringPtrOrNil(v.BranchID), SubjectType: v.SubjectType,
		SubjectRef: v.SubjectRef, ProductRef: stringPtrOrNil(v.ProductRef),
		VariantRef: stringPtrOrNil(v.VariantRef), Currency: v.Currency, PreviousAmount: v.PreviousAmount,
		Amount: v.Amount, Inherited: v.Inherited, ChangeReason: v.ChangeReason,
		SubjectVersion: v.SubjectVersion, Revision: v.Revision, ActorType: v.ActorType,
		CommandRef: stringPtrOrNil(v.CommandRef), RecordedAt: v.RecordedAt, EffectiveAt: v.EffectiveAt}
}

// ResolvedPriceView is the deterministic commercial answer for one request.
type ResolvedPriceView struct {
	ListingRef      string  `json:"listing_ref"`
	BranchRef       string  `json:"branch_ref"`
	VariantRef      string  `json:"variant_ref"`
	SellUnitRef     *string `json:"sell_unit_ref"`
	Currency        string  `json:"currency"`
	Quantity        int64   `json:"quantity"`
	Mode            string  `json:"mode"`
	UnitPriceMinor  *int64  `json:"unit_price_minor"`
	TotalPriceMinor *int64  `json:"total_price_minor"`
	Source          string  `json:"price_source"`
	QuantityBreak   *string `json:"quantity_break_ref"`
	MinimumOrderQty int64   `json:"minimum_order_quantity"`
	OrderMultiple   int64   `json:"order_multiple"`
	MaximumOrderQty *int64  `json:"maximum_order_quantity"`
	OrderRulesMet   bool    `json:"order_rules_satisfied"`
	Reason          string  `json:"reason,omitempty"`
	// Availability is honestly unavailable: Vendor knows what this costs, not
	// whether there is any left. Part B owns that answer.
	Availability string `json:"availability"`
}

// ResolvedPriceViewOf renders a resolved price.
func ResolvedPriceViewOf(p ResolvedPrice) ResolvedPriceView {
	return ResolvedPriceView{ListingRef: p.ListingRef, BranchRef: p.BranchRef, VariantRef: p.VariantRef,
		SellUnitRef: stringPtrOrNil(p.SellUnitRef), Currency: p.Currency, Quantity: p.Quantity,
		Mode: p.Mode, UnitPriceMinor: p.UnitPriceMinor, TotalPriceMinor: p.TotalPriceMinor,
		Source: p.Source, QuantityBreak: stringPtrOrNil(p.QuantityBreakID),
		MinimumOrderQty: p.MinimumOrderQty, OrderMultiple: p.OrderMultiple,
		MaximumOrderQty: p.MaximumOrderQty, OrderRulesMet: p.Satisfied, Reason: p.Reason,
		Availability: "UNAVAILABLE"}
}
