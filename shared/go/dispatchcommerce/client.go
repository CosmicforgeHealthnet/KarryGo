package dispatchcommerce

import (
	"bytes"
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

var (
	ErrNotConfigured        = errors.New("dispatch commerce client is not configured")
	ErrAuthorityUnavailable = errors.New("dispatch commerce authority is unavailable")
	ErrInvalidRequest       = errors.New("dispatch commerce request is invalid")
	ErrConflict             = errors.New("dispatch commerce idempotency conflict")

	// ErrMissionNotFound means Dispatch answered, and its answer is that it
	// holds no such operation.
	//
	// It also satisfies ErrInvalidRequest so no existing caller changes
	// behaviour. It exists because a reconciler resolving an ambiguous create
	// has to distinguish "definitely absent" -- the one state in which sending
	// the create again is safe -- from every other 4xx.
	ErrMissionNotFound = errors.New("dispatch commerce holds no such mission")
)

type Client struct {
	baseURL string
	secret  []byte
	http    *http.Client
	now     func() time.Time
}

type ClientOptions struct {
	BaseURL    string
	Secret     string
	Timeout    time.Duration
	HTTPClient *http.Client
}

func NewClient(options ClientOptions) *Client {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	return &Client{baseURL: strings.TrimRight(strings.TrimSpace(options.BaseURL), "/"), secret: []byte(options.Secret), http: httpClient, now: time.Now}
}

func (c *Client) Configured() bool {
	return c != nil && c.baseURL != "" && len(c.secret) > 0 && c.http != nil
}

func (c *Client) Quote(ctx context.Context, request QuoteRequest) (Quote, error) {
	if reason, ok := request.Validate(); !ok {
		return Quote{}, fmt.Errorf("%w: %s", ErrInvalidRequest, reason)
	}
	return do[Quote](c, ctx, http.MethodPost, QuotePath, request, request.QuoteReference)
}

func (c *Client) Capability(ctx context.Context) (MotorcycleCapability, error) {
	return do[MotorcycleCapability](c, ctx, http.MethodGet, CapabilityPath, nil, "")
}

func (c *Client) CreateMission(ctx context.Context, request MissionCreateRequest) (Mission, error) {
	if reason, ok := request.Validate(); !ok {
		return Mission{}, fmt.Errorf("%w: %s", ErrInvalidRequest, reason)
	}
	return do[Mission](c, ctx, http.MethodPost, MissionPath, request, request.OperationRef)
}

func (c *Client) ReadOperation(ctx context.Context, operationRef string) (Mission, error) {
	operationRef = strings.TrimSpace(operationRef)
	if operationRef == "" {
		return Mission{}, ErrInvalidRequest
	}
	return do[Mission](c, ctx, http.MethodGet, MissionPath+"/operations/"+url.PathEscape(operationRef), nil, "")
}

func (c *Client) ReadMission(ctx context.Context, missionRef string) (Mission, error) {
	missionRef = strings.TrimSpace(missionRef)
	if missionRef == "" {
		return Mission{}, ErrInvalidRequest
	}
	return do[Mission](c, ctx, http.MethodGet, MissionPath+"/"+url.PathEscape(missionRef), nil, "")
}

func (c *Client) CancelMission(ctx context.Context, missionRef string, request CommandRequest) (Mission, error) {
	if strings.TrimSpace(missionRef) == "" || strings.TrimSpace(request.OperationRef) == "" {
		return Mission{}, ErrInvalidRequest
	}
	return do[Mission](c, ctx, http.MethodPost, MissionPath+"/"+url.PathEscape(missionRef)+"/cancel", request, request.OperationRef)
}

func do[T any](c *Client, ctx context.Context, method, path string, request any, idempotencyKey string) (T, error) {
	var zero T
	if !c.Configured() {
		return zero, ErrNotConfigured
	}
	var body []byte
	var err error
	if request != nil {
		body, err = json.Marshal(request)
		if err != nil {
			return zero, fmt.Errorf("%w: encode", ErrInvalidRequest)
		}
	}
	httpRequest, err := http.NewRequestWithContext(ctx, method, c.baseURL+APIBase+path, bytes.NewReader(body))
	if err != nil {
		return zero, fmt.Errorf("%w: build", ErrAuthorityUnavailable)
	}
	if method != http.MethodGet {
		// Mutating requests are reconciled by their stable operation reference.
		// Disable net/http's transparent replay so an unknown response always
		// reaches the caller's authoritative read-back path first.
		httpRequest.GetBody = nil
	}
	httpRequest.Header.Set("Accept", "application/json")
	if request != nil {
		httpRequest.Header.Set("Content-Type", "application/json")
	}
	if idempotencyKey != "" {
		httpRequest.Header.Set("Idempotency-Key", idempotencyKey)
	}
	if err = serviceauth.SignRequest(httpRequest, SignatureCaller, c.secret, body, c.now().UTC()); err != nil {
		return zero, fmt.Errorf("%w: sign", ErrAuthorityUnavailable)
	}
	response, err := c.http.Do(httpRequest)
	if err != nil {
		return zero, fmt.Errorf("%w: transport: %v", ErrAuthorityUnavailable, err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return zero, fmt.Errorf("%w: read", ErrAuthorityUnavailable)
	}
	var envelope Envelope[T]
	if err = json.Unmarshal(raw, &envelope); err != nil {
		return zero, fmt.Errorf("%w: malformed response", ErrAuthorityUnavailable)
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 && envelope.Success {
		return envelope.Data, nil
	}
	message := strings.TrimSpace(envelope.Error.Message)
	if message == "" {
		message = fmt.Sprintf("status %d", response.StatusCode)
	}
	if response.StatusCode == http.StatusConflict {
		return zero, fmt.Errorf("%w: %s", ErrConflict, message)
	}
	if response.StatusCode == http.StatusNotFound {
		return zero, fmt.Errorf("%w: %w: %s", ErrInvalidRequest, ErrMissionNotFound, message)
	}
	if response.StatusCode >= 400 && response.StatusCode < 500 {
		return zero, fmt.Errorf("%w: %s", ErrInvalidRequest, message)
	}
	return zero, fmt.Errorf("%w: %s", ErrAuthorityUnavailable, message)
}
