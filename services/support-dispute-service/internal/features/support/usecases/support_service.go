package supportusecases

import (
	"context"
	"strings"

	supportmodels "cosmicforge/logistics/services/support-dispute-service/internal/features/support/models"
	supportrepositories "cosmicforge/logistics/services/support-dispute-service/internal/features/support/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
)

type SupportService struct {
	repo supportrepositories.SupportRepository
}

func NewSupportService(repo supportrepositories.SupportRepository) *SupportService {
	return &SupportService{repo: repo}
}

// ─── Complaint inputs ──────────────────────────────────────────────────────────

type CreateComplaintInput struct {
	ComplainantType  supportmodels.ComplainantType
	ComplainantID    string
	ServiceType      supportmodels.ServiceType
	BookingReference string
	Subject          string
	Description      string
}

type AddEvidenceInput struct {
	ComplaintID  string
	UploaderType supportmodels.ComplainantType
	UploaderID   string
	MediaAssetID string
	MediaURL     string
	Note         string
}

type UpdateComplaintStatusInput struct {
	ComplaintID    string
	Status         supportmodels.ComplaintStatus
	ResolutionNote string
}

type EscalateToDisputeInput struct {
	ComplaintID      string
	RespondentType   supportmodels.ComplainantType
	RespondentID     string
}

type ResolveDisputeInput struct {
	DisputeID     string
	Outcome       supportmodels.DisputeOutcome
	Note          string
	AdjudicatorID string
}

// ─── Complaint operations ─────────────────────────────────────────────────────

func (s *SupportService) CreateComplaint(ctx context.Context, in CreateComplaintInput) (supportmodels.Complaint, error) {
	if err := validateComplaint(in); err != nil {
		return supportmodels.Complaint{}, err
	}

	var bookingRef *string
	if strings.TrimSpace(in.BookingReference) != "" {
		ref := strings.TrimSpace(in.BookingReference)
		bookingRef = &ref
	}

	return s.repo.CreateComplaint(ctx, supportrepositories.CreateComplaintInput{
		ComplainantType:  in.ComplainantType,
		ComplainantID:    in.ComplainantID,
		ServiceType:      in.ServiceType,
		BookingReference: bookingRef,
		Subject:          strings.TrimSpace(in.Subject),
		Description:      strings.TrimSpace(in.Description),
	})
}

func (s *SupportService) GetComplaint(ctx context.Context, id string, requesterType supportmodels.ComplainantType, requesterID string) (supportmodels.Complaint, error) {
	complaint, err := s.repo.GetComplaintByID(ctx, id)
	if err != nil {
		return supportmodels.Complaint{}, err
	}
	// Non-admin callers may only see their own complaints.
	if requesterType != "" && (complaint.ComplainantType != requesterType || complaint.ComplainantID != requesterID) {
		return supportmodels.Complaint{}, apperrors.Forbidden("You do not have access to this complaint.", nil)
	}
	return complaint, nil
}

func (s *SupportService) MyComplaints(ctx context.Context, complainantType supportmodels.ComplainantType, complainantID string, limit, offset int) ([]supportmodels.Complaint, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.repo.ListComplaintsByComplainant(ctx, complainantType, complainantID, limit, offset)
}

func (s *SupportService) ListComplaintsByServiceType(ctx context.Context, serviceType supportmodels.ServiceType, limit, offset int) ([]supportmodels.Complaint, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.repo.ListComplaintsByServiceType(ctx, serviceType, limit, offset)
}

func (s *SupportService) UpdateStatus(ctx context.Context, in UpdateComplaintStatusInput) (supportmodels.Complaint, error) {
	var note *string
	if strings.TrimSpace(in.ResolutionNote) != "" {
		n := strings.TrimSpace(in.ResolutionNote)
		note = &n
	}
	return s.repo.UpdateComplaintStatus(ctx, in.ComplaintID, in.Status, note)
}

// ─── Evidence ─────────────────────────────────────────────────────────────────

func (s *SupportService) AddEvidence(ctx context.Context, in AddEvidenceInput) (supportmodels.Evidence, error) {
	// Ensure complaint exists and belongs to the uploader.
	complaint, err := s.repo.GetComplaintByID(ctx, in.ComplaintID)
	if err != nil {
		return supportmodels.Evidence{}, err
	}
	if complaint.Status == supportmodels.ComplaintStatusResolved || complaint.Status == supportmodels.ComplaintStatusClosed {
		return supportmodels.Evidence{}, apperrors.BadRequest("Evidence cannot be added to a closed complaint.", nil)
	}

	var assetID, mediaURL, note *string
	if v := strings.TrimSpace(in.MediaAssetID); v != "" {
		assetID = &v
	}
	if v := strings.TrimSpace(in.MediaURL); v != "" {
		mediaURL = &v
	}
	if v := strings.TrimSpace(in.Note); v != "" {
		note = &v
	}

	return s.repo.AddEvidence(ctx, supportrepositories.AddEvidenceInput{
		ComplaintID:  in.ComplaintID,
		UploaderType: in.UploaderType,
		UploaderID:   in.UploaderID,
		MediaAssetID: assetID,
		MediaURL:     mediaURL,
		Note:         note,
	})
}

func (s *SupportService) ListEvidence(ctx context.Context, complaintID string) ([]supportmodels.Evidence, error) {
	return s.repo.ListEvidence(ctx, complaintID)
}

