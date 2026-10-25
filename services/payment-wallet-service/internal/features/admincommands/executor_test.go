package admincommands

import "testing"

func TestWalletCommandDictionaryHasNoDirectLedgerOrCompletionSetter(t *testing.T) {
	spec := Spec()
	for _, required := range []string{"RESTRICT_ACCOUNT", "REQUEST_REFUND", "APPROVE_REFUND", "REQUEST_REVERSAL", "APPROVE_REVERSAL", "REQUEST_SETTLEMENT", "APPROVE_SETTLEMENT", "APPROVE_PROVIDER_WITHDRAWAL", "REJECT_PROVIDER_WITHDRAWAL"} {
		if _, ok := spec.Actions[required]; !ok {
			t.Errorf("missing owner-backed action %s", required)
		}
	}
	for _, forbidden := range []string{"ADJUST_BALANCE", "FORCE_PAID", "FORCE_REFUND", "FORCE_SETTLEMENT", "POST_LEDGER", "MARK_PAYOUT_COMPLETE"} {
		if _, ok := spec.Actions[forbidden]; ok {
			t.Errorf("unsafe generic financial setter registered: %s", forbidden)
		}
	}
	for action, operation := range spec.Actions {
		if operation.VersionOf == "" {
			t.Errorf("%s is not source/case version fenced", action)
		}
		if (action == "APPROVE_REFUND" || action == "APPROVE_REVERSAL" || action == "APPROVE_SETTLEMENT" || action == "APPROVE_PROVIDER_WITHDRAWAL" || action == "REJECT_PROVIDER_WITHDRAWAL") && !operation.RequiresApproval {
			t.Errorf("%s bypasses independent Bridge approval", action)
		}
	}
}
