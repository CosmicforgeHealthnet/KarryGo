package cart

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const defaultCartTTL = 30 * 24 * time.Hour

type Service struct {
	store     *Store
	food      FoodAuthority
	shop      ShopAuthority
	addresses CustomerAddressAuthority
	events    EventPublisher
	now       func() time.Time
	// domainEnabled is the server-side public Food/Shop contract gate
	// (Karryam V4 §24: CUSTOMER_FOOD_ENABLED / CUSTOMER_SHOP_ENABLED). nil
	// means ungated, which only tests and pre-gate callers use.
	domainEnabled func(Domain) bool
}

// WithDomainGate installs the public Food/Shop contract gate. A disabled
// domain refuses new carts and new lines even from an outdated or malicious
// client; existing carts stay readable so the customer can see what they had.
func (s *Service) WithDomainGate(enabled func(Domain) bool) *Service {
	s.domainEnabled = enabled
	return s
}

func (s *Service) domainOpen(domain Domain) error {
	if s.domainEnabled != nil && !s.domainEnabled(domain) {
		return ErrDomainDisabled
	}
	return nil
}

func NewService(
	store *Store,
	food FoodAuthority,
	shop ShopAuthority,
	addresses CustomerAddressAuthority,
	publishers ...EventPublisher,
) *Service {
	var publisher EventPublisher

	if len(publishers) > 0 {
		publisher = publishers[0]
	}

	return &Service{
		store:     store,
		food:      food,
		shop:      shop,
		addresses: addresses,
		events:    publisher,
		now:       time.Now,
	}
}
func (s *Service) recordHistory(
	ctx context.Context,
	tx pgx.Tx,
	cartID string,
	customerID string,
	action string,
	itemRef string,
	addressRef string,
	version int64,
) error {
	if err := appendHistory(
		ctx,
		tx,
		cartID,
		customerID,
		action,
		itemRef,
		addressRef,
		version,
	); err != nil {
		return err
	}

	if s.events == nil {
		return nil
	}

	payload := map[string]any{
		"customer_ref": customerID,
	}

	if strings.TrimSpace(itemRef) != "" {
		payload["item_ref"] = itemRef
	}

	if strings.TrimSpace(addressRef) != "" {
		payload["address_ref"] = addressRef
	}

	return s.events.Publish(
		ctx,
		tx,
		action,
		"commerce_cart",
		cartID,
		version,
		"",
		cartID,
		"",
		payload,
	)
}
func newID(prefix string) string {
	return prefix + uuid.NewString()
}

func validateCustomerID(customerID string) (string, error) {
	customerID = strings.TrimSpace(customerID)

	if customerID == "" {
		return "", ErrCartNotFound
	}

	return customerID, nil
}

func checkExpectedVersion(cart Cart, expected int64) error {
	if expected <= 0 || cart.Version != expected {
		return VersionConflictError{
			Expected: expected,
			Actual:   cart.Version,
		}
	}

	return nil
}

func requireMutable(cart Cart) error {
	if !cart.Status.MutableByCartCommands() {
		return ErrCartNotMutable
	}

	return nil
}

