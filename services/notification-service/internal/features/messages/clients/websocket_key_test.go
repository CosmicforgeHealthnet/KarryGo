package messageclients

import "testing"

func TestRealtimeRecipientKeysNeverCollideAcrossAudiences(t *testing.T) {
	pairs := [][2][2]string{
		{{"customer", "a:b"}, {"customer:a", "b"}},
		{{"provider", "x"}, {"provider:", "x"}},
		{{"fleet_organization", "org-1"}, {"customer", "org-1"}},
	}
	for _, pair := range pairs {
		a := realtimeRecipientKey(pair[0][0], pair[0][1])
		b := realtimeRecipientKey(pair[1][0], pair[1][1])
		if a == b {
			t.Errorf("%v and %v share realtime bucket %q", pair[0], pair[1], a)
		}
	}
}
