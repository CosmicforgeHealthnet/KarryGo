package inventory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"
)

const (
	ItemTypeShopVariant         = "SHOP_VARIANT"
	ItemTypeIngredient          = "INGREDIENT"
	ItemTypePackaging           = "PACKAGING"
	ItemTypeProductionComponent = "PRODUCTION_COMPONENT"

	ItemStatusActive   = "ACTIVE"
	ItemStatusArchived = "ARCHIVED"
)

// InventoryItem is the generic Inventory identity / stock-basis definition.
//
// It carries no physical stock quantity in Section 14B.
type InventoryItem struct {
	ID string

	BusinessID string
	BranchID   string

	ItemType  string
	SourceRef string

	StockMeasureRef      string
	StockMeasureOwnerKey string
	StockUnitKind        string
	StockBasisMode       string

	TrackingMode       string
	LotPolicy          string
	LotTrackingEnabled bool

	Status  string
	Version int64

	CreatedAt time.Time
	UpdatedAt time.Time
}

type RegisterInventoryItemInput struct {
	BranchID string

	ItemType  string
	SourceRef string

	StockMeasureRef string
	StockBasisMode  string

	TrackingMode       string
	LotPolicy          string
	LotTrackingEnabled bool
}

// InventoryItemStore owns generic item identity persistence.
type InventoryItemStore interface {
	CreateInventoryItem(
		ctx context.Context,
		item InventoryItem,
	) (InventoryItem, error)

	GetInventoryItem(
		ctx context.Context,
		businessID string,
		itemID string,
	) (InventoryItem, error)

	InventoryRevision(
		ctx context.Context,
		businessID string,
		branchID string,
	) (int64, time.Time, error)
}

// ItemService implements the Section 14B generic item identity boundary.
type ItemService struct {
	foundation *Service
	store      InventoryItemStore
	resolver   InventoryItemResolver
	measures   StockMeasureResolver
}

func NewItemService(
	foundation *Service,
	store InventoryItemStore,
	resolver InventoryItemResolver,
	measures StockMeasureResolver,
) *ItemService {
	return &ItemService{
		foundation: foundation,
		store:      store,
		resolver:   resolver,
		measures:   measures,
	}
}

