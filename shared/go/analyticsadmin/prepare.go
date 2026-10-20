package analyticsadmin

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/admincontext"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// PrepareBinding fetches minimized native lineage facts for independent owner
// review. The response is a draft, never an authorization or signed allocation.
func (c *PublisherClient) PrepareBinding(ctx context.Context, kind, eventID, purpose string) (admincontext.RecordBinding, error) {
	var empty admincontext.RecordBinding
	for _, v := range []string{kind, eventID} {
		if !adminread.Opaque(v) || v == "." || v == ".." || strings.ContainsAny(v, "/\\%?#") {
			return empty, ErrInvalid
		}
	}
	if !adminread.Opaque(purpose) {
		return empty, ErrInvalid
	}
	u := *c.transport.origin
	u.Path = SourceContextPath + c.owner + "/bindings/" + kind + "/" + eventID
	u.RawPath = ""
	u.RawQuery = url.Values{"purpose": {purpose}}.Encode()
	r, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return empty, ErrInvalid
	}
	if serviceauth.SignRequest(r, ProducerPublisherCaller, c.transport.secret, nil, time.Now()) != nil {
		return empty, ErrUnavailable
	}
	response, err := c.transport.http.Do(r)
	if err != nil {
		return empty, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode == 401 || response.StatusCode == 403 {
		return empty, ErrDenied
	}
	if response.StatusCode != 200 {
		return empty, ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	if err != nil || len(raw) > 4096 {
		return empty, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if unique(d, 0) != nil {
		return empty, ErrInvalid
	}
	if _, err := d.Token(); err != io.EOF {
		return empty, ErrInvalid
	}
	var out struct {
		State   string                     `json:"state"`
		Binding admincontext.RecordBinding `json:"binding"`
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&out) != nil {
		return empty, ErrInvalid
	}
	hash, err := hex.DecodeString(out.Binding.Fingerprint)
	if err != nil || len(hash) != 32 || out.Binding.SourceContract != "analytics.cohort-lineage.v1:"+kind || out.State != "DRAFT_PENDING_OWNER_APPROVAL" || out.Binding.Canonical != (adminpolicy.Scope{Product: "karryam"}) {
		return empty, ErrInvalid
	}
	return out.Binding, nil
}
