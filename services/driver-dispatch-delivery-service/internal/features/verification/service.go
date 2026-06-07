package verification

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"karrygo/shared/go/apperrors"
)

const (
	maxGovtIDFileSize   int64 = 5 * 1024 * 1024
	maxLicenceFileSize  int64 = 5 * 1024 * 1024
	maxProfilePhotoSize int64 = 3 * 1024 * 1024
	maxSelfieFileSize   int64 = 3 * 1024 * 1024
)

type Service struct {
	repository Repository
	faceClient FaceMatcher
	uploader   FileUploader
	events     EventPublisher
}

type ServiceOption func(*Service)

func WithUploader(uploader FileUploader) ServiceOption {
	return func(s *Service) {
		s.uploader = uploader
	}
}

func WithEventPublisher(events EventPublisher) ServiceOption {
	return func(s *Service) {
		s.events = events
	}
}

func NewService(repository Repository, faceClient FaceMatcher, options ...ServiceOption) *Service {
	service := &Service{
		repository: repository,
		faceClient: faceClient,
		uploader:   UnconfiguredUploader{},
	}
	for _, option := range options {
		option(service)
	}
	if service.faceClient == nil {
		service.faceClient = NewStubSmileIdentityClient()
	}
	if service.uploader == nil {
		service.uploader = UnconfiguredUploader{}
	}
	return service
}

func (s *Service) SubmitIdentity(ctx context.Context, input IdentitySubmissionInput) (StepSubmissionResponse, error) {
	if err := validateProviderID(input.ProviderID); err != nil {
		return StepSubmissionResponse{}, err
	}
	if err := validateIdentityInput(input); err != nil {
		return StepSubmissionResponse{}, err
	}

	govtID, err := validateFile(input.GovtIDFile, "govt_id_file", maxGovtIDFileSize, map[string]struct{}{
		"image/jpeg":      {},
		"image/png":       {},
		"application/pdf": {},
	})
	if err != nil {
		return StepSubmissionResponse{}, err
	}
	profilePhoto, err := validateFile(input.ProfilePhoto, "profile_photo", maxProfilePhotoSize, map[string]struct{}{
		"image/jpeg": {},
		"image/png":  {},
	})
	if err != nil {
		return StepSubmissionResponse{}, err
	}

	step, err := s.requireSubmittableStep(ctx, input.ProviderID, StepIdentity)
	if err != nil {
		return StepSubmissionResponse{}, err
	}

	govtIDURL, err := s.upload(ctx, input.GovtIDFile, buildVerificationObjectPath(input.ProviderID, StepIdentity, input.GovtIDFile.Header.GetFilename()))
	if err != nil {
		return StepSubmissionResponse{}, apperrors.Internal("Storage upload failed.", err)
	}
	profilePhotoURL, err := s.upload(ctx, input.ProfilePhoto, buildVerificationObjectPath(input.ProviderID, StepIdentity, input.ProfilePhoto.Header.GetFilename()))
	if err != nil {
		return StepSubmissionResponse{}, apperrors.Internal("Storage upload failed.", err)
	}

	if err := s.repository.InsertDocument(ctx, DocumentInput{
		StepID:       step.ID,
		ProviderID:   input.ProviderID,
		DocumentType: "govt_id",
		FileURL:      govtIDURL,
		FileSize:     intPtrFromInt64(govtID.size),
		MimeType:     &govtID.mimeType,
	}); err != nil {
		return StepSubmissionResponse{}, err
	}
	if err := s.repository.InsertDocument(ctx, DocumentInput{
		StepID:       step.ID,
		ProviderID:   input.ProviderID,
		DocumentType: "profile_photo",
		FileURL:      profilePhotoURL,
		FileSize:     intPtrFromInt64(profilePhoto.size),
		MimeType:     &profilePhoto.mimeType,
	}); err != nil {
		return StepSubmissionResponse{}, err
	}

	updated, err := s.repository.UpdateStepSubmitted(ctx, input.ProviderID, StepIdentity)
	if err != nil {
		return StepSubmissionResponse{}, err
	}
	if err := s.insertSubmissionAudit(ctx, input.ProviderID, StepIdentity, step.Status, "Identity documents submitted."); err != nil {
		return StepSubmissionResponse{}, err
	}
	if err := s.repository.UpdateProviderProfilePhoto(ctx, input.ProviderID, profilePhotoURL); err != nil {
		return StepSubmissionResponse{}, err
	}
	if err := s.publishStepSubmitted(ctx, input.CorrelationID, input.ProviderID, StepIdentity); err != nil {
		return StepSubmissionResponse{}, apperrors.Internal("Event publishing failed.", err)
	}

	return StepSubmissionResponse{
		Step:        StepIdentity,
		Status:      updated.Status,
		SubmittedAt: derefTime(updated.SubmittedAt),
		Message:     "Documents submitted. Under review.",
	}, nil
}

