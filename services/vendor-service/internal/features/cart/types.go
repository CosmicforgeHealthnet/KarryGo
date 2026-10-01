package cart

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Domain string

const (
	DomainFood Domain = "FOOD"
	DomainShop Domain = "SHOP"
)

func ParseDomain(value string) (Domain, error) {
	switch Domain(strings.ToUpper(strings.TrimSpace(value))) {
	case DomainFood:
		return DomainFood, nil
	case DomainShop:
		return DomainShop, nil
	default:
		return "", ErrInvalidDomain
	}
}

type Status string

const (
	StatusActive             Status = "ACTIVE"
	StatusCheckoutInProgress Status = "CHECKOUT_IN_PROGRESS"
	StatusConverted          Status = "CONVERTED"
	StatusAbandoned          Status = "ABANDONED"
	StatusExpired            Status = "EXPIRED"
)

func (s Status) MutableByCartCommands() bool {
	return s == StatusActive
}

const (
	PricingBasisFixedUnit       = "FIXED_UNIT"
	PricingBasisFixedMeasure    = "FIXED_MEASURE"
	PricingBasisVariableMeasure = "VARIABLE_MEASURE"

	MaxCartItemQuantity = int64(100000)
	MaxOptionQuantity   = int64(100)
	MaxReceiverNameLen  = 160
	MaxReceiverPhoneLen = 32
)

var (
	ErrInvalidDomain        = errors.New("commerce cart domain must be FOOD or SHOP")
	ErrDomainDisabled       = errors.New("commerce cart domain is not enabled for customers")
	ErrCartNotFound         = errors.New("commerce cart not found")
	ErrCartNotMutable       = errors.New("commerce cart is not mutable")
	ErrVersionConflict      = errors.New("commerce cart version conflict")
	ErrDomainConflict       = errors.New("FOOD and SHOP cannot coexist in one cart")
	ErrFoodBranchConflict   = errors.New("Food cart is already bound to another vendor branch")
	ErrInvalidQuantity      = errors.New("invalid cart quantity")
	ErrInvalidReceiver      = errors.New("invalid third-party receiver")
	ErrInvalidAddress       = errors.New("invalid delivery address snapshot")
	ErrInvalidCatalogueItem = errors.New("invalid authoritative catalogue result")
	ErrDependencyMissing    = errors.New("required authoritative dependency is not connected")

	ErrInvalidSubstitutionPolicy = errors.New("invalid substitution policy for this cart line")
)

// Customer-selectable substitution rules. Only rules that need no extra
// parameters are offered; Food lines are always EXACT_ONLY.
const (
	SubstitutionExactOnly               = "EXACT_ONLY"
	SubstitutionAutoRefund              = "AUTO_REFUND"
	SubstitutionContactFirst            = "CONTACT_FIRST"
	SubstitutionVendorSuggestAndApprove = "VENDOR_SUGGEST_AND_APPROVE"
)

// NormalizeSubstitutionPolicy resolves the customer's stated rule for a line.
// An empty rule is EXACT_ONLY: a vendor may never replace what the customer
// did not allow.
func NormalizeSubstitutionPolicy(domain Domain, value string) (string, error) {
	policy := strings.ToUpper(strings.TrimSpace(value))
	if policy == "" {
		return SubstitutionExactOnly, nil
	}
	switch policy {
	case SubstitutionExactOnly:
		return policy, nil
	case SubstitutionAutoRefund, SubstitutionContactFirst, SubstitutionVendorSuggestAndApprove:
		if domain != DomainShop {
			return "", ErrInvalidSubstitutionPolicy
		}
		return policy, nil
	default:
		return "", ErrInvalidSubstitutionPolicy
	}
}

type VersionConflictError struct {
	Expected int64
	Actual   int64
}

func (e VersionConflictError) Error() string {
	return fmt.Sprintf(
		"%s: expected version %d, actual version %d",
		ErrVersionConflict.Error(),
		e.Expected,
		e.Actual,
	)
}

