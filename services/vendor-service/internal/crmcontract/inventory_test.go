// Package crmcontract is the Section 8I global contract guard.
//
// The per-section contract packages each pin their own surface. Nothing before
// this pinned the WHOLE of it, which is the gap that lets drift hide: a route
// can be registered in production, documented nowhere, and still pass every
// section test because no section claims it.
//
// This package builds the real production route set from the same Register*
// helpers cmd/main.go calls, then holds it against every OpenAPI specification
// and the route register in both directions. A route that exists but is
// undocumented fails here; so does an operation that is documented but does not
// exist.
package crmcontract

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/serviceauth"

	vendorauthhttp "cosmicforge/logistics/services/vendor-service/internal/features/auth/http"
	vendorauthrepositories "cosmicforge/logistics/services/vendor-service/internal/features/auth/repositories"
	vendorauthusecases "cosmicforge/logistics/services/vendor-service/internal/features/auth/usecases"
	"cosmicforge/logistics/services/vendor-service/internal/features/basket"
	"cosmicforge/logistics/services/vendor-service/internal/features/cart"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkout"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkoutquote"
	"cosmicforge/logistics/services/vendor-service/internal/features/cloud_kitchen"
	"cosmicforge/logistics/services/vendor-service/internal/features/cost"
	"cosmicforge/logistics/services/vendor-service/internal/features/crm"
	"cosmicforge/logistics/services/vendor-service/internal/features/delivery"
	financeprojection "cosmicforge/logistics/services/vendor-service/internal/features/finance_projection"
	"cosmicforge/logistics/services/vendor-service/internal/features/food"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilment"
	"cosmicforge/logistics/services/vendor-service/internal/features/fulfilmentgroup"
	"cosmicforge/logistics/services/vendor-service/internal/features/inventory"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"cosmicforge/logistics/services/vendor-service/internal/features/pastry"
	"cosmicforge/logistics/services/vendor-service/internal/features/production"
	"cosmicforge/logistics/services/vendor-service/internal/features/productionbatch"
	"cosmicforge/logistics/services/vendor-service/internal/features/productionmaster"
	"cosmicforge/logistics/services/vendor-service/internal/features/promotions"
	"cosmicforge/logistics/services/vendor-service/internal/features/purchasing"
	"cosmicforge/logistics/services/vendor-service/internal/features/recipes"
	"cosmicforge/logistics/services/vendor-service/internal/features/searchprojection"
	"cosmicforge/logistics/services/vendor-service/internal/features/shop"
	staffhttp "cosmicforge/logistics/services/vendor-service/internal/features/staff/http"
	staffrepositories "cosmicforge/logistics/services/vendor-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/vendor-service/internal/features/taxonomy"
	"cosmicforge/logistics/services/vendor-service/internal/features/vendors"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
)

// ExpectedRouteCount is the accepted fully-enabled Vendor surface at the close
// of Section 16: the 646-route Section 15 surface, plus the 49 Section
// 16A/16B/16C routes, plus Section 16D's 17 operational cost routes.
//
// The Section 16A/16B/16C routes were live in cmd/main.go from their own
// sections but were never registered in this gate's stand-in router, which is
// why it reported 34 Section 16 OpenAPI operations as phantom: the routes
// existed, this router simply did not mount them. Registering them in
// buildProductionRouter closed that hole, exactly as Section 14I closed the
// identical one for Section 14.
//
// Section 17 was invisible to this gate until Section 17B: neither the
// Section 17A cart nor the Section 17B checkout was registered here, so the
// figure excluded both and their specifications read as 15 phantom
// operations. Registering the two customer commerce surfaces closes the same
// hole Section 14I and Section 16D each had to close before it.
//
// Section 17C adds four: two customer planning routes under the commerce base
// and two Vendor Own Delivery policy routes under the vendor base. Registering
// them here is the same hole Section 14I, Section 16D and Section 17B each had
// to close - and this gate caught it, reporting the four as phantom before
// they were mounted.

