package supportclients

// Outbound collaborators for Section 15: reading operational context from the
// owning services, and delivering action requests to them.
//
// Both only ever go to the OWNER of a job, through a contract that owner
// exposes. Support has no client here that writes another service's state:
// the one delivery below calls an owner's own recovery-start operation, which
// validates everything and may refuse.

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
	"cosmicforge/logistics/shared/go/supportcase"
)

// ContextReader reads minimized operational context from one owner.
type ContextReader interface {
	Read(ctx context.Context, request supportcase.ContextRequest) (supportcase.OperationalContext, error)
}

// ContextRegistry routes a context type to the service that owns it.
//
// A context type with no reader is simply absent. Enriching such a complaint
// reports "no owner configured" rather than guessing at another service.
type ContextRegistry struct {
	readers map[string]ContextReader
	owners  map[string]string
}

// NewContextRegistry builds an empty registry.
func NewContextRegistry() *ContextRegistry {
	return &ContextRegistry{readers: map[string]ContextReader{}, owners: map[string]string{}}
}

// Register binds a context type to its owner's reader.
func (r *ContextRegistry) Register(contextType, owner string, reader ContextReader) {
	if r == nil || reader == nil {
		return
	}
	r.readers[contextType] = reader
	r.owners[contextType] = owner
}

// ReaderFor returns the reader and owner for a context type.
func (r *ContextRegistry) ReaderFor(contextType string) (ContextReader, string, bool) {
	if r == nil {
		return nil, "", false
	}
	reader, ok := r.readers[contextType]
	return reader, r.owners[contextType], ok
}

// NewContextRegistryFromConfig wires the owners that expose the context read.
//
// Each base is the owner's internal base -- origin plus API base -- because the
// services do not share one: /api/v1/taxi, /api/v1/dispatch and
// /api/v1/hauling -- the same bases Support's identity clients already use.
func NewContextRegistryFromConfig(taxiBase, taxiSecret, dispatchBase, dispatchSecret,
	haulingBase, haulingSecret string, timeout time.Duration) *ContextRegistry {
	registry := NewContextRegistry()
	add := func(base, secret, owner string, types ...string) {
		if strings.TrimSpace(base) == "" || strings.TrimSpace(secret) == "" {
			return
		}
		client := supportcase.NewContextClient(base, []byte(secret), timeout)
		for _, contextType := range types {
			registry.Register(contextType, owner, client)
		}
	}
	add(taxiBase, taxiSecret, "driver-taxi-service",
		supportcase.ContextTaxiBooking, supportcase.ContextTaxiGoodsMission)
	add(dispatchBase, dispatchSecret, "driver-dispatch-delivery-service",
		supportcase.ContextDispatchMission)
	add(haulingBase, haulingSecret, "driver-hauling-service",
		supportcase.ContextHaulageBooking)
	return registry
}

// ActionOutcome is what an owner said about one delivered action.
type ActionOutcome struct {
	// Definite is false when the owner could not be reached or its answer
	// could not be read. The request is then re-sent under the SAME
	// operation reference, which the owner treats as a replay.
	Definite  bool
	Accepted  bool
	ResultRef string
	Code      string
}

// ActionDelivery is one owner operation Support may ask for.
type ActionDelivery struct {
	TargetRef    string
	OperationRef string
	Parameters   map[string]string
	Correlation  string
}

// ActionDeliverer sends one kind of action to its owner.
type ActionDeliverer interface {
	Deliver(ctx context.Context, delivery ActionDelivery) (ActionOutcome, error)
}

// HaulingRecoveryEscalation asks Hauling to start recovery for a booking,
// through Hauling's own recovery-start contract.
//
// Support does not pick a replacement, does not change the booking, and does
// not decide whether recovery is warranted. It supplies the booking, a trigger
// the agent observed, and an idempotency reference; Hauling validates all of
// it and may refuse.
type HaulingRecoveryEscalation struct {
	base   string
	secret []byte
	http   *http.Client
	now    func() time.Time
}

// NewHaulingRecoveryEscalation builds the deliverer. base is Hauling's origin
// plus its API base, e.g. "http://hauling:8104/api/v1/hauling".
func NewHaulingRecoveryEscalation(base, secret string, timeout time.Duration) *HaulingRecoveryEscalation {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &HaulingRecoveryEscalation{
		base: strings.TrimRight(strings.TrimSpace(base), "/"), secret: []byte(secret),
		http: &http.Client{Timeout: timeout},
		now:  func() time.Time { return time.Now().UTC() },
	}
}

// Configured reports whether the deliverer can reach Hauling.
func (h *HaulingRecoveryEscalation) Configured() bool {
	return h != nil && h.base != "" && len(h.secret) > 0
}

// Deliver sends the escalation.
func (h *HaulingRecoveryEscalation) Deliver(ctx context.Context,
	delivery ActionDelivery) (ActionOutcome, error) {
	if !h.Configured() {
		return ActionOutcome{}, errors.New("hauling recovery escalation is not configured")
	}
	body, err := json.Marshal(map[string]string{
		"operation_ref":  delivery.OperationRef,
		"trigger_code":   delivery.Parameters["trigger_code"],
		"correlation_id": delivery.Correlation,
	})
	if err != nil {
		return ActionOutcome{}, err
	}
	endpoint := fmt.Sprintf("%s/internal/recovery/bookings/%s/start",
		h.base, url.PathEscape(delivery.TargetRef))
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ActionOutcome{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	if err = serviceauth.SignRequest(request, supportcase.ContextCaller, h.secret, body, h.now()); err != nil {
		return ActionOutcome{}, err
	}
	response, err := h.http.Do(request)
	if err != nil {
		// Transport failure: the request may or may not have landed. Not an
		// answer, so not definite.
		return ActionOutcome{Definite: false, Code: "OWNER_UNREACHABLE"}, nil
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	var envelope struct {
		Data struct {
			RecoveryCaseID string `json:"recovery_case_id"`
		} `json:"data"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &envelope)
	switch {
	case response.StatusCode >= 200 && response.StatusCode < 300:
		return ActionOutcome{Definite: true, Accepted: true,
			ResultRef: envelope.Data.RecoveryCaseID, Code: "RECOVERY_STARTED"}, nil
	case response.StatusCode >= 400 && response.StatusCode < 500 &&
		response.StatusCode != http.StatusRequestTimeout && response.StatusCode != http.StatusTooManyRequests:
		// The owner answered and said no. That is final.
		code := strings.ToUpper(strings.TrimSpace(envelope.Error.Code))
		if code == "" {
			code = fmt.Sprintf("HTTP_%d", response.StatusCode)
		}
		return ActionOutcome{Definite: true, Accepted: false, Code: code}, nil
	default:
		return ActionOutcome{Definite: false, Code: fmt.Sprintf("HTTP_%d", response.StatusCode)}, nil
	}
}