func (s *Service) SubmitLicence(ctx context.Context, input LicenceSubmissionInput) (StepSubmissionResponse, error) {
	if err := validateProviderID(input.ProviderID); err != nil {
		return StepSubmissionResponse{}, err
	}
	if err := validateLicenceInput(input); err != nil {
		return StepSubmissionResponse{}, err
	}
	licenceFile, err := validateFile(input.LicenceFile, "licence_file", maxLicenceFileSize, map[string]struct{}{
		"image/jpeg":      {},
		"image/png":       {},
		"application/pdf": {},
	})
	if err != nil {
		return StepSubmissionResponse{}, err
	}

	step, err := s.requireSubmittableStep(ctx, input.ProviderID, StepLicence)
	if err != nil {
		return StepSubmissionResponse{}, err
	}
	licenceURL, err := s.upload(ctx, input.LicenceFile, buildVerificationObjectPath(input.ProviderID, StepLicence, input.LicenceFile.Header.GetFilename()))
	if err != nil {
		return StepSubmissionResponse{}, apperrors.Internal("Storage upload failed.", err)
	}
	if err := s.repository.InsertDocument(ctx, DocumentInput{
		StepID:       step.ID,
		ProviderID:   input.ProviderID,
		DocumentType: "licence_doc",
		FileURL:      licenceURL,
		FileSize:     intPtrFromInt64(licenceFile.size),
		MimeType:     &licenceFile.mimeType,
	}); err != nil {
		return StepSubmissionResponse{}, err
	}
	updated, err := s.repository.UpdateStepSubmitted(ctx, input.ProviderID, StepLicence)
	if err != nil {
		return StepSubmissionResponse{}, err
	}
	if err := s.insertSubmissionAudit(ctx, input.ProviderID, StepLicence, step.Status, "Licence submitted."); err != nil {
		return StepSubmissionResponse{}, err
	}
	if err := s.publishStepSubmitted(ctx, input.CorrelationID, input.ProviderID, StepLicence); err != nil {
		return StepSubmissionResponse{}, apperrors.Internal("Event publishing failed.", err)
	}

	return StepSubmissionResponse{
		Step:        StepLicence,
		Status:      updated.Status,
		SubmittedAt: derefTime(updated.SubmittedAt),
		Message:     "Licence submitted. Under review.",
	}, nil
}