// Section 17D adds three customer quote routes: create, historical read and
// explicit fingerprint approval.
//
// Section 17E adds twelve more: four customer order routes (create from an
// approved quote, list, read and the customer's own single-vendor withdrawal)
// and eight vendor staff routes (the queue, one order, and the six lifecycle
// commands).
//
// Section 17F adds two: the customer payment-binding start and its read. They
// exist only when a money authority is wired, and a fully enabled deployment
// has one. Section 20 adds production operations; Section 21 Prompt 2 adds
// RFQ counter-offer/conversion operations and tenant-scoped recurring feeding
// operations. The Promotions and Delivery Funding capability adds six Vendor
// management routes plus one definition update route, taking the accepted
// fully enabled surface to 810.
//
// The fulfilment group, parcel and mission model adds thirteen vendor staff
// routes: the group read and its plan history, the parcel list/pack/read/
// custody/manifest/return operations, the pickup-readiness command, and the
// four vendor-owned mission routes. Every one is a read or a vendor-executed
// action; none creates or mutates a platform logistics mission, because that
// mission belongs to the service that owns it.
//
// Multi-vendor consolidation and mission planning adds eleven customer
// routes, all of them under the commerce base: two optimization entry points
// (budget shopping and Optimize My Cart), two proposal routes (read and
// accept), and seven recurring-basket routes (create, list, read, edit, pause
// or resume, occurrence history, and shop-now).
//
// Ten of the eleven are read-or-propose. The single exception is the proposal
// acceptance, which is the only route in the capability that changes anything
// the customer owns - and what it changes is their cart, by creating a new
// version of it.
//
// The planning surface itself gained no route. The customer's delivery
// preference travels in the existing plan request's body, because it is a
// CHOICE among alternatives the server built rather than a new thing to ask
// for.
//
// Dispatch Commerce delivery adds one HMAC-v2 lifecycle callback. It projects
// mission, stop and parcel evidence and exposes no customer or Vendor-staff
// command surface.
//
// The figure is measured from production route registration, never targeted.
const ExpectedRouteCount = 855

// allFlagsOn enables every CRM module, because the inventory must describe the
// full surface a fully-enabled deployment exposes.
func allFlagsOn() crm.Flags {
	return crm.Flags{
		CRMEnabled: true, WritesEnabled: true,
		CustomerProjectionEnabled: true, ConsentIntegrationEnabled: true,
		PrivacyIntegrationEnabled: true,
		SegmentationEnabled:       true, SegmentMaterializationEnabled: true,
		InteractionsEnabled: true, TasksEnabled: true, LoyaltyEnabled: true,
		CampaignsEnabled: true, CampaignPreparationEnabled: true, CampaignExecutionEnabled: true,
		CorporateEnabled: true, IntelligenceEnabled: true, IntelligenceWorkerEnabled: true,
		ExportsEnabled: true,
	}
}

