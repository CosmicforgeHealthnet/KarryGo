package main

// Section 17C composition root.
//
// It wires fulfilment planning to the authorities it needs and to nothing
// else. Notably absent: any Wallet, ParentOrder, VendorOrder or payment
// dependency. Section 17C plans and quotes; it moves no money and creates no
// order, and a dependency wired here would be the first step towards doing so.

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/config"
	"cosmicforge/logistics/services/vendor-service/internal/features/delivery"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
	taxiintegration "cosmicforge/logistics/services/vendor-service/internal/integrations/taxi"
	"cosmicforge/logistics/services/vendor-service/internal/jobs"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/commercequote"
)

// -----------------------------------------------------------------------------
// Quote authority adapters
// -----------------------------------------------------------------------------

// section17CQuoteAuthority adapts shared/go/commercequote to the planner's
// narrow port, so the fulfilment feature never learns a base URL or a secret.
type section17CQuoteAuthority struct {
	client *commercequote.Client

	// enabled is the deployment's own feature flag for this integration. It is
	// kept separate from client configuration so a deployment can hold working
	// credentials and still deliberately not offer the option.
	enabled bool
}

func newSection17CQuoteAuthority(
	enabled bool,
	baseURL, apiBase, service, secret string,
	timeout time.Duration,
) *section17CQuoteAuthority {
	return &section17CQuoteAuthority{
		client: commercequote.New(commercequote.Options{
			BaseURL: baseURL,
			APIBase: apiBase,
			Service: service,
			Secret:  secret,
			Timeout: timeout,
		}),
		enabled: enabled,
	}
}

// Available implements fulfilment.QuoteAuthority.
func (a *section17CQuoteAuthority) Available() bool {
	return a != nil && a.enabled && a.client.Configured()
}

// RequestQuote implements fulfilment.QuoteAuthority.
func (a *section17CQuoteAuthority) RequestQuote(
	ctx context.Context,
	request commercequote.Request,
) (commercequote.Quote, error) {
	return a.client.RequestQuote(ctx, request)
}

// -----------------------------------------------------------------------------
// Vendor own delivery adapter
// -----------------------------------------------------------------------------

// section17CVendorDelivery adapts the delivery feature to the planner's port.
type section17CVendorDelivery struct {
	service *delivery.Service
}

func (a section17CVendorDelivery) PlatformEnabled() bool {
	return a.service != nil && a.service.Enabled()
}

func (a section17CVendorDelivery) ResolvePolicy(
	ctx context.Context,
	businessID, branchID string,
) (delivery.Policy, bool, error) {
	return a.service.ResolvePolicy(ctx, businessID, branchID)
}

// -----------------------------------------------------------------------------
// Checkout adapter
// -----------------------------------------------------------------------------

// section17CCheckoutAuthority resolves a validated Section 17B checkout for
// planning.
//
// It reads the checkout tables directly rather than importing the checkout
// feature, for the same reason every other Section 17 adapter lives here:
// fulfilment must not import checkout, and checkout must learn nothing about
// fulfilment.
//
// Only VALIDATED, unexpired sessions are plannable. A session still validating,
// failed, cancelled or lapsed cannot support a plan, and planning refuses
// rather than quoting against evidence that may not hold.
type section17CCheckoutAuthority struct {
	db *pgxpool.Pool

	now func() time.Time
}

func newSection17CCheckoutAuthority(
	db *pgxpool.Pool,
) *section17CCheckoutAuthority {
	return &section17CCheckoutAuthority{db: db, now: time.Now}
}

// ResolveForPlanning implements fulfilment.CheckoutAuthority.
func (a *section17CCheckoutAuthority) ResolveForPlanning(
	ctx context.Context,
	customerID, sessionID string,
) (fulfilment.CheckoutSnapshot, error) {
	if a == nil || a.db == nil {
		return fulfilment.CheckoutSnapshot{}, fulfilment.ErrDependencyMissing
	}

	customerID = strings.TrimSpace(customerID)
	sessionID = strings.TrimSpace(sessionID)

	snapshot := fulfilment.CheckoutSnapshot{
		SessionID:  sessionID,
		CustomerID: customerID,
	}

	var (
		status    string
		expiresAt time.Time
	)

	// The delivery address is the Section 17A snapshot the checkout validated
	// against, joined here rather than re-resolved: re-asking Customer Service
	// could return an address the customer has since edited, which is not the
	// one this checkout was validated for.
	err := a.db.QueryRow(ctx, `
		SELECT s.status, s.validation_revision, s.currency, s.expires_at,
		       COALESCE(a.address_snapshot, ''),
		       COALESCE(a.latitude_snapshot, 0),
		       COALESCE(a.longitude_snapshot, 0)
		  FROM commerce_checkout_sessions s
		  LEFT JOIN commerce_cart_addresses a
		         ON a.cart_id = s.cart_id
		 WHERE s.id = $1 AND s.customer_id = $2`,
		sessionID, customerID,
	).Scan(
		&status,
		&snapshot.ValidationRevision,
		&snapshot.Currency,
		&expiresAt,
		&snapshot.Destination.Address,
		&snapshot.Destination.Latitude,
		&snapshot.Destination.Longitude,
	)
	if err != nil {
		// A session that does not exist and one belonging to another customer
		// are the same answer, so a probe cannot tell them apart.
		return fulfilment.CheckoutSnapshot{}, nil
	}

	snapshot.Plannable = status == "VALIDATED" &&
		expiresAt.After(a.now().UTC())

	if !snapshot.Plannable {
		return snapshot, nil
	}

	groups, err := a.groups(ctx, customerID, sessionID)
	if err != nil {
		return fulfilment.CheckoutSnapshot{}, err
	}

	snapshot.Groups = groups

	return snapshot, nil
}

