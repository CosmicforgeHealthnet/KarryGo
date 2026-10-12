package media

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

type Client struct {
	baseURL   string
	publicURL string
	secret    []byte
	http      *http.Client
}

func NewClient(baseURL, secret string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), secret: []byte(secret), http: &http.Client{Timeout: timeout}}
}

type Object struct {
	BindingType  string `json:"bindingType"`
	BindingID    string `json:"bindingId"`
	MIMEType     string `json:"detectedMime"`
	FileName     string `json:"fileName"`
	SizeBytes    int64  `json:"sizeBytes"`
	Checksum     string `json:"checksumSha256"`
	Visibility   string `json:"visibility"`
	OwnerService string `json:"ownerService"`

	ID             string `json:"mediaObjectId"`
	OwnerReference string `json:"ownerReference"`
	UploadPurpose  string `json:"uploadPurpose"`
	UploadStatus   string `json:"uploadStatus"`
	SafetyStatus   string `json:"safetyStatus"`
	DeletionState  string `json:"deletionState"`
	SourceVersion  int64  `json:"sourceVersion"`
}
type envelope struct {
	Success bool   `json:"success"`
	Data    Object `json:"data"`
}

func (c *Client) GetObject(ctx context.Context, businessID, ref string) (Object, error) {
	path := "/api/v1/media-files/internal/media/objects/" + url.PathEscape(ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return Object{}, err
	}
	req.Header.Set("X-Organization-ID", businessID)
	req.Header.Set("X-Service-Nonce", randomNonce())
	if err = serviceauth.SignRequest(req, "vendor-service", c.secret, nil, time.Now()); err != nil {
		return Object{}, err
	}
	client := *c.http
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(req)
	if err != nil {
		return Object{}, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return Object{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Object{}, fmt.Errorf("media authority returned status %d", response.StatusCode)
	}
	var out envelope
	if err = json.Unmarshal(raw, &out); err != nil {
		return Object{}, err
	}
	if !out.Success || out.Data.ID != ref || out.Data.OwnerReference != businessID || out.Data.SourceVersion < 1 {
		return Object{}, fmt.Errorf("media authority response is missing source metadata")
	}
	return out.Data, nil
}
