package supportusecases

import (
	"context"
	"fmt"
	"testing"

	supportclients "cosmicforge/logistics/services/support-dispute-service/internal/features/support/clients"
	supportmodels "cosmicforge/logistics/services/support-dispute-service/internal/features/support/models"
	supportrepositories "cosmicforge/logistics/services/support-dispute-service/internal/features/support/repositories"
	"cosmicforge/logistics/shared/go/apperrors"
)

// ─── In-memory repository ──────────────────────────────────────────────────────

type memRepo struct {
	seq        int
	complaints map[string]supportmodels.Complaint
	evidence   map[string][]supportmodels.Evidence
	disputes   map[string]supportmodels.Dispute // keyed by dispute ID
	messages   map[string][]supportmodels.ChatMessage
	events     map[string][]supportmodels.ComplaintEvent
}

func newMemRepo() *memRepo {
	return &memRepo{
		complaints: map[string]supportmodels.Complaint{},
		evidence:   map[string][]supportmodels.Evidence{},
		disputes:   map[string]supportmodels.Dispute{},
		messages:   map[string][]supportmodels.ChatMessage{},
		events:     map[string][]supportmodels.ComplaintEvent{},
	}
}

func (m *memRepo) nextID() string { m.seq++; return fmt.Sprintf("id-%d", m.seq) }

func (m *memRepo) CreateComplaint(_ context.Context, in supportrepositories.CreateComplaintInput) (supportmodels.Complaint, error) {
	c := supportmodels.Complaint{
		ID:               m.nextID(),
		ComplainantType:  in.ComplainantType,
		ComplainantID:    in.ComplainantID,
		ComplainantName:  in.ComplainantName,
		ComplainantPhone: in.ComplainantPhone,
		ServiceType:      in.ServiceType,
		BookingReference: in.BookingReference,
		Category:         in.Category,
		Priority:         in.Priority,
		IncidentLat:      in.IncidentLat,
		IncidentLng:      in.IncidentLng,
		Subject:          in.Subject,
		Description:      in.Description,
		Status:           supportmodels.ComplaintStatusOpen,
	}
	if c.Priority == "" {
		c.Priority = supportmodels.PriorityNormal
	}
	m.complaints[c.ID] = c
	return c, nil
}

func (m *memRepo) GetComplaintByID(_ context.Context, id string) (supportmodels.Complaint, error) {
	c, ok := m.complaints[id]
	if !ok {
		return supportmodels.Complaint{}, apperrors.NotFound("Complaint not found.", nil)
	}
	return c, nil
}

func (m *memRepo) ListComplaintsByComplainant(_ context.Context, t supportmodels.ComplainantType, id string, _, _ int) ([]supportmodels.Complaint, error) {
	var out []supportmodels.Complaint
	for _, c := range m.complaints {
		if c.ComplainantType == t && c.ComplainantID == id {
			out = append(out, c)
		}
	}
	return out, nil
}

