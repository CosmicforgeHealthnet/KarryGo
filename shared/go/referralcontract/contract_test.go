package referralcontract

import (
	"encoding/json"
	"testing"
	"time"
)

func TestActorTypeForWalletProvider(t *testing.T) {
	cases := map[string]string{
		"truck_provider":    ActorHauling,
		"hauling":           ActorHauling,
		"dispatch":          ActorDispatch,
		"dispatch_provider": ActorDispatch,
		"taxi":              ActorTaxi,
		"vendor_business":   "",
		"":                  "",
	}
	for in, want := range cases {
		if got := ActorTypeForWalletProvider(in); got != want {
			t.Fatalf("ActorTypeForWalletProvider(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWalletProviderTypeRoundTrips(t *testing.T) {
	for _, actor := range []string{ActorHauling, ActorDispatch, ActorTaxi} {
		if back := ActorTypeForWalletProvider(WalletProviderTypeForActor(actor)); back != actor {
			t.Fatalf("%s round-tripped to %q", actor, back)
		}
	}
	if WalletProviderTypeForActor(ActorCustomer) != "" {
		t.Fatal("customers have no wallet provider type")
	}
}

func TestHashesAreKeyedAndNormalized(t *testing.T) {
	pepper := []byte("pepper")
	if HashEmail(pepper, " Ada@Example.com ") != HashEmail(pepper, "ada@example.com") {
		t.Fatal("email hash must ignore case and surrounding space")
	}
	if HashPhone(pepper, "+2348012345678") == HashPhone([]byte("other"), "+2348012345678") {
		t.Fatal("hash must depend on the pepper")
	}
	if HashPhone(pepper, "+2348012345678") == HashEmail(pepper, "+2348012345678") {
		t.Fatal("phone and email hashes of the same text must differ")
	}
	if HashPhone(nil, "+2348012345678") != "" || HashPhone(pepper, "") != "" {
		t.Fatal("missing pepper or value must give an empty hash")
	}
	if len(HashDevice(pepper, "device-1")) != 64 {
		t.Fatal("expected a hex SHA-256 digest")
	}
}

func TestReferralCodePrefersLinkCode(t *testing.T) {
	if got := (AccountRegistered{LinkCode: "LINK", TypedCode: "TYPED"}).ReferralCode(); got != "LINK" {
		t.Fatalf("ReferralCode() = %q, want LINK", got)
	}
	if got := (AccountRegistered{TypedCode: "TYPED"}).ReferralCode(); got != "TYPED" {
		t.Fatalf("ReferralCode() = %q, want TYPED", got)
	}
}

func TestValidation(t *testing.T) {
	now := time.Now()
	good := JobSettled{SourceService: "s", SourceReference: "r", AmountKobo: 1000, PlatformFeeKobo: 150, JobCompletedAt: now}
	if err := good.Validate(); err != nil {
		t.Fatalf("valid job rejected: %v", err)
	}
	bad := good
	bad.PlatformFeeKobo = 2000
	if bad.Validate() == nil {
		t.Fatal("fee above amount must be rejected")
	}
	if (AccountRegistered{ActorType: "vendor", ActorID: "1", PhoneHash: "h", SignupAt: now}).Validate() == nil {
		t.Fatal("unknown actor type must be rejected")
	}
	if (AccountRegistered{ActorType: ActorCustomer, ActorID: "1", SignupAt: now}).Validate() == nil {
		t.Fatal("registration without any identifier hash must be rejected")
	}
	if (ProviderOnboardingCompleted{ActorType: ActorCustomer, ActorID: "1", CompletedAt: now}).Validate() == nil {
		t.Fatal("customers cannot complete driver onboarding")
	}
}

// TestWireFormatIsStable pins the JSON field names. Producers and the
// consumer share these structs, so a renamed field would otherwise silently
// zero a value on the other side of an old deployment.
func TestWireFormatIsStable(t *testing.T) {
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	raw, err := json.Marshal(JobSettled{
		PaymentIntentID: "pi", PaymentReference: "ref", SourceService: "svc", SourceReference: "job",
		CustomerID: "c", ProviderID: "p", ProviderType: "taxi", AmountKobo: 1000, PlatformFeeKobo: 150,
		ProviderNetKobo: 850, Currency: "NGN", JobCompletedAt: at, JobCompletedAtSource: CompletedAtFromProvider, SettledAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"paymentIntentId":"pi","paymentReference":"ref","sourceService":"svc","sourceReference":"job","customerId":"c","providerId":"p","providerType":"taxi","amountKobo":1000,"platformFeeKobo":150,"providerNetKobo":850,"currency":"NGN","jobCompletedAt":"2026-09-01T10:00:00Z","jobCompletedAtSource":"provider","settledAt":"2026-09-01T10:00:00Z"}`
	if string(raw) != want {
		t.Fatalf("JobSettled wire format changed:\n got %s\nwant %s", raw, want)
	}

	raw, err = json.Marshal(AccountRegistered{
		ActorType: ActorHauling, ActorID: "h-1", PhoneHash: "ph", LinkCode: "ABC", SignupAt: at, IsNewAccount: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	const wantAccount = `{"actorType":"hauling","actorId":"h-1","phoneHash":"ph","linkCode":"ABC","signupAt":"2026-09-01T10:00:00Z","isNewAccount":true}`
	if string(raw) != wantAccount {
		t.Fatalf("AccountRegistered wire format changed:\n got %s\nwant %s", raw, wantAccount)
	}
}

func TestFeedRefusesToRunWithoutAPepper(t *testing.T) {
	if _, err := NewFeed(nil, "customer-service", nil); err == nil {
		t.Fatal("a feed without the shared pepper would emit unmatchable hashes")
	}
	if _, err := NewFeed(nil, "", []byte("p")); err == nil {
		t.Fatal("a feed must know which service it speaks for")
	}
}
