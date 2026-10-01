package integration_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	referralhttp "cosmicforge/logistics/services/referral-service/internal/features/referrals/http"
	referralusecases "cosmicforge/logistics/services/referral-service/internal/features/referrals/usecases"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/referralcontract"
)

var testPepper = []byte("test-pepper")

// account is a sign-up as a producer service would announce it.
type account struct {
	actorType string
	actorID   string
	phone     string
	email     string
	device    string
}

func newAccount(actorType string) account {
	return account{actorType: actorType, actorID: actorType + "_" + uuid.NewString(), phone: "+23480" + uuid.NewString()[:8]}
}

func producerFor(actorType string) string {
	switch actorType {
	case referralcontract.ActorHauling:
		return referralusecases.ProducerHauling
	case referralcontract.ActorDispatch:
		return referralusecases.ProducerDispatch
	case referralcontract.ActorTaxi:
		return referralusecases.ProducerTaxi
	}
	return referralusecases.ProducerCustomer
}

// signUp announces an account the way its owning service would.
func signUp(t *testing.T, svc *referralusecases.Service, a account, linkCode, typedCode string, isNew bool) referralusecases.Outcome {
	t.Helper()
	eventType := referralcontract.EventProviderRegistered
	if a.actorType == referralcontract.ActorCustomer {
		eventType = referralcontract.EventCustomerRegistered
	}
	payload := referralcontract.AccountRegistered{
		ActorType: a.actorType, ActorID: a.actorID,
		PhoneHash: referralcontract.HashPhone(testPepper, a.phone), EmailHash: referralcontract.HashEmail(testPepper, a.email),
		DeviceHash: referralcontract.HashDevice(testPepper, a.device),
		LinkCode:   linkCode, TypedCode: typedCode, SignupAt: time.Now().UTC(), IsNewAccount: isNew,
	}
	producer := producerFor(a.actorType)
	return mustHandle(t, svc, producer, envelopeJSON(t, producer, eventType, a.actorID, payload))
}

func codeOf(t *testing.T, svc *referralusecases.Service, a account) string {
	t.Helper()
	view, err := svc.MyCode(context.Background(), a.actorType, a.actorID)
	if err != nil {
		t.Fatalf("MyCode(%s) error = %v", a.actorID, err)
	}
	return view.Code
}

func attributionOutcome(t *testing.T, pool *pgxpool.Pool, a account) string {
	t.Helper()
	var outcome string
	if err := pool.QueryRow(context.Background(), `SELECT attribution_outcome FROM referral_actors WHERE actor_type = $1 AND actor_id = $2`, a.actorType, a.actorID).Scan(&outcome); err != nil {
		t.Fatalf("read outcome: %v", err)
	}
	return outcome
}

// Step 3 "done when": signing up with someone's code links the two people.
func TestSignUpWithCodeCreatesReferral(t *testing.T) {
	pool := testPool(t)
	svc := newService(t, pool)
	ada, bola := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorCustomer)
	signUp(t, svc, ada, "", "", true)
	adaCode := codeOf(t, svc, ada)
	if len(adaCode) != 8 {
		t.Fatalf("code %q is not 8 characters", adaCode)
	}

	// Typed lower case with a dash, as a person would.
	signUp(t, svc, bola, "", strings.ToLower(adaCode[:4])+"-"+strings.ToLower(adaCode[4:]), true)
	if got := attributionOutcome(t, pool, bola); got != "attributed" {
		t.Fatalf("outcome = %s, want attributed", got)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM referrals WHERE code = $1 AND code_source = 'typed'`, adaCode); n != 1 {
		t.Fatalf("referrals = %d, want 1", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM referral_tracks WHERE model = 'customer' AND status = 'pending_qualification'`); n != 1 {
		t.Fatalf("customer tracks = %d, want 1", n)
	}
}

