package admincase

import (
	"bytes"
	"context"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrUnavailable = errors.New("source case authorization unavailable")
var ErrInvalid = errors.New("invalid source case contract")

type Client struct {
	owner  string
	origin *url.URL
	secret []byte
	http   *http.Client
}

func NewClient(owner, origin string, key []byte) (*Client, error) {
	known := false
	for _, s := range Specs() {
		known = known || s.Owner == owner
	}
	u, err := url.Parse(origin)
	if err != nil || !known || len(key) < 32 || u.Scheme != "http" && u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return nil, ErrInvalid
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: time.Second}).DialContext, ResponseHeaderTimeout: 2 * time.Second, MaxConnsPerHost: 8, IdleConnTimeout: 30 * time.Second}
	return &Client{owner: owner, origin: u, secret: append([]byte{}, key...), http: &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) Close() { c.http.CloseIdleConnections() }
func (c *Client) Authorize(ctx context.Context, q adminread.Query, kind, ref, subject, subjectRef string) (Proof, error) {
	var empty Proof
	spec, ok := Find(c.owner, kind)
	if !ok || len(spec.Subjects[subject]) == 0 || q.Limit != 1 || q.Cursor != "" || q.Validate(time.Now()) != nil {
		return empty, ErrInvalid
	}
	for _, part := range []string{ref, subjectRef} {
		if !adminread.Opaque(part) || strings.ContainsAny(part, "/\\") || part == "." || part == ".." {
			return empty, ErrInvalid
		}
	}
	u := *c.origin
	u.Path = spec.Path() + "/proofs/" + kind + "/" + ref + "/" + subject + "/" + subjectRef
	u.RawQuery = q.Values().Encode()
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return empty, ErrInvalid
	}
	if serviceauth.SignRequest(r, Caller, c.secret, nil, time.Now()) != nil {
		return empty, ErrUnavailable
	}
	response, err := c.http.Do(r)
	if err != nil {
		return empty, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return empty, ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 16385))
	if err != nil || len(raw) > 16384 || Strict(raw, &empty) != nil {
		return Proof{}, ErrInvalid
	}
	expected := adminpolicy.Scope{Product: "karryam", Market: q.Scope.Market, Zone: q.Scope.Zone, Organization: q.Scope.Organization, Tenant: q.Scope.Tenant}
	hash, e := hex.DecodeString(empty.NativeFingerprint)
	if empty.ContractVersion != Contract || empty.Owner != c.owner || empty.CaseKind != kind || empty.CaseReference != ref || empty.SubjectType != subject || empty.SubjectReference != subjectRef || empty.SourceContract != spec.SourceContract || empty.Scope != expected || empty.PolicyID != q.PolicyID || empty.PolicyVersion != q.PolicyVersion || empty.PolicyChecksum != q.PolicyChecksum || !empty.ExpiresAt.After(time.Now()) || e != nil || len(hash) != 32 || hex.EncodeToString(hash) != empty.NativeFingerprint {
		return Proof{}, ErrInvalid
	}
	return empty, nil
}

// Strict rejects JSON aliases/duplicates before typed unknown-field decoding.
func Strict(raw []byte, out any) error {
	if len(raw) == 0 || len(raw) > 16384 {
		return ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 6 {
			return ErrInvalid
		}
		token, err := d.Token()
		if err != nil {
			return err
		}
		switch token {
		case json.Delim('{'):
			seen := map[string]bool{}
			for d.More() {
				token, err = d.Token()
				key, ok := token.(string)
				if err != nil || !ok || key != strings.ToLower(key) || seen[key] {
					return ErrInvalid
				}
				seen[key] = true
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
			token, err = d.Token()
			if err != nil || token != json.Delim('}') {
				return ErrInvalid
			}
		case json.Delim('['):
			for d.More() {
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
			token, err = d.Token()
			if err != nil || token != json.Delim(']') {
				return ErrInvalid
			}
		case json.Delim('}'), json.Delim(']'):
			return ErrInvalid
		}
		return nil
	}
	if walk(0) != nil {
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
