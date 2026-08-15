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
	"cosmicforge/logistics/services/vendor-service/internal/features/cloudkitchen"
	"cosmicforge/logistics/services/vendor-service/internal/features/crm"
	"cosmicforge/logistics/services/vendor-service/internal/features/food"
	"cosmicforge/logistics/services/vendor-service/internal/features/pastry"
	"cosmicforge/logistics/services/vendor-service/internal/features/production"
	"cosmicforge/logistics/services/vendor-service/internal/features/shop"
	staffhttp "cosmicforge/logistics/services/vendor-service/internal/features/staff/http"
	staffrepositories "cosmicforge/logistics/services/vendor-service/internal/features/staff/repositories"
	"cosmicforge/logistics/services/vendor-service/internal/features/vendors"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
)

// ExpectedRouteCount is the accepted fully-enabled Vendor surface at the close
// of final Section 12: the 443-route Section 12A surface plus 42 public
// inventory routes and 11 root-mounted HMAC discovery/reservation routes.
// The figure is asserted against production route
// registration rather than counted from prose.
const ExpectedRouteCount = 496

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

	staffhttp.RegisterInternalRoutes(router, nil, verifier)
	vendors.RegisterInternalOnboardingRoutes(router, vendorHandler, verifier)
	crm.RegisterInternalRoutes(router, crm.NewInternalHandler(crmService), verifier, flags)
	food.RegisterInternalRoutes(router, foodInternalHandler, verifier, foodFlags)
	shop.RegisterInternalInventoryRoutes(router, shop.NewInventoryInternalHandler(shop.NewService(shop.Options{Flags: shopFlags}), []string{"order-service"}), verifier, shopFlags)
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
var registerPathPattern = regexp.MustCompile(`(/(?:api/v1/vendor|internal|auth|staff|roles|permissions|businesses|onboarding|assignments|crm)[A-Za-z0-9_\-/{}:\.]*)`)

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
