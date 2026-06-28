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
	ServiceTypeWallet   ServiceType = "wallet"
)

// Complaint priorities. Emergency/SOS reports surface at the top of the admin
// queue.
const (
	PriorityNormal    = "normal"
	PriorityHigh      = "high"
	PriorityEmergency = "emergency"
)

// ValidCategories is the optional category allow-list. A complaint may omit a
// category; when provided it must be one of these (drives admin triage +
// analytics). Kept in code (not a DB enum) so adding one needs no migration.
var ValidCategories = map[string]bool{
	"incorrect_delivery":  true,
	"delayed_arrival":     true,
	"payment":             true,
	"damaged_goods":       true,
	"provider_misconduct": true,
	"fraud":               true,
	"emergency":           true,
	"other":               true,
}

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
	ComplainantName  *string
	ComplainantPhone *string
	ServiceType      ServiceType
	BookingReference *string
	Category         *string
	Priority         string
	IncidentLat      *float64
	IncidentLng      *float64
	Subject          string
	Description      string
	Status           ComplaintStatus
	AssignedTo       *string
	ResolutionNote   *string
	ResolvedAt       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time

	// UnreadCount is populated only by list/get queries that join the chat
	// table; it is the number of messages the requester has not yet read.
	UnreadCount int
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
	RespondentName   *string
	RespondentPhone  *string
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
	ComplainantName  *string         `json:"complainant_name,omitempty"`
	ComplainantPhone *string         `json:"complainant_phone,omitempty"`
	ServiceType      ServiceType     `json:"service_type"`
	BookingReference *string         `json:"booking_reference,omitempty"`
	Category         *string         `json:"category,omitempty"`
	Priority         string          `json:"priority"`
	IncidentLat      *float64        `json:"incident_lat,omitempty"`
	IncidentLng      *float64        `json:"incident_lng,omitempty"`
	Subject          string          `json:"subject"`
	Description      string          `json:"description"`
	Status           ComplaintStatus `json:"status"`
	ResolutionNote   *string         `json:"resolution_note,omitempty"`
	ResolvedAt       *time.Time      `json:"resolved_at,omitempty"`
	UnreadCount      int             `json:"unread_count"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

func (c Complaint) Public() PublicComplaint {
	priority := c.Priority
	if priority == "" {
		priority = PriorityNormal
	}
	return PublicComplaint{
		ID:               c.ID,
		ComplainantType:  c.ComplainantType,
		ComplainantID:    c.ComplainantID,
		ComplainantName:  c.ComplainantName,
		ComplainantPhone: c.ComplainantPhone,
		ServiceType:      c.ServiceType,
		BookingReference: c.BookingReference,
		Category:         c.Category,
		Priority:         priority,
		IncidentLat:      c.IncidentLat,
		IncidentLng:      c.IncidentLng,
		Subject:          c.Subject,
		Description:      c.Description,
		Status:           c.Status,
		ResolutionNote:   c.ResolutionNote,
		ResolvedAt:       c.ResolvedAt,
		UnreadCount:      c.UnreadCount,
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
	RespondentName   *string         `json:"respondent_name,omitempty"`
	RespondentPhone  *string         `json:"respondent_phone,omitempty"`
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
		RespondentName:   d.RespondentName,
		RespondentPhone:  d.RespondentPhone,
		Outcome:          d.Outcome,
		AdjudicationNote: d.AdjudicationNote,
		ResolvedAt:       d.ResolvedAt,
		CreatedAt:        d.CreatedAt,
	}
}

// HelpArticle is a published FAQ/help-center entry served to the apps for
// self-service.
type HelpArticle struct {
	ID        string    `json:"id"`
	Audience  string    `json:"audience"`
	Category  *string   `json:"category,omitempty"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
}

// ComplaintEvent is one row of the complaint audit trail.
type ComplaintEvent struct {
	ID          string         `json:"id"`
	ComplaintID string         `json:"complaint_id"`
	ActorType   string         `json:"actor_type"`
	ActorID     string         `json:"actor_id"`
	EventType   string         `json:"event_type"`
	Payload     map[string]any `json:"payload,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}
