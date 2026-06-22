package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// This file adds the read/management calls an owning service needs to broker
// notification access for its end-user apps. Apps hold a customer/provider
// bearer token, not the notification HMAC secret, so customer-service and the
// provider services proxy these calls on the app's behalf.

// RealtimeTokenResult mirrors the notification-service realtime token response.
type RealtimeTokenResult struct {
	Token     string `json:"token"`
	ExpiresIn int64  `json:"expires_in"`
}

// DeviceInput registers a push device token with notification-service.
type DeviceInput struct {
	RecipientType string `json:"recipient_type"`
	RecipientID   string `json:"recipient_id"`
	Token         string `json:"token"`
	Platform      string `json:"platform"`
	App           string `json:"app"`
}

// ListMessages fetches the most recent notification messages for a recipient.
// The raw message objects from notification-service are returned as decoded
// JSON so the proxying service can forward them to the app without coupling to
// the notification message struct.
func (c Client) ListMessages(ctx context.Context, recipientType, recipientID string, limit int) ([]map[string]interface{}, error) {
	query := url.Values{}
	query.Set("recipient_type", recipientType)
	query.Set("recipient_id", recipientID)
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	path := "/api/v1/notifications/messages?" + query.Encode()

	var messages []map[string]interface{}
	if err := c.do(ctx, http.MethodGet, path, nil, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

// MintRealtimeToken requests a short-lived websocket token for a recipient.
func (c Client) MintRealtimeToken(ctx context.Context, recipientType, recipientID string) (RealtimeTokenResult, error) {
	body := map[string]string{"recipient_type": recipientType, "recipient_id": recipientID}
	var result RealtimeTokenResult
	if err := c.do(ctx, http.MethodPost, "/api/v1/notifications/realtime/token", body, &result); err != nil {
		return RealtimeTokenResult{}, err
	}
	return result, nil
}

// RegisterDevice forwards a push device token to notification-service.
func (c Client) RegisterDevice(ctx context.Context, input DeviceInput) error {
	return c.do(ctx, http.MethodPost, "/api/v1/notifications/devices", input, nil)
}

// do performs a signed request to notification-service and decodes the success
// envelope's data into out (when non-nil). It centralises HMAC signing, error
// envelope handling, and JSON decoding for the proxy calls above.
func (c Client) do(ctx context.Context, method, path string, payload interface{}, out interface{}) error {
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			return err
		}
	}

	endpoint := strings.TrimRight(c.BaseURL, "/") + path
	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader([]byte{})
	}
	httpRequest, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	if body != nil {
		httpRequest.Header.Set("Content-Type", "application/json")
	}
	if err := serviceauth.SignRequest(httpRequest, c.ServiceName, c.Secret, body, time.Now()); err != nil {
		return err
	}

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(httpRequest)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return err
	}
	if response.StatusCode >= http.StatusBadRequest || !envelope.Success {
		message := envelope.Error.Message
		if message == "" {
			message = fmt.Sprintf("notification request failed with status %d", response.StatusCode)
		}
		return errors.New(message)
	}
	if out != nil && len(envelope.Data) > 0 {
		return json.Unmarshal(envelope.Data, out)
	}
	return nil
}
