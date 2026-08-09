package platformclients

import (
	"context"
	"net/http"
	"net/url"
)

// RestrictionCommand is the versioned, idempotent command accepted by an
// authoritative execution service. External identifiers remain opaque.
type RestrictionCommand struct {
	RestrictionID    string `json:"restrictionId"`
	OrganizationID   string `json:"organizationId"`
	SubjectType      string `json:"subjectType"`
	SubjectReference string `json:"subjectReference"`
	ReasonCode       string `json:"reasonCode"`
	SourceReference  string `json:"sourceReference"`
	ExpectedVersion  int64  `json:"expectedVersion"`
}

type RestrictionOutcome struct {
	RestrictionID    string `json:"restrictionId"`
	State            string `json:"state"`
	SourceVersion    int64  `json:"sourceVersion"`
	SubjectType      string `json:"subjectType"`
	SubjectReference string `json:"subjectReference"`
}

type restrictionEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		RestrictionID  string `json:"restrictionId"`
		RequestedState string `json:"requestedState"`
		Outcome        string `json:"outcome"`
		SourceVersion  int64  `json:"sourceVersion"`
	} `json:"data"`
}

// RestrictionAuthority is implemented by Dispatch and Hauling adapters.
// Timed-out mutations must be followed by Read/ReadClear rather than assumed.
type RestrictionAuthority interface {
	Configured() bool
	Apply(context.Context, CallMeta, RestrictionCommand) (RestrictionOutcome, error)
	Read(context.Context, CallMeta, string) (RestrictionOutcome, error)
	Clear(context.Context, CallMeta, RestrictionCommand) (RestrictionOutcome, error)
	ReadClear(context.Context, CallMeta, string) (RestrictionOutcome, error)
}

type RestrictionClient struct {
	client *Client
	base   string
}

func NewRestrictionClient(client *Client, base string) *RestrictionClient {
	return &RestrictionClient{client: client, base: base}
}
func (c *RestrictionClient) Configured() bool {
	return c != nil && c.client != nil && c.client.Configured()
}
func (c *RestrictionClient) Apply(ctx context.Context, meta CallMeta, in RestrictionCommand) (RestrictionOutcome, error) {
	var out restrictionEnvelope
	err := c.client.Do(ctx, meta, http.MethodPost, c.base+"/apply", c.command(meta, in), &out)
	return outcome(out, in), err
}
func (c *RestrictionClient) Read(ctx context.Context, meta CallMeta, id string) (RestrictionOutcome, error) {
	var out restrictionEnvelope
	err := c.client.Do(ctx, meta, http.MethodGet, c.base+"/operations/"+url.PathEscape(id), nil, &out)
	return outcome(out, RestrictionCommand{RestrictionID: id}), err
}
func (c *RestrictionClient) Clear(ctx context.Context, meta CallMeta, in RestrictionCommand) (RestrictionOutcome, error) {
	var out restrictionEnvelope
	err := c.client.Do(ctx, meta, http.MethodPost, c.base+"/clear", c.command(meta, in), &out)
	return outcome(out, in), err
}

func (c *RestrictionClient) command(meta CallMeta, in RestrictionCommand) map[string]any {
	operation := meta.IdempotencyKey
	if operation == "" {
		operation = in.RestrictionID
	}
	return map[string]any{
		"operationReference": operation, "organizationId": in.OrganizationID,
		"subjectType": in.SubjectType, "subjectReference": in.SubjectReference,
		"reasonCode": in.ReasonCode, "sourceReference": in.RestrictionID,
		"sourceVersion": in.ExpectedVersion,
	}
}

func outcome(v restrictionEnvelope, in RestrictionCommand) RestrictionOutcome {
	return RestrictionOutcome{RestrictionID: v.Data.RestrictionID, State: v.Data.RequestedState,
		SourceVersion: v.Data.SourceVersion, SubjectType: in.SubjectType, SubjectReference: in.SubjectReference}
}
func (c *RestrictionClient) ReadClear(ctx context.Context, meta CallMeta, id string) (RestrictionOutcome, error) {
	return c.Read(ctx, meta, id)
}
