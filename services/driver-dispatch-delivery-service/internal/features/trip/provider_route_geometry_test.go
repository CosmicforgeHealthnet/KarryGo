package trip

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestProviderRouteGeometryKeepsCredentialsPrivate(t *testing.T) {
	data, err := json.Marshal(Trip{RoutePickupPolyline: "pickup-shape", RouteDropoffPolyline: "delivery-shape", PickupCode: "private-pickup", DeliveryCode: "private-delivery"})
	if err != nil {
		t.Fatal(err)
	}
	value := string(data)
	for _, key := range []string{`"route_pickup_polyline":"pickup-shape"`, `"route_dropoff_polyline":"delivery-shape"`} {
		if !strings.Contains(value, key) {
			t.Fatalf("route field missing: %s", key)
		}
	}
	if strings.Contains(value, "private-pickup") || strings.Contains(value, "private-delivery") {
		t.Fatal("private handover code leaked")
	}
}

func TestProviderRouteGeometrySkipsFailedOwnershipLookup(t *testing.T) {
	repository := &PostgresRepository{}
	item, err := repository.withBookingGeometry(context.Background(), nil, nil)
	if item != nil || err != nil {
		t.Fatal("missing trip should not trigger a geometry query")
	}
	denied := errors.New("not authorized")
	_, err = repository.withBookingGeometry(context.Background(), &Trip{}, denied)
	if !errors.Is(err, denied) {
		t.Fatal("ownership error must be preserved")
	}
}
