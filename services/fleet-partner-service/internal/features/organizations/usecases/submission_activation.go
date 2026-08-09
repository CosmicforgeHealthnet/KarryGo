package usecases

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/enforcement"
	eligibilitymodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	orgrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// SubmitInput submits the organization for verification.
type SubmitInput struct {
	Tenant          tenant.Context
	ExpectedVersion int64
}

// Submit records an immutable submission snapshot and dispatches it to
// verification-compliance-service.
//
// Ordering matters: the organization moves to BUSINESS_SUBMITTED only after
// the submission is durably recorded, and to UNDER_REVIEW only after the
// external service confirms acceptance (or a consumed verification event does).
// A timeout leaves UNKNOWN_OUTCOME for the reconciliation worker to resolve by
// read-back — never an assumed success.
func (s *Service) Submit(ctx context.Context, input SubmitInput) (*models.VerificationRequest, error) {
	organization, err := s.opts.Organizations.GetByID(ctx, input.Tenant.OrganizationID)
	if err != nil {
		return nil, err
	}
	switch organization.Status {
	case models.OrganizationStatusBusinessSubmitted, models.OrganizationStatusUnderReview:
		return nil, errorcodes.Conflict(errorcodes.OrganizationAlreadySubmitted,
			"This organization has already been submitted for verification.")
	case models.OrganizationStatusActive:
		return nil, errorcodes.Conflict(errorcodes.OrganizationAlreadySubmitted,
			"This organization is already active.")
	}
	if !AcceptsOnboardingEdits(organization.Status) {
		return nil, errorcodes.Conflict(errorcodes.OrganizationOnboardingIncomplete,
			"This organization cannot be submitted in its current state.")
	}

	stages, err := s.opts.Onboarding.ListStages(ctx, input.Tenant.OrganizationID)
	if err != nil {
		return nil, err
	}
	if missing := MissingStages(stages); len(missing) > 0 {
		return nil, errorcodes.UnprocessableEntity(errorcodes.OrganizationOnboardingIncomplete,
			"Complete every required onboarding step before submitting.")
	}

	documents, err := s.opts.Onboarding.ListDocuments(ctx, input.Tenant.OrganizationID)
	if err != nil {
		return nil, err
	}
	readyFiles := make([]string, 0, len(documents))
	for _, document := range documents {
		if document.Status == models.DocumentStatusReady {
			readyFiles = append(readyFiles, document.FileID)
		}
	}
	if len(readyFiles) == 0 {
		return nil, errorcodes.UnprocessableEntity(errorcodes.OrganizationOnboardingIncomplete,
			"Attach the required business documents before submitting.")
	}

	capabilities, err := s.opts.Onboarding.ListCapabilities(ctx, input.Tenant.OrganizationID)
	if err != nil {
		return nil, err
	}

	var request *models.VerificationRequest
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		version, err := s.opts.Onboarding.NextSubmissionVersion(ctx, tx, input.Tenant.OrganizationID)
		if err != nil {
			return err
		}
		request = &models.VerificationRequest{
			SubmissionVersion: version,
			SubmissionSnapshot: map[string]any{
				"legalName":          organization.LegalName,
				"registrationNumber": organization.RegistrationNumber,
				"countryCode":        organization.CountryCode,
				"timezone":           organization.Timezone,
				"defaultCurrency":    organization.DefaultCurrency,
				"capabilities":       capabilities,
				"documentFileIds":    readyFiles,
				"submittedAt":        s.opts.Now().UTC().Format(time.RFC3339),
			},
			SubmittedBy:   input.Tenant.ActorID,
			RequestID:     input.Tenant.RequestID,
			CorrelationID: input.Tenant.CorrelationID,
		}
		if err := s.opts.Onboarding.CreateVerificationRequest(ctx, tx, input.Tenant.OrganizationID, request); err != nil {
			return err
		}
		if err := s.opts.Onboarding.UpsertStage(ctx, tx, input.Tenant.OrganizationID,
			models.StageVerification, models.StageStatusInProgress, nil, nil); err != nil {
			return err
		}

		// Durable record exists; now move the lifecycle forward.
		if err := s.opts.Organizations.UpdateStatus(ctx, tx, input.Tenant.OrganizationID, organization.Version, models.StatusChange{
			PreviousStatus: organization.Status,
			NewStatus:      models.OrganizationStatusBusinessSubmitted,
			ActorType:      string(input.Tenant.ActorType),
			ActorID:        input.Tenant.ActorID,
			ReasonCode:     "submitted_for_verification",
			RequestID:      input.Tenant.RequestID,
			CorrelationID:  input.Tenant.CorrelationID,
		}); err != nil {
			return err
		}

		entry := audit.EntryFromTenant(input.Tenant, "organization.submitted", "organization", input.Tenant.OrganizationID)
		entry.NewValues = map[string]any{"submissionVersion": version}
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "organization.business_submitted.v1", "organization",
			input.Tenant.OrganizationID, organization.Version+1, input.Tenant.OrganizationID,
			input.Tenant.CorrelationID, map[string]any{
				"organizationId":    input.Tenant.OrganizationID,
				"submissionVersion": version,
			})
	})
	if err != nil {
		return nil, err
	}
	s.opts.Metrics.IncCounter("fleet_partner_organization_submission_total", map[string]string{"outcome": "recorded"}, 1)

	// Dispatch outside the transaction: an external call must never hold a
	// database lock. Failure leaves the submission recorded for the worker.
	s.dispatchVerification(ctx, input.Tenant, request, readyFiles)
	return request, nil
}

