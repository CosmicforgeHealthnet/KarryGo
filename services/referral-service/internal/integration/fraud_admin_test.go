package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/referralcontract"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func referralStatus(t *testing.T, p programme, referred account) string {
	t.Helper()
	var status string
	if err := p.pool.QueryRow(context.Background(), `SELECT status FROM referrals WHERE id = $1`, referralOf(t, p.pool, referred)).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

// Signing up on the referrer's own phone is the classic self-referral with a
// second identity: the referral is held, the new account's sign-up credits
// are frozen, and nothing is paid until an operator clears it.
func TestSharedDeviceWithReferrerHoldsTheReferral(t *testing.T) {
	p := newProgramme(t)
	ada, bola := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorCustomer)
	ada.device, bola.device = "phone-1", "phone-1"
	signUp(t, p.svc, ada, "", "", true)
	signUp(t, p.svc, bola, codeOf(t, p.svc, ada), "", true)
	if st := referralStatus(t, p, bola); st != "under_review" {
		t.Fatalf("referral = %s, want under review", st)
	}
	if n := countRows(t, p.pool, `SELECT count(*) FROM fraud_flags WHERE flag_type = 'duplicate_device' AND referral_id IS NOT NULL AND evidence->>'shared_with_referrer' = 'true'`); n != 1 {
		t.Fatalf("shared-device flags = %d, want 1", n)
	}
	p.decideAndPost(t)
	if got := p.wallet.creditsTo(bola.actorID); len(got) != 0 {
		t.Fatalf("sign-up credits were paid while under review: %+v", got)
	}
	if err := p.svc.ClearReferral(context.Background(), referralOf(t, p.pool, bola), adminReq); err != nil {
		t.Fatal(err)
	}
	p.decideAndPost(t)
	if got := len(p.wallet.creditsTo(bola.actorID)); got != 2 {
		t.Fatalf("sign-up credits after clearing = %d, want 2", got)
	}
}

// Two people brought in by the same referrer on one phone look like one
// person farming referrals: the second referral is held too.
func TestSharedDeviceBetweenOneReferrersReferralsIsHeld(t *testing.T) {
	p := newProgramme(t)
	ada, bola, chika := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorCustomer)
	ada.device, bola.device, chika.device = "phone-a", "phone-b", "phone-b"
	signUp(t, p.svc, ada, "", "", true)
	code := codeOf(t, p.svc, ada)
	signUp(t, p.svc, bola, code, "", true)
	signUp(t, p.svc, chika, code, "", true)
	if bolaStatus, chikaStatus := referralStatus(t, p, bola), referralStatus(t, p, chika); bolaStatus != "active" || chikaStatus != "under_review" {
		t.Fatalf("referrals = %s / %s, want the second one held", bolaStatus, chikaStatus)
	}
}

// A device shared with someone unrelated (a family phone) is flagged for a
// look but blocks nothing.
func TestDeviceSharedWithAStrangerIsOnlyFlagged(t *testing.T) {
	p := newProgramme(t)
	ada, stranger, bola, other := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorCustomer),
		newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorCustomer)
	stranger.device, bola.device, other.device = "family-phone", "family-phone", "family-phone"
	signUp(t, p.svc, ada, "", "", true)
	signUp(t, p.svc, stranger, "", "", true)
	signUp(t, p.svc, bola, codeOf(t, p.svc, ada), "", true)
	signUp(t, p.svc, other, "", "", true)
	if st := referralStatus(t, p, bola); st != "active" {
		t.Fatalf("referral = %s, want active", st)
	}
	if n := countRows(t, p.pool, `SELECT count(*) FROM fraud_flags WHERE flag_type = 'duplicate_device' AND severity = 'review' AND referral_id IS NOT NULL`); n != 1 {
		t.Fatalf("review flags on the referral = %d, want 1", n)
	}
	if n := countRows(t, p.pool, `SELECT count(*) FROM fraud_flags WHERE flag_type = 'duplicate_device' AND severity = 'info' AND referral_id IS NULL`); n != 1 {
		t.Fatalf("informational flags = %d, want 1 (the unreferred sign-up)", n)
	}
}

// A referred customer's first ride with their own referrer cannot qualify
// the referral; their next ride with anyone else does.
func TestRideWithOwnReferrerDoesNotQualify(t *testing.T) {
	p := newProgramme(t)
	ada, bola := newAccount(referralcontract.ActorTaxi), newAccount(referralcontract.ActorCustomer)
	signUp(t, p.svc, ada, "", "", true)
	signUp(t, p.svc, bola, codeOf(t, p.svc, ada), "", true)
	start := p.clock.Now()
	p.paidJob(t, bola.actorID, referralcontract.ActorTaxi, ada.actorID, start.Add(time.Minute), 10_000)
	if status, _ := customerTrack(t, p, bola); status != "pending_qualification" {
		t.Fatalf("a ride with the referrer qualified the referral (%s)", status)
	}
	if n := countRows(t, p.pool, `SELECT count(*) FROM fraud_flags WHERE flag_type = 'collusive_job'`); n != 1 {
		t.Fatalf("collusion flags = %d, want 1", n)
	}
	p.book(t, bola, start.Add(time.Hour))
	if status, _ := customerTrack(t, p, bola); status != "qualified" {
		t.Fatalf("a genuine ride did not qualify the referral (%s)", status)
	}
}

