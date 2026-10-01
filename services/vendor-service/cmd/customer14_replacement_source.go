package main

import (
	"context"
	"errors"
	"time"

	"cosmicforge/logistics/services/vendor-service/internal/features/checkout"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkoutquote"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
	"github.com/jackc/pgx/v5/pgxpool"
)

type customer14ReplacementSource struct {
	db       *pgxpool.Pool
	checkout *checkout.Service
	planning *fulfilment.PlanningService
}

func (a customer14ReplacementSource) PrepareReplacement(ctx context.Context, customer, session, plan, cartItem, item, variant, listing, sellUnit string, price int64) (string, string, error) {
	if a.db == nil || a.checkout == nil || a.planning == nil {
		return "", "", checkoutquote.ErrDependencyMissing
	}
	// Release losing connections while waiting, so duplicate requests cannot
	// exhaust the pool needed by the request performing authority calls.
	var lease *pgxpool.Conn
	for {
		conn, err := a.db.Acquire(ctx)
		if err != nil {
			return "", "", err
		}
		var acquired bool
		err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1,14124))`, session).Scan(&acquired)
		if err != nil {
			conn.Release()
			return "", "", err
		}
		if acquired {
			lease = conn
			break
		}
		conn.Release()
		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	defer func() {
		_, _ = lease.Exec(context.Background(), `SELECT pg_advisory_unlock(hashtextextended($1,14124))`, session)
		lease.Release()
	}()
	original, err := a.planning.Get(ctx, customer, plan)
	if err != nil {
		return "", "", err
	}
	if original.Status != fulfilment.StatusPlanned || original.CheckoutSessionID != session {
		return "", "", checkoutquote.ErrSourceNotQuotable
	}
	fresh, err := a.checkout.ValidateReplacement(ctx, customer, session, checkout.ReplacementLine{
		CartItemID: cartItem, ItemRef: item, VariantRef: variant, ListingRef: listing, SellUnitRef: sellUnit, UnitPriceMinor: price})
	if err != nil {
		return "", "", err
	}
	successor, err := a.planning.GetForCheckout(ctx, customer, fresh.ID)
	if errors.Is(err, fulfilment.ErrPlanNotFound) {
		successor, err = a.planning.PlanFor(ctx, customer, fresh.ID, original.Mode)
	}
	if err != nil {
		return "", "", err
	}
	if successor.Status != fulfilment.StatusPlanned {
		return "", "", checkoutquote.ErrSourceNotQuotable
	}
	return fresh.ID, successor.ID, nil
}

var _ checkoutquote.ReplacementSourceAuthority = customer14ReplacementSource{}
