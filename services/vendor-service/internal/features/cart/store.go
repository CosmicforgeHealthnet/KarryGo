package cart

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type queryer interface {
	QueryRow(
		ctx context.Context,
		sql string,
		args ...any,
	) pgx.Row

	Query(
		ctx context.Context,
		sql string,
		args ...any,
	) (pgx.Rows, error)
}

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) DB() *pgxpool.Pool {
	return s.db
}

const cartColumns = `
    id,
    customer_id,
    domain,
    status,
    COALESCE(food_business_id,''),
    COALESCE(food_branch_id,''),
    version,
    expires_at,
    created_at,
    updated_at
`

func scanCart(row pgx.Row) (Cart, error) {
	var cart Cart

	err := row.Scan(
		&cart.ID,
		&cart.CustomerID,
		&cart.Domain,
		&cart.Status,
		&cart.FoodBusinessID,
		&cart.FoodBranchID,
		&cart.Version,
		&cart.ExpiresAt,
		&cart.CreatedAt,
		&cart.UpdatedAt,
	)

	return cart, err
}

func loadOwnedCart(
	ctx context.Context,
	q queryer,
	customerID string,
	cartID string,
	forUpdate bool,
) (Cart, error) {
	sql := `
        SELECT ` + cartColumns + `
        FROM commerce_carts
        WHERE id=$1
          AND customer_id=$2
    `

	if forUpdate {
		sql += ` FOR UPDATE`
	}

	cart, err := scanCart(
		q.QueryRow(
			ctx,
			sql,
			cartID,
			customerID,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Cart{}, ErrCartNotFound
	}

	if err != nil {
		return Cart{}, err
	}

	if err = loadCartAddress(ctx, q, &cart); err != nil {
		return Cart{}, err
	}

	if err = loadCartReceiver(ctx, q, &cart); err != nil {
		return Cart{}, err
	}

	if err = loadCartItems(ctx, q, &cart); err != nil {
		return Cart{}, err
	}

	return cart, nil
}

func loadCartAddress(
	ctx context.Context,
	q queryer,
	cart *Cart,
) error {
	var address AddressSnapshot

	err := q.QueryRow(
		ctx,
		`
        SELECT
            source_address_ref,
            title_snapshot,
            address_snapshot,
            latitude_snapshot,
            longitude_snapshot,
            description_snapshot,
            snapshotted_at
        FROM commerce_cart_addresses
        WHERE cart_id=$1
        `,
		cart.ID,
	).Scan(
		&address.SourceAddressRef,
		&address.Title,
		&address.Address,
		&address.Latitude,
		&address.Longitude,
		&address.Description,
		&address.SnapshottedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}

	if err != nil {
		return err
	}

	cart.Address = &address

	return nil
}

func loadCartReceiver(
	ctx context.Context,
	q queryer,
	cart *Cart,
) error {
	var receiver Receiver

	err := q.QueryRow(
		ctx,
		`
        SELECT receiver_name, receiver_phone
        FROM commerce_cart_receivers
        WHERE cart_id=$1
        `,
		cart.ID,
	).Scan(
		&receiver.Name,
		&receiver.Phone,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}

	if err != nil {
		return err
	}

	cart.Receiver = &receiver

	return nil
}

func loadCartItems(
	ctx context.Context,
	q queryer,
	cart *Cart,
) error {
	rows, err := q.Query(
		ctx,
		`
        SELECT
            id,
            business_id,
            branch_id,
            domain,
            item_ref,
            COALESCE(variant_ref,''),
            COALESCE(menu_ref,''),
            COALESCE(listing_ref,''),
            COALESCE(sell_unit_ref,''),
            COALESCE(size_ref,''),
            quantity,
            unit_price_minor_snapshot,
            currency_snapshot,
            price_reference,
            catalogue_revision,
            inventory_revision,
            display_name_snapshot,
            variant_name_snapshot,
            sell_unit_name_snapshot,
            COALESCE(requested_measure_ref,''),
            COALESCE(expected_measure_quantity,0),
            pricing_basis,
            COALESCE(logistics_profile_ref,''),
            estimated_weight_grams,
            COALESCE(temperature_zone,''),
            version,
            created_at,
            updated_at
        FROM commerce_cart_items
        WHERE cart_id=$1
        ORDER BY created_at,id
        `,
		cart.ID,
	)

	if err != nil {
		return err
	}

	defer rows.Close()

	cart.Items = nil
	index := make(map[string]int)

	for rows.Next() {
		var item CartItem

		if err = rows.Scan(
			&item.ID,
			&item.BusinessID,
			&item.BranchID,
			&item.Domain,
			&item.ItemRef,
			&item.VariantRef,
			&item.MenuRef,
			&item.ListingRef,
			&item.SellUnitRef,
			&item.SizeRef,
			&item.Quantity,
			&item.UnitPriceMinor,
			&item.Currency,
			&item.PriceReference,
			&item.CatalogueRevision,
			&item.InventoryRevision,
			&item.DisplayName,
			&item.VariantName,
			&item.SellUnitName,
			&item.RequestedMeasureRef,
			&item.ExpectedMeasureQuantity,
			&item.PricingBasis,
			&item.LogisticsProfileRef,
			&item.EstimatedWeightGrams,
			&item.TemperatureZone,
			&item.Version,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return err
		}

		index[item.ID] = len(cart.Items)
		cart.Items = append(cart.Items, item)
	}

	if err = rows.Err(); err != nil {
		return err
	}

	if len(cart.Items) == 0 {
		return nil
	}

	optionRows, err := q.Query(
		ctx,
		`
        SELECT
            id,
            cart_item_id,
            group_ref,
            option_ref,
            quantity,
            group_name_snapshot,
            option_name_snapshot,
            price_delta_minor_snapshot,
            price_reference,
            catalogue_revision
        FROM commerce_cart_item_options
        WHERE cart_id=$1
        ORDER BY cart_item_id,id
        `,
		cart.ID,
	)

	if err != nil {
		return err
	}

	defer optionRows.Close()

	for optionRows.Next() {
		var (
			itemID string
			option CartItemOption
		)

		if err = optionRows.Scan(
			&option.ID,
			&itemID,
			&option.GroupRef,
			&option.OptionRef,
			&option.Quantity,
			&option.GroupName,
			&option.OptionName,
			&option.PriceDeltaMinor,
			&option.PriceReference,
			&option.CatalogueRevision,
		); err != nil {
			return err
		}

		itemIndex, ok := index[itemID]

		if !ok {
			continue
		}

		cart.Items[itemIndex].Options = append(
			cart.Items[itemIndex].Options,
			option,
		)
	}

	return optionRows.Err()
}
