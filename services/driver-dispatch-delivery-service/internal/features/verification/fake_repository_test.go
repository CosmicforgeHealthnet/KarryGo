package verification

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type fakeVerificationRepository struct {
	steps            map[string]map[Step]VerificationStep
	providerStates   map[string]ProviderVerificationState
	documents        []VerificationDocument
	faceChecks       []FaceCheck
	auditRows        []VerificationAudit
	profilePhotoURLs map[string]string
	err              error
}

func newFakeVerificationRepository() *fakeVerificationRepository {
	return &fakeVerificationRepository{
		steps:            make(map[string]map[Step]VerificationStep),
		providerStates:   make(map[string]ProviderVerificationState),
		profilePhotoURLs: make(map[string]string),
	}
}

func (r *fakeVerificationRepository) ListSteps(ctx context.Context, providerID string) ([]VerificationStep, error) {
	if r.err != nil {
		return nil, r.err
	}
	byStep := r.steps[providerID]
	ordered := []Step{StepIdentity, StepLicence, StepVehicle, StepFace, StepGuarantor, StepEmergency}
	result := make([]VerificationStep, 0, len(byStep))
	for _, step := range ordered {
		if value, ok := byStep[step]; ok {
			result = append(result, value)
		}
	}
	return result, nil
}

func (r *fakeVerificationRepository) GetStep(ctx context.Context, providerID string, step Step) (VerificationStep, bool, error) {
	if r.err != nil {
		return VerificationStep{}, false, r.err
	}
	value, ok := r.steps[providerID][step]
	return value, ok, nil
}

func (r *fakeVerificationRepository) ListStepDocuments(ctx context.Context, providerID string, step Step) ([]VerificationDocument, error) {
	if r.err != nil {
		return nil, r.err
	}
	var result []VerificationDocument
	for _, doc := range r.documents {
		stepValue, ok := r.steps[providerID][step]
		if ok && doc.ProviderID == providerID && doc.StepID == stepValue.ID {
			result = append(result, doc)
		}
	}
	return result, nil
}

func (r *fakeVerificationRepository) GetLastFaceCheck(ctx context.Context, providerID string) (FaceCheck, bool, error) {
	if r.err != nil {
		return FaceCheck{}, false, r.err
	}
	for i := len(r.faceChecks) - 1; i >= 0; i-- {
		if r.faceChecks[i].ProviderID == providerID {
			return r.faceChecks[i], true, nil
		}
	}
	return FaceCheck{}, false, nil
}

func (r *fakeVerificationRepository) GetProviderVerificationState(ctx context.Context, providerID string) (ProviderVerificationState, bool, error) {
	if r.err != nil {
		return ProviderVerificationState{}, false, r.err
	}
	state, ok := r.providerStates[providerID]
	if !ok {
		return ProviderVerificationState{ProviderID: providerID, VerificationStatus: "unverified", IsActive: true}, true, nil
	}
	return state, true, nil
}

func (r *fakeVerificationRepository) InitializeStepsForProvider(ctx context.Context, providerID string) (InitializationResult, error) {
	if r.err != nil {
		return InitializationResult{}, r.err
	}
	if providerID == "" {
		return InitializationResult{}, errors.New("provider_id is required")
	}
	if r.steps[providerID] == nil {
		r.steps[providerID] = make(map[Step]VerificationStep)
	}
	if _, ok := r.providerStates[providerID]; !ok {
		r.providerStates[providerID] = ProviderVerificationState{ProviderID: providerID, VerificationStatus: "unverified", IsActive: true}
	}

	var result InitializationResult
	now := time.Now().UTC()
	for _, def := range defaultStepDefinitions() {
		if _, exists := r.steps[providerID][def.step]; exists {
			continue
		}
		step := VerificationStep{
			ID:              "step-" + string(def.step),
			ProviderID:      providerID,
			Step:            def.step,
			IsOptional:      def.isOptional,
			IsAutoConfirmed: def.isAutoConfirmed,
			ConfirmMethod:   def.confirmMethod,
			Status:          def.status,
			UpdatedAt:       now,
		}
		if def.isAutoConfirmed {
			step.ReviewedAt = &now
		}
		r.steps[providerID][def.step] = step
		result.InsertedSteps++

		if def.isAutoConfirmed && !r.hasAudit(def.step, AuditActionAutoConfirmed) {
			notes := "Auto-confirmed from completed profile onboarding."
			r.auditRows = append(r.auditRows, VerificationAudit{
				ID:         "audit-" + string(def.step),
				ProviderID: providerID,
				Step:       def.step,
				Action:     AuditActionAutoConfirmed,
				FromStatus: StatusPending,
				ToStatus:   StatusApproved,
				Notes:      &notes,
				CreatedAt:  now,
			})
			result.AutoConfirmedAuditRows++
		}
	}
	return result, nil
}

