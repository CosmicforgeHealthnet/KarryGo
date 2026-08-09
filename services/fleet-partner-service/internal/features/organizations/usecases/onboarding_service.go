package usecases

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/enforcement"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
	orgrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/observability"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// EventPublisher appends domain events to the outbox in the caller's tx.
type EventPublisher interface {
	Publish(ctx context.Context, tx pgx.Tx, eventType string, aggregateType string, aggregateID string, aggregateVersion int64, organizationID string, correlationID string, payload any) error
}

// MembershipCreator creates the owner membership when a draft is created.
type MembershipCreator interface {
	CreateOwnerMembership(ctx context.Context, tx pgx.Tx, organizationID string, identityID string, fullName string) (membershipID string, err error)
}

// Options configure the onboarding service.
type Options struct {
	Pool          *pgxpool.Pool
	Organizations *orgrepos.OrganizationRepository
	Onboarding    *orgrepos.OnboardingRepository
	Branches      *orgrepos.BranchRepository
	Settings      *orgrepos.SettingsRepository
	Memberships   MembershipCreator
	Contracts     platformclients.ContractSet
	// Eligibility gates activation. Activation is the moment an organization
	// becomes able to take real work, so it is gated on a durable decision
	// rather than only on the local assessment.
	Eligibility enforcement.Gate
	Events      EventPublisher
	Audit       *audit.Recorder
	Metrics     *observability.Metrics
	Now         func() time.Time
}

// Service implements organization onboarding and lifecycle.
type Service struct {
	opts Options
}

// NewService builds the service.
func NewService(opts Options) *Service {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Metrics == nil {
		opts.Metrics = observability.NewMetrics()
	}
	if opts.Audit == nil {
		opts.Audit = audit.NewRecorder()
	}
	return &Service{opts: opts}
}

// OrganizationStatus returns an organization's name and status. It backs the
// staff feature's OrganizationReader port.
func (s *Service) OrganizationStatus(ctx context.Context, organizationID string) (string, string, error) {
	organization, err := s.opts.Organizations.GetByID(ctx, organizationID)
	if err != nil {
		return "", "", err
	}
	name := organization.TradingName
	if name == "" {
		name = organization.LegalName
	}
	return name, organization.Status, nil
}

func (s *Service) OrganizationCurrency(ctx context.Context, organizationID string) (string, error) {
	organization, err := s.opts.Organizations.GetByID(ctx, organizationID)
	if err != nil {
		return "", err
	}
	return organization.DefaultCurrency, nil
}

// BranchName returns a branch's safe display name and type. It backs the
// providers feature's OrganizationReader port (Section 30's membership
// summary), the same narrow "safe projection only" shape as
// OrganizationStatus above -- never the branch's address or coordinates.
func (s *Service) BranchName(ctx context.Context, organizationID string, branchID string) (string, string, error) {
	branch, err := s.opts.Branches.GetByID(ctx, organizationID, branchID)
	if err != nil {
		return "", "", err
	}
	return branch.Name, branch.Type, nil
}

// CreateDraftInput creates an organization draft.
type CreateDraftInput struct {
	IdentityID         string
	OwnerName          string
	LegalName          string
	TradingName        string
	RegistrationNumber string
	CountryCode        string
	OrganizationType   string
	Timezone           string
	DefaultCurrency    string
	RequestID          string
	CorrelationID      string
}

