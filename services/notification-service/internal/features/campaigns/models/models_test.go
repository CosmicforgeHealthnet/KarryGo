package campaignmodels

import (
	"testing"

	"cosmicforge/logistics/shared/go/admincampaign"
)

func TestResolveStateNeverClaimsDelivery(t *testing.T) {
	cases := []struct {
		c    Counts
		want string
	}{
		{Counts{Audience: 2, Planned: 2}, StatePlanned},
		{Counts{Audience: 2, Planned: 1, Queued: 1}, StateQueued},
		{Counts{Audience: 2, Queued: 1, Sent: 1}, StateSent},
		{Counts{Audience: 2, Sent: 2}, StateCompleted},
		{Counts{Audience: 1, Failed: 1}, StateFailed},
		{Counts{Audience: 1, LoggedOnly: 1}, StateCompleted},
		{Counts{Audience: 2, Suppressed: 2}, StateSuppressed},
		{Counts{}, StateCompleted},
	}
	for _, tc := range cases {
		if got := ResolveState(StatePlanned, tc.c); got != tc.want || got == "DELIVERED" {
			t.Errorf("%+v: %s want %s", tc.c, got, tc.want)
		}
	}
	if ResolveState(StateCancelled, Counts{Audience: 1, Sent: 1}) != StateCancelled {
		t.Fatal("terminal state changed")
	}
}

func TestContentHashAndKeysAndPairs(t *testing.T) {
	if ContentSHA256("a", "b") == ContentSHA256("a\x00", "b") || len(ContentSHA256("t", "b")) != 64 {
		t.Fatal("content hash")
	}
	if MessageIdempotencyKey("c", 3, "r", "push") != "campaign:c:3:r:push" {
		t.Fatal("key")
	}
	pairs := Pairs(admincampaign.Intent{Markets: []string{"NG-LAG", "NG-ABJ"}, Channels: []string{"push", "email"}})
	if len(pairs) != 4 || pairs[0] != (Pair{"NG-ABJ", "email"}) {
		t.Fatalf("pairs: %v", pairs)
	}
	if s := (Revision{CampaignID: "c", Revision: 1, State: StateSent}).Status(Revision{}.UpdatedAt); s.DeliveredCount != nil || s.DeliveryConfirmation != "UNAVAILABLE" || s.ConversionReporting != "UNAVAILABLE" {
		t.Fatalf("status: %+v", s)
	}
}
