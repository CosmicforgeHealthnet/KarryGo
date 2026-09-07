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

// Dispatch relays only source-side pickup verification and opaque read-back.
type Dispatch struct {
	Origin  string
	Secret  []byte
	HTTP    *http.Client
	Enabled bool
}

func (a *Dispatch) Read(ctx context.Context, b Binding, stage, correlation string) (Observation, error) {
	if b.Authority != "DISPATCH" || !b.Valid() || b.StopRef == "" || (stage != "PICKUP" && stage != "DELIVERY") {
		return Observation{}, ErrInvalid
	}
	var data struct {
		MissionRef    string `json:"mission_ref"`
		HandoverRef   string `json:"handover_ref"`
		Stage         string `json:"stage"`
		Status        string `json:"status"`
		SourceVersion int64  `json:"source_version"`
	}
	path := "/missions/" + url.PathEscape(b.MissionRef) + "/stops/" + url.PathEscape(b.StopRef) + "/handover/" + stage
	if stage == "DELIVERY" {
		path = "/missions/" + url.PathEscape(b.MissionRef) + "/handover/" + stage
	}
	if err := a.call(ctx, http.MethodGet, path, nil, "", correlation, &data); err != nil {
		return Observation{}, err
	}
	o := Observation{Binding: b, HandoverRef: data.HandoverRef, Stage: data.Stage, Status: data.Status, SourceVersion: data.SourceVersion}
	return validateObservation(o, b, stage)
}

func (a *Dispatch) ConfirmPickup(ctx context.Context, c Confirmation) (Observation, error) {
	if c.Authority != "DISPATCH" || c.StopRef == "" || c.ManifestRef == "" || c.ManifestVersion < 1 {
		return Observation{}, ErrInvalid
	}
	body := struct {
		ChallengeRef    string `json:"challenge_ref"`
		Method          string `json:"method"`
		Credential      string `json:"credential"`
		ManifestRef     string `json:"manifest_ref"`
		ManifestVersion int64  `json:"manifest_version"`
	}{c.HandoverRef, c.Method, c.Credential, c.ManifestRef, c.ManifestVersion}
	path := "/missions/" + url.PathEscape(c.MissionRef) + "/stops/" + url.PathEscape(c.StopRef) + "/pickup/verify"
	var ignored json.RawMessage
	if err := a.call(ctx, http.MethodPost, path, body, c.OperationRef, c.CorrelationID, &ignored); err != nil {
		return Observation{}, err
	}
	// Re-read even after 2xx; only the authority's persisted state proves
	// custody, and this same path reconciles a lost response.
	return a.Read(ctx, c.Binding, "PICKUP", c.CorrelationID)
}

func (a *Dispatch) call(ctx context.Context, method, path string, input any, operation, correlation string, output any) error {
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
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(a.Origin, "/")+"/api/v1/internal/commerce/dispatch"+path, bytes.NewReader(body))
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
	if raw, ok := output.(*json.RawMessage); ok {
		*raw = append((*raw)[:0], envelope.Data...)
		return nil
	}
	if json.Unmarshal(envelope.Data, output) != nil {
		return ErrUnknown
	}
	return nil
}