// dispatchVerification calls the authoritative verification service and
// projects the outcome. Unknown outcomes are marked for reconciliation.
func (s *Service) dispatchVerification(ctx context.Context, tenantCtx tenant.Context, request *models.VerificationRequest, fileIDs []string) {
	outcome, err := s.opts.Contracts.Verification.SubmitOrganization(ctx, platformclients.CallMeta{
		RequestID: tenantCtx.RequestID, CorrelationID: tenantCtx.CorrelationID,
	}, platformclients.VerificationSubmission{
		OrganizationID:  tenantCtx.OrganizationID,
		SubmissionID:    request.ID,
		Snapshot:        request.SubmissionSnapshot,
		DocumentFileIDs: fileIDs,
		IdempotencyKey:  "verification:" + request.ID,
	})
	if err != nil {
		status := models.VerificationStatusUnknownOutcome
		if clientErr, ok := err.(*platformclients.Error); ok && clientErr.Outcome != platformclients.OutcomeUnknown {
			// A classified failure (e.g. unavailable) is retryable and does
			// not leave the outcome genuinely unknown.
			status = models.VerificationStatusPendingDispatch
		}
		_ = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
			return s.opts.Onboarding.UpdateVerificationStatus(ctx, tx, tenantCtx.OrganizationID,
				request.ID, status, "", request.SourceVersion, "")
		})
		s.opts.Metrics.IncCounter("fleet_partner_verification_reconciliation_total",
			map[string]string{"reason": "dispatch_failed"}, 1)
		return
	}

	_ = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Onboarding.UpdateVerificationStatus(ctx, tx, tenantCtx.OrganizationID, request.ID,
			models.VerificationStatusSubmitted, outcome.ExternalReference, outcome.SourceVersion, ""); err != nil {
			return err
		}
		// Confirmed acceptance moves the organization to UNDER_REVIEW.
		organization, err := s.opts.Organizations.GetByID(ctx, tenantCtx.OrganizationID)
		if err != nil {
			return err
		}
		if organization.Status != models.OrganizationStatusBusinessSubmitted {
			return nil
		}
		return s.opts.Organizations.UpdateStatus(ctx, tx, tenantCtx.OrganizationID, organization.Version, models.StatusChange{
			PreviousStatus: organization.Status,
			NewStatus:      models.OrganizationStatusUnderReview,
			ActorType:      string(tenant.ActorTypeSystem),
			ActorID:        "verification-dispatch",
			ReasonCode:     "verification_accepted",
			RequestID:      tenantCtx.RequestID,
			CorrelationID:  tenantCtx.CorrelationID,
		})
	})
}

