package checkout

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type queryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Store is Section 17B persistence. It owns only commerce_checkout_* tables;
// the cart tables belong to Section 17A and Inventory tables to Section 14.
type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

// DB exposes the pool for the transaction-scoped paths.
func (s *Store) DB() *pgxpool.Pool {
	return s.db
}

func newID(prefix string) string {
	return prefix + uuid.NewString()
}

const sessionColumns = `
    id,
    customer_id,
    cart_id,
    cart_version,
    domain,
    status,
    validation_revision,
    address_ref,
    address_snapshotted_at,
    currency,
    quoted_items_total_minor,
    max_authorized_items_total_minor,
    price_changed,
    catalogue_changed,
    failure_code,
    reservation_ttl_seconds,
    expires_at,
    reservation_coordination_state,
    version,
    created_at,
    updated_at,
    validated_at,
    settled_at
`

func scanSession(row pgx.Row) (Session, error) {
	var session Session

	err := row.Scan(
		&session.ID,
		&session.CustomerID,
		&session.CartID,
		&session.CartVersion,
		&session.Domain,
		&session.Status,
		&session.ValidationRevision,
		&session.AddressRef,
		&session.AddressSnapshottedAt,
		&session.Currency,
		&session.QuotedItemsTotalMinor,
		&session.MaxAuthorizedItemsTotalMinor,
		&session.PriceChanged,
		&session.CatalogueChanged,
		&session.FailureCode,
		&session.ReservationTTLSeconds,
		&session.ExpiresAt,
		&session.ReservationCoordinationState,
		&session.Version,
		&session.CreatedAt,
		&session.UpdatedAt,
		&session.ValidatedAt,
		&session.SettledAt,
	)

	return session, err
}

// loadOwnedSession reads one customer-owned session.
//
// customerID is always a separate server-side argument resolved from the
// verified Customer Service bearer token. There is no request field anywhere
// in Section 17B that can name a different customer.
func loadOwnedSession(
	ctx context.Context,
	q queryer,
	customerID string,
	sessionID string,
	forUpdate bool,
) (Session, error) {
	sql := `
        SELECT ` + sessionColumns + `
        FROM commerce_checkout_sessions
        WHERE id=$1
          AND customer_id=$2
    `

	if forUpdate {
		sql += ` FOR UPDATE`
	}

	session, err := scanSession(
		q.QueryRow(ctx, sql, sessionID, customerID),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrSessionNotFound
	}

	if err != nil {
		return Session{}, err
	}

	if err = loadSessionChildren(ctx, q, &session); err != nil {
		return Session{}, err
	}

	return session, nil
}

// loadLiveSessionForCart reads the at-most-one live session a cart owns.
func loadLiveSessionForCart(
	ctx context.Context,
	q queryer,
	cartID string,
	forUpdate bool,
) (Session, error) {
	sql := `
        SELECT ` + sessionColumns + `
        FROM commerce_checkout_sessions
        WHERE cart_id=$1
          AND status IN ('VALIDATING','VALIDATED')
    `

	if forUpdate {
		sql += ` FOR UPDATE`
	}

	session, err := scanSession(
		q.QueryRow(ctx, sql, cartID),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrSessionNotFound
	}

	if err != nil {
		return Session{}, err
	}

	if err = loadSessionChildren(ctx, q, &session); err != nil {
		return Session{}, err
	}

	return session, nil
}

func loadSessionChildren(
	ctx context.Context,
	q queryer,
	session *Session,
) error {
	groups, err := loadGroups(ctx, q, session.ID)

	if err != nil {
		return err
	}

	lines, err := loadLines(ctx, q, session.ID)

	if err != nil {
		return err
	}

	byGroup := make(map[string][]Line, len(groups))

	for _, line := range lines {
		byGroup[line.GroupID] = append(byGroup[line.GroupID], line)
	}

	for index := range groups {
		groups[index].Lines = byGroup[groups[index].ID]
	}

	session.Groups = groups

	return nil
}

const groupColumns = `
    id,
    domain,
    business_id,
    branch_id,
    requires_stock_reservation,
    reservation_status,
    reservation_source_ref,
    inventory_reservation_ref,
    reservation_expires_at,
    reservation_failure_code,
    catalogue_revision,
    inventory_revision,
    currency,
    quoted_total_minor,
    max_authorized_total_minor
`

func loadGroups(
	ctx context.Context,
	q queryer,
	sessionID string,
) ([]Group, error) {
	rows, err := q.Query(
		ctx,
		`
        SELECT `+groupColumns+`
        FROM commerce_checkout_groups
        WHERE session_id=$1
        ORDER BY business_id, branch_id, id
        `,
		sessionID,
	)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	groups := make([]Group, 0, 4)

	for rows.Next() {
		var group Group

		if err = rows.Scan(
			&group.ID,
			&group.Domain,
			&group.BusinessID,
			&group.BranchID,
			&group.RequiresStockReservation,
			&group.ReservationStatus,
			&group.ReservationSourceRef,
			&group.InventoryReservationRef,
			&group.ReservationExpiresAt,
			&group.ReservationFailureCode,
			&group.CatalogueRevision,
			&group.InventoryRevision,
			&group.Currency,
			&group.QuotedTotalMinor,
			&group.MaxAuthorizedTotalMinor,
		); err != nil {
			return nil, err
		}

		group.Lines = []Line{}

		groups = append(groups, group)
	}

	return groups, rows.Err()
}