// Step 3 "done when": using your own code is rejected.
func TestOwnCodeIsRejectedAndFlagged(t *testing.T) {
	pool := testPool(t)
	svc := newService(t, pool)
	ada := newAccount(referralcontract.ActorCustomer)
	signUp(t, svc, ada, "", "", true)
	adaAsDriver := newAccount(referralcontract.ActorHauling)
	adaAsDriver.phone = ada.phone
	signUp(t, svc, adaAsDriver, codeOf(t, svc, ada), "", true)

	if got := attributionOutcome(t, pool, adaAsDriver); got != "self_referral" {
		t.Fatalf("outcome = %s, want self_referral", got)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM referrals`); n != 0 {
		t.Fatal("a self-referral created a referral")
	}
	if n := countRows(t, pool, `SELECT count(*) FROM fraud_flags WHERE flag_type = 'self_referral'`); n != 1 {
		t.Fatalf("self-referral flags = %d, want 1", n)
	}
	// The database refuses a self-referral outright, whatever the code does.
	var personID, code string
	_ = pool.QueryRow(context.Background(), `SELECT person_id, code FROM referral_codes LIMIT 1`).Scan(&personID, &code)
	if _, err := pool.Exec(context.Background(), `INSERT INTO referrals (id, referrer_person_id, referred_person_id, code, code_source, attributed_at) VALUES ('x', $1, $1, $2, 'typed', now())`, personID, code); err == nil {
		t.Fatal("the database accepted a self-referral")
	}
}

// Step 3 "done when": the same phone signing up as a customer and later as a
// driver is still one person and one referral -- now with two tracks.
func TestCustomerLaterBecomingDriverIsOneReferral(t *testing.T) {
	pool := testPool(t)
	svc := newService(t, pool)
	ada, bola := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorCustomer)
	signUp(t, svc, ada, "", "", true)
	signUp(t, svc, bola, codeOf(t, svc, ada), "", true)

	bolaDriver := newAccount(referralcontract.ActorDispatch)
	bolaDriver.phone = bola.phone
	signUp(t, svc, bolaDriver, "", "", true) // no code this time
	if got := attributionOutcome(t, pool, bolaDriver); got != "already_referred" {
		t.Fatalf("outcome = %s, want already_referred", got)
	}
	if n := countRows(t, pool, `SELECT count(DISTINCT person_id) FROM referral_actors WHERE actor_id IN ($1, $2)`, bola.actorID, bolaDriver.actorID); n != 1 {
		t.Fatalf("Bola's two accounts map to %d people, want 1", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM referrals`); n != 1 {
		t.Fatalf("referrals = %d, want 1", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM referral_tracks`); n != 2 {
		t.Fatalf("tracks = %d, want customer + provider", n)
	}
	if codeOf(t, svc, bola) != codeOf(t, svc, bolaDriver) {
		t.Fatal("one person must have one code across accounts")
	}
}

// A second referrer's code for someone already referred changes nothing but
// is flagged: first attribution wins, forever.
func TestFirstAttributionWins(t *testing.T) {
	pool := testPool(t)
	svc := newService(t, pool)
	ada, chidi, bola := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorCustomer)
	signUp(t, svc, ada, "", "", true)
	signUp(t, svc, chidi, "", "", true)
	signUp(t, svc, bola, codeOf(t, svc, ada), "", true)

	bolaDriver := newAccount(referralcontract.ActorTaxi)
	bolaDriver.phone = bola.phone
	signUp(t, svc, bolaDriver, codeOf(t, svc, chidi), "", true)
	if n := countRows(t, pool, `SELECT count(*) FROM referrals`); n != 1 {
		t.Fatalf("referrals = %d, want 1", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM fraud_flags WHERE flag_type = 'duplicate_identity'`); n != 1 {
		t.Fatalf("duplicate-identity flags = %d, want 1", n)
	}
}

// Someone already on the platform is not "new", so a code on their second
// account does not make them a referral.
func TestExistingPersonCannotBecomeAReferral(t *testing.T) {
	pool := testPool(t)
	svc := newService(t, pool)
	ada, bola := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorCustomer)
	signUp(t, svc, ada, "", "", true)
	signUp(t, svc, bola, "", "", true) // joined without a code
	bolaDriver := newAccount(referralcontract.ActorHauling)
	bolaDriver.phone = bola.phone
	signUp(t, svc, bolaDriver, codeOf(t, svc, ada), "", true)
	if got := attributionOutcome(t, pool, bolaDriver); got != "existing_person" {
		t.Fatalf("outcome = %s, want existing_person", got)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM referrals`); n != 0 {
		t.Fatal("an existing person became a referral")
	}
}

func TestCodeOutcomes(t *testing.T) {
	pool := testPool(t)
	svc := newService(t, pool)
	ada := newAccount(referralcontract.ActorCustomer)
	signUp(t, svc, ada, "", "", true)
	adaCode := codeOf(t, svc, ada)

	cases := []struct {
		name        string
		link, type_ string
		isNew       bool
		want        string
	}{
		{"no code", "", "", true, "no_code"},
		{"malformed", "", "NOPE", true, "invalid_code"},
		{"well-formed but unknown", "", flipLast(adaCode), true, "invalid_code"},
		{"backfilled account never attributes", adaCode, "", false, "backfill"},
		{"link wins over typed", adaCode, "ZZZZZZZZ", true, "attributed"},
	}
	for _, tc := range cases {
		a := newAccount(referralcontract.ActorCustomer)
		signUp(t, svc, a, tc.link, tc.type_, tc.isNew)
		if got := attributionOutcome(t, pool, a); got != tc.want {
			t.Errorf("%s: outcome = %s, want %s", tc.name, got, tc.want)
		}
		if _, err := svc.MyCode(context.Background(), a.actorType, a.actorID); err != nil {
			t.Errorf("%s: every account gets a code, got %v", tc.name, err)
		}
	}
}

// flipLast changes a code's check character, so it is well-formed-looking
// but not a real code.
func flipLast(code string) string {
	last := code[len(code)-1]
	next := byte('0')
	if last == '0' {
		next = '1'
	}
	return code[:len(code)-1] + string(next)
}

func TestRepeatedAnnouncementIsADuplicate(t *testing.T) {
	pool := testPool(t)
	svc := newService(t, pool)
	ada := newAccount(referralcontract.ActorCustomer)
	signUp(t, svc, ada, "", "", true)
	if outcome := signUp(t, svc, ada, "", "", false); outcome != referralusecases.OutcomeDuplicate {
		t.Fatalf("re-announced account outcome = %s, want duplicate", outcome)
	}
}

// Each service may only announce its own kind of account.
func TestServicesCanOnlyAnnounceTheirOwnAccounts(t *testing.T) {
	pool := testPool(t)
	svc := newService(t, pool)
	payload := referralcontract.AccountRegistered{
		ActorType: referralcontract.ActorTaxi, ActorID: "taxi_" + uuid.NewString(),
		PhoneHash: referralcontract.HashPhone(testPepper, "+2348000000001"), SignupAt: time.Now(), IsNewAccount: true,
	}
	raw := envelopeJSON(t, referralusecases.ProducerHauling, referralcontract.EventProviderRegistered, payload.ActorID, payload)
	if outcome := mustHandle(t, svc, referralusecases.ProducerHauling, raw); outcome != referralusecases.OutcomeRejected {
		t.Fatalf("hauling announcing a taxi account: outcome = %s, want rejected", outcome)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM referral_actors`); n != 0 {
		t.Fatal("a mismatched announcement created an account")
	}
}

