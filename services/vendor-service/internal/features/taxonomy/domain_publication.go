package taxonomy

import (
	"encoding/json"
	"time"
)

type PublicationHistoryRecord struct {
	ID                string
	NamespaceID       string
	VersionID         string
	PreviousVersionID *string
	EventType         string
	ActorRef          string
	CommandRef        string
	CorrelationID     string
	CausationID       string
	Metadata          json.RawMessage
	OccurredAt        time.Time
}
