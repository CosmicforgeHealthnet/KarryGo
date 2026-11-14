package outbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// TargetFleetMarketplace is the outbox target for marketplace request and
// organization-trip lifecycle events delivered to Fleet's event ingestion
// endpoint (Sections 11-12). Rows are written by the taxi_bookings feed
// triggers (migration 029).
const TargetFleetMarketplace = "fleet-marketplace"

// FleetEventsPath is Fleet's internal marketplace event ingestion route.
const FleetEventsPath = "/internal/fleet-partner/v1/events"

// FleetMarketplaceEventDestination delivers one feed row to Fleet as the
// envelope Fleet's marketplace ingestion expects (eventId, eventType,
// sourceService, aggregate identity and version, payloadHash, payload),
// signed with the service HMAC.
type FleetMarketplaceEventDestination struct {
	BaseURL     string
	ServiceName string
	Secret      []byte
	HTTPClient  *http.Client
	Enabled     bool
}

// Deliver posts one row. A 400 is Fleet's permanent rejection of the event
// itself (unknown type, bad envelope, hash mismatch) and dead-letters at once;
// everything else -- including 403, which means a caller allowlist not yet
// configured -- is retried, so a configuration fix loses nothing.
func (d FleetMarketplaceEventDestination) Deliver(ctx context.Context, record Record) error {
	if !d.Enabled || strings.TrimSpace(d.BaseURL) == "" || len(d.Secret) == 0 {
		return errors.New("fleet marketplace event delivery is not enabled")
	}
	var payload map[string]any
	if err := json.Unmarshal(record.Payload, &payload); err != nil {
		return &TerminalDeliveryError{Code: "payload_decode_failed", Err: err}
	}
	version, ok := payload["sourceVersion"].(float64)
	if !ok || version < 1 {
		return &TerminalDeliveryError{Code: "missing_source_version"}
	}
	// The hash covers the canonical encoding (sorted keys, compact) Fleet
	// recomputes from the payload it decodes.
	canonical, err := json.Marshal(payload)
	if err != nil {
		return &TerminalDeliveryError{Code: "payload_encode_failed", Err: err}
	}
	sum := sha256.Sum256(canonical)
	envelope := map[string]any{
		"eventId":          record.ID,
		"eventType":        record.EventType,
		"schemaVersion":    1,
		"sourceService":    d.ServiceName,
		"aggregateType":    strings.ToLower(record.AggregateType),
		"aggregateId":      record.AggregateID,
		"aggregateVersion": int64(version),
		"occurredAt":       record.OccurredAt.UTC(),
		"payloadHash":      hex.EncodeToString(sum[:]),
		"payload":          payload,
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return &TerminalDeliveryError{Code: "envelope_encode_failed", Err: err}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(d.BaseURL, "/")+FleetEventsPath, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Service-Nonce", record.ID)
	if err := serviceauth.SignRequest(request, d.ServiceName, d.Secret, body, time.Now().UTC()); err != nil {
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
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	switch {
	case response.StatusCode >= 200 && response.StatusCode < 300:
		return nil
	case response.StatusCode == http.StatusBadRequest:
		// Fleet's standard error envelope carries the code in error.code;
		// before FLEET-6h it was bare at the top level (Fleet still repeats it
		// there as a deprecated alias). Read the envelope first.
		var rejection struct {
			Code  string `json:"code"`
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(responseBody, &rejection)
		code := rejection.Code
		if rejection.Error != nil && rejection.Error.Code != "" {
			code = rejection.Error.Code
		}
		return &TerminalDeliveryError{Code: codeOr(code, "fleet_rejected")}
	default:
		return fmt.Errorf("fleet event ingestion returned %d", response.StatusCode)
	}
}