// A referrer booking their own referred driver earns no commission and the
// job does not count towards the driver's activation.
func TestReferrerBookingTheirOwnDriverEarnsNothing(t *testing.T) {
	p := newProgramme(t)
	ada, chidi := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorHauling)
	signUp(t, p.svc, ada, "", "", true)
	signUp(t, p.svc, chidi, codeOf(t, p.svc, ada), "", true)
	start := p.clock.Now()
	p.approve(t, chidi, start)
	p.paidJob(t, ada.actorID, referralcontract.ActorHauling, chidi.actorID, start.Add(time.Minute), 750_000)
	if _, counted, _, _ := p.trackStatus(t, "provider"); counted != 0 {
		t.Fatalf("the referrer's own booking counted towards activation (%d)", counted)
	}
	if n := countRows(t, p.pool, `SELECT count(*) FROM reward_ledger WHERE reward_type = 'provider_commission'`); n != 0 {
		t.Fatal("the referrer earned commission on their own booking")
	}
	if n := countRows(t, p.pool, `SELECT count(*) FROM referral_track_jobs WHERE reason = 'collusive_job'`); n != 1 {
		t.Fatalf("recorded collusive jobs = %d, want 1", n)
	}
}

// The admin surface: only admin callers, a reason for everything, every
// decision audited, and freezes, clears, take-backs and disqualification
// behave as the programme requires.
func TestAdminActionsOverHTTP(t *testing.T) {
	p := newProgramme(t)
	ctx := context.Background()
	adminSecret, walletSecret := []byte("admin-secret"), []byte("wallet-secret")
	router := signedRouter(t, p.pool, p.svc, serviceauth.Secrets{"admin-backoffice-service": adminSecret, "payment-wallet-service": walletSecret})
	call := func(caller string, secret []byte, method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, signedRequest(t, method, path, caller, secret, []byte(body)))
		return recorder
	}
	admin := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		return call("admin-backoffice-service", adminSecret, method, path, body)
	}
	const why = `{"performed_by": "ops-7", "reason": "customer complaint"}`

	ada, bola := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorCustomer)
	signUp(t, p.svc, ada, "", "", true)
	start := p.clock.Now()
	p.bookings(t, ada.actorID, 5, start)
	signUp(t, p.svc, bola, codeOf(t, p.svc, ada), "", true)
	p.book(t, bola, start.Add(time.Minute))
	p.clock.Advance(61 * time.Minute)
	if _, err := p.svc.DecidePendingRewards(ctx, 100); err != nil {
		t.Fatal(err)
	}
	adaPerson := personOf(t, p.pool, ada)
	freeze := "/internal/referral/v1/admin/persons/" + adaPerson + "/freeze"

	if rec := call("payment-wallet-service", walletSecret, http.MethodPost, freeze, why); rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin caller got %d, want 403", rec.Code)
	}
	if rec := admin(http.MethodPost, freeze, `{"performed_by": "ops-7"}`); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("freeze without a reason got %d, want 422: %s", rec.Code, rec.Body)
	}
	if rec := admin(http.MethodPost, freeze, why); rec.Code != http.StatusOK {
		t.Fatalf("freeze got %d: %s", rec.Code, rec.Body)
	}
	if st, _, _ := rewardStatus(t, p.pool, "customer_referral_bonus", ada.actorID); st != "frozen" {
		t.Fatalf("Ada's queued bonus = %s, want frozen", st)
	}
	p.decideAndPost(t)
	if len(p.wallet.creditsTo(ada.actorID)) != 0 {
		t.Fatal("a frozen person was paid")
	}
	if rec := admin(http.MethodPost, freeze, why); rec.Code != http.StatusConflict {
		t.Fatalf("freezing twice got %d, want 409", rec.Code)
	}
	if rec := admin(http.MethodPost, "/internal/referral/v1/admin/persons/"+adaPerson+"/clear", why); rec.Code != http.StatusOK {
		t.Fatalf("clear got %d: %s", rec.Code, rec.Body)
	}
	p.decideAndPost(t)
	if got := len(p.wallet.creditsTo(ada.actorID)); got != 1 {
		t.Fatalf("credits after clearing = %d, want the ₦500", got)
	}

	rec := admin(http.MethodGet, "/internal/referral/v1/admin/accounts/customer/"+ada.actorID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("overview got %d: %s", rec.Code, rec.Body)
	}
	var overview struct {
		Data struct {
			Person struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"person"`
			Referrals                  []struct{ ID string } `json:"referrals"`
			QualifiedCustomerReferrals int                   `json:"qualified_customer_referrals"`
			RecentRewards              []struct {
				ID         string `json:"id"`
				RewardType string `json:"reward_type"`
			} `json:"recent_rewards"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &overview); err != nil {
		t.Fatal(err)
	}
	if overview.Data.Person.ID != adaPerson || overview.Data.Person.Status != "active" || len(overview.Data.Referrals) != 1 || overview.Data.QualifiedCustomerReferrals != 1 {
		t.Fatalf("overview = %+v", overview.Data)
	}
	var bonusID string
	for _, r := range overview.Data.RecentRewards {
		if r.RewardType == "customer_referral_bonus" {
			bonusID = r.ID
		}
	}

	reverse := "/internal/referral/v1/admin/rewards/" + bonusID + "/reverse"
	if rec := admin(http.MethodPost, reverse, why); rec.Code != http.StatusOK {
		t.Fatalf("reverse got %d: %s", rec.Code, rec.Body)
	}
	if rec := admin(http.MethodPost, reverse, why); rec.Code != http.StatusConflict {
		t.Fatalf("reversing twice got %d, want 409", rec.Code)
	}
	p.decideAndPost(t)
	if takeBacks := p.wallet.reversalsTo(ada.actorID); len(takeBacks) != 1 || takeBacks[0].AmountKobo != 50_000 {
		t.Fatalf("take-backs = %+v", takeBacks)
	}

	// Disqualifying with reverse_paid also takes back Bola's paid sign-up
	// credits, and the referral leaves Ada's milestone count.
	disqualify := "/internal/referral/v1/admin/referrals/" + referralOf(t, p.pool, bola) + "/disqualify"
	if rec := admin(http.MethodPost, disqualify, `{"performed_by": "ops-7", "reason": "fake account", "reverse_paid": true}`); rec.Code != http.StatusOK {
		t.Fatalf("disqualify got %d: %s", rec.Code, rec.Body)
	}
	p.decideAndPost(t)
	if got := len(p.wallet.reversalsTo(bola.actorID)); got != 2 {
		t.Fatalf("Bola's credits taken back = %d, want 2", got)
	}
	if view, _ := p.svc.MyCode(ctx, ada.actorType, ada.actorID); view.QualifiedCustomerReferrals != 0 {
		t.Fatalf("a disqualified referral still counts (%d)", view.QualifiedCustomerReferrals)
	}

	if rec := admin(http.MethodPost, "/internal/referral/v1/admin/persons/"+adaPerson+"/explode", why); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown action got %d, want 404", rec.Code)
	}
	if n := countRows(t, p.pool, `SELECT count(*) FROM referral_admin_actions WHERE performed_by = 'ops-7' AND caller = 'admin-backoffice-service'`); n != 4 {
		t.Fatalf("audited actions = %d, want 4 (freeze, clear, reverse, disqualify)", n)
	}
}

