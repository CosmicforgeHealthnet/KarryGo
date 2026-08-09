package platformclients

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// ReferenceCheck is the answer to "does this case exist in the authoritative
// service?".
//
// It deliberately carries no case content. Fleet asks only whether the
// identifier resolves; the complaint's text, complainant and status belong to
// the support-dispute service and to the people it authorizes, not to every
// organization that can type an identifier.
type ReferenceCheck struct {
	Exists bool
}

// ErrValidationEndpointAbsent means the authoritative service exposes no
// by-identifier lookup for this kind of reference.
//
// Distinct from ErrNotConfigured, and the distinction matters: "this
// deployment has no credentials" is an operational fault that must fail
// closed, while "the platform has no such endpoint" is a permanent structural
// fact that the caller answers by storing an explicitly unverified record.
var ErrValidationEndpointAbsent = errors.New("platformclients: no authoritative validation endpoint for this reference type")

// SafetyReferenceValidator confirms that an incident or dispute identifier
// resolves in the authoritative service.
type SafetyReferenceValidator interface {
	// ValidateIncident checks a complaint or safety incident identifier.
	ValidateIncident(ctx context.Context, meta CallMeta, reference string) (ReferenceCheck, error)
	// ValidateDispute checks a dispute identifier. Implementations that have
	// no by-identifier dispute endpoint return ErrValidationEndpointAbsent.
	ValidateDispute(ctx context.Context, meta CallMeta, reference string) (ReferenceCheck, error)
	// Configured reports whether live validation calls are possible.
	Configured() bool
}

// SupportDisputeReferenceClient validates references against
// support-dispute-service.
type SupportDisputeReferenceClient struct {
	client *Client
	// apiBase is the service's mounted API prefix. Kept configurable rather
	// than hard-coded so a gateway that mounts it elsewhere does not require a
	// code change.
	apiBase string
}

// SupportDisputeAPIBase is the prefix support-dispute-service mounts today.
const SupportDisputeAPIBase = "/api/v1/support-disputes"

// NewSupportDisputeReferenceClient builds the validator.
func NewSupportDisputeReferenceClient(opts Options) *SupportDisputeReferenceClient {
	return &SupportDisputeReferenceClient{client: New(opts), apiBase: SupportDisputeAPIBase}
}

// WithAPIBase overrides the mounted prefix.
func (c *SupportDisputeReferenceClient) WithAPIBase(base string) *SupportDisputeReferenceClient {
	if base != "" {
		c.apiBase = strings.TrimSuffix(base, "/")
	}
	return c
}

// Configured reports whether a base URL and secret are present.
func (c *SupportDisputeReferenceClient) Configured() bool {
	return c != nil && c.client.Configured()
}

// ValidateIncident asks support-dispute-service whether a complaint exists.
//
// It calls the admin/internal read, which is guarded by HMAC service
// authentication and a caller allowlist on that service's side. A 404 is a
// definitive "no": it is reported as a non-existent reference rather than an
// error, because the caller must be able to reject the write cleanly. Any
// other failure is returned as an error so the caller fails closed instead of
// recording an unverified reference it had no right to downgrade.
func (c *SupportDisputeReferenceClient) ValidateIncident(ctx context.Context, meta CallMeta,
	reference string) (ReferenceCheck, error) {
	if strings.TrimSpace(reference) == "" {
		return ReferenceCheck{}, nil
	}
	if !c.Configured() {
		return ReferenceCheck{}, ErrNotConfigured
	}

	path := c.apiBase + "/admin/complaints/" + url.PathEscape(reference)
	err := c.client.Do(ctx, meta, http.MethodGet, path, nil, nil)
	if err == nil {
		return ReferenceCheck{Exists: true}, nil
	}

	var clientErr *Error
	if errors.As(err, &clientErr) && clientErr.Outcome == OutcomeNotFound {
		return ReferenceCheck{Exists: false}, nil
	}
	return ReferenceCheck{}, err
}

// ValidateDispute reports that no authoritative by-identifier dispute lookup
// exists.
//
// support-dispute-service exposes disputes only as a list and as a child of a
// complaint (`/admin/complaints/:id/dispute`); there is no
// `/admin/disputes/:id`. Rather than guess a dispute's parent complaint — which
// would be fleet inventing a relationship it does not own — this returns
// ErrValidationEndpointAbsent, and the caller stores an explicitly unverified
// reference that authorizes nothing.
//
// When that endpoint is added, this method becomes a real call and existing
// UNVERIFIED rows can be re-checked; nothing else has to change.
func (c *SupportDisputeReferenceClient) ValidateDispute(context.Context, CallMeta,
	string) (ReferenceCheck, error) {
	return ReferenceCheck{}, ErrValidationEndpointAbsent
}

// Compile-time proof that the adapter satisfies the port.
var _ SafetyReferenceValidator = (*SupportDisputeReferenceClient)(nil)