// RegisterInventoryItem registers one source-domain item as a generic
// Inventory stock identity.
//
// Section 14B enables SHOP_VARIANT only. Future item-type constants are
// intentionally represented now without activating Ingredient/Recipe logic.
func (s *ItemService) RegisterInventoryItem(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	input RegisterInventoryItemInput,
) (InventoryItem, error) {
	if s == nil ||
		s.foundation == nil ||
		s.store == nil ||
		s.resolver == nil ||
		s.measures == nil {
		return InventoryItem{}, ErrDependencyMissing
	}

	if err := s.foundation.RequireWrite(
		ctx,
		actor,
		scope,
	); err != nil {
		return InventoryItem{}, err
	}

	input.BranchID = strings.TrimSpace(input.BranchID)
	input.ItemType = strings.ToUpper(strings.TrimSpace(input.ItemType))
	input.SourceRef = strings.TrimSpace(input.SourceRef)
	input.StockMeasureRef = strings.TrimSpace(input.StockMeasureRef)
	input.StockBasisMode = strings.ToUpper(strings.TrimSpace(input.StockBasisMode))
	input.TrackingMode = strings.ToUpper(strings.TrimSpace(input.TrackingMode))
	input.LotPolicy = strings.ToUpper(strings.TrimSpace(input.LotPolicy))

	if input.ItemType != ItemTypeShopVariant {
		return InventoryItem{}, ErrUnsupportedItemType
	}

	if input.BranchID == "" ||
		input.SourceRef == "" ||
		input.StockMeasureRef == "" {
		return InventoryItem{}, ErrInvalidSource
	}

	if !scope.Covers(
		actor.BusinessID,
		input.BranchID,
	) {
		return InventoryItem{}, ErrScopeViolation
	}

	switch input.StockBasisMode {
	case StockBasisSameUnit,
		StockBasisExactConversion,
		StockBasisVariableFulfilment:
	default:
		return InventoryItem{}, ErrInvalidMeasure
	}

	switch input.TrackingMode {
	case TrackingNone,
		TrackingSimple,
		TrackingLot,
		TrackingExpiry,
		TrackingVariableMeasure:
	default:
		return InventoryItem{}, ErrInvalidSource
	}

	if input.LotPolicy == "" {
		input.LotPolicy = LotPolicyFIFO
	}

	switch input.LotPolicy {
	case LotPolicyFIFO, LotPolicyFEFO:
	default:
		return InventoryItem{}, ErrInvalidSource
	}

	if (input.TrackingMode == TrackingLot ||
		input.TrackingMode == TrackingExpiry) &&
		!input.LotTrackingEnabled {
		return InventoryItem{}, ErrInvalidSource
	}

	if input.StockBasisMode == StockBasisVariableFulfilment &&
		input.TrackingMode != TrackingVariableMeasure {
		return InventoryItem{}, ErrInvalidSource
	}

	exactScope, err := ExactBranchScope(
		actor.BusinessID,
		input.BranchID,
	)
	if err != nil {
		return InventoryItem{}, err
	}

	source := SourceReference{
		Type: input.ItemType,
		Ref:  input.SourceRef,
	}

	resolvedSource, err :=
		s.resolver.ResolveInventorySource(
			ctx,
			actor,
			exactScope,
			source,
		)
	if err != nil {
		return InventoryItem{}, err
	}

	if !resolvedSource.Active {
		return InventoryItem{}, ErrSourceInactive
	}

	if strings.TrimSpace(resolvedSource.BusinessID) !=
		strings.TrimSpace(actor.BusinessID) {
		return InventoryItem{}, ErrScopeViolation
	}

	if resolvedSource.BranchID != nil &&
		strings.TrimSpace(*resolvedSource.BranchID) != input.BranchID {
		return InventoryItem{}, ErrScopeViolation
	}

	measure, err :=
		s.measures.ResolveStockMeasure(
			ctx,
			actor,
			exactScope,
			source,
			input.StockMeasureRef,
		)
	if err != nil {
		return InventoryItem{}, err
	}

	if !measure.Active ||
		strings.TrimSpace(measure.Ref) != input.StockMeasureRef ||
		strings.TrimSpace(measure.OwnerKey) == "" {
		return InventoryItem{}, ErrInvalidMeasure
	}

	switch measure.UnitKind {
	case StockUnitCount,
		StockUnitMass,
		StockUnitVolume,
		StockUnitConfiguredMeasure:
	default:
		return InventoryItem{}, ErrInvalidMeasure
	}

	switch input.StockBasisMode {
	case StockBasisSameUnit:
		if !measure.SameUnitEligible {
			return InventoryItem{}, ErrMeasurementNotExact
		}

	case StockBasisExactConversion:
		if !measure.ExactConvertible {
			return InventoryItem{}, ErrMeasurementNotExact
		}

	case StockBasisVariableFulfilment:
		if !measure.VariableFulfilment {
			return InventoryItem{}, ErrInvalidMeasure
		}
	}

	id, err := newInventoryItemID()
	if err != nil {
		return InventoryItem{}, err
	}

	item := InventoryItem{
		ID: id,

		BusinessID: actor.BusinessID,
		BranchID:   input.BranchID,

		ItemType:  input.ItemType,
		SourceRef: input.SourceRef,

		StockMeasureRef:      input.StockMeasureRef,
		StockMeasureOwnerKey: measure.OwnerKey,
		StockUnitKind:        measure.UnitKind,
		StockBasisMode:       input.StockBasisMode,

		TrackingMode:       input.TrackingMode,
		LotPolicy:          input.LotPolicy,
		LotTrackingEnabled: input.LotTrackingEnabled,

		Status:  ItemStatusActive,
		Version: 1,
	}

	return s.store.CreateInventoryItem(
		ctx,
		item,
	)
}

func newInventoryItemID() (string, error) {
	var raw [16]byte

	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}

	return "invitem_" +
			hex.EncodeToString(raw[:]),
		nil
}