func (r *fakeVerificationRepository) InsertDocument(ctx context.Context, input DocumentInput) error {
	if r.err != nil {
		return r.err
	}
	r.documents = append(r.documents, VerificationDocument{
		ID:           "doc-" + input.DocumentType,
		StepID:       input.StepID,
		ProviderID:   input.ProviderID,
		DocumentType: input.DocumentType,
		FileURL:      input.FileURL,
		FileSize:     input.FileSize,
		MimeType:     input.MimeType,
		UploadedAt:   time.Now().UTC(),
	})
	return nil
}

func (r *fakeVerificationRepository) GetLatestIdentityGovtIDDocument(ctx context.Context, providerID string) (VerificationDocument, bool, error) {
	if r.err != nil {
		return VerificationDocument{}, false, r.err
	}
	for i := len(r.documents) - 1; i >= 0; i-- {
		doc := r.documents[i]
		if doc.ProviderID == providerID && doc.DocumentType == "govt_id" {
			return doc, true, nil
		}
	}
	return VerificationDocument{}, false, nil
}

func (r *fakeVerificationRepository) UpdateStepSubmitted(ctx context.Context, providerID string, step Step) (VerificationStep, error) {
	if r.err != nil {
		return VerificationStep{}, r.err
	}
	now := time.Now().UTC()
	value := r.steps[providerID][step]
	value.Status = StatusSubmitted
	value.SubmittedAt = &now
	value.RejectionReason = nil
	value.UpdatedAt = now
	r.steps[providerID][step] = value
	return value, nil
}

func (r *fakeVerificationRepository) UpdateVehicleStepApproved(ctx context.Context, providerID string, _ string) (VerificationStep, bool, error) {
	if r.err != nil {
		return VerificationStep{}, false, r.err
	}
	value, ok := r.steps[providerID][StepVehicle]
	if !ok {
		return VerificationStep{}, false, nil
	}
	now := time.Now().UTC()
	auto := ConfirmAuto
	value.Status = StatusApproved
	value.ConfirmMethod = &auto
	value.ReviewedAt = &now
	value.RejectionReason = nil
	value.UpdatedAt = now
	r.steps[providerID][StepVehicle] = value
	return value, true, nil
}

func (r *fakeVerificationRepository) UpdateVehicleStepRejected(ctx context.Context, providerID string, reason string) (VerificationStep, bool, error) {
	if r.err != nil {
		return VerificationStep{}, false, r.err
	}
	value, ok := r.steps[providerID][StepVehicle]
	if !ok {
		return VerificationStep{}, false, nil
	}
	now := time.Now().UTC()
	value.Status = StatusRejected
	value.RejectionReason = &reason
	value.ReviewedAt = &now
	value.UpdatedAt = now
	r.steps[providerID][StepVehicle] = value
	return value, true, nil
}

func (r *fakeVerificationRepository) UpdateProviderProfilePhoto(ctx context.Context, providerID string, photoURL string) error {
	if r.err != nil {
		return r.err
	}
	r.profilePhotoURLs[providerID] = photoURL
	return nil
}

func (r *fakeVerificationRepository) UpdateProviderVerificationStatus(ctx context.Context, providerID string, status string) error {
	if r.err != nil {
		return r.err
	}
	state, ok := r.providerStates[providerID]
	if !ok {
		state = ProviderVerificationState{ProviderID: providerID, IsActive: true}
	}
	state.VerificationStatus = status
	r.providerStates[providerID] = state
	return nil
}

func (r *fakeVerificationRepository) CheckRequiredStepsApproved(ctx context.Context, providerID string) (bool, error) {
	if r.err != nil {
		return false, r.err
	}
	for _, step := range requiredVerificationSteps() {
		if r.steps[providerID][step].Status != StatusApproved {
			return false, nil
		}
	}
	return true, nil
}

