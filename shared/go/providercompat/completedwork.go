package providercompat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// CompletedWorkPath is the canonical Section 7 Fleet ingestion endpoint. Every
// execution service submits managed completed work here; there is deliberately
// no per-service variant of this contract.
const CompletedWorkPath = "/internal/fleet-partner/v1/completed-work"

// Source service identifiers accepted by Fleet's completed-work ingestion.
const (
	SourceServiceTaxi     = "TAXI"
	SourceServiceDispatch = "DISPATCH"
	SourceServiceHauling  = "HAULING"
)

// CompletedWorkRequest is the canonical Fleet completed-work DTO.
//
// It intentionally carries no pricing inputs beyond an already-authorized
// monetary basis: EligibleBasisMinor is a value the execution service has
// already settled, never a price this contract computes.
type CompletedWorkRequest struct {
	SourceService      string    `json:"sourceService"`
	SourceJobID        string    `json:"sourceJobId"`
	SourceAssignmentID string    `json:"sourceAssignmentId"`
	ProviderID         string    `json:"providerId"`
	MembershipID       string    `json:"membershipId"`
	OrganizationID     string    `json:"organizationId"`
	CompletedAt        time.Time `json:"completedAt"`
	Currency           string    `json:"currency"`
	EligibleBasisMinor int64     `json:"eligibleBasisMinor"`
	SourceVersion      int64     `json:"sourceVersion"`
	IdempotencyKey     string    `json:"idempotencyKey"`
	CorrelationID      string    `json:"correlationId"`
}

// Validate applies the rules Fleet itself enforces, so an unsubmittable
// payload is caught before it ever consumes a delivery attempt.
func (r CompletedWorkRequest) Validate() error {
	switch r.SourceService {
	case SourceServiceTaxi, SourceServiceDispatch, SourceServiceHauling:
	default:
		return fmt.Errorf("completed work: sourceService %q is not a valid execution source", r.SourceService)
	}
	for name, value := range map[string]string{
		"sourceJobId":    r.SourceJobID,
		"providerId":     r.ProviderID,
		"membershipId":   r.MembershipID,
		"organizationId": r.OrganizationID,
		"idempotencyKey": r.IdempotencyKey,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("completed work: %s is required", name)
		}
	}
	if strings.TrimSpace(r.Currency) == "" {
		return errors.New("completed work: currency is required")
	}
	if r.EligibleBasisMinor < 0 {
		return errors.New("completed work: eligibleBasisMinor must be non-negative")
	}
	if r.CompletedAt.IsZero() {
		return errors.New("completed work: completedAt is required")
	}
	if r.CompletedAt.Location() != time.UTC {
		return errors.New("completed work: completedAt must be UTC")
	}
	return nil
}

// CompletedWorkAck is what Fleet returns once the record is durable. A replay
// of the same idempotency key returns the same record rather than a new one.
type CompletedWorkAck struct {
	CompletedWorkID string `json:"completedWorkId"`
	IngestionStatus string `json:"ingestionStatus"`
	Replayed        bool   `json:"replayed"`
}

// SubmitOutcome classifies one completed-work delivery attempt.
type SubmitOutcome string

const (
	// SubmitAccepted means Fleet durably recorded the completed work.
	SubmitAccepted SubmitOutcome = "ACCEPTED"
	// SubmitDuplicate means Fleet already held this idempotency identity.
	// This is a success: it proves the record exists exactly once.
	SubmitDuplicate SubmitOutcome = "DUPLICATE"
	// SubmitRetryable means Fleet did not commit and a plain retry is safe.
	SubmitRetryable SubmitOutcome = "RETRYABLE"
	// SubmitUnknown means we never learned whether Fleet committed. The retry
	// must reuse the SAME idempotency key so Fleet returns the existing
	// result instead of creating a second record.
	SubmitUnknown SubmitOutcome = "UNKNOWN"
	// SubmitTerminal means Fleet authoritatively rejected the payload and no
	// retry can succeed. The row moves to reconciliation, not to oblivion.
	SubmitTerminal SubmitOutcome = "TERMINAL"
)

// Retryable reports whether the delivery should be attempted again.
func (o SubmitOutcome) Retryable() bool {
	return o == SubmitRetryable || o == SubmitUnknown
}

// Delivered reports whether Fleet is known to hold the record.
func (o SubmitOutcome) Delivered() bool {
	return o == SubmitAccepted || o == SubmitDuplicate
}

// SubmitResult is the classified result of one attempt.
type SubmitResult struct {
	Outcome SubmitOutcome
	Code    string
	Ack     CompletedWorkAck
}