// CreateDraft creates an organization, its owner membership, default settings
// and onboarding stages atomically.
//
// The organization is never activated here: it starts in DRAFT and must pass
// every activation gate.
func (s *Service) CreateDraft(ctx context.Context, input CreateDraftInput) (*models.Organization, error) {
	if input.LegalName == "" || input.CountryCode == "" {
		return nil, apperrors.Validation("Legal name and country are required.", []apperrors.FieldViolation{
			{Field: "legalName", Message: "Required."}})
	}

	organization := &models.Organization{
		ID:                 ids.New(ids.PrefixOrganization),
		Status:             models.OrganizationStatusDraft,
		LegalName:          input.LegalName,
		TradingName:        input.TradingName,
		RegistrationNumber: input.RegistrationNumber,
		CountryCode:        input.CountryCode,
		Timezone:           input.Timezone,
		DefaultCurrency:    input.DefaultCurrency,
	}

	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Organizations.Create(ctx, tx, organization); err != nil {
			return err
		}
		if _, err := s.opts.Memberships.CreateOwnerMembership(ctx, tx, organization.ID, input.IdentityID, input.OwnerName); err != nil {
			return err
		}
		if err := s.opts.Settings.Create(ctx, tx, organization.ID, &models.Settings{
			AssignmentTimeoutSeconds: 300,
			ProviderVisibility:       "ORGANIZATION",
		}); err != nil {
			return err
		}
		// Owner contact is already verified — the caller reached here with a
		// verified corporate session.
		if err := s.opts.Onboarding.UpsertStage(ctx, tx, organization.ID, models.StageOwnerContact, models.StageStatusComplete, nil, nil); err != nil {
			return err
		}
		for _, stage := range models.AllStages[1:] {
			if err := s.opts.Onboarding.UpsertStage(ctx, tx, organization.ID, stage, models.StageStatusNotStarted, nil, nil); err != nil {
				return err
			}
		}

		entry := audit.Entry{
			OrganizationID: organization.ID,
			ActorType:      string(tenant.ActorTypeOrganizationUser),
			ActorID:        input.IdentityID,
			Action:         "organization.created",
			ResourceType:   "organization",
			ResourceID:     organization.ID,
			NewValues:      map[string]any{"legalName": input.LegalName, "countryCode": input.CountryCode},
			RequestID:      input.RequestID,
			CorrelationID:  input.CorrelationID,
		}
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "organization.created.v1", "organization",
			organization.ID, 1, organization.ID, input.CorrelationID, map[string]any{
				"organizationId": organization.ID,
				"legalName":      input.LegalName,
				"countryCode":    input.CountryCode,
			})
	})
	if err != nil {
		return nil, err
	}
	s.opts.Metrics.IncCounter("fleet_partner_organization_onboarding_stage_total",
		map[string]string{"stage": models.StageOwnerContact, "status": models.StageStatusComplete}, 1)
	return organization, nil
}

// OnboardingStatus is the resumable onboarding view.
type OnboardingStatus struct {
	OrganizationID      string
	OrganizationStatus  string
	OrganizationVersion int64
	Stages              []models.OnboardingStage
	Verification        *models.VerificationRequest
	VerificationIssues  []models.VerificationIssue
	Documents           []models.Document
	Wallet              *models.WalletProvisioning
	AllowedNextActions  []string
	UpdatedAt           time.Time
}

