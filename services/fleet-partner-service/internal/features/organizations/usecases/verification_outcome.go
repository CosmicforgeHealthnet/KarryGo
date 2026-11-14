package usecases

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	orgrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// MapVerificationStatus translates verification-compliance-service's
// decision vocabulary into Fleet's verification request statuses (ADR-006).
//
// Verification decides PENDING, IN_PROGRESS, CHANGES_REQUIRED, VERIFIED,
// REJECTED or SUSPENDED. A SUSPENDED decision is recorded as REJECTED here:
// the request can no longer activate the company, and the organization
// lifecycle carries the suspension itself. An unrecognised value maps to ""
// and is ignored rather than guessed at.
func MapVerificationStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "VERIFIED", models.VerificationStatusApproved:
		return models.VerificationStatusApproved
	case "PENDING", "IN_PROGRESS", "PENDING_REVIEW", models.VerificationStatusUnderReview, models.VerificationStatusSubmitted:
		return models.VerificationStatusUnderReview
	case models.VerificationStatusChangesRequired, "REMEDIATION_REQUIRED":
		return models.VerificationStatusChangesRequired
	case models.VerificationStatusRejected, "SUSPENDED":
		return models.VerificationStatusRejected
	}
	return ""
}

// VerificationOutcomeInput is one authoritative decision about a submission.
type VerificationOutcomeInput struct {
	OrganizationID string
	// SubmissionID is Fleet's verification request id, which Fleet sends to
	// Verification as its submission id.
	SubmissionID      string
	Status            string
	ExternalReference string
	SourceVersion     int64
	EventID           string
	Issues            []platformclients.VerificationIssue
	CorrelationID     string
}

// VerificationOutcomeResult reports what the outcome changed.
type VerificationOutcomeResult struct {
	Status    string
	Applied   bool
	Activated bool
}

// ApplyVerificationOutcome records a verification decision, moves the
// organization lifecycle and, for an approval, provisions the wallet and
// attempts activation.
//
// It is idempotent and order-safe: an older source version is ignored, so a
// replayed or out-of-order event cannot regress a newer decision.
func (s *Service) ApplyVerificationOutcome(ctx context.Context, input VerificationOutcomeInput) (*VerificationOutcomeResult, error) {
	mapped := MapVerificationStatus(input.Status)
	if mapped == "" {
		return &VerificationOutcomeResult{Status: input.Status}, nil
	}
	result := &VerificationOutcomeResult{Status: mapped}

	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		onboarding := orgrepos.NewOnboardingRepository(tx)
		request, err := onboarding.LatestVerificationRequest(ctx, input.OrganizationID)
		if err != nil {
			return err
		}
		if input.SubmissionID != "" && request.ID != input.SubmissionID {
			// A decision about a superseded submission does not change the
			// company: only the latest submission counts.
			return nil
		}
		if input.SourceVersion < request.SourceVersion {
			return nil
		}
		if input.SourceVersion == request.SourceVersion && request.Status == mapped {
			return nil
		}
		if err := onboarding.UpdateVerificationStatus(ctx, tx, input.OrganizationID, request.ID,
			mapped, input.ExternalReference, input.SourceVersion, input.EventID); err != nil {
			return err
		}
		result.Applied = true

		if len(input.Issues) > 0 && (mapped == models.VerificationStatusChangesRequired || mapped == models.VerificationStatusRejected) {
			issues := make([]models.VerificationIssue, 0, len(input.Issues))
			for _, issue := range input.Issues {
				code := strings.TrimSpace(issue.Code)
				if code == "" {
					code = mapped
				}
				issues = append(issues, models.VerificationIssue{
					IssueCode:         code,
					FieldReference:    optionalString(issue.FieldReference),
					DocumentReference: optionalString(issue.DocumentReference),
					Remediation:       issue.Remediation,
				})
			}
			if err := onboarding.AppendVerificationIssues(ctx, tx, input.OrganizationID, request.ID, issues); err != nil {
				return err
			}
		}

		stageStatus := models.StageStatusInProgress
		switch mapped {
		case models.VerificationStatusApproved:
			stageStatus = models.StageStatusComplete
		case models.VerificationStatusChangesRequired, models.VerificationStatusRejected:
			stageStatus = models.StageStatusChangesRequired
		}
		if err := onboarding.UpsertStage(ctx, tx, input.OrganizationID, models.StageVerification, stageStatus, nil, nil); err != nil {
			return err
		}

		organization, err := orgrepos.NewOrganizationRepository(tx).GetByID(ctx, input.OrganizationID)
		if err != nil {
			return err
		}
		target := ""
		switch mapped {
		case models.VerificationStatusApproved, models.VerificationStatusUnderReview:
			if organization.Status == models.OrganizationStatusBusinessSubmitted {
				target = models.OrganizationStatusUnderReview
			}
			// FLEET-8.13: a later VERIFIED decision lifts a suspension that
			// verification itself imposed. A suspension imposed by anyone else
			// (an operator, closure review) is not verification's to undo.
			if mapped == models.VerificationStatusApproved && organization.Status == models.OrganizationStatusSuspended {
				var actor string
				err := tx.QueryRow(ctx, `SELECT actor_id FROM fleet_partner_organization_status_history
					WHERE organization_id = $1 AND new_status = $2 ORDER BY occurred_at DESC, id DESC LIMIT 1`,
					input.OrganizationID, models.OrganizationStatusSuspended).Scan(&actor)
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return err
				}
				if actor == "verification-outcome" {
					target = models.OrganizationStatusActive
				}
			}
		case models.VerificationStatusChangesRequired, models.VerificationStatusRejected:
			switch organization.Status {
			case models.OrganizationStatusBusinessSubmitted, models.OrganizationStatusUnderReview:
				target = models.OrganizationStatusChangesRequired
			case models.OrganizationStatusActive:
				if strings.EqualFold(strings.TrimSpace(input.Status), "SUSPENDED") {
					target = models.OrganizationStatusSuspended
				}
			}
		}
		if target == "" {
			return nil
		}
		return orgrepos.NewOrganizationRepository(tx).UpdateStatus(ctx, tx, input.OrganizationID, organization.Version, models.StatusChange{
			PreviousStatus: organization.Status,
			NewStatus:      target,
			ActorType:      string(tenant.ActorTypeSystem),
			ActorID:        "verification-outcome",
			ReasonCode:     "verification_" + strings.ToLower(mapped),
			CorrelationID:  input.CorrelationID,
		})
	})
	if err != nil {
		return nil, err
	}
	s.opts.Metrics.IncCounter("fleet_partner_verification_outcome_total", map[string]string{"status": mapped}, 1)

	if mapped == models.VerificationStatusApproved {
		if !s.opts.WalletGateWaived {
			s.ensureWalletRequested(ctx, input.OrganizationID, input.CorrelationID)
		}
		activated, err := s.TryActivate(ctx, input.OrganizationID, input.CorrelationID)
		if err != nil {
			return result, err
		}
		result.Activated = activated
	}
	return result, nil
}

