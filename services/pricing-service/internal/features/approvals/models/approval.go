// Package models holds the approval domain: a request for a material change and
// the single decision made on it.
package models

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Subject types that require approval. Each is a change that can affect every
// transaction in a market, which is why one person may not make it alone.
const (
	SubjectPolicyVersion = "POLICY_VERSION"
	SubjectRollback      = "ROLLBACK"
	SubjectRollout       = "ROLLOUT"
	SubjectMarketUnfreze = "MARKET_UNFREEZE"
	SubjectIndexBatch    = "INDEX_BATCH"
	SubjectIndexOverride = "INDEX_OVERRIDE"
)

// Request statuses.
const (
	StatusPending   = "PENDING"
	StatusApproved  = "APPROVED"
	StatusRejected  = "REJECTED"
	StatusCancelled = "CANCELLED"
)

// Errors the usecase layer maps to HTTP responses.
var (
	// ErrSelfApproval is the four-eye guarantee. The database enforces it too;
	// this exists so the caller gets a sentence rather than a constraint name.
	ErrSelfApproval = errors.New("approvals: the requester of a change may not decide it")
	// ErrAlreadyDecided prevents a second approver overwriting the first.
	ErrAlreadyDecided = errors.New("approvals: this request has already been decided")
	// ErrSubjectChanged means the thing being approved is no longer what the
	// approver was shown.
	ErrSubjectChanged = errors.New("approvals: the subject has changed since approval was requested")
	// ErrNotFound is returned for an unknown request.
	ErrNotFound = errors.New("approvals: request not found")
)

// Request is a pending material change.
type Request struct {
	ID          string
	SubjectType string
	SubjectID   string
	// SubjectHash binds the approval to exact content. An approval is for a
	// specific body, not for a name: editing a draft after it is approved must
	// invalidate that approval rather than smuggle new numbers past the approver.
	SubjectHash string
	RequestedBy string
	Reason      string
	Status      string
	CreatedAt   time.Time
	DecidedAt   *time.Time
}

// Decision is the single recorded outcome.
type Decision struct {
	ID        string
	RequestID string
	DecidedBy string
	Decision  string
	Reason    string
	CreatedAt time.Time
}

// ValidSubjectType reports whether the subject type is one that needs approval.
func ValidSubjectType(subjectType string) bool {
	switch subjectType {
	case SubjectPolicyVersion, SubjectRollback, SubjectRollout,
		SubjectMarketUnfreze, SubjectIndexBatch, SubjectIndexOverride:
		return true
	}
	return false
}

// ValidateRequest checks a request is complete enough to be reviewable.
func ValidateRequest(subjectType, subjectID, subjectHash, requestedBy, reason string) error {
	var problems []string
	if !ValidSubjectType(subjectType) {
		problems = append(problems, fmt.Sprintf("subject_type %q is not an approvable change", subjectType))
	}
	if strings.TrimSpace(subjectID) == "" {
		problems = append(problems, "subject_id is required")
	}
	if !isHash(subjectHash) {
		problems = append(problems, "subject_hash must be a sha256 hex digest")
	}
	if strings.TrimSpace(requestedBy) == "" {
		problems = append(problems, "requested_by is required")
	}
	if strings.TrimSpace(reason) == "" {
		problems = append(problems, "a reason is required: an approver cannot review a change with no stated purpose")
	}
	if len(problems) > 0 {
		return fmt.Errorf("approvals: %s", strings.Join(problems, "; "))
	}
	return nil
}

func isHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		switch {
		case char >= '0' && char <= '9', char >= 'a' && char <= 'f':
		default:
			return false
		}
	}
	return true
}