// RequestWalletProvisioning starts or retries organization wallet creation.
func (s *Service) RequestWalletProvisioning(ctx context.Context, tenantCtx tenant.Context) (*models.WalletProvisioning, error) {
	organization, err := s.opts.Organizations.GetByID(ctx, tenantCtx.OrganizationID)
	if err != nil {
		return nil, err
	}
	idempotencyKey := "org-wallet:" + tenantCtx.OrganizationID

	if err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		return s.opts.Onboarding.EnsureWalletProvisioning(ctx, tx, tenantCtx.OrganizationID, idempotencyKey)
	}); err != nil {
		return nil, err
	}

	account, err := s.opts.Contracts.Wallet.EnsureOrganizationWallet(ctx, platformclients.CallMeta{
		OrganizationID: tenantCtx.OrganizationID, RequestID: tenantCtx.RequestID, CorrelationID: tenantCtx.CorrelationID,
	}, platformclients.WalletProvisionRequest{
		OrganizationID: tenantCtx.OrganizationID,
		Currency:       organization.DefaultCurrency,
		IdempotencyKey: idempotencyKey,
	})

	next := s.opts.Now().Add(2 * time.Minute)
	if err != nil {
		status := models.WalletStatusFailedRetryable
		if clientErr, ok := err.(*platformclients.Error); ok && clientErr.Outcome == platformclients.OutcomeUnknown {
			// Unknown outcome: a wallet may or may not have been created, so
			// the reconciler must read back rather than blindly retry.
			status = models.WalletStatusReconciliationRequired
		}
		_ = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
			return s.opts.Onboarding.SetWalletStatus(ctx, tx, tenantCtx.OrganizationID, status, "", "wallet_unavailable", &next)
		})
		s.opts.Metrics.IncCounter("fleet_partner_wallet_provisioning_total", map[string]string{"outcome": status}, 1)
		return s.opts.Onboarding.GetWalletProvisioning(ctx, tenantCtx.OrganizationID)
	}

	// The account ID is stored only from a confirmed response.
	status := models.WalletStatusProvisioning
	if account.Status == "ACTIVE" {
		status = models.WalletStatusActive
	}
	if err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Onboarding.SetWalletStatus(ctx, tx, tenantCtx.OrganizationID, status, account.AccountID, "", nil); err != nil {
			return err
		}
		stageStatus := models.StageStatusInProgress
		if status == models.WalletStatusActive {
			stageStatus = models.StageStatusComplete
		}
		return s.opts.Onboarding.UpsertStage(ctx, tx, tenantCtx.OrganizationID, models.StageWalletProvisioning, stageStatus, nil, nil)
	}); err != nil {
		return nil, err
	}
	s.opts.Metrics.IncCounter("fleet_partner_wallet_provisioning_total", map[string]string{"outcome": status}, 1)
	return s.opts.Onboarding.GetWalletProvisioning(ctx, tenantCtx.OrganizationID)
}

// ActivationGate is one activation precondition.
type ActivationGate struct {
	Name   string
	Passed bool
	Detail string
}

// ActivationAssessment is the result of evaluating every gate.
type ActivationAssessment struct {
	OrganizationID string
	Gates          []ActivationGate
	Ready          bool
}