// Reviewers work through open flags and resolve each one once.
func TestResolvingFlags(t *testing.T) {
	p := newProgramme(t)
	ctx := context.Background()
	ada, bola := newAccount(referralcontract.ActorCustomer), newAccount(referralcontract.ActorCustomer)
	ada.device, bola.device = "phone-1", "phone-1"
	signUp(t, p.svc, ada, "", "", true)
	signUp(t, p.svc, bola, codeOf(t, p.svc, ada), "", true)
	flags, err := p.svc.ListFlags(ctx, "open", 10)
	if err != nil || len(flags) != 1 {
		t.Fatalf("open flags = %d (%v), want 1", len(flags), err)
	}
	if err := p.svc.ResolveFlag(ctx, flags[0].ID, "maybe", adminReq); err == nil || !strings.Contains(err.Error(), "Check the request") {
		t.Fatalf("an invalid outcome was accepted: %v", err)
	}
	if err := p.svc.ResolveFlag(ctx, flags[0].ID, "cleared", adminReq); err != nil {
		t.Fatal(err)
	}
	if err := p.svc.ResolveFlag(ctx, flags[0].ID, "upheld", adminReq); err == nil {
		t.Fatal("a flag was resolved twice")
	}
	if open, _ := p.svc.ListFlags(ctx, "open", 10); len(open) != 0 {
		t.Fatalf("open flags after resolving = %d", len(open))
	}
	// Resolving a flag is a record, not a release: the referral stays held
	// until it is cleared explicitly.
	if st := referralStatus(t, p, bola); st != "under_review" {
		t.Fatalf("referral = %s after resolving its flag", st)
	}
}
