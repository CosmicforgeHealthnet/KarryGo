// Package operations owns bridge orchestration metadata, never domain state.
package operations

import (
	"cosmicforge/logistics/services/admin-backoffice-service/internal/security"
	"errors"
	"github.com/google/uuid"
	"time"
)

var ErrConflict = errors.New("operation identity conflict")
var ErrNotFound = errors.New("operation not found")

type Target struct {
	Type            string `json:"type"`
	ID              string `json:"id"`
	ExpectedVersion int64  `json:"expected_version"`
}
type Command struct {
	ContractVersion   string         `json:"contract_version"`
	CommandID         string         `json:"command_id"`
	Product           string         `json:"product"`
	Action            string         `json:"action"`
	Target            Target         `json:"target"`
	Scope             security.Scope `json:"scope"`
	ReasonCode        string         `json:"reason_code"`
	Reason            string         `json:"reason"`
	CaseReference     string         `json:"case_reference"`
	IdempotencyKey    string         `json:"idempotency_key"`
	ApprovalReference string         `json:"approval_reference,omitempty"`
	RequestedAt       time.Time      `json:"requested_at"`
}

func (c Command) Validate(now time.Time) error {
	id, err := uuid.Parse(c.CommandID)
	if err != nil || id.String() != c.CommandID || c.ContractVersion != "1.0" || c.Product != "karryam" || !security.Opaque(c.Action) || !security.Opaque(c.Target.Type) || !security.Opaque(c.Target.ID) || c.Target.ExpectedVersion < 1 || !security.Opaque(c.Scope.Market) || !security.Opaque(c.Scope.Organization) || !security.Opaque(c.ReasonCode) || len(c.Reason) < 1 || len(c.Reason) > 512 || !security.Opaque(c.CaseReference) || !security.Opaque(c.IdempotencyKey) || (c.ApprovalReference != "" && !security.Opaque(c.ApprovalReference)) || c.RequestedAt.Before(now.Add(-5*time.Minute)) || c.RequestedAt.After(now) {
		return errors.New("invalid command envelope")
	}
	return nil
}

type Operation struct {
	ID            string         `json:"operation_id"`
	Action        string         `json:"action"`
	Target        Target         `json:"target"`
	Scope         security.Scope `json:"scope"`
	SourceVersion *int64         `json:"source_version"`
	Status        string         `json:"status"`
	CreatedAt     time.Time      `json:"created_at"`
	AsOf          time.Time      `json:"as_of"`
}

// CanTransition deliberately excludes retrying EXECUTING/UNKNOWN_OUTCOME.
// Only authoritative owner read-back can resolve an uncertain result.
func CanTransition(from, to string, authoritativeReadback bool) bool {
	switch from {
	case "PENDING":
		return to == "EXECUTING" || to == "REJECTED" || to == "CANCELLED_WHEN_SAFE"
	case "EXECUTING":
		return to == "CONFIRMED" || to == "REJECTED" || to == "FAILED_RETRYABLE" || to == "FAILED_TERMINAL" || to == "UNKNOWN_OUTCOME"
	case "UNKNOWN_OUTCOME":
		return authoritativeReadback && (to == "CONFIRMED" || to == "REJECTED" || to == "FAILED_TERMINAL")
	}
	return false
}
