package usecases

import (
	"context"
	"errors"
	"strings"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/providerreceipt"
	"github.com/jackc/pgx/v5"
)

func (s *Service) matchRecordedDecision(ctx context.Context, response *models.Response, decision, reason, note string, evidence []string) error {
	files, err := s.opts.Assignments.ResponseEvidence(ctx, response)
	if err != nil {
		return err
	}
	stored := providerreceipt.Intent{Decision: response.Response, Evidence: files}
	if response.ReasonCode != nil {
		stored.ReasonCode = *response.ReasonCode
	}
	if response.ReasonText != nil {
		stored.Note = *response.ReasonText
	}
	expected := providerreceipt.Intent{Decision: decision, ReasonCode: reason, Note: note, Evidence: evidence}
	if stored.Digest() != expected.Digest() {
		return errorcodes.Conflict(errorcodes.IdempotencyConflictCode, "This response identifier was already used for a different decision.")
	}
	return nil
}

// ProviderDecisionReceipt reads the original committed decision under the
// assignment's canonical provider/membership/organization binding. Cancellation,
// expiry or completion after that commit does not erase the historical receipt.
// Current execution permission remains exclusively VerifyProviderAcceptance.
func (s *Service) ProviderDecisionReceipt(ctx context.Context, assignmentID, providerID, sourceService, key string) (*providerreceipt.Receipt, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if strings.TrimSpace(key) == "" || len(key) > 128 || len(assignmentID) > 128 || providerID == "" || len(providerID) > 128 || (sourceService != models.SourceDispatch && sourceService != models.SourceTaxi && sourceService != models.SourceHauling) {
		return nil, apperrors.BadRequest("Decision identity is invalid.", nil)
	}
	// One statement gives a consistent snapshot of binding, decision and evidence.
	var receipt providerreceipt.Receipt
	var reason, note string
	var evidence []string
	var err error
	if strings.HasPrefix(assignmentID, "pas_") {
		err = s.opts.Pool.QueryRow(ctx, `SELECT d.id,a.id,a.provider_id,d.idempotency_key,
		 CASE d.decision WHEN 'ACCEPT' THEN 'ACCEPTED' WHEN 'DECLINE' THEN 'DECLINED' END,
		 d.reason_code,d.safe_note,d.decided_at
		 FROM fleet_partner_private_assignments a JOIN fleet_partner_private_assignment_decisions d
		 ON d.assignment_id=a.id AND d.organization_id=a.organization_id AND d.provider_id=a.provider_id
		 WHERE a.id=$1 AND a.provider_id=$2 AND a.execution_service=$3 AND d.idempotency_key=$4`, assignmentID, providerID, sourceService, key).Scan(&receipt.ReceiptID, &receipt.AssignmentID, &receipt.ProviderID, &receipt.ClientResponseID, &receipt.Decision, &reason, &note, &receipt.RecordedAt)
	} else {
		err = s.opts.Pool.QueryRow(ctx, `SELECT r.id,a.id,a.executing_provider_id,r.client_response_id,r.response,
	 COALESCE(r.reason_code,''),COALESCE(r.reason_text,''),r.occurred_at,
	 ARRAY(SELECT e.file_id FROM fleet_partner_assignment_response_evidence e
	 WHERE e.response_id=r.id AND e.organization_id=r.organization_id ORDER BY e.file_id)
	 FROM fleet_partner_provider_assignments a JOIN fleet_partner_assignment_responses r
	 ON r.assignment_id=a.id AND r.organization_id=a.organization_id AND r.membership_id=a.membership_id
	 WHERE a.id=$1 AND a.executing_provider_id=$2 AND a.source_service=$3
	 AND r.client_response_id=$4 AND r.response IN ('ACCEPTED','DECLINED')`, assignmentID, providerID, sourceService, key).
			Scan(&receipt.ReceiptID, &receipt.AssignmentID, &receipt.ProviderID, &receipt.ClientResponseID, &receipt.Decision, &reason, &note, &receipt.RecordedAt, &evidence)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.NotFound("Decision receipt not found.", nil)
	}
	if err != nil {
		return nil, err
	}
	receipt.ContractVersion = providerreceipt.Version
	receipt.IntentDigest = (providerreceipt.Intent{AssignmentID: receipt.AssignmentID, ProviderID: receipt.ProviderID, ClientResponseID: receipt.ClientResponseID, Decision: receipt.Decision, ReasonCode: reason, Note: note, Evidence: evidence}).Digest()
	return &receipt, nil
}
