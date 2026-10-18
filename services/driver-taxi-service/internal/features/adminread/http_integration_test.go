package adminread

import (
	"context"
	"cosmicforge/logistics/services/taxi-service/internal/database"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	contract "cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ownerDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TAXI_ADMIN_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TAXI_ADMIN_TEST_DATABASE_URL required for owner integration")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil || !strings.Contains(cfg.ConnConfig.Database, "test") {
		t.Fatal("isolated test database required")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "taxi_admin_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); admin.Close() })
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	_, file, _, _ := runtime.Caller(0)
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(filepath.Clean(filepath.Join(filepath.Dir(file), "../../.."))); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(previous)
	// Exercise Taxi's actual tracking/transaction runner twice, including the
	// new nonce/audit migration, rather than a synthetic table fixture.
	for pass := 0; pass < 2; pass++ {
		if err = database.ApplyMigrations(ctx, pool); err != nil {
			t.Fatalf("owner migration pass %d: %v", pass, err)
		}
	}
	return pool
}
func seedRide(t *testing.T, db *pgxpool.Pool, market, zone, org string, at time.Time) string {
	t.Helper()
	ctx := context.Background()
	var mid, zid string
	if err := db.QueryRow(ctx, `INSERT INTO markets(country,name,code) VALUES('test','test',$1) ON CONFLICT(code) DO UPDATE SET code=excluded.code RETURNING id::text`, market).Scan(&mid); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO operating_zones(market_id,name,code,center_lat,center_lng) VALUES($1,'test',$2,0,0) ON CONFLICT(market_id,code) DO UPDATE SET code=excluded.code RETURNING id::text`, mid, zone).Scan(&zid); err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	_, err := db.Exec(ctx, `INSERT INTO taxi_bookings(id,customer_id,market_id,operating_zone_id,organization_id,is_corporate_managed,membership_id,fleet_assignment_id,status,source_context,created_at,updated_at,ride_notes) VALUES($1::uuid,$2,$3,$4,NULLIF($5,''),$6,'membership-test',$1::text,'completed','FLEET_ASSIGNMENT',$7,$7,'SECRET_CONTACT_AND_GPS')`, id, uuid.NewString(), mid, zid, org, org != "", at)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
func TestTaxiAdminOwnerSnapshotIsolationAndReplay(t *testing.T) {
	db := ownerDB(t)
	secret := []byte(strings.Repeat("s", 32))
	now := time.Now().UTC().Truncate(time.Second)
	first := seedRide(t, db, "M1", "Z1", "O1", now.Add(-time.Hour))
	second := seedRide(t, db, "M1", "Z1", "O1", now.Add(-time.Hour))
	seedRide(t, db, "M2", "Z1", "O1", now.Add(-time.Hour))
	seedRide(t, db, "M1", "Z2", "O1", now.Add(-time.Hour))
	seedRide(t, db, "M1", "Z1", "O2", now.Add(-time.Hour))
	seedRide(t, db, "M1", "Z1", "", now.Add(-time.Hour))
	_, err := db.Exec(context.Background(), `INSERT INTO booking_events(booking_id,event_type,actor_type,actor_id,metadata) VALUES($1,'completed','provider','SECRET_ACTOR','{"otp":"SECRET_OTP","gps":"SECRET_GPS"}')`, first)
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	root := gin.New()
	Register(root, db, secret, true)
	srv := httptest.NewServer(root)
	defer srv.Close()
	q := contract.Query{Scope: contract.Scope{Market: "M1", Zone: "Z1", Organization: "O1"}, Staff: "staff-1", Purpose: "case-1", Start: now.Add(-24 * time.Hour), End: now, Limit: 1}
	request := func(q contract.Query, id, caller string) *http.Request {
		path := contract.TaxiPath
		if id != "" {
			path += "/" + id
		}
		r, _ := http.NewRequest("GET", srv.URL+path+"?"+q.Values().Encode(), nil)
		if err := serviceauth.SignRequest(r, caller, secret, nil, time.Now()); err != nil {
			t.Fatal(err)
		}
		return r
	}
	read := func(r *http.Request, want int) contract.TaxiSnapshot {
		res, e := srv.Client().Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		if res.StatusCode != want {
			t.Fatalf("status=%d want=%d: %s", res.StatusCode, want, raw)
		}
		if strings.Contains(string(raw), "SECRET") {
			t.Fatalf("private data escaped: %s", raw)
		}
		var out contract.TaxiSnapshot
		if want == 200 && json.Unmarshal(raw, &out) != nil {
			t.Fatal(string(raw))
		}
		return out
	}
	r := request(q, "", contract.Caller)
	one := read(r, 200)
	read(r.Clone(context.Background()), 409)
	if len(one.Items) != 1 || one.NextCursor == "" || one.Items[0].SourceVersion != nil || one.Coverage != "PARTIAL_COVERAGE" {
		t.Fatalf("snapshot=%+v", one)
	}
	q.Cursor = one.NextCursor
	two := read(request(q, "", contract.Caller), 200)
	if len(two.Items) != 1 || two.Items[0].ID == one.Items[0].ID || two.NextCursor != "" {
		t.Fatalf("pagination=%+v", two)
	}
	ids := map[string]bool{one.Items[0].ID: true, two.Items[0].ID: true}
	if !ids[first] || !ids[second] {
		t.Fatal("foreign record or missing tied timestamp row")
	}
	for _, field := range []string{"staff", "market", "zone", "organization", "purpose", "window", "tamper"} {
		t.Run(field, func(t *testing.T) {
			bad := q
			switch field {
			case "staff":
				bad.Staff = "staff-2"
			case "market":
				bad.Scope.Market = "M2"
			case "zone":
				bad.Scope.Zone = "Z2"
			case "organization":
				bad.Scope.Organization = "O2"
			case "purpose":
				bad.Purpose = "other"
			case "window":
				bad.Start = bad.Start.Add(time.Hour)
			case "tamper":
				bad.Cursor = "x" + bad.Cursor
			}
			read(request(bad, "", contract.Caller), 400)
		})
	}
	q.Cursor = ""
	detail := read(request(q, first, contract.Caller), 200)
	if len(detail.Events) != 1 || detail.Events[0].Type != "completed" {
		t.Fatal("real event missing")
	}
	q.Scope.Organization = "O2"
	read(request(q, first, contract.Caller), 404)
	q.Scope.Organization = "O1"
	read(request(q, "", "customer-service"), 401)
	// A replay race across independent HTTP connections has one winner.
	signed := request(q, "", contract.Caller)
	var wg sync.WaitGroup
	codes := make(chan int, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, e := srv.Client().Do(signed.Clone(context.Background()))
			if e != nil {
				codes <- 0
				return
			}
			io.Copy(io.Discard, res.Body)
			res.Body.Close()
			codes <- res.StatusCode
		}()
	}
	wg.Wait()
	close(codes)
	ok, denied := 0, 0
	for code := range codes {
		if code == 200 {
			ok++
		} else if code == 409 {
			denied++
		}
	}
	if ok != 1 || denied != 7 {
		t.Fatalf("replay race=%d/%d", ok, denied)
	}
	// Source audit fails closed and cannot be mutated by ordinary DML.
	if _, err := db.Exec(context.Background(), `UPDATE taxi_admin_read_audit SET purpose_code='rewritten'`); err == nil {
		t.Fatal("audit mutable")
	}
	if _, err := db.Exec(context.Background(), `ALTER TABLE taxi_admin_read_audit RENAME TO unavailable_audit`); err != nil {
		t.Fatal(err)
	}
	read(request(q, "", contract.Caller), 503)
}

func TestTaxiAdminDisabledWithoutPersistence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := gin.New()
	Register(root, nil, nil, false)
	w := httptest.NewRecorder()
	root.ServeHTTP(w, httptest.NewRequest("GET", contract.TaxiPath, nil))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
}