func (a *section17CCheckoutAuthority) groups(
	ctx context.Context,
	customerID, sessionID string,
) ([]fulfilment.CheckoutGroupSnapshot, error) {
	rows, err := a.db.Query(ctx, `
		SELECT id, business_id, branch_id, quoted_total_minor, currency
		  FROM commerce_checkout_groups
		 WHERE session_id = $1 AND customer_id = $2
		 ORDER BY id`,
		sessionID, customerID,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var groups []fulfilment.CheckoutGroupSnapshot

	for rows.Next() {
		var group fulfilment.CheckoutGroupSnapshot

		if err = rows.Scan(
			&group.CheckoutGroupID, &group.BusinessID, &group.BranchID,
			&group.QuotedTotalMinor, &group.Currency,
		); err != nil {
			return nil, err
		}

		groups = append(groups, group)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	for index := range groups {
		lines, err := a.lines(ctx, sessionID, groups[index].CheckoutGroupID)
		if err != nil {
			return nil, err
		}

		groups[index].Lines = lines
	}

	return groups, nil
}

// lines reads only the lines a delivery must actually carry.
//
// UNAVAILABLE and INVALID lines are excluded: Section 17B already refused
// them, and planning a vehicle around goods nobody will ship would overstate
// the load and the fee.
func (a *section17CCheckoutAuthority) lines(
	ctx context.Context,
	sessionID, groupID string,
) ([]fulfilment.LineLogistics, error) {
	rows, err := a.db.Query(ctx, `
		SELECT id, domain, business_id, branch_id,
		       item_ref, variant_ref, listing_ref, sell_unit_ref, quantity,
		       CASE WHEN pricing_basis='VARIABLE_MEASURE' THEN expected_measure_quantity*quantity ELSE 0 END
		  FROM commerce_checkout_lines
		 WHERE session_id = $1 AND group_id = $2
		   AND validation_status IN ('VALID', 'CHANGED')
		 ORDER BY id`,
		sessionID, groupID,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var lines []fulfilment.LineLogistics

	for rows.Next() {
		var line fulfilment.LineLogistics

		if err = rows.Scan(
			&line.CheckoutLineID, &line.Domain,
			&line.BusinessID, &line.BranchID,
			&line.ItemRef, &line.VariantRef, &line.ListingRef,
			&line.SellUnitRef, &line.Quantity, &line.MeasureQuantity,
		); err != nil {
			return nil, err
		}

		lines = append(lines, line)
	}

	return lines, rows.Err()
}

// -----------------------------------------------------------------------------
// Wiring
// -----------------------------------------------------------------------------

// The API base each logistics authority mounts its quote endpoint under.
//
// These are the halves of a cross-service path that no compiler checks. The
// full URL is baseURL + apiBase + commercequote.QuotePath, and the far side
// must register exactly that. Getting it wrong produces a 404 on every call
// with no other symptom - which is precisely the bug this platform already
// shipped once, when HAULING_PAYMENT_URL was configured with a full path and
// the wallet client appended its own.
//
// Each constant is asserted against the route the owning service actually
// registers, in that service's own test.
const (
	dispatchQuoteAPIBase = "/api/v1"
	taxiQuoteAPIBase     = "/api/v1/taxi"
	haulingQuoteAPIBase  = "/api/v1/hauling"
)

// buildSection17CFulfilmentService composes the Section 17C boundary.
//
// It returns nil when planning is disabled or a dependency is missing, so a
// deployment that has not enabled it never registers a route and never starts
// a worker.
func buildSection17CFulfilmentService(
	cfg config.Config,
	db *pgxpool.Pool,
	deliveryService *delivery.Service,
) *fulfilment.PlanningService {
	if !cfg.Flags.CommerceFulfilmentPlanningEnabled || db == nil {
		return nil
	}

	planner, err := newSection17CPlanner(cfg, db, deliveryService)
	if err != nil {
		return nil
	}

	service, err := fulfilment.NewService(fulfilment.ServiceOptions{
		Store:    fulfilment.NewStore(db),
		Planner:  planner,
		Checkout: newSection17CCheckoutAuthority(db),
	})
	if err != nil {
		return nil
	}

	return service
}

// newSection17CPlanner builds THE fulfilment planner: checkout planning, the
// cart delivery preview and post-pick re-evaluation all use this one, so the
// same load always meets the same policy, capabilities and authorities.
func newSection17CPlanner(
	cfg config.Config,
	db *pgxpool.Pool,
	deliveryService *delivery.Service,
) (*fulfilment.Planner, error) {
	return fulfilment.NewPlanner(fulfilment.Options{
		Logistics: newSection17CLogisticsAuthority(db),
		Branches:  newSection17CBranchAuthority(db),
		VendorOwn: section17CVendorDelivery{service: deliveryService},

		Dispatch: newSection17CQuoteAuthority(
			cfg.Flags.DispatchCommerceIntegrationEnabled,
			cfg.DispatchURL, dispatchQuoteAPIBase, "DISPATCH",
			cfg.DispatchSecret, cfg.Fulfilment.QuoteTimeout,
		),

		Hauling: newSection17CQuoteAuthority(
			cfg.Flags.HaulingCommerceIntegrationEnabled,
			cfg.HaulingURL, haulingQuoteAPIBase, "HAULING",
			cfg.HaulingSecret, cfg.Fulfilment.QuoteTimeout,
		),

		Taxi: newSection17CQuoteAuthority(
			cfg.Flags.TaxiGoodsDeliveryEnabled,
			cfg.TaxiURL, taxiQuoteAPIBase, "TAXI",
			cfg.TaxiSecret, cfg.Fulfilment.QuoteTimeout,
		),

		ConsolidationRadiusMeters: cfg.Fulfilment.ConsolidationRadiusMeters,
		PlanTTL:                   cfg.Fulfilment.PlanTTL,

		Optimization: fulfilment.OptimizationPolicy{
			ProximityRadiusMeters:  cfg.Fulfilment.ProximityRadiusMeters,
			ColocationRadiusMeters: cfg.Fulfilment.ConsolidationRadiusMeters,
		},

		// The runtime multi-pickup capability.
		//
		// Dispatch and hauling still declare theirs as deployment data,
		// because neither publishes a capability contract yet. Taxi does, so
		// its entry is read from Taxi itself below: a deployment can no
		// longer claim, or deny, a capability the authority owns.
		MultiPickup: map[fulfilment.Service]bool{
			fulfilment.ServiceDispatch: cfg.Fulfilment.DispatchMultiPickupSupported,
			fulfilment.ServiceHauling:  cfg.Fulfilment.HaulingMultiPickupSupported,
		},

		CapabilityAuthorities: taxiCapabilityAuthorities(cfg),
	})
}

// registerSection17CFulfilmentRoutes mounts the customer planning surface.
func registerSection17CFulfilmentRoutes(
	router *gin.Engine,
	cfg config.Config,
	service *fulfilment.PlanningService,
) {
	if router == nil || service == nil {
		return
	}

	fulfilment.RegisterCustomerRoutes(
		router.Group("/api/v1/commerce"),
		fulfilment.NewHandler(service),
		sharedauth.NewTokenSigner(cfg.CustomerAccessTokenSecret),
		cfg.Flags.CommerceFulfilmentPlanningEnabled,
	)
}

// section17CExpiryJob sweeps lapsed plans.
//
// A plan past its expiry holds quotes no authority still stands behind. The
// sweep is what stops a later section finding one that looks live.
func section17CExpiryJob(
	cfg config.Config,
	service *fulfilment.PlanningService,
	coordinator *jobs.DurableCoordinator,
	owner string,
) (jobs.DurableJob, bool) {
	if service == nil ||
		!cfg.Flags.CommerceFulfilmentPlanningEnabled ||
		!cfg.Flags.CommerceFulfilmentWorkersEnabled {
		return jobs.DurableJob{}, false
	}

	interval := cfg.Fulfilment.ExpiryInterval

	return jobs.DurableJob{
		Job: jobs.Job{
			Name:     "commerce-fulfilment-plan-expiry",
			Interval: interval,
			Pass: func(runCtx context.Context) error {
				_, err := service.SweepExpired(runCtx, 200)

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

// taxiCapabilityAuthorities wires Taxi's live capability contract. An
// unconfigured or disabled integration contributes no entry, which leaves
// Taxi with no multi-pickup capability rather than an assumed one.
func taxiCapabilityAuthorities(cfg config.Config) map[fulfilment.Service]fulfilment.CapabilityAuthority {
	authority := taxiintegration.NewCapabilityAuthority(
		cfg.Flags.TaxiGoodsDeliveryEnabled, cfg.TaxiURL, commercemission.SignatureCaller, cfg.TaxiSecret,
	)
	if authority == nil {
		return nil
	}

	return map[fulfilment.Service]fulfilment.CapabilityAuthority{
		fulfilment.ServiceTaxi: authority,
	}
}
