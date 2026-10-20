package adminreview

import (
	"bytes"
	"context"
	"cosmicforge/logistics/shared/go/adminowner"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/customerreview"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/gin-gonic/gin"
)

func TestPostgresCustomerReviewSignedBoundaryAndReadbackKeySeparation(t *testing.T) {
	f := setupReview(t)
	write := bytes.Repeat([]byte{41}, 32)
	read := bytes.Repeat([]byte{42}, 32)
	h := &Handler{Service: f.service, WriteSecret: write, ReadSecret: read}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	h.Register(engine)
	q := adminread.Query{Scope: adminread.Scope{Market: f.scope.Market, Zone: f.scope.Zone, Organization: f.scope.Organization, Tenant: f.scope.Tenant}, Staff: f.actor.Staff, Role: "staff", Purpose: "case-1", PolicyID: f.service.Policy.ID(), PolicyVersion: f.service.Policy.Version(), PolicyChecksum: f.service.Policy.Checksum(), Start: time.Now().UTC().Add(-time.Hour).Truncate(time.Second), End: time.Now().UTC().Truncate(time.Second), Limit: 1}
	raw, err := json.Marshal(f.input)
	if err != nil {
		t.Fatal(err)
	}
	call := func(method, path string, body, key []byte, query adminread.Query) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path+"?"+query.Values().Encode(), bytes.NewReader(body))
		if body == nil {
			req.Body = nil
		}
		if method == "POST" {
			req.Header.Set("Content-Type", "application/json")
		}
		if err := serviceauth.SignRequest(req, adminread.Caller, key, body, time.Now()); err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		return response
	}
	// Read credentials cannot become mutation credentials, and JSON aliases
	// cannot override a signed target after authentication.
	if r := call("POST", customerreview.Path, raw, read, q); r.Code != 401 {
		t.Fatal("read key acquired writes", r.Code, r.Body.String())
	}
	duplicate := bytes.Replace(raw, []byte(`"customer_id":`), []byte(`"customer_id":"`+f.input.CustomerID+`","customer_id":`), 1)
	if r := call("POST", customerreview.Path, duplicate, write, q); r.Code != 400 {
		t.Fatal("duplicate signed target accepted", r.Code, r.Body.String())
	}
	alias := bytes.Replace(raw, []byte(`"customer_id":`), []byte(`"Customer_id":`), 1)
	if r := call("POST", customerreview.Path, alias, write, q); r.Code != 400 {
		t.Fatal("case alias accepted", r.Code, r.Body.String())
	}
	foreign := q
	foreign.Scope.Tenant = "foreign"
	if r := call("POST", customerreview.Path, raw, write, foreign); r.Code != 403 {
		t.Fatal("query tenant supplied authority", r.Code, r.Body.String())
	}
	r := call("POST", customerreview.Path, raw, write, q)
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	var recorded customerreview.Receipt
	if json.Unmarshal(r.Body.Bytes(), &recorded) != nil || recorded.Status != customerreview.Recorded {
		t.Fatal(r.Body.String())
	}
	path := customerreview.Path + "/" + f.input.CustomerID + "/" + f.input.IdempotencyKey
	if r := call("GET", path, nil, write, q); r.Code != 401 {
		t.Fatal("write credential acquired receipt read access", r.Code, r.Body.String())
	}
	f.service.WriteEnabled = false
	r = call("GET", path, nil, read, q)
	if r.Code != 200 {
		t.Fatal("write shutdown disabled safe readback", r.Code, r.Body.String())
	}
	if strings.Contains(r.Body.String(), "PRIVATE_") || strings.Contains(r.Body.String(), f.input.ReasonCode) {
		t.Fatal("private request details leaked")
	}
	// A signed request replay is rejected by the shared durable nonce store.
	req := httptest.NewRequest("GET", path+"?"+q.Values().Encode(), nil)
	if err := serviceauth.SignRequest(req, adminread.Caller, read, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	first := httptest.NewRecorder()
	engine.ServeHTTP(first, req)
	retry := httptest.NewRequest("GET", req.URL.String(), nil)
	retry.Header = req.Header.Clone()
	repeated := httptest.NewRecorder()
	engine.ServeHTTP(repeated, retry)
	if first.Code != 200 || repeated.Code != 409 {
		t.Fatal("readback request nonce replay accepted", first.Code, repeated.Code)
	}
}

