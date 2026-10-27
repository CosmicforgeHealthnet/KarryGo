package integration

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/pricing-service/internal/engine"
	geomodels "cosmicforge/logistics/services/pricing-service/internal/features/geography/models"
	policymodels "cosmicforge/logistics/services/pricing-service/internal/features/policies/models"
	quotehttp "cosmicforge/logistics/services/pricing-service/internal/features/quotes/http"
	quotemodels "cosmicforge/logistics/services/pricing-service/internal/features/quotes/models"
	quoterepos "cosmicforge/logistics/services/pricing-service/internal/features/quotes/repositories"
	quoteusecases "cosmicforge/logistics/services/pricing-service/internal/features/quotes/usecases"
	"cosmicforge/logistics/services/pricing-service/internal/platform/canonical"
	"cosmicforge/logistics/services/pricing-service/internal/platform/ids"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/pricingclient"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// quoteWorld is a place with an active taxi tariff and commission, whose zone is
// a small circle at a random spot. Quote tables are append-only, so earlier runs
// stay in the database; a fresh location per run keeps their zones out of this
// run's "which zone contains the pickup" answer.
type quoteWorld struct {
	h        policyHarness
	quotes   *quoteusecases.Service
	where    place
	pickup   pricingclient.Point
	tariffID string
}

func newQuoteWorld(t *testing.T) quoteWorld {
	t.Helper()
	h := newPolicyHarness(t)
	ctx := context.Background()
	where := h.newPlace(t)

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	pickup := pricingclient.Point{Lat: -50 + rng.Float64()*100, Lng: -150 + rng.Float64()*300}
	shape := geomodels.CircleShape{
		Type: geomodels.ShapeCircle, CenterLatE6: canonical.Micro(pickup.Lat), CenterLngE6: canonical.Micro(pickup.Lng), RadiusM: 2000,
	}
	raw, err := json.Marshal(shape)
	if err != nil {
		t.Fatalf("encode shape: %v", err)
	}
	hash, err := canonical.Hash(shape)
	if err != nil {
		t.Fatalf("hash shape: %v", err)
	}
	if err := h.geo.AddZoneVersion(ctx, geomodels.ZoneVersion{
		ID: ids.New(ids.PrefixZoneVersion), ZoneID: where.zoneID, VersionNo: 1, Shape: raw, ShapeHash: hash, CreatedBy: h.editor,
	}); err != nil {
		t.Fatalf("seed zone shape: %v", err)
	}

	tariff := h.draft(t, engine.KindTaxiTariff, policymodels.LevelZone, where.zoneID, seedBody(t, "taxi_tariff_ng.json", nil), "V1 baseline")
	h.approveAndActivate(t, tariff, time.Time{})
	commission := h.draft(t, engine.KindCommission, policymodels.LevelZone, where.zoneID, seedBody(t, "commission_ng.json", nil), "V1 commission")
	h.approveAndActivate(t, commission, time.Time{})

	return quoteWorld{
		h:        h,
		quotes:   quoteusecases.NewService(quoterepos.NewStore(h.pool), h.policies, h.rollouts),
		where:    where,
		pickup:   pickup,
		tariffID: tariff.ID,
	}
}

func (w quoteWorld) request(ref string, classes ...string) pricingclient.QuoteRequest {
	pickup := w.pickup
	return pricingclient.QuoteRequest{
		ConsumerRef: ref, Scope: pricingclient.ScopeTaxi, Channel: pricingclient.ChannelCustomerApp,
		Classes: classes, Pickup: &pickup,
		Route:          pricingclient.Route{DistanceMeters: 8000, DurationSeconds: 1230, Confidence: pricingclient.RouteConfidenceHigh},
		LegacyFareKobo: map[string]int64{"economy": 165000},
	}
}

const taxiCaller = "driver-taxi-service"