func (s *Service) SubmitFace(ctx context.Context, input FaceSubmissionInput) (FaceSubmissionResponse, error) {
	if err := validateProviderID(input.ProviderID); err != nil {
		return FaceSubmissionResponse{}, err
	}
	if _, err := validateFile(input.Selfie, "selfie", maxSelfieFileSize, map[string]struct{}{
		"image/jpeg": {},
		"image/png":  {},
	}); err != nil {
		return FaceSubmissionResponse{}, err
	}

	identity, ok, err := s.repository.GetStep(ctx, input.ProviderID, StepIdentity)
	if err != nil {
		return FaceSubmissionResponse{}, err
	}
	if !ok || (identity.Status != StatusSubmitted && identity.Status != StatusApproved) {
		return FaceSubmissionResponse{}, preconditionFailed("Identity verification must be submitted before face verification.")
	}
	govtIDDoc, ok, err := s.repository.GetLatestIdentityGovtIDDocument(ctx, input.ProviderID)
	if err != nil {
		return FaceSubmissionResponse{}, err
	}
	if !ok {
		return FaceSubmissionResponse{}, preconditionFailed("Government ID document is required before face verification.")
	}

	step, err := s.requireSubmittableStep(ctx, input.ProviderID, StepFace)
	if err != nil {
		return FaceSubmissionResponse{}, err
	}
	selfieURL, err := s.upload(ctx, input.Selfie, buildVerificationObjectPath(input.ProviderID, StepFace, input.Selfie.Header.GetFilename()))
	if err != nil {
		return FaceSubmissionResponse{}, apperrors.Internal("Storage upload failed.", err)
	}

	check, err := s.repository.InsertFaceCheck(ctx, FaceCheckInput{
		ProviderID:   input.ProviderID,
		StepID:       step.ID,
		SelfieURL:    selfieURL,
		IDDocURL:     govtIDDoc.FileURL,
		ProviderUsed: "smile_identity",
	})
	if err != nil {
		return FaceSubmissionResponse{}, err
	}

	match, err := s.faceClient.MatchFace(ctx, selfieURL, govtIDDoc.FileURL)
	if err != nil {
		message := err.Error()
		_, _ = s.repository.UpdateFaceCheckResult(ctx, FaceCheckResultInput{ID: check.ID, ErrorMessage: &message})
		return FaceSubmissionResponse{}, apperrors.Internal("Smile Identity API unavailable.", err)
	}

	result := "fail"
	if match.Passed {
		result = "pass"
	}
	_, err = s.repository.UpdateFaceCheckResult(ctx, FaceCheckResultInput{
		ID:         check.ID,
		MatchScore: &match.MatchScore,
		Result:     &result,
	})
	if err != nil {
		return FaceSubmissionResponse{}, err
	}

	if !match.Passed {
		notes := "Face did not match ID document."
		if err := s.repository.InsertAudit(ctx, AuditInput{
			ProviderID: input.ProviderID,
			Step:       StepFace,
			Action:     AuditActionFaceFailed,
			FromStatus: step.Status,
			ToStatus:   step.Status,
			Notes:      &notes,
		}); err != nil {
			return FaceSubmissionResponse{}, err
		}
		if err := s.publishFaceFailed(ctx, input.CorrelationID, input.ProviderID, match.MatchScore); err != nil {
			return FaceSubmissionResponse{}, apperrors.Internal("Event publishing failed.", err)
		}
		return FaceSubmissionResponse{
			Step:       StepFace,
			Result:     "fail",
			MatchScore: match.MatchScore,
			Status:     step.Status,
			Message:    "Face did not match ID document. Please retake selfie in good lighting.",
		}, nil
	}

	updated, err := s.repository.UpdateStepSubmitted(ctx, input.ProviderID, StepFace)
	if err != nil {
		return FaceSubmissionResponse{}, err
	}
	notes := "Face matched. Pending admin review."
	if err := s.insertSubmissionAudit(ctx, input.ProviderID, StepFace, step.Status, notes); err != nil {
		return FaceSubmissionResponse{}, err
	}
	if err := s.publishStepSubmitted(ctx, input.CorrelationID, input.ProviderID, StepFace); err != nil {
		return FaceSubmissionResponse{}, apperrors.Internal("Event publishing failed.", err)
	}

	return FaceSubmissionResponse{
		Step:       StepFace,
		Result:     "pass",
		MatchScore: match.MatchScore,
		Status:     updated.Status,
		Message:    "Face verified. Pending final admin review.",
	}, nil
}

// manualReviewSteps are the only steps that accept manual admin review.
// vehicle is handled by the vehicle feature event.
// guarantor and emergency are auto-confirmed and cannot be manually reviewed.
func manualReviewSteps() map[Step]struct{} {
	return map[Step]struct{}{
		StepIdentity: {},
		StepLicence:  {},
		StepFace:     {},
	}
}