// ensureWalletRequested starts wallet provisioning for an approved company
// that has none yet. A failure is recorded on the provisioning row and retried
// by the reconciler; it never blocks recording the approval.
func (s *Service) ensureWalletRequested(ctx context.Context, organizationID string, correlationID string) {
	wallet, err := s.opts.Onboarding.GetWalletProvisioning(ctx, organizationID)
	if err == nil && wallet.Status == models.WalletStatusActive {
		return
	}
	if err != nil && !errors.Is(err, orgrepos.ErrNotFound) {
		return
	}
	if err == nil && wallet.WalletAccountID != nil && *wallet.WalletAccountID != "" {
		// The wallet service already confirmed an account for this company:
		// read its state back instead of sending another provisioning request.
		s.readBackWallet(ctx, organizationID, *wallet.WalletAccountID, correlationID)
		return
	}
	_, _ = s.RequestWalletProvisioning(ctx, tenant.Context{
		OrganizationID: organizationID,
		ActorType:      tenant.ActorTypeSystem,
		ActorID:        "verification-outcome",
		CorrelationID:  correlationID,
	})
}

// ReconcileVerificationsResult counts one reconciliation pass.
type ReconcileVerificationsResult struct {
	Redispatched int
	ReadBack     int
	Activated    int
	Failed       int
}

// ReconcileVerifications repairs organization verification without relying on
// an event being delivered:
//
//   - a submission that never reached Verification (PENDING_DISPATCH or
//     UNKNOWN_OUTCOME) is dispatched again with the same idempotency key, so a
//     retry can never create a second review;
//   - a submission under review has its decision read back;
//   - an approved company that is not yet active is offered activation again
//     (for example once its wallet becomes active).
func (s *Service) ReconcileVerifications(ctx context.Context, limit int) (ReconcileVerificationsResult, error) {
	var out ReconcileVerificationsResult
	if limit <= 0 {
		limit = 25
	}
	pending, err := s.opts.Onboarding.ListVerificationRequestsToReconcile(ctx, limit)
	if err != nil {
		return out, err
	}
	for _, request := range pending {
		correlationID := "verification-reconcile:" + request.ID
		switch request.Status {
		case models.VerificationStatusPendingDispatch, models.VerificationStatusUnknownOutcome:
			if !s.opts.Contracts.Verification.Configured() {
				continue
			}
			tenantCtx := tenant.Context{
				OrganizationID: request.OrganizationID,
				ActorType:      tenant.ActorTypeSystem,
				ActorID:        "verification-reconciler",
				CorrelationID:  correlationID,
			}
			s.dispatchVerification(ctx, tenantCtx, &request, snapshotFileIDs(request.SubmissionSnapshot))
			out.Redispatched++
		case models.VerificationStatusSubmitted, models.VerificationStatusUnderReview:
			if !s.opts.Contracts.Verification.Configured() {
				continue
			}
			outcome, err := s.opts.Contracts.Verification.ReadOrganizationStatus(ctx, platformclients.CallMeta{
				OrganizationID: request.OrganizationID, CorrelationID: correlationID,
			}, request.OrganizationID, request.ID)
			if err != nil {
				out.Failed++
				continue
			}
			out.ReadBack++
			applied, err := s.ApplyVerificationOutcome(ctx, VerificationOutcomeInput{
				OrganizationID:    request.OrganizationID,
				SubmissionID:      request.ID,
				Status:            outcome.Status,
				ExternalReference: outcome.ExternalReference,
				SourceVersion:     outcome.SourceVersion,
				Issues:            outcome.Issues,
				CorrelationID:     correlationID,
			})
			if err != nil {
				out.Failed++
				continue
			}
			if applied.Activated {
				out.Activated++
			}
		case models.VerificationStatusApproved:
			if !s.opts.WalletGateWaived {
				s.ensureWalletRequested(ctx, request.OrganizationID, correlationID)
			}
			activated, err := s.TryActivate(ctx, request.OrganizationID, correlationID)
			if err != nil {
				out.Failed++
				continue
			}
			if activated {
				out.Activated++
			}
		}
	}
	return out, nil
}

// snapshotFileIDs recovers the document file ids recorded in the submission
// snapshot, so a re-dispatch sends exactly what was submitted.
func snapshotFileIDs(snapshot map[string]any) []string {
	raw, ok := snapshot["documentFileIds"].([]any)
	if !ok {
		return nil
	}
	ids := make([]string, 0, len(raw))
	for _, value := range raw {
		if id, ok := value.(string); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