// TestShadowQuoteLifecycle walks issue, idempotent re-issue, bind and
// reconcile, and the refusals around each.
func TestShadowQuoteLifecycle(t *testing.T) {
	w := newQuoteWorld(t)
	ctx := context.Background()
	ref := unique("booking")

	response, err := w.quotes.Issue(ctx, taxiCaller, w.request(ref, "economy", "rocket"))
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if len(response.Quotes) != 1 || len(response.Unpriced) != 1 || response.Unpriced[0].Class != "rocket" {
		t.Fatalf("want economy priced and rocket unpriced, got %+v", response)
	}
	quote := response.Quotes[0]
	// 8 km and 20m30s in economy is NGN 1,700 under the V1 book, 15% to KarryGo.
	if quote.GrossKobo != 170000 || quote.PlatformFeeKobo != 25500 || quote.ProviderKobo != 144500 {
		t.Fatalf("priced %+v, want gross 170000 / fee 25500 / provider 144500", quote)
	}
	if quote.Mode != pricingclient.ModeShadow || quote.TariffVersionID != w.tariffID {
		t.Fatalf("mode %s tariff %s, want SHADOW from %s", quote.Mode, quote.TariffVersionID, w.tariffID)
	}
	if quote.DifferenceKobo == nil || *quote.DifferenceKobo != 5000 {
		t.Fatalf("difference = %v, want +5000 against the legacy 165000", quote.DifferenceKobo)
	}

	t.Run("asking again with the same reference returns the same quote", func(t *testing.T) {
		again, err := w.quotes.Issue(ctx, taxiCaller, w.request(ref, "economy"))
		if err != nil || len(again.Quotes) != 1 || again.Quotes[0].ID != quote.ID {
			t.Fatalf("re-issue gave %+v, %v; want quote %s", again, err, quote.ID)
		}
		found, err := w.quotes.FindByRef(ctx, taxiCaller, ref, "economy")
		if err != nil || found.ID != quote.ID {
			t.Fatalf("find by ref gave %+v, %v", found, err)
		}
	})

	t.Run("another caller cannot see or bind it", func(t *testing.T) {
		if _, err := w.quotes.FindByRef(ctx, "driver-hauling-service", ref, "economy"); !errors.Is(err, quotemodels.ErrNotFound) {
			t.Fatalf("want not found for another caller, got %v", err)
		}
		if _, err := w.quotes.Bind(ctx, "driver-hauling-service", quote.ID, "b1"); !errors.Is(err, quotemodels.ErrNotFound) {
			t.Fatalf("want not found binding another caller's quote, got %v", err)
		}
	})

	t.Run("reconcile refuses an unbound quote", func(t *testing.T) {
		_, err := w.quotes.Reconcile(ctx, taxiCaller, quote.ID, pricingclient.ReconcileRequest{Route: pricingclient.Route{DistanceMeters: 9000, DurationSeconds: 1300}})
		if !errors.Is(err, quotemodels.ErrNotBound) {
			t.Fatalf("want ErrNotBound, got %v", err)
		}
	})

	t.Run("bind is idempotent and exclusive", func(t *testing.T) {
		first, err := w.quotes.Bind(ctx, taxiCaller, quote.ID, ref)
		if err != nil {
			t.Fatalf("bind: %v", err)
		}
		again, err := w.quotes.Bind(ctx, taxiCaller, quote.ID, ref)
		if err != nil || !again.BoundAt.Equal(first.BoundAt) {
			t.Fatalf("re-bind gave %+v, %v", again, err)
		}
		if _, err := w.quotes.Bind(ctx, taxiCaller, quote.ID, "another-booking"); !errors.Is(err, quotemodels.ErrBindConflict) {
			t.Fatalf("want ErrBindConflict for a second booking, got %v", err)
		}
	})

	t.Run("reconcile prices the actual route with the original versions, once", func(t *testing.T) {
		legacy := int64(195000)
		first, err := w.quotes.Reconcile(ctx, taxiCaller, quote.ID, pricingclient.ReconcileRequest{
			Route: pricingclient.Route{DistanceMeters: 10000, DurationSeconds: 1500}, LegacyFinalKobo: &legacy,
		})
		if err != nil {
			t.Fatalf("reconcile: %v", err)
		}
		if first.GrossKobo <= quote.GrossKobo {
			t.Fatalf("a longer actual trip should cost more: %d vs quoted %d", first.GrossKobo, quote.GrossKobo)
		}
		if first.DifferenceKobo == nil || *first.DifferenceKobo != first.GrossKobo-legacy {
			t.Fatalf("difference %v against legacy final %d", first.DifferenceKobo, legacy)
		}
		again, err := w.quotes.Reconcile(ctx, taxiCaller, quote.ID, pricingclient.ReconcileRequest{
			Route: pricingclient.Route{DistanceMeters: 99000, DurationSeconds: 9000},
		})
		if err != nil || again.GrossKobo != first.GrossKobo {
			t.Fatalf("second reconcile changed the answer: %+v, %v", again, err)
		}
	})

	t.Run("the shadow summary counts it", func(t *testing.T) {
		rows, err := w.quotes.ShadowSummary(ctx, time.Now().UTC().Add(-time.Hour))
		if err != nil {
			t.Fatalf("summary: %v", err)
		}
		for _, row := range rows {
			if row.Scope == "TAXI" && row.Class == "economy" && row.Compared > 0 && row.Reconciled > 0 {
				return
			}
		}
		t.Fatalf("summary has no compared, reconciled TAXI/economy row: %+v", rows)
	})
}

