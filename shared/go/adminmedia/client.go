package adminmedia

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"github.com/google/uuid"
)

var ErrInvalid = errors.New("invalid private Media case contract")
var ErrDenied = errors.New("private Media case access denied")
var ErrNotFound = errors.New("private Media case grant unavailable")
var ErrUnavailable = errors.New("private Media case authority unavailable")

const MaxBody = 32 << 10
const MaxContent = 32 << 20

type Client struct {
	origin *url.URL
	secret []byte
	http   *http.Client
}

func NewClient(origin string, secret []byte) (*Client, error) {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" || len(secret) < 32 {
		return nil, ErrInvalid
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: time.Second}).DialContext, ResponseHeaderTimeout: 2 * time.Second, MaxConnsPerHost: 8, MaxIdleConnsPerHost: 4, IdleConnTimeout: 30 * time.Second}
	return &Client{origin: u, secret: append([]byte{}, secret...), http: &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) Close() { c.http.CloseIdleConnections() }
func validID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed != uuid.Nil && parsed.String() == id
}
func (c *Client) call(ctx context.Context, path string, payload any, limit int64) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil || len(raw) > MaxBody {
		return nil, ErrInvalid
	}
	u := *c.origin
	u.Path = path
	u.RawPath = ""
	u.RawQuery = ""
	request, err := http.NewRequestWithContext(ctx, "POST", u.String(), bytes.NewReader(raw))
	if err != nil {
		return nil, ErrInvalid
	}
	request.Header.Set("Content-Type", "application/json")
	if serviceauth.SignRequest(request, Caller, c.secret, raw, time.Now()) != nil {
		return nil, ErrUnavailable
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case 200:
	case 401, 403:
		return nil, ErrDenied
	case 404:
		return nil, ErrNotFound
	default:
		return nil, ErrUnavailable
	}
	if read, ok := payload.(ReadRequest); ok {
		if response.Header.Get("X-Media-Case-Contract") != Contract || response.Header.Get("X-Media-Object-ID") != read.Request.MediaObjectID || response.Header.Get("Content-Type") != "application/octet-stream" {
			return nil, ErrInvalid
		}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(body)) > limit {
		return nil, ErrInvalid
	}
	return body, nil
}
func (c *Client) Issue(ctx context.Context, actor Actor, q Request) (Grant, error) {
	var empty Grant
	if !actor.Valid(time.Now()) || !q.Valid() {
		return empty, ErrInvalid
	}
	raw, err := c.call(ctx, GrantPath, IssueRequest{Actor: actor, Request: q}, MaxBody)
	if err != nil {
		return empty, err
	}
	var out Grant
	if Decode(raw, &out) != nil {
		return empty, ErrInvalid
	}
	token, err := base64.RawURLEncoding.DecodeString(out.Token)
	if out.ContractVersion != Contract || !validID(out.ID) || err != nil || len(token) != 32 || out.MediaObjectID != q.MediaObjectID || !out.ExpiresAt.After(time.Now()) || out.ExpiresAt.After(actor.ExpiresAt) {
		return empty, ErrInvalid
	}
	return out, nil
}
func (c *Client) Read(ctx context.Context, actor Actor, q Request, id, token string) ([]byte, error) {
	if !actor.Valid(time.Now()) || !q.Valid() || !validID(id) || len(token) != 43 {
		return nil, ErrInvalid
	}
	return c.call(ctx, GrantPath+"/"+id+"/content", ReadRequest{Actor: actor, Request: q, Token: token}, MaxContent)
}
func (c *Client) Revoke(ctx context.Context, actor Actor, id, reason string) error {
	if !actor.Valid(time.Now()) || !validID(id) || !adminread.Opaque(reason) {
		return ErrInvalid
	}
	raw, err := c.call(ctx, GrantPath+"/"+id+"/revocations", RevokeRequest{Actor: actor, ReasonCode: reason}, 1024)
	if err != nil {
		return err
	}
	var out struct {
		Status string `json:"status"`
	}
	if Decode(raw, &out) != nil || out.Status != "REVOKED" {
		return ErrInvalid
	}
	return nil
}

// Decode rejects duplicate/case-alias keys, unknown fields, nested abuse and
// trailing documents before decoding into an exact private contract.
func Decode(raw []byte, out any) error {
	if len(raw) == 0 || len(raw) > MaxBody {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if unique(d, 0) != nil {
		return ErrInvalid
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrInvalid
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return ErrInvalid
	}
	return nil
}
func unique(d *json.Decoder, depth int) error {
	if depth > 12 {
		return ErrInvalid
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return ErrInvalid
			}
			name = strings.ToLower(name)
			if seen[name] {
				return ErrInvalid
			}
			seen[name] = true
			if err := unique(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := unique(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return ErrInvalid
	}
	_, err = d.Token()
	return err
}