const lineColumns = `
    id,
    COALESCE(group_id,''),
    cart_item_id,
    domain,
    business_id,
    branch_id,
    item_ref,
    variant_ref,
    menu_ref,
    listing_ref,
    sell_unit_ref,
    quantity,
    display_name,
    unit_price_minor,
    currency,
    price_reference,
    catalogue_revision,
    inventory_revision,
    snapshot_unit_price_minor,
    snapshot_price_reference,
    snapshot_catalogue_revision,
    price_changed,
    catalogue_changed,
    pricing_basis,
    requested_measure_ref,
    expected_measure_quantity,
    measure_underfill_bps,
    measure_overfill_bps,
    quoted_total_minor,
    max_authorized_total_minor,
    inventory_item_ref,
    reservation_quantity,
    reservation_basis,
    validation_status,
    reason_code,
    reason_detail
`

func loadLines(
	ctx context.Context,
	q queryer,
	sessionID string,
) ([]Line, error) {
	rows, err := q.Query(
		ctx,
		`
        SELECT `+lineColumns+`
        FROM commerce_checkout_lines
        WHERE session_id=$1
        ORDER BY business_id, branch_id, id
        `,
		sessionID,
	)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	lines := make([]Line, 0, 8)

	for rows.Next() {
		var line Line

		if err = rows.Scan(
			&line.ID,
			&line.GroupID,
			&line.CartItemID,
			&line.Domain,
			&line.BusinessID,
			&line.BranchID,
			&line.ItemRef,
			&line.VariantRef,
			&line.MenuRef,
			&line.ListingRef,
			&line.SellUnitRef,
			&line.Quantity,
			&line.DisplayName,
			&line.UnitPriceMinor,
			&line.Currency,
			&line.PriceReference,
			&line.CatalogueRevision,
			&line.InventoryRevision,
			&line.SnapshotUnitPriceMinor,
			&line.SnapshotPriceReference,
			&line.SnapshotCatalogueRevision,
			&line.PriceChanged,
			&line.CatalogueChanged,
			&line.PricingBasis,
			&line.RequestedMeasureRef,
			&line.ExpectedMeasureQuantity,
			&line.MeasureUnderfillBps,
			&line.MeasureOverfillBps,
			&line.QuotedTotalMinor,
			&line.MaxAuthorizedTotalMinor,
			&line.InventoryItemRef,
			&line.ReservationQuantity,
			&line.ReservationBasis,
			&line.Status,
			&line.ReasonCode,
			&line.ReasonDetail,
		); err != nil {
			return nil, err
		}

		lines = append(lines, line)
	}

	return lines, rows.Err()
}

// nextValidationRevision reads the next per-cart validation attempt number.
//
// It runs under the cart's row lock, so two concurrent checkouts of one cart
// cannot claim the same revision.
func nextValidationRevision(
	ctx context.Context,
	q queryer,
	cartID string,
) (int64, error) {
	var revision int64

	err := q.QueryRow(
		ctx,
		`
        SELECT COALESCE(MAX(validation_revision),0)+1
        FROM commerce_checkout_sessions
        WHERE cart_id=$1
        `,
		cartID,
	).Scan(&revision)

	return revision, err
}

func insertSession(
	ctx context.Context,
	tx pgx.Tx,
	session Session,
) error {
	_, err := tx.Exec(
		ctx,
		`
        INSERT INTO commerce_checkout_sessions(
            id,
            customer_id,
            cart_id,
            cart_version,
            domain,
            status,
            validation_revision,
            address_ref,
            address_snapshotted_at,
            currency,
            quoted_items_total_minor,
            max_authorized_items_total_minor,
            price_changed,
            catalogue_changed,
            failure_code,
            reservation_ttl_seconds,
            expires_at,
            reservation_coordination_state,
            version,
            validated_at,
            settled_at
        )
        VALUES(
            $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,
            $11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21
        )
        `,
		session.ID,
		session.CustomerID,
		session.CartID,
		session.CartVersion,
		session.Domain,
		session.Status,
		session.ValidationRevision,
		session.AddressRef,
		session.AddressSnapshottedAt,
		session.Currency,
		session.QuotedItemsTotalMinor,
		session.MaxAuthorizedItemsTotalMinor,
		session.PriceChanged,
		session.CatalogueChanged,
		session.FailureCode,
		session.ReservationTTLSeconds,
		session.ExpiresAt,
		session.ReservationCoordinationState,
		session.Version,
		session.ValidatedAt,
		session.SettledAt,
	)

	return err
}