func (s *Service) AdminReview(ctx context.Context, reviewerID, correlationID, providerID string, input AdminReviewRequest) (AdminReviewResponse, error) {
	if err := validateProviderID(providerID); err != nil {
		return AdminReviewResponse{}, err
	}
	if err := validateAdminReviewInput(input); err != nil {
		return AdminReviewResponse{}, err
	}

	// Enforce manual-review-only steps.
	if _, ok := manualReviewSteps()[input.Step]; !ok {
		switch input.Step {
		case StepVehicle:
			return AdminReviewResponse{}, validationError("step", "Vehicle step is reviewed via the vehicle feature and cannot be manually reviewed here.")
		default:
			return AdminReviewResponse{}, validationError("step", "This step is auto-confirmed and cannot be manually reviewed.")
		}
	}

	current, ok, err := s.repository.GetStep(ctx, providerID, input.Step)
	if err != nil {
		return AdminReviewResponse{}, err
	}
	if !ok {
		return AdminReviewResponse{}, apperrors.NotFound("Verification step was not found.", nil)
	}

	if input.Action == AdminActionApprove && current.Status == StatusApproved {
		return AdminReviewResponse{}, apperrors.Conflict("Verification step is already approved.", nil)
	}

	var updated VerificationStep
	var auditAction AuditAction

	switch input.Action {
	case AdminActionApprove:
		updated, err = s.repository.AdminApproveStep(ctx, providerID, input.Step, reviewerID)
		if err != nil {
			return AdminReviewResponse{}, err
		}
		auditAction = AuditActionApproved
	case AdminActionReject:
		updated, err = s.repository.AdminRejectStep(ctx, providerID, input.Step, reviewerID, input.Reason)
		if err != nil {
			return AdminReviewResponse{}, err
		}
		auditAction = AuditActionRejected
	}

	// Insert audit row.
	notes := input.Reason
	var notesPtr *string
	if notes != "" {
		notesPtr = &notes
	}
	if err := s.repository.InsertAudit(ctx, AuditInput{
		ProviderID:  providerID,
		Step:        input.Step,
		Action:      auditAction,
		FromStatus:  current.Status,
		ToStatus:    updated.Status,
		PerformedBy: &reviewerID,
		Notes:       notesPtr,
	}); err != nil {
		return AdminReviewResponse{}, err
	}

	// Publish step-level status update event.
	if s.events != nil {
		_ = s.events.PublishVerificationStatusUpdated(ctx, VerificationStatusUpdatedEvent{
			Event:         TopicVerificationStatusUpdated,
			CorrelationID: correlationID,
			ProviderID:    providerID,
			Step:          input.Step,
			Status:        updated.Status,
			CreatedAt:     time.Now().UTC(),
		})
	}

	if input.Action == AdminActionReject {
		// Publish rejection event.
		if s.events != nil {
			_ = s.events.PublishVerificationRejected(ctx, VerificationRejectedEvent{
				Event:         TopicVerificationRejected,
				CorrelationID: correlationID,
				ProviderID:    providerID,
				Step:          input.Step,
				Reason:        input.Reason,
				CreatedAt:     time.Now().UTC(),
			})
		}
	} else {
		// On approve: run the fully-approved gate (idempotent).
		if err := s.markProviderVerifiedIfComplete(ctx, providerID, correlationID); err != nil {
			return AdminReviewResponse{}, err
		}
	}

	return AdminReviewResponse{
		Step:            updated.Step,
		Status:          updated.Status,
		ReviewedAt:      updated.ReviewedAt,
		ReviewerID:      updated.ReviewerID,
		RejectionReason: updated.RejectionReason,
	}, nil
}

func (s *Service) GetAllStatus(ctx context.Context, providerID string) (AllStatusResponse, error) {
	if err := validateProviderID(providerID); err != nil {
		return AllStatusResponse{}, err
	}
	steps, err := s.repository.ListSteps(ctx, providerID)
	if err != nil {
		return AllStatusResponse{}, err
	}
	if len(steps) == 0 {
		return AllStatusResponse{}, apperrors.NotFound("Verification steps were not found.", nil)
	}
	state, ok, err := s.repository.GetProviderVerificationState(ctx, providerID)
	if err != nil {
		return AllStatusResponse{}, err
	}
	return AllStatusResponse{
		OverallStatus:        calculateOverallStatus(steps, state, ok),
		CompletionPercentage: calculateCompletionPercentage(steps),
		Steps:                summarizeSteps(steps),
	}, nil
}

func (s *Service) GetStepStatus(ctx context.Context, providerID string, step Step) (StepStatusResponse, error) {
	if err := validateProviderID(providerID); err != nil {
		return StepStatusResponse{}, err
	}
	if !IsValidStep(step) {
		return StepStatusResponse{}, apperrors.NotFound("Verification step was not found.", nil)
	}
	result, ok, err := s.repository.GetStep(ctx, providerID, step)
	if err != nil {
		return StepStatusResponse{}, err
	}
	if !ok {
		return StepStatusResponse{}, apperrors.NotFound("Verification step was not found.", nil)
	}
	documents, err := s.repository.ListStepDocuments(ctx, providerID, step)
	if err != nil {
		return StepStatusResponse{}, err
	}
	response := stepStatusResponse(result, documents)
	if step == StepFace {
		check, ok, err := s.repository.GetLastFaceCheck(ctx, providerID)
		if err != nil {
			return StepStatusResponse{}, err
		}
		if ok {
			response.LastFaceCheck = &LastFaceCheckResponse{
				Result:     check.Result,
				MatchScore: check.MatchScore,
				CheckedAt:  check.CheckedAt,
			}
		}
	}
	return response, nil
}

