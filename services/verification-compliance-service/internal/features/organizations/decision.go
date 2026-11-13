package organizations

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Organization decision statuses. CHANGES_REQUIRED asks the company to
// correct its submission (with issues); IN_PROGRESS marks review started.
const (
	StatusPending         = "PENDING"
	StatusInProgress      = "IN_PROGRESS"
	StatusChangesRequired = "CHANGES_REQUIRED"
	StatusVerified        = "VERIFIED"
	StatusRejected        = "REJECTED"
	StatusSuspended       = "SUSPENDED"

	// StatusChangedEventType is delivered to fleet-partner-service.
	StatusChangedEventType = "organization.verification.status_changed.v1"
	// SubmittedEventType records an accepted submission.
	SubmittedEventType = "organization.verification.submitted.v1"
)

var decisionStatuses = map[string]bool{StatusPending: true, StatusInProgress: true, StatusChangesRequired: true,
	StatusVerified: true, StatusRejected: true, StatusSuspended: true}
var restrictionStatuses = map[string]bool{"NONE": true, "RESTRICTED": true, "SUSPENDED": true}

// Decision errors.
var (
	ErrInvalidDecision    = errors.New("organizations: invalid decision")
	ErrSubmissionNotFound = errors.New("organizations: submission not found or version changed")
)

// DecisionInput is one authorized decision about an organization submission.
type DecisionInput struct {
	OrganizationID    string
	SubmissionID      string
	Status            string
	RestrictionStatus string
	ExpectedVersion   int64
	Issues            []map[string]any
	// DecidedBy names the human or service that decided.
	DecidedBy string
}

// ApplyDecision fences on the expected version, records the decision and
// queues its delivery to Fleet, all in the caller's transaction.
func ApplyDecision(ctx context.Context, tx pgx.Tx, in DecisionInput) (view, error) {
	var result view
	in.Status = strings.ToUpper(strings.TrimSpace(in.Status))
	in.RestrictionStatus = strings.ToUpper(strings.TrimSpace(in.RestrictionStatus))
	if in.RestrictionStatus == "" {
		in.RestrictionStatus = "NONE"
	}
	if !decisionStatuses[in.Status] || !restrictionStatuses[in.RestrictionStatus] || in.ExpectedVersion < 1 {
		return result, ErrInvalidDecision
	}
	if in.Status == StatusChangesRequired && len(in.Issues) == 0 {
		// A company cannot correct what it is not told.
		return result, ErrInvalidDecision
	}
	if in.Issues == nil {
		in.Issues = []map[string]any{}
	}
	err := tx.QueryRow(ctx, `UPDATE organization_verification_submissions
		SET status=$3, restriction_status=$4, issues=$5, source_version=source_version+1, decided_at=now(), updated_at=now()
		WHERE organization_id=$1 AND submission_id=$2 AND source_version=$6
		RETURNING id::text, status, restriction_status, source_version, issues`,
		in.OrganizationID, in.SubmissionID, in.Status, in.RestrictionStatus, in.Issues, in.ExpectedVersion).
		Scan(&result.ExternalReference, &result.Status, &result.RestrictionStatus, &result.SourceVersion, &result.Issues)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrSubmissionNotFound
	}
	if err != nil {
		return result, err
	}
	if err := queueOrganizationEvent(ctx, tx, StatusChangedEventType, in.OrganizationID, in.SubmissionID, result, in.DecidedBy); err != nil {
		return result, err
	}
	return result, nil
}

// queueOrganizationEvent writes the outbox row the Fleet delivery worker
// sends. The payload carries the organization and submission ids so delivery
// never has to reconstruct them.
func queueOrganizationEvent(ctx context.Context, tx pgx.Tx, eventType string, organizationID string, submissionID string, v view, decidedBy string) error {
	payload, err := json.Marshal(map[string]any{
		"organizationId":    organizationID,
		"submissionId":      submissionID,
		"externalReference": v.ExternalReference,
		"status":            v.Status,
		"restrictionStatus": v.RestrictionStatus,
		"sourceVersion":     v.SourceVersion,
		"issues":            v.Issues,
		"decidedBy":         decidedBy,
	})
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO verification_outbox(aggregate_id,event_type,event_version,payload)
		VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, organizationID, eventType, v.SourceVersion, payload)
	return err
}
