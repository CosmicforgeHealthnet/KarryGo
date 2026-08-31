package main

// Basket optimization composition root.
//
// Notably absent from everything wired below: Wallet, orders, payments,
// inventory reservation and the operational fulfilment aggregate. That is not
// an oversight - it is what makes "evaluating a hundred candidate baskets
// creates zero missions and zero payment intents" structural rather than a
// promise. A dependency wired here would be the first step towards breaking
// it, and there is nothing here to wire.
//
// The one logistics dependency is the QUOTE-ONLY planner, handed over as a
// narrow port so the optimizer cannot reach the planning service's persisting
// side even by accident.

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/config"
	"cosmicforge/logistics/services/vendor-service/internal/features/basket"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
	"cosmicforge/logistics/services/vendor-service/internal/jobs"
	sharedauth "cosmicforge/logistics/shared/go/auth"
)

// basketQuoter adapts the fulfilment planner's quote-only entry point.
//
// A type of its own rather than passing the planner directly, so the
// optimizer's PlanQuoter port is satisfied by exactly ONE method. It cannot
// call Build, it cannot reach the store, and it cannot persist a plan,
// because none of those is on the value it holds.
type basketQuoter struct {
	planning *fulfilment.PlanningService
}

// QuotePlan implements basket.PlanQuoter.
func (q basketQuoter) QuotePlan(
	ctx context.Context,
	request fulfilment.QuoteOnlyRequest,
) (fulfilment.QuoteOnlyResult, error) {
	return q.planning.QuotePlan(ctx, request)
}

// buildBasketOptimizationService composes the optimizer.
//
// It returns nil when the feature is disabled or the planner is absent. An
// optimizer with no way to price delivery would compare baskets on
// merchandise alone and reach the wrong answer confidently, which is worse
// than not offering the feature.
func buildBasketOptimizationService(
	cfg config.Config,
	db *pgxpool.Pool,
	planning *fulfilment.PlanningService,
) *basket.Service {
	if !cfg.Flags.CommerceBasketOptimizationEnabled ||
		db == nil || planning == nil {
		return nil
	}

	service, err := basket.NewService(basket.ServiceOptions{
		Catalogue:   newBasketCatalogue(db),
		Quoter:      basketQuoter{planning: planning},
		Carts:       newBasketCarts(db),
		Store:       basket.NewStore(db),
		ProposalTTL: cfg.Basket.ProposalTTL,
	})
	if err != nil {
		return nil
	}

	return service.WithAddressResolver(newBasketAddresses(db))
}

// registerBasketOptimizationRoutes mounts the customer optimization surface.
func registerBasketOptimizationRoutes(
	router *gin.Engine,
	cfg config.Config,
	service *basket.Service,
) {
	if router == nil || service == nil {
		return
	}

	basket.RegisterCustomerRoutes(
		router.Group("/api/v1/commerce"),
		basket.NewHandler(service),
		sharedauth.NewTokenSigner(cfg.CustomerAccessTokenSecret),
		cfg.Flags.CommerceBasketOptimizationEnabled,
	)
}

// basketProposalExpiryJob sweeps lapsed offers.
//
// A proposal past its life quoted prices, stock and logistics fees that are
// no longer evidence. The read path already refuses one, but a proposal
// nobody reads must still stop being acceptable - otherwise a browser tab
// left open overnight keeps yesterday's prices clickable.
func basketProposalExpiryJob(
	cfg config.Config,
	service *basket.Service,
	coordinator *jobs.DurableCoordinator,
	owner string,
) (jobs.DurableJob, bool) {
	if service == nil ||
		!cfg.Flags.CommerceBasketOptimizationEnabled ||
		!cfg.Flags.CommerceFulfilmentWorkersEnabled {
		return jobs.DurableJob{}, false
	}

	interval := cfg.Basket.ExpiryInterval

	return jobs.DurableJob{
		Job: jobs.Job{
			Name:     "commerce-basket-proposal-expiry",
			Interval: interval,
			Pass: func(runCtx context.Context) error {
				_, err := service.SweepExpiredProposals(runCtx, 200)

				return err
			},
		},
		Coordinator: coordinator,
		Owner:       owner,
		LeaseFor:    2 * interval,
		MaxAttempts: 5,
		RetryBase:   interval,
	}, true
}

// basketRecurrenceJob generates recurring baskets that have fallen due.
//
// Safe on any number of workers at any interval, which is why it needs no
// coordination beyond the existing durable lease. Every occurrence is claimed
// through a unique index BEFORE any shopping is done, so two sweeps produce
// one generation between them and a sweep that dies after claiming finds its
// own claim next time rather than duplicating it.
//
// The lease and retry settings here reduce wasted work; they are deliberately
// not what makes exactly-once true. A guarantee that depended on a lease
// would fail the moment a lease expired under load, which is precisely when
// duplicate shopping would be most expensive.
func basketRecurrenceJob(
	cfg config.Config,
	service *basket.Service,
	coordinator *jobs.DurableCoordinator,
	owner string,
) (jobs.DurableJob, bool) {
	if service == nil ||
		!cfg.Flags.CommerceBasketOptimizationEnabled ||
		!cfg.Flags.CommerceRecurringBasketEnabled ||
		!cfg.Flags.CommerceFulfilmentWorkersEnabled {
		return jobs.DurableJob{}, false
	}

	interval := cfg.Basket.RecurrenceInterval

	if interval <= 0 {
		interval = time.Minute
	}

	return jobs.DurableJob{
		Job: jobs.Job{
			Name:     "commerce-recurring-basket-generation",
			Interval: interval,
			Pass: func(runCtx context.Context) error {
				_, err := service.SweepRecurringTemplates(runCtx, 100)

				return err
			},
		},
		Coordinator: coordinator,
		Owner:       owner,
		LeaseFor:    2 * interval,
		MaxAttempts: 5,
		RetryBase:   interval,
	}, true
}