// EvaluateActivation checks every activation gate.
//
// Activation is never a direct owner-callable action: this evaluator runs
// from events and the activation worker, and only it can move an
// organization to ACTIVE.
func (s *Service) EvaluateActivation(ctx context.Context, organizationID string) (*ActivationAssessment, error) {
	assessment := &ActivationAssessment{OrganizationID: organizationID}

	organization, err := s.opts.Organizations.GetByID(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	assessment.Gates = append(assessment.Gates, ActivationGate{
		Name:   "organization_under_review",
		Passed: organization.Status == models.OrganizationStatusUnderReview,
		Detail: organization.Status,
	})

	stages, err := s.opts.Onboarding.ListStages(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	assessment.Gates = append(assessment.Gates, ActivationGate{
		Name:   "onboarding_stages_complete",
		Passed: allComplete(stages, requiredForSubmission),
	})

	// Verification must be approved by the authoritative service; it is never
	// marked approved locally.
	verificationPassed := false
	if verification, err := s.opts.Onboarding.LatestVerificationRequest(ctx, organizationID); err == nil {
		verificationPassed = verification.Status == models.VerificationStatusApproved
	} else if !errors.Is(err, orgrepos.ErrNotFound) {
		return nil, err
	}
	assessment.Gates = append(assessment.Gates, ActivationGate{Name: "verification_approved", Passed: verificationPassed})

	// The organization wallet must be confirmed active by the wallet service.
	walletPassed := false
	if wallet, err := s.opts.Onboarding.GetWalletProvisioning(ctx, organizationID); err == nil {
		walletPassed = wallet.Status == models.WalletStatusActive && wallet.WalletAccountID != nil
	} else if !errors.Is(err, orgrepos.ErrNotFound) {
		return nil, err
	}
	assessment.Gates = append(assessment.Gates, ActivationGate{Name: "organization_wallet_active", Passed: walletPassed})

	branches, err := s.opts.Branches.List(ctx, tenant.Query{OrganizationID: organizationID, Limit: 100})
	if err != nil {
		return nil, err
	}
	activeBranch := false
	for _, branch := range branches {
		if branch.Status == models.BranchStatusActive {
			activeBranch = true
			break
		}
	}
	assessment.Gates = append(assessment.Gates, ActivationGate{Name: "active_default_branch", Passed: activeBranch})

	settings, err := s.opts.Settings.Get(ctx, organizationID)
	settingsValid := err == nil && settings != nil && settings.AssignmentTimeoutSeconds >= 30
	assessment.Gates = append(assessment.Gates, ActivationGate{Name: "settings_valid", Passed: settingsValid})

	assessment.Ready = true
	for _, gate := range assessment.Gates {
		if !gate.Passed {
			assessment.Ready = false
			break
		}
	}
	if !assessment.Ready {
		s.opts.Metrics.IncCounter("fleet_partner_organization_activation_blocked_total", nil, 1)
	}
	return assessment, nil
}

// TryActivate activates the organization when every gate passes. It is
// idempotent: an already-active organization is a no-op success.
func (s *Service) TryActivate(ctx context.Context, organizationID string, correlationID string) (bool, error) {
	assessment, err := s.EvaluateActivation(ctx, organizationID)
	if err != nil {
		return false, err
	}
	if !assessment.Ready {
		return false, nil
	}

	// The local assessment says the onboarding stages are done. The durable
	// gate says whether the platform currently vouches for this organization —
	// verification approved and fresh, no restriction, required documents in
	// order. Both must hold, and the gate's answer is the one that is recorded.
	gateSubject := enforcement.Company(organizationID, eligibilitymodels.OperationCompanyActivation)
	gateSubject.ActorType = eligibilitymodels.ActorSystem
	gateSubject.ActorID = "activation-evaluator"
	gateSubject.CorrelationID = correlationID
	if _, gateErr := enforcement.Require(ctx, s.opts.Eligibility, gateSubject); gateErr != nil {
		// Not an error to the caller: activation is evaluated repeatedly by a
		// worker, and "not yet eligible" is the ordinary answer. It is
		// reported as "not activated" so the sweep retries rather than
		// dead-lettering.
		return false, nil
	}

	err = s.Transition(ctx, TransitionInput{
		Tenant: tenant.Context{
			OrganizationID: organizationID,
			ActorType:      tenant.ActorTypeSystem,
			ActorID:        "activation-evaluator",
			CorrelationID:  correlationID,
		},
		OrganizationID: organizationID,
		TargetStatus:   models.OrganizationStatusActive,
		ReasonCode:     "all_activation_gates_passed",
		EventType:      "organization.activated.v1",
	})
	if err != nil {
		return false, err
	}

	_ = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		return s.opts.Onboarding.UpsertStage(ctx, tx, organizationID, models.StageActivation, models.StageStatusComplete, nil, nil)
	})
	return true, nil
}
