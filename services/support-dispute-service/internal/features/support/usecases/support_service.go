package supportusecases

import (
	"context"
	"fmt"
	"strings"

	supportclients "cosmicforge/logistics/services/support-dispute-service/internal/features/support/clients"
	supportmodels "cosmicforge/logistics/services/support-dispute-service/internal/features/support/models"
	supportrepositories "cosmicforge/logistics/services/support-dispute-service/internal/features/support/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/walletclient"
)

type SupportService struct {
	repo     supportrepositories.SupportRepository
	notifier *supportclients.SupportNotifier
	identity supportclients.IdentityResolver
	payments *walletclient.Client // nil disables refunds (local dev)
}

// Options configures optional collaborators. Nil/empty fields fall back to safe
// no-ops so the service runs without notification-service / owning services /
// payment-wallet (local dev).
type Options struct {
	Notifier *supportclients.SupportNotifier
	Identity supportclients.IdentityResolver
	Payments *walletclient.Client
}

func NewSupportService(repo supportrepositories.SupportRepository, opts Options) *SupportService {
	if opts.Notifier == nil {
		opts.Notifier = supportclients.NewSupportNotifier("", nil)
	}
	if opts.Identity == nil {
		opts.Identity = supportclients.NoopIdentityResolver{}
	}
	return &SupportService{repo: repo, notifier: opts.Notifier, identity: opts.Identity, payments: opts.Payments}
}

// ─── Inputs ────────────────────────────────────────────────────────────────────

type CreateComplaintInput struct {
	ComplainantType  supportmodels.ComplainantType
	ComplainantID    string
	ServiceType      supportmodels.ServiceType
	BookingReference string
	Category         string
	Subject          string
	Description      string
}

type SOSInput struct {
	ComplainantType supportmodels.ComplainantType
	ComplainantID   string
	ServiceType     supportmodels.ServiceType
	Description     string
	Lat             *float64
	Lng             *float64
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
	ActorID        string
}

type EscalateToDisputeInput struct {
	ComplaintID    string
	RequesterType  supportmodels.ComplainantType
	RequesterID    string
	RespondentType supportmodels.ComplainantType
	RespondentID   string
}

type ResolveDisputeInput struct {
	DisputeID             string
	Outcome               supportmodels.DisputeOutcome
	Note                  string
	AdjudicatorID         string
	RefundAmountKobo      int64
	RefundSourceReference string
}

// ─── Complaint operations ─────────────────────────────────────────────────────

func (s *SupportService) CreateComplaint(ctx context.Context, in CreateComplaintInput) (supportmodels.Complaint, error) {
	if err := validateComplaint(in); err != nil {
		return supportmodels.Complaint{}, err
	}

	name, phone := s.resolveSnapshot(ctx, in.ComplainantType, in.ComplainantID)
	complaint, err := s.repo.CreateComplaint(ctx, supportrepositories.CreateComplaintInput{
		ComplainantType:  in.ComplainantType,
		ComplainantID:    in.ComplainantID,
		ComplainantName:  name,
		ComplainantPhone: phone,
		ServiceType:      in.ServiceType,
		BookingReference: trimToPtr(in.BookingReference),
		Category:         trimToPtr(in.Category),
		Priority:         supportmodels.PriorityNormal,
		Subject:          strings.TrimSpace(in.Subject),
		Description:      strings.TrimSpace(in.Description),
	})
	if err != nil {
		return supportmodels.Complaint{}, err
	}
	s.recordEvent(ctx, complaint.ID, string(in.ComplainantType), in.ComplainantID, "complaint_created", nil)
	return complaint, nil
}