// ─── Disputes ─────────────────────────────────────────────────────────────────

func (s *SupportService) EscalateToDispute(ctx context.Context, in EscalateToDisputeInput) (supportmodels.Dispute, error) {
	complaint, err := s.repo.GetComplaintByID(ctx, in.ComplaintID)
	if err != nil {
		return supportmodels.Dispute{}, err
	}
	if complaint.Status == supportmodels.ComplaintStatusResolved || complaint.Status == supportmodels.ComplaintStatusClosed {
		return supportmodels.Dispute{}, apperrors.BadRequest("Cannot escalate a resolved or closed complaint.", nil)
	}

	// Mark complaint as escalated.
	if _, err := s.repo.UpdateComplaintStatus(ctx, complaint.ID, supportmodels.ComplaintStatusEscalated, nil); err != nil {
		return supportmodels.Dispute{}, err
	}

	return s.repo.CreateDispute(ctx, supportrepositories.CreateDisputeInput{
		ComplaintID:      complaint.ID,
		ServiceType:      complaint.ServiceType,
		BookingReference: complaint.BookingReference,
		RespondentType:   in.RespondentType,
		RespondentID:     in.RespondentID,
	})
}

func (s *SupportService) GetDispute(ctx context.Context, complaintID string) (supportmodels.Dispute, error) {
	return s.repo.GetDisputeByComplaintID(ctx, complaintID)
}

func (s *SupportService) ResolveDispute(ctx context.Context, in ResolveDisputeInput) (supportmodels.Dispute, error) {
	return s.repo.ResolveDispute(ctx, in.DisputeID, in.Outcome, in.Note, in.AdjudicatorID)
}

// ─── Chat ─────────────────────────────────────────────────────────────────────

func (s *SupportService) StartSupportChat(ctx context.Context, complainantType supportmodels.ComplainantType, complainantID string) (supportmodels.Complaint, error) {
	complaint, err := s.repo.FindActiveSupportChat(ctx, complainantType, complainantID)
	if err == nil {
		return complaint, nil
	}
	// No active support chat — create one.
	return s.CreateComplaint(ctx, CreateComplaintInput{
		ComplainantType: complainantType,
		ComplainantID:   complainantID,
		ServiceType:     supportmodels.ServiceTypePlatform,
		Subject:         "Support Chat",
		Description:     "Support conversation initiated.",
	})
}

func (s *SupportService) SendChatMessage(ctx context.Context, complaintID string, senderType supportmodels.SenderType, senderID, content string) (supportmodels.ChatMessage, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return supportmodels.ChatMessage{}, apperrors.BadRequest("Message content is required.", nil)
	}

	complaint, err := s.repo.GetComplaintByID(ctx, complaintID)
	if err != nil {
		return supportmodels.ChatMessage{}, err
	}

	// Ensure non-admin callers only message their own complaints.
	if senderType != supportmodels.SenderTypeAdmin {
		expectedType := supportmodels.ComplainantType(senderType)
		if complaint.ComplainantType != expectedType || complaint.ComplainantID != senderID {
			return supportmodels.ChatMessage{}, apperrors.Forbidden("You do not have access to this complaint.", nil)
		}
	}

	return s.repo.CreateChatMessage(ctx, supportmodels.ChatMessage{
		ComplaintID: complaintID,
		SenderType:  senderType,
		SenderID:    senderID,
		Content:     content,
	})
}

func (s *SupportService) ListChatMessages(ctx context.Context, complaintID string, requestorType supportmodels.SenderType, requestorID string, limit, offset int) ([]supportmodels.ChatMessage, error) {
	complaint, err := s.repo.GetComplaintByID(ctx, complaintID)
	if err != nil {
		return nil, err
	}

	if requestorType != supportmodels.SenderTypeAdmin {
		expectedType := supportmodels.ComplainantType(requestorType)
		if complaint.ComplainantType != expectedType || complaint.ComplainantID != requestorID {
			return nil, apperrors.Forbidden("You do not have access to this complaint.", nil)
		}
	}

	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.repo.ListChatMessages(ctx, complaintID, limit, offset)
}

// ─── Validation ───────────────────────────────────────────────────────────────

func validateComplaint(in CreateComplaintInput) *apperrors.Error {
	var fields []apperrors.FieldViolation
	if strings.TrimSpace(in.Subject) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "subject", Message: "Subject is required."})
	}
	if len(strings.TrimSpace(in.Subject)) > 200 {
		fields = append(fields, apperrors.FieldViolation{Field: "subject", Message: "Subject must be 200 characters or fewer."})
	}
	if strings.TrimSpace(in.Description) == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "description", Message: "Description is required."})
	}
	if in.ComplainantID == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "complainant_id", Message: "Complainant ID is required."})
	}
	if !isValidServiceType(in.ServiceType) {
		fields = append(fields, apperrors.FieldViolation{Field: "service_type", Message: "Service type must be one of: taxi, dispatch, hauling, platform."})
	}
	if len(fields) > 0 {
		return apperrors.Validation("Please check your details.", fields)
	}
	return nil
}

func isValidServiceType(s supportmodels.ServiceType) bool {
	switch s {
	case supportmodels.ServiceTypeTaxi,
		supportmodels.ServiceTypeDispatch,
		supportmodels.ServiceTypeHauling,
		supportmodels.ServiceTypePlatform:
		return true
	}
	return false
}
