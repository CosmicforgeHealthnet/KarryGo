package customerreview

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/google/uuid"
)

func TestCustomerReviewClientRejectsUnboundProofAndNeverRepostsReadback(t *testing.T) {
	key := bytes.Repeat([]byte{41}, 32)
	scope := adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "org-1", Tenant: "tenant-1"}
	q := adminread.Query{Scope: adminread.Scope{Market: scope.Market, Zone: scope.Zone, Organization: scope.Organization, Tenant: scope.Tenant}, Staff: "staff-1", Role: "staff", Purpose: "case-1", PolicyID: "policy-1", PolicyVersion: 1, PolicyChecksum: strings.Repeat("a", 64), Start: time.Now().UTC().Add(-time.Hour).Truncate(time.Second), End: time.Now().UTC().Truncate(time.Second), Limit: 1}
	customer := uuid.NewString()
	proof := Receipt{ContractVersion: Contract, Status: Recorded, ReviewRequestID: uuid.NewString(), RequestID: uuid.NewString(), CustomerID: customer, IdempotencyKey: "original-key", IntentDigest: strings.Repeat("a", 64), RecordedReviewRevision: 2, Scope: scope, RecordedAt: time.Now().UTC()}
	for _, mode := range []string{"valid", "tenant", "key", "extra", "duplicate", "absent", "redirect", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			var hits, posts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				if r.Method != "GET" {
					posts.Add(1)
				}
				if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					t.Error("browser credentials forwarded")
				}
				if _, err := serviceauth.NewVerifier(serviceauth.Secrets{adminread.Caller: key}, time.Minute).VerifyRequest(r); err != nil {
					t.Error(err)
					w.WriteHeader(401)
					return
				}
				out := proof
				switch mode {
				case "tenant":
					out.Scope.Tenant = "foreign"
				case "key":
					out.IdempotencyKey = "fresh-key"
				case "absent":
					w.WriteHeader(404)
					return
				case "redirect":
					http.Redirect(w, r, "http://unapproved.example", 302)
					return
				case "oversize":
					w.Write([]byte(strings.Repeat("x", 16385)))
					return
				}
				raw, _ := json.Marshal(out)
				switch mode {
				case "extra":
					raw = append(raw[:len(raw)-1], []byte(`,"storage_url":"private"}`)...)
				case "duplicate":
					raw = bytes.Replace(raw, []byte(`"status":`), []byte(`"status":"REVIEW_REQUEST_RECORDED","status":`), 1)
				}
				w.Write(raw)
			}))
			defer server.Close()
			client, err := NewClient(server.URL, nil, key)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			_, err = client.Readback(context.Background(), q, customer, "original-key")
			if (mode == "valid") != (err == nil) {
				t.Fatal("malformed or unbound proof decision", mode, err)
			}
			if hits.Load() != 1 || posts.Load() != 0 {
				t.Fatal("readback repeated or became a mutation", hits.Load(), posts.Load())
			}
		})
	}
}

func TestCustomerReviewRequestRejectsCaseAliasesAndNoncanonicalTargets(t *testing.T) {
	input := Request{ContractVersion: Contract, RequestID: uuid.NewString(), CustomerID: uuid.NewString(), ExpectedReviewRevision: 1, IdempotencyKey: "original-key", ReasonCode: "ACCOUNT_REVIEW", RequestedAt: time.Now().UTC().Add(-time.Second)}
	raw, _ := json.Marshal(input)
	if _, err := DecodeRequest(raw); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range [][]byte{bytes.Replace(raw, []byte(`"customer_id":`), []byte(`"CUSTOMER_ID":`), 1), append(append([]byte{}, raw...), raw...), bytes.Replace(raw, []byte(`"request_id":`), []byte(`"request_id":"`+input.RequestID+`","request_id":`), 1)} {
		if _, err := DecodeRequest(invalid); err == nil {
			t.Fatal("ambiguous signed intent accepted")
		}
	}
	for _, origin := range []string{"http://owner.example/arbitrary", "http://user:password@owner.example", "file:///owner", "http://owner.example?target=x"} {
		if _, err := NewClient(origin, nil, bytes.Repeat([]byte{41}, 32)); err == nil {
			t.Fatal("arbitrary proxy origin accepted")
		}
	}
}