// CreateSOS files a high-priority emergency complaint and acknowledges it to the
// reporter. It surfaces at the top of the admin queue via its priority.
func (s *SupportService) CreateSOS(ctx context.Context, in SOSInput) (supportmodels.Complaint, error) {
	if in.ComplainantID == "" {
		return supportmodels.Complaint{}, apperrors.Validation("Please check your details.", []apperrors.FieldViolation{
			{Field: "complainant_id", Message: "Complainant ID is required."},
		})
	}
	serviceType := in.ServiceType
	if !isValidServiceType(serviceType) {
		serviceType = supportmodels.ServiceTypePlatform
	}
	desc := strings.TrimSpace(in.Description)
	if desc == "" {
		desc = "Emergency assistance requested."
	}
	category := "emergency"

	name, phone := s.resolveSnapshot(ctx, in.ComplainantType, in.ComplainantID)
	complaint, err := s.repo.CreateComplaint(ctx, supportrepositories.CreateComplaintInput{
		ComplainantType:  in.ComplainantType,
		ComplainantID:    in.ComplainantID,
		ComplainantName:  name,
		ComplainantPhone: phone,
		ServiceType:      serviceType,
		Category:         &category,
		Priority:         supportmodels.PriorityEmergency,
		IncidentLat:      in.Lat,
		IncidentLng:      in.Lng,
		Subject:          "Emergency SOS",
		Description:      desc,
	})
	if err != nil {
		return supportmodels.Complaint{}, err
	}
	s.recordEvent(ctx, complaint.ID, string(in.ComplainantType), in.ComplainantID, "sos_raised", map[string]any{"priority": complaint.Priority})
	s.notifier.NotifyEmergencyAck(ctx, complaint)
	return complaint, nil
}

func (s *SupportService) GetComplaint(ctx context.Context, id string, requesterType supportmodels.ComplainantType, requesterID string) (supportmodels.Complaint, error) {
	complaint, err := s.repo.GetComplaintByID(ctx, id)
	if err != nil {
		return supportmodels.Complaint{}, err
	}
	if err := assertOwnership(complaint, requesterType, requesterID); err != nil {
		return supportmodels.Complaint{}, err
	}
	if requesterType != "" {
		// Surface unread count to the owner.
		if n, e := s.repo.CountUnread(ctx, complaint.ID, string(requesterType)); e == nil {
			complaint.UnreadCount = n
		}
	}
	return complaint, nil
}

func (s *SupportService) MyComplaints(ctx context.Context, complainantType supportmodels.ComplainantType, complainantID string, limit, offset int) ([]supportmodels.Complaint, error) {
	limit = clampLimit(limit)
	return s.repo.ListComplaintsByComplainant(ctx, complainantType, complainantID, limit, offset)
}

// ListComplaintsAdmin lists/filter complaints for admins (service-auth).
func (s *SupportService) ListComplaintsAdmin(ctx context.Context, filter supportrepositories.ComplaintFilter, limit, offset int) ([]supportmodels.Complaint, error) {
	limit = clampLimit(limit)
	return s.repo.ListComplaints(ctx, filter, limit, offset)
}

func (s *SupportService) UpdateStatus(ctx context.Context, in UpdateComplaintStatusInput) (supportmodels.Complaint, error) {
	if !isValidComplaintStatus(in.Status) {
		return supportmodels.Complaint{}, apperrors.Validation("Please check your details.", []apperrors.FieldViolation{
			{Field: "status", Message: "Invalid complaint status."},
		})
	}
	complaint, err := s.repo.UpdateComplaintStatus(ctx, in.ComplaintID, in.Status, trimToPtr(in.ResolutionNote))
	if err != nil {
		return supportmodels.Complaint{}, err
	}
	actor := in.ActorID
	if actor == "" {
		actor = "admin"
	}
	s.recordEvent(ctx, complaint.ID, "admin", actor, "status_changed", map[string]any{"status": string(in.Status)})
	s.notifier.NotifyStatusChanged(ctx, complaint)
	return complaint, nil
}

// RefreshIdentity re-pulls the complainant identity snapshot from the owning
// service (admin-driven, for stale/empty snapshots).
func (s *SupportService) RefreshIdentity(ctx context.Context, complaintID string) (supportmodels.Complaint, error) {
	complaint, err := s.repo.GetComplaintByID(ctx, complaintID)
	if err != nil {
		return supportmodels.Complaint{}, err
	}
	name, phone := s.resolveSnapshot(ctx, complaint.ComplainantType, complaint.ComplainantID)
	if name == nil && phone == nil {
		return complaint, nil
	}
	return s.repo.UpdateComplaintIdentity(ctx, complaintID, name, phone)
}

// ─── Evidence ─────────────────────────────────────────────────────────────────

