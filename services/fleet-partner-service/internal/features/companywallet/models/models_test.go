package models

import "testing"

// Section 15 reference types must not be accepted before Section 15 exists.
// Accepting one would allow money to move against work this system cannot yet
// describe, so the guard is asserted here rather than assumed from the schema.
func TestSection15ReferenceTypesAreRejected(t *testing.T) {
	for _, name := range []string{"PRIVATE_ORDER", "SHIPMENT", "ORDER", "PACKAGE", "MANIFEST"} {
		if ValidBusinessReferenceType(name) {
			t.Errorf("business reference type %q must not be accepted before Section 15", name)
		}
	}
}

func TestSupportedReferenceTypes(t *testing.T) {
	for _, name := range []string{"TRIP", "ASSIGNMENT", "PAYROLL_REFERENCE",
		"EXPENSE_REFERENCE", "MANUAL_APPROVED_REFERENCE", "trip"} {
		if !ValidBusinessReferenceType(name) {
			t.Errorf("business reference type %q should be accepted", name)
		}
	}
}

func TestSupportedAllocationTypes(t *testing.T) {
	for _, name := range []string{"SALARY", "WAGE", "PER_JOB", "ALLOWANCE", "REIMBURSEMENT",
		"BONUS", "ADJUSTMENT_CREDIT", "APPROVED_DEDUCTION", "REVERSAL"} {
		if !ValidAllocationType(name) {
			t.Errorf("allocation type %q should be accepted", name)
		}
	}
	if ValidAllocationType("COMMISSION") {
		t.Error("COMMISSION is not a company allocation type")
	}
}

func TestDeductionsDebitTheProvider(t *testing.T) {
	if !DebitsProvider("APPROVED_DEDUCTION") || !DebitsProvider("REVERSAL") {
		t.Fatal("deductions and reversals must debit the provider")
	}
	for _, name := range []string{"SALARY", "PER_JOB", "BONUS", "ALLOWANCE"} {
		if DebitsProvider(name) {
			t.Errorf("%s must credit the provider", name)
		}
	}
}

// A per-job allocation may only be raised against work that can be shown to
// have finished, so it is bound to a trip or assignment reference.
func TestPerJobRequiresCompletedWork(t *testing.T) {
	if !RequiresCompletedWork("PER_JOB", "TRIP") {
		t.Fatal("a per-job allocation against a trip must require completed work")
	}
	if RequiresCompletedWork("SALARY", "PAYROLL_REFERENCE") {
		t.Fatal("salary is not tied to one unit of completed work")
	}
}

// A link that is ACTIVE but whose local copy is not CURRENT must not authorize
// anything: the account may have been restricted since, and the freshness
// state exists precisely because Fleet cannot tell.
func TestOnlyCurrentActiveLinksAreOperational(t *testing.T) {
	if !OperationalLink(LinkActive, FreshnessCurrent) {
		t.Fatal("an active, current link must be operational")
	}
	for _, freshness := range []string{FreshnessStale, FreshnessUnknown,
		FreshnessReconciling, FreshnessGapDetected} {
		if OperationalLink(LinkActive, freshness) {
			t.Errorf("an active link with %s freshness must not be operational", freshness)
		}
	}
	for _, status := range []string{LinkSuspended, LinkRestricted, LinkUnknown, LinkStale,
		LinkReconciling, LinkFailedRetryable, LinkFailedPermanent, LinkClosed, LinkDisabled} {
		if OperationalLink(status, FreshnessCurrent) {
			t.Errorf("a %s link must not be operational", status)
		}
	}
}

// An organization that never configured a policy gets the strict one. An
// unconfigured control must not be an open one.
func TestDefaultPolicyRequiresApproval(t *testing.T) {
	policy := DefaultWithdrawalPolicy("org_1")
	if policy.PolicyMode != PolicyCompanyApproval {
		t.Fatalf("default policy mode = %q, want %q", policy.PolicyMode, PolicyCompanyApproval)
	}
	if !policy.RequiresApproval(1) {
		t.Fatal("the default policy must require approval")
	}
	if !policy.AllowsProviderRequest() {
		t.Fatal("the default policy still lets a provider open a request")
	}
}

func TestPolicyModes(t *testing.T) {
	direct := WithdrawalPolicy{PolicyMode: PolicyProviderDirect}
	if direct.RequiresApproval(1_000) {
		t.Fatal("PROVIDER_DIRECT must not require approval below any threshold")
	}
	withThreshold := WithdrawalPolicy{PolicyMode: PolicyProviderDirect,
		ApprovalThresholdMinor: 500}
	if !withThreshold.RequiresApproval(500) {
		t.Fatal("a request at the threshold must require approval")
	}
	if withThreshold.RequiresApproval(499) {
		t.Fatal("a request below the threshold must not require approval")
	}
	companyOnly := WithdrawalPolicy{PolicyMode: PolicyCompanyInitiated}
	if companyOnly.AllowsProviderRequest() {
		t.Fatal("COMPANY_INITIATED_ONLY must not let a provider open a request")
	}
	disabled := WithdrawalPolicy{PolicyMode: PolicyDisabled}
	if disabled.AllowsProviderRequest() {
		t.Fatal("WITHDRAWALS_DISABLED must not let a provider open a request")
	}
}

// A deduction always needs approval regardless of amount: it takes money from
// somebody who is not present to object.
func TestDeductionAlwaysRequiresApproval(t *testing.T) {
	policy := WithdrawalPolicy{PolicyMode: PolicyProviderDirect,
		AllocationApprovalThresholdMinor: 0}
	if !policy.RequiresAllocationApproval(1, AllocationApprovedDeduction) {
		t.Fatal("an approved deduction must always require approval")
	}
	if policy.RequiresAllocationApproval(1_000_000, AllocationSalary) {
		t.Fatal("with no threshold configured a salary allocation needs no approval")
	}
	thresholded := WithdrawalPolicy{AllocationApprovalThresholdMinor: 100_000}
	if !thresholded.RequiresAllocationApproval(100_000, AllocationSalary) {
		t.Fatal("an allocation at the threshold must require approval")
	}
}

func TestTerminalAllocationStatuses(t *testing.T) {
	for _, status := range []string{AllocationCompleted, AllocationFailedPermanent,
		AllocationRejected, AllocationReversed, AllocationDeadLetter} {
		if !TerminalAllocationStatus(status) {
			t.Errorf("%s must be terminal", status)
		}
	}
	for _, status := range []string{AllocationDraft, AllocationPendingApproval, AllocationApproved,
		AllocationProcessing, AllocationUnknown, AllocationFailedRetryable, AllocationReconciling} {
		if TerminalAllocationStatus(status) {
			t.Errorf("%s must not be terminal", status)
		}
	}
}

func TestDecidableWithdrawalStatuses(t *testing.T) {
	if !DecidableWithdrawal(WithdrawalRequested) || !DecidableWithdrawal(WithdrawalPendingApproval) {
		t.Fatal("a requested or pending withdrawal must accept a decision")
	}
	for _, status := range []string{WithdrawalApproved, WithdrawalRejected, WithdrawalCompleted,
		WithdrawalCancelled, WithdrawalFailedPermanent, WithdrawalDeadLetter} {
		if DecidableWithdrawal(status) {
			t.Errorf("%s must not accept a second decision", status)
		}
	}
}
