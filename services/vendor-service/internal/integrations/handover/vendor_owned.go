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

// VendorOwned talks to the Dispatch-owned VendorMission custody authority.
// It never receives issued secrets; Notification delivers them to the bound
// provider or receiver.
type VendorOwned struct {
	Origin  string
	Secret  []byte
	HTTP    *http.Client
	Enabled bool
}

func (a *VendorOwned) Bind(ctx context.Context, b Binding, operation, correlation string) (Observation, error) {
	if b.Authority != "VENDOR_OWN_DELIVERY" || !b.Valid() || b.BusinessID == "" || b.BranchID == "" || b.ProviderRef == "" || b.ReceiverRef == "" || b.ManifestRef == "" || b.ManifestVersion < 1 {
		return Observation{}, ErrInvalid
	}
	body := map[string]any{"mission_ref": b.MissionRef, "fulfilment_ref": b.FulfilmentRef, "business_id": b.BusinessID, "branch_id": b.BranchID, "provider_ref": b.ProviderRef, "receiver_ref": b.ReceiverRef, "receiver_phone": b.ReceiverPhone, "manifest_ref": b.ManifestRef, "manifest_version": b.ManifestVersion, "parcel_refs": b.ParcelRefs}
	var data authorityObservation
	err := a.call(ctx, http.MethodPost, "", body, operation, correlation, &data)
	if err == ErrRejected {
		// Vendor's assignment/receiver commands commit in their owning service.
		// Reconcile those exact frozen bindings through explicit authority
		// commands; neither endpoint can alter manifest or tenant scope.
		changePath := "/" + url.PathEscape(b.MissionRef) + "/reassignments"
		change := map[string]any{"provider_ref": b.ProviderRef}
		changeErr := a.call(ctx, http.MethodPost, changePath, change, "reassign:"+b.FulfilmentRef+":"+b.ProviderRef, correlation, &data)
		if changeErr == ErrRejected {
			changePath = "/" + url.PathEscape(b.MissionRef) + "/receiver-changes"
			change = map[string]any{"receiver_ref": b.ReceiverRef, "receiver_phone": b.ReceiverPhone}
			changeErr = a.call(ctx, http.MethodPost, changePath, change, "receiver:"+b.FulfilmentRef+":"+b.ReceiverRef, correlation, &data)
		}
		if changeErr != nil {
			return Observation{}, err
		}
		err = a.call(ctx, http.MethodPost, "", body, operation, correlation, &data)
	}
	if err != nil {
		return Observation{}, err
	}
	return data.toVendor(b)
}
func (a *VendorOwned) Read(ctx context.Context, b Binding, stage, correlation string) (Observation, error) {
	if b.Authority != "VENDOR_OWN_DELIVERY" || !b.Valid() || (stage != "PICKUP" && stage != "DELIVERY") {
		return Observation{}, ErrInvalid
	}
	if _, err := a.Bind(ctx, b, "bind:"+b.FulfilmentRef, correlation); err != nil && err != ErrRejected {
		return Observation{}, err
	}
	var data authorityObservation
	err := a.call(ctx, http.MethodGet, "/"+url.PathEscape(b.MissionRef)+"/handover/"+stage, nil, "", correlation, &data)
	if err != nil {
		return Observation{}, err
	}
	return data.toVendor(b)
}
func (a *VendorOwned) ConfirmPickup(ctx context.Context, c Confirmation) (Observation, error) {
	if c.Authority != "VENDOR_OWN_DELIVERY" || c.ProviderRef == "" || c.ManifestRef == "" || c.ManifestVersion < 1 {
		return Observation{}, ErrInvalid
	}
	if _, err := a.Bind(ctx, c.Binding, "bind:"+c.FulfilmentRef, c.CorrelationID); err != nil && err != ErrRejected {
		return Observation{}, err
	}
	body := map[string]any{"challenge_ref": c.HandoverRef, "provider_ref": c.ProviderRef, "method": c.Method, "credential": c.Credential, "manifest_ref": c.ManifestRef, "manifest_version": c.ManifestVersion}
	var data authorityObservation
	if err := a.call(ctx, http.MethodPost, "/"+url.PathEscape(c.MissionRef)+"/handover/PICKUP/confirm", body, c.OperationRef, c.CorrelationID, &data); err != nil {
		return Observation{}, err
	}
	return a.Read(ctx, c.Binding, "PICKUP", c.CorrelationID)
}
func (a *VendorOwned) ConfirmDelivery(ctx context.Context, b Binding, handoverRef, method, credential, operation, correlation string) (Observation, error) {
	body := map[string]any{"challenge_ref": handoverRef, "provider_ref": b.ProviderRef, "method": method, "credential": credential, "manifest_ref": b.ManifestRef, "manifest_version": b.ManifestVersion}
	var data authorityObservation
	if err := a.call(ctx, http.MethodPost, "/"+url.PathEscape(b.MissionRef)+"/handover/DELIVERY/confirm", body, operation, correlation, &data); err != nil {
		return Observation{}, err
	}
	return a.Read(ctx, b, "DELIVERY", correlation)
}

type authorityObservation struct {
	MissionRef    string `json:"mission_ref"`
	HandoverRef   string `json:"handover_ref"`
	Stage         string `json:"stage"`
	Status        string `json:"status"`
	SourceVersion int64  `json:"source_version"`
}

func (d authorityObservation) toVendor(b Binding) (Observation, error) {
	return validateObservation(Observation{Binding: b, HandoverRef: d.HandoverRef, Stage: d.Stage, Status: d.Status, SourceVersion: d.SourceVersion}, b, d.Stage)
}
func (a *VendorOwned) call(ctx context.Context, method, path string, input any, operation, correlation string, out any) error {
	if a == nil || !a.Enabled || a.Origin == "" || len(a.Secret) == 0 {
		return ErrUnavailable
	}
	var body []byte
	var err error
	if input != nil {
		body, err = json.Marshal(input)
		if err != nil {
			return ErrInvalid
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(a.Origin, "/")+"/api/v1/internal/commerce/vendor-owned/missions"+path, bytes.NewReader(body))
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
	res, err := client.Do(req)
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
	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(&env) != nil || !env.Success {
		return ErrUnknown
	}
	if json.Unmarshal(env.Data, out) != nil {
		return ErrUnknown
	}
	return nil
}
