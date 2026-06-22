package supportmodels

import "time"

type SenderType string

const (
	SenderTypeCustomer         SenderType = "customer"
	SenderTypeTaxiProvider     SenderType = "taxi_provider"
	SenderTypeDispatchProvider SenderType = "dispatch_provider"
	SenderTypeHaulingProvider  SenderType = "hauling_provider"
	SenderTypeAdmin            SenderType = "admin"
)

type ChatMessage struct {
	ID          string
	ComplaintID string
	SenderType  SenderType
	SenderID    string
	Content     string
	MediaURL    *string
	IsRead      bool
	CreatedAt   time.Time
}

type PublicChatMessage struct {
	ID          string     `json:"id"`
	ComplaintID string     `json:"complaint_id"`
	SenderType  SenderType `json:"sender_type"`
	SenderID    string     `json:"sender_id"`
	Content     string     `json:"content"`
	MediaURL    *string    `json:"media_url,omitempty"`
	IsRead      bool       `json:"is_read"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (m ChatMessage) Public() PublicChatMessage {
	return PublicChatMessage{
		ID:          m.ID,
		ComplaintID: m.ComplaintID,
		SenderType:  m.SenderType,
		SenderID:    m.SenderID,
		Content:     m.Content,
		MediaURL:    m.MediaURL,
		IsRead:      m.IsRead,
		CreatedAt:   m.CreatedAt,
	}
}

type ServiceType string

const (
	ServiceTypeTaxi     ServiceType = "taxi"
	ServiceTypeDispatch ServiceType = "dispatch"
	ServiceTypeHauling  ServiceType = "hauling"
	ServiceTypePlatform ServiceType = "platform"
)

type ComplainantType string

const (
	ComplainantCustomer         ComplainantType = "customer"
	ComplainantTaxiProvider     ComplainantType = "taxi_provider"
	ComplainantDispatchProvider ComplainantType = "dispatch_provider"
	ComplainantHaulingProvider  ComplainantType = "hauling_provider"
)

type ComplaintStatus string

const (
	ComplaintStatusOpen             ComplaintStatus = "open"
	ComplaintStatusUnderReview      ComplaintStatus = "under_review"
	ComplaintStatusAwaitingEvidence ComplaintStatus = "awaiting_evidence"
	ComplaintStatusResolved         ComplaintStatus = "resolved"
	ComplaintStatusClosed           ComplaintStatus = "closed"
	ComplaintStatusEscalated        ComplaintStatus = "escalated"
)

type DisputeOutcome string

const (
	DisputeOutcomePending           DisputeOutcome = "pending"
	DisputeOutcomeFavourComplainant DisputeOutcome = "favour_complainant"
	DisputeOutcomeFavourRespondent  DisputeOutcome = "favour_respondent"
	DisputeOutcomeSplit             DisputeOutcome = "split"
	DisputeOutcomeDismissed         DisputeOutcome = "dismissed"
)

type Complaint struct {
	ID               string
	ComplainantType  ComplainantType
	ComplainantID    string
	ServiceType      ServiceType
	BookingReference *string
	Subject          string
	Description      string
	Status           ComplaintStatus
	AssignedTo       *string
	ResolutionNote   *string
	ResolvedAt       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type Evidence struct {
	ID           string
	ComplaintID  string
	UploaderType ComplainantType
	UploaderID   string
	MediaAssetID *string
	MediaURL     *string
	Note         *string
	CreatedAt    time.Time
}

type Dispute struct {
	ID               string
	ComplaintID      string
	ServiceType      ServiceType
	BookingReference *string
	RespondentType   ComplainantType
	RespondentID     string
	Outcome          DisputeOutcome
	AdjudicatorID    *string
	AdjudicationNote *string
	ResolvedAt       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// PublicComplaint is the API response shape.
type PublicComplaint struct {
	ID               string          `json:"id"`
	ComplainantType  ComplainantType `json:"complainant_type"`
	ComplainantID    string          `json:"complainant_id"`
	ServiceType      ServiceType     `json:"service_type"`
	BookingReference *string         `json:"booking_reference,omitempty"`
	Subject          string          `json:"subject"`
	Description      string          `json:"description"`
	Status           ComplaintStatus `json:"status"`
	ResolutionNote   *string         `json:"resolution_note,omitempty"`
	ResolvedAt       *time.Time      `json:"resolved_at,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

func (c Complaint) Public() PublicComplaint {
	return PublicComplaint{
		ID:               c.ID,
		ComplainantType:  c.ComplainantType,
		ComplainantID:    c.ComplainantID,
		ServiceType:      c.ServiceType,
		BookingReference: c.BookingReference,
		Subject:          c.Subject,
		Description:      c.Description,
		Status:           c.Status,
		ResolutionNote:   c.ResolutionNote,
		ResolvedAt:       c.ResolvedAt,
		CreatedAt:        c.CreatedAt,
		UpdatedAt:        c.UpdatedAt,
	}
}

// PublicDispute is the API response shape for disputes.
type PublicDispute struct {
	ID               string          `json:"id"`
	ComplaintID      string          `json:"complaint_id"`
	ServiceType      ServiceType     `json:"service_type"`
	BookingReference *string         `json:"booking_reference,omitempty"`
	RespondentType   ComplainantType `json:"respondent_type"`
	RespondentID     string          `json:"respondent_id"`
	Outcome          DisputeOutcome  `json:"outcome"`
	AdjudicationNote *string         `json:"adjudication_note,omitempty"`
	ResolvedAt       *time.Time      `json:"resolved_at,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
}

func (d Dispute) Public() PublicDispute {
	return PublicDispute{
		ID:               d.ID,
		ComplaintID:      d.ComplaintID,
		ServiceType:      d.ServiceType,
		BookingReference: d.BookingReference,
		RespondentType:   d.RespondentType,
		RespondentID:     d.RespondentID,
		Outcome:          d.Outcome,
		AdjudicationNote: d.AdjudicationNote,
		ResolvedAt:       d.ResolvedAt,
		CreatedAt:        d.CreatedAt,
	}
}
