package ref

import (
	"strings"
	"testing"
)

func TestGenerateBookingRefFormat(t *testing.T) {
	r, err := GenerateBookingRef()
	if err != nil {
		t.Fatalf("GenerateBookingRef() error = %v", err)
	}
	if !strings.HasPrefix(r, "KGP-") {
		t.Fatalf("ref = %q, want KGP- prefix", r)
	}
	body := strings.TrimPrefix(r, "KGP-")
	if len(body) != 8 {
		t.Fatalf("ref body len = %d, want 8", len(body))
	}
	// Excludes confusing characters I, O, 0, 1.
	for _, c := range body {
		if strings.ContainsRune("IO01", c) {
			t.Fatalf("ref %q contains a confusing character %q", r, string(c))
		}
		if !strings.ContainsRune(charset, c) {
			t.Fatalf("ref %q contains out-of-charset character %q", r, string(c))
		}
	}
}

func TestGenerateBookingRefIsRandom(t *testing.T) {
	seen := map[string]struct{}{}
	for i := 0; i < 50; i++ {
		r, err := GenerateBookingRef()
		if err != nil {
			t.Fatalf("GenerateBookingRef() error = %v", err)
		}
		if _, dup := seen[r]; dup {
			t.Fatalf("duplicate ref generated: %q", r)
		}
		seen[r] = struct{}{}
	}
}
