package verification

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
)

// GetFinalReviewDossier runs a read-only automated readiness assessment for an
// authorized final reviewer. It NEVER reports face similarity or liveness as
// passed. Machine-readable completeness is not proof of identity.
func (s *Service) GetFinalReviewDossier(ctx context.Context, providerID string) (FinalReviewDossier, error) {
	if err := validateProviderID(providerID); err != nil { return FinalReviewDossier{}, err }
	steps, err := s.repository.ListSteps(ctx, providerID)
	if err != nil { return FinalReviewDossier{}, err }
	if len(steps) == 0 { return FinalReviewDossier{}, apperrors.NotFound("Verification case was not found.", nil) }
	byStep := mapSteps(steps)
	checks := make([]FinalReviewCheck, 0, 12)
	ready := true
	add := func(name string, passed bool, detail string) {
		status := "pass"
		if !passed { status = "needs_action"; ready = false }
		checks = append(checks, FinalReviewCheck{Name: name, Status: status, Detail: detail})
	}
	identity, identityExists := byStep[StepIdentity]
	face, faceExists := byStep[StepFace]
	vehicle, vehicleExists := byStep[StepVehicle]
	guarantor, guarantorExists := byStep[StepGuarantor]
	emergency, emergencyExists := byStep[StepEmergency]
	add("identity_submission", identityExists && (identity.Status == StatusSubmitted || identity.Status == StatusApproved), "Government identity submitted for final review")
	add("selfie_submission", faceExists && face.Status == StatusSubmitted, "Selfie quality prescreen passed at submission")
	add("vehicle_verification", vehicleExists && vehicle.Status == StatusApproved, "Vehicle verification must already be approved")
	add("guarantor_details", guarantorExists && guarantor.Status == StatusApproved, "Required guarantor details completed")
	add("emergency_contact", emergencyExists && emergency.Status == StatusApproved, "Required emergency contact details completed")
	if licence, exists := byStep[StepLicence]; exists {
		// Licence is optional for some operation models; an actively submitted
		// licence is still part of the one-off human final review.
		add("licence_status", licence.Status != StatusRejected &&
			(licence.IsOptional || licence.Status == StatusSubmitted || licence.Status == StatusApproved),
			"Submitted licences are included in the final human review")
	}

	docs, err := s.repository.ListStepDocuments(ctx, providerID, StepIdentity)
	if err != nil { return FinalReviewDossier{}, err }
	governmentID, photo := false, false
	for _, doc := range docs {
		if doc.FileURL == "" { continue }
		switch doc.DocumentType {
		case "govt_id": governmentID = true
		case "profile_photo": photo = true
		}
	}
	add("government_id_document", governmentID, "Government ID file must be stored privately")
	add("profile_photo", photo, "Registration photo is required")

	state, exists, err := s.repository.GetProviderVerificationState(ctx, providerID)
	if err != nil { return FinalReviewDossier{}, err }
	add("account_active", exists && state.IsActive && state.VerificationStatus != string(OverallStatusSuspended),
		"Suspended or inactive rider accounts cannot pass final review")

	var evidence *AdminFaceEvidenceResponse
	found, evidenceErr := s.GetAdminFaceEvidence(ctx, providerID)
	if evidenceErr == nil {
		evidence = &found
	}
	add("selfie_and_identity_linked",
		evidenceErr == nil && found.FaceCheckStatus == "pending_review",
		"Latest selfie must reference the current government ID and await a human decision")
	checks = append(checks,
		FinalReviewCheck{Name: "face_to_id_match", Status: "pending_manual",
			Detail: "Human reviewer must compare the submitted ID portrait and selfie"},
		FinalReviewCheck{Name: "liveness", Status: "pending_manual",
			Detail: "No certified biometric anti-spoof model is active; human review cannot establish strong live presence"},
		FinalReviewCheck{Name: "document_authenticity", Status: "pending_manual",
			Detail: "Image/paper document authenticity must be checked by reviewer until a validated document engine is installed"},
	)
	return FinalReviewDossier{
		ProviderID: providerID, ReadyForReview: ready,
		Checks: checks, Steps: summarizeSteps(steps),
		Evidence: evidence, NoAutomatedBiometrics: true,
	}, nil
}

