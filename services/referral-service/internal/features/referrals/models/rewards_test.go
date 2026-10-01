package referralmodels

import "testing"

func TestCommissionRoundsDownAndNeverExceedsTheFee(t *testing.T) {
	cases := []struct{ fee, bps, want int64 }{
		{750_000, 1_000, 75_000}, // ₦7,500 platform cut -> ₦750
		{1_234_567, 1_000, 123_456},
		{9, 1_000, 0},      // under a kobo rounds to nothing
		{100, 20_000, 100}, // a misconfigured rate can never pay more than the fee
		{0, 1_000, 0},
		{500, 0, 0},
	}
	for _, tc := range cases {
		if got := Commission(tc.fee, tc.bps); got != tc.want {
			t.Errorf("Commission(%d, %d) = %d, want %d", tc.fee, tc.bps, got, tc.want)
		}
	}
}

func TestRewardKeysComeFromBusinessIdentity(t *testing.T) {
	a := RewardKey(RewardProviderCommission, "ref_1", "svc", "job_1")
	if a != RewardKey(RewardProviderCommission, "ref_1", "svc", "job_1") {
		t.Fatal("the same reward must always get the same key")
	}
	if a == RewardKey(RewardProviderCommission, "ref_1", "svc", "job_2") || a == RewardKey(RewardCustomerReferralBonus, "ref_1") {
		t.Fatal("different rewards must get different keys")
	}
}

func TestReversalTarget(t *testing.T) {
	commission := Reward{RewardType: RewardProviderCommission, AmountKobo: 75_000, BasisKobo: 750_000, RateBPS: 1_000}
	bonus := Reward{RewardType: RewardCustomerReferralBonus, AmountKobo: 50_000}
	tests := []struct {
		name        string
		reward      Reward
		feeReversed int64
		fully       bool
		want        int64
	}{
		{"full reversal takes everything", commission, 0, true, 75_000},
		{"partial fee reversal takes its share", commission, 300_000, false, 30_000},
		{"cumulative fee is capped at the basis", commission, 900_000, false, 75_000},
		{"rounds down like the commission", commission, 9, false, 0},
		{"no fee reversed, nothing to take", commission, 0, false, 0},
		{"flat bonus survives a partial refund", bonus, 500_000, false, 0},
		{"flat bonus goes on a full refund", bonus, 0, true, 50_000},
		{"a take-back row is never itself reversed", Reward{RewardType: RewardReversal, AmountKobo: -10}, 0, true, 0},
	}
	for _, tt := range tests {
		if got := ReversalTarget(tt.reward, tt.feeReversed, tt.fully); got != tt.want {
			t.Errorf("%s: ReversalTarget() = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestMilestonesInOrder(t *testing.T) {
	m := Rules{Milestone5CreditKobo: 1, Milestone10BonusKobo: 2}.Milestones()
	if len(m) != 2 || m[0].Referrals != 5 || m[0].Balance != BalancePromotional || m[0].Badge ||
		m[1].Referrals != 10 || m[1].Balance != BalanceEarnings || !m[1].Badge || m[1].AmountKobo != 2 {
		t.Fatalf("milestones = %+v", m)
	}
}
