package inventory

import (
	"context"
	"errors"
	"testing"
	"time"
)

type section14BStore struct {
	item InventoryItem
	err  error
}

func (s *section14BStore) CreateInventoryItem(
	_ context.Context,
	item InventoryItem,
) (InventoryItem, error) {
	if s.err != nil {
		return InventoryItem{}, s.err
	}

	item.CreatedAt = time.Now().UTC()
	item.UpdatedAt = item.CreatedAt
	s.item = item

	return item, nil
}

func (s *section14BStore) GetInventoryItem(
	_ context.Context,
	_ string,
	_ string,
) (InventoryItem, error) {
	if s.item.ID == "" {
		return InventoryItem{}, ErrItemNotFound
	}

	return s.item, nil
}

func (s *section14BStore) InventoryRevision(
	_ context.Context,
	_ string,
	_ string,
) (int64, time.Time, error) {
	return 1, time.Now().UTC(), nil
}

type section14BResolver struct {
	businessID string
	branchID   string
	active     bool
	err        error
}

func (r section14BResolver) ResolveInventorySource(
	_ context.Context,
	_ Actor,
	_ BranchScope,
	source SourceReference,
) (
	ResolvedInventorySource,
	error,
) {
	if r.err != nil {
		return ResolvedInventorySource{}, r.err
	}

	branch := r.branchID

	return ResolvedInventorySource{
		Source:     source,
		BusinessID: r.businessID,
		BranchID:   &branch,
		Active:     r.active,
	}, nil
}

type section14BMeasureResolver struct {
	resolution ResolvedStockMeasure
	err        error
}

func (r section14BMeasureResolver) ResolveStockMeasure(
	_ context.Context,
	_ Actor,
	_ BranchScope,
	_ SourceReference,
	_ string,
) (
	ResolvedStockMeasure,
	error,
) {
	if r.err != nil {
		return ResolvedStockMeasure{}, r.err
	}

	return r.resolution, nil
}

func section14BService(
	store InventoryItemStore,
	source InventoryItemResolver,
	measure StockMeasureResolver,
) *ItemService {
	foundation := NewService(
		Dependencies{
			Flags: Flags{
				Enabled:       true,
				WritesEnabled: true,
			},
		},
	)

	return NewItemService(
		foundation,
		store,
		source,
		measure,
	)
}

func section14BActor() Actor {
	return Actor{
		StaffID:    "staff-a",
		BusinessID: "business-a",
	}
}

func section14BScope(t *testing.T) BranchScope {
	t.Helper()

	scope, err := BusinessScope(
		"business-a",
	)
	if err != nil {
		t.Fatal(err)
	}

	return scope
}

func section14BInput() RegisterInventoryItemInput {
	return RegisterInventoryItemInput{
		BranchID:        "branch-a",
		ItemType:        ItemTypeShopVariant,
		SourceRef:       "variant-a",
		StockMeasureRef: "smd_kg",
		StockBasisMode:  StockBasisExactConversion,
		TrackingMode:    TrackingSimple,
		LotPolicy:       LotPolicyFIFO,
	}
}

func section14BGoodMeasure() ResolvedStockMeasure {
	return ResolvedStockMeasure{
		Ref:                "smd_kg",
		OwnerKey:           "PLATFORM",
		Family:             "MASS",
		UnitKind:           StockUnitMass,
		Active:             true,
		SameUnitEligible:   true,
		ExactConvertible:   true,
		VariableFulfilment: true,
	}
}

func TestSection14BShopVariantCanBecomeGenericInventoryItem(
	t *testing.T,
) {
	store := &section14BStore{}

	service := section14BService(
		store,
		section14BResolver{
			businessID: "business-a",
			branchID:   "branch-a",
			active:     true,
		},
		section14BMeasureResolver{
			resolution: section14BGoodMeasure(),
		},
	)

	item, err := service.RegisterInventoryItem(
		context.Background(),
		section14BActor(),
		section14BScope(t),
		section14BInput(),
	)
	if err != nil {
		t.Fatalf(
			"RegisterInventoryItem: %v",
			err,
		)
	}

	if item.ItemType != ItemTypeShopVariant {
		t.Fatalf(
			"item type = %q",
			item.ItemType,
		)
	}

	if item.SourceRef != "variant-a" {
		t.Fatalf(
			"source = %q",
			item.SourceRef,
		)
	}

	if item.StockUnitKind != StockUnitMass {
		t.Fatalf(
			"unit kind = %q",
			item.StockUnitKind,
		)
	}
}

