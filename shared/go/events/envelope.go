package events

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// Envelope is the canonical cross-service event envelope. Producers write
// it through their transactional outbox. Consumers deduplicate on EventID
// and order per source entity with SourceVersion.
type Envelope struct {
	EventID          string          `json:"event_id"`
	EventName        string          `json:"event_name"`
	SchemaVersion    int             `json:"schema_version"`
	SourceService    string          `json:"source_service"`
	SourceEntityType string          `json:"source_entity_type"`
	SourceEntityID   string          `json:"source_entity_id"`
	SourceVersion    int64           `json:"source_version"`
	OccurredAt       time.Time       `json:"occurred_at"`
	CorrelationID    string          `json:"correlation_id,omitempty"`
	CausationID      string          `json:"causation_id,omitempty"`
	Payload          json.RawMessage `json:"payload"`
}

var ErrInvalidEnvelope = errors.New("event envelope is invalid")

// Validate enforces the fields every consumer relies on for idempotency and
// ordering. Correlation and causation ids are optional but bounded.
func (e Envelope) Validate() error {
	switch {
	case strings.TrimSpace(e.EventID) == "", len(e.EventID) > 128:
		return fmt.Errorf("%w: event_id", ErrInvalidEnvelope)
	case strings.TrimSpace(e.EventName) == "", len(e.EventName) > 128:
		return fmt.Errorf("%w: event_name", ErrInvalidEnvelope)
	case e.SchemaVersion < 1:
		return fmt.Errorf("%w: schema_version", ErrInvalidEnvelope)
	case strings.TrimSpace(e.SourceService) == "":
		return fmt.Errorf("%w: source_service", ErrInvalidEnvelope)
	case strings.TrimSpace(e.SourceEntityType) == "", strings.TrimSpace(e.SourceEntityID) == "":
		return fmt.Errorf("%w: source entity", ErrInvalidEnvelope)
	case e.SourceVersion < 1:
		return fmt.Errorf("%w: source_version", ErrInvalidEnvelope)
	case e.OccurredAt.IsZero():
		return fmt.Errorf("%w: occurred_at", ErrInvalidEnvelope)
	case len(e.CorrelationID) > 128, len(e.CausationID) > 128:
		return fmt.Errorf("%w: correlation", ErrInvalidEnvelope)
	case len(bytes.TrimSpace(e.Payload)) == 0 || !json.Valid(e.Payload):
		return fmt.Errorf("%w: payload", ErrInvalidEnvelope)
	}
	return nil
}

// HTTPDeliverer posts one envelope to an HMAC-protected ingress. A 2xx
// response acknowledges delivery. The consumer is idempotent, so outbox
// retries after an ambiguous failure are safe.
type HTTPDeliverer struct {
	BaseURL     string
	Path        string
	ServiceName string
	Secret      []byte
	HTTPClient  *http.Client
	Now         func() time.Time
}

func (d HTTPDeliverer) Configured() bool {
	return strings.TrimSpace(d.BaseURL) != "" && strings.TrimSpace(d.Path) != "" && strings.TrimSpace(d.ServiceName) != "" && len(d.Secret) > 0
}

func (d HTTPDeliverer) Deliver(ctx context.Context, envelope Envelope) error {
	if !d.Configured() {
		return errors.New("event destination is not configured")
	}
	if err := envelope.Validate(); err != nil {
		return err
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(d.BaseURL, "/")+d.Path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	now := time.Now
	if d.Now != nil {
		now = d.Now
	}
	if err = serviceauth.SignRequest(request, d.ServiceName, d.Secret, body, now().UTC()); err != nil {
		return err
	}
	client := d.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	_ = response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("event ingress returned %d", response.StatusCode)
	}
	return nil
}
