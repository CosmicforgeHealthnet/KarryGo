package fuelrelay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/pricingclient"
)

type fakeReporter struct{ got []pricingclient.FuelReport }

func (f *fakeReporter) ReportFuel(_ context.Context, r pricingclient.FuelReport) (pricingclient.FuelReportResult, error) {
	f.got = append(f.got, r)
	return pricingclient.FuelReportResult{MarketCode: "LAGOS-METRO", Duplicate: len(f.got) > 1}, nil
}

func serve(reporter Reporter, signedIn bool, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(httpx.ErrorHandler())
	r.POST(Path, Handler(reporter, func(*gin.Context) (string, bool) { return "driver-1", signedIn }))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, Path, strings.NewReader(body)))
	return w
}

func TestRelaysAReport(t *testing.T) {
	f := &fakeReporter{}
	w := serve(f, true, `{"fuel_type":"pms","price_per_litre_kobo":95000,"lat":6.5244,"lng":3.3792}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"counted":true`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	got := f.got[0]
	if got.ProviderRef != "driver-1" || got.FuelType != pricingclient.FuelPetrol || got.LatE6 != 6_524_400 || got.LngE6 != 3_379_200 {
		t.Fatalf("relayed %+v", got)
	}
}

func TestRefusesBadReports(t *testing.T) {
	f := &fakeReporter{}
	for _, body := range []string{
		`{"fuel_type":"kerosene","price_per_litre_kobo":95000,"lat":6.5,"lng":3.3}`,
		`{"fuel_type":"PETROL","price_per_litre_kobo":950,"lat":6.5,"lng":3.3}`,
		`{"fuel_type":"PETROL","price_per_litre_kobo":95000}`,
	} {
		if w := serve(f, true, body); w.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d", body, w.Code)
		}
	}
	if len(f.got) != 0 {
		t.Fatal("a bad report reached pricing")
	}
	if w := serve(f, false, `{}`); w.Code != http.StatusUnauthorized {
		t.Fatalf("signed out: %d", w.Code)
	}
	if w := serve(nil, true, `{"fuel_type":"PETROL","price_per_litre_kobo":95000,"lat":6.5,"lng":3.3}`); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("no pricing: %d", w.Code)
	}
}
