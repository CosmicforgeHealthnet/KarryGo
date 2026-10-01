package financeops

// Section 12/13: handing one Fleet settlement instruction to the money
// authority.
//
// Fleet decides the split and never posts a journal (Invariant 4). What this
// file adds is the delivery of that decision, reusing the same posting
// machinery payroll already uses -- idempotency key plus request fingerprint,
// recorded attempts, scheduled retry, and an ambiguous outcome resolved by
// reading back rather than by assuming.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// ManagedSettlementSubmission is one instruction on its way to Wallet.
//
// Every field is a figure Fleet already resolved and stored. Nothing is
// recomputed at submission time: a number that could change between resolution
// and delivery would make the stored resolution a lie.
type ManagedSettlementSubmission struct {
	InstructionID      string
	InstructionVersion int64

	OrganizationID string
	MembershipID   string
	ProviderID     string
	ProviderType   string

	SourceService      string
	SourceJobID        string
	SourceAssignmentID string

	Currency               string
	GrossBasisMinor        int64
	ProviderShareMinor     int64
	OrganizationShareMinor int64

	CompensationPlanID            string
	CompensationPlanVersion       int64
	CompensationResolutionVersion int64

	CorrelationID  string
	IdempotencyKey string
}

// Validate checks the submission before it is sent.
//
// Refusing an incoherent instruction here keeps a bad figure out of the money
// authority entirely, rather than relying on it to catch everything.
func (s ManagedSettlementSubmission) Validate() error {
	switch {
	case strings.TrimSpace(s.InstructionID) == "":
		return fmt.Errorf("instructionId is required")
	case s.InstructionVersion <= 0:
		return fmt.Errorf("instructionVersion must be positive")
	case strings.TrimSpace(s.OrganizationID) == "":
		return fmt.Errorf("organizationId is required")
	case strings.TrimSpace(s.ProviderID) == "" || strings.TrimSpace(s.ProviderType) == "":
		return fmt.Errorf("provider identity is required")
	case strings.TrimSpace(s.SourceService) == "" || strings.TrimSpace(s.SourceJobID) == "":
		return fmt.Errorf("source job identity is required")
	case len(strings.TrimSpace(s.Currency)) != 3:
		return fmt.Errorf("currency must be a three letter code")
	case s.ProviderShareMinor < 0 || s.OrganizationShareMinor < 0 || s.GrossBasisMinor < 0:
		return fmt.Errorf("monetary shares cannot be negative")
	case s.ProviderShareMinor+s.OrganizationShareMinor != s.GrossBasisMinor:
		return fmt.Errorf("provider and organization shares must sum to the gross basis")
	case strings.TrimSpace(s.IdempotencyKey) == "":
		return fmt.Errorf("idempotencyKey is required")
	}
	return nil
}

// payload is the wire body the Wallet adapter sends.
func (s ManagedSettlementSubmission) payload() (json.RawMessage, error) {
	return json.Marshal(map[string]any{
		"instruction_id":                  s.InstructionID,
		"instruction_version":             s.InstructionVersion,
		"organization_id":                 s.OrganizationID,
		"membership_id":                   s.MembershipID,
		"provider_id":                     s.ProviderID,
		"provider_type":                   s.ProviderType,
		"source_service":                  s.SourceService,
		"source_job_id":                   s.SourceJobID,
		"source_assignment_id":            s.SourceAssignmentID,
		"currency":                        s.Currency,
		"gross_basis_minor":               s.GrossBasisMinor,
		"provider_share_minor":            s.ProviderShareMinor,
		"organization_share_minor":        s.OrganizationShareMinor,
		"compensation_plan_id":            s.CompensationPlanID,
		"compensation_plan_version":       s.CompensationPlanVersion,
		"compensation_resolution_version": s.CompensationResolutionVersion,
		"correlation_id":                  s.CorrelationID,
		"idempotency_key":                 s.IdempotencyKey,
	})
}

// SubmitManagedSettlement delivers one instruction to the money authority.
//
// It returns the posting operation id and whether the money authority has
// confirmed. A confirmed-false result is not a failure: the operation is
// durable, carries its own retry schedule, and the reconciliation pass resumes
// it. What never happens is a second journal, because the idempotency key is
// the instruction's and Wallet keys its posting on the same identity.
func (s *Service) SubmitManagedSettlement(
	ctx context.Context, t tenant.Context, in ManagedSettlementSubmission,
) (string, bool, error) {
	if err := in.Validate(); err != nil {
		return "", false, apperrors.BadRequest("Check the settlement instruction.", err)
	}
	if t.OrganizationID != "" && t.OrganizationID != in.OrganizationID {
		return "", false, apperrors.Forbidden("Settlement instruction belongs to another organization.", nil)
	}
	body, err := in.payload()
	if err != nil {
		return "", false, err
	}
	request := PostingRequest{
		OrganizationID: in.OrganizationID,
		RecordType:     "MANAGED_WORK_SETTLEMENT_INSTRUCTION",
		RecordID:       in.InstructionID,
		OperationType:  OperationManagedWorkSettlement,
		IdempotencyKey: in.IdempotencyKey,
		AmountMinor:    in.GrossBasisMinor,
		Currency:       in.Currency,
		Payload:        body,
	}

	operationID, confirmed, err := s.createPosting(ctx, t, request)
	if err != nil {
		return "", false, err
	}
	if confirmed {
		// Already posted under this identity. Re-sending would be harmless at
		// Wallet, but there is no reason to make the call.
		return operationID, true, nil
	}
	outcome, err := s.executePosting(ctx, t, operationID, request)
	if err != nil {
		return operationID, false, err
	}
	return operationID, postingConfirmed(outcome.Status), nil
}