func (r *fakeVerificationRepository) HasFullyApprovedAudit(ctx context.Context, providerID string) (bool, error) {
	if r.err != nil {
		return false, r.err
	}
	for _, audit := range r.auditRows {
		if audit.ProviderID == providerID && audit.Action == AuditActionFullyApproved {
			return true, nil
		}
	}
	return false, nil
}

func (r *fakeVerificationRepository) InsertAudit(ctx context.Context, input AuditInput) error {
	if r.err != nil {
		return r.err
	}
	r.auditRows = append(r.auditRows, VerificationAudit{
		ID:          "audit-" + string(input.Step) + "-" + string(input.Action),
		ProviderID:  input.ProviderID,
		Step:        input.Step,
		Action:      input.Action,
		FromStatus:  input.FromStatus,
		ToStatus:    input.ToStatus,
		PerformedBy: input.PerformedBy,
		Notes:       input.Notes,
		CreatedAt:   time.Now().UTC(),
	})
	return nil
}

func (r *fakeVerificationRepository) InsertFaceCheck(ctx context.Context, input FaceCheckInput) (FaceCheck, error) {
	if r.err != nil {
		return FaceCheck{}, r.err
	}
	check := FaceCheck{
		ID:           fmt.Sprintf("face-check-%d", len(r.faceChecks)+1),
		ProviderID:   input.ProviderID,
		StepID:       input.StepID,
		SelfieURL:    input.SelfieURL,
		IDDocURL:     input.IDDocURL,
		MatchScore:   input.MatchScore,
		Result:       input.Result,
		ProviderUsed: input.ProviderUsed,
		ErrorMessage: input.ErrorMessage,
	}
	now := time.Now().UTC()
	check.CheckedAt = &now
	r.faceChecks = append(r.faceChecks, check)
	return check, nil
}

func (r *fakeVerificationRepository) UpdateFaceCheckResult(ctx context.Context, input FaceCheckResultInput) (FaceCheck, error) {
	if r.err != nil {
		return FaceCheck{}, r.err
	}
	for i := range r.faceChecks {
		if r.faceChecks[i].ID == input.ID {
			r.faceChecks[i].MatchScore = input.MatchScore
			r.faceChecks[i].Result = input.Result
			r.faceChecks[i].ErrorMessage = input.ErrorMessage
			now := time.Now().UTC()
			r.faceChecks[i].CheckedAt = &now
			return r.faceChecks[i], nil
		}
	}
	return FaceCheck{}, errors.New("face check not found")
}

func (r *fakeVerificationRepository) AdminApproveStep(ctx context.Context, providerID string, step Step, reviewerID string) (VerificationStep, error) {
	if r.err != nil {
		return VerificationStep{}, r.err
	}
	value, ok := r.steps[providerID][step]
	if !ok {
		return VerificationStep{}, fmt.Errorf("step %s not found for provider %s", step, providerID)
	}
	now := time.Now().UTC()
	manual := ConfirmManual
	value.Status = StatusApproved
	value.ReviewedAt = &now
	value.ReviewerID = &reviewerID
	value.ConfirmMethod = &manual
	value.RejectionReason = nil
	value.UpdatedAt = now
	r.steps[providerID][step] = value
	return value, nil
}

func (r *fakeVerificationRepository) AdminRejectStep(ctx context.Context, providerID string, step Step, reviewerID string, reason string) (VerificationStep, error) {
	if r.err != nil {
		return VerificationStep{}, r.err
	}
	value, ok := r.steps[providerID][step]
	if !ok {
		return VerificationStep{}, fmt.Errorf("step %s not found for provider %s", step, providerID)
	}
	now := time.Now().UTC()
	value.Status = StatusRejected
	value.ReviewedAt = &now
	value.ReviewerID = &reviewerID
	value.RejectionReason = &reason
	value.UpdatedAt = now
	r.steps[providerID][step] = value
	return value, nil
}

func (r *fakeVerificationRepository) hasAudit(step Step, action AuditAction) bool {
	for _, audit := range r.auditRows {
		if audit.Step == step && audit.Action == action {
			return true
		}
	}
	return false
}