// GetOnboardingStatus assembles the resumable onboarding state.
//
// It returns only organization-safe data: no reviewer-only notes and no
// internal references beyond the organization's own records.
func (s *Service) GetOnboardingStatus(ctx context.Context, organizationID string) (*OnboardingStatus, error) {
	organization, err := s.opts.Organizations.GetByID(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	stages, err := s.opts.Onboarding.ListStages(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	documents, err := s.opts.Onboarding.ListDocuments(ctx, organizationID)
	if err != nil {
		return nil, err
	}

	status := &OnboardingStatus{
		OrganizationID:      organizationID,
		OrganizationStatus:  organization.Status,
		OrganizationVersion: organization.Version,
		Stages:              stages,
		Documents:           documents,
		UpdatedAt:           organization.UpdatedAt,
	}

	if verification, err := s.opts.Onboarding.LatestVerificationRequest(ctx, organizationID); err == nil {
		status.Verification = verification
		issues, err := s.opts.Onboarding.ListVerificationIssues(ctx, organizationID, verification.ID)
		if err != nil {
			return nil, err
		}
		status.VerificationIssues = issues
	} else if !errors.Is(err, orgrepos.ErrNotFound) {
		return nil, err
	}

	if wallet, err := s.opts.Onboarding.GetWalletProvisioning(ctx, organizationID); err == nil {
		status.Wallet = wallet
	} else if !errors.Is(err, orgrepos.ErrNotFound) {
		return nil, err
	}

	status.AllowedNextActions = allowedActions(organization.Status, stages)
	return status, nil
}

func allowedActions(organizationStatus string, stages []models.OnboardingStage) []string {
	actions := []string{}
	if AcceptsOnboardingEdits(organizationStatus) {
		actions = append(actions, "UPDATE_PROFILE", "MANAGE_REPRESENTATIVES",
			"SET_CAPABILITIES", "MANAGE_BRANCHES", "ATTACH_DOCUMENTS", "UPDATE_SETTINGS")
		if allComplete(stages, requiredForSubmission) {
			actions = append(actions, "SUBMIT_FOR_VERIFICATION")
		}
	}
	switch organizationStatus {
	case models.OrganizationStatusActive:
		actions = append(actions, "REQUEST_CLOSURE")
	case models.OrganizationStatusUnderReview, models.OrganizationStatusBusinessSubmitted:
		actions = append(actions, "AWAIT_VERIFICATION")
	}
	return actions
}

// requiredForSubmission lists the stages that must be COMPLETE before an
// organization may be submitted for verification.
var requiredForSubmission = []string{
	models.StageOwnerContact, models.StageBusinessProfile, models.StageAuthorizedRepresentative,
	models.StageOperatingCapabilities, models.StageHeadquarters, models.StageBusinessDocuments,
}

func allComplete(stages []models.OnboardingStage, required []string) bool {
	byName := map[string]string{}
	for _, stage := range stages {
		byName[stage.Stage] = stage.Status
	}
	for _, name := range required {
		if byName[name] != models.StageStatusComplete {
			return false
		}
	}
	return true
}

// MissingStages returns the required stages that are not complete.
func MissingStages(stages []models.OnboardingStage) []string {
	byName := map[string]string{}
	for _, stage := range stages {
		byName[stage.Stage] = stage.Status
	}
	missing := []string{}
	for _, name := range requiredForSubmission {
		if byName[name] != models.StageStatusComplete {
			missing = append(missing, name)
		}
	}
	return missing
}

// TransitionInput drives a lifecycle status change.
type TransitionInput struct {
	Tenant          tenant.Context
	OrganizationID  string
	TargetStatus    string
	ReasonCode      string
	ReasonText      string
	EvidenceFileIDs []string
	ExpectedVersion int64
	EventType       string
}

// Transition performs a validated lifecycle move.
//
// It locks the aggregate through a compare-and-swap update, appends status
// history, writes audit and emits the outbox event in one transaction. This
// is the only path that changes organizations.status.
func (s *Service) Transition(ctx context.Context, input TransitionInput) error {
	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		organization, err := s.opts.Organizations.GetByID(ctx, input.OrganizationID)
		if err != nil {
			return err
		}
		if err := RequireTransition(organization.Status, input.TargetStatus); err != nil {
			return apperrors.Conflict("This organization cannot move to that state right now.", err)
		}
		expected := input.ExpectedVersion
		if expected == 0 {
			expected = organization.Version
		}

		if err := s.opts.Organizations.UpdateStatus(ctx, tx, input.OrganizationID, expected, models.StatusChange{
			PreviousStatus:  organization.Status,
			NewStatus:       input.TargetStatus,
			ActorType:       string(input.Tenant.ActorType),
			ActorID:         input.Tenant.ActorID,
			ReasonCode:      input.ReasonCode,
			ReasonText:      input.ReasonText,
			EvidenceFileIDs: input.EvidenceFileIDs,
			RequestID:       input.Tenant.RequestID,
			CorrelationID:   input.Tenant.CorrelationID,
		}); err != nil {
			return err
		}

		entry := audit.EntryFromTenant(input.Tenant, "organization.status_changed", "organization", input.OrganizationID)
		entry.PreviousValues = map[string]any{"status": organization.Status}
		entry.NewValues = map[string]any{"status": input.TargetStatus}
		entry.Reason = input.ReasonCode
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}

		eventType := input.EventType
		if eventType == "" {
			eventType = eventForStatus(input.TargetStatus)
		}
		return s.opts.Events.Publish(ctx, tx, eventType, "organization", input.OrganizationID,
			expected+1, input.OrganizationID, input.Tenant.CorrelationID, map[string]any{
				"organizationId": input.OrganizationID,
				"previousStatus": organization.Status,
				"status":         input.TargetStatus,
				"reasonCode":     input.ReasonCode,
			})
	})
}