func (s *Service) expireLiveCart(
	ctx context.Context,
	customerID string,
	domain Domain,
) error {
	tx, err := s.store.db.Begin(ctx)

	if err != nil {
		return err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	rows, err := tx.Query(
		ctx,
		`
        SELECT id,version
        FROM commerce_carts
        WHERE customer_id=$1
          AND domain=$2
          AND status='ACTIVE'
          AND expires_at <= $3
        FOR UPDATE
        `,
		customerID,
		domain,
		s.now(),
	)

	if err != nil {
		return err
	}

	type expired struct {
		id      string
		version int64
	}

	var candidates []expired

	for rows.Next() {
		var candidate expired

		if err = rows.Scan(
			&candidate.id,
			&candidate.version,
		); err != nil {
			rows.Close()
			return err
		}

		candidates = append(candidates, candidate)
	}

	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}

	rows.Close()

	for _, candidate := range candidates {
		nextVersion := candidate.version + 1

		if _, err = tx.Exec(
			ctx,
			`
            UPDATE commerce_carts
            SET status='EXPIRED',
                version=$2,
                updated_at=now()
            WHERE id=$1
            `,
			candidate.id,
			nextVersion,
		); err != nil {
			return err
		}

		if err = s.recordHistory(
			ctx,
			tx,
			candidate.id,
			customerID,
			"commerce.cart.expired",
			"",
			"",
			nextVersion,
		); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// CreateOrGetActiveCart creates at most one live cart for a customer/domain.
//
// customerID is deliberately a separate server-side argument. No public
// create-cart request requires or accepts customer_id authority.
func (s *Service) CreateOrGetActiveCart(
	ctx context.Context,
	customerID string,
	domain Domain,
) (Cart, error) {
	customerID, err := validateCustomerID(customerID)

	if err != nil {
		return Cart{}, err
	}

	if domain != DomainFood && domain != DomainShop {
		return Cart{}, ErrInvalidDomain
	}

	if err = s.domainOpen(domain); err != nil {
		return Cart{}, err
	}

	if err = s.expireLiveCart(
		ctx,
		customerID,
		domain,
	); err != nil {
		return Cart{}, err
	}

	tx, err := s.store.db.Begin(ctx)

	if err != nil {
		return Cart{}, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	cart, err := scanCart(
		tx.QueryRow(
			ctx,
			`
            SELECT `+cartColumns+`
            FROM commerce_carts
            WHERE customer_id=$1
              AND domain=$2
              AND status IN ('ACTIVE','CHECKOUT_IN_PROGRESS')
            FOR UPDATE
            `,
			customerID,
			domain,
		),
	)

	if err == nil {
		if err = loadCartAddress(ctx, tx, &cart); err != nil {
			return Cart{}, err
		}

		if err = loadCartReceiver(ctx, tx, &cart); err != nil {
			return Cart{}, err
		}

		if err = loadCartItems(ctx, tx, &cart); err != nil {
			return Cart{}, err
		}

		if err = tx.Commit(ctx); err != nil {
			return Cart{}, err
		}

		return cart, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return Cart{}, err
	}

	id := newID("cart_")
	expires := s.now().Add(defaultCartTTL)

	tag, err := tx.Exec(
		ctx,
		`
        INSERT INTO commerce_carts(
            id,
            customer_id,
            domain,
            status,
            version,
            expires_at
        )
        VALUES($1,$2,$3,'ACTIVE',1,$4)
        ON CONFLICT DO NOTHING
        `,
		id,
		customerID,
		domain,
		expires,
	)

	if err != nil {
		return Cart{}, err
	}

	created := tag.RowsAffected() == 1

	if created {
		if err = s.recordHistory(
			ctx,
			tx,
			id,
			customerID,
			"commerce.cart.created",
			"",
			"",
			1,
		); err != nil {
			return Cart{}, err
		}
	}

	cart, err = scanCart(
		tx.QueryRow(
			ctx,
			`
            SELECT `+cartColumns+`
            FROM commerce_carts
            WHERE customer_id=$1
              AND domain=$2
              AND status IN ('ACTIVE','CHECKOUT_IN_PROGRESS')
            FOR UPDATE
            `,
			customerID,
			domain,
		),
	)

	if err != nil {
		return Cart{}, err
	}

	if err = loadCartAddress(ctx, tx, &cart); err != nil {
		return Cart{}, err
	}

	if err = loadCartReceiver(ctx, tx, &cart); err != nil {
		return Cart{}, err
	}

	if err = loadCartItems(ctx, tx, &cart); err != nil {
		return Cart{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Cart{}, err
	}

	return cart, nil
}

func (s *Service) GetCart(
	ctx context.Context,
	customerID string,
	cartID string,
) (Cart, error) {
	customerID, err := validateCustomerID(customerID)
	if err != nil {
		return Cart{}, err
	}

	// Preview and ordinary reads must see one coherent version of the header,
	// lines, recipient and address while cart commands commit concurrently.
	tx, err := s.store.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Cart{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	current, err := loadOwnedCart(
		ctx,
		tx,
		customerID,
		strings.TrimSpace(cartID),
		false,
	)
	if err != nil {
		return Cart{}, err
	}

	if err = loadCartAddress(
		ctx,
		tx,
		&current,
	); err != nil {
		return Cart{}, err
	}

	if err = loadCartReceiver(
		ctx,
		tx,
		&current,
	); err != nil {
		return Cart{}, err
	}

	if err = loadCartItems(
		ctx,
		tx,
		&current,
	); err != nil {
		return Cart{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Cart{}, err
	}

	return current, nil
}

func (s *Service) validateAuthoritativeItem(
	ctx context.Context,
	domain Domain,
	input AddItemInput,
) (ValidatedItem, error) {
	if input.Quantity < 1 ||
		input.Quantity > MaxCartItemQuantity {
		return ValidatedItem{}, ErrInvalidQuantity
	}

	input.ItemRef = strings.TrimSpace(input.ItemRef)
	input.VariantRef = strings.TrimSpace(input.VariantRef)
	input.MenuRef = strings.TrimSpace(input.MenuRef)
	input.ListingRef = strings.TrimSpace(input.ListingRef)
	input.SellUnitRef = strings.TrimSpace(input.SellUnitRef)
	input.SizeRef = strings.TrimSpace(input.SizeRef)
	input.RequestedMeasureRef = strings.TrimSpace(
		input.RequestedMeasureRef,
	)
	input.PricingBasis = normalizePricingBasis(
		input.PricingBasis,
	)

	for i := range input.Options {
		input.Options[i].GroupRef = strings.TrimSpace(
			input.Options[i].GroupRef,
		)
		input.Options[i].OptionRef = strings.TrimSpace(
			input.Options[i].OptionRef,
		)

		if input.Options[i].Quantity == 0 {
			input.Options[i].Quantity = 1
		}
	}

	var (
		validated ValidatedItem
		err       error
	)

	switch domain {
	case DomainFood:
		if s.food == nil {
			return ValidatedItem{}, ErrDependencyMissing
		}

		validated, err = s.food.ValidateFoodCartItem(
			ctx,
			input,
		)

	case DomainShop:
		if s.shop == nil {
			return ValidatedItem{}, ErrDependencyMissing
		}

		validated, err = s.shop.ValidateShopCartItem(
			ctx,
			input,
		)

	default:
		return ValidatedItem{}, ErrInvalidDomain
	}

	if err != nil {
		return ValidatedItem{}, err
	}

	if validated.Quantity == 0 {
		validated.Quantity = input.Quantity
	}

	return ValidateValidatedItem(
		domain,
		validated,
	)
}

func (s *Service) AddItem(
	ctx context.Context,
	customerID string,
	cartID string,
	expectedVersion int64,
	input AddItemInput,
) (Cart, error) {
	customerID, err := validateCustomerID(customerID)

	if err != nil {
		return Cart{}, err
	}

	current, err := s.GetCart(
		ctx,
		customerID,
		cartID,
	)

	if err != nil {
		return Cart{}, err
	}

	if err = requireMutable(current); err != nil {
		return Cart{}, err
	}

	validated, err := s.validateAuthoritativeItem(
		ctx,
		current.Domain,
		input,
	)

	if err != nil {
		return Cart{}, err
	}

	substitutionPolicy, err := NormalizeSubstitutionPolicy(
		current.Domain,
		input.SubstitutionPolicy,
	)

	if err != nil {
		return Cart{}, err
	}

	tx, err := s.store.db.Begin(ctx)

	if err != nil {
		return Cart{}, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	cart, err := loadOwnedCart(
		ctx,
		tx,
		customerID,
		cartID,
		true,
	)

	if err != nil {
		return Cart{}, err
	}

	if err = requireMutable(cart); err != nil {
		return Cart{}, err
	}

	if err = checkExpectedVersion(
		cart,
		expectedVersion,
	); err != nil {
		return Cart{}, err
	}

	if validated.Domain != cart.Domain {
		return Cart{}, ErrDomainConflict
	}

	if err = s.domainOpen(cart.Domain); err != nil {
		return Cart{}, err
	}

	if cart.Domain == DomainFood {
		if cart.FoodBranchID != "" &&
			(cart.FoodBranchID != validated.BranchID ||
				cart.FoodBusinessID != validated.BusinessID) {
			return Cart{}, ErrFoodBranchConflict
		}

		if cart.FoodBranchID == "" {
			if _, err = tx.Exec(
				ctx,
				`
                UPDATE commerce_carts
                SET food_business_id=$2,
                    food_branch_id=$3,
                    updated_at=now()
                WHERE id=$1
                `,
				cart.ID,
				validated.BusinessID,
				validated.BranchID,
			); err != nil {
				return Cart{}, err
			}

			cart.FoodBusinessID = validated.BusinessID
			cart.FoodBranchID = validated.BranchID
		}
	}

	itemID := newID("citem_")

	_, err = tx.Exec(
		ctx,
		`
        INSERT INTO commerce_cart_items(
            id,
            cart_id,
            domain,
            business_id,
            branch_id,
            item_ref,
            variant_ref,
            menu_ref,
            listing_ref,
            sell_unit_ref,
            size_ref,
            quantity,
            unit_price_minor_snapshot,
            currency_snapshot,
            price_reference,
            catalogue_revision,
            inventory_revision,
            display_name_snapshot,
            variant_name_snapshot,
            sell_unit_name_snapshot,
            requested_measure_ref,
            expected_measure_quantity,
            pricing_basis,
            logistics_profile_ref,
            estimated_weight_grams,
            temperature_zone,
            substitution_policy,
            version
        )
        VALUES(
            $1,$2,$3,$4,$5,$6,
            NULLIF($7,''),
            NULLIF($8,''),
            NULLIF($9,''),
            NULLIF($10,''),
            NULLIF($11,''),
            $12,$13,$14,$15,$16,$17,$18,$19,$20,
            NULLIF($21,''),
            NULLIF($22,0),
            $23,
            NULLIF($24,''),
            $25,
            NULLIF($26,''),
            $27,
            1
        )
        `,
		itemID,
		cart.ID,
		validated.Domain,
		validated.BusinessID,
		validated.BranchID,
		validated.ItemRef,
		validated.VariantRef,
		validated.MenuRef,
		validated.ListingRef,
		validated.SellUnitRef,
		validated.SizeRef,
		validated.Quantity,
		validated.UnitPriceMinor,
		validated.Currency,
		validated.PriceReference,
		validated.CatalogueRevision,
		validated.InventoryRevision,
		validated.DisplayName,
		validated.VariantName,
		validated.SellUnitName,
		validated.RequestedMeasureRef,
		validated.ExpectedMeasureQuantity,
		validated.PricingBasis,
		validated.LogisticsProfileRef,
		validated.EstimatedWeightGrams,
		validated.TemperatureZone,
		substitutionPolicy,
	)

	if err != nil {
		return Cart{}, err
	}

	for _, option := range validated.Options {
		_, err = tx.Exec(
			ctx,
			`
            INSERT INTO commerce_cart_item_options(
                id,
                cart_id,
                cart_item_id,
                group_ref,
                option_ref,
                quantity,
                group_name_snapshot,
                option_name_snapshot,
                price_delta_minor_snapshot,
                price_reference,
                catalogue_revision
            )
            VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
            `,
			newID("copt_"),
			cart.ID,
			itemID,
			option.GroupRef,
			option.OptionRef,
			option.Quantity,
			option.GroupName,
			option.OptionName,
			option.PriceDeltaMinor,
			option.PriceReference,
			option.CatalogueRevision,
		)

		if err != nil {
			return Cart{}, err
		}
	}

	nextVersion, err := bumpCartVersion(
		ctx,
		tx,
		cart.ID,
	)

	if err != nil {
		return Cart{}, err
	}

	if err = s.recordHistory(
		ctx,
		tx,
		cart.ID,
		customerID,
		"commerce.cart.item_added",
		itemID,
		"",
		nextVersion,
	); err != nil {
		return Cart{}, err
	}

	updated, err := loadOwnedCart(
		ctx,
		tx,
		customerID,
		cart.ID,
		false,
	)

	if err != nil {
		return Cart{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Cart{}, err
	}

	return updated, nil
}

func (s *Service) UpdateQuantity(
	ctx context.Context,
	customerID string,
	cartID string,
	itemID string,
	quantity int64,
	expectedVersion int64,
) (Cart, error) {
	customerID, err := validateCustomerID(customerID)

	if err != nil {
		return Cart{}, err
	}

	if quantity < 1 || quantity > MaxCartItemQuantity {
		return Cart{}, ErrInvalidQuantity
	}

	tx, err := s.store.db.Begin(ctx)

	if err != nil {
		return Cart{}, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	cart, err := loadOwnedCart(
		ctx,
		tx,
		customerID,
		cartID,
		true,
	)

	if err != nil {
		return Cart{}, err
	}

	if err = requireMutable(cart); err != nil {
		return Cart{}, err
	}

	if err = checkExpectedVersion(
		cart,
		expectedVersion,
	); err != nil {
		return Cart{}, err
	}

	tag, err := tx.Exec(
		ctx,
		`
        UPDATE commerce_cart_items
        SET quantity=$3,
            version=version+1,
            updated_at=now()
        WHERE id=$1
          AND cart_id=$2
        `,
		itemID,
		cart.ID,
		quantity,
	)

	if err != nil {
		return Cart{}, err
	}

	if tag.RowsAffected() != 1 {
		return Cart{}, ErrCartNotFound
	}

	nextVersion, err := bumpCartVersion(
		ctx,
		tx,
		cart.ID,
	)

	if err != nil {
		return Cart{}, err
	}

	if err = s.recordHistory(
		ctx,
		tx,
		cart.ID,
		customerID,
		"commerce.cart.item_updated",
		itemID,
		"",
		nextVersion,
	); err != nil {
		return Cart{}, err
	}

	updated, err := loadOwnedCart(
		ctx,
		tx,
		customerID,
		cart.ID,
		false,
	)

	if err != nil {
		return Cart{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Cart{}, err
	}

	return updated, nil
}

func (s *Service) RemoveItem(
	ctx context.Context,
	customerID string,
	cartID string,
	itemID string,
	expectedVersion int64,
) (Cart, error) {
	customerID, err := validateCustomerID(customerID)

	if err != nil {
		return Cart{}, err
	}

	tx, err := s.store.db.Begin(ctx)

	if err != nil {
		return Cart{}, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	cart, err := loadOwnedCart(
		ctx,
		tx,
		customerID,
		cartID,
		true,
	)

	if err != nil {
		return Cart{}, err
	}

	if err = requireMutable(cart); err != nil {
		return Cart{}, err
	}

	if err = checkExpectedVersion(
		cart,
		expectedVersion,
	); err != nil {
		return Cart{}, err
	}

	tag, err := tx.Exec(
		ctx,
		`
        DELETE FROM commerce_cart_items
        WHERE id=$1
          AND cart_id=$2
        `,
		itemID,
		cart.ID,
	)

	if err != nil {
		return Cart{}, err
	}

	if tag.RowsAffected() != 1 {
		return Cart{}, ErrCartNotFound
	}

	if cart.Domain == DomainFood {
		var remaining int64

		if err = tx.QueryRow(
			ctx,
			`
            SELECT count(*)
            FROM commerce_cart_items
            WHERE cart_id=$1
            `,
			cart.ID,
		).Scan(&remaining); err != nil {
			return Cart{}, err
		}

		if remaining == 0 {
			if _, err = tx.Exec(
				ctx,
				`
                UPDATE commerce_carts
                SET food_business_id=NULL,
                    food_branch_id=NULL,
                    updated_at=now()
                WHERE id=$1
                `,
				cart.ID,
			); err != nil {
				return Cart{}, err
			}
		}
	}

	nextVersion, err := bumpCartVersion(
		ctx,
		tx,
		cart.ID,
	)

	if err != nil {
		return Cart{}, err
	}

	if err = s.recordHistory(
		ctx,
		tx,
		cart.ID,
		customerID,
		"commerce.cart.item_removed",
		itemID,
		"",
		nextVersion,
	); err != nil {
		return Cart{}, err
	}

	updated, err := loadOwnedCart(
		ctx,
		tx,
		customerID,
		cart.ID,
		false,
	)

	if err != nil {
		return Cart{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Cart{}, err
	}

	return updated, nil
}

func (s *Service) ClearCart(
	ctx context.Context,
	customerID string,
	cartID string,
	expectedVersion int64,
) (Cart, error) {
	customerID, err := validateCustomerID(customerID)

	if err != nil {
		return Cart{}, err
	}

	tx, err := s.store.db.Begin(ctx)

	if err != nil {
		return Cart{}, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	cart, err := loadOwnedCart(
		ctx,
		tx,
		customerID,
		cartID,
		true,
	)

	if err != nil {
		return Cart{}, err
	}

	if err = requireMutable(cart); err != nil {
		return Cart{}, err
	}

	if err = checkExpectedVersion(
		cart,
		expectedVersion,
	); err != nil {
		return Cart{}, err
	}

	if _, err = tx.Exec(
		ctx,
		`
        DELETE FROM commerce_cart_items
        WHERE cart_id=$1
        `,
		cart.ID,
	); err != nil {
		return Cart{}, err
	}

	if _, err = tx.Exec(
		ctx,
		`
        UPDATE commerce_carts
        SET food_business_id=NULL,
            food_branch_id=NULL,
            updated_at=now()
        WHERE id=$1
        `,
		cart.ID,
	); err != nil {
		return Cart{}, err
	}

	nextVersion, err := bumpCartVersion(
		ctx,
		tx,
		cart.ID,
	)

	if err != nil {
		return Cart{}, err
	}

	if err = s.recordHistory(
		ctx,
		tx,
		cart.ID,
		customerID,
		"commerce.cart.cleared",
		"",
		"",
		nextVersion,
	); err != nil {
		return Cart{}, err
	}

	updated, err := loadOwnedCart(
		ctx,
		tx,
		customerID,
		cart.ID,
		false,
	)

	if err != nil {
		return Cart{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Cart{}, err
	}

	return updated, nil
}

// SetDeliveryAddress resolves the address through Customer Service authority.
//
// No request body may provide the authoritative customer ID or the address
// snapshot itself.
func (s *Service) SetDeliveryAddress(
	ctx context.Context,
	customerID string,
	cartID string,
	addressRef string,
	expectedVersion int64,
) (Cart, error) {
	customerID, err := validateCustomerID(customerID)

	if err != nil {
		return Cart{}, err
	}

	if s.addresses == nil {
		return Cart{}, ErrDependencyMissing
	}

	addressRef = strings.TrimSpace(addressRef)

	if addressRef == "" {
		return Cart{}, ErrInvalidAddress
	}

	snapshot, err := s.addresses.ResolveSavedAddress(
		ctx,
		customerID,
		addressRef,
	)

	if err != nil {
		return Cart{}, err
	}

	snapshot, err = ValidateAddressSnapshot(snapshot)

	if err != nil {
		return Cart{}, err
	}

	if snapshot.SourceAddressRef != addressRef {
		return Cart{}, ErrInvalidAddress
	}

	if snapshot.SnapshottedAt.IsZero() {
		snapshot.SnapshottedAt = s.now()
	}

	tx, err := s.store.db.Begin(ctx)

	if err != nil {
		return Cart{}, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	cart, err := loadOwnedCart(
		ctx,
		tx,
		customerID,
		cartID,
		true,
	)

	if err != nil {
		return Cart{}, err
	}

	if err = requireMutable(cart); err != nil {
		return Cart{}, err
	}

	if err = checkExpectedVersion(
		cart,
		expectedVersion,
	); err != nil {
		return Cart{}, err
	}

	_, err = tx.Exec(
		ctx,
		`
        INSERT INTO commerce_cart_addresses(
            cart_id,
            source_address_ref,
            title_snapshot,
            address_snapshot,
            latitude_snapshot,
            longitude_snapshot,
            description_snapshot,
            snapshotted_at
        )
        VALUES($1,$2,$3,$4,$5,$6,$7,$8)
        ON CONFLICT (cart_id)
        DO UPDATE SET
            source_address_ref=EXCLUDED.source_address_ref,
            title_snapshot=EXCLUDED.title_snapshot,
            address_snapshot=EXCLUDED.address_snapshot,
            latitude_snapshot=EXCLUDED.latitude_snapshot,
            longitude_snapshot=EXCLUDED.longitude_snapshot,
            description_snapshot=EXCLUDED.description_snapshot,
            snapshotted_at=EXCLUDED.snapshotted_at,
            updated_at=now()
        `,
		cart.ID,
		snapshot.SourceAddressRef,
		snapshot.Title,
		snapshot.Address,
		snapshot.Latitude,
		snapshot.Longitude,
		snapshot.Description,
		snapshot.SnapshottedAt,
	)

	if err != nil {
		return Cart{}, err
	}

	nextVersion, err := bumpCartVersion(
		ctx,
		tx,
		cart.ID,
	)

	if err != nil {
		return Cart{}, err
	}

	if err = s.recordHistory(
		ctx,
		tx,
		cart.ID,
		customerID,
		"commerce.cart.address_changed",
		"",
		snapshot.SourceAddressRef,
		nextVersion,
	); err != nil {
		return Cart{}, err
	}

	updated, err := loadOwnedCart(
		ctx,
		tx,
		customerID,
		cart.ID,
		false,
	)

	if err != nil {
		return Cart{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Cart{}, err
	}

	return updated, nil
}

func (s *Service) RemoveDeliveryAddress(
	ctx context.Context,
	customerID string,
	cartID string,
	expectedVersion int64,
) (Cart, error) {
	return s.removeOwnedAuxiliary(
		ctx,
		customerID,
		cartID,
		expectedVersion,
		`
        DELETE FROM commerce_cart_addresses
        WHERE cart_id=$1
        `,
		"commerce.cart.address_removed",
	)
}

func (s *Service) SetReceiver(
	ctx context.Context,
	customerID string,
	cartID string,
	receiver Receiver,
	expectedVersion int64,
) (Cart, error) {
	customerID, err := validateCustomerID(customerID)

	if err != nil {
		return Cart{}, err
	}

	receiver, err = ValidateReceiver(receiver)

	if err != nil {
		return Cart{}, err
	}

	tx, err := s.store.db.Begin(ctx)

	if err != nil {
		return Cart{}, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	cart, err := loadOwnedCart(
		ctx,
		tx,
		customerID,
		cartID,
		true,
	)

	if err != nil {
		return Cart{}, err
	}

	if err = requireMutable(cart); err != nil {
		return Cart{}, err
	}

	if err = checkExpectedVersion(
		cart,
		expectedVersion,
	); err != nil {
		return Cart{}, err
	}

	_, err = tx.Exec(
		ctx,
		`
        INSERT INTO commerce_cart_receivers(
            cart_id,
            receiver_name,
            receiver_phone,
            receiver_customer_id
        )
        VALUES($1,$2,$3,NULLIF($4,'')::uuid)
        ON CONFLICT (cart_id)
        DO UPDATE SET
            receiver_name=EXCLUDED.receiver_name,
            receiver_phone=EXCLUDED.receiver_phone,
            receiver_customer_id=EXCLUDED.receiver_customer_id,
            updated_at=now()
        `,
		cart.ID,
		receiver.Name,
		receiver.Phone,
		receiver.CustomerID,
	)

	if err != nil {
		return Cart{}, err
	}

	nextVersion, err := bumpCartVersion(
		ctx,
		tx,
		cart.ID,
	)

	if err != nil {
		return Cart{}, err
	}

	if err = s.recordHistory(
		ctx,
		tx,
		cart.ID,
		customerID,
		"commerce.cart.receiver_changed",
		"",
		"",
		nextVersion,
	); err != nil {
		return Cart{}, err
	}

	updated, err := loadOwnedCart(
		ctx,
		tx,
		customerID,
		cart.ID,
		false,
	)

	if err != nil {
		return Cart{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Cart{}, err
	}

	return updated, nil
}

func (s *Service) RemoveReceiver(
	ctx context.Context,
	customerID string,
	cartID string,
	expectedVersion int64,
) (Cart, error) {
	return s.removeOwnedAuxiliary(
		ctx,
		customerID,
		cartID,
		expectedVersion,
		`
        DELETE FROM commerce_cart_receivers
        WHERE cart_id=$1
        `,
		"commerce.cart.receiver_removed",
	)
}

func (s *Service) removeOwnedAuxiliary(
	ctx context.Context,
	customerID string,
	cartID string,
	expectedVersion int64,
	deleteSQL string,
	action string,
) (Cart, error) {
	customerID, err := validateCustomerID(customerID)

	if err != nil {
		return Cart{}, err
	}

	tx, err := s.store.db.Begin(ctx)

	if err != nil {
		return Cart{}, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	cart, err := loadOwnedCart(
		ctx,
		tx,
		customerID,
		cartID,
		true,
	)

	if err != nil {
		return Cart{}, err
	}

	if err = requireMutable(cart); err != nil {
		return Cart{}, err
	}

	if err = checkExpectedVersion(
		cart,
		expectedVersion,
	); err != nil {
		return Cart{}, err
	}

	if _, err = tx.Exec(
		ctx,
		deleteSQL,
		cart.ID,
	); err != nil {
		return Cart{}, err
	}

	nextVersion, err := bumpCartVersion(
		ctx,
		tx,
		cart.ID,
	)

	if err != nil {
		return Cart{}, err
	}

	if err = s.recordHistory(
		ctx,
		tx,
		cart.ID,
		customerID,
		action,
		"",
		"",
		nextVersion,
	); err != nil {
		return Cart{}, err
	}

	updated, err := loadOwnedCart(
		ctx,
		tx,
		customerID,
		cart.ID,
		false,
	)

	if err != nil {
		return Cart{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Cart{}, err
	}

	return updated, nil
}

func (s *Service) AbandonCart(
	ctx context.Context,
	customerID string,
	cartID string,
	expectedVersion int64,
) (Cart, error) {
	customerID, err := validateCustomerID(customerID)

	if err != nil {
		return Cart{}, err
	}

	tx, err := s.store.db.Begin(ctx)

	if err != nil {
		return Cart{}, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	cart, err := loadOwnedCart(
		ctx,
		tx,
		customerID,
		cartID,
		true,
	)

	if err != nil {
		return Cart{}, err
	}

	if err = requireMutable(cart); err != nil {
		return Cart{}, err
	}

	if err = checkExpectedVersion(
		cart,
		expectedVersion,
	); err != nil {
		return Cart{}, err
	}

	nextVersion := cart.Version + 1

	_, err = tx.Exec(
		ctx,
		`
        UPDATE commerce_carts
        SET status='ABANDONED',
            version=$2,
            updated_at=now()
        WHERE id=$1
        `,
		cart.ID,
		nextVersion,
	)

	if err != nil {
		return Cart{}, err
	}

	if err = s.recordHistory(
		ctx,
		tx,
		cart.ID,
		customerID,
		"commerce.cart.abandoned",
		"",
		"",
		nextVersion,
	); err != nil {
		return Cart{}, err
	}

	updated, err := loadOwnedCart(
		ctx,
		tx,
		customerID,
		cart.ID,
		false,
	)

	if err != nil {
		return Cart{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Cart{}, err
	}

	return updated, nil
}

func bumpCartVersion(
	ctx context.Context,
	tx pgx.Tx,
	cartID string,
) (int64, error) {
	var version int64

	err := tx.QueryRow(
		ctx,
		`
        UPDATE commerce_carts
        SET version=version+1,
            updated_at=now()
        WHERE id=$1
        RETURNING version
        `,
		cartID,
	).Scan(&version)

	if err != nil {
		return 0, fmt.Errorf(
			"bump commerce cart version: %w",
			err,
		)
	}

	return version, nil
}

func appendHistory(
	ctx context.Context,
	tx pgx.Tx,
	cartID string,
	customerID string,
	action string,
	itemID string,
	addressRef string,
	cartVersion int64,
) error {
	_, err := tx.Exec(
		ctx,
		`
        INSERT INTO commerce_cart_history(
            id,
            cart_id,
            customer_id,
            action,
            item_id,
            address_ref,
            cart_version
        )
        VALUES(
            $1,$2,$3,$4,
            NULLIF($5,''),
            NULLIF($6,''),
            $7
        )
        `,
		newID("chist_"),
		cartID,
		customerID,
		action,
		itemID,
		addressRef,
		cartVersion,
	)

	return err
}
