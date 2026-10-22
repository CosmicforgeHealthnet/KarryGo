package admincommands

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	contexthttp "cosmicforge/logistics/services/customer-service/internal/features/customer_context/http"
	contextusecases "cosmicforge/logistics/services/customer-service/internal/features/customer_context/usecases"
	mobilityrepositories "cosmicforge/logistics/services/customer-service/internal/features/mobility_context/repositories"
	"cosmicforge/logistics/shared/go/customerstanding"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// TestPostgresAccountStandingScopeDrivesSharedClient runs the real
// customer-service route on real standing rows and reads it through the
// shared booking-authority client: eligible, held, and lifted (with no
// caching between reads).
func TestPostgresAccountStandingScopeDrivesSharedClient(t *testing.T) {
	h := setup(t)
	gin.SetMode(gin.TestMode)
	scopes, err := contextusecases.ParseCallerScopes("driver-hauling-service=account_standing")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	contexthttp.RegisterInternalRoutes(router.Group("/api/v1/customer"),
		contextusecases.NewService(scopes, mobilityrepositories.NewPostgresRepository(h.db), nil).WithStanding(StandingReader{DB: h.db}),
		serviceauth.NewVerifier(serviceauth.Secrets{"driver-hauling-service": []byte("hauling-secret")}, 5*time.Minute))
	srv := httptest.NewServer(router)
	defer srv.Close()
	client, err := customerstanding.NewClient(customerstanding.Config{BaseURL: srv.URL, ServiceName: "driver-hauling-service", Secret: []byte("hauling-secret")})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	got, err := client.Check(ctx, h.customer)
	if err != nil || !got.BookingEligible || got.StandingVersion != 0 || got.AsOf.IsZero() {
		t.Fatalf("initial standing = %+v %v", got, err)
	}
	hold := h.cmd("RESTRICT_ACCOUNT", 0, map[string]string{"restriction": "BOOKING_HOLD"}, "")
	h.approve(&hold, "staff-b")
	h.mustExec(hold)
	got, err = client.Check(ctx, h.customer)
	if !errors.Is(err, customerstanding.ErrBookingHold) || got.Restriction != "BOOKING_HOLD" || got.StandingVersion != 1 {
		t.Fatalf("hold standing = %+v %v", got, err)
	}
	lift := h.cmd("LIFT_RESTRICTION", 1, nil, "")
	h.approve(&lift, "staff-b")
	h.mustExec(lift)
	got, err = client.Check(ctx, h.customer)
	if err != nil || !got.BookingEligible || got.StandingVersion != 2 {
		t.Fatalf("lifted standing = %+v %v", got, err)
	}

	// An ungranted caller is refused by the owner and the client fails closed.
	other, _ := customerstanding.NewClient(customerstanding.Config{BaseURL: srv.URL, ServiceName: "driver-taxi-service", Secret: []byte("x")})
	if _, err := other.Check(ctx, h.customer); !errors.Is(err, customerstanding.ErrUnavailable) {
		t.Fatalf("unauthenticated caller must fail closed, got %v", err)
	}
}