// Concurrent sign-ups sharing a phone still resolve to one person.
func TestConcurrentSignUpsWithOnePhoneMakeOnePerson(t *testing.T) {
	pool := testPool(t)
	svc := newService(t, pool)
	phone := "+23481" + uuid.NewString()[:8]
	var wg sync.WaitGroup
	for _, actorType := range []string{referralcontract.ActorCustomer, referralcontract.ActorHauling, referralcontract.ActorDispatch, referralcontract.ActorTaxi} {
		a := newAccount(actorType)
		a.phone = phone
		wg.Add(1)
		go func(a account) {
			defer wg.Done()
			signUp(t, svc, a, "", "", true)
		}(a)
	}
	wg.Wait()
	if n := countRows(t, pool, `SELECT count(*) FROM referral_persons`); n != 1 {
		t.Fatalf("people = %d, want 1", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM referral_codes`); n != 1 {
		t.Fatalf("codes = %d, want 1", n)
	}
}

// The app-facing routes, through the real HTTP stack and real tokens.
func TestMyCodeAndCodeCheckOverHTTP(t *testing.T) {
	pool := testPool(t)
	svc := newService(t, pool)
	customerSecret, haulingSecret, taxiSecret := []byte("customer-secret"), []byte("hauling-secret"), []byte("taxi-secret")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpx.ErrorHandler())
	referralhttp.RegisterPublicRoutes(router.Group("/api/v1/referrals"), referralhttp.PublicOptions{
		Service:              svc,
		CustomerTokenSecret:  customerSecret,
		ProviderTokenSecrets: map[string][]byte{"hauling": haulingSecret, "taxi": taxiSecret},
	})
	ada := newAccount(referralcontract.ActorCustomer)
	signUp(t, svc, ada, "", "", true)
	driver := newAccount(referralcontract.ActorHauling)
	signUp(t, svc, driver, "", "", true)

	token := func(secret []byte, role, service, subject string) string {
		signed, err := sharedauth.NewTokenSigner(secret).Sign(sharedauth.Claims{
			Subject: subject, Role: role, Service: service, SessionID: "s", Type: sharedauth.TokenTypeAccess,
			IssuedAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix(),
		})
		if err != nil {
			t.Fatal(err)
		}
		return signed
	}
	get := func(path, bearer string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}

	rec := get("/api/v1/referrals/me", token(customerSecret, "customer", "customer", ada.actorID))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), codeOf(t, svc, ada)) || !strings.Contains(rec.Body.String(), "https://example.test/r/") {
		t.Fatalf("customer /me: %d %s", rec.Code, rec.Body.String())
	}
	if rec = get("/api/v1/referrals/me", token(haulingSecret, "truck_provider", "hauling", driver.actorID)); rec.Code != http.StatusOK {
		t.Fatalf("driver /me: %d %s", rec.Code, rec.Body.String())
	}
	// A hauling-signed token claiming to be a taxi account is refused.
	if rec = get("/api/v1/referrals/me", token(haulingSecret, "taxi_provider", "taxi", driver.actorID)); rec.Code != http.StatusUnauthorized {
		t.Fatalf("cross-issuer token: %d, want 401", rec.Code)
	}
	if rec = get("/api/v1/referrals/me", ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: %d, want 401", rec.Code)
	}
	if rec = get("/api/v1/referrals/me", token(customerSecret, "customer", "customer", "unknown")); rec.Code != http.StatusNotFound {
		t.Fatalf("unannounced account: %d, want 404", rec.Code)
	}

	if rec = get("/api/v1/referrals/codes/"+codeOf(t, svc, ada), ""); !strings.Contains(rec.Body.String(), `"valid":true`) {
		t.Fatalf("real code check: %s", rec.Body.String())
	}
	if rec = get("/api/v1/referrals/codes/"+flipLast(codeOf(t, svc, ada)), ""); !strings.Contains(rec.Body.String(), `"valid":false`) {
		t.Fatalf("fake code check: %s", rec.Body.String())
	}
}
