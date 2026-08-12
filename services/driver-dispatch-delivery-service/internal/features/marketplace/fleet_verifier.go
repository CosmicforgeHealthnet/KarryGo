package marketplace

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

// HTTPAcceptanceVerifier is the REAL provider-acceptance verifier.
//
// It replaces the stub with an authenticated call to fleet-partner-service.
// This is the component that makes "no canonical trip without a real provider
// acceptance" true in production rather than only in tests.
type HTTPAcceptanceVerifier struct {
	baseURL     string
	serviceName string
	secret      []byte
	client      *http.Client
}

// VerifierConfig configures the real verifier.
type VerifierConfig struct {
	// BaseURL of fleet-partner-service, e.g. http://fleet-partner:8080
	BaseURL string
	// ServiceName is THIS service's identity as fleet knows it.
	ServiceName string
	// Secret is the shared HMAC secret for that identity.
	Secret []byte
	// Timeout bounds the call. A verification that hangs must fail closed
	// rather than stall trip creation indefinitely.
	Timeout time.Duration
}

// NewHTTPAcceptanceVerifier builds the real verifier, or reports that
// configuration is incomplete so the caller can fall back to the fail-closed
// UnconfiguredVerifier rather than silently proceeding.
func NewHTTPAcceptanceVerifier(config VerifierConfig) (AcceptanceVerifier, bool) {
	if strings.TrimSpace(config.BaseURL) == "" ||
		strings.TrimSpace(config.ServiceName) == "" || len(config.Secret) == 0 {
		return UnconfiguredVerifier{}, false
	}
	if _, err := url.Parse(config.BaseURL); err != nil {
		return UnconfiguredVerifier{}, false
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &HTTPAcceptanceVerifier{
		baseURL:     strings.TrimRight(config.BaseURL, "/"),
		serviceName: config.ServiceName,
		secret:      config.Secret,
		client:      &http.Client{Timeout: timeout},
	}, true
}

// Configured reports true: this verifier can actually reach fleet.
func (v *HTTPAcceptanceVerifier) Configured() bool { return true }

// fleetAcceptanceEnvelope is fleet's canonical {data} response.
type fleetAcceptanceEnvelope struct {
	Data struct {
		AssignmentID             string     `json:"assignmentId"`
		OrganizationID           string     `json:"organizationId"`
		ProviderID               string     `json:"providerId"`
		ProviderService          string     `json:"providerService"`
		OrganizationMembershipID string     `json:"organizationMembershipId"`
		ProviderResponseID       string     `json:"providerResponseId"`
		ResponseStatus           string     `json:"responseStatus"`
		AcceptedAt               *time.Time `json:"acceptedAt"`
		AssignmentStatus         string     `json:"assignmentStatus"`
		AssignmentVersion        int64      `json:"assignmentVersion"`
		SourceService            string     `json:"sourceService"`
		SourceRequestID          string     `json:"sourceRequestId"`
		SourceBookingID          string     `json:"sourceBookingId"`
	} `json:"data"`
}

// fleetErrorEnvelope is fleet's canonical {error} response.
type fleetErrorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// VerifyProviderAcceptance asks fleet whether the provider accepted.
//
// Failure classification matters here more than anywhere else in this service:
//   - a definite "no" must stop trip creation permanently;
//   - an unreachable fleet must stop it TEMPORARILY, never approve it.
//
// Both outcomes refuse to create a trip, so an outage can never manufacture
// consent — it only delays work that a provider genuinely accepted.
func (v *HTTPAcceptanceVerifier) VerifyProviderAcceptance(ctx context.Context, meta CallMeta,
	assignmentID string) (*ProviderAcceptance, error) {
	if strings.TrimSpace(assignmentID) == "" {
		return nil, ErrFleetAssignmentNotFound
	}

	endpoint := fmt.Sprintf("%s/internal/fleet-partner/v1/assignments/%s/provider-acceptance",
		v.baseURL, url.PathEscape(assignmentID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, ErrFleetUnavailable
	}
	req.Header.Set("Accept", "application/json")
	if meta.RequestID != "" {
		req.Header.Set(HeaderRequestID, meta.RequestID)
	}
	if meta.CorrelationID != "" {
		req.Header.Set(HeaderCorrelationID, meta.CorrelationID)
	}
	// A GET has no body; the signature still covers method, path and timestamp.
	if err := serviceauth.SignRequest(req, v.serviceName, v.secret, nil, time.Now()); err != nil {
		return nil, ErrFleetUnavailable
	}

	resp, err := v.client.Do(req)
	if err != nil {
		// Timeout, DNS failure, connection reset: unknown, never a rejection.
		return nil, ErrFleetUnavailable
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	// Bound the body: a hostile or broken peer must not be able to exhaust
	// memory through this path.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, ErrFleetUnavailable
	}

	switch {
	case resp.StatusCode == http.StatusOK:
		// fall through to parsing
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrFleetAssignmentNotFound
	case resp.StatusCode == http.StatusConflict:
		return nil, classifyFleetConflict(body)
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		// Misconfiguration on our side. Fail closed and make it loud, but do
		// not treat it as the provider having declined.
		return nil, ErrFleetUnavailable
	default:
		return nil, ErrFleetUnavailable
	}

	var envelope fleetAcceptanceEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, ErrFleetUnavailable
	}
	data := envelope.Data

	// Validate the payload before trusting any of it. A response that omits
	// the binding identifiers is not usable, and treating a zero value as a
	// match would defeat every downstream comparison.
	if data.AssignmentID == "" || data.ProviderID == "" ||
		data.ProviderResponseID == "" || data.OrganizationID == "" {
		return nil, ErrAcceptanceMismatch
	}
	if data.AssignmentID != assignmentID {
		return nil, ErrAcceptanceMismatch
	}
	if data.ResponseStatus != "ACCEPTED" {
		return nil, ErrAcceptanceNotConfirmed
	}
	// A provider from another service must never satisfy a dispatch execution.
	if data.ProviderService != "DISPATCH" {
		return nil, ErrAcceptanceMismatch
	}

	return &ProviderAcceptance{
		AssignmentID:             data.AssignmentID,
		OrganizationID:           data.OrganizationID,
		ProviderID:               data.ProviderID,
		OrganizationMembershipID: data.OrganizationMembershipID,
		ProviderResponseID:       data.ProviderResponseID,
		ResponseStatus:           data.ResponseStatus,
		AcceptedAt:               data.AcceptedAt,
		AssignmentStatus:         data.AssignmentStatus,
		AssignmentVersion:        data.AssignmentVersion,
		SourceRequestID:          data.SourceRequestID,
	}, nil
}

// classifyFleetConflict turns fleet's stable error code into a typed error.
//
// The distinction is not cosmetic: "cancelled" and "expired" are permanent and
// must stop retrying, while an unrecognised conflict is treated as
// not-confirmed so the caller still refuses to create a trip.
func classifyFleetConflict(body []byte) error {
	var envelope fleetErrorEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ErrAcceptanceNotConfirmed
	}
	switch envelope.Error.Code {
	case "fleet_assignment_not_found":
		return ErrFleetAssignmentNotFound
	case "provider_acceptance_mismatch":
		return ErrAcceptanceMismatch
	case "provider_acceptance_cancelled":
		return ErrAcceptanceCancelled
	case "provider_acceptance_expired":
		return ErrAcceptanceExpired
	case "provider_acceptance_not_confirmed":
		return ErrAcceptanceNotConfirmed
	default:
		return ErrAcceptanceNotConfirmed
	}
}

// Permanent acceptance failures. These must not be retried: the assignment
// will never become executable.
var (
	ErrAcceptanceCancelled = errors.New("marketplace: provider acceptance cancelled")
	ErrAcceptanceExpired   = errors.New("marketplace: provider acceptance expired")
)