func (s *Service) InitializeForCompletedOnboarding(ctx context.Context, providerID string) (InitializationResult, error) {
	if err := validateProviderID(providerID); err != nil {
		return InitializationResult{}, err
	}
	return s.repository.InitializeStepsForProvider(ctx, providerID)
}

func (s *Service) ApplyVehicleVerified(ctx context.Context, event VehicleVerifiedEvent) error {
	if err := validateProviderID(event.ProviderID); err != nil {
		return err
	}

	previous, ok, err := s.repository.GetStep(ctx, event.ProviderID, StepVehicle)
	if err != nil {
		return err
	}
	if !ok {
		log.Printf("verification vehicle.verified provider_id=%s ignored because vehicle step does not exist", event.ProviderID)
		return nil
	}
	if previous.Status == StatusApproved {
		log.Printf("verification vehicle.verified provider_id=%s ignored because vehicle step is already approved", event.ProviderID)
		return s.markProviderVerifiedIfComplete(ctx, event.ProviderID, event.CorrelationID)
	}

	updated, ok, err := s.repository.UpdateVehicleStepApproved(ctx, event.ProviderID, event.BikeID)
	if err != nil {
		return err
	}
	if !ok {
		log.Printf("verification vehicle.verified provider_id=%s ignored because vehicle step does not exist", event.ProviderID)
		return nil
	}

	notes := "Vehicle approved from vehicle.verified event."
	if err := s.repository.InsertAudit(ctx, AuditInput{
		ProviderID: event.ProviderID,
		Step:       StepVehicle,
		Action:     AuditActionApproved,
		FromStatus: previous.Status,
		ToStatus:   updated.Status,
		Notes:      &notes,
	}); err != nil {
		return err
	}
	return s.markProviderVerifiedIfComplete(ctx, event.ProviderID, event.CorrelationID)
}

func (s *Service) ApplyVehicleRejected(ctx context.Context, event VehicleRejectedEvent) error {
	if err := validateProviderID(event.ProviderID); err != nil {
		return err
	}

	reason := strings.TrimSpace(event.Reason)
	if reason == "" {
		reason = "Vehicle rejected from vehicle.rejected event."
	}

	previous, ok, err := s.repository.GetStep(ctx, event.ProviderID, StepVehicle)
	if err != nil {
		return err
	}
	if !ok {
		log.Printf("verification vehicle.rejected provider_id=%s ignored because vehicle step does not exist", event.ProviderID)
		return nil
	}
	if previous.Status == StatusRejected && previous.RejectionReason != nil && *previous.RejectionReason == reason {
		log.Printf("verification vehicle.rejected provider_id=%s ignored because vehicle step is already rejected with same reason", event.ProviderID)
		return nil
	}

	updated, ok, err := s.repository.UpdateVehicleStepRejected(ctx, event.ProviderID, reason)
	if err != nil {
		return err
	}
	if !ok {
		log.Printf("verification vehicle.rejected provider_id=%s ignored because vehicle step does not exist", event.ProviderID)
		return nil
	}

	if err := s.repository.InsertAudit(ctx, AuditInput{
		ProviderID: event.ProviderID,
		Step:       StepVehicle,
		Action:     AuditActionRejected,
		FromStatus: previous.Status,
		ToStatus:   updated.Status,
		Notes:      &reason,
	}); err != nil {
		return err
	}
	return nil
}