func insertGroup(
	ctx context.Context,
	tx pgx.Tx,
	sessionID string,
	customerID string,
	group Group,
) error {
	_, err := tx.Exec(
		ctx,
		`
        INSERT INTO commerce_checkout_groups(
            id,
            session_id,
            customer_id,
            domain,
            business_id,
            branch_id,
            requires_stock_reservation,
            reservation_status,
            reservation_source_ref,
            inventory_reservation_ref,
            reservation_expires_at,
            reservation_failure_code,
            catalogue_revision,
            inventory_revision,
            currency,
            quoted_total_minor,
            max_authorized_total_minor
        )
        VALUES(
            $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,
            $11,$12,$13,$14,$15,$16,$17
        )
        `,
		group.ID,
		sessionID,
		customerID,
		group.Domain,
		group.BusinessID,
		group.BranchID,
		group.RequiresStockReservation,
		group.ReservationStatus,
		group.ReservationSourceRef,
		group.InventoryReservationRef,
		group.ReservationExpiresAt,
		group.ReservationFailureCode,
		group.CatalogueRevision,
		group.InventoryRevision,
		group.Currency,
		group.QuotedTotalMinor,
		group.MaxAuthorizedTotalMinor,
	)

	return err
}

func insertLine(
	ctx context.Context,
	tx pgx.Tx,
	sessionID string,
	customerID string,
	line Line,
) error {
	var groupID any

	if line.GroupID != "" {
		groupID = line.GroupID
	}

	_, err := tx.Exec(
		ctx,
		`
        INSERT INTO commerce_checkout_lines(
            id,
            session_id,
            group_id,
            customer_id,
            cart_item_id,
            domain,
            business_id,
            branch_id,
            item_ref,
            variant_ref,
            menu_ref,
            listing_ref,
            sell_unit_ref,
            quantity,
            display_name,
            unit_price_minor,
            currency,
            price_reference,
            catalogue_revision,
            inventory_revision,
            snapshot_unit_price_minor,
            snapshot_price_reference,
            snapshot_catalogue_revision,
            price_changed,
            catalogue_changed,
            pricing_basis,
            requested_measure_ref,
            expected_measure_quantity,
            measure_underfill_bps,
            measure_overfill_bps,
            quoted_total_minor,
            max_authorized_total_minor,
            inventory_item_ref,
            reservation_quantity,
            reservation_basis,
            validation_status,
            reason_code,
            reason_detail
        )
        VALUES(
            $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,
            $11,$12,$13,$14,$15,$16,$17,$18,$19,$20,
            $21,$22,$23,$24,$25,$26,$27,$28,$29,$30,
            $31,$32,$33,$34,$35,$36,$37,$38
        )
        `,
		line.ID,
		sessionID,
		groupID,
		customerID,
		line.CartItemID,
		line.Domain,
		line.BusinessID,
		line.BranchID,
		line.ItemRef,
		line.VariantRef,
		line.MenuRef,
		line.ListingRef,
		line.SellUnitRef,
		line.Quantity,
		line.DisplayName,
		line.UnitPriceMinor,
		line.Currency,
		line.PriceReference,
		line.CatalogueRevision,
		line.InventoryRevision,
		line.SnapshotUnitPriceMinor,
		line.SnapshotPriceReference,
		line.SnapshotCatalogueRevision,
		line.PriceChanged,
		line.CatalogueChanged,
		line.PricingBasis,
		line.RequestedMeasureRef,
		line.ExpectedMeasureQuantity,
		line.MeasureUnderfillBps,
		line.MeasureOverfillBps,
		line.QuotedTotalMinor,
		line.MaxAuthorizedTotalMinor,
		line.InventoryItemRef,
		line.ReservationQuantity,
		line.ReservationBasis,
		line.Status,
		line.ReasonCode,
		line.ReasonDetail,
	)

	return err
}

func appendEvent(
	ctx context.Context,
	tx pgx.Tx,
	sessionID string,
	customerID string,
	action string,
	groupID string,
	detail string,
	sessionVersion int64,
) error {
	_, err := tx.Exec(
		ctx,
		`
        INSERT INTO commerce_checkout_events(
            id,
            session_id,
            customer_id,
            action,
            group_id,
            detail,
            session_version
        )
        VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7)
        `,
		newID("chkev_"),
		sessionID,
		customerID,
		action,
		groupID,
		detail,
		sessionVersion,
	)

	return err
}

// bumpSessionVersion advances the session's optimistic version.
func bumpSessionVersion(
	ctx context.Context,
	tx pgx.Tx,
	sessionID string,
) (int64, error) {
	var version int64

	err := tx.QueryRow(
		ctx,
		`
        UPDATE commerce_checkout_sessions
        SET version=version+1,
            updated_at=now()
        WHERE id=$1
        RETURNING version
        `,
		sessionID,
	).Scan(&version)

	return version, err
}
