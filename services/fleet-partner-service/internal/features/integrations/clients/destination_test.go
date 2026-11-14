package clients

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
	client := SecureHTTPClient(v, 2*time.Second)
	resp, err := client.Get("http://safe.test:" + serverPort + "/hook")
	if resp != nil {
		resp.Body.Close()
	}
	if !errors.Is(err, ErrUnsafeDestination) {
		t.Fatalf("unsafe redirect was not rejected: %v", err)
	}
}

func TestParseRetryAfterSupportsSecondsDatesAndBounds(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	if got := ParseRetryAfter("45", now); got != 45*time.Second {
		t.Fatalf("seconds Retry-After = %s", got)
	}
	if got := ParseRetryAfter(now.Add(2*time.Minute).Format(http.TimeFormat), now); got != 2*time.Minute {
		t.Fatalf("date Retry-After = %s", got)
	}
	if got := ParseRetryAfter("999999", now); got != 24*time.Hour {
		t.Fatalf("unbounded Retry-After = %s", got)
	}
	for _, raw := range []string{"", "invalid", "-1"} {
		if got := ParseRetryAfter(raw, now); got != 0 {
			t.Fatalf("Retry-After %q = %s, want zero", raw, got)
		}
	}
}
