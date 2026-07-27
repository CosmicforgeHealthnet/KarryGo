package marketplace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// maxFleetResponseBytes bounds every response body we read.
//
// A hostile or broken peer must not be able to exhaust this process's memory
// through a relay delivery, and a provider's decision is a small document — a
// megabyte is already far more than any legitimate answer needs.
const maxFleetResponseBytes = 1 << 20

// HTTPFleetResponder is the REAL FleetResponder.
//
// It delivers a provider's Accept or Decline to fleet-partner-service over
// authenticated HTTP. The relay worker owns durability and retry policy; this
// type owns exactly one delivery attempt and the honest classification of what
// came back.
type HTTPFleetResponder struct {
	baseURL     string
	serviceName string
	secret      []byte
	client      *http.Client
}

// ResponderConfig configures the real responder.
type ResponderConfig struct {
	// BaseURL of fleet-partner-service.
	BaseURL string
	// ServiceName is THIS service's identity as fleet knows it. It must match
	// an entry in fleet's caller allowlist or every delivery is refused.
	ServiceName string
	// Secret is the shared HMAC secret for that identity.
	Secret []byte
	// Timeout bounds a single delivery attempt. Without it a hung fleet would
	// hold a relay lease until it expired, delaying a provider's decision.
	Timeout time.Duration
}

// NewHTTPFleetResponder builds the real responder, or reports that
// configuration is incomplete so the caller falls back to the fail-closed
// UnconfiguredResponder rather than a half-working client.
func NewHTTPFleetResponder(config ResponderConfig) (FleetResponder, bool) {
	if strings.TrimSpace(config.BaseURL) == "" ||
		strings.TrimSpace(config.ServiceName) == "" || len(config.Secret) == 0 {
		return UnconfiguredResponder{}, false
	}
	if _, err := url.Parse(config.BaseURL); err != nil {
		return UnconfiguredResponder{}, false
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &HTTPFleetResponder{
		baseURL:     strings.TrimRight(config.BaseURL, "/"),
		serviceName: config.ServiceName,
		secret:      config.Secret,
		client:      &http.Client{Timeout: timeout},
	}, true
}

// Configured reports true: this responder can actually reach fleet.
func (r *HTTPFleetResponder) Configured() bool { return true }

// permanentFleetCodes are fleet answers that will never change on retry.
//
// Retrying any of these would burn the relay's attempt budget on a settled
// question, and would delay the operator alert that the decision needs human
// attention.
var permanentFleetCodes = map[string]bool{
	"assignment_not_owned_by_provider":   true,
	"assignment_already_resolved":        true,
	"assignment_response_expired":        true,
	"assignment_not_found":               true,
	"marketplace_offer_not_found":        true,
	"marketplace_offer_expired":          true,
	"marketplace_offer_cancelled":        true,
	"marketplace_offer_already_resolved": true,
	"expected_version_mismatch":          true,
	"idempotency_conflict":               true,
	"decline_reason_required":            true,
	"decline_reason_invalid":             true,
	"decline_note_required":              true,
	"decline_evidence_invalid":           true,
	"validation_error":                   true,
	"provider_acceptance_cancelled":      true,
	"provider_acceptance_expired":        true,
	"provider_acceptance_mismatch":       true,
	"forbidden":                          true,
	"unauthorized":                       true,
}

// relayPath maps an operation type onto its fleet route.
func relayPath(op RelayOperation) (string, error) {
	switch op.OperationType {
	case RelayProviderOfferAccept:
		if op.OfferID == nil {
			return "", errors.New("marketplace: offer relay without an offer")
		}
		return "/internal/fleet-partner/v1/marketplace-offers/" +
			url.PathEscape(*op.OfferID) + "/provider-accept", nil
	case RelayProviderOfferDecline:
		if op.OfferID == nil {
			return "", errors.New("marketplace: offer relay without an offer")
		}
		return "/internal/fleet-partner/v1/marketplace-offers/" +
			url.PathEscape(*op.OfferID) + "/provider-decline", nil
	case RelayProviderAssignmentAccept:
		if op.AssignmentID == nil {
			return "", errors.New("marketplace: assignment relay without an assignment")
		}
		return "/internal/fleet-partner/v1/assignments/" +
			url.PathEscape(*op.AssignmentID) + "/provider-accept", nil
	case RelayProviderAssignmentDecline:
		if op.AssignmentID == nil {
			return "", errors.New("marketplace: assignment relay without an assignment")
		}
		return "/internal/fleet-partner/v1/assignments/" +
			url.PathEscape(*op.AssignmentID) + "/provider-decline", nil
	default:
		return "", fmt.Errorf("marketplace: unknown relay operation %q", op.OperationType)
	}
}

// fleetResponseEnvelope is fleet's canonical success shape.
type fleetResponseEnvelope struct {
	Data struct {
		OfferID        string   `json:"offerId"`
		RecipientID    string   `json:"recipientId"`
		AssignmentID   string   `json:"assignmentId"`
		Status         string   `json:"status"`
		AllowedActions []string `json:"allowedActions"`
		Replayed       bool     `json:"replayed"`
	} `json:"data"`
}

// Deliver sends one provider decision to fleet.
//
// The three outcomes are deliberately distinct, and getting them wrong has
// very different costs:
//   - permanent  → stop; the answer will not change
//   - temporary  → retry; fleet is briefly unwell
//   - unknown    → retry with the SAME key; fleet may already have recorded it
//
// Anything ambiguous is classified as unknown, never as failure, because
// re-sending under a new identity could record a provider's decision twice.
func (r *HTTPFleetResponder) Deliver(ctx context.Context, meta CallMeta,
	op RelayOperation) (RelayDeliveryResult, error) {
	path, err := relayPath(op)
	if err != nil {
		// A malformed relay cannot become well-formed by retrying.
		return RelayDeliveryResult{Permanent: true, ErrorCode: "relay_malformed"}, nil
	}

	body := op.Payload
	if len(body) == 0 {
		body = []byte(`{}`)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+path,
		bytes.NewReader(body))
	if err != nil {
		return RelayDeliveryResult{}, ErrFleetUnavailable
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	// The ORIGINAL idempotency key. This is what lets fleet recognise a
	// redelivery as the same decision rather than a second one.
	req.Header.Set(HeaderIdempotencyKey, op.IdempotencyKey)
	if meta.RequestID != "" {
		req.Header.Set(HeaderRequestID, meta.RequestID)
	}
	if meta.CorrelationID != "" {
		req.Header.Set(HeaderCorrelationID, meta.CorrelationID)
	}
	if err := serviceauth.SignRequest(req, r.serviceName, r.secret, body, time.Now()); err != nil {
		return RelayDeliveryResult{}, ErrFleetUnavailable
	}

	resp, err := r.client.Do(req)
	if err != nil {
		// Timeout, connection reset, DNS failure: the request may already have
		// been transmitted and committed. Unknown, never a rejection.
		return RelayDeliveryResult{}, ErrFleetUnavailable
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxFleetResponseBytes))
	if err != nil {
		return RelayDeliveryResult{}, ErrFleetUnavailable
	}

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return r.classifySuccess(payload)

	case resp.StatusCode == http.StatusUnauthorized ||
		resp.StatusCode == http.StatusForbidden:
		// Misconfiguration on our side: a wrong secret or a caller fleet does
		// not allowlist. Permanent — retrying cannot fix credentials — and it
		// must surface to an operator rather than silently draining retries.
		return RelayDeliveryResult{Permanent: true,
			ErrorCode: fleetErrorCode(payload, "unauthorized")}, nil

	case resp.StatusCode == http.StatusTooManyRequests:
		return RelayDeliveryResult{ErrorCode: "rate_limited"}, nil

	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		code := fleetErrorCode(payload, "fleet_rejected")
		return RelayDeliveryResult{Permanent: permanentFleetCodes[code], ErrorCode: code}, nil

	case resp.StatusCode == http.StatusServiceUnavailable ||
		resp.StatusCode == http.StatusBadGateway ||
		resp.StatusCode == http.StatusGatewayTimeout:
		// Fleet is briefly unwell and almost certainly did not process it.
		return RelayDeliveryResult{ErrorCode: fleetErrorCode(payload, "fleet_unavailable")}, nil

	default:
		// A 500 may have committed before failing to answer. Ambiguous.
		return RelayDeliveryResult{}, ErrFleetUnavailable
	}
}