// FinalReview submits one human decision after all automated prechecks. The
// database owns atomicity; individual step writes may not be used here.
func (s *Service) FinalReview(ctx context.Context, reviewerID, correlationID, providerID string, decision FinalReviewDecision) (FinalReviewOutcome, error) {
	if !s.inHouseManualReview {
		return FinalReviewOutcome{}, apperrors.Conflict("One-time final review is not enabled for this verification backend.", nil)
	}
	if err := validateProviderID(providerID); err != nil { return FinalReviewOutcome{}, err }
	if reviewerID == "" || reviewerID == providerID {
		return FinalReviewOutcome{}, apperrors.Forbidden("Independent platform administrator review is required.", nil)
	}
	if decision.Action != "approve" {
		return FinalReviewOutcome{}, validationError("action", "Use approve after checking the case, or request corrections with the existing review action.")
	}
	if len(strings.TrimSpace(decision.Notes)) < 20 {
		return FinalReviewOutcome{}, validationError("notes", "Record the documents and face comparison you personally reviewed (at least 20 characters).")
	}
	dossier, err := s.GetFinalReviewDossier(ctx, providerID)
	if err != nil { return FinalReviewOutcome{}, err }
	if !dossier.ReadyForReview {
		return FinalReviewOutcome{}, preconditionFailed("Automated checks are incomplete. Review the dossier and request rider corrections.")
	}
	storage, ok := s.repository.(FinalCaseDecisionRepository)
	if !ok {
		return FinalReviewOutcome{}, apperrors.Unavailable("Atomic verification decisions are not configured.", nil)
	}
	steps, err := storage.ApproveInHouseCase(ctx, providerID, reviewerID, strings.TrimSpace(decision.Notes))
	if errors.Is(err, ErrFinalReviewNotReady) {
		return FinalReviewOutcome{}, apperrors.Conflict("Verification evidence changed. Refresh the dossier and review again.", err)
	}
	if err != nil {
		return FinalReviewOutcome{}, apperrors.Internal("Final review could not be committed.", err)
	}
	// Publish after successful commit. An event outage must not cause the
	// client to retry an already committed multi-step approval.
	if s.events != nil {
		for _, step := range steps {
			if err := s.events.PublishVerificationStatusUpdated(ctx, VerificationStatusUpdatedEvent{
				Event: TopicVerificationStatusUpdated, CorrelationID: correlationID,
				ProviderID: providerID, Step: step, Status: StatusApproved,
				CreatedAt: time.Now().UTC(),
			}); err != nil {
				log.Printf("verification: final-review event retry required: %v", err)
			}
		}
		if err := s.events.PublishVerificationFullyApproved(ctx, VerificationFullyApprovedEvent{
			Event: TopicVerificationFullyApproved, CorrelationID: correlationID,
			ProviderID: providerID, ApprovedAt: time.Now().UTC(),
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			log.Printf("verification: final-review fully-approved event retry required: %v", err)
		}
		if err := s.events.PublishVerificationStatusUpdated(ctx, VerificationStatusUpdatedEvent{
			Event: TopicVerificationStatusUpdated, CorrelationID: correlationID,
			ProviderID: providerID, VerificationStatus: string(OverallStatusVerified),
			CreatedAt: time.Now().UTC(),
		}); err != nil {
			log.Printf("verification: final-review profile status event retry required: %v", err)
		}
	}
	return FinalReviewOutcome{
		ProviderID: providerID, Decision: "approved", ApprovedSteps: steps,
		Message: "Final verification approved by an authorized reviewer.",
	}, nil
}
