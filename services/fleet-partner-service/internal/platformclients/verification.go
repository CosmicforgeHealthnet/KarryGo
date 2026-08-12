package platformclients

import (
	"context"
	"net/http"
	"net/url"
)

type VerificationClient struct {
	client  *Client
	apiBase string
}

func NewVerificationClient(opts Options) *VerificationClient {
	return &VerificationClient{client: New(opts), apiBase: "/api/v1/verification-compliance/internal/organizations"}
}

func (c *VerificationClient) Configured() bool { return c != nil && c.client.Configured() }

type verificationEnvelope struct {
	Data VerificationOutcome `json:"data"`
}

func (c *VerificationClient) SubmitOrganization(ctx context.Context, meta CallMeta, submission VerificationSubmission) (VerificationOutcome, error) {
	if !c.Configured() {
		return VerificationOutcome{}, ErrNotConfigured
	}
	meta.OrganizationID = submission.OrganizationID
	meta.IdempotencyKey = submission.IdempotencyKey
	var out verificationEnvelope
	err := c.client.Do(ctx, meta, http.MethodPost, c.apiBase+"/submissions", map[string]any{
		"organizationId":  submission.OrganizationID,
		"submissionId":    submission.SubmissionID,
		"snapshot":        submission.Snapshot,
		"documentFileIds": submission.DocumentFileIDs,
	}, &out)
	return out.Data, err
}

func (c *VerificationClient) ReadOrganizationStatus(ctx context.Context, meta CallMeta, organizationID, submissionID string) (VerificationOutcome, error) {
	if !c.Configured() {
		return VerificationOutcome{}, ErrNotConfigured
	}
	var out verificationEnvelope
	err := c.client.Do(ctx, meta, http.MethodGet, c.apiBase+"/"+url.PathEscape(organizationID)+"/submissions/"+url.PathEscape(submissionID), nil, &out)
	return out.Data, err
}

var _ VerificationContract = (*VerificationClient)(nil)