// classifySuccess validates a 2xx body before treating it as success.
//
// A 2xx with an unparseable or empty body is NOT success: fleet may have
// committed, or a proxy may have produced it. Reporting fake success would
// confirm a relay that was never recorded, losing the provider's decision.
func (r *HTTPFleetResponder) classifySuccess(payload []byte) (RelayDeliveryResult, error) {
	var envelope fleetResponseEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return RelayDeliveryResult{}, ErrFleetUnavailable
	}
	data := envelope.Data
	// A real answer always names what it answered and how.
	if data.Status == "" || (data.OfferID == "" && data.AssignmentID == "") {
		return RelayDeliveryResult{}, ErrFleetUnavailable
	}

	return RelayDeliveryResult{
		Accepted: true,
		Snapshot: map[string]any{
			"status":       data.Status,
			"offerId":      data.OfferID,
			"recipientId":  data.RecipientID,
			"assignmentId": data.AssignmentID,
			"replayed":     data.Replayed,
		},
	}, nil
}

// fleetErrorCode extracts fleet's stable error code.
//
// Only the CODE is retained. Fleet error messages can quote request content,
// and this value is persisted on the relay row and written to logs, so keeping
// the free-text message would risk leaking payload detail into operator-visible
// storage.
func fleetErrorCode(payload []byte, fallback string) string {
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return fallback
	}
	if envelope.Error.Code == "" {
		return fallback
	}
	return envelope.Error.Code
}
