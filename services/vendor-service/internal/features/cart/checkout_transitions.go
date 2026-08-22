package cart

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Section 17B cart-side transitions.
//
// Cart status is cart-owned state, so the ACTIVE <-> CHECKOUT_IN_PROGRESS
// transition lives here rather than in the checkout feature. The helpers are
// transaction-scoped on purpose: Section 17B has to lock the cart, decide, and
// move it in one atomic step, and a pool-scoped method could not be part of
// that decision.
//
// Section 17A already made CHECKOUT_IN_PROGRESS immutable to cart commands and
// still "live" for the one-live-cart-per-domain index. These helpers are what
// finally put a cart into that state.

const (
	// HistoryCheckoutStarted records the ACTIVE -> CHECKOUT_IN_PROGRESS move.
	HistoryCheckoutStarted = "commerce.cart.checkout_started"

	// HistoryCheckoutReleased records the CHECKOUT_IN_PROGRESS -> ACTIVE move
	// made when a checkout session fails, expires or is cancelled.
	HistoryCheckoutReleased = "commerce.cart.checkout_released"
)

var (
	// ErrCheckoutAlreadyInProgress means the cart is already owned by a live
	// checkout session.
	ErrCheckoutAlreadyInProgress = errors.New(
		"commerce cart already has a checkout in progress",
	)

	// ErrCartEmpty means there is nothing to check out.
	ErrCartEmpty = errors.New("commerce cart is empty")
)

// Queryer is the read surface Section 17B needs in order to read a cart from
// either the pool or its own transaction.
type Queryer interface {
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

// ReadOwnedCart loads one customer-owned cart without taking a row lock.
//
// Section 17B revalidates against this read first and only then takes the
// lock, so the authoritative Food/Shop/Inventory reads never happen while a
// cart row lock is held. The lock path re-asserts the version, so nothing
// decided against this unlocked read is trusted.
func ReadOwnedCart(
	ctx context.Context,
	q Queryer,
	customerID string,
	cartID string,
) (Cart, error) {
	customerID, err := validateCustomerID(customerID)

	if err != nil {
		return Cart{}, err
	}

	return loadOwnedCart(
		ctx,
		q,
		customerID,
		cartID,
		false,
	)
}

// LockOwnedCartForCheckoutTx loads one customer-owned cart under FOR UPDATE.
//
// The lock is what serialises Section 17B against every Section 17A mutation:
// each cart command takes the same row lock before it changes anything.
func LockOwnedCartForCheckoutTx(
	ctx context.Context,
	tx pgx.Tx,
	customerID string,
	cartID string,
) (Cart, error) {
	customerID, err := validateCustomerID(customerID)

	if err != nil {
		return Cart{}, err
	}

	return loadOwnedCart(
		ctx,
		tx,
		customerID,
		cartID,
		true,
	)
}

// BeginCheckoutTx moves an ACTIVE cart to CHECKOUT_IN_PROGRESS.
//
// It re-asserts ownership, mutability, the expected version and a non-empty
// cart under the caller's lock, so nothing decided before the lock is trusted.
func BeginCheckoutTx(
	ctx context.Context,
	tx pgx.Tx,
	cart Cart,
	expectedVersion int64,
) (int64, error) {
	if cart.Status == StatusCheckoutInProgress {
		return 0, ErrCheckoutAlreadyInProgress
	}

	if err := requireMutable(cart); err != nil {
		return 0, err
	}

	if err := checkExpectedVersion(
		cart,
		expectedVersion,
	); err != nil {
		return 0, err
	}

	if len(cart.Items) == 0 {
		return 0, ErrCartEmpty
	}

	var version int64

	err := tx.QueryRow(
		ctx,
		`
        UPDATE commerce_carts
        SET status='CHECKOUT_IN_PROGRESS',
            version=version+1,
            updated_at=now()
        WHERE id=$1
          AND customer_id=$2
          AND status='ACTIVE'
          AND version=$3
        RETURNING version
        `,
		cart.ID,
		cart.CustomerID,
		expectedVersion,
	).Scan(&version)

	if errors.Is(err, pgx.ErrNoRows) {
		// The guard refused. Under the row lock this can only mean the cart
		// moved between the read and the write, which is a conflict, never a
		// silent no-op.
		return 0, VersionConflictError{
			Expected: expectedVersion,
			Actual:   cart.Version,
		}
	}

	if err != nil {
		return 0, err
	}

	if err = appendHistory(
		ctx,
		tx,
		cart.ID,
		cart.CustomerID,
		HistoryCheckoutStarted,
		"",
		"",
		version,
	); err != nil {
		return 0, err
	}

	return version, nil
}

// ReleaseCheckoutTx returns a CHECKOUT_IN_PROGRESS cart to ACTIVE.
//
// It is deliberately idempotent: a cart that is not in checkout is left
// exactly as it is and no error is raised, because every caller
// (cancellation, validation failure, TTL sweep, crash recovery) may legally
// race another one and none of them may wedge a customer's cart.
func ReleaseCheckoutTx(
	ctx context.Context,
	tx pgx.Tx,
	customerID string,
	cartID string,
) (bool, error) {
	var version int64

	err := tx.QueryRow(
		ctx,
		`
        UPDATE commerce_carts
        SET status='ACTIVE',
            version=version+1,
            updated_at=now()
        WHERE id=$1
          AND customer_id=$2
          AND status='CHECKOUT_IN_PROGRESS'
        RETURNING version
        `,
		cartID,
		customerID,
	).Scan(&version)

	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	if err = appendHistory(
		ctx,
		tx,
		cartID,
		customerID,
		HistoryCheckoutReleased,
		"",
		"",
		version,
	); err != nil {
		return false, err
	}

	return true, nil
}
