package financeops

import (
	"encoding/json"
	"testing"
)

func validSubmission() ManagedSettlementSubmission {
	return ManagedSettlementSubmission{
		InstructionID: "inst_1", InstructionVersion: 1,
		OrganizationID: "org_1", MembershipID: "mem_1",
		ProviderID: "prov_1", ProviderType: "taxi",
		SourceService: "taxi", SourceJobID: "job_1",
		Currency: "NGN", GrossBasisMinor: 9000,
		ProviderShareMinor: 1500, OrganizationShareMinor: 7500,
		CompensationPlanID: "plan_1", CompensationPlanVersion: 2,
		CompensationResolutionVersion: 1,
		IdempotencyKey:                "settlement:job_1",
	}
}

func TestSubmissionRefusesAnAllocationThatDoesNotSum(t *testing.T) {
	submission := validSubmission()
	if err := submission.Validate(); err != nil {
		t.Fatalf("a coherent submission must validate: %v", err)
	}
	// A residual that belongs to nobody is how money goes missing quietly.
	submission.OrganizationShareMinor = 7000
	if err := submission.Validate(); err == nil {
		t.Fatal("an allocation whose parts do not sum to the basis must be refused")
	}
}

func TestSubmissionRequiresEveryIdentity(t *testing.T) {
	mutations := map[string]func(*ManagedSettlementSubmission){
		"no instruction": func(s *ManagedSettlementSubmission) { s.InstructionID = "" },
		"zero version":   func(s *ManagedSettlementSubmission) { s.InstructionVersion = 0 },
		"no org":         func(s *ManagedSettlementSubmission) { s.OrganizationID = "" },
		"no provider":    func(s *ManagedSettlementSubmission) { s.ProviderID = "" },
		"no type":        func(s *ManagedSettlementSubmission) { s.ProviderType = "" },
		"no job":         func(s *ManagedSettlementSubmission) { s.SourceJobID = "" },
		"bad currency":   func(s *ManagedSettlementSubmission) { s.Currency = "NG" },
		"no key":         func(s *ManagedSettlementSubmission) { s.IdempotencyKey = "" },
		"negative share": func(s *ManagedSettlementSubmission) { s.ProviderShareMinor = -1 },
	}
	for name, mutate := range mutations {
		submission := validSubmission()
		mutate(&submission)
		if err := submission.Validate(); err == nil {
			t.Fatalf("%s: expected the submission to be refused", name)
		}
	}
}

// TestSubmissionCarriesFleetsResolvedFiguresVerbatim is Invariant 3 on the
// wire: what leaves Fleet is what Fleet resolved, so Wallet has nothing to
// recompute from.
func TestSubmissionCarriesFleetsResolvedFiguresVerbatim(t *testing.T) {
	submission := validSubmission()
	raw, err := submission.payload()
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	for field, want := range map[string]float64{
		"gross_basis_minor":        9000,
		"provider_share_minor":     1500,
		"organization_share_minor": 7500,
	} {
		got, ok := body[field].(float64)
		if !ok || got != want {
			t.Fatalf("%s = %v, want %v", field, body[field], want)
		}
	}
	// No rate, no basis mode, no plan rules: Wallet is given answers, never
	// the inputs it would need to reach a different one.
	for _, forbidden := range []string{"rate_bps", "commission_basis", "rules", "basis_mode", "platform_share_minor"} {
		if _, present := body[forbidden]; present {
			t.Fatalf("submission carries %q: Wallet must not be able to recalculate compensation", forbidden)
		}
	}
	if body["compensation_plan_id"] != "plan_1" || body["compensation_plan_version"].(float64) != 2 {
		t.Fatalf("the plan snapshot must travel with the instruction: %v", body)
	}
}

func TestManagedSettlementOperationIsItsOwnPostingType(t *testing.T) {
	// Sharing PROVIDER_ALLOCATION's operation type would make the two
	// indistinguishable in the posting-operation table and in read-back.
	if OperationManagedWorkSettlement == "PROVIDER_ALLOCATION" || OperationManagedWorkSettlement == "" {
		t.Fatal("managed work settlement needs a distinct operation type")
	}
}
