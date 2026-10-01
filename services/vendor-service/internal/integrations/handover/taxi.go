package handover

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// Taxi uses the existing Taxi Goods HMAC boundary. It never calls passenger
// booking APIs or returns the upstream mission's commercial/customer fields.
type Taxi struct {
	Origin  string
	Secret  []byte
	HTTP    *http.Client
	Enabled bool
}

func (a *Taxi) Read(ctx context.Context, b Binding, stage, correlation string) (Observation, error) {
	if b.Authority != "TAXI" || !b.Valid() || (stage != "PICKUP" && stage != "DELIVERY") {
		return Observation{}, ErrInvalid
	}
	var data struct {
		MissionRef    string `json:"mission_ref"`
		HandoverRef   string `json:"handover_ref"`
		Stage         string `json:"stage"`
		Status        string `json:"status"`
		SourceVersion int64  `json:"source_version"`
	}
	err := a.call(ctx, http.MethodGet, "/missions/"+url.PathEscape(b.MissionRef)+"/handover/"+stage, nil, "", correlation, &data)
	if err != nil {
		return Observation{}, err
	}
	if data.MissionRef != b.MissionRef {
		return Observation{}, ErrInvalid
	}
	return validateObservation(Observation{Binding: b, HandoverRef: data.HandoverRef, Stage: data.Stage, Status: data.Status, SourceVersion: data.SourceVersion}, b, stage)
}

func (a *Taxi) ConfirmPickup(ctx context.Context, c Confirmation) (Observation, error) {
	if c.Authority != "TAXI" {
		return Observation{}, ErrInvalid
	}
	// Credential encoding is confined to this one transient authority request.
	body := struct {
		ChallengeRef string `json:"challenge_ref"`
		Method       string `json:"method"`
		Credential   string `json:"credential"`
		OperationRef string `json:"operation_ref"`
	}{c.HandoverRef, c.Method, c.Credential, c.OperationRef}
	var data struct {
		MissionRef string `json:"mission_ref"`
	}
	err := a.call(ctx, http.MethodPost, "/missions/"+url.PathEscape(c.MissionRef)+"/pickup/verify", body, c.OperationRef, c.CorrelationID, &data)
	if err != nil {
		return Observation{}, err
	}
	if data.MissionRef != c.MissionRef {
		return Observation{}, ErrUnknown
	}
	// A transport 2xx is not evidence of custody. Read the exact authority
	// challenge, including evidence-gated consumption, before projecting.
	o, err := a.Read(ctx, c.Binding, "PICKUP", c.CorrelationID)
	if err != nil {
		return Observation{}, ErrUnknown
	}
	return o, nil
}

func (a *Taxi) call(ctx context.Context, method, path string, input any, operation, correlation string, output any) error {
	if a == nil || !a.Enabled || a.Origin == "" || len(a.Secret) == 0 {
		return ErrUnavailable
	}
	var body []byte
	if input != nil {
		var err error
		body, err = json.Marshal(input)
		if err != nil {
			return ErrInvalid
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(a.Origin, "/")+"/api/v1/taxi/internal/commerce"+path, bytes.NewReader(body))
	if err != nil {
		return ErrUnavailable
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", operation)
	req.Header.Set("X-Correlation-ID", correlation)
	if serviceauth.SignRequest(req, "vendor-service", a.Secret, body, time.Now()) != nil {
		return ErrUnavailable
	}
	client := a.HTTP
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	bounded := *client
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := bounded.Do(req)
	if err != nil {
		return ErrUnknown
	}
	defer res.Body.Close()
	if res.StatusCode >= 500 {
		return ErrUnknown
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return ErrRejected
	}
	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&envelope) != nil || !envelope.Success {
		return ErrUnknown
	}
	if json.Unmarshal(envelope.Data, output) != nil {
		return ErrUnknown
	}
	return nil
}
