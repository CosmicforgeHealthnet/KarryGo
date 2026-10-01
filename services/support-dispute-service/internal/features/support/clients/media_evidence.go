package supportclients

// Section 15/16: asking Media whether a file can be relied on as evidence.
//
// Support never receives the file, a storage location or a download link --
// only Media's verdict that the object is active, safe, of an evidence
// purpose and bound to what the complaint is about. An agent who needs to
// look at the file asks Media for access through Media's own authorization.

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
	"cosmicforge/logistics/shared/go/supportcase"
)

// ErrMediaUnavailable: Media could not be asked. Not a verdict.
var ErrMediaUnavailable = errors.New("media service unavailable")

// EvidencePurposes are the Section 16 purposes a complaint may cite.
var EvidencePurposes = []string{"DAMAGE_EVIDENCE", "RECOVERY_TRANSFER", "CARGO_CONDITION", "SAFETY_INCIDENT",
	"PROOF_IMAGE", "SIGNATURE", "DISPUTE_ATTACHMENT"}

// EvidenceVerdict is Media's answer.
type EvidenceVerdict struct {
	Usable   bool     `json:"usable"`
	Status   string   `json:"status"`
	Purpose  string   `json:"purpose"`
	Failures []string `json:"failures"`
}

// MediaEvidenceValidator asks media-file-service's verdict-only route.
type MediaEvidenceValidator struct {
	base   string
	secret []byte
	http   *http.Client
}

// NewMediaEvidenceValidator builds the validator. origin is Media's bare origin.
func NewMediaEvidenceValidator(origin, secret string, timeout time.Duration) *MediaEvidenceValidator {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &MediaEvidenceValidator{base: strings.TrimRight(strings.TrimSpace(origin), "/"),
		secret: []byte(secret), http: &http.Client{Timeout: timeout}}
}

// Configured reports whether Media can be asked.
func (v *MediaEvidenceValidator) Configured() bool {
	return v != nil && v.base != "" && len(v.secret) > 0
}

// Validate asks whether objectID is usable evidence bound to bindingType/bindingID.
func (v *MediaEvidenceValidator) Validate(ctx context.Context, objectID, bindingType, bindingID string) (EvidenceVerdict, error) {
	if !v.Configured() {
		return EvidenceVerdict{}, ErrMediaUnavailable
	}
	body, err := json.Marshal(map[string]any{
		"mediaObjectId": objectID, "bindingType": bindingType, "bindingId": bindingID,
		"purposes": EvidencePurposes,
	})
	if err != nil {
		return EvidenceVerdict{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost,
		v.base+"/api/v1/media-files/internal/media/evidence-validations", bytes.NewReader(body))
	if err != nil {
		return EvidenceVerdict{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	if err = serviceauth.SignRequest(request, supportcase.ContextCaller, v.secret, body, time.Now().UTC()); err != nil {
		return EvidenceVerdict{}, err
	}
	response, err := v.http.Do(request)
	if err != nil {
		return EvidenceVerdict{}, fmt.Errorf("%w: %v", ErrMediaUnavailable, err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if response.StatusCode != http.StatusOK {
		return EvidenceVerdict{}, fmt.Errorf("%w: media answered %d", ErrMediaUnavailable, response.StatusCode)
	}
	var envelope struct {
		Data EvidenceVerdict `json:"data"`
	}
	if err = json.Unmarshal(raw, &envelope); err != nil {
		return EvidenceVerdict{}, fmt.Errorf("%w: unreadable verdict", ErrMediaUnavailable)
	}
	return envelope.Data, nil
}