func TestCustomerReviewRuntimeOffDoesNotReadSecretFiles(t *testing.T) {
	t.Setenv("CUSTOMER_ADMIN_REVIEW_WRITES_ENABLED", "false")
	t.Setenv("CUSTOMER_ADMIN_REVIEW_READBACK_ENABLED", "false")
	t.Setenv("CUSTOMER_ADMIN_REVIEW_WRITE_SECRET_FILE", "missing-write-key")
	t.Setenv("CUSTOMER_ADMIN_REVIEW_READBACK_SECRET_FILE", "missing-read-key")
	h, err := Runtime(nil)
	if err != nil || h.Service.WriteEnabled || h.Service.ReadEnabled {
		t.Fatal(h, err)
	}
	t.Setenv("CUSTOMER_ADMIN_REVIEW_WRITES_ENABLED", "true")
	if _, err := Runtime(nil); err == nil {
		t.Fatal("write flag invented source authorization dependencies")
	}
}

func TestPostgresCustomerReviewActualTypedClientUsesOriginalReadback(t *testing.T) {
	f := setupReview(t)
	write := bytes.Repeat([]byte{51}, 32)
	read := bytes.Repeat([]byte{52}, 32)
	h := &Handler{Service: f.service, WriteSecret: write, ReadSecret: read}
	engine := gin.New()
	h.Register(engine)
	server := httptest.NewServer(engine)
	defer server.Close()
	client, err := customerreview.NewClient(server.URL, write, read)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	q := adminread.Query{Scope: adminread.Scope{Market: f.scope.Market, Zone: f.scope.Zone, Organization: f.scope.Organization, Tenant: f.scope.Tenant}, Staff: f.actor.Staff, Role: "staff", Purpose: "case-1", PolicyID: f.service.Policy.ID(), PolicyVersion: f.service.Policy.Version(), PolicyChecksum: f.service.Policy.Checksum(), Start: time.Now().UTC().Add(-time.Hour).Truncate(time.Second), End: time.Now().UTC().Truncate(time.Second), Limit: 1}
	out, err := client.Request(context.Background(), q, f.input)
	if err != nil {
		t.Fatal(err)
	}
	if dir := os.Getenv("ADMIN_OWNER_CUSTODY_CAPTURE_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		for name, value := range map[string]any{"customer-review-receipt.json": out, "customer-review-request.json": f.input} {
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	f.service.WriteEnabled = false
	proof, err := client.Readback(context.Background(), q, f.input.CustomerID, f.input.IdempotencyKey)
	if err != nil || proof != out {
		t.Fatal("typed readback changed original action identity", proof, err)
	}
	if _, err := client.Readback(context.Background(), q, f.input.CustomerID, "absent-key"); !errors.Is(err, customerreview.ErrAbsent) {
		t.Fatal("absence became a terminal result or a POST", err)
	}
}

func TestPostgresCustomerReviewRuntimeRequiresDistinctKeysAndNativeSchema(t *testing.T) {
	f := setupReview(t)
	resourceKey := bytes.Repeat([]byte{61}, 32)
	writeKey := bytes.Repeat([]byte{62}, 32)
	readKey := bytes.Repeat([]byte{63}, 32)
	owner := &adminowner.Handler{Enabled: true, DB: f.service.DB, Policy: f.service.Policy, Contexts: f.service.Contexts, Secret: resourceKey}
	dir := t.TempDir()
	writeFile := filepath.Join(dir, "write.key")
	readFile := filepath.Join(dir, "read.key")
	save := func(path string, key []byte) {
		t.Helper()
		if err := os.WriteFile(path, []byte(base64.RawURLEncoding.EncodeToString(key)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	save(writeFile, writeKey)
	save(readFile, readKey)
	t.Setenv("CUSTOMER_ADMIN_REVIEW_WRITES_ENABLED", "true")
	t.Setenv("CUSTOMER_ADMIN_REVIEW_READBACK_ENABLED", "true")
	t.Setenv("CUSTOMER_ADMIN_REVIEW_WRITE_SECRET_FILE", writeFile)
	t.Setenv("CUSTOMER_ADMIN_REVIEW_READBACK_SECRET_FILE", readFile)
	if h, err := Runtime(owner); err != nil || !h.Service.WriteEnabled || !h.Service.ReadEnabled {
		t.Fatal("valid native source configuration refused", err)
	}
	save(writeFile, resourceKey)
	if _, err := Runtime(owner); err == nil {
		t.Fatal("resource key acquired review writes")
	}
	save(writeFile, writeKey)
	save(readFile, writeKey)
	if _, err := Runtime(owner); err == nil {
		t.Fatal("write key acquired independent readback audience")
	}
	save(readFile, readKey)
	t.Setenv("CUSTOMER_ADMIN_REVIEW_WRITES_ENABLED", "false")
	if h, err := Runtime(owner); err != nil || h.Service.WriteEnabled || !h.Service.ReadEnabled {
		t.Fatal("readback-only restart failed", err)
	}
}