// markProviderVerifiedIfComplete checks if all required verification steps are
// approved and, if so, fires the fully_approved gate exactly once.
//
// Idempotency: a fully_approved audit row acts as the guard — if one already
// exists the function returns immediately without re-publishing or re-inserting.
func (s *Service) markProviderVerifiedIfComplete(ctx context.Context, providerID, correlationID string) error {
	approved, err := s.repository.CheckRequiredStepsApproved(ctx, providerID)
	if err != nil {
		return err
	}
	if !approved {
		return nil
	}

	// Idempotency guard: do nothing if the audit row already exists.
	already, err := s.repository.HasFullyApprovedAudit(ctx, providerID)
	if err != nil {
		return err
	}
	if already {
		return nil
	}

	// Insert the audit row FIRST so concurrent calls don't duplicate.
	notes := "All required verification steps approved."
	if err := s.repository.InsertAudit(ctx, AuditInput{
		ProviderID: providerID,
		Step:       StepAll,
		Action:     AuditActionFullyApproved,
		FromStatus: StatusPending,
		ToStatus:   StatusApproved,
		Notes:      &notes,
	}); err != nil {
		return err
	}

	// Update provider verification status.
	if err := s.repository.UpdateProviderVerificationStatus(ctx, providerID, string(OverallStatusVerified)); err != nil {
		return err
	}

	// Publish events.
	if s.events != nil {
		now := time.Now().UTC()
		_ = s.events.PublishVerificationFullyApproved(ctx, VerificationFullyApprovedEvent{
			Event:         TopicVerificationFullyApproved,
			CorrelationID: correlationID,
			ProviderID:    providerID,
			ApprovedAt:    now,
			CreatedAt:     now,
		})
		// Also notify profile mirror subscriber so it can update providers.verification_status.
		_ = s.events.PublishVerificationStatusUpdated(ctx, VerificationStatusUpdatedEvent{
			Event:              TopicVerificationStatusUpdated,
			CorrelationID:      correlationID,
			ProviderID:         providerID,
			VerificationStatus: string(OverallStatusVerified),
			CreatedAt:          now,
		})
	}
	return nil
}

func summarizeSteps(steps []VerificationStep) []VerificationStepSummary {
	result := make([]VerificationStepSummary, 0, len(steps))
	for _, step := range steps {
		result = append(result, VerificationStepSummary{
			Step:        step.Step,
			Status:      step.Status,
			IsOptional:  step.IsOptional,
			SubmittedAt: step.SubmittedAt,
			ReviewedAt:  step.ReviewedAt,
		})
	}
	return result
}

func stepStatusResponse(step VerificationStep, documents []VerificationDocument) StepStatusResponse {
	return StepStatusResponse{
		ID:              step.ID,
		ProviderID:      step.ProviderID,
		Step:            step.Step,
		Status:          step.Status,
		IsOptional:      step.IsOptional,
		IsAutoConfirmed: step.IsAutoConfirmed,
		ConfirmMethod:   step.ConfirmMethod,
		SubmittedAt:     step.SubmittedAt,
		ReviewedAt:      step.ReviewedAt,
		ReviewerID:      step.ReviewerID,
		RejectionReason: step.RejectionReason,
		UpdatedAt:       step.UpdatedAt,
		Documents:       documents,
	}
}

func calculateCompletionPercentage(steps []VerificationStep) int {
	required := requiredVerificationSteps()
	if len(required) == 0 {
		return 0
	}
	byStep := mapSteps(steps)
	approved := 0
	for _, step := range required {
		if byStep[step].Status == StatusApproved {
			approved++
		}
	}
	return approved * 100 / len(required)
}

func calculateOverallStatus(steps []VerificationStep, state ProviderVerificationState, hasState bool) OverallStatus {
	if hasState && (!state.IsActive || state.VerificationStatus == string(OverallStatusSuspended)) {
		return OverallStatusSuspended
	}

	byStep := mapSteps(steps)
	required := requiredVerificationSteps()
	for _, step := range required {
		if byStep[step].Status == StatusRejected {
			return OverallStatusRejected
		}
	}

	allApproved := true
	allSubmittedOrApproved := true
	for _, step := range required {
		status := byStep[step].Status
		if status != StatusApproved {
			allApproved = false
		}
		if status != StatusSubmitted && status != StatusApproved {
			allSubmittedOrApproved = false
		}
	}
	if allApproved {
		return OverallStatusVerified
	}
	if allSubmittedOrApproved {
		return OverallStatusPendingReview
	}

	for _, step := range manualRequiredVerificationSteps() {
		status := byStep[step].Status
		if status == StatusSubmitted || status == StatusApproved {
			return OverallStatusInProgress
		}
	}
	return OverallStatusNotStarted
}