func TestSection14BRejectsUnsupportedFutureItemTypes(
	t *testing.T,
) {
	store := &section14BStore{}

	service := section14BService(
		store,
		section14BResolver{},
		section14BMeasureResolver{},
	)

	input := section14BInput()
	input.ItemType = ItemTypeIngredient

	_, err := service.RegisterInventoryItem(
		context.Background(),
		section14BActor(),
		section14BScope(t),
		input,
	)

	if !errors.Is(
		err,
		ErrUnsupportedItemType,
	) {
		t.Fatalf(
			"error = %v; want ErrUnsupportedItemType",
			err,
		)
	}
}

func TestSection14BCrossBusinessSourceCannotRegister(
	t *testing.T,
) {
	service := section14BService(
		&section14BStore{},
		section14BResolver{
			businessID: "business-b",
			branchID:   "branch-a",
			active:     true,
		},
		section14BMeasureResolver{
			resolution: section14BGoodMeasure(),
		},
	)

	_, err := service.RegisterInventoryItem(
		context.Background(),
		section14BActor(),
		section14BScope(t),
		section14BInput(),
	)

	if !errors.Is(
		err,
		ErrScopeViolation,
	) {
		t.Fatalf(
			"error = %v; want ErrScopeViolation",
			err,
		)
	}
}

func TestSection14BForeignBranchSourceCannotRegister(
	t *testing.T,
) {
	service := section14BService(
		&section14BStore{},
		section14BResolver{
			businessID: "business-a",
			branchID:   "branch-b",
			active:     true,
		},
		section14BMeasureResolver{
			resolution: section14BGoodMeasure(),
		},
	)

	_, err := service.RegisterInventoryItem(
		context.Background(),
		section14BActor(),
		section14BScope(t),
		section14BInput(),
	)

	if !errors.Is(
		err,
		ErrScopeViolation,
	) {
		t.Fatalf(
			"error = %v; want ErrScopeViolation",
			err,
		)
	}
}

func TestSection14BArchivedSourceCannotBecomeActiveBasis(
	t *testing.T,
) {
	service := section14BService(
		&section14BStore{},
		section14BResolver{
			businessID: "business-a",
			branchID:   "branch-a",
			active:     false,
		},
		section14BMeasureResolver{
			resolution: section14BGoodMeasure(),
		},
	)

	_, err := service.RegisterInventoryItem(
		context.Background(),
		section14BActor(),
		section14BScope(t),
		section14BInput(),
	)

	if !errors.Is(
		err,
		ErrSourceInactive,
	) {
		t.Fatalf(
			"error = %v; want ErrSourceInactive",
			err,
		)
	}
}

func TestSection14BExactArithmeticFailsClosed(
	t *testing.T,
) {
	measure := section14BGoodMeasure()
	measure.ExactConvertible = false

	service := section14BService(
		&section14BStore{},
		section14BResolver{
			businessID: "business-a",
			branchID:   "branch-a",
			active:     true,
		},
		section14BMeasureResolver{
			resolution: measure,
		},
	)

	_, err := service.RegisterInventoryItem(
		context.Background(),
		section14BActor(),
		section14BScope(t),
		section14BInput(),
	)

	if !errors.Is(
		err,
		ErrMeasurementNotExact,
	) {
		t.Fatalf(
			"error = %v; want ErrMeasurementNotExact",
			err,
		)
	}
}

func TestSection14BDuplicateLogicalItemFails(
	t *testing.T,
) {
	store := &section14BStore{
		err: ErrDuplicateItem,
	}

	service := section14BService(
		store,
		section14BResolver{
			businessID: "business-a",
			branchID:   "branch-a",
			active:     true,
		},
		section14BMeasureResolver{
			resolution: section14BGoodMeasure(),
		},
	)

	_, err := service.RegisterInventoryItem(
		context.Background(),
		section14BActor(),
		section14BScope(t),
		section14BInput(),
	)

	if !errors.Is(
		err,
		ErrDuplicateItem,
	) {
		t.Fatalf(
			"error = %v; want ErrDuplicateItem",
			err,
		)
	}
}

func TestSection14BVariableFulfilmentRequiresVariableTracking(
	t *testing.T,
) {
	input := section14BInput()
	input.StockBasisMode = StockBasisVariableFulfilment
	input.TrackingMode = TrackingSimple

	service := section14BService(
		&section14BStore{},
		section14BResolver{
			businessID: "business-a",
			branchID:   "branch-a",
			active:     true,
		},
		section14BMeasureResolver{
			resolution: section14BGoodMeasure(),
		},
	)

	_, err := service.RegisterInventoryItem(
		context.Background(),
		section14BActor(),
		section14BScope(t),
		input,
	)

	if err == nil {
		t.Fatal(
			"VARIABLE_FULFILMENT accepted non-variable tracking",
		)
	}
}
