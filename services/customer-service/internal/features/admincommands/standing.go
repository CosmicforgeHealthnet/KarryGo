package admincommands

import (
	"context"
	"errors"

	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/adminowner"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StandingReader exposes the customer's administrative standing to the rest
// of customer-service (auth login/refresh and the internal customer context)
// as plain facts. It implements authusecases.AccountStanding and
// contextusecases.StandingReader without either package importing this one.
type StandingReader struct{ DB *pgxpool.Pool }

func (r StandingReader) read(ctx context.Context, customerID string) (string, int64, error) {
	if _, err := uuid.Parse(customerID); err != nil {
		return RestrictionNone, 0, nil
	}
	var restriction string
	var version int64
	err := r.DB.QueryRow(ctx, `SELECT restriction,version FROM customer_account_standing WHERE customer_id=$1`, customerID).Scan(&restriction, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return RestrictionNone, 0, nil
	}
	return restriction, version, err
}

// LoginBlocked reports whether the customer's standing blocks login/refresh.
func (r StandingReader) LoginBlocked(ctx context.Context, customerID string) (bool, error) {
	restriction, _, err := r.read(ctx, customerID)
	return restriction == RestrictionLogin, err
}

// AccountStanding returns the current restriction token and standing version.
func (r StandingReader) AccountStanding(ctx context.Context, customerID string) (string, int64, error) {
	return r.read(ctx, customerID)
}

// Runtime composes the customer command boundary on the already initialized
// source-owner handler. CUSTOMER_ADMIN_COMMAND_WRITES_ENABLED and
// CUSTOMER_ADMIN_COMMAND_READBACK_ENABLED both default off.
func Runtime(owner *adminowner.Handler) (*admincommand.Handler, error) {
	return admincommand.Runtime(owner, Spec(), Executor{}, Prefix)
}