func (s *SupportService) AddEvidence(ctx context.Context, in AddEvidenceInput) (supportmodels.Evidence, error) {
	complaint, err := s.repo.GetComplaintByID(ctx, in.ComplaintID)
	if err != nil {
		return supportmodels.Evidence{}, err
	}
	if err := assertOwnership(complaint, in.UploaderType, in.UploaderID); err != nil {
		return supportmodels.Evidence{}, err
	}
	if complaint.Status == supportmodels.ComplaintStatusResolved || complaint.Status == supportmodels.ComplaintStatusClosed {
		return supportmodels.Evidence{}, apperrors.BadRequest("Evidence cannot be added to a closed complaint.", nil)
	}

	evidence, err := s.repo.AddEvidence(ctx, supportrepositories.AddEvidenceInput{
		ComplaintID:  in.ComplaintID,
		UploaderType: in.UploaderType,
		UploaderID:   in.UploaderID,
		MediaAssetID: trimToPtr(in.MediaAssetID),
		MediaURL:     trimToPtr(in.MediaURL),
		Note:         trimToPtr(in.Note),
	})
	if err != nil {
		return supportmodels.Evidence{}, err
	}
	s.recordEvent(ctx, complaint.ID, string(in.UploaderType), in.UploaderID, "evidence_added", nil)
	return evidence, nil
}

func (s *SupportService) ListEvidence(ctx context.Context, complaintID string, requesterType supportmodels.ComplainantType, requesterID string) ([]supportmodels.Evidence, error) {
	complaint, err := s.repo.GetComplaintByID(ctx, complaintID)
	if err != nil {
		return nil, err
	}
	if err := assertOwnership(complaint, requesterType, requesterID); err != nil {
		return nil, err
	}
	return s.repo.ListEvidence(ctx, complaintID)
}

// ─── Disputes ─────────────────────────────────────────────────────────────────

func (s *SupportService) EscalateToDispute(ctx context.Context, in EscalateToDisputeInput) (supportmodels.Dispute, error) {
	if !isValidComplainantType(in.RespondentType) {
		return supportmodels.Dispute{}, apperrors.Validation("Please check your details.", []apperrors.FieldViolation{
			{Field: "respondent_type", Message: "Respondent type must be one of: customer, taxi_provider, dispatch_provider, hauling_provider."},
		})
	}
	if strings.TrimSpace(in.RespondentID) == "" {
		return supportmodels.Dispute{}, apperrors.Validation("Please check your details.", []apperrors.FieldViolation{
			{Field: "respondent_id", Message: "Respondent ID is required."},
		})
	}

	complaint, err := s.repo.GetComplaintByID(ctx, in.ComplaintID)
	if err != nil {
		return supportmodels.Dispute{}, err
	}
	if err := assertOwnership(complaint, in.RequesterType, in.RequesterID); err != nil {
		return supportmodels.Dispute{}, err
	}
	if complaint.Status == supportmodels.ComplaintStatusResolved || complaint.Status == supportmodels.ComplaintStatusClosed {
		return supportmodels.Dispute{}, apperrors.BadRequest("Cannot escalate a resolved or closed complaint.", nil)
	}

	if _, err := s.repo.UpdateComplaintStatus(ctx, complaint.ID, supportmodels.ComplaintStatusEscalated, nil); err != nil {
		return supportmodels.Dispute{}, err
	}

	name, phone := s.resolveSnapshot(ctx, in.RespondentType, in.RespondentID)
	dispute, err := s.repo.CreateDispute(ctx, supportrepositories.CreateDisputeInput{
		ComplaintID:      complaint.ID,
		ServiceType:      complaint.ServiceType,
		BookingReference: complaint.BookingReference,
		RespondentType:   in.RespondentType,
		RespondentID:     in.RespondentID,
		RespondentName:   name,
		RespondentPhone:  phone,
	})
	if err != nil {
		return supportmodels.Dispute{}, err
	}
	s.recordEvent(ctx, complaint.ID, string(in.RequesterType), in.RequesterID, "escalated", map[string]any{"dispute_id": dispute.ID})
	return dispute, nil
}

func (s *SupportService) GetDispute(ctx context.Context, complaintID string, requesterType supportmodels.ComplainantType, requesterID string) (supportmodels.Dispute, error) {
	complaint, err := s.repo.GetComplaintByID(ctx, complaintID)
	if err != nil {
		return supportmodels.Dispute{}, err
	}
	if err := assertOwnership(complaint, requesterType, requesterID); err != nil {
		return supportmodels.Dispute{}, err
	}
	return s.repo.GetDisputeByComplaintID(ctx, complaintID)
}