func (e VersionConflictError) Unwrap() error {
	return ErrVersionConflict
}

type AddressSnapshot struct {
	SourceAddressRef string    `json:"source_address_ref"`
	Title            string    `json:"title"`
	Address          string    `json:"address"`
	Latitude         float64   `json:"latitude"`
	Longitude        float64   `json:"longitude"`
	Description      string    `json:"description,omitempty"`
	SnapshottedAt    time.Time `json:"snapshotted_at"`
}

type Receiver struct {
	Name       string `json:"name"`
	Phone      string `json:"phone"`
	CustomerID string `json:"customer_id,omitempty"`
}

type RequestedOption struct {
	GroupRef  string `json:"group_ref"`
	OptionRef string `json:"option_ref"`
	Quantity  int64  `json:"quantity"`
}

type AddItemInput struct {
	ItemRef     string
	VariantRef  string
	MenuRef     string
	ListingRef  string
	SellUnitRef string
	SizeRef     string
	Quantity    int64

	RequestedMeasureRef     string
	ExpectedMeasureQuantity int64
	PricingBasis            string

	// SubstitutionPolicy is the customer's rule for this Shop line.
	SubstitutionPolicy string

	Options []RequestedOption
}

type ValidatedOption struct {
	GroupRef          string
	OptionRef         string
	Quantity          int64
	GroupName         string
	OptionName        string
	PriceDeltaMinor   int64
	PriceReference    string
	CatalogueRevision int64
}

type ValidatedItem struct {
	Domain     Domain
	BusinessID string
	BranchID   string

	ItemRef     string
	VariantRef  string
	MenuRef     string
	ListingRef  string
	SellUnitRef string
	SizeRef     string

	Quantity int64

	UnitPriceMinor    int64
	Currency          string
	PriceReference    string
	CatalogueRevision int64
	InventoryRevision *int64

	DisplayName  string
	VariantName  string
	SellUnitName string

	RequestedMeasureRef     string
	ExpectedMeasureQuantity int64
	PricingBasis            string

	LogisticsProfileRef  string
	EstimatedWeightGrams *int64
	TemperatureZone      string

	Options []ValidatedOption
}

type CartItemOption struct {
	ID                string `json:"id"`
	GroupRef          string `json:"group_ref"`
	OptionRef         string `json:"option_ref"`
	Quantity          int64  `json:"quantity"`
	GroupName         string `json:"group_name"`
	OptionName        string `json:"option_name"`
	PriceDeltaMinor   int64  `json:"price_delta_minor_snapshot"`
	PriceReference    string `json:"price_reference"`
	CatalogueRevision int64  `json:"catalogue_revision"`
}

type CartItem struct {
	ID         string `json:"id"`
	BusinessID string `json:"business_id"`
	BranchID   string `json:"branch_id"`

	Domain Domain `json:"domain"`

	ItemRef     string `json:"item_ref"`
	VariantRef  string `json:"variant_ref,omitempty"`
	MenuRef     string `json:"menu_ref,omitempty"`
	ListingRef  string `json:"listing_ref,omitempty"`
	SellUnitRef string `json:"sell_unit_ref,omitempty"`
	SizeRef     string `json:"size_ref,omitempty"`

	Quantity int64 `json:"quantity"`

	UnitPriceMinor    int64  `json:"unit_price_minor_snapshot"`
	Currency          string `json:"currency_snapshot"`
	PriceReference    string `json:"price_reference"`
	CatalogueRevision int64  `json:"catalogue_revision"`
	InventoryRevision *int64 `json:"inventory_revision,omitempty"`

	DisplayName  string `json:"display_name"`
	VariantName  string `json:"variant_name,omitempty"`
	SellUnitName string `json:"sell_unit_name,omitempty"`

	RequestedMeasureRef     string `json:"requested_measure_ref,omitempty"`
	ExpectedMeasureQuantity int64  `json:"expected_measure_quantity,omitempty"`
	PricingBasis            string `json:"pricing_basis"`

	LogisticsProfileRef  string `json:"logistics_profile_ref,omitempty"`
	EstimatedWeightGrams *int64 `json:"estimated_weight_grams,omitempty"`
	TemperatureZone      string `json:"temperature_zone,omitempty"`

	SubstitutionPolicy string `json:"substitution_policy"`

	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Options []CartItemOption `json:"options"`
}

