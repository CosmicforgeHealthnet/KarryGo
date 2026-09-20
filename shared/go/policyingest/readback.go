package policyingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// HTTPReader reads a referenced catalogue back from the control plane over the
// platform's HMAC v2 service auth.
//
// It never fetches anonymously. A document that decides which vehicles may
// take which work is not something to accept from an unauthenticated URL,
// however trustworthy the event that named it looked -- the event and the
// document travel separately and only the second one actually governs
// matching.
type HTTPReader struct {
	client      *http.Client
	callerName  string
	secret      []byte
	allowedHost map[string]bool
	maxBytes    int64
	now         func() time.Time
}

// ReaderOptions configure an HTTPReader.
type ReaderOptions struct {
	// CallerName is this service's own identity in the HMAC signature.
	CallerName string
	// Secret is the shared secret Admin verifies the signature with.
	Secret []byte
	// AllowedHosts restricts which hosts a document_ref may name.
	//
	// Without it, a publication could point this service at any URL and have
	// it sign an authenticated request to that host -- handing an attacker
	// both a request forgery and a signature oracle. An empty list disables
	// read-back entirely rather than allowing everything, because "allow all"
	// is never the safe reading of a missing allowlist.
	AllowedHosts []string
	Timeout      time.Duration
	// MaxDocumentBytes caps the response. A catalogue is kilobytes; an
	// unbounded read from a compromised or confused host is a memory
	// exhaustion vector.
	MaxDocumentBytes int64
	Now              func() time.Time
}

const (
	defaultReadBackTimeout = 10 * time.Second
	defaultMaxDocument     = 4 << 20 // 4 MiB
)

// NewHTTPReader builds the reader. It returns nil when the deployment has not
// configured read-back, so a referenced publication is parked pending rather
// than rejected.
func NewHTTPReader(options ReaderOptions) *HTTPReader {
	if strings.TrimSpace(options.CallerName) == "" || len(options.Secret) == 0 || len(options.AllowedHosts) == 0 {
		return nil
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultReadBackTimeout
	}
	maxBytes := options.MaxDocumentBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxDocument
	}
	now := options.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	hosts := make(map[string]bool, len(options.AllowedHosts))
	for _, host := range options.AllowedHosts {
		if trimmed := strings.ToLower(strings.TrimSpace(host)); trimmed != "" {
			hosts[trimmed] = true
		}
	}
	return &HTTPReader{
		client:      &http.Client{Timeout: timeout},
		callerName:  options.CallerName,
		secret:      options.Secret,
		allowedHost: hosts,
		now:         now,
		maxBytes:    maxBytes,
	}
}

// ReadDocument fetches and returns the catalogue.
//
// The retryable flag is the contract's most important output. It is true for
// every outcome where Admin MIGHT have served the document -- a timeout, a
// connection reset, a 5xx, a 429 -- and false only where the answer is a
// definite refusal that refetching cannot change.
func (r *HTTPReader) ReadDocument(ctx context.Context, ref string) (json.RawMessage, bool, error) {
	if r == nil {
		return nil, true, errors.New("policyingest: read-back is not configured")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, ref, nil)
	if err != nil {
		return nil, false, fmt.Errorf("policyingest: bad document_ref: %w", err)
	}
	host := strings.ToLower(request.URL.Hostname())
	if !r.allowedHost[host] {
		// Terminal on purpose. A reference to an unexpected host is not a
		// transient problem, and retrying it would keep re-attempting a
		// request forgery.
		return nil, false, fmt.Errorf("policyingest: document_ref host %q is not allowed", host)
	}

	// A GET has no body, and the signature must be computed over the same
	// empty body the server will hash.
	if err := serviceauth.SignRequest(request, r.callerName, r.secret, nil, r.now()); err != nil {
		return nil, false, fmt.Errorf("policyingest: sign read-back: %w", err)
	}

	response, err := r.client.Do(request)
	if err != nil {
		// Transport failures are ALWAYS unknown: the request may have been
		// served and the response lost. This is the single most important
		// branch in the package.
		return nil, true, fmt.Errorf("policyingest: read-back transport: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<16))
		_ = response.Body.Close()
	}()

	switch {
	case response.StatusCode == http.StatusOK:
	case response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500:
		return nil, true, fmt.Errorf("policyingest: read-back status %d", response.StatusCode)
	default:
		// 4xx other than 429 is a definite refusal: a bad signature, a gone
		// document, a malformed reference. Retrying repeats the same answer.
		return nil, false, fmt.Errorf("policyingest: read-back refused with status %d", response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, r.maxBytes+1))
	if err != nil {
		return nil, true, fmt.Errorf("policyingest: read-back body: %w", err)
	}
	if int64(len(body)) > r.maxBytes {
		return nil, false, errors.New("policyingest: read-back document exceeds the size limit")
	}
	if !json.Valid(body) {
		return nil, false, fmt.Errorf("%w: read-back document is not JSON", ErrInvalidPublication)
	}
	return json.RawMessage(body), false, nil
}

// Retryable classifies a transport error for callers that do their own fetch.
func Retryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
