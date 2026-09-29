package fulfilmentgroup

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestStackOwnDeliveryShare drives Vendor Own Delivery through the RUNNING
// local stack: Customer N01/N06 -> running Vendor N11 -> canonical runtime.
// The runtime transitions use Vendor's own use cases; the mission location
// stays UNAVAILABLE because Vendor has no authoritative GPS for it.
func TestStackOwnDeliveryShare(t *testing.T) {
	if os.Getenv("SECTIONS911_STACK_E2E") != "1" {
		t.Skip("SECTIONS911_STACK_E2E=1 and a running local stack are required")
	}
	secret := []byte(os.Getenv("SECTIONS911_STACK_SECRET"))
	customerDB, err := pgxpool.New(context.Background(), os.Getenv("SECTIONS911_STACK_CUSTOMER_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer customerDB.Close()
	customer := uuid.NewString()
	if _, err = customerDB.Exec(context.Background(), `INSERT INTO customers(id,email) VALUES($1,$2)`, customer, "e2e-"+customer+"@example.test"); err != nil {
		t.Fatal(err)
	}
	bearer, err := auth.NewTokenSigner(secret).Sign(auth.Claims{Subject: customer, Role: "customer", Service: "customer", SessionID: uuid.NewString(), Type: "access", ExpiresAt: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	f := seedVendorDeliveryRuntimeFor(t, 1, customer)
	call := func(method, url string, headers map[string]string, body any) (int, map[string]any, string) {
		t.Helper()
		var reader io.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			reader = bytes.NewReader(raw)
		}
		req, _ := http.NewRequest(method, url, reader)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		var out map[string]any
		_ = json.Unmarshal(raw, &out)
		data, _ := out["data"].(map[string]any)
		return res.StatusCode, data, string(raw)
	}
	code, data, raw := call("POST", "http://127.0.0.1:58101/api/v1/customer/journey-shares", map[string]string{"Authorization": "Bearer " + bearer, "Idempotency-Key": uuid.NewString()}, map[string]any{"authority": "VENDOR_COMMERCE", "source_ref": f.mission.ID, "product_code": "FOOD"})
	if code != 200 {
		t.Fatalf("N01 own delivery %d %s", code, raw)
	}
	token, _ := data["token"].(string)
	public := func(stage string) map[string]any {
		t.Helper()
		code, data, raw := call("GET", "http://127.0.0.1:58101/api/v1/customer/public/journey-shares/"+token, map[string]string{"X-Forwarded-For": "198.51.100.7"}, nil)
		if code != 200 {
			t.Fatalf("N06 %s %d %s", stage, code, raw)
		}
		for _, private := range []string{customer, f.businessID, f.branchID, f.mission.ID, "handover", "proof", "payment", "staff"} {
			if strings.Contains(raw, private) {
				t.Fatalf("N06 %s leaked %q: %s", stage, private, raw)
			}
		}
		if loc, _ := data["location"].(map[string]any); loc["available"] == true || loc["state"] != "UNAVAILABLE" {
			t.Fatalf("N06 %s fabricated location: %s", stage, raw)
		}
		return data
	}
	if got := public("planned")["public_status"]; got != "PLANNED" {
		t.Fatalf("planned status %v", got)
	}
	mission := assignFixtureRider(t, f, 0)
	if got := public("assigned")["public_status"]; got != "ASSIGNED" {
		t.Fatalf("assigned status %v", got)
	}
	for _, action := range []RiderAction{ActionAcknowledge, ActionStartPickup, ActionArrivePickup, ActionCompletePickup, ActionStartReceiver, ActionArriveReceiver, ActionCompleteDelivery} {
		mission = applyRuntimeAction(t, f, mission, action).Mission
	}
	done := public("completed")
	if done["public_status"] != "COMPLETED" || done["terminal"] != true {
		t.Fatalf("completed %v", done)
	}
	// N11 scope on the running Vendor: foreign owner and wrong caller refused.
	for caller, owner := range map[string]string{"customer-service": uuid.NewString(), "vendor-service": customer} {
		req, _ := http.NewRequest("GET", "http://127.0.0.1:58112/internal/vendor/v1/customer-share/missions/"+f.mission.ID+"?customer_id="+owner, nil)
		if err = serviceauth.SignRequest(req, caller, secret, nil, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		res.Body.Close()
		if res.StatusCode == 200 {
			t.Fatalf("N11 accepted caller=%s owner=%s", caller, owner)
		}
	}
}
