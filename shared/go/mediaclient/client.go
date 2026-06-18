package mediaclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"
)

type Config struct {
	BaseURL     string
	ServiceName string
	Token       string
	HTTPClient  *http.Client
}

type Client struct {
	baseURL     string
	serviceName string
	token       string
	httpClient  *http.Client
}

func New(config Config) *Client {
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &Client{
		baseURL:     strings.TrimRight(config.BaseURL, "/"),
		serviceName: config.ServiceName,
		token:       config.Token,
		httpClient:  httpClient,
	}
}

type UploadRequest struct {
	OwnerID     string
	Purpose     string
	Filename    string
	ContentType string
	Body        io.Reader
	Metadata    map[string]interface{}
}

type MediaAsset struct {
	ID                string                 `json:"id"`
	OwnerService      string                 `json:"owner_service"`
	OwnerID           string                 `json:"owner_id"`
	Purpose           string                 `json:"purpose"`
	OriginalFilename  string                 `json:"original_filename"`
	ContentType       string                 `json:"content_type"`
	SizeBytes         int64                  `json:"size_bytes"`
	ChecksumSHA256    string                 `json:"checksum_sha256"`
	StorageBucket     string                 `json:"bucket"`
	StoragePath       string                 `json:"path"`
	PublicURL         string                 `json:"url"`
	Status            string                 `json:"status"`
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	UploadedByService string                 `json:"uploaded_by_service,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

type ListFilter struct {
	OwnerID string
	Purpose string
}

func (c *Client) Upload(ctx context.Context, upload UploadRequest) (MediaAsset, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err := writer.WriteField("owner_service", c.serviceName); err != nil {
		return MediaAsset{}, err
	}
	if err := writer.WriteField("owner_id", upload.OwnerID); err != nil {
		return MediaAsset{}, err
	}
	if err := writer.WriteField("purpose", upload.Purpose); err != nil {
		return MediaAsset{}, err
	}
	if len(upload.Metadata) > 0 {
		metadata, err := json.Marshal(upload.Metadata)
		if err != nil {
			return MediaAsset{}, err
		}
		if err := writer.WriteField("metadata", string(metadata)); err != nil {
			return MediaAsset{}, err
		}
	}

	filePart, err := writer.CreatePart(filePartHeader(upload.Filename, upload.ContentType))
	if err != nil {
		return MediaAsset{}, err
	}
	if _, err := io.Copy(filePart, upload.Body); err != nil {
		return MediaAsset{}, err
	}
	if err := writer.Close(); err != nil {
		return MediaAsset{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("/uploads"), &body)
	if err != nil {
		return MediaAsset{}, err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Accept", "application/json")
	c.authorize(request)

	var response envelope[MediaAsset]
	if err := c.do(request, &response); err != nil {
		return MediaAsset{}, err
	}

	return response.Data, nil
}

func (c *Client) GetByID(ctx context.Context, id string) (MediaAsset, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint("/files/"+url.PathEscape(id)), nil)
	if err != nil {
		return MediaAsset{}, err
	}
	request.Header.Set("Accept", "application/json")
	c.authorize(request)

	var response envelope[MediaAsset]
	if err := c.do(request, &response); err != nil {
		return MediaAsset{}, err
	}

	return response.Data, nil
}

func (c *Client) List(ctx context.Context, filter ListFilter) ([]MediaAsset, error) {
	endpoint, err := url.Parse(c.endpoint("/files"))
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	if filter.OwnerID != "" {
		query.Set("owner_id", filter.OwnerID)
	}
	if filter.Purpose != "" {
		query.Set("purpose", filter.Purpose)
	}
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	c.authorize(request)

	var response envelope[listResponse]
	if err := c.do(request, &response); err != nil {
		return nil, err
	}

	return response.Data.Files, nil
}

func (c *Client) Delete(ctx context.Context, id string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.endpoint("/files/"+url.PathEscape(id)), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	c.authorize(request)

	var response envelope[map[string]bool]
	return c.do(request, &response)
}

func (c *Client) endpoint(path string) string {
	if path == "" {
		return c.baseURL
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	return c.baseURL + path
}

func (c *Client) authorize(request *http.Request) {
	request.Header.Set("X-Karrygo-Service", c.serviceName)
	request.Header.Set("Authorization", "Bearer "+c.token)
}

func (c *Client) do(request *http.Request, output interface{}) error {
	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if err := json.NewDecoder(response.Body).Decode(output); err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ErrorFromEnvelope(output, response.StatusCode)
	}

	return nil
}

func filePartHeader(filename string, contentType string) textproto.MIMEHeader {
	if filename == "" {
		filename = "upload"
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, escapeQuotes(filename)))
	header.Set("Content-Type", contentType)
	return header
}

func escapeQuotes(value string) string {
	return strings.NewReplacer("\\", "\\\\", `"`, "\\\"").Replace(value)
}

type envelope[T any] struct {
	Success bool     `json:"success"`
	Data    T        `json:"data,omitempty"`
	Error   *Problem `json:"error,omitempty"`
}

type listResponse struct {
	Files []MediaAsset `json:"files"`
}

type Problem struct {
	Code      string       `json:"code"`
	Message   string       `json:"message"`
	RequestID string       `json:"request_id,omitempty"`
	Fields    []FieldError `json:"fields,omitempty"`
}

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type Error struct {
	StatusCode int
	Problem    Problem
}

func (e Error) Error() string {
	return fmt.Sprintf("media service error %d [%s]: %s", e.StatusCode, e.Problem.Code, e.Problem.Message)
}

func ErrorFromEnvelope(output interface{}, statusCode int) error {
	payload, err := json.Marshal(output)
	if err != nil {
		return Error{StatusCode: statusCode}
	}

	var parsed struct {
		Error Problem `json:"error"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return Error{StatusCode: statusCode}
	}

	return Error{StatusCode: statusCode, Problem: parsed.Error}
}
