package supportcase

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// bannedContextWords are the shapes of data Support must never receive through
// this contract: where somebody was, how to reach them, and what anybody paid.
var bannedContextWords = []string{
	"lat", "lng", "latitude", "longitude", "location", "coordinate", "geo", "route",
	"polyline", "gps", "address", "phone", "email", "plate", "registration",
	"amount", "fare", "price", "kobo", "fee", "payout", "note", "evidencebytes",
}

func fieldNamesOf(value reflect.Type, prefix string, out *[]string) {
	for value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return
	}
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		name := prefix + field.Name
		*out = append(*out, name)
		fieldNamesOf(field.Type, name+".", out)
	}
}

// TestOperationalContextCarriesNoLocationContactOrMoney is the privacy guard.
//
// It walks every field, including nested ones, because the likeliest way this
// contract leaks is somebody adding a convenient field to AssetSummary.
func TestOperationalContextCarriesNoLocationContactOrMoney(t *testing.T) {
	names := []string{}
	fieldNamesOf(reflect.TypeOf(OperationalContext{}), "", &names)
	if len(names) < 10 {
		t.Fatalf("walked only %d fields; the guard is not looking at the contract", len(names))
	}
	for _, name := range names {
		lowered := strings.ToLower(name)
		for _, word := range bannedContextWords {
			// "Milestones" legitimately contains nothing banned; match whole
			// field-name segments rather than arbitrary substrings.
			for _, segment := range strings.Split(lowered, ".") {
				if segment == word || strings.HasPrefix(segment, word) ||
					strings.HasSuffix(segment, word) {
					t.Errorf("OperationalContext field %s looks like %q data", name, word)
				}
			}
		}
	}
}

// TestTheGuardWouldCatchALeak proves the walker is not vacuous.
func TestTheGuardWouldCatchALeak(t *testing.T) {
	type leaky struct{ PickupLatitude float64 }
	names := []string{}
	fieldNamesOf(reflect.TypeOf(leaky{}), "", &names)
	caught := false
	for _, name := range names {
		if strings.Contains(strings.ToLower(name), "latitude") {
			caught = true
		}
	}
	if !caught {
		t.Fatal("the privacy guard would not notice a latitude field")
	}
}

type fixedProjector struct {
	actor   string
	context OperationalContext
	calls   int
}

func (p *fixedProjector) ProjectSupportContext(_ context.Context,
	request ContextRequest) (OperationalContext, error) {
	p.calls++
	if request.ExpectedActorRef != p.actor {
		return OperationalContext{}, ErrContextNotFound
	}
	return p.context, nil
}

func contextServer(t *testing.T, projector ContextProjector) (*httptest.Server, []byte) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	secret := []byte("support-context-test-secret")
	engine := gin.New()
	RegisterContextRoute(engine.Group("/api/v1/taxi"),
		serviceauth.NewVerifier(serviceauth.Secrets{
			ContextCaller:       secret,
			"some-other-service": []byte("other-secret"),
		}, 5*time.Minute), projector)
	server := httptest.NewServer(engine)
	t.Cleanup(server.Close)
	return server, secret
}

func TestContextRoundTrip(t *testing.T) {
	projector := &fixedProjector{actor: "cus_1", context: OperationalContext{
		SourceService: "driver-taxi-service", State: "IN_RECOVERY", SourceVersion: 7,
		RecoveryStatus: "IN_PROGRESS", RecoveryCaseRef: "rc_1",
		ActorRefs: map[string]string{"customer_ref": "cus_1", "provider_ref": "prv_2"},
	}}
	server, secret := contextServer(t, projector)
	client := NewContextClient(server.URL+"/api/v1/taxi", secret, time.Second)

	got, err := client.Read(context.Background(), ContextRequest{
		ContextType: ContextTaxiBooking, SourceRef: "bk_1", ExpectedActorRef: "cus_1"})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.State != "IN_RECOVERY" || got.RecoveryCaseRef != "rc_1" ||
		got.ContextType != ContextTaxiBooking || got.SourceRef != "bk_1" {
		t.Fatalf("round trip lost data: %+v", got)
	}
	if got.SchemaVersion != OperationalContextSchemaVersion {
		t.Fatalf("schema version = %d", got.SchemaVersion)
	}
}

// TestAnotherUsersJobIsIndistinguishableFromNone is the cross-user rule.
func TestAnotherUsersJobIsIndistinguishableFromNone(t *testing.T) {
	server, secret := contextServer(t, &fixedProjector{actor: "cus_1"})
	client := NewContextClient(server.URL+"/api/v1/taxi", secret, time.Second)
	_, err := client.Read(context.Background(), ContextRequest{
		ContextType: ContextTaxiBooking, SourceRef: "bk_1", ExpectedActorRef: "cus_intruder"})
	if err != ErrContextNotFound {
		t.Fatalf("another user's job answered %v, want not-found", err)
	}
}

func TestOnlySupportMayRead(t *testing.T) {
	server, _ := contextServer(t, &fixedProjector{actor: "cus_1"})
	request, _ := http.NewRequest(http.MethodGet,
		server.URL+"/api/v1/taxi/internal/support-context/v1/TAXI_BOOKING/bk_1?actor_ref=cus_1", nil)
	if err := serviceauth.SignRequest(request, "some-other-service",
		[]byte("other-secret"), nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("a non-support caller got %d", response.StatusCode)
	}
}

func TestUnsignedReadIsRefused(t *testing.T) {
	server, _ := contextServer(t, &fixedProjector{actor: "cus_1"})
	response, err := http.Get(server.URL +
		"/api/v1/taxi/internal/support-context/v1/TAXI_BOOKING/bk_1?actor_ref=cus_1")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unsigned read got %d", response.StatusCode)
	}
}

func TestMissingActorIsRefused(t *testing.T) {
	projector := &fixedProjector{actor: "cus_1"}
	server, secret := contextServer(t, projector)
	client := NewContextClient(server.URL+"/api/v1/taxi", secret, time.Second)
	if _, err := client.Read(context.Background(), ContextRequest{
		ContextType: ContextTaxiBooking, SourceRef: "bk_1"}); err == nil {
		t.Fatal("a read with no expected actor was answered")
	}
	if projector.calls != 0 {
		t.Fatal("the projector ran for a request that should have been refused first")
	}
}

// TestOwnerOutageIsNotNotFound keeps "cannot ask" distinct from "no such job".
func TestOwnerOutageIsNotNotFound(t *testing.T) {
	client := NewContextClient("http://127.0.0.1:1/api/v1/taxi", []byte("s"), 200*time.Millisecond)
	_, err := client.Read(context.Background(), ContextRequest{
		ContextType: ContextTaxiBooking, SourceRef: "bk_1", ExpectedActorRef: "cus_1"})
	if err == ErrContextNotFound || err == nil {
		t.Fatalf("an unreachable owner answered %v", err)
	}
}

// TestMinimizeDropsWhatTheContractDoesNotHold proves a snapshot can only ever
// contain contract fields, whatever an owner sends.
func TestMinimizeDropsWhatTheContractDoesNotHold(t *testing.T) {
	var decoded OperationalContext
	if err := json.Unmarshal([]byte(`{"state":"X","pickup_latitude":6.4,"customer_phone":"080"}`),
		&decoded); err != nil {
		t.Fatal(err)
	}
	stored, err := Minimize(decoded)
	if err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{"latitude", "phone", "6.4", "080"} {
		if strings.Contains(string(stored), leaked) {
			t.Fatalf("snapshot kept %q: %s", leaked, stored)
		}
	}
}