func eventForStatus(status string) string {
	switch status {
	case models.OrganizationStatusContactVerified:
		return "organization.contact_verified.v1"
	case models.OrganizationStatusBusinessSubmitted:
		return "organization.business_submitted.v1"
	case models.OrganizationStatusUnderReview:
		return "organization.under_review.v1"
	case models.OrganizationStatusChangesRequired:
		return "organization.changes_required.v1"
	case models.OrganizationStatusActive:
		return "organization.activated.v1"
	case models.OrganizationStatusSuspended:
		return "organization.suspended.v1"
	case models.OrganizationStatusClosed:
		return "organization.closed.v1"
	default:
		return "organization.updated.v1"
	}
}

// AttachDocumentInput attaches a media-file-service file.
type AttachDocumentInput struct {
	Tenant       tenant.Context
	DocumentType string
	FileID       string
	IssuedAt     *time.Time
	ExpiresAt    *time.Time
}

// AttachDocument validates file ownership and readiness with
// media-file-service before recording the reference.
//
// A file owned by another organization, or one that is not fully uploaded and
// scanned, is refused. No binary content is ever stored locally.
func (s *Service) AttachDocument(ctx context.Context, input AttachDocumentInput) (*models.Document, error) {
	if input.FileID == "" || input.DocumentType == "" {
		return nil, apperrors.Validation("A document type and file are required.", []apperrors.FieldViolation{
			{Field: "fileId", Message: "Required."}})
	}

	metadata, err := s.opts.Contracts.Media.GetFile(ctx, platformclients.CallMeta{
		RequestID: input.Tenant.RequestID, CorrelationID: input.Tenant.CorrelationID,
	}, input.FileID)
	if err != nil {
		// Unconfigured or unreachable media service must not let an
		// unverified file be attached.
		return nil, errorcodes.Unavailable(errorcodes.OrganizationOnboardingIncomplete,
			"Document verification is unavailable right now. Please try again shortly.")
	}
	if metadata.OwnerID != input.Tenant.OrganizationID {
		// Never reveal that the file exists under another organization.
		return nil, apperrors.NotFound("File not found.", nil)
	}
	if !metadata.Ready() {
		return nil, apperrors.Conflict("This file is still being processed. Try again once it is ready.", nil)
	}

	document := &models.Document{
		DocumentType:  input.DocumentType,
		FileID:        input.FileID,
		Status:        models.DocumentStatusReady,
		IssuedAt:      input.IssuedAt,
		ExpiresAt:     input.ExpiresAt,
		SourceVersion: metadata.SourceVersion,
	}
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Onboarding.AttachDocument(ctx, tx, input.Tenant.OrganizationID, document); err != nil {
			return err
		}
		if err := s.opts.Onboarding.UpsertStage(ctx, tx, input.Tenant.OrganizationID,
			models.StageBusinessDocuments, models.StageStatusInProgress, nil, nil); err != nil {
			return err
		}
		entry := audit.EntryFromTenant(input.Tenant, "organization.document_attached", "organization_document", document.ID)
		entry.NewValues = map[string]any{"documentType": input.DocumentType, "fileId": input.FileID}
		return s.opts.Audit.Record(ctx, tx, entry)
	})
	if err != nil {
		return nil, err
	}
	return document, nil
}

// Repositories exposes read paths to handlers.
func (s *Service) Repositories() (*orgrepos.OrganizationRepository, *orgrepos.OnboardingRepository) {
	return s.opts.Organizations, s.opts.Onboarding
}

// MarkStage records progress for one stage.
func (s *Service) MarkStage(ctx context.Context, organizationID string, stage string, status string, missing []string, blocking []string) error {
	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		return s.opts.Onboarding.UpsertStage(ctx, tx, organizationID, stage, status, missing, blocking)
	})
}

func (s *Service) metricsStage(stage string, status string) {
	s.opts.Metrics.IncCounter("fleet_partner_organization_onboarding_stage_total",
		map[string]string{"stage": stage, "status": status}, 1)
}

var _ = fmt.Sprintf

// SetMembershipCreator wires the staff service after construction. The two
// services depend on each other's ports (organization status / owner
// membership), so one edge is completed here rather than through an import.
func (s *Service) SetMembershipCreator(creator MembershipCreator) {
	s.opts.Memberships = creator
}