// TestQuotePlaceResolution covers the caller's own zone mapping and a pickup
// outside every zone.
func TestQuotePlaceResolution(t *testing.T) {
	w := newQuoteWorld(t)
	ctx := context.Background()

	consumerZone := unique("taxi-zone")
	if err := w.h.geo.MapConsumerZone(ctx, ids.New(ids.PrefixZoneMapping), taxiCaller, consumerZone, w.where.zoneID, w.h.editor,
		time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatalf("map consumer zone: %v", err)
	}
	request := w.request(unique("ref"), "economy")
	request.Pickup = nil
	request.ConsumerZoneID = consumerZone
	response, err := w.quotes.Issue(ctx, taxiCaller, request)
	if err != nil || len(response.Quotes) != 1 || response.Quotes[0].TariffVersionID != w.tariffID {
		t.Fatalf("a mapped zone should price with its rates: %+v, %v", response, err)
	}

	nowhere := w.request(unique("ref"), "economy")
	far := pricingclient.Point{Lat: w.pickup.Lat + 1, Lng: w.pickup.Lng}
	nowhere.Pickup = &far
	if _, err := w.quotes.Issue(ctx, taxiCaller, nowhere); !errors.Is(err, quotemodels.ErrNoMarket) {
		t.Fatalf("want ErrNoMarket a degree away from the only nearby zone, got %v", err)
	}
}

// TestIssuedQuotesCannotBeEdited proves the database itself refuses to change a
// quote after it was issued.
func TestIssuedQuotesCannotBeEdited(t *testing.T) {
	w := newQuoteWorld(t)
	ctx := context.Background()
	response, err := w.quotes.Issue(ctx, taxiCaller, w.request(unique("ref"), "economy"))
	if err != nil || len(response.Quotes) != 1 {
		t.Fatalf("issue: %+v, %v", response, err)
	}
	if _, err := w.h.pool.Exec(ctx, `UPDATE pricing_quotes SET gross_kobo = 1 WHERE id = $1`, response.Quotes[0].ID); err == nil {
		t.Fatal("an issued quote was edited; the append-only guard is missing")
	}
	if _, err := w.h.pool.Exec(ctx, `DELETE FROM pricing_quotes WHERE id = $1`, response.Quotes[0].ID); err == nil {
		t.Fatal("an issued quote was deleted outside the retention purge")
	}
}

// TestQuoteRoutesOverSignedHTTP proves the production wiring: a quote caller is
// served through the shared client, and a service off the quote allowlist is
// refused even with a valid signature.
func TestQuoteRoutesOverSignedHTTP(t *testing.T) {
	w := newQuoteWorld(t)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpx.RequestID(), httpx.Recovery(), httpx.ErrorHandler())
	quotehttp.RegisterRoutes(router, quotehttp.Options{
		Handler: quotehttp.NewHandler(w.quotes),
		Verifier: serviceauth.NewVerifier(serviceauth.Secrets{
			taxiCaller:                 []byte("taxi-secret"),
			"admin-backoffice-service": []byte("admin-secret"),
		}, 5*time.Minute),
		NonceStore:   w.h.pool,
		QuoteCallers: []string{taxiCaller},
		AdminCallers: []string{"admin-backoffice-service"},
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	client := pricingclient.New(pricingclient.Options{BaseURL: server.URL, ServiceName: taxiCaller, Secret: "taxi-secret"})
	ref := unique("http-booking")
	response, err := client.Quote(context.Background(), w.request(ref, "economy"))
	if err != nil || len(response.Quotes) != 1 || response.Quotes[0].GrossKobo != 170000 {
		t.Fatalf("quote over HTTP: %+v, %v", response, err)
	}
	if _, err := client.Bind(context.Background(), response.Quotes[0].ID, pricingclient.BindRequest{BookingRef: ref}); err != nil {
		t.Fatalf("bind over HTTP: %v", err)
	}
	found, err := client.FindByRef(context.Background(), ref, "economy")
	if err != nil || found.ID != response.Quotes[0].ID {
		t.Fatalf("find by ref over HTTP: %+v, %v", found, err)
	}

	admin := pricingclient.New(pricingclient.Options{BaseURL: server.URL, ServiceName: "admin-backoffice-service", Secret: "admin-secret"})
	if _, err := admin.Quote(context.Background(), w.request(unique("ref"), "economy")); !errors.Is(err, pricingclient.ErrRefused) {
		t.Fatalf("an admin caller must not be able to quote, got %v", err)
	}
}