func (s *SupportService) ListDisputesAdmin(ctx context.Context, filter supportrepositories.DisputeFilter, limit, offset int) ([]supportmodels.Dispute, error) {
	limit = clampLimit(limit)
	return s.repo.ListDisputes(ctx, filter, limit, offset)
}

func (s *SupportService) ResolveDispute(ctx context.Context, in ResolveDisputeInput) (supportmodels.Dispute, error) {
	if !isValidDisputeOutcome(in.Outcome) {
		return supportmodels.Dispute{}, apperrors.Validation("Please check your details.", []apperrors.FieldViolation{
			{Field: "outcome", Message: "Outcome must be one of: pending, favour_complainant, favour_respondent, split, dismissed."},
		})
	}
	dispute, err := s.repo.ResolveDispute(ctx, in.DisputeID, in.Outcome, in.Note, in.AdjudicatorID)
	if err != nil {
		return supportmodels.Dispute{}, err
	}

	// Best-effort refund/compensation when the outcome favours the complainant
	// and the admin supplied the payment reference + amount. Never blocks the
	// resolution write.
	s.maybeRefund(ctx, in, dispute)

	if complaint, e := s.repo.GetComplaintByID(ctx, dispute.ComplaintID); e == nil {
		s.recordEvent(ctx, complaint.ID, "admin", in.AdjudicatorID, "dispute_resolved", map[string]any{"outcome": string(in.Outcome)})
		s.notifier.NotifyDisputeResolved(ctx, complaint, dispute)
	}
	return dispute, nil
}

func (s *SupportService) maybeRefund(ctx context.Context, in ResolveDisputeInput, dispute supportmodels.Dispute) {
	favoursComplainant := in.Outcome == supportmodels.DisputeOutcomeFavourComplainant || in.Outcome == supportmodels.DisputeOutcomeSplit
	if s.payments == nil || !favoursComplainant {
		return
	}
	if in.RefundAmountKobo <= 0 || strings.TrimSpace(in.RefundSourceReference) == "" {
		return
	}
	_, err := s.payments.RequestRefund(ctx, walletclient.RefundRequest{
		PaymentReference: strings.TrimSpace(in.RefundSourceReference),
		AmountKobo:       in.RefundAmountKobo,
		Currency:         "NGN",
		Reason:           "dispute_resolution",
		IdempotencyKey:   "dispute-refund-" + dispute.ID,
	})
	if err != nil {
		logging.Error("refund", "dispute=%s ref=%s: %v", dispute.ID, in.RefundSourceReference, err)
	}
}

// ─── Chat ─────────────────────────────────────────────────────────────────────

func (s *SupportService) StartSupportChat(ctx context.Context, complainantType supportmodels.ComplainantType, complainantID string) (supportmodels.Complaint, error) {
	complaint, err := s.repo.FindActiveSupportChat(ctx, complainantType, complainantID)
	if err == nil {
		return complaint, nil
	}
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

	if senderType != supportmodels.SenderTypeAdmin {
		expectedType := supportmodels.ComplainantType(senderType)
		if complaint.ComplainantType != expectedType || complaint.ComplainantID != senderID {
			return supportmodels.ChatMessage{}, apperrors.Forbidden("You do not have access to this complaint.", nil)
		}
	}

	msg, err := s.repo.CreateChatMessage(ctx, supportmodels.ChatMessage{
		ComplaintID: complaintID,
		SenderType:  senderType,
		SenderID:    senderID,
		Content:     content,
	})
	if err != nil {
		return supportmodels.ChatMessage{}, err
	}
	// Notify the complainant when someone else (typically admin) replies.
	s.notifier.NotifyNewMessage(ctx, complaint, msg)
	return msg, nil
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

	return s.repo.ListChatMessages(ctx, complaintID, clampLimit(limit), offset)
}

// MarkMessagesRead marks the counterpart's messages as read for the requester.
func (s *SupportService) MarkMessagesRead(ctx context.Context, complaintID string, requestorType supportmodels.SenderType, requestorID string) error {
	complaint, err := s.repo.GetComplaintByID(ctx, complaintID)
	if err != nil {
		return err
	}
	if requestorType != supportmodels.SenderTypeAdmin {
		expectedType := supportmodels.ComplainantType(requestorType)
		if complaint.ComplainantType != expectedType || complaint.ComplainantID != requestorID {
			return apperrors.Forbidden("You do not have access to this complaint.", nil)
		}
	}
	return s.repo.MarkRead(ctx, complaintID, string(requestorType))
}