// buildProductionRouter mirrors cmd/main.go's Register and RegisterRoot blocks.
//
// Every dependency is nil or zero: route registration attaches handlers, and no
// handler is invoked here. That keeps the inventory a pure structural fact
// about the code rather than something that needs a database to establish.
func buildProductionRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()

	flags := allFlagsOn()
	crmService := crm.NewService(crm.Options{Flags: flags})
	store := idempotency.NewStore(nil, time.Hour)

	authHandler := vendorauthhttp.NewHandler(nil, nil, vendorauthhttp.RateLimits{})
	staffHandler := staffhttp.NewHandler(nil, nil, nil)
	vendorHandler := vendors.NewHandler(nil, store, true, false)
	signer := sharedauth.NewTokenSigner([]byte("inventory-only"))
	sessionRepo := vendorauthrepositories.NewSessionRepository(nil)
	staffRepo := staffrepositories.NewStaffRepository(nil)
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{}, 5*time.Minute)

	group := router.Group("/api/v1/vendor")
	vendorauthhttp.RegisterRoutes(group, authHandler, signer, sessionRepo, staffResolver{staff: staffRepo})
	protected := group.Group("")
	staffhttp.RegisterRoutes(protected, staffHandler)
	vendors.RegisterRoutes(protected, vendorHandler)
	vendors.RegisterOnboardingRoutes(protected, vendorHandler)
	financeprojection.RegisterRoutes(protected, financeprojection.NewHandler(nil), true, store)
	crm.RegisterRoutes(protected, crm.NewHandler(crmService), flags)
	crm.RegisterSegmentationRoutes(protected, crm.NewSegmentationHandler(crmService, store), flags)
	crm.RegisterActivityRoutes(protected, crm.NewActivityHandler(crmService, store), flags)
	crm.RegisterLoyaltyRoutes(protected, crm.NewLoyaltyHandler(crmService, store), flags)
	crm.RegisterLoyaltyTierRoutes(protected, crm.NewLoyaltyHandler(crmService, store), flags)
	campaignHandler := crm.NewCampaignHandler(crmService, store)
	crm.RegisterCampaignRoutes(protected, campaignHandler, flags)
	crm.RegisterCampaignExecutionRoutes(protected, campaignHandler, flags)
	crm.RegisterCorporateRoutes(protected, crm.NewCorporateHandler(crmService, store), flags)
	crm.RegisterIntelligenceRoutes(protected, crm.NewIntelligenceHandler(crmService, store), flags)
	crm.RegisterExportRoutes(protected, crm.NewExportHandler(crmService, store), flags)
	foodFlags := food.Flags{CatalogueEnabled: true, WritesEnabled: true}
	foodHandler := food.NewHandler(food.NewService(food.Options{Flags: foodFlags}), store)
	foodService := food.NewService(food.Options{Flags: foodFlags})
	foodInternalHandler := food.NewInternalHandler(foodService)
	food.RegisterRoutes(protected, foodHandler, foodFlags)
	food.RegisterOperationsRoutes(protected, foodHandler, foodFlags)
	productionFlags := production.Flags{ProductionEnabled: true, WritesEnabled: true}
	productionService := production.NewService(production.Options{Flags: productionFlags})
	production.RegisterRoutes(protected, production.NewHandler(productionService), productionFlags)
	cloudkitchen.RegisterRoutes(protected, cloudkitchen.NewHandler(cloudkitchen.NewService(nil, productionService)), productionFlags)
	pastry.RegisterRoutes(protected, pastry.NewHandler(pastry.NewService(nil, productionService)), productionFlags)
	shopFlags := shop.Flags{ShopEnabled: true, WritesEnabled: true}
	shop.RegisterRoutes(protected, shop.NewHandler(shop.NewService(shop.Options{Flags: shopFlags}), store), shopFlags)

	// Section 14: the generic Inventory authority. Shop keeps its own
	// Section 12B inventory surface above; these are the generic routes.
	inventoryFlags := inventory.Flags{Enabled: true, WritesEnabled: true}
	inventoryHandler := inventory.NewHandler(nil, nil)
	inventory.RegisterRoutes(protected, inventoryHandler, inventoryFlags)
	inventory.RegisterReservationRoutes(protected, inventoryHandler, inventoryFlags)
	inventory.RegisterOperationalRoutes(protected, inventoryHandler, inventoryFlags)

	// Section 15: suppliers, purchasing and goods receiving. Live in
	// cmd/main.go since 15A but never registered here, so this service-wide
	// gate could not see any of its 56 routes until 15D.
	purchasingBase := purchasing.NewHandler(nil, nil)
	purchasing.RegisterRoutes(protected, purchasingBase, true)
	purchasing.RegisterPurchaseOrderRoutes(protected,
		purchasing.NewPurchaseOrderHandler(nil, purchasingBase, nil), true)
	purchasing.RegisterGoodsReceivingRoutes(protected,
		purchasing.NewGoodsReceivingHandler(nil, nil, nil), true)
	purchasing.RegisterOperationsRoutes(protected,
		purchasing.NewOperationsHandler(nil, nil), true)

	// Section 16A/16B/16C: production master data, recipes and production
	// batches. These have been live in cmd/main.go since their own sections,
	// but were never registered here - which is why this gate reported 34
	// Section 16 OpenAPI operations as phantom. The routes existed; this
	// stand-in router simply did not mount them. Registering them makes the
	// gate measure the real surface again.
	productionMasterHandler := productionmaster.NewHandler(
		productionmaster.NewService(nil, nil), nil, store)
	productionmaster.RegisterRoutes(protected, productionMasterHandler)

	recipes.RegisterRoutes(protected,
		recipes.NewHandler(recipes.NewService(nil, nil), nil, store), true)

	productionbatch.RegisterRoutes(protected,
		productionbatch.NewHandler(nil, store), true)

	// Section 16D: operational cost.
	cost.RegisterRoutes(protected, cost.NewHandler(nil, store), true)

	// Section 17C: the Vendor Own Delivery policy surface. It belongs to the
	// vendor staff base rather than the customer commerce one, which is why
	// its OpenAPI document is separate from the planning contract.
	delivery.RegisterRoutes(
		protected,
		delivery.NewHandler(delivery.NewService(nil, nil, true)),
		true,
	)
	promotions.RegisterRoutes(protected, promotions.NewHandler(promotions.NewService(nil, nil, nil, true)), true)

	// Section 17E: the vendor order queue and its lifecycle commands. Like the
	// delivery policy above it belongs to the vendor staff base, because it is
	// driven by vendor staff IAM rather than by a customer token.
	orders.RegisterVendorRoutes(protected, &orders.Service{}, true)
	orders.RegisterSection21VendorRoutes(protected, orders.NewSection21Service(orders.Section21Options{Enabled: true}), true)

	// The post-order fulfilment aggregate: groups, pickup obligations, parcels,
	// versioned plans, vendor-owned missions and the read-only projections of
	// platform logistics missions. Vendor staff base, same reason as above.
	fulfilmentgroup.RegisterRoutes(
		protected,
		fulfilmentgroup.NewHandler(fulfilmentgroup.NewService(fulfilmentgroup.Options{Enabled: true})),
		true,
	)

	// Section 13: canonical taxonomy classification (vendor) plus the two
	// root-mounted internal surfaces registered below.
	taxonomyFlags := taxonomy.Flags{TaxonomyEnabled: true, WritesEnabled: true}
	taxonomyService := taxonomy.NewService(taxonomy.NewRepository(nil), nil)
	taxonomyAccess := taxonomy.NewVendorAccess(nil, nil)
	taxonomy.RegisterRoutes(protected, taxonomy.NewHandler(taxonomyService, taxonomyAccess, taxonomyFlags), taxonomyFlags)

	// Section 17: the customer commerce surfaces. They are root-mounted under
	// /api/v1/commerce rather than under the vendor base, because they are
	// driven by a Customer Service bearer token and not by vendor staff IAM.
	customerCommerce := router.Group("/api/v1/commerce")
	customerSigner := sharedauth.NewTokenSigner([]byte("inventory-only"))

	cart.RegisterCustomerRoutes(customerCommerce, &cart.Service{}, customerSigner, store)
	checkout.RegisterCustomerRoutes(customerCommerce, &checkout.Service{}, customerSigner, store, true)
	fulfilment.RegisterCustomerRoutes(
		customerCommerce,
		fulfilment.NewHandler(&fulfilment.PlanningService{}),
		customerSigner,
		true,
	)
	basket.RegisterCustomerRoutes(
		customerCommerce,
		basket.NewHandler(&basket.Service{}),
		customerSigner,
		true,
	)
	checkoutquote.RegisterCustomerRoutes(
		customerCommerce,
		&checkoutquote.Service{},
		customerSigner,
		store,
		true,
	)
	orders.RegisterCustomerRoutes(
		customerCommerce,
		&orders.Service{},
		customerSigner,
		store,
		true,
		// Section 17F: the fully-enabled surface includes payment.
		true,
	)
	orders.RegisterSection21CustomerRoutes(router, orders.NewSection21Service(orders.Section21Options{Enabled: true}), customerSigner, true)

	staffhttp.RegisterInternalRoutes(router, nil, verifier)
	vendors.RegisterInternalOnboardingRoutes(router, vendorHandler, verifier)
	crm.RegisterInternalRoutes(router, crm.NewInternalHandler(crmService), verifier, flags)
	food.RegisterInternalRoutes(router, foodInternalHandler, verifier, foodFlags)
	shop.RegisterInternalInventoryRoutes(router, shop.NewInventoryInternalHandler(shop.NewService(shop.Options{Flags: shopFlags}), []string{"order-service"}), verifier, shopFlags)
	taxonomy.RegisterInternalRoutes(router, taxonomy.NewInternalHandler(taxonomyService, []string{"admin-backoffice-service"}, nil, taxonomyFlags), verifier, taxonomyFlags)
	inventory.RegisterInternalRoutes(router, inventory.NewInternalInventoryHandler(
		nil, []string{"order-service"}), verifier, inventoryFlags)
	orders.RegisterInternalLogisticsRoutes(router, &orders.Service{}, verifier)
	orders.RegisterDispatchCommerceEventRoutes(router, &orders.Service{}, verifier)
	orders.RegisterSupportResolutionRoutes(router, &orders.Service{}, verifier)
	searchFlags := searchprojection.Flags{SearchEnabled: true, AdminEnabled: true}
	searchprojection.RegisterInternalSearchRoutes(router, searchprojection.NewInternalHandler(searchprojection.InternalHandlerOptions{
		Callers: []string{"order-service"}, Flags: searchFlags,
	}), verifier, searchFlags)
	return router
}