func mapSteps(steps []VerificationStep) map[Step]VerificationStep {
	result := make(map[Step]VerificationStep, len(steps))
	for _, step := range steps {
		result[step.Step] = step
	}
	return result
}

func requiredVerificationSteps() []Step {
	return []Step{StepIdentity, StepVehicle, StepFace, StepGuarantor, StepEmergency}
}

func manualRequiredVerificationSteps() []Step {
	return []Step{StepIdentity, StepVehicle, StepFace}
}

func (s *Service) requireSubmittableStep(ctx context.Context, providerID string, stepName Step) (VerificationStep, error) {
	step, ok, err := s.repository.GetStep(ctx, providerID, stepName)
	if err != nil {
		return VerificationStep{}, err
	}
	if !ok {
		return VerificationStep{}, preconditionFailed("Verification steps are not initialized.")
	}
	if step.Status == StatusApproved {
		return VerificationStep{}, apperrors.Conflict("Verification step is already approved.", nil)
	}
	return step, nil
}

func (s *Service) upload(ctx context.Context, upload FileUpload, objectPath string) (string, error) {
	file, err := upload.Header.Open()
	if err != nil {
		return "", err
	}
	defer file.Close()
	return s.uploader.Upload(ctx, objectPath, file, upload.Header)
}

func (s *Service) insertSubmissionAudit(ctx context.Context, providerID string, step Step, previous StepStatus, notes string) error {
	action := AuditActionSubmitted
	if previous == StatusSubmitted || previous == StatusRejected {
		action = AuditActionResubmitted
	}
	return s.repository.InsertAudit(ctx, AuditInput{
		ProviderID: providerID,
		Step:       step,
		Action:     action,
		FromStatus: previous,
		ToStatus:   StatusSubmitted,
		Notes:      &notes,
	})
}

func (s *Service) publishStepSubmitted(ctx context.Context, correlationID string, providerID string, step Step) error {
	if s.events == nil {
		return nil
	}
	return s.events.PublishStepSubmitted(ctx, StepSubmittedEvent{
		Event:         TopicVerificationStepSubmitted,
		CorrelationID: correlationID,
		ProviderID:    providerID,
		Step:          step,
		Status:        StatusSubmitted,
		CreatedAt:     time.Now().UTC(),
	})
}

func (s *Service) publishFaceFailed(ctx context.Context, correlationID string, providerID string, matchScore float64) error {
	if s.events == nil {
		return nil
	}
	return s.events.PublishFaceFailed(ctx, FaceFailedEvent{
		Event:         TopicVerificationFaceFailed,
		CorrelationID: correlationID,
		ProviderID:    providerID,
		Step:          StepFace,
		Result:        "fail",
		MatchScore:    matchScore,
		CreatedAt:     time.Now().UTC(),
	})
}

func validateIdentityInput(input IdentitySubmissionInput) error {
	var fields []apperrors.FieldViolation
	if !isValidGovtIDType(input.GovtIDType) {
		fieldMessage := "Government ID type is invalid."
		if strings.TrimSpace(input.GovtIDType) == "" {
			fieldMessage = "Government ID type is required."
		}
		fields = append(fields, apperrors.FieldViolation{Field: "govt_id_type", Message: fieldMessage})
	}
	if strings.TrimSpace(input.GovtIDNumber) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "govt_id_number", Message: "Government ID number is required."})
	}
	if input.GovtIDFile.Header == nil {
		fields = append(fields, apperrors.FieldViolation{Field: "govt_id_file", Message: "Government ID file is required."})
	}
	if input.ProfilePhoto.Header == nil {
		fields = append(fields, apperrors.FieldViolation{Field: "profile_photo", Message: "Profile photo is required."})
	}
	if len(fields) > 0 {
		return validationErrors(fields)
	}
	return nil
}

func validateLicenceInput(input LicenceSubmissionInput) error {
	var fields []apperrors.FieldViolation
	if strings.TrimSpace(input.LicenceNumber) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "licence_number", Message: "Licence number is required."})
	}
	if !isFourDigitYear(input.ExpiryYear) {
		fields = append(fields, apperrors.FieldViolation{Field: "expiry_year", Message: "Expiry year must be a 4-digit year."})
	}
	if !isTwoDigitMonth(input.ExpiryMonth) {
		fields = append(fields, apperrors.FieldViolation{Field: "expiry_month", Message: "Expiry month must be between 01 and 12."})
	}
	if input.LicenceFile.Header == nil {
		fields = append(fields, apperrors.FieldViolation{Field: "licence_file", Message: "Licence file is required."})
	}
	if len(fields) > 0 {
		return validationErrors(fields)
	}
	return nil
}

