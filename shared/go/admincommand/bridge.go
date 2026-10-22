package admincommand

import (
	"time"

	"cosmicforge/logistics/shared/go/adminpolicy"
)

// Submission is the HQ-facing command body on the private Bridge. It carries
// only an approval identity; the Bridge attaches the approval it stored and
// verified, so HQ can never present a self-made approval object to an owner.
type Submission struct {
	ContractVersion string            `json:"contract_version"`
	OperationID     string            `json:"operation_id"`
	SubjectKind     string            `json:"subject_kind"`
	SubjectID       string            `json:"subject_id"`
	CaseRef         string            `json:"case_ref"`
	Action          string            `json:"action"`
	ExpectedVersion int64             `json:"expected_version"`
	IdempotencyKey  string            `json:"idempotency_key"`
	ReasonCode      string            `json:"reason_code"`
	Params          map[string]string `json:"params"`
	ApprovalID      string            `json:"approval_id"`
	RequestedAt     time.Time         `json:"requested_at"`
}

func (s Submission) Command() Command {
	params := map[string]string{}
	for k, v := range s.Params {
		params[k] = v
	}
	return Command{ContractVersion: s.ContractVersion, OperationID: s.OperationID, SubjectKind: s.SubjectKind, SubjectID: s.SubjectID, CaseRef: s.CaseRef, Action: s.Action, ExpectedVersion: s.ExpectedVersion, IdempotencyKey: s.IdempotencyKey, ReasonCode: s.ReasonCode, Params: params, RequestedAt: s.RequestedAt}
}

// Proposal asks a second, independently delegated staff member to approve
// one exact intent. ApprovalID is client generated for idempotent proposal.
type Proposal struct {
	ContractVersion string            `json:"contract_version"`
	ApprovalID      string            `json:"approval_id"`
	SubjectKind     string            `json:"subject_kind"`
	SubjectID       string            `json:"subject_id"`
	CaseRef         string            `json:"case_ref"`
	Action          string            `json:"action"`
	ExpectedVersion int64             `json:"expected_version"`
	ReasonCode      string            `json:"reason_code"`
	Params          map[string]string `json:"params"`
}

func (p Proposal) Command() Command {
	params := map[string]string{}
	for k, v := range p.Params {
		params[k] = v
	}
	return Command{ContractVersion: p.ContractVersion, SubjectKind: p.SubjectKind, SubjectID: p.SubjectID, CaseRef: p.CaseRef, Action: p.Action, ExpectedVersion: p.ExpectedVersion, ReasonCode: p.ReasonCode, Params: params}
}

type ApprovalDecision struct {
	Decision   string `json:"decision"`
	ReasonCode string `json:"reason_code"`
}

// ApprovalView never exposes requester or approver identities to each other
// beyond the fact that they differ; identities stay in the audit evidence.
type ApprovalView struct {
	ApprovalID      string            `json:"approval_id"`
	Owner           string            `json:"owner"`
	Action          string            `json:"action"`
	SubjectKind     string            `json:"subject_kind"`
	SubjectID       string            `json:"subject_id"`
	CaseRef         string            `json:"case_ref"`
	ExpectedVersion int64             `json:"expected_version"`
	IntentDigest    string            `json:"intent_digest"`
	Status          string            `json:"status"`
	Scope           adminpolicy.Scope `json:"scope"`
	ExpiresAt       time.Time         `json:"expires_at"`
	DecidedAt       *time.Time        `json:"decided_at,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
}

// Operation is the Bridge's durable orchestration record for one command.
type Operation struct {
	OperationID     string            `json:"operation_id"`
	Owner           string            `json:"owner"`
	Action          string            `json:"action"`
	SubjectKind     string            `json:"subject_kind"`
	SubjectID       string            `json:"subject_id"`
	CaseRef         string            `json:"case_ref"`
	ExpectedVersion int64             `json:"expected_version"`
	ApprovalID      string            `json:"approval_id,omitempty"`
	Scope           adminpolicy.Scope `json:"scope"`
	Status          string            `json:"status"`
	RejectionCode   string            `json:"rejection_code,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	AsOf            time.Time         `json:"as_of"`
	Receipt         *Receipt          `json:"receipt,omitempty"`
}

var submissionFields = []string{"contract_version", "operation_id", "subject_kind", "subject_id", "case_ref", "action", "expected_version", "idempotency_key", "reason_code", "params", "approval_id", "requested_at"}
var proposalFields = []string{"contract_version", "approval_id", "subject_kind", "subject_id", "case_ref", "action", "expected_version", "reason_code", "params"}

func DecodeSubmission(raw []byte) (Submission, error) {
	var s Submission
	if err := strict(raw, &s, submissionFields, map[string][]string{"params": nil}); err != nil {
		return Submission{}, err
	}
	if s.ApprovalID != "" && !CanonicalID(s.ApprovalID) {
		return Submission{}, ErrInvalid
	}
	return s, nil
}
func DecodeProposal(raw []byte) (Proposal, error) {
	var p Proposal
	if err := strict(raw, &p, proposalFields, map[string][]string{"params": nil}); err != nil {
		return Proposal{}, err
	}
	if !CanonicalID(p.ApprovalID) {
		return Proposal{}, ErrInvalid
	}
	return p, nil
}
func DecodeDecision(raw []byte) (ApprovalDecision, error) {
	var d ApprovalDecision
	if err := strict(raw, &d, []string{"decision", "reason_code"}, nil); err != nil {
		return ApprovalDecision{}, err
	}
	if d.Decision != "APPROVE" && d.Decision != "REJECT" || !token.MatchString(d.ReasonCode) {
		return ApprovalDecision{}, ErrInvalid
	}
	return d, nil
}