// ─── Events / Help ─────────────────────────────────────────────────────────────

func (s *SupportService) ListEvents(ctx context.Context, complaintID string) ([]supportmodels.ComplaintEvent, error) {
	return s.repo.ListEvents(ctx, complaintID)
}

func (s *SupportService) ListHelpArticles(ctx context.Context, audience string) ([]supportmodels.HelpArticle, error) {
	if audience == "" {
		audience = "all"
	}
	return s.repo.ListHelpArticles(ctx, audience)
}

// Categories returns the supported complaint category codes.
func (s *SupportService) Categories() []string {
	return []string{
		"incorrect_delivery", "delayed_arrival", "payment",
		"damaged_goods", "provider_misconduct", "fraud", "other",
	}
}

// ─── Internal helpers ──────────────────────────────────────────────────────────

func (s *SupportService) resolveSnapshot(ctx context.Context, ctype supportmodels.ComplainantType, id string) (*string, *string) {
	identity, err := s.identity.Resolve(ctx, ctype, id)
	if err != nil {
		logging.Error("identity", "resolve type=%s id=%s: %v", ctype, id, err)
		return nil, nil
	}
	if !identity.Found {
		return nil, nil
	}
	return trimToPtr(identity.Name), trimToPtr(identity.Phone)
}

func (s *SupportService) recordEvent(ctx context.Context, complaintID, actorType, actorID, eventType string, payload map[string]any) {
	if err := s.repo.RecordEvent(ctx, complaintID, actorType, actorID, eventType, payload); err != nil {
		logging.Error("event", "record %s for complaint=%s: %v", eventType, complaintID, err)
	}
}

func assertOwnership(complaint supportmodels.Complaint, requesterType supportmodels.ComplainantType, requesterID string) error {
	// Empty requesterType = admin/service caller → bypass ownership.
	if requesterType == "" {
		return nil
	}
	if complaint.ComplainantType != requesterType || complaint.ComplainantID != requesterID {
		return apperrors.Forbidden("You do not have access to this complaint.", nil)
	}
	return nil
}

func clampLimit(limit int) int {
	if limit <= 0 || limit > 100 {
		return 50
	}
	return limit
}

func trimToPtr(v string) *string {
	if t := strings.TrimSpace(v); t != "" {
		return &t
	}
	return nil
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
		fields = append(fields, apperrors.FieldViolation{Field: "service_type", Message: "Service type must be one of: taxi, dispatch, hauling, wallet, platform."})
	}
	if c := strings.TrimSpace(in.Category); c != "" && !supportmodels.ValidCategories[c] {
		fields = append(fields, apperrors.FieldViolation{Field: "category", Message: fmt.Sprintf("Unsupported category %q.", c)})
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
		supportmodels.ServiceTypeWallet,
		supportmodels.ServiceTypePlatform:
		return true
	}
	return false
}

func isValidComplainantType(t supportmodels.ComplainantType) bool {
	switch t {
	case supportmodels.ComplainantCustomer,
		supportmodels.ComplainantTaxiProvider,
		supportmodels.ComplainantDispatchProvider,
		supportmodels.ComplainantHaulingProvider:
		return true
	}
	return false
}

func isValidComplaintStatus(s supportmodels.ComplaintStatus) bool {
	switch s {
	case supportmodels.ComplaintStatusOpen,
		supportmodels.ComplaintStatusUnderReview,
		supportmodels.ComplaintStatusAwaitingEvidence,
		supportmodels.ComplaintStatusResolved,
		supportmodels.ComplaintStatusClosed,
		supportmodels.ComplaintStatusEscalated:
		return true
	}
	return false
}

func isValidDisputeOutcome(o supportmodels.DisputeOutcome) bool {
	switch o {
	case supportmodels.DisputeOutcomePending,
		supportmodels.DisputeOutcomeFavourComplainant,
		supportmodels.DisputeOutcomeFavourRespondent,
		supportmodels.DisputeOutcomeSplit,
		supportmodels.DisputeOutcomeDismissed:
		return true
	}
	return false
}