// SubmitCompletedWork posts one canonical completed-work record.
//
// The classification is the contract: a transport failure is UNKNOWN (Fleet
// may have committed), a 409 is DUPLICATE (Fleet holds it), a 4xx that cannot
// be fixed by retrying is TERMINAL, and everything transient is RETRYABLE.
// Callers must never mint a fresh idempotency key on retry.
func (c *Client) SubmitCompletedWork(ctx context.Context, request CompletedWorkRequest) (SubmitResult, error) {
	if c == nil || strings.TrimSpace(c.BaseURL) == "" || strings.TrimSpace(c.ServiceName) == "" || len(c.Secret) == 0 {
		return SubmitResult{Outcome: SubmitRetryable, Code: "fleet_client_not_configured"},
			errors.New("completed work: fleet client is not configured")
	}
	if err := request.Validate(); err != nil {
		// Unsubmittable by construction: never burn retries on it.
		return SubmitResult{Outcome: SubmitTerminal, Code: "invalid_payload"}, err
	}
	body, err := json.Marshal(request)
	if err != nil {
		return SubmitResult{Outcome: SubmitTerminal, Code: "payload_encode_failed"}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+CompletedWorkPath, bytes.NewReader(body))
	if err != nil {
		return SubmitResult{Outcome: SubmitRetryable, Code: "request_build_failed"}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Idempotency-Key", request.IdempotencyKey)
	if request.CorrelationID != "" {
		req.Header.Set("X-Correlation-ID", request.CorrelationID)
	}
	if err := serviceauth.SignRequest(req, c.ServiceName, c.Secret, body, time.Now()); err != nil {
		return SubmitResult{Outcome: SubmitRetryable, Code: "request_sign_failed"}, err
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		// The request may have been received and committed before the
		// connection broke. Anything stronger than UNKNOWN would be a guess.
		return SubmitResult{Outcome: SubmitUnknown, Code: "transport_failure"}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return classifyCompletedWork(resp.StatusCode, raw), nil
}

// terminalCompletedWorkCodes are Fleet rejections that retrying cannot fix.
// Every one of them corresponds to a Sections 8/9 terminal classification:
// the caller is not the source owner, the provider or membership is wrong, or
// the payload itself is invalid.
var terminalCompletedWorkCodes = map[string]bool{
	"forbidden":         true,
	"validation_failed": true,
	"bad_request":       true,
	"not_found":         true,
}

func classifyCompletedWork(status int, raw []byte) SubmitResult {
	var envelope struct {
		Success bool `json:"success"`
		Data    struct {
			CompletedWork struct {
				ID              string `json:"ID"`
				IngestionStatus string `json:"IngestionStatus"`
			} `json:"completedWork"`
		} `json:"data"`
		Error struct {
			Code      string `json:"code"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &envelope)
	code := strings.TrimSpace(envelope.Error.Code)
	ack := CompletedWorkAck{
		CompletedWorkID: envelope.Data.CompletedWork.ID,
		IngestionStatus: envelope.Data.CompletedWork.IngestionStatus,
	}

	switch {
	case status == http.StatusOK || status == http.StatusCreated || status == http.StatusAccepted:
		return SubmitResult{Outcome: SubmitAccepted, Ack: ack}
	case status == http.StatusConflict:
		// Fleet already holds this idempotency identity. Idempotent success:
		// exactly one record exists, which is what the contract guarantees.
		ack.Replayed = true
		return SubmitResult{Outcome: SubmitDuplicate, Code: code, Ack: ack}
	case status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status >= 500:
		return SubmitResult{Outcome: SubmitRetryable, Code: completedWorkCode(code, status)}
	case status == http.StatusServiceUnavailable:
		return SubmitResult{Outcome: SubmitRetryable, Code: completedWorkCode(code, status)}
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		// Wrong caller identity or source ownership. Retrying with the same
		// credentials cannot succeed; an operator has to intervene.
		return SubmitResult{Outcome: SubmitTerminal, Code: completedWorkCode(code, status)}
	case status >= 400:
		// Fleet marks genuinely transient rejections retryable in the error
		// envelope itself. Honour that before any code heuristic.
		if envelope.Error.Retryable {
			return SubmitResult{Outcome: SubmitRetryable, Code: completedWorkCode(code, status)}
		}
		if code != "" && !terminalCompletedWorkCodes[code] {
			// An unrecognised 4xx code may be a transient policy gate (for
			// example a projection flag that is not enabled yet), so keep it
			// queued rather than dead-lettering authoritative completion.
			return SubmitResult{Outcome: SubmitRetryable, Code: code}
		}
		return SubmitResult{Outcome: SubmitTerminal, Code: completedWorkCode(code, status)}
	default:
		return SubmitResult{Outcome: SubmitUnknown, Code: completedWorkCode(code, status)}
	}
}

func completedWorkCode(code string, status int) string {
	if code != "" {
		return code
	}
	return fmt.Sprintf("http_%d", status)
}
