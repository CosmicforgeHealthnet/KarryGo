package privateexecution

import (
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

// Acceptance is Fleet's record that a provider agreed to this work.
type Acceptance struct {
	AssignmentID             string
	OrganizationID           string
	ProviderID               string
	ProviderService          string
	OrganizationMembershipID string
	ProviderResponseID       string
	ResponseStatus           string
}

// AcceptanceVerifier asks Fleet whether an assignment was really accepted.
type AcceptanceVerifier interface {
	Verify(ctx context.Context, assignmentID string) (Acceptance, error)
}

// HTTPAcceptanceVerifier reads Fleet's authoritative acceptance record.
//
// Taxi does not take Fleet's word for it from the request body. The body is
// what a caller claims; this is what Fleet actually holds. Without the second
// read, anything able to sign as Fleet could dispatch any driver for any
// organization by simply asserting an acceptance that never happened.
type HTTPAcceptanceVerifier struct {
	baseURL string
	caller  string
	secret  []byte
	client  *http.Client
}

// NewHTTPAcceptanceVerifier builds the verifier. baseURL is a bare Fleet
// origin; the internal path is appended here so a misconfigured base cannot
// silently point at a different surface.
func NewHTTPAcceptanceVerifier(baseURL, caller string, secret []byte, timeout time.Duration) *HTTPAcceptanceVerifier {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &HTTPAcceptanceVerifier{
		baseURL: strings.TrimRight(baseURL, "/"),
		caller:  caller,
		secret:  secret,
		client:  &http.Client{Timeout: timeout},
	}
}

// Configured reports whether the verifier can actually reach Fleet.
func (v *HTTPAcceptanceVerifier) Configured() bool {
	return v != nil && v.baseURL != "" && v.caller != "" && len(v.secret) > 0
}

// Verify reads one assignment's provider acceptance.
func (v *HTTPAcceptanceVerifier) Verify(ctx context.Context, assignmentID string) (Acceptance, error) {
	if !v.Configured() {
		return Acceptance{}, ErrDisabled
	}
	endpoint := fmt.Sprintf("%s/internal/fleet-partner/v1/assignments/%s/provider-acceptance",
		v.baseURL, url.PathEscape(assignmentID))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Acceptance{}, err
	}
	if err = serviceauth.SignRequest(request, v.caller, v.secret, nil, time.Now()); err != nil {
		return Acceptance{}, err
	}
	response, err := v.client.Do(request)
	if err != nil {
		return Acceptance{}, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return Acceptance{}, err
	}
	if response.StatusCode == http.StatusNotFound {
		return Acceptance{}, ErrNotFound
	}
	if response.StatusCode != http.StatusOK {
		return Acceptance{}, fmt.Errorf("acceptance verification returned status %d", response.StatusCode)
	}
	var envelope struct {
		Data struct {
			AssignmentID             string `json:"assignmentId"`
			OrganizationID           string `json:"organizationId"`
			ProviderID               string `json:"providerId"`
			ProviderService          string `json:"providerService"`
			OrganizationMembershipID string `json:"organizationMembershipId"`
			ProviderResponseID       string `json:"providerResponseId"`
			ResponseStatus           string `json:"responseStatus"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return Acceptance{}, errors.New("invalid acceptance response")
	}
	data := envelope.Data
	return Acceptance{
		AssignmentID: data.AssignmentID, OrganizationID: data.OrganizationID,
		ProviderID: data.ProviderID, ProviderService: data.ProviderService,
		OrganizationMembershipID: data.OrganizationMembershipID,
		ProviderResponseID:       data.ProviderResponseID,
		ResponseStatus:           data.ResponseStatus,
	}, nil
}