// staffResolver satisfies the auth middleware's staff port without a database.
// No method is ever called: registration attaches handlers, it does not run
// them.
type staffResolver struct {
	staff *staffrepositories.StaffRepository
}

func (s staffResolver) ListActiveByUser(ctx context.Context, userID string) ([]vendorauthusecases.StaffAffiliation, error) {
	return nil, nil
}

func (s staffResolver) GetByVendorAndUser(ctx context.Context, vendorID, userID string) (vendorauthusecases.StaffAffiliation, error) {
	return vendorauthusecases.StaffAffiliation{}, nil
}

func (s staffResolver) GetByID(ctx context.Context, staffID string) (vendorauthusecases.StaffAffiliation, error) {
	return vendorauthusecases.StaffAffiliation{}, nil
}

// routeKey is the canonical "METHOD /path" form, with gin's :param syntax
// normalised to the OpenAPI {param} form so the two surfaces are comparable.
func routeKey(method, path string) string {
	segments := strings.Split(path, "/")
	for index, segment := range segments {
		if strings.HasPrefix(segment, ":") {
			segments[index] = "{" + strings.TrimPrefix(segment, ":") + "}"
		}
	}
	return method + " " + strings.Join(segments, "/")
}

func productionRoutes(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, route := range buildProductionRouter(t).Routes() {
		out[routeKey(route.Method, route.Path)] = true
	}
	return out
}

