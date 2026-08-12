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

type Acceptance struct {
	AssignmentID, OrganizationID, ProviderID, ProviderService    string
	OrganizationMembershipID, ProviderResponseID, ResponseStatus string
}

type AcceptanceVerifier interface {
	Verify(context.Context, string) (Acceptance, error)
}

type HTTPAcceptanceVerifier struct {
	baseURL, caller string
	secret          []byte
	client          *http.Client
}

func NewHTTPAcceptanceVerifier(baseURL, caller string, secret []byte, timeout time.Duration) *HTTPAcceptanceVerifier {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &HTTPAcceptanceVerifier{strings.TrimRight(baseURL, "/"), caller, secret, &http.Client{Timeout: timeout}}
}

func (v *HTTPAcceptanceVerifier) Configured() bool {
	return v != nil && v.baseURL != "" && v.caller != "" && len(v.secret) > 0
}

func (v *HTTPAcceptanceVerifier) Verify(ctx context.Context, assignmentID string) (Acceptance, error) {
	if !v.Configured() {
		return Acceptance{}, ErrDisabled
	}
	endpoint := fmt.Sprintf("%s/internal/fleet-partner/v1/assignments/%s/provider-acceptance", v.baseURL, url.PathEscape(assignmentID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Acceptance{}, err
	}
	if err = serviceauth.SignRequest(req, v.caller, v.secret, nil, time.Now()); err != nil {
		return Acceptance{}, err
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return Acceptance{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Acceptance{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Acceptance{}, fmt.Errorf("acceptance verification status %d", resp.StatusCode)
	}
	var env struct {
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
	if json.Unmarshal(raw, &env) != nil {
		return Acceptance{}, errors.New("invalid acceptance response")
	}
	d := env.Data
	return Acceptance{d.AssignmentID, d.OrganizationID, d.ProviderID, d.ProviderService, d.OrganizationMembershipID, d.ProviderResponseID, d.ResponseStatus}, nil
}
