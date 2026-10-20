package bridge

import (
	"cosmicforge/logistics/services/admin-backoffice-service/internal/config"
	"cosmicforge/logistics/shared/go/serviceauth"
	"io"
	"strings"
	"testing"
	"time"
)

func TestCanonicalNonceAndIndependentBodyBinding(t *testing.T) {
	f := newFixture(t)
	r := f.request(t, "GET", "/capabilities"+scopeQuery, nil)
	r.Header.Set(serviceauth.HeaderNonce, " "+r.Header.Get(serviceauth.HeaderNonce)+" ")
	if w := invoke(f.b, r); w.Code != 401 {
		t.Fatal("HMAC whitespace nonce alias accepted", w.Code)
	}
	f = newFixture(t)
	r = f.request(t, "POST", "/operations"+scopeQuery, []byte(`{}`))
	body := []byte(`{"performed_by":"forged"}`)
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	if err := serviceauth.SignRequest(r, config.HQPrincipal, f.secret, body, time.Now()); err != nil {
		t.Fatal(err)
	}
	if w := invoke(f.b, r); w.Code != 401 || f.repo.records != 0 {
		t.Fatal("valid workload signature bypassed human request binding", w.Code)
	}
}