// TestProductionRouteCountIsTheAcceptedSurface derives the count from code.
func TestProductionRouteCountIsTheAcceptedSurface(t *testing.T) {
	routes := productionRoutes(t)
	if len(routes) != ExpectedRouteCount {
		keys := make([]string, 0, len(routes))
		for key := range routes {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		t.Fatalf("the production router exposes %d routes; the accepted surface is %d.\n%s",
			len(routes), ExpectedRouteCount, strings.Join(keys, "\n"))
	}
}

// TestEveryProductionRouteIsDocumented is the drift guard in the direction that
// matters most: a live route nobody wrote down.
func TestEveryProductionRouteIsDocumented(t *testing.T) {
	routes := productionRoutes(t)
	documented := documentedOperations(t)
	register := routeRegisterPaths(t)

	var missingSpec, missingRegister []string
	for route := range routes {
		method, path, _ := strings.Cut(route, " ")
		// Health, readiness and meta come free from serviceapp and are the
		// platform's contract rather than this service's; every other route is
		// this service's to document.
		if isPlatformRoute(path) {
			continue
		}
		if !documented[route] {
			missingSpec = append(missingSpec, route)
		}
		if !register[path] {
			missingRegister = append(missingRegister, method+" "+path)
		}
	}
	sort.Strings(missingSpec)
	sort.Strings(missingRegister)
	if len(missingSpec) != 0 {
		t.Errorf("%d live routes appear in no OpenAPI specification:\n%s",
			len(missingSpec), strings.Join(missingSpec, "\n"))
	}
	if len(missingRegister) != 0 {
		t.Errorf("%d live routes are absent from API_ROUTE_REGISTER.md:\n%s",
			len(missingRegister), strings.Join(missingRegister, "\n"))
	}
}

// TestNoOpenAPIOperationIsPhantom is the other direction: a documented
// operation that does not exist would send a client at a 404.
func TestNoOpenAPIOperationIsPhantom(t *testing.T) {
	routes := productionRoutes(t)
	var phantom []string
	for route, spec := range documentedOperationSources(t) {
		if !routes[route] {
			phantom = append(phantom, route+"  ("+spec+")")
		}
	}
	sort.Strings(phantom)
	if len(phantom) != 0 {
		t.Fatalf("%d documented operations do not exist in the router:\n%s",
			len(phantom), strings.Join(phantom, "\n"))
	}
}

// TestInternalRoutesAreNeverMountedUnderThePublicBase is the Section 8I
// authority check on ingress. An internal route reachable through the gateway
// base path would be an authority surface behind the wrong door.
func TestInternalRoutesAreNeverMountedUnderThePublicBase(t *testing.T) {
	for route := range productionRoutes(t) {
		_, path, _ := strings.Cut(route, " ")
		if strings.Contains(path, "/internal/") && strings.HasPrefix(path, "/api/v1/vendor/") {
			t.Errorf("internal route %s is mounted under the public vendor base", route)
		}
	}
}

// ---------------------------------------------------------------------------
// Documentation sources
// ---------------------------------------------------------------------------

const specDir = "../../docs/openapi"

type specDocument struct {
	Servers []struct {
		URL string `json:"url"`
	} `json:"servers"`
	Paths map[string]map[string]json.RawMessage `json:"paths"`
}

func isMethod(value string) bool {
	switch value {
	case "get", "post", "put", "patch", "delete":
		return true
	default:
		return false
	}
}

// isPlatformRoute names the endpoints serviceapp mounts for every service.
func isPlatformRoute(path string) bool {
	switch path {
	case "/health", "/ready", "/api/v1/vendor/meta":
		return true
	}
	return false
}

// documentedOperationSources maps each documented route to the spec that
// declares it, so a phantom operation can be reported with its source.
func documentedOperationSources(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("read openapi directory: %v", err)
	}
	specs := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		specs++
		raw, err := os.ReadFile(filepath.Join(specDir, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		var document specDocument
		if err = json.Unmarshal(raw, &document); err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		base := ""
		if len(document.Servers) > 0 {
			base = strings.TrimRight(document.Servers[0].URL, "/")
		}
		for path, methods := range document.Paths {
			for method := range methods {
				if !isMethod(method) {
					continue
				}
				full := path
				// A spec whose path already carries the internal root is
				// mounted there rather than under the server base.
				if !strings.HasPrefix(path, "/internal/") {
					full = base + path
				}
				out[strings.ToUpper(method)+" "+full] = entry.Name()
			}
		}
	}
	if specs == 0 {
		t.Fatal("no OpenAPI specifications were found")
	}
	return out
}

func documentedOperations(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for route := range documentedOperationSources(t) {
		out[route] = true
	}
	return out
}

// routeRegisterPaths reads the paths named in the route register.
//
// The register is prose with tables, and it records paths in two syntaxes: the
// earlier sections use gin's `:param` form, the later ones OpenAPI's `{param}`.
// Both are normalised to one shape here so the comparison measures real drift
// rather than which decade a row was written in. The parse is otherwise
// deliberately loose — it asks only whether each live route is mentioned
// somewhere, because a stricter parse would fail on table formatting.
var registerPathPattern = regexp.MustCompile(`(/(?:api/v1/vendor|api/v1/internal|internal|auth|staff|roles|permissions|businesses|onboarding|assignments|crm)[A-Za-z0-9_\-/{}:\.]*)`)

func normalizeRegisterPath(path string) string {
	segments := strings.Split(path, "/")
	for index, segment := range segments {
		if strings.HasPrefix(segment, ":") {
			segments[index] = "{" + strings.TrimPrefix(segment, ":") + "}"
		}
	}
	return strings.Join(segments, "/")
}

func routeRegisterPaths(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile("../../docs/API_ROUTE_REGISTER.md")
	if err != nil {
		t.Fatalf("read route register: %v", err)
	}
	text := string(raw)
	out := map[string]bool{}
	record := func(path string) {
		path = normalizeRegisterPath(strings.TrimRight(path, ".,;)`"))
		if path == "" {
			return
		}
		out[path] = true
		// Most rows record a CRM or business path relative to the vendor base.
		if !strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/internal/") {
			out["/api/v1/vendor"+path] = true
		}
	}
	for _, match := range registerPathPattern.FindAllString(text, -1) {
		record(match)
	}
	for _, line := range strings.Split(text, "\n") {
		for _, field := range strings.Split(line, "|") {
			if field = strings.TrimSpace(field); strings.HasPrefix(field, "/") {
				record(field)
			}
		}
	}
	return out
}
