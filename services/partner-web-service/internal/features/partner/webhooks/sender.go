package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"
)

// Sender delivers a signed webhook payload to a partner endpoint. It performs a
// single attempt; retries are the worker's responsibility.
type Sender interface {
	Send(ctx context.Context, w Webhook, d WebhookDelivery, plaintextSecret string, payload WebhookPayload, now time.Time) (statusCode int, err error)
}

// HTTPSender is the production Sender over net/http.
type HTTPSender struct {
	client *http.Client
}

// NewHTTPSender builds an HTTPSender with a 10s timeout that refuses to follow
// https->http downgrade redirects.
func NewHTTPSender() *HTTPSender {
	return &HTTPSender{
		client: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if req.URL.Scheme != "https" {
					return errors.New("refusing insecure redirect to non-https URL")
				}
				if len(via) >= 5 {
					return errors.New("too many redirects")
				}
				return nil
			},
		},
	}
}

// Send marshals the payload once, signs timestamp + "." + raw body, sets the
// X-KarryGo-* headers, and reports the response status. Non-2xx is an error.
func (s *HTTPSender) Send(ctx context.Context, w Webhook, d WebhookDelivery, plaintextSecret string, payload WebhookPayload, now time.Time) (int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	ts := now.UTC().Unix()
	signature := SignPayload(plaintextSecret, ts, body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderSignature, signature)
	req.Header.Set(HeaderEvent, payload.Event)
	req.Header.Set(HeaderDelivery, d.ID)
	req.Header.Set(HeaderTimestamp, strconv.FormatInt(ts, 10))

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, errors.New("webhook endpoint returned non-2xx status")
	}
	return resp.StatusCode, nil
}
