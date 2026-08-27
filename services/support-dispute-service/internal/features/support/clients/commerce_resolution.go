package supportclients

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

type CommerceResolutionPublisher interface {
	PublishResolution(context.Context, supportcase.ResolutionEvent) error
}

type CommerceResolutionClient struct {
	baseURL string
	secret  []byte
	http    *http.Client
	now     func() time.Time
}

func NewCommerceResolutionClient(baseURL, secret string) *CommerceResolutionClient {
	return &CommerceResolutionClient{baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), secret: []byte(secret), http: &http.Client{Timeout: 5 * time.Second}, now: time.Now}
}
func (c *CommerceResolutionClient) PublishResolution(ctx context.Context, event supportcase.ResolutionEvent) error {
	if c == nil || c.baseURL == "" || len(c.secret) == 0 {
		return errors.New("vendor commerce resolution target is not configured")
	}
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/internal/vendor/commerce/support-case-resolutions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if err = serviceauth.SignRequest(req, "support-dispute-service", c.secret, body, c.now().UTC()); err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("vendor resolution callback returned %d", resp.StatusCode)
	}
	return nil
}