type validatedFile struct {
	mimeType string
	size     int64
}

func validateFile(upload FileUpload, field string, maxSize int64, allowed map[string]struct{}) (validatedFile, error) {
	if upload.Header == nil {
		return validatedFile{}, validationError(field, "File is required.")
	}
	size := upload.Header.GetSize()
	if size <= 0 {
		return validatedFile{}, validationError(field, "File is required.")
	}
	if size > maxSize {
		return validatedFile{}, validationError(field, "File is too large.")
	}

	mimeType, err := detectFileMIME(upload.Header)
	if err != nil {
		return validatedFile{}, apperrors.Internal("Uploaded file could not be read.", err)
	}
	if _, ok := allowed[mimeType]; !ok {
		return validatedFile{}, validationError(field, "File type is not supported.")
	}
	return validatedFile{mimeType: mimeType, size: size}, nil
}

func detectFileMIME(header FileHeader) (string, error) {
	file, err := header.Open()
	if err != nil {
		return "", err
	}
	defer file.Close()

	buf := make([]byte, 512)
	n, err := io.ReadFull(file, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", err
	}
	sniffed := buf[:n]
	if bytes.HasPrefix(sniffed, []byte("%PDF")) {
		return "application/pdf", nil
	}
	return http.DetectContentType(sniffed), nil
}

func isValidGovtIDType(value string) bool {
	switch strings.TrimSpace(value) {
	case "nin", "passport", "voter_card", "drivers_licence":
		return true
	default:
		return false
	}
}

func isFourDigitYear(value string) bool {
	if len(value) != 4 {
		return false
	}
	year, err := strconv.Atoi(value)
	return err == nil && year >= 1900 && year <= 9999
}

func isTwoDigitMonth(value string) bool {
	if len(value) != 2 {
		return false
	}
	month, err := strconv.Atoi(value)
	return err == nil && month >= 1 && month <= 12
}

func intPtrFromInt64(value int64) *int {
	if value <= 0 {
		return nil
	}
	converted := int(value)
	return &converted
}

func derefTime(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}

func validateAdminReviewInput(input AdminReviewRequest) error {
	var fields []apperrors.FieldViolation
	if !IsValidStep(input.Step) {
		msg := "Step is required."
		if input.Step != "" {
			msg = "Step is invalid."
		}
		fields = append(fields, apperrors.FieldViolation{Field: "step", Message: msg})
	}
	if input.Action != AdminActionApprove && input.Action != AdminActionReject {
		msg := "Action is required."
		if input.Action != "" {
			msg = "Action must be approve or reject."
		}
		fields = append(fields, apperrors.FieldViolation{Field: "action", Message: msg})
	}
	if input.Action == AdminActionReject && strings.TrimSpace(input.Reason) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "reason", Message: "Reason is required when rejecting."})
	}
	if len(fields) > 0 {
		return validationErrors(fields)
	}
	return nil
}

func validateProviderID(providerID string) error {
	if _, err := uuid.Parse(strings.TrimSpace(providerID)); err != nil {
		return validationError("provider_id", "Provider ID must be a valid UUID.")
	}
	return nil
}

func validationError(field string, message string) *apperrors.Error {
	return validationErrors([]apperrors.FieldViolation{{Field: field, Message: message}})
}

func validationErrors(fields []apperrors.FieldViolation) *apperrors.Error {
	err := apperrors.New(http.StatusBadRequest, apperrors.CodeValidationFailed, "Check your details.", nil)
	err.Fields = fields
	return err
}

func preconditionFailed(message string) *apperrors.Error {
	return apperrors.New(http.StatusPreconditionFailed, apperrors.Code("precondition_failed"), message, nil)
}

func notImplemented() *apperrors.Error {
	return apperrors.New(http.StatusNotImplemented, apperrors.Code("not_implemented"), "Verification endpoint is not implemented yet.", nil)
}

func wrapInternal(message string, err error) *apperrors.Error {
	return apperrors.Internal(message, fmt.Errorf("%w", err))
}
