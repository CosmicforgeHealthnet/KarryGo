package cursor

import (
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	now := time.Now().UTC()
	token := EncodeTime(now, "brn_abc")
	decoded, ok, err := Decode(token)
	if err != nil || !ok {
		t.Fatalf("Decode() = %v, %v", ok, err)
	}
	if decoded.LastID != "brn_abc" {
		t.Fatalf("LastID = %s", decoded.LastID)
	}
	if !decoded.SortTime().Equal(now) {
		t.Fatalf("SortTime = %s want %s", decoded.SortTime(), now)
	}
}

func TestEmptyTokenMeansFirstPage(t *testing.T) {
	_, ok, err := Decode("")
	if err != nil || ok {
		t.Fatalf("empty token should be first page, got ok=%v err=%v", ok, err)
	}
}

func TestMalformedTokensRejected(t *testing.T) {
	for _, token := range []string{"not-base64!!!", "YWJj", "eyJzIjoiIiwiaWQiOiIifQ", "eyJzIjoibm90LWEtdGltZSIsImlkIjoieCJ9"} {
		if _, _, err := Decode(token); err == nil {
			t.Fatalf("token %q should be rejected", token)
		}
	}
}

func TestClampLimit(t *testing.T) {
	if got := ClampLimit(0); got != DefaultLimit {
		t.Fatalf("ClampLimit(0) = %d", got)
	}
	if got := ClampLimit(1000); got != MaxLimit {
		t.Fatalf("ClampLimit(1000) = %d", got)
	}
	if got := ClampLimit(10); got != 10 {
		t.Fatalf("ClampLimit(10) = %d", got)
	}
}
