package integrations

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type resolverFunc func(context.Context, string) ([]net.IPAddr, error)

func (f resolverFunc) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return f(ctx, host)
}

func TestClientSecretsAreRandomHashedAndComparedExactly(t *testing.T) {
	one, err := randomToken(32)
	if err != nil {
		t.Fatal(err)
	}
	two, err := randomToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if one == two || len(one) < 40 {
		t.Fatalf("credentials are not independently random: %q %q", one, two)
	}
	key := []byte("0123456789abcdef0123456789abcdef")
	hashed := hashSecret(key, one)
	if strings.Contains(string(hashed), one) || !secretMatches(key, one, hashed) || secretMatches(key, two, hashed) {
		t.Fatal("secret hashing or constant-time comparison contract failed")
	}
	if !containsExact([]string{ScopeIntegrationContextRead}, ScopeIntegrationContextRead) ||
		containsExact([]string{ScopeIntegrationContextRead + ".admin"}, ScopeIntegrationContextRead) {
		t.Fatal("scope comparison must be exact")
	}
}

func TestWebhookSecretEncryptionRoundTripDoesNotStorePlaintext(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	secret := "webhook-secret-that-must-not-be-stored"
	ciphertext, err := encryptSecret(key, secret)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(ciphertext), secret) {
		t.Fatal("ciphertext contains plaintext secret")
	}
	plain, err := decryptSecret(key, ciphertext)
	if err != nil || plain != secret {
		t.Fatalf("round trip failed: %q %v", plain, err)
	}
	ciphertext[0] ^= 0xff
	if _, err = decryptSecret(key, ciphertext); err == nil {
		t.Fatal("tampered ciphertext was accepted")
	}
}

func TestWebhookSignatureBindsBodyIdentifiersAndReplayWindow(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	body := []byte(`{"eventId":"evt_1","data":{"status":"CONFIRMED"}}`)
	header := signWebhook("sub_1", "del_1", "evt_1", now.Unix(), body,
		[]signingSecret{{Version: 2, Secret: "current"}, {Version: 1, Secret: "previous"}})
	for _, tc := range []struct {
		name    string
		secret  string
		version int
		body    []byte
		now     time.Time
		wantErr bool
	}{
		{name: "current", secret: "current", version: 2, body: body, now: now},
		{name: "previous overlap", secret: "previous", version: 1, body: body, now: now},
		{name: "tampered body", secret: "current", version: 2, body: []byte(`{"eventId":"evt_2"}`), now: now, wantErr: true},
		{name: "wrong delivery", secret: "current", version: 2, body: body, now: now, wantErr: true},
		{name: "expired timestamp", secret: "current", version: 2, body: body, now: now.Add(6 * time.Minute), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deliveryID := "del_1"
			if tc.name == "wrong delivery" {
				deliveryID = "del_other"
			}
			err := VerifyWebhookSignature(tc.secret, tc.version, "sub_1", deliveryID, "evt_1", now.Unix(), tc.body, header, tc.now, 5*time.Minute)
			if (err != nil) != tc.wantErr {
				t.Fatalf("VerifyWebhookSignature() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestDestinationValidatorRejectsSSRFIPv4IPv6AndUnsafeURLs(t *testing.T) {
	v, err := NewDestinationValidator(true, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	v.Resolver = resolverFunc(func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host == "public.example" {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		}
		return nil, errors.New("not found")
	})
	unsafe := []string{
		"http://public.example/hook",
		"file:///etc/passwd",
		"https://user:password@public.example/hook",
		"https://127.0.0.1/hook",
		"https://0.0.0.0/hook",
		"https://10.1.2.3/hook",
		"https://172.16.1.2/hook",
		"https://192.168.1.2/hook",
		"https://169.254.169.254/latest/meta-data",
		"https://[::1]/hook",
		"https://[::]/hook",
		"https://[fe80::1]/hook",
		"https://[fc00::1]/hook",
		"https://[fd00:ec2::254]/hook",
	}
	for _, raw := range unsafe {
		t.Run(raw, func(t *testing.T) {
			if _, _, err := v.Validate(context.Background(), raw); !errors.Is(err, ErrUnsafeDestination) {
				t.Fatalf("unsafe destination accepted: %s (%v)", raw, err)
			}
		})
	}
	if _, _, err = v.Validate(context.Background(), "https://public.example/hook"); err != nil {
		t.Fatalf("public HTTPS destination rejected: %v", err)
	}
}

func TestDestinationValidatorDNSRebindingIsRevalidated(t *testing.T) {
	lookups := 0
	v, _ := NewDestinationValidator(false, true, nil)
	v.Resolver = resolverFunc(func(_ context.Context, _ string) ([]net.IPAddr, error) {
		lookups++
		if lookups == 1 {
			return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	})
	if _, _, err := v.Validate(context.Background(), "http://rebind.example/hook"); err != nil {
		t.Fatalf("initial safe DNS answer rejected: %v", err)
	}
	if _, _, err := v.Validate(context.Background(), "http://rebind.example/hook"); !errors.Is(err, ErrUnsafeDestination) {
		t.Fatalf("rebound private answer accepted: %v", err)
	}
}

func TestSecureHTTPClientRevalidatesRedirectDestinations(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data", http.StatusFound)
	}))
	defer server.Close()
	serverPort := strings.Split(strings.TrimPrefix(server.URL, "http://"), ":")[1]
	v, _ := NewDestinationValidator(false, true, nil)
	v.AllowTestLoopback = true
	v.Resolver = resolverFunc(func(_ context.Context, host string) ([]net.IPAddr, error) {
		if host == "safe.test" {
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		}
		return nil, errors.New("not found")
	})
	client := secureHTTPClient(v, 2*time.Second)
	resp, err := client.Get("http://safe.test:" + serverPort + "/hook")
	if resp != nil {
		resp.Body.Close()
	}
	if !errors.Is(err, ErrUnsafeDestination) {
		t.Fatalf("unsafe redirect was not rejected: %v", err)
	}
}

func TestWebhookPayloadRejectsSecretsRecursively(t *testing.T) {
	if err := validateWebhookPayload([]byte(`{"data":{"nested":[{"access_token":"leak"}]}}`)); err == nil {
		t.Fatal("prohibited nested credential field accepted")
	}
	if err := validateWebhookPayload([]byte(`{"data":{"status":"CONFIRMED","amountMinor":100}}`)); err != nil {
		t.Fatalf("safe payload rejected: %v", err)
	}
}

func TestParseRetryAfterSupportsSecondsDatesAndBounds(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	if got := parseRetryAfter("45", now); got != 45*time.Second {
		t.Fatalf("seconds Retry-After = %s", got)
	}
	if got := parseRetryAfter(now.Add(2*time.Minute).Format(http.TimeFormat), now); got != 2*time.Minute {
		t.Fatalf("date Retry-After = %s", got)
	}
	if got := parseRetryAfter("999999", now); got != 24*time.Hour {
		t.Fatalf("unbounded Retry-After = %s", got)
	}
	for _, raw := range []string{"", "invalid", "-1"} {
		if got := parseRetryAfter(raw, now); got != 0 {
			t.Fatalf("Retry-After %q = %s, want zero", raw, got)
		}
	}
}