func (m *memRepo) ListComplaints(_ context.Context, f supportrepositories.ComplaintFilter, _, _ int) ([]supportmodels.Complaint, error) {
	var out []supportmodels.Complaint
	for _, c := range m.complaints {
		if f.Priority != "" && c.Priority != f.Priority {
			continue
		}
		if f.Status != "" && c.Status != f.Status {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

func (m *memRepo) UpdateComplaintStatus(_ context.Context, id string, status supportmodels.ComplaintStatus, note *string) (supportmodels.Complaint, error) {
	c, ok := m.complaints[id]
	if !ok {
		return supportmodels.Complaint{}, apperrors.NotFound("Complaint not found.", nil)
	}
	c.Status = status
	if note != nil {
		c.ResolutionNote = note
	}
	m.complaints[id] = c
	return c, nil
}

func (m *memRepo) UpdateComplaintIdentity(_ context.Context, id string, name, phone *string) (supportmodels.Complaint, error) {
	c, ok := m.complaints[id]
	if !ok {
		return supportmodels.Complaint{}, apperrors.NotFound("Complaint not found.", nil)
	}
	if name != nil {
		c.ComplainantName = name
	}
	if phone != nil {
		c.ComplainantPhone = phone
	}
	m.complaints[id] = c
	return c, nil
}

func (m *memRepo) AddEvidence(_ context.Context, in supportrepositories.AddEvidenceInput) (supportmodels.Evidence, error) {
	e := supportmodels.Evidence{ID: m.nextID(), ComplaintID: in.ComplaintID, UploaderType: in.UploaderType, UploaderID: in.UploaderID, MediaURL: in.MediaURL}
	m.evidence[in.ComplaintID] = append(m.evidence[in.ComplaintID], e)
	return e, nil
}

func (m *memRepo) ListEvidence(_ context.Context, complaintID string) ([]supportmodels.Evidence, error) {
	return m.evidence[complaintID], nil
}

func (m *memRepo) CreateDispute(_ context.Context, in supportrepositories.CreateDisputeInput) (supportmodels.Dispute, error) {
	d := supportmodels.Dispute{
		ID: m.nextID(), ComplaintID: in.ComplaintID, ServiceType: in.ServiceType,
		RespondentType: in.RespondentType, RespondentID: in.RespondentID,
		RespondentName: in.RespondentName, RespondentPhone: in.RespondentPhone,
		Outcome: supportmodels.DisputeOutcomePending,
	}
	m.disputes[d.ID] = d
	return d, nil
}

func (m *memRepo) GetDisputeByComplaintID(_ context.Context, complaintID string) (supportmodels.Dispute, error) {
	for _, d := range m.disputes {
		if d.ComplaintID == complaintID {
			return d, nil
		}
	}
	return supportmodels.Dispute{}, apperrors.NotFound("Dispute not found.", nil)
}

func (m *memRepo) GetDisputeByID(_ context.Context, id string) (supportmodels.Dispute, error) {
	d, ok := m.disputes[id]
	if !ok {
		return supportmodels.Dispute{}, apperrors.NotFound("Dispute not found.", nil)
	}
	return d, nil
}

func (m *memRepo) ListDisputes(_ context.Context, _ supportrepositories.DisputeFilter, _, _ int) ([]supportmodels.Dispute, error) {
	var out []supportmodels.Dispute
	for _, d := range m.disputes {
		out = append(out, d)
	}
	return out, nil
}

func (m *memRepo) ResolveDispute(_ context.Context, id string, outcome supportmodels.DisputeOutcome, note, adjudicatorID string) (supportmodels.Dispute, error) {
	d, ok := m.disputes[id]
	if !ok {
		return supportmodels.Dispute{}, apperrors.NotFound("Dispute not found.", nil)
	}
	d.Outcome = outcome
	m.disputes[id] = d
	return d, nil
}

func (m *memRepo) FindActiveSupportChat(_ context.Context, _ supportmodels.ComplainantType, _ string) (supportmodels.Complaint, error) {
	return supportmodels.Complaint{}, apperrors.NotFound("No active support chat found.", nil)
}

func (m *memRepo) CreateChatMessage(_ context.Context, msg supportmodels.ChatMessage) (supportmodels.ChatMessage, error) {
	msg.ID = m.nextID()
	m.messages[msg.ComplaintID] = append(m.messages[msg.ComplaintID], msg)
	return msg, nil
}

func (m *memRepo) ListChatMessages(_ context.Context, complaintID string, _, _ int) ([]supportmodels.ChatMessage, error) {
	return m.messages[complaintID], nil
}

func (m *memRepo) CountUnread(_ context.Context, complaintID, readerSenderType string) (int, error) {
	n := 0
	for _, msg := range m.messages[complaintID] {
		if string(msg.SenderType) != readerSenderType && !msg.IsRead {
			n++
		}
	}
	return n, nil
}

func (m *memRepo) MarkRead(_ context.Context, complaintID, readerSenderType string) error {
	msgs := m.messages[complaintID]
	for i := range msgs {
		if string(msgs[i].SenderType) != readerSenderType {
			msgs[i].IsRead = true
		}
	}
	return nil
}

func (m *memRepo) RecordEvent(_ context.Context, complaintID, actorType, actorID, eventType string, payload map[string]any) error {
	m.events[complaintID] = append(m.events[complaintID], supportmodels.ComplaintEvent{ComplaintID: complaintID, ActorType: actorType, ActorID: actorID, EventType: eventType, Payload: payload})
	return nil
}

func (m *memRepo) ListEvents(_ context.Context, complaintID string) ([]supportmodels.ComplaintEvent, error) {
	return m.events[complaintID], nil
}

func (m *memRepo) ListHelpArticles(_ context.Context, _ string) ([]supportmodels.HelpArticle, error) {
	return nil, nil
}

// ─── Fakes ─────────────────────────────────────────────────────────────────────

type fakeResolver struct{ name, phone string }

func (f fakeResolver) Resolve(context.Context, supportmodels.ComplainantType, string) (supportclients.Identity, error) {
	return supportclients.Identity{Name: f.name, Phone: f.phone, Found: true}, nil
}

func newService(repo supportrepositories.SupportRepository, identity supportclients.IdentityResolver) *SupportService {
	return NewSupportService(repo, Options{Identity: identity})
}

func codeOf(err error) apperrors.Code {
	if err == nil {
		return ""
	}
	return apperrors.From(err).Code
}

// ─── Tests ─────────────────────────────────────────────────────────────────────

func TestCreateComplaint_WalletServiceTypeAccepted(t *testing.T) {
	svc := newService(newMemRepo(), supportclients.NoopIdentityResolver{})
	_, err := svc.CreateComplaint(context.Background(), CreateComplaintInput{
		ComplainantType: supportmodels.ComplainantCustomer, ComplainantID: "cust-1",
		ServiceType: supportmodels.ServiceTypeWallet, Subject: "Wallet issue", Description: "Charged twice",
	})
	if err != nil {
		t.Fatalf("wallet service_type should be accepted, got %v", err)
	}
}

func TestCreateComplaint_InvalidServiceType(t *testing.T) {
	svc := newService(newMemRepo(), supportclients.NoopIdentityResolver{})
	_, err := svc.CreateComplaint(context.Background(), CreateComplaintInput{
		ComplainantType: supportmodels.ComplainantCustomer, ComplainantID: "cust-1",
		ServiceType: "bogus", Subject: "x", Description: "y",
	})
	if codeOf(err) != apperrors.CodeValidationFailed {
		t.Fatalf("want validation_failed, got %v", err)
	}
}

func TestCreateComplaint_InvalidCategory(t *testing.T) {
	svc := newService(newMemRepo(), supportclients.NoopIdentityResolver{})
	_, err := svc.CreateComplaint(context.Background(), CreateComplaintInput{
		ComplainantType: supportmodels.ComplainantCustomer, ComplainantID: "cust-1",
		ServiceType: supportmodels.ServiceTypeHauling, Category: "nonsense", Subject: "x", Description: "y",
	})
	if codeOf(err) != apperrors.CodeValidationFailed {
		t.Fatalf("want validation_failed for bad category, got %v", err)
	}
}

func TestCreateComplaint_IdentitySnapshot(t *testing.T) {
	svc := newService(newMemRepo(), fakeResolver{name: "Ada Okafor", phone: "+2348000000000"})
	c, err := svc.CreateComplaint(context.Background(), CreateComplaintInput{
		ComplainantType: supportmodels.ComplainantCustomer, ComplainantID: "cust-1",
		ServiceType: supportmodels.ServiceTypeHauling, Subject: "x", Description: "y",
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if c.ComplainantName == nil || *c.ComplainantName != "Ada Okafor" {
		t.Fatalf("expected enriched name snapshot, got %v", c.ComplainantName)
	}
}

func seedComplaint(t *testing.T, svc *SupportService) supportmodels.Complaint {
	t.Helper()
	c, err := svc.CreateComplaint(context.Background(), CreateComplaintInput{
		ComplainantType: supportmodels.ComplainantCustomer, ComplainantID: "owner",
		ServiceType: supportmodels.ServiceTypeHauling, Subject: "x", Description: "y",
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	return c
}

func TestAddEvidence_OwnershipDenied(t *testing.T) {
	svc := newService(newMemRepo(), supportclients.NoopIdentityResolver{})
	c := seedComplaint(t, svc)
	_, err := svc.AddEvidence(context.Background(), AddEvidenceInput{
		ComplaintID: c.ID, UploaderType: supportmodels.ComplainantCustomer, UploaderID: "attacker", MediaURL: "http://x",
	})
	if codeOf(err) != apperrors.CodeForbidden {
		t.Fatalf("want forbidden, got %v", err)
	}
}

func TestListEvidence_OwnershipDenied(t *testing.T) {
	svc := newService(newMemRepo(), supportclients.NoopIdentityResolver{})
	c := seedComplaint(t, svc)
	_, err := svc.ListEvidence(context.Background(), c.ID, supportmodels.ComplainantCustomer, "attacker")
	if codeOf(err) != apperrors.CodeForbidden {
		t.Fatalf("want forbidden, got %v", err)
	}
}

func TestEscalateToDispute_OwnershipDenied(t *testing.T) {
	svc := newService(newMemRepo(), supportclients.NoopIdentityResolver{})
	c := seedComplaint(t, svc)
	_, err := svc.EscalateToDispute(context.Background(), EscalateToDisputeInput{
		ComplaintID: c.ID, RequesterType: supportmodels.ComplainantCustomer, RequesterID: "attacker",
		RespondentType: supportmodels.ComplainantHaulingProvider, RespondentID: "prov-1",
	})
	if codeOf(err) != apperrors.CodeForbidden {
		t.Fatalf("want forbidden, got %v", err)
	}
}

func TestEscalateToDispute_InvalidRespondentType(t *testing.T) {
	svc := newService(newMemRepo(), supportclients.NoopIdentityResolver{})
	c := seedComplaint(t, svc)
	_, err := svc.EscalateToDispute(context.Background(), EscalateToDisputeInput{
		ComplaintID: c.ID, RequesterType: supportmodels.ComplainantCustomer, RequesterID: "owner",
		RespondentType: "bogus", RespondentID: "prov-1",
	})
	if codeOf(err) != apperrors.CodeValidationFailed {
		t.Fatalf("want validation_failed, got %v", err)
	}
}

func TestGetDispute_OwnershipDenied(t *testing.T) {
	svc := newService(newMemRepo(), supportclients.NoopIdentityResolver{})
	c := seedComplaint(t, svc)
	// owner escalates so a dispute exists
	if _, err := svc.EscalateToDispute(context.Background(), EscalateToDisputeInput{
		ComplaintID: c.ID, RequesterType: supportmodels.ComplainantCustomer, RequesterID: "owner",
		RespondentType: supportmodels.ComplainantHaulingProvider, RespondentID: "prov-1",
	}); err != nil {
		t.Fatalf("escalate: %v", err)
	}
	_, err := svc.GetDispute(context.Background(), c.ID, supportmodels.ComplainantCustomer, "attacker")
	if codeOf(err) != apperrors.CodeForbidden {
		t.Fatalf("want forbidden, got %v", err)
	}
}

func TestUpdateStatus_InvalidStatus(t *testing.T) {
	svc := newService(newMemRepo(), supportclients.NoopIdentityResolver{})
	c := seedComplaint(t, svc)
	_, err := svc.UpdateStatus(context.Background(), UpdateComplaintStatusInput{ComplaintID: c.ID, Status: "nonsense"})
	if codeOf(err) != apperrors.CodeValidationFailed {
		t.Fatalf("want validation_failed, got %v", err)
	}
}

func TestResolveDispute_InvalidOutcome(t *testing.T) {
	svc := newService(newMemRepo(), supportclients.NoopIdentityResolver{})
	_, err := svc.ResolveDispute(context.Background(), ResolveDisputeInput{DisputeID: "d-1", Outcome: "nonsense"})
	if codeOf(err) != apperrors.CodeValidationFailed {
		t.Fatalf("want validation_failed, got %v", err)
	}
}

func TestCreateSOS_EmergencyPriority(t *testing.T) {
	svc := newService(newMemRepo(), supportclients.NoopIdentityResolver{})
	c, err := svc.CreateSOS(context.Background(), SOSInput{
		ComplainantType: supportmodels.ComplainantCustomer, ComplainantID: "cust-1",
	})
	if err != nil {
		t.Fatalf("sos: %v", err)
	}
	if c.Priority != supportmodels.PriorityEmergency {
		t.Fatalf("want emergency priority, got %q", c.Priority)
	}
}

func TestGetComplaint_UnreadCount(t *testing.T) {
	repo := newMemRepo()
	svc := newService(repo, supportclients.NoopIdentityResolver{})
	c := seedComplaint(t, svc)
	// admin replies → unread for the customer
	if _, err := svc.SendChatMessage(context.Background(), c.ID, supportmodels.SenderTypeAdmin, "admin", "hello"); err != nil {
		t.Fatalf("send: %v", err)
	}
	got, err := svc.GetComplaint(context.Background(), c.ID, supportmodels.ComplainantCustomer, "owner")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.UnreadCount != 1 {
		t.Fatalf("want unread 1, got %d", got.UnreadCount)
	}
	// after marking read it should clear
	if err := svc.MarkMessagesRead(context.Background(), c.ID, supportmodels.SenderType(supportmodels.ComplainantCustomer), "owner"); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	got, _ = svc.GetComplaint(context.Background(), c.ID, supportmodels.ComplainantCustomer, "owner")
	if got.UnreadCount != 0 {
		t.Fatalf("want unread 0 after read, got %d", got.UnreadCount)
	}
}
