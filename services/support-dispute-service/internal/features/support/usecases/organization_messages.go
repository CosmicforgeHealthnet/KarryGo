package supportusecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	supportmodels "cosmicforge/logistics/services/support-dispute-service/internal/features/support/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Organization message limits.
const (
	maxOrganizationMessageRunes = 4000
	maxOrganizationMessageFiles = 10
)

// OrganizationMessageInput is a corporate staff member's message on their
// organization's platform support case (Fleet spec 59.10).
type OrganizationMessageInput struct {
	OrganizationReference string
	CaseID                string
	ClientMessageID       string
	SenderReference       string
	Text                  string
	FileIDs               []string
	Visibility            string
}

// OrganizationMessageRequestHash fingerprints the parts of a message that a
// reused client message id must repeat exactly.
func OrganizationMessageRequestHash(sender, text, visibility string, fileIDs []string) string {
	body, _ := json.Marshal(map[string]any{"sender": sender, "text": text, "visibility": visibility, "file_ids": fileIDs})
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func normalizeOrganizationMessage(in OrganizationMessageInput) (OrganizationMessageInput, error) {
	in.OrganizationReference = strings.TrimSpace(in.OrganizationReference)
	in.CaseID = strings.TrimSpace(in.CaseID)
	in.SenderReference = strings.TrimSpace(in.SenderReference)
	in.Text = strings.TrimSpace(in.Text)
	in.Visibility = strings.ToUpper(strings.TrimSpace(in.Visibility))
	if in.Visibility == "" {
		in.Visibility = supportmodels.VisibilityShared
	}
	var fields []apperrors.FieldViolation
	if in.OrganizationReference == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "organization_reference", Message: "Organization reference is required."})
	}
	if in.SenderReference == "" {
		fields = append(fields, apperrors.FieldViolation{Field: "sender_reference", Message: "Sender reference is required."})
	}
	if id, err := uuid.Parse(strings.TrimSpace(in.ClientMessageID)); err != nil {
		fields = append(fields, apperrors.FieldViolation{Field: "client_message_id", Message: "Client message id must be a UUID."})
	} else {
		in.ClientMessageID = id.String()
	}
	if in.Text == "" || len([]rune(in.Text)) > maxOrganizationMessageRunes {
		fields = append(fields, apperrors.FieldViolation{Field: "text", Message: "Text is required and at most 4000 characters."})
	}
	if in.Visibility != supportmodels.VisibilityShared && in.Visibility != supportmodels.VisibilityInternal {
		fields = append(fields, apperrors.FieldViolation{Field: "visibility", Message: "Visibility must be SHARED or INTERNAL."})
	}
	seen := map[string]bool{}
	files := make([]string, 0, len(in.FileIDs))
	for _, f := range in.FileIDs {
		f = strings.TrimSpace(f)
		if f == "" || seen[f] {
			fields = append(fields, apperrors.FieldViolation{Field: "file_ids", Message: "File ids must be non-empty and unique."})
			break
		}
		seen[f] = true
		files = append(files, f)
	}
	if len(files) > maxOrganizationMessageFiles {
		fields = append(fields, apperrors.FieldViolation{Field: "file_ids", Message: "At most 10 files may be attached."})
	}
	in.FileIDs = files
	if len(fields) > 0 {
		return in, apperrors.Validation("Check the message.", fields)
	}
	return in, nil
}

// CreateOrganizationMessage stores a message on an organization's own case
// exactly once per client message id. Repeating the same client id with the
// same body returns the stored message; a different body is a conflict, so a
// retry can never create a second copy or silently change the first.
// File ids are references the calling service validated with media-file-
// service for this organization; they are stored, never fetched here.
func (s *SupportService) CreateOrganizationMessage(ctx context.Context, in OrganizationMessageInput) (supportmodels.ChatMessage, bool, error) {
	in, err := normalizeOrganizationMessage(in)
	if err != nil {
		return supportmodels.ChatMessage{}, false, err
	}
	complaint, err := s.repo.GetOrganizationComplaintByID(ctx, in.OrganizationReference, in.CaseID)
	if err != nil {
		return supportmodels.ChatMessage{}, false, err
	}
	hash := OrganizationMessageRequestHash(in.SenderReference, in.Text, in.Visibility, in.FileIDs)
	if existing, err := s.repo.GetChatMessageByClientID(ctx, complaint.ID, in.ClientMessageID); err == nil {
		return organizationReplay(existing, hash)
	}
	if complaint.Status == supportmodels.ComplaintStatusClosed {
		return supportmodels.ChatMessage{}, false, apperrors.Conflict("This support case is closed.", nil)
	}
	clientID := in.ClientMessageID
	stored, created, err := s.repo.CreateOrganizationMessage(ctx, supportmodels.ChatMessage{ComplaintID: complaint.ID,
		SenderType: supportmodels.SenderTypeOrganizationUser, SenderID: in.SenderReference, Content: in.Text,
		Visibility: in.Visibility, FileIDs: in.FileIDs, ClientMessageID: &clientID, RequestHash: &hash})
	if err != nil {
		return supportmodels.ChatMessage{}, false, err
	}
	if !created {
		return organizationReplay(stored, hash)
	}
	s.recordEvent(ctx, complaint.ID, "service", "fleet-partner-service", "organization_message_created",
		map[string]any{"organization_reference": in.OrganizationReference, "message_id": stored.ID, "client_message_id": clientID,
			"visibility": in.Visibility, "file_count": len(in.FileIDs)})
	return stored, true, nil
}

func organizationReplay(existing supportmodels.ChatMessage, hash string) (supportmodels.ChatMessage, bool, error) {
	if existing.RequestHash == nil || *existing.RequestHash != hash {
		return supportmodels.ChatMessage{}, false, apperrors.Conflict("This client message id was already used for a different message.", nil)
	}
	return existing, false, nil
}

// GetOrganizationMessage reads a message back by its client id, scoped to the
// organization's own case: the authoritative answer to "did my submission
// land?" after an ambiguous outcome.
func (s *SupportService) GetOrganizationMessage(ctx context.Context, organizationReference, caseID, clientMessageID string) (supportmodels.ChatMessage, error) {
	if strings.TrimSpace(organizationReference) == "" {
		return supportmodels.ChatMessage{}, apperrors.Validation("Organization reference is required.", nil)
	}
	id, err := uuid.Parse(strings.TrimSpace(clientMessageID))
	if err != nil {
		return supportmodels.ChatMessage{}, apperrors.Validation("Client message id must be a UUID.", nil)
	}
	complaint, err := s.repo.GetOrganizationComplaintByID(ctx, organizationReference, caseID)
	if err != nil {
		return supportmodels.ChatMessage{}, err
	}
	return s.repo.GetChatMessageByClientID(ctx, complaint.ID, id.String())
}