type Cart struct {
	ID         string `json:"id"`
	CustomerID string `json:"-"`
	Domain     Domain `json:"domain"`
	Status     Status `json:"status"`

	FoodBusinessID string `json:"food_business_id,omitempty"`
	FoodBranchID   string `json:"food_branch_id,omitempty"`

	Version   int64     `json:"version"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Address  *AddressSnapshot `json:"delivery_address,omitempty"`
	Receiver *Receiver        `json:"receiver,omitempty"`

	Items []CartItem `json:"items"`
}

type ShopGroup struct {
	BusinessID string     `json:"business_id"`
	BranchID   string     `json:"branch_id"`
	Items      []CartItem `json:"items"`
}

func GroupShopCart(cart Cart) ([]ShopGroup, error) {
	if cart.Domain != DomainShop {
		return nil, ErrDomainConflict
	}

	type key struct {
		business string
		branch   string
	}

	grouped := make(map[key][]CartItem)

	for _, item := range cart.Items {
		if item.Domain != DomainShop {
			return nil, ErrDomainConflict
		}

		k := key{
			business: item.BusinessID,
			branch:   item.BranchID,
		}

		grouped[k] = append(grouped[k], item)
	}

	keys := make([]key, 0, len(grouped))

	for k := range grouped {
		keys = append(keys, k)
	}

	sort.Slice(
		keys,
		func(i, j int) bool {
			if keys[i].business == keys[j].business {
				return keys[i].branch < keys[j].branch
			}

			return keys[i].business < keys[j].business
		},
	)

	out := make([]ShopGroup, 0, len(keys))

	for _, k := range keys {
		out = append(
			out,
			ShopGroup{
				BusinessID: k.business,
				BranchID:   k.branch,
				Items:      grouped[k],
			},
		)
	}

	return out, nil
}

func ValidateReceiver(receiver Receiver) (Receiver, error) {
	receiver.Name = strings.TrimSpace(receiver.Name)
	receiver.Phone = strings.TrimSpace(receiver.Phone)
	receiver.CustomerID = strings.TrimSpace(receiver.CustomerID)

	if receiver.Name == "" ||
		len(receiver.Name) > MaxReceiverNameLen ||
		len(receiver.Phone) < 3 ||
		len(receiver.Phone) > MaxReceiverPhoneLen {
		return Receiver{}, ErrInvalidReceiver
	}
	if receiver.CustomerID != "" {
		if _, err := uuid.Parse(receiver.CustomerID); err != nil {
			return Receiver{}, ErrInvalidReceiver
		}
	}

	return receiver, nil
}

func ValidateAddressSnapshot(snapshot AddressSnapshot) (AddressSnapshot, error) {
	snapshot.SourceAddressRef = strings.TrimSpace(snapshot.SourceAddressRef)
	snapshot.Title = strings.TrimSpace(snapshot.Title)
	snapshot.Address = strings.TrimSpace(snapshot.Address)
	snapshot.Description = strings.TrimSpace(snapshot.Description)

	if snapshot.SourceAddressRef == "" ||
		snapshot.Address == "" ||
		snapshot.Latitude < -90 ||
		snapshot.Latitude > 90 ||
		snapshot.Longitude < -180 ||
		snapshot.Longitude > 180 {
		return AddressSnapshot{}, ErrInvalidAddress
	}

	return snapshot, nil
}

func normalizePricingBasis(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))

	if value == "" {
		return PricingBasisFixedUnit
	}

	return value
}

func ValidateValidatedItem(
	expectedDomain Domain,
	item ValidatedItem,
) (ValidatedItem, error) {
	item.BusinessID = strings.TrimSpace(item.BusinessID)
	item.BranchID = strings.TrimSpace(item.BranchID)

	item.ItemRef = strings.TrimSpace(item.ItemRef)
	item.VariantRef = strings.TrimSpace(item.VariantRef)
	item.MenuRef = strings.TrimSpace(item.MenuRef)
	item.ListingRef = strings.TrimSpace(item.ListingRef)
	item.SellUnitRef = strings.TrimSpace(item.SellUnitRef)
	item.SizeRef = strings.TrimSpace(item.SizeRef)

	item.Currency = strings.ToUpper(strings.TrimSpace(item.Currency))
	item.PriceReference = strings.TrimSpace(item.PriceReference)

	item.DisplayName = strings.TrimSpace(item.DisplayName)
	item.VariantName = strings.TrimSpace(item.VariantName)
	item.SellUnitName = strings.TrimSpace(item.SellUnitName)

	item.RequestedMeasureRef = strings.TrimSpace(item.RequestedMeasureRef)
	item.PricingBasis = normalizePricingBasis(item.PricingBasis)

	item.LogisticsProfileRef = strings.TrimSpace(item.LogisticsProfileRef)
	item.TemperatureZone = strings.TrimSpace(item.TemperatureZone)

	if item.Domain != expectedDomain {
		return ValidatedItem{}, ErrDomainConflict
	}

	if item.BusinessID == "" ||
		item.BranchID == "" ||
		item.ItemRef == "" ||
		item.DisplayName == "" ||
		item.Quantity < 1 ||
		item.Quantity > MaxCartItemQuantity ||
		item.UnitPriceMinor < 0 ||
		len(item.Currency) != 3 ||
		item.PriceReference == "" ||
		item.CatalogueRevision < 0 {
		return ValidatedItem{}, ErrInvalidCatalogueItem
	}

	switch item.Domain {
	case DomainFood:
		if item.MenuRef == "" ||
			item.ListingRef != "" ||
			item.SellUnitRef != "" {
			return ValidatedItem{}, ErrInvalidCatalogueItem
		}

	case DomainShop:
		if item.ListingRef == "" ||
			item.MenuRef != "" ||
			item.SellUnitRef == "" {
			return ValidatedItem{}, ErrInvalidCatalogueItem
		}

	default:
		return ValidatedItem{}, ErrInvalidCatalogueItem
	}

	switch item.PricingBasis {
	case PricingBasisFixedUnit,
		PricingBasisFixedMeasure:

	case PricingBasisVariableMeasure:
		if item.RequestedMeasureRef == "" ||
			item.ExpectedMeasureQuantity <= 0 {
			return ValidatedItem{}, ErrInvalidCatalogueItem
		}

	default:
		return ValidatedItem{}, ErrInvalidCatalogueItem
	}

	if item.InventoryRevision != nil &&
		*item.InventoryRevision < 0 {
		return ValidatedItem{}, ErrInvalidCatalogueItem
	}

	if item.EstimatedWeightGrams != nil &&
		*item.EstimatedWeightGrams < 0 {
		return ValidatedItem{}, ErrInvalidCatalogueItem
	}

	for i := range item.Options {
		option := &item.Options[i]

		option.GroupRef = strings.TrimSpace(option.GroupRef)
		option.OptionRef = strings.TrimSpace(option.OptionRef)
		option.GroupName = strings.TrimSpace(option.GroupName)
		option.OptionName = strings.TrimSpace(option.OptionName)
		option.PriceReference = strings.TrimSpace(option.PriceReference)

		if option.GroupRef == "" ||
			option.OptionRef == "" ||
			option.OptionName == "" ||
			option.PriceReference == "" ||
			option.Quantity < 1 ||
			option.Quantity > MaxOptionQuantity ||
			option.CatalogueRevision < 0 {
			return ValidatedItem{}, ErrInvalidCatalogueItem
		}
	}

	return item, nil
}
