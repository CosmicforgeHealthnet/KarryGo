package walletclients

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBankDirectoryPagesAndFiltersPayoutBanks(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer test-only" || r.URL.Query().Get("currency") != "NGN" || r.URL.Query().Get("country") != "nigeria" || r.URL.Query().Get("use_cursor") != "true" {
			t.Errorf("missing private credentials/market filters")
		}
		if calls == 1 {
			fmt.Fprint(w, `{"status":true,"data":[{"name":"Zulu","code":"002","currency":"NGN","type":"nuban","active":true},{"name":"Inactive","code":"003","currency":"NGN","type":"nuban","active":false},{"name":"Deleted","code":"004","currency":"NGN","type":"nuban","active":true,"is_deleted":true}],"meta":{"next":"next+cursor="}}`)
		} else {
			if r.URL.Query().Get("next") != "next+cursor=" {
				t.Error("cursor not preserved")
			}
			fmt.Fprint(w, `{"status":true,"data":[{"name":"Alpha","code":"001","currency":"NGN","type":"nuban","active":true},{"name":"Zulu","code":"002","currency":"NGN","type":"nuban","active":true},{"name":"Other market","code":"005","currency":"GHS","type":"ghipss","active":true}],"meta":{"next":null}}`)
		}
	}))
	defer server.Close()
	client := HTTPPaystackClient{BaseURL: server.URL, SecretKey: "test-only", HTTPClient: server.Client()}
	banks, err := client.ListBanks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(banks) != 2 || banks[0].Code != "001" || banks[1].Code != "002" {
		t.Fatalf("unexpected directory: %v calls=%d", banks, calls)
	}
}

func TestBankDirectoryNeverReturnsPartialList(t *testing.T) {
	for _, mode := range []string{"failure", "repeated_cursor"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if mode == "failure" && calls > 1 {
					w.WriteHeader(503)
					return
				}
				fmt.Fprint(w, `{"status":true,"data":[{"name":"First","code":"001","currency":"NGN","type":"nuban","active":true}],"meta":{"next":"same"}}`)
			}))
			defer server.Close()
			banks, err := (HTTPPaystackClient{BaseURL: server.URL, SecretKey: "test-only", HTTPClient: server.Client()}).ListBanks(context.Background())
			if err == nil || banks != nil {
				t.Fatalf("partial directory returned: %v %v", banks, err)
			}
		})
	}
}
