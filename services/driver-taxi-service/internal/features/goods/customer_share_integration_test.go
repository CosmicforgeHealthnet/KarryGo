package goods

import (
	"context"
	"cosmicforge/logistics/shared/go/journeyshare"
	"cosmicforge/logistics/shared/go/serviceauth"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGoodsCustomerShareOwnershipPrivacyAndBoundary(t *testing.T) {
	db := goodsTestDB(t)
	f := seedGoodsRaceFixture(t, db)
	ctx := context.Background()
	var ref, owner string
	if err := db.QueryRow(ctx, `SELECT mission_ref,customer_ref FROM taxi_goods_missions WHERE id=$1`, f.mission).Scan(&ref, &owner); err != nil {
		t.Fatal(err)
	}
	s := NewService(Options{Repository: NewRepository(db), Enabled: true})
	secret := []byte("goods-share-local-test")
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{"vendor-service": secret, "customer-service": secret}, 5*time.Minute)
	engine := gin.New()
	RegisterCustomerShare(engine, s, verifier)
	server := httptest.NewServer(engine)
	defer server.Close()
	client := &journeyshare.Client{Caller: "vendor-service", Endpoints: map[string]journeyshare.Endpoint{"TAXI_GOODS": {BaseURL: server.URL, Secret: secret}}, Paths: map[string]string{"TAXI_GOODS": "/internal/taxi/v1/commerce-share/missions/"}}
	p, err := client.Read(ctx, "TAXI_GOODS", ref, owner)
	if err != nil || !p.Eligible || p.Location.Available || p.Location.State != "UNAVAILABLE" {
		t.Fatal(p, err)
	}
	raw, _ := json.Marshal(p.Public([]string{"route", "provider", "location", "eta"}))
	for _, private := range []string{owner, ref, f.booking, "otp", "qr", "evidence", "payment", "receiver_phone", "accepted_fee"} {
		if strings.Contains(string(raw), private) {
			t.Fatal("leak", private)
		}
	}
	if _, err = client.Read(ctx, "TAXI_GOODS", ref, "foreign"); err == nil {
		t.Fatal("IDOR")
	}
	request, _ := http.NewRequest("GET", server.URL+"/internal/taxi/v1/commerce-share/missions/"+ref+"?customer_id="+owner, nil)
	if err = serviceauth.SignRequest(request, "customer-service", secret, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("wrong caller accepted", response.StatusCode)
	}
	request, _ = http.NewRequest("GET", server.URL+"/internal/taxi/v1/commerce-share/missions/"+ref+"?customer_id="+owner, nil)
	_ = serviceauth.SignRequest(request, "vendor-service", secret, nil, time.Now())
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode < 400 {
		t.Fatal("nonce replay accepted")
	}
	if _, err = db.Exec(ctx, `UPDATE taxi_goods_missions SET status='COMPLETED',version=version+1,completed_at=now() WHERE id=$1`, f.mission); err != nil {
		t.Fatal(err)
	}
	p, err = client.Read(ctx, "TAXI_GOODS", ref, owner)
	if err != nil || !p.Terminal || p.Eligible || p.SourceVersion < 2 || p.Location.Available {
		t.Fatal(p, err)
	}
	s.enabled = false
	if _, err = s.CustomerShare(ctx, ref, owner); err == nil {
		t.Fatal("flag ignored")
	}
}
