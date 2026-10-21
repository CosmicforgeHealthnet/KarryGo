package analyticsadmin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/serviceauth"
)

const ProducerPublisherCaller = "owner-authorization-context-publisher"
const SourceContextPath = "/internal/analytics/v1/admin/source-contexts/"

// PublisherClient transports already signed owner envelopes. It holds no owner
// signing key and cannot issue an allocation or select missing policy values.
type PublisherClient struct {
	transport *Client
	owner     string
}

func NewPublisherClient(origin, owner string, key []byte) (*PublisherClient, error) {
	switch owner {
	case "driver-taxi-service", "driver-dispatch-delivery-service", "driver-hauling-service", "vendor-service":
	default:
		return nil, ErrInvalid
	}
	c, err := NewClient(origin, key)
	if err != nil {
		return nil, err
	}
	return &PublisherClient{transport: c, owner: owner}, nil
}
func (c *PublisherClient) Close() { c.transport.Close() }
func (c *PublisherClient) Publish(ctx context.Context, raw []byte) error {
	if len(raw) == 0 || len(raw) > 32<<10 {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if unique(d, 0) != nil {
		return ErrInvalid
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrInvalid
	}
	var envelope admincontext.Envelope
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&envelope) != nil || envelope.KeyID == "" || envelope.Signature == "" {
		return ErrInvalid
	}
	var approval admincontext.Approval
	decoder = json.NewDecoder(bytes.NewReader(envelope.Document))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&approval) != nil || approval.Owner != c.owner {
		return ErrInvalid
	}
	u := *c.transport.origin
	u.Path = SourceContextPath + c.owner
	u.RawPath = ""
	u.RawQuery = ""
	request, err := http.NewRequestWithContext(ctx, "POST", u.String(), bytes.NewReader(raw))
	if err != nil {
		return ErrInvalid
	}
	request.Header.Set("Content-Type", "application/json")
	if serviceauth.SignRequest(request, ProducerPublisherCaller, c.transport.secret, raw, time.Now()) != nil {
		return ErrUnavailable
	}
	response, err := c.transport.http.Do(request)
	if err != nil {
		return ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return ErrDenied
	}
	if response.StatusCode != 200 {
		return ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1025))
	if err != nil || len(body) > 1024 {
		return ErrInvalid
	}
	d = json.NewDecoder(bytes.NewReader(body))
	if unique(d, 0) != nil {
		return ErrInvalid
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrInvalid
	}
	var result struct {
		Status string `json:"status"`
	}
	decoder = json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || result.Status != "RECORDED" {
		return ErrInvalid
	}
	return nil
}
